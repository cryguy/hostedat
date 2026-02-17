package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sort"
	"testing"

	"github.com/cryguy/hostedat/internal/config"
	"github.com/cryguy/hostedat/internal/models"
	"github.com/cryguy/hostedat/internal/seaweedfs"
	"github.com/labstack/echo/v4"
	minio "github.com/minio/minio-go/v7"
	"gorm.io/gorm"
)

type fakeBucketClient struct {
	makeErr   error
	removeErr error
	made      []string
	removed   []string
}

func (f *fakeBucketClient) MakeBucket(_ context.Context, bucketName string, _ minio.MakeBucketOptions) error {
	f.made = append(f.made, bucketName)
	return f.makeErr
}

func (f *fakeBucketClient) RemoveBucket(_ context.Context, bucketName string) error {
	f.removed = append(f.removed, bucketName)
	return f.removeErr
}

type fakeIAMClient struct {
	createUserErr      error
	createAccessKeyErr error
	putPolicyErr       error
	deleteAccessKeyErr error
	deleteUserErr      error

	accessKeyResult *seaweedfs.AccessKeyResult

	createdUsers []string
	deletedUsers []string
	deletedKeys  []string
	policyUsers  []string
}

func (f *fakeIAMClient) CreateUser(userName string) error {
	f.createdUsers = append(f.createdUsers, userName)
	return f.createUserErr
}

func (f *fakeIAMClient) DeleteUser(userName string) error {
	f.deletedUsers = append(f.deletedUsers, userName)
	return f.deleteUserErr
}

func (f *fakeIAMClient) CreateAccessKey(_ string) (*seaweedfs.AccessKeyResult, error) {
	if f.createAccessKeyErr != nil {
		return nil, f.createAccessKeyErr
	}
	if f.accessKeyResult == nil {
		f.accessKeyResult = &seaweedfs.AccessKeyResult{AccessKeyID: "AKIA_TEST", SecretAccessKey: "secret"}
	}
	return f.accessKeyResult, nil
}

func (f *fakeIAMClient) DeleteAccessKey(accessKeyID string) error {
	f.deletedKeys = append(f.deletedKeys, accessKeyID)
	return f.deleteAccessKeyErr
}

func (f *fakeIAMClient) PutUserPolicy(userName, _, _ string) error {
	f.policyUsers = append(f.policyUsers, userName)
	return f.putPolicyErr
}

func setupStorageHandlerTest(t *testing.T) (*StorageHandler, *gorm.DB, models.User, models.Site, *fakeBucketClient, *fakeIAMClient) {
	t.Helper()
	db, err := models.InitDB(config.DBConfig{Driver: "sqlite", DSN: ":memory:"})
	if err != nil {
		t.Fatalf("InitDB: %v", err)
	}

	user := models.User{Email: "storage@test.local", PasswordHash: "hash", Role: "user"}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}

	site := models.Site{UserID: user.ID, SubdomainSlug: "storage-site", Name: "Storage Site"}
	if err := db.Create(&site).Error; err != nil {
		t.Fatalf("create site: %v", err)
	}

	s3 := &fakeBucketClient{}
	iam := &fakeIAMClient{}
	h := &StorageHandler{DB: db, S3Client: s3, IAMClient: iam, Region: "us-east-1"}
	return h, db, user, site, s3, iam
}

func newAuthedContext(t *testing.T, method, path string, body interface{}, user models.User) (echo.Context, *httptest.ResponseRecorder) {
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

func TestCreateS3Credential_PolicyAttachFailure_RollsBack(t *testing.T) {
	h, db, user, site, _, iam := setupStorageHandlerTest(t)
	iam.putPolicyErr = assertErr("policy failure")

	if err := db.Create(&models.StorageBucket{SiteID: site.ID, Name: "IMAGES", BucketName: site.ID + "-images"}).Error; err != nil {
		t.Fatalf("seed bucket: %v", err)
	}

	c, rec := newAuthedContext(t, http.MethodPost, "/api/v1/s3-credentials", map[string]string{"name": "ci-key"}, user)
	if err := h.CreateS3Credential(c); err != nil {
		t.Fatalf("CreateS3Credential returned error: %v", err)
	}

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500; body=%s", rec.Code, rec.Body.String())
	}

	var count int64
	db.Model(&models.S3Credential{}).Where("user_id = ?", user.ID).Count(&count)
	if count != 0 {
		t.Fatalf("credentials persisted despite policy failure: %d", count)
	}
	if len(iam.deletedKeys) != 1 {
		t.Fatalf("expected cleanup to delete access key once, got %d", len(iam.deletedKeys))
	}
	if len(iam.deletedUsers) != 1 {
		t.Fatalf("expected cleanup to delete IAM user once, got %d", len(iam.deletedUsers))
	}
}

