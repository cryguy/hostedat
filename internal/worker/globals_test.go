package worker

import (
	"encoding/json"
	"testing"
)

func TestGlobals_StructuredClone(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request, env) {
    const orig = { a: 1, b: { c: [2, 3] } };
    const cloned = structuredClone(orig);
    // Mutate original — clone should be independent.
    orig.b.c.push(4);
    return Response.json({
      origLen: orig.b.c.length,
      clonedLen: cloned.b.c.length,
      clonedA: cloned.a,
    });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		OrigLen   int `json:"origLen"`
		ClonedLen int `json:"clonedLen"`
		ClonedA   int `json:"clonedA"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if data.OrigLen != 3 {
		t.Errorf("origLen = %d, want 3", data.OrigLen)
	}
	if data.ClonedLen != 2 {
		t.Errorf("clonedLen = %d, want 2 (should be independent)", data.ClonedLen)
	}
	if data.ClonedA != 1 {
		t.Errorf("clonedA = %d, want 1", data.ClonedA)
	}
}

func TestGlobals_StructuredCloneClonesMap(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request, env) {
    const orig = new Map([["a", 1], ["b", 2]]);
    const cloned = structuredClone(orig);
    orig.set("c", 3);
    return Response.json({
      isMap: cloned instanceof Map,
      size: cloned.size,
      a: cloned.get("a"),
      hasC: cloned.has("c"),
    });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		IsMap bool `json:"isMap"`
		Size  int  `json:"size"`
		A     int  `json:"a"`
		HasC  bool `json:"hasC"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if !data.IsMap {
		t.Error("cloned value should be a Map")
	}
	if data.Size != 2 {
		t.Errorf("cloned map size = %d, want 2", data.Size)
	}
	if data.A != 1 {
		t.Errorf("cloned map get('a') = %d, want 1", data.A)
	}
	if data.HasC {
		t.Error("cloned map should not have 'c' (added after clone)")
	}
}

func TestGlobals_StructuredCloneRejectsFunction(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request, env) {
    try {
      structuredClone(function() {});
      return Response.json({ threw: false });
    } catch(e) {
      return Response.json({ threw: true, name: e.name });
    }
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Threw bool   `json:"threw"`
		Name  string `json:"name"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if !data.Threw {
		t.Error("structuredClone(function) should throw DataCloneError")
	}
}

func TestGlobals_PerformanceNow(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request, env) {
    const t1 = performance.now();
    // Do some work.
    let sum = 0;
    for (let i = 0; i < 10000; i++) sum += i;
    const t2 = performance.now();
    return Response.json({
      t1Type: typeof t1,
      t2Type: typeof t2,
      positive: t1 >= 0,
      elapsed: t2 >= t1,
    });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		T1Type  string `json:"t1Type"`
		T2Type  string `json:"t2Type"`
		Pos     bool   `json:"positive"`
		Elapsed bool   `json:"elapsed"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if data.T1Type != "number" {
		t.Errorf("t1Type = %q, want number", data.T1Type)
	}
	if !data.Pos {
		t.Error("performance.now() should return non-negative")
	}
	if !data.Elapsed {
		t.Error("t2 should be >= t1")
	}
}

func TestGlobals_Navigator(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request, env) {
    return Response.json({
      ua: navigator.userAgent,
      hasNavigator: typeof navigator === 'object',
    });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		UA  string `json:"ua"`
		Has bool   `json:"hasNavigator"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if !data.Has {
		t.Error("navigator should be an object")
	}
	if data.UA != "hostedat-worker/1.0" {
		t.Errorf("userAgent = %q", data.UA)
	}
}

func TestGlobals_StructuredCloneRejectsUndefined(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request, env) {
    try {
      structuredClone(undefined);
      return Response.json({ threw: false });
    } catch(e) {
      return Response.json({ threw: true, name: e.name });
    }
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Threw bool   `json:"threw"`
		Name  string `json:"name"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatal(err)
	}
	if !data.Threw {
		t.Error("structuredClone(undefined) should throw")
	}
}

