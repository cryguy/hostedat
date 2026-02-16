package s3

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	gonanoid "github.com/matoous/go-nanoid/v2"

	"github.com/cryguy/hostedat/internal/models"
	"gorm.io/gorm"
)

const (
	maxObjectSize  = 500 << 20 // 500 MB
	maxKeyLength   = 1024
	defaultMaxKeys = 1000
)

// Handler handles S3-compatible API requests.
type Handler struct {
	DB          *gorm.DB
	StoragePath string // base path for object storage files
}

// NewHandler creates a new S3 handler.
func NewHandler(db *gorm.DB, storagePath string) *Handler {
	return &Handler{
		DB:          db,
		StoragePath: storagePath,
	}
}

// ServeHTTP is the main entry point. It routes based on method and path.
// Expected path format: /:siteId/:key...
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Parse the path: /:siteId/:key
	path := strings.TrimPrefix(r.URL.Path, "/")
	parts := strings.SplitN(path, "/", 2)
	if len(parts) == 0 || parts[0] == "" {
		WriteErrorResponse(w, http.StatusBadRequest, "InvalidBucketName", "Missing bucket name", r.URL.Path)
		return
	}

	siteID := parts[0]
	key := ""
	if len(parts) > 1 {
		key = parts[1]
	}

	// Authenticate via SigV4.
	cred, err := h.authenticate(r, siteID)
	if err != nil {
		if strings.Contains(err.Error(), "invalid access key") {
			WriteErrorResponse(w, http.StatusForbidden, "InvalidAccessKeyId", "The AWS Access Key Id you provided does not exist in our records.", r.URL.Path)
		} else if strings.Contains(err.Error(), "signature does not match") {
			WriteErrorResponse(w, http.StatusForbidden, "SignatureDoesNotMatch", "The request signature we calculated does not match the signature you provided.", r.URL.Path)
		} else if strings.Contains(err.Error(), "timestamp expired") {
			WriteErrorResponse(w, http.StatusForbidden, "RequestTimeTooSkewed", "The difference between the request time and the current time is too large.", r.URL.Path)
		} else {
			WriteErrorResponse(w, http.StatusForbidden, "AccessDenied", err.Error(), r.URL.Path)
		}
		return
	}

	// Verify the credential belongs to this site.
	if cred.SiteID != siteID {
		WriteErrorResponse(w, http.StatusForbidden, "AccessDenied", "Access denied", r.URL.Path)
		return
	}

	// Route to the appropriate handler.
	switch r.Method {
	case http.MethodGet:
		if key == "" {
			h.listObjectsV2(w, r, siteID)
		} else {
			h.getObject(w, r, siteID, key)
		}
	case http.MethodHead:
		if key == "" {
			// HeadBucket - just return 200
			w.WriteHeader(http.StatusOK)
		} else {
			h.headObject(w, r, siteID, key)
		}
	case http.MethodPut:
		if key == "" {
			WriteErrorResponse(w, http.StatusMethodNotAllowed, "MethodNotAllowed", "PUT without key is not supported", r.URL.Path)
			return
		}
		// Check for CopyObject (x-amz-copy-source header).
		if r.Header.Get("X-Amz-Copy-Source") != "" {
			h.copyObject(w, r, siteID, key)
		} else {
			// Check for UploadPart (uploadId query param).
			if r.URL.Query().Get("uploadId") != "" {
				h.uploadPart(w, r, siteID, key)
			} else {
				h.putObject(w, r, siteID, key)
			}
		}
	case http.MethodDelete:
		if key == "" {
			WriteErrorResponse(w, http.StatusMethodNotAllowed, "MethodNotAllowed", "DELETE without key is not supported", r.URL.Path)
			return
		}
		h.deleteObject(w, r, siteID, key)
	case http.MethodPost:
		if key == "" && r.URL.Query().Has("delete") {
			h.deleteObjects(w, r, siteID)
		} else if key != "" && r.URL.Query().Has("uploads") {
			h.createMultipartUpload(w, r, siteID, key)
		} else if key != "" && r.URL.Query().Get("uploadId") != "" {
			h.completeMultipartUpload(w, r, siteID, key)
		} else {
			WriteErrorResponse(w, http.StatusMethodNotAllowed, "MethodNotAllowed", "Unsupported operation", r.URL.Path)
		}
	default:
		WriteErrorResponse(w, http.StatusMethodNotAllowed, "MethodNotAllowed", "Method not allowed", r.URL.Path)
	}
}

