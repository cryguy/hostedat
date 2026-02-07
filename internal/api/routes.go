package api

import (
	"github.com/cryguy/hostedat/internal/config"
	"github.com/cryguy/hostedat/internal/storage"
	"github.com/labstack/echo/v4"
	"gorm.io/gorm"
)

func RegisterRoutes(e *echo.Echo, db *gorm.DB, cfg *config.Config, store *storage.Manager) {
	api := e.Group("/api/v1")

	authHandler := &AuthHandler{DB: db, JWTSecret: cfg.JWTSecret}
	siteHandler := &SiteHandler{DB: db, Storage: store}
	deployHandler := &DeployHandler{DB: db, Storage: store}
	apiKeyHandler := &APIKeyHandler{DB: db}
	adminHandler := &AdminHandler{DB: db, Storage: store}

	// Public auth routes
	authGroup := api.Group("/auth")
	authGroup.POST("/register", authHandler.Register)
	authGroup.POST("/login", authHandler.Login)
	authGroup.POST("/logout", authHandler.Logout)

	// Protected routes
	protected := api.Group("", AuthMiddleware(db, cfg.JWTSecret))

	// Site routes
	sites := protected.Group("/sites")
	sites.POST("", siteHandler.Create)
	sites.GET("", siteHandler.List)
	sites.GET("/:id", siteHandler.Get)
	sites.PATCH("/:id", siteHandler.Update)
	sites.DELETE("/:id", siteHandler.Delete)

	// Deployment routes
	sites.POST("/:id/deploy", deployHandler.Deploy)
	sites.GET("/:id/deployments", deployHandler.List)

	// API key routes
	keys := protected.Group("/keys")
	keys.POST("", apiKeyHandler.Create)
	keys.GET("", apiKeyHandler.List)
	keys.DELETE("/:id", apiKeyHandler.Delete)

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
