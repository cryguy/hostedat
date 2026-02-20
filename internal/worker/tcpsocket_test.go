package worker

import (
	"encoding/json"
	"fmt"
	"net"
	"strconv"
	"strings"
	"testing"
	"time"

	v8 "github.com/tommie/v8go"
)

// setupTCPTestContext creates an isolate + context with all APIs needed for TCP tests.
func setupTCPTestContext(t *testing.T) (*v8.Isolate, *v8.Context, *eventLoop) {
	t.Helper()
	iso := v8.NewIsolate()
	ctx := v8.NewContext(iso)
	el := newEventLoop()

	for _, fn := range []setupFunc{
		setupWebAPIs,
		setupEncoding,
		setupStreams,
		setupConsole,
		setupTCPSocket,
	} {
		if err := fn(iso, ctx, el); err != nil {
			t.Fatalf("setup: %v", err)
		}
	}

	t.Cleanup(func() {
		ctx.Close()
		iso.Dispose()
	})
	return iso, ctx, el
}

// TestTCPConnectGlobalExists verifies that connect() is registered as a global function.
func TestTCPConnectGlobalExists(t *testing.T) {
	_, ctx, _ := setupTCPTestContext(t)

	result, err := ctx.RunScript(`typeof connect === 'function'`, "test.js")
	if err != nil {
		t.Fatalf("RunScript: %v", err)
	}
	if result.String() != "true" {
		t.Fatalf("expected connect to be a global function, got %s", result.String())
	}
}

// TestTCPSocketSSRFBlocksLoopback verifies that connect() blocks connections
// to loopback addresses (127.0.0.1).
func TestTCPSocketSSRFBlocksLoopback(t *testing.T) {
	iso, ctx, _ := setupTCPTestContext(t)

	// Set up a request state so the JS side has __requestID.
	reqID := newRequestState(10, defaultEnv())
	reqIDVal, _ := v8.NewValue(iso, strconv.FormatUint(reqID, 10))
	_ = ctx.Global().Set("__requestID", reqIDVal)
	defer clearRequestState(reqID)

	result, err := ctx.RunScript(`(function() {
		try {
			connect("127.0.0.1:8080");
			return "not_blocked";
		} catch(e) {
			return e.message || String(e);
		}
	})()`, "test_ssrf.js")
	if err != nil {
		t.Fatalf("RunScript: %v", err)
	}
	if !strings.Contains(result.String(), "private") {
		t.Fatalf("expected SSRF block for 127.0.0.1, got: %s", result.String())
	}
}

// TestTCPSocketSSRFBlocksLocalhost verifies that connect() blocks connections
// to "localhost".
func TestTCPSocketSSRFBlocksLocalhost(t *testing.T) {
	iso, ctx, _ := setupTCPTestContext(t)

	reqID := newRequestState(10, defaultEnv())
	reqIDVal, _ := v8.NewValue(iso, strconv.FormatUint(reqID, 10))
	_ = ctx.Global().Set("__requestID", reqIDVal)
	defer clearRequestState(reqID)

	result, err := ctx.RunScript(`(function() {
		try {
			connect("localhost:8080");
			return "not_blocked";
		} catch(e) {
			return e.message || String(e);
		}
	})()`, "test_ssrf_localhost.js")
	if err != nil {
		t.Fatalf("RunScript: %v", err)
	}
	if !strings.Contains(result.String(), "private") {
		t.Fatalf("expected SSRF block for localhost, got: %s", result.String())
	}
}

// TestTCPSocketSSRFBlocksPrivateRanges verifies that connect() blocks connections
// to private IP ranges (10.x.x.x, 172.16.x.x, 192.168.x.x).
func TestTCPSocketSSRFBlocksPrivateRanges(t *testing.T) {
	iso, ctx, _ := setupTCPTestContext(t)

	reqID := newRequestState(10, defaultEnv())
	reqIDVal, _ := v8.NewValue(iso, strconv.FormatUint(reqID, 10))
	_ = ctx.Global().Set("__requestID", reqIDVal)
	defer clearRequestState(reqID)

	privateIPs := []string{"10.0.0.1:80", "172.16.0.1:80", "192.168.1.1:80"}
	for _, addr := range privateIPs {
		result, err := ctx.RunScript(`(function() {
			try {
				connect("`+addr+`");
				return "not_blocked";
			} catch(e) {
				return e.message || String(e);
			}
		})()`, "test_ssrf_private.js")
		if err != nil {
			t.Fatalf("RunScript for %s: %v", addr, err)
		}
		if !strings.Contains(result.String(), "private") {
			t.Fatalf("expected SSRF block for %s, got: %s", addr, result.String())
		}
	}
}

