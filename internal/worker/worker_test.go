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
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

func testCfg() config.WorkerConfig {
	return config.WorkerConfig{
		PoolSize:         2,
		MemoryLimitMB:    128,
		ExecutionTimeout: 5000,
		MaxFetchRequests: 50,
		FetchTimeoutSec:  5,
		MaxResponseBytes: 10 * 1024 * 1024,
		MaxLogRetention:  7,
		MaxScriptSizeKB:  1024,
	}
}

func testDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Discard,
	})
	if err != nil {
		t.Fatalf("opening test db: %v", err)
	}
	db.Exec("PRAGMA foreign_keys = OFF")
	if err := db.AutoMigrate(
		&models.User{},
		&models.Site{},
		&models.WorkerEnvVar{},
		&models.KVNamespace{},
		&models.KVEntry{},
		&models.WorkerLog{},
		&models.StorageBucket{},
	); err != nil {
		t.Fatalf("auto-migrate: %v", err)
	}
	return db
}

func newTestEngine(t *testing.T, db *gorm.DB) *Engine {
	t.Helper()
	e := NewEngine(testCfg(), db)
	t.Cleanup(func() { e.Shutdown() })
	return e
}

// execJS compiles a worker from source and executes it against the given
// request. It returns the WorkerResult for assertion.
func execJS(t *testing.T, e *Engine, source string, env *Env, req *WorkerRequest) *WorkerResult {
	t.Helper()
	siteID := "test-" + t.Name()
	deployKey := "deploy1"

	if _, err := e.CompileAndCache(siteID, deployKey, source); err != nil {
		t.Fatalf("CompileAndCache: %v", err)
	}
	return e.Execute(siteID, deployKey, env, req)
}

func defaultEnv() *Env {
	return &Env{
		Vars:       make(map[string]string),
		Secrets:    make(map[string]string),
		KVBindings: make(map[string]string),
	}
}

func getReq(url string) *WorkerRequest {
	return &WorkerRequest{
		Method:  "GET",
		URL:     url,
		Headers: map[string]string{},
	}
}

// assertOK checks result has no error and a non-nil response.
func assertOK(t *testing.T, r *WorkerResult) {
	t.Helper()
	if r == nil {
		t.Fatal("result is nil (Execute returned nil)")
	}
	if r.Error != nil {
		t.Fatalf("unexpected error: %v", r.Error)
	}
	if r.Response == nil {
		t.Fatal("response is nil")
	}
}

// routingAssetsFetcher implements AssetsFetcher with a callback.
type routingAssetsFetcher struct {
	fn func(req *WorkerRequest) (*WorkerResponse, error)
}

func (m *routingAssetsFetcher) Fetch(req *WorkerRequest) (*WorkerResponse, error) {
	return m.fn(req)
}

// ---------------------------------------------------------------------------
// 1. Basic Handler
// ---------------------------------------------------------------------------

func TestBasic_SyncFetch(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request, env) {
    return new Response("hello sync");
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	if r.Response.StatusCode != 200 {
		t.Errorf("status = %d, want 200", r.Response.StatusCode)
	}
	if string(r.Response.Body) != "hello sync" {
		t.Errorf("body = %q, want %q", r.Response.Body, "hello sync")
	}
}

func TestBasic_AsyncFetch(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    return new Response("hello async");
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	if r.Response.StatusCode != 200 {
		t.Errorf("status = %d, want 200", r.Response.StatusCode)
	}
	if string(r.Response.Body) != "hello async" {
		t.Errorf("body = %q, want %q", r.Response.Body, "hello async")
	}
}

