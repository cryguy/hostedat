package storage

import (
	"fmt"
	"sync"
)

type SiteRules struct {
	Redirects []RedirectRule
	Headers   []HeaderRule
}

type SiteRulesCache struct {
	mu    sync.RWMutex
	rules map[string]*SiteRules
}

func NewSiteRulesCache() *SiteRulesCache {
	return &SiteRulesCache{
		rules: make(map[string]*SiteRules),
	}
}

func cacheKey(siteID string, version int) string {
	return fmt.Sprintf("%s:%d", siteID, version)
}

func (c *SiteRulesCache) Get(siteID string, version int) (*SiteRules, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	r, ok := c.rules[cacheKey(siteID, version)]
	return r, ok
}

func (c *SiteRulesCache) Set(siteID string, version int, rules *SiteRules) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.rules[cacheKey(siteID, version)] = rules
}

func (c *SiteRulesCache) Invalidate(siteID string, version int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.rules, cacheKey(siteID, version))
}
