package api

import (
	"net/http"
	"strconv"
	"time"

	"github.com/cryguy/hostedat/internal/models"
	"github.com/labstack/echo/v4"
	"gorm.io/gorm"
)

// AuditHandler serves the audit log listing endpoint.
type AuditHandler struct {
	DB *gorm.DB
}

// List returns paginated audit log entries.
// Admins see all entries; regular users see only their own.
func (h *AuditHandler) List(c echo.Context) error {
	userID, _, role := GetUserFromContext(c)

	page, _ := strconv.Atoi(c.QueryParam("page"))
	if page < 1 {
		page = 1
	}
	perPage := 50
	offset := (page - 1) * perPage

	query := h.DB.Model(&models.AuditLog{})

	// Scope: regular users can only see their own logs.
	if role != "admin" && role != "superadmin" {
		query = query.Where("actor_id = ?", userID)
	}

	// Optional filters
	if actorID := c.QueryParam("actor_id"); actorID != "" {
		query = query.Where("actor_id = ?", actorID)
	}
	if action := c.QueryParam("action"); action != "" {
		query = query.Where("action = ?", action)
	}
	if resourceType := c.QueryParam("resource_type"); resourceType != "" {
		query = query.Where("resource_type = ?", resourceType)
	}
	if resourceID := c.QueryParam("resource_id"); resourceID != "" {
		query = query.Where("resource_id = ?", resourceID)
	}
	if from := c.QueryParam("from"); from != "" {
		if t, err := time.Parse(time.RFC3339, from); err == nil {
			query = query.Where("created_at >= ?", t)
		}
	}
	if to := c.QueryParam("to"); to != "" {
		if t, err := time.Parse(time.RFC3339, to); err == nil {
			query = query.Where("created_at <= ?", t)
		}
	}

	var total int64
	query.Count(&total)

	var logs []models.AuditLog
	if err := query.Order("created_at DESC").Offset(offset).Limit(perPage).Find(&logs).Error; err != nil {
		return errorJSON(c, http.StatusInternalServerError, "failed to list audit logs")
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"items": logs,
		"total": total,
		"page":  page,
	})
}
