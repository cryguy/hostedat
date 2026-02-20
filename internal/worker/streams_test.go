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

func TestStreams_WritableStreamAbort(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    let abortReason = null;
    const stream = new WritableStream({
      abort(reason) { abortReason = reason; },
    });
    const writer = stream.getWriter();
    await writer.abort("cancelled");
    return Response.json({ abortReason });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		AbortReason string `json:"abortReason"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if data.AbortReason != "cancelled" {
		t.Errorf("abortReason = %q, want 'cancelled'", data.AbortReason)
	}
}

func TestStreams_WritableStreamLocked(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request, env) {
    const stream = new WritableStream();
    const writer = stream.getWriter();
    let threw = false;
    try {
      stream.getWriter();
    } catch(e) {
      threw = true;
    }
    return Response.json({ threw, locked: stream.locked });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Threw  bool `json:"threw"`
		Locked bool `json:"locked"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatal(err)
	}
	if !data.Threw {
		t.Error("second getWriter should throw")
	}
	if !data.Locked {
		t.Error("stream should be locked")
	}
}

func TestStreams_WriterReleaseLock(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request, env) {
    const stream = new WritableStream();
    const w1 = stream.getWriter();
    const locked1 = stream.locked;
    w1.releaseLock();
    const locked2 = stream.locked;
    // Should be able to get a new writer after release.
    const w2 = stream.getWriter();
    const locked3 = stream.locked;
    return Response.json({ locked1, locked2, locked3 });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Locked1 bool `json:"locked1"`
		Locked2 bool `json:"locked2"`
		Locked3 bool `json:"locked3"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatal(err)
	}
	if !data.Locked1 {
		t.Error("should be locked after getWriter")
	}
	if data.Locked2 {
		t.Error("should be unlocked after releaseLock")
	}
	if !data.Locked3 {
		t.Error("should be locked again after second getWriter")
	}
}

func TestStreams_ReaderReleaseLock(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request, env) {
    const stream = new ReadableStream();
    const r1 = stream.getReader();
    r1.releaseLock();
    const r2 = stream.getReader();
    return Response.json({ locked: stream.locked });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Locked bool `json:"locked"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatal(err)
	}
	if !data.Locked {
		t.Error("stream should be locked after second getReader")
	}
}

func TestStreams_ReadableStreamAsyncIterator(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    const stream = new ReadableStream({
      start(controller) {
        controller.enqueue("a");
        controller.enqueue("b");
        controller.enqueue("c");
        controller.close();
      }
    });
    let result = '';
    for await (const chunk of stream) {
      result += chunk;
    }
    return new Response(result);
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	if string(r.Response.Body) != "abc" {
		t.Errorf("body = %q, want 'abc'", r.Response.Body)
	}
}

func TestStreams_TransformStreamFlush(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    let count = 0;
    const ts = new TransformStream({
      transform(chunk, controller) {
        count++;
        controller.enqueue(chunk);
      },
      flush(controller) {
        controller.enqueue("flush:" + count);
      }
    });

    const writer = ts.writable.getWriter();
    const reader = ts.readable.getReader();

    writer.write("a");
    writer.write("b");
    writer.close();

    let result = '';
    while (true) {
      const { value, done } = await reader.read();
      if (done) break;
      result += value + ",";
    }
    return new Response(result);
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	if string(r.Response.Body) != "a,b,flush:2," {
		t.Errorf("body = %q, want 'a,b,flush:2,'", r.Response.Body)
	}
}

func TestStreams_ReadableStreamWithPull(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    let pullCount = 0;
    const stream = new ReadableStream({
      pull(controller) {
        pullCount++;
        if (pullCount <= 3) {
          controller.enqueue("chunk" + pullCount);
        } else {
          controller.close();
        }
      }
    });
    const reader = stream.getReader();
    let result = '';
    while (true) {
      const { value, done } = await reader.read();
      if (done) break;
      result += value + ",";
    }
    return Response.json({ result, pullCount });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Result    string `json:"result"`
		PullCount int    `json:"pullCount"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatal(err)
	}
	if data.Result != "chunk1,chunk2,chunk3," {
		t.Errorf("result = %q", data.Result)
	}
}

func TestStreams_WriterClosedPromise(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    const stream = new WritableStream();
    const writer = stream.getWriter();
    const p1 = writer.closed;
    const p2 = writer.closed;
    const same = p1 === p2;
    await writer.close();
    return Response.json({ same });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Same bool `json:"same"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatal(err)
	}
	if !data.Same {
		t.Error("writer.closed should return the same promise")
	}
}

