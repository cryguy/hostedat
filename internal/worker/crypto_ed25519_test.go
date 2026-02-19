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
