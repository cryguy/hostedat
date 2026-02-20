package models

import (
	"time"

	gonanoid "github.com/matoous/go-nanoid/v2"
	"gorm.io/gorm"
)

const nanoidAlphabet = "0123456789abcdefghijklmnopqrstuvwxyz"
const nanoidLength = 12

func generateID() string {
	return GenerateID()
}

// GenerateID creates a new nanoid suitable for use as a primary key or deploy path key.
func GenerateID() string {
	id, err := gonanoid.Generate(nanoidAlphabet, nanoidLength)
	if err != nil {
		panic("nanoid generation failed: " + err.Error())
	}
	return id
}

type User struct {
	ID           string    `gorm:"primaryKey;size:20" json:"id"`
	Email        string    `gorm:"uniqueIndex;not null" json:"email"`
	PasswordHash string    `gorm:"not null" json:"-"`
	Role         string    `gorm:"not null;default:user" json:"role"` // superadmin | admin | user
	InvitedBy    *string   `gorm:"size:20" json:"invited_by,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
}

func (u *User) BeforeCreate(_ *gorm.DB) error {
	if u.ID == "" {
		u.ID = generateID()
	}
	if u.Role == "" {
		u.Role = "user"
	}
	return nil
}

type Site struct {
	ID             string    `gorm:"primaryKey;size:20" json:"id"`
	UserID         string    `gorm:"index;not null" json:"user_id"`
	SubdomainSlug  string    `gorm:"uniqueIndex;not null" json:"subdomain_slug"`
	Name           string    `gorm:"not null" json:"name"`
	SPAMode        bool      `gorm:"default:false" json:"spa_mode"`
	HasWorker      bool      `gorm:"default:false" json:"has_worker"`
	ActiveVersion  *int      `json:"active_version"`
	ActiveDeployID *string   `gorm:"size:20" json:"active_deploy_id,omitempty"`
	CreatedAt      time.Time `json:"created_at"`

	User        User         `gorm:"foreignKey:UserID" json:"-"`
	Deployments []Deployment `gorm:"foreignKey:SiteID" json:"deployments,omitempty"`
}

func (s *Site) BeforeCreate(_ *gorm.DB) error {
	if s.ID == "" {
		s.ID = generateID()
	}
	return nil
}

type Deployment struct {
	ID         string    `gorm:"primaryKey;size:20" json:"id"`
	SiteID     string    `gorm:"not null;uniqueIndex:idx_deployments_site_version,priority:1" json:"site_id"`
	Version    int       `gorm:"not null;uniqueIndex:idx_deployments_site_version,priority:2" json:"version"`
	FileHash   string    `gorm:"not null" json:"file_hash"`
	HasWorker  bool      `gorm:"default:false" json:"has_worker"`
	UploadedAt time.Time `json:"uploaded_at"`

	Site Site `gorm:"foreignKey:SiteID" json:"-"`
}

func (d *Deployment) BeforeCreate(_ *gorm.DB) error {
	if d.ID == "" {
		d.ID = generateID()
	}
	if d.UploadedAt.IsZero() {
		d.UploadedAt = time.Now()
	}
	return nil
}

type APIKey struct {
	ID         string     `gorm:"primaryKey;size:20" json:"id"`
	UserID     string     `gorm:"index;not null" json:"user_id"`
	KeyHash    string     `gorm:"uniqueIndex;not null" json:"-"`
	Name       string     `gorm:"not null" json:"name"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`

	User User `gorm:"foreignKey:UserID" json:"-"`
}

func (k *APIKey) BeforeCreate(_ *gorm.DB) error {
	if k.ID == "" {
		k.ID = generateID()
	}
	return nil
}

type Invite struct {
	ID        string     `gorm:"primaryKey;size:20" json:"id"`
	Code      string     `gorm:"uniqueIndex;not null" json:"code"`
	CreatedBy string     `gorm:"not null" json:"created_by"`
	MaxUses   *int       `json:"max_uses,omitempty"`
	UseCount  int        `gorm:"default:0" json:"use_count"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
	Active    bool       `gorm:"default:true" json:"active"`
	CreatedAt time.Time  `json:"created_at"`

	Creator User `gorm:"foreignKey:CreatedBy" json:"-"`
}

