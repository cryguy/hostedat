package workeradapter

import (
	"testing"

	"github.com/cryguy/hostedat/internal/models"
	"github.com/cryguy/worker/v2"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

// setupFullEnvTestDB creates an in-memory SQLite database with all models
// needed by BuildEnvFromDB, including the full site + binding schema.
func setupFullEnvTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		DisableForeignKeyConstraintWhenMigrating: true,
	})
	if err != nil {
		t.Fatalf("opening test db: %v", err)
	}
	if err := db.AutoMigrate(
		&models.WorkerEnvVar{},
		&models.KVNamespace{},
		&models.D1Database{},
		&models.DurableObjectNamespace{},
		&models.StorageBucket{},
	); err != nil {
		t.Fatalf("migrating: %v", err)
	}
	return db
}

// mockDispatcher is a minimal WorkerDispatcher for testing that env.Dispatcher
// is wired through correctly.
type mockDispatcher struct {
	called bool
}

func (m *mockDispatcher) Execute(siteID, deployKey string, env *worker.Env, req *worker.WorkerRequest) *worker.WorkerResult {
	m.called = true
	return &worker.WorkerResult{}
}

// TestBuildEnvFromDB_AllBindingsPopulated verifies that BuildEnvFromDB populates
// every field when the database contains all binding types.
func TestBuildEnvFromDB_AllBindingsPopulated(t *testing.T) {
	db := setupFullEnvTestDB(t)
	siteID := "site_full"

	// Env vars (plain + secret).
	db.Create(&models.WorkerEnvVar{ID: "ev1", SiteID: siteID, Name: "API_URL", Value: "https://api.example.com", Secret: false})
	db.Create(&models.WorkerEnvVar{ID: "ev2", SiteID: siteID, Name: "DB_PASS", Value: "s3cret", Secret: true})
	db.Create(&models.WorkerEnvVar{ID: "ev3", SiteID: siteID, Name: "DEBUG", Value: "true", Secret: false})

	// KV namespaces.
	db.Create(&models.KVNamespace{ID: "kv1", SiteID: siteID, Name: "CACHE"})
	db.Create(&models.KVNamespace{ID: "kv2", SiteID: siteID, Name: "SESSIONS"})
	db.Create(&models.KVNamespace{ID: "kv3", SiteID: siteID, Name: "METADATA"})

	// Storage buckets (will be nil since MinioClient is nil, but we insert them to test the nil path).
	db.Create(&models.StorageBucket{ID: "sb1", SiteID: siteID, Name: "UPLOADS", BucketName: "uploads-bucket"})
	db.Create(&models.StorageBucket{ID: "sb2", SiteID: siteID, Name: "ASSETS", BucketName: "assets-bucket"})

	// D1 databases.
	db.Create(&models.D1Database{ID: "d1a", SiteID: siteID, Name: "MAIN_DB", DatabaseID: "maindb001"})
	db.Create(&models.D1Database{ID: "d1b", SiteID: siteID, Name: "ANALYTICS", DatabaseID: "analyticsdb"})

	// Durable Object namespaces.
	db.Create(&models.DurableObjectNamespace{ID: "do1", SiteID: siteID, Name: "COUNTER", NamespaceID: "ns_counter"})
	db.Create(&models.DurableObjectNamespace{ID: "do2", SiteID: siteID, Name: "ROOM", NamespaceID: "ns_room"})

	disp := &mockDispatcher{}

	env := BuildEnvFromDB(BuildEnvOptions{
		DB:         db,
		D1DataDir:  "/data/d1",
		Dispatcher: disp,
	}, siteID, nil)

	// --- SiteID ---
	if env.SiteID != siteID {
		t.Errorf("SiteID = %q, want %q", env.SiteID, siteID)
	}

	// --- Vars ---
	if len(env.Vars) != 2 {
		t.Errorf("len(Vars) = %d, want 2 (non-secret vars)", len(env.Vars))
	}
	if env.Vars["API_URL"] != "https://api.example.com" {
		t.Errorf("Vars[API_URL] = %q", env.Vars["API_URL"])
	}
	if env.Vars["DEBUG"] != "true" {
		t.Errorf("Vars[DEBUG] = %q", env.Vars["DEBUG"])
	}

	// --- Secrets ---
	if len(env.Secrets) != 1 {
		t.Errorf("len(Secrets) = %d, want 1", len(env.Secrets))
	}
	if env.Secrets["DB_PASS"] != "s3cret" {
		t.Errorf("Secrets[DB_PASS] = %q", env.Secrets["DB_PASS"])
	}

	// --- KV ---
	if env.KV == nil {
		t.Fatal("KV should not be nil")
	}
	if len(env.KV) != 3 {
		t.Errorf("len(KV) = %d, want 3", len(env.KV))
	}
	for _, name := range []string{"CACHE", "SESSIONS", "METADATA"} {
		if _, ok := env.KV[name]; !ok {
			t.Errorf("missing KV binding %q", name)
		}
	}

	// --- Storage (nil because MinioClient is nil) ---
	if env.Storage != nil {
		t.Errorf("Storage should be nil when MinioClient is nil, got %d entries", len(env.Storage))
	}

	// --- D1Bindings ---
	if env.D1Bindings == nil {
		t.Fatal("D1Bindings should not be nil")
	}
	if len(env.D1Bindings) != 2 {
		t.Errorf("len(D1Bindings) = %d, want 2", len(env.D1Bindings))
	}
	if env.D1Bindings["MAIN_DB"] != siteID+"_maindb001" {
		t.Errorf("D1Bindings[MAIN_DB] = %q, want %q", env.D1Bindings["MAIN_DB"], siteID+"_maindb001")
	}
	if env.D1Bindings["ANALYTICS"] != siteID+"_analyticsdb" {
		t.Errorf("D1Bindings[ANALYTICS] = %q, want %q", env.D1Bindings["ANALYTICS"], siteID+"_analyticsdb")
	}

	// --- DurableObjects ---
	if env.DurableObjects == nil {
		t.Fatal("DurableObjects should not be nil")
	}
	if len(env.DurableObjects) != 2 {
		t.Errorf("len(DurableObjects) = %d, want 2", len(env.DurableObjects))
	}
	for _, name := range []string{"COUNTER", "ROOM"} {
		if _, ok := env.DurableObjects[name]; !ok {
			t.Errorf("missing DurableObject binding %q", name)
		}
	}

	// --- D1DataDir ---
	if env.D1DataDir != "/data/d1" {
		t.Errorf("D1DataDir = %q, want %q", env.D1DataDir, "/data/d1")
	}

	// --- Dispatcher ---
	if env.Dispatcher == nil {
		t.Fatal("Dispatcher should not be nil")
	}

	// --- Assets (passed as nil) ---
	if env.Assets != nil {
		t.Error("Assets should be nil when passed as nil")
	}
}

