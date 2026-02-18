package worker

import (
	"fmt"
	"time"

	v8 "github.com/tommie/v8go"
)

// globalsJS defines pure-JS polyfills for simple global APIs.
const globalsJS = `
globalThis.structuredClone = function(value) {
	if (value === undefined) {
		throw new DOMException('value could not be cloned', 'DataCloneError');
	}
	if (typeof value === 'function' || typeof value === 'symbol') {
		throw new DOMException('value could not be cloned', 'DataCloneError');
	}
	if (value !== null && typeof value === 'object') {
		if (typeof Map !== 'undefined' && value instanceof Map) {
			throw new DOMException('Map cannot be cloned', 'DataCloneError');
		}
		if (typeof Set !== 'undefined' && value instanceof Set) {
			throw new DOMException('Set cannot be cloned', 'DataCloneError');
		}
		if (typeof WeakMap !== 'undefined' && value instanceof WeakMap) {
			throw new DOMException('WeakMap cannot be cloned', 'DataCloneError');
		}
		if (typeof WeakSet !== 'undefined' && value instanceof WeakSet) {
			throw new DOMException('WeakSet cannot be cloned', 'DataCloneError');
		}
	}
	try {
		return JSON.parse(JSON.stringify(value));
	} catch(e) {
		throw new DOMException('value could not be cloned: ' + e.message, 'DataCloneError');
	}
};

globalThis.queueMicrotask = function(fn) {
	Promise.resolve().then(fn);
};

Object.defineProperty(globalThis, 'navigator', {
	value: { userAgent: "hostedat-worker/1.0" },
	writable: true,
	configurable: true,
});
`

// setupGlobals registers structuredClone, performance.now(), navigator,
// queueMicrotask, and the Event/EventTarget base classes.
func setupGlobals(iso *v8.Isolate, ctx *v8.Context, el *eventLoop) error {
	// Evaluate pure-JS polyfills first.
	if _, err := ctx.RunScript(globalsJS, "globals.js"); err != nil {
		return fmt.Errorf("evaluating globals.js: %w", err)
	}

	// performance.now()  EGo-backed for high-resolution timing.
	startTime := time.Now()
	perf, err := newJSObject(iso, ctx)
	if err != nil {
		return fmt.Errorf("creating performance object: %w", err)
	}

	ft := v8.NewFunctionTemplate(iso, func(info *v8.FunctionCallbackInfo) *v8.Value {
		elapsed := time.Since(startTime)
		ms := float64(elapsed.Nanoseconds()) / 1e6
		val, _ := v8.NewValue(iso, ms)
		return val
	})
	perf.Set("now", ft.GetFunction(ctx))
	ctx.Global().Set("performance", perf)

	return nil
}

// errMissingArg returns a formatted error for functions called with too few arguments.
func errMissingArg(name string, required int) error {
	return fmt.Errorf("%s requires at least %d argument(s)", name, required)
}

// errInvalidArg returns a formatted error for invalid argument values.
func errInvalidArg(name, reason string) error {
	return fmt.Errorf("%s: %s", name, reason)
}
