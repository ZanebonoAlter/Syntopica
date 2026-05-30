package database

import (
	"fmt"

	"gorm.io/gorm"
)

// bootstrapPostgresSchema runs baseline adjustments for the first-time Postgres setup.
// GORM AutoMigrate handles table/column creation automatically on every startup,
// so this only handles adjustments that AutoMigrate cannot do (column type changes, indexes, FK cascades).
func bootstrapPostgresSchema(db *gorm.DB) error {
	if db.Name() == "postgres" {
		for _, statement := range postgresColumnAdjustmentStatements() {
			if err := db.Exec(statement).Error; err != nil {
				return fmt.Errorf("apply postgres column adjustment %q: %w", statement, err)
			}
		}
	}

	for _, statement := range postgresBaselineIndexStatements() {
		if err := db.Exec(statement).Error; err != nil {
			return fmt.Errorf("apply postgres baseline index %q: %w", statement, err)
		}
	}

	for _, statement := range postgresForeignKeyCascadeStatements() {
		if err := db.Exec(statement).Error; err != nil {
			return fmt.Errorf("apply postgres fk cascade %q: %w", statement, err)
		}
	}

	return nil
}

func postgresColumnAdjustmentStatements() []string {
	return []string{
		"ALTER TABLE feeds ALTER COLUMN icon TYPE VARCHAR(1000)",
	}
}

func postgresBaselineIndexStatements() []string {
	return []string{
		"CREATE INDEX IF NOT EXISTS idx_articles_feed_created_at ON articles(feed_id, created_at DESC)",
		"CREATE INDEX IF NOT EXISTS idx_articles_pub_date ON articles(pub_date)",

		"CREATE INDEX IF NOT EXISTS idx_article_topic_tags_topic_article ON article_topic_tags(topic_tag_id, article_id)",

		"CREATE INDEX IF NOT EXISTS idx_reading_behaviors_feed_created_at ON reading_behaviors(feed_id, created_at DESC)",
		"CREATE INDEX IF NOT EXISTS idx_firecrawl_jobs_status_available_at ON firecrawl_jobs(status, available_at)",
		"CREATE INDEX IF NOT EXISTS idx_firecrawl_jobs_lease_expires_at ON firecrawl_jobs(lease_expires_at)",
		"CREATE INDEX IF NOT EXISTS idx_tag_jobs_status_available_at ON tag_jobs(status, available_at)",
		"CREATE INDEX IF NOT EXISTS idx_tag_jobs_lease_expires_at ON tag_jobs(lease_expires_at)",
	}
}

func postgresForeignKeyCascadeStatements() []string {
	return []string{
		`DO $$
		BEGIN
			IF EXISTS (SELECT 1 FROM information_schema.table_constraints WHERE constraint_name = 'fk_reading_behaviors_article' AND table_name = 'reading_behaviors') THEN
				ALTER TABLE reading_behaviors DROP CONSTRAINT fk_reading_behaviors_article;
			END IF;
		END$$`,
		`ALTER TABLE reading_behaviors ADD CONSTRAINT fk_reading_behaviors_article FOREIGN KEY (article_id) REFERENCES articles(id) ON DELETE CASCADE`,
	}
}
