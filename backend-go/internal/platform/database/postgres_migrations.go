package database

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"gorm.io/gorm"
	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/articlerefs"
	"syntopica-backend/internal/platform/logging"
	"syntopica-backend/internal/platform/textutil"
)

// PruneRelationsRebuild is an optional hook called after PruneUnderqualifiedCandidates
// deletes topics. When set, it is invoked once per affected semantic board to
// rebuild daily_report_section_relations (drop stale identity/similarity edges).
// Set from a package that imports both database and the relation-rebuild logic.
var PruneRelationsRebuild func(tx *gorm.DB, boardID uint) error

// tableExists reports whether a table exists in the public schema. Migrations
// that target optional/domain-registered tables (e.g. the daily_report_* tables
// registered by internal/topicgraph) use it to no-op when the table is absent,
// rather than failing on CREATE INDEX / ALTER TABLE. This keeps migrations
// safe on deployments that don't register those domain models.
// EmbeddingConfigDefault is the payload for the EmbeddingConfigSeeder hook
// (declared here so migrations and the tagmanagement implementation agree
// without database importing tagmanagement/models — decouple-backend-domains).
type EmbeddingConfigDefault struct {
	Key         string
	Value       string
	Description string
}

var (
	// EmbeddingConfigSeeder seeds embedding_config defaults. Wired by the
	// tagmanagement root (production) and platform/testutil (golden schema).
	EmbeddingConfigSeeder func(db *gorm.DB, defaults []EmbeddingConfigDefault) error
	// AuxLabelDupMerge performs the one-shot auxiliary-label dedup migration.
	// Wired by the tagmanagement root (production) and platform/testutil.
	AuxLabelDupMerge func(db *gorm.DB) error
)

func runAuxLabelDupMergeHook(db *gorm.DB) error {
	if AuxLabelDupMerge == nil {
		return fmt.Errorf("aux label dup-merge hook not wired (tagmanagement root import missing)")
	}
	if err := AuxLabelDupMerge(db); err != nil {
		return err
	}
	// Invalidate board composition cache here (hook owner side): the moved
	// implementation cannot import database.InvalidateBoardCache.
	if InvalidateBoardCache != nil {
		InvalidateBoardCache()
	}
	return nil
}

func tableExists(db *gorm.DB, table string) bool {
	var exists bool
	if err := db.Raw(`SELECT to_regclass(?) IS NOT NULL`, "public."+table).Row().Scan(&exists); err != nil {
		return false
	}
	return exists
}

// columnIsNullable reports whether a column is nullable (is_nullable = 'YES').
// Returns true (treat as nullable/safe-to-constrain) on any lookup error so
// callers proceed to apply the constraint.
func columnIsNullable(db *gorm.DB, table, column string) bool {
	var nullable string
	if err := db.Raw(
		`SELECT is_nullable FROM information_schema.columns WHERE table_schema='public' AND table_name=? AND column_name=?`,
		table, column,
	).Row().Scan(&nullable); err != nil {
		return true
	}
	return nullable == "YES"
}

// columnExists reports whether a column exists on a table in the public schema.
// Callers use it to skip DDL on columns that were never created (or were
// dropped) — without the guard "column ... does not exist" aborts startup.
func columnExists(db *gorm.DB, table, column string) bool {
	var exists bool
	if err := db.Raw(
		`SELECT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_schema='public' AND table_name=? AND column_name=?)`,
		table, column,
	).Row().Scan(&exists); err != nil {
		return false
	}
	return exists
}

// columnMaxLength reads the character length limit of a column
// (information_schema.columns.character_maximum_length) — the same lookup
// shape as columnIsNullable. Returns (limit, true) when the column exists AND
// has a length limit; returns (0, false) when the column is missing or has no
// limit at all (text and friends — nothing to widen there).
func columnMaxLength(db *gorm.DB, table, column string) (int, bool) {
	var maxLen sql.NullInt64
	if err := db.Raw(
		`SELECT character_maximum_length FROM information_schema.columns WHERE table_schema='public' AND table_name=? AND column_name=?`,
		table, column,
	).Row().Scan(&maxLen); err != nil {
		return 0, false
	}
	if !maxLen.Valid {
		return 0, false
	}
	return int(maxLen.Int64), true
}

// ensureNotNullDefault backfills NULL rows with a literal default value, then
// sets the column NOT NULL — but only if it is currently nullable, so the
// migration is idempotent (re-running on an already-NOT-NULL column no-ops).
// defaultLit is a raw SQL literal (e.g. ”, 0, false, 'active') used for the
// one-shot backfill UPDATE. SET DEFAULT (if needed) is the caller's separate
// responsibility and is itself idempotent in PostgreSQL.
func ensureNotNullDefault(db *gorm.DB, table, column, defaultLit string) error {
	if !columnIsNullable(db, table, column) {
		return nil // already NOT NULL — idempotent no-op
	}
	if err := db.Exec(fmt.Sprintf(`UPDATE %s SET %s = %s WHERE %s IS NULL`, table, column, defaultLit, column)).Error; err != nil {
		return fmt.Errorf("backfill %s.%s: %w", table, column, err)
	}
	if err := db.Exec(fmt.Sprintf(`ALTER TABLE %s ALTER COLUMN %s SET NOT NULL`, table, column)).Error; err != nil {
		return fmt.Errorf("set %s.%s NOT NULL: %w", table, column, err)
	}
	return nil
}

// withLockTimeout runs fn inside a lock_timeout guard, resetting lock_timeout to
// DEFAULT after fn returns. Use it around long-locking DDL (ALTER COLUMN TYPE,
// ADD CONSTRAINT UNIQUE) so a large table is not blocked indefinitely — if the
// statement cannot acquire the lock within timeout, it fails fast instead of
// stalling writers forever.
//
// timeout is a PostgreSQL interval-style string such as "5s" or "10s". It uses
// SET LOCAL (effective for the rest of the current transaction), and the
// trailing reset is defensive: migrations run inside a transaction so SET LOCAL
// would auto-reset at COMMIT anyway, but the explicit reset protects against
// this helper ever being reused on a bare/pooled connection where SET LOCAL
// would otherwise leak to the next caller.
//
// timeout is a hardcoded constant string supplied by the migration author (e.g.
// "5s"), never external/user input, so the fmt.Sprintf SQL build below is not a
// SQL injection vector — hence the inline #nosec G201 (mirrors the
// daily_report_models.go:299 precedent for SET LOCAL lock_timeout).
func withLockTimeout(db *gorm.DB, timeout string, fn func(*gorm.DB) error) error {
	if err := db.Exec(fmt.Sprintf("SET LOCAL lock_timeout = %q" /* #nosec G201 */, timeout)).Error; err != nil {
		return fmt.Errorf("set lock_timeout=%s: %w", timeout, err)
	}
	if err := fn(db); err != nil {
		return err
	}
	// Defensive reset (SET LOCAL auto-resets at tx end, but guard against GUC
	// leakage on pooled/bare connections if this helper is ever reused there).
	_ = db.Exec("SET LOCAL lock_timeout = DEFAULT").Error
	return nil
}

// preMigrateEmbeddingCacheBytea converts ai_embedding_cache.embedding from
// jsonb to bytea BEFORE RunAutoMigrate touches the table. AutoMigrate would
// issue ALTER COLUMN ... TYPE bytea on its own, which fails with "cannot cast
// type jsonb to bytea" (no implicit cast) and aborts startup — so the column
// must already be bytea when AutoMigrate compares types.
//
// The conversion is NON-destructive: legacy jsonb float arrays are decoded
// and re-encoded as float32 LE binary (see models/embedding_codec.go), so
// cached rows keep serving hits. No MIGRATIONS_ALLOW_DESTRUCTIVE gate needed
// (no data loss; malformed legacy rows degrade to NULL with a warn).
// Idempotent: tables/columns already in the target shape are left alone.
func preMigrateEmbeddingCacheBytea(db *gorm.DB) error {
	if !tableExists(db, "ai_embedding_cache") {
		return nil // fresh DB: AutoMigrate will create the table with bytea
	}
	var dataType string
	err := db.Raw(`SELECT a.atttypid::regtype::text
		FROM pg_attribute a
		JOIN pg_class c ON c.oid = a.attrelid
		JOIN pg_namespace n ON n.oid = c.relnamespace
		WHERE c.relname = 'ai_embedding_cache'
		  AND a.attname = 'embedding'
		  AND n.nspname = 'public'`).Scan(&dataType).Error
	if err != nil {
		return fmt.Errorf("check ai_embedding_cache.embedding type: %w", err)
	}
	if dataType != "jsonb" {
		return nil // already converted (or never was jsonb): nothing to do
	}

	// Stage 1 (short tx): add the target column. Re-running after an aborted
	// conversion finds it already there — tolerate that.
	var hasNewCol int64
	if err := db.Raw(`SELECT count(*) FROM pg_attribute a
		JOIN pg_class c ON c.oid = a.attrelid
		JOIN pg_namespace n ON n.oid = c.relnamespace
		WHERE c.relname = 'ai_embedding_cache' AND a.attname = 'embedding_bytea'
		  AND n.nspname = 'public'`).Scan(&hasNewCol).Error; err != nil {
		return fmt.Errorf("check embedding_bytea column: %w", err)
	}
	if hasNewCol == 0 {
		if err := db.Exec(`ALTER TABLE ai_embedding_cache ADD COLUMN embedding_bytea bytea`).Error; err != nil {
			return fmt.Errorf("add embedding_bytea: %w", err)
		}
	}

	// Stage 2 (batched, autocommit per batch): stream legacy rows through the
	// codec in keyset pages so memory stays constant — a full legacy table is
	// 50k+ rows x 31KB jsonb and holding all payloads at once gets the
	// process OOM-killed. Idempotent: rows already converted are re-written
	// with identical bytes, so an aborted conversion just resumes.
	const batchSize = 500
	lastKey := ""
	for {
		type legacyRow struct {
			CacheKey  string
			Embedding string
		}
		var rows []legacyRow
		if err := db.Raw(`SELECT cache_key, embedding::text FROM ai_embedding_cache
			WHERE embedding IS NOT NULL AND cache_key > $1
			ORDER BY cache_key LIMIT $2`, lastKey, batchSize).Scan(&rows).Error; err != nil {
			return fmt.Errorf("read legacy ai_embedding_cache rows: %w", err)
		}
		if len(rows) == 0 {
			break
		}
		for _, r := range rows {
			var vectors [][]float64
			payload := []byte(nil)
			if err := json.Unmarshal([]byte(r.Embedding), &vectors); err != nil {
				// Malformed legacy row degrades to NULL: warn, keep going.
				logging.Warnf("preMigrate ai_embedding_cache: malformed legacy row %s left as NULL: %v", r.CacheKey, err)
			} else {
				payload = models.EncodeEmbeddingVectors(vectors)
			}
			if err := db.Exec(`UPDATE ai_embedding_cache SET embedding_bytea = $1 WHERE cache_key = $2`, payload, r.CacheKey).Error; err != nil {
				return fmt.Errorf("backfill cache row %s: %w", r.CacheKey, err)
			}
			lastKey = r.CacheKey
		}
		logging.Infof("preMigrate ai_embedding_cache: converted through cache_key %s", lastKey)
	}

	// Stage 3: refuse to drop the legacy column while unconverted rows exist
	// (an aborted Stage 2 must resume, not lose data).
	var pending int64
	if err := db.Raw(`SELECT count(*) FROM ai_embedding_cache
		WHERE embedding IS NOT NULL AND embedding_bytea IS NULL`).Scan(&pending).Error; err != nil {
		return fmt.Errorf("count pending rows: %w", err)
	}
	if pending > 0 {
		return fmt.Errorf("ai_embedding_cache conversion incomplete: %d rows pending (re-run startup to resume)", pending)
	}

	// Stage 4 (short tx): swap columns.
	return db.Transaction(func(tx *gorm.DB) error {
		return withLockTimeout(tx, "5s", func(tx *gorm.DB) error {
			if err := tx.Exec(`ALTER TABLE ai_embedding_cache DROP COLUMN embedding`).Error; err != nil {
				return fmt.Errorf("drop legacy embedding column: %w", err)
			}
			if err := tx.Exec(`ALTER TABLE ai_embedding_cache RENAME COLUMN embedding_bytea TO embedding`).Error; err != nil {
				return fmt.Errorf("rename embedding_bytea: %w", err)
			}
			return nil
		})
	})
}

