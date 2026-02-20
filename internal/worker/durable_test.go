package worker

import (
	"encoding/json"
	"testing"

	v8 "github.com/tommie/v8go"
)

// ---------------------------------------------------------------------------
// Go bridge tests
// ---------------------------------------------------------------------------

func TestDurableBridge_PutAndGet(t *testing.T) {
	db := testDB(t)
	_ = db.AutoMigrate(&DurableObjectEntry{})
	b := &DurableObjectBridge{DB: db}

	if err := b.Put("ns1", "obj1", "greeting", `"hello"`); err != nil {
		t.Fatalf("Put: %v", err)
	}

	val, err := b.Get("ns1", "obj1", "greeting")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if val != `"hello"` {
		t.Errorf("Get = %q, want %q", val, `"hello"`)
	}
}

func TestDurableBridge_GetNotFound(t *testing.T) {
	db := testDB(t)
	_ = db.AutoMigrate(&DurableObjectEntry{})
	b := &DurableObjectBridge{DB: db}

	val, err := b.Get("ns1", "obj1", "missing")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if val != "" {
		t.Errorf("Get = %q, want empty", val)
	}
}

func TestDurableBridge_PutOverwrite(t *testing.T) {
	db := testDB(t)
	_ = db.AutoMigrate(&DurableObjectEntry{})
	b := &DurableObjectBridge{DB: db}

	_ = b.Put("ns1", "obj1", "key", `"v1"`)
	_ = b.Put("ns1", "obj1", "key", `"v2"`)

	val, err := b.Get("ns1", "obj1", "key")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if val != `"v2"` {
		t.Errorf("Get = %q, want %q", val, `"v2"`)
	}
}

func TestDurableBridge_Delete(t *testing.T) {
	db := testDB(t)
	_ = db.AutoMigrate(&DurableObjectEntry{})
	b := &DurableObjectBridge{DB: db}

	_ = b.Put("ns1", "obj1", "key", `"val"`)
	if err := b.Delete("ns1", "obj1", "key"); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	val, err := b.Get("ns1", "obj1", "key")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if val != "" {
		t.Errorf("Get after delete = %q, want empty", val)
	}
}

func TestDurableBridge_DeleteAll(t *testing.T) {
	db := testDB(t)
	_ = db.AutoMigrate(&DurableObjectEntry{})
	b := &DurableObjectBridge{DB: db}

	_ = b.Put("ns1", "obj1", "a", `1`)
	_ = b.Put("ns1", "obj1", "b", `2`)
	_ = b.Put("ns1", "obj1", "c", `3`)

	if err := b.DeleteAll("ns1", "obj1"); err != nil {
		t.Fatalf("DeleteAll: %v", err)
	}

	entries, err := b.List("ns1", "obj1", "", 0, false)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("List after deleteAll: got %d entries, want 0", len(entries))
	}
}

func TestDurableBridge_DeleteMulti(t *testing.T) {
	db := testDB(t)
	_ = db.AutoMigrate(&DurableObjectEntry{})
	b := &DurableObjectBridge{DB: db}

	_ = b.Put("ns1", "obj1", "a", `1`)
	_ = b.Put("ns1", "obj1", "b", `2`)
	_ = b.Put("ns1", "obj1", "c", `3`)

	count, err := b.DeleteMulti("ns1", "obj1", []string{"a", "b"})
	if err != nil {
		t.Fatalf("DeleteMulti: %v", err)
	}
	if count != 2 {
		t.Errorf("DeleteMulti count = %d, want 2", count)
	}

	val, _ := b.Get("ns1", "obj1", "c")
	if val != `3` {
		t.Errorf("remaining entry: %q, want %q", val, `3`)
	}
}

func TestDurableBridge_ListPrefix(t *testing.T) {
	db := testDB(t)
	_ = db.AutoMigrate(&DurableObjectEntry{})
	b := &DurableObjectBridge{DB: db}

	_ = b.Put("ns1", "obj1", "user:1", `"alice"`)
	_ = b.Put("ns1", "obj1", "user:2", `"bob"`)
	_ = b.Put("ns1", "obj1", "other:1", `"nope"`)

	entries, err := b.List("ns1", "obj1", "user:", 0, false)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(entries) != 2 {
		t.Errorf("List count = %d, want 2", len(entries))
	}
}

