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
