package worker

import (
	"encoding/json"
	"testing"
)

// --- AbortSignal.any tests ---

func TestAbortSignalAny_ComposesSignals(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request, env) {
    const c1 = new AbortController();
    const c2 = new AbortController();
    const combined = AbortSignal.any([c1.signal, c2.signal]);
    const beforeAborted = combined.aborted;
    c1.abort("from c1");
    const afterAborted = combined.aborted;
    const reason = combined.reason;
    return Response.json({ beforeAborted, afterAborted, reason });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		BeforeAborted bool   `json:"beforeAborted"`
		AfterAborted  bool   `json:"afterAborted"`
		Reason        string `json:"reason"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if data.BeforeAborted {
		t.Error("combined signal should not be aborted before any source aborts")
	}
	if !data.AfterAborted {
		t.Error("combined signal should be aborted after c1 aborts")
	}
	if data.Reason != "from c1" {
		t.Errorf("reason = %q, want 'from c1'", data.Reason)
	}
}

func TestAbortSignalAny_AlreadyAborted(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request, env) {
    const preAborted = AbortSignal.abort("pre");
    const c2 = new AbortController();
    const combined = AbortSignal.any([preAborted, c2.signal]);
    return Response.json({
      aborted: combined.aborted,
      reason: combined.reason,
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
		t.Error("combined signal should be immediately aborted when input is already aborted")
	}
	if data.Reason != "pre" {
		t.Errorf("reason = %q, want 'pre'", data.Reason)
	}
}

func TestAbortSignalAny_ReasonPropagation(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request, env) {
    const c1 = new AbortController();
    const c2 = new AbortController();
    const combined = AbortSignal.any([c1.signal, c2.signal]);
    c2.abort("from c2");
    return Response.json({
      aborted: combined.aborted,
      reason: combined.reason,
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
		t.Error("combined should be aborted")
	}
	if data.Reason != "from c2" {
		t.Errorf("reason = %q, want 'from c2'", data.Reason)
	}
}

func TestAbortSignalAny_ListenerFires(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request, env) {
    const c1 = new AbortController();
    const combined = AbortSignal.any([c1.signal]);
    let fired = false;
    combined.addEventListener('abort', () => { fired = true; });
    c1.abort();
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
		t.Error("abort listener on combined signal should fire")
	}
}

// --- Extended console tests ---

func TestConsoleExt_TimeAndTimeEnd(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    console.time('test');
    await new Promise(r => setTimeout(r, 10));
    console.timeEnd('test');
    return new Response('ok');
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	found := false
	for _, log := range r.Logs {
		if len(log.Message) > 5 && log.Message[:5] == "test:" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected log with 'test: Xms', got logs: %v", r.Logs)
	}
}

func TestConsoleExt_Count(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request, env) {
    console.count('myLabel');
    console.count('myLabel');
    console.count('myLabel');
    console.countReset('myLabel');
    console.count('myLabel');
    return new Response('ok');
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	expected := []string{"myLabel: 1", "myLabel: 2", "myLabel: 3", "myLabel: 1"}
	if len(r.Logs) < len(expected) {
		t.Fatalf("expected at least %d logs, got %d: %v", len(expected), len(r.Logs), r.Logs)
	}
	for i, exp := range expected {
		if r.Logs[i].Message != exp {
			t.Errorf("log[%d] = %q, want %q", i, r.Logs[i].Message, exp)
		}
	}
}

func TestConsoleExt_Assert(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request, env) {
    console.assert(true, 'should not appear');
    console.assert(false, 'assertion message');
    console.assert(false);
    return new Response('ok');
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	errorCount := 0
	for _, log := range r.Logs {
		if log.Level == "error" {
			errorCount++
		}
	}
	if errorCount != 2 {
		t.Errorf("expected 2 error logs for failed assertions, got %d: %v", errorCount, r.Logs)
	}
}

func TestConsoleExt_Table(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request, env) {
    console.table({ a: 1, b: 2 });
    return new Response('ok');
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	if len(r.Logs) < 1 {
		t.Fatal("expected at least 1 log for console.table")
	}
	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(r.Logs[0].Message), &parsed); err != nil {
		t.Errorf("console.table output should be valid JSON, got %q", r.Logs[0].Message)
	}
}

func TestConsoleExt_Trace(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request, env) {
    console.trace('hello', 'world');
    return new Response('ok');
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	if len(r.Logs) < 1 {
		t.Fatal("expected at least 1 log for console.trace")
	}
	if r.Logs[0].Message != "Trace: hello world" {
		t.Errorf("trace log = %q, want 'Trace: hello world'", r.Logs[0].Message)
	}
}

// --- Blob.stream() / Blob.bytes() tests ---

func TestBlobStream_ReadsToCompletion(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    const blob = new Blob(['hello ', 'world']);
    const stream = blob.stream();
    const reader = stream.getReader();
    let result = '';
    while (true) {
      const { value, done } = await reader.read();
      if (done) break;
      result += new TextDecoder().decode(value);
    }
    return Response.json({ result, size: blob.size });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Result string `json:"result"`
		Size   int    `json:"size"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if data.Result != "hello world" {
		t.Errorf("result = %q, want 'hello world'", data.Result)
	}
	if data.Size != 11 {
		t.Errorf("size = %d, want 11", data.Size)
	}
}

