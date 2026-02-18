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

	"github.com/fastschema/qjs"
)

// cryptoJS wires up the global crypto object with getRandomValues and randomUUID
// backed by Go helper functions, plus a crypto.subtle proxy that delegates
// digest/sign/verify/encrypt/decrypt/importKey/exportKey to Go-backed functions.
//
// Key material is scoped per-request via __requestID — no global key store.
// Key IDs are allocated by Go (not JS) to prevent cross-request collisions.
const cryptoJS = `
(function() {
	// Pure-JS base64 encode/decode for the crypto internals.
	// We intentionally avoid atob/btoa here because binary strings containing
	// null bytes (0x00) get truncated when crossing the QJS/Go C-string boundary
	// (JS_ToCString and JS_NewString are null-terminated). By encoding/decoding
	// base64 directly from byte arrays we never create an intermediate binary
	// string, eliminating the corruption entirely.
	const _b64e = 'ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/';
	const _b64d = new Uint8Array(128);
	for (let i = 0; i < _b64e.length; i++) _b64d[_b64e.charCodeAt(i)] = i;

	const crypto = {};

	crypto.getRandomValues = function(typedArray) {
		if (!typedArray || typeof typedArray.length !== 'number') {
			throw new TypeError('getRandomValues requires a TypedArray');
		}
		const b64 = __cryptoGetRandomBytes(typedArray.length);
		// Decode base64 directly to byte values — no atob binary string.
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
	// Uses pure JS base64 encoding directly from byte values to avoid
	// the String.fromCharCode + btoa path that corrupts null bytes.
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
	// Uses pure JS base64 decoding directly to byte values to avoid
	// the atob path that corrupts null bytes.
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
func getReqIDFromJS(c *qjs.Context) uint64 {
	v := c.Global().GetPropertyStr("__requestID")
	id := uint64(v.Int64())
	v.Free()
	return id
}

// setupCrypto registers Go-backed crypto helpers and evaluates the JS wrapper
// that builds the crypto global object with getRandomValues, randomUUID,
// and crypto.subtle methods. Key material is scoped per-request.
func setupCrypto(rt *qjs.Runtime) error {
	ctx := rt.Context()

	// __cryptoGetRandomBytes(n) -> base64 string of n random bytes.
	ctx.SetFunc("__cryptoGetRandomBytes", func(this *qjs.This) (*qjs.Value, error) {
		c := this.Context()
		args := this.Args()
		if len(args) < 1 {
			return nil, errMissingArg("__cryptoGetRandomBytes", 1)
		}
		n := int(args[0].Int32())
		if n <= 0 || n > 65536 {
			return nil, errInvalidArg("getRandomValues", "byte length must be 1-65536")
		}
		buf := make([]byte, n)
		if _, err := rand.Read(buf); err != nil {
			return nil, fmt.Errorf("crypto/rand: %w", err)
		}
		return c.NewString(base64.StdEncoding.EncodeToString(buf)), nil
	})

	// __cryptoRandomUUID() -> UUID v4 string.
	ctx.SetFunc("__cryptoRandomUUID", func(this *qjs.This) (*qjs.Value, error) {
		c := this.Context()
		var uuid [16]byte
		if _, err := rand.Read(uuid[:]); err != nil {
			return nil, fmt.Errorf("crypto/rand: %w", err)
		}
		// Set version (4) and variant (RFC 4122).
		uuid[6] = (uuid[6] & 0x0f) | 0x40
		uuid[8] = (uuid[8] & 0x3f) | 0x80
		s := fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
			uuid[0:4], uuid[4:6], uuid[6:8], uuid[8:10], uuid[10:16])
		return c.NewString(s), nil
	})

	// __cryptoDigest(algorithm, dataBase64) -> resultBase64
	ctx.SetFunc("__cryptoDigest", func(this *qjs.This) (*qjs.Value, error) {
		c := this.Context()
		args := this.Args()
		if len(args) < 2 {
			return nil, errMissingArg("crypto.subtle.digest", 2)
		}
		algo := args[0].String()
		dataB64 := args[1].String()

		data, err := base64.StdEncoding.DecodeString(dataB64)
		if err != nil {
			return nil, fmt.Errorf("digest: invalid base64 data")
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
			return nil, fmt.Errorf("digest: unsupported algorithm %q", algo)
		}

		h.Write(data)
		result := h.Sum(nil)
		return c.NewString(base64.StdEncoding.EncodeToString(result)), nil
	})

	// __cryptoImportKey(algoName, hashAlgo, dataBase64) -> keyID
	// Key material is stored in the per-request state, not in a global map.
	ctx.SetFunc("__cryptoImportKey", func(this *qjs.This) (*qjs.Value, error) {
		c := this.Context()
		args := this.Args()
		if len(args) < 3 {
			return nil, errMissingArg("importKey", 3)
		}
		hashAlgo := args[1].String()
		dataB64 := args[2].String()

		keyData, err := base64.StdEncoding.DecodeString(dataB64)
		if err != nil {
			return nil, fmt.Errorf("importKey: invalid base64")
		}

		reqID := getReqIDFromJS(c)
		id := importCryptoKey(reqID, hashAlgo, keyData)
		if id < 0 {
			return nil, fmt.Errorf("importKey: no active request state")
		}

		return c.NewInt64(id), nil
	})

	// __cryptoExportKey(keyID) -> base64
	ctx.SetFunc("__cryptoExportKey", func(this *qjs.This) (*qjs.Value, error) {
		c := this.Context()
		args := this.Args()
		if len(args) < 1 {
			return nil, errMissingArg("exportKey", 1)
		}
		keyID := args[0].Int64()

		reqID := getReqIDFromJS(c)
		entry := getCryptoKey(reqID, keyID)
		if entry == nil {
			return nil, fmt.Errorf("exportKey: key not found")
		}

		return c.NewString(base64.StdEncoding.EncodeToString(entry.data)), nil
	})

	// __cryptoSign(algorithm, keyID, dataBase64) -> signatureBase64
	// Uses the hash algorithm stored at importKey time.
	ctx.SetFunc("__cryptoSign", func(this *qjs.This) (*qjs.Value, error) {
		c := this.Context()
		args := this.Args()
		if len(args) < 3 {
			return nil, errMissingArg("sign", 3)
		}
		algo := args[0].String()
		keyID := args[1].Int64()
		dataB64 := args[2].String()

		data, err := base64.StdEncoding.DecodeString(dataB64)
		if err != nil {
			return nil, fmt.Errorf("sign: invalid base64")
		}

		reqID := getReqIDFromJS(c)
		entry := getCryptoKey(reqID, keyID)
		if entry == nil {
			return nil, fmt.Errorf("sign: key not found")
		}

		switch normalizeAlgo(algo) {
		case "HMAC":
			hashFn := hashFuncFromAlgo(entry.hashAlgo)
			if hashFn == nil {
				return nil, fmt.Errorf("sign: unsupported HMAC hash %q", entry.hashAlgo)
			}
			mac := hmac.New(hashFn, entry.data)
			mac.Write(data)
			sig := mac.Sum(nil)
			return c.NewString(base64.StdEncoding.EncodeToString(sig)), nil
		default:
			return nil, fmt.Errorf("sign: unsupported algorithm %q", algo)
		}
	})

	// __cryptoVerify(algorithm, keyID, signatureBase64, dataBase64) -> bool
	// Uses the hash algorithm stored at importKey time.
	ctx.SetFunc("__cryptoVerify", func(this *qjs.This) (*qjs.Value, error) {
		c := this.Context()
		args := this.Args()
		if len(args) < 4 {
			return nil, errMissingArg("verify", 4)
		}
		algo := args[0].String()
		keyID := args[1].Int64()
		sigB64 := args[2].String()
		dataB64 := args[3].String()

		sig, err := base64.StdEncoding.DecodeString(sigB64)
		if err != nil {
			return nil, fmt.Errorf("verify: invalid signature base64")
		}
		data, err := base64.StdEncoding.DecodeString(dataB64)
		if err != nil {
			return nil, fmt.Errorf("verify: invalid data base64")
		}

		reqID := getReqIDFromJS(c)
		entry := getCryptoKey(reqID, keyID)
		if entry == nil {
			return nil, fmt.Errorf("verify: key not found")
		}

		switch normalizeAlgo(algo) {
		case "HMAC":
			hashFn := hashFuncFromAlgo(entry.hashAlgo)
			if hashFn == nil {
				return nil, fmt.Errorf("verify: unsupported HMAC hash %q", entry.hashAlgo)
			}
			mac := hmac.New(hashFn, entry.data)
			mac.Write(data)
			expected := mac.Sum(nil)
			return c.NewBool(hmac.Equal(sig, expected)), nil
		default:
			return nil, fmt.Errorf("verify: unsupported algorithm %q", algo)
		}
	})

	// __cryptoEncrypt(algorithm, keyID, dataBase64, ivBase64) -> ciphertextBase64
	ctx.SetFunc("__cryptoEncrypt", func(this *qjs.This) (*qjs.Value, error) {
		c := this.Context()
		args := this.Args()
		if len(args) < 4 {
			return nil, errMissingArg("encrypt", 4)
		}
		algo := args[0].String()
		keyID := args[1].Int64()
		dataB64 := args[2].String()
		ivB64 := args[3].String()

		data, err := base64.StdEncoding.DecodeString(dataB64)
		if err != nil {
			return nil, fmt.Errorf("encrypt: invalid base64 data")
		}

		reqID := getReqIDFromJS(c)
		entry := getCryptoKey(reqID, keyID)
		if entry == nil {
			return nil, fmt.Errorf("encrypt: key not found")
		}

		switch normalizeAlgo(algo) {
		case "AES-GCM":
			iv, err := base64.StdEncoding.DecodeString(ivB64)
			if err != nil {
				return nil, fmt.Errorf("encrypt: invalid IV base64")
			}
			if len(iv) != gcmStandardNonceSize {
				return nil, fmt.Errorf("encrypt: AES-GCM IV must be exactly %d bytes, got %d", gcmStandardNonceSize, len(iv))
			}
			block, err := aes.NewCipher(entry.data)
			if err != nil {
				return nil, fmt.Errorf("encrypt: %w", err)
			}
			gcm, err := cipher.NewGCM(block)
			if err != nil {
				return nil, fmt.Errorf("encrypt: %w", err)
			}
			ciphertext := gcm.Seal(nil, iv, data, nil)
			return c.NewString(base64.StdEncoding.EncodeToString(ciphertext)), nil
		default:
			return nil, fmt.Errorf("encrypt: unsupported algorithm %q", algo)
		}
	})

	// __cryptoDecrypt(algorithm, keyID, dataBase64, ivBase64) -> plaintextBase64
	ctx.SetFunc("__cryptoDecrypt", func(this *qjs.This) (*qjs.Value, error) {
		c := this.Context()
		args := this.Args()
		if len(args) < 4 {
			return nil, errMissingArg("decrypt", 4)
		}
		algo := args[0].String()
		keyID := args[1].Int64()
		dataB64 := args[2].String()
		ivB64 := args[3].String()

		data, err := base64.StdEncoding.DecodeString(dataB64)
		if err != nil {
			return nil, fmt.Errorf("decrypt: invalid base64 data")
		}

		reqID := getReqIDFromJS(c)
		entry := getCryptoKey(reqID, keyID)
		if entry == nil {
			return nil, fmt.Errorf("decrypt: key not found")
		}

		switch normalizeAlgo(algo) {
		case "AES-GCM":
			iv, err := base64.StdEncoding.DecodeString(ivB64)
			if err != nil {
				return nil, fmt.Errorf("decrypt: invalid IV base64")
			}
			if len(iv) != gcmStandardNonceSize {
				return nil, fmt.Errorf("decrypt: AES-GCM IV must be exactly %d bytes, got %d", gcmStandardNonceSize, len(iv))
			}
			block, err := aes.NewCipher(entry.data)
			if err != nil {
				return nil, fmt.Errorf("decrypt: %w", err)
			}
			gcm, err := cipher.NewGCM(block)
			if err != nil {
				return nil, fmt.Errorf("decrypt: %w", err)
			}
			plaintext, err := gcm.Open(nil, iv, data, nil)
			if err != nil {
				return nil, fmt.Errorf("decrypt: %w", err)
			}
			return c.NewString(base64.StdEncoding.EncodeToString(plaintext)), nil
		default:
			return nil, fmt.Errorf("decrypt: unsupported algorithm %q", algo)
		}
	})

	// Evaluate the JS wrapper that builds the crypto global.
	if _, err := rt.Eval("crypto.js", qjs.Code(cryptoJS)); err != nil {
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
		// Default to SHA-256 for backward compatibility when no hash is specified.
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
	default:
		return name
	}
}
