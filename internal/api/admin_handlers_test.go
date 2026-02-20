package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/cryguy/hostedat/internal/models"
	"github.com/labstack/echo/v4"
)

func setupAdminTest(t *testing.T) (*AdminHandler, *testEnv, models.User, string) {
	t.Helper()
	env := setupTestEnv(t)
	admin, token := env.createTestUser(t, "admin@test.com", "password123", "superadmin")
	h := &AdminHandler{DB: env.db, Storage: env.store}
	return h, env, admin, token
}

// ──────────────────────────────────────────────
// ListUsers tests
// ──────────────────────────────────────────────

func TestListUsers_Success(t *testing.T) {
	_, env, _, token := setupAdminTest(t)
	env.createTestUser(t, "user1@test.com", "password123", "user")
	env.createTestUser(t, "user2@test.com", "password123", "user")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/users", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := env.doRequest(req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}

	body := parseJSON(t, rec)
	users, ok := body["users"].([]interface{})
	if !ok {
		t.Fatal("expected users array in response")
	}
	if len(users) != 3 {
		t.Errorf("got %d users, want 3", len(users))
	}
}

func TestListUsers_Pagination(t *testing.T) {
	_, env, _, token := setupAdminTest(t)
	for i := 0; i < 25; i++ {
		env.createTestUser(t, "user"+string(rune(i))+"@test.com", "password123", "user")
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/users?page=2", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := env.doRequest(req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}

	body := parseJSON(t, rec)
	page := body["page"].(float64)
	if page != 2 {
		t.Errorf("page = %v, want 2", page)
	}
}

func TestListUsers_RequiresAdmin(t *testing.T) {
	env := setupTestEnv(t)
	_, token := env.createTestUser(t, "user@test.com", "password123", "user")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/users", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := env.doRequest(req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", rec.Code)
	}
}

// ──────────────────────────────────────────────
// UpdateUserRole tests
// ──────────────────────────────────────────────

func TestUpdateUserRole_Success(t *testing.T) {
	_, env, _, token := setupAdminTest(t)
	target, _ := env.createTestUser(t, "target@test.com", "password123", "user")

	req := httptest.NewRequest(http.MethodPatch, "/api/v1/admin/users/"+target.ID+"/role",
		jsonBody(map[string]string{"role": "admin"}))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	rec := env.doRequest(req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}

	var updated models.User
	env.db.First(&updated, "id = ?", target.ID)
	if updated.Role != "admin" {
		t.Errorf("role = %s, want admin", updated.Role)
	}
}

func TestUpdateUserRole_CannotChangeSuperadmin(t *testing.T) {
	_, env, superadmin, token := setupAdminTest(t)

	req := httptest.NewRequest(http.MethodPatch, "/api/v1/admin/users/"+superadmin.ID+"/role",
		jsonBody(map[string]string{"role": "user"}))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	rec := env.doRequest(req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", rec.Code)
	}
}

