package worker

import (
	"encoding/json"
	"testing"
)

func TestDecompression_LimitConstantExists(t *testing.T) {
	if maxDecompressedSize < 1*1024*1024 {
		t.Errorf("maxDecompressedSize = %d, too small", maxDecompressedSize)
	}
	if maxDecompressedSize > 512*1024*1024 {
		t.Errorf("maxDecompressedSize = %d, too large", maxDecompressedSize)
	}
}

func TestCompression_GzipRoundTrip(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    const original = "Hello, this is a compression test with some repeated content. " +
      "Hello, this is a compression test with some repeated content. " +
      "Hello, this is a compression test with some repeated content.";

    // Compress
    const cs = new CompressionStream("gzip");
    const writer = cs.writable.getWriter();
    writer.write(new TextEncoder().encode(original));
    writer.close();
    const compressedChunks = [];
    const reader = cs.readable.getReader();
    while (true) {
      const { done, value } = await reader.read();
      if (done) break;
      compressedChunks.push(value);
    }
    let compressedLen = 0;
    for (const c of compressedChunks) compressedLen += c.length;
    const compressed = new Uint8Array(compressedLen);
    let offset = 0;
    for (const c of compressedChunks) { compressed.set(c, offset); offset += c.length; }

    // Decompress
    const ds = new DecompressionStream("gzip");
    const dwriter = ds.writable.getWriter();
    dwriter.write(compressed);
    dwriter.close();
    const decompressedChunks = [];
    const dreader = ds.readable.getReader();
    while (true) {
      const { done, value } = await dreader.read();
      if (done) break;
      decompressedChunks.push(value);
    }
    let decompressedLen = 0;
    for (const c of decompressedChunks) decompressedLen += c.length;
    const decompressed = new Uint8Array(decompressedLen);
    offset = 0;
    for (const c of decompressedChunks) { decompressed.set(c, offset); offset += c.length; }

    const result = new TextDecoder().decode(decompressed);
    return Response.json({
      match: result === original,
      originalLen: original.length,
      compressedLen: compressed.length,
      smallerAfterCompress: compressed.length < original.length,
    });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Match                bool `json:"match"`
		OriginalLen          int  `json:"originalLen"`
		CompressedLen        int  `json:"compressedLen"`
		SmallerAfterCompress bool `json:"smallerAfterCompress"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if !data.Match {
		t.Error("gzip round-trip should return the original string")
	}
	if !data.SmallerAfterCompress {
		t.Errorf("compressed (%d) should be smaller than original (%d)", data.CompressedLen, data.OriginalLen)
	}
}

func TestCompression_DeflateRoundTrip(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    const original = "Deflate compression test data. Repeated for size. " +
      "Deflate compression test data. Repeated for size. " +
      "Deflate compression test data. Repeated for size.";

    const cs = new CompressionStream("deflate");
    const writer = cs.writable.getWriter();
    writer.write(new TextEncoder().encode(original));
    writer.close();
    const chunks = [];
    const reader = cs.readable.getReader();
    while (true) {
      const { done, value } = await reader.read();
      if (done) break;
      chunks.push(value);
    }
    let compressedLen = 0;
    for (const c of chunks) compressedLen += c.length;
    const compressed = new Uint8Array(compressedLen);
    let offset = 0;
    for (const c of chunks) { compressed.set(c, offset); offset += c.length; }

    const ds = new DecompressionStream("deflate");
    const dwriter = ds.writable.getWriter();
    dwriter.write(compressed);
    dwriter.close();
    const dchunks = [];
    const dreader = ds.readable.getReader();
    while (true) {
      const { done, value } = await dreader.read();
      if (done) break;
      dchunks.push(value);
    }
    let decompressedLen = 0;
    for (const c of dchunks) decompressedLen += c.length;
    const decompressed = new Uint8Array(decompressedLen);
    offset = 0;
    for (const c of dchunks) { decompressed.set(c, offset); offset += c.length; }

    const result = new TextDecoder().decode(decompressed);
    return Response.json({ match: result === original });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Match bool `json:"match"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if !data.Match {
		t.Error("deflate round-trip should return the original string")
	}
}

func TestCompression_DeflateRawRoundTrip(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    const original = "deflate-raw test content repeated enough to compress well. " +
      "deflate-raw test content repeated enough to compress well.";

    const cs = new CompressionStream("deflate-raw");
    const writer = cs.writable.getWriter();
    writer.write(new TextEncoder().encode(original));
    writer.close();
    const chunks = [];
    const reader = cs.readable.getReader();
    while (true) {
      const { done, value } = await reader.read();
      if (done) break;
      chunks.push(value);
    }
    let compressedLen = 0;
    for (const c of chunks) compressedLen += c.length;
    const compressed = new Uint8Array(compressedLen);
    let offset = 0;
    for (const c of chunks) { compressed.set(c, offset); offset += c.length; }

    const ds = new DecompressionStream("deflate-raw");
    const dwriter = ds.writable.getWriter();
    dwriter.write(compressed);
    dwriter.close();
    const dchunks = [];
    const dreader = ds.readable.getReader();
    while (true) {
      const { done, value } = await dreader.read();
      if (done) break;
      dchunks.push(value);
    }
    let decompressedLen = 0;
    for (const c of dchunks) decompressedLen += c.length;
    const decompressed = new Uint8Array(decompressedLen);
    offset = 0;
    for (const c of dchunks) { decompressed.set(c, offset); offset += c.length; }

    const result = new TextDecoder().decode(decompressed);
    return Response.json({ match: result === original });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Match bool `json:"match"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if !data.Match {
		t.Error("deflate-raw round-trip should return the original string")
	}
}

func TestCompression_UnsupportedFormat(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    let threw = false;
    try {
      new CompressionStream("brotli");
    } catch (e) {
      threw = true;
    }
    let threw2 = false;
    try {
      new DecompressionStream("brotli");
    } catch (e) {
      threw2 = true;
    }
    return Response.json({ compressionThrew: threw, decompressionThrew: threw2 });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		CompressionThrew   bool `json:"compressionThrew"`
		DecompressionThrew bool `json:"decompressionThrew"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if !data.CompressionThrew {
		t.Error("CompressionStream with unsupported format should throw")
	}
	if !data.DecompressionThrew {
		t.Error("DecompressionStream with unsupported format should throw")
	}
}

