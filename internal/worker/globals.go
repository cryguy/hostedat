package worker

import (
	"fmt"
	"time"

	"github.com/fastschema/qjs"
)

// globalsJS defines pure-JS polyfills for simple global APIs.
const globalsJS = `
globalThis.structuredClone = function(value) {
	return JSON.parse(JSON.stringify(value));
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
func setupGlobals(rt *qjs.Runtime) error {
	ctx := rt.Context()

	// Evaluate pure-JS polyfills first.
	if _, err := rt.Eval("globals.js", qjs.Code(globalsJS)); err != nil {
		return fmt.Errorf("evaluating globals.js: %w", err)
	}

	// performance.now() — Go-backed for high-resolution timing.
	startTime := time.Now()
	perf := ctx.NewObject()
	nowFn := ctx.Function(func(this *qjs.This) (*qjs.Value, error) {
		c := this.Context()
		elapsed := time.Since(startTime)
		ms := float64(elapsed.Nanoseconds()) / 1e6
		return c.NewFloat64(ms), nil
	}, false)
	perf.SetPropertyStr("now", nowFn)
	ctx.Global().SetPropertyStr("performance", perf)

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
