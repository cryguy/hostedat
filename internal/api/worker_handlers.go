package api

import (
	"errors"
	"net/http"

	"github.com/cryguy/hostedat/internal/models"
	"github.com/cryguy/hostedat/internal/worker"
	"github.com/labstack/echo/v4"
	"gorm.io/gorm"
)

type WorkerHandler struct {
	DB *gorm.DB
}

// Env Vars

func (h *WorkerHandler) SetEnvVar(c echo.Context) error {
	userID, _, role := GetUserFromContext(c)
	siteID := c.Param("id")

	var site models.Site
	if err := h.DB.First(&site, "id = ?", siteID).Error; err != nil {
		return errorJSON(c, http.StatusNotFound, "site not found")
	}

	if site.UserID != userID && role != "admin" && role != "superadmin" {
		return errorJSON(c, http.StatusForbidden, "access denied")
	}

	var req struct {
		Name   string `json:"name"`
		Value  string `json:"value"`
		Secret bool   `json:"secret"`
	}
	if err := c.Bind(&req); err != nil {
		return errorJSON(c, http.StatusBadRequest, "invalid request")
	}

	if req.Name == "" {
		return errorJSON(c, http.StatusBadRequest, "name is required")
	}

	// Upsert: update if exists, create if not
	var envVar models.WorkerEnvVar
	result := h.DB.Where("site_id = ? AND name = ?", siteID, req.Name).First(&envVar)
	if errors.Is(result.Error, gorm.ErrRecordNotFound) {
		// Create new
		envVar = models.WorkerEnvVar{
			SiteID: siteID,
			Name:   req.Name,
			Value:  req.Value,
			Secret: req.Secret,
		}
		if err := h.DB.Create(&envVar).Error; err != nil {
			return errorJSON(c, http.StatusInternalServerError, "failed to create env var")
		}
	} else if result.Error != nil {
		return errorJSON(c, http.StatusInternalServerError, "database error")
	}

	// Update existing
	envVar.Value = req.Value
	envVar.Secret = req.Secret
	if err := h.DB.Save(&envVar).Error; err != nil {
		return errorJSON(c, http.StatusInternalServerError, "failed to update env var")
	}

	return c.JSON(http.StatusOK, envVar)
}

func (h *WorkerHandler) ListEnvVars(c echo.Context) error {
	userID, _, role := GetUserFromContext(c)
	siteID := c.Param("id")

	var site models.Site
	if err := h.DB.First(&site, "id = ?", siteID).Error; err != nil {
		return errorJSON(c, http.StatusNotFound, "site not found")
	}

	if site.UserID != userID && role != "admin" && role != "superadmin" {
		return errorJSON(c, http.StatusForbidden, "access denied")
	}

	var envVars []models.WorkerEnvVar
	if err := h.DB.Where("site_id = ?", siteID).Find(&envVars).Error; err != nil {
		return errorJSON(c, http.StatusInternalServerError, "failed to list env vars")
	}

	// Mask secret values
	for i := range envVars {
		if envVars[i].Secret {
			envVars[i].Value = "********"
		}
	}

	return c.JSON(http.StatusOK, envVars)
}

func (h *WorkerHandler) DeleteEnvVar(c echo.Context) error {
	userID, _, role := GetUserFromContext(c)
	siteID := c.Param("id")
	varID := c.Param("varId")

	var site models.Site
	if err := h.DB.First(&site, "id = ?", siteID).Error; err != nil {
		return errorJSON(c, http.StatusNotFound, "site not found")
	}

	if site.UserID != userID && role != "admin" && role != "superadmin" {
		return errorJSON(c, http.StatusForbidden, "access denied")
	}

	var envVar models.WorkerEnvVar
	if err := h.DB.First(&envVar, "id = ? AND site_id = ?", varID, siteID).Error; err != nil {
		return errorJSON(c, http.StatusNotFound, "env var not found")
	}

	if err := h.DB.Delete(&envVar).Error; err != nil {
		return errorJSON(c, http.StatusInternalServerError, "failed to delete env var")
	}

	return c.NoContent(http.StatusNoContent)
}

// KV Namespaces

