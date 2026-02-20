package worker

import (
	"fmt"

	v8 "github.com/tommie/v8go"
)

// encodingJS implements global atob() and btoa() as pure JavaScript.
// Using a pure-JS implementation avoids any boundary-crossing issues
// with binary strings containing null bytes.
const encodingJS = `
(function() {
	const _e = 'ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/';
	const _d = new Uint8Array(128);
	for (let i = 0; i < _e.length; i++) _d[_e.charCodeAt(i)] = i;
	const _v = new Uint8Array(128);
	for (let i = 0; i < _e.length; i++) _v[_e.charCodeAt(i)] = 1;
	_v[61] = 1; // '='

	// btoa(data) — encodes a binary (Latin-1) string to base64.
	// Matches the Web API: each char must have code point 0-255.
	globalThis.btoa = function(data) {
		if (arguments.length < 1) throw new TypeError("btoa requires at least 1 argument(s)");
		const s = String(data);
		const len = s.length;
		if (len === 0) return '';
		// Validate Latin-1 range and collect bytes in one pass.
		const bytes = new Uint8Array(len);
		for (let i = 0; i < len; i++) {
			const ch = s.charCodeAt(i);
			if (ch > 255) throw new Error("btoa: string contains characters outside of the Latin1 range");
			bytes[i] = ch;
		}
		// Base64 encode using array + join to avoid O(n^2) concatenation.
		const out = [];
		for (let i = 0; i < len; i += 3) {
			const a = bytes[i];
			const b = i + 1 < len ? bytes[i + 1] : 0;
			const c = i + 2 < len ? bytes[i + 2] : 0;
			out.push(
				_e[a >> 2],
				_e[((a & 3) << 4) | (b >> 4)],
				i + 1 < len ? _e[((b & 15) << 2) | (c >> 6)] : '=',
				i + 2 < len ? _e[c & 63] : '='
			);
		}
		return out.join('');
	};

	// atob(data) — decodes a base64-encoded string to a binary (Latin-1) string.
	// Matches the Web API: tolerates missing padding and ASCII whitespace.
	globalThis.atob = function(data) {
		if (arguments.length < 1) throw new TypeError("atob requires at least 1 argument(s)");
		let b64 = String(data);
		// Strip ASCII whitespace (matches browser behavior).
		b64 = b64.replace(/[\t\n\f\r ]/g, '');
		if (b64.length === 0) return '';
		// Per the HTML spec: if length is divisible by 4, strip trailing '=' or '=='.
		// Then reject if length mod 4 is 1 (no valid encoding produces this).
		if (b64.length % 4 === 0) {
			if (b64[b64.length - 1] === '=') {
				b64 = b64.slice(0, b64[b64.length - 2] === '=' ? -2 : -1);
			}
		}
		if (b64.length % 4 === 1) {
			throw new Error("atob: invalid base64 string");
		}
		// Validate: only base64 alphabet allowed (no '=' at this point).
		for (let i = 0; i < b64.length; i++) {
			const ch = b64.charCodeAt(i);
			if (ch >= 128 || !_v[ch] || ch === 61) {
				throw new Error("atob: invalid base64 string");
			}
		}
		// Re-add padding for the decoder.
		while (b64.length % 4 !== 0) b64 += '=';
		// Decode to byte array.
		let pad = 0;
		if (b64[b64.length - 1] === '=') pad++;
		if (b64[b64.length - 2] === '=') pad++;
		const outLen = (b64.length / 4) * 3 - pad;
		const bytes = new Uint8Array(outLen);
		let j = 0;
		for (let i = 0; i < b64.length; i += 4) {
			const a = _d[b64.charCodeAt(i)];
			const b = _d[b64.charCodeAt(i + 1)];
			const c = _d[b64.charCodeAt(i + 2)];
			const d = _d[b64.charCodeAt(i + 3)];
			bytes[j++] = (a << 2) | (b >> 4);
			if (j < outLen) bytes[j++] = ((b & 15) << 4) | (c >> 2);
			if (j < outLen) bytes[j++] = ((c & 3) << 6) | d;
		}
		// Convert bytes to Latin-1 string. Chunked String.fromCharCode.apply
		// avoids stack overflow on large buffers and is efficient.
		const CHUNK = 4096;
		let result = '';
		for (let i = 0; i < outLen; i += CHUNK) {
			const end = Math.min(i + CHUNK, outLen);
			result += String.fromCharCode.apply(null, bytes.subarray(i, end));
		}
		return result;
	};
})();
`

// setupEncoding evaluates the pure-JS atob/btoa implementations.
func setupEncoding(_ *v8.Isolate, ctx *v8.Context, _ *eventLoop) error {
	if _, err := ctx.RunScript(encodingJS, "encoding.js"); err != nil {
		return fmt.Errorf("evaluating encoding.js: %w", err)
	}
	return nil
}
