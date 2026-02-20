package worker

import (
	"encoding/json"
	"testing"
)

func TestTextEncoderStream_Basic(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    const stream = new TextEncoderStream();
    const writer = stream.writable.getWriter();
    writer.write("Hello");
    writer.write(" World");
    writer.close();

    const reader = stream.readable.getReader();
    const chunks = [];
    while (true) {
      const { done, value } = await reader.read();
      if (done) break;
      chunks.push(value);
    }

    // Each write should produce one Uint8Array chunk
    const allBytes = [];
    for (const c of chunks) {
      for (let i = 0; i < c.length; i++) allBytes.push(c[i]);
    }
    const result = new TextDecoder().decode(new Uint8Array(allBytes));
    return Response.json({
      result,
      chunkCount: chunks.length,
      isUint8Array: chunks[0] instanceof Uint8Array,
    });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Result       string `json:"result"`
		ChunkCount   int    `json:"chunkCount"`
		IsUint8Array bool   `json:"isUint8Array"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if data.Result != "Hello World" {
		t.Errorf("result = %q, want %q", data.Result, "Hello World")
	}
	if data.ChunkCount != 2 {
		t.Errorf("chunkCount = %d, want 2", data.ChunkCount)
	}
	if !data.IsUint8Array {
		t.Error("chunks should be Uint8Array instances")
	}
}

func TestTextEncoderStream_Encoding(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    const stream = new TextEncoderStream();
    return Response.json({ encoding: stream.encoding });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Encoding string `json:"encoding"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if data.Encoding != "utf-8" {
		t.Errorf("encoding = %q, want %q", data.Encoding, "utf-8")
	}
}

func TestTextEncoderStream_NonStringThrows(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    const stream = new TextEncoderStream();
    const writer = stream.writable.getWriter();
    let threw = false;
    try {
      await writer.write(new Uint8Array([1,2,3]));
    } catch(e) {
      threw = true;
    }
    return Response.json({ threw });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Threw bool `json:"threw"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !data.Threw {
		t.Error("TextEncoderStream should throw on non-string input")
	}
}

func TestTextDecoderStream_Basic(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    const stream = new TextDecoderStream();
    const writer = stream.writable.getWriter();
    writer.write(new TextEncoder().encode("Hello"));
    writer.write(new TextEncoder().encode(" World"));
    writer.close();

    const reader = stream.readable.getReader();
    const chunks = [];
    while (true) {
      const { done, value } = await reader.read();
      if (done) break;
      chunks.push(value);
    }

    const result = chunks.join('');
    return Response.json({
      result,
      chunkCount: chunks.length,
      isString: typeof chunks[0] === 'string',
    });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Result     string `json:"result"`
		ChunkCount int    `json:"chunkCount"`
		IsString   bool   `json:"isString"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if data.Result != "Hello World" {
		t.Errorf("result = %q, want %q", data.Result, "Hello World")
	}
	if data.ChunkCount != 2 {
		t.Errorf("chunkCount = %d, want 2", data.ChunkCount)
	}
	if !data.IsString {
		t.Error("chunks should be strings")
	}
}

func TestTextDecoderStream_Properties(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    const stream = new TextDecoderStream();
    return Response.json({
      encoding: stream.encoding,
      fatal: stream.fatal,
      ignoreBOM: stream.ignoreBOM,
    });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Encoding  string `json:"encoding"`
		Fatal     bool   `json:"fatal"`
		IgnoreBOM bool   `json:"ignoreBOM"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if data.Encoding != "utf-8" {
		t.Errorf("encoding = %q, want %q", data.Encoding, "utf-8")
	}
	if data.Fatal {
		t.Error("fatal should be false by default")
	}
	if data.IgnoreBOM {
		t.Error("ignoreBOM should be false by default")
	}
}

func TestTextDecoderStream_NonBufferSourceThrows(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    const stream = new TextDecoderStream();
    const writer = stream.writable.getWriter();
    let threw = false;
    try {
      await writer.write("not a buffer");
    } catch(e) {
      threw = true;
    }
    return Response.json({ threw });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Threw bool `json:"threw"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !data.Threw {
		t.Error("TextDecoderStream should throw on non-BufferSource input")
	}
}

func TestIdentityTransformStream_Basic(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    const stream = new IdentityTransformStream();
    const writer = stream.writable.getWriter();
    writer.write("hello");
    writer.write(42);
    writer.write(new Uint8Array([1,2,3]));
    writer.close();

    const reader = stream.readable.getReader();
    const chunks = [];
    while (true) {
      const { done, value } = await reader.read();
      if (done) break;
      chunks.push(value);
    }

    return Response.json({
      count: chunks.length,
      first: chunks[0],
      second: chunks[1],
      thirdLen: chunks[2].length,
    });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Count    int    `json:"count"`
		First    string `json:"first"`
		Second   int    `json:"second"`
		ThirdLen int    `json:"thirdLen"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if data.Count != 3 {
		t.Errorf("count = %d, want 3", data.Count)
	}
	if data.First != "hello" {
		t.Errorf("first = %q, want %q", data.First, "hello")
	}
	if data.Second != 42 {
		t.Errorf("second = %d, want 42", data.Second)
	}
	if data.ThirdLen != 3 {
		t.Errorf("thirdLen = %d, want 3", data.ThirdLen)
	}
}

func TestTextEncoderStream_PipeThrough(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    // Create a ReadableStream of strings and pipe through TextEncoderStream
    const input = new ReadableStream({
      start(controller) {
        controller.enqueue("Hello, ");
        controller.enqueue("streaming ");
        controller.enqueue("world!");
        controller.close();
      }
    });

    const encoded = input.pipeThrough(new TextEncoderStream());
    const reader = encoded.getReader();
    const chunks = [];
    while (true) {
      const { done, value } = await reader.read();
      if (done) break;
      chunks.push(value);
    }

    const allBytes = [];
    for (const c of chunks) {
      for (let i = 0; i < c.length; i++) allBytes.push(c[i]);
    }
    const result = new TextDecoder().decode(new Uint8Array(allBytes));
    return Response.json({ result, chunkCount: chunks.length });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Result     string `json:"result"`
		ChunkCount int    `json:"chunkCount"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if data.Result != "Hello, streaming world!" {
		t.Errorf("result = %q, want %q", data.Result, "Hello, streaming world!")
	}
	if data.ChunkCount != 3 {
		t.Errorf("chunkCount = %d, want 3", data.ChunkCount)
	}
}

func TestTextStreams_RoundTrip(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    // Pipe strings through encoder then decoder to get strings back
    const input = new ReadableStream({
      start(controller) {
        controller.enqueue("Round");
        controller.enqueue("Trip");
        controller.close();
      }
    });

    const encoded = input.pipeThrough(new TextEncoderStream());
    const decoded = encoded.pipeThrough(new TextDecoderStream());
    const reader = decoded.getReader();
    const chunks = [];
    while (true) {
      const { done, value } = await reader.read();
      if (done) break;
      chunks.push(value);
    }

    return Response.json({ result: chunks.join(''), chunkCount: chunks.length });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Result     string `json:"result"`
		ChunkCount int    `json:"chunkCount"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if data.Result != "RoundTrip" {
		t.Errorf("result = %q, want %q", data.Result, "RoundTrip")
	}
}
