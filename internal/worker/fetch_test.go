package worker

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestIsPrivateIP(t *testing.T) {
	tests := []struct {
		ip      string
		private bool
	}{
		{"127.0.0.1", true},
		{"127.0.0.2", true},
		{"10.0.0.1", true},
		{"10.255.255.255", true},
		{"172.16.0.1", true},
		{"172.31.255.255", true},
		{"172.32.0.1", false},
		{"192.168.0.1", true},
		{"192.168.255.255", true},
		{"169.254.0.1", true},
		{"169.254.169.254", true}, // Cloud metadata
		{"0.0.0.1", true},         // "This" network
		{"100.64.0.1", true},      // CGNAT
		{"100.128.0.1", false},    // Above CGNAT range
		{"192.0.0.1", true},       // IETF protocol assignments
		{"192.0.2.1", true},       // TEST-NET-1
		{"198.18.0.1", true},      // Benchmarking
		{"198.51.100.1", true},    // TEST-NET-2
		{"203.0.113.1", true},     // TEST-NET-3
		{"240.0.0.1", true},       // Reserved
		{"8.8.8.8", false},
		{"1.1.1.1", false},
		{"::1", true},
	}

	for _, tt := range tests {
		t.Run(tt.ip, func(t *testing.T) {
			ip := net.ParseIP(tt.ip)
			if ip == nil {
				t.Fatalf("failed to parse IP: %s", tt.ip)
			}
			got := isPrivateIP(ip)
			if got != tt.private {
				t.Errorf("isPrivateIP(%s) = %v, want %v", tt.ip, got, tt.private)
			}
		})
	}
}

func TestIsPrivateIP_IPv6(t *testing.T) {
	tests := []struct {
		ip      string
		private bool
	}{
		{"::1", true},                       // loopback
		{"fc00::1", true},                   // unique local
		{"fd12:3456:789a::1", true},         // unique local
		{"fe80::1", true},                   // link-local
		{"fe80::abcd:ef01:2345:6789", true}, // link-local
		{"2001:db8::1", false},              // documentation, not in our list
		{"2607:f8b0:4004:800::200e", false}, // Google public
	}

	for _, tt := range tests {
		t.Run(tt.ip, func(t *testing.T) {
			ip := net.ParseIP(tt.ip)
			if ip == nil {
				t.Fatalf("failed to parse IP: %s", tt.ip)
			}
			got := isPrivateIP(ip)
			if got != tt.private {
				t.Errorf("isPrivateIP(%s) = %v, want %v", tt.ip, got, tt.private)
			}
		})
	}
}

func TestIsPrivateHostname_EdgeCases(t *testing.T) {
	tests := []struct {
		url     string
		private bool
	}{
		{"http://LOCALHOST/api", true},          // case insensitive
		{"http://sub.sub.localhost/api", true},  // deeply nested .localhost
		{"http://172.16.0.1:8080/api", true},    // private with port
		{"http://169.254.169.254/latest", true}, // cloud metadata
		{"http://[fc00::1]/api", true},          // IPv6 unique local
		{"http://[fe80::1]/api", true},          // IPv6 link-local
		{"http://8.8.8.8/api", false},           // public
		{"http://example.com:443/api", false},   // public with port
	}

	for _, tt := range tests {
		t.Run(tt.url, func(t *testing.T) {
			got := isPrivateHostname(tt.url)
			if got != tt.private {
				t.Errorf("isPrivateHostname(%q) = %v, want %v", tt.url, got, tt.private)
			}
		})
	}
}

