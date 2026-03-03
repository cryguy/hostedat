package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/cryguy/hostedat/internal/audit"
	"github.com/cryguy/hostedat/internal/models"
	"github.com/cryguy/hostedat/internal/seaweedfs"
	"github.com/cryguy/hostedat/internal/storage"
	"github.com/labstack/echo/v4"
	"github.com/minio/minio-go/v7"
	"gorm.io/gorm"
)

type AdminHandler struct {
	DB        *gorm.DB
	Storage   *storage.Manager
	S3Client  *minio.Client     // optional; nil when object storage is disabled
	IAMClient *seaweedfs.Client // optional; nil when object storage is disabled
}

func (h *AdminHandler) ListUsers(c echo.Context) error {
	page, _ := strconv.Atoi(c.QueryParam("page"))
	if page < 1 {
		page = 1
	}
	perPage := 20
	offset := (page - 1) * perPage

	var users []models.User
	var total int64
	h.DB.Model(&models.User{}).Count(&total)
	h.DB.Order("created_at DESC").Offset(offset).Limit(perPage).Find(&users)

	return c.JSON(http.StatusOK, map[string]interface{}{
		"users": users,
		"total": total,
		"page":  page,
	})
}

type updateRoleRequest struct {
	Role string `json:"role"`
}

func (h *AdminHandler) UpdateUserRole(c echo.Context) error {
	targetID := c.Param("id")

	var target models.User
	if err := h.DB.First(&target, "id = ?", targetID).Error; err != nil {
		return errorJSON(c, http.StatusNotFound, "user not found")
	}

	if target.Role == "superadmin" {
		return errorJSON(c, http.StatusForbidden, "cannot change superadmin role")
	}

	var req updateRoleRequest
	if err := c.Bind(&req); err != nil {
		return errorJSON(c, http.StatusBadRequest, "invalid request body")
	}

	if req.Role != "admin" && req.Role != "user" {
		return errorJSON(c, http.StatusBadRequest, "role must be 'admin' or 'user'")
	}

	target.Role = req.Role
	if err := h.DB.Save(&target).Error; err != nil {
		return errorJSON(c, http.StatusInternalServerError, "failed to update role")
	}

	audit.Record(h.DB, c, "admin.user.role", "user", targetID, audit.Ptr(req.Role))

	return c.JSON(http.StatusOK, target)
}

func (h *AdminHandler) DeleteUser(c echo.Context) error {
	targetID := c.Param("id")

	var target models.User
	if err := h.DB.First(&target, "id = ?", targetID).Error; err != nil {
		return errorJSON(c, http.StatusNotFound, "user not found")
	}

	if target.Role == "superadmin" {
		return errorJSON(c, http.StatusForbidden, "cannot delete superadmin")
	}

	// Collect data for external cleanup after DB transaction.
	var sites []models.Site
	h.DB.Where("user_id = ?", targetID).Find(&sites)
	var siteIDs []string
	for _, site := range sites {
		siteIDs = append(siteIDs, site.ID)
	}

	var storageBuckets []models.StorageBucket
	for _, siteID := range siteIDs {
		var buckets []models.StorageBucket
		h.DB.Where("site_id = ?", siteID).Find(&buckets)
		storageBuckets = append(storageBuckets, buckets...)
	}

	var s3Creds []models.S3Credential
	h.DB.Where("user_id = ?", targetID).Find(&s3Creds)

	// Delete all user data in a transaction.
	if err := h.DB.Transaction(func(tx *gorm.DB) error {
		for _, siteID := range siteIDs {
			// Delete KV entries via namespace IDs.
			var nsIDs []string
			tx.Model(&models.KVNamespace{}).Where("site_id = ?", siteID).Pluck("id", &nsIDs)
			if len(nsIDs) > 0 {
				if err := tx.Where("namespace_id IN ?", nsIDs).Delete(&models.KVEntry{}).Error; err != nil {
					return err
				}
			}

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
		}
		if err := tx.Where("user_id = ?", targetID).Delete(&models.Site{}).Error; err != nil {
			return err
		}
		if err := tx.Where("user_id = ?", targetID).Delete(&models.APIKey{}).Error; err != nil {
			return err
		}
		if err := tx.Where("user_id = ?", targetID).Delete(&models.S3Credential{}).Error; err != nil {
			return err
		}
		return tx.Delete(&target).Error
	}); err != nil {
		return errorJSON(c, http.StatusInternalServerError, "failed to delete user")
	}

	// External cleanup after successful transaction (best-effort).
	for _, siteID := range siteIDs {
		_ = h.Storage.DeleteSite(siteID)
	}

	// Remove external S3 buckets.
	if h.S3Client != nil {
		for _, b := range storageBuckets {
			if err := h.S3Client.RemoveBucket(context.Background(), b.BucketName); err != nil {
				log.Printf("warning: failed to remove external bucket %s: %v", b.BucketName, err)
			}
		}
	}

	// Revoke external IAM access keys and users.
	if h.IAMClient != nil && len(s3Creds) > 0 {
		revokeIAMCredentials(h.IAMClient, s3Creds)
	}

	audit.Record(h.DB, c, "admin.user.delete", "user", targetID, audit.Ptr(target.Email))

	return c.JSON(http.StatusOK, map[string]string{"message": "user deleted"})
}

