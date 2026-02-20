package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/cryguy/hostedat/internal/models"
)

func TestSiteCreate_Success(t *testing.T) {
	env := setupTestEnv(t)
	_, token := env.createTestUser(t, "u@t.com", "password123", "user")

	req := httptest.NewRequest(http.MethodPost, "/api/v1/sites",
		jsonBody(map[string]string{"name": "My Site", "subdomain_slug": "mysite"}))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	rec := env.doRequest(req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201: %s", rec.Code, rec.Body.String())
	}

	body := parseJSON(t, rec)
	if body["subdomain_slug"] != "mysite" {
		t.Errorf("subdomain_slug = %v, want mysite", body["subdomain_slug"])
	}
	if body["name"] != "My Site" {
		t.Errorf("name = %v, want My Site", body["name"])
	}
}

func TestSiteCreate_AutoGenerateSlug(t *testing.T) {
	env := setupTestEnv(t)
	_, token := env.createTestUser(t, "u@t.com", "password123", "user")

	req := httptest.NewRequest(http.MethodPost, "/api/v1/sites",
		jsonBody(map[string]string{"name": "My Cool Site"}))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	rec := env.doRequest(req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201: %s", rec.Code, rec.Body.String())
	}

	body := parseJSON(t, rec)
	slug, ok := body["subdomain_slug"].(string)
	if !ok || slug == "" {
		t.Errorf("subdomain_slug should be auto-generated, got %v", body["subdomain_slug"])
	}
}

func TestSiteCreate_MissingName(t *testing.T) {
	env := setupTestEnv(t)
	_, token := env.createTestUser(t, "u@t.com", "password123", "user")

	req := httptest.NewRequest(http.MethodPost, "/api/v1/sites",
		jsonBody(map[string]string{"subdomain_slug": "test"}))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	rec := env.doRequest(req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

func TestSiteCreate_InvalidSlug(t *testing.T) {
	env := setupTestEnv(t)
	_, token := env.createTestUser(t, "u@t.com", "password123", "user")

	tests := []struct {
		name string
		slug string
	}{
		{"too short", "ab"},
		{"special chars", "my_site"},
		{"spaces", "my site"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/v1/sites",
				jsonBody(map[string]string{"name": "Test", "subdomain_slug": tt.slug}))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", "Bearer "+token)
			rec := env.doRequest(req)

			if rec.Code != http.StatusBadRequest {
				t.Errorf("slug %q: status = %d, want 400", tt.slug, rec.Code)
			}
		})
	}
}

func TestSiteCreate_ReservedSlug(t *testing.T) {
	env := setupTestEnv(t)
	_, token := env.createTestUser(t, "u@t.com", "password123", "user")

	reserved := []string{"www", "api", "admin", "storage"}
	for _, slug := range reserved {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/sites",
			jsonBody(map[string]string{"name": "Test", "subdomain_slug": slug}))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)
		rec := env.doRequest(req)

		if rec.Code != http.StatusBadRequest {
			t.Errorf("reserved slug %q: status = %d, want 400", slug, rec.Code)
		}
	}
}

func TestSiteCreate_DuplicateSlug(t *testing.T) {
	env := setupTestEnv(t)
	user, token := env.createTestUser(t, "u@t.com", "password123", "user")

	// Create first site
	env.db.Create(&models.Site{UserID: user.ID, SubdomainSlug: "existing", Name: "Existing"})

	// Try to create second site with same slug
	req := httptest.NewRequest(http.MethodPost, "/api/v1/sites",
		jsonBody(map[string]string{"name": "New", "subdomain_slug": "existing"}))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	rec := env.doRequest(req)

	if rec.Code != http.StatusConflict {
		t.Errorf("status = %d, want 409", rec.Code)
	}
}

func TestSiteCreate_Unauthenticated(t *testing.T) {
	env := setupTestEnv(t)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/sites",
		jsonBody(map[string]string{"name": "Test"}))
	req.Header.Set("Content-Type", "application/json")
	rec := env.doRequest(req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
}

