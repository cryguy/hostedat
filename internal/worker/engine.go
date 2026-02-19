package worker

import (
	"encoding/json"
	"fmt"
	"log"
	"runtime"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/cryguy/hostedat/internal/config"
	"github.com/cryguy/hostedat/internal/models"
	"github.com/cryguy/hostedat/internal/storage"
	v8 "github.com/tommie/v8go"
	"gorm.io/gorm"
)

// wsConnectionTimeout is the maximum duration for a WebSocket connection.
const wsConnectionTimeout = 5 * time.Minute

// maxWSMessageBytes is the maximum size of a single WebSocket message (64 KB).
const maxWSMessageBytes = 64 * 1024

// poolKey uniquely identifies a compiled worker deployment for a site.
type poolKey struct {
	SiteID    string
	DeployKey string
}

// sitePool wraps a v8Pool with an invalidation flag so that stale pools
// are replaced transparently on the next Execute call.
type sitePool struct {
	pool    *v8Pool
	invalid bool
	mu      sync.RWMutex
}

func (sp *sitePool) isValid() bool {
	sp.mu.RLock()
	defer sp.mu.RUnlock()
	return !sp.invalid
}

func (sp *sitePool) markInvalid() {
	sp.mu.Lock()
	defer sp.mu.Unlock()
	sp.invalid = true
}

// S3Client abstracts the minio.Client methods used by worker storage bindings.
type S3Client interface {
	GetObject(ctx interface{}, bucketName, objectName string, opts interface{}) (interface{}, error)
}

// Engine manages per-site worker pools and executes JS worker scripts.
type Engine struct {
	pools              sync.Map // poolKey -> *sitePool
	sources            sync.Map // poolKey -> string (JS source)
	config             config.WorkerConfig
	db                 *gorm.DB
	store              *storage.Manager
	minioClient        interface{} // *minio.Client
	presignMinioClient interface{} // *minio.Client for presigned URL generation
	publicS3URL        string
	logDone            chan struct{}
}

// NewEngine creates an Engine with the given configuration and database handle.
func NewEngine(cfg config.WorkerConfig, db *gorm.DB) *Engine {
	e := &Engine{
		config:  cfg,
		db:      db,
		logDone: make(chan struct{}),
	}
	go e.logRetentionLoop()
	return e
}

// SetStore sets the storage manager for source reload on server restart.
func (e *Engine) SetStore(store *storage.Manager) {
	e.store = store
}

// SetMinioClient sets the minio-go client for R2-compatible storage bindings.
func (e *Engine) SetMinioClient(client interface{}) {
	e.minioClient = client
}

// SetPresignMinioClient sets the minio-go client used for presigned URL generation.
func (e *Engine) SetPresignMinioClient(client interface{}) {
	e.presignMinioClient = client
}

// SetPublicS3URL sets the public-facing S3 URL used for direct object URLs.
func (e *Engine) SetPublicS3URL(u string) {
	e.publicS3URL = u
}

// logRetentionLoop deletes worker logs older than max_log_retention days.
func (e *Engine) logRetentionLoop() {
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-e.logDone:
			return
		case <-ticker.C:
			cutoff := time.Now().AddDate(0, 0, -e.config.MaxLogRetention)
			if result := e.db.Where("created_at < ?", cutoff).Delete(&models.WorkerLog{}); result.Error != nil {
				log.Printf("worker log cleanup error: %v", result.Error)
			}
		}
	}
}

// EnsureSource loads the worker JS source into memory if not already cached.
// Handles the server restart scenario where in-memory caches are lost.
func (e *Engine) EnsureSource(siteID string, deployKey string) error {
	key := poolKey{SiteID: siteID, DeployKey: deployKey}
	if _, ok := e.sources.Load(key); ok {
		return nil
	}

	if e.store == nil {
		return fmt.Errorf("storage manager not set")
	}

	source, err := e.store.GetWorkerScript(siteID, deployKey)
	if err != nil {
		return fmt.Errorf("no source for site %s deploy %s: %w", siteID, deployKey, err)
	}

	e.sources.Store(key, source)
	return nil
}

