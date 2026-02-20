package worker

import (
	"encoding/json"
	"testing"
)

// ---------------------------------------------------------------------------
// JWK import/export (Priority 1)
// ---------------------------------------------------------------------------

func TestCryptoExt_JWK_HMAC_ImportExport(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    const jwk = {
      kty: "oct",
      k: "bXktc2VjcmV0LWtleQ", // base64url of "my-secret-key"
      alg: "HS256",
    };
    const key = await crypto.subtle.importKey(
      "jwk", jwk, { name: "HMAC", hash: "SHA-256" }, true, ["sign", "verify"]
    );
    const sig = await crypto.subtle.sign("HMAC", key, new TextEncoder().encode("test"));
    const valid = await crypto.subtle.verify("HMAC", key, sig, new TextEncoder().encode("test"));

    const exported = await crypto.subtle.exportKey("jwk", key);

    return Response.json({
      sigLen: new Uint8Array(sig).length,
      valid,
      kty: exported.kty,
      alg: exported.alg,
    });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		SigLen int    `json:"sigLen"`
		Valid  bool   `json:"valid"`
		Kty    string `json:"kty"`
		Alg    string `json:"alg"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if data.SigLen != 32 {
		t.Errorf("sig length = %d, want 32", data.SigLen)
	}
	if !data.Valid {
		t.Error("JWK HMAC sign/verify should succeed")
	}
	if data.Kty != "oct" {
		t.Errorf("exported kty = %q, want oct", data.Kty)
	}
	if data.Alg != "HS256" {
		t.Errorf("exported alg = %q, want HS256", data.Alg)
	}
}

// ---------------------------------------------------------------------------
// Security Fixes - H7, M6, M11
// ---------------------------------------------------------------------------

func TestAESCBC_PaddingValidation(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    // Generate a key and encrypt some data
    const key = await crypto.subtle.generateKey(
      { name: "AES-CBC", length: 256 }, true, ["encrypt", "decrypt"]
    );
    const iv = new Uint8Array(16);
    crypto.getRandomValues(iv);
    const pt = new TextEncoder().encode("test padding validation!");
    const ct = await crypto.subtle.encrypt({ name: "AES-CBC", iv }, key, pt);

    // Normal decrypt should work
    const decrypted = await crypto.subtle.decrypt({ name: "AES-CBC", iv }, key, ct);
    const result = new TextDecoder().decode(decrypted);

    // Tamper with ciphertext (corrupt last block = padding)
    const tampered = new Uint8Array(ct);
    tampered[tampered.length - 1] ^= 0xFF;
    var tamperedBlocked = false;
    try {
      await crypto.subtle.decrypt({ name: "AES-CBC", iv }, key, tampered.buffer);
    } catch(e) {
      tamperedBlocked = true;
    }

    return Response.json({
      match: result === "test padding validation!",
      tamperedBlocked: tamperedBlocked,
    });
  },
};`
	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)
	var data struct {
		Match           bool `json:"match"`
		TamperedBlocked bool `json:"tamperedBlocked"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !data.Match {
		t.Error("AES-CBC decrypt should produce original plaintext")
	}
	if !data.TamperedBlocked {
		t.Error("AES-CBC decrypt with tampered ciphertext should fail")
	}
}

func TestAESGCM_WithAAD(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)
	source := `export default {
  async fetch(request, env) {
    const key = await crypto.subtle.generateKey(
      { name: "AES-GCM", length: 256 }, true, ["encrypt", "decrypt"]
    );
    const iv = new Uint8Array(12);
    crypto.getRandomValues(iv);
    const aad = new TextEncoder().encode("associated data");
    const pt = new TextEncoder().encode("secret message");

    // Encrypt with AAD
    const ct = await crypto.subtle.encrypt({ name: "AES-GCM", iv, additionalData: aad }, key, pt);

    // Decrypt with same AAD should work
    const decrypted = await crypto.subtle.decrypt({ name: "AES-GCM", iv, additionalData: aad }, key, ct);
    const result = new TextDecoder().decode(decrypted);

    // Decrypt with wrong AAD should fail
    var wrongAADFailed = false;
    try {
      const wrongAAD = new TextEncoder().encode("wrong data");
      await crypto.subtle.decrypt({ name: "AES-GCM", iv, additionalData: wrongAAD }, key, ct);
    } catch(e) {
      wrongAADFailed = true;
    }

    // Decrypt with no AAD should fail
    var noAADFailed = false;
    try {
      await crypto.subtle.decrypt({ name: "AES-GCM", iv }, key, ct);
    } catch(e) {
      noAADFailed = true;
    }

    return Response.json({ match: result === "secret message", wrongAADFailed, noAADFailed });
  },
};`
	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)
	var data struct {
		Match          bool `json:"match"`
		WrongAADFailed bool `json:"wrongAADFailed"`
		NoAADFailed    bool `json:"noAADFailed"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !data.Match {
		t.Error("decrypt with correct AAD should work")
	}
	if !data.WrongAADFailed {
		t.Error("decrypt with wrong AAD should fail")
	}
	if !data.NoAADFailed {
		t.Error("decrypt with no AAD (when encrypted with AAD) should fail")
	}
}

func TestAES_GenerateKey_128bit(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)
	source := `export default {
  async fetch(request, env) {
    const key128 = await crypto.subtle.generateKey(
      { name: "AES-GCM", length: 128 }, true, ["encrypt", "decrypt"]
    );
    const raw128 = await crypto.subtle.exportKey("raw", key128);

    const key256 = await crypto.subtle.generateKey(
      { name: "AES-GCM", length: 256 }, true, ["encrypt", "decrypt"]
    );
    const raw256 = await crypto.subtle.exportKey("raw", key256);

    return Response.json({
      len128: raw128.byteLength,
      len256: raw256.byteLength,
    });
  },
};`
	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)
	var data struct {
		Len128 int `json:"len128"`
		Len256 int `json:"len256"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if data.Len128 != 16 {
		t.Errorf("128-bit key raw length = %d, want 16", data.Len128)
	}
	if data.Len256 != 32 {
		t.Errorf("256-bit key raw length = %d, want 32", data.Len256)
	}
}