func TestStreams_ReadableStreamError(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    const stream = new ReadableStream({
      start(controller) {
        controller.error("stream broken");
      }
    });
    let caught = false;
    let msg = "";
    try {
      const reader = stream.getReader();
      await reader.read();
    } catch(e) {
      caught = true;
      msg = String(e);
    }
    return Response.json({ caught, msg });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Caught bool   `json:"caught"`
		Msg    string `json:"msg"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatal(err)
	}
	if !data.Caught {
		t.Error("reading from errored stream should throw")
	}
}

func TestStreams_TransformStreamWithTransformAndFlush(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    let count = 0;
    const ts = new TransformStream({
      transform(chunk, controller) {
        count++;
        controller.enqueue("[" + chunk + "]");
      },
      flush(controller) {
        controller.enqueue("done:" + count);
      }
    });

    const writer = ts.writable.getWriter();
    const reader = ts.readable.getReader();

    writer.write("x");
    writer.write("y");
    writer.close();

    let result = '';
    while (true) {
      const { value, done } = await reader.read();
      if (done) break;
      result += value + ",";
    }
    return Response.json({ result });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Result string `json:"result"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatal(err)
	}
	if data.Result != "[x],[y],done:2," {
		t.Errorf("result = %q, want '[x],[y],done:2,'", data.Result)
	}
}

