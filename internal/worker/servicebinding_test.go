package worker

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestServiceBinding_HasFetch(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	// Target worker that returns a simple JSON response.
	targetSource := `export default {
  async fetch(request, env) {
    return Response.json({ hello: "world" });
  },
};`
	targetSiteID := "target-site"
	targetDeployKey := "deploy1"
	if _, err := e.CompileAndCache(targetSiteID, targetDeployKey, targetSource); err != nil {
		t.Fatalf("CompileAndCache target: %v", err)
	}

	// Caller worker that checks if the binding has fetch.
	callerSource := `export default {
  async fetch(request, env) {
    const hasFetch = typeof env.TARGET.fetch === 'function';
    return Response.json({ hasFetch });
  },
};`

	env := &Env{
		Vars:       make(map[string]string),
		Secrets:    make(map[string]string),
		KVBindings: make(map[string]string),
		ServiceBindings: map[string]ServiceBindingConfig{
			"TARGET": {
				TargetSiteID:    targetSiteID,
				TargetDeployKey: targetDeployKey,
			},
		},
	}

	r := execJS(t, e, callerSource, env, getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		HasFetch bool `json:"hasFetch"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatal(err)
	}
	if !data.HasFetch {
		t.Error("env.TARGET.fetch should be a function")
	}
}

func TestServiceBinding_FetchCallsTarget(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	// Target worker that returns a JSON response.
	targetSource := `export default {
  async fetch(request, env) {
    return Response.json({ hello: "world" });
  },
};`
	targetSiteID := "sb-target"
	targetDeployKey := "deploy1"
	if _, err := e.CompileAndCache(targetSiteID, targetDeployKey, targetSource); err != nil {
		t.Fatalf("CompileAndCache target: %v", err)
	}

	// Caller worker that fetches from the service binding.
	callerSource := `export default {
  async fetch(request, env) {
    const resp = await env.TARGET.fetch("https://fake-host/test");
    const data = await resp.json();
    return Response.json({ fromTarget: data });
  },
};`

	env := &Env{
		Vars:       make(map[string]string),
		Secrets:    make(map[string]string),
		KVBindings: make(map[string]string),
		ServiceBindings: map[string]ServiceBindingConfig{
			"TARGET": {
				TargetSiteID:    targetSiteID,
				TargetDeployKey: targetDeployKey,
			},
		},
	}

	r := execJS(t, e, callerSource, env, getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		FromTarget struct {
			Hello string `json:"hello"`
		} `json:"fromTarget"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatal(err)
	}
	if data.FromTarget.Hello != "world" {
		t.Errorf("fromTarget.hello = %q, want %q", data.FromTarget.Hello, "world")
	}
}

