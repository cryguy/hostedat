package worker

import (
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"

	v8 "github.com/tommie/v8go"
)

// ecdhCurveFromName returns the ecdh.Curve for the given Web Crypto curve name.
func ecdhCurveFromName(name string) ecdh.Curve {
	switch name {
	case "P-256":
		return ecdh.P256()
	case "P-384":
		return ecdh.P384()
	case "P-521":
		return ecdh.P521()
	case "X25519":
		return ecdh.X25519()
	default:
		return nil
	}
}

// cryptoECDHJS patches crypto.subtle to support ECDH and X25519 key agreement.
// Uses chain-of-responsibility: saves references to previous implementations
// and delegates non-ECDH/X25519 calls to them.
const cryptoECDHJS = `
(function() {
var subtle = crypto.subtle;
var CK = CryptoKey;
var _prevGenerateKey = subtle.generateKey;
var _prevDeriveBits = subtle.deriveBits;
var _prevDeriveKey = subtle.deriveKey;
var _prevImportKey = subtle.importKey;
var _prevExportKey = subtle.exportKey;

subtle.generateKey = async function(algorithm, extractable, usages) {
	var algo = typeof algorithm === 'string' ? { name: algorithm } : algorithm;
	if (algo.name === 'ECDH') {
		var curve = algo.namedCurve || 'P-256';
		var resultJSON = __cryptoGenerateECDH(curve, extractable);
		var result = JSON.parse(resultJSON);
		if (result.error) throw new TypeError(result.error);
		return {
			privateKey: new CK(result.privateKeyId, { name: 'ECDH', namedCurve: curve }, 'private', extractable,
				usages.filter(function(u) { return u === 'deriveBits' || u === 'deriveKey'; })),
			publicKey: new CK(result.publicKeyId, { name: 'ECDH', namedCurve: curve }, 'public', extractable, []),
		};
	}
	if (algo.name === 'X25519') {
		var resultJSON = __cryptoGenerateX25519(extractable);
		var result = JSON.parse(resultJSON);
		if (result.error) throw new TypeError(result.error);
		return {
			privateKey: new CK(result.privateKeyId, { name: 'X25519' }, 'private', extractable,
				usages.filter(function(u) { return u === 'deriveBits' || u === 'deriveKey'; })),
			publicKey: new CK(result.publicKeyId, { name: 'X25519' }, 'public', extractable, []),
		};
	}
	return _prevGenerateKey.call(this, algorithm, extractable, usages);
};

subtle.deriveBits = async function(algorithm, baseKey, length) {
	var algo = typeof algorithm === 'string' ? { name: algorithm } : algorithm;
	if (algo.name === 'ECDH') {
		var pubKey = algo.public;
		if (!pubKey || !pubKey._id && pubKey._id !== 0) throw new TypeError('ECDH deriveBits requires algorithm.public');
		var resultB64 = __cryptoDeriveECDH(baseKey._id, pubKey._id, length);
		return __b64ToBuffer(resultB64);
	}
	if (algo.name === 'X25519') {
		var pubKey = algo.public;
		if (!pubKey || !pubKey._id && pubKey._id !== 0) throw new TypeError('X25519 deriveBits requires algorithm.public');
		var resultB64 = __cryptoDeriveX25519(baseKey._id, pubKey._id, length);
		return __b64ToBuffer(resultB64);
	}
	return _prevDeriveBits.call(this, algorithm, baseKey, length);
};

subtle.deriveKey = async function(algorithm, baseKey, derivedKeyAlgorithm, extractable, usages) {
	var algo = typeof algorithm === 'string' ? { name: algorithm } : algorithm;
	if (algo.name === 'ECDH' || algo.name === 'X25519') {
		var dkAlgo = typeof derivedKeyAlgorithm === 'string' ? { name: derivedKeyAlgorithm } : derivedKeyAlgorithm;
		var length = dkAlgo.length || 0;
		if (!length) {
			var algoName = (dkAlgo.name || '').toUpperCase();
			if (algoName === 'HMAC') {
				var h = dkAlgo.hash ? (typeof dkAlgo.hash === 'string' ? dkAlgo.hash : dkAlgo.hash.name) : 'SHA-256';
				switch (h) {
					case 'SHA-1': length = 160; break;
					case 'SHA-384': length = 384; break;
					case 'SHA-512': length = 512; break;
					default: length = 256; break;
				}
			} else if (algoName.indexOf('AES') === 0) {
				length = 256;
			} else {
				length = 256;
			}
		}
		var bits = await subtle.deriveBits(algorithm, baseKey, length);
		return subtle.importKey('raw', bits, derivedKeyAlgorithm, extractable, usages);
	}
	return _prevDeriveKey.call(this, algorithm, baseKey, derivedKeyAlgorithm, extractable, usages);
};

subtle.importKey = async function(format, keyData, algorithm, extractable, usages) {
	var algo = typeof algorithm === 'string' ? { name: algorithm } : algorithm;
	if (algo.name === 'ECDH') {
		var curve = algo.namedCurve || 'P-256';
		var dataStr;
		if (format === 'jwk') {
			dataStr = JSON.stringify(keyData);
		} else {
			dataStr = __bufferSourceToB64(keyData);
		}
		var resultJSON = __cryptoImportECDH(format, dataStr, curve, extractable);
		var result = JSON.parse(resultJSON);
		if (result.error) throw new TypeError(result.error);
		return new CK(result.keyId, { name: 'ECDH', namedCurve: curve }, result.keyType, extractable, usages);
	}
	if (algo.name === 'X25519') {
		var dataStr = __bufferSourceToB64(keyData);
		var keyType = 'public';
		if (usages && (usages.indexOf('deriveBits') >= 0 || usages.indexOf('deriveKey') >= 0)) {
			keyType = 'private';
		}
		var resultJSON = __cryptoImportX25519(format, dataStr, keyType, extractable);
		var result = JSON.parse(resultJSON);
		if (result.error) throw new TypeError(result.error);
		return new CK(result.keyId, { name: 'X25519' }, result.keyType, extractable, usages);
	}
	return _prevImportKey.call(this, format, keyData, algorithm, extractable, usages);
};

subtle.exportKey = async function(format, key) {
	if (key.algorithm.name === 'ECDH') {
		if (!key.extractable) throw new DOMException('key is not extractable', 'InvalidAccessError');
		var resultStr = __cryptoExportECDH(key._id, format);
		if (format === 'jwk') {
			return JSON.parse(resultStr);
		}
		return __b64ToBuffer(resultStr);
	}
	if (key.algorithm.name === 'X25519') {
		if (!key.extractable) throw new DOMException('key is not extractable', 'InvalidAccessError');
		var resultStr = __cryptoExportX25519(key._id, format);
		return __b64ToBuffer(resultStr);
	}
	return _prevExportKey.call(this, format, key);
};

})();
`

