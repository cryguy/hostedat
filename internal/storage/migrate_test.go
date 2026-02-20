package storage

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/cryguy/hostedat/internal/config"
	"github.com/cryguy/hostedat/internal/models"
	"gorm.io/gorm"
)

func setupTestDBForMigration(t *testing.T) (*gorm.DB, *Manager) {
	t.Helper()
	db, err := models.InitDB(config.DBConfig{Driver: "sqlite", DSN: ":memory:"})
	if err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	store := NewManager(t.TempDir())
	return db, store
}

func TestMigrateDeployPaths_NoSitesToMigrate(t *testing.T) {
	db, store := setupTestDBForMigration(t)

	// Empty database - no sites with active_version
	if err := MigrateDeployPaths(db, store); err != nil {
		t.Fatalf("MigrateDeployPaths: %v", err)
	}
}

func TestMigrateDeployPaths_SiteWithoutActiveVersion(t *testing.T) {
	db, store := setupTestDBForMigration(t)

	u := models.User{Email: "user@test.com", PasswordHash: "hash"}
	db.Create(&u)
	s := models.Site{UserID: u.ID, SubdomainSlug: "test", Name: "Test", ActiveVersion: nil}
	db.Create(&s)

	// Site has no active_version - should be skipped
	if err := MigrateDeployPaths(db, store); err != nil {
		t.Fatalf("MigrateDeployPaths: %v", err)
	}

	// Verify site unchanged
	var site models.Site
	db.First(&site, "id = ?", s.ID)
	if site.ActiveDeployID != nil {
		t.Error("ActiveDeployID should remain nil")
	}
}

func TestMigrateDeployPaths_SiteWithActiveDeployIDAlreadySet(t *testing.T) {
	db, store := setupTestDBForMigration(t)

	u := models.User{Email: "user@test.com", PasswordHash: "hash"}
	db.Create(&u)

	deployID := "existing_dep"
	version := 1
	s := models.Site{
		UserID:         u.ID,
		SubdomainSlug:  "test",
		Name:           "Test",
		ActiveVersion:  &version,
		ActiveDeployID: &deployID,
	}
	db.Create(&s)

	// Site already has active_deploy_id - should be skipped
	if err := MigrateDeployPaths(db, store); err != nil {
		t.Fatalf("MigrateDeployPaths: %v", err)
	}

	// Verify site unchanged
	var site models.Site
	db.First(&site, "id = ?", s.ID)
	if site.ActiveDeployID == nil || *site.ActiveDeployID != deployID {
		t.Errorf("ActiveDeployID = %v, want %s", site.ActiveDeployID, deployID)
	}
}

func TestMigrateDeployPaths_BackfillsActiveDeployID(t *testing.T) {
	db, store := setupTestDBForMigration(t)

	u := models.User{Email: "user@test.com", PasswordHash: "hash"}
	db.Create(&u)

	version := 1
	s := models.Site{
		UserID:        u.ID,
		SubdomainSlug: "test",
		Name:          "Test",
		ActiveVersion: &version,
	}
	db.Create(&s)

	dep := models.Deployment{
		SiteID:   s.ID,
		Version:  version,
		FileHash: "abc123",
	}
	db.Create(&dep)

	if err := MigrateDeployPaths(db, store); err != nil {
		t.Fatalf("MigrateDeployPaths: %v", err)
	}

	// Verify active_deploy_id was set
	var site models.Site
	db.First(&site, "id = ?", s.ID)
	if site.ActiveDeployID == nil {
		t.Fatal("expected ActiveDeployID to be set")
	}
	if *site.ActiveDeployID != dep.ID {
		t.Errorf("ActiveDeployID = %s, want %s", *site.ActiveDeployID, dep.ID)
	}
}

func TestMigrateDeployPaths_RenamesDirectory(t *testing.T) {
	db, store := setupTestDBForMigration(t)

	u := models.User{Email: "user@test.com", PasswordHash: "hash"}
	db.Create(&u)

	version := 1
	s := models.Site{
		UserID:        u.ID,
		SubdomainSlug: "test",
		Name:          "Test",
		ActiveVersion: &version,
	}
	db.Create(&s)

	dep := models.Deployment{
		SiteID:   s.ID,
		Version:  version,
		FileHash: "abc123",
	}
	db.Create(&dep)

	// Create old-style directory: {siteID}/{version}
	oldPath := filepath.Join(store.BasePath, s.ID, strconv.Itoa(version))
	if err := os.MkdirAll(oldPath, 0755); err != nil {
		t.Fatalf("create old path: %v", err)
	}
	testFile := filepath.Join(oldPath, "index.html")
	if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
		t.Fatalf("write test file: %v", err)
	}

	if err := MigrateDeployPaths(db, store); err != nil {
		t.Fatalf("MigrateDeployPaths: %v", err)
	}

	// Verify old path no longer exists
	if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
		t.Error("old path should be removed")
	}

	// Verify new path exists: {siteID}/{deployID}
	newPath := store.GetDeploymentPath(s.ID, dep.ID)
	if _, err := os.Stat(newPath); err != nil {
		t.Errorf("new path should exist: %v", err)
	}

	// Verify file was moved
	newFile := filepath.Join(newPath, "index.html")
	data, err := os.ReadFile(newFile)
	if err != nil {
		t.Fatalf("read migrated file: %v", err)
	}
	if string(data) != "test" {
		t.Errorf("file content = %q, want test", string(data))
	}
}