func TestGlobals_StructuredClonePrimitives(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request, env) {
    return Response.json({
      num: structuredClone(42),
      str: structuredClone("hello"),
      bool: structuredClone(true),
      nil: structuredClone(null),
    });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Num  int    `json:"num"`
		Str  string `json:"str"`
		Bool bool   `json:"bool"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatal(err)
	}
	if data.Num != 42 {
		t.Errorf("num = %d, want 42", data.Num)
	}
	if data.Str != "hello" {
		t.Errorf("str = %q, want hello", data.Str)
	}
	if !data.Bool {
		t.Error("bool should be true")
	}
}

// Pure Go helper tests
func TestErrMissingArg(t *testing.T) {
	err := errMissingArg("foo", 2)
	if err.Error() != "foo requires at least 2 argument(s)" {
		t.Errorf("errMissingArg = %q", err.Error())
	}
}

func TestErrInvalidArg(t *testing.T) {
	err := errInvalidArg("bar", "must be positive")
	if err.Error() != "bar: must be positive" {
		t.Errorf("errInvalidArg = %q", err.Error())
	}
}

func TestGlobals_QueueMicrotask(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    let called = false;
    queueMicrotask(() => { called = true; });
    // queueMicrotask uses Promise.resolve().then(), so await to let it run.
    await new Promise(r => r());
    return Response.json({ called });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Called bool `json:"called"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if !data.Called {
		t.Error("queueMicrotask callback was not called")
	}
}

func TestGlobals_StructuredCloneClonesSet(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request, env) {
    const orig = new Set([1, 2, 3]);
    const cloned = structuredClone(orig);
    orig.add(4);
    return Response.json({
      isSet: cloned instanceof Set,
      size: cloned.size,
      has1: cloned.has(1),
      has4: cloned.has(4),
    });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		IsSet bool `json:"isSet"`
		Size  int  `json:"size"`
		Has1  bool `json:"has1"`
		Has4  bool `json:"has4"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatal(err)
	}
	if !data.IsSet {
		t.Error("cloned value should be a Set")
	}
	if data.Size != 3 {
		t.Errorf("cloned set size = %d, want 3", data.Size)
	}
	if !data.Has1 {
		t.Error("cloned set should have 1")
	}
	if data.Has4 {
		t.Error("cloned set should not have 4 (added after clone)")
	}
}

func TestGlobals_StructuredCloneRejectsWeakMap(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request, env) {
    try {
      structuredClone(new WeakMap());
      return Response.json({ threw: false });
    } catch(e) {
      return Response.json({ threw: true, name: e.name });
    }
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Threw bool `json:"threw"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatal(err)
	}
	if !data.Threw {
		t.Error("structuredClone(WeakMap) should throw DataCloneError")
	}
}

func TestGlobals_StructuredCloneRejectsWeakSet(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request, env) {
    try {
      structuredClone(new WeakSet());
      return Response.json({ threw: false });
    } catch(e) {
      return Response.json({ threw: true, name: e.name });
    }
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Threw bool `json:"threw"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatal(err)
	}
	if !data.Threw {
		t.Error("structuredClone(WeakSet) should throw DataCloneError")
	}
}

func TestGlobals_StructuredCloneRejectsSymbol(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request, env) {
    try {
      structuredClone(Symbol("test"));
      return Response.json({ threw: false });
    } catch(e) {
      return Response.json({ threw: true, name: e.name });
    }
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Threw bool `json:"threw"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatal(err)
	}
	if !data.Threw {
		t.Error("structuredClone(Symbol) should throw DataCloneError")
	}
}

func TestGlobals_StructuredCloneCircularThrows(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request, env) {
    try {
      var obj = {};
      obj.self = obj;
      structuredClone(obj);
      return Response.json({ threw: false });
    } catch(e) {
      return Response.json({ threw: true, name: e.name });
    }
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Threw bool `json:"threw"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatal(err)
	}
	if !data.Threw {
		t.Error("structuredClone with circular reference should throw DataCloneError")
	}
}

func TestGlobals_StructuredCloneDate(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request, env) {
    const orig = new Date('2024-01-15T10:30:00Z');
    const cloned = structuredClone(orig);
    return Response.json({
      isDate: cloned instanceof Date,
      time: cloned.getTime(),
      origTime: orig.getTime(),
      equal: cloned.getTime() === orig.getTime(),
    });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		IsDate bool  `json:"isDate"`
		Time   int64 `json:"time"`
		Equal  bool  `json:"equal"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatal(err)
	}
	if !data.IsDate {
		t.Error("cloned value should be a Date")
	}
	if !data.Equal {
		t.Error("cloned Date should have same time as original")
	}
}

