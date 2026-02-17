package api

import (
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
)

// NewS3Proxy creates a reverse proxy that forwards requests to the SeaweedFS S3 endpoint.
// When requireSigV4 is true, unsigned requests are rejected before proxying.
func NewS3Proxy(s3Endpoint string, requireSigV4 bool) http.Handler {
	target, err := url.Parse(s3Endpoint)
	if err != nil {
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "invalid S3 proxy endpoint", http.StatusInternalServerError)
		})
	}
	proxy := httputil.NewSingleHostReverseProxy(target)
	// Preserve the target host for SigV4 compatibility.
	originalDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		originalDirector(req)
		req.Host = target.Host
	}

	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if requireSigV4 && !hasSigV4Signature(req) {
			http.Error(w, "missing SigV4 authentication", http.StatusForbidden)
			return
		}
		proxy.ServeHTTP(w, req)
	})
}

func hasSigV4Signature(req *http.Request) bool {
	auth := req.Header.Get("Authorization")
	if strings.HasPrefix(auth, "AWS4-HMAC-SHA256 ") {
		return true
	}

	q := req.URL.Query()
	if q.Get("X-Amz-Signature") == "" {
		return false
	}
	return strings.EqualFold(q.Get("X-Amz-Algorithm"), "AWS4-HMAC-SHA256")
}