// CompileAndCache validates that a worker script compiles and stores the
// source for later pool creation. Returns the source bytes for disk storage.
func (e *Engine) CompileAndCache(siteID string, deployKey string, source string) ([]byte, error) {
	key := poolKey{SiteID: siteID, DeployKey: deployKey}

	// Validate the source compiles in a temporary isolate.
	iso := v8.NewIsolate()
	defer iso.Dispose()

	wrapped := wrapESModule(source)
	if _, err := iso.CompileUnboundScript(wrapped, "worker.js", v8.CompileOptions{}); err != nil {
		return nil, fmt.Errorf("compiling worker script: %w", err)
	}

	e.sources.Store(key, source)
	return []byte(source), nil
}

// GetOrCreatePool returns the worker pool for the given site/deploy,
// creating it if necessary. Each worker in the pool has all Web APIs,
// console, fetch, crypto, and the compiled worker script loaded.
func (e *Engine) GetOrCreatePool(siteID string, deployKey string, env *Env) (*v8Pool, error) {
	key := poolKey{SiteID: siteID, DeployKey: deployKey}

	// Check for a valid existing pool.
	if val, ok := e.pools.Load(key); ok {
		sp := val.(*sitePool)
		if sp.isValid() {
			return sp.pool, nil
		}
		e.pools.Delete(key)
		sp.pool.dispose()
	}

	// Load source.
	srcVal, ok := e.sources.Load(key)
	if !ok {
		return nil, fmt.Errorf("no source for site %s deploy %s", siteID, deployKey)
	}
	source := srcVal.(string)

	cfg := e.config

	setupFns := []setupFunc{
		// Web APIs: Headers, Request, Response, URL, URLSearchParams, TextEncoder/Decoder
		setupWebAPIs,
		setupURLSearchParamsExt,
		// Globals: structuredClone, performance.now(), navigator, queueMicrotask
		setupGlobals,
		// Encoding: atob, btoa
		setupEncoding,
		// Timers: setTimeout, setInterval, clearTimeout, clearInterval (real event loop)
		setupTimers,
		// Abort: AbortController, AbortSignal, Event, EventTarget, DOMException
		setupAbort,
		// Crypto: crypto.getRandomValues, crypto.subtle, crypto.randomUUID
		setupCrypto,
		// Crypto extensions: JWK, ECDSA, AES-CBC, generateKey
		setupCryptoExt,
		// Crypto: HKDF, PBKDF2 deriveBits/deriveKey
		setupCryptoDerive,
		// Crypto: RSA-OAEP, RSASSA-PKCS1-v1_5, RSA-PSS
		setupCryptoRSA,
		// Crypto: Ed25519 sign/verify
		setupCryptoEd25519,
		// URLPattern: URL pattern matching API
		setupURLPattern,
		// Streams: ReadableStream, WritableStream, TransformStream
		setupStreams,
		// TextStreams: TextEncoderStream, TextDecoderStream, IdentityTransformStream
		setupTextStreams,
		// FormData: FormData, Blob, File
		setupFormData,
		// Compression: CompressionStream, DecompressionStream
		setupCompression,
		// Body types: patches Request/Response for non-string bodies
		setupBodyTypes,
		// WebSocket: WebSocketPair, WebSocket class
		setupWebSocket,
		// HTMLRewriter: streaming HTML transformation
		setupHTMLRewriter,
		// Console: log/info/warn/error/debug capture
		setupConsole,
		// Fetch: Go-backed fetch() with SSRF protection
		func(iso *v8.Isolate, ctx *v8.Context, el *eventLoop) error {
			return setupFetch(iso, ctx, el, cfg)
		},
		// BYOB Reader: ReadableStreamBYOBReader, ReadableByteStreamController
		setupBYOBReader,
		// MessageChannel: MessageChannel, MessagePort
		setupMessageChannel,
		// Unhandled Rejection: PromiseRejectionEvent, unhandledrejection tracking
		setupUnhandledRejection,
		// Scheduler: scheduler.wait()
		setupScheduler,
		// DigestStream: crypto.DigestStream for streaming hash computation
		setupDigestStream,
		// EventSource: Server-Sent Events client
		setupEventSource,
		// TCP Sockets: connect() for outbound TCP connections
		setupTCPSocket,
		// D1: SQL database bindings (per-binding setup in buildEnvObject)
		setupD1,
		// Cache: Cache API (caches.default, caches.open)
		setupCache,
	}

	pool, err := newV8Pool(cfg.PoolSize, source, setupFns, e.config.MemoryLimitMB)
	if err != nil {
		return nil, fmt.Errorf("creating v8 pool: %w", err)
	}

	sp := &sitePool{pool: pool}
	e.pools.Store(key, sp)
	return pool, nil
}

