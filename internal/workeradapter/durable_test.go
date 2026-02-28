package workeradapter

import (
	"testing"

	"github.com/cryguy/hostedat/internal/models"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func setupDurableTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		DisableForeignKeyConstraintWhenMigrating: true,
	})
	if err != nil {
		t.Fatalf("opening test db: %v", err)
	}
	if err := db.AutoMigrate(&models.DurableObjectEntry{}); err != nil {
		t.Fatalf("migrating: %v", err)
	}
	return db
}

func TestDurableObjectStore_PutAndGet(t *testing.T) {
	db := setupDurableTestDB(t)
	store := &GORMDurableObjectStore{DB: db}

	if err := store.Put("ns1", "obj1", "key1", `{"value": 42}`); err != nil {
		t.Fatalf("Put: %v", err)
	}

	val, err := store.Get("ns1", "obj1", "key1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if val != `{"value": 42}` {
		t.Errorf("Get = %q, want %q", val, `{"value": 42}`)
	}
}

func TestDurableObjectStore_GetNotFound(t *testing.T) {
	db := setupDurableTestDB(t)
	store := &GORMDurableObjectStore{DB: db}

	val, err := store.Get("ns1", "obj1", "missing")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if val != "" {
		t.Errorf("expected empty string for missing key, got %q", val)
	}
}

func TestDurableObjectStore_PutUpsert(t *testing.T) {
	db := setupDurableTestDB(t)
	store := &GORMDurableObjectStore{DB: db}

	if err := store.Put("ns1", "obj1", "key1", "v1"); err != nil {
		t.Fatalf("Put v1: %v", err)
	}
	if err := store.Put("ns1", "obj1", "key1", "v2"); err != nil {
		t.Fatalf("Put v2: %v", err)
	}

	val, err := store.Get("ns1", "obj1", "key1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if val != "v2" {
		t.Errorf("expected upserted value %q, got %q", "v2", val)
	}
}