func TestReadableStreamFrom(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	t.Run("from sync iterable array", func(t *testing.T) {
		source := `export default {
  async fetch(request, env) {
    const stream = ReadableStream.from([1, 2, 3]);
    const reader = stream.getReader();
    const chunks = [];
    while (true) {
      const { value, done } = await reader.read();
      if (done) break;
      chunks.push(value);
    }
    return Response.json({ chunks });
  },
};`
		r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
		assertOK(t, r)

		var data struct {
			Chunks []int `json:"chunks"`
		}
		if err := json.Unmarshal(r.Response.Body, &data); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if len(data.Chunks) != 3 || data.Chunks[0] != 1 || data.Chunks[1] != 2 || data.Chunks[2] != 3 {
			t.Errorf("chunks = %v, want [1,2,3]", data.Chunks)
		}
	})

	t.Run("from async iterable generator", func(t *testing.T) {
		source := `export default {
  async fetch(request, env) {
    async function* gen() {
      yield "a";
      yield "b";
      yield "c";
    }
    const stream = ReadableStream.from(gen());
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

		if string(r.Response.Body) != "abc" {
			t.Errorf("body = %q, want 'abc'", r.Response.Body)
		}
	})

	t.Run("from null throws", func(t *testing.T) {
		source := `export default {
  fetch(request, env) {
    let caught = false;
    try {
      ReadableStream.from(null);
    } catch(e) {
      caught = true;
    }
    return Response.json({ caught });
  },
};`
		r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
		assertOK(t, r)

		var data struct {
			Caught bool `json:"caught"`
		}
		if err := json.Unmarshal(r.Response.Body, &data); err != nil {
			t.Fatal(err)
		}
		if !data.Caught {
			t.Error("ReadableStream.from(null) should throw")
		}
	})

	t.Run("from non-iterable throws", func(t *testing.T) {
		source := `export default {
  fetch(request, env) {
    let caught = false;
    try {
      ReadableStream.from(42);
    } catch(e) {
      caught = true;
    }
    return Response.json({ caught });
  },
};`
		r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
		assertOK(t, r)

		var data struct {
			Caught bool `json:"caught"`
		}
		if err := json.Unmarshal(r.Response.Body, &data); err != nil {
			t.Fatal(err)
		}
		if !data.Caught {
			t.Error("ReadableStream.from(42) should throw")
		}
	})
}

func TestFixedLengthStream(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	t.Run("exact length passes through", func(t *testing.T) {
		source := `export default {
  async fetch(request, env) {
    const fls = new FixedLengthStream(5);
    const writer = fls.writable.getWriter();
    const reader = fls.readable.getReader();

    writer.write("hello");
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

		if string(r.Response.Body) != "hello" {
			t.Errorf("body = %q, want 'hello'", r.Response.Body)
		}
	})

	t.Run("exceeding length errors", func(t *testing.T) {
		source := `export default {
  async fetch(request, env) {
    const fls = new FixedLengthStream(3);
    const writer = fls.writable.getWriter();

    let caught = false;
    let msg = "";
    try {
      await writer.write("hello");
    } catch(e) {
      caught = true;
      msg = e.message || String(e);
    }
    return Response.json({ caught, msg });
  },
};`
		r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
		assertOK(t, r)

		var data struct {
			Caught bool   `json:"caught"`
			Msg    string `json:"msg"`
		}
		if err := json.Unmarshal(r.Response.Body, &data); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if !data.Caught {
			t.Error("writing beyond expected length should error")
		}
	})

	t.Run("under length errors on close", func(t *testing.T) {
		source := `export default {
  async fetch(request, env) {
    const fls = new FixedLengthStream(10);
    const writer = fls.writable.getWriter();
    const reader = fls.readable.getReader();

    await writer.write("hi");

    let caught = false;
    let msg = "";
    try {
      await writer.close();
      // Drain to trigger the flush
      while (true) {
        const { done } = await reader.read();
        if (done) break;
      }
    } catch(e) {
      caught = true;
      msg = e.message || String(e);
    }
    return Response.json({ caught, msg });
  },
};`
		r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
		assertOK(t, r)

		var data struct {
			Caught bool   `json:"caught"`
			Msg    string `json:"msg"`
		}
		if err := json.Unmarshal(r.Response.Body, &data); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if !data.Caught {
			t.Error("closing with fewer bytes than expected should error")
		}
	})

	t.Run("zero length empty write", func(t *testing.T) {
		source := `export default {
  async fetch(request, env) {
    const fls = new FixedLengthStream(0);
    const writer = fls.writable.getWriter();
    const reader = fls.readable.getReader();

    writer.close();

    const { done } = await reader.read();
    return Response.json({ done });
  },
};`
		r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
		assertOK(t, r)

		var data struct {
			Done bool `json:"done"`
		}
		if err := json.Unmarshal(r.Response.Body, &data); err != nil {
			t.Fatal(err)
		}
		if !data.Done {
			t.Error("zero-length stream should close immediately")
		}
	})

	t.Run("invalid constructor throws", func(t *testing.T) {
		source := `export default {
  fetch(request, env) {
    let caught = false;
    try {
      new FixedLengthStream(-1);
    } catch(e) {
      caught = true;
    }
    return Response.json({ caught });
  },
};`
		r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
		assertOK(t, r)

		var data struct {
			Caught bool `json:"caught"`
		}
		if err := json.Unmarshal(r.Response.Body, &data); err != nil {
			t.Fatal(err)
		}
		if !data.Caught {
			t.Error("FixedLengthStream(-1) should throw")
		}
	})
}

func TestReadableStreamFrom_EmptyIterable(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    const stream = ReadableStream.from([]);
    const reader = stream.getReader();
    const { done } = await reader.read();
    return Response.json({ done });
  },
};`
	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Done bool `json:"done"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatal(err)
	}
	if !data.Done {
		t.Error("empty iterable should produce a stream that is immediately done")
	}
}

func TestReadableStreamFrom_SetIterable(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    const s = new Set(["a", "b", "c"]);
    const stream = ReadableStream.from(s);
    const reader = stream.getReader();
    const chunks = [];
    while (true) {
      const { value, done } = await reader.read();
      if (done) break;
      chunks.push(value);
    }
    return Response.json({ chunks });
  },
};`
	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Chunks []string `json:"chunks"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatal(err)
	}
	if len(data.Chunks) != 3 || data.Chunks[0] != "a" || data.Chunks[1] != "b" || data.Chunks[2] != "c" {
		t.Errorf("chunks = %v, want [a b c]", data.Chunks)
	}
}