func (h *WorkerHandler) CreateKVNamespace(c echo.Context) error {
	userID, _, role := GetUserFromContext(c)
	siteID := c.Param("id")

	var site models.Site
	if err := h.DB.First(&site, "id = ?", siteID).Error; err != nil {
		return errorJSON(c, http.StatusNotFound, "site not found")
	}

	if site.UserID != userID && role != "admin" && role != "superadmin" {
		return errorJSON(c, http.StatusForbidden, "access denied")
	}

	var req struct {
		Name string `json:"name"`
	}
	if err := c.Bind(&req); err != nil {
		return errorJSON(c, http.StatusBadRequest, "invalid request")
	}

	if req.Name == "" {
		return errorJSON(c, http.StatusBadRequest, "name is required")
	}

	namespace := models.KVNamespace{
		SiteID: siteID,
		Name:   req.Name,
	}
	if err := h.DB.Create(&namespace).Error; err != nil {
		return errorJSON(c, http.StatusInternalServerError, "failed to create namespace")
	}

	return c.JSON(http.StatusCreated, namespace)
}

func (h *WorkerHandler) ListKVNamespaces(c echo.Context) error {
	userID, _, role := GetUserFromContext(c)
	siteID := c.Param("id")

	var site models.Site
	if err := h.DB.First(&site, "id = ?", siteID).Error; err != nil {
		return errorJSON(c, http.StatusNotFound, "site not found")
	}

	if site.UserID != userID && role != "admin" && role != "superadmin" {
		return errorJSON(c, http.StatusForbidden, "access denied")
	}

	var namespaces []models.KVNamespace
	if err := h.DB.Where("site_id = ?", siteID).Find(&namespaces).Error; err != nil {
		return errorJSON(c, http.StatusInternalServerError, "failed to list namespaces")
	}

	return c.JSON(http.StatusOK, namespaces)
}

func (h *WorkerHandler) DeleteKVNamespace(c echo.Context) error {
	userID, _, role := GetUserFromContext(c)
	siteID := c.Param("id")
	nsID := c.Param("nsId")

	var site models.Site
	if err := h.DB.First(&site, "id = ?", siteID).Error; err != nil {
		return errorJSON(c, http.StatusNotFound, "site not found")
	}

	if site.UserID != userID && role != "admin" && role != "superadmin" {
		return errorJSON(c, http.StatusForbidden, "access denied")
	}

	var namespace models.KVNamespace
	if err := h.DB.First(&namespace, "id = ? AND site_id = ?", nsID, siteID).Error; err != nil {
		return errorJSON(c, http.StatusNotFound, "namespace not found")
	}

	// Delete all KV entries for this namespace
	if err := h.DB.Where("namespace_id = ?", nsID).Delete(&models.KVEntry{}).Error; err != nil {
		return errorJSON(c, http.StatusInternalServerError, "failed to delete namespace entries")
	}

	if err := h.DB.Delete(&namespace).Error; err != nil {
		return errorJSON(c, http.StatusInternalServerError, "failed to delete namespace")
	}

	return c.NoContent(http.StatusNoContent)
}

// Cron Schedules

func (h *WorkerHandler) CreateCronSchedule(c echo.Context) error {
	userID, _, role := GetUserFromContext(c)
	siteID := c.Param("id")

	var site models.Site
	if err := h.DB.First(&site, "id = ?", siteID).Error; err != nil {
		return errorJSON(c, http.StatusNotFound, "site not found")
	}

	if site.UserID != userID && role != "admin" && role != "superadmin" {
		return errorJSON(c, http.StatusForbidden, "access denied")
	}

	var req struct {
		Cron    string `json:"cron"`
		Enabled *bool  `json:"enabled"`
	}
	if err := c.Bind(&req); err != nil {
		return errorJSON(c, http.StatusBadRequest, "invalid request")
	}

	if req.Cron == "" {
		return errorJSON(c, http.StatusBadRequest, "cron is required")
	}

	if err := worker.ValidateCron(req.Cron); err != nil {
		return errorJSON(c, http.StatusBadRequest, "invalid cron expression: "+err.Error())
	}

	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}

	schedule := models.CronSchedule{
		SiteID:  siteID,
		Cron:    req.Cron,
		Enabled: enabled,
	}
	if err := h.DB.Create(&schedule).Error; err != nil {
		return errorJSON(c, http.StatusInternalServerError, "failed to create cron schedule")
	}

	return c.JSON(http.StatusCreated, schedule)
}

