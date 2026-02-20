package worker

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"strings"
	"time"

	"github.com/minio/minio-go/v7"
	v8 "github.com/tommie/v8go"
)

const maxObjectSize = 128 * 1024 * 1024 // 128 MB

// StorageBridge backs a single R2-compatible bucket binding.
type StorageBridge struct {
	Client        *minio.Client
	PresignClient *minio.Client // optional client configured with public S3 host for presigning
	BucketName    string
	PublicS3URL   string // public-facing S3 URL (e.g. https://storage.example.com) for direct object URLs
}

// buildStorageBinding creates a JS object with R2-compatible get/put/delete/head/list
// methods backed by the given StorageBridge.
//
// All operations run synchronously on the JS thread (same rationale as KV bindings
// in kv.go). Minio-go calls are HTTP to localhost SeaweedFS and respond quickly.
func buildStorageBinding(iso *v8.Isolate, ctx *v8.Context, bridge *StorageBridge) (*v8.Value, error) {
	bucket, err := newJSObject(iso, ctx)
	if err != nil {
		return nil, fmt.Errorf("creating storage binding object: %w", err)
	}

	// get(key) -> Promise<R2ObjectBody|null>
	_ = bucket.Set("get", v8.NewFunctionTemplate(iso, func(info *v8.FunctionCallbackInfo) *v8.Value {
		resolver, _ := v8.NewPromiseResolver(ctx)
		args := info.Args()
		if len(args) == 0 {
			errVal, _ := v8.NewValue(iso, "BUCKET.get requires a key argument")
			resolver.Reject(errVal)
			return resolver.GetPromise().Value
		}
		key := args[0].String()

		obj, err := bridge.Client.GetObject(context.Background(), bridge.BucketName, key, minio.GetObjectOptions{})
		if err != nil {
			resolver.Resolve(v8.Null(iso))
			return resolver.GetPromise().Value
		}
		defer func() { _ = obj.Close() }()

		stat, err := obj.Stat()
		if err != nil {
			resolver.Resolve(v8.Null(iso))
			return resolver.GetPromise().Value
		}

		if stat.Size > int64(maxObjectSize) {
			errVal, _ := v8.NewValue(iso, fmt.Sprintf("object too large: %d bytes (max %d)", stat.Size, maxObjectSize))
			resolver.Reject(errVal)
			return resolver.GetPromise().Value
		}

		data, err := io.ReadAll(io.LimitReader(obj, int64(maxObjectSize)+1))
		if err != nil {
			errVal, _ := v8.NewValue(iso, fmt.Sprintf("reading object: %s", err.Error()))
			resolver.Reject(errVal)
			return resolver.GetPromise().Value
		}

		r2obj, err := buildR2ObjectBody(iso, ctx, key, data, &stat)
		if err != nil {
			errVal, _ := v8.NewValue(iso, err.Error())
			resolver.Reject(errVal)
			return resolver.GetPromise().Value
		}
		resolver.Resolve(r2obj.Value)
		return resolver.GetPromise().Value
	}).GetFunction(ctx))

	// put(key, value, opts?) -> Promise<R2Object>
	_ = bucket.Set("put", v8.NewFunctionTemplate(iso, func(info *v8.FunctionCallbackInfo) *v8.Value {
		resolver, _ := v8.NewPromiseResolver(ctx)
		args := info.Args()
		if len(args) < 2 {
			errVal, _ := v8.NewValue(iso, "BUCKET.put requires key and value arguments")
			resolver.Reject(errVal)
			return resolver.GetPromise().Value
		}
		key := args[0].String()

		// Coerce body to bytes via JS (handles string, ArrayBuffer, TypedArray, Blob).
		_ = ctx.Global().Set("__tmp_put_body", args[1])
		coerceResult, jsErr := ctx.RunScript(`(function() {
			var v = globalThis.__tmp_put_body;
			delete globalThis.__tmp_put_body;
			if (typeof v === 'string') return JSON.stringify({type:'string',data:v});
			if (v instanceof ArrayBuffer) {
				var arr = new Uint8Array(v);
				var s = '';
				for (var i = 0; i < arr.length; i++) s += String.fromCharCode(arr[i]);
				return JSON.stringify({type:'binary',data:btoa(s)});
			}
			if (ArrayBuffer.isView(v)) {
				var arr = new Uint8Array(v.buffer, v.byteOffset, v.byteLength);
				var s = '';
				for (var i = 0; i < arr.length; i++) s += String.fromCharCode(arr[i]);
				return JSON.stringify({type:'binary',data:btoa(s)});
			}
			if (typeof Blob !== 'undefined' && v instanceof Blob) {
				var parts = v._parts;
				if (!parts) return JSON.stringify({type:'error',data:'Blob has no _parts'});
				var result = '';
				for (var i = 0; i < parts.length; i++) result += String(parts[i]);
				return JSON.stringify({type:'string',data:result});
			}
			return JSON.stringify({type:'error',data:'unsupported body type: use string, ArrayBuffer, TypedArray, or Blob'});
		})()`, "storage_coerce.js")
		if jsErr != nil {
			errVal, _ := v8.NewValue(iso, fmt.Sprintf("BUCKET.put body coercion: %s", jsErr.Error()))
			resolver.Reject(errVal)
			return resolver.GetPromise().Value
		}

		var coerced struct {
			Type string `json:"type"`
			Data string `json:"data"`
		}
		if err := json.Unmarshal([]byte(coerceResult.String()), &coerced); err != nil {
			errVal, _ := v8.NewValue(iso, "BUCKET.put: failed to parse coerced body")
			resolver.Reject(errVal)
			return resolver.GetPromise().Value
		}

		var valueBytes []byte
		switch coerced.Type {
		case "string":
			valueBytes = []byte(coerced.Data)
		case "binary":
			var decErr error
			valueBytes, decErr = base64.StdEncoding.DecodeString(coerced.Data)
			if decErr != nil {
				errVal, _ := v8.NewValue(iso, "BUCKET.put: invalid binary body encoding")
				resolver.Reject(errVal)
				return resolver.GetPromise().Value
			}
		case "error":
			errVal, _ := v8.NewValue(iso, fmt.Sprintf("BUCKET.put: %s", coerced.Data))
			resolver.Reject(errVal)
			return resolver.GetPromise().Value
		}

		// Extract options via JS.
		opts := minio.PutObjectOptions{}
		customMeta := map[string]string{}

		if len(args) > 2 && args[2].IsObject() {
			_ = ctx.Global().Set("__tmp_put_opts", args[2])
			optsResult, err := ctx.RunScript(`(function() {
				var o = globalThis.__tmp_put_opts;
				delete globalThis.__tmp_put_opts;
				var result = {httpMetadata:{},customMetadata:{}};
				if (o.httpMetadata && typeof o.httpMetadata === 'object') {
					var h = o.httpMetadata;
					if (h.contentType != null) result.httpMetadata.contentType = String(h.contentType);
					if (h.contentEncoding != null) result.httpMetadata.contentEncoding = String(h.contentEncoding);
					if (h.contentDisposition != null) result.httpMetadata.contentDisposition = String(h.contentDisposition);
					if (h.contentLanguage != null) result.httpMetadata.contentLanguage = String(h.contentLanguage);
					if (h.cacheControl != null) result.httpMetadata.cacheControl = String(h.cacheControl);
				}
				if (o.customMetadata && typeof o.customMetadata === 'object') {
					for (var k in o.customMetadata) {
						if (o.customMetadata.hasOwnProperty(k)) result.customMetadata[k] = String(o.customMetadata[k]);
					}
				}
				return JSON.stringify(result);
			})()`, "storage_put_opts.js")
			if err == nil {
				var parsed struct {
					HTTPMetadata struct {
						ContentType        string `json:"contentType"`
						ContentEncoding    string `json:"contentEncoding"`
						ContentDisposition string `json:"contentDisposition"`
						ContentLanguage    string `json:"contentLanguage"`
						CacheControl       string `json:"cacheControl"`
					} `json:"httpMetadata"`
					CustomMetadata map[string]string `json:"customMetadata"`
				}
				if json.Unmarshal([]byte(optsResult.String()), &parsed) == nil {
					if parsed.HTTPMetadata.ContentType != "" {
						opts.ContentType = parsed.HTTPMetadata.ContentType
					}
					if parsed.HTTPMetadata.ContentEncoding != "" {
						opts.ContentEncoding = parsed.HTTPMetadata.ContentEncoding
					}
					if parsed.HTTPMetadata.ContentDisposition != "" {
						opts.ContentDisposition = parsed.HTTPMetadata.ContentDisposition
					}
					if parsed.HTTPMetadata.ContentLanguage != "" {
						opts.ContentLanguage = parsed.HTTPMetadata.ContentLanguage
					}
					if parsed.HTTPMetadata.CacheControl != "" {
						opts.CacheControl = parsed.HTTPMetadata.CacheControl
					}
					customMeta = parsed.CustomMetadata
				}
			}
		}

		if len(customMeta) > 0 {
			opts.UserMetadata = customMeta
		}

		reader := bytes.NewReader(valueBytes)
		putInfo, err := bridge.Client.PutObject(context.Background(), bridge.BucketName, key, reader, int64(len(valueBytes)), opts)
		if err != nil {
			errVal, _ := v8.NewValue(iso, fmt.Sprintf("putting object: %s", err.Error()))
			resolver.Reject(errVal)
			return resolver.GetPromise().Value
		}

		r2obj, err := buildR2Object(iso, ctx, key, putInfo.Size, putInfo.ETag, opts.ContentType, customMeta)
		if err != nil {
			errVal, _ := v8.NewValue(iso, err.Error())
			resolver.Reject(errVal)
			return resolver.GetPromise().Value
		}
		resolver.Resolve(r2obj.Value)
		return resolver.GetPromise().Value
	}).GetFunction(ctx))

	// delete(key|keys) -> Promise<void>
	_ = bucket.Set("delete", v8.NewFunctionTemplate(iso, func(info *v8.FunctionCallbackInfo) *v8.Value {
		resolver, _ := v8.NewPromiseResolver(ctx)
		args := info.Args()
		if len(args) == 0 {
			errVal, _ := v8.NewValue(iso, "BUCKET.delete requires a key argument")
			resolver.Reject(errVal)
			return resolver.GetPromise().Value
		}

		// Support single key (string) or array of keys via JS extraction.
		_ = ctx.Global().Set("__tmp_del_arg", args[0])
		keysResult, err := ctx.RunScript(`(function() {
			var v = globalThis.__tmp_del_arg;
			delete globalThis.__tmp_del_arg;
			if (Array.isArray(v)) return JSON.stringify(v.map(String));
			return JSON.stringify([String(v)]);
		})()`, "storage_del_keys.js")
		if err != nil {
			errVal, _ := v8.NewValue(iso, fmt.Sprintf("BUCKET.delete: %s", err.Error()))
			resolver.Reject(errVal)
			return resolver.GetPromise().Value
		}

		var keys []string
		if err := json.Unmarshal([]byte(keysResult.String()), &keys); err != nil {
			errVal, _ := v8.NewValue(iso, "BUCKET.delete: failed to parse keys")
			resolver.Reject(errVal)
			return resolver.GetPromise().Value
		}

		for _, k := range keys {
			_ = bridge.Client.RemoveObject(context.Background(), bridge.BucketName, k, minio.RemoveObjectOptions{})
		}

		resolver.Resolve(v8.Undefined(iso))
		return resolver.GetPromise().Value
	}).GetFunction(ctx))

	// head(key) -> Promise<R2Object|null>
	_ = bucket.Set("head", v8.NewFunctionTemplate(iso, func(info *v8.FunctionCallbackInfo) *v8.Value {
		resolver, _ := v8.NewPromiseResolver(ctx)
		args := info.Args()
		if len(args) == 0 {
			errVal, _ := v8.NewValue(iso, "BUCKET.head requires a key argument")
			resolver.Reject(errVal)
			return resolver.GetPromise().Value
		}
		key := args[0].String()

		stat, err := bridge.Client.StatObject(context.Background(), bridge.BucketName, key, minio.StatObjectOptions{})
		if err != nil {
			resolver.Resolve(v8.Null(iso))
			return resolver.GetPromise().Value
		}

		r2obj, err := buildR2Object(iso, ctx, key, stat.Size, stat.ETag, stat.ContentType, stat.UserMetadata)
		if err != nil {
			errVal, _ := v8.NewValue(iso, err.Error())
			resolver.Reject(errVal)
			return resolver.GetPromise().Value
		}
		resolver.Resolve(r2obj.Value)
		return resolver.GetPromise().Value
	}).GetFunction(ctx))

	// list(opts?) -> Promise<R2Objects>
	_ = bucket.Set("list", v8.NewFunctionTemplate(iso, func(info *v8.FunctionCallbackInfo) *v8.Value {
		resolver, _ := v8.NewPromiseResolver(ctx)
		args := info.Args()

		var prefix, cursor, delimiter string
		limit := 1000

		if len(args) > 0 && args[0].IsObject() {
			_ = ctx.Global().Set("__tmp_list_opts", args[0])
			optsResult, err := ctx.RunScript(`(function() {
				var o = globalThis.__tmp_list_opts;
				delete globalThis.__tmp_list_opts;
				return JSON.stringify({
					prefix: o.prefix != null ? String(o.prefix) : '',
					cursor: o.cursor != null ? String(o.cursor) : '',
					delimiter: o.delimiter != null ? String(o.delimiter) : '',
					limit: o.limit != null ? Number(o.limit) : 1000,
				});
			})()`, "storage_list_opts.js")
			if err == nil {
				var opts struct {
					Prefix    string `json:"prefix"`
					Cursor    string `json:"cursor"`
					Delimiter string `json:"delimiter"`
					Limit     int    `json:"limit"`
				}
				if json.Unmarshal([]byte(optsResult.String()), &opts) == nil {
					prefix = opts.Prefix
					cursor = opts.Cursor
					delimiter = opts.Delimiter
					limit = opts.Limit
				}
			}
		}

		listOpts := minio.ListObjectsOptions{
			Prefix:    prefix,
			MaxKeys:   limit,
			Recursive: delimiter == "",
		}
		if cursor != "" {
			listOpts.StartAfter = cursor
		}

		var objects []map[string]interface{}
		var delimitedPrefixes []string
		count := 0

		for obj := range bridge.Client.ListObjects(context.Background(), bridge.BucketName, listOpts) {
			if obj.Err != nil {
				break
			}
			if strings.HasSuffix(obj.Key, "/") && delimiter != "" {
				delimitedPrefixes = append(delimitedPrefixes, obj.Key)
				continue
			}
			objects = append(objects, map[string]interface{}{
				"key":      obj.Key,
				"size":     obj.Size,
				"etag":     obj.ETag,
				"uploaded": obj.LastModified.UnixMilli(),
			})
			count++
			if count >= limit {
				break
			}
		}

		truncated := count >= limit
		var nextCursor string
		if truncated && len(objects) > 0 {
			nextCursor = objects[len(objects)-1]["key"].(string)
		}

		result := map[string]interface{}{
			"objects":           objects,
			"truncated":         truncated,
			"cursor":            nextCursor,
			"delimitedPrefixes": delimitedPrefixes,
		}
		data, _ := json.Marshal(result)

		jsResult, err := ctx.RunScript(fmt.Sprintf("JSON.parse(%q)", string(data)), "storage_list_result.js")
		if err != nil {
			errVal, _ := v8.NewValue(iso, err.Error())
			resolver.Reject(errVal)
			return resolver.GetPromise().Value
		}
		resolver.Resolve(jsResult)
		return resolver.GetPromise().Value
	}).GetFunction(ctx))

	// createSignedUrl(key, opts?) -> Promise<string>
	if bridge.Client != nil || bridge.PresignClient != nil {
		_ = bucket.Set("createSignedUrl", v8.NewFunctionTemplate(iso, func(info *v8.FunctionCallbackInfo) *v8.Value {
			resolver, _ := v8.NewPromiseResolver(ctx)
			args := info.Args()
			if len(args) == 0 {
				errVal, _ := v8.NewValue(iso, "BUCKET.createSignedUrl requires a key argument")
				resolver.Reject(errVal)
				return resolver.GetPromise().Value
			}
			key := args[0].String()

			expiry := 3600 // default 1 hour
			if len(args) > 1 && args[1].IsObject() {
				_ = ctx.Global().Set("__tmp_sign_opts", args[1])
				optsResult, err := ctx.RunScript(`(function() {
					var o = globalThis.__tmp_sign_opts;
					delete globalThis.__tmp_sign_opts;
					return o.expiresIn != null ? Number(o.expiresIn) : 3600;
				})()`, "storage_sign_opts.js")
				if err == nil {
					expiry = int(optsResult.Int32())
				}
			}
			if expiry < 1 {
				expiry = 1
			}
			if expiry > 604800 {
				expiry = 604800 // cap at 7 days
			}

			signClient := bridge.PresignClient
			if signClient == nil {
				if bridge.PublicS3URL != "" {
					errVal, _ := v8.NewValue(iso, "creating signed URL: presign client not configured for public S3 host")
					resolver.Reject(errVal)
					return resolver.GetPromise().Value
				}
				signClient = bridge.Client
			}
			if signClient == nil {
				errVal, _ := v8.NewValue(iso, "creating signed URL: storage client not configured")
				resolver.Reject(errVal)
				return resolver.GetPromise().Value
			}

			presigned, err := signClient.PresignedGetObject(
				context.Background(),
				bridge.BucketName,
				key,
				time.Duration(expiry)*time.Second,
				nil,
			)
			if err != nil {
				errVal, _ := v8.NewValue(iso, fmt.Sprintf("creating signed URL: %s", err.Error()))
				resolver.Reject(errVal)
				return resolver.GetPromise().Value
			}

			urlVal, _ := v8.NewValue(iso, presigned.String())
			resolver.Resolve(urlVal)
			return resolver.GetPromise().Value
		}).GetFunction(ctx))
	}

	// publicUrl(key) -> string (synchronous)
	if bridge.PublicS3URL != "" {
		_ = bucket.Set("publicUrl", v8.NewFunctionTemplate(iso, func(info *v8.FunctionCallbackInfo) *v8.Value {
			args := info.Args()
			if len(args) == 0 {
				return throwError(iso, "BUCKET.publicUrl requires a key argument")
			}
			key := args[0].String()

			objectURL, err := buildPublicObjectURL(bridge.PublicS3URL, bridge.BucketName, key)
			if err != nil {
				return throwError(iso, fmt.Sprintf("creating public object URL: %s", err.Error()))
			}

			val, _ := v8.NewValue(iso, objectURL)
			return val
		}).GetFunction(ctx))
	}

	return bucket.Value, nil
}

