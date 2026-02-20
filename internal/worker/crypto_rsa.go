package worker

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"

	v8 "github.com/tommie/v8go"
)

// cryptoHashFromAlgo returns the crypto.Hash for the given algorithm name.
func cryptoHashFromAlgo(algo string) crypto.Hash {
	switch normalizeAlgo(algo) {
	case "SHA-1":
		return crypto.SHA1
	case "SHA-256":
		return crypto.SHA256
	case "SHA-384":
		return crypto.SHA384
	case "SHA-512":
		return crypto.SHA512
	default:
		return 0
	}
}

// rsaJWKAlg returns the JWK "alg" value for an RSA algorithm+hash combination.
func rsaJWKAlg(algoName, hashAlgo string) string {
	h := normalizeAlgo(hashAlgo)
	switch normalizeAlgo(algoName) {
	case "RSASSA-PKCS1-v1_5":
		switch h {
		case "SHA-1":
			return "RS1"
		case "SHA-256":
			return "RS256"
		case "SHA-384":
			return "RS384"
		case "SHA-512":
			return "RS512"
		}
	case "RSA-PSS":
		switch h {
		case "SHA-256":
			return "PS256"
		case "SHA-384":
			return "PS384"
		case "SHA-512":
			return "PS512"
		}
	case "RSA-OAEP":
		switch h {
		case "SHA-1":
			return "RSA-OAEP"
		case "SHA-256":
			return "RSA-OAEP-256"
		case "SHA-384":
			return "RSA-OAEP-384"
		case "SHA-512":
			return "RSA-OAEP-512"
		}
	}
	return ""
}

