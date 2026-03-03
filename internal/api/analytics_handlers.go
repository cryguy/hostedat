package api

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/cryguy/hostedat/internal/analytics"
	"github.com/cryguy/hostedat/internal/models"
	"github.com/labstack/echo/v4"
	"gorm.io/gorm"
)

// errResponseWritten is a sentinel indicating the HTTP response was already
// sent (via errorJSON). Callers should return nil to Echo since the response
// is complete.
var errResponseWritten = errors.New("response already written")

// AnalyticsHandler serves analytics data for sites.
type AnalyticsHandler struct {
	DB          *gorm.DB // main database (for site ownership checks)
	AnalyticsDB *gorm.DB // analytics database
}

// getSiteAndCheckOwnership looks up a site and verifies the caller owns it
// (or is admin/superadmin). Same authorization pattern as all other site handlers.
// Returns errResponseWritten when an error response has already been sent.
func (h *AnalyticsHandler) getSiteAndCheckOwnership(c echo.Context) (*models.Site, error) {
	siteID := c.Param("id")
	userID := c.Get("user_id").(string)
	role, _ := c.Get("role").(string)

	var site models.Site
	if err := h.DB.First(&site, "id = ?", siteID).Error; err != nil {
		_ = errorJSON(c, http.StatusNotFound, "site not found")
		return nil, errResponseWritten
	}

	if site.UserID != userID && role != "admin" && role != "superadmin" {
		_ = errorJSON(c, http.StatusForbidden, "forbidden")
		return nil, errResponseWritten
	}

	return &site, nil
}

// GetSummary returns aggregate analytics totals for a site.
// GET /sites/:id/analytics/summary?period=24h|7d|30d
func (h *AnalyticsHandler) GetSummary(c echo.Context) error {
	site, err := h.getSiteAndCheckOwnership(c)
	if err != nil {
		return nil
	}

	pf := analytics.ParsePeriod(c.QueryParam("period"))
	result, err := analytics.GetSummary(h.AnalyticsDB, site.ID, pf)
	if err != nil {
		return errorJSON(c, http.StatusInternalServerError, "failed to query analytics")
	}

	return c.JSON(http.StatusOK, result)
}

// GetTimeseries returns time-bucketed analytics data for charting.
// GET /sites/:id/analytics/timeseries?period=24h|7d|30d
func (h *AnalyticsHandler) GetTimeseries(c echo.Context) error {
	site, err := h.getSiteAndCheckOwnership(c)
	if err != nil {
		return nil
	}

	pf := analytics.ParsePeriod(c.QueryParam("period"))
	// Allow explicit bucket override.
	if b := c.QueryParam("bucket"); b == "hour" || b == "day" {
		pf.Bucket = b
	}

	points, err := analytics.GetTimeseries(h.AnalyticsDB, site.ID, pf)
	if err != nil {
		return errorJSON(c, http.StatusInternalServerError, "failed to query analytics")
	}

	return c.JSON(http.StatusOK, points)
}

// GetPages returns top pages for a site.
// GET /sites/:id/analytics/pages?period=24h|7d|30d&limit=10
func (h *AnalyticsHandler) GetPages(c echo.Context) error {
	site, err := h.getSiteAndCheckOwnership(c)
	if err != nil {
		return nil
	}

	pf := analytics.ParsePeriod(c.QueryParam("period"))
	limit := 10
	if l, err := strconv.Atoi(c.QueryParam("limit")); err == nil && l > 0 && l <= 100 {
		limit = l
	}

	entries, err := analytics.GetTopPages(h.AnalyticsDB, site.ID, pf, limit)
	if err != nil {
		return errorJSON(c, http.StatusInternalServerError, "failed to query analytics")
	}

	return c.JSON(http.StatusOK, entries)
}

// GetReferrers returns top referrers for a site.
// GET /sites/:id/analytics/referrers?period=24h|7d|30d&limit=10
func (h *AnalyticsHandler) GetReferrers(c echo.Context) error {
	site, err := h.getSiteAndCheckOwnership(c)
	if err != nil {
		return nil
	}

	pf := analytics.ParsePeriod(c.QueryParam("period"))
	limit := 10
	if l, err := strconv.Atoi(c.QueryParam("limit")); err == nil && l > 0 && l <= 100 {
		limit = l
	}

	entries, err := analytics.GetTopReferrers(h.AnalyticsDB, site.ID, pf, limit)
	if err != nil {
		return errorJSON(c, http.StatusInternalServerError, "failed to query analytics")
	}

	return c.JSON(http.StatusOK, entries)
}