// postgresMigrations returns versioned migrations for operations that GORM AutoMigrate
// cannot handle: extensions, custom indexes, triggers, data migrations, column/table drops.
//
// Pure ADD COLUMN and CREATE TABLE migrations have been removed — AutoMigrate handles those
// automatically on every startup via RunAutoMigrate(). Only operations requiring explicit SQL
// are kept here.
func postgresMigrations() []Migration {
	migrations := []Migration{
		// ── Extensions ──────────────────────────────────────────────
		{
			Version:     "20260403_0001",
			Description: "Enable pgvector support.",
			Up: func(db *gorm.DB) error {
				return db.Exec("CREATE EXTENSION IF NOT EXISTS vector").Error
			},
		},

		// ── Column type adjustments ─────────────────────────────────
		{
			Version:     "20260403_0003",
			Description: "确保向量列存在且维度无关（目标类型无维度限定）。",
			Up: func(db *gorm.DB) error {
				// 目标类型写「无维度限定的 vector」而非 vector(N)：维度由运行时 embedding
				// 模型的实际输出决定（逐行存于 Dimension 字段），不属于 schema 契约。
				// 写成 vector(N) 会让本迁移在「库中维度 ≠ 硬编码维度」时直接失败——
				// 灾备恢复场景下（历史备份档没有迁移历史 → 迁移链在有数据的库上从零重放）
				// 这会把恢复变成一颗地雷。去掉 typmod 是放宽而非收紧约束，任意维度的
				// 存量数据都能安全 ALTER。
				if err := db.Exec("ALTER TABLE topic_tag_embeddings ADD COLUMN IF NOT EXISTS embedding vector").Error; err != nil {
					return fmt.Errorf("add topic_tag_embeddings.embedding column: %w", err)
				}
				// ALTER COLUMN TYPE 在有存量数据时会拿 AccessExclusiveLock ——
				// 套 lock_timeout 守卫，大表快速失败而不是无限期阻塞写入。
				// 列已是无限定 vector 时该语句是幂等 no-op（不重写表、不改列类型）。
				if err := withLockTimeout(db, "5s", func(tx *gorm.DB) error {
					if err := tx.Exec("ALTER TABLE topic_tag_embeddings ALTER COLUMN embedding TYPE vector").Error; err != nil {
						return fmt.Errorf("set topic_tag_embeddings.embedding dimensions: %w", err)
					}
					return nil
				}); err != nil {
					return err
				}
				return nil
			},
		},

		// ── Seed data ───────────────────────────────────────────────
		{
			Version:     "20260413_0002",
			Description: "Seed embedding_config default values.",
			Up: func(db *gorm.DB) error {
				if EmbeddingConfigSeeder == nil {
					return fmt.Errorf("embedding config seeder hook not wired (tagmanagement root import missing)")
				}
				return EmbeddingConfigSeeder(db, []EmbeddingConfigDefault{
					{Key: "high_similarity_threshold", Value: "0.97", Description: "Auto-reuse existing tag if similarity >= this value"},
					{Key: "low_similarity_threshold", Value: "0.78", Description: "Auto-create new tag if similarity < this value"},
					{Key: "embedding_model", Value: "", Description: "Override embedding model name (empty = read from provider)"},
					{Key: "embedding_dimension", Value: "1024", Description: "Embedding vector dimension"},
				})
			},
		},
		{
			Version:     "20260514_0002",
			Description: "Seed event clustering config keys into embedding_config.",
			Up: func(db *gorm.DB) error {
				if EmbeddingConfigSeeder == nil {
					return fmt.Errorf("embedding config seeder hook not wired (tagmanagement root import missing)")
				}
				return EmbeddingConfigSeeder(db, []EmbeddingConfigDefault{
					{Key: "event_cluster_kw_min_overlap", Value: "2", Description: "Minimum shared keyword count for Stage 1 event tag keyword-overlap clustering"},
					{Key: "event_cluster_sem_threshold", Value: "0.80", Description: "Minimum semantic cosine similarity for Stage 2 event tag clustering filter"},
				})
			},
		},

		// ── Indexes ─────────────────────────────────────────────────
		{
			Version:     "20260417_0001",
			Description: "Add missing indexes for CRUD performance optimization.",
			Up: func(db *gorm.DB) error {
				indexes := []string{
					"CREATE INDEX IF NOT EXISTS idx_articles_read ON articles(read)",
					"CREATE INDEX IF NOT EXISTS idx_articles_favorite ON articles(favorite)",
					"CREATE INDEX IF NOT EXISTS idx_articles_feed_pub_date ON articles(feed_id, pub_date DESC)",
					"CREATE INDEX IF NOT EXISTS idx_article_topic_tags_article_id ON article_topic_tags(article_id)",
					"CREATE INDEX IF NOT EXISTS idx_feeds_category_id ON feeds(category_id)",
					"CREATE INDEX IF NOT EXISTS idx_articles_feed_id_title ON articles(feed_id, title)",
				}
				for _, s := range indexes {
					if err := db.Exec(s).Error; err != nil {
						return fmt.Errorf("create index: %w", err)
					}
				}
				return nil
			},
		},
		{
			Version:     "20260418_0001",
			Description: "Add embedding_type to topic_tag_embeddings and set unique index (topic_tag_id, embedding_type).",
			Up: func(db *gorm.DB) error {
				if err := db.Exec(`UPDATE topic_tag_embeddings SET embedding_type = 'identity' WHERE embedding_type IS NULL OR embedding_type = ''`).Error; err != nil {
					return fmt.Errorf("backfill embedding_type: %w", err)
				}
				if err := db.Exec(`DROP INDEX IF EXISTS idx_topic_tag_embeddings_topic_tag_id`).Error; err != nil {
					return fmt.Errorf("drop old unique index: %w", err)
				}
				if err := db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_topic_tag_embeddings_tag_type ON topic_tag_embeddings(topic_tag_id, embedding_type)`).Error; err != nil {
					return fmt.Errorf("create topic_tag_embeddings(tag_id, type) unique index: %w", err)
				}
				return nil
			},
		},
		{
			Version:     "20260420_0001",
			Description: "Add indexes for narrative_summaries scope columns.",
			Up: func(db *gorm.DB) error {
				// retire-narrative-legacy: table dropped (20260824_0001) / model removed —
				// fresh replays no longer have the table; skip instead of fail.
				if !tableExists(db, "narrative_summaries") {
					return nil
				}
				indexes := []string{
					"CREATE INDEX IF NOT EXISTS idx_narrative_scope ON narrative_summaries(scope_category_id)",
					"CREATE INDEX IF NOT EXISTS idx_narrative_scope_period ON narrative_summaries(scope_type, scope_category_id, period_date)",
				}
				for _, s := range indexes {
					if err := db.Exec(s).Error; err != nil {
						return fmt.Errorf("narrative scope index: %w", err)
					}
				}
				return nil
			},
		},
		{
			Version:     "20260430_0001",
			Description: "Add indexes for narrative_boards and narrative_summaries.board_id.",
			Up: func(db *gorm.DB) error {
				// retire-narrative-legacy: table dropped (20260824_0001) / model removed —
				// fresh replays no longer have the table; skip instead of fail.
				if !tableExists(db, "narrative_boards") {
					return nil
				}
				indexes := []string{
					"CREATE INDEX IF NOT EXISTS idx_narrative_boards_period ON narrative_boards(period_date)",
					"CREATE INDEX IF NOT EXISTS idx_narrative_boards_scope ON narrative_boards(scope_category_id)",
					"CREATE INDEX IF NOT EXISTS idx_narrative_summaries_board_id ON narrative_summaries(board_id)",
				}
				for _, s := range indexes {
					if err := db.Exec(s).Error; err != nil {
						return fmt.Errorf("narrative_boards index: %w", err)
					}
				}
				return nil
			},
		},

		// ── Full-text search ────────────────────────────────────────
		{
			Version:     "20260417_0002",
			Description: "Add GIN index for article full-text search using tsvector.",
			Up: func(db *gorm.DB) error {
				// 回填 search_vector：只更新「算出来与存量不同」的行。
				// 无条件 UPDATE 会把 59152 行全部重写一遍（含 ~1GB 的 GIN 索引重建），
				// 在 SD 卡上要 2 小时以上——灾备恢复（档内无迁移历史 → 迁移链从零重放）
				// 会卡在这里。存量行本来就由同一个表达式算过（迁移或触发器），
				// 加 IS DISTINCT FROM 守卫后终态完全相同，但重复执行是零写入。
				stmts := []string{
					`ALTER TABLE articles ADD COLUMN IF NOT EXISTS search_vector tsvector`,
					`CREATE INDEX IF NOT EXISTS idx_articles_search_vector ON articles USING GIN (search_vector)`,
					`CREATE OR REPLACE FUNCTION articles_search_vector_update() RETURNS trigger AS $$
					BEGIN
						NEW.search_vector :=
							setweight(to_tsvector('simple', COALESCE(NEW.title, '')), 'A') ||
							setweight(to_tsvector('simple', COALESCE(NEW.description, '')), 'B');
						RETURN NEW;
					END;
					$$ LANGUAGE plpgsql`,
					`DROP TRIGGER IF EXISTS articles_search_vector_trigger ON articles`,
					`CREATE TRIGGER articles_search_vector_trigger
						BEFORE INSERT OR UPDATE OF title, description ON articles
						FOR EACH ROW EXECUTE FUNCTION articles_search_vector_update()`,
					`UPDATE articles SET search_vector =
						setweight(to_tsvector('simple', COALESCE(title, '')), 'A') ||
						setweight(to_tsvector('simple', COALESCE(description, '')), 'B')
					WHERE search_vector IS DISTINCT FROM
						setweight(to_tsvector('simple', COALESCE(title, '')), 'A') ||
						setweight(to_tsvector('simple', COALESCE(description, '')), 'B')`,
				}
				for _, s := range stmts {
					if err := db.Exec(s).Error; err != nil {
						return fmt.Errorf("full-text search migration: %w", err)
					}
				}
				return nil
			},
		},

		// ── Embedding index changes ─────────────────────────────────
		{
			Version:     "20260514_0001",
			Description: "Change topic_tag_embeddings unique index to (topic_tag_id, embedding_type, text_hash).",
			Up: func(db *gorm.DB) error {
				if err := db.Exec("DROP INDEX IF EXISTS idx_topic_tag_embeddings_tag_type").Error; err != nil {
					return fmt.Errorf("drop old idx_topic_tag_embeddings_tag_type: %w", err)
				}
				if err := db.Exec("CREATE UNIQUE INDEX IF NOT EXISTS idx_topic_tag_embeddings_tag_type_hash ON topic_tag_embeddings(topic_tag_id, embedding_type, text_hash)").Error; err != nil {
					return fmt.Errorf("create idx_topic_tag_embeddings_tag_type_hash: %w", err)
				}
				return nil
			},
		},

		// ── Semantic label board indexes + seed ─────────────────────
		{
			Version:     "20260521_0001",
			Description: "Add indexes and seed settings for semantic label board system.",
			Up: func(db *gorm.DB) error {
				indexes := []string{
					"CREATE UNIQUE INDEX IF NOT EXISTS idx_semantic_labels_slug ON semantic_labels(slug)",
					"CREATE INDEX IF NOT EXISTS idx_semantic_labels_label_type ON semantic_labels(label_type)",
					"CREATE INDEX IF NOT EXISTS idx_semantic_labels_status ON semantic_labels(status)",
					"CREATE INDEX IF NOT EXISTS idx_topic_tag_semantic_labels_topic_tag_id ON topic_tag_semantic_labels(topic_tag_id)",
					"CREATE INDEX IF NOT EXISTS idx_topic_tag_semantic_labels_semantic_label_id ON topic_tag_semantic_labels(semantic_label_id)",
					"CREATE INDEX IF NOT EXISTS idx_topic_tag_board_labels_topic_tag_id ON topic_tag_board_labels(topic_tag_id)",
					"CREATE INDEX IF NOT EXISTS idx_topic_tag_board_labels_semantic_board_id ON topic_tag_board_labels(semantic_board_id)",
					"CREATE INDEX IF NOT EXISTS idx_board_composition_board_id ON board_composition(board_id)",
					"CREATE INDEX IF NOT EXISTS idx_board_composition_auxiliary_label_id ON board_composition(auxiliary_label_id)",
				}
				for _, s := range indexes {
					if err := db.Exec(s).Error; err != nil {
						return fmt.Errorf("semantic label board index: %w", err)
					}
				}
				// retire-narrative-legacy: table dropped (20260824_0001) / model removed —
				// fresh replays no longer have the table; skip instead of fail.
				if tableExists(db, "narrative_boards") {
					if err := db.Exec("CREATE INDEX IF NOT EXISTS idx_narrative_boards_semantic_board_id ON narrative_boards(semantic_board_id)").Error; err != nil {
						return fmt.Errorf("semantic label board index: %w", err)
					}
				}

				settingsDefaults := []struct {
					Key, Value, Description string
				}{
					{"semantic_board_match_sim_threshold", "0.6", "Minimum auxiliary label similarity counted as a SemanticBoard match"},
					{"semantic_board_match_direct_hit_rate", "0.5", "Minimum direct auxiliary label hit rate for a SemanticBoard match"},
					{"semantic_board_match_direct_max_sim", "0.8", "Maximum similarity threshold for direct SemanticBoard matching"},
					{"semantic_board_match_direct_max_sim_min_hits", "2", "Minimum number of auxiliary label hits required for max_sim matching rule"},
					{"semantic_board_match_direct_max_sim_min_hit_rate", "0.3", "Minimum auxiliary label hit rate required for max_sim matching rule"},
					{"semantic_board_match_min_effective_sample", "3", "Minimum denominator for hit rate calculation to prevent inflated scores from low auxiliary label counts"},
					{"semantic_board_match_hit_rate_sim_blend", "0.7", "Weight of maxSimilarity in hit_rate rule score (score = α×maxSim + (1-α)×hitRate)"},
					{"semantic_board_match_weight_sim", "0.6", "Similarity weight used in weighted SemanticBoard matching"},
					{"semantic_board_match_weight_density", "0.4", "Density weight used in weighted SemanticBoard matching"},
					{"semantic_board_match_weighted_threshold", "0.6", "Minimum weighted score for assigning a topic tag to a SemanticBoard"},
					{"semantic_board_match_direct_hit_min_overlap", "2", "Minimum auxiliary label overlap count for direct_hit matching rule"},
					{"semantic_board_match_max_boards", "3", "Maximum SemanticBoard matches retained for each topic tag"},
					{"semantic_board_upgrade_ref_count_threshold", "5", "Minimum reference count before suggesting a new SemanticBoard"},
					{"semantic_board_upgrade_cluster_distance_threshold", "0.35", "Cluster distance threshold for SemanticBoard upgrade suggestions (cosine distance; lower = stricter clustering, prevents unrelated candidates from being absorbed into existing boards)"},
					{"semantic_board_upgrade_cotag_window_days", "30", "Co-tag analysis window in days for SemanticBoard upgrade suggestions"},
					{"semantic_board_upgrade_cotag_top_n", "20", "Maximum co-tag candidates considered for SemanticBoard upgrade suggestions"},
					{"semantic_board_upgrade_cotag_dedupe_sim_threshold", "0.85", "Similarity threshold for deduplicating co-tag upgrade candidates"},
					{"semantic_board_upgrade_cotag_hard_limit", "15", "Hard limit for co-tag upgrade candidates"},
				}
				for _, d := range settingsDefaults {
					var existing models.AISettings
					if err := db.Where("key = ?", d.Key).First(&existing).Error; err != nil {
						if err := db.Create(&models.AISettings{
							Key:         d.Key,
							Value:       d.Value,
							Description: d.Description,
						}).Error; err != nil {
							logging.Warnf("Warning: failed to seed ai_settings key %s: %v", d.Key, err)
						}
					}
				}
				return nil
			},
		},

		// ── Drops (legacy cleanup) ─────────────────────────────────
		{
			Version:     "20260522_0001",
			Description: "Drop legacy board_concepts and hierarchy system tables/columns.",
			Up: func(db *gorm.DB) error {
				columnDrops := []string{
					"ALTER TABLE topic_tags DROP COLUMN IF EXISTS concept_id CASCADE",
				}
				// retire-narrative-legacy: narrative_boards dropped (20260824_0001) / model
				// removed — ALTER on a missing table errors on fresh replays; guard it.
				if tableExists(db, "narrative_boards") {
					columnDrops = append(columnDrops,
						"ALTER TABLE narrative_boards DROP COLUMN IF EXISTS abstract_tag_id CASCADE",
						"ALTER TABLE narrative_boards DROP COLUMN IF EXISTS board_concept_id CASCADE",
						"ALTER TABLE narrative_boards DROP COLUMN IF EXISTS is_system",
						"ALTER TABLE narrative_boards DROP COLUMN IF EXISTS abstract_tag_ids",
					)
				}
				for _, s := range columnDrops {
					if err := db.Exec(s).Error; err != nil {
						return fmt.Errorf("drop column: %w", err)
					}
				}
				tableDrops := []string{
					"DROP TABLE IF EXISTS board_concepts CASCADE",
					"DROP TABLE IF EXISTS hierarchy_configs CASCADE",
					"DROP TABLE IF EXISTS hierarchy_config_versions CASCADE",
					"DROP TABLE IF EXISTS hierarchy_pending_changes CASCADE",
					"DROP TABLE IF EXISTS hierarchy_anchor_signals CASCADE",
					"DROP TABLE IF EXISTS rebuild_jobs CASCADE",
					"DROP TABLE IF EXISTS abstract_tag_update_queues CASCADE",
					"DROP TABLE IF EXISTS adopt_narrower_queues CASCADE",
				}
				for _, s := range tableDrops {
					if err := db.Exec(s).Error; err != nil {
						return fmt.Errorf("drop table: %w", err)
					}
				}
				deprecatedSettings := []string{
					"narrative_board_embedding_threshold",
					"narrative_board_hotspot_threshold",
				}
				for _, key := range deprecatedSettings {
					if err := db.Exec("DELETE FROM ai_settings WHERE key = ?", key).Error; err != nil {
						logging.Warnf("Warning: failed to delete deprecated ai_settings key %s: %v", key, err)
					}
				}
				return nil
			},
		},
		{
			Version:     "20260523_0001",
			Description: "Drop topic_tags sub_type column.",
			Up: func(db *gorm.DB) error {
				stmts := []string{
					"DROP INDEX IF EXISTS idx_topic_tags_sub_type",
					"ALTER TABLE topic_tags DROP COLUMN IF EXISTS sub_type",
				}
				for _, s := range stmts {
					if err := db.Exec(s).Error; err != nil {
						return fmt.Errorf("drop topic_tags sub_type: %w", err)
					}
				}
				return nil
			},
		},

		// ── Daily report indexes ────────────────────────────────────
		{
			Version:     "20260526_0001",
			Description: "Add indexes for board_daily_reports.",
			Up: func(db *gorm.DB) error {
				// The daily_report_* tables are registered by internal/topicgraph;
				// deployments that don't import it (incl. the test harness) won't
				// have them. Skip rather than fail on CREATE INDEX over a missing
				// table.
				if !tableExists(db, "board_daily_reports") || !tableExists(db, "daily_report_sections") {
					return nil
				}
				indexes := []string{
					"CREATE INDEX IF NOT EXISTS idx_board_daily_reports_semantic_board_id ON board_daily_reports(semantic_board_id)",
					"CREATE INDEX IF NOT EXISTS idx_daily_report_sections_report_id ON daily_report_sections(report_id)",
				}
				for _, s := range indexes {
					if err := db.Exec(s).Error; err != nil {
						return fmt.Errorf("daily report index: %w", err)
					}
				}
				return nil
			},
		},

		// ── Daily report thread indexes + data migration ────────────
		{
			Version:     "20260529_0001",
			Description: "Add indexes for daily_report_threads.",
			Up: func(db *gorm.DB) error {
				if !tableExists(db, "daily_report_threads") {
					return nil
				}
				indexes := []string{
					"CREATE INDEX IF NOT EXISTS idx_daily_report_threads_report_id ON daily_report_threads(report_id)",
					"CREATE INDEX IF NOT EXISTS idx_daily_report_threads_section_id ON daily_report_threads(section_id)",
				}
				for _, s := range indexes {
					if err := db.Exec(s).Error; err != nil {
						return fmt.Errorf("daily_report_threads index: %w", err)
					}
				}
				// prev_thread_id index — only if column exists (dropped in later migration)
				var colExists bool
				if err := db.Raw(`SELECT EXISTS (
					SELECT 1 FROM information_schema.columns
					WHERE table_name = 'daily_report_threads' AND column_name = 'prev_thread_id'
				)`).Scan(&colExists).Error; err == nil && colExists {
					if err := db.Exec("CREATE INDEX IF NOT EXISTS idx_daily_report_threads_prev_thread_id ON daily_report_threads(prev_thread_id) WHERE prev_thread_id IS NOT NULL").Error; err != nil {
						return fmt.Errorf("daily_report_threads prev_thread_id index: %w", err)
					}
				}
				return nil
			},
		},
		{
			Version:     "20260529_0002",
			Description: "Migrate existing thread data from daily_report_sections.threads JSONB to daily_report_threads rows.",
			Up: func(db *gorm.DB) error {
				var colExists bool
				if err := db.Raw(`SELECT EXISTS (
					SELECT 1 FROM information_schema.columns
					WHERE table_name = 'daily_report_sections' AND column_name = 'threads'
				)`).Scan(&colExists).Error; err != nil {
					return fmt.Errorf("check threads column: %w", err)
				}
				if !colExists {
					return nil
				}

				err := db.Exec(`
					INSERT INTO daily_report_threads (report_id, section_id, title, summary, status, tag_ids, confidence, prev_thread_id, created_at)
					SELECT
						s.report_id,
						s.id,
						COALESCE(t->>'title', ''),
						t->>'summary',
						COALESCE(t->>'status', 'emerging'),
						COALESCE(t->'related_tag_ids', '[]'::jsonb),
						COALESCE((t->>'confidence')::double precision, 0),
						NULL,
						s.created_at
					FROM daily_report_sections s
					CROSS JOIN jsonb_array_elements(s.threads) AS t
					WHERE s.threads IS NOT NULL
					  AND jsonb_array_length(s.threads) > 0
				`).Error
				if err != nil {
					return fmt.Errorf("migrate threads JSONB to rows: %w", err)
				}
				return nil
			},
		},
		{
			Version:     "20260529_0003",
			Description: "Drop threads JSONB column from daily_report_sections after migration.",
			Up: func(db *gorm.DB) error {
				var colExists bool
				if err := db.Raw(`SELECT EXISTS (
					SELECT 1 FROM information_schema.columns
					WHERE table_name = 'daily_report_sections' AND column_name = 'threads'
				)`).Scan(&colExists).Error; err != nil {
					return fmt.Errorf("check threads column: %w", err)
				}
				if !colExists {
					return nil
				}
				if err := db.Exec(`ALTER TABLE daily_report_sections DROP COLUMN threads`).Error; err != nil {
					return fmt.Errorf("drop threads column: %w", err)
				}
				return nil
			},
		},

		// ── FK cascade: remove duplicate NO ACTION constraints on topic_tags children ──
		{
			Version:     "20260601_0001",
			Description: "Remove duplicate NO ACTION foreign keys on topic_tags child tables, ensure all are ON DELETE CASCADE.",
			Up: func(db *gorm.DB) error {
				stmts := []string{
					// embedding_queues
					`DO $$ BEGIN
						IF EXISTS (SELECT 1 FROM information_schema.table_constraints WHERE constraint_name = 'fk_embedding_queues_tag' AND table_name = 'embedding_queues') THEN
							ALTER TABLE embedding_queues DROP CONSTRAINT fk_embedding_queues_tag;
						END IF;
					END $$`,

					// merge_reembedding_queues
					`DO $$ BEGIN
						IF EXISTS (SELECT 1 FROM information_schema.table_constraints WHERE constraint_name = 'fk_merge_reembedding_queues_target_tag' AND table_name = 'merge_reembedding_queues') THEN
							ALTER TABLE merge_reembedding_queues DROP CONSTRAINT fk_merge_reembedding_queues_target_tag;
						END IF;
					END $$`,
					`DO $$ BEGIN
						IF EXISTS (SELECT 1 FROM information_schema.table_constraints WHERE constraint_name = 'fk_merge_reembedding_queues_source_tag' AND table_name = 'merge_reembedding_queues') THEN
							ALTER TABLE merge_reembedding_queues DROP CONSTRAINT fk_merge_reembedding_queues_source_tag;
						END IF;
					END $$`,

					// topic_tag_embeddings
					`DO $$ BEGIN
						IF EXISTS (SELECT 1 FROM information_schema.table_constraints WHERE constraint_name = 'fk_topic_tags_embedding' AND table_name = 'topic_tag_embeddings') THEN
							ALTER TABLE topic_tag_embeddings DROP CONSTRAINT fk_topic_tags_embedding;
						END IF;
					END $$`,
					`DO $$ BEGIN
						IF EXISTS (SELECT 1 FROM information_schema.table_constraints WHERE constraint_name = 'fk_topic_tags_embeddings' AND table_name = 'topic_tag_embeddings') THEN
							ALTER TABLE topic_tag_embeddings DROP CONSTRAINT fk_topic_tags_embeddings;
						END IF;
					END $$`,

					// topic_tag_relations
					`DO $$ BEGIN
						IF EXISTS (SELECT 1 FROM information_schema.table_constraints WHERE constraint_name = 'fk_topic_tag_relations_child' AND table_name = 'topic_tag_relations') THEN
							ALTER TABLE topic_tag_relations DROP CONSTRAINT fk_topic_tag_relations_child;
						END IF;
					END $$`,
					`DO $$ BEGIN
						IF EXISTS (SELECT 1 FROM information_schema.table_constraints WHERE constraint_name = 'fk_topic_tag_relations_parent' AND table_name = 'topic_tag_relations') THEN
							ALTER TABLE topic_tag_relations DROP CONSTRAINT fk_topic_tag_relations_parent;
						END IF;
					END $$`,

					// topic_tags self-ref (merged_into): drop old NO ACTION, add CASCADE
					`DO $$ BEGIN
						IF EXISTS (SELECT 1 FROM information_schema.table_constraints WHERE constraint_name = 'fk_topic_tags_merged_into' AND table_name = 'topic_tags') THEN
							ALTER TABLE topic_tags DROP CONSTRAINT fk_topic_tags_merged_into;
						END IF;
					END $$`,
					`DO $$ BEGIN
						IF EXISTS (SELECT 1 FROM information_schema.table_constraints WHERE constraint_name = 'topic_tags_merged_into_id_fkey' AND table_name = 'topic_tags') THEN
							ALTER TABLE topic_tags DROP CONSTRAINT topic_tags_merged_into_id_fkey;
						END IF;
					END $$`,
					`ALTER TABLE topic_tags ADD CONSTRAINT topic_tags_merged_into_id_fkey
						FOREIGN KEY (merged_into_id) REFERENCES topic_tags(id) ON DELETE CASCADE`,
				}
				for _, s := range stmts {
					if err := db.Exec(s).Error; err != nil {
						return fmt.Errorf("topic_tags FK cascade migration: %w", err)
					}
				}
				return nil
			},
		},

		// ── Drop deprecated vector text column ────────────────────
		{
			Version:     "20260601_0001b",
			Description: "Drop deprecated vector text column from topic_tag_embeddings.",
			Up: func(db *gorm.DB) error {
				return db.Exec("ALTER TABLE topic_tag_embeddings DROP COLUMN IF EXISTS vector").Error
			},
		},

		// ── Section relations (many-to-many) + drop legacy columns ────────
		{
			Version:     "20260603_0001",
			Description: "Add unique constraint to section_relations, migrate prev_section_id, drop status/prev_thread_id columns.",
			Up: func(db *gorm.DB) error {
				// daily_report_* tables are optional (registered by internal/topicgraph);
				// skip the whole block if the core table is absent.
				if !tableExists(db, "daily_report_section_relations") {
					return nil
				}

				// 1. Add unique constraint (table created by AutoMigrate)
				if err := db.Exec(`
					ALTER TABLE daily_report_section_relations
					DROP CONSTRAINT IF EXISTS uq_section_relations_pair
				`).Error; err != nil {
					return fmt.Errorf("drop old constraint: %w", err)
				}
				// ADD CONSTRAINT UNIQUE scans the whole table to verify
				// uniqueness under an AccessExclusiveLock — guard with
				// lock_timeout so a large table fails fast instead of blocking
				// writers indefinitely.
				if err := withLockTimeout(db, "5s", func(tx *gorm.DB) error {
					if err := tx.Exec(`
						ALTER TABLE daily_report_section_relations
						ADD CONSTRAINT uq_section_relations_pair UNIQUE (from_section_id, to_section_id)
					`).Error; err != nil {
						return fmt.Errorf("add unique constraint: %w", err)
					}
					return nil
				}); err != nil {
					return err
				}

				// 2. Migrate existing prev_section_id data into relations (if column exists)
				var prevColExists bool
				if err := db.Raw(`SELECT EXISTS (
					SELECT 1 FROM information_schema.columns
					WHERE table_name = 'daily_report_sections' AND column_name = 'prev_section_id'
				)`).Scan(&prevColExists).Error; err == nil && prevColExists {
					if err := db.Exec(`
						INSERT INTO daily_report_section_relations (from_section_id, to_section_id, distance, created_at)
						SELECT ps.id, s.id, 0.0, NOW()
						FROM daily_report_sections s
						JOIN daily_report_sections ps ON ps.id = s.prev_section_id
						WHERE s.prev_section_id IS NOT NULL
						ON CONFLICT (from_section_id, to_section_id) DO NOTHING
					`).Error; err != nil {
						logging.Warnf("migration: migrate prev_section_id data: %v", err)
					}
				}

				// 3. Drop prev_section_id column
				if err := db.Exec(`ALTER TABLE daily_report_sections DROP COLUMN IF EXISTS prev_section_id CASCADE`).Error; err != nil {
					logging.Warnf("migration: drop prev_section_id: %v", err)
				}

				// 4. Drop status column from sections
				if err := db.Exec(`ALTER TABLE daily_report_sections DROP COLUMN IF EXISTS status CASCADE`).Error; err != nil {
					logging.Warnf("migration: drop sections.status: %v", err)
				}

				// 5. Drop status column from threads
				if err := db.Exec(`ALTER TABLE daily_report_threads DROP COLUMN IF EXISTS status CASCADE`).Error; err != nil {
					logging.Warnf("migration: drop threads.status: %v", err)
				}

				// 6. Drop prev_thread_id column from threads
				if err := db.Exec(`DROP INDEX IF EXISTS idx_daily_report_threads_prev_thread_id`).Error; err != nil {
					logging.Warnf("migration: drop prev_thread_id index: %v", err)
				}
				if err := db.Exec(`ALTER TABLE daily_report_threads DROP COLUMN IF EXISTS prev_thread_id CASCADE`).Error; err != nil {
					logging.Warnf("migration: drop prev_thread_id: %v", err)
				}

				return nil
			},
		},

		// ── Clean up dead embedding config keys ──────────────────────────
		{
			Version:     "20260614_0001",
			Description: "Remove dead embedding_config and ai_settings rows that are no longer consumed by any runtime code.",
			Up: func(db *gorm.DB) error {
				deadEmbeddingKeys := []string{
					"high_similarity_threshold",
					"low_similarity_threshold",
					"embedding_dimension",
					"embedding_model",
				}
				if err := db.Exec("DELETE FROM embedding_config WHERE key IN (?, ?, ?, ?)",
					deadEmbeddingKeys[0], deadEmbeddingKeys[1], deadEmbeddingKeys[2], deadEmbeddingKeys[3]).Error; err != nil {
					return fmt.Errorf("delete dead embedding_config keys: %w", err)
				}
				if err := db.Exec("DELETE FROM ai_settings WHERE key = ?", "narrative_board_embedding_threshold").Error; err != nil {
					return fmt.Errorf("delete dead ai_settings key: %w", err)
				}
				return nil
			},
		},

		// ── Section embedding column ────────────────────────────────────
		{
			Version:     "20260601_0002",
			Description: "Add embedding vector column to daily_report_sections (dimension set at runtime).",
			Up: func(db *gorm.DB) error {
				if !tableExists(db, "daily_report_sections") {
					return nil
				}
				if err := db.Exec(`ALTER TABLE daily_report_sections ADD COLUMN IF NOT EXISTS embedding vector`).Error; err != nil {
					return fmt.Errorf("add daily_report_sections.embedding column: %w", err)
				}
				return nil
			},
		},

		// ── Seed daily_report_time default ───────────────────────────────
		{
			Version:     "20260614_0002",
			Description: "Seed daily_report_time default value into ai_settings.",
			Up: func(db *gorm.DB) error {
				var existing models.AISettings
				if err := db.Where("key = ?", "daily_report_time").First(&existing).Error; err == nil {
					return nil // already exists
				}
				return db.Create(&models.AISettings{
					Key:         "daily_report_time",
					Value:       "21:00",
					Description: "日报生成时刻（HH:MM）",
				}).Error
			},
		},

		// ── Feed icon_source backfill ───────────────────────────────────
		// AutoMigrate adds feeds.icon_source with default 'fallback', but
		// existing rows carry legacy icon values with mixed semantics. This
		// one-shot data migration classifies them by their icon value: URLs
		// -> auto, placeholder strings -> fallback, other iconify ids ->
		// custom (conservatively preserved as user-owned).
		//
		// Idempotency: versioned migrations run exactly once (tracked in
		// schema_migrations), so unconditional classification is safe here.
		// Rows created after this migration set icon_source correctly at write
		// time and are never touched by this block again.
		{
			Version:     "20260617_0001",
			Description: "Backfill feeds.icon_source from legacy icon values and normalize placeholders.",
			Up: func(db *gorm.DB) error {
				if !tableExists(db, "feeds") {
					return nil
				}
				// Classify every legacy row by its current icon value.
				if err := db.Exec(`
					UPDATE feeds SET icon_source = CASE
						WHEN icon IS NULL OR icon = '' OR icon = 'rss' OR icon = 'mdi:rss' THEN 'fallback'
						WHEN icon LIKE 'http://%' OR icon LIKE 'https://%' THEN 'auto'
						ELSE 'custom'
					END
					WHERE icon_source IS DISTINCT FROM (
						CASE
							WHEN icon IS NULL OR icon = '' OR icon = 'rss' OR icon = 'mdi:rss' THEN 'fallback'
							WHEN icon LIKE 'http://%' OR icon LIKE 'https://%' THEN 'auto'
							ELSE 'custom'
						END
					)
				`).Error; err != nil {
					return fmt.Errorf("backfill feeds.icon_source: %w", err)
				}
				// Normalize placeholder icon values for fallback rows (rss/"" -> mdi:rss)
				if err := db.Exec(`
				UPDATE feeds SET icon = 'mdi:rss'
				WHERE icon_source = 'fallback' AND icon IN ('', 'rss')
			`).Error; err != nil {
					return fmt.Errorf("normalize feeds.icon placeholders: %w", err)
				}
				return nil
			},
		},

		// ── Persistent topic layer ───────────────────────────────────────
		// board_persistent_topics is created by AutoMigrate (registered by
		// internal/topicgraph). This migration adds the domain-specific constraints
		// AutoMigrate cannot express: the status CHECK, the (board, status) lookup
		// index, and the relation_type column on section relations with its default
		// backfill. The embedding HNSW index is created at startup by
		// ensurePersistentTopicEmbeddingDimension (dimension-dependent, ≤2000 only),
		// so it is NOT created here.
		{
			Version:     "20260619_0001",
			Description: "Add persistent topic constraints and section relation_type column.",
			Up: func(db *gorm.DB) error {
				if tableExists(db, "board_persistent_topics") {
					if err := db.Exec(`
					DO $$ BEGIN
						IF NOT EXISTS (
							SELECT 1 FROM information_schema.table_constraints
							WHERE constraint_name = 'chk_board_persistent_topics_status'
							  AND table_name = 'board_persistent_topics'
						) THEN
							ALTER TABLE board_persistent_topics
								ADD CONSTRAINT chk_board_persistent_topics_status
								CHECK (status IN ('candidate', 'active', 'archived'));
						END IF;
					END $$
				`).Error; err != nil {
						return fmt.Errorf("add board_persistent_topics status CHECK: %w", err)
					}
					if err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_persistent_topics_board_status
					ON board_persistent_topics(semantic_board_id, status)`).Error; err != nil {
						return fmt.Errorf("add board_persistent_topics (board, status) index: %w", err)
					}
				}

				if tableExists(db, "daily_report_section_relations") {
					// Add relation_type column (AutoMigrate adds it on fresh DBs; this
					// ALTER covers DBs that had the table before the model gained the
					// column, and backfills legacy rows to 'similarity').
					var typeColExists bool
					if err := db.Raw(`SELECT EXISTS (
					SELECT 1 FROM information_schema.columns
					WHERE table_name = 'daily_report_section_relations' AND column_name = 'relation_type'
				)`).Scan(&typeColExists).Error; err == nil && !typeColExists {
						if err := db.Exec(`ALTER TABLE daily_report_section_relations
						ADD COLUMN relation_type VARCHAR(20) NOT NULL DEFAULT 'similarity'`).Error; err != nil {
							return fmt.Errorf("add daily_report_section_relations.relation_type: %w", err)
						}
					}
					if err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_section_relations_type
					ON daily_report_section_relations(relation_type)`).Error; err != nil {
						return fmt.Errorf("add section_relations relation_type index: %w", err)
					}
				}
				return nil
			},
		},

		// ── Seed persistent topic config keys ───────────────────────────
		{
			Version:     "20260619_0002",
			Description: "Seed persistent topic lifecycle config into ai_settings.",
			Up: func(db *gorm.DB) error {
				defaults := []struct {
					Key, Value, Description string
				}{
					{"persistent_topic_match_threshold", "0.30", "Section-to-topic assignment embedding distance ceiling (dual confirmation)"},
					{"persistent_topic_upgrade_threshold", "3", "Consecutive hit days for a candidate topic to auto-promote to active"},
					{"persistent_topic_decay_window", "30", "Days an active topic can go without a hit before auto-archiving"},
					{"persistent_topic_cluster_threshold", "0.28", "Complete-link clustering distance for backfilling topics from historical sections (0.28 calibrated on real data: 0.30 chained to 1 topic, 0.25 fragmented)"},
				}
				for _, d := range defaults {
					var existing models.AISettings
					if err := db.Where("key = ?", d.Key).First(&existing).Error; err != nil {
						if err := db.Create(&models.AISettings{
							Key:         d.Key,
							Value:       d.Value,
							Description: d.Description,
						}).Error; err != nil {
							logging.Warnf("Warning: failed to seed ai_settings key %s: %v", d.Key, err)
						}
					}
				}
				return nil
			},
		},

		// ── Quality scoring observability ──────────────────────────────
		{
			Version:     "20260625_0001",
			Description: "Add quality_breakdown JSONB column to daily_report_sections.",
			Up: func(db *gorm.DB) error {
				if !tableExists(db, "daily_report_sections") {
					return nil
				}
				return db.Exec("ALTER TABLE daily_report_sections ADD COLUMN IF NOT EXISTS quality_breakdown JSONB NULL").Error
			},
		},

		// ── Widen section_relations unique constraint to (from, to, type) ───
		// Lets an identity edge (same persistent topic) and a similarity edge
		// (Hungarian match) on the same section pair coexist as two rows.
		// Before this, identity silently overwrote a strong Hungarian match,
		// dropping the edge from the similarity-only timeline view. No data
		// migration is needed: the old (from, to) constraint already forbade
		// duplicates per pair, so widening to (from, to, relation_type) cannot
		// introduce a violation — every existing (from, to, type) is already
		// unique. Relation rows are rebuilt by RebuildBoardRelations.
		{
			Version:     "20260620_0001",
			Description: "Widen section_relations unique constraint to (from_section_id, to_section_id, relation_type) so identity and similarity edges coexist.",
			Up: func(db *gorm.DB) error {
				if !tableExists(db, "daily_report_section_relations") {
					return nil
				}
				if err := db.Exec(`
					ALTER TABLE daily_report_section_relations
					DROP CONSTRAINT IF EXISTS uq_section_relations_pair
				`).Error; err != nil {
					return fmt.Errorf("drop old section_relations pair constraint: %w", err)
				}
				// ADD CONSTRAINT UNIQUE (widened to include relation_type) scans
				// the whole table to verify uniqueness under an
				// AccessExclusiveLock — guard with lock_timeout so a large table
				// fails fast instead of blocking writers indefinitely.
				if err := withLockTimeout(db, "5s", func(tx *gorm.DB) error {
					if err := tx.Exec(`
						ALTER TABLE daily_report_section_relations
						ADD CONSTRAINT uq_section_relations_pair UNIQUE (from_section_id, to_section_id, relation_type)
					`).Error; err != nil {
						return fmt.Errorf("add widened section_relations pair constraint: %w", err)
					}
					return nil
				}); err != nil {
					return err
				}
				return nil
			},
		},

		// ── Data migration: enable_thinking semantics flip ──────────
		// AIProvider.EnableThinking's meaning changed from "strip <think> tags after-the-fact"
		// to "enable model reasoning (propagate chat_template_kwargs.enable_thinking)". Stale
		// true values (set under the old meaning) would accidentally enable thinking and slow
		// down topic tagging, so reset all to false on deploy. Idempotent.
		{
			Version:     "20260626_0001",
			Description: "Reset ai_providers.enable_thinking to false (semantics flipped from strip-think-tags to enable-thinking).",
			Up: func(db *gorm.DB) error {
				return db.Exec("UPDATE ai_providers SET enable_thinking = false").Error
			},
		},

		// ── Persistent topic attention separation ─────────────────────
		{
			Version:     "20260627_0001",
			Description: "Snapshot report-time topic status and seed candidate retention limits.",
			Up: func(db *gorm.DB) error {
				if tableExists(db, "daily_report_sections") {
					if err := db.Exec(`ALTER TABLE daily_report_sections
						ADD COLUMN IF NOT EXISTS topic_status_at_report VARCHAR(20) NULL`).Error; err != nil {
						return fmt.Errorf("add daily_report_sections.topic_status_at_report: %w", err)
					}
				}

				defaults := []struct {
					Key, Value, Description string
				}{
					{"persistent_topic_candidate_decay_window", "7", "Days a candidate topic remains anchorable without another hit"},
					{"persistent_topic_candidate_prompt_limit", "20", "Maximum candidate topics injected into clustering and assignment per board"},
				}
				for _, d := range defaults {
					var existing models.AISettings
					if err := db.Where("key = ?", d.Key).First(&existing).Error; err == nil {
						continue
					}
					if err := db.Create(&models.AISettings{
						Key: d.Key, Value: d.Value, Description: d.Description,
					}).Error; err != nil {
						return fmt.Errorf("seed ai_settings key %s: %w", d.Key, err)
					}
				}
				return nil
			},
		},

		// ── Prune underqualified observing candidates ───────────────
		{
			Version:     "20260628_0001",
			Description: "Hard-delete candidate topics with hit_count < upgrade_threshold (one-shot cleanup of historical observing candidates).",
			Up: func(db *gorm.DB) error {
				if !tableExists(db, "board_persistent_topics") || !tableExists(db, "daily_report_sections") {
					return nil
				}
				threshold := 3 // default
				var key models.AISettings
				if err := db.Where("key = ?", "persistent_topic_upgrade_threshold").First(&key).Error; err == nil {
					if v, err2 := strconv.Atoi(key.Value); err2 == nil && v > 0 {
						threshold = v
					}
				}
				deleted, err := PruneUnderqualifiedCandidates(db, threshold)
				if err != nil {
					return fmt.Errorf("prune underqualified candidates: %w", err)
				}
				logging.Infof("Migration 20260628_0001: pruned %d underqualified candidate topics (hit_count < %d)", deleted, threshold)
				return nil
			},
		},

		// ── Topic watch CHECK constraint ───────────────────────────────
		// board_topic_watches and topic_watch_hits tables are created by
		// AutoMigrate (registered by internal/topicgraph). This migration
		// adds the domain-specific CHECK constraint AutoMigrate cannot
		// express: board_topic_watches.status IN ('active','paused').
		// Idempotent — second run is a no-op.
		{
			Version:     "20260630_0001",
			Description: "Add CHECK constraint on board_topic_watches.status (active|paused).",
			Up: func(db *gorm.DB) error {
				if !tableExists(db, "board_topic_watches") {
					return nil
				}
				if err := db.Exec(`
					DO $$ BEGIN
						IF NOT EXISTS (
							SELECT 1 FROM information_schema.table_constraints
							WHERE constraint_name = 'chk_board_topic_watches_status'
							  AND table_name = 'board_topic_watches'
						) THEN
							ALTER TABLE board_topic_watches
								ADD CONSTRAINT chk_board_topic_watches_status
								CHECK (status IN ('active', 'paused'));
						END IF;
					END $$
				`).Error; err != nil {
					return fmt.Errorf("add board_topic_watches status CHECK: %w", err)
				}
				return nil
			},
		},
		{
			Version:     "20260630_0002",
			Description: "Add composite unique index on topic_watch_hits(watch_id, section_id, report_id).",
			Up: func(db *gorm.DB) error {
				if !tableExists(db, "topic_watch_hits") {
					return nil
				}
				// Rollback: DROP INDEX IF EXISTS idx_watch_section_report;
				if err := db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_watch_section_report ON topic_watch_hits(watch_id, section_id, report_id)`).Error; err != nil {
					return fmt.Errorf("add topic_watch_hits unique index: %w", err)
				}
				return nil
			},
		},

		// ── AI call log schema补齐 (ai-logging R2 必记字段) ──────
		{
			Version:     "20260704_0001",
			Description: "为 ai_call_logs 补 R2 必记字段: operation/prompt/token_usage/session_id/model 五列 + 索引.",
			Up: func(db *gorm.DB) error {
				stmts := []string{
					"ALTER TABLE ai_call_logs ADD COLUMN IF NOT EXISTS operation VARCHAR(80)",
					"ALTER TABLE ai_call_logs ADD COLUMN IF NOT EXISTS prompt TEXT",
					"ALTER TABLE ai_call_logs ADD COLUMN IF NOT EXISTS token_usage JSONB",
					"ALTER TABLE ai_call_logs ADD COLUMN IF NOT EXISTS session_id VARCHAR(120)",
					"ALTER TABLE ai_call_logs ADD COLUMN IF NOT EXISTS model VARCHAR(100)",
					"CREATE INDEX IF NOT EXISTS idx_call_logs_session ON ai_call_logs(session_id)",
					"CREATE INDEX IF NOT EXISTS idx_call_logs_op_time ON ai_call_logs(operation, created_at)",
				}
				for _, s := range stmts {
					if err := db.Exec(s).Error; err != nil {
						return fmt.Errorf("ai_call_logs schema migration: %w", err)
					}
				}
				// Backfill + SET NOT NULL via the idempotent helper (re-running this
				// migration on an already-NOT-NULL column is a no-op, unlike a bare
				// ALTER COLUMN ... SET NOT NULL which errors on a non-nullable column).
				if err := ensureNotNullDefault(db, "ai_call_logs", "operation", "'unknown'"); err != nil {
					return fmt.Errorf("ai_call_logs.operation NOT NULL: %w", err)
				}
				return nil
			},
		},

		// ── Manual topic lane source column ───────────────────────────
		{
			Version:     "20260702_0001",
			Description: "Add board_persistent_topics.source column (auto/manual) with CHECK constraint.",
			Up: func(db *gorm.DB) error {
				if !tableExists(db, "board_persistent_topics") {
					return nil
				}
				// Add column with default 'auto' (idempotent via IF NOT EXISTS).
				if err := db.Exec(`
					ALTER TABLE board_persistent_topics
					ADD COLUMN IF NOT EXISTS source VARCHAR(10) NOT NULL DEFAULT 'auto'
				`).Error; err != nil {
					return fmt.Errorf("add board_persistent_topics.source column: %w", err)
				}
				// Add CHECK constraint (idempotent: check information_schema first).
				if err := db.Exec(`
					DO $$ BEGIN
						IF NOT EXISTS (
							SELECT 1 FROM information_schema.table_constraints
							WHERE constraint_name = 'chk_board_persistent_topics_source'
							  AND table_name = 'board_persistent_topics'
						) THEN
							ALTER TABLE board_persistent_topics
								ADD CONSTRAINT chk_board_persistent_topics_source
								CHECK (source IN ('auto', 'manual'));
						END IF;
					END $$
				`).Error; err != nil {
					return fmt.Errorf("add board_persistent_topics.source CHECK: %w", err)
				}
				return nil
			},
		},

		// ── Lifeline context: period-archival model change ──────────
		{
			Version:     "20260706_0001",
			Description: "Drop old idx_topic_gran unique index; TRUNCATE topic_lifeline_context old data (pre-period model). Destructive — guarded by MIGRATIONS_ALLOW_DESTRUCTIVE. ⚠️ 不可逆 TRUNCATE（破坏性，受 `MIGRATIONS_ALLOW_DESTRUCTIVE` 守卫；生产执行前需备份）",
			Up: func(db *gorm.DB) error {
				// Destructive (TRUNCATE): skip unless MIGRATIONS_ALLOW_DESTRUCTIVE=1.
				// Production never enables this; dev/test set the env to clean pre-period data.
				if !IsDestructiveAllowed() {
					logging.Warnf("skipping destructive migration 20260706_0001 (set MIGRATIONS_ALLOW_DESTRUCTIVE=1 to enable)")
					return nil
				}
				// Drop old 2-column unique index (replaced by idx_topic_gran_period).
				if tableExists(db, "topic_lifeline_context") {
					if err := db.Exec(`
						DO $$ BEGIN
							IF EXISTS (
								SELECT 1 FROM pg_indexes
								WHERE indexname = 'idx_topic_gran'
								  AND tablename = 'topic_lifeline_context'
							) THEN
								ALTER TABLE topic_lifeline_context DROP CONSTRAINT IF EXISTS idx_topic_gran;
								DROP INDEX IF EXISTS idx_topic_gran;
							END IF;
						END $$`).Error; err != nil {
						return fmt.Errorf("drop old idx_topic_gran: %w", err)
					}
					// TRUNCATE old data (no period field → cannot satisfy new 3-col UNIQUE).
					if err := db.Exec("TRUNCATE topic_lifeline_context").Error; err != nil {
						return fmt.Errorf("truncate topic_lifeline_context: %w", err)
					}
				}
				return nil
			},
		},

		// ── Data-enrichment §11.5: clear pre-演进定位 schema data ──
		// 旧 result.sectors 用涨跌(direction/trigger)语义、旧 review.verdict 用兑现度(hit/part/miss)，
		// 与 §11.2/§11.3 的演进定位(position/signals、position_change)不兼容，不可复用，清空重跑。
		// stock_debate_result FK 引用 result，一并清。幂等：TRUNCATE 本身幂等 + 框架按 Version 去重。
		{
			Version:     "20260712_0001",
			Description: "TRUNCATE topic_enrichment_result/topic_enrichment_review/stock_debate_result — old 涨跌+兑现 schema incompatible with 演进定位 rewrite (§11.5). Destructive — guarded by MIGRATIONS_ALLOW_DESTRUCTIVE. ⚠️ 不可逆 TRUNCATE（破坏性，受 `MIGRATIONS_ALLOW_DESTRUCTIVE` 守卫；生产执行前需备份）",
			Up: func(db *gorm.DB) error {
				// Destructive (TRUNCATE): skip unless MIGRATIONS_ALLOW_DESTRUCTIVE=1.
				if !IsDestructiveAllowed() {
					logging.Warnf("skipping destructive migration 20260712_0001 (set MIGRATIONS_ALLOW_DESTRUCTIVE=1 to enable)")
					return nil
				}
				// 三张表由 AutoMigrate 在版本迁移前一并创建；result 表不存在则无需清理。
				if !tableExists(db, "topic_enrichment_result") {
					return nil
				}
				// CASCADE 断开 FK 子表引用；RESTART IDENTITY 复位 BIGSERIAL 序列。TRUNCATE 本身幂等。
				if err := db.Exec("TRUNCATE TABLE topic_enrichment_result, topic_enrichment_review, stock_debate_result RESTART IDENTITY CASCADE").Error; err != nil {
					return fmt.Errorf("truncate enrichment stale data: %w", err)
				}
				return nil
			},
		},

		// ── Board upgrade suggestions table ─────────────────────────────
		{
			Version:     "20260717_0001",
			Description: "Create board_upgrade_suggestions table and seed auxiliary_label_dedupe_sim setting.",
			Up: func(db *gorm.DB) error {
				// Table is created by AutoMigrate (model registered in models/).
				// Add partial unique index on suggestion_hash WHERE status='pending'.
				if err := db.Exec(`
					CREATE UNIQUE INDEX IF NOT EXISTS uq_board_upgrade_suggestions_hash
					ON board_upgrade_suggestions(suggestion_hash) WHERE status = 'pending'
				`).Error; err != nil {
					return fmt.Errorf("create board_upgrade_suggestions partial unique index: %w", err)
				}
				if err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_board_upgrade_suggestions_status ON board_upgrade_suggestions(status)`).Error; err != nil {
					return fmt.Errorf("create board_upgrade_suggestions status index: %w", err)
				}

				// Seed auxiliary_label_dedupe_sim default
				var existing models.AISettings
				if err := db.Where("key = ?", "auxiliary_label_dedupe_sim").First(&existing).Error; err != nil {
					if err := db.Create(&models.AISettings{
						Key:         "auxiliary_label_dedupe_sim",
						Value:       "0.95",
						Description: "辅助标签 embedding 去重阈值（cosine similarity，L2 gate），低于此值不合并",
					}).Error; err != nil {
						logging.Warnf("Warning: failed to seed ai_settings key auxiliary_label_dedupe_sim: %v", err)
					}
				}
				return nil
			},
		},

		// ── One-shot auxiliary label text-variant merge ─────────────────
		{
			Version:     "20260717_0002",
			Description: "Merge auxiliary label text variant duplicates by normalize_key grouping, reusing MergeAuxiliaryLabelAlias.",
			Up:          runAuxLabelDupMergeHook,
		},

		// ── causal-analysis-agent: clear stale 演进定位 enrichment data ──
		// 分析目标从「演进定位」改为「探索判断 agent」(causal-analysis-agent)。
		// 旧 result 的 position/signals、旧 review 的 verdict 语义随之作废，与新
		// agent 产出不兼容，不可复用，清空后由新 agent 重跑。报告追问交互层
		// (topic_enrichment_qa) 为另起新表，不在此清理范围。
		// 幂等：TRUNCATE 本身幂等 + RESTART IDENTITY 复位序列；版本迁移按 Version 去重只跑一次。
		{
			Version:     "20260718_0001",
			Description: "TRUNCATE topic_enrichment_result/topic_enrichment_review — stale 演进定位 (position/signals/verdict) semantics retired for causal-analysis-agent (探索判断). Destructive — guarded by MIGRATIONS_ALLOW_DESTRUCTIVE. ⚠️ 不可逆 TRUNCATE（破坏性，受 `MIGRATIONS_ALLOW_DESTRUCTIVE` 守卫；生产执行前需备份）",
			Up: func(db *gorm.DB) error {
				// Destructive (TRUNCATE): skip unless MIGRATIONS_ALLOW_DESTRUCTIVE=1.
				if !IsDestructiveAllowed() {
					logging.Warnf("skipping destructive migration 20260718_0001 (set MIGRATIONS_ALLOW_DESTRUCTIVE=1 to enable)")
					return nil
				}
				// 两张表由 AutoMigrate 在版本迁移前一并创建；result 表不存在则无需清理。
				if !tableExists(db, "topic_enrichment_result") {
					return nil
				}
				// 这两张表无 DB 级 FK（模型未声明 GORM relation），无需 CASCADE。
				if err := db.Exec("TRUNCATE TABLE topic_enrichment_result, topic_enrichment_review RESTART IDENTITY").Error; err != nil {
					return fmt.Errorf("truncate enrichment stale data: %w", err)
				}
				return nil
			},
		},

		// ── causal-analysis-agent 阶段2b: qa 沉淀标记列 ──
		// topic_enrichment_qa 加 sedimented 列（用户手动沉淀某轮追问为持久笔记）。
		// 沉淀只翻转 qa 行上的标志，绝不改 topic_enrichment_result 主表（业务约束#2）。
		// 幂等：ADD COLUMN IF NOT EXISTS + 版本迁移按 Version 去重只跑一次。
		{
			Version:     "20260719_0001",
			Description: "Add topic_enrichment_qa.sedimented column (user-pinned Q&A note flag; report itself stays immutable). Idempotent.",
			Up: func(db *gorm.DB) error {
				if !tableExists(db, "topic_enrichment_qa") {
					return nil
				}
				if err := db.Exec(`ALTER TABLE topic_enrichment_qa ADD COLUMN IF NOT EXISTS sedimented BOOLEAN NOT NULL DEFAULT false`).Error; err != nil {
					return fmt.Errorf("add topic_enrichment_qa.sedimented column: %w", err)
				}
				return nil
			},
		},

		// ── model tag 治理：把 NOT NULL/DEFAULT 约束从 GORM model tag 收敛到显式迁移 ──
		// 背景：见 standard/backend/code-style.md「GORM model tag 与迁移」。model tag 写
		// not null/default 会让 AutoMigrate 与显式迁移竞争（ai-call-logging-schema 事故）。
		// 本迁移把 ai_models/topic_graph/semantic_label 三个文件的列级约束落地到 DB，
		// 之后即可从 model tag 安全移除 not null/default（让显式迁移唯一管约束）。
		// 幂等：SET DEFAULT 本身幂等；SET NOT NULL 经 columnIsNullable 检查只在可空时执行。
		{
			Version:     "20260723_0001",
			Description: "Materialize NOT NULL/DEFAULT constraints (previously driven by GORM tags) for ai_models/topic_graph/semantic_label tables, so model tags can be stripped. Idempotent.",
			Up: func(db *gorm.DB) error {
				// helper: set DEFAULT then backfill+NOT NULL for one column.
				// Column-existence guard（ai-provider-model-overhaul）：ai_routes 的
				// priority/strategy 已按 D2 死列收口从 model 删除，全新库（RunAutoMigrate
				// 建表）不再有这两列——跳过不报错；存量库列在，照常落约束。
				constrain := func(table, column, defaultLit string, notNull bool) error {
					var colExists bool
					if err := db.Raw(`SELECT EXISTS (
						SELECT 1 FROM information_schema.columns
						WHERE table_name = ? AND column_name = ?
					)`, table, column).Scan(&colExists).Error; err != nil {
						return fmt.Errorf("check column %s.%s: %w", table, column, err)
					}
					if !colExists {
						return nil
					}
					if defaultLit != "" {
						if err := db.Exec(fmt.Sprintf(`ALTER TABLE %s ALTER COLUMN %s SET DEFAULT %s`, table, column, defaultLit)).Error; err != nil {
							return fmt.Errorf("set default %s.%s: %w", table, column, err)
						}
						// respect the notNull flag — the previous unconditional call
						// silently forced NOT NULL on columns declared nullable
						// (restore-gorm-default-tags: context_layers/aliases).
						if notNull {
							if err := ensureNotNullDefault(db, table, column, defaultLit); err != nil {
								return err
							}
						}
					}
					if notNull && defaultLit == "" {
						// No default: cannot backfill generically; rely on ensureNotNullDefault's
						// empty-string backfill only for text-like columns. Caller passes "" to skip.
						if err := ensureNotNullDefault(db, table, column, "''"); err != nil {
							return err
						}
					}
					return nil
				}

				type col struct {
					table, column, def string
					notNull            bool
				}
				// Columns with a DEFAULT value: backfill uses the default, then SET NOT NULL.
				cols := []col{
					// ── ai_models.go ──
					{"scheduler_tasks", "check_interval", "60", true},
					{"scheduler_tasks", "status", "'idle'", false},
					{"scheduler_tasks", "total_executions", "0", false},
					{"scheduler_tasks", "successful_executions", "0", false},
					{"scheduler_tasks", "failed_executions", "0", false},
					{"scheduler_tasks", "consecutive_failures", "0", false},
					{"ai_providers", "provider_type", "'openai_compatible'", true},
					{"ai_providers", "enabled", "true", true},
					{"ai_providers", "timeout_seconds", "120", true},
					{"ai_providers", "enable_thinking", "false", true},
					{"ai_routes", "enabled", "true", true},
					{"ai_routes", "priority", "100", true},
					{"ai_routes", "strategy", "'ordered_failover'", true},
					{"ai_routes", "max_concurrency", "0", true},
					{"ai_route_providers", "priority", "100", true},
					{"ai_route_providers", "enabled", "true", true},
					{"ai_call_logs", "is_fallback", "false", true},
					// ── topic_graph.go ──
					{"topic_tags", "category", "'keyword'", true},
					{"topic_tags", "is_canonical", "false", false},
					{"topic_tags", "source", "'llm'", false},
					{"topic_tags", "feed_count", "0", false},
					{"topic_tags", "status", "'active'", true},
					{"topic_tags", "is_watched", "false", false},
					{"topic_tags", "quality_score", "0", false},
					{"topic_tags", "metadata", "'{}'::jsonb", false},
					{"topic_tags", "kind", "'keyword'", false},
					{"topic_tag_embeddings", "embedding_type", "'identity'", true},
					{"tag_merge_suggestions", "status", "'pending'", true},
					{"tag_merge_suggestions", "source", "'incremental'", true},
					{"article_topic_tags", "score", "0", false},
					{"article_topic_tags", "source", "'llm'", false},
					// ── semantic_label.go ──
					{"semantic_labels", "ref_count", "0", true},
					{"semantic_labels", "display_order", "0", true},
					{"semantic_labels", "source", "'llm_extract'", true},
					{"semantic_labels", "status", "'active'", true},
					{"semantic_labels", "protected", "false", true},
					{"semantic_labels", "enrichment_enabled", "false", true},
					{"semantic_labels", "window_days", "14", true},
					{"semantic_labels", "aliases", "'[]'::jsonb", false},
					{"semantic_labels", "context_layers", `'["week","month","year","all"]'::jsonb`, false},
					{"topic_tag_board_labels", "score", "0", true},
					// downgraded 列已于 2026-11-12 随 retire-downgraded-mount 退役删除
					{"topic_tag_board_labels", "direction_mismatch", "false", true},
				}
				for _, c := range cols {
					if !tableExists(db, c.table) {
						continue
					}
					if err := constrain(c.table, c.column, c.def, c.notNull); err != nil {
						return err
					}
				}

				// Columns with NOT NULL but NO default: string columns backfill '' then NOT NULL.
				// FK/PK uint columns and bool(success)/float(similarity/dimension) cannot be
				// safely backfilled with a blanket value — these tables are populated only via
				// code paths that always set the value, so on a healthy DB they have no NULLs.
				// We still SET NOT NULL (idempotent via columnIsNullable); if NULLs exist the
				// migration fails loudly (preferred over silent constraint vacuum).
				notNullOnly := []col{
					{"scheduler_tasks", "name", "", true},
					{"ai_settings", "key", "", true},
					{"ai_providers", "name", "", true},
					{"ai_providers", "base_url", "", true},
					{"ai_providers", "model", "", true},
					{"ai_routes", "name", "", true},
					{"ai_routes", "capability", "", true},
					{"ai_route_providers", "route_id", "0", true},
					{"ai_route_providers", "provider_id", "0", true},
					{"ai_call_logs", "capability", "", true},
					{"ai_call_logs", "route_name", "", true},
					{"ai_call_logs", "provider_name", "", true},
					{"ai_call_logs", "success", "false", true},
					{"topic_tags", "slug", "", true},
					{"topic_tags", "label", "", true},
					{"topic_tag_embeddings", "topic_tag_id", "0", true},
					{"topic_tag_embeddings", "dimension", "0", true},
					{"topic_tag_embeddings", "model", "", true},
					{"tag_merge_suggestions", "new_tag_id", "0", true},
					{"tag_merge_suggestions", "existing_tag_id", "0", true},
					{"tag_merge_suggestions", "new_label", "", true},
					{"tag_merge_suggestions", "existing_label", "", true},
					{"tag_merge_suggestions", "category", "", true},
					{"tag_merge_suggestions", "similarity", "0", true},
					{"article_topic_tags", "article_id", "0", true},
					{"article_topic_tags", "topic_tag_id", "0", true},
					{"semantic_labels", "label", "", true},
					{"semantic_labels", "slug", "", true},
					{"semantic_labels", "label_type", "", true},
				}
				for _, c := range notNullOnly {
					if !tableExists(db, c.table) {
						continue
					}
					// String columns: backfill '' (empty); numeric/bool: backfill the literal.
					backfill := "''"
					if c.def != "" {
						backfill = c.def
					}
					if err := ensureNotNullDefault(db, c.table, c.column, backfill); err != nil {
						return err
					}
				}
				return nil
			},
		},
		{
			Version:     "20260724_0001",
			Description: "Re-apply DEFAULT+NOT NULL on status columns (topic_tags/tag_merge_suggestions/semantic_labels). 20260723_0001 intended this after stripping GORM default tags, but a DB rebuilt afterwards left column_default empty, producing empty/NULL status rows that broke strict status filters (e.g. board match tt.status='active'). Also backfills empty-string '' here — 20260723_0001's ensureNotNullDefault only handled NULL. Idempotent.",
			Up: func(db *gorm.DB) error {
				targets := []struct{ table, column, def string }{
					{"topic_tags", "status", "'active'"},
					{"tag_merge_suggestions", "status", "'pending'"},
					{"semantic_labels", "status", "'active'"},
				}
				for _, t := range targets {
					if !tableExists(db, t.table) {
						continue
					}
					if err := db.Exec(fmt.Sprintf(`ALTER TABLE %s ALTER COLUMN %s SET DEFAULT %s`, t.table, t.column, t.def)).Error; err != nil {
						return fmt.Errorf("set default %s.%s: %w", t.table, t.column, err)
					}
					if err := db.Exec(fmt.Sprintf(`UPDATE %s SET %s = %s WHERE %s IS NULL OR %s = ''`, t.table, t.column, t.def, t.column, t.column)).Error; err != nil {
						return fmt.Errorf("backfill %s.%s: %w", t.table, t.column, err)
					}
					if err := db.Exec(fmt.Sprintf(`ALTER TABLE %s ALTER COLUMN %s SET NOT NULL`, t.table, t.column)).Error; err != nil {
						return fmt.Errorf("set %s.%s NOT NULL: %w", t.table, t.column, err)
					}
				}
				return nil
			},
		},
		// ── preference-vector-feed-discovery: DROP 旧 user_preferences 表（D9，纯派生数据无迁移负担）──
		{
			Version:     "20260725_0001",
			Description: "DROP user_preferences table — old preference feature removed (preference-vector-feed-discovery D9). Destructive — guarded by MIGRATIONS_ALLOW_DESTRUCTIVE. user_preferences 为行为派生数据，可由 reading_behaviors 重算重建。",
			Up: func(db *gorm.DB) error {
				if !IsDestructiveAllowed() {
					logging.Warnf("skipping destructive migration 20260725_0001 (set MIGRATIONS_ALLOW_DESTRUCTIVE=1 to enable)")
					return nil
				}
				if tableExists(db, "user_preferences") {
					if err := db.Exec("DROP TABLE IF EXISTS user_preferences CASCADE").Error; err != nil {
						return fmt.Errorf("drop user_preferences: %w", err)
					}
				}
				return nil
			},
		},

		// ── daily-report-lane-driven-clustering: 质心 + 吸尘器 + lane 列 ──
		// 纯加法迁移：board_persistent_topics 增 centroid/is_vacuum/vacuum_strong/
		// vacuum_mid；daily_report_sections 增 lane_tier；seed 6 个 persistent_topic_*
		// 阈值；离线回填 centroid（近 30 条 section 均权 AVG）与 vacuum 统计（近 7 天
		// strong/mid 计数）。全部 IF NOT EXISTS / check-existing / SET=计算，重跑幂等。
		// 阈值常量（0.18/0.30/0.20/30/7）与 DefaultPersistentTopicConfig 对齐，运行时可被
		// ai_settings 覆盖；离线回填用默认值即可（迁移是一次性 snapshot）。
		{
			Version:     "20260727_0001",
			Description: "Add centroid/is_vacuum/vacuum_strong/vacuum_mid to board_persistent_topics, lane_tier to daily_report_sections; seed lane/vacuum/centroid config; offline-backfill centroid + vacuum stats (daily-report-lane-driven-clustering).",
			Up: func(db *gorm.DB) error {
				if !tableExists(db, "board_persistent_topics") || !tableExists(db, "daily_report_sections") {
					return nil // daily-report tables not registered on this deployment
				}

				// 1. ADD COLUMN IF NOT EXISTS (idempotent). centroid 用无维度 vector，
				//    运行时由 ensurePersistentTopicEmbeddingDimension 同步维度。
				addCols := []string{
					`ALTER TABLE board_persistent_topics ADD COLUMN IF NOT EXISTS centroid vector`,
					`ALTER TABLE board_persistent_topics ADD COLUMN IF NOT EXISTS is_vacuum boolean NOT NULL DEFAULT false`,
					`ALTER TABLE board_persistent_topics ADD COLUMN IF NOT EXISTS vacuum_strong integer NOT NULL DEFAULT 0`,
					`ALTER TABLE board_persistent_topics ADD COLUMN IF NOT EXISTS vacuum_mid integer NOT NULL DEFAULT 0`,
					`ALTER TABLE daily_report_sections ADD COLUMN IF NOT EXISTS lane_tier varchar(16)`,
				}
				for _, stmt := range addCols {
					if err := db.Exec(stmt).Error; err != nil {
						return fmt.Errorf("add lane/centroid column: %w", err)
					}
				}

				// 2. Seed ai_settings (check-existing-then-create，沿用 20260619_0002 模式)。
				laneDefaults := []struct {
					Key, Value, Description string
				}{
					{"persistent_topic_lane_l1_threshold", "0.18", "Lane L1 直挂阈值：tag 到 topic 质心余弦距离 < 此值直接归属（不调 LLM）"},
					{"persistent_topic_lane_l2_threshold", "0.30", "Lane L2 弱区上限：距离在 [l1,l2] 交 LLM 留/换/新"},
					{"persistent_topic_vacuum_ratio", "0.20", "吸尘器判定：strong/(strong+mid) < 此值则 is_vacuum=true"},
					{"persistent_topic_centroid_window", "30", "质心计算取最近 N 条 section embedding 加权平均"},
					{"persistent_topic_vacuum_window", "7", "吸尘器吸引统计窗口（天）"},
					{"persistent_topic_l2_candidate_k", "5", "L2 LLM 注入的 top-K 候选 topic 数"},
				}
				for _, d := range laneDefaults {
					var existing models.AISettings
					if err := db.Where("key = ?", d.Key).First(&existing).Error; err == nil {
						continue
					}
					if err := db.Create(&models.AISettings{
						Key: d.Key, Value: d.Value, Description: d.Description,
					}).Error; err != nil {
						return fmt.Errorf("seed ai_settings key %s: %w", d.Key, err)
					}
				}

				// 3. 离线回填 centroid：取每个 topic 近 30 条 section embedding 的均权
				//    AVG（pgvector >= 0.5 的 avg(vector)）。section<2 或无 section 的 topic
				//    子查询返回 NULL，保留 NULL（运行时 ComputeTopicCentroid 退化首义）。
				if err := db.Exec(`
					UPDATE board_persistent_topics t
					SET centroid = sub.c
					FROM (
						SELECT t2.id,
						       (
						           SELECT AVG(recent.embedding)
						           FROM (
						               SELECT s.embedding
						               FROM daily_report_sections s
						               JOIN board_daily_reports rpt ON rpt.id = s.report_id
						               WHERE s.persistent_topic_id = t2.id
						                 AND s.embedding IS NOT NULL
						               ORDER BY rpt.period_date DESC, s.id DESC
						               LIMIT 30
						           ) recent
						       ) AS c
						FROM board_persistent_topics t2
					) sub
					WHERE t.id = sub.id AND sub.c IS NOT NULL
				`).Error; err != nil {
					logging.Warnf("migration 20260727_0001: centroid AVG backfill failed (pgvector avg unavailable?), leaving centroid NULL for runtime fallback: %v", err)
					// non-fatal: ComputeTopicCentroid degrades to first-section vector
				}

				// 4. 初始化 vacuum 统计（近 7 天 strong/mid 计数 + is_vacuum）。
				//    strong=distance<0.18, mid=distance∈[0.18,0.30]；
				//    is_vacuum = strong/(strong+mid) < 0.20（CASE 防除零）。
				if err := db.Exec(`
					UPDATE board_persistent_topics t
					SET vacuum_strong = sub.strong,
					    vacuum_mid = sub.mid,
					    is_vacuum = CASE
					        WHEN sub.strong + sub.mid > 0
					        THEN sub.strong::float / (sub.strong + sub.mid)::float < 0.20
					        ELSE false
					    END
					FROM (
					    SELECT t2.id,
					           COUNT(*) FILTER (WHERE s.topic_match_distance < 0.18) AS strong,
					           COUNT(*) FILTER (WHERE s.topic_match_distance >= 0.18 AND s.topic_match_distance <= 0.30) AS mid
					    FROM board_persistent_topics t2
					    JOIN daily_report_sections s ON s.persistent_topic_id = t2.id
					    JOIN board_daily_reports rpt ON rpt.id = s.report_id
					    WHERE t2.status IN ('candidate', 'active')
					      AND rpt.period_date >= (CURRENT_DATE - 7)
					    GROUP BY t2.id
					) sub
					WHERE t.id = sub.id
				`).Error; err != nil {
					return fmt.Errorf("init vacuum stats: %w", err)
				}
				return nil
			},
		},

		// ── feed-param-options: route_param_options 人工字典 seed ───
		// 可选值源自 RSSHub 源码 description 表格（docs.rsshub.app 不可达绕行 GitHub raw）。
		// 幂等：ON CONFLICT (route_id, param_name, value) DO NOTHING，服务重启/迁移重跑安全。
		{
			Version:     "20260801_0001",
			Description: "Seed route_param_options manual dictionary (feed-param-options): qbitai/tencent/ithome ranking+tw/36kr param enums from RSSHub source.",
			Up: func(db *gorm.DB) error {
				if !tableExists(db, "route_param_options") {
					return nil
				}
				stmts := []string{
					`INSERT INTO route_param_options (route_id, param_name, value, label, source, created_at, updated_at)
SELECT r.id, 'category', v.value, v.label, 'manual', now(), now()
FROM rsshub_routes r CROSS JOIN (VALUES
	('资讯','资讯'),('ebandeng','数码'),('auto','智能车'),('zhiku','智库'),('huodong','活动')
) AS v(value,label)
WHERE r.namespace='qbitai' AND r.path='/category/:category'
ON CONFLICT (route_id, param_name, value) DO NOTHING`,
					`INSERT INTO route_param_options (route_id, param_name, value, label, source, created_at, updated_at)
SELECT r.id, 'type', v.value, v.label, 'manual', now(), now()
FROM rsshub_routes r CROSS JOIN (VALUES
	('all','全部'),('rm','热门'),('xw','新闻'),('gg','公告'),('hd','活动'),('ss','赛事'),('yh','优化')
) AS v(value,label)
WHERE r.namespace='tencent' AND r.path='/pvp/newsindex/:type'
ON CONFLICT (route_id, param_name, value) DO NOTHING`,
					`INSERT INTO route_param_options (route_id, param_name, value, label, source, created_at, updated_at)
SELECT r.id, 'type', v.value, v.label, 'manual', now(), now()
FROM rsshub_routes r CROSS JOIN (VALUES
	('24h','24小时阅读榜'),('7days','7天最热'),('monthly','月榜')
) AS v(value,label)
WHERE r.namespace='ithome' AND r.path='/ranking/:type'
ON CONFLICT (route_id, param_name, value) DO NOTHING`,
					`INSERT INTO route_param_options (route_id, param_name, value, label, source, created_at, updated_at)
SELECT r.id, 'category', v.value, v.label, 'manual', now(), now()
FROM rsshub_routes r CROSS JOIN (VALUES
	('news','新聞'),('big-data','AI'),('cloud','Cloud'),('devops','DevOps'),('security','資安')
) AS v(value,label)
WHERE r.namespace='ithome' AND r.path='/tw/feeds/:category'
ON CONFLICT (route_id, param_name, value) DO NOTHING`,
					`INSERT INTO route_param_options (route_id, param_name, value, label, source, created_at, updated_at)
SELECT r.id, 'category', v.value, v.label, 'manual', now(), now()
FROM rsshub_routes r CROSS JOIN (VALUES
	('news','最新资讯频道'),('newsflashes','快讯'),('recommend','推荐资讯'),('life','生活'),('estate','房产'),('workplace','职场')
) AS v(value,label)
WHERE r.namespace='36kr' AND r.path='/:category/:subCategory?/:keyword?'
ON CONFLICT (route_id, param_name, value) DO NOTHING`,
				}
				for _, s := range stmts {
					if err := db.Exec(s).Error; err != nil {
						return fmt.Errorf("seed route_param_options: %w", err)
					}
				}
				return nil
			},
		},

		// ── fix-watch-delete-cascade: topic_watch_hits FK ON DELETE CASCADE ──
		// GORM model tag 已声明 OnDelete:CASCADE（daily_report_models.go），但
		// DisableForeignKeyConstraintWhenMigrating=true 致 AutoMigrate 不建 FK，
		// 20260630_0001/_0002 也漏建——DeleteWatch 在 PG 上留下孤儿 hits。本迁移
		// 补齐真实 DB FK，对齐 model tag 意图。详见 openspec/changes/fix-watch-delete-cascade。
		// 幂等：孤儿清理天然幂等 + ADD CONSTRAINT 用 IF NOT EXISTS 守卫。
		{
			Version:     "20260801_0002",
			Description: "Add FK ON DELETE CASCADE on topic_watch_hits(watch_id)→board_topic_watches(id); clean orphan hits first. Idempotent.",
			Up: func(db *gorm.DB) error {
				if !tableExists(db, "topic_watch_hits") || !tableExists(db, "board_topic_watches") {
					return nil // watch tables not registered on this deployment
				}

				// 1. 清理孤儿 hits（引用已删 watch 的行）。必须无条件执行——否则
				//    ADD CONSTRAINT FK 校验现有行失败 → 迁移报错 → 启动失败。孤儿是
				//    垃圾数据（引用不存在的 watch），不套 IsDestructiveAllowed 守卫
				//    （守卫按 db-migration-safety spec 只留给 TRUNCATE/DROP）。
				res := db.Exec(`DELETE FROM topic_watch_hits WHERE watch_id NOT IN (SELECT id FROM board_topic_watches)`)
				if res.Error != nil {
					return fmt.Errorf("clean orphan topic_watch_hits: %w", res.Error)
				}
				if res.RowsAffected > 0 {
					logging.Infof("Migration 20260801_0002: cleaned %d orphan topic_watch_hits rows", res.RowsAffected)
				}

				// 2. 加 FK ON DELETE CASCADE（幂等 IF NOT EXISTS 守卫）。ADD CONSTRAINT
				//    FOREIGN KEY 与 UNIQUE 同构（AccessExclusiveLock + 全表行校验），
				//    按 db-migration-execution spec「长锁 DDL」套 withLockTimeout 守卫。
				if err := withLockTimeout(db, "5s", func(tx *gorm.DB) error {
					if err := tx.Exec(`DO $$ BEGIN
							IF NOT EXISTS (
								SELECT 1 FROM information_schema.table_constraints
								WHERE constraint_name = 'fk_topic_watch_hits_watch'
								  AND table_name = 'topic_watch_hits'
							) THEN
								ALTER TABLE topic_watch_hits
									ADD CONSTRAINT fk_topic_watch_hits_watch
									FOREIGN KEY (watch_id) REFERENCES board_topic_watches(id)
									ON DELETE CASCADE;
							END IF;
						END $$`).Error; err != nil {
						return fmt.Errorf("add fk_topic_watch_hits_watch: %w", err)
					}
					return nil
				}); err != nil {
					return err
				}
				return nil
			},
		},

		// ── AI provider model_kind backfill ────────────────────────
		// AutoMigrate adds ai_providers.model_kind (DEFAULT 'llm'). Existing
		// providers therefore all start at 'llm'. This one-shot data migration
		// flips providers that are EXCLUSIVELY on an embedding route to
		// 'embedding', and warns about providers bound to BOTH an embedding and an
		// llm route (ambiguous — the user must split the bindings manually; the
		// migration does not auto-change routes). Idempotent: the UPDATE only
		// touches rows still at 'llm'.
		{
			Version:     "20260802_0001",
			Description: "Backfill ai_providers.model_kind from embedding route membership; warn on embedding+llm conflict.",
			Up: func(db *gorm.DB) error {
				// Backfill: flip a provider to 'embedding' only if it is on an
				// embedding route AND not on any llm (non-embedding) route. A
				// provider on both routes is ambiguous — leave it at the default
				// 'llm' and warn below so the user decides.
				res := db.Exec(`
					UPDATE ai_providers p
					SET model_kind = 'embedding'
					WHERE p.model_kind = 'llm'
					  AND EXISTS (
					    SELECT 1 FROM ai_route_providers arp
					    JOIN ai_routes ar ON ar.id = arp.route_id
					    WHERE arp.provider_id = p.id AND ar.capability = 'embedding'
					  )
					  AND NOT EXISTS (
					    SELECT 1 FROM ai_route_providers arp
					    JOIN ai_routes ar ON ar.id = arp.route_id
					    WHERE arp.provider_id = p.id AND ar.capability <> 'embedding'
					  )
				`)
				if res.Error != nil {
					return fmt.Errorf("backfill ai_providers.model_kind: %w", res.Error)
				}
				if res.RowsAffected > 0 {
					logging.Infof("Migration 20260802_0001: backfilled %d embedding-exclusive providers to model_kind=embedding", res.RowsAffected)
				}

				// Conflict detection: providers bound to BOTH an embedding route and
				// an llm route. Warn only — do not auto-change route bindings.
				type conflictRow struct {
					ID   uint
					Name string
				}
				var conflicts []conflictRow
				if err := db.Raw(`
					SELECT p.id, p.name FROM ai_providers p
					WHERE EXISTS (
					    SELECT 1 FROM ai_route_providers arp
					    JOIN ai_routes ar ON ar.id = arp.route_id
					    WHERE arp.provider_id = p.id AND ar.capability = 'embedding'
					  )
					  AND EXISTS (
					    SELECT 1 FROM ai_route_providers arp
					    JOIN ai_routes ar ON ar.id = arp.route_id
					    WHERE arp.provider_id = p.id AND ar.capability <> 'embedding'
					  )
				`).Scan(&conflicts).Error; err != nil {
					return fmt.Errorf("detect embedding+llm provider conflicts: %w", err)
				}
				for _, c := range conflicts {
					logging.Warnf("migration 20260802_0001: provider %d (%s) 同时绑定在 embedding 路由与 llm 路由，model_kind 冲突，请手动拆分路由绑定", c.ID, c.Name)
				}
				return nil
			},
		},

		// ── Article archive flag ──────────────────────────────────────
		// CleanupOldArticles switches from physical DELETE to archive
		// (UPDATE archived=true). All existing rows start as active — no
		// backfill. No index: low-cardinality boolean, list queries are
		// covered by feed_id/pub_date indexes (design D2).
		{
			Version:     "20260818_0001",
			Description: "Add articles.archived boolean NOT NULL DEFAULT false — feed cleanup archives instead of deleting (article-archive-instead-of-delete).",
			Up: func(db *gorm.DB) error {
				if err := db.Exec(`ALTER TABLE articles ADD COLUMN IF NOT EXISTS archived boolean NOT NULL DEFAULT false`).Error; err != nil {
					return fmt.Errorf("add articles.archived: %w", err)
				}
				return nil
			},
		},

		// ── analysis-remediation W1：孤儿 embedding 清理 + FK 防复发 + 零使用索引清理 ──
		// GORM 声明了 OnDelete:CASCADE 但 DB 实际无此 FK，删 tag 后向量全部残留
		// （256k 孤儿行 ≈3GB，已由 scripts/db-cleanup-2026-08/ 手动清理）。本迁移在
		// 新部署上完成同样的清理+结构修复（幂等）。索引清理与 tracing/model.go 的
		// tag 摘除配套，否则 AutoMigrate 会重建。articles GIN 全文索引 idx_scan=0
		// 且代码零引用（analysis-reports/db-analysis-2026-08-20.md #3/#4）。
		{
			Version:     "20260820_0001",
			Description: "Clean orphan topic_tag_embeddings, add FK ON DELETE CASCADE to topic_tags, drop unused indexes (articles GIN + trigger, otel_spans trace_id/kind/status/name). Idempotent.",
			Up: func(db *gorm.DB) error {
				if !tableExists(db, "topic_tag_embeddings") || !tableExists(db, "topic_tags") {
					return nil
				}

				// 1) 清孤儿 embedding（同 20260801_0002 孤儿先清逻辑：垃圾数据不套
				// destructive 守卫，不清则 FK 校验现有行失败）
				res := db.Exec(`DELETE FROM topic_tag_embeddings WHERE topic_tag_id NOT IN (SELECT id FROM topic_tags)`)
				if res.Error != nil {
					return fmt.Errorf("clean orphan topic_tag_embeddings: %w", res.Error)
				}
				if res.RowsAffected > 0 {
					logging.Infof("Migration 20260820_0001: cleaned %d orphan topic_tag_embeddings rows", res.RowsAffected)
				}

				// 2) FK ON DELETE CASCADE（幂等；长锁 DDL 套 withLockTimeout）
				if err := withLockTimeout(db, "5s", func(tx *gorm.DB) error {
					if err := tx.Exec(`DO $$ BEGIN
						IF NOT EXISTS (
							SELECT 1 FROM information_schema.table_constraints
							WHERE constraint_name = 'fk_topic_tag_embeddings_tag'
							  AND table_name = 'topic_tag_embeddings'
						) THEN
							ALTER TABLE topic_tag_embeddings
								ADD CONSTRAINT fk_topic_tag_embeddings_tag
								FOREIGN KEY (topic_tag_id) REFERENCES topic_tags(id)
								ON DELETE CASCADE;
						END IF;
					END $$`).Error; err != nil {
						return fmt.Errorf("add fk_topic_tag_embeddings_tag: %w", err)
					}
					return nil
				}); err != nil {
					return err
				}

				// 3) 零使用索引清理（与 scripts/db-cleanup-2026-08/apply-structure.sql 一致）
				if tableExists(db, "articles") {
					if err := db.Exec(`DROP INDEX IF EXISTS idx_articles_search_vector`).Error; err != nil {
						return fmt.Errorf("drop idx_articles_search_vector: %w", err)
					}
					if err := db.Exec(`DROP TRIGGER IF EXISTS articles_search_vector_trigger ON articles`).Error; err != nil {
						return fmt.Errorf("drop articles_search_vector_trigger: %w", err)
					}
				}
				if tableExists(db, "otel_spans") {
					for _, idx := range []string{"idx_otel_spans_trace_id", "idx_otel_spans_kind", "idx_otel_spans_status", "idx_otel_spans_name"} {
						if err := db.Exec(fmt.Sprintf(`DROP INDEX IF EXISTS %s`, idx)).Error; err != nil {
							return fmt.Errorf("drop %s: %w", idx, err)
						}
					}
				}
				return nil
			},
		},

		// ── analysis-remediation W1：embedding_queues 30 天保留策略的查询支撑 ──
		// job_log_cleanup 新增 DELETE ... WHERE status='completed' AND created_at < ?，
		// 此部分索引（completed 行 15 万+，全体行不到 16 万）支撑该删除扫描。
		{
			Version:     "20260820_0002",
			Description: "Add partial index idx_embedding_queues_completed_created (created_at WHERE status='completed') to back the 30-day retention cleanup in job_log_cleanup. Idempotent.",
			Up: func(db *gorm.DB) error {
				if !tableExists(db, "embedding_queues") {
					return nil
				}
				if err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_embedding_queues_completed_created
					ON embedding_queues (created_at) WHERE status = 'completed'`).Error; err != nil {
					return fmt.Errorf("create idx_embedding_queues_completed_created: %w", err)
				}
				return nil
			},
		},

		// ── retire-narrative-legacy: drop dead narrative dual-track tables ──
		{
			Version:     "20260824_0001",
			Description: "Drop legacy narrative_summaries / narrative_boards (zero production writers since the daily-report pipeline took over; dead data, no downgrade path). Destructive — guarded by MIGRATIONS_ALLOW_DESTRUCTIVE per db-migration-safety spec (skip + WARN when not enabled). ⚠️ 不可逆 DROP（破坏性，受 `MIGRATIONS_ALLOW_DESTRUCTIVE` 守卫；生产执行前需备份）",
			Up: func(db *gorm.DB) error {
				// Destructive (DROP TABLE): skip unless MIGRATIONS_ALLOW_DESTRUCTIVE=1
				// (db-migration-safety spec: skip + WARN + record version, do not abort startup).
				if !IsDestructiveAllowed() {
					logging.Warnf("skipping destructive migration 20260824_0001 (set MIGRATIONS_ALLOW_DESTRUCTIVE=1 to drop legacy narrative tables)")
					return nil
				}
				tableDrops := []string{
					"DROP TABLE IF EXISTS narrative_summaries CASCADE",
					"DROP TABLE IF EXISTS narrative_boards CASCADE",
				}
				for _, s := range tableDrops {
					if err := db.Exec(s).Error; err != nil {
						return fmt.Errorf("drop legacy narrative table: %w", err)
					}
				}
				return nil
			},
		},

		// ── watch-keyword-and-quickadd: board_topic_watches.type column ──
		// Dual-track watch matching: 'label' (AI semantic, historical rows) vs
		// 'keyword' (pure text). AutoMigrate creates the column from the model tag
		// on fresh installs; this migration owns the CHECK constraint
		// (type IN ('label','keyword')) that AutoMigrate cannot express, and
		// guarantees NOT NULL + default 'label' on legacy databases. Idempotent.
		{
			Version:     "20260824_0002",
			Description: "Add board_topic_watches.type column (label|keyword) with CHECK constraint, default 'label'.",
			Up: func(db *gorm.DB) error {
				if !tableExists(db, "board_topic_watches") {
					return nil
				}
				// Add column with default 'label' (idempotent via IF NOT EXISTS;
				// mirrors the 20260702_0001 source-column pattern).
				if err := db.Exec(`
					ALTER TABLE board_topic_watches
					ADD COLUMN IF NOT EXISTS type VARCHAR(10) DEFAULT 'label'
				`).Error; err != nil {
					return fmt.Errorf("add board_topic_watches.type column: %w", err)
				}
				// Backfill + SET NOT NULL via the idempotent helper (no-ops when
				// the column is already NOT NULL, e.g. AutoMigrate created it).
				if err := ensureNotNullDefault(db, "board_topic_watches", "type", "'label'"); err != nil {
					return fmt.Errorf("board_topic_watches.type NOT NULL: %w", err)
				}
				// Add CHECK constraint (idempotent: information_schema guard first,
				// same pattern as 20260630_0001 status CHECK).
				if err := db.Exec(`
					DO $$ BEGIN
						IF NOT EXISTS (
							SELECT 1 FROM information_schema.table_constraints
							WHERE constraint_name = 'chk_board_topic_watches_type'
								  AND table_name = 'board_topic_watches'
						) THEN
							ALTER TABLE board_topic_watches
								ADD CONSTRAINT chk_board_topic_watches_type
								CHECK (type IN ('label', 'keyword'));
						END IF;
					END $$
				`).Error; err != nil {
					return fmt.Errorf("add board_topic_watches type CHECK: %w", err)
				}
				return nil
			},
		},

		// ── watch-materialized-topic: materialized watch tracks + sentence cache ──
		// watch type 扩展两值：keyword_topic（关键字物化——当天命中文章聚合为
		// 临时 section，零 AI）、sentence_topic（一句话向量检索辅助标签，物化为
		// 挂专属持久话题的 section）。type 列从 VARCHAR(10) 扩到 VARCHAR(16)
		// （'keyword_topic' 13 字符）；CHECK 重建为四值。sentence 轨新增
		// query（检索句）/ embedding_cache（向量缓存）/ persistent_topic_id
		// （专属话题 FK，ON DELETE SET NULL——话题被物理删除不断 watch，反向
		// 联动归档由 service 层显式做）。AutoMigrate 在新装库上建新列；本迁移
		// 兑底存量库并 owns CHECK/FK。幂等。
		{
			Version:     "20260825_0001",
			Description: "watch-materialized-topic: widen board_topic_watches.type CHECK to 4 values; add query/embedding_cache/persistent_topic_id.",
			Up: func(db *gorm.DB) error {
				if !tableExists(db, "board_topic_watches") {
					return nil
				}
				// 1. Widen type VARCHAR(10) → VARCHAR(16) ('keyword_topic' is 13
				//    chars). Idempotent: same-type ALTER is a no-op.
				if err := db.Exec(`ALTER TABLE board_topic_watches ALTER COLUMN type TYPE VARCHAR(16)`).Error; err != nil {
					return fmt.Errorf("widen board_topic_watches.type: %w", err)
				}
				// 2. Rebuild the type CHECK with the four-value set. DROP the
				//    20260824_0002 two-value constraint first, then ADD — guarded by
				//    withLockTimeout (constraint DDL takes AccessExclusiveLock,
				//    per db-migration-execution).
				if err := withLockTimeout(db, "5s", func(tx *gorm.DB) error {
					if err := tx.Exec(`ALTER TABLE board_topic_watches DROP CONSTRAINT IF EXISTS chk_board_topic_watches_type`).Error; err != nil {
						return fmt.Errorf("drop old board_topic_watches type CHECK: %w", err)
					}
					if err := tx.Exec(`DO $$ BEGIN
						IF NOT EXISTS (
							SELECT 1 FROM information_schema.table_constraints
							WHERE constraint_name = 'chk_board_topic_watches_type'
								  AND table_name = 'board_topic_watches'
						) THEN
							ALTER TABLE board_topic_watches
								ADD CONSTRAINT chk_board_topic_watches_type
								CHECK (type IN ('label', 'keyword', 'keyword_topic', 'sentence_topic'));
						END IF;
					END $$`).Error; err != nil {
						return fmt.Errorf("add board_topic_watches type CHECK: %w", err)
					}
					return nil
				}); err != nil {
					return err
				}
				// 3. New columns (idempotent; AutoMigrate also creates them on fresh
				//    installs from the model tags).
				if err := db.Exec(`ALTER TABLE board_topic_watches ADD COLUMN IF NOT EXISTS query TEXT`).Error; err != nil {
					return fmt.Errorf("add board_topic_watches.query: %w", err)
				}
				if err := db.Exec(`ALTER TABLE board_topic_watches ADD COLUMN IF NOT EXISTS embedding_cache vector`).Error; err != nil {
					return fmt.Errorf("add board_topic_watches.embedding_cache: %w", err)
				}
				if err := db.Exec(`ALTER TABLE board_topic_watches ADD COLUMN IF NOT EXISTS persistent_topic_id BIGINT`).Error; err != nil {
					return fmt.Errorf("add board_topic_watches.persistent_topic_id: %w", err)
				}
				// 4. FK persistent_topic_id → board_persistent_topics(id) ON DELETE
				//    SET NULL: deleting a topic must not delete or break the watch.
				if !tableExists(db, "board_persistent_topics") {
					return nil
				}
				if err := withLockTimeout(db, "5s", func(tx *gorm.DB) error {
					if err := tx.Exec(`DO $$ BEGIN
						IF NOT EXISTS (
							SELECT 1 FROM information_schema.table_constraints
							WHERE constraint_name = 'fk_board_topic_watches_topic'
								  AND table_name = 'board_topic_watches'
						) THEN
							ALTER TABLE board_topic_watches
								ADD CONSTRAINT fk_board_topic_watches_topic
								FOREIGN KEY (persistent_topic_id) REFERENCES board_persistent_topics(id)
								ON DELETE SET NULL;
						END IF;
					END $$`).Error; err != nil {
						return fmt.Errorf("add fk_board_topic_watches_topic: %w", err)
					}
					return nil
				}); err != nil {
					return err
				}
				return nil
			},
		},

		// ── board-level-deep-analysis ──────────────────────────────
		// Topic-scoped rows keep persistent_topic_id; board-scoped rows carry
		// semantic_board_id + analysis_scope='board' and leave topic NULL.
		// AutoMigrate adds the new columns/table, but it cannot DROP an existing
		// NOT NULL — that needs explicit DDL.
		{
			Version:     "20260826_0001",
			Description: "board-level-deep-analysis: backfill topic_enrichment_result.analysis_scope='topic', drop NOT NULL on persistent_topic_id (board-scope rows are NULL).",
			Up: func(db *gorm.DB) error {
				if !tableExists(db, "topic_enrichment_result") {
					return nil
				}
				// Backfill scope for pre-existing rows (AutoMigrate adds the column
				// with DEFAULT 'topic'; explicit backfill covers NULL edge from raw
				// SQL inserts).
				if err := db.Exec(`UPDATE topic_enrichment_result SET analysis_scope = 'topic' WHERE analysis_scope IS NULL OR analysis_scope = ''`).Error; err != nil {
					return fmt.Errorf("backfill analysis_scope: %w", err)
				}
				// Drop NOT NULL on both enrichment id columns (board-scope rows are
				// NULL). Guarded by withLockTimeout — constraint DDL takes
				// AccessExclusiveLock (per db-migration-execution).
				if err := withLockTimeout(db, "5s", func(tx *gorm.DB) error {
					if err := tx.Exec(`ALTER TABLE topic_enrichment_result ALTER COLUMN persistent_topic_id DROP NOT NULL`).Error; err != nil {
						return fmt.Errorf("drop NOT NULL topic_enrichment_result.persistent_topic_id: %w", err)
					}
					if tableExists(tx, "topic_enrichment_review") {
						if err := tx.Exec(`ALTER TABLE topic_enrichment_review ALTER COLUMN persistent_topic_id DROP NOT NULL`).Error; err != nil {
							return fmt.Errorf("drop NOT NULL topic_enrichment_review.persistent_topic_id: %w", err)
						}
					}
					return nil
				}); err != nil {
					return err
				}
				return nil
			},
		},

		// Seed the first reference role: 《内部看美国·方法论画像 v2》 extracted
		// from 7 full-video transcripts (docs/research/board-analysis-reference-role/).
		// Idempotent ON CONFLICT: re-runs and user edits survive; the row is a
		// starting point for the library, owned by the user from then on.
		{
			Version:     "20260826_0002",
			Description: "board-level-deep-analysis: seed first reference role (内部看美国·方法论画像 v2).",
			Up: func(db *gorm.DB) error {
				if !tableExists(db, "reference_roles") {
					return nil
				}
				if err := db.Exec(`INSERT INTO reference_roles (name, title, content, enabled, created_at, updated_at)
					VALUES ('inside-america-v2', '内部看美国·方法论画像（v2）', $1, true, now(), now())
					ON CONFLICT (name) DO NOTHING`, insideAmericaMethodologyProfile).Error; err != nil {
					return fmt.Errorf("seed reference role: %w", err)
				}
				return nil
			},
		},

		// ── board-level-deep-analysis revision: explicit result kinds ──
		// AutoMigrate owns the three pure ADD COLUMN operations. This migration
		// defensively repeats them for direct migration tests/upgrades, then owns
		// historical backfill, defaults, NOT NULL, CHECK/FK and cross-row parent
		// validation. It is forward-only; the migration runner has no Down path.
		{
			Version:     "20260828_0001",
			Description: "board-level-deep-analysis: classify result_kind and add immutable board brief/investigation parent linkage without rewriting sectors.",
			Up: func(db *gorm.DB) error {
				if !tableExists(db, "topic_enrichment_result") {
					return nil
				}
				if err := withLockTimeout(db, "5s", func(tx *gorm.DB) error {
					for _, statement := range []string{
						`ALTER TABLE topic_enrichment_result ADD COLUMN IF NOT EXISTS result_kind VARCHAR(32)`,
						`ALTER TABLE topic_enrichment_result ADD COLUMN IF NOT EXISTS parent_result_id BIGINT`,
						`ALTER TABLE topic_enrichment_result ADD COLUMN IF NOT EXISTS question_key VARCHAR(64)`,
					} {
						if err := tx.Exec(statement).Error; err != nil {
							return fmt.Errorf("add result-kind column: %w", err)
						}
					}
					return nil
				}); err != nil {
					return err
				}

				// Owner columns are an exclusive union. Refuse mixed historical rows
				// before classification so a migration can never hide data corruption by
				// clearing or reassigning an owner.
				var invalidOwnerRows int64
				if err := db.Raw(`SELECT count(*) FROM topic_enrichment_result
					WHERE CASE
						WHEN analysis_scope = 'topic' THEN persistent_topic_id IS NULL OR semantic_board_id IS NOT NULL
						WHEN analysis_scope = 'board' THEN semantic_board_id IS NULL OR persistent_topic_id IS NOT NULL
						ELSE true
					END`).Scan(&invalidOwnerRows).Error; err != nil {
					return fmt.Errorf("check topic_enrichment_result owner shape: %w", err)
				}
				if invalidOwnerRows > 0 {
					return fmt.Errorf("topic_enrichment_result has %d mixed or missing owner row(s); refusing result-kind migration", invalidOwnerRows)
				}

				// Every pre-existing board row is the old thesis/argument/depth report.
				// Only classifier columns change; sectors JSON is deliberately untouched.
				if err := db.Exec(`UPDATE topic_enrichment_result
					SET result_kind = CASE
						WHEN analysis_scope = 'board' THEN 'legacy_board_analysis'
						ELSE 'topic_analysis'
					END
					WHERE result_kind IS NULL OR result_kind = ''
					   OR (analysis_scope = 'board' AND result_kind = 'topic_analysis')`).Error; err != nil {
					return fmt.Errorf("backfill topic_enrichment_result.result_kind: %w", err)
				}
				var invalidInvestigationParents int64
				if err := db.Raw(`SELECT count(*)
					FROM topic_enrichment_result child
					LEFT JOIN topic_enrichment_result parent ON parent.id = child.parent_result_id
					WHERE child.result_kind = 'board_investigation'
					  AND (parent.id IS NULL
						OR parent.result_kind <> 'board_brief'
						OR parent.semantic_board_id IS DISTINCT FROM child.semantic_board_id)`).Scan(&invalidInvestigationParents).Error; err != nil {
					return fmt.Errorf("check board investigation parents: %w", err)
				}
				if invalidInvestigationParents > 0 {
					return fmt.Errorf("topic_enrichment_result has %d board investigation(s) without a same-board board_brief parent; refusing migration", invalidInvestigationParents)
				}

				if err := withLockTimeout(db, "5s", func(tx *gorm.DB) error {
					if err := tx.Exec(`ALTER TABLE topic_enrichment_result ALTER COLUMN result_kind SET DEFAULT 'topic_analysis'`).Error; err != nil {
						return fmt.Errorf("set topic_enrichment_result.result_kind default: %w", err)
					}
					if err := ensureNotNullDefault(tx, "topic_enrichment_result", "result_kind", "'topic_analysis'"); err != nil {
						return fmt.Errorf("topic_enrichment_result.result_kind NOT NULL: %w", err)
					}
					if err := tx.Exec(`DO $$ BEGIN
						IF NOT EXISTS (
							SELECT 1 FROM information_schema.table_constraints
							WHERE table_schema = 'public'
							  AND table_name = 'topic_enrichment_result'
							  AND constraint_name = 'chk_topic_enrichment_result_kind'
						) THEN
							ALTER TABLE topic_enrichment_result
								ADD CONSTRAINT chk_topic_enrichment_result_kind
								CHECK (result_kind IN ('topic_analysis', 'board_brief', 'board_investigation', 'legacy_board_analysis'));
						END IF;
					END $$`).Error; err != nil {
						return fmt.Errorf("add result_kind CHECK: %w", err)
					}
					if err := tx.Exec(`ALTER TABLE topic_enrichment_result
						DROP CONSTRAINT IF EXISTS chk_topic_enrichment_result_parent_shape`).Error; err != nil {
						return fmt.Errorf("drop stale result parent-shape CHECK: %w", err)
					}
					if err := tx.Exec(`ALTER TABLE topic_enrichment_result
						ADD CONSTRAINT chk_topic_enrichment_result_parent_shape CHECK (
							(result_kind = 'topic_analysis'
								AND analysis_scope = 'topic'
								AND persistent_topic_id IS NOT NULL AND semantic_board_id IS NULL
								AND parent_result_id IS NULL AND question_key IS NULL)
							OR (result_kind IN ('board_brief', 'legacy_board_analysis')
								AND analysis_scope = 'board'
								AND semantic_board_id IS NOT NULL AND persistent_topic_id IS NULL
								AND parent_result_id IS NULL AND question_key IS NULL)
							OR (result_kind = 'board_investigation'
								AND analysis_scope = 'board'
								AND semantic_board_id IS NOT NULL AND persistent_topic_id IS NULL
								AND parent_result_id IS NOT NULL
								AND question_key IS NOT NULL
								AND question_key ~ '^[0-9a-f]{64}$')
						)`).Error; err != nil {
						return fmt.Errorf("add result parent-shape CHECK: %w", err)
					}
					if err := tx.Exec(`DO $$ BEGIN
						IF NOT EXISTS (
							SELECT 1 FROM information_schema.table_constraints
							WHERE table_schema = 'public'
							  AND table_name = 'topic_enrichment_result'
							  AND constraint_name = 'uq_topic_enrichment_result_id_board'
						) THEN
							ALTER TABLE topic_enrichment_result
								ADD CONSTRAINT uq_topic_enrichment_result_id_board
								UNIQUE (id, semantic_board_id);
						END IF;
					END $$`).Error; err != nil {
						return fmt.Errorf("add result parent target unique constraint: %w", err)
					}
					if err := tx.Exec(`DO $$ BEGIN
						IF NOT EXISTS (
							SELECT 1 FROM information_schema.table_constraints
							WHERE table_schema = 'public'
							  AND table_name = 'topic_enrichment_result'
							  AND constraint_name = 'fk_topic_enrichment_result_parent_board'
						) THEN
							ALTER TABLE topic_enrichment_result
								ADD CONSTRAINT fk_topic_enrichment_result_parent_board
								FOREIGN KEY (parent_result_id, semantic_board_id)
								REFERENCES topic_enrichment_result(id, semantic_board_id)
								ON DELETE RESTRICT;
						END IF;
					END $$`).Error; err != nil {
						return fmt.Errorf("add same-board result parent FK: %w", err)
					}
					if err := tx.Exec(`CREATE OR REPLACE FUNCTION validate_topic_enrichment_result_parent()
						RETURNS trigger LANGUAGE plpgsql AS $$
						BEGIN
							IF NEW.result_kind = 'board_investigation' AND NOT EXISTS (
								SELECT 1 FROM topic_enrichment_result parent
								WHERE parent.id = NEW.parent_result_id
								  AND parent.result_kind = 'board_brief'
								  AND parent.analysis_scope = 'board'
								  AND parent.semantic_board_id = NEW.semantic_board_id
							) THEN
								RAISE EXCEPTION 'board_investigation parent must be a board_brief on the same board'
									USING ERRCODE = '23514';
							END IF;
							IF TG_OP = 'UPDATE' THEN
								IF OLD.result_kind = 'board_brief'
								   AND (NEW.result_kind IS DISTINCT FROM 'board_brief'
									OR NEW.semantic_board_id IS DISTINCT FROM OLD.semantic_board_id)
								   AND EXISTS (
									SELECT 1 FROM topic_enrichment_result child
									WHERE child.parent_result_id = OLD.id
									  AND child.result_kind = 'board_investigation'
								   ) THEN
									RAISE EXCEPTION 'cannot change a board_brief parent while investigations reference it'
										USING ERRCODE = '23514';
								END IF;
							END IF;
							RETURN NEW;
						END;
						$$`).Error; err != nil {
						return fmt.Errorf("create board investigation parent validation function: %w", err)
					}
					if err := tx.Exec(`DROP TRIGGER IF EXISTS trg_validate_topic_enrichment_result_parent ON topic_enrichment_result`).Error; err != nil {
						return fmt.Errorf("drop board investigation parent validation trigger: %w", err)
					}
					if err := tx.Exec(`CREATE TRIGGER trg_validate_topic_enrichment_result_parent
						BEFORE INSERT OR UPDATE OF result_kind, parent_result_id, semantic_board_id
						ON topic_enrichment_result
						FOR EACH ROW EXECUTE FUNCTION validate_topic_enrichment_result_parent()`).Error; err != nil {
						return fmt.Errorf("create board investigation parent validation trigger: %w", err)
					}
					return nil
				}); err != nil {
					return err
				}

				// The composite FK enforces parent existence/same-board identity; the
				// trigger adds the cross-row parent-kind invariant for direct SQL/GORM.
				if err := withLockTimeout(db, "5s", func(tx *gorm.DB) error {
					for _, statement := range []string{
						`CREATE INDEX IF NOT EXISTS idx_topic_enrichment_result_board_kind_id ON topic_enrichment_result (semantic_board_id, result_kind, id DESC)`,
						`CREATE INDEX IF NOT EXISTS idx_topic_enrichment_result_parent_question_id ON topic_enrichment_result (parent_result_id, question_key, id DESC) WHERE parent_result_id IS NOT NULL`,
					} {
						if err := tx.Exec(statement).Error; err != nil {
							return fmt.Errorf("create result-kind index: %w", err)
						}
					}
					return nil
				}); err != nil {
					return err
				}
				return nil
			},
		},
	}
	migrations = append(migrations, analysisMethodLegacyCopyMigration(), referenceRoleSeedRetireMigration())
	migrations = append(migrations, crossBoardRelationMigration())
	migrations = append(migrations, compositeComponentsMigration())
	migrations = append(migrations, watchMaterializedHintCleanupMigration())
	migrations = append(migrations, watchSuggestionCleanupMigration())
	migrations = append(migrations, legacyDiscoverNewPendingDismissMigration())
	migrations = append(migrations, dedupeRSSArticlesMigration())
	migrations = append(migrations, laneSnapshotFKMigration())
	migrations = append(migrations, healDanglingArticleRefsMigration())
	migrations = append(migrations, queueRetentionIndexMigration())
	migrations = append(migrations, completionOnRefreshOffMigration())
	migrations = append(migrations, normalizeArticleLinkFragmentsMigration())
	migrations = append(migrations, marginNotesTablesMigration())
	migrations = append(migrations, marginNotesWebSourcesMigration())
	migrations = append(migrations, marginNotesWebSearchStatusMigration())
	migrations = append(migrations, vendorModelMigration())
	migrations = append(migrations, keyEventMarkersMigration())
	migrations = append(migrations, pgTrgmMigration(), searchVectorDropMigration())
	migrations = append(migrations, entitySubjectMigration())
	migrations = append(migrations, boardSignalMigration(), boardSignalResearchProgressMigration(), dropProfileTablesMigration(), boardHealthSnapshotMigration(), signalDispositionMigration(), questionResearchBriefMigration(), searchSemanticHalfvecIndexMigration(), intraLaneBucketColumnsMigration(), auxiliaryDescriptionRepairMigration(), embeddingModelColumnWidthMigration(), embeddingModelNotNullMigration(), storyHeatTablesMigration())
	migrations = append(migrations, laneManualCorrectionPendingMigration(), laneManualCorrectionOverridesTableMigration(), laneManualCorrectionAssignmentTraceMigration(), laneManualCorrectionAssignmentOverridesTableMigration())
	migrations = append(migrations, dailyReportBackfillRunsMigration())
	return append(migrations, storySignalContentKeyMigration(), unifyVectorColumnDimensionMigration(), ensureTopicTagHalfvecIndexMigration(), tagJobUniqueMigration())
}

