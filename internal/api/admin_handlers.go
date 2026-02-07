package api

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"strconv"
	"time"

	"github.com/cryguy/hostedat/internal/models"
	"github.com/cryguy/hostedat/internal/storage"
	"github.com/labstack/echo/v4"
	"gorm.io/gorm"
)

type AdminHandler struct {
	DB      *gorm.DB
	Storage *storage.Manager
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

	// Delete user's sites and storage
	var sites []models.Site
	h.DB.Where("user_id = ?", targetID).Find(&sites)
	for _, site := range sites {
		h.DB.Where("site_id = ?", site.ID).Delete(&models.Deployment{})
		_ = h.Storage.DeleteSite(site.ID)
	}
	h.DB.Where("user_id = ?", targetID).Delete(&models.Site{})
	h.DB.Where("user_id = ?", targetID).Delete(&models.APIKey{})
	h.DB.Delete(&target)

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

	b := make([]byte, 4)
	rand.Read(b)
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

	return c.JSON(http.StatusOK, map[string]string{"message": "invite revoked"})
}
