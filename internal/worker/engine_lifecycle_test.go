package worker

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/cryguy/hostedat/internal/models"
	"github.com/cryguy/hostedat/internal/storage"
)

// ---------------------------------------------------------------------------
// EnsureBytecode tests
// ---------------------------------------------------------------------------

func TestEnsureBytecode_FromDisk(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	tmpDir := t.TempDir()
	store := storage.NewManager(tmpDir)
	e.SetStore(store)

	siteID := "bc-disk-test"
	version := 1

	// Compile and cache bytecode.
	source := `export default { fetch() { return new Response("from disk"); } };`
	bytecode, err := e.CompileAndCache(siteID, version, source)
	if err != nil {
		t.Fatalf("CompileAndCache: %v", err)
	}

	// Write bytecode to disk (simulating a previous compilation).
	bcDir := store.GetWorkerBytecodeDir(siteID, version)
	os.MkdirAll(bcDir, 0755)
	bcPath := filepath.Join(bcDir, "bytecode.bin")
	if err := os.WriteFile(bcPath, bytecode, 0644); err != nil {
		t.Fatalf("writing bytecode: %v", err)
	}

	// Clear the in-memory bytecode cache to simulate restart.
	e.bytecodes.Delete(poolKey{SiteID: siteID, Version: version})

	// EnsureBytecode should reload from disk.
	if err := e.EnsureBytecode(siteID, version); err != nil {
		t.Fatalf("EnsureBytecode: %v", err)
	}

	// Verify bytecode is now in memory.
	if _, ok := e.bytecodes.Load(poolKey{SiteID: siteID, Version: version}); !ok {
		t.Error("bytecode should be in memory after EnsureBytecode")
	}
}

func TestEnsureBytecode_AlreadyInMemory(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	siteID := "bc-mem-test"
	source := `export default { fetch() { return new Response("cached"); } };`
	e.CompileAndCache(siteID, 1, source)

	// EnsureBytecode should be a no-op when already in memory.
	if err := e.EnsureBytecode(siteID, 1); err != nil {
		t.Fatalf("EnsureBytecode: %v", err)
	}
}

func TestEnsureBytecode_NoStoreSet(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	// Don't call SetStore — EnsureBytecode should error.
	err := e.EnsureBytecode("unknown-site", 1)
	if err == nil {
		t.Fatal("expected error when store not set")
	}
}

func TestEnsureBytecode_RecompileFromSource(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	tmpDir := t.TempDir()
	store := storage.NewManager(tmpDir)
	e.SetStore(store)

	siteID := "bc-recompile-test"
	version := 1

	// Write a worker script to disk (simulating deploy).
	deployDir := store.GetDeploymentPath(siteID, version)
	os.MkdirAll(deployDir, 0755)
	os.WriteFile(filepath.Join(deployDir, "_worker.js"), []byte(`export default { fetch() { return new Response("recompiled"); } };`), 0644)

	// No bytecode on disk, no bytecode in memory — should recompile from source.
	if err := e.EnsureBytecode(siteID, version); err != nil {
		t.Fatalf("EnsureBytecode recompile: %v", err)
	}

	// Verify bytecode is now in memory.
	if _, ok := e.bytecodes.Load(poolKey{SiteID: siteID, Version: version}); !ok {
		t.Error("bytecode should be in memory after recompile")
	}

	// Verify it actually works.
	r := e.Execute(siteID, version, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)
	if string(r.Response.Body) != "recompiled" {
		t.Errorf("body = %q, want recompiled", r.Response.Body)
	}
}

// ---------------------------------------------------------------------------
// Log retention tests
// ---------------------------------------------------------------------------

func TestLogRetention_CleansOldLogs(t *testing.T) {
	db := testDB(t)

	cfg := testCfg()
	cfg.MaxLogRetention = 1 // 1 day retention
	e := NewEngine(cfg, db)
	t.Cleanup(func() { e.Shutdown() })

	// Insert old log (2 days ago).
	oldLog := models.WorkerLog{
		SiteID:    "site1",
		Level:     "log",
		Message:   "old message",
		CreatedAt: time.Now().AddDate(0, 0, -2),
	}
	db.Create(&oldLog)

	// Insert recent log.
	newLog := models.WorkerLog{
		SiteID:    "site1",
		Level:     "log",
		Message:   "recent message",
		CreatedAt: time.Now(),
	}
	db.Create(&newLog)

	// Run cleanup manually (same logic as logRetentionLoop).
	cutoff := time.Now().AddDate(0, 0, -cfg.MaxLogRetention)
	db.Where("created_at < ?", cutoff).Delete(&models.WorkerLog{})

	var logs []models.WorkerLog
	db.Find(&logs)

	if len(logs) != 1 {
		t.Fatalf("log count = %d, want 1", len(logs))
	}
	if logs[0].Message != "recent message" {
		t.Errorf("remaining log = %q, want 'recent message'", logs[0].Message)
	}
}

