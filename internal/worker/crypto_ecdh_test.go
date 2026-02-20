package worker

import (
	"encoding/json"
	"testing"
)

func TestCrypto_ECDHGenerateAndDeriveBitsP256(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    const aliceKeys = await crypto.subtle.generateKey(
      { name: "ECDH", namedCurve: "P-256" }, true, ["deriveBits"]
    );
    const bobKeys = await crypto.subtle.generateKey(
      { name: "ECDH", namedCurve: "P-256" }, true, ["deriveBits"]
    );

    const aliceShared = await crypto.subtle.deriveBits(
      { name: "ECDH", public: bobKeys.publicKey },
      aliceKeys.privateKey, 256
    );
    const bobShared = await crypto.subtle.deriveBits(
      { name: "ECDH", public: aliceKeys.publicKey },
      bobKeys.privateKey, 256
    );

    const aliceArr = new Uint8Array(aliceShared);
    const bobArr = new Uint8Array(bobShared);
    let match = aliceArr.length === bobArr.length;
    for (let i = 0; i < aliceArr.length; i++) {
      if (aliceArr[i] !== bobArr[i]) match = false;
    }

    return Response.json({
      match,
      sharedLen: aliceArr.length,
      privType: aliceKeys.privateKey.type,
      pubType: aliceKeys.publicKey.type,
      algoName: aliceKeys.privateKey.algorithm.name,
      curve: aliceKeys.privateKey.algorithm.namedCurve,
    });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Match     bool   `json:"match"`
		SharedLen int    `json:"sharedLen"`
		PrivType  string `json:"privType"`
		PubType   string `json:"pubType"`
		AlgoName  string `json:"algoName"`
		Curve     string `json:"curve"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !data.Match {
		t.Error("Alice and Bob should derive the same shared secret")
	}
	if data.SharedLen != 32 {
		t.Errorf("shared secret length = %d, want 32", data.SharedLen)
	}
	if data.PrivType != "private" {
		t.Errorf("private key type = %q, want 'private'", data.PrivType)
	}
	if data.PubType != "public" {
		t.Errorf("public key type = %q, want 'public'", data.PubType)
	}
	if data.AlgoName != "ECDH" {
		t.Errorf("algorithm name = %q, want 'ECDH'", data.AlgoName)
	}
	if data.Curve != "P-256" {
		t.Errorf("curve = %q, want 'P-256'", data.Curve)
	}
}

func TestCrypto_ECDHGenerateAndDeriveBitsP384(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    const aliceKeys = await crypto.subtle.generateKey(
      { name: "ECDH", namedCurve: "P-384" }, true, ["deriveBits"]
    );
    const bobKeys = await crypto.subtle.generateKey(
      { name: "ECDH", namedCurve: "P-384" }, true, ["deriveBits"]
    );

    const aliceShared = await crypto.subtle.deriveBits(
      { name: "ECDH", public: bobKeys.publicKey },
      aliceKeys.privateKey, 384
    );
    const bobShared = await crypto.subtle.deriveBits(
      { name: "ECDH", public: aliceKeys.publicKey },
      bobKeys.privateKey, 384
    );

    const aliceArr = new Uint8Array(aliceShared);
    const bobArr = new Uint8Array(bobShared);
    let match = aliceArr.length === bobArr.length;
    for (let i = 0; i < aliceArr.length; i++) {
      if (aliceArr[i] !== bobArr[i]) match = false;
    }

    return Response.json({
      match,
      sharedLen: aliceArr.length,
      curve: aliceKeys.privateKey.algorithm.namedCurve,
    });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Match     bool   `json:"match"`
		SharedLen int    `json:"sharedLen"`
		Curve     string `json:"curve"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !data.Match {
		t.Error("P-384 ECDH: Alice and Bob should derive the same shared secret")
	}
	if data.SharedLen != 48 {
		t.Errorf("P-384 shared secret length = %d, want 48", data.SharedLen)
	}
	if data.Curve != "P-384" {
		t.Errorf("curve = %q, want 'P-384'", data.Curve)
	}
}

