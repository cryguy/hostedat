package worker

import (
	"encoding/json"
	"testing"
)

func TestCrypto_GetRandomValues(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request, env) {
    const arr = new Uint8Array(16);
    crypto.getRandomValues(arr);
    // Check that at least some bytes are non-zero (extremely unlikely all zero).
    let nonZero = 0;
    for (let i = 0; i < arr.length; i++) {
      if (arr[i] !== 0) nonZero++;
    }
    return Response.json({ length: arr.length, nonZero: nonZero > 0 });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Length  int  `json:"length"`
		NonZero bool `json:"nonZero"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if data.Length != 16 {
		t.Errorf("length = %d, want 16", data.Length)
	}
	if !data.NonZero {
		t.Error("getRandomValues returned all zeros (extremely unlikely)")
	}
}

func TestCrypto_RandomUUID(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request, env) {
    const uuid = crypto.randomUUID();
    // UUID v4 format: xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx
    const parts = uuid.split('-');
    return Response.json({
      uuid,
      length: uuid.length,
      parts: parts.length,
      version: uuid[14],
    });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		UUID    string `json:"uuid"`
		Length  int    `json:"length"`
		Parts   int    `json:"parts"`
		Version string `json:"version"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if data.Length != 36 {
		t.Errorf("UUID length = %d, want 36", data.Length)
	}
	if data.Parts != 5 {
		t.Errorf("UUID parts = %d, want 5", data.Parts)
	}
	if data.Version != "4" {
		t.Errorf("UUID version = %q, want 4", data.Version)
	}
}

func TestCrypto_SubtleDigestSHA256(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    const data = new TextEncoder().encode("hello");
    const hash = await crypto.subtle.digest("SHA-256", data);
    const arr = new Uint8Array(hash);
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

	// SHA-256 of "hello" = 2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824
	expected := "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824"
	if data.Hex != expected {
		t.Errorf("SHA-256 hex = %q, want %q", data.Hex, expected)
	}
	if data.Length != 32 {
		t.Errorf("hash length = %d, want 32", data.Length)
	}
}

func TestCrypto_SubtleDigestSHA1(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    const data = new TextEncoder().encode("hello");
    const hash = await crypto.subtle.digest("SHA-1", data);
    const arr = new Uint8Array(hash);
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

	// SHA-1 of "hello" = aaf4c61ddcc5e8a2dabede0f3b482cd9aea9434d
	expected := "aaf4c61ddcc5e8a2dabede0f3b482cd9aea9434d"
	if data.Hex != expected {
		t.Errorf("SHA-1 hex = %q, want %q", data.Hex, expected)
	}
	if data.Length != 20 {
		t.Errorf("hash length = %d, want 20", data.Length)
	}
}

func TestCrypto_SubtleDigestSHA384(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    const data = new TextEncoder().encode("hello");
    const hash = await crypto.subtle.digest("SHA-384", data);
    const arr = new Uint8Array(hash);
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

	// SHA-384 of "hello" = 59e1748777448c69de6b800d7a33bbfb9ff1b463e44354c3553bcdb9c666fa90125a3c79f90397bdf5f6a13de828684f
	expected := "59e1748777448c69de6b800d7a33bbfb9ff1b463e44354c3553bcdb9c666fa90125a3c79f90397bdf5f6a13de828684f"
	if data.Hex != expected {
		t.Errorf("SHA-384 hex = %q, want %q", data.Hex, expected)
	}
	if data.Length != 48 {
		t.Errorf("hash length = %d, want 48", data.Length)
	}
}

func TestCrypto_SubtleDigestSHA512(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    const data = new TextEncoder().encode("hello");
    const hash = await crypto.subtle.digest("SHA-512", data);
    const arr = new Uint8Array(hash);
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

	// SHA-512 of "hello" = 9b71d224bd62f3785d96d46ad3ea3d73319bfbc2890caadae2dff72519673ca72323c3d99ba5c11d7c7acc6e14b8c5da0c4663475c2e5c3adef46f73bcdec043
	expected := "9b71d224bd62f3785d96d46ad3ea3d73319bfbc2890caadae2dff72519673ca72323c3d99ba5c11d7c7acc6e14b8c5da0c4663475c2e5c3adef46f73bcdec043"
	if data.Hex != expected {
		t.Errorf("SHA-512 hex = %q, want %q", data.Hex, expected)
	}
	if data.Length != 64 {
		t.Errorf("hash length = %d, want 64", data.Length)
	}
}

// TestCrypto_DigestDataWithNullBytes verifies that digest handles data
// containing null bytes correctly. This is a regression test for the
// null-byte truncation bug in the QJS/Go string boundary.
func TestCrypto_DigestDataWithNullBytes(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    // Data with embedded null bytes: [0x00, 0x01, 0x00, 0x02, 0x00]
    const data = new Uint8Array([0x00, 0x01, 0x00, 0x02, 0x00]);
    const hash = await crypto.subtle.digest("SHA-256", data);
    const arr = new Uint8Array(hash);
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

	// SHA-256 of [0x00, 0x01, 0x00, 0x02, 0x00] =
	// Computed via: printf '\x00\x01\x00\x02\x00' | sha256sum
	expected := "c7e5eb4738fcb5aff8c9ba9016737117167aecc5b371eb07f65caf981d9be0a1"
	if data.Hex != expected {
		t.Errorf("SHA-256 hex = %q, want %q", data.Hex, expected)
	}
	if data.Length != 32 {
		t.Errorf("hash length = %d, want 32", data.Length)
	}
}

func TestCrypto_SubtleHMACSignVerify(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    const keyData = new TextEncoder().encode("my-secret-key-0123456789abcdef");
    const key = await crypto.subtle.importKey(
      "raw", keyData, { name: "HMAC", hash: "SHA-256" }, true, ["sign", "verify"]
    );
    const data = new TextEncoder().encode("message to sign");
    const signature = await crypto.subtle.sign("HMAC", key, data);
    const valid = await crypto.subtle.verify("HMAC", key, signature, data);
    const tampered = new TextEncoder().encode("tampered message");
    const invalid = await crypto.subtle.verify("HMAC", key, signature, tampered);
    return Response.json({ valid, invalid, sigLength: new Uint8Array(signature).length });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Valid     bool `json:"valid"`
		Invalid   bool `json:"invalid"`
		SigLength int  `json:"sigLength"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if !data.Valid {
		t.Error("HMAC verify should return true for correct data")
	}
	if data.Invalid {
		t.Error("HMAC verify should return false for tampered data")
	}
	if data.SigLength != 32 {
		t.Errorf("HMAC-SHA256 signature length = %d, want 32", data.SigLength)
	}
}

func TestCrypto_SubtleHMACSHA512(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    const keyData = new TextEncoder().encode("my-secret-key-0123456789abcdef");
    const key = await crypto.subtle.importKey(
      "raw", keyData, { name: "HMAC", hash: "SHA-512" }, true, ["sign", "verify"]
    );
    const data = new TextEncoder().encode("message to sign");
    const signature = await crypto.subtle.sign("HMAC", key, data);
    const valid = await crypto.subtle.verify("HMAC", key, signature, data);
    const tampered = new TextEncoder().encode("tampered message");
    const invalid = await crypto.subtle.verify("HMAC", key, signature, tampered);
    return Response.json({ valid, invalid, sigLength: new Uint8Array(signature).length });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Valid     bool `json:"valid"`
		Invalid   bool `json:"invalid"`
		SigLength int  `json:"sigLength"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if !data.Valid {
		t.Error("HMAC-SHA512 verify should return true for correct data")
	}
	if data.Invalid {
		t.Error("HMAC-SHA512 verify should return false for tampered data")
	}
	// SHA-512 produces 64-byte signatures, not 32.
	if data.SigLength != 64 {
		t.Errorf("HMAC-SHA512 signature length = %d, want 64", data.SigLength)
	}
}

// TestCrypto_HMACWithNullBytesInKey is a deterministic regression test.
// A key containing embedded null bytes must produce correct HMAC signatures
// that verify successfully.
func TestCrypto_HMACWithNullBytesInKey(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    // 32-byte key with null bytes at positions 0, 2, 4, 15, 16, 31.
    const keyData = new Uint8Array([
      0x00, 0xAA, 0x00, 0xBB, 0x00, 0xCC, 0xDD, 0xEE,
      0xFF, 0x11, 0x22, 0x33, 0x44, 0x55, 0x66, 0x00,
      0x00, 0x77, 0x88, 0x99, 0xAA, 0xBB, 0xCC, 0xDD,
      0xEE, 0xFF, 0x01, 0x02, 0x03, 0x04, 0x05, 0x00,
    ]);
    const key = await crypto.subtle.importKey(
      "raw", keyData, { name: "HMAC", hash: "SHA-256" }, true, ["sign", "verify"]
    );
    const data = new TextEncoder().encode("test message");
    const signature = await crypto.subtle.sign("HMAC", key, data);
    const valid = await crypto.subtle.verify("HMAC", key, signature, data);

    // Also verify exportKey preserves the null bytes.
    const exported = await crypto.subtle.exportKey("raw", key);
    const exportedArr = new Uint8Array(exported);
    let keyMatch = exportedArr.length === 32;
    for (let i = 0; i < keyData.length && keyMatch; i++) {
      if (exportedArr[i] !== keyData[i]) keyMatch = false;
    }

    return Response.json({
      valid,
      sigLen: new Uint8Array(signature).length,
      keyLen: exportedArr.length,
      keyMatch,
    });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Valid    bool `json:"valid"`
		SigLen   int  `json:"sigLen"`
		KeyLen   int  `json:"keyLen"`
		KeyMatch bool `json:"keyMatch"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if !data.Valid {
		t.Error("HMAC with null-byte key: verify should return true")
	}
	if data.SigLen != 32 {
		t.Errorf("signature length = %d, want 32", data.SigLen)
	}
	if data.KeyLen != 32 {
		t.Errorf("exported key length = %d, want 32", data.KeyLen)
	}
	if !data.KeyMatch {
		t.Error("exported key does not match imported key (null bytes corrupted)")
	}
}

func TestCrypto_SubtleAESGCMEncryptDecrypt(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    // 128-bit key (16 bytes).
    const keyData = new Uint8Array(16);
    crypto.getRandomValues(keyData);
    const key = await crypto.subtle.importKey(
      "raw", keyData, { name: "AES-GCM" }, false, ["encrypt", "decrypt"]
    );
    // 96-bit IV (12 bytes, standard for AES-GCM).
    const iv = new Uint8Array(12);
    crypto.getRandomValues(iv);
    const plaintext = new TextEncoder().encode("secret data here");
    const ciphertext = await crypto.subtle.encrypt(
      { name: "AES-GCM", iv }, key, plaintext
    );
    const decrypted = await crypto.subtle.decrypt(
      { name: "AES-GCM", iv }, key, ciphertext
    );
    const result = new TextDecoder().decode(decrypted);
    return Response.json({
      match: result === "secret data here",
      ctLength: new Uint8Array(ciphertext).length,
    });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Match    bool `json:"match"`
		CtLength int  `json:"ctLength"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if !data.Match {
		t.Error("AES-GCM decrypt should return original plaintext")
	}
	// AES-GCM adds a 16-byte auth tag. Input is 16 bytes, output should be 32.
	if data.CtLength != 32 {
		t.Errorf("ciphertext length = %d, want 32 (16 data + 16 tag)", data.CtLength)
	}
}

