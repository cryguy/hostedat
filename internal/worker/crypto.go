package worker

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"fmt"
	"hash"

	v8 "github.com/tommie/v8go"
)

// cryptoJS wires up the global crypto object with getRandomValues and randomUUID
// backed by Go helper functions, plus a crypto.subtle proxy that delegates
// digest/sign/verify/encrypt/decrypt/importKey/exportKey to Go-backed functions.
//
// Key material is scoped per-request via __requestID  Eno global key store.
const cryptoJS = `
(function() {
	// Pure-JS base64 encode/decode for the crypto internals.
	const _b64e = 'ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/';
	const _b64d = new Uint8Array(128);
	for (let i = 0; i < _b64e.length; i++) _b64d[_b64e.charCodeAt(i)] = i;

	const crypto = {};

	crypto.getRandomValues = function(typedArray) {
		if (!typedArray || typeof typedArray.length !== 'number') {
			throw new TypeError('getRandomValues requires a TypedArray');
		}
		const b64 = __cryptoGetRandomBytes(typedArray.length);
		let j = 0;
		for (let i = 0; i < b64.length; i += 4) {
			const a = _b64d[b64.charCodeAt(i)];
			const b = _b64d[b64.charCodeAt(i + 1)];
			const c = _b64d[b64.charCodeAt(i + 2)];
			const d = _b64d[b64.charCodeAt(i + 3)];
			if (j < typedArray.length) typedArray[j++] = (a << 2) | (b >> 4);
			if (j < typedArray.length) typedArray[j++] = ((b & 15) << 4) | (c >> 2);
			if (j < typedArray.length) typedArray[j++] = ((c & 3) << 6) | d;
		}
		return typedArray;
	};

	crypto.randomUUID = function() {
		return __cryptoRandomUUID();
	};

	// --- crypto.subtle ---
	const subtle = {};

	subtle.digest = async function(algorithm, data) {
		const algo = typeof algorithm === 'string' ? algorithm : algorithm.name;
		const b64 = __bufferSourceToB64(data);
		const resultB64 = __cryptoDigest(algo, b64);
		return __b64ToBuffer(resultB64);
	};

	class CryptoKey {
		constructor(id, algorithm, type, extractable, usages) {
			this._id = id;
			this.algorithm = algorithm;
			this.type = type;
			this.extractable = extractable;
			this.usages = usages;
		}
	}

	subtle.importKey = async function(format, keyData, algorithm, extractable, usages) {
		const algo = typeof algorithm === 'string' ? { name: algorithm } : algorithm;
		if (format !== 'raw') {
			throw new TypeError('importKey: only raw format is supported');
		}
		const b64 = __bufferSourceToB64(keyData);
		const hashName = algo.hash ? (typeof algo.hash === 'string' ? algo.hash : algo.hash.name) : '';
		const id = __cryptoImportKey(algo.name, hashName, b64);
		const keyType = 'secret';
		return new CryptoKey(id, algo, keyType, extractable, usages);
	};

	subtle.exportKey = async function(format, key) {
		if (format !== 'raw') throw new TypeError('exportKey: only raw format is supported');
		if (!key.extractable) throw new DOMException('key is not extractable', 'InvalidAccessError');
		const b64 = __cryptoExportKey(key._id);
		return __b64ToBuffer(b64);
	};

	subtle.sign = async function(algorithm, key, data) {
		const algo = typeof algorithm === 'string' ? { name: algorithm } : algorithm;
		const dataB64 = __bufferSourceToB64(data);
		const resultB64 = __cryptoSign(algo.name, key._id, dataB64);
		return __b64ToBuffer(resultB64);
	};

	subtle.verify = async function(algorithm, key, signature, data) {
		const algo = typeof algorithm === 'string' ? { name: algorithm } : algorithm;
		const sigB64 = __bufferSourceToB64(signature);
		const dataB64 = __bufferSourceToB64(data);
		return __cryptoVerify(algo.name, key._id, sigB64, dataB64);
	};

	subtle.encrypt = async function(algorithm, key, data) {
		const algo = typeof algorithm === 'string' ? { name: algorithm } : algorithm;
		const dataB64 = __bufferSourceToB64(data);
		let ivB64 = '';
		if (algo.iv) {
			ivB64 = __bufferSourceToB64(algo.iv);
		}
		const resultB64 = __cryptoEncrypt(algo.name, key._id, dataB64, ivB64);
		return __b64ToBuffer(resultB64);
	};

	subtle.decrypt = async function(algorithm, key, data) {
		const algo = typeof algorithm === 'string' ? { name: algorithm } : algorithm;
		const dataB64 = __bufferSourceToB64(data);
		let ivB64 = '';
		if (algo.iv) {
			ivB64 = __bufferSourceToB64(algo.iv);
		}
		const resultB64 = __cryptoDecrypt(algo.name, key._id, dataB64, ivB64);
		return __b64ToBuffer(resultB64);
	};

	// Helper: convert any BufferSource or TypedArray to base64.
	function __bufferSourceToB64(data) {
		let arr;
		if (data instanceof ArrayBuffer) {
			arr = new Uint8Array(data);
		} else if (data && data.buffer instanceof ArrayBuffer) {
			arr = new Uint8Array(data.buffer, data.byteOffset || 0, data.byteLength || data.length);
		} else if (data && typeof data.length === 'number') {
			arr = new Uint8Array(data.length);
			for (let i = 0; i < data.length; i++) arr[i] = data[i];
		} else {
			throw new TypeError('expected BufferSource');
		}
		const len = arr.length;
		let r = '';
		for (let i = 0; i < len; i += 3) {
			const a = arr[i];
			const b = i + 1 < len ? arr[i + 1] : 0;
			const c = i + 2 < len ? arr[i + 2] : 0;
			r += _b64e[a >> 2];
			r += _b64e[((a & 3) << 4) | (b >> 4)];
			r += i + 1 < len ? _b64e[((b & 15) << 2) | (c >> 6)] : '=';
			r += i + 2 < len ? _b64e[c & 63] : '=';
		}
		return r;
	}

	// Helper: convert base64 to ArrayBuffer.
	function __b64ToBuffer(b64) {
		let pad = 0;
		if (b64.length > 0 && b64[b64.length - 1] === '=') pad++;
		if (b64.length > 1 && b64[b64.length - 2] === '=') pad++;
		const outLen = (b64.length * 3 / 4) - pad;
		const buf = new ArrayBuffer(outLen);
		const out = new Uint8Array(buf);
		let j = 0;
		for (let i = 0; i < b64.length; i += 4) {
			const a = _b64d[b64.charCodeAt(i)];
			const b = _b64d[b64.charCodeAt(i + 1)];
			const c = _b64d[b64.charCodeAt(i + 2)];
			const d = _b64d[b64.charCodeAt(i + 3)];
			out[j++] = (a << 2) | (b >> 4);
			if (j < outLen) out[j++] = ((b & 15) << 4) | (c >> 2);
			if (j < outLen) out[j++] = ((c & 3) << 6) | d;
		}
		return buf;
	}

	crypto.subtle = subtle;
	globalThis.crypto = crypto;
	globalThis.CryptoKey = CryptoKey;
	// Expose helpers globally so crypto_ext.js can use them.
	globalThis.__bufferSourceToB64 = __bufferSourceToB64;
	globalThis.__b64ToBuffer = __b64ToBuffer;
})();
`

