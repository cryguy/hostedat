package worker

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/cryguy/hostedat/internal/models"
	"gorm.io/gorm"
)

func TestKVBridge_PutAndGet(t *testing.T) {
	db := testDB(t)
	kv := &KVBridge{DB: db, NamespaceID: "test-ns"}

	if err := kv.Put("greeting", "hello", nil, nil); err != nil {
		t.Fatalf("Put: %v", err)
	}

	val, err := kv.Get("greeting")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if val != "hello" {
		t.Errorf("Get = %q, want %q", val, "hello")
	}
}

func TestKVBridge_GetNotFound(t *testing.T) {
	db := testDB(t)
	kv := &KVBridge{DB: db, NamespaceID: "test-ns"}

	val, err := kv.Get("nonexistent")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if val != "" {
		t.Errorf("Get = %q, want empty string", val)
	}
}

func TestKVBridge_GetExpired(t *testing.T) {
	db := testDB(t)
	kv := &KVBridge{DB: db, NamespaceID: "test-ns-expired"}

	ttl := 1 // 1 second
	if err := kv.Put("expiring", "gone-soon", nil, &ttl); err != nil {
		t.Fatalf("Put: %v", err)
	}

	time.Sleep(3 * time.Second)

	val, err := kv.Get("expiring")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if val != "" {
		t.Errorf("Get expired key = %q, want empty", val)
	}
}

func TestKVBridge_Delete(t *testing.T) {
	db := testDB(t)
	kv := &KVBridge{DB: db, NamespaceID: "test-ns-delete"}

	if err := kv.Put("key", "value", nil, nil); err != nil {
		t.Fatalf("Put: %v", err)
	}

	if err := kv.Delete("key"); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	val, err := kv.Get("key")
	if err != nil {
		t.Fatalf("Get after delete: %v", err)
	}
	if val != "" {
		t.Errorf("Get after delete = %q, want empty", val)
	}
}

func TestKVBridge_ListWithPrefix(t *testing.T) {
	db := testDB(t)
	kv := &KVBridge{DB: db, NamespaceID: "test-ns-prefix"}

	kv.Put("user:1", "alice", nil, nil)
	kv.Put("user:2", "bob", nil, nil)
	kv.Put("other:1", "nope", nil, nil)

	results, err := kv.List("user:", 0)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("List count = %d, want 2", len(results))
	}
}

func TestKVBridge_ListWithLimit(t *testing.T) {
	db := testDB(t)
	kv := &KVBridge{DB: db, NamespaceID: "test-ns-limit"}

	for i := 0; i < 5; i++ {
		kv.Put(fmt.Sprintf("k%d", i), fmt.Sprintf("v%d", i), nil, nil)
	}

	results, err := kv.List("", 2)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("List count = %d, want 2", len(results))
	}
}

func TestKVBridge_PutWithMetadata(t *testing.T) {
	db := testDB(t)
	kv := &KVBridge{DB: db, NamespaceID: "test-ns-meta"}

	meta := "some-metadata"
	if err := kv.Put("key", "value", &meta, nil); err != nil {
		t.Fatalf("Put: %v", err)
	}

	results, err := kv.List("key", 0)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("List count = %d, want 1", len(results))
	}
	if results[0]["metadata"] != "some-metadata" {
		t.Errorf("metadata = %v, want %q", results[0]["metadata"], "some-metadata")
	}
}

// JS-level KV binding tests — exercise the Go→JS callback paths in buildKVBinding.

func TestKV_JSGetNoArgs(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    try {
      await env.MY_KV.get();
      return Response.json({ rejected: false });
    } catch(e) {
      return Response.json({ rejected: true, msg: String(e) });
    }
  },
};`

	env := kvEnv(t, db, "js-get-noargs")
	r := execJS(t, e, source, env, getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Rejected bool   `json:"rejected"`
		Msg      string `json:"msg"`
	}
	json.Unmarshal(r.Response.Body, &data)
	if !data.Rejected {
		t.Error("KV.get() with no args should reject")
	}
}

func TestKV_JSPutNoArgs(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    try {
      await env.MY_KV.put();
      return Response.json({ rejected: false });
    } catch(e) {
      return Response.json({ rejected: true, msg: String(e) });
    }
  },
};`

	env := kvEnv(t, db, "js-put-noargs")
	r := execJS(t, e, source, env, getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Rejected bool `json:"rejected"`
	}
	json.Unmarshal(r.Response.Body, &data)
	if !data.Rejected {
		t.Error("KV.put() with no args should reject")
	}
}

