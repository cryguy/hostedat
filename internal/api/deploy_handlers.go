package api

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"

	"github.com/cryguy/hostedat/internal/models"
	"github.com/cryguy/hostedat/internal/storage"
	"github.com/labstack/echo/v4"
	"gorm.io/gorm"
)

type DeployHandler struct {
	DB      *gorm.DB
	Storage *storage.Manager
}

func (h *DeployHandler) Deploy(c echo.Context) error {
	userID, _, role := GetUserFromContext(c)
	siteID := c.Param("id")

	var site models.Site
	if err := h.DB.First(&site, "id = ?", siteID).Error; err != nil {
		return errorJSON(c, http.StatusNotFound, "site not found")
	}

	if site.UserID != userID && role != "admin" && role != "superadmin" {
		return errorJSON(c, http.StatusForbidden, "access denied")
	}

	// Read uploaded zip
	file, err := c.FormFile("file")
	if err != nil {
		return errorJSON(c, http.StatusBadRequest, "file is required (multipart field 'file')")
	}

	src, err := file.Open()
	if err != nil {
		return errorJSON(c, http.StatusInternalServerError, "failed to open uploaded file")
	}
	defer src.Close()

	// Read entire file into memory for hashing and zip processing
	data, err := io.ReadAll(src)
	if err != nil {
		return errorJSON(c, http.StatusInternalServerError, "failed to read uploaded file")
	}

	// SHA-256 hash
	hashBytes := sha256.Sum256(data)
	fileHash := hex.EncodeToString(hashBytes[:])

	// Determine next version
	var maxVersion int
	h.DB.Model(&models.Deployment{}).Where("site_id = ?", siteID).Select("COALESCE(MAX(version), 0)").Scan(&maxVersion)
	nextVersion := maxVersion + 1

	// Extract zip
	reader := bytes.NewReader(data)
	if err := h.Storage.ExtractZip(siteID, nextVersion, reader, int64(len(data))); err != nil {
		return errorJSON(c, http.StatusBadRequest, "failed to extract zip: "+err.Error())
	}

	// Create deployment record
	deployment := models.Deployment{
		SiteID:   siteID,
		Version:  nextVersion,
		FileHash: fileHash,
	}
	if err := h.DB.Create(&deployment).Error; err != nil {
		return errorJSON(c, http.StatusInternalServerError, "failed to create deployment")
	}

	// Update site active version
	h.DB.Model(&site).Update("active_version", nextVersion)

	return c.JSON(http.StatusCreated, deployment)
}

func (h *DeployHandler) List(c echo.Context) error {
	userID, _, role := GetUserFromContext(c)
	siteID := c.Param("id")

	var site models.Site
	if err := h.DB.First(&site, "id = ?", siteID).Error; err != nil {
		return errorJSON(c, http.StatusNotFound, "site not found")
	}

	if site.UserID != userID && role != "admin" && role != "superadmin" {
		return errorJSON(c, http.StatusForbidden, "access denied")
	}

	var deployments []models.Deployment
	if err := h.DB.Where("site_id = ?", siteID).Order("version DESC").Find(&deployments).Error; err != nil {
		return errorJSON(c, http.StatusInternalServerError, "failed to list deployments")
	}

	return c.JSON(http.StatusOK, deployments)
}
