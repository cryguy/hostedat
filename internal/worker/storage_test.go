package worker

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/fastschema/qjs"
	minio "github.com/minio/minio-go/v7"
)

func newStorageTestContext(t *testing.T) *qjs.Runtime {
	t.Helper()
	rt, err := qjs.New(qjs.Option{
		Context:          context.Background(),
		MemoryLimit:      128 * 1024 * 1024,
		MaxStackSize:     1024 * 1024,
		MaxExecutionTime: 5000,
		GCThreshold:      256 * 1024,
	})
	if err != nil {
		t.Fatalf("new runtime: %v", err)
	}
	return rt
}

// safeClose closes a QJS runtime, recovering from panics caused by
// known WASM cleanup bugs in the fastschema/qjs library.
func safeClose(rt *qjs.Runtime) {
	defer func() { recover() }()
	rt.Close()
}

// safeFree frees a QJS value, recovering from WASM panics that can
// occur when the runtime is in a corrupted state after promise rejection.
func safeFree(v *qjs.Value) {
	defer func() { recover() }()
	v.Free()
}

func awaitValue(t *testing.T, v *qjs.Value) (*qjs.Value, error) {
	t.Helper()
	if !v.IsPromise() {
		return v, nil
	}
	resolved, err := v.Await()
	return resolved, err
}

func TestStorageBinding_Put_UnsupportedBinaryRejected(t *testing.T) {
	rt := newStorageTestContext(t)
	defer safeClose(rt)

	ctx := rt.Context()
	bucket := buildStorageBinding(ctx, &StorageBridge{BucketName: "test"})
	defer safeFree(bucket)

	result, err := bucket.InvokeJS("put", ctx.NewString("k"), ctx.NewObject())
	if err != nil {
		t.Fatalf("InvokeJS put: %v", err)
	}
	defer safeFree(result)

	_, err = awaitValue(t, result)
	if err == nil || !strings.Contains(err.Error(), "supports string values only") {
		t.Fatalf("expected unsupported-value rejection, got %v", err)
	}
}

func TestStorageBinding_ArrayBuffer_ReturnsData(t *testing.T) {
	rt := newStorageTestContext(t)
	defer safeClose(rt)

	ctx := rt.Context()
	obj := buildR2ObjectBody(ctx, "k", []byte("hello"), &minio.ObjectInfo{
		Size:         5,
		ETag:         "etag",
		ContentType:  "text/plain",
		LastModified: time.Now(),
	})
	defer safeFree(obj)

	result, err := obj.InvokeJS("arrayBuffer")
	if err != nil {
		t.Fatalf("InvokeJS arrayBuffer: %v", err)
	}
	defer safeFree(result)

	resolved, err := awaitValue(t, result)
	if err != nil {
		t.Fatalf("await arrayBuffer: %v", err)
	}

	// Verify bodyUsed is set after consuming.
	after := obj.GetPropertyStr("bodyUsed")
	if after.String() != "true" {
		t.Fatalf("bodyUsed after arrayBuffer = %q, want true", after.String())
	}
	after.Free()

	// Verify the ArrayBuffer has the right byte length.
	bl := resolved.GetPropertyStr("byteLength")
	if bl.Int32() != 5 {
		t.Fatalf("byteLength = %d, want 5", bl.Int32())
	}
	bl.Free()

	// Verify body cannot be consumed again.
	result2, err := obj.InvokeJS("arrayBuffer")
	if err != nil {
		t.Fatalf("InvokeJS second arrayBuffer: %v", err)
	}
	defer safeFree(result2)

	_, err = awaitValue(t, result2)
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

	rt := newStorageTestContext(t)
	defer safeClose(rt)

	ctx := rt.Context()
	obj := buildR2ObjectBody(ctx, "image.png", pngHeader, &minio.ObjectInfo{
		Size:         int64(len(pngHeader)),
		ETag:         "png-etag",
		ContentType:  "image/png",
		LastModified: time.Now(),
	})
	defer safeFree(obj)

	result, err := obj.InvokeJS("arrayBuffer")
	if err != nil {
		t.Fatalf("InvokeJS arrayBuffer: %v", err)
	}
	defer safeFree(result)

	resolved, err := awaitValue(t, result)
	if err != nil {
		t.Fatalf("await arrayBuffer: %v", err)
	}

	// Verify byte length matches the original blob.
	bl := resolved.GetPropertyStr("byteLength")
	if bl.Int32() != int32(len(pngHeader)) {
		t.Fatalf("byteLength = %d, want %d", bl.Int32(), len(pngHeader))
	}
	bl.Free()

	// Store the ArrayBuffer as a global so we can inspect bytes from JS.
	g := ctx.Global()
	g.SetPropertyStr("__testBuf", resolved)
	g.Free()

	// Read back every byte via Uint8Array and compare to the Go source.
	for i, expected := range pngHeader {
		jsCode := fmt.Sprintf("new Uint8Array(__testBuf)[%d]", i)
		v, err := ctx.Eval("test.js", qjs.Code(jsCode))
		if err != nil {
			t.Fatalf("eval byte[%d]: %v", i, err)
		}
		got := byte(v.Int32())
		v.Free()
		if got != expected {
			t.Fatalf("byte[%d] = 0x%02X, want 0x%02X", i, got, expected)
		}
	}
}

