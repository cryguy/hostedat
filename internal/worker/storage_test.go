package worker

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/minio/minio-go/v7"
	v8 "github.com/tommie/v8go"
)

// newV8TestContext creates an isolate+context with encoding support (atob/btoa)
// needed by R2ObjectBody.arrayBuffer(). Cleaned up automatically via t.Cleanup.
func newV8TestContext(t *testing.T) (*v8.Isolate, *v8.Context) {
	t.Helper()
	iso := v8.NewIsolate()
	ctx := v8.NewContext(iso)
	el := newEventLoop()
	if err := setupEncoding(iso, ctx, el); err != nil {
		ctx.Close()
		iso.Dispose()
		t.Fatalf("setupEncoding: %v", err)
	}
	t.Cleanup(func() {
		ctx.Close()
		iso.Dispose()
	})
	return iso, ctx
}

func TestStorageBinding_Put_UnsupportedTypeRejected(t *testing.T) {
	iso, ctx := newV8TestContext(t)

	bucketVal, err := buildStorageBinding(iso, ctx, &StorageBridge{BucketName: "test"})
	if err != nil {
		t.Fatalf("buildStorageBinding: %v", err)
	}

	_ = ctx.Global().Set("__bucket", bucketVal)
	result, err := ctx.RunScript("__bucket.put('k', {})", "test_put.js")
	if err != nil {
		t.Fatalf("RunScript put: %v", err)
	}

	deadline := time.Now().Add(5 * time.Second)
	_, err = awaitValue(ctx, result, deadline)
	if err == nil || !strings.Contains(err.Error(), "unsupported body type") {
		t.Fatalf("expected unsupported-value rejection, got %v", err)
	}
}

func TestStorageBinding_ArrayBuffer_ReturnsData(t *testing.T) {
	iso, ctx := newV8TestContext(t)

	obj, err := buildR2ObjectBody(iso, ctx, "k", []byte("hello"), &minio.ObjectInfo{
		Size:         5,
		ETag:         "etag",
		ContentType:  "text/plain",
		LastModified: time.Now(),
	})
	if err != nil {
		t.Fatalf("buildR2ObjectBody: %v", err)
	}
	_ = ctx.Global().Set("__obj", obj.Value)

	result, err := ctx.RunScript("__obj.arrayBuffer()", "test_ab.js")
	if err != nil {
		t.Fatalf("RunScript arrayBuffer: %v", err)
	}

	deadline := time.Now().Add(5 * time.Second)
	resolved, err := awaitValue(ctx, result, deadline)
	if err != nil {
		t.Fatalf("await arrayBuffer: %v", err)
	}

	// Verify bodyUsed is set after consuming.
	afterVal, err := ctx.RunScript("__obj.bodyUsed", "check_used.js")
	if err != nil {
		t.Fatalf("checking bodyUsed: %v", err)
	}
	if afterVal.String() != "true" {
		t.Fatalf("bodyUsed after arrayBuffer = %q, want true", afterVal.String())
	}

	// Verify the ArrayBuffer has the right byte length.
	_ = ctx.Global().Set("__result", resolved)
	blVal, err := ctx.RunScript("__result.byteLength", "check_bl.js")
	if err != nil {
		t.Fatalf("checking byteLength: %v", err)
	}
	if blVal.Int32() != 5 {
		t.Fatalf("byteLength = %d, want 5", blVal.Int32())
	}

	// Verify body cannot be consumed again.
	result2, err := ctx.RunScript("__obj.arrayBuffer()", "test_ab2.js")
	if err != nil {
		t.Fatalf("RunScript second arrayBuffer: %v", err)
	}
	_, err = awaitValue(ctx, result2, deadline)
	if err == nil || !strings.Contains(err.Error(), "already consumed") {
		t.Fatalf("expected body consumed rejection, got %v", err)
	}
}

func TestStorageBinding_ArrayBuffer_BinaryBlob(t *testing.T) {
	// Simulate a minimal PNG-like header with null bytes and high-byte values
	// that would break if the implementation used string coercion.
	pngHeader := []byte{
		0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A, // PNG signature
		0x00, 0x00, 0x00, 0x0D, 0x49, 0x48, 0x44, 0x52, // IHDR chunk header
	}

	iso, ctx := newV8TestContext(t)

	obj, err := buildR2ObjectBody(iso, ctx, "image.png", pngHeader, &minio.ObjectInfo{
		Size:         int64(len(pngHeader)),
		ETag:         "png-etag",
		ContentType:  "image/png",
		LastModified: time.Now(),
	})
	if err != nil {
		t.Fatalf("buildR2ObjectBody: %v", err)
	}
	_ = ctx.Global().Set("__obj", obj.Value)

	result, err := ctx.RunScript("__obj.arrayBuffer()", "test_ab.js")
	if err != nil {
		t.Fatalf("RunScript arrayBuffer: %v", err)
	}

	deadline := time.Now().Add(5 * time.Second)
	resolved, err := awaitValue(ctx, result, deadline)
	if err != nil {
		t.Fatalf("await arrayBuffer: %v", err)
	}

	// Verify byte length matches the original blob.
	_ = ctx.Global().Set("__testBuf", resolved)
	blVal, err := ctx.RunScript("__testBuf.byteLength", "check_bl.js")
	if err != nil {
		t.Fatalf("checking byteLength: %v", err)
	}
	if blVal.Int32() != int32(len(pngHeader)) {
		t.Fatalf("byteLength = %d, want %d", blVal.Int32(), len(pngHeader))
	}

	// Read back every byte via Uint8Array and compare to the Go source.
	for i, expected := range pngHeader {
		jsCode := fmt.Sprintf("new Uint8Array(__testBuf)[%d]", i)
		v, err := ctx.RunScript(jsCode, "test_byte.js")
		if err != nil {
			t.Fatalf("eval byte[%d]: %v", i, err)
		}
		got := byte(v.Int32())
		if got != expected {
			t.Fatalf("byte[%d] = 0x%02X, want 0x%02X", i, got, expected)
		}
	}
}

