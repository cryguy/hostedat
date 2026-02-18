package worker

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"strings"
	"time"

	"github.com/fastschema/qjs"
	minio "github.com/minio/minio-go/v7"
)

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
func buildStorageBinding(ctx *qjs.Context, bridge *StorageBridge) *qjs.Value {
	bucket := ctx.NewObject()

	// get(key) -> Promise<R2ObjectBody|null>
	getFn := ctx.Function(func(this *qjs.This) (*qjs.Value, error) {
		c := this.Context()
		args := this.Args()
		promise := this.Promise()
		if len(args) == 0 {
			promise.Reject(c.NewError(fmt.Errorf("BUCKET.get requires a key argument")))
			return c.NewUndefined(), nil
		}
		key := args[0].String()

		obj, err := bridge.Client.GetObject(context.Background(), bridge.BucketName, key, minio.GetObjectOptions{})
		if err != nil {
			promise.Resolve(c.NewNull())
			return c.NewUndefined(), nil
		}
		defer obj.Close()

		stat, err := obj.Stat()
		if err != nil {
			// Object not found.
			promise.Resolve(c.NewNull())
			return c.NewUndefined(), nil
		}

		data, err := io.ReadAll(obj)
		if err != nil {
			promise.Reject(c.NewError(fmt.Errorf("reading object: %w", err)))
			return c.NewUndefined(), nil
		}

		r2obj := buildR2ObjectBody(c, key, data, &stat)
		promise.Resolve(r2obj)
		return c.NewUndefined(), nil
	}, true)
	bucket.SetPropertyStr("get", getFn)

	// put(key, value, opts?) -> Promise<R2Object>
	putFn := ctx.Function(func(this *qjs.This) (*qjs.Value, error) {
		c := this.Context()
		args := this.Args()
		promise := this.Promise()
		if len(args) < 2 {
			promise.Reject(c.NewError(fmt.Errorf("BUCKET.put requires key and value arguments")))
			return c.NewUndefined(), nil
		}
		key := args[0].String()
		valueBytes, err := coerceStoragePutBody(args[1])
		if err != nil {
			promise.Reject(c.NewError(err))
			return c.NewUndefined(), nil
		}

		opts := minio.PutObjectOptions{}
		customMeta := map[string]string{}

		if len(args) > 2 && args[2].IsObject() {
			optsObj := args[2]

			// httpMetadata
			httpMeta := optsObj.GetPropertyStr("httpMetadata")
			if httpMeta.IsObject() {
				ct := httpMeta.GetPropertyStr("contentType")
				if !ct.IsUndefined() && !ct.IsNull() {
					opts.ContentType = ct.String()
				}
				ct.Free()
				ce := httpMeta.GetPropertyStr("contentEncoding")
				if !ce.IsUndefined() && !ce.IsNull() {
					opts.ContentEncoding = ce.String()
				}
				ce.Free()
				cd := httpMeta.GetPropertyStr("contentDisposition")
				if !cd.IsUndefined() && !cd.IsNull() {
					opts.ContentDisposition = cd.String()
				}
				cd.Free()
				cl := httpMeta.GetPropertyStr("contentLanguage")
				if !cl.IsUndefined() && !cl.IsNull() {
					opts.ContentLanguage = cl.String()
				}
				cl.Free()
				cc := httpMeta.GetPropertyStr("cacheControl")
				if !cc.IsUndefined() && !cc.IsNull() {
					opts.CacheControl = cc.String()
				}
				cc.Free()
			}
			httpMeta.Free()

			// customMetadata
			cm := optsObj.GetPropertyStr("customMetadata")
			if cm.IsObject() {
				names, err := cm.GetOwnPropertyNames()
				if err == nil {
					for _, name := range names {
						v := cm.GetPropertyStr(name)
						customMeta[name] = v.String()
						v.Free()
					}
				}
			}
			cm.Free()
		}

		if len(customMeta) > 0 {
			opts.UserMetadata = customMeta
		}

		reader := bytes.NewReader(valueBytes)
		info, err := bridge.Client.PutObject(context.Background(), bridge.BucketName, key, reader, int64(len(valueBytes)), opts)
		if err != nil {
			promise.Reject(c.NewError(fmt.Errorf("putting object: %w", err)))
			return c.NewUndefined(), nil
		}

		r2obj := buildR2Object(c, key, info.Size, info.ETag, opts.ContentType, customMeta)
		promise.Resolve(r2obj)
		return c.NewUndefined(), nil
	}, true)
	bucket.SetPropertyStr("put", putFn)

	// delete(key|keys) -> Promise<void>
	delFn := ctx.Function(func(this *qjs.This) (*qjs.Value, error) {
		c := this.Context()
		args := this.Args()
		promise := this.Promise()
		if len(args) == 0 {
			promise.Reject(c.NewError(fmt.Errorf("BUCKET.delete requires a key argument")))
			return c.NewUndefined(), nil
		}

		// Support single key (string) or array of keys.
		if args[0].IsArray() {
			length := args[0].GetPropertyStr("length")
			n := int(length.Int32())
			length.Free()
			for i := 0; i < n; i++ {
				item := args[0].GetPropertyIndex(int64(i))
				k := item.String()
				item.Free()
				_ = bridge.Client.RemoveObject(context.Background(), bridge.BucketName, k, minio.RemoveObjectOptions{})
			}
		} else {
			key := args[0].String()
			_ = bridge.Client.RemoveObject(context.Background(), bridge.BucketName, key, minio.RemoveObjectOptions{})
		}

		promise.Resolve(c.NewUndefined())
		return c.NewUndefined(), nil
	}, true)
	bucket.SetPropertyStr("delete", delFn)

	// head(key) -> Promise<R2Object|null>
	headFn := ctx.Function(func(this *qjs.This) (*qjs.Value, error) {
		c := this.Context()
		args := this.Args()
		promise := this.Promise()
		if len(args) == 0 {
			promise.Reject(c.NewError(fmt.Errorf("BUCKET.head requires a key argument")))
			return c.NewUndefined(), nil
		}
		key := args[0].String()

		stat, err := bridge.Client.StatObject(context.Background(), bridge.BucketName, key, minio.StatObjectOptions{})
		if err != nil {
			promise.Resolve(c.NewNull())
			return c.NewUndefined(), nil
		}

		r2obj := buildR2Object(c, key, stat.Size, stat.ETag, stat.ContentType, stat.UserMetadata)
		promise.Resolve(r2obj)
		return c.NewUndefined(), nil
	}, true)
	bucket.SetPropertyStr("head", headFn)

	// list(opts?) -> Promise<R2Objects>
	listFn := ctx.Function(func(this *qjs.This) (*qjs.Value, error) {
		c := this.Context()
		args := this.Args()
		promise := this.Promise()

		var prefix, cursor, delimiter string
		limit := 1000

		if len(args) > 0 && args[0].IsObject() {
			opts := args[0]
			pVal := opts.GetPropertyStr("prefix")
			if !pVal.IsUndefined() && !pVal.IsNull() {
				prefix = pVal.String()
			}
			pVal.Free()

			cVal := opts.GetPropertyStr("cursor")
			if !cVal.IsUndefined() && !cVal.IsNull() {
				cursor = cVal.String()
			}
			cVal.Free()

			dVal := opts.GetPropertyStr("delimiter")
			if !dVal.IsUndefined() && !dVal.IsNull() {
				delimiter = dVal.String()
			}
			dVal.Free()

			lVal := opts.GetPropertyStr("limit")
			if !lVal.IsUndefined() && !lVal.IsNull() {
				limit = int(lVal.Int32())
			}
			lVal.Free()
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
			// CommonPrefixes show up as empty Key with a Prefix field in some S3 impls.
			// Minio-go for delimiter mode returns them differently.
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
		promise.Resolve(c.ParseJSON(string(data)))
		return c.NewUndefined(), nil
	}, true)
	bucket.SetPropertyStr("list", listFn)

	// createSignedUrl(key, opts?) -> Promise<string>
	// opts: { expiresIn?: number } (seconds, default 3600, max 604800)
	if bridge.Client != nil || bridge.PresignClient != nil {
		signFn := ctx.Function(func(this *qjs.This) (*qjs.Value, error) {
			c := this.Context()
			args := this.Args()
			promise := this.Promise()
			if len(args) == 0 {
				promise.Reject(c.NewError(fmt.Errorf("BUCKET.createSignedUrl requires a key argument")))
				return c.NewUndefined(), nil
			}
			key := args[0].String()

			expiry := 3600 // default 1 hour
			if len(args) > 1 && args[1].IsObject() {
				eVal := args[1].GetPropertyStr("expiresIn")
				if !eVal.IsUndefined() && !eVal.IsNull() {
					expiry = int(eVal.Int32())
				}
				eVal.Free()
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
					promise.Reject(c.NewError(fmt.Errorf("creating signed URL: presign client not configured for public S3 host")))
					return c.NewUndefined(), nil
				}
				signClient = bridge.Client
			}
			if signClient == nil {
				promise.Reject(c.NewError(fmt.Errorf("creating signed URL: storage client not configured")))
				return c.NewUndefined(), nil
			}

			presigned, err := signClient.PresignedGetObject(
				context.Background(),
				bridge.BucketName,
				key,
				time.Duration(expiry)*time.Second,
				nil,
			)
			if err != nil {
				promise.Reject(c.NewError(fmt.Errorf("creating signed URL: %w", err)))
				return c.NewUndefined(), nil
			}

			promise.Resolve(c.NewString(presigned.String()))
			return c.NewUndefined(), nil
		}, true)
		bucket.SetPropertyStr("createSignedUrl", signFn)
	}

	// publicUrl(key) -> string
	// Returns a direct object URL at {publicS3URL}/{bucket}/{key}. This is
	// intended for buckets with public-read enabled.
	if bridge.PublicS3URL != "" {
		publicURLFn := ctx.Function(func(this *qjs.This) (*qjs.Value, error) {
			c := this.Context()
			args := this.Args()
			if len(args) == 0 {
				return nil, fmt.Errorf("BUCKET.publicUrl requires a key argument")
			}
			key := args[0].String()

			objectURL, err := buildPublicObjectURL(bridge.PublicS3URL, bridge.BucketName, key)
			if err != nil {
				return nil, fmt.Errorf("creating public object URL: %w", err)
			}

			return c.NewString(objectURL), nil
		}, false)
		bucket.SetPropertyStr("publicUrl", publicURLFn)
	}

	return bucket
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

func coerceStoragePutBody(v *qjs.Value) ([]byte, error) {
	if v.IsString() {
		return []byte(v.String()), nil
	}

	if v.IsByteArray() {
		return v.ToByteArray(), nil
	}

	if qjs.IsTypedArray(v) {
		buf, err := qjs.JsTypedArrayToGo(v)
		if err != nil {
			return nil, fmt.Errorf("BUCKET.put failed to read TypedArray body: %w", err)
		}
		return buf, nil
	}

	if v.IsGlobalInstanceOf("Blob") {
		buf, err := blobToBytes(v)
		if err != nil {
			return nil, fmt.Errorf("BUCKET.put failed to read Blob body: %w", err)
		}
		return buf, nil
	}

	return nil, fmt.Errorf("BUCKET.put currently supports string, ArrayBuffer, TypedArray, DataView, Blob, and File values")
}

func blobToBytes(blob *qjs.Value) ([]byte, error) {
	arrayBufferResult, err := blob.InvokeJS("arrayBuffer")
	if err != nil {
		return nil, err
	}
	defer arrayBufferResult.Free()

	arrayBufferValue := arrayBufferResult
	if arrayBufferResult.IsPromise() {
		awaited, err := arrayBufferResult.Await()
		if err != nil {
			return nil, err
		}
		defer awaited.Free()
		arrayBufferValue = awaited
	}

	if arrayBufferValue.IsByteArray() {
		return arrayBufferValue.ToByteArray(), nil
	}

	if qjs.IsTypedArray(arrayBufferValue) {
		return qjs.JsTypedArrayToGo(arrayBufferValue)
	}

	return nil, fmt.Errorf("arrayBuffer() returned unsupported type %s", arrayBufferValue.Type())
}

// buildR2Object creates a JS object matching the Cloudflare R2Object shape.
func buildR2Object(c *qjs.Context, key string, size int64, etag string, contentType string, customMeta map[string]string) *qjs.Value {
	obj := c.NewObject()
	obj.SetPropertyStr("key", c.NewString(key))
	obj.SetPropertyStr("size", c.NewFloat64(float64(size)))
	obj.SetPropertyStr("etag", c.NewString(etag))
	obj.SetPropertyStr("httpEtag", c.NewString("\""+etag+"\""))
	obj.SetPropertyStr("version", c.NewString(etag))
	obj.SetPropertyStr("storageClass", c.NewString("STANDARD"))

	httpMeta := c.NewObject()
	if contentType != "" {
		httpMeta.SetPropertyStr("contentType", c.NewString(contentType))
	}
	obj.SetPropertyStr("httpMetadata", httpMeta)

	cm := c.NewObject()
	for k, v := range customMeta {
		cm.SetPropertyStr(k, c.NewString(v))
	}
	obj.SetPropertyStr("customMetadata", cm)

	checksums := c.NewObject()
	obj.SetPropertyStr("checksums", checksums)

	return obj
}

// buildR2ObjectBody extends R2Object with body reading methods.
func buildR2ObjectBody(c *qjs.Context, key string, data []byte, stat *minio.ObjectInfo) *qjs.Value {
	obj := buildR2Object(c, key, stat.Size, stat.ETag, stat.ContentType, stat.UserMetadata)
	obj.SetPropertyStr("uploaded", c.NewFloat64(float64(stat.LastModified.UnixMilli())))

	bodyUsed := false
	bodyData := string(data)
	markBodyUsed := func() {
		bodyUsed = true
		obj.SetPropertyStr("bodyUsed", c.NewBool(true))
	}

	// text() -> Promise<string>
	textFn := c.Function(func(this *qjs.This) (*qjs.Value, error) {
		cc := this.Context()
		p := this.Promise()
		if bodyUsed {
			p.Reject(cc.NewError(fmt.Errorf("body already consumed")))
			return cc.NewUndefined(), nil
		}
		markBodyUsed()
		p.Resolve(cc.NewString(bodyData))
		return cc.NewUndefined(), nil
	}, true)
	obj.SetPropertyStr("text", textFn)

	// arrayBuffer() -> Promise<ArrayBuffer>
	abFn := c.Function(func(this *qjs.This) (*qjs.Value, error) {
		cc := this.Context()
		p := this.Promise()
		if bodyUsed {
			p.Reject(cc.NewError(fmt.Errorf("body already consumed")))
			return cc.NewUndefined(), nil
		}
		markBodyUsed()
		p.Resolve(cc.NewArrayBuffer(data))
		return cc.NewUndefined(), nil
	}, true)
	obj.SetPropertyStr("arrayBuffer", abFn)

	// json() -> Promise<any>
	jsonFn := c.Function(func(this *qjs.This) (*qjs.Value, error) {
		cc := this.Context()
		p := this.Promise()
		if bodyUsed {
			p.Reject(cc.NewError(fmt.Errorf("body already consumed")))
			return cc.NewUndefined(), nil
		}
		markBodyUsed()
		if !json.Valid([]byte(bodyData)) {
			p.Reject(cc.NewError(fmt.Errorf("invalid JSON")))
			return cc.NewUndefined(), nil
		}
		parsed := cc.ParseJSON(bodyData)
		p.Resolve(parsed)
		return cc.NewUndefined(), nil
	}, true)
	obj.SetPropertyStr("json", jsonFn)

	obj.SetPropertyStr("bodyUsed", c.NewBool(false))

	return obj
}
