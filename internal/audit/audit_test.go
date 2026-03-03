package audit

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/cryguy/hostedat/internal/config"
	"github.com/cryguy/hostedat/internal/models"
	"github.com/labstack/echo/v4"
)

// newEchoCtx creates a minimal echo.Context backed by a real HTTP request and recorder.
// userID and email are injected as context values the way AuthMiddleware would.
func newEchoCtx(userID, email string) echo.Context {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set("user_id", userID)
	c.Set("email", email)
	return c
}

func TestPtr_ReturnsPointerToSameString(t *testing.T) {
	s := "hello"
	p := Ptr(s)
	if p == nil {
		t.Fatal("Ptr returned nil, want non-nil pointer")
	}
	if *p != s {
		t.Errorf("*Ptr(%q) = %q, want %q", s, *p, s)
	}
}

func TestPtr_EmptyString(t *testing.T) {
	p := Ptr("")
	if p == nil {
		t.Fatal("Ptr(\"\") returned nil")
	}
	if *p != "" {
		t.Errorf("*Ptr(\"\") = %q, want empty string", *p)
	}
}

func TestRecord_SavesAllFields(t *testing.T) {
	db, err := models.InitDB(config.DBConfig{Driver: "sqlite", DSN: ":memory:"})
	if err != nil {
		t.Fatalf("InitDB: %v", err)
	}

	details := Ptr("some detail")
	c := newEchoCtx("actor123", "actor@example.com")

	Record(db, c, "site.create", "site", "res456", details)

	var entry models.AuditLog
	if err := db.First(&entry).Error; err != nil {
		t.Fatalf("querying audit log: %v", err)
	}

	if entry.ActorID != "actor123" {
		t.Errorf("ActorID = %q, want %q", entry.ActorID, "actor123")
	}
	if entry.ActorEmail != "actor@example.com" {
		t.Errorf("ActorEmail = %q, want %q", entry.ActorEmail, "actor@example.com")
	}
	if entry.Action != "site.create" {
		t.Errorf("Action = %q, want %q", entry.Action, "site.create")
	}
	if entry.ResourceType != "site" {
		t.Errorf("ResourceType = %q, want %q", entry.ResourceType, "site")
	}
	if entry.ResourceID != "res456" {
		t.Errorf("ResourceID = %q, want %q", entry.ResourceID, "res456")
	}
	if entry.Details == nil || *entry.Details != "some detail" {
		t.Errorf("Details = %v, want %q", entry.Details, "some detail")
	}
	if entry.CreatedAt.IsZero() {
		t.Error("CreatedAt should not be zero")
	}
}

func TestRecord_ReadsActorFromContext(t *testing.T) {
	db, err := models.InitDB(config.DBConfig{Driver: "sqlite", DSN: ":memory:"})
	if err != nil {
		t.Fatalf("InitDB: %v", err)
	}

	c := newEchoCtx("ctx-user-id", "ctx@example.com")
	Record(db, c, "deploy.create", "deployment", "dep001", nil)

	var entry models.AuditLog
	if err := db.First(&entry).Error; err != nil {
		t.Fatalf("querying audit log: %v", err)
	}

	if entry.ActorID != "ctx-user-id" {
		t.Errorf("ActorID = %q, want %q", entry.ActorID, "ctx-user-id")
	}
	if entry.ActorEmail != "ctx@example.com" {
		t.Errorf("ActorEmail = %q, want %q", entry.ActorEmail, "ctx@example.com")
	}
}

func TestRecord_EmptyContextValuesDoNotPanic(t *testing.T) {
	db, err := models.InitDB(config.DBConfig{Driver: "sqlite", DSN: ":memory:"})
	if err != nil {
		t.Fatalf("InitDB: %v", err)
	}

	// Context with no user_id / email set — should not panic.
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	Record(db, c, "user.login", "user", "", nil)

	var entry models.AuditLog
	if err := db.First(&entry).Error; err != nil {
		t.Fatalf("querying audit log: %v", err)
	}
	if entry.ActorID != "" {
		t.Errorf("ActorID = %q, want empty string", entry.ActorID)
	}
}

func TestRecordWithActor_ExplicitActorOverridesContext(t *testing.T) {
	db, err := models.InitDB(config.DBConfig{Driver: "sqlite", DSN: ":memory:"})
	if err != nil {
		t.Fatalf("InitDB: %v", err)
	}

	// Context has different actor values than what we pass explicitly.
	c := newEchoCtx("context-user", "context@example.com")

	RecordWithActor(db, c, "explicit-actor", "explicit@example.com", "user.register", "user", "u001", nil)

	var entry models.AuditLog
	if err := db.First(&entry).Error; err != nil {
		t.Fatalf("querying audit log: %v", err)
	}

	if entry.ActorID != "explicit-actor" {
		t.Errorf("ActorID = %q, want %q", entry.ActorID, "explicit-actor")
	}
	if entry.ActorEmail != "explicit@example.com" {
		t.Errorf("ActorEmail = %q, want %q", entry.ActorEmail, "explicit@example.com")
	}
}

func TestRecord_NilDetailsDoesNotCauseError(t *testing.T) {
	db, err := models.InitDB(config.DBConfig{Driver: "sqlite", DSN: ":memory:"})
	if err != nil {
		t.Fatalf("InitDB: %v", err)
	}

	c := newEchoCtx("u1", "u@example.com")
	Record(db, c, "site.delete", "site", "s123", nil)

	var entry models.AuditLog
	if err := db.First(&entry).Error; err != nil {
		t.Fatalf("querying audit log: %v", err)
	}
	if entry.Details != nil {
		t.Errorf("Details = %v, want nil", entry.Details)
	}
}

func TestRecord_DBError_DoesNotPanic(t *testing.T) {
	// Use a DB connection that has been explicitly closed to force an error.
	// models.InitDB returns a *gorm.DB; we can drop the table to simulate failure.
	db, err := models.InitDB(config.DBConfig{Driver: "sqlite", DSN: ":memory:"})
	if err != nil {
		t.Fatalf("InitDB: %v", err)
	}

	// Drop the audit_logs table so the insert will fail.
	if err := db.Migrator().DropTable(&models.AuditLog{}); err != nil {
		t.Fatalf("DropTable: %v", err)
	}

	c := newEchoCtx("u1", "u@example.com")

	// Must not panic; error is swallowed and only logged internally.
	Record(db, c, "site.create", "site", "s1", nil)
}

func TestRecordWithActor_MultipleEntriesAreDistinct(t *testing.T) {
	db, err := models.InitDB(config.DBConfig{Driver: "sqlite", DSN: ":memory:"})
	if err != nil {
		t.Fatalf("InitDB: %v", err)
	}

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	RecordWithActor(db, c, "user-a", "a@example.com", "site.create", "site", "s1", nil)
	RecordWithActor(db, c, "user-b", "b@example.com", "site.delete", "site", "s2", nil)

	var count int64
	db.Model(&models.AuditLog{}).Count(&count)
	if count != 2 {
		t.Errorf("count = %d, want 2", count)
	}

	var entries []models.AuditLog
	db.Order("id ASC").Find(&entries)

	if entries[0].ActorID != "user-a" {
		t.Errorf("first entry ActorID = %q, want user-a", entries[0].ActorID)
	}
	if entries[1].ActorID != "user-b" {
		t.Errorf("second entry ActorID = %q, want user-b", entries[1].ActorID)
	}
}