// buildPublicObjectURL returns an object URL using the configured public S3 base.
func buildPublicObjectURL(publicBase string, bucket string, key string) (string, error) {
	pub, err := url.Parse(publicBase)
	if err != nil {
		return "", err
	}
	if pub.Scheme == "" || pub.Host == "" {
		return "", fmt.Errorf("public S3 URL must include scheme and host")
	}

	cleanBucket := strings.Trim(bucket, "/")
	cleanKey := strings.TrimPrefix(key, "/")
	base := strings.TrimRight(pub.Path, "/")
	pub.Path = base + "/" + cleanBucket + "/" + cleanKey
	pub.RawPath = base + "/" + url.PathEscape(cleanBucket) + "/" + escapePathSegments(cleanKey)
	pub.RawQuery = ""
	pub.Fragment = ""

	return pub.String(), nil
}

func escapePathSegments(path string) string {
	if path == "" {
		return ""
	}
	parts := strings.Split(path, "/")
	for i, p := range parts {
		parts[i] = url.PathEscape(p)
	}
	return strings.Join(parts, "/")
}

// buildR2Object creates a JS object matching the Cloudflare R2Object shape.
func buildR2Object(iso *v8.Isolate, ctx *v8.Context, key string, size int64, etag string, contentType string, customMeta map[string]string) (*v8.Object, error) {
	obj, err := newJSObject(iso, ctx)
	if err != nil {
		return nil, fmt.Errorf("creating R2Object: %w", err)
	}

	keyVal, _ := v8.NewValue(iso, key)
	_ = obj.Set("key", keyVal)
	sizeVal, _ := v8.NewValue(iso, float64(size))
	_ = obj.Set("size", sizeVal)
	etagVal, _ := v8.NewValue(iso, etag)
	_ = obj.Set("etag", etagVal)
	httpEtagVal, _ := v8.NewValue(iso, "\""+etag+"\"")
	_ = obj.Set("httpEtag", httpEtagVal)
	versionVal, _ := v8.NewValue(iso, etag)
	_ = obj.Set("version", versionVal)
	scVal, _ := v8.NewValue(iso, "STANDARD")
	_ = obj.Set("storageClass", scVal)

	httpMeta, _ := newJSObject(iso, ctx)
	if contentType != "" {
		ctVal, _ := v8.NewValue(iso, contentType)
		_ = httpMeta.Set("contentType", ctVal)
	}
	_ = obj.Set("httpMetadata", httpMeta)

	cm, _ := newJSObject(iso, ctx)
	for k, v := range customMeta {
		vVal, _ := v8.NewValue(iso, v)
		_ = cm.Set(k, vVal)
	}
	_ = obj.Set("customMetadata", cm)

	checksums, _ := newJSObject(iso, ctx)
	_ = obj.Set("checksums", checksums)

	return obj, nil
}

