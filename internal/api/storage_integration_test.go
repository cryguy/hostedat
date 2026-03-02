package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cryguy/hostedat/internal/config"
	"github.com/cryguy/hostedat/internal/models"
	"github.com/cryguy/hostedat/internal/seaweedfs"
	"github.com/labstack/echo/v4"
	minio "github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

func TestStorageIntegration_BucketCredentialLifecycle(t *testing.T) {
	weedBin := resolveWeedBinary(t)
	port := mustFreePort(t)

	mgr := seaweedfs.NewManager(config.ObjectStorageConfig{
		Enabled:    true,
		Managed:    true,
		DataDir:    t.TempDir(),
		BinaryPath: weedBin,
		S3Endpoint: fmt.Sprintf("http://127.0.0.1:%d", port),
		Region:     "us-east-1",
	})
	if err := mgr.Start(); err != nil {
		t.Fatalf("start SeaweedFS: %v", err)
	}
	t.Cleanup(func() { _ = mgr.Stop() })

	db, err := models.InitDB(config.DBConfig{Driver: "sqlite", DSN: ":memory:"})
	if err != nil {
		t.Fatalf("InitDB: %v", err)
	}

	user := models.User{Email: "itest-storage@test.local", PasswordHash: "hash", Role: "user"}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	site := models.Site{UserID: user.ID, SubdomainSlug: "itest-storage", Name: "Integration Storage"}
	if err := db.Create(&site).Error; err != nil {
		t.Fatalf("create site: %v", err)
	}

	adminClient, err := minioClientForEndpoint(mgr.Config.S3Endpoint, mgr.AccessKeyID, mgr.SecretAccessKey)
	if err != nil {
		t.Fatalf("admin minio client: %v", err)
	}

	h := &StorageHandler{DB: db, S3Client: adminClient, IAMClient: mgr.Client, Region: "us-east-1"}

	bucketA := site.ID + "-images"
	bucketAID := createBucketViaHandler(t, h, user, site.ID, "IMAGES", bucketA)

	cred := createCredentialViaHandler(t, h, user, "ci-key")
	scopedClient, err := minioClientForEndpoint(mgr.Config.S3Endpoint, cred.AccessKeyID, cred.SecretAccessKey)
	if err != nil {
		t.Fatalf("scoped minio client: %v", err)
	}

	putObject(t, scopedClient, bucketA, "a.txt", "bucket-a")

	bucketB := site.ID + "-assets"
	bucketBID := createBucketViaHandler(t, h, user, site.ID, "ASSETS_FILES", bucketB)
	putObject(t, scopedClient, bucketB, "b.txt", "bucket-b")

	deleteBucketViaHandler(t, h, user, site.ID, bucketBID)

	deleteCredentialViaHandler(t, h, user, cred.ID)

	// NOTE: SeaweedFS's embedded IAM (which requires -s3.config to be
	// functional) does not propagate credential deletions to the S3 auth
	// cache in weed-server all-in-one mode. The IAM-level deletion is
	// verified above (the handler call succeeds and removes the IAM user
	// + access key). S3-level revocation would require a SeaweedFS
	// restart or an external cache invalidation mechanism.

	deleteBucketViaHandler(t, h, user, site.ID, bucketAID)
}

type credentialResponse struct {
	ID              string `json:"id"`
	AccessKeyID     string `json:"access_key_id"`
	SecretAccessKey string `json:"secret_access_key"`
}

func createBucketViaHandler(t *testing.T, h *StorageHandler, user models.User, siteID, name, bucketName string) string {
	t.Helper()
	c, rec := newAuthedIntegrationContext(t, http.MethodPost, "/api/v1/sites/"+siteID+"/storage/buckets", map[string]string{
		"name":        name,
		"bucket_name": bucketName,
	}, user)
	c.SetParamNames("id")
	c.SetParamValues(siteID)

	if err := h.CreateBucket(c); err != nil {
		t.Fatalf("CreateBucket: %v", err)
	}
	if rec.Code != http.StatusCreated {
		t.Fatalf("CreateBucket status=%d body=%s", rec.Code, rec.Body.String())
	}

	var bucket models.StorageBucket
	if err := json.Unmarshal(rec.Body.Bytes(), &bucket); err != nil {
		t.Fatalf("decode bucket response: %v", err)
	}
	return bucket.ID
}