// ---------------------------------------------------------------------------
// BuildEnvFromDB tests
// ---------------------------------------------------------------------------

func TestBuildEnvFromDB_LoadsVarsAndSecrets(t *testing.T) {
	db := testDB(t)
	db.AutoMigrate(&models.WorkerEnvVar{}, &models.KVNamespace{})

	siteID := "env-test-site"

	// Create env vars.
	db.Create(&models.WorkerEnvVar{SiteID: siteID, Name: "API_URL", Value: "https://api.example.com", Secret: false})
	db.Create(&models.WorkerEnvVar{SiteID: siteID, Name: "API_KEY", Value: "secret123", Secret: true})

	// Create KV namespace.
	db.Create(&models.KVNamespace{ID: "ns-123", SiteID: siteID, Name: "CACHE"})

	env := BuildEnvFromDB(db, siteID, nil)

	if env.Vars["API_URL"] != "https://api.example.com" {
		t.Errorf("API_URL = %q", env.Vars["API_URL"])
	}
	if env.Secrets["API_KEY"] != "secret123" {
		t.Errorf("API_KEY = %q", env.Secrets["API_KEY"])
	}
	if env.KVBindings["CACHE"] != "ns-123" {
		t.Errorf("CACHE binding = %q", env.KVBindings["CACHE"])
	}
	if env.Assets != nil {
		t.Error("Assets should be nil when no fetcher passed")
	}
}

func TestBuildEnvFromDB_EmptySite(t *testing.T) {
	db := testDB(t)
	db.AutoMigrate(&models.WorkerEnvVar{}, &models.KVNamespace{})

	env := BuildEnvFromDB(db, "nonexistent-site", nil)

	if len(env.Vars) != 0 {
		t.Errorf("Vars should be empty, got %v", env.Vars)
	}
	if len(env.Secrets) != 0 {
		t.Errorf("Secrets should be empty, got %v", env.Secrets)
	}
	if len(env.KVBindings) != 0 {
		t.Errorf("KVBindings should be empty, got %v", env.KVBindings)
	}
}

// ---------------------------------------------------------------------------
// Shutdown tests
// ---------------------------------------------------------------------------

func TestEngine_Shutdown_ClearsPools(t *testing.T) {
	db := testDB(t)
	// Don't use newTestEngine (it registers a cleanup that also calls Shutdown).
	cfg := testCfg()
	e := NewEngine(cfg, db)

	source := `export default { fetch() { return new Response("ok"); } };`
	e.CompileAndCache("site1", 1, source)

	// Trigger pool creation.
	e.Execute("site1", 1, defaultEnv(), getReq("http://localhost/"))

	e.Shutdown()

	// After shutdown, pools should be empty.
	poolCount := 0
	e.pools.Range(func(_, _ any) bool {
		poolCount++
		return true
	})
	if poolCount != 0 {
		t.Errorf("pool count = %d, want 0 after shutdown", poolCount)
	}

	bcCount := 0
	e.bytecodes.Range(func(_, _ any) bool {
		bcCount++
		return true
	})
	if bcCount != 0 {
		t.Errorf("bytecode count = %d, want 0 after shutdown", bcCount)
	}
}

func TestEngine_MaxResponseBytes(t *testing.T) {
	db := testDB(t)
	cfg := testCfg()
	cfg.MaxResponseBytes = 5 * 1024 * 1024
	e := NewEngine(cfg, db)
	t.Cleanup(func() { e.Shutdown() })

	if e.MaxResponseBytes() != 5*1024*1024 {
		t.Errorf("MaxResponseBytes = %d, want %d", e.MaxResponseBytes(), 5*1024*1024)
	}
}
