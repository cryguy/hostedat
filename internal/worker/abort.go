package worker

import (
	"fmt"

	v8 "github.com/tommie/v8go"
)

// abortJS defines EventTarget, Event, AbortSignal, and AbortController as
// pure JS polyfills. No Go backing is needed since these are purely
// in-memory event-driven constructs.
const abortJS = `
class Event {
	constructor(type, options) {
		this.type = type;
		this.bubbles = !!(options && options.bubbles);
		this.cancelable = !!(options && options.cancelable);
		this.defaultPrevented = false;
		this.target = null;
		this.currentTarget = null;
		this.timeStamp = performance.now();
	}
	preventDefault() {
		if (this.cancelable) this.defaultPrevented = true;
	}
	stopPropagation() {}
	stopImmediatePropagation() {}
}

class EventTarget {
	constructor() {
		this._listeners = {};
	}
	addEventListener(type, callback, options) {
		if (typeof callback !== 'function') return;
		if (!this._listeners[type]) this._listeners[type] = [];
		const once = options && options.once;
		this._listeners[type].push({ callback, once });
	}
	removeEventListener(type, callback) {
		if (!this._listeners[type]) return;
		this._listeners[type] = this._listeners[type].filter(l => l.callback !== callback);
	}
	dispatchEvent(event) {
		event.target = this;
		event.currentTarget = this;
		const listeners = this._listeners[event.type];
		if (!listeners) return true;
		const copy = listeners.slice();
		for (const entry of copy) {
			entry.callback.call(this, event);
			if (entry.once) {
				this.removeEventListener(event.type, entry.callback);
			}
		}
		return !event.defaultPrevented;
	}
}

class AbortSignal extends EventTarget {
	constructor() {
		super();
		this.aborted = false;
		this.reason = undefined;
	}
	throwIfAborted() {
		if (this.aborted) throw this.reason;
	}
	static abort(reason) {
		const signal = new AbortSignal();
		signal.aborted = true;
		signal.reason = reason !== undefined ? reason : new DOMException('signal is aborted without reason', 'AbortError');
		return signal;
	}
	static timeout(ms) {
		const signal = new AbortSignal();
		setTimeout(() => {
			if (!signal.aborted) {
				signal.aborted = true;
				signal.reason = new DOMException('signal timed out', 'TimeoutError');
				signal.dispatchEvent(new Event('abort'));
			}
		}, ms);
		return signal;
	}
}

class AbortController {
	constructor() {
		this.signal = new AbortSignal();
	}
	abort(reason) {
		if (this.signal.aborted) return;
		this.signal.aborted = true;
		this.signal.reason = reason !== undefined ? reason : new DOMException('signal is aborted without reason', 'AbortError');
		this.signal.dispatchEvent(new Event('abort'));
	}
}

// DOMException polyfill (minimal).
if (typeof DOMException === 'undefined') {
	globalThis.DOMException = class DOMException extends Error {
		constructor(message, name) {
			super(message);
			this.name = name || 'Error';
			this.code = 0;
		}
	};
}

class ScheduledEvent extends Event {
	constructor(scheduledTime, cron) {
		super('scheduled');
		this.scheduledTime = scheduledTime;
		this.cron = cron;
		this._waitUntilPromises = [];
	}
	waitUntil(promise) {
		this._waitUntilPromises.push(Promise.resolve(promise));
	}
}

globalThis.Event = Event;
globalThis.EventTarget = EventTarget;
globalThis.AbortSignal = AbortSignal;
globalThis.AbortController = AbortController;
globalThis.ScheduledEvent = ScheduledEvent;
`

// setupAbort evaluates the AbortController/AbortSignal polyfills.
func setupAbort(iso *v8.Isolate, ctx *v8.Context, el *eventLoop) error {
	if _, err := ctx.RunScript(abortJS, "abort.js"); err != nil {
		return fmt.Errorf("evaluating abort.js: %w", err)
	}
	return nil
}
