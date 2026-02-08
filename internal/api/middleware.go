package api

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/cryguy/hostedat/internal/auth"
	"github.com/cryguy/hostedat/internal/config"
	"github.com/cryguy/hostedat/internal/models"
	"github.com/labstack/echo/v4"
	"gorm.io/gorm"
)

const (
	contextKeyUserID = "user_id"
	contextKeyEmail  = "email"
	contextKeyRole   = "role"
)

func GetUserFromContext(c echo.Context) (userID, email, role string) {
	userID, _ = c.Get(contextKeyUserID).(string)
	email, _ = c.Get(contextKeyEmail).(string)
	role, _ = c.Get(contextKeyRole).(string)
	return
}

func AuthMiddleware(db *gorm.DB, jwtSecret string) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			header := c.Request().Header.Get("Authorization")
			if header == "" {
				return errorJSON(c, http.StatusUnauthorized, "missing authorization header")
			}

			token := strings.TrimPrefix(header, "Bearer ")
			if token == header {
				return errorJSON(c, http.StatusUnauthorized, "invalid authorization format")
			}

			// API key auth
			if strings.HasPrefix(token, "hd_") {
				hash := auth.HashAPIKey(token)
				var key models.APIKey
				if err := db.Where("key_hash = ?", hash).First(&key).Error; err != nil {
					return errorJSON(c, http.StatusUnauthorized, "invalid API key")
				}

				// Update last used
				now := time.Now()
				db.Model(&key).Update("last_used_at", &now)

				var user models.User
				if err := db.First(&user, "id = ?", key.UserID).Error; err != nil {
					return errorJSON(c, http.StatusUnauthorized, "user not found")
				}

				c.Set(contextKeyUserID, user.ID)
				c.Set(contextKeyEmail, user.Email)
				c.Set(contextKeyRole, user.Role)
				return next(c)
			}

			// JWT auth
			claims, err := auth.ValidateToken(token, jwtSecret)
			if err != nil {
				return errorJSON(c, http.StatusUnauthorized, "invalid or expired token")
			}

			c.Set(contextKeyUserID, claims.UserID)
			c.Set(contextKeyEmail, claims.Email)
			c.Set(contextKeyRole, claims.Role)
			return next(c)
		}
	}
}

func RequireRole(roles ...string) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			_, _, role := GetUserFromContext(c)
			for _, r := range roles {
				if role == r {
					return next(c)
				}
			}
			return errorJSON(c, http.StatusForbidden, "insufficient permissions")
		}
	}
}

func RequireAdmin() echo.MiddlewareFunc {
	return RequireRole("superadmin", "admin")
}

// VersionCheckMiddleware rejects requests from CLI clients whose version
// is below the configured minimum. Skips if no minimum is set, if the
// header is absent (browser/curl), or if the version is unparseable (dev builds).
func VersionCheckMiddleware(minVersion string) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			if minVersion == "" {
				return next(c)
			}
			clientVersion := c.Request().Header.Get("X-Hostedat-Version")
			if clientVersion == "" {
				return next(c)
			}
			if !config.SemverAtLeast(clientVersion, minVersion) {
				return errorJSON(c, http.StatusUpgradeRequired,
					fmt.Sprintf("CLI version %s is too old, minimum required: %s", clientVersion, minVersion))
			}
			return next(c)
		}
	}
}