// setupCryptoECDH registers ECDH and X25519 key agreement operations.
// Must run after setupCryptoDerive.
func setupCryptoECDH(iso *v8.Isolate, ctx *v8.Context, _ *eventLoop) error {
	// __cryptoGenerateECDH(curve, extractable) -> JSON { privateKeyId, publicKeyId }
	_ = ctx.Global().Set("__cryptoGenerateECDH", v8.NewFunctionTemplate(iso, func(info *v8.FunctionCallbackInfo) *v8.Value {
		args := info.Args()
		if len(args) < 2 {
			return throwError(iso, "generateECDH requires 2 argument(s)")
		}
		curveName := args[0].String()
		extractableVal := args[1].Boolean()

		reqID := getReqIDFromJS(ctx)
		if getRequestState(reqID) == nil {
			val, _ := v8.NewValue(iso, `{"error":"no active request state"}`)
			return val
		}

		curve := ecdhCurveFromName(curveName)
		if curve == nil {
			val, _ := v8.NewValue(iso, fmt.Sprintf(`{"error":"unsupported curve %q"}`, curveName))
			return val
		}

		privKey, err := curve.GenerateKey(rand.Reader)
		if err != nil {
			val, _ := v8.NewValue(iso, fmt.Sprintf(`{"error":"key generation failed: %s"}`, err.Error()))
			return val
		}

		privID := importCryptoKeyFull(reqID, &cryptoKeyEntry{
			algoName: "ECDH", keyType: "private", namedCurve: curveName, ecKey: privKey, extractable: extractableVal,
		})
		pubID := importCryptoKeyFull(reqID, &cryptoKeyEntry{
			algoName: "ECDH", keyType: "public", namedCurve: curveName, ecKey: privKey.PublicKey(), extractable: extractableVal,
		})

		val, _ := v8.NewValue(iso, fmt.Sprintf(`{"privateKeyId":%d,"publicKeyId":%d}`, privID, pubID))
		return val
	}).GetFunction(ctx))

	// __cryptoDeriveECDH(privateKeyID, publicKeyID, lengthBits) -> base64 shared secret
	_ = ctx.Global().Set("__cryptoDeriveECDH", v8.NewFunctionTemplate(iso, func(info *v8.FunctionCallbackInfo) *v8.Value {
		args := info.Args()
		if len(args) < 3 {
			return throwError(iso, "deriveECDH requires 3 argument(s)")
		}
		privKeyID := args[0].Integer()
		pubKeyID := args[1].Integer()
		lengthBits := int(args[2].Int32())

		reqID := getReqIDFromJS(ctx)
		privEntry := getCryptoKey(reqID, privKeyID)
		if privEntry == nil {
			return throwError(iso, "deriveECDH: private key not found")
		}
		pubEntry := getCryptoKey(reqID, pubKeyID)
		if pubEntry == nil {
			return throwError(iso, "deriveECDH: public key not found")
		}

		privKey, ok := privEntry.ecKey.(*ecdh.PrivateKey)
		if !ok {
			return throwError(iso, "deriveECDH: key is not an ECDH private key")
		}
		pubKey, ok := pubEntry.ecKey.(*ecdh.PublicKey)
		if !ok {
			return throwError(iso, "deriveECDH: key is not an ECDH public key")
		}

		shared, err := privKey.ECDH(pubKey)
		if err != nil {
			return throwError(iso, fmt.Sprintf("deriveECDH: %s", err.Error()))
		}

		// Truncate to requested bit length
		lengthBytes := lengthBits / 8
		if lengthBytes > len(shared) {
			lengthBytes = len(shared)
		}

		val, _ := v8.NewValue(iso, base64.StdEncoding.EncodeToString(shared[:lengthBytes]))
		return val
	}).GetFunction(ctx))

	// __cryptoImportECDH(format, dataB64, curve, extractable) -> JSON { keyId, keyType }
	_ = ctx.Global().Set("__cryptoImportECDH", v8.NewFunctionTemplate(iso, func(info *v8.FunctionCallbackInfo) *v8.Value {
		args := info.Args()
		if len(args) < 4 {
			return throwError(iso, "importECDH requires 4 argument(s)")
		}
		format := args[0].String()
		dataStr := args[1].String()
		curveName := args[2].String()
		extractableVal := args[3].Boolean()

		reqID := getReqIDFromJS(ctx)
		if getRequestState(reqID) == nil {
			val, _ := v8.NewValue(iso, `{"error":"no active request state"}`)
			return val
		}

		curve := ecdhCurveFromName(curveName)
		if curve == nil {
			val, _ := v8.NewValue(iso, fmt.Sprintf(`{"error":"unsupported curve %q"}`, curveName))
			return val
		}

		switch format {
		case "raw":
			keyData, err := base64.StdEncoding.DecodeString(dataStr)
			if err != nil {
				val, _ := v8.NewValue(iso, `{"error":"invalid base64"}`)
				return val
			}
			// Raw format is always a public key (uncompressed point)
			pubKey, err := curve.NewPublicKey(keyData)
			if err != nil {
				val, _ := v8.NewValue(iso, fmt.Sprintf(`{"error":"invalid ECDH public key: %s"}`, err.Error()))
				return val
			}
			id := importCryptoKeyFull(reqID, &cryptoKeyEntry{
				algoName: "ECDH", keyType: "public", namedCurve: curveName, ecKey: pubKey, extractable: extractableVal,
			})
			val, _ := v8.NewValue(iso, fmt.Sprintf(`{"keyId":%d,"keyType":"public"}`, id))
			return val

		case "jwk":
			return importECDHJWK(iso, reqID, dataStr, curveName, curve, extractableVal)

		default:
			val, _ := v8.NewValue(iso, fmt.Sprintf(`{"error":"unsupported format %q"}`, format))
			return val
		}
	}).GetFunction(ctx))

	// __cryptoExportECDH(keyID, format) -> base64 or JSON string
	_ = ctx.Global().Set("__cryptoExportECDH", v8.NewFunctionTemplate(iso, func(info *v8.FunctionCallbackInfo) *v8.Value {
		args := info.Args()
		if len(args) < 2 {
			return throwError(iso, "exportECDH requires 2 argument(s)")
		}
		keyID := args[0].Integer()
		format := args[1].String()

		reqID := getReqIDFromJS(ctx)
		entry := getCryptoKey(reqID, keyID)
		if entry == nil {
			return throwError(iso, "exportECDH: key not found")
		}
		if !entry.extractable {
			return throwError(iso, "key is not extractable")
		}

		switch format {
		case "raw":
			switch k := entry.ecKey.(type) {
			case *ecdh.PublicKey:
				val, _ := v8.NewValue(iso, base64.StdEncoding.EncodeToString(k.Bytes()))
				return val
			case *ecdh.PrivateKey:
				// Export public key bytes for raw format
				val, _ := v8.NewValue(iso, base64.StdEncoding.EncodeToString(k.PublicKey().Bytes()))
				return val
			default:
				return throwError(iso, "exportECDH: not an ECDH key")
			}

		case "jwk":
			return exportECDHJWK(iso, entry)

		default:
			return throwError(iso, fmt.Sprintf("exportECDH: unsupported format %q", format))
		}
	}).GetFunction(ctx))

	// --- X25519 callbacks ---

	// __cryptoGenerateX25519(extractable) -> JSON { privateKeyId, publicKeyId }
	_ = ctx.Global().Set("__cryptoGenerateX25519", v8.NewFunctionTemplate(iso, func(info *v8.FunctionCallbackInfo) *v8.Value {
		args := info.Args()
		extractableVal := false
		if len(args) > 0 {
			extractableVal = args[0].Boolean()
		}

		reqID := getReqIDFromJS(ctx)
		if getRequestState(reqID) == nil {
			val, _ := v8.NewValue(iso, `{"error":"no active request state"}`)
			return val
		}

		privKey, err := ecdh.X25519().GenerateKey(rand.Reader)
		if err != nil {
			val, _ := v8.NewValue(iso, fmt.Sprintf(`{"error":"key generation failed: %s"}`, err.Error()))
			return val
		}

		privID := importCryptoKeyFull(reqID, &cryptoKeyEntry{
			algoName: "X25519", keyType: "private", ecKey: privKey, extractable: extractableVal,
		})
		pubID := importCryptoKeyFull(reqID, &cryptoKeyEntry{
			algoName: "X25519", keyType: "public", ecKey: privKey.PublicKey(), extractable: extractableVal,
		})

		val, _ := v8.NewValue(iso, fmt.Sprintf(`{"privateKeyId":%d,"publicKeyId":%d}`, privID, pubID))
		return val
	}).GetFunction(ctx))

	// __cryptoDeriveX25519(privateKeyID, publicKeyID, lengthBits) -> base64 shared secret
	_ = ctx.Global().Set("__cryptoDeriveX25519", v8.NewFunctionTemplate(iso, func(info *v8.FunctionCallbackInfo) *v8.Value {
		args := info.Args()
		if len(args) < 3 {
			return throwError(iso, "deriveX25519 requires 3 argument(s)")
		}
		privKeyID := args[0].Integer()
		pubKeyID := args[1].Integer()
		lengthBits := int(args[2].Int32())

		reqID := getReqIDFromJS(ctx)
		privEntry := getCryptoKey(reqID, privKeyID)
		if privEntry == nil {
			return throwError(iso, "deriveX25519: private key not found")
		}
		pubEntry := getCryptoKey(reqID, pubKeyID)
		if pubEntry == nil {
			return throwError(iso, "deriveX25519: public key not found")
		}

		privKey, ok := privEntry.ecKey.(*ecdh.PrivateKey)
		if !ok {
			return throwError(iso, "deriveX25519: key is not an X25519 private key")
		}
		pubKey, ok := pubEntry.ecKey.(*ecdh.PublicKey)
		if !ok {
			return throwError(iso, "deriveX25519: key is not an X25519 public key")
		}

		shared, err := privKey.ECDH(pubKey)
		if err != nil {
			return throwError(iso, fmt.Sprintf("deriveX25519: %s", err.Error()))
		}

		// Truncate to requested bit length
		lengthBytes := lengthBits / 8
		if lengthBytes > len(shared) {
			lengthBytes = len(shared)
		}

		val, _ := v8.NewValue(iso, base64.StdEncoding.EncodeToString(shared[:lengthBytes]))
		return val
	}).GetFunction(ctx))

	// __cryptoImportX25519(format, dataB64, keyType, extractable) -> JSON { keyId, keyType }
	_ = ctx.Global().Set("__cryptoImportX25519", v8.NewFunctionTemplate(iso, func(info *v8.FunctionCallbackInfo) *v8.Value {
		args := info.Args()
		if len(args) < 4 {
			return throwError(iso, "importX25519 requires 4 argument(s)")
		}
		format := args[0].String()
		dataStr := args[1].String()
		keyType := args[2].String()
		extractableVal := args[3].Boolean()

		reqID := getReqIDFromJS(ctx)
		if getRequestState(reqID) == nil {
			val, _ := v8.NewValue(iso, `{"error":"no active request state"}`)
			return val
		}

		if format != "raw" {
			val, _ := v8.NewValue(iso, fmt.Sprintf(`{"error":"X25519 only supports raw format, got %q"}`, format))
			return val
		}

		keyData, err := base64.StdEncoding.DecodeString(dataStr)
		if err != nil {
			val, _ := v8.NewValue(iso, `{"error":"invalid base64"}`)
			return val
		}

		if len(keyData) != 32 {
			val, _ := v8.NewValue(iso, fmt.Sprintf(`{"error":"X25519 key must be 32 bytes, got %d"}`, len(keyData)))
			return val
		}

		curve := ecdh.X25519()
		if keyType == "private" {
			privKey, err := curve.NewPrivateKey(keyData)
			if err != nil {
				val, _ := v8.NewValue(iso, fmt.Sprintf(`{"error":"invalid X25519 private key: %s"}`, err.Error()))
				return val
			}
			id := importCryptoKeyFull(reqID, &cryptoKeyEntry{
				algoName: "X25519", keyType: "private", ecKey: privKey, extractable: extractableVal,
			})
			val, _ := v8.NewValue(iso, fmt.Sprintf(`{"keyId":%d,"keyType":"private"}`, id))
			return val
		}

		pubKey, err := curve.NewPublicKey(keyData)
		if err != nil {
			val, _ := v8.NewValue(iso, fmt.Sprintf(`{"error":"invalid X25519 public key: %s"}`, err.Error()))
			return val
		}
		id := importCryptoKeyFull(reqID, &cryptoKeyEntry{
			algoName: "X25519", keyType: "public", ecKey: pubKey, extractable: extractableVal,
		})
		val, _ := v8.NewValue(iso, fmt.Sprintf(`{"keyId":%d,"keyType":"public"}`, id))
		return val
	}).GetFunction(ctx))

	// __cryptoExportX25519(keyID, format) -> base64
	_ = ctx.Global().Set("__cryptoExportX25519", v8.NewFunctionTemplate(iso, func(info *v8.FunctionCallbackInfo) *v8.Value {
		args := info.Args()
		if len(args) < 2 {
			return throwError(iso, "exportX25519 requires 2 argument(s)")
		}
		keyID := args[0].Integer()
		format := args[1].String()

		if format != "raw" {
			return throwError(iso, fmt.Sprintf("exportX25519: only raw format supported, got %q", format))
		}

		reqID := getReqIDFromJS(ctx)
		entry := getCryptoKey(reqID, keyID)
		if entry == nil {
			return throwError(iso, "exportX25519: key not found")
		}
		if !entry.extractable {
			return throwError(iso, "key is not extractable")
		}

		switch k := entry.ecKey.(type) {
		case *ecdh.PublicKey:
			val, _ := v8.NewValue(iso, base64.StdEncoding.EncodeToString(k.Bytes()))
			return val
		case *ecdh.PrivateKey:
			val, _ := v8.NewValue(iso, base64.StdEncoding.EncodeToString(k.Bytes()))
			return val
		default:
			return throwError(iso, "exportX25519: not an X25519 key")
		}
	}).GetFunction(ctx))

	if _, err := ctx.RunScript(cryptoECDHJS, "crypto_ecdh.js"); err != nil {
		return fmt.Errorf("evaluating crypto_ecdh.js: %w", err)
	}
	return nil
}