// cryptoRSAJS patches crypto.subtle with RSA support using chain-of-responsibility.
const cryptoRSAJS = `
(function() {
var subtle = crypto.subtle;
var CK = CryptoKey;
var _prevSign = subtle.sign;
var _prevVerify = subtle.verify;
var _prevEncrypt = subtle.encrypt;
var _prevDecrypt = subtle.decrypt;
var _prevImportKey = subtle.importKey;
var _prevExportKey = subtle.exportKey;
var _prevGenerateKey = subtle.generateKey;

function isRSA(name) {
	return name === 'RSASSA-PKCS1-v1_5' || name === 'RSA-PSS' || name === 'RSA-OAEP';
}

subtle.sign = async function(algorithm, key, data) {
	var algo = typeof algorithm === 'string' ? { name: algorithm } : algorithm;
	if (algo.name === 'RSASSA-PKCS1-v1_5' || algo.name === 'RSA-PSS') {
		var hashName = key.algorithm.hash ? (typeof key.algorithm.hash === 'string' ? key.algorithm.hash : key.algorithm.hash.name) : '';
		var saltLength = algo.saltLength || 0;
		var resultB64 = __cryptoSignRSA(algo.name, key._id, __bufferSourceToB64(data), hashName, saltLength);
		return __b64ToBuffer(resultB64);
	}
	return _prevSign.call(this, algorithm, key, data);
};

subtle.verify = async function(algorithm, key, signature, data) {
	var algo = typeof algorithm === 'string' ? { name: algorithm } : algorithm;
	if (algo.name === 'RSASSA-PKCS1-v1_5' || algo.name === 'RSA-PSS') {
		var hashName = key.algorithm.hash ? (typeof key.algorithm.hash === 'string' ? key.algorithm.hash : key.algorithm.hash.name) : '';
		var saltLength = algo.saltLength || 0;
		return __cryptoVerifyRSA(algo.name, key._id, __bufferSourceToB64(signature), __bufferSourceToB64(data), hashName, saltLength);
	}
	return _prevVerify.call(this, algorithm, key, signature, data);
};

subtle.encrypt = async function(algorithm, key, data) {
	var algo = typeof algorithm === 'string' ? { name: algorithm } : algorithm;
	if (algo.name === 'RSA-OAEP') {
		var labelB64 = algo.label ? __bufferSourceToB64(algo.label) : '';
		var resultB64 = __cryptoEncryptRSA(key._id, __bufferSourceToB64(data), labelB64);
		return __b64ToBuffer(resultB64);
	}
	return _prevEncrypt.call(this, algorithm, key, data);
};

subtle.decrypt = async function(algorithm, key, data) {
	var algo = typeof algorithm === 'string' ? { name: algorithm } : algorithm;
	if (algo.name === 'RSA-OAEP') {
		var labelB64 = algo.label ? __bufferSourceToB64(algo.label) : '';
		var resultB64 = __cryptoDecryptRSA(key._id, __bufferSourceToB64(data), labelB64);
		return __b64ToBuffer(resultB64);
	}
	return _prevDecrypt.call(this, algorithm, key, data);
};

subtle.importKey = async function(format, keyData, algorithm, extractable, usages) {
	var algo = typeof algorithm === 'string' ? { name: algorithm } : algorithm;
	if (isRSA(algo.name)) {
		var hashName = algo.hash ? (typeof algo.hash === 'string' ? algo.hash : algo.hash.name) : '';
		var dataStr;
		if (format === 'jwk') {
			dataStr = JSON.stringify(keyData);
		} else {
			dataStr = __bufferSourceToB64(keyData);
		}
		var resultJSON = __cryptoImportKeyRSA(format, dataStr, algo.name, hashName);
		var result = JSON.parse(resultJSON);
		if (result.error) throw new TypeError(result.error);
		var keyAlgo = { name: algo.name, hash: { name: hashName } };
		if (result.modulusLength) keyAlgo.modulusLength = result.modulusLength;
		if (result.publicExponent) {
			var peBytes = [];
			var pe = result.publicExponent;
			if (pe > 0) {
				while (pe > 0) { peBytes.unshift(pe & 0xFF); pe = pe >>> 8; }
			}
			keyAlgo.publicExponent = new Uint8Array(peBytes);
		}
		return new CK(result.keyId, keyAlgo, result.keyType, extractable, usages);
	}
	return _prevImportKey.call(this, format, keyData, algorithm, extractable, usages);
};

subtle.exportKey = async function(format, key) {
	if (isRSA(key.algorithm.name)) {
		if (!key.extractable) throw new DOMException('key is not extractable', 'InvalidAccessError');
		var hashName = key.algorithm.hash ? (typeof key.algorithm.hash === 'string' ? key.algorithm.hash : key.algorithm.hash.name) : '';
		var resultStr = __cryptoExportKeyRSA(key._id, format, key.algorithm.name, hashName);
		if (format === 'jwk') {
			return JSON.parse(resultStr);
		}
		return __b64ToBuffer(resultStr);
	}
	return _prevExportKey.call(this, format, key);
};

subtle.generateKey = async function(algorithm, extractable, usages) {
	var algo = typeof algorithm === 'string' ? { name: algorithm } : algorithm;
	if (isRSA(algo.name)) {
		var hashName = algo.hash ? (typeof algo.hash === 'string' ? algo.hash : algo.hash.name) : '';
		var modulusLength = algo.modulusLength || 2048;
		var pubExp = 65537;
		if (algo.publicExponent) {
			var pe = algo.publicExponent;
			pubExp = 0;
			for (var i = 0; i < pe.length; i++) {
				pubExp = (pubExp << 8) | pe[i];
			}
		}
		var resultJSON = __cryptoGenerateKeyRSA(algo.name, modulusLength, hashName, pubExp);
		var result = JSON.parse(resultJSON);
		if (result.error) throw new TypeError(result.error);
		var keyAlgo = { name: algo.name, hash: { name: hashName }, modulusLength: modulusLength };
		if (algo.publicExponent) keyAlgo.publicExponent = algo.publicExponent;
		else keyAlgo.publicExponent = new Uint8Array([1, 0, 1]);
		return {
			privateKey: new CK(result.privateKeyId, keyAlgo, 'private', extractable,
				usages.filter(function(u) { return u === 'sign' || u === 'decrypt'; })),
			publicKey: new CK(result.publicKeyId, keyAlgo, 'public', extractable,
				usages.filter(function(u) { return u === 'verify' || u === 'encrypt'; })),
		};
	}
	return _prevGenerateKey.call(this, algorithm, extractable, usages);
};

})();
`