// Execute runs the worker's fetch handler for the given request and returns
// the result including the response, captured logs, and any error.
func (e *Engine) Execute(siteID string, deployKey string, env *Env, req *WorkerRequest) (result *WorkerResult) {
	start := time.Now()
	result = &WorkerResult{}

	// Set engine and DB references so buildEnvObject can wire up service bindings
	// and global APIs like Cache can access the database.
	env.engine = e
	env.db = e.db
	env.d1DataDir = e.config.DataDir

	// Ensure source is loaded (handles server restart).
	if err := e.EnsureSource(siteID, deployKey); err != nil {
		result.Error = err
		result.Duration = time.Since(start)
		return result
	}

	pool, err := e.GetOrCreatePool(siteID, deployKey, env)
	if err != nil {
		result.Error = err
		result.Duration = time.Since(start)
		return result
	}

	w, err := pool.get()
	if err != nil {
		result.Error = fmt.Errorf("acquiring worker from pool: %w", err)
		result.Duration = time.Since(start)
		return result
	}

	// keepWorker is set to true when a WebSocket upgrade response is detected,
	// preventing the deferred cleanup from returning the worker to the pool.
	var keepWorker bool

	// Watchdog: iso.TerminateExecution() is the one thread-safe V8 call.
	var timedOut atomic.Bool
	timeout := time.Duration(e.config.ExecutionTimeout) * time.Millisecond
	watchdog := time.AfterFunc(timeout, func() {
		timedOut.Store(true)
		w.iso.TerminateExecution()
	})

	var panicked bool
	defer func() {
		stopped := watchdog.Stop()
		if r := recover(); r != nil {
			panicked = true
			if timedOut.Load() {
				result.Error = fmt.Errorf("worker execution timed out (limit: %v)", timeout)
			} else {
				result.Error = fmt.Errorf("worker panic: %v", r)
			}
		}
		result.Duration = time.Since(start)
		// WebSocket: worker is managed by WebSocketHandler.Bridge().
		if keepWorker {
			return
		}
		// Only return healthy workers to the pool.
		if stopped && !timedOut.Load() && !panicked {
			pool.put(w)
		} else {
			log.Printf("worker: discarding worker for site %s deploy %s (timed out or panicked)", siteID, deployKey)
			w.ctx.Close()
			w.iso.Dispose()
			key := poolKey{SiteID: siteID, DeployKey: deployKey}
			if val, ok := e.pools.Load(key); ok {
				sp := val.(*sitePool)
				sp.markInvalid()
			}
		}
	}()

	iso := w.iso
	ctx := w.ctx

	// Set up per-request state.
	reqID := newRequestState(e.config.MaxFetchRequests, env)
	reqIDVal, _ := v8.NewValue(iso, strconv.FormatUint(reqID, 10))
	if err := ctx.Global().Set("__requestID", reqIDVal); err != nil {
		clearRequestState(reqID)
		result.Error = fmt.Errorf("setting request ID: %w", err)
		return result
	}

	// Build the JS arguments: request, env, ctx.
	jsReq, err := goRequestToJS(iso, ctx, req)
	if err != nil {
		clearRequestState(reqID)
		result.Error = fmt.Errorf("building JS request: %w", err)
		return result
	}

	jsEnv, err := buildEnvObject(iso, ctx, env, e.db, e.minioClient, e.presignMinioClient, e.publicS3URL)
	if err != nil {
		state := clearRequestState(reqID)
		if state != nil {
			result.Logs = state.logs
		}
		result.Error = fmt.Errorf("building JS env: %w", err)
		return result
	}

	jsCtx, err := buildExecContext(iso, ctx)
	if err != nil {
		state := clearRequestState(reqID)
		if state != nil {
			result.Logs = state.logs
		}
		result.Error = fmt.Errorf("building JS context: %w", err)
		return result
	}

	// Call __worker_module__.fetch(request, env, ctx).
	moduleVal, err := ctx.Global().Get("__worker_module__")
	if err != nil || moduleVal.IsUndefined() || moduleVal.IsNull() {
		state := clearRequestState(reqID)
		if state != nil {
			result.Logs = state.logs
		}
		result.Error = fmt.Errorf("worker module has no default export")
		return result
	}

	moduleObj, err := moduleVal.AsObject()
	if err != nil {
		state := clearRequestState(reqID)
		if state != nil {
			result.Logs = state.logs
		}
		result.Error = fmt.Errorf("worker module is not an object: %w", err)
		return result
	}

	fetchVal, err := moduleObj.Get("fetch")
	if err != nil || fetchVal.IsUndefined() {
		state := clearRequestState(reqID)
		if state != nil {
			result.Logs = state.logs
		}
		result.Error = fmt.Errorf("worker module has no fetch handler")
		return result
	}

	fetchFn, err := fetchVal.AsFunction()
	if err != nil {
		state := clearRequestState(reqID)
		if state != nil {
			result.Logs = state.logs
		}
		result.Error = fmt.Errorf("worker fetch is not a function: %w", err)
		return result
	}

	fetchResult, err := fetchFn.Call(moduleObj, jsReq, jsEnv, jsCtx)
	if err != nil {
		state := clearRequestState(reqID)
		if state != nil {
			result.Logs = state.logs
		}
		if timedOut.Load() {
			result.Error = fmt.Errorf("worker execution timed out (limit: %v)", timeout)
		} else {
			result.Error = fmt.Errorf("invoking worker fetch: %w", err)
		}
		return result
	}

	// Pump microtasks to settle any immediately-resolved promises.
	ctx.PerformMicrotaskCheckpoint()

	// Drain event loop (fire timers, each followed by microtask checkpoint).
	deadline := start.Add(timeout)
	if w.eventLoop.hasPending() {
		w.eventLoop.drain(iso, ctx, deadline)
	}

	// Await the result if it's a Promise.
	fetchResult, err = awaitValue(ctx, fetchResult, deadline)
	if err != nil {
		state := clearRequestState(reqID)
		if state != nil {
			result.Logs = state.logs
		}
		result.Error = fmt.Errorf("awaiting worker response: %w", err)
		return result
	}

	// Convert JS Response to Go.
	resp, err := jsResponseToGo(ctx, fetchResult)

	if err != nil {
		state := clearRequestState(reqID)
		if state != nil {
			result.Logs = state.logs
		}
		result.Error = fmt.Errorf("converting worker response: %w", err)
		return result
	}

	// Drain waitUntil promises after the response is ready.
	drainWaitUntil(ctx, deadline)

	// WebSocket upgrade: if the response is 101 with a webSocket, hold the
	// worker for the bridge loop instead of returning it to the pool.
	if resp.HasWebSocket && resp.StatusCode == 101 {
		// Store the server WebSocket reference for the bridge.
		ctx.RunScript(`
			if (globalThis.__ws_check_resp && globalThis.__ws_check_resp._peer) {
				globalThis.__ws_active_server = globalThis.__ws_check_resp._peer;
			}
			delete globalThis.__ws_check_resp;
		`, "ws_setup_bridge.js")

		state := getRequestState(reqID)
		if state != nil {
			result.Logs = state.logs
		}

		keepWorker = true
		result.Response = resp
		result.WebSocket = &WebSocketHandler{
			worker:  w,
			pool:    pool,
			reqID:   reqID,
			timeout: wsConnectionTimeout,
		}
		return result
	}

	state := clearRequestState(reqID)
	if state != nil {
		result.Logs = state.logs
	}

	result.Response = resp
	return result
}