// authenticate validates the SigV4 signature and returns the matching credential.
func (h *Handler) authenticate(r *http.Request, siteID string) (*models.StorageCredential, error) {
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		return nil, fmt.Errorf("missing authorization header")
	}

	auth, err := ParseAuthorization(authHeader)
	if err != nil {
		return nil, fmt.Errorf("invalid authorization: %w", err)
	}

	// Look up the credential.
	var cred models.StorageCredential
	if err := h.DB.Where("access_key_id = ?", auth.AccessKeyID).First(&cred).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("invalid access key")
		}
		return nil, fmt.Errorf("database error: %w", err)
	}

	// Read body for signature verification (we need to buffer it).
	body, err := io.ReadAll(io.LimitReader(r.Body, maxObjectSize+1))
	if err != nil {
		return nil, fmt.Errorf("reading request body: %w", err)
	}

	// Verify the signature using the stored secret key.
	// We store the raw secret key (not hashed) because SigV4 requires it
	// for HMAC computation.
	if err := VerifySignature(r, auth, cred.SecretAccessKey, body); err != nil {
		return nil, fmt.Errorf("signature does not match: %w", err)
	}

	// Put the body back for downstream handlers.
	r.Body = io.NopCloser(strings.NewReader(string(body)))

	return &cred, nil
}

// putObject handles PUT /:bucket/:key
func (h *Handler) putObject(w http.ResponseWriter, r *http.Request, siteID, key string) {
	if len(key) > maxKeyLength {
		WriteErrorResponse(w, http.StatusBadRequest, "KeyTooLongError", "Your key is too long", key)
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, maxObjectSize+1))
	if err != nil {
		WriteErrorResponse(w, http.StatusInternalServerError, "InternalError", "Failed to read request body", key)
		return
	}
	if int64(len(body)) > maxObjectSize {
		WriteErrorResponse(w, http.StatusBadRequest, "EntityTooLarge", "Object too large", key)
		return
	}

	contentType := r.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	// Calculate MD5 for ETag.
	md5Sum := md5.Sum(body)
	etag := hex.EncodeToString(md5Sum[:])

	// Extract x-amz-meta-* headers.
	metadata := extractMetadata(r)

	// Write file to disk.
	storagePath, err := h.writeObjectFile(siteID, key, body)
	if err != nil {
		WriteErrorResponse(w, http.StatusInternalServerError, "InternalError", "Failed to store object", key)
		return
	}

	metaJSON, _ := json.Marshal(metadata)
	now := time.Now().UTC()

	// Upsert the object record.
	var existing models.StorageObject
	err = h.DB.Where("site_id = ? AND key = ?", siteID, key).First(&existing).Error
	if err == nil {
		// Update existing - remove old file if path changed.
		if existing.StoragePath != storagePath {
			os.Remove(existing.StoragePath)
		}
		h.DB.Model(&existing).Updates(map[string]interface{}{
			"size":          int64(len(body)),
			"content_type":  contentType,
			"etag":          QuoteETag(etag),
			"metadata":      string(metaJSON),
			"last_modified": now,
			"storage_path":  storagePath,
		})
	} else {
		obj := models.StorageObject{
			SiteID:       siteID,
			Key:          key,
			Size:         int64(len(body)),
			ContentType:  contentType,
			ETag:         QuoteETag(etag),
			Metadata:     string(metaJSON),
			LastModified: now,
			StoragePath:  storagePath,
		}
		if err := h.DB.Create(&obj).Error; err != nil {
			os.Remove(storagePath)
			WriteErrorResponse(w, http.StatusInternalServerError, "InternalError", "Failed to create object record", key)
			return
		}
	}

	w.Header().Set("ETag", QuoteETag(etag))
	w.WriteHeader(http.StatusOK)
}

