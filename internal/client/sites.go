package client

import "time"

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
