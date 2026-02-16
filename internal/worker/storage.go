package worker

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	gonanoid "github.com/matoous/go-nanoid/v2"

	"github.com/cryguy/hostedat/internal/models"
	"github.com/fastschema/qjs"
	"gorm.io/gorm"
)

// StorageBridge provides Go methods backing the R2-compatible STORAGE binding.
type StorageBridge struct {
	DB          *gorm.DB
	SiteID      string
	StoragePath string
}

// Put stores an object with optional metadata.
func (s *StorageBridge) Put(key string, body []byte, contentType string, customMetadata map[string]string) (map[string]interface{}, error) {
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	// Calculate ETag (MD5).
	md5Sum := md5.Sum(body)
	etag := fmt.Sprintf(`"%s"`, hex.EncodeToString(md5Sum[:]))

	// Write to disk.
	storagePath, err := s.writeFile(body)
	if err != nil {
		return nil, fmt.Errorf("writing object: %w", err)
	}

	metaJSON, _ := json.Marshal(customMetadata)
	now := time.Now().UTC()

	// Upsert.
	var existing models.StorageObject
	err = s.DB.Where("site_id = ? AND key = ?", s.SiteID, key).First(&existing).Error
	if err == nil {
		if existing.StoragePath != storagePath {
			os.Remove(existing.StoragePath)
		}
		s.DB.Model(&existing).Updates(map[string]interface{}{
			"size":          int64(len(body)),
			"content_type":  contentType,
			"etag":          etag,
			"metadata":      string(metaJSON),
			"last_modified": now,
			"storage_path":  storagePath,
		})
	} else {
		obj := models.StorageObject{
			SiteID:       s.SiteID,
			Key:          key,
			Size:         int64(len(body)),
			ContentType:  contentType,
			ETag:         etag,
			Metadata:     string(metaJSON),
			LastModified: now,
			StoragePath:  storagePath,
		}
		if err := s.DB.Create(&obj).Error; err != nil {
			os.Remove(storagePath)
			return nil, fmt.Errorf("creating object record: %w", err)
		}
	}

	return map[string]interface{}{
		"key":          key,
		"size":         len(body),
		"etag":         etag,
		"uploaded":     now.Format(time.RFC3339),
		"httpMetadata": map[string]interface{}{"contentType": contentType},
	}, nil
}

// Get retrieves an object. Returns nil if not found.
func (s *StorageBridge) Get(key string) (map[string]interface{}, []byte, error) {
	var obj models.StorageObject
	if err := s.DB.Where("site_id = ? AND key = ?", s.SiteID, key).First(&obj).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil, nil
		}
		return nil, nil, err
	}

	data, err := os.ReadFile(obj.StoragePath)
	if err != nil {
		return nil, nil, fmt.Errorf("reading object file: %w", err)
	}

	var meta map[string]string
	json.Unmarshal([]byte(obj.Metadata), &meta)

	info := map[string]interface{}{
		"key":            obj.Key,
		"size":           obj.Size,
		"etag":           obj.ETag,
		"uploaded":       obj.LastModified.Format(time.RFC3339),
		"httpMetadata":   map[string]interface{}{"contentType": obj.ContentType},
		"customMetadata": meta,
	}

	return info, data, nil
}

// Head retrieves object metadata without body. Returns nil if not found.
func (s *StorageBridge) Head(key string) (map[string]interface{}, error) {
	var obj models.StorageObject
	if err := s.DB.Where("site_id = ? AND key = ?", s.SiteID, key).First(&obj).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}

	var meta map[string]string
	json.Unmarshal([]byte(obj.Metadata), &meta)

	return map[string]interface{}{
		"key":            obj.Key,
		"size":           obj.Size,
		"etag":           obj.ETag,
		"uploaded":       obj.LastModified.Format(time.RFC3339),
		"httpMetadata":   map[string]interface{}{"contentType": obj.ContentType},
		"customMetadata": meta,
	}, nil
}

// Delete removes an object.
func (s *StorageBridge) Delete(key string) error {
	var obj models.StorageObject
	if err := s.DB.Where("site_id = ? AND key = ?", s.SiteID, key).First(&obj).Error; err == nil {
		os.Remove(obj.StoragePath)
		s.DB.Delete(&obj)
	}
	return nil
}

