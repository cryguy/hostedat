package storage

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"

	"gorm.io/gorm"
)

// MigrateDeployPaths backfills ActiveDeployID for existing sites that have
// ActiveVersion set but no ActiveDeployID, and renames the on-disk deployment
// directories from numeric version paths ({siteID}/{version}) to deploy ID
// paths ({siteID}/{deployID}).
//
// This is a one-time data migration for the version→deployKey refactor.
// It is safe to run multiple times (idempotent).
func MigrateDeployPaths(db *gorm.DB, store *Manager) error {
	// siteRow matches the columns we SELECT below.
	type siteRow struct {
		ID            string
		ActiveVersion int
	}

	var sites []siteRow
	if err := db.Raw(`
		SELECT id, active_version
		FROM sites
		WHERE active_version IS NOT NULL
		  AND (active_deploy_id IS NULL OR active_deploy_id = '')
	`).Scan(&sites).Error; err != nil {
		return fmt.Errorf("querying sites for migration: %w", err)
	}

	if len(sites) == 0 {
		return nil
	}

	log.Printf("migrate: backfilling active_deploy_id for %d site(s)", len(sites))

	// deployRow matches the columns we SELECT below.
	type deployRow struct {
		ID      string
		Version int
	}

	for _, s := range sites {
		// Find the deployment record matching the active version.
		var dep deployRow
		if err := db.Raw(`
			SELECT id, version FROM deployments
			WHERE site_id = ? AND version = ?
		`, s.ID, s.ActiveVersion).Scan(&dep).Error; err != nil || dep.ID == "" {
			log.Printf("migrate: skipping site %s — no deployment found for version %d", s.ID, s.ActiveVersion)
			continue
		}

		// Rename on-disk directory: {siteID}/{version} → {siteID}/{deployID}
		oldPath := filepath.Join(store.BasePath, s.ID, strconv.Itoa(dep.Version))
		newPath := store.GetDeploymentPath(s.ID, dep.ID)

		if oldPath != newPath {
			if _, err := os.Stat(oldPath); err == nil {
				if err := os.Rename(oldPath, newPath); err != nil {
					log.Printf("migrate: failed to rename %s → %s: %v", oldPath, newPath, err)
					continue
				}
				log.Printf("migrate: renamed %s → %s", oldPath, newPath)
			}
		}

		// Update the site record.
		if err := db.Exec(`UPDATE sites SET active_deploy_id = ? WHERE id = ?`, dep.ID, s.ID).Error; err != nil {
			log.Printf("migrate: failed to update site %s: %v", s.ID, err)
			continue
		}

		// Also rename all other deployment directories for this site.
		var allDeps []deployRow
		db.Raw(`SELECT id, version FROM deployments WHERE site_id = ?`, s.ID).Scan(&allDeps)
		for _, d := range allDeps {
			if d.ID == dep.ID {
				continue // Already handled above.
			}
			op := filepath.Join(store.BasePath, s.ID, strconv.Itoa(d.Version))
			np := store.GetDeploymentPath(s.ID, d.ID)
			if op != np {
				if _, err := os.Stat(op); err == nil {
					if err := os.Rename(op, np); err != nil {
						log.Printf("migrate: failed to rename %s → %s: %v", op, np, err)
					}
				}
			}
		}
	}

	log.Printf("migrate: deploy path migration complete")
	return nil
}
