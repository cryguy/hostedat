package worker

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"

	v8 "github.com/tommie/v8go"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// DurableObjectEntry is the GORM model for durable object storage.
type DurableObjectEntry struct {
	ID        uint   `gorm:"primaryKey"`
	Namespace string `gorm:"uniqueIndex:idx_do_lookup"`
	ObjectID  string `gorm:"uniqueIndex:idx_do_lookup"`
	Key       string `gorm:"uniqueIndex:idx_do_lookup"`
	ValueJSON string
}

// DurableObjectBridge provides Go methods backing the Durable Object JS bindings.
type DurableObjectBridge struct {
	DB *gorm.DB
}

// Get retrieves a single value by key, returning "" if not found.
func (b *DurableObjectBridge) Get(namespace, objectID, key string) (string, error) {
	var entry DurableObjectEntry
	err := b.DB.Where("namespace = ? AND object_id = ? AND key = ?", namespace, objectID, key).First(&entry).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", nil
		}
		return "", err
	}
	return entry.ValueJSON, nil
}

// GetMulti retrieves multiple values by keys.
func (b *DurableObjectBridge) GetMulti(namespace, objectID string, keys []string) (map[string]string, error) {
	var entries []DurableObjectEntry
	err := b.DB.Where("namespace = ? AND object_id = ? AND key IN ?", namespace, objectID, keys).Find(&entries).Error
	if err != nil {
		return nil, err
	}
	result := make(map[string]string, len(entries))
	for _, e := range entries {
		result[e.Key] = e.ValueJSON
	}
	return result, nil
}

// Put upserts a single key-value pair.
func (b *DurableObjectBridge) Put(namespace, objectID, key, valueJSON string) error {
	entry := DurableObjectEntry{
		Namespace: namespace,
		ObjectID:  objectID,
		Key:       key,
		ValueJSON: valueJSON,
	}
	return b.DB.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "namespace"}, {Name: "object_id"}, {Name: "key"}},
		DoUpdates: clause.AssignmentColumns([]string{"value_json"}),
	}).Create(&entry).Error
}

