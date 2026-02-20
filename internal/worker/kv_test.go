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

	_ = kv.Put("user:1", "alice", nil, nil)
	_ = kv.Put("user:2", "bob", nil, nil)
	_ = kv.Put("other:1", "nope", nil, nil)

	listResult, err := kv.List("user:", 0, "")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(listResult.Keys) != 2 {
		t.Errorf("List count = %d, want 2", len(listResult.Keys))
	}
}

func TestKVBridge_ListWithLimit(t *testing.T) {
	db := testDB(t)
	kv := &KVBridge{DB: db, NamespaceID: "test-ns-limit"}

	for i := 0; i < 5; i++ {
		_ = kv.Put(fmt.Sprintf("k%d", i), fmt.Sprintf("v%d", i), nil, nil)
	}

	listResult, err := kv.List("", 2, "")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(listResult.Keys) != 2 {
		t.Errorf("List count = %d, want 2", len(listResult.Keys))
	}
}

func TestKVBridge_PutWithMetadata(t *testing.T) {
	db := testDB(t)
	kv := &KVBridge{DB: db, NamespaceID: "test-ns-meta"}

	meta := "some-metadata"
	if err := kv.Put("key", "value", &meta, nil); err != nil {
		t.Fatalf("Put: %v", err)
	}

	listResult, err := kv.List("key", 0, "")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(listResult.Keys) != 1 {
		t.Fatalf("List count = %d, want 1", len(listResult.Keys))
	}
	if listResult.Keys[0]["metadata"] != "some-metadata" {
		t.Errorf("metadata = %v, want %q", listResult.Keys[0]["metadata"], "some-metadata")
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
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatal(err)
	}
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
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatal(err)
	}
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
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatal(err)
	}
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
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatal(err)
	}
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
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatal(err)
	}
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
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatal(err)
	}
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
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatal(err)
	}
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
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatal(err)
	}
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
	if result := db.Create(&ns); result.Error != nil {
		t.Fatal(result.Error)
	}
	return &Env{
		Vars:       make(map[string]string),
		Secrets:    make(map[string]string),
		KVBindings: map[string]string{"MY_KV": nsID},
	}
}

// ---------------------------------------------------------------------------
// Go-level GetWithMetadata tests
// ---------------------------------------------------------------------------

func TestKVBridge_GetWithMetadata(t *testing.T) {
	db := testDB(t)
	kv := &KVBridge{DB: db, NamespaceID: "test-ns-gwm"}

	meta := "some-meta"
	if err := kv.Put("key", "value", &meta, nil); err != nil {
		t.Fatalf("Put: %v", err)
	}

	result, err := kv.GetWithMetadata("key")
	if err != nil {
		t.Fatalf("GetWithMetadata: %v", err)
	}
	if result == nil {
		t.Fatal("GetWithMetadata returned nil")
	}
	if result.Value != "value" {
		t.Errorf("Value = %q, want %q", result.Value, "value")
	}
	if result.Metadata == nil || *result.Metadata != "some-meta" {
		t.Errorf("Metadata = %v, want %q", result.Metadata, "some-meta")
	}
}

func TestKVBridge_GetWithMetadataNotFound(t *testing.T) {
	db := testDB(t)
	kv := &KVBridge{DB: db, NamespaceID: "test-ns-gwm-nf"}

	result, err := kv.GetWithMetadata("missing")
	if err != nil {
		t.Fatalf("GetWithMetadata: %v", err)
	}
	if result != nil {
		t.Errorf("expected nil for missing key, got %+v", result)
	}
}

func TestKVBridge_GetWithMetadataNoMeta(t *testing.T) {
	db := testDB(t)
	kv := &KVBridge{DB: db, NamespaceID: "test-ns-gwm-nometa"}

	if err := kv.Put("key", "value", nil, nil); err != nil {
		t.Fatalf("Put: %v", err)
	}

	result, err := kv.GetWithMetadata("key")
	if err != nil {
		t.Fatalf("GetWithMetadata: %v", err)
	}
	if result == nil {
		t.Fatal("GetWithMetadata returned nil")
	}
	if result.Value != "value" {
		t.Errorf("Value = %q, want %q", result.Value, "value")
	}
	if result.Metadata != nil {
		t.Errorf("Metadata = %v, want nil", result.Metadata)
	}
}

// ---------------------------------------------------------------------------
// Go-level cursor pagination tests
// ---------------------------------------------------------------------------

