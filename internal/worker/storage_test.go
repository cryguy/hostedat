package worker

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cryguy/hostedat/internal/models"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// ---------------------------------------------------------------------------
// Test helpers for StorageBridge
// ---------------------------------------------------------------------------

func storageBridgeSetup(t *testing.T) (*StorageBridge, *gorm.DB) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Discard,
	})
	if err != nil {
		t.Fatalf("opening test db: %v", err)
	}
	db.Exec("PRAGMA foreign_keys = OFF")
	if err := db.AutoMigrate(&models.StorageObject{}); err != nil {
		t.Fatalf("auto-migrate: %v", err)
	}

	tmpDir := t.TempDir()
	bridge := &StorageBridge{
		DB:          db,
		SiteID:      "site-" + t.Name(),
		StoragePath: tmpDir,
	}
	return bridge, db
}

// ---------------------------------------------------------------------------
// Put tests
// ---------------------------------------------------------------------------

func TestStorage_Put_Basic(t *testing.T) {
	bridge, _ := storageBridgeSetup(t)

	result, err := bridge.Put("test.txt", []byte("hello world"), "text/plain", nil)
	if err != nil {
		t.Fatalf("Put: %v", err)
	}

	if result["key"] != "test.txt" {
		t.Errorf("key = %v, want test.txt", result["key"])
	}
	if result["size"] != 11 {
		t.Errorf("size = %v, want 11", result["size"])
	}

	// ETag should be quoted MD5.
	md5Sum := md5.Sum([]byte("hello world"))
	expectedETag := fmt.Sprintf(`"%s"`, hex.EncodeToString(md5Sum[:]))
	if result["etag"] != expectedETag {
		t.Errorf("etag = %v, want %v", result["etag"], expectedETag)
	}

	httpMeta := result["httpMetadata"].(map[string]interface{})
	if httpMeta["contentType"] != "text/plain" {
		t.Errorf("contentType = %v", httpMeta["contentType"])
	}
}

func TestStorage_Put_DefaultContentType(t *testing.T) {
	bridge, _ := storageBridgeSetup(t)

	result, err := bridge.Put("data.bin", []byte("binary"), "", nil)
	if err != nil {
		t.Fatalf("Put: %v", err)
	}

	httpMeta := result["httpMetadata"].(map[string]interface{})
	if httpMeta["contentType"] != "application/octet-stream" {
		t.Errorf("contentType = %v, want application/octet-stream", httpMeta["contentType"])
	}
}

func TestStorage_Put_WithMetadata(t *testing.T) {
	bridge, _ := storageBridgeSetup(t)

	meta := map[string]string{"author": "alice", "version": "2"}
	result, err := bridge.Put("doc.txt", []byte("content"), "text/plain", meta)
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	if result["key"] != "doc.txt" {
		t.Errorf("key = %v", result["key"])
	}

	// Verify metadata is retrievable via Get.
	info, _, err := bridge.Get("doc.txt")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	customMeta := info["customMetadata"].(map[string]string)
	if customMeta["author"] != "alice" {
		t.Errorf("author = %v", customMeta["author"])
	}
	if customMeta["version"] != "2" {
		t.Errorf("version = %v", customMeta["version"])
	}
}

func TestStorage_Put_Overwrite(t *testing.T) {
	bridge, _ := storageBridgeSetup(t)

	bridge.Put("key.txt", []byte("v1"), "text/plain", nil)
	result, err := bridge.Put("key.txt", []byte("v2-updated"), "text/html", nil)
	if err != nil {
		t.Fatalf("Put overwrite: %v", err)
	}

	if result["size"] != 10 {
		t.Errorf("size = %v, want 10", result["size"])
	}

	// Verify content updated.
	info, body, err := bridge.Get("key.txt")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if string(body) != "v2-updated" {
		t.Errorf("body = %q, want v2-updated", body)
	}
	httpMeta := info["httpMetadata"].(map[string]interface{})
	if httpMeta["contentType"] != "text/html" {
		t.Errorf("contentType = %v, want text/html", httpMeta["contentType"])
	}
}