// setupCryptoRSA registers RSA Go functions and evaluates the JS patches.
// Must run after setupCryptoExt.
func setupCryptoRSA(iso *v8.Isolate, ctx *v8.Context, _ *eventLoop) error {
	// __cryptoSignRSA(algoName, keyID, dataB64, hashAlgo, saltLength) -> sigB64
	_ = ctx.Global().Set("__cryptoSignRSA", v8.NewFunctionTemplate(iso, func(info *v8.FunctionCallbackInfo) *v8.Value {
		args := info.Args()
		if len(args) < 5 {
			return throwError(iso, "signRSA requires 5 argument(s)")
		}
		algoName := args[0].String()
		keyID := args[1].Integer()
		dataB64 := args[2].String()
		hashAlgo := args[3].String()
		saltLength := int(args[4].Int32())

		data, err := base64.StdEncoding.DecodeString(dataB64)
		if err != nil {
			return throwError(iso, "signRSA: invalid base64")
		}

		reqID := getReqIDFromJS(ctx)
		entry := getCryptoKey(reqID, keyID)
		if entry == nil {
			return throwError(iso, "signRSA: key not found")
		}

		privKey, ok := entry.ecKey.(*rsa.PrivateKey)
		if !ok {
			return throwError(iso, "signRSA: key is not an RSA private key")
		}

		ch := cryptoHashFromAlgo(hashAlgo)
		if ch == 0 {
			return throwError(iso, fmt.Sprintf("signRSA: unsupported hash %q", hashAlgo))
		}
		hashFn := hashFuncFromAlgo(hashAlgo)
		h := hashFn()
		h.Write(data)
		digest := h.Sum(nil)

		var sig []byte
		switch normalizeAlgo(algoName) {
		case "RSASSA-PKCS1-v1_5":
			sig, err = rsa.SignPKCS1v15(rand.Reader, privKey, ch, digest)
		case "RSA-PSS":
			opts := &rsa.PSSOptions{SaltLength: saltLength}
			if saltLength == 0 {
				opts.SaltLength = rsa.PSSSaltLengthEqualsHash
			}
			sig, err = rsa.SignPSS(rand.Reader, privKey, ch, digest, opts)
		default:
			return throwError(iso, fmt.Sprintf("signRSA: unsupported algorithm %q", algoName))
		}

		if err != nil {
			return throwError(iso, fmt.Sprintf("signRSA: %s", err.Error()))
		}
		val, _ := v8.NewValue(iso, base64.StdEncoding.EncodeToString(sig))
		return val
	}).GetFunction(ctx))

	// __cryptoVerifyRSA(algoName, keyID, sigB64, dataB64, hashAlgo, saltLength) -> bool
	_ = ctx.Global().Set("__cryptoVerifyRSA", v8.NewFunctionTemplate(iso, func(info *v8.FunctionCallbackInfo) *v8.Value {
		args := info.Args()
		if len(args) < 6 {
			return throwError(iso, "verifyRSA requires 6 argument(s)")
		}
		algoName := args[0].String()
		keyID := args[1].Integer()
		sigB64 := args[2].String()
		dataB64 := args[3].String()
		hashAlgo := args[4].String()
		saltLength := int(args[5].Int32())

		sig, err := base64.StdEncoding.DecodeString(sigB64)
		if err != nil {
			return throwError(iso, "verifyRSA: invalid signature base64")
		}
		data, err := base64.StdEncoding.DecodeString(dataB64)
		if err != nil {
			return throwError(iso, "verifyRSA: invalid data base64")
		}

		reqID := getReqIDFromJS(ctx)
		entry := getCryptoKey(reqID, keyID)
		if entry == nil {
			return throwError(iso, "verifyRSA: key not found")
		}

		var pubKey *rsa.PublicKey
		switch k := entry.ecKey.(type) {
		case *rsa.PublicKey:
			pubKey = k
		case *rsa.PrivateKey:
			pubKey = &k.PublicKey
		default:
			return throwError(iso, "verifyRSA: key is not an RSA key")
		}

		ch := cryptoHashFromAlgo(hashAlgo)
		if ch == 0 {
			return throwError(iso, fmt.Sprintf("verifyRSA: unsupported hash %q", hashAlgo))
		}
		hashFn := hashFuncFromAlgo(hashAlgo)
		h := hashFn()
		h.Write(data)
		digest := h.Sum(nil)

		switch normalizeAlgo(algoName) {
		case "RSASSA-PKCS1-v1_5":
			err = rsa.VerifyPKCS1v15(pubKey, ch, digest, sig)
		case "RSA-PSS":
			opts := &rsa.PSSOptions{SaltLength: saltLength}
			if saltLength == 0 {
				opts.SaltLength = rsa.PSSSaltLengthEqualsHash
			}
			err = rsa.VerifyPSS(pubKey, ch, digest, sig, opts)
		default:
			return throwError(iso, fmt.Sprintf("verifyRSA: unsupported algorithm %q", algoName))
		}

		val, _ := v8.NewValue(iso, err == nil)
		return val
	}).GetFunction(ctx))

	// __cryptoEncryptRSA(keyID, dataB64, labelB64) -> ctB64
	_ = ctx.Global().Set("__cryptoEncryptRSA", v8.NewFunctionTemplate(iso, func(info *v8.FunctionCallbackInfo) *v8.Value {
		args := info.Args()
		if len(args) < 3 {
			return throwError(iso, "encryptRSA requires 3 argument(s)")
		}
		keyID := args[0].Integer()
		dataB64 := args[1].String()
		labelB64 := args[2].String()

		data, err := base64.StdEncoding.DecodeString(dataB64)
		if err != nil {
			return throwError(iso, "encryptRSA: invalid base64")
		}

		var label []byte
		if labelB64 != "" {
			label, err = base64.StdEncoding.DecodeString(labelB64)
			if err != nil {
				return throwError(iso, "encryptRSA: invalid label base64")
			}
		}

		reqID := getReqIDFromJS(ctx)
		entry := getCryptoKey(reqID, keyID)
		if entry == nil {
			return throwError(iso, "encryptRSA: key not found")
		}

		var pubKey *rsa.PublicKey
		switch k := entry.ecKey.(type) {
		case *rsa.PublicKey:
			pubKey = k
		case *rsa.PrivateKey:
			pubKey = &k.PublicKey
		default:
			return throwError(iso, "encryptRSA: key is not an RSA key")
		}

		hashAlgo := entry.hashAlgo
		if hashAlgo == "" {
			hashAlgo = "SHA-256"
		}
		hashFn := hashFuncFromAlgo(hashAlgo)
		if hashFn == nil {
			return throwError(iso, fmt.Sprintf("encryptRSA: unsupported hash %q", hashAlgo))
		}

		ct, err := rsa.EncryptOAEP(hashFn(), rand.Reader, pubKey, data, label)
		if err != nil {
			return throwError(iso, fmt.Sprintf("encryptRSA: %s", err.Error()))
		}
		val, _ := v8.NewValue(iso, base64.StdEncoding.EncodeToString(ct))
		return val
	}).GetFunction(ctx))

	// __cryptoDecryptRSA(keyID, ctB64, labelB64) -> ptB64
	_ = ctx.Global().Set("__cryptoDecryptRSA", v8.NewFunctionTemplate(iso, func(info *v8.FunctionCallbackInfo) *v8.Value {
		args := info.Args()
		if len(args) < 3 {
			return throwError(iso, "decryptRSA requires 3 argument(s)")
		}
		keyID := args[0].Integer()
		ctB64 := args[1].String()
		labelB64 := args[2].String()

		ct, err := base64.StdEncoding.DecodeString(ctB64)
		if err != nil {
			return throwError(iso, "decryptRSA: invalid base64")
		}

		var label []byte
		if labelB64 != "" {
			label, err = base64.StdEncoding.DecodeString(labelB64)
			if err != nil {
				return throwError(iso, "decryptRSA: invalid label base64")
			}
		}

		reqID := getReqIDFromJS(ctx)
		entry := getCryptoKey(reqID, keyID)
		if entry == nil {
			return throwError(iso, "decryptRSA: key not found")
		}

		privKey, ok := entry.ecKey.(*rsa.PrivateKey)
		if !ok {
			return throwError(iso, "decryptRSA: key is not an RSA private key")
		}

		hashAlgo := entry.hashAlgo
		if hashAlgo == "" {
			hashAlgo = "SHA-256"
		}
		hashFn := hashFuncFromAlgo(hashAlgo)
		if hashFn == nil {
			return throwError(iso, fmt.Sprintf("decryptRSA: unsupported hash %q", hashAlgo))
		}

		pt, err := rsa.DecryptOAEP(hashFn(), rand.Reader, privKey, ct, label)
		if err != nil {
			return throwError(iso, fmt.Sprintf("decryptRSA: %s", err.Error()))
		}
		val, _ := v8.NewValue(iso, base64.StdEncoding.EncodeToString(pt))
		return val
	}).GetFunction(ctx))

	// __cryptoGenerateKeyRSA(algoName, modulusLength, hashAlgo, publicExponent) -> JSON
	_ = ctx.Global().Set("__cryptoGenerateKeyRSA", v8.NewFunctionTemplate(iso, func(info *v8.FunctionCallbackInfo) *v8.Value {
		args := info.Args()
		if len(args) < 4 {
			return throwError(iso, "generateKeyRSA requires 4 argument(s)")
		}
		algoName := args[0].String()
		modulusLength := int(args[1].Int32())
		hashAlgo := args[2].String()
		pubExp := int(args[3].Int32())

		reqID := getReqIDFromJS(ctx)
		if getRequestState(reqID) == nil {
			val, _ := v8.NewValue(iso, `{"error":"no active request state"}`)
			return val
		}

		if pubExp == 0 {
			pubExp = 65537
		}

		privKey, err := rsa.GenerateKey(rand.Reader, modulusLength)
		if err != nil {
			val, _ := v8.NewValue(iso, fmt.Sprintf(`{"error":"key generation failed: %s"}`, err.Error()))
			return val
		}
		privKey.PublicKey.E = pubExp

		privID := importCryptoKeyFull(reqID, &cryptoKeyEntry{
			algoName: normalizeAlgo(algoName), hashAlgo: hashAlgo,
			keyType: "private", ecKey: privKey,
		})
		pubID := importCryptoKeyFull(reqID, &cryptoKeyEntry{
			algoName: normalizeAlgo(algoName), hashAlgo: hashAlgo,
			keyType: "public", ecKey: &privKey.PublicKey,
		})

		val, _ := v8.NewValue(iso, fmt.Sprintf(`{"privateKeyId":%d,"publicKeyId":%d}`, privID, pubID))
		return val
	}).GetFunction(ctx))

	// __cryptoImportKeyRSA(format, dataStr, algoName, hashAlgo) -> JSON
	_ = ctx.Global().Set("__cryptoImportKeyRSA", v8.NewFunctionTemplate(iso, func(info *v8.FunctionCallbackInfo) *v8.Value {
		args := info.Args()
		if len(args) < 4 {
			return throwError(iso, "importKeyRSA requires 4 argument(s)")
		}
		format := args[0].String()
		dataStr := args[1].String()
		algoName := args[2].String()
		hashAlgo := args[3].String()

		reqID := getReqIDFromJS(ctx)
		if getRequestState(reqID) == nil {
			val, _ := v8.NewValue(iso, `{"error":"no active request state"}`)
			return val
		}

		switch format {
		case "jwk":
			return importRSAJWK(iso, reqID, dataStr, algoName, hashAlgo)
		case "spki":
			return importRSASPKI(iso, reqID, dataStr, algoName, hashAlgo)
		case "pkcs8":
			return importRSAPKCS8(iso, reqID, dataStr, algoName, hashAlgo)
		default:
			val, _ := v8.NewValue(iso, fmt.Sprintf(`{"error":"unsupported format %q for RSA"}`, format))
			return val
		}
	}).GetFunction(ctx))

	// __cryptoExportKeyRSA(keyID, format, algoName, hashAlgo) -> base64 or JSON string
	_ = ctx.Global().Set("__cryptoExportKeyRSA", v8.NewFunctionTemplate(iso, func(info *v8.FunctionCallbackInfo) *v8.Value {
		args := info.Args()
		if len(args) < 4 {
			return throwError(iso, "exportKeyRSA requires 4 argument(s)")
		}
		keyID := args[0].Integer()
		format := args[1].String()
		algoName := args[2].String()
		hashAlgo := args[3].String()

		reqID := getReqIDFromJS(ctx)
		entry := getCryptoKey(reqID, keyID)
		if entry == nil {
			return throwError(iso, "exportKeyRSA: key not found")
		}

		switch format {
		case "jwk":
			return exportRSAJWK(iso, entry, algoName, hashAlgo)
		case "spki":
			return exportRSASPKI(iso, entry)
		case "pkcs8":
			return exportRSAPKCS8(iso, entry)
		default:
			return throwError(iso, fmt.Sprintf("exportKeyRSA: unsupported format %q", format))
		}
	}).GetFunction(ctx))

	if _, err := ctx.RunScript(cryptoRSAJS, "crypto_rsa.js"); err != nil {
		return fmt.Errorf("evaluating crypto_rsa.js: %w", err)
	}
	return nil
}