const gcmStandardNonceSize = 12

// getReqIDFromJS reads the __requestID global from the JS context.
func getReqIDFromJS(ctx *v8.Context) uint64 {
	v, err := ctx.Global().Get("__requestID")
	if err != nil || v.IsUndefined() || v.IsNull() {
		return 0
	}
	return uint64(v.Integer())
}

// throwError is a helper to throw a JS exception from a FunctionCallback.
func throwError(iso *v8.Isolate, msg string) *v8.Value {
	errVal, _ := v8.NewValue(iso, msg)
	return iso.ThrowException(errVal)
}

// setupCrypto registers Go-backed crypto helpers and evaluates the JS wrapper.
func setupCrypto(iso *v8.Isolate, ctx *v8.Context, el *eventLoop) error {
	// __cryptoGetRandomBytes(n) -> base64 string of n random bytes.
	ctx.Global().Set("__cryptoGetRandomBytes", v8.NewFunctionTemplate(iso, func(info *v8.FunctionCallbackInfo) *v8.Value {
		args := info.Args()
		if len(args) < 1 {
			return throwError(iso, errMissingArg("__cryptoGetRandomBytes", 1).Error())
		}
		n := int(args[0].Int32())
		if n <= 0 || n > 65536 {
			return throwError(iso, errInvalidArg("getRandomValues", "byte length must be 1-65536").Error())
		}
		buf := make([]byte, n)
		if _, err := rand.Read(buf); err != nil {
			return throwError(iso, fmt.Sprintf("crypto/rand: %v", err))
		}
		val, _ := v8.NewValue(iso, base64.StdEncoding.EncodeToString(buf))
		return val
	}).GetFunction(ctx))

	// __cryptoRandomUUID() -> UUID v4 string.
	ctx.Global().Set("__cryptoRandomUUID", v8.NewFunctionTemplate(iso, func(info *v8.FunctionCallbackInfo) *v8.Value {
		var uuid [16]byte
		if _, err := rand.Read(uuid[:]); err != nil {
			return throwError(iso, fmt.Sprintf("crypto/rand: %v", err))
		}
		uuid[6] = (uuid[6] & 0x0f) | 0x40
		uuid[8] = (uuid[8] & 0x3f) | 0x80
		s := fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
			uuid[0:4], uuid[4:6], uuid[6:8], uuid[8:10], uuid[10:16])
		val, _ := v8.NewValue(iso, s)
		return val
	}).GetFunction(ctx))

	// __cryptoDigest(algorithm, dataBase64) -> resultBase64
	ctx.Global().Set("__cryptoDigest", v8.NewFunctionTemplate(iso, func(info *v8.FunctionCallbackInfo) *v8.Value {
		args := info.Args()
		if len(args) < 2 {
			return throwError(iso, errMissingArg("crypto.subtle.digest", 2).Error())
		}
		algo := args[0].String()
		dataB64 := args[1].String()
		data, err := base64.StdEncoding.DecodeString(dataB64)
		if err != nil {
			return throwError(iso, "digest: invalid base64 data")
		}
		var h hash.Hash
		switch normalizeAlgo(algo) {
		case "SHA-1":
			h = sha1.New()
		case "SHA-256":
			h = sha256.New()
		case "SHA-384":
			h = sha512.New384()
		case "SHA-512":
			h = sha512.New()
		default:
			return throwError(iso, fmt.Sprintf("digest: unsupported algorithm %q", algo))
		}
		h.Write(data)
		val, _ := v8.NewValue(iso, base64.StdEncoding.EncodeToString(h.Sum(nil)))
		return val
	}).GetFunction(ctx))

	// __cryptoImportKey(algoName, hashAlgo, dataBase64) -> keyID
	ctx.Global().Set("__cryptoImportKey", v8.NewFunctionTemplate(iso, func(info *v8.FunctionCallbackInfo) *v8.Value {
		args := info.Args()
		if len(args) < 3 {
			return throwError(iso, errMissingArg("importKey", 3).Error())
		}
		hashAlgo := args[1].String()
		dataB64 := args[2].String()
		keyData, err := base64.StdEncoding.DecodeString(dataB64)
		if err != nil {
			return throwError(iso, "importKey: invalid base64")
		}
		reqID := getReqIDFromJS(ctx)
		id := importCryptoKey(reqID, hashAlgo, keyData)
		if id < 0 {
			return throwError(iso, "importKey: no active request state")
		}
		val, _ := v8.NewValue(iso, int32(id))
		return val
	}).GetFunction(ctx))

	// __cryptoExportKey(keyID) -> base64
	ctx.Global().Set("__cryptoExportKey", v8.NewFunctionTemplate(iso, func(info *v8.FunctionCallbackInfo) *v8.Value {
		args := info.Args()
		if len(args) < 1 {
			return throwError(iso, errMissingArg("exportKey", 1).Error())
		}
		keyID := args[0].Integer()
		reqID := getReqIDFromJS(ctx)
		entry := getCryptoKey(reqID, keyID)
		if entry == nil {
			return throwError(iso, "exportKey: key not found")
		}
		val, _ := v8.NewValue(iso, base64.StdEncoding.EncodeToString(entry.data))
		return val
	}).GetFunction(ctx))

	// __cryptoSign(algorithm, keyID, dataBase64) -> signatureBase64
	ctx.Global().Set("__cryptoSign", v8.NewFunctionTemplate(iso, func(info *v8.FunctionCallbackInfo) *v8.Value {
		args := info.Args()
		if len(args) < 3 {
			return throwError(iso, errMissingArg("sign", 3).Error())
		}
		algo := args[0].String()
		keyID := args[1].Integer()
		dataB64 := args[2].String()
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
			val, _ := v8.NewValue(iso, base64.StdEncoding.EncodeToString(mac.Sum(nil)))
			return val
		default:
			return throwError(iso, fmt.Sprintf("sign: unsupported algorithm %q", algo))
		}
	}).GetFunction(ctx))

	// __cryptoVerify(algorithm, keyID, signatureBase64, dataBase64) -> bool
	ctx.Global().Set("__cryptoVerify", v8.NewFunctionTemplate(iso, func(info *v8.FunctionCallbackInfo) *v8.Value {
		args := info.Args()
		if len(args) < 4 {
			return throwError(iso, errMissingArg("verify", 4).Error())
		}
		algo := args[0].String()
		keyID := args[1].Integer()
		sigB64 := args[2].String()
		dataB64 := args[3].String()
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
		default:
			return throwError(iso, fmt.Sprintf("verify: unsupported algorithm %q", algo))
		}
	}).GetFunction(ctx))

	// __cryptoEncrypt(algorithm, keyID, dataBase64, ivBase64) -> ciphertextBase64
	ctx.Global().Set("__cryptoEncrypt", v8.NewFunctionTemplate(iso, func(info *v8.FunctionCallbackInfo) *v8.Value {
		args := info.Args()
		if len(args) < 4 {
			return throwError(iso, errMissingArg("encrypt", 4).Error())
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
			if len(iv) != gcmStandardNonceSize {
				return throwError(iso, fmt.Sprintf("encrypt: AES-GCM IV must be exactly %d bytes, got %d", gcmStandardNonceSize, len(iv)))
			}
			block, err := aes.NewCipher(entry.data)
			if err != nil {
				return throwError(iso, fmt.Sprintf("encrypt: %v", err))
			}
			gcm, err := cipher.NewGCM(block)
			if err != nil {
				return throwError(iso, fmt.Sprintf("encrypt: %v", err))
			}
			ct := gcm.Seal(nil, iv, data, nil)
			val, _ := v8.NewValue(iso, base64.StdEncoding.EncodeToString(ct))
			return val
		default:
			return throwError(iso, fmt.Sprintf("encrypt: unsupported algorithm %q", algo))
		}
	}).GetFunction(ctx))

	// __cryptoDecrypt(algorithm, keyID, dataBase64, ivBase64) -> plaintextBase64
	ctx.Global().Set("__cryptoDecrypt", v8.NewFunctionTemplate(iso, func(info *v8.FunctionCallbackInfo) *v8.Value {
		args := info.Args()
		if len(args) < 4 {
			return throwError(iso, errMissingArg("decrypt", 4).Error())
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
			if len(iv) != gcmStandardNonceSize {
				return throwError(iso, fmt.Sprintf("decrypt: AES-GCM IV must be exactly %d bytes, got %d", gcmStandardNonceSize, len(iv)))
			}
			block, err := aes.NewCipher(entry.data)
			if err != nil {
				return throwError(iso, fmt.Sprintf("decrypt: %v", err))
			}
			gcm, err := cipher.NewGCM(block)
			if err != nil {
				return throwError(iso, fmt.Sprintf("decrypt: %v", err))
			}
			pt, err := gcm.Open(nil, iv, data, nil)
			if err != nil {
				return throwError(iso, fmt.Sprintf("decrypt: %v", err))
			}
			val, _ := v8.NewValue(iso, base64.StdEncoding.EncodeToString(pt))
			return val
		default:
			return throwError(iso, fmt.Sprintf("decrypt: unsupported algorithm %q", algo))
		}
	}).GetFunction(ctx))

	// Evaluate the JS wrapper that builds the crypto global.
	if _, err := ctx.RunScript(cryptoJS, "crypto.js"); err != nil {
		return fmt.Errorf("evaluating crypto.js: %w", err)
	}

	return nil
}

