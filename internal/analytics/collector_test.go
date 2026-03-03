package analytics

import (
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func setupTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	if err := db.AutoMigrate(&RequestLog{}, &HourlyStat{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func TestCollector_RecordAndFlush(t *testing.T) {
	db := setupTestDB(t)
	c := NewCollector(db)

	c.Record(Event{
		SiteID:    "site1",
		Timestamp: time.Now(),
		Method:    "GET",
		Path:      "/index.html",
		Status:    200,
		BytesSent: 1024,
		ClientIP:  "1.2.3.4",
		ServeType: "static",
	})
	c.Record(Event{
		SiteID:    "site1",
		Timestamp: time.Now(),
		Method:    "GET",
		Path:      "/about",
		Status:    200,
		BytesSent: 512,
		ClientIP:  "1.2.3.5",
		ServeType: "static",
	})

	// Stop flushes remaining events.
	c.Stop()

	var count int64
	db.Model(&RequestLog{}).Count(&count)
	if count != 2 {
		t.Fatalf("expected 2 request logs, got %d", count)
	}
}

func TestCollector_NonBlockingOnFullChannel(t *testing.T) {
	db := setupTestDB(t)
	c := &Collector{
		db:   db,
		ch:   make(chan Event, 1), // tiny buffer
		done: make(chan struct{}),
	}

	// Fill the channel.
	c.ch <- Event{SiteID: "a"}

	// This should not block — it drops silently.
	c.Record(Event{SiteID: "b"})

	// Drain and verify only 1 event.
	ev := <-c.ch
	if ev.SiteID != "a" {
		t.Fatalf("expected event 'a', got %q", ev.SiteID)
	}
}

func TestCollector_StopDrainsEvents(t *testing.T) {
	db := setupTestDB(t)
	c := NewCollector(db)

	for i := 0; i < 50; i++ {
		c.Record(Event{
			SiteID:    "site1",
			Timestamp: time.Now(),
			Method:    "GET",
			Path:      "/page",
			Status:    200,
			ServeType: "static",
		})
	}

	c.Stop()

	var count int64
	db.Model(&RequestLog{}).Count(&count)
	if count != 50 {
		t.Fatalf("expected 50 request logs after drain, got %d", count)
	}
}

func TestCollector_BatchSizeTriggersFlush(t *testing.T) {
	db := setupTestDB(t)
	c := NewCollector(db)

	// Send more than batchSize events.
	for i := 0; i < batchSize+10; i++ {
		c.Record(Event{
			SiteID:    "site1",
			Timestamp: time.Now(),
			Method:    "GET",
			Path:      "/",
			Status:    200,
			ServeType: "static",
		})
	}

	// Give the goroutine time to process the batch.
	time.Sleep(200 * time.Millisecond)

	var count int64
	db.Model(&RequestLog{}).Count(&count)
	// At least one full batch should have flushed.
	if count < int64(batchSize) {
		t.Fatalf("expected at least %d flushed rows, got %d", batchSize, count)
	}

	c.Stop()
}
