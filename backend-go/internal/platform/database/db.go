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

	if err := runDatabaseMigrations(db); err != nil {
		return fmt.Errorf("run database migrations: %w", err)
	}

	logging.Infof("Database initialized successfully")
	return nil
}

func Migrate() error {
	if DB == nil {
		return fmt.Errorf("database not initialized")
	}

	return autoMigrateModels(DB)
}

func EnsureTables() error {
	if DB == nil {
		return fmt.Errorf("database not initialized")
	}

	return RunMigrations(DB)
}