func (h *WorkerHandler) ListCronSchedules(c echo.Context) error {
	userID, _, role := GetUserFromContext(c)
	siteID := c.Param("id")

	var site models.Site
	if err := h.DB.First(&site, "id = ?", siteID).Error; err != nil {
		return errorJSON(c, http.StatusNotFound, "site not found")
	}

	if site.UserID != userID && role != "admin" && role != "superadmin" {
		return errorJSON(c, http.StatusForbidden, "access denied")
	}

	var schedules []models.CronSchedule
	if err := h.DB.Where("site_id = ?", siteID).Find(&schedules).Error; err != nil {
		return errorJSON(c, http.StatusInternalServerError, "failed to list cron schedules")
	}

	return c.JSON(http.StatusOK, schedules)
}

func (h *WorkerHandler) DeleteCronSchedule(c echo.Context) error {
	userID, _, role := GetUserFromContext(c)
	siteID := c.Param("id")
	cronID := c.Param("cronId")

	var site models.Site
	if err := h.DB.First(&site, "id = ?", siteID).Error; err != nil {
		return errorJSON(c, http.StatusNotFound, "site not found")
	}

	if site.UserID != userID && role != "admin" && role != "superadmin" {
		return errorJSON(c, http.StatusForbidden, "access denied")
	}

	var schedule models.CronSchedule
	if err := h.DB.First(&schedule, "id = ? AND site_id = ?", cronID, siteID).Error; err != nil {
		return errorJSON(c, http.StatusNotFound, "cron schedule not found")
	}

	if err := h.DB.Delete(&schedule).Error; err != nil {
		return errorJSON(c, http.StatusInternalServerError, "failed to delete cron schedule")
	}

	return c.NoContent(http.StatusNoContent)
}

// Worker Logs

func (h *WorkerHandler) GetLogs(c echo.Context) error {
	userID, _, role := GetUserFromContext(c)
	siteID := c.Param("id")

	var site models.Site
	if err := h.DB.First(&site, "id = ?", siteID).Error; err != nil {
		return errorJSON(c, http.StatusNotFound, "site not found")
	}

	if site.UserID != userID && role != "admin" && role != "superadmin" {
		return errorJSON(c, http.StatusForbidden, "access denied")
	}

	var logs []models.WorkerLog
	if err := h.DB.Where("site_id = ?", siteID).Order("created_at DESC").Limit(100).Find(&logs).Error; err != nil {
		return errorJSON(c, http.StatusInternalServerError, "failed to get logs")
	}

	return c.JSON(http.StatusOK, logs)
}

// D1 Databases

func (h *WorkerHandler) CreateD1Database(c echo.Context) error {
	userID, _, role := GetUserFromContext(c)
	siteID := c.Param("id")

	var site models.Site
	if err := h.DB.First(&site, "id = ?", siteID).Error; err != nil {
		return errorJSON(c, http.StatusNotFound, "site not found")
	}

	if site.UserID != userID && role != "admin" && role != "superadmin" {
		return errorJSON(c, http.StatusForbidden, "access denied")
	}

	var req struct {
		Name string `json:"name"`
	}
	if err := c.Bind(&req); err != nil {
		return errorJSON(c, http.StatusBadRequest, "invalid request")
	}

	if req.Name == "" {
		return errorJSON(c, http.StatusBadRequest, "name is required")
	}

	database := models.D1Database{
		SiteID: siteID,
		Name:   req.Name,
	}
	if err := h.DB.Create(&database).Error; err != nil {
		return errorJSON(c, http.StatusInternalServerError, "failed to create database")
	}

	return c.JSON(http.StatusCreated, database)
}

func (h *WorkerHandler) ListD1Databases(c echo.Context) error {
	userID, _, role := GetUserFromContext(c)
	siteID := c.Param("id")

	var site models.Site
	if err := h.DB.First(&site, "id = ?", siteID).Error; err != nil {
		return errorJSON(c, http.StatusNotFound, "site not found")
	}

	if site.UserID != userID && role != "admin" && role != "superadmin" {
		return errorJSON(c, http.StatusForbidden, "access denied")
	}

	var databases []models.D1Database
	if err := h.DB.Where("site_id = ?", siteID).Find(&databases).Error; err != nil {
		return errorJSON(c, http.StatusInternalServerError, "failed to list databases")
	}

	var total int64
	h.DB.Model(&models.D1Database{}).Where("site_id = ?", siteID).Count(&total)

	return c.JSON(http.StatusOK, map[string]interface{}{
		"items": databases,
		"total": total,
	})
}

