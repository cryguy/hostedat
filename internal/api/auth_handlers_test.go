package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/cryguy/hostedat/internal/auth"
	"github.com/cryguy/hostedat/internal/models"
)

// ──────────────────────────────────────────────
// Register tests (additional coverage)
// ──────────────────────────────────────────────

func TestRegister_MalformedEmail(t *testing.T) {
	env := setupTestEnv(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register",
		jsonBody(map[string]string{"email": "not-an-email", "password": "password123"}))
	req.Header.Set("Content-Type", "application/json")
	rec := env.doRequest(req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

func TestRegister_EmptyEmail(t *testing.T) {
	env := setupTestEnv(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register",
		jsonBody(map[string]string{"email": "", "password": "password123"}))
	req.Header.Set("Content-Type", "application/json")
	rec := env.doRequest(req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

func TestRegister_EmptyPassword(t *testing.T) {
	env := setupTestEnv(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register",
		jsonBody(map[string]string{"email": "test@test.com", "password": ""}))
	req.Header.Set("Content-Type", "application/json")
	rec := env.doRequest(req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

func TestRegister_InvalidInviteCode(t *testing.T) {
	env := setupTestEnv(t)
	env.createTestUser(t, "admin@test.com", "password123", "superadmin")
	if err := models.SetSetting(env.db, "invite_required", "true"); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register",
		jsonBody(map[string]string{"email": "new@test.com", "password": "password123", "invite_code": "invalid"}))
	req.Header.Set("Content-Type", "application/json")
	rec := env.doRequest(req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

func TestRegister_ExpiredInvite(t *testing.T) {
	env := setupTestEnv(t)
	admin, _ := env.createTestUser(t, "admin@test.com", "password123", "superadmin")
	if err := models.SetSetting(env.db, "invite_required", "true"); err != nil {
		t.Fatal(err)
	}

	expiredTime := time.Now().Add(-1 * time.Hour)
	invite := models.Invite{
		Code:      "expired",
		CreatedBy: admin.ID,
		Active:    true,
		ExpiresAt: &expiredTime,
	}
	env.db.Create(&invite)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register",
		jsonBody(map[string]string{"email": "new@test.com", "password": "password123", "invite_code": "expired"}))
	req.Header.Set("Content-Type", "application/json")
	rec := env.doRequest(req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

func TestRegister_MaxUsesReached(t *testing.T) {
	env := setupTestEnv(t)
	admin, _ := env.createTestUser(t, "admin@test.com", "password123", "superadmin")
	if err := models.SetSetting(env.db, "invite_required", "true"); err != nil {
		t.Fatal(err)
	}

	maxUses := 1
	invite := models.Invite{
		Code:      "maxed",
		CreatedBy: admin.ID,
		Active:    true,
		MaxUses:   &maxUses,
		UseCount:  1,
	}
	env.db.Create(&invite)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register",
		jsonBody(map[string]string{"email": "new@test.com", "password": "password123", "invite_code": "maxed"}))
	req.Header.Set("Content-Type", "application/json")
	rec := env.doRequest(req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

func TestRegister_InvalidRequestBody(t *testing.T) {
	env := setupTestEnv(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", nil)
	req.Header.Set("Content-Type", "application/json")
	rec := env.doRequest(req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

// ──────────────────────────────────────────────
// Login tests (additional coverage)
// ──────────────────────────────────────────────

func TestLogin_EmptyCredentials(t *testing.T) {
	env := setupTestEnv(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login",
		jsonBody(map[string]string{"email": "", "password": ""}))
	req.Header.Set("Content-Type", "application/json")
	rec := env.doRequest(req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

func TestLogin_InvalidRequestBody(t *testing.T) {
	env := setupTestEnv(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", nil)
	req.Header.Set("Content-Type", "application/json")
	rec := env.doRequest(req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

// ──────────────────────────────────────────────
// Logout tests
// ──────────────────────────────────────────────

func TestLogout_RevokesJWT(t *testing.T) {
	env := setupTestEnv(t)
	_, token := env.createTestUser(t, "logout@test.com", "password123", "user")

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := env.doRequest(req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}

	// Verify token is revoked
	if !IsTokenRevoked(env.db, token) {
		t.Error("token should be revoked after logout")
	}
}

func TestLogout_WithoutToken(t *testing.T) {
	env := setupTestEnv(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
	rec := env.doRequest(req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200 (logout always succeeds)", rec.Code)
	}
}

func TestLogout_WithAPIKey(t *testing.T) {
	env := setupTestEnv(t)
	user, _ := env.createTestUser(t, "apikey@test.com", "password123", "user")

	rawKey, hash, _ := auth.GenerateAPIKey()
	key := models.APIKey{UserID: user.ID, KeyHash: hash, Name: "test"}
	env.db.Create(&key)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
	req.Header.Set("Authorization", "Bearer "+rawKey)
	rec := env.doRequest(req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
}

// ──────────────────────────────────────────────
// CLI Login tests
// ──────────────────────────────────────────────

func TestCLILogin_Success(t *testing.T) {
	env := setupTestEnv(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/cli?port=8080&state=test-state&code_challenge=test123&code_challenge_method=S256", nil)
	rec := env.doRequest(req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("Content-Type") != "text/html; charset=UTF-8" {
		t.Errorf("content-type = %q, want text/html", rec.Header().Get("Content-Type"))
	}
}

func TestCLILogin_MissingPort(t *testing.T) {
	env := setupTestEnv(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/cli?state=test-state&code_challenge=test123&code_challenge_method=S256", nil)
	rec := env.doRequest(req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

func TestCLILogin_MissingState(t *testing.T) {
	env := setupTestEnv(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/cli?port=8080&code_challenge=test123&code_challenge_method=S256", nil)
	rec := env.doRequest(req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

func TestCLILogin_NonNumericPort(t *testing.T) {
	env := setupTestEnv(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/cli?port=abc&state=test-state&code_challenge=test123&code_challenge_method=S256", nil)
	rec := env.doRequest(req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

func TestCLILogin_PortOutOfRange(t *testing.T) {
	env := setupTestEnv(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/cli?port=99999&state=test-state&code_challenge=test123&code_challenge_method=S256", nil)
	rec := env.doRequest(req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

func TestCLILogin_MissingCodeChallenge(t *testing.T) {
	env := setupTestEnv(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/cli?port=8080&state=test-state&code_challenge_method=S256", nil)
	rec := env.doRequest(req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

func TestCLILogin_WrongCodeChallengeMethod(t *testing.T) {
	env := setupTestEnv(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/cli?port=8080&state=test-state&code_challenge=test123&code_challenge_method=plain", nil)
	rec := env.doRequest(req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

// ──────────────────────────────────────────────
// CLI Login Submit tests
// ──────────────────────────────────────────────

func TestCLILoginSubmit_Success(t *testing.T) {
	env := setupTestEnv(t)
	env.createTestUser(t, "cli@test.com", "password123", "user")

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/cli",
		jsonBody(map[string]string{
			"email":          "cli@test.com",
			"password":       "password123",
			"port":           "8080",
			"state":          "test-state",
			"code_challenge": "test123",
		}))
	req.Header.Set("Content-Type", "application/json")
	rec := env.doRequest(req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}

	body := parseJSON(t, rec)
	redirect, ok := body["redirect"].(string)
	if !ok || redirect == "" {
		t.Fatal("expected redirect URL in response")
	}
}

func TestCLILoginSubmit_InvalidCredentials(t *testing.T) {
	env := setupTestEnv(t)
	env.createTestUser(t, "cli@test.com", "password123", "user")

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/cli",
		jsonBody(map[string]string{
			"email":          "cli@test.com",
			"password":       "wrong",
			"port":           "8080",
			"state":          "test-state",
			"code_challenge": "test123",
		}))
	req.Header.Set("Content-Type", "application/json")
	rec := env.doRequest(req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
}

func TestCLILoginSubmit_MissingFields(t *testing.T) {
	env := setupTestEnv(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/cli",
		jsonBody(map[string]string{"email": "test@test.com"}))
	req.Header.Set("Content-Type", "application/json")
	rec := env.doRequest(req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

func TestCLILoginSubmit_NonNumericPort(t *testing.T) {
	env := setupTestEnv(t)
	env.createTestUser(t, "cli@test.com", "password123", "user")

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/cli",
		jsonBody(map[string]string{
			"email":          "cli@test.com",
			"password":       "password123",
			"port":           "invalid",
			"state":          "test-state",
			"code_challenge": "test123",
		}))
	req.Header.Set("Content-Type", "application/json")
	rec := env.doRequest(req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

// ──────────────────────────────────────────────
// Token Exchange tests
// ──────────────────────────────────────────────

func TestTokenExchange_Success(t *testing.T) {
	env := setupTestEnv(t)
	user, _ := env.createTestUser(t, "exchange@test.com", "password123", "user")

	codeVerifier := "test-verifier"
	codeChallenge := auth.ComputeCodeChallenge(codeVerifier)

	code := "test-auth-code"
	authCode := models.AuthCode{
		Code:          code,
		UserID:        user.ID,
		CodeChallenge: codeChallenge,
		ExpiresAt:     time.Now().Add(60 * time.Second),
		Used:          false,
	}
	env.db.Create(&authCode)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/token",
		jsonBody(map[string]string{
			"code":          code,
			"code_verifier": codeVerifier,
		}))
	req.Header.Set("Content-Type", "application/json")
	rec := env.doRequest(req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}

	body := parseJSON(t, rec)
	if body["token"] == nil || body["token"] == "" {
		t.Fatal("expected token in response")
	}
}

func TestTokenExchange_InvalidCode(t *testing.T) {
	env := setupTestEnv(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/token",
		jsonBody(map[string]string{
			"code":          "invalid",
			"code_verifier": "verifier",
		}))
	req.Header.Set("Content-Type", "application/json")
	rec := env.doRequest(req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
}

func TestTokenExchange_AlreadyUsed(t *testing.T) {
	env := setupTestEnv(t)
	user, _ := env.createTestUser(t, "exchange@test.com", "password123", "user")

	code := "used-code"
	authCode := models.AuthCode{
		Code:          code,
		UserID:        user.ID,
		CodeChallenge: "challenge",
		ExpiresAt:     time.Now().Add(60 * time.Second),
		Used:          true,
	}
	env.db.Create(&authCode)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/token",
		jsonBody(map[string]string{
			"code":          code,
			"code_verifier": "verifier",
		}))
	req.Header.Set("Content-Type", "application/json")
	rec := env.doRequest(req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
}

func TestTokenExchange_Expired(t *testing.T) {
	env := setupTestEnv(t)
	user, _ := env.createTestUser(t, "exchange@test.com", "password123", "user")

	code := "expired-code"
	authCode := models.AuthCode{
		Code:          code,
		UserID:        user.ID,
		CodeChallenge: "challenge",
		ExpiresAt:     time.Now().Add(-60 * time.Second),
		Used:          false,
	}
	env.db.Create(&authCode)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/token",
		jsonBody(map[string]string{
			"code":          code,
			"code_verifier": "verifier",
		}))
	req.Header.Set("Content-Type", "application/json")
	rec := env.doRequest(req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
}

func TestTokenExchange_WrongVerifier(t *testing.T) {
	env := setupTestEnv(t)
	user, _ := env.createTestUser(t, "exchange@test.com", "password123", "user")

	codeChallenge := auth.ComputeCodeChallenge("correct-verifier")
	code := "test-code"
	authCode := models.AuthCode{
		Code:          code,
		UserID:        user.ID,
		CodeChallenge: codeChallenge,
		ExpiresAt:     time.Now().Add(60 * time.Second),
		Used:          false,
	}
	env.db.Create(&authCode)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/token",
		jsonBody(map[string]string{
			"code":          code,
			"code_verifier": "wrong-verifier",
		}))
	req.Header.Set("Content-Type", "application/json")
	rec := env.doRequest(req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
}

func TestTokenExchange_MissingFields(t *testing.T) {
	env := setupTestEnv(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/token",
		jsonBody(map[string]string{"code": "test"}))
	req.Header.Set("Content-Type", "application/json")
	rec := env.doRequest(req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

func TestTokenExchange_InvalidRequestBody(t *testing.T) {
	env := setupTestEnv(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/token", nil)
	req.Header.Set("Content-Type", "application/json")
	rec := env.doRequest(req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}