// hashFuncFromAlgo returns the hash.Hash constructor for the given algorithm name.
func hashFuncFromAlgo(algo string) func() hash.Hash {
	switch normalizeAlgo(algo) {
	case "SHA-1":
		return sha1.New
	case "SHA-256":
		return sha256.New
	case "SHA-384":
		return sha512.New384
	case "SHA-512":
		return sha512.New
	default:
		if algo == "" {
			return sha256.New
		}
		return nil
	}
}

// normalizeAlgo normalizes algorithm names to their canonical form.
func normalizeAlgo(name string) string {
	switch name {
	case "sha-1", "SHA-1", "sha1", "SHA1":
		return "SHA-1"
	case "sha-256", "SHA-256", "sha256", "SHA256":
		return "SHA-256"
	case "sha-384", "SHA-384", "sha384", "SHA384":
		return "SHA-384"
	case "sha-512", "SHA-512", "sha512", "SHA512":
		return "SHA-512"
	case "hmac", "HMAC", "Hmac":
		return "HMAC"
	case "aes-gcm", "AES-GCM", "Aes-Gcm":
		return "AES-GCM"
	case "aes-cbc", "AES-CBC", "Aes-Cbc":
		return "AES-CBC"
	case "ecdsa", "ECDSA", "Ecdsa":
		return "ECDSA"
	case "hkdf", "HKDF", "Hkdf":
		return "HKDF"
	case "pbkdf2", "PBKDF2", "Pbkdf2":
		return "PBKDF2"
	case "rsa-oaep", "RSA-OAEP", "Rsa-Oaep":
		return "RSA-OAEP"
	case "rsassa-pkcs1-v1_5", "RSASSA-PKCS1-v1_5", "RSASSA-PKCS1-V1_5":
		return "RSASSA-PKCS1-v1_5"
	case "rsa-pss", "RSA-PSS", "Rsa-Pss":
		return "RSA-PSS"
	case "ed25519", "Ed25519", "ED25519":
		return "Ed25519"
	default:
		return name
	}
}
