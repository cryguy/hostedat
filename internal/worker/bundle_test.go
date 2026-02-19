package worker

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNeedsBundling(t *testing.T) {
	tests := []struct {
		name   string
		source string
		want   bool
	}{
		{"no imports", "export default { fetch() {} }", false},
		{"import statement", `import { foo } from './utils.js';`, true},
		{"import no space", `import{foo} from './utils.js';`, true},
		{"dynamic import", `const m = import('./mod.js');`, true},
		{"comment with import word", `// this is important\nexport default {}`, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := needsBundling(tt.source)
			if got != tt.want {
				t.Errorf("needsBundling(%q) = %v, want %v", tt.source, got, tt.want)
			}
		})
	}
}

func TestBundleWorkerScript_NoImports(t *testing.T) {
	dir := t.TempDir()
	src := `export default { fetch(req) { return new Response("ok"); } }`
	if err := os.WriteFile(filepath.Join(dir, "_worker.js"), []byte(src), 0644); err != nil {
		t.Fatal(err)
	}

	result, err := BundleWorkerScript(dir)
	if err != nil {
		t.Fatal(err)
	}
	// Should return source as-is since no imports
	if result != src {
		t.Errorf("expected source unchanged, got %q", result)
	}
}

func TestBundleWorkerScript_WithImports(t *testing.T) {
	dir := t.TempDir()

	// Create a utility module
	utilSrc := `export function greet(name) { return "Hello " + name; }`
	if err := os.WriteFile(filepath.Join(dir, "utils.js"), []byte(utilSrc), 0644); err != nil {
		t.Fatal(err)
	}

	// Create worker that imports from utils
	workerSrc := `import { greet } from './utils.js';
export default {
  fetch(req) {
    return new Response(greet("World"));
  }
}`
	if err := os.WriteFile(filepath.Join(dir, "_worker.js"), []byte(workerSrc), 0644); err != nil {
		t.Fatal(err)
	}

	result, err := BundleWorkerScript(dir)
	if err != nil {
		t.Fatal(err)
	}

	// Bundled output should contain the greet function inline
	if result == workerSrc {
		t.Error("bundled output should differ from source")
	}
	if len(result) == 0 {
		t.Error("bundled output should not be empty")
	}
}

func TestBundleWorkerScript_MissingFile(t *testing.T) {
	dir := t.TempDir()
	_, err := BundleWorkerScript(dir)
	if err == nil {
		t.Fatal("expected error for missing _worker.js")
	}
}

func TestBundleWorkerScript_InvalidImport(t *testing.T) {
	dir := t.TempDir()

	// Worker that imports from a nonexistent file
	workerSrc := `import { foo } from './nonexistent.js';
export default { fetch(req) { return new Response(foo()); } }`
	if err := os.WriteFile(filepath.Join(dir, "_worker.js"), []byte(workerSrc), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := BundleWorkerScript(dir)
	if err == nil {
		t.Fatal("expected error for invalid import")
	}
}
