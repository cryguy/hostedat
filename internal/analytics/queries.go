package analytics

import (
	"encoding/json"
	"sort"
	"time"

	"gorm.io/gorm"
)

// PeriodFilter defines the time range and bucket granularity for queries.
type PeriodFilter struct {
	From   time.Time
	To     time.Time
	Bucket string // "hour" or "day"
}

// SummaryResult holds aggregate totals for a site over a time period.
type SummaryResult struct {
	Requests       int64 `json:"requests"`
	UniqueVisitors int64 `json:"unique_visitors"`
	BytesSent      int64 `json:"bytes_sent"`
	Status2xx      int64 `json:"status_2xx"`
	Status3xx      int64 `json:"status_3xx"`
	Status4xx      int64 `json:"status_4xx"`
	Status5xx      int64 `json:"status_5xx"`
}

// TimeseriesPoint is a single data point in a time series.
type TimeseriesPoint struct {
	Bucket         time.Time `json:"bucket"`
	Requests       int64     `json:"requests"`
	UniqueVisitors int64     `json:"unique_visitors"`
	BytesSent      int64     `json:"bytes_sent"`
}

// ParsePeriod converts a period string ("24h", "7d", "30d") into a PeriodFilter.
// Defaults to "24h" for unrecognized values.
func ParsePeriod(period string) PeriodFilter {
	now := time.Now().UTC()
	pf := PeriodFilter{To: now}

	switch period {
	case "7d":
		pf.From = now.AddDate(0, 0, -7)
		pf.Bucket = "day"
	case "30d":
		pf.From = now.AddDate(0, 0, -30)
		pf.Bucket = "day"
	default: // "24h"
		pf.From = now.Add(-24 * time.Hour)
		pf.Bucket = "hour"
	}
	return pf
}

// GetSummary returns aggregate totals from hourly_stats for a site and period.
func GetSummary(db *gorm.DB, siteID string, pf PeriodFilter) (SummaryResult, error) {
	var result SummaryResult
	err := db.Model(&HourlyStat{}).
		Select(`
			COALESCE(SUM(requests), 0) AS requests,
			COALESCE(SUM(unique_visitors), 0) AS unique_visitors,
			COALESCE(SUM(bytes_sent), 0) AS bytes_sent,
			COALESCE(SUM(status2xx), 0) AS status2xx,
			COALESCE(SUM(status3xx), 0) AS status3xx,
			COALESCE(SUM(status4xx), 0) AS status4xx,
			COALESCE(SUM(status5xx), 0) AS status5xx
		`).
		Where("site_id = ? AND bucket >= ? AND bucket < ?", siteID, pf.From, pf.To).
		Scan(&result).Error
	return result, err
}

// GetTimeseries returns per-bucket data points for charting.
// Uses DATE(bucket) grouping for day buckets, raw bucket for hour buckets.
func GetTimeseries(db *gorm.DB, siteID string, pf PeriodFilter) ([]TimeseriesPoint, error) {
	var points []TimeseriesPoint

	if pf.Bucket == "day" {
		err := db.Model(&HourlyStat{}).
			Select(`
				DATE(bucket) AS bucket,
				SUM(requests) AS requests,
				SUM(unique_visitors) AS unique_visitors,
				SUM(bytes_sent) AS bytes_sent
			`).
			Where("site_id = ? AND bucket >= ? AND bucket < ?", siteID, pf.From, pf.To).
			Group("DATE(bucket)").
			Order("bucket ASC").
			Scan(&points).Error
		return points, err
	}

	// Hour buckets — one row per hourly_stat row.
	err := db.Model(&HourlyStat{}).
		Select("bucket, requests, unique_visitors, bytes_sent").
		Where("site_id = ? AND bucket >= ? AND bucket < ?", siteID, pf.From, pf.To).
		Order("bucket ASC").
		Scan(&points).Error
	return points, err
}

// GetTopPages merges top_paths JSON from each HourlyStat in the period,
// re-aggregates totals in Go, and returns the top N.
func GetTopPages(db *gorm.DB, siteID string, pf PeriodFilter, limit int) ([]TopEntry, error) {
	return getTopField(db, siteID, pf, "top_paths", limit)
}

// GetTopReferrers merges top_referrers JSON from each HourlyStat in the period,
// re-aggregates totals in Go, and returns the top N.
func GetTopReferrers(db *gorm.DB, siteID string, pf PeriodFilter, limit int) ([]TopEntry, error) {
	return getTopField(db, siteID, pf, "top_referrers", limit)
}

func getTopField(db *gorm.DB, siteID string, pf PeriodFilter, field string, limit int) ([]TopEntry, error) {
	var rows []struct {
		Data string
	}
	err := db.Model(&HourlyStat{}).
		Select(field + " AS data").
		Where("site_id = ? AND bucket >= ? AND bucket < ?", siteID, pf.From, pf.To).
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}

	merged := make(map[string]int64)
	for _, row := range rows {
		if row.Data == "" {
			continue
		}
		var entries []TopEntry
		if err := json.Unmarshal([]byte(row.Data), &entries); err != nil {
			continue
		}
		for _, e := range entries {
			merged[e.Value] += e.Requests
		}
	}

	result := make([]TopEntry, 0, len(merged))
	for k, v := range merged {
		result = append(result, TopEntry{Value: k, Requests: v})
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Requests > result[j].Requests
	})
	if len(result) > limit {
		result = result[:limit]
	}
	return result, nil
}