func TestDurableBridge_GetMulti(t *testing.T) {
	db := testDB(t)
	_ = db.AutoMigrate(&DurableObjectEntry{})
	b := &DurableObjectBridge{DB: db}

	_ = b.Put("ns1", "obj1", "a", `"alpha"`)
	_ = b.Put("ns1", "obj1", "b", `"bravo"`)

	result, err := b.GetMulti("ns1", "obj1", []string{"a", "b", "c"})
	if err != nil {
		t.Fatalf("GetMulti: %v", err)
	}
	if len(result) != 2 {
		t.Errorf("GetMulti count = %d, want 2", len(result))
	}
	if result["a"] != `"alpha"` {
		t.Errorf("a = %q", result["a"])
	}
}

func TestDurableID_Deterministic(t *testing.T) {
	id1 := durableObjectID("ns", "myobj")
	id2 := durableObjectID("ns", "myobj")
	if id1 != id2 {
		t.Errorf("idFromName not deterministic: %q != %q", id1, id2)
	}
	if len(id1) != 64 {
		t.Errorf("id length = %d, want 64 (sha256 hex)", len(id1))
	}
}

func TestDurableID_DifferentInputs(t *testing.T) {
	id1 := durableObjectID("ns", "obj1")
	id2 := durableObjectID("ns", "obj2")
	if id1 == id2 {
		t.Error("different names should produce different IDs")
	}
}

func TestDurableUniqueID(t *testing.T) {
	id1, err := durableObjectUniqueID()
	if err != nil {
		t.Fatal(err)
	}
	id2, err := durableObjectUniqueID()
	if err != nil {
		t.Fatal(err)
	}
	if id1 == id2 {
		t.Error("newUniqueId should return different IDs")
	}
	if len(id1) != 32 {
		t.Errorf("unique id length = %d, want 32", len(id1))
	}
}

// ---------------------------------------------------------------------------
// JS-level binding tests
// ---------------------------------------------------------------------------

// doTestCtx creates a V8 isolate+context with the Response polyfill and
// a Durable Object binding on globalThis.MY_DO for testing.
func doTestCtx(t *testing.T) (*v8.Isolate, *v8.Context, *DurableObjectBridge) {
	t.Helper()
	db := testDB(t)
	_ = db.AutoMigrate(&DurableObjectEntry{})

	iso := v8.NewIsolate()
	ctx := v8.NewContext(iso)
	el := newEventLoop()

	t.Cleanup(func() {
		ctx.Close()
		iso.Dispose()
	})

	// Set up Response and Response.json polyfills needed for tests.
	if err := setupWebAPIs(iso, ctx, el); err != nil {
		t.Fatalf("setupWebAPIs: %v", err)
	}

	bridge := &DurableObjectBridge{DB: db}
	doVal, err := buildDurableObjectBinding(iso, ctx, bridge, "test-ns")
	if err != nil {
		t.Fatalf("buildDurableObjectBinding: %v", err)
	}
	if err := ctx.Global().Set("MY_DO", doVal); err != nil {
		t.Fatalf("setting MY_DO: %v", err)
	}

	return iso, ctx, bridge
}

