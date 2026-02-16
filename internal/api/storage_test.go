package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// ──────────────────────────────────────────────
// Storage Credential Management Tests
// ──────────────────────────────────────────────

func TestStorageCredentials_Create(t *testing.T) {
	env := setupTestEnv(t)
	_, token := env.createTestUser(t, "storage@test.com", "password123", "user")

	// Create a site first.
	siteReq := httptest.NewRequest(http.MethodPost, "/api/v1/sites",
		jsonBody(map[string]string{"name": "storage-test", "subdomain": "storage-test"}))
	siteReq.Header.Set("Content-Type", "application/json")
	siteReq.Header.Set("Authorization", "Bearer "+token)
	siteRec := env.doRequest(siteReq)

	if siteRec.Code != http.StatusCreated {
		t.Fatalf("create site status = %d: %s", siteRec.Code, siteRec.Body.String())
	}
	siteBody := parseJSON(t, siteRec)
	siteID := siteBody["id"].(string)

	// Create storage credentials.
	credReq := httptest.NewRequest(http.MethodPost, "/api/v1/sites/"+siteID+"/storage/credentials", nil)
	credReq.Header.Set("Authorization", "Bearer "+token)
	credRec := env.doRequest(credReq)

	if credRec.Code != http.StatusCreated {
		t.Fatalf("create credentials status = %d: %s", credRec.Code, credRec.Body.String())
	}

	var cred map[string]interface{}
	json.Unmarshal(credRec.Body.Bytes(), &cred)

	if cred["access_key_id"] == nil || cred["access_key_id"] == "" {
		t.Error("expected access_key_id")
	}
	if cred["secret_access_key"] == nil || cred["secret_access_key"] == "" {
		t.Error("expected secret_access_key (only returned on creation)")
	}
	if cred["site_id"] != siteID {
		t.Errorf("site_id = %q, want %q", cred["site_id"], siteID)
	}
}

func TestStorageCredentials_List(t *testing.T) {
	env := setupTestEnv(t)
	_, token := env.createTestUser(t, "list@test.com", "password123", "user")

	siteReq := httptest.NewRequest(http.MethodPost, "/api/v1/sites",
		jsonBody(map[string]string{"name": "list-test", "subdomain": "list-test"}))
	siteReq.Header.Set("Content-Type", "application/json")
	siteReq.Header.Set("Authorization", "Bearer "+token)
	siteRec := env.doRequest(siteReq)
	siteBody := parseJSON(t, siteRec)
	siteID := siteBody["id"].(string)

	// Create two credentials.
	for i := 0; i < 2; i++ {
		credReq := httptest.NewRequest(http.MethodPost, "/api/v1/sites/"+siteID+"/storage/credentials", nil)
		credReq.Header.Set("Authorization", "Bearer "+token)
		env.doRequest(credReq)
	}

	// List credentials.
	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/sites/"+siteID+"/storage/credentials", nil)
	listReq.Header.Set("Authorization", "Bearer "+token)
	listRec := env.doRequest(listReq)

	if listRec.Code != http.StatusOK {
		t.Fatalf("list status = %d: %s", listRec.Code, listRec.Body.String())
	}

	var creds []map[string]interface{}
	json.Unmarshal(listRec.Body.Bytes(), &creds)

	if len(creds) != 2 {
		t.Errorf("credential count = %d, want 2", len(creds))
	}

	// List should NOT include secret_access_key.
	for _, c := range creds {
		if _, ok := c["secret_access_key"]; ok {
			t.Error("list should not include secret_access_key")
		}
	}
}

