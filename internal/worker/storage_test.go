package worker

import (
	"fmt"
	"strings"
	"testing"
	"time"

	v8 "github.com/tommie/v8go"
	minio "github.com/minio/minio-go/v7"
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

	ctx.Global().Set("__bucket", bucketVal)
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
	ctx.Global().Set("__obj", obj.Value)

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
	ctx.Global().Set("__result", resolved)
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
	ctx.Global().Set("__obj", obj.Value)

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
	ctx.Global().Set("__testBuf", resolved)
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
	ctx.Global().Set("__obj", obj.Value)

	result, err := ctx.RunScript("__obj.arrayBuffer()", "test_ab.js")
	if err != nil {
		t.Fatalf("RunScript arrayBuffer: %v", err)
	}

	deadline := time.Now().Add(5 * time.Second)
	resolved, err := awaitValue(ctx, result, deadline)
	if err != nil {
		t.Fatalf("await arrayBuffer: %v", err)
	}

	ctx.Global().Set("__result", resolved)
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
	ctx.Global().Set("__obj", obj.Value)

	result, err := ctx.RunScript("__obj.arrayBuffer()", "test_ab.js")
	if err != nil {
		t.Fatalf("RunScript arrayBuffer: %v", err)
	}

	deadline := time.Now().Add(5 * time.Second)
	resolved, err := awaitValue(ctx, result, deadline)
	if err != nil {
		t.Fatalf("await arrayBuffer: %v", err)
	}

	ctx.Global().Set("__testBuf", resolved)
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
	ctx.Global().Set("__obj", obj.Value)
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
	ctx.Global().Set("__obj", obj.Value)
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
	ctx.Global().Set("__obj", obj.Value)
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
	ctx.Global().Set("__obj", obj.Value)

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
	ctx.Global().Set("__obj", obj.Value)

	result, err := ctx.RunScript("__obj.json()", "test_json.js")
	if err != nil {
		t.Fatalf("RunScript json: %v", err)
	}

	deadline := time.Now().Add(5 * time.Second)
	resolved, err := awaitValue(ctx, result, deadline)
	if err != nil {
		t.Fatalf("await json: %v", err)
	}

	ctx.Global().Set("__result", resolved)
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

	ctx.Global().Set("__obj", obj.Value)

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
