package api

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"

	"github.com/cryguy/hostedat/internal/models"
	"github.com/cryguy/hostedat/internal/storage"
	"github.com/labstack/echo/v4"
	"gorm.io/gorm"
)

type DeployHandler struct {
	DB           *gorm.DB
	Storage      *storage.Manager
	WorkerEngine interface {
		CompileAndCache(siteID string, version int, source string) ([]byte, error)
		InvalidatePool(siteID string, version int)
	}
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

	// Check for _worker.js and compile if present
	hasWorker := h.Storage.HasWorkerScript(siteID, nextVersion)
	if hasWorker && h.WorkerEngine != nil {
		source, err := h.Storage.GetWorkerScript(siteID, nextVersion)
		if err != nil {
			return errorJSON(c, http.StatusBadRequest, "failed to read _worker.js: "+err.Error())
		}

		// Validate script size
		if len(source) > 1024*1024 { // 1MB default
			return errorJSON(c, http.StatusBadRequest, "_worker.js exceeds maximum size")
		}

		// Compile to bytecode
		bytecode, err := h.WorkerEngine.CompileAndCache(siteID, nextVersion, source)
		if err != nil {
			return errorJSON(c, http.StatusBadRequest, "worker compilation failed: "+err.Error())
		}

		// Save bytecode to disk for persistence across restarts
		bcDir := h.Storage.GetWorkerBytecodeDir(siteID, nextVersion)
		if mkErr := os.MkdirAll(bcDir, 0755); mkErr == nil {
			_ = os.WriteFile(filepath.Join(bcDir, "bytecode.bin"), bytecode, 0644)
		}
	}

	// Save the old active version before updating so we can invalidate its pool
	// AFTER the DB update, avoiding a race where requests see the old version
	// but find no valid pool for it.
	oldActiveVersion := site.ActiveVersion

	// Create deployment record
	deployment := models.Deployment{
		SiteID:    siteID,
		Version:   nextVersion,
		FileHash:  fileHash,
		HasWorker: hasWorker,
	}
	if err := h.DB.Create(&deployment).Error; err != nil {
		return errorJSON(c, http.StatusInternalServerError, "failed to create deployment")
	}

	// Update site active version and worker flag
	h.DB.Model(&site).Updates(map[string]interface{}{
		"active_version": nextVersion,
		"has_worker":     hasWorker,
	})

	// Invalidate old worker pool AFTER DB update. This ensures requests
	// reading the old version still have a valid pool until the switch.
	if hasWorker && h.WorkerEngine != nil && oldActiveVersion != nil {
		h.WorkerEngine.InvalidatePool(siteID, *oldActiveVersion)
	}

	return c.JSON(http.StatusCreated, deployment)
}

func (h *DeployHandler) Rollback(c echo.Context) error {
	userID, _, role := GetUserFromContext(c)
	siteID := c.Param("id")
	version := 0
	if err := echo.PathParamsBinder(c).Int("version", &version).BindError(); err != nil || version <= 0 {
		return errorJSON(c, http.StatusBadRequest, "invalid version")
	}

	var site models.Site
	if err := h.DB.First(&site, "id = ?", siteID).Error; err != nil {
		return errorJSON(c, http.StatusNotFound, "site not found")
	}

	if site.UserID != userID && role != "admin" && role != "superadmin" {
		return errorJSON(c, http.StatusForbidden, "access denied")
	}

	if site.ActiveVersion != nil && *site.ActiveVersion == version {
		return errorJSON(c, http.StatusBadRequest, "already the active version")
	}

	// Verify the deployment exists
	var dep models.Deployment
	if err := h.DB.First(&dep, "site_id = ? AND version = ?", siteID, version).Error; err != nil {
		return errorJSON(c, http.StatusNotFound, "deployment not found")
	}

	// Invalidate old worker pool if applicable
	if h.WorkerEngine != nil && site.ActiveVersion != nil {
		h.WorkerEngine.InvalidatePool(siteID, *site.ActiveVersion)
	}

	// Update site to the target version
	h.DB.Model(&site).Updates(map[string]interface{}{
		"active_version": version,
		"has_worker":     dep.HasWorker,
	})

	return c.JSON(http.StatusOK, map[string]interface{}{
		"message":        "rolled back",
		"active_version": version,
	})
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