func TestCompression_MultipleChunks(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    const cs = new CompressionStream("gzip");
    const writer = cs.writable.getWriter();
    // Write multiple chunks
    writer.write(new TextEncoder().encode("chunk one "));
    writer.write(new TextEncoder().encode("chunk two "));
    writer.write(new TextEncoder().encode("chunk three"));
    writer.close();

    const chunks = [];
    const reader = cs.readable.getReader();
    while (true) {
      const { done, value } = await reader.read();
      if (done) break;
      chunks.push(value);
    }
    let compressedLen = 0;
    for (const c of chunks) compressedLen += c.length;
    const compressed = new Uint8Array(compressedLen);
    let offset = 0;
    for (const c of chunks) { compressed.set(c, offset); offset += c.length; }

    const ds = new DecompressionStream("gzip");
    const dwriter = ds.writable.getWriter();
    dwriter.write(compressed);
    dwriter.close();
    const dchunks = [];
    const dreader = ds.readable.getReader();
    while (true) {
      const { done, value } = await dreader.read();
      if (done) break;
      dchunks.push(value);
    }
    let decompressedLen = 0;
    for (const c of dchunks) decompressedLen += c.length;
    const decompressed = new Uint8Array(decompressedLen);
    offset = 0;
    for (const c of dchunks) { decompressed.set(c, offset); offset += c.length; }

    const result = new TextDecoder().decode(decompressed);
    return Response.json({ match: result === "chunk one chunk two chunk three" });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Match bool `json:"match"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if !data.Match {
		t.Error("multiple chunks should be concatenated and compressed correctly")
	}
}

func TestCompression_BinaryData(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    // Create binary data with all byte values 0-255
    const binary = new Uint8Array(256);
    for (let i = 0; i < 256; i++) binary[i] = i;

    const cs = new CompressionStream("gzip");
    const writer = cs.writable.getWriter();
    writer.write(binary);
    writer.close();
    const chunks = [];
    const reader = cs.readable.getReader();
    while (true) {
      const { done, value } = await reader.read();
      if (done) break;
      chunks.push(value);
    }
    let compressedLen = 0;
    for (const c of chunks) compressedLen += c.length;
    const compressed = new Uint8Array(compressedLen);
    let offset = 0;
    for (const c of chunks) { compressed.set(c, offset); offset += c.length; }

    const ds = new DecompressionStream("gzip");
    const dwriter = ds.writable.getWriter();
    dwriter.write(compressed);
    dwriter.close();
    const dchunks = [];
    const dreader = ds.readable.getReader();
    while (true) {
      const { done, value } = await dreader.read();
      if (done) break;
      dchunks.push(value);
    }
    let decompressedLen = 0;
    for (const c of dchunks) decompressedLen += c.length;
    const decompressed = new Uint8Array(decompressedLen);
    offset = 0;
    for (const c of dchunks) { decompressed.set(c, offset); offset += c.length; }

    let match = decompressed.length === 256;
    for (let i = 0; i < 256 && match; i++) {
      if (decompressed[i] !== i) match = false;
    }
    return Response.json({ match, length: decompressed.length });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Match  bool `json:"match"`
		Length int  `json:"length"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if !data.Match {
		t.Error("binary data round-trip through gzip should preserve all bytes")
	}
	if data.Length != 256 {
		t.Errorf("decompressed length = %d, want 256", data.Length)
	}
}

func TestCompression_EmptyInput(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    // Compress empty data
    const cs = new CompressionStream("gzip");
    const writer = cs.writable.getWriter();
    writer.write(new Uint8Array(0));
    writer.close();
    const chunks = [];
    const reader = cs.readable.getReader();
    while (true) {
      const { done, value } = await reader.read();
      if (done) break;
      chunks.push(value);
    }
    let compressedLen = 0;
    for (const c of chunks) compressedLen += c.length;
    const compressed = new Uint8Array(compressedLen);
    let offset = 0;
    for (const c of chunks) { compressed.set(c, offset); offset += c.length; }

    // Decompress
    const ds = new DecompressionStream("gzip");
    const dwriter = ds.writable.getWriter();
    dwriter.write(compressed);
    dwriter.close();
    const dchunks = [];
    const dreader = ds.readable.getReader();
    while (true) {
      const { done, value } = await dreader.read();
      if (done) break;
      dchunks.push(value);
    }
    let decompressedLen = 0;
    for (const c of dchunks) decompressedLen += c.length;

    return Response.json({ compressedLen, decompressedLen });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		CompressedLen   int `json:"compressedLen"`
		DecompressedLen int `json:"decompressedLen"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	// gzip of empty should produce a valid (non-zero) gzip stream
	if data.CompressedLen == 0 {
		t.Error("gzip of empty data should produce gzip header/trailer")
	}
	if data.DecompressedLen != 0 {
		t.Errorf("decompressed empty data length = %d, want 0", data.DecompressedLen)
	}
}

func TestCompression_IncompressibleData(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    // Random-ish bytes (hard to compress)
    const data = new Uint8Array(512);
    for (let i = 0; i < data.length; i++) {
      data[i] = (i * 31 + 17) & 0xFF;
    }

    const cs = new CompressionStream("gzip");
    const writer = cs.writable.getWriter();
    writer.write(data);
    writer.close();
    const chunks = [];
    const reader = cs.readable.getReader();
    while (true) {
      const { done, value } = await reader.read();
      if (done) break;
      chunks.push(value);
    }
    let compressedLen = 0;
    for (const c of chunks) compressedLen += c.length;
    const compressed = new Uint8Array(compressedLen);
    let offset = 0;
    for (const c of chunks) { compressed.set(c, offset); offset += c.length; }

    const ds = new DecompressionStream("gzip");
    const dwriter = ds.writable.getWriter();
    dwriter.write(compressed);
    dwriter.close();
    const dchunks = [];
    const dreader = ds.readable.getReader();
    while (true) {
      const { done, value } = await dreader.read();
      if (done) break;
      dchunks.push(value);
    }
    let decompressedLen = 0;
    for (const c of dchunks) decompressedLen += c.length;
    const decompressed = new Uint8Array(decompressedLen);
    offset = 0;
    for (const c of dchunks) { decompressed.set(c, offset); offset += c.length; }

    let match = decompressed.length === data.length;
    for (let i = 0; i < data.length && match; i++) {
      if (decompressed[i] !== data[i]) match = false;
    }
    return Response.json({ match, originalLen: data.length, decompressedLen: decompressed.length });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Match           bool `json:"match"`
		OriginalLen     int  `json:"originalLen"`
		DecompressedLen int  `json:"decompressedLen"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !data.Match {
		t.Error("incompressible data should round-trip correctly")
	}
}

func TestCompression_DirectCompressMissingArgs(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request, env) {
    try {
      __compress("gzip");
      return Response.json({ threw: false });
    } catch(e) {
      return Response.json({ threw: true, msg: e.message });
    }
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Threw bool   `json:"threw"`
		Msg   string `json:"msg"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatal(err)
	}
	if !data.Threw {
		t.Error("__compress with 1 arg should throw")
	}
}

func TestCompression_DirectCompressBadBase64(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request, env) {
    try {
      __compress("gzip", "not-valid-base64!!!");
      return Response.json({ threw: false });
    } catch(e) {
      return Response.json({ threw: true, msg: e.message });
    }
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Threw bool   `json:"threw"`
		Msg   string `json:"msg"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatal(err)
	}
	if !data.Threw {
		t.Error("__compress with bad base64 should throw")
	}
}