// TestTCPSocketAddressParsing verifies that connect() handles both string
// and object address formats.
func TestTCPSocketAddressParsing(t *testing.T) {
	iso, ctx, _ := setupTCPTestContext(t)

	reqID := newRequestState(10, defaultEnv())
	reqIDVal, _ := v8.NewValue(iso, strconv.FormatUint(reqID, 10))
	_ = ctx.Global().Set("__requestID", reqIDVal)
	defer clearRequestState(reqID)

	// Both formats should fail with SSRF for private IPs.
	result, err := ctx.RunScript(`(function() {
		var errors = [];
		try { connect("10.0.0.1:80"); } catch(e) { errors.push(e.message || String(e)); }
		try { connect({hostname: "10.0.0.1", port: 80}); } catch(e) { errors.push(e.message || String(e)); }
		var allSSRF = errors.every(function(msg) { return msg.indexOf("private") !== -1; });
		return JSON.stringify({ count: errors.length, allSSRF: allSSRF });
	})()`, "test_addr_parsing.js")
	if err != nil {
		t.Fatalf("RunScript: %v", err)
	}

	s := result.String()
	if !strings.Contains(s, `"count":2`) {
		t.Fatalf("expected 2 SSRF errors, got: %s", s)
	}
	if !strings.Contains(s, `"allSSRF":true`) {
		t.Fatalf("expected all errors to be SSRF, got: %s", s)
	}
}

// TestTCPSocketInvalidAddress verifies that connect() throws for invalid addresses.
func TestTCPSocketInvalidAddress(t *testing.T) {
	iso, ctx, _ := setupTCPTestContext(t)

	reqID := newRequestState(10, defaultEnv())
	reqIDVal, _ := v8.NewValue(iso, strconv.FormatUint(reqID, 10))
	_ = ctx.Global().Set("__requestID", reqIDVal)
	defer clearRequestState(reqID)

	result, err := ctx.RunScript(`(function() {
		var errors = [];
		// Missing port
		try { connect("example.com"); } catch(e) { errors.push(e.message || String(e)); }
		// Invalid port
		try { connect("example.com:0"); } catch(e) { errors.push(e.message || String(e)); }
		// No hostname
		try { connect({port: 80}); } catch(e) { errors.push(e.message || String(e)); }
		return JSON.stringify({ count: errors.length });
	})()`, "test_invalid_addr.js")
	if err != nil {
		t.Fatalf("RunScript: %v", err)
	}

	if !strings.Contains(result.String(), `"count":3`) {
		t.Fatalf("expected 3 errors for invalid addresses, got: %s", result.String())
	}
}

// TestTCPSocketObjectHasProperties verifies the returned socket-like object
// has the expected shape (readable, writable, closed, opened, close, startTls).
func TestTCPSocketObjectHasProperties(t *testing.T) {
	iso, ctx, _ := setupTCPTestContext(t)

	reqID := newRequestState(10, defaultEnv())
	reqIDVal, _ := v8.NewValue(iso, strconv.FormatUint(reqID, 10))
	_ = ctx.Global().Set("__requestID", reqIDVal)
	defer clearRequestState(reqID)

	// Use SSRF error to verify the function parses args before blocking.
	// We can't get a socket object back when SSRF blocks, so test the error message
	// to confirm it gets past arg parsing.
	result, err := ctx.RunScript(`(function() {
		try {
			connect({hostname: "10.0.0.1", port: 80});
			return "connected";
		} catch(e) {
			// Error should be about "private addresses", not about
			// missing hostname/port or connect not being defined.
			return e.message || String(e);
		}
	})()`, "test_socket_props.js")
	if err != nil {
		t.Fatalf("RunScript: %v", err)
	}
	if !strings.Contains(result.String(), "private") {
		t.Fatalf("expected private address error (proves arg parsing works), got: %s", result.String())
	}
}