// buildR2ObjectBody extends R2Object with body reading methods.
func buildR2ObjectBody(iso *v8.Isolate, ctx *v8.Context, key string, data []byte, stat *minio.ObjectInfo) (*v8.Object, error) {
	obj, err := buildR2Object(iso, ctx, key, stat.Size, stat.ETag, stat.ContentType, stat.UserMetadata)
	if err != nil {
		return nil, err
	}

	uploadedVal, _ := v8.NewValue(iso, float64(stat.LastModified.UnixMilli()))
	_ = obj.Set("uploaded", uploadedVal)

	bodyUsed := false
	bodyData := string(data)

	// text() -> Promise<string>
	_ = obj.Set("text", v8.NewFunctionTemplate(iso, func(info *v8.FunctionCallbackInfo) *v8.Value {
		resolver, _ := v8.NewPromiseResolver(ctx)
		if bodyUsed {
			errVal, _ := v8.NewValue(iso, "body already consumed")
			resolver.Reject(errVal)
			return resolver.GetPromise().Value
		}
		bodyUsed = true
		boolVal, _ := v8.NewValue(iso, true)
		_ = obj.Set("bodyUsed", boolVal)
		textVal, _ := v8.NewValue(iso, bodyData)
		resolver.Resolve(textVal)
		return resolver.GetPromise().Value
	}).GetFunction(ctx))

	// arrayBuffer() -> Promise<ArrayBuffer>
	_ = obj.Set("arrayBuffer", v8.NewFunctionTemplate(iso, func(info *v8.FunctionCallbackInfo) *v8.Value {
		resolver, _ := v8.NewPromiseResolver(ctx)
		if bodyUsed {
			errVal, _ := v8.NewValue(iso, "body already consumed")
			resolver.Reject(errVal)
			return resolver.GetPromise().Value
		}
		bodyUsed = true
		boolVal, _ := v8.NewValue(iso, true)
		_ = obj.Set("bodyUsed", boolVal)
		// Create ArrayBuffer via JS from base64.
		b64 := base64.StdEncoding.EncodeToString(data)
		b64Val, _ := v8.NewValue(iso, b64)
		_ = ctx.Global().Set("__tmp_ab_b64", b64Val)
		abResult, err := ctx.RunScript(`(function() {
			var b64 = globalThis.__tmp_ab_b64;
			delete globalThis.__tmp_ab_b64;
			var raw = atob(b64);
			var buf = new ArrayBuffer(raw.length);
			var arr = new Uint8Array(buf);
			for (var i = 0; i < raw.length; i++) arr[i] = raw.charCodeAt(i);
			return buf;
		})()`, "r2_arraybuffer.js")
		if err != nil {
			errVal, _ := v8.NewValue(iso, err.Error())
			resolver.Reject(errVal)
			return resolver.GetPromise().Value
		}
		resolver.Resolve(abResult)
		return resolver.GetPromise().Value
	}).GetFunction(ctx))

	// json() -> Promise<any>
	_ = obj.Set("json", v8.NewFunctionTemplate(iso, func(info *v8.FunctionCallbackInfo) *v8.Value {
		resolver, _ := v8.NewPromiseResolver(ctx)
		if bodyUsed {
			errVal, _ := v8.NewValue(iso, "body already consumed")
			resolver.Reject(errVal)
			return resolver.GetPromise().Value
		}
		bodyUsed = true
		boolVal, _ := v8.NewValue(iso, true)
		_ = obj.Set("bodyUsed", boolVal)
		if !json.Valid([]byte(bodyData)) {
			errVal, _ := v8.NewValue(iso, "invalid JSON")
			resolver.Reject(errVal)
			return resolver.GetPromise().Value
		}
		parsed, err := ctx.RunScript(fmt.Sprintf("JSON.parse(%q)", bodyData), "r2_json_parse.js")
		if err != nil {
			errVal, _ := v8.NewValue(iso, err.Error())
			resolver.Reject(errVal)
			return resolver.GetPromise().Value
		}
		resolver.Resolve(parsed)
		return resolver.GetPromise().Value
	}).GetFunction(ctx))

	falseVal, _ := v8.NewValue(iso, false)
	_ = obj.Set("bodyUsed", falseVal)

	return obj, nil
}
