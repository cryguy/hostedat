package client

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestClient_ListSites_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if r.URL.Path != "/api/v1/sites" {
			t.Errorf("expected /api/v1/sites, got %s", r.URL.Path)
		}
		w.WriteHeader(200)
		_ = json.NewEncoder(w).Encode([]Site{
			{ID: "site1", Name: "Site One", SubdomainSlug: "site-one", SPAMode: false},
			{ID: "site2", Name: "Site Two", SubdomainSlug: "site-two", SPAMode: true},
		})
	}))
	defer srv.Close()

	c := New(srv.URL, "test-token")
	sites, err := c.ListSites()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sites) != 2 {
		t.Fatalf("expected 2 sites, got %d", len(sites))
	}
	if sites[0].ID != "site1" || sites[0].Name != "Site One" {
		t.Errorf("unexpected first site: %+v", sites[0])
	}
	if sites[1].SPAMode != true {
		t.Error("expected second site to have SPAMode true")
	}
}

func TestClient_ListSites_Empty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		_ = json.NewEncoder(w).Encode([]Site{})
	}))
	defer srv.Close()

	c := New(srv.URL, "test-token")
	sites, err := c.ListSites()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sites) != 0 {
		t.Errorf("expected 0 sites, got %d", len(sites))
	}
}

func TestClient_ListSites_Unauthorized(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(401)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "unauthorized"})
	}))
	defer srv.Close()

	c := New(srv.URL, "invalid-token")
	_, err := c.ListSites()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected *APIError, got %T", err)
	}
	if apiErr.StatusCode != 401 {
		t.Errorf("expected StatusCode 401, got %d", apiErr.StatusCode)
	}
}

func TestClient_CreateSite_Conflict(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(409)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "subdomain already exists"})
	}))
	defer srv.Close()

	c := New(srv.URL, "test-token")
	_, err := c.CreateSite("My Site", "taken")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected *APIError, got %T", err)
	}
	if apiErr.StatusCode != 409 {
		t.Errorf("expected StatusCode 409, got %d", apiErr.StatusCode)
	}
}

func TestClient_DeleteSite_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "DELETE" {
			t.Errorf("expected DELETE, got %s", r.Method)
		}
		if r.URL.Path != "/api/v1/sites/site123" {
			t.Errorf("expected /api/v1/sites/site123, got %s", r.URL.Path)
		}
		w.WriteHeader(204)
	}))
	defer srv.Close()

	c := New(srv.URL, "test-token")
	err := c.DeleteSite("site123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestClient_DeleteSite_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "site not found"})
	}))
	defer srv.Close()

	c := New(srv.URL, "test-token")
	err := c.DeleteSite("nonexistent")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected *APIError, got %T", err)
	}
	if apiErr.StatusCode != 404 {
		t.Errorf("expected StatusCode 404, got %d", apiErr.StatusCode)
	}
}

func TestClient_ResolveSite_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode([]Site{})
	}))
	defer srv.Close()

	c := New(srv.URL, "test-token")
	_, err := c.ResolveSite("nonexistent")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "no site found") {
		t.Errorf("expected 'no site found' error, got %v", err)
	}
}

func TestClient_ResolveSiteID_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode([]Site{
			{ID: "site123", Name: "Test", SubdomainSlug: "test"},
		})
	}))
	defer srv.Close()

	c := New(srv.URL, "test-token")
	id, err := c.ResolveSiteID("test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != "site123" {
		t.Errorf("expected ID 'site123', got %s", id)
	}
}

func TestClient_ResolveSiteID_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode([]Site{})
	}))
	defer srv.Close()

	c := New(srv.URL, "test-token")
	_, err := c.ResolveSiteID("nonexistent")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestClient_UpdateSite_BothFields(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("failed to decode request: %v", err)
		}
		if req["name"] != "New Name" {
			t.Errorf("expected name 'New Name', got %v", req["name"])
		}
		if req["spa_mode"] != false {
			t.Errorf("expected spa_mode false, got %v", req["spa_mode"])
		}
		w.WriteHeader(200)
		_ = json.NewEncoder(w).Encode(Site{
			ID:      "site123",
			Name:    "New Name",
			SPAMode: false,
		})
	}))
	defer srv.Close()

	c := New(srv.URL, "test-token")
	name := "New Name"
	spaMode := false
	site, err := c.UpdateSite("site123", &name, &spaMode)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if site.Name != "New Name" {
		t.Errorf("expected Name 'New Name', got %s", site.Name)
	}
	if site.SPAMode {
		t.Error("expected SPAMode false")
	}
}

func TestClient_UpdateSite_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "site not found"})
	}))
	defer srv.Close()

	c := New(srv.URL, "test-token")
	name := "New Name"
	_, err := c.UpdateSite("nonexistent", &name, nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected *APIError, got %T", err)
	}
	if apiErr.StatusCode != 404 {
		t.Errorf("expected StatusCode 404, got %d", apiErr.StatusCode)
	}
}

func TestClient_UpdateSite_NilBoth(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("failed to decode request: %v", err)
		}
		if len(req) != 0 {
			t.Errorf("expected empty request body, got %v", req)
		}
		w.WriteHeader(200)
		_ = json.NewEncoder(w).Encode(Site{ID: "site123"})
	}))
	defer srv.Close()

	c := New(srv.URL, "test-token")
	site, err := c.UpdateSite("site123", nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if site.ID != "site123" {
		t.Errorf("expected ID 'site123', got %s", site.ID)
	}
}
