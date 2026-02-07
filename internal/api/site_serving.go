package api

import (
	"mime"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/cryguy/hostedat/internal/models"
	"github.com/cryguy/hostedat/internal/storage"
	"github.com/labstack/echo/v4"
	"gorm.io/gorm"
)

// SubdomainRouter inspects the Host header and routes subdomain requests
// to the static site handler. Bare-domain requests pass through to the API.
func SubdomainRouter(db *gorm.DB, store *storage.Manager, cache *storage.SiteRulesCache, domain string) echo.MiddlewareFunc {
	handler := staticSiteHandler(db, store, cache, domain)

	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			host := c.Request().Host
			// Strip port if present
			if idx := strings.LastIndex(host, ":"); idx != -1 {
				host = host[:idx]
			}

			// Bare domain or localhost → pass through to API
			if host == domain || host == "localhost" || host == "127.0.0.1" {
				return next(c)
			}

			// Check if it's a subdomain of our domain
			suffix := "." + domain
			if strings.HasSuffix(host, suffix) {
				subdomain := strings.TrimSuffix(host, suffix)
				if subdomain != "" && !strings.Contains(subdomain, ".") {
					c.Set("subdomain", subdomain)
					return handler(c)
				}
			}

			// For development: check *.localhost pattern
			if strings.HasSuffix(host, ".localhost") {
				subdomain := strings.TrimSuffix(host, ".localhost")
				if subdomain != "" {
					c.Set("subdomain", subdomain)
					return handler(c)
				}
			}

			return next(c)
		}
	}
}

func staticSiteHandler(db *gorm.DB, store *storage.Manager, cache *storage.SiteRulesCache, domain string) echo.HandlerFunc {
	return func(c echo.Context) error {
		subdomain, _ := c.Get("subdomain").(string)
		if subdomain == "" {
			return errorJSON(c, http.StatusNotFound, "site not found")
		}

		// Look up site
		var site models.Site
		if err := db.Where("subdomain_slug = ?", subdomain).First(&site).Error; err != nil {
			return errorJSON(c, http.StatusNotFound, "site not found")
		}

		if site.ActiveVersion == nil {
			return errorJSON(c, http.StatusNotFound, "no deployment available")
		}

		version := *site.ActiveVersion
		deployPath := store.GetDeploymentPath(site.ID, version)
		reqPath := c.Request().URL.Path

		// Load/cache rules
		rules := loadRules(store, cache, site.ID, version, deployPath)

		// 1. Apply matching headers
		if rules != nil {
			headers := storage.MatchHeaders(rules.Headers, reqPath)
			for k, v := range headers {
				c.Response().Header().Set(k, v)
			}
		}

		// 2. Check redirects (301/302 only)
		if rules != nil {
			if rule, target, matched := storage.MatchRedirect(rules.Redirects, reqPath); matched {
				if rule.StatusCode == 301 || rule.StatusCode == 302 {
					return c.Redirect(rule.StatusCode, target)
				}
			}
		}

		// 3. Try static file
		if filePath, found := store.ResolveFile(deployPath, reqPath); found {
			return serveFile(c, filePath)
		}

		// 4. Check rewrite rules (200)
		if rules != nil {
			if rule, target, matched := storage.MatchRedirect(rules.Redirects, reqPath); matched {
				if rule.StatusCode == 200 {
					if filePath, found := store.ResolveFile(deployPath, target); found {
						return serveFile(c, filePath)
					}
				}
			}
		}

		// 5. SPA mode fallback
		if site.SPAMode {
			if filePath, found := store.ResolveFile(deployPath, "/index.html"); found {
				return serveFile(c, filePath)
			}
		}

		// 6. Custom 404.html
		if filePath, found := store.ResolveFile(deployPath, "/404.html"); found {
			c.Response().WriteHeader(http.StatusNotFound)
			return serveFile(c, filePath)
		}

		return errorJSON(c, http.StatusNotFound, "not found")
	}
}

func loadRules(store *storage.Manager, cache *storage.SiteRulesCache, siteID string, version int, deployPath string) *storage.SiteRules {
	if cached, ok := cache.Get(siteID, version); ok {
		return cached
	}

	rules := &storage.SiteRules{}
	rules.Redirects, _ = storage.ParseRedirects(filepath.Join(deployPath, "_redirects"))
	rules.Headers, _ = storage.ParseHeaders(filepath.Join(deployPath, "_headers"))

	cache.Set(siteID, version, rules)
	return rules
}

func serveFile(c echo.Context, filePath string) error {
	ext := filepath.Ext(filePath)
	contentType := mime.TypeByExtension(ext)
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	c.Response().Header().Set("Content-Type", contentType)
	return c.File(filePath)
}
