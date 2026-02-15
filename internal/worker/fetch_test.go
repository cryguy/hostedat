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

func TestIsPrivateURL(t *testing.T) {
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
	}

	for _, tt := range tests {
		t.Run(tt.url, func(t *testing.T) {
			got := isPrivateURL(tt.url)
			if got != tt.private {
				t.Errorf("isPrivateURL(%q) = %v, want %v", tt.url, got, tt.private)
			}
		})
	}
}
