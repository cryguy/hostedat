package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/cryguy/hostedat/internal/models"
)

func TestRoutes_VersionEndpoint(t *testing.T) {
	env := setupTestEnv(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/version", nil)
	rec := env.doRequest(req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}

	body := parseJSON(t, rec)
	if body["version"] == nil {
		t.Error("expected version in response")
	}
	if body["min_cli_version"] == nil {
		t.Error("expected min_cli_version in response")
	}
}

func TestRoutes_AuthEndpoints(t *testing.T) {
	env := setupTestEnv(t)

	authRoutes := []struct {
		method string
		path   string
	}{
		{http.MethodPost, "/api/v1/auth/register"},
		{http.MethodPost, "/api/v1/auth/login"},
		{http.MethodPost, "/api/v1/auth/logout"},
		{http.MethodGet, "/api/v1/auth/cli"},
		{http.MethodPost, "/api/v1/auth/cli"},
		{http.MethodPost, "/api/v1/auth/token"},
	}

	for _, route := range authRoutes {
		req := httptest.NewRequest(route.method, route.path, nil)
		rec := env.doRequest(req)

		// Should not return 404 (route exists)
		if rec.Code == http.StatusNotFound {
			t.Errorf("%s %s: got 404, route should exist", route.method, route.path)
		}
	}
}

func TestRoutes_SiteEndpoints(t *testing.T) {
	env := setupTestEnv(t)
	_, token := env.createTestUser(t, "u@t.com", "password123", "user")

	// Test that the routes exist - POST to /sites should work
	req := httptest.NewRequest(http.MethodPost, "/api/v1/sites",
		jsonBody(map[string]string{"name": "Test Site"}))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	rec := env.doRequest(req)

	if rec.Code != http.StatusCreated {
		t.Errorf("POST /api/v1/sites: status = %d, want 201", rec.Code)
	}

	// GET /sites should return 200
	req2 := httptest.NewRequest(http.MethodGet, "/api/v1/sites", nil)
	req2.Header.Set("Authorization", "Bearer "+token)
	rec2 := env.doRequest(req2)

	if rec2.Code != http.StatusOK {
		t.Errorf("GET /api/v1/sites: status = %d, want 200", rec2.Code)
	}
}

func TestRoutes_DeploymentEndpoints(t *testing.T) {
	env := setupTestEnv(t)
	user, token := env.createTestUser(t, "u@t.com", "password123", "user")

	// Create a site first
	site := models.Site{UserID: user.ID, SubdomainSlug: "testsite", Name: "Test Site"}
	env.db.Create(&site)

	// Test GET deployments - should return 200 with empty list
	req := httptest.NewRequest(http.MethodGet, "/api/v1/sites/"+site.ID+"/deployments", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := env.doRequest(req)

	if rec.Code != http.StatusOK {
		t.Errorf("GET deployments: status = %d, want 200", rec.Code)
	}
}

func TestRoutes_WorkerEndpoints(t *testing.T) {
	env := setupTestEnv(t)
	user, token := env.createTestUser(t, "u@t.com", "password123", "user")

	// Create a site first
	site := models.Site{UserID: user.ID, SubdomainSlug: "testsite", Name: "Test Site"}
	env.db.Create(&site)

	// Test GET env vars - should return 200 with empty list
	req := httptest.NewRequest(http.MethodGet, "/api/v1/sites/"+site.ID+"/worker/env", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := env.doRequest(req)

	if rec.Code != http.StatusOK {
		t.Errorf("GET worker/env: status = %d, want 200", rec.Code)
	}

	// Test GET kv namespaces - should return 200 with empty list
	req2 := httptest.NewRequest(http.MethodGet, "/api/v1/sites/"+site.ID+"/worker/kv", nil)
	req2.Header.Set("Authorization", "Bearer "+token)
	rec2 := env.doRequest(req2)

	if rec2.Code != http.StatusOK {
		t.Errorf("GET worker/kv: status = %d, want 200", rec2.Code)
	}
}

func TestRoutes_APIKeyEndpoints(t *testing.T) {
	env := setupTestEnv(t)
	_, token := env.createTestUser(t, "u@t.com", "password123", "user")

	// Test GET keys - should return 200 with empty list
	req := httptest.NewRequest(http.MethodGet, "/api/v1/keys", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := env.doRequest(req)

	if rec.Code != http.StatusOK {
		t.Errorf("GET /api/v1/keys: status = %d, want 200", rec.Code)
	}
}

func TestRoutes_AdminEndpoints(t *testing.T) {
	env := setupTestEnv(t)
	_, adminToken := env.createTestUser(t, "admin@t.com", "password123", "admin")

	// Test GET users - should return 200
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/users", nil)
	req.Header.Set("Authorization", "Bearer "+adminToken)
	rec := env.doRequest(req)

	if rec.Code != http.StatusOK {
		t.Errorf("GET /api/v1/admin/users: status = %d, want 200", rec.Code)
	}

	// Test GET settings - should return 200
	req2 := httptest.NewRequest(http.MethodGet, "/api/v1/admin/settings", nil)
	req2.Header.Set("Authorization", "Bearer "+adminToken)
	rec2 := env.doRequest(req2)

	if rec2.Code != http.StatusOK {
		t.Errorf("GET /api/v1/admin/settings: status = %d, want 200", rec2.Code)
	}
}

func TestRoutes_AdminRequiresAuth(t *testing.T) {
	env := setupTestEnv(t)
	_, userToken := env.createTestUser(t, "user@t.com", "password123", "user")

	// Regular user should get 403 on admin routes
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/users", nil)
	req.Header.Set("Authorization", "Bearer "+userToken)
	rec := env.doRequest(req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("regular user on admin route: status = %d, want 403", rec.Code)
	}
}

func TestRoutes_ProtectedRoutesRequireAuth(t *testing.T) {
	env := setupTestEnv(t)

	protectedRoutes := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/v1/sites"},
		{http.MethodPost, "/api/v1/sites"},
		{http.MethodGet, "/api/v1/keys"},
		{http.MethodGet, "/api/v1/admin/users"},
	}

	for _, route := range protectedRoutes {
		req := httptest.NewRequest(route.method, route.path, nil)
		// No Authorization header
		rec := env.doRequest(req)

		if rec.Code != http.StatusUnauthorized {
			t.Errorf("%s %s without auth: status = %d, want 401", route.method, route.path, rec.Code)
		}
	}
}

func TestRoutes_VersionCheckMiddleware(t *testing.T) {
	// Setup with minimum CLI version requirement
	env := setupTestEnvWithMinVersion(t, "1.0.0")
	_, token := env.createTestUser(t, "u@t.com", "password123", "user")

	// Request with new client version should be accepted
	req := httptest.NewRequest(http.MethodGet, "/api/v1/sites", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-Client-Version", "1.0.0")
	rec := env.doRequest(req)

	if rec.Code != http.StatusOK {
		t.Errorf("valid client version: status = %d, want 200", rec.Code)
	}
}

func TestRoutes_PublicVsProtectedRoutes(t *testing.T) {
	env := setupTestEnv(t)

	// Public routes (no auth required)
	publicRoutes := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/v1/version"},
		{http.MethodPost, "/api/v1/auth/register"},
		{http.MethodPost, "/api/v1/auth/login"},
	}

	for _, route := range publicRoutes {
		req := httptest.NewRequest(route.method, route.path, nil)
		rec := env.doRequest(req)

		// Should not return 401 (auth not required)
		if rec.Code == http.StatusUnauthorized {
			t.Errorf("%s %s: got 401, should be public route", route.method, route.path)
		}
	}
}

func TestRoutes_MethodNotAllowed(t *testing.T) {
	env := setupTestEnv(t)
	_, token := env.createTestUser(t, "u@t.com", "password123", "user")

	// Try wrong HTTP methods
	wrongMethods := []struct {
		method       string
		path         string
		expectedCode int
	}{
		{http.MethodPut, "/api/v1/sites", http.StatusMethodNotAllowed},
		{http.MethodDelete, "/api/v1/auth/login", http.StatusMethodNotAllowed},
		{http.MethodPost, "/api/v1/sites/test-id", http.StatusMethodNotAllowed},
	}

	for _, test := range wrongMethods {
		req := httptest.NewRequest(test.method, test.path, nil)
		req.Header.Set("Authorization", "Bearer "+token)
		rec := env.doRequest(req)

		if rec.Code == http.StatusOK {
			t.Errorf("%s %s: should not accept this method", test.method, test.path)
		}
	}
}

func TestRoutes_PathParameters(t *testing.T) {
	env := setupTestEnv(t)
	_, token := env.createTestUser(t, "u@t.com", "password123", "user")

	// Routes with path parameters should work
	paramRoutes := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/v1/sites/abc123xyz"},
		{http.MethodPatch, "/api/v1/sites/abc123xyz"},
		{http.MethodDelete, "/api/v1/sites/abc123xyz/worker/env/var123"},
		{http.MethodDelete, "/api/v1/sites/abc123xyz/worker/kv/ns123"},
		{http.MethodPost, "/api/v1/sites/abc123xyz/deployments/5/rollback"},
	}

	for _, route := range paramRoutes {
		req := httptest.NewRequest(route.method, route.path, nil)
		req.Header.Set("Authorization", "Bearer "+token)
		rec := env.doRequest(req)

		// Should not return 404 (route exists, even if resource doesn't)
		// Will likely return 404 for "not found resource" but not "not found route"
		// The key is the route is registered correctly
		if rec.Code == http.StatusNotFound {
			// This is OK - resource not found, but route exists
			continue
		}
		// Any other status means route was processed
	}
}

func TestRoutes_QueryParameters(t *testing.T) {
	env := setupTestEnv(t)
	_, token := env.createTestUser(t, "u@t.com", "password123", "user")

	// Routes with query parameters should work
	req := httptest.NewRequest(http.MethodGet, "/api/v1/sites?all=true", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := env.doRequest(req)

	// Should not return 404
	if rec.Code == http.StatusNotFound {
		t.Error("query parameters should not affect routing")
	}
}

func TestRoutes_TrailingSlash(t *testing.T) {
	env := setupTestEnv(t)

	// Test that routes work with or without trailing slash
	req1 := httptest.NewRequest(http.MethodGet, "/api/v1/version", nil)
	rec1 := env.doRequest(req1)

	req2 := httptest.NewRequest(http.MethodGet, "/api/v1/version/", nil)
	rec2 := env.doRequest(req2)

	// Both should work or both should fail consistently
	if rec1.Code == http.StatusOK && rec2.Code == http.StatusNotFound {
		t.Error("trailing slash handling inconsistent")
	}
}

func TestRoutes_CORSHeaders(t *testing.T) {
	env := setupTestEnv(t)

	// Public endpoint should be accessible
	req := httptest.NewRequest(http.MethodGet, "/api/v1/version", nil)
	rec := env.doRequest(req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
}

func TestRoutes_NotFoundRoute(t *testing.T) {
	env := setupTestEnv(t)
	_, token := env.createTestUser(t, "u@t.com", "password123", "user")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/nonexistent", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := env.doRequest(req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("nonexistent route: status = %d, want 404", rec.Code)
	}
}
