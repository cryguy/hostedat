package worker

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

// disableEventSourceSSRF temporarily disables SSRF protection so tests can
// connect to httptest servers on 127.0.0.1. Restored via t.Cleanup.
func disableEventSourceSSRF(t *testing.T) {
	t.Helper()
	origSSRF := eventSourceSSRFEnabled
	origTransport := eventSourceTransport
	eventSourceSSRFEnabled = false
	eventSourceTransport = http.DefaultTransport
	t.Cleanup(func() {
		eventSourceSSRFEnabled = origSSRF
		eventSourceTransport = origTransport
	})
}

// ---------------------------------------------------------------------------
// EventSource Tests
// ---------------------------------------------------------------------------

func TestEventSource_ExistsGlobally(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    return Response.json({
      hasEventSource: typeof EventSource === 'function',
      hasCONNECTING: EventSource.CONNECTING === 0,
      hasOPEN: EventSource.OPEN === 1,
      hasCLOSED: EventSource.CLOSED === 2,
    });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		HasEventSource bool `json:"hasEventSource"`
		HasCONNECTING  bool `json:"hasCONNECTING"`
		HasOPEN        bool `json:"hasOPEN"`
		HasCLOSED      bool `json:"hasCLOSED"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !data.HasEventSource {
		t.Error("EventSource should be a function")
	}
	if !data.HasCONNECTING {
		t.Error("EventSource.CONNECTING should be 0")
	}
	if !data.HasOPEN {
		t.Error("EventSource.OPEN should be 1")
	}
	if !data.HasCLOSED {
		t.Error("EventSource.CLOSED should be 2")
	}
}

func TestEventSource_ConstructorSetsURL(t *testing.T) {
	disableEventSourceSSRF(t)
	// Start a simple SSE server that sends one event then closes.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.WriteHeader(200)
		_, _ = fmt.Fprintf(w, "data: hello\n\n")
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
	}))
	defer srv.Close()

	db := testDB(t)
	e := newTestEngine(t, db)

	source := fmt.Sprintf(`export default {
  async fetch(request, env) {
    const es = new EventSource("%s");
    const url = es.url;
    const state = es.readyState;
    es.close();
    return Response.json({ url: url, readyState: state });
  },
};`, srv.URL)

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		URL        string `json:"url"`
		ReadyState int    `json:"readyState"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if data.URL != srv.URL {
		t.Errorf("url = %q, want %q", data.URL, srv.URL)
	}
	// readyState should be OPEN (1) since the connection succeeded.
	if data.ReadyState != 1 {
		t.Errorf("readyState = %d, want 1 (OPEN)", data.ReadyState)
	}
}

func TestEventSource_CloseSetsClosed(t *testing.T) {
	disableEventSourceSSRF(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(200)
		_, _ = fmt.Fprintf(w, "data: hello\n\n")
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
	}))
	defer srv.Close()

	db := testDB(t)
	e := newTestEngine(t, db)

	source := fmt.Sprintf(`export default {
  async fetch(request, env) {
    const es = new EventSource("%s");
    es.close();
    return Response.json({ readyState: es.readyState });
  },
};`, srv.URL)

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		ReadyState int `json:"readyState"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if data.ReadyState != 2 {
		t.Errorf("readyState = %d, want 2 (CLOSED)", data.ReadyState)
	}
}

func TestEventSource_ReceivesMessages(t *testing.T) {
	disableEventSourceSSRF(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.WriteHeader(200)
		_, _ = fmt.Fprintf(w, "data: msg1\n\n")
		_, _ = fmt.Fprintf(w, "data: msg2\n\n")
		_, _ = fmt.Fprintf(w, "event: custom\ndata: msg3\n\n")
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		// Close the response to signal end of stream.
	}))
	defer srv.Close()

	db := testDB(t)
	e := newTestEngine(t, db)

	source := fmt.Sprintf(`export default {
  async fetch(request, env) {
    const es = new EventSource("%s");
    const messages = [];
    const customMessages = [];

    return new Promise((resolve) => {
      es.addEventListener('message', (e) => {
        messages.push(e.data);
        if (messages.length >= 2) {
          // Wait a tiny bit for custom event.
          setTimeout(() => {
            es.close();
            resolve(Response.json({
              messages: messages,
              customMessages: customMessages,
            }));
          }, 200);
        }
      });
      es.addEventListener('custom', (e) => {
        customMessages.push(e.data);
      });
      // Timeout safety.
      setTimeout(() => {
        es.close();
        resolve(Response.json({
          messages: messages,
          customMessages: customMessages,
          timeout: true,
        }));
      }, 3000);
    });
  },
};`, srv.URL)

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Messages       []string `json:"messages"`
		CustomMessages []string `json:"customMessages"`
		Timeout        bool     `json:"timeout"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if data.Timeout {
		t.Log("test timed out waiting for messages (may be a timing issue)")
	}
	if len(data.Messages) < 2 {
		t.Errorf("got %d messages, want at least 2", len(data.Messages))
	}
	if len(data.Messages) >= 1 && data.Messages[0] != "msg1" {
		t.Errorf("messages[0] = %q, want msg1", data.Messages[0])
	}
	if len(data.Messages) >= 2 && data.Messages[1] != "msg2" {
		t.Errorf("messages[1] = %q, want msg2", data.Messages[1])
	}
}

