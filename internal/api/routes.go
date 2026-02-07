package api

import (
	"github.com/cryguy/hostedat/internal/config"
	"github.com/labstack/echo/v4"
	"gorm.io/gorm"
)

func RegisterRoutes(e *echo.Echo, db *gorm.DB, cfg *config.Config) {
	api := e.Group("/api/v1")

	authHandler := &AuthHandler{DB: db, JWTSecret: cfg.JWTSecret}

	// Public auth routes
	authGroup := api.Group("/auth")
	authGroup.POST("/register", authHandler.Register)
	authGroup.POST("/login", authHandler.Login)
	authGroup.POST("/logout", authHandler.Logout)

	// Protected routes (added in later steps)
	_ = api.Group("", AuthMiddleware(db, cfg.JWTSecret))
}
