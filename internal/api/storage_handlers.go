package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/cryguy/hostedat/internal/models"
	"github.com/cryguy/hostedat/internal/seaweedfs"
	"github.com/labstack/echo/v4"
	"github.com/minio/minio-go/v7"
	"gorm.io/gorm"
)

var (
	bucketNamePattern     = regexp.MustCompile(`^[a-z0-9][a-z0-9.-]{1,61}[a-z0-9]$`)
	bindingNamePattern    = regexp.MustCompile(`^[A-Z][A-Z0-9_]{0,63}$`)
	credentialNamePattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]{0,31}$`)
	reservedBindingNames  = map[string]struct{}{
		"ASSETS":      {},
		"__PROTO__":   {},
		"PROTOTYPE":   {},
		"CONSTRUCTOR": {},
	}
)

type bucketClient interface {
	MakeBucket(ctx context.Context, bucketName string, opts minio.MakeBucketOptions) error
	RemoveBucket(ctx context.Context, bucketName string) error
	PresignedPutObject(ctx context.Context, bucketName, objectName string, expires time.Duration) (*url.URL, error)
	GetObject(ctx context.Context, bucketName, objectName string, opts minio.GetObjectOptions) (*minio.Object, error)
}

type iamClient interface {
	CreateUser(userName string) error
	DeleteUser(userName string) error
	CreateAccessKey(userName string) (*seaweedfs.AccessKeyResult, error)
	DeleteAccessKey(accessKeyID string) error
	PutUserPolicy(userName, policyName, policyJSON string) error
}

// StorageHandler manages object storage buckets and S3 credentials.
type StorageHandler struct {
	DB            *gorm.DB
	S3Client      bucketClient
	PresignClient bucketClient // minio client configured with public S3 endpoint for presigned URL generation
	IAMClient     iamClient
	Region        string
	PublicS3URL   string // public-facing S3 URL for presigned URLs (e.g. https://storage.example.com)
}

// CreateBucket creates a storage bucket bound to a site.
func (h *StorageHandler) CreateBucket(c echo.Context) error {
	userID, _, role := GetUserFromContext(c)
	siteID := c.Param("id")
	ctx := c.Request().Context()

	var site models.Site
	if err := h.DB.First(&site, "id = ?", siteID).Error; err != nil {
		return errorJSON(c, http.StatusNotFound, "site not found")
	}
	if site.UserID != userID && role != "admin" && role != "superadmin" {
		return errorJSON(c, http.StatusForbidden, "access denied")
	}

	var req struct {
		Name       string `json:"name"`        // binding name (e.g. "IMAGES")
		BucketName string `json:"bucket_name"` // S3 bucket name
		Public     bool   `json:"public"`      // allow unauthenticated read access
	}
	if err := c.Bind(&req); err != nil {
		return errorJSON(c, http.StatusBadRequest, "invalid request")
	}
	if req.Name == "" || req.BucketName == "" {
		return errorJSON(c, http.StatusBadRequest, "name and bucket_name are required")
	}
	if err := validateBindingName(req.Name); err != nil {
		return errorJSON(c, http.StatusBadRequest, err.Error())
	}
	if err := validateBucketName(site.ID, req.BucketName); err != nil {
		return errorJSON(c, http.StatusBadRequest, err.Error())
	}

	// Check uniqueness of bucket_name and per-site binding names in our DB.
	var existing models.StorageBucket
	if err := h.DB.Where("bucket_name = ?", req.BucketName).First(&existing).Error; err == nil {
		return errorJSON(c, http.StatusConflict, "bucket name already taken")
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return errorJSON(c, http.StatusInternalServerError, "failed to validate bucket name")
	}

	if err := h.DB.Where("site_id = ? AND name = ?", site.ID, req.Name).First(&existing).Error; err == nil {
		return errorJSON(c, http.StatusConflict, "binding name already exists for site")
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return errorJSON(c, http.StatusInternalServerError, "failed to validate binding name")
	}

	// Create bucket via S3 API.
	if err := h.S3Client.MakeBucket(ctx, req.BucketName, minio.MakeBucketOptions{Region: h.Region}); err != nil {
		return errorJSON(c, http.StatusInternalServerError, "failed to create bucket: "+err.Error())
	}

	bucket := models.StorageBucket{
		SiteID:     siteID,
		Name:       req.Name,
		BucketName: req.BucketName,
		Public:     req.Public,
	}
	if err := h.DB.Create(&bucket).Error; err != nil {
		_ = h.S3Client.RemoveBucket(ctx, req.BucketName)
		if isUniqueConstraintError(err) {
			return errorJSON(c, http.StatusConflict, "bucket or binding name already exists")
		}
		return errorJSON(c, http.StatusInternalServerError, "failed to save bucket")
	}

	if err := h.reconcileUserCredentialPolicies(site.UserID); err != nil {
		_ = h.DB.Delete(&bucket).Error
		_ = h.S3Client.RemoveBucket(ctx, req.BucketName)
		return errorJSON(c, http.StatusInternalServerError, "failed to reconcile credential policy: "+err.Error())
	}

	return c.JSON(http.StatusCreated, bucket)
}

// ListBuckets returns all storage buckets for a site.
func (h *StorageHandler) ListBuckets(c echo.Context) error {
	userID, _, role := GetUserFromContext(c)
	siteID := c.Param("id")

	var site models.Site
	if err := h.DB.First(&site, "id = ?", siteID).Error; err != nil {
		return errorJSON(c, http.StatusNotFound, "site not found")
	}
	if site.UserID != userID && role != "admin" && role != "superadmin" {
		return errorJSON(c, http.StatusForbidden, "access denied")
	}

	var buckets []models.StorageBucket
	if err := h.DB.Where("site_id = ?", siteID).Find(&buckets).Error; err != nil {
		return errorJSON(c, http.StatusInternalServerError, "failed to list buckets")
	}

	return c.JSON(http.StatusOK, buckets)
}

// DeleteBucket deletes a storage bucket.
func (h *StorageHandler) DeleteBucket(c echo.Context) error {
	userID, _, role := GetUserFromContext(c)
	siteID := c.Param("id")
	bucketID := c.Param("bucketId")
	ctx := c.Request().Context()

	var site models.Site
	if err := h.DB.First(&site, "id = ?", siteID).Error; err != nil {
		return errorJSON(c, http.StatusNotFound, "site not found")
	}
	if site.UserID != userID && role != "admin" && role != "superadmin" {
		return errorJSON(c, http.StatusForbidden, "access denied")
	}

	var bucket models.StorageBucket
	if err := h.DB.First(&bucket, "id = ? AND site_id = ?", bucketID, siteID).Error; err != nil {
		return errorJSON(c, http.StatusNotFound, "bucket not found")
	}

	// Delete via S3 API.
	if err := h.S3Client.RemoveBucket(ctx, bucket.BucketName); err != nil {
		return errorJSON(c, http.StatusInternalServerError, "failed to delete bucket: "+err.Error())
	}

	if err := h.DB.Delete(&bucket).Error; err != nil {
		return errorJSON(c, http.StatusInternalServerError, "failed to delete bucket")
	}

	if err := h.reconcileUserCredentialPolicies(site.UserID); err != nil {
		return errorJSON(c, http.StatusInternalServerError, "failed to reconcile credential policy: "+err.Error())
	}

	return c.NoContent(http.StatusNoContent)
}

// UpdateBucket updates a storage bucket's settings (e.g. public access toggle).
func (h *StorageHandler) UpdateBucket(c echo.Context) error {
	userID, _, role := GetUserFromContext(c)
	siteID := c.Param("id")
	bucketID := c.Param("bucketId")

	var site models.Site
	if err := h.DB.First(&site, "id = ?", siteID).Error; err != nil {
		return errorJSON(c, http.StatusNotFound, "site not found")
	}
	if site.UserID != userID && role != "admin" && role != "superadmin" {
		return errorJSON(c, http.StatusForbidden, "access denied")
	}

	var bucket models.StorageBucket
	if err := h.DB.First(&bucket, "id = ? AND site_id = ?", bucketID, siteID).Error; err != nil {
		return errorJSON(c, http.StatusNotFound, "bucket not found")
	}

	var req struct {
		Public *bool `json:"public"`
	}
	if err := c.Bind(&req); err != nil {
		return errorJSON(c, http.StatusBadRequest, "invalid request")
	}

	if req.Public != nil {
		bucket.Public = *req.Public
	}

	if err := h.DB.Save(&bucket).Error; err != nil {
		return errorJSON(c, http.StatusInternalServerError, "failed to update bucket")
	}

	return c.JSON(http.StatusOK, bucket)
}

// UploadURL generates a presigned PUT URL for uploading an object to a bucket.
func (h *StorageHandler) UploadURL(c echo.Context) error {
	userID, _, role := GetUserFromContext(c)
	siteID := c.Param("id")
	bucketID := c.Param("bucketId")
	ctx := c.Request().Context()

	var site models.Site
	if err := h.DB.First(&site, "id = ?", siteID).Error; err != nil {
		return errorJSON(c, http.StatusNotFound, "site not found")
	}
	if site.UserID != userID && role != "admin" && role != "superadmin" {
		return errorJSON(c, http.StatusForbidden, "access denied")
	}

	var bucket models.StorageBucket
	if err := h.DB.First(&bucket, "id = ? AND site_id = ?", bucketID, siteID).Error; err != nil {
		return errorJSON(c, http.StatusNotFound, "bucket not found")
	}

	var req struct {
		Key       string `json:"key"`
		ExpiresIn int    `json:"expires_in"` // seconds, default 3600
	}
	if err := c.Bind(&req); err != nil {
		return errorJSON(c, http.StatusBadRequest, "invalid request")
	}
	if req.Key == "" {
		return errorJSON(c, http.StatusBadRequest, "key is required")
	}
	if req.ExpiresIn <= 0 {
		req.ExpiresIn = 3600
	}
	if req.ExpiresIn > 604800 {
		req.ExpiresIn = 604800
	}

	// Use the presign client (configured with the public endpoint) so the
	// SigV4 signature matches the Host header the client will send.
	presignClient := h.PresignClient
	if presignClient == nil {
		presignClient = h.S3Client
	}
	presigned, err := presignClient.PresignedPutObject(ctx, bucket.BucketName, req.Key, time.Duration(req.ExpiresIn)*time.Second)
	if err != nil {
		return errorJSON(c, http.StatusInternalServerError, "failed to generate upload URL: "+err.Error())
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"upload_url": presigned.String(),
		"key":        req.Key,
		"bucket":     bucket.BucketName,
		"expires_in": req.ExpiresIn,
	})
}

// CreateS3Credential creates an S3 credential for the current user.
func (h *StorageHandler) CreateS3Credential(c echo.Context) error {
	userID, _, _ := GetUserFromContext(c)

	var req struct {
		Name string `json:"name"`
	}
	if err := c.Bind(&req); err != nil {
		return errorJSON(c, http.StatusBadRequest, "invalid request")
	}
	if req.Name == "" {
		return errorJSON(c, http.StatusBadRequest, "name is required")
	}
	if err := validateCredentialName(req.Name); err != nil {
		return errorJSON(c, http.StatusBadRequest, err.Error())
	}

	// IAM username: prefix with user ID for uniqueness.
	userName := fmt.Sprintf("hd-%s-%s", userID, req.Name)

	// Create IAM user.
	if err := h.IAMClient.CreateUser(userName); err != nil {
		return errorJSON(c, http.StatusInternalServerError, "failed to create IAM user: "+err.Error())
	}

	// Create access key for the user.
	keyResult, err := h.IAMClient.CreateAccessKey(userName)
	if err != nil {
		_ = h.IAMClient.DeleteUser(userName)
		return errorJSON(c, http.StatusInternalServerError, "failed to create access key: "+err.Error())
	}

	policyJSON, err := h.buildPolicyDocumentForUser(userID)
	if err != nil {
		h.cleanupCredentialCreation(userName, keyResult.AccessKeyID)
		return errorJSON(c, http.StatusInternalServerError, "failed to build IAM policy")
	}
	if err := h.IAMClient.PutUserPolicy(userName, "bucket-access", policyJSON); err != nil {
		h.cleanupCredentialCreation(userName, keyResult.AccessKeyID)
		return errorJSON(c, http.StatusInternalServerError, "failed to attach IAM policy: "+err.Error())
	}

	cred := models.S3Credential{
		UserID:        userID,
		ExternalKeyID: userName,
		AccessKeyID:   keyResult.AccessKeyID,
		Name:          req.Name,
	}
	if err := h.DB.Create(&cred).Error; err != nil {
		h.cleanupCredentialCreation(userName, keyResult.AccessKeyID)
		return errorJSON(c, http.StatusInternalServerError, "failed to save credential")
	}

	// Return with secret (one-time only).
	return c.JSON(http.StatusCreated, map[string]interface{}{
		"id":                cred.ID,
		"access_key_id":     keyResult.AccessKeyID,
		"secret_access_key": keyResult.SecretAccessKey,
		"name":              cred.Name,
		"created_at":        cred.CreatedAt,
	})
}

// ListS3Credentials returns the user's S3 credentials (no secrets).
func (h *StorageHandler) ListS3Credentials(c echo.Context) error {
	userID, _, _ := GetUserFromContext(c)

	var creds []models.S3Credential
	if err := h.DB.Where("user_id = ?", userID).Find(&creds).Error; err != nil {
		return errorJSON(c, http.StatusInternalServerError, "failed to list credentials")
	}

	return c.JSON(http.StatusOK, creds)
}

// DeleteS3Credential deletes an S3 credential.
func (h *StorageHandler) DeleteS3Credential(c echo.Context) error {
	userID, _, _ := GetUserFromContext(c)
	credID := c.Param("id")

	var cred models.S3Credential
	if err := h.DB.First(&cred, "id = ? AND user_id = ?", credID, userID).Error; err != nil {
		return errorJSON(c, http.StatusNotFound, "credential not found")
	}

	if err := h.IAMClient.DeleteAccessKey(cred.AccessKeyID); err != nil {
		return errorJSON(c, http.StatusInternalServerError, "failed to delete IAM access key: "+err.Error())
	}

	// Delete IAM user (also removes its access keys).
	if err := h.IAMClient.DeleteUser(cred.ExternalKeyID); err != nil {
		return errorJSON(c, http.StatusInternalServerError, "failed to delete IAM user: "+err.Error())
	}

	if err := h.DB.Delete(&cred).Error; err != nil {
		return errorJSON(c, http.StatusInternalServerError, "failed to delete credential")
	}

	return c.NoContent(http.StatusNoContent)
}

func (h *StorageHandler) cleanupCredentialCreation(userName, accessKeyID string) {
	if accessKeyID != "" {
		_ = h.IAMClient.DeleteAccessKey(accessKeyID)
	}
	_ = h.IAMClient.DeleteUser(userName)
}

func (h *StorageHandler) buildPolicyDocumentForUser(userID string) (string, error) {
	var buckets []models.StorageBucket
	if err := h.DB.Joins("JOIN sites ON sites.id = storage_buckets.site_id").
		Where("sites.user_id = ?", userID).
		Find(&buckets).Error; err != nil {
		return "", err
	}

	resources := make([]string, 0, len(buckets)*2)
	for _, b := range buckets {
		resources = append(resources, fmt.Sprintf("arn:aws:s3:::%s", b.BucketName))
		resources = append(resources, fmt.Sprintf("arn:aws:s3:::%s/*", b.BucketName))
	}

	var statements []map[string]interface{}
	if len(resources) > 0 {
		statements = append(statements, map[string]interface{}{
			"Effect":   "Allow",
			"Action":   "s3:*",
			"Resource": resources,
		})
	}

	policy := map[string]interface{}{
		"Version":   "2012-10-17",
		"Statement": statements,
	}

	policyJSON, err := json.Marshal(policy)
	if err != nil {
		return "", err
	}

	return string(policyJSON), nil
}

func (h *StorageHandler) reconcileUserCredentialPolicies(userID string) error {
	var creds []models.S3Credential
	if err := h.DB.Where("user_id = ?", userID).Find(&creds).Error; err != nil {
		return err
	}
	if len(creds) == 0 {
		return nil
	}

	policyJSON, err := h.buildPolicyDocumentForUser(userID)
	if err != nil {
		return err
	}

	for _, cred := range creds {
		if err := h.IAMClient.PutUserPolicy(cred.ExternalKeyID, "bucket-access", policyJSON); err != nil {
			return fmt.Errorf("updating policy for credential %s: %w", cred.ID, err)
		}
	}

	return nil
}

func validateBucketName(siteID, bucketName string) error {
	if len(bucketName) < 3 || len(bucketName) > 63 {
		return fmt.Errorf("bucket_name must be between 3 and 63 characters")
	}
	if !bucketNamePattern.MatchString(bucketName) {
		return fmt.Errorf("bucket_name must contain only lowercase letters, digits, dots, and hyphens")
	}
	if strings.Contains(bucketName, "..") {
		return fmt.Errorf("bucket_name must not contain consecutive dots")
	}
	if strings.Contains(bucketName, ".-") || strings.Contains(bucketName, "-.") {
		return fmt.Errorf("bucket_name must not contain mixed dot-hyphen labels")
	}
	if ip := net.ParseIP(bucketName); ip != nil {
		return fmt.Errorf("bucket_name must not be an IP address")
	}
	if !strings.HasPrefix(bucketName, strings.ToLower(siteID)+"-") {
		return fmt.Errorf("bucket_name must start with %q", strings.ToLower(siteID)+"-")
	}
	return nil
}

func validateBindingName(name string) error {
	if !bindingNamePattern.MatchString(name) {
		return fmt.Errorf("name must match ^[A-Z][A-Z0-9_]{0,63}$")
	}
	if _, reserved := reservedBindingNames[strings.ToUpper(name)]; reserved {
		return fmt.Errorf("name is reserved")
	}
	return nil
}

func validateCredentialName(name string) error {
	if !credentialNamePattern.MatchString(name) {
		return fmt.Errorf("name must be 1-32 characters of letters, digits, underscore, or hyphen")
	}
	return nil
}

func isUniqueConstraintError(err error) bool {
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return true
	}
	errMsg := strings.ToLower(err.Error())
	return strings.Contains(errMsg, "unique constraint") || strings.Contains(errMsg, "duplicate key")
}
