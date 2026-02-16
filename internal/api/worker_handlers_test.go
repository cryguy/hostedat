package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/cryguy/hostedat/internal/models"
)

// ---------------------------------------------------------------------------
// Worker Env Var tests
// ---------------------------------------------------------------------------

func workerTestSetup(t *testing.T) (*testEnv, string, string) {
	t.Helper()
	env := setupTestEnv(t)
	_, token := env.createTestUser(t, "worker@test.com", "password123", "user")

	// Create a site.
	siteReq := httptest.NewRequest(http.MethodPost, "/api/v1/sites",
		jsonBody(map[string]string{"name": "worker-test", "subdomain_slug": "worker-test"}))
	siteReq.Header.Set("Content-Type", "application/json")
	siteReq.Header.Set("Authorization", "Bearer "+token)
	siteRec := env.doRequest(siteReq)
	if siteRec.Code != http.StatusCreated {
		t.Fatalf("create site: status = %d: %s", siteRec.Code, siteRec.Body.String())
	}
	siteBody := parseJSON(t, siteRec)
	siteID := siteBody["id"].(string)

	return env, token, siteID
}

func TestWorkerEnvVar_SetAndList(t *testing.T) {
	env, token, siteID := workerTestSetup(t)

	// Set an env var.
	setReq := httptest.NewRequest(http.MethodPost, "/api/v1/sites/"+siteID+"/worker/env",
		jsonBody(map[string]interface{}{"name": "API_URL", "value": "https://api.example.com", "secret": false}))
	setReq.Header.Set("Content-Type", "application/json")
	setReq.Header.Set("Authorization", "Bearer "+token)
	setRec := env.doRequest(setReq)

	if setRec.Code != http.StatusOK {
		t.Fatalf("set env var: status = %d: %s", setRec.Code, setRec.Body.String())
	}

	// List env vars.
	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/sites/"+siteID+"/worker/env", nil)
	listReq.Header.Set("Authorization", "Bearer "+token)
	listRec := env.doRequest(listReq)

	if listRec.Code != http.StatusOK {
		t.Fatalf("list env vars: status = %d: %s", listRec.Code, listRec.Body.String())
	}

	var vars []map[string]interface{}
	json.Unmarshal(listRec.Body.Bytes(), &vars)
	if len(vars) != 1 {
		t.Fatalf("var count = %d, want 1", len(vars))
	}
	if vars[0]["name"] != "API_URL" {
		t.Errorf("name = %v", vars[0]["name"])
	}
	if vars[0]["value"] != "https://api.example.com" {
		t.Errorf("value = %v", vars[0]["value"])
	}
}

func TestWorkerEnvVar_SecretMasked(t *testing.T) {
	env, token, siteID := workerTestSetup(t)

	// Set a secret.
	setReq := httptest.NewRequest(http.MethodPost, "/api/v1/sites/"+siteID+"/worker/env",
		jsonBody(map[string]interface{}{"name": "SECRET_KEY", "value": "super-secret-123", "secret": true}))
	setReq.Header.Set("Content-Type", "application/json")
	setReq.Header.Set("Authorization", "Bearer "+token)
	env.doRequest(setReq)

	// List should mask secret.
	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/sites/"+siteID+"/worker/env", nil)
	listReq.Header.Set("Authorization", "Bearer "+token)
	listRec := env.doRequest(listReq)

	var vars []map[string]interface{}
	json.Unmarshal(listRec.Body.Bytes(), &vars)
	if len(vars) != 1 {
		t.Fatalf("var count = %d, want 1", len(vars))
	}
	if vars[0]["value"] != "********" {
		t.Errorf("secret value = %v, should be masked", vars[0]["value"])
	}
}