func TestDurableObjectStore_GetMulti(t *testing.T) {
	db := setupDurableTestDB(t)
	store := &GORMDurableObjectStore{DB: db}

	store.Put("ns1", "obj1", "a", "1")
	store.Put("ns1", "obj1", "b", "2")
	store.Put("ns1", "obj1", "c", "3")

	result, err := store.GetMulti("ns1", "obj1", []string{"a", "c"})
	if err != nil {
		t.Fatalf("GetMulti: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("expected 2 results, got %d", len(result))
	}
	if result["a"] != "1" {
		t.Errorf("a = %q, want %q", result["a"], "1")
	}
	if result["c"] != "3" {
		t.Errorf("c = %q, want %q", result["c"], "3")
	}
}

func TestDurableObjectStore_GetMultiEmpty(t *testing.T) {
	db := setupDurableTestDB(t)
	store := &GORMDurableObjectStore{DB: db}

	result, err := store.GetMulti("ns1", "obj1", []string{"nonexistent"})
	if err != nil {
		t.Fatalf("GetMulti: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("expected empty map, got %d entries", len(result))
	}
}

func TestDurableObjectStore_PutMulti(t *testing.T) {
	db := setupDurableTestDB(t)
	store := &GORMDurableObjectStore{DB: db}

	entries := map[string]string{
		"x": "10",
		"y": "20",
		"z": "30",
	}
	if err := store.PutMulti("ns1", "obj1", entries); err != nil {
		t.Fatalf("PutMulti: %v", err)
	}

	for k, want := range entries {
		got, err := store.Get("ns1", "obj1", k)
		if err != nil {
			t.Fatalf("Get(%q): %v", k, err)
		}
		if got != want {
			t.Errorf("Get(%q) = %q, want %q", k, got, want)
		}
	}
}

func TestDurableObjectStore_Delete(t *testing.T) {
	db := setupDurableTestDB(t)
	store := &GORMDurableObjectStore{DB: db}

	store.Put("ns1", "obj1", "key1", "value")

	if err := store.Delete("ns1", "obj1", "key1"); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	val, err := store.Get("ns1", "obj1", "key1")
	if err != nil {
		t.Fatalf("Get after delete: %v", err)
	}
	if val != "" {
		t.Errorf("expected empty after delete, got %q", val)
	}
}

func TestDurableObjectStore_DeleteMulti(t *testing.T) {
	db := setupDurableTestDB(t)
	store := &GORMDurableObjectStore{DB: db}

	store.Put("ns1", "obj1", "a", "1")
	store.Put("ns1", "obj1", "b", "2")
	store.Put("ns1", "obj1", "c", "3")

	count, err := store.DeleteMulti("ns1", "obj1", []string{"a", "c"})
	if err != nil {
		t.Fatalf("DeleteMulti: %v", err)
	}
	if count != 2 {
		t.Errorf("deleted %d, want 2", count)
	}

	// "b" should still exist.
	val, err := store.Get("ns1", "obj1", "b")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if val != "2" {
		t.Errorf("b = %q, want %q", val, "2")
	}
}

func TestDurableObjectStore_DeleteAll(t *testing.T) {
	db := setupDurableTestDB(t)
	store := &GORMDurableObjectStore{DB: db}

	store.Put("ns1", "obj1", "a", "1")
	store.Put("ns1", "obj1", "b", "2")

	if err := store.DeleteAll("ns1", "obj1"); err != nil {
		t.Fatalf("DeleteAll: %v", err)
	}

	result, err := store.GetMulti("ns1", "obj1", []string{"a", "b"})
	if err != nil {
		t.Fatalf("GetMulti: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("expected empty after DeleteAll, got %d", len(result))
	}
}

func TestDurableObjectStore_List(t *testing.T) {
	db := setupDurableTestDB(t)
	store := &GORMDurableObjectStore{DB: db}

	store.Put("ns1", "obj1", "alpha", "1")
	store.Put("ns1", "obj1", "beta", "2")
	store.Put("ns1", "obj1", "gamma", "3")

	pairs, err := store.List("ns1", "obj1", "", 10, false)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(pairs) != 3 {
		t.Fatalf("expected 3 pairs, got %d", len(pairs))
	}
	// Should be sorted ASC.
	if pairs[0].Key != "alpha" || pairs[1].Key != "beta" || pairs[2].Key != "gamma" {
		t.Errorf("unexpected order: %v", pairs)
	}
}

func TestDurableObjectStore_ListWithPrefix(t *testing.T) {
	db := setupDurableTestDB(t)
	store := &GORMDurableObjectStore{DB: db}

	store.Put("ns1", "obj1", "user:1", "a")
	store.Put("ns1", "obj1", "user:2", "b")
	store.Put("ns1", "obj1", "item:1", "c")

	pairs, err := store.List("ns1", "obj1", "user:", 10, false)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(pairs) != 2 {
		t.Errorf("expected 2 pairs with prefix 'user:', got %d", len(pairs))
	}
}

func TestDurableObjectStore_ListReverse(t *testing.T) {
	db := setupDurableTestDB(t)
	store := &GORMDurableObjectStore{DB: db}

	store.Put("ns1", "obj1", "a", "1")
	store.Put("ns1", "obj1", "b", "2")
	store.Put("ns1", "obj1", "c", "3")

	pairs, err := store.List("ns1", "obj1", "", 10, true)
	if err != nil {
		t.Fatalf("List reverse: %v", err)
	}
	if len(pairs) != 3 {
		t.Fatalf("expected 3, got %d", len(pairs))
	}
	if pairs[0].Key != "c" || pairs[1].Key != "b" || pairs[2].Key != "a" {
		t.Errorf("unexpected reverse order: %v", pairs)
	}
}

func TestDurableObjectStore_ListDefaultLimit(t *testing.T) {
	db := setupDurableTestDB(t)
	store := &GORMDurableObjectStore{DB: db}

	store.Put("ns1", "obj1", "a", "1")

	// limit <= 0 should default to 128.
	pairs, err := store.List("ns1", "obj1", "", 0, false)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(pairs) != 1 {
		t.Errorf("expected 1 pair, got %d", len(pairs))
	}
}

func TestDurableObjectStore_NamespaceIsolation(t *testing.T) {
	db := setupDurableTestDB(t)
	store := &GORMDurableObjectStore{DB: db}

	store.Put("ns1", "obj1", "key", "ns1-value")
	store.Put("ns2", "obj1", "key", "ns2-value")

	val, err := store.Get("ns1", "obj1", "key")
	if err != nil {
		t.Fatalf("Get ns1: %v", err)
	}
	if val != "ns1-value" {
		t.Errorf("ns1 value = %q, want %q", val, "ns1-value")
	}

	val, err = store.Get("ns2", "obj1", "key")
	if err != nil {
		t.Fatalf("Get ns2: %v", err)
	}
	if val != "ns2-value" {
		t.Errorf("ns2 value = %q, want %q", val, "ns2-value")
	}
}