func TestKVBridge_ListCursorPagination(t *testing.T) {
	db := testDB(t)
	kv := &KVBridge{DB: db, NamespaceID: "test-ns-cursor"}

	for i := 0; i < 5; i++ {
		_ = kv.Put(fmt.Sprintf("k%d", i), fmt.Sprintf("v%d", i), nil, nil)
	}

	// First page: limit 2
	page1, err := kv.List("", 2, "")
	if err != nil {
		t.Fatalf("List page1: %v", err)
	}
	if len(page1.Keys) != 2 {
		t.Fatalf("page1 count = %d, want 2", len(page1.Keys))
	}
	if page1.ListComplete {
		t.Error("page1 should not be list_complete")
	}
	if page1.Cursor == "" {
		t.Error("page1 cursor should not be empty")
	}

	// Second page: use cursor from first page
	page2, err := kv.List("", 2, page1.Cursor)
	if err != nil {
		t.Fatalf("List page2: %v", err)
	}
	if len(page2.Keys) != 2 {
		t.Fatalf("page2 count = %d, want 2", len(page2.Keys))
	}
	if page2.ListComplete {
		t.Error("page2 should not be list_complete")
	}

	// Third page: should have 1 remaining
	page3, err := kv.List("", 2, page2.Cursor)
	if err != nil {
		t.Fatalf("List page3: %v", err)
	}
	if len(page3.Keys) != 1 {
		t.Fatalf("page3 count = %d, want 1", len(page3.Keys))
	}
	if !page3.ListComplete {
		t.Error("page3 should be list_complete")
	}
	if page3.Cursor != "" {
		t.Error("page3 cursor should be empty when list_complete")
	}
}

func TestKVBridge_ListComplete(t *testing.T) {
	db := testDB(t)
	kv := &KVBridge{DB: db, NamespaceID: "test-ns-complete"}

	for i := 0; i < 3; i++ {
		_ = kv.Put(fmt.Sprintf("k%d", i), fmt.Sprintf("v%d", i), nil, nil)
	}

	result, err := kv.List("", 10, "")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(result.Keys) != 3 {
		t.Fatalf("count = %d, want 3", len(result.Keys))
	}
	if !result.ListComplete {
		t.Error("should be list_complete when all results fit")
	}
	if result.Cursor != "" {
		t.Error("cursor should be empty when list_complete")
	}
}

// ---------------------------------------------------------------------------
// JS-level getWithMetadata tests
// ---------------------------------------------------------------------------

func TestKV_JSGetWithMetadata(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    await env.MY_KV.put("key", "hello", { metadata: "meta-info" });
    const result = await env.MY_KV.getWithMetadata("key");
    return Response.json({
      value: result.value,
      metadata: result.metadata,
    });
  },
};`

	env := kvEnv(t, db, "js-gwm")
	r := execJS(t, e, source, env, getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Value    string `json:"value"`
		Metadata string `json:"metadata"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatal(err)
	}
	if data.Value != "hello" {
		t.Errorf("value = %q, want %q", data.Value, "hello")
	}
	if data.Metadata != "meta-info" {
		t.Errorf("metadata = %q, want %q", data.Metadata, "meta-info")
	}
}

func TestKV_JSGetWithMetadataNotFound(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    const result = await env.MY_KV.getWithMetadata("missing");
    return Response.json({
      valueNull: result.value === null,
      metaNull: result.metadata === null,
    });
  },
};`

	env := kvEnv(t, db, "js-gwm-nf")
	r := execJS(t, e, source, env, getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		ValueNull bool `json:"valueNull"`
		MetaNull  bool `json:"metaNull"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatal(err)
	}
	if !data.ValueNull {
		t.Error("value should be null for missing key")
	}
	if !data.MetaNull {
		t.Error("metadata should be null for missing key")
	}
}

// ---------------------------------------------------------------------------
// JS-level get with type option tests
// ---------------------------------------------------------------------------

func TestKV_JSGetTypeJSON(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    await env.MY_KV.put("data", JSON.stringify({name: "alice", age: 30}));
    const obj = await env.MY_KV.get("data", {type: "json"});
    return Response.json({
      name: obj.name,
      age: obj.age,
      isObject: typeof obj === "object",
    });
  },
};`

	env := kvEnv(t, db, "js-get-json")
	r := execJS(t, e, source, env, getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Name     string `json:"name"`
		Age      int    `json:"age"`
		IsObject bool   `json:"isObject"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatal(err)
	}
	if data.Name != "alice" {
		t.Errorf("name = %q, want %q", data.Name, "alice")
	}
	if data.Age != 30 {
		t.Errorf("age = %d, want 30", data.Age)
	}
	if !data.IsObject {
		t.Error("get with type:json should return an object")
	}
}

