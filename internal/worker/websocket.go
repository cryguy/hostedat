package worker

import (
	"context"
	"encoding/base64"
	"fmt"
	"log"
	"strconv"
	"time"

	"github.com/coder/websocket"
	v8 "github.com/tommie/v8go"
)

// webSocketJS defines the WebSocket and WebSocketPair classes available to workers.
// This follows the Cloudflare Workers WebSocket API:
//   - WebSocketPair() creates two linked WebSocket objects
//   - server.accept() marks the server socket as ready
//   - server.addEventListener('message'|'close'|'error', handler)
//   - server.send(data) sends data to the client
//   - Response with status 101 and webSocket property triggers upgrade
const webSocketJS = `
(function() {

class WebSocket {
	constructor(url) {
		this._listeners = {};
		this._readyState = 0;
		this._url = url || '';
		this._protocol = '';
		this._extensions = '';
		this._peer = null;
		this.binaryType = 'arraybuffer';
	}

	accept() {
		this._readyState = 1;
	}

	send(data) {
		if (this._readyState !== 1) {
			throw new DOMException('WebSocket is not open', 'InvalidStateError');
		}
		if (typeof data === 'string') {
			__wsSend(data, false);
		} else if (data instanceof ArrayBuffer) {
			__wsSend(__bufferSourceToB64(data), true);
		} else if (ArrayBuffer.isView(data)) {
			__wsSend(__bufferSourceToB64(data), true);
		} else {
			__wsSend(String(data), false);
		}
	}

	close(code, reason) {
		if (this._readyState >= 2) return;
		this._readyState = 2;
		__wsClose(code || 1000, reason || '');
		this._readyState = 3;
	}

	addEventListener(type, handler) {
		if (!this._listeners[type]) this._listeners[type] = [];
		this._listeners[type].push(handler);
	}

	removeEventListener(type, handler) {
		var list = this._listeners[type];
		if (!list) return;
		this._listeners[type] = list.filter(function(h) { return h !== handler; });
	}

	_dispatch(type, event) {
		var prop = 'on' + type;
		if (typeof this[prop] === 'function') {
			this[prop](event);
		}
		var list = this._listeners[type] || [];
		for (var i = 0; i < list.length; i++) {
			list[i](event);
		}
	}

	get readyState() { return this._readyState; }
	get url() { return this._url; }
	get protocol() { return this._protocol; }
	get extensions() { return this._extensions; }
}

WebSocket.CONNECTING = 0;
WebSocket.OPEN = 1;
WebSocket.CLOSING = 2;
WebSocket.CLOSED = 3;

class WebSocketPair {
	constructor() {
		var ws0 = new WebSocket();
		var ws1 = new WebSocket();
		ws0._peer = ws1;
		ws1._peer = ws0;
		this[0] = ws0;
		this[1] = ws1;
	}
}

globalThis.WebSocket = WebSocket;
globalThis.WebSocketPair = WebSocketPair;

})();
`

// setupWebSocket registers the WebSocket/WebSocketPair JS classes and the
// Go-backed __wsSend/__wsClose functions that bridge to the HTTP WebSocket.
func setupWebSocket(iso *v8.Isolate, ctx *v8.Context, el *eventLoop) error {
	// __wsSend(data, isBinary) — sends a message to the HTTP WebSocket client.
	// If isBinary is true, data is base64-encoded and sent as a binary message.
	sendFn := v8.NewFunctionTemplate(iso, func(info *v8.FunctionCallbackInfo) *v8.Value {
		args := info.Args()
		if len(args) < 1 {
			return nil
		}
		data := args[0].String()
		isBinary := len(args) >= 2 && args[1].Boolean()

		reqIDVal, err := info.Context().Global().Get("__requestID")
		if err != nil {
			return nil
		}
		reqID, _ := strconv.ParseUint(reqIDVal.String(), 10, 64)
		state := getRequestState(reqID)
		if state == nil || state.wsConn == nil {
			return nil
		}

		state.wsMu.Lock()
		defer state.wsMu.Unlock()
		if state.wsClosed {
			return nil
		}

		writeCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if isBinary {
			decoded, decErr := base64.StdEncoding.DecodeString(data)
			if decErr != nil {
				log.Printf("worker: ws send base64 decode error: %v", decErr)
				return nil
			}
			if writeErr := state.wsConn.Write(writeCtx, websocket.MessageBinary, decoded); writeErr != nil {
				log.Printf("worker: ws send error: %v", writeErr)
			}
		} else {
			if writeErr := state.wsConn.Write(writeCtx, websocket.MessageText, []byte(data)); writeErr != nil {
				log.Printf("worker: ws send error: %v", writeErr)
			}
		}
		return nil
	})
	if err := ctx.Global().Set("__wsSend", sendFn.GetFunction(ctx)); err != nil {
		return fmt.Errorf("setting __wsSend: %w", err)
	}

	// __wsClose(code, reason) — closes the HTTP WebSocket connection.
	closeFn := v8.NewFunctionTemplate(iso, func(info *v8.FunctionCallbackInfo) *v8.Value {
		args := info.Args()
		code := websocket.StatusNormalClosure
		reason := ""
		if len(args) >= 1 {
			code = websocket.StatusCode(args[0].Int32())
		}
		if len(args) >= 2 {
			reason = args[1].String()
		}

		reqIDVal, err := info.Context().Global().Get("__requestID")
		if err != nil {
			return nil
		}
		reqID, _ := strconv.ParseUint(reqIDVal.String(), 10, 64)
		state := getRequestState(reqID)
		if state == nil || state.wsConn == nil {
			return nil
		}

		state.wsMu.Lock()
		defer state.wsMu.Unlock()
		if !state.wsClosed {
			state.wsClosed = true
			state.wsConn.Close(code, reason)
		}
		return nil
	})
	if err := ctx.Global().Set("__wsClose", closeFn.GetFunction(ctx)); err != nil {
		return fmt.Errorf("setting __wsClose: %w", err)
	}

	// Evaluate the JS class definitions.
	if _, err := ctx.RunScript(webSocketJS, "websocket.js"); err != nil {
		return fmt.Errorf("evaluating websocket.js: %w", err)
	}
	return nil
}

