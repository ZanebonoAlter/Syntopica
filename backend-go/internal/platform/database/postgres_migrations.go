package database

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"syntopica-backend/internal/models"
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
			Description: "Staged rollout: set topic_tag_embeddings.embedding column type to vector(4096); runtime dimension configured via embedding_config JSON (embedding_dimension).",
			Up: func(db *gorm.DB) error {
				if err := db.Exec("ALTER TABLE topic_tag_embeddings ADD COLUMN IF NOT EXISTS embedding vector(4096)").Error; err != nil {
					return fmt.Errorf("add topic_tag_embeddings.embedding column: %w", err)
				}
				// ALTER COLUMN TYPE rewrites the whole table under an
				// AccessExclusiveLock — guard with lock_timeout so a large table
				// fails fast instead of blocking writers indefinitely.
				if err := withLockTimeout(db, "5s", func(tx *gorm.DB) error {
					if err := tx.Exec("ALTER TABLE topic_tag_embeddings ALTER COLUMN embedding TYPE vector(4096)").Error; err != nil {
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
				defaults := []models.EmbeddingConfig{
					{Key: "high_similarity_threshold", Value: "0.97", Description: "Auto-reuse existing tag if similarity >= this value"},
					{Key: "low_similarity_threshold", Value: "0.78", Description: "Auto-create new tag if similarity < this value"},
					{Key: "embedding_model", Value: "", Description: "Override embedding model name (empty = read from provider)"},
					{Key: "embedding_dimension", Value: "1024", Description: "Embedding vector dimension"},
				}
				for _, d := range defaults {
					var existing models.EmbeddingConfig
					if err := db.Where("key = ?", d.Key).First(&existing).Error; err != nil {
						if err := db.Create(&d).Error; err != nil {
							logging.Warnf("Warning: failed to seed embedding_config key %s: %v", d.Key, err)
						}
					}
				}
				return nil
			},
		},
		{
			Version:     "20260514_0002",
			Description: "Seed event clustering config keys into embedding_config.",
			Up: func(db *gorm.DB) error {
				defaults := []models.EmbeddingConfig{
					{Key: "event_cluster_kw_min_overlap", Value: "2", Description: "Minimum shared keyword count for Stage 1 event tag keyword-overlap clustering"},
					{Key: "event_cluster_sem_threshold", Value: "0.80", Description: "Minimum semantic cosine similarity for Stage 2 event tag clustering filter"},
				}
				for _, d := range defaults {
					var existing models.EmbeddingConfig
					if err := db.Where("key = ?", d.Key).First(&existing).Error; err != nil {
						if err := db.Create(&d).Error; err != nil {
							logging.Warnf("Warning: failed to seed embedding_config key %s: %v", d.Key, err)
						}
					}
				}
				return nil
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
			Up:          runAuxLabelDupMerge,
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
				constrain := func(table, column, defaultLit string, notNull bool) error {
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
					{"topic_tag_board_labels", "downgraded", "false", true},
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
	return append(migrations, legacyDiscoverNewPendingDismissMigration())
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
				statements := []string{
					`ALTER TABLE cross_board_relations ADD CONSTRAINT ck_cross_board_relations_status CHECK (status IN ('unresolved','proposed','confirmed','dismissed','expired'))`,
					`ALTER TABLE cross_board_relations ADD CONSTRAINT ck_cross_board_relations_type CHECK (relation_type IN ('causal','common_driver','divergence','correlated','contextual','unclear'))`,
					`ALTER TABLE cross_board_relations ADD CONSTRAINT ck_cross_board_relations_verdict CHECK (verification_verdict IN ('supported','contested','insufficient','rejected'))`,
					`ALTER TABLE cross_board_relations ADD CONSTRAINT ck_cross_board_relations_grade CHECK (quality_grade IN ('high','medium','low'))`,
					`ALTER TABLE cross_board_relation_runs ADD CONSTRAINT ck_cross_board_relation_runs_status CHECK (status IN ('queued','running','succeeded','partial','failed'))`,
					`ALTER TABLE cross_board_relation_runs ADD CONSTRAINT ck_cross_board_relation_runs_trigger CHECK (trigger_kind IN ('manual','auto'))`,
					// Idempotent open-row uniqueness: one pending suggestion per hash.
					`CREATE UNIQUE INDEX IF NOT EXISTS uq_cross_board_relations_open ON cross_board_relations (suggestion_hash) WHERE status IN ('unresolved','proposed')`,
					`CREATE INDEX IF NOT EXISTS idx_cross_board_relations_confirmed_active ON cross_board_relations (source_board_id, target_board_id, quality_grade DESC, confirmed_at DESC) WHERE status = 'confirmed'`,
				}
				for _, stmt := range statements {
					// CREATE INDEX IF NOT EXISTS is idempotent; ADD CONSTRAINT is not,
					// so drop-then-add keeps re-runs safe (constraint content is frozen).
					if err := tx.Exec(stmt).Error; err != nil {
						if strings.Contains(err.Error(), "already exists") {
							// tolerate re-run on a constraint that already exists
							continue
						}
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

// runAuxLabelDupMerge performs a one-shot deduplication of active auxiliary
// labels whose normalizeKey is identical (text-variant duplicates like
// "SK 海力士" / "SK海力士").
//
// For each group: the label with the highest ref_count (ties broken by smallest
// id) is the primary; all others are merged into it by moving aliases,
// topic_tag_semantic_labels, and board_composition references, then disabling
// the source. This mirrors the runtime MergeAuxiliaryLabelAlias semantics.
//
// Idempotent: after a successful run no group will have count>1 (disabled
// labels are excluded from the grouping query).
func runAuxLabelDupMerge(db *gorm.DB) error {
	// Query active auxiliary labels.
	type auxRow struct {
		ID       uint
		Label    string
		RefCount int
	}
	var allRows []auxRow
	if err := db.Model(&models.SemanticLabel{}).
		Select("id, label, ref_count").
		Where("label_type = ? AND status = ?", "auxiliary", "active").
		Order("id ASC").
		Find(&allRows).Error; err != nil {
		return fmt.Errorf("dup-merge: query active auxiliary labels: %w", err)
	}

	// Group by normalizeKey.
	groups := make(map[string][]auxRow)
	for _, r := range allRows {
		nk := textutil.NormalizeLabelKey(r.Label)
		groups[nk] = append(groups[nk], r)
	}

	var mergeCount int
	for nk, group := range groups {
		if len(group) < 2 {
			continue
		}

		// Primary = highest ref_count, ties broken by smallest id.
		primary := group[0]
		for i := 1; i < len(group); i++ {
			if group[i].RefCount > primary.RefCount ||
				(group[i].RefCount == primary.RefCount && group[i].ID < primary.ID) {
				primary = group[i]
			}
		}

		for _, secondary := range group {
			if secondary.ID == primary.ID {
				continue
			}

			logging.Infof("Dup-merge: normalize_key=%q: merging source=%d(%q, ref=%d) → target=%d(%q, ref=%d)",
				nk, secondary.ID, secondary.Label, secondary.RefCount, primary.ID, primary.Label, primary.RefCount)

			if err := mergeOneAuxLabelDup(db, secondary.ID, primary.ID); err != nil {
				return fmt.Errorf("dup-merge: merge source=%d into target=%d: %w", secondary.ID, primary.ID, err)
			}
			mergeCount++
		}

		logging.Infof("Dup-merge: normalized %d duplicates into primary %q (id=%d)", len(group)-1, primary.Label, primary.ID)
	}

	logging.Infof("Dup-merge: complete — %d auxiliary labels merged across all groups", mergeCount)

	// Invalidate board composition cache (board_composition rows may have been reassigned).
	if InvalidateBoardCache != nil {
		InvalidateBoardCache()
	}

	return nil
}

// mergeOneAuxLabelDup merges a single source auxiliary label into a target.
// Mirrors MergeAuxiliaryLabelAlias semantics but uses direct DB operations
// (the service method lives in a package that would create a circular import).
func mergeOneAuxLabelDup(db *gorm.DB, sourceID, targetID uint) error {
	return db.Transaction(func(tx *gorm.DB) error {
		var source, target models.SemanticLabel
		if err := tx.Where("id = ? AND label_type = ?", sourceID, "auxiliary").First(&source).Error; err != nil {
			return fmt.Errorf("load source: %w", err)
		}
		if err := tx.Where("id = ? AND label_type = ?", targetID, "auxiliary").First(&target).Error; err != nil {
			return fmt.Errorf("load target: %w", err)
		}

		// Merge aliases: source label + source aliases → target aliases (dedup).
		aliasSet := make(map[string]bool)
		for _, a := range target.Aliases {
			aliasSet[strings.ToLower(strings.TrimSpace(a))] = true
		}
		for _, a := range append([]string{source.Label}, source.Aliases...) {
			key := strings.ToLower(strings.TrimSpace(a))
			if !aliasSet[key] && !strings.EqualFold(target.Label, a) {
				target.Aliases = append(target.Aliases, a)
				aliasSet[key] = true
			}
		}
		if err := tx.Save(&target).Error; err != nil {
			return fmt.Errorf("save target aliases: %w", err)
		}

		// Migrate topic_tag_semantic_labels: source → target, ON CONFLICT DO NOTHING.
		var links []models.TopicTagSemanticLabel
		if err := tx.Where("semantic_label_id = ?", sourceID).Find(&links).Error; err != nil {
			return fmt.Errorf("load source links: %w", err)
		}
		for _, link := range links {
			migrated := models.TopicTagSemanticLabel{TopicTagID: link.TopicTagID, SemanticLabelID: targetID}
			if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&migrated).Error; err != nil {
				return fmt.Errorf("migrate link topic_tag=%d: %w", link.TopicTagID, err)
			}
		}
		if err := tx.Where("semantic_label_id = ?", sourceID).Delete(&models.TopicTagSemanticLabel{}).Error; err != nil {
			return fmt.Errorf("delete source links: %w", err)
		}

		// Migrate board_composition: source → target, ON CONFLICT DO NOTHING.
		type boardCompRow struct {
			BoardID          uint `gorm:"column:board_id"`
			AuxiliaryLabelID uint `gorm:"column:auxiliary_label_id"`
		}
		var comps []boardCompRow
		if err := tx.Table("board_composition").Where("auxiliary_label_id = ?", sourceID).Find(&comps).Error; err != nil {
			return fmt.Errorf("load source board_composition: %w", err)
		}
		for _, comp := range comps {
			if err := tx.Exec(`
				INSERT INTO board_composition (board_id, auxiliary_label_id)
				VALUES (?, ?) ON CONFLICT DO NOTHING
			`, comp.BoardID, targetID).Error; err != nil {
				return fmt.Errorf("migrate board_composition board=%d: %w", comp.BoardID, err)
			}
		}
		if err := tx.Where("auxiliary_label_id = ?", sourceID).Delete(&models.BoardComposition{}).Error; err != nil {
			return fmt.Errorf("delete source board_composition: %w", err)
		}

		// Recalculate ref_counts.
		var targetRefCount int64
		if err := tx.Model(&models.TopicTagSemanticLabel{}).Where("semantic_label_id = ?", targetID).Count(&targetRefCount).Error; err != nil {
			return fmt.Errorf("count target refs: %w", err)
		}
		var sourceRefCount int64
		if err := tx.Model(&models.TopicTagSemanticLabel{}).Where("semantic_label_id = ?", sourceID).Count(&sourceRefCount).Error; err != nil {
			return fmt.Errorf("count source refs: %w", err)
		}
		if err := tx.Model(&models.SemanticLabel{}).Where("id = ?", targetID).Update("ref_count", int(targetRefCount)).Error; err != nil {
			return fmt.Errorf("update target ref_count: %w", err)
		}
		if err := tx.Model(&models.SemanticLabel{}).Where("id = ?", sourceID).Updates(map[string]any{
			"ref_count":       int(sourceRefCount),
			"status":          "disabled",
			"embedding":       nil,
			"merge_embedding": nil,
		}).Error; err != nil {
			return fmt.Errorf("disable source: %w", err)
		}

		return nil
	})
}

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
