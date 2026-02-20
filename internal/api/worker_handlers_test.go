package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/cryguy/hostedat/internal/models"
)

func TestWorkerSetEnvVar_Create(t *testing.T) {
	env := setupTestEnv(t)
	user, token := env.createTestUser(t, "u@t.com", "password123", "user")

	site := models.Site{UserID: user.ID, SubdomainSlug: "mysite", Name: "Site"}
	env.db.Create(&site)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/sites/"+site.ID+"/worker/env",
		jsonBody(map[string]interface{}{"name": "API_KEY", "value": "secret123", "secret": true}))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	rec := env.doRequest(req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}

	body := parseJSON(t, rec)
	if body["name"] != "API_KEY" {
		t.Errorf("name = %v, want API_KEY", body["name"])
	}
	if body["secret"] != true {
		t.Errorf("secret = %v, want true", body["secret"])
	}
}

func TestWorkerSetEnvVar_Update(t *testing.T) {
	env := setupTestEnv(t)
	user, token := env.createTestUser(t, "u@t.com", "password123", "user")

	site := models.Site{UserID: user.ID, SubdomainSlug: "mysite", Name: "Site"}
	env.db.Create(&site)
	env.db.Create(&models.WorkerEnvVar{SiteID: site.ID, Name: "KEY", Value: "old", Secret: false})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/sites/"+site.ID+"/worker/env",
		jsonBody(map[string]interface{}{"name": "KEY", "value": "new", "secret": true}))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	rec := env.doRequest(req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}

	body := parseJSON(t, rec)
	if body["value"] != "new" {
		t.Errorf("value = %v, want new", body["value"])
	}
}

func TestWorkerSetEnvVar_MissingName(t *testing.T) {
	env := setupTestEnv(t)
	user, token := env.createTestUser(t, "u@t.com", "password123", "user")

	site := models.Site{UserID: user.ID, SubdomainSlug: "mysite", Name: "Site"}
	env.db.Create(&site)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/sites/"+site.ID+"/worker/env",
		jsonBody(map[string]string{"value": "test"}))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	rec := env.doRequest(req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

func TestWorkerSetEnvVar_SiteNotFound(t *testing.T) {
	env := setupTestEnv(t)
	_, token := env.createTestUser(t, "u@t.com", "password123", "user")

	req := httptest.NewRequest(http.MethodPost, "/api/v1/sites/nonexistent/worker/env",
		jsonBody(map[string]string{"name": "KEY", "value": "val"}))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	rec := env.doRequest(req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}

func TestWorkerSetEnvVar_Forbidden(t *testing.T) {
	env := setupTestEnv(t)
	user, _ := env.createTestUser(t, "u@t.com", "password123", "user")
	_, otherToken := env.createTestUser(t, "other@t.com", "password123", "user")

	site := models.Site{UserID: user.ID, SubdomainSlug: "mysite", Name: "Site"}
	env.db.Create(&site)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/sites/"+site.ID+"/worker/env",
		jsonBody(map[string]string{"name": "KEY", "value": "val"}))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+otherToken)
	rec := env.doRequest(req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", rec.Code)
	}
}

func TestWorkerListEnvVars_Success(t *testing.T) {
	env := setupTestEnv(t)
	user, token := env.createTestUser(t, "u@t.com", "password123", "user")

	site := models.Site{UserID: user.ID, SubdomainSlug: "mysite", Name: "Site"}
	env.db.Create(&site)
	env.db.Create(&models.WorkerEnvVar{SiteID: site.ID, Name: "PUBLIC_KEY", Value: "public123", Secret: false})
	env.db.Create(&models.WorkerEnvVar{SiteID: site.ID, Name: "SECRET_KEY", Value: "secret123", Secret: true})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/sites/"+site.ID+"/worker/env", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := env.doRequest(req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}

	var vars []map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &vars); err != nil {
		t.Fatalf("parse vars: %v", err)
	}

	if len(vars) != 2 {
		t.Errorf("got %d vars, want 2", len(vars))
	}

	// Check that secret values are masked
	for _, v := range vars {
		if v["name"] == "SECRET_KEY" && v["value"] != "********" {
			t.Errorf("secret value should be masked, got %v", v["value"])
		}
		if v["name"] == "PUBLIC_KEY" && v["value"] != "public123" {
			t.Errorf("public value = %v, want public123", v["value"])
		}
	}
}

