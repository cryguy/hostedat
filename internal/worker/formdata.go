package worker

import (
	"fmt"

	"github.com/fastschema/qjs"
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

		if (parts) {
			for (const part of parts) {
				if (typeof part === 'string') {
					this._parts.push(part);
				} else if (part instanceof Blob) {
					this._parts.push(...part._parts);
				} else if (part instanceof ArrayBuffer) {
					const arr = new Uint8Array(part);
					let s = '';
					for (let i = 0; i < arr.length; i++) s += String.fromCharCode(arr[i]);
					this._parts.push(s);
				} else if (ArrayBuffer.isView(part)) {
					const arr = new Uint8Array(part.buffer, part.byteOffset, part.byteLength);
					let s = '';
					for (let i = 0; i < arr.length; i++) s += String.fromCharCode(arr[i]);
					this._parts.push(s);
				} else {
					this._parts.push(String(part));
				}
			}
		}
	}

	get size() {
		let total = 0;
		for (const part of this._parts) {
			// Use TextEncoder for accurate byte count.
			const enc = new TextEncoder();
			total += enc.encode(part).length;
		}
		return total;
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
func setupFormData(rt *qjs.Runtime) error {
	if _, err := rt.Eval("formdata.js", qjs.Code(formdataJS)); err != nil {
		return fmt.Errorf("evaluating formdata.js: %w", err)
	}
	return nil
}
