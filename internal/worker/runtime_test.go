package worker

import (
	"testing"
)

func TestRequestState_Lifecycle(t *testing.T) {
	env := &Env{
		Vars:    make(map[string]string),
		Secrets: make(map[string]string),
	}
	id := newRequestState(50, env)
	defer clearRequestState(id)

	state := getRequestState(id)
	if state == nil {
		t.Fatal("expected non-nil state")
	}
	if state.maxFetches != 50 {
		t.Errorf("maxFetches = %d, want 50", state.maxFetches)
	}
	if state.env != env {
		t.Error("env pointer mismatch")
	}
}

func TestRequestState_ClearReturnsState(t *testing.T) {
	id := newRequestState(10, nil)

	state := clearRequestState(id)
	if state == nil {
		t.Fatal("clearRequestState returned nil")
	}
	if state.maxFetches != 10 {
		t.Errorf("maxFetches = %d, want 10", state.maxFetches)
	}

	// After clear, get should return nil.
	if got := getRequestState(id); got != nil {
		t.Error("expected nil after clear")
	}
}

func TestRequestState_GetNonexistent(t *testing.T) {
	if got := getRequestState(999999999); got != nil {
		t.Error("expected nil for nonexistent ID")
	}
}

func TestRequestState_ClearNonexistent(t *testing.T) {
	if got := clearRequestState(999999998); got != nil {
		t.Error("expected nil for nonexistent ID")
	}
}

func TestCryptoKey_ImportAndGet(t *testing.T) {
	id := newRequestState(10, nil)
	defer clearRequestState(id)

	keyData := []byte("secret-key-material")
	keyID := importCryptoKey(id, "SHA-256", keyData)
	if keyID < 0 {
		t.Fatalf("importCryptoKey returned %d", keyID)
	}

	entry := getCryptoKey(id, keyID)
	if entry == nil {
		t.Fatal("getCryptoKey returned nil")
	}
	if string(entry.data) != "secret-key-material" {
		t.Errorf("data = %q", entry.data)
	}
	if entry.hashAlgo != "SHA-256" {
		t.Errorf("hashAlgo = %q, want SHA-256", entry.hashAlgo)
	}
}

func TestCryptoKey_GetWrongRequest(t *testing.T) {
	id1 := newRequestState(10, nil)
	defer clearRequestState(id1)
	id2 := newRequestState(10, nil)
	defer clearRequestState(id2)

	keyID := importCryptoKey(id1, "SHA-256", []byte("key"))

	// Should not find key in a different request's state.
	if got := getCryptoKey(id2, keyID); got != nil {
		t.Error("expected nil for wrong request ID")
	}
}

func TestCryptoKey_IncrementingIDs(t *testing.T) {
	id := newRequestState(10, nil)
	defer clearRequestState(id)

	k1 := importCryptoKey(id, "SHA-256", []byte("a"))
	k2 := importCryptoKey(id, "SHA-256", []byte("b"))
	k3 := importCryptoKey(id, "SHA-256", []byte("c"))

	if k2 != k1+1 || k3 != k2+1 {
		t.Errorf("expected incrementing IDs, got %d, %d, %d", k1, k2, k3)
	}
}

func TestAddLog(t *testing.T) {
	id := newRequestState(10, nil)
	defer clearRequestState(id)

	addLog(id, "log", "first message")
	addLog(id, "warn", "second message")
	addLog(id, "error", "third message")

	state := getRequestState(id)
	if len(state.logs) != 3 {
		t.Fatalf("log count = %d, want 3", len(state.logs))
	}

	expected := []struct{ level, msg string }{
		{"log", "first message"},
		{"warn", "second message"},
		{"error", "third message"},
	}
	for i, exp := range expected {
		if state.logs[i].Level != exp.level {
			t.Errorf("logs[%d].Level = %q, want %q", i, state.logs[i].Level, exp.level)
		}
		if state.logs[i].Message != exp.msg {
			t.Errorf("logs[%d].Message = %q, want %q", i, state.logs[i].Message, exp.msg)
		}
		if state.logs[i].Time.IsZero() {
			t.Errorf("logs[%d].Time is zero", i)
		}
	}
}

func TestAddLog_NonexistentRequest(t *testing.T) {
	// Should be a no-op, not panic.
	addLog(999999997, "log", "nobody home")
}

func TestImportCryptoKey_NonexistentRequest(t *testing.T) {
	keyID := importCryptoKey(999999996, "SHA-256", []byte("data"))
	if keyID != -1 {
		t.Errorf("importCryptoKey on missing request = %d, want -1", keyID)
	}
}

