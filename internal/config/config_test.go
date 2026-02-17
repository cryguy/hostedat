package config

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func writeTemp(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("writing temp config: %v", err)
	}
	return path
}

func TestLoad_FullConfig(t *testing.T) {
	cfg, err := Load(writeTemp(t, `
domain: example.com
listen: ":9090"
storage_path: /data/custom
jwt_secret: this-is-a-test-secret-that-is-at-least-32-chars-long
database:
  driver: sqlite
  dsn: /data/test.db
registration:
  enabled: true
  invite_required: true
cloudflare:
  api_token: cftoken
`))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Domain != "example.com" {
		t.Errorf("Domain = %q", cfg.Domain)
	}
	if cfg.Listen != ":9090" {
		t.Errorf("Listen = %q", cfg.Listen)
	}
	if cfg.StoragePath != "/data/custom" {
		t.Errorf("StoragePath = %q", cfg.StoragePath)
	}
	if cfg.JWTSecret != "this-is-a-test-secret-that-is-at-least-32-chars-long" {
		t.Errorf("JWTSecret = %q", cfg.JWTSecret)
	}
	if cfg.Database.Driver != "sqlite" {
		t.Errorf("Driver = %q", cfg.Database.Driver)
	}
	if cfg.Database.DSN != "/data/test.db" {
		t.Errorf("DSN = %q", cfg.Database.DSN)
	}
	if !cfg.Registration.Enabled {
		t.Error("Registration.Enabled = false")
	}
	if !cfg.Registration.InviteRequired {
		t.Error("Registration.InviteRequired = false")
	}
	if cfg.Cloudflare.APIToken != "cftoken" {
		t.Errorf("Cloudflare.APIToken = %q", cfg.Cloudflare.APIToken)
	}
}

func TestLoad_Defaults(t *testing.T) {
	cfg, err := Load(writeTemp(t, `
domain: example.com
jwt_secret: this-is-a-test-secret-that-is-at-least-32-chars-long
database:
  dsn: test.db
`))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Listen != ":8080" {
		t.Errorf("Listen default = %q, want :8080", cfg.Listen)
	}
	if cfg.Database.Driver != "sqlite" {
		t.Errorf("Driver default = %q, want sqlite", cfg.Database.Driver)
	}
	if cfg.StoragePath != "./data/sites" {
		t.Errorf("StoragePath default = %q, want ./data/sites", cfg.StoragePath)
	}
}

func TestLoad_MissingDomain(t *testing.T) {
	_, err := Load(writeTemp(t, `
jwt_secret: this-is-a-test-secret-that-is-at-least-32-chars-long
database:
  dsn: test.db
`))
	if err == nil || !strings.Contains(err.Error(), "domain") {
		t.Fatalf("expected domain error, got %v", err)
	}
}

func TestLoad_MissingDSN(t *testing.T) {
	_, err := Load(writeTemp(t, `
domain: example.com
jwt_secret: this-is-a-test-secret-that-is-at-least-32-chars-long
`))
	if err == nil || !strings.Contains(err.Error(), "dsn") {
		t.Fatalf("expected dsn error, got %v", err)
	}
}

func TestLoad_MissingJWTSecret(t *testing.T) {
	_, err := Load(writeTemp(t, `
domain: example.com
database:
  dsn: test.db
`))
	if err == nil || !strings.Contains(err.Error(), "jwt_secret") {
		t.Fatalf("expected jwt_secret error, got %v", err)
	}
}

func TestLoad_ShortJWTSecret(t *testing.T) {
	_, err := Load(writeTemp(t, `
domain: example.com
jwt_secret: tooshort
database:
  dsn: test.db
`))
	if err == nil || !strings.Contains(err.Error(), "jwt_secret") {
		t.Fatalf("expected jwt_secret length error, got %v", err)
	}
}