func TestIsPrivateHostname(t *testing.T) {
	tests := []struct {
		url     string
		private bool
	}{
		{"http://localhost/api", true},
		{"http://foo.localhost/api", true},
		{"http://127.0.0.1/api", true},
		{"http://10.0.0.1/api", true},
		{"http://192.168.1.1/api", true},
		{"http://[::1]/api", true},
		{"not-a-url", true},
		{"", true},
		// Non-literal hostnames are not blocked by the pre-check;
		// actual SSRF protection happens in ssrfSafeDialContext.
		{"http://example.com/api", false},
	}

	for _, tt := range tests {
		t.Run(tt.url, func(t *testing.T) {
			got := isPrivateHostname(tt.url)
			if got != tt.private {
				t.Errorf("isPrivateHostname(%q) = %v, want %v", tt.url, got, tt.private)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Fetch SSRF test helper
// ---------------------------------------------------------------------------

// disableFetchSSRF temporarily disables SSRF protection so tests can
// connect to httptest servers on 127.0.0.1. Restored via t.Cleanup.
func disableFetchSSRF(t *testing.T) {
	t.Helper()
	origSSRF := fetchSSRFEnabled
	origTransport := fetchTransport
	fetchSSRFEnabled = false
	fetchTransport = http.DefaultTransport
	t.Cleanup(func() {
		fetchSSRFEnabled = origSSRF
		fetchTransport = origTransport
	})
}

// ---------------------------------------------------------------------------
// Redirect: follow (default)
// ---------------------------------------------------------------------------

func TestFetch_Redirect_Follow(t *testing.T) {
	disableFetchSSRF(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/redirect" {
			http.Redirect(w, r, "/final", http.StatusFound)
			return
		}
		w.Header().Set("Content-Type", "text/plain")
		_, _ = fmt.Fprint(w, "arrived")
	}))
	defer srv.Close()

	db := testDB(t)
	e := newTestEngine(t, db)

	source := fmt.Sprintf(`export default {
  async fetch(request, env) {
    var resp = await fetch("%s/redirect");
    var body = await resp.text();
    return new Response(JSON.stringify({
      status: resp.status,
      body: body,
      redirected: resp.redirected || false,
      url: resp.url
    }), {headers: {"content-type": "application/json"}});
  },
};`, srv.URL)

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Status     int    `json:"status"`
		Body       string `json:"body"`
		Redirected bool   `json:"redirected"`
		URL        string `json:"url"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if data.Status != 200 {
		t.Errorf("status = %d, want 200", data.Status)
	}
	if data.Body != "arrived" {
		t.Errorf("body = %q, want %q", data.Body, "arrived")
	}
	if !data.Redirected {
		t.Error("redirected should be true after following a redirect")
	}
	if !strings.HasSuffix(data.URL, "/final") {
		t.Errorf("url = %q, should end with /final", data.URL)
	}
}

// ---------------------------------------------------------------------------
// Redirect: manual
// ---------------------------------------------------------------------------

func TestFetch_Redirect_Manual(t *testing.T) {
	disableFetchSSRF(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/redirect" {
			w.Header().Set("Location", "/final")
			w.WriteHeader(http.StatusFound)
			return
		}
		_, _ = fmt.Fprint(w, "should not reach")
	}))
	defer srv.Close()

	db := testDB(t)
	e := newTestEngine(t, db)

	source := fmt.Sprintf(`export default {
  async fetch(request, env) {
    var resp = await fetch("%s/redirect", {redirect: "manual"});
    return new Response(JSON.stringify({
      status: resp.status,
      location: resp.headers.get("location") || "",
      redirected: resp.redirected || false
    }), {headers: {"content-type": "application/json"}});
  },
};`, srv.URL)

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Status     int    `json:"status"`
		Location   string `json:"location"`
		Redirected bool   `json:"redirected"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if data.Status != 302 {
		t.Errorf("status = %d, want 302", data.Status)
	}
	if data.Location != "/final" {
		t.Errorf("location = %q, want %q", data.Location, "/final")
	}
	if data.Redirected {
		t.Error("redirected should be false for manual mode")
	}
}

// ---------------------------------------------------------------------------
// Redirect: error
// ---------------------------------------------------------------------------

func TestFetch_Redirect_Error(t *testing.T) {
	disableFetchSSRF(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/final", http.StatusFound)
	}))
	defer srv.Close()

	db := testDB(t)
	e := newTestEngine(t, db)

	source := fmt.Sprintf(`export default {
  async fetch(request, env) {
    try {
      await fetch("%s/redirect", {redirect: "error"});
      return new Response("should not reach", {status: 200});
    } catch(e) {
      return new Response(JSON.stringify({
        caught: true,
        name: e.name || "Error",
        message: e.message
      }), {headers: {"content-type": "application/json"}});
    }
  },
};`, srv.URL)

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Caught  bool   `json:"caught"`
		Name    string `json:"name"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !data.Caught {
		t.Fatal("expected error to be caught")
	}
	if data.Name != "TypeError" {
		t.Errorf("error name = %q, want TypeError", data.Name)
	}
	if !strings.Contains(data.Message, "redirect") {
		t.Errorf("error message = %q, should mention redirect", data.Message)
	}
}

// ---------------------------------------------------------------------------
// Redirect: follow with no redirect (passthrough)
// ---------------------------------------------------------------------------

func TestFetch_Redirect_Follow_NoRedirect(t *testing.T) {
	disableFetchSSRF(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = fmt.Fprint(w, "direct")
	}))
	defer srv.Close()

	db := testDB(t)
	e := newTestEngine(t, db)

	source := fmt.Sprintf(`export default {
  async fetch(request, env) {
    var resp = await fetch("%s/hello", {redirect: "follow"});
    var body = await resp.text();
    return new Response(JSON.stringify({
      status: resp.status,
      body: body,
      redirected: resp.redirected || false
    }), {headers: {"content-type": "application/json"}});
  },
};`, srv.URL)

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Status     int    `json:"status"`
		Body       string `json:"body"`
		Redirected bool   `json:"redirected"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if data.Status != 200 {
		t.Errorf("status = %d, want 200", data.Status)
	}
	if data.Body != "direct" {
		t.Errorf("body = %q, want %q", data.Body, "direct")
	}
	if data.Redirected {
		t.Error("redirected should be false when no redirect occurred")
	}
}