func TestCompression_DirectDecompressMissingArgs(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request, env) {
    try {
      __decompress();
      return Response.json({ threw: false });
    } catch(e) {
      return Response.json({ threw: true, msg: e.message });
    }
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Threw bool `json:"threw"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatal(err)
	}
	if !data.Threw {
		t.Error("__decompress with no args should throw")
	}
}

func TestCompression_DirectDecompressBadBase64(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request, env) {
    try {
      __decompress("gzip", "not-valid!!!");
      return Response.json({ threw: false });
    } catch(e) {
      return Response.json({ threw: true, msg: e.message });
    }
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Threw bool `json:"threw"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatal(err)
	}
	if !data.Threw {
		t.Error("__decompress with bad base64 should throw")
	}
}

func TestCompression_DirectDecompressCorruptData(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request, env) {
    // Valid base64 but not valid gzip data
    try {
      __decompress("gzip", "aGVsbG8=");
      return Response.json({ threw: false });
    } catch(e) {
      return Response.json({ threw: true, msg: e.message });
    }
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Threw bool `json:"threw"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatal(err)
	}
	if !data.Threw {
		t.Error("__decompress with corrupt gzip data should throw")
	}
}

func TestCompression_DirectCompressUnsupportedFormat(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request, env) {
    try {
      __compress("brotli", "aGVsbG8=");
      return Response.json({ threw: false });
    } catch(e) {
      return Response.json({ threw: true, msg: e.message });
    }
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Threw bool `json:"threw"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatal(err)
	}
	if !data.Threw {
		t.Error("__compress with unsupported format should throw")
	}
}

func TestCompression_DirectDecompressUnsupportedFormat(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request, env) {
    try {
      __decompress("brotli", "aGVsbG8=");
      return Response.json({ threw: false });
    } catch(e) {
      return Response.json({ threw: true, msg: e.message });
    }
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Threw bool `json:"threw"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatal(err)
	}
	if !data.Threw {
		t.Error("__decompress with unsupported format should throw")
	}
}

func TestCompression_StreamingChunkByChunk(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    // Write multiple chunks and verify compressed output is produced per-chunk
    // (not just at flush time).
    const cs = new CompressionStream("gzip");
    const writer = cs.writable.getWriter();
    const reader = cs.readable.getReader();

    // Write first chunk
    writer.write(new TextEncoder().encode("Hello, streaming compression! ".repeat(10)));
    // Read what's available so far
    const firstRead = await reader.read();
    const gotChunkBeforeClose = !firstRead.done && firstRead.value.length > 0;

    // Write second chunk
    writer.write(new TextEncoder().encode("Second chunk of data! ".repeat(10)));
    writer.close();

    // Read remaining chunks
    const chunks = [firstRead.value];
    while (true) {
      const { done, value } = await reader.read();
      if (done) break;
      chunks.push(value);
    }

    let totalCompressed = 0;
    for (const c of chunks) totalCompressed += c.length;
    const compressed = new Uint8Array(totalCompressed);
    let offset = 0;
    for (const c of chunks) { compressed.set(c, offset); offset += c.length; }

    // Decompress to verify correctness
    const ds = new DecompressionStream("gzip");
    const dwriter = ds.writable.getWriter();
    dwriter.write(compressed);
    dwriter.close();
    const dchunks = [];
    const dreader = ds.readable.getReader();
    while (true) {
      const { done, value } = await dreader.read();
      if (done) break;
      dchunks.push(value);
    }
    let totalDecompressed = 0;
    for (const c of dchunks) totalDecompressed += c.length;
    const decompressed = new Uint8Array(totalDecompressed);
    offset = 0;
    for (const c of dchunks) { decompressed.set(c, offset); offset += c.length; }

    const original = "Hello, streaming compression! ".repeat(10) + "Second chunk of data! ".repeat(10);
    const result = new TextDecoder().decode(decompressed);
    return Response.json({
      match: result === original,
      gotChunkBeforeClose,
      compressedChunks: chunks.length,
    });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Match               bool `json:"match"`
		GotChunkBeforeClose bool `json:"gotChunkBeforeClose"`
		CompressedChunks    int  `json:"compressedChunks"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !data.Match {
		t.Error("streaming chunk-by-chunk compression should round-trip correctly")
	}
	if !data.GotChunkBeforeClose {
		t.Error("should receive compressed output before the stream is closed (streaming)")
	}
	if data.CompressedChunks < 2 {
		t.Errorf("expected multiple compressed chunks, got %d", data.CompressedChunks)
	}
}

