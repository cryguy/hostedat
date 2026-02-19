package worker

import (
	"encoding/json"
	"testing"
)

func TestCrypto_HKDFDeriveBits(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    const ikm = new TextEncoder().encode("input keying material");
    const baseKey = await crypto.subtle.importKey(
      "raw", ikm, { name: "HKDF" }, false, ["deriveBits"]
    );
    const salt = new TextEncoder().encode("salt value");
    const info = new TextEncoder().encode("info value");
    const bits = await crypto.subtle.deriveBits(
      { name: "HKDF", hash: "SHA-256", salt, info }, baseKey, 256
    );
    const arr = new Uint8Array(bits);
    let hex = '';
    for (let i = 0; i < arr.length; i++) {
      hex += arr[i].toString(16).padStart(2, '0');
    }
    return Response.json({ hex, length: arr.length });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Hex    string `json:"hex"`
		Length int    `json:"length"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if data.Length != 32 {
		t.Errorf("derived key length = %d, want 32", data.Length)
	}
	if data.Hex == "" {
		t.Error("derived key is empty")
	}
}

func TestCrypto_HKDFDeriveKey(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    const ikm = new TextEncoder().encode("shared secret");
    const baseKey = await crypto.subtle.importKey(
      "raw", ikm, { name: "HKDF" }, false, ["deriveKey"]
    );
    const salt = new TextEncoder().encode("salt");
    const info = new TextEncoder().encode("context");
    const derivedKey = await crypto.subtle.deriveKey(
      { name: "HKDF", hash: "SHA-256", salt, info },
      baseKey,
      { name: "HMAC", hash: "SHA-256" },
      true,
      ["sign", "verify"]
    );
    // Use derived key to sign and verify
    const msg = new TextEncoder().encode("test message");
    const sig = await crypto.subtle.sign("HMAC", derivedKey, msg);
    const valid = await crypto.subtle.verify("HMAC", derivedKey, sig, msg);
    return Response.json({ valid, sigLen: new Uint8Array(sig).length });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Valid  bool `json:"valid"`
		SigLen int  `json:"sigLen"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if !data.Valid {
		t.Error("HMAC verify with HKDF-derived key should return true")
	}
	if data.SigLen != 32 {
		t.Errorf("signature length = %d, want 32", data.SigLen)
	}
}

func TestCrypto_HKDFDeterministic(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	// Derive the same key twice with the same inputs and verify identical output.
	source := `export default {
  async fetch(request, env) {
    const ikm = new TextEncoder().encode("deterministic test");
    const salt = new TextEncoder().encode("fixed salt");
    const info = new TextEncoder().encode("fixed info");

    const key1 = await crypto.subtle.importKey("raw", ikm, { name: "HKDF" }, false, ["deriveBits"]);
    const bits1 = await crypto.subtle.deriveBits({ name: "HKDF", hash: "SHA-256", salt, info }, key1, 256);

    const key2 = await crypto.subtle.importKey("raw", ikm, { name: "HKDF" }, false, ["deriveBits"]);
    const bits2 = await crypto.subtle.deriveBits({ name: "HKDF", hash: "SHA-256", salt, info }, key2, 256);

    const arr1 = new Uint8Array(bits1);
    const arr2 = new Uint8Array(bits2);
    let match = arr1.length === arr2.length;
    for (let i = 0; i < arr1.length && match; i++) {
      if (arr1[i] !== arr2[i]) match = false;
    }
    return Response.json({ match, length: arr1.length });
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
		t.Error("HKDF deriveBits should be deterministic for same inputs")
	}
}

func TestCrypto_HKDFEmptySalt(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    const ikm = new TextEncoder().encode("test key material");
    const key = await crypto.subtle.importKey("raw", ikm, { name: "HKDF" }, false, ["deriveBits"]);
    // Empty salt (should use zero-filled salt per RFC 5869)
    const bits = await crypto.subtle.deriveBits(
      { name: "HKDF", hash: "SHA-256", salt: new Uint8Array(0), info: new Uint8Array(0) },
      key, 128
    );
    const arr = new Uint8Array(bits);
    return Response.json({ length: arr.length });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Length int `json:"length"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if data.Length != 16 {
		t.Errorf("derived key length = %d, want 16", data.Length)
	}
}

