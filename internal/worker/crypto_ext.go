package worker

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/hmac"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"

	v8 "github.com/tommie/v8go"
)

// cryptoExtJS patches crypto.subtle with JWK import/export, ECDSA, generateKey,
// and AES-CBC support. Must be evaluated AFTER the base cryptoJS.
const cryptoExtJS = `
(function() {
var subtle = crypto.subtle;
var CK = CryptoKey;

subtle.importKey = async function(format, keyData, algorithm, extractable, usages) {
	var algo = typeof algorithm === 'string' ? { name: algorithm } : algorithm;
	var hashName = algo.hash ? (typeof algo.hash === 'string' ? algo.hash : algo.hash.name) : '';
	var namedCurve = algo.namedCurve || '';
	if (format === 'raw') {
		var b64 = __bufferSourceToB64(keyData);
		var id = __cryptoImportKey(algo.name, hashName, b64, namedCurve);
		return new CK(id, algo, 'secret', extractable, usages);
	} else if (format === 'jwk') {
		var jwkJSON = JSON.stringify(keyData);
		var resultJSON = __cryptoImportKeyJWK(algo.name, hashName, jwkJSON, namedCurve);
		var result = JSON.parse(resultJSON);
		if (result.error) throw new TypeError(result.error);
		return new CK(result.keyId, algo, result.keyType || 'secret', extractable, usages);
	}
	throw new TypeError('importKey: unsupported format "' + format + '"');
};

subtle.exportKey = async function(format, key) {
	if (!key.extractable) throw new DOMException('key is not extractable', 'InvalidAccessError');
	if (format === 'raw') {
		var b64 = __cryptoExportKey(key._id);
		return __b64ToBuffer(b64);
	} else if (format === 'jwk') {
		var algoName = key.algorithm.name || '';
		var hashName = key.algorithm.hash ? (typeof key.algorithm.hash === 'string' ? key.algorithm.hash : key.algorithm.hash.name) : '';
		var namedCurve = key.algorithm.namedCurve || '';
		var resultJSON = __cryptoExportKeyJWK(key._id, algoName, hashName, namedCurve);
		return JSON.parse(resultJSON);
	}
	throw new TypeError('exportKey: unsupported format "' + format + '"');
};

subtle.generateKey = async function(algorithm, extractable, usages) {
	var algo = typeof algorithm === 'string' ? { name: algorithm } : algorithm;
	var hashName = algo.hash ? (typeof algo.hash === 'string' ? algo.hash : algo.hash.name) : '';
	var namedCurve = algo.namedCurve || '';
	var resultJSON = __cryptoGenerateKey(algo.name, hashName, namedCurve);
	var result = JSON.parse(resultJSON);
	if (result.error) throw new TypeError(result.error);
	if (result.privateKeyId !== undefined) {
		return {
			privateKey: new CK(result.privateKeyId, algo, 'private', extractable,
				usages.filter(function(u) { return u === 'sign'; })),
			publicKey: new CK(result.publicKeyId, algo, 'public', extractable,
				usages.filter(function(u) { return u === 'verify'; })),
		};
	}
	return new CK(result.keyId, algo, 'secret', extractable, usages);
};

subtle.sign = async function(algorithm, key, data) {
	var algo = typeof algorithm === 'string' ? { name: algorithm } : algorithm;
	var dataB64 = __bufferSourceToB64(data);
	var hashName = algo.hash ? (typeof algo.hash === 'string' ? algo.hash : algo.hash.name) : '';
	var resultB64 = __cryptoSign(algo.name, key._id, dataB64, hashName);
	return __b64ToBuffer(resultB64);
};

subtle.verify = async function(algorithm, key, signature, data) {
	var algo = typeof algorithm === 'string' ? { name: algorithm } : algorithm;
	var sigB64 = __bufferSourceToB64(signature);
	var dataB64 = __bufferSourceToB64(data);
	var hashName = algo.hash ? (typeof algo.hash === 'string' ? algo.hash : algo.hash.name) : '';
	return __cryptoVerify(algo.name, key._id, sigB64, dataB64, hashName);
};

})();
`

// curveFromName returns the elliptic curve for the given name.
func curveFromName(name string) elliptic.Curve {
	switch name {
	case "P-256":
		return elliptic.P256()
	case "P-384":
		return elliptic.P384()
	default:
		return nil
	}
}

