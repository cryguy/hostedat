package analytics

import "time"

// RequestLog stores raw per-request data for short-term drill-down.
// Uses integer auto-increment PK (not nanoid) for write throughput.
type RequestLog struct {
	ID         int64     `gorm:"primaryKey;autoIncrement"`
	SiteID     string    `gorm:"index:idx_request_log_site_ts,priority:1;size:12"`
	Timestamp  time.Time `gorm:"index:idx_request_log_site_ts,priority:2"`
	Method     string    `gorm:"size:10"`
	Path       string    `gorm:"size:2048"`
	Status     int
	BytesSent  int64
	DurationMs int64
	ClientIP   string `gorm:"size:45"` // IPv6 max length
	UserAgent  string `gorm:"size:512"`
	Referer    string `gorm:"size:2048"`
	ServeType  string `gorm:"size:20"` // static, worker, redirect, rewrite, spa, 404, error
}

// HourlyStat stores pre-aggregated hourly rollups for long-term dashboards.
type HourlyStat struct {
	ID             int64     `gorm:"primaryKey;autoIncrement"`
	SiteID         string    `gorm:"uniqueIndex:idx_hourly_site_bucket,priority:1;size:12"`
	Bucket         time.Time `gorm:"uniqueIndex:idx_hourly_site_bucket,priority:2"` // hour-truncated UTC
	Requests       int64
	UniqueVisitors int64
	BytesSent      int64
	Status2xx      int64
	Status3xx      int64
	Status4xx      int64
	Status5xx      int64
	TopPaths       string `gorm:"type:text"` // JSON array of TopEntry
	TopReferrers   string `gorm:"type:text"` // JSON array of TopEntry
}

// TopEntry represents a single entry in top-N rankings (paths, referrers).
type TopEntry struct {
	Value    string `json:"value"`
	Requests int64  `json:"requests"`
}
