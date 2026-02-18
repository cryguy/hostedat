package main

import (
	"context"
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

	"github.com/cryguy/hostedat/internal/api"
	"github.com/cryguy/hostedat/internal/certs"
	"github.com/cryguy/hostedat/internal/config"
	"github.com/cryguy/hostedat/internal/models"
	"github.com/cryguy/hostedat/internal/seaweedfs"
	"github.com/cryguy/hostedat/internal/storage"
	"github.com/cryguy/hostedat/internal/worker"
	"github.com/cryguy/hostedat/web"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	minio "github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
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
			defer mgr.Stop()
			iamClient = mgr.Client

			// Use auto-configured credentials if not explicitly set.
			if s3AccessKey == "" && s3SecretKey == "" {
				s3AccessKey = mgr.AccessKeyID
				s3SecretKey = mgr.SecretAccessKey
			}
		} else {
			iamClient = seaweedfs.NewClient(cfg.ObjectStorage.S3Endpoint)
		}
		requireSigV4 := true
		if cfg.ObjectStorage.Auth.RequireSigV4 != nil {
			requireSigV4 = *cfg.ObjectStorage.Auth.RequireSigV4
		}
		s3Proxy = api.NewS3Proxy(cfg.ObjectStorage.S3Endpoint, requireSigV4)

		// Create minio-go client for S3 bucket ops and worker bindings.
		s3Host := strings.TrimPrefix(cfg.ObjectStorage.S3Endpoint, "http://")
		s3Host = strings.TrimPrefix(s3Host, "https://")
		useSSL := strings.HasPrefix(cfg.ObjectStorage.S3Endpoint, "https://")

		var creds *credentials.Credentials
		if s3AccessKey != "" && s3SecretKey != "" {
			creds = credentials.NewStaticV4(s3AccessKey, s3SecretKey, "")
		}

		minioClient, err := minio.New(s3Host, &minio.Options{
			Secure: useSSL,
			Region: cfg.ObjectStorage.Region,
			Creds:  creds,
		})
		if err != nil {
			log.Printf("Warning: failed to create S3 client: %v", err)
		} else {
			s3Client = minioClient
			workerEngine.SetMinioClient(minioClient)
			workerEngine.SetPublicS3URL("https://storage." + cfg.Domain)

			// Wrap S3 proxy to serve public bucket objects without auth.
			if s3Proxy != nil {
				s3Proxy = api.NewPublicS3Wrapper(s3Proxy, db, minioClient)
			}
		}
	}

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
		ContentSecurityPolicy: "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'",
		ReferrerPolicy:        "strict-origin-when-cross-origin",
	}))

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

	// Global rate limiter: 20 req/s per IP
	e.Use(middleware.RateLimiter(middleware.NewRateLimiterMemoryStore(rate.Limit(20))))

	rulesCache := storage.NewSiteRulesCache()

	// Subdomain router must come before API routes
	e.Use(api.SubdomainRouter(db, store, rulesCache, cfg.Domain, workerEngine, s3Proxy))

	api.RegisterRoutes(e, db, cfg, store, version, workerEngine, s3Client, iamClient, cfg.ObjectStorage.Region)

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

	// Graceful shutdown on SIGINT/SIGTERM.
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)

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

		go func() {
			if err := server.ListenAndServeTLS("", ""); err != nil && err != http.ErrServerClosed {
				log.Fatalf("Server failed: %v", err)
			}
		}()

		<-quit
		log.Println("Shutting down server...")
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := server.Shutdown(ctx); err != nil {
			log.Fatalf("Server forced to shutdown: %v", err)
		}
	} else {
		log.Printf("Starting hostedat %s (%s) (no TLS) on %s for domain %s", version, commit, cfg.Listen, cfg.Domain)

		go func() {
			if err := e.Start(cfg.Listen); err != nil && err != http.ErrServerClosed {
				log.Fatalf("Server failed: %v", err)
			}
		}()

		<-quit
		log.Println("Shutting down server...")
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := e.Shutdown(ctx); err != nil {
			log.Fatalf("Server forced to shutdown: %v", err)
		}
	}

	log.Println("Server exited cleanly")
}