// PutMulti upserts multiple key-value pairs.
func (b *DurableObjectBridge) PutMulti(namespace, objectID string, entries map[string]string) error {
	return b.DB.Transaction(func(tx *gorm.DB) error {
		for k, v := range entries {
			entry := DurableObjectEntry{
				Namespace: namespace,
				ObjectID:  objectID,
				Key:       k,
				ValueJSON: v,
			}
			if err := tx.Clauses(clause.OnConflict{
				Columns:   []clause.Column{{Name: "namespace"}, {Name: "object_id"}, {Name: "key"}},
				DoUpdates: clause.AssignmentColumns([]string{"value_json"}),
			}).Create(&entry).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// Delete removes a single key.
func (b *DurableObjectBridge) Delete(namespace, objectID, key string) error {
	return b.DB.Where("namespace = ? AND object_id = ? AND key = ?", namespace, objectID, key).
		Delete(&DurableObjectEntry{}).Error
}

// DeleteMulti removes multiple keys, returning the count deleted.
func (b *DurableObjectBridge) DeleteMulti(namespace, objectID string, keys []string) (int, error) {
	result := b.DB.Where("namespace = ? AND object_id = ? AND key IN ?", namespace, objectID, keys).
		Delete(&DurableObjectEntry{})
	return int(result.RowsAffected), result.Error
}

// DeleteAll removes all entries for a given object.
func (b *DurableObjectBridge) DeleteAll(namespace, objectID string) error {
	return b.DB.Where("namespace = ? AND object_id = ?", namespace, objectID).
		Delete(&DurableObjectEntry{}).Error
}

// List returns entries matching an optional prefix, with limit and reverse support.
func (b *DurableObjectBridge) List(namespace, objectID, prefix string, limit int, reverse bool) (map[string]string, error) {
	if limit <= 0 {
		limit = 128
	}

	q := b.DB.Where("namespace = ? AND object_id = ?", namespace, objectID)
	if prefix != "" {
		q = q.Where("key LIKE ?", prefix+"%")
	}
	if reverse {
		q = q.Order("key DESC")
	} else {
		q = q.Order("key ASC")
	}

	var entries []DurableObjectEntry
	if err := q.Limit(limit).Find(&entries).Error; err != nil {
		return nil, err
	}

	result := make(map[string]string, len(entries))
	for _, e := range entries {
		result[e.Key] = e.ValueJSON
	}
	return result, nil
}

// durableObjectID generates a deterministic hex ID from a namespace and name.
func durableObjectID(namespace, name string) string {
	h := sha256.New()
	h.Write([]byte(namespace))
	h.Write([]byte(":"))
	h.Write([]byte(name))
	return hex.EncodeToString(h.Sum(nil))
}

// durableObjectUniqueID generates a random 32-char hex ID.
func durableObjectUniqueID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// buildDurableObjectBinding creates a DurableObjectNamespace JS object with
// idFromName, idFromString, newUniqueId, and get methods.
func buildDurableObjectBinding(iso *v8.Isolate, ctx *v8.Context, bridge *DurableObjectBridge, namespace string) (*v8.Value, error) {
	ns, err := newJSObject(iso, ctx)
	if err != nil {
		return nil, fmt.Errorf("creating DO namespace object: %w", err)
	}

	// Helper: build a DurableObjectId JS object from a hex string.
	buildIdObject := func(hexID string) (*v8.Object, error) {
		idObj, err := newJSObject(iso, ctx)
		if err != nil {
			return nil, err
		}
		hexVal, _ := v8.NewValue(iso, hexID)
		_ = idObj.Set("_hex", hexVal)

		_ = idObj.Set("toString", v8.NewFunctionTemplate(iso, func(info *v8.FunctionCallbackInfo) *v8.Value {
			self := info.This()
			h, _ := self.Get("_hex")
			return h
		}).GetFunction(ctx))

		_ = idObj.Set("equals", v8.NewFunctionTemplate(iso, func(info *v8.FunctionCallbackInfo) *v8.Value {
			args := info.Args()
			if len(args) == 0 {
				val, _ := v8.NewValue(iso, false)
				return val
			}
			self := info.This()
			selfHex, _ := self.Get("_hex")
			otherObj := args[0].Object()
			if otherObj == nil {
				val, _ := v8.NewValue(iso, false)
				return val
			}
			otherHex, err := otherObj.Get("_hex")
			if err != nil {
				val, _ := v8.NewValue(iso, false)
				return val
			}
			val, _ := v8.NewValue(iso, selfHex.String() == otherHex.String())
			return val
		}).GetFunction(ctx))

		return idObj, nil
	}

	// Helper: build a DurableObjectStub from an id object.
	buildStub := func(hexID string, idObj *v8.Object) (*v8.Object, error) {
		stub, err := newJSObject(iso, ctx)
		if err != nil {
			return nil, err
		}

		_ = stub.Set("id", idObj)

		// stub.fetch() returns Response("ok")
		_ = stub.Set("fetch", v8.NewFunctionTemplate(iso, func(info *v8.FunctionCallbackInfo) *v8.Value {
			resolver, _ := v8.NewPromiseResolver(ctx)
			resp, err := ctx.RunScript(`new Response("ok")`, "do_stub_fetch.js")
			if err != nil {
				errVal, _ := v8.NewValue(iso, err.Error())
				resolver.Reject(errVal)
				return resolver.GetPromise().Value
			}
			resolver.Resolve(resp)
			return resolver.GetPromise().Value
		}).GetFunction(ctx))

		// Build storage object
		storage, err := buildDurableObjectStorage(iso, ctx, bridge, namespace, hexID)
		if err != nil {
			return nil, err
		}
		_ = stub.Set("storage", storage)

		return stub, nil
	}

	// namespace.idFromName(name) -> DurableObjectId
	_ = ns.Set("idFromName", v8.NewFunctionTemplate(iso, func(info *v8.FunctionCallbackInfo) *v8.Value {
		args := info.Args()
		if len(args) == 0 {
			errVal, _ := v8.NewValue(iso, "")
			return errVal
		}
		name := args[0].String()
		hexID := durableObjectID(namespace, name)
		idObj, err := buildIdObject(hexID)
		if err != nil {
			errVal, _ := v8.NewValue(iso, "")
			return errVal
		}
		return idObj.Value
	}).GetFunction(ctx))

	// namespace.idFromString(hex) -> DurableObjectId
	_ = ns.Set("idFromString", v8.NewFunctionTemplate(iso, func(info *v8.FunctionCallbackInfo) *v8.Value {
		args := info.Args()
		if len(args) == 0 {
			errVal, _ := v8.NewValue(iso, "")
			return errVal
		}
		hexID := args[0].String()
		idObj, err := buildIdObject(hexID)
		if err != nil {
			errVal, _ := v8.NewValue(iso, "")
			return errVal
		}
		return idObj.Value
	}).GetFunction(ctx))

	// namespace.newUniqueId() -> DurableObjectId
	_ = ns.Set("newUniqueId", v8.NewFunctionTemplate(iso, func(info *v8.FunctionCallbackInfo) *v8.Value {
		hexID, err := durableObjectUniqueID()
		if err != nil {
			errVal, _ := v8.NewValue(iso, "")
			return errVal
		}
		idObj, err := buildIdObject(hexID)
		if err != nil {
			errVal, _ := v8.NewValue(iso, "")
			return errVal
		}
		return idObj.Value
	}).GetFunction(ctx))

	// namespace.get(id) -> DurableObjectStub
	_ = ns.Set("get", v8.NewFunctionTemplate(iso, func(info *v8.FunctionCallbackInfo) *v8.Value {
		args := info.Args()
		if len(args) == 0 {
			errVal, _ := v8.NewValue(iso, "")
			return errVal
		}
		idObj := args[0].Object()
		if idObj == nil {
			errVal, _ := v8.NewValue(iso, "")
			return errVal
		}
		hexVal, err := idObj.Get("_hex")
		if err != nil {
			errVal, _ := v8.NewValue(iso, "")
			return errVal
		}
		hexID := hexVal.String()
		stub, err := buildStub(hexID, idObj)
		if err != nil {
			errVal, _ := v8.NewValue(iso, "")
			return errVal
		}
		return stub.Value
	}).GetFunction(ctx))

	return ns.Value, nil
}

// buildDurableObjectStorage creates the storage object with get/put/delete/deleteAll/list.
func buildDurableObjectStorage(iso *v8.Isolate, ctx *v8.Context, bridge *DurableObjectBridge, namespace, objectID string) (*v8.Object, error) {
	stor, err := newJSObject(iso, ctx)
	if err != nil {
		return nil, fmt.Errorf("creating DO storage object: %w", err)
	}

	// storage.get(key) or storage.get([keys]) -> Promise
	_ = stor.Set("get", v8.NewFunctionTemplate(iso, func(info *v8.FunctionCallbackInfo) *v8.Value {
		resolver, _ := v8.NewPromiseResolver(ctx)
		args := info.Args()
		if len(args) == 0 {
			errVal, _ := v8.NewValue(iso, "storage.get requires a key argument")
			resolver.Reject(errVal)
			return resolver.GetPromise().Value
		}

		arg := args[0]
		if arg.IsArray() {
			// Extract keys via JS
			_ = ctx.Global().Set("__tmp_do_keys", arg)
			keysResult, err := ctx.RunScript(`(function() {
				var a = globalThis.__tmp_do_keys;
				delete globalThis.__tmp_do_keys;
				return JSON.stringify(Array.from(a));
			})()`, "do_get_keys.js")
			if err != nil {
				errVal, _ := v8.NewValue(iso, err.Error())
				resolver.Reject(errVal)
				return resolver.GetPromise().Value
			}
			var keys []string
			_ = json.Unmarshal([]byte(keysResult.String()), &keys)
			result, err := bridge.GetMulti(namespace, objectID, keys)
			if err != nil {
				errVal, _ := v8.NewValue(iso, err.Error())
				resolver.Reject(errVal)
				return resolver.GetPromise().Value
			}
			// Return as a Map via JS
			data, _ := json.Marshal(result)
			jsResult, err := ctx.RunScript(fmt.Sprintf(`(function() {
				var obj = JSON.parse(%q);
				var m = new Map();
				for (var k in obj) { m.set(k, JSON.parse(obj[k])); }
				return m;
			})()`, string(data)), "do_get_multi_result.js")
			if err != nil {
				errVal, _ := v8.NewValue(iso, err.Error())
				resolver.Reject(errVal)
				return resolver.GetPromise().Value
			}
			resolver.Resolve(jsResult)
		} else {
			key := arg.String()
			val, err := bridge.Get(namespace, objectID, key)
			if err != nil {
				errVal, _ := v8.NewValue(iso, err.Error())
				resolver.Reject(errVal)
				return resolver.GetPromise().Value
			}
			if val == "" {
				resolver.Resolve(v8.Null(iso))
			} else {
				// Parse stored JSON value back to JS value via JSON.parse
				jsVal, err := ctx.RunScript(fmt.Sprintf("JSON.parse(%q)", val), "do_get_val.js")
				if err != nil {
					// If not valid JSON, return as string
					strVal, _ := v8.NewValue(iso, val)
					resolver.Resolve(strVal)
				} else {
					resolver.Resolve(jsVal)
				}
			}
		}
		return resolver.GetPromise().Value
	}).GetFunction(ctx))

	// storage.put(key, value) or storage.put(entries) -> Promise
	_ = stor.Set("put", v8.NewFunctionTemplate(iso, func(info *v8.FunctionCallbackInfo) *v8.Value {
		resolver, _ := v8.NewPromiseResolver(ctx)
		args := info.Args()
		if len(args) == 0 {
			errVal, _ := v8.NewValue(iso, "storage.put requires arguments")
			resolver.Reject(errVal)
			return resolver.GetPromise().Value
		}

		if len(args) >= 2 {
			// storage.put(key, value)
			key := args[0].String()
			// Serialize value to JSON via JS
			_ = ctx.Global().Set("__tmp_do_put_val", args[1])
			jsonResult, err := ctx.RunScript(`(function() {
				var v = globalThis.__tmp_do_put_val;
				delete globalThis.__tmp_do_put_val;
				return JSON.stringify(v);
			})()`, "do_put_serialize.js")
			if err != nil {
				errVal, _ := v8.NewValue(iso, err.Error())
				resolver.Reject(errVal)
				return resolver.GetPromise().Value
			}
			valueJSON := jsonResult.String()
			if err := bridge.Put(namespace, objectID, key, valueJSON); err != nil {
				errVal, _ := v8.NewValue(iso, err.Error())
				resolver.Reject(errVal)
				return resolver.GetPromise().Value
			}
			resolver.Resolve(v8.Undefined(iso))
		} else if args[0].IsObject() {
			// storage.put(entries) - entries is an object {key: value, ...}
			_ = ctx.Global().Set("__tmp_do_put_entries", args[0])
			entriesResult, err := ctx.RunScript(`(function() {
				var o = globalThis.__tmp_do_put_entries;
				delete globalThis.__tmp_do_put_entries;
				var result = {};
				for (var k in o) {
					if (o.hasOwnProperty(k)) result[k] = JSON.stringify(o[k]);
				}
				return JSON.stringify(result);
			})()`, "do_put_entries.js")
			if err != nil {
				errVal, _ := v8.NewValue(iso, err.Error())
				resolver.Reject(errVal)
				return resolver.GetPromise().Value
			}
			var entries map[string]string
			if err := json.Unmarshal([]byte(entriesResult.String()), &entries); err != nil {
				errVal, _ := v8.NewValue(iso, err.Error())
				resolver.Reject(errVal)
				return resolver.GetPromise().Value
			}
			if err := bridge.PutMulti(namespace, objectID, entries); err != nil {
				errVal, _ := v8.NewValue(iso, err.Error())
				resolver.Reject(errVal)
				return resolver.GetPromise().Value
			}
			resolver.Resolve(v8.Undefined(iso))
		} else {
			errVal, _ := v8.NewValue(iso, "storage.put requires (key, value) or (entries)")
			resolver.Reject(errVal)
		}
		return resolver.GetPromise().Value
	}).GetFunction(ctx))

	// storage.delete(key) or storage.delete([keys]) -> Promise
	_ = stor.Set("delete", v8.NewFunctionTemplate(iso, func(info *v8.FunctionCallbackInfo) *v8.Value {
		resolver, _ := v8.NewPromiseResolver(ctx)
		args := info.Args()
		if len(args) == 0 {
			errVal, _ := v8.NewValue(iso, "storage.delete requires a key argument")
			resolver.Reject(errVal)
			return resolver.GetPromise().Value
		}

		arg := args[0]
		if arg.IsArray() {
			_ = ctx.Global().Set("__tmp_do_del_keys", arg)
			keysResult, err := ctx.RunScript(`(function() {
				var a = globalThis.__tmp_do_del_keys;
				delete globalThis.__tmp_do_del_keys;
				return JSON.stringify(Array.from(a));
			})()`, "do_del_keys.js")
			if err != nil {
				errVal, _ := v8.NewValue(iso, err.Error())
				resolver.Reject(errVal)
				return resolver.GetPromise().Value
			}
			var keys []string
			_ = json.Unmarshal([]byte(keysResult.String()), &keys)
			count, err := bridge.DeleteMulti(namespace, objectID, keys)
			if err != nil {
				errVal, _ := v8.NewValue(iso, err.Error())
				resolver.Reject(errVal)
				return resolver.GetPromise().Value
			}
			countVal, _ := v8.NewValue(iso, int32(count))
			resolver.Resolve(countVal)
		} else {
			key := arg.String()
			if err := bridge.Delete(namespace, objectID, key); err != nil {
				errVal, _ := v8.NewValue(iso, err.Error())
				resolver.Reject(errVal)
				return resolver.GetPromise().Value
			}
			boolVal, _ := v8.NewValue(iso, true)
			resolver.Resolve(boolVal)
		}
		return resolver.GetPromise().Value
	}).GetFunction(ctx))

	// storage.deleteAll() -> Promise
	_ = stor.Set("deleteAll", v8.NewFunctionTemplate(iso, func(info *v8.FunctionCallbackInfo) *v8.Value {
		resolver, _ := v8.NewPromiseResolver(ctx)
		if err := bridge.DeleteAll(namespace, objectID); err != nil {
			errVal, _ := v8.NewValue(iso, err.Error())
			resolver.Reject(errVal)
			return resolver.GetPromise().Value
		}
		resolver.Resolve(v8.Undefined(iso))
		return resolver.GetPromise().Value
	}).GetFunction(ctx))

	// storage.list(options?) -> Promise<Map>
	_ = stor.Set("list", v8.NewFunctionTemplate(iso, func(info *v8.FunctionCallbackInfo) *v8.Value {
		resolver, _ := v8.NewPromiseResolver(ctx)
		args := info.Args()

		var prefix string
		limit := 128
		reverse := false

		if len(args) > 0 && args[0].IsObject() {
			_ = ctx.Global().Set("__tmp_do_list_opts", args[0])
			optsResult, err := ctx.RunScript(`(function() {
				var o = globalThis.__tmp_do_list_opts;
				delete globalThis.__tmp_do_list_opts;
				return JSON.stringify({
					prefix: o.prefix !== undefined && o.prefix !== null ? String(o.prefix) : "",
					limit: o.limit !== undefined && o.limit !== null ? Number(o.limit) : 128,
					reverse: o.reverse === true,
				});
			})()`, "do_list_opts.js")
			if err == nil {
				var opts struct {
					Prefix  string `json:"prefix"`
					Limit   int    `json:"limit"`
					Reverse bool   `json:"reverse"`
				}
				if json.Unmarshal([]byte(optsResult.String()), &opts) == nil {
					prefix = opts.Prefix
					limit = opts.Limit
					reverse = opts.Reverse
				}
			}
		}

		entries, err := bridge.List(namespace, objectID, prefix, limit, reverse)
		if err != nil {
			errVal, _ := v8.NewValue(iso, err.Error())
			resolver.Reject(errVal)
			return resolver.GetPromise().Value
		}

		// Build a Map in JS
		data, _ := json.Marshal(entries)
		jsResult, err := ctx.RunScript(fmt.Sprintf(`(function() {
			var obj = JSON.parse(%q);
			var m = new Map();
			for (var k in obj) { m.set(k, JSON.parse(obj[k])); }
			return m;
		})()`, string(data)), "do_list_result.js")
		if err != nil {
			errVal, _ := v8.NewValue(iso, err.Error())
			resolver.Reject(errVal)
			return resolver.GetPromise().Value
		}
		resolver.Resolve(jsResult)
		return resolver.GetPromise().Value
	}).GetFunction(ctx))

	return stor, nil
}
