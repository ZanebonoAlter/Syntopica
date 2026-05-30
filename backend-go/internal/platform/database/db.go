package database

import (
	"fmt"
	"time"

	"gorm.io/gorm"
	"syntopica-backend/internal/platform/config"
	"syntopica-backend/internal/platform/logging"
)

var DB *gorm.DB

var openPostgres = connectPostgres
var runDatabaseMigrations = RunMigrations

func InitDB(cfg *config.Config) error {
	cstZone := time.FixedZone("CST", 8*3600)
	gormCfg := &gorm.Config{
		Logger: NewSlowLogger(200 * time.Millisecond),
		NowFunc: func() time.Time {
			return time.Now().In(cstZone)
		},
	}

	db, err := openPostgres(cfg, gormCfg)
	if err != nil {
		return err
	}

	DB = db

	// Phase 1: AutoMigrate — syncs all model tables/columns on every startup.
	// Handles ADD COLUMN and CREATE TABLE automatically. Never drops or alters.
	if err := RunAutoMigrate(db); err != nil {
		logging.Warnf("AutoMigrate warning (non-fatal): %v", err)
	}

	// Phase 2: Versioned migrations — for operations AutoMigrate can't handle:
	// extensions (pgvector), indexes, triggers, data migrations, column drops.
	if err := runDatabaseMigrations(db); err != nil {
		return fmt.Errorf("run database migrations: %w", err)
	}

	logging.Infof("Database initialized successfully")
	return nil
}

// Deprecated: auto-migration now runs automatically in InitDB.
func Migrate() error {
	if DB == nil {
		return fmt.Errorf("database not initialized")
	}
	return RunAutoMigrate(DB)
}

// Deprecated: use InitDB which handles both auto-migration and versioned migrations.
func EnsureTables() error {
	if DB == nil {
		return fmt.Errorf("database not initialized")
	}
	return RunMigrations(DB)
}