// TestCrypto_AESGCMWithNullBytesInKeyAndIV is a deterministic regression test
// for the null-byte truncation bug. Uses a fixed key and IV with embedded 0x00
// bytes to guarantee the exact scenario that previously failed.
func TestCrypto_AESGCMWithNullBytesInKeyAndIV(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    // 16-byte key with null bytes at positions 1, 3, 7, 15.
    const keyData = new Uint8Array([
      0xDE, 0x00, 0xAD, 0x00, 0xBE, 0xEF, 0xCA, 0x00,
      0xFE, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x00,
    ]);
    // 12-byte IV with null bytes at positions 0 and 5.
    const iv = new Uint8Array([
      0x00, 0x11, 0x22, 0x33, 0x44, 0x00, 0x66, 0x77,
      0x88, 0x99, 0xAA, 0xBB,
    ]);
    const key = await crypto.subtle.importKey(
      "raw", keyData, { name: "AES-GCM" }, false, ["encrypt", "decrypt"]
    );
    const plaintext = new TextEncoder().encode("null byte key+iv test");
    const ciphertext = await crypto.subtle.encrypt(
      { name: "AES-GCM", iv }, key, plaintext
    );
    const decrypted = await crypto.subtle.decrypt(
      { name: "AES-GCM", iv }, key, ciphertext
    );
    const result = new TextDecoder().decode(decrypted);
    return Response.json({
      match: result === "null byte key+iv test",
      ctLen: new Uint8Array(ciphertext).length,
      ptLen: plaintext.length,
    });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Match bool `json:"match"`
		CtLen int  `json:"ctLen"`
		PtLen int  `json:"ptLen"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if !data.Match {
		t.Error("AES-GCM with null-byte key+IV: decrypt should return original plaintext")
	}
	// plaintext "null byte key+iv test" is 21 bytes + 16-byte tag = 37
	if data.CtLen != data.PtLen+16 {
		t.Errorf("ciphertext length = %d, want %d (plaintext %d + 16 tag)", data.CtLen, data.PtLen+16, data.PtLen)
	}
}

// TestCrypto_AESGCMAllZeroKey tests the extreme case where every byte
// of the key and IV is 0x00.
func TestCrypto_AESGCMAllZeroKey(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    const keyData = new Uint8Array(16); // all zeros
    const iv = new Uint8Array(12);      // all zeros
    const key = await crypto.subtle.importKey(
      "raw", keyData, { name: "AES-GCM" }, false, ["encrypt", "decrypt"]
    );
    const plaintext = new TextEncoder().encode("all-zero key and iv");
    const ciphertext = await crypto.subtle.encrypt(
      { name: "AES-GCM", iv }, key, plaintext
    );
    const decrypted = await crypto.subtle.decrypt(
      { name: "AES-GCM", iv }, key, ciphertext
    );
    const result = new TextDecoder().decode(decrypted);
    return Response.json({ match: result === "all-zero key and iv" });
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
		t.Error("AES-GCM with all-zero key+IV: decrypt should return original plaintext")
	}
}

func TestCrypto_AESGCMRejectsInvalidIVLength(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    const keyData = new Uint8Array(16);
    crypto.getRandomValues(keyData);
    const key = await crypto.subtle.importKey(
      "raw", keyData, { name: "AES-GCM" }, false, ["encrypt", "decrypt"]
    );
    // Wrong IV length: 8 bytes instead of 12.
    const badIV = new Uint8Array(8);
    crypto.getRandomValues(badIV);
    try {
      await crypto.subtle.encrypt({ name: "AES-GCM", iv: badIV }, key, new Uint8Array([1,2,3]));
      return Response.json({ error: false });
    } catch(e) {
      return Response.json({ error: true, message: String(e) });
    }
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Error   bool   `json:"error"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if !data.Error {
		t.Error("AES-GCM encrypt should reject non-12-byte IV")
	}
}

func TestCrypto_KeysIsolatedPerRequest(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	// First request imports a key and signs.
	source1 := `export default {
  async fetch(request, env) {
    const keyData = new TextEncoder().encode("request-one-secret-key!!");
    const key = await crypto.subtle.importKey(
      "raw", keyData, { name: "HMAC", hash: "SHA-256" }, true, ["sign"]
    );
    const sig = await crypto.subtle.sign("HMAC", key, new TextEncoder().encode("msg"));
    return Response.json({ sigLen: new Uint8Array(sig).length });
  },
};`

	r1 := execJS(t, e, source1, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r1)

	// Second request tries to use key ID 1 (same ID the first request used)
	// but should fail because keys are scoped per-request.
	source2 := `export default {
  async fetch(request, env) {
    try {
      // Try to export key ID 1 from a previous request — should fail.
      const b64 = __cryptoExportKey(1);
      return Response.json({ leaked: true });
    } catch(e) {
      return Response.json({ leaked: false, error: String(e) });
    }
  },
};`

	r2 := execJS(t, e, source2, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r2)

	var data struct {
		Leaked bool   `json:"leaked"`
		Error  string `json:"error"`
	}
	if err := json.Unmarshal(r2.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if data.Leaked {
		t.Error("key material from a previous request should not be accessible")
	}
}

// TestCrypto_ImportExportKeyWithNullBytes verifies that importKey/exportKey
// preserves key material containing null bytes through the full round-trip.
func TestCrypto_DigestUnsupportedAlgo(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    try {
      await crypto.subtle.digest("MD5", new TextEncoder().encode("test"));
      return Response.json({ threw: false });
    } catch(e) {
      return Response.json({ threw: true, msg: String(e) });
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
		t.Error("digest with unsupported algorithm should throw")
	}
}

func TestCrypto_ImportKeyNonRawFormat(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    try {
      await crypto.subtle.importKey("pkcs8", new Uint8Array(16), { name: "HMAC", hash: "SHA-256" }, false, ["sign"]);
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
		t.Error("importKey with non-raw format should throw")
	}
}

func TestCrypto_ExportKeyUnsupportedFormat(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    const key = await crypto.subtle.importKey(
      "raw", new Uint8Array(16), { name: "HMAC", hash: "SHA-256" }, true, ["sign"]
    );
    try {
      await crypto.subtle.exportKey("pkcs8", key);
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
		t.Error("exportKey with unsupported format should throw")
	}
}

func TestCrypto_SignUnsupportedAlgo(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    const key = await crypto.subtle.importKey(
      "raw", new Uint8Array(16), { name: "HMAC", hash: "SHA-256" }, false, ["sign"]
    );
    try {
      await crypto.subtle.sign("UNKNOWN-ALGO", key, new TextEncoder().encode("test"));
      return Response.json({ threw: false });
    } catch(e) {
      return Response.json({ threw: true, msg: String(e) });
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
		t.Error("sign with unsupported algorithm should throw")
	}
}

func TestCrypto_EncryptUnsupportedAlgo(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    const key = await crypto.subtle.importKey(
      "raw", new Uint8Array(16), { name: "AES-GCM" }, false, ["encrypt"]
    );
    try {
      await crypto.subtle.encrypt({ name: "AES-CTR", iv: new Uint8Array(12) }, key, new Uint8Array(8));
      return Response.json({ threw: false });
    } catch(e) {
      return Response.json({ threw: true, msg: String(e) });
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
		t.Error("encrypt with unsupported algorithm should throw")
	}
}

func TestCrypto_DigestAlgoAsObject(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    const data = new TextEncoder().encode("hello");
    const hash = await crypto.subtle.digest({name: "SHA-256"}, data);
    return Response.json({ len: new Uint8Array(hash).length });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Len int `json:"len"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatal(err)
	}
	if data.Len != 32 {
		t.Errorf("digest length = %d, want 32", data.Len)
	}
}

func TestCrypto_GetRandomValuesRejectsNull(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request, env) {
    let threw = false;
    try {
      crypto.getRandomValues(null);
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
		t.Fatal(err)
	}
	if !data.Threw {
		t.Error("getRandomValues(null) should throw")
	}
}

func TestCrypto_RandomUUIDUniqueness(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request, env) {
    const uuids = new Set();
    for (let i = 0; i < 100; i++) {
      uuids.add(crypto.randomUUID());
    }
    return Response.json({ unique: uuids.size });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Unique int `json:"unique"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatal(err)
	}
	if data.Unique != 100 {
		t.Errorf("expected 100 unique UUIDs, got %d", data.Unique)
	}
}

func TestCrypto_RSAGenerateKeyAndSign(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    const keyPair = await crypto.subtle.generateKey(
      { name: "RSASSA-PKCS1-v1_5", modulusLength: 2048, publicExponent: new Uint8Array([1, 0, 1]), hash: "SHA-256" },
      true, ["sign", "verify"]
    );
    const data = new TextEncoder().encode("test message");
    const sig = await crypto.subtle.sign("RSASSA-PKCS1-v1_5", keyPair.privateKey, data);
    const valid = await crypto.subtle.verify("RSASSA-PKCS1-v1_5", keyPair.publicKey, sig, data);
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
		t.Error("RSA PKCS1v15 signature should verify")
	}
	if data.SigLen != 256 {
		t.Errorf("sigLen = %d, want 256 (2048-bit key)", data.SigLen)
	}
}

func TestCrypto_RSAOAEPEncryptDecrypt(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    const keyPair = await crypto.subtle.generateKey(
      { name: "RSA-OAEP", modulusLength: 2048, publicExponent: new Uint8Array([1, 0, 1]), hash: "SHA-256" },
      true, ["encrypt", "decrypt"]
    );
    const plaintext = new TextEncoder().encode("secret data");
    const ct = await crypto.subtle.encrypt({ name: "RSA-OAEP" }, keyPair.publicKey, plaintext);
    const pt = await crypto.subtle.decrypt({ name: "RSA-OAEP" }, keyPair.privateKey, ct);
    const decoded = new TextDecoder().decode(pt);
    return Response.json({ decoded, ctLen: new Uint8Array(ct).length });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Decoded string `json:"decoded"`
		CtLen   int    `json:"ctLen"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if data.Decoded != "secret data" {
		t.Errorf("decoded = %q, want 'secret data'", data.Decoded)
	}
}

func TestCrypto_RSAPSSSignVerify(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    const keyPair = await crypto.subtle.generateKey(
      { name: "RSA-PSS", modulusLength: 2048, publicExponent: new Uint8Array([1, 0, 1]), hash: "SHA-256" },
      true, ["sign", "verify"]
    );
    const data = new TextEncoder().encode("pss test");
    const sig = await crypto.subtle.sign({ name: "RSA-PSS", saltLength: 32 }, keyPair.privateKey, data);
    const valid = await crypto.subtle.verify({ name: "RSA-PSS", saltLength: 32 }, keyPair.publicKey, sig, data);
    return Response.json({ valid });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Valid bool `json:"valid"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !data.Valid {
		t.Error("RSA-PSS signature should verify")
	}
}