func TestSiteList_UserSites(t *testing.T) {
	env := setupTestEnv(t)
	user, token := env.createTestUser(t, "u@t.com", "password123", "user")
	otherUser, _ := env.createTestUser(t, "other@t.com", "password123", "user")

	// Create sites for both users
	env.db.Create(&models.Site{UserID: user.ID, SubdomainSlug: "mysite1", Name: "Site 1"})
	env.db.Create(&models.Site{UserID: user.ID, SubdomainSlug: "mysite2", Name: "Site 2"})
	env.db.Create(&models.Site{UserID: otherUser.ID, SubdomainSlug: "othersite", Name: "Other Site"})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/sites", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := env.doRequest(req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}

	var sites []map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &sites); err != nil {
		t.Fatalf("parse sites: %v", err)
	}

	if len(sites) != 2 {
		t.Errorf("got %d sites, want 2", len(sites))
	}
}

func TestSiteList_AdminAllSites(t *testing.T) {
	env := setupTestEnv(t)
	user, _ := env.createTestUser(t, "u@t.com", "password123", "user")
	_, adminToken := env.createTestUser(t, "admin@t.com", "password123", "admin")

	env.db.Create(&models.Site{UserID: user.ID, SubdomainSlug: "usersite", Name: "User Site"})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/sites?all=true", nil)
	req.Header.Set("Authorization", "Bearer "+adminToken)
	rec := env.doRequest(req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}

	var sites []map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &sites); err != nil {
		t.Fatalf("parse sites: %v", err)
	}

	if len(sites) != 1 {
		t.Errorf("admin should see 1 site, got %d", len(sites))
	}
}

func TestSiteGet_Success(t *testing.T) {
	env := setupTestEnv(t)
	user, token := env.createTestUser(t, "u@t.com", "password123", "user")

	site := models.Site{UserID: user.ID, SubdomainSlug: "mysite", Name: "My Site"}
	env.db.Create(&site)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/sites/"+site.ID, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := env.doRequest(req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}

	body := parseJSON(t, rec)
	if body["id"] != site.ID {
		t.Errorf("id = %v, want %v", body["id"], site.ID)
	}
}

func TestSiteGet_NotFound(t *testing.T) {
	env := setupTestEnv(t)
	_, token := env.createTestUser(t, "u@t.com", "password123", "user")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/sites/nonexistent", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := env.doRequest(req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}

func TestSiteGet_Forbidden(t *testing.T) {
	env := setupTestEnv(t)
	user, _ := env.createTestUser(t, "u@t.com", "password123", "user")
	_, otherToken := env.createTestUser(t, "other@t.com", "password123", "user")

	site := models.Site{UserID: user.ID, SubdomainSlug: "mysite", Name: "My Site"}
	env.db.Create(&site)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/sites/"+site.ID, nil)
	req.Header.Set("Authorization", "Bearer "+otherToken)
	rec := env.doRequest(req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", rec.Code)
	}
}

func TestSiteGet_AdminCanAccess(t *testing.T) {
	env := setupTestEnv(t)
	user, _ := env.createTestUser(t, "u@t.com", "password123", "user")
	_, adminToken := env.createTestUser(t, "admin@t.com", "password123", "admin")

	site := models.Site{UserID: user.ID, SubdomainSlug: "mysite", Name: "My Site"}
	env.db.Create(&site)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/sites/"+site.ID, nil)
	req.Header.Set("Authorization", "Bearer "+adminToken)
	rec := env.doRequest(req)

	if rec.Code != http.StatusOK {
		t.Errorf("admin: status = %d, want 200", rec.Code)
	}
}

func TestSiteUpdate_Name(t *testing.T) {
	env := setupTestEnv(t)
	user, token := env.createTestUser(t, "u@t.com", "password123", "user")

	site := models.Site{UserID: user.ID, SubdomainSlug: "mysite", Name: "Old Name"}
	env.db.Create(&site)

	newName := "New Name"
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/sites/"+site.ID,
		jsonBody(map[string]interface{}{"name": newName}))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	rec := env.doRequest(req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}

	body := parseJSON(t, rec)
	if body["name"] != newName {
		t.Errorf("name = %v, want %v", body["name"], newName)
	}
}

func TestSiteUpdate_SPAMode(t *testing.T) {
	env := setupTestEnv(t)
	user, token := env.createTestUser(t, "u@t.com", "password123", "user")

	site := models.Site{UserID: user.ID, SubdomainSlug: "mysite", Name: "Site", SPAMode: false}
	env.db.Create(&site)

	req := httptest.NewRequest(http.MethodPatch, "/api/v1/sites/"+site.ID,
		jsonBody(map[string]interface{}{"spa_mode": true}))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	rec := env.doRequest(req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}

	body := parseJSON(t, rec)
	if body["spa_mode"] != true {
		t.Errorf("spa_mode = %v, want true", body["spa_mode"])
	}
}

