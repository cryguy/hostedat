package main

import (
	"context"
	"errors"
	"flag"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/cryguy/hostedat/internal/analytics"
	"github.com/cryguy/hostedat/internal/api"
	"github.com/cryguy/hostedat/internal/audit"
	"github.com/cryguy/hostedat/internal/certs"
	"github.com/cryguy/hostedat/internal/config"
	"github.com/cryguy/hostedat/internal/models"
	"github.com/cryguy/hostedat/internal/seaweedfs"
	"github.com/cryguy/hostedat/internal/storage"
	"github.com/cryguy/hostedat/internal/workeradapter"
	"github.com/cryguy/hostedat/web"
	"github.com/cryguy/worker/v2"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"golang.org/x/time/rate"
	"gorm.io/gorm"
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

	store := storage.NewManager(cfg.StoragePath)

	// Analytics (opt-in via config)
	var analyticsCollector *analytics.Collector
	var analyticsDB *gorm.DB
	if cfg.Analytics.Enabled {
		analyticsDir := filepath.Dir(cfg.Analytics.DSN)
		if err := os.MkdirAll(analyticsDir, 0755); err != nil {
			log.Fatalf("Failed to create analytics data directory: %v", err)
		}
		aDB, err := analytics.InitDB(cfg.Analytics.DSN)
		if err != nil {
			log.Fatalf("Failed to initialize analytics database: %v", err)
		}
		analyticsDB = aDB
		defer func() {
			if sqlDB, err := analyticsDB.DB(); err == nil {
				_ = sqlDB.Close()
			}
		}()

		analyticsCollector = analytics.NewCollector(analyticsDB)
		defer analyticsCollector.Stop()

		rollupRunner := analytics.NewRollupRunner(analyticsDB)
		defer rollupRunner.Stop()

		retentionRunner := analytics.NewRetentionRunner(analyticsDB, cfg.Analytics.RawRetentionDays, cfg.Analytics.RollupRetentionDays)
		defer retentionRunner.Stop()

		log.Printf("Analytics enabled (DSN: %s, raw retention: %dd, rollup retention: %dd)",
			cfg.Analytics.DSN, cfg.Analytics.RawRetentionDays, cfg.Analytics.RollupRetentionDays)
	}

	// Create worker engine
	workerEngine := worker.NewEngine(worker.EngineConfig{
		PoolSize:         cfg.Worker.PoolSize,
		MemoryLimitMB:    cfg.Worker.MemoryLimitMB,
		ExecutionTimeout: cfg.Worker.ExecutionTimeout,
		MaxFetchRequests: cfg.Worker.MaxFetchRequests,
		FetchTimeoutSec:  cfg.Worker.FetchTimeoutSec,
		MaxResponseBytes: cfg.Worker.MaxResponseBytes,
		MaxScriptSizeKB:  cfg.Worker.MaxScriptSizeKB,
	}, store)
	defer workerEngine.Shutdown()

	// One-time migration: backfill ActiveDeployID and rename deploy directories.
	if err := storage.MigrateDeployPaths(db, store); err != nil {
		log.Printf("Warning: deploy path migration failed: %v", err)
	}

	// Object storage (SeaweedFS)
	var s3Client *minio.Client
	var iamClient *seaweedfs.Client
	var s3Proxy http.Handler
	if cfg.ObjectStorage.Enabled {
		// Resolve S3 admin credentials: config > managed auto-generated.
		s3AccessKey := cfg.ObjectStorage.Auth.AccessKeyID
		s3SecretKey := cfg.ObjectStorage.Auth.SecretAccessKey

		if cfg.ObjectStorage.Managed {
			mgr := seaweedfs.NewManager(cfg.ObjectStorage)
			if err := mgr.Start(); err != nil {
				log.Fatalf("Failed to start SeaweedFS: %v", err)
			}
			defer func() { _ = mgr.Stop() }()
			iamClient = mgr.Client

			// Use auto-configured credentials if not explicitly set.
			if s3AccessKey == "" && s3SecretKey == "" {
				s3AccessKey = mgr.AccessKeyID
				s3SecretKey = mgr.SecretAccessKey
			}
		} else {
			iamClient = seaweedfs.NewClient(cfg.ObjectStorage.S3Endpoint)
		}
		s3Proxy = api.NewS3Proxy(cfg.ObjectStorage.S3Endpoint)

		// Single minio client via the public S3 endpoint (domain_name).
		// SeaweedFS is started with -s3.domainName so it accepts SigV4
		// signatures for this host, making presigned URLs work without
		// a separate presign client.
		var creds *credentials.Credentials
		if s3AccessKey != "" && s3SecretKey != "" {
			creds = credentials.NewStaticV4(s3AccessKey, s3SecretKey, "")
		}

		minioClient, err := minio.New(cfg.ObjectStorage.DomainName, &minio.Options{
			Secure: true,
			Region: cfg.ObjectStorage.Region,
			Creds:  creds,
		})
		if err != nil {
			log.Printf("Warning: failed to create S3 client: %v", err)
		} else {
			s3Client = minioClient
		}
	}

	rulesCache := storage.NewSiteRulesCache()

	// Build env factory for cron and service bindings.
	envFactory := func(siteID string) *worker.Env {
		return workeradapter.BuildEnvFromDB(workeradapter.BuildEnvOptions{
			DB:          db,
			MinioClient: s3Client,
			PublicS3URL: "https://" + cfg.ObjectStorage.DomainName,
			D1DataDir:   cfg.Worker.DataDir,
			Store:       store,
			Cache:       rulesCache,
		}, siteID, nil)
	}

	// Start cron runner
	cronRunner := workeradapter.NewCronRunner(db, workerEngine, envFactory)
	defer cronRunner.Shutdown()

	// Start log retention runner
	logRetention := workeradapter.NewLogRetentionRunner(db, cfg.Worker.MaxLogRetention)
	defer logRetention.Stop()

	// Start audit log retention runner
	auditRetention := audit.NewRetentionRunner(db, cfg.AuditLog.RetentionDays)
	defer auditRetention.Stop()

	e := echo.New()
	e.HideBanner = true
	e.HTTPErrorHandler = api.CustomErrorHandler

	e.Use(middleware.RequestLogger())
	e.Use(middleware.Recover())

	// Security headers (CSP applied separately — only for admin UI, not hosted sites)
	e.Use(middleware.SecureWithConfig(middleware.SecureConfig{
		XSSProtection:      "1; mode=block",
		ContentTypeNosniff: "nosniff",
		XFrameOptions:      "DENY",
		HSTSMaxAge:         31536000,
		ReferrerPolicy:     "strict-origin-when-cross-origin",
	}))

	// CSP only for API/admin routes (bare domain), not subdomain-hosted sites
	e.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			host := c.Request().Host
			if idx := strings.LastIndex(host, ":"); idx != -1 {
				host = host[:idx]
			}
			if host == cfg.Domain || host == "localhost" || host == "127.0.0.1" {
				c.Response().Header().Set("Content-Security-Policy",
					"default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline' https://fonts.googleapis.com; font-src 'self' https://fonts.gstatic.com")
			}
			return next(c)
		}
	})

	// CORS — only allow configured domain, its subdomains, and localhost for dev
	e.Use(middleware.CORSWithConfig(middleware.CORSConfig{
		AllowOriginFunc: func(origin string) (bool, error) {
			origin = strings.ToLower(origin)
			// Allow localhost for development (exact host match with optional port).
			if strings.HasPrefix(origin, "http://localhost:") || origin == "http://localhost" ||
				strings.HasPrefix(origin, "http://127.0.0.1:") || origin == "http://127.0.0.1" {
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

	// Global rate limiter (configurable, default 100 req/s per IP; 0 = disabled)
	if cfg.RateLimit.Global > 0 {
		e.Use(middleware.RateLimiter(middleware.NewRateLimiterMemoryStore(rate.Limit(cfg.RateLimit.Global))))
	}

	workerDeps := &api.WorkerDeps{
		MinioClient: s3Client,
		PublicS3URL: "https://" + cfg.ObjectStorage.DomainName,
		D1DataDir:   cfg.Worker.DataDir,
	}

	// Subdomain router must come before API routes
	e.Use(api.SubdomainRouter(db, store, rulesCache, cfg.Domain, workerEngine, s3Proxy, workerDeps, analyticsCollector))

	api.RegisterRoutes(e, db, cfg, store, version, workerEngine, s3Client, iamClient, cfg.ObjectStorage.Region, analyticsDB)

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
			_ = f.Close()
			fileServer.ServeHTTP(w, r)
			return
		}
		// SPA fallback: serve index.html for any unmatched route
		r.URL.Path = "/"
		fileServer.ServeHTTP(w, r)
	})))

	// Graceful shutdown on SIGINT/SIGTERM.
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)

	// Start with TLS if Cloudflare token is configured, otherwise plain HTTP
	var shutdownServer func(ctx context.Context) error

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

		go func() {
			if err := server.ListenAndServeTLS("", ""); err != nil && !errors.Is(err, http.ErrServerClosed) {
				log.Fatalf("Server failed: %v", err)
			}
		}()

		shutdownServer = server.Shutdown
	} else {
		log.Printf("Starting hostedat %s (%s) (no TLS) on %s for domain %s", version, commit, cfg.Listen, cfg.Domain)

		go func() {
			if err := e.Start(cfg.Listen); err != nil && !errors.Is(err, http.ErrServerClosed) {
				log.Fatalf("Server failed: %v", err)
			}
		}()

		shutdownServer = e.Shutdown
	}

	// Migrate public bucket policies now that the S3 reverse proxy is reachable.
	if s3Client != nil {
		go api.MigratePublicBucketPolicies(db, s3Client)
	}

	<-quit
	log.Println("Shutting down server...")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := shutdownServer(ctx); err != nil {
		log.Fatalf("Server forced to shutdown: %v", err)
	}

	log.Println("Server exited cleanly")
}
