package client

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// --- Client basics ---

func TestNew(t *testing.T) {
	c := New("https://example.com", "test-token")
	if c.BaseURL != "https://example.com" {
		t.Errorf("expected BaseURL https://example.com, got %s", c.BaseURL)
	}
	if c.Token != "test-token" {
		t.Errorf("expected Token test-token, got %s", c.Token)
	}
	if c.HTTPClient == nil {
		t.Error("expected HTTPClient to be initialized")
	}
	if c.HTTPClient.Timeout.Seconds() != 60 {
		t.Errorf("expected HTTPClient timeout 60s, got %v", c.HTTPClient.Timeout)
	}
}

func TestClient_VersionHeader(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		version := r.Header.Get("X-Hostedat-Version")
		if version != "1.0.0" {
			t.Errorf("expected X-Hostedat-Version: 1.0.0, got %s", version)
		}
		w.WriteHeader(200)
		json.NewEncoder(w).Encode([]Site{})
	}))
	defer srv.Close()

	c := New(srv.URL, "test-token")
	c.Version = "1.0.0"
	_, err := c.ListSites()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestClient_AuthHeader(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth != "Bearer test-token" {
			t.Errorf("expected Authorization: Bearer test-token, got %s", auth)
		}
		w.WriteHeader(200)
		json.NewEncoder(w).Encode([]Site{})
	}))
	defer srv.Close()

	c := New(srv.URL, "test-token")
	_, err := c.ListSites()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestClient_ErrorResponse_JSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(400)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid request"})
	}))
	defer srv.Close()

	c := New(srv.URL, "test-token")
	_, err := c.ListSites()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	apiErr, ok := err.(*APIError)
	if !ok {
		t.Fatalf("expected *APIError, got %T", err)
	}
	if apiErr.StatusCode != 400 {
		t.Errorf("expected StatusCode 400, got %d", apiErr.StatusCode)
	}
	if apiErr.Message != "invalid request" {
		t.Errorf("expected Message 'invalid request', got %s", apiErr.Message)
	}
}

func TestClient_ErrorResponse_NonJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		w.Write([]byte("internal server error"))
	}))
	defer srv.Close()

	c := New(srv.URL, "test-token")
	_, err := c.ListSites()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	apiErr, ok := err.(*APIError)
	if !ok {
		t.Fatalf("expected *APIError, got %T", err)
	}
	if apiErr.StatusCode != 500 {
		t.Errorf("expected StatusCode 500, got %d", apiErr.StatusCode)
	}
	if apiErr.Message != "Internal Server Error" {
		t.Errorf("expected Message 'Internal Server Error', got %s", apiErr.Message)
	}
}

func TestAPIError_Error(t *testing.T) {
	err := &APIError{StatusCode: 404, Message: "not found"}
	expected := "API error 404: not found"
	if err.Error() != expected {
		t.Errorf("expected %q, got %q", expected, err.Error())
	}
}

func TestClient_doJSON_NilReqBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Errorf("expected GET, got %s", r.Method)
		}
		w.WriteHeader(200)
		json.NewEncoder(w).Encode([]Site{{ID: "abc", Name: "Test"}})
	}))
	defer srv.Close()

	c := New(srv.URL, "test-token")
	var sites []Site
	err := c.doJSON("GET", "/api/v1/sites", nil, &sites)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sites) != 1 {
		t.Errorf("expected 1 site, got %d", len(sites))
	}
}

func TestClient_doJSON_NilRespBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "DELETE" {
			t.Errorf("expected DELETE, got %s", r.Method)
		}
		w.WriteHeader(204)
	}))
	defer srv.Close()

	c := New(srv.URL, "test-token")
	err := c.doJSON("DELETE", "/api/v1/sites/abc", nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// --- Sites ---

func TestClient_ListSites(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if r.URL.Path != "/api/v1/sites" {
			t.Errorf("expected /api/v1/sites, got %s", r.URL.Path)
		}
		w.WriteHeader(200)
		json.NewEncoder(w).Encode([]Site{
			{ID: "abc", Name: "Test 1", SubdomainSlug: "test1"},
			{ID: "def", Name: "Test 2", SubdomainSlug: "test2"},
		})
	}))
	defer srv.Close()

	c := New(srv.URL, "test-token")
	sites, err := c.ListSites()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sites) != 2 {
		t.Fatalf("expected 2 sites, got %d", len(sites))
	}
	if sites[0].ID != "abc" || sites[0].Name != "Test 1" {
		t.Errorf("unexpected first site: %+v", sites[0])
	}
}

