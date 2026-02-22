package workeradapter

import (
	"testing"
	"time"

	"github.com/cryguy/hostedat/internal/models"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupLogRetentionTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		DisableForeignKeyConstraintWhenMigrating: true,
	})
	if err != nil {
		t.Fatalf("opening test db: %v", err)
	}
	if err := db.AutoMigrate(&models.WorkerLog{}); err != nil {
		t.Fatalf("migrating: %v", err)
	}
	return db
}

// TestLogRetention_DeletesOldLogs verifies that logs older than maxDays are
// deleted while recent logs survive.
func TestLogRetention_DeletesOldLogs(t *testing.T) {
	db := setupLogRetentionTestDB(t)

	now := time.Now()
	oldTime := now.AddDate(0, 0, -10) // 10 days ago
	recentTime := now.Add(-time.Hour) // 1 hour ago

	// Insert old logs.
	db.Create(&models.WorkerLog{ID: "old1", SiteID: "site1", Level: "info", Message: "old log 1", CreatedAt: oldTime})
	db.Create(&models.WorkerLog{ID: "old2", SiteID: "site1", Level: "error", Message: "old log 2", CreatedAt: oldTime.Add(-24 * time.Hour)})

	// Insert recent logs.
	db.Create(&models.WorkerLog{ID: "new1", SiteID: "site1", Level: "info", Message: "recent log 1", CreatedAt: recentTime})
	db.Create(&models.WorkerLog{ID: "new2", SiteID: "site1", Level: "warn", Message: "recent log 2", CreatedAt: now})

	// Verify all 4 logs exist.
	var countBefore int64
	db.Model(&models.WorkerLog{}).Count(&countBefore)
	if countBefore != 4 {
		t.Fatalf("expected 4 logs before cleanup, got %d", countBefore)
	}

	// Simulate the cleanup logic directly (same as LogRetentionRunner.run ticker case).
	maxDays := 7
	cutoff := now.AddDate(0, 0, -maxDays)
	result := db.Where("created_at < ?", cutoff).Delete(&models.WorkerLog{})
	if result.Error != nil {
		t.Fatalf("cleanup error: %v", result.Error)
	}

	// Verify only recent logs survive.
	var countAfter int64
	db.Model(&models.WorkerLog{}).Count(&countAfter)
	if countAfter != 2 {
		t.Errorf("expected 2 logs after cleanup, got %d", countAfter)
	}

	// Verify the specific recent logs survived.
	var surviving []models.WorkerLog
	db.Find(&surviving)
	for _, log := range surviving {
		if log.ID != "new1" && log.ID != "new2" {
			t.Errorf("unexpected surviving log ID: %s", log.ID)
		}
	}
}

// TestLogRetention_NoLogsToDelete verifies that cleanup is a no-op when all
// logs are within the retention window.
func TestLogRetention_NoLogsToDelete(t *testing.T) {
	db := setupLogRetentionTestDB(t)

	now := time.Now()
	db.Create(&models.WorkerLog{ID: "r1", SiteID: "site1", Level: "info", Message: "recent 1", CreatedAt: now})
	db.Create(&models.WorkerLog{ID: "r2", SiteID: "site1", Level: "info", Message: "recent 2", CreatedAt: now.Add(-time.Hour)})

	maxDays := 7
	cutoff := now.AddDate(0, 0, -maxDays)
	result := db.Where("created_at < ?", cutoff).Delete(&models.WorkerLog{})
	if result.Error != nil {
		t.Fatalf("cleanup error: %v", result.Error)
	}
	if result.RowsAffected != 0 {
		t.Errorf("expected 0 rows deleted, got %d", result.RowsAffected)
	}

	var count int64
	db.Model(&models.WorkerLog{}).Count(&count)
	if count != 2 {
		t.Errorf("expected 2 logs, got %d", count)
	}
}

// TestLogRetention_AllLogsExpired verifies that all logs are deleted when all
// are beyond the retention window.
func TestLogRetention_AllLogsExpired(t *testing.T) {
	db := setupLogRetentionTestDB(t)

	old := time.Now().AddDate(0, 0, -30) // 30 days ago
	db.Create(&models.WorkerLog{ID: "e1", SiteID: "site1", Level: "info", Message: "expired 1", CreatedAt: old})
	db.Create(&models.WorkerLog{ID: "e2", SiteID: "site1", Level: "info", Message: "expired 2", CreatedAt: old.Add(-48 * time.Hour)})

	maxDays := 7
	cutoff := time.Now().AddDate(0, 0, -maxDays)
	db.Where("created_at < ?", cutoff).Delete(&models.WorkerLog{})

	var count int64
	db.Model(&models.WorkerLog{}).Count(&count)
	if count != 0 {
		t.Errorf("expected 0 logs after full cleanup, got %d", count)
	}
}

// TestLogRetentionRunner_StopsCleanly verifies that Stop() returns without
// hanging (no goroutine leak).
func TestLogRetentionRunner_StopsCleanly(t *testing.T) {
	db := setupLogRetentionTestDB(t)

	runner := NewLogRetentionRunner(db, 7)

	// Stop immediately — should return without blocking indefinitely.
	done := make(chan struct{})
	go func() {
		runner.Stop()
		close(done)
	}()

	select {
	case <-done:
		// Success: Stop returned.
	case <-time.After(5 * time.Second):
		t.Fatal("LogRetentionRunner.Stop() did not return within 5 seconds")
	}
}

// TestLogRetention_MultiSiteCleanup verifies that cleanup applies across all
// sites, not just one.
func TestLogRetention_MultiSiteCleanup(t *testing.T) {
	db := setupLogRetentionTestDB(t)

	old := time.Now().AddDate(0, 0, -10)
	recent := time.Now().Add(-time.Hour)

	// Old logs for different sites.
	db.Create(&models.WorkerLog{ID: "s1old", SiteID: "site1", Level: "info", Message: "old", CreatedAt: old})
	db.Create(&models.WorkerLog{ID: "s2old", SiteID: "site2", Level: "info", Message: "old", CreatedAt: old})

	// Recent logs for different sites.
	db.Create(&models.WorkerLog{ID: "s1new", SiteID: "site1", Level: "info", Message: "new", CreatedAt: recent})
	db.Create(&models.WorkerLog{ID: "s2new", SiteID: "site2", Level: "info", Message: "new", CreatedAt: recent})

	maxDays := 7
	cutoff := time.Now().AddDate(0, 0, -maxDays)
	db.Where("created_at < ?", cutoff).Delete(&models.WorkerLog{})

	var count int64
	db.Model(&models.WorkerLog{}).Count(&count)
	if count != 2 {
		t.Errorf("expected 2 surviving logs across sites, got %d", count)
	}
}
