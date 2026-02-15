package worker

import "testing"

func TestParseURL(t *testing.T) {
	tests := []struct {
		name     string
		rawURL   string
		base     string
		wantErr  bool
		href     string
		protocol string
		hostname string
		pathname string
		search   string
		hash     string
	}{
		{
			name:     "absolute URL with query and hash",
			rawURL:   "https://example.com/path?q=1#hash",
			href:     "https://example.com/path?q=1#hash",
			protocol: "https:",
			hostname: "example.com",
			pathname: "/path",
			search:   "?q=1",
			hash:     "#hash",
		},
		{
			name:     "with port",
			rawURL:   "http://localhost:8080/api",
			href:     "http://localhost:8080/api",
			protocol: "http:",
			hostname: "localhost",
			pathname: "/api",
		},
		{
			name:     "relative with base",
			rawURL:   "/path",
			base:     "https://example.com",
			href:     "https://example.com/path",
			protocol: "https:",
			hostname: "example.com",
			pathname: "/path",
		},
		{
			name:    "no scheme errors",
			rawURL:  "not-a-url",
			wantErr: true,
		},
		{
			name:     "simple https",
			rawURL:   "https://test.com",
			href:     "https://test.com",
			protocol: "https:",
			hostname: "test.com",
			pathname: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parsed, err := parseURL(tt.rawURL, tt.base)
			if (err != nil) != tt.wantErr {
				t.Fatalf("parseURL(%q, %q) error = %v, wantErr %v", tt.rawURL, tt.base, err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if parsed.Href != tt.href {
				t.Errorf("href = %q, want %q", parsed.Href, tt.href)
			}
			if parsed.Protocol != tt.protocol {
				t.Errorf("protocol = %q, want %q", parsed.Protocol, tt.protocol)
			}
			if parsed.Hostname != tt.hostname {
				t.Errorf("hostname = %q, want %q", parsed.Hostname, tt.hostname)
			}
			if parsed.Pathname != tt.pathname {
				t.Errorf("pathname = %q, want %q", parsed.Pathname, tt.pathname)
			}
			if tt.search != "" && parsed.Search != tt.search {
				t.Errorf("search = %q, want %q", parsed.Search, tt.search)
			}
			if tt.hash != "" && parsed.Hash != tt.hash {
				t.Errorf("hash = %q, want %q", parsed.Hash, tt.hash)
			}
		})
	}
}
