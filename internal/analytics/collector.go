package analytics

import (
	"log"
	"sync"
	"time"

	"gorm.io/gorm"
)

// Event holds all fields needed to record a single request.
type Event struct {
	SiteID     string
	Timestamp  time.Time
	Method     string
	Path       string
	Status     int
	BytesSent  int64
	DurationMs int64
	ClientIP   string
	UserAgent  string
	Referer    string
	ServeType  string
}

const (
	channelCap = 4096
	batchSize  = 100
	flushEvery = time.Second
)

// Collector buffers analytics events and batch-writes them to the database.
// It is safe for concurrent use — Record never blocks the HTTP handler.
type Collector struct {
	db   *gorm.DB
	ch   chan Event
	done chan struct{}
	wg   sync.WaitGroup
}

// NewCollector creates a Collector and starts its background flush goroutine.
func NewCollector(db *gorm.DB) *Collector {
	c := &Collector{
		db:   db,
		ch:   make(chan Event, channelCap),
		done: make(chan struct{}),
	}
	c.wg.Add(1)
	go c.run()
	return c
}

// Record enqueues an event for writing. If the internal buffer is full the
// event is silently dropped — this ensures analytics never slows down request
// serving.
func (c *Collector) Record(ev Event) {
	select {
	case c.ch <- ev:
	default:
		// Channel full — drop rather than block the HTTP handler.
	}
}

// Stop signals the background goroutine to drain remaining events and exit.
func (c *Collector) Stop() {
	close(c.done)
	c.wg.Wait()
}

func (c *Collector) run() {
	defer c.wg.Done()

	ticker := time.NewTicker(flushEvery)
	defer ticker.Stop()

	buf := make([]RequestLog, 0, batchSize)

	flush := func() {
		if len(buf) == 0 {
			return
		}
		if err := c.db.CreateInBatches(buf, len(buf)).Error; err != nil {
			log.Printf("analytics: batch insert error: %v", err)
		}
		buf = buf[:0]
	}

	for {
		select {
		case ev := <-c.ch:
			buf = append(buf, eventToLog(ev))
			if len(buf) >= batchSize {
				flush()
			}

		case <-ticker.C:
			flush()

		case <-c.done:
			// Drain remaining events from the channel.
			for {
				select {
				case ev := <-c.ch:
					buf = append(buf, eventToLog(ev))
				default:
					flush()
					return
				}
			}
		}
	}
}

func eventToLog(ev Event) RequestLog {
	return RequestLog{
		SiteID:     ev.SiteID,
		Timestamp:  ev.Timestamp,
		Method:     ev.Method,
		Path:       ev.Path,
		Status:     ev.Status,
		BytesSent:  ev.BytesSent,
		DurationMs: ev.DurationMs,
		ClientIP:   ev.ClientIP,
		UserAgent:  ev.UserAgent,
		Referer:    ev.Referer,
		ServeType:  ev.ServeType,
	}
}