func TestSiteUpdate_NotFound(t *testing.T) {
	env := setupTestEnv(t)
	_, token := env.createTestUser(t, "u@t.com", "password123", "user")

	req := httptest.NewRequest(http.MethodPatch, "/api/v1/sites/nonexistent",
		jsonBody(map[string]string{"name": "New"}))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	rec := env.doRequest(req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}

func TestSiteUpdate_Forbidden(t *testing.T) {
	env := setupTestEnv(t)
	user, _ := env.createTestUser(t, "u@t.com", "password123", "user")
	_, otherToken := env.createTestUser(t, "other@t.com", "password123", "user")

	site := models.Site{UserID: user.ID, SubdomainSlug: "mysite", Name: "Site"}
	env.db.Create(&site)

	req := httptest.NewRequest(http.MethodPatch, "/api/v1/sites/"+site.ID,
		jsonBody(map[string]string{"name": "New"}))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+otherToken)
	rec := env.doRequest(req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", rec.Code)
	}
}

func TestSiteDelete_Success(t *testing.T) {
	env := setupTestEnv(t)
	user, token := env.createTestUser(t, "u@t.com", "password123", "user")

	site := models.Site{UserID: user.ID, SubdomainSlug: "mysite", Name: "Site"}
	env.db.Create(&site)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/sites/"+site.ID, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := env.doRequest(req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}

	// Verify site is deleted
	var count int64
	env.db.Model(&models.Site{}).Where("id = ?", site.ID).Count(&count)
	if count != 0 {
		t.Errorf("site should be deleted, but still exists")
	}
}

func TestSiteDelete_WithChildRecords(t *testing.T) {
	env := setupTestEnv(t)
	user, token := env.createTestUser(t, "u@t.com", "password123", "user")

	site := models.Site{UserID: user.ID, SubdomainSlug: "mysite", Name: "Site"}
	env.db.Create(&site)

	// Create child records
	env.db.Create(&models.Deployment{SiteID: site.ID, Version: 1, FileHash: "hash1"})
	env.db.Create(&models.WorkerEnvVar{SiteID: site.ID, Name: "KEY", Value: "value"})
	env.db.Create(&models.KVNamespace{SiteID: site.ID, Name: "KV"})
	env.db.Create(&models.CronSchedule{SiteID: site.ID, Cron: "* * * * *"})
	env.db.Create(&models.WorkerLog{SiteID: site.ID, Level: "info", Message: "test"})

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/sites/"+site.ID, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := env.doRequest(req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}

	// Verify all child records are deleted
	var depCount, envCount, kvCount, cronCount, logCount int64
	env.db.Model(&models.Deployment{}).Where("site_id = ?", site.ID).Count(&depCount)
	env.db.Model(&models.WorkerEnvVar{}).Where("site_id = ?", site.ID).Count(&envCount)
	env.db.Model(&models.KVNamespace{}).Where("site_id = ?", site.ID).Count(&kvCount)
	env.db.Model(&models.CronSchedule{}).Where("site_id = ?", site.ID).Count(&cronCount)
	env.db.Model(&models.WorkerLog{}).Where("site_id = ?", site.ID).Count(&logCount)

	if depCount != 0 || envCount != 0 || kvCount != 0 || cronCount != 0 || logCount != 0 {
		t.Errorf("child records not deleted: dep=%d env=%d kv=%d cron=%d log=%d",
			depCount, envCount, kvCount, cronCount, logCount)
	}
}

func TestSiteDelete_NotFound(t *testing.T) {
	env := setupTestEnv(t)
	_, token := env.createTestUser(t, "u@t.com", "password123", "user")

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/sites/nonexistent", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := env.doRequest(req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}

func TestSiteDelete_Forbidden(t *testing.T) {
	env := setupTestEnv(t)
	user, _ := env.createTestUser(t, "u@t.com", "password123", "user")
	_, otherToken := env.createTestUser(t, "other@t.com", "password123", "user")

	site := models.Site{UserID: user.ID, SubdomainSlug: "mysite", Name: "Site"}
	env.db.Create(&site)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/sites/"+site.ID, nil)
	req.Header.Set("Authorization", "Bearer "+otherToken)
	rec := env.doRequest(req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", rec.Code)
	}
}
