package models

import (
	"testing"
	"time"

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

func TestWorkerEnvVar_BeforeCreate_SetsID(t *testing.T) {
	db := setupTestDB(t)
	u := User{Email: "u@t.com", PasswordHash: "h"}
	db.Create(&u)
	s := Site{UserID: u.ID, SubdomainSlug: "test", Name: "Test"}
	db.Create(&s)

	w := WorkerEnvVar{SiteID: s.ID, Name: "API_KEY", Value: "secret123", Secret: false}
	if err := db.Create(&w).Error; err != nil {
		t.Fatalf("Create: %v", err)
	}
	if w.ID == "" {
		t.Error("expected auto-generated ID")
	}
	if len(w.ID) != nanoidLength {
		t.Errorf("ID length = %d, want %d", len(w.ID), nanoidLength)
	}
}

func TestWorkerEnvVar_Secret(t *testing.T) {
	db := setupTestDB(t)
	u := User{Email: "u@t.com", PasswordHash: "h"}
	db.Create(&u)
	s := Site{UserID: u.ID, SubdomainSlug: "test", Name: "Test"}
	db.Create(&s)

	w1 := WorkerEnvVar{SiteID: s.ID, Name: "PUBLIC", Value: "public", Secret: false}
	db.Create(&w1)
	if w1.Secret {
		t.Error("expected Secret=false")
	}

	w2 := WorkerEnvVar{SiteID: s.ID, Name: "SECRET", Value: "secret", Secret: true}
	db.Create(&w2)
	if !w2.Secret {
		t.Error("expected Secret=true")
	}
}

func TestKVNamespace_BeforeCreate_SetsID(t *testing.T) {
	db := setupTestDB(t)
	u := User{Email: "u@t.com", PasswordHash: "h"}
	db.Create(&u)
	s := Site{UserID: u.ID, SubdomainSlug: "test", Name: "Test"}
	db.Create(&s)

	kv := KVNamespace{SiteID: s.ID, Name: "MY_KV"}
	if err := db.Create(&kv).Error; err != nil {
		t.Fatalf("Create: %v", err)
	}
	if kv.ID == "" {
		t.Error("expected auto-generated ID")
	}
	if len(kv.ID) != nanoidLength {
		t.Errorf("ID length = %d, want %d", len(kv.ID), nanoidLength)
	}
}

func TestKVEntry_BeforeCreate_SetsID(t *testing.T) {
	db := setupTestDB(t)
	u := User{Email: "u@t.com", PasswordHash: "h"}
	db.Create(&u)
	s := Site{UserID: u.ID, SubdomainSlug: "test", Name: "Test"}
	db.Create(&s)
	ns := KVNamespace{SiteID: s.ID, Name: "MY_KV"}
	db.Create(&ns)

	entry := KVEntry{NamespaceID: ns.ID, Key: "key1", Value: "value1"}
	if err := db.Create(&entry).Error; err != nil {
		t.Fatalf("Create: %v", err)
	}
	if entry.ID == "" {
		t.Error("expected auto-generated ID")
	}
	if len(entry.ID) != nanoidLength {
		t.Errorf("ID length = %d, want %d", len(entry.ID), nanoidLength)
	}
}

func TestKVEntry_WithMetadataAndExpiry(t *testing.T) {
	db := setupTestDB(t)
	u := User{Email: "u@t.com", PasswordHash: "h"}
	db.Create(&u)
	s := Site{UserID: u.ID, SubdomainSlug: "test", Name: "Test"}
	db.Create(&s)
	ns := KVNamespace{SiteID: s.ID, Name: "MY_KV"}
	db.Create(&ns)

	// Test with nil metadata and expiry
	e1 := KVEntry{NamespaceID: ns.ID, Key: "k1", Value: "v1"}
	db.Create(&e1)
	if e1.Metadata != nil {
		t.Error("expected nil Metadata")
	}
	if e1.ExpiresAt != nil {
		t.Error("expected nil ExpiresAt")
	}

	// Test with metadata and expiry
	metadata := `{"version":1}`
	expiry := time.Now().Add(24 * time.Hour)
	e2 := KVEntry{NamespaceID: ns.ID, Key: "k2", Value: "v2", Metadata: &metadata, ExpiresAt: &expiry}
	db.Create(&e2)
	if e2.Metadata == nil || *e2.Metadata != metadata {
		t.Errorf("Metadata = %v, want %q", e2.Metadata, metadata)
	}
	if e2.ExpiresAt == nil {
		t.Error("expected non-nil ExpiresAt")
	}
}

func TestCronSchedule_BeforeCreate_SetsID(t *testing.T) {
	db := setupTestDB(t)
	u := User{Email: "u@t.com", PasswordHash: "h"}
	db.Create(&u)
	s := Site{UserID: u.ID, SubdomainSlug: "test", Name: "Test"}
	db.Create(&s)

	cron := CronSchedule{SiteID: s.ID, Cron: "0 * * * *", Enabled: true}
	if err := db.Create(&cron).Error; err != nil {
		t.Fatalf("Create: %v", err)
	}
	if cron.ID == "" {
		t.Error("expected auto-generated ID")
	}
	if len(cron.ID) != nanoidLength {
		t.Errorf("ID length = %d, want %d", len(cron.ID), nanoidLength)
	}
}

func TestCronSchedule_Enabled(t *testing.T) {
	db := setupTestDB(t)
	u := User{Email: "u@t.com", PasswordHash: "h"}
	db.Create(&u)
	s := Site{UserID: u.ID, SubdomainSlug: "test", Name: "Test"}
	db.Create(&s)

	c1 := CronSchedule{SiteID: s.ID, Cron: "0 0 * * *", Enabled: true}
	db.Create(&c1)
	if !c1.Enabled {
		t.Error("expected Enabled=true")
	}

	// For false values with GORM defaults, we need to use Select to force the field update
	c2 := CronSchedule{SiteID: s.ID, Cron: "0 1 * * *", Enabled: false}
	db.Select("SiteID", "Cron", "Enabled").Create(&c2)

	// Re-read from DB to verify
	var fetched CronSchedule
	db.First(&fetched, "id = ?", c2.ID)
	if fetched.Enabled {
		t.Error("expected Enabled=false")
	}
}

func TestWorkerLog_BeforeCreate_SetsID(t *testing.T) {
	db := setupTestDB(t)
	u := User{Email: "u@t.com", PasswordHash: "h"}
	db.Create(&u)
	s := Site{UserID: u.ID, SubdomainSlug: "test", Name: "Test"}
	db.Create(&s)

	log := WorkerLog{SiteID: s.ID, Level: "info", Message: "test log"}
	if err := db.Create(&log).Error; err != nil {
		t.Fatalf("Create: %v", err)
	}
	if log.ID == "" {
		t.Error("expected auto-generated ID")
	}
	if len(log.ID) != nanoidLength {
		t.Errorf("ID length = %d, want %d", len(log.ID), nanoidLength)
	}
}

func TestAuthCode_BeforeCreate_SetsID(t *testing.T) {
	db := setupTestDB(t)
	u := User{Email: "u@t.com", PasswordHash: "h"}
	db.Create(&u)

	code := AuthCode{
		Code:          "auth_code_123",
		UserID:        u.ID,
		CodeChallenge: "challenge",
		ExpiresAt:     time.Now().Add(10 * time.Minute),
	}
	if err := db.Create(&code).Error; err != nil {
		t.Fatalf("Create: %v", err)
	}
	if code.ID == "" {
		t.Error("expected auto-generated ID")
	}
	if len(code.ID) != nanoidLength {
		t.Errorf("ID length = %d, want %d", len(code.ID), nanoidLength)
	}
}

func TestAPIKey_BeforeCreate_SetsID(t *testing.T) {
	db := setupTestDB(t)
	u := User{Email: "u@t.com", PasswordHash: "h"}
	db.Create(&u)

	key := APIKey{UserID: u.ID, KeyHash: "hash123", Name: "Test Key"}
	if err := db.Create(&key).Error; err != nil {
		t.Fatalf("Create: %v", err)
	}
	if key.ID == "" {
		t.Error("expected auto-generated ID")
	}
	if len(key.ID) != nanoidLength {
		t.Errorf("ID length = %d, want %d", len(key.ID), nanoidLength)
	}
}

func TestInvite_BeforeCreate_SetsID(t *testing.T) {
	db := setupTestDB(t)
	u := User{Email: "u@t.com", PasswordHash: "h"}
	db.Create(&u)

	inv := Invite{Code: "INVITE123", CreatedBy: u.ID}
	if err := db.Create(&inv).Error; err != nil {
		t.Fatalf("Create: %v", err)
	}
	if inv.ID == "" {
		t.Error("expected auto-generated ID")
	}
	if len(inv.ID) != nanoidLength {
		t.Errorf("ID length = %d, want %d", len(inv.ID), nanoidLength)
	}
}
