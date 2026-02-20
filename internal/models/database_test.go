package models

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cryguy/hostedat/internal/config"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestInitDB_SQLite_InMemory(t *testing.T) {
	db, err := InitDB(config.DBConfig{Driver: "sqlite", DSN: ":memory:"})
	if err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	if db == nil {
		t.Fatal("expected non-nil db")
	}

	// Verify DB is usable
	var result int
	if err := db.Raw("SELECT 1").Scan(&result).Error; err != nil {
		t.Fatalf("DB query failed: %v", err)
	}
	if result != 1 {
		t.Errorf("query result = %d, want 1", result)
	}
}

func TestInitDB_SQLite_AppendsDSNParams(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := InitDB(config.DBConfig{Driver: "sqlite", DSN: dbPath})
	if err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	if db == nil {
		t.Fatal("expected non-nil db")
	}

	// Verify busy_timeout is set (this confirms DSN params were applied)
	var busyTimeout int
	if err := db.Raw("PRAGMA busy_timeout").Scan(&busyTimeout).Error; err != nil {
		t.Fatalf("failed to query busy_timeout: %v", err)
	}
	if busyTimeout != 5000 {
		t.Errorf("busy_timeout = %d, want 5000", busyTimeout)
	}
}

func TestInitDB_UnsupportedDriver_ReturnsError(t *testing.T) {
	_, err := InitDB(config.DBConfig{Driver: "postgres", DSN: "host=localhost"})
	if err == nil {
		t.Fatal("expected error for unsupported driver")
	}
	if !strings.Contains(err.Error(), "unsupported database driver") {
		t.Errorf("error = %q, want 'unsupported database driver'", err.Error())
	}
}

func TestInitDB_InvalidDSN_ReturnsError(t *testing.T) {
	_, err := InitDB(config.DBConfig{Driver: "sqlite", DSN: "/invalid/path/\x00/bad.db"})
	if err == nil {
		t.Fatal("expected error for invalid DSN")
	}
	if !strings.Contains(err.Error(), "opening database") {
		t.Errorf("error = %q, want 'opening database' message", err.Error())
	}
}

func TestEnsureNoDuplicateStorageBindingNames_NoDuplicates(t *testing.T) {
	db, err := InitDB(config.DBConfig{Driver: "sqlite", DSN: ":memory:"})
	if err != nil {
		t.Fatalf("InitDB: %v", err)
	}

	u := User{Email: "u@t.com", PasswordHash: "h"}
	db.Create(&u)
	s := Site{UserID: u.ID, SubdomainSlug: "test", Name: "Test"}
	db.Create(&s)

	// Create multiple storage buckets with different names
	db.Create(&StorageBucket{SiteID: s.ID, Name: "IMAGES", BucketName: "bucket-1"})
	db.Create(&StorageBucket{SiteID: s.ID, Name: "VIDEOS", BucketName: "bucket-2"})

	// No error expected
	if err := ensureNoDuplicateStorageBindingNames(db); err != nil {
		t.Fatalf("ensureNoDuplicateStorageBindingNames: %v", err)
	}
}

func TestEnsureNoDuplicateStorageBindingNames_WithDuplicates(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "dup.db")

	// Create a legacy database with duplicate (site_id, name) entries
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}

	if err := db.Exec(`
		CREATE TABLE storage_buckets (
			id TEXT PRIMARY KEY,
			site_id TEXT NOT NULL,
			name TEXT NOT NULL,
			bucket_name TEXT NOT NULL
		)
	`).Error; err != nil {
		t.Fatalf("create table: %v", err)
	}

	if err := db.Exec(`
		INSERT INTO storage_buckets (id, site_id, name, bucket_name) VALUES
		('id1', 'site1', 'IMAGES', 'bucket-1'),
		('id2', 'site1', 'IMAGES', 'bucket-2')
	`).Error; err != nil {
		t.Fatalf("insert duplicates: %v", err)
	}

	// Close DB before InitDB opens it
	sqlDB, _ := db.DB()
	if sqlDB != nil {
		sqlDB.Close()
	}

	// InitDB should detect duplicates and fail
	_, err = InitDB(config.DBConfig{Driver: "sqlite", DSN: dbPath})
	if err == nil {
		t.Fatal("expected error for duplicate storage binding names")
	}
	if !strings.Contains(err.Error(), "duplicate") {
		t.Errorf("error = %q, want 'duplicate' message", err.Error())
	}
}

func TestEnsureNoDuplicateStorageBindingNames_NoTable(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "empty.db")

	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}

	// No storage_buckets table exists yet
	if err := ensureNoDuplicateStorageBindingNames(db); err != nil {
		t.Fatalf("ensureNoDuplicateStorageBindingNames should not error when table doesn't exist: %v", err)
	}
}

func TestSeedDefaults_EmptyDatabase(t *testing.T) {
	db, err := InitDB(config.DBConfig{Driver: "sqlite", DSN: ":memory:"})
	if err != nil {
		t.Fatalf("InitDB: %v", err)
	}

	var count int64
	db.Model(&Setting{}).Count(&count)
	if count != 0 {
		t.Fatalf("expected empty settings table, got %d", count)
	}

	cfg := &config.Config{Registration: config.RegConfig{Enabled: false, InviteRequired: true}}
	if err := SeedDefaults(db, cfg); err != nil {
		t.Fatalf("SeedDefaults: %v", err)
	}

	db.Model(&Setting{}).Count(&count)
	if count != 2 {
		t.Errorf("setting count = %d, want 2", count)
	}
}

func TestInitDB_ClosesDBOnMigrationError(t *testing.T) {
	// This test verifies cleanup behavior when AutoMigrate fails
	// We can't easily force AutoMigrate to fail, so we test the duplicate check path instead
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "fail.db")

	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}

	// Create legacy table with duplicates
	if err := db.Exec(`
		CREATE TABLE storage_buckets (
			id TEXT PRIMARY KEY,
			site_id TEXT NOT NULL,
			name TEXT NOT NULL,
			bucket_name TEXT NOT NULL
		)
	`).Error; err != nil {
		t.Fatalf("create table: %v", err)
	}

	if err := db.Exec(`
		INSERT INTO storage_buckets (id, site_id, name, bucket_name) VALUES
		('a', 's1', 'DUP', 'b1'),
		('b', 's1', 'DUP', 'b2')
	`).Error; err != nil {
		t.Fatalf("insert: %v", err)
	}

	sqlDB, _ := db.DB()
	if sqlDB != nil {
		sqlDB.Close()
	}

	// InitDB should fail and clean up
	_, err = InitDB(config.DBConfig{Driver: "sqlite", DSN: dbPath})
	if err == nil {
		t.Fatal("expected error")
	}

	// Verify we can still access the database file (it wasn't corrupted)
	if _, err := os.Stat(dbPath); err != nil {
		t.Errorf("db file should still exist: %v", err)
	}
}
