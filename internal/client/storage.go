package client

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"time"
)

type StorageBucket struct {
	ID         string    `json:"id"`
	SiteID     string    `json:"site_id"`
	Name       string    `json:"name"`
	BucketName string    `json:"bucket_name"`
	Public     bool      `json:"public"`
	CreatedAt  time.Time `json:"created_at"`
}

type StorageCredential struct {
	ID              string     `json:"id"`
	UserID          string     `json:"user_id"`
	AccessKeyID     string     `json:"access_key_id"`
	SecretAccessKey string     `json:"secret_access_key,omitempty"`
	Name            string     `json:"name"`
	LastUsedAt      *time.Time `json:"last_used_at,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
}

func (c *Client) ListBuckets(siteID string) ([]StorageBucket, error) {
	var buckets []StorageBucket
	err := c.get(fmt.Sprintf("/api/v1/sites/%s/storage/buckets", siteID), &buckets)
	return buckets, err
}

func (c *Client) CreateBucket(siteID, name, bucketName string, public bool) (*StorageBucket, error) {
	body := map[string]interface{}{
		"name":        name,
		"bucket_name": bucketName,
		"public":      public,
	}
	var bucket StorageBucket
	err := c.post(fmt.Sprintf("/api/v1/sites/%s/storage/buckets", siteID), body, &bucket)
	if err != nil {
		return nil, err
	}
	return &bucket, nil
}

func (c *Client) UpdateBucket(siteID, bucketID string, public bool) (*StorageBucket, error) {
	body := map[string]interface{}{
		"public": public,
	}
	var bucket StorageBucket
	err := c.patch(fmt.Sprintf("/api/v1/sites/%s/storage/buckets/%s", siteID, bucketID), body, &bucket)
	if err != nil {
		return nil, err
	}
	return &bucket, nil
}

func (c *Client) DeleteBucket(siteID, bucketID string) error {
	return c.delete(fmt.Sprintf("/api/v1/sites/%s/storage/buckets/%s", siteID, bucketID), nil)
}

func (c *Client) ListStorageCredentials() ([]StorageCredential, error) {
	var creds []StorageCredential
	err := c.get("/api/v1/s3-credentials", &creds)
	return creds, err
}

func (c *Client) CreateStorageCredential(name string) (*StorageCredential, error) {
	body := map[string]string{"name": name}
	var cred StorageCredential
	err := c.post("/api/v1/s3-credentials", body, &cred)
	if err != nil {
		return nil, err
	}
	return &cred, nil
}

func (c *Client) DeleteStorageCredential(id string) error {
	return c.delete("/api/v1/s3-credentials/"+id, nil)
}

// UploadURLResponse is the response from the upload-url endpoint.
type UploadURLResponse struct {
	UploadURL string `json:"upload_url"`
	Key       string `json:"key"`
	Bucket    string `json:"bucket"`
	ExpiresIn int    `json:"expires_in"`
}

// GetUploadURL returns a presigned PUT URL for uploading an object to a bucket.
func (c *Client) GetUploadURL(siteID, bucketID, key string) (*UploadURLResponse, error) {
	body := map[string]string{"key": key}
	var result UploadURLResponse
	err := c.post(fmt.Sprintf("/api/v1/sites/%s/storage/buckets/%s/upload-url", siteID, bucketID), body, &result)
	if err != nil {
		return nil, err
	}
	return &result, nil
}

// UploadToBucket uploads a file to a storage bucket via presigned URL.
func (c *Client) UploadToBucket(siteID, bucketID, key string, data []byte) error {
	urlResp, err := c.GetUploadURL(siteID, bucketID, key)
	if err != nil {
		return fmt.Errorf("getting upload URL: %w", err)
	}

	req, err := http.NewRequest(http.MethodPut, urlResp.UploadURL, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("creating upload request: %w", err)
	}
	req.ContentLength = int64(len(data))
	req.Header.Set("Content-Type", "application/octet-stream")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("uploading: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("upload failed with status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}