func TestEventSource_SSRFBlocked(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	// The EventSource constructor does NOT throw on connection failure —
	// it catches errors internally, sets readyState to CLOSED, and
	// dispatches an 'error' event. So we check readyState and error event.
	source := `export default {
  async fetch(request, env) {
    const es = new EventSource("http://127.0.0.1:9999/sse");
    let errorFired = false;
    let errorMsg = '';
    es.addEventListener('error', (e) => {
      errorFired = true;
      errorMsg = e.message || '';
    });
    // readyState should be CLOSED (2) because connection was blocked.
    const state = es.readyState;
    es.close();
    return Response.json({ readyState: state, errorFired, errorMsg });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		ReadyState int    `json:"readyState"`
		ErrorFired bool   `json:"errorFired"`
		ErrorMsg   string `json:"errorMsg"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if data.ReadyState != 2 {
		t.Errorf("readyState = %d, want 2 (CLOSED) for SSRF-blocked connection", data.ReadyState)
	}
}

func TestEventSource_LastEventId(t *testing.T) {
	disableEventSourceSSRF(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.WriteHeader(200)
		_, _ = fmt.Fprintf(w, "id: evt-42\ndata: with-id\n\n")
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
	}))
	defer srv.Close()

	db := testDB(t)
	e := newTestEngine(t, db)

	source := fmt.Sprintf(`export default {
  async fetch(request, env) {
    const es = new EventSource("%s");
    return new Promise((resolve) => {
      es.addEventListener('message', (e) => {
        es.close();
        resolve(Response.json({ data: e.data, lastEventId: e.lastEventId }));
      });
      setTimeout(() => {
        es.close();
        resolve(Response.json({ data: null, lastEventId: null, timeout: true }));
      }, 3000);
    });
  },
};`, srv.URL)

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Data        string `json:"data"`
		LastEventId string `json:"lastEventId"`
		Timeout     bool   `json:"timeout"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if data.Timeout {
		t.Log("test timed out waiting for SSE message")
	}
	if data.Data != "with-id" {
		t.Errorf("data = %q, want %q", data.Data, "with-id")
	}
	if data.LastEventId != "evt-42" {
		t.Errorf("lastEventId = %q, want %q", data.LastEventId, "evt-42")
	}
}

func TestEventSource_OnOpenSetter(t *testing.T) {
	disableEventSourceSSRF(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(200)
		_, _ = fmt.Fprintf(w, "data: hello\n\n")
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
	}))
	defer srv.Close()

	db := testDB(t)
	e := newTestEngine(t, db)

	source := fmt.Sprintf(`export default {
  async fetch(request, env) {
    const es = new EventSource("%s");
    // The 'open' event fires during construction if connection succeeds.
    // Since the constructor already dispatched 'open', we check that onopen getter works.
    const hasOnOpen = 'onopen' in es;
    const hasOnError = 'onerror' in es;
    const hasOnMessage = 'onmessage' in es;
    es.close();
    return Response.json({ hasOnOpen, hasOnError, hasOnMessage });
  },
};`, srv.URL)

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		HasOnOpen    bool `json:"hasOnOpen"`
		HasOnError   bool `json:"hasOnError"`
		HasOnMessage bool `json:"hasOnMessage"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !data.HasOnOpen {
		t.Error("onopen property should exist on EventSource")
	}
	if !data.HasOnError {
		t.Error("onerror property should exist on EventSource")
	}
	if !data.HasOnMessage {
		t.Error("onmessage property should exist on EventSource")
	}
}

// TestEventSource_InvalidURL exercises the http.NewRequestWithContext error
// path in the EventSource goroutine (eventsource.go ~line 188).
func TestEventSource_InvalidURL(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	// A URL with no scheme/host causes NewRequestWithContext to fail.
	source := `export default {
  async fetch(request, env) {
    const es = new EventSource("://bad url");
    // Give the goroutine time to hit the error path.
    await new Promise(r => setTimeout(r, 300));
    const state = es.readyState;
    es.close();
    return Response.json({ readyState: state });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		ReadyState int `json:"readyState"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	// Should be CLOSED (2) because the goroutine sets closed=true on error.
	if data.ReadyState != 2 {
		t.Errorf("readyState = %d, want 2 (CLOSED)", data.ReadyState)
	}
}

// TestEventSource_ConnectionRefused exercises the client.Do error path
// in the EventSource goroutine (eventsource.go ~line 203).
func TestEventSource_ConnectionRefused(t *testing.T) {
	disableEventSourceSSRF(t)
	db := testDB(t)
	e := newTestEngine(t, db)

	// Port 1 on localhost is almost certainly not listening.
	source := `export default {
  async fetch(request, env) {
    const es = new EventSource("http://127.0.0.1:1/nope");
    // Give the goroutine time to attempt the connection and fail.
    await new Promise(r => setTimeout(r, 500));
    const state = es.readyState;
    es.close();
    return Response.json({ readyState: state });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		ReadyState int `json:"readyState"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if data.ReadyState != 2 {
		t.Errorf("readyState = %d, want 2 (CLOSED)", data.ReadyState)
	}
}
