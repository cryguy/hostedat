package analytics

import (
	"log"
	"sync"
	"time"

	"gorm.io/gorm"
)

// RetentionRunner periodically deletes expired analytics data.
// Mirrors the pattern of workeradapter.LogRetentionRunner.
type RetentionRunner struct {
	db       *gorm.DB
	rawDays  int
	rollDays int
	done     chan struct{}
	wg       sync.WaitGroup
}

// NewRetentionRunner creates and starts a retention runner that prunes
// request logs older than rawDays and hourly stats older than rollDays.
func NewRetentionRunner(db *gorm.DB, rawDays, rollDays int) *RetentionRunner {
	r := &RetentionRunner{
		db:       db,
		rawDays:  rawDays,
		rollDays: rollDays,
		done:     make(chan struct{}),
	}
	r.wg.Add(1)
	go r.run()
	return r
}

// Stop stops the retention runner and waits for it to finish.
func (r *RetentionRunner) Stop() {
	close(r.done)
	r.wg.Wait()
}

func (r *RetentionRunner) run() {
	defer r.wg.Done()

	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-r.done:
			return
		case <-ticker.C:
			r.prune()
		}
	}
}

func (r *RetentionRunner) prune() {
	rawCutoff := time.Now().AddDate(0, 0, -r.rawDays)
	if result := r.db.Where("timestamp < ?", rawCutoff).Delete(&RequestLog{}); result.Error != nil {
		log.Printf("analytics: raw log cleanup error: %v", result.Error)
	}

	rollCutoff := time.Now().AddDate(0, 0, -r.rollDays)
	if result := r.db.Where("bucket < ?", rollCutoff).Delete(&HourlyStat{}); result.Error != nil {
		log.Printf("analytics: rollup cleanup error: %v", result.Error)
	}
}