// importECDHJWK imports an ECDH key from JWK format.
func importECDHJWK(iso *v8.Isolate, reqID uint64, dataStr, curveName string, curve ecdh.Curve, extractable bool) *v8.Value {
	var jwk struct {
		Kty string `json:"kty"`
		Crv string `json:"crv"`
		X   string `json:"x"`
		Y   string `json:"y"`
		D   string `json:"d"`
	}
	if err := decodeJSON([]byte(dataStr), &jwk); err != nil {
		val, _ := v8.NewValue(iso, `{"error":"invalid JWK JSON"}`)
		return val
	}
	if jwk.Kty != "EC" {
		val, _ := v8.NewValue(iso, `{"error":"JWK kty must be EC for ECDH"}`)
		return val
	}
	if jwk.Crv != curveName {
		val, _ := v8.NewValue(iso, fmt.Sprintf(`{"error":"JWK crv %q does not match algorithm curve %q"}`, jwk.Crv, curveName))
		return val
	}

	xBytes, err := base64.RawURLEncoding.DecodeString(jwk.X)
	if err != nil {
		val, _ := v8.NewValue(iso, `{"error":"invalid JWK x value"}`)
		return val
	}
	yBytes, err := base64.RawURLEncoding.DecodeString(jwk.Y)
	if err != nil {
		val, _ := v8.NewValue(iso, `{"error":"invalid JWK y value"}`)
		return val
	}

	if jwk.D != "" {
		// Private key
		dBytes, err := base64.RawURLEncoding.DecodeString(jwk.D)
		if err != nil {
			val, _ := v8.NewValue(iso, `{"error":"invalid JWK d value"}`)
			return val
		}
		privKey, err := curve.NewPrivateKey(dBytes)
		if err != nil {
			val, _ := v8.NewValue(iso, fmt.Sprintf(`{"error":"invalid ECDH private key: %s"}`, err.Error()))
			return val
		}
		id := importCryptoKeyFull(reqID, &cryptoKeyEntry{
			algoName: "ECDH", keyType: "private", namedCurve: curveName, ecKey: privKey, extractable: extractable,
		})
		val, _ := v8.NewValue(iso, fmt.Sprintf(`{"keyId":%d,"keyType":"private"}`, id))
		return val
	}

	// Public key - reconstruct uncompressed point: 0x04 || x || y
	uncompressed := make([]byte, 1+len(xBytes)+len(yBytes))
	uncompressed[0] = 0x04
	copy(uncompressed[1:], xBytes)
	copy(uncompressed[1+len(xBytes):], yBytes)

	pubKey, err := curve.NewPublicKey(uncompressed)
	if err != nil {
		val, _ := v8.NewValue(iso, fmt.Sprintf(`{"error":"invalid ECDH public key: %s"}`, err.Error()))
		return val
	}
	id := importCryptoKeyFull(reqID, &cryptoKeyEntry{
		algoName: "ECDH", keyType: "public", namedCurve: curveName, ecKey: pubKey, extractable: extractable,
	})
	val, _ := v8.NewValue(iso, fmt.Sprintf(`{"keyId":%d,"keyType":"public"}`, id))
	return val
}