func TestStorageBinding_ArrayBuffer_EmptyBlob(t *testing.T) {
	rt := newStorageTestContext(t)
	defer safeClose(rt)

	ctx := rt.Context()
	obj := buildR2ObjectBody(ctx, "empty.bin", []byte{}, &minio.ObjectInfo{
		Size:         0,
		ETag:         "empty-etag",
		ContentType:  "application/octet-stream",
		LastModified: time.Now(),
	})
	defer safeFree(obj)

	result, err := obj.InvokeJS("arrayBuffer")
	if err != nil {
		t.Fatalf("InvokeJS arrayBuffer: %v", err)
	}
	defer safeFree(result)

	resolved, err := awaitValue(t, result)
	if err != nil {
		t.Fatalf("await arrayBuffer: %v", err)
	}

	bl := resolved.GetPropertyStr("byteLength")
	if bl.Int32() != 0 {
		t.Fatalf("byteLength = %d, want 0", bl.Int32())
	}
	bl.Free()
}

func TestStorageBinding_ArrayBuffer_AllByteValues(t *testing.T) {
	// Create a 256-byte blob containing every possible byte value 0x00..0xFF.
	// This catches any truncation or encoding issues across the full byte range.
	blob := make([]byte, 256)
	for i := range blob {
		blob[i] = byte(i)
	}

	rt := newStorageTestContext(t)
	defer safeClose(rt)

	ctx := rt.Context()
	obj := buildR2ObjectBody(ctx, "allbytes.bin", blob, &minio.ObjectInfo{
		Size:         256,
		ETag:         "all-etag",
		ContentType:  "application/octet-stream",
		LastModified: time.Now(),
	})
	defer safeFree(obj)

	result, err := obj.InvokeJS("arrayBuffer")
	if err != nil {
		t.Fatalf("InvokeJS arrayBuffer: %v", err)
	}
	defer safeFree(result)

	resolved, err := awaitValue(t, result)
	if err != nil {
		t.Fatalf("await arrayBuffer: %v", err)
	}

	bl := resolved.GetPropertyStr("byteLength")
	if bl.Int32() != 256 {
		t.Fatalf("byteLength = %d, want 256", bl.Int32())
	}
	bl.Free()

	// Spot-check key byte values: null, high bytes, and boundaries.
	g := ctx.Global()
	g.SetPropertyStr("__testBuf", resolved)
	g.Free()

	checks := []int{0x00, 0x01, 0x7F, 0x80, 0xFE, 0xFF}
	for _, idx := range checks {
		jsCode := fmt.Sprintf("new Uint8Array(__testBuf)[%d]", idx)
		v, err := ctx.Eval("test.js", qjs.Code(jsCode))
		if err != nil {
			t.Fatalf("eval byte[%d]: %v", idx, err)
		}
		got := byte(v.Int32())
		v.Free()
		if got != byte(idx) {
			t.Fatalf("byte[%d] = 0x%02X, want 0x%02X", idx, got, byte(idx))
		}
	}
}

