package worker

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	v8 "github.com/tommie/v8go"
)

// globalsJS defines pure-JS polyfills for simple global APIs.
const globalsJS = `
globalThis.structuredClone = (function() {
	var TYPED_ARRAY_CONSTRUCTORS = [
		typeof Uint8Array !== 'undefined' && Uint8Array,
		typeof Int8Array !== 'undefined' && Int8Array,
		typeof Uint8ClampedArray !== 'undefined' && Uint8ClampedArray,
		typeof Int16Array !== 'undefined' && Int16Array,
		typeof Uint16Array !== 'undefined' && Uint16Array,
		typeof Int32Array !== 'undefined' && Int32Array,
		typeof Uint32Array !== 'undefined' && Uint32Array,
		typeof Float32Array !== 'undefined' && Float32Array,
		typeof Float64Array !== 'undefined' && Float64Array,
		typeof BigInt64Array !== 'undefined' && BigInt64Array,
		typeof BigUint64Array !== 'undefined' && BigUint64Array,
	].filter(Boolean);

	function cloneError(msg) {
		return new DOMException(msg, 'DataCloneError');
	}

	function deepClone(value, seen) {
		if (value === undefined) throw cloneError('value could not be cloned');
		if (value === null) return null;

		var type = typeof value;
		if (type === 'boolean' || type === 'number' || type === 'string' || type === 'bigint') return value;
		if (type === 'function') throw cloneError('value could not be cloned');
		if (type === 'symbol') throw cloneError('value could not be cloned');

		// value is an object from here on
		if (typeof WeakMap !== 'undefined' && value instanceof WeakMap) throw cloneError('WeakMap cannot be cloned');
		if (typeof WeakSet !== 'undefined' && value instanceof WeakSet) throw cloneError('WeakSet cannot be cloned');
		if (typeof Promise !== 'undefined' && value instanceof Promise) throw cloneError('Promise cannot be cloned');

		// Circular reference detection
		if (seen.has(value)) throw cloneError('value could not be cloned: circular reference');
		seen.set(value, true);

		// Date
		if (value instanceof Date) return new Date(value.getTime());

		// RegExp
		if (value instanceof RegExp) return new RegExp(value.source, value.flags);

		// ArrayBuffer
		if (value instanceof ArrayBuffer) {
			return value.slice(0);
		}

		// TypedArrays
		for (var ti = 0; ti < TYPED_ARRAY_CONSTRUCTORS.length; ti++) {
			var TA = TYPED_ARRAY_CONSTRUCTORS[ti];
			if (value instanceof TA) {
				var clonedBuf = value.buffer.slice(value.byteOffset, value.byteOffset + value.byteLength);
				return new TA(clonedBuf);
			}
		}

		// DataView
		if (typeof DataView !== 'undefined' && value instanceof DataView) {
			var dvBuf = value.buffer.slice(value.byteOffset, value.byteOffset + value.byteLength);
			return new DataView(dvBuf);
		}

		// Map
		if (typeof Map !== 'undefined' && value instanceof Map) {
			var clonedMap = new Map();
			value.forEach(function(v, k) {
				clonedMap.set(deepClone(k, seen), deepClone(v, seen));
			});
			return clonedMap;
		}

		// Set
		if (typeof Set !== 'undefined' && value instanceof Set) {
			var clonedSet = new Set();
			value.forEach(function(v) {
				clonedSet.add(deepClone(v, seen));
			});
			return clonedSet;
		}

		// Array
		if (Array.isArray(value)) {
			var arr = new Array(value.length);
			for (var i = 0; i < value.length; i++) {
				arr[i] = deepClone(value[i], seen);
			}
			return arr;
		}

		// Plain object
		var result = {};
		var keys = Object.keys(value);
		for (var j = 0; j < keys.length; j++) {
			result[keys[j]] = deepClone(value[keys[j]], seen);
		}
		return result;
	}

	return function structuredClone(value) {
		return deepClone(value, new WeakMap());
	};
})();

globalThis.queueMicrotask = function(fn) {
	Promise.resolve().then(fn);
};

Object.defineProperty(globalThis, 'navigator', {
	value: {
		userAgent: "hostedat-worker/1.0",
		scheduling: { isInputPending: function() { return false; } },
		sendBeacon: function(url, data) {
			var body = '';
			var ct = 'text/plain;charset=UTF-8';
			if (data !== undefined && data !== null) {
				body = String(data);
			}
			try {
				return __sendBeacon(url, body, ct);
			} catch(e) {
				return false;
			}
		},
	},
	writable: true,
	configurable: true,
});
`

