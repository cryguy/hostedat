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
// ECDSA (Priority 2)
// ---------------------------------------------------------------------------

func TestCryptoExt_ECDSA_GenerateSignVerify(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    const keyPair = await crypto.subtle.generateKey(
      { name: "ECDSA", namedCurve: "P-256" },
      true,
      ["sign", "verify"]
    );

    const data = new TextEncoder().encode("hello");
    const signature = await crypto.subtle.sign(
      { name: "ECDSA", hash: "SHA-256" },
      keyPair.privateKey,
      data
    );
    const valid = await crypto.subtle.verify(
      { name: "ECDSA", hash: "SHA-256" },
      keyPair.publicKey,
      signature,
      data
    );

    const invalid = await crypto.subtle.verify(
      { name: "ECDSA", hash: "SHA-256" },
      keyPair.publicKey,
      signature,
      new TextEncoder().encode("tampered")
    );

    return Response.json({
      valid,
      invalid,
      sigLen: new Uint8Array(signature).length,
      privType: keyPair.privateKey.type,
      pubType: keyPair.publicKey.type,
    });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Valid    bool   `json:"valid"`
		Invalid  bool   `json:"invalid"`
		SigLen   int    `json:"sigLen"`
		PrivType string `json:"privType"`
		PubType  string `json:"pubType"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !data.Valid {
		t.Error("ECDSA verify should return true for correct data")
	}
	if data.Invalid {
		t.Error("ECDSA verify should return false for tampered data")
	}
	// P-256 signature is 64 bytes (32 bytes for r + 32 bytes for s)
	if data.SigLen != 64 {
		t.Errorf("signature length = %d, want 64", data.SigLen)
	}
	if data.PrivType != "private" {
		t.Errorf("private key type = %q, want private", data.PrivType)
	}
	if data.PubType != "public" {
		t.Errorf("public key type = %q, want public", data.PubType)
	}
}

func TestCryptoExt_ECDSA_P384(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    const keyPair = await crypto.subtle.generateKey(
      { name: "ECDSA", namedCurve: "P-384" },
      true,
      ["sign", "verify"]
    );

    const data = new TextEncoder().encode("P-384 test");
    const signature = await crypto.subtle.sign(
      { name: "ECDSA", hash: "SHA-384" },
      keyPair.privateKey,
      data
    );
    const valid = await crypto.subtle.verify(
      { name: "ECDSA", hash: "SHA-384" },
      keyPair.publicKey,
      signature,
      data
    );

    return Response.json({
      valid,
      sigLen: new Uint8Array(signature).length,
    });
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
		t.Error("ECDSA P-384 verify should return true")
	}
	// P-384 signature is 96 bytes (48 bytes for r + 48 bytes for s)
	if data.SigLen != 96 {
		t.Errorf("P-384 signature length = %d, want 96", data.SigLen)
	}
}

func TestCryptoExt_ECDSA_JWK_RoundTrip(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    const keyPair = await crypto.subtle.generateKey(
      { name: "ECDSA", namedCurve: "P-256" },
      true,
      ["sign", "verify"]
    );

    // Export both keys as JWK
    const privJWK = await crypto.subtle.exportKey("jwk", keyPair.privateKey);
    const pubJWK = await crypto.subtle.exportKey("jwk", keyPair.publicKey);

    // Re-import from JWK
    const reimportedPriv = await crypto.subtle.importKey(
      "jwk", privJWK, { name: "ECDSA", namedCurve: "P-256" }, true, ["sign"]
    );
    const reimportedPub = await crypto.subtle.importKey(
      "jwk", pubJWK, { name: "ECDSA", namedCurve: "P-256" }, true, ["verify"]
    );

    // Sign with re-imported private key, verify with re-imported public key
    const data = new TextEncoder().encode("JWK round-trip");
    const sig = await crypto.subtle.sign(
      { name: "ECDSA", hash: "SHA-256" }, reimportedPriv, data
    );
    const valid = await crypto.subtle.verify(
      { name: "ECDSA", hash: "SHA-256" }, reimportedPub, sig, data
    );

    return Response.json({
      valid,
      privKty: privJWK.kty,
      privCrv: privJWK.crv,
      privHasD: typeof privJWK.d === 'string' && privJWK.d.length > 0,
      pubKty: pubJWK.kty,
      pubCrv: pubJWK.crv,
      pubHasD: pubJWK.d !== undefined,
    });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Valid   bool   `json:"valid"`
		PrivKty string `json:"privKty"`
		PrivCrv string `json:"privCrv"`
		PrivHasD bool  `json:"privHasD"`
		PubKty  string `json:"pubKty"`
		PubCrv  string `json:"pubCrv"`
		PubHasD bool   `json:"pubHasD"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !data.Valid {
		t.Error("ECDSA JWK round-trip: sign/verify should succeed")
	}
	if data.PrivKty != "EC" {
		t.Errorf("private JWK kty = %q, want EC", data.PrivKty)
	}
	if data.PrivCrv != "P-256" {
		t.Errorf("private JWK crv = %q, want P-256", data.PrivCrv)
	}
	if !data.PrivHasD {
		t.Error("private JWK should have 'd' field")
	}
	if data.PubKty != "EC" {
		t.Errorf("public JWK kty = %q, want EC", data.PubKty)
	}
	if data.PubHasD {
		t.Error("public JWK should not have 'd' field")
	}
}

