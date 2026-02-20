package worker

import (
	"encoding/json"
	"testing"
)

func TestCrypto_RSAOAEP_EncryptDecrypt(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    const keyPair = await crypto.subtle.generateKey(
      { name: "RSA-OAEP", modulusLength: 2048, publicExponent: new Uint8Array([1, 0, 1]), hash: "SHA-256" },
      true, ["encrypt", "decrypt"]
    );
    const plaintext = new TextEncoder().encode("secret message for RSA-OAEP");
    const ct = await crypto.subtle.encrypt({ name: "RSA-OAEP" }, keyPair.publicKey, plaintext);
    const pt = await crypto.subtle.decrypt({ name: "RSA-OAEP" }, keyPair.privateKey, ct);
    const result = new TextDecoder().decode(pt);
    return Response.json({
      match: result === "secret message for RSA-OAEP",
      ctLength: new Uint8Array(ct).length,
      privType: keyPair.privateKey.type,
      pubType: keyPair.publicKey.type,
    });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Match    bool   `json:"match"`
		CtLength int    `json:"ctLength"`
		PrivType string `json:"privType"`
		PubType  string `json:"pubType"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if !data.Match {
		t.Error("RSA-OAEP decrypt should return the original plaintext")
	}
	if data.CtLength != 256 {
		t.Errorf("RSA-OAEP ciphertext length = %d, want 256 (2048-bit key)", data.CtLength)
	}
	if data.PrivType != "private" {
		t.Errorf("private key type = %q, want 'private'", data.PrivType)
	}
	if data.PubType != "public" {
		t.Errorf("public key type = %q, want 'public'", data.PubType)
	}
}

func TestCrypto_RSAOAEP_WithLabel(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    const keyPair = await crypto.subtle.generateKey(
      { name: "RSA-OAEP", modulusLength: 2048, publicExponent: new Uint8Array([1, 0, 1]), hash: "SHA-256" },
      true, ["encrypt", "decrypt"]
    );
    const label = new TextEncoder().encode("my-label");
    const plaintext = new TextEncoder().encode("labeled data");
    const ct = await crypto.subtle.encrypt({ name: "RSA-OAEP", label }, keyPair.publicKey, plaintext);
    const pt = await crypto.subtle.decrypt({ name: "RSA-OAEP", label }, keyPair.privateKey, ct);
    const result = new TextDecoder().decode(pt);

    // Decrypting with wrong label should fail
    let wrongLabelFailed = false;
    try {
      await crypto.subtle.decrypt(
        { name: "RSA-OAEP", label: new TextEncoder().encode("wrong-label") },
        keyPair.privateKey, ct
      );
    } catch (e) {
      wrongLabelFailed = true;
    }

    return Response.json({ match: result === "labeled data", wrongLabelFailed });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Match            bool `json:"match"`
		WrongLabelFailed bool `json:"wrongLabelFailed"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if !data.Match {
		t.Error("RSA-OAEP with label should decrypt correctly")
	}
	if !data.WrongLabelFailed {
		t.Error("RSA-OAEP decrypt with wrong label should fail")
	}
}

func TestCrypto_RSASSA_PKCS1v15_SignVerify(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    const keyPair = await crypto.subtle.generateKey(
      { name: "RSASSA-PKCS1-v1_5", modulusLength: 2048, publicExponent: new Uint8Array([1, 0, 1]), hash: "SHA-256" },
      true, ["sign", "verify"]
    );
    const data = new TextEncoder().encode("JWT payload simulation");
    const sig = await crypto.subtle.sign("RSASSA-PKCS1-v1_5", keyPair.privateKey, data);
    const valid = await crypto.subtle.verify("RSASSA-PKCS1-v1_5", keyPair.publicKey, sig, data);

    const tampered = new TextEncoder().encode("tampered payload");
    const invalid = await crypto.subtle.verify("RSASSA-PKCS1-v1_5", keyPair.publicKey, sig, tampered);

    return Response.json({
      valid, invalid,
      sigLength: new Uint8Array(sig).length,
    });
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
		t.Error("RSASSA-PKCS1-v1_5 verify should return true for correct data")
	}
	if data.Invalid {
		t.Error("RSASSA-PKCS1-v1_5 verify should return false for tampered data")
	}
	if data.SigLength != 256 {
		t.Errorf("RSASSA-PKCS1-v1_5 signature length = %d, want 256", data.SigLength)
	}
}

