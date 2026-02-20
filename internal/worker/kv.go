package worker

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
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
		if errors.Is(err, gorm.ErrRecordNotFound) {
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

// KVValueWithMetadata holds a value and its associated metadata.
type KVValueWithMetadata struct {
	Value    string
	Metadata *string
}

// GetWithMetadata retrieves a value and its metadata by key.
func (kv *KVBridge) GetWithMetadata(key string) (*KVValueWithMetadata, error) {
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

	return &KVValueWithMetadata{Value: entry.Value, Metadata: entry.Metadata}, nil
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
func (kv *KVBridge) Delete(key string) error {
	return kv.DB.Where("namespace_id = ? AND key = ?", kv.NamespaceID, key).
		Delete(&models.KVEntry{}).Error
}

// KVListResult holds the result of a List operation with pagination info.
type KVListResult struct {
	Keys         []map[string]interface{}
	ListComplete bool
	Cursor       string // base64-encoded offset, empty when list is complete
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
func (kv *KVBridge) List(prefix string, limit int, cursor string) (*KVListResult, error) {
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
			item["metadata"] = *e.Metadata
		}
		keys = append(keys, item)
	}

	result := &KVListResult{
		Keys:         keys,
		ListComplete: listComplete,
	}
	if !listComplete {
		result.Cursor = encodeCursor(offset + limit)
	}

	return result, nil
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

	// get(key, options?) -> Promise<string|object|ArrayBuffer|ReadableStream|null>
	// options.type: "text" (default), "json", "arrayBuffer", "stream"
	_ = kv.Set("get", v8.NewFunctionTemplate(iso, func(info *v8.FunctionCallbackInfo) *v8.Value {
		resolver, _ := v8.NewPromiseResolver(ctx)
		args := info.Args()
		if len(args) == 0 {
			errVal, _ := v8.NewValue(iso, "KV.get requires a key argument")
			resolver.Reject(errVal)
			return resolver.GetPromise().Value
		}
		key := args[0].String()

		// Parse optional type from second argument.
		valType := "text"
		if len(args) > 1 && args[1].IsObject() {
			_ = ctx.Global().Set("__tmp_kv_get_opts", args[1])
			optsResult, err := ctx.RunScript(`(function() {
				var o = globalThis.__tmp_kv_get_opts;
				delete globalThis.__tmp_kv_get_opts;
				return o.type !== undefined && o.type !== null ? String(o.type) : "text";
			})()`, "kv_get_opts.js")
			if err == nil {
				valType = optsResult.String()
			}
		}

		val, err := bridge.Get(key)
		if err != nil {
			errVal, _ := v8.NewValue(iso, err.Error())
			resolver.Reject(errVal)
			return resolver.GetPromise().Value
		}
		if val == "" {
			resolver.Resolve(v8.Null(iso))
			return resolver.GetPromise().Value
		}

		switch valType {
		case "json":
			jsVal, err := ctx.RunScript(fmt.Sprintf("JSON.parse(%q)", val), "kv_get_json.js")
			if err != nil {
				errVal, _ := v8.NewValue(iso, "KV.get: invalid JSON value")
				resolver.Reject(errVal)
			} else {
				resolver.Resolve(jsVal)
			}
		case "arrayBuffer":
			strVal, _ := v8.NewValue(iso, val)
			_ = ctx.Global().Set("__tmp_kv_ab_val", strVal)
			jsVal, err := ctx.RunScript(`(function() {
				var s = globalThis.__tmp_kv_ab_val;
				delete globalThis.__tmp_kv_ab_val;
				var enc = new TextEncoder();
				return enc.encode(s).buffer;
			})()`, "kv_get_arraybuffer.js")
			if err != nil {
				errVal, _ := v8.NewValue(iso, "KV.get: failed to create ArrayBuffer")
				resolver.Reject(errVal)
			} else {
				resolver.Resolve(jsVal)
			}
		case "stream":
			strVal, _ := v8.NewValue(iso, val)
			_ = ctx.Global().Set("__tmp_kv_stream_val", strVal)
			jsVal, err := ctx.RunScript(`(function() {
				var s = globalThis.__tmp_kv_stream_val;
				delete globalThis.__tmp_kv_stream_val;
				var enc = new TextEncoder();
				var bytes = enc.encode(s);
				return new ReadableStream({
					start(controller) {
						controller.enqueue(bytes);
						controller.close();
					}
				});
			})()`, "kv_get_stream.js")
			if err != nil {
				errVal, _ := v8.NewValue(iso, "KV.get: failed to create ReadableStream")
				resolver.Reject(errVal)
			} else {
				resolver.Resolve(jsVal)
			}
		default: // "text"
			strVal, _ := v8.NewValue(iso, val)
			resolver.Resolve(strVal)
		}
		return resolver.GetPromise().Value
	}).GetFunction(ctx))

	// getWithMetadata(key, options?) -> Promise<{value, metadata}>
	_ = kv.Set("getWithMetadata", v8.NewFunctionTemplate(iso, func(info *v8.FunctionCallbackInfo) *v8.Value {
		resolver, _ := v8.NewPromiseResolver(ctx)
		args := info.Args()
		if len(args) == 0 {
			errVal, _ := v8.NewValue(iso, "KV.getWithMetadata requires a key argument")
			resolver.Reject(errVal)
			return resolver.GetPromise().Value
		}
		key := args[0].String()

		// Parse optional type from second argument.
		valType := "text"
		if len(args) > 1 && args[1].IsObject() {
			_ = ctx.Global().Set("__tmp_kv_gwm_opts", args[1])
			optsResult, err := ctx.RunScript(`(function() {
				var o = globalThis.__tmp_kv_gwm_opts;
				delete globalThis.__tmp_kv_gwm_opts;
				return o.type !== undefined && o.type !== null ? String(o.type) : "text";
			})()`, "kv_gwm_opts.js")
			if err == nil {
				valType = optsResult.String()
			}
		}

		result, err := bridge.GetWithMetadata(key)
		if err != nil {
			errVal, _ := v8.NewValue(iso, err.Error())
			resolver.Reject(errVal)
			return resolver.GetPromise().Value
		}
		if result == nil {
			// Return {value: null, metadata: null}
			jsVal, _ := ctx.RunScript(`({value: null, metadata: null})`, "kv_gwm_null.js")
			resolver.Resolve(jsVal)
			return resolver.GetPromise().Value
		}

		// Build metadata JS value
		var metaJSON string
		if result.Metadata != nil {
			metaJSON = fmt.Sprintf("%q", *result.Metadata)
		} else {
			metaJSON = "null"
		}

		switch valType {
		case "json":
			jsVal, err := ctx.RunScript(fmt.Sprintf(`({value: JSON.parse(%q), metadata: %s})`, result.Value, metaJSON), "kv_gwm_json.js")
			if err != nil {
				errVal, _ := v8.NewValue(iso, "KV.getWithMetadata: invalid JSON value")
				resolver.Reject(errVal)
			} else {
				resolver.Resolve(jsVal)
			}
		case "arrayBuffer":
			strVal, _ := v8.NewValue(iso, result.Value)
			_ = ctx.Global().Set("__tmp_kv_gwm_ab", strVal)
			jsVal, err := ctx.RunScript(fmt.Sprintf(`(function() {
				var s = globalThis.__tmp_kv_gwm_ab;
				delete globalThis.__tmp_kv_gwm_ab;
				var enc = new TextEncoder();
				return {value: enc.encode(s).buffer, metadata: %s};
			})()`, metaJSON), "kv_gwm_arraybuffer.js")
			if err != nil {
				errVal, _ := v8.NewValue(iso, "KV.getWithMetadata: failed to create ArrayBuffer")
				resolver.Reject(errVal)
			} else {
				resolver.Resolve(jsVal)
			}
		case "stream":
			strVal, _ := v8.NewValue(iso, result.Value)
			_ = ctx.Global().Set("__tmp_kv_gwm_stream", strVal)
			jsVal, err := ctx.RunScript(fmt.Sprintf(`(function() {
				var s = globalThis.__tmp_kv_gwm_stream;
				delete globalThis.__tmp_kv_gwm_stream;
				var enc = new TextEncoder();
				var bytes = enc.encode(s);
				var stream = new ReadableStream({
					start(controller) {
						controller.enqueue(bytes);
						controller.close();
					}
				});
				return {value: stream, metadata: %s};
			})()`, metaJSON), "kv_gwm_stream.js")
			if err != nil {
				errVal, _ := v8.NewValue(iso, "KV.getWithMetadata: failed to create ReadableStream")
				resolver.Reject(errVal)
			} else {
				resolver.Resolve(jsVal)
			}
		default: // "text"
			jsVal, err := ctx.RunScript(fmt.Sprintf(`({value: %q, metadata: %s})`, result.Value, metaJSON), "kv_gwm_text.js")
			if err != nil {
				errVal, _ := v8.NewValue(iso, err.Error())
				resolver.Reject(errVal)
			} else {
				resolver.Resolve(jsVal)
			}
		}
		return resolver.GetPromise().Value
	}).GetFunction(ctx))

	// put(key, value, options?) -> Promise<void>
	_ = kv.Set("put", v8.NewFunctionTemplate(iso, func(info *v8.FunctionCallbackInfo) *v8.Value {
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
			_ = ctx.Global().Set("__tmp_kv_opts", args[2])
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
	_ = kv.Set("delete", v8.NewFunctionTemplate(iso, func(info *v8.FunctionCallbackInfo) *v8.Value {
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

	// list(options?) -> Promise<{keys: [{name, metadata?}], list_complete, cursor?}>
	_ = kv.Set("list", v8.NewFunctionTemplate(iso, func(info *v8.FunctionCallbackInfo) *v8.Value {
		resolver, _ := v8.NewPromiseResolver(ctx)
		args := info.Args()

		var prefix string
		var cursor string
		limit := 1000
		if len(args) > 0 && args[0].IsObject() {
			_ = ctx.Global().Set("__tmp_kv_list_opts", args[0])
			optsResult, err := ctx.RunScript(`(function() {
				var o = globalThis.__tmp_kv_list_opts;
				delete globalThis.__tmp_kv_list_opts;
				return JSON.stringify({
					prefix: o.prefix !== undefined && o.prefix !== null ? String(o.prefix) : "",
					limit: o.limit !== undefined && o.limit !== null ? Number(o.limit) : 1000,
					cursor: o.cursor !== undefined && o.cursor !== null ? String(o.cursor) : "",
				});
			})()`, "kv_list_opts.js")
			if err == nil {
				var opts struct {
					Prefix string `json:"prefix"`
					Limit  int    `json:"limit"`
					Cursor string `json:"cursor"`
				}
				if json.Unmarshal([]byte(optsResult.String()), &opts) == nil {
					prefix = opts.Prefix
					limit = opts.Limit
					cursor = opts.Cursor
				}
			}
		}

		listResult, err := bridge.List(prefix, limit, cursor)
		if err != nil {
			errVal, _ := v8.NewValue(iso, err.Error())
			resolver.Reject(errVal)
			return resolver.GetPromise().Value
		}

		result := map[string]interface{}{
			"keys":          listResult.Keys,
			"list_complete": listResult.ListComplete,
		}
		if listResult.Cursor != "" {
			result["cursor"] = listResult.Cursor
		}

		data, _ := json.Marshal(result)
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