// ---------------------------------------------------------------------------
// Get tests
// ---------------------------------------------------------------------------

func TestStorage_Get_Basic(t *testing.T) {
	bridge, _ := storageBridgeSetup(t)

	bridge.Put("hello.txt", []byte("hello!"), "text/plain", nil)

	info, body, err := bridge.Get("hello.txt")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if info == nil {
		t.Fatal("info is nil")
	}
	if string(body) != "hello!" {
		t.Errorf("body = %q, want hello!", body)
	}
	if info["key"] != "hello.txt" {
		t.Errorf("key = %v", info["key"])
	}
	if info["size"] != int64(6) {
		t.Errorf("size = %v, want 6", info["size"])
	}
}

func TestStorage_Get_NotFound(t *testing.T) {
	bridge, _ := storageBridgeSetup(t)

	info, body, err := bridge.Get("nonexistent.txt")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if info != nil {
		t.Errorf("info should be nil for missing key, got %v", info)
	}
	if body != nil {
		t.Errorf("body should be nil for missing key, got %v", body)
	}
}

// ---------------------------------------------------------------------------
// Head tests
// ---------------------------------------------------------------------------

func TestStorage_Head_Basic(t *testing.T) {
	bridge, _ := storageBridgeSetup(t)

	bridge.Put("meta.txt", []byte("metadata test"), "text/plain", map[string]string{"tag": "v1"})

	info, err := bridge.Head("meta.txt")
	if err != nil {
		t.Fatalf("Head: %v", err)
	}
	if info == nil {
		t.Fatal("info is nil")
	}
	if info["key"] != "meta.txt" {
		t.Errorf("key = %v", info["key"])
	}
	if info["size"] != int64(13) {
		t.Errorf("size = %v, want 13", info["size"])
	}
	customMeta := info["customMetadata"].(map[string]string)
	if customMeta["tag"] != "v1" {
		t.Errorf("tag = %v", customMeta["tag"])
	}
}

func TestStorage_Head_NotFound(t *testing.T) {
	bridge, _ := storageBridgeSetup(t)

	info, err := bridge.Head("nope.txt")
	if err != nil {
		t.Fatalf("Head: %v", err)
	}
	if info != nil {
		t.Errorf("info should be nil for missing key, got %v", info)
	}
}

// ---------------------------------------------------------------------------
// Delete tests
// ---------------------------------------------------------------------------

func TestStorage_Delete_Basic(t *testing.T) {
	bridge, _ := storageBridgeSetup(t)

	bridge.Put("del.txt", []byte("to delete"), "text/plain", nil)

	if err := bridge.Delete("del.txt"); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	info, body, _ := bridge.Get("del.txt")
	if info != nil || body != nil {
		t.Error("object should be deleted")
	}
}

func TestStorage_Delete_Idempotent(t *testing.T) {
	bridge, _ := storageBridgeSetup(t)

	// Deleting a nonexistent key should not error.
	if err := bridge.Delete("nothing.txt"); err != nil {
		t.Fatalf("Delete nonexistent: %v", err)
	}
}

func TestStorage_Delete_CleansUpFile(t *testing.T) {
	bridge, _ := storageBridgeSetup(t)

	bridge.Put("cleanup.txt", []byte("data"), "text/plain", nil)

	// Find the storage path.
	var obj models.StorageObject
	bridge.DB.Where("site_id = ? AND key = ?", bridge.SiteID, "cleanup.txt").First(&obj)
	storagePath := obj.StoragePath

	// Verify file exists.
	if _, err := os.Stat(storagePath); os.IsNotExist(err) {
		t.Fatal("storage file should exist before delete")
	}

	bridge.Delete("cleanup.txt")

	// Verify file removed.
	if _, err := os.Stat(storagePath); !os.IsNotExist(err) {
		t.Error("storage file should be removed after delete")
	}
}

// ---------------------------------------------------------------------------
// List tests
// ---------------------------------------------------------------------------

