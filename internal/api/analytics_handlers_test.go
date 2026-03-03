package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/cryguy/hostedat/internal/analytics"
	"github.com/cryguy/hostedat/internal/config"
	"github.com/cryguy/hostedat/internal/models"
	"github.com/cryguy/hostedat/internal/storage"
	"github.com/glebarez/sqlite"
	"github.com/labstack/echo/v4"
	"gorm.io/gorm"
)

type analyticsTestEnv struct {
	*testEnv
	analyticsDB *gorm.DB
}

func setupTestEnvWithAnalytics(t *testing.T) *analyticsTestEnv {
	t.Helper()

	db, err := models.InitDB(config.DBConfig{Driver: "sqlite", DSN: ":memory:"})
	if err != nil {
		t.Fatalf("InitDB: %v", err)
	}

	aDB, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open analytics db: %v", err)
	}
	if err := aDB.AutoMigrate(&analytics.RequestLog{}, &analytics.HourlyStat{}); err != nil {
		t.Fatalf("migrate analytics: %v", err)
	}

	cfg := &config.Config{
		Domain:    "test.local",
		JWTSecret: "test-jwt-secret-that-is-at-least-32-characters-long",
		Registration: config.RegConfig{
			Enabled: true,
		},
	}
	if err := models.SeedDefaults(db, cfg); err != nil {
		t.Fatal(err)
	}

	store := storage.NewManager(t.TempDir())
	cache := storage.NewSiteRulesCache()

	e := echo.New()
	e.HTTPErrorHandler = CustomErrorHandler
	e.Use(SubdomainRouter(db, store, cache, cfg.Domain, nil, nil, nil, nil))
	RegisterRoutes(e, db, cfg, store, "0.1.0", nil, nil, nil, "", aDB)

	return &analyticsTestEnv{
		testEnv: &testEnv{
			e:         e,
			db:        db,
			store:     store,
			cache:     cache,
			jwtSecret: cfg.JWTSecret,
			domain:    cfg.Domain,
		},
		analyticsDB: aDB,
	}
}