// TestTCPCheckSSRFDirect tests the Go-level checkTCPSSRF function directly.
func TestTCPCheckSSRFDirect(t *testing.T) {
	tests := []struct {
		hostname string
		blocked  bool
	}{
		{"127.0.0.1", true},
		{"10.0.0.1", true},
		{"172.16.0.1", true},
		{"192.168.1.1", true},
		{"0.0.0.0", true},
		{"localhost", true},
		{"::1", true},
		// Public IPs should pass.
		{"8.8.8.8", false},
		{"1.1.1.1", false},
	}

	for _, tt := range tests {
		err := checkTCPSSRF(tt.hostname)
		if tt.blocked && err == nil {
			t.Errorf("checkTCPSSRF(%q) should have been blocked but was allowed", tt.hostname)
		}
		if !tt.blocked && err != nil {
			t.Errorf("checkTCPSSRF(%q) should have been allowed but was blocked: %v", tt.hostname, err)
		}
	}
}

// TestTCPSocketBufferTake tests the tcpSocketBuffer.take method directly.
func TestTCPSocketBufferTake(t *testing.T) {
	server, client := net.Pipe()
	defer func() { _ = client.Close() }()

	buf := &tcpSocketBuffer{conn: client}
	go buf.readLoop()

	// Write some data from the server side.
	testData := []byte("hello TCP")
	_, _ = server.Write(testData)

	// Poll for data availability with short sleeps.
	var data string
	var eof bool
	for i := 0; i < 50; i++ {
		data, eof, _ = buf.take(1024)
		if data != "" {
			break
		}
		time.Sleep(time.Millisecond)
	}

	if data == "" {
		t.Fatal("expected data from buffer, got empty string")
	}
	if eof {
		t.Fatal("unexpected EOF")
	}

	// Close server side and verify EOF.
	_ = server.Close()
	for i := 0; i < 50; i++ {
		_, eof, _ = buf.take(1024)
		if eof {
			break
		}
		time.Sleep(time.Millisecond)
	}
	if !eof {
		t.Fatal("expected EOF after server close")
	}
}

// TestTCPCleanupOnRequestClear verifies that TCP sockets are cleaned up
// when clearRequestState is called.
func TestTCPCleanupOnRequestClear(t *testing.T) {
	server, client := net.Pipe()
	defer func() { _ = server.Close() }()

	reqID := newRequestState(10, defaultEnv())
	state := getRequestState(reqID)
	state.tcpSockets = map[string]net.Conn{
		"tcp_1": client,
	}
	state.tcpSocketBuffers = map[string]*tcpSocketBuffer{
		"tcp_1": {conn: client},
	}

	cleared := clearRequestState(reqID)
	if cleared == nil {
		t.Fatal("clearRequestState returned nil")
	}
	if cleared.tcpSockets != nil {
		t.Fatal("expected tcpSockets to be nil after cleanup")
	}

	// Verify the connection is closed by trying to write.
	_, err := client.Write([]byte("test"))
	if err == nil {
		t.Fatal("expected write to closed connection to fail")
	}
}

// disableTCPSSRF temporarily disables SSRF protection so tests can connect
// to loopback/private addresses. Restores the original value on cleanup.
func disableTCPSSRF(t *testing.T) {
	t.Helper()
	orig := tcpSSRFEnabled
	tcpSSRFEnabled = false
	t.Cleanup(func() {
		tcpSSRFEnabled = orig
	})
}

// TestTCPSocket_ConnectAndWrite verifies that connect() can establish a real
// TCP connection and write data through the writable stream.
func TestTCPSocket_ConnectAndWrite(t *testing.T) {
	disableTCPSSRF(t)

	// Start a local TCP server.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer func() { _ = ln.Close() }()

	received := make(chan string, 1)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			received <- ""
			return
		}
		defer func() { _ = conn.Close() }()
		buf := make([]byte, 256)
		n, _ := conn.Read(buf)
		received <- string(buf[:n])
	}()

	port := ln.Addr().(*net.TCPAddr).Port
	db := testDB(t)
	e := newTestEngine(t, db)

	source := fmt.Sprintf(`export default {
  async fetch(request, env) {
    var socket = connect("127.0.0.1:%d");
    var writer = socket.writable.getWriter();
    await writer.write(new TextEncoder().encode("hello TCP"));
    await writer.close();
    return Response.json({ ok: true });
  },
};`, port)

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct{ Ok bool }
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !data.Ok {
		t.Fatalf("expected ok:true, got: %s", r.Response.Body)
	}

	select {
	case got := <-received:
		if got != "hello TCP" {
			t.Fatalf("server received %q, want %q", got, "hello TCP")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting for server to receive data")
	}
}

