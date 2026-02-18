package worker

import (
	"encoding/json"
	"testing"
)

func TestURLSearchParams_Set(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request, env) {
    const p = new URLSearchParams("a=1&b=2&a=3");
    p.set("a", "99");
    return Response.json({ result: p.toString() });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Result string `json:"result"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if data.Result != "a=99&b=2" {
		t.Errorf("set: got %q, want %q", data.Result, "a=99&b=2")
	}
}

func TestURLSearchParams_Append(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request, env) {
    const p = new URLSearchParams("a=1&b=2");
    p.append("c", "3");
    return Response.json({ result: p.toString() });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Result string `json:"result"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if data.Result != "a=1&b=2&c=3" {
		t.Errorf("append: got %q, want %q", data.Result, "a=1&b=2&c=3")
	}
}

func TestURLSearchParams_Delete(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request, env) {
    const p = new URLSearchParams("a=1&b=2&a=3");
    p.delete("a");
    return Response.json({ result: p.toString(), hasA: p.has("a"), hasB: p.has("b") });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Result string `json:"result"`
		HasA   bool   `json:"hasA"`
		HasB   bool   `json:"hasB"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if data.Result != "b=2" {
		t.Errorf("delete: got %q, want %q", data.Result, "b=2")
	}
	if data.HasA {
		t.Error("has('a') should be false after delete")
	}
	if !data.HasB {
		t.Error("has('b') should still be true")
	}
}

func TestURLSearchParams_DeleteNonexistent(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request, env) {
    const p = new URLSearchParams("a=1");
    p.delete("nonexistent");
    return Response.json({ result: p.toString() });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Result string `json:"result"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if data.Result != "a=1" {
		t.Errorf("delete nonexistent: got %q, want %q", data.Result, "a=1")
	}
}

func TestURLSearchParams_GetAll(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request, env) {
    const q = new URLSearchParams("x=1&x=2&x=3");
    const all = q.getAll("x");
    const missing = q.getAll("nope");
    return Response.json({ all: JSON.stringify(all), missing: JSON.stringify(missing) });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		All     string `json:"all"`
		Missing string `json:"missing"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if data.All != `["1","2","3"]` {
		t.Errorf("getAll: got %q, want %q", data.All, `["1","2","3"]`)
	}
	if data.Missing != `[]` {
		t.Errorf("getAll missing: got %q, want %q", data.Missing, `[]`)
	}
}

func TestURLSearchParams_Sort(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request, env) {
    const s = new URLSearchParams("c=3&a=1&b=2&a=0");
    s.sort();
    return Response.json({ result: s.toString() });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Result string `json:"result"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if data.Result != "a=1&a=0&b=2&c=3" {
		t.Errorf("sort stable: got %q, want %q", data.Result, "a=1&a=0&b=2&c=3")
	}
}

func TestURLSearchParams_SetOnEmpty(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request, env) {
    const p = new URLSearchParams();
    p.set("a", "1");
    return Response.json({ result: p.toString() });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Result string `json:"result"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if data.Result != "a=1" {
		t.Errorf("set on empty: got %q, want %q", data.Result, "a=1")
	}
}

func TestURLSearchParams_URLIntegration(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request, env) {
    const url = new URL("https://example.com/?foo=bar");
    url.searchParams.set("foo", "baz");
    url.searchParams.append("key", "val");
    return Response.json({
      search: url.search,
      hrefHasParams: url.href.includes("foo=baz&key=val"),
    });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Search        string `json:"search"`
		HrefHasParams bool   `json:"hrefHasParams"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if data.Search != "?foo=baz&key=val" {
		t.Errorf("URL.search = %q, want %q", data.Search, "?foo=baz&key=val")
	}
	if !data.HrefHasParams {
		t.Error("URL.href should reflect searchParams mutations")
	}
}

func TestURLSearchParams_CombinedMutations(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request, env) {
    const p = new URLSearchParams("a=1&b=2&a=3");
    p.set("a", "99");
    p.append("c", "4");
    p.delete("b");
    return Response.json({ result: p.toString() });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Result string `json:"result"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if data.Result != "a=99&c=4" {
		t.Errorf("combined: got %q, want %q", data.Result, "a=99&c=4")
	}
}