func TestWorkerDeleteEnvVar_Success(t *testing.T) {
	env := setupTestEnv(t)
	user, token := env.createTestUser(t, "u@t.com", "password123", "user")

	site := models.Site{UserID: user.ID, SubdomainSlug: "mysite", Name: "Site"}
	env.db.Create(&site)

	envVar := models.WorkerEnvVar{SiteID: site.ID, Name: "KEY", Value: "val"}
	env.db.Create(&envVar)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/sites/"+site.ID+"/worker/env/"+envVar.ID, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := env.doRequest(req)

	if rec.Code != http.StatusNoContent {
		t.Errorf("status = %d, want 204", rec.Code)
	}

	var count int64
	env.db.Model(&models.WorkerEnvVar{}).Where("id = ?", envVar.ID).Count(&count)
	if count != 0 {
		t.Errorf("env var should be deleted")
	}
}

func TestWorkerDeleteEnvVar_NotFound(t *testing.T) {
	env := setupTestEnv(t)
	user, token := env.createTestUser(t, "u@t.com", "password123", "user")

	site := models.Site{UserID: user.ID, SubdomainSlug: "mysite", Name: "Site"}
	env.db.Create(&site)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/sites/"+site.ID+"/worker/env/nonexistent", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := env.doRequest(req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}

func TestWorkerCreateKVNamespace_Success(t *testing.T) {
	env := setupTestEnv(t)
	user, token := env.createTestUser(t, "u@t.com", "password123", "user")

	site := models.Site{UserID: user.ID, SubdomainSlug: "mysite", Name: "Site"}
	env.db.Create(&site)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/sites/"+site.ID+"/worker/kv",
		jsonBody(map[string]string{"name": "CACHE"}))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	rec := env.doRequest(req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201: %s", rec.Code, rec.Body.String())
	}

	body := parseJSON(t, rec)
	if body["name"] != "CACHE" {
		t.Errorf("name = %v, want CACHE", body["name"])
	}
}

func TestWorkerCreateKVNamespace_MissingName(t *testing.T) {
	env := setupTestEnv(t)
	user, token := env.createTestUser(t, "u@t.com", "password123", "user")

	site := models.Site{UserID: user.ID, SubdomainSlug: "mysite", Name: "Site"}
	env.db.Create(&site)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/sites/"+site.ID+"/worker/kv",
		jsonBody(map[string]string{}))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	rec := env.doRequest(req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

func TestWorkerListKVNamespaces_Success(t *testing.T) {
	env := setupTestEnv(t)
	user, token := env.createTestUser(t, "u@t.com", "password123", "user")

	site := models.Site{UserID: user.ID, SubdomainSlug: "mysite", Name: "Site"}
	env.db.Create(&site)
	env.db.Create(&models.KVNamespace{SiteID: site.ID, Name: "NS1"})
	env.db.Create(&models.KVNamespace{SiteID: site.ID, Name: "NS2"})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/sites/"+site.ID+"/worker/kv", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := env.doRequest(req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}

	var namespaces []map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &namespaces); err != nil {
		t.Fatalf("parse namespaces: %v", err)
	}

	if len(namespaces) != 2 {
		t.Errorf("got %d namespaces, want 2", len(namespaces))
	}
}

func TestWorkerDeleteKVNamespace_Success(t *testing.T) {
	env := setupTestEnv(t)
	user, token := env.createTestUser(t, "u@t.com", "password123", "user")

	site := models.Site{UserID: user.ID, SubdomainSlug: "mysite", Name: "Site"}
	env.db.Create(&site)

	ns := models.KVNamespace{SiteID: site.ID, Name: "NS"}
	env.db.Create(&ns)

	// Create KV entries
	env.db.Create(&models.KVEntry{NamespaceID: ns.ID, Key: "key1", Value: "val1"})
	env.db.Create(&models.KVEntry{NamespaceID: ns.ID, Key: "key2", Value: "val2"})

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/sites/"+site.ID+"/worker/kv/"+ns.ID, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := env.doRequest(req)

	if rec.Code != http.StatusNoContent {
		t.Errorf("status = %d, want 204", rec.Code)
	}

	// Verify namespace and entries are deleted
	var nsCount, entryCount int64
	env.db.Model(&models.KVNamespace{}).Where("id = ?", ns.ID).Count(&nsCount)
	env.db.Model(&models.KVEntry{}).Where("namespace_id = ?", ns.ID).Count(&entryCount)

	if nsCount != 0 {
		t.Errorf("namespace should be deleted")
	}
	if entryCount != 0 {
		t.Errorf("namespace entries should be deleted")
	}
}

