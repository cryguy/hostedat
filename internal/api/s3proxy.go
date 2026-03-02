package api

import (
	"net/http"
	"net/http/httputil"
	"net/url"
)

// NewS3Proxy creates a reverse proxy that forwards requests to the SeaweedFS
// S3 endpoint. Auth and public access are handled by SeaweedFS itself via its
// s3.config.json and bucket policies.
func NewS3Proxy(s3Endpoint string) http.Handler {
	target, err := url.Parse(s3Endpoint)
	if err != nil {
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "invalid S3 proxy endpoint", http.StatusInternalServerError)
		})
	}
	proxy := httputil.NewSingleHostReverseProxy(target)
	// Do NOT rewrite req.Host — SigV4 signatures include the Host header,
	// so the forwarded request must preserve the original Host that the
	// client (or presigned URL) was signed for. The proxy routes to the
	// backend via req.URL.Host (set by the default Director).

	// Strip CORS headers from the upstream S3 response. Echo's CORS
	// middleware already sets these on the outer response; if the backend
	// also returns them the values get duplicated (e.g. two identical
	// Access-Control-Allow-Origin values) which browsers reject.
	proxy.ModifyResponse = func(resp *http.Response) error {
		resp.Header.Del("Access-Control-Allow-Origin")
		resp.Header.Del("Access-Control-Allow-Methods")
		resp.Header.Del("Access-Control-Allow-Headers")
		resp.Header.Del("Access-Control-Expose-Headers")
		resp.Header.Del("Access-Control-Allow-Credentials")
		resp.Header.Del("Access-Control-Max-Age")
		return nil
	}

	return proxy
}
