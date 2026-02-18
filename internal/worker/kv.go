package worker

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/cryguy/hostedat/internal/models"
	v8 "github.com/tommie/v8go"
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
		return kv.DB.Model(&existing).Updates(map[string]interface{}{
			"value":      value,
			"metadata":   metadata,
			"expires_at": expiresAt,
		}).Error
	}
	if err != gorm.ErrRecordNotFound {
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
//
// All operations run synchronously on the JS thread. SQLite queries are fast
// local I/O, so synchronous execution within a PromiseResolver is fine.
func buildKVBinding(iso *v8.Isolate, ctx *v8.Context, bridge *KVBridge) (*v8.Value, error) {
	kv, err := newJSObject(iso, ctx)
	if err != nil {
		return nil, fmt.Errorf("creating KV object: %w", err)
	}

	// get(key) -> Promise<string|null>
	kv.Set("get", v8.NewFunctionTemplate(iso, func(info *v8.FunctionCallbackInfo) *v8.Value {
		resolver, _ := v8.NewPromiseResolver(ctx)
		args := info.Args()
		if len(args) == 0 {
			errVal, _ := v8.NewValue(iso, "KV.get requires a key argument")
			resolver.Reject(errVal)
			return resolver.GetPromise().Value
		}
		key := args[0].String()
		val, err := bridge.Get(key)
		if err != nil {
			errVal, _ := v8.NewValue(iso, err.Error())
			resolver.Reject(errVal)
			return resolver.GetPromise().Value
		}
		if val == "" {
			resolver.Resolve(v8.Null(iso))
		} else {
			strVal, _ := v8.NewValue(iso, val)
			resolver.Resolve(strVal)
		}
		return resolver.GetPromise().Value
	}).GetFunction(ctx))

	// put(key, value, options?) -> Promise<void>
	kv.Set("put", v8.NewFunctionTemplate(iso, func(info *v8.FunctionCallbackInfo) *v8.Value {
		resolver, _ := v8.NewPromiseResolver(ctx)
		args := info.Args()
		if len(args) < 2 {
			errVal, _ := v8.NewValue(iso, "KV.put requires key and value arguments")
			resolver.Reject(errVal)
			return resolver.GetPromise().Value
		}
		key := args[0].String()
		value := args[1].String()

		var metadata *string
		var ttl *int
		if len(args) > 2 && args[2].IsObject() {
			// Extract options via JS to avoid complex property iteration.
			ctx.Global().Set("__tmp_kv_opts", args[2])
			optsResult, err := ctx.RunScript(`(function() {
				var o = globalThis.__tmp_kv_opts;
				delete globalThis.__tmp_kv_opts;
				return JSON.stringify({
					metadata: o.metadata !== undefined && o.metadata !== null ? String(o.metadata) : null,
					expirationTtl: o.expirationTtl !== undefined && o.expirationTtl !== null ? Number(o.expirationTtl) : null,
				});
			})()`, "kv_opts.js")
			if err == nil {
				var opts struct {
					Metadata      *string `json:"metadata"`
					ExpirationTtl *int    `json:"expirationTtl"`
				}
				if json.Unmarshal([]byte(optsResult.String()), &opts) == nil {
					metadata = opts.Metadata
					ttl = opts.ExpirationTtl
				}
			}
		}

		if err := bridge.Put(key, value, metadata, ttl); err != nil {
			errVal, _ := v8.NewValue(iso, err.Error())
			resolver.Reject(errVal)
			return resolver.GetPromise().Value
		}
		resolver.Resolve(v8.Undefined(iso))
		return resolver.GetPromise().Value
	}).GetFunction(ctx))

	// delete(key) -> Promise<void>
	kv.Set("delete", v8.NewFunctionTemplate(iso, func(info *v8.FunctionCallbackInfo) *v8.Value {
		resolver, _ := v8.NewPromiseResolver(ctx)
		args := info.Args()
		if len(args) == 0 {
			errVal, _ := v8.NewValue(iso, "KV.delete requires a key argument")
			resolver.Reject(errVal)
			return resolver.GetPromise().Value
		}
		key := args[0].String()
		if err := bridge.Delete(key); err != nil {
			errVal, _ := v8.NewValue(iso, err.Error())
			resolver.Reject(errVal)
			return resolver.GetPromise().Value
		}
		resolver.Resolve(v8.Undefined(iso))
		return resolver.GetPromise().Value
	}).GetFunction(ctx))

	// list(options?) -> Promise<{keys: [{name, metadata?}]}>
	kv.Set("list", v8.NewFunctionTemplate(iso, func(info *v8.FunctionCallbackInfo) *v8.Value {
		resolver, _ := v8.NewPromiseResolver(ctx)
		args := info.Args()

		var prefix string
		limit := 1000
		if len(args) > 0 && args[0].IsObject() {
			ctx.Global().Set("__tmp_kv_list_opts", args[0])
			optsResult, err := ctx.RunScript(`(function() {
				var o = globalThis.__tmp_kv_list_opts;
				delete globalThis.__tmp_kv_list_opts;
				return JSON.stringify({
					prefix: o.prefix !== undefined && o.prefix !== null ? String(o.prefix) : "",
					limit: o.limit !== undefined && o.limit !== null ? Number(o.limit) : 1000,
				});
			})()`, "kv_list_opts.js")
			if err == nil {
				var opts struct {
					Prefix string `json:"prefix"`
					Limit  int    `json:"limit"`
				}
				if json.Unmarshal([]byte(optsResult.String()), &opts) == nil {
					prefix = opts.Prefix
					limit = opts.Limit
				}
			}
		}

		entries, err := bridge.List(prefix, limit)
		if err != nil {
			errVal, _ := v8.NewValue(iso, err.Error())
			resolver.Reject(errVal)
			return resolver.GetPromise().Value
		}

		data, _ := json.Marshal(map[string]interface{}{
			"keys": entries,
		})
		// Parse JSON into a JS object.
		jsResult, err := ctx.RunScript(fmt.Sprintf("JSON.parse(%q)", string(data)), "kv_list_result.js")
		if err != nil {
			errVal, _ := v8.NewValue(iso, err.Error())
			resolver.Reject(errVal)
			return resolver.GetPromise().Value
		}
		resolver.Resolve(jsResult)
		return resolver.GetPromise().Value
	}).GetFunction(ctx))

	return kv.Value, nil
}
