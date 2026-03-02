package workeradapter

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/url"
	"strings"
	"time"

	"github.com/cryguy/worker/v2"
	"github.com/minio/minio-go/v7"
)

// Compile-time interface check.
var _ worker.R2Store = (*MinioR2Store)(nil)

const maxObjectSize = 128 * 1024 * 1024 // 128 MB

// MinioR2Store implements worker.R2Store using minio-go.
type MinioR2Store struct {
	Client      *minio.Client
	BucketName  string
	PublicS3URL string // public-facing S3 URL for direct object URLs
}

// Get retrieves an object's data and metadata.
func (s *MinioR2Store) Get(key string) ([]byte, *worker.R2Object, error) {
	obj, err := s.Client.GetObject(context.Background(), s.BucketName, key, minio.GetObjectOptions{})
	if err != nil {
		return nil, nil, err
	}
	defer func() { _ = obj.Close() }()

	stat, err := obj.Stat()
	if err != nil {
		return nil, nil, err
	}

	if stat.Size > int64(maxObjectSize) {
		return nil, nil, fmt.Errorf("object too large: %d bytes (max %d)", stat.Size, maxObjectSize)
	}

	data, err := io.ReadAll(io.LimitReader(obj, int64(maxObjectSize)+1))
	if err != nil {
		return nil, nil, fmt.Errorf("reading object: %w", err)
	}

	r2obj := &worker.R2Object{
		Key:            key,
		Size:           stat.Size,
		ContentType:    stat.ContentType,
		ETag:           stat.ETag,
		LastModified:   stat.LastModified,
		CustomMetadata: stat.UserMetadata,
	}
	return data, r2obj, nil
}

// Put stores an object and returns its metadata.
func (s *MinioR2Store) Put(key string, data []byte, opts worker.R2PutOptions) (*worker.R2Object, error) {
	putOpts := minio.PutObjectOptions{}
	if opts.ContentType != "" {
		putOpts.ContentType = opts.ContentType
	}
	if len(opts.CustomMetadata) > 0 {
		putOpts.UserMetadata = opts.CustomMetadata
	}

	reader := bytes.NewReader(data)
	info, err := s.Client.PutObject(context.Background(), s.BucketName, key, reader, int64(len(data)), putOpts)
	if err != nil {
		return nil, fmt.Errorf("putting object: %w", err)
	}

	return &worker.R2Object{
		Key:            key,
		Size:           info.Size,
		ContentType:    opts.ContentType,
		ETag:           info.ETag,
		LastModified:   time.Now(),
		CustomMetadata: opts.CustomMetadata,
	}, nil
}

// Delete removes one or more objects by key.
func (s *MinioR2Store) Delete(keys []string) error {
	for _, k := range keys {
		if err := s.Client.RemoveObject(context.Background(), s.BucketName, k, minio.RemoveObjectOptions{}); err != nil {
			return err
		}
	}
	return nil
}

// Head retrieves object metadata without downloading the body.
func (s *MinioR2Store) Head(key string) (*worker.R2Object, error) {
	stat, err := s.Client.StatObject(context.Background(), s.BucketName, key, minio.StatObjectOptions{})
	if err != nil {
		return nil, err
	}

	return &worker.R2Object{
		Key:            key,
		Size:           stat.Size,
		ContentType:    stat.ContentType,
		ETag:           stat.ETag,
		LastModified:   stat.LastModified,
		CustomMetadata: stat.UserMetadata,
	}, nil
}

// List lists objects in the bucket with optional filtering.
func (s *MinioR2Store) List(opts worker.R2ListOptions) (*worker.R2ListResult, error) {
	limit := opts.Limit
	if limit <= 0 {
		limit = 1000
	}

	listOpts := minio.ListObjectsOptions{
		Prefix:    opts.Prefix,
		MaxKeys:   limit,
		Recursive: opts.Delimiter == "",
	}
	if opts.Cursor != "" {
		listOpts.StartAfter = opts.Cursor
	}

	var objects []worker.R2Object
	var delimitedPrefixes []string
	count := 0

	for obj := range s.Client.ListObjects(context.Background(), s.BucketName, listOpts) {
		if obj.Err != nil {
			return nil, obj.Err
		}
		if strings.HasSuffix(obj.Key, "/") && opts.Delimiter != "" {
			delimitedPrefixes = append(delimitedPrefixes, obj.Key)
			continue
		}
		objects = append(objects, worker.R2Object{
			Key:          obj.Key,
			Size:         obj.Size,
			ETag:         obj.ETag,
			LastModified: obj.LastModified,
		})
		count++
		if count >= limit {
			break
		}
	}

	truncated := count >= limit
	var nextCursor string
	if truncated && len(objects) > 0 {
		nextCursor = objects[len(objects)-1].Key
	}

	return &worker.R2ListResult{
		Objects:           objects,
		Truncated:         truncated,
		Cursor:            nextCursor,
		DelimitedPrefixes: delimitedPrefixes,
	}, nil
}

// PresignedGetURL generates a pre-signed URL for downloading an object.
func (s *MinioR2Store) PresignedGetURL(key string, expiry time.Duration) (string, error) {
	if s.Client == nil {
		return "", fmt.Errorf("storage client not configured")
	}

	presigned, err := s.Client.PresignedGetObject(
		context.Background(),
		s.BucketName,
		key,
		expiry,
		nil,
	)
	if err != nil {
		return "", fmt.Errorf("creating signed URL: %w", err)
	}

	return presigned.String(), nil
}

// PublicURL returns the public URL for an object.
func (s *MinioR2Store) PublicURL(key string) (string, error) {
	if s.PublicS3URL == "" {
		return "", fmt.Errorf("public S3 URL not configured")
	}
	return buildPublicObjectURL(s.PublicS3URL, s.BucketName, key)
}

// buildPublicObjectURL returns an object URL using the configured public S3 base.
func buildPublicObjectURL(publicBase, bucket, key string) (string, error) {
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
