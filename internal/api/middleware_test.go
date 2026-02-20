package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/cryguy/hostedat/internal/auth"
	"github.com/cryguy/hostedat/internal/models"
	"github.com/labstack/echo/v4"
)

// ──────────────────────────────────────────────
// AuthMiddleware tests
// ──────────────────────────────────────────────

func TestAuthMiddleware_NoHeader(t *testing.T) {
	env := setupTestEnv(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/sites", nil)
	rec := env.doRequest(req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
}

func TestAuthMiddleware_InvalidFormat(t *testing.T) {
	env := setupTestEnv(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/sites", nil)
	req.Header.Set("Authorization", "InvalidFormat token123")
	rec := env.doRequest(req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
}

func TestAuthMiddleware_ValidJWT(t *testing.T) {
	env := setupTestEnv(t)
	_, token := env.createTestUser(t, "jwt@test.com", "password123", "user")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/sites", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := env.doRequest(req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200 (valid JWT)", rec.Code)
	}
}

func TestAuthMiddleware_InvalidJWT(t *testing.T) {
	env := setupTestEnv(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/sites", nil)
	req.Header.Set("Authorization", "Bearer invalid.jwt.token")
	rec := env.doRequest(req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
}

func TestAuthMiddleware_RevokedJWT(t *testing.T) {
	env := setupTestEnv(t)
	_, token := env.createTestUser(t, "revoked@test.com", "password123", "user")

	claims, _ := auth.ValidateToken(token, env.jwtSecret)
	RevokeToken(env.db, token, claims.ExpiresAt.Time)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/sites", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := env.doRequest(req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401 (revoked token)", rec.Code)
	}
}

func TestAuthMiddleware_DeletedUser(t *testing.T) {
	env := setupTestEnv(t)
	user, token := env.createTestUser(t, "deleted@test.com", "password123", "user")

	// Delete user
	env.db.Delete(&user)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/sites", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := env.doRequest(req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401 (deleted user)", rec.Code)
	}
}

func TestAuthMiddleware_ValidAPIKey(t *testing.T) {
	env := setupTestEnv(t)
	user, _ := env.createTestUser(t, "apikey@test.com", "password123", "user")

	rawKey, hash, _ := auth.GenerateAPIKey()
	key := models.APIKey{UserID: user.ID, KeyHash: hash, Name: "test"}
	env.db.Create(&key)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/sites", nil)
	req.Header.Set("Authorization", "Bearer "+rawKey)
	rec := env.doRequest(req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200 (valid API key)", rec.Code)
	}

	// Verify last_used_at was updated
	var updated models.APIKey
	env.db.First(&updated, "id = ?", key.ID)
	if updated.LastUsedAt == nil {
		t.Error("last_used_at should be set")
	}
}

func TestAuthMiddleware_InvalidAPIKey(t *testing.T) {
	env := setupTestEnv(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/sites", nil)
	req.Header.Set("Authorization", "Bearer hd_invalid_key")
	rec := env.doRequest(req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
}

func TestAuthMiddleware_APIKeyDeletedUser(t *testing.T) {
	env := setupTestEnv(t)
	user, _ := env.createTestUser(t, "apikey@test.com", "password123", "user")

	rawKey, hash, _ := auth.GenerateAPIKey()
	key := models.APIKey{UserID: user.ID, KeyHash: hash, Name: "test"}
	env.db.Create(&key)

	// Delete user
	env.db.Delete(&user)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/sites", nil)
	req.Header.Set("Authorization", "Bearer "+rawKey)
	rec := env.doRequest(req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401 (deleted user)", rec.Code)
	}
}

// ──────────────────────────────────────────────
// RequireRole tests
// ──────────────────────────────────────────────

func TestRequireRole_HasRequiredRole(t *testing.T) {
	env := setupTestEnv(t)
	_, token := env.createTestUser(t, "admin@test.com", "password123", "admin")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/users", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := env.doRequest(req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200 (has admin role)", rec.Code)
	}
}

func TestRequireRole_LacksRequiredRole(t *testing.T) {
	env := setupTestEnv(t)
	_, token := env.createTestUser(t, "user@test.com", "password123", "user")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/users", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := env.doRequest(req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", rec.Code)
	}
}

func TestRequireAdmin_Superadmin(t *testing.T) {
	env := setupTestEnv(t)
	_, token := env.createTestUser(t, "superadmin@test.com", "password123", "superadmin")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/users", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := env.doRequest(req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200 (superadmin has admin access)", rec.Code)
	}
}

func TestRequireAdmin_Admin(t *testing.T) {
	env := setupTestEnv(t)
	env.createTestUser(t, "superadmin@test.com", "password123", "superadmin")
	_, token := env.createTestUser(t, "admin@test.com", "password123", "admin")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/users", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := env.doRequest(req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200 (admin has admin access)", rec.Code)
	}
}

// ──────────────────────────────────────────────
// VersionCheckMiddleware tests
// ──────────────────────────────────────────────

