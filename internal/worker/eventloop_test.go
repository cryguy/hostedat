package worker

import (
	"testing"
	"time"

	v8 "github.com/tommie/v8go"
)

func TestEventLoop_NewEventLoop(t *testing.T) {
	el := newEventLoop()
	if el == nil {
		t.Fatal("newEventLoop returned nil")
	}
	if el.timers == nil {
		t.Error("timers map should be initialized")
	}
	if el.nextID != 0 {
		t.Errorf("nextID = %d, want 0", el.nextID)
	}
	if el.hasPending() {
		t.Error("new event loop should have no pending timers")
	}
}

func TestEventLoop_SetTimeout(t *testing.T) {
	iso := v8.NewIsolate()
	defer iso.Dispose()
	ctx := v8.NewContext(iso)
	defer ctx.Close()

	el := newEventLoop()

	// Create a dummy callback function
	fn, err := ctx.RunScript("(function() {})", "cb.js")
	if err != nil {
		t.Fatalf("creating callback: %v", err)
	}
	callback, err := fn.AsFunction()
	if err != nil {
		t.Fatalf("as function: %v", err)
	}

	id1 := el.setTimeout(callback, 100*time.Millisecond)
	if id1 != 1 {
		t.Errorf("first timer ID = %d, want 1", id1)
	}
	if !el.hasPending() {
		t.Error("should have pending timers after setTimeout")
	}

	id2 := el.setTimeout(callback, 200*time.Millisecond)
	if id2 != 2 {
		t.Errorf("second timer ID = %d, want 2", id2)
	}
}

func TestEventLoop_SetInterval(t *testing.T) {
	iso := v8.NewIsolate()
	defer iso.Dispose()
	ctx := v8.NewContext(iso)
	defer ctx.Close()

	el := newEventLoop()

	fn, err := ctx.RunScript("(function() {})", "cb.js")
	if err != nil {
		t.Fatalf("creating callback: %v", err)
	}
	callback, _ := fn.AsFunction()

	id := el.setInterval(callback, 50*time.Millisecond)
	if id != 1 {
		t.Errorf("interval ID = %d, want 1", id)
	}

	el.mu.Lock()
	entry := el.timers[id]
	el.mu.Unlock()

	if entry == nil {
		t.Fatal("timer entry not found")
	}
	if entry.interval != 50*time.Millisecond {
		t.Errorf("interval = %v, want 50ms", entry.interval)
	}
}

func TestEventLoop_ClearTimer(t *testing.T) {
	iso := v8.NewIsolate()
	defer iso.Dispose()
	ctx := v8.NewContext(iso)
	defer ctx.Close()

	el := newEventLoop()

	fn, _ := ctx.RunScript("(function() {})", "cb.js")
	callback, _ := fn.AsFunction()

	id := el.setTimeout(callback, 100*time.Millisecond)
	if !el.hasPending() {
		t.Error("should have pending timer")
	}

	el.clearTimer(id)
	if el.hasPending() {
		t.Error("should not have pending timers after clear")
	}
}

func TestEventLoop_ClearTimer_NonExistent(t *testing.T) {
	el := newEventLoop()
	// Should not panic when clearing a non-existent timer
	el.clearTimer(999)
	if el.hasPending() {
		t.Error("should not have pending timers")
	}
}

func TestEventLoop_Reset(t *testing.T) {
	iso := v8.NewIsolate()
	defer iso.Dispose()
	ctx := v8.NewContext(iso)
	defer ctx.Close()

	el := newEventLoop()

	fn, _ := ctx.RunScript("(function() {})", "cb.js")
	callback, _ := fn.AsFunction()

	el.setTimeout(callback, 100*time.Millisecond)
	el.setInterval(callback, 200*time.Millisecond)
	el.setTimeout(callback, 300*time.Millisecond)

	if !el.hasPending() {
		t.Error("should have pending timers")
	}

	el.reset()

	if el.hasPending() {
		t.Error("should not have pending timers after reset")
	}
	if el.nextID != 0 {
		t.Errorf("nextID after reset = %d, want 0", el.nextID)
	}
}

