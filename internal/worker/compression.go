package worker

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"encoding/base64"
	"fmt"
	"io"
	"strconv"
	"sync"

	"github.com/andybalholm/brotli"
	v8 "github.com/tommie/v8go"
)

// compressStreamState holds the Go-side state for one streaming compressor or
// decompressor. For compression the writer writes compressed chunks to buf.
// For decompression an io.Pipe feeds a background goroutine that runs the
// decompressor, producing decompressed output incrementally per chunk.
type compressStreamState struct {
	format string
	mode   string // "compress" or "decompress"

	// Compression state: writer writes compressed data into buf.
	buf    bytes.Buffer
	writer io.WriteCloser

	// Streaming decompression state.
	decompPW   *io.PipeWriter
	decompMu   sync.Mutex
	decompOut  bytes.Buffer
	decompErr  error
	decompDone chan struct{} // closed when goroutine exits
}

// compressionJS implements CompressionStream and DecompressionStream.
// Each chunk is sent to Go-backed functions for real streaming compression.
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

function __chunkToUint8Array(chunk) {
	if (typeof chunk === 'string') {
		return new TextEncoder().encode(chunk);
	} else if (chunk instanceof ArrayBuffer) {
		return new Uint8Array(chunk);
	} else if (ArrayBuffer.isView(chunk)) {
		return new Uint8Array(chunk.buffer, chunk.byteOffset, chunk.byteLength);
	} else {
		return new TextEncoder().encode(String(chunk));
	}
}

class CompressionStream {
	constructor(format) {
		if (format !== 'gzip' && format !== 'deflate' && format !== 'deflate-raw' && format !== 'br') {
			throw new TypeError('Unsupported compression format: ' + format);
		}
		var reqID = globalThis.__requestID;
		var streamID = __compressInit(reqID, format);
		var ts = new TransformStream({
			transform(chunk, controller) {
				var data = __chunkToUint8Array(chunk);
				var resultB64 = __compressChunk(reqID, streamID, __bufferSourceToB64(data));
				if (resultB64.length > 0) {
					controller.enqueue(__b64ToUint8Array(resultB64));
				}
			},
			flush(controller) {
				var resultB64 = __compressFlush(reqID, streamID);
				if (resultB64.length > 0) {
					controller.enqueue(__b64ToUint8Array(resultB64));
				}
			}
		});
		this.readable = ts.readable;
		this.writable = ts.writable;
	}
}

