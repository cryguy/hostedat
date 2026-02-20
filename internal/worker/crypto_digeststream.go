package worker

import (
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"fmt"
	"hash"
	"strconv"

	v8 "github.com/tommie/v8go"
)

// digestStreamJS defines the DigestStream class, a Cloudflare Workers-compatible
// WritableStream that computes a hash digest as data is written to it.
const digestStreamJS = `
(function() {
	const _b64e = 'ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/';
	const _b64d = new Uint8Array(128);
	for (let i = 0; i < _b64e.length; i++) _b64d[_b64e.charCodeAt(i)] = i;

	function bufToB64(arr) {
		if (arr instanceof ArrayBuffer) arr = new Uint8Array(arr);
		else if (arr.buffer instanceof ArrayBuffer) arr = new Uint8Array(arr.buffer, arr.byteOffset || 0, arr.byteLength || arr.length);
		const len = arr.length;
		let r = '';
		for (let i = 0; i < len; i += 3) {
			const a = arr[i];
			const b = i + 1 < len ? arr[i + 1] : 0;
			const c = i + 2 < len ? arr[i + 2] : 0;
			r += _b64e[a >> 2];
			r += _b64e[((a & 3) << 4) | (b >> 4)];
			r += i + 1 < len ? _b64e[((b & 15) << 2) | (c >> 6)] : '=';
			r += i + 2 < len ? _b64e[c & 63] : '=';
		}
		return r;
	}

	function b64ToBuf(b64) {
		let pad = 0;
		if (b64.length > 0 && b64[b64.length - 1] === '=') pad++;
		if (b64.length > 1 && b64[b64.length - 2] === '=') pad++;
		const outLen = (b64.length * 3 / 4) - pad;
		const buf = new ArrayBuffer(outLen);
		const out = new Uint8Array(buf);
		let j = 0;
		for (let i = 0; i < b64.length; i += 4) {
			const a = _b64d[b64.charCodeAt(i)];
			const b = _b64d[b64.charCodeAt(i + 1)];
			const c = _b64d[b64.charCodeAt(i + 2)];
			const d = _b64d[b64.charCodeAt(i + 3)];
			out[j++] = (a << 2) | (b >> 4);
			if (j < outLen) out[j++] = ((b & 15) << 4) | (c >> 2);
			if (j < outLen) out[j++] = ((c & 3) << 6) | d;
		}
		return buf;
	}

	class DigestStream extends WritableStream {
		constructor(algorithm) {
			const algo = typeof algorithm === 'string' ? algorithm : (algorithm && algorithm.name ? algorithm.name : String(algorithm));
			const reqID = globalThis.__requestID;
			const streamID = __cryptoDigestStreamCreate(reqID, algo);
			let digestResolve;
			const digestPromise = new Promise(function(resolve) { digestResolve = resolve; });

			super({
				write(chunk) {
					let data;
					if (typeof chunk === 'string') {
						const enc = new TextEncoder();
						data = enc.encode(chunk);
					} else if (chunk instanceof ArrayBuffer) {
						data = new Uint8Array(chunk);
					} else if (ArrayBuffer.isView(chunk)) {
						data = new Uint8Array(chunk.buffer, chunk.byteOffset, chunk.byteLength);
					} else {
						throw new TypeError('DigestStream write: expected BufferSource or string');
					}
					const b64 = bufToB64(data);
					__cryptoDigestStreamWrite(reqID, streamID, b64);
				},
				close() {
					const resultB64 = __cryptoDigestStreamFinish(reqID, streamID);
					digestResolve(b64ToBuf(resultB64));
				}
			});
			this.digest = digestPromise;
		}
	}

	// Wire DigestStream onto the crypto object.
	if (typeof globalThis.crypto !== 'undefined') {
		globalThis.crypto.DigestStream = DigestStream;
	}
	globalThis.DigestStream = DigestStream;
})();
`