// importRSAJWK imports an RSA key from JWK format.
func importRSAJWK(iso *v8.Isolate, reqID uint64, jwkJSON, algoName, hashAlgo string) *v8.Value {
	var jwk map[string]interface{}
	if err := json.Unmarshal([]byte(jwkJSON), &jwk); err != nil {
		val, _ := v8.NewValue(iso, `{"error":"invalid JWK JSON"}`)
		return val
	}

	kty, _ := jwk["kty"].(string)
	if kty != "RSA" {
		val, _ := v8.NewValue(iso, `{"error":"JWK kty must be RSA"}`)
		return val
	}

	// Decode n and e (required for both public and private keys)
	nB64, _ := jwk["n"].(string)
	eB64, _ := jwk["e"].(string)
	if nB64 == "" || eB64 == "" {
		val, _ := v8.NewValue(iso, `{"error":"JWK missing required n or e field"}`)
		return val
	}
	nBytes, err := base64.RawURLEncoding.DecodeString(nB64)
	if err != nil {
		val, _ := v8.NewValue(iso, `{"error":"invalid JWK n value"}`)
		return val
	}
	eBytes, err := base64.RawURLEncoding.DecodeString(eB64)
	if err != nil {
		val, _ := v8.NewValue(iso, `{"error":"invalid JWK e value"}`)
		return val
	}

	n := new(big.Int).SetBytes(nBytes)
	e := int(new(big.Int).SetBytes(eBytes).Int64())

	pubKey := &rsa.PublicKey{N: n, E: e}

	// Check if private key fields are present
	dB64, hasD := jwk["d"].(string)
	if hasD && dB64 != "" {
		dBytes, err := base64.RawURLEncoding.DecodeString(dB64)
		if err != nil {
			val, _ := v8.NewValue(iso, `{"error":"invalid JWK d value"}`)
			return val
		}
		privKey := &rsa.PrivateKey{
			PublicKey: *pubKey,
			D:         new(big.Int).SetBytes(dBytes),
		}

		// Optional CRT values
		if pB64, ok := jwk["p"].(string); ok {
			pBytes, _ := base64.RawURLEncoding.DecodeString(pB64)
			qB64, _ := jwk["q"].(string)
			qBytes, _ := base64.RawURLEncoding.DecodeString(qB64)
			if len(pBytes) > 0 && len(qBytes) > 0 {
				privKey.Primes = []*big.Int{
					new(big.Int).SetBytes(pBytes),
					new(big.Int).SetBytes(qBytes),
				}
			}
		}

		privKey.Precompute()

		id := importCryptoKeyFull(reqID, &cryptoKeyEntry{
			algoName: normalizeAlgo(algoName), hashAlgo: hashAlgo,
			keyType: "private", ecKey: privKey,
		})
		val, _ := v8.NewValue(iso, fmt.Sprintf(
			`{"keyId":%d,"keyType":"private","modulusLength":%d,"publicExponent":%d}`,
			id, pubKey.N.BitLen(), e))
		return val
	}

	id := importCryptoKeyFull(reqID, &cryptoKeyEntry{
		algoName: normalizeAlgo(algoName), hashAlgo: hashAlgo,
		keyType: "public", ecKey: pubKey,
	})
	val, _ := v8.NewValue(iso, fmt.Sprintf(
		`{"keyId":%d,"keyType":"public","modulusLength":%d,"publicExponent":%d}`,
		id, pubKey.N.BitLen(), e))
	return val
}

