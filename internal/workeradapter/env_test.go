package workeradapter

import (
	"testing"

	"github.com/cryguy/hostedat/internal/models"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupEnvTestDB(t *testing.T) *gorm.DB {
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

func TestBuildEnvFromDB_VarsAndSecrets(t *testing.T) {
	db := setupEnvTestDB(t)
	siteID := "site1"

	db.Create(&models.WorkerEnvVar{ID: "ev1", SiteID: siteID, Name: "API_URL", Value: "https://api.example.com", Secret: false})
	db.Create(&models.WorkerEnvVar{ID: "ev2", SiteID: siteID, Name: "API_KEY", Value: "secret123", Secret: true})
	db.Create(&models.WorkerEnvVar{ID: "ev3", SiteID: siteID, Name: "DEBUG", Value: "true", Secret: false})

	env := BuildEnvFromDB(BuildEnvOptions{DB: db}, siteID, nil)

	if env.SiteID != siteID {
		t.Errorf("SiteID = %q, want %q", env.SiteID, siteID)
	}
	if env.Vars["API_URL"] != "https://api.example.com" {
		t.Errorf("Vars[API_URL] = %q", env.Vars["API_URL"])
	}
	if env.Vars["DEBUG"] != "true" {
		t.Errorf("Vars[DEBUG] = %q", env.Vars["DEBUG"])
	}
	if env.Secrets["API_KEY"] != "secret123" {
		t.Errorf("Secrets[API_KEY] = %q", env.Secrets["API_KEY"])
	}
	// Secrets should not appear in Vars.
	if _, ok := env.Vars["API_KEY"]; ok {
		t.Error("API_KEY should not be in Vars")
	}
}

func TestBuildEnvFromDB_KVBindings(t *testing.T) {
	db := setupEnvTestDB(t)
	siteID := "site1"

	db.Create(&models.KVNamespace{ID: "kvns1", SiteID: siteID, Name: "MY_KV"})
	db.Create(&models.KVNamespace{ID: "kvns2", SiteID: siteID, Name: "SESSIONS"})

	env := BuildEnvFromDB(BuildEnvOptions{DB: db}, siteID, nil)

	if env.KV == nil {
		t.Fatal("KV should not be nil")
	}
	if len(env.KV) != 2 {
		t.Fatalf("expected 2 KV bindings, got %d", len(env.KV))
	}
	if _, ok := env.KV["MY_KV"]; !ok {
		t.Error("missing KV binding MY_KV")
	}
	if _, ok := env.KV["SESSIONS"]; !ok {
		t.Error("missing KV binding SESSIONS")
	}
}

func TestBuildEnvFromDB_D1Bindings(t *testing.T) {
	db := setupEnvTestDB(t)
	siteID := "site1"

	db.Create(&models.D1Database{ID: "d1-1", SiteID: siteID, Name: "MY_DB", DatabaseID: "db123"})

	env := BuildEnvFromDB(BuildEnvOptions{DB: db}, siteID, nil)

	if env.D1Bindings == nil {
		t.Fatal("D1Bindings should not be nil")
	}
	want := siteID + "_db123"
	if env.D1Bindings["MY_DB"] != want {
		t.Errorf("D1Bindings[MY_DB] = %q, want %q", env.D1Bindings["MY_DB"], want)
	}
}

func TestBuildEnvFromDB_DurableObjectBindings(t *testing.T) {
	db := setupEnvTestDB(t)
	siteID := "site1"

	db.Create(&models.DurableObjectNamespace{ID: "do1", SiteID: siteID, Name: "COUNTER", NamespaceID: "ns123"})

	env := BuildEnvFromDB(BuildEnvOptions{DB: db}, siteID, nil)

	if env.DurableObjects == nil {
		t.Fatal("DurableObjects should not be nil")
	}
	if _, ok := env.DurableObjects["COUNTER"]; !ok {
		t.Error("missing DurableObject binding COUNTER")
	}
}

func TestBuildEnvFromDB_NoBindings(t *testing.T) {
	db := setupEnvTestDB(t)

	env := BuildEnvFromDB(BuildEnvOptions{DB: db}, "empty-site", nil)

	if env.SiteID != "empty-site" {
		t.Errorf("SiteID = %q", env.SiteID)
	}
	if len(env.Vars) != 0 {
		t.Errorf("expected empty Vars, got %d", len(env.Vars))
	}
	if len(env.Secrets) != 0 {
		t.Errorf("expected empty Secrets, got %d", len(env.Secrets))
	}
	if env.KV != nil {
		t.Error("KV should be nil when no namespaces")
	}
	if env.D1Bindings != nil {
		t.Error("D1Bindings should be nil when no databases")
	}
	if env.DurableObjects != nil {
		t.Error("DurableObjects should be nil when no namespaces")
	}
}

func TestBuildEnvFromDB_D1DataDir(t *testing.T) {
	db := setupEnvTestDB(t)

	env := BuildEnvFromDB(BuildEnvOptions{DB: db, D1DataDir: "/custom/d1"}, "site1", nil)

	if env.D1DataDir != "/custom/d1" {
		t.Errorf("D1DataDir = %q, want %q", env.D1DataDir, "/custom/d1")
	}
}

func TestBuildEnvFromDB_SiteIsolation(t *testing.T) {
	db := setupEnvTestDB(t)

	db.Create(&models.WorkerEnvVar{ID: "ev1", SiteID: "site1", Name: "KEY", Value: "site1-val", Secret: false})
	db.Create(&models.WorkerEnvVar{ID: "ev2", SiteID: "site2", Name: "KEY", Value: "site2-val", Secret: false})

	env1 := BuildEnvFromDB(BuildEnvOptions{DB: db}, "site1", nil)
	env2 := BuildEnvFromDB(BuildEnvOptions{DB: db}, "site2", nil)

	if env1.Vars["KEY"] != "site1-val" {
		t.Errorf("site1 KEY = %q", env1.Vars["KEY"])
	}
	if env2.Vars["KEY"] != "site2-val" {
		t.Errorf("site2 KEY = %q", env2.Vars["KEY"])
	}
}

func TestBuildEnvFromDB_StorageSkippedWithoutMinio(t *testing.T) {
	db := setupEnvTestDB(t)
	siteID := "site1"

	db.Create(&models.StorageBucket{ID: "sb1", SiteID: siteID, Name: "FILES", BucketName: "files-bucket"})

	env := BuildEnvFromDB(BuildEnvOptions{DB: db, MinioClient: nil}, siteID, nil)

	if env.Storage != nil {
		t.Error("Storage should be nil when MinioClient is nil")
	}
}
