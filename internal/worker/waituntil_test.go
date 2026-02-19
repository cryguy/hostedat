package worker

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestWaitUntil_SinglePromise(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env, ctx) {
    let sideEffect = 0;
    ctx.waitUntil(new Promise(resolve => {
      setTimeout(() => { sideEffect = 42; resolve(); }, 10);
    }));
    // Return immediately; the waitUntil promise runs in the background.
    return new Response(JSON.stringify({ before: sideEffect }));
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	// The response is returned before the timer fires, so before should be 0.
	var data struct {
		Before int `json:"before"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if data.Before != 0 {
		t.Errorf("before = %d, want 0 (waitUntil should be background)", data.Before)
	}
}

func TestWaitUntil_MultiplePromises(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env, ctx) {
    let count = 0;
    ctx.waitUntil(new Promise(resolve => {
      setTimeout(() => { count++; resolve(); }, 10);
    }));
    ctx.waitUntil(new Promise(resolve => {
      setTimeout(() => { count++; resolve(); }, 20);
    }));
    ctx.waitUntil(Promise.resolve().then(() => { count++; }));

    // Return response immediately.
    const resp = new Response("ok");

    // The drainWaitUntil runs after response is built.
    // We can't check count from here, but the test verifies no errors.
    return resp;
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)
	if r.Response.StatusCode != 200 {
		t.Errorf("status = %d, want 200", r.Response.StatusCode)
	}
}

func TestWaitUntil_PromiseWithConsoleLog(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	// The waitUntil promise logs during drain - we should see the log.
	source := `export default {
  async fetch(request, env, ctx) {
    ctx.waitUntil(new Promise(resolve => {
      setTimeout(() => {
        console.log("background task completed");
        resolve();
      }, 10);
    }));
    return new Response("ok");
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	found := false
	for _, log := range r.Logs {
		if strings.Contains(log.Message, "background task completed") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected 'background task completed' log from waitUntil promise")
	}
}

func TestWaitUntil_RejectedPromiseDoesNotBreakResponse(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env, ctx) {
    ctx.waitUntil(Promise.reject(new Error("background failure")));
    ctx.waitUntil(Promise.resolve("success"));
    return new Response("ok");
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)
	if string(r.Response.Body) != "ok" {
		t.Errorf("body = %q, want 'ok'", r.Response.Body)
	}
}

func TestWaitUntil_NoPromises(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	// waitUntil is available but never called - should work fine.
	source := `export default {
  async fetch(request, env, ctx) {
    return new Response("no waituntil");
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)
	if string(r.Response.Body) != "no waituntil" {
		t.Errorf("body = %q, want 'no waituntil'", r.Response.Body)
	}
}

func TestWaitUntil_CalledWithNonPromise(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	// Passing a non-promise value should be wrapped via Promise.resolve().
	source := `export default {
  async fetch(request, env, ctx) {
    ctx.waitUntil(42);
    ctx.waitUntil("hello");
    return new Response("ok");
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)
	if string(r.Response.Body) != "ok" {
		t.Errorf("body = %q, want 'ok'", r.Response.Body)
	}
}

func TestWaitUntil_WithKVWrite(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	nsID := "test-wu-kv-ns"
	env := &Env{
		Vars:       make(map[string]string),
		Secrets:    make(map[string]string),
		KVBindings: map[string]string{"MY_KV": nsID},
	}

	source := `export default {
  async fetch(request, env, ctx) {
    // Schedule a background KV write via waitUntil.
    ctx.waitUntil((async () => {
      await env.MY_KV.put("bg_key", "bg_value");
    })());
    return new Response("ok");
  },
};`

	siteID := "test-wu-kv-site"
	deployKey := "deploy1"
	if _, err := e.CompileAndCache(siteID, deployKey, source); err != nil {
		t.Fatalf("CompileAndCache: %v", err)
	}
	r := e.Execute(siteID, deployKey, env, getReq("http://localhost/"))
	assertOK(t, r)

	// Verify the KV write happened during the waitUntil drain.
	val, err := (&KVBridge{DB: db, NamespaceID: nsID}).Get("bg_key")
	if err != nil {
		t.Fatalf("KV get: %v", err)
	}
	if val != "bg_value" {
		t.Errorf("KV value = %q, want 'bg_value'", val)
	}
}