// runDOScript runs a JS script in the DO test context and returns the JSON result.
func runDOScript(t *testing.T, ctx *v8.Context, script string) map[string]interface{} {
	t.Helper()
	val, err := ctx.RunScript(script, "test.js")
	if err != nil {
		t.Fatalf("RunScript: %v", err)
	}

	// If the result is a promise, pump microtasks to resolve it.
	if val.IsPromise() {
		// Set up promise resolution capture
		if err := ctx.Global().Set("__test_promise", val); err != nil {
			t.Fatalf("setting test promise: %v", err)
		}
		_, err := ctx.RunScript(`
			globalThis.__test_resolved = undefined;
			globalThis.__test_rejected = undefined;
			Promise.resolve(globalThis.__test_promise).then(
				r => { globalThis.__test_resolved = r; },
				e => { globalThis.__test_rejected = String(e); }
			);
			delete globalThis.__test_promise;
		`, "promise_setup.js")
		if err != nil {
			t.Fatalf("promise setup: %v", err)
		}

		for i := 0; i < 100; i++ {
			ctx.PerformMicrotaskCheckpoint()
			check, _ := ctx.RunScript("globalThis.__test_resolved !== undefined || globalThis.__test_rejected !== undefined", "check.js")
			if check != nil && check.Boolean() {
				break
			}
		}

		rejected, _ := ctx.RunScript("globalThis.__test_rejected", "check_reject.js")
		if rejected != nil && !rejected.IsUndefined() {
			t.Fatalf("promise rejected: %s", rejected.String())
		}

		val, err = ctx.RunScript("globalThis.__test_resolved", "get_result.js")
		if err != nil {
			t.Fatalf("getting resolved value: %v", err)
		}
	}

	// Convert val to JSON string then parse
	if err := ctx.Global().Set("__test_val", val); err != nil {
		t.Fatalf("setting test val: %v", err)
	}
	jsonVal, err := ctx.RunScript("JSON.stringify(globalThis.__test_val)", "stringify.js")
	if err != nil {
		t.Fatalf("stringify: %v", err)
	}
	_, _ = ctx.RunScript("delete globalThis.__test_val; delete globalThis.__test_resolved; delete globalThis.__test_rejected;", "cleanup.js")

	var result map[string]interface{}
	if err := json.Unmarshal([]byte(jsonVal.String()), &result); err != nil {
		t.Fatalf("unmarshal %q: %v", jsonVal.String(), err)
	}
	return result
}

func TestDurable_JSIdFromName(t *testing.T) {
	_, ctx, _ := doTestCtx(t)

	data := runDOScript(t, ctx, `(function() {
		var id = MY_DO.idFromName("test-object");
		var id2 = MY_DO.idFromName("test-object");
		var id3 = MY_DO.idFromName("different-object");
		return {
			hex: id.toString(),
			hexLen: id.toString().length,
			same: id.toString() === id2.toString(),
			different: id.toString() !== id3.toString(),
			equals: id.equals(id2),
			notEquals: !id.equals(id3),
		};
	})()`)

	if data["hex"] == "" {
		t.Error("hex should not be empty")
	}
	if data["hexLen"].(float64) != 64 {
		t.Errorf("hex length = %v, want 64", data["hexLen"])
	}
	if data["same"] != true {
		t.Error("same name should produce same ID")
	}
	if data["different"] != true {
		t.Error("different names should produce different IDs")
	}
	if data["equals"] != true {
		t.Error("equals should return true for same IDs")
	}
	if data["notEquals"] != true {
		t.Error("notEquals should be true for different IDs")
	}
}

func TestDurable_JSIdFromString(t *testing.T) {
	_, ctx, _ := doTestCtx(t)

	data := runDOScript(t, ctx, `(function() {
		var id = MY_DO.idFromName("test-object");
		var hex = id.toString();
		var id2 = MY_DO.idFromString(hex);
		return {
			roundTrip: id2.toString() === hex,
			equals: id.equals(id2),
		};
	})()`)

	if data["roundTrip"] != true {
		t.Error("idFromString round-trip failed")
	}
	if data["equals"] != true {
		t.Error("equals should be true after round-trip")
	}
}

func TestDurable_JSNewUniqueId(t *testing.T) {
	_, ctx, _ := doTestCtx(t)

	data := runDOScript(t, ctx, `(function() {
		var id1 = MY_DO.newUniqueId();
		var id2 = MY_DO.newUniqueId();
		return {
			hex1: id1.toString(),
			hex2: id2.toString(),
			len1: id1.toString().length,
			different: id1.toString() !== id2.toString(),
			notEquals: !id1.equals(id2),
		};
	})()`)

	if data["hex1"] == "" || data["hex2"] == "" {
		t.Error("unique IDs should not be empty")
	}
	if data["len1"].(float64) != 32 {
		t.Errorf("unique id length = %v, want 32", data["len1"])
	}
	if data["different"] != true {
		t.Error("newUniqueId should return different IDs")
	}
	if data["notEquals"] != true {
		t.Error("notEquals should be true for unique IDs")
	}
}