func TestKV_JSGetTypeArrayBuffer(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    await env.MY_KV.put("buf", "hello");
    const ab = await env.MY_KV.get("buf", {type: "arrayBuffer"});
    const isAB = ab instanceof ArrayBuffer;
    const text = new TextDecoder().decode(ab);
    return Response.json({ isAB, text, byteLength: ab.byteLength });
  },
};`

	env := kvEnv(t, db, "js-get-ab")
	r := execJS(t, e, source, env, getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		IsAB       bool   `json:"isAB"`
		Text       string `json:"text"`
		ByteLength int    `json:"byteLength"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatal(err)
	}
	if !data.IsAB {
		t.Error("get with type:arrayBuffer should return ArrayBuffer")
	}
	if data.Text != "hello" {
		t.Errorf("text = %q, want %q", data.Text, "hello")
	}
	if data.ByteLength != 5 {
		t.Errorf("byteLength = %d, want 5", data.ByteLength)
	}
}

func TestKV_JSGetTypeStream(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    await env.MY_KV.put("stream", "world");
    const stream = await env.MY_KV.get("stream", {type: "stream"});
    const isRS = stream instanceof ReadableStream;
    const reader = stream.getReader();
    const { value } = await reader.read();
    const text = new TextDecoder().decode(value);
    return Response.json({ isRS, text });
  },
};`

	env := kvEnv(t, db, "js-get-stream")
	r := execJS(t, e, source, env, getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		IsRS bool   `json:"isRS"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatal(err)
	}
	if !data.IsRS {
		t.Error("get with type:stream should return ReadableStream")
	}
	if data.Text != "world" {
		t.Errorf("text = %q, want %q", data.Text, "world")
	}
}

func TestKV_JSGetTypeText(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    await env.MY_KV.put("txt", "plain text");
    const val = await env.MY_KV.get("txt", {type: "text"});
    return Response.json({ val, isString: typeof val === "string" });
  },
};`

	env := kvEnv(t, db, "js-get-text")
	r := execJS(t, e, source, env, getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Val      string `json:"val"`
		IsString bool   `json:"isString"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatal(err)
	}
	if data.Val != "plain text" {
		t.Errorf("val = %q, want %q", data.Val, "plain text")
	}
	if !data.IsString {
		t.Error("get with type:text should return a string")
	}
}

func TestKV_JSGetTypeNullReturnsNull(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    const val = await env.MY_KV.get("missing", {type: "json"});
    return Response.json({ isNull: val === null });
  },
};`

	env := kvEnv(t, db, "js-get-type-null")
	r := execJS(t, e, source, env, getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		IsNull bool `json:"isNull"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatal(err)
	}
	if !data.IsNull {
		t.Error("get with any type for missing key should return null")
	}
}

// ---------------------------------------------------------------------------
// JS-level cursor pagination tests
// ---------------------------------------------------------------------------

func TestKV_JSListCursorPagination(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    for (let i = 0; i < 5; i++) {
      await env.MY_KV.put("k" + i, "v" + i);
    }

    // First page: limit 2
    const page1 = await env.MY_KV.list({ limit: 2 });
    // Second page: use cursor
    const page2 = await env.MY_KV.list({ limit: 2, cursor: page1.cursor });
    // Third page
    const page3 = await env.MY_KV.list({ limit: 2, cursor: page2.cursor });

    return Response.json({
      p1Count: page1.keys.length,
      p1Complete: page1.list_complete,
      p1HasCursor: typeof page1.cursor === "string" && page1.cursor.length > 0,
      p2Count: page2.keys.length,
      p2Complete: page2.list_complete,
      p3Count: page3.keys.length,
      p3Complete: page3.list_complete,
      p3NoCursor: page3.cursor === undefined || page3.cursor === null,
      totalKeys: [
        ...page1.keys.map(k => k.name),
        ...page2.keys.map(k => k.name),
        ...page3.keys.map(k => k.name),
      ].sort(),
    });
  },
};`

	env := kvEnv(t, db, "js-cursor")
	r := execJS(t, e, source, env, getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		P1Count    int      `json:"p1Count"`
		P1Complete bool     `json:"p1Complete"`
		P1HasCur   bool     `json:"p1HasCursor"`
		P2Count    int      `json:"p2Count"`
		P2Complete bool     `json:"p2Complete"`
		P3Count    int      `json:"p3Count"`
		P3Complete bool     `json:"p3Complete"`
		P3NoCursor bool     `json:"p3NoCursor"`
		TotalKeys  []string `json:"totalKeys"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatal(err)
	}

	if data.P1Count != 2 {
		t.Errorf("page1 count = %d, want 2", data.P1Count)
	}
	if data.P1Complete {
		t.Error("page1 should not be complete")
	}
	if !data.P1HasCur {
		t.Error("page1 should have a cursor")
	}
	if data.P2Count != 2 {
		t.Errorf("page2 count = %d, want 2", data.P2Count)
	}
	if data.P3Count != 1 {
		t.Errorf("page3 count = %d, want 1", data.P3Count)
	}
	if !data.P3Complete {
		t.Error("page3 should be complete")
	}
	if !data.P3NoCursor {
		t.Error("page3 should have no cursor")
	}
	if len(data.TotalKeys) != 5 {
		t.Errorf("total keys = %d, want 5", len(data.TotalKeys))
	}
}