// getObject handles GET /:bucket/:key
func (h *Handler) getObject(w http.ResponseWriter, r *http.Request, siteID, key string) {
	var obj models.StorageObject
	if err := h.DB.Where("site_id = ? AND key = ?", siteID, key).First(&obj).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			WriteErrorResponse(w, http.StatusNotFound, "NoSuchKey", "The specified key does not exist.", key)
			return
		}
		WriteErrorResponse(w, http.StatusInternalServerError, "InternalError", "Database error", key)
		return
	}

	// Conditional requests.
	if handled := h.handleConditional(w, r, &obj); handled {
		return
	}

	// Open the file.
	f, err := os.Open(obj.StoragePath)
	if err != nil {
		WriteErrorResponse(w, http.StatusInternalServerError, "InternalError", "Failed to read object", key)
		return
	}
	defer f.Close()

	// Set response headers.
	w.Header().Set("Content-Type", obj.ContentType)
	w.Header().Set("ETag", obj.ETag)
	w.Header().Set("Last-Modified", FormatHTTPTime(obj.LastModified))
	w.Header().Set("Content-Length", strconv.FormatInt(obj.Size, 10))

	// Write metadata headers.
	writeMetadataHeaders(w, obj.Metadata)

	// Handle Range requests.
	rangeHeader := r.Header.Get("Range")
	if rangeHeader != "" {
		h.serveRange(w, r, f, obj.Size, rangeHeader)
		return
	}

	w.WriteHeader(http.StatusOK)
	io.Copy(w, f)
}

// headObject handles HEAD /:bucket/:key
func (h *Handler) headObject(w http.ResponseWriter, r *http.Request, siteID, key string) {
	var obj models.StorageObject
	if err := h.DB.Where("site_id = ? AND key = ?", siteID, key).First(&obj).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			WriteErrorResponse(w, http.StatusNotFound, "NoSuchKey", "The specified key does not exist.", key)
			return
		}
		WriteErrorResponse(w, http.StatusInternalServerError, "InternalError", "Database error", key)
		return
	}

	// Conditional requests.
	if handled := h.handleConditional(w, r, &obj); handled {
		return
	}

	w.Header().Set("Content-Type", obj.ContentType)
	w.Header().Set("ETag", obj.ETag)
	w.Header().Set("Last-Modified", FormatHTTPTime(obj.LastModified))
	w.Header().Set("Content-Length", strconv.FormatInt(obj.Size, 10))

	writeMetadataHeaders(w, obj.Metadata)

	w.WriteHeader(http.StatusOK)
}

// deleteObject handles DELETE /:bucket/:key — idempotent
func (h *Handler) deleteObject(w http.ResponseWriter, r *http.Request, siteID, key string) {
	var obj models.StorageObject
	if err := h.DB.Where("site_id = ? AND key = ?", siteID, key).First(&obj).Error; err == nil {
		os.Remove(obj.StoragePath)
		h.DB.Delete(&obj)
	}
	// S3 returns 204 even if the object didn't exist.
	w.WriteHeader(http.StatusNoContent)
}

