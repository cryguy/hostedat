package api

import (
	"crypto/sha256"
	"encoding/hex"
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

			// Check if the token has been explicitly revoked (e.g. via logout).
			if IsTokenRevoked(db, token) {
				return errorJSON(c, http.StatusUnauthorized, "token has been revoked")
			}

			// Reload user from DB to reflect current role/status.
			// This catches deleted users and role changes before token expiry.
			var user models.User
			if err := db.First(&user, "id = ?", claims.UserID).Error; err != nil {
				return errorJSON(c, http.StatusUnauthorized, "user not found")
			}

			c.Set(contextKeyUserID, user.ID)
			c.Set(contextKeyEmail, user.Email)
			c.Set(contextKeyRole, user.Role)
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

// RequireSiteOwner checks that the authenticated user owns the site identified
// by the :id path parameter, or has an admin/superadmin role.
func RequireSiteOwner(db *gorm.DB) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			userID, _, role := GetUserFromContext(c)
			siteID := c.Param("id")
			if siteID == "" {
				return next(c)
			}

			var site models.Site
			if err := db.First(&site, "id = ?", siteID).Error; err != nil {
				return errorJSON(c, http.StatusNotFound, "site not found")
			}

			if site.UserID != userID && role != "admin" && role != "superadmin" {
				return errorJSON(c, http.StatusForbidden, "access denied")
			}

			// Store for downstream handlers to avoid re-fetching.
			c.Set("site", &site)
			return next(c)
		}
	}
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

// HashToken returns the SHA-256 hex digest of a raw JWT string.
func HashToken(token string) string {
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:])
}

// RevokeToken adds a token to the revocation table. The expiresAt should
// match the token's natural expiry so the entry can be cleaned up later.
func RevokeToken(db *gorm.DB, token string, expiresAt time.Time) error {
	return db.Create(&models.RevokedToken{
		TokenHash: HashToken(token),
		ExpiresAt: expiresAt,
	}).Error
}

// IsTokenRevoked checks whether a token has been explicitly revoked.
func IsTokenRevoked(db *gorm.DB, token string) bool {
	var count int64
	db.Model(&models.RevokedToken{}).Where("token_hash = ?", HashToken(token)).Count(&count)
	return count > 0
}

// CleanExpiredTokens removes revocation entries whose JWTs have naturally
// expired. Called periodically to keep the table small.
func CleanExpiredTokens(db *gorm.DB) {
	db.Where("expires_at < ?", time.Now()).Delete(&models.RevokedToken{})
}
