package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/cryguy/hostedat/internal/config"
	"github.com/cryguy/hostedat/internal/models"
	"github.com/cryguy/hostedat/internal/storage"
	"github.com/labstack/echo/v4"
)

func TestSubdomainRouter_StorageSubdomain_ProxyEnabled(t *testing.T) {
	db, err := models.InitDB(config.DBConfig{Driver: "sqlite", DSN: ":memory:"})
	if err != nil {
		t.Fatalf("InitDB: %v", err)
	}

	proxyCalls := 0
	proxy := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		proxyCalls++
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("proxied"))
	})

	e := echo.New()
	e.Use(SubdomainRouter(db, storage.NewManager(t.TempDir()), storage.NewSiteRulesCache(), "test.local", nil, proxy, nil))
	nextCalls := 0
	e.Any("/*", func(c echo.Context) error {
		nextCalls++
		return c.String(http.StatusOK, "next")
	})

	req := httptest.NewRequest(http.MethodGet, "/bucket/object", nil)
	req.Host = "storage.test.local"
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if proxyCalls != 1 {
		t.Fatalf("proxy calls = %d, want 1", proxyCalls)
	}
	if nextCalls != 0 {
		t.Fatalf("next calls = %d, want 0", nextCalls)
	}
}

func TestSubdomainRouter_StorageSubdomain_ProxyDisabled_NotFound(t *testing.T) {
	db, err := models.InitDB(config.DBConfig{Driver: "sqlite", DSN: ":memory:"})
	if err != nil {
		t.Fatalf("InitDB: %v", err)
	}

	e := echo.New()
	e.Use(SubdomainRouter(db, storage.NewManager(t.TempDir()), storage.NewSiteRulesCache(), "test.local", nil, nil, nil))
	e.Any("/*", func(c echo.Context) error {
		return c.String(http.StatusOK, "next")
	})

	req := httptest.NewRequest(http.MethodGet, "/bucket/object", nil)
	req.Host = "storage.test.local"
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestSubdomainRouter_StorageSubdomain_DoesNotHitSiteServing(t *testing.T) {
	db, err := models.InitDB(config.DBConfig{Driver: "sqlite", DSN: ":memory:"})
	if err != nil {
		t.Fatalf("InitDB: %v", err)
	}

	proxy := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	})

	e := echo.New()
	e.Use(SubdomainRouter(db, storage.NewManager(t.TempDir()), storage.NewSiteRulesCache(), "test.local", nil, proxy, nil))
	e.Any("/*", func(c echo.Context) error {
		return c.String(http.StatusOK, "next")
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Host = "storage.test.local"
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusTeapot {
		t.Fatalf("status = %d, want 418", rec.Code)
	}
}

func TestS3Proxy_PassthroughProxies(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer target.Close()

	proxy := NewS3Proxy(target.URL)

	// Unsigned request — proxy passes it through; SeaweedFS decides auth.
	req := httptest.NewRequest(http.MethodGet, "http://storage.test.local/bucket", nil)
	rec := httptest.NewRecorder()
	proxy.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", rec.Code)
	}
}

func TestS3Proxy_SignedRequestPassthrough(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer target.Close()

	proxy := NewS3Proxy(target.URL)
	req := httptest.NewRequest(http.MethodGet, "http://storage.test.local/bucket", nil)
	req.Header.Set("Authorization", "AWS4-HMAC-SHA256 Credential=a/b/c, SignedHeaders=host;x-amz-date, Signature=abc")
	rec := httptest.NewRecorder()

	proxy.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", rec.Code)
	}
}

func TestS3Proxy_StripsCORSHeaders(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET")
		w.WriteHeader(http.StatusOK)
	}))
	defer target.Close()

	proxy := NewS3Proxy(target.URL)
	req := httptest.NewRequest(http.MethodGet, "http://storage.test.local/bucket/key", nil)
	rec := httptest.NewRecorder()
	proxy.ServeHTTP(rec, req)

	if rec.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Error("expected CORS headers to be stripped by proxy")
	}
}
