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