func TestServiceBinding_Construction(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	// Simple test to verify binding construction works without error.
	source := `export default {
  async fetch(request, env) {
    const hasTarget = env.TARGET !== undefined;
    const targetType = typeof env.TARGET;
    return Response.json({ hasTarget, targetType });
  },
};`

	targetSource := `export default {
  async fetch(request, env) {
    return new Response("ok");
  },
};`
	if _, err := e.CompileAndCache("constr-target", "deploy1", targetSource); err != nil {
		t.Fatalf("CompileAndCache target: %v", err)
	}

	env := &Env{
		Vars:       make(map[string]string),
		Secrets:    make(map[string]string),
		KVBindings: make(map[string]string),
		ServiceBindings: map[string]ServiceBindingConfig{
			"TARGET": {
				TargetSiteID:    "constr-target",
				TargetDeployKey: "deploy1",
			},
		},
	}

	r := execJS(t, e, source, env, getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		HasTarget  bool   `json:"hasTarget"`
		TargetType string `json:"targetType"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatal(err)
	}
	if !data.HasTarget {
		t.Error("env.TARGET should exist")
	}
	if data.TargetType != "object" {
		t.Errorf("typeof env.TARGET = %q, want %q", data.TargetType, "object")
	}
}

func TestServiceBinding_RequestForwarding(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	// Target worker that echoes back method, url, content-type, and body.
	targetSource := `export default {
  async fetch(request, env) {
    const body = await request.text();
    const ct = request.headers.get('content-type') || '';
    return Response.json({
      method: request.method,
      url: request.url,
      contentType: ct,
      body: body,
    });
  },
};`
	targetSiteID := "fwd-target"
	targetDeployKey := "deploy1"
	if _, err := e.CompileAndCache(targetSiteID, targetDeployKey, targetSource); err != nil {
		t.Fatalf("CompileAndCache target: %v", err)
	}

	// Caller worker that POSTs to the service binding with headers and body.
	callerSource := `export default {
  async fetch(request, env) {
    const resp = await env.TARGET.fetch("https://fake-host/test-path", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ key: "value" }),
    });
    const data = await resp.json();
    return Response.json({ fromTarget: data });
  },
};`

	env := &Env{
		Vars:       make(map[string]string),
		Secrets:    make(map[string]string),
		KVBindings: make(map[string]string),
		ServiceBindings: map[string]ServiceBindingConfig{
			"TARGET": {
				TargetSiteID:    targetSiteID,
				TargetDeployKey: targetDeployKey,
			},
		},
	}

	r := execJS(t, e, callerSource, env, getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		FromTarget struct {
			Method      string `json:"method"`
			URL         string `json:"url"`
			ContentType string `json:"contentType"`
			Body        string `json:"body"`
		} `json:"fromTarget"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if data.FromTarget.Method != "POST" {
		t.Errorf("fromTarget.method = %q, want %q", data.FromTarget.Method, "POST")
	}
	if !strings.Contains(data.FromTarget.URL, "test-path") {
		t.Errorf("fromTarget.url = %q, want to contain test-path", data.FromTarget.URL)
	}
	if !strings.Contains(data.FromTarget.Body, "key") {
		t.Errorf("fromTarget.body = %q, want to contain key", data.FromTarget.Body)
	}
}

