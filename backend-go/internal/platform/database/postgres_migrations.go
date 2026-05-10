package database

import (
	"fmt"

	"gorm.io/gorm"
	"my-robot-backend/internal/domain/models"
	"my-robot-backend/internal/platform/logging"
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
			Description: "Staged groundwork for the later pgvector cutover: add the embedding vector column while runtime still reads the legacy JSON vector field.",
			Up: func(db *gorm.DB) error {
				if err := db.Exec("ALTER TABLE topic_tag_embeddings ADD COLUMN IF NOT EXISTS embedding vector(1536)").Error; err != nil {
					return fmt.Errorf("add topic_tag_embeddings.embedding column: %w", err)
				}
				return nil
			},
		},
		{
			Version:     "20260413_0001",
			Description: "Add HNSW index on topic_tag_embeddings.embedding for fast cosine similarity search.",
			Up: func(db *gorm.DB) error {
				if err := db.Exec("CREATE INDEX IF NOT EXISTS idx_topic_tag_embeddings_embedding ON topic_tag_embeddings USING hnsw (embedding vector_cosine_ops)").Error; err != nil {
					return fmt.Errorf("create hnsw index on topic_tag_embeddings.embedding: %w", err)
				}
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
					{Key: "embedding_dimension", Value: "1536", Description: "Embedding vector dimension"},
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
						embedding vector(1536),
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
				if err := db.AutoMigrate(&models.HierarchyConfig{}, &models.HierarchyConfigVersion{}); err != nil {
					return fmt.Errorf("auto-migrate hierarchy_config tables: %w", err)
				}
				var count int64
				if err := db.Model(&models.HierarchyConfig{}).Count(&count).Error; err != nil {
					return fmt.Errorf("check hierarchy_config count: %w", err)
				}
				if count == 0 {
					if err := db.Create(&models.HierarchyConfig{
						Templates: models.HierarchyTemplatesJSON{},
						Version:   1,
					}).Error; err != nil {
						return fmt.Errorf("seed hierarchy_config: %w", err)
					}
				}
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
	}
}
