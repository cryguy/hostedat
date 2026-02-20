package worker

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/cryguy/hostedat/internal/models"
)

func TestCacheBridge_PutAndMatch(t *testing.T) {
	db := testDB(t)
	bridge := &CacheBridge{DB: db}

	err := bridge.Put("default", "https://example.com/test", 200, `{"content-type":"text/html"}`, []byte("hello"), nil)
	if err != nil {
		t.Fatalf("Put: %v", err)
	}

	entry, err := bridge.Match("default", "https://example.com/test")
	if err != nil {
		t.Fatalf("Match: %v", err)
	}
	if entry == nil {
		t.Fatal("Match returned nil, expected entry")
	}
	if entry.Status != 200 {
		t.Errorf("Status = %d, want 200", entry.Status)
	}
	if string(entry.Body) != "hello" {
		t.Errorf("Body = %q, want 'hello'", string(entry.Body))
	}
}

func TestCacheBridge_MatchNotFound(t *testing.T) {
	db := testDB(t)
	bridge := &CacheBridge{DB: db}

	entry, err := bridge.Match("default", "https://example.com/missing")
	if err != nil {
		t.Fatalf("Match: %v", err)
	}
	if entry != nil {
		t.Errorf("expected nil for missing entry, got %+v", entry)
	}
}

func TestCacheBridge_PutReplaces(t *testing.T) {
	db := testDB(t)
	bridge := &CacheBridge{DB: db}

	_ = bridge.Put("default", "https://example.com/page", 200, "{}", []byte("first"), nil)
	_ = bridge.Put("default", "https://example.com/page", 201, "{}", []byte("second"), nil)

	entry, _ := bridge.Match("default", "https://example.com/page")
	if entry == nil {
		t.Fatal("Match returned nil")
	}
	if entry.Status != 201 {
		t.Errorf("Status = %d, want 201 (replaced)", entry.Status)
	}
	if string(entry.Body) != "second" {
		t.Errorf("Body = %q, want 'second'", string(entry.Body))
	}

	// Verify only one entry exists.
	var count int64
	db.Model(&models.CacheEntry{}).Where("url = ?", "https://example.com/page").Count(&count)
	if count != 1 {
		t.Errorf("expected 1 entry, got %d", count)
	}
}

func TestCacheBridge_Delete(t *testing.T) {
	db := testDB(t)
	bridge := &CacheBridge{DB: db}

	_ = bridge.Put("default", "https://example.com/del", 200, "{}", []byte("data"), nil)
	deleted, err := bridge.Delete("default", "https://example.com/del")
	if err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if !deleted {
		t.Error("Delete should return true for existing entry")
	}

	entry, _ := bridge.Match("default", "https://example.com/del")
	if entry != nil {
		t.Error("entry should be deleted")
	}
}

func TestCacheBridge_DeleteNotFound(t *testing.T) {
	db := testDB(t)
	bridge := &CacheBridge{DB: db}

	deleted, err := bridge.Delete("default", "https://example.com/nope")
	if err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if deleted {
		t.Error("Delete should return false for non-existent entry")
	}
}

func TestCacheBridge_TTLExpiration(t *testing.T) {
	db := testDB(t)
	bridge := &CacheBridge{DB: db}

	// Use TTL of 0 to create an already-expired entry.
	ttl := -1 // negative value won't set ExpiresAt via Put
	_ = bridge.Put("default", "https://example.com/ttl", 200, "{}", []byte("expired"), &ttl)

	// Manually set a past expiration.
	db.Model(&models.CacheEntry{}).Where("url = ?", "https://example.com/ttl").
		Update("expires_at", "2020-01-01 00:00:00")

	entry, _ := bridge.Match("default", "https://example.com/ttl")
	if entry != nil {
		t.Error("expired entry should not be returned by Match")
	}
}

