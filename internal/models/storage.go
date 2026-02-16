package models

import (
	"time"

	"gorm.io/gorm"
)

// StorageCredential holds S3-compatible access credentials for a site.
type StorageCredential struct {
	ID              string    `gorm:"primaryKey;size:20" json:"id"`
	SiteID          string    `gorm:"index;not null" json:"site_id"`
	AccessKeyID     string    `gorm:"uniqueIndex;not null;size:40" json:"access_key_id"`
	SecretAccessKey string    `gorm:"not null" json:"-"` // stored hashed; only returned on creation
	SecretKeyHash   string    `gorm:"not null" json:"-"`
	CreatedAt       time.Time `json:"created_at"`

	Site Site `gorm:"foreignKey:SiteID" json:"-"`
}

func (sc *StorageCredential) BeforeCreate(tx *gorm.DB) error {
	if sc.ID == "" {
		sc.ID = generateID()
	}
	return nil
}

// StorageObject represents a stored object in a site's bucket.
type StorageObject struct {
	ID           string    `gorm:"primaryKey;size:20" json:"id"`
	SiteID       string    `gorm:"index:idx_site_key,unique;not null" json:"site_id"`
	Key          string    `gorm:"index:idx_site_key,unique;not null;size:1024" json:"key"`
	Size         int64     `gorm:"not null" json:"size"`
	ContentType  string    `gorm:"not null;default:'application/octet-stream'" json:"content_type"`
	ETag         string    `gorm:"column:etag;not null;size:34" json:"etag"` // MD5 hex with quotes
	Metadata     string    `gorm:"type:text" json:"metadata,omitempty"` // JSON-encoded x-amz-meta-* headers
	LastModified time.Time `gorm:"not null;index" json:"last_modified"`
	StoragePath  string    `gorm:"not null" json:"-"` // path on disk

	Site Site `gorm:"foreignKey:SiteID" json:"-"`
}

func (so *StorageObject) BeforeCreate(tx *gorm.DB) error {
	if so.ID == "" {
		so.ID = generateID()
	}
	return nil
}

// MultipartUpload tracks an in-progress multipart upload.
type MultipartUpload struct {
	ID          string    `gorm:"primaryKey;size:20" json:"id"`
	UploadID    string    `gorm:"uniqueIndex;not null;size:40" json:"upload_id"`
	SiteID      string    `gorm:"index;not null" json:"site_id"`
	Key         string    `gorm:"not null;size:1024" json:"key"`
	ContentType string    `gorm:"not null;default:'application/octet-stream'" json:"content_type"`
	Metadata    string    `gorm:"type:text" json:"metadata,omitempty"`
	CreatedAt   time.Time `json:"created_at"`

	Site  Site            `gorm:"foreignKey:SiteID" json:"-"`
	Parts []MultipartPart `gorm:"foreignKey:UploadID;references:UploadID" json:"parts,omitempty"`
}

func (mu *MultipartUpload) BeforeCreate(tx *gorm.DB) error {
	if mu.ID == "" {
		mu.ID = generateID()
	}
	return nil
}

// MultipartPart tracks a single part of a multipart upload.
type MultipartPart struct {
	ID          string    `gorm:"primaryKey;size:20" json:"id"`
	UploadID    string    `gorm:"index;not null;size:40" json:"upload_id"`
	PartNumber  int       `gorm:"not null" json:"part_number"`
	Size        int64     `gorm:"not null" json:"size"`
	ETag        string    `gorm:"column:etag;not null;size:34" json:"etag"`
	StoragePath string    `gorm:"not null" json:"-"`
	CreatedAt   time.Time `json:"created_at"`
}

func (mp *MultipartPart) BeforeCreate(tx *gorm.DB) error {
	if mp.ID == "" {
		mp.ID = generateID()
	}
	return nil
}
