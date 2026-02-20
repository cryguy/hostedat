package worker

import (
	"fmt"

	v8 "github.com/tommie/v8go"
)

// messageChannelJS defines MessagePort and MessageChannel as pure JS polyfills
// using the existing EventTarget class. Messages between ports are cloned
// via structuredClone (JSON round-trip).
const messageChannelJS = `
(function() {

class MessagePort extends EventTarget {
	constructor() {
		super();
		this._remote = null;
		this._started = false;
		this._closed = false;
		this._queue = [];
		this.onmessage = null;
		this.onmessageerror = null;
	}

	postMessage(data) {
		if (this._closed) return;
		if (!this._remote) return;
		let cloned;
		try {
			cloned = structuredClone(data);
		} catch(e) {
			if (this._remote.onmessageerror) {
				const errEvt = new Event('messageerror');
				errEvt.data = e;
				this._remote.onmessageerror(errEvt);
			}
			this._remote.dispatchEvent(Object.assign(new Event('messageerror'), { data: e }));
			return;
		}
		const remote = this._remote;
		const evt = new Event('message');
		evt.data = cloned;
		if (remote._started) {
			queueMicrotask(function() {
				if (remote.onmessage) remote.onmessage(evt);
				remote.dispatchEvent(evt);
			});
		} else {
			remote._queue.push(evt);
		}
	}

	start() {
		if (this._started) return;
		this._started = true;
		const self = this;
		const pending = this._queue.splice(0);
		for (const evt of pending) {
			queueMicrotask(function() {
				if (self.onmessage) self.onmessage(evt);
				self.dispatchEvent(evt);
			});
		}
	}

	close() {
		this._closed = true;
		this._remote = null;
	}
}

class MessageChannel {
	constructor() {
		this.port1 = new MessagePort();
		this.port2 = new MessagePort();
		this.port1._remote = this.port2;
		this.port2._remote = this.port1;
		// Auto-start ports (Cloudflare Workers behavior).
		this.port1._started = true;
		this.port2._started = true;
	}
}

globalThis.MessagePort = MessagePort;
globalThis.MessageChannel = MessageChannel;

})();
`

// setupMessageChannel registers MessageChannel and MessagePort globals.
func setupMessageChannel(_ *v8.Isolate, ctx *v8.Context, _ *eventLoop) error {
	if _, err := ctx.RunScript(messageChannelJS, "messagechannel.js"); err != nil {
		return fmt.Errorf("evaluating messagechannel.js: %w", err)
	}
	return nil
}
