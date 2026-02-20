package config

import (
	"fmt"
	"os"
	"runtime"
	"strings"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Domain        string              `yaml:"domain"`
	Listen        string              `yaml:"listen"`
	StoragePath   string              `yaml:"storage_path"`
	JWTSecret     string              `yaml:"jwt_secret"`
	MinCLIVersion string              `yaml:"min_cli_version"`
	Database      DBConfig            `yaml:"database"`
	Registration  RegConfig           `yaml:"registration"`
	Cloudflare    CFConfig            `yaml:"cloudflare"`
	Worker        WorkerConfig        `yaml:"worker"`
	ObjectStorage ObjectStorageConfig `yaml:"object_storage"`
}

type ObjectStorageConfig struct {
	Enabled    bool                    `yaml:"enabled"`
	Managed    bool                    `yaml:"managed"`
	DataDir    string                  `yaml:"data_dir"`
	BinaryPath string                  `yaml:"binary_path"`
	S3Endpoint string                  `yaml:"s3_endpoint"`
	Region     string                  `yaml:"region"`
	Auth       ObjectStorageAuthConfig `yaml:"auth"`
}

type ObjectStorageAuthConfig struct {
	AccessKeyID     string `yaml:"access_key_id"`
	SecretAccessKey string `yaml:"secret_access_key"`
	RequireSigV4    *bool  `yaml:"require_sigv4"`
}

type WorkerConfig struct {
	PoolSize         int    `yaml:"pool_size"`
	MemoryLimitMB    int    `yaml:"memory_limit_mb"`
	ExecutionTimeout int    `yaml:"execution_timeout"`
	MaxFetchRequests int    `yaml:"max_fetch_requests"`
	FetchTimeoutSec  int    `yaml:"fetch_timeout_sec"`
	MaxResponseBytes int    `yaml:"max_response_bytes"`
	MaxLogRetention  int    `yaml:"max_log_retention"`
	MaxScriptSizeKB  int    `yaml:"max_script_size_kb"`
	DataDir          string `yaml:"data_dir"`
}

type DBConfig struct {
	Driver string `yaml:"driver"`
	DSN    string `yaml:"dsn"`
}

type RegConfig struct {
	Enabled        bool `yaml:"enabled"`
	InviteRequired bool `yaml:"invite_required"`
}

type CFConfig struct {
	APIToken string `yaml:"api_token"`
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config file: %w", err)
	}

	cfg := &Config{}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parsing config file: %w", err)
	}

	// Defaults
	if cfg.Listen == "" {
		cfg.Listen = ":8080"
	}
	if cfg.Database.Driver == "" {
		cfg.Database.Driver = "sqlite"
	}
	if cfg.StoragePath == "" {
		cfg.StoragePath = "./data/sites"
	}
	if cfg.Worker.PoolSize == 0 {
		cfg.Worker.PoolSize = 4
	}
	if cfg.Worker.MemoryLimitMB == 0 {
		cfg.Worker.MemoryLimitMB = 128
	}
	if cfg.Worker.ExecutionTimeout == 0 {
		cfg.Worker.ExecutionTimeout = 30000
	}
	if cfg.Worker.MaxFetchRequests == 0 {
		cfg.Worker.MaxFetchRequests = 50
	}
	if cfg.Worker.FetchTimeoutSec == 0 {
		cfg.Worker.FetchTimeoutSec = 10
	}
	if cfg.Worker.MaxResponseBytes == 0 {
		cfg.Worker.MaxResponseBytes = 10 << 20 // 10 MB
	}
	if cfg.Worker.MaxLogRetention == 0 {
		cfg.Worker.MaxLogRetention = 7
	}
	if cfg.Worker.MaxScriptSizeKB == 0 {
		cfg.Worker.MaxScriptSizeKB = 1024
	}

	// Object storage defaults
	if cfg.ObjectStorage.Enabled {
		if cfg.ObjectStorage.S3Endpoint == "" {
			cfg.ObjectStorage.S3Endpoint = "http://127.0.0.1:8333"
		}
		if cfg.ObjectStorage.Managed && cfg.ObjectStorage.DataDir == "" {
			cfg.ObjectStorage.DataDir = "./data/seaweedfs"
		}
		if cfg.ObjectStorage.Managed && cfg.ObjectStorage.BinaryPath == "" {
			if runtime.GOOS == "windows" {
				cfg.ObjectStorage.BinaryPath = "./weed.exe"
			} else {
				cfg.ObjectStorage.BinaryPath = "weed"
			}
		}
		if cfg.ObjectStorage.Region == "" {
			cfg.ObjectStorage.Region = "us-east-1"
		}
		if cfg.ObjectStorage.Auth.RequireSigV4 == nil {
			requireSigV4 := true
			cfg.ObjectStorage.Auth.RequireSigV4 = &requireSigV4
		}

		hasAccessKey := strings.TrimSpace(cfg.ObjectStorage.Auth.AccessKeyID) != ""
		hasSecretKey := strings.TrimSpace(cfg.ObjectStorage.Auth.SecretAccessKey) != ""
		if hasAccessKey != hasSecretKey {
			return nil, fmt.Errorf("config: object_storage.auth.access_key_id and object_storage.auth.secret_access_key must be set together")
		}
	}

	// Validation
	if cfg.Domain == "" {
		return nil, fmt.Errorf("config: domain is required")
	}
	if cfg.Database.DSN == "" {
		return nil, fmt.Errorf("config: database.dsn is required")
	}
	if len(cfg.JWTSecret) < 32 {
		return nil, fmt.Errorf("config: jwt_secret must be at least 32 characters")
	}
	if cfg.MinCLIVersion != "" {
		if _, err := ParseSemver(cfg.MinCLIVersion); err != nil {
			return nil, fmt.Errorf("config: invalid min_cli_version: %w", err)
		}
	}

	return cfg, nil
}
