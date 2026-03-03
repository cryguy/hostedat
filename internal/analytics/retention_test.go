package analytics

import (
	"testing"
	"time"
)

func TestRetention_PrunesOldRawLogs(t *testing.T) {
	db := setupTestDB(t)

	now := time.Now()
	old := now.AddDate(0, 0, -31)
	recent := now.AddDate(0, 0, -1)

	db.Create(&RequestLog{SiteID: "s1", Timestamp: old, Method: "GET", Path: "/old", Status: 200, ClientIP: "1.1.1.1"})
	db.Create(&RequestLog{SiteID: "s1", Timestamp: recent, Method: "GET", Path: "/recent", Status: 200, ClientIP: "1.1.1.2"})

	r := &RetentionRunner{db: db, rawDays: 30, rollDays: 365}
	r.prune()

	var count int64
	db.Model(&RequestLog{}).Count(&count)
	if count != 1 {
		t.Fatalf("expected 1 remaining log, got %d", count)
	}

	var log RequestLog
	db.First(&log)
	if log.Path != "/recent" {
		t.Errorf("expected /recent to survive, got %s", log.Path)
	}
}

func TestRetention_PrunesOldRollups(t *testing.T) {
	db := setupTestDB(t)

	now := time.Now()
	old := now.AddDate(-2, 0, 0) // 2 years ago
	recent := now.AddDate(0, -1, 0)

	db.Create(&HourlyStat{SiteID: "s1", Bucket: old, Requests: 100})
	db.Create(&HourlyStat{SiteID: "s1", Bucket: recent, Requests: 50})

	r := &RetentionRunner{db: db, rawDays: 30, rollDays: 365}
	r.prune()

	var count int64
	db.Model(&HourlyStat{}).Count(&count)
	if count != 1 {
		t.Fatalf("expected 1 remaining rollup, got %d", count)
	}

	var stat HourlyStat
	db.First(&stat)
	if stat.Requests != 50 {
		t.Errorf("expected recent stat with 50 requests, got %d", stat.Requests)
	}
}

func TestRetention_KeepsRecentData(t *testing.T) {
	db := setupTestDB(t)

	now := time.Now()
	db.Create(&RequestLog{SiteID: "s1", Timestamp: now.Add(-time.Hour), Method: "GET", Path: "/new", Status: 200, ClientIP: "1.1.1.1"})
	db.Create(&HourlyStat{SiteID: "s1", Bucket: now.Add(-time.Hour), Requests: 10})

	r := &RetentionRunner{db: db, rawDays: 30, rollDays: 365}
	r.prune()

	var logCount, statCount int64
	db.Model(&RequestLog{}).Count(&logCount)
	db.Model(&HourlyStat{}).Count(&statCount)

	if logCount != 1 {
		t.Errorf("expected 1 log, got %d", logCount)
	}
	if statCount != 1 {
		t.Errorf("expected 1 stat, got %d", statCount)
	}
}
