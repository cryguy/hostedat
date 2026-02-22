package workeradapter

import (
	"encoding/base64"
	"strings"
	"testing"
	"time"

	"github.com/cryguy/hostedat/internal/models"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupKVTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		DisableForeignKeyConstraintWhenMigrating: true,
	})
	if err != nil {
		t.Fatalf("opening test db: %v", err)
	}
	if err := db.AutoMigrate(&models.KVEntry{}); err != nil {
		t.Fatalf("migrating: %v", err)
	}
	return db
}

func TestGORMKVStore_PutAndGet(t *testing.T) {
	db := setupKVTestDB(t)
	kv := &GORMKVStore{DB: db, NamespaceID: "ns1"}

	if err := kv.Put("key1", "value1", nil, nil); err != nil {
		t.Fatalf("Put: %v", err)
	}

	val, err := kv.Get("key1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if val != "value1" {
		t.Errorf("Get = %q, want %q", val, "value1")
	}
}

func TestGORMKVStore_GetNotFound(t *testing.T) {
	db := setupKVTestDB(t)
	kv := &GORMKVStore{DB: db, NamespaceID: "ns1"}

	val, err := kv.Get("missing")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if val != "" {
		t.Errorf("expected empty for missing key, got %q", val)
	}
}

func TestGORMKVStore_GetExpired(t *testing.T) {
	db := setupKVTestDB(t)
	kv := &GORMKVStore{DB: db, NamespaceID: "ns1"}

	past := time.Now().Add(-time.Hour)
	db.Create(&models.KVEntry{
		ID:          "expired1",
		NamespaceID: "ns1",
		Key:         "old-key",
		Value:       "old-value",
		ExpiresAt:   &past,
	})

	val, err := kv.Get("old-key")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if val != "" {
		t.Errorf("expected empty for expired key, got %q", val)
	}

	// Verify cleanup.
	var count int64
	db.Model(&models.KVEntry{}).Where("key = ?", "old-key").Count(&count)
	if count != 0 {
		t.Error("expected expired entry to be deleted")
	}
}

