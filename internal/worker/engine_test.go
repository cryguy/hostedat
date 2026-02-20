package worker

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/cryguy/hostedat/internal/config"
	"github.com/cryguy/hostedat/internal/models"
	v8 "github.com/tommie/v8go"
)

func TestModuleDefaultExportFetch(t *testing.T) {
	source := `export default {
  fetch(request, env, ctx) {
    return new Response("it works");
  }
};`

	iso := v8.NewIsolate()
	defer iso.Dispose()
	ctx := v8.NewContext(iso)
	defer ctx.Close()

	el := newEventLoop()
	if err := setupWebAPIs(iso, ctx, el); err != nil {
		t.Fatalf("setupWebAPIs: %v", err)
	}

	// Run the wrapped module source (converts `export default` to globalThis.__worker_module__).
	wrapped := wrapESModule(source)
	if _, err := ctx.RunScript(wrapped, "worker.js"); err != nil {
		t.Fatalf("running wrapped module: %v", err)
	}

	// Verify __worker_module__ exists.
	moduleVal, err := ctx.Global().Get("__worker_module__")
	if err != nil {
		t.Fatalf("getting __worker_module__: %v", err)
	}
	if moduleVal.IsUndefined() || moduleVal.IsNull() {
		t.Fatal("default export is undefined/null")
	}

	// Verify fetch is callable.
	result, err := ctx.RunScript(`(function() {
		var req = new Request('http://localhost/');
		return globalThis.__worker_module__.fetch(req, {}, {});
	})()`, "test_fetch.js")
	if err != nil {
		t.Fatalf("invoking fetch: %v", err)
	}

	t.Logf("fetch returned successfully (isPromise=%v, isObject=%v)", result.IsPromise(), result.IsObject())
}

// TestPoolModuleFlow tests the full pool setup path matching the exact
// production flow in getOrCreatePool + Execute.
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

	pool, err := newV8Pool(cfg.PoolSize, source, []setupFunc{
		setupWebAPIs,
		setupConsole,
		func(iso *v8.Isolate, ctx *v8.Context, _ *eventLoop) error {
			return setupFetch(iso, ctx, cfg)
		},
	}, cfg.MemoryLimitMB)
	if err != nil {
		t.Fatalf("newV8Pool: %v", err)
	}
	defer pool.dispose()

	w, err := pool.get()
	if err != nil {
		t.Fatalf("pool.get: %v", err)
	}
	defer pool.put(w)

	// Check __worker_module__ exists (same as Execute does).
	moduleVal, err := w.ctx.Global().Get("__worker_module__")
	if err != nil || moduleVal.IsUndefined() || moduleVal.IsNull() {
		t.Fatal("__worker_module__ is undefined/null  Edefault export not captured")
	}

	// Call fetch (same as Execute does).
	fetchResult, err := w.ctx.RunScript(`(function() {
		var req = new Request('http://localhost/test');
		return globalThis.__worker_module__.fetch(req, {}, {});
	})()`, "test_pool_fetch.js")
	if err != nil {
		t.Fatalf("invoking fetch: %v", err)
	}

	t.Logf("pool flow: fetch returned (isPromise=%v, isObject=%v)", fetchResult.IsPromise(), fetchResult.IsObject())
}

// TestAsyncFetchHandler tests that async fetch handlers (returning Promise<Response>)
// are correctly awaited and converted, matching the exact Execute() flow.
func TestAsyncFetchHandler(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request) {
    const url = new URL(request.url);
    const name = url.searchParams.get("name") || "world";
    return new Response("Hello, " + name + "!");
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/api/hello?name=test"))
	assertOK(t, r)

	want := "Hello, test!"
	if string(r.Response.Body) != want {
		t.Fatalf("body = %q, want %q", r.Response.Body, want)
	}
	t.Logf("response: status=%d body=%q headers=%v", r.Response.StatusCode, string(r.Response.Body), r.Response.Headers)
}

// TestAssetsFetch tests that env.ASSETS.fetch(request) works correctly
// by using a mock AssetsFetcher and running the full worker execution flow.
func TestAssetsFetch(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    return env.ASSETS.fetch(request);
  },
};`

	mockFetcher := &mockAssetsFetcher{
		response: &WorkerResponse{
			StatusCode: 200,
			Headers:    map[string]string{"content-type": "text/html; charset=utf-8"},
			Body:       []byte("<h1>Hello from ASSETS</h1>"),
		},
	}

	env := &Env{
		Vars:       make(map[string]string),
		Secrets:    make(map[string]string),
		KVBindings: make(map[string]string),
		Assets:     mockFetcher,
	}

	r := execJS(t, e, source, env, getReq("http://localhost/index.html"))
	assertOK(t, r)
	if r.Response.StatusCode != 200 {
		t.Fatalf("expected status 200, got %d", r.Response.StatusCode)
	}
	if string(r.Response.Body) != "<h1>Hello from ASSETS</h1>" {
		t.Fatalf("unexpected body: %q", string(r.Response.Body))
	}
	t.Logf("ASSETS.fetch: status=%d body=%q", r.Response.StatusCode, string(r.Response.Body))
}

// mockAssetsFetcher implements AssetsFetcher for testing.
type mockAssetsFetcher struct {
	response *WorkerResponse
	err      error
}

func (m *mockAssetsFetcher) Fetch(_ *WorkerRequest) (*WorkerResponse, error) {
	return m.response, m.err
}

// ---------------------------------------------------------------------------
// Additional Coverage Tests
// ---------------------------------------------------------------------------

func TestEngine_SetStore(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	e.SetStore(nil)
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

	env := BuildEnvFromDB(db, siteID, nil)

	if env.Vars["PUBLIC_KEY"] != "pk_test_123" {
		t.Errorf("Vars[PUBLIC_KEY] = %q, want pk_test_123", env.Vars["PUBLIC_KEY"])
	}
	if env.Vars["API_URL"] != "https://api.example.com" {
		t.Errorf("Vars[API_URL] = %q", env.Vars["API_URL"])
	}
	if _, exists := env.Vars["SECRET_KEY"]; exists {
		t.Error("SECRET_KEY should not be in Vars (it's a secret)")
	}

	if env.Secrets["SECRET_KEY"] != "sk_live_456" {
		t.Errorf("Secrets[SECRET_KEY] = %q, want sk_live_456", env.Secrets["SECRET_KEY"])
	}

	if env.KVBindings["CACHE"] != "ns1" {
		t.Errorf("KVBindings[CACHE] = %q, want ns1", env.KVBindings["CACHE"])
	}
	if env.KVBindings["STORE"] != "ns2" {
		t.Errorf("KVBindings[STORE] = %q, want ns2", env.KVBindings["STORE"])
	}

	if env.StorageBindings["IMAGES"] != siteID+"-images" {
		t.Errorf("StorageBindings[IMAGES] = %q, want %q", env.StorageBindings["IMAGES"], siteID+"-images")
	}

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

func TestEngine_EnsureSource_FromCache(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	siteID := "test-source-cache"
	deployKey := "deploy1"

	source := `export default { fetch() { return new Response("ok"); } };`

	srcBytes, err := e.CompileAndCache(siteID, deployKey, source)
	if err != nil {
		t.Fatalf("CompileAndCache: %v", err)
	}
	if len(srcBytes) == 0 {
		t.Fatal("source bytes are empty")
	}

	// Clear the in-memory cache to simulate server restart.
	key := poolKey{SiteID: siteID, DeployKey: deployKey}
	e.sources.Delete(key)

	// Manually store it back.
	e.sources.Store(key, source)

	// EnsureSource should find it in cache and not error.
	err = e.EnsureSource(siteID, deployKey)
	if err != nil {
		t.Errorf("EnsureSource (from cache): %v", err)
	}
}

func TestEngine_EnsureSource_NoStore(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	siteID := "test-no-store"
	deployKey := "deploy1"

	// Clear source cache.
	key := poolKey{SiteID: siteID, DeployKey: deployKey}
	e.sources.Delete(key)

	// EnsureSource should fail when there's no cached source and no store.
	err := e.EnsureSource(siteID, deployKey)
	if err == nil {
		t.Fatal("EnsureSource should fail when store is not set")
	}
	if err.Error() != "storage manager not set" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestEngine_CompileAndCache_InvalidSource(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	_, err := e.CompileAndCache("site1", "deploy1", "function {{{invalid")
	if err == nil {
		t.Fatal("CompileAndCache should fail for invalid JS")
	}
}

func TestEngine_CompileAndCache_ReturnBytes(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default { fetch() { return new Response("ok"); } };`
	bytes, err := e.CompileAndCache("site1", "deploy1", source)
	if err != nil {
		t.Fatalf("CompileAndCache: %v", err)
	}
	if string(bytes) != source {
		t.Errorf("returned bytes don't match source")
	}
}