// intraLaneBucketColumnsMigration implements 20261024_0001
// (add-intra-lane-theme-bucketing D6): add the two bucket columns to
// daily_report_sections. AutoMigrate also creates them from the model, but the
// explicit migration pins the DB defaults (historical rows keep
// bucket_index=0 / bucket_title NULL — no backfill) and documents the
// "same cluster_index may repeat" contract. Idempotent (IF NOT EXISTS) and
// skips when the table is absent (domain models not registered).
func intraLaneBucketColumnsMigration() Migration {
	return Migration{
		Version:     "20261024_0001",
		Description: "intra-lane-theme-bucketing: add daily_report_sections.bucket_title (nullable) / bucket_index (NOT NULL default 0); sibling bucket sections share cluster_index, no unique constraint added",
		Up: func(db *gorm.DB) error {
			if !tableExists(db, "daily_report_sections") {
				return nil
			}
			if err := db.Exec(`ALTER TABLE daily_report_sections ADD COLUMN IF NOT EXISTS bucket_title VARCHAR(200)`).Error; err != nil {
				return fmt.Errorf("add daily_report_sections.bucket_title: %w", err)
			}
			if err := db.Exec(`ALTER TABLE daily_report_sections ADD COLUMN IF NOT EXISTS bucket_index INTEGER NOT NULL DEFAULT 0`).Error; err != nil {
				return fmt.Errorf("add daily_report_sections.bucket_index: %w", err)
			}
			return nil
		},
	}
}

