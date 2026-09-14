package database

import (
	"database/sql"
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
		// improve-discovery-recommendations: 候选实体（design D1）——先于运行账本五模型注册，
		// PG-only 回填（runDiscoveryV2Backfill）向本表回填路由候选。
		&models.FeedCandidate{},
		// improve-discovery-recommendations: 运行账本 / 兴趣记录 / 候选排除 / 候选向量（design D2/D3/D6）
		&models.DiscoveryRun{},
		&models.DiscoveryRunItem{},
		&models.DiscoveryInterestEntry{},
		&models.CandidatePreference{},
		&models.CandidateEmbedding{},
		// improve-discovery-recommendations: 候选可用性状态（design D7）
		&models.CandidateAvailability{},
		&models.FirecrawlJob{},
		&models.TagJob{},
	}
	allModels = append(allModels, extraModels...)
	if err := db.AutoMigrate(allModels...); err != nil {
		return err
	}
	// improve-discovery-recommendations：PG-only 迁移收尾（design.md Migration Plan
	// 步骤 2/3——pending 部分唯一索引切换 + 候选/关联/旧种子/冷却回填）。sqlite（模型
	// 单测）跳过；每步幂等，随每次启动重跑只补缺。
	return runDiscoveryV2Backfill(db)
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

// runDiscoveryV2Backfill runs the improve-discovery-recommendations PG-only
// migration steps (design.md Migration Plan 2/3) right after AutoMigrate:
//
//	a. 旧 pending 重复 hash 去重 + 旧 pending 转 legacy 历史；
//	b. DROP 旧 hash 全表唯一索引，建 pending 部分唯一索引；
//	c. rsshub_routes → feed_candidates 回填；
//	d. feed_recommendations.candidate_id 回填；
//	e. preference_vectors(seed) → discovery_interest_entries(legacy_inactive)；
//	f. 30 天内 dismissed → candidate_preferences 冷却回填。
//
// 每步幂等（IF NOT EXISTS / ON CONFLICT DO NOTHING / 条件 UPDATE / NOT EXISTS 挡重），
// 随每次启动重跑只补缺、不重复生成记录；非 postgres 方言（sqlite 模型单测）直接跳过。
func runDiscoveryV2Backfill(db *gorm.DB) error {
	if db.Dialector == nil || db.Name() != "postgres" {
		return nil
	}

	// a. 旧 pending 去重：同 recommendation_hash 多条 pending 时保留 id 最大者继续 pending，
	//    其余（重复组非最大 id）与全部迁移前已存在的 pending（cutoff=本步执行时 max(id)，
	//    重复组最大 id 也在此范围但被 NOT IN 豁免）转 status='legacy' 并落 expires_at，
	//    不伪装成新精排结果（design Migration Plan 3）。哨兵：部分唯一索引已存在即视为
	//    已迁移，整个 a 步跳过——新系统上线后写入的 pending 永不被本步触碰（cutoff 只覆盖
	//    首迁时刻已存在的行，哨兵保证后续启动根本不再计算 cutoff）。
	var pendingIdx int64
	if err := db.Raw(
		`SELECT COUNT(*) FROM pg_indexes WHERE schemaname = 'public' AND indexname = 'idx_feed_recommendations_hash_pending'`,
	).Row().Scan(&pendingIdx); err != nil {
		return fmt.Errorf("discovery backfill a (probe pending index): %w", err)
	}
	if pendingIdx == 0 {
		var cutoff sql.NullInt64
		if err := db.Raw(
			`SELECT max(id) FROM feed_recommendations WHERE status = 'pending'`,
		).Row().Scan(&cutoff); err != nil {
			return fmt.Errorf("discovery backfill a (pending cutoff): %w", err)
		}
		if cutoff.Valid {
			if err := db.Exec(`
				UPDATE feed_recommendations
				SET status = 'legacy', expires_at = now(), updated_at = now()
				WHERE status = 'pending' AND id <= ?
				  AND id NOT IN (
					SELECT max(id) FROM feed_recommendations
					WHERE status = 'pending' AND id <= ?
					GROUP BY recommendation_hash HAVING count(*) > 1
				)`,
				cutoff.Int64, cutoff.Int64,
			).Error; err != nil {
				return fmt.Errorf("discovery backfill a (dedupe legacy pending): %w", err)
			}
		}
	}

	// b. 切换 hash 约束：DROP 旧全表唯一索引（GORM AutoMigrate 不会删旧索引，需显式
	//    DROP），建 pending 部分唯一索引（历史行允许同 hash 多条，仅 pending 唯一）。
	if err := db.Exec(`DROP INDEX IF EXISTS idx_feed_recommendations_hash`).Error; err != nil {
		return fmt.Errorf("discovery backfill b (drop legacy hash index): %w", err)
	}
	if err := db.Exec(`
		CREATE UNIQUE INDEX IF NOT EXISTS idx_feed_recommendations_hash_pending
		ON feed_recommendations (recommendation_hash)
		WHERE status = 'pending'`).Error; err != nil {
		return fmt.Errorf("discovery backfill b (create pending unique index): %w", err)
	}

	// c. 回填候选：每条 rsshub_routes 路由对应一条 rsshub 候选（stable_key=小写
	//    namespace/path，D1）；冲突（历史上大小写变体已收敛）保留已有行。
	if err := db.Exec(`
		INSERT INTO feed_candidates
			(stable_key, kind, route_id, canonical_key, manual_metadata,
			 recommendation_enabled, access_scope, revision, created_at, updated_at)
		SELECT lower(rt.namespace) || '/' || lower(rt.path), 'rsshub', rt.id, '', '{}',
		       true, 'public', 1, now(), now()
		FROM rsshub_routes rt
		ON CONFLICT (stable_key) DO NOTHING`).Error; err != nil {
		return fmt.Errorf("discovery backfill c (backfill route candidates): %w", err)
	}

	// d. 回填关联：旧推荐按 route_id 对应到候选（多写 updated_at 记录本次变更）。
	if err := db.Exec(`
		UPDATE feed_recommendations r
		SET candidate_id = c.id, updated_at = now()
		FROM feed_candidates c
		JOIN rsshub_routes rt ON rt.id = c.route_id
		WHERE r.candidate_id IS NULL
		  AND rt.id = r.route_id
		  AND c.stable_key = lower(rt.namespace) || '/' || lower(rt.path)`).Error; err != nil {
		return fmt.Errorf("discovery backfill d (link recommendations to candidates): %w", err)
	}

	// e. 旧种子迁移：source=seed 的 preference_vectors 迁为 legacy_inactive 兴趣记录，
	//    不编造原查询（query_text 固定占位文案），legacy_ref 保留旧行引用溯源；
	//    behavior 源不迁移（行为画像由 scheduler 全量重算通道继续维护）。NOT EXISTS 挡重，
	//    重跑不产生副本。
	if err := db.Exec(`
		INSERT INTO discovery_interest_entries
			(run_id, query_text, board_id, embedding, dimension, model, status, legacy_ref, created_at, updated_at)
		SELECT NULL, 'legacy seed（无原始查询）', p.board_id, p.embedding, p.dimension, p.model,
		       'legacy_inactive', 'preference_vectors:' || p.id, p.updated_at, p.updated_at
		FROM preference_vectors p
		WHERE p.source = 'seed'
		  AND NOT EXISTS (
			SELECT 1 FROM discovery_interest_entries e
			WHERE e.legacy_ref = 'preference_vectors:' || p.id
		)`).Error; err != nil {
		return fmt.Errorf("discovery backfill e (migrate legacy seeds): %w", err)
	}

	// f. 冷却回填：30 天内 dismissed 的旧推荐推导剩余冷却（snoozed_until = dismissed_at
	//    + 30 天，与 D5 默认冷却一致）；更早的 dismissed 已自然到期不再冷却；ON CONFLICT
	//    DO NOTHING 不覆盖新系统写入的排除/冷却行（回滚排除策略的权威在候选表）。
	if err := db.Exec(`
		INSERT INTO candidate_preferences (candidate_id, snoozed_until, note, created_at, updated_at)
		SELECT r.candidate_id, r.dismissed_at + interval '30 days', 'legacy dismissed', now(), now()
		FROM feed_recommendations r
		WHERE r.dismissed_at IS NOT NULL
		  AND r.candidate_id IS NOT NULL
		  AND r.dismissed_at > now() - interval '30 days'
		ON CONFLICT (candidate_id) DO NOTHING`).Error; err != nil {
		return fmt.Errorf("discovery backfill f (backfill dismiss cooldowns): %w", err)
	}

	return nil
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
