package workeradapter

import (
	"errors"
	"time"

	"github.com/cryguy/hostedat/internal/models"
	"github.com/cryguy/worker"
	"gorm.io/gorm"
)

// Compile-time interface check.
var _ worker.CacheStore = (*GORMCacheStore)(nil)

// GORMCacheStore implements worker.CacheStore using GORM.
type GORMCacheStore struct {
	DB     *gorm.DB
	SiteID string
}

// Match retrieves a cached response for the given cache name and URL.
// Returns nil if not found or expired.
func (cs *GORMCacheStore) Match(cacheName, url string) (*worker.CacheEntry, error) {
	var entry models.CacheEntry
	err := cs.DB.Where("site_id = ? AND cache_name = ? AND url = ?", cs.SiteID, cacheName, url).First(&entry).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}

	// Check expiration.
	if entry.ExpiresAt != nil && entry.ExpiresAt.Before(time.Now()) {
		cs.DB.Delete(&entry)
		return nil, nil
	}

	return &worker.CacheEntry{
		Status:    entry.Status,
		Headers:   entry.Headers,
		Body:      entry.Body,
		ExpiresAt: entry.ExpiresAt,
	}, nil
}

// Put stores a response in the cache, replacing any existing entry for the URL.
func (cs *GORMCacheStore) Put(cacheName, url string, status int, headers string, body []byte, ttl *int) error {
	var expiresAt *time.Time
	if ttl != nil && *ttl > 0 {
		t := time.Now().Add(time.Duration(*ttl) * time.Second)
		expiresAt = &t
	}

	// Delete existing entry if present.
	cs.DB.Where("site_id = ? AND cache_name = ? AND url = ?", cs.SiteID, cacheName, url).Delete(&models.CacheEntry{})

	entry := models.CacheEntry{
		SiteID:    cs.SiteID,
		CacheName: cacheName,
		URL:       url,
		Status:    status,
		Headers:   headers,
		Body:      body,
		ExpiresAt: expiresAt,
		CreatedAt: time.Now(),
	}
	return cs.DB.Create(&entry).Error
}

// Delete removes a cached response. Returns true if an entry was deleted.
func (cs *GORMCacheStore) Delete(cacheName, url string) (bool, error) {
	result := cs.DB.Where("site_id = ? AND cache_name = ? AND url = ?", cs.SiteID, cacheName, url).Delete(&models.CacheEntry{})
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected > 0, nil
}