func TestClient_CreateSite_WithSubdomain(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/api/v1/sites" {
			t.Errorf("expected /api/v1/sites, got %s", r.URL.Path)
		}
		var req map[string]string
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("failed to decode request: %v", err)
		}
		if req["name"] != "My Site" {
			t.Errorf("expected name 'My Site', got %s", req["name"])
		}
		if req["subdomain_slug"] != "my-site" {
			t.Errorf("expected subdomain_slug 'my-site', got %s", req["subdomain_slug"])
		}
		w.WriteHeader(201)
		json.NewEncoder(w).Encode(Site{ID: "abc", Name: "My Site", SubdomainSlug: "my-site"})
	}))
	defer srv.Close()

	c := New(srv.URL, "test-token")
	site, err := c.CreateSite("My Site", "my-site")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if site.ID != "abc" || site.Name != "My Site" || site.SubdomainSlug != "my-site" {
		t.Errorf("unexpected site: %+v", site)
	}
}

func TestClient_CreateSite_WithoutSubdomain(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req map[string]string
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("failed to decode request: %v", err)
		}
		if req["name"] != "My Site" {
			t.Errorf("expected name 'My Site', got %s", req["name"])
		}
		if _, exists := req["subdomain_slug"]; exists {
			t.Error("expected subdomain_slug to be absent")
		}
		w.WriteHeader(201)
		json.NewEncoder(w).Encode(Site{ID: "abc", Name: "My Site", SubdomainSlug: "generated"})
	}))
	defer srv.Close()

	c := New(srv.URL, "test-token")
	site, err := c.CreateSite("My Site", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if site.ID != "abc" || site.Name != "My Site" || site.SubdomainSlug != "generated" {
		t.Errorf("unexpected site: %+v", site)
	}
}

func TestClient_DeleteSite(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "DELETE" {
			t.Errorf("expected DELETE, got %s", r.Method)
		}
		if r.URL.Path != "/api/v1/sites/abc" {
			t.Errorf("expected /api/v1/sites/abc, got %s", r.URL.Path)
		}
		w.WriteHeader(204)
	}))
	defer srv.Close()

	c := New(srv.URL, "test-token")
	err := c.DeleteSite("abc")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestClient_ResolveSite_ByID(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]Site{
			{ID: "abc", Name: "Test 1", SubdomainSlug: "test1"},
			{ID: "def", Name: "Test 2", SubdomainSlug: "test2"},
		})
	}))
	defer srv.Close()

	c := New(srv.URL, "test-token")
	site, err := c.ResolveSite("abc")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if site.ID != "abc" || site.Name != "Test 1" {
		t.Errorf("unexpected site: %+v", site)
	}
}

func TestClient_ResolveSite_BySlug(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]Site{
			{ID: "abc", Name: "Test 1", SubdomainSlug: "test1"},
			{ID: "def", Name: "Test 2", SubdomainSlug: "test2"},
		})
	}))
	defer srv.Close()

	c := New(srv.URL, "test-token")
	site, err := c.ResolveSite("test2")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if site.ID != "def" || site.SubdomainSlug != "test2" {
		t.Errorf("unexpected site: %+v", site)
	}
}

func TestClient_ResolveSite_ByName(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]Site{
			{ID: "abc", Name: "Test 1", SubdomainSlug: "test1"},
			{ID: "def", Name: "Test 2", SubdomainSlug: "test2"},
		})
	}))
	defer srv.Close()

	c := New(srv.URL, "test-token")
	site, err := c.ResolveSite("test 2")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if site.ID != "def" || site.Name != "Test 2" {
		t.Errorf("unexpected site: %+v", site)
	}
}