// TestBuildEnvFromDB_GracefulDegradation verifies that missing optional
// dependencies result in nil/empty fields rather than panics.
func TestBuildEnvFromDB_GracefulDegradation(t *testing.T) {
	db := setupFullEnvTestDB(t)
	siteID := "site_degrade"

	// Only create storage buckets (no KV, D1, DO, or env vars).
	db.Create(&models.StorageBucket{ID: "sb1", SiteID: siteID, Name: "FILES", BucketName: "files-bucket"})

	t.Run("MinioNil_StorageNil", func(t *testing.T) {
		env := BuildEnvFromDB(BuildEnvOptions{DB: db, MinioClient: nil}, siteID, nil)
		if env.Storage != nil {
			t.Error("Storage should be nil when MinioClient is nil")
		}
	})

	t.Run("D1DataDirEmpty", func(t *testing.T) {
		env := BuildEnvFromDB(BuildEnvOptions{DB: db, D1DataDir: ""}, siteID, nil)
		if env.D1DataDir != "" {
			t.Errorf("D1DataDir = %q, want empty", env.D1DataDir)
		}
	})

	t.Run("NoKVNamespaces", func(t *testing.T) {
		env := BuildEnvFromDB(BuildEnvOptions{DB: db}, siteID, nil)
		if env.KV != nil {
			t.Errorf("KV should be nil when no namespaces exist, got %d entries", len(env.KV))
		}
	})

	t.Run("NoD1Databases", func(t *testing.T) {
		env := BuildEnvFromDB(BuildEnvOptions{DB: db}, siteID, nil)
		if env.D1Bindings != nil {
			t.Errorf("D1Bindings should be nil when no databases exist, got %d entries", len(env.D1Bindings))
		}
	})

	t.Run("NoDurableObjects", func(t *testing.T) {
		env := BuildEnvFromDB(BuildEnvOptions{DB: db}, siteID, nil)
		if env.DurableObjects != nil {
			t.Errorf("DurableObjects should be nil when no namespaces exist, got %d entries", len(env.DurableObjects))
		}
	})

	t.Run("NoEnvVars_EmptyMaps", func(t *testing.T) {
		env := BuildEnvFromDB(BuildEnvOptions{DB: db}, siteID, nil)
		if env.Vars == nil {
			t.Fatal("Vars should be non-nil empty map")
		}
		if len(env.Vars) != 0 {
			t.Errorf("Vars should be empty, got %d entries", len(env.Vars))
		}
		if env.Secrets == nil {
			t.Fatal("Secrets should be non-nil empty map")
		}
		if len(env.Secrets) != 0 {
			t.Errorf("Secrets should be empty, got %d entries", len(env.Secrets))
		}
	})

	t.Run("DispatcherNil", func(t *testing.T) {
		env := BuildEnvFromDB(BuildEnvOptions{DB: db, Dispatcher: nil}, siteID, nil)
		if env.Dispatcher != nil {
			t.Error("Dispatcher should be nil when not provided")
		}
	})
}