// boardHealthSnapshotMigration implements 20261006_0001
// (add-board-health-checkup design D2): add the FK
// board_health_snapshots.semantic_board_id → semantic_labels(id) ON DELETE
// CASCADE. AutoMigrate creates the table + composite index but runs with
// DisableForeignKeyConstraintWhenMigrating, so the FK lives here (same policy
// as fk_topic_lane_snapshots_topic / 20260910_0001). Hard-deleting a board
// removes its snapshots — they are pure log-style data, orphan rows are
// meaningless. Idempotent (IF NOT EXISTS); skips when the table is absent.
func boardHealthSnapshotMigration() Migration {
	return Migration{
		Version:     "20261006_0001",
		Description: "board-health-checkup: FK board_health_snapshots.semantic_board_id -> semantic_labels(id) ON DELETE CASCADE (AutoMigrate owns the table; migration owns the FK)",
		Up: func(db *gorm.DB) error {
			var tableExists bool
			if err := db.Raw(`SELECT EXISTS (
				SELECT 1 FROM information_schema.tables WHERE table_name = 'board_health_snapshots')`).Scan(&tableExists).Error; err != nil {
				return fmt.Errorf("check board_health_snapshots: %w", err)
			}
			if !tableExists {
				return nil
			}
			// 约束已存在时不得靠「报错后再查」来兼底：PG 里任何一条语句报错都会中止
			// 当前事务（后续语句只会拿到 25P02），迁移闭包在事务内跑，事后补救无效。
			// 正确做法是**先查后加**（幂等且不产生事务中止）。
			// 并发首跑（AutoMigrate + 迁移竞争同一进程）仍可能撞名——那种情况让迁移
			// 失败重试即可（下一次启动先查到约束存在 → 直接跳过）。
			var constraintExists bool
			if err := db.Raw(`SELECT EXISTS (
				SELECT 1 FROM information_schema.table_constraints
				WHERE constraint_name = 'fk_board_health_snapshots_board'
				  AND table_name = 'board_health_snapshots')`).Scan(&constraintExists).Error; err != nil {
				return fmt.Errorf("check fk_board_health_snapshots_board: %w", err)
			}
			if constraintExists {
				return nil
			}
			if err := db.Exec(`ALTER TABLE board_health_snapshots
				ADD CONSTRAINT fk_board_health_snapshots_board
				FOREIGN KEY (semantic_board_id) REFERENCES semantic_labels(id)
				ON DELETE CASCADE`).Error; err != nil {
				return fmt.Errorf("add fk_board_health_snapshots_board: %w", err)
			}
			return nil
		},
	}
}