func TestCompression_StreamingMultipleFormats(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	for _, format := range []string{"gzip", "deflate", "deflate-raw"} {
		t.Run(format, func(t *testing.T) {
			source := `export default {
  async fetch(request, env) {
    const format = "` + format + `";
    const original = "Streaming test for " + format + "! ".repeat(20);

    const cs = new CompressionStream(format);
    const writer = cs.writable.getWriter();
    // Write in small pieces
    const encoded = new TextEncoder().encode(original);
    const chunkSize = 50;
    for (let i = 0; i < encoded.length; i += chunkSize) {
      writer.write(encoded.slice(i, Math.min(i + chunkSize, encoded.length)));
    }
    writer.close();

    const chunks = [];
    const reader = cs.readable.getReader();
    while (true) {
      const { done, value } = await reader.read();
      if (done) break;
      chunks.push(value);
    }
    let totalLen = 0;
    for (const c of chunks) totalLen += c.length;
    const compressed = new Uint8Array(totalLen);
    let offset = 0;
    for (const c of chunks) { compressed.set(c, offset); offset += c.length; }

    const ds = new DecompressionStream(format);
    const dwriter = ds.writable.getWriter();
    dwriter.write(compressed);
    dwriter.close();
    const dchunks = [];
    const dreader = ds.readable.getReader();
    while (true) {
      const { done, value } = await dreader.read();
      if (done) break;
      dchunks.push(value);
    }
    let dTotal = 0;
    for (const c of dchunks) dTotal += c.length;
    const decompressed = new Uint8Array(dTotal);
    offset = 0;
    for (const c of dchunks) { decompressed.set(c, offset); offset += c.length; }

    const result = new TextDecoder().decode(decompressed);
    return Response.json({ match: result === original });
  },
};`

			r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
			assertOK(t, r)

			var data struct {
				Match bool `json:"match"`
			}
			if err := json.Unmarshal(r.Response.Body, &data); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if !data.Match {
				t.Errorf("%s streaming compression with small chunks should round-trip correctly", format)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// CompressionStream: pipeThrough, string chunks, empty chunks
// ---------------------------------------------------------------------------

func TestCompression_PipeThroughCompressionStream(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    const original = "Hello, World! This is a test of pipeThrough with CompressionStream.";
    const encoder = new TextEncoder();
    const data = encoder.encode(original);

    // Create a ReadableStream from the data.
    const inputStream = new ReadableStream({
      start(controller) {
        controller.enqueue(data);
        controller.close();
      }
    });

    // Pipe through CompressionStream.
    const compressedStream = inputStream.pipeThrough(new CompressionStream("gzip"));

    // Collect compressed chunks.
    const reader = compressedStream.getReader();
    const chunks = [];
    while (true) {
      const { done, value } = await reader.read();
      if (done) break;
      chunks.push(value);
    }

    // Total compressed bytes.
    let totalBytes = 0;
    for (const c of chunks) totalBytes += c.length;

    return Response.json({
      hasChunks: chunks.length > 0,
      compressed: totalBytes > 0,
      smallerOrReasonable: totalBytes < original.length * 2,
    });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		HasChunks           bool `json:"hasChunks"`
		Compressed          bool `json:"compressed"`
		SmallerOrReasonable bool `json:"smallerOrReasonable"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !data.HasChunks {
		t.Error("pipeThrough CompressionStream should produce chunks")
	}
	if !data.Compressed {
		t.Error("compressed output should have bytes")
	}
	if !data.SmallerOrReasonable {
		t.Error("compressed size should be reasonable")
	}
}

func TestCompression_StringChunks(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    const inputStream = new ReadableStream({
      start(controller) {
        controller.enqueue("Hello ");
        controller.enqueue("World!");
        controller.close();
      }
    });

    const compressedStream = inputStream.pipeThrough(new CompressionStream("gzip"));
    const decompressedStream = compressedStream.pipeThrough(new DecompressionStream("gzip"));

    const reader = decompressedStream.getReader();
    const chunks = [];
    while (true) {
      const { done, value } = await reader.read();
      if (done) break;
      chunks.push(value);
    }

    let total = 0;
    for (const c of chunks) total += c.length;
    const result = new Uint8Array(total);
    let offset = 0;
    for (const c of chunks) { result.set(c, offset); offset += c.length; }

    const text = new TextDecoder().decode(result);
    return Response.json({ text, match: text === "Hello World!" });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Text  string `json:"text"`
		Match bool   `json:"match"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !data.Match {
		t.Errorf("string chunks round-trip failed, got %q", data.Text)
	}
}

func TestCompression_EmptyChunkHandling(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    const inputStream = new ReadableStream({
      start(controller) {
        controller.enqueue(new Uint8Array(0));
        controller.enqueue(new TextEncoder().encode("data"));
        controller.enqueue(new Uint8Array(0));
        controller.close();
      }
    });

    const compressedStream = inputStream.pipeThrough(new CompressionStream("gzip"));
    const decompressedStream = compressedStream.pipeThrough(new DecompressionStream("gzip"));

    const reader = decompressedStream.getReader();
    const chunks = [];
    while (true) {
      const { done, value } = await reader.read();
      if (done) break;
      chunks.push(value);
    }

    let total = 0;
    for (const c of chunks) total += c.length;
    const result = new Uint8Array(total);
    let offset = 0;
    for (const c of chunks) { result.set(c, offset); offset += c.length; }

    const text = new TextDecoder().decode(result);
    return Response.json({ text, match: text === "data" });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Text  string `json:"text"`
		Match bool   `json:"match"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !data.Match {
		t.Errorf("empty chunk handling failed, got %q", data.Text)
	}
}

func TestCompression_DecompressionStreamMultiChunkInput(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	// This test verifies that DecompressionStream correctly handles compressed
	// data fed in multiple separate chunks via the streaming io.Pipe path.
	// We split compressed bytes into 3 pieces, write them separately, and
	// verify the full round-trip produces the original data.
	source := `export default {
  async fetch(request, env) {
    const original = "Streaming decompression test! ".repeat(100);

    // Compress.
    const cs = new CompressionStream("gzip");
    const cw = cs.writable.getWriter();
    cw.write(new TextEncoder().encode(original));
    cw.close();
    const cchunks = [];
    const cr = cs.readable.getReader();
    while (true) {
      const { done, value } = await cr.read();
      if (done) break;
      cchunks.push(value);
    }
    let cLen = 0;
    for (const c of cchunks) cLen += c.length;
    const compressed = new Uint8Array(cLen);
    let off = 0;
    for (const c of cchunks) { compressed.set(c, off); off += c.length; }

    // Decompress by feeding 3 separate chunks, then close.
    const ds = new DecompressionStream("gzip");
    const dw = ds.writable.getWriter();
    const dr = ds.readable.getReader();

    const chunkSize = Math.ceil(compressed.length / 3);
    for (let i = 0; i < compressed.length; i += chunkSize) {
      dw.write(compressed.slice(i, Math.min(i + chunkSize, compressed.length)));
    }
    dw.close();

    // Read all decompressed output.
    const dchunks = [];
    while (true) {
      const { done, value } = await dr.read();
      if (done) break;
      dchunks.push(value);
    }
    let dLen = 0;
    for (const c of dchunks) dLen += c.length;
    const decompressed = new Uint8Array(dLen);
    off = 0;
    for (const c of dchunks) { decompressed.set(c, off); off += c.length; }

    const result = new TextDecoder().decode(decompressed);
    return Response.json({
      match: result === original,
      compressedLen: compressed.length,
      decompressedLen: dLen,
      decompressedChunks: dchunks.length,
    });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Match              bool `json:"match"`
		CompressedLen      int  `json:"compressedLen"`
		DecompressedLen    int  `json:"decompressedLen"`
		DecompressedChunks int  `json:"decompressedChunks"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !data.Match {
		t.Error("multi-chunk streaming decompression should round-trip correctly")
	}
	if data.DecompressedLen != len("Streaming decompression test! ")*100 {
		t.Errorf("decompressed length = %d, want %d", data.DecompressedLen, len("Streaming decompression test! ")*100)
	}
}

// ---------------------------------------------------------------------------
// Brotli ("br") compression tests
// ---------------------------------------------------------------------------

func TestCompression_BrotliBulkRoundTrip(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    const original = "Brotli bulk compression test! Repeated content for good ratio. " +
      "Brotli bulk compression test! Repeated content for good ratio. " +
      "Brotli bulk compression test! Repeated content for good ratio.";
    const encoded = new TextEncoder().encode(original);
    const b64 = __bufferSourceToB64(encoded);

    const compressedB64 = __compress("br", b64);
    const decompressedB64 = __decompress("br", compressedB64);

    const decompressedBytes = Uint8Array.from(atob(decompressedB64), c => c.charCodeAt(0));
    const result = new TextDecoder().decode(decompressedBytes);

    const compressedBytes = Uint8Array.from(atob(compressedB64), c => c.charCodeAt(0));

    return Response.json({
      match: result === original,
      originalLen: encoded.length,
      compressedLen: compressedBytes.length,
      smallerAfterCompress: compressedBytes.length < encoded.length,
    });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Match                bool `json:"match"`
		OriginalLen          int  `json:"originalLen"`
		CompressedLen        int  `json:"compressedLen"`
		SmallerAfterCompress bool `json:"smallerAfterCompress"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !data.Match {
		t.Error("brotli bulk round-trip should return the original string")
	}
	if !data.SmallerAfterCompress {
		t.Errorf("brotli compressed (%d) should be smaller than original (%d)", data.CompressedLen, data.OriginalLen)
	}
}

func TestCompression_BrotliStreamingRoundTrip(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    const original = "Brotli streaming test with repeated content for compression. " +
      "Brotli streaming test with repeated content for compression. " +
      "Brotli streaming test with repeated content for compression.";

    // Compress
    const cs = new CompressionStream("br");
    const writer = cs.writable.getWriter();
    writer.write(new TextEncoder().encode(original));
    writer.close();
    const compressedChunks = [];
    const reader = cs.readable.getReader();
    while (true) {
      const { done, value } = await reader.read();
      if (done) break;
      compressedChunks.push(value);
    }
    let compressedLen = 0;
    for (const c of compressedChunks) compressedLen += c.length;
    const compressed = new Uint8Array(compressedLen);
    let offset = 0;
    for (const c of compressedChunks) { compressed.set(c, offset); offset += c.length; }

    // Decompress
    const ds = new DecompressionStream("br");
    const dwriter = ds.writable.getWriter();
    dwriter.write(compressed);
    dwriter.close();
    const decompressedChunks = [];
    const dreader = ds.readable.getReader();
    while (true) {
      const { done, value } = await dreader.read();
      if (done) break;
      decompressedChunks.push(value);
    }
    let decompressedLen = 0;
    for (const c of decompressedChunks) decompressedLen += c.length;
    const decompressed = new Uint8Array(decompressedLen);
    offset = 0;
    for (const c of decompressedChunks) { decompressed.set(c, offset); offset += c.length; }

    const result = new TextDecoder().decode(decompressed);
    return Response.json({
      match: result === original,
      originalLen: original.length,
      compressedLen: compressed.length,
      smallerAfterCompress: compressed.length < original.length,
    });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Match                bool `json:"match"`
		OriginalLen          int  `json:"originalLen"`
		CompressedLen        int  `json:"compressedLen"`
		SmallerAfterCompress bool `json:"smallerAfterCompress"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !data.Match {
		t.Error("brotli streaming round-trip should return the original string")
	}
	if !data.SmallerAfterCompress {
		t.Errorf("brotli compressed (%d) should be smaller than original (%d)", data.CompressedLen, data.OriginalLen)
	}
}

func TestCompression_BrotliStreamingMultiChunk(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    const cs = new CompressionStream("br");
    const writer = cs.writable.getWriter();
    // Write multiple chunks
    writer.write(new TextEncoder().encode("chunk one "));
    writer.write(new TextEncoder().encode("chunk two "));
    writer.write(new TextEncoder().encode("chunk three"));
    writer.close();

    const chunks = [];
    const reader = cs.readable.getReader();
    while (true) {
      const { done, value } = await reader.read();
      if (done) break;
      chunks.push(value);
    }
    let compressedLen = 0;
    for (const c of chunks) compressedLen += c.length;
    const compressed = new Uint8Array(compressedLen);
    let offset = 0;
    for (const c of chunks) { compressed.set(c, offset); offset += c.length; }

    const ds = new DecompressionStream("br");
    const dwriter = ds.writable.getWriter();
    dwriter.write(compressed);
    dwriter.close();
    const dchunks = [];
    const dreader = ds.readable.getReader();
    while (true) {
      const { done, value } = await dreader.read();
      if (done) break;
      dchunks.push(value);
    }
    let decompressedLen = 0;
    for (const c of dchunks) decompressedLen += c.length;
    const decompressed = new Uint8Array(decompressedLen);
    offset = 0;
    for (const c of dchunks) { decompressed.set(c, offset); offset += c.length; }

    const result = new TextDecoder().decode(decompressed);
    return Response.json({ match: result === "chunk one chunk two chunk three" });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Match bool `json:"match"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !data.Match {
		t.Error("brotli multi-chunk streaming should round-trip correctly")
	}
}

func TestCompression_BrotliPipeThrough(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    const original = "Brotli pipeThrough test data with enough content to compress well. " +
      "Brotli pipeThrough test data with enough content to compress well.";
    const encoder = new TextEncoder();
    const data = encoder.encode(original);

    // Create a ReadableStream from the data.
    const inputStream = new ReadableStream({
      start(controller) {
        controller.enqueue(data);
        controller.close();
      }
    });

    // Pipe through CompressionStream then DecompressionStream.
    const compressedStream = inputStream.pipeThrough(new CompressionStream("br"));
    const decompressedStream = compressedStream.pipeThrough(new DecompressionStream("br"));

    const reader = decompressedStream.getReader();
    const chunks = [];
    while (true) {
      const { done, value } = await reader.read();
      if (done) break;
      chunks.push(value);
    }

    let total = 0;
    for (const c of chunks) total += c.length;
    const result = new Uint8Array(total);
    let offset = 0;
    for (const c of chunks) { result.set(c, offset); offset += c.length; }

    const text = new TextDecoder().decode(result);
    return Response.json({
      match: text === original,
      text: text,
    });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Match bool   `json:"match"`
		Text  string `json:"text"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !data.Match {
		t.Errorf("brotli pipeThrough round-trip failed, got %q", data.Text)
	}
}

func TestCompression_BrotliCrossFormatFails(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	// Use bulk __compress/__decompress to avoid streaming goroutine hangs
	// when feeding brotli data to a gzip decompressor.
	source := `export default {
  fetch(request, env) {
    const original = "Cross-format test data for brotli vs gzip mismatch. ".repeat(5);
    const encoded = new TextEncoder().encode(original);
    const b64 = __bufferSourceToB64(encoded);

    // Compress with brotli using bulk function
    const compressedB64 = __compress("br", b64);

    // Try to decompress brotli data with gzip - should throw
    let failed = false;
    try {
      __decompress("gzip", compressedB64);
    } catch(e) {
      failed = true;
    }

    // Also try the reverse: compress with gzip, decompress with brotli
    const gzipB64 = __compress("gzip", b64);
    let reverseFailed = false;
    try {
      __decompress("br", gzipB64);
    } catch(e) {
      reverseFailed = true;
    }

    return Response.json({ crossFormatFailed: failed, reverseFailed: reverseFailed });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		CrossFormatFailed bool `json:"crossFormatFailed"`
		ReverseFailed     bool `json:"reverseFailed"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !data.CrossFormatFailed {
		t.Error("decompressing brotli data with gzip should fail")
	}
	if !data.ReverseFailed {
		t.Error("decompressing gzip data with brotli should fail")
	}
}

func TestCompression_BrotliEmptyInput(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    // Compress empty data with brotli
    const cs = new CompressionStream("br");
    const writer = cs.writable.getWriter();
    writer.write(new Uint8Array(0));
    writer.close();
    const chunks = [];
    const reader = cs.readable.getReader();
    while (true) {
      const { done, value } = await reader.read();
      if (done) break;
      chunks.push(value);
    }
    let compressedLen = 0;
    for (const c of chunks) compressedLen += c.length;
    const compressed = new Uint8Array(compressedLen);
    let offset = 0;
    for (const c of chunks) { compressed.set(c, offset); offset += c.length; }

    // Decompress
    const ds = new DecompressionStream("br");
    const dwriter = ds.writable.getWriter();
    dwriter.write(compressed);
    dwriter.close();
    const dchunks = [];
    const dreader = ds.readable.getReader();
    while (true) {
      const { done, value } = await dreader.read();
      if (done) break;
      dchunks.push(value);
    }
    let decompressedLen = 0;
    for (const c of dchunks) decompressedLen += c.length;

    return Response.json({ compressedLen, decompressedLen });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		CompressedLen   int `json:"compressedLen"`
		DecompressedLen int `json:"decompressedLen"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if data.CompressedLen == 0 {
		t.Error("brotli of empty data should produce a valid brotli stream (non-zero)")
	}
	if data.DecompressedLen != 0 {
		t.Errorf("decompressed empty brotli data length = %d, want 0", data.DecompressedLen)
	}
}

func TestCompression_BrotliLargeData(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    // Generate large compressible data
    const original = "Brotli large data compression test! ".repeat(500);

    const cs = new CompressionStream("br");
    const writer = cs.writable.getWriter();
    writer.write(new TextEncoder().encode(original));
    writer.close();
    const chunks = [];
    const reader = cs.readable.getReader();
    while (true) {
      const { done, value } = await reader.read();
      if (done) break;
      chunks.push(value);
    }
    let compressedLen = 0;
    for (const c of chunks) compressedLen += c.length;
    const compressed = new Uint8Array(compressedLen);
    let offset = 0;
    for (const c of chunks) { compressed.set(c, offset); offset += c.length; }

    // Decompress
    const ds = new DecompressionStream("br");
    const dwriter = ds.writable.getWriter();
    dwriter.write(compressed);
    dwriter.close();
    const dchunks = [];
    const dreader = ds.readable.getReader();
    while (true) {
      const { done, value } = await dreader.read();
      if (done) break;
      dchunks.push(value);
    }
    let decompressedLen = 0;
    for (const c of dchunks) decompressedLen += c.length;
    const decompressed = new Uint8Array(decompressedLen);
    offset = 0;
    for (const c of dchunks) { decompressed.set(c, offset); offset += c.length; }

    const result = new TextDecoder().decode(decompressed);
    return Response.json({
      match: result === original,
      originalLen: original.length,
      compressedLen: compressed.length,
      decompressedLen: decompressedLen,
      smallerAfterCompress: compressed.length < original.length,
      ratio: (compressed.length / original.length).toFixed(4),
    });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Match                bool   `json:"match"`
		OriginalLen          int    `json:"originalLen"`
		CompressedLen        int    `json:"compressedLen"`
		DecompressedLen      int    `json:"decompressedLen"`
		SmallerAfterCompress bool   `json:"smallerAfterCompress"`
		Ratio                string `json:"ratio"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !data.Match {
		t.Error("brotli large data round-trip should produce the original string")
	}
	if !data.SmallerAfterCompress {
		t.Errorf("brotli compressed (%d) should be smaller than original (%d)", data.CompressedLen, data.OriginalLen)
	}
}

func TestCompression_InvalidFormatString(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    const results = {};

    // Test various invalid format strings
    const invalidFormats = ["brotli", "lz4", "zstd", "snappy", ""];
    for (const fmt of invalidFormats) {
      let compThrew = false;
      try { new CompressionStream(fmt); } catch(e) { compThrew = true; }
      let decompThrew = false;
      try { new DecompressionStream(fmt); } catch(e) { decompThrew = true; }
      results[fmt || "empty"] = { compThrew, decompThrew };
    }

    // Valid formats should NOT throw
    const validFormats = ["gzip", "deflate", "deflate-raw", "br"];
    for (const fmt of validFormats) {
      let compThrew = false;
      try { new CompressionStream(fmt); } catch(e) { compThrew = true; }
      let decompThrew = false;
      try { new DecompressionStream(fmt); } catch(e) { decompThrew = true; }
      results[fmt] = { compThrew, decompThrew };
    }

    return Response.json(results);
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data map[string]struct {
		CompThrew   bool `json:"compThrew"`
		DecompThrew bool `json:"decompThrew"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	// Invalid formats should throw
	for _, fmt := range []string{"brotli", "lz4", "zstd", "snappy", "empty"} {
		d, ok := data[fmt]
		if !ok {
			t.Errorf("missing result for format %q", fmt)
			continue
		}
		if !d.CompThrew {
			t.Errorf("CompressionStream(%q) should throw", fmt)
		}
		if !d.DecompThrew {
			t.Errorf("DecompressionStream(%q) should throw", fmt)
		}
	}

	// Valid formats should NOT throw
	for _, fmt := range []string{"gzip", "deflate", "deflate-raw", "br"} {
		d, ok := data[fmt]
		if !ok {
			t.Errorf("missing result for format %q", fmt)
			continue
		}
		if d.CompThrew {
			t.Errorf("CompressionStream(%q) should NOT throw", fmt)
		}
		if d.DecompThrew {
			t.Errorf("DecompressionStream(%q) should NOT throw", fmt)
		}
	}
}

// TestCompression_DecompressInvalidGzip exercises the gzip.NewReader error
// path (compression.go ~line 366) by feeding non-gzip bytes.
func TestCompression_DecompressInvalidGzip(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	// The error surfaces on the writable side (transform callback throws),
	// so we catch on writer.write() or writer.close().
	source := `export default {
  async fetch(request, env) {
    const ds = new DecompressionStream("gzip");
    const writer = ds.writable.getWriter();
    let writeError = null;
    let closeError = null;
    try {
      await writer.write(new TextEncoder().encode("this is not valid gzip data at all!!!"));
    } catch(e) {
      writeError = String(e);
    }
    if (!writeError) {
      try {
        await writer.close();
      } catch(e) {
        closeError = String(e);
      }
    }
    return Response.json({ writeError, closeError, gotError: !!(writeError || closeError) });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		WriteError string `json:"writeError"`
		CloseError string `json:"closeError"`
		GotError   bool   `json:"gotError"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatal(err)
	}
	if !data.GotError {
		t.Error("decompressing invalid gzip data should produce an error")
	}
}

func TestCompression_BrotliInStreamingFormats(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	// Include "br" alongside existing formats in the streaming multi-format test
	for _, format := range []string{"gzip", "deflate", "deflate-raw", "br"} {
		t.Run(format, func(t *testing.T) {
			source := `export default {
  async fetch(request, env) {
    const format = "` + format + `";
    const original = "Streaming test for " + format + "! ".repeat(20);

    const cs = new CompressionStream(format);
    const writer = cs.writable.getWriter();
    const encoded = new TextEncoder().encode(original);
    const chunkSize = 50;
    for (let i = 0; i < encoded.length; i += chunkSize) {
      writer.write(encoded.slice(i, Math.min(i + chunkSize, encoded.length)));
    }
    writer.close();

    const chunks = [];
    const reader = cs.readable.getReader();
    while (true) {
      const { done, value } = await reader.read();
      if (done) break;
      chunks.push(value);
    }
    let totalLen = 0;
    for (const c of chunks) totalLen += c.length;
    const compressed = new Uint8Array(totalLen);
    let offset = 0;
    for (const c of chunks) { compressed.set(c, offset); offset += c.length; }

    const ds = new DecompressionStream(format);
    const dwriter = ds.writable.getWriter();
    dwriter.write(compressed);
    dwriter.close();
    const dchunks = [];
    const dreader = ds.readable.getReader();
    while (true) {
      const { done, value } = await dreader.read();
      if (done) break;
      dchunks.push(value);
    }
    let dTotal = 0;
    for (const c of dchunks) dTotal += c.length;
    const decompressed = new Uint8Array(dTotal);
    offset = 0;
    for (const c of dchunks) { decompressed.set(c, offset); offset += c.length; }

    const result = new TextDecoder().decode(decompressed);
    return Response.json({ match: result === original });
  },
};`

			r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
			assertOK(t, r)

			var data struct {
				Match bool `json:"match"`
			}
			if err := json.Unmarshal(r.Response.Body, &data); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if !data.Match {
				t.Errorf("%s streaming compression with small chunks should round-trip correctly", format)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Bug 2 regression: new Response(stream).text() / .arrayBuffer() with
// ReadableStream bodies from CompressionStream/DecompressionStream.
//
// Before the fix, Response.prototype.text() and .arrayBuffer() did not
// handle ReadableStream bodies -- they fell through to bodyToString() which
// only drained the synchronous _queue, missing data produced asynchronously
// by transform streams. The fix added __readStreamBytes() in bodytypes.go
// to properly await and collect all chunks from a ReadableStream.
// ---------------------------------------------------------------------------

// TestCompression_ResponseTextFromDecompressionStream verifies that
// new Response(decompressedStream).text() correctly reads a ReadableStream
// body produced by DecompressionStream (gzip round-trip).
func TestCompression_ResponseTextFromDecompressionStream(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    const original = "Bug2 regression: Response.text() must handle ReadableStream bodies! " +
      "Bug2 regression: Response.text() must handle ReadableStream bodies! " +
      "Bug2 regression: Response.text() must handle ReadableStream bodies!";

    // Compress via CompressionStream
    const cs = new CompressionStream("gzip");
    const cw = cs.writable.getWriter();
    cw.write(new TextEncoder().encode(original));
    cw.close();

    // Collect compressed bytes
    const chunks = [];
    const cr = cs.readable.getReader();
    while (true) {
      const { done, value } = await cr.read();
      if (done) break;
      chunks.push(value);
    }
    let cLen = 0;
    for (const c of chunks) cLen += c.length;
    const compressed = new Uint8Array(cLen);
    let off = 0;
    for (const c of chunks) { compressed.set(c, off); off += c.length; }

    // Decompress -- pass the ReadableStream directly to new Response()
    const ds = new DecompressionStream("gzip");
    const dw = ds.writable.getWriter();
    dw.write(compressed);
    dw.close();

    // KEY: This is the regression path -- new Response(readableStream).text()
    const resp = new Response(ds.readable);
    const text = await resp.text();

    return Response.json({
      match: text === original,
      textLen: text.length,
      originalLen: original.length,
    });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Match       bool `json:"match"`
		TextLen     int  `json:"textLen"`
		OriginalLen int  `json:"originalLen"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !data.Match {
		t.Errorf("new Response(decompressedStream).text() mismatch: got len %d, want %d", data.TextLen, data.OriginalLen)
	}
}

// TestCompression_ResponseArrayBufferFromDecompressionStream verifies that
// new Response(decompressedStream).arrayBuffer() correctly reads a
// ReadableStream body produced by DecompressionStream (gzip round-trip).
func TestCompression_ResponseArrayBufferFromDecompressionStream(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    const original = "Bug2 regression: Response.arrayBuffer() must handle ReadableStream! " +
      "Bug2 regression: Response.arrayBuffer() must handle ReadableStream! " +
      "Bug2 regression: Response.arrayBuffer() must handle ReadableStream!";
    const originalBytes = new TextEncoder().encode(original);

    // Compress
    const cs = new CompressionStream("gzip");
    const cw = cs.writable.getWriter();
    cw.write(originalBytes);
    cw.close();
    const chunks = [];
    const cr = cs.readable.getReader();
    while (true) {
      const { done, value } = await cr.read();
      if (done) break;
      chunks.push(value);
    }
    let cLen = 0;
    for (const c of chunks) cLen += c.length;
    const compressed = new Uint8Array(cLen);
    let off = 0;
    for (const c of chunks) { compressed.set(c, off); off += c.length; }

    // Decompress -- pass the ReadableStream directly to new Response()
    const ds = new DecompressionStream("gzip");
    const dw = ds.writable.getWriter();
    dw.write(compressed);
    dw.close();

    // KEY: This is the regression path -- new Response(readableStream).arrayBuffer()
    const resp = new Response(ds.readable);
    const ab = await resp.arrayBuffer();
    const resultBytes = new Uint8Array(ab);

    // Verify byte-for-byte match
    let match = resultBytes.length === originalBytes.length;
    for (let i = 0; i < originalBytes.length && match; i++) {
      if (resultBytes[i] !== originalBytes[i]) match = false;
    }

    return Response.json({
      match,
      resultLen: resultBytes.length,
      originalLen: originalBytes.length,
    });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Match       bool `json:"match"`
		ResultLen   int  `json:"resultLen"`
		OriginalLen int  `json:"originalLen"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !data.Match {
		t.Errorf("new Response(decompressedStream).arrayBuffer() mismatch: got len %d, want %d", data.ResultLen, data.OriginalLen)
	}
}

// TestCompression_ResponseTextFromCompressionStream verifies that
// new Response(compressedStream).arrayBuffer() works, and the compressed
// bytes can then be decompressed back to the original text. This exercises
// the Response.arrayBuffer() -> ReadableStream path on the compression side.
func TestCompression_ResponseTextFromCompressionStream(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    const original = "Testing Response.arrayBuffer() on compressed stream output. ".repeat(5);
    const encoded = new TextEncoder().encode(original);

    // Compress
    const cs = new CompressionStream("gzip");
    const cw = cs.writable.getWriter();
    cw.write(encoded);
    cw.close();

    // Use new Response(compressedStream).arrayBuffer() to read compressed bytes
    const compResp = new Response(cs.readable);
    const compressedAB = await compResp.arrayBuffer();
    const compressed = new Uint8Array(compressedAB);

    // Now decompress those bytes and use Response.text() to read the result
    const ds = new DecompressionStream("gzip");
    const dw = ds.writable.getWriter();
    dw.write(compressed);
    dw.close();

    const decompResp = new Response(ds.readable);
    const text = await decompResp.text();

    return Response.json({
      match: text === original,
      compressedLen: compressed.length,
      decompressedTextLen: text.length,
      originalLen: original.length,
      wasCompressed: compressed.length < encoded.length,
    });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Match               bool `json:"match"`
		CompressedLen       int  `json:"compressedLen"`
		DecompressedTextLen int  `json:"decompressedTextLen"`
		OriginalLen         int  `json:"originalLen"`
		WasCompressed       bool `json:"wasCompressed"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !data.Match {
		t.Errorf("Response.arrayBuffer()+Response.text() roundtrip mismatch: got len %d, want %d", data.DecompressedTextLen, data.OriginalLen)
	}
	if !data.WasCompressed {
		t.Errorf("compressed (%d) should be smaller than original (%d)", data.CompressedLen, data.OriginalLen)
	}
}

// TestCompression_ResponseTextAllFormats verifies new Response(decompressedStream).text()
// works for all supported compression formats: gzip, deflate, deflate-raw, br.
func TestCompression_ResponseTextAllFormats(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	for _, format := range []string{"gzip", "deflate", "deflate-raw", "br"} {
		t.Run(format, func(t *testing.T) {
			source := `export default {
  async fetch(request, env) {
    const format = "` + format + `";
    const original = "Response.text() regression for " + format + "! ".repeat(10);

    // Compress
    const cs = new CompressionStream(format);
    const cw = cs.writable.getWriter();
    cw.write(new TextEncoder().encode(original));
    cw.close();

    // Read compressed via Response.arrayBuffer()
    const compResp = new Response(cs.readable);
    const compAB = await compResp.arrayBuffer();

    // Decompress
    const ds = new DecompressionStream(format);
    const dw = ds.writable.getWriter();
    dw.write(new Uint8Array(compAB));
    dw.close();

    // Read decompressed via Response.text()
    const decompResp = new Response(ds.readable);
    const text = await decompResp.text();

    return Response.json({
      match: text === original,
      textLen: text.length,
      originalLen: original.length,
    });
  },
};`

			r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
			assertOK(t, r)

			var data struct {
				Match       bool `json:"match"`
				TextLen     int  `json:"textLen"`
				OriginalLen int  `json:"originalLen"`
			}
			if err := json.Unmarshal(r.Response.Body, &data); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if !data.Match {
				t.Errorf("Response.text() for %s: got len %d, want %d", format, data.TextLen, data.OriginalLen)
			}
		})
	}
}

// TestCompression_ResponseJsonFromDecompressionStream verifies that
// new Response(decompressedStream).json() works, since json() delegates
// to text() which uses __readStreamBytes() for ReadableStream bodies.
func TestCompression_ResponseJsonFromDecompressionStream(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    const obj = { key: "value", nested: { arr: [1, 2, 3] }, flag: true };
    const jsonStr = JSON.stringify(obj);

    // Compress the JSON string
    const cs = new CompressionStream("gzip");
    const cw = cs.writable.getWriter();
    cw.write(new TextEncoder().encode(jsonStr));
    cw.close();

    // Read compressed bytes via Response.arrayBuffer()
    const compResp = new Response(cs.readable);
    const compAB = await compResp.arrayBuffer();

    // Decompress and read via Response.json()
    const ds = new DecompressionStream("gzip");
    const dw = ds.writable.getWriter();
    dw.write(new Uint8Array(compAB));
    dw.close();

    const decompResp = new Response(ds.readable);
    const parsed = await decompResp.json();

    return Response.json({
      keyMatch: parsed.key === "value",
      arrMatch: JSON.stringify(parsed.nested.arr) === "[1,2,3]",
      flagMatch: parsed.flag === true,
    });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		KeyMatch  bool `json:"keyMatch"`
		ArrMatch  bool `json:"arrMatch"`
		FlagMatch bool `json:"flagMatch"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !data.KeyMatch {
		t.Error("Response.json() from decompression stream: key mismatch")
	}
	if !data.ArrMatch {
		t.Error("Response.json() from decompression stream: nested array mismatch")
	}
	if !data.FlagMatch {
		t.Error("Response.json() from decompression stream: flag mismatch")
	}
}

// TestCompression_ResponseTextBinaryRoundTrip verifies that binary data
// (all byte values 0-255) survives a compress -> Response.arrayBuffer() ->
// decompress -> Response.arrayBuffer() round-trip without corruption.
func TestCompression_ResponseTextBinaryRoundTrip(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    // Create binary data with all byte values
    const binary = new Uint8Array(256);
    for (let i = 0; i < 256; i++) binary[i] = i;

    // Compress
    const cs = new CompressionStream("gzip");
    const cw = cs.writable.getWriter();
    cw.write(binary);
    cw.close();

    // Read compressed via Response.arrayBuffer()
    const compResp = new Response(cs.readable);
    const compAB = await compResp.arrayBuffer();

    // Decompress
    const ds = new DecompressionStream("gzip");
    const dw = ds.writable.getWriter();
    dw.write(new Uint8Array(compAB));
    dw.close();

    // Read decompressed via Response.arrayBuffer()
    const decompResp = new Response(ds.readable);
    const decompAB = await decompResp.arrayBuffer();
    const result = new Uint8Array(decompAB);

    let match = result.length === 256;
    for (let i = 0; i < 256 && match; i++) {
      if (result[i] !== i) match = false;
    }

    return Response.json({ match, length: result.length });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Match  bool `json:"match"`
		Length int  `json:"length"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !data.Match {
		t.Error("binary data should survive compress->Response.arrayBuffer()->decompress->Response.arrayBuffer() round-trip")
	}
	if data.Length != 256 {
		t.Errorf("decompressed length = %d, want 256", data.Length)
	}
}
