package worker

import (
	"fmt"

	"github.com/fastschema/qjs"
)

// timersJS implements setTimeout/setInterval/clearTimeout/clearInterval.
// In the Workers model, timers only fire during microtask processing (Promise
// chains). setTimeout(fn, 0) schedules fn on the next microtask tick.
// Longer delays are approximate and constrained by the execution timeout.
//
// Implementation: We use Promise-based scheduling since QuickJS processes
// Promise microtasks synchronously. setTimeout(fn, 0) is equivalent to
// queueMicrotask(fn). For non-zero delays, we still use microtask scheduling
// since we cannot use real timers in the single-threaded WASM environment.
const timersJS = `
(function() {
	let __timerID = 0;
	const __timers = new Map();

	function __scheduleCallback(fn, delay) {
		if (delay <= 0) {
			Promise.resolve().then(fn);
		} else {
			// Approximate delay using a chain of microtasks.
			// Each microtask takes ~0ms, so for non-zero delays we just
			// schedule on the next microtask. True wall-clock delay is not
			// achievable in synchronous WASM, but this matches the Workers
			// contract: timers fire when the JS thread yields.
			Promise.resolve().then(fn);
		}
	}

	globalThis.setTimeout = function(fn, delay) {
		if (typeof fn !== 'function') return 0;
		const id = ++__timerID;
		let cancelled = false;
		__timers.set(id, { cancel: function() { cancelled = true; } });
		__scheduleCallback(function() {
			if (!cancelled) {
				__timers.delete(id);
				fn();
			}
		}, delay || 0);
		return id;
	};

	globalThis.clearTimeout = function(id) {
		const timer = __timers.get(id);
		if (timer) {
			timer.cancel();
			__timers.delete(id);
		}
	};

	globalThis.setInterval = function(fn, delay) {
		if (typeof fn !== 'function') return 0;
		const id = ++__timerID;
		let cancelled = false;
		__timers.set(id, { cancel: function() { cancelled = true; } });
		function tick() {
			if (cancelled) return;
			fn();
			if (!cancelled) {
				__scheduleCallback(tick, delay || 0);
			}
		}
		__scheduleCallback(tick, delay || 0);
		return id;
	};

	globalThis.clearInterval = function(id) {
		const timer = __timers.get(id);
		if (timer) {
			timer.cancel();
			__timers.delete(id);
		}
	};
})();
`

// setupTimers evaluates the timer polyfills.
func setupTimers(rt *qjs.Runtime) error {
	if _, err := rt.Eval("timers.js", qjs.Code(timersJS)); err != nil {
		return fmt.Errorf("evaluating timers.js: %w", err)
	}
	return nil
}
