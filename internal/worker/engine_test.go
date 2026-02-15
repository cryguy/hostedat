package worker

import (
	"context"
	"testing"

	"github.com/fastschema/qjs"
)

func TestModuleDefaultExportFetch(t *testing.T) {
	source := `export default {
  fetch(request, env, ctx) {
    return new Response("it works");
  }
};`

	opt := qjs.Option{
		Context:          context.Background(),
		MemoryLimit:      128 * 1024 * 1024,
		MaxStackSize:     1024 * 1024,
		MaxExecutionTime: 5000,
		GCThreshold:      256 * 1024,
	}

	// Compile on one runtime (same as CompileAndCache).
	compileRT, err := qjs.New(opt)
	if err != nil {
		t.Fatalf("creating compile runtime: %v", err)
	}
	bytecode, err := compileRT.Compile("worker.js", qjs.Code(source), qjs.TypeModule())
	compileRT.Close()
	if err != nil {
		t.Fatalf("compiling worker: %v", err)
	}

	// Evaluate on a separate runtime (same as pool setup).
	evalRT, err := qjs.New(opt)
	if err != nil {
		t.Fatalf("creating eval runtime: %v", err)
	}
	defer evalRT.Close()

	// Inject Response constructor so the worker code can use it.
	if err := setupWebAPIs(evalRT); err != nil {
		t.Fatalf("setupWebAPIs: %v", err)
	}

	// Load the module (registers it in the runtime).
	if _, err := evalRT.Load("worker.js", qjs.Bytecode(bytecode)); err != nil {
		t.Fatalf("loading bytecode: %v", err)
	}

	// Import the default export.
	defaultExport, err := evalRT.Eval("__worker_import__.js",
		qjs.Code(`import mod from 'worker.js'; export default mod;`),
		qjs.TypeModule(),
	)
	if err != nil {
		t.Fatalf("importing module: %v", err)
	}
	defer defaultExport.Free()

	if defaultExport.IsUndefined() || defaultExport.IsNull() {
		t.Fatal("default export is undefined/null")
	}

	// Verify fetch is callable.
	ctx := evalRT.Context()
	reqObj := ctx.NewObject()
	reqObj.SetPropertyStr("method", ctx.NewString("GET"))
	reqObj.SetPropertyStr("url", ctx.NewString("http://localhost/"))
	reqObj.SetPropertyStr("headers", ctx.NewObject())

	result, err := defaultExport.InvokeJS("fetch", reqObj, ctx.NewObject(), ctx.NewObject())
	if err != nil {
		t.Fatalf("invoking fetch: %v", err)
	}
	defer result.Free()

	t.Logf("fetch returned successfully (isPromise=%v, isObject=%v)", result.IsPromise(), result.IsObject())
}
