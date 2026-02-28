package workeradapter

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/cryguy/hostedat/internal/models"
	"github.com/cryguy/worker/v2"
	"gorm.io/gorm"
)

// Compile-time interface check.
var _ worker.KVStore = (*GORMKVStore)(nil)

// maxKVValueSize is the maximum size of a KV value (1 MB).
const maxKVValueSize = 1 << 20

// GORMKVStore implements worker.KVStore using GORM.
type GORMKVStore struct {
	DB          *gorm.DB
	NamespaceID string
}

// Get retrieves a value by key, returning nil if not found or expired.
func (kv *GORMKVStore) Get(key string) (*string, error) {
	var entry models.KVEntry
	err := kv.DB.Where("namespace_id = ? AND key = ?", kv.NamespaceID, key).First(&entry).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}

	if entry.ExpiresAt != nil && entry.ExpiresAt.Before(time.Now()) {
		kv.DB.Delete(&entry)
		return nil, nil
	}

	return &entry.Value, nil
}

// GetWithMetadata retrieves a value and its metadata by key.
func (kv *GORMKVStore) GetWithMetadata(key string) (*worker.KVValueWithMetadata, error) {
	var entry models.KVEntry
	err := kv.DB.Where("namespace_id = ? AND key = ?", kv.NamespaceID, key).First(&entry).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}

	if entry.ExpiresAt != nil && entry.ExpiresAt.Before(time.Now()) {
		kv.DB.Delete(&entry)
		return nil, nil
	}

	return &worker.KVValueWithMetadata{Value: entry.Value, Metadata: entry.Metadata}, nil
}

// Put upserts a key-value pair with optional metadata and TTL (seconds).
func (kv *GORMKVStore) Put(key, value string, metadata *string, ttl *int) error {
	if len(value) > maxKVValueSize {
		return fmt.Errorf("value exceeds maximum size of %d bytes", maxKVValueSize)
	}

	var expiresAt *time.Time
	if ttl != nil && *ttl > 0 {
		t := time.Now().Add(time.Duration(*ttl) * time.Second)
		expiresAt = &t
	}

	var existing models.KVEntry
	err := kv.DB.Where("namespace_id = ? AND key = ?", kv.NamespaceID, key).First(&existing).Error
	if err == nil {
		return kv.DB.Model(&existing).Updates(map[string]interface{}{
			"value":      value,
			"metadata":   metadata,
			"expires_at": expiresAt,
		}).Error
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}

	entry := models.KVEntry{
		NamespaceID: kv.NamespaceID,
		Key:         key,
		Value:       value,
		Metadata:    metadata,
		ExpiresAt:   expiresAt,
	}
	return kv.DB.Create(&entry).Error
}

// Delete removes a key from the namespace.
func (kv *GORMKVStore) Delete(key string) error {
	return kv.DB.Where("namespace_id = ? AND key = ?", kv.NamespaceID, key).
		Delete(&models.KVEntry{}).Error
}

// decodeCursor decodes a base64-encoded cursor to an integer offset.
func decodeCursor(cursor string) int {
	if cursor == "" {
		return 0
	}
	data, err := base64.StdEncoding.DecodeString(cursor)
	if err != nil {
		return 0
	}
	offset, err := strconv.Atoi(string(data))
	if err != nil {
		return 0
	}
	return offset
}

// encodeCursor encodes an integer offset to a base64 cursor string.
func encodeCursor(offset int) string {
	return base64.StdEncoding.EncodeToString([]byte(strconv.Itoa(offset)))
}

// List returns keys (and metadata) matching a prefix, up to limit entries,
// with cursor-based pagination support.
func (kv *GORMKVStore) List(prefix string, limit int, cursor string) (*worker.KVListResult, error) {
	if limit <= 0 {
		limit = 1000
	}

	offset := decodeCursor(cursor)

	var entries []models.KVEntry
	q := kv.DB.Where("namespace_id = ?", kv.NamespaceID)
	if prefix != "" {
		q = q.Where("key LIKE ?", prefix+"%")
	}
	q = q.Where("expires_at IS NULL OR expires_at > ?", time.Now())
	q = q.Order("key ASC")
	// Fetch one extra to determine if there are more results.
	if err := q.Offset(offset).Limit(limit + 1).Find(&entries).Error; err != nil {
		return nil, err
	}

	listComplete := len(entries) <= limit
	if !listComplete {
		entries = entries[:limit]
	}

	keys := make([]map[string]interface{}, 0, len(entries))
	for _, e := range entries {
		item := map[string]interface{}{
			"name": e.Key,
		}
		if e.Metadata != nil {
			// Embed metadata as raw JSON so it round-trips as a JS object,
			// matching Cloudflare's KV list() behavior.
			if json.Valid([]byte(*e.Metadata)) {
				item["metadata"] = json.RawMessage(*e.Metadata)
			} else {
				item["metadata"] = *e.Metadata
			}
		}
		keys = append(keys, item)
	}

	result := &worker.KVListResult{
		Keys:         keys,
		ListComplete: listComplete,
	}
	if !listComplete {
		result.Cursor = encodeCursor(offset + limit)
	}

	return result, nil
}