// TestBuildEnvFromDB_WithDispatcher verifies that a mock dispatcher flows
// through to the env correctly.
func TestBuildEnvFromDB_WithDispatcher(t *testing.T) {
	db := setupFullEnvTestDB(t)
	disp := &mockDispatcher{}

	env := BuildEnvFromDB(BuildEnvOptions{
		DB:         db,
		Dispatcher: disp,
	}, "site_disp", nil)

	if env.Dispatcher == nil {
		t.Fatal("Dispatcher should not be nil")
	}

	// Verify it is the same instance by calling Execute.
	env.Dispatcher.Execute("x", "y", nil, nil)
	if !disp.called {
		t.Error("Dispatcher.Execute was not called on the mock")
	}
}

// TestBuildEnvFromDB_SiteIsolation_AllBindings verifies that when bindings
// exist for two different sites, BuildEnvFromDB only returns the bindings
// belonging to the requested site.
func TestBuildEnvFromDB_SiteIsolation_AllBindings(t *testing.T) {
	db := setupFullEnvTestDB(t)

	// Site A bindings.
	db.Create(&models.WorkerEnvVar{ID: "eva1", SiteID: "siteA", Name: "KEY", Value: "A-val", Secret: false})
	db.Create(&models.KVNamespace{ID: "kvA", SiteID: "siteA", Name: "NS_A"})
	db.Create(&models.D1Database{ID: "d1A", SiteID: "siteA", Name: "DB_A", DatabaseID: "dbA"})
	db.Create(&models.DurableObjectNamespace{ID: "doA", SiteID: "siteA", Name: "DO_A", NamespaceID: "nsA"})
	db.Create(&models.StorageBucket{ID: "sbA", SiteID: "siteA", Name: "BUCKET_A", BucketName: "bucket-a"})

	// Site B bindings.
	db.Create(&models.WorkerEnvVar{ID: "evb1", SiteID: "siteB", Name: "KEY", Value: "B-val", Secret: false})
	db.Create(&models.KVNamespace{ID: "kvB", SiteID: "siteB", Name: "NS_B"})
	db.Create(&models.D1Database{ID: "d1B", SiteID: "siteB", Name: "DB_B", DatabaseID: "dbB"})
	db.Create(&models.DurableObjectNamespace{ID: "doB", SiteID: "siteB", Name: "DO_B", NamespaceID: "nsB"})
	db.Create(&models.StorageBucket{ID: "sbB", SiteID: "siteB", Name: "BUCKET_B", BucketName: "bucket-b"})

	envA := BuildEnvFromDB(BuildEnvOptions{DB: db}, "siteA", nil)

	// Vars: only siteA.
	if envA.Vars["KEY"] != "A-val" {
		t.Errorf("siteA Vars[KEY] = %q, want %q", envA.Vars["KEY"], "A-val")
	}

	// KV: only siteA.
	if len(envA.KV) != 1 {
		t.Errorf("siteA KV count = %d, want 1", len(envA.KV))
	}
	if _, ok := envA.KV["NS_A"]; !ok {
		t.Error("siteA missing KV binding NS_A")
	}
	if _, ok := envA.KV["NS_B"]; ok {
		t.Error("siteA should not have siteB's KV binding NS_B")
	}

	// D1: only siteA.
	if len(envA.D1Bindings) != 1 {
		t.Errorf("siteA D1 count = %d, want 1", len(envA.D1Bindings))
	}
	if envA.D1Bindings["DB_A"] != "siteA_dbA" {
		t.Errorf("siteA D1Bindings[DB_A] = %q", envA.D1Bindings["DB_A"])
	}
	if _, ok := envA.D1Bindings["DB_B"]; ok {
		t.Error("siteA should not have siteB's D1 binding DB_B")
	}

	// DurableObjects: only siteA.
	if len(envA.DurableObjects) != 1 {
		t.Errorf("siteA DO count = %d, want 1", len(envA.DurableObjects))
	}
	if _, ok := envA.DurableObjects["DO_A"]; !ok {
		t.Error("siteA missing DO binding DO_A")
	}
	if _, ok := envA.DurableObjects["DO_B"]; ok {
		t.Error("siteA should not have siteB's DO binding DO_B")
	}

	// Storage: nil because MinioClient is nil, but verify with siteB too.
	if envA.Storage != nil {
		t.Error("siteA Storage should be nil without MinioClient")
	}
}