// exportECDHJWK exports an ECDH key as JWK.
func exportECDHJWK(iso *v8.Isolate, entry *cryptoKeyEntry) *v8.Value {
	jwk := map[string]string{
		"kty": "EC",
		"crv": entry.namedCurve,
	}

	var pubBytes []byte
	switch k := entry.ecKey.(type) {
	case *ecdh.PublicKey:
		pubBytes = k.Bytes()
	case *ecdh.PrivateKey:
		pubBytes = k.PublicKey().Bytes()
		jwk["d"] = base64.RawURLEncoding.EncodeToString(k.Bytes())
	default:
		return throwError(iso, "exportECDH: not an ECDH key")
	}

	// Uncompressed point: 0x04 || x || y
	if len(pubBytes) < 3 || pubBytes[0] != 0x04 {
		return throwError(iso, "exportECDH: unexpected public key format")
	}
	coordLen := (len(pubBytes) - 1) / 2
	jwk["x"] = base64.RawURLEncoding.EncodeToString(pubBytes[1 : 1+coordLen])
	jwk["y"] = base64.RawURLEncoding.EncodeToString(pubBytes[1+coordLen:])

	data, _ := encodeJSON(jwk)
	val, _ := v8.NewValue(iso, string(data))
	return val
}

// decodeJSON is a small wrapper for json.Unmarshal used in ECDH JWK import.
func decodeJSON(data []byte, v any) error {
	return json.Unmarshal(data, v)
}

// encodeJSON is a small wrapper for json.Marshal used in ECDH JWK export.
func encodeJSON(v any) ([]byte, error) {
	return json.Marshal(v)
}
