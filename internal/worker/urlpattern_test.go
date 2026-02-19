package worker

import (
	"encoding/json"
	"testing"
)

func TestURLPattern_BasicPathname(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request, env) {
    const pattern = new URLPattern({ pathname: '/users/:id' });
    const result = pattern.exec('https://example.com/users/123');
    return Response.json({
      matched: result !== null,
      id: result ? result.pathname.groups.id : null,
      pathname: pattern.pathname,
    });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Matched  bool   `json:"matched"`
		ID       string `json:"id"`
		Pathname string `json:"pathname"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !data.Matched {
		t.Error("pattern should match /users/123")
	}
	if data.ID != "123" {
		t.Errorf("id = %q, want '123'", data.ID)
	}
	if data.Pathname != "/users/:id" {
		t.Errorf("pathname = %q, want '/users/:id'", data.Pathname)
	}
}

func TestURLPattern_Wildcard(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request, env) {
    const pattern = new URLPattern({ pathname: '/api/*' });
    return Response.json({
      matchesDeep: pattern.test('https://example.com/api/v1/users'),
      matchesShallow: pattern.test('https://example.com/api/health'),
      noMatch: pattern.test('https://example.com/other/path'),
    });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		MatchesDeep    bool `json:"matchesDeep"`
		MatchesShallow bool `json:"matchesShallow"`
		NoMatch        bool `json:"noMatch"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !data.MatchesDeep {
		t.Error("should match /api/v1/users")
	}
	if !data.MatchesShallow {
		t.Error("should match /api/health")
	}
	if data.NoMatch {
		t.Error("should not match /other/path")
	}
}

func TestURLPattern_NoMatch(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request, env) {
    const pattern = new URLPattern({ pathname: '/users/:id' });
    const result = pattern.exec('https://example.com/posts/456');
    return Response.json({
      matched: result !== null,
      testResult: pattern.test('https://example.com/posts/456'),
    });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Matched    bool `json:"matched"`
		TestResult bool `json:"testResult"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if data.Matched {
		t.Error("should not match /posts/456")
	}
	if data.TestResult {
		t.Error("test() should return false")
	}
}

func TestURLPattern_ExecGroups(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request, env) {
    const pattern = new URLPattern({ pathname: '/api/v:version/users/:userId' });
    const result = pattern.exec('https://example.com/api/v2/users/abc');
    return Response.json({
      matched: result !== null,
      version: result ? result.pathname.groups.version : null,
      userId: result ? result.pathname.groups.userId : null,
    });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Matched bool   `json:"matched"`
		Version string `json:"version"`
		UserID  string `json:"userId"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !data.Matched {
		t.Error("pattern should match")
	}
	if data.Version != "2" {
		t.Errorf("version = %q, want '2'", data.Version)
	}
	if data.UserID != "abc" {
		t.Errorf("userId = %q, want 'abc'", data.UserID)
	}
}

func TestURLPattern_TestReturnsBoolean(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request, env) {
    const pattern = new URLPattern({ pathname: '/hello' });
    return Response.json({
      matchTrue: pattern.test('https://example.com/hello'),
      matchFalse: pattern.test('https://example.com/world'),
      typeOfTrue: typeof pattern.test('https://example.com/hello'),
      typeOfFalse: typeof pattern.test('https://example.com/world'),
    });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		MatchTrue   bool   `json:"matchTrue"`
		MatchFalse  bool   `json:"matchFalse"`
		TypeOfTrue  string `json:"typeOfTrue"`
		TypeOfFalse string `json:"typeOfFalse"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !data.MatchTrue {
		t.Error("test should return true for /hello")
	}
	if data.MatchFalse {
		t.Error("test should return false for /world")
	}
	if data.TypeOfTrue != "boolean" {
		t.Errorf("typeof = %q, want 'boolean'", data.TypeOfTrue)
	}
}

func TestURLPattern_FullURLString(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request, env) {
    const pattern = new URLPattern('https://example.com/users/:id');
    const result = pattern.exec('https://example.com/users/42');
    return Response.json({
      matched: result !== null,
      id: result ? result.pathname.groups.id : null,
      protocol: pattern.protocol,
      hostname: pattern.hostname,
    });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Matched  bool   `json:"matched"`
		ID       string `json:"id"`
		Protocol string `json:"protocol"`
		Hostname string `json:"hostname"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !data.Matched {
		t.Error("should match full URL pattern")
	}
	if data.ID != "42" {
		t.Errorf("id = %q, want '42'", data.ID)
	}
	if data.Protocol != "https" {
		t.Errorf("protocol = %q, want 'https'", data.Protocol)
	}
	if data.Hostname != "example.com" {
		t.Errorf("hostname = %q, want 'example.com'", data.Hostname)
	}
}

func TestURLPattern_DictInput(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request, env) {
    const pattern = new URLPattern({
      protocol: 'https',
      hostname: 'example.com',
      pathname: '/items/:itemId',
    });
    const matchSame = pattern.test('https://example.com/items/99');
    const matchDiffHost = pattern.test('https://other.com/items/99');
    return Response.json({ matchSame, matchDiffHost });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		MatchSame     bool `json:"matchSame"`
		MatchDiffHost bool `json:"matchDiffHost"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !data.MatchSame {
		t.Error("should match same host")
	}
	if data.MatchDiffHost {
		t.Error("should not match different host")
	}
}
