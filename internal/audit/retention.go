package audit

import (
	"log"
	"sync"
	"time"

	"github.com/cryguy/hostedat/internal/models"
	"gorm.io/gorm"
)

// RetentionRunner periodically deletes old audit log entries.
type RetentionRunner struct {
	db      *gorm.DB
	maxDays int
	done    chan struct{}
	wg      sync.WaitGroup
}

// NewRetentionRunner creates and starts a retention runner that deletes
// audit logs older than maxDays. Ticks every hour.
func NewRetentionRunner(db *gorm.DB, maxDays int) *RetentionRunner {
	r := &RetentionRunner{
		db:      db,
		maxDays: maxDays,
		done:    make(chan struct{}),
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
			cutoff := time.Now().AddDate(0, 0, -r.maxDays)
			if result := r.db.Where("created_at < ?", cutoff).Delete(&models.AuditLog{}); result.Error != nil {
				log.Printf("audit log cleanup error: %v", result.Error)
			}
		}
	}
}
