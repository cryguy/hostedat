package main

import (
	"flag"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"

	"github.com/cryguy/hostedat/internal/api"
	"github.com/cryguy/hostedat/internal/certs"
	"github.com/cryguy/hostedat/internal/config"
	"github.com/cryguy/hostedat/internal/models"
	"github.com/cryguy/hostedat/internal/storage"
	"github.com/cryguy/hostedat/web"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
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

	e := echo.New()
	e.HideBanner = true
	e.HTTPErrorHandler = api.CustomErrorHandler

	e.Use(middleware.Logger())
	e.Use(middleware.Recover())
	e.Use(middleware.CORSWithConfig(middleware.CORSConfig{
		AllowOrigins: []string{"*"},
		AllowMethods: []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowHeaders: []string{"Authorization", "Content-Type"},
	}))

	store := storage.NewManager(cfg.StoragePath)
	rulesCache := storage.NewSiteRulesCache()

	// Subdomain router must come before API routes
	e.Use(api.SubdomainRouter(db, store, rulesCache, cfg.Domain))

	api.RegisterRoutes(e, db, cfg, store)

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
		log.Printf("Starting server with TLS on %s for domain %s", cfg.Listen, cfg.Domain)

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
		log.Printf("Starting server (no TLS) on %s for domain %s", cfg.Listen, cfg.Domain)
		if err := e.Start(cfg.Listen); err != nil {
			log.Fatalf("Server failed: %v", err)
		}
	}
}
