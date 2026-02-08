package models

import (
	"fmt"
	"strconv"

	"github.com/cryguy/hostedat/internal/config"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func InitDB(cfg config.DBConfig) (*gorm.DB, error) {
	var db *gorm.DB
	var err error

	switch cfg.Driver {
	case "sqlite":
		dsn := cfg.DSN + "?_journal_mode=WAL&_busy_timeout=5000"
		db, err = gorm.Open(sqlite.Open(dsn), &gorm.Config{
			Logger: logger.Default.LogMode(logger.Warn),
		})
	default:
		return nil, fmt.Errorf("unsupported database driver: %s", cfg.Driver)
	}

	if err != nil {
		return nil, fmt.Errorf("opening database: %w", err)
	}

	if err := db.AutoMigrate(
		&User{},
		&Site{},
		&Deployment{},
		&APIKey{},
		&Invite{},
		&AuthCode{},
		&Setting{},
	); err != nil {
		return nil, fmt.Errorf("auto-migrating: %w", err)
	}

	return db, nil
}

func SeedDefaults(db *gorm.DB, cfg *config.Config) error {
	var count int64
	db.Model(&Setting{}).Count(&count)
	if count > 0 {
		return nil
	}

	settings := []Setting{
		{Key: "registration_enabled", Value: strconv.FormatBool(cfg.Registration.Enabled)},
		{Key: "invite_required", Value: strconv.FormatBool(cfg.Registration.InviteRequired)},
	}

	return db.Create(&settings).Error
}

func GetSetting(db *gorm.DB, key string) (string, error) {
	var s Setting
	if err := db.Where("key = ?", key).First(&s).Error; err != nil {
		return "", err
	}
	return s.Value, nil
}

func SetSetting(db *gorm.DB, key, value string) error {
	return db.Save(&Setting{Key: key, Value: value}).Error
}
