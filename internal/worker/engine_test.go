package worker

import (
	"testing"

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
// production flow in GetOrCreatePool + Execute.
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
		func(iso *v8.Isolate, ctx *v8.Context, el *eventLoop) error {
			return setupFetch(iso, ctx, el, cfg)
		},
	})
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

func (m *mockAssetsFetcher) Fetch(req *WorkerRequest) (*WorkerResponse, error) {
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
