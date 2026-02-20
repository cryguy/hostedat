package worker

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"fmt"

	v8 "github.com/tommie/v8go"
)

// aesKeyWrap implements RFC 3394 AES Key Wrap.
// The plaintext must be a multiple of 8 bytes (64-bit blocks).
func aesKeyWrap(kek, plaintext []byte) ([]byte, error) {
	if len(plaintext)%8 != 0 || len(plaintext) < 16 {
		return nil, fmt.Errorf("plaintext must be a multiple of 8 bytes and at least 16 bytes")
	}

	block, err := aes.NewCipher(kek)
	if err != nil {
		return nil, err
	}

	n := len(plaintext) / 8 // number of 64-bit blocks

	// Initialize: A = IV (default 0xA6A6A6A6A6A6A6A6), R[1..n] = plaintext blocks
	var a [8]byte
	for i := range a {
		a[i] = 0xA6
	}

	r := make([][]byte, n)
	for i := 0; i < n; i++ {
		r[i] = make([]byte, 8)
		copy(r[i], plaintext[i*8:(i+1)*8])
	}

	// Wrap: 6 rounds
	buf := make([]byte, 16)
	for j := 0; j < 6; j++ {
		for i := 0; i < n; i++ {
			copy(buf[:8], a[:])
			copy(buf[8:], r[i])
			block.Encrypt(buf, buf)
			copy(a[:], buf[:8])
			// XOR counter t = n*j + i + 1 into A
			t := uint64(n*j + i + 1)
			var tBytes [8]byte
			binary.BigEndian.PutUint64(tBytes[:], t)
			for k := 0; k < 8; k++ {
				a[k] ^= tBytes[k]
			}
			copy(r[i], buf[8:])
		}
	}

	// Output: A || R[1] || R[2] || ... || R[n]
	out := make([]byte, 8+n*8)
	copy(out[:8], a[:])
	for i := 0; i < n; i++ {
		copy(out[8+i*8:8+(i+1)*8], r[i])
	}
	return out, nil
}

// aesKeyUnwrap implements RFC 3394 AES Key Unwrap.
func aesKeyUnwrap(kek, ciphertext []byte) ([]byte, error) {
	if len(ciphertext)%8 != 0 || len(ciphertext) < 24 {
		return nil, fmt.Errorf("ciphertext must be a multiple of 8 bytes and at least 24 bytes")
	}

	block, err := aes.NewCipher(kek)
	if err != nil {
		return nil, err
	}

	n := (len(ciphertext) / 8) - 1 // number of 64-bit blocks (excluding A)

	// Initialize: A = C[0], R[1..n] = C[1..n]
	var a [8]byte
	copy(a[:], ciphertext[:8])

	r := make([][]byte, n)
	for i := 0; i < n; i++ {
		r[i] = make([]byte, 8)
		copy(r[i], ciphertext[(i+1)*8:(i+2)*8])
	}

	// Unwrap: 6 rounds in reverse
	buf := make([]byte, 16)
	for j := 5; j >= 0; j-- {
		for i := n - 1; i >= 0; i-- {
			// XOR counter t = n*j + i + 1 out of A
			t := uint64(n*j + i + 1)
			var tBytes [8]byte
			binary.BigEndian.PutUint64(tBytes[:], t)
			for k := 0; k < 8; k++ {
				a[k] ^= tBytes[k]
			}
			copy(buf[:8], a[:])
			copy(buf[8:], r[i])
			block.Decrypt(buf, buf)
			copy(a[:], buf[:8])
			copy(r[i], buf[8:])
		}
	}

	// Verify IV
	for i := 0; i < 8; i++ {
		if a[i] != 0xA6 {
			return nil, fmt.Errorf("AES-KW unwrap: integrity check failed")
		}
	}

	// Output: R[1] || R[2] || ... || R[n]
	out := make([]byte, n*8)
	for i := 0; i < n; i++ {
		copy(out[i*8:(i+1)*8], r[i])
	}
	return out, nil
}