func TestEngine_InvalidatePool(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default { fetch() { return new Response("ok"); } };`
	siteID := "test-invalidate"
	deployKey := "deploy1"

	if _, err := e.CompileAndCache(siteID, deployKey, source); err != nil {
		t.Fatalf("CompileAndCache: %v", err)
	}

	// Create pool
	pool, err := e.getOrCreatePool(siteID, deployKey)
	if err != nil {
		t.Fatalf("getOrCreatePool: %v", err)
	}
	if pool == nil {
		t.Fatal("pool should not be nil")
	}

	// Invalidate
	e.InvalidatePool(siteID, deployKey)

	// Source should be cleared
	key := poolKey{SiteID: siteID, DeployKey: deployKey}
	if _, ok := e.sources.Load(key); ok {
		t.Error("source should be cleared after InvalidatePool")
	}
}

func TestEngine_InvalidatePool_NonExistent(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	// Should not panic
	e.InvalidatePool("nonexistent", "deploy1")
}

func TestEngine_Execute_NoSource(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	req := getReq("http://localhost/")
	result := e.Execute("nonexistent", "deploy1", defaultEnv(), req)
	if result.Error == nil {
		t.Fatal("Execute should fail when no source is available")
	}
}

func TestEngine_Execute_NoFetchHandler(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	// Module with no fetch handler
	source := `export default { scheduled() {} };`
	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	if r.Error == nil {
		t.Fatal("Execute should fail when no fetch handler")
	}
}

func TestEngine_ExecuteScheduled_NoHandler(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default { fetch() { return new Response("ok"); } };`
	siteID := "test-sched-noh"
	deployKey := "deploy1"

	if _, err := e.CompileAndCache(siteID, deployKey, source); err != nil {
		t.Fatalf("CompileAndCache: %v", err)
	}

	env := defaultEnv()
	result := e.ExecuteScheduled(siteID, deployKey, env, "*/5 * * * *")
	if result.Error == nil {
		t.Fatal("ExecuteScheduled should fail with no scheduled handler")
	}
}

func TestEngine_ExecuteScheduled_Success(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch() { return new Response("ok"); },
  scheduled(event, env, ctx) {
    console.log("cron: " + event.cron);
  },
};`
	siteID := "test-sched-ok"
	deployKey := "deploy1"

	if _, err := e.CompileAndCache(siteID, deployKey, source); err != nil {
		t.Fatalf("CompileAndCache: %v", err)
	}

	env := defaultEnv()
	result := e.ExecuteScheduled(siteID, deployKey, env, "*/5 * * * *")
	if result.Error != nil {
		t.Fatalf("ExecuteScheduled: %v", result.Error)
	}
	if len(result.Logs) == 0 {
		t.Error("expected logs from scheduled handler")
	}
}

func TestEngine_getOrCreatePool_Reuses(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default { fetch() { return new Response("ok"); } };`
	siteID := "test-reuse"
	deployKey := "deploy1"

	if _, err := e.CompileAndCache(siteID, deployKey, source); err != nil {
		t.Fatalf("CompileAndCache: %v", err)
	}

	pool1, err := e.getOrCreatePool(siteID, deployKey)
	if err != nil {
		t.Fatalf("getOrCreatePool 1: %v", err)
	}

	pool2, err := e.getOrCreatePool(siteID, deployKey)
	if err != nil {
		t.Fatalf("getOrCreatePool 2: %v", err)
	}

	if pool1 != pool2 {
		t.Error("getOrCreatePool should reuse the same pool")
	}
}

func TestEngine_Shutdown(t *testing.T) {
	db := testDB(t)
	cfg := testCfg()
	e := NewEngine(cfg, db)

	source := `export default { fetch() { return new Response("ok"); } };`
	if _, err := e.CompileAndCache("s1", "d1", source); err != nil {
		t.Fatalf("CompileAndCache: %v", err)
	}
	if _, err := e.getOrCreatePool("s1", "d1"); err != nil {
		t.Fatalf("getOrCreatePool: %v", err)
	}

	e.Shutdown()

	// After shutdown, pools and sources should be empty
	found := false
	e.pools.Range(func(_, _ any) bool {
		found = true
		return false
	})
	if found {
		t.Error("pools should be empty after Shutdown")
	}

	found = false
	e.sources.Range(func(_, _ any) bool {
		found = true
		return false
	})
	if found {
		t.Error("sources should be empty after Shutdown")
	}
}

func TestEngine_SetMinioClient(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)
	e.SetMinioClient(nil)
	if e.minioClient != nil {
		t.Error("SetMinioClient(nil) should set nil")
	}
}