// padBytes left-pads b with zeroes to the given length.
func padBytes(b []byte, length int) []byte {
	if len(b) >= length {
		return b
	}
	padded := make([]byte, length)
	copy(padded[length-len(b):], b)
	return padded
}

// importCryptoKeyFull stores a full cryptoKeyEntry and returns its ID.
func importCryptoKeyFull(reqID uint64, entry *cryptoKeyEntry) int64 {
	state := getRequestState(reqID)
	if state == nil {
		return -1
	}
	state.nextKeyID++
	id := state.nextKeyID
	if state.cryptoKeys == nil {
		state.cryptoKeys = make(map[int64]*cryptoKeyEntry)
	}
	state.cryptoKeys[id] = entry
	return id
}

// setupCryptoExt registers extended crypto Go functions and evaluates the JS
// patches for JWK, ECDSA, generateKey, and AES-CBC. Must run after setupCrypto.
func setupCryptoExt(iso *v8.Isolate, ctx *v8.Context, el *eventLoop) error {
	// Override __cryptoImportKey to accept namedCurve and handle ECDSA raw keys.
	ctx.Global().Set("__cryptoImportKey", v8.NewFunctionTemplate(iso, func(info *v8.FunctionCallbackInfo) *v8.Value {
		args := info.Args()
		if len(args) < 3 {
			return throwError(iso, "importKey requires at least 3 argument(s)")
		}
		algoName := args[0].String()
		hashAlgo := args[1].String()
		dataB64 := args[2].String()
		namedCurve := ""
		if len(args) > 3 {
			namedCurve = args[3].String()
		}

		keyData, err := base64.StdEncoding.DecodeString(dataB64)
		if err != nil {
			return throwError(iso, "importKey: invalid base64")
		}

		reqID := getReqIDFromJS(ctx)

		if normalizeAlgo(algoName) == "ECDSA" && namedCurve != "" {
			curve := curveFromName(namedCurve)
			if curve == nil {
				return throwError(iso, fmt.Sprintf("importKey: unsupported curve %q", namedCurve))
			}
			x, y := elliptic.Unmarshal(curve, keyData)
			if x == nil {
				return throwError(iso, "importKey: invalid EC public key")
			}
			pubKey := &ecdsa.PublicKey{Curve: curve, X: x, Y: y}
			id := importCryptoKeyFull(reqID, &cryptoKeyEntry{
				algoName:   "ECDSA",
				hashAlgo:   hashAlgo,
				keyType:    "public",
				namedCurve: namedCurve,
				ecKey:      pubKey,
			})
			val, _ := v8.NewValue(iso, id)
			return val
		}

		id := importCryptoKey(reqID, hashAlgo, keyData)
		if id < 0 {
			return throwError(iso, "importKey: no active request state")
		}
		val, _ := v8.NewValue(iso, id)
		return val
	}).GetFunction(ctx))

	// __cryptoImportKeyJWK(algoName, hashAlgo, jwkJSON, namedCurve) -> JSON result
	ctx.Global().Set("__cryptoImportKeyJWK", v8.NewFunctionTemplate(iso, func(info *v8.FunctionCallbackInfo) *v8.Value {
		args := info.Args()
		if len(args) < 4 {
			return throwError(iso, "importKeyJWK requires at least 4 argument(s)")
		}
		algoName := args[0].String()
		hashAlgo := args[1].String()
		jwkJSON := args[2].String()
		namedCurve := args[3].String()

		reqID := getReqIDFromJS(ctx)
		if getRequestState(reqID) == nil {
			val, _ := v8.NewValue(iso, `{"error":"no active request state"}`)
			return val
		}

		var jwk map[string]interface{}
		if err := json.Unmarshal([]byte(jwkJSON), &jwk); err != nil {
			val, _ := v8.NewValue(iso, `{"error":"invalid JWK JSON"}`)
			return val
		}

		kty, _ := jwk["kty"].(string)
		switch kty {
		case "oct":
			kB64URL, _ := jwk["k"].(string)
			keyData, err := base64.RawURLEncoding.DecodeString(kB64URL)
			if err != nil {
				val, _ := v8.NewValue(iso, `{"error":"invalid JWK k value"}`)
				return val
			}
			entry := &cryptoKeyEntry{
				data:     keyData,
				hashAlgo: hashAlgo,
				algoName: normalizeAlgo(algoName),
				keyType:  "secret",
			}
			id := importCryptoKeyFull(reqID, entry)
			val, _ := v8.NewValue(iso, fmt.Sprintf(`{"keyId":%d,"keyType":"secret"}`, id))
			return val

		case "EC":
			crv, _ := jwk["crv"].(string)
			if namedCurve == "" {
				namedCurve = crv
			}
			curve := curveFromName(namedCurve)
			if curve == nil {
				val, _ := v8.NewValue(iso, fmt.Sprintf(`{"error":"unsupported curve %q"}`, namedCurve))
				return val
			}
			xB64, _ := jwk["x"].(string)
			yB64, _ := jwk["y"].(string)
			xBytes, err := base64.RawURLEncoding.DecodeString(xB64)
			if err != nil {
				val, _ := v8.NewValue(iso, `{"error":"invalid JWK x value"}`)
				return val
			}
			yBytes, err := base64.RawURLEncoding.DecodeString(yB64)
			if err != nil {
				val, _ := v8.NewValue(iso, `{"error":"invalid JWK y value"}`)
				return val
			}
			x := new(big.Int).SetBytes(xBytes)
			y := new(big.Int).SetBytes(yBytes)
			pubKey := &ecdsa.PublicKey{Curve: curve, X: x, Y: y}

			dB64, hasD := jwk["d"].(string)
			if hasD && dB64 != "" {
				dBytes, err := base64.RawURLEncoding.DecodeString(dB64)
				if err != nil {
					val, _ := v8.NewValue(iso, `{"error":"invalid JWK d value"}`)
					return val
				}
				privKey := &ecdsa.PrivateKey{
					PublicKey: *pubKey,
					D:         new(big.Int).SetBytes(dBytes),
				}
				id := importCryptoKeyFull(reqID, &cryptoKeyEntry{
					algoName: "ECDSA", hashAlgo: hashAlgo, keyType: "private",
					namedCurve: namedCurve, ecKey: privKey,
				})
				val, _ := v8.NewValue(iso, fmt.Sprintf(`{"keyId":%d,"keyType":"private"}`, id))
				return val
			}

			id := importCryptoKeyFull(reqID, &cryptoKeyEntry{
				algoName: "ECDSA", hashAlgo: hashAlgo, keyType: "public",
				namedCurve: namedCurve, ecKey: pubKey,
			})
			val, _ := v8.NewValue(iso, fmt.Sprintf(`{"keyId":%d,"keyType":"public"}`, id))
			return val

		default:
			val, _ := v8.NewValue(iso, fmt.Sprintf(`{"error":"unsupported JWK kty %q"}`, kty))
			return val
		}
	}).GetFunction(ctx))

	// __cryptoExportKeyJWK(keyID, algoName, hashAlgo, namedCurve) -> JSON JWK
	ctx.Global().Set("__cryptoExportKeyJWK", v8.NewFunctionTemplate(iso, func(info *v8.FunctionCallbackInfo) *v8.Value {
		args := info.Args()
		if len(args) < 4 {
			return throwError(iso, "exportKeyJWK requires at least 4 argument(s)")
		}
		keyID := args[0].Integer()
		algoName := args[1].String()
		hashAlgo := args[2].String()

		reqID := getReqIDFromJS(ctx)
		entry := getCryptoKey(reqID, keyID)
		if entry == nil {
			return throwError(iso, "exportKeyJWK: key not found")
		}

		if entry.ecKey != nil {
			jwk := map[string]string{"kty": "EC"}
			curveName := entry.namedCurve
			if curveName == "" {
				curveName = "P-256"
			}
			jwk["crv"] = curveName

			switch k := entry.ecKey.(type) {
			case *ecdsa.PrivateKey:
				byteLen := (k.Curve.Params().BitSize + 7) / 8
				jwk["x"] = base64.RawURLEncoding.EncodeToString(padBytes(k.PublicKey.X.Bytes(), byteLen))
				jwk["y"] = base64.RawURLEncoding.EncodeToString(padBytes(k.PublicKey.Y.Bytes(), byteLen))
				jwk["d"] = base64.RawURLEncoding.EncodeToString(padBytes(k.D.Bytes(), byteLen))
			case *ecdsa.PublicKey:
				byteLen := (k.Curve.Params().BitSize + 7) / 8
				jwk["x"] = base64.RawURLEncoding.EncodeToString(padBytes(k.X.Bytes(), byteLen))
				jwk["y"] = base64.RawURLEncoding.EncodeToString(padBytes(k.Y.Bytes(), byteLen))
			}
			data, _ := json.Marshal(jwk)
			val, _ := v8.NewValue(iso, string(data))
			return val
		}

		jwk := map[string]string{
			"kty": "oct",
			"k":   base64.RawURLEncoding.EncodeToString(entry.data),
		}
		algo := normalizeAlgo(algoName)
		h := normalizeAlgo(hashAlgo)
		switch algo {
		case "HMAC":
			switch h {
			case "SHA-256":
				jwk["alg"] = "HS256"
			case "SHA-384":
				jwk["alg"] = "HS384"
			case "SHA-512":
				jwk["alg"] = "HS512"
			}
		case "AES-GCM":
			switch len(entry.data) {
			case 16:
				jwk["alg"] = "A128GCM"
			case 32:
				jwk["alg"] = "A256GCM"
			}
		}
		data, _ := json.Marshal(jwk)
		val, _ := v8.NewValue(iso, string(data))
		return val
	}).GetFunction(ctx))

	// __cryptoGenerateKey(algoName, hashAlgo, namedCurve) -> JSON result
	ctx.Global().Set("__cryptoGenerateKey", v8.NewFunctionTemplate(iso, func(info *v8.FunctionCallbackInfo) *v8.Value {
		args := info.Args()
		if len(args) < 3 {
			return throwError(iso, "generateKey requires at least 3 argument(s)")
		}
		algoName := args[0].String()
		hashAlgo := args[1].String()
		namedCurve := args[2].String()

		reqID := getReqIDFromJS(ctx)
		if getRequestState(reqID) == nil {
			val, _ := v8.NewValue(iso, `{"error":"no active request state"}`)
			return val
		}

		switch normalizeAlgo(algoName) {
		case "ECDSA":
			curve := curveFromName(namedCurve)
			if curve == nil {
				val, _ := v8.NewValue(iso, fmt.Sprintf(`{"error":"unsupported curve %q"}`, namedCurve))
				return val
			}
			privKey, err := ecdsa.GenerateKey(curve, rand.Reader)
			if err != nil {
				val, _ := v8.NewValue(iso, fmt.Sprintf(`{"error":"key generation failed: %s"}`, err.Error()))
				return val
			}
			privID := importCryptoKeyFull(reqID, &cryptoKeyEntry{
				algoName: "ECDSA", hashAlgo: hashAlgo, keyType: "private",
				namedCurve: namedCurve, ecKey: privKey,
			})
			pubID := importCryptoKeyFull(reqID, &cryptoKeyEntry{
				algoName: "ECDSA", hashAlgo: hashAlgo, keyType: "public",
				namedCurve: namedCurve, ecKey: &privKey.PublicKey,
			})
			val, _ := v8.NewValue(iso, fmt.Sprintf(`{"privateKeyId":%d,"publicKeyId":%d}`, privID, pubID))
			return val

		case "HMAC":
			keyLen := 32
			switch normalizeAlgo(hashAlgo) {
			case "SHA-384":
				keyLen = 48
			case "SHA-512":
				keyLen = 64
			case "SHA-1":
				keyLen = 20
			}
			keyData := make([]byte, keyLen)
			if _, err := rand.Read(keyData); err != nil {
				val, _ := v8.NewValue(iso, fmt.Sprintf(`{"error":"key generation failed: %s"}`, err.Error()))
				return val
			}
			id := importCryptoKey(reqID, hashAlgo, keyData)
			val, _ := v8.NewValue(iso, fmt.Sprintf(`{"keyId":%d}`, id))
			return val

		case "AES-GCM", "AES-CBC":
			keyData := make([]byte, 32)
			if _, err := rand.Read(keyData); err != nil {
				val, _ := v8.NewValue(iso, fmt.Sprintf(`{"error":"key generation failed: %s"}`, err.Error()))
				return val
			}
			id := importCryptoKey(reqID, hashAlgo, keyData)
			val, _ := v8.NewValue(iso, fmt.Sprintf(`{"keyId":%d}`, id))
			return val

		default:
			val, _ := v8.NewValue(iso, fmt.Sprintf(`{"error":"generateKey: unsupported algorithm %q"}`, algoName))
			return val
		}
	}).GetFunction(ctx))

	// Override __cryptoSign to support ECDSA + extra hash arg.
	ctx.Global().Set("__cryptoSign", v8.NewFunctionTemplate(iso, func(info *v8.FunctionCallbackInfo) *v8.Value {
		args := info.Args()
		if len(args) < 3 {
			return throwError(iso, "sign requires at least 3 argument(s)")
		}
		algo := args[0].String()
		keyID := args[1].Integer()
		dataB64 := args[2].String()
		signHashAlgo := ""
		if len(args) > 3 {
			signHashAlgo = args[3].String()
		}

		data, err := base64.StdEncoding.DecodeString(dataB64)
		if err != nil {
			return throwError(iso, "sign: invalid base64")
		}

		reqID := getReqIDFromJS(ctx)
		entry := getCryptoKey(reqID, keyID)
		if entry == nil {
			return throwError(iso, "sign: key not found")
		}

		switch normalizeAlgo(algo) {
		case "HMAC":
			hashFn := hashFuncFromAlgo(entry.hashAlgo)
			if hashFn == nil {
				return throwError(iso, fmt.Sprintf("sign: unsupported HMAC hash %q", entry.hashAlgo))
			}
			mac := hmac.New(hashFn, entry.data)
			mac.Write(data)
			sig := mac.Sum(nil)
			val, _ := v8.NewValue(iso, base64.StdEncoding.EncodeToString(sig))
			return val

		case "ECDSA":
			privKey, ok := entry.ecKey.(*ecdsa.PrivateKey)
			if !ok {
				return throwError(iso, "sign: key is not an ECDSA private key")
			}
			ha := signHashAlgo
			if ha == "" {
				ha = entry.hashAlgo
			}
			hashFn := hashFuncFromAlgo(ha)
			if hashFn == nil {
				return throwError(iso, fmt.Sprintf("sign: unsupported hash %q", ha))
			}
			h := hashFn()
			h.Write(data)
			digest := h.Sum(nil)

			r, s, err := ecdsa.Sign(rand.Reader, privKey, digest)
			if err != nil {
				return throwError(iso, fmt.Sprintf("sign: %s", err.Error()))
			}
			byteLen := (privKey.Curve.Params().BitSize + 7) / 8
			sig := make([]byte, byteLen*2)
			copy(sig[:byteLen], padBytes(r.Bytes(), byteLen))
			copy(sig[byteLen:], padBytes(s.Bytes(), byteLen))
			val, _ := v8.NewValue(iso, base64.StdEncoding.EncodeToString(sig))
			return val

		default:
			return throwError(iso, fmt.Sprintf("sign: unsupported algorithm %q", algo))
		}
	}).GetFunction(ctx))

	// Override __cryptoVerify to support ECDSA + extra hash arg.
	ctx.Global().Set("__cryptoVerify", v8.NewFunctionTemplate(iso, func(info *v8.FunctionCallbackInfo) *v8.Value {
		args := info.Args()
		if len(args) < 4 {
			return throwError(iso, "verify requires at least 4 argument(s)")
		}
		algo := args[0].String()
		keyID := args[1].Integer()
		sigB64 := args[2].String()
		dataB64 := args[3].String()
		verifyHashAlgo := ""
		if len(args) > 4 {
			verifyHashAlgo = args[4].String()
		}

		sig, err := base64.StdEncoding.DecodeString(sigB64)
		if err != nil {
			return throwError(iso, "verify: invalid signature base64")
		}
		data, err := base64.StdEncoding.DecodeString(dataB64)
		if err != nil {
			return throwError(iso, "verify: invalid data base64")
		}

		reqID := getReqIDFromJS(ctx)
		entry := getCryptoKey(reqID, keyID)
		if entry == nil {
			return throwError(iso, "verify: key not found")
		}

		switch normalizeAlgo(algo) {
		case "HMAC":
			hashFn := hashFuncFromAlgo(entry.hashAlgo)
			if hashFn == nil {
				return throwError(iso, fmt.Sprintf("verify: unsupported HMAC hash %q", entry.hashAlgo))
			}
			mac := hmac.New(hashFn, entry.data)
			mac.Write(data)
			expected := mac.Sum(nil)
			val, _ := v8.NewValue(iso, hmac.Equal(sig, expected))
			return val

		case "ECDSA":
			var pubKey *ecdsa.PublicKey
			switch k := entry.ecKey.(type) {
			case *ecdsa.PublicKey:
				pubKey = k
			case *ecdsa.PrivateKey:
				pubKey = &k.PublicKey
			default:
				return throwError(iso, "verify: key is not an ECDSA key")
			}
			ha := verifyHashAlgo
			if ha == "" {
				ha = entry.hashAlgo
			}
			hashFn := hashFuncFromAlgo(ha)
			if hashFn == nil {
				return throwError(iso, fmt.Sprintf("verify: unsupported hash %q", ha))
			}
			h := hashFn()
			h.Write(data)
			digest := h.Sum(nil)

			byteLen := (pubKey.Curve.Params().BitSize + 7) / 8
			if len(sig) != byteLen*2 {
				val, _ := v8.NewValue(iso, false)
				return val
			}
			r := new(big.Int).SetBytes(sig[:byteLen])
			s := new(big.Int).SetBytes(sig[byteLen:])
			val, _ := v8.NewValue(iso, ecdsa.Verify(pubKey, digest, r, s))
			return val

		default:
			return throwError(iso, fmt.Sprintf("verify: unsupported algorithm %q", algo))
		}
	}).GetFunction(ctx))

	// Override __cryptoEncrypt to add AES-CBC.
	ctx.Global().Set("__cryptoEncrypt", v8.NewFunctionTemplate(iso, func(info *v8.FunctionCallbackInfo) *v8.Value {
		args := info.Args()
		if len(args) < 4 {
			return throwError(iso, "encrypt requires at least 4 argument(s)")
		}
		algo := args[0].String()
		keyID := args[1].Integer()
		dataB64 := args[2].String()
		ivB64 := args[3].String()

		data, err := base64.StdEncoding.DecodeString(dataB64)
		if err != nil {
			return throwError(iso, "encrypt: invalid base64 data")
		}
		reqID := getReqIDFromJS(ctx)
		entry := getCryptoKey(reqID, keyID)
		if entry == nil {
			return throwError(iso, "encrypt: key not found")
		}

		switch normalizeAlgo(algo) {
		case "AES-GCM":
			iv, err := base64.StdEncoding.DecodeString(ivB64)
			if err != nil {
				return throwError(iso, "encrypt: invalid IV base64")
			}
			if len(iv) != 12 {
				return throwError(iso, fmt.Sprintf("encrypt: AES-GCM IV must be exactly 12 bytes, got %d", len(iv)))
			}
			block, err := aes.NewCipher(entry.data)
			if err != nil {
				return throwError(iso, fmt.Sprintf("encrypt: %s", err.Error()))
			}
			gcm, err := cipher.NewGCM(block)
			if err != nil {
				return throwError(iso, fmt.Sprintf("encrypt: %s", err.Error()))
			}
			ct := gcm.Seal(nil, iv, data, nil)
			val, _ := v8.NewValue(iso, base64.StdEncoding.EncodeToString(ct))
			return val

		case "AES-CBC":
			iv, err := base64.StdEncoding.DecodeString(ivB64)
			if err != nil {
				return throwError(iso, "encrypt: invalid IV base64")
			}
			if len(iv) != aes.BlockSize {
				return throwError(iso, fmt.Sprintf("encrypt: AES-CBC IV must be exactly %d bytes", aes.BlockSize))
			}
			block, err := aes.NewCipher(entry.data)
			if err != nil {
				return throwError(iso, fmt.Sprintf("encrypt: %s", err.Error()))
			}
			padLen := aes.BlockSize - (len(data) % aes.BlockSize)
			padded := make([]byte, len(data)+padLen)
			copy(padded, data)
			for i := len(data); i < len(padded); i++ {
				padded[i] = byte(padLen)
			}
			mode := cipher.NewCBCEncrypter(block, iv)
			ct := make([]byte, len(padded))
			mode.CryptBlocks(ct, padded)
			val, _ := v8.NewValue(iso, base64.StdEncoding.EncodeToString(ct))
			return val

		default:
			return throwError(iso, fmt.Sprintf("encrypt: unsupported algorithm %q", algo))
		}
	}).GetFunction(ctx))

	// Override __cryptoDecrypt to add AES-CBC.
	ctx.Global().Set("__cryptoDecrypt", v8.NewFunctionTemplate(iso, func(info *v8.FunctionCallbackInfo) *v8.Value {
		args := info.Args()
		if len(args) < 4 {
			return throwError(iso, "decrypt requires at least 4 argument(s)")
		}
		algo := args[0].String()
		keyID := args[1].Integer()
		dataB64 := args[2].String()
		ivB64 := args[3].String()

		data, err := base64.StdEncoding.DecodeString(dataB64)
		if err != nil {
			return throwError(iso, "decrypt: invalid base64 data")
		}
		reqID := getReqIDFromJS(ctx)
		entry := getCryptoKey(reqID, keyID)
		if entry == nil {
			return throwError(iso, "decrypt: key not found")
		}

		switch normalizeAlgo(algo) {
		case "AES-GCM":
			iv, err := base64.StdEncoding.DecodeString(ivB64)
			if err != nil {
				return throwError(iso, "decrypt: invalid IV base64")
			}
			if len(iv) != 12 {
				return throwError(iso, fmt.Sprintf("decrypt: AES-GCM IV must be exactly 12 bytes, got %d", len(iv)))
			}
			block, err := aes.NewCipher(entry.data)
			if err != nil {
				return throwError(iso, fmt.Sprintf("decrypt: %s", err.Error()))
			}
			gcm, err := cipher.NewGCM(block)
			if err != nil {
				return throwError(iso, fmt.Sprintf("decrypt: %s", err.Error()))
			}
			pt, err := gcm.Open(nil, iv, data, nil)
			if err != nil {
				return throwError(iso, fmt.Sprintf("decrypt: %s", err.Error()))
			}
			val, _ := v8.NewValue(iso, base64.StdEncoding.EncodeToString(pt))
			return val

		case "AES-CBC":
			iv, err := base64.StdEncoding.DecodeString(ivB64)
			if err != nil {
				return throwError(iso, "decrypt: invalid IV base64")
			}
			if len(iv) != aes.BlockSize {
				return throwError(iso, fmt.Sprintf("decrypt: AES-CBC IV must be exactly %d bytes", aes.BlockSize))
			}
			if len(data)%aes.BlockSize != 0 {
				return throwError(iso, "decrypt: ciphertext not a multiple of block size")
			}
			block, err := aes.NewCipher(entry.data)
			if err != nil {
				return throwError(iso, fmt.Sprintf("decrypt: %s", err.Error()))
			}
			mode := cipher.NewCBCDecrypter(block, iv)
			pt := make([]byte, len(data))
			mode.CryptBlocks(pt, data)
			if len(pt) == 0 {
				val, _ := v8.NewValue(iso, base64.StdEncoding.EncodeToString(pt))
				return val
			}
			padLen := int(pt[len(pt)-1])
			if padLen < 1 || padLen > aes.BlockSize {
				return throwError(iso, "decrypt: invalid PKCS7 padding")
			}
			for i := 0; i < padLen; i++ {
				if pt[len(pt)-1-i] != byte(padLen) {
					return throwError(iso, "decrypt: invalid PKCS7 padding")
				}
			}
			pt = pt[:len(pt)-padLen]
			val, _ := v8.NewValue(iso, base64.StdEncoding.EncodeToString(pt))
			return val

		default:
			return throwError(iso, fmt.Sprintf("decrypt: unsupported algorithm %q", algo))
		}
	}).GetFunction(ctx))

	// Evaluate the JS patches.
	if _, err := ctx.RunScript(cryptoExtJS, "crypto_ext.js"); err != nil {
		return fmt.Errorf("evaluating crypto_ext.js: %w", err)
	}

	return nil
}