func TestCacheBridge_SeparateCacheNames(t *testing.T) {
	db := testDB(t)
	bridge := &CacheBridge{DB: db}

	_ = bridge.Put("cache-a", "https://example.com/url", 200, "{}", []byte("from-a"), nil)
	_ = bridge.Put("cache-b", "https://example.com/url", 200, "{}", []byte("from-b"), nil)

	entryA, _ := bridge.Match("cache-a", "https://example.com/url")
	entryB, _ := bridge.Match("cache-b", "https://example.com/url")

	if entryA == nil || string(entryA.Body) != "from-a" {
		t.Errorf("cache-a entry body = %v", entryA)
	}
	if entryB == nil || string(entryB.Body) != "from-b" {
		t.Errorf("cache-b entry body = %v", entryB)
	}
}

// ---------------------------------------------------------------------------
// Integration tests (V8)
// ---------------------------------------------------------------------------

func TestCache_DefaultPutAndMatch(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    // Put a response into the default cache.
    var url = 'https://example.com/cached-page';
    var resp = new Response('cached body', {
      status: 200,
      headers: { 'Content-Type': 'text/html', 'Cache-Control': 'max-age=3600' },
    });
    await caches.default.put(url, resp);

    // Match it back.
    var matched = await caches.default.match(url);
    if (!matched) {
      return new Response('MISS', { status: 500 });
    }
    var body = matched._body;
    return Response.json({
      status: matched.status,
      body: body,
      ct: matched.headers.get('content-type'),
    });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Status int    `json:"status"`
		Body   string `json:"body"`
		CT     string `json:"ct"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if data.Status != 200 {
		t.Errorf("status = %d, want 200", data.Status)
	}
	if data.Body != "cached body" {
		t.Errorf("body = %q, want 'cached body'", data.Body)
	}
	if !strings.Contains(data.CT, "text/html") {
		t.Errorf("content-type = %q, want text/html", data.CT)
	}
}

func TestCache_MatchMiss(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    var matched = await caches.default.match('https://example.com/not-cached');
    return Response.json({ hit: matched !== undefined });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Hit bool `json:"hit"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatal(err)
	}
	if data.Hit {
		t.Error("should be a cache miss")
	}
}

func TestCache_Delete(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    var url = 'https://example.com/to-delete';
    await caches.default.put(url, new Response('data'));

    // Verify it exists.
    var before = await caches.default.match(url);
    if (!before) return new Response('put failed', { status: 500 });

    // Delete it.
    var deleted = await caches.default.delete(url);

    // Verify it's gone.
    var after = await caches.default.match(url);

    return Response.json({
      deleted: deleted,
      gone: after === undefined,
    });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Deleted bool `json:"deleted"`
		Gone    bool `json:"gone"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatal(err)
	}
	if !data.Deleted {
		t.Error("delete should return true")
	}
	if !data.Gone {
		t.Error("entry should be gone after delete")
	}
}

func TestCache_OpenNamedCache(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    var myCache = await caches.open('my-cache');
    var url = 'https://example.com/named';
    await myCache.put(url, new Response('named-data'));

    // Should NOT appear in default cache.
    var inDefault = await caches.default.match(url);

    // Should appear in named cache.
    var inNamed = await myCache.match(url);

    return Response.json({
      inDefault: inDefault !== undefined,
      inNamed: inNamed !== undefined,
      body: inNamed ? inNamed._body : null,
    });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		InDefault bool    `json:"inDefault"`
		InNamed   bool    `json:"inNamed"`
		Body      *string `json:"body"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatal(err)
	}
	if data.InDefault {
		t.Error("entry should not be in default cache")
	}
	if !data.InNamed {
		t.Error("entry should be in named cache")
	}
	if data.Body == nil || *data.Body != "named-data" {
		t.Errorf("body = %v, want 'named-data'", data.Body)
	}
}

