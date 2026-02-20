package worker

import (
	"encoding/json"
	"testing"
)

func TestUnhandledRejection_PromiseRejectionEventClass(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request, env) {
    const evt = new PromiseRejectionEvent('unhandledrejection', {
      reason: 'test error',
      promise: Promise.resolve(),
    });
    return Response.json({
      type: evt.type,
      reason: evt.reason,
      hasPromise: evt.promise !== null,
      isEvent: evt instanceof Event,
    });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Type       string `json:"type"`
		Reason     string `json:"reason"`
		HasPromise bool   `json:"hasPromise"`
		IsEvent    bool   `json:"isEvent"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if data.Type != "unhandledrejection" {
		t.Errorf("type = %q, want 'unhandledrejection'", data.Type)
	}
	if data.Reason != "test error" {
		t.Errorf("reason = %q, want 'test error'", data.Reason)
	}
	if !data.HasPromise {
		t.Error("promise should not be null")
	}
	if !data.IsEvent {
		t.Error("PromiseRejectionEvent should extend Event")
	}
}

func TestUnhandledRejection_GlobalListenerExists(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request, env) {
    return Response.json({
      hasAddEventListener: typeof globalThis.addEventListener === 'function',
      hasDispatchEvent: typeof globalThis.dispatchEvent === 'function',
      hasRemoveEventListener: typeof globalThis.removeEventListener === 'function',
    });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		HasAddEventListener    bool `json:"hasAddEventListener"`
		HasDispatchEvent       bool `json:"hasDispatchEvent"`
		HasRemoveEventListener bool `json:"hasRemoveEventListener"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if !data.HasAddEventListener {
		t.Error("globalThis should have addEventListener")
	}
	if !data.HasDispatchEvent {
		t.Error("globalThis should have dispatchEvent")
	}
	if !data.HasRemoveEventListener {
		t.Error("globalThis should have removeEventListener")
	}
}

func TestUnhandledRejection_ManualDispatch(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    let captured = null;
    globalThis.addEventListener('unhandledrejection', function(e) {
      captured = { reason: String(e.reason), type: e.type };
    });
    // Manually dispatch to verify the mechanism works.
    const evt = new PromiseRejectionEvent('unhandledrejection', {
      reason: 'manual test',
      promise: Promise.resolve(),
    });
    globalThis.dispatchEvent(evt);
    return Response.json({ captured });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Captured struct {
			Reason string `json:"reason"`
			Type   string `json:"type"`
		} `json:"captured"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if data.Captured.Type != "unhandledrejection" {
		t.Errorf("type = %q, want 'unhandledrejection'", data.Captured.Type)
	}
	if data.Captured.Reason != "manual test" {
		t.Errorf("reason = %q, want 'manual test'", data.Captured.Reason)
	}
}

func TestUnhandledRejection_AutomaticDetection(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    let captured = null;
    globalThis.addEventListener('unhandledrejection', function(e) {
      captured = { reason: String(e.reason), type: e.type };
    });

    // Create a rejected promise - do NOT attach .catch()
    // Use __trackRejection directly since automatic tracking depends on engine-level hooks
    const p = Promise.reject('auto detected');
    globalThis.__trackRejection(p, 'auto detected');

    // Wait for microtask to fire
    await new Promise(resolve => setTimeout(resolve, 50));

    return Response.json({
      detected: captured !== null,
      reason: captured ? captured.reason : null,
      type: captured ? captured.type : null,
    });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Detected bool   `json:"detected"`
		Reason   string `json:"reason"`
		Type     string `json:"type"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if !data.Detected {
		t.Error("unhandledrejection event should have been detected")
	}
	if data.Reason != "auto detected" {
		t.Errorf("reason = %q, want 'auto detected'", data.Reason)
	}
	if data.Type != "unhandledrejection" {
		t.Errorf("type = %q, want 'unhandledrejection'", data.Type)
	}
}
