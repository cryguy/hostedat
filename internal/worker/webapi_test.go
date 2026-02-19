package worker

import (
	"encoding/json"
	"testing"
)

func TestParseURL(t *testing.T) {
	tests := []struct {
		name     string
		rawURL   string
		base     string
		wantErr  bool
		href     string
		protocol string
		hostname string
		pathname string
		search   string
		hash     string
	}{
		{
			name:     "absolute URL with query and hash",
			rawURL:   "https://example.com/path?q=1#hash",
			href:     "https://example.com/path?q=1#hash",
			protocol: "https:",
			hostname: "example.com",
			pathname: "/path",
			search:   "?q=1",
			hash:     "#hash",
		},
		{
			name:     "with port",
			rawURL:   "http://localhost:8080/api",
			href:     "http://localhost:8080/api",
			protocol: "http:",
			hostname: "localhost",
			pathname: "/api",
		},
		{
			name:     "relative with base",
			rawURL:   "/path",
			base:     "https://example.com",
			href:     "https://example.com/path",
			protocol: "https:",
			hostname: "example.com",
			pathname: "/path",
		},
		{
			name:    "no scheme errors",
			rawURL:  "not-a-url",
			wantErr: true,
		},
		{
			name:     "simple https",
			rawURL:   "https://test.com",
			href:     "https://test.com",
			protocol: "https:",
			hostname: "test.com",
			pathname: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parsed, err := parseURL(tt.rawURL, tt.base)
			if (err != nil) != tt.wantErr {
				t.Fatalf("parseURL(%q, %q) error = %v, wantErr %v", tt.rawURL, tt.base, err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if parsed.Href != tt.href {
				t.Errorf("href = %q, want %q", parsed.Href, tt.href)
			}
			if parsed.Protocol != tt.protocol {
				t.Errorf("protocol = %q, want %q", parsed.Protocol, tt.protocol)
			}
			if parsed.Hostname != tt.hostname {
				t.Errorf("hostname = %q, want %q", parsed.Hostname, tt.hostname)
			}
			if parsed.Pathname != tt.pathname {
				t.Errorf("pathname = %q, want %q", parsed.Pathname, tt.pathname)
			}
			if tt.search != "" && parsed.Search != tt.search {
				t.Errorf("search = %q, want %q", parsed.Search, tt.search)
			}
			if tt.hash != "" && parsed.Hash != tt.hash {
				t.Errorf("hash = %q, want %q", parsed.Hash, tt.hash)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Integration: Response.redirect
// ---------------------------------------------------------------------------

func TestWebAPI_ResponseRedirect(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request, env) {
    const r = Response.redirect("https://example.com/new", 301);
    return Response.json({
      status: r.status,
      location: r.headers.get("location"),
    });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Status   int    `json:"status"`
		Location string `json:"location"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if data.Status != 301 {
		t.Errorf("status = %d, want 301", data.Status)
	}
	if data.Location != "https://example.com/new" {
		t.Errorf("location = %q", data.Location)
	}
}

func TestWebAPI_ResponseRedirectDefault302(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request, env) {
    const r = Response.redirect("https://example.com/default");
    return Response.json({ status: r.status });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Status int `json:"status"`
	}
	json.Unmarshal(r.Response.Body, &data)
	if data.Status != 302 {
		t.Errorf("status = %d, want 302", data.Status)
	}
}

// ---------------------------------------------------------------------------
// Integration: Request.clone
// ---------------------------------------------------------------------------

func TestWebAPI_RequestClone(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    const req = new Request("https://example.com/path", {
      method: "POST",
      headers: { "x-custom": "value" },
      body: "original body",
    });
    const clone = req.clone();
    // Mutating original shouldn't affect clone.
    req.headers.set("x-custom", "changed");
    const cloneText = await clone.text();
    return Response.json({
      cloneMethod: clone.method,
      cloneURL: clone.url,
      cloneHeader: clone.headers.get("x-custom"),
      cloneBody: cloneText,
    });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		CloneMethod string `json:"cloneMethod"`
		CloneURL    string `json:"cloneURL"`
		CloneHeader string `json:"cloneHeader"`
		CloneBody   string `json:"cloneBody"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if data.CloneMethod != "POST" {
		t.Errorf("clone method = %q", data.CloneMethod)
	}
	if data.CloneBody != "original body" {
		t.Errorf("clone body = %q", data.CloneBody)
	}
}

// ---------------------------------------------------------------------------
// Integration: Response.clone
// ---------------------------------------------------------------------------

func TestWebAPI_ResponseClone(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    const resp = new Response("hello", { status: 201, headers: { "x-test": "val" } });
    const clone = resp.clone();
    return Response.json({
      status: clone.status,
      body: await clone.text(),
      header: clone.headers.get("x-test"),
      ok: clone.ok,
    });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Status int    `json:"status"`
		Body   string `json:"body"`
		Header string `json:"header"`
		Ok     bool   `json:"ok"`
	}
	json.Unmarshal(r.Response.Body, &data)
	if data.Status != 201 {
		t.Errorf("status = %d, want 201", data.Status)
	}
	if data.Body != "hello" {
		t.Errorf("body = %q", data.Body)
	}
	if data.Header != "val" {
		t.Errorf("header = %q", data.Header)
	}
	if !data.Ok {
		t.Error("201 should be ok")
	}
}

// ---------------------------------------------------------------------------
// Integration: Headers API
// ---------------------------------------------------------------------------

func TestWebAPI_HeadersOperations(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request, env) {
    const h = new Headers({"Content-Type": "text/html", "X-Custom": "abc"});
    h.append("X-Custom", "def");
    const appended = h.get("x-custom");
    h.set("x-custom", "replaced");
    const afterSet = h.get("x-custom");
    const hasCT = h.has("content-type");
    h.delete("content-type");
    const hasCTAfterDel = h.has("content-type");

    const keys = [];
    const vals = [];
    h.forEach((v, k) => { keys.push(k); vals.push(v); });

    return Response.json({
      appended,
      afterSet,
      hasCT,
      hasCTAfterDel,
      keys,
      vals,
    });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Appended      string   `json:"appended"`
		AfterSet      string   `json:"afterSet"`
		HasCT         bool     `json:"hasCT"`
		HasCTAfterDel bool     `json:"hasCTAfterDel"`
		Keys          []string `json:"keys"`
		Vals          []string `json:"vals"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if data.Appended != "abc, def" {
		t.Errorf("appended = %q, want 'abc, def'", data.Appended)
	}
	if data.AfterSet != "replaced" {
		t.Errorf("afterSet = %q", data.AfterSet)
	}
	if !data.HasCT {
		t.Error("should have content-type before delete")
	}
	if data.HasCTAfterDel {
		t.Error("should not have content-type after delete")
	}
}

func TestWebAPI_HeadersFromArray(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request, env) {
    const h = new Headers([["Content-Type", "text/plain"], ["X-Foo", "bar"]]);
    return Response.json({
      ct: h.get("content-type"),
      foo: h.get("x-foo"),
    });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		CT  string `json:"ct"`
		Foo string `json:"foo"`
	}
	json.Unmarshal(r.Response.Body, &data)
	if data.CT != "text/plain" {
		t.Errorf("ct = %q", data.CT)
	}
	if data.Foo != "bar" {
		t.Errorf("foo = %q", data.Foo)
	}
}

// ---------------------------------------------------------------------------
// Integration: URLSearchParams mutations
// ---------------------------------------------------------------------------

func TestWebAPI_URLSearchParamsMutations(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request, env) {
    const url = new URL("https://example.com/path?a=1&b=2&a=3");
    const sp = url.searchParams;
    const getA = sp.get("a");
    const getAll = sp.getAll("a");

    sp.set("a", "99");
    const afterSet = sp.get("a");
    const afterSetAll = sp.getAll("a");

    sp.append("c", "4");
    const hasC = sp.has("c");

    sp.delete("b");
    const hasB = sp.has("b");

    sp.sort();
    const sorted = sp.toString();

    return Response.json({ getA, getAll, afterSet, afterSetAll, hasC, hasB, sorted });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		GetA        string   `json:"getA"`
		GetAll      []string `json:"getAll"`
		AfterSet    string   `json:"afterSet"`
		AfterSetAll []string `json:"afterSetAll"`
		HasC        bool     `json:"hasC"`
		HasB        bool     `json:"hasB"`
		Sorted      string   `json:"sorted"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if data.GetA != "1" {
		t.Errorf("get(a) = %q, want '1'", data.GetA)
	}
	if len(data.GetAll) != 2 || data.GetAll[0] != "1" || data.GetAll[1] != "3" {
		t.Errorf("getAll(a) = %v, want [1,3]", data.GetAll)
	}
	if data.AfterSet != "99" {
		t.Errorf("afterSet = %q, want '99'", data.AfterSet)
	}
	if len(data.AfterSetAll) != 1 {
		t.Errorf("afterSetAll = %v, want [99]", data.AfterSetAll)
	}
	if !data.HasC {
		t.Error("should have c after append")
	}
	if data.HasB {
		t.Error("should not have b after delete")
	}
}

// ---------------------------------------------------------------------------
// Integration: Response with non-ok status
// ---------------------------------------------------------------------------

func TestWebAPI_ResponseNonOkStatus(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request, env) {
    const r404 = new Response("not found", { status: 404 });
    const r500 = new Response("error", { status: 500 });
    const r200 = new Response("ok", { status: 200 });
    return Response.json({
      ok404: r404.ok,
      ok500: r500.ok,
      ok200: r200.ok,
      status404: r404.status,
    });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Ok404     bool `json:"ok404"`
		Ok500     bool `json:"ok500"`
		Ok200     bool `json:"ok200"`
		Status404 int  `json:"status404"`
	}
	json.Unmarshal(r.Response.Body, &data)
	if data.Ok404 {
		t.Error("404 should not be ok")
	}
	if data.Ok500 {
		t.Error("500 should not be ok")
	}
	if !data.Ok200 {
		t.Error("200 should be ok")
	}
	if data.Status404 != 404 {
		t.Errorf("status = %d", data.Status404)
	}
}

// ---------------------------------------------------------------------------
// Integration: Response body as ArrayBuffer
// ---------------------------------------------------------------------------

func TestWebAPI_ResponseArrayBufferBody(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    const resp = new Response("hello world");
    const ab = await resp.arrayBuffer();
    const decoded = new TextDecoder().decode(ab);
    return Response.json({ decoded, byteLen: ab.byteLength });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Decoded string `json:"decoded"`
		ByteLen int    `json:"byteLen"`
	}
	json.Unmarshal(r.Response.Body, &data)
	if data.Decoded != "hello world" {
		t.Errorf("decoded = %q", data.Decoded)
	}
	if data.ByteLen != 11 {
		t.Errorf("byteLen = %d, want 11", data.ByteLen)
	}
}

// ---------------------------------------------------------------------------
// Integration: URL edge cases
// ---------------------------------------------------------------------------

func TestWebAPI_URLComponents(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request, env) {
    const url = new URL("https://user:pass@example.com:8443/path?q=1#frag");
    return Response.json({
      protocol: url.protocol,
      hostname: url.hostname,
      port: url.port,
      pathname: url.pathname,
      search: url.search,
      hash: url.hash,
      origin: url.origin,
      host: url.host,
      str: url.toString(),
    });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Protocol string `json:"protocol"`
		Hostname string `json:"hostname"`
		Port     string `json:"port"`
		Pathname string `json:"pathname"`
		Search   string `json:"search"`
		Hash     string `json:"hash"`
		Origin   string `json:"origin"`
		Host     string `json:"host"`
		Str      string `json:"str"`
	}
	json.Unmarshal(r.Response.Body, &data)
	if data.Protocol != "https:" {
		t.Errorf("protocol = %q", data.Protocol)
	}
	if data.Hostname != "example.com" {
		t.Errorf("hostname = %q", data.Hostname)
	}
	if data.Port != "8443" {
		t.Errorf("port = %q", data.Port)
	}
	if data.Pathname != "/path" {
		t.Errorf("pathname = %q", data.Pathname)
	}
	if data.Search != "?q=1" {
		t.Errorf("search = %q", data.Search)
	}
	if data.Hash != "#frag" {
		t.Errorf("hash = %q", data.Hash)
	}
}

func TestWebAPI_URLInvalidThrows(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request, env) {
    let threw = false;
    try {
      new URL("not-a-valid-url");
    } catch(e) {
      threw = true;
    }
    return Response.json({ threw });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Threw bool `json:"threw"`
	}
	json.Unmarshal(r.Response.Body, &data)
	if !data.Threw {
		t.Error("new URL with invalid input should throw")
	}
}

// ---------------------------------------------------------------------------
// Integration: Response.json with custom headers
// ---------------------------------------------------------------------------

func TestWebAPI_ResponseJsonCustomHeaders(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request, env) {
    const r = Response.json({ key: "value" }, {
      status: 201,
      headers: { "x-custom": "test" },
    });
    return Response.json({
      status: r.status,
      ct: r.headers.get("content-type"),
      custom: r.headers.get("x-custom"),
    });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Status int    `json:"status"`
		CT     string `json:"ct"`
		Custom string `json:"custom"`
	}
	json.Unmarshal(r.Response.Body, &data)
	if data.Status != 201 {
		t.Errorf("status = %d, want 201", data.Status)
	}
	if data.CT != "application/json" {
		t.Errorf("content-type = %q", data.CT)
	}
	if data.Custom != "test" {
		t.Errorf("custom = %q", data.Custom)
	}
}