// dropProfileTablesMigration implements 20260926_0001
// (restructure-settings-navigation): drop the two retired profile tables.
// 事实依据：reference_roles 仅 1 行 seed 演示数据（UI 入口 2026-09-04 已摘）；
// analysis_methods 仅 1 条 legacy 停用卡（从未启用，注入零次发生）——两表
// 无用户数据价值，drop 无行为损失。幂等：IF EXISTS。回滚 = 按下方快照重建
// 空表（无数据恢复诉求）。
//
// 表结构快照（供考古/重建，2026-09-26 取自真库）：
//
//	reference_roles:
//	  id bigint PK (seq), name varchar(120) UNIQUE NOT NULL,
//	  title varchar(200), content text NOT NULL, enabled boolean NOT NULL,
//	  created_at/updated_at timestamptz
//
//	analysis_methods:
//	  id bigint PK (seq), name varchar(120) UNIQUE NOT NULL,
//	  title varchar(200), summary text,
//	  selection_meta jsonb NOT NULL DEFAULT '{}', content text NOT NULL,
//	  enabled boolean NOT NULL, legacy boolean NOT NULL DEFAULT false,
//	  deleted_at timestamptz (indexed), created_at/updated_at timestamptz
func dropProfileTablesMigration() Migration {
	return Migration{
		Version:     "20260926_0001",
		Description: "restructure-settings-navigation: drop retired profile tables reference_roles and analysis_methods (zero usage, no user data).",
		Up: func(db *gorm.DB) error {
			if err := db.Exec(`DROP TABLE IF EXISTS reference_roles`).Error; err != nil {
				return fmt.Errorf("drop reference_roles: %w", err)
			}
			if err := db.Exec(`DROP TABLE IF EXISTS analysis_methods`).Error; err != nil {
				return fmt.Errorf("drop analysis_methods: %w", err)
			}
			return nil
		},
	}
}

// boardSignalResearchProgressMigration implements 20260922_0002
// (board-signal-reports tasks 4.7「断了不能白跑」):
//
// board_signal_research_progress is created by AutoMigrate (model registered
// in internal/dataenrichment). This migration adds what AutoMigrate cannot
// express:
//
// ① the status CHECK (running|abandoned|superseded),
// ② the composite FK pinning owner/period to the candidate (candidate_id,
//
//	semantic_board_id, granularity, period → board_signal_candidate same
//	columns; a progress row can never disagree with its candidate),
//
// ③ the (candidate_id, updated_at DESC) lookup index and the job_id unique
//
//	(repeated defensively; AutoMigrate owns both on fresh DBs).
//
// Forward-only; idempotent via DROP CONSTRAINT IF EXISTS + ADD and
// CREATE INDEX IF NOT EXISTS.
func boardSignalResearchProgressMigration() Migration {
	return Migration{
		Version:     "20260922_0002",
		Description: "board-signal-reports: research progress table constraints (status CHECK, candidate composite FK, candidate-updated_at index, job_id unique) — per-round rolling progress survives job timeout/failure.",
		Up: func(db *gorm.DB) error {
			if !tableExists(db, "board_signal_research_progress") {
				return nil
			}
			if err := withLockTimeout(db, "5s", func(tx *gorm.DB) error {
				statements := []string{
					`ALTER TABLE board_signal_research_progress
						DROP CONSTRAINT IF EXISTS chk_board_signal_research_progress_status`,
					`ALTER TABLE board_signal_research_progress
						ADD CONSTRAINT chk_board_signal_research_progress_status
						CHECK (status IN ('running', 'abandoned', 'superseded'))`,
				}
				if tableExists(tx, "board_signal_candidate") {
					statements = append(statements,
						`ALTER TABLE board_signal_research_progress
							DROP CONSTRAINT IF EXISTS fk_board_signal_research_progress_candidate`,
						`ALTER TABLE board_signal_research_progress
							ADD CONSTRAINT fk_board_signal_research_progress_candidate
							FOREIGN KEY (candidate_id, semantic_board_id, granularity, period)
							REFERENCES board_signal_candidate (id, semantic_board_id, granularity, period)
							ON DELETE RESTRICT`)
				}
				for _, stmt := range statements {
					if err := tx.Exec(stmt).Error; err != nil {
						return fmt.Errorf("board signal research progress constraint: %w", err)
					}
				}
				return nil
			}); err != nil {
				return err
			}

			return withLockTimeout(db, "5s", func(tx *gorm.DB) error {
				for _, statement := range []string{
					`CREATE INDEX IF NOT EXISTS idx_board_signal_research_progress_candidate ON board_signal_research_progress (candidate_id, updated_at DESC)`,
					`CREATE UNIQUE INDEX IF NOT EXISTS uq_board_signal_research_progress_job ON board_signal_research_progress (job_id)`,
				} {
					if err := tx.Exec(statement).Error; err != nil {
						return fmt.Errorf("create board signal research progress index: %w", err)
					}
				}
				return nil
			})
		},
	}
}

// boardSignalMigration implements 20260922_0001 (board-signal-reports phase 1):
//
// ① topic_enrichment_result gains kind='signal_report' plus three nullable
//
//	columns (granularity/period/source_signal_id). The two named CHECKs are
//	DROPped and re-ADDed with the new branch (PostgreSQL CHECK has no
//	incremental alter; 20260828_0001 sample). Existing rows never get the
//	new columns backfilled — every other kind keeps them NULL.
//
// ② board_signal_discovery / board_signal_candidate (created by AutoMigrate;
//
//	the migration defensively repeats the ADD COLUMNs only) get their
//	composite owner uniqueness and the composite FKs that make a candidate's
//	board/granularity/period immutable relative to its batch, and a
//	signal_report's owner/period immutable relative to its candidate.
//
// ③ board-period-id list indexes and the source_signal_id index.
//
// Forward-only; idempotent via DROP CONSTRAINT IF EXISTS + ADD and
// CREATE INDEX IF NOT EXISTS.
func boardSignalMigration() Migration {
	return Migration{
		Version:     "20260922_0001",
		Description: "board-signal-reports: result kind=signal_report with nullable granularity/period/source_signal_id; discovery/candidate tables with composite owner FKs and board-period-id indexes.",
		Up: func(db *gorm.DB) error {
			// Dependent FKs must drop BEFORE the uniques they reference
			// (PostgreSQL refuses dropping a depended-on constraint); the
			// result→candidate FK is dropped first so the new-table block can
			// safely rebuild the candidate unique on idempotent re-runs.
			if tableExists(db, "topic_enrichment_result") && tableExists(db, "board_signal_discovery") && tableExists(db, "board_signal_candidate") {
				if err := withLockTimeout(db, "5s", func(tx *gorm.DB) error {
					if err := tx.Exec(`ALTER TABLE topic_enrichment_result
						DROP CONSTRAINT IF EXISTS fk_topic_enrichment_result_signal_candidate`).Error; err != nil {
						return fmt.Errorf("drop stale signal candidate FK: %w", err)
					}
					return nil
				}); err != nil {
					return err
				}
			}

			// ── new-table constraints (tables exist after AutoMigrate; skip
			// topicgraph-less or not-yet-migrated deployments defensively).
			if tableExists(db, "board_signal_discovery") && tableExists(db, "board_signal_candidate") {
				if err := withLockTimeout(db, "5s", func(tx *gorm.DB) error {
					statements := []string{
						`ALTER TABLE board_signal_candidate
							DROP CONSTRAINT IF EXISTS fk_board_signal_candidate_discovery`,
						// Composite owner identity — FK targets.
						`ALTER TABLE board_signal_discovery
							DROP CONSTRAINT IF EXISTS uq_board_signal_discovery_id_owner`,
						`ALTER TABLE board_signal_discovery
							ADD CONSTRAINT uq_board_signal_discovery_id_owner
							UNIQUE (id, semantic_board_id, granularity, period)`,
						// 复合唯一约束**不能**先 DROP 再 ADD：它被
						// fk_board_signal_research_progress_candidate（另一个迁移建的
						// 复合外键）引用，PG 会拒绝 DROP（2BP01 dependent objects）。
						// 改为 DO 块守卫：不存在才添加，已存在则原样保留（约束内容已冻结）。
						`DO $$
						BEGIN
							IF NOT EXISTS (SELECT 1 FROM pg_constraint
								WHERE conname = 'uq_board_signal_candidate_id_owner'
								  AND conrelid = 'public.board_signal_candidate'::regclass) THEN
								ALTER TABLE board_signal_candidate
									ADD CONSTRAINT uq_board_signal_candidate_id_owner
									UNIQUE (id, semantic_board_id, granularity, period);
							END IF;
						END $$`,
						// A candidate can never disagree with its batch's owner/period.
						`ALTER TABLE board_signal_candidate
							ADD CONSTRAINT fk_board_signal_candidate_discovery
							FOREIGN KEY (discovery_id, semantic_board_id, granularity, period)
							REFERENCES board_signal_discovery (id, semantic_board_id, granularity, period)
							ON DELETE RESTRICT`,
					}
					for _, stmt := range statements {
						if err := tx.Exec(stmt).Error; err != nil {
							return fmt.Errorf("board signal discovery/candidate constraint: %w", err)
						}
					}
					return nil
				}); err != nil {
					return err
				}
			}

			if !tableExists(db, "topic_enrichment_result") {
				return nil
			}

			// Pure ADD COLUMN (AutoMigrate owns them; repeated defensively so the
			// migration also works on a direct testcontainer).
			if err := withLockTimeout(db, "5s", func(tx *gorm.DB) error {
				for _, statement := range []string{
					`ALTER TABLE topic_enrichment_result ADD COLUMN IF NOT EXISTS granularity VARCHAR(10)`,
					`ALTER TABLE topic_enrichment_result ADD COLUMN IF NOT EXISTS period VARCHAR(12)`,
					`ALTER TABLE topic_enrichment_result ADD COLUMN IF NOT EXISTS source_signal_id BIGINT`,
				} {
					if err := tx.Exec(statement).Error; err != nil {
						return fmt.Errorf("add signal-report column: %w", err)
					}
				}
				return nil
			}); err != nil {
				return err
			}

			// No backfill by design: pre-signal rows keep the three columns NULL
			// (旧行不回填，不猜测历史周期归属). Defensively refuse rows that would
			// violate the new shape before the constraints re-ADD hides them.
			var invalidSignalRows int64
			if err := db.Raw(`SELECT count(*) FROM topic_enrichment_result
				WHERE result_kind <> 'signal_report'
				  AND (granularity IS NOT NULL OR period IS NOT NULL OR source_signal_id IS NOT NULL)`).Scan(&invalidSignalRows).Error; err != nil {
				return fmt.Errorf("check topic_enrichment_result signal columns: %w", err)
			}
			if invalidSignalRows > 0 {
				return fmt.Errorf("topic_enrichment_result has %d non-signal row(s) with granularity/period/source_signal_id set; refusing migration", invalidSignalRows)
			}

			if err := withLockTimeout(db, "5s", func(tx *gorm.DB) error {
				// Kind enum: re-ADD with signal_report included.
				if err := tx.Exec(`ALTER TABLE topic_enrichment_result
					DROP CONSTRAINT IF EXISTS chk_topic_enrichment_result_kind`).Error; err != nil {
					return fmt.Errorf("drop stale result kind CHECK: %w", err)
				}
				if err := tx.Exec(`ALTER TABLE topic_enrichment_result
					ADD CONSTRAINT chk_topic_enrichment_result_kind
					CHECK (result_kind IN ('topic_analysis', 'board_brief', 'board_investigation', 'legacy_board_analysis', 'signal_report'))`).Error; err != nil {
					return fmt.Errorf("add result kind CHECK: %w", err)
				}
				// Owner/period shape: re-ADD with the signal_report branch.
				if err := tx.Exec(`ALTER TABLE topic_enrichment_result
					DROP CONSTRAINT IF EXISTS chk_topic_enrichment_result_parent_shape`).Error; err != nil {
					return fmt.Errorf("drop stale result parent-shape CHECK: %w", err)
				}
				if err := tx.Exec(`ALTER TABLE topic_enrichment_result
					ADD CONSTRAINT chk_topic_enrichment_result_parent_shape CHECK (
						(result_kind = 'topic_analysis'
							AND analysis_scope = 'topic'
							AND persistent_topic_id IS NOT NULL AND semantic_board_id IS NULL
							AND parent_result_id IS NULL AND question_key IS NULL)
						OR (result_kind IN ('board_brief', 'legacy_board_analysis')
							AND analysis_scope = 'board'
							AND semantic_board_id IS NOT NULL AND persistent_topic_id IS NULL
							AND parent_result_id IS NULL AND question_key IS NULL
							AND granularity IS NULL AND period IS NULL AND source_signal_id IS NULL)
						OR (result_kind = 'board_investigation'
							AND analysis_scope = 'board'
							AND semantic_board_id IS NOT NULL AND persistent_topic_id IS NULL
							AND parent_result_id IS NOT NULL
							AND question_key IS NOT NULL
							AND question_key ~ '^[0-9a-f]{64}$'
							AND granularity IS NULL AND period IS NULL AND source_signal_id IS NULL)
						OR (result_kind = 'signal_report'
							AND analysis_scope = 'board'
							AND semantic_board_id IS NOT NULL AND persistent_topic_id IS NULL
							AND parent_result_id IS NULL AND question_key IS NULL
							-- NULL-aware guards: a CHECK is satisfied by an UNKNOWN
							-- (NULL) predicate in PostgreSQL, so NULL granularity/period
							-- must be rejected explicitly, not left to IN/regex NULL
							-- propagation.
							AND granularity IS NOT NULL AND period IS NOT NULL
							AND granularity IN ('month', 'year')
							AND ((granularity = 'month' AND period ~ '^[0-9]{4}-(0[1-9]|1[0-2])$')
								OR (granularity = 'year' AND period ~ '^[0-9]{4}$' AND period BETWEEN '2000' AND '2100'))
							AND source_signal_id IS NOT NULL)
					)`).Error; err != nil {
					return fmt.Errorf("add result parent-shape CHECK: %w", err)
				}
				// A signal_report's owner/period can never disagree with its
				// candidate (NULL source_signal_id rows simply don't match —
				// MATCH SIMPLE; non-signal rows are outside the FK entirely).
				if tableExists(tx, "board_signal_candidate") {
					if err := tx.Exec(`ALTER TABLE topic_enrichment_result
						DROP CONSTRAINT IF EXISTS fk_topic_enrichment_result_signal_candidate`).Error; err != nil {
						return fmt.Errorf("drop stale signal candidate FK: %w", err)
					}
					if err := tx.Exec(`ALTER TABLE topic_enrichment_result
						ADD CONSTRAINT fk_topic_enrichment_result_signal_candidate
						FOREIGN KEY (source_signal_id, semantic_board_id, granularity, period)
						REFERENCES board_signal_candidate (id, semantic_board_id, granularity, period)
						ON DELETE RESTRICT`).Error; err != nil {
						return fmt.Errorf("add signal candidate FK: %w", err)
					}
				}
				return nil
			}); err != nil {
				return err
			}

			if err := withLockTimeout(db, "5s", func(tx *gorm.DB) error {
				for _, statement := range []string{
					`CREATE INDEX IF NOT EXISTS idx_board_signal_discovery_board_period ON board_signal_discovery (semantic_board_id, granularity, period, id DESC)`,
					`CREATE INDEX IF NOT EXISTS idx_board_signal_candidate_board_period ON board_signal_candidate (semantic_board_id, granularity, period, id DESC)`,
					`CREATE INDEX IF NOT EXISTS idx_board_signal_candidate_discovery ON board_signal_candidate (discovery_id, id DESC)`,
					`CREATE INDEX IF NOT EXISTS idx_topic_enrichment_result_signal_board_period ON topic_enrichment_result (semantic_board_id, granularity, period, id DESC) WHERE result_kind = 'signal_report'`,
					`CREATE INDEX IF NOT EXISTS idx_topic_enrichment_result_source_signal ON topic_enrichment_result (source_signal_id, id DESC) WHERE source_signal_id IS NOT NULL`,
				} {
					if err := tx.Exec(statement).Error; err != nil {
						return fmt.Errorf("create board signal index: %w", err)
					}
				}
				return nil
			}); err != nil {
				return err
			}
			return nil
		},
	}
}

// normalizeArticleLinkFragmentsMigration implements 20260920_0001 (v2ex link
// fragment dedupe fix): strip the URL fragment from articles.link, then merge
// the duplicates this surfaces. V2EX appends a drifting #replyN anchor (reply
// count at feed-generation time) to every entry link, so the same topic changed
// link between refreshes and slipped past the (feed_id, link) dedupe — 93
// groups / 282 surplus rows across the two v2ex tab feeds alone, every copy
// crawling firecrawl separately. The parser now normalizes entry links at
// ingestion (textutil.StripURLFragment); this one-shot repairs the stored rows.
// Fragments never reach the server, so stripping loses nothing; `#!` hashbang
// fragments (SPA route identity) are preserved.
//
// Irreversible: the deleted copies are not recoverable. Groups reuse the
// 20260917_0001 merge contract (tags, reading behaviors, queued jobs and
// daily-report refs rewire to the keeper, tag_count recomputed). Groups are
// keyed by stripped link in Go (dialect-neutral) and disjoint by construction,
// so the per-group normalization UPDATEs cannot collide with the
// uq_articles_feed_link unique index. Re-running is a no-op.
func normalizeArticleLinkFragmentsMigration() Migration {
	return Migration{
		Version:     "20260920_0001",
		Description: "strip drifting #fragment anchors from articles.link (V2EX #replyN) and merge the duplicates this surfaces (20260917_0001 merge contract; #! hashbang preserved).",
		Up: func(db *gorm.DB) error {
			if !tableExists(db, "articles") {
				logging.Infof("normalize-article-link-fragments: articles absent; nothing to repair")
				return nil
			}

			var rows []struct {
				ID     uint
				FeedID uint
				Link   string
			}
			if err := db.Raw(`
				SELECT id, feed_id, link FROM articles
				WHERE link <> '' AND link LIKE '%#%'`).Scan(&rows).Error; err != nil {
				return fmt.Errorf("load fragment-carrying article links: %w", err)
			}

			type groupKey struct {
				feedID uint
				base   string
			}
			members := make(map[groupKey][]uint)
			for _, r := range rows {
				key := groupKey{r.FeedID, textutil.StripURLFragment(r.Link)}
				members[key] = append(members[key], r.ID)
			}
			// Fold in the exact stripped-link row when one exists (a topic stored
			// once without a fragment): it is the natural keeper candidate.
			for key := range members {
				var exact []uint
				if err := db.Raw(`SELECT id FROM articles WHERE feed_id = ? AND link = ?`,
					key.feedID, key.base).Scan(&exact).Error; err != nil {
					return fmt.Errorf("probe exact-link row (feed=%d link=%s): %w", key.feedID, key.base, err)
				}
				members[key] = append(members[key], exact...)
			}

			var merged, normalized int
			for key, ids := range members {
				if len(ids) > 1 {
					if err := mergeDuplicateArticleRows(db, key.feedID, "a.id IN ?", ids); err != nil {
						return fmt.Errorf("merge fragment group (feed=%d link=%s): %w", key.feedID, key.base, err)
					}
					merged++
				}
				// After the merge exactly one row per group survives; stamping the
				// stripped link cannot collide (each base appears once per feed).
				if err := db.Model(&models.Article{}).
					Where("id IN ?", ids).
					Update("link", key.base).Error; err != nil {
					return fmt.Errorf("normalize link to %s (feed=%d): %w", key.base, key.feedID, err)
				}
				normalized++
			}
			logging.Infof("normalize-article-link-fragments: fragment_rows=%d groups=%d merged=%d links_normalized=%d",
				len(rows), len(members), merged, normalized)
			return nil
		},
	}
}

// queueRetentionIndexMigration implements 20260917_0003
// (add-notification-center, log-cleanup delta): partial indexes backing the
// extended queue-row retention DELETEs in job_log_cleanup — completed rows
// older than 1 day, failed rows older than 30 days — across tag_jobs,
// firecrawl_jobs and embedding_queues. Same partial-index precedent as
// idx_embedding_queues_completed_created (20260820_0002). Idempotent.
func queueRetentionIndexMigration() Migration {
	type queueIdx struct {
		table string
		name  string
		where string
	}
	indexes := []queueIdx{
		{"tag_jobs", "idx_tag_jobs_completed_created", "status = 'completed'"},
		{"tag_jobs", "idx_tag_jobs_failed_created", "status = 'failed'"},
		{"firecrawl_jobs", "idx_firecrawl_jobs_completed_created", "status = 'completed'"},
		{"firecrawl_jobs", "idx_firecrawl_jobs_failed_created", "status = 'failed'"},
		{"embedding_queues", "idx_embedding_queues_failed_created", "status = 'failed'"},
	}
	return Migration{
		Version:     "20260917_0003",
		Description: "Add partial indexes on created_at WHERE status IN ('completed','failed') for tag_jobs/firecrawl_jobs/embedding_queues to back the extended log_cleanup queue-row retention DELETEs (completed 1d, failed 30d). Idempotent.",
		Up: func(db *gorm.DB) error {
			for _, idx := range indexes {
				if !tableExists(db, idx.table) {
					continue
				}
				if err := db.Exec(fmt.Sprintf(
					`CREATE INDEX IF NOT EXISTS %s ON %s (created_at) WHERE %s`,
					idx.name, idx.table, idx.where)).Error; err != nil {
					return fmt.Errorf("create %s: %w", idx.name, err)
				}
			}
			return nil
		},
	}
}

// healDanglingArticleRefsMigration implements 20260917_0002
// (heal-dangling-article-refs D5): one-shot repair of the article references that
// pre-fix delete paths orphaned inside daily_report_threads.related_article_ids —
// the loser copies removed by 20260917_0001 carried references nobody rewired,
// which made daily-report 线索 resolve to "文章 #<id>" instead of a source.
//
// Two steps, in this order: normalize non-array values (JSON null scalar and SQL
// NULL) to [] so the sweep can read every row, then prune references without a
// live articles row. Irreversible data repair — no Down; generated-at-the-time
// snapshot counters (board_daily_reports.article_count and friends) are
// deliberately NOT recomputed (they are report-time snapshots, not live counts).
// Re-running is a no-op.
func healDanglingArticleRefsMigration() Migration {
	return Migration{
		Version:     "20260917_0002",
		Description: "heal-dangling-article-refs: normalize daily_report_threads.related_article_ids (JSON null → []) and prune references to deleted articles (irreversible data repair; report snapshot counters untouched).",
		Up: func(db *gorm.DB) error {
			// daily_report_threads is AutoMigrated from the daily-report models, not
			// created by a versioned migration, so a binary that never registers
			// those models (CLI tools, narrow test packages) has no table to repair.
			// Same guard the thread-index migration uses.
			if !tableExists(db, "daily_report_threads") {
				logging.Infof("heal-dangling-article-refs: daily_report_threads absent; nothing to repair")
				return nil
			}
			normalized, err := articlerefs.NormalizeThreadRefs(db)
			if err != nil {
				return fmt.Errorf("normalize thread article refs: %w", err)
			}
			rows, refs, err := articlerefs.PruneDanglingRefs(db, articlerefs.DefaultBatchSize)
			if err != nil {
				return fmt.Errorf("prune dangling article refs: %w", err)
			}
			logging.Infof("heal-dangling-article-refs: normalized=%d rows_repaired=%d refs_removed=%d",
				normalized, rows, refs)
			return nil
		},
	}
}

