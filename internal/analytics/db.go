package analytics

import (
	"fmt"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// InitDB opens a separate SQLite database for analytics data.
// Uses WAL mode and busy timeout for concurrent access, mirroring
// the main database configuration in models.InitDB.
func InitDB(dsn string) (*gorm.DB, error) {
	connStr := dsn + "?_journal_mode=WAL&_busy_timeout=5000"
	db, err := gorm.Open(sqlite.Open(connStr), &gorm.Config{
		Logger:                                   logger.Default.LogMode(logger.Warn),
		DisableForeignKeyConstraintWhenMigrating: true,
	})
	if err != nil {
		return nil, fmt.Errorf("opening analytics database: %w", err)
	}

	if err := db.AutoMigrate(&RequestLog{}, &HourlyStat{}); err != nil {
		if sqlDB, cerr := db.DB(); cerr == nil {
			_ = sqlDB.Close()
		}
		return nil, fmt.Errorf("migrating analytics tables: %w", err)
	}

	return db, nil
}
