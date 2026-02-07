package storage

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func createTestZip(t *testing.T, files map[string]string) *bytes.Buffer {
	t.Helper()
	buf := new(bytes.Buffer)
	w := zip.NewWriter(buf)
	for name, content := range files {
		f, err := w.Create(name)
		if err != nil {
			t.Fatalf("creating zip entry %s: %v", name, err)
		}
		if _, err := f.Write([]byte(content)); err != nil {
			t.Fatalf("writing zip entry %s: %v", name, err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatalf("closing zip: %v", err)
	}
	return buf
}

func TestGetDeploymentPath(t *testing.T) {
	m := NewManager("/base")
	got := m.GetDeploymentPath("site1", 3)
	want := filepath.Join("/base", "site1", "3")
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestExtractZip_Basic(t *testing.T) {
	m := NewManager(t.TempDir())
	buf := createTestZip(t, map[string]string{
		"index.html":       "<html>hello</html>",
		"assets/style.css": "body{}",
	})

	if err := m.ExtractZip("s1", 1, bytes.NewReader(buf.Bytes()), int64(buf.Len())); err != nil {
		t.Fatalf("ExtractZip: %v", err)
	}

	deployPath := m.GetDeploymentPath("s1", 1)
	data, err := os.ReadFile(filepath.Join(deployPath, "index.html"))
	if err != nil {
		t.Fatalf("reading index.html: %v", err)
	}
	if string(data) != "<html>hello</html>" {
		t.Errorf("index.html content = %q", string(data))
	}

	data, err = os.ReadFile(filepath.Join(deployPath, "assets", "style.css"))
	if err != nil {
		t.Fatalf("reading style.css: %v", err)
	}
	if string(data) != "body{}" {
		t.Errorf("style.css content = %q", string(data))
	}
}

func TestExtractZip_FlattensTopDir(t *testing.T) {
	m := NewManager(t.TempDir())
	buf := createTestZip(t, map[string]string{
		"dist/index.html": "hello",
		"dist/app.js":     "console.log(1)",
	})

	if err := m.ExtractZip("s1", 1, bytes.NewReader(buf.Bytes()), int64(buf.Len())); err != nil {
		t.Fatalf("ExtractZip: %v", err)
	}

	deployPath := m.GetDeploymentPath("s1", 1)
	if _, err := os.Stat(filepath.Join(deployPath, "index.html")); err != nil {
		t.Error("expected index.html at root (flattened), not inside dist/")
	}
}

func TestExtractZip_NoFlattenMultipleTopDirs(t *testing.T) {
	m := NewManager(t.TempDir())
	buf := createTestZip(t, map[string]string{
		"dir1/a.txt": "a",
		"dir2/b.txt": "b",
	})

	if err := m.ExtractZip("s1", 1, bytes.NewReader(buf.Bytes()), int64(buf.Len())); err != nil {
		t.Fatalf("ExtractZip: %v", err)
	}

	deployPath := m.GetDeploymentPath("s1", 1)
	if _, err := os.Stat(filepath.Join(deployPath, "dir1", "a.txt")); err != nil {
		t.Error("dir1/a.txt should be preserved")
	}
	if _, err := os.Stat(filepath.Join(deployPath, "dir2", "b.txt")); err != nil {
		t.Error("dir2/b.txt should be preserved")
	}
}

func TestExtractZip_ZipSlipProtection(t *testing.T) {
	// Manually create a zip with a path-traversal entry
	buf := new(bytes.Buffer)
	w := zip.NewWriter(buf)
	f, _ := w.Create("../../../etc/passwd")
	f.Write([]byte("evil"))
	w.Close()

	m := NewManager(t.TempDir())
	err := m.ExtractZip("s1", 1, bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err == nil {
		t.Fatal("expected zip slip error")
	}
	if !strings.Contains(err.Error(), "zip slip") {
		t.Errorf("error = %q, want zip slip message", err.Error())
	}
}

func TestResolveFile_ExactFile(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "about.html"), []byte("about"), 0644)

	m := NewManager("")
	path, ok := m.ResolveFile(dir, "/about.html")
	if !ok {
		t.Fatal("expected to find about.html")
	}
	if !strings.HasSuffix(path, "about.html") {
		t.Errorf("path = %q", path)
	}
}

func TestResolveFile_IndexHtml(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "blog")
	os.MkdirAll(sub, 0755)
	os.WriteFile(filepath.Join(sub, "index.html"), []byte("blog index"), 0644)

	m := NewManager("")
	path, ok := m.ResolveFile(dir, "/blog")
	if !ok {
		t.Fatal("expected to find blog/index.html")
	}
	if !strings.HasSuffix(path, filepath.Join("blog", "index.html")) {
		t.Errorf("path = %q", path)
	}
}

func TestResolveFile_DotHtml(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "about.html"), []byte("about"), 0644)

	m := NewManager("")
	path, ok := m.ResolveFile(dir, "/about")
	if !ok {
		t.Fatal("expected to find about.html via .html fallback")
	}
	if !strings.HasSuffix(path, "about.html") {
		t.Errorf("path = %q", path)
	}
}

func TestResolveFile_Root(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "index.html"), []byte("home"), 0644)

	m := NewManager("")
	path, ok := m.ResolveFile(dir, "/")
	if !ok {
		t.Fatal("expected to find index.html for /")
	}
	if !strings.HasSuffix(path, "index.html") {
		t.Errorf("path = %q", path)
	}
}

func TestResolveFile_NotFound(t *testing.T) {
	dir := t.TempDir()
	m := NewManager("")
	_, ok := m.ResolveFile(dir, "/nope.txt")
	if ok {
		t.Fatal("expected not found")
	}
}

func TestDeleteSite(t *testing.T) {
	base := t.TempDir()
	m := NewManager(base)

	siteDir := filepath.Join(base, "s1")
	os.MkdirAll(filepath.Join(siteDir, "1"), 0755)
	os.WriteFile(filepath.Join(siteDir, "1", "index.html"), []byte("hi"), 0644)

	if err := m.DeleteSite("s1"); err != nil {
		t.Fatalf("DeleteSite: %v", err)
	}
	if _, err := os.Stat(siteDir); !os.IsNotExist(err) {
		t.Error("site directory should be removed")
	}
}