func TestGlobals_StructuredCloneRegExp(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request, env) {
    const orig = /hello\s+world/gi;
    const cloned = structuredClone(orig);
    return Response.json({
      isRegExp: cloned instanceof RegExp,
      source: cloned.source,
      flags: cloned.flags,
      equal: cloned.source === orig.source && cloned.flags === orig.flags,
    });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		IsRegExp bool   `json:"isRegExp"`
		Source   string `json:"source"`
		Flags    string `json:"flags"`
		Equal    bool   `json:"equal"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatal(err)
	}
	if !data.IsRegExp {
		t.Error("cloned value should be a RegExp")
	}
	if data.Source != `hello\s+world` {
		t.Errorf("source = %q", data.Source)
	}
	if !data.Equal {
		t.Error("cloned RegExp should match original source and flags")
	}
}

func TestGlobals_StructuredCloneTypedArray(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request, env) {
    const orig = new Uint8Array([10, 20, 30, 40]);
    const cloned = structuredClone(orig);
    orig[0] = 99;
    return Response.json({
      isUint8: cloned instanceof Uint8Array,
      length: cloned.length,
      first: cloned[0],
      origFirst: orig[0],
      independent: cloned[0] !== orig[0],
    });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		IsUint8     bool `json:"isUint8"`
		Length      int  `json:"length"`
		First       int  `json:"first"`
		Independent bool `json:"independent"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatal(err)
	}
	if !data.IsUint8 {
		t.Error("cloned value should be a Uint8Array")
	}
	if data.Length != 4 {
		t.Errorf("length = %d, want 4", data.Length)
	}
	if data.First != 10 {
		t.Errorf("first = %d, want 10 (original was mutated to 99)", data.First)
	}
	if !data.Independent {
		t.Error("cloned TypedArray should be independent of original")
	}
}

func TestGlobals_StructuredCloneArrayBuffer(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request, env) {
    const orig = new ArrayBuffer(4);
    const view = new Uint8Array(orig);
    view[0] = 1; view[1] = 2; view[2] = 3; view[3] = 4;
    const cloned = structuredClone(orig);
    view[0] = 99;
    const clonedView = new Uint8Array(cloned);
    return Response.json({
      isAB: cloned instanceof ArrayBuffer,
      byteLength: cloned.byteLength,
      first: clonedView[0],
      independent: clonedView[0] !== view[0],
    });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		IsAB        bool `json:"isAB"`
		ByteLength  int  `json:"byteLength"`
		First       int  `json:"first"`
		Independent bool `json:"independent"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatal(err)
	}
	if !data.IsAB {
		t.Error("cloned value should be an ArrayBuffer")
	}
	if data.ByteLength != 4 {
		t.Errorf("byteLength = %d, want 4", data.ByteLength)
	}
	if data.First != 1 {
		t.Errorf("first = %d, want 1", data.First)
	}
	if !data.Independent {
		t.Error("cloned ArrayBuffer should be independent")
	}
}

func TestGlobals_StructuredCloneRejectsPromise(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request, env) {
    try {
      structuredClone(Promise.resolve(42));
      return Response.json({ threw: false });
    } catch(e) {
      return Response.json({ threw: true, name: e.name });
    }
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Threw bool   `json:"threw"`
		Name  string `json:"name"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatal(err)
	}
	if !data.Threw {
		t.Error("structuredClone(Promise) should throw DataCloneError")
	}
	if data.Name != "DataCloneError" {
		t.Errorf("error name = %q, want DataCloneError", data.Name)
	}
}

func TestGlobals_StructuredCloneNestedMapSet(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request, env) {
    const m = new Map([["items", new Set([1, 2, 3])]]);
    const cloned = structuredClone(m);
    const clonedSet = cloned.get("items");
    return Response.json({
      mapSize: cloned.size,
      isSet: clonedSet instanceof Set,
      setSize: clonedSet.size,
      has2: clonedSet.has(2),
    });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		MapSize int  `json:"mapSize"`
		IsSet   bool `json:"isSet"`
		SetSize int  `json:"setSize"`
		Has2    bool `json:"has2"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatal(err)
	}
	if data.MapSize != 1 {
		t.Errorf("map size = %d, want 1", data.MapSize)
	}
	if !data.IsSet {
		t.Error("nested value should be a Set")
	}
	if data.SetSize != 3 {
		t.Errorf("set size = %d, want 3", data.SetSize)
	}
}

// ---------------------------------------------------------------------------
// navigator.sendBeacon
// ---------------------------------------------------------------------------

func TestGlobals_NavigatorSendBeacon(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request, env) {
    const hasFn = typeof navigator.sendBeacon === 'function';
    // sendBeacon to a private address should return false (SSRF blocked)
    const result = navigator.sendBeacon("http://127.0.0.1:9999/track", "test data");
    return Response.json({ hasFn, result });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		HasFn  bool `json:"hasFn"`
		Result bool `json:"result"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatal(err)
	}
	if !data.HasFn {
		t.Error("navigator.sendBeacon should be a function")
	}
	if data.Result {
		t.Error("sendBeacon to localhost should return false (SSRF blocked)")
	}
}

