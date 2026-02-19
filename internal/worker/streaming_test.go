package worker

import (
	"encoding/json"
	"testing"
)

func TestStreaming_ReadableStreamResponseBody(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    const stream = new ReadableStream({
      start(controller) {
        controller.enqueue(new TextEncoder().encode("Hello "));
        controller.enqueue(new TextEncoder().encode("streaming "));
        controller.enqueue(new TextEncoder().encode("world!"));
        controller.close();
      }
    });
    return new Response(stream, {
      status: 200,
      headers: { "content-type": "text/plain" },
    });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	body := string(r.Response.Body)
	if body != "Hello streaming world!" {
		t.Errorf("streaming body = %q, want %q", body, "Hello streaming world!")
	}
	if r.Response.Headers["content-type"] != "text/plain" {
		t.Errorf("content-type = %q, want 'text/plain'", r.Response.Headers["content-type"])
	}
}

func TestStreaming_ReadableStreamSingleChunk(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    const stream = new ReadableStream({
      start(controller) {
        controller.enqueue(new TextEncoder().encode("single chunk"));
        controller.close();
      }
    });
    return new Response(stream);
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	body := string(r.Response.Body)
	if body != "single chunk" {
		t.Errorf("body = %q, want 'single chunk'", body)
	}
}

func TestStreaming_ReadableStreamBinaryData(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    const stream = new ReadableStream({
      start(controller) {
        // Write binary data with bytes 0-255
        const data = new Uint8Array(256);
        for (let i = 0; i < 256; i++) data[i] = i;
        controller.enqueue(data);
        controller.close();
      }
    });
    return new Response(stream, {
      headers: { "content-type": "application/octet-stream" },
    });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	if len(r.Response.Body) != 256 {
		t.Fatalf("body length = %d, want 256", len(r.Response.Body))
	}
	for i := 0; i < 256; i++ {
		if r.Response.Body[i] != byte(i) {
			t.Errorf("byte[%d] = %d, want %d", i, r.Response.Body[i], i)
			break
		}
	}
}

func TestStreaming_ReadableStreamWithStringChunks(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    const stream = new ReadableStream({
      start(controller) {
        controller.enqueue("string ");
        controller.enqueue("chunks ");
        controller.enqueue("work");
        controller.close();
      }
    });
    return new Response(stream);
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	body := string(r.Response.Body)
	if body != "string chunks work" {
		t.Errorf("body = %q, want 'string chunks work'", body)
	}
}

func TestStreaming_ReadableStreamEmpty(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    const stream = new ReadableStream({
      start(controller) {
        controller.close();
      }
    });
    return new Response(stream);
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	if len(r.Response.Body) != 0 {
		t.Errorf("empty stream body length = %d, want 0", len(r.Response.Body))
	}
}

func TestStreaming_ArrayBufferResponseBody(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    const data = new TextEncoder().encode("arraybuffer body");
    return new Response(data.buffer, {
      headers: { "content-type": "application/octet-stream" },
    });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	body := string(r.Response.Body)
	if body != "arraybuffer body" {
		t.Errorf("body = %q, want 'arraybuffer body'", body)
	}
}

func TestStreaming_TypedArrayResponseBody(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    const data = new TextEncoder().encode("typed array body");
    return new Response(data, {
      headers: { "content-type": "text/plain" },
    });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	body := string(r.Response.Body)
	if body != "typed array body" {
		t.Errorf("body = %q, want 'typed array body'", body)
	}
}

func TestStreaming_ReadableStreamWithJSON(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    const jsonData = JSON.stringify({ message: "streamed JSON", count: 42 });
    const stream = new ReadableStream({
      start(controller) {
        controller.enqueue(new TextEncoder().encode(jsonData));
        controller.close();
      }
    });
    return new Response(stream, {
      headers: { "content-type": "application/json" },
    });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Message string `json:"message"`
		Count   int    `json:"count"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if data.Message != "streamed JSON" {
		t.Errorf("message = %q, want 'streamed JSON'", data.Message)
	}
	if data.Count != 42 {
		t.Errorf("count = %d, want 42", data.Count)
	}
}

func TestStreaming_MixedChunkTypes(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    const stream = new ReadableStream({
      start(controller) {
        // Mix string and Uint8Array chunks
        controller.enqueue("Hello ");
        controller.enqueue(new TextEncoder().encode("binary "));
        controller.enqueue("world");
        controller.close();
      }
    });
    return new Response(stream);
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	body := string(r.Response.Body)
	if body != "Hello binary world" {
		t.Errorf("mixed chunk body = %q, want 'Hello binary world'", body)
	}
}

func TestStreaming_NonOKStatusWithStream(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    const stream = new ReadableStream({
      start(controller) {
        controller.enqueue(new TextEncoder().encode('{"error":"not found"}'));
        controller.close();
      }
    });
    return new Response(stream, {
      status: 404,
      headers: { "content-type": "application/json" },
    });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	if r.Response.StatusCode != 404 {
		t.Errorf("status = %d, want 404", r.Response.StatusCode)
	}

	var data struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if data.Error != "not found" {
		t.Errorf("error = %q, want 'not found'", data.Error)
	}
}