func TestKV_JSDeleteNoArgs(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    try {
      await env.MY_KV.delete();
      return Response.json({ rejected: false });
    } catch(e) {
      return Response.json({ rejected: true, msg: String(e) });
    }
  },
};`

	env := kvEnv(t, db, "js-del-noargs")
	r := execJS(t, e, source, env, getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Rejected bool `json:"rejected"`
	}
	json.Unmarshal(r.Response.Body, &data)
	if !data.Rejected {
		t.Error("KV.delete() with no args should reject")
	}
}

func TestKV_JSPutGetRoundTrip(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    await env.MY_KV.put("greeting", "hello world");
    const val = await env.MY_KV.get("greeting");
    return Response.json({ val });
  },
};`

	env := kvEnv(t, db, "js-roundtrip")
	r := execJS(t, e, source, env, getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Val string `json:"val"`
	}
	json.Unmarshal(r.Response.Body, &data)
	if data.Val != "hello world" {
		t.Errorf("KV round-trip: got %q, want %q", data.Val, "hello world")
	}
}

func TestKV_JSGetNotFound(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    const val = await env.MY_KV.get("nonexistent");
    return Response.json({ isNull: val === null });
  },
};`

	env := kvEnv(t, db, "js-notfound")
	r := execJS(t, e, source, env, getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		IsNull bool `json:"isNull"`
	}
	json.Unmarshal(r.Response.Body, &data)
	if !data.IsNull {
		t.Error("KV.get for missing key should return null")
	}
}

func TestKV_JSPutWithOptions(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    await env.MY_KV.put("key1", "value1", { metadata: "meta-data", expirationTtl: 3600 });
    const result = await env.MY_KV.list();
    const keys = result.keys;
    const found = keys.find(k => k.name === "key1");
    return Response.json({
      keyCount: keys.length,
      foundMeta: found ? found.metadata : null,
    });
  },
};`

	env := kvEnv(t, db, "js-put-opts")
	r := execJS(t, e, source, env, getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		KeyCount  int    `json:"keyCount"`
		FoundMeta string `json:"foundMeta"`
	}
	json.Unmarshal(r.Response.Body, &data)
	if data.KeyCount != 1 {
		t.Errorf("key count = %d, want 1", data.KeyCount)
	}
	if data.FoundMeta != "meta-data" {
		t.Errorf("metadata = %q, want %q", data.FoundMeta, "meta-data")
	}
}

func TestKV_JSListWithOptions(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    await env.MY_KV.put("user:1", "alice");
    await env.MY_KV.put("user:2", "bob");
    await env.MY_KV.put("other:1", "charlie");

    const all = await env.MY_KV.list();
    const prefixed = await env.MY_KV.list({ prefix: "user:" });
    const limited = await env.MY_KV.list({ limit: 1 });

    return Response.json({
      allCount: all.keys.length,
      prefixedCount: prefixed.keys.length,
      limitedCount: limited.keys.length,
    });
  },
};`

	env := kvEnv(t, db, "js-list-opts")
	r := execJS(t, e, source, env, getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		AllCount      int `json:"allCount"`
		PrefixedCount int `json:"prefixedCount"`
		LimitedCount  int `json:"limitedCount"`
	}
	json.Unmarshal(r.Response.Body, &data)
	if data.AllCount != 3 {
		t.Errorf("all count = %d, want 3", data.AllCount)
	}
	if data.PrefixedCount != 2 {
		t.Errorf("prefixed count = %d, want 2", data.PrefixedCount)
	}
	if data.LimitedCount != 1 {
		t.Errorf("limited count = %d, want 1", data.LimitedCount)
	}
}

func TestKV_JSDeleteAndVerify(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    await env.MY_KV.put("to-delete", "value");
    const before = await env.MY_KV.get("to-delete");
    await env.MY_KV.delete("to-delete");
    const after = await env.MY_KV.get("to-delete");
    return Response.json({ before, afterNull: after === null });
  },
};`

	env := kvEnv(t, db, "js-delete-verify")
	r := execJS(t, e, source, env, getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Before    string `json:"before"`
		AfterNull bool   `json:"afterNull"`
	}
	json.Unmarshal(r.Response.Body, &data)
	if data.Before != "value" {
		t.Errorf("before = %q, want %q", data.Before, "value")
	}
	if !data.AfterNull {
		t.Error("after delete should return null")
	}
}

// kvEnv creates an Env with a KV namespace binding backed by the test DB.
func kvEnv(t *testing.T, db *gorm.DB, nsID string) *Env {
	t.Helper()
	// Create namespace in DB.
	ns := models.KVNamespace{ID: nsID, SiteID: "test-site-kv-" + nsID, Name: "MY_KV"}
	db.Create(&ns)
	return &Env{
		Vars:       make(map[string]string),
		Secrets:    make(map[string]string),
		KVBindings: map[string]string{"MY_KV": nsID},
	}
}

func TestKVBridge_PutOverwrite(t *testing.T) {
	db := testDB(t)
	kv := &KVBridge{DB: db, NamespaceID: "test-ns-overwrite"}

	if err := kv.Put("key", "v1", nil, nil); err != nil {
		t.Fatalf("Put v1: %v", err)
	}
	if err := kv.Put("key", "v2", nil, nil); err != nil {
		t.Fatalf("Put v2: %v", err)
	}

	val, err := kv.Get("key")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if val != "v2" {
		t.Errorf("Get = %q, want %q", val, "v2")
	}
}