// List lists objects with optional prefix, delimiter, cursor, and limit.
func (s *StorageBridge) List(prefix, delimiter, cursor string, limit int) (map[string]interface{}, error) {
	if limit <= 0 {
		limit = 1000
	}

	query := s.DB.Where("site_id = ?", s.SiteID)
	if prefix != "" {
		query = query.Where("key LIKE ?", prefix+"%")
	}
	if cursor != "" {
		query = query.Where("key > ?", cursor)
	}
	query = query.Order("key ASC")

	var objects []models.StorageObject
	query.Limit(limit + 1).Find(&objects)

	truncated := len(objects) > limit
	if truncated {
		objects = objects[:limit]
	}

	result := map[string]interface{}{
		"truncated": truncated,
	}

	if delimiter != "" {
		prefixSet := make(map[string]bool)
		var items []map[string]interface{}
		var delimitedPrefixes []string

		for _, obj := range objects {
			rest := strings.TrimPrefix(obj.Key, prefix)
			idx := strings.Index(rest, delimiter)
			if idx >= 0 {
				cp := prefix + rest[:idx+len(delimiter)]
				if !prefixSet[cp] {
					prefixSet[cp] = true
					delimitedPrefixes = append(delimitedPrefixes, cp)
				}
			} else {
				items = append(items, map[string]interface{}{
					"key":      obj.Key,
					"size":     obj.Size,
					"etag":     obj.ETag,
					"uploaded": obj.LastModified.Format(time.RFC3339),
				})
			}
		}
		result["objects"] = items
		result["delimitedPrefixes"] = delimitedPrefixes
	} else {
		items := make([]map[string]interface{}, 0, len(objects))
		for _, obj := range objects {
			items = append(items, map[string]interface{}{
				"key":      obj.Key,
				"size":     obj.Size,
				"etag":     obj.ETag,
				"uploaded": obj.LastModified.Format(time.RFC3339),
			})
		}
		result["objects"] = items
	}

	if truncated && len(objects) > 0 {
		result["cursor"] = objects[len(objects)-1].Key
	}

	return result, nil
}

func (s *StorageBridge) writeFile(data []byte) (string, error) {
	fileID, _ := gonanoid.Generate("0123456789abcdefghijklmnopqrstuvwxyz", 20)
	dir := filepath.Join(s.StoragePath, "objects", s.SiteID)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	path := filepath.Join(dir, fileID)
	if err := os.WriteFile(path, data, 0644); err != nil {
		return "", err
	}
	return path, nil
}