func TestCreateS3Credential_Success_PersistsAndReturnsSecret(t *testing.T) {
	h, db, user, site, _, _ := setupStorageHandlerTest(t)
	if err := db.Create(&models.StorageBucket{SiteID: site.ID, Name: "IMAGES", BucketName: site.ID + "-images"}).Error; err != nil {
		t.Fatalf("seed bucket: %v", err)
	}

	c, rec := newAuthedContext(t, http.MethodPost, "/api/v1/s3-credentials", map[string]string{"name": "ci-key"}, user)
	if err := h.CreateS3Credential(c); err != nil {
		t.Fatalf("CreateS3Credential returned error: %v", err)
	}

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body=%s", rec.Code, rec.Body.String())
	}

	resp := map[string]interface{}{}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if resp["secret_access_key"] == "" {
		t.Fatal("expected secret_access_key in response")
	}

	var count int64
	db.Model(&models.S3Credential{}).Where("user_id = ?", user.ID).Count(&count)
	if count != 1 {
		t.Fatalf("credentials persisted = %d, want 1", count)
	}
}

func TestCreateBucket_ReconcilesExistingCredentials(t *testing.T) {
	h, db, user, site, _, iam := setupStorageHandlerTest(t)

	if err := db.Create(&models.S3Credential{UserID: user.ID, ExternalKeyID: "u-cred-1", AccessKeyID: "AK1", Name: "first"}).Error; err != nil {
		t.Fatalf("seed credential 1: %v", err)
	}
	if err := db.Create(&models.S3Credential{UserID: user.ID, ExternalKeyID: "u-cred-2", AccessKeyID: "AK2", Name: "second"}).Error; err != nil {
		t.Fatalf("seed credential 2: %v", err)
	}

	bucketName := site.ID + "-images"
	c, rec := newAuthedContext(t, http.MethodPost, "/api/v1/sites/"+site.ID+"/storage/buckets", map[string]string{
		"name":        "IMAGES",
		"bucket_name": bucketName,
	}, user)
	c.SetParamNames("id")
	c.SetParamValues(site.ID)

	if err := h.CreateBucket(c); err != nil {
		t.Fatalf("CreateBucket returned error: %v", err)
	}
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body=%s", rec.Code, rec.Body.String())
	}

	sort.Strings(iam.policyUsers)
	want := []string{"u-cred-1", "u-cred-2"}
	if len(iam.policyUsers) != len(want) || iam.policyUsers[0] != want[0] || iam.policyUsers[1] != want[1] {
		t.Fatalf("policy users = %v, want %v", iam.policyUsers, want)
	}
}

func TestDeleteBucket_ReconcilesExistingCredentials(t *testing.T) {
	h, db, user, site, _, iam := setupStorageHandlerTest(t)

	bucket := models.StorageBucket{SiteID: site.ID, Name: "IMAGES", BucketName: site.ID + "-images"}
	if err := db.Create(&bucket).Error; err != nil {
		t.Fatalf("seed bucket: %v", err)
	}
	if err := db.Create(&models.S3Credential{UserID: user.ID, ExternalKeyID: "u-cred-1", AccessKeyID: "AK1", Name: "first"}).Error; err != nil {
		t.Fatalf("seed credential 1: %v", err)
	}
	if err := db.Create(&models.S3Credential{UserID: user.ID, ExternalKeyID: "u-cred-2", AccessKeyID: "AK2", Name: "second"}).Error; err != nil {
		t.Fatalf("seed credential 2: %v", err)
	}

	c, rec := newAuthedContext(t, http.MethodDelete, "/api/v1/sites/"+site.ID+"/storage/buckets/"+bucket.ID, nil, user)
	c.SetParamNames("id", "bucketId")
	c.SetParamValues(site.ID, bucket.ID)

	if err := h.DeleteBucket(c); err != nil {
		t.Fatalf("DeleteBucket returned error: %v", err)
	}
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204; body=%s", rec.Code, rec.Body.String())
	}

	sort.Strings(iam.policyUsers)
	want := []string{"u-cred-1", "u-cred-2"}
	if len(iam.policyUsers) != len(want) || iam.policyUsers[0] != want[0] || iam.policyUsers[1] != want[1] {
		t.Fatalf("policy users = %v, want %v", iam.policyUsers, want)
	}
}

func TestCreateBucket_DuplicateBindingNameSameSite_Conflict(t *testing.T) {
	h, db, user, site, _, _ := setupStorageHandlerTest(t)
	if err := db.Create(&models.StorageBucket{SiteID: site.ID, Name: "IMAGES", BucketName: site.ID + "-images"}).Error; err != nil {
		t.Fatalf("seed bucket: %v", err)
	}

	c, rec := newAuthedContext(t, http.MethodPost, "/api/v1/sites/"+site.ID+"/storage/buckets", map[string]string{
		"name":        "IMAGES",
		"bucket_name": site.ID + "-assets",
	}, user)
	c.SetParamNames("id")
	c.SetParamValues(site.ID)

	if err := h.CreateBucket(c); err != nil {
		t.Fatalf("CreateBucket returned error: %v", err)
	}
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409; body=%s", rec.Code, rec.Body.String())
	}
}

