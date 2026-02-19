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

func TestGlobals_StructuredCloneRejectsMap(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request, env) {
    try {
      structuredClone(new Map([["k", "v"]]));
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
		t.Error("structuredClone(Map) should throw DataCloneError")
	}
	if data.Name != "DataCloneError" {
		t.Errorf("error name = %q, want DataCloneError", data.Name)
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
	json.Unmarshal(r.Response.Body, &data)
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
	json.Unmarshal(r.Response.Body, &data)
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

func TestGlobals_StructuredCloneRejectsSet(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request, env) {
    try {
      structuredClone(new Set([1, 2, 3]));
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
	json.Unmarshal(r.Response.Body, &data)
	if !data.Threw {
		t.Error("structuredClone(Set) should throw DataCloneError")
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
	json.Unmarshal(r.Response.Body, &data)
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
	json.Unmarshal(r.Response.Body, &data)
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
	json.Unmarshal(r.Response.Body, &data)
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
	json.Unmarshal(r.Response.Body, &data)
	if !data.Threw {
		t.Error("structuredClone with circular reference should throw DataCloneError")
	}
}