// ---------------------------------------------------------------------------
// Signal: pre-aborted AbortSignal
// ---------------------------------------------------------------------------

func TestFetch_Signal_PreAborted(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    var controller = new AbortController();
    controller.abort();
    try {
      await fetch("http://example.com", {signal: controller.signal});
      return new Response("should not reach", {status: 200});
    } catch(e) {
      return new Response(JSON.stringify({
        caught: true,
        name: e.name || "Error",
        message: e.message
      }), {headers: {"content-type": "application/json"}});
    }
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Caught  bool   `json:"caught"`
		Name    string `json:"name"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !data.Caught {
		t.Fatal("expected abort error to be caught")
	}
	if data.Name != "AbortError" {
		t.Errorf("error name = %q, want AbortError", data.Name)
	}
	if !strings.Contains(data.Message, "aborted") {
		t.Errorf("error message = %q, should mention aborted", data.Message)
	}
}

// ---------------------------------------------------------------------------
// Signal: AbortSignal.abort() static helper
// ---------------------------------------------------------------------------

func TestFetch_Signal_StaticAbort(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    try {
      await fetch("http://example.com", {signal: AbortSignal.abort()});
      return new Response("should not reach", {status: 200});
    } catch(e) {
      return new Response(JSON.stringify({
        caught: true,
        name: e.name || "Error",
        message: e.message
      }), {headers: {"content-type": "application/json"}});
    }
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Caught  bool   `json:"caught"`
		Name    string `json:"name"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !data.Caught {
		t.Fatal("expected abort error to be caught")
	}
	if data.Name != "AbortError" {
		t.Errorf("error name = %q, want AbortError", data.Name)
	}
}

// ---------------------------------------------------------------------------
// Signal: non-aborted signal should not interfere
// ---------------------------------------------------------------------------