func TestMigrateDeployPaths_MigratesAllDeployments(t *testing.T) {
	db, store := setupTestDBForMigration(t)

	u := models.User{Email: "user@test.com", PasswordHash: "hash"}
	db.Create(&u)

	activeVersion := 2
	s := models.Site{
		UserID:        u.ID,
		SubdomainSlug: "test",
		Name:          "Test",
		ActiveVersion: &activeVersion,
	}
	db.Create(&s)

	// Create multiple deployments
	dep1 := models.Deployment{SiteID: s.ID, Version: 1, FileHash: "hash1"}
	dep2 := models.Deployment{SiteID: s.ID, Version: 2, FileHash: "hash2"}
	dep3 := models.Deployment{SiteID: s.ID, Version: 3, FileHash: "hash3"}
	db.Create(&dep1)
	db.Create(&dep2)
	db.Create(&dep3)

	// Create old-style directories for all deployments
	for i := 1; i <= 3; i++ {
		oldPath := filepath.Join(store.BasePath, s.ID, strconv.Itoa(i))
		if err := os.MkdirAll(oldPath, 0755); err != nil {
			t.Fatalf("create old path %d: %v", i, err)
		}
		if err := os.WriteFile(filepath.Join(oldPath, "file.txt"), []byte("v"+strconv.Itoa(i)), 0644); err != nil {
			t.Fatalf("write file %d: %v", i, err)
		}
	}

	if err := MigrateDeployPaths(db, store); err != nil {
		t.Fatalf("MigrateDeployPaths: %v", err)
	}

	// Verify all deployments were migrated to new paths
	deps := []models.Deployment{dep1, dep2, dep3}
	for i, dep := range deps {
		newPath := store.GetDeploymentPath(s.ID, dep.ID)
		data, err := os.ReadFile(filepath.Join(newPath, "file.txt"))
		if err != nil {
			t.Errorf("deployment %d not migrated: %v", i+1, err)
			continue
		}
		expected := "v" + strconv.Itoa(i+1)
		if string(data) != expected {
			t.Errorf("deployment %d content = %q, want %q", i+1, string(data), expected)
		}
	}
}

func TestMigrateDeployPaths_NoDeploymentForActiveVersion(t *testing.T) {
	db, store := setupTestDBForMigration(t)

	u := models.User{Email: "user@test.com", PasswordHash: "hash"}
	db.Create(&u)

	version := 99
	s := models.Site{
		UserID:        u.ID,
		SubdomainSlug: "test",
		Name:          "Test",
		ActiveVersion: &version,
	}
	db.Create(&s)

	// No deployment exists for version 99 - should be skipped gracefully
	if err := MigrateDeployPaths(db, store); err != nil {
		t.Fatalf("MigrateDeployPaths: %v", err)
	}

	// Verify site was not updated
	var site models.Site
	db.First(&site, "id = ?", s.ID)
	if site.ActiveDeployID != nil {
		t.Error("ActiveDeployID should remain nil when deployment not found")
	}
}

func TestMigrateDeployPaths_OldPathDoesNotExist(t *testing.T) {
	db, store := setupTestDBForMigration(t)

	u := models.User{Email: "user@test.com", PasswordHash: "hash"}
	db.Create(&u)

	version := 1
	s := models.Site{
		UserID:        u.ID,
		SubdomainSlug: "test",
		Name:          "Test",
		ActiveVersion: &version,
	}
	db.Create(&s)

	dep := models.Deployment{
		SiteID:   s.ID,
		Version:  version,
		FileHash: "abc123",
	}
	db.Create(&dep)

	// Don't create old directory - migration should still backfill active_deploy_id
	if err := MigrateDeployPaths(db, store); err != nil {
		t.Fatalf("MigrateDeployPaths: %v", err)
	}

	// Verify active_deploy_id was still set even though no directory exists
	var site models.Site
	db.First(&site, "id = ?", s.ID)
	if site.ActiveDeployID == nil {
		t.Fatal("expected ActiveDeployID to be set")
	}
	if *site.ActiveDeployID != dep.ID {
		t.Errorf("ActiveDeployID = %s, want %s", *site.ActiveDeployID, dep.ID)
	}
}