func TestWorkerEnvVar_Upsert(t *testing.T) {
	env, token, siteID := workerTestSetup(t)

	// Set initial value.
	setReq := httptest.NewRequest(http.MethodPost, "/api/v1/sites/"+siteID+"/worker/env",
		jsonBody(map[string]interface{}{"name": "MY_VAR", "value": "v1", "secret": false}))
	setReq.Header.Set("Content-Type", "application/json")
	setReq.Header.Set("Authorization", "Bearer "+token)
	env.doRequest(setReq)

	// Update the same var.
	setReq2 := httptest.NewRequest(http.MethodPost, "/api/v1/sites/"+siteID+"/worker/env",
		jsonBody(map[string]interface{}{"name": "MY_VAR", "value": "v2", "secret": false}))
	setReq2.Header.Set("Content-Type", "application/json")
	setReq2.Header.Set("Authorization", "Bearer "+token)
	env.doRequest(setReq2)

	// Should have only one var with updated value.
	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/sites/"+siteID+"/worker/env", nil)
	listReq.Header.Set("Authorization", "Bearer "+token)
	listRec := env.doRequest(listReq)

	var vars []map[string]interface{}
	json.Unmarshal(listRec.Body.Bytes(), &vars)
	if len(vars) != 1 {
		t.Fatalf("var count = %d, want 1 (upsert)", len(vars))
	}
	if vars[0]["value"] != "v2" {
		t.Errorf("value = %v, want v2", vars[0]["value"])
	}
}

func TestWorkerEnvVar_Delete(t *testing.T) {
	env, token, siteID := workerTestSetup(t)

	// Set a var.
	setReq := httptest.NewRequest(http.MethodPost, "/api/v1/sites/"+siteID+"/worker/env",
		jsonBody(map[string]interface{}{"name": "TO_DELETE", "value": "bye", "secret": false}))
	setReq.Header.Set("Content-Type", "application/json")
	setReq.Header.Set("Authorization", "Bearer "+token)
	setRec := env.doRequest(setReq)

	var envVar map[string]interface{}
	json.Unmarshal(setRec.Body.Bytes(), &envVar)
	varID := envVar["id"].(string)

	// Delete it.
	delReq := httptest.NewRequest(http.MethodDelete, "/api/v1/sites/"+siteID+"/worker/env/"+varID, nil)
	delReq.Header.Set("Authorization", "Bearer "+token)
	delRec := env.doRequest(delReq)

	if delRec.Code != http.StatusNoContent {
		t.Fatalf("delete env var: status = %d", delRec.Code)
	}

	// Verify gone.
	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/sites/"+siteID+"/worker/env", nil)
	listReq.Header.Set("Authorization", "Bearer "+token)
	listRec := env.doRequest(listReq)
	var vars []map[string]interface{}
	json.Unmarshal(listRec.Body.Bytes(), &vars)
	if len(vars) != 0 {
		t.Errorf("var count after delete = %d, want 0", len(vars))
	}
}

func TestWorkerEnvVar_EmptyName(t *testing.T) {
	env, token, siteID := workerTestSetup(t)

	setReq := httptest.NewRequest(http.MethodPost, "/api/v1/sites/"+siteID+"/worker/env",
		jsonBody(map[string]interface{}{"name": "", "value": "val"}))
	setReq.Header.Set("Content-Type", "application/json")
	setReq.Header.Set("Authorization", "Bearer "+token)
	setRec := env.doRequest(setReq)

	if setRec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", setRec.Code)
	}
}

func TestWorkerEnvVar_AccessDenied(t *testing.T) {
	env, token, siteID := workerTestSetup(t)
	_ = token
	_, otherToken := env.createTestUser(t, "other@test.com", "password123", "user")

	setReq := httptest.NewRequest(http.MethodPost, "/api/v1/sites/"+siteID+"/worker/env",
		jsonBody(map[string]interface{}{"name": "NOPE", "value": "val"}))
	setReq.Header.Set("Content-Type", "application/json")
	setReq.Header.Set("Authorization", "Bearer "+otherToken)
	setRec := env.doRequest(setReq)

	if setRec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", setRec.Code)
	}
}

// ---------------------------------------------------------------------------
// KV Namespace tests
// ---------------------------------------------------------------------------

