package storage

import (
	"fmt"
	"sync"
	"testing"
)

func TestCache_GetMiss(t *testing.T) {
	c := NewSiteRulesCache()
	if _, ok := c.Get("nope", "deploy1"); ok {
		t.Fatal("expected miss on empty cache")
	}
}

func TestCache_SetAndGet(t *testing.T) {
	c := NewSiteRulesCache()
	rules := &SiteRules{
		Redirects: []RedirectRule{{From: "/a", To: "/b", StatusCode: 301}},
	}
	c.Set("site1", "deploy1", rules)

	got, ok := c.Get("site1", "deploy1")
	if !ok {
		t.Fatal("expected cache hit")
	}
	if len(got.Redirects) != 1 || got.Redirects[0].From != "/a" {
		t.Errorf("unexpected rules: %+v", got)
	}
}

func TestCache_Invalidate(t *testing.T) {
	c := NewSiteRulesCache()
	c.Set("site1", "deploy1", &SiteRules{})
	c.Invalidate("site1", "deploy1")
	if _, ok := c.Get("site1", "deploy1"); ok {
		t.Fatal("expected miss after invalidate")
	}
}

func TestCache_DifferentVersions(t *testing.T) {
	c := NewSiteRulesCache()
	r1 := &SiteRules{Redirects: []RedirectRule{{From: "/v1"}}}
	r2 := &SiteRules{Redirects: []RedirectRule{{From: "/v2"}}}
	c.Set("site1", "deploy1", r1)
	c.Set("site1", "deploy2", r2)

	got1, _ := c.Get("site1", "deploy1")
	got2, _ := c.Get("site1", "deploy2")
	if got1.Redirects[0].From != "/v1" {
		t.Errorf("deploy1 wrong: %+v", got1)
	}
	if got2.Redirects[0].From != "/v2" {
		t.Errorf("deploy2 wrong: %+v", got2)
	}
}

func TestCache_ConcurrentAccess(t *testing.T) {
	c := NewSiteRulesCache()
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(3)
		key := fmt.Sprintf("deploy%d", i)
		go func() {
			defer wg.Done()
			c.Set("site", key, &SiteRules{})
		}()
		go func() {
			defer wg.Done()
			c.Get("site", key)
		}()
		go func() {
			defer wg.Done()
			c.Invalidate("site", key)
		}()
	}
	wg.Wait()
}