func TestReadableStreamFrom_IteratorThrows(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    const iterable = {
      [Symbol.iterator]() {
        let i = 0;
        return {
          next() {
            if (i++ === 0) return { value: "first", done: false };
            throw new Error("iterator failure");
          }
        };
      }
    };
    const stream = ReadableStream.from(iterable);
    const reader = stream.getReader();
    const first = await reader.read();
    let caught = false;
    try {
      await reader.read();
    } catch(e) {
      caught = true;
    }
    return Response.json({ firstValue: first.value, caught });
  },
};`
	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		FirstValue string `json:"firstValue"`
		Caught     bool   `json:"caught"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatal(err)
	}
	if data.FirstValue != "first" {
		t.Errorf("firstValue = %q, want 'first'", data.FirstValue)
	}
	if !data.Caught {
		t.Error("iterator error should propagate to reader")
	}
}

func TestFixedLengthStream_MultipleWrites(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    const fls = new FixedLengthStream(11);
    const writer = fls.writable.getWriter();
    const reader = fls.readable.getReader();

    writer.write("hello");
    writer.write(" ");
    writer.write("world");
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

	if string(r.Response.Body) != "hello world" {
		t.Errorf("body = %q, want 'hello world'", r.Response.Body)
	}
}

func TestFixedLengthStream_BinaryData(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    const fls = new FixedLengthStream(8);
    const writer = fls.writable.getWriter();
    const reader = fls.readable.getReader();

    const chunk1 = new Uint8Array([1, 2, 3]);
    const chunk2 = new Uint8Array([4, 5, 6, 7, 8]);
    await writer.write(chunk1);
    await writer.write(chunk2);
    writer.close();

    const chunks = [];
    while (true) {
      const { value, done } = await reader.read();
      if (done) break;
      chunks.push(value.length);
    }
    return Response.json({ chunkLengths: chunks, total: chunks.reduce((a,b) => a+b, 0) });
  },
};`
	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		ChunkLengths []int `json:"chunkLengths"`
		Total        int   `json:"total"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatal(err)
	}
	if data.Total != 8 {
		t.Errorf("total = %d, want 8", data.Total)
	}
}

func TestFixedLengthStream_BoundaryOverflow(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    const fls = new FixedLengthStream(5);
    const writer = fls.writable.getWriter();

    // Write exactly 5 bytes (OK)
    await writer.write("hello");

    // Write 1 more byte (should fail)
    let caught = false;
    try {
      await writer.write("!");
    } catch(e) {
      caught = true;
    }
    return Response.json({ caught });
  },
};`
	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Caught bool `json:"caught"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatal(err)
	}
	if !data.Caught {
		t.Error("writing past exact boundary should error")
	}
}