// TestBuildEnvFromDB_MultipleVarsCorrectlySorted verifies that many env vars
// are properly categorized into Vars vs Secrets.
func TestBuildEnvFromDB_MultipleVarsCorrectlySorted(t *testing.T) {
	db := setupFullEnvTestDB(t)
	siteID := "site_multi"

	vars := []models.WorkerEnvVar{
		{ID: "v1", SiteID: siteID, Name: "PUBLIC_1", Value: "pub1", Secret: false},
		{ID: "v2", SiteID: siteID, Name: "PUBLIC_2", Value: "pub2", Secret: false},
		{ID: "v3", SiteID: siteID, Name: "SECRET_1", Value: "sec1", Secret: true},
		{ID: "v4", SiteID: siteID, Name: "SECRET_2", Value: "sec2", Secret: true},
		{ID: "v5", SiteID: siteID, Name: "SECRET_3", Value: "sec3", Secret: true},
		{ID: "v6", SiteID: siteID, Name: "PUBLIC_3", Value: "pub3", Secret: false},
	}
	for _, v := range vars {
		db.Create(&v)
	}

	env := BuildEnvFromDB(BuildEnvOptions{DB: db}, siteID, nil)

	if len(env.Vars) != 3 {
		t.Errorf("len(Vars) = %d, want 3", len(env.Vars))
	}
	if len(env.Secrets) != 3 {
		t.Errorf("len(Secrets) = %d, want 3", len(env.Secrets))
	}

	// Verify no cross-contamination.
	for _, name := range []string{"SECRET_1", "SECRET_2", "SECRET_3"} {
		if _, ok := env.Vars[name]; ok {
			t.Errorf("%s should not appear in Vars", name)
		}
	}
	for _, name := range []string{"PUBLIC_1", "PUBLIC_2", "PUBLIC_3"} {
		if _, ok := env.Secrets[name]; ok {
			t.Errorf("%s should not appear in Secrets", name)
		}
	}
}

// TestBuildEnvFromDB_KVStoreType verifies that KV bindings are wired to
// GORMKVStore instances with the correct namespace IDs.
func TestBuildEnvFromDB_KVStoreType(t *testing.T) {
	db := setupFullEnvTestDB(t)
	siteID := "site_kvtype"

	db.Create(&models.KVNamespace{ID: "kvns_abc", SiteID: siteID, Name: "MY_KV"})

	env := BuildEnvFromDB(BuildEnvOptions{DB: db}, siteID, nil)

	kvStore, ok := env.KV["MY_KV"]
	if !ok {
		t.Fatal("missing KV binding MY_KV")
	}

	gormKV, ok := kvStore.(*GORMKVStore)
	if !ok {
		t.Fatalf("KV[MY_KV] type = %T, want *GORMKVStore", kvStore)
	}
	if gormKV.NamespaceID != "kvns_abc" {
		t.Errorf("NamespaceID = %q, want %q", gormKV.NamespaceID, "kvns_abc")
	}
}

// TestBuildEnvFromDB_DurableObjectStoreType verifies that DO bindings are wired
// to GORMDurableObjectStore instances.
func TestBuildEnvFromDB_DurableObjectStoreType(t *testing.T) {
	db := setupFullEnvTestDB(t)
	siteID := "site_dotype"

	db.Create(&models.DurableObjectNamespace{ID: "do_abc", SiteID: siteID, Name: "MY_DO", NamespaceID: "ns_abc"})

	env := BuildEnvFromDB(BuildEnvOptions{DB: db}, siteID, nil)

	doStore, ok := env.DurableObjects["MY_DO"]
	if !ok {
		t.Fatal("missing DO binding MY_DO")
	}

	_, ok = doStore.(*GORMDurableObjectStore)
	if !ok {
		t.Fatalf("DurableObjects[MY_DO] type = %T, want *GORMDurableObjectStore", doStore)
	}
}
