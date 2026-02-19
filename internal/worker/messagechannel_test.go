package worker

import (
	"encoding/json"
	"testing"
)

func TestMessageChannel_BasicMessagePassing(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    const channel = new MessageChannel();
    let received = null;
    channel.port2.onmessage = function(e) {
      received = e.data;
    };
    channel.port1.postMessage({ hello: "world" });
    // Wait for microtask to deliver.
    await new Promise(resolve => queueMicrotask(resolve));
    return Response.json({ received });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Received struct {
			Hello string `json:"hello"`
		} `json:"received"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if data.Received.Hello != "world" {
		t.Errorf("received.hello = %q, want 'world'", data.Received.Hello)
	}
}

func TestMessageChannel_BidirectionalMessages(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    const channel = new MessageChannel();
    let port1Received = null;
    let port2Received = null;

    channel.port1.onmessage = function(e) {
      port1Received = e.data;
    };
    channel.port2.onmessage = function(e) {
      port2Received = e.data;
    };

    channel.port1.postMessage("from port1");
    channel.port2.postMessage("from port2");

    await new Promise(resolve => queueMicrotask(resolve));
    return Response.json({ port1Received, port2Received });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Port1Received string `json:"port1Received"`
		Port2Received string `json:"port2Received"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if data.Port1Received != "from port2" {
		t.Errorf("port1Received = %q, want 'from port2'", data.Port1Received)
	}
	if data.Port2Received != "from port1" {
		t.Errorf("port2Received = %q, want 'from port1'", data.Port2Received)
	}
}

func TestMessageChannel_Close(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    const channel = new MessageChannel();
    let received = false;
    channel.port2.onmessage = function(e) {
      received = true;
    };
    channel.port1.close();
    channel.port1.postMessage("test");
    await new Promise(resolve => queueMicrotask(resolve));
    return Response.json({ received });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Received bool `json:"received"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if data.Received {
		t.Error("messages should not be delivered after close()")
	}
}

func TestMessageChannel_MultipleListeners(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    const channel = new MessageChannel();
    let count = 0;
    channel.port2.addEventListener('message', function(e) {
      count++;
    });
    channel.port2.addEventListener('message', function(e) {
      count++;
    });
    channel.port1.postMessage("test");
    await new Promise(resolve => queueMicrotask(resolve));
    return Response.json({ count });
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

	if data.Count != 2 {
		t.Errorf("count = %d, want 2 (both listeners should fire)", data.Count)
	}
}

func TestMessageChannel_DataCloned(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    const channel = new MessageChannel();
    const original = { a: 1, b: [2, 3] };
    let received = null;
    channel.port2.onmessage = function(e) {
      received = e.data;
    };
    channel.port1.postMessage(original);
    await new Promise(resolve => queueMicrotask(resolve));
    // Mutate original after sending.
    original.a = 99;
    original.b.push(4);
    return Response.json({
      receivedA: received.a,
      receivedBLen: received.b.length,
    });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		ReceivedA    int `json:"receivedA"`
		ReceivedBLen int `json:"receivedBLen"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if data.ReceivedA != 1 {
		t.Errorf("receivedA = %d, want 1 (should be cloned)", data.ReceivedA)
	}
	if data.ReceivedBLen != 2 {
		t.Errorf("receivedBLen = %d, want 2 (should be cloned)", data.ReceivedBLen)
	}
}
