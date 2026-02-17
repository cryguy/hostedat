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
const cryptoJS = `
(function() {
	const crypto = {};

	crypto.getRandomValues = function(typedArray) {
		if (!typedArray || typeof typedArray.length !== 'number') {
			throw new TypeError('getRandomValues requires a TypedArray');
		}
		const b64 = __cryptoGetRandomBytes(typedArray.length);
		const raw = atob(b64);
		for (let i = 0; i < typedArray.length; i++) {
			typedArray[i] = raw.charCodeAt(i);
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
		const raw = atob(resultB64);
		const buf = new ArrayBuffer(raw.length);
		const view = new Uint8Array(buf);
		for (let i = 0; i < raw.length; i++) view[i] = raw.charCodeAt(i);
		return buf;
	};

	// Internal key store: CryptoKey objects hold an ID that maps to Go-side key material.
	let __keyID = 0;

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
		const id = ++__keyID;
		__cryptoImportKey(id, algo.name, b64);
		const keyType = 'secret';
		return new CryptoKey(id, algo, keyType, extractable, usages);
	};

	subtle.exportKey = async function(format, key) {
		if (format !== 'raw') throw new TypeError('exportKey: only raw format is supported');
		if (!key.extractable) throw new DOMException('key is not extractable', 'InvalidAccessError');
		const b64 = __cryptoExportKey(key._id);
		const raw = atob(b64);
		const buf = new ArrayBuffer(raw.length);
		const view = new Uint8Array(buf);
		for (let i = 0; i < raw.length; i++) view[i] = raw.charCodeAt(i);
		return buf;
	};

	subtle.sign = async function(algorithm, key, data) {
		const algo = typeof algorithm === 'string' ? { name: algorithm } : algorithm;
		const dataB64 = __bufferToB64(data);
		const resultB64 = __cryptoSign(algo.name, key._id, dataB64);
		return __b64ToBuffer(resultB64);
	};

	subtle.verify = async function(algorithm, key, signature, data) {
		const algo = typeof algorithm === 'string' ? { name: algorithm } : algorithm;
		const sigB64 = __bufferToB64(signature);
		const dataB64 = __bufferToB64(data);
		return __cryptoVerify(algo.name, key._id, sigB64, dataB64);
	};

	subtle.encrypt = async function(algorithm, key, data) {
		const algo = typeof algorithm === 'string' ? { name: algorithm } : algorithm;
		const dataB64 = __bufferToB64(data);
		let ivB64 = '';
		if (algo.iv) {
			ivB64 = __bufferToB64(algo.iv);
		}
		const resultB64 = __cryptoEncrypt(algo.name, key._id, dataB64, ivB64);
		return __b64ToBuffer(resultB64);
	};

	subtle.decrypt = async function(algorithm, key, data) {
		const algo = typeof algorithm === 'string' ? { name: algorithm } : algorithm;
		const dataB64 = __bufferToB64(data);
		let ivB64 = '';
		if (algo.iv) {
			ivB64 = __bufferToB64(algo.iv);
		}
		const resultB64 = __cryptoDecrypt(algo.name, key._id, dataB64, ivB64);
		return __b64ToBuffer(resultB64);
	};

	// Helper: robustly convert any BufferSource or TypedArray to base64.
	// Uses duck-typing (data.buffer, data.length) instead of ArrayBuffer.isView
	// for QuickJS WASM compatibility.
	function __bufferSourceToB64(data) {
		let arr;
		if (data instanceof ArrayBuffer) {
			arr = new Uint8Array(data);
		} else if (data && data.buffer instanceof ArrayBuffer) {
			// TypedArray (Uint8Array, etc.)
			arr = new Uint8Array(data.buffer, data.byteOffset || 0, data.byteLength || data.length);
		} else if (data && typeof data.length === 'number') {
			// Array-like (e.g. plain Uint8Array without .buffer in some engines)
			arr = new Uint8Array(data.length);
			for (let i = 0; i < data.length; i++) arr[i] = data[i];
		} else if (typeof data === 'string') {
			const enc = new TextEncoder();
			arr = enc.encode(data);
		} else {
			throw new TypeError('expected BufferSource');
		}
		let s = '';
		for (let i = 0; i < arr.length; i++) s += String.fromCharCode(arr[i]);
		return btoa(s);
	}

	// Alias for backward compat.
	function __bufferToB64(data) { return __bufferSourceToB64(data); }
	globalThis.__bufferToB64 = __bufferToB64;

	// Helper: convert base64 to ArrayBuffer.
	function __b64ToBuffer(b64) {
		const raw = atob(b64);
		const buf = new ArrayBuffer(raw.length);
		const view = new Uint8Array(buf);
		for (let i = 0; i < raw.length; i++) view[i] = raw.charCodeAt(i);
		return buf;
	}

	crypto.subtle = subtle;
	globalThis.crypto = crypto;
	globalThis.CryptoKey = CryptoKey;
})();
`