func TestCrypto_ECDHDeriveKeyAESGCM(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    const aliceKeys = await crypto.subtle.generateKey(
      { name: "ECDH", namedCurve: "P-256" }, true, ["deriveKey"]
    );
    const bobKeys = await crypto.subtle.generateKey(
      { name: "ECDH", namedCurve: "P-256" }, true, ["deriveKey"]
    );

    // Derive AES-GCM key from ECDH shared secret
    const aesKey = await crypto.subtle.deriveKey(
      { name: "ECDH", public: bobKeys.publicKey },
      aliceKeys.privateKey,
      { name: "AES-GCM", length: 256 },
      false,
      ["encrypt", "decrypt"]
    );

    // Encrypt with derived key
    const iv = crypto.getRandomValues(new Uint8Array(12));
    const plaintext = new TextEncoder().encode("Hello ECDH derived key!");
    const ciphertext = await crypto.subtle.encrypt(
      { name: "AES-GCM", iv }, aesKey, plaintext
    );

    // Bob derives same key and decrypts
    const bobAesKey = await crypto.subtle.deriveKey(
      { name: "ECDH", public: aliceKeys.publicKey },
      bobKeys.privateKey,
      { name: "AES-GCM", length: 256 },
      false,
      ["encrypt", "decrypt"]
    );
    const decrypted = await crypto.subtle.decrypt(
      { name: "AES-GCM", iv }, bobAesKey, ciphertext
    );
    const decryptedText = new TextDecoder().decode(decrypted);

    return Response.json({
      decryptedText,
      aesKeyType: aesKey.type,
      aesAlgo: aesKey.algorithm.name,
    });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		DecryptedText string `json:"decryptedText"`
		AesKeyType    string `json:"aesKeyType"`
		AesAlgo       string `json:"aesAlgo"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if data.DecryptedText != "Hello ECDH derived key!" {
		t.Errorf("decrypted = %q, want 'Hello ECDH derived key!'", data.DecryptedText)
	}
	if data.AesKeyType != "secret" {
		t.Errorf("aes key type = %q, want 'secret'", data.AesKeyType)
	}
	if data.AesAlgo != "AES-GCM" {
		t.Errorf("aes algo = %q, want 'AES-GCM'", data.AesAlgo)
	}
}

func TestCrypto_ECDHImportExportRaw(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    const keyPair = await crypto.subtle.generateKey(
      { name: "ECDH", namedCurve: "P-256" }, true, ["deriveBits"]
    );

    // Export public key as raw
    const rawPub = await crypto.subtle.exportKey("raw", keyPair.publicKey);
    const pubArr = new Uint8Array(rawPub);

    // Re-import public key
    const importedPub = await crypto.subtle.importKey(
      "raw", rawPub, { name: "ECDH", namedCurve: "P-256" }, true, []
    );

    // Generate another keypair and derive with imported key
    const otherKeys = await crypto.subtle.generateKey(
      { name: "ECDH", namedCurve: "P-256" }, true, ["deriveBits"]
    );

    const shared1 = await crypto.subtle.deriveBits(
      { name: "ECDH", public: keyPair.publicKey },
      otherKeys.privateKey, 256
    );
    const shared2 = await crypto.subtle.deriveBits(
      { name: "ECDH", public: importedPub },
      otherKeys.privateKey, 256
    );

    const arr1 = new Uint8Array(shared1);
    const arr2 = new Uint8Array(shared2);
    let match = arr1.length === arr2.length;
    for (let i = 0; i < arr1.length; i++) {
      if (arr1[i] !== arr2[i]) match = false;
    }

    return Response.json({
      match,
      pubKeyLen: pubArr.length,
      importedType: importedPub.type,
    });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Match        bool   `json:"match"`
		PubKeyLen    int    `json:"pubKeyLen"`
		ImportedType string `json:"importedType"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !data.Match {
		t.Error("deriveBits with imported key should produce same result as original")
	}
	// P-256 uncompressed public key: 1 + 32 + 32 = 65 bytes
	if data.PubKeyLen != 65 {
		t.Errorf("P-256 raw public key length = %d, want 65", data.PubKeyLen)
	}
	if data.ImportedType != "public" {
		t.Errorf("imported key type = %q, want 'public'", data.ImportedType)
	}
}