func TestCrypto_RSA_PSS_SignVerify(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    const keyPair = await crypto.subtle.generateKey(
      { name: "RSA-PSS", modulusLength: 2048, publicExponent: new Uint8Array([1, 0, 1]), hash: "SHA-256" },
      true, ["sign", "verify"]
    );
    const data = new TextEncoder().encode("PSS signing test");
    const sig = await crypto.subtle.sign(
      { name: "RSA-PSS", saltLength: 32 }, keyPair.privateKey, data
    );
    const valid = await crypto.subtle.verify(
      { name: "RSA-PSS", saltLength: 32 }, keyPair.publicKey, sig, data
    );
    const tampered = new TextEncoder().encode("wrong data");
    const invalid = await crypto.subtle.verify(
      { name: "RSA-PSS", saltLength: 32 }, keyPair.publicKey, sig, tampered
    );
    return Response.json({ valid, invalid, sigLength: new Uint8Array(sig).length });
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
		t.Error("RSA-PSS verify should return true for correct data")
	}
	if data.Invalid {
		t.Error("RSA-PSS verify should return false for tampered data")
	}
	if data.SigLength != 256 {
		t.Errorf("RSA-PSS signature length = %d, want 256", data.SigLength)
	}
}

func TestCrypto_RSA_JWKExportImportRoundTrip(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    const keyPair = await crypto.subtle.generateKey(
      { name: "RSASSA-PKCS1-v1_5", modulusLength: 2048, publicExponent: new Uint8Array([1, 0, 1]), hash: "SHA-256" },
      true, ["sign", "verify"]
    );

    // Export both keys as JWK
    const pubJWK = await crypto.subtle.exportKey("jwk", keyPair.publicKey);
    const privJWK = await crypto.subtle.exportKey("jwk", keyPair.privateKey);

    // Verify JWK structure
    const pubOK = !!(pubJWK.kty === "RSA" && pubJWK.n && pubJWK.e && !pubJWK.d);
    const privOK = !!(privJWK.kty === "RSA" && privJWK.n && privJWK.e && privJWK.d);

    // Re-import from JWK
    const importedPriv = await crypto.subtle.importKey(
      "jwk", privJWK,
      { name: "RSASSA-PKCS1-v1_5", hash: "SHA-256" },
      true, ["sign"]
    );
    const importedPub = await crypto.subtle.importKey(
      "jwk", pubJWK,
      { name: "RSASSA-PKCS1-v1_5", hash: "SHA-256" },
      true, ["verify"]
    );

    // Sign with re-imported private, verify with re-imported public
    const msg = new TextEncoder().encode("JWK round trip test");
    const sig = await crypto.subtle.sign("RSASSA-PKCS1-v1_5", importedPriv, msg);
    const verified = await crypto.subtle.verify("RSASSA-PKCS1-v1_5", importedPub, sig, msg);

    return Response.json({ pubOK, privOK, verified, alg: pubJWK.alg });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		PubOK    bool   `json:"pubOK"`
		PrivOK   bool   `json:"privOK"`
		Verified bool   `json:"verified"`
		Alg      string `json:"alg"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if !data.PubOK {
		t.Error("public JWK should have kty=RSA, n, e, no d")
	}
	if !data.PrivOK {
		t.Error("private JWK should have kty=RSA, n, e, d")
	}
	if !data.Verified {
		t.Error("JWK round-trip import/export should produce working keys")
	}
	if data.Alg != "RS256" {
		t.Errorf("JWK alg = %q, want 'RS256'", data.Alg)
	}
}

func TestCrypto_RSA_SPKIExportImportRoundTrip(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    const keyPair = await crypto.subtle.generateKey(
      { name: "RSASSA-PKCS1-v1_5", modulusLength: 2048, publicExponent: new Uint8Array([1, 0, 1]), hash: "SHA-256" },
      true, ["sign", "verify"]
    );

    // Export public key as SPKI
    const spkiData = await crypto.subtle.exportKey("spki", keyPair.publicKey);
    const spkiBytes = new Uint8Array(spkiData);

    // Re-import from SPKI
    const importedPub = await crypto.subtle.importKey(
      "spki", spkiData,
      { name: "RSASSA-PKCS1-v1_5", hash: "SHA-256" },
      true, ["verify"]
    );

    // Sign with original, verify with re-imported
    const msg = new TextEncoder().encode("SPKI round trip");
    const sig = await crypto.subtle.sign("RSASSA-PKCS1-v1_5", keyPair.privateKey, msg);
    const verified = await crypto.subtle.verify("RSASSA-PKCS1-v1_5", importedPub, sig, msg);

    return Response.json({ verified, spkiLength: spkiBytes.length });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Verified   bool `json:"verified"`
		SPKILength int  `json:"spkiLength"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if !data.Verified {
		t.Error("SPKI round-trip should produce a working public key")
	}
	if data.SPKILength == 0 {
		t.Error("SPKI export should produce non-empty data")
	}
}