func TestStorageBinding_ArrayBuffer_ThenTextRejects(t *testing.T) {
	rt := newStorageTestContext(t)
	defer safeClose(rt)

	ctx := rt.Context()
	obj := buildR2ObjectBody(ctx, "img.png", []byte{0x89, 0x50, 0x4E, 0x47}, &minio.ObjectInfo{
		Size:         4,
		ETag:         "etag",
		ContentType:  "image/png",
		LastModified: time.Now(),
	})
	defer safeFree(obj)

	// Consume via arrayBuffer().
	result, err := obj.InvokeJS("arrayBuffer")
	if err != nil {
		t.Fatalf("InvokeJS arrayBuffer: %v", err)
	}
	defer safeFree(result)
	if _, err := awaitValue(t, result); err != nil {
		t.Fatalf("await arrayBuffer: %v", err)
	}

	// text() must reject after arrayBuffer() consumed the body.
	r2, err := obj.InvokeJS("text")
	if err != nil {
		t.Fatalf("InvokeJS text: %v", err)
	}
	defer safeFree(r2)
	_, err = awaitValue(t, r2)
	if err == nil || !strings.Contains(err.Error(), "already consumed") {
		t.Fatalf("expected body consumed rejection from text(), got %v", err)
	}

	// json() must also reject.
	r3, err := obj.InvokeJS("json")
	if err != nil {
		t.Fatalf("InvokeJS json: %v", err)
	}
	defer safeFree(r3)
	_, err = awaitValue(t, r3)
	if err == nil || !strings.Contains(err.Error(), "already consumed") {
		t.Fatalf("expected body consumed rejection from json(), got %v", err)
	}
}

func TestStorageBinding_Text_ThenArrayBufferRejects(t *testing.T) {
	rt := newStorageTestContext(t)
	defer safeClose(rt)

	ctx := rt.Context()
	obj := buildR2ObjectBody(ctx, "doc.txt", []byte("hello world"), &minio.ObjectInfo{
		Size:         11,
		ETag:         "etag",
		ContentType:  "text/plain",
		LastModified: time.Now(),
	})
	defer safeFree(obj)

	// Consume via text().
	result, err := obj.InvokeJS("text")
	if err != nil {
		t.Fatalf("InvokeJS text: %v", err)
	}
	defer safeFree(result)
	if _, err := awaitValue(t, result); err != nil {
		t.Fatalf("await text: %v", err)
	}

	// arrayBuffer() must reject after text() consumed the body.
	r2, err := obj.InvokeJS("arrayBuffer")
	if err != nil {
		t.Fatalf("InvokeJS arrayBuffer: %v", err)
	}
	defer safeFree(r2)
	_, err = awaitValue(t, r2)
	if err == nil || !strings.Contains(err.Error(), "already consumed") {
		t.Fatalf("expected body consumed rejection from arrayBuffer(), got %v", err)
	}
}

func TestStorageBinding_BodyUsed_TransitionsAfterRead(t *testing.T) {
	rt := newStorageTestContext(t)
	defer safeClose(rt)

	ctx := rt.Context()
	obj := buildR2ObjectBody(ctx, "k", []byte("hello"), &minio.ObjectInfo{
		Size:         5,
		ETag:         "etag",
		ContentType:  "text/plain",
		LastModified: time.Now(),
	})
	defer safeFree(obj)

	initial := obj.GetPropertyStr("bodyUsed")
	if initial.String() != "false" {
		t.Fatalf("initial bodyUsed = %q, want false", initial.String())
	}
	initial.Free()

	result, err := obj.InvokeJS("text")
	if err != nil {
		t.Fatalf("InvokeJS text: %v", err)
	}
	defer safeFree(result)

	resolved, err := awaitValue(t, result)
	if err != nil {
		t.Fatalf("await text: %v", err)
	}
	if resolved.String() != "hello" {
		t.Fatalf("text result = %q, want hello", resolved.String())
	}

	after := obj.GetPropertyStr("bodyUsed")
	if after.String() != "true" {
		t.Fatalf("bodyUsed after text = %q, want true", after.String())
	}
	after.Free()

	result2, err := obj.InvokeJS("text")
	if err != nil {
		t.Fatalf("InvokeJS second text: %v", err)
	}
	defer safeFree(result2)

	_, err = awaitValue(t, result2)
	if err == nil || !strings.Contains(err.Error(), "already consumed") {
		t.Fatalf("expected body consumed rejection, got %v", err)
	}
}

func TestStorageBinding_JSON_InvalidJSONRejects(t *testing.T) {
	rt := newStorageTestContext(t)
	defer safeClose(rt)

	ctx := rt.Context()
	obj := buildR2ObjectBody(ctx, "k", []byte("{not-json"), &minio.ObjectInfo{
		Size:         9,
		ETag:         "etag",
		ContentType:  "application/json",
		LastModified: time.Now(),
	})
	defer safeFree(obj)

	result, err := obj.InvokeJS("json")
	if err != nil {
		t.Fatalf("InvokeJS json: %v", err)
	}
	defer safeFree(result)

	_, err = awaitValue(t, result)
	if err == nil || !strings.Contains(err.Error(), "invalid JSON") {
		t.Fatalf("expected invalid JSON rejection, got %v", err)
	}
}

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