func TestDurable_JSIdEquals(t *testing.T) {
	_, ctx, _ := doTestCtx(t)

	data := runDOScript(t, ctx, `(function() {
		var id1 = MY_DO.idFromName("same");
		var id2 = MY_DO.idFromName("same");
		var id3 = MY_DO.idFromName("other");
		return {
			sameEquals: id1.equals(id2),
			diffNotEquals: !id1.equals(id3),
			noArgFalse: id1.equals() === false,
		};
	})()`)

	if data["sameEquals"] != true {
		t.Error("equals should be true for same name")
	}
	if data["diffNotEquals"] != true {
		t.Error("notEquals should be true for different names")
	}
	if data["noArgFalse"] != true {
		t.Error("equals() with no arg should return false")
	}
}

func TestDurable_JSStoragePutGet(t *testing.T) {
	_, ctx, _ := doTestCtx(t)

	data := runDOScript(t, ctx, `(async function() {
		var id = MY_DO.idFromName("storage-test");
		var stub = MY_DO.get(id);
		await stub.storage.put("greeting", "hello world");
		var val = await stub.storage.get("greeting");
		return { val: val };
	})()`)

	if data["val"] != "hello world" {
		t.Errorf("val = %v, want hello world", data["val"])
	}
}

func TestDurable_JSStoragePutGetObject(t *testing.T) {
	_, ctx, _ := doTestCtx(t)

	data := runDOScript(t, ctx, `(async function() {
		var id = MY_DO.idFromName("obj-test");
		var stub = MY_DO.get(id);
		await stub.storage.put("data", { name: "alice", age: 30 });
		var val = await stub.storage.get("data");
		return { name: val.name, age: val.age };
	})()`)

	if data["name"] != "alice" {
		t.Errorf("name = %v, want alice", data["name"])
	}
	if data["age"].(float64) != 30 {
		t.Errorf("age = %v, want 30", data["age"])
	}
}

func TestDurable_JSStorageDelete(t *testing.T) {
	_, ctx, _ := doTestCtx(t)

	data := runDOScript(t, ctx, `(async function() {
		var id = MY_DO.idFromName("del-test");
		var stub = MY_DO.get(id);
		await stub.storage.put("key", "value");
		var before = await stub.storage.get("key");
		await stub.storage.delete("key");
		var after = await stub.storage.get("key");
		return { before: before, afterNull: after === null };
	})()`)

	if data["before"] != "value" {
		t.Errorf("before = %v, want value", data["before"])
	}
	if data["afterNull"] != true {
		t.Error("after delete should be null")
	}
}

func TestDurable_JSStorageDeleteAll(t *testing.T) {
	_, ctx, _ := doTestCtx(t)

	data := runDOScript(t, ctx, `(async function() {
		var id = MY_DO.idFromName("delall-test");
		var stub = MY_DO.get(id);
		await stub.storage.put("a", 1);
		await stub.storage.put("b", 2);
		await stub.storage.put("c", 3);
		await stub.storage.deleteAll();
		var list = await stub.storage.list();
		return { size: list.size };
	})()`)

	if data["size"].(float64) != 0 {
		t.Errorf("size = %v, want 0", data["size"])
	}
}

func TestDurable_JSStorageList(t *testing.T) {
	_, ctx, _ := doTestCtx(t)

	data := runDOScript(t, ctx, `(async function() {
		var id = MY_DO.idFromName("list-test");
		var stub = MY_DO.get(id);
		await stub.storage.put("user:1", "alice");
		await stub.storage.put("user:2", "bob");
		await stub.storage.put("other:1", "nope");

		var all = await stub.storage.list();
		var prefixed = await stub.storage.list({ prefix: "user:" });
		var limited = await stub.storage.list({ limit: 1 });

		return {
			allSize: all.size,
			prefixedSize: prefixed.size,
			limitedSize: limited.size,
		};
	})()`)

	if data["allSize"].(float64) != 3 {
		t.Errorf("allSize = %v, want 3", data["allSize"])
	}
	if data["prefixedSize"].(float64) != 2 {
		t.Errorf("prefixedSize = %v, want 2", data["prefixedSize"])
	}
	if data["limitedSize"].(float64) != 1 {
		t.Errorf("limitedSize = %v, want 1", data["limitedSize"])
	}
}