func TestEventLoop_Drain_Empty(t *testing.T) {
	iso := v8.NewIsolate()
	defer iso.Dispose()
	ctx := v8.NewContext(iso)
	defer ctx.Close()

	el := newEventLoop()
	// Drain on empty event loop should return immediately.
	el.drain(iso, ctx, time.Now().Add(time.Second))
}

func TestEventLoop_Drain_Timeout(t *testing.T) {
	iso := v8.NewIsolate()
	defer iso.Dispose()
	ctx := v8.NewContext(iso)
	defer ctx.Close()

	el := newEventLoop()

	fn, _ := ctx.RunScript("(function() {})", "cb.js")
	callback, _ := fn.AsFunction()

	// Set a timer far in the future
	el.setTimeout(callback, 10*time.Second)

	// Drain with a deadline in the past
	el.drain(iso, ctx, time.Now().Add(-1*time.Second))

	// Timer should still be pending (not fired because deadline passed)
	if !el.hasPending() {
		t.Error("timer should still be pending after expired deadline")
	}
}

func TestEventLoop_Drain_FiresCallback(t *testing.T) {
	iso := v8.NewIsolate()
	defer iso.Dispose()
	ctx := v8.NewContext(iso)
	defer ctx.Close()

	el := newEventLoop()

	// Set a global that the callback will modify
	_, _ = ctx.RunScript("globalThis.__timer_fired = false", "init.js")

	fn, _ := ctx.RunScript("(function() { globalThis.__timer_fired = true; })", "cb.js")
	callback, _ := fn.AsFunction()

	el.setTimeout(callback, 1*time.Millisecond)

	// Drain with sufficient deadline
	el.drain(iso, ctx, time.Now().Add(5*time.Second))

	result, _ := ctx.RunScript("globalThis.__timer_fired", "check.js")
	if !result.Boolean() {
		t.Error("timer callback should have fired")
	}
}

func TestEventLoop_Drain_IntervalRepeats(t *testing.T) {
	iso := v8.NewIsolate()
	defer iso.Dispose()
	ctx := v8.NewContext(iso)
	defer ctx.Close()

	el := newEventLoop()

	_, _ = ctx.RunScript("globalThis.__count = 0", "init.js")
	fn, _ := ctx.RunScript("(function() { globalThis.__count++; if (globalThis.__count >= 3) { /* stop */ } })", "cb.js")
	callback, _ := fn.AsFunction()

	el.setInterval(callback, 1*time.Millisecond)

	// Drain for a short period - interval should fire multiple times
	el.drain(iso, ctx, time.Now().Add(100*time.Millisecond))

	result, _ := ctx.RunScript("globalThis.__count", "check.js")
	count := result.Int32()
	if count < 2 {
		t.Errorf("interval should have fired at least twice, got %d", count)
	}
}

func TestEventLoop_ClearTimer_DuringDrain(t *testing.T) {
	iso := v8.NewIsolate()
	defer iso.Dispose()
	ctx := v8.NewContext(iso)
	defer ctx.Close()

	el := newEventLoop()

	fn, _ := ctx.RunScript("(function() {})", "cb.js")
	callback, _ := fn.AsFunction()

	id := el.setInterval(callback, 1*time.Millisecond)

	// Clear the timer from another goroutine during drain
	go func() {
		time.Sleep(10 * time.Millisecond)
		el.clearTimer(id)
	}()

	el.drain(iso, ctx, time.Now().Add(200*time.Millisecond))
	// Should complete without hanging
}

func TestEventLoop_IDsIncrement(t *testing.T) {
	iso := v8.NewIsolate()
	defer iso.Dispose()
	ctx := v8.NewContext(iso)
	defer ctx.Close()

	el := newEventLoop()

	fn, _ := ctx.RunScript("(function() {})", "cb.js")
	callback, _ := fn.AsFunction()

	id1 := el.setTimeout(callback, time.Second)
	id2 := el.setTimeout(callback, time.Second)
	id3 := el.setInterval(callback, time.Second)

	if id1 != 1 || id2 != 2 || id3 != 3 {
		t.Errorf("IDs should increment: got %d, %d, %d", id1, id2, id3)
	}
}
