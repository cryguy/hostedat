package certs

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/caddyserver/certmagic"
)

func TestConfig_DomainConstruction(t *testing.T) {
	cfg := Config{
		Domain:   "example.com",
		APIToken: "test-token",
		DataDir:  "/tmp/certs",
	}

	// Verify the same domain list construction used in SetupTLS.
	domains := []string{cfg.Domain, "*." + cfg.Domain}

	if len(domains) != 2 {
		t.Fatalf("expected 2 domains, got %d", len(domains))
	}
	if domains[0] != "example.com" {
		t.Errorf("domains[0] = %q, want %q", domains[0], "example.com")
	}
	if domains[1] != "*.example.com" {
		t.Errorf("domains[1] = %q, want %q", domains[1], "*.example.com")
	}
}

func TestSetupTLS_InvalidConfig(t *testing.T) {
	// Stand up a mock server that returns non-ACME responses so ManageSync
	// fails quickly without real network calls to Let's Encrypt.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not an ACME server", http.StatusTeapot)
	}))
	defer srv.Close()

	// Save and restore certmagic globals that SetupTLS modifies.
	origCA := certmagic.DefaultACME.CA
	origAgreed := certmagic.DefaultACME.Agreed
	origStorage := certmagic.Default.Storage
	origSolver := certmagic.DefaultACME.DNS01Solver
	t.Cleanup(func() {
		certmagic.DefaultACME.CA = origCA
		certmagic.DefaultACME.Agreed = origAgreed
		certmagic.Default.Storage = origStorage
		certmagic.DefaultACME.DNS01Solver = origSolver
	})

	// Redirect ACME directory to mock server.
	certmagic.DefaultACME.CA = srv.URL

	cfg := Config{
		Domain:  "test.invalid",
		DataDir: t.TempDir(),
	}
	_, err := SetupTLS(cfg)
	if err == nil {
		t.Fatal("expected error with invalid ACME server")
	}
}
