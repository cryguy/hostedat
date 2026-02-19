package worker

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/cryguy/hostedat/internal/models"
	v8 "github.com/tommie/v8go"
	"gorm.io/gorm"
)

// CacheBridge provides Go methods backing the Cache API JS bindings.
type CacheBridge struct {
	DB *gorm.DB
}

// Match retrieves a cached response for the given cache name and URL.
// Returns nil if not found or expired.
func (cb *CacheBridge) Match(cacheName, url string) (*models.CacheEntry, error) {
	var entry models.CacheEntry
	err := cb.DB.Where("cache_name = ? AND url = ?", cacheName, url).First(&entry).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}

	// Check expiration.
	if entry.ExpiresAt != nil && entry.ExpiresAt.Before(time.Now()) {
		cb.DB.Delete(&entry)
		return nil, nil
	}

	return &entry, nil
}

// Put stores a response in the cache, replacing any existing entry for the URL.
func (cb *CacheBridge) Put(cacheName, url string, status int, headers string, body []byte, ttl *int) error {
	var expiresAt *time.Time
	if ttl != nil && *ttl > 0 {
		t := time.Now().Add(time.Duration(*ttl) * time.Second)
		expiresAt = &t
	}

	// Delete existing entry if present.
	cb.DB.Where("cache_name = ? AND url = ?", cacheName, url).Delete(&models.CacheEntry{})

	entry := models.CacheEntry{
		CacheName: cacheName,
		URL:       url,
		Status:    status,
		Headers:   headers,
		Body:      body,
		ExpiresAt: expiresAt,
		CreatedAt: time.Now(),
	}
	return cb.DB.Create(&entry).Error
}

// Delete removes a cached response. Returns true if an entry was deleted.
func (cb *CacheBridge) Delete(cacheName, url string) (bool, error) {
	result := cb.DB.Where("cache_name = ? AND url = ?", cacheName, url).Delete(&models.CacheEntry{})
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected > 0, nil
}

// cacheJS defines the Cache and CacheStorage classes available to workers.
const cacheJS = `
(function() {

class Cache {
	constructor(name) {
		this._name = name;
	}

	match(request, options) {
		var url;
		if (typeof request === 'string') {
			url = request;
		} else if (request && request.url) {
			url = request.url;
		} else {
			return Promise.resolve(undefined);
		}

		var result = __cache_match(this._name, url);
		if (result === 'null' || result === null || result === undefined) {
			return Promise.resolve(undefined);
		}

		try {
			var parsed = JSON.parse(result);
			var hdrs = new Headers(parsed.headers || {});
			var resp = new Response(parsed.body, {
				status: parsed.status,
				headers: hdrs,
			});
			return Promise.resolve(resp);
		} catch(e) {
			return Promise.resolve(undefined);
		}
	}

	put(request, response) {
		var url;
		if (typeof request === 'string') {
			url = request;
		} else if (request && request.url) {
			url = request.url;
		} else {
			return Promise.reject(new Error('Cache.put requires a request'));
		}

		if (!response) {
			return Promise.reject(new Error('Cache.put requires a response'));
		}

		// Extract Cache-Control max-age for TTL.
		var ttl = -1;
		var cc = '';
		if (response.headers && typeof response.headers.get === 'function') {
			cc = response.headers.get('Cache-Control') || '';
		}
		if (cc) {
			var match = cc.match(/max-age=(\d+)/);
			if (match) {
				ttl = parseInt(match[1], 10);
			}
		}

		// Serialize headers.
		var hdrs = {};
		if (response.headers) {
			if (typeof response.headers.forEach === 'function') {
				response.headers.forEach(function(v, k) { hdrs[k] = v; });
			} else if (response.headers._map) {
				var m = response.headers._map;
				for (var k in m) { if (m.hasOwnProperty(k)) hdrs[k] = String(m[k]); }
			}
		}

		var body = '';
		if (response._body !== null && response._body !== undefined) {
			body = String(response._body);
		}

		__cache_put(
			this._name,
			url,
			response.status || 200,
			JSON.stringify(hdrs),
			body,
			ttl
		);

		return Promise.resolve(undefined);
	}

	delete(request, options) {
		var url;
		if (typeof request === 'string') {
			url = request;
		} else if (request && request.url) {
			url = request.url;
		} else {
			return Promise.resolve(false);
		}

		var result = __cache_delete(this._name, url);
		return Promise.resolve(result === 'true' || result === true);
	}
}

class CacheStorage {
	constructor() {
		this._caches = {};
		this.default = new Cache('default');
	}

	open(name) {
		if (!this._caches[name]) {
			this._caches[name] = new Cache(name);
		}
		return Promise.resolve(this._caches[name]);
	}
}

globalThis.caches = new CacheStorage();

})();
`