func TestCrypto_RSA_PKCS8ExportImportRoundTrip(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    const keyPair = await crypto.subtle.generateKey(
      { name: "RSASSA-PKCS1-v1_5", modulusLength: 2048, publicExponent: new Uint8Array([1, 0, 1]), hash: "SHA-256" },
      true, ["sign", "verify"]
    );

    // Export private key as PKCS8
    const pkcs8Data = await crypto.subtle.exportKey("pkcs8", keyPair.privateKey);
    const pkcs8Bytes = new Uint8Array(pkcs8Data);

    // Re-import from PKCS8
    const importedPriv = await crypto.subtle.importKey(
      "pkcs8", pkcs8Data,
      { name: "RSASSA-PKCS1-v1_5", hash: "SHA-256" },
      true, ["sign"]
    );

    // Sign with re-imported, verify with original public
    const msg = new TextEncoder().encode("PKCS8 round trip");
    const sig = await crypto.subtle.sign("RSASSA-PKCS1-v1_5", importedPriv, msg);
    const verified = await crypto.subtle.verify("RSASSA-PKCS1-v1_5", keyPair.publicKey, sig, msg);

    return Response.json({ verified, pkcs8Length: pkcs8Bytes.length });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Verified    bool `json:"verified"`
		PKCS8Length int  `json:"pkcs8Length"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if !data.Verified {
		t.Error("PKCS8 round-trip should produce a working private key")
	}
	if data.PKCS8Length == 0 {
		t.Error("PKCS8 export should produce non-empty data")
	}
}

func TestCrypto_RSAOAEP_SHA384(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    const keyPair = await crypto.subtle.generateKey(
      { name: "RSA-OAEP", modulusLength: 2048, publicExponent: new Uint8Array([1, 0, 1]), hash: "SHA-384" },
      false, ["encrypt", "decrypt"]
    );
    const plaintext = new TextEncoder().encode("SHA-384 test");
    const ct = await crypto.subtle.encrypt({ name: "RSA-OAEP" }, keyPair.publicKey, plaintext);
    const pt = await crypto.subtle.decrypt({ name: "RSA-OAEP" }, keyPair.privateKey, ct);
    const result = new TextDecoder().decode(pt);
    return Response.json({ match: result === "SHA-384 test" });
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
		t.Error("RSA-OAEP with SHA-384 should encrypt/decrypt correctly")
	}
}

func TestCrypto_RSA_JWTWorkflow(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	// Simulates RS256 JWT signing and verification workflow
	source := `export default {
  async fetch(request, env) {
    const keyPair = await crypto.subtle.generateKey(
      { name: "RSASSA-PKCS1-v1_5", modulusLength: 2048, publicExponent: new Uint8Array([1, 0, 1]), hash: "SHA-256" },
      true, ["sign", "verify"]
    );

    // Build JWT header and payload
    function base64url(data) {
      var b64 = '';
      var bytes = new Uint8Array(data);
      var _b64chars = 'ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/';
      for (var i = 0; i < bytes.length; i += 3) {
        var b1 = bytes[i], b2 = bytes[i+1] || 0, b3 = bytes[i+2] || 0;
        b64 += _b64chars[b1 >> 2];
        b64 += _b64chars[((b1 & 3) << 4) | (b2 >> 4)];
        b64 += (i+1 < bytes.length) ? _b64chars[((b2 & 15) << 2) | (b3 >> 6)] : '=';
        b64 += (i+2 < bytes.length) ? _b64chars[b3 & 63] : '=';
      }
      return b64.replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, '');
    }

    const header = JSON.stringify({ alg: "RS256", typ: "JWT" });
    const payload = JSON.stringify({ sub: "1234567890", name: "Test User", iat: 1516239022 });
    const headerB64 = base64url(new TextEncoder().encode(header));
    const payloadB64 = base64url(new TextEncoder().encode(payload));
    const signingInput = headerB64 + "." + payloadB64;

    // Sign
    const sig = await crypto.subtle.sign(
      "RSASSA-PKCS1-v1_5",
      keyPair.privateKey,
      new TextEncoder().encode(signingInput)
    );
    const sigB64 = base64url(sig);
    const jwt = signingInput + "." + sigB64;

    // Verify
    const parts = jwt.split(".");
    const verifyInput = parts[0] + "." + parts[1];
    const valid = await crypto.subtle.verify(
      "RSASSA-PKCS1-v1_5",
      keyPair.publicKey,
      sig,
      new TextEncoder().encode(verifyInput)
    );

    return Response.json({ valid, parts: parts.length });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Valid bool `json:"valid"`
		Parts int  `json:"parts"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if !data.Valid {
		t.Error("RS256 JWT signature should verify correctly")
	}
	if data.Parts != 3 {
		t.Errorf("JWT should have 3 parts, got %d", data.Parts)
	}
}

func TestCrypto_RSA_SignWithPublicKeyErrors(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    const keyPair = await crypto.subtle.generateKey(
      { name: "RSASSA-PKCS1-v1_5", modulusLength: 2048, publicExponent: new Uint8Array([1, 0, 1]), hash: "SHA-256" },
      true, ["sign", "verify"]
    );
    let signWithPubFailed = false;
    try {
      const data = new TextEncoder().encode("test");
      await crypto.subtle.sign("RSASSA-PKCS1-v1_5", keyPair.publicKey, data);
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
		t.Error("signing with RSA public key should fail")
	}
}

func TestCrypto_RSA_DecryptWithPublicKeyErrors(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    const keyPair = await crypto.subtle.generateKey(
      { name: "RSA-OAEP", modulusLength: 2048, publicExponent: new Uint8Array([1, 0, 1]), hash: "SHA-256" },
      true, ["encrypt", "decrypt"]
    );
    const pt = new TextEncoder().encode("test");
    const ct = await crypto.subtle.encrypt({ name: "RSA-OAEP" }, keyPair.publicKey, pt);

    let decryptWithPubFailed = false;
    try {
      await crypto.subtle.decrypt({ name: "RSA-OAEP" }, keyPair.publicKey, ct);
    } catch (e) {
      decryptWithPubFailed = true;
    }
    return Response.json({ decryptWithPubFailed });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		DecryptWithPubFailed bool `json:"decryptWithPubFailed"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !data.DecryptWithPubFailed {
		t.Error("decrypting with RSA public key should fail")
	}
}

func TestCrypto_RSA_NonExtractableExportErrors(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    const keyPair = await crypto.subtle.generateKey(
      { name: "RSASSA-PKCS1-v1_5", modulusLength: 2048, publicExponent: new Uint8Array([1, 0, 1]), hash: "SHA-256" },
      false, ["sign", "verify"]
    );
    let exportFailed = false;
    try {
      await crypto.subtle.exportKey("jwk", keyPair.privateKey);
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
		t.Error("exporting non-extractable RSA key should fail")
	}
}

func TestCrypto_RSA_CrossKeyPairVerifyFails(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    const keyPairA = await crypto.subtle.generateKey(
      { name: "RSASSA-PKCS1-v1_5", modulusLength: 2048, publicExponent: new Uint8Array([1, 0, 1]), hash: "SHA-256" },
      false, ["sign", "verify"]
    );
    const keyPairB = await crypto.subtle.generateKey(
      { name: "RSASSA-PKCS1-v1_5", modulusLength: 2048, publicExponent: new Uint8Array([1, 0, 1]), hash: "SHA-256" },
      false, ["sign", "verify"]
    );
    const data = new TextEncoder().encode("cross key test");
    const sig = await crypto.subtle.sign("RSASSA-PKCS1-v1_5", keyPairA.privateKey, data);

    // Verify with correct key should pass
    const validSame = await crypto.subtle.verify("RSASSA-PKCS1-v1_5", keyPairA.publicKey, sig, data);
    // Verify with different key should fail
    const validCross = await crypto.subtle.verify("RSASSA-PKCS1-v1_5", keyPairB.publicKey, sig, data);

    return Response.json({ validSame, validCross });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		ValidSame  bool `json:"validSame"`
		ValidCross bool `json:"validCross"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !data.ValidSame {
		t.Error("verify with correct key pair should pass")
	}
	if data.ValidCross {
		t.Error("verify with different key pair should fail")
	}
}

func TestCrypto_RSA_OAEPPlaintextTooLargeErrors(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    const keyPair = await crypto.subtle.generateKey(
      { name: "RSA-OAEP", modulusLength: 2048, publicExponent: new Uint8Array([1, 0, 1]), hash: "SHA-256" },
      false, ["encrypt", "decrypt"]
    );
    // RSA-OAEP with SHA-256 and 2048-bit key: max plaintext = 256 - 2*32 - 2 = 190 bytes
    // Create plaintext larger than the key can handle
    const tooLarge = new Uint8Array(250);
    let encryptFailed = false;
    try {
      await crypto.subtle.encrypt({ name: "RSA-OAEP" }, keyPair.publicKey, tooLarge);
    } catch (e) {
      encryptFailed = true;
    }
    return Response.json({ encryptFailed });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		EncryptFailed bool `json:"encryptFailed"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !data.EncryptFailed {
		t.Error("encrypting plaintext larger than RSA-OAEP max should fail")
	}
}

func TestCrypto_RSA_ImportMalformedJWKErrors(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    // Missing required fields
    let missingNFailed = false;
    try {
      await crypto.subtle.importKey(
        "jwk", { kty: "RSA", e: "AQAB" },
        { name: "RSASSA-PKCS1-v1_5", hash: "SHA-256" },
        true, ["verify"]
      );
    } catch (e) {
      missingNFailed = true;
    }

    // Wrong kty
    let wrongKtyFailed = false;
    try {
      await crypto.subtle.importKey(
        "jwk", { kty: "EC", n: "test", e: "AQAB" },
        { name: "RSASSA-PKCS1-v1_5", hash: "SHA-256" },
        true, ["verify"]
      );
    } catch (e) {
      wrongKtyFailed = true;
    }

    return Response.json({ missingNFailed, wrongKtyFailed });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		MissingNFailed bool `json:"missingNFailed"`
		WrongKtyFailed bool `json:"wrongKtyFailed"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !data.MissingNFailed {
		t.Error("importing JWK with missing 'n' should fail")
	}
	if !data.WrongKtyFailed {
		t.Error("importing JWK with wrong kty should fail")
	}
}

func TestCrypto_RSA_SPKIExportFromPrivateKey(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    const keyPair = await crypto.subtle.generateKey(
      { name: "RSASSA-PKCS1-v1_5", modulusLength: 2048, publicExponent: new Uint8Array([1, 0, 1]), hash: "SHA-256" },
      true, ["sign", "verify"]
    );
    // Export SPKI from public key
    const spki = await crypto.subtle.exportKey("spki", keyPair.publicKey);
    // Re-import SPKI
    const imported = await crypto.subtle.importKey(
      "spki", spki, { name: "RSASSA-PKCS1-v1_5", hash: "SHA-256" }, true, ["verify"]
    );
    // Sign with original private, verify with re-imported public
    const data = new TextEncoder().encode("spki test");
    const sig = await crypto.subtle.sign("RSASSA-PKCS1-v1_5", keyPair.privateKey, data);
    const valid = await crypto.subtle.verify("RSASSA-PKCS1-v1_5", imported, sig, data);
    return Response.json({ valid, type: imported.type });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Valid bool   `json:"valid"`
		Type  string `json:"type"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatal(err)
	}
	if !data.Valid {
		t.Error("SPKI re-imported key should verify signatures")
	}
	if data.Type != "public" {
		t.Errorf("type = %q, want 'public'", data.Type)
	}
}