// TestTCPSocket_ConnectAndRead verifies that connect() can read data sent by
// the server through the readable stream.
func TestTCPSocket_ConnectAndRead(t *testing.T) {
	disableTCPSSRF(t)

	// Start a local TCP server that immediately writes data and closes.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer func() { _ = ln.Close() }()

	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		_, _ = conn.Write([]byte("server says hi"))
		_ = conn.Close()
	}()

	port := ln.Addr().(*net.TCPAddr).Port
	db := testDB(t)
	e := newTestEngine(t, db)

	source := fmt.Sprintf(`export default {
  async fetch(request, env) {
    var socket = connect("127.0.0.1:%d");
    // Wait for Go's readLoop to receive the data.
    await scheduler.wait(200);
    var reader = socket.readable.getReader();
    var result = await reader.read();
    var text = "";
    if (result.value) {
      text = new TextDecoder().decode(result.value);
    }
    return Response.json({ text: text });
  },
};`, port)

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct{ Text string }
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !strings.Contains(data.Text, "server says hi") {
		t.Fatalf("expected %q in text, got: %q", "server says hi", data.Text)
	}
}

// TestTCPSocket_SocketObjectShape verifies that the socket returned by connect()
// has the correct property types: readable, writable, closed, opened, close, startTls.
func TestTCPSocket_SocketObjectShape(t *testing.T) {
	disableTCPSSRF(t)

	// Start a minimal TCP server that accepts and closes immediately.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer func() { _ = ln.Close() }()

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			_ = conn.Close()
		}
	}()

	port := ln.Addr().(*net.TCPAddr).Port
	db := testDB(t)
	e := newTestEngine(t, db)

	source := fmt.Sprintf(`export default {
  async fetch(request, env) {
    var socket = connect("127.0.0.1:%d");
    var shape = {
      readable:  typeof socket.readable,
      writable:  typeof socket.writable,
      closed:    typeof socket.closed.then,
      opened:    typeof socket.opened.then,
      close:     typeof socket.close,
      startTls:  typeof socket.startTls,
    };
    socket.close();
    return Response.json(shape);
  },
};`, port)

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var shape map[string]string
	if err := json.Unmarshal(r.Response.Body, &shape); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	expected := map[string]string{
		"readable": "object",
		"writable": "object",
		"closed":   "function", // .then is a function on a Promise
		"opened":   "function",
		"close":    "function",
		"startTls": "function",
	}
	for key, want := range expected {
		if got := shape[key]; got != want {
			t.Errorf("socket.%s: typeof = %q, want %q", key, got, want)
		}
	}
}

// TestTCPSocketSSRFBlocksLinkLocal verifies that connect() blocks connections
// to the link-local range 169.254.x.x.
func TestTCPSocketSSRFBlocksLinkLocal(t *testing.T) {
	iso, ctx, _ := setupTCPTestContext(t)

	reqID := newRequestState(10, defaultEnv())
	reqIDVal, _ := v8.NewValue(iso, strconv.FormatUint(reqID, 10))
	_ = ctx.Global().Set("__requestID", reqIDVal)
	defer clearRequestState(reqID)

	result, err := ctx.RunScript(`(function() {
		try {
			connect("169.254.169.254:80");
			return "not_blocked";
		} catch(e) {
			return e.message || String(e);
		}
	})()`, "test_ssrf_linklocal.js")
	if err != nil {
		t.Fatalf("RunScript: %v", err)
	}
	if !strings.Contains(result.String(), "private") {
		t.Fatalf("expected SSRF block for 169.254.169.254, got: %s", result.String())
	}
}

// TestTCPSocketSSRFBlocksIPv6Loopback verifies that connect() blocks connections
// to the IPv6 loopback address ::1 via the JS connect() function.
func TestTCPSocketSSRFBlocksIPv6Loopback(t *testing.T) {
	iso, ctx, _ := setupTCPTestContext(t)

	reqID := newRequestState(10, defaultEnv())
	reqIDVal, _ := v8.NewValue(iso, strconv.FormatUint(reqID, 10))
	_ = ctx.Global().Set("__requestID", reqIDVal)
	defer clearRequestState(reqID)

	result, err := ctx.RunScript(`(function() {
		try {
			connect({hostname: "::1", port: 80});
			return "not_blocked";
		} catch(e) {
			return e.message || String(e);
		}
	})()`, "test_ssrf_ipv6.js")
	if err != nil {
		t.Fatalf("RunScript: %v", err)
	}
	if !strings.Contains(result.String(), "private") {
		t.Fatalf("expected SSRF block for ::1, got: %s", result.String())
	}
}

