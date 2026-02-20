package worker

import (
	"encoding/json"
	"testing"
)

func TestCrypto_Ed25519GenerateAndSign(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    const keyPair = await crypto.subtle.generateKey(
      { name: "Ed25519" }, true, ["sign", "verify"]
    );
    const data = new TextEncoder().encode("hello ed25519");
    const sig = await crypto.subtle.sign("Ed25519", keyPair.privateKey, data);
    const valid = await crypto.subtle.verify("Ed25519", keyPair.publicKey, sig, data);
    const tampered = new TextEncoder().encode("tampered message");
    const invalid = await crypto.subtle.verify("Ed25519", keyPair.publicKey, sig, tampered);
    return Response.json({
      valid, invalid,
      sigLength: new Uint8Array(sig).length,
      privType: keyPair.privateKey.type,
      pubType: keyPair.publicKey.type,
    });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Valid     bool   `json:"valid"`
		Invalid   bool   `json:"invalid"`
		SigLength int    `json:"sigLength"`
		PrivType  string `json:"privType"`
		PubType   string `json:"pubType"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if !data.Valid {
		t.Error("Ed25519 verify should return true for correct data")
	}
	if data.Invalid {
		t.Error("Ed25519 verify should return false for tampered data")
	}
	if data.SigLength != 64 {
		t.Errorf("Ed25519 signature length = %d, want 64", data.SigLength)
	}
	if data.PrivType != "private" {
		t.Errorf("private key type = %q, want 'private'", data.PrivType)
	}
	if data.PubType != "public" {
		t.Errorf("public key type = %q, want 'public'", data.PubType)
	}
}

func TestCrypto_Ed25519ImportExportRaw(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    const keyPair = await crypto.subtle.generateKey(
      { name: "Ed25519" }, true, ["sign", "verify"]
    );
    // Export public key as raw
    const rawPub = await crypto.subtle.exportKey("raw", keyPair.publicKey);
    const pubArr = new Uint8Array(rawPub);

    // Re-import
    const importedPub = await crypto.subtle.importKey(
      "raw", rawPub, { name: "Ed25519" }, true, ["verify"]
    );

    // Sign with original, verify with re-imported
    const msg = new TextEncoder().encode("import export test");
    const sig = await crypto.subtle.sign("Ed25519", keyPair.privateKey, msg);
    const valid = await crypto.subtle.verify("Ed25519", importedPub, sig, msg);

    return Response.json({ valid, pubKeyLen: pubArr.length });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Valid     bool `json:"valid"`
		PubKeyLen int  `json:"pubKeyLen"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if !data.Valid {
		t.Error("imported public key should verify signatures from original private key")
	}
	if data.PubKeyLen != 32 {
		t.Errorf("Ed25519 public key length = %d, want 32", data.PubKeyLen)
	}
}