// cryptoAesCtrKwJS patches crypto.subtle with AES-CTR encrypt/decrypt and
// AES-KW wrapKey/unwrapKey using the chain-of-responsibility pattern.
const cryptoAesCtrKwJS = `
(function() {
var subtle = crypto.subtle;
var CK = CryptoKey;
var _prevEncrypt = subtle.encrypt;
var _prevDecrypt = subtle.decrypt;
var _prevGenerateKey = subtle.generateKey;
var _prevWrapKey = subtle.wrapKey;
var _prevUnwrapKey = subtle.unwrapKey;

subtle.encrypt = async function(algorithm, key, data) {
	var algo = typeof algorithm === 'string' ? { name: algorithm } : algorithm;
	if (algo.name === 'AES-CTR') {
		if (!algo.counter) throw new TypeError('AES-CTR requires counter parameter');
		var counterB64 = __bufferSourceToB64(algo.counter);
		var length = algo.length || 64;
		var dataB64 = __bufferSourceToB64(data);
		var resultB64 = __cryptoEncryptAesCtr(key._id, dataB64, counterB64, length);
		return __b64ToBuffer(resultB64);
	}
	return _prevEncrypt.call(this, algorithm, key, data);
};

subtle.decrypt = async function(algorithm, key, data) {
	var algo = typeof algorithm === 'string' ? { name: algorithm } : algorithm;
	if (algo.name === 'AES-CTR') {
		if (!algo.counter) throw new TypeError('AES-CTR requires counter parameter');
		var counterB64 = __bufferSourceToB64(algo.counter);
		var length = algo.length || 64;
		var dataB64 = __bufferSourceToB64(data);
		var resultB64 = __cryptoDecryptAesCtr(key._id, dataB64, counterB64, length);
		return __b64ToBuffer(resultB64);
	}
	return _prevDecrypt.call(this, algorithm, key, data);
};

subtle.generateKey = async function(algorithm, extractable, usages) {
	var algo = typeof algorithm === 'string' ? { name: algorithm } : algorithm;
	if (algo.name === 'AES-CTR' || algo.name === 'AES-KW') {
		var length = algo.length || 256;
		var resultJSON = __cryptoGenerateKeyAes(algo.name, length);
		var result = JSON.parse(resultJSON);
		if (result.error) throw new TypeError(result.error);
		return new CK(result.keyId, algo, 'secret', extractable, usages);
	}
	return _prevGenerateKey.call(this, algorithm, extractable, usages);
};

subtle.wrapKey = async function(format, key, wrappingKey, wrapAlgorithm) {
	var wrapAlgo = typeof wrapAlgorithm === 'string' ? { name: wrapAlgorithm } : wrapAlgorithm;
	if (wrapAlgo.name === 'AES-KW') {
		var exported = await subtle.exportKey('raw', key);
		var dataB64 = __bufferSourceToB64(exported);
		var resultB64 = __cryptoWrapKeyAESKW(wrappingKey._id, dataB64);
		return __b64ToBuffer(resultB64);
	}
	return _prevWrapKey.call(this, format, key, wrappingKey, wrapAlgorithm);
};

subtle.unwrapKey = async function(format, wrappedKey, unwrappingKey, unwrapAlgorithm, unwrappedKeyAlgorithm, extractable, keyUsages) {
	var unwrapAlgo = typeof unwrapAlgorithm === 'string' ? { name: unwrapAlgorithm } : unwrapAlgorithm;
	if (unwrapAlgo.name === 'AES-KW') {
		var wrappedB64 = __bufferSourceToB64(wrappedKey);
		var resultB64 = __cryptoUnwrapKeyAESKW(unwrappingKey._id, wrappedB64);
		var unwrappedData = __b64ToBuffer(resultB64);
		return subtle.importKey('raw', unwrappedData, unwrappedKeyAlgorithm, extractable, keyUsages);
	}
	return _prevUnwrapKey.call(this, format, wrappedKey, unwrappingKey, unwrapAlgorithm, unwrappedKeyAlgorithm, extractable, keyUsages);
};

})();
`

