package worker

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/cryguy/hostedat/internal/config"
	"github.com/cryguy/hostedat/internal/models"
	"github.com/cryguy/hostedat/internal/storage"
	"github.com/fastschema/qjs"
	"gorm.io/gorm"
)

// poolKey uniquely identifies a compiled worker version for a site.
type poolKey struct {
	SiteID  string
	Version int
}

// sitePool wraps a qjs.Pool with an invalidation flag so that stale pools
// are replaced transparently on the next Execute call.
type sitePool struct {
	pool    *qjs.Pool
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

// Engine manages per-site worker pools and executes JS worker scripts.
type Engine struct {
	pools     sync.Map // poolKey -> *sitePool
	bytecodes sync.Map // poolKey -> []byte
	config    config.WorkerConfig
	db        *gorm.DB
	store     *storage.Manager
	logDone   chan struct{}
}

// NewEngine creates an Engine with the given configuration and database handle.
// It starts a background goroutine for log retention cleanup.
func NewEngine(cfg config.WorkerConfig, db *gorm.DB) *Engine {
	e := &Engine{
		config:  cfg,
		db:      db,
		logDone: make(chan struct{}),
	}
	go e.logRetentionLoop()
	return e
}

// SetStore sets the storage manager for bytecode reload on server restart.
func (e *Engine) SetStore(store *storage.Manager) {
	e.store = store
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

// EnsureBytecode loads bytecode from disk if not already in memory.
// This handles the server restart scenario where pools and bytecodes are lost
// but the compiled bytecode.bin files remain on disk.
func (e *Engine) EnsureBytecode(siteID string, version int) error {
	key := poolKey{SiteID: siteID, Version: version}
	if _, ok := e.bytecodes.Load(key); ok {
		return nil
	}

	if e.store == nil {
		return fmt.Errorf("storage manager not set")
	}

	// Try reading cached bytecode from disk.
	bcPath := filepath.Join(e.store.GetWorkerBytecodeDir(siteID, version), "bytecode.bin")
	bytecode, err := os.ReadFile(bcPath)
	if err == nil && len(bytecode) > 0 {
		e.bytecodes.Store(key, bytecode)
		return nil
	}

	// Fallback: recompile from source.
	source, err := e.store.GetWorkerScript(siteID, version)
	if err != nil {
		return fmt.Errorf("no bytecode or source for site %s version %d: %w", siteID, version, err)
	}

	if _, err := e.CompileAndCache(siteID, version, source); err != nil {
		return fmt.Errorf("recompiling worker: %w", err)
	}

	return nil
}

// CompileAndCache compiles a worker script into QuickJS bytecode and stores
// it for later pool creation. The source must be a valid ES module that
// exports a default object with a fetch() handler.
func (e *Engine) CompileAndCache(siteID string, version int, source string) ([]byte, error) {
	key := poolKey{SiteID: siteID, Version: version}

	rt, err := qjs.New(qjs.Option{
		Context:            context.Background(),
		CloseOnContextDone: true, // Must match pool option to initialize global Wazero config correctly.
		MemoryLimit:        e.config.MemoryLimitMB * 1024 * 1024,
		MaxStackSize:       1024 * 1024,
		MaxExecutionTime:   e.config.ExecutionTimeout,
		GCThreshold:        256 * 1024,
	})
	if err != nil {
		return nil, fmt.Errorf("creating compile runtime: %w", err)
	}
	defer rt.Close()

	bytecode, err := rt.Compile("worker.js", qjs.Code(source), qjs.TypeModule())
	if err != nil {
		return nil, fmt.Errorf("compiling worker script: %w", err)
	}

	e.bytecodes.Store(key, bytecode)
	return bytecode, nil
}

// GetOrCreatePool returns the runtime pool for the given site/version,
// creating it if necessary. Each runtime in the pool has the Web APIs,
// console, and fetch injected, and the compiled worker bytecode evaluated.
func (e *Engine) GetOrCreatePool(siteID string, version int, env *Env) (*qjs.Pool, error) {
	key := poolKey{SiteID: siteID, Version: version}

	// Check for a valid existing pool.
	if val, ok := e.pools.Load(key); ok {
		sp := val.(*sitePool)
		if sp.isValid() {
			return sp.pool, nil
		}
		// Stale pool -- remove and create a new one.
		e.pools.Delete(key)
	}

	// Load bytecode.
	bcVal, ok := e.bytecodes.Load(key)
	if !ok {
		return nil, fmt.Errorf("no compiled bytecode for site %s version %d", siteID, version)
	}
	bytecode := bcVal.([]byte)

	cfg := e.config
	option := qjs.Option{
		Context:            context.Background(),
		CloseOnContextDone: true, // Required for rt.Close() to interrupt running WASM execution.
		MemoryLimit:        cfg.MemoryLimitMB * 1024 * 1024,
		MaxStackSize:       1024 * 1024,
		MaxExecutionTime:   cfg.ExecutionTimeout,
		GCThreshold:        256 * 1024,
	}

	pool := qjs.NewPool(cfg.PoolSize, option,
		// Setup function 1: Web APIs (Headers, Request, Response, URL, etc.)
		setupWebAPIs,
		// Setup function 2: globals (structuredClone, performance, navigator, queueMicrotask)
		setupGlobals,
		// Setup function 3: encoding (atob, btoa)
		setupEncoding,
		// Setup function 4: timers (setTimeout, setInterval, clearTimeout, clearInterval)
		setupTimers,
		// Setup function 5: abort (AbortController, AbortSignal, Event, EventTarget, DOMException)
		setupAbort,
		// Setup function 6: crypto (crypto.getRandomValues, crypto.subtle, crypto.randomUUID)
		setupCrypto,
		// Setup function 7: streams (ReadableStream, WritableStream, TransformStream)
		setupStreams,
		// Setup function 8: formdata (FormData, Blob, File)
		setupFormData,
		// Setup function 9: console capture
		setupConsole,
		// Setup function 10: fetch()
		func(rt *qjs.Runtime) error {
			return setupFetch(rt, cfg)
		},
		// Setup function 11: load worker module and extract default export
		func(rt *qjs.Runtime) error {
			// Load registers the compiled module in the runtime.
			if _, err := rt.Load("worker.js", qjs.Bytecode(bytecode)); err != nil {
				return fmt.Errorf("loading worker bytecode: %w", err)
			}
			// Import the default export from the registered module.
			moduleVal, err := rt.Eval("__worker_import__.js",
				qjs.Code(`import mod from 'worker.js'; export default mod;`),
				qjs.TypeModule(),
			)
			if err != nil {
				return fmt.Errorf("importing worker module: %w", err)
			}
			rt.Context().Global().SetPropertyStr("__worker_module__", moduleVal)
			return nil
		},
	)

	sp := &sitePool{pool: pool}
	e.pools.Store(key, sp)
	return pool, nil
}

// Execute runs the worker's fetch handler for the given request and returns
// the result including the response, captured logs, and any error.
func (e *Engine) Execute(siteID string, version int, env *Env, req *WorkerRequest) (result *WorkerResult) {
	start := time.Now()
	result = &WorkerResult{}

	// Ensure bytecode is loaded (handles server restart).
	if err := e.EnsureBytecode(siteID, version); err != nil {
		result.Error = err
		result.Duration = time.Since(start)
		return result
	}

	pool, err := e.GetOrCreatePool(siteID, version, env)
	if err != nil {
		result.Error = err
		result.Duration = time.Since(start)
		return result
	}

	rt, err := pool.Get()
	if err != nil {
		result.Error = fmt.Errorf("acquiring runtime from pool: %w", err)
		result.Duration = time.Since(start)
		return result
	}

	// Watchdog: cancel the runtime's context if execution exceeds timeout.
	// Wazero's CloseOnContextDone inserts periodic checks during WASM execution;
	// when the context is cancelled, the running function call is interrupted.
	var timedOut atomic.Bool
	timeout := time.Duration(e.config.ExecutionTimeout) * time.Millisecond
	reqCtx, cancelReq := context.WithCancel(context.Background())
	origCtx := rt.Context().Context
	rt.Context().Context = reqCtx

	watchdog := time.AfterFunc(timeout, func() {
		timedOut.Store(true)
		cancelReq()
	})

	defer func() {
		stopped := watchdog.Stop()
		if r := recover(); r != nil {
			if timedOut.Load() {
				result.Error = fmt.Errorf("worker execution timed out (limit: %v)", timeout)
			} else {
				result.Error = fmt.Errorf("worker panic: %v", r)
			}
		}
		result.Duration = time.Since(start)
		// Only return healthy runtimes to the pool. If the watchdog fired
		// (stopped==false), the context is cancelled and the runtime may be
		// in a broken state — discard it.
		if stopped && !timedOut.Load() {
			rt.Context().Context = origCtx
			cancelReq() // Release context resources.
			pool.Put(rt)
		}
	}()

	ctx := rt.Context()

	// Set up per-request state.
	reqID := newRequestState(e.config.MaxFetchRequests, env)
	ctx.Global().SetPropertyStr("__requestID", ctx.NewInt64(int64(reqID)))

	// Build the JS arguments: request, env, ctx.
	jsReq, err := goRequestToJS(ctx, req)
	if err != nil {
		clearRequestState(reqID)
		result.Error = fmt.Errorf("building JS request: %w", err)
		return result
	}

	jsEnv := buildEnvObject(ctx, env, e.db)
	jsCtx := buildExecContext(ctx)

	// Call __worker_module__.fetch(request, env, ctx).
	// Note: qjs Eval with TypeModule() returns the default export directly,
	// not a module namespace object, so __worker_module__ IS the default export.
	defaultExport := ctx.Global().GetPropertyStr("__worker_module__")

	if defaultExport.IsUndefined() || defaultExport.IsNull() {
		defaultExport.Free()
		state := clearRequestState(reqID)
		if state != nil {
			result.Logs = state.logs
		}
		result.Error = fmt.Errorf("worker module has no default export")
		return result
	}

	fetchResult, err := defaultExport.InvokeJS("fetch", jsReq, jsEnv, jsCtx)
	defaultExport.Free()

	if err != nil {
		state := clearRequestState(reqID)
		if state != nil {
			result.Logs = state.logs
		}
		result.Error = fmt.Errorf("invoking worker fetch: %w", err)
		return result
	}

	// The fetch handler returns a Promise<Response>.
	if fetchResult.IsPromise() {
		awaited, err := fetchResult.Await()
		// Do NOT Free the Promise here — in the QuickJS WASM build, freeing
		// a resolved Promise can cause out-of-bounds memory access when the
		// resolved value was created by static constructors (e.g. Response.json).
		// The Promise will be garbage-collected by QuickJS on the next GC cycle.
		if err != nil {
			state := clearRequestState(reqID)
			if state != nil {
				result.Logs = state.logs
			}
			result.Error = fmt.Errorf("awaiting worker response: %w", err)
			return result
		}
		fetchResult = awaited
	}

	// Convert JS Response to Go.
	resp, err := jsResponseToGo(ctx, fetchResult)
	// Same caution: only Free non-Promise results. For awaited Promises the
	// resolved value shares internal WASM memory with the Promise and must
	// be left for GC.  Sync results are safe to Free immediately.

	state := clearRequestState(reqID)
	if state != nil {
		result.Logs = state.logs
	}

	if err != nil {
		result.Error = fmt.Errorf("converting worker response: %w", err)
		return result
	}

	result.Response = resp
	return result
}

// ExecuteScheduled runs the worker's scheduled handler for cron triggers.
func (e *Engine) ExecuteScheduled(siteID string, version int, env *Env, cron string) (result *WorkerResult) {
	start := time.Now()
	result = &WorkerResult{}

	pool, err := e.GetOrCreatePool(siteID, version, env)
	if err != nil {
		result.Error = err
		result.Duration = time.Since(start)
		return result
	}

	rt, err := pool.Get()
	if err != nil {
		result.Error = fmt.Errorf("acquiring runtime from pool: %w", err)
		result.Duration = time.Since(start)
		return result
	}

	// Watchdog: cancel the runtime's context if execution exceeds timeout.
	var timedOut atomic.Bool
	timeout := time.Duration(e.config.ExecutionTimeout) * time.Millisecond
	reqCtx, cancelReq := context.WithCancel(context.Background())
	origCtx := rt.Context().Context
	rt.Context().Context = reqCtx

	watchdog := time.AfterFunc(timeout, func() {
		timedOut.Store(true)
		cancelReq()
	})

	defer func() {
		stopped := watchdog.Stop()
		if r := recover(); r != nil {
			if timedOut.Load() {
				result.Error = fmt.Errorf("worker execution timed out (limit: %v)", timeout)
			} else {
				result.Error = fmt.Errorf("worker panic: %v", r)
			}
		}
		result.Duration = time.Since(start)
		if stopped && !timedOut.Load() {
			rt.Context().Context = origCtx
			cancelReq()
			pool.Put(rt)
		}
	}()

	ctx := rt.Context()

	// Set up per-request state.
	reqID := newRequestState(e.config.MaxFetchRequests, env)
	ctx.Global().SetPropertyStr("__requestID", ctx.NewInt64(int64(reqID)))

	// Build the scheduled event.
	event := ctx.NewObject()
	event.SetPropertyStr("scheduledTime", ctx.NewFloat64(float64(time.Now().UnixMilli())))
	event.SetPropertyStr("cron", ctx.NewString(cron))

	jsEnv := buildEnvObject(ctx, env, e.db)
	jsCtx := buildExecContext(ctx)

	// Call __worker_module__.scheduled(event, env, ctx).
	// Note: qjs Eval with TypeModule() returns the default export directly.
	defaultExport := ctx.Global().GetPropertyStr("__worker_module__")

	if defaultExport.IsUndefined() || defaultExport.IsNull() {
		defaultExport.Free()
		state := clearRequestState(reqID)
		if state != nil {
			result.Logs = state.logs
		}
		result.Error = fmt.Errorf("worker module has no default export")
		return result
	}

	schedResult, err := defaultExport.InvokeJS("scheduled", event, jsEnv, jsCtx)
	defaultExport.Free()

	if err != nil {
		state := clearRequestState(reqID)
		if state != nil {
			result.Logs = state.logs
		}
		result.Error = fmt.Errorf("invoking worker scheduled: %w", err)
		return result
	}

	// Await if the handler returns a promise.
	// Do NOT Free Promise/awaited values — see Execute() comment for rationale.
	if schedResult != nil && schedResult.IsPromise() {
		_, err := schedResult.Await()
		if err != nil {
			state := clearRequestState(reqID)
			if state != nil {
				result.Logs = state.logs
			}
			result.Error = fmt.Errorf("awaiting scheduled handler: %w", err)
			return result
		}
	}

	state := clearRequestState(reqID)
	if state != nil {
		result.Logs = state.logs
	}
	return result
}

// InvalidatePool marks the pool for the given site/version as invalid.
// The next Execute call will create a fresh pool.
func (e *Engine) InvalidatePool(siteID string, version int) {
	key := poolKey{SiteID: siteID, Version: version}
	if val, ok := e.pools.LoadAndDelete(key); ok {
		sp := val.(*sitePool)
		sp.markInvalid()
	}
	e.bytecodes.Delete(key)
}

// Shutdown invalidates all pools, clears all cached bytecode, and stops
// the log retention goroutine.
func (e *Engine) Shutdown() {
	close(e.logDone)
	e.pools.Range(func(key, val any) bool {
		sp := val.(*sitePool)
		sp.markInvalid()
		e.pools.Delete(key)
		return true
	})
	e.bytecodes.Range(func(key, _ any) bool {
		e.bytecodes.Delete(key)
		return true
	})
}

// MaxResponseBytes returns the configured maximum response body size.
func (e *Engine) MaxResponseBytes() int {
	return e.config.MaxResponseBytes
}

// BuildEnvFromDB loads environment variables, secrets, and KV bindings for
// a site from the database. Pass an AssetsFetcher to enable env.ASSETS,
// or nil if assets are not available (e.g., cron context).
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

	return env
}