func TestCrypto_RSAExportImportJWK(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    const keyPair = await crypto.subtle.generateKey(
      { name: "RSASSA-PKCS1-v1_5", modulusLength: 2048, publicExponent: new Uint8Array([1, 0, 1]), hash: "SHA-256" },
      true, ["sign", "verify"]
    );
    const pubJWK = await crypto.subtle.exportKey("jwk", keyPair.publicKey);
    const privJWK = await crypto.subtle.exportKey("jwk", keyPair.privateKey);

    // Re-import public key from JWK.
    const importedPub = await crypto.subtle.importKey(
      "jwk", pubJWK,
      { name: "RSASSA-PKCS1-v1_5", hash: "SHA-256" },
      true, ["verify"]
    );

    // Sign with original, verify with imported.
    const data = new TextEncoder().encode("jwk round-trip");
    const sig = await crypto.subtle.sign("RSASSA-PKCS1-v1_5", keyPair.privateKey, data);
    const valid = await crypto.subtle.verify("RSASSA-PKCS1-v1_5", importedPub, sig, data);

    return Response.json({
      valid,
      pubKty: pubJWK.kty,
      privHasD: !!privJWK.d,
      pubAlg: pubJWK.alg,
    });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Valid    bool   `json:"valid"`
		PubKty   string `json:"pubKty"`
		PrivHasD bool   `json:"privHasD"`
		PubAlg   string `json:"pubAlg"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !data.Valid {
		t.Error("JWK import/verify should work")
	}
	if data.PubKty != "RSA" {
		t.Errorf("pubKty = %q, want RSA", data.PubKty)
	}
	if !data.PrivHasD {
		t.Error("private JWK should have d field")
	}
	if data.PubAlg != "RS256" {
		t.Errorf("pubAlg = %q, want RS256", data.PubAlg)
	}
}

func TestCrypto_RSAExportImportSPKI(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    const keyPair = await crypto.subtle.generateKey(
      { name: "RSASSA-PKCS1-v1_5", modulusLength: 2048, publicExponent: new Uint8Array([1, 0, 1]), hash: "SHA-256" },
      true, ["sign", "verify"]
    );
    const spki = await crypto.subtle.exportKey("spki", keyPair.publicKey);
    const imported = await crypto.subtle.importKey(
      "spki", spki,
      { name: "RSASSA-PKCS1-v1_5", hash: "SHA-256" },
      true, ["verify"]
    );

    const data = new TextEncoder().encode("spki test");
    const sig = await crypto.subtle.sign("RSASSA-PKCS1-v1_5", keyPair.privateKey, data);
    const valid = await crypto.subtle.verify("RSASSA-PKCS1-v1_5", imported, sig, data);
    return Response.json({ valid, spkiLen: new Uint8Array(spki).length });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Valid   bool `json:"valid"`
		SPKILen int  `json:"spkiLen"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !data.Valid {
		t.Error("SPKI export/import should verify correctly")
	}
	if data.SPKILen == 0 {
		t.Error("SPKI export should produce non-empty data")
	}
}

func TestCrypto_RSAExportImportPKCS8(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    const keyPair = await crypto.subtle.generateKey(
      { name: "RSASSA-PKCS1-v1_5", modulusLength: 2048, publicExponent: new Uint8Array([1, 0, 1]), hash: "SHA-256" },
      true, ["sign", "verify"]
    );
    const pkcs8 = await crypto.subtle.exportKey("pkcs8", keyPair.privateKey);
    const imported = await crypto.subtle.importKey(
      "pkcs8", pkcs8,
      { name: "RSASSA-PKCS1-v1_5", hash: "SHA-256" },
      true, ["sign"]
    );

    const data = new TextEncoder().encode("pkcs8 test");
    const sig = await crypto.subtle.sign("RSASSA-PKCS1-v1_5", imported, data);
    const valid = await crypto.subtle.verify("RSASSA-PKCS1-v1_5", keyPair.publicKey, sig, data);
    return Response.json({ valid, pkcs8Len: new Uint8Array(pkcs8).length });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Valid    bool `json:"valid"`
		PKCS8Len int  `json:"pkcs8Len"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !data.Valid {
		t.Error("PKCS8 export/import should verify correctly")
	}
}

func TestCrypto_AESCBCEncryptDecrypt(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    const key = await crypto.subtle.generateKey(
      { name: "AES-CBC" }, true, ["encrypt", "decrypt"]
    );
    const iv = crypto.getRandomValues(new Uint8Array(16));
    const plaintext = new TextEncoder().encode("hello aes-cbc");
    const ct = await crypto.subtle.encrypt({ name: "AES-CBC", iv }, key, plaintext);
    const pt = await crypto.subtle.decrypt({ name: "AES-CBC", iv }, key, ct);
    const decoded = new TextDecoder().decode(pt);
    return Response.json({ decoded });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Decoded string `json:"decoded"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if data.Decoded != "hello aes-cbc" {
		t.Errorf("decoded = %q, want 'hello aes-cbc'", data.Decoded)
	}
}

func TestCrypto_ECDSAGenerateKeySignVerify(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    const keyPair = await crypto.subtle.generateKey(
      { name: "ECDSA", namedCurve: "P-256" }, true, ["sign", "verify"]
    );
    const data = new TextEncoder().encode("ecdsa test");
    const sig = await crypto.subtle.sign(
      { name: "ECDSA", hash: "SHA-256" }, keyPair.privateKey, data
    );
    const valid = await crypto.subtle.verify(
      { name: "ECDSA", hash: "SHA-256" }, keyPair.publicKey, sig, data
    );
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
		t.Error("ECDSA P-256 signature should verify")
	}
	if data.SigLen != 64 {
		t.Errorf("sigLen = %d, want 64 (P-256)", data.SigLen)
	}
}

func TestCrypto_ECDSAExportImportJWK(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    const keyPair = await crypto.subtle.generateKey(
      { name: "ECDSA", namedCurve: "P-256" }, true, ["sign", "verify"]
    );
    const pubJWK = await crypto.subtle.exportKey("jwk", keyPair.publicKey);
    const privJWK = await crypto.subtle.exportKey("jwk", keyPair.privateKey);

    // Reimport private from JWK and sign.
    const imported = await crypto.subtle.importKey(
      "jwk", privJWK,
      { name: "ECDSA", namedCurve: "P-256" }, true, ["sign"]
    );
    const data = new TextEncoder().encode("ec jwk round-trip");
    const sig = await crypto.subtle.sign({ name: "ECDSA", hash: "SHA-256" }, imported, data);
    const valid = await crypto.subtle.verify(
      { name: "ECDSA", hash: "SHA-256" }, keyPair.publicKey, sig, data
    );
    return Response.json({
      valid,
      pubKty: pubJWK.kty,
      pubCrv: pubJWK.crv,
      privHasD: !!privJWK.d,
    });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Valid    bool   `json:"valid"`
		PubKty   string `json:"pubKty"`
		PubCrv   string `json:"pubCrv"`
		PrivHasD bool   `json:"privHasD"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !data.Valid {
		t.Error("ECDSA JWK round-trip should verify")
	}
	if data.PubKty != "EC" {
		t.Errorf("kty = %q, want EC", data.PubKty)
	}
	if data.PubCrv != "P-256" {
		t.Errorf("crv = %q, want P-256", data.PubCrv)
	}
	if !data.PrivHasD {
		t.Error("private JWK should have d field")
	}
}

func TestCrypto_HMACGenerateKey(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    const key = await crypto.subtle.generateKey(
      { name: "HMAC", hash: "SHA-256" }, true, ["sign", "verify"]
    );
    const data = new TextEncoder().encode("generated key test");
    const sig = await crypto.subtle.sign("HMAC", key, data);
    const valid = await crypto.subtle.verify("HMAC", key, sig, data);
    const exported = await crypto.subtle.exportKey("raw", key);
    return Response.json({ valid, keyLen: new Uint8Array(exported).length });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Valid  bool `json:"valid"`
		KeyLen int  `json:"keyLen"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !data.Valid {
		t.Error("HMAC generateKey sign/verify should work")
	}
	if data.KeyLen != 32 {
		t.Errorf("keyLen = %d, want 32 (SHA-256)", data.KeyLen)
	}
}

func TestCrypto_Ed25519SignVerify(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    const keyPair = await crypto.subtle.generateKey(
      { name: "Ed25519" }, true, ["sign", "verify"]
    );
    const data = new TextEncoder().encode("ed25519 test message");
    const sig = await crypto.subtle.sign("Ed25519", keyPair.privateKey, data);
    const valid = await crypto.subtle.verify("Ed25519", keyPair.publicKey, sig, data);
    const tampered = new TextEncoder().encode("tampered");
    const invalid = await crypto.subtle.verify("Ed25519", keyPair.publicKey, sig, tampered);
    return Response.json({ valid, invalid, sigLen: new Uint8Array(sig).length });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Valid   bool `json:"valid"`
		Invalid bool `json:"invalid"`
		SigLen  int  `json:"sigLen"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !data.Valid {
		t.Error("Ed25519 signature should verify")
	}
	if data.Invalid {
		t.Error("Ed25519 verify should fail for tampered data")
	}
	if data.SigLen != 64 {
		t.Errorf("sigLen = %d, want 64", data.SigLen)
	}
}

func TestCrypto_Ed25519ExportImportJWK(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    const keyPair = await crypto.subtle.generateKey(
      { name: "Ed25519" }, true, ["sign", "verify"]
    );
    const pubJWK = await crypto.subtle.exportKey("jwk", keyPair.publicKey);
    const privJWK = await crypto.subtle.exportKey("jwk", keyPair.privateKey);

    // Re-import and verify round-trip.
    const imported = await crypto.subtle.importKey(
      "jwk", privJWK, { name: "Ed25519" }, true, ["sign"]
    );
    const data = new TextEncoder().encode("ed25519 jwk");
    const sig = await crypto.subtle.sign("Ed25519", imported, data);
    const valid = await crypto.subtle.verify("Ed25519", keyPair.publicKey, sig, data);

    return Response.json({
      valid,
      pubKty: pubJWK.kty,
      pubCrv: pubJWK.crv,
      privHasD: !!privJWK.d,
    });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Valid    bool   `json:"valid"`
		PubKty   string `json:"pubKty"`
		PubCrv   string `json:"pubCrv"`
		PrivHasD bool   `json:"privHasD"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !data.Valid {
		t.Error("Ed25519 JWK round-trip should verify")
	}
	if data.PubKty != "OKP" {
		t.Errorf("kty = %q, want OKP", data.PubKty)
	}
	if data.PubCrv != "Ed25519" {
		t.Errorf("crv = %q, want Ed25519", data.PubCrv)
	}
}

