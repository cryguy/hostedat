package storage

import (
	"sync"
	"time"
)

const (
	maxCacheEntries = 1000
	cacheTTL        = 5 * time.Minute
)

type SiteRules struct {
	Redirects []RedirectRule
	Headers   []HeaderRule
}

type cacheEntry struct {
	rules   *SiteRules
	addedAt time.Time
}

// SiteRulesCache is a bounded cache with TTL-based eviction for parsed
// _redirects and _headers rules. It caps at maxCacheEntries and evicts
// stale entries on access and periodically during Set.
type SiteRulesCache struct {
	mu      sync.RWMutex
	entries map[string]*cacheEntry
}

func NewSiteRulesCache() *SiteRulesCache {
	return &SiteRulesCache{
		entries: make(map[string]*cacheEntry),
	}
}

func cacheKey(siteID string, deployKey string) string {
	return siteID + ":" + deployKey
}

func (c *SiteRulesCache) Get(siteID string, deployKey string) (*SiteRules, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	e, ok := c.entries[cacheKey(siteID, deployKey)]
	if !ok {
		return nil, false
	}
	if time.Since(e.addedAt) > cacheTTL {
		return nil, false // stale; will be cleaned on next Set
	}
	return e.rules, true
}

func (c *SiteRulesCache) Set(siteID string, deployKey string, rules *SiteRules) {
	c.mu.Lock()
	defer c.mu.Unlock()

	key := cacheKey(siteID, deployKey)
	c.entries[key] = &cacheEntry{rules: rules, addedAt: time.Now()}

	// Evict stale entries and enforce size cap.
	if len(c.entries) > maxCacheEntries {
		c.evictLocked()
	}
}

func (c *SiteRulesCache) Invalidate(siteID string, deployKey string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.entries, cacheKey(siteID, deployKey))
}

// evictLocked removes expired entries and, if still over capacity, removes
// the oldest entries. Must be called with c.mu held.
func (c *SiteRulesCache) evictLocked() {
	now := time.Now()

	// First pass: remove expired entries.
	for k, e := range c.entries {
		if now.Sub(e.addedAt) > cacheTTL {
			delete(c.entries, k)
		}
	}

	// Second pass: if still over cap, evict oldest entries.
	for len(c.entries) > maxCacheEntries {
		var oldestKey string
		var oldestTime time.Time
		for k, e := range c.entries {
			if oldestKey == "" || e.addedAt.Before(oldestTime) {
				oldestKey = k
				oldestTime = e.addedAt
			}
		}
		if oldestKey != "" {
			delete(c.entries, oldestKey)
		}
	}
}
