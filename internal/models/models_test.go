package models

import (
	"testing"

	"github.com/cryguy/hostedat/internal/config"
	"gorm.io/gorm"
)

func setupTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := InitDB(config.DBConfig{Driver: "sqlite", DSN: ":memory:"})
	if err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	return db
}

func TestInitDB_CreatesAllTables(t *testing.T) {
	db := setupTestDB(t)
	for _, table := range []interface{}{&User{}, &Site{}, &Deployment{}, &APIKey{}, &Invite{}, &Setting{}} {
		if !db.Migrator().HasTable(table) {
			t.Errorf("missing table for %T", table)
		}
	}
}

func TestInitDB_UnsupportedDriver(t *testing.T) {
	_, err := InitDB(config.DBConfig{Driver: "postgres", DSN: "test"})
	if err == nil {
		t.Fatal("expected error for unsupported driver")
	}
}

func TestUser_BeforeCreate_SetsIDAndRole(t *testing.T) {
	db := setupTestDB(t)
	u := User{Email: "a@b.com", PasswordHash: "hash"}
	if err := db.Create(&u).Error; err != nil {
		t.Fatalf("Create: %v", err)
	}
	if u.ID == "" {
		t.Error("expected auto-generated ID")
	}
	if len(u.ID) != nanoidLength {
		t.Errorf("ID length = %d, want %d", len(u.ID), nanoidLength)
	}
	if u.Role != "user" {
		t.Errorf("Role = %q, want user", u.Role)
	}
}

func TestUser_BeforeCreate_PreservesExplicitValues(t *testing.T) {
	db := setupTestDB(t)
	u := User{ID: "custom_id_1234", Email: "b@c.com", PasswordHash: "hash", Role: "admin"}
	if err := db.Create(&u).Error; err != nil {
		t.Fatalf("Create: %v", err)
	}
	if u.ID != "custom_id_1234" {
		t.Errorf("ID = %q, want custom_id_1234", u.ID)
	}
	if u.Role != "admin" {
		t.Errorf("Role = %q, want admin", u.Role)
	}
}

func TestSite_BeforeCreate_SetsID(t *testing.T) {
	db := setupTestDB(t)
	// Create a user first for FK
	u := User{Email: "u@t.com", PasswordHash: "h"}
	db.Create(&u)

	s := Site{UserID: u.ID, SubdomainSlug: "mysite", Name: "My Site"}
	if err := db.Create(&s).Error; err != nil {
		t.Fatalf("Create: %v", err)
	}
	if s.ID == "" {
		t.Error("expected auto-generated ID")
	}
}

func TestDeployment_BeforeCreate_SetsIDAndTimestamp(t *testing.T) {
	db := setupTestDB(t)
	u := User{Email: "u@t.com", PasswordHash: "h"}
	db.Create(&u)
	s := Site{UserID: u.ID, SubdomainSlug: "dep-test", Name: "Test"}
	db.Create(&s)

	d := Deployment{SiteID: s.ID, Version: 1, FileHash: "abc"}
	if err := db.Create(&d).Error; err != nil {
		t.Fatalf("Create: %v", err)
	}
	if d.ID == "" {
		t.Error("expected auto-generated ID")
	}
	if d.UploadedAt.IsZero() {
		t.Error("expected auto-set UploadedAt")
	}
}

func TestSeedDefaults_SeedsSettings(t *testing.T) {
	db := setupTestDB(t)
	cfg := &config.Config{Registration: config.RegConfig{Enabled: true, InviteRequired: false}}
	if err := SeedDefaults(db, cfg); err != nil {
		t.Fatalf("SeedDefaults: %v", err)
	}

	val, err := GetSetting(db, "registration_enabled")
	if err != nil {
		t.Fatalf("GetSetting: %v", err)
	}
	if val != "true" {
		t.Errorf("registration_enabled = %q, want true", val)
	}

	val, err = GetSetting(db, "invite_required")
	if err != nil {
		t.Fatalf("GetSetting: %v", err)
	}
	if val != "false" {
		t.Errorf("invite_required = %q, want false", val)
	}
}

func TestSeedDefaults_Idempotent(t *testing.T) {
	db := setupTestDB(t)
	cfg := &config.Config{Registration: config.RegConfig{Enabled: true}}

	if err := SeedDefaults(db, cfg); err != nil {
		t.Fatalf("first SeedDefaults: %v", err)
	}
	// Change config and seed again — should be no-op
	cfg2 := &config.Config{Registration: config.RegConfig{Enabled: false}}
	if err := SeedDefaults(db, cfg2); err != nil {
		t.Fatalf("second SeedDefaults: %v", err)
	}

	val, _ := GetSetting(db, "registration_enabled")
	if val != "true" {
		t.Errorf("value changed on second seed: %q", val)
	}
}

func TestGetSetting_NotFound(t *testing.T) {
	db := setupTestDB(t)
	_, err := GetSetting(db, "nonexistent_key")
	if err == nil {
		t.Fatal("expected error for missing setting")
	}
}

func TestSetSetting_CreateAndUpdate(t *testing.T) {
	db := setupTestDB(t)

	// Create
	if err := SetSetting(db, "test_key", "value1"); err != nil {
		t.Fatalf("SetSetting create: %v", err)
	}
	val, _ := GetSetting(db, "test_key")
	if val != "value1" {
		t.Errorf("got %q, want value1", val)
	}

	// Update
	if err := SetSetting(db, "test_key", "value2"); err != nil {
		t.Fatalf("SetSetting update: %v", err)
	}
	val, _ = GetSetting(db, "test_key")
	if val != "value2" {
		t.Errorf("got %q, want value2", val)
	}
}

func TestUser_EmailUniqueIndex(t *testing.T) {
	db := setupTestDB(t)
	db.Create(&User{Email: "dup@test.com", PasswordHash: "h"})
	err := db.Create(&User{Email: "dup@test.com", PasswordHash: "h2"}).Error
	if err == nil {
		t.Fatal("expected unique constraint error for duplicate email")
	}
}

func TestSite_SubdomainSlugUniqueIndex(t *testing.T) {
	db := setupTestDB(t)
	u := User{Email: "u@t.com", PasswordHash: "h"}
	db.Create(&u)
	db.Create(&Site{UserID: u.ID, SubdomainSlug: "taken", Name: "Site 1"})
	err := db.Create(&Site{UserID: u.ID, SubdomainSlug: "taken", Name: "Site 2"}).Error
	if err == nil {
		t.Fatal("expected unique constraint error for duplicate slug")
	}
}