// dedupeRSSArticlesMigration implements 20260917_0001 (dedupe-rss-articles D4):
// one-shot merge of articles duplicated inside one feed (same feed_id + link).
// Historically the refresh path deduped on (feed_id, title) only, so live-news
// feeds that roll the title under a stable URL and concurrent refreshes left
// 2..5 rows per article; every copy was tagged separately (duplicate AI calls
// and duplicate tagging records).
//
// Per group the most complete row is kept — active before archived (an
// archived keeper would silently drop the article out of the live window),
// then tagged / crawled / summarised, then engagement — and ties go to the
// lowest id (earliest). Tag links move to the keeper (conflicts are dropped,
// the (article_id, topic_tag_id) unique index survives either way), reading
// behaviours are re-pointed, queued jobs are re-pointed or dropped, copies are
// deleted, and the keeper's denormalised tag_count is recomputed. The unique
// partial index on (feed_id, link) is created in the same transaction so
// "merge succeeded" and "constraint enforced" stay atomic.
//
// Irreversible: the deleted copies are not recoverable. Rows with an empty link
// are never touched (they cannot be deduped) and are excluded from the unique
// index.
func dedupeRSSArticlesMigration() Migration {
	return Migration{
		Version:     "20260917_0001",
		Description: "dedupe-rss-articles: merge articles duplicated per (feed_id, link), rewire tags/behaviors/jobs, add unique (feed_id, link) index.",
		Up: func(db *gorm.DB) error {
			// Plain link index: serves the merge below and the runtime cross-feed
			// tag-reuse lookup (same name as the GORM model tag).
			if err := db.Exec("CREATE INDEX IF NOT EXISTS idx_articles_link ON articles(link)").Error; err != nil {
				return fmt.Errorf("create articles.link index: %w", err)
			}

			var groups []struct {
				FeedID uint
				Link   string
			}
			if err := db.Raw(`
				SELECT feed_id, link FROM articles
				WHERE link <> ''
				GROUP BY feed_id, link
				HAVING COUNT(*) > 1`).Scan(&groups).Error; err != nil {
				return fmt.Errorf("find duplicate article groups: %w", err)
			}
			for _, g := range groups {
				if err := mergeDuplicateArticleGroup(db, g.FeedID, g.Link); err != nil {
					return err
				}
			}
			if len(groups) > 0 {
				logging.Infof("dedupe-rss-articles: merged %d duplicate article groups", len(groups))
			}

			if err := db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS uq_articles_feed_link
				ON articles(feed_id, link) WHERE link <> ''`).Error; err != nil {
				return fmt.Errorf("create unique (feed_id, link) index: %w", err)
			}
			return nil
		},
	}
}

// mergeDuplicateArticleGroup collapses one (feed_id, link) group to a single
// surviving article row. No-op for groups that shrank below 2 rows.
func mergeDuplicateArticleGroup(db *gorm.DB, feedID uint, link string) error {
	return mergeDuplicateArticleRows(db, feedID, "a.link = ?", link)
}

// mergeDuplicateArticleRows collapses one group of same-feed article rows —
// selected by whereLink (a condition over the `a` alias, variadic-arg bound
// via arg) — down to a single surviving row. The 20260917_0001 merge selects
// with "a.link = ?"; the 20260920_0001 fragment migration selects by id list
// ("a.id IN ?") because its rows differ only in the drifting #fragment.
func mergeDuplicateArticleRows(db *gorm.DB, feedID uint, whereLink string, arg any) error {
	var rows []struct {
		ID               uint
		Archived         bool
		Favorite         bool
		Read             bool
		FirecrawlContent string
		AIContentSummary string
		HasTags          bool
		HasBehavior      bool
	}
	// has_tags / has_behavior come from EXISTS rather than the denormalised
	// tag_count column so a stale counter can never steer the keeper choice.
	if err := db.Raw(`
		SELECT a.id, a.archived, a.favorite, a."read",
		       COALESCE(a.firecrawl_content, '') AS firecrawl_content,
		       COALESCE(a.ai_content_summary, '') AS ai_content_summary,
		       EXISTS(SELECT 1 FROM article_topic_tags t WHERE t.article_id = a.id) AS has_tags,
		       EXISTS(SELECT 1 FROM reading_behaviors b WHERE b.article_id = a.id) AS has_behavior
		FROM articles a
		WHERE a.feed_id = ? AND `+whereLink+`
		ORDER BY a.id`, feedID, arg).Scan(&rows).Error; err != nil {
		return fmt.Errorf("load duplicate group (feed=%d): %w", feedID, err)
	}
	if len(rows) < 2 {
		return nil
	}

	score := func(i int) int {
		r := rows[i]
		s := 0
		if !r.Archived {
			s += 1000
		}
		if r.HasTags {
			s += 100
		}
		if r.FirecrawlContent != "" {
			s += 10
		}
		if r.AIContentSummary != "" {
			s += 10
		}
		if r.Favorite || r.Read || r.HasBehavior {
			s++
		}
		return s
	}

	best := 0
	for i := 1; i < len(rows); i++ {
		if score(i) > score(best) {
			best = i
		}
	}
	keeper := rows[best].ID

	for _, r := range rows {
		if r.ID == keeper {
			continue
		}
		// Move the copy's tag links the keeper does not already carry; the
		// remaining (conflicting) rows are dropped below so the unique index
		// on (article_id, topic_tag_id) is never violated.
		if err := db.Exec(`
			UPDATE article_topic_tags t SET article_id = ?
			WHERE t.article_id = ?
			  AND NOT EXISTS (
				SELECT 1 FROM article_topic_tags k
				WHERE k.article_id = ? AND k.topic_tag_id = t.topic_tag_id)`,
			keeper, r.ID, keeper).Error; err != nil {
			return fmt.Errorf("rewire tag links from article %d to %d: %w", r.ID, keeper, err)
		}
		if err := db.Exec(`DELETE FROM article_topic_tags WHERE article_id = ?`, r.ID).Error; err != nil {
			return fmt.Errorf("drop conflicting tag links of article %d: %w", r.ID, err)
		}

		// Reading behaviours are real events: re-point them (and their
		// denormalised feed id) so the surviving article keeps the history.
		if err := db.Exec(`UPDATE reading_behaviors SET article_id = ?, feed_id = ? WHERE article_id = ?`,
			keeper, feedID, r.ID).Error; err != nil {
			return fmt.Errorf("re-point reading behaviors of article %d: %w", r.ID, err)
		}

		// Queued work for a row about to disappear is dropped when finished
		// and re-pointed when still pending, so no job points at a dead id.
		for _, job := range []struct{ table string }{
			{"tag_jobs"}, {"firecrawl_jobs"},
		} {
			if !tableExists(db, job.table) {
				continue
			}
			if err := db.Exec("DELETE FROM "+job.table+" WHERE article_id = ? AND status IN ('completed','failed')", r.ID).Error; err != nil {
				return fmt.Errorf("drop finished %s rows of article %d: %w", job.table, r.ID, err)
			}
			if err := db.Exec("UPDATE "+job.table+" SET article_id = ? WHERE article_id = ?", keeper, r.ID).Error; err != nil {
				return fmt.Errorf("re-point %s rows of article %d: %w", job.table, r.ID, err)
			}
		}

		// Daily-report threads reference articles by ID. Point the doomed copy's
		// references at the keeper before the row disappears, otherwise every
		// 线索 that cited it becomes a dead link (heal-dangling-article-refs D2).
		// A failure aborts the whole merge: a dangling reference costs more than
		// a rolled-back group.
		if _, err := articlerefs.RewireArticleRefs(db, r.ID, keeper); err != nil {
			return fmt.Errorf("rewire article refs of article %d to %d: %w", r.ID, keeper, err)
		}

		if err := db.Exec(`DELETE FROM articles WHERE id = ?`, r.ID).Error; err != nil {
			return fmt.Errorf("delete duplicate article %d: %w", r.ID, err)
		}
	}

	// The survivor's counter must match the edges it actually holds.
	if err := db.Exec(`UPDATE articles SET tag_count =
		(SELECT COUNT(*) FROM article_topic_tags WHERE article_id = ?) WHERE id = ?`,
		keeper, keeper).Error; err != nil {
		return fmt.Errorf("recompute tag_count for article %d: %w", keeper, err)
	}
	return nil
}

// watchMaterializedHintCleanupMigration implements 20260905_0001: one-shot
// removal of topic_watch_hits rows owned by materialized-track watches
// (type=keyword_topic / sentence_topic). These rows violate the topic-watch
// spec (物化轨 SHALL NOT 产生命中提示记录) — they were written because the
// pre-fix hint evaluation fell back every non-keyword type into the label AI
// branch. Deletion is safe: hits are a read-only overlay with no downstream
// dependency, and label/keyword-track hits are untouched.

// compositeComponentsMigration implements 20260902_0001: composite label
// support (add-composite-labels). AutoMigrate owns composite_components table
// creation from the model (PK + FK CASCADE tags); this migration owns the
// idempotent belt-and-braces FK ensure for deployments where AutoMigrate tags
// did not apply, plus seeding the three ai_settings knobs the feature reads.
func compositeComponentsMigration() Migration {
	return Migration{
		Version:     "20260902_0001",
		Description: "add-composite-labels: composite_components FK cascade ensure + seed composite_label_dedupe_sim / semantic_board_match_direct_hit_score_factor / semantic_board_upgrade_composite_min_cooccurrence.",
		Up: func(db *gorm.DB) error {
			if !tableExists(db, "composite_components") {
				return nil // model not registered on this deployment
			}
			if err := withLockTimeout(db, "5s", func(tx *gorm.DB) error {
				if err := tx.Exec(`DO $$ BEGIN
					IF NOT EXISTS (
						SELECT 1 FROM information_schema.table_constraints
						WHERE constraint_name = 'fk_composite_components_composite'
						  AND table_name = 'composite_components'
					) THEN
						ALTER TABLE composite_components
							ADD CONSTRAINT fk_composite_components_composite
							FOREIGN KEY (composite_id) REFERENCES semantic_labels(id)
							ON DELETE CASCADE;
					END IF;
				END $$`).Error; err != nil {
					return fmt.Errorf("add fk_composite_components_composite: %w", err)
				}
				return nil
			}); err != nil {
				return err
			}

			seeds := []models.AISettings{
				{Key: "composite_label_dedupe_sim", Value: "0.95", Description: "组合标签 L2 去重阈值（cosine similarity，组合 embedding），高于等于此值追加 alias 而非新建（add-composite-labels）"},
				{Key: "semantic_board_match_direct_hit_score_factor", Value: "0.7", Description: "单标签重叠 direct_hit 降级折扣分（0-1，乘在原 score=1.0 上）；composite_hit 保持 1.0 不折扣（add-composite-labels）"},
				{Key: "semantic_board_upgrade_composite_min_cooccurrence", Value: "10", Description: "compose 建议候选共现对的最小共现次数（co-tag 窗口内），低于此值不进 LLM 裁决（add-composite-labels）"},
			}
			for _, s := range seeds {
				var existing models.AISettings
				if err := db.Where("key = ?", s.Key).First(&existing).Error; err != nil {
					if err := db.Create(&s).Error; err != nil {
						logging.Warnf("Migration 20260902_0001: failed to seed ai_settings key %s: %v", s.Key, err)
					}
				}
			}
			return nil
		},
	}
}

// crossBoardRelationMigration implements 20260901_0001: enum CHECKs and the
// partial unique index for cross-board relation discovery. AutoMigrate owns
// table/column creation; this migration owns the constraints AutoMigrate
// cannot express (add-evidence-backed-cross-board-relations design D4).
func crossBoardRelationMigration() Migration {
	return Migration{
		Version:     "20260901_0001",
		Description: "add-evidence-backed-cross-board-relations: CHECK enums + partial unique index for cross_board_relations and cross_board_relation_runs.",
		Up: func(db *gorm.DB) error {
			if !tableExists(db, "cross_board_relations") {
				return nil
			}
			if err := withLockTimeout(db, "5s", func(tx *gorm.DB) error {
				// 约束内容已冻结，重放时**不得**先 DROP 再 ADD：
				// ① PG 里 ADD CONSTRAINT 撞名会报错「already exists」，而任何一条语句
				//    报错都会中止当前事务——「吞掉 already exists 继续往下走」在事务内
				//    根本不成立（下一条只会拿到 25P02 current transaction is aborted）；
				// ② DROP 复合唯一约束还会被引用它的外键挡住（2BP01）。
				// 改为 DO 块守卫：约束不存在才添加，已存在则原样保留。
				statements := []string{
					`DO $$
					BEGIN
						IF NOT EXISTS (SELECT 1 FROM pg_constraint
							WHERE conname = 'ck_cross_board_relations_status'
							  AND conrelid = 'public.cross_board_relations'::regclass) THEN
							ALTER TABLE cross_board_relations ADD CONSTRAINT ck_cross_board_relations_status
								CHECK (status IN ('unresolved','proposed','confirmed','dismissed','expired'));
						END IF;
						IF NOT EXISTS (SELECT 1 FROM pg_constraint
							WHERE conname = 'ck_cross_board_relations_type'
							  AND conrelid = 'public.cross_board_relations'::regclass) THEN
							ALTER TABLE cross_board_relations ADD CONSTRAINT ck_cross_board_relations_type
								CHECK (relation_type IN ('causal','common_driver','divergence','correlated','contextual','unclear'));
						END IF;
						IF NOT EXISTS (SELECT 1 FROM pg_constraint
							WHERE conname = 'ck_cross_board_relations_verdict'
							  AND conrelid = 'public.cross_board_relations'::regclass) THEN
							ALTER TABLE cross_board_relations ADD CONSTRAINT ck_cross_board_relations_verdict
								CHECK (verification_verdict IN ('supported','contested','insufficient','rejected'));
						END IF;
						IF NOT EXISTS (SELECT 1 FROM pg_constraint
							WHERE conname = 'ck_cross_board_relations_grade'
							  AND conrelid = 'public.cross_board_relations'::regclass) THEN
							ALTER TABLE cross_board_relations ADD CONSTRAINT ck_cross_board_relations_grade
								CHECK (quality_grade IN ('high','medium','low'));
						END IF;
					END $$`,
					// Idempotent open-row uniqueness: one pending suggestion per hash.
					`CREATE UNIQUE INDEX IF NOT EXISTS uq_cross_board_relations_open ON cross_board_relations (suggestion_hash) WHERE status IN ('unresolved','proposed')`,
					`CREATE INDEX IF NOT EXISTS idx_cross_board_relations_confirmed_active ON cross_board_relations (source_board_id, target_board_id, quality_grade DESC, confirmed_at DESC) WHERE status = 'confirmed'`,
				}
				if tableExists(tx, "cross_board_relation_runs") {
					statements = append(statements, `DO $$
					BEGIN
						IF NOT EXISTS (SELECT 1 FROM pg_constraint
							WHERE conname = 'ck_cross_board_relation_runs_status'
							  AND conrelid = 'public.cross_board_relation_runs'::regclass) THEN
							ALTER TABLE cross_board_relation_runs ADD CONSTRAINT ck_cross_board_relation_runs_status
								CHECK (status IN ('queued','running','succeeded','partial','failed'));
						END IF;
						IF NOT EXISTS (SELECT 1 FROM pg_constraint
							WHERE conname = 'ck_cross_board_relation_runs_trigger'
							  AND conrelid = 'public.cross_board_relation_runs'::regclass) THEN
							ALTER TABLE cross_board_relation_runs ADD CONSTRAINT ck_cross_board_relation_runs_trigger
								CHECK (trigger_kind IN ('manual','auto'));
						END IF;
					END $$`)
				}
				for _, stmt := range statements {
					if err := tx.Exec(stmt).Error; err != nil {
						return fmt.Errorf("cross-board relation migration: %w", err)
					}
				}
				return nil
			}); err != nil {
				return err
			}
			return nil
		},
	}
}

// referenceRoleSeedRetireMigration disables the system's pristine seed
// author profile (board-level-deep-analysis tasks 6.3): the retired write
// chain no longer injects reference roles into any prompt, so the seeded
// enabled=true default must flip off. Identity is pinned to name + seeded
// title + the frozen embedded content bytes — a row the user actually
// edited (content or title drifted) is deliberately left untouched (it is
// user content now; it stays inert anyway because no prompt caller reads
// the table anymore). The old table and original bytes are preserved.

// analysisMethodLegacyCopyMigration copies every old reference role into the
// new method-card library once, disabled and marked legacy. Name conflicts are
// deliberately skipped so a user-edited method is never overwritten.
func analysisMethodLegacyCopyMigration() Migration {
	return Migration{
		Version:     "20260828_0002",
		Description: "board-level-deep-analysis: non-destructively copy reference_roles into disabled legacy analysis_methods.",
		Up: func(db *gorm.DB) error {
			if !tableExists(db, "reference_roles") || !tableExists(db, "analysis_methods") {
				return nil
			}
			selectionMeta := `{"applicable_when":[],"avoid_when":[],"required_evidence":[],"failure_modes":[]}`
			if err := db.Exec(`INSERT INTO analysis_methods
				(name, title, summary, selection_meta, content, enabled, legacy, created_at, updated_at)
				SELECT rr.name, rr.title,
					'从旧参考角色迁移；需补齐适用边界并人工启用。',
					?::jsonb, rr.content, false, true, rr.created_at, rr.updated_at
				FROM reference_roles rr
				ON CONFLICT (name) DO NOTHING`, selectionMeta).Error; err != nil {
				return fmt.Errorf("copy legacy reference roles to analysis methods: %w", err)
			}
			return nil
		},
	}
}

// referenceRoleSeedRetireMigration implements 20260831_0001: flip the pristine
// system seed (enabled=true by 20260826_0002) to disabled. Identity is pinned
// to the frozen bytes (seeded name + title + embedded content) so a row the
// user actually edited is never touched — see the append-site comment.
func referenceRoleSeedRetireMigration() Migration {
	return Migration{
		Version:     "20260831_0001",
		Description: "board-level-deep-analysis: disable the untouched system seed reference role (author profiles retired from all prompts).",
		Up: func(db *gorm.DB) error {
			if !tableExists(db, "reference_roles") {
				return nil
			}
			// Only a row still byte-identical to the embedded seed (name + seeded
			// title + original content) is flipped; the 20260826_0002 seed history
			// is never rewritten. User-edited rows stay as-is (user content, and
			// inert regardless — no prompt caller reads the table anymore).
			if err := db.Exec(`UPDATE reference_roles
				SET enabled = false, updated_at = now()
				WHERE name = 'inside-america-v2'
				  AND title = '内部看美国·方法论画像（v2）'
				  AND content = $1
				  AND enabled`, insideAmericaMethodologyProfile).Error; err != nil {
				return fmt.Errorf("disable seed reference role: %w", err)
			}
			return nil
		},
	}
}

// InvalidateBoardCache is an optional hook set by the board service to invalidate
// the semantic board cache after the aux-label dup-merge migration (which modifies
// board_composition). Set from semantic_board_cache.go init().
var InvalidateBoardCache func()

// PruneUnderqualifiedCandidates hard-deletes all candidate topics with
// hit_count < upgradeThreshold. Sections referencing them are unlinked
// (persistent_topic_id / match fields / topic_status_at_report set to NULL).
// Relations pointing to deleted topics are cleaned up. Idempotent — second run
// is a no-op. Returns the number of topics deleted.
func PruneUnderqualifiedCandidates(db *gorm.DB, upgradeThreshold int) (deleted int, err error) {
	var topicIDs []uint
	if err = db.Table("board_persistent_topics").
		Where("status = ? AND hit_count < ?", "candidate", upgradeThreshold).
		Pluck("id", &topicIDs).Error; err != nil {
		return 0, fmt.Errorf("prune: query candidates: %w", err)
	}
	if len(topicIDs) == 0 {
		return 0, nil
	}

	// Collect affected board IDs before any mutation so we can rebuild
	// their relations after topic deletion.
	var boardIDs []uint
	if PruneRelationsRebuild != nil {
		if err = db.Table("board_persistent_topics").
			Where("id IN ?", topicIDs).
			Distinct("semantic_board_id").
			Pluck("semantic_board_id", &boardIDs).Error; err != nil {
			return 0, fmt.Errorf("prune: query board IDs: %w", err)
		}
	}

	err = db.Transaction(func(tx *gorm.DB) error {
		// Unlink sections that reference the to-be-deleted topics.
		if err2 := tx.Table("daily_report_sections").
			Where("persistent_topic_id IN ?", topicIDs).
			Updates(map[string]interface{}{
				"persistent_topic_id":    nil,
				"topic_match_distance":   nil,
				"topic_match_confidence": nil,
				"topic_status_at_report": nil,
			}).Error; err2 != nil {
			return fmt.Errorf("prune: unlink sections: %w", err2)
		}

		// Delete the candidate rows.
		if err2 := tx.Table("board_persistent_topics").
			Where("id IN ?", topicIDs).
			Delete(nil).Error; err2 != nil {
			return fmt.Errorf("prune: delete topics: %w", err2)
		}

		// Rebuild relations for each affected board so stale identity /
		// similarity edges pointing to now-deleted topics are dropped.
		for _, boardID := range boardIDs {
			if err2 := PruneRelationsRebuild(tx, boardID); err2 != nil {
				return fmt.Errorf("prune: rebuild relations for board %d: %w", boardID, err2)
			}
		}

		return nil
	})
	if err != nil {
		return 0, err
	}
	return len(topicIDs), nil
}

func watchMaterializedHintCleanupMigration() Migration {
	return Migration{
		Version:     "20260905_0001",
		Description: "watch-materialize-llm-adjudication: one-shot delete of hint rows illegally produced by materialized-track watches.",
		Up: func(db *gorm.DB) error {
			if !tableExists(db, "topic_watch_hits") || !tableExists(db, "board_topic_watches") {
				return nil // no watch tables on this deployment — nothing to clean
			}
			if err := db.Exec(`DELETE FROM topic_watch_hits
				WHERE watch_id IN (
					SELECT id FROM board_topic_watches
					WHERE type IN ('keyword_topic', 'sentence_topic'))`).Error; err != nil {
				return fmt.Errorf("delete materialized-watch hint rows: %w", err)
			}
			return nil
		},
	}
}

// legacyDiscoverNewPendingDismissMigration one-shot dismisses pending suggestions
// produced by the retired discover_new pipeline (split-board-upgrade-directions
// 报障修复，2026-09-07)：旧管线语义与四格新语义不兼容，且 hash 含旧 mode 值永不
// 被新幂等命中——150 条旧 pending 建议永久混入列表干扰判断。置 dismissed 留痕
// （非物理删除，可事后查询回溯），幂等，二次执行 no-op。
func legacyDiscoverNewPendingDismissMigration() Migration {
	return Migration{
		Version:     "20260907_0001",
		Description: "split-board-upgrade-directions: one-shot dismiss of legacy discover_new pending suggestions (old pipeline retired, hash format incompatible).",
		Up: func(db *gorm.DB) error {
			if !tableExists(db, "board_upgrade_suggestions") {
				return nil
			}
			if err := db.Exec(`UPDATE board_upgrade_suggestions
				SET status = 'dismissed',
					dismiss_reason = 'legacy_discover_new_cleanup',
					resolved_at = now()
				WHERE status = 'pending' AND mode = 'discover_new'`).Error; err != nil {
				return fmt.Errorf("dismiss legacy discover_new pending suggestions: %w", err)
			}
			return nil
		},
	}
}

// laneSnapshotFKMigration implements 20260910_0001: add the FK
// topic_lane_snapshots.persistent_topic_id → board_persistent_topics(id)
// ON DELETE CASCADE (overview-lane-dynamics design D2). AutoMigrate creates
// the table + unique index but runs with DisableForeignKeyConstraintWhenMigrating,
// so the FK lives here (same policy as fk_topic_watch_hits_watch / 20260825_0001).
// Hard-deleting a topic removes its snapshot — the snapshot is a pure derived
// cache, orphan rows are meaningless. Idempotent; skips when either table is
// absent (topicgraph-less deployments).
func laneSnapshotFKMigration() Migration {
	return Migration{
		Version:     "20260910_0001",
		Description: "overview-lane-dynamics: FK topic_lane_snapshots.persistent_topic_id → board_persistent_topics ON DELETE CASCADE.",
		Up: func(db *gorm.DB) error {
			if !tableExists(db, "topic_lane_snapshots") || !tableExists(db, "board_persistent_topics") {
				return nil
			}
			// Orphan cleanup BEFORE the ADD CONSTRAINT (it validates existing rows;
			// a stray orphan would fail the whole migration). The table ships new
			// in this deploy so orphans cannot pre-exist — the guard is defensive,
			// mirroring the watch-hit FK policy.
			if err := db.Exec(`DELETE FROM topic_lane_snapshots
				WHERE persistent_topic_id NOT IN (SELECT id FROM board_persistent_topics)`).Error; err != nil {
				return fmt.Errorf("delete orphan lane snapshots: %w", err)
			}
			// FK (constraint DDL takes AccessExclusiveLock — guard with lock timeout).
			if err := withLockTimeout(db, "5s", func(tx *gorm.DB) error {
				if err := tx.Exec(`DO $$ BEGIN
					IF NOT EXISTS (
						SELECT 1 FROM information_schema.table_constraints
						WHERE constraint_name = 'fk_topic_lane_snapshots_topic'
							  AND table_name = 'topic_lane_snapshots'
					) THEN
						ALTER TABLE topic_lane_snapshots
							ADD CONSTRAINT fk_topic_lane_snapshots_topic
							FOREIGN KEY (persistent_topic_id) REFERENCES board_persistent_topics(id)
							ON DELETE CASCADE;
					END IF;
				END $$`).Error; err != nil {
					return fmt.Errorf("add fk_topic_lane_snapshots_topic: %w", err)
				}
				return nil
			}); err != nil {
				return err
			}
			return nil
		},
	}
}

// completionOnRefreshOffMigration implements 20260920_0002
// (unify-feed-summary-toggles):
// ①存量 feed 的 completion_on_refresh 全部置 false——历史遗产 24/24 全 true，
// 且该字段从死字段变为真正生效的"刷新后自动总结"闸门，置 false 后自动总结
// 静默关闭（想恢复的逐 feed 手动打开）；②冻结存量自动路径积压标记
// pending/incomplete → complete——闸门关后这些文章不会再被调度扫描捞起，
// 重置为 complete 避免 overview 待处理计数虚高。failed 保留以维持失败
// 可观测性（迁移后不会再被扫描捞起，无需重置）。幂等：两条 UPDATE 的 WHERE
// 条件在第二次执行时均命中 0 行。
func completionOnRefreshOffMigration() Migration {
	return Migration{
		Version:     "20260920_0002",
		Description: "unify-feed-summary-toggles: set all feeds completion_on_refresh=false and freeze legacy pending/incomplete summary_status to complete.",
		Up: func(db *gorm.DB) error {
			if !tableExists(db, "feeds") {
				return nil
			}
			if err := db.Exec(`
				UPDATE feeds SET completion_on_refresh = false
				WHERE completion_on_refresh = true
			`).Error; err != nil {
				return fmt.Errorf("reset feeds.completion_on_refresh: %w", err)
			}
			if !tableExists(db, "articles") {
				return nil
			}
			if err := db.Exec(`
				UPDATE articles SET summary_status = 'complete'
				WHERE summary_status IN ('pending', 'incomplete')
			`).Error; err != nil {
				return fmt.Errorf("freeze legacy pending/incomplete summary_status: %w", err)
			}
			return nil
		},
	}
}

// watchSuggestionCleanupMigration one-shot deletes pending watch suggestions
// (split-board-upgrade-directions: watch 观察池退役——存量 pending watch 行
// 清理，幂等，二次执行 no-op。高置信 merge 存量 pending 行保留可确认).
func watchSuggestionCleanupMigration() Migration {
	return Migration{
		Version:     "20260905_0002",
		Description: "split-board-upgrade-directions: one-shot delete of pending watch suggestions (observation pool retired).",
		Up: func(db *gorm.DB) error {
			if !tableExists(db, "board_upgrade_suggestions") {
				return nil
			}
			if err := db.Exec(`DELETE FROM board_upgrade_suggestions WHERE decision = 'watch'`).Error; err != nil {
				return fmt.Errorf("delete pending watch suggestions: %w", err)
			}
			return nil
		},
	}
}

// marginNotesTablesMigration implements 20260922_0003
// (daily-report-margin-notes design D1):
//
// 三张新表全部由本版本化迁移创建（模型不注册 AutoMigrate，SQL 是 schema 唯一
// 权威——vector 维度、jsonb 数组契约、term_norm 唯一索引需要显式控制）：
//
//	report_annotations  批注锚点：quoted_text + report/section/thread 归属 +
//	                    字符偏移线索（日报 period 不可变保证可重放定位）；
//	annotation_qas      问答轮：cited_article_ids/extracted_terms jsonb 数组
//	                    契约（空写 [] 不写 null，对齐 thread 引用数组），审计
//	                    冗余存 operation/provider/model（ai_call_logs 7 天清理，
//	                    长期分析另行保存）；FK ON DELETE CASCADE 兑现「删除批注
//	                    连带问答」；
//	term_notes          术语库：term_norm 归一化唯一键（trim+全角→半角+大小写
//	                    折叠），hit_count 累计不重复建条；embedding 为**无维度限定**
//	                    的 vector（维度是运行时属性，见
//	                    unify-vector-column-dimension），可空列为 P2 相似词归并
//	                    预留，P1 不写入。
//
// 无存量数据迁移，纯新增；回滚=前端还原+表保留（孤儿数据无害）。幂等：
// CREATE TABLE/INDEX IF NOT EXISTS + FK 按约束名探测。
func marginNotesTablesMigration() Migration {
	return Migration{
		Version:     "20260922_0003",
		Description: "daily-report-margin-notes: create report_annotations / annotation_qas / term_notes (margin note anchors, QA turns with jsonb array contract, term library with normalized unique key and nullable dimensionless vector embedding reserved for P2 — the dimension is a runtime property, not a schema contract).",
		Up: func(db *gorm.DB) error {
			if err := db.Exec(`
				CREATE TABLE IF NOT EXISTS report_annotations (
					id                  BIGSERIAL PRIMARY KEY,
					report_id           BIGINT       NOT NULL,
					section_id          BIGINT       NOT NULL,
					thread_id           BIGINT       NULL,
					quoted_text         TEXT         NOT NULL,
					anchor_offset_start INT          NOT NULL DEFAULT 0,
					anchor_offset_end   INT          NOT NULL DEFAULT 0,
					created_at          TIMESTAMPTZ  NOT NULL DEFAULT now()
				)`).Error; err != nil {
				return fmt.Errorf("create report_annotations: %w", err)
			}
			if err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_report_annotations_report ON report_annotations (report_id)`).Error; err != nil {
				return fmt.Errorf("create idx_report_annotations_report: %w", err)
			}
			if err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_report_annotations_created ON report_annotations (created_at)`).Error; err != nil {
				return fmt.Errorf("create idx_report_annotations_created: %w", err)
			}

			if err := db.Exec(`
				CREATE TABLE IF NOT EXISTS annotation_qas (
					id                BIGSERIAL PRIMARY KEY,
					annotation_id     BIGINT        NOT NULL,
					question          TEXT          NOT NULL,
					answer            TEXT          NOT NULL,
					cited_article_ids JSONB         NOT NULL DEFAULT '[]'::jsonb,
					extracted_terms   JSONB         NOT NULL DEFAULT '[]'::jsonb,
					operation         VARCHAR(80)   NULL,
					provider          VARCHAR(100)  NULL,
					model             VARCHAR(100)  NULL,
					created_at        TIMESTAMPTZ   NOT NULL DEFAULT now()
				)`).Error; err != nil {
				return fmt.Errorf("create annotation_qas: %w", err)
			}
			if err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_annotation_qas_annotation ON annotation_qas (annotation_id)`).Error; err != nil {
				return fmt.Errorf("create idx_annotation_qas_annotation: %w", err)
			}
			// 删除批注连带问答（spec「删除批注连带问答」）——DB 级 CASCADE 兜底，
			// repository 删除路径仍显式先删问答再删批注（双保险，语义一致）。
			if err := withLockTimeout(db, "5s", func(tx *gorm.DB) error {
				if err := tx.Exec(`DO $$ BEGIN
					IF NOT EXISTS (
						SELECT 1 FROM information_schema.table_constraints
						WHERE constraint_name = 'fk_annotation_qas_annotation'
							  AND table_name = 'annotation_qas'
					) THEN
						ALTER TABLE annotation_qas
							ADD CONSTRAINT fk_annotation_qas_annotation
							FOREIGN KEY (annotation_id) REFERENCES report_annotations(id)
							ON DELETE CASCADE;
					END IF;
				END $$`).Error; err != nil {
					return fmt.Errorf("add fk_annotation_qas_annotation: %w", err)
				}
				return nil
			}); err != nil {
				return err
			}

			if err := db.Exec(`
				CREATE TABLE IF NOT EXISTS term_notes (
					id                  BIGSERIAL PRIMARY KEY,
					term_norm           TEXT         NOT NULL,
					term_display        TEXT         NOT NULL,
					hit_count           INT          NOT NULL DEFAULT 1,
					first_seen_board_id BIGINT       NULL,
					first_seen_date     DATE         NULL,
					last_seen_at        TIMESTAMPTZ  NOT NULL DEFAULT now(),
					created_at          TIMESTAMPTZ  NOT NULL DEFAULT now(),
					embedding           vector DEFAULT NULL
				)`).Error; err != nil {
				return fmt.Errorf("create term_notes: %w", err)
			}
			if err := db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS uq_term_notes_term_norm ON term_notes (term_norm)`).Error; err != nil {
				return fmt.Errorf("create uq_term_notes_term_norm: %w", err)
			}
			return nil
		},
	}
}

// vendorSnapshotDir is the directory the pre-migration ai_providers JSON
// snapshot is written to. Package-level so the migration test can redirect it
// to a t.TempDir() instead of polluting the repo working directory.
var vendorSnapshotDir = "data"

// vendorModelMigration implements 20261005_0001
// (ai-provider-model-overhaul design D1/D2)：
//
//	ai_providers 单层实体拆解为 ai_vendors + ai_models 两层（每行 provider →
//	1 vendor + 1 model），ai_route_providers 绑定改挂 ai_route_models（priority
//	保序、thinking_level 置 NULL=继承模型默认档）。方言/结构化档按 provider 名
//	前缀推断（存量 3 行硬编码映射）；enable_thinking 布尔按 true→high/false→off
//	写入模型默认思考档（embedding 行恒 NULL）。拆解前全表 JSON 快照落
//	ai_providers_legacy_snapshot.json（失败仅告警不阻断）。死字段
//	（ai_providers.metadata、ai_routes.strategy/priority）不迁不建；旧表旧列
//	物理保留不 DROP（回滚保险，下个大版本清理）。
//
// 幂等：vendor 按 legacy_provider_id 判重命中即跳过整行；name 重名跳过并告警；
//
//	绑定改挂 NOT EXISTS 防重；建表/建索引全部 IF NOT EXISTS。
func vendorModelMigration() Migration {
	return Migration{
		Version:     "20261005_0001",
		Description: "ai-provider-model-overhaul: split ai_providers into ai_vendors/ai_models two-layer tables, reattach ai_route_providers bindings to ai_route_models (priority preserved), legacy snapshot to data/.",
		Up: func(db *gorm.DB) error {
			// 1. 建三层新表（字段与 models.AIVendor/AIModel/AIRouteModel 一致，含全部索引）
			stmts := []string{
				`CREATE TABLE IF NOT EXISTS ai_vendors (
					id                 BIGSERIAL PRIMARY KEY,
					name               VARCHAR(100) NOT NULL,
					preset_id          VARCHAR(50)  NOT NULL DEFAULT '',
					provider_type      VARCHAR(50)  NOT NULL,
					base_url           VARCHAR(500) NOT NULL,
					api_key            TEXT         NULL,
					timeout_seconds    BIGINT       NOT NULL,
					start_command      TEXT         NULL,
					thinking_dialect   VARCHAR(50)  NOT NULL DEFAULT 'none',
					structured_output  VARCHAR(50)  NOT NULL DEFAULT 'json_schema',
					legacy_provider_id BIGINT       NULL,
					enabled            BOOLEAN      NOT NULL,
					created_at         TIMESTAMPTZ  NOT NULL DEFAULT now(),
					updated_at         TIMESTAMPTZ  NOT NULL DEFAULT now()
				)`,
				`CREATE UNIQUE INDEX IF NOT EXISTS idx_ai_vendors_name ON ai_vendors(name)`,
				`CREATE INDEX IF NOT EXISTS idx_ai_vendors_provider_type ON ai_vendors(provider_type)`,
				`CREATE INDEX IF NOT EXISTS idx_ai_vendors_legacy_provider_id ON ai_vendors(legacy_provider_id)`,
				`CREATE INDEX IF NOT EXISTS idx_ai_vendors_enabled ON ai_vendors(enabled)`,
				`CREATE TABLE IF NOT EXISTS ai_models (
					id                 BIGSERIAL PRIMARY KEY,
					vendor_id          BIGINT       NOT NULL,
					name               VARCHAR(100) NOT NULL,
					model_kind         VARCHAR(20)  NOT NULL DEFAULT 'llm',
					thinking_level     VARCHAR(20)  NULL,
					supports_xhigh_max BOOLEAN      NOT NULL DEFAULT false,
					enabled            BOOLEAN      NOT NULL,
					status             VARCHAR(20)  NOT NULL DEFAULT 'ok',
					last_seen_at       TIMESTAMPTZ  NULL,
					created_at         TIMESTAMPTZ  NOT NULL DEFAULT now(),
					updated_at         TIMESTAMPTZ  NOT NULL DEFAULT now()
				)`,
				`CREATE INDEX IF NOT EXISTS idx_ai_models_vendor_id ON ai_models(vendor_id)`,
				`CREATE UNIQUE INDEX IF NOT EXISTS idx_ai_models_vendor_name ON ai_models(vendor_id, name)`,
				`CREATE INDEX IF NOT EXISTS idx_ai_models_model_kind ON ai_models(model_kind)`,
				`CREATE INDEX IF NOT EXISTS idx_ai_models_enabled ON ai_models(enabled)`,
				`CREATE INDEX IF NOT EXISTS idx_ai_models_status ON ai_models(status)`,
				`CREATE TABLE IF NOT EXISTS ai_route_models (
					id             BIGSERIAL PRIMARY KEY,
					route_id       BIGINT      NOT NULL,
					model_id       BIGINT      NOT NULL,
					priority       BIGINT      NOT NULL,
					thinking_level VARCHAR(20) NULL,
					enabled        BOOLEAN     NOT NULL,
					created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
					updated_at     TIMESTAMPTZ NOT NULL DEFAULT now()
				)`,
				`CREATE UNIQUE INDEX IF NOT EXISTS idx_air_route_model_link ON ai_route_models(route_id, model_id)`,
				`CREATE INDEX IF NOT EXISTS idx_air_route_models_priority ON ai_route_models(priority)`,
				`CREATE INDEX IF NOT EXISTS idx_air_route_models_enabled ON ai_route_models(enabled)`,
			}
			for _, stmt := range stmts {
				if err := db.Exec(stmt).Error; err != nil {
					return fmt.Errorf("create vendor/model tables: %w", err)
				}
			}

			// 2. trace_id 观测索引：降级观测面板按 trace_id 分组还原降级链（D6），
			// 同表加索引非 schema 语义变更，幂等。
			if err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_call_logs_trace ON ai_call_logs(trace_id)`).Error; err != nil {
				return fmt.Errorf("create idx_call_logs_trace: %w", err)
			}

			// 3. 拆解前快照（ai_providers 存在且有行才写，失败仅告警）
			if tableExists(db, "ai_providers") {
				snapshotLegacyProviders(db)
			}

			// 4. 拆解（ai_providers 不存在则无事可做）
			if !tableExists(db, "ai_providers") {
				return nil
			}
			vendors, models, bindings, err := splitLegacyProviders(db)
			if err != nil {
				return err
			}

			// 5. 冲突告警：同一 model 同时挂在 embedding 路由与 llm 路由——存量脏
			// 数据只可见化不阻断（ai-route-model spec「存量迁移后冲突告警」）
			var conflicts []struct {
				ID   uint
				Name string
			}
			if err := db.Raw(`SELECT rm.model_id AS id, m.name AS name FROM ai_route_models rm
				JOIN ai_routes r ON r.id = rm.route_id
				JOIN ai_models m ON m.id = rm.model_id
				GROUP BY rm.model_id, m.name
				HAVING COUNT(*) FILTER (WHERE r.capability = 'embedding') > 0
				   AND COUNT(*) FILTER (WHERE r.capability <> 'embedding') > 0`).Scan(&conflicts).Error; err != nil {
				return fmt.Errorf("check embedding/llm binding conflicts: %w", err)
			}
			for _, c := range conflicts {
				logging.Warnf("vendor-model migration: model %q (id=%d) bound to both embedding and llm routes", c.Name, c.ID)
			}

			// 6. 对账日志：部署后验证清单核对计数（design Migration Plan）
			logging.Infof("vendor-model migration: vendors=%d models=%d bindings=%d", vendors, models, bindings)
			return nil
		},
	}
}

