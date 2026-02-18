package worker

import (
	"fmt"

	"github.com/fastschema/qjs"
)

// bodyTypesJS patches Request and Response prototypes with:
// - text()/arrayBuffer()/json() that handle non-string body types
//   (ArrayBuffer, TypedArray, Blob, URLSearchParams, FormData, ReadableStream)
// - formData() method for parsing multipart/form-data and url-encoded bodies
//
// Uses prototype patching (not constructor wrapping) because QuickJS compiled
// bytecode modules resolve constructors at load time, before wrapper functions
// can intercept them.
//
// This must be evaluated AFTER setupWebAPIs, setupStreams, and setupFormData
// so that all referenced types (Blob, FormData, ReadableStream, File) are defined.
const bodyTypesJS = `
(function() {

// Convert any body type to a string.
function bodyToString(body) {
	if (body === null || body === undefined) return '';
	if (typeof body === 'string') return body;
	if (body instanceof ArrayBuffer) {
		var arr = new Uint8Array(body);
		var s = '';
		for (var i = 0; i < arr.length; i++) s += String.fromCharCode(arr[i]);
		return s;
	}
	if (ArrayBuffer.isView(body)) {
		var arr2 = new Uint8Array(body.buffer, body.byteOffset, body.byteLength);
		var s2 = '';
		for (var i2 = 0; i2 < arr2.length; i2++) s2 += String.fromCharCode(arr2[i2]);
		return s2;
	}
	if (body instanceof Blob) {
		return body._parts.join('');
	}
	if (body instanceof URLSearchParams) {
		return body.toString();
	}
	if (body instanceof FormData) {
		var boundary = '----FormDataBoundary' + Math.random().toString(36).slice(2);
		var result = '';
		body.forEach(function(value, name) {
			result += '--' + boundary + '\r\n';
			if (typeof value === 'string') {
				result += 'Content-Disposition: form-data; name="' + name + '"\r\n\r\n';
				result += value + '\r\n';
			} else {
				var fname = value.name || 'blob';
				result += 'Content-Disposition: form-data; name="' + name + '"; filename="' + fname + '"\r\n';
				if (value.type) result += 'Content-Type: ' + value.type + '\r\n';
				result += '\r\n';
				result += value._parts.join('') + '\r\n';
			}
		});
		result += '--' + boundary + '--\r\n';
		return result;
	}
	if (body instanceof ReadableStream) {
		var s3 = '';
		for (var i3 = 0; i3 < body._queue.length; i3++) {
			var chunk = body._queue[i3];
			if (typeof chunk === 'string') { s3 += chunk; }
			else if (chunk instanceof Uint8Array) {
				for (var j = 0; j < chunk.length; j++) s3 += String.fromCharCode(chunk[j]);
			} else { s3 += String(chunk); }
		}
		body._queue = [];
		return s3;
	}
	return String(body);
}

// Parse multipart/form-data body.
function parseMultipart(text, contentType) {
	var fd = new FormData();
	var m = contentType.match(/boundary=([^\s;]+)/);
	if (!m) return fd;
	var boundary = m[1];
	var parts = text.split('--' + boundary);
	for (var i = 1; i < parts.length; i++) {
		var part = parts[i];
		if (part.indexOf('--') === 0) break;
		var sepIdx = part.indexOf('\r\n\r\n');
		if (sepIdx === -1) continue;
		var headerSection = part.slice(0, sepIdx);
		var body = part.slice(sepIdx + 4).replace(/\r\n$/, '');
		var dispMatch = headerSection.match(/Content-Disposition:\s*form-data;\s*name="([^"]+)"(?:;\s*filename="([^"]+)")?/i);
		if (!dispMatch) continue;
		var name = dispMatch[1];
		var filename = dispMatch[2];
		if (filename !== undefined) {
			var ctMatch = headerSection.match(/Content-Type:\s*([^\r\n]+)/i);
			var ftype = ctMatch ? ctMatch[1].trim() : '';
			fd.append(name, new File([body], filename, { type: ftype }));
		} else {
			fd.append(name, body);
		}
	}
	return fd;
}

// Patch text() on both prototypes to handle non-string _body.
Request.prototype.text = async function() {
	return bodyToString(this._body);
};

Response.prototype.text = async function() {
	return bodyToString(this._body);
};

// Patch arrayBuffer() to handle non-string _body.
Request.prototype.arrayBuffer = async function() {
	if (this._body instanceof ArrayBuffer) return this._body;
	if (ArrayBuffer.isView(this._body)) return this._body.buffer.slice(this._body.byteOffset, this._body.byteOffset + this._body.byteLength);
	var t = bodyToString(this._body);
	var enc = new TextEncoder();
	return enc.encode(t).buffer;
};

Response.prototype.arrayBuffer = async function() {
	if (this._body instanceof ArrayBuffer) return this._body;
	if (ArrayBuffer.isView(this._body)) return this._body.buffer.slice(this._body.byteOffset, this._body.byteOffset + this._body.byteLength);
	var t = bodyToString(this._body);
	var enc = new TextEncoder();
	return enc.encode(t).buffer;
};

// Patch json() to use bodyToString.
Request.prototype.json = async function() {
	return JSON.parse(bodyToString(this._body));
};

Response.prototype.json = async function() {
	return JSON.parse(bodyToString(this._body));
};

// Add formData() to both prototypes.
Request.prototype.formData = async function() {
	var ct = this.headers.get('content-type') || '';
	var text = bodyToString(this._body);
	if (ct.indexOf('application/x-www-form-urlencoded') !== -1) {
		var fd = new FormData();
		var params = new URLSearchParams(text);
		params.forEach(function(v, k) { fd.append(k, v); });
		return fd;
	}
	if (ct.indexOf('multipart/form-data') !== -1) {
		return parseMultipart(text, ct);
	}
	throw new TypeError('Could not parse content as FormData');
};

Response.prototype.formData = async function() {
	var ct = this.headers.get('content-type') || '';
	var text = bodyToString(this._body);
	if (ct.indexOf('application/x-www-form-urlencoded') !== -1) {
		var fd = new FormData();
		var params = new URLSearchParams(text);
		params.forEach(function(v, k) { fd.append(k, v); });
		return fd;
	}
	if (ct.indexOf('multipart/form-data') !== -1) {
		return parseMultipart(text, ct);
	}
	throw new TypeError('Could not parse content as FormData');
};

})();
`

// setupBodyTypes patches Request/Response with extended body type support.
// Must be called after setupWebAPIs, setupStreams, and setupFormData.
func setupBodyTypes(rt *qjs.Runtime) error {
	if _, err := rt.Eval("bodytypes.js", qjs.Code(bodyTypesJS)); err != nil {
		return fmt.Errorf("evaluating bodytypes.js: %w", err)
	}
	return nil
}
