package workeradapter

import (
	"log"
	"sync"
	"time"

	"github.com/cryguy/hostedat/internal/models"
	"gorm.io/gorm"
)

// LogRetentionRunner periodically deletes old worker logs.
type LogRetentionRunner struct {
	db      *gorm.DB
	maxDays int
	done    chan struct{}
	wg      sync.WaitGroup
}

// NewLogRetentionRunner creates and starts a log retention runner that
// deletes worker logs older than maxDays. Ticks every hour.
func NewLogRetentionRunner(db *gorm.DB, maxDays int) *LogRetentionRunner {
	lr := &LogRetentionRunner{
		db:      db,
		maxDays: maxDays,
		done:    make(chan struct{}),
	}
	lr.wg.Add(1)
	go lr.run()
	return lr
}

// Stop stops the log retention runner and waits for it to finish.
func (lr *LogRetentionRunner) Stop() {
	close(lr.done)
	lr.wg.Wait()
}

func (lr *LogRetentionRunner) run() {
	defer lr.wg.Done()

	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-lr.done:
			return
		case <-ticker.C:
			cutoff := time.Now().AddDate(0, 0, -lr.maxDays)
			if result := lr.db.Where("created_at < ?", cutoff).Delete(&models.WorkerLog{}); result.Error != nil {
				log.Printf("worker log cleanup error: %v", result.Error)
			}
		}
	}
}