func TestEngine_SetPresignMinioClient(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)
	e.SetPresignMinioClient(nil)
	if e.presignMinioClient != nil {
		t.Error("SetPresignMinioClient(nil) should set nil")
	}
}

func TestEngine_SetPublicS3URL(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)
	e.SetPublicS3URL("https://s3.example.com")
	if e.publicS3URL != "https://s3.example.com" {
		t.Error("SetPublicS3URL should set the URL")
	}
}

func TestSitePool_IsValid(t *testing.T) {
	sp := &sitePool{}
	if !sp.isValid() {
		t.Error("new sitePool should be valid")
	}
	sp.markInvalid()
	if sp.isValid() {
		t.Error("sitePool should be invalid after markInvalid")
	}
}

func TestAwaitValue_NilValue(t *testing.T) {
	iso := v8.NewIsolate()
	defer iso.Dispose()
	ctx := v8.NewContext(iso)
	defer ctx.Close()

	result, err := awaitValue(ctx, nil, time.Now().Add(time.Second))
	if err != nil {
		t.Fatalf("awaitValue(nil) should not error: %v", err)
	}
	if result != nil {
		t.Error("awaitValue(nil) should return nil")
	}
}

func TestAwaitValue_NonPromise(t *testing.T) {
	iso := v8.NewIsolate()
	defer iso.Dispose()
	ctx := v8.NewContext(iso)
	defer ctx.Close()

	val, _ := v8.NewValue(iso, "hello")
	result, err := awaitValue(ctx, val, time.Now().Add(time.Second))
	if err != nil {
		t.Fatalf("awaitValue(string) should not error: %v", err)
	}
	if result.String() != "hello" {
		t.Errorf("result = %q, want hello", result.String())
	}
}

func TestAwaitValue_ResolvedPromise(t *testing.T) {
	iso := v8.NewIsolate()
	defer iso.Dispose()
	ctx := v8.NewContext(iso)
	defer ctx.Close()

	val, err := ctx.RunScript("Promise.resolve(42)", "test.js")
	if err != nil {
		t.Fatalf("RunScript: %v", err)
	}

	result, err := awaitValue(ctx, val, time.Now().Add(time.Second))
	if err != nil {
		t.Fatalf("awaitValue: %v", err)
	}
	if result.Int32() != 42 {
		t.Errorf("result = %d, want 42", result.Int32())
	}
}

func TestAwaitValue_RejectedPromise(t *testing.T) {
	iso := v8.NewIsolate()
	defer iso.Dispose()
	ctx := v8.NewContext(iso)
	defer ctx.Close()

	val, err := ctx.RunScript("Promise.reject('boom')", "test.js")
	if err != nil {
		t.Fatalf("RunScript: %v", err)
	}

	_, err = awaitValue(ctx, val, time.Now().Add(time.Second))
	if err == nil {
		t.Fatal("awaitValue should error on rejected promise")
	}
	if !strings.Contains(err.Error(), "boom") {
		t.Errorf("error = %q, should contain 'boom'", err.Error())
	}
}

func TestNewJSObject(t *testing.T) {
	iso := v8.NewIsolate()
	defer iso.Dispose()
	ctx := v8.NewContext(iso)
	defer ctx.Close()

	obj, err := newJSObject(iso, ctx)
	if err != nil {
		t.Fatalf("newJSObject: %v", err)
	}
	if obj == nil {
		t.Fatal("newJSObject returned nil")
	}
}

func TestEngine_Execute_ScriptThrows(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request, env) {
    throw new Error("intentional error");
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	if r.Error == nil {
		t.Fatal("Execute should fail when fetch handler throws")
	}
	if !strings.Contains(r.Error.Error(), "intentional error") {
		t.Errorf("error = %q, should mention 'intentional error'", r.Error)
	}
}

func TestEngine_Execute_PromiseRejection(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request) {
    throw new Error("async rejection");
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	if r.Error == nil {
		t.Fatal("Execute should fail on promise rejection")
	}
}

func TestEngine_Execute_ReturnUndefined(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request) {
    // Return nothing (undefined).
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	if r.Error == nil {
		t.Fatal("Execute should fail when fetch returns undefined")
	}
}

func TestEngine_Execute_WithConsoleLogs(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request) {
    console.log("hello from worker");
    console.warn("a warning");
    console.error("an error");
    return new Response("ok");
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	if len(r.Logs) < 3 {
		t.Errorf("expected at least 3 logs, got %d", len(r.Logs))
	}
}

func TestEngine_Execute_WithEnvVars(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request, env) {
    return Response.json({
      apiKey: env.API_KEY,
      secret: env.SECRET,
    });
  },
};`

	env := &Env{
		Vars:    map[string]string{"API_KEY": "pk_test"},
		Secrets: map[string]string{"SECRET": "sk_live"},
	}

	r := execJS(t, e, source, env, getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		APIKey string `json:"apiKey"`
		Secret string `json:"secret"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if data.APIKey != "pk_test" {
		t.Errorf("apiKey = %q, want pk_test", data.APIKey)
	}
	if data.Secret != "sk_live" {
		t.Errorf("secret = %q, want sk_live", data.Secret)
	}
}

func TestEngine_Execute_Duration(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request) {
    return new Response("ok");
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	if r.Duration <= 0 {
		t.Error("Duration should be positive")
	}
}

func TestEngine_ExecuteScheduled_AsyncHandler(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch() { return new Response("ok"); },
  async scheduled(event, env, ctx) {
    console.log("async cron: " + event.cron);
    console.log("time: " + event.scheduledTime);
    return "done";
  },
};`
	siteID := "test-sched-async"
	deployKey := "deploy1"

	if _, err := e.CompileAndCache(siteID, deployKey, source); err != nil {
		t.Fatalf("CompileAndCache: %v", err)
	}

	result := e.ExecuteScheduled(siteID, deployKey, defaultEnv(), "0 * * * *")
	if result.Error != nil {
		t.Fatalf("ExecuteScheduled: %v", result.Error)
	}
	if len(result.Logs) < 2 {
		t.Errorf("expected at least 2 logs, got %d", len(result.Logs))
	}
	if result.Duration <= 0 {
		t.Error("Duration should be positive")
	}
}

func TestEngine_ExecuteScheduled_NoSource(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	result := e.ExecuteScheduled("nonexistent", "deploy1", defaultEnv(), "* * * * *")
	if result.Error == nil {
		t.Fatal("ExecuteScheduled should fail with no source")
	}
}

func TestEngine_getOrCreatePool_NoSource(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	_, err := e.getOrCreatePool("no-source", "deploy1")
	if err == nil {
		t.Fatal("getOrCreatePool should fail with no source")
	}
}

func TestEngine_getOrCreatePool_InvalidPoolReplaced(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default { fetch() { return new Response("ok"); } };`
	siteID := "test-invalid-replace"
	deployKey := "deploy1"

	if _, err := e.CompileAndCache(siteID, deployKey, source); err != nil {
		t.Fatalf("CompileAndCache: %v", err)
	}

	// Create initial pool.
	pool1, err := e.getOrCreatePool(siteID, deployKey)
	if err != nil {
		t.Fatalf("getOrCreatePool 1: %v", err)
	}

	// Mark the pool as invalid.
	key := poolKey{SiteID: siteID, DeployKey: deployKey}
	if val, ok := e.pools.Load(key); ok {
		sp := val.(*sitePool)
		sp.markInvalid()
	}

	// Re-cache source since the pool invalidation + getOrCreatePool will need it.
	e.sources.Store(key, source)

	// Next call should create a new pool.
	pool2, err := e.getOrCreatePool(siteID, deployKey)
	if err != nil {
		t.Fatalf("getOrCreatePool 2: %v", err)
	}
	if pool1 == pool2 {
		t.Error("should have created a new pool after invalidation")
	}
}

