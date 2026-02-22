package workeradapter

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGetD1Path(t *testing.T) {
	got := GetD1Path("/data", "mydb123")
	want := filepath.Join("/data", "d1", "mydb123.sqlite3")
	if got != want {
		t.Errorf("GetD1Path() = %q, want %q", got, want)
	}
}

func TestDeleteFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	if err := os.WriteFile(path, []byte("data"), 0644); err != nil {
		t.Fatalf("writing file: %v", err)
	}

	if err := DeleteFile(path); err != nil {
		t.Fatalf("DeleteFile: %v", err)
	}

	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("file should not exist after delete")
	}
}

func TestDeleteFile_NotFound(t *testing.T) {
	err := DeleteFile(filepath.Join(t.TempDir(), "nonexistent.txt"))
	if err == nil {
		t.Error("expected error for nonexistent file")
	}
}
