package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Domain      string     `yaml:"domain"`
	Listen      string     `yaml:"listen"`
	StoragePath string     `yaml:"storage_path"`
	JWTSecret   string     `yaml:"jwt_secret"`
	Database    DBConfig   `yaml:"database"`
	Registration RegConfig `yaml:"registration"`
	Cloudflare  CFConfig   `yaml:"cloudflare"`
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

	// Validation
	if cfg.Domain == "" {
		return nil, fmt.Errorf("config: domain is required")
	}
	if cfg.Database.DSN == "" {
		return nil, fmt.Errorf("config: database.dsn is required")
	}
	if cfg.JWTSecret == "" {
		return nil, fmt.Errorf("config: jwt_secret is required")
	}

	return cfg, nil
}
