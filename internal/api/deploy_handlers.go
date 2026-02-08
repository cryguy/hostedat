package api

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"log"
	"net/http"
	"os"

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

	// Stream to temp file while computing hash
	tmpFile, err := os.CreateTemp("", "hostedat-deploy-*.zip")
	if err != nil {
		return errorJSON(c, http.StatusInternalServerError, "failed to create temp file")
	}
	defer os.Remove(tmpFile.Name())
	defer tmpFile.Close()

	hasher := sha256.New()
	written, err := io.Copy(tmpFile, io.TeeReader(io.LimitReader(src, storage.MaxZipSize+1), hasher))
	if err != nil {
		return errorJSON(c, http.StatusInternalServerError, "failed to read uploaded file")
	}
	if written > storage.MaxZipSize {
		return errorJSON(c, http.StatusBadRequest, "file too large")
	}

	fileHash := hex.EncodeToString(hasher.Sum(nil))

	// Determine next version
	var maxVersion int
	h.DB.Model(&models.Deployment{}).Where("site_id = ?", siteID).Select("COALESCE(MAX(version), 0)").Scan(&maxVersion)
	nextVersion := maxVersion + 1

	// Extract zip — *os.File implements io.ReaderAt
	if _, err := tmpFile.Seek(0, io.SeekStart); err != nil {
		return errorJSON(c, http.StatusInternalServerError, "failed to process uploaded file")
	}
	if err := h.Storage.ExtractZip(siteID, nextVersion, tmpFile, written); err != nil {
		log.Printf("deploy: failed to extract zip for site %s: %v", siteID, err)
		return errorJSON(c, http.StatusBadRequest, "invalid zip file")
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