func TestBlobBytes_ReturnsUint8Array(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    const blob = new Blob(['test']);
    const bytes = await blob.bytes();
    return Response.json({
      isUint8Array: bytes instanceof Uint8Array,
      length: bytes.length,
      first: bytes[0],
      second: bytes[1],
      third: bytes[2],
      fourth: bytes[3],
    });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		IsUint8Array bool `json:"isUint8Array"`
		Length       int  `json:"length"`
		First        int  `json:"first"`
		Second       int  `json:"second"`
		Third        int  `json:"third"`
		Fourth       int  `json:"fourth"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !data.IsUint8Array {
		t.Error("bytes() should return a Uint8Array")
	}
	if data.Length != 4 {
		t.Errorf("length = %d, want 4", data.Length)
	}
	// 't' = 116, 'e' = 101, 's' = 115, 't' = 116
	if data.First != 116 || data.Second != 101 || data.Third != 115 || data.Fourth != 116 {
		t.Errorf("bytes = [%d,%d,%d,%d], want [116,101,115,116]", data.First, data.Second, data.Third, data.Fourth)
	}
}

// --- Queuing strategy tests ---

func TestQueuingStrategy_ByteLength(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request, env) {
    const strategy = new ByteLengthQueuingStrategy({ highWaterMark: 1024 });
    const chunk = new Uint8Array(42);
    return Response.json({
      highWaterMark: strategy.highWaterMark,
      size: strategy.size(chunk),
    });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		HighWaterMark int `json:"highWaterMark"`
		Size          int `json:"size"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if data.HighWaterMark != 1024 {
		t.Errorf("highWaterMark = %d, want 1024", data.HighWaterMark)
	}
	if data.Size != 42 {
		t.Errorf("size = %d, want 42", data.Size)
	}
}

func TestQueuingStrategy_Count(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request, env) {
    const strategy = new CountQueuingStrategy({ highWaterMark: 10 });
    return Response.json({
      highWaterMark: strategy.highWaterMark,
      sizeString: strategy.size("hello"),
      sizeObject: strategy.size({ foo: 'bar' }),
      sizeArray: strategy.size([1, 2, 3]),
    });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		HighWaterMark int `json:"highWaterMark"`
		SizeString    int `json:"sizeString"`
		SizeObject    int `json:"sizeObject"`
		SizeArray     int `json:"sizeArray"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if data.HighWaterMark != 10 {
		t.Errorf("highWaterMark = %d, want 10", data.HighWaterMark)
	}
	if data.SizeString != 1 || data.SizeObject != 1 || data.SizeArray != 1 {
		t.Errorf("CountQueuingStrategy.size() should always return 1, got string=%d, object=%d, array=%d",
			data.SizeString, data.SizeObject, data.SizeArray)
	}
}

// --- reportError tests ---

func TestReportError_DispatchesErrorEvent(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request, env) {
    let captured = null;
    globalThis.addEventListener('error', (ev) => {
      captured = {
        type: ev.type,
        message: ev.message,
        hasError: ev.error !== null && ev.error !== undefined,
        errorMessage: ev.error ? ev.error.message : '',
        isErrorEvent: ev instanceof ErrorEvent,
      };
    });
    reportError(new Error('test error'));
    return Response.json(captured);
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Type         string `json:"type"`
		Message      string `json:"message"`
		HasError     bool   `json:"hasError"`
		ErrorMessage string `json:"errorMessage"`
		IsErrorEvent bool   `json:"isErrorEvent"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if data.Type != "error" {
		t.Errorf("event type = %q, want 'error'", data.Type)
	}
	if data.Message != "test error" {
		t.Errorf("message = %q, want 'test error'", data.Message)
	}
	if !data.HasError {
		t.Error("event.error should be set")
	}
	if data.ErrorMessage != "test error" {
		t.Errorf("error.message = %q, want 'test error'", data.ErrorMessage)
	}
	if !data.IsErrorEvent {
		t.Error("event should be an instance of ErrorEvent")
	}
}

func TestReportError_StringError(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request, env) {
    let captured = null;
    globalThis.addEventListener('error', (ev) => {
      captured = {
        message: ev.message,
        error: ev.error,
      };
    });
    reportError("string error");
    return Response.json(captured);
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Message string `json:"message"`
		Error   string `json:"error"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if data.Message != "string error" {
		t.Errorf("message = %q, want 'string error'", data.Message)
	}
	if data.Error != "string error" {
		t.Errorf("error = %q, want 'string error'", data.Error)
	}
}

func TestReportError_ErrorEventProperties(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request, env) {
    const ev = new ErrorEvent('error', {
      message: 'test',
      filename: 'worker.js',
      lineno: 42,
      colno: 7,
      error: new TypeError('type error'),
    });
    return Response.json({
      type: ev.type,
      message: ev.message,
      filename: ev.filename,
      lineno: ev.lineno,
      colno: ev.colno,
      isEvent: ev instanceof Event,
      isErrorEvent: ev instanceof ErrorEvent,
      errorMsg: ev.error.message,
    });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Type         string `json:"type"`
		Message      string `json:"message"`
		Filename     string `json:"filename"`
		Lineno       int    `json:"lineno"`
		Colno        int    `json:"colno"`
		IsEvent      bool   `json:"isEvent"`
		IsErrorEvent bool   `json:"isErrorEvent"`
		ErrorMsg     string `json:"errorMsg"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if data.Type != "error" {
		t.Errorf("type = %q", data.Type)
	}
	if data.Message != "test" {
		t.Errorf("message = %q", data.Message)
	}
	if data.Filename != "worker.js" {
		t.Errorf("filename = %q", data.Filename)
	}
	if data.Lineno != 42 {
		t.Errorf("lineno = %d", data.Lineno)
	}
	if data.Colno != 7 {
		t.Errorf("colno = %d", data.Colno)
	}
	if !data.IsEvent {
		t.Error("ErrorEvent should be an instance of Event")
	}
	if !data.IsErrorEvent {
		t.Error("should be instance of ErrorEvent")
	}
	if data.ErrorMsg != "type error" {
		t.Errorf("error.message = %q", data.ErrorMsg)
	}
}