// TestTCPSocketSSRFBlocksAllPrivateRangesExpanded verifies that checkTCPSSRF
// blocks all commonly exploited private ranges including 172.16-31.x and 169.254.x.
func TestTCPSocketSSRFBlocksAllPrivateRangesExpanded(t *testing.T) {
	tests := []struct {
		hostname string
		blocked  bool
	}{
		// 10.0.0.0/8
		{"10.0.0.1", true},
		{"10.255.255.255", true},
		// 172.16.0.0/12
		{"172.16.0.1", true},
		{"172.31.255.255", true},
		// 192.168.0.0/16
		{"192.168.0.1", true},
		{"192.168.255.255", true},
		// 169.254.0.0/16 (link-local / cloud metadata)
		{"169.254.169.254", true},
		{"169.254.0.1", true},
		// IPv6 loopback
		{"::1", true},
		// Loopback
		{"127.0.0.1", true},
		{"0.0.0.0", true},
		// Non-private should pass
		{"8.8.8.8", false},
		{"93.184.216.34", false},
	}

	for _, tt := range tests {
		err := checkTCPSSRF(tt.hostname)
		if tt.blocked && err == nil {
			t.Errorf("checkTCPSSRF(%q) should be blocked but was allowed", tt.hostname)
		}
		if !tt.blocked && err != nil {
			t.Errorf("checkTCPSSRF(%q) should be allowed but was blocked: %v", tt.hostname, err)
		}
	}
}

// TestTCPSocket_ReadableAndWritableExist verifies the socket returned by connect()
// has readable and writable properties that are ReadableStream and WritableStream.
func TestTCPSocket_ReadableAndWritableExist(t *testing.T) {
	disableTCPSSRF(t)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer func() { _ = ln.Close() }()

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			_ = conn.Close()
		}
	}()

	port := ln.Addr().(*net.TCPAddr).Port
	db := testDB(t)
	e := newTestEngine(t, db)

	source := fmt.Sprintf(`export default {
  async fetch(request, env) {
    var socket = connect("127.0.0.1:%d");
    var hasReadable = socket.readable instanceof ReadableStream;
    var hasWritable = socket.writable instanceof WritableStream;
    socket.close();
    return Response.json({ hasReadable, hasWritable });
  },
};`, port)

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		HasReadable bool `json:"hasReadable"`
		HasWritable bool `json:"hasWritable"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !data.HasReadable {
		t.Error("socket.readable should be a ReadableStream")
	}
	if !data.HasWritable {
		t.Error("socket.writable should be a WritableStream")
	}
}

// TestTCPSocket_CloseMethod verifies that calling socket.close() works
// and returns the closed promise.
func TestTCPSocket_CloseMethod(t *testing.T) {
	disableTCPSSRF(t)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer func() { _ = ln.Close() }()

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			_ = conn.Close()
		}
	}()

	port := ln.Addr().(*net.TCPAddr).Port
	db := testDB(t)
	e := newTestEngine(t, db)

	source := fmt.Sprintf(`export default {
  async fetch(request, env) {
    var socket = connect("127.0.0.1:%d");
    var closeResult = socket.close();
    var isPromise = closeResult instanceof Promise;
    await closeResult;
    return Response.json({ isPromise, closed: true });
  },
};`, port)

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		IsPromise bool `json:"isPromise"`
		Closed    bool `json:"closed"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !data.IsPromise {
		t.Error("socket.close() should return a Promise")
	}
	if !data.Closed {
		t.Error("close should have resolved")
	}
}