func TestVersionCheck_NoMinVersion(t *testing.T) {
	env := setupTestEnvWithMinVersion(t, "")
	_, token := env.createTestUser(t, "user@test.com", "password123", "user")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/sites", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-Hostedat-Version", "0.0.1")
	rec := env.doRequest(req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200 (no min version enforced)", rec.Code)
	}
}

func TestVersionCheck_NoClientVersion(t *testing.T) {
	env := setupTestEnvWithMinVersion(t, "1.0.0")
	_, token := env.createTestUser(t, "user@test.com", "password123", "user")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/sites", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := env.doRequest(req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200 (no version header, skip check)", rec.Code)
	}
}

func TestVersionCheck_SufficientVersion(t *testing.T) {
	env := setupTestEnvWithMinVersion(t, "1.0.0")
	_, token := env.createTestUser(t, "user@test.com", "password123", "user")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/sites", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-Hostedat-Version", "1.5.0")
	rec := env.doRequest(req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200 (version meets minimum)", rec.Code)
	}
}

func TestVersionCheck_InsufficientVersion(t *testing.T) {
	env := setupTestEnvWithMinVersion(t, "2.0.0")
	_, token := env.createTestUser(t, "user@test.com", "password123", "user")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/sites", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-Hostedat-Version", "1.0.0")
	rec := env.doRequest(req)

	if rec.Code != http.StatusUpgradeRequired {
		t.Errorf("status = %d, want 426", rec.Code)
	}
}

// ──────────────────────────────────────────────
// Token revocation tests
// ──────────────────────────────────────────────

func TestRevokeToken_Success(t *testing.T) {
	env := setupTestEnv(t)
	_, token := env.createTestUser(t, "revoke@test.com", "password123", "user")

	expiresAt := time.Now().Add(24 * time.Hour)
	if err := RevokeToken(env.db, token, expiresAt); err != nil {
		t.Fatalf("RevokeToken failed: %v", err)
	}

	if !IsTokenRevoked(env.db, token) {
		t.Error("token should be revoked")
	}
}

func TestHashToken_Consistency(t *testing.T) {
	token := "test-token-123"
	hash1 := HashToken(token)
	hash2 := HashToken(token)

	if hash1 != hash2 {
		t.Error("HashToken should produce consistent results")
	}
	if hash1 == token {
		t.Error("hash should not equal input")
	}
}

func TestIsTokenRevoked_NotRevoked(t *testing.T) {
	env := setupTestEnv(t)
	_, token := env.createTestUser(t, "active@test.com", "password123", "user")

	if IsTokenRevoked(env.db, token) {
		t.Error("fresh token should not be revoked")
	}
}

func TestCleanExpiredTokens_RemovesExpired(t *testing.T) {
	env := setupTestEnv(t)

	expiredTime := time.Now().Add(-1 * time.Hour)
	env.db.Create(&models.RevokedToken{TokenHash: "expired", ExpiresAt: expiredTime})

	futureTime := time.Now().Add(1 * time.Hour)
	env.db.Create(&models.RevokedToken{TokenHash: "active", ExpiresAt: futureTime})

	CleanExpiredTokens(env.db)

	var count int64
	env.db.Model(&models.RevokedToken{}).Where("token_hash = ?", "expired").Count(&count)
	if count != 0 {
		t.Error("expired token should be cleaned")
	}

	env.db.Model(&models.RevokedToken{}).Where("token_hash = ?", "active").Count(&count)
	if count != 1 {
		t.Error("active token should remain")
	}
}

// ──────────────────────────────────────────────
// GetUserFromContext tests
// ──────────────────────────────────────────────

func TestGetUserFromContext_Success(t *testing.T) {
	env := setupTestEnv(t)
	user, token := env.createTestUser(t, "context@test.com", "password123", "user")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/sites", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()

	handlerCalled := false
	env.e.GET("/test-context", func(c echo.Context) error {
		handlerCalled = true
		userID, email, role := GetUserFromContext(c)
		if userID != user.ID {
			t.Errorf("userID = %s, want %s", userID, user.ID)
		}
		if email != user.Email {
			t.Errorf("email = %s, want %s", email, user.Email)
		}
		if role != user.Role {
			t.Errorf("role = %s, want %s", role, user.Role)
		}
		return c.NoContent(http.StatusOK)
	}, AuthMiddleware(env.db, env.jwtSecret))

	req = httptest.NewRequest(http.MethodGet, "/test-context", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	env.e.ServeHTTP(rec, req)

	if !handlerCalled {
		t.Error("handler was not called")
	}
}

func TestGetUserFromContext_Empty(t *testing.T) {
	env := setupTestEnv(t)

	handlerCalled := false
	env.e.GET("/test-empty-context", func(c echo.Context) error {
		handlerCalled = true
		userID, email, role := GetUserFromContext(c)
		if userID != "" || email != "" || role != "" {
			t.Errorf("expected empty values, got userID=%s email=%s role=%s", userID, email, role)
		}
		return c.NoContent(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/test-empty-context", nil)
	rec := httptest.NewRecorder()
	env.e.ServeHTTP(rec, req)

	if !handlerCalled {
		t.Error("handler was not called")
	}
}
