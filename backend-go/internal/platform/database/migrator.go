package database

import (
	"fmt"
	"sort"

	"gorm.io/gorm"

	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/config"
	"syntopica-backend/internal/platform/logging"
)

type Migration struct {
	Version     string
	Description string
	Up          func(db *gorm.DB) error
	// RunOutsideTx controls whether Up runs outside an outer transaction.
	// The zero value (false) keeps the default in-transaction behavior: Up +
	// version-record share one transaction (atomic apply/record, roll back both
	// on failure). Set true for migrations whose Up uses transaction-incompatible
	// DDL such as CREATE INDEX CONCURRENTLY — Up runs on the bare *gorm.DB, and
	// on success the version is recorded with a separate non-transactional
	// INSERT. An outside-tx Up failure is NOT recorded, so the next startup
	// retries (mirroring the in-tx path's rollback semantics). Only use
	// RunOutsideTx=true for a single transaction-incompatible statement; multi-
	// step migrations that need atomicity must stay in-transaction.
	RunOutsideTx bool
	// Down declares this migration's rollback logic; nil (the zero value) means
	// the migration is irreversible. This is a declarative placeholder only —
	// no executor is implemented yet (no CLI/HTTP rollback entry point exists,
	// per AGENTS.md "Simplicity First"). Destructive migrations should leave
	// Down nil and instead annotate irreversibility in Description.
	Down func(db *gorm.DB) error
}

// allowDestructive controls whether destructive migrations (TRUNCATE/DROP) run.
// Initialized in RunMigrations from config.AppConfig.Database.AllowDestructiveMigrations
// (env MIGRATIONS_ALLOW_DESTRUCTIVE=1). Defaults to false (production-safe).
// Migrations read it via IsDestructiveAllowed() to self-guard destructive operations.
var allowDestructive bool

// IsDestructiveAllowed reports whether destructive migrations are permitted.
// Call at the top of a migration Up closure to self-guard TRUNCATE/DROP operations.
func IsDestructiveAllowed() bool {
	return allowDestructive
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
	// Pre-AutoMigrate fixups for legacy column types GORM cannot ALTER itself
	// (jsonb→bytea has no implicit cast; AutoMigrate would fail startup).
	if err := preMigrateEmbeddingCacheBytea(db); err != nil {
		return err
	}
	allModels := []any{
		&models.Category{},
		&models.Feed{},
		&models.Article{},
		&models.TopicTag{},
		&models.SemanticLabel{},
		&models.TopicTagSemanticLabel{},
		&models.TopicTagBoardLabel{},
		&models.BoardComposition{},
		&models.CompositeComponent{},
		&models.BoardUpgradeSuggestion{},
		&models.TopicTagEmbedding{},
		&models.TopicTagAnalysis{},
		&models.TopicAnalysisCursor{},
		&models.ArticleTopicTag{},
		&models.TagMergeSuggestion{},
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
		&models.AIEmbeddingCache{},
		&models.ReadingBehavior{},
		// preference-vector-feed-discovery: 偏好向量 / RSSHub 路由目录 / 订阅源推荐
		&models.PreferenceVector{},
		&models.RSSHubRoute{},
		&models.RouteParamOption{},
		&models.RouteEmbedding{},
		&models.FeedRecommendation{},
		&models.FirecrawlJob{},
		&models.TagJob{},
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

	// Initialize the destructive-migration gate from config. Production never sets
	// MIGRATIONS_ALLOW_DESTRUCTIVE, so allowDestructive stays false and destructive
	// migrations self-skip. See db-migration-safety capability.
	allowDestructive = false
	if config.AppConfig != nil {
		allowDestructive = config.AppConfig.Database.AllowDestructiveMigrations
	}
	if allowDestructive {
		logging.Warnf("Destructive migrations ENABLED (MIGRATIONS_ALLOW_DESTRUCTIVE=1): TRUNCATE/DROP migrations will execute")
	}

	return runMigrationsList(db, migrationsSorted())
}

// runMigrationsList is the core migration loop, factored out so tests can drive
// it with a hand-built migration list (e.g. a probe migration declaring
// RunOutsideTx=true) without mutating the production postgresMigrations() slice.
// It is responsible for: ensuring the schema_migrations table exists, loading
// already-applied versions, and executing each pending migration with the
// in-tx or outside-tx path chosen by Migration.RunOutsideTx.
//
// The caller (RunMigrations) is responsible for setting allowDestructive from
// config before calling, since this function must not read config itself.
func runMigrationsList(db *gorm.DB, migrations []Migration) error {
	if err := ensureSchemaMigrationsTable(db); err != nil {
		return err
	}

	appliedVersions, err := loadAppliedMigrationVersions(db)
	if err != nil {
		return err
	}

	for _, migration := range migrations {
		if appliedVersions[migration.Version] {
			continue
		}

		if migration.RunOutsideTx {
			// Outside-tx path: Up runs on the bare db (no surrounding
			// transaction), so CREATE INDEX CONCURRENTLY and other
			// transaction-incompatible DDL can succeed. On Up failure the
			// version is NOT recorded — the next startup retries. This mirrors
			// the in-tx path's rollback semantics (Up + INSERT roll back
			// together), so "a failed migration is retried" is invariant across
			// both paths. Idempotency/retry-safety is the migration's own
			// responsibility (IF NOT EXISTS / cleanup guards), not the
			// executor's.
			if err := migration.Up(db); err != nil {
				return fmt.Errorf("apply migration %s (outside tx): %w", migration.Version, err)
			}
			if err := db.Exec(
				"INSERT INTO schema_migrations (version, driver) VALUES (?, 'postgres')",
				migration.Version,
			).Error; err != nil {
				return fmt.Errorf("record migration %s: %w", migration.Version, err)
			}
			continue
		}

		// In-tx path (default, unchanged): Up + version-record share one
		// transaction — atomic apply/record, both roll back on Up failure.
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
