package certs

import (
	"context"
	"crypto/tls"
	"fmt"

	"github.com/caddyserver/certmagic"
	"github.com/libdns/cloudflare"
)

type Config struct {
	Domain   string
	APIToken string
	DataDir  string
}

// SetupTLS configures CertMagic with Cloudflare DNS-01 challenge
// for automatic wildcard certificate management.
func SetupTLS(cfg Config) (*tls.Config, error) {
	certmagic.DefaultACME.Agreed = true

	// Use the default storage but in our data dir
	if cfg.DataDir != "" {
		certmagic.Default.Storage = &certmagic.FileStorage{Path: cfg.DataDir}
	}

	// Configure Cloudflare DNS-01 solver
	solver := &certmagic.DNS01Solver{
		DNSManager: certmagic.DNSManager{
			DNSProvider: &cloudflare.Provider{
				APIToken: cfg.APIToken,
			},
		},
	}

	certmagic.DefaultACME.DNS01Solver = solver

	// Manage both the base domain and wildcard
	domains := []string{cfg.Domain, "*." + cfg.Domain}

	magic := certmagic.NewDefault()
	if err := magic.ManageSync(context.TODO(), domains); err != nil {
		return nil, fmt.Errorf("managing certificates: %w", err)
	}

	return magic.TLSConfig(), nil
}