func TestDurable_JSStubFetch(t *testing.T) {
	_, ctx, _ := doTestCtx(t)

	data := runDOScript(t, ctx, `(async function() {
		var id = MY_DO.idFromName("fetch-test");
		var stub = MY_DO.get(id);
		var resp = await stub.fetch("http://fake/");
		var text = await resp.text();
		return { status: resp.status, body: text };
	})()`)

	if data["status"].(float64) != 200 {
		t.Errorf("status = %v, want 200", data["status"])
	}
	if data["body"] != "ok" {
		t.Errorf("body = %v, want ok", data["body"])
	}
}

func TestDurable_JSStoragePutMulti(t *testing.T) {
	_, ctx, _ := doTestCtx(t)

	data := runDOScript(t, ctx, `(async function() {
		var id = MY_DO.idFromName("putmulti-test");
		var stub = MY_DO.get(id);
		await stub.storage.put({ x: 10, y: 20, z: 30 });
		var xVal = await stub.storage.get("x");
		var yVal = await stub.storage.get("y");
		var zVal = await stub.storage.get("z");
		return { x: xVal, y: yVal, z: zVal };
	})()`)

	if data["x"].(float64) != 10 {
		t.Errorf("x = %v, want 10", data["x"])
	}
	if data["y"].(float64) != 20 {
		t.Errorf("y = %v, want 20", data["y"])
	}
	if data["z"].(float64) != 30 {
		t.Errorf("z = %v, want 30", data["z"])
	}
}

func TestDurable_JSStorageGetMulti(t *testing.T) {
	_, ctx, _ := doTestCtx(t)

	data := runDOScript(t, ctx, `(async function() {
		var id = MY_DO.idFromName("getmulti-test");
		var stub = MY_DO.get(id);
		await stub.storage.put("a", "alpha");
		await stub.storage.put("b", "bravo");
		await stub.storage.put("c", "charlie");
		var result = await stub.storage.get(["a", "c"]);
		// result is a Map
		return { aVal: result.get("a"), cVal: result.get("c"), size: result.size };
	})()`)

	if data["aVal"] != "alpha" {
		t.Errorf("aVal = %v, want alpha", data["aVal"])
	}
	if data["cVal"] != "charlie" {
		t.Errorf("cVal = %v, want charlie", data["cVal"])
	}
	if data["size"].(float64) != 2 {
		t.Errorf("size = %v, want 2", data["size"])
	}
}

func TestDurable_JSStorageDeleteMulti(t *testing.T) {
	_, ctx, _ := doTestCtx(t)

	data := runDOScript(t, ctx, `(async function() {
		var id = MY_DO.idFromName("delmulti-test");
		var stub = MY_DO.get(id);
		await stub.storage.put("a", 1);
		await stub.storage.put("b", 2);
		await stub.storage.put("c", 3);
		var count = await stub.storage.delete(["a", "b"]);
		var remaining = await stub.storage.list();
		return { count: count, remainingSize: remaining.size };
	})()`)

	if data["count"].(float64) != 2 {
		t.Errorf("count = %v, want 2", data["count"])
	}
	if data["remainingSize"].(float64) != 1 {
		t.Errorf("remainingSize = %v, want 1", data["remainingSize"])
	}
}

// ---------------------------------------------------------------------------
// Integration tests — exercise Durable Objects through the full engine pipeline
// ---------------------------------------------------------------------------

// doEnv creates an Env with a Durable Object binding wired up.
func doEnv(namespace string) *Env {
	return &Env{
		Vars:                  make(map[string]string),
		Secrets:               make(map[string]string),
		KVBindings:            make(map[string]string),
		DurableObjectBindings: map[string]string{"MY_DO": namespace},
	}
}

