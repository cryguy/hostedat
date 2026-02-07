package main

import (
	"flag"
	"fmt"
	"log"

	"github.com/cryguy/hostedat/internal/config"
)

func main() {
	configPath := flag.String("config", "config.yaml", "path to config file")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	fmt.Printf("Loaded config for domain: %s\n", cfg.Domain)
	fmt.Printf("Listen: %s\n", cfg.Listen)
	fmt.Printf("Storage: %s\n", cfg.StoragePath)
	fmt.Printf("DB Driver: %s, DSN: %s\n", cfg.Database.Driver, cfg.Database.DSN)
	fmt.Printf("Registration: enabled=%v, invite_required=%v\n", cfg.Registration.Enabled, cfg.Registration.InviteRequired)
}