func (h *AdminHandler) GetSettings(c echo.Context) error {
	regEnabled, _ := models.GetSetting(h.DB, "registration_enabled")
	inviteRequired, _ := models.GetSetting(h.DB, "invite_required")

	return c.JSON(http.StatusOK, map[string]interface{}{
		"registration_enabled": regEnabled == "true",
		"invite_required":      inviteRequired == "true",
	})
}

type updateSettingsRequest struct {
	RegistrationEnabled *bool `json:"registration_enabled,omitempty"`
	InviteRequired      *bool `json:"invite_required,omitempty"`
}

func (h *AdminHandler) UpdateSettings(c echo.Context) error {
	var req updateSettingsRequest
	if err := c.Bind(&req); err != nil {
		return errorJSON(c, http.StatusBadRequest, "invalid request body")
	}

	if req.RegistrationEnabled != nil {
		if err := models.SetSetting(h.DB, "registration_enabled", strconv.FormatBool(*req.RegistrationEnabled)); err != nil {
			return errorJSON(c, http.StatusInternalServerError, "failed to update setting")
		}
	}
	if req.InviteRequired != nil {
		if err := models.SetSetting(h.DB, "invite_required", strconv.FormatBool(*req.InviteRequired)); err != nil {
			return errorJSON(c, http.StatusInternalServerError, "failed to update setting")
		}
	}

	audit.Record(h.DB, c, "admin.settings.update", "settings", "", nil)

	return h.GetSettings(c)
}

type createInviteRequest struct {
	MaxUses   *int       `json:"max_uses,omitempty"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
}

func (h *AdminHandler) CreateInvite(c echo.Context) error {
	userID, _, _ := GetUserFromContext(c)

	var req createInviteRequest
	if err := c.Bind(&req); err != nil {
		return errorJSON(c, http.StatusBadRequest, "invalid request body")
	}

	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return errorJSON(c, http.StatusInternalServerError, "failed to generate invite code")
	}
	code := hex.EncodeToString(b)

	invite := models.Invite{
		Code:      code,
		CreatedBy: userID,
		MaxUses:   req.MaxUses,
		ExpiresAt: req.ExpiresAt,
		Active:    true,
	}

	if err := h.DB.Create(&invite).Error; err != nil {
		return errorJSON(c, http.StatusInternalServerError, "failed to create invite")
	}

	audit.Record(h.DB, c, "admin.invite.create", "invite", invite.ID, nil)

	return c.JSON(http.StatusCreated, invite)
}

func (h *AdminHandler) ListInvites(c echo.Context) error {
	var invites []models.Invite
	if err := h.DB.Order("created_at DESC").Find(&invites).Error; err != nil {
		return errorJSON(c, http.StatusInternalServerError, "failed to list invites")
	}
	return c.JSON(http.StatusOK, invites)
}

func (h *AdminHandler) RevokeInvite(c echo.Context) error {
	inviteID := c.Param("id")

	var invite models.Invite
	if err := h.DB.First(&invite, "id = ?", inviteID).Error; err != nil {
		return errorJSON(c, http.StatusNotFound, "invite not found")
	}

	invite.Active = false
	if err := h.DB.Save(&invite).Error; err != nil {
		return errorJSON(c, http.StatusInternalServerError, "failed to revoke invite")
	}

	audit.Record(h.DB, c, "admin.invite.revoke", "invite", inviteID, nil)

	return c.JSON(http.StatusOK, map[string]string{"message": "invite revoked"})
}
