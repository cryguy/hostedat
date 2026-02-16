package api

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"

	"github.com/cryguy/hostedat/internal/models"
	"github.com/labstack/echo/v4"
	"gorm.io/gorm"
)

// StorageHandler handles management API endpoints for object storage.
type StorageHandler struct {
	DB *gorm.DB
}

type createCredentialResponse struct {
	ID              string `json:"id"`
	SiteID          string `json:"site_id"`
	AccessKeyID     string `json:"access_key_id"`
	SecretAccessKey string `json:"secret_access_key"`
	CreatedAt       string `json:"created_at"`
}

type credentialListItem struct {
	ID          string `json:"id"`
	AccessKeyID string `json:"access_key_id"`
	CreatedAt   string `json:"created_at"`
}

type storageUsageResponse struct {
	TotalSize   int64 `json:"total_size"`
	ObjectCount int64 `json:"object_count"`
}

// CreateCredential creates a new S3 access key pair for a site.
// POST /api/v1/sites/:id/storage/credentials
func (h *StorageHandler) CreateCredential(c echo.Context) error {
	siteID := c.Param("id")
	userID, _, _ := GetUserFromContext(c)

	// Verify site ownership.
	var site models.Site
	if err := h.DB.First(&site, "id = ?", siteID).Error; err != nil {
		return errorJSON(c, http.StatusNotFound, "site not found")
	}
	if site.UserID != userID {
		return errorJSON(c, http.StatusForbidden, "access denied")
	}

	// Generate access key ID and secret access key.
	accessKeyID := generateAccessKeyID()
	secretAccessKey := generateSecretAccessKey()

	cred := models.StorageCredential{
		SiteID:          siteID,
		AccessKeyID:     accessKeyID,
		SecretAccessKey: secretAccessKey,
		SecretKeyHash:   "", // We store the raw key for SigV4 HMAC
	}

	if err := h.DB.Create(&cred).Error; err != nil {
		return errorJSON(c, http.StatusInternalServerError, "failed to create credential")
	}

	return c.JSON(http.StatusCreated, createCredentialResponse{
		ID:              cred.ID,
		SiteID:          siteID,
		AccessKeyID:     accessKeyID,
		SecretAccessKey: secretAccessKey,
		CreatedAt:       cred.CreatedAt.Format("2006-01-02T15:04:05Z"),
	})
}

// ListCredentials lists S3 credentials for a site.
// GET /api/v1/sites/:id/storage/credentials
func (h *StorageHandler) ListCredentials(c echo.Context) error {
	siteID := c.Param("id")
	userID, _, _ := GetUserFromContext(c)

	var site models.Site
	if err := h.DB.First(&site, "id = ?", siteID).Error; err != nil {
		return errorJSON(c, http.StatusNotFound, "site not found")
	}
	if site.UserID != userID {
		return errorJSON(c, http.StatusForbidden, "access denied")
	}

	var creds []models.StorageCredential
	h.DB.Where("site_id = ?", siteID).Find(&creds)

	items := make([]credentialListItem, 0, len(creds))
	for _, cr := range creds {
		items = append(items, credentialListItem{
			ID:          cr.ID,
			AccessKeyID: cr.AccessKeyID,
			CreatedAt:   cr.CreatedAt.Format("2006-01-02T15:04:05Z"),
		})
	}

	return c.JSON(http.StatusOK, items)
}

// DeleteCredential revokes an S3 credential.
// DELETE /api/v1/sites/:id/storage/credentials/:accessKeyId
func (h *StorageHandler) DeleteCredential(c echo.Context) error {
	siteID := c.Param("id")
	accessKeyID := c.Param("accessKeyId")
	userID, _, _ := GetUserFromContext(c)

	var site models.Site
	if err := h.DB.First(&site, "id = ?", siteID).Error; err != nil {
		return errorJSON(c, http.StatusNotFound, "site not found")
	}
	if site.UserID != userID {
		return errorJSON(c, http.StatusForbidden, "access denied")
	}

	result := h.DB.Where("site_id = ? AND access_key_id = ?", siteID, accessKeyID).Delete(&models.StorageCredential{})
	if result.RowsAffected == 0 {
		return errorJSON(c, http.StatusNotFound, "credential not found")
	}

	return c.NoContent(http.StatusNoContent)
}

// GetUsage returns storage usage stats for a site.
// GET /api/v1/sites/:id/storage/usage
func (h *StorageHandler) GetUsage(c echo.Context) error {
	siteID := c.Param("id")
	userID, _, _ := GetUserFromContext(c)

	var site models.Site
	if err := h.DB.First(&site, "id = ?", siteID).Error; err != nil {
		return errorJSON(c, http.StatusNotFound, "site not found")
	}
	if site.UserID != userID {
		return errorJSON(c, http.StatusForbidden, "access denied")
	}

	var totalSize int64
	var objectCount int64

	h.DB.Model(&models.StorageObject{}).Where("site_id = ?", siteID).Count(&objectCount)

	row := h.DB.Model(&models.StorageObject{}).Where("site_id = ?", siteID).Select("COALESCE(SUM(size), 0)").Row()
	row.Scan(&totalSize)

	return c.JSON(http.StatusOK, storageUsageResponse{
		TotalSize:   totalSize,
		ObjectCount: objectCount,
	})
}

// generateAccessKeyID generates an S3-style access key ID (20 chars, uppercase).
func generateAccessKeyID() string {
	b := make([]byte, 10)
	rand.Read(b)
	return "HDAK" + hex.EncodeToString(b)[:16]
}

// generateSecretAccessKey generates an S3-style secret access key (40 chars).
func generateSecretAccessKey() string {
	b := make([]byte, 30)
	rand.Read(b)
	return hex.EncodeToString(b)[:40]
}
