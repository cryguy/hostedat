package api

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"regexp"
	"strings"

	gonanoid "github.com/matoous/go-nanoid/v2"

	"github.com/cryguy/hostedat/internal/models"
	"github.com/cryguy/hostedat/internal/seaweedfs"
	"github.com/cryguy/hostedat/internal/storage"
	"github.com/labstack/echo/v4"
	minio "github.com/minio/minio-go/v7"
	"gorm.io/gorm"
)

var slugRegex = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{1,61}[a-z0-9]$`)

var reservedSlugs = map[string]bool{
	"www": true, "api": true, "admin": true, "mail": true,
	"smtp": true, "ftp": true, "ns1": true, "ns2": true,
	"app": true, "dashboard": true, "status": true, "storage": true,
}

type SiteHandler struct {
	DB        *gorm.DB
	Storage   *storage.Manager
	S3Client  *minio.Client  // optional; nil when object storage is disabled
	IAMClient *seaweedfs.Client // optional; nil when object storage is disabled
}

type createSiteRequest struct {
	Name          string `json:"name"`
	SubdomainSlug string `json:"subdomain_slug,omitempty"`
}

type updateSiteRequest struct {
	Name    *string `json:"name,omitempty"`
	SPAMode *bool   `json:"spa_mode,omitempty"`
}

func (h *SiteHandler) Create(c echo.Context) error {
	userID, _, _ := GetUserFromContext(c)

	var req createSiteRequest
	if err := c.Bind(&req); err != nil {
		return errorJSON(c, http.StatusBadRequest, "invalid request body")
	}

	if req.Name == "" {
		return errorJSON(c, http.StatusBadRequest, "name is required")
	}

	slug := req.SubdomainSlug
	if slug == "" {
		// Auto-generate from name
		slug = strings.ToLower(strings.ReplaceAll(req.Name, " ", "-"))
		slug = regexp.MustCompile(`[^a-z0-9-]`).ReplaceAllString(slug, "")
		if len(slug) < 3 {
			suffix, _ := gonanoid.Generate("0123456789abcdefghijklmnopqrstuvwxyz", 6)
			slug = slug + suffix
		}
	}

	slug = strings.ToLower(slug)
	if !slugRegex.MatchString(slug) {
		return errorJSON(c, http.StatusBadRequest, "subdomain slug must be 3-63 chars, lowercase alphanumeric and hyphens")
	}
	if reservedSlugs[slug] {
		return errorJSON(c, http.StatusBadRequest, "subdomain slug is reserved")
	}

	// Check uniqueness
	var existing models.Site
	if err := h.DB.Where("subdomain_slug = ?", slug).First(&existing).Error; err == nil {
		return errorJSON(c, http.StatusConflict, "subdomain slug already taken")
	}

	site := models.Site{
		UserID:        userID,
		SubdomainSlug: slug,
		Name:          req.Name,
	}

	if err := h.DB.Create(&site).Error; err != nil {
		return errorJSON(c, http.StatusInternalServerError, "failed to create site")
	}

	return c.JSON(http.StatusCreated, site)
}

func (h *SiteHandler) List(c echo.Context) error {
	userID, _, role := GetUserFromContext(c)

	var sites []models.Site
	query := h.DB

	if c.QueryParam("all") == "true" && (role == "admin" || role == "superadmin") {
		// Admin: list all sites
	} else {
		query = query.Where("user_id = ?", userID)
	}

	if err := query.Order("created_at DESC").Find(&sites).Error; err != nil {
		return errorJSON(c, http.StatusInternalServerError, "failed to list sites")
	}

	return c.JSON(http.StatusOK, sites)
}

func (h *SiteHandler) Get(c echo.Context) error {
	userID, _, role := GetUserFromContext(c)
	siteID := c.Param("id")

	var site models.Site
	if err := h.DB.First(&site, "id = ?", siteID).Error; err != nil {
		return errorJSON(c, http.StatusNotFound, "site not found")
	}

	if site.UserID != userID && role != "admin" && role != "superadmin" {
		return errorJSON(c, http.StatusForbidden, "access denied")
	}

	return c.JSON(http.StatusOK, site)
}

func (h *SiteHandler) Update(c echo.Context) error {
	userID, _, role := GetUserFromContext(c)
	siteID := c.Param("id")

	var site models.Site
	if err := h.DB.First(&site, "id = ?", siteID).Error; err != nil {
		return errorJSON(c, http.StatusNotFound, "site not found")
	}

	if site.UserID != userID && role != "admin" && role != "superadmin" {
		return errorJSON(c, http.StatusForbidden, "access denied")
	}

	var req updateSiteRequest
	if err := c.Bind(&req); err != nil {
		return errorJSON(c, http.StatusBadRequest, "invalid request body")
	}

	if req.Name != nil {
		site.Name = *req.Name
	}
	if req.SPAMode != nil {
		site.SPAMode = *req.SPAMode
	}

	if err := h.DB.Save(&site).Error; err != nil {
		return errorJSON(c, http.StatusInternalServerError, "failed to update site")
	}

	return c.JSON(http.StatusOK, site)
}

func (h *SiteHandler) Delete(c echo.Context) error {
	userID, _, role := GetUserFromContext(c)
	siteID := c.Param("id")

	var site models.Site
	if err := h.DB.First(&site, "id = ?", siteID).Error; err != nil {
		return errorJSON(c, http.StatusNotFound, "site not found")
	}

	if site.UserID != userID && role != "admin" && role != "superadmin" {
		return errorJSON(c, http.StatusForbidden, "access denied")
	}

	// Collect storage buckets and S3 credentials before deleting DB records.
	var storageBuckets []models.StorageBucket
	h.DB.Where("site_id = ?", siteID).Find(&storageBuckets)

	var s3Creds []models.S3Credential
	h.DB.Where("user_id = ?", site.UserID).Find(&s3Creds)

	// Delete all child records and the site in a single transaction.
	if err := h.DB.Transaction(func(tx *gorm.DB) error {
		// Delete KV entries via namespace IDs for this site.
		var nsIDs []string
		tx.Model(&models.KVNamespace{}).Where("site_id = ?", siteID).Pluck("id", &nsIDs)
		if len(nsIDs) > 0 {
			if err := tx.Where("namespace_id IN ?", nsIDs).Delete(&models.KVEntry{}).Error; err != nil {
				return err
			}
		}

		// Delete all site-scoped child records.
		for _, model := range []interface{}{
			&models.Deployment{},
			&models.WorkerEnvVar{},
			&models.KVNamespace{},
			&models.CronSchedule{},
			&models.WorkerLog{},
			&models.StorageBucket{},
		} {
			if err := tx.Where("site_id = ?", siteID).Delete(model).Error; err != nil {
				return err
			}
		}

		// Delete the site itself.
		if err := tx.Delete(&site).Error; err != nil {
			return err
		}
		return nil
	}); err != nil {
		return errorJSON(c, http.StatusInternalServerError, "failed to delete site")
	}

	// External cleanup after successful DB transaction (best-effort).
	_ = h.Storage.DeleteSite(siteID)

	// Remove external S3 buckets that belonged to this site.
	if h.S3Client != nil {
		for _, b := range storageBuckets {
			if err := h.S3Client.RemoveBucket(context.Background(), b.BucketName); err != nil {
				log.Printf("warning: failed to remove external bucket %s: %v", b.BucketName, err)
			}
		}
	}

	// Reconcile IAM policies for the user's remaining credentials (the site's
	// buckets are gone, so policies must be updated to remove access).
	if h.IAMClient != nil && len(s3Creds) > 0 {
		reconcileIAMPoliciesForUser(h.DB, h.IAMClient, site.UserID)
	}

	return c.JSON(http.StatusOK, map[string]string{"message": "site deleted"})
}

// reconcileIAMPoliciesForUser rebuilds IAM policies for all S3 credentials
// belonging to a user after bucket changes. Best-effort: logs errors but
// does not propagate them since the primary DB operation already succeeded.
func reconcileIAMPoliciesForUser(db *gorm.DB, iamClient *seaweedfs.Client, userID string) {
	var creds []models.S3Credential
	if err := db.Where("user_id = ?", userID).Find(&creds).Error; err != nil {
		log.Printf("warning: failed to load S3 credentials for user %s: %v", userID, err)
		return
	}
	if len(creds) == 0 {
		return
	}

	// Build updated policy from remaining buckets.
	var buckets []models.StorageBucket
	if err := db.Joins("JOIN sites ON sites.id = storage_buckets.site_id").
		Where("sites.user_id = ?", userID).
		Find(&buckets).Error; err != nil {
		log.Printf("warning: failed to load buckets for user %s: %v", userID, err)
		return
	}

	resources := make([]string, 0, len(buckets)*2)
	for _, b := range buckets {
		resources = append(resources, "arn:aws:s3:::"+b.BucketName)
		resources = append(resources, "arn:aws:s3:::"+b.BucketName+"/*")
	}

	statements := []map[string]interface{}{}
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
		log.Printf("warning: failed to marshal IAM policy for user %s: %v", userID, err)
		return
	}

	for _, cred := range creds {
		if err := iamClient.PutUserPolicy(cred.ExternalKeyID, "bucket-access", string(policyJSON)); err != nil {
			log.Printf("warning: failed to update IAM policy for credential %s: %v", cred.ID, err)
		}
	}
}

// revokeIAMCredentials deletes external IAM access keys and users for the
// given S3 credentials. Best-effort: logs errors instead of propagating them.
func revokeIAMCredentials(iamClient *seaweedfs.Client, creds []models.S3Credential) {
	for _, cred := range creds {
		if err := iamClient.DeleteAccessKey(cred.AccessKeyID); err != nil {
			log.Printf("warning: failed to delete IAM access key %s: %v", cred.AccessKeyID, err)
		}
		if err := iamClient.DeleteUser(cred.ExternalKeyID); err != nil {
			log.Printf("warning: failed to delete IAM user %s: %v", cred.ExternalKeyID, err)
		}
	}
}
