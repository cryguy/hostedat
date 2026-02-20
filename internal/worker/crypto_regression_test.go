package worker

import (
	"encoding/json"
	"testing"
)

// These tests verify that adding new crypto algorithms (RSA, Ed25519, HKDF, PBKDF2)
// didn't break existing ECDSA, HMAC, and AES functionality via the chain-of-responsibility
// pattern in the JS overrides.

func TestRegression_ECDSAStillWorks(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    const keyPair = await crypto.subtle.generateKey(
      { name: "ECDSA", namedCurve: "P-256" }, true, ["sign", "verify"]
    );
    const data = new TextEncoder().encode("ECDSA regression test");
    const sig = await crypto.subtle.sign(
      { name: "ECDSA", hash: "SHA-256" }, keyPair.privateKey, data
    );
    const valid = await crypto.subtle.verify(
      { name: "ECDSA", hash: "SHA-256" }, keyPair.publicKey, sig, data
    );
    const tampered = new TextEncoder().encode("tampered");
    const invalid = await crypto.subtle.verify(
      { name: "ECDSA", hash: "SHA-256" }, keyPair.publicKey, sig, tampered
    );
    return Response.json({ valid, invalid });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Valid   bool `json:"valid"`
		Invalid bool `json:"invalid"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !data.Valid {
		t.Error("ECDSA verify should still work after adding new algorithms")
	}
	if data.Invalid {
		t.Error("ECDSA verify should still reject tampered data")
	}
}

func TestRegression_HMACStillWorks(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    const key = await crypto.subtle.generateKey(
      { name: "HMAC", hash: "SHA-256" }, true, ["sign", "verify"]
    );
    const data = new TextEncoder().encode("HMAC regression test");
    const sig = await crypto.subtle.sign("HMAC", key, data);
    const valid = await crypto.subtle.verify("HMAC", key, sig, data);
    const tampered = new TextEncoder().encode("tampered");
    const invalid = await crypto.subtle.verify("HMAC", key, sig, tampered);
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
		t.Error("HMAC verify should still work after adding new algorithms")
	}
	if data.Invalid {
		t.Error("HMAC verify should still reject tampered data")
	}
	if data.SigLen != 32 {
		t.Errorf("HMAC-SHA256 sig length = %d, want 32", data.SigLen)
	}
}

func TestRegression_AESGCMStillWorks(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    const key = await crypto.subtle.generateKey(
      { name: "AES-GCM", length: 256 }, true, ["encrypt", "decrypt"]
    );
    const iv = new Uint8Array(12);
    crypto.getRandomValues(iv);
    const plaintext = new TextEncoder().encode("AES-GCM regression test");
    const ct = await crypto.subtle.encrypt({ name: "AES-GCM", iv }, key, plaintext);
    const pt = await crypto.subtle.decrypt({ name: "AES-GCM", iv }, key, ct);
    const result = new TextDecoder().decode(pt);
    return Response.json({ match: result === "AES-GCM regression test" });
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
		t.Error("AES-GCM should still work after adding new algorithms")
	}
}

func TestRegression_AESCBCStillWorks(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    const key = await crypto.subtle.generateKey(
      { name: "AES-CBC", length: 256 }, true, ["encrypt", "decrypt"]
    );
    const iv = new Uint8Array(16);
    crypto.getRandomValues(iv);
    const plaintext = new TextEncoder().encode("AES-CBC regression test!");
    const ct = await crypto.subtle.encrypt({ name: "AES-CBC", iv }, key, plaintext);
    const pt = await crypto.subtle.decrypt({ name: "AES-CBC", iv }, key, ct);
    const result = new TextDecoder().decode(pt);
    return Response.json({ match: result === "AES-CBC regression test!" });
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
		t.Error("AES-CBC should still work after adding new algorithms")
	}
}

func TestRegression_ECDSAImportExportJWK(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    const keyPair = await crypto.subtle.generateKey(
      { name: "ECDSA", namedCurve: "P-256" }, true, ["sign", "verify"]
    );
    const pubJWK = await crypto.subtle.exportKey("jwk", keyPair.publicKey);
    const imported = await crypto.subtle.importKey(
      "jwk", pubJWK, { name: "ECDSA", namedCurve: "P-256" }, true, ["verify"]
    );
    const msg = new TextEncoder().encode("ECDSA JWK regression");
    const sig = await crypto.subtle.sign({ name: "ECDSA", hash: "SHA-256" }, keyPair.privateKey, msg);
    const valid = await crypto.subtle.verify({ name: "ECDSA", hash: "SHA-256" }, imported, sig, msg);
    return Response.json({ valid, kty: pubJWK.kty, crv: pubJWK.crv });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Valid bool   `json:"valid"`
		Kty   string `json:"kty"`
		Crv   string `json:"crv"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !data.Valid {
		t.Error("ECDSA JWK import/export should still work")
	}
	if data.Kty != "EC" {
		t.Errorf("ECDSA JWK kty = %q, want 'EC'", data.Kty)
	}
	if data.Crv != "P-256" {
		t.Errorf("ECDSA JWK crv = %q, want 'P-256'", data.Crv)
	}
}

