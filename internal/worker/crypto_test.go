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
	json.Unmarshal(r.Response.Body, &data)

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
	json.Unmarshal(r.Response.Body, &data)

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
	json.Unmarshal(r.Response.Body, &data)

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
	json.Unmarshal(r.Response.Body, &data)

	// SHA-1 of "hello" = aaf4c61ddcc5e8a2dabede0f3b482cd9aea9434d
	expected := "aaf4c61ddcc5e8a2dabede0f3b482cd9aea9434d"
	if data.Hex != expected {
		t.Errorf("SHA-1 hex = %q, want %q", data.Hex, expected)
	}
	if data.Length != 20 {
		t.Errorf("hash length = %d, want 20", data.Length)
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
	json.Unmarshal(r.Response.Body, &data)

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
	json.Unmarshal(r.Response.Body, &data)

	if !data.Match {
		t.Error("AES-GCM decrypt should return original plaintext")
	}
	// AES-GCM adds a 16-byte auth tag. Input is 16 bytes, output should be 32.
	if data.CtLength != 32 {
		t.Errorf("ciphertext length = %d, want 32 (16 data + 16 tag)", data.CtLength)
	}
}
