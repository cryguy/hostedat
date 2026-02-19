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
	json.Unmarshal(r.Response.Body, &data)
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
	json.Unmarshal(r.Response.Body, &data)
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
	json.Unmarshal(r.Response.Body, &data)
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
	json.Unmarshal(r.Response.Body, &data)
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
	json.Unmarshal(r.Response.Body, &data)
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
	json.Unmarshal(r.Response.Body, &data)
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
	json.Unmarshal(r.Response.Body, &data)
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
		json.Unmarshal(r.Response.Body, &data)
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
		json.Unmarshal(r.Response.Body, &data)
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
		json.Unmarshal(r.Response.Body, &data)
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
		json.Unmarshal(r.Response.Body, &data)
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
	json.Unmarshal(r.Response.Body, &data)
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
	json.Unmarshal(r.Response.Body, &data)
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
	json.Unmarshal(r.Response.Body, &data)
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
	json.Unmarshal(r.Response.Body, &data)
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
	json.Unmarshal(r.Response.Body, &data)
	if !data.Caught {
		t.Error("writing past exact boundary should error")
	}
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
	json.Unmarshal(r.Response.Body, &data)
	if !data.ReadyUndefined {
		t.Error("writer.ready should resolve to undefined")
	}
}
