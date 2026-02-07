package main

import (
	"flag"
	"log"
	"os"
	"path/filepath"

	"github.com/cryguy/hostedat/internal/config"
	"github.com/cryguy/hostedat/internal/models"
)

func main() {
	configPath := flag.String("config", "config.yaml", "path to config file")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Ensure data directory exists for SQLite
	if cfg.Database.Driver == "sqlite" {
		dir := filepath.Dir(cfg.Database.DSN)
		if err := os.MkdirAll(dir, 0755); err != nil {
			log.Fatalf("Failed to create data directory: %v", err)
		}
	}

	// Ensure storage directory exists
	if err := os.MkdirAll(cfg.StoragePath, 0755); err != nil {
		log.Fatalf("Failed to create storage directory: %v", err)
	}

	db, err := models.InitDB(cfg.Database)
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}

	if err := models.SeedDefaults(db, cfg); err != nil {
		log.Fatalf("Failed to seed defaults: %v", err)
	}

	_ = db
	log.Printf("Server ready — domain=%s listen=%s", cfg.Domain, cfg.Listen)
}
