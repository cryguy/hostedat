package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/cryguy/hostedat/internal/models"
	"github.com/cryguy/hostedat/internal/seaweedfs"
	"github.com/labstack/echo/v4"
	minio "github.com/minio/minio-go/v7"
	"gorm.io/gorm"
)

// StorageHandler manages object storage buckets and S3 credentials.
type StorageHandler struct {
	DB        *gorm.DB
	S3Client  *minio.Client
	IAMClient *seaweedfs.Client
	Region    string
}

// CreateBucket creates a storage bucket bound to a site.
func (h *StorageHandler) CreateBucket(c echo.Context) error {
	userID, _, role := GetUserFromContext(c)
	siteID := c.Param("id")

	var site models.Site
	if err := h.DB.First(&site, "id = ?", siteID).Error; err != nil {
		return errorJSON(c, http.StatusNotFound, "site not found")
	}
	if site.UserID != userID && role != "admin" && role != "superadmin" {
		return errorJSON(c, http.StatusForbidden, "access denied")
	}

	var req struct {
		Name       string `json:"name"`        // binding name (e.g. "IMAGES")
		BucketName string `json:"bucket_name"`  // S3 bucket name
	}
	if err := c.Bind(&req); err != nil {
		return errorJSON(c, http.StatusBadRequest, "invalid request")
	}
	if req.Name == "" || req.BucketName == "" {
		return errorJSON(c, http.StatusBadRequest, "name and bucket_name are required")
	}

	// Check uniqueness of bucket_name in our DB.
	var existing models.StorageBucket
	if err := h.DB.Where("bucket_name = ?", req.BucketName).First(&existing).Error; err == nil {
		return errorJSON(c, http.StatusConflict, "bucket name already taken")
	}

	// Create bucket via S3 API.
	if err := h.S3Client.MakeBucket(context.Background(), req.BucketName, minio.MakeBucketOptions{Region: h.Region}); err != nil {
		return errorJSON(c, http.StatusInternalServerError, "failed to create bucket: "+err.Error())
	}

	bucket := models.StorageBucket{
		SiteID:     siteID,
		Name:       req.Name,
		BucketName: req.BucketName,
	}
	if err := h.DB.Create(&bucket).Error; err != nil {
		return errorJSON(c, http.StatusInternalServerError, "failed to save bucket")
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
	if err := h.S3Client.RemoveBucket(context.Background(), bucket.BucketName); err != nil {
		return errorJSON(c, http.StatusInternalServerError, "failed to delete bucket: "+err.Error())
	}

	if err := h.DB.Delete(&bucket).Error; err != nil {
		return errorJSON(c, http.StatusInternalServerError, "failed to delete bucket")
	}

	return c.NoContent(http.StatusNoContent)
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

	// IAM username: prefix with user ID for uniqueness.
	userName := fmt.Sprintf("hd-%s-%s", userID, req.Name)

	// Create IAM user.
	if err := h.IAMClient.CreateUser(userName); err != nil {
		return errorJSON(c, http.StatusInternalServerError, "failed to create IAM user: "+err.Error())
	}

	// Create access key for the user.
	keyResult, err := h.IAMClient.CreateAccessKey(userName)
	if err != nil {
		return errorJSON(c, http.StatusInternalServerError, "failed to create access key: "+err.Error())
	}

	// Grant access to all user's site buckets via IAM policy.
	var buckets []models.StorageBucket
	h.DB.Joins("JOIN sites ON sites.id = storage_buckets.site_id").
		Where("sites.user_id = ?", userID).Find(&buckets)

	if len(buckets) > 0 {
		var resources []interface{}
		for _, b := range buckets {
			resources = append(resources, fmt.Sprintf("arn:aws:s3:::%s", b.BucketName))
			resources = append(resources, fmt.Sprintf("arn:aws:s3:::%s/*", b.BucketName))
		}
		policy := map[string]interface{}{
			"Version": "2012-10-17",
			"Statement": []map[string]interface{}{
				{
					"Effect":   "Allow",
					"Action":   "s3:*",
					"Resource": resources,
				},
			},
		}
		policyJSON, _ := json.Marshal(policy)
		_ = h.IAMClient.PutUserPolicy(userName, "bucket-access", string(policyJSON))
	}

	cred := models.S3Credential{
		UserID:        userID,
		ExternalKeyID: userName,
		AccessKeyID:   keyResult.AccessKeyID,
		Name:          req.Name,
	}
	if err := h.DB.Create(&cred).Error; err != nil {
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

	// Delete IAM user (also removes its access keys).
	if err := h.IAMClient.DeleteUser(cred.ExternalKeyID); err != nil {
		return errorJSON(c, http.StatusInternalServerError, "failed to delete IAM user: "+err.Error())
	}

	if err := h.DB.Delete(&cred).Error; err != nil {
		return errorJSON(c, http.StatusInternalServerError, "failed to delete credential")
	}

	return c.NoContent(http.StatusNoContent)
}
