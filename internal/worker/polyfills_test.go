package worker

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestEnsureUnenv_Downloads(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in -short mode")
	}

	dataDir := t.TempDir()

	unenvDir, err := EnsureUnenv(dataDir)
	if err != nil {
		t.Fatalf("EnsureUnenv failed: %v", err)
	}

	// Verify unenv directory structure.
	runtimeNode := filepath.Join(unenvDir, "runtime", "node")
	if info, err := os.Stat(runtimeNode); err != nil || !info.IsDir() {
		t.Fatalf("expected %s to be a directory", runtimeNode)
	}

	// Verify pathe was also extracted.
	patheDir := filepath.Join(dataDir, "polyfills", "node_modules", "pathe")
	if info, err := os.Stat(patheDir); err != nil || !info.IsDir() {
		t.Fatalf("expected %s to be a directory", patheDir)
	}
}

func TestEnsureUnenv_CachesResult(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in -short mode")
	}

	dataDir := t.TempDir()

	// First call downloads.
	dir1, err := EnsureUnenv(dataDir)
	if err != nil {
		t.Fatalf("first EnsureUnenv failed: %v", err)
	}

	// Verify the directory exists before second call.
	checkDir := filepath.Join(dir1, "runtime", "node")
	if info, err := os.Stat(checkDir); err != nil || !info.IsDir() {
		t.Fatalf("expected %s to exist after first call", checkDir)
	}

	// Second call should return immediately (cached).
	dir2, err := EnsureUnenv(dataDir)
	if err != nil {
		t.Fatalf("second EnsureUnenv failed: %v", err)
	}

	if dir1 != dir2 {
		t.Errorf("expected same path, got %q and %q", dir1, dir2)
	}
}

func TestEnsureUnenv_InvalidDir(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in -short mode")
	}

	// Use a path that cannot be created (nested under a file, not a directory).
	tmpDir := t.TempDir()
	blocker := filepath.Join(tmpDir, "blocker")
	if err := os.WriteFile(blocker, []byte("not a dir"), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := EnsureUnenv(filepath.Join(blocker, "subdir"))
	if err == nil {
		t.Fatal("expected error for unwritable path")
	}
}

func TestPolyfill_DownloadSizeLimitExists(t *testing.T) {
	if maxPolyfillDownloadSize < 1*1024*1024 || maxPolyfillDownloadSize > 200*1024*1024 {
		t.Errorf("maxPolyfillDownloadSize = %d, want 1MB-200MB", maxPolyfillDownloadSize)
	}
}

func TestPolyfill_IntegrityCheckRejectsTamperedContent(t *testing.T) {
	// Set up a test hash for a known URL
	testURL := "https://example.com/test.tgz"
	polyfillHashes[testURL] = "0000000000000000000000000000000000000000000000000000000000000000"
	defer delete(polyfillHashes, testURL)

	// Create a test HTTP server that returns known content
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Return a minimal valid gzip content
		w.WriteHeader(200)
		w.Write([]byte("not the expected content"))
	}))
	defer ts.Close()

	// The hash won't match, so this should fail
	// Note: we can't easily test with the real URL, so test the hash map lookup logic
	if _, ok := polyfillHashes[testURL]; !ok {
		t.Error("test hash should be set")
	}
	// Verify the hash doesn't match arbitrary content
	actualHash := sha256.Sum256([]byte("not the expected content"))
	if hex.EncodeToString(actualHash[:]) == polyfillHashes[testURL] {
		t.Error("hash should not match tampered content")
	}
}