// listObjectsV2 handles GET /:bucket?list-type=2
func (h *Handler) listObjectsV2(w http.ResponseWriter, r *http.Request, siteID string) {
	q := r.URL.Query()
	prefix := q.Get("prefix")
	delimiter := q.Get("delimiter")
	startAfter := q.Get("start-after")
	continuationToken := q.Get("continuation-token")
	maxKeysStr := q.Get("max-keys")

	maxKeys := defaultMaxKeys
	if maxKeysStr != "" {
		if mk, err := strconv.Atoi(maxKeysStr); err == nil && mk >= 0 {
			if mk > 1000 {
				mk = 1000
			}
			maxKeys = mk
		}
	}

	// Use continuation token as start-after if present.
	marker := startAfter
	if continuationToken != "" {
		marker = continuationToken
	}

	// Query objects.
	query := h.DB.Where("site_id = ?", siteID)
	if prefix != "" {
		query = query.Where("key LIKE ?", prefix+"%")
	}
	if marker != "" {
		query = query.Where("key > ?", marker)
	}
	query = query.Order("key ASC")

	// Fetch one more than maxKeys to detect truncation.
	var objects []models.StorageObject
	query.Limit(maxKeys + 1).Find(&objects)

	isTruncated := len(objects) > maxKeys
	if isTruncated {
		objects = objects[:maxKeys]
	}

	result := ListBucketResult{
		XMLNS:             "http://s3.amazonaws.com/doc/2006-03-01/",
		Name:              siteID,
		Prefix:            prefix,
		Delimiter:         delimiter,
		MaxKeys:           maxKeys,
		IsTruncated:       isTruncated,
		StartAfter:        startAfter,
		ContinuationToken: continuationToken,
	}

	if delimiter != "" {
		// Group by delimiter.
		prefixSet := make(map[string]bool)
		var contents []ListObject
		for _, obj := range objects {
			// Check if the key (after prefix) contains the delimiter.
			rest := strings.TrimPrefix(obj.Key, prefix)
			idx := strings.Index(rest, delimiter)
			if idx >= 0 {
				// This is a common prefix.
				cp := prefix + rest[:idx+len(delimiter)]
				if !prefixSet[cp] {
					prefixSet[cp] = true
					result.CommonPrefixes = append(result.CommonPrefixes, CommonPrefix{Prefix: cp})
				}
			} else {
				contents = append(contents, ListObject{
					Key:          obj.Key,
					LastModified: FormatS3Time(obj.LastModified),
					ETag:         obj.ETag,
					Size:         obj.Size,
					StorageClass: "STANDARD",
				})
			}
		}
		result.Contents = contents
		result.KeyCount = len(contents) + len(result.CommonPrefixes)
	} else {
		for _, obj := range objects {
			result.Contents = append(result.Contents, ListObject{
				Key:          obj.Key,
				LastModified: FormatS3Time(obj.LastModified),
				ETag:         obj.ETag,
				Size:         obj.Size,
				StorageClass: "STANDARD",
			})
		}
		result.KeyCount = len(result.Contents)
	}

	if isTruncated && len(objects) > 0 {
		result.NextContinuationToken = objects[len(objects)-1].Key
	}

	WriteXMLResponse(w, http.StatusOK, result)
}

// copyObject handles PUT /:bucket/:key with x-amz-copy-source
func (h *Handler) copyObject(w http.ResponseWriter, r *http.Request, siteID, destKey string) {
	copySource := r.Header.Get("X-Amz-Copy-Source")
	// Parse copy source: /bucket/key or bucket/key
	copySource = strings.TrimPrefix(copySource, "/")
	sourceParts := strings.SplitN(copySource, "/", 2)
	if len(sourceParts) < 2 {
		WriteErrorResponse(w, http.StatusBadRequest, "InvalidArgument", "Invalid x-amz-copy-source", destKey)
		return
	}

	sourceBucket := sourceParts[0]
	sourceKey := sourceParts[1]

	// Verify the source bucket belongs to the same site (cross-site copy is not allowed).
	if sourceBucket != siteID {
		WriteErrorResponse(w, http.StatusForbidden, "AccessDenied", "Cross-bucket copy not allowed", destKey)
		return
	}

	// Find the source object.
	var srcObj models.StorageObject
	if err := h.DB.Where("site_id = ? AND key = ?", sourceBucket, sourceKey).First(&srcObj).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			WriteErrorResponse(w, http.StatusNotFound, "NoSuchKey", "The specified key does not exist.", sourceKey)
			return
		}
		WriteErrorResponse(w, http.StatusInternalServerError, "InternalError", "Database error", sourceKey)
		return
	}

	// Read source file.
	srcData, err := os.ReadFile(srcObj.StoragePath)
	if err != nil {
		WriteErrorResponse(w, http.StatusInternalServerError, "InternalError", "Failed to read source object", sourceKey)
		return
	}

	// Determine metadata directive.
	metadataDirective := r.Header.Get("X-Amz-Metadata-Directive")
	contentType := srcObj.ContentType
	metadata := srcObj.Metadata

	if strings.EqualFold(metadataDirective, "REPLACE") {
		contentType = r.Header.Get("Content-Type")
		if contentType == "" {
			contentType = "application/octet-stream"
		}
		meta := extractMetadata(r)
		metaJSON, _ := json.Marshal(meta)
		metadata = string(metaJSON)
	}

	// Write the destination file.
	destPath, err := h.writeObjectFile(siteID, destKey, srcData)
	if err != nil {
		WriteErrorResponse(w, http.StatusInternalServerError, "InternalError", "Failed to store copied object", destKey)
		return
	}

	now := time.Now().UTC()

	// Upsert destination.
	var existing models.StorageObject
	err = h.DB.Where("site_id = ? AND key = ?", siteID, destKey).First(&existing).Error
	if err == nil {
		if existing.StoragePath != destPath {
			os.Remove(existing.StoragePath)
		}
		h.DB.Model(&existing).Updates(map[string]interface{}{
			"size":          srcObj.Size,
			"content_type":  contentType,
			"etag":          srcObj.ETag,
			"metadata":      metadata,
			"last_modified": now,
			"storage_path":  destPath,
		})
	} else {
		obj := models.StorageObject{
			SiteID:       siteID,
			Key:          destKey,
			Size:         srcObj.Size,
			ContentType:  contentType,
			ETag:         srcObj.ETag,
			Metadata:     metadata,
			LastModified: now,
			StoragePath:  destPath,
		}
		if err := h.DB.Create(&obj).Error; err != nil {
			os.Remove(destPath)
			WriteErrorResponse(w, http.StatusInternalServerError, "InternalError", "Failed to create copy record", destKey)
			return
		}
	}

	result := CopyObjectResult{
		LastModified: FormatS3Time(now),
		ETag:         srcObj.ETag,
	}
	WriteXMLResponse(w, http.StatusOK, result)
}