class DecompressionStream {
	constructor(format) {
		if (format !== 'gzip' && format !== 'deflate' && format !== 'deflate-raw' && format !== 'br') {
			throw new TypeError('Unsupported compression format: ' + format);
		}
		var reqID = globalThis.__requestID;
		var streamID = __decompressInit(reqID, format);
		var ts = new TransformStream({
			transform(chunk, controller) {
				var data = __chunkToUint8Array(chunk);
				var resultB64 = __decompressChunk(reqID, streamID, __bufferSourceToB64(data));
				if (resultB64.length > 0) {
					controller.enqueue(__b64ToUint8Array(resultB64));
				}
			},
			flush(controller) {
				var resultB64 = __decompressFlush(reqID, streamID);
				if (resultB64.length > 0) {
					controller.enqueue(__b64ToUint8Array(resultB64));
				}
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

// newCompressWriter creates a compression writer for the given format.
func newCompressWriter(buf *bytes.Buffer, format string) (io.WriteCloser, error) {
	switch format {
	case "gzip":
		return gzip.NewWriter(buf), nil
	case "deflate", "deflate-raw":
		return flate.NewWriter(buf, flate.DefaultCompression)
	case "br":
		return brotli.NewWriter(buf), nil
	default:
		return nil, fmt.Errorf("unsupported format %q", format)
	}
}

// setupCompression registers Go-backed streaming compress/decompress functions
// and evaluates the JS classes. Must run after setupStreams and setupCrypto.
func setupCompression(iso *v8.Isolate, ctx *v8.Context, _ *eventLoop) error {

	// --- Legacy bulk functions (kept for backward compat with direct callers) ---

	// __compress(format, dataB64) -> compressedB64
	_ = ctx.Global().Set("__compress", v8.NewFunctionTemplate(iso, func(info *v8.FunctionCallbackInfo) *v8.Value {
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
		w, err := newCompressWriter(&buf, format)
		if err != nil {
			return throwError(iso, fmt.Sprintf("compress: %s", err.Error()))
		}
		if _, err := w.Write(data); err != nil {
			return throwError(iso, fmt.Sprintf("compress: %s", err.Error()))
		}
		if err := w.Close(); err != nil {
			return throwError(iso, fmt.Sprintf("compress: %s", err.Error()))
		}

		val, _ := v8.NewValue(iso, base64.StdEncoding.EncodeToString(buf.Bytes()))
		return val
	}).GetFunction(ctx))

	// __decompress(format, dataB64) -> decompressedB64
	_ = ctx.Global().Set("__decompress", v8.NewFunctionTemplate(iso, func(info *v8.FunctionCallbackInfo) *v8.Value {
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
			_ = r.Close()
		case "deflate", "deflate-raw":
			r := flate.NewReader(bytes.NewReader(data))
			result, err = io.ReadAll(r)
			if err != nil {
				return throwError(iso, fmt.Sprintf("decompress: %s", err.Error()))
			}
			_ = r.Close()
		case "br":
			r := brotli.NewReader(bytes.NewReader(data))
			result, err = io.ReadAll(r)
			if err != nil {
				return throwError(iso, fmt.Sprintf("decompress: %s", err.Error()))
			}
		default:
			return throwError(iso, fmt.Sprintf("decompress: unsupported format %q", format))
		}

		val, _ := v8.NewValue(iso, base64.StdEncoding.EncodeToString(result))
		return val
	}).GetFunction(ctx))

	// --- Streaming compression functions ---

	// __compressInit(requestID, format) -> streamID
	_ = ctx.Global().Set("__compressInit", v8.NewFunctionTemplate(iso, func(info *v8.FunctionCallbackInfo) *v8.Value {
		args := info.Args()
		if len(args) < 2 {
			return throwError(iso, errMissingArg("__compressInit", 2).Error())
		}
		reqID, _ := strconv.ParseUint(args[0].String(), 10, 64)
		format := args[1].String()

		state := getRequestState(reqID)
		if state == nil {
			return throwError(iso, "compressInit: invalid request state")
		}

		cs := &compressStreamState{format: format, mode: "compress"}
		w, err := newCompressWriter(&cs.buf, format)
		if err != nil {
			return throwError(iso, fmt.Sprintf("compressInit: %s", err.Error()))
		}
		cs.writer = w

		if state.compressStreams == nil {
			state.compressStreams = make(map[string]*compressStreamState)
		}
		state.nextCompressID++
		streamID := strconv.FormatInt(state.nextCompressID, 10)
		state.compressStreams[streamID] = cs

		val, _ := v8.NewValue(iso, streamID)
		return val
	}).GetFunction(ctx))

	// __compressChunk(requestID, streamID, base64data) -> base64 compressed output
	_ = ctx.Global().Set("__compressChunk", v8.NewFunctionTemplate(iso, func(info *v8.FunctionCallbackInfo) *v8.Value {
		args := info.Args()
		if len(args) < 3 {
			return throwError(iso, errMissingArg("__compressChunk", 3).Error())
		}
		reqID, _ := strconv.ParseUint(args[0].String(), 10, 64)
		streamID := args[1].String()
		dataB64 := args[2].String()

		data, err := base64.StdEncoding.DecodeString(dataB64)
		if err != nil {
			return throwError(iso, "compressChunk: invalid base64")
		}

		state := getRequestState(reqID)
		if state == nil || state.compressStreams == nil {
			return throwError(iso, "compressChunk: invalid state")
		}
		cs, ok := state.compressStreams[streamID]
		if !ok {
			return throwError(iso, "compressChunk: unknown stream")
		}

		// Reset buf to capture only this chunk's compressed output.
		cs.buf.Reset()
		if _, err := cs.writer.Write(data); err != nil {
			return throwError(iso, fmt.Sprintf("compressChunk: %s", err.Error()))
		}

		val, _ := v8.NewValue(iso, base64.StdEncoding.EncodeToString(cs.buf.Bytes()))
		return val
	}).GetFunction(ctx))

	// __compressFlush(requestID, streamID) -> base64 remaining compressed data
	_ = ctx.Global().Set("__compressFlush", v8.NewFunctionTemplate(iso, func(info *v8.FunctionCallbackInfo) *v8.Value {
		args := info.Args()
		if len(args) < 2 {
			return throwError(iso, errMissingArg("__compressFlush", 2).Error())
		}
		reqID, _ := strconv.ParseUint(args[0].String(), 10, 64)
		streamID := args[1].String()

		state := getRequestState(reqID)
		if state == nil || state.compressStreams == nil {
			return throwError(iso, "compressFlush: invalid state")
		}
		cs, ok := state.compressStreams[streamID]
		if !ok {
			return throwError(iso, "compressFlush: unknown stream")
		}

		// Reset buf, then close the writer to flush final compressed data.
		cs.buf.Reset()
		if err := cs.writer.Close(); err != nil {
			return throwError(iso, fmt.Sprintf("compressFlush: %s", err.Error()))
		}
		delete(state.compressStreams, streamID)

		val, _ := v8.NewValue(iso, base64.StdEncoding.EncodeToString(cs.buf.Bytes()))
		return val
	}).GetFunction(ctx))

	// --- Streaming decompression functions ---

	// __decompressInit(requestID, format) -> streamID
	_ = ctx.Global().Set("__decompressInit", v8.NewFunctionTemplate(iso, func(info *v8.FunctionCallbackInfo) *v8.Value {
		args := info.Args()
		if len(args) < 2 {
			return throwError(iso, errMissingArg("__decompressInit", 2).Error())
		}
		reqID, _ := strconv.ParseUint(args[0].String(), 10, 64)
		format := args[1].String()

		state := getRequestState(reqID)
		if state == nil {
			return throwError(iso, "decompressInit: invalid request state")
		}

		pr, pw := io.Pipe()
		cs := &compressStreamState{
			format:     format,
			mode:       "decompress",
			decompPW:   pw,
			decompDone: make(chan struct{}),
		}

		// Spawn goroutine that reads from the pipe through a decompressor
		// and accumulates decompressed output in cs.decompOut.
		go func() {
			defer close(cs.decompDone)
			defer func() { _ = pr.Close() }()

			var reader io.ReadCloser
			switch format {
			case "gzip":
				r, err := gzip.NewReader(pr)
				if err != nil {
					cs.decompMu.Lock()
					cs.decompErr = err
					cs.decompMu.Unlock()
					return
				}
				reader = r
			case "deflate", "deflate-raw":
				reader = flate.NewReader(pr)
			case "br":
				reader = io.NopCloser(brotli.NewReader(pr))
			default:
				cs.decompMu.Lock()
				cs.decompErr = fmt.Errorf("unsupported format %q", format)
				cs.decompMu.Unlock()
				return
			}
			defer func() { _ = reader.Close() }()

			buf := make([]byte, 32*1024)
			for {
				n, err := reader.Read(buf)
				if n > 0 {
					cs.decompMu.Lock()
					cs.decompOut.Write(buf[:n])
					cs.decompMu.Unlock()
				}
				if err != nil {
					if err != io.EOF {
						cs.decompMu.Lock()
						cs.decompErr = err
						cs.decompMu.Unlock()
					}
					return
				}
			}
		}()

		if state.compressStreams == nil {
			state.compressStreams = make(map[string]*compressStreamState)
		}
		state.nextCompressID++
		streamID := strconv.FormatInt(state.nextCompressID, 10)
		state.compressStreams[streamID] = cs

		val, _ := v8.NewValue(iso, streamID)
		return val
	}).GetFunction(ctx))

	// __decompressChunk(requestID, streamID, base64data) -> base64 decompressed output
	// Feeds compressed data to the decompressor goroutine via io.Pipe and
	// returns any decompressed output that is already available.
	_ = ctx.Global().Set("__decompressChunk", v8.NewFunctionTemplate(iso, func(info *v8.FunctionCallbackInfo) *v8.Value {
		args := info.Args()
		if len(args) < 3 {
			return throwError(iso, errMissingArg("__decompressChunk", 3).Error())
		}
		reqID, _ := strconv.ParseUint(args[0].String(), 10, 64)
		streamID := args[1].String()
		dataB64 := args[2].String()

		data, err := base64.StdEncoding.DecodeString(dataB64)
		if err != nil {
			return throwError(iso, "decompressChunk: invalid base64")
		}

		state := getRequestState(reqID)
		if state == nil || state.compressStreams == nil {
			return throwError(iso, "decompressChunk: invalid state")
		}
		cs, ok := state.compressStreams[streamID]
		if !ok {
			return throwError(iso, "decompressChunk: unknown stream")
		}

		// Feed compressed data to the decompressor goroutine. Write in a
		// sub-goroutine because PipeWriter.Write blocks until the reader
		// consumes the data.
		errCh := make(chan error, 1)
		go func() {
			_, werr := cs.decompPW.Write(data)
			errCh <- werr
		}()

		if werr := <-errCh; werr != nil {
			return throwError(iso, fmt.Sprintf("decompressChunk: %s", werr.Error()))
		}

		// Collect any decompressed output the goroutine has produced so far.
		cs.decompMu.Lock()
		out := make([]byte, cs.decompOut.Len())
		copy(out, cs.decompOut.Bytes())
		cs.decompOut.Reset()
		derr := cs.decompErr
		cs.decompMu.Unlock()

		if derr != nil {
			return throwError(iso, fmt.Sprintf("decompressChunk: %s", derr.Error()))
		}

		val, _ := v8.NewValue(iso, base64.StdEncoding.EncodeToString(out))
		return val
	}).GetFunction(ctx))

	// __decompressFlush(requestID, streamID) -> base64 remaining decompressed data
	_ = ctx.Global().Set("__decompressFlush", v8.NewFunctionTemplate(iso, func(info *v8.FunctionCallbackInfo) *v8.Value {
		args := info.Args()
		if len(args) < 2 {
			return throwError(iso, errMissingArg("__decompressFlush", 2).Error())
		}
		reqID, _ := strconv.ParseUint(args[0].String(), 10, 64)
		streamID := args[1].String()

		state := getRequestState(reqID)
		if state == nil || state.compressStreams == nil {
			return throwError(iso, "decompressFlush: invalid state")
		}
		cs, ok := state.compressStreams[streamID]
		if !ok {
			return throwError(iso, "decompressFlush: unknown stream")
		}

		// Close the pipe writer to signal EOF to the decompressor goroutine,
		// then wait for it to finish processing all remaining data.
		_ = cs.decompPW.Close()
		<-cs.decompDone

		cs.decompMu.Lock()
		result := make([]byte, cs.decompOut.Len())
		copy(result, cs.decompOut.Bytes())
		cs.decompOut.Reset()
		derr := cs.decompErr
		cs.decompMu.Unlock()

		delete(state.compressStreams, streamID)

		if derr != nil {
			return throwError(iso, fmt.Sprintf("decompressFlush: %s", derr.Error()))
		}

		val, _ := v8.NewValue(iso, base64.StdEncoding.EncodeToString(result))
		return val
	}).GetFunction(ctx))

	if _, err := ctx.RunScript(compressionJS, "compression.js"); err != nil {
		return fmt.Errorf("evaluating compression.js: %w", err)
	}
	return nil
}
