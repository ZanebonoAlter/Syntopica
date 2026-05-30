package database

import (
	"fmt"
	"sort"

	"gorm.io/gorm"

	"syntopica-backend/internal/domain/models"
)

type Migration struct {
	Version     string
	Description string
	Up          func(db *gorm.DB) error
}

// extraModels holds domain-specific models registered via RegisterModels.
// This avoids circular imports — domain packages (e.g. daily_report) register
// their models via init(), and migrator picks them up during startup.
var extraModels []any

// RegisterModels registers additional GORM models for AutoMigrate.
// Call from domain package init() functions.
func RegisterModels(models ...any) {
	extraModels = append(extraModels, models...)
}

// RunAutoMigrate syncs all model tables via GORM AutoMigrate.
// Runs on every startup — adds missing tables/columns, never drops or alters existing ones.
func RunAutoMigrate(db *gorm.DB) error {
	allModels := []any{
		&models.Category{},
		&models.Feed{},
		&models.Article{},
		&models.TopicTag{},
		&models.SemanticLabel{},
		&models.TopicTagSemanticLabel{},
		&models.TopicTagBoardLabel{},
		&models.BoardComposition{},
		&models.TopicTagEmbedding{},
		&models.TopicTagAnalysis{},
		&models.TopicAnalysisCursor{},
		&models.ArticleTopicTag{},
		&models.TopicTagRelation{},
		&models.SchedulerTask{},
		&models.AISettings{},
		&models.EmbeddingConfig{},
		&models.EmbeddingQueue{},
		&models.MergeReembeddingQueue{},
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
	}
	allModels = append(allModels, extraModels...)
	return db.AutoMigrate(allModels...)
}

// RunMigrations executes versioned migrations for operations that GORM AutoMigrate
// cannot handle: extensions, indexes, triggers, data migrations, column drops.
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

// Deprecated: use RunAutoMigrate instead.
func autoMigrateModels(db *gorm.DB) error {
	return RunAutoMigrate(db)
}