func TestCrypto_HKDFDeriveBitsRoundTrip(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    const keyMaterial = new TextEncoder().encode("my-secret-key");
    const baseKey = await crypto.subtle.importKey(
      "raw", keyMaterial, { name: "HKDF" }, false, ["deriveBits"]
    );
    const salt = new TextEncoder().encode("salt");
    const info = new TextEncoder().encode("info");
    const derived = await crypto.subtle.deriveBits(
      { name: "HKDF", hash: "SHA-256", salt, info },
      baseKey, 256
    );
    return Response.json({ len: new Uint8Array(derived).length });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Len int `json:"len"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if data.Len != 32 {
		t.Errorf("derived key len = %d, want 32", data.Len)
	}
}

func TestCrypto_PBKDF2DeriveBitsRoundTrip(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    const keyMaterial = new TextEncoder().encode("password");
    const baseKey = await crypto.subtle.importKey(
      "raw", keyMaterial, { name: "PBKDF2" }, false, ["deriveBits"]
    );
    const salt = new TextEncoder().encode("salt-value");
    const derived = await crypto.subtle.deriveBits(
      { name: "PBKDF2", hash: "SHA-256", salt, iterations: 1000 },
      baseKey, 256
    );
    return Response.json({ len: new Uint8Array(derived).length });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Len int `json:"len"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if data.Len != 32 {
		t.Errorf("derived key len = %d, want 32", data.Len)
	}
}

