package worker

import (
	"encoding/json"
	"testing"
)

func TestWebSocket_PairCreation(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request, env) {
    var pair = new WebSocketPair();
    return Response.json({
      has0: pair[0] !== undefined,
      has1: pair[1] !== undefined,
      is0WS: pair[0] instanceof WebSocket,
      is1WS: pair[1] instanceof WebSocket,
    });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Has0 bool `json:"has0"`
		Has1 bool `json:"has1"`
		Is0  bool `json:"is0WS"`
		Is1  bool `json:"is1WS"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !data.Has0 || !data.Has1 {
		t.Error("pair should have [0] and [1]")
	}
	if !data.Is0 || !data.Is1 {
		t.Error("pair members should be WebSocket instances")
	}
}

func TestWebSocket_AcceptAndReadyState(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request, env) {
    var pair = new WebSocketPair();
    var server = pair[1];
    var stateBefore = server.readyState;
    server.accept();
    var stateAfter = server.readyState;
    return Response.json({
      before: stateBefore,
      after: stateAfter,
      CONNECTING: WebSocket.CONNECTING,
      OPEN: WebSocket.OPEN,
      CLOSING: WebSocket.CLOSING,
      CLOSED: WebSocket.CLOSED,
    });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Before     int `json:"before"`
		After      int `json:"after"`
		CONNECTING int `json:"CONNECTING"`
		OPEN       int `json:"OPEN"`
		CLOSING    int `json:"CLOSING"`
		CLOSED     int `json:"CLOSED"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if data.Before != 0 {
		t.Errorf("readyState before accept = %d, want 0 (CONNECTING)", data.Before)
	}
	if data.After != 1 {
		t.Errorf("readyState after accept = %d, want 1 (OPEN)", data.After)
	}
	if data.CONNECTING != 0 || data.OPEN != 1 || data.CLOSING != 2 || data.CLOSED != 3 {
		t.Errorf("WebSocket constants: CONNECTING=%d, OPEN=%d, CLOSING=%d, CLOSED=%d",
			data.CONNECTING, data.OPEN, data.CLOSING, data.CLOSED)
	}
}

func TestWebSocket_SendThrowsWhenNotOpen(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request, env) {
    var pair = new WebSocketPair();
    var server = pair[1];
    try {
      server.send("test");
      return Response.json({ error: false });
    } catch (e) {
      return Response.json({ error: true, message: e.message });
    }
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Error   bool   `json:"error"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !data.Error {
		t.Error("send() before accept() should throw")
	}
}

func TestWebSocket_EventListeners(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request, env) {
    var pair = new WebSocketPair();
    var server = pair[1];
    var received = [];

    server.addEventListener('message', function(event) {
      received.push(event.data);
    });
    server.accept();

    // Manually dispatch a message event for testing
    server._dispatch('message', { data: 'hello' });
    server._dispatch('message', { data: 'world' });

    return Response.json({ received: received });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Received []string `json:"received"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(data.Received) != 2 || data.Received[0] != "hello" || data.Received[1] != "world" {
		t.Errorf("received = %v, want ['hello', 'world']", data.Received)
	}
}