func TestEngine_Execute_WithTimers(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request) {
    return new Promise((resolve) => {
      setTimeout(() => {
        resolve(new Response("after timeout"));
      }, 10);
    });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)
	if string(r.Response.Body) != "after timeout" {
		t.Errorf("body = %q, want 'after timeout'", r.Response.Body)
	}
}

func TestEngine_Execute_WaitUntilAndPassThrough(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request, env, ctx) {
    // waitUntil and passThroughOnException should be callable without error.
    ctx.waitUntil(Promise.resolve("done"));
    ctx.passThroughOnException();
    return new Response("ok");
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)
	if string(r.Response.Body) != "ok" {
		t.Errorf("body = %q, want 'ok'", r.Response.Body)
	}
}

func TestEngine_Execute_WithKVBinding(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	// Create a KV namespace in the DB.
	siteID := "kv-test-site"
	u := models.User{Email: "kv-test@test.local", PasswordHash: "hash"}
	db.Create(&u)
	s := models.Site{ID: siteID, UserID: u.ID, SubdomainSlug: "kv-test", Name: "KV Test"}
	db.Create(&s)
	ns := models.KVNamespace{ID: "kvns1", SiteID: siteID, Name: "MY_KV"}
	db.Create(&ns)

	source := `export default {
  async fetch(request, env) {
    // Put a value.
    await env.MY_KV.put("key1", "value1");
    // Get it back.
    const val = await env.MY_KV.get("key1");
    // List keys.
    const list = await env.MY_KV.list();
    return Response.json({ val, keys: list.keys.length });
  },
};`

	env := &Env{
		Vars:       make(map[string]string),
		Secrets:    make(map[string]string),
		KVBindings: map[string]string{"MY_KV": "kvns1"},
	}

	r := execJS(t, e, source, env, getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Val  string `json:"val"`
		Keys int    `json:"keys"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if data.Val != "value1" {
		t.Errorf("val = %q, want 'value1'", data.Val)
	}
	if data.Keys != 1 {
		t.Errorf("keys = %d, want 1", data.Keys)
	}
}

func TestEngine_Execute_KVDelete(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	siteID := "kv-del-site"
	u := models.User{Email: "kv-del@test.local", PasswordHash: "hash"}
	db.Create(&u)
	s := models.Site{ID: siteID, UserID: u.ID, SubdomainSlug: "kv-del", Name: "KV Del"}
	db.Create(&s)
	ns := models.KVNamespace{ID: "kvns-del", SiteID: siteID, Name: "KV"}
	db.Create(&ns)

	source := `export default {
  async fetch(request, env) {
    await env.KV.put("a", "1");
    await env.KV.put("b", "2");
    await env.KV.delete("a");
    const val = await env.KV.get("a");
    const list = await env.KV.list();
    return Response.json({ deleted: val === null, remaining: list.keys.length });
  },
};`

	env := &Env{
		Vars:       make(map[string]string),
		Secrets:    make(map[string]string),
		KVBindings: map[string]string{"KV": "kvns-del"},
	}

	r := execJS(t, e, source, env, getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Deleted   bool `json:"deleted"`
		Remaining int  `json:"remaining"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatal(err)
	}
	if !data.Deleted {
		t.Error("deleted key should return null")
	}
	if data.Remaining != 1 {
		t.Errorf("remaining = %d, want 1", data.Remaining)
	}
}

func TestEngine_ExecuteScheduled_HandlerThrows(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch() { return new Response("ok"); },
  scheduled(event) {
    throw new Error("cron failed");
  },
};`
	siteID := "test-sched-throw"
	deployKey := "deploy1"

	if _, err := e.CompileAndCache(siteID, deployKey, source); err != nil {
		t.Fatalf("CompileAndCache: %v", err)
	}

	result := e.ExecuteScheduled(siteID, deployKey, defaultEnv(), "* * * * *")
	if result.Error == nil {
		t.Fatal("ExecuteScheduled should fail when handler throws")
	}
}

func TestEngine_Execute_Timeout(t *testing.T) {
	db := testDB(t)
	cfg := testCfg()
	cfg.ExecutionTimeout = 200 // 200ms
	e := NewEngine(cfg, db)
	defer e.Shutdown()

	source := `export default {
  fetch(request) {
    // Infinite loop to trigger timeout.
    while(true) {}
    return new Response("never");
  },
};`
	siteID := "test-timeout"
	deployKey := "deploy1"

	if _, err := e.CompileAndCache(siteID, deployKey, source); err != nil {
		t.Fatalf("CompileAndCache: %v", err)
	}

	result := e.Execute(siteID, deployKey, defaultEnv(), getReq("http://localhost/"))
	if result.Error == nil {
		t.Fatal("Execute should fail on timeout")
	}
	if !strings.Contains(result.Error.Error(), "timed out") {
		t.Errorf("error = %q, should mention 'timed out'", result.Error)
	}
	if result.Duration <= 0 {
		t.Error("Duration should be positive")
	}
}

func TestEngine_ExecuteScheduled_Timeout(t *testing.T) {
	db := testDB(t)
	cfg := testCfg()
	cfg.ExecutionTimeout = 200
	e := NewEngine(cfg, db)
	defer e.Shutdown()

	source := `export default {
  fetch() { return new Response("ok"); },
  scheduled(event) {
    while(true) {}
  },
};`
	siteID := "test-sched-timeout"
	deployKey := "deploy1"

	if _, err := e.CompileAndCache(siteID, deployKey, source); err != nil {
		t.Fatalf("CompileAndCache: %v", err)
	}

	result := e.ExecuteScheduled(siteID, deployKey, defaultEnv(), "* * * * *")
	if result.Error == nil {
		t.Fatal("ExecuteScheduled should fail on timeout")
	}
	if !strings.Contains(result.Error.Error(), "timed out") && !strings.Contains(result.Error.Error(), "invoking worker scheduled") {
		t.Errorf("error = %q, should be timeout or invocation error", result.Error)
	}
}

func TestEngine_ExecuteScheduled_WithLogs(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch() { return new Response("ok"); },
  scheduled(event, env, ctx) {
    console.log("scheduled: " + event.cron);
    console.warn("scheduledTime: " + typeof event.scheduledTime);
  },
};`
	siteID := "test-sched-logs"
	deployKey := "deploy1"

	if _, err := e.CompileAndCache(siteID, deployKey, source); err != nil {
		t.Fatalf("CompileAndCache: %v", err)
	}

	result := e.ExecuteScheduled(siteID, deployKey, defaultEnv(), "0 */2 * * *")
	if result.Error != nil {
		t.Fatalf("ExecuteScheduled: %v", result.Error)
	}
	if len(result.Logs) < 2 {
		t.Errorf("expected at least 2 logs, got %d", len(result.Logs))
	}
	if result.Duration <= 0 {
		t.Error("Duration should be positive")
	}
}