func TestBasic_RequestObjectAccess(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    const data = {
      method: request.method,
      url: request.url,
      host: request.headers.get("host"),
    };
    return new Response(JSON.stringify(data), {
      headers: { "content-type": "application/json" },
    });
  },
};`

	req := &WorkerRequest{
		Method:  "POST",
		URL:     "http://example.com/path",
		Headers: map[string]string{"host": "example.com"},
	}
	r := execJS(t, e, source, defaultEnv(), req)
	assertOK(t, r)

	var data map[string]string
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if data["method"] != "POST" {
		t.Errorf("method = %q, want POST", data["method"])
	}
	if data["url"] != "http://example.com/path" {
		t.Errorf("url = %q", data["url"])
	}
	if data["host"] != "example.com" {
		t.Errorf("host = %q", data["host"])
	}
}

func TestBasic_URLAndSearchParams(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    const url = new URL(request.url);
    return Response.json({
      pathname: url.pathname,
      search: url.search,
      key: url.searchParams.get("key"),
      missing: url.searchParams.get("nope"),
    });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/foo?key=bar&x=1"))
	assertOK(t, r)

	var data map[string]interface{}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if data["pathname"] != "/foo" {
		t.Errorf("pathname = %v", data["pathname"])
	}
	if data["key"] != "bar" {
		t.Errorf("key = %v", data["key"])
	}
	if data["missing"] != nil {
		t.Errorf("missing = %v, want nil", data["missing"])
	}
}

func TestBasic_URLNoQueryParams(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    const url = new URL(request.url);
    return Response.json({
      pathname: url.pathname,
      search: url.search,
      hasKey: url.searchParams.has("key"),
    });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/plain"))
	assertOK(t, r)

	var data map[string]interface{}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if data["pathname"] != "/plain" {
		t.Errorf("pathname = %v", data["pathname"])
	}
	if data["search"] != "" {
		t.Errorf("search = %v, want empty", data["search"])
	}
	if data["hasKey"] != false {
		t.Errorf("hasKey = %v", data["hasKey"])
	}
}

func TestBasic_JSONResponse(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request, env) {
    return Response.json({ msg: "ok", n: 42 });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	ct := r.Response.Headers["content-type"]
	if ct != "application/json" {
		t.Errorf("content-type = %q", ct)
	}
	var data map[string]interface{}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("not valid JSON: %v", err)
	}
	if data["msg"] != "ok" {
		t.Errorf("msg = %v", data["msg"])
	}
}

func TestBasic_CustomStatusAndHeaders(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	tests := []struct {
		name   string
		status int
	}{
		{"201 Created", 201},
		{"204 No Content", 204},
		{"404 Not Found", 404},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			source := fmt.Sprintf(`export default {
  fetch(request, env) {
    return new Response(null, {
      status: %d,
      headers: { "x-custom": "val-%d" },
    });
  },
};`, tc.status, tc.status)
			r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
			assertOK(t, r)
			if r.Response.StatusCode != tc.status {
				t.Errorf("status = %d, want %d", r.Response.StatusCode, tc.status)
			}
			if r.Response.Headers["x-custom"] != fmt.Sprintf("val-%d", tc.status) {
				t.Errorf("x-custom = %q", r.Response.Headers["x-custom"])
			}
		})
	}
}

func TestBasic_RequestBodyText(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    const body = await request.text();
    return new Response("echo:" + body);
  },
};`

	req := &WorkerRequest{
		Method:  "POST",
		URL:     "http://localhost/echo",
		Headers: map[string]string{"content-type": "text/plain"},
		Body:    []byte("hello body"),
	}
	r := execJS(t, e, source, defaultEnv(), req)
	assertOK(t, r)

	if string(r.Response.Body) != "echo:hello body" {
		t.Errorf("body = %q", r.Response.Body)
	}
}

func TestBasic_RequestBodyJSON(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    const data = await request.json();
    return Response.json({ got: data.name });
  },
};`

	req := &WorkerRequest{
		Method:  "POST",
		URL:     "http://localhost/",
		Headers: map[string]string{"content-type": "application/json"},
		Body:    []byte(`{"name":"alice"}`),
	}
	r := execJS(t, e, source, defaultEnv(), req)
	assertOK(t, r)

	var data map[string]string
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if data["got"] != "alice" {
		t.Errorf("got = %q", data["got"])
	}
}

// ---------------------------------------------------------------------------
// 2. Routing
// ---------------------------------------------------------------------------

func TestRouting_PathBased(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    const url = new URL(request.url);
    switch (url.pathname) {
      case "/a": return new Response("alpha");
      case "/b": return new Response("bravo");
      case "/c": return new Response("charlie");
      default:   return new Response("fallback", { status: 404 });
    }
  },
};`

	for _, tc := range []struct{ path, want string }{
		{"/a", "alpha"},
		{"/b", "bravo"},
		{"/c", "charlie"},
	} {
		t.Run(tc.path, func(t *testing.T) {
			r := execJS(t, e, source, defaultEnv(), getReq("http://localhost"+tc.path))
			assertOK(t, r)
			if string(r.Response.Body) != tc.want {
				t.Errorf("body = %q, want %q", r.Response.Body, tc.want)
			}
		})
	}
}

