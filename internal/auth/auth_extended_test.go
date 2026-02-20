package auth

import (
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// JWT Extended Tests

func TestGenerateToken_EmptyClaims(t *testing.T) {
	secret := "test-secret"
	token, err := GenerateToken("", "", "", secret)
	if err != nil {
		t.Fatalf("GenerateToken with empty claims: %v", err)
	}

	claims, err := ValidateToken(token, secret)
	if err != nil {
		t.Fatalf("ValidateToken: %v", err)
	}

	if claims.UserID != "" {
		t.Errorf("UserID = %q, want empty string", claims.UserID)
	}
	if claims.Email != "" {
		t.Errorf("Email = %q, want empty string", claims.Email)
	}
	if claims.Role != "" {
		t.Errorf("Role = %q, want empty string", claims.Role)
	}
}

func TestGenerateToken_LongClaims(t *testing.T) {
	secret := "test-secret"
	longString := strings.Repeat("a", 1000)
	token, err := GenerateToken(longString, longString+"@example.com", longString, secret)
	if err != nil {
		t.Fatalf("GenerateToken with long claims: %v", err)
	}

	claims, err := ValidateToken(token, secret)
	if err != nil {
		t.Fatalf("ValidateToken: %v", err)
	}

	if claims.UserID != longString {
		t.Errorf("UserID length = %d, want %d", len(claims.UserID), len(longString))
	}
}

func TestValidateToken_EmptyToken(t *testing.T) {
	_, err := ValidateToken("", "secret")
	if err == nil {
		t.Fatal("expected error for empty token")
	}
}

func TestValidateToken_EmptySecret(t *testing.T) {
	token, err := GenerateToken("user1", "test@example.com", "user", "secret")
	if err != nil {
		t.Fatalf("GenerateToken: %v", err)
	}

	// Validating with empty secret should fail
	if _, err := ValidateToken(token, ""); err == nil {
		t.Fatal("expected error when validating with empty secret")
	}
}

func TestValidateToken_WrongSigningMethod(t *testing.T) {
	// Create a token with RS256 instead of HS256
	claims := Claims{
		UserID: "user1",
		Email:  "test@example.com",
		Role:   "user",
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(1 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}

	// Create token with None algorithm (unsigned)
	token := jwt.NewWithClaims(jwt.SigningMethodNone, claims)
	tokenString, err := token.SignedString(jwt.UnsafeAllowNoneSignatureType)
	if err != nil {
		t.Fatalf("creating unsigned token: %v", err)
	}

	// Should fail validation due to wrong signing method
	if _, err := ValidateToken(tokenString, "secret"); err == nil {
		t.Error("expected error for unsigned token")
	}
}

func TestValidateToken_TokenWithoutExpiry(t *testing.T) {
	secret := "test-secret"
	// Create token without expiry
	claims := Claims{
		UserID: "user1",
		Email:  "test@example.com",
		Role:   "user",
		RegisteredClaims: jwt.RegisteredClaims{
			IssuedAt: jwt.NewNumericDate(time.Now()),
			// No ExpiresAt set
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString([]byte(secret))
	if err != nil {
		t.Fatalf("creating token without expiry: %v", err)
	}

	// Should still be valid (no expiry means it doesn't expire)
	validatedClaims, err := ValidateToken(tokenString, secret)
	if err != nil {
		t.Errorf("ValidateToken for token without expiry: %v", err)
	}
	if validatedClaims.UserID != "user1" {
		t.Errorf("UserID = %q, want user1", validatedClaims.UserID)
	}
}

func TestValidateToken_TamperedPayload(t *testing.T) {
	secret := "test-secret"
	token, err := GenerateToken("user1", "test@example.com", "user", secret)
	if err != nil {
		t.Fatalf("GenerateToken: %v", err)
	}

	// Tamper with the token by changing a character in the payload section
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		t.Fatalf("expected 3 parts in JWT, got %d", len(parts))
	}

	// Change one character in the payload
	tamperedPayload := parts[1]
	if len(tamperedPayload) > 0 {
		// Replace last character
		tamperedPayload = tamperedPayload[:len(tamperedPayload)-1] + "X"
	}
	tamperedToken := parts[0] + "." + tamperedPayload + "." + parts[2]

	// Should fail validation
	if _, err := ValidateToken(tamperedToken, secret); err == nil {
		t.Error("expected error for tampered token")
	}
}

func TestValidateToken_TamperedSignature(t *testing.T) {
	secret := "test-secret"
	token, err := GenerateToken("user1", "test@example.com", "user", secret)
	if err != nil {
		t.Fatalf("GenerateToken: %v", err)
	}

	// Tamper with the signature
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		t.Fatalf("expected 3 parts in JWT, got %d", len(parts))
	}

	tamperedSignature := parts[2]
	if len(tamperedSignature) > 0 {
		tamperedSignature = tamperedSignature[:len(tamperedSignature)-1] + "X"
	}
	tamperedToken := parts[0] + "." + parts[1] + "." + tamperedSignature

	// Should fail validation
	if _, err := ValidateToken(tamperedToken, secret); err == nil {
		t.Error("expected error for tampered signature")
	}
}

func TestGenerateToken_ConsistentFormat(t *testing.T) {
	secret := "test-secret"
	token, err := GenerateToken("user1", "test@example.com", "admin", secret)
	if err != nil {
		t.Fatalf("GenerateToken: %v", err)
	}

	// JWT should have exactly 3 parts separated by dots
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		t.Errorf("JWT parts = %d, want 3", len(parts))
	}

	// Each part should be base64-encoded (non-empty)
	for i, part := range parts {
		if part == "" {
			t.Errorf("JWT part %d is empty", i)
		}
	}
}

// API Key Extended Tests

func TestHashAPIKey_EmptyKey(t *testing.T) {
	hash := HashAPIKey("")
	// Should produce a valid SHA-256 hash even for empty input
	if len(hash) != 64 {
		t.Errorf("hash length = %d, want 64", len(hash))
	}
}

func TestHashAPIKey_DifferentInputs(t *testing.T) {
	h1 := HashAPIKey("hd_key1")
	h2 := HashAPIKey("hd_key2")
	if h1 == h2 {
		t.Error("different inputs should produce different hashes")
	}
}

func TestHashAPIKey_CaseSensitive(t *testing.T) {
	h1 := HashAPIKey("hd_ABC")
	h2 := HashAPIKey("hd_abc")
	if h1 == h2 {
		t.Error("hashing should be case-sensitive")
	}
}

func TestGenerateAPIKey_UniquenessCheck(t *testing.T) {
	// Generate multiple keys and ensure they're all unique
	keys := make(map[string]bool)
	hashes := make(map[string]bool)

	for i := 0; i < 100; i++ {
		rawKey, hash, err := GenerateAPIKey()
		if err != nil {
			t.Fatalf("GenerateAPIKey iteration %d: %v", i, err)
		}

		if keys[rawKey] {
			t.Errorf("duplicate raw key generated: %s", rawKey)
		}
		if hashes[hash] {
			t.Errorf("duplicate hash generated: %s", hash)
		}

		keys[rawKey] = true
		hashes[hash] = true
	}
}

func TestGenerateAPIKey_PrefixConsistency(t *testing.T) {
	for i := 0; i < 10; i++ {
		rawKey, _, err := GenerateAPIKey()
		if err != nil {
			t.Fatalf("GenerateAPIKey iteration %d: %v", i, err)
		}
		if !strings.HasPrefix(rawKey, "hd_") {
			t.Errorf("iteration %d: rawKey = %q, want hd_ prefix", i, rawKey)
		}
	}
}

func TestGenerateAPIKey_HexEncoding(t *testing.T) {
	rawKey, _, err := GenerateAPIKey()
	if err != nil {
		t.Fatalf("GenerateAPIKey: %v", err)
	}

	// Remove prefix and check if remainder is valid hex
	keyWithoutPrefix := strings.TrimPrefix(rawKey, "hd_")
	for _, ch := range keyWithoutPrefix {
		if !((ch >= '0' && ch <= '9') || (ch >= 'a' && ch <= 'f')) {
			t.Errorf("non-hex character %q in key %q", ch, rawKey)
		}
	}
}

func TestHashAPIKey_SHA256Format(t *testing.T) {
	hash := HashAPIKey("hd_test")
	// SHA-256 produces 32 bytes = 64 hex characters
	if len(hash) != 64 {
		t.Errorf("hash length = %d, want 64", len(hash))
	}

	// Verify it's valid hex
	for _, ch := range hash {
		if !((ch >= '0' && ch <= '9') || (ch >= 'a' && ch <= 'f')) {
			t.Errorf("non-hex character %q in hash %q", ch, hash)
		}
	}
}

func TestHashAPIKey_NoCollisions(t *testing.T) {
	// Test that similar inputs produce different hashes
	testCases := []string{
		"hd_test",
		"hd_test1",
		"hd_Test",
		"hd_tes",
		"hd_testx",
	}

	hashes := make(map[string]string)
	for _, input := range testCases {
		hash := HashAPIKey(input)
		if existing, found := hashes[hash]; found {
			t.Errorf("collision: %q and %q produce same hash %q", input, existing, hash)
		}
		hashes[hash] = input
	}
}
