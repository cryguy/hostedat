package worker

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"encoding/base64"
	"fmt"
	"io"

	v8 "github.com/tommie/v8go"
)

// compressionJS implements CompressionStream and DecompressionStream.
// These use Go-backed compress/decompress functions with a buffering
// TransformStream pattern: chunks are collected during transform() and
// compressed/decompressed in bulk during flush().
const compressionJS = `
(function() {

// Helper: convert base64 to Uint8Array (needed for binary stream output).
function __b64ToUint8Array(b64) {
	var _b64e = 'ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/';
	var _b64d = new Uint8Array(128);
	for (var i = 0; i < _b64e.length; i++) _b64d[_b64e.charCodeAt(i)] = i;

	var pad = 0;
	if (b64.length > 0 && b64[b64.length - 1] === '=') pad++;
	if (b64.length > 1 && b64[b64.length - 2] === '=') pad++;
	var outLen = (b64.length * 3 / 4) - pad;
	var out = new Uint8Array(outLen);
	var j = 0;
	for (var i = 0; i < b64.length; i += 4) {
		var a = _b64d[b64.charCodeAt(i)];
		var b = _b64d[b64.charCodeAt(i + 1)];
		var c = _b64d[b64.charCodeAt(i + 2)];
		var d = _b64d[b64.charCodeAt(i + 3)];
		out[j++] = (a << 2) | (b >> 4);
		if (j < outLen) out[j++] = ((b & 15) << 4) | (c >> 2);
		if (j < outLen) out[j++] = ((c & 3) << 6) | d;
	}
	return out;
}

class CompressionStream {
	constructor(format) {
		if (format !== 'gzip' && format !== 'deflate' && format !== 'deflate-raw') {
			throw new TypeError('Unsupported compression format: ' + format);
		}
		var chunks = [];
		var fmt = format;
		var ts = new TransformStream({
			transform(chunk, controller) {
				if (typeof chunk === 'string') {
					chunks.push(new TextEncoder().encode(chunk));
				} else if (chunk instanceof ArrayBuffer) {
					chunks.push(new Uint8Array(chunk));
				} else if (ArrayBuffer.isView(chunk)) {
					chunks.push(new Uint8Array(chunk.buffer, chunk.byteOffset, chunk.byteLength));
				} else {
					chunks.push(new TextEncoder().encode(String(chunk)));
				}
			},
			flush(controller) {
				var totalLen = 0;
				for (var i = 0; i < chunks.length; i++) totalLen += chunks[i].length;
				var combined = new Uint8Array(totalLen);
				var offset = 0;
				for (var i = 0; i < chunks.length; i++) {
					combined.set(chunks[i], offset);
					offset += chunks[i].length;
				}
				var resultB64 = __compress(fmt, __bufferSourceToB64(combined));
				controller.enqueue(__b64ToUint8Array(resultB64));
			}
		});
		this.readable = ts.readable;
		this.writable = ts.writable;
	}
}

class DecompressionStream {
	constructor(format) {
		if (format !== 'gzip' && format !== 'deflate' && format !== 'deflate-raw') {
			throw new TypeError('Unsupported compression format: ' + format);
		}
		var chunks = [];
		var fmt = format;
		var ts = new TransformStream({
			transform(chunk, controller) {
				if (typeof chunk === 'string') {
					chunks.push(new TextEncoder().encode(chunk));
				} else if (chunk instanceof ArrayBuffer) {
					chunks.push(new Uint8Array(chunk));
				} else if (ArrayBuffer.isView(chunk)) {
					chunks.push(new Uint8Array(chunk.buffer, chunk.byteOffset, chunk.byteLength));
				} else {
					chunks.push(new TextEncoder().encode(String(chunk)));
				}
			},
			flush(controller) {
				var totalLen = 0;
				for (var i = 0; i < chunks.length; i++) totalLen += chunks[i].length;
				var combined = new Uint8Array(totalLen);
				var offset = 0;
				for (var i = 0; i < chunks.length; i++) {
					combined.set(chunks[i], offset);
					offset += chunks[i].length;
				}
				var resultB64 = __decompress(fmt, __bufferSourceToB64(combined));
				controller.enqueue(__b64ToUint8Array(resultB64));
			}
		});
		this.readable = ts.readable;
		this.writable = ts.writable;
	}
}

globalThis.CompressionStream = CompressionStream;
globalThis.DecompressionStream = DecompressionStream;

})();
`

