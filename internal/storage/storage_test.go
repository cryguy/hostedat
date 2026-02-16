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

func TestResolveFile_PathTraversal(t *testing.T) {
	dir := t.TempDir()
	// Create a file outside the deployment dir
	os.WriteFile(filepath.Join(dir, "secret.txt"), []byte("secret"), 0644)

	deployDir := filepath.Join(dir, "deploy")
	os.MkdirAll(deployDir, 0755)
	os.WriteFile(filepath.Join(deployDir, "index.html"), []byte("home"), 0644)

	m := NewManager("")
	// Attempt path traversal
	_, ok := m.ResolveFile(deployDir, "/../secret.txt")
	if ok {
		t.Error("path traversal should be blocked")
	}

	_, ok = m.ResolveFile(deployDir, "/../../../etc/passwd")
	if ok {
		t.Error("path traversal to /etc/passwd should be blocked")
	}

	// Normal file should still work
	_, ok = m.ResolveFile(deployDir, "/index.html")
	if !ok {
		t.Error("normal file should resolve")
	}
}

func TestExtractZip_RejectsSymlinks(t *testing.T) {
	// zip.File entries with mode ModeSymlink are rejected.
	// Create a zip manually with a symlink entry.
	buf := new(bytes.Buffer)
	w := zip.NewWriter(buf)

	// Create a symlink entry by setting the external attrs
	header := &zip.FileHeader{
		Name: "link.txt",
	}
	header.SetMode(os.ModeSymlink | 0777)
	fw, err := w.CreateHeader(header)
	if err != nil {
		t.Fatalf("creating symlink entry: %v", err)
	}
	fw.Write([]byte("../etc/passwd"))
	w.Close()

	m := NewManager(t.TempDir())
	err = m.ExtractZip("s1", 1, bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err == nil {
		t.Fatal("expected error for symlink in zip")
	}
	if !strings.Contains(err.Error(), "symlink") {
		t.Errorf("error = %q, want symlink message", err.Error())
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

func TestHasWorkerScript_Present(t *testing.T) {
	store := NewManager(t.TempDir())
	siteID := "test-site"
	version := 1

	// Create deployment dir with _worker.js
	deployPath := store.GetDeploymentPath(siteID, version)
	os.MkdirAll(deployPath, 0755)
	os.WriteFile(filepath.Join(deployPath, "_worker.js"), []byte("export default {}"), 0644)

	if !store.HasWorkerScript(siteID, version) {
		t.Error("expected HasWorkerScript to return true")
	}
}

func TestHasWorkerScript_Absent(t *testing.T) {
	store := NewManager(t.TempDir())
	// No _worker.js file
	if store.HasWorkerScript("nonexistent", 1) {
		t.Error("expected HasWorkerScript to return false")
	}
}

func TestGetWorkerScript(t *testing.T) {
	store := NewManager(t.TempDir())
	siteID := "test-site"
	version := 1

	deployPath := store.GetDeploymentPath(siteID, version)
	os.MkdirAll(deployPath, 0755)
	content := "export default { fetch() { return new Response('hi') } }"
	os.WriteFile(filepath.Join(deployPath, "_worker.js"), []byte(content), 0644)

	got, err := store.GetWorkerScript(siteID, version)
	if err != nil {
		t.Fatal(err)
	}
	if got != content {
		t.Errorf("got %q, want %q", got, content)
	}
}

func TestGetWorkerScript_NotFound(t *testing.T) {
	store := NewManager(t.TempDir())
	_, err := store.GetWorkerScript("nonexistent", 1)
	if err == nil {
		t.Fatal("expected error for missing _worker.js")
	}
}

func TestGetWorkerBytecodeDir(t *testing.T) {
	store := NewManager(t.TempDir())
	dir := store.GetWorkerBytecodeDir("my-site", 3)
	// Should be a reasonable path containing the site ID and version
	if dir == "" {
		t.Error("expected non-empty path")
	}
	// Verify it contains expected components
	if !strings.Contains(dir, "my-site") {
		t.Error("expected siteID in path")
	}
	if !strings.Contains(dir, "3") {
		t.Error("expected version in path")
	}
	if !strings.HasSuffix(dir, ".worker") {
		t.Error("expected .worker suffix")
	}
}

func TestExtractZip_WithWorkerScript(t *testing.T) {
	m := NewManager(t.TempDir())
	buf := createTestZip(t, map[string]string{
		"index.html":  "<html>app</html>",
		"_worker.js":  "export default { fetch() { return new Response('worker') } }",
		"assets/a.js": "console.log('a')",
	})

	if err := m.ExtractZip("s1", 1, bytes.NewReader(buf.Bytes()), int64(buf.Len())); err != nil {
		t.Fatalf("ExtractZip: %v", err)
	}

	// Verify worker script was extracted
	if !m.HasWorkerScript("s1", 1) {
		t.Error("expected _worker.js to be present")
	}

	script, err := m.GetWorkerScript("s1", 1)
	if err != nil {
		t.Fatalf("GetWorkerScript: %v", err)
	}
	if !strings.Contains(script, "fetch()") {
		t.Errorf("worker script content = %q", script)
	}
}

func TestExtractZip_NestedSubdirectory(t *testing.T) {
	// Test that subdirectories within a single top-level dir are preserved
	m := NewManager(t.TempDir())
	buf := createTestZip(t, map[string]string{
		"dist/index.html":         "home",
		"dist/assets/css/app.css": "body{}",
		"dist/assets/js/app.js":   "console.log(1)",
	})

	if err := m.ExtractZip("s1", 1, bytes.NewReader(buf.Bytes()), int64(buf.Len())); err != nil {
		t.Fatalf("ExtractZip: %v", err)
	}

	deployPath := m.GetDeploymentPath("s1", 1)
	// dist/ should be stripped, but subdirs preserved
	if _, err := os.Stat(filepath.Join(deployPath, "index.html")); err != nil {
		t.Error("expected index.html at root (flattened)")
	}
	if _, err := os.Stat(filepath.Join(deployPath, "assets", "css", "app.css")); err != nil {
		t.Error("expected assets/css/app.css to be preserved")
	}
	if _, err := os.Stat(filepath.Join(deployPath, "assets", "js", "app.js")); err != nil {
		t.Error("expected assets/js/app.js to be preserved")
	}
}

func TestExtractZip_EmptyZip(t *testing.T) {
	// Test extraction with empty zip (0 files)
	m := NewManager(t.TempDir())
	buf := createTestZip(t, map[string]string{})

	if err := m.ExtractZip("s1", 1, bytes.NewReader(buf.Bytes()), int64(buf.Len())); err != nil {
		t.Fatalf("ExtractZip with empty zip: %v", err)
	}

	deployPath := m.GetDeploymentPath("s1", 1)
	// Directory should still be created
	if _, err := os.Stat(deployPath); err != nil {
		t.Error("expected deployment directory to be created even for empty zip")
	}
}

func TestExtractZip_TooLarge(t *testing.T) {
	m := NewManager(t.TempDir())
	// Create a buffer and claim it's too large
	buf := createTestZip(t, map[string]string{"index.html": "test"})

	err := m.ExtractZip("s1", 1, bytes.NewReader(buf.Bytes()), MaxZipSize+1)
	if err == nil {
		t.Fatal("expected error for zip too large")
	}
	if !strings.Contains(err.Error(), "too large") {
		t.Errorf("error = %q, want 'too large' message", err.Error())
	}
}