func TestClient_ResolveSite_NoMatch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]Site{
			{ID: "abc", Name: "Test 1", SubdomainSlug: "test1"},
		})
	}))
	defer srv.Close()

	c := New(srv.URL, "test-token")
	_, err := c.ResolveSite("nonexistent")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "no site found matching") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestClient_ResolveSite_Ambiguous(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]Site{
			{ID: "abc", Name: "Test", SubdomainSlug: "test1"},
			{ID: "def", Name: "Test", SubdomainSlug: "test2"},
		})
	}))
	defer srv.Close()

	c := New(srv.URL, "test-token")
	_, err := c.ResolveSite("test")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	errMsg := err.Error()
	if !strings.Contains(errMsg, "ambiguous site reference") {
		t.Errorf("expected ambiguous error, got: %v", err)
	}
	if !strings.Contains(errMsg, "abc") || !strings.Contains(errMsg, "def") {
		t.Errorf("expected both IDs in error, got: %v", err)
	}
}

func TestClient_ResolveSiteID(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]Site{
			{ID: "abc", Name: "Test", SubdomainSlug: "test"},
		})
	}))
	defer srv.Close()

	c := New(srv.URL, "test-token")
	id, err := c.ResolveSiteID("test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != "abc" {
		t.Errorf("expected id 'abc', got %s", id)
	}
}

func TestClient_UpdateSite_NameOnly(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "PATCH" {
			t.Errorf("expected PATCH, got %s", r.Method)
		}
		if r.URL.Path != "/api/v1/sites/abc" {
			t.Errorf("expected /api/v1/sites/abc, got %s", r.URL.Path)
		}
		var req map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("failed to decode request: %v", err)
		}
		if req["name"] != "Updated" {
			t.Errorf("expected name 'Updated', got %v", req["name"])
		}
		if _, exists := req["spa_mode"]; exists {
			t.Error("expected spa_mode to be absent")
		}
		w.WriteHeader(200)
		json.NewEncoder(w).Encode(Site{ID: "abc", Name: "Updated"})
	}))
	defer srv.Close()

	c := New(srv.URL, "test-token")
	name := "Updated"
	site, err := c.UpdateSite("abc", &name, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if site.Name != "Updated" {
		t.Errorf("expected Name 'Updated', got %s", site.Name)
	}
}

func TestClient_UpdateSite_SPAModeOnly(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("failed to decode request: %v", err)
		}
		if _, exists := req["name"]; exists {
			t.Error("expected name to be absent")
		}
		if req["spa_mode"] != true {
			t.Errorf("expected spa_mode true, got %v", req["spa_mode"])
		}
		w.WriteHeader(200)
		json.NewEncoder(w).Encode(Site{ID: "abc", SPAMode: true})
	}))
	defer srv.Close()

	c := New(srv.URL, "test-token")
	spaMode := true
	site, err := c.UpdateSite("abc", nil, &spaMode)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !site.SPAMode {
		t.Error("expected SPAMode true")
	}
}

func TestClient_UpdateSite_Both(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("failed to decode request: %v", err)
		}
		if req["name"] != "Updated" {
			t.Errorf("expected name 'Updated', got %v", req["name"])
		}
		if req["spa_mode"] != false {
			t.Errorf("expected spa_mode false, got %v", req["spa_mode"])
		}
		w.WriteHeader(200)
		json.NewEncoder(w).Encode(Site{ID: "abc", Name: "Updated", SPAMode: false})
	}))
	defer srv.Close()

	c := New(srv.URL, "test-token")
	name := "Updated"
	spaMode := false
	site, err := c.UpdateSite("abc", &name, &spaMode)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if site.Name != "Updated" {
		t.Errorf("expected Name 'Updated', got %s", site.Name)
	}
	if site.SPAMode {
		t.Error("expected SPAMode false")
	}
}

// --- Deploy ---