func (h *WorkerHandler) DeleteD1Database(c echo.Context) error {
	userID, _, role := GetUserFromContext(c)
	siteID := c.Param("id")
	d1ID := c.Param("d1Id")

	var site models.Site
	if err := h.DB.First(&site, "id = ?", siteID).Error; err != nil {
		return errorJSON(c, http.StatusNotFound, "site not found")
	}

	if site.UserID != userID && role != "admin" && role != "superadmin" {
		return errorJSON(c, http.StatusForbidden, "access denied")
	}

	var database models.D1Database
	if err := h.DB.First(&database, "id = ? AND site_id = ?", d1ID, siteID).Error; err != nil {
		return errorJSON(c, http.StatusNotFound, "database not found")
	}

	if err := h.DB.Delete(&database).Error; err != nil {
		return errorJSON(c, http.StatusInternalServerError, "failed to delete database")
	}

	// Remove the SQLite file from disk (best-effort)
	dataDir := worker.GetDataDir()
	dbPath := worker.GetD1Path(dataDir, database.DatabaseID)
	_ = worker.DeleteFile(dbPath)

	return c.NoContent(http.StatusNoContent)
}

// Durable Objects

func (h *WorkerHandler) CreateDurableObjectNamespace(c echo.Context) error {
	userID, _, role := GetUserFromContext(c)
	siteID := c.Param("id")

	var site models.Site
	if err := h.DB.First(&site, "id = ?", siteID).Error; err != nil {
		return errorJSON(c, http.StatusNotFound, "site not found")
	}

	if site.UserID != userID && role != "admin" && role != "superadmin" {
		return errorJSON(c, http.StatusForbidden, "access denied")
	}

	var req struct {
		Name string `json:"name"`
	}
	if err := c.Bind(&req); err != nil {
		return errorJSON(c, http.StatusBadRequest, "invalid request")
	}

	if req.Name == "" {
		return errorJSON(c, http.StatusBadRequest, "name is required")
	}

	namespace := models.DurableObjectNamespace{
		SiteID: siteID,
		Name:   req.Name,
	}
	if err := h.DB.Create(&namespace).Error; err != nil {
		return errorJSON(c, http.StatusInternalServerError, "failed to create namespace")
	}

	return c.JSON(http.StatusCreated, namespace)
}

func (h *WorkerHandler) ListDurableObjectNamespaces(c echo.Context) error {
	userID, _, role := GetUserFromContext(c)
	siteID := c.Param("id")

	var site models.Site
	if err := h.DB.First(&site, "id = ?", siteID).Error; err != nil {
		return errorJSON(c, http.StatusNotFound, "site not found")
	}

	if site.UserID != userID && role != "admin" && role != "superadmin" {
		return errorJSON(c, http.StatusForbidden, "access denied")
	}

	var namespaces []models.DurableObjectNamespace
	if err := h.DB.Where("site_id = ?", siteID).Find(&namespaces).Error; err != nil {
		return errorJSON(c, http.StatusInternalServerError, "failed to list namespaces")
	}

	var total int64
	h.DB.Model(&models.DurableObjectNamespace{}).Where("site_id = ?", siteID).Count(&total)

	return c.JSON(http.StatusOK, map[string]interface{}{
		"items": namespaces,
		"total": total,
	})
}

func (h *WorkerHandler) DeleteDurableObjectNamespace(c echo.Context) error {
	userID, _, role := GetUserFromContext(c)
	siteID := c.Param("id")
	doID := c.Param("doId")

	var site models.Site
	if err := h.DB.First(&site, "id = ?", siteID).Error; err != nil {
		return errorJSON(c, http.StatusNotFound, "site not found")
	}

	if site.UserID != userID && role != "admin" && role != "superadmin" {
		return errorJSON(c, http.StatusForbidden, "access denied")
	}

	var namespace models.DurableObjectNamespace
	if err := h.DB.First(&namespace, "id = ? AND site_id = ?", doID, siteID).Error; err != nil {
		return errorJSON(c, http.StatusNotFound, "namespace not found")
	}

	// Delete all DurableObjectEntry records for this namespace
	if err := h.DB.Where("namespace = ?", namespace.NamespaceID).Delete(&models.DurableObjectEntry{}).Error; err != nil {
		return errorJSON(c, http.StatusInternalServerError, "failed to delete namespace entries")
	}

	if err := h.DB.Delete(&namespace).Error; err != nil {
		return errorJSON(c, http.StatusInternalServerError, "failed to delete namespace")
	}

	return c.NoContent(http.StatusNoContent)
}