func TestGetCryptoKey_NilKeys(t *testing.T) {
	// Request exists but has no keys imported.
	id := newRequestState(10, nil)
	defer clearRequestState(id)

	if got := getCryptoKey(id, 1); got != nil {
		t.Error("expected nil for key ID on request with no keys")
	}
}

func TestHashFuncFromAlgo(t *testing.T) {
	tests := []struct {
		algo     string
		wantNil  bool
		hashSize int // expected digest size in bytes
	}{
		{"SHA-1", false, 20},
		{"sha1", false, 20},
		{"SHA-256", false, 32},
		{"sha256", false, 32},
		{"SHA-384", false, 48},
		{"sha-384", false, 48},
		{"SHA-512", false, 64},
		{"sha512", false, 64},
		{"", false, 32},           // empty defaults to SHA-256
		{"unknown-algo", true, 0}, // unsupported returns nil
	}

	for _, tt := range tests {
		t.Run(tt.algo, func(t *testing.T) {
			fn := hashFuncFromAlgo(tt.algo)
			if tt.wantNil {
				if fn != nil {
					t.Error("expected nil for unsupported algorithm")
				}
				return
			}
			if fn == nil {
				t.Fatal("expected non-nil hash function")
			}
			h := fn()
			if h.Size() != tt.hashSize {
				t.Errorf("hash size = %d, want %d", h.Size(), tt.hashSize)
			}
		})
	}
}

func TestCryptoHashFromAlgo(t *testing.T) {
	tests := []struct {
		algo string
		want int // crypto.Hash value (0 means unsupported)
	}{
		{"SHA-1", 3},   // crypto.SHA1 = 3
		{"sha-1", 3},
		{"SHA-256", 5}, // crypto.SHA256 = 5
		{"sha256", 5},
		{"SHA-384", 6}, // crypto.SHA384 = 6
		{"SHA-512", 7}, // crypto.SHA512 = 7
		{"MD5", 0},     // unsupported
		{"", 0},
	}

	for _, tt := range tests {
		t.Run(tt.algo, func(t *testing.T) {
			got := cryptoHashFromAlgo(tt.algo)
			if int(got) != tt.want {
				t.Errorf("cryptoHashFromAlgo(%q) = %d, want %d", tt.algo, got, tt.want)
			}
		})
	}
}

func TestRsaJWKAlg(t *testing.T) {
	tests := []struct {
		algo, hash, want string
	}{
		{"RSASSA-PKCS1-v1_5", "SHA-1", "RS1"},
		{"RSASSA-PKCS1-v1_5", "SHA-256", "RS256"},
		{"RSASSA-PKCS1-v1_5", "SHA-384", "RS384"},
		{"RSASSA-PKCS1-v1_5", "SHA-512", "RS512"},
		{"RSA-PSS", "SHA-256", "PS256"},
		{"RSA-PSS", "SHA-384", "PS384"},
		{"RSA-PSS", "SHA-512", "PS512"},
		{"RSA-OAEP", "SHA-1", "RSA-OAEP"},
		{"RSA-OAEP", "SHA-256", "RSA-OAEP-256"},
		{"RSA-OAEP", "SHA-384", "RSA-OAEP-384"},
		{"RSA-OAEP", "SHA-512", "RSA-OAEP-512"},
		// Case insensitive
		{"rsa-pss", "sha-256", "PS256"},
		{"rsassa-pkcs1-v1_5", "sha256", "RS256"},
		// Unsupported combinations
		{"RSA-PSS", "SHA-1", ""},
		{"HMAC", "SHA-256", ""},
	}

	for _, tt := range tests {
		t.Run(tt.algo+"_"+tt.hash, func(t *testing.T) {
			got := rsaJWKAlg(tt.algo, tt.hash)
			if got != tt.want {
				t.Errorf("rsaJWKAlg(%q, %q) = %q, want %q", tt.algo, tt.hash, got, tt.want)
			}
		})
	}
}

func TestPadBytes(t *testing.T) {
	tests := []struct {
		name   string
		input  []byte
		length int
		want   int // expected length of result
	}{
		{"no padding needed", []byte{1, 2, 3}, 3, 3},
		{"already longer", []byte{1, 2, 3, 4}, 3, 4},
		{"pad to 32", []byte{1, 2}, 32, 32},
		{"empty input", []byte{}, 4, 4},
		{"pad to 1", []byte{}, 1, 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := padBytes(tt.input, tt.length)
			if len(result) != tt.want {
				t.Errorf("len(padBytes) = %d, want %d", len(result), tt.want)
			}
			// Verify original bytes are at the end.
			if len(tt.input) > 0 && len(result) >= len(tt.input) {
				for i, b := range tt.input {
					pos := len(result) - len(tt.input) + i
					if result[pos] != b {
						t.Errorf("byte at pos %d = %d, want %d", pos, result[pos], b)
					}
				}
			}
			// Verify leading bytes are zero.
			if len(result) > len(tt.input) {
				for i := 0; i < len(result)-len(tt.input); i++ {
					if result[i] != 0 {
						t.Errorf("padding byte at %d = %d, want 0", i, result[i])
					}
				}
			}
		})
	}
}

