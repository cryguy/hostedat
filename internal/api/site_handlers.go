package api

import (
	"net/http"
	"regexp"
	"strings"

	gonanoid "github.com/matoous/go-nanoid/v2"

	"github.com/cryguy/hostedat/internal/models"
	"github.com/cryguy/hostedat/internal/storage"
	"github.com/labstack/echo/v4"
	"gorm.io/gorm"
)

var slugRegex = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{1,61}[a-z0-9]$`)

var reservedSlugs = map[string]bool{
	"www": true, "api": true, "admin": true, "mail": true,
	"smtp": true, "ftp": true, "ns1": true, "ns2": true,
	"app": true, "dashboard": true, "status": true,
}

type SiteHandler struct {
	DB      *gorm.DB
	Storage *storage.Manager
}

type createSiteRequest struct {
	Name          string `json:"name"`
	SubdomainSlug string `json:"subdomain_slug,omitempty"`
}

type updateSiteRequest struct {
	Name    *string `json:"name,omitempty"`
	SPAMode *bool   `json:"spa_mode,omitempty"`
}

func (h *SiteHandler) Create(c echo.Context) error {
	userID, _, _ := GetUserFromContext(c)

	var req createSiteRequest
	if err := c.Bind(&req); err != nil {
		return errorJSON(c, http.StatusBadRequest, "invalid request body")
	}

	if req.Name == "" {
		return errorJSON(c, http.StatusBadRequest, "name is required")
	}

	slug := req.SubdomainSlug
	if slug == "" {
		// Auto-generate from name
		slug = strings.ToLower(strings.ReplaceAll(req.Name, " ", "-"))
		slug = regexp.MustCompile(`[^a-z0-9-]`).ReplaceAllString(slug, "")
		if len(slug) < 3 {
			suffix, _ := gonanoid.Generate("0123456789abcdefghijklmnopqrstuvwxyz", 6)
			slug = slug + suffix
		}
	}

	slug = strings.ToLower(slug)
	if !slugRegex.MatchString(slug) {
		return errorJSON(c, http.StatusBadRequest, "subdomain slug must be 3-63 chars, lowercase alphanumeric and hyphens")
	}
	if reservedSlugs[slug] {
		return errorJSON(c, http.StatusBadRequest, "subdomain slug is reserved")
	}

	// Check uniqueness
	var existing models.Site
	if err := h.DB.Where("subdomain_slug = ?", slug).First(&existing).Error; err == nil {
		return errorJSON(c, http.StatusConflict, "subdomain slug already taken")
	}

	site := models.Site{
		UserID:        userID,
		SubdomainSlug: slug,
		Name:          req.Name,
	}

	if err := h.DB.Create(&site).Error; err != nil {
		return errorJSON(c, http.StatusInternalServerError, "failed to create site")
	}

	return c.JSON(http.StatusCreated, site)
}

func (h *SiteHandler) List(c echo.Context) error {
	userID, _, role := GetUserFromContext(c)

	var sites []models.Site
	query := h.DB

	if c.QueryParam("all") == "true" && (role == "admin" || role == "superadmin") {
		// Admin: list all sites
	} else {
		query = query.Where("user_id = ?", userID)
	}

	if err := query.Order("created_at DESC").Find(&sites).Error; err != nil {
		return errorJSON(c, http.StatusInternalServerError, "failed to list sites")
	}

	return c.JSON(http.StatusOK, sites)
}

func (h *SiteHandler) Get(c echo.Context) error {
	userID, _, role := GetUserFromContext(c)
	siteID := c.Param("id")

	var site models.Site
	if err := h.DB.First(&site, "id = ?", siteID).Error; err != nil {
		return errorJSON(c, http.StatusNotFound, "site not found")
	}

	if site.UserID != userID && role != "admin" && role != "superadmin" {
		return errorJSON(c, http.StatusForbidden, "access denied")
	}

	return c.JSON(http.StatusOK, site)
}

func (h *SiteHandler) Update(c echo.Context) error {
	userID, _, role := GetUserFromContext(c)
	siteID := c.Param("id")

	var site models.Site
	if err := h.DB.First(&site, "id = ?", siteID).Error; err != nil {
		return errorJSON(c, http.StatusNotFound, "site not found")
	}

	if site.UserID != userID && role != "admin" && role != "superadmin" {
		return errorJSON(c, http.StatusForbidden, "access denied")
	}

	var req updateSiteRequest
	if err := c.Bind(&req); err != nil {
		return errorJSON(c, http.StatusBadRequest, "invalid request body")
	}

	if req.Name != nil {
		site.Name = *req.Name
	}
	if req.SPAMode != nil {
		site.SPAMode = *req.SPAMode
	}

	if err := h.DB.Save(&site).Error; err != nil {
		return errorJSON(c, http.StatusInternalServerError, "failed to update site")
	}

	return c.JSON(http.StatusOK, site)
}

func (h *SiteHandler) Delete(c echo.Context) error {
	userID, _, role := GetUserFromContext(c)
	siteID := c.Param("id")

	var site models.Site
	if err := h.DB.First(&site, "id = ?", siteID).Error; err != nil {
		return errorJSON(c, http.StatusNotFound, "site not found")
	}

	if site.UserID != userID && role != "admin" && role != "superadmin" {
		return errorJSON(c, http.StatusForbidden, "access denied")
	}

	// Delete deployments
	h.DB.Where("site_id = ?", siteID).Delete(&models.Deployment{})

	// Delete site record
	if err := h.DB.Delete(&site).Error; err != nil {
		return errorJSON(c, http.StatusInternalServerError, "failed to delete site")
	}

	// Delete storage
	_ = h.Storage.DeleteSite(siteID)

	return c.JSON(http.StatusOK, map[string]string{"message": "site deleted"})
}