// ExecuteScheduled runs the worker's scheduled handler for cron triggers.
func (e *Engine) ExecuteScheduled(siteID string, deployKey string, env *Env, cron string) (result *WorkerResult) {
	start := time.Now()
	result = &WorkerResult{}

	env.engine = e
	env.db = e.db
	env.d1DataDir = e.config.DataDir

	if err := e.EnsureSource(siteID, deployKey); err != nil {
		result.Error = err
		result.Duration = time.Since(start)
		return result
	}

	pool, err := e.GetOrCreatePool(siteID, deployKey, env)
	if err != nil {
		result.Error = err
		result.Duration = time.Since(start)
		return result
	}

	w, err := pool.get()
	if err != nil {
		result.Error = fmt.Errorf("acquiring worker from pool: %w", err)
		result.Duration = time.Since(start)
		return result
	}

	var timedOut atomic.Bool
	timeout := time.Duration(e.config.ExecutionTimeout) * time.Millisecond
	watchdog := time.AfterFunc(timeout, func() {
		timedOut.Store(true)
		w.iso.TerminateExecution()
	})

	var panicked bool
	defer func() {
		stopped := watchdog.Stop()
		if r := recover(); r != nil {
			panicked = true
			if timedOut.Load() {
				result.Error = fmt.Errorf("worker execution timed out (limit: %v)", timeout)
			} else {
				result.Error = fmt.Errorf("worker panic: %v", r)
			}
		}
		result.Duration = time.Since(start)
		if stopped && !timedOut.Load() && !panicked {
			pool.put(w)
		} else {
			log.Printf("worker: discarding scheduled worker for site %s deploy %s (timed out or panicked)", siteID, deployKey)
			w.ctx.Close()
			w.iso.Dispose()
			key := poolKey{SiteID: siteID, DeployKey: deployKey}
			if val, ok := e.pools.Load(key); ok {
				sp := val.(*sitePool)
				sp.markInvalid()
			}
		}
	}()

	iso := w.iso
	ctx := w.ctx

	// Set up per-request state.
	reqID := newRequestState(e.config.MaxFetchRequests, env)
	reqIDVal, _ := v8.NewValue(iso, strconv.FormatUint(reqID, 10))
	ctx.Global().Set("__requestID", reqIDVal)

	// Build the scheduled event using the ScheduledEvent class.
	scheduledTimeMs := float64(time.Now().UnixMilli())
	eventScript := fmt.Sprintf(`new ScheduledEvent(%f, %q)`, scheduledTimeMs, cron)
	eventVal, err := ctx.RunScript(eventScript, "scheduled_event.js")
	if err != nil {
		clearRequestState(reqID)
		result.Error = fmt.Errorf("creating ScheduledEvent: %w", err)
		return result
	}
	event, err := eventVal.AsObject()
	if err != nil {
		clearRequestState(reqID)
		result.Error = fmt.Errorf("ScheduledEvent is not an object: %w", err)
		return result
	}

	jsEnv, err := buildEnvObject(iso, ctx, env, e.db, e.minioClient, e.presignMinioClient, e.publicS3URL)
	if err != nil {
		state := clearRequestState(reqID)
		if state != nil {
			result.Logs = state.logs
		}
		result.Error = fmt.Errorf("building JS env: %w", err)
		return result
	}

	jsCtx, err := buildExecContext(iso, ctx)
	if err != nil {
		state := clearRequestState(reqID)
		if state != nil {
			result.Logs = state.logs
		}
		result.Error = fmt.Errorf("building JS context: %w", err)
		return result
	}

	// Call __worker_module__.scheduled(event, env, ctx).
	moduleVal, err := ctx.Global().Get("__worker_module__")
	if err != nil || moduleVal.IsUndefined() || moduleVal.IsNull() {
		state := clearRequestState(reqID)
		if state != nil {
			result.Logs = state.logs
		}
		result.Error = fmt.Errorf("worker module has no default export")
		return result
	}

	moduleObj, err := moduleVal.AsObject()
	if err != nil {
		state := clearRequestState(reqID)
		if state != nil {
			result.Logs = state.logs
		}
		result.Error = fmt.Errorf("worker module is not an object: %w", err)
		return result
	}

	scheduledVal, err := moduleObj.Get("scheduled")
	if err != nil || scheduledVal.IsUndefined() {
		state := clearRequestState(reqID)
		if state != nil {
			result.Logs = state.logs
		}
		result.Error = fmt.Errorf("worker module has no scheduled handler")
		return result
	}

	scheduledFn, err := scheduledVal.AsFunction()
	if err != nil {
		state := clearRequestState(reqID)
		if state != nil {
			result.Logs = state.logs
		}
		result.Error = fmt.Errorf("worker scheduled is not a function: %w", err)
		return result
	}

	schedResult, err := scheduledFn.Call(moduleObj, event, jsEnv, jsCtx)
	if err != nil {
		state := clearRequestState(reqID)
		if state != nil {
			result.Logs = state.logs
		}
		result.Error = fmt.Errorf("invoking worker scheduled: %w", err)
		return result
	}

	// Pump microtasks and drain event loop.
	ctx.PerformMicrotaskCheckpoint()
	deadline := start.Add(timeout)
	if w.eventLoop.hasPending() {
		w.eventLoop.drain(iso, ctx, deadline)
	}

	// Await if the handler returns a promise.
	if schedResult != nil && schedResult.IsPromise() {
		if _, err := awaitValue(ctx, schedResult, deadline); err != nil {
			state := clearRequestState(reqID)
			if state != nil {
				result.Logs = state.logs
			}
			result.Error = fmt.Errorf("awaiting scheduled handler: %w", err)
			return result
		}
	}

	// Drain waitUntil promises after the handler completes.
	drainWaitUntil(ctx, deadline)

	state := clearRequestState(reqID)
	if state != nil {
		result.Logs = state.logs
	}
	return result
}

