package worker

import (
	"fmt"

	v8 "github.com/tommie/v8go"
)

// streamsJS implements ReadableStream, WritableStream, and TransformStream
// as pure JS polyfills. This covers the core patterns Workers scripts use:
// constructing streams with start/pull/cancel controllers, reading via
// getReader(), writing via getWriter(), and piping via TransformStream.
const streamsJS = `
(function() {

// --- ReadableStream ---

class ReadableStreamDefaultController {
	constructor(stream) {
		this._stream = stream;
		this._closeRequested = false;
	}
	enqueue(chunk) {
		if (this._closeRequested) throw new TypeError('Cannot enqueue after close');
		this._stream._queue.push(chunk);
		this._stream._pull();
	}
	close() {
		this._closeRequested = true;
		this._stream._closeInternal();
	}
	error(e) {
		this._stream._errorInternal(e);
	}
	get desiredSize() {
		return this._stream._highWaterMark - this._stream._queue.length;
	}
}

class ReadableStreamDefaultReader {
	constructor(stream) {
		if (stream._locked) throw new TypeError('ReadableStream is already locked');
		this._stream = stream;
		stream._locked = true;
		stream._reader = this;
		this._closed = false;
		// Cache the closed promise and its resolver (spec requires same identity).
		const self = this;
		this._closedPromise = new Promise(function(resolve, reject) {
			self._closedResolve = resolve;
			self._closedReject = reject;
		});
		if (stream._closed) {
			this._closedResolve();
		}
	}
	async read() {
		const stream = this._stream;
		if (stream._queue.length > 0) {
			return { value: stream._queue.shift(), done: false };
		}
		if (stream._closed) {
			return { value: undefined, done: true };
		}
		if (stream._errored) {
			throw stream._error;
		}
		// Wait for data.
		return new Promise((resolve, reject) => {
			stream._pendingReads.push({ resolve, reject });
			// Trigger pull if source has one.
			if (stream._pullFn && !stream._pulling) {
				stream._pulling = true;
				Promise.resolve().then(() => {
					stream._pulling = false;
					try { var r = stream._pullFn(stream._controller); if (r && typeof r.then === "function") r.then(undefined, function(e) { stream._errorInternal(e); }); } catch(e) { stream._errorInternal(e); }
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

class ReadableStream {
	constructor(underlyingSource, strategy) {
		this._queue = [];
		this._locked = false;
		this._reader = null;
		this._closed = false;
		this._errored = false;
		this._error = null;
		this._pendingReads = [];
		this._pulling = false;
		this._highWaterMark = (strategy && strategy.highWaterMark) || 1;

		this._controller = new ReadableStreamDefaultController(this);
		this._pullFn = null;
		this._cancelFn = null;

		if (underlyingSource) {
			if (typeof underlyingSource.pull === 'function') {
				this._pullFn = underlyingSource.pull.bind(underlyingSource);
			}
			if (typeof underlyingSource.cancel === 'function') {
				this._cancelFn = underlyingSource.cancel.bind(underlyingSource);
			}
			if (typeof underlyingSource.start === 'function') {
				underlyingSource.start(this._controller);
			}
		}
	}

	getReader() {
		return new ReadableStreamDefaultReader(this);
	}

	cancel(reason) {
		this._closed = true;
		if (this._cancelFn) this._cancelFn(reason);
		this._drainPending();
		return Promise.resolve();
	}

	get locked() { return this._locked; }

	pipeTo(destination, options) {
		if (this._locked) return Promise.reject(new TypeError('ReadableStream is locked'));
		if (!(destination instanceof WritableStream)) return Promise.reject(new TypeError('pipeTo requires a WritableStream'));
		options = options || {};
		const preventClose = !!options.preventClose;
		const preventAbort = !!options.preventAbort;
		const preventCancel = !!options.preventCancel;
		const reader = this.getReader();
		const writer = destination.getWriter();
		async function pump() {
			try {
				while (true) {
					const { value, done } = await reader.read();
					if (done) {
						if (!preventClose) await writer.close();
						else writer.releaseLock();
						break;
					}
					await writer.write(value);
				}
			} catch(e) {
				if (!preventAbort) {
					try { await writer.abort(e); } catch(_) {}
				}
				if (!preventCancel) {
					try { await reader.cancel(e); } catch(_) {}
				}
				throw e;
			} finally {
				reader.releaseLock();
			}
		}
		return pump();
	}

	pipeThrough(transform, options) {
		if (this._locked) throw new TypeError('ReadableStream is locked');
		if (!transform || typeof transform !== 'object') throw new TypeError('pipeThrough requires a transform object');
		if (!(transform.writable instanceof WritableStream)) throw new TypeError('pipeThrough requires transform.writable to be a WritableStream');
		this.pipeTo(transform.writable, options);
		return transform.readable;
	}

	tee() {
		if (this._locked) throw new TypeError('ReadableStream is locked');
		const reader = this.getReader();
		let closed = false;
		let branch1Controller;
		let branch2Controller;
		const branch1 = new ReadableStream({
			start(controller) { branch1Controller = controller; },
		});
		const branch2 = new ReadableStream({
			start(controller) { branch2Controller = controller; },
		});
		async function pump() {
			try {
				while (true) {
					const { value, done } = await reader.read();
					if (done) {
						if (!closed) {
							closed = true;
							branch1Controller.close();
							branch2Controller.close();
						}
						return;
					}
					branch1Controller.enqueue(value);
					branch2Controller.enqueue(value);
				}
			} catch(e) {
				branch1Controller.error(e);
				branch2Controller.error(e);
			}
		}
		pump();
		return [branch1, branch2];
	}

	_pull() {
		// Deliver queued chunks to pending reads.
		while (this._queue.length > 0 && this._pendingReads.length > 0) {
			const chunk = this._queue.shift();
			const { resolve } = this._pendingReads.shift();
			resolve({ value: chunk, done: false });
		}
	}

	_closeInternal() {
		this._closed = true;
		this._drainPending();
		if (this._reader && this._reader._closedResolve) {
			this._reader._closedResolve();
		}
	}

	_errorInternal(e) {
		this._errored = true;
		this._error = e;
		for (const { reject } of this._pendingReads) {
			reject(e);
		}
		this._pendingReads = [];
		if (this._reader && this._reader._closedReject) {
			this._reader._closedReject(e);
		}
	}

	_drainPending() {
		while (this._pendingReads.length > 0) {
			const { resolve } = this._pendingReads.shift();
			if (this._queue.length > 0) {
				resolve({ value: this._queue.shift(), done: false });
			} else {
				resolve({ value: undefined, done: true });
			}
		}
	}

	// Async iteration support.
	async *[Symbol.asyncIterator]() {
		const reader = this.getReader();
		try {
			while (true) {
				const { value, done } = await reader.read();
				if (done) return;
				yield value;
			}
		} finally {
			reader.releaseLock();
		}
	}
}

// --- WritableStream ---

class WritableStreamDefaultController {
	constructor(stream) {
		this._stream = stream;
	}
	error(e) {
		this._stream._errorInternal(e);
	}
}

class WritableStreamDefaultWriter {
	constructor(stream) {
		if (stream._locked) throw new TypeError('WritableStream is already locked');
		this._stream = stream;
		stream._locked = true;
		// Cache the closed promise and its resolver (spec requires same identity).
		const self = this;
		this._closedPromise = new Promise(function(resolve, reject) {
			self._closedResolve = resolve;
			self._closedReject = reject;
		});
		if (stream._closed) {
			this._closedResolve();
		}
	}
	write(chunk) {
		if (this._stream._closed) throw new TypeError('Cannot write to a closed stream');
		if (this._stream._errored) throw this._stream._error;
		if (this._stream._writeFn) {
			const result = this._stream._writeFn(chunk, this._stream._controller);
			if (result && typeof result.then === 'function') return result;
		}
		return Promise.resolve();
	}
	close() {
		const self = this;
		if (this._stream._closeFn) {
			const result = this._stream._closeFn();
			if (result && typeof result.then === 'function') {
				return result.then(function() {
					self._stream._closed = true;
					if (self._closedResolve) self._closedResolve();
				});
			}
		}
		this._stream._closed = true;
		if (this._closedResolve) this._closedResolve();
		return Promise.resolve();
	}
	abort(reason) {
		const self = this;
		if (this._stream._abortFn) {
			const result = this._stream._abortFn(reason);
			if (result && typeof result.then === 'function') {
				return result.then(function() {
					self._stream._closed = true;
					if (self._closedResolve) self._closedResolve();
				});
			}
		}
		this._stream._closed = true;
		if (this._closedResolve) this._closedResolve();
		return Promise.resolve();
	}
	releaseLock() {
		this._stream._locked = false;
	}
	get closed() {
		return this._closedPromise;
	}
	get ready() { return Promise.resolve(); }
}

class WritableStream {
	constructor(underlyingSink, strategy) {
		this._locked = false;
		this._closed = false;
		this._errored = false;
		this._error = null;
		this._controller = new WritableStreamDefaultController(this);
		this._writeFn = null;
		this._closeFn = null;
		this._abortFn = null;

		if (underlyingSink) {
			if (typeof underlyingSink.write === 'function') {
				this._writeFn = underlyingSink.write.bind(underlyingSink);
			}
			if (typeof underlyingSink.close === 'function') {
				this._closeFn = underlyingSink.close.bind(underlyingSink);
			}
			if (typeof underlyingSink.abort === 'function') {
				this._abortFn = underlyingSink.abort.bind(underlyingSink);
			}
			if (typeof underlyingSink.start === 'function') {
				underlyingSink.start(this._controller);
			}
		}
	}

	getWriter() {
		return new WritableStreamDefaultWriter(this);
	}

	get locked() { return this._locked; }

	_errorInternal(e) {
		this._errored = true;
		this._error = e;
	}
}

// --- TransformStream ---

class TransformStream {
	constructor(transformer, writableStrategy, readableStrategy) {
		const self = this;
		let readableController;

		this.readable = new ReadableStream({
			start(controller) {
				readableController = controller;
			}
		}, readableStrategy);

		const transformFn = transformer && typeof transformer.transform === 'function'
			? transformer.transform.bind(transformer)
			: null;
		const flushFn = transformer && typeof transformer.flush === 'function'
			? transformer.flush.bind(transformer)
			: null;

		const transformController = {
			enqueue(chunk) { readableController.enqueue(chunk); },
			error(e) { readableController.error(e); },
			terminate() { readableController.close(); },
		};

		this.writable = new WritableStream({
			async write(chunk) {
				if (transformFn) {
					await transformFn(chunk, transformController);
				} else {
					// Identity transform.
					readableController.enqueue(chunk);
				}
			},
			async close() {
				if (flushFn) {
					await flushFn(transformController);
				}
				readableController.close();
			}
		}, writableStrategy);
	}
}

// --- ReadableStream.from() ---

ReadableStream.from = function(asyncIterable) {
	if (asyncIterable == null) {
		throw new TypeError('ReadableStream.from called on null or undefined');
	}
	const asyncIteratorMethod = asyncIterable[Symbol.asyncIterator];
	const iteratorMethod = asyncIterable[Symbol.iterator];
	if (typeof asyncIteratorMethod !== 'function' && typeof iteratorMethod !== 'function') {
		throw new TypeError('ReadableStream.from requires an iterable or async iterable');
	}
	const iterator = typeof asyncIteratorMethod === 'function'
		? asyncIterable[Symbol.asyncIterator]()
		: asyncIterable[Symbol.iterator]();
	return new ReadableStream({
		async pull(controller) {
			const { value, done } = await iterator.next();
			if (done) {
				controller.close();
			} else {
				controller.enqueue(value);
			}
		}
	});
};

// --- FixedLengthStream ---

class FixedLengthStream {
	constructor(expectedLength) {
		if (typeof expectedLength !== 'number' || expectedLength < 0) {
			throw new TypeError('FixedLengthStream requires a non-negative number');
		}
		this._expectedLength = expectedLength;
		let bytesWritten = 0;
		const expected = expectedLength;
		const ts = new TransformStream({
			transform(chunk, controller) {
				const len = (chunk && typeof chunk.byteLength === 'number')
					? chunk.byteLength
					: (chunk && typeof chunk.length === 'number' ? chunk.length : 0);
				bytesWritten += len;
				if (bytesWritten > expected) {
					throw new TypeError(
						'FixedLengthStream: exceeded expected length of ' + expected + ' bytes (wrote ' + bytesWritten + ')'
					);
				}
				controller.enqueue(chunk);
			},
			flush(controller) {
				if (bytesWritten !== expected) {
					throw new TypeError(
						'FixedLengthStream: expected ' + expected + ' bytes but received ' + bytesWritten
					);
				}
			}
		});
		this.readable = ts.readable;
		this.writable = ts.writable;
	}
}

globalThis.ReadableStream = ReadableStream;
globalThis.ReadableStreamDefaultReader = ReadableStreamDefaultReader;
globalThis.WritableStream = WritableStream;
globalThis.WritableStreamDefaultWriter = WritableStreamDefaultWriter;
globalThis.TransformStream = TransformStream;
globalThis.FixedLengthStream = FixedLengthStream;

})();
`

// setupStreams evaluates the Streams API polyfills.
func setupStreams(_ *v8.Isolate, ctx *v8.Context, _ *eventLoop) error {
	if _, err := ctx.RunScript(streamsJS, "streams.js"); err != nil {
		return fmt.Errorf("evaluating streams.js: %w", err)
	}
	return nil
}