func TestStreams_PipeTo(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	t.Run("basic pipe", func(t *testing.T) {
		source := `export default {
  async fetch(request, env) {
    const chunks = [];
    const readable = new ReadableStream({
      start(controller) {
        controller.enqueue("hello");
        controller.enqueue(" world");
        controller.close();
      }
    });
    const writable = new WritableStream({
      write(chunk) { chunks.push(chunk); },
    });
    await readable.pipeTo(writable);
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
		if len(data.Chunks) >= 1 && data.Chunks[0] != "hello" {
			t.Errorf("chunks[0] = %q, want 'hello'", data.Chunks[0])
		}
		if len(data.Chunks) >= 2 && data.Chunks[1] != " world" {
			t.Errorf("chunks[1] = %q, want ' world'", data.Chunks[1])
		}
	})

	t.Run("locked stream rejects", func(t *testing.T) {
		source := `export default {
  async fetch(request, env) {
    const readable = new ReadableStream();
    readable.getReader(); // lock it
    let caught = false;
    try {
      await readable.pipeTo(new WritableStream());
    } catch(e) {
      caught = true;
    }
    return Response.json({ caught });
  },
};`
		r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
		assertOK(t, r)

		var data struct {
			Caught bool `json:"caught"`
		}
		if err := json.Unmarshal(r.Response.Body, &data); err != nil {
			t.Fatal(err)
		}
		if !data.Caught {
			t.Error("pipeTo on locked stream should reject")
		}
	})

	t.Run("preventClose option", func(t *testing.T) {
		source := `export default {
  async fetch(request, env) {
    const readable = new ReadableStream({
      start(controller) {
        controller.enqueue("data");
        controller.close();
      }
    });
    let writableClosed = false;
    const writable = new WritableStream({
      close() { writableClosed = true; },
    });
    await readable.pipeTo(writable, { preventClose: true });
    return Response.json({ writableClosed });
  },
};`
		r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
		assertOK(t, r)

		var data struct {
			WritableClosed bool `json:"writableClosed"`
		}
		if err := json.Unmarshal(r.Response.Body, &data); err != nil {
			t.Fatal(err)
		}
		if data.WritableClosed {
			t.Error("writable should not be closed when preventClose is true")
		}
	})
}

func TestStreams_PipeThrough(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	t.Run("uppercase transform", func(t *testing.T) {
		source := `export default {
  async fetch(request, env) {
    const readable = new ReadableStream({
      start(controller) {
        controller.enqueue("hello");
        controller.enqueue(" world");
        controller.close();
      }
    });
    const ts = new TransformStream({
      transform(chunk, controller) {
        controller.enqueue(chunk.toUpperCase());
      }
    });
    const transformed = readable.pipeThrough(ts);
    const reader = transformed.getReader();
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
	})

	t.Run("locked stream throws", func(t *testing.T) {
		source := `export default {
  fetch(request, env) {
    const readable = new ReadableStream();
    readable.getReader(); // lock it
    let caught = false;
    try {
      readable.pipeThrough(new TransformStream());
    } catch(e) {
      caught = true;
    }
    return Response.json({ caught });
  },
};`
		r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
		assertOK(t, r)

		var data struct {
			Caught bool `json:"caught"`
		}
		if err := json.Unmarshal(r.Response.Body, &data); err != nil {
			t.Fatal(err)
		}
		if !data.Caught {
			t.Error("pipeThrough on locked stream should throw")
		}
	})
}

func TestStreams_Tee(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	t.Run("both branches get same data", func(t *testing.T) {
		source := `export default {
  async fetch(request, env) {
    const readable = new ReadableStream({
      start(controller) {
        controller.enqueue("a");
        controller.enqueue("b");
        controller.enqueue("c");
        controller.close();
      }
    });
    const [branch1, branch2] = readable.tee();
    const reader1 = branch1.getReader();
    const reader2 = branch2.getReader();
    let result1 = '';
    let result2 = '';
    while (true) {
      const { value, done } = await reader1.read();
      if (done) break;
      result1 += value;
    }
    while (true) {
      const { value, done } = await reader2.read();
      if (done) break;
      result2 += value;
    }
    return Response.json({ result1, result2, same: result1 === result2 });
  },
};`
		r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
		assertOK(t, r)

		var data struct {
			Result1 string `json:"result1"`
			Result2 string `json:"result2"`
			Same    bool   `json:"same"`
		}
		if err := json.Unmarshal(r.Response.Body, &data); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if data.Result1 != "abc" {
			t.Errorf("result1 = %q, want 'abc'", data.Result1)
		}
		if data.Result2 != "abc" {
			t.Errorf("result2 = %q, want 'abc'", data.Result2)
		}
		if !data.Same {
			t.Error("both branches should produce the same data")
		}
	})

	t.Run("tee returns array of two streams", func(t *testing.T) {
		source := `export default {
  fetch(request, env) {
    const readable = new ReadableStream();
    const branches = readable.tee();
    return Response.json({
      isArray: Array.isArray(branches),
      length: branches.length,
      areDifferent: branches[0] !== branches[1],
    });
  },
};`
		r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
		assertOK(t, r)

		var data struct {
			IsArray      bool `json:"isArray"`
			Length       int  `json:"length"`
			AreDifferent bool `json:"areDifferent"`
		}
		if err := json.Unmarshal(r.Response.Body, &data); err != nil {
			t.Fatal(err)
		}
		if !data.IsArray {
			t.Error("tee should return an array")
		}
		if data.Length != 2 {
			t.Errorf("tee should return 2 branches, got %d", data.Length)
		}
		if !data.AreDifferent {
			t.Error("tee branches should be different objects")
		}
	})

	t.Run("locked stream throws", func(t *testing.T) {
		source := `export default {
  fetch(request, env) {
    const readable = new ReadableStream();
    readable.getReader(); // lock it
    let caught = false;
    try {
      readable.tee();
    } catch(e) {
      caught = true;
    }
    return Response.json({ caught });
  },
};`
		r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
		assertOK(t, r)

		var data struct {
			Caught bool `json:"caught"`
		}
		if err := json.Unmarshal(r.Response.Body, &data); err != nil {
			t.Fatal(err)
		}
		if !data.Caught {
			t.Error("tee on locked stream should throw")
		}
	})
}

func TestRequestBody_ReturnsReadableStream(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    const req = new Request("http://example.com", { method: "POST", body: "hello world" });
    const body = req.body;
    const isStream = body instanceof ReadableStream;
    const reader = body.getReader();
    let result = '';
    while (true) {
      const { value, done } = await reader.read();
      if (done) break;
      result += new TextDecoder().decode(value);
    }
    return Response.json({ isStream, result });
  },
};`
	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		IsStream bool   `json:"isStream"`
		Result   string `json:"result"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !data.IsStream {
		t.Error("Request.body should return a ReadableStream")
	}
	if data.Result != "hello world" {
		t.Errorf("body content = %q, want 'hello world'", data.Result)
	}
}

