package api

import (
	"testing"

	"github.com/cryguy/hostedat/internal/config"
	"github.com/cryguy/hostedat/internal/models"
	"github.com/cryguy/hostedat/internal/storage"
	"github.com/cryguy/hostedat/internal/workeradapter"
)

// TestBuildWorkerEnv_PopulatesAllBindings verifies that the package-private
// buildWorkerEnv function correctly wires database bindings into a worker.Env.
func TestBuildWorkerEnv_PopulatesAllBindings(t *testing.T) {
	db, err := models.InitDB(config.DBConfig{Driver: "sqlite", DSN: ":memory:"})
	if err != nil {
		t.Fatalf("InitDB: %v", err)
	}

	store := storage.NewManager(t.TempDir())
	cache := storage.NewSiteRulesCache()

	// Create a site.
	site := models.Site{ID: "site_wenv", UserID: "user1", SubdomainSlug: "wenv", Name: "WorkerEnvTest"}
	db.Create(&models.User{ID: "user1", Email: "test@test.com", PasswordHash: "hash", Role: "user"})
	db.Create(&site)

	deployID := models.GenerateID()
	db.Create(&models.Deployment{ID: deployID, SiteID: site.ID, Version: 1, FileHash: "abc"})

	// Create bindings for the site.
	db.Create(&models.WorkerEnvVar{ID: "ev1", SiteID: site.ID, Name: "API_URL", Value: "https://api.test.com", Secret: false})
	db.Create(&models.WorkerEnvVar{ID: "ev2", SiteID: site.ID, Name: "TOKEN", Value: "secret-token", Secret: true})

	db.Create(&models.KVNamespace{ID: "kvns1", SiteID: site.ID, Name: "CACHE_NS"})
	db.Create(&models.KVNamespace{ID: "kvns2", SiteID: site.ID, Name: "SESSIONS_NS"})

	db.Create(&models.D1Database{ID: "d1db1", SiteID: site.ID, Name: "MAIN_DB", DatabaseID: "maindb"})

	db.Create(&models.DurableObjectNamespace{ID: "do1", SiteID: site.ID, Name: "COUNTER", NamespaceID: "ns_counter"})

	db.Create(&models.StorageBucket{ID: "sb1", SiteID: site.ID, Name: "UPLOADS", BucketName: "uploads-bucket"})

	// Create bindings for a DIFFERENT site to test isolation.
	db.Create(&models.User{ID: "user2", Email: "other@test.com", PasswordHash: "hash", Role: "user"})
	otherSite := models.Site{ID: "site_other", UserID: "user2", SubdomainSlug: "other", Name: "Other"}
	db.Create(&otherSite)
	db.Create(&models.WorkerEnvVar{ID: "ev_other", SiteID: otherSite.ID, Name: "OTHER_KEY", Value: "other-val", Secret: false})
	db.Create(&models.KVNamespace{ID: "kv_other", SiteID: otherSite.ID, Name: "OTHER_KV"})

	// Call buildWorkerEnv with WorkerDeps but nil minio (unit test).
	deps := &WorkerDeps{
		D1DataDir: "/test/d1",
	}
	env := buildWorkerEnv(db, store, cache, &site, deployID, "test.local", deps)

	// --- SiteID ---
	if env.SiteID != site.ID {
		t.Errorf("SiteID = %q, want %q", env.SiteID, site.ID)
	}

	// --- Vars ---
	if env.Vars["API_URL"] != "https://api.test.com" {
		t.Errorf("Vars[API_URL] = %q", env.Vars["API_URL"])
	}
	if _, ok := env.Vars["TOKEN"]; ok {
		t.Error("TOKEN should not be in Vars (it's a secret)")
	}

	// --- Secrets ---
	if env.Secrets["TOKEN"] != "secret-token" {
		t.Errorf("Secrets[TOKEN] = %q", env.Secrets["TOKEN"])
	}
	if _, ok := env.Secrets["API_URL"]; ok {
		t.Error("API_URL should not be in Secrets")
	}

	// --- KV ---
	if env.KV == nil {
		t.Fatal("KV should not be nil")
	}
	if len(env.KV) != 2 {
		t.Errorf("len(KV) = %d, want 2", len(env.KV))
	}
	if _, ok := env.KV["CACHE_NS"]; !ok {
		t.Error("missing KV binding CACHE_NS")
	}
	if _, ok := env.KV["SESSIONS_NS"]; !ok {
		t.Error("missing KV binding SESSIONS_NS")
	}
	// Other site's KV should not be present.
	if _, ok := env.KV["OTHER_KV"]; ok {
		t.Error("other site's KV binding should not be present")
	}

	// --- D1Bindings ---
	if env.D1Bindings == nil {
		t.Fatal("D1Bindings should not be nil")
	}
	if env.D1Bindings["MAIN_DB"] != site.ID+"_maindb" {
		t.Errorf("D1Bindings[MAIN_DB] = %q, want %q", env.D1Bindings["MAIN_DB"], site.ID+"_maindb")
	}

	// --- D1DataDir ---
	if env.D1DataDir != "/test/d1" {
		t.Errorf("D1DataDir = %q, want /test/d1", env.D1DataDir)
	}

	// --- DurableObjects ---
	if env.DurableObjects == nil {
		t.Fatal("DurableObjects should not be nil")
	}
	if _, ok := env.DurableObjects["COUNTER"]; !ok {
		t.Error("missing DurableObject binding COUNTER")
	}

	// --- Cache ---
	if env.Cache == nil {
		t.Fatal("Cache should not be nil")
	}

	// --- Storage (nil without MinioClient) ---
	if env.Storage != nil {
		t.Error("Storage should be nil when MinioClient is nil in WorkerDeps")
	}

	// --- Assets ---
	if env.Assets == nil {
		t.Fatal("Assets should not be nil")
	}
	fetcher, ok := env.Assets.(*workeradapter.StaticAssetsFetcher)
	if !ok {
		t.Fatalf("Assets type = %T, want *workeradapter.StaticAssetsFetcher", env.Assets)
	}
	if fetcher.SiteID != site.ID {
		t.Errorf("Assets.SiteID = %q, want %q", fetcher.SiteID, site.ID)
	}
	if fetcher.DeployKey != deployID {
		t.Errorf("Assets.DeployKey = %q, want %q", fetcher.DeployKey, deployID)
	}
	if fetcher.Domain != "test.local" {
		t.Errorf("Assets.Domain = %q, want %q", fetcher.Domain, "test.local")
	}

	// --- Isolation: no cross-site leakage ---
	if _, ok := env.Vars["OTHER_KEY"]; ok {
		t.Error("other site's env var should not be present")
	}
}

