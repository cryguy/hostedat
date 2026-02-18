package worker

import (
	"net"
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
		{"0.0.0.1", true},        // "This" network
		{"100.64.0.1", true},     // CGNAT
		{"100.128.0.1", false},   // Above CGNAT range
		{"192.0.0.1", true},      // IETF protocol assignments
		{"192.0.2.1", true},      // TEST-NET-1
		{"198.18.0.1", true},     // Benchmarking
		{"198.51.100.1", true},   // TEST-NET-2
		{"203.0.113.1", true},    // TEST-NET-3
		{"240.0.0.1", true},      // Reserved
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
