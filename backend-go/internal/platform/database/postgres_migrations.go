package database

import (
	"fmt"

	"gorm.io/gorm"
	"syntopica-backend/internal/domain/models"
	"syntopica-backend/internal/platform/logging"
)

func postgresMigrations() []Migration {
	return []Migration{
		{
			Version:     "20260403_0001",
			Description: "Enable pgvector support before any Postgres vector-aware schema changes.",
			Up: func(db *gorm.DB) error {
				if err := db.Exec("CREATE EXTENSION IF NOT EXISTS vector").Error; err != nil {
					return fmt.Errorf("create vector extension: %w", err)
				}
				return nil
			},
		},
		{
			Version:     "20260403_0002",
			Description: "Create the baseline Postgres schema used by the current runtime.",
			Up: func(db *gorm.DB) error {
				if err := bootstrapPostgresSchema(db); err != nil {
					return fmt.Errorf("bootstrap postgres schema: %w", err)
				}
				return nil
			},
		},
		{
			Version:     "20260403_0003",
			Description: "Ensure topic_tag_embeddings.embedding column exists as vector(4096).",
			Up: func(db *gorm.DB) error {
				if err := db.Exec("ALTER TABLE topic_tag_embeddings ADD COLUMN IF NOT EXISTS embedding vector(4096)").Error; err != nil {
					return fmt.Errorf("add topic_tag_embeddings.embedding column: %w", err)
				}
				if err := db.Exec("ALTER TABLE topic_tag_embeddings ALTER COLUMN embedding TYPE vector(4096)").Error; err != nil {
					return fmt.Errorf("set topic_tag_embeddings.embedding dimensions: %w", err)
				}
				return nil
			},
		},
		{
			Version:     "20260413_0001",
			Description: "HNSW index skipped — embedding dimensions exceed HNSW 2000-dim limit; sequential scan is sufficient for this workload.",
			Up: func(db *gorm.DB) error {
				return nil
			},
		},
		{
			Version:     "20260413_0002",
			Description: "Create embedding_config table with default configuration values.",
			Up: func(db *gorm.DB) error {
				if err := db.AutoMigrate(&models.EmbeddingConfig{}); err != nil {
					return fmt.Errorf("auto-migrate embedding_config: %w", err)
				}
				// Seed default config values (upsert)
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
			Version:     "20260413_0003",
			Description: "Add status and merged_into_id columns to topic_tags for tag merge support.",
			Up: func(db *gorm.DB) error {
				if err := db.Exec("ALTER TABLE topic_tags ADD COLUMN IF NOT EXISTS status VARCHAR(20) NOT NULL DEFAULT 'active'").Error; err != nil {
					return fmt.Errorf("add topic_tags.status column: %w", err)
				}
				if err := db.Exec("ALTER TABLE topic_tags ADD COLUMN IF NOT EXISTS merged_into_id INTEGER REFERENCES topic_tags(id)").Error; err != nil {
					return fmt.Errorf("add topic_tags.merged_into_id column: %w", err)
				}
				if err := db.Exec("CREATE INDEX IF NOT EXISTS idx_topic_tags_status ON topic_tags(status)").Error; err != nil {
					return fmt.Errorf("create idx_topic_tags_status: %w", err)
				}
				if err := db.Exec("CREATE INDEX IF NOT EXISTS idx_topic_tags_merged_into_id ON topic_tags(merged_into_id)").Error; err != nil {
					return fmt.Errorf("create idx_topic_tags_merged_into_id: %w", err)
				}
				return nil
			},
		},
		{
			Version:     "20260413_0004",
			Description: "Create embedding_queue table for tracking embedding generation progress.",
			Up: func(db *gorm.DB) error {
				stmts := []string{
					`CREATE TABLE IF NOT EXISTS embedding_queues (
					id BIGSERIAL PRIMARY KEY,
					tag_id BIGINT NOT NULL REFERENCES topic_tags(id) ON DELETE CASCADE,
					status VARCHAR(20) NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'processing', 'completed', 'failed')),
					error_message TEXT,
					retry_count INTEGER NOT NULL DEFAULT 0,
					created_at TIMESTAMP NOT NULL DEFAULT NOW(),
					started_at TIMESTAMP,
					completed_at TIMESTAMP
				)`,
					"CREATE INDEX IF NOT EXISTS idx_embedding_queues_status ON embedding_queues(status)",
					"CREATE INDEX IF NOT EXISTS idx_embedding_queues_tag_id ON embedding_queues(tag_id)",
				}
				for _, s := range stmts {
					if err := db.Exec(s).Error; err != nil {
						return fmt.Errorf("embedding_queue migration: %w", err)
					}
				}
				return nil
			},
		},
		{
			Version:     "20260413_0005",
			Description: "Create merge_reembedding_queues table for merge-triggered embedding regeneration.",
			Up: func(db *gorm.DB) error {
				stmts := []string{
					`CREATE TABLE IF NOT EXISTS merge_reembedding_queues (
					id BIGSERIAL PRIMARY KEY,
					source_tag_id BIGINT NOT NULL REFERENCES topic_tags(id) ON DELETE CASCADE,
					target_tag_id BIGINT NOT NULL REFERENCES topic_tags(id) ON DELETE CASCADE,
					status VARCHAR(20) NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'processing', 'completed', 'failed')),
					error_message TEXT,
					retry_count INTEGER NOT NULL DEFAULT 0,
					created_at TIMESTAMP NOT NULL DEFAULT NOW(),
					started_at TIMESTAMP,
					completed_at TIMESTAMP
				)`,
					"CREATE INDEX IF NOT EXISTS idx_merge_reembedding_queues_status ON merge_reembedding_queues(status)",
					"CREATE INDEX IF NOT EXISTS idx_merge_reembedding_queues_source_tag_id ON merge_reembedding_queues(source_tag_id)",
					"CREATE INDEX IF NOT EXISTS idx_merge_reembedding_queues_target_tag_id ON merge_reembedding_queues(target_tag_id)",
				}
				for _, s := range stmts {
					if err := db.Exec(s).Error; err != nil {
						return fmt.Errorf("merge_reembedding_queue migration: %w", err)
					}
				}
				return nil
			},
		},
		{
			Version:     "20260414_0001",
			Description: "Add description column to topic_tags for LLM-generated tag descriptions.",
			Up: func(db *gorm.DB) error {
				if err := db.Exec("ALTER TABLE topic_tags ADD COLUMN IF NOT EXISTS description TEXT").Error; err != nil {
					return fmt.Errorf("add topic_tags.description column: %w", err)
				}
				return nil
			},
		},
		{
			Version:     "20260414_0002",
			Description: "Create topic_tag_relations table for abstract tag hierarchical relationships.",
			Up: func(db *gorm.DB) error {
				stmts := []string{
					`CREATE TABLE IF NOT EXISTS topic_tag_relations (
					id SERIAL PRIMARY KEY,
					parent_id INTEGER NOT NULL REFERENCES topic_tags(id) ON DELETE CASCADE,
					child_id INTEGER NOT NULL REFERENCES topic_tags(id) ON DELETE CASCADE,
					relation_type VARCHAR(20) NOT NULL DEFAULT 'abstract',
					similarity_score FLOAT,
					created_at TIMESTAMP DEFAULT NOW(),
					UNIQUE(parent_id, child_id)
				)`,
					"CREATE INDEX IF NOT EXISTS idx_tag_relations_parent ON topic_tag_relations(parent_id)",
					"CREATE INDEX IF NOT EXISTS idx_tag_relations_child ON topic_tag_relations(child_id)",
				}
				for _, s := range stmts {
					if err := db.Exec(s).Error; err != nil {
						return fmt.Errorf("topic_tag_relations migration: %w", err)
					}
				}
				return nil
			},
		},
		{
			Version:     "20260415_0001",
			Description: "Add is_watched and watched_at columns to topic_tags for watched tag support.",
			Up: func(db *gorm.DB) error {
				stmts := []string{
					"ALTER TABLE topic_tags ADD COLUMN IF NOT EXISTS is_watched BOOLEAN NOT NULL DEFAULT false",
					"ALTER TABLE topic_tags ADD COLUMN IF NOT EXISTS watched_at TIMESTAMP",
				}
				for _, s := range stmts {
					if err := db.Exec(s).Error; err != nil {
						return fmt.Errorf("add watched columns to topic_tags: %w", err)
					}
				}
				return nil
			},
		},
		{
			Version:     "20260416_0001",
			Description: "Create abstract_tag_update_queues table for refreshing abstract tag descriptions and embeddings.",
			Up: func(db *gorm.DB) error {
				stmts := []string{
					`CREATE TABLE IF NOT EXISTS abstract_tag_update_queues (
					id BIGSERIAL PRIMARY KEY,
					abstract_tag_id BIGINT NOT NULL REFERENCES topic_tags(id) ON DELETE CASCADE,
					trigger_reason VARCHAR(50) NOT NULL,
					status VARCHAR(20) NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'processing', 'completed', 'failed')),
					error_message TEXT,
					retry_count INTEGER NOT NULL DEFAULT 0,
					created_at TIMESTAMP NOT NULL DEFAULT NOW(),
					started_at TIMESTAMP,
					completed_at TIMESTAMP
				)`,
					"CREATE INDEX IF NOT EXISTS idx_abstract_tag_update_queues_status ON abstract_tag_update_queues(status)",
					"CREATE INDEX IF NOT EXISTS idx_abstract_tag_update_queues_abstract_tag_id ON abstract_tag_update_queues(abstract_tag_id)",
				}
				for _, s := range stmts {
					if err := db.Exec(s).Error; err != nil {
						return fmt.Errorf("abstract_tag_update_queue migration: %w", err)
					}
				}
				return nil
			},
		},
		{
			Version:     "20260416_0002",
			Description: "Add metadata JSONB column to topic_tags for structured tag attributes.",
			Up: func(db *gorm.DB) error {
				if err := db.Exec("ALTER TABLE topic_tags ADD COLUMN IF NOT EXISTS metadata JSONB DEFAULT '{}'::jsonb").Error; err != nil {
					return fmt.Errorf("add metadata column to topic_tags: %w", err)
				}
				return nil
			},
		},
		{
			Version:     "20260417_0001",
			Description: "Add missing indexes for CRUD performance optimization.",
			Up: func(db *gorm.DB) error {
				stmts := []string{
					"CREATE INDEX IF NOT EXISTS idx_articles_read ON articles(read)",
					"CREATE INDEX IF NOT EXISTS idx_articles_favorite ON articles(favorite)",
					"CREATE INDEX IF NOT EXISTS idx_articles_feed_pub_date ON articles(feed_id, pub_date DESC)",
					"CREATE INDEX IF NOT EXISTS idx_article_topic_tags_article_id ON article_topic_tags(article_id)",
					"CREATE INDEX IF NOT EXISTS idx_feeds_category_id ON feeds(category_id)",
					"CREATE INDEX IF NOT EXISTS idx_articles_feed_id_title ON articles(feed_id, title)",
				}
				for _, s := range stmts {
					if err := db.Exec(s).Error; err != nil {
						return fmt.Errorf("create index: %w", err)
					}
				}
				return nil
			},
		},
		{
			Version:     "20260418_0001",
			Description: "Add embedding_type to topic_tag_embeddings and allow dual embeddings per tag.",
			Up: func(db *gorm.DB) error {
				if err := db.Exec(`ALTER TABLE topic_tag_embeddings ADD COLUMN IF NOT EXISTS embedding_type VARCHAR(20) NOT NULL DEFAULT 'identity'`).Error; err != nil {
					return fmt.Errorf("add embedding_type to topic_tag_embeddings: %w", err)
				}
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
		{
			Version:     "20260420_0001",
			Description: "Add scope columns to narrative_summaries for feed-category-scoped narratives.",
			Up: func(db *gorm.DB) error {
				stmts := []string{
					"ALTER TABLE narrative_summaries ADD COLUMN IF NOT EXISTS scope_type VARCHAR(20) NOT NULL DEFAULT 'global'",
					"ALTER TABLE narrative_summaries ADD COLUMN IF NOT EXISTS scope_category_id INTEGER",
					"ALTER TABLE narrative_summaries ADD COLUMN IF NOT EXISTS scope_label VARCHAR(100)",
					"CREATE INDEX IF NOT EXISTS idx_narrative_scope ON narrative_summaries(scope_category_id)",
					"CREATE INDEX IF NOT EXISTS idx_narrative_scope_period ON narrative_summaries(scope_type, scope_category_id, period_date)",
				}
				for _, s := range stmts {
					if err := db.Exec(s).Error; err != nil {
						return fmt.Errorf("narrative scope columns migration: %w", err)
					}
				}
				return nil
			},
		},
		{
			Version:     "20260425_0001",
			Description: "Add enable_thinking column to ai_providers for reasoning model support.",
			Up: func(db *gorm.DB) error {
				if err := db.Exec(`ALTER TABLE ai_providers ADD COLUMN IF NOT EXISTS enable_thinking BOOLEAN NOT NULL DEFAULT false`).Error; err != nil {
					return fmt.Errorf("add enable_thinking to ai_providers: %w", err)
				}
				return nil
			},
		},
		{
			Version:     "20260430_0001",
			Description: "Create narrative_boards table and add board_id to narrative_summaries.",
			Up: func(db *gorm.DB) error {
				stmts := []string{
					`CREATE TABLE IF NOT EXISTS narrative_boards (
						id SERIAL PRIMARY KEY,
						period_date TIMESTAMP NOT NULL,
						name VARCHAR(300) NOT NULL,
						description TEXT,
						scope_type VARCHAR(20) NOT NULL DEFAULT 'global',
						scope_category_id INTEGER,
						event_tag_ids TEXT,
						abstract_tag_ids TEXT,
						prev_board_ids TEXT,
						created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
					)`,
					"CREATE INDEX IF NOT EXISTS idx_narrative_boards_period ON narrative_boards(period_date)",
					"CREATE INDEX IF NOT EXISTS idx_narrative_boards_scope ON narrative_boards(scope_category_id)",
					"ALTER TABLE narrative_summaries ADD COLUMN IF NOT EXISTS board_id INTEGER",
					"CREATE INDEX IF NOT EXISTS idx_narrative_summaries_board_id ON narrative_summaries(board_id)",
				}
				for _, s := range stmts {
					if err := db.Exec(s).Error; err != nil {
						return fmt.Errorf("narrative_boards migration: %w", err)
					}
				}
				return nil
			},
		},
		{
			Version:     "20260430_0002",
			Description: "Add abstract_tag_id column to narrative_boards for deterministic prev-board matching.",
			Up: func(db *gorm.DB) error {
				if err := db.Exec("ALTER TABLE narrative_boards ADD COLUMN IF NOT EXISTS abstract_tag_id INTEGER REFERENCES topic_tags(id) ON DELETE SET NULL").Error; err != nil {
					return fmt.Errorf("add abstract_tag_id to narrative_boards: %w", err)
				}
				if err := db.Exec("CREATE INDEX IF NOT EXISTS idx_narrative_boards_abstract_tag_id ON narrative_boards(abstract_tag_id)").Error; err != nil {
					return fmt.Errorf("create idx_narrative_boards_abstract_tag_id: %w", err)
				}
				return nil
			},
		},
		{
			Version:     "20260426_0001",
			Description: "Create adopt_narrower_queues table with partial unique index for dedup.",
			Up: func(db *gorm.DB) error {
				if err := db.Exec(`CREATE TABLE IF NOT EXISTS adopt_narrower_queues (
					id SERIAL PRIMARY KEY,
					abstract_tag_id INTEGER NOT NULL,
					source VARCHAR(50) NOT NULL,
					status VARCHAR(20) NOT NULL DEFAULT 'pending',
					error_message TEXT,
					retry_count INTEGER DEFAULT 0,
					created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
					started_at TIMESTAMP,
					completed_at TIMESTAMP,
					CONSTRAINT fk_adopt_narrower_queues_abstract_tag FOREIGN KEY (abstract_tag_id) REFERENCES topic_tags(id) ON DELETE CASCADE
				)`).Error; err != nil {
					return fmt.Errorf("create adopt_narrower_queues table: %w", err)
				}
				if err := db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_adopt_narrower_active ON adopt_narrower_queues (abstract_tag_id) WHERE status IN ('pending', 'processing')`).Error; err != nil {
					return fmt.Errorf("create adopt_narrower active unique index: %w", err)
				}
				return nil
			},
		},
		{
			Version:     "20260430_0003",
			Description: "Add scope_label column to narrative_boards for category label display.",
			Up: func(db *gorm.DB) error {
				if err := db.Exec("ALTER TABLE narrative_boards ADD COLUMN IF NOT EXISTS scope_label VARCHAR(100) DEFAULT ''").Error; err != nil {
					return fmt.Errorf("add scope_label to narrative_boards: %w", err)
				}
				return nil
			},
		},
		{
			Version:     "20260501_0001",
			Description: "Create board_concepts table and add board_concept_id + is_system to narrative_boards.",
			Up: func(db *gorm.DB) error {
				stmts := []string{
					`CREATE TABLE IF NOT EXISTS board_concepts (
						id SERIAL PRIMARY KEY,
						name VARCHAR(300) NOT NULL,
						description TEXT,
						embedding vector(4096),
						scope_type VARCHAR(20) NOT NULL DEFAULT 'global',
						scope_category_id INTEGER,
						is_system BOOLEAN NOT NULL DEFAULT false,
						is_active BOOLEAN NOT NULL DEFAULT true,
						display_order INTEGER NOT NULL DEFAULT 0,
						created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
						updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
					)`,
					"CREATE INDEX IF NOT EXISTS idx_board_concepts_active ON board_concepts(is_active)",
					"CREATE INDEX IF NOT EXISTS idx_board_concepts_scope ON board_concepts(scope_type, scope_category_id)",
					"ALTER TABLE narrative_boards ADD COLUMN IF NOT EXISTS board_concept_id INTEGER REFERENCES board_concepts(id) ON DELETE SET NULL",
					"ALTER TABLE narrative_boards ADD COLUMN IF NOT EXISTS is_system BOOLEAN NOT NULL DEFAULT false",
					"CREATE INDEX IF NOT EXISTS idx_narrative_boards_concept_id ON narrative_boards(board_concept_id)",
				}
				for _, s := range stmts {
					if err := db.Exec(s).Error; err != nil {
						return fmt.Errorf("board_concepts migration: %w", err)
					}
				}
				// Seed narrative board AI settings
				settingsDefaults := []struct {
					Key, Value, Description string
				}{
					{"narrative_board_embedding_threshold", "0.7", "Embedding cosine similarity threshold for tag-to-board-concept matching"},
					{"narrative_board_hotspot_threshold", "3", "Minimum abstract tree node count to create a daily hotspot board"},
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
		{
			Version:     "20260510_0001",
			Description: "Add tagging_enabled column to feeds for per-feed tagging control.",
			Up: func(db *gorm.DB) error {
				if err := db.Exec("ALTER TABLE feeds ADD COLUMN IF NOT EXISTS tagging_enabled BOOLEAN NOT NULL DEFAULT true").Error; err != nil {
					return fmt.Errorf("add tagging_enabled to feeds: %w", err)
				}
				return nil
			},
		},
		{
			Version:     "20260510_0002",
			Description: "Create hierarchy_config table for tag hierarchy template configuration.",
			Up: func(db *gorm.DB) error {
				stmts := []string{
					`CREATE TABLE IF NOT EXISTS hierarchy_configs (
						id SERIAL PRIMARY KEY,
						templates JSONB NOT NULL DEFAULT '{}',
						version INTEGER NOT NULL DEFAULT 1,
						created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
						updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
					)`,
					`CREATE TABLE IF NOT EXISTS hierarchy_config_versions (
						id SERIAL PRIMARY KEY,
						config_id INTEGER REFERENCES hierarchy_configs(id),
						templates JSONB NOT NULL,
						version INTEGER NOT NULL,
						created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
					)`,
				}
				for _, s := range stmts {
					if err := db.Exec(s).Error; err != nil {
						return fmt.Errorf("hierarchy_config migration: %w", err)
					}
				}
				return nil
			},
		},
		{
			Version:     "20260512_0001",
			Description: "Schema changes for hierarchy-concept-fence: board_concepts +category/+status/-is_active, topic_tags +concept_id.",
			Up: func(db *gorm.DB) error {
				stmts := []string{
					"ALTER TABLE board_concepts ADD COLUMN IF NOT EXISTS category VARCHAR(20) NOT NULL DEFAULT 'keyword'",
					"ALTER TABLE board_concepts ADD COLUMN IF NOT EXISTS status VARCHAR(20) NOT NULL DEFAULT 'pending'",
					"ALTER TABLE topic_tags ADD COLUMN IF NOT EXISTS concept_id INTEGER REFERENCES board_concepts(id) ON DELETE SET NULL",
					"CREATE INDEX IF NOT EXISTS idx_board_concepts_category ON board_concepts(category)",
					"CREATE INDEX IF NOT EXISTS idx_board_concepts_status ON board_concepts(status)",
					"CREATE INDEX IF NOT EXISTS idx_topic_tags_concept_id ON topic_tags(concept_id)",
					"DROP INDEX IF EXISTS idx_board_concepts_active",
				}
				for _, s := range stmts {
					if err := db.Exec(s).Error; err != nil {
						return fmt.Errorf("hierarchy-concept-fence migration: %w", err)
					}
				}
				// GORM can't drop columns, so is_active stays in table but unused.
				// The model no longer has the IsActive field, so it won't be read/written.
				return nil
			},
		},
		{
			Version:     "20260510_0003",
			Description: "Create hierarchy_pending_changes table for template violation tracking.",
			Up: func(db *gorm.DB) error {
				stmts := []string{
					`CREATE TABLE IF NOT EXISTS hierarchy_pending_changes (
						id SERIAL PRIMARY KEY,
						tag_id INTEGER NOT NULL REFERENCES topic_tags(id) ON DELETE CASCADE,
						tag_label VARCHAR(160) NOT NULL,
						change_type VARCHAR(50) NOT NULL,
						current_parent_id INTEGER REFERENCES topic_tags(id) ON DELETE SET NULL,
						current_parent_label VARCHAR(160) DEFAULT '',
						reason TEXT DEFAULT '',
						status VARCHAR(20) NOT NULL DEFAULT 'pending',
						created_at TIMESTAMP NOT NULL DEFAULT NOW(),
						resolved_at TIMESTAMP
					)`,
					"CREATE INDEX IF NOT EXISTS idx_hierarchy_pending_changes_status ON hierarchy_pending_changes(status)",
					"CREATE INDEX IF NOT EXISTS idx_hierarchy_pending_changes_tag_id ON hierarchy_pending_changes(tag_id)",
					"CREATE INDEX IF NOT EXISTS idx_hierarchy_pending_changes_type_status ON hierarchy_pending_changes(change_type, status)",
				}
				for _, s := range stmts {
					if err := db.Exec(s).Error; err != nil {
						return fmt.Errorf("hierarchy_pending_changes migration: %w", err)
					}
				}
				return nil
			},
		},
		{
			Version:     "20260513_0001",
			Description: "Add sub_type column to topic_tags for keyword classification.",
			Up: func(db *gorm.DB) error {
				stmts := []string{
					"ALTER TABLE topic_tags ADD COLUMN IF NOT EXISTS sub_type VARCHAR(30)",
					"CREATE INDEX IF NOT EXISTS idx_topic_tags_sub_type ON topic_tags(sub_type)",
				}
				for _, s := range stmts {
					if err := db.Exec(s).Error; err != nil {
						return fmt.Errorf("sub_type migration: %w", err)
					}
				}
				return nil
			},
		},
		{
			Version:     "20260514_0001",
			Description: "Change topic_tag_embeddings unique index to (topic_tag_id, embedding_type, text_hash) for event-keyword-embedding support.",
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
		{
			Version:     "20260516_0001",
			Description: "Create rebuild_jobs table for hierarchy rebuild tracking",
			Up: func(db *gorm.DB) error {
				stmts := []string{
					`CREATE TABLE IF NOT EXISTS rebuild_jobs (
						id SERIAL PRIMARY KEY,
						category TEXT NOT NULL,
						trigger TEXT NOT NULL,
						status TEXT NOT NULL DEFAULT 'pending',
						total_tags INT DEFAULT 0,
						processed_tags INT DEFAULT 0,
						failed_tags INT DEFAULT 0,
						estimated_end TIMESTAMP,
						started_at TIMESTAMP,
						completed_at TIMESTAMP,
						last_tag_id INT DEFAULT 0,
						config_snapshot JSONB,
						error_detail TEXT,
						created_at TIMESTAMP DEFAULT NOW()
					)`,
					"CREATE INDEX IF NOT EXISTS idx_rebuild_jobs_category_status ON rebuild_jobs(category, status)",
				}
				for _, s := range stmts {
					if err := db.Exec(s).Error; err != nil {
						return fmt.Errorf("rebuild_jobs migration: %w", err)
					}
				}
				return nil
			},
		},
		{
			Version:     "20260516_0002",
			Description: "Add source, protected, declining, peak_tag_count to board_concepts",
			Up: func(db *gorm.DB) error {
				stmts := []string{
					"ALTER TABLE board_concepts ADD COLUMN IF NOT EXISTS source TEXT NOT NULL DEFAULT 'auto'",
					"ALTER TABLE board_concepts ADD COLUMN IF NOT EXISTS protected BOOLEAN NOT NULL DEFAULT false",
					"ALTER TABLE board_concepts ADD COLUMN IF NOT EXISTS declining BOOLEAN NOT NULL DEFAULT false",
					"ALTER TABLE board_concepts ADD COLUMN IF NOT EXISTS peak_tag_count INT NOT NULL DEFAULT 0",
				}
				for _, s := range stmts {
					if err := db.Exec(s).Error; err != nil {
						return fmt.Errorf("board_concepts columns migration: %w", err)
					}
				}
				return nil
			},
		},
		{
			Version:     "20260516_0003",
			Description: "Add index on topic_tags concept_id for abstract source queries",
			Up: func(db *gorm.DB) error {
				if err := db.Exec("CREATE INDEX IF NOT EXISTS idx_topic_tags_source_concept ON topic_tags (source, concept_id) WHERE concept_id IS NOT NULL").Error; err != nil {
					return fmt.Errorf("create idx_topic_tags_source_concept: %w", err)
				}
				return nil
			},
		},
		{
			Version:     "20260519_0001",
			Description: "Create hierarchy_anchor_signals table for temporary placement anchor context.",
			Up: func(db *gorm.DB) error {
				stmts := []string{
					`CREATE TABLE IF NOT EXISTS hierarchy_anchor_signals (
						id SERIAL PRIMARY KEY,
						category VARCHAR(20) NOT NULL,
						center_tag_id INTEGER NOT NULL REFERENCES topic_tags(id) ON DELETE CASCADE,
						member_tag_ids JSONB NOT NULL DEFAULT '[]'::jsonb,
						expires_at TIMESTAMP NOT NULL,
						created_at TIMESTAMP NOT NULL DEFAULT NOW(),
						updated_at TIMESTAMP NOT NULL DEFAULT NOW()
					)`,
					"CREATE INDEX IF NOT EXISTS idx_hierarchy_anchor_signals_category ON hierarchy_anchor_signals(category)",
					"CREATE INDEX IF NOT EXISTS idx_hierarchy_anchor_signals_center_tag_id ON hierarchy_anchor_signals(center_tag_id)",
					"CREATE INDEX IF NOT EXISTS idx_hierarchy_anchor_signals_expires_at ON hierarchy_anchor_signals(expires_at)",
				}
				for _, s := range stmts {
					if err := db.Exec(s).Error; err != nil {
						return fmt.Errorf("hierarchy_anchor_signals migration: %w", err)
					}
				}
				return nil
			},
		},
		{
			Version:     "20260521_0001",
			Description: "Create semantic label board schema alongside legacy board concept hierarchy artifacts.",
			Up: func(db *gorm.DB) error {
				stmts := []string{
					`CREATE TABLE IF NOT EXISTS semantic_labels (
						id SERIAL PRIMARY KEY,
						label VARCHAR(160) NOT NULL,
						slug VARCHAR(160) NOT NULL,
						embedding vector(4096),
						merge_embedding vector(4096),
						label_type VARCHAR(20) NOT NULL,
						aliases JSONB NOT NULL DEFAULT '[]'::jsonb,
						ref_count INTEGER NOT NULL DEFAULT 0,
						description TEXT,
						display_order INTEGER NOT NULL DEFAULT 0,
						source VARCHAR(50) NOT NULL DEFAULT 'llm_extract',
						status VARCHAR(20) NOT NULL DEFAULT 'active',
						protected BOOLEAN NOT NULL DEFAULT false,
						created_at TIMESTAMP NOT NULL DEFAULT NOW(),
						updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
						CHECK (label_type IN ('auxiliary', 'board'))
					)`,
					"CREATE UNIQUE INDEX IF NOT EXISTS idx_semantic_labels_slug ON semantic_labels(slug)",
					"CREATE INDEX IF NOT EXISTS idx_semantic_labels_label_type ON semantic_labels(label_type)",
					"CREATE INDEX IF NOT EXISTS idx_semantic_labels_status ON semantic_labels(status)",
					`CREATE TABLE IF NOT EXISTS topic_tag_semantic_labels (
						topic_tag_id INTEGER NOT NULL REFERENCES topic_tags(id) ON DELETE CASCADE,
						semantic_label_id INTEGER NOT NULL REFERENCES semantic_labels(id) ON DELETE CASCADE,
						PRIMARY KEY (topic_tag_id, semantic_label_id)
					)`,
					"CREATE INDEX IF NOT EXISTS idx_topic_tag_semantic_labels_topic_tag_id ON topic_tag_semantic_labels(topic_tag_id)",
					"CREATE INDEX IF NOT EXISTS idx_topic_tag_semantic_labels_semantic_label_id ON topic_tag_semantic_labels(semantic_label_id)",
					`CREATE TABLE IF NOT EXISTS topic_tag_board_labels (
						topic_tag_id INTEGER NOT NULL REFERENCES topic_tags(id) ON DELETE CASCADE,
						semantic_board_id INTEGER NOT NULL REFERENCES semantic_labels(id) ON DELETE CASCADE,
						score DOUBLE PRECISION NOT NULL DEFAULT 0,
						match_reason TEXT,
						created_at TIMESTAMP NOT NULL DEFAULT NOW(),
						updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
						PRIMARY KEY (topic_tag_id, semantic_board_id)
					)`,
					"CREATE INDEX IF NOT EXISTS idx_topic_tag_board_labels_topic_tag_id ON topic_tag_board_labels(topic_tag_id)",
					"CREATE INDEX IF NOT EXISTS idx_topic_tag_board_labels_semantic_board_id ON topic_tag_board_labels(semantic_board_id)",
					`CREATE TABLE IF NOT EXISTS board_composition (
						board_id INTEGER NOT NULL REFERENCES semantic_labels(id) ON DELETE CASCADE,
						auxiliary_label_id INTEGER NOT NULL REFERENCES semantic_labels(id) ON DELETE CASCADE,
						PRIMARY KEY (board_id, auxiliary_label_id)
					)`,
					"CREATE INDEX IF NOT EXISTS idx_board_composition_board_id ON board_composition(board_id)",
					"CREATE INDEX IF NOT EXISTS idx_board_composition_auxiliary_label_id ON board_composition(auxiliary_label_id)",
					"ALTER TABLE narrative_boards ADD COLUMN IF NOT EXISTS semantic_board_id INTEGER REFERENCES semantic_labels(id) ON DELETE SET NULL",
					"CREATE INDEX IF NOT EXISTS idx_narrative_boards_semantic_board_id ON narrative_boards(semantic_board_id)",
				}
				for _, s := range stmts {
					if err := db.Exec(s).Error; err != nil {
						return fmt.Errorf("semantic label board system migration: %w", err)
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
		{
			Version:     "20260522_0001",
			Description: "Drop legacy board_concepts and hierarchy system tables/columns replaced by semantic label board system.",
			Up: func(db *gorm.DB) error {
				columnDrops := []string{
					"ALTER TABLE topic_tags DROP COLUMN IF EXISTS concept_id CASCADE",
					"ALTER TABLE narrative_boards DROP COLUMN IF EXISTS abstract_tag_id CASCADE",
					"ALTER TABLE narrative_boards DROP COLUMN IF EXISTS board_concept_id CASCADE",
					"ALTER TABLE narrative_boards DROP COLUMN IF EXISTS is_system",
					"ALTER TABLE narrative_boards DROP COLUMN IF EXISTS abstract_tag_ids",
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
			Version:     "20260522_0002",
			Description: "Add semantic label merge embedding column and index for label-only auxiliary label merge checks.",
			Up: func(db *gorm.DB) error {
				stmts := []string{
					"ALTER TABLE semantic_labels ADD COLUMN IF NOT EXISTS merge_embedding vector(4096)",
				}
				for _, s := range stmts {
					if err := db.Exec(s).Error; err != nil {
						return fmt.Errorf("add semantic label merge embedding: %w", err)
					}
				}
				return nil
			},
		},
		{
			Version:     "20260523_0001",
			Description: "Drop topic_tags sub_type column after removing keyword subtype contract.",
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
		{
			Version:     "20260526_0001",
			Description: "Create board_daily_reports and daily_report_sections tables for the daily report feature.",
			Up: func(db *gorm.DB) error {
				stmts := []string{
					`CREATE TABLE IF NOT EXISTS board_daily_reports (
						id SERIAL PRIMARY KEY,
						semantic_board_id INTEGER NOT NULL,
						period_date DATE NOT NULL,
						title TEXT NOT NULL DEFAULT '',
						summary TEXT NOT NULL DEFAULT '',
						highlights JSONB,
						dynamics TEXT,
						article_count INTEGER NOT NULL DEFAULT 0,
						event_tag_count INTEGER NOT NULL DEFAULT 0,
						cluster_count INTEGER NOT NULL DEFAULT 0,
						status VARCHAR(20) NOT NULL DEFAULT 'generating',
						raw_clusters JSONB,
						prev_report_id INTEGER,
						generation_prompt_version VARCHAR(20),
						created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
						updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
					)`,
					`CREATE INDEX IF NOT EXISTS idx_board_daily_reports_semantic_board_id ON board_daily_reports(semantic_board_id)`,
					`CREATE TABLE IF NOT EXISTS daily_report_sections (
						id SERIAL PRIMARY KEY,
						report_id INTEGER NOT NULL REFERENCES board_daily_reports(id) ON DELETE CASCADE,
						cluster_index INTEGER NOT NULL DEFAULT 0,
						cluster_label VARCHAR(200) NOT NULL DEFAULT '',
						cluster_tag_ids JSONB,
						threads JSONB,
						article_count INTEGER NOT NULL DEFAULT 0,
						created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
					)`,
					`CREATE INDEX IF NOT EXISTS idx_daily_report_sections_report_id ON daily_report_sections(report_id)`,
				}
				for _, s := range stmts {
					if err := db.Exec(s).Error; err != nil {
						return fmt.Errorf("create daily report tables: %w", err)
					}
				}
				return nil
			},
		},
	}
}