// buildStorageBinding creates an R2-compatible JS binding for env.STORAGE.
func buildStorageBinding(ctx *qjs.Context, bridge *StorageBridge) *qjs.Value {
	storage := ctx.NewObject()

	// put(key, body, options?) -> Promise<R2Object>
	putFn := ctx.Function(func(this *qjs.This) (*qjs.Value, error) {
		c := this.Context()
		args := this.Args()
		promise := this.Promise()
		if len(args) < 2 {
			promise.Reject(c.NewError(fmt.Errorf("STORAGE.put requires key and body arguments")))
			return c.NewUndefined(), nil
		}

		key := args[0].String()
		body := []byte(args[1].String())

		contentType := "application/octet-stream"
		var customMetadata map[string]string

		if len(args) > 2 && args[2].IsObject() {
			opts := args[2]

			httpMeta := opts.GetPropertyStr("httpMetadata")
			if httpMeta.IsObject() {
				ct := httpMeta.GetPropertyStr("contentType")
				if !ct.IsUndefined() && !ct.IsNull() {
					contentType = ct.String()
				}
				ct.Free()
			}
			httpMeta.Free()

			cm := opts.GetPropertyStr("customMetadata")
			if cm.IsObject() {
				customMetadata = make(map[string]string)
				names, err := cm.GetOwnPropertyNames()
				if err == nil {
					for _, name := range names {
						v := cm.GetPropertyStr(name)
						customMetadata[name] = v.String()
						v.Free()
					}
				}
			}
			cm.Free()
		}

		result, err := bridge.Put(key, body, contentType, customMetadata)
		if err != nil {
			promise.Reject(c.NewError(err))
			return c.NewUndefined(), nil
		}

		data, _ := json.Marshal(result)
		promise.Resolve(c.ParseJSON(string(data)))
		return c.NewUndefined(), nil
	}, true)
	storage.SetPropertyStr("put", putFn)

	// get(key) -> Promise<R2ObjectBody|null>
	getFn := ctx.Function(func(this *qjs.This) (*qjs.Value, error) {
		c := this.Context()
		args := this.Args()
		promise := this.Promise()
		if len(args) == 0 {
			promise.Reject(c.NewError(fmt.Errorf("STORAGE.get requires a key argument")))
			return c.NewUndefined(), nil
		}

		key := args[0].String()
		info, body, err := bridge.Get(key)
		if err != nil {
			promise.Reject(c.NewError(err))
			return c.NewUndefined(), nil
		}
		if info == nil {
			promise.Resolve(c.NewNull())
			return c.NewUndefined(), nil
		}

		// Build result object with body.
		data, _ := json.Marshal(info)
		result := c.ParseJSON(string(data))
		result.SetPropertyStr("body", c.NewString(string(body)))

		// Add text() and arrayBuffer() methods.
		bodyStr := string(body)
		textFn := c.Function(func(this *qjs.This) (*qjs.Value, error) {
			p := this.Promise()
			p.Resolve(this.Context().NewString(bodyStr))
			return this.Context().NewUndefined(), nil
		}, true)
		result.SetPropertyStr("text", textFn)

		promise.Resolve(result)
		return c.NewUndefined(), nil
	}, true)
	storage.SetPropertyStr("get", getFn)

	// head(key) -> Promise<R2Object|null>
	headFn := ctx.Function(func(this *qjs.This) (*qjs.Value, error) {
		c := this.Context()
		args := this.Args()
		promise := this.Promise()
		if len(args) == 0 {
			promise.Reject(c.NewError(fmt.Errorf("STORAGE.head requires a key argument")))
			return c.NewUndefined(), nil
		}

		key := args[0].String()
		info, err := bridge.Head(key)
		if err != nil {
			promise.Reject(c.NewError(err))
			return c.NewUndefined(), nil
		}
		if info == nil {
			promise.Resolve(c.NewNull())
			return c.NewUndefined(), nil
		}

		data, _ := json.Marshal(info)
		promise.Resolve(c.ParseJSON(string(data)))
		return c.NewUndefined(), nil
	}, true)
	storage.SetPropertyStr("head", headFn)

	// delete(key) -> Promise<void>
	deleteFn := ctx.Function(func(this *qjs.This) (*qjs.Value, error) {
		c := this.Context()
		args := this.Args()
		promise := this.Promise()
		if len(args) == 0 {
			promise.Reject(c.NewError(fmt.Errorf("STORAGE.delete requires a key argument")))
			return c.NewUndefined(), nil
		}

		key := args[0].String()
		if err := bridge.Delete(key); err != nil {
			promise.Reject(c.NewError(err))
			return c.NewUndefined(), nil
		}
		promise.Resolve(c.NewUndefined())
		return c.NewUndefined(), nil
	}, true)
	storage.SetPropertyStr("delete", deleteFn)

	// list(options?) -> Promise<R2Objects>
	listFn := ctx.Function(func(this *qjs.This) (*qjs.Value, error) {
		c := this.Context()
		args := this.Args()
		promise := this.Promise()

		var prefix, delimiter, cursor string
		limit := 1000

		if len(args) > 0 && args[0].IsObject() {
			opts := args[0]

			pVal := opts.GetPropertyStr("prefix")
			if !pVal.IsUndefined() && !pVal.IsNull() {
				prefix = pVal.String()
			}
			pVal.Free()

			dVal := opts.GetPropertyStr("delimiter")
			if !dVal.IsUndefined() && !dVal.IsNull() {
				delimiter = dVal.String()
			}
			dVal.Free()

			cVal := opts.GetPropertyStr("cursor")
			if !cVal.IsUndefined() && !cVal.IsNull() {
				cursor = cVal.String()
			}
			cVal.Free()

			lVal := opts.GetPropertyStr("limit")
			if !lVal.IsUndefined() && !lVal.IsNull() {
				limit = int(lVal.Int32())
			}
			lVal.Free()
		}

		result, err := bridge.List(prefix, delimiter, cursor, limit)
		if err != nil {
			promise.Reject(c.NewError(err))
			return c.NewUndefined(), nil
		}

		data, _ := json.Marshal(result)
		promise.Resolve(c.ParseJSON(string(data)))
		return c.NewUndefined(), nil
	}, true)
	storage.SetPropertyStr("list", listFn)

	return storage
}
