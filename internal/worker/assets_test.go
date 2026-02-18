package worker

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/cryguy/hostedat/internal/storage"
)

func TestAssetsFetcher_StaticFile(t *testing.T) {
	dir := t.TempDir()
	siteID := "test-site"
	deployKey := "deploy1"

	// Create the deployment directory structure (basePath/siteID/deployKey)
	deployPath := filepath.Join(dir, siteID, deployKey)
	if err := os.MkdirAll(deployPath, 0755); err != nil {
		t.Fatalf("creating deploy path: %v", err)
	}
	if err := os.WriteFile(filepath.Join(deployPath, "index.html"), []byte("<h1>Hi</h1>"), 0644); err != nil {
		t.Fatalf("writing index.html: %v", err)
	}

	store := storage.NewManager(dir)
	cache := storage.NewSiteRulesCache()

	fetcher := &StaticAssetsFetcher{
		Store:     store,
		Cache:     cache,
		SiteID:    siteID,
		DeployKey: deployKey,
		SPAMode:   false,
		Domain:    "test.local",
	}

	req := &WorkerRequest{Method: "GET", URL: "http://test.local/index.html", Headers: map[string]string{}}
	resp, err := fetcher.Fetch(req)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
	if string(resp.Body) != "<h1>Hi</h1>" {
		t.Errorf("body = %q, want <h1>Hi</h1>", string(resp.Body))
	}
	if resp.Headers["content-type"] != "text/html; charset=utf-8" {
		t.Errorf("content-type = %q", resp.Headers["content-type"])
	}
}

func TestAssetsFetcher_FileNotFound(t *testing.T) {
	dir := t.TempDir()
	siteID := "test-site-404"
	deployKey := "deploy1"

	deployPath := filepath.Join(dir, siteID, deployKey)
	os.MkdirAll(deployPath, 0755)

	store := storage.NewManager(dir)
	cache := storage.NewSiteRulesCache()

	fetcher := &StaticAssetsFetcher{
		Store:     store,
		Cache:     cache,
		SiteID:    siteID,
		DeployKey: deployKey,
		SPAMode:   false,
		Domain:    "test.local",
	}

	req := &WorkerRequest{Method: "GET", URL: "http://test.local/missing.html", Headers: map[string]string{}}
	resp, err := fetcher.Fetch(req)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if resp.StatusCode != 404 {
		t.Errorf("status = %d, want 404", resp.StatusCode)
	}
	if string(resp.Body) != "Not Found" {
		t.Errorf("body = %q, want 'Not Found'", string(resp.Body))
	}
}

func TestAssetsFetcher_SPAMode(t *testing.T) {
	dir := t.TempDir()
	siteID := "test-spa"
	deployKey := "deploy1"

	deployPath := filepath.Join(dir, siteID, deployKey)
	os.MkdirAll(deployPath, 0755)
	os.WriteFile(filepath.Join(deployPath, "index.html"), []byte("<h1>SPA</h1>"), 0644)

	store := storage.NewManager(dir)
	cache := storage.NewSiteRulesCache()

	fetcher := &StaticAssetsFetcher{
		Store:     store,
		Cache:     cache,
		SiteID:    siteID,
		DeployKey: deployKey,
		SPAMode:   true,
		Domain:    "test.local",
	}

	// Request a path that doesn't exist — SPA mode should serve index.html
	req := &WorkerRequest{Method: "GET", URL: "http://test.local/about", Headers: map[string]string{}}
	resp, err := fetcher.Fetch(req)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Errorf("status = %d, want 200 (SPA fallback)", resp.StatusCode)
	}
	if string(resp.Body) != "<h1>SPA</h1>" {
		t.Errorf("body = %q, want <h1>SPA</h1>", string(resp.Body))
	}
}

func TestAssetsFetcher_Custom404(t *testing.T) {
	dir := t.TempDir()
	siteID := "test-404"
	deployKey := "deploy1"

	deployPath := filepath.Join(dir, siteID, deployKey)
	os.MkdirAll(deployPath, 0755)
	os.WriteFile(filepath.Join(deployPath, "404.html"), []byte("<h1>Custom 404</h1>"), 0644)

	store := storage.NewManager(dir)
	cache := storage.NewSiteRulesCache()

	fetcher := &StaticAssetsFetcher{
		Store:     store,
		Cache:     cache,
		SiteID:    siteID,
		DeployKey: deployKey,
		SPAMode:   false,
		Domain:    "test.local",
	}

	req := &WorkerRequest{Method: "GET", URL: "http://test.local/nope", Headers: map[string]string{}}
	resp, err := fetcher.Fetch(req)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if resp.StatusCode != 404 {
		t.Errorf("status = %d, want 404", resp.StatusCode)
	}
	if string(resp.Body) != "<h1>Custom 404</h1>" {
		t.Errorf("body = %q, want custom 404", string(resp.Body))
	}
}

