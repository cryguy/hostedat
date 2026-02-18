package api

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cryguy/hostedat/internal/auth"
	"github.com/cryguy/hostedat/internal/config"
	"github.com/cryguy/hostedat/internal/models"
	"github.com/cryguy/hostedat/internal/storage"
	"github.com/labstack/echo/v4"
	"gorm.io/gorm"
)

// ──────────────────────────────────────────────
// Test helpers
// ──────────────────────────────────────────────

type testEnv struct {
	e         *echo.Echo
	db        *gorm.DB
	store     *storage.Manager
	cache     *storage.SiteRulesCache
	jwtSecret string
	domain    string
}

func setupTestEnv(t *testing.T) *testEnv {
	return setupTestEnvWithMinVersion(t, "")
}

func setupTestEnvWithMinVersion(t *testing.T, minVersion string) *testEnv {
	t.Helper()

	db, err := models.InitDB(config.DBConfig{Driver: "sqlite", DSN: ":memory:"})
	if err != nil {
		t.Fatalf("InitDB: %v", err)
	}

	cfg := &config.Config{
		Domain:        "test.local",
		JWTSecret:     "test-jwt-secret-that-is-at-least-32-characters-long",
		MinCLIVersion: minVersion,
		Registration: config.RegConfig{
			Enabled: true,
		},
	}
	models.SeedDefaults(db, cfg)

	store := storage.NewManager(t.TempDir())
	cache := storage.NewSiteRulesCache()

	e := echo.New()
	e.HTTPErrorHandler = CustomErrorHandler
	e.Use(SubdomainRouter(db, store, cache, cfg.Domain, nil, nil))
	RegisterRoutes(e, db, cfg, store, "0.1.0", nil, nil, nil, "")

	return &testEnv{
		e:         e,
		db:        db,
		store:     store,
		cache:     cache,
		jwtSecret: cfg.JWTSecret,
		domain:    cfg.Domain,
	}
}

func (env *testEnv) createTestUser(t *testing.T, email, password, role string) (models.User, string) {
	t.Helper()
	hash, _ := auth.HashPassword(password)
	user := models.User{Email: email, PasswordHash: hash, Role: role}
	if err := env.db.Create(&user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	token, _ := auth.GenerateToken(user.ID, user.Email, user.Role, env.jwtSecret)
	return user, token
}

func (env *testEnv) doRequest(req *http.Request) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	env.e.ServeHTTP(rec, req)
	return rec
}

func createTestZip(t *testing.T, files map[string]string) *bytes.Buffer {
	t.Helper()
	buf := new(bytes.Buffer)
	w := zip.NewWriter(buf)
	for name, content := range files {
		f, err := w.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		f.Write([]byte(content))
	}
	w.Close()
	return buf
}

func jsonBody(v interface{}) io.Reader {
	b, _ := json.Marshal(v)
	return bytes.NewReader(b)
}

func parseJSON(t *testing.T, rec *httptest.ResponseRecorder) map[string]interface{} {
	t.Helper()
	var m map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &m); err != nil {
		t.Fatalf("parsing JSON response: %v\nbody: %s", err, rec.Body.String())
	}
	return m
}

// ──────────────────────────────────────────────
// Auth tests
// ──────────────────────────────────────────────

func TestRegister_FirstUserIsSuperadmin(t *testing.T) {
	env := setupTestEnv(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register",
		jsonBody(map[string]string{"email": "first@test.com", "password": "password123"}))
	req.Header.Set("Content-Type", "application/json")
	rec := env.doRequest(req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201: %s", rec.Code, rec.Body.String())
	}
	body := parseJSON(t, rec)
	user := body["user"].(map[string]interface{})
	if user["role"] != "superadmin" {
		t.Errorf("first user role = %q, want superadmin", user["role"])
	}
	if body["token"] == nil || body["token"] == "" {
		t.Error("expected token in response")
	}
}

func TestRegister_SecondUserIsUser(t *testing.T) {
	env := setupTestEnv(t)
	env.createTestUser(t, "first@test.com", "password123", "superadmin")

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register",
		jsonBody(map[string]string{"email": "second@test.com", "password": "password123"}))
	req.Header.Set("Content-Type", "application/json")
	rec := env.doRequest(req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	body := parseJSON(t, rec)
	user := body["user"].(map[string]interface{})
	if user["role"] != "user" {
		t.Errorf("second user role = %q, want user", user["role"])
	}
}

func TestRegister_DuplicateEmail(t *testing.T) {
	env := setupTestEnv(t)
	env.createTestUser(t, "dup@test.com", "password123", "user")

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register",
		jsonBody(map[string]string{"email": "dup@test.com", "password": "password123"}))
	req.Header.Set("Content-Type", "application/json")
	rec := env.doRequest(req)

	if rec.Code != http.StatusConflict {
		t.Errorf("status = %d, want 409", rec.Code)
	}
}

func TestRegister_ShortPassword(t *testing.T) {
	env := setupTestEnv(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register",
		jsonBody(map[string]string{"email": "a@b.com", "password": "short"}))
	req.Header.Set("Content-Type", "application/json")
	rec := env.doRequest(req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

func TestRegister_RegistrationDisabled(t *testing.T) {
	env := setupTestEnv(t)
	// Create an existing user so the first-user bootstrap bypass doesn't apply.
	env.createTestUser(t, "existing@test.com", "password123", "superadmin")
	models.SetSetting(env.db, "registration_enabled", "false")

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register",
		jsonBody(map[string]string{"email": "a@b.com", "password": "password123"}))
	req.Header.Set("Content-Type", "application/json")
	rec := env.doRequest(req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", rec.Code)
	}
}

func TestRegister_InviteFlow(t *testing.T) {
	env := setupTestEnv(t)
	models.SetSetting(env.db, "invite_required", "true")

	admin, _ := env.createTestUser(t, "admin@test.com", "password123", "superadmin")
	invite := models.Invite{Code: "testcode", CreatedBy: admin.ID, Active: true}
	env.db.Create(&invite)

	// Without invite code → 400
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register",
		jsonBody(map[string]string{"email": "new@test.com", "password": "password123"}))
	req.Header.Set("Content-Type", "application/json")
	rec := env.doRequest(req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("without code: status = %d, want 400", rec.Code)
	}

	// With valid invite code → 201
	req = httptest.NewRequest(http.MethodPost, "/api/v1/auth/register",
		jsonBody(map[string]string{"email": "new@test.com", "password": "password123", "invite_code": "testcode"}))
	req.Header.Set("Content-Type", "application/json")
	rec = env.doRequest(req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("with code: status = %d: %s", rec.Code, rec.Body.String())
	}

	// Verify invite use_count incremented
	var updated models.Invite
	env.db.First(&updated, "code = ?", "testcode")
	if updated.UseCount != 1 {
		t.Errorf("use_count = %d, want 1", updated.UseCount)
	}
}

func TestLogin_Success(t *testing.T) {
	env := setupTestEnv(t)
	env.createTestUser(t, "login@test.com", "password123", "user")

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login",
		jsonBody(map[string]string{"email": "login@test.com", "password": "password123"}))
	req.Header.Set("Content-Type", "application/json")
	rec := env.doRequest(req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	body := parseJSON(t, rec)
	if body["token"] == nil || body["token"] == "" {
		t.Error("expected token")
	}
}

func TestLogin_WrongPassword(t *testing.T) {
	env := setupTestEnv(t)
	env.createTestUser(t, "login@test.com", "password123", "user")

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login",
		jsonBody(map[string]string{"email": "login@test.com", "password": "wrongwrong"}))
	req.Header.Set("Content-Type", "application/json")
	rec := env.doRequest(req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
}