func TestCurveFromName(t *testing.T) {
	tests := []struct {
		name    string
		wantNil bool
	}{
		{"P-256", false},
		{"P-384", false},
		{"P-521", true}, // not supported
		{"", true},
		{"invalid", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			curve := curveFromName(tt.name)
			if tt.wantNil && curve != nil {
				t.Errorf("curveFromName(%q) should return nil", tt.name)
			}
			if !tt.wantNil && curve == nil {
				t.Errorf("curveFromName(%q) should not return nil", tt.name)
			}
		})
	}
}

func TestImportCryptoKeyFull(t *testing.T) {
	id := newRequestState(10, nil)
	defer clearRequestState(id)

	entry := &cryptoKeyEntry{
		algoName: "ECDSA",
		hashAlgo: "SHA-256",
		keyType:  "public",
	}

	keyID := importCryptoKeyFull(id, entry)
	if keyID < 0 {
		t.Fatalf("importCryptoKeyFull returned %d", keyID)
	}

	got := getCryptoKey(id, keyID)
	if got == nil {
		t.Fatal("getCryptoKey returned nil")
	}
	if got.algoName != "ECDSA" {
		t.Errorf("algoName = %q, want ECDSA", got.algoName)
	}
}

func TestImportCryptoKeyFull_NonexistentRequest(t *testing.T) {
	keyID := importCryptoKeyFull(999999990, &cryptoKeyEntry{})
	if keyID != -1 {
		t.Errorf("importCryptoKeyFull on missing request = %d, want -1", keyID)
	}
}

func TestNormalizeAlgo(t *testing.T) {
	tests := []struct {
		input, want string
	}{
		{"sha-1", "SHA-1"},
		{"SHA-1", "SHA-1"},
		{"sha1", "SHA-1"},
		{"SHA1", "SHA-1"},
		{"sha-256", "SHA-256"},
		{"SHA-256", "SHA-256"},
		{"sha256", "SHA-256"},
		{"SHA256", "SHA-256"},
		{"sha-384", "SHA-384"},
		{"SHA-384", "SHA-384"},
		{"sha384", "SHA-384"},
		{"SHA384", "SHA-384"},
		{"sha-512", "SHA-512"},
		{"SHA-512", "SHA-512"},
		{"sha512", "SHA-512"},
		{"SHA512", "SHA-512"},
		// HMAC variants
		{"hmac", "HMAC"}, {"HMAC", "HMAC"}, {"Hmac", "HMAC"},
		// AES variants
		{"aes-gcm", "AES-GCM"}, {"AES-GCM", "AES-GCM"}, {"Aes-Gcm", "AES-GCM"},
		{"aes-cbc", "AES-CBC"}, {"AES-CBC", "AES-CBC"}, {"Aes-Cbc", "AES-CBC"},
		// ECDSA variants
		{"ecdsa", "ECDSA"}, {"ECDSA", "ECDSA"}, {"Ecdsa", "ECDSA"},
		// Key derivation
		{"hkdf", "HKDF"}, {"HKDF", "HKDF"}, {"Hkdf", "HKDF"},
		{"pbkdf2", "PBKDF2"}, {"PBKDF2", "PBKDF2"}, {"Pbkdf2", "PBKDF2"},
		// RSA variants
		{"rsa-oaep", "RSA-OAEP"}, {"RSA-OAEP", "RSA-OAEP"}, {"Rsa-Oaep", "RSA-OAEP"},
		{"rsassa-pkcs1-v1_5", "RSASSA-PKCS1-v1_5"}, {"RSASSA-PKCS1-v1_5", "RSASSA-PKCS1-v1_5"}, {"RSASSA-PKCS1-V1_5", "RSASSA-PKCS1-v1_5"},
		{"rsa-pss", "RSA-PSS"}, {"RSA-PSS", "RSA-PSS"}, {"Rsa-Pss", "RSA-PSS"},
		// Ed25519 variants
		{"ed25519", "Ed25519"}, {"Ed25519", "Ed25519"}, {"ED25519", "Ed25519"},
		// Unknown passthrough
		{"unknown", "unknown"}, // passthrough
		{"", ""},               // passthrough
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := normalizeAlgo(tt.input)
			if got != tt.want {
				t.Errorf("normalizeAlgo(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
