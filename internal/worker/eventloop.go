package worker

import (
	"sync"
	"time"

	v8 "github.com/tommie/v8go"
)

// timerEntry represents a pending setTimeout or setInterval callback.
type timerEntry struct {
	callback *v8.Function
	deadline time.Time
	interval time.Duration // 0 for setTimeout, >0 for setInterval
	id       int
	cleared  bool
}

// eventLoop manages Go-backed timers for setTimeout/setInterval.
// Provides real wall-clock delays backed by Go timers.
type eventLoop struct {
	mu     sync.Mutex
	timers map[int]*timerEntry
	nextID int
}

func newEventLoop() *eventLoop {
	return &eventLoop{
		timers: make(map[int]*timerEntry),
	}
}

// setTimeout registers a one-shot timer and returns its ID.
func (el *eventLoop) setTimeout(callback *v8.Function, delay time.Duration) int {
	el.mu.Lock()
	defer el.mu.Unlock()
	el.nextID++
	id := el.nextID
	el.timers[id] = &timerEntry{
		callback: callback,
		deadline: time.Now().Add(delay),
		id:       id,
	}
	return id
}

// setInterval registers a repeating timer and returns its ID.
func (el *eventLoop) setInterval(callback *v8.Function, interval time.Duration) int {
	el.mu.Lock()
	defer el.mu.Unlock()
	el.nextID++
	id := el.nextID
	el.timers[id] = &timerEntry{
		callback: callback,
		deadline: time.Now().Add(interval),
		interval: interval,
		id:       id,
	}
	return id
}

// clearTimer cancels a timer by ID.
func (el *eventLoop) clearTimer(id int) {
	el.mu.Lock()
	defer el.mu.Unlock()
	if t, ok := el.timers[id]; ok {
		t.cleared = true
		delete(el.timers, id)
	}
}

// drain fires all pending timers until none remain or the deadline is reached.
// Must be called on the isolate's goroutine (V8 is single-threaded per isolate).
func (el *eventLoop) drain(iso *v8.Isolate, ctx *v8.Context, deadline time.Time) {
	for {
		el.mu.Lock()
		if len(el.timers) == 0 {
			el.mu.Unlock()
			return
		}

		// Find the next timer to fire.
		var next *timerEntry
		for _, t := range el.timers {
			if t.cleared {
				continue
			}
			if next == nil || t.deadline.Before(next.deadline) {
				next = t
			}
		}
		el.mu.Unlock()

		if next == nil {
			return
		}

		// Wait until timer fires or execution deadline.
		now := time.Now()
		if next.deadline.After(now) {
			wait := next.deadline.Sub(now)
			if now.Add(wait).After(deadline) {
				return // Would exceed execution timeout.
			}
			time.Sleep(wait)
		}

		if time.Now().After(deadline) {
			return
		}

		// Fire the callback.
		el.mu.Lock()
		if next.cleared {
			el.mu.Unlock()
			continue
		}
		if next.interval > 0 {
			next.deadline = time.Now().Add(next.interval)
		} else {
			delete(el.timers, next.id)
		}
		cb := next.callback
		el.mu.Unlock()

		// Call on the isolate's goroutine. Ignore errors from timer callbacks.
		undefinedVal := v8.Undefined(iso)
		_, _ = cb.Call(undefinedVal, undefinedVal)
		ctx.PerformMicrotaskCheckpoint()
	}
}

// hasPending returns true if there are any active timers.
func (el *eventLoop) hasPending() bool {
	el.mu.Lock()
	defer el.mu.Unlock()
	return len(el.timers) > 0
}

// reset clears all timers. Called when a worker is returned to the pool.
func (el *eventLoop) reset() {
	el.mu.Lock()
	defer el.mu.Unlock()
	el.timers = make(map[int]*timerEntry)
	el.nextID = 0
}
