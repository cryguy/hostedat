package auth

import (
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func TestHashPassword_and_CheckPassword(t *testing.T) {
	hash, err := HashPassword("correcthorse")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if err := CheckPassword("correcthorse", hash); err != nil {
		t.Fatalf("CheckPassword with correct password: %v", err)
	}
}

func TestCheckPassword_WrongPassword(t *testing.T) {
	hash, err := HashPassword("correcthorse")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if err := CheckPassword("wrongpassword", hash); err == nil {
		t.Fatal("expected error for wrong password")
	}
}

func TestGenerateToken_and_ValidateToken(t *testing.T) {
	secret := "test-secret-key"
	token, err := GenerateToken("user123", "test@example.com", "admin", secret)
	if err != nil {
		t.Fatalf("GenerateToken: %v", err)
	}

	claims, err := ValidateToken(token, secret)
	if err != nil {
		t.Fatalf("ValidateToken: %v", err)
	}

	if claims.UserID != "user123" {
		t.Errorf("UserID = %q, want %q", claims.UserID, "user123")
	}
	if claims.Email != "test@example.com" {
		t.Errorf("Email = %q, want %q", claims.Email, "test@example.com")
	}
	if claims.Role != "admin" {
		t.Errorf("Role = %q, want %q", claims.Role, "admin")
	}
}

func TestValidateToken_WrongSecret(t *testing.T) {
	token, err := GenerateToken("u1", "a@b.com", "user", "secret-a")
	if err != nil {
		t.Fatalf("GenerateToken: %v", err)
	}
	if _, err := ValidateToken(token, "secret-b"); err == nil {
		t.Fatal("expected error for wrong secret")
	}
}

func TestValidateToken_MalformedToken(t *testing.T) {
	if _, err := ValidateToken("not.a.jwt", "secret"); err == nil {
		t.Fatal("expected error for malformed token")
	}
}

func TestGenerateAPIKey(t *testing.T) {
	rawKey, hash, err := GenerateAPIKey()
	if err != nil {
		t.Fatalf("GenerateAPIKey: %v", err)
	}

	if !strings.HasPrefix(rawKey, "hd_") {
		t.Errorf("rawKey prefix = %q, want hd_", rawKey[:3])
	}
	// hd_ (3) + 30 bytes hex (60) = 63
	if len(rawKey) != 63 {
		t.Errorf("len(rawKey) = %d, want 63", len(rawKey))
	}
	// SHA-256 hex = 64
	if len(hash) != 64 {
		t.Errorf("len(hash) = %d, want 64", len(hash))
	}
}

func TestHashAPIKey_Deterministic(t *testing.T) {
	h1 := HashAPIKey("hd_abc123")
	h2 := HashAPIKey("hd_abc123")
	if h1 != h2 {
		t.Errorf("HashAPIKey not deterministic: %q != %q", h1, h2)
	}
}

func TestGenerateAPIKey_HashMatches(t *testing.T) {
	rawKey, hash, err := GenerateAPIKey()
	if err != nil {
		t.Fatalf("GenerateAPIKey: %v", err)
	}
	recomputed := HashAPIKey(rawKey)
	if hash != recomputed {
		t.Errorf("hash mismatch: GenerateAPIKey gave %q, HashAPIKey gave %q", hash, recomputed)
	}
}

func TestHashPassword_EmptyPassword(t *testing.T) {
	hash, err := HashPassword("")
	if err != nil {
		t.Fatalf("HashPassword with empty string: %v", err)
	}
	if err := CheckPassword("", hash); err != nil {
		t.Fatalf("CheckPassword with empty password: %v", err)
	}
	if err := CheckPassword("nonempty", hash); err == nil {
		t.Error("expected error when checking nonempty password against empty hash")
	}
}

func TestHashPassword_LongPassword(t *testing.T) {
	// bcrypt has a 72-byte limit, so test with a 72-byte password
	longPw := strings.Repeat("a", 72)
	hash, err := HashPassword(longPw)
	if err != nil {
		t.Fatalf("HashPassword with long password: %v", err)
	}
	if err := CheckPassword(longPw, hash); err != nil {
		t.Fatalf("CheckPassword with long password: %v", err)
	}
	// Verify that a wrong password still fails
	if err := CheckPassword(strings.Repeat("b", 72), hash); err == nil {
		t.Error("expected error for wrong password")
	}
}

func TestValidateToken_ExpiredToken(t *testing.T) {
	secret := "test-secret"
	// Create a token that's already expired by using a custom Claims with past expiry
	claims := Claims{
		UserID: "user1",
		Email:  "test@example.com",
		Role:   "user",
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(-1 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now().Add(-2 * time.Hour)),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString([]byte(secret))
	if err != nil {
		t.Fatalf("creating expired token: %v", err)
	}

	if _, err := ValidateToken(tokenString, secret); err == nil {
		t.Error("expected error for expired token")
	}
}