// cryptoKeys stores imported key material by ID (per-runtime).
// Since QuickJS is single-threaded and each runtime serves one request at a
// time, a global map keyed by runtime pointer would work, but it's simpler
// to use the JS global's __requestID pattern. Instead, we store keys in a
// package-level map keyed by a counter ID that the JS side manages.
var (
	cryptoKeyStore     = make(map[int64][]byte)
	cryptoKeyStoreLock = make(chan struct{}, 1)
)

func init() {
	cryptoKeyStoreLock <- struct{}{} // Initialize the semaphore.
}

func withKeyStore(fn func()) {
	<-cryptoKeyStoreLock
	defer func() { cryptoKeyStoreLock <- struct{}{} }()
	fn()
}

// setupCrypto registers Go-backed crypto helpers and evaluates the JS wrapper
// that builds the crypto global object with getRandomValues, randomUUID,
// and crypto.subtle methods.
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

	// __cryptoImportKey(id, algorithm, dataBase64)
	ctx.SetFunc("__cryptoImportKey", func(this *qjs.This) (*qjs.Value, error) {
		c := this.Context()
		args := this.Args()
		if len(args) < 3 {
			return nil, errMissingArg("importKey", 3)
		}
		id := args[0].Int64()
		dataB64 := args[2].String()

		keyData, err := base64.StdEncoding.DecodeString(dataB64)
		if err != nil {
			return nil, fmt.Errorf("importKey: invalid base64")
		}

		withKeyStore(func() {
			cryptoKeyStore[id] = keyData
		})

		return c.NewUndefined(), nil
	})

	// __cryptoExportKey(id) -> base64
	ctx.SetFunc("__cryptoExportKey", func(this *qjs.This) (*qjs.Value, error) {
		c := this.Context()
		args := this.Args()
		if len(args) < 1 {
			return nil, errMissingArg("exportKey", 1)
		}
		id := args[0].Int64()

		var keyData []byte
		withKeyStore(func() {
			keyData = cryptoKeyStore[id]
		})
		if keyData == nil {
			return nil, fmt.Errorf("exportKey: key not found")
		}

		return c.NewString(base64.StdEncoding.EncodeToString(keyData)), nil
	})

	// __cryptoSign(algorithm, keyID, dataBase64) -> signatureBase64
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

		var keyData []byte
		withKeyStore(func() {
			keyData = cryptoKeyStore[keyID]
		})
		if keyData == nil {
			return nil, fmt.Errorf("sign: key not found")
		}

		switch normalizeAlgo(algo) {
		case "HMAC":
			// Default to SHA-256 for HMAC (most common usage).
			mac := hmac.New(sha256.New, keyData)
			mac.Write(data)
			sig := mac.Sum(nil)
			return c.NewString(base64.StdEncoding.EncodeToString(sig)), nil
		default:
			return nil, fmt.Errorf("sign: unsupported algorithm %q", algo)
		}
	})

	// __cryptoVerify(algorithm, keyID, signatureBase64, dataBase64) -> bool
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

		var keyData []byte
		withKeyStore(func() {
			keyData = cryptoKeyStore[keyID]
		})
		if keyData == nil {
			return nil, fmt.Errorf("verify: key not found")
		}

		switch normalizeAlgo(algo) {
		case "HMAC":
			mac := hmac.New(sha256.New, keyData)
			mac.Write(data)
			expected := mac.Sum(nil)
			match := hmac.Equal(sig, expected)
			if match {
				return c.NewBool(true), nil
			}
			return c.NewBool(false), nil
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

		var keyData []byte
		withKeyStore(func() {
			keyData = cryptoKeyStore[keyID]
		})
		if keyData == nil {
			return nil, fmt.Errorf("encrypt: key not found")
		}

		switch normalizeAlgo(algo) {
		case "AES-GCM":
			iv, err := base64.StdEncoding.DecodeString(ivB64)
			if err != nil {
				return nil, fmt.Errorf("encrypt: invalid IV base64")
			}
			block, err := aes.NewCipher(keyData)
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

		var keyData []byte
		withKeyStore(func() {
			keyData = cryptoKeyStore[keyID]
		})
		if keyData == nil {
			return nil, fmt.Errorf("decrypt: key not found")
		}

		switch normalizeAlgo(algo) {
		case "AES-GCM":
			iv, err := base64.StdEncoding.DecodeString(ivB64)
			if err != nil {
				return nil, fmt.Errorf("decrypt: invalid IV base64")
			}
			block, err := aes.NewCipher(keyData)
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
	default:
		return name
	}
}
