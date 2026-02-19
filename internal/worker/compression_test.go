package worker

import (
	"encoding/json"
	"testing"
)

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
	json.Unmarshal(r.Response.Body, &data)
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
	json.Unmarshal(r.Response.Body, &data)
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
	json.Unmarshal(r.Response.Body, &data)
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
	json.Unmarshal(r.Response.Body, &data)
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
	json.Unmarshal(r.Response.Body, &data)
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
	json.Unmarshal(r.Response.Body, &data)
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
	json.Unmarshal(r.Response.Body, &data)
	if !data.Threw {
		t.Error("__decompress with unsupported format should throw")
	}
}