// snapshotLegacyProviders writes the full ai_providers table (SELECT * scanned
// into generic row maps) to <vendorSnapshotDir>/ai_providers_legacy_snapshot.json
// before the split mutates anything. Snapshot is a rollback aid only: any failure
// warns and continues, never blocks the migration.
func snapshotLegacyProviders(db *gorm.DB) {
	var count int64
	if err := db.Raw(`SELECT count(*) FROM ai_providers`).Scan(&count).Error; err != nil || count == 0 {
		return // 空表/读失败：无内容可快照（读失败留给拆解步骤显式报错）
	}
	var rows []map[string]any
	if err := db.Raw(`SELECT * FROM ai_providers`).Scan(&rows).Error; err != nil {
		logging.Warnf("vendor-model migration: legacy snapshot read failed: %v", err)
		return
	}
	payload, err := json.Marshal(rows)
	if err != nil {
		logging.Warnf("vendor-model migration: legacy snapshot marshal failed: %v", err)
		return
	}
	if err := os.MkdirAll(vendorSnapshotDir, 0o750); err != nil {
		logging.Warnf("vendor-model migration: legacy snapshot mkdir %s: %v", vendorSnapshotDir, err)
		return
	}
	path := filepath.Join(vendorSnapshotDir, "ai_providers_legacy_snapshot.json")
	if err := os.WriteFile(path, payload, 0o600); err != nil {
		logging.Warnf("vendor-model migration: legacy snapshot write %s: %v", path, err)
	}
}

// legacyProviderRow is the projection of ai_providers the split consumes.
// All columns COALESCEd at read time: AutoMigrate-created legacy columns are
// nullable and pre-default rows may hold NULLs.
type legacyProviderRow struct {
	ID             uint
	Name           string
	ProviderType   string
	BaseURL        string
	APIKey         string
	Model          string
	Enabled        bool
	TimeoutSeconds int
	ModelKind      string
	StartCommand   string
	EnableThinking bool
}

// splitLegacyProviders expands each ai_providers row into one ai_vendors row +
// one ai_models row and reattaches ai_route_providers bindings to ai_route_models.
// Returns the split counters for the reconciliation log.
func splitLegacyProviders(db *gorm.DB) (vendors, models, bindings int, err error) {
	var legacy []legacyProviderRow
	if err := db.Raw(`SELECT id,
		COALESCE(name, '') AS name,
		COALESCE(provider_type, '') AS provider_type,
		COALESCE(base_url, '') AS base_url,
		COALESCE(api_key, '') AS api_key,
		COALESCE(model, '') AS model,
		COALESCE(enabled, false) AS enabled,
		COALESCE(timeout_seconds, 0) AS timeout_seconds,
		COALESCE(model_kind, '') AS model_kind,
		COALESCE(start_command, '') AS start_command,
		COALESCE(enable_thinking, false) AS enable_thinking
		FROM ai_providers`).Scan(&legacy).Error; err != nil {
		return 0, 0, 0, fmt.Errorf("read legacy ai_providers: %w", err)
	}

	for _, p := range legacy {
		// 已迁移判重：legacy_provider_id 命中即跳过整行（含 model 与绑定改挂）
		var migrated uint
		if err := db.Raw(`SELECT id FROM ai_vendors WHERE legacy_provider_id = ?`, p.ID).Scan(&migrated).Error; err != nil {
			return vendors, models, bindings, fmt.Errorf("check legacy_provider_id %d: %w", p.ID, err)
		}
		if migrated != 0 {
			continue
		}
		// 重名判重：vendor.name 全局唯一（继承 provider.name 语义），重名跳过并告警
		var nameOwner uint
		if err := db.Raw(`SELECT id FROM ai_vendors WHERE name = ?`, p.Name).Scan(&nameOwner).Error; err != nil {
			return vendors, models, bindings, fmt.Errorf("check vendor name %q: %w", p.Name, err)
		}
		if nameOwner != 0 {
			logging.Warnf("vendor-model migration: vendor name %q already exists, skipping legacy provider %d", p.Name, p.ID)
			continue
		}

		dialect, structured := inferVendorDialect(p.Name)
		timeout := p.TimeoutSeconds
		if timeout <= 0 {
			timeout = 120
		}
		var vendorID uint
		if err := db.Raw(`INSERT INTO ai_vendors
			(name, preset_id, provider_type, base_url, api_key, timeout_seconds, start_command,
			 thinking_dialect, structured_output, legacy_provider_id, enabled, created_at, updated_at)
			VALUES (?, '', ?, ?, ?, ?, ?, ?, ?, ?, ?, now(), now()) RETURNING id`,
			p.Name, p.ProviderType, p.BaseURL, p.APIKey, timeout, p.StartCommand,
			dialect, structured, p.ID, p.Enabled).Scan(&vendorID).Error; err != nil {
			return vendors, models, bindings, fmt.Errorf("insert vendor for legacy provider %d: %w", p.ID, err)
		}
		vendors++

		// 模型：name=provider.model；model_kind 空则 llm；thinking_level——llm 行
		// 按 enable_thinking true→high/false→off，embedding 行恒 NULL（无思考语义）
		kind := p.ModelKind
		if kind == "" {
			kind = "llm"
		}
		var thinkingLevel any // nil → NULL
		if kind != "embedding" {
			if p.EnableThinking {
				thinkingLevel = "high"
			} else {
				thinkingLevel = "off"
			}
		}
		var modelID uint
		if err := db.Raw(`INSERT INTO ai_models
			(vendor_id, name, model_kind, thinking_level, supports_xhigh_max, enabled, status,
			 last_seen_at, created_at, updated_at)
			VALUES (?, ?, ?, ?, false, ?, 'ok', now(), now(), now()) RETURNING id`,
			vendorID, p.Model, kind, thinkingLevel, p.Enabled).Scan(&modelID).Error; err != nil {
			return vendors, models, bindings, fmt.Errorf("insert model for legacy provider %d: %w", p.ID, err)
		}
		models++

		// 绑定改挂：provider_id → 拆出 model_id，priority 保序（旧列可空取 0）、
		// thinking_level 置 NULL=继承；NOT EXISTS 防重复改挂
		res := db.Exec(`INSERT INTO ai_route_models (route_id, model_id, priority, thinking_level, enabled, created_at, updated_at)
			SELECT rp.route_id, ?, COALESCE(rp.priority, 0), NULL, COALESCE(rp.enabled, true), now(), now()
			FROM ai_route_providers rp
			WHERE rp.provider_id = ?
			  AND NOT EXISTS (
				SELECT 1 FROM ai_route_models rm
				WHERE rm.route_id = rp.route_id AND rm.model_id = ?
			  )`, modelID, p.ID, modelID)
		if res.Error != nil {
			return vendors, models, bindings, fmt.Errorf("reattach bindings for legacy provider %d: %w", p.ID, res.Error)
		}
		bindings += int(res.RowsAffected)
	}
	return vendors, models, bindings, nil
}

// inferVendorDialect maps a legacy provider name to (thinking_dialect,
// structured_output). 存量就 3 行，硬编码前缀映射：qwen*→qwen-chat-template+
// json_schema、deepseek*→deepseek+json_object、其余 none+json_schema。
func inferVendorDialect(name string) (dialect, structured string) {
	switch {
	case strings.HasPrefix(name, "qwen"):
		return "qwen-chat-template", "json_schema"
	case strings.HasPrefix(name, "deepseek"):
		return "deepseek", "json_object"
	default:
		return "none", "json_schema"
	}
}

// keyEventMarkersMigration implements 20261012_0001
// (add-topic-key-event-markers design D1): the topic_key_event_markers /
// key_event_recommendation_runs tables are created by AutoMigrate (with the
// board+source_key unique index); this migration owns what AutoMigrate cannot
// express — the state CHECK, the recommendation-run state CHECK, and the FK
// deletion semantics that keep user choices alive:
//   - semantic_board_id → semantic_boards(id) ON DELETE CASCADE（板块删除沿用删除语义）
//   - persistent_topic_id → board_persistent_topics(id) ON DELETE SET NULL
//     （topic 硬删保留注记，孤立注记不混入别的泳道）
//   - source_report_id / source_thread_id / source_section_id → SET NULL
//     （重建/删除只断开引用，不删人工选择；快照正文保留）
//
// Idempotent (drop-then-add CHECK + IF NOT EXISTS probes for FKs); skips
// when the tables are absent (topicgraph-less deployments).
func keyEventMarkersMigration() Migration {
	return Migration{
		Version:     "20261012_0001",
		Description: "add-topic-key-event-markers: topic_key_event_markers state CHECK + SET NULL/CASCADE FKs; key_event_recommendation_runs state CHECK.",
		Up: func(db *gorm.DB) error {
			if !tableExists(db, "topic_key_event_markers") {
				return nil
			}
			if err := withLockTimeout(db, "5s", func(tx *gorm.DB) error {
				statements := []string{
					`ALTER TABLE topic_key_event_markers DROP CONSTRAINT IF EXISTS ck_topic_key_event_markers_state`,
					`ALTER TABLE topic_key_event_markers ADD CONSTRAINT ck_topic_key_event_markers_state CHECK (state IN ('suggested','confirmed','dismissed'))`,
				}
				for _, stmt := range statements {
					if err := tx.Exec(stmt).Error; err != nil {
						return fmt.Errorf("key event markers CHECK: %w", err)
					}
				}
				return nil
			}); err != nil {
				return err
			}
			if tableExists(db, "key_event_recommendation_runs") {
				if err := withLockTimeout(db, "5s", func(tx *gorm.DB) error {
					if err := tx.Exec(`ALTER TABLE key_event_recommendation_runs DROP CONSTRAINT IF EXISTS ck_key_event_recommendation_runs_state`).Error; err != nil {
						return fmt.Errorf("drop run state CHECK: %w", err)
					}
					if err := tx.Exec(`ALTER TABLE key_event_recommendation_runs ADD CONSTRAINT ck_key_event_recommendation_runs_state CHECK (state IN ('running','done','failed'))`).Error; err != nil {
						return fmt.Errorf("add run state CHECK: %w", err)
					}
					return nil
				}); err != nil {
					return err
				}
			}
			fks := []struct{ name, table, column, ref, del string }{
				// Boards are SemanticLabel rows (label_type='board') — the board FK
				// references semantic_labels(id), same as every other board-owned table.
				{"fk_topic_key_event_markers_board", "topic_key_event_markers", "semantic_board_id", "semantic_labels(id)", "CASCADE"},
				{"fk_topic_key_event_markers_topic", "topic_key_event_markers", "persistent_topic_id", "board_persistent_topics(id)", "SET NULL"},
				{"fk_topic_key_event_markers_report", "topic_key_event_markers", "source_report_id", "board_daily_reports(id)", "SET NULL"},
				{"fk_topic_key_event_markers_section", "topic_key_event_markers", "source_section_id", "daily_report_sections(id)", "SET NULL"},
				{"fk_topic_key_event_markers_thread", "topic_key_event_markers", "source_thread_id", "daily_report_threads(id)", "SET NULL"},
			}
			for _, fk := range fks {
				// Guard: skip when the referenced table is absent on this deploy
				// (e.g. semantic_boards may be absent on minimal deployments).
				refTable := strings.TrimSuffix(fk.ref, "(id)")
				if !tableExists(db, refTable) {
					continue
				}
				if err := withLockTimeout(db, "5s", func(tx *gorm.DB) error {
					if err := tx.Exec(fmt.Sprintf(`DO $$ BEGIN
						IF NOT EXISTS (
							SELECT 1 FROM information_schema.table_constraints
							WHERE constraint_name = '%s' AND table_name = '%s'
						) THEN
							ALTER TABLE %s ADD CONSTRAINT %s FOREIGN KEY (%s) REFERENCES %s ON DELETE %s;
						END IF;
					END $$`, fk.name, fk.table, fk.table, fk.name, fk.column, fk.ref, fk.del)).Error; err != nil {
						return fmt.Errorf("add %s: %w", fk.name, err)
					}
					return nil
				}); err != nil {
					return err
				}
			}
			return nil
		},
	}
}

// marginNotesWebSourcesMigration implements 20260924_0001
// (daily-report-margin-notes 联网扩充，design D7)：
//
//	annotation_qas 追加 cited_web_sources jsonb NOT NULL DEFAULT '[]'——
//	AI 回答引用的网络来源结构化沉淀（[{"title","url"}] 保序去重，url 经
//	本次搜索结果集白名单校验，空写 [] 不写 null——对齐 cited_article_ids
//	数组契约）。存量行为回填为 []（无网络来源，语义正确）。
//
// 幂等：ADD COLUMN IF NOT EXISTS。回滚=列保留（旧前端不读则无害）。
func marginNotesWebSourcesMigration() Migration {
	return Migration{
		Version:     "20260924_0001",
		Description: "daily-report-margin-notes web augmentation: annotation_qas.cited_web_sources jsonb (whitelisted [{title,url}] web citations, array contract empty=[])",
		Up: func(db *gorm.DB) error {
			if err := db.Exec(`ALTER TABLE annotation_qas
				ADD COLUMN IF NOT EXISTS cited_web_sources JSONB NOT NULL DEFAULT '[]'::jsonb`).Error; err != nil {
				return fmt.Errorf("add annotation_qas.cited_web_sources: %w", err)
			}
			return nil
		},
	}
}

// marginNotesWebSearchStatusMigration implements 20261007_0001
// (add-searxng-health-indicator design D5)：
//
//	annotation_qas 追加 web_search_status text NULL——问答轮联网搜索状态
//	（disabled/ok/empty；empty=链路启用但全部搜索失败或结果为空，落库不细分
//	unreachable/degraded——对读者语义同为「联网没帮上」）。存量行为 NULL
//	（历史数据无标记语义，不回填；前端 NULL 不渲染标记）。
//
// 幂等：ADD COLUMN IF NOT EXISTS。回滚=列残留无害（旧前端不读则忽略）。
func marginNotesWebSearchStatusMigration() Migration {
	return Migration{
		Version:     "20261007_0001",
		Description: "add-searxng-health-indicator: annotation_qas.web_search_status text NULL (per-turn web search status disabled/ok/empty, legacy rows stay NULL)",
		Up: func(db *gorm.DB) error {
			if err := db.Exec(`ALTER TABLE annotation_qas
				ADD COLUMN IF NOT EXISTS web_search_status TEXT NULL`).Error; err != nil {
				return fmt.Errorf("add annotation_qas.web_search_status: %w", err)
			}
			return nil
		},
	}
}

// pgTrgmMigration implements 20261013_0001 (add-article-search design D1):
// enable the pg_trgm contrib extension and build GIN trigram indexes on
// articles.title / articles.description. These back the keyword search path
// (LIKE '%q%' acceleration + similarity()), which replaces the retired
// tsvector path (its 'simple' tokenizer fails on Chinese — 97% recall loss,
// see the change proposal). Indexes cover title+description only; content is
// deliberately left unindexed (write-amplification trade-off, design Non-Goal).
//
// Idempotent: IF NOT EXISTS throughout. Reversible in practice: dropping the
// extension/indexes has no data dependency (Down stays nil per convention).
func pgTrgmMigration() Migration {
	return Migration{
		Version:     "20261013_0001",
		Description: "add-article-search design D1: enable pg_trgm and add GIN trgm indexes on articles.title/articles.description (LIKE acceleration replacing the tsvector path).",
		Up: func(db *gorm.DB) error {
			stmts := []string{
				`CREATE EXTENSION IF NOT EXISTS pg_trgm`,
				`CREATE INDEX IF NOT EXISTS idx_articles_title_trgm ON articles USING GIN (title gin_trgm_ops)`,
				`CREATE INDEX IF NOT EXISTS idx_articles_description_trgm ON articles USING GIN (description gin_trgm_ops)`,
			}
			for _, s := range stmts {
				if err := db.Exec(s).Error; err != nil {
					return fmt.Errorf("pg_trgm migration: %w", err)
				}
			}
			return nil
		},
	}
}

// searchVectorDropMigration implements 20261013_0002 (add-article-search
// design D7): retire the tsvector full-text apparatus on articles — the
// Chinese-tokenization-dead search_vector column, its update trigger, the
// trigger function and the legacy GIN index. The last consumer of the column
// switches to the pg_trgm path in the same change (20261013_0001 installs the
// replacement indexes first).
//
// The column data is regenerable (recomputable from title/description via the
// very trigger being dropped), so the drop loses nothing that cannot be
// rebuilt — design D7 rollback note: with migration A kept, a rolled-back code
// path simply cannot use full-text search until the column is recreated.
//
// Destructive (DROP COLUMN): self-skips unless MIGRATIONS_ALLOW_DESTRUCTIVE=1
// (production default = skip, by design — see db-migration-safety). Fully
// idempotent via IF EXISTS; trigger drops before the function so no dependency
// is left dangling.
func searchVectorDropMigration() Migration {
	return Migration{
		Version:     "20261013_0002",
		Description: "add-article-search design D7: DROP articles.search_vector column + articles_search_vector_trigger + articles_search_vector_update() + idx_articles_search_vector (tsvector path retired for pg_trgm; column data recomputable from title/description). Destructive — guarded by MIGRATIONS_ALLOW_DESTRUCTIVE. ⚠️ 不可逆 DROP（破坏性，受 `MIGRATIONS_ALLOW_DESTRUCTIVE` 守卫；列数据可再生，生产执行前需备份）",
		Up: func(db *gorm.DB) error {
			// Destructive (DROP COLUMN): skip unless MIGRATIONS_ALLOW_DESTRUCTIVE=1.
			// Production never sets the env — the self-skip there is expected (the
			// column simply outlives the code that stopped reading it); dev/test
			// open the gate to complete the retirement.
			if !IsDestructiveAllowed() {
				logging.Infof("skipping destructive migration 20261013_0002 (set MIGRATIONS_ALLOW_DESTRUCTIVE=1 to drop articles.search_vector)")
				return nil
			}
			stmts := []string{
				`DROP TRIGGER IF EXISTS articles_search_vector_trigger ON articles`,
				`DROP FUNCTION IF EXISTS articles_search_vector_update()`,
				`DROP INDEX IF EXISTS idx_articles_search_vector`,
				`ALTER TABLE articles DROP COLUMN IF EXISTS search_vector`,
			}
			for _, s := range stmts {
				if err := db.Exec(s).Error; err != nil {
					return fmt.Errorf("search_vector drop migration: %w", err)
				}
			}
			return nil
		},
	}
}

// questionResearchBriefMigration implements 20261019_0001
// (add-question-driven-news-research D1): board_signal_candidate gains a
// nullable research_brief jsonb column ({unknown, verification, source_ids}).
// Old rows stay NULL — never backfilled, never guessed; absence = legacy
// frozen-research protocol (seven sources only). AutoMigrate adds the column
// on fresh DBs; this migration only guards the ADD COLUMN IF NOT EXISTS path
// for existing deployments. Forward-only, idempotent.
func questionResearchBriefMigration() Migration {
	return Migration{
		Version:     "20261019_0001",
		Description: "add-question-driven-news-research D1: board_signal_candidate.research_brief nullable jsonb (unknown/verification/source_ids; NULL=legacy protocol, no backfill)",
		Up: func(db *gorm.DB) error {
			if !tableExists(db, "board_signal_candidate") {
				return nil
			}
			return withLockTimeout(db, "5s", func(tx *gorm.DB) error {
				if err := tx.Exec(`ALTER TABLE board_signal_candidate
					ADD COLUMN IF NOT EXISTS research_brief jsonb NULL`).Error; err != nil {
					return fmt.Errorf("board signal candidate research_brief: %w", err)
				}
				return nil
			})
		},
	}
}

// signalDispositionMigration implements 20261015_0001
// (redesign-enrichment-research-workbench design D1): the ignore marker table
// board_signal_disposition is created by AutoMigrate (model registered in the
// dataenrichment repository); AutoMigrate runs with
// DisableForeignKeyConstraintWhenMigrating, so the candidate FK lives here —
// same policy as fk_board_signal_research_progress_candidate (20260922_0002).
// The marker is meaningless without its candidate: ON DELETE CASCADE. No new
// columns on candidate/report tables; no backfill (absence of a row = not
// ignored). Idempotent (DROP CONSTRAINT IF EXISTS first; no-op when the
// table is absent).
func signalDispositionMigration() Migration {
	return Migration{
		Version:     "20261015_0001",
		Description: "redesign-enrichment-research-workbench design D1: FK board_signal_disposition.candidate_id -> board_signal_candidate(id) ON DELETE CASCADE (AutoMigrate owns the table; migration owns the FK; row present = ignored, no row = normal)",
		Up: func(db *gorm.DB) error {
			if !tableExists(db, "board_signal_disposition") {
				return nil
			}
			if !tableExists(db, "board_signal_candidate") {
				return nil
			}
			return withLockTimeout(db, "5s", func(tx *gorm.DB) error {
				for _, stmt := range []string{
					`ALTER TABLE board_signal_disposition
						DROP CONSTRAINT IF EXISTS fk_board_signal_disposition_candidate`,
					`ALTER TABLE board_signal_disposition
						ADD CONSTRAINT fk_board_signal_disposition_candidate
						FOREIGN KEY (candidate_id)
						REFERENCES board_signal_candidate (id)
						ON DELETE CASCADE`,
				} {
					if err := tx.Exec(stmt).Error; err != nil {
						return fmt.Errorf("board signal disposition constraint: %w", err)
					}
				}
				return nil
			})
		},
	}
}

// TopicTagIdentityHalfvecIndexName 是语义搜索标签召回使用的 halfvec HNSW
// 部分索引名（optimize-search-semantic-index design D1）。float32 HNSW 有
// 2000 维硬上限（当前 embedding 列 2560 维建不了），halfvec 支持到 4000 维；
// 只索引 identity 行（语义路径只查 identity），索引更小更快。
const TopicTagIdentityHalfvecIndexName = "idx_topic_tag_embeddings_identity_halfvec_hnsw"

// TopicTagIdentityHalfvecIndexDDL 是 halfvec identity 部分 HNSW 索引的单一
// DDL 事实源（design D1/D4）：启动迁移 20261023_0001 与 tagmanagement 的
// ensureVectorDimension 共用，避免两处 DDL 漂移。dim 必须是列的实际维度
// （halfvec 1..4000）；索引表达式把 float32 向量转半精度存储，查询侧用同一
// 表达式取候选。
func TopicTagIdentityHalfvecIndexDDL(dim int) string {
	// 参数是 int（非用户输入），无注入面；gosec 仍按格式化 SQL 报警。
	return fmt.Sprintf(`CREATE INDEX IF NOT EXISTS %s ON topic_tag_embeddings USING hnsw ((embedding::halfvec(%d)) halfvec_cosine_ops) WHERE embedding_type = 'identity'`, TopicTagIdentityHalfvecIndexName, dim) //nolint:gosec // G201: dim 为已校验的整数，无注入面
}

// vectorColumnDimension 从 format_type 输出解析 pgvector 列维度：
// "vector(2560)" → 2560；"vector"（未限定维度）/空串/非法 → 0（调用方据此
// 跳过建索引）。
func vectorColumnDimension(typeStr string) int {
	s := strings.TrimSpace(typeStr)
	const prefix = "vector("
	if !strings.HasPrefix(s, prefix) || !strings.HasSuffix(s, ")") {
		return 0
	}
	dim, err := strconv.Atoi(strings.TrimSuffix(s[len(prefix):], ")"))
	if err != nil || dim <= 0 {
		return 0
	}
	return dim
}

// VectorColumnType 读取列的实际类型声明（format_type 带 typmod，例如
// "vector" / "vector(2560)"）。列不存在或查询失败时返回错误。
//
// 运行时纠正器（tagmanagement core / auxlabel、topicgraph repository）用它区分
// 「列已是无维度限定的 vector」（目标形态，不再 ALTER）与「列仍带维度限定」
// （迁移过渡期）。
func VectorColumnType(db *gorm.DB, table, column string) (string, error) {
	var typeStr string
	if err := db.Raw(`
SELECT format_type(a.atttypid, a.atttypmod)
FROM pg_attribute a
JOIN pg_class c ON c.oid = a.attrelid
WHERE c.relname = ? AND a.attname = ?`, table, column).Row().Scan(&typeStr); err != nil {
		return "", err
	}
	return typeStr, nil
}

// sampledVectorDimension 从存量数据采样列的实际维度——裸 vector 列的维度来源
// （列类型不再声明维度后，维度只能从数据里读）。
//
// 表名与列名来自包内常量表（vectorDimensionlessColumns），无注入面。
//
// 返回 (dim, mixed, err)：
//   - 表空 / 无非空向量 → (0, false, nil)；
//   - 只有一种维度 → (该维度, false, nil)；
//   - **多种维度并存** → (众数维度, true, nil)：裸列允许混合维度（放宽约束的
//     代价），此时**不能**据此建 halfvec 索引——索引表达式内嵌维度，会把另一
//     维度的写入直接拒掉（实测 "expected N dimensions, not M"）。调用方据此
//     跳过建索引并降级为精确扫描。
func sampledVectorDimension(db *gorm.DB, table, column string) (dim int, mixed bool, err error) {
	type dimRow struct {
		Dim   int
		Count int64
	}
	var rows []dimRow
	stmt := fmt.Sprintf( //nolint:gosec // G201: 表名/列名来自包内常量表，无注入面
		"SELECT vector_dims(%s) AS dim, count(*) FROM %s WHERE %s IS NOT NULL GROUP BY 1 ORDER BY 2 DESC",
		column, table, column)
	if err := db.Raw(stmt).Scan(&rows).Error; err != nil {
		return 0, false, err
	}
	if len(rows) == 0 {
		return 0, false, nil
	}
	if len(rows) > 1 {
		return rows[0].Dim, true, nil
	}
	return rows[0].Dim, false, nil
}

// topicTagEmbeddingColumnDimension 解析 topic_tag_embeddings.embedding 的维度：
//
//   - 列有限定（vector(N)）→ N（沿用 vectorColumnDimension 解析）；
//   - 列无限定（vector）→ 从存量数据采样（SELECT vector_dims(...) GROUP BY）——
//     列类型不再承载维度后，数据是唯一的维度事实源；
//   - 表空（采样不到任何非空向量）→ 0，调用方跳过建索引，等运行时首次写入时
//     由 ensureVectorDimension 按运行时维度建。
//
// mixed = true 表示存量数据里并存多种维度（裸列的放宽语义带来的可能）——调用方
// 不应据此建带维度的索引。
//
// 迁移 20261023_0001 与 20261117_0002 共用本函数（单一真相源）。
func topicTagEmbeddingColumnDimension(db *gorm.DB) (dim int, typeStr string, mixed bool, err error) {
	typeStr, err = VectorColumnType(db, "topic_tag_embeddings", "embedding")
	if err != nil {
		return 0, "", false, err
	}
	if dim = vectorColumnDimension(typeStr); dim > 0 {
		return dim, typeStr, false, nil
	}
	if typeStr != "vector" {
		// 非 vector 类型（不该出现）：不做数据采样，避免把非向量列当向量读。
		return 0, typeStr, false, nil
	}
	sampled, mixed, err := sampledVectorDimension(db, "topic_tag_embeddings", "embedding")
	if err != nil {
		return 0, typeStr, false, err
	}
	return sampled, typeStr, mixed, nil
}

// halfvecExpressionDimension 从索引定义文本解析 "::halfvec(N)" 内嵌的维度。
// 表达式不可解析时返回 0。
func halfvecExpressionDimension(indexDef string) int {
	const marker = "::halfvec("
	i := strings.Index(indexDef, marker)
	if i < 0 {
		return 0
	}
	rest := indexDef[i+len(marker):]
	j := strings.Index(rest, ")")
	if j < 0 {
		return 0
	}
	dim, err := strconv.Atoi(rest[:j])
	if err != nil || dim <= 0 {
		return 0
	}
	return dim
}

// topicTagIdentityHalfvecIndexState 返回既有 halfvec identity 索引表达式内嵌的
// 维度与其有效性（indisvalid）。索引不存在时返回 (0, true)。
func topicTagIdentityHalfvecIndexState(db *gorm.DB) (dim int, valid bool) {
	var def string
	var isValid bool
	if err := db.Raw(`
SELECT pg_get_indexdef(i.indexrelid), i.indisvalid
FROM pg_index i
JOIN pg_class c ON c.oid = i.indexrelid
WHERE c.relname = ? AND i.indrelid = to_regclass('public.topic_tag_embeddings')`,
		TopicTagIdentityHalfvecIndexName).Row().Scan(&def, &isValid); err != nil {
		return 0, true
	}
	return halfvecExpressionDimension(def), isValid
}

// TopicTagIdentityHalfvecIndexState 返回既有 halfvec identity 索引表达式内嵌的
// 维度与其有效性（indisvalid）。索引不存在时返回 (0, true)。
//
// 运行时纠正器用它判断「索引表达式内嵌维度是否与当前运行时维度一致」——不一致
// 必须重建（CREATE INDEX IF NOT EXISTS 不会纠正既有表达式）。
func TopicTagIdentityHalfvecIndexState(db *gorm.DB) (dim int, valid bool) {
	return topicTagIdentityHalfvecIndexState(db)
}

// VectorIndexExists 报告 public schema 下是否存在同名索引。
func VectorIndexExists(db *gorm.DB, indexName string) (bool, error) {
	var exists bool
	if err := db.Raw(`SELECT to_regclass(?) IS NOT NULL`, "public."+indexName).Row().Scan(&exists); err != nil {
		return false, err
	}
	return exists, nil
}

// TopicTagIdentityHalfvecIndexDimension 返回既有 halfvec identity 索引表达式内嵌
// 的维度；索引不存在或表达式不可解析时返回 0。运行时纠正器用它判断「索引表达式
// 内嵌维度是否与当前运行时维度一致」（不一致必须重建——CREATE INDEX IF NOT
// EXISTS 不会纠正既有表达式）。
func TopicTagIdentityHalfvecIndexDimension(db *gorm.DB) int {
	dim, _ := topicTagIdentityHalfvecIndexState(db)
	return dim
}

// EnsureTopicTagIdentityHalfvecIndex 确保 halfvec identity 部分 HNSW 索引存在，
// 且其表达式内嵌维度与 dim 一致：
//
//   - 索引不存在 → 建；
//   - 索引存在但表达式内嵌维度 ≠ dim（**含表达式不可解析**——那就不是本索引，
//     留着会静默挡住 CREATE INDEX IF NOT EXISTS）→ 先 DROP 再建；
//   - 索引 INVALID（历史构建失败残留）→ 先 DROP 再建。
//
// 调用方负责 maintenance_work_mem / max_parallel_maintenance_workers 的设置
// （迁移在事务内用 SET LOCAL，运行期在固定连接上用会话级 SET）。
func EnsureTopicTagIdentityHalfvecIndex(db *gorm.DB, dim int) error {
	exists, err := VectorIndexExists(db, TopicTagIdentityHalfvecIndexName)
	if err != nil {
		return fmt.Errorf("probe halfvec identity index: %w", err)
	}
	if exists {
		existingDim, valid := topicTagIdentityHalfvecIndexState(db)
		if !valid || existingDim != dim {
			if err := db.Exec("DROP INDEX IF EXISTS " + TopicTagIdentityHalfvecIndexName).Error; err != nil { //nolint:gosec // G201: 索引名是包内常量
				return fmt.Errorf("drop halfvec identity index: %w", err)
			}
		}
	}
	if err := db.Exec(TopicTagIdentityHalfvecIndexDDL(dim)).Error; err != nil {
		return fmt.Errorf("create halfvec identity HNSW index (dim %d): %w", dim, err)
	}
	return nil
}