// ExecuteTail runs the worker's tail handler for log forwarding.
func (e *Engine) ExecuteTail(siteID string, deployKey string, env *Env, events []TailEvent) (result *WorkerResult) {
	start := time.Now()
	result = &WorkerResult{}

	env.engine = e
	env.db = e.db
	env.d1DataDir = e.config.DataDir

	if err := e.EnsureSource(siteID, deployKey); err != nil {
		result.Error = err
		result.Duration = time.Since(start)
		return result
	}

	pool, err := e.GetOrCreatePool(siteID, deployKey, env)
	if err != nil {
		result.Error = err
		result.Duration = time.Since(start)
		return result
	}

	w, err := pool.get()
	if err != nil {
		result.Error = fmt.Errorf("acquiring worker from pool: %w", err)
		result.Duration = time.Since(start)
		return result
	}

	var timedOut atomic.Bool
	timeout := time.Duration(e.config.ExecutionTimeout) * time.Millisecond
	watchdog := time.AfterFunc(timeout, func() {
		timedOut.Store(true)
		w.iso.TerminateExecution()
	})

	var panicked bool
	defer func() {
		stopped := watchdog.Stop()
		if r := recover(); r != nil {
			panicked = true
			if timedOut.Load() {
				result.Error = fmt.Errorf("worker execution timed out (limit: %v)", timeout)
			} else {
				result.Error = fmt.Errorf("worker panic: %v", r)
			}
		}
		result.Duration = time.Since(start)
		if stopped && !timedOut.Load() && !panicked {
			pool.put(w)
		} else {
			log.Printf("worker: discarding tail worker for site %s deploy %s (timed out or panicked)", siteID, deployKey)
			w.ctx.Close()
			w.iso.Dispose()
			key := poolKey{SiteID: siteID, DeployKey: deployKey}
			if val, ok := e.pools.Load(key); ok {
				sp := val.(*sitePool)
				sp.markInvalid()
			}
		}
	}()

	iso := w.iso
	ctx := w.ctx

	// Set up per-request state.
	reqID := newRequestState(e.config.MaxFetchRequests, env)
	reqIDVal, _ := v8.NewValue(iso, strconv.FormatUint(reqID, 10))
	ctx.Global().Set("__requestID", reqIDVal)

	// Serialize tail events to JSON and inject into JS context.
	eventsJSON, err := json.Marshal(events)
	if err != nil {
		clearRequestState(reqID)
		result.Error = fmt.Errorf("marshaling tail events: %w", err)
		return result
	}
	eventsScript := fmt.Sprintf(`JSON.parse(%q)`, string(eventsJSON))
	jsEvents, err := ctx.RunScript(eventsScript, "tail_events.js")
	if err != nil {
		clearRequestState(reqID)
		result.Error = fmt.Errorf("creating tail events array: %w", err)
		return result
	}

	jsEnv, err := buildEnvObject(iso, ctx, env, e.db, e.minioClient, e.presignMinioClient, e.publicS3URL)
	if err != nil {
		state := clearRequestState(reqID)
		if state != nil {
			result.Logs = state.logs
		}
		result.Error = fmt.Errorf("building JS env: %w", err)
		return result
	}

	jsCtx, err := buildExecContext(iso, ctx)
	if err != nil {
		state := clearRequestState(reqID)
		if state != nil {
			result.Logs = state.logs
		}
		result.Error = fmt.Errorf("building JS context: %w", err)
		return result
	}

	// Call __worker_module__.tail(events, env, ctx).
	moduleVal, err := ctx.Global().Get("__worker_module__")
	if err != nil || moduleVal.IsUndefined() || moduleVal.IsNull() {
		state := clearRequestState(reqID)
		if state != nil {
			result.Logs = state.logs
		}
		result.Error = fmt.Errorf("worker module has no default export")
		return result
	}

	moduleObj, err := moduleVal.AsObject()
	if err != nil {
		state := clearRequestState(reqID)
		if state != nil {
			result.Logs = state.logs
		}
		result.Error = fmt.Errorf("worker module is not an object: %w", err)
		return result
	}

	tailVal, err := moduleObj.Get("tail")
	if err != nil || tailVal.IsUndefined() {
		state := clearRequestState(reqID)
		if state != nil {
			result.Logs = state.logs
		}
		result.Error = fmt.Errorf("worker module has no tail handler")
		return result
	}

	tailFn, err := tailVal.AsFunction()
	if err != nil {
		state := clearRequestState(reqID)
		if state != nil {
			result.Logs = state.logs
		}
		result.Error = fmt.Errorf("worker tail is not a function: %w", err)
		return result
	}

	tailResult, err := tailFn.Call(moduleObj, jsEvents, jsEnv, jsCtx)
	if err != nil {
		state := clearRequestState(reqID)
		if state != nil {
			result.Logs = state.logs
		}
		if timedOut.Load() {
			result.Error = fmt.Errorf("worker execution timed out (limit: %v)", timeout)
		} else {
			result.Error = fmt.Errorf("invoking worker tail: %w", err)
		}
		return result
	}

	// Pump microtasks and drain event loop.
	ctx.PerformMicrotaskCheckpoint()
	deadline := start.Add(timeout)
	if w.eventLoop.hasPending() {
		w.eventLoop.drain(iso, ctx, deadline)
	}

	// Await if the handler returns a promise.
	if tailResult != nil && tailResult.IsPromise() {
		if _, err := awaitValue(ctx, tailResult, deadline); err != nil {
			state := clearRequestState(reqID)
			if state != nil {
				result.Logs = state.logs
			}
			result.Error = fmt.Errorf("awaiting tail handler: %w", err)
			return result
		}
	}

	// Drain waitUntil promises after the handler completes.
	drainWaitUntil(ctx, deadline)

	state := clearRequestState(reqID)
	if state != nil {
		result.Logs = state.logs
	}
	return result
}