func (i *Invite) BeforeCreate(_ *gorm.DB) error {
	if i.ID == "" {
		i.ID = generateID()
	}
	return nil
}

type AuthCode struct {
	ID            string    `gorm:"primaryKey;size:20" json:"id"`
	Code          string    `gorm:"uniqueIndex;not null" json:"-"`
	UserID        string    `gorm:"not null" json:"user_id"`
	CodeChallenge string    `gorm:"not null" json:"-"`
	Used          bool      `gorm:"default:false" json:"-"`
	ExpiresAt     time.Time `gorm:"not null" json:"-"`
	CreatedAt     time.Time `json:"created_at"`
}

func (a *AuthCode) BeforeCreate(_ *gorm.DB) error {
	if a.ID == "" {
		a.ID = generateID()
	}
	return nil
}

type Setting struct {
	Key   string `gorm:"primaryKey" json:"key"`
	Value string `gorm:"not null" json:"value"`
}

// RevokedToken tracks JWTs that have been explicitly revoked (e.g. logout).
// Entries are cleaned up after they expire.
type RevokedToken struct {
	TokenHash string    `gorm:"primaryKey;size:64"` // SHA-256 of the raw JWT
	ExpiresAt time.Time `gorm:"index;not null"`     // When the JWT would naturally expire
}

// Worker-related models

type WorkerEnvVar struct {
	ID     string `gorm:"primaryKey;size:20" json:"id"`
	SiteID string `gorm:"index;not null" json:"site_id"`
	Name   string `gorm:"not null" json:"name"`
	Value  string `gorm:"not null" json:"value"`
	Secret bool   `gorm:"default:false" json:"secret"`

	Site Site `gorm:"foreignKey:SiteID" json:"-"`
}

func (w *WorkerEnvVar) BeforeCreate(_ *gorm.DB) error {
	if w.ID == "" {
		w.ID = generateID()
	}
	return nil
}

type KVNamespace struct {
	ID        string    `gorm:"primaryKey;size:20" json:"id"`
	SiteID    string    `gorm:"index;not null" json:"site_id"`
	Name      string    `gorm:"not null" json:"name"`
	CreatedAt time.Time `json:"created_at"`

	Site Site `gorm:"foreignKey:SiteID" json:"-"`
}

func (k *KVNamespace) BeforeCreate(_ *gorm.DB) error {
	if k.ID == "" {
		k.ID = generateID()
	}
	return nil
}

type KVEntry struct {
	ID          string     `gorm:"primaryKey;size:20" json:"id"`
	NamespaceID string     `gorm:"index;not null" json:"namespace_id"`
	Key         string     `gorm:"not null" json:"key"`
	Value       string     `gorm:"type:text" json:"value"`
	Metadata    *string    `gorm:"type:text" json:"metadata,omitempty"`
	ExpiresAt   *time.Time `json:"expires_at,omitempty"`

	Namespace KVNamespace `gorm:"foreignKey:NamespaceID" json:"-"`
}

func (k *KVEntry) BeforeCreate(_ *gorm.DB) error {
	if k.ID == "" {
		k.ID = generateID()
	}
	return nil
}

type CronSchedule struct {
	ID        string     `gorm:"primaryKey;size:20" json:"id"`
	SiteID    string     `gorm:"index;not null" json:"site_id"`
	Cron      string     `gorm:"not null" json:"cron"`
	Enabled   bool       `gorm:"default:true" json:"enabled"`
	LastRunAt *time.Time `json:"last_run_at,omitempty"`
	CreatedAt time.Time  `json:"created_at"`

	Site Site `gorm:"foreignKey:SiteID" json:"-"`
}

func (cs *CronSchedule) BeforeCreate(_ *gorm.DB) error {
	if cs.ID == "" {
		cs.ID = generateID()
	}
	return nil
}

type WorkerLog struct {
	ID        string    `gorm:"primaryKey;size:20" json:"id"`
	SiteID    string    `gorm:"index;not null" json:"site_id"`
	Level     string    `gorm:"not null" json:"level"`
	Message   string    `gorm:"type:text;not null" json:"message"`
	CreatedAt time.Time `gorm:"index" json:"created_at"`

	Site Site `gorm:"foreignKey:SiteID" json:"-"`
}

func (wl *WorkerLog) BeforeCreate(_ *gorm.DB) error {
	if wl.ID == "" {
		wl.ID = generateID()
	}
	return nil
}

