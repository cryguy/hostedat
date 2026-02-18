package api

import (
	"io"
	"mime"
	"net/http"
	"net/http/httputil"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/cryguy/hostedat/internal/models"
	minio "github.com/minio/minio-go/v7"
	"gorm.io/gorm"
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
	// Do NOT rewrite req.Host — SigV4 signatures include the Host header,
	// so the forwarded request must preserve the original Host that the
	// client (or presigned URL) was signed for. The proxy routes to the
	// backend via req.URL.Host (set by the default Director).

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

// NewPublicS3Wrapper wraps the existing S3 proxy to serve objects from public
// buckets without authentication. Unauthenticated GET/HEAD requests for public
// buckets are served directly via the minio client. All other requests (or
// requests for non-public buckets) pass through to the underlying S3 proxy.
func NewPublicS3Wrapper(s3Proxy http.Handler, db *gorm.DB, s3Client bucketClient) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		// Only intercept unauthenticated GET/HEAD requests.
		if (req.Method != http.MethodGet && req.Method != http.MethodHead) || hasSigV4Signature(req) {
			s3Proxy.ServeHTTP(w, req)
			return
		}

		bucketName, objectKey := extractBucketAndKey(req.URL.Path)
		if bucketName == "" || objectKey == "" {
			s3Proxy.ServeHTTP(w, req)
			return
		}

		// Check if this bucket exists and is marked public.
		var bucket models.StorageBucket
		if err := db.Where("bucket_name = ? AND public = ?", bucketName, true).First(&bucket).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				http.Error(w, "access denied", http.StatusForbidden)
				return
			}
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		// Serve the object directly via minio client.
		obj, err := s3Client.GetObject(req.Context(), bucketName, objectKey, minio.GetObjectOptions{})
		if err != nil {
			http.Error(w, "failed to retrieve object", http.StatusInternalServerError)
			return
		}
		defer obj.Close()

		info, err := obj.Stat()
		if err != nil {
			errResp := minio.ToErrorResponse(err)
			if errResp.Code == "NoSuchKey" {
				http.Error(w, "not found", http.StatusNotFound)
				return
			}
			http.Error(w, "failed to retrieve object", http.StatusInternalServerError)
			return
		}

		contentType := info.ContentType
		if contentType == "" {
			contentType = mime.TypeByExtension(filepath.Ext(objectKey))
		}
		if contentType == "" {
			contentType = "application/octet-stream"
		}

		w.Header().Set("Content-Type", contentType)
		w.Header().Set("Content-Length", strconv.FormatInt(info.Size, 10))
		w.Header().Set("Cache-Control", "public, max-age=3600")
		w.Header().Set("ETag", info.ETag)

		if req.Method == http.MethodHead {
			w.WriteHeader(http.StatusOK)
			return
		}

		w.WriteHeader(http.StatusOK)
		io.Copy(w, obj)
	})
}

// extractBucketAndKey splits a path like "/bucketName/path/to/object" into
// bucket name and object key. Returns empty strings if the path is invalid.
func extractBucketAndKey(path string) (string, string) {
	path = strings.TrimPrefix(path, "/")
	if path == "" {
		return "", ""
	}
	idx := strings.IndexByte(path, '/')
	if idx < 0 || idx == len(path)-1 {
		return "", ""
	}
	return path[:idx], path[idx+1:]
}