func TestStorageCredentials_Delete(t *testing.T) {
	env := setupTestEnv(t)
	_, token := env.createTestUser(t, "delete@test.com", "password123", "user")

	siteReq := httptest.NewRequest(http.MethodPost, "/api/v1/sites",
		jsonBody(map[string]string{"name": "del-test", "subdomain": "del-test"}))
	siteReq.Header.Set("Content-Type", "application/json")
	siteReq.Header.Set("Authorization", "Bearer "+token)
	siteRec := env.doRequest(siteReq)
	siteBody := parseJSON(t, siteRec)
	siteID := siteBody["id"].(string)

	// Create credential.
	credReq := httptest.NewRequest(http.MethodPost, "/api/v1/sites/"+siteID+"/storage/credentials", nil)
	credReq.Header.Set("Authorization", "Bearer "+token)
	credRec := env.doRequest(credReq)
	var cred map[string]interface{}
	json.Unmarshal(credRec.Body.Bytes(), &cred)
	accessKeyID := cred["access_key_id"].(string)

	// Delete credential.
	delReq := httptest.NewRequest(http.MethodDelete, "/api/v1/sites/"+siteID+"/storage/credentials/"+accessKeyID, nil)
	delReq.Header.Set("Authorization", "Bearer "+token)
	delRec := env.doRequest(delReq)

	if delRec.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d: %s", delRec.Code, delRec.Body.String())
	}

	// Verify it's gone.
	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/sites/"+siteID+"/storage/credentials", nil)
	listReq.Header.Set("Authorization", "Bearer "+token)
	listRec := env.doRequest(listReq)
	var creds []map[string]interface{}
	json.Unmarshal(listRec.Body.Bytes(), &creds)
	if len(creds) != 0 {
		t.Errorf("after delete, credential count = %d, want 0", len(creds))
	}
}

func TestStorageCredentials_AccessDenied(t *testing.T) {
	env := setupTestEnv(t)
	_, token1 := env.createTestUser(t, "owner@test.com", "password123", "user")
	_, token2 := env.createTestUser(t, "other@test.com", "password123", "user")

	// User 1 creates a site.
	siteReq := httptest.NewRequest(http.MethodPost, "/api/v1/sites",
		jsonBody(map[string]string{"name": "acl-test", "subdomain": "acl-test"}))
	siteReq.Header.Set("Content-Type", "application/json")
	siteReq.Header.Set("Authorization", "Bearer "+token1)
	siteRec := env.doRequest(siteReq)
	siteBody := parseJSON(t, siteRec)
	siteID := siteBody["id"].(string)

	// User 2 tries to create credentials.
	credReq := httptest.NewRequest(http.MethodPost, "/api/v1/sites/"+siteID+"/storage/credentials", nil)
	credReq.Header.Set("Authorization", "Bearer "+token2)
	credRec := env.doRequest(credReq)

	if credRec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", credRec.Code)
	}
}

// ──────────────────────────────────────────────
// Storage Usage Tests
// ──────────────────────────────────────────────

func TestStorageUsage_Empty(t *testing.T) {
	env := setupTestEnv(t)
	_, token := env.createTestUser(t, "usage@test.com", "password123", "user")

	siteReq := httptest.NewRequest(http.MethodPost, "/api/v1/sites",
		jsonBody(map[string]string{"name": "usage-test", "subdomain": "usage-test"}))
	siteReq.Header.Set("Content-Type", "application/json")
	siteReq.Header.Set("Authorization", "Bearer "+token)
	siteRec := env.doRequest(siteReq)
	siteBody := parseJSON(t, siteRec)
	siteID := siteBody["id"].(string)

	usageReq := httptest.NewRequest(http.MethodGet, "/api/v1/sites/"+siteID+"/storage/usage", nil)
	usageReq.Header.Set("Authorization", "Bearer "+token)
	usageRec := env.doRequest(usageReq)

	if usageRec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", usageRec.Code, usageRec.Body.String())
	}

	var usage map[string]interface{}
	json.Unmarshal(usageRec.Body.Bytes(), &usage)

	if usage["total_size"].(float64) != 0 {
		t.Errorf("total_size = %v, want 0", usage["total_size"])
	}
	if usage["object_count"].(float64) != 0 {
		t.Errorf("object_count = %v, want 0", usage["object_count"])
	}
}