// setupCryptoAesCtrKw registers AES-CTR and AES-KW Go functions and evaluates
// the JS patches. Must run after setupCryptoRSA (or at least after setupCryptoExt).
func setupCryptoAesCtrKw(iso *v8.Isolate, ctx *v8.Context, _ *eventLoop) error {
	// __cryptoEncryptAesCtr(keyID, dataB64, counterB64, length) -> resultB64
	_ = ctx.Global().Set("__cryptoEncryptAesCtr", v8.NewFunctionTemplate(iso, func(info *v8.FunctionCallbackInfo) *v8.Value {
		args := info.Args()
		if len(args) < 4 {
			return throwError(iso, "encryptAesCtr requires 4 argument(s)")
		}
		keyID := args[0].Integer()
		dataB64 := args[1].String()
		counterB64 := args[2].String()
		_ = int(args[3].Int32()) // length param (bits of counter block to use)

		data, err := base64.StdEncoding.DecodeString(dataB64)
		if err != nil {
			return throwError(iso, "encryptAesCtr: invalid base64 data")
		}
		counter, err := base64.StdEncoding.DecodeString(counterB64)
		if err != nil {
			return throwError(iso, "encryptAesCtr: invalid counter base64")
		}
		if len(counter) != 16 {
			return throwError(iso, fmt.Sprintf("encryptAesCtr: counter must be exactly 16 bytes, got %d", len(counter)))
		}

		reqID := getReqIDFromJS(ctx)
		entry := getCryptoKey(reqID, keyID)
		if entry == nil {
			return throwError(iso, "encryptAesCtr: key not found")
		}

		block, err := aes.NewCipher(entry.data)
		if err != nil {
			return throwError(iso, fmt.Sprintf("encryptAesCtr: %s", err.Error()))
		}

		ct := make([]byte, len(data))
		stream := cipher.NewCTR(block, counter)
		stream.XORKeyStream(ct, data)

		val, _ := v8.NewValue(iso, base64.StdEncoding.EncodeToString(ct))
		return val
	}).GetFunction(ctx))

	// __cryptoDecryptAesCtr(keyID, dataB64, counterB64, length) -> resultB64
	_ = ctx.Global().Set("__cryptoDecryptAesCtr", v8.NewFunctionTemplate(iso, func(info *v8.FunctionCallbackInfo) *v8.Value {
		args := info.Args()
		if len(args) < 4 {
			return throwError(iso, "decryptAesCtr requires 4 argument(s)")
		}
		keyID := args[0].Integer()
		dataB64 := args[1].String()
		counterB64 := args[2].String()
		_ = int(args[3].Int32()) // length param

		data, err := base64.StdEncoding.DecodeString(dataB64)
		if err != nil {
			return throwError(iso, "decryptAesCtr: invalid base64 data")
		}
		counter, err := base64.StdEncoding.DecodeString(counterB64)
		if err != nil {
			return throwError(iso, "decryptAesCtr: invalid counter base64")
		}
		if len(counter) != 16 {
			return throwError(iso, fmt.Sprintf("decryptAesCtr: counter must be exactly 16 bytes, got %d", len(counter)))
		}

		reqID := getReqIDFromJS(ctx)
		entry := getCryptoKey(reqID, keyID)
		if entry == nil {
			return throwError(iso, "decryptAesCtr: key not found")
		}

		block, err := aes.NewCipher(entry.data)
		if err != nil {
			return throwError(iso, fmt.Sprintf("decryptAesCtr: %s", err.Error()))
		}

		pt := make([]byte, len(data))
		stream := cipher.NewCTR(block, counter)
		stream.XORKeyStream(pt, data)

		val, _ := v8.NewValue(iso, base64.StdEncoding.EncodeToString(pt))
		return val
	}).GetFunction(ctx))

	// __cryptoGenerateKeyAes(algoName, length) -> JSON { keyId } or { error }
	_ = ctx.Global().Set("__cryptoGenerateKeyAes", v8.NewFunctionTemplate(iso, func(info *v8.FunctionCallbackInfo) *v8.Value {
		args := info.Args()
		if len(args) < 2 {
			return throwError(iso, "generateKeyAes requires 2 argument(s)")
		}
		algoName := args[0].String()
		bitLength := int(args[1].Int32())

		var byteLength int
		switch bitLength {
		case 128:
			byteLength = 16
		case 192:
			byteLength = 24
		case 256:
			byteLength = 32
		default:
			val, _ := v8.NewValue(iso, fmt.Sprintf(`{"error":"generateKey: unsupported key length %d for %s"}`, bitLength, algoName))
			return val
		}

		reqID := getReqIDFromJS(ctx)
		if getRequestState(reqID) == nil {
			val, _ := v8.NewValue(iso, `{"error":"no active request state"}`)
			return val
		}

		keyData := make([]byte, byteLength)
		if _, err := rand.Read(keyData); err != nil {
			val, _ := v8.NewValue(iso, fmt.Sprintf(`{"error":"key generation failed: %s"}`, err.Error()))
			return val
		}

		id := importCryptoKeyFull(reqID, &cryptoKeyEntry{
			data:     keyData,
			algoName: normalizeAlgo(algoName),
			keyType:  "secret",
		})
		val, _ := v8.NewValue(iso, fmt.Sprintf(`{"keyId":%d}`, id))
		return val
	}).GetFunction(ctx))

	// __cryptoWrapKeyAESKW(wrappingKeyID, dataB64) -> wrappedB64
	_ = ctx.Global().Set("__cryptoWrapKeyAESKW", v8.NewFunctionTemplate(iso, func(info *v8.FunctionCallbackInfo) *v8.Value {
		args := info.Args()
		if len(args) < 2 {
			return throwError(iso, "wrapKeyAESKW requires 2 argument(s)")
		}
		wrappingKeyID := args[0].Integer()
		dataB64 := args[1].String()

		data, err := base64.StdEncoding.DecodeString(dataB64)
		if err != nil {
			return throwError(iso, "wrapKeyAESKW: invalid base64 data")
		}

		reqID := getReqIDFromJS(ctx)
		wrappingEntry := getCryptoKey(reqID, wrappingKeyID)
		if wrappingEntry == nil {
			return throwError(iso, "wrapKeyAESKW: wrapping key not found")
		}

		// Key data to wrap must be a multiple of 8 bytes
		if len(data)%8 != 0 {
			return throwError(iso, "wrapKeyAESKW: key data must be a multiple of 8 bytes")
		}
		if len(data) < 16 {
			return throwError(iso, "wrapKeyAESKW: key data must be at least 16 bytes")
		}

		wrapped, err := aesKeyWrap(wrappingEntry.data, data)
		if err != nil {
			return throwError(iso, fmt.Sprintf("wrapKeyAESKW: %s", err.Error()))
		}

		val, _ := v8.NewValue(iso, base64.StdEncoding.EncodeToString(wrapped))
		return val
	}).GetFunction(ctx))

	// __cryptoUnwrapKeyAESKW(unwrappingKeyID, wrappedB64) -> unwrappedB64
	_ = ctx.Global().Set("__cryptoUnwrapKeyAESKW", v8.NewFunctionTemplate(iso, func(info *v8.FunctionCallbackInfo) *v8.Value {
		args := info.Args()
		if len(args) < 2 {
			return throwError(iso, "unwrapKeyAESKW requires 2 argument(s)")
		}
		unwrappingKeyID := args[0].Integer()
		wrappedB64 := args[1].String()

		wrappedData, err := base64.StdEncoding.DecodeString(wrappedB64)
		if err != nil {
			return throwError(iso, "unwrapKeyAESKW: invalid base64 data")
		}

		reqID := getReqIDFromJS(ctx)
		unwrappingEntry := getCryptoKey(reqID, unwrappingKeyID)
		if unwrappingEntry == nil {
			return throwError(iso, "unwrapKeyAESKW: unwrapping key not found")
		}

		unwrapped, err := aesKeyUnwrap(unwrappingEntry.data, wrappedData)
		if err != nil {
			return throwError(iso, fmt.Sprintf("unwrapKeyAESKW: %s", err.Error()))
		}

		val, _ := v8.NewValue(iso, base64.StdEncoding.EncodeToString(unwrapped))
		return val
	}).GetFunction(ctx))

	// Evaluate the JS patches.
	if _, err := ctx.RunScript(cryptoAesCtrKwJS, "crypto_kw.js"); err != nil {
		return fmt.Errorf("evaluating crypto_kw.js: %w", err)
	}

	return nil
}