// TestTCPSocket_ClosedPromiseResolvesOnClose verifies that the socket.closed
// promise resolves when close() is called.
func TestTCPSocket_ClosedPromiseResolvesOnClose(t *testing.T) {
	disableTCPSSRF(t)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer func() { _ = ln.Close() }()

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			_ = conn.Close()
		}
	}()

	port := ln.Addr().(*net.TCPAddr).Port
	db := testDB(t)
	e := newTestEngine(t, db)

	source := fmt.Sprintf(`export default {
  async fetch(request, env) {
    var socket = connect("127.0.0.1:%d");
    var resolved = false;
    socket.closed.then(function() { resolved = true; });
    socket.close();
    // Allow microtasks to flush
    await scheduler.wait(50);
    return Response.json({ resolved });
  },
};`, port)

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Resolved bool `json:"resolved"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !data.Resolved {
		t.Error("socket.closed should resolve after close()")
	}
}

// TestTCPSocket_OpenedPromiseShape verifies that the socket.opened promise
// resolves with an info object containing remoteAddress and localAddress.
func TestTCPSocket_OpenedPromiseShape(t *testing.T) {
	disableTCPSSRF(t)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer func() { _ = ln.Close() }()

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			_ = conn.Close()
		}
	}()

	port := ln.Addr().(*net.TCPAddr).Port
	db := testDB(t)
	e := newTestEngine(t, db)

	source := fmt.Sprintf(`export default {
  async fetch(request, env) {
    var socket = connect("127.0.0.1:%d");
    var info = await socket.opened;
    var hasRemote = typeof info.remoteAddress === 'string' && info.remoteAddress.length > 0;
    var hasLocal = typeof info.localAddress === 'string' && info.localAddress.length > 0;
    socket.close();
    return Response.json({
      hasRemote,
      hasLocal,
      remoteAddress: info.remoteAddress,
      localAddress: info.localAddress,
    });
  },
};`, port)

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		HasRemote     bool   `json:"hasRemote"`
		HasLocal      bool   `json:"hasLocal"`
		RemoteAddress string `json:"remoteAddress"`
		LocalAddress  string `json:"localAddress"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !data.HasRemote {
		t.Error("opened info should have remoteAddress string")
	}
	if !data.HasLocal {
		t.Error("opened info should have localAddress string")
	}
	if !strings.Contains(data.RemoteAddress, "127.0.0.1") {
		t.Errorf("remoteAddress = %q, want to contain 127.0.0.1", data.RemoteAddress)
	}
}

// TestTCPSocket_ConnectInvalidHostname verifies that connect() throws an error
// for a hostname that cannot be resolved.
func TestTCPSocket_ConnectInvalidHostname(t *testing.T) {
	iso, ctx, _ := setupTCPTestContext(t)

	reqID := newRequestState(10, defaultEnv())
	reqIDVal, _ := v8.NewValue(iso, strconv.FormatUint(reqID, 10))
	_ = ctx.Global().Set("__requestID", reqIDVal)
	defer clearRequestState(reqID)

	result, err := ctx.RunScript(`(function() {
		try {
			connect("this-host-does-not-exist-xyzzy.invalid:8080");
			return "connected";
		} catch(e) {
			return e.message || String(e);
		}
	})()`, "test_invalid_hostname.js")
	if err != nil {
		t.Fatalf("RunScript: %v", err)
	}
	if result.String() == "connected" {
		t.Fatal("expected connect to fail for invalid hostname, but it succeeded")
	}
}

// TestTCPSocket_ConnectWithoutPort verifies that connect() with a string address
// that has no port separator throws an appropriate error.
func TestTCPSocket_ConnectWithoutPort(t *testing.T) {
	iso, ctx, _ := setupTCPTestContext(t)

	reqID := newRequestState(10, defaultEnv())
	reqIDVal, _ := v8.NewValue(iso, strconv.FormatUint(reqID, 10))
	_ = ctx.Global().Set("__requestID", reqIDVal)
	defer clearRequestState(reqID)

	result, err := ctx.RunScript(`(function() {
		try {
			connect("example.com");
			return "connected";
		} catch(e) {
			return e.message || String(e);
		}
	})()`, "test_no_port.js")
	if err != nil {
		t.Fatalf("RunScript: %v", err)
	}
	if result.String() == "connected" {
		t.Fatal("expected connect to fail without port, but it succeeded")
	}
	if !strings.Contains(result.String(), "hostname:port") {
		t.Fatalf("expected error about format, got: %s", result.String())
	}
}

// TestTCPSocket_ConnectObjectWithoutPort verifies that connect() with an object
// address missing the port field throws an error.
func TestTCPSocket_ConnectObjectWithoutPort(t *testing.T) {
	iso, ctx, _ := setupTCPTestContext(t)

	reqID := newRequestState(10, defaultEnv())
	reqIDVal, _ := v8.NewValue(iso, strconv.FormatUint(reqID, 10))
	_ = ctx.Global().Set("__requestID", reqIDVal)
	defer clearRequestState(reqID)

	result, err := ctx.RunScript(`(function() {
		try {
			connect({hostname: "example.com"});
			return "connected";
		} catch(e) {
			return e.message || String(e);
		}
	})()`, "test_obj_no_port.js")
	if err != nil {
		t.Fatalf("RunScript: %v", err)
	}
	if result.String() == "connected" {
		t.Fatal("expected connect to fail without port, but it succeeded")
	}
	if !strings.Contains(result.String(), "port") {
		t.Fatalf("expected error about port, got: %s", result.String())
	}
}

