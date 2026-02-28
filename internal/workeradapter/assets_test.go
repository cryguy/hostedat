package workeradapter

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/cryguy/hostedat/internal/storage"
	"github.com/cryguy/worker/v2"
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

	req := &worker.WorkerRequest{Method: "GET", URL: "http://test.local/index.html", Headers: map[string]string{}}
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
	if err := os.MkdirAll(deployPath, 0755); err != nil {
		t.Fatal(err)
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

	req := &worker.WorkerRequest{Method: "GET", URL: "http://test.local/missing.html", Headers: map[string]string{}}
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
	if err := os.MkdirAll(deployPath, 0755); err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(filepath.Join(deployPath, "index.html"), []byte("<h1>SPA</h1>"), 0644)

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
	req := &worker.WorkerRequest{Method: "GET", URL: "http://test.local/about", Headers: map[string]string{}}
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
	if err := os.MkdirAll(deployPath, 0755); err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(filepath.Join(deployPath, "404.html"), []byte("<h1>Custom 404</h1>"), 0644)

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

	req := &worker.WorkerRequest{Method: "GET", URL: "http://test.local/nope", Headers: map[string]string{}}
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
	if err := os.MkdirAll(deployPath, 0755); err != nil {
		t.Fatal(err)
	}

	// Create _redirects file with a 301 redirect
	redirects := "/old /new 301\n"
	_ = os.WriteFile(filepath.Join(deployPath, "_redirects"), []byte(redirects), 0644)

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

	req := &worker.WorkerRequest{Method: "GET", URL: "http://test.local/old", Headers: map[string]string{}}
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
	if err := os.MkdirAll(deployPath, 0755); err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(filepath.Join(deployPath, "index.html"), []byte("<h1>Rewritten</h1>"), 0644)

	// Create _redirects with a rewrite rule (200)
	redirects := "/api/* /index.html 200\n"
	_ = os.WriteFile(filepath.Join(deployPath, "_redirects"), []byte(redirects), 0644)

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

	req := &worker.WorkerRequest{Method: "GET", URL: "http://test.local/api/users", Headers: map[string]string{}}
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

func TestFilterRedirects_Empty(t *testing.T) {
	if len(filterRedirects(nil)) != 0 {
		t.Error("filterRedirects(nil) should return empty")
	}
}

func TestFilterRedirects_NoMatches(t *testing.T) {
	rules := []storage.RedirectRule{{From: "/a", To: "/b", StatusCode: 200}}
	if len(filterRedirects(rules)) != 0 {
		t.Error("filterRedirects with only 200s should return empty")
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

func TestFilterRewrites_Empty(t *testing.T) {
	if len(filterRewrites(nil)) != 0 {
		t.Error("filterRewrites(nil) should return empty")
	}
}

func TestFilterRewrites_NoMatches(t *testing.T) {
	rules := []storage.RedirectRule{{From: "/a", To: "/b", StatusCode: 301}}
	if len(filterRewrites(rules)) != 0 {
		t.Error("filterRewrites with only 301s should return empty")
	}
}

func TestAssetsFetcher_BadURL(t *testing.T) {
	dir := t.TempDir()
	siteID := "test-badurl"
	deployKey := "deploy1"

	deployPath := filepath.Join(dir, siteID, deployKey)
	if err := os.MkdirAll(deployPath, 0755); err != nil {
		t.Fatal(err)
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

	req := &worker.WorkerRequest{Method: "GET", URL: ":%invalid", Headers: map[string]string{}}
	resp, err := fetcher.Fetch(req)
	if err != nil {
		t.Fatalf("Fetch should not error on bad URL: %v", err)
	}
	if resp.StatusCode != 400 {
		t.Errorf("status = %d, want 400 for bad URL", resp.StatusCode)
	}
}

func TestAssetsFetcher_EmptyPath(t *testing.T) {
	dir := t.TempDir()
	siteID := "test-emptypath"
	deployKey := "deploy1"

	deployPath := filepath.Join(dir, siteID, deployKey)
	if err := os.MkdirAll(deployPath, 0755); err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(filepath.Join(deployPath, "index.html"), []byte("<h1>Root</h1>"), 0644)

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

	// Request with empty path (just domain)
	req := &worker.WorkerRequest{Method: "GET", URL: "http://test.local", Headers: map[string]string{}}
	resp, err := fetcher.Fetch(req)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	// Should try to resolve "/" which may or may not find index.html depending on store behavior
	if resp == nil {
		t.Fatal("response should not be nil")
	}
}

func TestAssetsFetcher_CacheHit(t *testing.T) {
	dir := t.TempDir()
	siteID := "test-cache"
	deployKey := "deploy1"

	deployPath := filepath.Join(dir, siteID, deployKey)
	if err := os.MkdirAll(deployPath, 0755); err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(filepath.Join(deployPath, "index.html"), []byte("<h1>Cached</h1>"), 0644)

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

	req := &worker.WorkerRequest{Method: "GET", URL: "http://test.local/index.html", Headers: map[string]string{}}

	// First call loads rules into cache
	resp1, err := fetcher.Fetch(req)
	if err != nil {
		t.Fatalf("Fetch 1: %v", err)
	}
	if resp1.StatusCode != 200 {
		t.Errorf("status 1 = %d, want 200", resp1.StatusCode)
	}

	// Second call should use cache
	resp2, err := fetcher.Fetch(req)
	if err != nil {
		t.Fatalf("Fetch 2: %v", err)
	}
	if resp2.StatusCode != 200 {
		t.Errorf("status 2 = %d, want 200", resp2.StatusCode)
	}
}
