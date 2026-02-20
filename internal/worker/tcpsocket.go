package worker

import (
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"net"
	"strconv"
	"sync"
	"time"

	v8 "github.com/tommie/v8go"
)

// tcpSocketBuffer provides a thread-safe read buffer for a TCP socket.
type tcpSocketBuffer struct {
	mu      sync.Mutex
	conn    net.Conn
	buf     []byte         // accumulated unread data
	err     error          // sticky read error (io.EOF, etc.)
	done    bool           // true once background reader exits
	hasData chan struct{}   // signaled (non-blocking) when new data arrives or done
}

// readLoop reads from conn into the buffer in the background.
func (b *tcpSocketBuffer) readLoop() {
	tmp := make([]byte, 4096)
	for {
		n, err := b.conn.Read(tmp)
		b.mu.Lock()
		if n > 0 {
			b.buf = append(b.buf, tmp[:n]...)
		}
		if err != nil {
			b.err = err
			b.done = true
			b.mu.Unlock()
			b.signal()
			return
		}
		b.mu.Unlock()
		b.signal()
	}
}

// signal notifies any blocked reader that data (or EOF) is available.
func (b *tcpSocketBuffer) signal() {
	select {
	case b.hasData <- struct{}{}:
	default:
	}
}

// waitForData blocks until new data arrives or the timeout elapses.
func (b *tcpSocketBuffer) waitForData(timeout time.Duration) {
	select {
	case <-b.hasData:
	case <-time.After(timeout):
	}
}

// take returns up to maxBytes from the buffer as base64, or "" if empty.
// Returns (data, eof, error).
func (b *tcpSocketBuffer) take(maxBytes int) (string, bool, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if len(b.buf) == 0 {
		if b.done {
			return "", true, b.err
		}
		return "", false, nil
	}

	n := maxBytes
	if n > len(b.buf) {
		n = len(b.buf)
	}
	data := make([]byte, n)
	copy(data, b.buf[:n])
	b.buf = b.buf[n:]

	// Drain any pending signal so future signals from readLoop aren't blocked
	// by a stale entry in the channel. Without this, a signal sent for chunk N
	// can sit unconsumed, causing signal() for chunk N+1 to be dropped.
	select {
	case <-b.hasData:
	default:
	}

	eof := len(b.buf) == 0 && b.done
	return base64.StdEncoding.EncodeToString(data), eof, nil
}