func TestUpdateUserRole_InvalidRole(t *testing.T) {
	_, env, _, token := setupAdminTest(t)
	target, _ := env.createTestUser(t, "target@test.com", "password123", "user")

	req := httptest.NewRequest(http.MethodPatch, "/api/v1/admin/users/"+target.ID+"/role",
		jsonBody(map[string]string{"role": "invalid"}))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	rec := env.doRequest(req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

func TestUpdateUserRole_UserNotFound(t *testing.T) {
	_, env, _, token := setupAdminTest(t)

	req := httptest.NewRequest(http.MethodPatch, "/api/v1/admin/users/nonexistent/role",
		jsonBody(map[string]string{"role": "admin"}))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	rec := env.doRequest(req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}

func TestUpdateUserRole_InvalidRequestBody(t *testing.T) {
	_, env, _, token := setupAdminTest(t)
	target, _ := env.createTestUser(t, "target@test.com", "password123", "user")

	req := httptest.NewRequest(http.MethodPatch, "/api/v1/admin/users/"+target.ID+"/role", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	rec := env.doRequest(req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

// ──────────────────────────────────────────────
// DeleteUser tests
// ──────────────────────────────────────────────

func TestDeleteUser_Success(t *testing.T) {
	_, env, _, token := setupAdminTest(t)
	target, _ := env.createTestUser(t, "target@test.com", "password123", "user")

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/admin/users/"+target.ID, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := env.doRequest(req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}

	var count int64
	env.db.Model(&models.User{}).Where("id = ?", target.ID).Count(&count)
	if count != 0 {
		t.Error("user should be deleted")
	}
}

func TestDeleteUser_CannotDeleteSuperadmin(t *testing.T) {
	_, env, superadmin, token := setupAdminTest(t)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/admin/users/"+superadmin.ID, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := env.doRequest(req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", rec.Code)
	}
}

func TestDeleteUser_NotFound(t *testing.T) {
	_, env, _, token := setupAdminTest(t)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/admin/users/nonexistent", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := env.doRequest(req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}

func TestDeleteUser_CascadesRelatedData(t *testing.T) {
	_, env, _, token := setupAdminTest(t)
	target, _ := env.createTestUser(t, "target@test.com", "password123", "user")

	// Create related data
	site := models.Site{UserID: target.ID, SubdomainSlug: "test-site", Name: "Test"}
	env.db.Create(&site)

	apiKey := models.APIKey{UserID: target.ID, KeyHash: "hash", Name: "key"}
	env.db.Create(&apiKey)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/admin/users/"+target.ID, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := env.doRequest(req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}

	// Verify cascaded deletions
	var count int64
	env.db.Model(&models.Site{}).Where("user_id = ?", target.ID).Count(&count)
	if count != 0 {
		t.Error("sites should be deleted")
	}

	env.db.Model(&models.APIKey{}).Where("user_id = ?", target.ID).Count(&count)
	if count != 0 {
		t.Error("API keys should be deleted")
	}
}

// ──────────────────────────────────────────────
// GetSettings tests
// ──────────────────────────────────────────────

func TestGetSettings_Success(t *testing.T) {
	_, env, _, token := setupAdminTest(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/settings", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := env.doRequest(req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}

	body := parseJSON(t, rec)
	if body["registration_enabled"] == nil {
		t.Error("expected registration_enabled in response")
	}
	if body["invite_required"] == nil {
		t.Error("expected invite_required in response")
	}
}

// ──────────────────────────────────────────────
// UpdateSettings tests
// ──────────────────────────────────────────────

func TestUpdateSettings_RegistrationEnabled(t *testing.T) {
	_, env, _, token := setupAdminTest(t)

	enabled := false
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/admin/settings",
		jsonBody(map[string]interface{}{"registration_enabled": &enabled}))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	rec := env.doRequest(req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}

	val, _ := models.GetSetting(env.db, "registration_enabled")
	if val != "false" {
		t.Errorf("registration_enabled = %s, want false", val)
	}
}

func TestUpdateSettings_InviteRequired(t *testing.T) {
	_, env, _, token := setupAdminTest(t)

	required := true
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/admin/settings",
		jsonBody(map[string]interface{}{"invite_required": &required}))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	rec := env.doRequest(req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}

	val, _ := models.GetSetting(env.db, "invite_required")
	if val != "true" {
		t.Errorf("invite_required = %s, want true", val)
	}
}

func TestUpdateSettings_EmptyBody(t *testing.T) {
	_, env, _, token := setupAdminTest(t)

	req := httptest.NewRequest(http.MethodPatch, "/api/v1/admin/settings",
		jsonBody(map[string]interface{}{}))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	rec := env.doRequest(req)

	// Empty body is valid - just returns current settings without updating
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200 (empty body is valid)", rec.Code)
	}
}

// ──────────────────────────────────────────────
// CreateInvite tests
// ──────────────────────────────────────────────

func TestCreateInvite_Success(t *testing.T) {
	h, _, admin, _ := setupAdminTest(t)

	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/invites", bytes.NewReader([]byte("{}")))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set(contextKeyUserID, admin.ID)

	if err := h.CreateInvite(c); err != nil {
		t.Fatalf("CreateInvite returned error: %v", err)
	}

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201: %s", rec.Code, rec.Body.String())
	}

	var result models.Invite
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Code == "" {
		t.Error("expected invite code")
	}
	if !result.Active {
		t.Error("invite should be active")
	}
}

func TestCreateInvite_WithMaxUses(t *testing.T) {
	h, _, admin, _ := setupAdminTest(t)

	maxUses := 5
	e := echo.New()
	body, _ := json.Marshal(map[string]interface{}{"max_uses": maxUses})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/invites", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set(contextKeyUserID, admin.ID)

	if err := h.CreateInvite(c); err != nil {
		t.Fatalf("CreateInvite returned error: %v", err)
	}

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201: %s", rec.Code, rec.Body.String())
	}

	var result models.Invite
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.MaxUses == nil || *result.MaxUses != maxUses {
		t.Errorf("max_uses = %v, want %d", result.MaxUses, maxUses)
	}
}

func TestCreateInvite_WithExpiresAt(t *testing.T) {
	h, _, admin, _ := setupAdminTest(t)

	expiresAt := time.Now().Add(24 * time.Hour)
	e := echo.New()
	body, _ := json.Marshal(map[string]interface{}{"expires_at": expiresAt})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/invites", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set(contextKeyUserID, admin.ID)

	if err := h.CreateInvite(c); err != nil {
		t.Fatalf("CreateInvite returned error: %v", err)
	}

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201: %s", rec.Code, rec.Body.String())
	}
}

// ──────────────────────────────────────────────
// ListInvites tests
// ──────────────────────────────────────────────

func TestListInvites_Success(t *testing.T) {
	h, env, admin, _ := setupAdminTest(t)

	env.db.Create(&models.Invite{Code: "code1", CreatedBy: admin.ID, Active: true})
	env.db.Create(&models.Invite{Code: "code2", CreatedBy: admin.ID, Active: true})

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/invites", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := h.ListInvites(c); err != nil {
		t.Fatalf("ListInvites returned error: %v", err)
	}

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}

	var invites []models.Invite
	if err := json.Unmarshal(rec.Body.Bytes(), &invites); err != nil {
		t.Fatal(err)
	}
	if len(invites) != 2 {
		t.Errorf("got %d invites, want 2", len(invites))
	}
}

// ──────────────────────────────────────────────
// RevokeInvite tests
// ──────────────────────────────────────────────

func TestRevokeInvite_Success(t *testing.T) {
	h, env, admin, _ := setupAdminTest(t)

	invite := models.Invite{Code: "revoke-test", CreatedBy: admin.ID, Active: true}
	env.db.Create(&invite)

	e := echo.New()
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/admin/invites/"+invite.ID, nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(invite.ID)

	if err := h.RevokeInvite(c); err != nil {
		t.Fatalf("RevokeInvite returned error: %v", err)
	}

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}

	var updated models.Invite
	env.db.First(&updated, "id = ?", invite.ID)
	if updated.Active {
		t.Error("invite should be revoked")
	}
}

func TestRevokeInvite_NotFound(t *testing.T) {
	h, _, _, _ := setupAdminTest(t)

	e := echo.New()
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/admin/invites/nonexistent", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("nonexistent")

	if err := h.RevokeInvite(c); err != nil {
		t.Fatalf("RevokeInvite returned error: %v", err)
	}

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}