// deleteObjects handles POST /:bucket?delete (batch delete)
func (h *Handler) deleteObjects(w http.ResponseWriter, r *http.Request, siteID string) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		WriteErrorResponse(w, http.StatusBadRequest, "MalformedXML", "Failed to read request body", "")
		return
	}

	var req DeleteRequest
	if err := xml.Unmarshal(body, &req); err != nil {
		WriteErrorResponse(w, http.StatusBadRequest, "MalformedXML", "Failed to parse XML body", "")
		return
	}

	result := DeleteResult{
		XMLNS: "http://s3.amazonaws.com/doc/2006-03-01/",
	}

	for _, obj := range req.Objects {
		var existing models.StorageObject
		if err := h.DB.Where("site_id = ? AND key = ?", siteID, obj.Key).First(&existing).Error; err == nil {
			os.Remove(existing.StoragePath)
			h.DB.Delete(&existing)
		}
		// Always report as deleted (S3 behavior).
		if !req.Quiet {
			result.Deleted = append(result.Deleted, DeletedObject{Key: obj.Key})
		}
	}

	WriteXMLResponse(w, http.StatusOK, result)
}

// Multipart upload handlers

func (h *Handler) createMultipartUpload(w http.ResponseWriter, r *http.Request, siteID, key string) {
	uploadID, _ := gonanoid.Generate("0123456789abcdefghijklmnopqrstuvwxyz", 32)

	contentType := r.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	meta := extractMetadata(r)
	metaJSON, _ := json.Marshal(meta)

	upload := models.MultipartUpload{
		UploadID:    uploadID,
		SiteID:      siteID,
		Key:         key,
		ContentType: contentType,
		Metadata:    string(metaJSON),
	}

	if err := h.DB.Create(&upload).Error; err != nil {
		WriteErrorResponse(w, http.StatusInternalServerError, "InternalError", "Failed to create multipart upload", key)
		return
	}

	result := InitiateMultipartUploadResult{
		XMLNS:    "http://s3.amazonaws.com/doc/2006-03-01/",
		Bucket:   siteID,
		Key:      key,
		UploadId: uploadID,
	}
	WriteXMLResponse(w, http.StatusOK, result)
}