func TestCrypto_PBKDF2DeriveBits(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    const password = new TextEncoder().encode("my-password");
    const baseKey = await crypto.subtle.importKey(
      "raw", password, { name: "PBKDF2" }, false, ["deriveBits"]
    );
    const salt = new TextEncoder().encode("random-salt");
    const bits = await crypto.subtle.deriveBits(
      { name: "PBKDF2", hash: "SHA-256", salt, iterations: 100000 },
      baseKey, 256
    );
    const arr = new Uint8Array(bits);
    let hex = '';
    for (let i = 0; i < arr.length; i++) {
      hex += arr[i].toString(16).padStart(2, '0');
    }
    return Response.json({ hex, length: arr.length });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Hex    string `json:"hex"`
		Length int    `json:"length"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if data.Length != 32 {
		t.Errorf("derived key length = %d, want 32", data.Length)
	}
	if data.Hex == "" {
		t.Error("derived key is empty")
	}
}

func TestCrypto_PBKDF2DeriveKeyForAES(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    const password = new TextEncoder().encode("encrypt-password");
    const baseKey = await crypto.subtle.importKey(
      "raw", password, { name: "PBKDF2" }, false, ["deriveKey"]
    );
    const salt = new TextEncoder().encode("aes-salt");
    const aesKey = await crypto.subtle.deriveKey(
      { name: "PBKDF2", hash: "SHA-256", salt, iterations: 10000 },
      baseKey,
      { name: "AES-GCM", length: 256 },
      false,
      ["encrypt", "decrypt"]
    );
    // Use derived AES key to encrypt and decrypt
    const iv = new Uint8Array(12);
    crypto.getRandomValues(iv);
    const plaintext = new TextEncoder().encode("secret data");
    const ct = await crypto.subtle.encrypt({ name: "AES-GCM", iv }, aesKey, plaintext);
    const pt = await crypto.subtle.decrypt({ name: "AES-GCM", iv }, aesKey, ct);
    const result = new TextDecoder().decode(pt);
    return Response.json({ match: result === "secret data" });
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
		t.Error("PBKDF2-derived AES key should encrypt/decrypt correctly")
	}
}

func TestCrypto_PBKDF2Deterministic(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    const password = new TextEncoder().encode("deterministic");
    const salt = new TextEncoder().encode("fixed");

    const key1 = await crypto.subtle.importKey("raw", password, { name: "PBKDF2" }, false, ["deriveBits"]);
    const bits1 = await crypto.subtle.deriveBits({ name: "PBKDF2", hash: "SHA-256", salt, iterations: 1000 }, key1, 256);

    const key2 = await crypto.subtle.importKey("raw", password, { name: "PBKDF2" }, false, ["deriveBits"]);
    const bits2 = await crypto.subtle.deriveBits({ name: "PBKDF2", hash: "SHA-256", salt, iterations: 1000 }, key2, 256);

    const arr1 = new Uint8Array(bits1);
    const arr2 = new Uint8Array(bits2);
    let match = arr1.length === arr2.length;
    for (let i = 0; i < arr1.length && match; i++) {
      if (arr1[i] !== arr2[i]) match = false;
    }
    return Response.json({ match });
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
		t.Error("PBKDF2 should be deterministic for same inputs")
	}
}

func TestCrypto_HKDFSHA512(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    const ikm = new TextEncoder().encode("sha512 test");
    const key = await crypto.subtle.importKey("raw", ikm, { name: "HKDF" }, false, ["deriveBits"]);
    const bits = await crypto.subtle.deriveBits(
      { name: "HKDF", hash: "SHA-512", salt: new Uint8Array(64), info: new TextEncoder().encode("ctx") },
      key, 512
    );
    return Response.json({ length: new Uint8Array(bits).length });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Length int `json:"length"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if data.Length != 64 {
		t.Errorf("HKDF-SHA512 512-bit output length = %d, want 64", data.Length)
	}
}

func TestCrypto_HKDFSHA1(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    const ikm = new TextEncoder().encode("sha1 test");
    const key = await crypto.subtle.importKey("raw", ikm, { name: "HKDF" }, false, ["deriveBits"]);
    const bits = await crypto.subtle.deriveBits(
      { name: "HKDF", hash: "SHA-1", salt: new Uint8Array(20), info: new TextEncoder().encode("ctx") },
      key, 160
    );
    return Response.json({ length: new Uint8Array(bits).length });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Length int `json:"length"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if data.Length != 20 {
		t.Errorf("HKDF-SHA1 160-bit output length = %d, want 20", data.Length)
	}
}

func TestCrypto_HKDFSHA384(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    const ikm = new TextEncoder().encode("sha384 test");
    const key = await crypto.subtle.importKey("raw", ikm, { name: "HKDF" }, false, ["deriveBits"]);
    const bits = await crypto.subtle.deriveBits(
      { name: "HKDF", hash: "SHA-384", salt: new Uint8Array(48), info: new TextEncoder().encode("ctx") },
      key, 384
    );
    return Response.json({ length: new Uint8Array(bits).length });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Length int `json:"length"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if data.Length != 48 {
		t.Errorf("HKDF-SHA384 384-bit output length = %d, want 48", data.Length)
	}
}

func TestCrypto_PBKDF2SHA512(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    const pw = new TextEncoder().encode("password");
    const key = await crypto.subtle.importKey("raw", pw, { name: "PBKDF2" }, false, ["deriveBits"]);
    const bits = await crypto.subtle.deriveBits(
      { name: "PBKDF2", hash: "SHA-512", salt: new TextEncoder().encode("salt"), iterations: 1000 },
      key, 512
    );
    return Response.json({ length: new Uint8Array(bits).length });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Length int `json:"length"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if data.Length != 64 {
		t.Errorf("PBKDF2-SHA512 512-bit output length = %d, want 64", data.Length)
	}
}

func TestCrypto_PBKDF2SingleIteration(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    const pw = new TextEncoder().encode("pw");
    const key = await crypto.subtle.importKey("raw", pw, { name: "PBKDF2" }, false, ["deriveBits"]);
    const bits = await crypto.subtle.deriveBits(
      { name: "PBKDF2", hash: "SHA-256", salt: new TextEncoder().encode("s"), iterations: 1 },
      key, 256
    );
    const arr = new Uint8Array(bits);
    // Verify it produces output and is deterministic
    const key2 = await crypto.subtle.importKey("raw", pw, { name: "PBKDF2" }, false, ["deriveBits"]);
    const bits2 = await crypto.subtle.deriveBits(
      { name: "PBKDF2", hash: "SHA-256", salt: new TextEncoder().encode("s"), iterations: 1 },
      key2, 256
    );
    const arr2 = new Uint8Array(bits2);
    let match = arr.length === arr2.length;
    for (let i = 0; i < arr.length && match; i++) {
      if (arr[i] !== arr2[i]) match = false;
    }
    return Response.json({ length: arr.length, match });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Length int  `json:"length"`
		Match  bool `json:"match"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if data.Length != 32 {
		t.Errorf("length = %d, want 32", data.Length)
	}
	if !data.Match {
		t.Error("PBKDF2 with 1 iteration should be deterministic")
	}
}