// importRSASPKI imports an RSA public key from SPKI (DER) format.
func importRSASPKI(iso *v8.Isolate, reqID uint64, dataB64, algoName, hashAlgo string) *v8.Value {
	derBytes, err := base64.StdEncoding.DecodeString(dataB64)
	if err != nil {
		val, _ := v8.NewValue(iso, `{"error":"invalid base64"}`)
		return val
	}

	pub, err := x509.ParsePKIXPublicKey(derBytes)
	if err != nil {
		val, _ := v8.NewValue(iso, fmt.Sprintf(`{"error":"invalid SPKI: %s"}`, err.Error()))
		return val
	}

	rsaPub, ok := pub.(*rsa.PublicKey)
	if !ok {
		val, _ := v8.NewValue(iso, `{"error":"SPKI key is not RSA"}`)
		return val
	}

	id := importCryptoKeyFull(reqID, &cryptoKeyEntry{
		algoName: normalizeAlgo(algoName), hashAlgo: hashAlgo,
		keyType: "public", ecKey: rsaPub,
	})
	val, _ := v8.NewValue(iso, fmt.Sprintf(
		`{"keyId":%d,"keyType":"public","modulusLength":%d,"publicExponent":%d}`,
		id, rsaPub.N.BitLen(), rsaPub.E))
	return val
}