func TestGORMKVStore_GetWithMetadata(t *testing.T) {
	db := setupKVTestDB(t)
	kv := &GORMKVStore{DB: db, NamespaceID: "ns1"}

	meta := `{"source":"test"}`
	if err := kv.Put("key1", "value1", &meta, nil); err != nil {
		t.Fatalf("Put: %v", err)
	}

	result, err := kv.GetWithMetadata("key1")
	if err != nil {
		t.Fatalf("GetWithMetadata: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Value != "value1" {
		t.Errorf("Value = %q, want %q", result.Value, "value1")
	}
	if result.Metadata == nil || *result.Metadata != meta {
		t.Errorf("Metadata = %v, want %q", result.Metadata, meta)
	}
}

func TestGORMKVStore_GetWithMetadataNotFound(t *testing.T) {
	db := setupKVTestDB(t)
	kv := &GORMKVStore{DB: db, NamespaceID: "ns1"}

	result, err := kv.GetWithMetadata("missing")
	if err != nil {
		t.Fatalf("GetWithMetadata: %v", err)
	}
	if result != nil {
		t.Error("expected nil for missing key")
	}
}

func TestGORMKVStore_GetWithMetadataExpired(t *testing.T) {
	db := setupKVTestDB(t)
	kv := &GORMKVStore{DB: db, NamespaceID: "ns1"}

	past := time.Now().Add(-time.Hour)
	db.Create(&models.KVEntry{
		ID:          "expired2",
		NamespaceID: "ns1",
		Key:         "expired-meta",
		Value:       "val",
		ExpiresAt:   &past,
	})

	result, err := kv.GetWithMetadata("expired-meta")
	if err != nil {
		t.Fatalf("GetWithMetadata: %v", err)
	}
	if result != nil {
		t.Error("expected nil for expired key")
	}
}

func TestGORMKVStore_PutUpdate(t *testing.T) {
	db := setupKVTestDB(t)
	kv := &GORMKVStore{DB: db, NamespaceID: "ns1"}

	if err := kv.Put("key1", "v1", nil, nil); err != nil {
		t.Fatalf("Put v1: %v", err)
	}
	if err := kv.Put("key1", "v2", nil, nil); err != nil {
		t.Fatalf("Put v2: %v", err)
	}

	val, err := kv.Get("key1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if val != "v2" {
		t.Errorf("expected updated value %q, got %q", "v2", val)
	}
}

func TestGORMKVStore_PutWithTTL(t *testing.T) {
	db := setupKVTestDB(t)
	kv := &GORMKVStore{DB: db, NamespaceID: "ns1"}

	ttl := 3600
	if err := kv.Put("ttl-key", "ttl-value", nil, &ttl); err != nil {
		t.Fatalf("Put: %v", err)
	}

	val, err := kv.Get("ttl-key")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if val != "ttl-value" {
		t.Errorf("Get = %q, want %q", val, "ttl-value")
	}
}

func TestGORMKVStore_PutMaxSize(t *testing.T) {
	db := setupKVTestDB(t)
	kv := &GORMKVStore{DB: db, NamespaceID: "ns1"}

	// Value exceeding 1MB should fail.
	bigValue := strings.Repeat("x", maxKVValueSize+1)
	err := kv.Put("big", bigValue, nil, nil)
	if err == nil {
		t.Error("expected error for oversized value")
	}
}

func TestGORMKVStore_Delete(t *testing.T) {
	db := setupKVTestDB(t)
	kv := &GORMKVStore{DB: db, NamespaceID: "ns1"}

	kv.Put("key1", "value1", nil, nil)

	if err := kv.Delete("key1"); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	val, err := kv.Get("key1")
	if err != nil {
		t.Fatalf("Get after delete: %v", err)
	}
	if val != "" {
		t.Errorf("expected empty after delete, got %q", val)
	}
}

func TestGORMKVStore_List(t *testing.T) {
	db := setupKVTestDB(t)
	kv := &GORMKVStore{DB: db, NamespaceID: "ns1"}

	kv.Put("alpha", "1", nil, nil)
	kv.Put("beta", "2", nil, nil)
	kv.Put("gamma", "3", nil, nil)

	result, err := kv.List("", 10, "")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(result.Keys) != 3 {
		t.Fatalf("expected 3 keys, got %d", len(result.Keys))
	}
	if !result.ListComplete {
		t.Error("expected ListComplete=true")
	}
	if result.Cursor != "" {
		t.Errorf("expected empty cursor, got %q", result.Cursor)
	}
}

func TestGORMKVStore_ListWithPrefix(t *testing.T) {
	db := setupKVTestDB(t)
	kv := &GORMKVStore{DB: db, NamespaceID: "ns1"}

	kv.Put("user:1", "a", nil, nil)
	kv.Put("user:2", "b", nil, nil)
	kv.Put("item:1", "c", nil, nil)

	result, err := kv.List("user:", 10, "")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(result.Keys) != 2 {
		t.Errorf("expected 2 keys with prefix 'user:', got %d", len(result.Keys))
	}
}

func TestGORMKVStore_ListPagination(t *testing.T) {
	db := setupKVTestDB(t)
	kv := &GORMKVStore{DB: db, NamespaceID: "ns1"}

	kv.Put("a", "1", nil, nil)
	kv.Put("b", "2", nil, nil)
	kv.Put("c", "3", nil, nil)

	// First page: limit 2.
	result, err := kv.List("", 2, "")
	if err != nil {
		t.Fatalf("List page 1: %v", err)
	}
	if len(result.Keys) != 2 {
		t.Fatalf("page 1: expected 2 keys, got %d", len(result.Keys))
	}
	if result.ListComplete {
		t.Error("page 1 should not be complete")
	}
	if result.Cursor == "" {
		t.Fatal("page 1 should have a cursor")
	}

	// Second page.
	result2, err := kv.List("", 2, result.Cursor)
	if err != nil {
		t.Fatalf("List page 2: %v", err)
	}
	if len(result2.Keys) != 1 {
		t.Fatalf("page 2: expected 1 key, got %d", len(result2.Keys))
	}
	if !result2.ListComplete {
		t.Error("page 2 should be complete")
	}
}

func TestGORMKVStore_ListDefaultLimit(t *testing.T) {
	db := setupKVTestDB(t)
	kv := &GORMKVStore{DB: db, NamespaceID: "ns1"}

	kv.Put("a", "1", nil, nil)

	// limit <= 0 defaults to 1000.
	result, err := kv.List("", 0, "")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(result.Keys) != 1 {
		t.Errorf("expected 1 key, got %d", len(result.Keys))
	}
}

func TestGORMKVStore_ListWithMetadata(t *testing.T) {
	db := setupKVTestDB(t)
	kv := &GORMKVStore{DB: db, NamespaceID: "ns1"}

	meta := `{"source":"test"}`
	kv.Put("key1", "val1", &meta, nil)
	kv.Put("key2", "val2", nil, nil)

	result, err := kv.List("", 10, "")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(result.Keys) != 2 {
		t.Fatalf("expected 2 keys, got %d", len(result.Keys))
	}
	// First key (key1) should have metadata.
	if result.Keys[0]["metadata"] == nil {
		t.Error("key1 should have metadata")
	}
}

func TestGORMKVStore_ListExcludesExpired(t *testing.T) {
	db := setupKVTestDB(t)
	kv := &GORMKVStore{DB: db, NamespaceID: "ns1"}

	kv.Put("valid", "v", nil, nil)

	past := time.Now().Add(-time.Hour)
	db.Create(&models.KVEntry{
		ID:          "exp1",
		NamespaceID: "ns1",
		Key:         "expired",
		Value:       "x",
		ExpiresAt:   &past,
	})

	result, err := kv.List("", 10, "")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(result.Keys) != 1 {
		t.Errorf("expected 1 valid key, got %d", len(result.Keys))
	}
}

func TestGORMKVStore_NamespaceIsolation(t *testing.T) {
	db := setupKVTestDB(t)
	kv1 := &GORMKVStore{DB: db, NamespaceID: "ns1"}
	kv2 := &GORMKVStore{DB: db, NamespaceID: "ns2"}

	kv1.Put("key", "ns1-value", nil, nil)

	val, err := kv2.Get("key")
	if err != nil {
		t.Fatalf("Get ns2: %v", err)
	}
	if val != "" {
		t.Errorf("ns2 should not see ns1 key, got %q", val)
	}
}

func TestDecodeCursor(t *testing.T) {
	tests := []struct {
		name   string
		cursor string
		want   int
	}{
		{"empty", "", 0},
		{"valid", base64.StdEncoding.EncodeToString([]byte("42")), 42},
		{"invalid base64", "!!!invalid!!!", 0},
		{"invalid number", base64.StdEncoding.EncodeToString([]byte("abc")), 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := decodeCursor(tt.cursor)
			if got != tt.want {
				t.Errorf("decodeCursor(%q) = %d, want %d", tt.cursor, got, tt.want)
			}
		})
	}
}

func TestEncodeCursor(t *testing.T) {
	cursor := encodeCursor(42)
	decoded := decodeCursor(cursor)
	if decoded != 42 {
		t.Errorf("encode/decode roundtrip: got %d, want 42", decoded)
	}
}