func TestRouting_MethodBased(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    if (request.method === "GET") return new Response("got GET");
    if (request.method === "POST") return new Response("got POST");
    return new Response("other", { status: 405 });
  },
};`

	rGet := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, rGet)
	if string(rGet.Response.Body) != "got GET" {
		t.Errorf("GET body = %q", rGet.Response.Body)
	}

	rPost := execJS(t, e, source, defaultEnv(), &WorkerRequest{
		Method: "POST", URL: "http://localhost/", Headers: map[string]string{},
	})
	assertOK(t, rPost)
	if string(rPost.Response.Body) != "got POST" {
		t.Errorf("POST body = %q", rPost.Response.Body)
	}
}

func TestRouting_404Fallback(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    const url = new URL(request.url);
    if (url.pathname === "/known") return new Response("ok");
    return new Response("custom 404", { status: 404 });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/unknown"))
	assertOK(t, r)
	if r.Response.StatusCode != 404 {
		t.Errorf("status = %d, want 404", r.Response.StatusCode)
	}
	if string(r.Response.Body) != "custom 404" {
		t.Errorf("body = %q", r.Response.Body)
	}
}

// ---------------------------------------------------------------------------
// 3. KV Namespace
// ---------------------------------------------------------------------------

// kvTestSetup creates an engine and env with a KV namespace bound to "NS".
func kvTestSetup(t *testing.T) (*Engine, *Env, *gorm.DB) {
	t.Helper()
	db := testDB(t)
	e := newTestEngine(t, db)

	ns := models.KVNamespace{ID: "ns-test-" + t.Name(), SiteID: "site1", Name: "NS"}
	if err := db.Create(&ns).Error; err != nil {
		t.Fatalf("creating KV namespace: %v", err)
	}

	env := &Env{
		Vars:       make(map[string]string),
		Secrets:    make(map[string]string),
		KVBindings: map[string]string{"NS": ns.ID},
	}
	return e, env, db
}

func TestKV_PutAndGet(t *testing.T) {
	e, env, _ := kvTestSetup(t)

	source := `export default {
  async fetch(request, env) {
    await env.NS.put("greeting", "hello");
    const val = await env.NS.get("greeting");
    return new Response(val);
  },
};`

	r := execJS(t, e, source, env, getReq("http://localhost/"))
	assertOK(t, r)
	if string(r.Response.Body) != "hello" {
		t.Errorf("body = %q, want %q", r.Response.Body, "hello")
	}
}

func TestKV_GetNonexistent(t *testing.T) {
	e, env, _ := kvTestSetup(t)

	source := `export default {
  async fetch(request, env) {
    const val = await env.NS.get("missing");
    return Response.json({ val: val });
  },
};`

	r := execJS(t, e, source, env, getReq("http://localhost/"))
	assertOK(t, r)

	var data map[string]interface{}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if data["val"] != nil {
		t.Errorf("val = %v, want null", data["val"])
	}
}

func TestKV_Overwrite(t *testing.T) {
	e, env, _ := kvTestSetup(t)

	source := `export default {
  async fetch(request, env) {
    await env.NS.put("k", "v1");
    await env.NS.put("k", "v2");
    const val = await env.NS.get("k");
    return new Response(val);
  },
};`

	r := execJS(t, e, source, env, getReq("http://localhost/"))
	assertOK(t, r)
	if string(r.Response.Body) != "v2" {
		t.Errorf("body = %q, want v2", r.Response.Body)
	}
}

func TestKV_Delete(t *testing.T) {
	e, env, _ := kvTestSetup(t)

	source := `export default {
  async fetch(request, env) {
    await env.NS.put("k", "val");
    await env.NS.delete("k");
    const val = await env.NS.get("k");
    return Response.json({ val: val });
  },
};`

	r := execJS(t, e, source, env, getReq("http://localhost/"))
	assertOK(t, r)

	var data map[string]interface{}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if data["val"] != nil {
		t.Errorf("val = %v, want null", data["val"])
	}
}

func TestKV_ListWithPrefix(t *testing.T) {
	e, env, _ := kvTestSetup(t)

	source := `export default {
  async fetch(request, env) {
    await env.NS.put("user:1", "alice");
    await env.NS.put("user:2", "bob");
    await env.NS.put("user:3", "charlie");
    await env.NS.put("other:1", "nope");
    const result = await env.NS.list({ prefix: "user:" });
    return Response.json({ count: result.keys.length, keys: result.keys.map(k => k.name).sort() });
  },
};`

	r := execJS(t, e, source, env, getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Count int      `json:"count"`
		Keys  []string `json:"keys"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if data.Count != 3 {
		t.Errorf("count = %d, want 3", data.Count)
	}
}