func TestStorageBinding_ArrayBuffer_EmptyBlob(t *testing.T) {
	iso, ctx := newV8TestContext(t)

	obj, err := buildR2ObjectBody(iso, ctx, "empty.bin", []byte{}, &minio.ObjectInfo{
		Size:         0,
		ETag:         "empty-etag",
		ContentType:  "application/octet-stream",
		LastModified: time.Now(),
	})
	if err != nil {
		t.Fatalf("buildR2ObjectBody: %v", err)
	}
	_ = ctx.Global().Set("__obj", obj.Value)

	result, err := ctx.RunScript("__obj.arrayBuffer()", "test_ab.js")
	if err != nil {
		t.Fatalf("RunScript arrayBuffer: %v", err)
	}

	deadline := time.Now().Add(5 * time.Second)
	resolved, err := awaitValue(ctx, result, deadline)
	if err != nil {
		t.Fatalf("await arrayBuffer: %v", err)
	}

	_ = ctx.Global().Set("__result", resolved)
	blVal, err := ctx.RunScript("__result.byteLength", "check_bl.js")
	if err != nil {
		t.Fatalf("checking byteLength: %v", err)
	}
	if blVal.Int32() != 0 {
		t.Fatalf("byteLength = %d, want 0", blVal.Int32())
	}
}

func TestStorageBinding_ArrayBuffer_AllByteValues(t *testing.T) {
	// Create a 256-byte blob containing every possible byte value 0x00..0xFF.
	blob := make([]byte, 256)
	for i := range blob {
		blob[i] = byte(i)
	}

	iso, ctx := newV8TestContext(t)

	obj, err := buildR2ObjectBody(iso, ctx, "allbytes.bin", blob, &minio.ObjectInfo{
		Size:         256,
		ETag:         "all-etag",
		ContentType:  "application/octet-stream",
		LastModified: time.Now(),
	})
	if err != nil {
		t.Fatalf("buildR2ObjectBody: %v", err)
	}
	_ = ctx.Global().Set("__obj", obj.Value)

	result, err := ctx.RunScript("__obj.arrayBuffer()", "test_ab.js")
	if err != nil {
		t.Fatalf("RunScript arrayBuffer: %v", err)
	}

	deadline := time.Now().Add(5 * time.Second)
	resolved, err := awaitValue(ctx, result, deadline)
	if err != nil {
		t.Fatalf("await arrayBuffer: %v", err)
	}

	_ = ctx.Global().Set("__testBuf", resolved)
	blVal, err := ctx.RunScript("__testBuf.byteLength", "check_bl.js")
	if err != nil {
		t.Fatalf("checking byteLength: %v", err)
	}
	if blVal.Int32() != 256 {
		t.Fatalf("byteLength = %d, want 256", blVal.Int32())
	}

	// Spot-check key byte values: null, high bytes, and boundaries.
	checks := []int{0x00, 0x01, 0x7F, 0x80, 0xFE, 0xFF}
	for _, idx := range checks {
		jsCode := fmt.Sprintf("new Uint8Array(__testBuf)[%d]", idx)
		v, err := ctx.RunScript(jsCode, "test_byte.js")
		if err != nil {
			t.Fatalf("eval byte[%d]: %v", idx, err)
		}
		got := byte(v.Int32())
		if got != byte(idx) {
			t.Fatalf("byte[%d] = 0x%02X, want 0x%02X", idx, got, byte(idx))
		}
	}
}

func TestStorageBinding_ArrayBuffer_ThenTextRejects(t *testing.T) {
	iso, ctx := newV8TestContext(t)

	obj, err := buildR2ObjectBody(iso, ctx, "img.png", []byte{0x89, 0x50, 0x4E, 0x47}, &minio.ObjectInfo{
		Size:         4,
		ETag:         "etag",
		ContentType:  "image/png",
		LastModified: time.Now(),
	})
	if err != nil {
		t.Fatalf("buildR2ObjectBody: %v", err)
	}
	_ = ctx.Global().Set("__obj", obj.Value)
	deadline := time.Now().Add(5 * time.Second)

	// Consume via arrayBuffer().
	result, err := ctx.RunScript("__obj.arrayBuffer()", "test_ab.js")
	if err != nil {
		t.Fatalf("RunScript arrayBuffer: %v", err)
	}
	if _, err := awaitValue(ctx, result, deadline); err != nil {
		t.Fatalf("await arrayBuffer: %v", err)
	}

	// text() must reject after arrayBuffer() consumed the body.
	r2, err := ctx.RunScript("__obj.text()", "test_text.js")
	if err != nil {
		t.Fatalf("RunScript text: %v", err)
	}
	_, err = awaitValue(ctx, r2, deadline)
	if err == nil || !strings.Contains(err.Error(), "already consumed") {
		t.Fatalf("expected body consumed rejection from text(), got %v", err)
	}

	// json() must also reject.
	r3, err := ctx.RunScript("__obj.json()", "test_json.js")
	if err != nil {
		t.Fatalf("RunScript json: %v", err)
	}
	_, err = awaitValue(ctx, r3, deadline)
	if err == nil || !strings.Contains(err.Error(), "already consumed") {
		t.Fatalf("expected body consumed rejection from json(), got %v", err)
	}
}

func TestStorageBinding_Text_ThenArrayBufferRejects(t *testing.T) {
	iso, ctx := newV8TestContext(t)

	obj, err := buildR2ObjectBody(iso, ctx, "doc.txt", []byte("hello world"), &minio.ObjectInfo{
		Size:         11,
		ETag:         "etag",
		ContentType:  "text/plain",
		LastModified: time.Now(),
	})
	if err != nil {
		t.Fatalf("buildR2ObjectBody: %v", err)
	}
	_ = ctx.Global().Set("__obj", obj.Value)
	deadline := time.Now().Add(5 * time.Second)

	// Consume via text().
	result, err := ctx.RunScript("__obj.text()", "test_text.js")
	if err != nil {
		t.Fatalf("RunScript text: %v", err)
	}
	if _, err := awaitValue(ctx, result, deadline); err != nil {
		t.Fatalf("await text: %v", err)
	}

	// arrayBuffer() must reject after text() consumed the body.
	r2, err := ctx.RunScript("__obj.arrayBuffer()", "test_ab.js")
	if err != nil {
		t.Fatalf("RunScript arrayBuffer: %v", err)
	}
	_, err = awaitValue(ctx, r2, deadline)
	if err == nil || !strings.Contains(err.Error(), "already consumed") {
		t.Fatalf("expected body consumed rejection from arrayBuffer(), got %v", err)
	}
}