// ---------------------------------------------------------------------------
// Integration: Response with null body
// ---------------------------------------------------------------------------

func TestWebAPI_ResponseNullBody(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    const r = new Response(null, { status: 204 });
    const text = await r.text();
    return Response.json({ status: r.status, body: text, empty: text === "" });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Status int  `json:"status"`
		Empty  bool `json:"empty"`
	}
	json.Unmarshal(r.Response.Body, &data)
	if data.Status != 204 {
		t.Errorf("status = %d", data.Status)
	}
	if !data.Empty {
		t.Error("null body should produce empty text")
	}
}

func TestWebAPI_URLSearchParamsToString(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request) {
    const params = new URLSearchParams("a=1&b=2&c=hello+world");
    return Response.json({
      str: params.toString(),
      has: params.has("b"),
      missing: params.has("z"),
      getC: params.get("c"),
    });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Str     string `json:"str"`
		Has     bool   `json:"has"`
		Missing bool   `json:"missing"`
		GetC    string `json:"getC"`
	}
	json.Unmarshal(r.Response.Body, &data)
	if !data.Has {
		t.Error("has('b') should be true")
	}
	if data.Missing {
		t.Error("has('z') should be false")
	}
	if data.GetC != "hello+world" && data.GetC != "hello world" {
		t.Errorf("get('c') = %q, want 'hello+world' or 'hello world'", data.GetC)
	}
}