func TestResponseBody_ReturnsReadableStream(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    const resp = new Response("test data");
    const body = resp.body;
    const isStream = body instanceof ReadableStream;
    const reader = body.getReader();
    let result = '';
    while (true) {
      const { value, done } = await reader.read();
      if (done) break;
      result += new TextDecoder().decode(value);
    }
    return Response.json({ isStream, result });
  },
};`
	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		IsStream bool   `json:"isStream"`
		Result   string `json:"result"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !data.IsStream {
		t.Error("Response.body should return a ReadableStream")
	}
	if data.Result != "test data" {
		t.Errorf("body content = %q, want 'test data'", data.Result)
	}
}

func TestBody_NullForNullBody(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request, env) {
    const req = new Request("http://example.com");
    const resp = new Response(null);
    return Response.json({
      reqBodyNull: req.body === null,
      respBodyNull: resp.body === null,
    });
  },
};`
	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		ReqBodyNull  bool `json:"reqBodyNull"`
		RespBodyNull bool `json:"respBodyNull"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatal(err)
	}
	if !data.ReqBodyNull {
		t.Error("Request.body should be null for no body")
	}
	if !data.RespBodyNull {
		t.Error("Response.body should be null for null body")
	}
}

func TestBody_Idempotent(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request, env) {
    const req = new Request("http://example.com", { method: "POST", body: "data" });
    const body1 = req.body;
    const body2 = req.body;
    return Response.json({ same: body1 === body2 });
  },
};`
	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Same bool `json:"same"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatal(err)
	}
	if !data.Same {
		t.Error("Request.body should return the same ReadableStream on multiple accesses")
	}
}

func TestBodyUsed_Tracking(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    const req = new Request("http://example.com", { method: "POST", body: "data" });
    const beforeAccess = req.bodyUsed;
    const body = req.body;
    const afterAccess = req.bodyUsed;
    const reader = body.getReader();
    const afterLock = req.bodyUsed;
    while (true) {
      const { done } = await reader.read();
      if (done) break;
    }
    const afterRead = req.bodyUsed;
    return Response.json({ beforeAccess, afterAccess, afterLock, afterRead });
  },
};`
	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		BeforeAccess bool `json:"beforeAccess"`
		AfterAccess  bool `json:"afterAccess"`
		AfterLock    bool `json:"afterLock"`
		AfterRead    bool `json:"afterRead"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatal(err)
	}
	if data.BeforeAccess {
		t.Error("bodyUsed should be false before accessing body")
	}
	if data.AfterAccess {
		t.Error("bodyUsed should be false after just accessing .body (not locked yet)")
	}
	if !data.AfterLock {
		t.Error("bodyUsed should be true after getReader() locks the stream")
	}
	if !data.AfterRead {
		t.Error("bodyUsed should be true after reading the body")
	}
}

