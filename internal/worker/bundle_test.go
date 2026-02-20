package worker

import (
	"os"
	"path/filepath"
	"strings"
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
		{"node: import single quotes", `import { Buffer } from 'node:buffer';`, true},
		{"node: import double quotes", `import crypto from "node:crypto";`, true},
		{"require call", `const fs = require('fs');`, true},
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

func TestFindUnenvPath(t *testing.T) {
	ResetUnenvCache()
	defer ResetUnenvCache()

	path := findUnenvPath()
	if path == "" {
		t.Skip("unenv not installed, skipping")
	}

	// Verify the path contains the expected structure.
	runtimeNode := filepath.Join(path, "runtime", "node")
	if info, err := os.Stat(runtimeNode); err != nil || !info.IsDir() {
		t.Errorf("expected %s to be a directory", runtimeNode)
	}

	for _, mod := range []string{"buffer", "crypto", "path", "events", "stream", "util"} {
		idx := filepath.Join(runtimeNode, mod, "index.mjs")
		if _, err := os.Stat(idx); err != nil {
			t.Errorf("expected polyfill at %s: %v", idx, err)
		}
	}
}

func TestBundleWorkerScript_NodeBuffer(t *testing.T) {
	ResetUnenvCache()
	defer ResetUnenvCache()

	if findUnenvPath() == "" {
		t.Skip("unenv not installed, skipping")
	}

	dir := t.TempDir()
	workerSrc := "import { Buffer } from 'node:buffer';\nexport default {\n  fetch(req) {\n    const buf = Buffer.from('hello');\n    return new Response(buf.toString('base64'));\n  }\n}"
	if err := os.WriteFile(filepath.Join(dir, "_worker.js"), []byte(workerSrc), 0644); err != nil {
		t.Fatal(err)
	}

	result, err := BundleWorkerScript(dir)
	if err != nil {
		t.Fatalf("bundling node:buffer import failed: %v", err)
	}
	if len(result) == 0 {
		t.Fatal("bundled output should not be empty")
	}
	if strings.Contains(result, `"node:buffer"`) || strings.Contains(result, `'node:buffer'`) {
		t.Error("bundled output still contains unresolved node:buffer import")
	}
}

func TestBundleWorkerScript_NodePath(t *testing.T) {
	ResetUnenvCache()
	defer ResetUnenvCache()

	if findUnenvPath() == "" {
		t.Skip("unenv not installed, skipping")
	}

	dir := t.TempDir()
	workerSrc := "import path from 'node:path';\nexport default {\n  fetch(req) {\n    return new Response(path.join('/a', 'b', 'c'));\n  }\n}"
	if err := os.WriteFile(filepath.Join(dir, "_worker.js"), []byte(workerSrc), 0644); err != nil {
		t.Fatal(err)
	}

	result, err := BundleWorkerScript(dir)
	if err != nil {
		t.Fatalf("bundling node:path import failed: %v", err)
	}
	if strings.Contains(result, `"node:path"`) || strings.Contains(result, `'node:path'`) {
		t.Error("bundled output still contains unresolved node:path import")
	}
}

func TestBundleWorkerScript_NodeCrypto(t *testing.T) {
	ResetUnenvCache()
	defer ResetUnenvCache()

	if findUnenvPath() == "" {
		t.Skip("unenv not installed, skipping")
	}

	dir := t.TempDir()
	workerSrc := "import crypto from 'node:crypto';\nexport default {\n  fetch(req) {\n    return new Response('ok');\n  }\n}"
	if err := os.WriteFile(filepath.Join(dir, "_worker.js"), []byte(workerSrc), 0644); err != nil {
		t.Fatal(err)
	}

	result, err := BundleWorkerScript(dir)
	if err != nil {
		t.Fatalf("bundling node:crypto import failed: %v", err)
	}
	if strings.Contains(result, `"node:crypto"`) || strings.Contains(result, `'node:crypto'`) {
		t.Error("bundled output still contains unresolved node:crypto import")
	}
}

func TestBundleWorkerScript_BareModuleSpecifier(t *testing.T) {
	ResetUnenvCache()
	defer ResetUnenvCache()

	if findUnenvPath() == "" {
		t.Skip("unenv not installed, skipping")
	}

	dir := t.TempDir()
	workerSrc := "import { join } from 'path';\nexport default {\n  fetch(req) {\n    return new Response(join('/a', 'b'));\n  }\n}"
	if err := os.WriteFile(filepath.Join(dir, "_worker.js"), []byte(workerSrc), 0644); err != nil {
		t.Fatal(err)
	}

	result, err := BundleWorkerScript(dir)
	if err != nil {
		t.Fatalf("bundling bare 'path' import failed: %v", err)
	}
	if len(result) == 0 {
		t.Fatal("bundled output should not be empty")
	}
}

func TestBundleWorkerScript_NoUnenv(t *testing.T) {
	ResetUnenvCache()
	defer ResetUnenvCache()

	// Point to a nonexistent path so unenv is "not installed".
	t.Setenv("HOSTEDAT_UNENV_PATH", filepath.Join(t.TempDir(), "nonexistent"))

	dir := t.TempDir()
	workerSrc := "import { Buffer } from 'node:buffer';\nexport default { fetch(req) { return new Response('ok'); } }"
	if err := os.WriteFile(filepath.Join(dir, "_worker.js"), []byte(workerSrc), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := BundleWorkerScript(dir)
	if err == nil {
		t.Fatal("expected error when bundling node: import without unenv")
	}
}

func TestBundleWorkerScript_AllNodeModules(t *testing.T) {
	ResetUnenvCache()
	defer ResetUnenvCache()

	if findUnenvPath() == "" {
		t.Skip("unenv not installed, skipping")
	}

	for _, mod := range nodeCompatModules {
		t.Run("node:"+mod, func(t *testing.T) {
			ResetUnenvCache()

			dir := t.TempDir()
			workerSrc := "import m from 'node:" + mod + "';\nexport default { fetch(req) { return new Response('ok'); } }"
			if err := os.WriteFile(filepath.Join(dir, "_worker.js"), []byte(workerSrc), 0644); err != nil {
				t.Fatal(err)
			}

			result, err := BundleWorkerScript(dir)
			if err != nil {
				t.Fatalf("bundling node:%s failed: %v", mod, err)
			}
			if len(result) == 0 {
				t.Fatalf("bundled output for node:%s should not be empty", mod)
			}
		})
	}
}