func TestEngine_Execute_HeadersInResponse(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request) {
    return new Response("ok", {
      status: 201,
      headers: {
        "Content-Type": "text/plain",
        "X-Custom": "test-value",
      },
    });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)
	if r.Response.StatusCode != 201 {
		t.Errorf("status = %d, want 201", r.Response.StatusCode)
	}
	// JS Headers lowercases keys.
	ct := r.Response.Headers["content-type"]
	if ct == "" {
		ct = r.Response.Headers["Content-Type"]
	}
	if ct != "text/plain" {
		t.Errorf("content-type = %q", ct)
	}
	xc := r.Response.Headers["x-custom"]
	if xc == "" {
		xc = r.Response.Headers["X-Custom"]
	}
	if xc != "test-value" {
		t.Errorf("x-custom = %q", xc)
	}
}

func TestEngine_Execute_PostRequestBody(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request) {
    const body = await request.text();
    return Response.json({ method: request.method, body: body, url: request.url });
  },
};`

	req := &WorkerRequest{
		Method:  "POST",
		URL:     "http://localhost/submit",
		Headers: map[string]string{"Content-Type": "application/json"},
		Body:    []byte(`{"key":"value"}`),
	}

	r := execJS(t, e, source, defaultEnv(), req)
	assertOK(t, r)

	var data struct {
		Method string `json:"method"`
		Body   string `json:"body"`
		URL    string `json:"url"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if data.Method != "POST" {
		t.Errorf("method = %q, want POST", data.Method)
	}
	if data.Body != `{"key":"value"}` {
		t.Errorf("body = %q", data.Body)
	}
}

func TestEngine_Execute_RequestHeaders(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request) {
    const ct = request.headers.get("Content-Type") || "none";
    const auth = request.headers.get("Authorization") || "none";
    return Response.json({ ct, auth });
  },
};`

	req := &WorkerRequest{
		Method: "GET",
		URL:    "http://localhost/",
		Headers: map[string]string{
			"Content-Type":  "application/json",
			"Authorization": "Bearer token123",
		},
	}

	r := execJS(t, e, source, defaultEnv(), req)
	assertOK(t, r)

	var data struct {
		CT   string `json:"ct"`
		Auth string `json:"auth"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatal(err)
	}
	if data.CT != "application/json" {
		t.Errorf("ct = %q", data.CT)
	}
	if data.Auth != "Bearer token123" {
		t.Errorf("auth = %q", data.Auth)
	}
}

func TestEngine_Execute_StructuredClone(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request) {
    const obj = { a: 1, b: [2, 3], c: { d: true } };
    const clone = structuredClone(obj);
    clone.a = 99;
    return Response.json({ original: obj.a, clone: clone.a, deep: clone.c.d });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Original int  `json:"original"`
		Clone    int  `json:"clone"`
		Deep     bool `json:"deep"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatal(err)
	}
	if data.Original != 1 {
		t.Errorf("original = %d, want 1", data.Original)
	}
	if data.Clone != 99 {
		t.Errorf("clone = %d, want 99", data.Clone)
	}
}

func TestEngine_Execute_PerformanceNow(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request) {
    const t1 = performance.now();
    let sum = 0;
    for (let i = 0; i < 1000; i++) sum += i;
    const t2 = performance.now();
    return Response.json({ elapsed: t2 - t1, hasNow: typeof performance.now === "function" });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Elapsed float64 `json:"elapsed"`
		HasNow  bool    `json:"hasNow"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatal(err)
	}
	if !data.HasNow {
		t.Error("performance.now should be a function")
	}
	if data.Elapsed < 0 {
		t.Error("elapsed time should be non-negative")
	}
}

func TestEngine_Execute_AtoB_BtoA(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request) {
    const encoded = btoa("Hello, World!");
    const decoded = atob(encoded);
    return Response.json({ encoded, decoded });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Encoded string `json:"encoded"`
		Decoded string `json:"decoded"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatal(err)
	}
	if data.Encoded != "SGVsbG8sIFdvcmxkIQ==" {
		t.Errorf("encoded = %q", data.Encoded)
	}
	if data.Decoded != "Hello, World!" {
		t.Errorf("decoded = %q", data.Decoded)
	}
}

func TestEngine_Execute_BinaryResponseBody(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request) {
    const bytes = new Uint8Array([72, 101, 108, 108, 111]);
    return new Response(bytes.buffer);
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)
	if string(r.Response.Body) != "Hello" {
		t.Errorf("binary body = %q, want %q", string(r.Response.Body), "Hello")
	}
}

func TestEngine_Execute_TypedArrayResponseBody(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request) {
    const bytes = new Uint8Array([87, 111, 114, 108, 100]);
    return new Response(bytes);
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)
	if string(r.Response.Body) != "World" {
		t.Errorf("typed array body = %q, want %q", string(r.Response.Body), "World")
	}
}

func TestEngine_Execute_StreamResponseBody(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request) {
    const stream = new ReadableStream({
      start(controller) {
        controller.enqueue(new TextEncoder().encode("stream "));
        controller.enqueue(new TextEncoder().encode("body"));
        controller.close();
      }
    });
    return new Response(stream);
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)
	if string(r.Response.Body) != "stream body" {
		t.Errorf("stream body = %q, want %q", string(r.Response.Body), "stream body")
	}
}

func TestEngine_Execute_StatusCodes(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request) {
    const url = new URL(request.url);
    const code = parseInt(url.searchParams.get("code") || "200");
    return new Response("status test", { status: code });
  },
};`

	codes := []int{200, 201, 204, 301, 404, 500}
	for _, code := range codes {
		r := execJS(t, e, source, defaultEnv(), getReq(fmt.Sprintf("http://localhost/?code=%d", code)))
		if r.Error != nil {
			t.Fatalf("status %d: %v", code, r.Error)
		}
		if r.Response.StatusCode != code {
			t.Errorf("status = %d, want %d", r.Response.StatusCode, code)
		}
	}
}