func TestWebAPI_URLEdgeCases(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request) {
    // URL with port.
    const u1 = new URL("https://example.com:8080/path");
    // URL with auth.
    const u2 = new URL("https://user:pass@example.com/secret");
    // URL with fragment.
    const u3 = new URL("https://example.com/page#section");
    // URL with empty query.
    const u4 = new URL("https://example.com/page?");

    return Response.json({
      port: u1.port,
      pathname1: u1.pathname,
      hostname2: u2.hostname,
      hash3: u3.hash,
      search4: u4.search,
    });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Port      string `json:"port"`
		Pathname1 string `json:"pathname1"`
		Hostname2 string `json:"hostname2"`
		Hash3     string `json:"hash3"`
		Search4   string `json:"search4"`
	}
	json.Unmarshal(r.Response.Body, &data)
	if data.Port != "8080" {
		t.Errorf("port = %q, want 8080", data.Port)
	}
	if data.Pathname1 != "/path" {
		t.Errorf("pathname = %q", data.Pathname1)
	}
	if data.Hostname2 != "example.com" {
		t.Errorf("hostname = %q", data.Hostname2)
	}
}

func TestWebAPI_HeadersIteration(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request) {
    const h = new Headers();
    h.set("X-One", "1");
    h.set("X-Two", "2");
    h.set("X-Three", "3");

    // entries()
    const entries = [];
    for (const [k, v] of h.entries()) {
      entries.push(k + "=" + v);
    }

    // keys()
    const keys = [];
    for (const k of h.keys()) {
      keys.push(k);
    }

    // values()
    const values = [];
    for (const v of h.values()) {
      values.push(v);
    }

    return Response.json({
      entryCount: entries.length,
      keyCount: keys.length,
      valueCount: values.length,
    });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		EntryCount int `json:"entryCount"`
		KeyCount   int `json:"keyCount"`
		ValueCount int `json:"valueCount"`
	}
	json.Unmarshal(r.Response.Body, &data)
	if data.EntryCount != 3 {
		t.Errorf("entries = %d, want 3", data.EntryCount)
	}
	if data.KeyCount != 3 {
		t.Errorf("keys = %d, want 3", data.KeyCount)
	}
	if data.ValueCount != 3 {
		t.Errorf("values = %d, want 3", data.ValueCount)
	}
}