// WebSocketHandler holds references needed for WebSocket bridging after
// the initial fetch handler returns a 101 response with a webSocket.
type WebSocketHandler struct {
	worker  *v8Worker
	pool    *v8Pool
	reqID   uint64
	timeout time.Duration
}

// Bridge starts the WebSocket message bridge between the HTTP connection
// and the V8 worker. This method blocks until the WebSocket connection closes
// or the timeout is reached. The worker is returned to the pool when done.
func (wsh *WebSocketHandler) Bridge(ctx context.Context, httpConn *websocket.Conn) {
	defer func() {
		// Dispatch close event to the server WebSocket.
		wsh.worker.ctx.RunScript(`
			if (globalThis.__ws_active_server) {
				globalThis.__ws_active_server._dispatch('close', {
					code: 1000, reason: '', wasClean: true
				});
				delete globalThis.__ws_active_server;
			}
		`, "ws_cleanup.js")
		wsh.worker.ctx.PerformMicrotaskCheckpoint()

		// Clean up request state and return worker to pool.
		clearRequestState(wsh.reqID)
		wsh.pool.put(wsh.worker)
	}()

	state := getRequestState(wsh.reqID)
	if state == nil {
		return
	}
	state.wsConn = httpConn

	w := wsh.worker
	iso := w.iso
	v8ctx := w.ctx

	// Apply message size limit.
	httpConn.SetReadLimit(maxWSMessageBytes)

	// Reader goroutine: reads from HTTP WebSocket into a channel.
	incoming := make(chan wsMessage, 64)
	go func() {
		defer close(incoming)
		for {
			msgType, data, err := httpConn.Read(ctx)
			if err != nil {
				return
			}
			select {
			case incoming <- wsMessage{typ: msgType, data: data}:
			case <-ctx.Done():
				return
			}
		}
	}()

	// Connection-level timeout.
	connDeadline := time.After(wsh.timeout)

	// Ping ticker to detect dead connections.
	pingTicker := time.NewTicker(30 * time.Second)
	defer pingTicker.Stop()

	// Event pump on the V8 goroutine.
	for {
		select {
		case msg, ok := <-incoming:
			if !ok {
				return // connection closed
			}
			// Dispatch message to the server WebSocket's handlers.
			if msg.typ == websocket.MessageBinary {
				// Binary message: base64-encode and convert to ArrayBuffer in JS.
				b64 := base64.StdEncoding.EncodeToString(msg.data)
				b64Val, _ := v8.NewValue(iso, b64)
				v8ctx.Global().Set("__ws_incoming_data", b64Val)
				v8ctx.RunScript(`
					(function() {
						var b64 = globalThis.__ws_incoming_data;
						delete globalThis.__ws_incoming_data;
						var binary = __b64ToBuffer(b64);
						if (globalThis.__ws_active_server) {
							globalThis.__ws_active_server._dispatch('message', { data: binary });
						}
					})();
				`, "ws_dispatch_binary.js")
			} else {
				// Text message: dispatch as string.
				dataStr := string(msg.data)
				dataVal, _ := v8.NewValue(iso, dataStr)
				v8ctx.Global().Set("__ws_incoming_data", dataVal)
				v8ctx.RunScript(`
					(function() {
						var data = globalThis.__ws_incoming_data;
						delete globalThis.__ws_incoming_data;
						if (globalThis.__ws_active_server) {
							globalThis.__ws_active_server._dispatch('message', { data: data });
						}
					})();
				`, "ws_dispatch.js")
			}
			v8ctx.PerformMicrotaskCheckpoint()

			// Fire any pending timers (for setTimeout in WS handlers).
			if w.eventLoop.hasPending() {
				deadline := time.Now().Add(50 * time.Millisecond)
				w.eventLoop.drain(iso, v8ctx, deadline)
			}

		case <-pingTicker.C:
			pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			err := httpConn.Ping(pingCtx)
			cancel()
			if err != nil {
				return // connection dead, release worker
			}

		case <-connDeadline:
			return

		case <-ctx.Done():
			return
		}
	}
}

type wsMessage struct {
	typ  websocket.MessageType
	data []byte
}
