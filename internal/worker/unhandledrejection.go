package worker

import (
	"fmt"

	v8 "github.com/tommie/v8go"
)

// unhandledRejectionJS provides a best-effort polyfill for unhandled promise
// rejection tracking. It creates PromiseRejectionEvent (extending Event) and
// uses microtask-based tracking to detect unhandled rejections.
const unhandledRejectionJS = `
(function() {

class PromiseRejectionEvent extends Event {
	constructor(type, init) {
		super(type, init);
		this.promise = (init && init.promise) || null;
		this.reason = (init && init.reason) !== undefined ? init.reason : undefined;
	}
}

// Track pending rejections. Each unhandled rejection is queued via microtask;
// if handled before the microtask fires, it is removed from the set.
const _pendingRejections = new Map();
let _rejectionId = 0;

const _origPromise = globalThis.Promise;

// Wrap Promise to intercept .then and .catch for rejection tracking.
const _origThen = _origPromise.prototype.then;

_origPromise.prototype.then = function(onFulfilled, onRejected) {
	const result = _origThen.call(this, onFulfilled, onRejected);
	// If a rejection handler is provided, mark this promise as handled.
	if (typeof onRejected === 'function' && this.__rejectionId !== undefined) {
		_pendingRejections.delete(this.__rejectionId);
	}
	return result;
};

// Patch Promise.prototype.catch too.
const _origCatch = _origPromise.prototype.catch;
_origPromise.prototype.catch = function(onRejected) {
	const result = _origCatch.call(this, onRejected);
	if (typeof onRejected === 'function' && this.__rejectionId !== undefined) {
		_pendingRejections.delete(this.__rejectionId);
	}
	return result;
};

// Override Promise constructor to detect unhandled rejections.
globalThis.__trackRejection = function(promise, reason) {
	const id = ++_rejectionId;
	try {
		Object.defineProperty(promise, '__rejectionId', { value: id, writable: true, configurable: true });
	} catch(e) {
		// If we can't define the property, skip tracking.
		return;
	}
	_pendingRejections.set(id, { promise, reason });
	// Use microtask to check if it's still unhandled.
	queueMicrotask(function() {
		if (_pendingRejections.has(id)) {
			_pendingRejections.delete(id);
			const event = new PromiseRejectionEvent('unhandledrejection', {
				promise: promise,
				reason: reason,
				cancelable: true,
			});
			globalThis.dispatchEvent(event);
		}
	});
};

// Make globalThis an EventTarget if it isn't already.
if (typeof globalThis.addEventListener !== 'function') {
	const et = new EventTarget();
	globalThis.addEventListener = et.addEventListener.bind(et);
	globalThis.removeEventListener = et.removeEventListener.bind(et);
	globalThis.dispatchEvent = et.dispatchEvent.bind(et);
}

globalThis.PromiseRejectionEvent = PromiseRejectionEvent;

})();
`

// setupUnhandledRejection registers PromiseRejectionEvent and best-effort
// unhandled rejection tracking on globalThis.
func setupUnhandledRejection(iso *v8.Isolate, ctx *v8.Context, el *eventLoop) error {
	if _, err := ctx.RunScript(unhandledRejectionJS, "unhandledrejection.js"); err != nil {
		return fmt.Errorf("evaluating unhandledrejection.js: %w", err)
	}
	return nil
}