// ensureTopicTagIdentityHalfvecIndexForColumn 按列的实际维度确保 halfvec identity
// 索引存在——迁移 20261023_0001 与 20261117_0002 共用的单一实现（维度解析 +
// 上限判断 + 重建逻辑都只写一遍）。
//
// 维度来源：列有限定 → 解析列类型；列无限定 → 从存量数据采样（表空则跳过，
// 等运行时首次写入时由 ensureVectorDimension 建）。
func ensureTopicTagIdentityHalfvecIndexForColumn(db *gorm.DB, migrationTag string) error {
	dim, typeStr, mixed, err := topicTagEmbeddingColumnDimension(db)
	if err != nil {
		return fmt.Errorf("read topic_tag_embeddings.embedding type: %w", err)
	}
	if mixed {
		// 裸列里并存多种维度：不能建带维度的索引——索引表达式会把另一维度的
		// 写入直接拒掉（实测 "expected N dimensions, not M"）。宁可精确扫描。
		//
		// 注意这里是**能力受限**不是单纯变慢：精确扫描同样要跨行比较向量，
		// 混维的存量行会让比较报 different vector dimensions，检索在清理掉
		// 少数派维度之前是不可用的。日志要如实说明，避免运维误以为只是慢。
		logging.Warnf("Migration %s: skip halfvec HNSW index (embedding type %q holds vectors of multiple dimensions; cross-dimension comparisons will error until the minority-dimension rows are cleaned), semantic search degraded", migrationTag, typeStr)
		return nil
	}
	if dim <= 0 || dim > 4000 {
		logging.Infof("Migration %s: skip halfvec HNSW index (embedding type %q, dim %d; halfvec supports 1..4000), semantic search falls back to exact scan", migrationTag, typeStr, dim)
		return nil
	}

	// 构建期维护内存：128MB 默认下 14k×2560 行会落盘（实测 64MB=73s，
	// 256MB=29s）。SET LOCAL 随事务结束复位，不污染连接池。
	if err := db.Exec(`SET LOCAL maintenance_work_mem = '256MB'`).Error; err != nil {
		return fmt.Errorf("set maintenance_work_mem for halfvec index build: %w", err)
	}
	// 禁用并行维护：Docker 默认 /dev/shm 只有 64MB，HNSW 并行构建会报
	// "could not resize shared memory segment ... No space left on device"
	// （真库首次迁移实测，SQLSTATE 53100）。单进程构建只吃
	// maintenance_work_mem，不依赖 /dev/shm。
	if err := db.Exec(`SET LOCAL max_parallel_maintenance_workers = 0`).Error; err != nil {
		return fmt.Errorf("disable parallel maintenance for halfvec index build: %w", err)
	}
	return EnsureTopicTagIdentityHalfvecIndex(db, dim)
}

// searchSemanticHalfvecIndexMigration implements 20261023_0001
// (optimize-search-semantic-index design D1/D5): build the halfvec identity
// partial HNSW index that lets semantic tag recall take ANN candidates instead
// of a full-table cosine scan (真库实测：14,658 行全表扫描 3.74s，索引 top-50
// 查询 15ms)。维度是运行时属性（当前 2560），不是编译期常量：列带维度限定
// 时从列 format_type 解析，列为无限定 vector 时从存量数据采样（表空则跳过，
// 等运行时首次写入时由 ensureVectorDimension 建）。halfvec 上限 4000：超限跳过
// 并记日志（语义路回退旧精确扫描，行为不中断）。
//
// 构建在事务内（默认 in-tx 路径）：SET LOCAL maintenance_work_mem 生效，
// 失败整体回滚、版本不记录、下次启动重试。同名索引存在但 INVALID（历史
// 构建中断）或表达式内嵌维度与当前维度不符时先 DROP 再建，否则 IF NOT EXISTS
// 会把坏索引/旧维度索引当成已完成。
func searchSemanticHalfvecIndexMigration() Migration {
	return Migration{
		Version:     "20261023_0001",
		Description: "optimize-search-semantic-index D1/D5: halfvec identity partial HNSW index on topic_tag_embeddings (dimension from the column type, or sampled from stored data when the column is dimensionless; skipped above halfvec's 4000-dim limit)",
		Up: func(db *gorm.DB) error {
			if !tableExists(db, "topic_tag_embeddings") {
				return nil
			}
			return ensureTopicTagIdentityHalfvecIndexForColumn(db, "20261023_0001")
		},
	}
}

// embeddingModelNameTables 列出存 embedding 模型名的表——模型名取自模型注册表
// ai_models.name。这五张表同病同源（注册表能放下 ⇒ 落库处必须放得下，
// fix-embedding-model-column-width D1/D2），一次全修不分批。
var embeddingModelNameTables = []string{
	"topic_tag_embeddings",
	"candidate_embeddings",
	"preference_vectors",
	"route_embeddings",
	"discovery_interest_entries",
}

// embeddingModelNameColumnWidth 是 embedding 模型名列的目标宽度，与
// ai_models.name（VARCHAR(100)）等宽——取注册表的实际列宽而非另取一个
// 「够用」的数字，好让不变式可机械断言（D1）。
const embeddingModelNameColumnWidth = 100

// embeddingModelColumnWidthMigration implements 20261103_0001
// (fix-embedding-model-column-width D1/D4): 把五张 embedding 表的 model 列
// 从 VARCHAR(50) 加宽到 VARCHAR(100)，与模型注册表 ai_models.name 对齐。
// 症状：本地 llama.cpp embedding 模型注册时「名字」是 GGUF 文件绝对路径
// （61 字符），超限 11 字符 → SQLSTATE 22001 → 落库被拒 → embedding 队列
// 批量失败（缓存命中时返回的 cached.Model 同值，同样炸）。
//
// 加宽是同类型 typmod 变更：不截断数据、不重写表体、存量向量零改动；
// 含 model 的唯一索引（idx_candidate_embedding_uniq）随列宽重建。
//
// 幂等口径：列宽已达目标（或更宽）时**完全不动**——不 ALTER、不取锁、
// 不重建索引。PG 实测：对已经是 varchar(100) 的列再执行
// ALTER COLUMN model TYPE VARCHAR(100)，表体不重写（relfilenode 不变），
// 但该列上的索引会被重建（索引 oid 变化），还白拿一次 ACCESS EXCLUSIVE
// 锁——生产 Phase 1 的 AutoMigrate 已经先加宽过，这一步是纯浪费。
// 跳过条件：表不存在（域模型未注册的部署）/ 列不存在 / 列无长度上限
// （text 等）/ 列宽 ≥ 目标宽度。
func embeddingModelColumnWidthMigration() Migration {
	return Migration{
		Version:     "20261103_0001",
		Description: "fix-embedding-model-column-width: widen embedding model name columns to VARCHAR(100) to match ai_models.name (skips missing table/column, unlimited-width columns, and columns already at/beyond the target width — no ALTER, no lock, no index rebuild in that case)",
		Up: func(db *gorm.DB) error {
			// 先做便宜的 catalog 判定，只有真需要加宽的窄列才进 DDL 分支。
			for _, table := range embeddingModelNameTables {
				if !tableExists(db, table) {
					continue // 域模型未注册的部署：跳过该表
				}
				width, ok := columnMaxLength(db, table, "model")
				if !ok {
					continue // 列不存在或无长度上限（text）：无从加宽
				}
				if width >= embeddingModelNameColumnWidth {
					continue // 已是目标宽度或更宽：真 no-op
				}
				// ALTER COLUMN TYPE 拿 AccessExclusiveLock——用 lock_timeout 守卫，
				// 抢不到锁就快速失败而不是无限阻塞写入（code-style 迁移规范）。
				if err := withLockTimeout(db, "5s", func(tx *gorm.DB) error {
					stmt := fmt.Sprintf("ALTER TABLE %s ALTER COLUMN model TYPE VARCHAR(%d)", table, embeddingModelNameColumnWidth)
					if err := tx.Exec(stmt).Error; err != nil {
						return fmt.Errorf("widen %s.model to VARCHAR(%d): %w", table, embeddingModelNameColumnWidth, err)
					}
					return nil
				}); err != nil {
					return err
				}
			}
			return nil
		},
	}
}

// embeddingModelNotNullMigration implements 20261103_0002
// (fix-embedding-model-column-width 回归修复): 恢复 topic_tag_embeddings.model
// 的 NOT NULL 约束——本 change 的上一版实现把它弄丢了。
//
// 事故链（已实证）：20260723_0001 用 ensureNotNullDefault 把
// topic_tag_embeddings.model 固化为 NOT NULL（{"topic_tag_embeddings",
// "model", "", true}）。本 change 把 gorm tag 从 size:50 改成 size:100 后，
// Phase 1 的 GORM AutoMigrate 判定列宽不符 → 发出 ALTER COLUMN；
// gorm.io/driver/postgres v1.5.11 的 AlterColumn 在 field.NotNull == false
// （本仓库约定「not null 不写进 model tag」）时执行
// ALTER COLUMN ... DROP NOT NULL → 把 20260723_0001 固化的约束静默丢弃。
// 结果：生产库该列变成可空，而仓库自己的 constraints_test 断言它是 notNull
// （该断言跑在全新 testcontainer 上，测不到生产这条路径）。
//
// 为什么必须是**第二个**迁移版本而不是改 0001 的 body：0001 已经在生产
// 记录过，改 body 不会在生产重跑。
// 为什么不给 gorm tag 加 not null：本仓库约定「not null 不写进 model tag、
// 由显式迁移固化」（见 20260723_0001 的 Description），加 tag 会让
// AutoMigrate 与显式迁移重新竞争。
// 其余四张表（candidate_embeddings / preference_vectors / route_embeddings
// / discovery_interest_entries）的 model 列本来就是可空，不动它们的空值性。
//
// 幂等：ensureNotNullDefault 只在列当前可空时回填 + SET NOT NULL；生产库
// 当前 NULL 行数为 0，回填是空操作。
func embeddingModelNotNullMigration() Migration {
	return Migration{
		Version:     "20261103_0002",
		Description: "fix-embedding-model-column-width: restore NOT NULL on topic_tag_embeddings.model (silently dropped by AutoMigrate's ALTER COLUMN when the gorm tag width changed 50->100; NOT NULL stays owned by explicit migrations, never by model tags)",
		Up: func(db *gorm.DB) error {
			if !tableExists(db, "topic_tag_embeddings") {
				return nil // 域模型未注册的部署：跳过
			}
			if !columnExists(db, "topic_tag_embeddings", "model") {
				return nil // 列不存在：无从恢复
			}
			// SET NOT NULL 拿 AccessExclusiveLock 并全表扫描校验——用
			// lock_timeout 守卫（code-style 迁移规范）。
			return withLockTimeout(db, "5s", func(tx *gorm.DB) error {
				return ensureNotNullDefault(tx, "topic_tag_embeddings", "model", "''")
			})
		},
	}
}

// storyHeatTablesMigration implements 20261105_0001
// (add-event-story-heat design §6.1)：事件线与热度证据两张新表 +
// daily_report_threads 三列。
//
// 建表/加列/索引全部幂等（IF NOT EXISTS），无表重写。标准路径上两张新表
// 已由 Phase 1 AutoMigrate 从注册模型建出（无 CHECK、无 FK、无部分索引），
// 因此 CHECK 除了写在 CREATE TABLE 内（裸重放路径自含），还用 DO 块守卫
// 重复确保；FK 统一由本迁移所有（AutoMigrate 跑在
// DisableForeignKeyConstraintWhenMigrating 下不建 FK，同 20260910_0001 /
// 20261012_0001 的「迁移所有 FK」政策），被引用表缺席时跳过。
//
// embedding / subject_embedding 刻意写**无维度限定的 vector**：维度由运行时
// embedding 模型决定，不属于 schema 契约（同 20260403_0003 口径；写
// vector(N) 会与 unify-vector-column-dimension 的去限定迁移冲突）。
//
// board FK 指向 semantic_labels(id)：boards are SemanticLabel rows
// (label_type='board')，design §6.1 的 semantic_boards 是业务名。
func storyHeatTablesMigration() Migration {
	return Migration{
		Version:     "20261105_0001",
		Description: "add-event-story-heat: create board_stories (story identity) + story_signals (heat evidence, composite PK) + daily_report_threads story_id/scope/is_development/story_manual/display_tier (scope/is_development/story_manual/display_tier NOT NULL pinned via ensureNotNullDefault for the AutoMigrate-first path); recall/subject/time indexes; CHECKs & FKs re-ensured idempotently (embedding columns dimension-less vector)",
		Up: func(db *gorm.DB) error {
			// ── board_stories（事件线）──────────────────────────────
			// CHECK 写在 CREATE TABLE 内；FK 另用 DO 块确保（见上）。
			if err := db.Exec(`CREATE TABLE IF NOT EXISTS board_stories (
				id                bigserial PRIMARY KEY,
				board_id          bigint NOT NULL,
				lane_id           bigint,
				title             text NOT NULL,
				subject           text,
				subject_key       text,
				action            text,
				object            text,
				occurred_at       timestamptz,
				status            text NOT NULL DEFAULT 'active'
				                  CHECK (status IN ('active','settled','merged')),
				first_seen_date   date NOT NULL,
				last_seen_date    date NOT NULL,
				thread_count      integer NOT NULL DEFAULT 0,
				article_count     integer NOT NULL DEFAULT 0,
				participant_count integer NOT NULL DEFAULT 0,
				embedding         vector,
				subject_embedding vector,
				merged_into       bigint,
				origin            text NOT NULL DEFAULT 'rule'
				                  CHECK (origin IN ('rule','model','manual')),
				created_at        timestamptz NOT NULL DEFAULT now(),
				updated_at        timestamptz NOT NULL DEFAULT now()
			)`).Error; err != nil {
				return fmt.Errorf("create board_stories: %w", err)
			}

			// ── story_signals（热度证据）────────────────────────────
			// 复合主键 (story_id, article_id)：同一篇文章对同一条事件线只计
			// 一次证据；身份（事件归属）与热度分离，去重只作用于身份层。
			if err := db.Exec(`CREATE TABLE IF NOT EXISTS story_signals (
				story_id           bigint NOT NULL,
				article_id         bigint NOT NULL,
				feed_id            bigint NOT NULL,
				participant_key    text NOT NULL,
				kind               text NOT NULL DEFAULT 'editorial'
				                   CHECK (kind IN ('editorial','signal')),
				observed_at        timestamptz NOT NULL,
				observed_at_source text NOT NULL DEFAULT 'pub_date'
				                   CHECK (observed_at_source IN ('pub_date','fallback')),
				PRIMARY KEY (story_id, article_id)
			)`).Error; err != nil {
				return fmt.Errorf("create story_signals: %w", err)
			}

			// ── daily_report_threads 五列（存量事件挂事件线 + 展示层）──
			// display_tier：跨期去重的展示层载体（tasks 6.1-6.3）——只改
			// 展示，身份/signals/热度永不读它；存量行落默认 body（未降级）。
			// 存量行不回填：story_id 保持 NULL（未归属事件降级为「当日独立
			// 事件」，不阻塞现有功能）；scope/is_development/story_manual 按默认值生效。
			// story_manual：人工「独立事件」标记（event-story spec 人工拆分后不
			// 回滚）——回填与自动判定遇到 true 必须跳过该事件。
			if tableExists(db, "daily_report_threads") {
				if err := db.Exec(`ALTER TABLE daily_report_threads
					ADD COLUMN IF NOT EXISTS story_id bigint,
					ADD COLUMN IF NOT EXISTS scope text NOT NULL DEFAULT 'single',
					ADD COLUMN IF NOT EXISTS is_development boolean NOT NULL DEFAULT false,
					ADD COLUMN IF NOT EXISTS story_manual boolean NOT NULL DEFAULT false,
					ADD COLUMN IF NOT EXISTS display_tier text NOT NULL DEFAULT 'body'`).Error; err != nil {
					return fmt.Errorf("add daily_report_threads story columns: %w", err)
				}
				// 正常启动路径上 RunAutoMigrate 已按模型建出这几列（可空带默认），
				// ADD COLUMN IF NOT EXISTS 是 no-op——NOT NULL 必须在这里固化
				// （幂等：已 NOT NULL 时 ensureNotNullDefault 直接跳过；回填值与
				// 列默认一致，存量行不改变语义）。
				if err := withLockTimeout(db, "5s", func(tx *gorm.DB) error {
					for _, col := range []struct{ name, def string }{
						{"scope", "'single'"},
						{"is_development", "false"},
						{"story_manual", "false"},
						{"display_tier", "'body'"},
					} {
						if err := ensureNotNullDefault(tx, "daily_report_threads", col.name, col.def); err != nil {
							return fmt.Errorf("pin daily_report_threads.%s NOT NULL: %w", col.name, err)
						}
					}
					return nil
				}); err != nil {
					return err
				}
			}

			// ── 索引 ─────────────────────────────────────────────
			// recall 部分索引：只覆盖可召回的存活事件线（归属判定的 14 天窗口
			// 扫描面）；subject 索引支撑主体归一化第一层（subject_key 精确匹配）。
			indexes := []string{
				`CREATE INDEX IF NOT EXISTS board_stories_recall_idx
					ON board_stories (board_id, last_seen_date DESC)
					WHERE merged_into IS NULL AND status = 'active'`,
				`CREATE INDEX IF NOT EXISTS board_stories_subject_idx ON board_stories (board_id, subject_key)`,
				`CREATE INDEX IF NOT EXISTS story_signals_time_idx ON story_signals (observed_at DESC)`,
				`CREATE INDEX IF NOT EXISTS story_signals_story_idx ON story_signals (story_id, participant_key, observed_at DESC)`,
			}
			for _, stmt := range indexes {
				if err := db.Exec(stmt).Error; err != nil {
					return fmt.Errorf("story heat index: %w", err)
				}
			}
			// daily_report_threads 由 topicgraph 模型 AutoMigrate 注册创建；不
			// import topicgraph 的测试二进制（如 tagmanagement/service/core 的
			// golden schema）里该表不存在，索引需同款 tableExists 守卫（与上方
			// 旧 daily_report_threads 索引迁移惯例一致）。
			if tableExists(db, "daily_report_threads") {
				if err := db.Exec(`CREATE INDEX IF NOT EXISTS daily_report_threads_story_idx ON daily_report_threads (story_id)`).Error; err != nil {
					return fmt.Errorf("story heat index: %w", err)
				}
			}

			// ── CHECK 确保块（AutoMigrate 先建裸表时补约束）────────────
			// DO 块先查后加（幂等且不产生事务中止）；列不存在时跳过——
			// 表缺席/列缺席的部署无从校验。
			checks := []struct{ name, table, column, expr string }{
				{"chk_board_stories_status", "board_stories", "status", "CHECK (status IN ('active','settled','merged'))"},
				{"chk_board_stories_origin", "board_stories", "origin", "CHECK (origin IN ('rule','model','manual'))"},
				{"chk_story_signals_kind", "story_signals", "kind", "CHECK (kind IN ('editorial','signal'))"},
				{"chk_story_signals_observed_at_source", "story_signals", "observed_at_source", "CHECK (observed_at_source IN ('pub_date','fallback'))"},
				{"chk_daily_report_threads_scope", "daily_report_threads", "scope", "CHECK (scope IN ('single','composite'))"},
				{"chk_daily_report_threads_display_tier", "daily_report_threads", "display_tier", "CHECK (display_tier IN ('body','flash','hidden'))"},
			}
			for _, c := range checks {
				if !tableExists(db, c.table) || !columnExists(db, c.table, c.column) {
					continue
				}
				if err := withLockTimeout(db, "5s", func(tx *gorm.DB) error {
					if err := tx.Exec(fmt.Sprintf(`DO $$ BEGIN
						IF NOT EXISTS (
							SELECT 1 FROM information_schema.table_constraints
							WHERE constraint_name = '%s' AND table_name = '%s'
						) THEN
							ALTER TABLE %s ADD CONSTRAINT %s %s;
						END IF;
					END $$`, c.name, c.table, c.table, c.name, c.expr)).Error; err != nil { //nolint:gosec // G201: 约束名/表达式是包内常量表
						return fmt.Errorf("ensure %s: %w", c.name, err)
					}
					return nil
				}); err != nil {
					return err
				}
			}

			// ── FK 确保块（迁移所有 FK；被引用表缺席时跳过）──────────
			// story_id 用 SET NULL 而非 CASCADE：事件线被删除/合并时事件保
			// 留、退回未归属状态（spec：SHALL NOT 外键级联删除）。
			fks := []struct{ name, table, column, ref, del string }{
				{"fk_board_stories_board", "board_stories", "board_id", "semantic_labels(id)", "CASCADE"},
				{"fk_board_stories_lane", "board_stories", "lane_id", "board_persistent_topics(id)", "SET NULL"},
				{"fk_board_stories_merged_into", "board_stories", "merged_into", "board_stories(id)", "NO ACTION"},
				{"fk_story_signals_story", "story_signals", "story_id", "board_stories(id)", "CASCADE"},
				{"fk_story_signals_article", "story_signals", "article_id", "articles(id)", "CASCADE"},
				{"fk_daily_report_threads_story", "daily_report_threads", "story_id", "board_stories(id)", "SET NULL"},
			}
			for _, fk := range fks {
				if !tableExists(db, fk.table) {
					continue
				}
				refTable := strings.TrimSuffix(fk.ref, "(id)")
				if !tableExists(db, refTable) {
					continue // 最小部署（域模型未注册）：跳过该 FK
				}
				if err := withLockTimeout(db, "5s", func(tx *gorm.DB) error {
					if err := tx.Exec(fmt.Sprintf(`DO $$ BEGIN
						IF NOT EXISTS (
							SELECT 1 FROM information_schema.table_constraints
							WHERE constraint_name = '%s' AND table_name = '%s'
						) THEN
							ALTER TABLE %s ADD CONSTRAINT %s FOREIGN KEY (%s) REFERENCES %s ON DELETE %s;
						END IF;
					END $$`, fk.name, fk.table, fk.table, fk.name, fk.column, fk.ref, fk.del)).Error; err != nil { //nolint:gosec // G201: 约束名/列名是包内常量表
						return fmt.Errorf("add %s: %w", fk.name, err)
					}
					return nil
				}); err != nil {
					return err
				}
			}
			return nil
		},
	}
}

// storySignalContentKeyMigration implements 20261110_0001
// (fix-duplicate-article-heat design D2/D5)：story_signals 落「内容身份」列。
//
// 同一篇内容被两个 feed 各抓一份会存两行 articles（articles 唯一键是
// (feed_id, link)，跨 feed 不去重），而 story_signals 的 participant_key 按
// feed 计参与者 → 一篇内容被算成 2 个参与者、热度虚高。content_key 存规范化
// 后的链接（内容身份键），供下一步统一去重键使用：content_key 非空即按内容
// 去重，空串（内容身份不可判定）时回退 participant_key。
//
// 列可空（存量行不改语义：空/NULL 都表示「内容身份待回填」，消费者退化为
// participant_key）；**本迁移不做数据回填**——回填走
// cmd/backfill-stories --backfill-content-keys（分批、可重入）；迁移里做
// 全量回填会在树莓派上变成长事务、且失败不可重试（design D5）。
//
// 幂等：ADD COLUMN IF NOT EXISTS + CREATE INDEX IF NOT EXISTS，可重入。
func storySignalContentKeyMigration() Migration {
	return Migration{
		Version:     "20261110_0001",
		Description: "fix-duplicate-article-heat: add story_signals.content_key (nullable text, normalized article link = content identity) + (story_id, content_key) index; no data backfill (cmd/backfill-stories --backfill-content-keys owns it)",
		Up: func(db *gorm.DB) error {
			// 表缺席时跳过（最小部署：域模型未注册，story_signals 不存在）。
			if !tableExists(db, "story_signals") {
				return nil
			}
			if err := db.Exec(`ALTER TABLE story_signals ADD COLUMN IF NOT EXISTS content_key text`).Error; err != nil {
				return fmt.Errorf("add story_signals.content_key: %w", err)
			}
			if err := db.Exec(`CREATE INDEX IF NOT EXISTS story_signals_story_content_idx
				ON story_signals (story_id, content_key)`).Error; err != nil {
				return fmt.Errorf("create story_signals_story_content_idx: %w", err)
			}
			return nil
		},
	}
}

// vectorDimensionlessColumns 列出要统一为「无维度限定 vector」的向量列。
//
// 维度是**运行时属性**（由 embedding 模型的实际输出决定，逐行存于 Dimension
// 字段），不是 schema 契约。带维度限定的列会逼出两类结构性问题：换模型就要
// ALTER 全表重写；model 声明与实际 schema 长期对不上（14 处 model 声明里 13 处
// 是无限定 `type:vector`，本 change 已把唯一的例外（margin_notes 的带维度限定
// 声明）也归一为无限定形态）。
//
// float32Indexes 是该列上可能存在的 float32 HNSW 索引：裸 vector 列**建不了**
// float32 索引（pgvector 报 "column does not have dimensions"），且存活下来的
// float32 索引会把列钉死在旧维度上——新维度向量写入直接被拒（"different vector
// dimensions"）。所以统一成裸 vector 之前先删掉它们（当前语义召回走 halfvec
// 表达式索引，float32 索引没有任何查询在用）。
var vectorDimensionlessColumns = []struct {
	table          string
	column         string
	float32Indexes []string
}{
	{
		table:          "topic_tag_embeddings",
		column:         "embedding",
		float32Indexes: []string{"idx_topic_tag_embeddings_embedding"},
	},
	{
		table:          "daily_report_sections",
		column:         "embedding",
		float32Indexes: []string{"idx_daily_report_sections_embedding"},
	},
	{
		table:  "semantic_labels",
		column: "embedding",
	},
	{
		table:  "semantic_labels",
		column: "merge_embedding",
	},
	{
		table:  "term_notes",
		column: "embedding",
	},
	{
		table:          "board_persistent_topics",
		column:         "embedding",
		float32Indexes: []string{"idx_board_persistent_topics_embedding"},
	},
}

// unifyVectorColumnDimensionMigration implements 20261117_0001
// (unify-vector-column-dimension design D1/D2)：把存量向量列从「带维度限定」
// 统一为「无维度限定的 vector」。
//
// 语义等价于放宽约束（去掉 typmod），不丢数据：Postgres 对同 base type 的
// typmod 变化走 binary coercion，不重写表体（真库实测：relfilenode 不变、
// 存量 vector_dims 不变——见 docs/reference/database/RESTORE.md §8 附录）。
// 与「加 typmod」（vector → vector(N)）方向相反：那条路需要 cast，pgvector 无
// 跨维度 cast，会在有数据的库上直接失败——正是 2026-11-04 灾备恢复失败的根因。
//
// 幂等：列已是无限定 vector 时直接跳过（不取锁、不 ALTER）。表/列不存在时跳过
// （域模型未注册的最小部署）。每张表用 withLockTimeout 守卫——ALTER COLUMN TYPE
// 拿 AccessExclusiveLock，抢不到锁就快速失败而不是无限阻塞写入。
//
// 顺带清理失败残留：tmp_hnsw_test 是建在 2560 维列上的 float32 HNSW 索引，
// 超过 pgvector float32 HNSW 的 2000 维硬上限 → 创建失败留下 invalid 残骸。
// 只删 invalid 的同名索引（万一是有效索引则保留，不误删用户索引）。
func unifyVectorColumnDimensionMigration() Migration {
	return Migration{
		Version:     "20261117_0001",
		Description: "unify-vector-column-dimension: unify stored vector columns (topic_tag_embeddings.embedding / daily_report_sections.embedding / semantic_labels.embedding+merge_embedding / term_notes.embedding / board_persistent_topics.embedding) to dimensionless vector (dimension is a runtime property, not a schema contract; idempotent, lock-timeout guarded) and drop the INVALID tmp_hnsw_test index leftover",
		Up: func(db *gorm.DB) error {
			for _, col := range vectorDimensionlessColumns {
				if !tableExists(db, col.table) {
					continue // 域模型未注册的部署：跳过该表
				}
				if !columnExists(db, col.table, col.column) {
					continue // 列不存在（老库尚未加列）：无从统一
				}
				typeStr, err := VectorColumnType(db, col.table, col.column)
				if err != nil {
					return fmt.Errorf("read %s.%s type: %w", col.table, col.column, err)
				}
				if typeStr == "vector" {
					continue // 幂等：已是无维度限定，跳过（不取锁、不 ALTER）
				}
				logging.Infof("Migration 20261117_0001: unifying %s.%s from %s to dimensionless vector", col.table, col.column, typeStr)
				if err := withLockTimeout(db, "5s", func(tx *gorm.DB) error {
					for _, indexName := range col.float32Indexes {
						// 裸 vector 列建不了 float32 HNSW 索引，且存活的 float32
						// 索引会把列钉在旧维度上——先删再 ALTER。
						if err := tx.Exec("DROP INDEX IF EXISTS " + indexName).Error; err != nil { //nolint:gosec // G201: 索引名是包内常量
							return fmt.Errorf("drop %s: %w", indexName, err)
						}
					}
					stmt := fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s TYPE vector", col.table, col.column) //nolint:gosec // G201: 表名/列名来自包内常量表
					if err := tx.Exec(stmt).Error; err != nil {
						return fmt.Errorf("unify %s.%s to dimensionless vector: %w", col.table, col.column, err)
					}
					return nil
				}); err != nil {
					return err
				}
			}
			return dropInvalidVectorIndexLeftovers(db, "20261117_0001")
		},
	}
}

// dropInvalidVectorIndexLeftovers 删除创建失败残留的 invalid 向量索引。
// 只删 invalid 的——有效索引一律保留（不误删用户索引）。IF EXISTS 保证幂等：
// 目标索引不存在时重复执行不报错。限定 public schema（与 VectorIndexExists 同
// 口径），避免跨 schema 同名索引被误判/误删。
func dropInvalidVectorIndexLeftovers(db *gorm.DB, migrationTag string) error {
	const leftover = "tmp_hnsw_test"
	var invalid bool
	if err := db.Raw(`
SELECT EXISTS (
	SELECT 1 FROM pg_index i
	JOIN pg_class c ON c.oid = i.indexrelid
	JOIN pg_namespace n ON n.oid = c.relnamespace
	WHERE c.relname = ? AND n.nspname = 'public' AND NOT i.indisvalid
)`, leftover).Row().Scan(&invalid); err != nil {
		return fmt.Errorf("probe %s validity: %w", leftover, err)
	}
	if !invalid {
		return nil
	}
	logging.Warnf("Migration %s: dropping INVALID vector index leftover %s", migrationTag, leftover)
	if err := db.Exec("DROP INDEX IF EXISTS public." + leftover).Error; err != nil { //nolint:gosec // G201: 索引名是包内常量
		return fmt.Errorf("drop %s: %w", leftover, err)
	}
	return nil
}

// ensureTopicTagHalfvecIndexMigration implements 20261117_0002
// (unify-vector-column-dimension 缺口 A)：列已是裸 vector 时补建 halfvec 索引。
//
// 为什么需要独立版本：20261023_0001 已在生产记录过版本，不会再跑；而它当初的
// 维度来源只有「列 format_type」——列被统一成无限定 vector 后解析不到维度，
// 索引就永远建不出来。20261117_0001（列统一）跑完之后，维度只能从数据采样：
// SELECT vector_dims(embedding) ... LIMIT 1。
//
// 表空（没有任何非空向量）时采样不到维度 → 跳过并记日志，等运行时首次写入时
// 由 ensureVectorDimension 按运行时维度建（不猜维度）。
//
// 与 20261023_0001 共用 ensureTopicTagIdentityHalfvecIndexForColumn（同一份
// 维度解析 + 上限判断 + 重建逻辑），不复制粘贴。
func ensureTopicTagHalfvecIndexMigration() Migration {
	return Migration{
		Version:     "20261117_0002",
		Description: "unify-vector-column-dimension: build/rebuild the halfvec identity partial HNSW index for a dimensionless topic_tag_embeddings.embedding column (dimension sampled from stored data; skipped on an empty table — the runtime ensureVectorDimension builds it on the first write)",
		Up: func(db *gorm.DB) error {
			if !tableExists(db, "topic_tag_embeddings") {
				return nil
			}
			return ensureTopicTagIdentityHalfvecIndexForColumn(db, "20261117_0002")
		},
	}
}