func TestAnalytics_AuthRequired(t *testing.T) {
	env := setupTestEnvWithAnalytics(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/sites/abc/analytics/summary", nil)
	rec := httptest.NewRecorder()
	env.e.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestAnalytics_SiteNotFound(t *testing.T) {
	env := setupTestEnvWithAnalytics(t)
	_, token := env.createTestUser(t, "user@test.com", "password123", "user")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/sites/nonexistent/analytics/summary", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	env.e.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestAnalytics_OwnershipCheck(t *testing.T) {
	env := setupTestEnvWithAnalytics(t)
	owner, _ := env.createTestUser(t, "owner@test.com", "password123", "user")
	_, otherToken := env.createTestUser(t, "other@test.com", "password123", "user")

	// Create site owned by 'owner'.
	site := models.Site{Name: "test-site", SubdomainSlug: "test"}
	// Set user ID from owner token — need the actual user.
	var ownerUser models.User
	env.db.Where("email = ?", "owner@test.com").First(&ownerUser)
	site.UserID = ownerUser.ID
	env.db.Create(&site)
	_ = owner // suppress unused

	req := httptest.NewRequest(http.MethodGet, "/api/v1/sites/"+site.ID+"/analytics/summary", nil)
	req.Header.Set("Authorization", "Bearer "+otherToken)
	rec := httptest.NewRecorder()
	env.e.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
}

func TestAnalytics_EmptyDataReturnsZeros(t *testing.T) {
	env := setupTestEnvWithAnalytics(t)
	_, token := env.createTestUser(t, "user@test.com", "password123", "user")

	var user models.User
	env.db.Where("email = ?", "user@test.com").First(&user)
	site := models.Site{Name: "test-site", SubdomainSlug: "test", UserID: user.ID}
	env.db.Create(&site)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/sites/"+site.ID+"/analytics/summary?period=24h", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	env.e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	var result analytics.SummaryResult
	if err := json.NewDecoder(rec.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if result.Requests != 0 {
		t.Errorf("requests = %d, want 0", result.Requests)
	}
}

func TestAnalytics_PreSeededDataReturnsCorrectTotals(t *testing.T) {
	env := setupTestEnvWithAnalytics(t)
	_, token := env.createTestUser(t, "user@test.com", "password123", "user")

	var user models.User
	env.db.Where("email = ?", "user@test.com").First(&user)
	site := models.Site{Name: "test-site", SubdomainSlug: "test", UserID: user.ID}
	env.db.Create(&site)

	// Seed analytics data.
	now := time.Now().UTC().Truncate(time.Hour)
	env.analyticsDB.Create(&analytics.HourlyStat{
		SiteID: site.ID, Bucket: now.Add(-2 * time.Hour),
		Requests: 100, UniqueVisitors: 50, BytesSent: 5000,
		Status2xx: 90, Status3xx: 5, Status4xx: 3, Status5xx: 2,
	})
	env.analyticsDB.Create(&analytics.HourlyStat{
		SiteID: site.ID, Bucket: now.Add(-1 * time.Hour),
		Requests: 200, UniqueVisitors: 100, BytesSent: 10000,
		Status2xx: 180, Status3xx: 10, Status4xx: 5, Status5xx: 5,
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/sites/"+site.ID+"/analytics/summary?period=24h", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	env.e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	var result analytics.SummaryResult
	json.NewDecoder(rec.Body).Decode(&result)

	if result.Requests != 300 {
		t.Errorf("requests = %d, want 300", result.Requests)
	}
	if result.UniqueVisitors != 150 {
		t.Errorf("visitors = %d, want 150", result.UniqueVisitors)
	}
	if result.BytesSent != 15000 {
		t.Errorf("bytes = %d, want 15000", result.BytesSent)
	}
}

func TestAnalytics_TimeseriesEndpoint(t *testing.T) {
	env := setupTestEnvWithAnalytics(t)
	_, token := env.createTestUser(t, "user@test.com", "password123", "user")

	var user models.User
	env.db.Where("email = ?", "user@test.com").First(&user)
	site := models.Site{Name: "test-site", SubdomainSlug: "test", UserID: user.ID}
	env.db.Create(&site)

	now := time.Now().UTC().Truncate(time.Hour)
	env.analyticsDB.Create(&analytics.HourlyStat{SiteID: site.ID, Bucket: now.Add(-2 * time.Hour), Requests: 10})
	env.analyticsDB.Create(&analytics.HourlyStat{SiteID: site.ID, Bucket: now.Add(-1 * time.Hour), Requests: 20})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/sites/"+site.ID+"/analytics/timeseries?period=24h", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	env.e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	var points []analytics.TimeseriesPoint
	json.NewDecoder(rec.Body).Decode(&points)
	if len(points) != 2 {
		t.Fatalf("expected 2 points, got %d", len(points))
	}
}

func TestAnalytics_PagesEndpoint(t *testing.T) {
	env := setupTestEnvWithAnalytics(t)
	_, token := env.createTestUser(t, "user@test.com", "password123", "user")

	var user models.User
	env.db.Where("email = ?", "user@test.com").First(&user)
	site := models.Site{Name: "test-site", SubdomainSlug: "test", UserID: user.ID}
	env.db.Create(&site)

	now := time.Now().UTC().Truncate(time.Hour)
	pathsJSON, _ := json.Marshal([]analytics.TopEntry{{Value: "/", Requests: 50}, {Value: "/about", Requests: 20}})
	env.analyticsDB.Create(&analytics.HourlyStat{SiteID: site.ID, Bucket: now.Add(-1 * time.Hour), Requests: 70, TopPaths: string(pathsJSON)})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/sites/"+site.ID+"/analytics/pages?period=24h", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	env.e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	var pages []analytics.TopEntry
	json.NewDecoder(rec.Body).Decode(&pages)
	if len(pages) != 2 {
		t.Fatalf("expected 2 pages, got %d", len(pages))
	}
	if pages[0].Value != "/" {
		t.Errorf("top page = %s, want /", pages[0].Value)
	}
}

func TestAnalytics_DisabledRoutesNotRegistered(t *testing.T) {
	// Use the standard test env (nil analytics DB).
	env := setupTestEnv(t)
	_, token := env.createTestUser(t, "user@test.com", "password123", "user")

	var user models.User
	env.db.Where("email = ?", "user@test.com").First(&user)
	site := models.Site{Name: "test-site", SubdomainSlug: "test", UserID: user.ID}
	env.db.Create(&site)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/sites/"+site.ID+"/analytics/summary", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	env.e.ServeHTTP(rec, req)

	// When analytics is disabled, the route doesn't exist → 404 from Echo.
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 (routes not registered)", rec.Code)
	}
}

func TestAnalytics_AdminCanAccessAnySite(t *testing.T) {
	env := setupTestEnvWithAnalytics(t)
	_, ownerToken := env.createTestUser(t, "owner@test.com", "password123", "user")
	_, adminToken := env.createTestUser(t, "admin@test.com", "password123", "admin")

	var owner models.User
	env.db.Where("email = ?", "owner@test.com").First(&owner)
	site := models.Site{Name: "test-site", SubdomainSlug: "test", UserID: owner.ID}
	env.db.Create(&site)
	_ = ownerToken

	req := httptest.NewRequest(http.MethodGet, "/api/v1/sites/"+site.ID+"/analytics/summary?period=24h", nil)
	req.Header.Set("Authorization", "Bearer "+adminToken)
	rec := httptest.NewRecorder()
	env.e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (admin access)", rec.Code)
	}
}