func TestCrypto_ECDHImportExportJWK(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    const keyPair = await crypto.subtle.generateKey(
      { name: "ECDH", namedCurve: "P-256" }, true, ["deriveBits"]
    );

    const pubJWK = await crypto.subtle.exportKey("jwk", keyPair.publicKey);
    const privJWK = await crypto.subtle.exportKey("jwk", keyPair.privateKey);

    const pubValid = !!(pubJWK.kty === "EC" && pubJWK.crv === "P-256" && pubJWK.x && pubJWK.y && !pubJWK.d);
    const privValid = !!(privJWK.kty === "EC" && privJWK.crv === "P-256" && privJWK.x && privJWK.y && privJWK.d);

    // Re-import and verify deriveBits matches
    const importedPriv = await crypto.subtle.importKey(
      "jwk", privJWK, { name: "ECDH", namedCurve: "P-256" }, true, ["deriveBits"]
    );
    const importedPub = await crypto.subtle.importKey(
      "jwk", pubJWK, { name: "ECDH", namedCurve: "P-256" }, true, []
    );

    const shared1 = await crypto.subtle.deriveBits(
      { name: "ECDH", public: importedPub },
      keyPair.privateKey, 256
    );
    const shared2 = await crypto.subtle.deriveBits(
      { name: "ECDH", public: keyPair.publicKey },
      importedPriv, 256
    );

    const arr1 = new Uint8Array(shared1);
    const arr2 = new Uint8Array(shared2);
    let match = arr1.length === arr2.length;
    for (let i = 0; i < arr1.length; i++) {
      if (arr1[i] !== arr2[i]) match = false;
    }

    return Response.json({ pubValid, privValid, match });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		PubValid  bool `json:"pubValid"`
		PrivValid bool `json:"privValid"`
		Match     bool `json:"match"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !data.PubValid {
		t.Error("public JWK should have kty=EC, crv=P-256, x, y, no d")
	}
	if !data.PrivValid {
		t.Error("private JWK should have kty=EC, crv=P-256, x, y, d")
	}
	if !data.Match {
		t.Error("JWK round-trip should produce same deriveBits results")
	}
}

func TestCrypto_X25519GenerateAndDeriveBits(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    const aliceKeys = await crypto.subtle.generateKey(
      { name: "X25519" }, true, ["deriveBits"]
    );
    const bobKeys = await crypto.subtle.generateKey(
      { name: "X25519" }, true, ["deriveBits"]
    );

    const aliceShared = await crypto.subtle.deriveBits(
      { name: "X25519", public: bobKeys.publicKey },
      aliceKeys.privateKey, 256
    );
    const bobShared = await crypto.subtle.deriveBits(
      { name: "X25519", public: aliceKeys.publicKey },
      bobKeys.privateKey, 256
    );

    const aliceArr = new Uint8Array(aliceShared);
    const bobArr = new Uint8Array(bobShared);
    let match = aliceArr.length === bobArr.length;
    for (let i = 0; i < aliceArr.length; i++) {
      if (aliceArr[i] !== bobArr[i]) match = false;
    }

    return Response.json({
      match,
      sharedLen: aliceArr.length,
      privType: aliceKeys.privateKey.type,
      pubType: aliceKeys.publicKey.type,
      algoName: aliceKeys.privateKey.algorithm.name,
    });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Match     bool   `json:"match"`
		SharedLen int    `json:"sharedLen"`
		PrivType  string `json:"privType"`
		PubType   string `json:"pubType"`
		AlgoName  string `json:"algoName"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !data.Match {
		t.Error("X25519: Alice and Bob should derive the same shared secret")
	}
	if data.SharedLen != 32 {
		t.Errorf("X25519 shared secret length = %d, want 32", data.SharedLen)
	}
	if data.PrivType != "private" {
		t.Errorf("private key type = %q, want 'private'", data.PrivType)
	}
	if data.PubType != "public" {
		t.Errorf("public key type = %q, want 'public'", data.PubType)
	}
	if data.AlgoName != "X25519" {
		t.Errorf("algorithm name = %q, want 'X25519'", data.AlgoName)
	}
}

