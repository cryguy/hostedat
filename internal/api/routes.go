package api

import (
	"net/http"

	"github.com/cryguy/hostedat/internal/config"
	"github.com/cryguy/hostedat/internal/seaweedfs"
	"github.com/cryguy/hostedat/internal/storage"
	"github.com/cryguy/worker/v2"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/minio/minio-go/v7"
	"golang.org/x/time/rate"
	"gorm.io/gorm"
)

func RegisterRoutes(e *echo.Echo, db *gorm.DB, cfg *config.Config, store *storage.Manager, serverVersion string, workerEngine *worker.Engine, s3Client *minio.Client, iamClient *seaweedfs.Client, region string, analyticsDB *gorm.DB) {
	// API-wide rate limit (default 20 req/s per IP, separate from global; 0 = disabled)
	var apiMiddlewares []echo.MiddlewareFunc
	if cfg.RateLimit.API > 0 {
		apiMiddlewares = append(apiMiddlewares, middleware.RateLimiter(middleware.NewRateLimiterMemoryStore(rate.Limit(cfg.RateLimit.API))))
	}
	api := e.Group("/api/v1", apiMiddlewares...)

	// Public version endpoint
	api.GET("/version", func(c echo.Context) error {
		return c.JSON(http.StatusOK, map[string]string{
			"version":         serverVersion,
			"min_cli_version": cfg.MinCLIVersion,
		})
	})

	authHandler := &AuthHandler{DB: db, JWTSecret: cfg.JWTSecret}
	siteHandler := &SiteHandler{DB: db, Storage: store, S3Client: s3Client, IAMClient: iamClient, DataDir: cfg.Worker.DataDir}
	deployHandler := &DeployHandler{DB: db, Storage: store, MaxScriptSizeKB: cfg.Worker.MaxScriptSizeKB}
	if workerEngine != nil {
		deployHandler.WorkerEngine = workerEngine
	}
	apiKeyHandler := &APIKeyHandler{DB: db}
	adminHandler := &AdminHandler{DB: db, Storage: store, S3Client: s3Client, IAMClient: iamClient}
	workerHandler := &WorkerHandler{DB: db, DataDir: cfg.Worker.DataDir}

	// Public auth routes — stricter rate limit (default 5 req/s per IP; 0 = disabled)
	// Auth is Bearer-token only (JWT/API key), never cookies, so CSRF is not applicable.
	var authMiddlewares []echo.MiddlewareFunc
	if cfg.RateLimit.Auth > 0 {
		authMiddlewares = append(authMiddlewares, middleware.RateLimiter(middleware.NewRateLimiterMemoryStore(rate.Limit(cfg.RateLimit.Auth))))
	}
	authGroup := api.Group("/auth", authMiddlewares...)
	authGroup.GET("/registration", authHandler.RegistrationInfo)
	authGroup.POST("/register", authHandler.Register)
	authGroup.POST("/login", authHandler.Login)
	authGroup.POST("/logout", authHandler.Logout)
	authGroup.GET("/cli", authHandler.CLILogin)
	authGroup.POST("/cli", authHandler.CLILoginSubmit)
	authGroup.POST("/token", authHandler.TokenExchange)

	// Version check middleware for authenticated routes
	versionCheck := VersionCheckMiddleware(cfg.MinCLIVersion)

	// Protected routes
	protected := api.Group("", AuthMiddleware(db, cfg.JWTSecret), versionCheck)

	// Site routes
	sites := protected.Group("/sites")
	sites.POST("", siteHandler.Create)
	sites.GET("", siteHandler.List)
	sites.GET("/:id", siteHandler.Get)
	sites.PATCH("/:id", siteHandler.Update)
	sites.DELETE("/:id", siteHandler.Delete)

	// Deployment routes — stricter rate limit for uploads (default 2 req/s per IP; 0 = disabled)
	if cfg.RateLimit.Deploy > 0 {
		deployLimiter := middleware.RateLimiter(middleware.NewRateLimiterMemoryStore(rate.Limit(cfg.RateLimit.Deploy)))
		sites.POST("/:id/deploy", deployHandler.Deploy, deployLimiter)
	} else {
		sites.POST("/:id/deploy", deployHandler.Deploy)
	}
	sites.GET("/:id/deployments", deployHandler.List)
	sites.POST("/:id/deployments/:version/rollback", deployHandler.Rollback)

	// Worker routes
	sites.POST("/:id/worker/env", workerHandler.SetEnvVar)
	sites.GET("/:id/worker/env", workerHandler.ListEnvVars)
	sites.DELETE("/:id/worker/env/:varId", workerHandler.DeleteEnvVar)
	sites.POST("/:id/worker/kv", workerHandler.CreateKVNamespace)
	sites.GET("/:id/worker/kv", workerHandler.ListKVNamespaces)
	sites.DELETE("/:id/worker/kv/:nsId", workerHandler.DeleteKVNamespace)
	sites.POST("/:id/worker/d1", workerHandler.CreateD1Database)
	sites.GET("/:id/worker/d1", workerHandler.ListD1Databases)
	sites.DELETE("/:id/worker/d1/:d1Id", workerHandler.DeleteD1Database)
	sites.POST("/:id/worker/do", workerHandler.CreateDurableObjectNamespace)
	sites.GET("/:id/worker/do", workerHandler.ListDurableObjectNamespaces)
	sites.DELETE("/:id/worker/do/:doId", workerHandler.DeleteDurableObjectNamespace)
	sites.POST("/:id/worker/crons", workerHandler.CreateCronSchedule)
	sites.GET("/:id/worker/crons", workerHandler.ListCronSchedules)
	sites.DELETE("/:id/worker/crons/:cronId", workerHandler.DeleteCronSchedule)
	sites.GET("/:id/worker/logs", workerHandler.GetLogs)

	// Analytics routes (only registered when analytics is enabled)
	if analyticsDB != nil {
		ah := &AnalyticsHandler{DB: db, AnalyticsDB: analyticsDB}
		sites.GET("/:id/analytics/summary", ah.GetSummary)
		sites.GET("/:id/analytics/timeseries", ah.GetTimeseries)
		sites.GET("/:id/analytics/pages", ah.GetPages)
		sites.GET("/:id/analytics/referrers", ah.GetReferrers)
	}

	// Audit log routes
	auditHandler := &AuditHandler{DB: db}
	protected.GET("/audit-logs", auditHandler.List)

	// API key routes
	keys := protected.Group("/keys")
	keys.POST("", apiKeyHandler.Create)
	keys.GET("", apiKeyHandler.List)
	keys.DELETE("/:id", apiKeyHandler.Delete)

	// Storage routes (object storage buckets per site, S3 credentials per user)
	if s3Client != nil && iamClient != nil {
		storageHandler := &StorageHandler{DB: db, S3Client: s3Client, IAMClient: iamClient, Region: region, PublicS3URL: "https://" + cfg.ObjectStorage.DomainName}
		sites.POST("/:id/storage/buckets", storageHandler.CreateBucket)
		sites.GET("/:id/storage/buckets", storageHandler.ListBuckets)
		sites.PATCH("/:id/storage/buckets/:bucketId", storageHandler.UpdateBucket)
		sites.DELETE("/:id/storage/buckets/:bucketId", storageHandler.DeleteBucket)
		sites.POST("/:id/storage/buckets/:bucketId/upload-url", storageHandler.UploadURL)

		s3creds := protected.Group("/s3-credentials")
		s3creds.POST("", storageHandler.CreateS3Credential)
		s3creds.GET("", storageHandler.ListS3Credentials)
		s3creds.DELETE("/:id", storageHandler.DeleteS3Credential)
	}

	// Admin routes
	admin := protected.Group("/admin", RequireAdmin())
	admin.GET("/users", adminHandler.ListUsers)
	admin.PATCH("/users/:id/role", adminHandler.UpdateUserRole)
	admin.DELETE("/users/:id", adminHandler.DeleteUser)
	admin.GET("/settings", adminHandler.GetSettings)
	admin.PATCH("/settings", adminHandler.UpdateSettings)
	admin.POST("/invites", adminHandler.CreateInvite)
	admin.GET("/invites", adminHandler.ListInvites)
	admin.DELETE("/invites/:id", adminHandler.RevokeInvite)
}
