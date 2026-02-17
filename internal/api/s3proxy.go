package api

import (
	"net/http"
	"net/http/httputil"
	"net/url"
)

// NewS3Proxy creates a reverse proxy that forwards requests to the SeaweedFS S3 endpoint.
func NewS3Proxy(s3Endpoint string) http.Handler {
	target, _ := url.Parse(s3Endpoint)
	proxy := httputil.NewSingleHostReverseProxy(target)
	// Preserve the target host for SigV4 compatibility.
	originalDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		originalDirector(req)
		req.Host = target.Host
	}
	return proxy
}
