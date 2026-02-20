package worker

import (
	"encoding/json"
	"testing"
)

func TestTimers_SetTimeoutZero(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    let called = false;
    setTimeout(() => { called = true; }, 0);
    // Yield to microtask queue so the timer fires.
    await new Promise(r => setTimeout(r, 0));
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
		t.Error("setTimeout(fn, 0) callback was not called")
	}
}

func TestTimers_ClearTimeout(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    let called = false;
    const id = setTimeout(() => { called = true; }, 0);
    clearTimeout(id);
    await new Promise(r => setTimeout(r, 0));
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

	if data.Called {
		t.Error("clearTimeout should prevent callback from firing")
	}
}

func TestTimers_SetTimeoutReturnsID(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request, env) {
    const id1 = setTimeout(() => {}, 0);
    const id2 = setTimeout(() => {}, 0);
    return Response.json({
      isNumber: typeof id1 === 'number',
      different: id1 !== id2,
    });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		IsNumber  bool `json:"isNumber"`
		Different bool `json:"different"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if !data.IsNumber {
		t.Error("setTimeout should return a number")
	}
	if !data.Different {
		t.Error("consecutive setTimeout calls should return different IDs")
	}
}

func TestTimers_SetTimeoutNoArgsReturnsZero(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request, env) {
    const id = setTimeout();
    return Response.json({ id, isZero: id === 0 });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		ID     int  `json:"id"`
		IsZero bool `json:"isZero"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatal(err)
	}
	if !data.IsZero {
		t.Errorf("setTimeout() with no args should return 0, got %d", data.ID)
	}
}

func TestTimers_SetIntervalMinimumDelay(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request, env) {
    const id = setInterval(() => {}, 0);
    clearInterval(id);
    return Response.json({ id, positive: id > 0 });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		ID       int  `json:"id"`
		Positive bool `json:"positive"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatal(err)
	}
	if !data.Positive {
		t.Error("setInterval should return a positive ID")
	}
}

func TestTimers_ClearNonExistentTimer(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request, env) {
    clearTimeout(9999);
    clearInterval(9999);
    return new Response("ok");
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)
}

func TestTimers_SetIntervalAndClear(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    let count = 0;
    const id = setInterval(() => { count++; }, 10);
    // Wait long enough for the interval to fire several times.
    await new Promise(r => setTimeout(r, 50));
    await new Promise(r => setTimeout(r, 50));
    await new Promise(r => setTimeout(r, 50));
    clearInterval(id);
    const afterClear = count;
    await new Promise(r => setTimeout(r, 50));
    return Response.json({ afterClear, afterWait: count });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		AfterClear int `json:"afterClear"`
		AfterWait  int `json:"afterWait"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if data.AfterClear < 1 {
		t.Errorf("interval should have fired at least once, count = %d", data.AfterClear)
	}
	if data.AfterWait != data.AfterClear {
		t.Errorf("count should not increase after clearInterval: afterClear=%d, afterWait=%d",
			data.AfterClear, data.AfterWait)
	}
}

func TestTimers_SetIntervalNoArgsReturnsZero(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request, env) {
    const id = setInterval();
    return Response.json({ id, isZero: id === 0 });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		IsZero bool `json:"isZero"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatal(err)
	}
	if !data.IsZero {
		t.Error("setInterval() with no args should return 0")
	}
}

func TestTimers_SetTimeoutNonFunctionReturnsZero(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request, env) {
    const id = setTimeout("not a function", 0);
    return Response.json({ id, isZero: id === 0 });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		IsZero bool `json:"isZero"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatal(err)
	}
	if !data.IsZero {
		t.Error("setTimeout('string', 0) should return 0")
	}
}

func TestTimers_SetIntervalNonFunctionReturnsZero(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request, env) {
    const id = setInterval(42, 10);
    return Response.json({ id, isZero: id === 0 });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		IsZero bool `json:"isZero"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatal(err)
	}
	if !data.IsZero {
		t.Error("setInterval(42, 10) should return 0")
	}
}

func TestTimers_ClearTimeoutNoArgs(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request, env) {
    // clearTimeout/clearInterval with no args should not crash.
    clearTimeout();
    clearInterval();
    return new Response("ok");
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)
}

func TestTimers_SetTimeoutWithDelay(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    const start = performance.now();
    await new Promise(r => setTimeout(r, 20));
    const elapsed = performance.now() - start;
    return Response.json({ elapsed: elapsed >= 15 });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Elapsed bool `json:"elapsed"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatal(err)
	}
	if !data.Elapsed {
		t.Error("setTimeout with 20ms delay should take at least 15ms")
	}
}