func TestCrypto_HKDFDeriveKeyToHMAC(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    const keyMaterial = new TextEncoder().encode("secret-key-material");
    const baseKey = await crypto.subtle.importKey(
      "raw", keyMaterial, { name: "HKDF" }, false, ["deriveKey"]
    );
    const derivedKey = await crypto.subtle.deriveKey(
      { name: "HKDF", hash: "SHA-256", salt: new Uint8Array(16), info: new Uint8Array(0) },
      baseKey,
      { name: "HMAC", hash: "SHA-256", length: 256 },
      true, ["sign"]
    );
    const exported = await crypto.subtle.exportKey("raw", derivedKey);
    const data = new TextEncoder().encode("test");
    const sig = await crypto.subtle.sign("HMAC", derivedKey, data);
    return Response.json({
      keyLen: new Uint8Array(exported).length,
      sigLen: new Uint8Array(sig).length,
    });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		KeyLen int `json:"keyLen"`
		SigLen int `json:"sigLen"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if data.KeyLen != 32 {
		t.Errorf("keyLen = %d, want 32", data.KeyLen)
	}
	if data.SigLen != 32 {
		t.Errorf("sigLen = %d, want 32", data.SigLen)
	}
}

func TestCrypto_DirectGoCallbackErrors(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	// Test all __crypto* Go callbacks with bad arguments directly.
	source := `export default {
  async fetch(request, env) {
    const results = {};

    // __cryptoGetRandomBytes with bad n.
    try { __cryptoGetRandomBytes(0); results.badN = false; }
    catch(e) { results.badN = true; }

    // __cryptoGetRandomBytes with too large n.
    try { __cryptoGetRandomBytes(100000); results.largeN = false; }
    catch(e) { results.largeN = true; }

    // __cryptoDigest with missing args.
    try { __cryptoDigest("SHA-256"); results.digestMissing = false; }
    catch(e) { results.digestMissing = true; }

    // __cryptoDigest with invalid base64.
    try { __cryptoDigest("SHA-256", "not-valid-base64!!!"); results.digestBadB64 = false; }
    catch(e) { results.digestBadB64 = true; }

    // __cryptoImportKey with missing args.
    try { __cryptoImportKey("HMAC"); results.importMissing = false; }
    catch(e) { results.importMissing = true; }

    // __cryptoImportKey with invalid base64.
    try { __cryptoImportKey("HMAC", "SHA-256", "not-valid-b64!!!"); results.importBadB64 = false; }
    catch(e) { results.importBadB64 = true; }

    // __cryptoExportKey with invalid key ID.
    try { __cryptoExportKey(9999); results.exportBadKey = false; }
    catch(e) { results.exportBadKey = true; }

    // __cryptoSign with missing args.
    try { __cryptoSign("HMAC"); results.signMissing = false; }
    catch(e) { results.signMissing = true; }

    // __cryptoSign with invalid base64.
    try { __cryptoSign("HMAC", 0, "bad-b64!!!"); results.signBadB64 = false; }
    catch(e) { results.signBadB64 = true; }

    // __cryptoSign with bad key ID.
    try { __cryptoSign("HMAC", 9999, btoa("data")); results.signBadKey = false; }
    catch(e) { results.signBadKey = true; }

    // __cryptoVerify with missing args.
    try { __cryptoVerify("HMAC", 0, btoa("sig")); results.verifyMissing = false; }
    catch(e) { results.verifyMissing = true; }

    // __cryptoVerify with bad sig base64.
    try { __cryptoVerify("HMAC", 0, "bad-sig!!!", btoa("data")); results.verifyBadSig = false; }
    catch(e) { results.verifyBadSig = true; }

    // __cryptoVerify with bad data base64.
    try { __cryptoVerify("HMAC", 0, btoa("sig"), "bad-data!!!"); results.verifyBadData = false; }
    catch(e) { results.verifyBadData = true; }

    // __cryptoVerify with bad key ID.
    try { __cryptoVerify("HMAC", 9999, btoa("sig"), btoa("data")); results.verifyBadKey = false; }
    catch(e) { results.verifyBadKey = true; }

    // __cryptoEncrypt with missing args.
    try { __cryptoEncrypt("AES-GCM", 0, btoa("data")); results.encryptMissing = false; }
    catch(e) { results.encryptMissing = true; }

    // __cryptoEncrypt with bad data base64.
    try { __cryptoEncrypt("AES-GCM", 0, "bad!!!", btoa("iv")); results.encryptBadData = false; }
    catch(e) { results.encryptBadData = true; }

    // __cryptoEncrypt with bad key ID.
    try { __cryptoEncrypt("AES-GCM", 9999, btoa("data"), btoa("iv")); results.encryptBadKey = false; }
    catch(e) { results.encryptBadKey = true; }

    // __cryptoDecrypt with missing args.
    try { __cryptoDecrypt("AES-GCM", 0, btoa("data")); results.decryptMissing = false; }
    catch(e) { results.decryptMissing = true; }

    // __cryptoDecrypt with bad data base64.
    try { __cryptoDecrypt("AES-GCM", 0, "bad!!!", btoa("iv")); results.decryptBadData = false; }
    catch(e) { results.decryptBadData = true; }

    // __cryptoDecrypt with bad key ID.
    try { __cryptoDecrypt("AES-GCM", 9999, btoa("data"), btoa("iv")); results.decryptBadKey = false; }
    catch(e) { results.decryptBadKey = true; }

    return Response.json(results);
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var results map[string]bool
	if err := json.Unmarshal(r.Response.Body, &results); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	expected := []string{
		"badN", "largeN", "digestMissing", "digestBadB64",
		"importMissing", "importBadB64", "exportBadKey",
		"signMissing", "signBadB64", "signBadKey",
		"verifyMissing", "verifyBadSig", "verifyBadData", "verifyBadKey",
		"encryptMissing", "encryptBadData", "encryptBadKey",
		"decryptMissing", "decryptBadData", "decryptBadKey",
	}
	for _, key := range expected {
		if !results[key] {
			t.Errorf("%s: expected error to be thrown (true), got %v", key, results[key])
		}
	}
}

func TestCrypto_ECDSAP384(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    const keyPair = await crypto.subtle.generateKey(
      { name: "ECDSA", namedCurve: "P-384" }, true, ["sign", "verify"]
    );
    const data = new TextEncoder().encode("p384 test");
    const sig = await crypto.subtle.sign(
      { name: "ECDSA", hash: "SHA-384" }, keyPair.privateKey, data
    );
    const valid = await crypto.subtle.verify(
      { name: "ECDSA", hash: "SHA-384" }, keyPair.publicKey, sig, data
    );
    const jwk = await crypto.subtle.exportKey("jwk", keyPair.publicKey);
    return Response.json({ valid, crv: jwk.crv, sigLen: new Uint8Array(sig).length });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Valid  bool   `json:"valid"`
		Crv    string `json:"crv"`
		SigLen int    `json:"sigLen"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !data.Valid {
		t.Error("ECDSA P-384 should verify")
	}
	if data.Crv != "P-384" {
		t.Errorf("crv = %q, want P-384", data.Crv)
	}
	if data.SigLen != 96 {
		t.Errorf("sigLen = %d, want 96 (P-384)", data.SigLen)
	}
}

func TestCrypto_AESCBCGenerateKey(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    const key = await crypto.subtle.generateKey(
      { name: "AES-CBC", length: 256 }, true, ["encrypt", "decrypt"]
    );
    const exported = await crypto.subtle.exportKey("raw", key);
    const iv = crypto.getRandomValues(new Uint8Array(16));
    const pt = new TextEncoder().encode("aes-cbc-256 test data here");
    const ct = await crypto.subtle.encrypt({ name: "AES-CBC", iv }, key, pt);
    const decrypted = await crypto.subtle.decrypt({ name: "AES-CBC", iv }, key, ct);
    const decoded = new TextDecoder().decode(decrypted);
    return Response.json({ keyLen: new Uint8Array(exported).length, decoded });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		KeyLen  int    `json:"keyLen"`
		Decoded string `json:"decoded"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if data.KeyLen != 32 {
		t.Errorf("keyLen = %d, want 32", data.KeyLen)
	}
	if data.Decoded != "aes-cbc-256 test data here" {
		t.Errorf("decoded = %q", data.Decoded)
	}
}

func TestCrypto_DecryptUnsupportedAlgo(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    const key = await crypto.subtle.importKey(
      "raw", new Uint8Array(16), { name: "AES-GCM" }, false, ["decrypt"]
    );
    try {
      await crypto.subtle.decrypt({ name: "AES-CTR", iv: new Uint8Array(12) }, key, new Uint8Array(32));
      return Response.json({ threw: false });
    } catch(e) {
      return Response.json({ threw: true, msg: String(e) });
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
		t.Error("decrypt with unsupported algorithm should throw")
	}
}

func TestCrypto_VerifyUnsupportedAlgo(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    const key = await crypto.subtle.importKey(
      "raw", new Uint8Array(32), { name: "HMAC", hash: "SHA-256" }, false, ["verify"]
    );
    try {
      await crypto.subtle.verify("UNKNOWN", key, new Uint8Array(32), new Uint8Array(8));
      return Response.json({ threw: false });
    } catch(e) {
      return Response.json({ threw: true });
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
		t.Error("verify with unsupported algorithm should throw")
	}
}

func TestCrypto_ImportExportKeyWithNullBytes(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    // Key material: every other byte is 0x00.
    const keyData = new Uint8Array([
      0x00, 0xFF, 0x00, 0xFF, 0x00, 0xFF, 0x00, 0xFF,
      0x00, 0xFF, 0x00, 0xFF, 0x00, 0xFF, 0x00, 0xFF,
    ]);
    const key = await crypto.subtle.importKey(
      "raw", keyData, { name: "HMAC", hash: "SHA-256" }, true, ["sign"]
    );
    const exported = await crypto.subtle.exportKey("raw", key);
    const exportedArr = new Uint8Array(exported);

    // Verify byte-by-byte match.
    let match = exportedArr.length === keyData.length;
    const diffs = [];
    for (let i = 0; i < keyData.length; i++) {
      if (exportedArr[i] !== keyData[i]) {
        match = false;
        diffs.push({ i, got: exportedArr[i], want: keyData[i] });
      }
    }
    return Response.json({ match, exportedLen: exportedArr.length, diffs });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Match       bool `json:"match"`
		ExportedLen int  `json:"exportedLen"`
		Diffs       []struct {
			I    int `json:"i"`
			Got  int `json:"got"`
			Want int `json:"want"`
		} `json:"diffs"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if data.ExportedLen != 16 {
		t.Errorf("exported key length = %d, want 16", data.ExportedLen)
	}
	if !data.Match {
		for _, d := range data.Diffs {
			t.Errorf("byte[%d]: got 0x%02x, want 0x%02x", d.I, d.Got, d.Want)
		}
		t.Error("import/export round-trip corrupted key with null bytes")
	}
}

func TestCrypto_DirectGoCallbackMoreErrors(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	// Test additional Go callback error paths not covered by DirectGoCallbackErrors.
	source := `export default {
  async fetch(request, env) {
    const results = {};

    // __cryptoGetRandomBytes with no args.
    try { __cryptoGetRandomBytes(); results.noArgs = false; }
    catch(e) { results.noArgs = true; }

    // __cryptoExportKey with no args.
    try { __cryptoExportKey(); results.exportNoArgs = false; }
    catch(e) { results.exportNoArgs = true; }

    // __cryptoDigest with unsupported algorithm and valid base64.
    try { __cryptoDigest("MD5", btoa("data")); results.digestUnsupported = false; }
    catch(e) { results.digestUnsupported = true; }

    // Import a valid HMAC key, then test sign/verify/encrypt/decrypt with unsupported algo.
    const keyID = __cryptoImportKey("HMAC", "SHA-256", btoa("my-secret-key-for-testing-purpose"));

    // __cryptoSign with unsupported algorithm.
    try { __cryptoSign("UNKNOWN-ALGO", keyID, btoa("data")); results.signUnsupportedAlgo = false; }
    catch(e) { results.signUnsupportedAlgo = true; }

    // __cryptoVerify with unsupported algorithm.
    try { __cryptoVerify("UNKNOWN-ALGO", keyID, btoa("sig"), btoa("data")); results.verifyUnsupportedAlgo = false; }
    catch(e) { results.verifyUnsupportedAlgo = true; }

    // __cryptoEncrypt with unsupported algorithm.
    try { __cryptoEncrypt("UNKNOWN-ALGO", keyID, btoa("data"), btoa("iv")); results.encryptUnsupportedAlgo = false; }
    catch(e) { results.encryptUnsupportedAlgo = true; }

    // __cryptoDecrypt with unsupported algorithm.
    try { __cryptoDecrypt("UNKNOWN-ALGO", keyID, btoa("data"), btoa("iv")); results.decryptUnsupportedAlgo = false; }
    catch(e) { results.decryptUnsupportedAlgo = true; }

    return Response.json(results);
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var results map[string]bool
	if err := json.Unmarshal(r.Response.Body, &results); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	expected := []string{
		"noArgs", "exportNoArgs", "digestUnsupported",
		"signUnsupportedAlgo", "verifyUnsupportedAlgo",
		"encryptUnsupportedAlgo", "decryptUnsupportedAlgo",
	}
	for _, key := range expected {
		if !results[key] {
			t.Errorf("%s: expected error to be thrown (true), got %v", key, results[key])
		}
	}
}

func TestCrypto_AESGCMBadIVErrors(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	// Test AES-GCM encrypt/decrypt with bad IV (wrong length, bad base64).
	source := `export default {
  async fetch(request, env) {
    const results = {};

    // Import a 128-bit AES key via Go callback.
    const keyBytes = new Uint8Array(16);
    crypto.getRandomValues(keyBytes);
    const keyB64 = btoa(String.fromCharCode(...keyBytes));
    const keyID = __cryptoImportKey("AES-GCM", "SHA-256", keyB64);

    // Encrypt with bad IV base64.
    try { __cryptoEncrypt("AES-GCM", keyID, btoa("plaintext"), "bad-iv!!!"); results.encBadIVB64 = false; }
    catch(e) { results.encBadIVB64 = true; }

    // Encrypt with wrong IV length (5 bytes instead of 12).
    try { __cryptoEncrypt("AES-GCM", keyID, btoa("plaintext"), btoa("short")); results.encBadIVLen = false; }
    catch(e) { results.encBadIVLen = true; }

    // Decrypt with bad IV base64.
    try { __cryptoDecrypt("AES-GCM", keyID, btoa("ciphertext"), "bad-iv!!!"); results.decBadIVB64 = false; }
    catch(e) { results.decBadIVB64 = true; }

    // Decrypt with wrong IV length.
    try { __cryptoDecrypt("AES-GCM", keyID, btoa("ciphertext"), btoa("short")); results.decBadIVLen = false; }
    catch(e) { results.decBadIVLen = true; }

    // Decrypt with correct IV length but corrupt ciphertext.
    const iv12 = new Uint8Array(12);
    crypto.getRandomValues(iv12);
    const ivB64 = btoa(String.fromCharCode(...iv12));
    try { __cryptoDecrypt("AES-GCM", keyID, btoa("corrupt-ciphertext-data"), ivB64); results.decCorrupt = false; }
    catch(e) { results.decCorrupt = true; }

    return Response.json(results);
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var results map[string]bool
	if err := json.Unmarshal(r.Response.Body, &results); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	expected := []string{
		"encBadIVB64", "encBadIVLen",
		"decBadIVB64", "decBadIVLen", "decCorrupt",
	}
	for _, key := range expected {
		if !results[key] {
			t.Errorf("%s: expected error to be thrown (true), got %v", key, results[key])
		}
	}
}

func TestCrypto_HMACSignVerifyBadHash(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	// Test HMAC sign/verify with an unsupported hash algorithm.
	source := `export default {
  async fetch(request, env) {
    const results = {};

    // Import key with a weird hash algo.
    const keyID = __cryptoImportKey("HMAC", "MD5", btoa("key-data-for-test"));

    // Sign with HMAC but key has unsupported hash.
    try { __cryptoSign("HMAC", keyID, btoa("data")); results.signBadHash = false; }
    catch(e) { results.signBadHash = true; }

    // Verify with HMAC but key has unsupported hash.
    try { __cryptoVerify("HMAC", keyID, btoa("sig"), btoa("data")); results.verifyBadHash = false; }
    catch(e) { results.verifyBadHash = true; }

    return Response.json(results);
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var results map[string]bool
	if err := json.Unmarshal(r.Response.Body, &results); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if !results["signBadHash"] {
		t.Error("HMAC sign with unsupported hash should throw")
	}
	if !results["verifyBadHash"] {
		t.Error("HMAC verify with unsupported hash should throw")
	}
}

func TestCrypto_AESGCMRoundTripDirect(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	// Test AES-GCM encrypt/decrypt round-trip via direct Go callbacks.
	source := `export default {
  async fetch(request, env) {
    // Generate a 256-bit key.
    const keyBytes = new Uint8Array(32);
    crypto.getRandomValues(keyBytes);
    const keyB64 = btoa(String.fromCharCode(...keyBytes));
    const keyID = __cryptoImportKey("AES-GCM", "", keyB64);

    // Generate 12-byte IV.
    const iv = new Uint8Array(12);
    crypto.getRandomValues(iv);
    const ivB64 = btoa(String.fromCharCode(...iv));

    // Encrypt.
    const plaintext = "Hello, AES-GCM direct test!";
    const ptB64 = btoa(plaintext);
    const ctB64 = __cryptoEncrypt("AES-GCM", keyID, ptB64, ivB64);

    // Decrypt.
    const rtB64 = __cryptoDecrypt("AES-GCM", keyID, ctB64, ivB64);
    const roundTrip = atob(rtB64);

    return Response.json({
      match: roundTrip === plaintext,
      original: plaintext,
      roundTrip,
    });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Match     bool   `json:"match"`
		Original  string `json:"original"`
		RoundTrip string `json:"roundTrip"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if !data.Match {
		t.Errorf("AES-GCM round-trip failed: original=%q roundTrip=%q", data.Original, data.RoundTrip)
	}
}

func TestCrypto_HMACSignVerifyDirect(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	// Test HMAC sign/verify via direct Go callbacks.
	source := `export default {
  async fetch(request, env) {
    // Import HMAC key.
    const keyID = __cryptoImportKey("HMAC", "SHA-256", btoa("my-hmac-key-data"));

    // Sign.
    const data = btoa("message to sign");
    const sigB64 = __cryptoSign("HMAC", keyID, data);

    // Verify correct signature.
    const valid = __cryptoVerify("HMAC", keyID, sigB64, data);

    // Verify wrong signature.
    const invalid = __cryptoVerify("HMAC", keyID, btoa("wrong-sig"), data);

    // Export key.
    const exportedB64 = __cryptoExportKey(keyID);
    const exported = atob(exportedB64);

    return Response.json({
      sigExists: sigB64.length > 0,
      valid,
      invalid,
      exportMatch: exported === "my-hmac-key-data",
    });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		SigExists   bool `json:"sigExists"`
		Valid       bool `json:"valid"`
		Invalid     bool `json:"invalid"`
		ExportMatch bool `json:"exportMatch"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if !data.SigExists {
		t.Error("signature should not be empty")
	}
	if !data.Valid {
		t.Error("HMAC verify should return true for correct signature")
	}
	if data.Invalid {
		t.Error("HMAC verify should return false for wrong signature")
	}
	if !data.ExportMatch {
		t.Error("exported key should match original")
	}
}

func TestCrypto_DigestAllAlgosDirect(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	// Test all digest algorithms via direct Go callback.
	source := `export default {
  async fetch(request, env) {
    const data = btoa("hello world");
    const results = {};

    // SHA-1
    const sha1 = __cryptoDigest("SHA-1", data);
    results.sha1Len = atob(sha1).length;

    // SHA-256
    const sha256 = __cryptoDigest("SHA-256", data);
    results.sha256Len = atob(sha256).length;

    // SHA-384
    const sha384 = __cryptoDigest("SHA-384", data);
    results.sha384Len = atob(sha384).length;

    // SHA-512
    const sha512 = __cryptoDigest("SHA-512", data);
    results.sha512Len = atob(sha512).length;

    return Response.json(results);
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		SHA1Len   int `json:"sha1Len"`
		SHA256Len int `json:"sha256Len"`
		SHA384Len int `json:"sha384Len"`
		SHA512Len int `json:"sha512Len"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if data.SHA1Len != 20 {
		t.Errorf("SHA-1 digest length = %d, want 20", data.SHA1Len)
	}
	if data.SHA256Len != 32 {
		t.Errorf("SHA-256 digest length = %d, want 32", data.SHA256Len)
	}
	if data.SHA384Len != 48 {
		t.Errorf("SHA-384 digest length = %d, want 48", data.SHA384Len)
	}
	if data.SHA512Len != 64 {
		t.Errorf("SHA-512 digest length = %d, want 64", data.SHA512Len)
	}
}

func TestCrypto_ImportKeyBadBase64(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    let importFailed = false;
    try {
      await crypto.subtle.importKey(
        "raw", new TextEncoder().encode("bad!!!"), { name: "HMAC", hash: "SHA-256" }, false, ["sign"]
      );
    } catch(e) {
      importFailed = true;
    }
    // Direct callback with explicitly bad base64
    let directFailed = false;
    try {
      __cryptoImportKey("HMAC", "SHA-256", "not-valid-base64!!!");
    } catch(e) {
      directFailed = true;
    }
    return Response.json({ importFailed, directFailed });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		ImportFailed bool `json:"importFailed"`
		DirectFailed bool `json:"directFailed"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatal(err)
	}
	if !data.DirectFailed {
		t.Error("__cryptoImportKey with bad base64 should throw")
	}
}

func TestCrypto_SignBadDataBase64(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    const key = await crypto.subtle.importKey(
      "raw", new TextEncoder().encode("key123456789012345678901234567890"),
      { name: "HMAC", hash: "SHA-256" }, false, ["sign"]
    );
    // Try direct sign with bad data base64
    let signFailed = false;
    try {
      __cryptoSign("HMAC", 0, "bad-base64!!!");
    } catch(e) {
      signFailed = true;
    }
    // Try direct verify with bad sig base64
    let verifySigFailed = false;
    try {
      __cryptoVerify("HMAC", 0, "bad!!!", btoa("data"));
    } catch(e) {
      verifySigFailed = true;
    }
    // Try direct verify with bad data base64
    let verifyDataFailed = false;
    try {
      __cryptoVerify("HMAC", 0, btoa("sig"), "bad!!!");
    } catch(e) {
      verifyDataFailed = true;
    }
    return Response.json({ signFailed, verifySigFailed, verifyDataFailed });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		SignFailed       bool `json:"signFailed"`
		VerifySigFailed  bool `json:"verifySigFailed"`
		VerifyDataFailed bool `json:"verifyDataFailed"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatal(err)
	}
	if !data.SignFailed {
		t.Error("sign with bad base64 should throw")
	}
	if !data.VerifySigFailed {
		t.Error("verify with bad sig base64 should throw")
	}
	if !data.VerifyDataFailed {
		t.Error("verify with bad data base64 should throw")
	}
}

func TestCrypto_EncryptDecryptMissingArgs(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request, env) {
    const results = {};
    // encrypt missing args
    try { __cryptoEncrypt("AES-GCM", 0, btoa("data")); results.encMissing = false; }
    catch(e) { results.encMissing = true; }

    // decrypt missing args
    try { __cryptoDecrypt("AES-GCM", 0, btoa("data")); results.decMissing = false; }
    catch(e) { results.decMissing = true; }

    // sign missing args
    try { __cryptoSign("HMAC", 0); results.signMissing = false; }
    catch(e) { results.signMissing = true; }

    // verify missing args
    try { __cryptoVerify("HMAC", 0, btoa("sig")); results.verifyMissing = false; }
    catch(e) { results.verifyMissing = true; }

    // importKey missing args
    try { __cryptoImportKey("HMAC"); results.importMissing = false; }
    catch(e) { results.importMissing = true; }

    // digest missing args
    try { __cryptoDigest("SHA-256"); results.digestMissing = false; }
    catch(e) { results.digestMissing = true; }

    // digest bad base64
    try { __cryptoDigest("SHA-256", "bad!!!"); results.digestBadB64 = false; }
    catch(e) { results.digestBadB64 = true; }

    return Response.json(results);
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var results map[string]bool
	if err := json.Unmarshal(r.Response.Body, &results); err != nil {
		t.Fatal(err)
	}

	expected := []string{"encMissing", "decMissing", "signMissing", "verifyMissing", "importMissing", "digestMissing", "digestBadB64"}
	for _, key := range expected {
		if !results[key] {
			t.Errorf("%s: expected error (true), got %v", key, results[key])
		}
	}
}

func TestCrypto_EncryptDecryptBadKeyID(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request, env) {
    const results = {};
    // encrypt with bad key
    try { __cryptoEncrypt("AES-GCM", 9999, btoa("data"), btoa("123456789012")); results.encBadKey = false; }
    catch(e) { results.encBadKey = true; }

    // decrypt with bad key
    try { __cryptoDecrypt("AES-GCM", 9999, btoa("data"), btoa("123456789012")); results.decBadKey = false; }
    catch(e) { results.decBadKey = true; }

    // sign with bad key
    try { __cryptoSign("HMAC", 9999, btoa("data")); results.signBadKey = false; }
    catch(e) { results.signBadKey = true; }

    // verify with bad key
    try { __cryptoVerify("HMAC", 9999, btoa("sig"), btoa("data")); results.verifyBadKey = false; }
    catch(e) { results.verifyBadKey = true; }

    // export bad key
    try { __cryptoExportKey(9999); results.exportBadKey = false; }
    catch(e) { results.exportBadKey = true; }

    return Response.json(results);
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var results map[string]bool
	if err := json.Unmarshal(r.Response.Body, &results); err != nil {
		t.Fatal(err)
	}

	expected := []string{"encBadKey", "decBadKey", "signBadKey", "verifyBadKey", "exportBadKey"}
	for _, key := range expected {
		if !results[key] {
			t.Errorf("%s: expected error (true), got %v", key, results[key])
		}
	}
}

func TestCrypto_EncryptBadBase64Data(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    const results = {};
    // Import a valid AES key first
    const key = await crypto.subtle.generateKey(
      { name: "AES-GCM", length: 256 }, true, ["encrypt", "decrypt"]
    );
    const rawKey = await crypto.subtle.exportKey("raw", key);
    const keyB64 = btoa(String.fromCharCode(...new Uint8Array(rawKey)));
    const keyId = __cryptoImportKey("AES-GCM", "", keyB64);

    // encrypt with bad data base64
    try { __cryptoEncrypt("AES-GCM", keyId, "bad!!!", btoa("123456789012")); results.encBadData = false; }
    catch(e) { results.encBadData = true; }

    // decrypt with bad data base64
    try { __cryptoDecrypt("AES-GCM", keyId, "bad!!!", btoa("123456789012")); results.decBadData = false; }
    catch(e) { results.decBadData = true; }

    return Response.json(results);
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var results map[string]bool
	if err := json.Unmarshal(r.Response.Body, &results); err != nil {
		t.Fatal(err)
	}

	if !results["encBadData"] {
		t.Error("encrypt with bad data base64 should throw")
	}
	if !results["decBadData"] {
		t.Error("decrypt with bad data base64 should throw")
	}
}

func TestCrypto_GetRandomBytesEdgeCases(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request, env) {
    const results = {};

    // Zero length should error
    try { __cryptoGetRandomBytes(0); results.zeroLen = false; }
    catch(e) { results.zeroLen = true; }

    // Negative length should error
    try { __cryptoGetRandomBytes(-1); results.negLen = false; }
    catch(e) { results.negLen = true; }

    // Over 65536 should error
    try { __cryptoGetRandomBytes(65537); results.overMax = false; }
    catch(e) { results.overMax = true; }

    // Valid length should work
    const bytes = __cryptoGetRandomBytes(16);
    results.validLen = typeof bytes === 'string' && bytes.length > 0;

    return Response.json(results);
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var results map[string]bool
	if err := json.Unmarshal(r.Response.Body, &results); err != nil {
		t.Fatal(err)
	}
	if !results["zeroLen"] {
		t.Error("getRandomBytes(0) should throw")
	}
	if !results["negLen"] {
		t.Error("getRandomBytes(-1) should throw")
	}
	if !results["overMax"] {
		t.Error("getRandomBytes(65537) should throw")
	}
	if !results["validLen"] {
		t.Error("getRandomBytes(16) should return a valid string")
	}
}

func TestCrypto_RandomUUIDv4Format(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request, env) {
    const uuid1 = crypto.randomUUID();
    const uuid2 = crypto.randomUUID();
    // UUID v4 format: xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx
    const uuidRegex = /^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/;
    return Response.json({
      valid1: uuidRegex.test(uuid1),
      valid2: uuidRegex.test(uuid2),
      different: uuid1 !== uuid2,
    });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Valid1    bool `json:"valid1"`
		Valid2    bool `json:"valid2"`
		Different bool `json:"different"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatal(err)
	}
	if !data.Valid1 {
		t.Error("first UUID should be valid v4")
	}
	if !data.Valid2 {
		t.Error("second UUID should be valid v4")
	}
	if !data.Different {
		t.Error("two UUIDs should be different")
	}
}

func TestCrypto_GenerateKeyHMAC(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    const key = await crypto.subtle.generateKey(
      { name: "HMAC", hash: "SHA-256" }, true, ["sign", "verify"]
    );
    const data = new TextEncoder().encode("test data");
    const sig = await crypto.subtle.sign("HMAC", key, data);
    const valid = await crypto.subtle.verify("HMAC", key, sig, data);
    const exported = await crypto.subtle.exportKey("raw", key);
    return Response.json({
      valid,
      keyType: key.type,
      exportLen: exported.byteLength,
    });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Valid     bool   `json:"valid"`
		KeyType   string `json:"keyType"`
		ExportLen int    `json:"exportLen"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatal(err)
	}
	if !data.Valid {
		t.Error("HMAC sign/verify with generated key should be valid")
	}
	if data.KeyType != "secret" {
		t.Errorf("keyType = %q, want secret", data.KeyType)
	}
	if data.ExportLen != 32 {
		t.Errorf("exported key length = %d, want 32", data.ExportLen)
	}
}

func TestCrypto_GenerateKeyAESGCM(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    const key = await crypto.subtle.generateKey(
      { name: "AES-GCM", length: 256 }, true, ["encrypt", "decrypt"]
    );
    const iv = crypto.getRandomValues(new Uint8Array(12));
    const plaintext = new TextEncoder().encode("hello aes-gcm");
    const ct = await crypto.subtle.encrypt({ name: "AES-GCM", iv }, key, plaintext);
    const pt = await crypto.subtle.decrypt({ name: "AES-GCM", iv }, key, ct);
    const decoded = new TextDecoder().decode(pt);
    return Response.json({
      keyType: key.type,
      roundtrip: decoded,
    });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		KeyType   string `json:"keyType"`
		Roundtrip string `json:"roundtrip"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatal(err)
	}
	if data.KeyType != "secret" {
		t.Errorf("keyType = %q, want secret", data.KeyType)
	}
	if data.Roundtrip != "hello aes-gcm" {
		t.Errorf("roundtrip = %q, want 'hello aes-gcm'", data.Roundtrip)
	}
}

func TestCrypto_GenerateKeyAESCBC(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    const key = await crypto.subtle.generateKey(
      { name: "AES-CBC", length: 256 }, true, ["encrypt", "decrypt"]
    );
    const iv = crypto.getRandomValues(new Uint8Array(16));
    const plaintext = new TextEncoder().encode("hello aes-cbc roundtrip");
    const ct = await crypto.subtle.encrypt({ name: "AES-CBC", iv }, key, plaintext);
    const pt = await crypto.subtle.decrypt({ name: "AES-CBC", iv }, key, ct);
    const decoded = new TextDecoder().decode(pt);
    return Response.json({
      keyType: key.type,
      roundtrip: decoded,
      ctLen: ct.byteLength,
    });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		KeyType   string `json:"keyType"`
		Roundtrip string `json:"roundtrip"`
		CtLen     int    `json:"ctLen"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatal(err)
	}
	if data.KeyType != "secret" {
		t.Errorf("keyType = %q, want secret", data.KeyType)
	}
	if data.Roundtrip != "hello aes-cbc roundtrip" {
		t.Errorf("roundtrip = %q", data.Roundtrip)
	}
	// AES-CBC with PKCS7 padding: 22 bytes input -> 32 bytes ciphertext
	if data.CtLen != 32 {
		t.Errorf("ciphertext length = %d, want 32", data.CtLen)
	}
}

func TestCrypto_ExportImportJWK_HMAC(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    const key = await crypto.subtle.importKey(
      "raw",
      new TextEncoder().encode("my-secret-key-32-bytes-long!!!!"),
      { name: "HMAC", hash: "SHA-256" },
      true, ["sign", "verify"]
    );
    const jwk = await crypto.subtle.exportKey("jwk", key);
    const reimported = await crypto.subtle.importKey(
      "jwk", jwk, { name: "HMAC", hash: "SHA-256" }, true, ["sign", "verify"]
    );
    const data = new TextEncoder().encode("test");
    const sig = await crypto.subtle.sign("HMAC", key, data);
    const valid = await crypto.subtle.verify("HMAC", reimported, sig, data);
    return Response.json({
      jwkKty: jwk.kty,
      jwkAlg: jwk.alg,
      valid,
    });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		JwkKty string `json:"jwkKty"`
		JwkAlg string `json:"jwkAlg"`
		Valid  bool   `json:"valid"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatal(err)
	}
	if data.JwkKty != "oct" {
		t.Errorf("jwk.kty = %q, want oct", data.JwkKty)
	}
	if data.JwkAlg != "HS256" {
		t.Errorf("jwk.alg = %q, want HS256", data.JwkAlg)
	}
	if !data.Valid {
		t.Error("verify with reimported JWK key should succeed")
	}
}

func TestCrypto_ExportImportJWK_AESGCM(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    const key = await crypto.subtle.generateKey(
      { name: "AES-GCM", length: 256 }, true, ["encrypt", "decrypt"]
    );
    const jwk = await crypto.subtle.exportKey("jwk", key);
    const reimported = await crypto.subtle.importKey(
      "jwk", jwk, { name: "AES-GCM" }, true, ["encrypt", "decrypt"]
    );
    const iv = crypto.getRandomValues(new Uint8Array(12));
    const ct = await crypto.subtle.encrypt({ name: "AES-GCM", iv }, key, new TextEncoder().encode("jwk test"));
    const pt = await crypto.subtle.decrypt({ name: "AES-GCM", iv }, reimported, ct);
    return Response.json({
      jwkKty: jwk.kty,
      jwkAlg: jwk.alg,
      roundtrip: new TextDecoder().decode(pt),
    });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		JwkKty    string `json:"jwkKty"`
		JwkAlg    string `json:"jwkAlg"`
		Roundtrip string `json:"roundtrip"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatal(err)
	}
	if data.JwkKty != "oct" {
		t.Errorf("jwk.kty = %q, want oct", data.JwkKty)
	}
	if data.JwkAlg != "A256GCM" {
		t.Errorf("jwk.alg = %q, want A256GCM", data.JwkAlg)
	}
	if data.Roundtrip != "jwk test" {
		t.Errorf("roundtrip = %q", data.Roundtrip)
	}
}

func TestCrypto_AESCBCImportEncryptDecrypt(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    const keyData = crypto.getRandomValues(new Uint8Array(32));
    const key = await crypto.subtle.importKey(
      "raw", keyData, { name: "AES-CBC" }, false, ["encrypt", "decrypt"]
    );
    const iv = crypto.getRandomValues(new Uint8Array(16));
    const plaintext = new TextEncoder().encode("aes-cbc import test");
    const ct = await crypto.subtle.encrypt({ name: "AES-CBC", iv }, key, plaintext);
    const pt = await crypto.subtle.decrypt({ name: "AES-CBC", iv }, key, ct);
    return Response.json({
      roundtrip: new TextDecoder().decode(pt),
    });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Roundtrip string `json:"roundtrip"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatal(err)
	}
	if data.Roundtrip != "aes-cbc import test" {
		t.Errorf("roundtrip = %q", data.Roundtrip)
	}
}

func TestCrypto_GenerateKeyECDSA(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    const keyPair = await crypto.subtle.generateKey(
      { name: "ECDSA", namedCurve: "P-256" }, true, ["sign", "verify"]
    );
    const data = new TextEncoder().encode("ecdsa generate test");
    const sig = await crypto.subtle.sign(
      { name: "ECDSA", hash: "SHA-256" }, keyPair.privateKey, data
    );
    const valid = await crypto.subtle.verify(
      { name: "ECDSA", hash: "SHA-256" }, keyPair.publicKey, sig, data
    );
    return Response.json({
      valid,
      privType: keyPair.privateKey.type,
      pubType: keyPair.publicKey.type,
    });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Valid    bool   `json:"valid"`
		PrivType string `json:"privType"`
		PubType  string `json:"pubType"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatal(err)
	}
	if !data.Valid {
		t.Error("ECDSA sign/verify with generated key should be valid")
	}
	if data.PrivType != "private" {
		t.Errorf("privateKey.type = %q, want private", data.PrivType)
	}
	if data.PubType != "public" {
		t.Errorf("publicKey.type = %q, want public", data.PubType)
	}
}

func TestCrypto_ExportKeyJWK_ECDSA(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    const keyPair = await crypto.subtle.generateKey(
      { name: "ECDSA", namedCurve: "P-256" }, true, ["sign", "verify"]
    );
    const pubJwk = await crypto.subtle.exportKey("jwk", keyPair.publicKey);
    const privJwk = await crypto.subtle.exportKey("jwk", keyPair.privateKey);
    return Response.json({
      pubKty: pubJwk.kty,
      pubCrv: pubJwk.crv,
      pubHasX: typeof pubJwk.x === "string",
      pubHasY: typeof pubJwk.y === "string",
      pubHasD: pubJwk.d !== undefined,
      privHasD: privJwk.d !== undefined,
    });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		PubKty   string `json:"pubKty"`
		PubCrv   string `json:"pubCrv"`
		PubHasX  bool   `json:"pubHasX"`
		PubHasY  bool   `json:"pubHasY"`
		PubHasD  bool   `json:"pubHasD"`
		PrivHasD bool   `json:"privHasD"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatal(err)
	}
	if data.PubKty != "EC" {
		t.Errorf("pubJwk.kty = %q, want EC", data.PubKty)
	}
	if data.PubCrv != "P-256" {
		t.Errorf("pubJwk.crv = %q, want P-256", data.PubCrv)
	}
	if !data.PubHasX || !data.PubHasY {
		t.Error("public JWK should have x and y")
	}
	if data.PubHasD {
		t.Error("public JWK should NOT have d")
	}
	if !data.PrivHasD {
		t.Error("private JWK should have d")
	}
}

func TestCrypto_ImportKeyJWK_ECDSA(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    const keyPair = await crypto.subtle.generateKey(
      { name: "ECDSA", namedCurve: "P-256" }, true, ["sign", "verify"]
    );
    const data = new TextEncoder().encode("jwk ecdsa roundtrip");
    const sig = await crypto.subtle.sign(
      { name: "ECDSA", hash: "SHA-256" }, keyPair.privateKey, data
    );
    // Export public key as JWK, reimport, and verify
    const pubJwk = await crypto.subtle.exportKey("jwk", keyPair.publicKey);
    const reimported = await crypto.subtle.importKey(
      "jwk", pubJwk, { name: "ECDSA", namedCurve: "P-256" }, true, ["verify"]
    );
    const valid = await crypto.subtle.verify(
      { name: "ECDSA", hash: "SHA-256" }, reimported, sig, data
    );
    return Response.json({ valid });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Valid bool `json:"valid"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatal(err)
	}
	if !data.Valid {
		t.Error("verify with reimported JWK ECDSA key should succeed")
	}
}