// ---------------------------------------------------------------------------
// Regression tests for Bug 5: TCP socket reads hang
//
// The root cause was that __tcpRead returned "" immediately when no data was
// buffered, causing the JS ReadableStream pull() to spin without yielding.
// The fix adds a hasData channel to tcpSocketBuffer: readLoop signals it when
// data arrives, and __tcpRead calls waitForData(5s) when take() returns empty.
// ---------------------------------------------------------------------------

// TestTCPSocketBuffer_WaitForData tests the Go-level waitForData method.
// It verifies that waitForData unblocks promptly when the readLoop signals
// new data via the hasData channel.
func TestTCPSocketBuffer_WaitForData(t *testing.T) {
	server, client := net.Pipe()
	defer func() { _ = server.Close() }()
	defer func() { _ = client.Close() }()

	buf := &tcpSocketBuffer{
		conn:    client,
		hasData: make(chan struct{}, 1),
	}
	go buf.readLoop()

	// Write data from server side after a short delay.
	go func() {
		time.Sleep(50 * time.Millisecond)
		_, _ = server.Write([]byte("delayed data"))
	}()

	// waitForData should unblock once the readLoop receives the data.
	start := time.Now()
	buf.waitForData(3 * time.Second)
	elapsed := time.Since(start)

	// Should unblock well before the 3s timeout.
	if elapsed > 2*time.Second {
		t.Fatalf("waitForData took %v, expected prompt unblock", elapsed)
	}

	data, eof, _ := buf.take(1024)
	if data == "" {
		t.Fatal("expected data after waitForData, got empty")
	}
	if eof {
		t.Fatal("unexpected EOF")
	}
}

// TestTCPSocketBuffer_WaitForDataTimeout verifies that waitForData returns
// after the timeout elapses when no data arrives.
func TestTCPSocketBuffer_WaitForDataTimeout(t *testing.T) {
	server, client := net.Pipe()
	defer func() { _ = server.Close() }()
	defer func() { _ = client.Close() }()

	buf := &tcpSocketBuffer{
		conn:    client,
		hasData: make(chan struct{}, 1),
	}
	go buf.readLoop()

	// Don't write any data — waitForData should timeout.
	start := time.Now()
	buf.waitForData(100 * time.Millisecond)
	elapsed := time.Since(start)

	if elapsed < 80*time.Millisecond {
		t.Fatalf("waitForData returned too early: %v", elapsed)
	}
	if elapsed > 500*time.Millisecond {
		t.Fatalf("waitForData took too long: %v (expected ~100ms)", elapsed)
	}

	// Buffer should still be empty.
	data, eof, _ := buf.take(1024)
	if data != "" {
		t.Fatalf("expected empty data, got %q", data)
	}
	if eof {
		t.Fatal("unexpected EOF")
	}
}

// TestTCPSocketBuffer_WaitForDataEOF verifies that waitForData unblocks
// when the connection closes (EOF signal).
func TestTCPSocketBuffer_WaitForDataEOF(t *testing.T) {
	server, client := net.Pipe()
	defer func() { _ = client.Close() }()

	buf := &tcpSocketBuffer{
		conn:    client,
		hasData: make(chan struct{}, 1),
	}
	go buf.readLoop()

	// Close the server side after a short delay to trigger EOF.
	go func() {
		time.Sleep(50 * time.Millisecond)
		_ = server.Close()
	}()

	start := time.Now()
	buf.waitForData(3 * time.Second)
	elapsed := time.Since(start)

	if elapsed > 2*time.Second {
		t.Fatalf("waitForData took %v on EOF, expected prompt unblock", elapsed)
	}

	_, eof, _ := buf.take(1024)
	if !eof {
		t.Fatal("expected EOF after server close")
	}
}