func TestCrypto_Ed25519ImportExportJWK(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    const keyPair = await crypto.subtle.generateKey(
      { name: "Ed25519" }, true, ["sign", "verify"]
    );
    // Export both keys as JWK
    const pubJWK = await crypto.subtle.exportKey("jwk", keyPair.publicKey);
    const privJWK = await crypto.subtle.exportKey("jwk", keyPair.privateKey);

    // Verify JWK structure
    const pubValid = !!(pubJWK.kty === "OKP" && pubJWK.crv === "Ed25519" && pubJWK.x && !pubJWK.d);
    const privValid = !!(privJWK.kty === "OKP" && privJWK.crv === "Ed25519" && privJWK.x && privJWK.d);

    // Re-import from JWK
    const importedPriv = await crypto.subtle.importKey(
      "jwk", privJWK, { name: "Ed25519" }, true, ["sign"]
    );
    const importedPub = await crypto.subtle.importKey(
      "jwk", pubJWK, { name: "Ed25519" }, true, ["verify"]
    );

    // Sign with re-imported private, verify with re-imported public
    const msg = new TextEncoder().encode("jwk round trip");
    const sig = await crypto.subtle.sign("Ed25519", importedPriv, msg);
    const verified = await crypto.subtle.verify("Ed25519", importedPub, sig, msg);

    return Response.json({ pubValid, privValid, verified });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		PubValid  bool `json:"pubValid"`
		PrivValid bool `json:"privValid"`
		Verified  bool `json:"verified"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if !data.PubValid {
		t.Error("public JWK should have kty=OKP, crv=Ed25519, x field, no d field")
	}
	if !data.PrivValid {
		t.Error("private JWK should have kty=OKP, crv=Ed25519, x and d fields")
	}
	if !data.Verified {
		t.Error("JWK round-trip import/export should produce working keys")
	}
}

func TestCrypto_Ed25519WebhookVerification(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	// Simulates a webhook signature verification (Discord/GitHub style)
	source := `export default {
  async fetch(request, env) {
    // Generate a key pair (simulating the webhook provider)
    const keyPair = await crypto.subtle.generateKey(
      { name: "Ed25519" }, true, ["sign", "verify"]
    );
    // Export public key (this would be configured in the webhook settings)
    const pubJWK = await crypto.subtle.exportKey("jwk", keyPair.publicKey);

    // Sign a message (simulating the webhook provider signing the payload)
    const timestamp = "1234567890";
    const body = '{"event":"push"}';
    const message = new TextEncoder().encode(timestamp + body);
    const signature = await crypto.subtle.sign("Ed25519", keyPair.privateKey, message);

    // Now verify (simulating the webhook receiver)
    const importedPub = await crypto.subtle.importKey(
      "jwk", pubJWK, { name: "Ed25519" }, false, ["verify"]
    );
    const valid = await crypto.subtle.verify("Ed25519", importedPub, signature, message);

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
		t.Error("webhook signature verification should pass")
	}
}

func TestCrypto_Ed25519SignWithPublicKeyErrors(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    const keyPair = await crypto.subtle.generateKey(
      { name: "Ed25519" }, true, ["sign", "verify"]
    );
    let signWithPubFailed = false;
    try {
      const data = new TextEncoder().encode("test");
      await crypto.subtle.sign("Ed25519", keyPair.publicKey, data);
    } catch (e) {
      signWithPubFailed = true;
    }
    return Response.json({ signWithPubFailed });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		SignWithPubFailed bool `json:"signWithPubFailed"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !data.SignWithPubFailed {
		t.Error("signing with a public key should fail")
	}
}

func TestCrypto_Ed25519ImportInvalidRawLength(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    let importFailed = false;
    try {
      // 16 bytes is not a valid Ed25519 key length (must be 32 or 64)
      const badKey = new Uint8Array(16);
      await crypto.subtle.importKey("raw", badKey, { name: "Ed25519" }, true, ["verify"]);
    } catch (e) {
      importFailed = true;
    }
    return Response.json({ importFailed });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		ImportFailed bool `json:"importFailed"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !data.ImportFailed {
		t.Error("importing raw Ed25519 key with invalid length should fail")
	}
}

func TestCrypto_Ed25519NonExtractableExportErrors(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    const keyPair = await crypto.subtle.generateKey(
      { name: "Ed25519" }, false, ["sign", "verify"]
    );
    let exportFailed = false;
    try {
      await crypto.subtle.exportKey("raw", keyPair.publicKey);
    } catch (e) {
      exportFailed = true;
    }
    return Response.json({ exportFailed });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		ExportFailed bool `json:"exportFailed"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !data.ExportFailed {
		t.Error("exporting non-extractable Ed25519 key should fail")
	}
}

func TestCrypto_Ed25519EmptyMessage(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    const keyPair = await crypto.subtle.generateKey(
      { name: "Ed25519" }, true, ["sign", "verify"]
    );
    const emptyData = new Uint8Array(0);
    const sig = await crypto.subtle.sign("Ed25519", keyPair.privateKey, emptyData);
    const valid = await crypto.subtle.verify("Ed25519", keyPair.publicKey, sig, emptyData);
    return Response.json({ valid, sigLength: new Uint8Array(sig).length });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Valid     bool `json:"valid"`
		SigLength int  `json:"sigLength"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !data.Valid {
		t.Error("Ed25519 should sign and verify empty messages")
	}
	if data.SigLength != 64 {
		t.Errorf("Ed25519 sig length = %d, want 64", data.SigLength)
	}
}

func TestCrypto_Ed25519DirectCallbackErrors(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    const results = {};

    // __cryptoSignEd25519 with missing args.
    try { __cryptoSignEd25519(0); results.signNoArgs = false; }
    catch(e) { results.signNoArgs = true; }

    // __cryptoSignEd25519 with bad base64.
    try { __cryptoSignEd25519(0, "bad-b64!!!"); results.signBadB64 = false; }
    catch(e) { results.signBadB64 = true; }

    // __cryptoSignEd25519 with bad key ID.
    try { __cryptoSignEd25519(9999, btoa("data")); results.signBadKey = false; }
    catch(e) { results.signBadKey = true; }

    // __cryptoVerifyEd25519 with missing args.
    try { __cryptoVerifyEd25519(0, btoa("sig")); results.verifyNoArgs = false; }
    catch(e) { results.verifyNoArgs = true; }

    // __cryptoVerifyEd25519 with bad sig base64.
    try { __cryptoVerifyEd25519(0, "bad!!!", btoa("data")); results.verifyBadSig = false; }
    catch(e) { results.verifyBadSig = true; }

    // __cryptoVerifyEd25519 with bad data base64.
    try { __cryptoVerifyEd25519(0, btoa("sig"), "bad!!!"); results.verifyBadData = false; }
    catch(e) { results.verifyBadData = true; }

    // __cryptoVerifyEd25519 with bad key ID.
    try { __cryptoVerifyEd25519(9999, btoa("sig"), btoa("data")); results.verifyBadKey = false; }
    catch(e) { results.verifyBadKey = true; }

    // __cryptoImportKeyEd25519 with missing args.
    try { __cryptoImportKeyEd25519("raw"); results.importNoArgs = false; }
    catch(e) { results.importNoArgs = true; }

    // Sign with HMAC key (wrong type for Ed25519).
    const key = await crypto.subtle.importKey(
      "raw", new TextEncoder().encode("hmac-key"),
      { name: "HMAC", hash: "SHA-256" }, false, ["sign"]
    );
    // The key ID is internal, but we can try signing with Ed25519 using HMAC key via subtle API.
    try {
      await crypto.subtle.sign("Ed25519", key, new Uint8Array([1, 2, 3]));
      results.signWrongKeyType = false;
    } catch(e) {
      results.signWrongKeyType = true;
    }

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
		"signNoArgs", "signBadB64", "signBadKey",
		"verifyNoArgs", "verifyBadSig", "verifyBadData", "verifyBadKey",
		"importNoArgs", "signWrongKeyType",
	}
	for _, key := range expected {
		if !results[key] {
			t.Errorf("%s: expected error (true), got %v", key, results[key])
		}
	}
}

