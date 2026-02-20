package client

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestClient_Deploy_Success(t *testing.T) {
	tmpDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmpDir, "index.html"), []byte("<h1>Hello</h1>"), 0644); err != nil {
		t.Fatalf("failed to write index.html: %v", err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/api/v1/sites/site123/deploy" {
			t.Errorf("expected /api/v1/sites/site123/deploy, got %s", r.URL.Path)
		}
		if !strings.Contains(r.Header.Get("Content-Type"), "multipart/form-data") {
			t.Errorf("expected multipart/form-data, got %s", r.Header.Get("Content-Type"))
		}

		w.WriteHeader(201)
		_ = json.NewEncoder(w).Encode(Deployment{
			ID:         "dep123",
			SiteID:     "site123",
			Version:    5,
			FileHash:   "abc123",
			UploadedAt: time.Now(),
		})
	}))
	defer srv.Close()

	c := New(srv.URL, "test-token")
	dep, err := c.Deploy("site123", tmpDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dep.ID != "dep123" {
		t.Errorf("expected ID 'dep123', got %s", dep.ID)
	}
	if dep.SiteID != "site123" {
		t.Errorf("expected SiteID 'site123', got %s", dep.SiteID)
	}
	if dep.Version != 5 {
		t.Errorf("expected Version 5, got %d", dep.Version)
	}
}

func TestClient_Deploy_NonexistentDirectory(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("server should not be called")
	}))
	defer srv.Close()

	c := New(srv.URL, "test-token")
	_, err := c.Deploy("site123", "/nonexistent/directory")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "directory not found") {
		t.Errorf("expected 'directory not found' error, got %v", err)
	}
}

func TestClient_Deploy_NotADirectory(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "not-a-dir-*")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("server should not be called")
	}))
	defer srv.Close()

	c := New(srv.URL, "test-token")
	_, err = c.Deploy("site123", tmpFile.Name())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "not a directory") {
		t.Errorf("expected 'not a directory' error, got %v", err)
	}
}

func TestClient_Deploy_Unauthorized(t *testing.T) {
	tmpDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmpDir, "index.html"), []byte("<h1>Hello</h1>"), 0644); err != nil {
		t.Fatalf("failed to write index.html: %v", err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(401)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "unauthorized"})
	}))
	defer srv.Close()

	c := New(srv.URL, "invalid-token")
	_, err := c.Deploy("site123", tmpDir)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected *APIError, got %T", err)
	}
	if apiErr.StatusCode != 401 {
		t.Errorf("expected StatusCode 401, got %d", apiErr.StatusCode)
	}
}

func TestClient_Deploy_SiteNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmpDir, "index.html"), []byte("<h1>Hello</h1>"), 0644); err != nil {
		t.Fatalf("failed to write index.html: %v", err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "site not found"})
	}))
	defer srv.Close()

	c := New(srv.URL, "test-token")
	_, err := c.Deploy("nonexistent", tmpDir)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected *APIError, got %T", err)
	}
	if apiErr.StatusCode != 404 {
		t.Errorf("expected StatusCode 404, got %d", apiErr.StatusCode)
	}
}

func TestClient_Deploy_InvalidJSON(t *testing.T) {
	tmpDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmpDir, "index.html"), []byte("<h1>Hello</h1>"), 0644); err != nil {
		t.Fatalf("failed to write index.html: %v", err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(201)
		_, _ = w.Write([]byte("not json"))
	}))
	defer srv.Close()

	c := New(srv.URL, "test-token")
	_, err := c.Deploy("site123", tmpDir)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "invalid character") {
		t.Errorf("expected JSON parse error, got %v", err)
	}
}