// ---------------------------------------------------------------------------
// AES-CBC (Priority 4)
// ---------------------------------------------------------------------------

func TestCryptoExt_AESCBC_EncryptDecrypt(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    const keyData = new Uint8Array(16);
    crypto.getRandomValues(keyData);
    const key = await crypto.subtle.importKey(
      "raw", keyData, { name: "AES-CBC" }, false, ["encrypt", "decrypt"]
    );
    const iv = new Uint8Array(16);
    crypto.getRandomValues(iv);

    const plaintext = new TextEncoder().encode("AES-CBC test data!!");
    const ciphertext = await crypto.subtle.encrypt(
      { name: "AES-CBC", iv }, key, plaintext
    );
    const decrypted = await crypto.subtle.decrypt(
      { name: "AES-CBC", iv }, key, ciphertext
    );
    const result = new TextDecoder().decode(decrypted);

    return Response.json({
      match: result === "AES-CBC test data!!",
      ctLen: new Uint8Array(ciphertext).length,
    });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Match bool `json:"match"`
		CtLen int  `json:"ctLen"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !data.Match {
		t.Error("AES-CBC decrypt should return original plaintext")
	}
	// 19 bytes plaintext -> padded to 32 bytes (next multiple of 16)
	if data.CtLen != 32 {
		t.Errorf("ciphertext length = %d, want 32", data.CtLen)
	}
}

func TestCryptoExt_AESCBC_BlockAligned(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    const keyData = new Uint8Array(32);
    crypto.getRandomValues(keyData);
    const key = await crypto.subtle.importKey(
      "raw", keyData, { name: "AES-CBC" }, false, ["encrypt", "decrypt"]
    );
    const iv = new Uint8Array(16);
    crypto.getRandomValues(iv);

    // Exactly 16 bytes (block-aligned)
    const plaintext = new TextEncoder().encode("exactly16bytes!!");
    const ciphertext = await crypto.subtle.encrypt(
      { name: "AES-CBC", iv }, key, plaintext
    );
    const decrypted = await crypto.subtle.decrypt(
      { name: "AES-CBC", iv }, key, ciphertext
    );
    const result = new TextDecoder().decode(decrypted);

    return Response.json({
      match: result === "exactly16bytes!!",
      ctLen: new Uint8Array(ciphertext).length,
    });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Match bool `json:"match"`
		CtLen int  `json:"ctLen"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !data.Match {
		t.Error("AES-CBC block-aligned decrypt should return original plaintext")
	}
	// 16 bytes plaintext -> PKCS7 adds full block of padding -> 32 bytes
	if data.CtLen != 32 {
		t.Errorf("ciphertext length = %d, want 32", data.CtLen)
	}
}

func TestCryptoExt_AESCBC_RejectsInvalidIV(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    const keyData = new Uint8Array(16);
    crypto.getRandomValues(keyData);
    const key = await crypto.subtle.importKey(
      "raw", keyData, { name: "AES-CBC" }, false, ["encrypt", "decrypt"]
    );
    // Wrong IV length: 12 bytes instead of 16
    const badIV = new Uint8Array(12);
    try {
      await crypto.subtle.encrypt({ name: "AES-CBC", iv: badIV }, key, new Uint8Array([1,2,3]));
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
		t.Error("AES-CBC encrypt should reject non-16-byte IV")
	}
}

// ---------------------------------------------------------------------------
// generateKey
// ---------------------------------------------------------------------------

func TestCryptoExt_GenerateKey_HMAC(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    const key = await crypto.subtle.generateKey(
      { name: "HMAC", hash: "SHA-256" },
      true,
      ["sign", "verify"]
    );

    const data = new TextEncoder().encode("test generate");
    const sig = await crypto.subtle.sign("HMAC", key, data);
    const valid = await crypto.subtle.verify("HMAC", key, sig, data);

    return Response.json({
      valid,
      type: key.type,
      sigLen: new Uint8Array(sig).length,
    });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Valid  bool   `json:"valid"`
		Type   string `json:"type"`
		SigLen int    `json:"sigLen"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !data.Valid {
		t.Error("generated HMAC key sign/verify should succeed")
	}
	if data.Type != "secret" {
		t.Errorf("key type = %q, want secret", data.Type)
	}
	if data.SigLen != 32 {
		t.Errorf("signature length = %d, want 32", data.SigLen)
	}
}

func TestCryptoExt_GenerateKey_AESGCM(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    const key = await crypto.subtle.generateKey(
      { name: "AES-GCM", length: 256 },
      false,
      ["encrypt", "decrypt"]
    );

    const iv = new Uint8Array(12);
    crypto.getRandomValues(iv);
    const plaintext = new TextEncoder().encode("generated key test");
    const ct = await crypto.subtle.encrypt({ name: "AES-GCM", iv }, key, plaintext);
    const pt = await crypto.subtle.decrypt({ name: "AES-GCM", iv }, key, ct);
    const result = new TextDecoder().decode(pt);

    return Response.json({ match: result === "generated key test", type: key.type });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Match bool   `json:"match"`
		Type  string `json:"type"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !data.Match {
		t.Error("generated AES-GCM key encrypt/decrypt should round-trip")
	}
	if data.Type != "secret" {
		t.Errorf("key type = %q, want secret", data.Type)
	}
}
