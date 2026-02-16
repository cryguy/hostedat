package s3

import (
	"bytes"
	"crypto/md5"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/cryguy/hostedat/internal/config"
	"github.com/cryguy/hostedat/internal/models"
	"gorm.io/gorm"
)

// testEnv holds the test environment for S3 handler integration tests.
type testEnv struct {
	handler   *Handler
	db        *gorm.DB
	siteID    string
	accessKey string
	secretKey string
}

func setupTestEnv(t *testing.T) *testEnv {
	t.Helper()

	db, err := models.InitDB(config.DBConfig{Driver: "sqlite", DSN: ":memory:"})
	if err != nil {
		t.Fatalf("InitDB: %v", err)
	}

	storagePath := t.TempDir()

	// Create a test site.
	site := models.Site{
		UserID:        "testuser",
		SubdomainSlug: "test-site",
		Name:          "Test Site",
	}
	if err := db.Create(&site).Error; err != nil {
		t.Fatalf("create site: %v", err)
	}

	// Create storage credentials.
	accessKey := "HDAK0123456789abcdef"
	secretKey := "secretkey0123456789abcdef01234567890abc"

	cred := models.StorageCredential{
		SiteID:          site.ID,
		AccessKeyID:     accessKey,
		SecretAccessKey: secretKey,
	}
	if err := db.Create(&cred).Error; err != nil {
		t.Fatalf("create credential: %v", err)
	}

	handler := NewHandler(db, storagePath)

	return &testEnv{
		handler:   handler,
		db:        db,
		siteID:    site.ID,
		accessKey: accessKey,
		secretKey: secretKey,
	}
}

func (env *testEnv) signedRequest(method, target string, body []byte) *http.Request {
	var bodyReader io.Reader
	if body != nil {
		bodyReader = bytes.NewReader(body)
	}
	// Separate path from query string, then encode each path segment so
	// httptest.NewRequest can parse the URL (e.g. spaces become %20).
	pathPart, queryPart, hasQuery := strings.Cut(target, "?")
	segments := strings.Split(pathPart, "/")
	for i, seg := range segments {
		segments[i] = url.PathEscape(seg)
	}
	encodedTarget := strings.Join(segments, "/")
	if hasQuery {
		encodedTarget += "?" + queryPart
	}
	req := httptest.NewRequest(method, encodedTarget, bodyReader)
	req.Host = "localhost"
	signRequest(req, env.accessKey, env.secretKey, "us-east-1", "s3", body)
	return req
}

func (env *testEnv) do(req *http.Request) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	env.handler.ServeHTTP(rec, req)
	return rec
}

// ──────────────────────────────────────────────
// SigV4 Auth Tests
// ──────────────────────────────────────────────

