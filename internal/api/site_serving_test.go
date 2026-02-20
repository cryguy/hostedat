package api

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/cryguy/hostedat/internal/models"
	"github.com/cryguy/hostedat/internal/storage"
)

func TestStaticSiteHandler_NoDeployment(t *testing.T) {
	env := setupTestEnv(t)
	user, _ := env.createTestUser(t, "u@t.com", "password123", "user")

	site := models.Site{UserID: user.ID, SubdomainSlug: "mysite", Name: "Site"}
	env.db.Create(&site)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Host = "mysite." + env.domain
	rec := env.doRequest(req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404 (no deployment)", rec.Code)
	}
}

func TestStaticSiteHandler_ServeFile(t *testing.T) {
	env := setupTestEnv(t)
	user, _ := env.createTestUser(t, "u@t.com", "password123", "user")

	site := models.Site{UserID: user.ID, SubdomainSlug: "mysite", Name: "Site"}
	env.db.Create(&site)

	// Create deployment and files
	deployID := models.GenerateID()
	version := 1
	env.db.Create(&models.Deployment{ID: deployID, SiteID: site.ID, Version: version, FileHash: "hash"})
	env.db.Model(&site).Updates(map[string]interface{}{
		"active_version":   version,
		"active_deploy_id": deployID,
	})

	deployPath := env.store.GetDeploymentPath(site.ID, deployID)
	_ = os.MkdirAll(deployPath, 0755)
	_ = os.WriteFile(filepath.Join(deployPath, "index.html"), []byte("<html>Hello</html>"), 0644)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Host = "mysite." + env.domain
	rec := env.doRequest(req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
	if rec.Body.String() != "<html>Hello</html>" {
		t.Errorf("body = %q, want %q", rec.Body.String(), "<html>Hello</html>")
	}
}

func TestStaticSiteHandler_BlockInternalFiles(t *testing.T) {
	env := setupTestEnv(t)
	user, _ := env.createTestUser(t, "u@t.com", "password123", "user")

	site := models.Site{UserID: user.ID, SubdomainSlug: "mysite", Name: "Site"}
	env.db.Create(&site)

	deployID := models.GenerateID()
	version := 1
	env.db.Create(&models.Deployment{ID: deployID, SiteID: site.ID, Version: version, FileHash: "hash"})
	env.db.Model(&site).Updates(map[string]interface{}{
		"active_version":   version,
		"active_deploy_id": deployID,
	})

	deployPath := env.store.GetDeploymentPath(site.ID, deployID)
	_ = os.MkdirAll(deployPath, 0755)
	_ = os.WriteFile(filepath.Join(deployPath, "_worker.js"), []byte("worker code"), 0644)
	_ = os.WriteFile(filepath.Join(deployPath, "_headers"), []byte("headers"), 0644)
	_ = os.WriteFile(filepath.Join(deployPath, "_redirects"), []byte("redirects"), 0644)
	_ = os.WriteFile(filepath.Join(deployPath, "_routes.json"), []byte("{}"), 0644)

	internalPaths := []string{"/_worker.js", "/_headers", "/_redirects", "/_routes.json"}
	for _, path := range internalPaths {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.Host = "mysite." + env.domain
		rec := env.doRequest(req)

		if rec.Code != http.StatusNotFound {
			t.Errorf("path %s: status = %d, want 404 (internal file blocked)", path, rec.Code)
		}
	}
}

func TestStaticSiteHandler_SPAMode(t *testing.T) {
	env := setupTestEnv(t)
	user, _ := env.createTestUser(t, "u@t.com", "password123", "user")

	site := models.Site{UserID: user.ID, SubdomainSlug: "mysite", Name: "Site", SPAMode: true}
	env.db.Create(&site)

	deployID := models.GenerateID()
	version := 1
	env.db.Create(&models.Deployment{ID: deployID, SiteID: site.ID, Version: version, FileHash: "hash"})
	env.db.Model(&site).Updates(map[string]interface{}{
		"active_version":   version,
		"active_deploy_id": deployID,
	})

	deployPath := env.store.GetDeploymentPath(site.ID, deployID)
	_ = os.MkdirAll(deployPath, 0755)
	_ = os.WriteFile(filepath.Join(deployPath, "index.html"), []byte("<html>SPA</html>"), 0644)

	// Request non-existent path should return index.html in SPA mode
	req := httptest.NewRequest(http.MethodGet, "/app/page", nil)
	req.Host = "mysite." + env.domain
	rec := env.doRequest(req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200 (SPA fallback)", rec.Code)
	}
	if rec.Body.String() != "<html>SPA</html>" {
		t.Errorf("body = %q, want SPA index.html", rec.Body.String())
	}
}

func TestStaticSiteHandler_Custom404(t *testing.T) {
	env := setupTestEnv(t)
	user, _ := env.createTestUser(t, "u@t.com", "password123", "user")

	site := models.Site{UserID: user.ID, SubdomainSlug: "mysite", Name: "Site", SPAMode: false}
	env.db.Create(&site)

	deployID := models.GenerateID()
	version := 1
	env.db.Create(&models.Deployment{ID: deployID, SiteID: site.ID, Version: version, FileHash: "hash"})
	env.db.Model(&site).Updates(map[string]interface{}{
		"active_version":   version,
		"active_deploy_id": deployID,
	})

	deployPath := env.store.GetDeploymentPath(site.ID, deployID)
	_ = os.MkdirAll(deployPath, 0755)
	_ = os.WriteFile(filepath.Join(deployPath, "404.html"), []byte("<html>Not Found</html>"), 0644)

	req := httptest.NewRequest(http.MethodGet, "/nonexistent", nil)
	req.Host = "mysite." + env.domain
	rec := env.doRequest(req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
	if rec.Body.String() != "<html>Not Found</html>" {
		t.Errorf("body = %q, want custom 404", rec.Body.String())
	}
}

func TestStaticSiteHandler_ContentType(t *testing.T) {
	env := setupTestEnv(t)
	user, _ := env.createTestUser(t, "u@t.com", "password123", "user")

	site := models.Site{UserID: user.ID, SubdomainSlug: "mysite", Name: "Site"}
	env.db.Create(&site)

	deployID := models.GenerateID()
	version := 1
	env.db.Create(&models.Deployment{ID: deployID, SiteID: site.ID, Version: version, FileHash: "hash"})
	env.db.Model(&site).Updates(map[string]interface{}{
		"active_version":   version,
		"active_deploy_id": deployID,
	})

	deployPath := env.store.GetDeploymentPath(site.ID, deployID)
	_ = os.MkdirAll(deployPath, 0755)
	_ = os.WriteFile(filepath.Join(deployPath, "index.html"), []byte("<html>Test</html>"), 0644)
	_ = os.WriteFile(filepath.Join(deployPath, "style.css"), []byte("body {}"), 0644)
	_ = os.WriteFile(filepath.Join(deployPath, "script.js"), []byte("console.log()"), 0644)

	tests := []struct {
		path        string
		contentType string
	}{
		{"/index.html", "text/html"},
		{"/style.css", "text/css"},
		{"/script.js", "text/javascript"},
	}

	for _, tt := range tests {
		req := httptest.NewRequest(http.MethodGet, tt.path, nil)
		req.Host = "mysite." + env.domain
		rec := env.doRequest(req)

		if rec.Code != http.StatusOK {
			t.Errorf("path %s: status = %d, want 200", tt.path, rec.Code)
		}

		ct := rec.Header().Get("Content-Type")
		if ct != tt.contentType && ct != tt.contentType+"; charset=utf-8" {
			t.Errorf("path %s: Content-Type = %q, want %q", tt.path, ct, tt.contentType)
		}
	}
}

func TestSubdomainRouter_BareDomain(t *testing.T) {
	env := setupTestEnv(t)

	// Bare domain should pass through to API routes, not static handler
	req := httptest.NewRequest(http.MethodGet, "/api/v1/version", nil)
	req.Host = env.domain
	rec := env.doRequest(req)

	if rec.Code != http.StatusOK {
		t.Errorf("bare domain API: status = %d, want 200", rec.Code)
	}
}

func TestSubdomainRouter_UnknownSubdomain(t *testing.T) {
	env := setupTestEnv(t)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Host = "nonexistent." + env.domain
	rec := env.doRequest(req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("unknown subdomain: status = %d, want 404", rec.Code)
	}
}

func TestIsAllowedRedirectTarget_RelativePaths(t *testing.T) {
	domain := "example.com"

	tests := []struct {
		target  string
		allowed bool
	}{
		{"/path", true},
		{"/path/to/page", true},
		{"//evil.com", false},
		{"", false},
		{"javascript:alert(1)", false},
		{"data:text/html,<script>alert(1)</script>", false},
		{"https://example.com/path", true},
		{"https://sub.example.com/path", true},
		{"https://evil.com/path", false},
	}

	for _, tt := range tests {
		result := isAllowedRedirectTarget(tt.target, domain)
		if result != tt.allowed {
			t.Errorf("isAllowedRedirectTarget(%q, %q) = %v, want %v",
				tt.target, domain, result, tt.allowed)
		}
	}
}

func TestStaticSiteHandler_Redirects(t *testing.T) {
	env := setupTestEnv(t)
	user, _ := env.createTestUser(t, "u@t.com", "password123", "user")

	site := models.Site{UserID: user.ID, SubdomainSlug: "mysite", Name: "Site"}
	env.db.Create(&site)

	deployID := models.GenerateID()
	version := 1
	env.db.Create(&models.Deployment{ID: deployID, SiteID: site.ID, Version: version, FileHash: "hash"})
	env.db.Model(&site).Updates(map[string]interface{}{
		"active_version":   version,
		"active_deploy_id": deployID,
	})

	deployPath := env.store.GetDeploymentPath(site.ID, deployID)
	_ = os.MkdirAll(deployPath, 0755)
	_ = os.WriteFile(filepath.Join(deployPath, "index.html"), []byte("<html>Home</html>"), 0644)
	_ = os.WriteFile(filepath.Join(deployPath, "about.html"), []byte("<html>About</html>"), 0644)

	// Create _redirects file
	redirects := "/old-path /index.html 301\n/legacy /about.html 302\n"
	_ = os.WriteFile(filepath.Join(deployPath, "_redirects"), []byte(redirects), 0644)

	// Clear cache to ensure rules are reloaded
	env.cache = storage.NewSiteRulesCache()

	req := httptest.NewRequest(http.MethodGet, "/old-path", nil)
	req.Host = "mysite." + env.domain
	rec := env.doRequest(req)

	if rec.Code != http.StatusMovedPermanently && rec.Code != http.StatusFound {
		t.Errorf("status = %d, want 301 or 302", rec.Code)
	}
}

func TestStaticSiteHandler_Headers(t *testing.T) {
	env := setupTestEnv(t)
	user, _ := env.createTestUser(t, "u@t.com", "password123", "user")

	site := models.Site{UserID: user.ID, SubdomainSlug: "mysite", Name: "Site"}
	env.db.Create(&site)

	deployID := models.GenerateID()
	version := 1
	env.db.Create(&models.Deployment{ID: deployID, SiteID: site.ID, Version: version, FileHash: "hash"})
	env.db.Model(&site).Updates(map[string]interface{}{
		"active_version":   version,
		"active_deploy_id": deployID,
	})

	deployPath := env.store.GetDeploymentPath(site.ID, deployID)
	_ = os.MkdirAll(deployPath, 0755)
	_ = os.WriteFile(filepath.Join(deployPath, "index.html"), []byte("<html>Test</html>"), 0644)

	// Create _headers file
	headers := "/*\n  X-Custom-Header: test-value\n"
	_ = os.WriteFile(filepath.Join(deployPath, "_headers"), []byte(headers), 0644)

	req := httptest.NewRequest(http.MethodGet, "/index.html", nil)
	req.Host = "mysite." + env.domain
	rec := env.doRequest(req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	customHeader := rec.Header().Get("X-Custom-Header")
	if customHeader != "test-value" {
		t.Errorf("X-Custom-Header = %q, want test-value", customHeader)
	}
}

func TestStaticSiteHandler_DeniedHeaders(t *testing.T) {
	env := setupTestEnv(t)
	user, _ := env.createTestUser(t, "u@t.com", "password123", "user")

	site := models.Site{UserID: user.ID, SubdomainSlug: "mysite", Name: "Site"}
	env.db.Create(&site)

	deployID := models.GenerateID()
	version := 1
	env.db.Create(&models.Deployment{ID: deployID, SiteID: site.ID, Version: version, FileHash: "hash"})
	env.db.Model(&site).Updates(map[string]interface{}{
		"active_version":   version,
		"active_deploy_id": deployID,
	})

	deployPath := env.store.GetDeploymentPath(site.ID, deployID)
	_ = os.MkdirAll(deployPath, 0755)
	_ = os.WriteFile(filepath.Join(deployPath, "index.html"), []byte("<html>Test</html>"), 0644)

	// Try to set denied headers
	headers := "/*\n  Set-Cookie: evil=true\n  Host: evil.com\n"
	_ = os.WriteFile(filepath.Join(deployPath, "_headers"), []byte(headers), 0644)

	req := httptest.NewRequest(http.MethodGet, "/index.html", nil)
	req.Host = "mysite." + env.domain
	rec := env.doRequest(req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	// Denied headers should not be set
	if rec.Header().Get("Set-Cookie") != "" {
		t.Errorf("Set-Cookie should not be settable from _headers")
	}
	if rec.Header().Get("Host") != "" {
		t.Errorf("Host should not be settable from _headers")
	}
}

func TestStaticSiteHandler_LocalhostDevelopment(t *testing.T) {
	env := setupTestEnv(t)
	user, _ := env.createTestUser(t, "u@t.com", "password123", "user")

	site := models.Site{UserID: user.ID, SubdomainSlug: "mysite", Name: "Site"}
	env.db.Create(&site)

	deployID := models.GenerateID()
	version := 1
	env.db.Create(&models.Deployment{ID: deployID, SiteID: site.ID, Version: version, FileHash: "hash"})
	env.db.Model(&site).Updates(map[string]interface{}{
		"active_version":   version,
		"active_deploy_id": deployID,
	})

	deployPath := env.store.GetDeploymentPath(site.ID, deployID)
	_ = os.MkdirAll(deployPath, 0755)
	_ = os.WriteFile(filepath.Join(deployPath, "index.html"), []byte("<html>Local</html>"), 0644)

	// Test *.localhost pattern for development
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Host = "mysite.localhost"
	rec := env.doRequest(req)

	if rec.Code != http.StatusOK {
		t.Errorf("localhost development: status = %d, want 200", rec.Code)
	}
}