func TestKV_JSListComplete(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    await env.MY_KV.put("a", "1");
    await env.MY_KV.put("b", "2");
    const result = await env.MY_KV.list();
    return Response.json({
      count: result.keys.length,
      complete: result.list_complete,
      noCursor: result.cursor === undefined || result.cursor === null,
    });
  },
};`

	env := kvEnv(t, db, "js-complete")
	r := execJS(t, e, source, env, getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Count    int  `json:"count"`
		Complete bool `json:"complete"`
		NoCursor bool `json:"noCursor"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatal(err)
	}
	if data.Count != 2 {
		t.Errorf("count = %d, want 2", data.Count)
	}
	if !data.Complete {
		t.Error("should be list_complete")
	}
	if !data.NoCursor {
		t.Error("should have no cursor when complete")
	}
}

// ---------------------------------------------------------------------------
// JS-level getWithMetadata with type option
// ---------------------------------------------------------------------------

func TestKV_JSGetWithMetadataJSON(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    await env.MY_KV.put("data", JSON.stringify({x: 42}), { metadata: "m" });
    const result = await env.MY_KV.getWithMetadata("data", {type: "json"});
    return Response.json({
      x: result.value.x,
      metadata: result.metadata,
      isObject: typeof result.value === "object",
    });
  },
};`

	env := kvEnv(t, db, "js-gwm-json")
	r := execJS(t, e, source, env, getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		X        int    `json:"x"`
		Metadata string `json:"metadata"`
		IsObject bool   `json:"isObject"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatal(err)
	}
	if data.X != 42 {
		t.Errorf("x = %d, want 42", data.X)
	}
	if data.Metadata != "m" {
		t.Errorf("metadata = %q, want %q", data.Metadata, "m")
	}
	if !data.IsObject {
		t.Error("value should be parsed object")
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

// ---------------------------------------------------------------------------
// JS-level getWithMetadata with arrayBuffer and stream type options
// ---------------------------------------------------------------------------

func TestKV_JSGetWithMetadataArrayBuffer(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    await env.MY_KV.put("buf", "hello", { metadata: "buf-meta" });
    const result = await env.MY_KV.getWithMetadata("buf", {type: "arrayBuffer"});
    const isAB = result.value instanceof ArrayBuffer;
    const text = new TextDecoder().decode(result.value);
    return Response.json({
      isAB,
      text,
      metadata: result.metadata,
    });
  },
};`

	env := kvEnv(t, db, "js-gwm-ab")
	r := execJS(t, e, source, env, getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		IsAB     bool   `json:"isAB"`
		Text     string `json:"text"`
		Metadata string `json:"metadata"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatal(err)
	}
	if !data.IsAB {
		t.Error("getWithMetadata type:arrayBuffer value should be ArrayBuffer")
	}
	if data.Text != "hello" {
		t.Errorf("text = %q, want %q", data.Text, "hello")
	}
	if data.Metadata != "buf-meta" {
		t.Errorf("metadata = %q, want %q", data.Metadata, "buf-meta")
	}
}

func TestKV_JSGetWithMetadataStream(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    await env.MY_KV.put("stream", "world", { metadata: "stream-meta" });
    const result = await env.MY_KV.getWithMetadata("stream", {type: "stream"});
    const isRS = result.value instanceof ReadableStream;
    const reader = result.value.getReader();
    const { value } = await reader.read();
    const text = new TextDecoder().decode(value);
    return Response.json({
      isRS,
      text,
      metadata: result.metadata,
    });
  },
};`

	env := kvEnv(t, db, "js-gwm-stream")
	r := execJS(t, e, source, env, getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		IsRS     bool   `json:"isRS"`
		Text     string `json:"text"`
		Metadata string `json:"metadata"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatal(err)
	}
	if !data.IsRS {
		t.Error("getWithMetadata type:stream value should be ReadableStream")
	}
	if data.Text != "world" {
		t.Errorf("text = %q, want %q", data.Text, "world")
	}
	if data.Metadata != "stream-meta" {
		t.Errorf("metadata = %q, want %q", data.Metadata, "stream-meta")
	}
}