// setupCache registers the Cache API JS classes and Go-backed functions.
func setupCache(iso *v8.Isolate, ctx *v8.Context, el *eventLoop) error {
	// We need a reference to the DB from the request state.
	// The cache bridge is created per-call using the global DB reference.

	// __cache_match(cacheName, url) -> JSON string or "null"
	matchFn := v8.NewFunctionTemplate(iso, func(info *v8.FunctionCallbackInfo) *v8.Value {
		args := info.Args()
		if len(args) < 2 {
			val, _ := v8.NewValue(iso, "null")
			return val
		}
		cacheName := args[0].String()
		url := args[1].String()

		bridge := getCacheBridge(info.Context())
		if bridge == nil {
			val, _ := v8.NewValue(iso, "null")
			return val
		}

		entry, err := bridge.Match(cacheName, url)
		if err != nil || entry == nil {
			val, _ := v8.NewValue(iso, "null")
			return val
		}

		var headers map[string]string
		if entry.Headers != "" {
			json.Unmarshal([]byte(entry.Headers), &headers)
		}
		if headers == nil {
			headers = make(map[string]string)
		}

		result := map[string]interface{}{
			"status":  entry.Status,
			"headers": headers,
			"body":    string(entry.Body),
		}
		data, _ := json.Marshal(result)
		val, _ := v8.NewValue(iso, string(data))
		return val
	})
	if err := ctx.Global().Set("__cache_match", matchFn.GetFunction(ctx)); err != nil {
		return fmt.Errorf("setting __cache_match: %w", err)
	}

	// __cache_put(cacheName, url, status, headersJSON, body, ttl)
	putFn := v8.NewFunctionTemplate(iso, func(info *v8.FunctionCallbackInfo) *v8.Value {
		args := info.Args()
		if len(args) < 6 {
			return v8.Undefined(iso)
		}
		cacheName := args[0].String()
		url := args[1].String()
		status := int(args[2].Int32())
		headersJSON := args[3].String()
		body := args[4].String()
		ttlVal := int(args[5].Int32())

		bridge := getCacheBridge(info.Context())
		if bridge == nil {
			return v8.Undefined(iso)
		}

		var ttl *int
		if ttlVal > 0 {
			ttl = &ttlVal
		}

		bridge.Put(cacheName, url, status, headersJSON, []byte(body), ttl)
		return v8.Undefined(iso)
	})
	if err := ctx.Global().Set("__cache_put", putFn.GetFunction(ctx)); err != nil {
		return fmt.Errorf("setting __cache_put: %w", err)
	}

	// __cache_delete(cacheName, url) -> "true" or "false"
	deleteFn := v8.NewFunctionTemplate(iso, func(info *v8.FunctionCallbackInfo) *v8.Value {
		args := info.Args()
		if len(args) < 2 {
			val, _ := v8.NewValue(iso, "false")
			return val
		}
		cacheName := args[0].String()
		url := args[1].String()

		bridge := getCacheBridge(info.Context())
		if bridge == nil {
			val, _ := v8.NewValue(iso, "false")
			return val
		}

		deleted, err := bridge.Delete(cacheName, url)
		if err != nil || !deleted {
			val, _ := v8.NewValue(iso, "false")
			return val
		}

		val, _ := v8.NewValue(iso, "true")
		return val
	})
	if err := ctx.Global().Set("__cache_delete", deleteFn.GetFunction(ctx)); err != nil {
		return fmt.Errorf("setting __cache_delete: %w", err)
	}

	// Evaluate JS classes.
	if _, err := ctx.RunScript(cacheJS, "cache.js"); err != nil {
		return fmt.Errorf("evaluating cache.js: %w", err)
	}
	return nil
}

// getCacheBridge retrieves the CacheBridge from the current request state.
// The DB reference is stored in the per-request Env.
func getCacheBridge(ctx *v8.Context) *CacheBridge {
	reqIDVal, err := ctx.Global().Get("__requestID")
	if err != nil || reqIDVal.IsUndefined() {
		return nil
	}

	var reqID uint64
	fmt.Sscanf(reqIDVal.String(), "%d", &reqID)
	state := getRequestState(reqID)
	if state == nil || state.env == nil || state.env.db == nil {
		return nil
	}

	return &CacheBridge{DB: state.env.db}
}
