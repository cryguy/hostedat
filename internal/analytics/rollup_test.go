package analytics

import (
	"encoding/json"
	"testing"
	"time"
)

func TestRollupHour_BasicAggregation(t *testing.T) {
	db := setupTestDB(t)
	bucket := time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC)

	// Insert raw logs.
	logs := []RequestLog{
		{SiteID: "s1", Timestamp: bucket.Add(5 * time.Minute), Method: "GET", Path: "/", Status: 200, BytesSent: 100, ClientIP: "1.1.1.1"},
		{SiteID: "s1", Timestamp: bucket.Add(10 * time.Minute), Method: "GET", Path: "/about", Status: 200, BytesSent: 200, ClientIP: "1.1.1.2"},
		{SiteID: "s1", Timestamp: bucket.Add(15 * time.Minute), Method: "GET", Path: "/", Status: 404, BytesSent: 50, ClientIP: "1.1.1.1"},
	}
	db.Create(&logs)

	runner := &RollupRunner{db: db}
	if err := runner.RollupHour(bucket); err != nil {
		t.Fatalf("rollup: %v", err)
	}

	var stat HourlyStat
	if err := db.First(&stat, "site_id = ? AND bucket = ?", "s1", bucket).Error; err != nil {
		t.Fatalf("query stat: %v", err)
	}

	if stat.Requests != 3 {
		t.Errorf("requests = %d, want 3", stat.Requests)
	}
	if stat.UniqueVisitors != 2 {
		t.Errorf("unique visitors = %d, want 2", stat.UniqueVisitors)
	}
	if stat.BytesSent != 350 {
		t.Errorf("bytes = %d, want 350", stat.BytesSent)
	}
	if stat.Status2xx != 2 {
		t.Errorf("2xx = %d, want 2", stat.Status2xx)
	}
	if stat.Status4xx != 1 {
		t.Errorf("4xx = %d, want 1", stat.Status4xx)
	}
}

func TestRollupHour_UniqueVisitorDedup(t *testing.T) {
	db := setupTestDB(t)
	bucket := time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC)

	// Same IP, multiple requests.
	for i := 0; i < 5; i++ {
		db.Create(&RequestLog{
			SiteID: "s1", Timestamp: bucket.Add(time.Duration(i) * time.Minute),
			Method: "GET", Path: "/", Status: 200, ClientIP: "1.1.1.1",
		})
	}

	runner := &RollupRunner{db: db}
	_ = runner.RollupHour(bucket)

	var stat HourlyStat
	db.First(&stat, "site_id = ? AND bucket = ?", "s1", bucket)
	if stat.UniqueVisitors != 1 {
		t.Errorf("unique visitors = %d, want 1", stat.UniqueVisitors)
	}
	if stat.Requests != 5 {
		t.Errorf("requests = %d, want 5", stat.Requests)
	}
}

func TestRollupHour_StatusBreakdown(t *testing.T) {
	db := setupTestDB(t)
	bucket := time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC)

	statuses := []int{200, 201, 301, 302, 404, 500}
	for i, s := range statuses {
		db.Create(&RequestLog{
			SiteID: "s1", Timestamp: bucket.Add(time.Duration(i) * time.Minute),
			Method: "GET", Path: "/", Status: s, ClientIP: "1.1.1.1",
		})
	}

	runner := &RollupRunner{db: db}
	_ = runner.RollupHour(bucket)

	var stat HourlyStat
	db.First(&stat, "site_id = ? AND bucket = ?", "s1", bucket)
	if stat.Status2xx != 2 {
		t.Errorf("2xx = %d, want 2", stat.Status2xx)
	}
	if stat.Status3xx != 2 {
		t.Errorf("3xx = %d, want 2", stat.Status3xx)
	}
	if stat.Status4xx != 1 {
		t.Errorf("4xx = %d, want 1", stat.Status4xx)
	}
	if stat.Status5xx != 1 {
		t.Errorf("5xx = %d, want 1", stat.Status5xx)
	}
}

func TestRollupHour_TopPathsJSON(t *testing.T) {
	db := setupTestDB(t)
	bucket := time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC)

	paths := []string{"/", "/", "/", "/about", "/about", "/contact"}
	for i, p := range paths {
		db.Create(&RequestLog{
			SiteID: "s1", Timestamp: bucket.Add(time.Duration(i) * time.Minute),
			Method: "GET", Path: p, Status: 200, ClientIP: "1.1.1.1",
		})
	}

	runner := &RollupRunner{db: db}
	_ = runner.RollupHour(bucket)

	var stat HourlyStat
	db.First(&stat, "site_id = ? AND bucket = ?", "s1", bucket)

	var entries []TopEntry
	if err := json.Unmarshal([]byte(stat.TopPaths), &entries); err != nil {
		t.Fatalf("unmarshal top paths: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("expected top paths entries")
	}
	if entries[0].Value != "/" || entries[0].Requests != 3 {
		t.Errorf("top path = %v, want / with 3 requests", entries[0])
	}
}

func TestRollupHour_IdempotentUpsert(t *testing.T) {
	db := setupTestDB(t)
	bucket := time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC)

	db.Create(&RequestLog{
		SiteID: "s1", Timestamp: bucket.Add(5 * time.Minute),
		Method: "GET", Path: "/", Status: 200, BytesSent: 100, ClientIP: "1.1.1.1",
	})

	runner := &RollupRunner{db: db}
	_ = runner.RollupHour(bucket)
	_ = runner.RollupHour(bucket) // re-run

	var count int64
	db.Model(&HourlyStat{}).Where("site_id = ? AND bucket = ?", "s1", bucket).Count(&count)
	if count != 1 {
		t.Errorf("expected exactly 1 hourly stat row after re-run, got %d", count)
	}
}

func TestRollupHour_EmptyHour(t *testing.T) {
	db := setupTestDB(t)
	bucket := time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC)

	runner := &RollupRunner{db: db}
	if err := runner.RollupHour(bucket); err != nil {
		t.Fatalf("rollup empty hour: %v", err)
	}

	var count int64
	db.Model(&HourlyStat{}).Count(&count)
	if count != 0 {
		t.Errorf("expected 0 rows for empty hour, got %d", count)
	}
}
