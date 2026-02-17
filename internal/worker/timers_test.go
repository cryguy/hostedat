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
	json.Unmarshal(r.Response.Body, &data)

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
	json.Unmarshal(r.Response.Body, &data)

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
	json.Unmarshal(r.Response.Body, &data)

	if !data.IsNumber {
		t.Error("setTimeout should return a number")
	}
	if !data.Different {
		t.Error("consecutive setTimeout calls should return different IDs")
	}
}

func TestTimers_SetIntervalAndClear(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    let count = 0;
    const id = setInterval(() => { count++; }, 0);
    // Let it tick a few times.
    await new Promise(r => setTimeout(r, 0));
    await new Promise(r => setTimeout(r, 0));
    await new Promise(r => setTimeout(r, 0));
    clearInterval(id);
    const afterClear = count;
    await new Promise(r => setTimeout(r, 0));
    return Response.json({ afterClear, afterWait: count });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		AfterClear int `json:"afterClear"`
		AfterWait  int `json:"afterWait"`
	}
	json.Unmarshal(r.Response.Body, &data)

	if data.AfterClear < 1 {
		t.Errorf("interval should have fired at least once, count = %d", data.AfterClear)
	}
	if data.AfterWait != data.AfterClear {
		t.Errorf("count should not increase after clearInterval: afterClear=%d, afterWait=%d",
			data.AfterClear, data.AfterWait)
	}
}
