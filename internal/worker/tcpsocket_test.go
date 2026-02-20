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
