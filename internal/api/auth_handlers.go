package api

import (
	"net/http"
	"strings"
	"time"

	"github.com/cryguy/hostedat/internal/auth"
	"github.com/cryguy/hostedat/internal/models"
	"github.com/labstack/echo/v4"
	"gorm.io/gorm"
)

type AuthHandler struct {
	DB        *gorm.DB
	JWTSecret string
}

type registerRequest struct {
	Email      string `json:"email"`
	Password   string `json:"password"`
	InviteCode string `json:"invite_code,omitempty"`
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type authResponse struct {
	Token string      `json:"token"`
	User  models.User `json:"user"`
}

func (h *AuthHandler) Register(c echo.Context) error {
	var req registerRequest
	if err := c.Bind(&req); err != nil {
		return errorJSON(c, http.StatusBadRequest, "invalid request body")
	}

	req.Email = strings.TrimSpace(strings.ToLower(req.Email))
	if req.Email == "" || req.Password == "" {
		return errorJSON(c, http.StatusBadRequest, "email and password are required")
	}
	if len(req.Password) < 8 {
		return errorJSON(c, http.StatusBadRequest, "password must be at least 8 characters")
	}

	// Check registration settings
	regEnabled, err := models.GetSetting(h.DB, "registration_enabled")
	if err != nil || regEnabled != "true" {
		return errorJSON(c, http.StatusForbidden, "registration is disabled")
	}

	// Check invite requirement
	var inviteID *string
	inviteRequired, _ := models.GetSetting(h.DB, "invite_required")
	if inviteRequired == "true" {
		if req.InviteCode == "" {
			return errorJSON(c, http.StatusBadRequest, "invite code is required")
		}

		var invite models.Invite
		if err := h.DB.Where("code = ? AND active = ?", req.InviteCode, true).First(&invite).Error; err != nil {
			return errorJSON(c, http.StatusBadRequest, "invalid invite code")
		}

		if invite.ExpiresAt != nil && invite.ExpiresAt.Before(time.Now()) {
			return errorJSON(c, http.StatusBadRequest, "invite code has expired")
		}

		if invite.MaxUses != nil && invite.UseCount >= *invite.MaxUses {
			return errorJSON(c, http.StatusBadRequest, "invite code has reached max uses")
		}

		inviteID = &invite.ID
	}

	// Check for duplicate email
	var existing models.User
	if err := h.DB.Where("email = ?", req.Email).First(&existing).Error; err == nil {
		return errorJSON(c, http.StatusConflict, "email already registered")
	}

	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		return errorJSON(c, http.StatusInternalServerError, "failed to hash password")
	}

	// First user becomes superadmin
	var userCount int64
	h.DB.Model(&models.User{}).Count(&userCount)
	role := "user"
	if userCount == 0 {
		role = "superadmin"
	}

	user := models.User{
		Email:        req.Email,
		PasswordHash: hash,
		Role:         role,
		InvitedBy:    inviteID,
	}

	if err := h.DB.Create(&user).Error; err != nil {
		return errorJSON(c, http.StatusInternalServerError, "failed to create user")
	}

	// Increment invite use count
	if inviteID != nil {
		h.DB.Model(&models.Invite{}).Where("id = ?", *inviteID).UpdateColumn("use_count", gorm.Expr("use_count + 1"))
	}

	token, err := auth.GenerateToken(user.ID, user.Email, user.Role, h.JWTSecret)
	if err != nil {
		return errorJSON(c, http.StatusInternalServerError, "failed to generate token")
	}

	return c.JSON(http.StatusCreated, authResponse{Token: token, User: user})
}

func (h *AuthHandler) Login(c echo.Context) error {
	var req loginRequest
	if err := c.Bind(&req); err != nil {
		return errorJSON(c, http.StatusBadRequest, "invalid request body")
	}

	req.Email = strings.TrimSpace(strings.ToLower(req.Email))
	if req.Email == "" || req.Password == "" {
		return errorJSON(c, http.StatusBadRequest, "email and password are required")
	}

	var user models.User
	if err := h.DB.Where("email = ?", req.Email).First(&user).Error; err != nil {
		return errorJSON(c, http.StatusUnauthorized, "invalid credentials")
	}

	if err := auth.CheckPassword(req.Password, user.PasswordHash); err != nil {
		return errorJSON(c, http.StatusUnauthorized, "invalid credentials")
	}

	token, err := auth.GenerateToken(user.ID, user.Email, user.Role, h.JWTSecret)
	if err != nil {
		return errorJSON(c, http.StatusInternalServerError, "failed to generate token")
	}

	return c.JSON(http.StatusOK, authResponse{Token: token, User: user})
}

func (h *AuthHandler) Logout(c echo.Context) error {
	return c.JSON(http.StatusOK, map[string]string{"message": "logged out"})
}