func TestAuth_ValidSignature(t *testing.T) {
	env := setupTestEnv(t)
	body := []byte("hello world")
	req := env.signedRequest(http.MethodPut, "/"+env.siteID+"/auth-test.txt", body)
	req.Header.Set("Content-Type", "text/plain")
	rec := env.do(req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
}

func TestAuth_InvalidAccessKey(t *testing.T) {
	env := setupTestEnv(t)
	body := []byte("test")
	req := httptest.NewRequest(http.MethodPut, "/"+env.siteID+"/test.txt", bytes.NewReader(body))
	req.Host = "localhost"
	signRequest(req, "HDAKINVALIDKEYXXX", env.secretKey, "us-east-1", "s3", body)
	rec := env.do(req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
	var errResp ErrorResponse
	if err := xml.Unmarshal(rec.Body.Bytes(), &errResp); err != nil {
		t.Fatalf("invalid XML: %v", err)
	}
	if errResp.Code != "InvalidAccessKeyId" {
		t.Errorf("error code = %q, want InvalidAccessKeyId", errResp.Code)
	}
}

func TestAuth_InvalidSignature(t *testing.T) {
	env := setupTestEnv(t)
	body := []byte("test")
	req := httptest.NewRequest(http.MethodPut, "/"+env.siteID+"/test.txt", bytes.NewReader(body))
	req.Host = "localhost"
	signRequest(req, env.accessKey, "wrongsecretkey1234567890abcdef012345", "us-east-1", "s3", body)
	rec := env.do(req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
	var errResp ErrorResponse
	if err := xml.Unmarshal(rec.Body.Bytes(), &errResp); err != nil {
		t.Fatalf("invalid XML: %v", err)
	}
	if errResp.Code != "SignatureDoesNotMatch" {
		t.Errorf("error code = %q, want SignatureDoesNotMatch", errResp.Code)
	}
}

func TestAuth_MissingAuthHeader(t *testing.T) {
	env := setupTestEnv(t)
	req := httptest.NewRequest(http.MethodGet, "/"+env.siteID+"/test.txt", nil)
	req.Host = "localhost"
	rec := env.do(req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
}

func TestAuth_ExpiredSignature(t *testing.T) {
	env := setupTestEnv(t)
	body := []byte("test")
	req := httptest.NewRequest(http.MethodGet, "/"+env.siteID+"/test.txt", nil)
	req.Host = "localhost"

	// Use a timestamp 20 minutes in the past.
	oldTime := time.Now().UTC().Add(-20 * time.Minute)
	dateStamp := oldTime.Format("20060102")
	amzDate := oldTime.Format("20060102T150405Z")
	req.Header.Set("X-Amz-Date", amzDate)
	req.Header.Set("X-Amz-Content-Sha256", hashSHA256(body))

	signedHeaders := []string{"host", "x-amz-content-sha256", "x-amz-date"}
	canonicalRequest := buildCanonicalRequest(req, signedHeaders, body)
	scope := dateStamp + "/us-east-1/s3/aws4_request"
	stringToSign := "AWS4-HMAC-SHA256\n" + amzDate + "\n" + scope + "\n" + hashSHA256([]byte(canonicalRequest))
	signingKey := deriveSigningKey(env.secretKey, dateStamp, "us-east-1", "s3")
	signature := hex.EncodeToString(hmacSHA256(signingKey, []byte(stringToSign)))

	authHeader := "AWS4-HMAC-SHA256 Credential=" + env.accessKey + "/" + scope +
		", SignedHeaders=" + strings.Join(signedHeaders, ";") +
		", Signature=" + signature
	req.Header.Set("Authorization", authHeader)

	rec := env.do(req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
	var errResp ErrorResponse
	xml.Unmarshal(rec.Body.Bytes(), &errResp)
	if errResp.Code != "RequestTimeTooSkewed" {
		t.Errorf("error code = %q, want RequestTimeTooSkewed", errResp.Code)
	}
}

// ──────────────────────────────────────────────
// CRUD Tests
// ──────────────────────────────────────────────

func TestPutGetHeadDelete_RoundTrip(t *testing.T) {
	env := setupTestEnv(t)
	key := "hello.txt"
	body := []byte("Hello, World!")
	expectedMD5 := md5.Sum(body)
	expectedETag := fmt.Sprintf(`"%s"`, hex.EncodeToString(expectedMD5[:]))

	// PUT
	putReq := env.signedRequest(http.MethodPut, "/"+env.siteID+"/"+key, body)
	putReq.Header.Set("Content-Type", "text/plain")
	putRec := env.do(putReq)

	if putRec.Code != http.StatusOK {
		t.Fatalf("PUT status = %d, want 200: %s", putRec.Code, putRec.Body.String())
	}
	if etag := putRec.Header().Get("ETag"); etag != expectedETag {
		t.Errorf("PUT ETag = %q, want %q", etag, expectedETag)
	}

	// GET
	getReq := env.signedRequest(http.MethodGet, "/"+env.siteID+"/"+key, nil)
	getRec := env.do(getReq)

	if getRec.Code != http.StatusOK {
		t.Fatalf("GET status = %d, want 200: %s", getRec.Code, getRec.Body.String())
	}
	if getRec.Body.String() != "Hello, World!" {
		t.Errorf("GET body = %q", getRec.Body.String())
	}
	if ct := getRec.Header().Get("Content-Type"); ct != "text/plain" {
		t.Errorf("Content-Type = %q, want text/plain", ct)
	}
	if etag := getRec.Header().Get("ETag"); etag != expectedETag {
		t.Errorf("GET ETag = %q, want %q", etag, expectedETag)
	}
	if getRec.Header().Get("Last-Modified") == "" {
		t.Error("missing Last-Modified header")
	}
	if getRec.Header().Get("Content-Length") != "13" {
		t.Errorf("Content-Length = %q, want 13", getRec.Header().Get("Content-Length"))
	}

	// HEAD
	headReq := env.signedRequest(http.MethodHead, "/"+env.siteID+"/"+key, nil)
	headRec := env.do(headReq)

	if headRec.Code != http.StatusOK {
		t.Fatalf("HEAD status = %d, want 200", headRec.Code)
	}
	if headRec.Header().Get("Content-Type") != "text/plain" {
		t.Errorf("HEAD Content-Type = %q", headRec.Header().Get("Content-Type"))
	}
	if headRec.Header().Get("ETag") != expectedETag {
		t.Errorf("HEAD ETag = %q", headRec.Header().Get("ETag"))
	}
	if headRec.Body.Len() != 0 {
		t.Errorf("HEAD should have empty body, got %d bytes", headRec.Body.Len())
	}

	// DELETE
	delReq := env.signedRequest(http.MethodDelete, "/"+env.siteID+"/"+key, nil)
	delRec := env.do(delReq)

	if delRec.Code != http.StatusNoContent {
		t.Fatalf("DELETE status = %d, want 204", delRec.Code)
	}

	// Verify gone
	getReq2 := env.signedRequest(http.MethodGet, "/"+env.siteID+"/"+key, nil)
	getRec2 := env.do(getReq2)

	if getRec2.Code != http.StatusNotFound {
		t.Fatalf("GET after DELETE status = %d, want 404", getRec2.Code)
	}
	var errResp ErrorResponse
	if err := xml.Unmarshal(getRec2.Body.Bytes(), &errResp); err != nil {
		t.Fatalf("invalid XML: %v", err)
	}
	if errResp.Code != "NoSuchKey" {
		t.Errorf("error code = %q, want NoSuchKey", errResp.Code)
	}
}

func TestPut_ContentTypePreservation(t *testing.T) {
	env := setupTestEnv(t)

	types := map[string]string{
		"test.html": "text/html",
		"test.json": "application/json",
		"test.bin":  "application/octet-stream",
		"test.png":  "image/png",
	}

	for key, ct := range types {
		body := []byte("content")
		req := env.signedRequest(http.MethodPut, "/"+env.siteID+"/"+key, body)
		req.Header.Set("Content-Type", ct)
		env.do(req)

		getReq := env.signedRequest(http.MethodGet, "/"+env.siteID+"/"+key, nil)
		getRec := env.do(getReq)
		if getRec.Header().Get("Content-Type") != ct {
			t.Errorf("key %q: Content-Type = %q, want %q", key, getRec.Header().Get("Content-Type"), ct)
		}
	}
}

func TestGet_404_XML(t *testing.T) {
	env := setupTestEnv(t)
	req := env.signedRequest(http.MethodGet, "/"+env.siteID+"/nonexistent.txt", nil)
	rec := env.do(req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}

	var errResp ErrorResponse
	if err := xml.Unmarshal(rec.Body.Bytes(), &errResp); err != nil {
		t.Fatalf("invalid XML: %v\nbody: %s", err, rec.Body.String())
	}
	if errResp.Code != "NoSuchKey" {
		t.Errorf("Code = %q, want NoSuchKey", errResp.Code)
	}
}

func TestDelete_Idempotent(t *testing.T) {
	env := setupTestEnv(t)
	key := "delete-me.txt"

	// Delete nonexistent key should return 204.
	req := env.signedRequest(http.MethodDelete, "/"+env.siteID+"/"+key, nil)
	rec := env.do(req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("DELETE nonexistent status = %d, want 204", rec.Code)
	}

	// Put then delete twice.
	putReq := env.signedRequest(http.MethodPut, "/"+env.siteID+"/"+key, []byte("data"))
	env.do(putReq)

	delReq1 := env.signedRequest(http.MethodDelete, "/"+env.siteID+"/"+key, nil)
	delRec1 := env.do(delReq1)
	if delRec1.Code != http.StatusNoContent {
		t.Errorf("first DELETE status = %d", delRec1.Code)
	}

	delReq2 := env.signedRequest(http.MethodDelete, "/"+env.siteID+"/"+key, nil)
	delRec2 := env.do(delReq2)
	if delRec2.Code != http.StatusNoContent {
		t.Errorf("second DELETE status = %d", delRec2.Code)
	}
}

func TestPut_ETagIsMD5(t *testing.T) {
	env := setupTestEnv(t)
	body := []byte("test content for MD5")
	expected := md5.Sum(body)
	expectedETag := fmt.Sprintf(`"%s"`, hex.EncodeToString(expected[:]))

	req := env.signedRequest(http.MethodPut, "/"+env.siteID+"/etag-test.txt", body)
	rec := env.do(req)

	if rec.Header().Get("ETag") != expectedETag {
		t.Errorf("ETag = %q, want %q", rec.Header().Get("ETag"), expectedETag)
	}
}

func TestPut_MetadataHeaders(t *testing.T) {
	env := setupTestEnv(t)
	body := []byte("data")
	req := env.signedRequest(http.MethodPut, "/"+env.siteID+"/meta.txt", body)
	req.Header.Set("Content-Type", "text/plain")
	req.Header.Set("X-Amz-Meta-Author", "tester")
	req.Header.Set("X-Amz-Meta-Version", "1.0")
	env.do(req)

	getReq := env.signedRequest(http.MethodGet, "/"+env.siteID+"/meta.txt", nil)
	getRec := env.do(getReq)

	if getRec.Code != http.StatusOK {
		t.Fatalf("GET status = %d", getRec.Code)
	}
	if getRec.Header().Get("X-Amz-Meta-Author") != "tester" {
		t.Errorf("X-Amz-Meta-Author = %q", getRec.Header().Get("X-Amz-Meta-Author"))
	}
	if getRec.Header().Get("X-Amz-Meta-Version") != "1.0" {
		t.Errorf("X-Amz-Meta-Version = %q", getRec.Header().Get("X-Amz-Meta-Version"))
	}
}

// ──────────────────────────────────────────────
// Overwrite Tests
// ──────────────────────────────────────────────

func TestOverwrite_UpdatesContentAndETag(t *testing.T) {
	env := setupTestEnv(t)
	key := "overwrite.txt"

	// First put.
	body1 := []byte("version 1")
	req1 := env.signedRequest(http.MethodPut, "/"+env.siteID+"/"+key, body1)
	rec1 := env.do(req1)
	etag1 := rec1.Header().Get("ETag")

	// Second put.
	body2 := []byte("version 2 with different content")
	req2 := env.signedRequest(http.MethodPut, "/"+env.siteID+"/"+key, body2)
	rec2 := env.do(req2)
	etag2 := rec2.Header().Get("ETag")

	if etag1 == etag2 {
		t.Error("ETag should change when content changes")
	}

	// Verify content is updated.
	getReq := env.signedRequest(http.MethodGet, "/"+env.siteID+"/"+key, nil)
	getRec := env.do(getReq)
	if getRec.Body.String() != "version 2 with different content" {
		t.Errorf("body = %q, want version 2", getRec.Body.String())
	}
}

// ──────────────────────────────────────────────
// ListObjectsV2 Tests
// ──────────────────────────────────────────────

func TestListObjectsV2_Basic(t *testing.T) {
	env := setupTestEnv(t)

	// Put some objects.
	keys := []string{"a.txt", "b.txt", "c.txt"}
	for _, key := range keys {
		req := env.signedRequest(http.MethodPut, "/"+env.siteID+"/"+key, []byte("data"))
		env.do(req)
	}

	// List.
	listReq := env.signedRequest(http.MethodGet, "/"+env.siteID+"?list-type=2", nil)
	listRec := env.do(listReq)

	if listRec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", listRec.Code, listRec.Body.String())
	}

	var result ListBucketResult
	if err := xml.Unmarshal(listRec.Body.Bytes(), &result); err != nil {
		t.Fatalf("invalid XML: %v", err)
	}

	if result.KeyCount != 3 {
		t.Errorf("KeyCount = %d, want 3", result.KeyCount)
	}
	if len(result.Contents) != 3 {
		t.Errorf("Contents length = %d, want 3", len(result.Contents))
	}
	if result.IsTruncated {
		t.Error("should not be truncated")
	}
}

func TestListObjectsV2_Prefix(t *testing.T) {
	env := setupTestEnv(t)

	keys := []string{"photos/a.jpg", "photos/b.jpg", "docs/readme.md"}
	for _, key := range keys {
		req := env.signedRequest(http.MethodPut, "/"+env.siteID+"/"+key, []byte("data"))
		env.do(req)
	}

	listReq := env.signedRequest(http.MethodGet, "/"+env.siteID+"?list-type=2&prefix=photos/", nil)
	listRec := env.do(listReq)

	var result ListBucketResult
	xml.Unmarshal(listRec.Body.Bytes(), &result)

	if result.KeyCount != 2 {
		t.Errorf("KeyCount = %d, want 2", result.KeyCount)
	}
	for _, obj := range result.Contents {
		if !strings.HasPrefix(obj.Key, "photos/") {
			t.Errorf("unexpected key %q in prefix results", obj.Key)
		}
	}
}

func TestListObjectsV2_Delimiter(t *testing.T) {
	env := setupTestEnv(t)

	keys := []string{"photos/a.jpg", "photos/b.jpg", "docs/readme.md", "root.txt"}
	for _, key := range keys {
		req := env.signedRequest(http.MethodPut, "/"+env.siteID+"/"+key, []byte("data"))
		env.do(req)
	}

	listReq := env.signedRequest(http.MethodGet, "/"+env.siteID+"?list-type=2&delimiter=/", nil)
	listRec := env.do(listReq)

	var result ListBucketResult
	xml.Unmarshal(listRec.Body.Bytes(), &result)

	// root.txt should be in Contents, photos/ and docs/ in CommonPrefixes.
	if len(result.Contents) != 1 {
		t.Errorf("Contents length = %d, want 1", len(result.Contents))
	}
	if len(result.Contents) == 1 && result.Contents[0].Key != "root.txt" {
		t.Errorf("Contents[0].Key = %q, want root.txt", result.Contents[0].Key)
	}
	if len(result.CommonPrefixes) != 2 {
		t.Errorf("CommonPrefixes length = %d, want 2", len(result.CommonPrefixes))
	}
}

func TestListObjectsV2_Pagination(t *testing.T) {
	env := setupTestEnv(t)

	for i := 0; i < 5; i++ {
		key := fmt.Sprintf("file-%02d.txt", i)
		req := env.signedRequest(http.MethodPut, "/"+env.siteID+"/"+key, []byte("data"))
		env.do(req)
	}

	// List with max-keys=2.
	listReq := env.signedRequest(http.MethodGet, "/"+env.siteID+"?list-type=2&max-keys=2", nil)
	listRec := env.do(listReq)

	var result ListBucketResult
	xml.Unmarshal(listRec.Body.Bytes(), &result)

	if !result.IsTruncated {
		t.Error("should be truncated")
	}
	if result.MaxKeys != 2 {
		t.Errorf("MaxKeys = %d, want 2", result.MaxKeys)
	}
	if len(result.Contents) != 2 {
		t.Errorf("Contents length = %d, want 2", len(result.Contents))
	}
	if result.NextContinuationToken == "" {
		t.Error("should have NextContinuationToken")
	}

	// Use continuation token for next page.
	listReq2 := env.signedRequest(http.MethodGet, "/"+env.siteID+"?list-type=2&max-keys=2&continuation-token="+result.NextContinuationToken, nil)
	listRec2 := env.do(listReq2)

	var result2 ListBucketResult
	xml.Unmarshal(listRec2.Body.Bytes(), &result2)

	if len(result2.Contents) != 2 {
		t.Errorf("page 2 Contents length = %d, want 2", len(result2.Contents))
	}
	if result2.Contents[0].Key == result.Contents[0].Key {
		t.Error("page 2 should have different keys than page 1")
	}
}

func TestListObjectsV2_Empty(t *testing.T) {
	env := setupTestEnv(t)

	listReq := env.signedRequest(http.MethodGet, "/"+env.siteID+"?list-type=2", nil)
	listRec := env.do(listReq)

	var result ListBucketResult
	xml.Unmarshal(listRec.Body.Bytes(), &result)

	if result.KeyCount != 0 {
		t.Errorf("KeyCount = %d, want 0", result.KeyCount)
	}
	if result.IsTruncated {
		t.Error("should not be truncated")
	}
}

func TestListObjectsV2_StartAfter(t *testing.T) {
	env := setupTestEnv(t)

	keys := []string{"a.txt", "b.txt", "c.txt", "d.txt"}
	for _, key := range keys {
		req := env.signedRequest(http.MethodPut, "/"+env.siteID+"/"+key, []byte("data"))
		env.do(req)
	}

	listReq := env.signedRequest(http.MethodGet, "/"+env.siteID+"?list-type=2&start-after=b.txt", nil)
	listRec := env.do(listReq)

	var result ListBucketResult
	xml.Unmarshal(listRec.Body.Bytes(), &result)

	if result.KeyCount != 2 {
		t.Errorf("KeyCount = %d, want 2", result.KeyCount)
	}
	if len(result.Contents) > 0 && result.Contents[0].Key != "c.txt" {
		t.Errorf("first key = %q, want c.txt", result.Contents[0].Key)
	}
}

// ──────────────────────────────────────────────
// Range Request Tests
// ──────────────────────────────────────────────

func TestRange_ByteRange(t *testing.T) {
	env := setupTestEnv(t)
	body := []byte("0123456789abcdef")
	req := env.signedRequest(http.MethodPut, "/"+env.siteID+"/range.txt", body)
	env.do(req)

	// bytes=0-3
	getReq := env.signedRequest(http.MethodGet, "/"+env.siteID+"/range.txt", nil)
	getReq.Header.Set("Range", "bytes=0-3")
	getRec := env.do(getReq)

	if getRec.Code != http.StatusPartialContent {
		t.Fatalf("status = %d, want 206", getRec.Code)
	}
	if getRec.Body.String() != "0123" {
		t.Errorf("body = %q, want 0123", getRec.Body.String())
	}
	if cr := getRec.Header().Get("Content-Range"); !strings.HasPrefix(cr, "bytes 0-3/16") {
		t.Errorf("Content-Range = %q", cr)
	}
}

func TestRange_SuffixRange(t *testing.T) {
	env := setupTestEnv(t)
	body := []byte("0123456789abcdef")
	req := env.signedRequest(http.MethodPut, "/"+env.siteID+"/range-suffix.txt", body)
	env.do(req)

	getReq := env.signedRequest(http.MethodGet, "/"+env.siteID+"/range-suffix.txt", nil)
	getReq.Header.Set("Range", "bytes=-4")
	getRec := env.do(getReq)

	if getRec.Code != http.StatusPartialContent {
		t.Fatalf("status = %d, want 206", getRec.Code)
	}
	if getRec.Body.String() != "cdef" {
		t.Errorf("body = %q, want cdef", getRec.Body.String())
	}
}

func TestRange_InvalidRange(t *testing.T) {
	env := setupTestEnv(t)
	body := []byte("short")
	req := env.signedRequest(http.MethodPut, "/"+env.siteID+"/range-invalid.txt", body)
	env.do(req)

	getReq := env.signedRequest(http.MethodGet, "/"+env.siteID+"/range-invalid.txt", nil)
	getReq.Header.Set("Range", "bytes=100-200")
	getRec := env.do(getReq)

	if getRec.Code != http.StatusRequestedRangeNotSatisfiable {
		t.Fatalf("status = %d, want 416", getRec.Code)
	}
}

// ──────────────────────────────────────────────
// Conditional Request Tests
// ──────────────────────────────────────────────

func TestConditional_IfNoneMatch_304(t *testing.T) {
	env := setupTestEnv(t)
	body := []byte("data")
	putReq := env.signedRequest(http.MethodPut, "/"+env.siteID+"/cond.txt", body)
	putRec := env.do(putReq)
	etag := putRec.Header().Get("ETag")

	getReq := env.signedRequest(http.MethodGet, "/"+env.siteID+"/cond.txt", nil)
	getReq.Header.Set("If-None-Match", etag)
	getRec := env.do(getReq)

	if getRec.Code != http.StatusNotModified {
		t.Fatalf("status = %d, want 304", getRec.Code)
	}
}

func TestConditional_IfMatch_412(t *testing.T) {
	env := setupTestEnv(t)
	body := []byte("data")
	putReq := env.signedRequest(http.MethodPut, "/"+env.siteID+"/cond-match.txt", body)
	env.do(putReq)

	getReq := env.signedRequest(http.MethodGet, "/"+env.siteID+"/cond-match.txt", nil)
	getReq.Header.Set("If-Match", `"wrongetag"`)
	getRec := env.do(getReq)

	if getRec.Code != http.StatusPreconditionFailed {
		t.Fatalf("status = %d, want 412", getRec.Code)
	}
}

func TestConditional_IfModifiedSince_304(t *testing.T) {
	env := setupTestEnv(t)
	body := []byte("data")
	putReq := env.signedRequest(http.MethodPut, "/"+env.siteID+"/cond-mod.txt", body)
	env.do(putReq)

	// Use a future time.
	futureTime := time.Now().Add(1 * time.Hour).UTC().Format(http.TimeFormat)
	getReq := env.signedRequest(http.MethodGet, "/"+env.siteID+"/cond-mod.txt", nil)
	getReq.Header.Set("If-Modified-Since", futureTime)
	getRec := env.do(getReq)

	if getRec.Code != http.StatusNotModified {
		t.Fatalf("status = %d, want 304", getRec.Code)
	}
}

func TestConditional_IfUnmodifiedSince_412(t *testing.T) {
	env := setupTestEnv(t)
	body := []byte("data")
	putReq := env.signedRequest(http.MethodPut, "/"+env.siteID+"/cond-unmod.txt", body)
	env.do(putReq)

	// Use a past time.
	pastTime := time.Now().Add(-1 * time.Hour).UTC().Format(http.TimeFormat)
	getReq := env.signedRequest(http.MethodGet, "/"+env.siteID+"/cond-unmod.txt", nil)
	getReq.Header.Set("If-Unmodified-Since", pastTime)
	getRec := env.do(getReq)

	if getRec.Code != http.StatusPreconditionFailed {
		t.Fatalf("status = %d, want 412", getRec.Code)
	}
}

// ──────────────────────────────────────────────
// CopyObject Tests
// ──────────────────────────────────────────────

func TestCopyObject_SameBucket(t *testing.T) {
	env := setupTestEnv(t)
	body := []byte("original content")
	putReq := env.signedRequest(http.MethodPut, "/"+env.siteID+"/src.txt", body)
	putReq.Header.Set("Content-Type", "text/plain")
	putReq.Header.Set("X-Amz-Meta-Author", "tester")
	env.do(putReq)

	// Copy.
	copyReq := env.signedRequest(http.MethodPut, "/"+env.siteID+"/dest.txt", nil)
	copyReq.Header.Set("X-Amz-Copy-Source", "/"+env.siteID+"/src.txt")
	copyRec := env.do(copyReq)

	if copyRec.Code != http.StatusOK {
		t.Fatalf("COPY status = %d, want 200: %s", copyRec.Code, copyRec.Body.String())
	}

	var result CopyObjectResult
	if err := xml.Unmarshal(copyRec.Body.Bytes(), &result); err != nil {
		t.Fatalf("invalid XML: %v", err)
	}
	if result.ETag == "" {
		t.Error("ETag should not be empty")
	}

	// Verify the copy exists with same content.
	getReq := env.signedRequest(http.MethodGet, "/"+env.siteID+"/dest.txt", nil)
	getRec := env.do(getReq)

	if getRec.Body.String() != "original content" {
		t.Errorf("copied body = %q", getRec.Body.String())
	}
	if getRec.Header().Get("Content-Type") != "text/plain" {
		t.Errorf("copied Content-Type = %q", getRec.Header().Get("Content-Type"))
	}
}

func TestCopyObject_MetadataReplace(t *testing.T) {
	env := setupTestEnv(t)
	body := []byte("content")
	putReq := env.signedRequest(http.MethodPut, "/"+env.siteID+"/copy-src.txt", body)
	putReq.Header.Set("Content-Type", "text/plain")
	putReq.Header.Set("X-Amz-Meta-Original", "yes")
	env.do(putReq)

	// Copy with metadata replace.
	copyReq := env.signedRequest(http.MethodPut, "/"+env.siteID+"/copy-dest.txt", nil)
	copyReq.Header.Set("X-Amz-Copy-Source", "/"+env.siteID+"/copy-src.txt")
	copyReq.Header.Set("X-Amz-Metadata-Directive", "REPLACE")
	copyReq.Header.Set("Content-Type", "application/json")
	copyReq.Header.Set("X-Amz-Meta-Replaced", "yes")
	env.do(copyReq)

	getReq := env.signedRequest(http.MethodGet, "/"+env.siteID+"/copy-dest.txt", nil)
	getRec := env.do(getReq)

	if getRec.Header().Get("Content-Type") != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", getRec.Header().Get("Content-Type"))
	}
	if getRec.Header().Get("X-Amz-Meta-Replaced") != "yes" {
		t.Errorf("X-Amz-Meta-Replaced = %q", getRec.Header().Get("X-Amz-Meta-Replaced"))
	}
}

// ──────────────────────────────────────────────
// DeleteObjects (Batch) Tests
// ──────────────────────────────────────────────

func TestDeleteObjects_Batch(t *testing.T) {
	env := setupTestEnv(t)

	for _, key := range []string{"batch1.txt", "batch2.txt", "batch3.txt"} {
		req := env.signedRequest(http.MethodPut, "/"+env.siteID+"/"+key, []byte("data"))
		env.do(req)
	}

	deleteXML := `<?xml version="1.0" encoding="UTF-8"?>
<Delete>
  <Object><Key>batch1.txt</Key></Object>
  <Object><Key>batch2.txt</Key></Object>
  <Object><Key>nonexistent.txt</Key></Object>
</Delete>`

	delReq := env.signedRequest(http.MethodPost, "/"+env.siteID+"?delete", []byte(deleteXML))
	delReq.Header.Set("Content-Type", "application/xml")
	delRec := env.do(delReq)

	if delRec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", delRec.Code, delRec.Body.String())
	}

	var result DeleteResult
	if err := xml.Unmarshal(delRec.Body.Bytes(), &result); err != nil {
		t.Fatalf("invalid XML: %v", err)
	}

	if len(result.Deleted) != 3 {
		t.Errorf("Deleted count = %d, want 3", len(result.Deleted))
	}

	// Verify batch1 and batch2 are gone but batch3 remains.
	getReq1 := env.signedRequest(http.MethodGet, "/"+env.siteID+"/batch1.txt", nil)
	if env.do(getReq1).Code != http.StatusNotFound {
		t.Error("batch1.txt should be deleted")
	}

	getReq3 := env.signedRequest(http.MethodGet, "/"+env.siteID+"/batch3.txt", nil)
	if env.do(getReq3).Code != http.StatusOK {
		t.Error("batch3.txt should still exist")
	}
}

func TestDeleteObjects_QuietMode(t *testing.T) {
	env := setupTestEnv(t)

	req := env.signedRequest(http.MethodPut, "/"+env.siteID+"/quiet-del.txt", []byte("data"))
	env.do(req)

	deleteXML := `<?xml version="1.0" encoding="UTF-8"?>
<Delete>
  <Quiet>true</Quiet>
  <Object><Key>quiet-del.txt</Key></Object>
</Delete>`

	delReq := env.signedRequest(http.MethodPost, "/"+env.siteID+"?delete", []byte(deleteXML))
	delReq.Header.Set("Content-Type", "application/xml")
	delRec := env.do(delReq)

	var result DeleteResult
	xml.Unmarshal(delRec.Body.Bytes(), &result)

	if len(result.Deleted) != 0 {
		t.Errorf("Quiet mode should not list Deleted items, got %d", len(result.Deleted))
	}
}

// ──────────────────────────────────────────────
// Key Edge Cases
// ──────────────────────────────────────────────

func TestKeyEdgeCases_Unicode(t *testing.T) {
	env := setupTestEnv(t)
	key := "日本語/テスト.txt"
	body := []byte("unicode content")
	req := env.signedRequest(http.MethodPut, "/"+env.siteID+"/"+key, body)
	rec := env.do(req)
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT unicode key status = %d: %s", rec.Code, rec.Body.String())
	}

	getReq := env.signedRequest(http.MethodGet, "/"+env.siteID+"/"+key, nil)
	getRec := env.do(getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("GET unicode key status = %d", getRec.Code)
	}
	if getRec.Body.String() != "unicode content" {
		t.Errorf("body = %q", getRec.Body.String())
	}
}

func TestKeyEdgeCases_Spaces(t *testing.T) {
	env := setupTestEnv(t)
	key := "path with spaces/file name.txt"
	body := []byte("spaces content")
	req := env.signedRequest(http.MethodPut, "/"+env.siteID+"/"+key, body)
	rec := env.do(req)
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT key with spaces status = %d: %s", rec.Code, rec.Body.String())
	}

	getReq := env.signedRequest(http.MethodGet, "/"+env.siteID+"/"+key, nil)
	getRec := env.do(getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("GET key with spaces status = %d", getRec.Code)
	}
}

