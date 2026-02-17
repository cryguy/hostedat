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
	json.Unmarshal(r.Response.Body, &data)

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
	json.Unmarshal(r.Response.Body, &data)

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
	json.Unmarshal(r.Response.Body, &data)

	if !data.Has {
		t.Error("navigator should be an object")
	}
	if data.UA != "hostedat-worker/1.0" {
		t.Errorf("userAgent = %q", data.UA)
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
	json.Unmarshal(r.Response.Body, &data)

	if !data.Called {
		t.Error("queueMicrotask callback was not called")
	}
}