func TestWorkerKV_CreateAndList(t *testing.T) {
	env, token, siteID := workerTestSetup(t)

	// Create namespace.
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/sites/"+siteID+"/worker/kv",
		jsonBody(map[string]string{"name": "CACHE"}))
	createReq.Header.Set("Content-Type", "application/json")
	createReq.Header.Set("Authorization", "Bearer "+token)
	createRec := env.doRequest(createReq)

	if createRec.Code != http.StatusCreated {
		t.Fatalf("create KV: status = %d: %s", createRec.Code, createRec.Body.String())
	}

	// List namespaces.
	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/sites/"+siteID+"/worker/kv", nil)
	listReq.Header.Set("Authorization", "Bearer "+token)
	listRec := env.doRequest(listReq)

	if listRec.Code != http.StatusOK {
		t.Fatalf("list KV: status = %d", listRec.Code)
	}

	var namespaces []map[string]interface{}
	json.Unmarshal(listRec.Body.Bytes(), &namespaces)
	if len(namespaces) != 1 {
		t.Fatalf("namespace count = %d, want 1", len(namespaces))
	}
	if namespaces[0]["name"] != "CACHE" {
		t.Errorf("name = %v", namespaces[0]["name"])
	}
}

func TestWorkerKV_Delete(t *testing.T) {
	env, token, siteID := workerTestSetup(t)

	// Create namespace.
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/sites/"+siteID+"/worker/kv",
		jsonBody(map[string]string{"name": "TO_DELETE"}))
	createReq.Header.Set("Content-Type", "application/json")
	createReq.Header.Set("Authorization", "Bearer "+token)
	createRec := env.doRequest(createReq)

	var ns map[string]interface{}
	json.Unmarshal(createRec.Body.Bytes(), &ns)
	nsID := ns["id"].(string)

	// Also add a KV entry to verify cascade.
	env.db.Create(&models.KVEntry{NamespaceID: nsID, Key: "k", Value: "v"})

	// Delete namespace.
	delReq := httptest.NewRequest(http.MethodDelete, "/api/v1/sites/"+siteID+"/worker/kv/"+nsID, nil)
	delReq.Header.Set("Authorization", "Bearer "+token)
	delRec := env.doRequest(delReq)

	if delRec.Code != http.StatusNoContent {
		t.Fatalf("delete KV: status = %d: %s", delRec.Code, delRec.Body.String())
	}

	// Verify namespace gone.
	var count int64
	env.db.Model(&models.KVNamespace{}).Where("id = ?", nsID).Count(&count)
	if count != 0 {
		t.Error("namespace should be deleted")
	}

	// Verify entries cascaded.
	env.db.Model(&models.KVEntry{}).Where("namespace_id = ?", nsID).Count(&count)
	if count != 0 {
		t.Error("KV entries should be deleted with namespace")
	}
}

func TestWorkerKV_EmptyName(t *testing.T) {
	env, token, siteID := workerTestSetup(t)

	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/sites/"+siteID+"/worker/kv",
		jsonBody(map[string]string{"name": ""}))
	createReq.Header.Set("Content-Type", "application/json")
	createReq.Header.Set("Authorization", "Bearer "+token)
	createRec := env.doRequest(createReq)

	if createRec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", createRec.Code)
	}
}

func TestWorkerKV_AccessDenied(t *testing.T) {
	env, token, siteID := workerTestSetup(t)
	_ = token
	_, otherToken := env.createTestUser(t, "attacker@test.com", "password123", "user")

	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/sites/"+siteID+"/worker/kv",
		jsonBody(map[string]string{"name": "STOLEN"}))
	createReq.Header.Set("Content-Type", "application/json")
	createReq.Header.Set("Authorization", "Bearer "+otherToken)
	createRec := env.doRequest(createReq)

	if createRec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", createRec.Code)
	}
}

// ---------------------------------------------------------------------------
// Cron Schedule tests
// ---------------------------------------------------------------------------

