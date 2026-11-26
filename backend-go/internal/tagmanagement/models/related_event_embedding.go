package models

import "time"

// RelatedEventEmbedding is the derived cache of related-event representation
// vectors (enable-story-cross-day-continuation D3). It is a DISCARDABLE index,
// not an identity registry: every row is reproducible from the tag's current
// text, and rows whose text_hash no longer matches the current text simply stop
// participating in queries until an explicit refresh rewrites them.
//
// Isolation: this table is deliberately separate from topic_tag_embeddings
// (identity/semantic/event_keyword vectors are read by tag dedupe/merge) and
// from topic_tag_relations (reader-side descendant expansion ignores
// relation_type, so 'related' rows there would pollute tag expansion).
//
// Primary key (tag_id, representation_version, space_key) encodes the two
// invalidation axes: a representation change (text layout) bumps the version,
// a model-space change (vendor/model swap, explicit index generation) changes
// space_key — same dimension never implies same space. Old rows are ignored,
// never silently reused.
type RelatedEventEmbedding struct {
	TopicTagID            uint      `gorm:"primaryKey;autoIncrement:false" json:"topic_tag_id"`
	RepresentationVersion string    `gorm:"primaryKey;size:40" json:"representation_version"`
	SpaceKey              string    `gorm:"primaryKey;size:220" json:"space_key"`
	TextHash              string    `gorm:"size:64;index" json:"text_hash"`
	Dimension             int       `json:"dimension"`
	EmbeddingVec          string    `gorm:"type:vector;column:embedding" json:"-"`
	CreatedAt             time.Time `json:"created_at"`
	UpdatedAt             time.Time `json:"updated_at"`
}

func (RelatedEventEmbedding) TableName() string { return "related_event_embeddings" }