func TestCrypto_RSA_PKCS8ExportFromPrivateKey(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    const keyPair = await crypto.subtle.generateKey(
      { name: "RSASSA-PKCS1-v1_5", modulusLength: 2048, publicExponent: new Uint8Array([1, 0, 1]), hash: "SHA-256" },
      true, ["sign", "verify"]
    );
    // Export PKCS8 from private key
    const pkcs8 = await crypto.subtle.exportKey("pkcs8", keyPair.privateKey);
    // Re-import PKCS8
    const imported = await crypto.subtle.importKey(
      "pkcs8", pkcs8, { name: "RSASSA-PKCS1-v1_5", hash: "SHA-256" }, true, ["sign"]
    );
    // Sign with re-imported key, verify with original public
    const data = new TextEncoder().encode("pkcs8 test");
    const sig = await crypto.subtle.sign("RSASSA-PKCS1-v1_5", imported, data);
    const valid = await crypto.subtle.verify("RSASSA-PKCS1-v1_5", keyPair.publicKey, sig, data);
    return Response.json({ valid, type: imported.type });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Valid bool   `json:"valid"`
		Type  string `json:"type"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatal(err)
	}
	if !data.Valid {
		t.Error("PKCS8 re-imported key should sign correctly")
	}
	if data.Type != "private" {
		t.Errorf("type = %q, want 'private'", data.Type)
	}
}

func TestCrypto_RSA_ImportBadSPKI(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    let badDerFailed = false;
    try {
      await crypto.subtle.importKey(
        "spki", new Uint8Array([1, 2, 3, 4, 5]),
        { name: "RSASSA-PKCS1-v1_5", hash: "SHA-256" }, true, ["verify"]
      );
    } catch(e) {
      badDerFailed = true;
    }
    return Response.json({ badDerFailed });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		BadDerFailed bool `json:"badDerFailed"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatal(err)
	}
	if !data.BadDerFailed {
		t.Error("importing bad SPKI DER should fail")
	}
}