func TestWorkerCron_CreateAndList(t *testing.T) {
	env, token, siteID := workerTestSetup(t)

	// Create schedule.
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/sites/"+siteID+"/worker/crons",
		jsonBody(map[string]interface{}{"cron": "*/5 * * * *"}))
	createReq.Header.Set("Content-Type", "application/json")
	createReq.Header.Set("Authorization", "Bearer "+token)
	createRec := env.doRequest(createReq)

	if createRec.Code != http.StatusCreated {
		t.Fatalf("create cron: status = %d: %s", createRec.Code, createRec.Body.String())
	}

	createBody := parseJSON(t, createRec)
	if createBody["cron"] != "*/5 * * * *" {
		t.Errorf("cron = %v", createBody["cron"])
	}
	if createBody["enabled"] != true {
		t.Errorf("enabled = %v, want true (default)", createBody["enabled"])
	}

	// List schedules.
	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/sites/"+siteID+"/worker/crons", nil)
	listReq.Header.Set("Authorization", "Bearer "+token)
	listRec := env.doRequest(listReq)

	if listRec.Code != http.StatusOK {
		t.Fatalf("list crons: status = %d", listRec.Code)
	}

	var schedules []map[string]interface{}
	json.Unmarshal(listRec.Body.Bytes(), &schedules)
	if len(schedules) != 1 {
		t.Fatalf("schedule count = %d, want 1", len(schedules))
	}
}

func TestWorkerCron_CreateDisabled(t *testing.T) {
	env, token, siteID := workerTestSetup(t)

	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/sites/"+siteID+"/worker/crons",
		jsonBody(map[string]interface{}{"cron": "0 * * * *", "enabled": false}))
	createReq.Header.Set("Content-Type", "application/json")
	createReq.Header.Set("Authorization", "Bearer "+token)
	createRec := env.doRequest(createReq)

	if createRec.Code != http.StatusCreated {
		t.Fatalf("status = %d: %s", createRec.Code, createRec.Body.String())
	}
	body := parseJSON(t, createRec)
	if body["enabled"] != false {
		t.Errorf("enabled = %v, want false", body["enabled"])
	}
}

func TestWorkerCron_InvalidExpression(t *testing.T) {
	env, token, siteID := workerTestSetup(t)

	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/sites/"+siteID+"/worker/crons",
		jsonBody(map[string]interface{}{"cron": "invalid cron"}))
	createReq.Header.Set("Content-Type", "application/json")
	createReq.Header.Set("Authorization", "Bearer "+token)
	createRec := env.doRequest(createReq)

	if createRec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", createRec.Code)
	}
}

func TestWorkerCron_EmptyCron(t *testing.T) {
	env, token, siteID := workerTestSetup(t)

	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/sites/"+siteID+"/worker/crons",
		jsonBody(map[string]interface{}{"cron": ""}))
	createReq.Header.Set("Content-Type", "application/json")
	createReq.Header.Set("Authorization", "Bearer "+token)
	createRec := env.doRequest(createReq)

	if createRec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", createRec.Code)
	}
}

func TestWorkerCron_Delete(t *testing.T) {
	env, token, siteID := workerTestSetup(t)

	// Create schedule.
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/sites/"+siteID+"/worker/crons",
		jsonBody(map[string]interface{}{"cron": "0 0 * * *"}))
	createReq.Header.Set("Content-Type", "application/json")
	createReq.Header.Set("Authorization", "Bearer "+token)
	createRec := env.doRequest(createReq)
	body := parseJSON(t, createRec)
	cronID := body["id"].(string)

	// Delete.
	delReq := httptest.NewRequest(http.MethodDelete, "/api/v1/sites/"+siteID+"/worker/crons/"+cronID, nil)
	delReq.Header.Set("Authorization", "Bearer "+token)
	delRec := env.doRequest(delReq)

	if delRec.Code != http.StatusNoContent {
		t.Fatalf("delete cron: status = %d", delRec.Code)
	}

	// Verify gone.
	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/sites/"+siteID+"/worker/crons", nil)
	listReq.Header.Set("Authorization", "Bearer "+token)
	listRec := env.doRequest(listReq)
	var schedules []map[string]interface{}
	json.Unmarshal(listRec.Body.Bytes(), &schedules)
	if len(schedules) != 0 {
		t.Errorf("schedule count after delete = %d, want 0", len(schedules))
	}
}

func TestWorkerCron_AccessDenied(t *testing.T) {
	env, token, siteID := workerTestSetup(t)
	_ = token
	_, otherToken := env.createTestUser(t, "hacker@test.com", "password123", "user")

	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/sites/"+siteID+"/worker/crons",
		jsonBody(map[string]interface{}{"cron": "* * * * *"}))
	createReq.Header.Set("Content-Type", "application/json")
	createReq.Header.Set("Authorization", "Bearer "+otherToken)
	createRec := env.doRequest(createReq)

	if createRec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", createRec.Code)
	}
}

