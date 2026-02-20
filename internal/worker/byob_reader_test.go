package worker

import (
	"encoding/json"
	"testing"
)

func TestBYOB_ReaderBasicRead(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    const stream = new ReadableStream({
      type: 'bytes',
      start(controller) {
        controller.enqueue(new Uint8Array([1, 2, 3, 4, 5]));
        controller.close();
      },
    });

    const reader = stream.getReader({ mode: 'byob' });
    const buffer = new Uint8Array(10);
    const { value, done } = await reader.read(buffer);
    return Response.json({
      length: value.length,
      bytes: Array.from(value),
      done: done,
    });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Length int   `json:"length"`
		Bytes  []int `json:"bytes"`
		Done   bool  `json:"done"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if data.Length != 5 {
		t.Errorf("length = %d, want 5", data.Length)
	}
	if data.Done {
		t.Error("done should be false for first read with data")
	}
	expected := []int{1, 2, 3, 4, 5}
	for i, v := range expected {
		if i >= len(data.Bytes) || data.Bytes[i] != v {
			t.Errorf("byte[%d] = %d, want %d", i, data.Bytes[i], v)
		}
	}
}

func TestBYOB_BytesTypeDetected(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request, env) {
    const stream = new ReadableStream({
      type: 'bytes',
      start(controller) {
        controller.close();
      },
    });
    // A bytes stream should allow getting a BYOB reader.
    let byobOk = false;
    try {
      const reader = stream.getReader({ mode: 'byob' });
      byobOk = true;
      reader.releaseLock();
    } catch(e) {
      byobOk = false;
    }

    // A normal stream should NOT allow BYOB reader.
    const normalStream = new ReadableStream({
      start(controller) {
        controller.close();
      },
    });
    let normalByobOk = true;
    try {
      normalStream.getReader({ mode: 'byob' });
    } catch(e) {
      normalByobOk = false;
    }

    return Response.json({ byobOk, normalByobOk });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		ByobOk       bool `json:"byobOk"`
		NormalByobOk bool `json:"normalByobOk"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if !data.ByobOk {
		t.Error("bytes stream should support getReader({mode: 'byob'})")
	}
	if data.NormalByobOk {
		t.Error("normal stream should NOT support getReader({mode: 'byob'})")
	}
}

func TestBYOB_ReleaseLock(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request, env) {
    const stream = new ReadableStream({
      type: 'bytes',
      start(controller) {
        controller.close();
      },
    });
    const reader = stream.getReader({ mode: 'byob' });
    const lockedBefore = stream.locked;
    reader.releaseLock();
    const lockedAfter = stream.locked;
    return Response.json({ lockedBefore, lockedAfter });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		LockedBefore bool `json:"lockedBefore"`
		LockedAfter  bool `json:"lockedAfter"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if !data.LockedBefore {
		t.Error("stream should be locked after getReader()")
	}
	if data.LockedAfter {
		t.Error("stream should be unlocked after releaseLock()")
	}
}

func TestBYOB_ReadAfterClose(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    const stream = new ReadableStream({
      type: 'bytes',
      start(controller) {
        controller.enqueue(new Uint8Array([10, 20]));
        controller.close();
      },
    });

    const reader = stream.getReader({ mode: 'byob' });
    // First read should get data.
    const first = await reader.read(new Uint8Array(10));
    // Second read should signal done.
    const second = await reader.read(new Uint8Array(10));
    return Response.json({
      firstDone: first.done,
      firstLen: first.value.length,
      secondDone: second.done,
      secondLen: second.value.length,
    });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		FirstDone  bool `json:"firstDone"`
		FirstLen   int  `json:"firstLen"`
		SecondDone bool `json:"secondDone"`
		SecondLen  int  `json:"secondLen"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if data.FirstDone {
		t.Error("first read should not be done")
	}
	if data.FirstLen != 2 {
		t.Errorf("first read length = %d, want 2", data.FirstLen)
	}
	if !data.SecondDone {
		t.Error("second read should be done")
	}
	if data.SecondLen != 0 {
		t.Errorf("second read length = %d, want 0", data.SecondLen)
	}
}

func TestBYOB_AsyncPull(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    const stream = new ReadableStream({
      type: 'bytes',
      pull(controller) {
        // Simulate async data source
        controller.enqueue(new Uint8Array([42, 43, 44]));
      },
    });
    const reader = stream.getReader({ mode: 'byob' });
    const { value, done } = await reader.read(new Uint8Array(10));
    return Response.json({ length: value.length, first: value[0], done });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Length int  `json:"length"`
		First  int  `json:"first"`
		Done   bool `json:"done"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if data.Length != 3 {
		t.Errorf("length = %d, want 3", data.Length)
	}
	if data.First != 42 {
		t.Errorf("first = %d, want 42", data.First)
	}
	if data.Done {
		t.Error("done should be false")
	}
}

func TestBYOB_ControllerError(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    let ctrl;
    const stream = new ReadableStream({
      type: 'bytes',
      start(controller) { ctrl = controller; },
    });
    const reader = stream.getReader({ mode: 'byob' });
    // Start a read that will be pending
    const readPromise = reader.read(new Uint8Array(10));
    // Error the stream
    ctrl.error(new Error('test error'));
    try {
      await readPromise;
      return Response.json({ caught: false });
    } catch(e) {
      return Response.json({ caught: true, message: e.message || String(e) });
    }
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Caught  bool   `json:"caught"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if !data.Caught {
		t.Error("should have caught an error from errored stream")
	}
	if data.Message == "" {
		t.Error("error message should not be empty")
	}
	// message should contain "test error"
	if len(data.Message) > 0 {
		found := false
		msg := data.Message
		target := "test error"
		if len(msg) >= len(target) {
			for i := 0; i <= len(msg)-len(target); i++ {
				if msg[i:i+len(target)] == target {
					found = true
					break
				}
			}
		}
		if !found {
			t.Errorf("message = %q, want it to contain 'test error'", data.Message)
		}
	}
}

func TestBYOB_PartialRead(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    const stream = new ReadableStream({
      type: 'bytes',
      start(controller) {
        controller.enqueue(new Uint8Array([1, 2, 3, 4, 5, 6, 7, 8]));
        controller.close();
      },
    });
    const reader = stream.getReader({ mode: 'byob' });
    // Read with a 3-byte view (smaller than 8 bytes enqueued)
    const first = await reader.read(new Uint8Array(3));
    return Response.json({
      firstLen: first.value.length,
      firstBytes: Array.from(first.value),
      firstDone: first.done,
    });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		FirstLen   int   `json:"firstLen"`
		FirstBytes []int `json:"firstBytes"`
		FirstDone  bool  `json:"firstDone"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if data.FirstLen != 3 {
		t.Errorf("firstLen = %d, want 3 (capped to view size)", data.FirstLen)
	}
	expected := []int{1, 2, 3}
	for i, v := range expected {
		if i >= len(data.FirstBytes) || data.FirstBytes[i] != v {
			t.Errorf("firstBytes[%d] = %d, want %d", i, data.FirstBytes[i], v)
		}
	}
	if data.FirstDone {
		t.Error("firstDone should be false")
	}
}

func TestBYOB_ReaderCancel(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    const stream = new ReadableStream({
      type: 'bytes',
      start(controller) {
        controller.enqueue(new Uint8Array([1, 2, 3]));
      },
    });
    const reader = stream.getReader({ mode: 'byob' });
    await reader.cancel('done with it');
    // After cancel, stream should be in a terminal state
    return Response.json({ cancelled: true });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Cancelled bool `json:"cancelled"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if !data.Cancelled {
		t.Error("cancelled should be true")
	}
}