// Storage-related models

type StorageBucket struct {
	ID         string    `gorm:"primaryKey;size:20" json:"id"`
	SiteID     string    `gorm:"not null;uniqueIndex:idx_storage_buckets_site_name,priority:1" json:"site_id"`
	Name       string    `gorm:"not null;uniqueIndex:idx_storage_buckets_site_name,priority:2" json:"name"`
	BucketName string    `gorm:"uniqueIndex;not null" json:"bucket_name"`
	Public     bool      `gorm:"default:false" json:"public"`
	CreatedAt  time.Time `json:"created_at"`

	Site Site `gorm:"foreignKey:SiteID" json:"-"`
}

func (sb *StorageBucket) BeforeCreate(_ *gorm.DB) error {
	if sb.ID == "" {
		sb.ID = generateID()
	}
	return nil
}

// D1Database represents a D1 database binding for a site.
type D1Database struct {
	ID         string    `gorm:"primaryKey;size:20" json:"id"`
	SiteID     string    `gorm:"not null;uniqueIndex:idx_d1_databases_site_name,priority:1" json:"site_id"`
	Name       string    `gorm:"not null;uniqueIndex:idx_d1_databases_site_name,priority:2" json:"name"`
	DatabaseID string    `gorm:"uniqueIndex;not null" json:"database_id"`
	CreatedAt  time.Time `json:"created_at"`

	Site Site `gorm:"foreignKey:SiteID" json:"-"`
}

func (d *D1Database) BeforeCreate(_ *gorm.DB) error {
	if d.ID == "" {
		d.ID = generateID()
	}
	if d.DatabaseID == "" {
		d.DatabaseID = generateID()
	}
	return nil
}

// DurableObjectNamespace represents a Durable Object namespace binding for a site.
type DurableObjectNamespace struct {
	ID          string    `gorm:"primaryKey;size:20" json:"id"`
	SiteID      string    `gorm:"not null;uniqueIndex:idx_do_namespaces_site_name,priority:1" json:"site_id"`
	Name        string    `gorm:"not null;uniqueIndex:idx_do_namespaces_site_name,priority:2" json:"name"`
	NamespaceID string    `gorm:"uniqueIndex;not null" json:"namespace_id"`
	CreatedAt   time.Time `json:"created_at"`

	Site Site `gorm:"foreignKey:SiteID" json:"-"`
}

func (d *DurableObjectNamespace) BeforeCreate(_ *gorm.DB) error {
	if d.ID == "" {
		d.ID = generateID()
	}
	if d.NamespaceID == "" {
		d.NamespaceID = generateID()
	}
	return nil
}

// DurableObjectEntry is the GORM model for durable object storage.
type DurableObjectEntry struct {
	ID        uint   `gorm:"primaryKey"`
	Namespace string `gorm:"uniqueIndex:idx_do_lookup"`
	ObjectID  string `gorm:"uniqueIndex:idx_do_lookup"`
	Key       string `gorm:"uniqueIndex:idx_do_lookup"`
	ValueJSON string
}

type S3Credential struct {
	ID            string     `gorm:"primaryKey;size:20" json:"id"`
	UserID        string     `gorm:"index;not null" json:"user_id"`
	ExternalKeyID string     `gorm:"uniqueIndex;not null" json:"external_key_id"`
	AccessKeyID   string     `gorm:"uniqueIndex;not null" json:"access_key_id"`
	Name          string     `gorm:"not null" json:"name"`
	LastUsedAt    *time.Time `json:"last_used_at,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`

	User User `gorm:"foreignKey:UserID" json:"-"`
}

func (sc *S3Credential) BeforeCreate(_ *gorm.DB) error {
	if sc.ID == "" {
		sc.ID = generateID()
	}
	return nil
}

// CacheEntry stores a cached HTTP response for the Cache API.
type CacheEntry struct {
	ID        uint   `gorm:"primaryKey"`
	SiteID    string `gorm:"index:idx_cache_lookup"`
	CacheName string `gorm:"index:idx_cache_lookup"`
	URL       string `gorm:"index:idx_cache_lookup"`
	Status    int
	Headers   string
	Body      []byte
	ExpiresAt *time.Time
	CreatedAt time.Time
}