func TestZipDirectory_ExcludesGit(t *testing.T) {
	tmpDir := t.TempDir()
	gitDir := filepath.Join(tmpDir, ".git")
	if err := os.Mkdir(gitDir, 0755); err != nil {
		t.Fatalf("failed to create .git dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(gitDir, "config"), []byte("git config"), 0644); err != nil {
		t.Fatalf("failed to write .git/config: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "index.html"), []byte("<h1>Test</h1>"), 0644); err != nil {
		t.Fatalf("failed to write index.html: %v", err)
	}

	zipData, err := zipDirectory(tmpDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	zipReader, err := zip.NewReader(bytes.NewReader(zipData), int64(len(zipData)))
	if err != nil {
		t.Fatalf("failed to read zip: %v", err)
	}

	for _, f := range zipReader.File {
		if strings.Contains(f.Name, ".git") {
			t.Errorf("expected .git to be excluded, but found %s", f.Name)
		}
	}

	foundIndex := false
	for _, f := range zipReader.File {
		if f.Name == "index.html" {
			foundIndex = true
		}
	}
	if !foundIndex {
		t.Error("expected index.html in zip")
	}
}

func TestZipDirectory_ExcludesEnvFiles(t *testing.T) {
	tmpDir := t.TempDir()
	envFiles := []string{".env", ".env.local", ".env.production", ".env.test"}
	for _, name := range envFiles {
		if err := os.WriteFile(filepath.Join(tmpDir, name), []byte("SECRET=123"), 0644); err != nil {
			t.Fatalf("failed to write %s: %v", name, err)
		}
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "index.html"), []byte("<h1>Test</h1>"), 0644); err != nil {
		t.Fatalf("failed to write index.html: %v", err)
	}

	zipData, err := zipDirectory(tmpDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	zipReader, err := zip.NewReader(bytes.NewReader(zipData), int64(len(zipData)))
	if err != nil {
		t.Fatalf("failed to read zip: %v", err)
	}

	for _, f := range zipReader.File {
		if strings.Contains(f.Name, ".env") {
			t.Errorf("expected .env files to be excluded, but found %s", f.Name)
		}
	}

	foundIndex := false
	for _, f := range zipReader.File {
		if f.Name == "index.html" {
			foundIndex = true
		}
	}
	if !foundIndex {
		t.Error("expected index.html in zip")
	}
}

func TestZipDirectory_ExcludesNodeModules(t *testing.T) {
	tmpDir := t.TempDir()
	nodeModules := filepath.Join(tmpDir, "node_modules")
	if err := os.Mkdir(nodeModules, 0755); err != nil {
		t.Fatalf("failed to create node_modules: %v", err)
	}
	if err := os.WriteFile(filepath.Join(nodeModules, "package.json"), []byte("{}"), 0644); err != nil {
		t.Fatalf("failed to write node_modules/package.json: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "index.html"), []byte("<h1>Test</h1>"), 0644); err != nil {
		t.Fatalf("failed to write index.html: %v", err)
	}

	zipData, err := zipDirectory(tmpDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	zipReader, err := zip.NewReader(bytes.NewReader(zipData), int64(len(zipData)))
	if err != nil {
		t.Fatalf("failed to read zip: %v", err)
	}

	for _, f := range zipReader.File {
		if strings.Contains(f.Name, "node_modules") {
			t.Errorf("expected node_modules to be excluded, but found %s", f.Name)
		}
	}
}

func TestZipDirectory_RelativePaths(t *testing.T) {
	tmpDir := t.TempDir()
	subdir := filepath.Join(tmpDir, "assets", "css")
	if err := os.MkdirAll(subdir, 0755); err != nil {
		t.Fatalf("failed to create subdirs: %v", err)
	}
	if err := os.WriteFile(filepath.Join(subdir, "style.css"), []byte("body{}"), 0644); err != nil {
		t.Fatalf("failed to write style.css: %v", err)
	}

	zipData, err := zipDirectory(tmpDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	zipReader, err := zip.NewReader(bytes.NewReader(zipData), int64(len(zipData)))
	if err != nil {
		t.Fatalf("failed to read zip: %v", err)
	}

	found := false
	for _, f := range zipReader.File {
		if f.Name == "assets/css/style.css" {
			found = true
		}
		// Verify forward slashes are used (zip standard)
		if strings.Contains(f.Name, "\\") {
			t.Errorf("expected forward slashes in zip paths, got %s", f.Name)
		}
	}
	if !found {
		t.Error("expected assets/css/style.css in zip with forward slashes")
	}
}

func TestIsExcluded(t *testing.T) {
	tests := []struct {
		name     string
		filename string
		want     bool
	}{
		{"git directory", ".git", true},
		{"env file", ".env", true},
		{"env local", ".env.local", true},
		{"env production", ".env.production", true},
		{"custom env", ".env.custom", true},
		{"DS_Store", ".DS_Store", true},
		{"node_modules", "node_modules", true},
		{"svn", ".svn", true},
		{"mercurial", ".hg", true},
		{"pycache", "__pycache__", true},
		{"terraform", ".terraform", true},
		{"regular file", "index.html", false},
		{"dotfile", ".gitignore", false},
		{"env in name", "env.js", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isExcluded(tt.filename)
			if got != tt.want {
				t.Errorf("isExcluded(%q) = %v, want %v", tt.filename, got, tt.want)
			}
		})
	}
}

func TestZipDirectory_FileContent(t *testing.T) {
	tmpDir := t.TempDir()
	content := "Hello, World!"
	if err := os.WriteFile(filepath.Join(tmpDir, "test.txt"), []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test.txt: %v", err)
	}

	zipData, err := zipDirectory(tmpDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	zipReader, err := zip.NewReader(bytes.NewReader(zipData), int64(len(zipData)))
	if err != nil {
		t.Fatalf("failed to read zip: %v", err)
	}

	for _, f := range zipReader.File {
		if f.Name == "test.txt" {
			rc, err := f.Open()
			if err != nil {
				t.Fatalf("failed to open test.txt in zip: %v", err)
			}
			defer rc.Close()
			data, err := io.ReadAll(rc)
			if err != nil {
				t.Fatalf("failed to read test.txt content: %v", err)
			}
			if string(data) != content {
				t.Errorf("expected content %q, got %q", content, string(data))
			}
			return
		}
	}
	t.Error("test.txt not found in zip")
}
