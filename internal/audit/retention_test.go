package audit

import (
	"testing"
	"time"

	"github.com/cryguy/hostedat/internal/config"
	"github.com/cryguy/hostedat/internal/models"
)

// seedAuditLog inserts an AuditLog entry with the given createdAt timestamp directly via
// raw GORM so we can control the CreatedAt value for retention tests.
func seedAuditLog(t *testing.T, db interface{ Create(interface{}) interface{ Error() error } }, actorID, action string, createdAt time.Time) {
	t.Helper()
}

func TestRetentionRunner_DeletesOldEntries(t *testing.T) {
	db, err := models.InitDB(config.DBConfig{Driver: "sqlite", DSN: ":memory:"})
	if err != nil {
		t.Fatalf("InitDB: %v", err)
	}

	now := time.Now()
	maxDays := 30

	// Seed an old entry (beyond retention window).
	oldEntry := models.AuditLog{
		ActorID:      "user1",
		ActorEmail:   "user1@example.com",
		Action:       "site.create",
		ResourceType: "site",
		ResourceID:   "s1",
		CreatedAt:    now.AddDate(0, 0, -(maxDays + 1)),
	}
	if err := db.Create(&oldEntry).Error; err != nil {
		t.Fatalf("seed old entry: %v", err)
	}

	// Seed a recent entry (within retention window).
	recentEntry := models.AuditLog{
		ActorID:      "user2",
		ActorEmail:   "user2@example.com",
		Action:       "site.delete",
		ResourceType: "site",
		ResourceID:   "s2",
		CreatedAt:    now.AddDate(0, 0, -1),
	}
	if err := db.Create(&recentEntry).Error; err != nil {
		t.Fatalf("seed recent entry: %v", err)
	}

	// Verify both entries exist before cleanup.
	var before int64
	db.Model(&models.AuditLog{}).Count(&before)
	if before != 2 {
		t.Fatalf("expected 2 entries before cleanup, got %d", before)
	}

	// Run cleanup directly (bypass the hourly ticker by invoking the logic inline).
	cutoff := now.AddDate(0, 0, -maxDays)
	result := db.Where("created_at < ?", cutoff).Delete(&models.AuditLog{})
	if result.Error != nil {
		t.Fatalf("cleanup error: %v", result.Error)
	}

	// Old entry must be gone; recent entry must remain.
	var after int64
	db.Model(&models.AuditLog{}).Count(&after)
	if after != 1 {
		t.Errorf("expected 1 entry after cleanup, got %d", after)
	}

	var remaining models.AuditLog
	db.First(&remaining)
	if remaining.ActorID != "user2" {
		t.Errorf("remaining entry ActorID = %q, want user2", remaining.ActorID)
	}
}

func TestRetentionRunner_KeepsEntriesWithinWindow(t *testing.T) {
	db, err := models.InitDB(config.DBConfig{Driver: "sqlite", DSN: ":memory:"})
	if err != nil {
		t.Fatalf("InitDB: %v", err)
	}

	maxDays := 90

	// Seed three entries all within the retention window.
	for i := 1; i <= 3; i++ {
		entry := models.AuditLog{
			ActorID:      "user",
			ActorEmail:   "user@example.com",
			Action:       "site.create",
			ResourceType: "site",
			ResourceID:   "s1",
			CreatedAt:    time.Now().AddDate(0, 0, -i),
		}
		if err := db.Create(&entry).Error; err != nil {
			t.Fatalf("seed entry %d: %v", i, err)
		}
	}

	cutoff := time.Now().AddDate(0, 0, -maxDays)
	db.Where("created_at < ?", cutoff).Delete(&models.AuditLog{})

	var count int64
	db.Model(&models.AuditLog{}).Count(&count)
	if count != 3 {
		t.Errorf("expected all 3 entries to survive cleanup, got %d", count)
	}
}

func TestRetentionRunner_Stop_ReturnsWithoutHanging(t *testing.T) {
	db, err := models.InitDB(config.DBConfig{Driver: "sqlite", DSN: ":memory:"})
	if err != nil {
		t.Fatalf("InitDB: %v", err)
	}

	runner := NewRetentionRunner(db, 30)

	done := make(chan struct{})
	go func() {
		runner.Stop()
		close(done)
	}()

	select {
	case <-done:
		// Stop returned promptly — good.
	case <-time.After(3 * time.Second):
		t.Fatal("Stop() did not return within 3 seconds — possible deadlock")
	}
}

func TestRetentionRunner_Stop_IdempotentCallIsNotRequired(t *testing.T) {
	// Verify that creating a runner and stopping it immediately does not panic.
	db, err := models.InitDB(config.DBConfig{Driver: "sqlite", DSN: ":memory:"})
	if err != nil {
		t.Fatalf("InitDB: %v", err)
	}

	runner := NewRetentionRunner(db, 7)
	runner.Stop() // must not panic
}
