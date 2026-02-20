package worker

import (
	"fmt"

	v8 "github.com/tommie/v8go"
)

// byobReaderJS adds ReadableStreamBYOBReader and ReadableByteStreamController
// support. It monkey-patches the existing ReadableStream to support
// { type: 'bytes' } underlying sources and getReader({ mode: 'byob' }).
const byobReaderJS = `
(function() {

class ReadableByteStreamController {
	constructor(stream) {
		this._stream = stream;
		this._closeRequested = false;
		this._byobRequest = null;
	}
	enqueue(chunk) {
		if (this._closeRequested) throw new TypeError('Cannot enqueue after close');
		// If there's a pending BYOB read, fulfill it directly.
		if (this._stream._byobReads && this._stream._byobReads.length > 0) {
			const { resolve, view } = this._stream._byobReads.shift();
			const src = chunk instanceof ArrayBuffer
				? new Uint8Array(chunk)
				: chunk instanceof Uint8Array ? chunk : new Uint8Array(chunk.buffer || chunk);
			const dst = new Uint8Array(view.buffer, view.byteOffset, view.byteLength);
			const copyLen = Math.min(src.length, dst.length);
			for (let i = 0; i < copyLen; i++) dst[i] = src[i];
			const filledView = new Uint8Array(view.buffer, view.byteOffset, copyLen);
			resolve({ value: filledView, done: false });
			return;
		}
		// Otherwise queue as normal.
		this._stream._queue.push(chunk);
		this._stream._pull();
	}
	close() {
		this._closeRequested = true;
		this._stream._closeInternal();
		if (this._stream._byobReads) {
			while (this._stream._byobReads.length > 0) {
				const { resolve, view } = this._stream._byobReads.shift();
				const emptyView = new Uint8Array(view.buffer, view.byteOffset, 0);
				resolve({ value: emptyView, done: true });
			}
		}
	}
	error(e) {
		this._stream._errorInternal(e);
		if (this._stream._byobReads) {
			while (this._stream._byobReads.length > 0) {
				const { reject } = this._stream._byobReads.shift();
				reject(e);
			}
		}
	}
	get desiredSize() {
		return this._stream._highWaterMark - this._stream._queue.length;
	}
	get byobRequest() {
		return this._byobRequest;
	}
}

class ReadableStreamBYOBReader {
	constructor(stream) {
		if (stream._locked) throw new TypeError('ReadableStream is already locked');
		if (!stream._byteStream) throw new TypeError('ReadableStreamBYOBReader can only be used with byte streams');
		this._stream = stream;
		stream._locked = true;
		stream._reader = this;
		this._closed = false;
		const self = this;
		this._closedPromise = new Promise(function(resolve, reject) {
			self._closedResolve = resolve;
			self._closedReject = reject;
		});
		if (stream._closed) {
			this._closedResolve();
		}
	}

	read(view) {
		const stream = this._stream;
		if (!view || typeof view.byteLength !== 'number') {
			return Promise.reject(new TypeError('read() requires a typed array view'));
		}
		if (view.byteLength === 0) {
			return Promise.reject(new TypeError('view must have non-zero byteLength'));
		}
		if (stream._errored) {
			return Promise.reject(stream._error);
		}
		// Check queue BEFORE closed — data may have been enqueued before close().
		if (stream._queue.length > 0) {
			const chunk = stream._queue.shift();
			const src = chunk instanceof ArrayBuffer
				? new Uint8Array(chunk)
				: chunk instanceof Uint8Array ? chunk : new Uint8Array(chunk.buffer || chunk);
			const dst = new Uint8Array(view.buffer, view.byteOffset, view.byteLength);
			const copyLen = Math.min(src.length, dst.length);
			for (let i = 0; i < copyLen; i++) dst[i] = src[i];
			const filledView = new Uint8Array(view.buffer, view.byteOffset, copyLen);
			return Promise.resolve({ value: filledView, done: false });
		}
		if (stream._closed) {
			const emptyView = new Uint8Array(view.buffer, view.byteOffset, 0);
			return Promise.resolve({ value: emptyView, done: true });
		}
		return new Promise(function(resolve, reject) {
			stream._byobReads.push({ resolve, reject, view });
			if (stream._pullFn && !stream._pulling) {
				stream._pulling = true;
				Promise.resolve().then(function() {
					stream._pulling = false;
					try {
						var r = stream._pullFn(stream._controller);
						if (r && typeof r.then === 'function') r.then(undefined, function(e) { stream._errorInternal(e); });
					} catch(e) { stream._errorInternal(e); }
				});
			}
		});
	}

	releaseLock() {
		if (this._stream) {
			this._stream._locked = false;
			this._stream._reader = null;
		}
	}

	get closed() {
		return this._closedPromise;
	}

	cancel(reason) {
		return this._stream.cancel(reason);
	}
}

// Monkey-patch ReadableStream to support byte streams and BYOB readers.
const OrigReadableStream = globalThis.ReadableStream;
const origGetReader = OrigReadableStream.prototype.getReader;

// Override the ReadableStream constructor to detect type: 'bytes'.
// We strip 'start' from the source passed to the original constructor so that
// start does NOT run with the default controller. Then we install a
// ReadableByteStreamController and call start ourselves.
globalThis.ReadableStream = function ReadableStream(underlyingSource, strategy) {
	if (underlyingSource && underlyingSource.type === 'bytes') {
		// Build a source without start/pull/cancel so the original constructor
		// creates a bare stream with an empty queue.
		const stream = new OrigReadableStream(undefined, strategy);
		stream._byteStream = true;
		stream._byobReads = [];
		// Replace the controller with a ReadableByteStreamController.
		stream._controller = new ReadableByteStreamController(stream);
		// Wire up pull/cancel from the underlying source.
		if (typeof underlyingSource.pull === 'function') {
			stream._pullFn = underlyingSource.pull.bind(underlyingSource);
		}
		if (typeof underlyingSource.cancel === 'function') {
			stream._cancelFn = underlyingSource.cancel.bind(underlyingSource);
		}
		// Now call start with the byte controller.
		if (typeof underlyingSource.start === 'function') {
			underlyingSource.start(stream._controller);
		}
		return stream;
	}
	// Non-bytes streams pass through to the original constructor.
	return new OrigReadableStream(underlyingSource, strategy);
};

// Copy static methods and prototype.
globalThis.ReadableStream.prototype = OrigReadableStream.prototype;
globalThis.ReadableStream.from = OrigReadableStream.from;

// Patch getReader to support { mode: 'byob' }.
OrigReadableStream.prototype.getReader = function(options) {
	if (options && options.mode === 'byob') {
		return new ReadableStreamBYOBReader(this);
	}
	return origGetReader.call(this, options);
};

globalThis.ReadableStreamBYOBReader = ReadableStreamBYOBReader;
globalThis.ReadableByteStreamController = ReadableByteStreamController;

})();
`

// setupBYOBReader registers ReadableStreamBYOBReader and
// ReadableByteStreamController, monkey-patching the existing ReadableStream.
func setupBYOBReader(_ *v8.Isolate, ctx *v8.Context, _ *eventLoop) error {
	if _, err := ctx.RunScript(byobReaderJS, "byob_reader.js"); err != nil {
		return fmt.Errorf("evaluating byob_reader.js: %w", err)
	}
	return nil
}