func TestCrypto_GenerateKeyHMAC_SHA384(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    const key = await crypto.subtle.generateKey(
      { name: "HMAC", hash: "SHA-384" }, true, ["sign", "verify"]
    );
    const exported = await crypto.subtle.exportKey("raw", key);
    return Response.json({ exportLen: exported.byteLength });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		ExportLen int `json:"exportLen"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatal(err)
	}
	if data.ExportLen != 48 {
		t.Errorf("SHA-384 HMAC key length = %d, want 48", data.ExportLen)
	}
}

func TestCrypto_GenerateKeyHMAC_SHA512(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    const key = await crypto.subtle.generateKey(
      { name: "HMAC", hash: "SHA-512" }, true, ["sign", "verify"]
    );
    const exported = await crypto.subtle.exportKey("raw", key);
    return Response.json({ exportLen: exported.byteLength });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		ExportLen int `json:"exportLen"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatal(err)
	}
	if data.ExportLen != 64 {
		t.Errorf("SHA-512 HMAC key length = %d, want 64", data.ExportLen)
	}
}

func TestCrypto_GenerateKeyHMAC_SHA1(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    const key = await crypto.subtle.generateKey(
      { name: "HMAC", hash: "SHA-1" }, true, ["sign", "verify"]
    );
    const exported = await crypto.subtle.exportKey("raw", key);
    return Response.json({ exportLen: exported.byteLength });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		ExportLen int `json:"exportLen"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatal(err)
	}
	if data.ExportLen != 20 {
		t.Errorf("SHA-1 HMAC key length = %d, want 20", data.ExportLen)
	}
}

func TestCrypto_ExportKeyJWK_HMAC_HS384(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    const key = await crypto.subtle.generateKey(
      { name: "HMAC", hash: "SHA-384" }, true, ["sign"]
    );
    const jwk = await crypto.subtle.exportKey("jwk", key);
    return Response.json({ alg: jwk.alg, kty: jwk.kty });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Alg string `json:"alg"`
		Kty string `json:"kty"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatal(err)
	}
	if data.Alg != "HS384" {
		t.Errorf("jwk.alg = %q, want HS384", data.Alg)
	}
	if data.Kty != "oct" {
		t.Errorf("jwk.kty = %q, want oct", data.Kty)
	}
}