func TestDurableIntegration_IdFromName(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    var id = env.MY_DO.idFromName("test");
    var id2 = env.MY_DO.idFromName("test");
    var id3 = env.MY_DO.idFromName("other");
    return Response.json({
      hex: id.toString(),
      hexLen: id.toString().length,
      same: id.toString() === id2.toString(),
      different: id.toString() !== id3.toString(),
    });
  },
};`

	env := doEnv("int-ns")
	r := execJS(t, e, source, env, getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Hex       string `json:"hex"`
		HexLen    int    `json:"hexLen"`
		Same      bool   `json:"same"`
		Different bool   `json:"different"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if data.HexLen != 64 {
		t.Errorf("hexLen = %d, want 64", data.HexLen)
	}
	if !data.Same {
		t.Error("same name should produce same ID")
	}
	if !data.Different {
		t.Error("different names should produce different IDs")
	}
}

func TestDurableIntegration_GetStubFetch(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    var id = env.MY_DO.idFromName("fetch-obj");
    var stub = env.MY_DO.get(id);
    var resp = await stub.fetch("http://fake/");
    var text = await resp.text();
    return Response.json({ status: resp.status, body: text });
  },
};`

	env := doEnv("int-ns")
	r := execJS(t, e, source, env, getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Status int    `json:"status"`
		Body   string `json:"body"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if data.Status != 200 {
		t.Errorf("status = %d, want 200", data.Status)
	}
	if data.Body != "ok" {
		t.Errorf("body = %q, want ok", data.Body)
	}
}

func TestDurableIntegration_StoragePutGetDelete(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    var id = env.MY_DO.idFromName("storage-obj");
    var stub = env.MY_DO.get(id);

    // put and get
    await stub.storage.put("greeting", "hello world");
    var val = await stub.storage.get("greeting");

    // put object
    await stub.storage.put("data", { name: "alice", age: 30 });
    var obj = await stub.storage.get("data");

    // delete
    await stub.storage.put("temp", "gone");
    await stub.storage.delete("temp");
    var afterDel = await stub.storage.get("temp");

    return Response.json({
      val: val,
      name: obj.name,
      age: obj.age,
      afterDelNull: afterDel === null,
    });
  },
};`

	env := doEnv("int-ns")
	r := execJS(t, e, source, env, getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Val          string `json:"val"`
		Name         string `json:"name"`
		Age          int    `json:"age"`
		AfterDelNull bool   `json:"afterDelNull"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if data.Val != "hello world" {
		t.Errorf("val = %q, want hello world", data.Val)
	}
	if data.Name != "alice" {
		t.Errorf("name = %q, want alice", data.Name)
	}
	if data.Age != 30 {
		t.Errorf("age = %d, want 30", data.Age)
	}
	if !data.AfterDelNull {
		t.Error("after delete should be null")
	}
}

func TestDurableIntegration_StorageDeleteAllAndList(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    var id = env.MY_DO.idFromName("list-obj");
    var stub = env.MY_DO.get(id);

    await stub.storage.put("user:1", "alice");
    await stub.storage.put("user:2", "bob");
    await stub.storage.put("other:1", "nope");

    var all = await stub.storage.list();
    var prefixed = await stub.storage.list({ prefix: "user:" });

    await stub.storage.deleteAll();
    var afterClear = await stub.storage.list();

    return Response.json({
      allSize: all.size,
      prefixedSize: prefixed.size,
      afterClearSize: afterClear.size,
    });
  },
};`

	env := doEnv("int-ns")
	r := execJS(t, e, source, env, getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		AllSize        int `json:"allSize"`
		PrefixedSize   int `json:"prefixedSize"`
		AfterClearSize int `json:"afterClearSize"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if data.AllSize != 3 {
		t.Errorf("allSize = %d, want 3", data.AllSize)
	}
	if data.PrefixedSize != 2 {
		t.Errorf("prefixedSize = %d, want 2", data.PrefixedSize)
	}
	if data.AfterClearSize != 0 {
		t.Errorf("afterClearSize = %d, want 0", data.AfterClearSize)
	}
}