// importRSAPKCS8 imports an RSA private key from PKCS#8 (DER) format.
func importRSAPKCS8(iso *v8.Isolate, reqID uint64, dataB64, algoName, hashAlgo string) *v8.Value {
	derBytes, err := base64.StdEncoding.DecodeString(dataB64)
	if err != nil {
		val, _ := v8.NewValue(iso, `{"error":"invalid base64"}`)
		return val
	}

	key, err := x509.ParsePKCS8PrivateKey(derBytes)
	if err != nil {
		val, _ := v8.NewValue(iso, fmt.Sprintf(`{"error":"invalid PKCS8: %s"}`, err.Error()))
		return val
	}

	rsaKey, ok := key.(*rsa.PrivateKey)
	if !ok {
		val, _ := v8.NewValue(iso, `{"error":"PKCS8 key is not RSA"}`)
		return val
	}

	id := importCryptoKeyFull(reqID, &cryptoKeyEntry{
		algoName: normalizeAlgo(algoName), hashAlgo: hashAlgo,
		keyType: "private", ecKey: rsaKey,
	})
	val, _ := v8.NewValue(iso, fmt.Sprintf(
		`{"keyId":%d,"keyType":"private","modulusLength":%d,"publicExponent":%d}`,
		id, rsaKey.PublicKey.N.BitLen(), rsaKey.PublicKey.E))
	return val
}

