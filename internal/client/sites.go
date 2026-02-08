package client

import (
	"fmt"
	"strings"
	"time"
)

type Site struct {
	ID            string    `json:"id"`
	UserID        string    `json:"user_id"`
	SubdomainSlug string    `json:"subdomain_slug"`
	Name          string    `json:"name"`
	SPAMode       bool      `json:"spa_mode"`
	ActiveVersion *int      `json:"active_version"`
	CreatedAt     time.Time `json:"created_at"`
}

func (c *Client) ListSites() ([]Site, error) {
	var sites []Site
	err := c.get("/api/v1/sites", &sites)
	return sites, err
}

func (c *Client) CreateSite(name, subdomain string) (*Site, error) {
	body := map[string]string{"name": name}
	if subdomain != "" {
		body["subdomain_slug"] = subdomain
	}
	var site Site
	err := c.post("/api/v1/sites", body, &site)
	if err != nil {
		return nil, err
	}
	return &site, nil
}

func (c *Client) DeleteSite(id string) error {
	return c.delete("/api/v1/sites/"+id, nil)
}

// ResolveSite takes a site name, subdomain slug, or ID and returns the matching site.
func (c *Client) ResolveSite(ref string) (*Site, error) {
	sites, err := c.ListSites()
	if err != nil {
		return nil, fmt.Errorf("failed to list sites: %w", err)
	}

	var matches []Site
	for _, s := range sites {
		if s.ID == ref || s.SubdomainSlug == ref || strings.EqualFold(s.Name, ref) {
			matches = append(matches, s)
		}
	}

	switch len(matches) {
	case 0:
		return nil, fmt.Errorf("no site found matching %q", ref)
	case 1:
		return &matches[0], nil
	default:
		lines := []string{fmt.Sprintf("ambiguous site reference %q matches multiple sites:", ref)}
		for _, s := range matches {
			lines = append(lines, fmt.Sprintf("  %s  %s  (%s)", s.ID, s.Name, s.SubdomainSlug))
		}
		lines = append(lines, "Use the site ID to be specific.")
		return nil, fmt.Errorf("%s", strings.Join(lines, "\n"))
	}
}

// ResolveSiteID is a convenience wrapper that returns just the site ID.
func (c *Client) ResolveSiteID(ref string) (string, error) {
	site, err := c.ResolveSite(ref)
	if err != nil {
		return "", err
	}
	return site.ID, nil
}

func (c *Client) UpdateSite(id string, name *string, spaMode *bool) (*Site, error) {
	body := map[string]interface{}{}
	if name != nil {
		body["name"] = *name
	}
	if spaMode != nil {
		body["spa_mode"] = *spaMode
	}
	var site Site
	err := c.patch("/api/v1/sites/"+id, body, &site)
	if err != nil {
		return nil, err
	}
	return &site, nil
}
