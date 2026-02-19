package worker

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"

	v8 "github.com/tommie/v8go"
)

// cryptoEd25519JS patches crypto.subtle to support Ed25519 sign/verify/import/export/generate.
// Uses chain-of-responsibility: saves references to previous implementations
// and delegates non-Ed25519 calls to them.
const cryptoEd25519JS = `
(function() {
var subtle = crypto.subtle;
var CK = CryptoKey;
var _prevSign = subtle.sign;
var _prevVerify = subtle.verify;
var _prevImportKey = subtle.importKey;
var _prevExportKey = subtle.exportKey;
var _prevGenerateKey = subtle.generateKey;

subtle.sign = async function(algorithm, key, data) {
	var algo = typeof algorithm === 'string' ? { name: algorithm } : algorithm;
	if (algo.name === 'Ed25519') {
		var resultB64 = __cryptoSignEd25519(key._id, __bufferSourceToB64(data));
		return __b64ToBuffer(resultB64);
	}
	return _prevSign.call(this, algorithm, key, data);
};

subtle.verify = async function(algorithm, key, signature, data) {
	var algo = typeof algorithm === 'string' ? { name: algorithm } : algorithm;
	if (algo.name === 'Ed25519') {
		return __cryptoVerifyEd25519(key._id, __bufferSourceToB64(signature), __bufferSourceToB64(data));
	}
	return _prevVerify.call(this, algorithm, key, signature, data);
};

subtle.importKey = async function(format, keyData, algorithm, extractable, usages) {
	var algo = typeof algorithm === 'string' ? { name: algorithm } : algorithm;
	if (algo.name === 'Ed25519') {
		var dataStr;
		if (format === 'jwk') {
			dataStr = JSON.stringify(keyData);
		} else {
			dataStr = __bufferSourceToB64(keyData);
		}
		var resultJSON = __cryptoImportKeyEd25519(format, dataStr);
		var result = JSON.parse(resultJSON);
		if (result.error) throw new TypeError(result.error);
		return new CK(result.keyId, { name: 'Ed25519' }, result.keyType, extractable, usages);
	}
	return _prevImportKey.call(this, format, keyData, algorithm, extractable, usages);
};

subtle.exportKey = async function(format, key) {
	if (key.algorithm.name === 'Ed25519') {
		if (!key.extractable) throw new DOMException('key is not extractable', 'InvalidAccessError');
		var resultStr = __cryptoExportKeyEd25519(key._id, format);
		if (format === 'jwk') {
			return JSON.parse(resultStr);
		}
		return __b64ToBuffer(resultStr);
	}
	return _prevExportKey.call(this, format, key);
};

subtle.generateKey = async function(algorithm, extractable, usages) {
	var algo = typeof algorithm === 'string' ? { name: algorithm } : algorithm;
	if (algo.name === 'Ed25519') {
		var resultJSON = __cryptoGenerateKeyEd25519();
		var result = JSON.parse(resultJSON);
		if (result.error) throw new TypeError(result.error);
		return {
			privateKey: new CK(result.privateKeyId, { name: 'Ed25519' }, 'private', extractable,
				usages.filter(function(u) { return u === 'sign'; })),
			publicKey: new CK(result.publicKeyId, { name: 'Ed25519' }, 'public', extractable,
				usages.filter(function(u) { return u === 'verify'; })),
		};
	}
	return _prevGenerateKey.call(this, algorithm, extractable, usages);
};

})();
`