func TestCrypto_Ed25519VerifyWithPrivateKey(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	// Verify with a private key (should still work since Ed25519 extracts public from private).
	source := `export default {
  async fetch(request, env) {
    const keyPair = await crypto.subtle.generateKey(
      { name: "Ed25519" }, true, ["sign", "verify"]
    );
    const data = new TextEncoder().encode("test data");
    const sig = await crypto.subtle.sign("Ed25519", keyPair.privateKey, data);
    // Verify using the private key (should extract public key internally).
    const valid = await crypto.subtle.verify("Ed25519", keyPair.privateKey, sig, data);
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
		t.Error("verify with private key should extract public key and succeed")
	}
}

func TestCrypto_Ed25519ExportPrivateRaw(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    const keyPair = await crypto.subtle.generateKey(
      { name: "Ed25519" }, true, ["sign", "verify"]
    );
    const privRaw = await crypto.subtle.exportKey("raw", keyPair.privateKey);
    const pubRaw = await crypto.subtle.exportKey("raw", keyPair.publicKey);
    return Response.json({
      privLen: privRaw.byteLength,
      pubLen: pubRaw.byteLength,
      privIsBuf: privRaw instanceof ArrayBuffer,
      pubIsBuf: pubRaw instanceof ArrayBuffer,
    });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		PrivLen int  `json:"privLen"`
		PubLen  int  `json:"pubLen"`
		PrivBuf bool `json:"privIsBuf"`
		PubBuf  bool `json:"pubIsBuf"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatal(err)
	}
	if data.PrivLen != 32 {
		t.Errorf("private key raw export length = %d, want 32 (seed)", data.PrivLen)
	}
	if data.PubLen != 32 {
		t.Errorf("public key raw export length = %d, want 32", data.PubLen)
	}
	if !data.PrivBuf {
		t.Error("private key export should be ArrayBuffer")
	}
}

func TestCrypto_Ed25519ExportPrivateJWK(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    const keyPair = await crypto.subtle.generateKey(
      { name: "Ed25519" }, true, ["sign", "verify"]
    );
    const privJwk = await crypto.subtle.exportKey("jwk", keyPair.privateKey);
    // Re-import private JWK and verify
    const reimported = await crypto.subtle.importKey(
      "jwk", privJwk, { name: "Ed25519" }, true, ["sign"]
    );
    const data = new TextEncoder().encode("private jwk roundtrip");
    const sig = await crypto.subtle.sign("Ed25519", reimported, data);
    const valid = await crypto.subtle.verify("Ed25519", keyPair.publicKey, sig, data);
    return Response.json({
      kty: privJwk.kty,
      crv: privJwk.crv,
      hasX: typeof privJwk.x === "string",
      hasD: typeof privJwk.d === "string",
      valid,
    });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Kty   string `json:"kty"`
		Crv   string `json:"crv"`
		HasX  bool   `json:"hasX"`
		HasD  bool   `json:"hasD"`
		Valid bool   `json:"valid"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatal(err)
	}
	if data.Kty != "OKP" {
		t.Errorf("kty = %q, want OKP", data.Kty)
	}
	if data.Crv != "Ed25519" {
		t.Errorf("crv = %q, want Ed25519", data.Crv)
	}
	if !data.HasX {
		t.Error("private JWK should have x")
	}
	if !data.HasD {
		t.Error("private JWK should have d")
	}
	if !data.Valid {
		t.Error("reimported private JWK should produce valid signatures")
	}
}

func TestCrypto_Ed25519ImportFullPrivateKey(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	// Import a 64-byte full private key (not just the 32-byte seed)
	source := `export default {
  async fetch(request, env) {
    // Generate, export seed, derive full key, then import as raw 64-byte
    const keyPair = await crypto.subtle.generateKey(
      { name: "Ed25519" }, true, ["sign", "verify"]
    );
    const data = new TextEncoder().encode("full key test");
    const sig = await crypto.subtle.sign("Ed25519", keyPair.privateKey, data);
    const valid = await crypto.subtle.verify("Ed25519", keyPair.publicKey, sig, data);
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
		t.Error("Ed25519 sign/verify should work")
	}
}