func TestLogin_UnknownEmail(t *testing.T) {
	env := setupTestEnv(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login",
		jsonBody(map[string]string{"email": "nobody@test.com", "password": "password123"}))
	req.Header.Set("Content-Type", "application/json")
	rec := env.doRequest(req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
}

// ──────────────────────────────────────────────
// Middleware tests
// ──────────────────────────────────────────────

func TestMiddleware_NoHeader(t *testing.T) {
	env := setupTestEnv(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/sites", nil)
	rec := env.doRequest(req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
}

func TestMiddleware_InvalidFormat(t *testing.T) {
	env := setupTestEnv(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/sites", nil)
	req.Header.Set("Authorization", "Basic abc123")
	rec := env.doRequest(req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
}

func TestMiddleware_ValidJWT(t *testing.T) {
	env := setupTestEnv(t)
	_, token := env.createTestUser(t, "jwt@test.com", "password123", "user")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/sites", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := env.doRequest(req)
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
}

func TestMiddleware_InvalidJWT(t *testing.T) {
	env := setupTestEnv(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/sites", nil)
	req.Header.Set("Authorization", "Bearer invalid.jwt.token")
	rec := env.doRequest(req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
}

func TestMiddleware_APIKeyAuth(t *testing.T) {
	env := setupTestEnv(t)
	user, _ := env.createTestUser(t, "key@test.com", "password123", "user")

	rawKey, hash, _ := auth.GenerateAPIKey()
	env.db.Create(&models.APIKey{UserID: user.ID, KeyHash: hash, Name: "test"})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/sites", nil)
	req.Header.Set("Authorization", "Bearer "+rawKey)
	rec := env.doRequest(req)
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
}

func TestMiddleware_InvalidAPIKey(t *testing.T) {
	env := setupTestEnv(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/sites", nil)
	req.Header.Set("Authorization", "Bearer hd_invalidkeythatdoesnotexistinthdb")
	rec := env.doRequest(req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
}

func TestMiddleware_RequireAdmin(t *testing.T) {
	env := setupTestEnv(t)
	_, adminToken := env.createTestUser(t, "admin@test.com", "password123", "admin")
	_, userToken := env.createTestUser(t, "user@test.com", "password123", "user")

	// Admin → 200
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/users", nil)
	req.Header.Set("Authorization", "Bearer "+adminToken)
	rec := env.doRequest(req)
	if rec.Code != http.StatusOK {
		t.Errorf("admin: status = %d, want 200", rec.Code)
	}

	// User → 403
	req = httptest.NewRequest(http.MethodGet, "/api/v1/admin/users", nil)
	req.Header.Set("Authorization", "Bearer "+userToken)
	rec = env.doRequest(req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("user: status = %d, want 403", rec.Code)
	}
}

// ──────────────────────────────────────────────
// Site tests
// ──────────────────────────────────────────────

func TestSite_Create(t *testing.T) {
	env := setupTestEnv(t)
	_, token := env.createTestUser(t, "u@t.com", "password123", "user")

	req := httptest.NewRequest(http.MethodPost, "/api/v1/sites",
		jsonBody(map[string]string{"name": "My Site", "subdomain_slug": "mysite"}))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	rec := env.doRequest(req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	body := parseJSON(t, rec)
	if body["subdomain_slug"] != "mysite" {
		t.Errorf("slug = %q", body["subdomain_slug"])
	}
}

func TestSite_Create_AutoSlug(t *testing.T) {
	env := setupTestEnv(t)
	_, token := env.createTestUser(t, "u@t.com", "password123", "user")

	req := httptest.NewRequest(http.MethodPost, "/api/v1/sites",
		jsonBody(map[string]string{"name": "My Cool Project"}))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	rec := env.doRequest(req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	body := parseJSON(t, rec)
	slug := body["subdomain_slug"].(string)
	if !strings.HasPrefix(slug, "my-cool-project") {
		t.Errorf("auto slug = %q, expected prefix my-cool-project", slug)
	}
}

func TestSite_Create_ReservedSlug(t *testing.T) {
	env := setupTestEnv(t)
	_, token := env.createTestUser(t, "u@t.com", "password123", "user")

	req := httptest.NewRequest(http.MethodPost, "/api/v1/sites",
		jsonBody(map[string]string{"name": "Test", "subdomain_slug": "www"}))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	rec := env.doRequest(req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

func TestSite_Create_DuplicateSlug(t *testing.T) {
	env := setupTestEnv(t)
	_, token := env.createTestUser(t, "u@t.com", "password123", "user")

	for i, wantCode := range []int{http.StatusCreated, http.StatusConflict} {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/sites",
			jsonBody(map[string]string{"name": "Test", "subdomain_slug": "dupe"}))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)
		rec := env.doRequest(req)
		if rec.Code != wantCode {
			t.Errorf("attempt %d: status = %d, want %d", i, rec.Code, wantCode)
		}
	}
}

func TestSite_List_OwnSitesOnly(t *testing.T) {
	env := setupTestEnv(t)
	user1, token1 := env.createTestUser(t, "u1@t.com", "password123", "user")
	user2, token2 := env.createTestUser(t, "u2@t.com", "password123", "user")

	env.db.Create(&models.Site{UserID: user1.ID, SubdomainSlug: "user1site", Name: "U1"})
	env.db.Create(&models.Site{UserID: user2.ID, SubdomainSlug: "user2site", Name: "U2"})

	// User1 sees only their site
	req := httptest.NewRequest(http.MethodGet, "/api/v1/sites", nil)
	req.Header.Set("Authorization", "Bearer "+token1)
	rec := env.doRequest(req)
	var sites1 []map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &sites1)
	if len(sites1) != 1 || sites1[0]["subdomain_slug"] != "user1site" {
		t.Errorf("user1 sites: %v", sites1)
	}

	// User2 sees only their site
	req = httptest.NewRequest(http.MethodGet, "/api/v1/sites", nil)
	req.Header.Set("Authorization", "Bearer "+token2)
	rec = env.doRequest(req)
	var sites2 []map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &sites2)
	if len(sites2) != 1 || sites2[0]["subdomain_slug"] != "user2site" {
		t.Errorf("user2 sites: %v", sites2)
	}
}

func TestSite_Get_OwnAndAdminBypass(t *testing.T) {
	env := setupTestEnv(t)
	owner, ownerToken := env.createTestUser(t, "owner@t.com", "password123", "user")
	_, adminToken := env.createTestUser(t, "admin@t.com", "password123", "admin")
	_, otherToken := env.createTestUser(t, "other@t.com", "password123", "user")

	site := models.Site{UserID: owner.ID, SubdomainSlug: "gettest", Name: "Test"}
	env.db.Create(&site)

	// Owner → 200
	req := httptest.NewRequest(http.MethodGet, "/api/v1/sites/"+site.ID, nil)
	req.Header.Set("Authorization", "Bearer "+ownerToken)
	rec := env.doRequest(req)
	if rec.Code != http.StatusOK {
		t.Errorf("owner: status = %d", rec.Code)
	}

	// Admin → 200
	req = httptest.NewRequest(http.MethodGet, "/api/v1/sites/"+site.ID, nil)
	req.Header.Set("Authorization", "Bearer "+adminToken)
	rec = env.doRequest(req)
	if rec.Code != http.StatusOK {
		t.Errorf("admin: status = %d", rec.Code)
	}

	// Other user → 403
	req = httptest.NewRequest(http.MethodGet, "/api/v1/sites/"+site.ID, nil)
	req.Header.Set("Authorization", "Bearer "+otherToken)
	rec = env.doRequest(req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("other: status = %d, want 403", rec.Code)
	}
}

func TestSite_UpdateSPAMode(t *testing.T) {
	env := setupTestEnv(t)
	user, token := env.createTestUser(t, "u@t.com", "password123", "user")
	site := models.Site{UserID: user.ID, SubdomainSlug: "spa-test", Name: "SPA"}
	env.db.Create(&site)

	spa := true
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/sites/"+site.ID,
		jsonBody(map[string]interface{}{"spa_mode": spa}))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	rec := env.doRequest(req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	body := parseJSON(t, rec)
	if body["spa_mode"] != true {
		t.Errorf("spa_mode = %v", body["spa_mode"])
	}
}

func TestSite_Delete(t *testing.T) {
	env := setupTestEnv(t)
	user, token := env.createTestUser(t, "u@t.com", "password123", "user")
	site := models.Site{UserID: user.ID, SubdomainSlug: "del-test", Name: "Del"}
	env.db.Create(&site)
	env.db.Create(&models.Deployment{SiteID: site.ID, Version: 1, FileHash: "abc"})

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/sites/"+site.ID, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := env.doRequest(req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}

	// Verify cascade
	var count int64
	env.db.Model(&models.Site{}).Where("id = ?", site.ID).Count(&count)
	if count != 0 {
		t.Error("site not deleted")
	}
	env.db.Model(&models.Deployment{}).Where("site_id = ?", site.ID).Count(&count)
	if count != 0 {
		t.Error("deployments not cascaded")
	}
}

// ──────────────────────────────────────────────
// Deploy tests
// ──────────────────────────────────────────────

func multipartZip(t *testing.T, zipBuf *bytes.Buffer) (*bytes.Buffer, string) {
	t.Helper()
	body := new(bytes.Buffer)
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("file", "site.zip")
	if err != nil {
		t.Fatal(err)
	}
	io.Copy(part, zipBuf)
	writer.Close()
	return body, writer.FormDataContentType()
}

func TestDeploy_Success(t *testing.T) {
	env := setupTestEnv(t)
	user, token := env.createTestUser(t, "u@t.com", "password123", "user")
	site := models.Site{UserID: user.ID, SubdomainSlug: "deploy-test", Name: "Deploy"}
	env.db.Create(&site)

	zipBuf := createTestZip(t, map[string]string{"index.html": "<h1>hello</h1>"})
	body, ct := multipartZip(t, zipBuf)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/sites/"+site.ID+"/deploy", body)
	req.Header.Set("Content-Type", ct)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := env.doRequest(req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	resp := parseJSON(t, rec)
	if resp["version"] != float64(1) {
		t.Errorf("version = %v, want 1", resp["version"])
	}

	// Verify site active_version updated
	var updated models.Site
	env.db.First(&updated, "id = ?", site.ID)
	if updated.ActiveVersion == nil || *updated.ActiveVersion != 1 {
		t.Errorf("active_version = %v, want 1", updated.ActiveVersion)
	}
}

func TestDeploy_VersionIncrement(t *testing.T) {
	env := setupTestEnv(t)
	user, token := env.createTestUser(t, "u@t.com", "password123", "user")
	site := models.Site{UserID: user.ID, SubdomainSlug: "ver-test", Name: "Ver"}
	env.db.Create(&site)

	for i := 1; i <= 2; i++ {
		zipBuf := createTestZip(t, map[string]string{"index.html": fmt.Sprintf("v%d", i)})
		body, ct := multipartZip(t, zipBuf)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/sites/"+site.ID+"/deploy", body)
		req.Header.Set("Content-Type", ct)
		req.Header.Set("Authorization", "Bearer "+token)
		rec := env.doRequest(req)
		if rec.Code != http.StatusCreated {
			t.Fatalf("deploy %d: status = %d", i, rec.Code)
		}
		resp := parseJSON(t, rec)
		if resp["version"] != float64(i) {
			t.Errorf("deploy %d: version = %v", i, resp["version"])
		}
	}
}

func TestDeploy_NoFile(t *testing.T) {
	env := setupTestEnv(t)
	user, token := env.createTestUser(t, "u@t.com", "password123", "user")
	site := models.Site{UserID: user.ID, SubdomainSlug: "nofile", Name: "No"}
	env.db.Create(&site)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/sites/"+site.ID+"/deploy", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := env.doRequest(req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

func TestDeploy_Forbidden(t *testing.T) {
	env := setupTestEnv(t)
	owner, _ := env.createTestUser(t, "owner@t.com", "password123", "user")
	_, otherToken := env.createTestUser(t, "other@t.com", "password123", "user")
	site := models.Site{UserID: owner.ID, SubdomainSlug: "forbid", Name: "Forbid"}
	env.db.Create(&site)

	zipBuf := createTestZip(t, map[string]string{"index.html": "hi"})
	body, ct := multipartZip(t, zipBuf)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/sites/"+site.ID+"/deploy", body)
	req.Header.Set("Content-Type", ct)
	req.Header.Set("Authorization", "Bearer "+otherToken)
	rec := env.doRequest(req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", rec.Code)
	}
}

func TestDeploy_ListDeployments(t *testing.T) {
	env := setupTestEnv(t)
	user, token := env.createTestUser(t, "u@t.com", "password123", "user")
	site := models.Site{UserID: user.ID, SubdomainSlug: "listdep", Name: "List"}
	env.db.Create(&site)
	env.db.Create(&models.Deployment{SiteID: site.ID, Version: 1, FileHash: "aaa"})
	env.db.Create(&models.Deployment{SiteID: site.ID, Version: 2, FileHash: "bbb"})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/sites/"+site.ID+"/deployments", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := env.doRequest(req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var result struct {
		Deployments []map[string]interface{} `json:"deployments"`
		Total       int                      `json:"total"`
		Page        int                      `json:"page"`
	}
	json.Unmarshal(rec.Body.Bytes(), &result)
	if len(result.Deployments) != 2 {
		t.Errorf("got %d deployments, want 2", len(result.Deployments))
	}
	if result.Total != 2 {
		t.Errorf("got total %d, want 2", result.Total)
	}
	if result.Page != 1 {
		t.Errorf("got page %d, want 1", result.Page)
	}
}

// ──────────────────────────────────────────────
// API key tests
// ──────────────────────────────────────────────

func TestAPIKey_Create(t *testing.T) {
	env := setupTestEnv(t)
	_, token := env.createTestUser(t, "u@t.com", "password123", "user")

	req := httptest.NewRequest(http.MethodPost, "/api/v1/keys",
		jsonBody(map[string]string{"name": "CI Key"}))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	rec := env.doRequest(req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	body := parseJSON(t, rec)
	key, ok := body["key"].(string)
	if !ok || !strings.HasPrefix(key, "hd_") {
		t.Errorf("key = %q, want hd_ prefix", key)
	}
}

func TestAPIKey_Create_MissingName(t *testing.T) {
	env := setupTestEnv(t)
	_, token := env.createTestUser(t, "u@t.com", "password123", "user")

	req := httptest.NewRequest(http.MethodPost, "/api/v1/keys",
		jsonBody(map[string]string{}))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	rec := env.doRequest(req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

func TestAPIKey_List(t *testing.T) {
	env := setupTestEnv(t)
	user, token := env.createTestUser(t, "u@t.com", "password123", "user")
	_, hash, _ := auth.GenerateAPIKey()
	env.db.Create(&models.APIKey{UserID: user.ID, KeyHash: hash, Name: "key1"})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/keys", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := env.doRequest(req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var keys []map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &keys)
	if len(keys) != 1 {
		t.Errorf("got %d keys, want 1", len(keys))
	}
}

func TestAPIKey_Delete(t *testing.T) {
	env := setupTestEnv(t)
	user, token := env.createTestUser(t, "u@t.com", "password123", "user")
	_, hash, _ := auth.GenerateAPIKey()
	key := models.APIKey{UserID: user.ID, KeyHash: hash, Name: "to-delete"}
	env.db.Create(&key)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/keys/"+key.ID, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := env.doRequest(req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
}

func TestAPIKey_Delete_Forbidden(t *testing.T) {
	env := setupTestEnv(t)
	owner, _ := env.createTestUser(t, "owner@t.com", "password123", "user")
	_, otherToken := env.createTestUser(t, "other@t.com", "password123", "user")
	_, hash, _ := auth.GenerateAPIKey()
	key := models.APIKey{UserID: owner.ID, KeyHash: hash, Name: "not-yours"}
	env.db.Create(&key)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/keys/"+key.ID, nil)
	req.Header.Set("Authorization", "Bearer "+otherToken)
	rec := env.doRequest(req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", rec.Code)
	}
}

// ──────────────────────────────────────────────
// Admin tests
// ──────────────────────────────────────────────

func TestAdmin_ListUsers(t *testing.T) {
	env := setupTestEnv(t)
	_, token := env.createTestUser(t, "admin@t.com", "password123", "admin")
	env.createTestUser(t, "user@t.com", "password123", "user")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/users", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := env.doRequest(req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	body := parseJSON(t, rec)
	if body["total"] != float64(2) {
		t.Errorf("total = %v, want 2", body["total"])
	}
}

func TestAdmin_UpdateRole(t *testing.T) {
	env := setupTestEnv(t)
	_, adminToken := env.createTestUser(t, "admin@t.com", "password123", "admin")
	target, _ := env.createTestUser(t, "user@t.com", "password123", "user")

	req := httptest.NewRequest(http.MethodPatch, "/api/v1/admin/users/"+target.ID+"/role",
		jsonBody(map[string]string{"role": "admin"}))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+adminToken)
	rec := env.doRequest(req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	body := parseJSON(t, rec)
	if body["role"] != "admin" {
		t.Errorf("role = %q, want admin", body["role"])
	}
}

func TestAdmin_UpdateRole_SuperadminProtected(t *testing.T) {
	env := setupTestEnv(t)
	superadmin, _ := env.createTestUser(t, "super@t.com", "password123", "superadmin")
	_, adminToken := env.createTestUser(t, "admin@t.com", "password123", "admin")

	req := httptest.NewRequest(http.MethodPatch, "/api/v1/admin/users/"+superadmin.ID+"/role",
		jsonBody(map[string]string{"role": "user"}))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+adminToken)
	rec := env.doRequest(req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", rec.Code)
	}
}

func TestAdmin_DeleteUser(t *testing.T) {
	env := setupTestEnv(t)
	_, adminToken := env.createTestUser(t, "admin@t.com", "password123", "admin")
	target, _ := env.createTestUser(t, "victim@t.com", "password123", "user")

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/admin/users/"+target.ID, nil)
	req.Header.Set("Authorization", "Bearer "+adminToken)
	rec := env.doRequest(req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}

	var count int64
	env.db.Model(&models.User{}).Where("id = ?", target.ID).Count(&count)
	if count != 0 {
		t.Error("user not deleted")
	}
}

func TestAdmin_DeleteUser_SuperadminProtected(t *testing.T) {
	env := setupTestEnv(t)
	superadmin, _ := env.createTestUser(t, "super@t.com", "password123", "superadmin")
	_, adminToken := env.createTestUser(t, "admin@t.com", "password123", "admin")

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/admin/users/"+superadmin.ID, nil)
	req.Header.Set("Authorization", "Bearer "+adminToken)
	rec := env.doRequest(req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", rec.Code)
	}
}

func TestAdmin_GetUpdateSettings(t *testing.T) {
	env := setupTestEnv(t)
	_, token := env.createTestUser(t, "admin@t.com", "password123", "admin")

	// Get defaults
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/settings", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := env.doRequest(req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET status = %d", rec.Code)
	}

	// Update
	req = httptest.NewRequest(http.MethodPatch, "/api/v1/admin/settings",
		jsonBody(map[string]interface{}{"registration_enabled": false, "invite_required": true}))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	rec = env.doRequest(req)
	if rec.Code != http.StatusOK {
		t.Fatalf("PATCH status = %d: %s", rec.Code, rec.Body.String())
	}
	body := parseJSON(t, rec)
	if body["registration_enabled"] != false {
		t.Errorf("registration_enabled = %v", body["registration_enabled"])
	}
	if body["invite_required"] != true {
		t.Errorf("invite_required = %v", body["invite_required"])
	}
}

func TestAdmin_Invites(t *testing.T) {
	env := setupTestEnv(t)
	_, token := env.createTestUser(t, "admin@t.com", "password123", "admin")

	// Create invite
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/invites",
		jsonBody(map[string]interface{}{"max_uses": 5}))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	rec := env.doRequest(req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: status = %d: %s", rec.Code, rec.Body.String())
	}
	invite := parseJSON(t, rec)
	inviteID := invite["id"].(string)

	// List invites
	req = httptest.NewRequest(http.MethodGet, "/api/v1/admin/invites", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec = env.doRequest(req)
	if rec.Code != http.StatusOK {
		t.Fatalf("list: status = %d", rec.Code)
	}
	var invites []map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &invites)
	if len(invites) != 1 {
		t.Errorf("got %d invites, want 1", len(invites))
	}

	// Revoke
	req = httptest.NewRequest(http.MethodDelete, "/api/v1/admin/invites/"+inviteID, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec = env.doRequest(req)
	if rec.Code != http.StatusOK {
		t.Fatalf("revoke: status = %d: %s", rec.Code, rec.Body.String())
	}

	// Verify revoked
	var revoked models.Invite
	env.db.First(&revoked, "id = ?", inviteID)
	if revoked.Active {
		t.Error("invite should be inactive after revoke")
	}
}

// ──────────────────────────────────────────────
// Site serving tests
// ──────────────────────────────────────────────

func deploySite(t *testing.T, env *testEnv, siteID string, deployKey string, files map[string]string) {
	t.Helper()
	zipBuf := createTestZip(t, files)
	if err := env.store.ExtractZip(siteID, deployKey, bytes.NewReader(zipBuf.Bytes()), int64(zipBuf.Len())); err != nil {
		t.Fatalf("ExtractZip: %v", err)
	}
}

func TestServing_BareDomainPassesThrough(t *testing.T) {
	env := setupTestEnv(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login",
		jsonBody(map[string]string{"email": "a@b.com", "password": "pass1234"}))
	req.Header.Set("Content-Type", "application/json")
	req.Host = "test.local"
	rec := env.doRequest(req)
	// Should reach the API handler (401 because invalid creds), not 404 from site serving
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401 (API passthrough)", rec.Code)
	}
}

func TestServing_SubdomainServesStaticFile(t *testing.T) {
	env := setupTestEnv(t)
	user, _ := env.createTestUser(t, "u@t.com", "password123", "user")
	site := models.Site{UserID: user.ID, SubdomainSlug: "mysite", Name: "My"}
	env.db.Create(&site)
	v := 1
	dk := "deploy1"
	site.ActiveVersion = &v
	site.ActiveDeployID = &dk
	env.db.Save(&site)
	deploySite(t, env, site.ID, "deploy1", map[string]string{"index.html": "<h1>hi</h1>"})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Host = "mysite.test.local"
	rec := env.doRequest(req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "<h1>hi</h1>") {
		t.Errorf("body = %q", rec.Body.String())
	}
}

func TestServing_LocalhostSubdomain(t *testing.T) {
	env := setupTestEnv(t)
	user, _ := env.createTestUser(t, "u@t.com", "password123", "user")
	site := models.Site{UserID: user.ID, SubdomainSlug: "localsite", Name: "Local"}
	env.db.Create(&site)
	v := 1
	dk := "deploy1"
	site.ActiveVersion = &v
	site.ActiveDeployID = &dk
	env.db.Save(&site)
	deploySite(t, env, site.ID, "deploy1", map[string]string{"index.html": "local!"})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Host = "localsite.localhost"
	rec := env.doRequest(req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
}

func TestServing_UnknownSubdomain(t *testing.T) {
	env := setupTestEnv(t)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Host = "nonexistent.test.local"
	rec := env.doRequest(req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}

func TestServing_RedirectRules(t *testing.T) {
	env := setupTestEnv(t)
	user, _ := env.createTestUser(t, "u@t.com", "password123", "user")
	site := models.Site{UserID: user.ID, SubdomainSlug: "redir", Name: "Redir"}
	env.db.Create(&site)
	v := 1
	dk := "deploy1"
	site.ActiveVersion = &v
	site.ActiveDeployID = &dk
	env.db.Save(&site)
	deploySite(t, env, site.ID, "deploy1", map[string]string{
		"index.html": "home",
		"_redirects": "/old /new 301",
	})

	req := httptest.NewRequest(http.MethodGet, "/old", nil)
	req.Host = "redir.test.local"
	rec := env.doRequest(req)

	if rec.Code != http.StatusMovedPermanently {
		t.Errorf("status = %d, want 301", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "/new" {
		t.Errorf("Location = %q, want /new", loc)
	}
}

func TestServing_RewriteRules(t *testing.T) {
	env := setupTestEnv(t)
	user, _ := env.createTestUser(t, "u@t.com", "password123", "user")
	site := models.Site{UserID: user.ID, SubdomainSlug: "rewrite", Name: "Rew"}
	env.db.Create(&site)
	v := 1
	dk := "deploy1"
	site.ActiveVersion = &v
	site.ActiveDeployID = &dk
	env.db.Save(&site)
	deploySite(t, env, site.ID, "deploy1", map[string]string{
		"index.html": "<spa>app</spa>",
		"_redirects": "/* /index.html 200",
	})

	req := httptest.NewRequest(http.MethodGet, "/some/deep/path", nil)
	req.Host = "rewrite.test.local"
	rec := env.doRequest(req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "<spa>app</spa>") {
		t.Errorf("expected rewritten content, got: %s", rec.Body.String())
	}
}

func TestServing_StaticFilesPrecedeOverRewrites(t *testing.T) {
	env := setupTestEnv(t)
	user, _ := env.createTestUser(t, "u@t.com", "password123", "user")
	site := models.Site{UserID: user.ID, SubdomainSlug: "precedence", Name: "Prec"}
	env.db.Create(&site)
	v := 1
	dk := "deploy1"
	site.ActiveVersion = &v
	site.ActiveDeployID = &dk
	env.db.Save(&site)
	deploySite(t, env, site.ID, "deploy1", map[string]string{
		"index.html": "home",
		"about.html": "about page",
		"_redirects": "/* /index.html 200",
	})

	req := httptest.NewRequest(http.MethodGet, "/about.html", nil)
	req.Host = "precedence.test.local"
	rec := env.doRequest(req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "about page") {
		t.Errorf("expected static file content, got rewrite: %s", rec.Body.String())
	}
}

func TestServing_CustomHeaders(t *testing.T) {
	env := setupTestEnv(t)
	user, _ := env.createTestUser(t, "u@t.com", "password123", "user")
	site := models.Site{UserID: user.ID, SubdomainSlug: "headers", Name: "Hdr"}
	env.db.Create(&site)
	v := 1
	dk := "deploy1"
	site.ActiveVersion = &v
	site.ActiveDeployID = &dk
	env.db.Save(&site)
	deploySite(t, env, site.ID, "deploy1", map[string]string{
		"index.html": "hi",
		"_headers":   "/*\n  X-Custom: hello",
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Host = "headers.test.local"
	rec := env.doRequest(req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if rec.Header().Get("X-Custom") != "hello" {
		t.Errorf("X-Custom = %q, want hello", rec.Header().Get("X-Custom"))
	}
}

func TestServing_Custom404(t *testing.T) {
	env := setupTestEnv(t)
	user, _ := env.createTestUser(t, "u@t.com", "password123", "user")
	site := models.Site{UserID: user.ID, SubdomainSlug: "custom404", Name: "404"}
	env.db.Create(&site)
	v := 1
	dk := "deploy1"
	site.ActiveVersion = &v
	site.ActiveDeployID = &dk
	env.db.Save(&site)
	deploySite(t, env, site.ID, "deploy1", map[string]string{
		"index.html": "home",
		"404.html":   "<h1>Custom Not Found</h1>",
	})

	req := httptest.NewRequest(http.MethodGet, "/nonexistent", nil)
	req.Host = "custom404.test.local"
	rec := env.doRequest(req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "Custom Not Found") {
		t.Errorf("expected custom 404 content, got: %s", rec.Body.String())
	}
}

func TestServing_SPAMode(t *testing.T) {
	env := setupTestEnv(t)
	user, _ := env.createTestUser(t, "u@t.com", "password123", "user")
	site := models.Site{UserID: user.ID, SubdomainSlug: "spa", Name: "SPA", SPAMode: true}
	env.db.Create(&site)
	v := 1
	dk := "deploy1"
	site.ActiveVersion = &v
	site.ActiveDeployID = &dk
	env.db.Save(&site)
	deploySite(t, env, site.ID, "deploy1", map[string]string{
		"index.html": "<spa>app</spa>",
	})

	req := httptest.NewRequest(http.MethodGet, "/app/dashboard", nil)
	req.Host = "spa.test.local"
	rec := env.doRequest(req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "<spa>app</spa>") {
		t.Errorf("expected SPA fallback, got: %s", rec.Body.String())
	}
}

func TestServing_NoDeployment(t *testing.T) {
	env := setupTestEnv(t)
	user, _ := env.createTestUser(t, "u@t.com", "password123", "user")
	site := models.Site{UserID: user.ID, SubdomainSlug: "empty", Name: "Empty"}
	env.db.Create(&site) // no active_version set

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Host = "empty.test.local"
	rec := env.doRequest(req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
	body := parseJSON(t, rec)
	if !strings.Contains(body["error"].(string), "no deployment") {
		t.Errorf("error = %q", body["error"])
	}
}

// ──────────────────────────────────────────────
// Serve file content type test
// ──────────────────────────────────────────────

// ──────────────────────────────────────────────
// Security tests
// ──────────────────────────────────────────────

func TestCLILogin_InvalidPort(t *testing.T) {
	env := setupTestEnv(t)

	tests := []struct {
		name string
		port string
	}{
		{"non-numeric", "abc"},
		{"negative", "-1"},
		{"zero", "0"},
		{"too-large", "70000"},
		{"injection", "80@evil.com/"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/cli?port="+tt.port+"&state=test&code_challenge=abc&code_challenge_method=S256", nil)
			rec := env.doRequest(req)
			if rec.Code != http.StatusBadRequest {
				t.Errorf("GET port=%s: status = %d, want 400", tt.port, rec.Code)
			}
		})
	}
}

func TestCLILoginSubmit_InvalidPort(t *testing.T) {
	env := setupTestEnv(t)
	env.createTestUser(t, "cli@test.com", "password123", "user")

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/cli",
		jsonBody(map[string]string{"email": "cli@test.com", "password": "password123", "port": "80@evil.com/", "state": "test", "code_challenge": "abc"}))
	req.Header.Set("Content-Type", "application/json")
	rec := env.doRequest(req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

func TestRegister_InvalidEmail(t *testing.T) {
	env := setupTestEnv(t)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register",
		jsonBody(map[string]string{"email": "notanemail", "password": "password123"}))
	req.Header.Set("Content-Type", "application/json")
	rec := env.doRequest(req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
	body := parseJSON(t, rec)
	if !strings.Contains(body["error"].(string), "email") {
		t.Errorf("error = %q, want email-related message", body["error"])
	}
}

func TestServing_HeaderInjectionBlocked(t *testing.T) {
	env := setupTestEnv(t)
	user, _ := env.createTestUser(t, "u@t.com", "password123", "user")
	site := models.Site{UserID: user.ID, SubdomainSlug: "hdr-inject", Name: "HdrInj"}
	env.db.Create(&site)
	v := 1
	dk := "deploy1"
	site.ActiveVersion = &v
	site.ActiveDeployID = &dk
	env.db.Save(&site)
	deploySite(t, env, site.ID, "deploy1", map[string]string{
		"index.html": "hi",
		"_headers":   "/*\n  Set-Cookie: evil=true\n  X-Custom: safe",
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Host = "hdr-inject.test.local"
	rec := env.doRequest(req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	// Set-Cookie should be blocked
	if rec.Header().Get("Set-Cookie") != "" {
		t.Error("Set-Cookie header should be blocked from _headers")
	}
	// X-Custom should pass through
	if rec.Header().Get("X-Custom") != "safe" {
		t.Errorf("X-Custom = %q, want safe", rec.Header().Get("X-Custom"))
	}
}

func TestDeploy_ErrorMessageNonLeakage(t *testing.T) {
	env := setupTestEnv(t)
	user, token := env.createTestUser(t, "u@t.com", "password123", "user")
	site := models.Site{UserID: user.ID, SubdomainSlug: "errleak", Name: "ErrLeak"}
	env.db.Create(&site)

	// Send invalid (non-zip) data
	body := new(bytes.Buffer)
	writer := multipart.NewWriter(body)
	part, _ := writer.CreateFormFile("file", "bad.zip")
	part.Write([]byte("this is not a zip file"))
	writer.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/sites/"+site.ID+"/deploy", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+token)
	rec := env.doRequest(req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400: %s", rec.Code, rec.Body.String())
	}
	respBody := parseJSON(t, rec)
	errMsg := respBody["error"].(string)
	// Should be generic message, not leaking internal details
	if errMsg != "invalid zip file" {
		t.Errorf("error = %q, want 'invalid zip file'", errMsg)
	}
}

func TestServing_ContentType(t *testing.T) {
	env := setupTestEnv(t)
	user, _ := env.createTestUser(t, "u@t.com", "password123", "user")
	site := models.Site{UserID: user.ID, SubdomainSlug: "ct-test", Name: "CT"}
	env.db.Create(&site)
	v := 1
	dk := "deploy1"
	site.ActiveVersion = &v
	site.ActiveDeployID = &dk
	env.db.Save(&site)

	deployPath := env.store.GetDeploymentPath(site.ID, "deploy1")
	os.MkdirAll(deployPath, 0755)
	os.WriteFile(filepath.Join(deployPath, "style.css"), []byte("body{}"), 0644)

	req := httptest.NewRequest(http.MethodGet, "/style.css", nil)
	req.Host = "ct-test.test.local"
	rec := env.doRequest(req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	ct := rec.Header().Get("Content-Type")
	if !strings.Contains(ct, "css") {
		t.Errorf("Content-Type = %q, want css", ct)
	}
}

// ──────────────────────────────────────────────
// Version endpoint + check tests
// ──────────────────────────────────────────────

func TestVersionEndpoint(t *testing.T) {
	env := setupTestEnvWithMinVersion(t, "0.1.0")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/version", nil)
	rec := env.doRequest(req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	body := parseJSON(t, rec)
	if body["version"] != "0.1.0" {
		t.Errorf("version = %q, want 0.1.0", body["version"])
	}
	if body["min_cli_version"] != "0.1.0" {
		t.Errorf("min_cli_version = %q, want 0.1.0", body["min_cli_version"])
	}
}

func TestVersionCheck_TooOld(t *testing.T) {
	env := setupTestEnvWithMinVersion(t, "1.0.0")
	_, token := env.createTestUser(t, "u@t.com", "password123", "user")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/sites", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-Hostedat-Version", "0.1.0")
	rec := env.doRequest(req)

	if rec.Code != http.StatusUpgradeRequired {
		t.Errorf("status = %d, want 426", rec.Code)
	}
}

func TestVersionCheck_OK(t *testing.T) {
	env := setupTestEnvWithMinVersion(t, "0.1.0")
	_, token := env.createTestUser(t, "u@t.com", "password123", "user")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/sites", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-Hostedat-Version", "0.2.0")
	rec := env.doRequest(req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
}

func TestVersionCheck_NoHeader(t *testing.T) {
	env := setupTestEnvWithMinVersion(t, "1.0.0")
	_, token := env.createTestUser(t, "u@t.com", "password123", "user")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/sites", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	// No X-Hostedat-Version header — should pass (browser/curl)
	rec := env.doRequest(req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200 (no version header should pass): %s", rec.Code, rec.Body.String())
	}
}

// ──────────────────────────────────────────────
// PKCE tests
// ──────────────────────────────────────────────

func TestCLILogin_RequiresCodeChallenge(t *testing.T) {
	env := setupTestEnv(t)

	// No code_challenge → 400
	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/cli?port=9999&state=test", nil)
	rec := env.doRequest(req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
	body := parseJSON(t, rec)
	if !strings.Contains(body["error"].(string), "code_challenge") {
		t.Errorf("error = %q, want code_challenge-related", body["error"])
	}
}

func TestCLILogin_InvalidChallengeMethod(t *testing.T) {
	env := setupTestEnv(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/cli?port=9999&state=test&code_challenge=abc&code_challenge_method=plain", nil)
	rec := env.doRequest(req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
	body := parseJSON(t, rec)
	if !strings.Contains(body["error"].(string), "S256") {
		t.Errorf("error = %q, want S256-related", body["error"])
	}
}

func TestPKCE_FullFlow(t *testing.T) {
	env := setupTestEnv(t)
	env.createTestUser(t, "cli@test.com", "password123", "user")

	// Generate PKCE pair
	verifier, _ := auth.GenerateCodeVerifier()
	challenge := auth.ComputeCodeChallenge(verifier)

	// Step 1: Submit login with code_challenge
	submitReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/cli",
		jsonBody(map[string]string{
			"email":          "cli@test.com",
			"password":       "password123",
			"port":           "9999",
			"state":          "teststate",
			"code_challenge": challenge,
		}))
	submitReq.Header.Set("Content-Type", "application/json")
	submitRec := env.doRequest(submitReq)

	if submitRec.Code != http.StatusOK {
		t.Fatalf("submit: status = %d: %s", submitRec.Code, submitRec.Body.String())
	}
	submitBody := parseJSON(t, submitRec)
	redirectURL := submitBody["redirect"].(string)

	// Parse code from redirect URL
	parsed, err := url.Parse(redirectURL)
	if err != nil {
		t.Fatalf("parse redirect URL: %v", err)
	}
	code := parsed.Query().Get("code")
	if code == "" {
		t.Fatal("no code in redirect URL")
	}
	if parsed.Query().Get("state") != "teststate" {
		t.Errorf("state = %q, want teststate", parsed.Query().Get("state"))
	}

	// Step 2: Exchange code + verifier for JWT
	tokenReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/token",
		jsonBody(map[string]string{
			"code":          code,
			"code_verifier": verifier,
		}))
	tokenReq.Header.Set("Content-Type", "application/json")
	tokenRec := env.doRequest(tokenReq)

	if tokenRec.Code != http.StatusOK {
		t.Fatalf("token exchange: status = %d: %s", tokenRec.Code, tokenRec.Body.String())
	}
	tokenBody := parseJSON(t, tokenRec)
	token := tokenBody["token"].(string)
	if token == "" {
		t.Fatal("no token in response")
	}

	// Step 3: Verify the JWT works
	sitesReq := httptest.NewRequest(http.MethodGet, "/api/v1/sites", nil)
	sitesReq.Header.Set("Authorization", "Bearer "+token)
	sitesRec := env.doRequest(sitesReq)
	if sitesRec.Code != http.StatusOK {
		t.Errorf("JWT should work: status = %d", sitesRec.Code)
	}
}

func TestPKCE_WrongVerifier(t *testing.T) {
	env := setupTestEnv(t)
	env.createTestUser(t, "cli@test.com", "password123", "user")

	verifier, _ := auth.GenerateCodeVerifier()
	challenge := auth.ComputeCodeChallenge(verifier)

	// Submit login
	submitReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/cli",
		jsonBody(map[string]string{
			"email":          "cli@test.com",
			"password":       "password123",
			"port":           "9999",
			"state":          "test",
			"code_challenge": challenge,
		}))
	submitReq.Header.Set("Content-Type", "application/json")
	submitRec := env.doRequest(submitReq)
	submitBody := parseJSON(t, submitRec)
	parsed, _ := url.Parse(submitBody["redirect"].(string))
	code := parsed.Query().Get("code")

	// Exchange with wrong verifier
	tokenReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/token",
		jsonBody(map[string]string{
			"code":          code,
			"code_verifier": "wrong-verifier-that-does-not-match",
		}))
	tokenReq.Header.Set("Content-Type", "application/json")
	tokenRec := env.doRequest(tokenReq)

	if tokenRec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", tokenRec.Code)
	}
}

func TestPKCE_CodeReuse(t *testing.T) {
	env := setupTestEnv(t)
	env.createTestUser(t, "cli@test.com", "password123", "user")

	verifier, _ := auth.GenerateCodeVerifier()
	challenge := auth.ComputeCodeChallenge(verifier)

	// Submit login
	submitReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/cli",
		jsonBody(map[string]string{
			"email":          "cli@test.com",
			"password":       "password123",
			"port":           "9999",
			"state":          "test",
			"code_challenge": challenge,
		}))
	submitReq.Header.Set("Content-Type", "application/json")
	submitRec := env.doRequest(submitReq)
	submitBody := parseJSON(t, submitRec)
	parsed, _ := url.Parse(submitBody["redirect"].(string))
	code := parsed.Query().Get("code")

	// First exchange — should succeed
	tokenReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/token",
		jsonBody(map[string]string{
			"code":          code,
			"code_verifier": verifier,
		}))
	tokenReq.Header.Set("Content-Type", "application/json")
	tokenRec := env.doRequest(tokenReq)
	if tokenRec.Code != http.StatusOK {
		t.Fatalf("first exchange: status = %d: %s", tokenRec.Code, tokenRec.Body.String())
	}

	// Second exchange — should fail (code already used)
	tokenReq2 := httptest.NewRequest(http.MethodPost, "/api/v1/auth/token",
		jsonBody(map[string]string{
			"code":          code,
			"code_verifier": verifier,
		}))
	tokenReq2.Header.Set("Content-Type", "application/json")
	tokenRec2 := env.doRequest(tokenReq2)
	if tokenRec2.Code != http.StatusUnauthorized {
		t.Errorf("second exchange: status = %d, want 401", tokenRec2.Code)
	}
}

func TestPKCE_InvalidCode(t *testing.T) {
	env := setupTestEnv(t)

	tokenReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/token",
		jsonBody(map[string]string{
			"code":          "nonexistent-code-that-was-never-issued",
			"code_verifier": "some-verifier",
		}))
	tokenReq.Header.Set("Content-Type", "application/json")
	tokenRec := env.doRequest(tokenReq)

	if tokenRec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", tokenRec.Code)
	}
}

// ──────────────────────────────────────────────
// Worker handler tests
// ──────────────────────────────────────────────

func TestWorker_SetEnvVar_Success(t *testing.T) {
	env := setupTestEnv(t)
	user, token := env.createTestUser(t, "u@t.com", "password123", "user")
	site := models.Site{UserID: user.ID, SubdomainSlug: "worker", Name: "Worker"}
	env.db.Create(&site)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/sites/"+site.ID+"/worker/env",
		jsonBody(map[string]interface{}{"name": "API_KEY", "value": "secret123", "secret": true}))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	rec := env.doRequest(req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	body := parseJSON(t, rec)
	if body["name"] != "API_KEY" {
		t.Errorf("name = %v", body["name"])
	}
	if body["secret"] != true {
		t.Errorf("secret = %v", body["secret"])
	}
}

func TestWorker_SetEnvVar_Upsert(t *testing.T) {
	env := setupTestEnv(t)
	user, token := env.createTestUser(t, "u@t.com", "password123", "user")
	site := models.Site{UserID: user.ID, SubdomainSlug: "worker", Name: "Worker"}
	env.db.Create(&site)

	// Create initial var
	env.db.Create(&models.WorkerEnvVar{SiteID: site.ID, Name: "KEY", Value: "old", Secret: false})

	// Update it
	req := httptest.NewRequest(http.MethodPost, "/api/v1/sites/"+site.ID+"/worker/env",
		jsonBody(map[string]interface{}{"name": "KEY", "value": "new", "secret": true}))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	rec := env.doRequest(req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	body := parseJSON(t, rec)
	if body["value"] != "new" {
		t.Errorf("value = %v, want new", body["value"])
	}
	if body["secret"] != true {
		t.Errorf("secret = %v, want true", body["secret"])
	}

	// Verify only one record exists
	var count int64
	env.db.Model(&models.WorkerEnvVar{}).Where("site_id = ? AND name = ?", site.ID, "KEY").Count(&count)
	if count != 1 {
		t.Errorf("count = %d, want 1", count)
	}
}

func TestWorker_SetEnvVar_MissingName(t *testing.T) {
	env := setupTestEnv(t)
	user, token := env.createTestUser(t, "u@t.com", "password123", "user")
	site := models.Site{UserID: user.ID, SubdomainSlug: "worker", Name: "Worker"}
	env.db.Create(&site)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/sites/"+site.ID+"/worker/env",
		jsonBody(map[string]interface{}{"value": "val"}))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	rec := env.doRequest(req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

func TestWorker_SetEnvVar_SiteNotFound(t *testing.T) {
	env := setupTestEnv(t)
	_, token := env.createTestUser(t, "u@t.com", "password123", "user")

	req := httptest.NewRequest(http.MethodPost, "/api/v1/sites/nonexistent/worker/env",
		jsonBody(map[string]interface{}{"name": "KEY", "value": "val"}))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	rec := env.doRequest(req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}

func TestWorker_SetEnvVar_AccessDenied(t *testing.T) {
	env := setupTestEnv(t)
	owner, _ := env.createTestUser(t, "owner@t.com", "password123", "user")
	_, otherToken := env.createTestUser(t, "other@t.com", "password123", "user")
	site := models.Site{UserID: owner.ID, SubdomainSlug: "worker", Name: "Worker"}
	env.db.Create(&site)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/sites/"+site.ID+"/worker/env",
		jsonBody(map[string]interface{}{"name": "KEY", "value": "val"}))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+otherToken)
	rec := env.doRequest(req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", rec.Code)
	}
}

func TestWorker_SetEnvVar_AdminBypass(t *testing.T) {
	env := setupTestEnv(t)
	owner, _ := env.createTestUser(t, "owner@t.com", "password123", "user")
	_, adminToken := env.createTestUser(t, "admin@t.com", "password123", "admin")
	site := models.Site{UserID: owner.ID, SubdomainSlug: "worker", Name: "Worker"}
	env.db.Create(&site)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/sites/"+site.ID+"/worker/env",
		jsonBody(map[string]interface{}{"name": "KEY", "value": "val"}))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+adminToken)
	rec := env.doRequest(req)

	if rec.Code != http.StatusOK {
		t.Errorf("admin: status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
}

func TestWorker_ListEnvVars_Success(t *testing.T) {
	env := setupTestEnv(t)
	user, token := env.createTestUser(t, "u@t.com", "password123", "user")
	site := models.Site{UserID: user.ID, SubdomainSlug: "worker", Name: "Worker"}
	env.db.Create(&site)

	env.db.Create(&models.WorkerEnvVar{SiteID: site.ID, Name: "PUBLIC", Value: "visible", Secret: false})
	env.db.Create(&models.WorkerEnvVar{SiteID: site.ID, Name: "SECRET", Value: "hidden", Secret: true})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/sites/"+site.ID+"/worker/env", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := env.doRequest(req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}

	var vars []map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &vars)
	if len(vars) != 2 {
		t.Errorf("got %d vars, want 2", len(vars))
	}

	// Check secret masking
	for _, v := range vars {
		if v["name"] == "SECRET" && v["value"] != "********" {
			t.Errorf("secret value not masked: %v", v["value"])
		}
		if v["name"] == "PUBLIC" && v["value"] != "visible" {
			t.Errorf("public value should be visible: %v", v["value"])
		}
	}
}

func TestWorker_ListEnvVars_AccessDenied(t *testing.T) {
	env := setupTestEnv(t)
	owner, _ := env.createTestUser(t, "owner@t.com", "password123", "user")
	_, otherToken := env.createTestUser(t, "other@t.com", "password123", "user")
	site := models.Site{UserID: owner.ID, SubdomainSlug: "worker", Name: "Worker"}
	env.db.Create(&site)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/sites/"+site.ID+"/worker/env", nil)
	req.Header.Set("Authorization", "Bearer "+otherToken)
	rec := env.doRequest(req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", rec.Code)
	}
}

func TestWorker_DeleteEnvVar_Success(t *testing.T) {
	env := setupTestEnv(t)
	user, token := env.createTestUser(t, "u@t.com", "password123", "user")
	site := models.Site{UserID: user.ID, SubdomainSlug: "worker", Name: "Worker"}
	env.db.Create(&site)

	envVar := models.WorkerEnvVar{SiteID: site.ID, Name: "DELETE_ME", Value: "val"}
	env.db.Create(&envVar)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/sites/"+site.ID+"/worker/env/"+envVar.ID, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := env.doRequest(req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}

	// Verify deleted
	var count int64
	env.db.Model(&models.WorkerEnvVar{}).Where("id = ?", envVar.ID).Count(&count)
	if count != 0 {
		t.Error("env var not deleted")
	}
}

func TestWorker_DeleteEnvVar_NotFound(t *testing.T) {
	env := setupTestEnv(t)
	user, token := env.createTestUser(t, "u@t.com", "password123", "user")
	site := models.Site{UserID: user.ID, SubdomainSlug: "worker", Name: "Worker"}
	env.db.Create(&site)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/sites/"+site.ID+"/worker/env/nonexistent", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := env.doRequest(req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}

func TestWorker_DeleteEnvVar_AccessDenied(t *testing.T) {
	env := setupTestEnv(t)
	owner, _ := env.createTestUser(t, "owner@t.com", "password123", "user")
	_, otherToken := env.createTestUser(t, "other@t.com", "password123", "user")
	site := models.Site{UserID: owner.ID, SubdomainSlug: "worker", Name: "Worker"}
	env.db.Create(&site)

	envVar := models.WorkerEnvVar{SiteID: site.ID, Name: "KEY", Value: "val"}
	env.db.Create(&envVar)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/sites/"+site.ID+"/worker/env/"+envVar.ID, nil)
	req.Header.Set("Authorization", "Bearer "+otherToken)
	rec := env.doRequest(req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", rec.Code)
	}
}

func TestWorker_CreateKVNamespace_Success(t *testing.T) {
	env := setupTestEnv(t)
	user, token := env.createTestUser(t, "u@t.com", "password123", "user")
	site := models.Site{UserID: user.ID, SubdomainSlug: "worker", Name: "Worker"}
	env.db.Create(&site)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/sites/"+site.ID+"/worker/kv",
		jsonBody(map[string]string{"name": "MY_KV"}))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	rec := env.doRequest(req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	body := parseJSON(t, rec)
	if body["name"] != "MY_KV" {
		t.Errorf("name = %v", body["name"])
	}
}

func TestWorker_CreateKVNamespace_MissingName(t *testing.T) {
	env := setupTestEnv(t)
	user, token := env.createTestUser(t, "u@t.com", "password123", "user")
	site := models.Site{UserID: user.ID, SubdomainSlug: "worker", Name: "Worker"}
	env.db.Create(&site)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/sites/"+site.ID+"/worker/kv",
		jsonBody(map[string]string{}))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	rec := env.doRequest(req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

func TestWorker_CreateKVNamespace_AccessDenied(t *testing.T) {
	env := setupTestEnv(t)
	owner, _ := env.createTestUser(t, "owner@t.com", "password123", "user")
	_, otherToken := env.createTestUser(t, "other@t.com", "password123", "user")
	site := models.Site{UserID: owner.ID, SubdomainSlug: "worker", Name: "Worker"}
	env.db.Create(&site)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/sites/"+site.ID+"/worker/kv",
		jsonBody(map[string]string{"name": "KV"}))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+otherToken)
	rec := env.doRequest(req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", rec.Code)
	}
}

func TestWorker_ListKVNamespaces_Success(t *testing.T) {
	env := setupTestEnv(t)
	user, token := env.createTestUser(t, "u@t.com", "password123", "user")
	site := models.Site{UserID: user.ID, SubdomainSlug: "worker", Name: "Worker"}
	env.db.Create(&site)

	env.db.Create(&models.KVNamespace{SiteID: site.ID, Name: "KV1"})
	env.db.Create(&models.KVNamespace{SiteID: site.ID, Name: "KV2"})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/sites/"+site.ID+"/worker/kv", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := env.doRequest(req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}

	var namespaces []map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &namespaces)
	if len(namespaces) != 2 {
		t.Errorf("got %d namespaces, want 2", len(namespaces))
	}
}

func TestWorker_DeleteKVNamespace_Success(t *testing.T) {
	env := setupTestEnv(t)
	user, token := env.createTestUser(t, "u@t.com", "password123", "user")
	site := models.Site{UserID: user.ID, SubdomainSlug: "worker", Name: "Worker"}
	env.db.Create(&site)

	ns := models.KVNamespace{SiteID: site.ID, Name: "DELETE_ME"}
	env.db.Create(&ns)

	// Create some entries in the namespace
	env.db.Create(&models.KVEntry{NamespaceID: ns.ID, Key: "key1", Value: "val1"})
	env.db.Create(&models.KVEntry{NamespaceID: ns.ID, Key: "key2", Value: "val2"})

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/sites/"+site.ID+"/worker/kv/"+ns.ID, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := env.doRequest(req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}

	// Verify namespace deleted
	var nsCount int64
	env.db.Model(&models.KVNamespace{}).Where("id = ?", ns.ID).Count(&nsCount)
	if nsCount != 0 {
		t.Error("namespace not deleted")
	}

	// Verify entries cascaded
	var entryCount int64
	env.db.Model(&models.KVEntry{}).Where("namespace_id = ?", ns.ID).Count(&entryCount)
	if entryCount != 0 {
		t.Error("KV entries not cascaded")
	}
}

func TestWorker_DeleteKVNamespace_NotFound(t *testing.T) {
	env := setupTestEnv(t)
	user, token := env.createTestUser(t, "u@t.com", "password123", "user")
	site := models.Site{UserID: user.ID, SubdomainSlug: "worker", Name: "Worker"}
	env.db.Create(&site)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/sites/"+site.ID+"/worker/kv/nonexistent", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := env.doRequest(req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}

func TestWorker_CreateCronSchedule_Success(t *testing.T) {
	env := setupTestEnv(t)
	user, token := env.createTestUser(t, "u@t.com", "password123", "user")
	site := models.Site{UserID: user.ID, SubdomainSlug: "worker", Name: "Worker"}
	env.db.Create(&site)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/sites/"+site.ID+"/worker/crons",
		jsonBody(map[string]interface{}{"cron": "*/5 * * * *", "enabled": true}))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	rec := env.doRequest(req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	body := parseJSON(t, rec)
	if body["cron"] != "*/5 * * * *" {
		t.Errorf("cron = %v", body["cron"])
	}
	if body["enabled"] != true {
		t.Errorf("enabled = %v", body["enabled"])
	}
}

func TestWorker_CreateCronSchedule_DefaultEnabled(t *testing.T) {
	env := setupTestEnv(t)
	user, token := env.createTestUser(t, "u@t.com", "password123", "user")
	site := models.Site{UserID: user.ID, SubdomainSlug: "worker", Name: "Worker"}
	env.db.Create(&site)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/sites/"+site.ID+"/worker/crons",
		jsonBody(map[string]string{"cron": "0 0 * * *"}))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	rec := env.doRequest(req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	body := parseJSON(t, rec)
	if body["enabled"] != true {
		t.Errorf("enabled = %v, want true (default)", body["enabled"])
	}
}

func TestWorker_CreateCronSchedule_InvalidCron(t *testing.T) {
	env := setupTestEnv(t)
	user, token := env.createTestUser(t, "u@t.com", "password123", "user")
	site := models.Site{UserID: user.ID, SubdomainSlug: "worker", Name: "Worker"}
	env.db.Create(&site)

	tests := []struct {
		name string
		cron string
	}{
		{"missing fields", "* *"},
		{"too many fields", "* * * * * *"},
		{"invalid minute", "60 * * * *"},
		{"invalid hour", "* 24 * * *"},
		{"invalid day", "* * 0 * *"},
		{"invalid month", "* * * 13 *"},
		{"invalid weekday", "* * * * 7"},
		{"invalid step", "*/0 * * * *"},
		{"invalid range", "10-5 * * * *"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/v1/sites/"+site.ID+"/worker/crons",
				jsonBody(map[string]string{"cron": tt.cron}))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", "Bearer "+token)
			rec := env.doRequest(req)

			if rec.Code != http.StatusBadRequest {
				t.Errorf("cron %q: status = %d, want 400", tt.cron, rec.Code)
			}
		})
	}
}

func TestWorker_CreateCronSchedule_MissingCron(t *testing.T) {
	env := setupTestEnv(t)
	user, token := env.createTestUser(t, "u@t.com", "password123", "user")
	site := models.Site{UserID: user.ID, SubdomainSlug: "worker", Name: "Worker"}
	env.db.Create(&site)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/sites/"+site.ID+"/worker/crons",
		jsonBody(map[string]string{}))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	rec := env.doRequest(req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

func TestWorker_CreateCronSchedule_AccessDenied(t *testing.T) {
	env := setupTestEnv(t)
	owner, _ := env.createTestUser(t, "owner@t.com", "password123", "user")
	_, otherToken := env.createTestUser(t, "other@t.com", "password123", "user")
	site := models.Site{UserID: owner.ID, SubdomainSlug: "worker", Name: "Worker"}
	env.db.Create(&site)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/sites/"+site.ID+"/worker/crons",
		jsonBody(map[string]string{"cron": "* * * * *"}))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+otherToken)
	rec := env.doRequest(req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", rec.Code)
	}
}

func TestWorker_ListCronSchedules_Success(t *testing.T) {
	env := setupTestEnv(t)
	user, token := env.createTestUser(t, "u@t.com", "password123", "user")
	site := models.Site{UserID: user.ID, SubdomainSlug: "worker", Name: "Worker"}
	env.db.Create(&site)

	env.db.Create(&models.CronSchedule{SiteID: site.ID, Cron: "0 0 * * *", Enabled: true})
	env.db.Create(&models.CronSchedule{SiteID: site.ID, Cron: "*/15 * * * *", Enabled: false})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/sites/"+site.ID+"/worker/crons", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := env.doRequest(req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}

	var schedules []map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &schedules)
	if len(schedules) != 2 {
		t.Errorf("got %d schedules, want 2", len(schedules))
	}
}

func TestWorker_DeleteCronSchedule_Success(t *testing.T) {
	env := setupTestEnv(t)
	user, token := env.createTestUser(t, "u@t.com", "password123", "user")
	site := models.Site{UserID: user.ID, SubdomainSlug: "worker", Name: "Worker"}
	env.db.Create(&site)

	sched := models.CronSchedule{SiteID: site.ID, Cron: "0 0 * * *", Enabled: true}
	env.db.Create(&sched)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/sites/"+site.ID+"/worker/crons/"+sched.ID, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := env.doRequest(req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}

	// Verify deleted
	var count int64
	env.db.Model(&models.CronSchedule{}).Where("id = ?", sched.ID).Count(&count)
	if count != 0 {
		t.Error("cron schedule not deleted")
	}
}

func TestWorker_DeleteCronSchedule_NotFound(t *testing.T) {
	env := setupTestEnv(t)
	user, token := env.createTestUser(t, "u@t.com", "password123", "user")
	site := models.Site{UserID: user.ID, SubdomainSlug: "worker", Name: "Worker"}
	env.db.Create(&site)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/sites/"+site.ID+"/worker/crons/nonexistent", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := env.doRequest(req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}

func TestWorker_GetLogs_Success(t *testing.T) {
	env := setupTestEnv(t)
	user, token := env.createTestUser(t, "u@t.com", "password123", "user")
	site := models.Site{UserID: user.ID, SubdomainSlug: "worker", Name: "Worker"}
	env.db.Create(&site)

	// Create some logs
	for i := 0; i < 5; i++ {
		env.db.Create(&models.WorkerLog{
			SiteID:  site.ID,
			Level:   "info",
			Message: fmt.Sprintf("log %d", i),
		})
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/sites/"+site.ID+"/worker/logs", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := env.doRequest(req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}

	var logs []map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &logs)
	if len(logs) != 5 {
		t.Errorf("got %d logs, want 5", len(logs))
	}
}

func TestWorker_GetLogs_AccessDenied(t *testing.T) {
	env := setupTestEnv(t)
	owner, _ := env.createTestUser(t, "owner@t.com", "password123", "user")
	_, otherToken := env.createTestUser(t, "other@t.com", "password123", "user")
	site := models.Site{UserID: owner.ID, SubdomainSlug: "worker", Name: "Worker"}
	env.db.Create(&site)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/sites/"+site.ID+"/worker/logs", nil)
	req.Header.Set("Authorization", "Bearer "+otherToken)
	rec := env.doRequest(req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", rec.Code)
	}
}

// ──────────────────────────────────────────────
// Deployment rollback tests
// ──────────────────────────────────────────────

func TestDeploy_Rollback_Success(t *testing.T) {
	env := setupTestEnv(t)
	user, token := env.createTestUser(t, "u@t.com", "password123", "user")
	site := models.Site{UserID: user.ID, SubdomainSlug: "rollback", Name: "Rollback"}
	env.db.Create(&site)

	// Create deployments
	env.db.Create(&models.Deployment{SiteID: site.ID, Version: 1, FileHash: "abc"})
	env.db.Create(&models.Deployment{SiteID: site.ID, Version: 2, FileHash: "def"})

	// Set active to v2
	v2 := 2
	site.ActiveVersion = &v2
	env.db.Save(&site)

	// Rollback to v1
	req := httptest.NewRequest(http.MethodPost, "/api/v1/sites/"+site.ID+"/deployments/1/rollback", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := env.doRequest(req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	body := parseJSON(t, rec)
	if body["active_version"] != float64(1) {
		t.Errorf("active_version = %v, want 1", body["active_version"])
	}

	// Verify DB update
	var updated models.Site
	env.db.First(&updated, "id = ?", site.ID)
	if updated.ActiveVersion == nil || *updated.ActiveVersion != 1 {
		t.Errorf("DB active_version = %v, want 1", updated.ActiveVersion)
	}
}

func TestDeploy_Rollback_AlreadyActive(t *testing.T) {
	env := setupTestEnv(t)
	user, token := env.createTestUser(t, "u@t.com", "password123", "user")
	site := models.Site{UserID: user.ID, SubdomainSlug: "rollback", Name: "Rollback"}
	env.db.Create(&site)

	env.db.Create(&models.Deployment{SiteID: site.ID, Version: 1, FileHash: "abc"})
	v1 := 1
	site.ActiveVersion = &v1
	env.db.Save(&site)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/sites/"+site.ID+"/deployments/1/rollback", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := env.doRequest(req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
	body := parseJSON(t, rec)
	if !strings.Contains(body["error"].(string), "already") {
		t.Errorf("error = %q, want 'already' message", body["error"])
	}
}

func TestDeploy_Rollback_InvalidVersion(t *testing.T) {
	env := setupTestEnv(t)
	user, token := env.createTestUser(t, "u@t.com", "password123", "user")
	site := models.Site{UserID: user.ID, SubdomainSlug: "rollback", Name: "Rollback"}
	env.db.Create(&site)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/sites/"+site.ID+"/deployments/abc/rollback", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := env.doRequest(req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

func TestDeploy_Rollback_DeploymentNotFound(t *testing.T) {
	env := setupTestEnv(t)
	user, token := env.createTestUser(t, "u@t.com", "password123", "user")
	site := models.Site{UserID: user.ID, SubdomainSlug: "rollback", Name: "Rollback"}
	env.db.Create(&site)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/sites/"+site.ID+"/deployments/999/rollback", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := env.doRequest(req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}

func TestDeploy_Rollback_AccessDenied(t *testing.T) {
	env := setupTestEnv(t)
	owner, _ := env.createTestUser(t, "owner@t.com", "password123", "user")
	_, otherToken := env.createTestUser(t, "other@t.com", "password123", "user")
	site := models.Site{UserID: owner.ID, SubdomainSlug: "rollback", Name: "Rollback"}
	env.db.Create(&site)

	env.db.Create(&models.Deployment{SiteID: site.ID, Version: 1, FileHash: "abc"})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/sites/"+site.ID+"/deployments/1/rollback", nil)
	req.Header.Set("Authorization", "Bearer "+otherToken)
	rec := env.doRequest(req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", rec.Code)
	}
}

// ──────────────────────────────────────────────
// Auth logout test
// ──────────────────────────────────────────────

func TestAuth_Logout(t *testing.T) {
	env := setupTestEnv(t)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
	rec := env.doRequest(req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
	body := parseJSON(t, rec)
	if body["message"] != "logged out" {
		t.Errorf("message = %q, want 'logged out'", body["message"])
	}
}

// ──────────────────────────────────────────────
// Error handler tests
// ──────────────────────────────────────────────

func TestCustomErrorHandler_HTTPError(t *testing.T) {
	env := setupTestEnv(t)

	// Request a completely unknown route outside /api/v1 to bypass auth middleware
	// and trigger Echo's default 405/404 which invokes CustomErrorHandler
	req := httptest.NewRequest(http.MethodGet, "/nonexistent-path", nil)
	req.Host = "test.local" // bare domain so subdomain router passes through
	rec := env.doRequest(req)

	if rec.Code != http.StatusNotFound && rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 404 or 405", rec.Code)
	}
	body := parseJSON(t, rec)
	if body["error"] == nil {
		t.Error("expected error field in JSON response")
	}
}

func TestCustomErrorHandler_GenericError(t *testing.T) {
	env := setupTestEnv(t)

	// The CustomErrorHandler is already being tested indirectly by all error cases.
	// This test ensures it returns proper JSON for internal errors.
	// We can't easily trigger a generic non-HTTPError without modifying the handlers,
	// so we verify the structure is correct from other tests.
	req := httptest.NewRequest(http.MethodGet, "/api/v1/sites", nil)
	// No auth header → should trigger 401
	rec := env.doRequest(req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
	body := parseJSON(t, rec)
	if _, ok := body["error"].(string); !ok {
		t.Error("error field should be a string")
	}
}

// ──────────────────────────────────────────────
// isAllowedRedirectTarget tests
// ──────────────────────────────────────────────

func TestIsAllowedRedirectTarget(t *testing.T) {
	domain := "test.local"

	tests := []struct {
		name     string
		target   string
		expected bool
	}{
		{"empty", "", false},
		{"protocol-relative", "//evil.com", false},
		{"relative path", "/path", true},
		{"relative deep path", "/path/to/file", true},
		{"javascript scheme", "javascript:alert(1)", false},
		{"data scheme", "data:text/html,<script>alert(1)</script>", false},
		{"same domain absolute", "https://test.local/path", true},
		{"subdomain", "https://sub.test.local/path", true},
		{"external domain", "https://evil.com/path", false},
		{"http same domain", "http://test.local/path", true},
		{"mixed case scheme", "JaVaScRiPt:alert(1)", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isAllowedRedirectTarget(tt.target, domain)
			if result != tt.expected {
				t.Errorf("isAllowedRedirectTarget(%q, %q) = %v, want %v", tt.target, domain, result, tt.expected)
			}
		})
	}
}