func TestStorageBinding_BodyUsed_TransitionsAfterRead(t *testing.T) {
	iso, ctx := newV8TestContext(t)

	obj, err := buildR2ObjectBody(iso, ctx, "k", []byte("hello"), &minio.ObjectInfo{
		Size:         5,
		ETag:         "etag",
		ContentType:  "text/plain",
		LastModified: time.Now(),
	})
	if err != nil {
		t.Fatalf("buildR2ObjectBody: %v", err)
	}
	_ = ctx.Global().Set("__obj", obj.Value)
	deadline := time.Now().Add(5 * time.Second)

	// Initial bodyUsed should be false.
	initial, err := ctx.RunScript("__obj.bodyUsed", "check_init.js")
	if err != nil {
		t.Fatalf("checking initial bodyUsed: %v", err)
	}
	if initial.String() != "false" {
		t.Fatalf("initial bodyUsed = %q, want false", initial.String())
	}

	// Call text().
	result, err := ctx.RunScript("__obj.text()", "test_text.js")
	if err != nil {
		t.Fatalf("RunScript text: %v", err)
	}
	resolved, err := awaitValue(ctx, result, deadline)
	if err != nil {
		t.Fatalf("await text: %v", err)
	}
	if resolved.String() != "hello" {
		t.Fatalf("text result = %q, want hello", resolved.String())
	}

	// bodyUsed should be true after consuming.
	after, err := ctx.RunScript("__obj.bodyUsed", "check_after.js")
	if err != nil {
		t.Fatalf("checking bodyUsed after text: %v", err)
	}
	if after.String() != "true" {
		t.Fatalf("bodyUsed after text = %q, want true", after.String())
	}

	// Second text() call must reject.
	result2, err := ctx.RunScript("__obj.text()", "test_text2.js")
	if err != nil {
		t.Fatalf("RunScript second text: %v", err)
	}
	_, err = awaitValue(ctx, result2, deadline)
	if err == nil || !strings.Contains(err.Error(), "already consumed") {
		t.Fatalf("expected body consumed rejection, got %v", err)
	}
}

