package worker

import (
	"fmt"
	"strings"
	"testing"

	v8 "github.com/tommie/v8go"
)

func TestWrapESModule_ExportDefault(t *testing.T) {
	source := `export default { fetch(req) { return new Response("ok"); } };`
	result := wrapESModule(source)
	if result == source {
		t.Error("wrapESModule should transform export default")
	}
	if !strings.Contains(result, "globalThis.__worker_module__") {
		t.Errorf("result should set __worker_module__, got %q", result)
	}
	if strings.Contains(result, "export default") {
		t.Errorf("result should not contain 'export default', got %q", result)
	}
}

func TestWrapESModule_ExportAsDefault(t *testing.T) {
	source := `var handler = { fetch(req) { return new Response("ok"); } };
export { handler as default };`
	result := wrapESModule(source)
	if !strings.Contains(result, "globalThis.__worker_module__ = handler") {
		t.Errorf("should assign handler as default, got %q", result)
	}
}

func TestWrapESModule_NamedExports(t *testing.T) {
	source := `function fetch(req) { return new Response("ok"); }
function scheduled(event) {}
export { fetch, scheduled };`
	result := wrapESModule(source)
	if !strings.Contains(result, "globalThis.__worker_module__") {
		t.Errorf("should set __worker_module__, got %q", result)
	}
	if !strings.Contains(result, "fetch") || !strings.Contains(result, "scheduled") {
		t.Errorf("should include both exports, got %q", result)
	}
}

func TestWrapESModule_InlineExports(t *testing.T) {
	source := `export async function fetch(req) { return new Response("ok"); }
export function scheduled(event) {}`
	result := wrapESModule(source)
	if !strings.Contains(result, "globalThis.__worker_module__") {
		t.Errorf("should set __worker_module__, got %q", result)
	}
	// Should strip 'export' keyword from inline exports
	if strings.Contains(result, "export async function") || strings.Contains(result, "export function") {
		t.Errorf("should strip export keyword, got %q", result)
	}
}

func TestWrapESModule_NoExport(t *testing.T) {
	source := `globalThis.__worker_module__ = { fetch(req) { return new Response("ok"); } };`
	result := wrapESModule(source)
	if result != source {
		t.Errorf("source without exports should pass through unchanged")
	}
}

func TestParseExportBlock_DefaultExport(t *testing.T) {
	defaultName, names := parseExportBlock("handler as default")
	if defaultName != "handler" {
		t.Errorf("defaultName = %q, want 'handler'", defaultName)
	}
	if len(names) != 0 {
		t.Errorf("names = %v, want empty", names)
	}
}

func TestParseExportBlock_NamedExports(t *testing.T) {
	_, names := parseExportBlock("fetch, scheduled")
	if len(names) != 2 {
		t.Fatalf("names len = %d, want 2", len(names))
	}
	if names[0] != "fetch" || names[1] != "scheduled" {
		t.Errorf("names = %v", names)
	}
}

func TestParseExportBlock_AliasedExport(t *testing.T) {
	_, names := parseExportBlock("myFetch as fetch")
	if len(names) != 1 || names[0] != "fetch: myFetch" {
		t.Errorf("names = %v, want ['fetch: myFetch']", names)
	}
}

func TestParseExportBlock_EmptyEntries(t *testing.T) {
	defaultName, names := parseExportBlock("  , , fetch ,  ")
	if defaultName != "" {
		t.Errorf("defaultName = %q, want empty", defaultName)
	}
	if len(names) != 1 || names[0] != "fetch" {
		t.Errorf("names = %v, want ['fetch']", names)
	}
}

func TestParseExportBlock_MixedExports(t *testing.T) {
	defaultName, names := parseExportBlock("handler as default, scheduled, myFetch as fetch")
	if defaultName != "handler" {
		t.Errorf("defaultName = %q, want 'handler'", defaultName)
	}
	if len(names) != 2 {
		t.Fatalf("names len = %d, want 2", len(names))
	}
}

func TestNewV8Pool_InvalidScript(t *testing.T) {
	_, err := newV8Pool(1, "function {{{invalid syntax", []setupFunc{}, 0)
	if err == nil {
		t.Fatal("newV8Pool should fail with invalid JS")
	}
}

func TestNewV8Pool_NoDefaultExport(t *testing.T) {
	_, err := newV8Pool(1, "var x = 42;", []setupFunc{}, 0)
	if err == nil {
		t.Fatal("newV8Pool should fail when script has no default export")
	}
	if !strings.Contains(err.Error(), "did not export a default module") {
		t.Errorf("error = %q, should mention missing default module", err)
	}
}

func TestNewV8Pool_WithMemoryLimit(t *testing.T) {
	source := `export default { fetch() { return new Response("ok"); } };`
	pool, err := newV8Pool(1, source, []setupFunc{setupWebAPIs}, 64)
	if err != nil {
		t.Fatalf("newV8Pool with memory limit: %v", err)
	}
	defer pool.dispose()

	if pool.size != 1 {
		t.Errorf("pool.size = %d, want 1", pool.size)
	}
}

