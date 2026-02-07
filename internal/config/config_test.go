package config

import (
	"os"
	"path/filepath"
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
jwt_secret: supersecret
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
	if cfg.JWTSecret != "supersecret" {
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
jwt_secret: secret
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
jwt_secret: s
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
jwt_secret: s
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
jwt_secret: s
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