// InvalidatePool marks the pool for the given site/deploy as invalid.
// The next Execute call will create a fresh pool.
func (e *Engine) InvalidatePool(siteID string, deployKey string) {
	key := poolKey{SiteID: siteID, DeployKey: deployKey}
	if val, ok := e.pools.LoadAndDelete(key); ok {
		sp := val.(*sitePool)
		sp.markInvalid()
		sp.pool.dispose()
	}
	e.sources.Delete(key)
}

// Shutdown invalidates all pools and clears all cached sources.
func (e *Engine) Shutdown() {
	close(e.logDone)
	e.pools.Range(func(key, val any) bool {
		sp := val.(*sitePool)
		sp.markInvalid()
		sp.pool.dispose()
		e.pools.Delete(key)
		return true
	})
	e.sources.Range(func(key, _ any) bool {
		e.sources.Delete(key)
		return true
	})
}

// MaxResponseBytes returns the configured maximum response body size.
func (e *Engine) MaxResponseBytes() int {
	return e.config.MaxResponseBytes
}

// BuildEnvFromDB loads environment variables, secrets, and KV/storage bindings
// for a site from the database.
func BuildEnvFromDB(db *gorm.DB, siteID string, assets AssetsFetcher) *Env {
	env := &Env{
		Vars:       make(map[string]string),
		Secrets:    make(map[string]string),
		KVBindings: make(map[string]string),
		Assets:     assets,
	}

	var envVars []models.WorkerEnvVar
	db.Where("site_id = ?", siteID).Find(&envVars)
	for _, ev := range envVars {
		if ev.Secret {
			env.Secrets[ev.Name] = ev.Value
		} else {
			env.Vars[ev.Name] = ev.Value
		}
	}

	var kvNamespaces []models.KVNamespace
	db.Where("site_id = ?", siteID).Find(&kvNamespaces)
	for _, ns := range kvNamespaces {
		env.KVBindings[ns.Name] = ns.ID
	}

	env.StorageBindings = make(map[string]string)
	var storageBuckets []models.StorageBucket
	db.Where("site_id = ?", siteID).Find(&storageBuckets)
	for _, b := range storageBuckets {
		env.StorageBindings[b.Name] = b.BucketName
	}

	return env
}