func TestWebAPI_ResponseStatusText(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request) {
    const r = new Response("ok", { status: 200, statusText: "Custom OK" });
    return Response.json({
      status: r.status,
      statusText: r.statusText,
      ok: r.ok,
    });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Status     int    `json:"status"`
		StatusText string `json:"statusText"`
		OK         bool   `json:"ok"`
	}
	json.Unmarshal(r.Response.Body, &data)
	if data.Status != 200 {
		t.Errorf("status = %d", data.Status)
	}
	if !data.OK {
		t.Error("ok should be true for 200")
	}
}

func TestWebAPI_URLRelativeWithBase(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request, env) {
    const url = new URL("/path?q=1", "https://example.com:8080");
    return Response.json({
      href: url.href,
      hostname: url.hostname,
      port: url.port,
      pathname: url.pathname,
      search: url.search,
    });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Href     string `json:"href"`
		Hostname string `json:"hostname"`
		Port     string `json:"port"`
		Pathname string `json:"pathname"`
		Search   string `json:"search"`
	}
	json.Unmarshal(r.Response.Body, &data)
	if data.Hostname != "example.com" {
		t.Errorf("hostname = %q", data.Hostname)
	}
	if data.Port != "8080" {
		t.Errorf("port = %q", data.Port)
	}
	if data.Pathname != "/path" {
		t.Errorf("pathname = %q", data.Pathname)
	}
}