func TestCrypto_ExportKeyJWK_HMAC_HS512(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    const key = await crypto.subtle.generateKey(
      { name: "HMAC", hash: "SHA-512" }, true, ["sign"]
    );
    const jwk = await crypto.subtle.exportKey("jwk", key);
    return Response.json({ alg: jwk.alg, kty: jwk.kty });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Alg string `json:"alg"`
		Kty string `json:"kty"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatal(err)
	}
	if data.Alg != "HS512" {
		t.Errorf("jwk.alg = %q, want HS512", data.Alg)
	}
}

func TestCrypto_WrapKey(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    // Generate an HMAC key to wrap
    const hmacKey = await crypto.subtle.generateKey(
      { name: "HMAC", hash: "SHA-256" }, true, ["sign", "verify"]
    );
    // Generate an AES-GCM wrapping key
    const wrappingKey = await crypto.subtle.generateKey(
      { name: "AES-GCM" }, true, ["encrypt", "decrypt", "wrapKey", "unwrapKey"]
    );
    const iv = crypto.getRandomValues(new Uint8Array(12));
    // Wrap the HMAC key
    const wrapped = await crypto.subtle.wrapKey(
      "raw", hmacKey, wrappingKey, { name: "AES-GCM", iv }
    );
    return Response.json({
      wrappedLength: new Uint8Array(wrapped).length,
      isArrayBuffer: wrapped instanceof ArrayBuffer,
    });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		WrappedLength int  `json:"wrappedLength"`
		IsArrayBuffer bool `json:"isArrayBuffer"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if data.WrappedLength == 0 {
		t.Error("wrapKey should return non-empty wrapped data")
	}
	if !data.IsArrayBuffer {
		t.Error("wrapKey should return an ArrayBuffer")
	}
}