// tcpSocketJS is the pure JS polyfill for the connect() global and Socket class.
const tcpSocketJS = `
(function() {

globalThis.connect = function(address, options) {
	var requestID = globalThis.__requestID;
	var hostname, port;
	if (typeof address === 'string') {
		var colonIdx = address.lastIndexOf(':');
		if (colonIdx === -1) throw new Error('connect: invalid address format, expected "hostname:port"');
		hostname = address.substring(0, colonIdx);
		port = parseInt(address.substring(colonIdx + 1), 10);
	} else if (address && typeof address === 'object') {
		hostname = address.hostname;
		port = typeof address.port === 'string' ? parseInt(address.port, 10) : address.port;
	} else {
		throw new Error('connect: address must be a string or {hostname, port} object');
	}

	if (!hostname) throw new Error('connect: hostname is required');
	if (isNaN(port) || port < 1 || port > 65535) throw new Error('connect: port must be 1-65535');

	options = options || {};
	var secure = (options.secureTransport === 'on') ? 'on' : 'off';
	var allowHalfOpen = !!options.allowHalfOpen;

	// Synchronous connect call - throws on error (including SSRF).
	var socketID = __tcpConnect(requestID, hostname, String(port), secure);

	var closedResolve, closedReject;
	var closedPromise = new Promise(function(resolve, reject) {
		closedResolve = resolve;
		closedReject = reject;
	});

	var socketClosed = false;

	// Build the opened promise (resolves immediately since connect is sync).
	var openedPromise = Promise.resolve({
		remoteAddress: hostname + ':' + port,
		localAddress: '0.0.0.0:0'
	});

	// Build readable stream backed by __tcpRead.
	var readable = new ReadableStream({
		pull: function(controller) {
			if (socketClosed) {
				controller.close();
				return;
			}
			var result = __tcpRead(requestID, socketID, 4096);
			if (result === '') {
				// No data available yet, return without enqueuing.
				return;
			}
			if (result === null) {
				// EOF
				controller.close();
				if (!socketClosed && !allowHalfOpen) {
					socketClosed = true;
					try { __tcpClose(requestID, socketID); } catch(e) {}
					closedResolve();
				}
				return;
			}
			// result is base64 data
			var raw = atob(result);
			var bytes = new Uint8Array(raw.length);
			for (var i = 0; i < raw.length; i++) {
				bytes[i] = raw.charCodeAt(i);
			}
			controller.enqueue(bytes);
		}
	});

	// Build writable stream backed by __tcpWrite.
	var writable = new WritableStream({
		write: function(chunk) {
			if (socketClosed) throw new Error('Socket is closed');
			var bytes;
			if (typeof chunk === 'string') {
				var enc = new TextEncoder();
				bytes = enc.encode(chunk);
			} else if (chunk instanceof Uint8Array) {
				bytes = chunk;
			} else if (chunk instanceof ArrayBuffer) {
				bytes = new Uint8Array(chunk);
			} else if (ArrayBuffer.isView(chunk)) {
				bytes = new Uint8Array(chunk.buffer, chunk.byteOffset, chunk.byteLength);
			} else {
				throw new Error('write: unsupported chunk type');
			}
			var b64 = btoa(String.fromCharCode.apply(null, bytes));
			__tcpWrite(requestID, socketID, b64);
		},
		close: function() {
			if (!socketClosed) {
				socketClosed = true;
				try { __tcpClose(requestID, socketID); } catch(e) {}
				closedResolve();
			}
		},
		abort: function(reason) {
			if (!socketClosed) {
				socketClosed = true;
				try { __tcpClose(requestID, socketID); } catch(e) {}
				closedReject(reason || new Error('Socket aborted'));
			}
		}
	});

	var socket = {
		readable: readable,
		writable: writable,
		closed: closedPromise,
		opened: openedPromise,
		close: function() {
			if (!socketClosed) {
				socketClosed = true;
				try { __tcpClose(requestID, socketID); } catch(e) {}
				closedResolve();
			}
			return closedPromise;
		},
		startTls: function() {
			if (socketClosed) throw new Error('Socket is closed');
			var newSocketID = __tcpStartTls(requestID, socketID, hostname);
			// Return a new socket-like object for the upgraded connection
			var tlsClosedResolve, tlsClosedReject;
			var tlsClosedPromise = new Promise(function(resolve, reject) {
				tlsClosedResolve = resolve;
				tlsClosedReject = reject;
			});
			var tlsClosed = false;
			var tlsReadable = new ReadableStream({
				pull: function(controller) {
					if (tlsClosed) { controller.close(); return; }
					var result = __tcpRead(requestID, newSocketID, 4096);
					if (result === '') return;
					if (result === null) {
						controller.close();
						if (!tlsClosed) { tlsClosed = true; try { __tcpClose(requestID, newSocketID); } catch(e) {} tlsClosedResolve(); }
						return;
					}
					var raw = atob(result);
					var bytes = new Uint8Array(raw.length);
					for (var i = 0; i < raw.length; i++) bytes[i] = raw.charCodeAt(i);
					controller.enqueue(bytes);
				}
			});
			var tlsWritable = new WritableStream({
				write: function(chunk) {
					if (tlsClosed) throw new Error('Socket is closed');
					var bytes;
					if (typeof chunk === 'string') { bytes = new TextEncoder().encode(chunk); }
					else if (chunk instanceof Uint8Array) { bytes = chunk; }
					else if (chunk instanceof ArrayBuffer) { bytes = new Uint8Array(chunk); }
					else if (ArrayBuffer.isView(chunk)) { bytes = new Uint8Array(chunk.buffer, chunk.byteOffset, chunk.byteLength); }
					else { throw new Error('write: unsupported chunk type'); }
					var b64 = btoa(String.fromCharCode.apply(null, bytes));
					__tcpWrite(requestID, newSocketID, b64);
				},
				close: function() {
					if (!tlsClosed) { tlsClosed = true; try { __tcpClose(requestID, newSocketID); } catch(e) {} tlsClosedResolve(); }
				}
			});
			return {
				readable: tlsReadable,
				writable: tlsWritable,
				closed: tlsClosedPromise,
				opened: Promise.resolve({ remoteAddress: hostname + ':' + port, localAddress: '0.0.0.0:0' }),
				close: function() {
					if (!tlsClosed) { tlsClosed = true; try { __tcpClose(requestID, newSocketID); } catch(e) {} tlsClosedResolve(); }
					return tlsClosedPromise;
				},
				startTls: function() { throw new Error('Already using TLS'); }
			};
		}
	};

	return socket;
};

})();
`