func TestWorkerCreateCronSchedule_Success(t *testing.T) {
	env := setupTestEnv(t)
	user, token := env.createTestUser(t, "u@t.com", "password123", "user")

	site := models.Site{UserID: user.ID, SubdomainSlug: "mysite", Name: "Site"}
	env.db.Create(&site)

	enabled := true
	req := httptest.NewRequest(http.MethodPost, "/api/v1/sites/"+site.ID+"/worker/crons",
		jsonBody(map[string]interface{}{"cron": "0 0 * * *", "enabled": enabled}))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	rec := env.doRequest(req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201: %s", rec.Code, rec.Body.String())
	}

	body := parseJSON(t, rec)
	if body["cron"] != "0 0 * * *" {
		t.Errorf("cron = %v, want 0 0 * * *", body["cron"])
	}
}

func TestWorkerCreateCronSchedule_InvalidCron(t *testing.T) {
	env := setupTestEnv(t)
	user, token := env.createTestUser(t, "u@t.com", "password123", "user")

	site := models.Site{UserID: user.ID, SubdomainSlug: "mysite", Name: "Site"}
	env.db.Create(&site)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/sites/"+site.ID+"/worker/crons",
		jsonBody(map[string]string{"cron": "invalid cron"}))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	rec := env.doRequest(req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

func TestWorkerCreateCronSchedule_MissingCron(t *testing.T) {
	env := setupTestEnv(t)
	user, token := env.createTestUser(t, "u@t.com", "password123", "user")

	site := models.Site{UserID: user.ID, SubdomainSlug: "mysite", Name: "Site"}
	env.db.Create(&site)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/sites/"+site.ID+"/worker/crons",
		jsonBody(map[string]string{}))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	rec := env.doRequest(req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

func TestWorkerListCronSchedules_Success(t *testing.T) {
	env := setupTestEnv(t)
	user, token := env.createTestUser(t, "u@t.com", "password123", "user")

	site := models.Site{UserID: user.ID, SubdomainSlug: "mysite", Name: "Site"}
	env.db.Create(&site)
	env.db.Create(&models.CronSchedule{SiteID: site.ID, Cron: "0 0 * * *", Enabled: true})
	env.db.Create(&models.CronSchedule{SiteID: site.ID, Cron: "0 12 * * *", Enabled: false})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/sites/"+site.ID+"/worker/crons", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := env.doRequest(req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}

	var schedules []map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &schedules); err != nil {
		t.Fatalf("parse schedules: %v", err)
	}

	if len(schedules) != 2 {
		t.Errorf("got %d schedules, want 2", len(schedules))
	}
}

func TestWorkerDeleteCronSchedule_Success(t *testing.T) {
	env := setupTestEnv(t)
	user, token := env.createTestUser(t, "u@t.com", "password123", "user")

	site := models.Site{UserID: user.ID, SubdomainSlug: "mysite", Name: "Site"}
	env.db.Create(&site)

	cron := models.CronSchedule{SiteID: site.ID, Cron: "0 0 * * *"}
	env.db.Create(&cron)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/sites/"+site.ID+"/worker/crons/"+cron.ID, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := env.doRequest(req)

	if rec.Code != http.StatusNoContent {
		t.Errorf("status = %d, want 204", rec.Code)
	}

	var count int64
	env.db.Model(&models.CronSchedule{}).Where("id = ?", cron.ID).Count(&count)
	if count != 0 {
		t.Errorf("cron schedule should be deleted")
	}
}

func TestWorkerGetLogs_Success(t *testing.T) {
	env := setupTestEnv(t)
	user, token := env.createTestUser(t, "u@t.com", "password123", "user")

	site := models.Site{UserID: user.ID, SubdomainSlug: "mysite", Name: "Site"}
	env.db.Create(&site)

	env.db.Create(&models.WorkerLog{SiteID: site.ID, Level: "info", Message: "Log 1"})
	env.db.Create(&models.WorkerLog{SiteID: site.ID, Level: "error", Message: "Log 2"})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/sites/"+site.ID+"/worker/logs", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := env.doRequest(req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}

	var logs []map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &logs); err != nil {
		t.Fatalf("parse logs: %v", err)
	}

	if len(logs) != 2 {
		t.Errorf("got %d logs, want 2", len(logs))
	}
}

func TestWorkerGetLogs_Forbidden(t *testing.T) {
	env := setupTestEnv(t)
	user, _ := env.createTestUser(t, "u@t.com", "password123", "user")
	_, otherToken := env.createTestUser(t, "other@t.com", "password123", "user")

	site := models.Site{UserID: user.ID, SubdomainSlug: "mysite", Name: "Site"}
	env.db.Create(&site)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/sites/"+site.ID+"/worker/logs", nil)
	req.Header.Set("Authorization", "Bearer "+otherToken)
	rec := env.doRequest(req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", rec.Code)
	}
}