// exportRSAJWK exports an RSA key to JWK format.
func exportRSAJWK(iso *v8.Isolate, entry *cryptoKeyEntry, algoName, hashAlgo string) *v8.Value {
	jwk := map[string]interface{}{"kty": "RSA"}

	alg := rsaJWKAlg(algoName, hashAlgo)
	if alg != "" {
		jwk["alg"] = alg
	}

	switch k := entry.ecKey.(type) {
	case *rsa.PublicKey:
		jwk["n"] = base64.RawURLEncoding.EncodeToString(k.N.Bytes())
		jwk["e"] = base64.RawURLEncoding.EncodeToString(big.NewInt(int64(k.E)).Bytes())
		jwk["key_ops"] = []string{"verify", "encrypt"}
	case *rsa.PrivateKey:
		jwk["n"] = base64.RawURLEncoding.EncodeToString(k.PublicKey.N.Bytes())
		jwk["e"] = base64.RawURLEncoding.EncodeToString(big.NewInt(int64(k.PublicKey.E)).Bytes())
		jwk["d"] = base64.RawURLEncoding.EncodeToString(k.D.Bytes())
		if len(k.Primes) >= 2 {
			jwk["p"] = base64.RawURLEncoding.EncodeToString(k.Primes[0].Bytes())
			jwk["q"] = base64.RawURLEncoding.EncodeToString(k.Primes[1].Bytes())
			if k.Precomputed.Dp != nil {
				jwk["dp"] = base64.RawURLEncoding.EncodeToString(k.Precomputed.Dp.Bytes())
				jwk["dq"] = base64.RawURLEncoding.EncodeToString(k.Precomputed.Dq.Bytes())
				jwk["qi"] = base64.RawURLEncoding.EncodeToString(k.Precomputed.Qinv.Bytes())
			}
		}
		jwk["key_ops"] = []string{"sign", "decrypt"}
	default:
		return throwError(iso, "exportKeyRSA: not an RSA key")
	}

	data, _ := json.Marshal(jwk)
	val, _ := v8.NewValue(iso, string(data))
	return val
}