// setupTCPSocket registers Go-backed TCP helpers and evaluates the JS wrapper.
func setupTCPSocket(iso *v8.Isolate, ctx *v8.Context, _ *eventLoop) error {
	// __tcpConnect(requestID, hostname, port, secure) -> socketID string
	_ = ctx.Global().Set("__tcpConnect", v8.NewFunctionTemplate(iso, func(info *v8.FunctionCallbackInfo) *v8.Value {
		args := info.Args()
		if len(args) < 4 {
			return throwError(iso, "tcpConnect: requires 4 arguments")
		}

		reqID, _ := strconv.ParseUint(args[0].String(), 10, 64)
		hostname := args[1].String()
		port := args[2].String()
		secure := args[3].String()

		state := getRequestState(reqID)
		if state == nil {
			return throwError(iso, "tcpConnect: invalid request state")
		}

		// SSRF protection: resolve hostname and check all IPs.
		if err := checkTCPSSRF(hostname); err != nil {
			return throwError(iso, fmt.Sprintf("tcpConnect: %s", err.Error()))
		}

		addr := net.JoinHostPort(hostname, port)
		var conn net.Conn
		var err error

		if secure == "on" {
			conn, err = tls.Dial("tcp", addr, &tls.Config{
				ServerName: hostname,
			})
		} else {
			conn, err = net.Dial("tcp", addr)
		}
		if err != nil {
			return throwError(iso, fmt.Sprintf("tcpConnect: %s", err.Error()))
		}

		// Store connection in request state.
		state.nextTCPSocketID++
		socketID := fmt.Sprintf("tcp_%d", state.nextTCPSocketID)

		if state.tcpSockets == nil {
			state.tcpSockets = make(map[string]net.Conn)
		}
		if state.tcpSocketBuffers == nil {
			state.tcpSocketBuffers = make(map[string]*tcpSocketBuffer)
		}

		state.tcpSockets[socketID] = conn

		buf := &tcpSocketBuffer{conn: conn, hasData: make(chan struct{}, 1)}
		state.tcpSocketBuffers[socketID] = buf
		go buf.readLoop()

		val, _ := v8.NewValue(iso, socketID)
		return val
	}).GetFunction(ctx))

	// __tcpRead(requestID, socketID, maxBytes) -> base64 string, "" if no data, null if EOF
	_ = ctx.Global().Set("__tcpRead", v8.NewFunctionTemplate(iso, func(info *v8.FunctionCallbackInfo) *v8.Value {
		args := info.Args()
		if len(args) < 3 {
			return throwError(iso, "tcpRead: requires 3 arguments")
		}

		reqID, _ := strconv.ParseUint(args[0].String(), 10, 64)
		socketID := args[1].String()
		maxBytes := int(args[2].Int32())

		state := getRequestState(reqID)
		if state == nil {
			return throwError(iso, "tcpRead: invalid request state")
		}

		buf, ok := state.tcpSocketBuffers[socketID]
		if !ok {
			return throwError(iso, "tcpRead: unknown socket ID")
		}

		// Loop until data is available or EOF. Each iteration waits
		// up to 1 second for the readLoop goroutine to deliver data.
		// Bounded to 30 iterations (30s) to prevent blocking forever
		// if no data arrives (the execution watchdog will also fire).
		for attempts := 0; attempts < 30; attempts++ {
			data, eof, _ := buf.take(maxBytes)
			if data != "" {
				val, _ := v8.NewValue(iso, data)
				return val
			}
			if eof {
				result, _ := ctx.RunScript("null", "null.js")
				return result
			}
			buf.waitForData(1 * time.Second)
		}
		// Exhausted retries without data — return empty string
		// so the JS pull() can yield control.
		val, _ := v8.NewValue(iso, "")
		return val
	}).GetFunction(ctx))

	// __tcpWrite(requestID, socketID, b64data) -> writes data to conn
	_ = ctx.Global().Set("__tcpWrite", v8.NewFunctionTemplate(iso, func(info *v8.FunctionCallbackInfo) *v8.Value {
		args := info.Args()
		if len(args) < 3 {
			return throwError(iso, "tcpWrite: requires 3 arguments")
		}

		reqID, _ := strconv.ParseUint(args[0].String(), 10, 64)
		socketID := args[1].String()
		b64data := args[2].String()

		state := getRequestState(reqID)
		if state == nil {
			return throwError(iso, "tcpWrite: invalid request state")
		}

		conn, ok := state.tcpSockets[socketID]
		if !ok {
			return throwError(iso, "tcpWrite: unknown socket ID")
		}

		data, err := base64.StdEncoding.DecodeString(b64data)
		if err != nil {
			return throwError(iso, fmt.Sprintf("tcpWrite: invalid base64: %s", err.Error()))
		}

		if _, err := conn.Write(data); err != nil {
			return throwError(iso, fmt.Sprintf("tcpWrite: %s", err.Error()))
		}

		val, _ := v8.NewValue(iso, true)
		return val
	}).GetFunction(ctx))

	// __tcpClose(requestID, socketID) -> closes the connection
	_ = ctx.Global().Set("__tcpClose", v8.NewFunctionTemplate(iso, func(info *v8.FunctionCallbackInfo) *v8.Value {
		args := info.Args()
		if len(args) < 2 {
			return throwError(iso, "tcpClose: requires 2 arguments")
		}

		reqID, _ := strconv.ParseUint(args[0].String(), 10, 64)
		socketID := args[1].String()

		state := getRequestState(reqID)
		if state == nil {
			return throwError(iso, "tcpClose: invalid request state")
		}

		conn, ok := state.tcpSockets[socketID]
		if !ok {
			return throwError(iso, "tcpClose: unknown socket ID")
		}

		_ = conn.Close()
		delete(state.tcpSockets, socketID)
		delete(state.tcpSocketBuffers, socketID)

		val, _ := v8.NewValue(iso, true)
		return val
	}).GetFunction(ctx))

	// __tcpStartTls(requestID, socketID, hostname) -> new socketID with TLS upgrade
	_ = ctx.Global().Set("__tcpStartTls", v8.NewFunctionTemplate(iso, func(info *v8.FunctionCallbackInfo) *v8.Value {
		args := info.Args()
		if len(args) < 3 {
			return throwError(iso, "tcpStartTls: requires 3 arguments")
		}

		reqID, _ := strconv.ParseUint(args[0].String(), 10, 64)
		socketID := args[1].String()
		hostname := args[2].String()

		state := getRequestState(reqID)
		if state == nil {
			return throwError(iso, "tcpStartTls: invalid request state")
		}

		conn, ok := state.tcpSockets[socketID]
		if !ok {
			return throwError(iso, "tcpStartTls: unknown socket ID")
		}

		// Remove old socket from maps (don't close - we're upgrading it).
		delete(state.tcpSockets, socketID)
		delete(state.tcpSocketBuffers, socketID)

		// Upgrade the raw connection to TLS.
		tlsConn := tls.Client(conn, &tls.Config{
			ServerName: hostname,
		})
		if err := tlsConn.Handshake(); err != nil {
			_ = conn.Close()
			return throwError(iso, fmt.Sprintf("tcpStartTls: TLS handshake failed: %s", err.Error()))
		}

		// Store new TLS connection.
		state.nextTCPSocketID++
		newSocketID := fmt.Sprintf("tcp_%d", state.nextTCPSocketID)
		state.tcpSockets[newSocketID] = tlsConn

		buf := &tcpSocketBuffer{conn: tlsConn, hasData: make(chan struct{}, 1)}
		state.tcpSocketBuffers[newSocketID] = buf
		go buf.readLoop()

		val, _ := v8.NewValue(iso, newSocketID)
		return val
	}).GetFunction(ctx))

	// Evaluate the JS polyfill.
	if _, err := ctx.RunScript(tcpSocketJS, "tcpsocket.js"); err != nil {
		return fmt.Errorf("evaluating tcpsocket.js: %w", err)
	}

	return nil
}