func TestGlobals_NavigatorSendBeaconPublicURL(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request, env) {
    // A public URL should return true (queued, fire-and-forget).
    const result = navigator.sendBeacon("https://example.com/analytics", "event=click");
    return Response.json({ result });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Result bool `json:"result"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatal(err)
	}
	if !data.Result {
		t.Error("sendBeacon to public URL should return true")
	}
}

func TestGlobals_NavigatorSendBeaconNoData(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request, env) {
    const result = navigator.sendBeacon("https://example.com/ping");
    return Response.json({ result });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Result bool `json:"result"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatal(err)
	}
	if !data.Result {
		t.Error("sendBeacon without data should return true")
	}
}

// ---------------------------------------------------------------------------
// structuredClone: DataView, Int32Array, Float64Array
// ---------------------------------------------------------------------------

func TestGlobals_StructuredCloneDataView(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request, env) {
    const buf = new ArrayBuffer(8);
    const orig = new DataView(buf);
    orig.setInt32(0, 123456);
    const cloned = structuredClone(orig);
    orig.setInt32(0, 999999);
    return Response.json({
      isDataView: cloned instanceof DataView,
      byteLength: cloned.byteLength,
      value: cloned.getInt32(0),
      independent: cloned.getInt32(0) !== orig.getInt32(0),
    });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		IsDataView  bool `json:"isDataView"`
		ByteLength  int  `json:"byteLength"`
		Value       int  `json:"value"`
		Independent bool `json:"independent"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatal(err)
	}
	if !data.IsDataView {
		t.Error("cloned value should be a DataView")
	}
	if data.ByteLength != 8 {
		t.Errorf("byteLength = %d, want 8", data.ByteLength)
	}
	if data.Value != 123456 {
		t.Errorf("value = %d, want 123456 (original was mutated to 999999)", data.Value)
	}
	if !data.Independent {
		t.Error("cloned DataView should be independent of original")
	}
}

func TestGlobals_StructuredCloneInt32Array(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request, env) {
    const orig = new Int32Array([100, 200, -300, 400]);
    const cloned = structuredClone(orig);
    orig[0] = 999;
    return Response.json({
      isInt32: cloned instanceof Int32Array,
      length: cloned.length,
      first: cloned[0],
      third: cloned[2],
      independent: cloned[0] !== orig[0],
    });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		IsInt32     bool `json:"isInt32"`
		Length      int  `json:"length"`
		First       int  `json:"first"`
		Third       int  `json:"third"`
		Independent bool `json:"independent"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatal(err)
	}
	if !data.IsInt32 {
		t.Error("cloned value should be an Int32Array")
	}
	if data.Length != 4 {
		t.Errorf("length = %d, want 4", data.Length)
	}
	if data.First != 100 {
		t.Errorf("first = %d, want 100 (original was mutated to 999)", data.First)
	}
	if data.Third != -300 {
		t.Errorf("third = %d, want -300", data.Third)
	}
	if !data.Independent {
		t.Error("cloned Int32Array should be independent of original")
	}
}

func TestGlobals_StructuredCloneFloat64Array(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request, env) {
    const orig = new Float64Array([1.1, 2.2, 3.3]);
    const cloned = structuredClone(orig);
    orig[0] = 99.9;
    return Response.json({
      isFloat64: cloned instanceof Float64Array,
      length: cloned.length,
      first: cloned[0],
      independent: cloned[0] !== orig[0],
    });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		IsFloat64   bool    `json:"isFloat64"`
		Length      int     `json:"length"`
		First       float64 `json:"first"`
		Independent bool    `json:"independent"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatal(err)
	}
	if !data.IsFloat64 {
		t.Error("cloned value should be a Float64Array")
	}
	if data.Length != 3 {
		t.Errorf("length = %d, want 3", data.Length)
	}
	if data.First < 1.09 || data.First > 1.11 {
		t.Errorf("first = %f, want ~1.1 (original was mutated to 99.9)", data.First)
	}
	if !data.Independent {
		t.Error("cloned Float64Array should be independent of original")
	}
}
