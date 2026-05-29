package database

import (
	"fmt"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"syntopica-backend/internal/platform/config"
)

func connectPostgres(cfg *config.Config, gormCfg *gorm.Config) (*gorm.DB, error) {
	dialector := postgres.New(postgres.Config{
		DSN:                  cfg.Database.DSN,
		PreferSimpleProtocol: true,
	})
	db, err := gorm.Open(dialector, gormCfg)
	if err != nil {
		return nil, fmt.Errorf("failed to connect postgres database: %w", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("failed to get postgres database instance: %w", err)
	}

	if cfg.Database.Postgres.MaxIdleConns > 0 {
		sqlDB.SetMaxIdleConns(cfg.Database.Postgres.MaxIdleConns)
	}
	if cfg.Database.Postgres.MaxOpenConns > 0 {
		sqlDB.SetMaxOpenConns(cfg.Database.Postgres.MaxOpenConns)
	}
	if cfg.Database.Postgres.ConnMaxLifetimeMinutes > 0 {
		sqlDB.SetConnMaxLifetime(time.Duration(cfg.Database.Postgres.ConnMaxLifetimeMinutes) * time.Minute)
	}
	if cfg.Database.Postgres.ConnMaxIdleTimeMinutes > 0 {
		sqlDB.SetConnMaxIdleTime(time.Duration(cfg.Database.Postgres.ConnMaxIdleTimeMinutes) * time.Minute)
	}

	return db, nil
}