// tcpSSRFEnabled controls SSRF protection for TCP socket connections.
// Tests can set this to false to allow connections to loopback/private IPs.
var tcpSSRFEnabled = true

// checkTCPSSRF resolves the hostname and checks all IPs against private ranges.
func checkTCPSSRF(hostname string) error {
	if !tcpSSRFEnabled {
		return nil
	}
	// Check literal "localhost".
	lower := hostname
	if lower == "localhost" || len(lower) > 10 && lower[len(lower)-10:] == ".localhost" {
		return fmt.Errorf("connections to private addresses are not allowed")
	}

	// Check if hostname is a literal IP.
	if ip := net.ParseIP(hostname); ip != nil {
		if isPrivateIP(ip) {
			return fmt.Errorf("connections to private addresses are not allowed")
		}
		return nil
	}

	// Resolve hostname and check all IPs.
	ips, err := net.LookupHost(hostname)
	if err != nil {
		return fmt.Errorf("DNS lookup failed: %s", err.Error())
	}

	for _, ipStr := range ips {
		ip := net.ParseIP(ipStr)
		if ip != nil && isPrivateIP(ip) {
			return fmt.Errorf("connections to private addresses are not allowed")
		}
	}

	return nil
}

// cleanupTCPSockets closes all TCP sockets for a request state.
// Called during clearRequestState cleanup.
func cleanupTCPSockets(state *requestState) {
	if state.tcpSockets == nil {
		return
	}
	for _, conn := range state.tcpSockets {
		_ = conn.Close()
	}
	state.tcpSockets = nil
	state.tcpSocketBuffers = nil
}
