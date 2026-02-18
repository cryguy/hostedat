package storage

import (
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

func cacheKey(siteID string, deployKey string) string {
	return siteID + ":" + deployKey
}

func (c *SiteRulesCache) Get(siteID string, deployKey string) (*SiteRules, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	r, ok := c.rules[cacheKey(siteID, deployKey)]
	return r, ok
}

func (c *SiteRulesCache) Set(siteID string, deployKey string, rules *SiteRules) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.rules[cacheKey(siteID, deployKey)] = rules
}

func (c *SiteRulesCache) Invalidate(siteID string, deployKey string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.rules, cacheKey(siteID, deployKey))
}
