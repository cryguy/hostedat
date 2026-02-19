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

func TestAbort_EventOnceListener(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request, env) {
    const target = new EventTarget();
    let count = 0;
    target.addEventListener('test', () => { count++; }, { once: true });
    target.dispatchEvent(new Event('test'));
    target.dispatchEvent(new Event('test'));
    target.dispatchEvent(new Event('test'));
    return Response.json({ count });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Count int `json:"count"`
	}
	json.Unmarshal(r.Response.Body, &data)
	if data.Count != 1 {
		t.Errorf("once listener count = %d, want 1", data.Count)
	}
}

func TestAbort_EventPreventDefault(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request, env) {
    const event = new Event('test', { cancelable: true });
    const beforePD = event.defaultPrevented;
    event.preventDefault();
    const afterPD = event.defaultPrevented;

    const nonCancelable = new Event('test2');
    nonCancelable.preventDefault();
    const ncPD = nonCancelable.defaultPrevented;

    return Response.json({ beforePD, afterPD, ncPD });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		BeforePD bool `json:"beforePD"`
		AfterPD  bool `json:"afterPD"`
		NcPD     bool `json:"ncPD"`
	}
	json.Unmarshal(r.Response.Body, &data)
	if data.BeforePD {
		t.Error("defaultPrevented should be false before preventDefault()")
	}
	if !data.AfterPD {
		t.Error("defaultPrevented should be true after preventDefault() on cancelable event")
	}
	if data.NcPD {
		t.Error("defaultPrevented should stay false on non-cancelable event")
	}
}

func TestAbort_SignalAbortDefaultReason(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request, env) {
    const signal = AbortSignal.abort();
    return Response.json({
      aborted: signal.aborted,
      isDOMException: signal.reason instanceof DOMException,
      reasonName: signal.reason ? signal.reason.name : "",
    });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Aborted        bool   `json:"aborted"`
		IsDOMException bool   `json:"isDOMException"`
		ReasonName     string `json:"reasonName"`
	}
	json.Unmarshal(r.Response.Body, &data)
	if !data.Aborted {
		t.Error("AbortSignal.abort() should be aborted")
	}
	if !data.IsDOMException {
		t.Error("default reason should be a DOMException")
	}
	if data.ReasonName != "AbortError" {
		t.Errorf("reason name = %q, want AbortError", data.ReasonName)
	}
}

func TestAbort_DoubleAbortIsNoop(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request, env) {
    const controller = new AbortController();
    let count = 0;
    controller.signal.addEventListener('abort', () => { count++; });
    controller.abort("first");
    controller.abort("second");
    return Response.json({
      count,
      reason: controller.signal.reason,
    });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Count  int    `json:"count"`
		Reason string `json:"reason"`
	}
	json.Unmarshal(r.Response.Body, &data)
	if data.Count != 1 {
		t.Errorf("abort event should only fire once, count = %d", data.Count)
	}
	if data.Reason != "first" {
		t.Errorf("reason should be first abort reason, got %q", data.Reason)
	}
}

func TestAbort_DispatchEventReturnValue(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request, env) {
    const target = new EventTarget();
    const event1 = new Event('test');
    const result1 = target.dispatchEvent(event1);

    const target2 = new EventTarget();
    const event2 = new Event('test', { cancelable: true });
    target2.addEventListener('test', (e) => { e.preventDefault(); });
    const result2 = target2.dispatchEvent(event2);

    return Response.json({ result1, result2 });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Result1 bool `json:"result1"`
		Result2 bool `json:"result2"`
	}
	json.Unmarshal(r.Response.Body, &data)
	if !data.Result1 {
		t.Error("dispatchEvent should return true when not prevented")
	}
	if data.Result2 {
		t.Error("dispatchEvent should return false when preventDefault called")
	}
}

func TestAbort_SignalTimeout(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    const signal = AbortSignal.timeout(10);
    const beforeAborted = signal.aborted;
    // Wait long enough for the timeout to fire.
    await new Promise(r => setTimeout(r, 50));
    return Response.json({
      beforeAborted,
      afterAborted: signal.aborted,
      reasonName: signal.reason ? signal.reason.name : "",
    });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		BeforeAborted bool   `json:"beforeAborted"`
		AfterAborted  bool   `json:"afterAborted"`
		ReasonName    string `json:"reasonName"`
	}
	json.Unmarshal(r.Response.Body, &data)
	if data.BeforeAborted {
		t.Error("signal should not be aborted before timeout fires")
	}
	if !data.AfterAborted {
		t.Error("signal should be aborted after timeout fires")
	}
	if data.ReasonName != "TimeoutError" {
		t.Errorf("reason name = %q, want TimeoutError", data.ReasonName)
	}
}

func TestAbort_SignalTimeoutListenerFires(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    const signal = AbortSignal.timeout(5);
    let fired = false;
    signal.addEventListener('abort', () => { fired = true; });
    await new Promise(r => setTimeout(r, 50));
    return Response.json({ fired });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Fired bool `json:"fired"`
	}
	json.Unmarshal(r.Response.Body, &data)
	if !data.Fired {
		t.Error("abort listener should fire after timeout")
	}
}