func TestStorage_List_Basic(t *testing.T) {
	bridge, _ := storageBridgeSetup(t)

	bridge.Put("a.txt", []byte("a"), "text/plain", nil)
	bridge.Put("b.txt", []byte("b"), "text/plain", nil)
	bridge.Put("c.txt", []byte("c"), "text/plain", nil)

	result, err := bridge.List("", "", "", 0)
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	objects := result["objects"].([]map[string]interface{})
	if len(objects) != 3 {
		t.Fatalf("object count = %d, want 3", len(objects))
	}
	if result["truncated"] != false {
		t.Errorf("truncated = %v, want false", result["truncated"])
	}
}

func TestStorage_List_Prefix(t *testing.T) {
	bridge, _ := storageBridgeSetup(t)

	bridge.Put("images/a.png", []byte("a"), "image/png", nil)
	bridge.Put("images/b.png", []byte("b"), "image/png", nil)
	bridge.Put("docs/c.txt", []byte("c"), "text/plain", nil)

	result, err := bridge.List("images/", "", "", 0)
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	objects := result["objects"].([]map[string]interface{})
	if len(objects) != 2 {
		t.Errorf("object count = %d, want 2", len(objects))
	}
}

func TestStorage_List_Delimiter(t *testing.T) {
	bridge, _ := storageBridgeSetup(t)

	bridge.Put("photos/2024/a.jpg", []byte("a"), "image/jpeg", nil)
	bridge.Put("photos/2024/b.jpg", []byte("b"), "image/jpeg", nil)
	bridge.Put("photos/2025/c.jpg", []byte("c"), "image/jpeg", nil)
	bridge.Put("photos/root.jpg", []byte("d"), "image/jpeg", nil)

	result, err := bridge.List("photos/", "/", "", 0)
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	// root.jpg should be in objects.
	objects := result["objects"].([]map[string]interface{})
	if len(objects) != 1 {
		t.Errorf("object count = %d, want 1 (root.jpg)", len(objects))
	}
	if len(objects) > 0 && objects[0]["key"] != "photos/root.jpg" {
		t.Errorf("object key = %v", objects[0]["key"])
	}

	// 2024/ and 2025/ should be in delimitedPrefixes.
	prefixes := result["delimitedPrefixes"].([]string)
	if len(prefixes) != 2 {
		t.Errorf("prefix count = %d, want 2", len(prefixes))
	}
}

func TestStorage_List_Pagination(t *testing.T) {
	bridge, _ := storageBridgeSetup(t)

	for i := 0; i < 5; i++ {
		bridge.Put(fmt.Sprintf("key-%02d", i), []byte("data"), "text/plain", nil)
	}

	// Page 1: limit 2
	result1, err := bridge.List("", "", "", 2)
	if err != nil {
		t.Fatalf("List page 1: %v", err)
	}
	if result1["truncated"] != true {
		t.Errorf("page 1 truncated = %v, want true", result1["truncated"])
	}
	objects1 := result1["objects"].([]map[string]interface{})
	if len(objects1) != 2 {
		t.Fatalf("page 1 count = %d, want 2", len(objects1))
	}
	cursor := result1["cursor"].(string)

	// Page 2: use cursor
	result2, err := bridge.List("", "", cursor, 2)
	if err != nil {
		t.Fatalf("List page 2: %v", err)
	}
	objects2 := result2["objects"].([]map[string]interface{})
	if len(objects2) != 2 {
		t.Fatalf("page 2 count = %d, want 2", len(objects2))
	}

	// Page 3: last page
	cursor2 := result2["cursor"].(string)
	result3, err := bridge.List("", "", cursor2, 2)
	if err != nil {
		t.Fatalf("List page 3: %v", err)
	}
	objects3 := result3["objects"].([]map[string]interface{})
	if len(objects3) != 1 {
		t.Errorf("page 3 count = %d, want 1", len(objects3))
	}
	if result3["truncated"] != false {
		t.Errorf("page 3 truncated = %v, want false", result3["truncated"])
	}
}