func TestCrypto_X25519ImportExportRaw(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    const keyPair = await crypto.subtle.generateKey(
      { name: "X25519" }, true, ["deriveBits"]
    );

    // Export public key as raw
    const rawPub = await crypto.subtle.exportKey("raw", keyPair.publicKey);
    const pubArr = new Uint8Array(rawPub);

    // Export private key as raw
    const rawPriv = await crypto.subtle.exportKey("raw", keyPair.privateKey);
    const privArr = new Uint8Array(rawPriv);

    // Re-import public key
    const importedPub = await crypto.subtle.importKey(
      "raw", rawPub, { name: "X25519" }, true, []
    );

    // Re-import private key
    const importedPriv = await crypto.subtle.importKey(
      "raw", rawPriv, { name: "X25519" }, true, ["deriveBits"]
    );

    // Verify round-trip: derive with imported keys matches original
    const otherKeys = await crypto.subtle.generateKey(
      { name: "X25519" }, true, ["deriveBits"]
    );

    const shared1 = await crypto.subtle.deriveBits(
      { name: "X25519", public: otherKeys.publicKey },
      keyPair.privateKey, 256
    );
    const shared2 = await crypto.subtle.deriveBits(
      { name: "X25519", public: otherKeys.publicKey },
      importedPriv, 256
    );

    const arr1 = new Uint8Array(shared1);
    const arr2 = new Uint8Array(shared2);
    let privMatch = arr1.length === arr2.length;
    for (let i = 0; i < arr1.length; i++) {
      if (arr1[i] !== arr2[i]) privMatch = false;
    }

    return Response.json({
      pubKeyLen: pubArr.length,
      privKeyLen: privArr.length,
      importedPubType: importedPub.type,
      importedPrivType: importedPriv.type,
      privMatch,
    });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		PubKeyLen        int    `json:"pubKeyLen"`
		PrivKeyLen       int    `json:"privKeyLen"`
		ImportedPubType  string `json:"importedPubType"`
		ImportedPrivType string `json:"importedPrivType"`
		PrivMatch        bool   `json:"privMatch"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if data.PubKeyLen != 32 {
		t.Errorf("X25519 public key length = %d, want 32", data.PubKeyLen)
	}
	if data.PrivKeyLen != 32 {
		t.Errorf("X25519 private key length = %d, want 32", data.PrivKeyLen)
	}
	if data.ImportedPubType != "public" {
		t.Errorf("imported pub type = %q, want 'public'", data.ImportedPubType)
	}
	if data.ImportedPrivType != "private" {
		t.Errorf("imported priv type = %q, want 'private'", data.ImportedPrivType)
	}
	if !data.PrivMatch {
		t.Error("imported private key should produce same deriveBits as original")
	}
}

func TestCrypto_X25519DeriveKey(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    const aliceKeys = await crypto.subtle.generateKey(
      { name: "X25519" }, true, ["deriveKey"]
    );
    const bobKeys = await crypto.subtle.generateKey(
      { name: "X25519" }, true, ["deriveKey"]
    );

    // Derive AES-GCM key
    const aesKey = await crypto.subtle.deriveKey(
      { name: "X25519", public: bobKeys.publicKey },
      aliceKeys.privateKey,
      { name: "AES-GCM", length: 256 },
      false,
      ["encrypt", "decrypt"]
    );

    // Encrypt
    const iv = crypto.getRandomValues(new Uint8Array(12));
    const plaintext = new TextEncoder().encode("X25519 derived key test");
    const ciphertext = await crypto.subtle.encrypt(
      { name: "AES-GCM", iv }, aesKey, plaintext
    );

    // Bob derives same key and decrypts
    const bobKey = await crypto.subtle.deriveKey(
      { name: "X25519", public: aliceKeys.publicKey },
      bobKeys.privateKey,
      { name: "AES-GCM", length: 256 },
      false,
      ["encrypt", "decrypt"]
    );
    const decrypted = await crypto.subtle.decrypt(
      { name: "AES-GCM", iv }, bobKey, ciphertext
    );
    const decryptedText = new TextDecoder().decode(decrypted);

    return Response.json({ decryptedText });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		DecryptedText string `json:"decryptedText"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if data.DecryptedText != "X25519 derived key test" {
		t.Errorf("decrypted = %q, want 'X25519 derived key test'", data.DecryptedText)
	}
}

func TestCrypto_ECDHNonExtractableExportErrors(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    const keyPair = await crypto.subtle.generateKey(
      { name: "ECDH", namedCurve: "P-256" }, false, ["deriveBits"]
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
		t.Error("exporting non-extractable ECDH key should fail")
	}
}

func TestCrypto_X25519NonExtractableExportErrors(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    const keyPair = await crypto.subtle.generateKey(
      { name: "X25519" }, false, ["deriveBits"]
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
		t.Error("exporting non-extractable X25519 key should fail")
	}
}