// setupCryptoEd25519 registers Ed25519 sign/verify/import/export/generate.
// Must run after setupCryptoExt.
func setupCryptoEd25519(iso *v8.Isolate, ctx *v8.Context, el *eventLoop) error {
	// __cryptoSignEd25519(keyID, dataB64) -> sigB64
	ctx.Global().Set("__cryptoSignEd25519", v8.NewFunctionTemplate(iso, func(info *v8.FunctionCallbackInfo) *v8.Value {
		args := info.Args()
		if len(args) < 2 {
			return throwError(iso, "signEd25519 requires 2 argument(s)")
		}
		keyID := args[0].Integer()
		dataB64 := args[1].String()

		data, err := base64.StdEncoding.DecodeString(dataB64)
		if err != nil {
			return throwError(iso, "signEd25519: invalid base64")
		}

		reqID := getReqIDFromJS(ctx)
		entry := getCryptoKey(reqID, keyID)
		if entry == nil {
			return throwError(iso, "signEd25519: key not found")
		}

		privKey, ok := entry.ecKey.(ed25519.PrivateKey)
		if !ok {
			return throwError(iso, "signEd25519: key is not an Ed25519 private key")
		}

		sig := ed25519.Sign(privKey, data)
		val, _ := v8.NewValue(iso, base64.StdEncoding.EncodeToString(sig))
		return val
	}).GetFunction(ctx))

	// __cryptoVerifyEd25519(keyID, sigB64, dataB64) -> bool
	ctx.Global().Set("__cryptoVerifyEd25519", v8.NewFunctionTemplate(iso, func(info *v8.FunctionCallbackInfo) *v8.Value {
		args := info.Args()
		if len(args) < 3 {
			return throwError(iso, "verifyEd25519 requires 3 argument(s)")
		}
		keyID := args[0].Integer()
		sigB64 := args[1].String()
		dataB64 := args[2].String()

		sig, err := base64.StdEncoding.DecodeString(sigB64)
		if err != nil {
			return throwError(iso, "verifyEd25519: invalid signature base64")
		}
		data, err := base64.StdEncoding.DecodeString(dataB64)
		if err != nil {
			return throwError(iso, "verifyEd25519: invalid data base64")
		}

		reqID := getReqIDFromJS(ctx)
		entry := getCryptoKey(reqID, keyID)
		if entry == nil {
			return throwError(iso, "verifyEd25519: key not found")
		}

		var pubKey ed25519.PublicKey
		switch k := entry.ecKey.(type) {
		case ed25519.PublicKey:
			pubKey = k
		case ed25519.PrivateKey:
			pubKey = k.Public().(ed25519.PublicKey)
		default:
			return throwError(iso, "verifyEd25519: key is not an Ed25519 key")
		}

		val, _ := v8.NewValue(iso, ed25519.Verify(pubKey, data, sig))
		return val
	}).GetFunction(ctx))

	// __cryptoGenerateKeyEd25519() -> JSON { privateKeyId, publicKeyId }
	ctx.Global().Set("__cryptoGenerateKeyEd25519", v8.NewFunctionTemplate(iso, func(info *v8.FunctionCallbackInfo) *v8.Value {
		reqID := getReqIDFromJS(ctx)
		if getRequestState(reqID) == nil {
			val, _ := v8.NewValue(iso, `{"error":"no active request state"}`)
			return val
		}

		pubKey, privKey, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			val, _ := v8.NewValue(iso, fmt.Sprintf(`{"error":"key generation failed: %s"}`, err.Error()))
			return val
		}

		privID := importCryptoKeyFull(reqID, &cryptoKeyEntry{
			algoName: "Ed25519", keyType: "private", ecKey: privKey,
		})
		pubID := importCryptoKeyFull(reqID, &cryptoKeyEntry{
			algoName: "Ed25519", keyType: "public", ecKey: pubKey,
		})

		val, _ := v8.NewValue(iso, fmt.Sprintf(`{"privateKeyId":%d,"publicKeyId":%d}`, privID, pubID))
		return val
	}).GetFunction(ctx))

	// __cryptoImportKeyEd25519(format, dataStr) -> JSON { keyId, keyType }
	ctx.Global().Set("__cryptoImportKeyEd25519", v8.NewFunctionTemplate(iso, func(info *v8.FunctionCallbackInfo) *v8.Value {
		args := info.Args()
		if len(args) < 2 {
			return throwError(iso, "importKeyEd25519 requires 2 argument(s)")
		}
		format := args[0].String()
		dataStr := args[1].String()

		reqID := getReqIDFromJS(ctx)
		if getRequestState(reqID) == nil {
			val, _ := v8.NewValue(iso, `{"error":"no active request state"}`)
			return val
		}

		switch format {
		case "raw":
			keyData, err := base64.StdEncoding.DecodeString(dataStr)
			if err != nil {
				val, _ := v8.NewValue(iso, `{"error":"invalid base64"}`)
				return val
			}
			if len(keyData) == ed25519.PublicKeySize {
				id := importCryptoKeyFull(reqID, &cryptoKeyEntry{
					algoName: "Ed25519", keyType: "public",
					ecKey: ed25519.PublicKey(keyData),
				})
				val, _ := v8.NewValue(iso, fmt.Sprintf(`{"keyId":%d,"keyType":"public"}`, id))
				return val
			}
			if len(keyData) == ed25519.SeedSize {
				privKey := ed25519.NewKeyFromSeed(keyData)
				id := importCryptoKeyFull(reqID, &cryptoKeyEntry{
					algoName: "Ed25519", keyType: "private",
					ecKey: privKey,
				})
				val, _ := v8.NewValue(iso, fmt.Sprintf(`{"keyId":%d,"keyType":"private"}`, id))
				return val
			}
			if len(keyData) == ed25519.PrivateKeySize {
				id := importCryptoKeyFull(reqID, &cryptoKeyEntry{
					algoName: "Ed25519", keyType: "private",
					ecKey: ed25519.PrivateKey(keyData),
				})
				val, _ := v8.NewValue(iso, fmt.Sprintf(`{"keyId":%d,"keyType":"private"}`, id))
				return val
			}
			val, _ := v8.NewValue(iso, fmt.Sprintf(`{"error":"invalid Ed25519 key length: %d"}`, len(keyData)))
			return val

		case "jwk":
			var jwk map[string]interface{}
			if err := json.Unmarshal([]byte(dataStr), &jwk); err != nil {
				val, _ := v8.NewValue(iso, `{"error":"invalid JWK JSON"}`)
				return val
			}
			kty, _ := jwk["kty"].(string)
			crv, _ := jwk["crv"].(string)
			if kty != "OKP" || crv != "Ed25519" {
				val, _ := v8.NewValue(iso, `{"error":"JWK must have kty=OKP and crv=Ed25519"}`)
				return val
			}
			xB64, _ := jwk["x"].(string)
			xBytes, err := base64.RawURLEncoding.DecodeString(xB64)
			if err != nil || len(xBytes) != ed25519.PublicKeySize {
				val, _ := v8.NewValue(iso, `{"error":"invalid JWK x value"}`)
				return val
			}

			dB64, hasD := jwk["d"].(string)
			if hasD && dB64 != "" {
				dBytes, err := base64.RawURLEncoding.DecodeString(dB64)
				if err != nil || len(dBytes) != ed25519.SeedSize {
					val, _ := v8.NewValue(iso, `{"error":"invalid JWK d value"}`)
					return val
				}
				privKey := ed25519.NewKeyFromSeed(dBytes)
				id := importCryptoKeyFull(reqID, &cryptoKeyEntry{
					algoName: "Ed25519", keyType: "private", ecKey: privKey,
				})
				val, _ := v8.NewValue(iso, fmt.Sprintf(`{"keyId":%d,"keyType":"private"}`, id))
				return val
			}

			id := importCryptoKeyFull(reqID, &cryptoKeyEntry{
				algoName: "Ed25519", keyType: "public",
				ecKey: ed25519.PublicKey(xBytes),
			})
			val, _ := v8.NewValue(iso, fmt.Sprintf(`{"keyId":%d,"keyType":"public"}`, id))
			return val

		default:
			val, _ := v8.NewValue(iso, fmt.Sprintf(`{"error":"unsupported format %q"}`, format))
			return val
		}
	}).GetFunction(ctx))

	// __cryptoExportKeyEd25519(keyID, format) -> base64 or JSON string
	ctx.Global().Set("__cryptoExportKeyEd25519", v8.NewFunctionTemplate(iso, func(info *v8.FunctionCallbackInfo) *v8.Value {
		args := info.Args()
		if len(args) < 2 {
			return throwError(iso, "exportKeyEd25519 requires 2 argument(s)")
		}
		keyID := args[0].Integer()
		format := args[1].String()

		reqID := getReqIDFromJS(ctx)
		entry := getCryptoKey(reqID, keyID)
		if entry == nil {
			return throwError(iso, "exportKeyEd25519: key not found")
		}

		switch format {
		case "raw":
			switch k := entry.ecKey.(type) {
			case ed25519.PublicKey:
				val, _ := v8.NewValue(iso, base64.StdEncoding.EncodeToString([]byte(k)))
				return val
			case ed25519.PrivateKey:
				// Export the seed (first 32 bytes) for raw private key export
				val, _ := v8.NewValue(iso, base64.StdEncoding.EncodeToString(k.Seed()))
				return val
			default:
				return throwError(iso, "exportKeyEd25519: not an Ed25519 key")
			}

		case "jwk":
			jwk := map[string]string{
				"kty": "OKP",
				"crv": "Ed25519",
			}
			switch k := entry.ecKey.(type) {
			case ed25519.PublicKey:
				jwk["x"] = base64.RawURLEncoding.EncodeToString([]byte(k))
			case ed25519.PrivateKey:
				pubKey := k.Public().(ed25519.PublicKey)
				jwk["x"] = base64.RawURLEncoding.EncodeToString([]byte(pubKey))
				jwk["d"] = base64.RawURLEncoding.EncodeToString(k.Seed())
			default:
				return throwError(iso, "exportKeyEd25519: not an Ed25519 key")
			}
			data, _ := json.Marshal(jwk)
			val, _ := v8.NewValue(iso, string(data))
			return val

		default:
			return throwError(iso, fmt.Sprintf("exportKeyEd25519: unsupported format %q", format))
		}
	}).GetFunction(ctx))

	if _, err := ctx.RunScript(cryptoEd25519JS, "crypto_ed25519.js"); err != nil {
		return fmt.Errorf("evaluating crypto_ed25519.js: %w", err)
	}
	return nil
}