func (h *Handler) uploadPart(w http.ResponseWriter, r *http.Request, siteID, key string) {
	uploadID := r.URL.Query().Get("uploadId")
	partNumberStr := r.URL.Query().Get("partNumber")

	partNumber, err := strconv.Atoi(partNumberStr)
	if err != nil || partNumber < 1 {
		WriteErrorResponse(w, http.StatusBadRequest, "InvalidArgument", "Invalid part number", key)
		return
	}

	// Verify upload exists.
	var upload models.MultipartUpload
	if err := h.DB.Where("upload_id = ? AND site_id = ?", uploadID, siteID).First(&upload).Error; err != nil {
		WriteErrorResponse(w, http.StatusNotFound, "NoSuchUpload", "The specified upload does not exist", key)
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, maxObjectSize+1))
	if err != nil {
		WriteErrorResponse(w, http.StatusInternalServerError, "InternalError", "Failed to read part body", key)
		return
	}

	// Calculate ETag.
	md5Sum := md5.Sum(body)
	etag := hex.EncodeToString(md5Sum[:])

	// Write part to disk.
	partDir := filepath.Join(h.StoragePath, "objects", siteID, ".multipart", uploadID)
	if err := os.MkdirAll(partDir, 0755); err != nil {
		WriteErrorResponse(w, http.StatusInternalServerError, "InternalError", "Failed to create part directory", key)
		return
	}

	partPath := filepath.Join(partDir, fmt.Sprintf("part-%d", partNumber))
	if err := os.WriteFile(partPath, body, 0644); err != nil {
		WriteErrorResponse(w, http.StatusInternalServerError, "InternalError", "Failed to write part", key)
		return
	}

	// Upsert part record.
	var existingPart models.MultipartPart
	err = h.DB.Where("upload_id = ? AND part_number = ?", uploadID, partNumber).First(&existingPart).Error
	if err == nil {
		h.DB.Model(&existingPart).Updates(map[string]interface{}{
			"size":         int64(len(body)),
			"etag":         QuoteETag(etag),
			"storage_path": partPath,
		})
	} else {
		part := models.MultipartPart{
			UploadID:    uploadID,
			PartNumber:  partNumber,
			Size:        int64(len(body)),
			ETag:        QuoteETag(etag),
			StoragePath: partPath,
		}
		h.DB.Create(&part)
	}

	w.Header().Set("ETag", QuoteETag(etag))
	w.WriteHeader(http.StatusOK)
}