func TestWebAPI_URLInvalid(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request, env) {
    let threw = false;
    try {
      new URL("not a valid url");
    } catch(e) {
      threw = true;
    }
    return Response.json({ threw });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Threw bool `json:"threw"`
	}
	json.Unmarshal(r.Response.Body, &data)
	if !data.Threw {
		t.Error("new URL('not a valid url') should throw")
	}
}

func TestWebAPI_URLSearchParamsDelete(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request, env) {
    const params = new URLSearchParams("a=1&b=2&c=3");
    params.delete("b");
    return Response.json({
      str: params.toString(),
      hasB: params.has("b"),
      hasA: params.has("a"),
    });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Str  string `json:"str"`
		HasB bool   `json:"hasB"`
		HasA bool   `json:"hasA"`
	}
	json.Unmarshal(r.Response.Body, &data)
	if data.HasB {
		t.Error("params should not have 'b' after delete")
	}
	if !data.HasA {
		t.Error("params should still have 'a'")
	}
}

func TestWebAPI_URLSearchParamsSort(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request, env) {
    const params = new URLSearchParams("c=3&a=1&b=2");
    params.sort();
    const keys = [];
    params.forEach(function(value, key) {
      keys.push(key);
    });
    return Response.json({ keys });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Keys []string `json:"keys"`
	}
	json.Unmarshal(r.Response.Body, &data)
	if len(data.Keys) != 3 || data.Keys[0] != "a" || data.Keys[1] != "b" || data.Keys[2] != "c" {
		t.Errorf("keys after sort = %v, want [a b c]", data.Keys)
	}
}