func TestCrypto_UnwrapKey(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    // Generate an HMAC key to wrap
    const hmacKey = await crypto.subtle.generateKey(
      { name: "HMAC", hash: "SHA-256" }, true, ["sign", "verify"]
    );
    // Generate an AES-GCM wrapping key
    const wrappingKey = await crypto.subtle.generateKey(
      { name: "AES-GCM" }, true, ["encrypt", "decrypt", "wrapKey", "unwrapKey"]
    );
    const iv = crypto.getRandomValues(new Uint8Array(12));
    // Wrap the HMAC key
    const wrapped = await crypto.subtle.wrapKey(
      "raw", hmacKey, wrappingKey, { name: "AES-GCM", iv }
    );
    // Unwrap it back
    const unwrappedKey = await crypto.subtle.unwrapKey(
      "raw", wrapped, wrappingKey, { name: "AES-GCM", iv },
      { name: "HMAC", hash: "SHA-256" }, true, ["sign", "verify"]
    );
    // Use the unwrapped key to sign data
    const data = new TextEncoder().encode("test message");
    const sig = await crypto.subtle.sign("HMAC", unwrappedKey, data);
    // Verify using the original key
    const valid = await crypto.subtle.verify("HMAC", hmacKey, sig, data);
    return Response.json({
      valid,
      hasKey: unwrappedKey !== null && unwrappedKey !== undefined,
    });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Valid  bool `json:"valid"`
		HasKey bool `json:"hasKey"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !data.Valid {
		t.Error("signature from unwrapped key should verify with original key")
	}
	if !data.HasKey {
		t.Error("unwrapKey should return a valid CryptoKey")
	}
}

func TestCrypto_WrapUnwrapRoundtrip(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    // Generate an HMAC key
    const hmacKey = await crypto.subtle.generateKey(
      { name: "HMAC", hash: "SHA-256" }, true, ["sign", "verify"]
    );
    // Export the original key
    const originalExport = await crypto.subtle.exportKey("raw", hmacKey);
    const originalBytes = new Uint8Array(originalExport);

    // Generate an AES-GCM wrapping key
    const wrappingKey = await crypto.subtle.generateKey(
      { name: "AES-GCM" }, true, ["encrypt", "decrypt", "wrapKey", "unwrapKey"]
    );
    const iv = crypto.getRandomValues(new Uint8Array(12));

    // Wrap and unwrap
    const wrapped = await crypto.subtle.wrapKey(
      "raw", hmacKey, wrappingKey, { name: "AES-GCM", iv }
    );
    const unwrappedKey = await crypto.subtle.unwrapKey(
      "raw", wrapped, wrappingKey, { name: "AES-GCM", iv },
      { name: "HMAC", hash: "SHA-256" }, true, ["sign", "verify"]
    );

    // Export the unwrapped key
    const unwrappedExport = await crypto.subtle.exportKey("raw", unwrappedKey);
    const unwrappedBytes = new Uint8Array(unwrappedExport);

    // Compare bytes
    let match = originalBytes.length === unwrappedBytes.length;
    if (match) {
      for (let i = 0; i < originalBytes.length; i++) {
        if (originalBytes[i] !== unwrappedBytes[i]) {
          match = false;
          break;
        }
      }
    }
    return Response.json({
      match,
      originalLength: originalBytes.length,
      unwrappedLength: unwrappedBytes.length,
    });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Match           bool `json:"match"`
		OriginalLength  int  `json:"originalLength"`
		UnwrappedLength int  `json:"unwrappedLength"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !data.Match {
		t.Errorf("round-trip key bytes should match: original=%d, unwrapped=%d",
			data.OriginalLength, data.UnwrappedLength)
	}
}

func TestCrypto_WrapKeyNonExtractable(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    // Generate a NON-extractable HMAC key
    const hmacKey = await crypto.subtle.generateKey(
      { name: "HMAC", hash: "SHA-256" }, false, ["sign", "verify"]
    );
    // Generate an AES-GCM wrapping key
    const wrappingKey = await crypto.subtle.generateKey(
      { name: "AES-GCM" }, true, ["encrypt", "decrypt", "wrapKey", "unwrapKey"]
    );
    const iv = crypto.getRandomValues(new Uint8Array(12));
    try {
      await crypto.subtle.wrapKey(
        "raw", hmacKey, wrappingKey, { name: "AES-GCM", iv }
      );
      return Response.json({ threw: false });
    } catch (e) {
      return Response.json({ threw: true, message: e.message });
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
		t.Error("wrapKey should throw when key is not extractable")
	}
}

func TestCrypto_WrapUnwrapAESCBC(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    // Generate an HMAC key to wrap
    const hmacKey = await crypto.subtle.generateKey(
      { name: "HMAC", hash: "SHA-256" }, true, ["sign", "verify"]
    );
    // Export original for comparison
    const originalExport = new Uint8Array(await crypto.subtle.exportKey("raw", hmacKey));

    // Generate an AES-CBC wrapping key
    const wrappingKey = await crypto.subtle.generateKey(
      { name: "AES-CBC" }, true, ["encrypt", "decrypt", "wrapKey", "unwrapKey"]
    );
    const iv = crypto.getRandomValues(new Uint8Array(16));

    // Wrap with AES-CBC
    const wrapped = await crypto.subtle.wrapKey(
      "raw", hmacKey, wrappingKey, { name: "AES-CBC", iv }
    );
    // Unwrap with AES-CBC
    const unwrappedKey = await crypto.subtle.unwrapKey(
      "raw", wrapped, wrappingKey, { name: "AES-CBC", iv },
      { name: "HMAC", hash: "SHA-256" }, true, ["sign", "verify"]
    );
    const unwrappedExport = new Uint8Array(await crypto.subtle.exportKey("raw", unwrappedKey));

    // Compare bytes
    let match = originalExport.length === unwrappedExport.length;
    for (let i = 0; match && i < originalExport.length; i++) {
      if (originalExport[i] !== unwrappedExport[i]) match = false;
    }

    // Verify the unwrapped key actually works
    const data = new TextEncoder().encode("test");
    const sig = await crypto.subtle.sign("HMAC", unwrappedKey, data);
    const valid = await crypto.subtle.verify("HMAC", hmacKey, sig, data);

    return Response.json({ match, valid, wrappedLength: new Uint8Array(wrapped).length });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Match         bool `json:"match"`
		Valid         bool `json:"valid"`
		WrappedLength int  `json:"wrappedLength"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !data.Match {
		t.Error("AES-CBC wrap/unwrap roundtrip should preserve key bytes")
	}
	if !data.Valid {
		t.Error("unwrapped key should produce valid signatures")
	}
	if data.WrappedLength == 0 {
		t.Error("wrapped data should not be empty")
	}
}

func TestCrypto_UnwrapKeyWrongKey(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    // Generate an HMAC key to wrap
    const hmacKey = await crypto.subtle.generateKey(
      { name: "HMAC", hash: "SHA-256" }, true, ["sign", "verify"]
    );
    // Generate two different AES-GCM keys
    const wrappingKey = await crypto.subtle.generateKey(
      { name: "AES-GCM" }, true, ["encrypt", "decrypt", "wrapKey", "unwrapKey"]
    );
    const wrongKey = await crypto.subtle.generateKey(
      { name: "AES-GCM" }, true, ["encrypt", "decrypt", "wrapKey", "unwrapKey"]
    );
    const iv = crypto.getRandomValues(new Uint8Array(12));

    // Wrap with the correct key
    const wrapped = await crypto.subtle.wrapKey(
      "raw", hmacKey, wrappingKey, { name: "AES-GCM", iv }
    );

    // Try to unwrap with the wrong key
    let caught = false;
    try {
      await crypto.subtle.unwrapKey(
        "raw", wrapped, wrongKey, { name: "AES-GCM", iv },
        { name: "HMAC", hash: "SHA-256" }, true, ["sign", "verify"]
      );
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
		t.Fatalf("unmarshal: %v", err)
	}
	if !data.Caught {
		t.Error("unwrapKey with wrong key should throw")
	}
}
