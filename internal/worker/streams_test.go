package worker

import (
	"encoding/json"
	"testing"
)

func TestStreams_ReadableStreamBasic(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    const stream = new ReadableStream({
      start(controller) {
        controller.enqueue("hello");
        controller.enqueue(" ");
        controller.enqueue("world");
        controller.close();
      }
    });

    const reader = stream.getReader();
    let result = '';
    while (true) {
      const { value, done } = await reader.read();
      if (done) break;
      result += value;
    }
    return new Response(result);
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	if string(r.Response.Body) != "hello world" {
		t.Errorf("body = %q, want 'hello world'", r.Response.Body)
	}
}

func TestStreams_ReadableStreamLocked(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request, env) {
    const stream = new ReadableStream();
    const reader = stream.getReader();
    try {
      stream.getReader();
      return new Response("should not reach");
    } catch(e) {
      return Response.json({ error: e.message, locked: stream.locked });
    }
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Error  string `json:"error"`
		Locked bool   `json:"locked"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if !data.Locked {
		t.Error("stream should be locked after getReader()")
	}
}

func TestStreams_WritableStreamBasic(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    const chunks = [];
    const stream = new WritableStream({
      write(chunk) { chunks.push(chunk); },
    });

    const writer = stream.getWriter();
    await writer.write("chunk1");
    await writer.write("chunk2");
    await writer.close();

    return Response.json({ chunks, count: chunks.length });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Chunks []string `json:"chunks"`
		Count  int      `json:"count"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if data.Count != 2 {
		t.Errorf("count = %d, want 2", data.Count)
	}
	if len(data.Chunks) >= 1 && data.Chunks[0] != "chunk1" {
		t.Errorf("chunks[0] = %q", data.Chunks[0])
	}
	if len(data.Chunks) >= 2 && data.Chunks[1] != "chunk2" {
		t.Errorf("chunks[1] = %q", data.Chunks[1])
	}
}

func TestStreams_TransformStreamIdentity(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    // Identity transform (no transform function).
    const ts = new TransformStream();
    const writer = ts.writable.getWriter();
    const reader = ts.readable.getReader();

    writer.write("a");
    writer.write("b");
    writer.close();

    let result = '';
    while (true) {
      const { value, done } = await reader.read();
      if (done) break;
      result += value;
    }
    return new Response(result);
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	if string(r.Response.Body) != "ab" {
		t.Errorf("body = %q, want 'ab'", r.Response.Body)
	}
}

func TestStreams_TransformStreamUpperCase(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    const ts = new TransformStream({
      transform(chunk, controller) {
        controller.enqueue(chunk.toUpperCase());
      }
    });

    const writer = ts.writable.getWriter();
    const reader = ts.readable.getReader();

    writer.write("hello");
    writer.write(" world");
    writer.close();

    let result = '';
    while (true) {
      const { value, done } = await reader.read();
      if (done) break;
      result += value;
    }
    return new Response(result);
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	if string(r.Response.Body) != "HELLO WORLD" {
		t.Errorf("body = %q, want 'HELLO WORLD'", r.Response.Body)
	}
}

func TestStreams_ReadableStreamCancel(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    let cancelReason = null;
    const stream = new ReadableStream({
      cancel(reason) { cancelReason = reason; }
    });
    await stream.cancel("done reading");
    return Response.json({ cancelReason });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		CancelReason string `json:"cancelReason"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if data.CancelReason != "done reading" {
		t.Errorf("cancelReason = %q", data.CancelReason)
	}
}

func TestStreams_ReaderClosedPromiseIdentity(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    const stream = new ReadableStream({
      start(controller) {
        controller.enqueue("data");
        controller.close();
      }
    });
    const reader = stream.getReader();
    const p1 = reader.closed;
    const p2 = reader.closed;
    const same = p1 === p2;
    await p1;
    return Response.json({ same });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Same bool `json:"same"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if !data.Same {
		t.Error("reader.closed should return the same promise on each access")
	}
}