// setupCompression registers Go-backed compress/decompress and evaluates the JS classes.
// Must run after setupStreams.
func setupCompression(iso *v8.Isolate, ctx *v8.Context, el *eventLoop) error {
	// __compress(format, dataB64) -> compressedB64
	ctx.Global().Set("__compress", v8.NewFunctionTemplate(iso, func(info *v8.FunctionCallbackInfo) *v8.Value {
		args := info.Args()
		if len(args) < 2 {
			return throwError(iso, "compress requires 2 argument(s)")
		}
		format := args[0].String()
		dataB64 := args[1].String()

		data, err := base64.StdEncoding.DecodeString(dataB64)
		if err != nil {
			return throwError(iso, "compress: invalid base64")
		}

		var buf bytes.Buffer
		switch format {
		case "gzip":
			w := gzip.NewWriter(&buf)
			if _, err := w.Write(data); err != nil {
				return throwError(iso, fmt.Sprintf("compress: %s", err.Error()))
			}
			if err := w.Close(); err != nil {
				return throwError(iso, fmt.Sprintf("compress: %s", err.Error()))
			}
		case "deflate":
			w, err := flate.NewWriter(&buf, flate.DefaultCompression)
			if err != nil {
				return throwError(iso, fmt.Sprintf("compress: %s", err.Error()))
			}
			if _, err := w.Write(data); err != nil {
				return throwError(iso, fmt.Sprintf("compress: %s", err.Error()))
			}
			if err := w.Close(); err != nil {
				return throwError(iso, fmt.Sprintf("compress: %s", err.Error()))
			}
		case "deflate-raw":
			w, err := flate.NewWriter(&buf, flate.DefaultCompression)
			if err != nil {
				return throwError(iso, fmt.Sprintf("compress: %s", err.Error()))
			}
			if _, err := w.Write(data); err != nil {
				return throwError(iso, fmt.Sprintf("compress: %s", err.Error()))
			}
			if err := w.Close(); err != nil {
				return throwError(iso, fmt.Sprintf("compress: %s", err.Error()))
			}
		default:
			return throwError(iso, fmt.Sprintf("compress: unsupported format %q", format))
		}

		val, _ := v8.NewValue(iso, base64.StdEncoding.EncodeToString(buf.Bytes()))
		return val
	}).GetFunction(ctx))

	// __decompress(format, dataB64) -> decompressedB64
	ctx.Global().Set("__decompress", v8.NewFunctionTemplate(iso, func(info *v8.FunctionCallbackInfo) *v8.Value {
		args := info.Args()
		if len(args) < 2 {
			return throwError(iso, "decompress requires 2 argument(s)")
		}
		format := args[0].String()
		dataB64 := args[1].String()

		data, err := base64.StdEncoding.DecodeString(dataB64)
		if err != nil {
			return throwError(iso, "decompress: invalid base64")
		}

		var result []byte
		switch format {
		case "gzip":
			r, err := gzip.NewReader(bytes.NewReader(data))
			if err != nil {
				return throwError(iso, fmt.Sprintf("decompress: %s", err.Error()))
			}
			result, err = io.ReadAll(r)
			if err != nil {
				return throwError(iso, fmt.Sprintf("decompress: %s", err.Error()))
			}
			r.Close()
		case "deflate", "deflate-raw":
			r := flate.NewReader(bytes.NewReader(data))
			result, err = io.ReadAll(r)
			if err != nil {
				return throwError(iso, fmt.Sprintf("decompress: %s", err.Error()))
			}
			r.Close()
		default:
			return throwError(iso, fmt.Sprintf("decompress: unsupported format %q", format))
		}

		val, _ := v8.NewValue(iso, base64.StdEncoding.EncodeToString(result))
		return val
	}).GetFunction(ctx))

	if _, err := ctx.RunScript(compressionJS, "compression.js"); err != nil {
		return fmt.Errorf("evaluating compression.js: %w", err)
	}
	return nil
}
