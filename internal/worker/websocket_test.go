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
