package seaweedfs

import (
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestParsePort(t *testing.T) {
	tests := []struct {
		name     string
		endpoint string
		want     int
		wantErr  bool
	}{
		{"HTTP default", "http://localhost", 80, false},
		{"HTTPS default", "https://localhost", 443, false},
		{"explicit port", "http://localhost:8333", 8333, false},
		{"explicit HTTPS port", "https://localhost:9443", 9443, false},
		{"no scheme defaults to 80", "//localhost", 80, false},
		{"non-numeric port", "http://localhost:abc", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parsePort(tt.endpoint)
			if (err != nil) != tt.wantErr {
				t.Fatalf("parsePort(%q) error = %v, wantErr %v", tt.endpoint, err, tt.wantErr)
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("parsePort(%q) = %d, want %d", tt.endpoint, got, tt.want)
			}
		})
	}
}

func TestEnsurePortsAvailable(t *testing.T) {
	// Bind a port to make it unavailable.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to bind port: %v", err)
	}
	defer ln.Close()
	busyPort := ln.Addr().(*net.TCPAddr).Port

	if err := ensurePortsAvailable(busyPort); err == nil {
		t.Error("expected error for busy port, got nil")
	}

	// Find a free port by binding and immediately releasing.
	ln2, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to bind port: %v", err)
	}
	freePort := ln2.Addr().(*net.TCPAddr).Port
	ln2.Close()

	if err := ensurePortsAvailable(freePort); err != nil {
		t.Errorf("expected free port %d to be available: %v", freePort, err)
	}
}

func TestManagerIsHealthy_NilClient(t *testing.T) {
	m := &Manager{}
	if m.IsHealthy() {
		t.Error("expected IsHealthy to return false for nil client")
	}
}

func TestClientDoIAM_MalformedXML(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte("this is not xml at all"))
	}))
	defer srv.Close()

	client := NewClient(srv.URL)
	_, err := client.CreateAccessKey("test-user")
	if err == nil {
		t.Fatal("expected error for malformed XML response")
	}
}

func TestClientDoIAM_EmptyBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		// intentionally empty body
	}))
	defer srv.Close()

	client := NewClient(srv.URL)
	_, err := client.CreateAccessKey("test-user")
	if err == nil {
		t.Fatal("expected error for empty response body")
	}
}

func TestClient_HTTPTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(3 * time.Second)
		w.WriteHeader(200)
	}))
	defer srv.Close()

	client := NewClient(srv.URL)
	client.HTTPClient.Timeout = 100 * time.Millisecond

	err := client.CreateUser("test-user")
	if err == nil {
		t.Fatal("expected timeout error")
	}
}

func TestHealth_ConnectionRefused(t *testing.T) {
	// Point at a port that nothing is listening on.
	client := NewClient("http://127.0.0.1:1")
	client.HTTPClient.Timeout = 2 * time.Second

	if err := client.Health(); err == nil {
		t.Fatal("expected error for connection refused")
	}
}