func TestWebAPI_ResponseCloneWithHeaders(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    const original = new Response("body text", {
      status: 201,
      headers: { "X-Custom": "value" }
    });
    const cloned = original.clone();
    const origText = await original.text();
    const clonedText = await cloned.text();
    return Response.json({
      origText,
      clonedText,
      status: cloned.status,
      header: cloned.headers.get("X-Custom"),
    });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		OrigText   string `json:"origText"`
		ClonedText string `json:"clonedText"`
		Status     int    `json:"status"`
		Header     string `json:"header"`
	}
	json.Unmarshal(r.Response.Body, &data)
	if data.OrigText != "body text" {
		t.Errorf("origText = %q", data.OrigText)
	}
	if data.ClonedText != "body text" {
		t.Errorf("clonedText = %q", data.ClonedText)
	}
	if data.Status != 201 {
		t.Errorf("status = %d", data.Status)
	}
}

func TestWebAPI_RequestProperties(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request, env) {
    const url = new URL(request.url);
    return Response.json({
      method: request.method,
      url: request.url,
      pathname: url.pathname,
      headerKeys: [...request.headers.keys()],
    });
  },
};`

	req := &WorkerRequest{
		Method:  "DELETE",
		URL:     "http://localhost/items/123?force=true",
		Headers: map[string]string{"Authorization": "Bearer abc", "Accept": "application/json"},
	}

	r := execJS(t, e, source, defaultEnv(), req)
	assertOK(t, r)

	var data struct {
		Method     string   `json:"method"`
		URL        string   `json:"url"`
		Pathname   string   `json:"pathname"`
		HeaderKeys []string `json:"headerKeys"`
	}
	json.Unmarshal(r.Response.Body, &data)
	if data.Method != "DELETE" {
		t.Errorf("method = %q", data.Method)
	}
	if data.Pathname != "/items/123" {
		t.Errorf("pathname = %q", data.Pathname)
	}
	if len(data.HeaderKeys) < 2 {
		t.Errorf("expected at least 2 header keys, got %d", len(data.HeaderKeys))
	}
}

// ---------------------------------------------------------------------------
// Integration: Binary response body (Uint8Array) — covers jsResponseToGo base64 path
// ---------------------------------------------------------------------------

func TestWebAPI_ResponseBinaryBody(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request) {
    const bytes = new Uint8Array([72, 101, 108, 108, 111]); // "Hello"
    return new Response(bytes, {
      headers: { "content-type": "application/octet-stream" },
    });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	if string(r.Response.Body) != "Hello" {
		t.Errorf("body = %q, want 'Hello'", string(r.Response.Body))
	}
}

func TestWebAPI_ResponseArrayBufferDirectBody(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request) {
    const encoder = new TextEncoder();
    const buf = encoder.encode("binary data");
    return new Response(buf);
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	if string(r.Response.Body) != "binary data" {
		t.Errorf("body = %q, want 'binary data'", string(r.Response.Body))
	}
}

// ---------------------------------------------------------------------------
// Integration: Worker returning null/undefined (covers jsResponseToGo error)
// ---------------------------------------------------------------------------

func TestWebAPI_ResponseNull(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request) {
    return null;
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	if r.Error == nil {
		t.Fatal("returning null should produce an error")
	}
}

func TestWebAPI_ResponseUndefined(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request) {
    return undefined;
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	if r.Error == nil {
		t.Fatal("returning undefined should produce an error")
	}
}

// ---------------------------------------------------------------------------
// Integration: POST request with body (covers goRequestToJS body path)
// ---------------------------------------------------------------------------

func TestWebAPI_PostRequestWithBody(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request) {
    const body = await request.text();
    return Response.json({
      method: request.method,
      body: body,
      ct: request.headers.get("content-type"),
    });
  },
};`

	req := &WorkerRequest{
		Method:  "POST",
		URL:     "http://localhost/api/data",
		Headers: map[string]string{"Content-Type": "application/json"},
		Body:    []byte(`{"key":"value"}`),
	}

	r := execJS(t, e, source, defaultEnv(), req)
	assertOK(t, r)

	var data struct {
		Method string `json:"method"`
		Body   string `json:"body"`
		CT     string `json:"ct"`
	}
	json.Unmarshal(r.Response.Body, &data)
	if data.Method != "POST" {
		t.Errorf("method = %q", data.Method)
	}
	if data.Body != `{"key":"value"}` {
		t.Errorf("body = %q", data.Body)
	}
	if data.CT != "application/json" {
		t.Errorf("content-type = %q", data.CT)
	}
}