func TestCrypto_ECDHUnsupportedCurveErrors(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    let failed = false;
    try {
      await crypto.subtle.generateKey(
        { name: "ECDH", namedCurve: "P-192" }, true, ["deriveBits"]
      );
    } catch (e) {
      failed = true;
    }
    return Response.json({ failed });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Failed bool `json:"failed"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !data.Failed {
		t.Error("generating ECDH key with unsupported curve should fail")
	}
}

func TestCrypto_X25519ImportInvalidLengthErrors(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    let importFailed = false;
    try {
      const badKey = new Uint8Array(16);
      await crypto.subtle.importKey("raw", badKey, { name: "X25519" }, true, []);
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
		t.Error("importing X25519 key with invalid length should fail")
	}
}

// TestCrypto_ECDHP521GenerateAndDeriveBits exercises the P-521 branch in ecdhCurveFromName.
func TestCrypto_ECDHP521GenerateAndDeriveBits(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    const aliceKeys = await crypto.subtle.generateKey(
      { name: "ECDH", namedCurve: "P-521" }, true, ["deriveBits"]
    );
    const bobKeys = await crypto.subtle.generateKey(
      { name: "ECDH", namedCurve: "P-521" }, true, ["deriveBits"]
    );

    const aliceShared = await crypto.subtle.deriveBits(
      { name: "ECDH", public: bobKeys.publicKey },
      aliceKeys.privateKey, 528
    );
    const bobShared = await crypto.subtle.deriveBits(
      { name: "ECDH", public: aliceKeys.publicKey },
      bobKeys.privateKey, 528
    );

    const a = new Uint8Array(aliceShared);
    const b = new Uint8Array(bobShared);
    let match = a.length === b.length;
    for (let i = 0; i < a.length; i++) {
      if (a[i] !== b[i]) match = false;
    }

    // Also test JWK round-trip for P-521
    const pubJWK = await crypto.subtle.exportKey("jwk", aliceKeys.publicKey);
    const privJWK = await crypto.subtle.exportKey("jwk", aliceKeys.privateKey);

    const importedPub = await crypto.subtle.importKey(
      "jwk", pubJWK, { name: "ECDH", namedCurve: "P-521" }, true, []
    );
    const importedPriv = await crypto.subtle.importKey(
      "jwk", privJWK, { name: "ECDH", namedCurve: "P-521" }, true, ["deriveBits"]
    );

    const shared2 = await crypto.subtle.deriveBits(
      { name: "ECDH", public: importedPub },
      importedPriv, 528
    );
    const c = new Uint8Array(shared2);
    let jwkMatch = c.length === a.length;

    return Response.json({
      match,
      jwkMatch,
      sharedLen: a.length,
      pubCrv: pubJWK.crv,
      privHasD: !!privJWK.d,
    });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Match     bool   `json:"match"`
		JWKMatch  bool   `json:"jwkMatch"`
		SharedLen int    `json:"sharedLen"`
		PubCrv    string `json:"pubCrv"`
		PrivHasD  bool   `json:"privHasD"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatal(err)
	}
	if !data.Match {
		t.Error("P-521 deriveBits should match between alice and bob")
	}
	if !data.JWKMatch {
		t.Error("P-521 JWK round-trip should produce valid keys")
	}
	if data.PubCrv != "P-521" {
		t.Errorf("pubJWK.crv = %q, want P-521", data.PubCrv)
	}
	if !data.PrivHasD {
		t.Error("private JWK should have d field")
	}
}

// TestCrypto_ECDHImportJWKErrors exercises error branches in importECDHJWK.
func TestCrypto_ECDHImportJWKErrors(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    var wrongKty = false;
    try {
      await crypto.subtle.importKey("jwk",
        { kty: "RSA", crv: "P-256", x: "AAAA", y: "BBBB" },
        { name: "ECDH", namedCurve: "P-256" }, true, []);
    } catch (e) { wrongKty = true; }

    var wrongCrv = false;
    try {
      await crypto.subtle.importKey("jwk",
        { kty: "EC", crv: "P-384", x: "AAAA", y: "BBBB" },
        { name: "ECDH", namedCurve: "P-256" }, true, []);
    } catch (e) { wrongCrv = true; }

    var badX = false;
    try {
      await crypto.subtle.importKey("jwk",
        { kty: "EC", crv: "P-256", x: "!!!invalid!!!", y: "BBBB" },
        { name: "ECDH", namedCurve: "P-256" }, true, []);
    } catch (e) { badX = true; }

    return Response.json({ wrongKty, wrongCrv, badX });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		WrongKty bool `json:"wrongKty"`
		WrongCrv bool `json:"wrongCrv"`
		BadX     bool `json:"badX"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatal(err)
	}
	if !data.WrongKty {
		t.Error("import JWK with wrong kty should fail")
	}
	if !data.WrongCrv {
		t.Error("import JWK with wrong crv should fail")
	}
	if !data.BadX {
		t.Error("import JWK with invalid x should fail")
	}
}
