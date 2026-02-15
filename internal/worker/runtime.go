package worker

import (
	"sync"
	"sync/atomic"
	"time"
)

// requestState holds per-request mutable state (logs, fetch counter, env).
// The engine sets it before calling into JS and clears it after.
type requestState struct {
	logs       []LogEntry
	fetchCount int
	maxFetches int
	env        *Env
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
