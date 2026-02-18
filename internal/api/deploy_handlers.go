package api

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"

	"github.com/cryguy/hostedat/internal/models"
	"github.com/cryguy/hostedat/internal/storage"
	"github.com/labstack/echo/v4"
	"gorm.io/gorm"
)

// clientError wraps errors that should be returned as 400 Bad Request.
type clientError struct {
	msg string
}

func (e *clientError) Error() string { return e.msg }

type DeployHandler struct {
	DB              *gorm.DB
	Storage         *storage.Manager
	MaxScriptSizeKB int
	WorkerEngine    interface {
		CompileAndCache(siteID string, deployKey string, source string) ([]byte, error)
		InvalidatePool(siteID string, deployKey string)
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

	// Pre-generate deployment ID to use as the storage directory key.
	// This avoids incrementing numeric paths entirely — each deploy gets
	// a unique, non-sequential directory name.
	deployID := models.GenerateID()

	// Extract zip before the DB transaction using the deployment ID as path key.
	if _, err := tmpFile.Seek(0, io.SeekStart); err != nil {
		return errorJSON(c, http.StatusInternalServerError, "failed to process uploaded file")
	}
	if err := h.Storage.ExtractZip(siteID, deployID, tmpFile, written); err != nil {
		log.Printf("deploy: failed to extract zip for site %s: %v", siteID, err)
		return errorJSON(c, http.StatusBadRequest, "invalid zip file")
	}

	// Check for _worker.js and compile if present
	hasWorker := h.Storage.HasWorkerScript(siteID, deployID)
	if hasWorker && h.WorkerEngine != nil {
		source, err := h.Storage.GetWorkerScript(siteID, deployID)
		if err != nil {
			_ = os.RemoveAll(h.Storage.GetDeploymentPath(siteID, deployID))
			return errorJSON(c, http.StatusBadRequest, "failed to read _worker.js: "+err.Error())
		}

		maxScriptBytes := h.MaxScriptSizeKB * 1024
		if maxScriptBytes <= 0 {
			maxScriptBytes = 1024 * 1024
		}
		if len(source) > maxScriptBytes {
			_ = os.RemoveAll(h.Storage.GetDeploymentPath(siteID, deployID))
			return errorJSON(c, http.StatusBadRequest, fmt.Sprintf("_worker.js exceeds maximum size (%d KB)", h.MaxScriptSizeKB))
		}

		bytecode, err := h.WorkerEngine.CompileAndCache(siteID, deployID, source)
		if err != nil {
			_ = os.RemoveAll(h.Storage.GetDeploymentPath(siteID, deployID))
			return errorJSON(c, http.StatusBadRequest, "worker compilation failed: "+err.Error())
		}

		bcDir := h.Storage.GetWorkerBytecodeDir(siteID, deployID)
		if mkErr := os.MkdirAll(bcDir, 0755); mkErr == nil {
			_ = os.WriteFile(filepath.Join(bcDir, "bytecode.bin"), bytecode, 0644)
		}
	}

	// DB transaction: create deployment record and update site atomically.
	var deployment models.Deployment
	var oldDeployID *string

	txErr := h.DB.Transaction(func(tx *gorm.DB) error {
		var maxVersion int
		tx.Model(&models.Deployment{}).Where("site_id = ?", siteID).Select("COALESCE(MAX(version), 0)").Scan(&maxVersion)
		nextVersion := maxVersion + 1

		var freshSite models.Site
		if err := tx.First(&freshSite, "id = ?", siteID).Error; err != nil {
			return fmt.Errorf("reload site: %w", err)
		}
		oldDeployID = freshSite.ActiveDeployID

		deployment = models.Deployment{
			ID:        deployID,
			SiteID:    siteID,
			Version:   nextVersion,
			FileHash:  fileHash,
			HasWorker: hasWorker,
		}
		if err := tx.Create(&deployment).Error; err != nil {
			return fmt.Errorf("create deployment: %w", err)
		}

		if err := tx.Model(&freshSite).Updates(map[string]interface{}{
			"active_version":   nextVersion,
			"active_deploy_id": deployID,
			"has_worker":       hasWorker,
		}).Error; err != nil {
			return fmt.Errorf("update site: %w", err)
		}

		return nil
	})
	if txErr != nil {
		_ = os.RemoveAll(h.Storage.GetDeploymentPath(siteID, deployID))
		return errorJSON(c, http.StatusInternalServerError, "failed to create deployment")
	}

	// Invalidate old worker pool AFTER DB transaction commits.
	if hasWorker && h.WorkerEngine != nil && oldDeployID != nil {
		h.WorkerEngine.InvalidatePool(siteID, *oldDeployID)
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

	oldDeployID := site.ActiveDeployID

	// Update site to the target version in a transaction.
	if err := h.DB.Transaction(func(tx *gorm.DB) error {
		return tx.Model(&site).Updates(map[string]interface{}{
			"active_version":   version,
			"active_deploy_id": dep.ID,
			"has_worker":       dep.HasWorker,
		}).Error
	}); err != nil {
		return errorJSON(c, http.StatusInternalServerError, "failed to rollback deployment")
	}

	// Invalidate old worker pool after successful transaction.
	if h.WorkerEngine != nil && oldDeployID != nil {
		h.WorkerEngine.InvalidatePool(siteID, *oldDeployID)
	}

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

	page, _ := strconv.Atoi(c.QueryParam("page"))
	if page < 1 {
		page = 1
	}
	perPage := 20
	offset := (page - 1) * perPage

	var deployments []models.Deployment
	var total int64
	h.DB.Model(&models.Deployment{}).Where("site_id = ?", siteID).Count(&total)
	if err := h.DB.Where("site_id = ?", siteID).Order("version DESC").Offset(offset).Limit(perPage).Find(&deployments).Error; err != nil {
		return errorJSON(c, http.StatusInternalServerError, "failed to list deployments")
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"deployments": deployments,
		"total":       total,
		"page":        page,
	})
}