// TestServiceBinding_FetchWithPOSTAndBody verifies service binding fetch() with POST method and body.
func TestServiceBinding_FetchWithPOSTAndBody(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	targetSource := `export default {
  async fetch(request, env) {
    const body = await request.text();
    return Response.json({
      method: request.method,
      body: body,
    });
  },
};`
	targetSiteID := "post-target"
	targetDeployKey := "deploy1"
	if _, err := e.CompileAndCache(targetSiteID, targetDeployKey, targetSource); err != nil {
		t.Fatalf("CompileAndCache target: %v", err)
	}

	callerSource := `export default {
  async fetch(request, env) {
    const resp = await env.TARGET.fetch("https://fake-host/api", {
      method: "POST",
      body: "request body content",
    });
    const data = await resp.json();
    return Response.json({ fromTarget: data });
  },
};`

	env := &Env{
		Vars:       make(map[string]string),
		Secrets:    make(map[string]string),
		KVBindings: make(map[string]string),
		ServiceBindings: map[string]ServiceBindingConfig{
			"TARGET": {
				TargetSiteID:    targetSiteID,
				TargetDeployKey: targetDeployKey,
			},
		},
	}

	r := execJS(t, e, callerSource, env, getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		FromTarget struct {
			Method string `json:"method"`
			Body   string `json:"body"`
		} `json:"fromTarget"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if data.FromTarget.Method != "POST" {
		t.Errorf("method = %q, want POST", data.FromTarget.Method)
	}
	if data.FromTarget.Body != "request body content" {
		t.Errorf("body = %q, want %q", data.FromTarget.Body, "request body content")
	}
}

// TestServiceBinding_FetchWithCustomHeaders verifies service binding fetch() with custom headers.
func TestServiceBinding_FetchWithCustomHeaders(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	targetSource := `export default {
  async fetch(request, env) {
    const xCustom = request.headers.get('x-custom-header') || '';
    const auth = request.headers.get('authorization') || '';
    return Response.json({
      xCustom: xCustom,
      auth: auth,
    });
  },
};`
	targetSiteID := "headers-target"
	targetDeployKey := "deploy1"
	if _, err := e.CompileAndCache(targetSiteID, targetDeployKey, targetSource); err != nil {
		t.Fatalf("CompileAndCache target: %v", err)
	}

	callerSource := `export default {
  async fetch(request, env) {
    const resp = await env.TARGET.fetch("https://fake-host/test", {
      headers: {
        "X-Custom-Header": "custom-value",
        "Authorization": "Bearer token123",
      },
    });
    const data = await resp.json();
    return Response.json({ fromTarget: data });
  },
};`

	env := &Env{
		Vars:       make(map[string]string),
		Secrets:    make(map[string]string),
		KVBindings: make(map[string]string),
		ServiceBindings: map[string]ServiceBindingConfig{
			"TARGET": {
				TargetSiteID:    targetSiteID,
				TargetDeployKey: targetDeployKey,
			},
		},
	}

	r := execJS(t, e, callerSource, env, getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		FromTarget struct {
			XCustom string `json:"xCustom"`
			Auth    string `json:"auth"`
		} `json:"fromTarget"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if data.FromTarget.XCustom != "custom-value" {
		t.Errorf("x-custom-header = %q, want %q", data.FromTarget.XCustom, "custom-value")
	}
	if data.FromTarget.Auth != "Bearer token123" {
		t.Errorf("authorization = %q, want %q", data.FromTarget.Auth, "Bearer token123")
	}
}

// TestServiceBinding_MultipleBindingsInEnv verifies that multiple service bindings
// can be configured and each routes to its own target worker.
func TestServiceBinding_MultipleBindingsInEnv(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	// Target worker A
	targetSourceA := `export default {
  async fetch(request, env) {
    return Response.json({ source: "worker-A" });
  },
};`
	if _, err := e.CompileAndCache("multi-target-a", "deploy1", targetSourceA); err != nil {
		t.Fatalf("CompileAndCache target A: %v", err)
	}

	// Target worker B
	targetSourceB := `export default {
  async fetch(request, env) {
    return Response.json({ source: "worker-B" });
  },
};`
	if _, err := e.CompileAndCache("multi-target-b", "deploy1", targetSourceB); err != nil {
		t.Fatalf("CompileAndCache target B: %v", err)
	}

	callerSource := `export default {
  async fetch(request, env) {
    const respA = await env.SERVICE_A.fetch("https://fake-host/");
    const dataA = await respA.json();
    const respB = await env.SERVICE_B.fetch("https://fake-host/");
    const dataB = await respB.json();
    return Response.json({ a: dataA.source, b: dataB.source });
  },
};`

	env := &Env{
		Vars:       make(map[string]string),
		Secrets:    make(map[string]string),
		KVBindings: make(map[string]string),
		ServiceBindings: map[string]ServiceBindingConfig{
			"SERVICE_A": {
				TargetSiteID:    "multi-target-a",
				TargetDeployKey: "deploy1",
			},
			"SERVICE_B": {
				TargetSiteID:    "multi-target-b",
				TargetDeployKey: "deploy1",
			},
		},
	}

	r := execJS(t, e, callerSource, env, getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		A string `json:"a"`
		B string `json:"b"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if data.A != "worker-A" {
		t.Errorf("SERVICE_A source = %q, want %q", data.A, "worker-A")
	}
	if data.B != "worker-B" {
		t.Errorf("SERVICE_B source = %q, want %q", data.B, "worker-B")
	}
}

// TestServiceBinding_FetchNoArgs verifies fetch() with no arguments rejects.
func TestServiceBinding_FetchNoArgs(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	targetSource := `export default {
  async fetch(request, env) {
    return new Response("ok");
  },
};`
	if _, err := e.CompileAndCache("noargs-target", "deploy1", targetSource); err != nil {
		t.Fatalf("CompileAndCache target: %v", err)
	}

	callerSource := `export default {
  async fetch(request, env) {
    try {
      await env.TARGET.fetch();
    } catch (e) {
      return Response.json({ rejected: true, msg: String(e) });
    }
    return Response.json({ rejected: false });
  },
};`

	env := &Env{
		Vars:       make(map[string]string),
		Secrets:    make(map[string]string),
		KVBindings: make(map[string]string),
		ServiceBindings: map[string]ServiceBindingConfig{
			"TARGET": {
				TargetSiteID:    "noargs-target",
				TargetDeployKey: "deploy1",
			},
		},
	}

	r := execJS(t, e, callerSource, env, getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Rejected bool   `json:"rejected"`
		Msg      string `json:"msg"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatal(err)
	}
	if !data.Rejected {
		t.Error("fetch() with no args should reject")
	}
	if !strings.Contains(data.Msg, "requires at least one argument") {
		t.Errorf("msg = %q, should mention requires argument", data.Msg)
	}
}

// TestServiceBinding_FetchWithRequestObject verifies fetch() with a Request-like object.
func TestServiceBinding_FetchWithRequestObject(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	targetSource := `export default {
  async fetch(request, env) {
    return Response.json({
      method: request.method,
      url: request.url,
    });
  },
};`
	if _, err := e.CompileAndCache("reqobj-target", "deploy1", targetSource); err != nil {
		t.Fatalf("CompileAndCache target: %v", err)
	}

	// Pass a Request-like object (not a string URL) to fetch.
	callerSource := `export default {
  async fetch(request, env) {
    var req = new Request("https://example.com/path", { method: "PUT" });
    var resp = await env.TARGET.fetch(req);
    var data = await resp.json();
    return Response.json({ fromTarget: data });
  },
};`

	env := &Env{
		Vars:       make(map[string]string),
		Secrets:    make(map[string]string),
		KVBindings: make(map[string]string),
		ServiceBindings: map[string]ServiceBindingConfig{
			"TARGET": {
				TargetSiteID:    "reqobj-target",
				TargetDeployKey: "deploy1",
			},
		},
	}

	r := execJS(t, e, callerSource, env, getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		FromTarget struct {
			Method string `json:"method"`
			URL    string `json:"url"`
		} `json:"fromTarget"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatal(err)
	}
	if data.FromTarget.Method != "PUT" {
		t.Errorf("method = %q, want PUT", data.FromTarget.Method)
	}
	if !strings.Contains(data.FromTarget.URL, "example.com") {
		t.Errorf("url = %q, want to contain example.com", data.FromTarget.URL)
	}
}

// TestServiceBinding_FetchTargetError verifies error handling when the target worker fails.
func TestServiceBinding_FetchTargetError(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	// Target worker that throws an error.
	targetSource := `export default {
  async fetch(request, env) {
    throw new Error("intentional error");
  },
};`
	if _, err := e.CompileAndCache("err-target", "deploy1", targetSource); err != nil {
		t.Fatalf("CompileAndCache target: %v", err)
	}

	callerSource := `export default {
  async fetch(request, env) {
    try {
      await env.TARGET.fetch("https://fake-host/");
    } catch (e) {
      return Response.json({ rejected: true, msg: String(e) });
    }
    return Response.json({ rejected: false });
  },
};`

	env := &Env{
		Vars:       make(map[string]string),
		Secrets:    make(map[string]string),
		KVBindings: make(map[string]string),
		ServiceBindings: map[string]ServiceBindingConfig{
			"TARGET": {
				TargetSiteID:    "err-target",
				TargetDeployKey: "deploy1",
			},
		},
	}

	r := execJS(t, e, callerSource, env, getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Rejected bool   `json:"rejected"`
		Msg      string `json:"msg"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatal(err)
	}
	if !data.Rejected {
		t.Error("fetch to failing target should reject")
	}
}