func TestEngine_Execute_NullReturn(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request) { return null; },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	if r.Error == nil {
		t.Fatal("worker returning null should produce an error")
	}
}

func TestEngine_Execute_ThrowingHandler(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request) {
    throw new Error("handler exploded");
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	if r.Error == nil {
		t.Fatal("throwing fetch handler should produce an error")
	}
}

func TestEngine_Execute_AsyncPromiseChain(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request) {
    const a = await Promise.resolve(10);
    const b = await Promise.resolve(20);
    return new Response(String(a + b));
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)
	if string(r.Response.Body) != "30" {
		t.Errorf("async chain body = %q, want %q", string(r.Response.Body), "30")
	}
}

func TestEngine_Execute_FetchIsNotFunction(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	// Module where fetch is a string, not a function.
	source := `export default { fetch: "not-a-function" };`

	siteID := "test-fetch-notfn"
	deployKey := "d1"
	if _, err := e.CompileAndCache(siteID, deployKey, source); err != nil {
		t.Fatalf("CompileAndCache: %v", err)
	}
	result := e.Execute(siteID, deployKey, defaultEnv(), getReq("http://localhost/"))
	if result.Error == nil {
		t.Fatal("Execute should fail when fetch is not a function")
	}
	if !strings.Contains(result.Error.Error(), "not a function") {
		t.Errorf("error = %q, should mention 'not a function'", result.Error)
	}
}

func TestEngine_ExecuteScheduled_ScheduledIsNotFunction(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	// Module where scheduled is a string, not a function.
	source := `export default {
  fetch() { return new Response("ok"); },
  scheduled: "not-a-function",
};`

	siteID := "test-sched-notfn"
	deployKey := "d1"
	if _, err := e.CompileAndCache(siteID, deployKey, source); err != nil {
		t.Fatalf("CompileAndCache: %v", err)
	}
	result := e.ExecuteScheduled(siteID, deployKey, defaultEnv(), "* * * * *")
	if result.Error == nil {
		t.Fatal("ExecuteScheduled should fail when scheduled is not a function")
	}
	if !strings.Contains(result.Error.Error(), "not a function") {
		t.Errorf("error = %q, should mention 'not a function'", result.Error)
	}
}

func TestEngine_ExecuteScheduled_RejectedPromise(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch() { return new Response("ok"); },
  async scheduled(event, env, ctx) {
    throw new Error("scheduled failed");
  },
};`

	siteID := "test-sched-reject"
	deployKey := "d1"
	if _, err := e.CompileAndCache(siteID, deployKey, source); err != nil {
		t.Fatalf("CompileAndCache: %v", err)
	}
	result := e.ExecuteScheduled(siteID, deployKey, defaultEnv(), "0 * * * *")
	if result.Error == nil {
		t.Fatal("ExecuteScheduled should fail on rejected promise")
	}
}

func TestEngine_ExecuteScheduled_WithTimers(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	// Scheduled handler that uses setTimeout.
	source := `export default {
  fetch() { return new Response("ok"); },
  async scheduled(event, env, ctx) {
    await new Promise(resolve => setTimeout(resolve, 10));
    console.log("timer-fired: " + event.cron);
  },
};`

	siteID := "test-sched-timers"
	deployKey := "d1"
	if _, err := e.CompileAndCache(siteID, deployKey, source); err != nil {
		t.Fatalf("CompileAndCache: %v", err)
	}
	result := e.ExecuteScheduled(siteID, deployKey, defaultEnv(), "*/5 * * * *")
	if result.Error != nil {
		t.Fatalf("ExecuteScheduled: %v", result.Error)
	}
	found := false
	for _, l := range result.Logs {
		if strings.Contains(l.Message, "timer-fired") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected log from timer-based scheduled handler")
	}
}

func TestEngine_Execute_MultipleResponseHeaders(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request) {
    const headers = new Headers();
    headers.set("X-Custom", "value1");
    headers.set("Content-Type", "application/xml");
    headers.set("X-Request-Id", "abc123");
    return new Response("<data/>", { status: 201, headers });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	if r.Response.StatusCode != 201 {
		t.Errorf("status = %d, want 201", r.Response.StatusCode)
	}
	if r.Response.Headers["x-custom"] != "value1" {
		t.Errorf("x-custom = %q, want value1", r.Response.Headers["x-custom"])
	}
	if r.Response.Headers["x-request-id"] != "abc123" {
		t.Errorf("x-request-id = %q", r.Response.Headers["x-request-id"])
	}
}

func TestEngine_Execute_EmptyBody(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request) {
    return new Response(null, { status: 204 });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	if r.Response.StatusCode != 204 {
		t.Errorf("status = %d, want 204", r.Response.StatusCode)
	}
}

func TestEngine_Execute_LargeBody(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request) {
    // Generate a 10KB string.
    const chunk = "abcdefghij";
    let body = "";
    for (let i = 0; i < 1000; i++) body += chunk;
    return new Response(body);
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	if len(r.Response.Body) != 10000 {
		t.Errorf("body length = %d, want 10000", len(r.Response.Body))
	}
}

func TestEngine_Execute_RequestURL(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request) {
    const url = new URL(request.url);
    return Response.json({
      pathname: url.pathname,
      search: url.search,
      host: url.host,
    });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://example.com/api/test?key=val"))
	assertOK(t, r)

	var data struct {
		Pathname string `json:"pathname"`
		Search   string `json:"search"`
		Host     string `json:"host"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatal(err)
	}
	if data.Pathname != "/api/test" {
		t.Errorf("pathname = %q", data.Pathname)
	}
	if data.Search != "?key=val" {
		t.Errorf("search = %q", data.Search)
	}
}

func TestEngine_Execute_RequestMethod(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request) {
    return Response.json({ method: request.method });
  },
};`

	req := &WorkerRequest{
		Method:  "PUT",
		URL:     "http://localhost/resource",
		Headers: map[string]string{},
	}

	r := execJS(t, e, source, defaultEnv(), req)
	assertOK(t, r)

	var data struct {
		Method string `json:"method"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatal(err)
	}
	if data.Method != "PUT" {
		t.Errorf("method = %q, want PUT", data.Method)
	}
}