func TestClient_Deploy(t *testing.T) {
	// Create temp directory with files
	tmpDir, err := os.MkdirTemp("", "deploy-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	if err := os.WriteFile(filepath.Join(tmpDir, "index.html"), []byte("<h1>Test</h1>"), 0644); err != nil {
		t.Fatalf("failed to write index.html: %v", err)
	}
	if err := os.Mkdir(filepath.Join(tmpDir, "assets"), 0755); err != nil {
		t.Fatalf("failed to create assets dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "assets", "style.css"), []byte("body{}"), 0644); err != nil {
		t.Fatalf("failed to write style.css: %v", err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/api/v1/sites/abc/deploy" {
			t.Errorf("expected /api/v1/sites/abc/deploy, got %s", r.URL.Path)
		}
		if !strings.Contains(r.Header.Get("Content-Type"), "multipart/form-data") {
			t.Errorf("expected multipart/form-data, got %s", r.Header.Get("Content-Type"))
		}

		// Parse multipart form
		if err := r.ParseMultipartForm(10 << 20); err != nil {
			t.Fatalf("failed to parse multipart form: %v", err)
		}
		file, _, err := r.FormFile("file")
		if err != nil {
			t.Fatalf("failed to get form file: %v", err)
		}
		defer file.Close()

		// Verify it's a valid zip with expected files
		zipBytes, _ := io.ReadAll(file)
		zipReader, err := zip.NewReader(bytes.NewReader(zipBytes), int64(len(zipBytes)))
		if err != nil {
			t.Fatalf("failed to read zip: %v", err)
		}

		foundIndex := false
		foundCSS := false
		for _, f := range zipReader.File {
			if f.Name == "index.html" {
				foundIndex = true
			}
			if f.Name == "assets/style.css" {
				foundCSS = true
			}
		}
		if !foundIndex {
			t.Error("expected index.html in zip")
		}
		if !foundCSS {
			t.Error("expected assets/style.css in zip")
		}

		w.WriteHeader(201)
		json.NewEncoder(w).Encode(Deployment{ID: "dep1", SiteID: "abc", Version: 1})
	}))
	defer srv.Close()

	c := New(srv.URL, "test-token")
	dep, err := c.Deploy("abc", tmpDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dep.ID != "dep1" || dep.SiteID != "abc" || dep.Version != 1 {
		t.Errorf("unexpected deployment: %+v", dep)
	}
}

func TestZipDirectory_Valid(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "zip-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	if err := os.WriteFile(filepath.Join(tmpDir, "file1.txt"), []byte("content1"), 0644); err != nil {
		t.Fatalf("failed to write file1.txt: %v", err)
	}
	if err := os.Mkdir(filepath.Join(tmpDir, "subdir"), 0755); err != nil {
		t.Fatalf("failed to create subdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "subdir", "file2.txt"), []byte("content2"), 0644); err != nil {
		t.Fatalf("failed to write file2.txt: %v", err)
	}

	zipData, err := zipDirectory(tmpDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify zip contents
	zipReader, err := zip.NewReader(bytes.NewReader(zipData), int64(len(zipData)))
	if err != nil {
		t.Fatalf("failed to read zip: %v", err)
	}

	fileMap := make(map[string]bool)
	for _, f := range zipReader.File {
		fileMap[f.Name] = true
	}

	if !fileMap["file1.txt"] {
		t.Error("expected file1.txt in zip")
	}
	if !fileMap["subdir/file2.txt"] {
		t.Error("expected subdir/file2.txt in zip")
	}
	if len(fileMap) != 2 {
		t.Errorf("expected 2 files in zip, got %d: %v", len(fileMap), fileMap)
	}
}

func TestZipDirectory_NotFound(t *testing.T) {
	_, err := zipDirectory("/nonexistent/path/that/does/not/exist")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "directory not found") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestZipDirectory_NotADirectory(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "not-a-dir")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	_, err = zipDirectory(tmpFile.Name())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "not a directory") {
		t.Errorf("unexpected error message: %v", err)
	}
}

// --- Auth ---

func TestClient_CreateAPIKey(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/api/v1/keys" {
			t.Errorf("expected /api/v1/keys, got %s", r.URL.Path)
		}
		var req map[string]string
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("failed to decode request: %v", err)
		}
		if req["name"] != "test-key" {
			t.Errorf("expected name 'test-key', got %s", req["name"])
		}
		w.WriteHeader(201)
		json.NewEncoder(w).Encode(APIKeyResponse{
			ID:   "key1",
			Name: "test-key",
			Key:  "sk_test_123",
		})
	}))
	defer srv.Close()

	c := New(srv.URL, "test-token")
	resp, err := c.CreateAPIKey("test-key")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.ID != "key1" || resp.Name != "test-key" || resp.Key != "sk_test_123" {
		t.Errorf("unexpected response: %+v", resp)
	}
}

