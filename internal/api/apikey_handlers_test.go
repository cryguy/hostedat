package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/cryguy/hostedat/internal/auth"
	"github.com/cryguy/hostedat/internal/models"
	"github.com/labstack/echo/v4"
)

func setupAPIKeyTest(t *testing.T) (*APIKeyHandler, *testEnv, models.User, string) {
	t.Helper()
	env := setupTestEnv(t)
	user, token := env.createTestUser(t, "apikey@test.com", "password123", "user")
	h := &APIKeyHandler{DB: env.db}
	return h, env, user, token
}

// ──────────────────────────────────────────────
// Create API Key tests
// ──────────────────────────────────────────────

func TestCreateAPIKey_Success(t *testing.T) {
	h, _, user, _ := setupAPIKeyTest(t)

	e := echo.New()
	body, _ := json.Marshal(map[string]string{"name": "ci-key"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/keys", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set(contextKeyUserID, user.ID)
	c.Set(contextKeyEmail, user.Email)
	c.Set(contextKeyRole, user.Role)

	if err := h.Create(c); err != nil {
		t.Fatalf("Create returned error: %v", err)
	}

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201: %s", rec.Code, rec.Body.String())
	}

	var result map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}

	if result["key"] == nil || result["key"] == "" {
		t.Error("expected raw key in response")
	}
	if result["name"] != "ci-key" {
		t.Errorf("name = %v, want ci-key", result["name"])
	}
	if result["id"] == nil {
		t.Error("expected id in response")
	}

	// Verify key was stored
	rawKey, ok := result["key"].(string)
	if !ok {
		t.Fatal("key is not a string")
	}
	hash := auth.HashAPIKey(rawKey)
	var count int64
	h.DB.Model(&models.APIKey{}).Where("key_hash = ?", hash).Count(&count)
	if count != 1 {
		t.Errorf("key count = %d, want 1", count)
	}
}

func TestCreateAPIKey_EmptyName(t *testing.T) {
	h, _, user, _ := setupAPIKeyTest(t)

	e := echo.New()
	body, _ := json.Marshal(map[string]string{"name": ""})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/keys", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set(contextKeyUserID, user.ID)

	if err := h.Create(c); err != nil {
		t.Fatalf("Create returned error: %v", err)
	}

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

func TestCreateAPIKey_InvalidRequestBody(t *testing.T) {
	h, _, user, _ := setupAPIKeyTest(t)

	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/keys", bytes.NewReader([]byte("invalid json")))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set(contextKeyUserID, user.ID)

	if err := h.Create(c); err != nil {
		t.Fatalf("Create returned error: %v", err)
	}

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

// ──────────────────────────────────────────────
// List API Keys tests
// ──────────────────────────────────────────────

func TestListAPIKeys_Success(t *testing.T) {
	h, env, user, _ := setupAPIKeyTest(t)

	env.db.Create(&models.APIKey{UserID: user.ID, KeyHash: "hash1", Name: "key1"})
	env.db.Create(&models.APIKey{UserID: user.ID, KeyHash: "hash2", Name: "key2"})

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/keys", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set(contextKeyUserID, user.ID)

	if err := h.List(c); err != nil {
		t.Fatalf("List returned error: %v", err)
	}

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}

	var keys []models.APIKey
	if err := json.Unmarshal(rec.Body.Bytes(), &keys); err != nil {
		t.Fatal(err)
	}
	if len(keys) != 2 {
		t.Errorf("got %d keys, want 2", len(keys))
	}
}

func TestListAPIKeys_Empty(t *testing.T) {
	h, _, user, _ := setupAPIKeyTest(t)

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/keys", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set(contextKeyUserID, user.ID)

	if err := h.List(c); err != nil {
		t.Fatalf("List returned error: %v", err)
	}

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}

	var keys []models.APIKey
	if err := json.Unmarshal(rec.Body.Bytes(), &keys); err != nil {
		t.Fatal(err)
	}
	if len(keys) != 0 {
		t.Errorf("got %d keys, want 0", len(keys))
	}
}

func TestListAPIKeys_OnlyOwnKeys(t *testing.T) {
	h, env, user, _ := setupAPIKeyTest(t)

	otherUser, _ := env.createTestUser(t, "other@test.com", "password123", "user")
	env.db.Create(&models.APIKey{UserID: user.ID, KeyHash: "hash1", Name: "my-key"})
	env.db.Create(&models.APIKey{UserID: otherUser.ID, KeyHash: "hash2", Name: "other-key"})

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/keys", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set(contextKeyUserID, user.ID)

	if err := h.List(c); err != nil {
		t.Fatalf("List returned error: %v", err)
	}

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}

	var keys []models.APIKey
	if err := json.Unmarshal(rec.Body.Bytes(), &keys); err != nil {
		t.Fatal(err)
	}
	if len(keys) != 1 {
		t.Errorf("got %d keys, want 1 (only own keys)", len(keys))
	}
	if keys[0].Name != "my-key" {
		t.Errorf("key name = %s, want my-key", keys[0].Name)
	}
}

// ──────────────────────────────────────────────
// Delete API Key tests
// ──────────────────────────────────────────────

func TestDeleteAPIKey_Success(t *testing.T) {
	h, env, user, _ := setupAPIKeyTest(t)

	key := models.APIKey{UserID: user.ID, KeyHash: "hash", Name: "delete-me"}
	env.db.Create(&key)

	e := echo.New()
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/keys/"+key.ID, nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set(contextKeyUserID, user.ID)
	c.SetParamNames("id")
	c.SetParamValues(key.ID)

	if err := h.Delete(c); err != nil {
		t.Fatalf("Delete returned error: %v", err)
	}

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}

	var count int64
	env.db.Model(&models.APIKey{}).Where("id = ?", key.ID).Count(&count)
	if count != 0 {
		t.Error("key should be deleted")
	}
}

func TestDeleteAPIKey_NotFound(t *testing.T) {
	h, _, user, _ := setupAPIKeyTest(t)

	e := echo.New()
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/keys/nonexistent", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set(contextKeyUserID, user.ID)
	c.SetParamNames("id")
	c.SetParamValues("nonexistent")

	if err := h.Delete(c); err != nil {
		t.Fatalf("Delete returned error: %v", err)
	}

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}

func TestDeleteAPIKey_CannotDeleteOthersKey(t *testing.T) {
	h, env, user, _ := setupAPIKeyTest(t)

	otherUser, _ := env.createTestUser(t, "other@test.com", "password123", "user")
	key := models.APIKey{UserID: otherUser.ID, KeyHash: "hash", Name: "other-key"}
	env.db.Create(&key)

	e := echo.New()
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/keys/"+key.ID, nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set(contextKeyUserID, user.ID)
	c.SetParamNames("id")
	c.SetParamValues(key.ID)

	if err := h.Delete(c); err != nil {
		t.Fatalf("Delete returned error: %v", err)
	}

	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", rec.Code)
	}

	// Verify key was not deleted
	var count int64
	env.db.Model(&models.APIKey{}).Where("id = ?", key.ID).Count(&count)
	if count != 1 {
		t.Error("other user's key should not be deleted")
	}
}