// awaitValue resolves a potentially-promise value by pumping V8's microtask
// queue. Uses JS-side Promise.resolve().then() to capture the result,
// avoiding the need for a direct AsPromise() API.
func awaitValue(ctx *v8.Context, val *v8.Value, deadline time.Time) (*v8.Value, error) {
	if val == nil || !val.IsPromise() {
		return val, nil
	}

	// Use JS Promise.then() to capture the resolved/rejected value into globals.
	if err := ctx.Global().Set("__await_input", val); err != nil {
		return nil, fmt.Errorf("setting await input: %w", err)
	}

	_, err := ctx.RunScript(`
		delete globalThis.__awaited_result;
		delete globalThis.__awaited_state;
		Promise.resolve(globalThis.__await_input).then(
			r => { globalThis.__awaited_result = r; globalThis.__awaited_state = 'fulfilled'; },
			e => { globalThis.__awaited_result = e; globalThis.__awaited_state = 'rejected'; }
		);
		delete globalThis.__await_input;
	`, "await.js")
	if err != nil {
		return nil, fmt.Errorf("setting up promise await: %w", err)
	}

	// Pump microtasks until the promise settles.
	for {
		ctx.PerformMicrotaskCheckpoint()

		stateVal, err := ctx.Global().Get("__awaited_state")
		if err != nil {
			return nil, fmt.Errorf("checking promise state: %w", err)
		}
		if !stateVal.IsUndefined() {
			break
		}

		if time.Now().After(deadline) {
			return nil, fmt.Errorf("promise resolution timed out")
		}
		runtime.Gosched()
	}

	stateVal, _ := ctx.Global().Get("__awaited_state")
	resultVal, _ := ctx.Global().Get("__awaited_result")

	// Clean up globals.
	ctx.RunScript("delete globalThis.__awaited_result; delete globalThis.__awaited_state;", "cleanup.js")

	if stateVal.String() == "rejected" {
		return nil, fmt.Errorf("promise rejected: %s", resultVal.String())
	}

	return resultVal, nil
}

// newJSObject creates a new empty JavaScript object.
func newJSObject(iso *v8.Isolate, ctx *v8.Context) (*v8.Object, error) {
	tmpl := v8.NewObjectTemplate(iso)
	return tmpl.NewInstance(ctx)
}