func TestFetch_BinaryRequestBodyExtraction(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	t.Run("ArrayBuffer body is base64 encoded", func(t *testing.T) {
		source := `export default {
  async fetch(request, env) {
    // Simulate what fetch extraction does with an ArrayBuffer body
    const buf = new Uint8Array([0, 1, 2, 255, 254, 253]).buffer;
    const req = new Request("http://example.com", { method: "POST", body: buf });
    // Verify the body can be base64 encoded
    const b64 = __bufferSourceToB64(new Uint8Array(buf));
    const roundtrip = __b64ToBuffer(b64);
    const arr = new Uint8Array(roundtrip);
    return Response.json({
      b64Length: b64.length,
      roundtripLength: arr.length,
      byte0: arr[0],
      byte3: arr[3],
      byte5: arr[5],
    });
  },
};`
		r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
		assertOK(t, r)

		var data struct {
			B64Length       int `json:"b64Length"`
			RoundtripLength int `json:"roundtripLength"`
			Byte0           int `json:"byte0"`
			Byte3           int `json:"byte3"`
			Byte5           int `json:"byte5"`
		}
		if err := json.Unmarshal(r.Response.Body, &data); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if data.RoundtripLength != 6 {
			t.Errorf("roundtrip length = %d, want 6", data.RoundtripLength)
		}
		if data.Byte0 != 0 {
			t.Errorf("byte0 = %d, want 0", data.Byte0)
		}
		if data.Byte3 != 255 {
			t.Errorf("byte3 = %d, want 255", data.Byte3)
		}
		if data.Byte5 != 253 {
			t.Errorf("byte5 = %d, want 253", data.Byte5)
		}
	})

	t.Run("TypedArray body is base64 encoded", func(t *testing.T) {
		source := `export default {
  async fetch(request, env) {
    const arr = new Uint8Array([72, 101, 108, 108, 111]);
    const b64 = __bufferSourceToB64(arr);
    const roundtrip = new Uint8Array(__b64ToBuffer(b64));
    let result = '';
    for (let i = 0; i < roundtrip.length; i++) result += String.fromCharCode(roundtrip[i]);
    return Response.json({ result, length: roundtrip.length });
  },
};`
		r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
		assertOK(t, r)

		var data struct {
			Result string `json:"result"`
			Length int    `json:"length"`
		}
		if err := json.Unmarshal(r.Response.Body, &data); err != nil {
			t.Fatal(err)
		}
		if data.Result != "Hello" {
			t.Errorf("result = %q, want 'Hello'", data.Result)
		}
		if data.Length != 5 {
			t.Errorf("length = %d, want 5", data.Length)
		}
	})

	t.Run("ReadableStream body extracted as base64", func(t *testing.T) {
		source := `export default {
  async fetch(request, env) {
    const stream = new ReadableStream({
      start(controller) {
        controller.enqueue(new Uint8Array([1, 2, 3]));
        controller.enqueue(new Uint8Array([4, 5]));
        controller.close();
      }
    });
    // Simulate extraction: read queue, collect bytes, base64-encode
    var chunks = [];
    for (var i = 0; i < stream._queue.length; i++) {
      var c = stream._queue[i];
      if (c instanceof Uint8Array || ArrayBuffer.isView(c)) {
        var arr = new Uint8Array(c.buffer || c, c.byteOffset || 0, c.byteLength || c.length);
        for (var j = 0; j < arr.length; j++) chunks.push(arr[j]);
      }
    }
    var combined = new Uint8Array(chunks);
    var b64 = __bufferSourceToB64(combined);
    var rt = new Uint8Array(__b64ToBuffer(b64));
    return Response.json({
      length: rt.length,
      bytes: Array.from(rt),
    });
  },
};`
		r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
		assertOK(t, r)

		var data struct {
			Length int   `json:"length"`
			Bytes  []int `json:"bytes"`
		}
		if err := json.Unmarshal(r.Response.Body, &data); err != nil {
			t.Fatal(err)
		}
		if data.Length != 5 {
			t.Errorf("length = %d, want 5", data.Length)
		}
		expected := []int{1, 2, 3, 4, 5}
		for i, b := range expected {
			if i < len(data.Bytes) && data.Bytes[i] != b {
				t.Errorf("bytes[%d] = %d, want %d", i, data.Bytes[i], b)
			}
		}
	})
}

func TestStreams_WritableStreamReady(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    const stream = new WritableStream();
    const writer = stream.getWriter();
    const ready = await writer.ready;
    return Response.json({ readyUndefined: ready === undefined });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		ReadyUndefined bool `json:"readyUndefined"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatal(err)
	}
	if !data.ReadyUndefined {
		t.Error("writer.ready should resolve to undefined")
	}
}