func TestDurableIntegration_NoBinding(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	// Worker tries to access env.MY_DO when no binding is configured.
	source := `export default {
  async fetch(request, env) {
    var hasDO = typeof env.MY_DO !== 'undefined';
    return Response.json({ hasDO: hasDO });
  },
};`

	env := defaultEnv()
	r := execJS(t, e, source, env, getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		HasDO bool `json:"hasDO"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if data.HasDO {
		t.Error("env.MY_DO should not exist when no binding is configured")
	}
}

func TestDurableIntegration_MultipleBindings(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    // Two separate DO namespaces
    var id1 = env.DO_A.idFromName("shared-name");
    var id2 = env.DO_B.idFromName("shared-name");

    var stub1 = env.DO_A.get(id1);
    var stub2 = env.DO_B.get(id2);

    await stub1.storage.put("key", "from-A");
    await stub2.storage.put("key", "from-B");

    var val1 = await stub1.storage.get("key");
    var val2 = await stub2.storage.get("key");

    return Response.json({
      val1: val1,
      val2: val2,
      isolated: val1 !== val2,
    });
  },
};`

	env := &Env{
		Vars:                  make(map[string]string),
		Secrets:               make(map[string]string),
		KVBindings:            make(map[string]string),
		DurableObjectBindings: map[string]string{"DO_A": "ns-alpha", "DO_B": "ns-beta"},
	}
	r := execJS(t, e, source, env, getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Val1     string `json:"val1"`
		Val2     string `json:"val2"`
		Isolated bool   `json:"isolated"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if data.Val1 != "from-A" {
		t.Errorf("val1 = %q, want from-A", data.Val1)
	}
	if data.Val2 != "from-B" {
		t.Errorf("val2 = %q, want from-B", data.Val2)
	}
	if !data.Isolated {
		t.Error("different namespaces should have isolated storage")
	}
}

// ---------------------------------------------------------------------------
// Edge-case tests — exercise rejection/validation branches in buildDurableObjectStorage
// ---------------------------------------------------------------------------

func TestDurableIntegration_StorageGetNoArgs(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    var id = env.MY_DO.idFromName("edge-get");
    var stub = env.MY_DO.get(id);
    try {
      await stub.storage.get();
    } catch (e) {
      return Response.json({ rejected: true, msg: String(e) });
    }
    return Response.json({ rejected: false });
  },
};`
	r := execJS(t, e, source, doEnv("edge-ns"), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Rejected bool   `json:"rejected"`
		Msg      string `json:"msg"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatal(err)
	}
	if !data.Rejected {
		t.Error("storage.get() with no args should reject")
	}
}

func TestDurableIntegration_StoragePutNoArgs(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    var id = env.MY_DO.idFromName("edge-put");
    var stub = env.MY_DO.get(id);
    try {
      await stub.storage.put();
    } catch (e) {
      return Response.json({ rejected: true, msg: String(e) });
    }
    return Response.json({ rejected: false });
  },
};`
	r := execJS(t, e, source, doEnv("edge-ns"), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Rejected bool   `json:"rejected"`
		Msg      string `json:"msg"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatal(err)
	}
	if !data.Rejected {
		t.Error("storage.put() with no args should reject")
	}
}

func TestDurableIntegration_StoragePutNonObject(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	// storage.put(42) — single non-object arg should reject
	source := `export default {
  async fetch(request, env) {
    var id = env.MY_DO.idFromName("edge-putno");
    var stub = env.MY_DO.get(id);
    try {
      await stub.storage.put(42);
    } catch (e) {
      return Response.json({ rejected: true, msg: String(e) });
    }
    return Response.json({ rejected: false });
  },
};`
	r := execJS(t, e, source, doEnv("edge-ns"), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Rejected bool   `json:"rejected"`
		Msg      string `json:"msg"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatal(err)
	}
	if !data.Rejected {
		t.Error("storage.put(42) should reject")
	}
}

func TestDurableIntegration_StorageDeleteNoArgs(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    var id = env.MY_DO.idFromName("edge-del");
    var stub = env.MY_DO.get(id);
    try {
      await stub.storage.delete();
    } catch (e) {
      return Response.json({ rejected: true, msg: String(e) });
    }
    return Response.json({ rejected: false });
  },
};`
	r := execJS(t, e, source, doEnv("edge-ns"), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Rejected bool   `json:"rejected"`
		Msg      string `json:"msg"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatal(err)
	}
	if !data.Rejected {
		t.Error("storage.delete() with no args should reject")
	}
}