func TestNewV8Pool_SetupFuncError(t *testing.T) {
	source := `export default { fetch() { return new Response("ok"); } };`
	badSetup := func(iso *v8.Isolate, ctx *v8.Context, el *eventLoop) error {
		return fmt.Errorf("setup failed")
	}
	_, err := newV8Pool(1, source, []setupFunc{badSetup}, 0)
	if err == nil {
		t.Fatal("newV8Pool should fail when setup function errors")
	}
	if !strings.Contains(err.Error(), "setup failed") {
		t.Errorf("error = %q, should mention 'setup failed'", err)
	}
}

func TestPool_GetPutCycle(t *testing.T) {
	source := `export default { fetch() { return new Response("ok"); } };`
	pool, err := newV8Pool(2, source, []setupFunc{setupWebAPIs}, 0)
	if err != nil {
		t.Fatalf("newV8Pool: %v", err)
	}
	defer pool.dispose()

	// Get a worker.
	w, err := pool.get()
	if err != nil {
		t.Fatalf("get: %v", err)
	}

	// Put it back.
	pool.put(w)

	// Should be able to get it again.
	w2, err := pool.get()
	if err != nil {
		t.Fatalf("get after put: %v", err)
	}
	pool.put(w2)
}

func TestPool_Dispose(t *testing.T) {
	source := `export default { fetch() { return new Response("ok"); } };`
	pool, err := newV8Pool(2, source, []setupFunc{setupWebAPIs}, 0)
	if err != nil {
		t.Fatalf("newV8Pool: %v", err)
	}

	// Dispose should not panic.
	pool.dispose()

	// Double dispose should not panic either.
	pool.dispose()
}

func TestPool_PutOverflow(t *testing.T) {
	source := `export default { fetch() { return new Response("ok"); } };`
	// Pool of size 1.
	pool, err := newV8Pool(1, source, []setupFunc{setupWebAPIs}, 0)
	if err != nil {
		t.Fatalf("newV8Pool: %v", err)
	}

	// Get the single worker.
	w1, err := pool.get()
	if err != nil {
		t.Fatalf("get: %v", err)
	}

	// Put it back (fills the channel).
	pool.put(w1)

	// Create a second worker manually.
	w2, err := newV8Worker(source, []setupFunc{setupWebAPIs}, 0)
	if err != nil {
		t.Fatalf("newV8Worker: %v", err)
	}

	// Put the second worker — channel is full, so it should be disposed (default branch).
	pool.put(w2)

	pool.dispose()
}

func TestPool_GetAfterDispose(t *testing.T) {
	source := `export default { fetch() { return new Response("ok"); } };`
	pool, err := newV8Pool(1, source, []setupFunc{setupWebAPIs}, 0)
	if err != nil {
		t.Fatalf("newV8Pool: %v", err)
	}

	// Dispose empties workers and closes aren't done explicitly, but drain.
	pool.dispose()

	// Pool workers channel is now empty. A get would block forever, so we
	// test via non-blocking select. The pool doesn't close the channel, so
	// get() would block. We just verify dispose didn't panic.
}

func TestWrapESModule_ExportConst(t *testing.T) {
	source := `export const handler = { fetch(req) { return new Response("ok"); } };`
	result := wrapESModule(source)
	if !strings.Contains(result, "globalThis.__worker_module__") {
		t.Errorf("should set __worker_module__, got %q", result)
	}
	if strings.Contains(result, "export const") {
		t.Errorf("should strip 'export const', got %q", result)
	}
}

func TestWrapESModule_ExportClass(t *testing.T) {
	source := `export class Worker { fetch(req) { return new Response("ok"); } }`
	result := wrapESModule(source)
	if !strings.Contains(result, "globalThis.__worker_module__") {
		t.Errorf("should set __worker_module__, got %q", result)
	}
	if strings.Contains(result, "export class") {
		t.Errorf("should strip 'export class', got %q", result)
	}
}

func TestWrapESModule_ExportLet(t *testing.T) {
	source := `export let x = 42;`
	result := wrapESModule(source)
	if !strings.Contains(result, "globalThis.__worker_module__") {
		t.Errorf("should set __worker_module__, got %q", result)
	}
	if strings.Contains(result, "export let") {
		t.Errorf("should strip 'export let', got %q", result)
	}
}

func TestWrapESModule_ExportVar(t *testing.T) {
	source := `export var y = 99;`
	result := wrapESModule(source)
	if !strings.Contains(result, "globalThis.__worker_module__") {
		t.Errorf("should set __worker_module__, got %q", result)
	}
	if strings.Contains(result, "export var") {
		t.Errorf("should strip 'export var', got %q", result)
	}
}
