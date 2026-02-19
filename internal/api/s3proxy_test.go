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
	e.Use(SubdomainRouter(db, storage.NewManager(t.TempDir()), storage.NewSiteRulesCache(), "test.local", nil, proxy))
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
	e.Use(SubdomainRouter(db, storage.NewManager(t.TempDir()), storage.NewSiteRulesCache(), "test.local", nil, nil))
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
	e.Use(SubdomainRouter(db, storage.NewManager(t.TempDir()), storage.NewSiteRulesCache(), "test.local", nil, proxy))
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

func TestS3Proxy_UnsignedRequestRejected(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer target.Close()

	proxy := NewS3Proxy(target.URL, true)
	req := httptest.NewRequest(http.MethodGet, "http://storage.test.local/bucket", nil)
	rec := httptest.NewRecorder()

	proxy.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
}

func TestS3Proxy_SignedRequestAllowed(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer target.Close()

	proxy := NewS3Proxy(target.URL, true)
	req := httptest.NewRequest(http.MethodGet, "http://storage.test.local/bucket", nil)
	req.Header.Set("Authorization", "AWS4-HMAC-SHA256 Credential=a/b/c, SignedHeaders=host;x-amz-date, Signature=abc")
	rec := httptest.NewRecorder()

	proxy.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", rec.Code)
	}
}

// ──────────────────────────────────────────────
// extractBucketAndKey tests
// ──────────────────────────────────────────────

func TestExtractBucketAndKey(t *testing.T) {
	tests := []struct {
		name       string
		path       string
		wantBucket string
		wantKey    string
	}{
		{"valid simple", "/bucket/key", "bucket", "key"},
		{"nested key", "/bucket/nested/path/file.txt", "bucket", "nested/path/file.txt"},
		{"trailing slash only", "/bucket/", "", ""},
		{"no key", "/bucket", "", ""},
		{"root only", "/", "", ""},
		{"empty string", "", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bucket, key := extractBucketAndKey(tt.path)
			if bucket != tt.wantBucket || key != tt.wantKey {
				t.Errorf("extractBucketAndKey(%q) = (%q, %q), want (%q, %q)", tt.path, bucket, key, tt.wantBucket, tt.wantKey)
			}
		})
	}
}

// ──────────────────────────────────────────────
// hasSigV4Signature query string tests
// ──────────────────────────────────────────────

func TestHasSigV4Signature_QueryString(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "http://storage.test.local/bucket/key?X-Amz-Signature=abc123&X-Amz-Algorithm=AWS4-HMAC-SHA256", nil)
	if !hasSigV4Signature(req) {
		t.Error("expected query-string SigV4 to be detected")
	}
}

func TestHasSigV4Signature_MissingAlgorithm(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "http://storage.test.local/bucket/key?X-Amz-Signature=abc123", nil)
	if hasSigV4Signature(req) {
		t.Error("expected false when X-Amz-Algorithm is missing")
	}
}

func TestHasSigV4Signature_NoAuth(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "http://storage.test.local/bucket/key", nil)
	if hasSigV4Signature(req) {
		t.Error("expected false for unsigned request")
	}
}

// ──────────────────────────────────────────────
// S3 proxy with requireSigV4=false
// ──────────────────────────────────────────────

func TestS3Proxy_NoSigV4Required(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("proxied"))
	}))
	defer target.Close()

	proxy := NewS3Proxy(target.URL, false)
	req := httptest.NewRequest(http.MethodGet, "http://storage.test.local/bucket", nil)
	rec := httptest.NewRecorder()

	proxy.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (unsigned allowed when requireSigV4=false)", rec.Code)
	}
}