func TestFetch_Signal_NotAborted(t *testing.T) {
	disableFetchSSRF(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = fmt.Fprint(w, "success")
	}))
	defer srv.Close()

	db := testDB(t)
	e := newTestEngine(t, db)

	source := fmt.Sprintf(`export default {
  async fetch(request, env) {
    var controller = new AbortController();
    var resp = await fetch("%s/", {signal: controller.signal});
    var body = await resp.text();
    return new Response(JSON.stringify({
      status: resp.status,
      body: body
    }), {headers: {"content-type": "application/json"}});
  },
};`, srv.URL)

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Status int    `json:"status"`
		Body   string `json:"body"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if data.Status != 200 {
		t.Errorf("status = %d, want 200", data.Status)
	}
	if data.Body != "success" {
		t.Errorf("body = %q, want %q", data.Body, "success")
	}
}

// ---------------------------------------------------------------------------
// fetch() with zero arguments
// ---------------------------------------------------------------------------

func TestFetch_ZeroArgs(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    try {
      await fetch();
      return new Response("should not reach", {status: 200});
    } catch(e) {
      return new Response(JSON.stringify({
        caught: true,
        message: e.message || String(e)
      }), {headers: {"content-type": "application/json"}});
    }
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Caught  bool   `json:"caught"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !data.Caught {
		t.Fatal("expected error to be caught")
	}
	if !strings.Contains(data.Message, "fetch requires at least 1 argument") {
		t.Errorf("message = %q, want to contain 'fetch requires at least 1 argument'", data.Message)
	}
}

// ---------------------------------------------------------------------------
// Rate limit enforcement
// ---------------------------------------------------------------------------

func TestFetch_RateLimit(t *testing.T) {
	disableFetchSSRF(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = fmt.Fprint(w, "ok")
	}))
	defer srv.Close()

	db := testDB(t)
	cfg := testCfg()
	cfg.MaxFetchRequests = 2
	e := NewEngine(cfg, db)
	t.Cleanup(func() { e.Shutdown() })

	source := fmt.Sprintf(`export default {
  async fetch(request, env) {
    var results = [];
    for (var i = 0; i < 3; i++) {
      try {
        var resp = await fetch("%s/");
        results.push({ok: true, status: resp.status});
      } catch(e) {
        results.push({ok: false, message: e.message || String(e)});
      }
    }
    return new Response(JSON.stringify(results), {headers: {"content-type": "application/json"}});
  },
};`, srv.URL)

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var results []struct {
		Ok      bool   `json:"ok"`
		Status  int    `json:"status"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(r.Response.Body, &results); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}
	if !results[0].Ok {
		t.Errorf("result[0] should succeed, got message: %s", results[0].Message)
	}
	if !results[1].Ok {
		t.Errorf("result[1] should succeed, got message: %s", results[1].Message)
	}
	if results[2].Ok {
		t.Errorf("result[2] should be rate-limited, but succeeded")
	}
	if !strings.Contains(results[2].Message, "exceeded maximum fetch requests") {
		t.Errorf("result[2] message = %q, want to contain 'exceeded maximum fetch requests'", results[2].Message)
	}
}

// ---------------------------------------------------------------------------
// Redirect: manual with 301
// ---------------------------------------------------------------------------

func TestFetch_Redirect_Manual_301(t *testing.T) {
	disableFetchSSRF(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/moved" {
			w.Header().Set("Location", "/new-location")
			w.WriteHeader(http.StatusMovedPermanently)
			return
		}
		_, _ = fmt.Fprint(w, "should not reach")
	}))
	defer srv.Close()

	db := testDB(t)
	e := newTestEngine(t, db)

	source := fmt.Sprintf(`export default {
  async fetch(request, env) {
    var resp = await fetch("%s/moved", {redirect: "manual"});
    return new Response(JSON.stringify({
      status: resp.status,
      location: resp.headers.get("location") || "",
      redirected: resp.redirected || false
    }), {headers: {"content-type": "application/json"}});
  },
};`, srv.URL)

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Status     int    `json:"status"`
		Location   string `json:"location"`
		Redirected bool   `json:"redirected"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if data.Status != 301 {
		t.Errorf("status = %d, want 301", data.Status)
	}
	if data.Location != "/new-location" {
		t.Errorf("location = %q, want /new-location", data.Location)
	}
	if data.Redirected {
		t.Error("redirected should be false for manual mode")
	}
}

