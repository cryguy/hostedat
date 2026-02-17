package worker

import (
	"fmt"
	"testing"
	"time"
)

func TestKVBridge_PutAndGet(t *testing.T) {
	db := testDB(t)
	kv := &KVBridge{DB: db, NamespaceID: "test-ns"}

	if err := kv.Put("greeting", "hello", nil, nil); err != nil {
		t.Fatalf("Put: %v", err)
	}

	val, err := kv.Get("greeting")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if val != "hello" {
		t.Errorf("Get = %q, want %q", val, "hello")
	}
}

func TestKVBridge_GetNotFound(t *testing.T) {
	db := testDB(t)
	kv := &KVBridge{DB: db, NamespaceID: "test-ns"}

	val, err := kv.Get("nonexistent")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if val != "" {
		t.Errorf("Get = %q, want empty string", val)
	}
}

func TestKVBridge_GetExpired(t *testing.T) {
	db := testDB(t)
	kv := &KVBridge{DB: db, NamespaceID: "test-ns-expired"}

	ttl := 1 // 1 second
	if err := kv.Put("expiring", "gone-soon", nil, &ttl); err != nil {
		t.Fatalf("Put: %v", err)
	}

	time.Sleep(2 * time.Second)

	val, err := kv.Get("expiring")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if val != "" {
		t.Errorf("Get expired key = %q, want empty", val)
	}
}

func TestKVBridge_Delete(t *testing.T) {
	db := testDB(t)
	kv := &KVBridge{DB: db, NamespaceID: "test-ns-delete"}

	if err := kv.Put("key", "value", nil, nil); err != nil {
		t.Fatalf("Put: %v", err)
	}

	if err := kv.Delete("key"); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	val, err := kv.Get("key")
	if err != nil {
		t.Fatalf("Get after delete: %v", err)
	}
	if val != "" {
		t.Errorf("Get after delete = %q, want empty", val)
	}
}

func TestKVBridge_ListWithPrefix(t *testing.T) {
	db := testDB(t)
	kv := &KVBridge{DB: db, NamespaceID: "test-ns-prefix"}

	kv.Put("user:1", "alice", nil, nil)
	kv.Put("user:2", "bob", nil, nil)
	kv.Put("other:1", "nope", nil, nil)

	results, err := kv.List("user:", 0)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("List count = %d, want 2", len(results))
	}
}

func TestKVBridge_ListWithLimit(t *testing.T) {
	db := testDB(t)
	kv := &KVBridge{DB: db, NamespaceID: "test-ns-limit"}

	for i := 0; i < 5; i++ {
		kv.Put(fmt.Sprintf("k%d", i), fmt.Sprintf("v%d", i), nil, nil)
	}

	results, err := kv.List("", 2)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("List count = %d, want 2", len(results))
	}
}

func TestKVBridge_PutWithMetadata(t *testing.T) {
	db := testDB(t)
	kv := &KVBridge{DB: db, NamespaceID: "test-ns-meta"}

	meta := "some-metadata"
	if err := kv.Put("key", "value", &meta, nil); err != nil {
		t.Fatalf("Put: %v", err)
	}

	results, err := kv.List("key", 0)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("List count = %d, want 1", len(results))
	}
	if results[0]["metadata"] != "some-metadata" {
		t.Errorf("metadata = %v, want %q", results[0]["metadata"], "some-metadata")
	}
}

func TestKVBridge_PutOverwrite(t *testing.T) {
	db := testDB(t)
	kv := &KVBridge{DB: db, NamespaceID: "test-ns-overwrite"}

	if err := kv.Put("key", "v1", nil, nil); err != nil {
		t.Fatalf("Put v1: %v", err)
	}
	if err := kv.Put("key", "v2", nil, nil); err != nil {
		t.Fatalf("Put v2: %v", err)
	}

	val, err := kv.Get("key")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if val != "v2" {
		t.Errorf("Get = %q, want %q", val, "v2")
	}
}
