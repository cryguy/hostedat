package worker

import (
	"context"
	"fmt"
	"testing"

	"github.com/cryguy/hostedat/internal/config"
	"github.com/cryguy/hostedat/internal/models"
	"github.com/fastschema/qjs"
)

func TestModuleDefaultExportFetch(t *testing.T) {
	source := `export default {
  fetch(request, env, ctx) {
    return new Response("it works");
  }
};`

	opt := qjs.Option{
		Context:            context.Background(),
		CloseOnContextDone: true,
		MemoryLimit:        128 * 1024 * 1024,
		MaxStackSize:       1024 * 1024,
		MaxExecutionTime:   5000,
		GCThreshold:        256 * 1024,
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

// TestPoolModuleFlow tests the full pool setup path (all 4 setup functions)
// matching the exact production flow in GetOrCreatePool + Execute.
func TestPoolModuleFlow(t *testing.T) {
	source := `export default {
  fetch(request, env, ctx) {
    return new Response("hello from pool test");
  }
};`

	cfg := config.WorkerConfig{
		PoolSize:         2,
		MemoryLimitMB:    128,
		ExecutionTimeout: 5000,
		MaxFetchRequests: 10,
		FetchTimeoutSec:  5,
		MaxResponseBytes: 1024 * 1024,
	}

	opt := qjs.Option{
		Context:          context.Background(),
		MemoryLimit:      cfg.MemoryLimitMB * 1024 * 1024,
		MaxStackSize:     1024 * 1024,
		MaxExecutionTime: cfg.ExecutionTimeout,
		GCThreshold:      256 * 1024,
	}

	// Step 1: Compile on a throwaway runtime (same as CompileAndCache).
	compileRT, err := qjs.New(opt)
	if err != nil {
		t.Fatalf("creating compile runtime: %v", err)
	}
	bytecode, err := compileRT.Compile("worker.js", qjs.Code(source), qjs.TypeModule())
	compileRT.Close()
	if err != nil {
		t.Fatalf("compiling: %v", err)
	}
	t.Logf("bytecode length: %d", len(bytecode))

	// Step 2: Create pool with the exact same setup functions as GetOrCreatePool.
	pool := qjs.NewPool(cfg.PoolSize, opt,
		setupWebAPIs,
		setupConsole,
		func(rt *qjs.Runtime) error {
			return setupFetch(rt, cfg)
		},
		func(rt *qjs.Runtime) error {
			if _, err := rt.Load("worker.js", qjs.Bytecode(bytecode)); err != nil {
				return fmt.Errorf("loading worker bytecode: %w", err)
			}
			moduleVal, err := rt.Eval("__worker_import__.js",
				qjs.Code(`import mod from 'worker.js'; export default mod;`),
				qjs.TypeModule(),
			)
			if err != nil {
				return fmt.Errorf("importing worker module: %w", err)
			}
			rt.Context().Global().SetPropertyStr("__worker_module__", moduleVal)
			return nil
		},
	)

	// Step 3: Get a runtime from the pool (triggers setup).
	rt, err := pool.Get()
	if err != nil {
		t.Fatalf("pool.Get: %v", err)
	}
	defer pool.Put(rt)

	// Step 4: Check __worker_module__ (same as Execute does).
	ctx := rt.Context()
	defaultExport := ctx.Global().GetPropertyStr("__worker_module__")

	if defaultExport.IsUndefined() || defaultExport.IsNull() {
		t.Fatal("__worker_module__ is undefined/null — default export not captured")
	}

	// Step 5: Call fetch (same as Execute does).
	reqObj := ctx.NewObject()
	reqObj.SetPropertyStr("method", ctx.NewString("GET"))
	reqObj.SetPropertyStr("url", ctx.NewString("http://localhost/test"))
	reqObj.SetPropertyStr("headers", ctx.NewObject())

	fetchResult, err := defaultExport.InvokeJS("fetch", reqObj, ctx.NewObject(), ctx.NewObject())
	defaultExport.Free()
	if err != nil {
		t.Fatalf("invoking fetch: %v", err)
	}
	defer fetchResult.Free()

	t.Logf("pool flow: fetch returned (isPromise=%v, isObject=%v)", fetchResult.IsPromise(), fetchResult.IsObject())
}

// TestAsyncFetchHandler tests that async fetch handlers (returning Promise<Response>)
// are correctly awaited and converted, matching the exact Execute() flow.
func TestAsyncFetchHandler(t *testing.T) {
	source := `export default {
  async fetch(request) {
    const url = new URL(request.url);
    const name = url.searchParams.get("name") || "world";
    return new Response("Hello, " + name + "!");
  },
};`

	cfg := config.WorkerConfig{
		PoolSize:         2,
		MemoryLimitMB:    128,
		ExecutionTimeout: 5000,
		MaxFetchRequests: 10,
		FetchTimeoutSec:  5,
		MaxResponseBytes: 1024 * 1024,
	}

	opt := qjs.Option{
		Context:          context.Background(),
		MemoryLimit:      cfg.MemoryLimitMB * 1024 * 1024,
		MaxStackSize:     1024 * 1024,
		MaxExecutionTime: cfg.ExecutionTimeout,
		GCThreshold:      256 * 1024,
	}

	// Compile.
	compileRT, err := qjs.New(opt)
	if err != nil {
		t.Fatalf("compile runtime: %v", err)
	}
	bytecode, err := compileRT.Compile("worker.js", qjs.Code(source), qjs.TypeModule())
	compileRT.Close()
	if err != nil {
		t.Fatalf("compiling: %v", err)
	}

	// Create pool with all setup functions.
	pool := qjs.NewPool(cfg.PoolSize, opt,
		setupWebAPIs,
		setupConsole,
		func(rt *qjs.Runtime) error {
			return setupFetch(rt, cfg)
		},
		func(rt *qjs.Runtime) error {
			if _, err := rt.Load("worker.js", qjs.Bytecode(bytecode)); err != nil {
				return fmt.Errorf("loading: %w", err)
			}
			moduleVal, err := rt.Eval("__worker_import__.js",
				qjs.Code(`import mod from 'worker.js'; export default mod;`),
				qjs.TypeModule(),
			)
			if err != nil {
				return fmt.Errorf("importing: %w", err)
			}
			rt.Context().Global().SetPropertyStr("__worker_module__", moduleVal)
			return nil
		},
	)

	rt, err := pool.Get()
	if err != nil {
		t.Fatalf("pool.Get: %v", err)
	}
	defer pool.Put(rt)

	ctx := rt.Context()

	// Build a proper Request via the constructor (same as goRequestToJS).
	requestCtor := ctx.Global().GetPropertyStr("Request")
	jsReq := requestCtor.CallConstructor(ctx.NewString("http://localhost/api/hello?name=test"), ctx.NewObject())
	requestCtor.Free()
	if jsReq.IsError() {
		t.Fatalf("creating Request: %s", jsReq.String())
	}

	defaultExport := ctx.Global().GetPropertyStr("__worker_module__")
	if defaultExport.IsUndefined() || defaultExport.IsNull() {
		t.Fatal("__worker_module__ is undefined/null")
	}

	fetchResult, err := defaultExport.InvokeJS("fetch", jsReq, ctx.NewObject(), ctx.NewObject())
	defaultExport.Free()
	if err != nil {
		t.Fatalf("invoking fetch: %v", err)
	}

	t.Logf("async fetch result: isPromise=%v isObject=%v isUndefined=%v isNull=%v",
		fetchResult.IsPromise(), fetchResult.IsObject(), fetchResult.IsUndefined(), fetchResult.IsNull())

	// Await the promise (same as Execute does).
	if fetchResult.IsPromise() {
		awaited, err := fetchResult.Await()
		fetchResult.Free()
		if err != nil {
			t.Fatalf("awaiting promise: %v", err)
		}
		fetchResult = awaited
		t.Logf("awaited result: isObject=%v isUndefined=%v isNull=%v",
			fetchResult.IsObject(), fetchResult.IsUndefined(), fetchResult.IsNull())
	}

	// Convert to Go response (same as jsResponseToGo).
	resp, err := jsResponseToGo(ctx, fetchResult)
	fetchResult.Free()
	if err != nil {
		t.Fatalf("jsResponseToGo: %v", err)
	}
	if resp == nil {
		t.Fatal("response is nil")
	}

	t.Logf("response: status=%d body=%q headers=%v", resp.StatusCode, string(resp.Body), resp.Headers)
}

// TestAssetsFetch tests that env.ASSETS.fetch(request) works correctly
// by using a mock AssetsFetcher and running the full worker execution flow.
func TestAssetsFetch(t *testing.T) {
	source := `export default {
  async fetch(request, env) {
    return env.ASSETS.fetch(request);
  },
};`

	cfg := config.WorkerConfig{
		PoolSize:         2,
		MemoryLimitMB:    128,
		ExecutionTimeout: 5000,
		MaxFetchRequests: 10,
		FetchTimeoutSec:  5,
		MaxResponseBytes: 1024 * 1024,
	}

	opt := qjs.Option{
		Context:          context.Background(),
		MemoryLimit:      cfg.MemoryLimitMB * 1024 * 1024,
		MaxStackSize:     1024 * 1024,
		MaxExecutionTime: cfg.ExecutionTimeout,
		GCThreshold:      256 * 1024,
	}

	compileRT, err := qjs.New(opt)
	if err != nil {
		t.Fatalf("compile runtime: %v", err)
	}
	bytecode, err := compileRT.Compile("worker.js", qjs.Code(source), qjs.TypeModule())
	compileRT.Close()
	if err != nil {
		t.Fatalf("compiling: %v", err)
	}

	pool := qjs.NewPool(cfg.PoolSize, opt,
		setupWebAPIs,
		setupConsole,
		func(rt *qjs.Runtime) error {
			return setupFetch(rt, cfg)
		},
		func(rt *qjs.Runtime) error {
			if _, err := rt.Load("worker.js", qjs.Bytecode(bytecode)); err != nil {
				return fmt.Errorf("loading: %w", err)
			}
			moduleVal, err := rt.Eval("__worker_import__.js",
				qjs.Code(`import mod from 'worker.js'; export default mod;`),
				qjs.TypeModule(),
			)
			if err != nil {
				return fmt.Errorf("importing: %w", err)
			}
			rt.Context().Global().SetPropertyStr("__worker_module__", moduleVal)
			return nil
		},
	)

	rt, err := pool.Get()
	if err != nil {
		t.Fatalf("pool.Get: %v", err)
	}
	defer pool.Put(rt)

	ctx := rt.Context()

	// Build env with a mock ASSETS fetcher.
	envObj := ctx.NewObject()
	mockFetcher := &mockAssetsFetcher{
		response: &WorkerResponse{
			StatusCode: 200,
			Headers:    map[string]string{"content-type": "text/html; charset=utf-8"},
			Body:       []byte("<h1>Hello from ASSETS</h1>"),
		},
	}
	envObj.SetPropertyStr("ASSETS", buildAssetsBinding(ctx, mockFetcher))

	// Build Request.
	requestCtor := ctx.Global().GetPropertyStr("Request")
	jsReq := requestCtor.CallConstructor(ctx.NewString("http://localhost/index.html"), ctx.NewObject())
	requestCtor.Free()

	defaultExport := ctx.Global().GetPropertyStr("__worker_module__")
	fetchResult, err := defaultExport.InvokeJS("fetch", jsReq, envObj, ctx.NewObject())
	defaultExport.Free()
	if err != nil {
		t.Fatalf("invoking fetch: %v", err)
	}

	if fetchResult.IsPromise() {
		awaited, err := fetchResult.Await()
		fetchResult.Free()
		if err != nil {
			t.Fatalf("awaiting: %v", err)
		}
		fetchResult = awaited
	}

	resp, err := jsResponseToGo(ctx, fetchResult)
	fetchResult.Free()
	if err != nil {
		t.Fatalf("jsResponseToGo: %v", err)
	}
	if resp == nil {
		t.Fatal("response is nil")
	}
	if resp.StatusCode != 200 {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}
	if string(resp.Body) != "<h1>Hello from ASSETS</h1>" {
		t.Fatalf("unexpected body: %q", string(resp.Body))
	}
	t.Logf("ASSETS.fetch: status=%d body=%q", resp.StatusCode, string(resp.Body))
}

// mockAssetsFetcher implements AssetsFetcher for testing.
type mockAssetsFetcher struct {
	response *WorkerResponse
	err      error
}

func (m *mockAssetsFetcher) Fetch(req *WorkerRequest) (*WorkerResponse, error) {
	return m.response, m.err
}

// ---------------------------------------------------------------------------
// Additional Coverage Tests
// ---------------------------------------------------------------------------

func TestEngine_SetStore(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	// SetStore should not panic and should set the store field
	e.SetStore(nil) // Should handle nil
	if e.store != nil {
		t.Error("SetStore(nil) should set store to nil")
	}
}

func TestEngine_MaxResponseBytes(t *testing.T) {
	db := testDB(t)
	cfg := testCfg()
	cfg.MaxResponseBytes = 12345678
	e := NewEngine(cfg, db)
	defer e.Shutdown()

	got := e.MaxResponseBytes()
	if got != 12345678 {
		t.Errorf("MaxResponseBytes() = %d, want 12345678", got)
	}
}

func TestBuildEnvFromDB(t *testing.T) {
	db := testDB(t)
	siteID := "test-site-env"

	u := models.User{Email: "worker-env@test.local", PasswordHash: "hash"}
	if err := db.Create(&u).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	s := models.Site{ID: siteID, UserID: u.ID, SubdomainSlug: "env-site", Name: "Env Site"}
	if err := db.Create(&s).Error; err != nil {
		t.Fatalf("create site: %v", err)
	}

	if err := db.Create(&models.WorkerEnvVar{SiteID: siteID, Name: "PUBLIC_KEY", Value: "pk_test_123", Secret: false}).Error; err != nil {
		t.Fatalf("create env var PUBLIC_KEY: %v", err)
	}
	if err := db.Create(&models.WorkerEnvVar{SiteID: siteID, Name: "SECRET_KEY", Value: "sk_live_456", Secret: true}).Error; err != nil {
		t.Fatalf("create env var SECRET_KEY: %v", err)
	}
	if err := db.Create(&models.WorkerEnvVar{SiteID: siteID, Name: "API_URL", Value: "https://api.example.com", Secret: false}).Error; err != nil {
		t.Fatalf("create env var API_URL: %v", err)
	}

	if err := db.Create(&models.KVNamespace{ID: "ns1", SiteID: siteID, Name: "CACHE"}).Error; err != nil {
		t.Fatalf("create kv namespace CACHE: %v", err)
	}
	if err := db.Create(&models.KVNamespace{ID: "ns2", SiteID: siteID, Name: "STORE"}).Error; err != nil {
		t.Fatalf("create kv namespace STORE: %v", err)
	}

	if err := db.Create(&models.StorageBucket{SiteID: siteID, Name: "IMAGES", BucketName: siteID + "-images"}).Error; err != nil {
		t.Fatalf("create storage bucket IMAGES: %v", err)
	}

	// Build env from DB
	env := BuildEnvFromDB(db, siteID, nil)

	// Verify regular vars
	if env.Vars["PUBLIC_KEY"] != "pk_test_123" {
		t.Errorf("Vars[PUBLIC_KEY] = %q, want pk_test_123", env.Vars["PUBLIC_KEY"])
	}
	if env.Vars["API_URL"] != "https://api.example.com" {
		t.Errorf("Vars[API_URL] = %q", env.Vars["API_URL"])
	}
	if _, exists := env.Vars["SECRET_KEY"]; exists {
		t.Error("SECRET_KEY should not be in Vars (it's a secret)")
	}

	// Verify secrets
	if env.Secrets["SECRET_KEY"] != "sk_live_456" {
		t.Errorf("Secrets[SECRET_KEY] = %q, want sk_live_456", env.Secrets["SECRET_KEY"])
	}

	// Verify KV bindings
	if env.KVBindings["CACHE"] != "ns1" {
		t.Errorf("KVBindings[CACHE] = %q, want ns1", env.KVBindings["CACHE"])
	}
	if env.KVBindings["STORE"] != "ns2" {
		t.Errorf("KVBindings[STORE] = %q, want ns2", env.KVBindings["STORE"])
	}

	if env.StorageBindings["IMAGES"] != siteID+"-images" {
		t.Errorf("StorageBindings[IMAGES] = %q, want %q", env.StorageBindings["IMAGES"], siteID+"-images")
	}

	// Verify ASSETS is nil when not provided
	if env.Assets != nil {
		t.Error("Assets should be nil when not provided")
	}
}

func TestBuildEnvFromDB_WithAssets(t *testing.T) {
	db := testDB(t)
	siteID := "test-site-assets"

	mockFetcher := &mockAssetsFetcher{
		response: &WorkerResponse{StatusCode: 200, Body: []byte("test")},
	}

	env := BuildEnvFromDB(db, siteID, mockFetcher)

	if env.Assets == nil {
		t.Error("Assets should not be nil when fetcher is provided")
	}
	if env.Assets != mockFetcher {
		t.Error("Assets should be the provided fetcher")
	}
}

func TestEngine_EnsureBytecode_FromCache(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	siteID := "test-bytecode-cache"
	deployKey := "deploy1"

	source := `export default { fetch() { return new Response("ok"); } };`

	// Compile and cache first
	bytecode, err := e.CompileAndCache(siteID, deployKey, source)
	if err != nil {
		t.Fatalf("CompileAndCache: %v", err)
	}
	if len(bytecode) == 0 {
		t.Fatal("bytecode is empty")
	}

	// Clear the in-memory cache to simulate server restart
	key := poolKey{SiteID: siteID, DeployKey: deployKey}
	e.bytecodes.Delete(key)

	// Manually store it back (simulating it's in memory now)
	e.bytecodes.Store(key, bytecode)

	// EnsureBytecode should find it in cache and not error
	err = e.EnsureBytecode(siteID, deployKey)
	if err != nil {
		t.Errorf("EnsureBytecode (from cache): %v", err)
	}
}

func TestEngine_EnsureBytecode_NoStore(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	// Don't set a store (e.store == nil)
	siteID := "test-no-store"
	deployKey := "deploy1"

	// Clear bytecode cache
	key := poolKey{SiteID: siteID, DeployKey: deployKey}
	e.bytecodes.Delete(key)

	// EnsureBytecode should fail when there's no cached bytecode and no store
	err := e.EnsureBytecode(siteID, deployKey)
	if err == nil {
		t.Fatal("EnsureBytecode should fail when store is not set")
	}
	if err.Error() != "storage manager not set" {
		t.Errorf("unexpected error: %v", err)
	}
}