func TestStorage_List_Empty(t *testing.T) {
	bridge, _ := storageBridgeSetup(t)

	result, err := bridge.List("", "", "", 0)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	objects := result["objects"].([]map[string]interface{})
	if len(objects) != 0 {
		t.Errorf("object count = %d, want 0", len(objects))
	}
}

// ---------------------------------------------------------------------------
// Site isolation
// ---------------------------------------------------------------------------

func TestStorage_SiteIsolation(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Discard,
	})
	if err != nil {
		t.Fatalf("opening test db: %v", err)
	}
	db.Exec("PRAGMA foreign_keys = OFF")
	db.AutoMigrate(&models.StorageObject{})

	tmpDir := t.TempDir()

	bridge1 := &StorageBridge{DB: db, SiteID: "site-1", StoragePath: tmpDir}
	bridge2 := &StorageBridge{DB: db, SiteID: "site-2", StoragePath: tmpDir}

	bridge1.Put("shared-key", []byte("site1 data"), "text/plain", nil)
	bridge2.Put("shared-key", []byte("site2 data"), "text/plain", nil)

	_, body1, _ := bridge1.Get("shared-key")
	_, body2, _ := bridge2.Get("shared-key")

	if string(body1) != "site1 data" {
		t.Errorf("site1 body = %q", body1)
	}
	if string(body2) != "site2 data" {
		t.Errorf("site2 body = %q", body2)
	}

	// List should only show each site's own objects.
	result1, _ := bridge1.List("", "", "", 0)
	result2, _ := bridge2.List("", "", "", 0)
	if len(result1["objects"].([]map[string]interface{})) != 1 {
		t.Errorf("site1 list count = %d, want 1", len(result1["objects"].([]map[string]interface{})))
	}
	if len(result2["objects"].([]map[string]interface{})) != 1 {
		t.Errorf("site2 list count = %d, want 1", len(result2["objects"].([]map[string]interface{})))
	}
}

// ---------------------------------------------------------------------------
// Overwrite cleans up old file
// ---------------------------------------------------------------------------

func TestStorage_Overwrite_CleansUpOldFile(t *testing.T) {
	bridge, _ := storageBridgeSetup(t)

	bridge.Put("replace.txt", []byte("original"), "text/plain", nil)

	var obj1 models.StorageObject
	bridge.DB.Where("site_id = ? AND key = ?", bridge.SiteID, "replace.txt").First(&obj1)
	oldPath := obj1.StoragePath

	bridge.Put("replace.txt", []byte("replacement"), "text/plain", nil)

	// Old file should be removed.
	if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
		t.Error("old storage file should be removed after overwrite")
	}

	// New content accessible.
	_, body, _ := bridge.Get("replace.txt")
	if string(body) != "replacement" {
		t.Errorf("body = %q", body)
	}
}

// ---------------------------------------------------------------------------
// ETag format
// ---------------------------------------------------------------------------

func TestStorage_ETag_Format(t *testing.T) {
	bridge, _ := storageBridgeSetup(t)

	result, _ := bridge.Put("etag.txt", []byte("test"), "text/plain", nil)
	etag := result["etag"].(string)

	if !strings.HasPrefix(etag, `"`) || !strings.HasSuffix(etag, `"`) {
		t.Errorf("ETag should be quoted, got %s", etag)
	}
}

// ---------------------------------------------------------------------------
// writeFile
// ---------------------------------------------------------------------------

func TestStorage_WriteFile(t *testing.T) {
	bridge, _ := storageBridgeSetup(t)

	path, err := bridge.writeFile([]byte("test data"))
	if err != nil {
		t.Fatalf("writeFile: %v", err)
	}

	// Verify path is within expected directory.
	expectedDir := filepath.Join(bridge.StoragePath, "objects", bridge.SiteID)
	if !strings.HasPrefix(path, expectedDir) {
		t.Errorf("path %q not under expected dir %q", path, expectedDir)
	}

	// Verify content.
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading file: %v", err)
	}
	if string(data) != "test data" {
		t.Errorf("data = %q", data)
	}
}