func TestAssetsFetcher_RedirectRule(t *testing.T) {
	dir := t.TempDir()
	siteID := "test-redirect"
	deployKey := "deploy1"

	deployPath := filepath.Join(dir, siteID, deployKey)
	os.MkdirAll(deployPath, 0755)

	// Create _redirects file with a 301 redirect
	redirects := "/old /new 301\n"
	os.WriteFile(filepath.Join(deployPath, "_redirects"), []byte(redirects), 0644)

	store := storage.NewManager(dir)
	cache := storage.NewSiteRulesCache()

	fetcher := &StaticAssetsFetcher{
		Store:     store,
		Cache:     cache,
		SiteID:    siteID,
		DeployKey: deployKey,
		SPAMode:   false,
		Domain:    "test.local",
	}

	req := &WorkerRequest{Method: "GET", URL: "http://test.local/old", Headers: map[string]string{}}
	resp, err := fetcher.Fetch(req)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if resp.StatusCode != 301 {
		t.Errorf("status = %d, want 301", resp.StatusCode)
	}
	if resp.Headers["location"] != "/new" {
		t.Errorf("location = %q, want /new", resp.Headers["location"])
	}
}

func TestAssetsFetcher_RewriteRule(t *testing.T) {
	dir := t.TempDir()
	siteID := "test-rewrite"
	deployKey := "deploy1"

	deployPath := filepath.Join(dir, siteID, deployKey)
	os.MkdirAll(deployPath, 0755)
	os.WriteFile(filepath.Join(deployPath, "index.html"), []byte("<h1>Rewritten</h1>"), 0644)

	// Create _redirects with a rewrite rule (200)
	redirects := "/api/* /index.html 200\n"
	os.WriteFile(filepath.Join(deployPath, "_redirects"), []byte(redirects), 0644)

	store := storage.NewManager(dir)
	cache := storage.NewSiteRulesCache()

	fetcher := &StaticAssetsFetcher{
		Store:     store,
		Cache:     cache,
		SiteID:    siteID,
		DeployKey: deployKey,
		SPAMode:   false,
		Domain:    "test.local",
	}

	req := &WorkerRequest{Method: "GET", URL: "http://test.local/api/users", Headers: map[string]string{}}
	resp, err := fetcher.Fetch(req)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
	if string(resp.Body) != "<h1>Rewritten</h1>" {
		t.Errorf("body = %q, want rewritten content", string(resp.Body))
	}
}

func TestContentType(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"index.html", "text/html; charset=utf-8"},
		{"style.css", "text/css; charset=utf-8"},
		{"app.js", "text/javascript; charset=utf-8"},
		{"data.json", "application/json"},
		{"image.png", "image/png"},
		{"no-extension", "application/octet-stream"},
		{"", "application/octet-stream"},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got := contentType(tt.path)
			if got != tt.want {
				t.Errorf("contentType(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

func TestFilterRedirects(t *testing.T) {
	rules := []storage.RedirectRule{
		{From: "/a", To: "/b", StatusCode: 301},
		{From: "/c", To: "/d", StatusCode: 200},
		{From: "/e", To: "/f", StatusCode: 302},
		{From: "/g", To: "/h", StatusCode: 200},
	}

	redirects := filterRedirects(rules)
	if len(redirects) != 2 {
		t.Errorf("filterRedirects returned %d rules, want 2", len(redirects))
	}
	for _, r := range redirects {
		if r.StatusCode != 301 && r.StatusCode != 302 {
			t.Errorf("filterRedirects included rule with status %d", r.StatusCode)
		}
	}
}

func TestFilterRewrites(t *testing.T) {
	rules := []storage.RedirectRule{
		{From: "/a", To: "/b", StatusCode: 301},
		{From: "/c", To: "/d", StatusCode: 200},
		{From: "/e", To: "/f", StatusCode: 302},
		{From: "/g", To: "/h", StatusCode: 200},
	}

	rewrites := filterRewrites(rules)
	if len(rewrites) != 2 {
		t.Errorf("filterRewrites returned %d rules, want 2", len(rewrites))
	}
	for _, r := range rewrites {
		if r.StatusCode != 200 {
			t.Errorf("filterRewrites included rule with status %d", r.StatusCode)
		}
	}
}