// TestBuildWorkerEnv_NilDeps verifies that buildWorkerEnv handles nil WorkerDeps
// without panicking.
func TestBuildWorkerEnv_NilDeps(t *testing.T) {
	db, err := models.InitDB(config.DBConfig{Driver: "sqlite", DSN: ":memory:"})
	if err != nil {
		t.Fatalf("InitDB: %v", err)
	}

	store := storage.NewManager(t.TempDir())
	cache := storage.NewSiteRulesCache()

	db.Create(&models.User{ID: "u1", Email: "t@t.com", PasswordHash: "h", Role: "user"})
	site := models.Site{ID: "site_nil", UserID: "u1", SubdomainSlug: "nilsite", Name: "NilDeps"}
	db.Create(&site)

	deployID := models.GenerateID()

	// Should not panic with nil deps.
	env := buildWorkerEnv(db, store, cache, &site, deployID, "test.local", nil)

	if env.SiteID != site.ID {
		t.Errorf("SiteID = %q, want %q", env.SiteID, site.ID)
	}
	if env.D1DataDir != "" {
		t.Errorf("D1DataDir = %q, want empty with nil deps", env.D1DataDir)
	}
	if env.Storage != nil {
		t.Error("Storage should be nil with nil deps")
	}
	if env.Cache == nil {
		t.Fatal("Cache should not be nil even with nil deps")
	}
}

// TestBuildWorkerEnv_SPAModePassthrough verifies that SPAMode from the site
// is forwarded to the assets fetcher.
func TestBuildWorkerEnv_SPAModePassthrough(t *testing.T) {
	db, err := models.InitDB(config.DBConfig{Driver: "sqlite", DSN: ":memory:"})
	if err != nil {
		t.Fatalf("InitDB: %v", err)
	}

	store := storage.NewManager(t.TempDir())
	cache := storage.NewSiteRulesCache()

	db.Create(&models.User{ID: "u1", Email: "t@t.com", PasswordHash: "h", Role: "user"})
	site := models.Site{ID: "site_spa", UserID: "u1", SubdomainSlug: "spa", Name: "SPA", SPAMode: true}
	db.Create(&site)

	deployID := models.GenerateID()
	env := buildWorkerEnv(db, store, cache, &site, deployID, "test.local", nil)

	fetcher, ok := env.Assets.(*workeradapter.StaticAssetsFetcher)
	if !ok {
		t.Fatalf("Assets type = %T, want *workeradapter.StaticAssetsFetcher", env.Assets)
	}
	if !fetcher.SPAMode {
		t.Error("SPAMode should be true on the assets fetcher")
	}
}
