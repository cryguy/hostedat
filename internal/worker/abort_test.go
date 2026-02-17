package worker

import (
	"encoding/json"
	"testing"
)

func TestAbort_ControllerAbortSetsSignal(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request, env) {
    const controller = new AbortController();
    const before = controller.signal.aborted;
    controller.abort();
    const after = controller.signal.aborted;
    return Response.json({ before, after });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Before bool `json:"before"`
		After  bool `json:"after"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if data.Before {
		t.Error("signal.aborted should be false before abort()")
	}
	if !data.After {
		t.Error("signal.aborted should be true after abort()")
	}
}

func TestAbort_ListenerFires(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request, env) {
    const controller = new AbortController();
    let fired = false;
    controller.signal.addEventListener('abort', () => { fired = true; });
    controller.abort();
    return Response.json({ fired });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Fired bool `json:"fired"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if !data.Fired {
		t.Error("abort event listener should have fired")
	}
}

func TestAbort_AbortReason(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request, env) {
    const controller = new AbortController();
    controller.abort("custom reason");
    return Response.json({
      reason: controller.signal.reason,
      aborted: controller.signal.aborted,
    });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Reason  string `json:"reason"`
		Aborted bool   `json:"aborted"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if data.Reason != "custom reason" {
		t.Errorf("reason = %q, want 'custom reason'", data.Reason)
	}
	if !data.Aborted {
		t.Error("signal should be aborted")
	}
}

func TestAbort_SignalAbortStatic(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request, env) {
    const signal = AbortSignal.abort("pre-aborted");
    return Response.json({
      aborted: signal.aborted,
      reason: signal.reason,
    });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Aborted bool   `json:"aborted"`
		Reason  string `json:"reason"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if !data.Aborted {
		t.Error("AbortSignal.abort() should return aborted signal")
	}
	if data.Reason != "pre-aborted" {
		t.Errorf("reason = %q", data.Reason)
	}
}

func TestAbort_ThrowIfAborted(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request, env) {
    const controller = new AbortController();
    controller.abort("stopped");
    try {
      controller.signal.throwIfAborted();
      return new Response("should not reach");
    } catch(e) {
      return Response.json({ caught: true, reason: String(e) });
    }
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Caught bool `json:"caught"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if !data.Caught {
		t.Error("throwIfAborted should throw when signal is aborted")
	}
}

func TestAbort_EventTargetBasics(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request, env) {
    const target = new EventTarget();
    let count = 0;
    const handler = () => { count++; };
    target.addEventListener('test', handler);
    target.dispatchEvent(new Event('test'));
    target.dispatchEvent(new Event('test'));
    target.removeEventListener('test', handler);
    target.dispatchEvent(new Event('test'));
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
		t.Errorf("count = %d, want 2 (listener removed before 3rd dispatch)", data.Count)
	}
}

func TestAbort_DOMException(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request, env) {
    const err = new DOMException("test message", "TestError");
    return Response.json({
      message: err.message,
      name: err.name,
      isError: err instanceof Error,
    });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Message string `json:"message"`
		Name    string `json:"name"`
		IsError bool   `json:"isError"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if data.Message != "test message" {
		t.Errorf("message = %q", data.Message)
	}
	if data.Name != "TestError" {
		t.Errorf("name = %q", data.Name)
	}
	if !data.IsError {
		t.Error("DOMException should extend Error")
	}
}
