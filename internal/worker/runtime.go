package worker

import (
	"sync"
	"sync/atomic"
	"time"
)

// cryptoKeyEntry holds imported key material and its associated hash algorithm.
type cryptoKeyEntry struct {
	data     []byte
	hashAlgo string
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
func clearRequestState(id uint64) *requestState {
	v, ok := requestStates.LoadAndDelete(id)
	if !ok {
		return nil
	}
	return v.(*requestState)
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
	state.cryptoKeys[id] = &cryptoKeyEntry{data: data, hashAlgo: hashAlgo}
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
	state.logs = append(state.logs, LogEntry{
		Level:   level,
		Message: message,
		Time:    time.Now(),
	})
}
