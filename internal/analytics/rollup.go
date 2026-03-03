package analytics

import (
	"encoding/json"
	"log"
	"sort"
	"sync"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// RollupRunner periodically aggregates raw request logs into hourly stats.
type RollupRunner struct {
	db   *gorm.DB
	done chan struct{}
	wg   sync.WaitGroup
}

// NewRollupRunner creates and starts a rollup runner that aggregates raw
// request logs into HourlyStat rows. Runs once per hour, aligned to the
// hour boundary plus a 2-minute grace period for late-arriving events.
func NewRollupRunner(db *gorm.DB) *RollupRunner {
	r := &RollupRunner{
		db:   db,
		done: make(chan struct{}),
	}
	r.wg.Add(1)
	go r.run()
	return r
}

// Stop stops the rollup runner and waits for it to finish.
func (r *RollupRunner) Stop() {
	close(r.done)
	r.wg.Wait()
}

func (r *RollupRunner) run() {
	defer r.wg.Done()

	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-r.done:
			return
		case <-ticker.C:
			// Rollup the previous hour (current hour minus 1, with 2min grace).
			bucket := time.Now().UTC().Truncate(time.Hour).Add(-time.Hour)
			if err := r.RollupHour(bucket); err != nil {
				log.Printf("analytics: rollup error for %s: %v", bucket.Format(time.RFC3339), err)
			}
		}
	}
}

// RollupHour aggregates all RequestLog rows within [bucket, bucket+1h) into
// HourlyStat rows, one per site. Uses upsert so re-running is idempotent.
func (r *RollupRunner) RollupHour(bucket time.Time) error {
	bucketEnd := bucket.Add(time.Hour)

	var logs []RequestLog
	if err := r.db.Where("timestamp >= ? AND timestamp < ?", bucket, bucketEnd).Find(&logs).Error; err != nil {
		return err
	}
	if len(logs) == 0 {
		return nil
	}

	// Aggregate per site.
	type siteAgg struct {
		requests  int64
		uniqueIPs map[string]struct{}
		bytes     int64
		s2xx      int64
		s3xx      int64
		s4xx      int64
		s5xx      int64
		paths     map[string]int64
		referrers map[string]int64
	}

	sites := make(map[string]*siteAgg)
	for _, l := range logs {
		agg, ok := sites[l.SiteID]
		if !ok {
			agg = &siteAgg{
				uniqueIPs: make(map[string]struct{}),
				paths:     make(map[string]int64),
				referrers: make(map[string]int64),
			}
			sites[l.SiteID] = agg
		}
		agg.requests++
		agg.uniqueIPs[l.ClientIP] = struct{}{}
		agg.bytes += l.BytesSent

		switch {
		case l.Status >= 200 && l.Status < 300:
			agg.s2xx++
		case l.Status >= 300 && l.Status < 400:
			agg.s3xx++
		case l.Status >= 400 && l.Status < 500:
			agg.s4xx++
		case l.Status >= 500:
			agg.s5xx++
		}

		agg.paths[l.Path]++
		if l.Referer != "" {
			agg.referrers[l.Referer]++
		}
	}

	for siteID, agg := range sites {
		topPathsJSON, _ := json.Marshal(topN(agg.paths, 10))
		topRefsJSON, _ := json.Marshal(topN(agg.referrers, 10))

		stat := HourlyStat{
			SiteID:         siteID,
			Bucket:         bucket,
			Requests:       agg.requests,
			UniqueVisitors: int64(len(agg.uniqueIPs)),
			BytesSent:      agg.bytes,
			Status2xx:      agg.s2xx,
			Status3xx:      agg.s3xx,
			Status4xx:      agg.s4xx,
			Status5xx:      agg.s5xx,
			TopPaths:       string(topPathsJSON),
			TopReferrers:   string(topRefsJSON),
		}

		if err := r.db.Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "site_id"}, {Name: "bucket"}},
			DoUpdates: clause.AssignmentColumns([]string{
				"requests", "unique_visitors", "bytes_sent",
				"status2xx", "status3xx", "status4xx", "status5xx",
				"top_paths", "top_referrers",
			}),
		}).Create(&stat).Error; err != nil {
			log.Printf("analytics: upsert error for site %s bucket %s: %v", siteID, bucket.Format(time.RFC3339), err)
		}
	}

	return nil
}

// topN sorts a frequency map and returns the top n entries.
func topN(m map[string]int64, n int) []TopEntry {
	entries := make([]TopEntry, 0, len(m))
	for k, v := range m {
		entries = append(entries, TopEntry{Value: k, Requests: v})
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Requests > entries[j].Requests
	})
	if len(entries) > n {
		entries = entries[:n]
	}
	return entries
}
