package client

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestClient_ListBuckets_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if r.URL.Path != "/api/v1/sites/site123/storage/buckets" {
			t.Errorf("expected /api/v1/sites/site123/storage/buckets, got %s", r.URL.Path)
		}
		w.WriteHeader(200)
		_ = json.NewEncoder(w).Encode([]StorageBucket{
			{ID: "bucket1", SiteID: "site123", Name: "Images", BucketName: "site123-images", Public: true},
			{ID: "bucket2", SiteID: "site123", Name: "Private", BucketName: "site123-private", Public: false},
		})
	}))
	defer srv.Close()

	c := New(srv.URL, "test-token")
	buckets, err := c.ListBuckets("site123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(buckets) != 2 {
		t.Fatalf("expected 2 buckets, got %d", len(buckets))
	}
	if buckets[0].Name != "Images" || !buckets[0].Public {
		t.Errorf("unexpected first bucket: %+v", buckets[0])
	}
	if buckets[1].Public {
		t.Error("expected second bucket to be private")
	}
}

func TestClient_ListBuckets_Empty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		_ = json.NewEncoder(w).Encode([]StorageBucket{})
	}))
	defer srv.Close()

	c := New(srv.URL, "test-token")
	buckets, err := c.ListBuckets("site123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(buckets) != 0 {
		t.Errorf("expected 0 buckets, got %d", len(buckets))
	}
}

func TestClient_CreateBucket_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/api/v1/sites/site123/storage/buckets" {
			t.Errorf("expected /api/v1/sites/site123/storage/buckets, got %s", r.URL.Path)
		}
		var req map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("failed to decode request: %v", err)
		}
		if req["name"] != "My Bucket" {
			t.Errorf("expected name 'My Bucket', got %v", req["name"])
		}
		if req["bucket_name"] != "my-bucket" {
			t.Errorf("expected bucket_name 'my-bucket', got %v", req["bucket_name"])
		}
		if req["public"] != true {
			t.Errorf("expected public true, got %v", req["public"])
		}
		w.WriteHeader(201)
		_ = json.NewEncoder(w).Encode(StorageBucket{
			ID:         "bucket123",
			SiteID:     "site123",
			Name:       "My Bucket",
			BucketName: "my-bucket",
			Public:     true,
		})
	}))
	defer srv.Close()

	c := New(srv.URL, "test-token")
	bucket, err := c.CreateBucket("site123", "My Bucket", "my-bucket", true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if bucket.ID != "bucket123" {
		t.Errorf("expected ID 'bucket123', got %s", bucket.ID)
	}
	if !bucket.Public {
		t.Error("expected Public true")
	}
}

func TestClient_CreateBucket_Private(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("failed to decode request: %v", err)
		}
		if req["public"] != false {
			t.Errorf("expected public false, got %v", req["public"])
		}
		w.WriteHeader(201)
		_ = json.NewEncoder(w).Encode(StorageBucket{
			ID:     "bucket123",
			Public: false,
		})
	}))
	defer srv.Close()

	c := New(srv.URL, "test-token")
	bucket, err := c.CreateBucket("site123", "Private Bucket", "private", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if bucket.Public {
		t.Error("expected Public false")
	}
}

func TestClient_CreateBucket_Conflict(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(409)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "bucket name already exists"})
	}))
	defer srv.Close()

	c := New(srv.URL, "test-token")
	_, err := c.CreateBucket("site123", "Duplicate", "duplicate", true)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected *APIError, got %T", err)
	}
	if apiErr.StatusCode != 409 {
		t.Errorf("expected StatusCode 409, got %d", apiErr.StatusCode)
	}
}

func TestClient_UpdateBucket_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "PATCH" {
			t.Errorf("expected PATCH, got %s", r.Method)
		}
		if r.URL.Path != "/api/v1/sites/site123/storage/buckets/bucket123" {
			t.Errorf("expected /api/v1/sites/site123/storage/buckets/bucket123, got %s", r.URL.Path)
		}
		var req map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("failed to decode request: %v", err)
		}
		if req["public"] != true {
			t.Errorf("expected public true, got %v", req["public"])
		}
		w.WriteHeader(200)
		_ = json.NewEncoder(w).Encode(StorageBucket{
			ID:     "bucket123",
			Public: true,
		})
	}))
	defer srv.Close()

	c := New(srv.URL, "test-token")
	bucket, err := c.UpdateBucket("site123", "bucket123", true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !bucket.Public {
		t.Error("expected Public true")
	}
}

