package models

import (
	"time"

	"gorm.io/gorm"
)

// Entity-subject identity layer (normalize-event-subject-entities D1).
//
// Three tables carry the global, stable entity identity:
//
//	entity_registry   — identity rows (stable ID, opaque entity_key the model
//	                     echoes back, canonical name WITHOUT a unique
//	                     constraint, stable short description, two retrieval
//	                     representations each with model/version/dim on-row)
//	entity_mentions   — name & evidence rows (every mention is evidence AND a
//	                     registered way of calling the entity; names are
//	                     allowed to be ambiguous across entities)
//	entity_aux_links  — auxiliary-label provenance rows: ONLY the aux row whose
//	                     normalized label text equals the normalized mention
//	                     text of this tagging may be linked here. It is a
//	                     provenance mapping, NOT same-entity evidence: when the
//	                     aux label is GC'd (disabled) or deleted, only these
//	                     link rows disappear — the entity identity survives
//	                     (FK direction: entity_aux_links.aux_label_id →
//	                     semantic_labels.id ON DELETE CASCADE; nothing ever
//	                     cascades INTO entity_registry from aux).
//
// Deliberately NO entity-type column: the only ready source of types is the
// model-invented subject_key prefix, which has been measured to drift
// (company:deepseek vs product:deepseek); letting the model guess types would
// bake that drift into identity (design D1).
//
// Vector columns are declared as bare `vector` (no typmod) — the dimension is
// a property of the embedding model that produced the row, recorded on-row
// (model/version/dim triplets), never of the schema.

// EntityRegistry is the identity row of one globally-stable entity.
// CanonicalName has NO unique constraint: different real-world entities may
// share a name and MUST be allowed to coexist (spec「同名不同实体可并存」);
// nothing ever auto-merges two rows by name or by similarity.
type EntityRegistry struct {
	ID uint `gorm:"primaryKey" json:"id"`
	// EntityKey is a short opaque handle (e.g. "E7F3K9Q2") the second-round
	// model echoes back. Unique; never reused.
	EntityKey string `gorm:"size:40;uniqueIndex:uq_entity_registry_key" json:"entity_key"`
	// CanonicalName — the entity's canonical display name. NOT unique.
	CanonicalName string `gorm:"size:200;index:idx_entity_registry_name" json:"canonical_name"`
	// StableDesc — stable short identity description from the FIRST-round
	// extraction (shadow mode only). Describes the entity itself
	// (category/country/role/affiliation), never the current event. May be
	// empty (first round did not produce one); NEVER backfilled from
	// subject_key and never fabricated.
	StableDesc string `gorm:"type:text" json:"stable_desc"`
	// NameEmbedding — retrieval representation of the canonical name alone.
	// Always present for searchable entities (the name path always exists).
	NameEmbedding *string `gorm:"type:vector;column:name_embedding" json:"-"`
	// Space triplet of NameEmbedding; rows whose triplet differs from the
	// query vector's are skipped (same-space-only comparison, D3).
	NameEmbeddingModel   string `gorm:"size:120;column:name_embedding_model" json:"name_embedding_model"`
	NameEmbeddingVersion string `gorm:"size:60;column:name_embedding_version" json:"name_embedding_version"`
	NameEmbeddingDim     int    `gorm:"column:name_embedding_dim" json:"name_embedding_dim"`
	// NameDescEmbedding — retrieval representation of name + stable short
	// description. Nullable: entities created from a mention without a
	// description have no second representation (name-only, never hidden).
	NameDescEmbedding *string `gorm:"type:vector;column:name_desc_embedding" json:"-"`
	// Space triplet of NameDescEmbedding.
	NameDescEmbeddingModel   string    `gorm:"size:120;column:name_desc_embedding_model" json:"name_desc_embedding_model"`
	NameDescEmbeddingVersion string    `gorm:"size:60;column:name_desc_embedding_version" json:"name_desc_embedding_version"`
	NameDescEmbeddingDim     int       `gorm:"column:name_desc_embedding_dim" json:"name_desc_embedding_dim"`
	CreatedAt                time.Time `json:"created_at"`
	UpdatedAt                time.Time `json:"updated_at"`
}

func (EntityRegistry) TableName() string { return "entity_registry" }

// EntityMention is one mention-evidence row. Every mention text is
// simultaneously registered as a way of calling that entity (third candidate
// path: registered-name hit), so the same spelling may legitimately map to
// several entities (ambiguity is allowed; name hit ≠ identity decision).
//
// Idempotency key = (source_tag_id, mention_key) — the SAME tag re-processing
// the SAME mention replays the prior decision without another model call.
// The key is deliberately NOT the mention text alone: the same bare text in a
// different article may point at a different entity (D8).
type EntityMention struct {
	ID          uint   `gorm:"primaryKey" json:"id"`
	EntityID    uint   `gorm:"index:idx_entity_mentions_entity" json:"entity_id"`
	MentionText string `gorm:"size:400" json:"mention_text"` // verbatim mention evidence
	// MentionKey = textutil.NormalizeEntityText(MentionText); used for the
	// registered-name hit path and for idempotency.
	MentionKey string `gorm:"size:400;index:idx_entity_mentions_key" json:"mention_key"`
	// SourceTagID — the topic_tags.id this mention came from (NULL for
	// eval-run synthetic sources that are not allowed to write business
	// tables). Together with MentionKey it forms the idempotency pair; the
	// unique index is partial (WHERE source_tag_id IS NOT NULL).
	SourceTagID *uint `gorm:"column:source_tag_id" json:"source_tag_id"`
	// SourceArticleID — optional provenance of the mention.
	SourceArticleID *uint     `gorm:"column:source_article_id" json:"source_article_id"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

func (EntityMention) TableName() string { return "entity_mentions" }

// EntityAuxLink is one auxiliary-label provenance row. The ONLY legal writer
// is the rule「the aux row whose normalized label equals the normalized
// mention text of this tagging」; auto-aliases and similarity hits are NOT
// same-entity evidence and never land here (spec「辅助标签溯源」).
type EntityAuxLink struct {
	ID       uint `gorm:"primaryKey" json:"id"`
	EntityID uint `gorm:"index:idx_entity_aux_links_entity" json:"entity_id"`
	// MentionID — the evidence row this link was recorded for.
	MentionID uint `gorm:"index:idx_entity_aux_links_mention" json:"mention_id"`
	// AuxLabelID — semantic_labels.id of the matched aux row. FK ON DELETE
	// CASCADE (migration): deleting the aux row removes ONLY this link row;
	// a GC-disabled aux row is filtered out at read time (status != active).
	AuxLabelID uint      `gorm:"index:idx_entity_aux_links_aux" json:"aux_label_id"`
	CreatedAt  time.Time `json:"created_at"`
}

func (EntityAuxLink) TableName() string { return "entity_aux_links" }

// EntitySubjectModels returns the three entity-subject table models for the
// shared AutoMigrate registry (tagmanagement/wire.go init registers them via
// database.RegisterModels).
func EntitySubjectModels() []any {
	return []any{&EntityRegistry{}, &EntityMention{}, &EntityAuxLink{}}
}

// RegisterModels auto-migrates the three entity-subject tables on the given
// connection. Standalone entry for CLIs (entity-resolve-eval) that build
// their own schema without pulling the whole tagmanagement wire set.
func RegisterModels(db *gorm.DB) error {
	return db.AutoMigrate(EntitySubjectModels()...)
}