func TestKeyEdgeCases_DeeplyNested(t *testing.T) {
	env := setupTestEnv(t)
	key := "a/b/c/d/e/f/g/h/deep.txt"
	body := []byte("deep content")
	req := env.signedRequest(http.MethodPut, "/"+env.siteID+"/"+key, body)
	rec := env.do(req)
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT deeply nested key status = %d", rec.Code)
	}

	getReq := env.signedRequest(http.MethodGet, "/"+env.siteID+"/"+key, nil)
	getRec := env.do(getReq)
	if getRec.Body.String() != "deep content" {
		t.Errorf("body = %q", getRec.Body.String())
	}
}

func TestKeyEdgeCases_SpecialChars(t *testing.T) {
	env := setupTestEnv(t)
	key := "special!@#$%&().txt"
	body := []byte("special chars")
	req := env.signedRequest(http.MethodPut, "/"+env.siteID+"/"+key, body)
	rec := env.do(req)
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT special chars key status = %d: %s", rec.Code, rec.Body.String())
	}
}

// ──────────────────────────────────────────────
// Isolation Tests
// ──────────────────────────────────────────────

func TestIsolation_CrossSiteAccessDenied(t *testing.T) {
	env := setupTestEnv(t)

	// Create a second site.
	site2 := models.Site{
		UserID:        "otheruser",
		SubdomainSlug: "other-site",
		Name:          "Other Site",
	}
	env.db.Create(&site2)

	// Put an object in site2 (directly in DB for setup).
	obj := models.StorageObject{
		SiteID:       site2.ID,
		Key:          "secret.txt",
		Size:         6,
		ContentType:  "text/plain",
		ETag:         `"abc"`,
		LastModified: time.Now(),
		StoragePath:  "/nonexistent",
	}
	env.db.Create(&obj)

	// Try to access site2's objects using site1's credentials.
	getReq := env.signedRequest(http.MethodGet, "/"+site2.ID+"/secret.txt", nil)
	getRec := env.do(getReq)

	if getRec.Code != http.StatusForbidden {
		t.Fatalf("cross-site access status = %d, want 403", getRec.Code)
	}
}