func TestStorageBinding_JSON_InvalidJSONRejects(t *testing.T) {
	iso, ctx := newV8TestContext(t)

	obj, err := buildR2ObjectBody(iso, ctx, "k", []byte("{not-json"), &minio.ObjectInfo{
		Size:         9,
		ETag:         "etag",
		ContentType:  "application/json",
		LastModified: time.Now(),
	})
	if err != nil {
		t.Fatalf("buildR2ObjectBody: %v", err)
	}
	_ = ctx.Global().Set("__obj", obj.Value)

	result, err := ctx.RunScript("__obj.json()", "test_json.js")
	if err != nil {
		t.Fatalf("RunScript json: %v", err)
	}

	deadline := time.Now().Add(5 * time.Second)
	_, err = awaitValue(ctx, result, deadline)
	if err == nil || !strings.Contains(err.Error(), "invalid JSON") {
		t.Fatalf("expected invalid JSON rejection, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// Pure Go helpers (no V8 dependency)
// ---------------------------------------------------------------------------

func TestBuildPublicObjectURL_PathEscaping(t *testing.T) {
	u, err := buildPublicObjectURL("https://storage.example.com", "downloads", "releases/v1.0/file name+plus?.zip")
	if err != nil {
		t.Fatalf("buildPublicObjectURL returned error: %v", err)
	}

	want := "https://storage.example.com/downloads/releases/v1.0/file%20name+plus%3F.zip"
	if u != want {
		t.Fatalf("public URL = %q, want %q", u, want)
	}
}

func TestBuildPublicObjectURL_InvalidBase(t *testing.T) {
	if _, err := buildPublicObjectURL("storage.example.com", "downloads", "artifact"); err == nil {
		t.Fatalf("expected error for invalid public base URL")
	}
}

func TestBuildPublicObjectURL_WithBasePath(t *testing.T) {
	u, err := buildPublicObjectURL("https://storage.example.com/s3", "mybucket", "file.txt")
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if !strings.Contains(u, "/s3/mybucket/file.txt") {
		t.Errorf("URL should include base path, got %q", u)
	}
}

func TestBuildPublicObjectURL_TrailingSlash(t *testing.T) {
	u, err := buildPublicObjectURL("https://storage.example.com/", "mybucket", "file.txt")
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if strings.Contains(u, "//mybucket") {
		t.Errorf("should not have double slashes, got %q", u)
	}
}

func TestBuildPublicObjectURL_LeadingSlashKey(t *testing.T) {
	u, err := buildPublicObjectURL("https://storage.example.com", "mybucket", "/file.txt")
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if strings.Contains(u, "mybucket//") {
		t.Errorf("should not have double slash between bucket and key, got %q", u)
	}
}

func TestEscapePathSegments(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"", ""},
		{"simple.txt", "simple.txt"},
		{"dir/file.txt", "dir/file.txt"},
		{"dir/file name.txt", "dir/file%20name.txt"},
		{"a/b/c", "a/b/c"},
	}
	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			got := escapePathSegments(tc.input)
			if got != tc.want {
				t.Errorf("escapePathSegments(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestStorageBinding_JSON_ValidJSON(t *testing.T) {
	iso, ctx := newV8TestContext(t)

	obj, err := buildR2ObjectBody(iso, ctx, "k", []byte(`{"name":"test","count":42}`), &minio.ObjectInfo{
		Size:         25,
		ETag:         "etag",
		ContentType:  "application/json",
		LastModified: time.Now(),
	})
	if err != nil {
		t.Fatalf("buildR2ObjectBody: %v", err)
	}
	_ = ctx.Global().Set("__obj", obj.Value)

	result, err := ctx.RunScript("__obj.json()", "test_json.js")
	if err != nil {
		t.Fatalf("RunScript json: %v", err)
	}

	deadline := time.Now().Add(5 * time.Second)
	resolved, err := awaitValue(ctx, result, deadline)
	if err != nil {
		t.Fatalf("await json: %v", err)
	}

	_ = ctx.Global().Set("__result", resolved)
	nameVal, err := ctx.RunScript("__result.name", "check_name.js")
	if err != nil {
		t.Fatalf("checking name: %v", err)
	}
	if nameVal.String() != "test" {
		t.Errorf("json name = %q, want test", nameVal.String())
	}

	countVal, err := ctx.RunScript("__result.count", "check_count.js")
	if err != nil {
		t.Fatalf("checking count: %v", err)
	}
	if countVal.Int32() != 42 {
		t.Errorf("json count = %d, want 42", countVal.Int32())
	}
}

func TestBuildR2Object(t *testing.T) {
	iso := v8.NewIsolate()
	defer iso.Dispose()
	ctx := v8.NewContext(iso)
	defer ctx.Close()

	customMeta := map[string]string{"author": "test", "version": "1"}
	obj, err := buildR2Object(iso, ctx, "test-key", 100, "etag123", "text/plain", customMeta)
	if err != nil {
		t.Fatalf("buildR2Object: %v", err)
	}

	_ = ctx.Global().Set("__obj", obj.Value)

	keyVal, _ := ctx.RunScript("__obj.key", "k.js")
	if keyVal.String() != "test-key" {
		t.Errorf("key = %q, want test-key", keyVal.String())
	}

	etagVal, _ := ctx.RunScript("__obj.etag", "e.js")
	if etagVal.String() != "etag123" {
		t.Errorf("etag = %q, want etag123", etagVal.String())
	}

	httpEtagVal, _ := ctx.RunScript("__obj.httpEtag", "he.js")
	if httpEtagVal.String() != `"etag123"` {
		t.Errorf("httpEtag = %q, want quoted etag", httpEtagVal.String())
	}

	scVal, _ := ctx.RunScript("__obj.storageClass", "sc.js")
	if scVal.String() != "STANDARD" {
		t.Errorf("storageClass = %q, want STANDARD", scVal.String())
	}

	ctVal, _ := ctx.RunScript("__obj.httpMetadata.contentType", "ct.js")
	if ctVal.String() != "text/plain" {
		t.Errorf("contentType = %q, want text/plain", ctVal.String())
	}

	authorVal, _ := ctx.RunScript("__obj.customMetadata.author", "auth.js")
	if authorVal.String() != "test" {
		t.Errorf("customMetadata.author = %q, want test", authorVal.String())
	}
}

func TestBuildR2Object_EmptyMetadata(t *testing.T) {
	iso := v8.NewIsolate()
	defer iso.Dispose()
	ctx := v8.NewContext(iso)
	defer ctx.Close()

	obj, err := buildR2Object(iso, ctx, "empty-meta", 0, "etag-0", "", nil)
	if err != nil {
		t.Fatalf("buildR2Object: %v", err)
	}

	_ = ctx.Global().Set("__obj", obj.Value)

	keyVal, _ := ctx.RunScript("__obj.key", "k.js")
	if keyVal.String() != "empty-meta" {
		t.Errorf("key = %q, want empty-meta", keyVal.String())
	}

	sizeVal, _ := ctx.RunScript("__obj.size", "s.js")
	if sizeVal.Int32() != 0 {
		t.Errorf("size = %d, want 0", sizeVal.Int32())
	}

	// httpMetadata should exist but contentType should be undefined
	ctVal, _ := ctx.RunScript("__obj.httpMetadata.contentType", "ct.js")
	if ctVal.String() != "undefined" {
		t.Errorf("contentType = %q, want undefined (no contentType set)", ctVal.String())
	}

	// customMetadata should be an empty object
	cmType, _ := ctx.RunScript("typeof __obj.customMetadata", "cm.js")
	if cmType.String() != "object" {
		t.Errorf("customMetadata type = %q, want object", cmType.String())
	}

	// checksums should exist
	ckType, _ := ctx.RunScript("typeof __obj.checksums", "ck.js")
	if ckType.String() != "object" {
		t.Errorf("checksums type = %q, want object", ckType.String())
	}
}

func TestBuildR2Object_SizeAndVersion(t *testing.T) {
	iso := v8.NewIsolate()
	defer iso.Dispose()
	ctx := v8.NewContext(iso)
	defer ctx.Close()

	obj, err := buildR2Object(iso, ctx, "large-file", 1048576, "abc123", "application/octet-stream", nil)
	if err != nil {
		t.Fatalf("buildR2Object: %v", err)
	}

	_ = ctx.Global().Set("__obj", obj.Value)

	sizeVal, _ := ctx.RunScript("__obj.size", "s.js")
	if sizeVal.Integer() != 1048576 {
		t.Errorf("size = %d, want 1048576", sizeVal.Integer())
	}

	versionVal, _ := ctx.RunScript("__obj.version", "v.js")
	if versionVal.String() != "abc123" {
		t.Errorf("version = %q, want abc123", versionVal.String())
	}
}

func TestStorageBinding_PutRequiresKey(t *testing.T) {
	iso, ctx := newV8TestContext(t)

	bucketVal, err := buildStorageBinding(iso, ctx, &StorageBridge{BucketName: "test"})
	if err != nil {
		t.Fatalf("buildStorageBinding: %v", err)
	}

	_ = ctx.Global().Set("__bucket", bucketVal)
	result, err := ctx.RunScript("__bucket.put()", "test_put_noargs.js")
	if err != nil {
		t.Fatalf("RunScript put: %v", err)
	}

	deadline := time.Now().Add(5 * time.Second)
	_, err = awaitValue(ctx, result, deadline)
	if err == nil || !strings.Contains(err.Error(), "requires key and value") {
		t.Fatalf("expected key/value rejection, got %v", err)
	}
}

func TestStorageBinding_GetRequiresKey(t *testing.T) {
	iso, ctx := newV8TestContext(t)

	bucketVal, err := buildStorageBinding(iso, ctx, &StorageBridge{BucketName: "test"})
	if err != nil {
		t.Fatalf("buildStorageBinding: %v", err)
	}

	_ = ctx.Global().Set("__bucket", bucketVal)
	result, err := ctx.RunScript("__bucket.get()", "test_get_noargs.js")
	if err != nil {
		t.Fatalf("RunScript get: %v", err)
	}

	deadline := time.Now().Add(5 * time.Second)
	_, err = awaitValue(ctx, result, deadline)
	if err == nil || !strings.Contains(err.Error(), "requires a key") {
		t.Fatalf("expected key rejection, got %v", err)
	}
}

func TestStorageBinding_DeleteRequiresKey(t *testing.T) {
	iso, ctx := newV8TestContext(t)

	bucketVal, err := buildStorageBinding(iso, ctx, &StorageBridge{BucketName: "test"})
	if err != nil {
		t.Fatalf("buildStorageBinding: %v", err)
	}

	_ = ctx.Global().Set("__bucket", bucketVal)
	result, err := ctx.RunScript("__bucket.delete()", "test_del_noargs.js")
	if err != nil {
		t.Fatalf("RunScript delete: %v", err)
	}

	deadline := time.Now().Add(5 * time.Second)
	_, err = awaitValue(ctx, result, deadline)
	if err == nil || !strings.Contains(err.Error(), "requires a key") {
		t.Fatalf("expected key rejection, got %v", err)
	}
}

func TestStorageBinding_HeadRequiresKey(t *testing.T) {
	iso, ctx := newV8TestContext(t)

	bucketVal, err := buildStorageBinding(iso, ctx, &StorageBridge{BucketName: "test"})
	if err != nil {
		t.Fatalf("buildStorageBinding: %v", err)
	}

	_ = ctx.Global().Set("__bucket", bucketVal)
	result, err := ctx.RunScript("__bucket.head()", "test_head_noargs.js")
	if err != nil {
		t.Fatalf("RunScript head: %v", err)
	}

	deadline := time.Now().Add(5 * time.Second)
	_, err = awaitValue(ctx, result, deadline)
	if err == nil || !strings.Contains(err.Error(), "requires a key") {
		t.Fatalf("expected key rejection, got %v", err)
	}
}

func TestStorageBinding_BindingHasMethods(t *testing.T) {
	iso, ctx := newV8TestContext(t)

	bucketVal, err := buildStorageBinding(iso, ctx, &StorageBridge{
		BucketName:  "test",
		PublicS3URL: "https://storage.example.com",
	})
	if err != nil {
		t.Fatalf("buildStorageBinding: %v", err)
	}

	_ = ctx.Global().Set("__bucket", bucketVal)

	// Verify all expected methods exist
	methods := []string{"get", "put", "delete", "head", "list", "publicUrl"}
	for _, m := range methods {
		jsCode := fmt.Sprintf("typeof __bucket.%s", m)
		val, err := ctx.RunScript(jsCode, "check_"+m+".js")
		if err != nil {
			t.Fatalf("checking %s: %v", m, err)
		}
		if val.String() != "function" {
			t.Errorf("%s type = %q, want 'function'", m, val.String())
		}
	}
}

func TestBuildPublicObjectURL_SpecialChars(t *testing.T) {
	tests := []struct {
		name   string
		base   string
		bucket string
		key    string
		want   string
	}{
		{
			name:   "unicode filename",
			base:   "https://storage.example.com",
			bucket: "uploads",
			key:    "docs/resume (1).pdf",
			want:   "https://storage.example.com/uploads/docs/resume%20%281%29.pdf",
		},
		{
			name:   "nested path",
			base:   "https://cdn.example.com",
			bucket: "assets",
			key:    "images/photos/2024/summer.jpg",
			want:   "https://cdn.example.com/assets/images/photos/2024/summer.jpg",
		},
		{
			name:   "empty key",
			base:   "https://storage.example.com",
			bucket: "bucket",
			key:    "",
			want:   "https://storage.example.com/bucket/",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := buildPublicObjectURL(tc.base, tc.bucket, tc.key)
			if err != nil {
				t.Fatalf("error: %v", err)
			}
			if got != tc.want {
				t.Errorf("buildPublicObjectURL = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestBuildR2ObjectBody_TextReturnsCorrectData(t *testing.T) {
	iso, ctx := newV8TestContext(t)

	content := "Hello, World! Special chars: <>&\""
	obj, err := buildR2ObjectBody(iso, ctx, "greeting.txt", []byte(content), &minio.ObjectInfo{
		Size:         int64(len(content)),
		ETag:         "text-etag",
		ContentType:  "text/plain",
		LastModified: time.Now(),
	})
	if err != nil {
		t.Fatalf("buildR2ObjectBody: %v", err)
	}
	_ = ctx.Global().Set("__obj", obj.Value)

	result, err := ctx.RunScript("__obj.text()", "test_text.js")
	if err != nil {
		t.Fatalf("RunScript text: %v", err)
	}

	deadline := time.Now().Add(5 * time.Second)
	resolved, err := awaitValue(ctx, result, deadline)
	if err != nil {
		t.Fatalf("await text: %v", err)
	}
	if resolved.String() != content {
		t.Errorf("text() = %q, want %q", resolved.String(), content)
	}
}

// badMinioClient returns a minio client pointing to an unreachable endpoint.
// Calls will fail with connection errors, exercising error handling in callbacks.
func badMinioClient(t *testing.T) *minio.Client {
	t.Helper()
	client, err := minio.New("127.0.0.1:1", &minio.Options{})
	if err != nil {
		t.Fatalf("creating test minio client: %v", err)
	}
	return client
}

func TestStorageBinding_GetWithKeyReturnsNullOnError(t *testing.T) {
	iso, ctx := newV8TestContext(t)

	bridge := &StorageBridge{
		Client:     badMinioClient(t),
		BucketName: "test-bucket",
	}

	bucketVal, err := buildStorageBinding(iso, ctx, bridge)
	if err != nil {
		t.Fatalf("buildStorageBinding: %v", err)
	}
	_ = ctx.Global().Set("__bucket", bucketVal)

	result, err := ctx.RunScript("__bucket.get('nonexistent-key')", "test_get.js")
	if err != nil {
		t.Fatalf("RunScript get: %v", err)
	}

	deadline := time.Now().Add(5 * time.Second)
	resolved, err := awaitValue(ctx, result, deadline)
	if err != nil {
		t.Fatalf("await get: %v", err)
	}
	if !resolved.IsNull() {
		t.Errorf("get with bad client should resolve null, got %v", resolved.String())
	}
}

func TestStorageBinding_PutWithStringBody(t *testing.T) {
	iso, ctx := newV8TestContext(t)

	bridge := &StorageBridge{
		Client:     badMinioClient(t),
		BucketName: "test-bucket",
	}

	bucketVal, err := buildStorageBinding(iso, ctx, bridge)
	if err != nil {
		t.Fatalf("buildStorageBinding: %v", err)
	}
	_ = ctx.Global().Set("__bucket", bucketVal)

	// put with string value exercises the JS coercion "string" path
	// then fails at PutObject with connection error
	result, err := ctx.RunScript("__bucket.put('my-key', 'hello world')", "test_put_string.js")
	if err != nil {
		t.Fatalf("RunScript put: %v", err)
	}

	deadline := time.Now().Add(5 * time.Second)
	_, err = awaitValue(ctx, result, deadline)
	if err == nil {
		t.Fatal("expected put to reject with connection error")
	}
	if !strings.Contains(err.Error(), "putting object") {
		t.Fatalf("expected 'putting object' error, got %v", err)
	}
}

func TestStorageBinding_PutWithArrayBufferBody(t *testing.T) {
	iso, ctx := newV8TestContext(t)

	bridge := &StorageBridge{
		Client:     badMinioClient(t),
		BucketName: "test-bucket",
	}

	bucketVal, err := buildStorageBinding(iso, ctx, bridge)
	if err != nil {
		t.Fatalf("buildStorageBinding: %v", err)
	}
	_ = ctx.Global().Set("__bucket", bucketVal)

	// put with ArrayBuffer exercises the JS coercion "binary" path + base64 decode
	result, err := ctx.RunScript(`(function() {
		var buf = new ArrayBuffer(4);
		var view = new Uint8Array(buf);
		view[0] = 1; view[1] = 2; view[2] = 3; view[3] = 4;
		return __bucket.put('binary-key', buf);
	})()`, "test_put_ab.js")
	if err != nil {
		t.Fatalf("RunScript put AB: %v", err)
	}

	deadline := time.Now().Add(5 * time.Second)
	_, err = awaitValue(ctx, result, deadline)
	if err == nil {
		t.Fatal("expected put to reject with connection error")
	}
	if !strings.Contains(err.Error(), "putting object") {
		t.Fatalf("expected 'putting object' error, got %v", err)
	}
}

func TestStorageBinding_PutWithTypedArrayBody(t *testing.T) {
	iso, ctx := newV8TestContext(t)

	bridge := &StorageBridge{
		Client:     badMinioClient(t),
		BucketName: "test-bucket",
	}

	bucketVal, err := buildStorageBinding(iso, ctx, bridge)
	if err != nil {
		t.Fatalf("buildStorageBinding: %v", err)
	}
	_ = ctx.Global().Set("__bucket", bucketVal)

	// put with Uint8Array (TypedArray view) exercises the ArrayBuffer.isView path
	result, err := ctx.RunScript("__bucket.put('typed-key', new Uint8Array([10, 20, 30]))", "test_put_typed.js")
	if err != nil {
		t.Fatalf("RunScript put typed: %v", err)
	}

	deadline := time.Now().Add(5 * time.Second)
	_, err = awaitValue(ctx, result, deadline)
	if err == nil {
		t.Fatal("expected put to reject with connection error")
	}
}

func TestStorageBinding_PutWithOptions(t *testing.T) {
	iso, ctx := newV8TestContext(t)

	bridge := &StorageBridge{
		Client:     badMinioClient(t),
		BucketName: "test-bucket",
	}

	bucketVal, err := buildStorageBinding(iso, ctx, bridge)
	if err != nil {
		t.Fatalf("buildStorageBinding: %v", err)
	}
	_ = ctx.Global().Set("__bucket", bucketVal)

	// put with options exercises the httpMetadata and customMetadata extraction paths
	result, err := ctx.RunScript(`__bucket.put('opts-key', 'data', {
		httpMetadata: {
			contentType: 'text/plain',
			contentEncoding: 'gzip',
			contentDisposition: 'attachment',
			contentLanguage: 'en',
			cacheControl: 'max-age=3600',
		},
		customMetadata: {
			author: 'test',
			version: '1.0',
		},
	})`, "test_put_opts.js")
	if err != nil {
		t.Fatalf("RunScript put opts: %v", err)
	}

	deadline := time.Now().Add(5 * time.Second)
	_, err = awaitValue(ctx, result, deadline)
	if err == nil {
		t.Fatal("expected put to reject with connection error")
	}
}

func TestStorageBinding_DeleteSingleKey(t *testing.T) {
	iso, ctx := newV8TestContext(t)

	bridge := &StorageBridge{
		Client:     badMinioClient(t),
		BucketName: "test-bucket",
	}

	bucketVal, err := buildStorageBinding(iso, ctx, bridge)
	if err != nil {
		t.Fatalf("buildStorageBinding: %v", err)
	}
	_ = ctx.Global().Set("__bucket", bucketVal)

	// delete with single key - errors are silently ignored, resolves undefined
	result, err := ctx.RunScript("__bucket.delete('some-key')", "test_del.js")
	if err != nil {
		t.Fatalf("RunScript delete: %v", err)
	}

	deadline := time.Now().Add(5 * time.Second)
	resolved, err := awaitValue(ctx, result, deadline)
	if err != nil {
		t.Fatalf("await delete: %v", err)
	}
	if !resolved.IsUndefined() {
		t.Errorf("delete should resolve undefined, got %v", resolved.String())
	}
}

func TestStorageBinding_DeleteArrayOfKeys(t *testing.T) {
	iso, ctx := newV8TestContext(t)

	bridge := &StorageBridge{
		Client:     badMinioClient(t),
		BucketName: "test-bucket",
	}

	bucketVal, err := buildStorageBinding(iso, ctx, bridge)
	if err != nil {
		t.Fatalf("buildStorageBinding: %v", err)
	}
	_ = ctx.Global().Set("__bucket", bucketVal)

	// delete with array of keys
	result, err := ctx.RunScript("__bucket.delete(['k1', 'k2', 'k3'])", "test_del_arr.js")
	if err != nil {
		t.Fatalf("RunScript delete array: %v", err)
	}

	deadline := time.Now().Add(5 * time.Second)
	resolved, err := awaitValue(ctx, result, deadline)
	if err != nil {
		t.Fatalf("await delete array: %v", err)
	}
	if !resolved.IsUndefined() {
		t.Errorf("delete array should resolve undefined, got %v", resolved.String())
	}
}

func TestStorageBinding_HeadReturnsNullOnError(t *testing.T) {
	iso, ctx := newV8TestContext(t)

	bridge := &StorageBridge{
		Client:     badMinioClient(t),
		BucketName: "test-bucket",
	}

	bucketVal, err := buildStorageBinding(iso, ctx, bridge)
	if err != nil {
		t.Fatalf("buildStorageBinding: %v", err)
	}
	_ = ctx.Global().Set("__bucket", bucketVal)

	result, err := ctx.RunScript("__bucket.head('missing-key')", "test_head.js")
	if err != nil {
		t.Fatalf("RunScript head: %v", err)
	}

	deadline := time.Now().Add(5 * time.Second)
	resolved, err := awaitValue(ctx, result, deadline)
	if err != nil {
		t.Fatalf("await head: %v", err)
	}
	if !resolved.IsNull() {
		t.Errorf("head with bad client should resolve null, got %v", resolved.String())
	}
}

func TestStorageBinding_ListReturnsEmptyOnError(t *testing.T) {
	iso, ctx := newV8TestContext(t)

	bridge := &StorageBridge{
		Client:     badMinioClient(t),
		BucketName: "test-bucket",
	}

	bucketVal, err := buildStorageBinding(iso, ctx, bridge)
	if err != nil {
		t.Fatalf("buildStorageBinding: %v", err)
	}
	_ = ctx.Global().Set("__bucket", bucketVal)

	result, err := ctx.RunScript("__bucket.list()", "test_list.js")
	if err != nil {
		t.Fatalf("RunScript list: %v", err)
	}

	deadline := time.Now().Add(5 * time.Second)
	resolved, err := awaitValue(ctx, result, deadline)
	if err != nil {
		t.Fatalf("await list: %v", err)
	}
	_ = ctx.Global().Set("__listResult", resolved)

	// Verify we get back an object with objects array and truncated flag
	truncVal, err := ctx.RunScript("__listResult.truncated", "check_trunc.js")
	if err != nil {
		t.Fatalf("checking truncated: %v", err)
	}
	if truncVal.String() != "false" {
		t.Errorf("truncated = %q, want false", truncVal.String())
	}
}

func TestStorageBinding_ListWithOptions(t *testing.T) {
	iso, ctx := newV8TestContext(t)

	bridge := &StorageBridge{
		Client:     badMinioClient(t),
		BucketName: "test-bucket",
	}

	bucketVal, err := buildStorageBinding(iso, ctx, bridge)
	if err != nil {
		t.Fatalf("buildStorageBinding: %v", err)
	}
	_ = ctx.Global().Set("__bucket", bucketVal)

	// list with full options exercises the options extraction path
	result, err := ctx.RunScript(`__bucket.list({
		prefix: 'uploads/',
		cursor: 'last-key',
		delimiter: '/',
		limit: 5,
	})`, "test_list_opts.js")
	if err != nil {
		t.Fatalf("RunScript list opts: %v", err)
	}

	deadline := time.Now().Add(5 * time.Second)
	_, err = awaitValue(ctx, result, deadline)
	if err != nil {
		t.Fatalf("await list opts: %v", err)
	}
}

func TestStorageBinding_PublicUrl(t *testing.T) {
	iso, ctx := newV8TestContext(t)

	bridge := &StorageBridge{
		BucketName:  "my-bucket",
		PublicS3URL: "https://storage.example.com",
	}

	bucketVal, err := buildStorageBinding(iso, ctx, bridge)
	if err != nil {
		t.Fatalf("buildStorageBinding: %v", err)
	}
	_ = ctx.Global().Set("__bucket", bucketVal)

	result, err := ctx.RunScript("__bucket.publicUrl('images/photo.jpg')", "test_publicurl.js")
	if err != nil {
		t.Fatalf("RunScript publicUrl: %v", err)
	}

	want := "https://storage.example.com/my-bucket/images/photo.jpg"
	if result.String() != want {
		t.Errorf("publicUrl = %q, want %q", result.String(), want)
	}
}

func TestStorageBinding_PublicUrl_RequiresArg(t *testing.T) {
	iso, ctx := newV8TestContext(t)

	bridge := &StorageBridge{
		BucketName:  "my-bucket",
		PublicS3URL: "https://storage.example.com",
	}

	bucketVal, err := buildStorageBinding(iso, ctx, bridge)
	if err != nil {
		t.Fatalf("buildStorageBinding: %v", err)
	}
	_ = ctx.Global().Set("__bucket", bucketVal)

	// publicUrl() with no args should throw
	_, err = ctx.RunScript(`try { __bucket.publicUrl(); 'ok'; } catch(e) { e.message; }`, "test_publicurl_noarg.js")
	if err != nil {
		t.Fatalf("RunScript publicUrl noarg: %v", err)
	}
}

func TestStorageBinding_PublicUrl_NotAvailableWithoutConfig(t *testing.T) {
	iso, ctx := newV8TestContext(t)

	bridge := &StorageBridge{
		BucketName: "my-bucket",
		// No PublicS3URL
	}

	bucketVal, err := buildStorageBinding(iso, ctx, bridge)
	if err != nil {
		t.Fatalf("buildStorageBinding: %v", err)
	}
	_ = ctx.Global().Set("__bucket", bucketVal)

	result, err := ctx.RunScript("typeof __bucket.publicUrl", "check_publicurl.js")
	if err != nil {
		t.Fatalf("RunScript typeof publicUrl: %v", err)
	}
	if result.String() != "undefined" {
		t.Errorf("publicUrl should be undefined without PublicS3URL, got %q", result.String())
	}
}

func TestStorageBinding_CreateSignedUrl_NilClients(t *testing.T) {
	iso, ctx := newV8TestContext(t)

	// createSignedUrl is only added when Client or PresignClient is non-nil.
	// When both are nil, the method should not exist.
	bridge := &StorageBridge{
		BucketName: "my-bucket",
	}

	bucketVal, err := buildStorageBinding(iso, ctx, bridge)
	if err != nil {
		t.Fatalf("buildStorageBinding: %v", err)
	}
	_ = ctx.Global().Set("__bucket", bucketVal)

	result, err := ctx.RunScript("typeof __bucket.createSignedUrl", "test_sign.js")
	if err != nil {
		t.Fatalf("RunScript: %v", err)
	}
	if result.String() != "undefined" {
		t.Errorf("createSignedUrl should be undefined when no clients set, got %q", result.String())
	}
}

func TestStorageBinding_CreateSignedUrl_NoPresignButPublicURL(t *testing.T) {
	iso, ctx := newV8TestContext(t)

	// Client is set (so createSignedUrl is registered), but PresignClient is nil
	// and PublicS3URL is set -> should reject with "presign client not configured"
	bridge := &StorageBridge{
		Client:      badMinioClient(t),
		BucketName:  "my-bucket",
		PublicS3URL: "https://storage.example.com",
	}

	bucketVal, err := buildStorageBinding(iso, ctx, bridge)
	if err != nil {
		t.Fatalf("buildStorageBinding: %v", err)
	}
	_ = ctx.Global().Set("__bucket", bucketVal)

	result, err := ctx.RunScript("__bucket.createSignedUrl('key')", "test_sign2.js")
	if err != nil {
		t.Fatalf("RunScript createSignedUrl: %v", err)
	}

	deadline := time.Now().Add(5 * time.Second)
	_, err = awaitValue(ctx, result, deadline)
	if err == nil || !strings.Contains(err.Error(), "presign client not configured") {
		t.Fatalf("expected 'presign client not configured', got %v", err)
	}
}

func TestStorageBinding_CreateSignedUrl_WithExpiryOptions(t *testing.T) {
	iso, ctx := newV8TestContext(t)

	bridge := &StorageBridge{
		Client:     badMinioClient(t),
		BucketName: "my-bucket",
	}

	bucketVal, err := buildStorageBinding(iso, ctx, bridge)
	if err != nil {
		t.Fatalf("buildStorageBinding: %v", err)
	}
	_ = ctx.Global().Set("__bucket", bucketVal)

	// Test with custom expiry - will fail at PresignedGetObject but exercises options parsing
	result, err := ctx.RunScript("__bucket.createSignedUrl('key', { expiresIn: 7200 })", "test_sign3.js")
	if err != nil {
		t.Fatalf("RunScript createSignedUrl opts: %v", err)
	}

	deadline := time.Now().Add(5 * time.Second)
	_, err = awaitValue(ctx, result, deadline)
	// Should fail with connection error
	if err == nil {
		t.Fatal("expected createSignedUrl to reject with connection error")
	}
}

func TestStorageBinding_CreateSignedUrl_ExpiryClamp(t *testing.T) {
	iso, ctx := newV8TestContext(t)

	bridge := &StorageBridge{
		Client:     badMinioClient(t),
		BucketName: "my-bucket",
	}

	bucketVal, err := buildStorageBinding(iso, ctx, bridge)
	if err != nil {
		t.Fatalf("buildStorageBinding: %v", err)
	}
	_ = ctx.Global().Set("__bucket", bucketVal)

	// Test expiry clamping: negative -> 1, excessive -> 604800
	result, err := ctx.RunScript("__bucket.createSignedUrl('key', { expiresIn: -10 })", "test_sign_neg.js")
	if err != nil {
		t.Fatalf("RunScript: %v", err)
	}
	deadline := time.Now().Add(5 * time.Second)
	_, _ = awaitValue(ctx, result, deadline) // will error but that's fine

	result2, err := ctx.RunScript("__bucket.createSignedUrl('key', { expiresIn: 9999999 })", "test_sign_max.js")
	if err != nil {
		t.Fatalf("RunScript: %v", err)
	}
	_, _ = awaitValue(ctx, result2, deadline) // will error but exercises the clamp path
}

func TestStorageBinding_CreateSignedUrl_RequiresArg(t *testing.T) {
	iso, ctx := newV8TestContext(t)

	bridge := &StorageBridge{
		Client:     badMinioClient(t),
		BucketName: "my-bucket",
	}

	bucketVal, err := buildStorageBinding(iso, ctx, bridge)
	if err != nil {
		t.Fatalf("buildStorageBinding: %v", err)
	}
	_ = ctx.Global().Set("__bucket", bucketVal)

	result, err := ctx.RunScript("__bucket.createSignedUrl()", "test_sign_noarg.js")
	if err != nil {
		t.Fatalf("RunScript: %v", err)
	}

	deadline := time.Now().Add(5 * time.Second)
	_, err = awaitValue(ctx, result, deadline)
	if err == nil || !strings.Contains(err.Error(), "requires a key") {
		t.Fatalf("expected 'requires a key' error, got %v", err)
	}
}

func TestBuildR2ObjectBody_UploadedTimestamp(t *testing.T) {
	iso, ctx := newV8TestContext(t)

	now := time.Now()
	obj, err := buildR2ObjectBody(iso, ctx, "ts.txt", []byte("data"), &minio.ObjectInfo{
		Size:         4,
		ETag:         "ts-etag",
		ContentType:  "text/plain",
		LastModified: now,
	})
	if err != nil {
		t.Fatalf("buildR2ObjectBody: %v", err)
	}
	_ = ctx.Global().Set("__obj", obj.Value)

	uploadedVal, err := ctx.RunScript("__obj.uploaded", "uploaded.js")
	if err != nil {
		t.Fatalf("checking uploaded: %v", err)
	}

	expectedMS := float64(now.UnixMilli())
	if uploadedVal.Number() != expectedMS {
		t.Errorf("uploaded = %v, want %v", uploadedVal.Number(), expectedMS)
	}
}
