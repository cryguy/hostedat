package api

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/cryguy/hostedat/internal/models"
)

func TestDeployDeploy_Success(t *testing.T) {
	env := setupTestEnv(t)
	user, token := env.createTestUser(t, "u@t.com", "password123", "user")

	site := models.Site{UserID: user.ID, SubdomainSlug: "mysite", Name: "Site"}
	env.db.Create(&site)

	// Create a multipart form with a zip file
	zipData := createTestZip(t, map[string]string{"index.html": "<html>Test</html>"})
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, _ := writer.CreateFormFile("file", "deploy.zip")
	io.Copy(part, zipData)
	writer.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/sites/"+site.ID+"/deploy", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+token)
	rec := env.doRequest(req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201: %s", rec.Code, rec.Body.String())
	}

	respBody := parseJSON(t, rec)
	if respBody["version"] != float64(1) {
		t.Errorf("version = %v, want 1", respBody["version"])
	}
	if respBody["has_worker"] != false {
		t.Errorf("has_worker = %v, want false", respBody["has_worker"])
	}

	// Verify site was updated
	var updatedSite models.Site
	env.db.First(&updatedSite, "id = ?", site.ID)
	if updatedSite.ActiveVersion == nil || *updatedSite.ActiveVersion != 1 {
		t.Errorf("site active_version = %v, want 1", updatedSite.ActiveVersion)
	}
}

func TestDeployDeploy_WithWorker(t *testing.T) {
	env := setupTestEnv(t)
	user, token := env.createTestUser(t, "u@t.com", "password123", "user")

	site := models.Site{UserID: user.ID, SubdomainSlug: "mysite", Name: "Site"}
	env.db.Create(&site)

	// Create zip with _worker.js
	zipData := createTestZip(t, map[string]string{
		"index.html": "<html>Test</html>",
		"_worker.js": "export default { async fetch(request) { return new Response('Hello'); } }",
	})
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, _ := writer.CreateFormFile("file", "deploy.zip")
	io.Copy(part, zipData)
	writer.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/sites/"+site.ID+"/deploy", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+token)
	rec := env.doRequest(req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201: %s", rec.Code, rec.Body.String())
	}

	respBody := parseJSON(t, rec)
	if respBody["has_worker"] != true {
		t.Errorf("has_worker = %v, want true", respBody["has_worker"])
	}
}

