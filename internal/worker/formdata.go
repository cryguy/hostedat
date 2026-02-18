package worker

import (
	"fmt"

	v8 "github.com/tommie/v8go"
)

// formdataJS implements Blob, File, and FormData as pure JS polyfills.
// These cover the common Workers patterns for handling multipart form data,
// file uploads, and binary data construction.
const formdataJS = `
(function() {

// --- Blob ---

class Blob {
	constructor(parts, options) {
		options = options || {};
		this.type = options.type || '';
		this._parts = [];
		this._size = 0;

		if (parts) {
			const enc = new TextEncoder();
			for (const part of parts) {
				if (typeof part === 'string') {
					this._parts.push(part);
					this._size += enc.encode(part).length;
				} else if (part instanceof Blob) {
					this._parts.push(...part._parts);
					this._size += part._size;
				} else if (part instanceof ArrayBuffer) {
					const arr = new Uint8Array(part);
					const CHUNK = 1024;
					let s = '';
					for (let i = 0; i < arr.length; i += CHUNK) {
						const end = Math.min(i + CHUNK, arr.length);
						s += String.fromCharCode.apply(null, arr.subarray(i, end));
					}
					this._parts.push(s);
					this._size += arr.length;
				} else if (ArrayBuffer.isView(part)) {
					const arr = new Uint8Array(part.buffer, part.byteOffset, part.byteLength);
					const CHUNK = 1024;
					let s = '';
					for (let i = 0; i < arr.length; i += CHUNK) {
						const end = Math.min(i + CHUNK, arr.length);
						s += String.fromCharCode.apply(null, arr.subarray(i, end));
					}
					this._parts.push(s);
					this._size += arr.length;
				} else {
					const str = String(part);
					this._parts.push(str);
					this._size += enc.encode(str).length;
				}
			}
		}
	}

	get size() {
		return this._size;
	}

	slice(start, end, contentType) {
		const full = this._parts.join('');
		const sliced = full.slice(start || 0, end !== undefined ? end : full.length);
		return new Blob([sliced], { type: contentType || this.type });
	}

	async text() {
		return this._parts.join('');
	}

	async arrayBuffer() {
		const text = this._parts.join('');
		const enc = new TextEncoder();
		return enc.encode(text).buffer;
	}
}

// --- File ---

class File extends Blob {
	constructor(parts, name, options) {
		super(parts, options);
		this.name = name;
		this.lastModified = (options && options.lastModified) || Date.now();
	}
}

// --- FormData ---

class FormData {
	constructor() {
		this._entries = [];
	}

	append(name, value, filename) {
		if (value instanceof Blob && !(value instanceof File)) {
			value = new File([value], filename || 'blob', { type: value.type });
		}
		this._entries.push([String(name), value]);
	}

	set(name, value, filename) {
		this.delete(name);
		this.append(name, value, filename);
	}

	get(name) {
		const entry = this._entries.find(([k]) => k === name);
		return entry ? entry[1] : null;
	}

	getAll(name) {
		return this._entries.filter(([k]) => k === name).map(([, v]) => v);
	}

	has(name) {
		return this._entries.some(([k]) => k === name);
	}

	delete(name) {
		this._entries = this._entries.filter(([k]) => k !== name);
	}

	entries() {
		return this._entries[Symbol.iterator]();
	}

	keys() {
		return this._entries.map(([k]) => k)[Symbol.iterator]();
	}

	values() {
		return this._entries.map(([, v]) => v)[Symbol.iterator]();
	}

	forEach(callback, thisArg) {
		for (const [name, value] of this._entries) {
			callback.call(thisArg, value, name, this);
		}
	}

	[Symbol.iterator]() {
		return this.entries();
	}
}

globalThis.Blob = Blob;
globalThis.File = File;
globalThis.FormData = FormData;

})();
`

// setupFormData evaluates the FormData/Blob/File polyfills.
func setupFormData(iso *v8.Isolate, ctx *v8.Context, el *eventLoop) error {
	if _, err := ctx.RunScript(formdataJS, "formdata.js"); err != nil {
		return fmt.Errorf("evaluating formdata.js: %w", err)
	}
	return nil
}