func TestLoad_FileNotFound(t *testing.T) {
	_, err := Load("/nonexistent/config.yaml")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestLoad_InvalidYAML(t *testing.T) {
	_, err := Load(writeTemp(t, `{{{invalid`))
	if err == nil {
		t.Fatal("expected error for invalid YAML")
	}
}

func TestLoad_RegistrationDefaults(t *testing.T) {
	cfg, err := Load(writeTemp(t, `
domain: example.com
jwt_secret: this-is-a-test-secret-that-is-at-least-32-chars-long
database:
  dsn: test.db
`))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Registration.Enabled {
		t.Error("Registration.Enabled should default to false")
	}
	if cfg.Registration.InviteRequired {
		t.Error("Registration.InviteRequired should default to false")
	}
}

func TestLoad_ObjectStorage_DefaultsWhenEnabled(t *testing.T) {
	cfg, err := Load(writeTemp(t, `
domain: example.com
jwt_secret: this-is-a-test-secret-that-is-at-least-32-chars-long
database:
  dsn: test.db
object_storage:
  enabled: true
  managed: true
`))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if cfg.ObjectStorage.S3Endpoint != "http://127.0.0.1:8333" {
		t.Errorf("S3Endpoint default = %q", cfg.ObjectStorage.S3Endpoint)
	}
	if cfg.ObjectStorage.DataDir != "./data/seaweedfs" {
		t.Errorf("DataDir default = %q", cfg.ObjectStorage.DataDir)
	}
	if cfg.ObjectStorage.Region != "us-east-1" {
		t.Errorf("Region default = %q", cfg.ObjectStorage.Region)
	}
	if cfg.ObjectStorage.Auth.RequireSigV4 == nil || !*cfg.ObjectStorage.Auth.RequireSigV4 {
		t.Error("RequireSigV4 should default to true")
	}
	if runtime.GOOS == "windows" {
		if cfg.ObjectStorage.BinaryPath != "./weed.exe" {
			t.Errorf("BinaryPath default = %q", cfg.ObjectStorage.BinaryPath)
		}
	} else if cfg.ObjectStorage.BinaryPath != "weed" {
		t.Errorf("BinaryPath default = %q", cfg.ObjectStorage.BinaryPath)
	}
}

func TestLoad_ObjectStorage_AuthConfig(t *testing.T) {
	cfg, err := Load(writeTemp(t, `
domain: example.com
jwt_secret: this-is-a-test-secret-that-is-at-least-32-chars-long
database:
  dsn: test.db
object_storage:
  enabled: true
  auth:
    access_key_id: test-access
    secret_access_key: test-secret
`))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.ObjectStorage.Auth.AccessKeyID != "test-access" {
		t.Errorf("AccessKeyID = %q", cfg.ObjectStorage.Auth.AccessKeyID)
	}
	if cfg.ObjectStorage.Auth.SecretAccessKey != "test-secret" {
		t.Errorf("SecretAccessKey = %q", cfg.ObjectStorage.Auth.SecretAccessKey)
	}
}

func TestLoad_ObjectStorage_InvalidPartialAuthConfig(t *testing.T) {
	_, err := Load(writeTemp(t, `
domain: example.com
jwt_secret: this-is-a-test-secret-that-is-at-least-32-chars-long
database:
  dsn: test.db
object_storage:
  enabled: true
  auth:
    access_key_id: only-access-key
`))
	if err == nil || !strings.Contains(err.Error(), "object_storage.auth.access_key_id") {
		t.Fatalf("expected auth pair validation error, got %v", err)
	}
}

func TestLoad_ObjectStorage_RequireSigV4CanBeDisabled(t *testing.T) {
	cfg, err := Load(writeTemp(t, `
domain: example.com
jwt_secret: this-is-a-test-secret-that-is-at-least-32-chars-long
database:
  dsn: test.db
object_storage:
  enabled: true
  auth:
    require_sigv4: false
`))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.ObjectStorage.Auth.RequireSigV4 == nil {
		t.Fatal("RequireSigV4 should not be nil")
	}
	if *cfg.ObjectStorage.Auth.RequireSigV4 {
		t.Fatal("RequireSigV4 should remain false when explicitly configured")
	}
}
