package database

import (
	"fmt"
	"sort"

	"gorm.io/gorm"

	"my-robot-backend/internal/domain/models"
)

type Migration struct {
	Version     string
	Description string
	Up          func(db *gorm.DB) error
}

func RunMigrations(db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("database connection is required")
	}

	if err := ensureSchemaMigrationsTable(db); err != nil {
		return err
	}

	appliedVersions, err := loadAppliedMigrationVersions(db)
	if err != nil {
		return err
	}

	for _, migration := range migrationsSorted() {
		if appliedVersions[migration.Version] {
			continue
		}

		if err := db.Transaction(func(tx *gorm.DB) error {
			if err := migration.Up(tx); err != nil {
				return fmt.Errorf("apply migration %s: %w", migration.Version, err)
			}

			if err := tx.Exec(
				"INSERT INTO schema_migrations (version, driver) VALUES (?, 'postgres')",
				migration.Version,
			).Error; err != nil {
				return fmt.Errorf("record migration %s: %w", migration.Version, err)
			}

			return nil
		}); err != nil {
			return err
		}
	}

	return nil
}

func migrationsSorted() []Migration {
	migrations := postgresMigrations()
	sort.Slice(migrations, func(i, j int) bool {
		return migrations[i].Version < migrations[j].Version
	})
	return migrations
}

func ensureSchemaMigrationsTable(db *gorm.DB) error {
	if err := db.Exec(`
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version VARCHAR(255) NOT NULL,
			driver VARCHAR(32) NOT NULL DEFAULT 'postgres',
			applied_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			PRIMARY KEY (driver, version)
		)
	`).Error; err != nil {
		return fmt.Errorf("ensure schema_migrations table: %w", err)
	}

	return nil
}

func loadAppliedMigrationVersions(db *gorm.DB) (map[string]bool, error) {
	var versions []string
	if err := db.Raw("SELECT version FROM schema_migrations").Scan(&versions).Error; err != nil {
		return nil, fmt.Errorf("load applied migrations: %w", err)
	}

	applied := make(map[string]bool, len(versions))
	for _, version := range versions {
		applied[version] = true
	}

	return applied, nil
}

// autoMigrateModels uses GORM AutoMigrate to sync all model tables.
// CAVEAT: This only runs inside bootstrap migration 20260403_0002 (first-time setup).
// For any column additions AFTER the initial bootstrap, you MUST add a versioned
// migration in postgres_migrations.go — AutoMigrate will NOT be re-run on an
// existing database, so new fields will silently be missing until a migration adds them.
// (This was the old SQLite-era approach; PostgreSQL relies on explicit migrations.)
func autoMigrateModels(db *gorm.DB) error {
	return db.AutoMigrate(
		&models.Category{},
		&models.Feed{},
		&models.Article{},
		&models.TopicTag{},
		&models.TopicTagEmbedding{},
		&models.TopicTagAnalysis{},
		&models.TopicAnalysisCursor{},
		&models.ArticleTopicTag{},
		&models.SchedulerTask{},
		&models.AISettings{},
		&models.AIProvider{},
		&models.AIRoute{},
		&models.AIRouteProvider{},
		&models.AICallLog{},
		&models.ReadingBehavior{},
		&models.UserPreference{},
		&models.FirecrawlJob{},
		&models.TagJob{},
		&models.NarrativeSummary{},
		&models.NarrativeBoard{},
		&models.AbstractTagUpdateQueue{},
		&models.HierarchyConfig{},
		&models.HierarchyConfigVersion{},
	)
}
