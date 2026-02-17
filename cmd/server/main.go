package main

import (
	"flag"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/cryguy/hostedat/internal/api"
	"github.com/cryguy/hostedat/internal/certs"
	"github.com/cryguy/hostedat/internal/config"
	"github.com/cryguy/hostedat/internal/models"
	"github.com/cryguy/hostedat/internal/storage"
	"github.com/cryguy/hostedat/internal/worker"
	"github.com/cryguy/hostedat/web"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"golang.org/x/time/rate"
)

var (
	version = "dev"
	commit  = "unknown"
)

func main() {
	configPath := flag.String("config", "config.yaml", "path to config file")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Ensure data directory exists for SQLite
	if cfg.Database.Driver == "sqlite" {
		dir := filepath.Dir(cfg.Database.DSN)
		if err := os.MkdirAll(dir, 0755); err != nil {
			log.Fatalf("Failed to create data directory: %v", err)
		}
	}

	// Ensure storage directory exists
	if err := os.MkdirAll(cfg.StoragePath, 0755); err != nil {
		log.Fatalf("Failed to create storage directory: %v", err)
	}

	db, err := models.InitDB(cfg.Database)
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}

	if err := models.SeedDefaults(db, cfg); err != nil {
		log.Fatalf("Failed to seed defaults: %v", err)
	}

	// Create worker engine
	workerEngine := worker.NewEngine(cfg.Worker, db)
	defer workerEngine.Shutdown()

	store := storage.NewManager(cfg.StoragePath)
	workerEngine.SetStore(store)

	// Start cron runner
	cronRunner := worker.NewCronRunner(db, workerEngine)
	defer cronRunner.Shutdown()

	e := echo.New()
	e.HideBanner = true
	e.HTTPErrorHandler = api.CustomErrorHandler

	e.Use(middleware.Logger())
	e.Use(middleware.Recover())

	// Security headers
	e.Use(middleware.SecureWithConfig(middleware.SecureConfig{
		XSSProtection:         "1; mode=block",
		ContentTypeNosniff:    "nosniff",
		XFrameOptions:         "DENY",
		HSTSMaxAge:            31536000,
		ContentSecurityPolicy: "default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'",
		ReferrerPolicy:        "strict-origin-when-cross-origin",
	}))

	// CORS — only allow configured domain, its subdomains, and localhost for dev
	e.Use(middleware.CORSWithConfig(middleware.CORSConfig{
		AllowOriginFunc: func(origin string) (bool, error) {
			origin = strings.ToLower(origin)
			// Allow localhost for development
			if strings.HasPrefix(origin, "http://localhost") || strings.HasPrefix(origin, "http://127.0.0.1") {
				return true, nil
			}
			// Allow the configured domain and its subdomains
			allowed := strings.ToLower(cfg.Domain)
			if strings.HasSuffix(origin, "://"+allowed) || strings.HasSuffix(origin, "."+allowed) {
				return true, nil
			}
			return false, nil
		},
		AllowMethods: []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowHeaders: []string{"Authorization", "Content-Type", "X-Hostedat-Version"},
	}))

	// Global rate limiter: 20 req/s per IP
	e.Use(middleware.RateLimiter(middleware.NewRateLimiterMemoryStore(rate.Limit(20))))

	rulesCache := storage.NewSiteRulesCache()

	// Subdomain router must come before API routes
	e.Use(api.SubdomainRouter(db, store, rulesCache, cfg.Domain, workerEngine))

	api.RegisterRoutes(e, db, cfg, store, version, workerEngine)

	// Serve embedded frontend (SPA fallback)
	distFS, err := fs.Sub(web.DistFS, "dist")
	if err != nil {
		log.Fatalf("Failed to load embedded frontend: %v", err)
	}
	httpFS := http.FS(distFS)
	fileServer := http.FileServer(httpFS)
	e.GET("/*", echo.WrapHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Try serving the exact file first
		path := r.URL.Path
		if f, err := httpFS.Open(path); err == nil {
			f.Close()
			fileServer.ServeHTTP(w, r)
			return
		}
		// SPA fallback: serve index.html for any unmatched route
		r.URL.Path = "/"
		fileServer.ServeHTTP(w, r)
	})))

	// Start with TLS if Cloudflare token is configured, otherwise plain HTTP
	if cfg.Cloudflare.APIToken != "" {
		log.Printf("Starting hostedat %s (%s) with TLS on %s for domain %s", version, commit, cfg.Listen, cfg.Domain)

		certsDir := filepath.Join(filepath.Dir(cfg.Database.DSN), "certs")
		tlsCfg, err := certs.SetupTLS(certs.Config{
			Domain:   cfg.Domain,
			APIToken: cfg.Cloudflare.APIToken,
			DataDir:  certsDir,
		})
		if err != nil {
			log.Fatalf("Failed to setup TLS: %v", err)
		}

		server := &http.Server{
			Addr:      cfg.Listen,
			Handler:   e,
			TLSConfig: tlsCfg,
		}

		if err := server.ListenAndServeTLS("", ""); err != nil {
			log.Fatalf("Server failed: %v", err)
		}
	} else {
		log.Printf("Starting hostedat %s (%s) (no TLS) on %s for domain %s", version, commit, cfg.Listen, cfg.Domain)
		if err := e.Start(cfg.Listen); err != nil {
			log.Fatalf("Server failed: %v", err)
		}
	}
}
