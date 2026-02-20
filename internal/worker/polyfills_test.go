package worker

import (
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
