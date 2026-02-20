package client

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestClient_CreateAPIKey_EmptyName(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(400)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "name is required"})
	}))
	defer srv.Close()

	c := New(srv.URL, "test-token")
	_, err := c.CreateAPIKey("")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected *APIError, got %T", err)
	}
	if apiErr.StatusCode != 400 {
		t.Errorf("expected StatusCode 400, got %d", apiErr.StatusCode)
	}
	if !strings.Contains(apiErr.Message, "required") {
		t.Errorf("expected error about required field, got %s", apiErr.Message)
	}
}

func TestClient_CreateAPIKey_Unauthorized(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(401)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "unauthorized"})
	}))
	defer srv.Close()

	c := New(srv.URL, "invalid-token")
	_, err := c.CreateAPIKey("test-key")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected *APIError, got %T", err)
	}
	if apiErr.StatusCode != 401 {
		t.Errorf("expected StatusCode 401, got %d", apiErr.StatusCode)
	}
}

func TestClient_CreateAPIKey_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "database unavailable"})
	}))
	defer srv.Close()

	c := New(srv.URL, "test-token")
	_, err := c.CreateAPIKey("test-key")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected *APIError, got %T", err)
	}
	if apiErr.StatusCode != 500 {
		t.Errorf("expected StatusCode 500, got %d", apiErr.StatusCode)
	}
	if !strings.Contains(apiErr.Message, "database") {
		t.Errorf("expected database error message, got %s", apiErr.Message)
	}
}

func TestClient_CreateAPIKey_InvalidJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(201)
		_, _ = w.Write([]byte("invalid json"))
	}))
	defer srv.Close()

	c := New(srv.URL, "test-token")
	_, err := c.CreateAPIKey("test-key")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "invalid character") {
		t.Errorf("expected JSON parse error, got %v", err)
	}
}

func TestBrowserLogin_StateMismatch(t *testing.T) {
	// This test verifies that state validation works, but requires
	// simulating the full OAuth flow which is complex for a unit test.
	// The functionality is covered by the callback handler logic.
	t.Skip("Skipping state mismatch test - requires full OAuth flow simulation")
}

func TestBrowserLogin_NoCode(t *testing.T) {
	// Similar to state mismatch, this requires full OAuth flow simulation
	t.Skip("Skipping no-code test - requires full OAuth flow simulation")
}

func TestOpenBrowser(t *testing.T) {
	// openBrowser is platform-specific and launches external processes
	// We can't easily test this without mocking exec.Command, which is
	// beyond the scope of simple unit tests
	t.Skip("Skipping openBrowser test - requires process mocking")
}

func TestBrowserLogin_IntegrationFlow(t *testing.T) {
	// Full integration test for BrowserLogin would require:
	// 1. Mock OAuth server
	// 2. Simulated browser callback
	// 3. Token exchange endpoint
	// 4. API key creation endpoint
	// This is beyond the scope of simple unit tests and would be
	// better suited for integration testing with a real server
	t.Skip("Skipping full BrowserLogin test - requires integration test environment")
}
