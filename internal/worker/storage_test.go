package worker

import (
	"context"
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

func TestStorageBinding_ArrayBuffer_NotImplementedRejected(t *testing.T) {
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

	_, err = awaitValue(t, result)
	if err == nil || !strings.Contains(err.Error(), "not implemented") {
		t.Fatalf("expected not implemented rejection, got %v", err)
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