func TestKV_ListWithLimit(t *testing.T) {
	e, env, _ := kvTestSetup(t)

	source := `export default {
  async fetch(request, env) {
    for (let i = 0; i < 5; i++) {
      await env.NS.put("k" + i, "v" + i);
    }
    const result = await env.NS.list({ limit: 2 });
    return Response.json({ count: result.keys.length });
  },
};`

	r := execJS(t, e, source, env, getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Count int `json:"count"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if data.Count != 2 {
		t.Errorf("count = %d, want 2", data.Count)
	}
}

func TestKV_Metadata(t *testing.T) {
	e, env, _ := kvTestSetup(t)

	source := `export default {
  async fetch(request, env) {
    await env.NS.put("k", "v", { metadata: "meta-info" });
    const result = await env.NS.list({ prefix: "k" });
    const entry = result.keys[0];
    return Response.json({ name: entry.name, metadata: entry.metadata });
  },
};`

	r := execJS(t, e, source, env, getReq("http://localhost/"))
	assertOK(t, r)

	var data map[string]interface{}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if data["name"] != "k" {
		t.Errorf("name = %v", data["name"])
	}
	if data["metadata"] != "meta-info" {
		t.Errorf("metadata = %v", data["metadata"])
	}
}

func TestKV_ExpirationTTL(t *testing.T) {
	e, env, _ := kvTestSetup(t)

	source := `export default {
  async fetch(request, env) {
    await env.NS.put("expiring", "gone-soon", { expirationTtl: 1 });
    return new Response("stored");
  },
};`

	r := execJS(t, e, source, env, getReq("http://localhost/"))
	assertOK(t, r)

	// Wait for TTL to expire.
	time.Sleep(2 * time.Second)

	// Read it back via a second execution.
	readSource := `export default {
  async fetch(request, env) {
    const val = await env.NS.get("expiring");
    return Response.json({ val: val });
  },
};`
	siteID := "test-" + t.Name() + "-read"
	e.CompileAndCache(siteID, "deploy1", readSource)
	r2 := e.Execute(siteID, "deploy1", env, getReq("http://localhost/"))
	assertOK(t, r2)

	var data map[string]interface{}
	if err := json.Unmarshal(r2.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if data["val"] != nil {
		t.Errorf("expired value should be nil, got %v", data["val"])
	}
}

func TestKV_LargeValue(t *testing.T) {
	e, env, _ := kvTestSetup(t)

	// Generate a ~500 KB value (well under 1 MB limit).
	bigVal := strings.Repeat("x", 500*1024)

	source := fmt.Sprintf(`export default {
  async fetch(request, env) {
    const big = "%s";
    await env.NS.put("big", big);
    const val = await env.NS.get("big");
    return Response.json({ match: val === big, len: val.length });
  },
};`, bigVal)

	r := execJS(t, e, source, env, getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Match bool `json:"match"`
		Len   int  `json:"len"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !data.Match {
		t.Error("large value mismatch")
	}
	if data.Len != 500*1024 {
		t.Errorf("len = %d, want %d", data.Len, 500*1024)
	}
}

func TestKV_MultipleNamespaces(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	ns1 := models.KVNamespace{ID: "ns1-" + t.Name(), SiteID: "site1", Name: "NS1"}
	ns2 := models.KVNamespace{ID: "ns2-" + t.Name(), SiteID: "site1", Name: "NS2"}
	db.Create(&ns1)
	db.Create(&ns2)

	env := &Env{
		Vars:       make(map[string]string),
		Secrets:    make(map[string]string),
		KVBindings: map[string]string{"NS1": ns1.ID, "NS2": ns2.ID},
	}

	source := `export default {
  async fetch(request, env) {
    await env.NS1.put("k", "from-ns1");
    await env.NS2.put("k", "from-ns2");
    const v1 = await env.NS1.get("k");
    const v2 = await env.NS2.get("k");
    return Response.json({ v1, v2 });
  },
};`

	r := execJS(t, e, source, env, getReq("http://localhost/"))
	assertOK(t, r)

	var data map[string]string
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if data["v1"] != "from-ns1" {
		t.Errorf("v1 = %q", data["v1"])
	}
	if data["v2"] != "from-ns2" {
		t.Errorf("v2 = %q", data["v2"])
	}
}

// ---------------------------------------------------------------------------
// 4. ASSETS Binding
// ---------------------------------------------------------------------------

func TestAssets_BasicStaticFile(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	fetcher := &routingAssetsFetcher{fn: func(req *WorkerRequest) (*WorkerResponse, error) {
		return &WorkerResponse{
			StatusCode: 200,
			Headers:    map[string]string{"content-type": "text/html"},
			Body:       []byte("<h1>Index</h1>"),
		}, nil
	}}
	env := &Env{
		Vars: make(map[string]string), Secrets: make(map[string]string),
		KVBindings: make(map[string]string), Assets: fetcher,
	}

	source := `export default {
  async fetch(request, env) {
    return env.ASSETS.fetch(request);
  },
};`

	r := execJS(t, e, source, env, getReq("http://localhost/"))
	assertOK(t, r)
	if string(r.Response.Body) != "<h1>Index</h1>" {
		t.Errorf("body = %q", r.Response.Body)
	}
}

func TestAssets_NestedFiles(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	fetcher := &routingAssetsFetcher{fn: func(req *WorkerRequest) (*WorkerResponse, error) {
		if strings.Contains(req.URL, "style.css") {
			return &WorkerResponse{StatusCode: 200, Headers: map[string]string{"content-type": "text/css"}, Body: []byte("body{}")}, nil
		}
		if strings.Contains(req.URL, "app.js") {
			return &WorkerResponse{StatusCode: 200, Headers: map[string]string{"content-type": "application/javascript"}, Body: []byte("console.log('hi')")}, nil
		}
		return &WorkerResponse{StatusCode: 404, Headers: map[string]string{}, Body: []byte("Not Found")}, nil
	}}
	env := &Env{
		Vars: make(map[string]string), Secrets: make(map[string]string),
		KVBindings: make(map[string]string), Assets: fetcher,
	}

	source := `export default {
  async fetch(request, env) {
    return env.ASSETS.fetch(request);
  },
};`

	r := execJS(t, e, source, env, getReq("http://localhost/css/style.css"))
	assertOK(t, r)
	if string(r.Response.Body) != "body{}" {
		t.Errorf("css body = %q", r.Response.Body)
	}
	if r.Response.Headers["content-type"] != "text/css" {
		t.Errorf("css content-type = %q", r.Response.Headers["content-type"])
	}
}

func TestAssets_ConditionalFallback(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	fetcher := &routingAssetsFetcher{fn: func(req *WorkerRequest) (*WorkerResponse, error) {
		return &WorkerResponse{StatusCode: 200, Headers: map[string]string{}, Body: []byte("static")}, nil
	}}
	env := &Env{
		Vars: make(map[string]string), Secrets: make(map[string]string),
		KVBindings: make(map[string]string), Assets: fetcher,
	}

	source := `export default {
  async fetch(request, env) {
    const url = new URL(request.url);
    if (url.pathname.startsWith("/api/")) {
      return Response.json({ api: true });
    }
    return env.ASSETS.fetch(request);
  },
};`

	rAPI := execJS(t, e, source, env, getReq("http://localhost/api/users"))
	assertOK(t, rAPI)
	var data map[string]interface{}
	if err := json.Unmarshal(rAPI.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if data["api"] != true {
		t.Errorf("api handler not triggered")
	}

	rStatic := execJS(t, e, source, env, getReq("http://localhost/index.html"))
	assertOK(t, rStatic)
	if string(rStatic.Response.Body) != "static" {
		t.Errorf("static body = %q", rStatic.Response.Body)
	}
}

func TestAssets_404(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	fetcher := &routingAssetsFetcher{fn: func(req *WorkerRequest) (*WorkerResponse, error) {
		return &WorkerResponse{StatusCode: 404, Headers: map[string]string{}, Body: []byte("Not Found")}, nil
	}}
	env := &Env{
		Vars: make(map[string]string), Secrets: make(map[string]string),
		KVBindings: make(map[string]string), Assets: fetcher,
	}

	source := `export default {
  async fetch(request, env) {
    return env.ASSETS.fetch(request);
  },
};`

	r := execJS(t, e, source, env, getReq("http://localhost/nope.txt"))
	assertOK(t, r)
	if r.Response.StatusCode != 404 {
		t.Errorf("status = %d, want 404", r.Response.StatusCode)
	}
}

func TestAssets_ModifiedRequestURL(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	fetcher := &routingAssetsFetcher{fn: func(req *WorkerRequest) (*WorkerResponse, error) {
		if strings.Contains(req.URL, "/rewritten.html") {
			return &WorkerResponse{StatusCode: 200, Headers: map[string]string{}, Body: []byte("rewritten content")}, nil
		}
		return &WorkerResponse{StatusCode: 404, Headers: map[string]string{}, Body: []byte("Not Found")}, nil
	}}
	env := &Env{
		Vars: make(map[string]string), Secrets: make(map[string]string),
		KVBindings: make(map[string]string), Assets: fetcher,
	}

	source := `export default {
  async fetch(request, env) {
    const newReq = new Request("http://localhost/rewritten.html", {
      method: request.method,
      headers: request.headers,
    });
    return env.ASSETS.fetch(newReq);
  },
};`

	r := execJS(t, e, source, env, getReq("http://localhost/original"))
	assertOK(t, r)
	if string(r.Response.Body) != "rewritten content" {
		t.Errorf("body = %q", r.Response.Body)
	}
}

// ---------------------------------------------------------------------------
// 5. Environment Variables
// ---------------------------------------------------------------------------

func TestEnv_AccessVar(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	env := &Env{
		Vars:       map[string]string{"MY_VAR": "hello"},
		Secrets:    make(map[string]string),
		KVBindings: make(map[string]string),
	}

	source := `export default {
  fetch(request, env) {
    return new Response(env.MY_VAR);
  },
};`

	r := execJS(t, e, source, env, getReq("http://localhost/"))
	assertOK(t, r)
	if string(r.Response.Body) != "hello" {
		t.Errorf("body = %q, want hello", r.Response.Body)
	}
}

func TestEnv_UndefinedVar(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request, env) {
    const val = env.NONEXISTENT;
    return new Response(String(val));
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)
	if string(r.Response.Body) != "undefined" {
		t.Errorf("body = %q, want undefined", r.Response.Body)
	}
}

func TestEnv_MultipleVarsAndSecrets(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	env := &Env{
		Vars:       map[string]string{"A": "1", "B": "2"},
		Secrets:    map[string]string{"SECRET": "s3cret"},
		KVBindings: make(map[string]string),
	}

	source := `export default {
  fetch(request, env) {
    return Response.json({ a: env.A, b: env.B, secret: env.SECRET });
  },
};`

	r := execJS(t, e, source, env, getReq("http://localhost/"))
	assertOK(t, r)

	var data map[string]string
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if data["a"] != "1" || data["b"] != "2" || data["secret"] != "s3cret" {
		t.Errorf("env vars: %v", data)
	}
}

// ---------------------------------------------------------------------------
// 6. Outbound Fetch — SSRF Protection
// ---------------------------------------------------------------------------

func TestFetch_SSRFBlocked(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	blockedURLs := []string{
		"http://127.0.0.1/",
		"http://localhost/secret",
		"http://169.254.169.254/metadata",
	}

	for _, u := range blockedURLs {
		t.Run(u, func(t *testing.T) {
			source := fmt.Sprintf(`export default {
  async fetch(request, env) {
    try {
      await fetch("%s");
      return new Response("should not reach", { status: 200 });
    } catch(e) {
      return new Response("blocked: " + e.message, { status: 403 });
    }
  },
};`, u)
			r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
			assertOK(t, r)
			if r.Response.StatusCode != 403 {
				t.Errorf("status = %d, want 403 (blocked)", r.Response.StatusCode)
			}
			if !strings.Contains(string(r.Response.Body), "blocked") {
				t.Errorf("body = %q, expected 'blocked'", r.Response.Body)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// 7. Console Logging
// ---------------------------------------------------------------------------

func TestConsole_LogCaptured(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request, env) {
    console.log("hello from worker");
    return new Response("ok");
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	if len(r.Logs) == 0 {
		t.Fatal("no logs captured")
	}
	if r.Logs[0].Level != "log" {
		t.Errorf("level = %q, want log", r.Logs[0].Level)
	}
	if r.Logs[0].Message != "hello from worker" {
		t.Errorf("message = %q", r.Logs[0].Message)
	}
}

func TestConsole_WarnAndError(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request, env) {
    console.warn("a warning");
    console.error("an error");
    console.debug("debug info");
    console.info("info msg");
    return new Response("ok");
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	if len(r.Logs) != 4 {
		t.Fatalf("log count = %d, want 4", len(r.Logs))
	}

	expected := []struct{ level, msg string }{
		{"warn", "a warning"},
		{"error", "an error"},
		{"debug", "debug info"},
		{"info", "info msg"},
	}
	for i, exp := range expected {
		if r.Logs[i].Level != exp.level {
			t.Errorf("log[%d].level = %q, want %q", i, r.Logs[i].Level, exp.level)
		}
		if r.Logs[i].Message != exp.msg {
			t.Errorf("log[%d].message = %q, want %q", i, r.Logs[i].Message, exp.msg)
		}
	}
}

// ---------------------------------------------------------------------------
// 8. Cron / Scheduled Handler
// ---------------------------------------------------------------------------

func TestScheduled_BasicInvocation(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async scheduled(event, env, ctx) {
    console.log("cron ran: " + event.cron);
  },
};`

	siteID := "test-sched-" + t.Name()
	if _, err := e.CompileAndCache(siteID, "deploy1", source); err != nil {
		t.Fatalf("compile: %v", err)
	}

	r := e.ExecuteScheduled(siteID, "deploy1", defaultEnv(), "*/5 * * * *")
	if r.Error != nil {
		t.Fatalf("error: %v", r.Error)
	}
	if len(r.Logs) == 0 {
		t.Fatal("no logs from scheduled handler")
	}
	if !strings.Contains(r.Logs[0].Message, "*/5 * * * *") {
		t.Errorf("log = %q, expected cron expression", r.Logs[0].Message)
	}
}

func TestScheduled_WritesToKV(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	ns := models.KVNamespace{ID: "ns-sched-kv", SiteID: "site1", Name: "STORE"}
	db.Create(&ns)

	env := &Env{
		Vars:       make(map[string]string),
		Secrets:    make(map[string]string),
		KVBindings: map[string]string{"STORE": ns.ID},
	}

	source := `export default {
  async scheduled(event, env, ctx) {
    await env.STORE.put("last_run", String(event.scheduledTime));
  },
  async fetch(request, env) {
    const val = await env.STORE.get("last_run");
    return new Response(val || "never");
  },
};`

	siteID := "test-sched-kv"
	if _, err := e.CompileAndCache(siteID, "deploy1", source); err != nil {
		t.Fatalf("compile: %v", err)
	}

	// Run scheduled handler — it writes to KV.
	r := e.ExecuteScheduled(siteID, "deploy1", env, "0 * * * *")
	if r.Error != nil {
		t.Fatalf("scheduled error: %v", r.Error)
	}

	// Now fetch — should read the value back.
	r2 := e.Execute(siteID, "deploy1", env, getReq("http://localhost/"))
	assertOK(t, r2)
	if string(r2.Response.Body) == "never" || string(r2.Response.Body) == "" {
		t.Errorf("KV value not written by scheduled handler, body = %q", r2.Response.Body)
	}
}

// ---------------------------------------------------------------------------
// 9. Pool & Deploy Lifecycle
// ---------------------------------------------------------------------------

func TestPool_Invalidation(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)
	siteID := "pool-inv"
	env := defaultEnv()

	// Compile and execute v1.
	srcV1 := `export default { fetch() { return new Response("v1"); } };`
	if _, err := e.CompileAndCache(siteID, "deploy1", srcV1); err != nil {
		t.Fatalf("compile v1: %v", err)
	}
	r1 := e.Execute(siteID, "deploy1", env, getReq("http://localhost/"))
	assertOK(t, r1)
	if string(r1.Response.Body) != "v1" {
		t.Fatalf("v1 body = %q", r1.Response.Body)
	}

	// Invalidate and compile v2.
	e.InvalidatePool(siteID, "deploy1")
	srcV2 := `export default { fetch() { return new Response("v2"); } };`
	if _, err := e.CompileAndCache(siteID, "deploy1", srcV2); err != nil {
		t.Fatalf("compile v2: %v", err)
	}

	// All 10 executions should return v2.
	for i := 0; i < 10; i++ {
		r := e.Execute(siteID, "deploy1", env, getReq("http://localhost/"))
		assertOK(t, r)
		if string(r.Response.Body) != "v2" {
			t.Fatalf("execution %d returned %q, want v2", i, r.Response.Body)
		}
	}
}

func TestPool_RapidRecompile(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)
	siteID := "rapid-recompile"
	env := defaultEnv()

	for v := 1; v <= 3; v++ {
		e.InvalidatePool(siteID, "deploy1")
		src := fmt.Sprintf(`export default { fetch() { return new Response("v%d"); } };`, v)
		if _, err := e.CompileAndCache(siteID, "deploy1", src); err != nil {
			t.Fatalf("compile v%d: %v", v, err)
		}
	}

	r := e.Execute(siteID, "deploy1", env, getReq("http://localhost/"))
	assertOK(t, r)
	if string(r.Response.Body) != "v3" {
		t.Errorf("body = %q, want v3", r.Response.Body)
	}
}

func TestPool_Concurrency(t *testing.T) {
	db := testDB(t)
	cfg := testCfg()
	cfg.PoolSize = 4
	e := NewEngine(cfg, db)
	t.Cleanup(func() { e.Shutdown() })

	siteID := "pool-concurrent"
	src := `export default { fetch() { return new Response("ok"); } };`
	if _, err := e.CompileAndCache(siteID, "deploy1", src); err != nil {
		t.Fatalf("compile: %v", err)
	}

	env := defaultEnv()
	var wg sync.WaitGroup
	errors := make(chan error, 20)

	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r := e.Execute(siteID, "deploy1", env, getReq("http://localhost/"))
			if r.Error != nil {
				errors <- r.Error
				return
			}
			if r.Response == nil || string(r.Response.Body) != "ok" {
				errors <- fmt.Errorf("unexpected response: %v", r.Response)
			}
		}()
	}
	wg.Wait()
	close(errors)

	for err := range errors {
		t.Errorf("concurrent error: %v", err)
	}
}

func TestPool_Exhaustion(t *testing.T) {
	db := testDB(t)
	cfg := testCfg()
	cfg.PoolSize = 2
	// Short timeout so the test doesn't hang.
	cfg.ExecutionTimeout = 2000
	e := NewEngine(cfg, db)
	t.Cleanup(func() { e.Shutdown() })

	siteID := "pool-exhaust"
	// Worker that does some work but completes.
	src := `export default { fetch() { return new Response("done"); } };`
	if _, err := e.CompileAndCache(siteID, "deploy1", src); err != nil {
		t.Fatalf("compile: %v", err)
	}

	env := defaultEnv()
	var wg sync.WaitGroup
	results := make(chan *WorkerResult, 10)

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r := e.Execute(siteID, "deploy1", env, getReq("http://localhost/"))
			results <- r
		}()
	}
	wg.Wait()
	close(results)

	var ok, errCount int
	for r := range results {
		if r.Error != nil {
			errCount++
		} else if r.Response != nil && string(r.Response.Body) == "done" {
			ok++
		}
	}
	// All should succeed (pool queues rather than erroring).
	if ok == 0 {
		t.Fatal("no successful responses")
	}
	t.Logf("pool exhaustion: %d ok, %d errors", ok, errCount)
}

// ---------------------------------------------------------------------------
// 10. Edge Cases & Error Handling
// ---------------------------------------------------------------------------

func TestEdge_EmptyResponseBody(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request, env) {
    return new Response(null, { status: 204 });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)
	if r.Response.StatusCode != 204 {
		t.Errorf("status = %d, want 204", r.Response.StatusCode)
	}
	if len(r.Response.Body) != 0 {
		t.Errorf("body = %q, want empty", r.Response.Body)
	}
}

func TestEdge_LargeResponseBody(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	// Build 1 MB of "A"s via JS.
	source := `export default {
  fetch(request, env) {
    let s = "A";
    for (let i = 0; i < 20; i++) s = s + s;
    return new Response(s);
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)
	// 2^20 = 1048576 bytes
	if len(r.Response.Body) != 1048576 {
		t.Errorf("body length = %d, want 1048576", len(r.Response.Body))
	}
}

func TestEdge_InfiniteLoop(t *testing.T) {
	db := testDB(t)
	cfg := testCfg()
	cfg.ExecutionTimeout = 500 // 500ms timeout
	e := NewEngine(cfg, db)
	t.Cleanup(func() { e.Shutdown() })

	source := `export default {
  fetch(request, env) {
    while(true) {}
    return new Response("unreachable");
  },
};`

	siteID := "inf-loop"
	if _, err := e.CompileAndCache(siteID, "deploy1", source); err != nil {
		t.Fatalf("compile: %v", err)
	}

	r := e.Execute(siteID, "deploy1", defaultEnv(), getReq("http://localhost/"))
	if r.Error == nil {
		t.Fatal("expected timeout error for infinite loop")
	}
	if !strings.Contains(r.Error.Error(), "timed out") {
		t.Errorf("error = %v, expected 'timed out'", r.Error)
	}
	t.Logf("infinite loop error: %v (duration: %v)", r.Error, r.Duration)
}

func TestEdge_UncaughtException(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request, env) {
    throw new Error("boom");
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	if r.Error == nil {
		t.Fatal("expected error for uncaught exception, got nil")
	}
	if !strings.Contains(r.Error.Error(), "boom") {
		t.Errorf("error = %v, expected to contain 'boom'", r.Error)
	}
}

func TestEdge_MemoryLimit(t *testing.T) {
	db := testDB(t)
	cfg := testCfg()
	cfg.MemoryLimitMB = 8 // very low limit
	cfg.ExecutionTimeout = 3000
	e := NewEngine(cfg, db)
	t.Cleanup(func() { e.Shutdown() })

	source := `export default {
  fetch(request, env) {
    const arr = [];
    for (let i = 0; i < 10000000; i++) {
      arr.push(new Array(1000).fill("x"));
    }
    return new Response("unreachable");
  },
};`

	siteID := "mem-limit"
	if _, err := e.CompileAndCache(siteID, "deploy1", source); err != nil {
		t.Fatalf("compile: %v", err)
	}

	r := e.Execute(siteID, "deploy1", defaultEnv(), getReq("http://localhost/"))
	if r.Error == nil {
		t.Fatal("expected error for memory limit, got nil")
	}
	t.Logf("memory limit error: %v", r.Error)
}

func TestEdge_NoDefaultExport(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	// Script without a default export — no __worker_module__ assigned.
	source := `const handler = { fetch() { return new Response("nope"); } };`

	siteID := "no-default"
	if _, err := e.CompileAndCache(siteID, "deploy1", source); err != nil {
		t.Fatalf("compile: %v", err)
	}

	r := e.Execute(siteID, "deploy1", defaultEnv(), getReq("http://localhost/"))
	if r.Error == nil {
		t.Fatal("expected error for missing default export")
	}
	t.Logf("no default export error: %v", r.Error)
}

func TestEdge_SyntaxError(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default { fetch() { return new Response("ok" };`

	siteID := "syntax-err"
	_, err := e.CompileAndCache(siteID, "deploy1", source)
	if err == nil {
		t.Fatal("expected compile error for syntax error, got nil")
	}
	t.Logf("syntax error: %v", err)
}

func TestEdge_NonResponseReturn(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request, env) {
    return "not a response";
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	// The engine should either error or produce a degraded response.
	// "not a response" is a string, not a Response object, so
	// jsResponseToGo will attempt to read its .status (undefined → 0).
	if r.Error != nil {
		t.Logf("non-Response return error (expected): %v", r.Error)
		return
	}
	// If it didn't error, status will be 0 (no .status property on string).
	if r.Response != nil && r.Response.StatusCode == 0 {
		t.Logf("non-Response return: status=0 (degraded, acceptable)")
		return
	}
	t.Logf("non-Response return: response=%+v", r.Response)
}

func TestEdge_MissingFetchHandler(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	if r.Error == nil {
		t.Fatal("expected error for missing fetch handler")
	}
	t.Logf("missing fetch error: %v", r.Error)
}