// exportRSASPKI exports an RSA public key to SPKI (DER) format.
func exportRSASPKI(iso *v8.Isolate, entry *cryptoKeyEntry) *v8.Value {
	var pubKey *rsa.PublicKey
	switch k := entry.ecKey.(type) {
	case *rsa.PublicKey:
		pubKey = k
	case *rsa.PrivateKey:
		pubKey = &k.PublicKey
	default:
		return throwError(iso, "exportKeyRSA: not an RSA key")
	}

	derBytes, err := x509.MarshalPKIXPublicKey(pubKey)
	if err != nil {
		return throwError(iso, fmt.Sprintf("exportKeyRSA: %s", err.Error()))
	}
	val, _ := v8.NewValue(iso, base64.StdEncoding.EncodeToString(derBytes))
	return val
}

// exportRSAPKCS8 exports an RSA private key to PKCS#8 (DER) format.
func exportRSAPKCS8(iso *v8.Isolate, entry *cryptoKeyEntry) *v8.Value {
	privKey, ok := entry.ecKey.(*rsa.PrivateKey)
	if !ok {
		return throwError(iso, "exportKeyRSA: not an RSA private key")
	}

	derBytes, err := x509.MarshalPKCS8PrivateKey(privKey)
	if err != nil {
		return throwError(iso, fmt.Sprintf("exportKeyRSA: %s", err.Error()))
	}
	val, _ := v8.NewValue(iso, base64.StdEncoding.EncodeToString(derBytes))
	return val
}