func TestRegression_MultiAlgoInSingleRequest(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	// Use ECDSA, RSA, Ed25519, HMAC, and AES all in the same request
	source := `export default {
  async fetch(request, env) {
    // HMAC
    const hmacKey = await crypto.subtle.generateKey(
      { name: "HMAC", hash: "SHA-256" }, false, ["sign", "verify"]
    );
    const hmacSig = await crypto.subtle.sign("HMAC", hmacKey, new TextEncoder().encode("hmac"));
    const hmacOK = await crypto.subtle.verify("HMAC", hmacKey, hmacSig, new TextEncoder().encode("hmac"));

    // AES-GCM
    const aesKey = await crypto.subtle.generateKey(
      { name: "AES-GCM", length: 128 }, false, ["encrypt", "decrypt"]
    );
    const iv = new Uint8Array(12);
    const aesCT = await crypto.subtle.encrypt({ name: "AES-GCM", iv }, aesKey, new TextEncoder().encode("aes"));
    const aesPT = await crypto.subtle.decrypt({ name: "AES-GCM", iv }, aesKey, aesCT);
    const aesOK = new TextDecoder().decode(aesPT) === "aes";

    // ECDSA
    const ecKey = await crypto.subtle.generateKey(
      { name: "ECDSA", namedCurve: "P-256" }, false, ["sign", "verify"]
    );
    const ecSig = await crypto.subtle.sign(
      { name: "ECDSA", hash: "SHA-256" }, ecKey.privateKey, new TextEncoder().encode("ec")
    );
    const ecOK = await crypto.subtle.verify(
      { name: "ECDSA", hash: "SHA-256" }, ecKey.publicKey, ecSig, new TextEncoder().encode("ec")
    );

    // Ed25519
    const edKey = await crypto.subtle.generateKey(
      { name: "Ed25519" }, false, ["sign", "verify"]
    );
    const edSig = await crypto.subtle.sign("Ed25519", edKey.privateKey, new TextEncoder().encode("ed"));
    const edOK = await crypto.subtle.verify("Ed25519", edKey.publicKey, edSig, new TextEncoder().encode("ed"));

    // RSA
    const rsaKey = await crypto.subtle.generateKey(
      { name: "RSASSA-PKCS1-v1_5", modulusLength: 2048, publicExponent: new Uint8Array([1, 0, 1]), hash: "SHA-256" },
      false, ["sign", "verify"]
    );
    const rsaSig = await crypto.subtle.sign("RSASSA-PKCS1-v1_5", rsaKey.privateKey, new TextEncoder().encode("rsa"));
    const rsaOK = await crypto.subtle.verify("RSASSA-PKCS1-v1_5", rsaKey.publicKey, rsaSig, new TextEncoder().encode("rsa"));

    return Response.json({ hmacOK, aesOK, ecOK, edOK, rsaOK });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		HmacOK bool `json:"hmacOK"`
		AesOK  bool `json:"aesOK"`
		EcOK   bool `json:"ecOK"`
		EdOK   bool `json:"edOK"`
		RsaOK  bool `json:"rsaOK"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !data.HmacOK {
		t.Error("HMAC failed in multi-algo request")
	}
	if !data.AesOK {
		t.Error("AES-GCM failed in multi-algo request")
	}
	if !data.EcOK {
		t.Error("ECDSA failed in multi-algo request")
	}
	if !data.EdOK {
		t.Error("Ed25519 failed in multi-algo request")
	}
	if !data.RsaOK {
		t.Error("RSA failed in multi-algo request")
	}
}