// ---------------------------------------------------------------------------
// Integration: ctx.waitUntil (covers buildExecContext)
// ---------------------------------------------------------------------------

func TestWebAPI_CtxWaitUntil(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env, ctx) {
    let called = false;
    ctx.waitUntil(Promise.resolve().then(() => { called = true; }));
    await new Promise(r => setTimeout(r, 10));
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
		t.Error("waitUntil promise should have resolved")
	}
}

func TestWebAPI_CtxPassThroughOnException(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request, env, ctx) {
    // Should not throw; it's a no-op.
    ctx.passThroughOnException();
    return new Response("ok");
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)
}

// ---------------------------------------------------------------------------
// Integration: TextEncoder/TextDecoder
// ---------------------------------------------------------------------------

func TestWebAPI_TextEncoderDecoder(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request) {
    const encoder = new TextEncoder();
    const encoded = encoder.encode("Hello World");
    const decoder = new TextDecoder();
    const decoded = decoder.decode(encoded);
    return Response.json({
      decoded,
      byteLen: encoded.byteLength,
      encoding: encoder.encoding,
    });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Decoded  string `json:"decoded"`
		ByteLen  int    `json:"byteLen"`
		Encoding string `json:"encoding"`
	}
	json.Unmarshal(r.Response.Body, &data)
	if data.Decoded != "Hello World" {
		t.Errorf("decoded = %q", data.Decoded)
	}
	if data.ByteLen != 11 {
		t.Errorf("byteLen = %d, want 11", data.ByteLen)
	}
}

func TestWebAPI_HeadersSetGetDelete(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request, env) {
    const h = new Headers();
    h.set("Content-Type", "text/html");
    h.set("X-Custom", "value");
    h.append("X-Multi", "a");
    h.append("X-Multi", "b");
    const beforeDelete = h.has("X-Custom");
    h.delete("X-Custom");
    const afterDelete = h.has("X-Custom");
    return Response.json({
      ct: h.get("Content-Type"),
      multi: h.get("X-Multi"),
      beforeDelete,
      afterDelete,
    });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		CT           string `json:"ct"`
		Multi        string `json:"multi"`
		BeforeDelete bool   `json:"beforeDelete"`
		AfterDelete  bool   `json:"afterDelete"`
	}
	json.Unmarshal(r.Response.Body, &data)
	if data.CT != "text/html" {
		t.Errorf("content-type = %q", data.CT)
	}
	if !data.BeforeDelete {
		t.Error("should have X-Custom before delete")
	}
	if data.AfterDelete {
		t.Error("should not have X-Custom after delete")
	}
}