// ---------------------------------------------------------------------------
// Worker Logs tests
// ---------------------------------------------------------------------------

func TestWorkerLogs_GetEmpty(t *testing.T) {
	env, token, siteID := workerTestSetup(t)

	logsReq := httptest.NewRequest(http.MethodGet, "/api/v1/sites/"+siteID+"/worker/logs", nil)
	logsReq.Header.Set("Authorization", "Bearer "+token)
	logsRec := env.doRequest(logsReq)

	if logsRec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", logsRec.Code, logsRec.Body.String())
	}

	var logs []map[string]interface{}
	json.Unmarshal(logsRec.Body.Bytes(), &logs)
	if len(logs) != 0 {
		t.Errorf("log count = %d, want 0", len(logs))
	}
}

func TestWorkerLogs_GetWithEntries(t *testing.T) {
	env, token, siteID := workerTestSetup(t)

	// Insert some logs directly.
	env.db.Create(&models.WorkerLog{SiteID: siteID, Level: "log", Message: "first"})
	env.db.Create(&models.WorkerLog{SiteID: siteID, Level: "error", Message: "second"})

	logsReq := httptest.NewRequest(http.MethodGet, "/api/v1/sites/"+siteID+"/worker/logs", nil)
	logsReq.Header.Set("Authorization", "Bearer "+token)
	logsRec := env.doRequest(logsReq)

	if logsRec.Code != http.StatusOK {
		t.Fatalf("status = %d", logsRec.Code)
	}

	var logs []map[string]interface{}
	json.Unmarshal(logsRec.Body.Bytes(), &logs)
	if len(logs) != 2 {
		t.Errorf("log count = %d, want 2", len(logs))
	}
}

func TestWorkerLogs_AccessDenied(t *testing.T) {
	env, token, siteID := workerTestSetup(t)
	_ = token
	_, otherToken := env.createTestUser(t, "spy@test.com", "password123", "user")

	logsReq := httptest.NewRequest(http.MethodGet, "/api/v1/sites/"+siteID+"/worker/logs", nil)
	logsReq.Header.Set("Authorization", "Bearer "+otherToken)
	logsRec := env.doRequest(logsReq)

	if logsRec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", logsRec.Code)
	}
}

// ---------------------------------------------------------------------------
// Non-existent site tests
// ---------------------------------------------------------------------------

func TestWorker_SiteNotFound(t *testing.T) {
	env := setupTestEnv(t)
	_, token := env.createTestUser(t, "u@test.com", "password123", "user")

	endpoints := []struct {
		method string
		path   string
		body   interface{}
	}{
		{http.MethodPost, "/api/v1/sites/nonexistent/worker/env", map[string]interface{}{"name": "X", "value": "Y"}},
		{http.MethodGet, "/api/v1/sites/nonexistent/worker/env", nil},
		{http.MethodPost, "/api/v1/sites/nonexistent/worker/kv", map[string]string{"name": "NS"}},
		{http.MethodGet, "/api/v1/sites/nonexistent/worker/kv", nil},
		{http.MethodPost, "/api/v1/sites/nonexistent/worker/crons", map[string]interface{}{"cron": "* * * * *"}},
		{http.MethodGet, "/api/v1/sites/nonexistent/worker/crons", nil},
		{http.MethodGet, "/api/v1/sites/nonexistent/worker/logs", nil},
	}

	for _, ep := range endpoints {
		t.Run(ep.method+" "+ep.path, func(t *testing.T) {
			var req *http.Request
			if ep.body != nil {
				req = httptest.NewRequest(ep.method, ep.path, jsonBody(ep.body))
				req.Header.Set("Content-Type", "application/json")
			} else {
				req = httptest.NewRequest(ep.method, ep.path, nil)
			}
			req.Header.Set("Authorization", "Bearer "+token)
			rec := env.doRequest(req)
			if rec.Code != http.StatusNotFound {
				t.Errorf("status = %d, want 404", rec.Code)
			}
		})
	}
}