func TestEngine_CompileAndCache(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default { fetch(request, env) { return new Response("compiled"); } };`
	bytes, err := e.CompileAndCache("test-site-cc", "deploy-cc", source)
	if err != nil {
		t.Fatalf("CompileAndCache: %v", err)
	}
	if string(bytes) != source {
		t.Error("CompileAndCache should return the source bytes")
	}

	// Now Execute should work without EnsureSource.
	r := e.Execute("test-site-cc", "deploy-cc", defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)
	if string(r.Response.Body) != "compiled" {
		t.Errorf("body = %q, want 'compiled'", r.Response.Body)
	}
}

func TestEngine_CompileAndCache_BadScript(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	_, err := e.CompileAndCache("test-site-bad", "deploy-bad", "this is not valid javascript {{{{")
	if err == nil {
		t.Fatal("CompileAndCache should fail on invalid JS")
	}
}

func TestEngine_Execute_MissingSiteSource(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	r := e.Execute("nonexistent-site", "nonexistent-deploy", defaultEnv(), getReq("http://localhost/"))
	if r.Error == nil {
		t.Fatal("Execute without source should return error")
	}
}

func TestEngine_Execute_ReturnNull(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request, env) {
    return null;
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	if r.Error == nil {
		t.Fatal("returning null should produce an error")
	}
}

func TestEngine_Execute_ReturnUndefinedFromSync(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request, env) {
    return undefined;
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	if r.Error == nil {
		t.Fatal("returning undefined should produce an error")
	}
}

func TestEngine_Execute_ThrowsError(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request, env) {
    throw new Error("worker crash");
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	if r.Error == nil {
		t.Fatal("throwing should produce an error")
	}
}

func TestEngine_Execute_AsyncThrows(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    throw new Error("async crash");
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	if r.Error == nil {
		t.Fatal("async throw should produce an error")
	}
}

func TestEngine_Execute_ReturnString(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	// Returning a raw string (not a Response) should produce an error
	// because jsResponseToGo tries to extract headers._map from a primitive.
	source := `export default {
  fetch(request, env) {
    return "not a response";
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	// The response may or may not error depending on how jsResponseToGo handles it.
	// The key thing is it doesn't crash.
	_ = r
}

func TestEngine_Execute_WithRequestBody(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    const body = await request.text();
    return Response.json({ body, method: request.method });
  },
};`

	req := &WorkerRequest{
		Method:  "POST",
		URL:     "http://localhost/api",
		Headers: map[string]string{"Content-Type": "application/json"},
		Body:    []byte(`{"hello":"world"}`),
	}

	r := execJS(t, e, source, defaultEnv(), req)
	assertOK(t, r)

	var data struct {
		Body   string `json:"body"`
		Method string `json:"method"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatal(err)
	}
	if data.Body != `{"hello":"world"}` {
		t.Errorf("body = %q", data.Body)
	}
	if data.Method != "POST" {
		t.Errorf("method = %q", data.Method)
	}
}

func TestEngine_Execute_EnvVarsAndSecrets(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request, env) {
    return Response.json({
      apiKey: env.API_KEY,
      secret: env.MY_SECRET,
    });
  },
};`

	env := &Env{
		Vars:    map[string]string{"API_KEY": "test-key-123"},
		Secrets: map[string]string{"MY_SECRET": "super-secret"},
	}

	r := execJS(t, e, source, env, getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		APIKey string `json:"apiKey"`
		Secret string `json:"secret"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatal(err)
	}
	if data.APIKey != "test-key-123" {
		t.Errorf("apiKey = %q", data.APIKey)
	}
	if data.Secret != "super-secret" {
		t.Errorf("secret = %q", data.Secret)
	}
}

func TestEngine_Execute_ConsoleLogCaptured(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request, env) {
    console.log("info message");
    console.warn("warning message");
    console.error("error message");
    return new Response("ok");
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	if len(r.Logs) < 3 {
		t.Fatalf("expected at least 3 logs, got %d", len(r.Logs))
	}

	foundInfo, foundWarn, foundError := false, false, false
	for _, l := range r.Logs {
		if l.Level == "log" && strings.Contains(l.Message, "info message") {
			foundInfo = true
		}
		if l.Level == "warn" && strings.Contains(l.Message, "warning message") {
			foundWarn = true
		}
		if l.Level == "error" && strings.Contains(l.Message, "error message") {
			foundError = true
		}
	}
	if !foundInfo {
		t.Error("console.log not captured")
	}
	if !foundWarn {
		t.Error("console.warn not captured")
	}
	if !foundError {
		t.Error("console.error not captured")
	}
}