func TestDurableIntegration_StorageListWithOptions(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	// Exercises the options-extraction branch in buildDurableObjectStorage list()
	// including prefix, limit, and reverse parameters. Verifies reverse ordering.
	source := `export default {
  async fetch(request, env) {
    var id = env.MY_DO.idFromName("list-opts");
    var stub = env.MY_DO.get(id);
    await stub.storage.put("a", 1);
    await stub.storage.put("b", 2);
    await stub.storage.put("c", 3);

    var all = await stub.storage.list();
    var limited = await stub.storage.list({ limit: 2 });
    var reversed = await stub.storage.list({ reverse: true });
    var prefixed = await stub.storage.list({ prefix: "a" });

    var fwdKeys = Array.from(all.keys());
    var revKeys = Array.from(reversed.keys());

    return Response.json({
      allSize: all.size,
      limitedSize: limited.size,
      reversedSize: reversed.size,
      prefixedSize: prefixed.size,
      fwdFirst: fwdKeys[0],
      fwdLast: fwdKeys[fwdKeys.length - 1],
      revFirst: revKeys[0],
      revLast: revKeys[revKeys.length - 1],
    });
  },
};`
	r := execJS(t, e, source, doEnv("edge-ns"), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		AllSize      int    `json:"allSize"`
		LimitedSize  int    `json:"limitedSize"`
		ReversedSize int    `json:"reversedSize"`
		PrefixedSize int    `json:"prefixedSize"`
		FwdFirst     string `json:"fwdFirst"`
		FwdLast      string `json:"fwdLast"`
		RevFirst     string `json:"revFirst"`
		RevLast      string `json:"revLast"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatal(err)
	}
	if data.AllSize != 3 {
		t.Errorf("allSize = %d, want 3", data.AllSize)
	}
	if data.LimitedSize != 2 {
		t.Errorf("limitedSize = %d, want 2", data.LimitedSize)
	}
	if data.ReversedSize != 3 {
		t.Errorf("reversedSize = %d, want 3", data.ReversedSize)
	}
	if data.PrefixedSize != 1 {
		t.Errorf("prefixedSize = %d, want 1", data.PrefixedSize)
	}
	if data.FwdFirst != "a" {
		t.Errorf("forward first key = %q, want a", data.FwdFirst)
	}
	if data.FwdLast != "c" {
		t.Errorf("forward last key = %q, want c", data.FwdLast)
	}
	if data.RevFirst != "c" {
		t.Errorf("reverse first key = %q, want c", data.RevFirst)
	}
	if data.RevLast != "a" {
		t.Errorf("reverse last key = %q, want a", data.RevLast)
	}
}

// TestDurableIntegration_CombinedBindings exercises buildEnvObject with KV + D1 + DO together.
func TestDurableIntegration_CombinedBindings(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    var hasKV = typeof env.MY_KV !== 'undefined' && typeof env.MY_KV.get === 'function';
    var hasD1 = typeof env.MY_DB !== 'undefined' && typeof env.MY_DB.exec === 'function';
    var hasDO = typeof env.MY_DO !== 'undefined' && typeof env.MY_DO.idFromName === 'function';
    var hasQueue = typeof env.MY_QUEUE !== 'undefined' && typeof env.MY_QUEUE.send === 'function';
    return Response.json({ hasKV, hasD1, hasDO, hasQueue });
  },
};`

	env := &Env{
		Vars:                  map[string]string{"FOO": "bar"},
		Secrets:               map[string]string{"SECRET": "shh"},
		KVBindings:            map[string]string{"MY_KV": "kv-ns"},
		QueueBindings:         map[string]string{"MY_QUEUE": "q1"},
		D1Bindings:            map[string]string{"MY_DB": "db1"},
		DurableObjectBindings: map[string]string{"MY_DO": "do-ns"},
	}
	r := execJS(t, e, source, env, getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		HasKV    bool `json:"hasKV"`
		HasD1    bool `json:"hasD1"`
		HasDO    bool `json:"hasDO"`
		HasQueue bool `json:"hasQueue"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatal(err)
	}
	if !data.HasKV {
		t.Error("should have KV binding")
	}
	if !data.HasD1 {
		t.Error("should have D1 binding")
	}
	if !data.HasDO {
		t.Error("should have DO binding")
	}
	if !data.HasQueue {
		t.Error("should have Queue binding")
	}
}