func TestCrypto_RSA_ImportBadPKCS8(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    let badDerFailed = false;
    try {
      await crypto.subtle.importKey(
        "pkcs8", new Uint8Array([1, 2, 3, 4, 5]),
        { name: "RSASSA-PKCS1-v1_5", hash: "SHA-256" }, true, ["sign"]
      );
    } catch(e) {
      badDerFailed = true;
    }
    return Response.json({ badDerFailed });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		BadDerFailed bool `json:"badDerFailed"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatal(err)
	}
	if !data.BadDerFailed {
		t.Error("importing bad PKCS8 DER should fail")
	}
}

func TestCrypto_RSA_OAEPWithSHA512(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    const keyPair = await crypto.subtle.generateKey(
      { name: "RSA-OAEP", modulusLength: 2048, publicExponent: new Uint8Array([1, 0, 1]), hash: "SHA-512" },
      true, ["encrypt", "decrypt"]
    );
    const data = new TextEncoder().encode("sha512 test");
    const ct = await crypto.subtle.encrypt({ name: "RSA-OAEP" }, keyPair.publicKey, data);
    const pt = await crypto.subtle.decrypt({ name: "RSA-OAEP" }, keyPair.privateKey, ct);
    const result = new TextDecoder().decode(pt);
    return Response.json({ result, ctLen: new Uint8Array(ct).length });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Result string `json:"result"`
		CTLen  int    `json:"ctLen"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatal(err)
	}
	if data.Result != "sha512 test" {
		t.Errorf("result = %q", data.Result)
	}
	if data.CTLen != 256 {
		t.Errorf("ciphertext length = %d, want 256", data.CTLen)
	}
}