// ──────────────────────────────────────────────
// Response Format Tests
// ──────────────────────────────────────────────

func TestResponseFormat_ErrorIsValidXML(t *testing.T) {
	env := setupTestEnv(t)

	// Missing auth.
	req := httptest.NewRequest(http.MethodGet, "/"+env.siteID+"/anything.txt", nil)
	req.Host = "localhost"
	rec := env.do(req)

	if ct := rec.Header().Get("Content-Type"); ct != "application/xml" {
		t.Errorf("Content-Type = %q, want application/xml", ct)
	}

	// Must be parseable XML.
	var errResp ErrorResponse
	if err := xml.Unmarshal(rec.Body.Bytes(), &errResp); err != nil {
		t.Fatalf("invalid XML: %v\nbody: %s", err, rec.Body.String())
	}

	// Must have XML declaration.
	if !strings.HasPrefix(rec.Body.String(), "<?xml") {
		t.Error("response should start with XML declaration")
	}
}

func TestResponseFormat_ListIsValidXML(t *testing.T) {
	env := setupTestEnv(t)

	req := env.signedRequest(http.MethodPut, "/"+env.siteID+"/xmltest.txt", []byte("data"))
	env.do(req)

	listReq := env.signedRequest(http.MethodGet, "/"+env.siteID+"?list-type=2", nil)
	listRec := env.do(listReq)

	if !strings.HasPrefix(listRec.Body.String(), "<?xml") {
		t.Error("list response should start with XML declaration")
	}

	var result ListBucketResult
	if err := xml.Unmarshal(listRec.Body.Bytes(), &result); err != nil {
		t.Fatalf("invalid XML: %v", err)
	}

	// Verify proper namespace.
	if result.XMLNS != "http://s3.amazonaws.com/doc/2006-03-01/" {
		t.Errorf("xmlns = %q", result.XMLNS)
	}
}