func TestWebSocket_RemoveEventListener(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request, env) {
    var pair = new WebSocketPair();
    var server = pair[1];
    var count = 0;

    var handler = function() { count++; };
    server.addEventListener('message', handler);
    server.accept();

    server._dispatch('message', { data: 'a' });
    server.removeEventListener('message', handler);
    server._dispatch('message', { data: 'b' });

    return Response.json({ count: count });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Count int `json:"count"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if data.Count != 1 {
		t.Errorf("count = %d, want 1 (listener removed after first dispatch)", data.Count)
	}
}

func TestWebSocket_UpgradeResponse(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	// Test that a 101 response with webSocket property is correctly constructed
	source := `export default {
  fetch(request, env) {
    var pair = new WebSocketPair();
    var client = pair[0];
    var server = pair[1];
    server.accept();

    var resp = new Response(null, {
      status: 101,
      webSocket: client,
    });

    return Response.json({
      status: resp.status,
      hasWebSocket: resp.webSocket !== null && resp.webSocket !== undefined,
    });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Status       int  `json:"status"`
		HasWebSocket bool `json:"hasWebSocket"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if data.Status != 101 {
		t.Errorf("status = %d, want 101", data.Status)
	}
	if !data.HasWebSocket {
		t.Error("response should have webSocket property")
	}
}

func TestWebSocket_OnPropertyHandler(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request, env) {
    var pair = new WebSocketPair();
    var server = pair[1];
    var received = null;

    server.onmessage = function(event) {
      received = event.data;
    };
    server.accept();
    server._dispatch('message', { data: 'via-onmessage' });

    return Response.json({ received: received });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Received string `json:"received"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if data.Received != "via-onmessage" {
		t.Errorf("received = %q, want 'via-onmessage'", data.Received)
	}
}

func TestWebSocket_PeerLinked(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request, env) {
    var pair = new WebSocketPair();
    return Response.json({
      peerLinked: pair[0]._peer === pair[1],
      reverseLinked: pair[1]._peer === pair[0],
    });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		PeerLinked    bool `json:"peerLinked"`
		ReverseLinked bool `json:"reverseLinked"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !data.PeerLinked || !data.ReverseLinked {
		t.Error("WebSocketPair members should be linked as peers")
	}
}

func TestWebSocket_BinaryType(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request, env) {
    var pair = new WebSocketPair();
    return Response.json({
      defaultBinaryType: pair[0].binaryType,
      serverBinaryType: pair[1].binaryType,
    });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		DefaultBinaryType string `json:"defaultBinaryType"`
		ServerBinaryType  string `json:"serverBinaryType"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if data.DefaultBinaryType != "arraybuffer" {
		t.Errorf("default binaryType = %q, want 'arraybuffer'", data.DefaultBinaryType)
	}
	if data.ServerBinaryType != "arraybuffer" {
		t.Errorf("server binaryType = %q, want 'arraybuffer'", data.ServerBinaryType)
	}
}

func TestWebSocket_SendStringCallsTextMode(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	// Test that send() with a string passes the string directly and isBinary=false.
	// Since __wsSend requires a wsConn (which we don't have in unit tests),
	// we override __wsSend to capture the arguments.
	source := `export default {
  fetch(request, env) {
    var captured = {};
    globalThis.__wsSend = function(data, isBinary) {
      captured.data = data;
      captured.isBinary = isBinary;
      captured.dataType = typeof data;
    };

    var pair = new WebSocketPair();
    var server = pair[1];
    server.accept();
    server.send("hello text");

    return Response.json(captured);
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Data     string `json:"data"`
		IsBinary bool   `json:"isBinary"`
		DataType string `json:"dataType"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if data.Data != "hello text" {
		t.Errorf("data = %q, want 'hello text'", data.Data)
	}
	if data.IsBinary {
		t.Error("isBinary should be false for string data")
	}
	if data.DataType != "string" {
		t.Errorf("dataType = %q, want 'string'", data.DataType)
	}
}

func TestWebSocket_SendArrayBufferCallsBinaryMode(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	// Test that send() with ArrayBuffer base64-encodes and passes isBinary=true.
	source := `export default {
  fetch(request, env) {
    var captured = {};
    globalThis.__wsSend = function(data, isBinary) {
      captured.data = data;
      captured.isBinary = isBinary;
      captured.dataType = typeof data;
    };

    var pair = new WebSocketPair();
    var server = pair[1];
    server.accept();

    // Create an ArrayBuffer with bytes [1, 2, 3, 4]
    var buf = new ArrayBuffer(4);
    var view = new Uint8Array(buf);
    view[0] = 1; view[1] = 2; view[2] = 3; view[3] = 4;
    server.send(buf);

    return Response.json(captured);
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Data     string `json:"data"`
		IsBinary bool   `json:"isBinary"`
		DataType string `json:"dataType"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !data.IsBinary {
		t.Error("isBinary should be true for ArrayBuffer data")
	}
	if data.DataType != "string" {
		t.Errorf("dataType = %q, want 'string' (base64-encoded)", data.DataType)
	}
	// Verify the base64 round-trips correctly: [1,2,3,4] -> "AQIDBA=="
	if data.Data != "AQIDBA==" {
		t.Errorf("base64 data = %q, want 'AQIDBA=='", data.Data)
	}
}

func TestWebSocket_SendTypedArrayCallsBinaryMode(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	// Test that send() with a Uint8Array (TypedArray view) also uses binary mode.
	source := `export default {
  fetch(request, env) {
    var captured = {};
    globalThis.__wsSend = function(data, isBinary) {
      captured.data = data;
      captured.isBinary = isBinary;
    };

    var pair = new WebSocketPair();
    var server = pair[1];
    server.accept();

    var arr = new Uint8Array([10, 20, 30]);
    server.send(arr);

    return Response.json(captured);
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Data     string `json:"data"`
		IsBinary bool   `json:"isBinary"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !data.IsBinary {
		t.Error("isBinary should be true for Uint8Array data")
	}
	// [10, 20, 30] -> "ChQe"
	if data.Data != "ChQe" {
		t.Errorf("base64 data = %q, want 'ChQe'", data.Data)
	}
}

func TestWebSocket_SendNonStringFallback(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	// Test that send() with a non-string, non-buffer value falls back to String(data) text mode.
	source := `export default {
  fetch(request, env) {
    var captured = {};
    globalThis.__wsSend = function(data, isBinary) {
      captured.data = data;
      captured.isBinary = isBinary;
    };

    var pair = new WebSocketPair();
    var server = pair[1];
    server.accept();
    server.send(42);

    return Response.json(captured);
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Data     string `json:"data"`
		IsBinary bool   `json:"isBinary"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if data.IsBinary {
		t.Error("isBinary should be false for number data")
	}
	if data.Data != "42" {
		t.Errorf("data = %q, want '42'", data.Data)
	}
}

func TestWebSocket_BinaryDispatchCreatesArrayBuffer(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	// Test that dispatching a binary message (simulated via _dispatch with ArrayBuffer)
	// creates an ArrayBuffer data value that can be read back as bytes.
	source := `export default {
  fetch(request, env) {
    var pair = new WebSocketPair();
    var server = pair[1];
    var result = {};

    server.addEventListener('message', function(event) {
      result.isArrayBuffer = event.data instanceof ArrayBuffer;
      if (event.data instanceof ArrayBuffer) {
        var view = new Uint8Array(event.data);
        result.bytes = Array.from(view);
        result.byteLength = event.data.byteLength;
      }
    });
    server.accept();

    // Simulate what Bridge() does for binary messages:
    // base64-encode, set global, run the dispatch script.
    var b64 = __bufferSourceToB64(new Uint8Array([72, 101, 108, 108, 111]));
    var binary = __b64ToBuffer(b64);
    server._dispatch('message', { data: binary });

    return Response.json(result);
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		IsArrayBuffer bool  `json:"isArrayBuffer"`
		Bytes         []int `json:"bytes"`
		ByteLength    int   `json:"byteLength"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !data.IsArrayBuffer {
		t.Error("binary message data should be an ArrayBuffer")
	}
	if data.ByteLength != 5 {
		t.Errorf("byteLength = %d, want 5", data.ByteLength)
	}
	expected := []int{72, 101, 108, 108, 111} // "Hello"
	if len(data.Bytes) != len(expected) {
		t.Fatalf("bytes length = %d, want %d", len(data.Bytes), len(expected))
	}
	for i, b := range expected {
		if data.Bytes[i] != b {
			t.Errorf("byte[%d] = %d, want %d", i, data.Bytes[i], b)
		}
	}
}

func TestWebSocket_TextDispatchRemainsString(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	// Verify that text message dispatch still delivers string data.
	source := `export default {
  fetch(request, env) {
    var pair = new WebSocketPair();
    var server = pair[1];
    var result = {};

    server.addEventListener('message', function(event) {
      result.isString = typeof event.data === 'string';
      result.data = event.data;
    });
    server.accept();

    // Simulate text message dispatch (same as Bridge() text path).
    server._dispatch('message', { data: 'hello text' });

    return Response.json(result);
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		IsString bool   `json:"isString"`
		Data     string `json:"data"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !data.IsString {
		t.Error("text message data should be a string")
	}
	if data.Data != "hello text" {
		t.Errorf("data = %q, want 'hello text'", data.Data)
	}
}