// ---------------------------------------------------------------------------
// Go-level edge case tests
// ---------------------------------------------------------------------------

func TestKVBridge_PutValueExceedsMaxSize(t *testing.T) {
	db := testDB(t)
	kv := &KVBridge{DB: db, NamespaceID: "test-ns-maxsize"}

	bigValue := make([]byte, maxKVValueSize+1)
	for i := range bigValue {
		bigValue[i] = 'x'
	}

	err := kv.Put("big", string(bigValue), nil, nil)
	if err == nil {
		t.Error("Put with value > 1MB should return an error")
	}
}

func TestKVBridge_ListWithCorruptedCursor(t *testing.T) {
	db := testDB(t)
	kv := &KVBridge{DB: db, NamespaceID: "test-ns-bad-cursor"}

	_ = kv.Put("a", "1", nil, nil)
	_ = kv.Put("b", "2", nil, nil)
	_ = kv.Put("c", "3", nil, nil)

	result, err := kv.List("", 0, "!!!not-valid-base64!!!")
	if err != nil {
		t.Fatalf("List with bad cursor: %v", err)
	}
	if len(result.Keys) != 3 {
		t.Errorf("expected 3 keys from offset 0, got %d", len(result.Keys))
	}
}

func TestKVBridge_PutWithTTLZeroNeverExpires(t *testing.T) {
	db := testDB(t)
	kv := &KVBridge{DB: db, NamespaceID: "test-ns-ttl-zero"}

	ttl := 0
	if err := kv.Put("key", "persistent", nil, &ttl); err != nil {
		t.Fatalf("Put: %v", err)
	}

	val, err := kv.Get("key")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if val != "persistent" {
		t.Errorf("Get = %q, want %q", val, "persistent")
	}
}

func TestKVBridge_NamespaceIsolation(t *testing.T) {
	db := testDB(t)
	kv1 := &KVBridge{DB: db, NamespaceID: "test-ns-iso-1"}
	kv2 := &KVBridge{DB: db, NamespaceID: "test-ns-iso-2"}

	if err := kv1.Put("shared-key", "ns1-value", nil, nil); err != nil {
		t.Fatalf("kv1.Put: %v", err)
	}
	if err := kv2.Put("shared-key", "ns2-value", nil, nil); err != nil {
		t.Fatalf("kv2.Put: %v", err)
	}

	val1, err := kv1.Get("shared-key")
	if err != nil {
		t.Fatalf("kv1.Get: %v", err)
	}
	if val1 != "ns1-value" {
		t.Errorf("kv1.Get = %q, want %q", val1, "ns1-value")
	}

	val2, err := kv2.Get("shared-key")
	if err != nil {
		t.Fatalf("kv2.Get: %v", err)
	}
	if val2 != "ns2-value" {
		t.Errorf("kv2.Get = %q, want %q", val2, "ns2-value")
	}
}

func TestKVBridge_ListCursorPastEnd(t *testing.T) {
	db := testDB(t)
	kv := &KVBridge{DB: db, NamespaceID: "test-ns-cursor-past-end"}

	_ = kv.Put("a", "1", nil, nil)
	_ = kv.Put("b", "2", nil, nil)
	_ = kv.Put("c", "3", nil, nil)

	cursor := encodeCursor(100)
	result, err := kv.List("", 10, cursor)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(result.Keys) != 0 {
		t.Errorf("expected 0 keys, got %d", len(result.Keys))
	}
	if !result.ListComplete {
		t.Error("list_complete should be true when no entries returned")
	}
}

func TestKVBridge_EncodeCursorDecodeCursorRoundtrip(t *testing.T) {
	offsets := []int{0, 1, 2, 100, 999, 1000, 123456}
	for _, offset := range offsets {
		cursor := encodeCursor(offset)
		if cursor == "" {
			t.Errorf("encodeCursor(%d) returned empty string", offset)
			continue
		}
		decoded := decodeCursor(cursor)
		if decoded != offset {
			t.Errorf("roundtrip offset %d: encodeCursor -> decodeCursor = %d", offset, decoded)
		}
	}

	if got := decodeCursor(""); got != 0 {
		t.Errorf("decodeCursor(\"\") = %d, want 0", got)
	}
}
