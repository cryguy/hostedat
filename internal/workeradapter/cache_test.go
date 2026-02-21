package workeradapter

import (
	"testing"
	"time"

	"github.com/cryguy/hostedat/internal/models"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupCacheTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		DisableForeignKeyConstraintWhenMigrating: true,
	})
	if err != nil {
		t.Fatalf("opening test db: %v", err)
	}
	if err := db.AutoMigrate(&models.CacheEntry{}); err != nil {
		t.Fatalf("migrating: %v", err)
	}
	return db
}

func TestGORMCacheStore_PutAndMatch(t *testing.T) {
	db := setupCacheTestDB(t)
	cs := &GORMCacheStore{DB: db, SiteID: "site1"}

	ttl := 3600
	if err := cs.Put("default", "https://example.com/page", 200, `{"content-type":"text/html"}`, []byte("<h1>Hello</h1>"), &ttl); err != nil {
		t.Fatalf("Put: %v", err)
	}

	entry, err := cs.Match("default", "https://example.com/page")
	if err != nil {
		t.Fatalf("Match: %v", err)
	}
	if entry == nil {
		t.Fatal("Match returned nil, expected entry")
	}
	if entry.Status != 200 {
		t.Errorf("Status = %d, want 200", entry.Status)
	}
	if entry.Headers != `{"content-type":"text/html"}` {
		t.Errorf("Headers = %q", entry.Headers)
	}
	if string(entry.Body) != "<h1>Hello</h1>" {
		t.Errorf("Body = %q", string(entry.Body))
	}
	if entry.ExpiresAt == nil {
		t.Error("ExpiresAt should not be nil when TTL is set")
	}
}

func TestGORMCacheStore_MatchNotFound(t *testing.T) {
	db := setupCacheTestDB(t)
	cs := &GORMCacheStore{DB: db, SiteID: "site1"}

	entry, err := cs.Match("default", "https://example.com/missing")
	if err != nil {
		t.Fatalf("Match: %v", err)
	}
	if entry != nil {
		t.Errorf("expected nil, got %+v", entry)
	}
}

func TestGORMCacheStore_MatchExpired(t *testing.T) {
	db := setupCacheTestDB(t)
	cs := &GORMCacheStore{DB: db, SiteID: "site1"}

	// Insert an expired entry directly.
	past := time.Now().Add(-time.Hour)
	db.Create(&models.CacheEntry{
		SiteID:    "site1",
		CacheName: "default",
		URL:       "https://example.com/old",
		Status:    200,
		Headers:   "{}",
		Body:      []byte("old"),
		ExpiresAt: &past,
		CreatedAt: time.Now(),
	})

	entry, err := cs.Match("default", "https://example.com/old")
	if err != nil {
		t.Fatalf("Match: %v", err)
	}
	if entry != nil {
		t.Error("expected nil for expired entry")
	}

	// Verify the expired entry was cleaned up.
	var count int64
	db.Model(&models.CacheEntry{}).Count(&count)
	if count != 0 {
		t.Errorf("expected expired entry to be deleted, found %d entries", count)
	}
}

func TestGORMCacheStore_PutReplacesExisting(t *testing.T) {
	db := setupCacheTestDB(t)
	cs := &GORMCacheStore{DB: db, SiteID: "site1"}

	ttl := 3600
	if err := cs.Put("default", "https://example.com/page", 200, "{}", []byte("v1"), &ttl); err != nil {
		t.Fatalf("Put v1: %v", err)
	}
	if err := cs.Put("default", "https://example.com/page", 201, "{}", []byte("v2"), &ttl); err != nil {
		t.Fatalf("Put v2: %v", err)
	}

	entry, err := cs.Match("default", "https://example.com/page")
	if err != nil {
		t.Fatalf("Match: %v", err)
	}
	if entry == nil {
		t.Fatal("Match returned nil")
	}
	if entry.Status != 201 {
		t.Errorf("Status = %d, want 201", entry.Status)
	}
	if string(entry.Body) != "v2" {
		t.Errorf("Body = %q, want v2", string(entry.Body))
	}

	// Should only have one entry.
	var count int64
	db.Model(&models.CacheEntry{}).Count(&count)
	if count != 1 {
		t.Errorf("expected 1 entry after replace, got %d", count)
	}
}

func TestGORMCacheStore_PutNoTTL(t *testing.T) {
	db := setupCacheTestDB(t)
	cs := &GORMCacheStore{DB: db, SiteID: "site1"}

	if err := cs.Put("default", "https://example.com/forever", 200, "{}", []byte("data"), nil); err != nil {
		t.Fatalf("Put: %v", err)
	}

	entry, err := cs.Match("default", "https://example.com/forever")
	if err != nil {
		t.Fatalf("Match: %v", err)
	}
	if entry == nil {
		t.Fatal("Match returned nil")
	}
	if entry.ExpiresAt != nil {
		t.Error("ExpiresAt should be nil for no TTL")
	}
}

func TestGORMCacheStore_Delete(t *testing.T) {
	db := setupCacheTestDB(t)
	cs := &GORMCacheStore{DB: db, SiteID: "site1"}

	if err := cs.Put("default", "https://example.com/page", 200, "{}", []byte("data"), nil); err != nil {
		t.Fatalf("Put: %v", err)
	}

	deleted, err := cs.Delete("default", "https://example.com/page")
	if err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if !deleted {
		t.Error("expected deleted=true")
	}

	// Verify it's gone.
	entry, err := cs.Match("default", "https://example.com/page")
	if err != nil {
		t.Fatalf("Match after delete: %v", err)
	}
	if entry != nil {
		t.Error("expected nil after delete")
	}
}

func TestGORMCacheStore_DeleteNotFound(t *testing.T) {
	db := setupCacheTestDB(t)
	cs := &GORMCacheStore{DB: db, SiteID: "site1"}

	deleted, err := cs.Delete("default", "https://example.com/nonexistent")
	if err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if deleted {
		t.Error("expected deleted=false for nonexistent entry")
	}
}

func TestGORMCacheStore_SiteIsolation(t *testing.T) {
	db := setupCacheTestDB(t)
	cs1 := &GORMCacheStore{DB: db, SiteID: "site1"}
	cs2 := &GORMCacheStore{DB: db, SiteID: "site2"}

	if err := cs1.Put("default", "https://example.com/page", 200, "{}", []byte("site1-data"), nil); err != nil {
		t.Fatalf("Put site1: %v", err)
	}

	// site2 should not see site1's entry.
	entry, err := cs2.Match("default", "https://example.com/page")
	if err != nil {
		t.Fatalf("Match site2: %v", err)
	}
	if entry != nil {
		t.Error("site2 should not see site1's cache entry")
	}
}