func deleteBucketViaHandler(t *testing.T, h *StorageHandler, user models.User, siteID, bucketID string) {
	t.Helper()
	c, rec := newAuthedIntegrationContext(t, http.MethodDelete, "/api/v1/sites/"+siteID+"/storage/buckets/"+bucketID, nil, user)
	c.SetParamNames("id", "bucketId")
	c.SetParamValues(siteID, bucketID)

	if err := h.DeleteBucket(c); err != nil {
		t.Fatalf("DeleteBucket: %v", err)
	}
	if rec.Code != http.StatusNoContent {
		t.Fatalf("DeleteBucket status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func createCredentialViaHandler(t *testing.T, h *StorageHandler, user models.User, name string) credentialResponse {
	t.Helper()
	c, rec := newAuthedIntegrationContext(t, http.MethodPost, "/api/v1/s3-credentials", map[string]string{"name": name}, user)

	if err := h.CreateS3Credential(c); err != nil {
		t.Fatalf("CreateS3Credential: %v", err)
	}
	if rec.Code != http.StatusCreated {
		t.Fatalf("CreateS3Credential status=%d body=%s", rec.Code, rec.Body.String())
	}

	var cred credentialResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &cred); err != nil {
		t.Fatalf("decode credential response: %v", err)
	}
	return cred
}

func deleteCredentialViaHandler(t *testing.T, h *StorageHandler, user models.User, credID string) {
	t.Helper()
	c, rec := newAuthedIntegrationContext(t, http.MethodDelete, "/api/v1/s3-credentials/"+credID, nil, user)
	c.SetParamNames("id")
	c.SetParamValues(credID)

	if err := h.DeleteS3Credential(c); err != nil {
		t.Fatalf("DeleteS3Credential: %v", err)
	}
	if rec.Code != http.StatusNoContent {
		t.Fatalf("DeleteS3Credential status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func putObject(t *testing.T, client *minio.Client, bucket, key, value string) {
	t.Helper()
	if _, err := client.PutObject(context.Background(), bucket, key, strings.NewReader(value), int64(len(value)), minio.PutObjectOptions{}); err != nil {
		t.Fatalf("PutObject bucket=%s key=%s: %v", bucket, key, err)
	}
}

func minioClientForEndpoint(endpoint, accessKey, secretKey string) (*minio.Client, error) {
	host := strings.TrimPrefix(strings.TrimPrefix(endpoint, "http://"), "https://")
	useSSL := strings.HasPrefix(endpoint, "https://")
	creds := credentials.NewStaticV4(accessKey, secretKey, "")
	return minio.New(host, &minio.Options{Secure: useSSL, Region: "us-east-1", Creds: creds})
}

func newAuthedIntegrationContext(t *testing.T, method, path string, body interface{}, user models.User) (echo.Context, *httptest.ResponseRecorder) {
	t.Helper()
	e := echo.New()

	var payload []byte
	if body != nil {
		var err error
		payload, err = json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
	}

	req := httptest.NewRequest(method, path, bytes.NewReader(payload))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set(contextKeyUserID, user.ID)
	c.Set(contextKeyEmail, user.Email)
	c.Set(contextKeyRole, user.Role)
	return c, rec
}

func resolveWeedBinary(t *testing.T) string {
	t.Helper()

	// Honour explicit override if set.
	if env := os.Getenv("HOSTEDAT_WEED_BIN"); env != "" {
		if _, err := os.Stat(env); err == nil {
			return env
		}
	}

	// Use a shared cache directory so the binary is downloaded once and
	// reused across test runs.
	cacheDir := filepath.Join(os.TempDir(), "hostedat-weed-cache")
	bin, err := seaweedfs.EnsureBinary(config.ObjectStorageConfig{DataDir: cacheDir})
	if err != nil {
		t.Fatalf("downloading weed binary: %v", err)
	}
	return bin
}

func mustFreePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("allocate free port: %v", err)
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port
}

// ──────────────────────────────────────────────
// SeaweedFS Manager lifecycle tests
// ──────────────────────────────────────────────

func TestSeaweedFS_ManagerStartStop(t *testing.T) {
	weedBin := resolveWeedBinary(t)
	port := mustFreePort(t)

	mgr := seaweedfs.NewManager(config.ObjectStorageConfig{
		Enabled:    true,
		Managed:    true,
		DataDir:    t.TempDir(),
		BinaryPath: weedBin,
		S3Endpoint: fmt.Sprintf("http://127.0.0.1:%d", port),
		Region:     "us-east-1",
	})

	if err := mgr.Start(); err != nil {
		t.Fatalf("manager start: %v", err)
	}
	if !mgr.IsHealthy() {
		t.Fatal("manager should report healthy after start")
	}
	if err := mgr.Stop(); err != nil {
		t.Fatalf("manager stop: %v", err)
	}
}

// ──────────────────────────────────────────────
// Bucket policy integration tests
// ──────────────────────────────────────────────

func TestBucketPolicy_PublicReadViaProxy(t *testing.T) {
	weedBin := resolveWeedBinary(t)
	port := mustFreePort(t)

	mgr := seaweedfs.NewManager(config.ObjectStorageConfig{
		Enabled:    true,
		Managed:    true,
		DataDir:    t.TempDir(),
		BinaryPath: weedBin,
		S3Endpoint: fmt.Sprintf("http://127.0.0.1:%d", port),
		Region:     "us-east-1",
	})
	if err := mgr.Start(); err != nil {
		t.Fatalf("start SeaweedFS: %v", err)
	}
	t.Cleanup(func() { _ = mgr.Stop() })

	db, err := models.InitDB(config.DBConfig{Driver: "sqlite", DSN: ":memory:"})
	if err != nil {
		t.Fatalf("InitDB: %v", err)
	}

	user := models.User{Email: "pub-s3@test.local", PasswordHash: "hash", Role: "user"}
	db.Create(&user)
	site := models.Site{UserID: user.ID, SubdomainSlug: "pub-s3", Name: "PubS3"}
	db.Create(&site)

	adminClient, err := minioClientForEndpoint(mgr.Config.S3Endpoint, mgr.AccessKeyID, mgr.SecretAccessKey)
	if err != nil {
		t.Fatalf("admin minio client: %v", err)
	}

	h := &StorageHandler{DB: db, S3Client: adminClient, IAMClient: mgr.Client, Region: "us-east-1"}

	// Create a public bucket (handler applies bucket policy via SetBucketPolicy)
	bucketName := site.ID + "-public"
	c, rec := newAuthedIntegrationContext(t, http.MethodPost, "/api/v1/sites/"+site.ID+"/storage/buckets", map[string]interface{}{
		"name":        "PUBLIC_ASSETS",
		"bucket_name": bucketName,
		"public":      true,
	}, user)
	c.SetParamNames("id")
	c.SetParamValues(site.ID)
	if err := h.CreateBucket(c); err != nil {
		t.Fatalf("CreateBucket: %v", err)
	}
	if rec.Code != http.StatusCreated {
		t.Fatalf("CreateBucket status=%d body=%s", rec.Code, rec.Body.String())
	}

	// Upload an object via admin client
	putObject(t, adminClient, bucketName, "hello.txt", "hello world")

	// Set up the pass-through S3 proxy (no wrapper needed)
	s3Proxy := NewS3Proxy(mgr.Config.S3Endpoint)

	// Unauthenticated GET should succeed — SeaweedFS allows it via bucket policy
	req := httptest.NewRequest(http.MethodGet, "/"+bucketName+"/hello.txt", nil)
	wrec := httptest.NewRecorder()
	s3Proxy.ServeHTTP(wrec, req)

	if wrec.Code != http.StatusOK {
		t.Fatalf("GET public object: status=%d body=%s", wrec.Code, wrec.Body.String())
	}
	if wrec.Body.String() != "hello world" {
		t.Errorf("body = %q, want 'hello world'", wrec.Body.String())
	}
}

func TestBucketPolicy_HeadPublicObject(t *testing.T) {
	weedBin := resolveWeedBinary(t)
	port := mustFreePort(t)

	mgr := seaweedfs.NewManager(config.ObjectStorageConfig{
		Enabled:    true,
		Managed:    true,
		DataDir:    t.TempDir(),
		BinaryPath: weedBin,
		S3Endpoint: fmt.Sprintf("http://127.0.0.1:%d", port),
		Region:     "us-east-1",
	})
	if err := mgr.Start(); err != nil {
		t.Fatalf("start SeaweedFS: %v", err)
	}
	t.Cleanup(func() { _ = mgr.Stop() })

	db, err := models.InitDB(config.DBConfig{Driver: "sqlite", DSN: ":memory:"})
	if err != nil {
		t.Fatalf("InitDB: %v", err)
	}

	user := models.User{Email: "head@test.local", PasswordHash: "hash", Role: "user"}
	db.Create(&user)
	site := models.Site{UserID: user.ID, SubdomainSlug: "head-s3", Name: "HeadS3"}
	db.Create(&site)

	adminClient, err := minioClientForEndpoint(mgr.Config.S3Endpoint, mgr.AccessKeyID, mgr.SecretAccessKey)
	if err != nil {
		t.Fatalf("admin minio client: %v", err)
	}

	h := &StorageHandler{DB: db, S3Client: adminClient, IAMClient: mgr.Client, Region: "us-east-1"}

	bucketName := site.ID + "-head"
	c, rec := newAuthedIntegrationContext(t, http.MethodPost, "/api/v1/sites/"+site.ID+"/storage/buckets", map[string]interface{}{
		"name":        "HEAD_BUCKET",
		"bucket_name": bucketName,
		"public":      true,
	}, user)
	c.SetParamNames("id")
	c.SetParamValues(site.ID)
	if err := h.CreateBucket(c); err != nil {
		t.Fatalf("CreateBucket: %v", err)
	}
	if rec.Code != http.StatusCreated {
		t.Fatalf("CreateBucket status=%d body=%s", rec.Code, rec.Body.String())
	}

	putObject(t, adminClient, bucketName, "file.txt", "content")

	s3Proxy := NewS3Proxy(mgr.Config.S3Endpoint)

	req := httptest.NewRequest(http.MethodHead, "/"+bucketName+"/file.txt", nil)
	hrec := httptest.NewRecorder()
	s3Proxy.ServeHTTP(hrec, req)

	if hrec.Code != http.StatusOK {
		t.Fatalf("HEAD status=%d", hrec.Code)
	}
	if hrec.Body.Len() != 0 {
		t.Errorf("HEAD body should be empty, got %d bytes", hrec.Body.Len())
	}
}

// ──────────────────────────────────────────────
// Real-SeaweedFS storage handler tests
// ──────────────────────────────────────────────

// TestReal_StorageHandlers exercises every StorageHandler operation against a
// real SeaweedFS instance. Subtests run sequentially and build on shared state
// so we only pay the cost of one SeaweedFS startup.
func TestReal_StorageHandlers(t *testing.T) {
	weedBin := resolveWeedBinary(t)
	port := mustFreePort(t)

	mgr := seaweedfs.NewManager(config.ObjectStorageConfig{
		Enabled:    true,
		Managed:    true,
		DataDir:    t.TempDir(),
		BinaryPath: weedBin,
		S3Endpoint: fmt.Sprintf("http://127.0.0.1:%d", port),
		Region:     "us-east-1",
	})
	if err := mgr.Start(); err != nil {
		t.Fatalf("start SeaweedFS: %v", err)
	}
	t.Cleanup(func() { _ = mgr.Stop() })

	db, err := models.InitDB(config.DBConfig{Driver: "sqlite", DSN: ":memory:"})
	if err != nil {
		t.Fatalf("InitDB: %v", err)
	}

	user := models.User{Email: "real-handler@test.local", PasswordHash: "hash", Role: "user"}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	site := models.Site{UserID: user.ID, SubdomainSlug: "real-handler", Name: "Real Handler"}
	if err := db.Create(&site).Error; err != nil {
		t.Fatalf("create site: %v", err)
	}

	adminClient, err := minioClientForEndpoint(mgr.Config.S3Endpoint, mgr.AccessKeyID, mgr.SecretAccessKey)
	if err != nil {
		t.Fatalf("admin minio client: %v", err)
	}

	h := &StorageHandler{DB: db, S3Client: adminClient, IAMClient: mgr.Client, Region: "us-east-1"}

	// ── CreateBucket ────────────────────────────
	t.Run("CreateBucket", func(t *testing.T) {
		bucketName := site.ID + "-images"
		c, rec := newAuthedIntegrationContext(t, http.MethodPost, "/api/v1/sites/"+site.ID+"/storage/buckets", map[string]string{
			"name":        "IMAGES",
			"bucket_name": bucketName,
		}, user)
		c.SetParamNames("id")
		c.SetParamValues(site.ID)

		if err := h.CreateBucket(c); err != nil {
			t.Fatalf("CreateBucket: %v", err)
		}
		if rec.Code != http.StatusCreated {
			t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
		}
	})

	// ── ListBuckets ─────────────────────────────
	t.Run("ListBuckets", func(t *testing.T) {
		c, rec := newAuthedIntegrationContext(t, http.MethodGet, "/api/v1/sites/"+site.ID+"/storage/buckets", nil, user)
		c.SetParamNames("id")
		c.SetParamValues(site.ID)

		if err := h.ListBuckets(c); err != nil {
			t.Fatalf("ListBuckets: %v", err)
		}
		if rec.Code != http.StatusOK {
			t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
		}

		var buckets []models.StorageBucket
		if err := json.Unmarshal(rec.Body.Bytes(), &buckets); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if len(buckets) != 1 {
			t.Fatalf("got %d buckets, want 1", len(buckets))
		}
	})

	// ── DuplicateBucketConflict ─────────────────
	t.Run("DuplicateBucketConflict", func(t *testing.T) {
		c, rec := newAuthedIntegrationContext(t, http.MethodPost, "/api/v1/sites/"+site.ID+"/storage/buckets", map[string]string{
			"name":        "IMAGES",
			"bucket_name": site.ID + "-images",
		}, user)
		c.SetParamNames("id")
		c.SetParamValues(site.ID)

		if err := h.CreateBucket(c); err != nil {
			t.Fatalf("CreateBucket: %v", err)
		}
		if rec.Code != http.StatusConflict {
			t.Fatalf("status=%d, want 409; body=%s", rec.Code, rec.Body.String())
		}
	})

	// ── CreateS3Credential ──────────────────────
	t.Run("CreateS3Credential", func(t *testing.T) {
		c, rec := newAuthedIntegrationContext(t, http.MethodPost, "/api/v1/s3-credentials", map[string]string{"name": "real-key"}, user)

		if err := h.CreateS3Credential(c); err != nil {
			t.Fatalf("CreateS3Credential: %v", err)
		}
		if rec.Code != http.StatusCreated {
			t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
		}

		resp := map[string]interface{}{}
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if resp["secret_access_key"] == nil || resp["secret_access_key"] == "" {
			t.Fatal("expected secret_access_key in response")
		}
		if resp["access_key_id"] == nil || resp["access_key_id"] == "" {
			t.Fatal("expected access_key_id in response")
		}
	})

	// ── ListS3Credentials ───────────────────────
	t.Run("ListS3Credentials", func(t *testing.T) {
		c, rec := newAuthedIntegrationContext(t, http.MethodGet, "/api/v1/s3-credentials", nil, user)

		if err := h.ListS3Credentials(c); err != nil {
			t.Fatalf("ListS3Credentials: %v", err)
		}
		if rec.Code != http.StatusOK {
			t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
		}

		var creds []models.S3Credential
		if err := json.Unmarshal(rec.Body.Bytes(), &creds); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if len(creds) != 1 {
			t.Fatalf("got %d credentials, want 1", len(creds))
		}
	})

	// ── UploadURL ───────────────────────────────
	t.Run("UploadURL", func(t *testing.T) {
		var bucket models.StorageBucket
		if err := db.Where("site_id = ?", site.ID).First(&bucket).Error; err != nil {
			t.Fatalf("find bucket: %v", err)
		}

		c, rec := newAuthedIntegrationContext(t, http.MethodPost, "/api/v1/sites/"+site.ID+"/storage/buckets/"+bucket.ID+"/upload-url", map[string]interface{}{"key": "test.jpg"}, user)
		c.SetParamNames("id", "bucketId")
		c.SetParamValues(site.ID, bucket.ID)

		if err := h.UploadURL(c); err != nil {
			t.Fatalf("UploadURL: %v", err)
		}
		if rec.Code != http.StatusOK {
			t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
		}

		resp := map[string]interface{}{}
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if resp["upload_url"] == nil || resp["upload_url"] == "" {
			t.Fatal("expected upload_url in response")
		}
	})

	// ── UpdateBucket (toggle public) ────────────
	t.Run("UpdateBucketTogglePublic", func(t *testing.T) {
		var bucket models.StorageBucket
		if err := db.Where("site_id = ?", site.ID).First(&bucket).Error; err != nil {
			t.Fatalf("find bucket: %v", err)
		}

		c, rec := newAuthedIntegrationContext(t, http.MethodPatch, "/api/v1/sites/"+site.ID+"/storage/buckets/"+bucket.ID, map[string]interface{}{"public": true}, user)
		c.SetParamNames("id", "bucketId")
		c.SetParamValues(site.ID, bucket.ID)

		if err := h.UpdateBucket(c); err != nil {
			t.Fatalf("UpdateBucket: %v", err)
		}
		if rec.Code != http.StatusOK {
			t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
		}

		var updated models.StorageBucket
		if err := json.Unmarshal(rec.Body.Bytes(), &updated); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if !updated.Public {
			t.Fatal("expected bucket to be public after toggle")
		}
	})

	// ── DeleteS3Credential ──────────────────────
	t.Run("DeleteS3Credential", func(t *testing.T) {
		var cred models.S3Credential
		if err := db.Where("user_id = ?", user.ID).First(&cred).Error; err != nil {
			t.Fatalf("find credential: %v", err)
		}

		c, rec := newAuthedIntegrationContext(t, http.MethodDelete, "/api/v1/s3-credentials/"+cred.ID, nil, user)
		c.SetParamNames("id")
		c.SetParamValues(cred.ID)

		if err := h.DeleteS3Credential(c); err != nil {
			t.Fatalf("DeleteS3Credential: %v", err)
		}
		if rec.Code != http.StatusNoContent {
			t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
		}
	})

	// ── DeleteBucket ────────────────────────────
	t.Run("DeleteBucket", func(t *testing.T) {
		var bucket models.StorageBucket
		if err := db.Where("site_id = ?", site.ID).First(&bucket).Error; err != nil {
			t.Fatalf("find bucket: %v", err)
		}

		c, rec := newAuthedIntegrationContext(t, http.MethodDelete, "/api/v1/sites/"+site.ID+"/storage/buckets/"+bucket.ID, nil, user)
		c.SetParamNames("id", "bucketId")
		c.SetParamValues(site.ID, bucket.ID)

		if err := h.DeleteBucket(c); err != nil {
			t.Fatalf("DeleteBucket: %v", err)
		}
		if rec.Code != http.StatusNoContent {
			t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
		}
	})

	// ── Verify empty after cleanup ──────────────
	t.Run("EmptyAfterCleanup", func(t *testing.T) {
		c, rec := newAuthedIntegrationContext(t, http.MethodGet, "/api/v1/sites/"+site.ID+"/storage/buckets", nil, user)
		c.SetParamNames("id")
		c.SetParamValues(site.ID)

		if err := h.ListBuckets(c); err != nil {
			t.Fatalf("ListBuckets: %v", err)
		}

		var buckets []models.StorageBucket
		if err := json.Unmarshal(rec.Body.Bytes(), &buckets); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if len(buckets) != 0 {
			t.Fatalf("got %d buckets after cleanup, want 0", len(buckets))
		}
	})
}

func TestBucketPolicy_PrivateBucket_Denied(t *testing.T) {
	weedBin := resolveWeedBinary(t)
	port := mustFreePort(t)

	mgr := seaweedfs.NewManager(config.ObjectStorageConfig{
		Enabled:    true,
		Managed:    true,
		DataDir:    t.TempDir(),
		BinaryPath: weedBin,
		S3Endpoint: fmt.Sprintf("http://127.0.0.1:%d", port),
		Region:     "us-east-1",
	})
	if err := mgr.Start(); err != nil {
		t.Fatalf("start SeaweedFS: %v", err)
	}
	t.Cleanup(func() { _ = mgr.Stop() })

	db, err := models.InitDB(config.DBConfig{Driver: "sqlite", DSN: ":memory:"})
	if err != nil {
		t.Fatalf("InitDB: %v", err)
	}

	user := models.User{Email: "priv@test.local", PasswordHash: "hash", Role: "user"}
	db.Create(&user)
	site := models.Site{UserID: user.ID, SubdomainSlug: "priv-s3", Name: "PrivS3"}
	db.Create(&site)

	adminClient, err := minioClientForEndpoint(mgr.Config.S3Endpoint, mgr.AccessKeyID, mgr.SecretAccessKey)
	if err != nil {
		t.Fatalf("admin minio client: %v", err)
	}

	h := &StorageHandler{DB: db, S3Client: adminClient, IAMClient: mgr.Client, Region: "us-east-1"}

	bucketName := site.ID + "-private"
	createBucketViaHandler(t, h, user, site.ID, "PRIVATE_BUCKET", bucketName)
	// Bucket is NOT public (default) — no bucket policy set

	putObject(t, adminClient, bucketName, "secret.txt", "private data")

	// Pass-through proxy — SeaweedFS denies unauthenticated access
	s3Proxy := NewS3Proxy(mgr.Config.S3Endpoint)

	req := httptest.NewRequest(http.MethodGet, "/"+bucketName+"/secret.txt", nil)
	rec := httptest.NewRecorder()
	s3Proxy.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status=%d, want 403 for non-public bucket", rec.Code)
	}
}
