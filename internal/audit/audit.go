package audit

import (
	"log"
	"time"

	"github.com/cryguy/hostedat/internal/models"
	"github.com/labstack/echo/v4"
	"gorm.io/gorm"
)

// Record logs an audit event for an authenticated handler.
// It reads the actor from the Echo context (set by AuthMiddleware).
// Errors are logged but never returned — audit must not fail user requests.
func Record(db *gorm.DB, c echo.Context, action, resourceType, resourceID string, details *string) {
	userID, _ := c.Get("user_id").(string)
	email, _ := c.Get("email").(string)
	RecordWithActor(db, c, userID, email, action, resourceType, resourceID, details)
}

// RecordWithActor logs an audit event with an explicit actor.
// Use this for pre-auth handlers (login, register) where context values
// are not yet populated.
func RecordWithActor(db *gorm.DB, c echo.Context, actorID, actorEmail, action, resourceType, resourceID string, details *string) {
	entry := models.AuditLog{
		ActorID:      actorID,
		ActorEmail:   actorEmail,
		Action:       action,
		ResourceType: resourceType,
		ResourceID:   resourceID,
		IPAddress:    c.RealIP(),
		Details:      details,
		CreatedAt:    time.Now(),
	}
	if err := db.Create(&entry).Error; err != nil {
		log.Printf("audit: failed to record %s: %v", action, err)
	}
}

// Ptr is a convenience helper to create a *string for the details field.
func Ptr(s string) *string {
	return &s
}
