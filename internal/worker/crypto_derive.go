package worker

import (
	"crypto/hmac"
	"encoding/base64"
	"fmt"
	"hash"
	"math"

	v8 "github.com/tommie/v8go"
)

// hkdfDeriveBits implements HKDF (RFC 5869) Extract+Expand.
func hkdfDeriveBits(hashFn func() hash.Hash, ikm, salt, info []byte, lengthBits int) ([]byte, error) {
	lengthBytes := lengthBits / 8
	if lengthBits%8 != 0 {
		return nil, fmt.Errorf("HKDF: length must be a multiple of 8")
	}

	// Extract: PRK = HMAC-Hash(salt, IKM)
	if len(salt) == 0 {
		salt = make([]byte, hashFn().Size())
	}
	extractor := hmac.New(hashFn, salt)
	extractor.Write(ikm)
	prk := extractor.Sum(nil)

	// Expand
	hashLen := hashFn().Size()
	n := int(math.Ceil(float64(lengthBytes) / float64(hashLen)))
	if n > 255 {
		return nil, fmt.Errorf("HKDF: output length too large")
	}

	okm := make([]byte, 0, lengthBytes)
	var prev []byte
	for i := 1; i <= n; i++ {
		h := hmac.New(hashFn, prk)
		h.Write(prev)
		h.Write(info)
		h.Write([]byte{byte(i)})
		prev = h.Sum(nil)
		okm = append(okm, prev...)
	}

	return okm[:lengthBytes], nil
}

// pbkdf2DeriveBits implements PBKDF2 (RFC 2898).
func pbkdf2DeriveBits(hashFn func() hash.Hash, password, salt []byte, iterations, lengthBits int) ([]byte, error) {
	lengthBytes := lengthBits / 8
	if lengthBits%8 != 0 {
		return nil, fmt.Errorf("PBKDF2: length must be a multiple of 8")
	}
	if iterations < 1 {
		return nil, fmt.Errorf("PBKDF2: iterations must be at least 1")
	}

	hashLen := hashFn().Size()
	numBlocks := (lengthBytes + hashLen - 1) / hashLen

	dk := make([]byte, 0, numBlocks*hashLen)
	for block := 1; block <= numBlocks; block++ {
		mac := hmac.New(hashFn, password)
		mac.Write(salt)
		mac.Write([]byte{byte(block >> 24), byte(block >> 16), byte(block >> 8), byte(block)})
		u := mac.Sum(nil)
		result := make([]byte, len(u))
		copy(result, u)

		for i := 2; i <= iterations; i++ {
			mac = hmac.New(hashFn, password)
			mac.Write(u)
			u = mac.Sum(nil)
			for j := range result {
				result[j] ^= u[j]
			}
		}
		dk = append(dk, result...)
	}

	return dk[:lengthBytes], nil
}

// cryptoDeriveJS adds deriveBits and deriveKey to crypto.subtle.
const cryptoDeriveJS = `
(function() {
var subtle = crypto.subtle;

subtle.deriveBits = async function(algorithm, baseKey, length) {
	var algo = typeof algorithm === 'string' ? { name: algorithm } : algorithm;
	var hashName = algo.hash ? (typeof algo.hash === 'string' ? algo.hash : algo.hash.name) : '';
	var saltB64 = algo.salt ? __bufferSourceToB64(algo.salt) : '';
	var infoB64 = algo.info ? __bufferSourceToB64(algo.info) : '';
	var iterations = algo.iterations || 0;
	var resultB64 = __cryptoDeriveBits(algo.name, baseKey._id, length, hashName, saltB64, infoB64, iterations);
	return __b64ToBuffer(resultB64);
};

subtle.deriveKey = async function(algorithm, baseKey, derivedKeyAlgorithm, extractable, usages) {
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
};

})();
`

// setupCryptoDerive registers HKDF and PBKDF2 deriveBits/deriveKey.
// Must run after setupCryptoExt.
func setupCryptoDerive(iso *v8.Isolate, ctx *v8.Context, _ *eventLoop) error {
	_ = ctx.Global().Set("__cryptoDeriveBits", v8.NewFunctionTemplate(iso, func(info *v8.FunctionCallbackInfo) *v8.Value {
		args := info.Args()
		if len(args) < 7 {
			return throwError(iso, "deriveBits requires 7 argument(s)")
		}
		algoName := args[0].String()
		keyID := args[1].Integer()
		lengthBits := int(args[2].Int32())
		hashName := args[3].String()
		saltB64 := args[4].String()
		infoB64 := args[5].String()
		iterations := int(args[6].Int32())

		reqID := getReqIDFromJS(ctx)
		entry := getCryptoKey(reqID, keyID)
		if entry == nil {
			return throwError(iso, "deriveBits: key not found")
		}

		hashFn := hashFuncFromAlgo(hashName)
		if hashFn == nil {
			return throwError(iso, fmt.Sprintf("deriveBits: unsupported hash %q", hashName))
		}

		switch normalizeAlgo(algoName) {
		case "HKDF":
			salt, err := base64.StdEncoding.DecodeString(saltB64)
			if err != nil {
				return throwError(iso, "deriveBits: invalid salt base64")
			}
			infoBytes, err := base64.StdEncoding.DecodeString(infoB64)
			if err != nil {
				return throwError(iso, "deriveBits: invalid info base64")
			}
			result, err := hkdfDeriveBits(hashFn, entry.data, salt, infoBytes, lengthBits)
			if err != nil {
				return throwError(iso, fmt.Sprintf("deriveBits: %s", err.Error()))
			}
			val, _ := v8.NewValue(iso, base64.StdEncoding.EncodeToString(result))
			return val

		case "PBKDF2":
			salt, err := base64.StdEncoding.DecodeString(saltB64)
			if err != nil {
				return throwError(iso, "deriveBits: invalid salt base64")
			}
			result, err := pbkdf2DeriveBits(hashFn, entry.data, salt, iterations, lengthBits)
			if err != nil {
				return throwError(iso, fmt.Sprintf("deriveBits: %s", err.Error()))
			}
			val, _ := v8.NewValue(iso, base64.StdEncoding.EncodeToString(result))
			return val

		default:
			return throwError(iso, fmt.Sprintf("deriveBits: unsupported algorithm %q", algoName))
		}
	}).GetFunction(ctx))

	if _, err := ctx.RunScript(cryptoDeriveJS, "crypto_derive.js"); err != nil {
		return fmt.Errorf("evaluating crypto_derive.js: %w", err)
	}
	return nil
}
