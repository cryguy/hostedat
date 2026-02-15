package worker

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/cryguy/hostedat/internal/models"
	"github.com/fastschema/qjs"
	"gorm.io/gorm"
)

// KVBridge provides Go methods that back the KV namespace JS bindings.
type KVBridge struct {
	DB          *gorm.DB
	NamespaceID string
}

// Get retrieves a value by key, returning "" if not found or expired.
func (kv *KVBridge) Get(key string) (string, error) {
	var entry models.KVEntry
	err := kv.DB.Where("namespace_id = ? AND key = ?", kv.NamespaceID, key).First(&entry).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return "", nil
		}
		return "", err
	}

	// Check expiration.
	if entry.ExpiresAt != nil && entry.ExpiresAt.Before(time.Now()) {
		kv.DB.Delete(&entry)
		return "", nil
	}

	return entry.Value, nil
}

// maxKVValueSize is the maximum size of a KV value (1 MB).
const maxKVValueSize = 1 << 20

// Put upserts a key-value pair with optional metadata and TTL (seconds).
func (kv *KVBridge) Put(key, value string, metadata *string, ttl *int) error {
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
		// Update existing.
		return kv.DB.Model(&existing).Updates(map[string]interface{}{
			"value":      value,
			"metadata":   metadata,
			"expires_at": expiresAt,
		}).Error
	}
	if err != gorm.ErrRecordNotFound {
		return err
	}

	// Insert new.
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
func (kv *KVBridge) Delete(key string) error {
	return kv.DB.Where("namespace_id = ? AND key = ?", kv.NamespaceID, key).
		Delete(&models.KVEntry{}).Error
}

// List returns keys (and metadata) matching a prefix, up to limit entries.
func (kv *KVBridge) List(prefix string, limit int) ([]map[string]interface{}, error) {
	if limit <= 0 {
		limit = 1000
	}

	var entries []models.KVEntry
	q := kv.DB.Where("namespace_id = ?", kv.NamespaceID)
	if prefix != "" {
		q = q.Where("key LIKE ?", prefix+"%")
	}
	q = q.Where("expires_at IS NULL OR expires_at > ?", time.Now())
	if err := q.Limit(limit).Find(&entries).Error; err != nil {
		return nil, err
	}

	results := make([]map[string]interface{}, 0, len(entries))
	for _, e := range entries {
		item := map[string]interface{}{
			"name": e.Key,
		}
		if e.Metadata != nil {
			item["metadata"] = *e.Metadata
		}
		results = append(results, item)
	}

	return results, nil
}

// buildKVBinding creates a JS object with async get/put/delete/list methods
// backed by the given KVBridge.
func buildKVBinding(ctx *qjs.Context, bridge *KVBridge) *qjs.Value {
	kv := ctx.NewObject()

	// get(key) -> Promise<string|null>
	getFn := ctx.Function(func(this *qjs.This) (*qjs.Value, error) {
		c := this.Context()
		args := this.Args()
		promise := this.Promise()
		if len(args) == 0 {
			go func() { promise.Reject(c.NewError(fmt.Errorf("KV.get requires a key argument"))) }()
			return c.NewUndefined(), nil
		}
		key := args[0].String()
		go func() {
			val, err := bridge.Get(key)
			if err != nil {
				promise.Reject(c.NewError(err))
				return
			}
			if val == "" {
				promise.Resolve(c.NewNull())
				return
			}
			promise.Resolve(c.NewString(val))
		}()
		return c.NewUndefined(), nil
	}, true)
	kv.SetPropertyStr("get", getFn)

	// put(key, value, options?) -> Promise<void>
	putFn := ctx.Function(func(this *qjs.This) (*qjs.Value, error) {
		c := this.Context()
		args := this.Args()
		promise := this.Promise()
		if len(args) < 2 {
			go func() { promise.Reject(c.NewError(fmt.Errorf("KV.put requires key and value arguments"))) }()
			return c.NewUndefined(), nil
		}
		key := args[0].String()
		value := args[1].String()

		var metadata *string
		var ttl *int
		if len(args) > 2 && args[2].IsObject() {
			opts := args[2]
			metaVal := opts.GetPropertyStr("metadata")
			if !metaVal.IsUndefined() && !metaVal.IsNull() {
				m := metaVal.String()
				metadata = &m
			}
			metaVal.Free()

			ttlVal := opts.GetPropertyStr("expirationTtl")
			if !ttlVal.IsUndefined() && !ttlVal.IsNull() {
				t := int(ttlVal.Int32())
				ttl = &t
			}
			ttlVal.Free()
		}

		go func() {
			if err := bridge.Put(key, value, metadata, ttl); err != nil {
				promise.Reject(c.NewError(err))
				return
			}
			promise.Resolve(c.NewUndefined())
		}()
		return c.NewUndefined(), nil
	}, true)
	kv.SetPropertyStr("put", putFn)

	// delete(key) -> Promise<void>
	delFn := ctx.Function(func(this *qjs.This) (*qjs.Value, error) {
		c := this.Context()
		args := this.Args()
		promise := this.Promise()
		if len(args) == 0 {
			go func() { promise.Reject(c.NewError(fmt.Errorf("KV.delete requires a key argument"))) }()
			return c.NewUndefined(), nil
		}
		key := args[0].String()
		go func() {
			if err := bridge.Delete(key); err != nil {
				promise.Reject(c.NewError(err))
				return
			}
			promise.Resolve(c.NewUndefined())
		}()
		return c.NewUndefined(), nil
	}, true)
	kv.SetPropertyStr("delete", delFn)

	// list(options?) -> Promise<{keys: [{name, metadata?}]}>
	listFn := ctx.Function(func(this *qjs.This) (*qjs.Value, error) {
		c := this.Context()
		args := this.Args()
		promise := this.Promise()

		var prefix string
		limit := 1000
		if len(args) > 0 && args[0].IsObject() {
			opts := args[0]
			pVal := opts.GetPropertyStr("prefix")
			if !pVal.IsUndefined() && !pVal.IsNull() {
				prefix = pVal.String()
			}
			pVal.Free()

			lVal := opts.GetPropertyStr("limit")
			if !lVal.IsUndefined() && !lVal.IsNull() {
				limit = int(lVal.Int32())
			}
			lVal.Free()
		}

		go func() {
			entries, err := bridge.List(prefix, limit)
			if err != nil {
				promise.Reject(c.NewError(err))
				return
			}

			data, _ := json.Marshal(map[string]interface{}{
				"keys": entries,
			})
			promise.Resolve(c.ParseJSON(string(data)))
		}()
		return c.NewUndefined(), nil
	}, true)
	kv.SetPropertyStr("list", listFn)

	return kv
}
