package api

import (
	"net/http"

	"github.com/cryguy/hostedat/internal/auth"
	"github.com/cryguy/hostedat/internal/models"
	"github.com/labstack/echo/v4"
	"gorm.io/gorm"
)

type APIKeyHandler struct {
	DB *gorm.DB
}

type createKeyRequest struct {
	Name string `json:"name"`
}

func (h *APIKeyHandler) Create(c echo.Context) error {
	userID, _, _ := GetUserFromContext(c)

	var req createKeyRequest
	if err := c.Bind(&req); err != nil {
		return errorJSON(c, http.StatusBadRequest, "invalid request body")
	}

	if req.Name == "" {
		return errorJSON(c, http.StatusBadRequest, "name is required")
	}

	rawKey, hash, err := auth.GenerateAPIKey()
	if err != nil {
		return errorJSON(c, http.StatusInternalServerError, "failed to generate API key")
	}

	key := models.APIKey{
		UserID:  userID,
		KeyHash: hash,
		Name:    req.Name,
	}

	if err := h.DB.Create(&key).Error; err != nil {
		return errorJSON(c, http.StatusInternalServerError, "failed to create API key")
	}

	return c.JSON(http.StatusCreated, map[string]interface{}{
		"id":         key.ID,
		"name":       key.Name,
		"key":        rawKey,
		"created_at": key.CreatedAt,
	})
}

func (h *APIKeyHandler) List(c echo.Context) error {
	userID, _, _ := GetUserFromContext(c)

	var keys []models.APIKey
	if err := h.DB.Where("user_id = ?", userID).Order("created_at DESC").Find(&keys).Error; err != nil {
		return errorJSON(c, http.StatusInternalServerError, "failed to list API keys")
	}

	return c.JSON(http.StatusOK, keys)
}

func (h *APIKeyHandler) Delete(c echo.Context) error {
	userID, _, _ := GetUserFromContext(c)
	keyID := c.Param("id")

	var key models.APIKey
	if err := h.DB.First(&key, "id = ?", keyID).Error; err != nil {
		return errorJSON(c, http.StatusNotFound, "API key not found")
	}

	if key.UserID != userID {
		return errorJSON(c, http.StatusForbidden, "access denied")
	}

	if err := h.DB.Delete(&key).Error; err != nil {
		return errorJSON(c, http.StatusInternalServerError, "failed to delete API key")
	}

	return c.JSON(http.StatusOK, map[string]string{"message": "API key deleted"})
}