func TestCache_PutWithRequest(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    var req = new Request('https://example.com/req-cache');
    await caches.default.put(req, new Response('from-request'));

    var matched = await caches.default.match(req);
    return Response.json({
      hit: matched !== undefined,
      body: matched ? matched._body : null,
    });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Hit  bool    `json:"hit"`
		Body *string `json:"body"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatal(err)
	}
	if !data.Hit {
		t.Error("should match using Request object")
	}
	if data.Body == nil || *data.Body != "from-request" {
		t.Errorf("body = %v, want 'from-request'", data.Body)
	}
}

func TestCache_MatchWithStringURL(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    await caches.default.put('https://example.com/str', new Response('string-url'));
    var matched = await caches.default.match('https://example.com/str');
    return Response.json({
      hit: matched !== undefined,
      body: matched ? matched._body : null,
    });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Hit  bool    `json:"hit"`
		Body *string `json:"body"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatal(err)
	}
	if !data.Hit {
		t.Error("should match with string URL")
	}
}

func TestCache_CachesOpenReturnsSameInstance(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    var c1 = await caches.open('test');
    var c2 = await caches.open('test');
    return Response.json({ same: c1 === c2 });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Same bool `json:"same"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatal(err)
	}
	if !data.Same {
		t.Error("caches.open should return same instance for same name")
	}
}

func TestCache_DeleteNonExistent(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    var deleted = await caches.default.delete('https://example.com/nonexistent');
    return Response.json({ deleted: deleted });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Deleted bool `json:"deleted"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatal(err)
	}
	if data.Deleted {
		t.Error("deleting non-existent entry should return false")
	}
}

func TestCacheBridge_BinaryBody(t *testing.T) {
	db := testDB(t)
	bridge := &CacheBridge{DB: db}

	body := []byte{0x00, 0xFF, 0x01, 0xFE, 0x80, 0x7F, 0xAB, 0xCD}
	err := bridge.Put("default", "https://example.com/binary", 200, "{}", body, nil)
	if err != nil {
		t.Fatalf("Put: %v", err)
	}

	entry, err := bridge.Match("default", "https://example.com/binary")
	if err != nil {
		t.Fatalf("Match: %v", err)
	}
	if entry == nil {
		t.Fatal("Match returned nil, expected entry")
	}
	if len(entry.Body) != len(body) {
		t.Fatalf("body length = %d, want %d", len(entry.Body), len(body))
	}
	for i, b := range body {
		if entry.Body[i] != b {
			t.Errorf("body[%d] = 0x%02X, want 0x%02X", i, entry.Body[i], b)
		}
	}
}

func TestCache_PutNoResponse(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    var rejected = false;
    var msg = '';
    try {
      await caches.default.put('https://example.com/no-resp', undefined);
    } catch(e) {
      rejected = true;
      msg = e.message;
    }
    return Response.json({ rejected: rejected, msg: msg });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Rejected bool   `json:"rejected"`
		Msg      string `json:"msg"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !data.Rejected {
		t.Error("cache.put(url, undefined) should reject")
	}
	if !strings.Contains(data.Msg, "Cache.put requires a response") {
		t.Errorf("error message = %q, want 'Cache.put requires a response'", data.Msg)
	}
}

func TestCacheBridge_TTLZeroNoExpiry(t *testing.T) {
	db := testDB(t)
	bridge := &CacheBridge{DB: db}

	ttl := 0
	err := bridge.Put("default", "https://example.com/ttl-zero", 200, "{}", []byte("data"), &ttl)
	if err != nil {
		t.Fatalf("Put: %v", err)
	}

	entry, err := bridge.Match("default", "https://example.com/ttl-zero")
	if err != nil {
		t.Fatalf("Match: %v", err)
	}
	if entry == nil {
		t.Fatal("Match returned nil, expected entry")
	}
	if entry.ExpiresAt != nil {
		t.Errorf("ExpiresAt = %v, want nil for ttl=0", entry.ExpiresAt)
	}
}

func TestCache_MatchNullRequest(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    var matched = await caches.default.match(null);
    return Response.json({ hit: matched !== undefined });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Hit bool `json:"hit"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if data.Hit {
		t.Error("match(null) should return undefined, not a hit")
	}
}