// newDigestHash creates a hash.Hash for the given algorithm name.
func newDigestHash(algo string) (hash.Hash, error) {
	switch normalizeAlgo(algo) {
	case "SHA-1":
		return sha1.New(), nil
	case "SHA-256":
		return sha256.New(), nil
	case "SHA-384":
		return sha512.New384(), nil
	case "SHA-512":
		return sha512.New(), nil
	default:
		return nil, fmt.Errorf("DigestStream: unsupported algorithm %q", algo)
	}
}

// setupDigestStream registers Go-backed helpers for DigestStream and evaluates
// the JS wrapper.
func setupDigestStream(iso *v8.Isolate, ctx *v8.Context, _ *eventLoop) error {
	// __cryptoDigestStreamCreate(requestID, algorithm) -> streamID string
	_ = ctx.Global().Set("__cryptoDigestStreamCreate", v8.NewFunctionTemplate(iso, func(info *v8.FunctionCallbackInfo) *v8.Value {
		args := info.Args()
		if len(args) < 2 {
			return throwError(iso, errMissingArg("__cryptoDigestStreamCreate", 2).Error())
		}
		reqID, _ := strconv.ParseUint(args[0].String(), 10, 64)
		algo := args[1].String()

		h, err := newDigestHash(algo)
		if err != nil {
			return throwError(iso, err.Error())
		}

		state := getRequestState(reqID)
		if state == nil {
			return throwError(iso, "DigestStream: invalid request state")
		}

		if state.digestStreams == nil {
			state.digestStreams = make(map[string]hash.Hash)
		}
		state.nextDigestID++
		streamID := strconv.FormatInt(state.nextDigestID, 10)
		state.digestStreams[streamID] = h

		val, _ := v8.NewValue(iso, streamID)
		return val
	}).GetFunction(ctx))

	// __cryptoDigestStreamWrite(requestID, streamID, base64data)
	_ = ctx.Global().Set("__cryptoDigestStreamWrite", v8.NewFunctionTemplate(iso, func(info *v8.FunctionCallbackInfo) *v8.Value {
		args := info.Args()
		if len(args) < 3 {
			return throwError(iso, errMissingArg("__cryptoDigestStreamWrite", 3).Error())
		}
		reqID, _ := strconv.ParseUint(args[0].String(), 10, 64)
		streamID := args[1].String()
		dataB64 := args[2].String()

		data, err := base64.StdEncoding.DecodeString(dataB64)
		if err != nil {
			return throwError(iso, "DigestStream write: invalid base64")
		}

		state := getRequestState(reqID)
		if state == nil || state.digestStreams == nil {
			return throwError(iso, "DigestStream write: invalid state")
		}

		h, ok := state.digestStreams[streamID]
		if !ok {
			return throwError(iso, "DigestStream write: unknown stream")
		}

		h.Write(data)

		undef, _ := v8.NewValue(iso, true)
		return undef
	}).GetFunction(ctx))

	// __cryptoDigestStreamFinish(requestID, streamID) -> base64 hash
	_ = ctx.Global().Set("__cryptoDigestStreamFinish", v8.NewFunctionTemplate(iso, func(info *v8.FunctionCallbackInfo) *v8.Value {
		args := info.Args()
		if len(args) < 2 {
			return throwError(iso, errMissingArg("__cryptoDigestStreamFinish", 2).Error())
		}
		reqID, _ := strconv.ParseUint(args[0].String(), 10, 64)
		streamID := args[1].String()

		state := getRequestState(reqID)
		if state == nil || state.digestStreams == nil {
			return throwError(iso, "DigestStream finish: invalid state")
		}

		h, ok := state.digestStreams[streamID]
		if !ok {
			return throwError(iso, "DigestStream finish: unknown stream")
		}

		result := h.Sum(nil)
		delete(state.digestStreams, streamID)

		val, _ := v8.NewValue(iso, base64.StdEncoding.EncodeToString(result))
		return val
	}).GetFunction(ctx))

	// Evaluate the JS wrapper.
	if _, err := ctx.RunScript(digestStreamJS, "crypto_digeststream.js"); err != nil {
		return fmt.Errorf("evaluating crypto_digeststream.js: %w", err)
	}

	return nil
}
