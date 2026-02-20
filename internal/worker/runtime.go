package worker

import (
	"context"
	"hash"
	"io"
	"net"
	"net/http"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/coder/websocket"
)

const maxLogEntries = 1000
const maxLogMessageSize = 4096

// cryptoKeyEntry holds imported key material and its associated hash algorithm.
type cryptoKeyEntry struct {
	data        []byte // Raw key bytes (symmetric keys)
	hashAlgo    string // Associated hash algorithm
	algoName    string // Algorithm name (HMAC, AES-GCM, ECDSA, AES-CBC)
	keyType     string // "secret", "public", "private"
	namedCurve  string // For EC keys: "P-256", "P-384"
	ecKey       any    // *ecdsa.PrivateKey or *ecdsa.PublicKey
	extractable bool   // WebCrypto extractable flag — checked in export functions
}

// requestState holds per-request mutable state (logs, fetch counter, env, crypto keys).
// The engine sets it before calling into JS and clears it after.
type requestState struct {
	logs       []LogEntry
	fetchCount int
	maxFetches int
	env        *Env
	cryptoKeys map[int64]*cryptoKeyEntry
	nextKeyID  int64

	// WebSocket bridge state (set when status 101 response is returned).
	wsConn   *websocket.Conn
	wsMu     sync.Mutex
	wsClosed bool

	// DigestStream state: per-request hash instances keyed by stream ID.
	digestStreams map[string]hash.Hash
	nextDigestID  int64

	// EventSource state: per-request SSE connections keyed by source ID.
	eventSources map[string]*eventSourceState
	nextSourceID int64

	// TCP socket state: per-request TCP connections keyed by socket ID.
	tcpSockets       map[string]net.Conn
	tcpSocketBuffers map[string]*tcpSocketBuffer
	nextTCPSocketID  int64

	// Compression stream state: per-request streaming compressors/decompressors.
	compressStreams map[string]*compressStreamState
	nextCompressID  int64

	// In-flight fetch cancellation: maps fetchID -> cancel function.
	fetchCancels map[string]context.CancelFunc
	nextFetchID  int64

	// D1 database bridges: tracked for cleanup on request completion.
	d1Bridges []*D1Bridge
}

// eventSourceState holds state for a single EventSource SSE connection.
type eventSourceState struct {
	url        string
	events     []sseEvent
	mu         sync.Mutex
	closed     bool
	connected  bool
	resp       *http.Response
	body       io.ReadCloser
	cancelFunc func()
}

var (
	requestCounter atomic.Uint64
	requestStates  sync.Map // uint64 -> *requestState
)

// newRequestState creates a new request state and returns its unique ID.
func newRequestState(maxFetches int, env *Env) uint64 {
	id := requestCounter.Add(1)
	requestStates.Store(id, &requestState{
		maxFetches: maxFetches,
		env:        env,
	})
	return id
}

// getRequestState returns the state for the given request ID, or nil.
func getRequestState(id uint64) *requestState {
	v, ok := requestStates.Load(id)
	if !ok {
		return nil
	}
	return v.(*requestState)
}

// clearRequestState removes the state for the given request ID and returns it.
// It cleans up all per-request resources: event sources, TCP sockets,
// compression streams, in-flight fetches, and D1 database bridges.
func clearRequestState(id uint64) *requestState {
	v, ok := requestStates.LoadAndDelete(id)
	if !ok {
		return nil
	}
	state := v.(*requestState)

	// Clean up EventSource SSE connections.
	for _, es := range state.eventSources {
		closeEventSource(es)
	}
	state.eventSources = nil

	// Clean up TCP sockets (existing).
	cleanupTCPSockets(state)

	// Clean up compression streams.
	for _, cs := range state.compressStreams {
		if cs.writer != nil {
			_ = cs.writer.Close()
		}
		if cs.decompPW != nil {
			_ = cs.decompPW.Close()
		}
	}
	state.compressStreams = nil

	// Cancel in-flight fetches.
	for _, cancel := range state.fetchCancels {
		cancel()
	}
	state.fetchCancels = nil

	// Close D1 database bridges.
	for _, b := range state.d1Bridges {
		_ = b.Close()
	}
	state.d1Bridges = nil

	return state
}

// importCryptoKey stores key material scoped to the request and returns its ID.
func importCryptoKey(reqID uint64, hashAlgo string, data []byte) int64 {
	state := getRequestState(reqID)
	if state == nil {
		return -1
	}
	state.nextKeyID++
	id := state.nextKeyID
	if state.cryptoKeys == nil {
		state.cryptoKeys = make(map[int64]*cryptoKeyEntry)
	}
	state.cryptoKeys[id] = &cryptoKeyEntry{data: data, hashAlgo: hashAlgo, extractable: true}
	return id
}

// getCryptoKey retrieves key material scoped to the request.
func getCryptoKey(reqID uint64, keyID int64) *cryptoKeyEntry {
	state := getRequestState(reqID)
	if state == nil {
		return nil
	}
	if state.cryptoKeys == nil {
		return nil
	}
	return state.cryptoKeys[keyID]
}

// addLog appends a log entry to the request state identified by id.
func addLog(id uint64, level, message string) {
	state := getRequestState(id)
	if state == nil {
		return
	}
	if len(state.logs) >= maxLogEntries {
		return
	}
	if len(message) > maxLogMessageSize {
		message = message[:maxLogMessageSize] + "...(truncated)"
	}
	state.logs = append(state.logs, LogEntry{
		Level:   level,
		Message: message,
		Time:    time.Now(),
	})
}

// registerFetchCancel stores a cancel function for an in-flight fetch and
// returns the unique fetchID string key. The cancel is keyed by reqID+fetchID.
func registerFetchCancel(reqID uint64, cancel context.CancelFunc) string {
	state := getRequestState(reqID)
	if state == nil {
		return ""
	}
	state.nextFetchID++
	id := strconv.FormatInt(state.nextFetchID, 10)
	if state.fetchCancels == nil {
		state.fetchCancels = make(map[string]context.CancelFunc)
	}
	state.fetchCancels[id] = cancel
	return id
}

// removeFetchCancel removes and returns the cancel function for a fetch.
func removeFetchCancel(reqID uint64, fetchID string) context.CancelFunc {
	state := getRequestState(reqID)
	if state == nil || state.fetchCancels == nil {
		return nil
	}
	cancel := state.fetchCancels[fetchID]
	delete(state.fetchCancels, fetchID)
	return cancel
}

// callFetchCancel calls the cancel function for the given fetch, if present.
func callFetchCancel(reqID uint64, fetchID string) {
	if cancel := removeFetchCancel(reqID, fetchID); cancel != nil {
		cancel()
	}
}
