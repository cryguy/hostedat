package api

import (
	"io"
	"log"
	"mime"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/coder/websocket"
	"github.com/cryguy/hostedat/internal/models"
	"github.com/cryguy/hostedat/internal/storage"
	"github.com/cryguy/hostedat/internal/workeradapter"
	"github.com/cryguy/worker/v2"
	"github.com/labstack/echo/v4"
	"github.com/minio/minio-go/v7"
	"gorm.io/gorm"
)

// WorkerDeps bundles worker-related dependencies needed by the HTTP serving
// chain to build complete worker environments.
type WorkerDeps struct {
	MinioClient   *minio.Client
	PresignClient *minio.Client
	PublicS3URL   string
	D1DataDir     string
}

// internalFiles are files that should never be served directly to visitors.
// These are configuration/runtime files similar to Cloudflare Pages behavior.
var internalFiles = map[string]bool{
	"/_worker.js":   true,
	"/_headers":     true,
	"/_redirects":   true,
	"/_routes.json": true,
}

// deniedHeaders are headers that user _headers files must not be able to set.
var deniedHeaders = map[string]bool{
	"content-length":    true,
	"transfer-encoding": true,
	"set-cookie":        true,
	"host":              true,
	// content-security-policy: allowed so sites can set their own CSP via _headers
	"strict-transport-security": true,
	"x-frame-options":           true,
	"x-content-type-options":    true,
}

func isAllowedRedirectTarget(target, domain string) bool {
	// Block empty
	if target == "" {
		return false
	}

	// Block protocol-relative URLs (//evil.com)
	if strings.HasPrefix(target, "//") {
		return false
	}

	// Allow relative paths starting with /
	if strings.HasPrefix(target, "/") {
		return true
	}

	// Block dangerous schemes
	lower := strings.ToLower(target)
	if strings.HasPrefix(lower, "javascript:") || strings.HasPrefix(lower, "data:") {
		return false
	}

	// For absolute URLs, only allow same domain and subdomains
	parsed, err := url.Parse(target)
	if err != nil {
		return false
	}
	host := parsed.Hostname()
	if host == domain || strings.HasSuffix(host, "."+domain) {
		return true
	}

	return false
}

// SubdomainRouter inspects the Host header and routes subdomain requests
// to the static site handler. Bare-domain requests pass through to the API.
func SubdomainRouter(db *gorm.DB, store *storage.Manager, cache *storage.SiteRulesCache, domain string, workerEngine *worker.Engine, s3Proxy http.Handler, workerDeps *WorkerDeps) echo.MiddlewareFunc {
	handler := staticSiteHandler(db, store, cache, domain, workerEngine, workerDeps)

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
					// Intercept storage subdomain -> S3 proxy.
					// The proxy enforces SigV4 request signing when configured.
					if subdomain == "storage" {
						if s3Proxy != nil {
							s3Proxy.ServeHTTP(c.Response().Writer, c.Request())
							return nil
						}
						return echo.ErrNotFound
					}
					c.Set("subdomain", subdomain)
					return handler(c)
				}
			}

			// For development: check *.localhost pattern
			if strings.HasSuffix(host, ".localhost") {
				subdomain := strings.TrimSuffix(host, ".localhost")
				if subdomain != "" {
					// Intercept storage.localhost → S3 proxy
					if subdomain == "storage" {
						if s3Proxy != nil {
							s3Proxy.ServeHTTP(c.Response().Writer, c.Request())
							return nil
						}
						return echo.ErrNotFound
					}
					c.Set("subdomain", subdomain)
					return handler(c)
				}
			}

			return next(c)
		}
	}
}