func TestClient_UpdateBucket_MakePrivate(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("failed to decode request: %v", err)
		}
		if req["public"] != false {
			t.Errorf("expected public false, got %v", req["public"])
		}
		w.WriteHeader(200)
		_ = json.NewEncoder(w).Encode(StorageBucket{
			ID:     "bucket123",
			Public: false,
		})
	}))
	defer srv.Close()

	c := New(srv.URL, "test-token")
	bucket, err := c.UpdateBucket("site123", "bucket123", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if bucket.Public {
		t.Error("expected Public false")
	}
}

func TestClient_DeleteBucket_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "DELETE" {
			t.Errorf("expected DELETE, got %s", r.Method)
		}
		if r.URL.Path != "/api/v1/sites/site123/storage/buckets/bucket123" {
			t.Errorf("expected /api/v1/sites/site123/storage/buckets/bucket123, got %s", r.URL.Path)
		}
		w.WriteHeader(204)
	}))
	defer srv.Close()

	c := New(srv.URL, "test-token")
	err := c.DeleteBucket("site123", "bucket123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestClient_DeleteBucket_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "bucket not found"})
	}))
	defer srv.Close()

	c := New(srv.URL, "test-token")
	err := c.DeleteBucket("site123", "nonexistent")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected *APIError, got %T", err)
	}
	if apiErr.StatusCode != 404 {
		t.Errorf("expected StatusCode 404, got %d", apiErr.StatusCode)
	}
}

func TestClient_ListStorageCredentials_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if r.URL.Path != "/api/v1/s3-credentials" {
			t.Errorf("expected /api/v1/s3-credentials, got %s", r.URL.Path)
		}
		lastUsed := time.Now()
		w.WriteHeader(200)
		_ = json.NewEncoder(w).Encode([]StorageCredential{
			{ID: "cred1", Name: "My Creds", AccessKeyID: "AKI123", LastUsedAt: &lastUsed},
			{ID: "cred2", Name: "Other Creds", AccessKeyID: "AKI456", LastUsedAt: nil},
		})
	}))
	defer srv.Close()

	c := New(srv.URL, "test-token")
	creds, err := c.ListStorageCredentials()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(creds) != 2 {
		t.Fatalf("expected 2 credentials, got %d", len(creds))
	}
	if creds[0].AccessKeyID != "AKI123" {
		t.Errorf("expected AccessKeyID 'AKI123', got %s", creds[0].AccessKeyID)
	}
	if creds[0].LastUsedAt == nil {
		t.Error("expected LastUsedAt to be set")
	}
	if creds[1].LastUsedAt != nil {
		t.Error("expected LastUsedAt to be nil")
	}
}

func TestClient_CreateStorageCredential_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/api/v1/s3-credentials" {
			t.Errorf("expected /api/v1/s3-credentials, got %s", r.URL.Path)
		}
		var req map[string]string
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("failed to decode request: %v", err)
		}
		if req["name"] != "Production Creds" {
			t.Errorf("expected name 'Production Creds', got %s", req["name"])
		}
		w.WriteHeader(201)
		_ = json.NewEncoder(w).Encode(StorageCredential{
			ID:              "cred123",
			Name:            "Production Creds",
			AccessKeyID:     "AKI789",
			SecretAccessKey: "SECRET123",
		})
	}))
	defer srv.Close()

	c := New(srv.URL, "test-token")
	cred, err := c.CreateStorageCredential("Production Creds")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cred.ID != "cred123" {
		t.Errorf("expected ID 'cred123', got %s", cred.ID)
	}
	if cred.SecretAccessKey != "SECRET123" {
		t.Errorf("expected SecretAccessKey to be returned on creation, got %s", cred.SecretAccessKey)
	}
}

func TestClient_DeleteStorageCredential_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "DELETE" {
			t.Errorf("expected DELETE, got %s", r.Method)
		}
		if r.URL.Path != "/api/v1/s3-credentials/cred123" {
			t.Errorf("expected /api/v1/s3-credentials/cred123, got %s", r.URL.Path)
		}
		w.WriteHeader(204)
	}))
	defer srv.Close()

	c := New(srv.URL, "test-token")
	err := c.DeleteStorageCredential("cred123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestClient_GetUploadURL_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/api/v1/sites/site123/storage/buckets/bucket123/upload-url" {
			t.Errorf("expected /api/v1/sites/site123/storage/buckets/bucket123/upload-url, got %s", r.URL.Path)
		}
		var req map[string]string
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("failed to decode request: %v", err)
		}
		if req["key"] != "images/photo.jpg" {
			t.Errorf("expected key 'images/photo.jpg', got %s", req["key"])
		}
		w.WriteHeader(200)
		_ = json.NewEncoder(w).Encode(UploadURLResponse{
			UploadURL: "https://s3.amazonaws.com/presigned-url",
			Key:       "images/photo.jpg",
			Bucket:    "my-bucket",
			ExpiresIn: 3600,
		})
	}))
	defer srv.Close()

	c := New(srv.URL, "test-token")
	resp, err := c.GetUploadURL("site123", "bucket123", "images/photo.jpg")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.UploadURL != "https://s3.amazonaws.com/presigned-url" {
		t.Errorf("expected presigned URL, got %s", resp.UploadURL)
	}
	if resp.Key != "images/photo.jpg" {
		t.Errorf("expected Key 'images/photo.jpg', got %s", resp.Key)
	}
	if resp.ExpiresIn != 3600 {
		t.Errorf("expected ExpiresIn 3600, got %d", resp.ExpiresIn)
	}
}