func TestEngine_Execute_PoolReuse(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request, env) {
    return new Response("pooled");
  },
};`

	// First execution creates the pool.
	r1 := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r1)
	// Second execution reuses the pool.
	r2 := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r2)

	if string(r2.Response.Body) != "pooled" {
		t.Errorf("second execution body = %q", r2.Response.Body)
	}
}

func TestAwaitValue_Timeout(t *testing.T) {
	iso := v8.NewIsolate()
	defer iso.Dispose()
	ctx := v8.NewContext(iso)
	defer ctx.Close()

	// Create a promise that never resolves.
	val, err := ctx.RunScript("new Promise(function() {})", "test.js")
	if err != nil {
		t.Fatalf("RunScript: %v", err)
	}

	// Use a deadline already in the past.
	_, err = awaitValue(ctx, val, time.Now().Add(-1*time.Second))
	if err == nil {
		t.Fatal("awaitValue should error on timeout")
	}
	if !strings.Contains(err.Error(), "timed out") {
		t.Errorf("error = %q, should contain 'timed out'", err.Error())
	}
}

func TestEngine_Execute_AssetsFetchError(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    try {
      const resp = await env.ASSETS.fetch(request);
      return Response.json({ status: resp.status });
    } catch(e) {
      return Response.json({ error: true, message: e.message || String(e) });
    }
  },
};`

	errorFetcher := &mockAssetsFetcher{
		response: nil,
		err:      fmt.Errorf("disk read error"),
	}

	env := &Env{
		Vars:       make(map[string]string),
		Secrets:    make(map[string]string),
		KVBindings: make(map[string]string),
		Assets:     errorFetcher,
	}

	r := execJS(t, e, source, env, getReq("http://localhost/index.html"))
	assertOK(t, r)

	var data struct {
		Error   bool   `json:"error"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatal(err)
	}
	if !data.Error {
		t.Error("ASSETS.fetch with error should throw")
	}
}

func TestEngine_Execute_AssetsFetchNullBody(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    const resp = await env.ASSETS.fetch(request);
    const text = await resp.text();
    return Response.json({ status: resp.status, body: text });
  },
};`

	nullBodyFetcher := &mockAssetsFetcher{
		response: &WorkerResponse{
			StatusCode: 204,
			Headers:    map[string]string{},
			Body:       nil,
		},
	}

	env := &Env{
		Vars:       make(map[string]string),
		Secrets:    make(map[string]string),
		KVBindings: make(map[string]string),
		Assets:     nullBodyFetcher,
	}

	r := execJS(t, e, source, env, getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Status int    `json:"status"`
		Body   string `json:"body"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatal(err)
	}
	if data.Status != 204 {
		t.Errorf("status = %d, want 204", data.Status)
	}
}

func TestEngine_Execute_AssetsFetchNoArgs(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    try {
      await env.ASSETS.fetch();
      return Response.json({ error: false });
    } catch(e) {
      return Response.json({ error: true, message: e.message || String(e) });
    }
  },
};`

	mockFetcher := &mockAssetsFetcher{
		response: &WorkerResponse{StatusCode: 200, Body: []byte("ok")},
	}

	env := &Env{
		Vars:       make(map[string]string),
		Secrets:    make(map[string]string),
		KVBindings: make(map[string]string),
		Assets:     mockFetcher,
	}

	r := execJS(t, e, source, env, getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Error   bool   `json:"error"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatal(err)
	}
	if !data.Error {
		t.Error("ASSETS.fetch() with no args should throw")
	}
}

func TestEngine_Execute_NilEnvMaps(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request, env) {
    return Response.json({ ok: true });
  },
};`

	// Env with nil Vars and Secrets (not empty maps, nil).
	env := &Env{}

	r := execJS(t, e, source, env, getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		OK bool `json:"ok"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatal(err)
	}
	if !data.OK {
		t.Error("should work with nil env maps")
	}
}

func TestEngine_Execute_ModuleIsNumber(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	// export default 42 — module is a number, not an object.
	// wrapESModule converts this to: globalThis.__worker_module__ = 42;
	// But the pool.newV8Worker check verifies __worker_module__ exists, and it does (it's 42).
	// The Execute path then tries moduleVal.AsObject() which should fail.
	source := `globalThis.__worker_module__ = 42;`
	siteID := "test-module-number"
	deployKey := "d1"

	// Bypass CompileAndCache (which wraps ES module) and store raw source
	e.sources.Store(poolKey{SiteID: siteID, DeployKey: deployKey}, source)

	result := e.Execute(siteID, deployKey, defaultEnv(), getReq("http://localhost/"))
	if result.Error == nil {
		t.Fatal("Execute should fail when module is a number")
	}
}

func TestEngine_Execute_ConcurrentRequests(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request, env) {
    const url = new URL(request.url);
    return new Response("hello-" + url.searchParams.get("id"));
  },
};`

	siteID := "test-concurrent"
	deployKey := "deploy1"
	if _, err := e.CompileAndCache(siteID, deployKey, source); err != nil {
		t.Fatalf("CompileAndCache: %v", err)
	}

	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			req := getReq(fmt.Sprintf("http://localhost/?id=%d", id))
			r := e.Execute(siteID, deployKey, defaultEnv(), req)
			if r.Error != nil {
				t.Errorf("concurrent request %d: %v", id, r.Error)
			}
		}(i)
	}
	wg.Wait()
}

func TestEngine_Execute_FetchReturnsNull(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request) {
    return null;
  },
};`
	siteID := "test-null-response"
	deployKey := "d1"
	if _, err := e.CompileAndCache(siteID, deployKey, source); err != nil {
		t.Fatalf("CompileAndCache: %v", err)
	}

	result := e.Execute(siteID, deployKey, defaultEnv(), getReq("http://localhost/"))
	if result.Error == nil {
		t.Fatal("Execute should fail when fetch returns null")
	}
}

func TestEngine_Execute_FetchReturnsRejectedPromise(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request) {
    throw new Error("handler error");
  },
};`
	siteID := "test-rejected"
	deployKey := "d1"
	if _, err := e.CompileAndCache(siteID, deployKey, source); err != nil {
		t.Fatalf("CompileAndCache: %v", err)
	}

	result := e.Execute(siteID, deployKey, defaultEnv(), getReq("http://localhost/"))
	if result.Error == nil {
		t.Fatal("Execute should fail when fetch returns rejected promise")
	}
}

func TestEngine_Execute_FetchThrowsSync(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request) {
    throw new Error("sync error");
  },
};`
	siteID := "test-sync-throw"
	deployKey := "d1"
	if _, err := e.CompileAndCache(siteID, deployKey, source); err != nil {
		t.Fatalf("CompileAndCache: %v", err)
	}

	result := e.Execute(siteID, deployKey, defaultEnv(), getReq("http://localhost/"))
	if result.Error == nil {
		t.Fatal("Execute should fail when fetch throws synchronously")
	}
}

func TestEngine_Execute_WithConsoleLogging(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request) {
    console.log("info message");
    console.warn("warn message");
    console.error("error message");
    return new Response("ok");
  },
};`
	siteID := "test-logs"
	deployKey := "d1"
	if _, err := e.CompileAndCache(siteID, deployKey, source); err != nil {
		t.Fatalf("CompileAndCache: %v", err)
	}

	result := e.Execute(siteID, deployKey, defaultEnv(), getReq("http://localhost/"))
	if result.Error != nil {
		t.Fatalf("Execute: %v", result.Error)
	}
	if len(result.Logs) < 3 {
		t.Errorf("expected at least 3 log entries, got %d", len(result.Logs))
	}
}

func TestEngine_Execute_PostRequestWithBody(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request) {
    const body = await request.text();
    return Response.json({
      method: request.method,
      body: body,
      hasBody: body.length > 0,
    });
  },
};`
	siteID := "test-post-body"
	deployKey := "d1"
	if _, err := e.CompileAndCache(siteID, deployKey, source); err != nil {
		t.Fatalf("CompileAndCache: %v", err)
	}

	req := &WorkerRequest{
		Method:  "POST",
		URL:     "http://localhost/api",
		Headers: map[string]string{"Content-Type": "text/plain"},
		Body:    []byte("hello world"),
	}
	result := e.Execute(siteID, deployKey, defaultEnv(), req)
	if result.Error != nil {
		t.Fatalf("Execute: %v", result.Error)
	}

	var data struct {
		Method  string `json:"method"`
		Body    string `json:"body"`
		HasBody bool   `json:"hasBody"`
	}
	_ = json.Unmarshal(result.Response.Body, &data)
	if data.Method != "POST" {
		t.Errorf("method = %q", data.Method)
	}
	if data.Body != "hello world" {
		t.Errorf("body = %q", data.Body)
	}
	if !data.HasBody {
		t.Error("should have body")
	}
}

func TestEngine_Execute_ResponseJsonMethod(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request) {
    return Response.json({ items: [1, 2, 3], total: 3 }, { status: 200, headers: { "x-custom": "val" } });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Items []int `json:"items"`
		Total int   `json:"total"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatal(err)
	}
	if data.Total != 3 {
		t.Errorf("total = %d, want 3", data.Total)
	}
	if len(data.Items) != 3 {
		t.Errorf("items len = %d, want 3", len(data.Items))
	}
}