// ---------------------------------------------------------------------------
// Redirect: manual with 307
// ---------------------------------------------------------------------------

func TestFetch_Redirect_Manual_307(t *testing.T) {
	disableFetchSSRF(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/temp" {
			w.Header().Set("Location", "/elsewhere")
			w.WriteHeader(http.StatusTemporaryRedirect)
			return
		}
		_, _ = fmt.Fprint(w, "should not reach")
	}))
	defer srv.Close()

	db := testDB(t)
	e := newTestEngine(t, db)

	source := fmt.Sprintf(`export default {
  async fetch(request, env) {
    var resp = await fetch("%s/temp", {redirect: "manual"});
    return new Response(JSON.stringify({
      status: resp.status,
      location: resp.headers.get("location") || "",
      redirected: resp.redirected || false
    }), {headers: {"content-type": "application/json"}});
  },
};`, srv.URL)

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Status     int    `json:"status"`
		Location   string `json:"location"`
		Redirected bool   `json:"redirected"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if data.Status != 307 {
		t.Errorf("status = %d, want 307", data.Status)
	}
	if data.Location != "/elsewhere" {
		t.Errorf("location = %q, want /elsewhere", data.Location)
	}
	if data.Redirected {
		t.Error("redirected should be false for manual mode")
	}
}

// ---------------------------------------------------------------------------
// Header filtering: forbidden headers
// ---------------------------------------------------------------------------

func TestFetch_ForbiddenHeadersFiltered(t *testing.T) {
	forbidden := []string{"Host", "Transfer-Encoding", "Connection", "X-Forwarded-For", "Proxy-Authorization"}
	for _, h := range forbidden {
		if !forbiddenFetchHeaders[strings.ToLower(h)] {
			t.Errorf("expected %q to be in forbidden list", h)
		}
	}
}

func TestFetch_AllowedHeadersNotFiltered(t *testing.T) {
	allowed := []string{"Content-Type", "Authorization", "Accept", "X-Custom-Header"}
	for _, h := range allowed {
		if forbiddenFetchHeaders[strings.ToLower(h)] {
			t.Errorf("expected %q to NOT be in forbidden list", h)
		}
	}
}

// ---------------------------------------------------------------------------
// Binary body round-trip
// ---------------------------------------------------------------------------

func TestFetch_BinaryBody(t *testing.T) {
	disableFetchSSRF(t)

	var receivedBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		receivedBody = body
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"received":%d}`, len(body))
	}))
	defer srv.Close()

	db := testDB(t)
	e := newTestEngine(t, db)

	source := fmt.Sprintf(`export default {
  async fetch(request, env) {
    var buf = new Uint8Array([1, 2, 3, 255, 254]).buffer;
    var resp = await fetch("%s/binary", {
      method: "POST",
      body: buf
    });
    var data = await resp.json();
    return new Response(JSON.stringify(data), {headers: {"content-type": "application/json"}});
  },
};`, srv.URL)

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Received int `json:"received"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if data.Received != 5 {
		t.Errorf("received = %d bytes, want 5", data.Received)
	}
	want := []byte{0x01, 0x02, 0x03, 0xFF, 0xFE}
	if len(receivedBody) != len(want) {
		t.Fatalf("server received %d bytes, want %d", len(receivedBody), len(want))
	}
	for i, b := range want {
		if receivedBody[i] != b {
			t.Errorf("byte[%d] = 0x%02X, want 0x%02X", i, receivedBody[i], b)
		}
	}
}