func TestCreateAPIKey_ErrorResponse(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		body       string
	}{
		{"401 Unauthorized", 401, `{"error":"unauthorized"}`},
		{"500 Internal Server Error", 500, `{"error":"internal error"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.statusCode)
				w.Write([]byte(tt.body))
			}))
			defer srv.Close()

			c := New(srv.URL, "test-token")
			_, err := c.CreateAPIKey("test-key")
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			apiErr, ok := err.(*APIError)
			if !ok {
				t.Fatalf("expected *APIError, got %T", err)
			}
			if apiErr.StatusCode != tt.statusCode {
				t.Errorf("expected StatusCode %d, got %d", tt.statusCode, apiErr.StatusCode)
			}
		})
	}
}

func TestDeploy_EmptyDirectory(t *testing.T) {
	tmpDir := t.TempDir()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseMultipartForm(10 << 20); err != nil {
			t.Fatalf("failed to parse multipart form: %v", err)
		}
		file, _, err := r.FormFile("file")
		if err != nil {
			t.Fatalf("failed to get form file: %v", err)
		}
		defer file.Close()

		zipBytes, _ := io.ReadAll(file)
		zipReader, err := zip.NewReader(bytes.NewReader(zipBytes), int64(len(zipBytes)))
		if err != nil {
			t.Fatalf("failed to read zip: %v", err)
		}
		if len(zipReader.File) != 0 {
			t.Errorf("expected 0 files in zip, got %d", len(zipReader.File))
		}

		w.WriteHeader(201)
		json.NewEncoder(w).Encode(Deployment{ID: "dep1", SiteID: "abc"})
	}))
	defer srv.Close()

	c := New(srv.URL, "test-token")
	dep, err := c.Deploy("abc", tmpDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dep.ID != "dep1" {
		t.Errorf("unexpected deployment: %+v", dep)
	}
}

func TestDeploy_NestedDirectories(t *testing.T) {
	tmpDir := t.TempDir()

	// Create deeply nested directory structure.
	dirs := []string{
		filepath.Join(tmpDir, "a", "b", "c"),
		filepath.Join(tmpDir, "x", "y"),
	}
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0755); err != nil {
			t.Fatalf("failed to create dir %s: %v", d, err)
		}
	}
	files := map[string]string{
		filepath.Join(tmpDir, "root.txt"):             "root",
		filepath.Join(tmpDir, "a", "a.txt"):           "a",
		filepath.Join(tmpDir, "a", "b", "b.txt"):      "b",
		filepath.Join(tmpDir, "a", "b", "c", "c.txt"): "c",
		filepath.Join(tmpDir, "x", "y", "y.txt"):      "y",
	}
	for path, content := range files {
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatalf("failed to write %s: %v", path, err)
		}
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseMultipartForm(10 << 20); err != nil {
			t.Fatalf("failed to parse multipart form: %v", err)
		}
		file, _, err := r.FormFile("file")
		if err != nil {
			t.Fatalf("failed to get form file: %v", err)
		}
		defer file.Close()

		zipBytes, _ := io.ReadAll(file)
		zipReader, err := zip.NewReader(bytes.NewReader(zipBytes), int64(len(zipBytes)))
		if err != nil {
			t.Fatalf("failed to read zip: %v", err)
		}

		expectedPaths := map[string]bool{
			"root.txt":    true,
			"a/a.txt":     true,
			"a/b/b.txt":   true,
			"a/b/c/c.txt": true,
			"x/y/y.txt":   true,
		}

		for _, f := range zipReader.File {
			if !expectedPaths[f.Name] {
				t.Errorf("unexpected file in zip: %s", f.Name)
			}
			delete(expectedPaths, f.Name)
		}
		for missing := range expectedPaths {
			t.Errorf("missing file in zip: %s", missing)
		}

		w.WriteHeader(201)
		json.NewEncoder(w).Encode(Deployment{ID: "dep1", SiteID: "abc", Version: 1})
	}))
	defer srv.Close()

	c := New(srv.URL, "test-token")
	dep, err := c.Deploy("abc", tmpDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dep.ID != "dep1" || dep.SiteID != "abc" || dep.Version != 1 {
		t.Errorf("unexpected deployment: %+v", dep)
	}
}