func TestDeployDeploy_MissingFile(t *testing.T) {
	env := setupTestEnv(t)
	user, token := env.createTestUser(t, "u@t.com", "password123", "user")

	site := models.Site{UserID: user.ID, SubdomainSlug: "mysite", Name: "Site"}
	env.db.Create(&site)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/sites/"+site.ID+"/deploy", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := env.doRequest(req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

func TestDeployDeploy_SiteNotFound(t *testing.T) {
	env := setupTestEnv(t)
	_, token := env.createTestUser(t, "u@t.com", "password123", "user")

	zipData := createTestZip(t, map[string]string{"index.html": "<html>Test</html>"})
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, _ := writer.CreateFormFile("file", "deploy.zip")
	io.Copy(part, zipData)
	writer.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/sites/nonexistent/deploy", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+token)
	rec := env.doRequest(req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}

func TestDeployDeploy_Forbidden(t *testing.T) {
	env := setupTestEnv(t)
	user, _ := env.createTestUser(t, "u@t.com", "password123", "user")
	_, otherToken := env.createTestUser(t, "other@t.com", "password123", "user")

	site := models.Site{UserID: user.ID, SubdomainSlug: "mysite", Name: "Site"}
	env.db.Create(&site)

	zipData := createTestZip(t, map[string]string{"index.html": "<html>Test</html>"})
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, _ := writer.CreateFormFile("file", "deploy.zip")
	io.Copy(part, zipData)
	writer.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/sites/"+site.ID+"/deploy", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+otherToken)
	rec := env.doRequest(req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", rec.Code)
	}
}

func TestDeployDeploy_MultipleVersions(t *testing.T) {
	env := setupTestEnv(t)
	user, token := env.createTestUser(t, "u@t.com", "password123", "user")

	site := models.Site{UserID: user.ID, SubdomainSlug: "mysite", Name: "Site"}
	env.db.Create(&site)

	// First deployment
	zipData := createTestZip(t, map[string]string{"index.html": "<html>V1</html>"})
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, _ := writer.CreateFormFile("file", "deploy.zip")
	io.Copy(part, zipData)
	writer.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/sites/"+site.ID+"/deploy", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+token)
	rec := env.doRequest(req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("first deploy: status = %d, want 201: %s", rec.Code, rec.Body.String())
	}

	// Second deployment
	zipData2 := createTestZip(t, map[string]string{"index.html": "<html>V2</html>"})
	body2 := &bytes.Buffer{}
	writer2 := multipart.NewWriter(body2)
	part2, _ := writer2.CreateFormFile("file", "deploy.zip")
	io.Copy(part2, zipData2)
	writer2.Close()

	req2 := httptest.NewRequest(http.MethodPost, "/api/v1/sites/"+site.ID+"/deploy", body2)
	req2.Header.Set("Content-Type", writer2.FormDataContentType())
	req2.Header.Set("Authorization", "Bearer "+token)
	rec2 := env.doRequest(req2)

	if rec2.Code != http.StatusCreated {
		t.Fatalf("second deploy: status = %d, want 201: %s", rec2.Code, rec2.Body.String())
	}

	respBody := parseJSON(t, rec2)
	if respBody["version"] != float64(2) {
		t.Errorf("version = %v, want 2", respBody["version"])
	}

	// Verify both deployments exist
	var count int64
	env.db.Model(&models.Deployment{}).Where("site_id = ?", site.ID).Count(&count)
	if count != 2 {
		t.Errorf("deployment count = %d, want 2", count)
	}
}

func TestDeployRollback_Success(t *testing.T) {
	env := setupTestEnv(t)
	user, token := env.createTestUser(t, "u@t.com", "password123", "user")

	site := models.Site{UserID: user.ID, SubdomainSlug: "mysite", Name: "Site"}
	env.db.Create(&site)

	// Create deployments
	dep1 := models.Deployment{SiteID: site.ID, Version: 1, FileHash: "hash1"}
	env.db.Create(&dep1)
	dep2 := models.Deployment{SiteID: site.ID, Version: 2, FileHash: "hash2"}
	env.db.Create(&dep2)

	// Set active to version 2
	version2 := 2
	env.db.Model(&site).Updates(map[string]interface{}{
		"active_version":   version2,
		"active_deploy_id": dep2.ID,
	})

	// Rollback to version 1
	req := httptest.NewRequest(http.MethodPost, "/api/v1/sites/"+site.ID+"/deployments/1/rollback", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := env.doRequest(req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}

	respBody := parseJSON(t, rec)
	if respBody["active_version"] != float64(1) {
		t.Errorf("active_version = %v, want 1", respBody["active_version"])
	}

	// Verify site was updated
	var updatedSite models.Site
	env.db.First(&updatedSite, "id = ?", site.ID)
	if updatedSite.ActiveVersion == nil || *updatedSite.ActiveVersion != 1 {
		t.Errorf("site active_version = %v, want 1", updatedSite.ActiveVersion)
	}
}

func TestDeployRollback_AlreadyActive(t *testing.T) {
	env := setupTestEnv(t)
	user, token := env.createTestUser(t, "u@t.com", "password123", "user")

	site := models.Site{UserID: user.ID, SubdomainSlug: "mysite", Name: "Site"}
	env.db.Create(&site)

	dep := models.Deployment{SiteID: site.ID, Version: 1, FileHash: "hash1"}
	env.db.Create(&dep)

	version1 := 1
	env.db.Model(&site).Updates(map[string]interface{}{
		"active_version":   version1,
		"active_deploy_id": dep.ID,
	})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/sites/"+site.ID+"/deployments/1/rollback", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := env.doRequest(req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

func TestDeployRollback_InvalidVersion(t *testing.T) {
	env := setupTestEnv(t)
	user, token := env.createTestUser(t, "u@t.com", "password123", "user")

	site := models.Site{UserID: user.ID, SubdomainSlug: "mysite", Name: "Site"}
	env.db.Create(&site)

	tests := []string{
		"/api/v1/sites/" + site.ID + "/deployments/0/rollback",
		"/api/v1/sites/" + site.ID + "/deployments/-1/rollback",
		"/api/v1/sites/" + site.ID + "/deployments/abc/rollback",
	}

	for _, path := range tests {
		req := httptest.NewRequest(http.MethodPost, path, nil)
		req.Header.Set("Authorization", "Bearer "+token)
		rec := env.doRequest(req)

		if rec.Code != http.StatusBadRequest {
			t.Errorf("path %s: status = %d, want 400", path, rec.Code)
		}
	}
}

func TestDeployRollback_DeploymentNotFound(t *testing.T) {
	env := setupTestEnv(t)
	user, token := env.createTestUser(t, "u@t.com", "password123", "user")

	site := models.Site{UserID: user.ID, SubdomainSlug: "mysite", Name: "Site"}
	env.db.Create(&site)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/sites/"+site.ID+"/deployments/999/rollback", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := env.doRequest(req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}

func TestDeployRollback_SiteNotFound(t *testing.T) {
	env := setupTestEnv(t)
	_, token := env.createTestUser(t, "u@t.com", "password123", "user")

	req := httptest.NewRequest(http.MethodPost, "/api/v1/sites/nonexistent/deployments/1/rollback", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := env.doRequest(req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}

func TestDeployRollback_Forbidden(t *testing.T) {
	env := setupTestEnv(t)
	user, _ := env.createTestUser(t, "u@t.com", "password123", "user")
	_, otherToken := env.createTestUser(t, "other@t.com", "password123", "user")

	site := models.Site{UserID: user.ID, SubdomainSlug: "mysite", Name: "Site"}
	env.db.Create(&site)

	dep := models.Deployment{SiteID: site.ID, Version: 1, FileHash: "hash1"}
	env.db.Create(&dep)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/sites/"+site.ID+"/deployments/1/rollback", nil)
	req.Header.Set("Authorization", "Bearer "+otherToken)
	rec := env.doRequest(req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", rec.Code)
	}
}

func TestDeployList_Success(t *testing.T) {
	env := setupTestEnv(t)
	user, token := env.createTestUser(t, "u@t.com", "password123", "user")

	site := models.Site{UserID: user.ID, SubdomainSlug: "mysite", Name: "Site"}
	env.db.Create(&site)

	// Create multiple deployments
	for i := 1; i <= 5; i++ {
		env.db.Create(&models.Deployment{SiteID: site.ID, Version: i, FileHash: "hash"})
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/sites/"+site.ID+"/deployments", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := env.doRequest(req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}

	var response map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("parse response: %v", err)
	}

	if response["total"] != float64(5) {
		t.Errorf("total = %v, want 5", response["total"])
	}

	deployments, ok := response["deployments"].([]interface{})
	if !ok {
		t.Fatalf("deployments should be array")
	}

	if len(deployments) != 5 {
		t.Errorf("got %d deployments, want 5", len(deployments))
	}
}

func TestDeployList_Pagination(t *testing.T) {
	env := setupTestEnv(t)
	user, token := env.createTestUser(t, "u@t.com", "password123", "user")

	site := models.Site{UserID: user.ID, SubdomainSlug: "mysite", Name: "Site"}
	env.db.Create(&site)

	// Create 25 deployments
	for i := 1; i <= 25; i++ {
		env.db.Create(&models.Deployment{SiteID: site.ID, Version: i, FileHash: "hash"})
	}

	// Get page 1
	req := httptest.NewRequest(http.MethodGet, "/api/v1/sites/"+site.ID+"/deployments?page=1", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := env.doRequest(req)

	if rec.Code != http.StatusOK {
		t.Fatalf("page 1: status = %d, want 200: %s", rec.Code, rec.Body.String())
	}

	var response map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("parse response: %v", err)
	}

	deployments, _ := response["deployments"].([]interface{})
	if len(deployments) != 20 {
		t.Errorf("page 1: got %d deployments, want 20", len(deployments))
	}

	// Get page 2
	req2 := httptest.NewRequest(http.MethodGet, "/api/v1/sites/"+site.ID+"/deployments?page=2", nil)
	req2.Header.Set("Authorization", "Bearer "+token)
	rec2 := env.doRequest(req2)

	var response2 map[string]interface{}
	json.Unmarshal(rec2.Body.Bytes(), &response2)

	deployments2, _ := response2["deployments"].([]interface{})
	if len(deployments2) != 5 {
		t.Errorf("page 2: got %d deployments, want 5", len(deployments2))
	}
}

func TestDeployList_SiteNotFound(t *testing.T) {
	env := setupTestEnv(t)
	_, token := env.createTestUser(t, "u@t.com", "password123", "user")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/sites/nonexistent/deployments", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := env.doRequest(req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}

func TestDeployList_Forbidden(t *testing.T) {
	env := setupTestEnv(t)
	user, _ := env.createTestUser(t, "u@t.com", "password123", "user")
	_, otherToken := env.createTestUser(t, "other@t.com", "password123", "user")

	site := models.Site{UserID: user.ID, SubdomainSlug: "mysite", Name: "Site"}
	env.db.Create(&site)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/sites/"+site.ID+"/deployments", nil)
	req.Header.Set("Authorization", "Bearer "+otherToken)
	rec := env.doRequest(req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", rec.Code)
	}
}
