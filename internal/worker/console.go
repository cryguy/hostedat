package worker

import (
	"fmt"
	"strconv"
	"strings"

	v8 "github.com/tommie/v8go"
)

// setupConsole replaces globalThis.console with a Go-backed version
// that captures output into the per-request log buffer.
func setupConsole(iso *v8.Isolate, ctx *v8.Context, _ *eventLoop) error {
	console, err := newJSObject(iso, ctx)
	if err != nil {
		return fmt.Errorf("creating console object: %w", err)
	}

	levels := []string{"log", "info", "warn", "error", "debug"}
	for _, level := range levels {
		lvl := level // capture for closure
		ft := v8.NewFunctionTemplate(iso, func(info *v8.FunctionCallbackInfo) *v8.Value {
			reqIDVal, _ := ctx.Global().Get("__requestID")
			var reqID uint64
			if reqIDVal != nil && !reqIDVal.IsUndefined() && !reqIDVal.IsNull() {
				reqID, _ = strconv.ParseUint(reqIDVal.String(), 10, 64)
			}

			args := info.Args()
			parts := make([]string, 0, len(args))
			for _, arg := range args {
				parts = append(parts, arg.String())
			}
			msg := strings.Join(parts, " ")
			addLog(reqID, lvl, msg)
			return v8.Undefined(iso)
		})
		_ = console.Set(lvl, ft.GetFunction(ctx))
	}

	_ = ctx.Global().Set("console", console)
	return nil
}

// consoleExtJS adds extended console methods (time, count, assert, table, etc.)
// as pure JS on top of the Go-backed base console.
const consoleExtJS = `
(function() {
var __timers = {};
var __counters = {};
var __groupDepth = 0;

console.time = function(label) {
	__timers[label || 'default'] = performance.now();
};
console.timeEnd = function(label) {
	var l = label || 'default';
	var start = __timers[l];
	if (start === undefined) { console.warn('Timer "' + l + '" does not exist'); return; }
	var elapsed = performance.now() - start;
	delete __timers[l];
	console.log(l + ': ' + elapsed.toFixed(3) + 'ms');
};
console.timeLog = function(label) {
	var l = label || 'default';
	var start = __timers[l];
	if (start === undefined) { console.warn('Timer "' + l + '" does not exist'); return; }
	var elapsed = performance.now() - start;
	var args = Array.prototype.slice.call(arguments, 1);
	if (args.length > 0) {
		console.log(l + ': ' + elapsed.toFixed(3) + 'ms', args.join(' '));
	} else {
		console.log(l + ': ' + elapsed.toFixed(3) + 'ms');
	}
};
console.count = function(label) {
	var l = label || 'default';
	__counters[l] = (__counters[l] || 0) + 1;
	console.log(l + ': ' + __counters[l]);
};
console.countReset = function(label) {
	__counters[label || 'default'] = 0;
};
console.assert = function(cond) {
	if (!cond) {
		var args = Array.prototype.slice.call(arguments, 1);
		if (args.length > 0) {
			console.error('Assertion failed:', args.join(' '));
		} else {
			console.error('Assertion failed');
		}
	}
};
console.table = function(data) {
	console.log(JSON.stringify(data, null, 2));
};
console.trace = function() {
	var args = Array.prototype.slice.call(arguments);
	if (args.length > 0) {
		console.log('Trace:', args.join(' '));
	} else {
		console.log('Trace');
	}
};
console.group = function(label) {
	if (label) console.log(label);
	__groupDepth++;
};
console.groupEnd = function() {
	if (__groupDepth > 0) __groupDepth--;
};
console.dir = function(obj) {
	console.log(JSON.stringify(obj, null, 2));
};
})();
`

// setupConsoleExt evaluates the extended console methods polyfill.
// Must be called AFTER setupConsole so the base console object exists.
func setupConsoleExt(_ *v8.Isolate, ctx *v8.Context, _ *eventLoop) error {
	if _, err := ctx.RunScript(consoleExtJS, "console_ext.js"); err != nil {
		return fmt.Errorf("evaluating console_ext.js: %w", err)
	}
	return nil
}