func staticSiteHandler(db *gorm.DB, store *storage.Manager, cache *storage.SiteRulesCache, domain string, workerEngine *worker.Engine, workerDeps *WorkerDeps) echo.HandlerFunc {
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

		if site.ActiveDeployID == nil || site.ActiveVersion == nil {
			return errorJSON(c, http.StatusNotFound, "no deployment available")
		}

		deployID := *site.ActiveDeployID

		// Block access to internal files (_worker.js, _headers, _redirects, etc.)
		reqPath := c.Request().URL.Path
		if internalFiles[reqPath] {
			return errorJSON(c, http.StatusNotFound, "not found")
		}

		// Worker intercept: if site has a worker, execute it before static pipeline
		if site.HasWorker && workerEngine != nil {
			return handleWorkerRequest(c, db, store, cache, &site, deployID, domain, workerEngine, workerDeps)
		}

		deployPath := store.GetDeploymentPath(site.ID, deployID)

		// Load/cache rules
		rules := loadRules(cache, site.ID, deployID, deployPath)

		// 1. Apply matching headers (with denylist)
		if rules != nil {
			headers := storage.MatchHeaders(rules.Headers, reqPath)
			for k, v := range headers {
				if deniedHeaders[strings.ToLower(k)] {
					continue
				}
				// Strip CRLF as defense-in-depth
				v = strings.NewReplacer("\r", "", "\n", "").Replace(v)
				c.Response().Header().Set(k, v)
			}
		}

		// 2. Check redirects (301/302 only)
		if rules != nil {
			if rule, target, matched := storage.MatchRedirect(rules.Redirects, reqPath); matched {
				if rule.StatusCode == 301 || rule.StatusCode == 302 {
					if isAllowedRedirectTarget(target, domain) {
						return c.Redirect(rule.StatusCode, target)
					}
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

func loadRules(cache *storage.SiteRulesCache, siteID string, deployID string, deployPath string) *storage.SiteRules {
	if cached, ok := cache.Get(siteID, deployID); ok {
		return cached
	}

	rules := &storage.SiteRules{}
	rules.Redirects, _ = storage.ParseRedirects(filepath.Join(deployPath, "_redirects"))
	rules.Headers, _ = storage.ParseHeaders(filepath.Join(deployPath, "_headers"))

	cache.Set(siteID, deployID, rules)
	return rules
}

func handleWorkerRequest(c echo.Context, db *gorm.DB, store *storage.Manager, cache *storage.SiteRulesCache, site *models.Site, deployID string, domain string, workerEngine *worker.Engine, workerDeps *WorkerDeps) error {
	// Build WorkerRequest from Echo context.
	req := c.Request()
	headers := make(map[string]string)
	for k, vals := range req.Header {
		if len(vals) > 0 {
			headers[strings.ToLower(k)] = vals[0]
		}
	}

	scheme := "https"
	if req.TLS == nil {
		scheme = "http"
	}
	fullURL := scheme + "://" + req.Host + req.RequestURI

	var body []byte
	maxBody := int64(workerEngine.MaxResponseBytes())
	if req.Body != nil {
		var err error
		body, err = io.ReadAll(io.LimitReader(req.Body, maxBody+1))
		if err != nil {
			return errorJSON(c, http.StatusBadRequest, "failed to read request body")
		}
		if int64(len(body)) > maxBody {
			return errorJSON(c, http.StatusRequestEntityTooLarge, "request body too large")
		}
	}

	workerReq := &worker.WorkerRequest{
		Method:  req.Method,
		URL:     fullURL,
		Headers: headers,
		Body:    body,
	}

	// Build Env: load env vars, secrets, KV bindings from DB.
	env := buildWorkerEnv(db, store, cache, site, deployID, domain, workerDeps)

	// Execute worker.
	result := workerEngine.Execute(site.ID, deployID, env, workerReq)

	// Store logs async.
	if len(result.Logs) > 0 {
		go storeWorkerLogs(db, site.ID, result.Logs)
	}

	if result.Error != nil {
		log.Printf("worker error for site %s: %v", site.ID, result.Error)
		return errorJSON(c, http.StatusInternalServerError, "worker execution failed")
	}

	// Write response.
	resp := result.Response
	if resp == nil {
		log.Printf("worker error for site %s: fetch returned nil response without error", site.ID)
		return errorJSON(c, http.StatusInternalServerError, "worker returned empty response")
	}

	// WebSocket upgrade: bridge the HTTP connection to the worker's WebSocket.
	if result.WebSocket != nil && resp.HasWebSocket && resp.StatusCode == 101 {
		conn, err := websocket.Accept(c.Response(), c.Request(), nil)
		if err != nil {
			log.Printf("worker ws upgrade error for site %s: %v", site.ID, err)
			return errorJSON(c, http.StatusInternalServerError, "websocket upgrade failed")
		}
		result.WebSocket.Bridge(c.Request().Context(), conn)
		return nil
	}

	for k, v := range resp.Headers {
		c.Response().Header().Set(k, v)
	}
	ct := c.Response().Header().Get("Content-Type")
	if ct == "" {
		ct = "text/plain"
	}
	return c.Blob(resp.StatusCode, ct, resp.Body)
}

func buildWorkerEnv(db *gorm.DB, store *storage.Manager, cache *storage.SiteRulesCache, site *models.Site, deployID string, domain string, deps *WorkerDeps) *worker.Env {
	opts := workeradapter.BuildEnvOptions{DB: db, Store: store, Cache: cache}
	if deps != nil {
		opts.MinioClient = deps.MinioClient
		opts.PresignClient = deps.PresignClient
		opts.PublicS3URL = deps.PublicS3URL
		opts.D1DataDir = deps.D1DataDir
	}
	return workeradapter.BuildEnvFromDB(opts, site.ID, &workeradapter.StaticAssetsFetcher{
		Store:     store,
		Cache:     cache,
		SiteID:    site.ID,
		DeployKey: deployID,
		SPAMode:   site.SPAMode,
		Domain:    domain,
	})
}

func storeWorkerLogs(db *gorm.DB, siteID string, logs []worker.LogEntry) {
	for _, l := range logs {
		db.Create(&models.WorkerLog{
			SiteID:    siteID,
			Level:     l.Level,
			Message:   l.Message,
			CreatedAt: l.Time,
		})
	}
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
