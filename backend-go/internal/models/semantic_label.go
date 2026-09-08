package models

import (
	"time"

	"gorm.io/gorm"
)

type SemanticLabel struct {
	ID    uint   `gorm:"primaryKey" json:"id"`
	Label string `gorm:"size:160" json:"label"`
	Slug  string `gorm:"size:160;uniqueIndex:idx_semantic_labels_slug" json:"slug"`
	// Vector columns are declared without a fixed dimension. The actual dimension is
	// determined at runtime by the configured embedder (see auxlabel.EnsureVectorDimensionOnce)
	// and may differ across deployments, so hardcoding it here would race AutoMigrate.
	Embedding      *string `gorm:"type:vector;column:embedding" json:"-"`
	MergeEmbedding *string `gorm:"type:vector;column:merge_embedding" json:"-"`
	// LabelType — "auxiliary" (neutral semantic anchor), "board" (SemanticBoard),
	// or "composite" (directed semantic unit built from ordered auxiliary
	// components, e.g. 「美债收益率」 = 美国国债 × 收益率; add-composite-labels).
	// Composites reuse this table plus composite_components; their embedding is
	// LLM-generated from the composite phrase (never synthesized from component
	// vectors) and merge_embedding stays unused.
	LabelType    string   `gorm:"size:20;index:idx_semantic_labels_label_type" json:"label_type"`
	Aliases      []string `gorm:"type:jsonb;serializer:json;default:'[]'" json:"aliases"`
	RefCount     int      `json:"ref_count"`
	Description  string   `gorm:"type:text" json:"description"`
	DisplayOrder int      `json:"display_order"`
	Source       string   `gorm:"size:50" json:"source"`
	Status       string   `gorm:"size:20;index:idx_semantic_labels_status" json:"status"`
	Protected    bool     `json:"protected"`
	// EnrichmentEnabled — whether cycle-B enrichment is enabled for this board (default false).
	EnrichmentEnabled bool `json:"enrichment_enabled"`
	// RelationAutoDiscoveryEnabled — whether automatic cross-board relation
	// discovery runs after new board briefs for this board (default false;
	// manual discovery is always available). add-evidence-backed-cross-board-relations.
	RelationAutoDiscoveryEnabled bool `json:"relation_auto_discovery_enabled"`
	// WindowDays — real-time detail window for cycle-B (default 14).
	WindowDays int `json:"window_days"`
	// ContextLayers — which granularity layers the interpreter reads (default ["week","month","year","all"]).
	// No GORM default tag: a JSON string array requires embedded double quotes inside
	// the tag value, which reflect.StructTag syntax cannot express (the `\"` spelling
	// passes vet but corrupts GORM's tag parsing — root cause of the NULL-insert bug).
	// Default is filled by BeforeCreate instead.
	ContextLayers []string  `gorm:"type:jsonb;serializer:json" json:"context_layers"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

func (SemanticLabel) TableName() string {
	return "semantic_labels"
}

// BeforeCreate fills the ContextLayers default on insert. The tag-based default
// is impossible here (see field comment), and a NULL insert would violate the
// NOT NULL constraint carried by existing databases (migration 20260723_0001).
func (s *SemanticLabel) BeforeCreate(tx *gorm.DB) error {
	if len(s.ContextLayers) == 0 {
		s.ContextLayers = []string{"week", "month", "year", "all"}
	}
	return nil
}

type TopicTagSemanticLabel struct {
	TopicTagID      uint `gorm:"primaryKey" json:"topic_tag_id"`
	SemanticLabelID uint `gorm:"primaryKey" json:"semantic_label_id"`

	TopicTag      *TopicTag      `gorm:"foreignKey:TopicTagID;constraint:OnDelete:CASCADE" json:"topic_tag,omitempty"`
	SemanticLabel *SemanticLabel `gorm:"foreignKey:SemanticLabelID;constraint:OnDelete:CASCADE" json:"semantic_label,omitempty"`
}

func (TopicTagSemanticLabel) TableName() string {
	return "topic_tag_semantic_labels"
}

type TopicTagBoardLabel struct {
	TopicTagID        uint      `gorm:"primaryKey" json:"topic_tag_id"`
	SemanticBoardID   uint      `gorm:"primaryKey" json:"semantic_board_id"`
	Score             float64   `json:"score"`
	MatchReason       string    `gorm:"type:text" json:"match_reason"`
	Downgraded        bool      `json:"downgraded"`
	DirectionMismatch bool      `json:"direction_mismatch"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`

	TopicTag      *TopicTag      `gorm:"foreignKey:TopicTagID;constraint:OnDelete:CASCADE" json:"topic_tag,omitempty"`
	SemanticBoard *SemanticLabel `gorm:"foreignKey:SemanticBoardID;constraint:OnDelete:CASCADE" json:"semantic_board,omitempty"`
}

func (TopicTagBoardLabel) TableName() string {
	return "topic_tag_board_labels"
}

type BoardComposition struct {
	BoardID          uint `gorm:"primaryKey" json:"board_id"`
	AuxiliaryLabelID uint `gorm:"primaryKey" json:"auxiliary_label_id"`

	Board          *SemanticLabel `gorm:"foreignKey:BoardID;constraint:OnDelete:CASCADE" json:"board,omitempty"`
	AuxiliaryLabel *SemanticLabel `gorm:"foreignKey:AuxiliaryLabelID;constraint:OnDelete:CASCADE" json:"auxiliary_label,omitempty"`
}

func (BoardComposition) TableName() string {
	return "board_composition"
}

// CompositeComponent stores the ordered auxiliary-label components of a
// composite semantic label (add-composite-labels). PK is (CompositeID,
// ComponentLabelID): a component appears at most once per composite; Position
// carries the 1-based ordering. Deleting the composite label row cascades.
type CompositeComponent struct {
	CompositeID      uint `gorm:"primaryKey" json:"composite_id"`
	ComponentLabelID uint `gorm:"primaryKey" json:"component_label_id"`
	Position         int  `json:"position"`

	Composite      *SemanticLabel `gorm:"foreignKey:CompositeID;constraint:OnDelete:CASCADE" json:"composite,omitempty"`
	ComponentLabel *SemanticLabel `gorm:"foreignKey:ComponentLabelID;constraint:OnDelete:CASCADE" json:"component_label,omitempty"`
}

func (CompositeComponent) TableName() string {
	return "composite_components"
}