func (h *Handler) completeMultipartUpload(w http.ResponseWriter, r *http.Request, siteID, key string) {
	uploadID := r.URL.Query().Get("uploadId")

	var upload models.MultipartUpload
	if err := h.DB.Where("upload_id = ? AND site_id = ?", uploadID, siteID).First(&upload).Error; err != nil {
		WriteErrorResponse(w, http.StatusNotFound, "NoSuchUpload", "The specified upload does not exist", key)
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		WriteErrorResponse(w, http.StatusBadRequest, "MalformedXML", "Failed to read request body", key)
		return
	}

	var completeReq CompleteMultipartUploadRequest
	if err := xml.Unmarshal(body, &completeReq); err != nil {
		WriteErrorResponse(w, http.StatusBadRequest, "MalformedXML", "Failed to parse XML body", key)
		return
	}

	// Sort parts by part number.
	sort.Slice(completeReq.Parts, func(i, j int) bool {
		return completeReq.Parts[i].PartNumber < completeReq.Parts[j].PartNumber
	})

	// Load all part records.
	var parts []models.MultipartPart
	h.DB.Where("upload_id = ?", uploadID).Order("part_number ASC").Find(&parts)

	// Concatenate parts.
	var combined []byte
	hasher := md5.New()
	for _, reqPart := range completeReq.Parts {
		found := false
		for _, dbPart := range parts {
			if dbPart.PartNumber == reqPart.PartNumber {
				data, err := os.ReadFile(dbPart.StoragePath)
				if err != nil {
					WriteErrorResponse(w, http.StatusInternalServerError, "InternalError", "Failed to read part", key)
					return
				}
				combined = append(combined, data...)
				hasher.Write(data)
				found = true
				break
			}
		}
		if !found {
			WriteErrorResponse(w, http.StatusBadRequest, "InvalidPart", fmt.Sprintf("Part %d not found", reqPart.PartNumber), key)
			return
		}
	}

	// Calculate combined ETag (S3 multipart ETag format: md5-partcount).
	combinedMD5 := md5.Sum(combined)
	etag := fmt.Sprintf("%s-%d", hex.EncodeToString(combinedMD5[:]), len(completeReq.Parts))

	// Write final object.
	storagePath, err := h.writeObjectFile(siteID, key, combined)
	if err != nil {
		WriteErrorResponse(w, http.StatusInternalServerError, "InternalError", "Failed to store object", key)
		return
	}

	now := time.Now().UTC()

	// Upsert object record.
	var existing models.StorageObject
	err = h.DB.Where("site_id = ? AND key = ?", siteID, key).First(&existing).Error
	if err == nil {
		if existing.StoragePath != storagePath {
			os.Remove(existing.StoragePath)
		}
		h.DB.Model(&existing).Updates(map[string]interface{}{
			"size":          int64(len(combined)),
			"content_type":  upload.ContentType,
			"etag":          QuoteETag(etag),
			"metadata":      upload.Metadata,
			"last_modified": now,
			"storage_path":  storagePath,
		})
	} else {
		obj := models.StorageObject{
			SiteID:       siteID,
			Key:          key,
			Size:         int64(len(combined)),
			ContentType:  upload.ContentType,
			ETag:         QuoteETag(etag),
			Metadata:     upload.Metadata,
			LastModified: now,
			StoragePath:  storagePath,
		}
		h.DB.Create(&obj)
	}

	// Clean up parts.
	for _, part := range parts {
		os.Remove(part.StoragePath)
	}
	h.DB.Where("upload_id = ?", uploadID).Delete(&models.MultipartPart{})
	h.DB.Delete(&upload)

	// Remove multipart directory.
	os.RemoveAll(filepath.Join(h.StoragePath, "objects", siteID, ".multipart", uploadID))

	result := CompleteMultipartUploadResult{
		XMLNS:  "http://s3.amazonaws.com/doc/2006-03-01/",
		Bucket: siteID,
		Key:    key,
		ETag:   QuoteETag(etag),
	}
	WriteXMLResponse(w, http.StatusOK, result)
}

// Helper methods

func (h *Handler) writeObjectFile(siteID, key string, data []byte) (string, error) {
	// Generate a unique filename to avoid path issues with special key chars.
	fileID, _ := gonanoid.Generate("0123456789abcdefghijklmnopqrstuvwxyz", 20)
	dir := filepath.Join(h.StoragePath, "objects", siteID)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	path := filepath.Join(dir, fileID)
	if err := os.WriteFile(path, data, 0644); err != nil {
		return "", err
	}
	return path, nil
}

func (h *Handler) handleConditional(w http.ResponseWriter, r *http.Request, obj *models.StorageObject) bool {
	// If-None-Match
	ifNoneMatch := r.Header.Get("If-None-Match")
	if ifNoneMatch != "" {
		if ifNoneMatch == obj.ETag || ifNoneMatch == UnquoteETag(obj.ETag) || ifNoneMatch == "*" {
			w.Header().Set("ETag", obj.ETag)
			w.WriteHeader(http.StatusNotModified)
			return true
		}
	}

	// If-Match
	ifMatch := r.Header.Get("If-Match")
	if ifMatch != "" {
		if ifMatch != obj.ETag && ifMatch != UnquoteETag(obj.ETag) && ifMatch != "*" {
			WriteErrorResponse(w, http.StatusPreconditionFailed, "PreconditionFailed", "At least one of the preconditions you specified did not hold.", obj.Key)
			return true
		}
	}

	// If-Modified-Since
	ifModifiedSince := r.Header.Get("If-Modified-Since")
	if ifModifiedSince != "" {
		t, err := http.ParseTime(ifModifiedSince)
		if err == nil {
			if !obj.LastModified.After(t) {
				w.WriteHeader(http.StatusNotModified)
				return true
			}
		}
	}

	// If-Unmodified-Since
	ifUnmodifiedSince := r.Header.Get("If-Unmodified-Since")
	if ifUnmodifiedSince != "" {
		t, err := http.ParseTime(ifUnmodifiedSince)
		if err == nil {
			if obj.LastModified.After(t) {
				WriteErrorResponse(w, http.StatusPreconditionFailed, "PreconditionFailed", "At least one of the preconditions you specified did not hold.", obj.Key)
				return true
			}
		}
	}

	return false
}

