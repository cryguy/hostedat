//go:build integration

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
	"os/exec"
	"path/filepath"
	"runtime"
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

	adminClient, err := minioClientForEndpoint(mgr.Config.S3Endpoint, "", "")
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

	if _, err := scopedClient.PutObject(context.Background(), bucketA, "after-delete.txt", strings.NewReader("x"), 1, minio.PutObjectOptions{}); err == nil {
		t.Fatal("expected credential to be revoked after DeleteS3Credential")
	}

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
	creds := credentials.NewAnonymousCredentials()
	if accessKey != "" && secretKey != "" {
		creds = credentials.NewStaticV4(accessKey, secretKey, "")
	}
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

	if env := strings.TrimSpace(os.Getenv("HOSTEDAT_WEED_BIN")); env != "" {
		if _, err := os.Stat(env); err == nil {
			return env
		}
		if _, err := exec.LookPath(env); err == nil {
			return env
		}
		t.Skipf("integration requires HOSTEDAT_WEED_BIN; not found: %s", env)
	}

	if runtime.GOOS == "windows" {
		candidates := []string{"./weed.exe", filepath.Join("..", "..", "weed.exe")}
		for _, p := range candidates {
			if _, err := os.Stat(p); err == nil {
				return p
			}
		}
		t.Skip("integration requires weed.exe (set HOSTEDAT_WEED_BIN)")
	}

	if _, err := exec.LookPath("weed"); err == nil {
		return "weed"
	}
	t.Skip("integration requires 'weed' in PATH (or set HOSTEDAT_WEED_BIN)")
	return ""
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
