package analytics

import (
	"encoding/json"
	"testing"
	"time"
)

func TestGetSummary_Totals(t *testing.T) {
	db := setupTestDB(t)
	now := time.Now().UTC().Truncate(time.Hour)

	db.Create(&HourlyStat{SiteID: "s1", Bucket: now.Add(-2 * time.Hour), Requests: 100, UniqueVisitors: 50, BytesSent: 1000, Status2xx: 80, Status3xx: 10, Status4xx: 5, Status5xx: 5})
	db.Create(&HourlyStat{SiteID: "s1", Bucket: now.Add(-1 * time.Hour), Requests: 200, UniqueVisitors: 100, BytesSent: 2000, Status2xx: 180, Status3xx: 10, Status4xx: 5, Status5xx: 5})

	pf := PeriodFilter{From: now.Add(-24 * time.Hour), To: now, Bucket: "hour"}
	result, err := GetSummary(db, "s1", pf)
	if err != nil {
		t.Fatalf("GetSummary: %v", err)
	}

	if result.Requests != 300 {
		t.Errorf("requests = %d, want 300", result.Requests)
	}
	if result.UniqueVisitors != 150 {
		t.Errorf("visitors = %d, want 150", result.UniqueVisitors)
	}
	if result.BytesSent != 3000 {
		t.Errorf("bytes = %d, want 3000", result.BytesSent)
	}
	if result.Status2xx != 260 {
		t.Errorf("2xx = %d, want 260", result.Status2xx)
	}
}

func TestGetSummary_PeriodFiltering(t *testing.T) {
	db := setupTestDB(t)
	now := time.Now().UTC().Truncate(time.Hour)

	// One inside the window, one outside.
	db.Create(&HourlyStat{SiteID: "s1", Bucket: now.Add(-2 * time.Hour), Requests: 100})
	db.Create(&HourlyStat{SiteID: "s1", Bucket: now.Add(-48 * time.Hour), Requests: 999})

	pf := PeriodFilter{From: now.Add(-24 * time.Hour), To: now, Bucket: "hour"}
	result, _ := GetSummary(db, "s1", pf)

	if result.Requests != 100 {
		t.Errorf("requests = %d, want 100 (should exclude old data)", result.Requests)
	}
}

func TestGetSummary_EmptyData(t *testing.T) {
	db := setupTestDB(t)
	pf := ParsePeriod("24h")
	result, err := GetSummary(db, "nonexistent", pf)
	if err != nil {
		t.Fatalf("GetSummary: %v", err)
	}
	if result.Requests != 0 {
		t.Errorf("expected 0 requests for empty data, got %d", result.Requests)
	}
}

func TestGetTimeseries_HourBuckets(t *testing.T) {
	db := setupTestDB(t)
	now := time.Now().UTC().Truncate(time.Hour)

	db.Create(&HourlyStat{SiteID: "s1", Bucket: now.Add(-3 * time.Hour), Requests: 10, UniqueVisitors: 5, BytesSent: 100})
	db.Create(&HourlyStat{SiteID: "s1", Bucket: now.Add(-2 * time.Hour), Requests: 20, UniqueVisitors: 10, BytesSent: 200})
	db.Create(&HourlyStat{SiteID: "s1", Bucket: now.Add(-1 * time.Hour), Requests: 30, UniqueVisitors: 15, BytesSent: 300})

	pf := PeriodFilter{From: now.Add(-24 * time.Hour), To: now, Bucket: "hour"}
	points, err := GetTimeseries(db, "s1", pf)
	if err != nil {
		t.Fatalf("GetTimeseries: %v", err)
	}

	if len(points) != 3 {
		t.Fatalf("expected 3 points, got %d", len(points))
	}
	if points[0].Requests != 10 {
		t.Errorf("first point requests = %d, want 10", points[0].Requests)
	}
	if points[2].Requests != 30 {
		t.Errorf("last point requests = %d, want 30", points[2].Requests)
	}
}

func TestGetTopPages_MergeAcrossHours(t *testing.T) {
	db := setupTestDB(t)
	now := time.Now().UTC().Truncate(time.Hour)

	hour1Paths, _ := json.Marshal([]TopEntry{{Value: "/", Requests: 10}, {Value: "/about", Requests: 5}})
	hour2Paths, _ := json.Marshal([]TopEntry{{Value: "/", Requests: 20}, {Value: "/contact", Requests: 3}})

	db.Create(&HourlyStat{SiteID: "s1", Bucket: now.Add(-2 * time.Hour), Requests: 15, TopPaths: string(hour1Paths)})
	db.Create(&HourlyStat{SiteID: "s1", Bucket: now.Add(-1 * time.Hour), Requests: 23, TopPaths: string(hour2Paths)})

	pf := PeriodFilter{From: now.Add(-24 * time.Hour), To: now, Bucket: "hour"}
	pages, err := GetTopPages(db, "s1", pf, 10)
	if err != nil {
		t.Fatalf("GetTopPages: %v", err)
	}

	if len(pages) != 3 {
		t.Fatalf("expected 3 pages, got %d", len(pages))
	}
	// "/" should be first with 30 total
	if pages[0].Value != "/" || pages[0].Requests != 30 {
		t.Errorf("top page = %v, want / with 30", pages[0])
	}
}

func TestGetTopPages_LimitParameter(t *testing.T) {
	db := setupTestDB(t)
	now := time.Now().UTC().Truncate(time.Hour)

	entries := make([]TopEntry, 20)
	for i := range entries {
		entries[i] = TopEntry{Value: "/page" + string(rune('a'+i)), Requests: int64(20 - i)}
	}
	pathsJSON, _ := json.Marshal(entries)

	db.Create(&HourlyStat{SiteID: "s1", Bucket: now.Add(-1 * time.Hour), Requests: 210, TopPaths: string(pathsJSON)})

	pf := PeriodFilter{From: now.Add(-24 * time.Hour), To: now, Bucket: "hour"}
	pages, err := GetTopPages(db, "s1", pf, 5)
	if err != nil {
		t.Fatalf("GetTopPages: %v", err)
	}

	if len(pages) != 5 {
		t.Fatalf("expected 5 pages (limit), got %d", len(pages))
	}
}

func TestParsePeriod(t *testing.T) {
	tests := []struct {
		input  string
		bucket string
	}{
		{"24h", "hour"},
		{"7d", "day"},
		{"30d", "day"},
		{"invalid", "hour"}, // defaults to 24h
	}
	for _, tt := range tests {
		pf := ParsePeriod(tt.input)
		if pf.Bucket != tt.bucket {
			t.Errorf("ParsePeriod(%q).Bucket = %q, want %q", tt.input, pf.Bucket, tt.bucket)
		}
		if pf.From.IsZero() || pf.To.IsZero() {
			t.Errorf("ParsePeriod(%q) has zero times", tt.input)
		}
	}
}