func (h *Handler) serveRange(w http.ResponseWriter, r *http.Request, f *os.File, totalSize int64, rangeHeader string) {
	if !strings.HasPrefix(rangeHeader, "bytes=") {
		WriteErrorResponse(w, http.StatusRequestedRangeNotSatisfiable, "InvalidRange", "Invalid range header", "")
		return
	}

	rangeSpec := strings.TrimPrefix(rangeHeader, "bytes=")
	parts := strings.SplitN(rangeSpec, "-", 2)
	if len(parts) != 2 {
		WriteErrorResponse(w, http.StatusRequestedRangeNotSatisfiable, "InvalidRange", "Invalid range format", "")
		return
	}

	var start, end int64
	if parts[0] == "" {
		// Suffix range: bytes=-N
		n, err := strconv.ParseInt(parts[1], 10, 64)
		if err != nil || n <= 0 {
			WriteErrorResponse(w, http.StatusRequestedRangeNotSatisfiable, "InvalidRange", "Invalid suffix range", "")
			return
		}
		start = totalSize - n
		if start < 0 {
			start = 0
		}
		end = totalSize - 1
	} else if parts[1] == "" {
		// Range from start to end: bytes=N-
		var err error
		start, err = strconv.ParseInt(parts[0], 10, 64)
		if err != nil || start < 0 || start >= totalSize {
			WriteErrorResponse(w, http.StatusRequestedRangeNotSatisfiable, "InvalidRange", "Invalid range start", "")
			return
		}
		end = totalSize - 1
	} else {
		var err error
		start, err = strconv.ParseInt(parts[0], 10, 64)
		if err != nil || start < 0 {
			WriteErrorResponse(w, http.StatusRequestedRangeNotSatisfiable, "InvalidRange", "Invalid range start", "")
			return
		}
		end, err = strconv.ParseInt(parts[1], 10, 64)
		if err != nil || end < start {
			WriteErrorResponse(w, http.StatusRequestedRangeNotSatisfiable, "InvalidRange", "Invalid range end", "")
			return
		}
		if end >= totalSize {
			end = totalSize - 1
		}
	}

	if start >= totalSize {
		WriteErrorResponse(w, http.StatusRequestedRangeNotSatisfiable, "InvalidRange", "Range start exceeds file size", "")
		return
	}

	length := end - start + 1
	w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, totalSize))
	w.Header().Set("Content-Length", strconv.FormatInt(length, 10))
	w.Header().Set("Accept-Ranges", "bytes")
	w.WriteHeader(http.StatusPartialContent)

	f.Seek(start, io.SeekStart)
	io.CopyN(w, f, length)
}

func extractMetadata(r *http.Request) map[string]string {
	meta := make(map[string]string)
	for key, values := range r.Header {
		lower := strings.ToLower(key)
		if strings.HasPrefix(lower, "x-amz-meta-") {
			metaKey := strings.TrimPrefix(lower, "x-amz-meta-")
			meta[metaKey] = values[0]
		}
	}
	return meta
}

func writeMetadataHeaders(w http.ResponseWriter, metadataJSON string) {
	if metadataJSON == "" || metadataJSON == "{}" || metadataJSON == "null" {
		return
	}
	var meta map[string]string
	if err := json.Unmarshal([]byte(metadataJSON), &meta); err != nil {
		return
	}
	for k, v := range meta {
		w.Header().Set("X-Amz-Meta-"+titleCase(k), v)
	}
}

func titleCase(s string) string {
	if s == "" {
		return s
	}
	b := []byte(s)
	if b[0] >= 'a' && b[0] <= 'z' {
		b[0] -= 'a' - 'A'
	}
	return string(b)
}
