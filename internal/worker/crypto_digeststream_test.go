package worker

import (
	"encoding/json"
	"testing"
)

// ---------------------------------------------------------------------------
// DigestStream Tests
// ---------------------------------------------------------------------------

func TestDigestStream_SHA256KnownHash(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	// SHA-256 of "hello world" = b94d27b9934d3e08a52e52d7da7dabfac484efe37a5380ee9088f7ace2efcde9
	source := `export default {
  async fetch(request, env) {
    const ds = new crypto.DigestStream("SHA-256");
    const writer = ds.getWriter();
    const enc = new TextEncoder();
    await writer.write(enc.encode("hello world"));
    await writer.close();
    const digest = await ds.digest;
    const arr = new Uint8Array(digest);
    const hex = Array.from(arr).map(b => b.toString(16).padStart(2, '0')).join('');
    return Response.json({ hex: hex, len: arr.length });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Hex string `json:"hex"`
		Len int    `json:"len"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	expected := "b94d27b9934d3e08a52e52d7da7dabfac484efe37a5380ee9088f7ace2efcde9"
	if data.Hex != expected {
		t.Errorf("hex = %q, want %q", data.Hex, expected)
	}
	if data.Len != 32 {
		t.Errorf("len = %d, want 32", data.Len)
	}
}

func TestDigestStream_MultipleChunks(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	// Writing "hello" then " world" should produce the same hash as "hello world".
	source := `export default {
  async fetch(request, env) {
    const ds = new crypto.DigestStream("SHA-256");
    const writer = ds.getWriter();
    const enc = new TextEncoder();
    await writer.write(enc.encode("hello"));
    await writer.write(enc.encode(" world"));
    await writer.close();
    const digest = await ds.digest;
    const arr = new Uint8Array(digest);
    const hex = Array.from(arr).map(b => b.toString(16).padStart(2, '0')).join('');
    return Response.json({ hex: hex });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Hex string `json:"hex"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	expected := "b94d27b9934d3e08a52e52d7da7dabfac484efe37a5380ee9088f7ace2efcde9"
	if data.Hex != expected {
		t.Errorf("hex = %q, want %q", data.Hex, expected)
	}
}

func TestDigestStream_ExistsOnCrypto(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    return Response.json({
      hasCryptoDigestStream: typeof crypto.DigestStream === 'function',
      hasGlobalDigestStream: typeof DigestStream === 'function',
    });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		HasCryptoDigestStream bool `json:"hasCryptoDigestStream"`
		HasGlobalDigestStream bool `json:"hasGlobalDigestStream"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !data.HasCryptoDigestStream {
		t.Error("crypto.DigestStream should be a function")
	}
	if !data.HasGlobalDigestStream {
		t.Error("globalThis.DigestStream should be a function")
	}
}

func TestDigestStream_SHA1(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	// SHA-1 of "test" = a94a8fe5ccb19ba61c4c0873d391e987982fbbd3
	source := `export default {
  async fetch(request, env) {
    const ds = new crypto.DigestStream("SHA-1");
    const writer = ds.getWriter();
    await writer.write(new TextEncoder().encode("test"));
    await writer.close();
    const digest = await ds.digest;
    const arr = new Uint8Array(digest);
    const hex = Array.from(arr).map(b => b.toString(16).padStart(2, '0')).join('');
    return Response.json({ hex: hex, len: arr.length });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Hex string `json:"hex"`
		Len int    `json:"len"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	expected := "a94a8fe5ccb19ba61c4c0873d391e987982fbbd3"
	if data.Hex != expected {
		t.Errorf("hex = %q, want %q", data.Hex, expected)
	}
	if data.Len != 20 {
		t.Errorf("len = %d, want 20", data.Len)
	}
}

func TestDigestStream_SHA384(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	// SHA-384 of "test" = 768412320f7b0aa5812fce428dc4706b3cae50e02a64caa16a782249bfe8efc4b7ef1ccb126255d196047dfedf17a0a9
	source := `export default {
  async fetch(request, env) {
    const ds = new crypto.DigestStream("SHA-384");
    const writer = ds.getWriter();
    await writer.write(new TextEncoder().encode("test"));
    await writer.close();
    const digest = await ds.digest;
    const arr = new Uint8Array(digest);
    const hex = Array.from(arr).map(b => b.toString(16).padStart(2, '0')).join('');
    return Response.json({ hex, len: arr.length });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Hex string `json:"hex"`
		Len int    `json:"len"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	expected := "768412320f7b0aa5812fce428dc4706b3cae50e02a64caa16a782249bfe8efc4b7ef1ccb126255d196047dfedf17a0a9"
	if data.Hex != expected {
		t.Errorf("hex = %q, want %q", data.Hex, expected)
	}
	if data.Len != 48 {
		t.Errorf("len = %d, want 48", data.Len)
	}
}

func TestDigestStream_SHA512(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	// SHA-512 of "test" = ee26b0dd4af7e749aa1a8ee3c10ae9923f618980772e473f8819a5d4940e0db27ac185f8a0e1d5f84f88bc887fd67b143732c304cc5fa9ad8e6f57f50028a8ff
	source := `export default {
  async fetch(request, env) {
    const ds = new crypto.DigestStream("SHA-512");
    const writer = ds.getWriter();
    await writer.write(new TextEncoder().encode("test"));
    await writer.close();
    const digest = await ds.digest;
    const arr = new Uint8Array(digest);
    const hex = Array.from(arr).map(b => b.toString(16).padStart(2, '0')).join('');
    return Response.json({ hex, len: arr.length });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Hex string `json:"hex"`
		Len int    `json:"len"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	expected := "ee26b0dd4af7e749aa1a8ee3c10ae9923f618980772e473f8819a5d4940e0db27ac185f8a0e1d5f84f88bc887fd67b143732c304cc5fa9ad8e6f57f50028a8ff"
	if data.Hex != expected {
		t.Errorf("hex = %q, want %q", data.Hex, expected)
	}
	if data.Len != 64 {
		t.Errorf("len = %d, want 64", data.Len)
	}
}

func TestDigestStream_InvalidAlgorithm(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    try {
      const ds = new crypto.DigestStream("SHA-999");
      return Response.json({ threw: false });
    } catch(e) {
      return Response.json({ threw: true, message: e.message || String(e) });
    }
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Threw   bool   `json:"threw"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !data.Threw {
		t.Error("should have thrown for unsupported algorithm SHA-999")
	}
	if data.Message == "" {
		t.Error("error message should not be empty")
	}
	// message should contain "unsupported"
	found := false
	msg := data.Message
	target := "unsupported"
	if len(msg) >= len(target) {
		for i := 0; i <= len(msg)-len(target); i++ {
			if msg[i:i+len(target)] == target {
				found = true
				break
			}
		}
	}
	if !found {
		t.Errorf("message = %q, want it to contain 'unsupported'", data.Message)
	}
}