// ──────────────────────────────────────────────
// Multipart Upload Tests
// ──────────────────────────────────────────────

func TestMultipartUpload_CreateUploadComplete(t *testing.T) {
	env := setupTestEnv(t)
	key := "multipart.bin"

	// Create multipart upload.
	createReq := env.signedRequest(http.MethodPost, "/"+env.siteID+"/"+key+"?uploads", nil)
	createRec := env.do(createReq)

	if createRec.Code != http.StatusOK {
		t.Fatalf("CreateMultipartUpload status = %d: %s", createRec.Code, createRec.Body.String())
	}

	var initResult InitiateMultipartUploadResult
	if err := xml.Unmarshal(createRec.Body.Bytes(), &initResult); err != nil {
		t.Fatalf("invalid XML: %v", err)
	}
	if initResult.UploadId == "" {
		t.Fatal("UploadId should not be empty")
	}
	uploadID := initResult.UploadId

	// Upload parts.
	part1Body := []byte("part1 data here")
	part1Req := env.signedRequest(http.MethodPut, fmt.Sprintf("/%s/%s?uploadId=%s&partNumber=1", env.siteID, key, uploadID), part1Body)
	part1Rec := env.do(part1Req)
	if part1Rec.Code != http.StatusOK {
		t.Fatalf("UploadPart 1 status = %d", part1Rec.Code)
	}
	part1ETag := part1Rec.Header().Get("ETag")

	part2Body := []byte("part2 data here")
	part2Req := env.signedRequest(http.MethodPut, fmt.Sprintf("/%s/%s?uploadId=%s&partNumber=2", env.siteID, key, uploadID), part2Body)
	part2Rec := env.do(part2Req)
	if part2Rec.Code != http.StatusOK {
		t.Fatalf("UploadPart 2 status = %d", part2Rec.Code)
	}
	part2ETag := part2Rec.Header().Get("ETag")

	// Complete multipart upload.
	completeXML := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<CompleteMultipartUpload>
  <Part><PartNumber>1</PartNumber><ETag>%s</ETag></Part>
  <Part><PartNumber>2</PartNumber><ETag>%s</ETag></Part>
</CompleteMultipartUpload>`, part1ETag, part2ETag)

	completeReq := env.signedRequest(http.MethodPost, fmt.Sprintf("/%s/%s?uploadId=%s", env.siteID, key, uploadID), []byte(completeXML))
	completeRec := env.do(completeReq)

	if completeRec.Code != http.StatusOK {
		t.Fatalf("CompleteMultipartUpload status = %d: %s", completeRec.Code, completeRec.Body.String())
	}

	var completeResult CompleteMultipartUploadResult
	if err := xml.Unmarshal(completeRec.Body.Bytes(), &completeResult); err != nil {
		t.Fatalf("invalid XML: %v", err)
	}
	if completeResult.ETag == "" {
		t.Error("ETag should not be empty")
	}

	// Verify the object exists and has combined content.
	getReq := env.signedRequest(http.MethodGet, "/"+env.siteID+"/"+key, nil)
	getRec := env.do(getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("GET after complete status = %d", getRec.Code)
	}
	expected := "part1 data herepart2 data here"
	if getRec.Body.String() != expected {
		t.Errorf("body = %q, want %q", getRec.Body.String(), expected)
	}
}