// setupGlobals registers structuredClone, performance.now(), navigator,
// queueMicrotask, and the Event/EventTarget base classes.
func setupGlobals(iso *v8.Isolate, ctx *v8.Context, _ *eventLoop) error {
	// __sendBeacon: Go-backed fire-and-forget POST with SSRF protection.
	beaconFT := v8.NewFunctionTemplate(iso, func(info *v8.FunctionCallbackInfo) *v8.Value {
		args := info.Args()
		if len(args) < 3 {
			val, _ := v8.NewValue(iso, false)
			return val
		}
		targetURL := args[0].String()
		body := args[1].String()
		contentType := args[2].String()

		// SSRF pre-check (same as fetch.go).
		if isPrivateHostname(targetURL) {
			val, _ := v8.NewValue(iso, false)
			return val
		}

		// Fire-and-forget in a goroutine.
		go func() {
			httpReq, err := http.NewRequest("POST", targetURL, strings.NewReader(body))
			if err != nil {
				return
			}
			httpReq.Header.Set("Content-Type", contentType)
			client := &http.Client{
				Timeout: 10 * time.Second,
				Transport: &http.Transport{
					DialContext: ssrfSafeDialContext,
				},
			}
			resp, err := client.Do(httpReq)
			if err != nil {
				return
			}
			_ = resp.Body.Close()
		}()

		val, _ := v8.NewValue(iso, true)
		return val
	})
	_ = ctx.Global().Set("__sendBeacon", beaconFT.GetFunction(ctx))

	// Evaluate pure-JS polyfills.
	if _, err := ctx.RunScript(globalsJS, "globals.js"); err != nil {
		return fmt.Errorf("evaluating globals.js: %w", err)
	}

	// performance.now() - Go-backed for high-resolution timing.
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
	_ = perf.Set("now", ft.GetFunction(ctx))
	_ = ctx.Global().Set("performance", perf)

	return nil
}

// reportErrorJS defines ErrorEvent and reportError.
// Must be evaluated AFTER setupAbort (which defines Event and EventTarget on globalThis).
const reportErrorJS = `
class ErrorEvent extends Event {
	constructor(type, init) {
		super(type);
		this.error = init && init.error !== undefined ? init.error : null;
		this.message = (init && init.message) || '';
		this.filename = (init && init.filename) || '';
		this.lineno = (init && init.lineno) || 0;
		this.colno = (init && init.colno) || 0;
	}
}
globalThis.ErrorEvent = ErrorEvent;
globalThis.reportError = function(error) {
	var msg = '';
	if (error !== null && error !== undefined) {
		msg = error.message !== undefined ? error.message : String(error);
	}
	var ev = new ErrorEvent('error', { error: error, message: msg });
	globalThis.dispatchEvent(ev);
};
`

// setupReportError evaluates the reportError/ErrorEvent polyfill.
// Must be called AFTER setupAbort so Event and EventTarget exist.
func setupReportError(_ *v8.Isolate, ctx *v8.Context, _ *eventLoop) error {
	// Ensure globalThis is an EventTarget.
	if _, err := ctx.RunScript(`
		if (typeof globalThis.addEventListener !== 'function') {
			var __gt = new EventTarget();
			globalThis.addEventListener = __gt.addEventListener.bind(__gt);
			globalThis.removeEventListener = __gt.removeEventListener.bind(__gt);
			globalThis.dispatchEvent = __gt.dispatchEvent.bind(__gt);
			globalThis._listeners = __gt._listeners;
		}
	`, "globalthis_eventtarget.js"); err != nil {
		return fmt.Errorf("setting up globalThis as EventTarget: %w", err)
	}
	if _, err := ctx.RunScript(reportErrorJS, "report_error.js"); err != nil {
		return fmt.Errorf("evaluating report_error.js: %w", err)
	}
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