func TestCreateBucket_DuplicateBindingNameDifferentSite_Allowed(t *testing.T) {
	h, db, user, site, _, _ := setupStorageHandlerTest(t)

	otherUser := models.User{Email: "other@test.local", PasswordHash: "hash", Role: "user"}
	if err := db.Create(&otherUser).Error; err != nil {
		t.Fatalf("create other user: %v", err)
	}
	otherSite := models.Site{UserID: otherUser.ID, SubdomainSlug: "other-site", Name: "Other Site"}
	if err := db.Create(&otherSite).Error; err != nil {
		t.Fatalf("create other site: %v", err)
	}
	if err := db.Create(&models.StorageBucket{SiteID: otherSite.ID, Name: "IMAGES", BucketName: otherSite.ID + "-images"}).Error; err != nil {
		t.Fatalf("seed other-site bucket: %v", err)
	}

	c, rec := newAuthedContext(t, http.MethodPost, "/api/v1/sites/"+site.ID+"/storage/buckets", map[string]string{
		"name":        "IMAGES",
		"bucket_name": site.ID + "-images",
	}, user)
	c.SetParamNames("id")
	c.SetParamValues(site.ID)

	if err := h.CreateBucket(c); err != nil {
		t.Fatalf("CreateBucket returned error: %v", err)
	}
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body=%s", rec.Code, rec.Body.String())
	}
}

func TestCreateS3Credential_InvalidCredentialName_BadRequest(t *testing.T) {
	h, _, user, _, _, _ := setupStorageHandlerTest(t)
	c, rec := newAuthedContext(t, http.MethodPost, "/api/v1/s3-credentials", map[string]string{"name": "bad name"}, user)

	if err := h.CreateS3Credential(c); err != nil {
		t.Fatalf("CreateS3Credential returned error: %v", err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", rec.Code, rec.Body.String())
	}
}

func TestCreateBucket_InvalidBucketName_BadRequest(t *testing.T) {
	h, _, user, site, _, _ := setupStorageHandlerTest(t)
	c, rec := newAuthedContext(t, http.MethodPost, "/api/v1/sites/"+site.ID+"/storage/buckets", map[string]string{
		"name":        "IMAGES",
		"bucket_name": "INVALID-BUCKET",
	}, user)
	c.SetParamNames("id")
	c.SetParamValues(site.ID)

	if err := h.CreateBucket(c); err != nil {
		t.Fatalf("CreateBucket returned error: %v", err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", rec.Code, rec.Body.String())
	}
}

func TestCreateBucket_InvalidBindingName_BadRequest(t *testing.T) {
	h, _, user, site, _, _ := setupStorageHandlerTest(t)
	c, rec := newAuthedContext(t, http.MethodPost, "/api/v1/sites/"+site.ID+"/storage/buckets", map[string]string{
		"name":        "__proto__",
		"bucket_name": site.ID + "-images",
	}, user)
	c.SetParamNames("id")
	c.SetParamValues(site.ID)

	if err := h.CreateBucket(c); err != nil {
		t.Fatalf("CreateBucket returned error: %v", err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", rec.Code, rec.Body.String())
	}
}

func TestStorageHandlers_Authorization(t *testing.T) {
	h, db, _, site, _, _ := setupStorageHandlerTest(t)

	otherUser := models.User{Email: "unauthorized@test.local", PasswordHash: "hash", Role: "user"}
	if err := db.Create(&otherUser).Error; err != nil {
		t.Fatalf("create other user: %v", err)
	}

	c, rec := newAuthedContext(t, http.MethodPost, "/api/v1/sites/"+site.ID+"/storage/buckets", map[string]string{
		"name":        "IMAGES",
		"bucket_name": site.ID + "-images",
	}, otherUser)
	c.SetParamNames("id")
	c.SetParamValues(site.ID)

	if err := h.CreateBucket(c); err != nil {
		t.Fatalf("CreateBucket returned error: %v", err)
	}
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403; body=%s", rec.Code, rec.Body.String())
	}
}

func TestDeleteS3Credential_RevokesAccess(t *testing.T) {
	h, db, user, _, _, iam := setupStorageHandlerTest(t)
	cred := models.S3Credential{UserID: user.ID, ExternalKeyID: "hd-user-test", AccessKeyID: "AKIA_DELETE", Name: "to-delete"}
	if err := db.Create(&cred).Error; err != nil {
		t.Fatalf("create credential: %v", err)
	}

	c, rec := newAuthedContext(t, http.MethodDelete, "/api/v1/s3-credentials/"+cred.ID, nil, user)
	c.SetParamNames("id")
	c.SetParamValues(cred.ID)

	if err := h.DeleteS3Credential(c); err != nil {
		t.Fatalf("DeleteS3Credential returned error: %v", err)
	}
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204; body=%s", rec.Code, rec.Body.String())
	}
	if len(iam.deletedKeys) != 1 || iam.deletedKeys[0] != cred.AccessKeyID {
		t.Fatalf("deleted keys = %v, want [%s]", iam.deletedKeys, cred.AccessKeyID)
	}
	if len(iam.deletedUsers) != 1 || iam.deletedUsers[0] != cred.ExternalKeyID {
		t.Fatalf("deleted users = %v, want [%s]", iam.deletedUsers, cred.ExternalKeyID)
	}
}

type testError string

func (e testError) Error() string { return string(e) }

func assertErr(msg string) error {
	return testError(msg)
}