func TestClient_UploadToBucket_Success(t *testing.T) {
	// First server handles the GetUploadURL request
	apiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/upload-url") {
			// Return a presigned URL pointing to our S3 mock server
			s3URL := "http://localhost:9999/upload"
			_ = json.NewEncoder(w).Encode(UploadURLResponse{
				UploadURL: s3URL,
				Key:       "test.txt",
				Bucket:    "test-bucket",
			})
		}
	}))
	defer apiSrv.Close()

	// Second server mocks the S3 presigned URL upload
	uploadCalled := false
	var uploadedData []byte
	s3Srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "PUT" {
			t.Errorf("expected PUT to S3, got %s", r.Method)
		}
		uploadCalled = true
		var err error
		uploadedData, err = io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("failed to read upload body: %v", err)
		}
		w.WriteHeader(200)
	}))
	defer s3Srv.Close()

	// Override the upload URL to point to our mock S3 server
	apiSrvWithS3 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/upload-url") {
			_ = json.NewEncoder(w).Encode(UploadURLResponse{
				UploadURL: s3Srv.URL + "/upload",
				Key:       "test.txt",
				Bucket:    "test-bucket",
			})
		}
	}))
	defer apiSrvWithS3.Close()

	c := New(apiSrvWithS3.URL, "test-token")
	data := []byte("Hello, World!")
	err := c.UploadToBucket("site123", "bucket123", "test.txt", data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !uploadCalled {
		t.Error("expected upload to S3 to be called")
	}
	if !bytes.Equal(uploadedData, data) {
		t.Errorf("expected uploaded data %q, got %q", string(data), string(uploadedData))
	}
}

func TestClient_UploadToBucket_GetUploadURLFails(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "bucket not found"})
	}))
	defer srv.Close()

	c := New(srv.URL, "test-token")
	err := c.UploadToBucket("site123", "nonexistent", "test.txt", []byte("data"))
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "getting upload URL") {
		t.Errorf("expected 'getting upload URL' error, got %v", err)
	}
}

func TestClient_UploadToBucket_S3Fails(t *testing.T) {
	// API server returns presigned URL
	apiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(UploadURLResponse{
			UploadURL: "http://localhost:9999/nonexistent",
			Key:       "test.txt",
		})
	}))
	defer apiSrv.Close()

	// S3 mock returns error
	s3Srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(403)
		_, _ = w.Write([]byte("Access Denied"))
	}))
	defer s3Srv.Close()

	// Override with working S3 URL
	apiSrvWithS3 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(UploadURLResponse{
			UploadURL: s3Srv.URL + "/upload",
			Key:       "test.txt",
		})
	}))
	defer apiSrvWithS3.Close()

	c := New(apiSrvWithS3.URL, "test-token")
	err := c.UploadToBucket("site123", "bucket123", "test.txt", []byte("data"))
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "upload failed with status 403") {
		t.Errorf("expected S3 upload failure error, got %v", err)
	}
}

func TestClient_UploadToBucket_EmptyData(t *testing.T) {
	apiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(UploadURLResponse{
			UploadURL: "http://localhost:9999/upload",
			Key:       "empty.txt",
		})
	}))
	defer apiSrv.Close()

	var uploadedSize int64
	s3Srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		uploadedSize = r.ContentLength
		w.WriteHeader(200)
	}))
	defer s3Srv.Close()

	apiSrvWithS3 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(UploadURLResponse{
			UploadURL: s3Srv.URL + "/upload",
			Key:       "empty.txt",
		})
	}))
	defer apiSrvWithS3.Close()

	c := New(apiSrvWithS3.URL, "test-token")
	err := c.UploadToBucket("site123", "bucket123", "empty.txt", []byte{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if uploadedSize != 0 {
		t.Errorf("expected ContentLength 0, got %d", uploadedSize)
	}
}
