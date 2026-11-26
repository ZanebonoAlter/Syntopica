package database

import (
	"fmt"

	"gorm.io/gorm"
)

// entitySubjectMigration implements 20261112_0001
// (normalize-event-subject-entities T1): indexes and FK constraints for the
// three entity-subject tables. The tables themselves come from AutoMigrate
// (tagmanagement/models/entity_subject.go — registered by tagmanagement/wire.go
// init); this migration pins the constraints AutoMigrate cannot express
// (foreign keys are globally disabled during AutoMigrate, and the partial
// unique idempotency index needs explicit SQL):
//
//   - entity_mentions.entity_id    → entity_registry(id)  ON DELETE CASCADE
//   - entity_aux_links.entity_id   → entity_registry(id)  ON DELETE CASCADE
//   - entity_aux_links.mention_id  → entity_mentions(id)  ON DELETE CASCADE
//   - entity_aux_links.aux_label_id→ semantic_labels(id)  ON DELETE CASCADE
//     ← THE cascade direction contract: deleting an aux label removes ONLY the
//     provenance row; the entity identity is never touched from the aux side.
//     A GC-disabled aux row (status != 'active') is filtered at read time.
//   - uq_entity_mentions_source_key ON entity_mentions(source_tag_id,
//     mention_key) WHERE source_tag_id IS NOT NULL — the idempotency pair
//     (source tag, mention key); NULL sources (eval runs) never collide.
//
// Canonical name is deliberately NOT unique (spec「同名不同实体可并存」).
// Vector columns are bare `vector` (no typmod) — dims live on-row. No
// backfill, no historical rewrite: brand-new empty tables.
//
// Idempotent (IF NOT EXISTS); no-ops when the tables are absent (binaries
// that do not pull the tagmanagement wire registration build their schema
// without them).
func entitySubjectMigration() Migration {
	return Migration{
		Version:     "20261112_0001",
		Description: "实体主体身份层三表（entity_registry/entity_mentions/entity_aux_links）的 FK 级联方向与幂等唯一索引；无回填。",
		Up: func(db *gorm.DB) error {
			if db.Dialector == nil || db.Name() != "postgres" {
				return nil
			}
			if err := ensureEntitySubjectTables(db); err != nil {
				return err
			}
			statements := []string{
				// Idempotency pair (source tag, mention key). Partial: eval-run
				// mentions with NULL source never take part.
				`CREATE UNIQUE INDEX IF NOT EXISTS uq_entity_mentions_source_key
					ON entity_mentions(source_tag_id, mention_key)
					WHERE source_tag_id IS NOT NULL`,
				// FK cascade directions (see file comment).
				`DO $$ BEGIN
					IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'fk_entity_mentions_entity') THEN
						ALTER TABLE entity_mentions ADD CONSTRAINT fk_entity_mentions_entity
							FOREIGN KEY (entity_id) REFERENCES entity_registry(id) ON DELETE CASCADE;
					END IF;
				END $$`,
				`DO $$ BEGIN
					IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'fk_entity_aux_links_entity') THEN
						ALTER TABLE entity_aux_links ADD CONSTRAINT fk_entity_aux_links_entity
							FOREIGN KEY (entity_id) REFERENCES entity_registry(id) ON DELETE CASCADE;
					END IF;
				END $$`,
				`DO $$ BEGIN
					IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'fk_entity_aux_links_mention') THEN
						ALTER TABLE entity_aux_links ADD CONSTRAINT fk_entity_aux_links_mention
							FOREIGN KEY (mention_id) REFERENCES entity_mentions(id) ON DELETE CASCADE;
					END IF;
				END $$`,
				`DO $$ BEGIN
					IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'fk_entity_aux_links_aux') THEN
						ALTER TABLE entity_aux_links ADD CONSTRAINT fk_entity_aux_links_aux
							FOREIGN KEY (aux_label_id) REFERENCES semantic_labels(id) ON DELETE CASCADE;
					END IF;
				END $$`,
			}
			for _, stmt := range statements {
				if err := db.Exec(stmt).Error; err != nil {
					return fmt.Errorf("entity-subject migration: %w", err)
				}
			}
			return nil
		},
	}
}

// ensureEntitySubjectTables creates the three tables when AutoMigrate has not
// run yet (a binary that runs migrations before wiring the tagmanagement
// domain registration would otherwise record the version on a no-op that
// never re-runs). Mirrors the models verbatim.
func ensureEntitySubjectTables(db *gorm.DB) error {
	var count int64
	if err := db.Raw(`SELECT count(*) FROM information_schema.tables WHERE table_schema='public'
		AND table_name IN ('entity_registry','entity_mentions','entity_aux_links')`).Scan(&count).Error; err != nil {
		return fmt.Errorf("check entity-subject tables: %w", err)
	}
	if count == 3 {
		return nil
	}
	if err := db.Exec(`CREATE TABLE IF NOT EXISTS entity_registry (
		id bigserial PRIMARY KEY,
		entity_key varchar(40) NOT NULL,
		canonical_name varchar(200),
		stable_desc text,
		name_embedding vector,
		name_embedding_model varchar(120),
		name_embedding_version varchar(60),
		name_embedding_dim integer,
		name_desc_embedding vector,
		name_desc_embedding_model varchar(120),
		name_desc_embedding_version varchar(60),
		name_desc_embedding_dim integer,
		created_at timestamptz,
		updated_at timestamptz
	)`).Error; err != nil {
		return fmt.Errorf("create entity_registry: %w", err)
	}
	if err := db.Exec(`CREATE TABLE IF NOT EXISTS entity_mentions (
		id bigserial PRIMARY KEY,
		entity_id bigint NOT NULL,
		mention_text varchar(400),
		mention_key varchar(400),
		source_tag_id bigint,
		source_article_id bigint,
		created_at timestamptz,
		updated_at timestamptz
	)`).Error; err != nil {
		return fmt.Errorf("create entity_mentions: %w", err)
	}
	if err := db.Exec(`CREATE TABLE IF NOT EXISTS entity_aux_links (
		id bigserial PRIMARY KEY,
		entity_id bigint NOT NULL,
		mention_id bigint NOT NULL,
		aux_label_id bigint NOT NULL,
		created_at timestamptz
	)`).Error; err != nil {
		return fmt.Errorf("create entity_aux_links: %w", err)
	}
	for _, stmt := range []string{
		`CREATE UNIQUE INDEX IF NOT EXISTS uq_entity_registry_key ON entity_registry(entity_key)`,
		`CREATE INDEX IF NOT EXISTS idx_entity_registry_name ON entity_registry(canonical_name)`,
		`CREATE INDEX IF NOT EXISTS idx_entity_mentions_entity ON entity_mentions(entity_id)`,
		`CREATE INDEX IF NOT EXISTS idx_entity_mentions_key ON entity_mentions(mention_key)`,
		`CREATE INDEX IF NOT EXISTS idx_entity_aux_links_entity ON entity_aux_links(entity_id)`,
		`CREATE INDEX IF NOT EXISTS idx_entity_aux_links_mention ON entity_aux_links(mention_id)`,
		`CREATE INDEX IF NOT EXISTS idx_entity_aux_links_aux ON entity_aux_links(aux_label_id)`,
	} {
		if err := db.Exec(stmt).Error; err != nil {
			return fmt.Errorf("entity-subject fallback indexes: %w", err)
		}
	}
	return nil
}