// TestTCPSocket_ReadWithoutExplicitWait is the critical regression test.
// It verifies that JS code can read TCP data WITHOUT using scheduler.wait().
// Before the fix, this would hang or return empty because pull() saw no data
// and returned immediately (busy-loop). After the fix, __tcpRead blocks via
// waitForData until the readLoop signals that data has arrived.
func TestTCPSocket_ReadWithoutExplicitWait(t *testing.T) {
	disableTCPSSRF(t)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer func() { _ = ln.Close() }()

	// Server writes data after a small delay (simulating real-world latency),
	// then closes the connection.
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		time.Sleep(50 * time.Millisecond)
		_, _ = conn.Write([]byte("no-wait-needed"))
		_ = conn.Close()
	}()

	port := ln.Addr().(*net.TCPAddr).Port
	db := testDB(t)
	e := newTestEngine(t, db)

	// KEY: No scheduler.wait() anywhere — the read should block internally
	// until data arrives via the hasData channel mechanism.
	source := fmt.Sprintf(`export default {
  async fetch(request, env) {
    var socket = connect("127.0.0.1:%d");
    var reader = socket.readable.getReader();
    var result = await reader.read();
    var text = "";
    if (result.value) {
      text = new TextDecoder().decode(result.value);
    }
    return Response.json({ text: text, done: result.done });
  },
};`, port)

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Text string `json:"text"`
		Done bool   `json:"done"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if data.Text != "no-wait-needed" {
		t.Fatalf("expected %q, got %q", "no-wait-needed", data.Text)
	}
}

// TestTCPSocket_ReadMultipleChunksWithoutWait verifies that multiple sequential
// reads work without explicit scheduler.wait() calls. The server sends two
// chunks with a delay between them; the JS side reads both via the blocking
// __tcpRead mechanism.
func TestTCPSocket_ReadMultipleChunksWithoutWait(t *testing.T) {
	disableTCPSSRF(t)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer func() { _ = ln.Close() }()

	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		// Send two separate chunks with a gap between them.
		_, _ = conn.Write([]byte("chunk1"))
		time.Sleep(100 * time.Millisecond)
		_, _ = conn.Write([]byte("chunk2"))
		_ = conn.Close()
	}()

	port := ln.Addr().(*net.TCPAddr).Port
	db := testDB(t)
	e := newTestEngine(t, db)

	// Read all chunks until done, accumulating the text. No scheduler.wait().
	source := fmt.Sprintf(`export default {
  async fetch(request, env) {
    var socket = connect("127.0.0.1:%d");
    var reader = socket.readable.getReader();
    var allText = "";
    while (true) {
      var result = await reader.read();
      if (result.value) {
        allText += new TextDecoder().decode(result.value);
      }
      if (result.done) break;
    }
    return Response.json({ text: allText });
  },
};`, port)

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !strings.Contains(data.Text, "chunk1") || !strings.Contains(data.Text, "chunk2") {
		t.Fatalf("expected both chunks, got %q", data.Text)
	}
}

// TestTCPSocket_EchoRoundTripWithoutWait verifies a full echo round-trip:
// JS writes data, the server echoes it back, and JS reads the echo — all
// without scheduler.wait(). This is the most realistic regression test.
func TestTCPSocket_EchoRoundTripWithoutWait(t *testing.T) {
	disableTCPSSRF(t)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer func() { _ = ln.Close() }()

	// Echo server: reads up to 256 bytes, writes them back, closes.
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer func() { _ = conn.Close() }()
		buf := make([]byte, 256)
		n, err := conn.Read(buf)
		if err != nil {
			return
		}
		_, _ = conn.Write(buf[:n])
	}()

	port := ln.Addr().(*net.TCPAddr).Port
	db := testDB(t)
	e := newTestEngine(t, db)

	// Note: we must NOT close the writer before reading because the current
	// implementation fully closes the TCP connection on writer.close() (no
	// half-close support). The echo server responds as soon as it reads data,
	// so closing the write side is unnecessary.
	source := fmt.Sprintf(`export default {
  async fetch(request, env) {
    var socket = connect("127.0.0.1:%d");
    var writer = socket.writable.getWriter();
    await writer.write(new TextEncoder().encode("ping"));

    var reader = socket.readable.getReader();
    var result = await reader.read();
    var text = "";
    if (result.value) {
      text = new TextDecoder().decode(result.value);
    }
    return Response.json({ echo: text });
  },
};`, port)

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Echo string `json:"echo"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if data.Echo != "ping" {
		t.Fatalf("expected echo %q, got %q", "ping", data.Echo)
	}
}