func TestMigrateDeployPaths_MultipleSites(t *testing.T) {
	db, store := setupTestDBForMigration(t)

	u := models.User{Email: "user@test.com", PasswordHash: "hash"}
	db.Create(&u)

	// Create multiple sites
	version1 := 1
	s1 := models.Site{UserID: u.ID, SubdomainSlug: "site1", Name: "Site 1", ActiveVersion: &version1}
	db.Create(&s1)
	dep1 := models.Deployment{SiteID: s1.ID, Version: version1, FileHash: "hash1"}
	db.Create(&dep1)

	version2 := 1
	s2 := models.Site{UserID: u.ID, SubdomainSlug: "site2", Name: "Site 2", ActiveVersion: &version2}
	db.Create(&s2)
	dep2 := models.Deployment{SiteID: s2.ID, Version: version2, FileHash: "hash2"}
	db.Create(&dep2)

	// Create old paths for both sites
	oldPath1 := filepath.Join(store.BasePath, s1.ID, "1")
	oldPath2 := filepath.Join(store.BasePath, s2.ID, "1")
	os.MkdirAll(oldPath1, 0755)
	os.MkdirAll(oldPath2, 0755)
	os.WriteFile(filepath.Join(oldPath1, "f1.txt"), []byte("site1"), 0644)
	os.WriteFile(filepath.Join(oldPath2, "f2.txt"), []byte("site2"), 0644)

	if err := MigrateDeployPaths(db, store); err != nil {
		t.Fatalf("MigrateDeployPaths: %v", err)
	}

	// Verify both sites were migrated
	var site1, site2 models.Site
	db.First(&site1, "id = ?", s1.ID)
	db.First(&site2, "id = ?", s2.ID)

	if site1.ActiveDeployID == nil || *site1.ActiveDeployID != dep1.ID {
		t.Error("site1 ActiveDeployID not set correctly")
	}
	if site2.ActiveDeployID == nil || *site2.ActiveDeployID != dep2.ID {
		t.Error("site2 ActiveDeployID not set correctly")
	}

	// Verify files migrated
	newPath1 := store.GetDeploymentPath(s1.ID, dep1.ID)
	newPath2 := store.GetDeploymentPath(s2.ID, dep2.ID)
	data1, _ := os.ReadFile(filepath.Join(newPath1, "f1.txt"))
	data2, _ := os.ReadFile(filepath.Join(newPath2, "f2.txt"))
	if string(data1) != "site1" {
		t.Errorf("site1 file content = %q", string(data1))
	}
	if string(data2) != "site2" {
		t.Errorf("site2 file content = %q", string(data2))
	}
}

func TestMigrateDeployPaths_Idempotent(t *testing.T) {
	db, store := setupTestDBForMigration(t)

	u := models.User{Email: "user@test.com", PasswordHash: "hash"}
	db.Create(&u)

	version := 1
	s := models.Site{
		UserID:        u.ID,
		SubdomainSlug: "test",
		Name:          "Test",
		ActiveVersion: &version,
	}
	db.Create(&s)

	dep := models.Deployment{
		SiteID:   s.ID,
		Version:  version,
		FileHash: "abc123",
	}
	db.Create(&dep)

	oldPath := filepath.Join(store.BasePath, s.ID, "1")
	os.MkdirAll(oldPath, 0755)
	os.WriteFile(filepath.Join(oldPath, "test.txt"), []byte("data"), 0644)

	// First migration
	if err := MigrateDeployPaths(db, store); err != nil {
		t.Fatalf("first MigrateDeployPaths: %v", err)
	}

	// Second migration should be safe (no-op)
	if err := MigrateDeployPaths(db, store); err != nil {
		t.Fatalf("second MigrateDeployPaths: %v", err)
	}

	// Verify data intact
	newPath := store.GetDeploymentPath(s.ID, dep.ID)
	data, _ := os.ReadFile(filepath.Join(newPath, "test.txt"))
	if string(data) != "data" {
		t.Errorf("file content changed after second migration: %q", string(data))
	}
}

func TestMigrateDeployPaths_EmptyActiveDeployID(t *testing.T) {
	db, store := setupTestDBForMigration(t)

	u := models.User{Email: "user@test.com", PasswordHash: "hash"}
	db.Create(&u)

	version := 1
	emptyStr := ""
	s := models.Site{
		UserID:         u.ID,
		SubdomainSlug:  "test",
		Name:           "Test",
		ActiveVersion:  &version,
		ActiveDeployID: &emptyStr,
	}
	db.Create(&s)

	dep := models.Deployment{
		SiteID:   s.ID,
		Version:  version,
		FileHash: "abc123",
	}
	db.Create(&dep)

	// Empty string should be treated as needing migration
	if err := MigrateDeployPaths(db, store); err != nil {
		t.Fatalf("MigrateDeployPaths: %v", err)
	}

	// Verify active_deploy_id was set
	var site models.Site
	db.First(&site, "id = ?", s.ID)
	if site.ActiveDeployID == nil || *site.ActiveDeployID == "" {
		t.Error("expected non-empty ActiveDeployID")
	}
	if *site.ActiveDeployID != dep.ID {
		t.Errorf("ActiveDeployID = %s, want %s", *site.ActiveDeployID, dep.ID)
	}
}
