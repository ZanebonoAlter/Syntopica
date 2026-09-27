package models

import (
	"time"

	"syntopica-backend/internal/models"
)

// tagmanagement 域独占模型，迁自 internal/models（decouple-backend-domains D3）。

// TagCategoryMeta defines default display properties for each category
type TagCategoryMeta struct {
	Key         string // category key: event, person, keyword
	Label       string // display label: 事件, 人物, 关键词
	DefaultIcon string // Iconify icon id
	Color       string // default color for nodes/badges
}

// TopicTagEmbedding stores vector embeddings for tag similarity matching
type TopicTagEmbedding struct {
	ID            uint      `gorm:"primaryKey" json:"id"`
	TopicTagID    uint      `gorm:"uniqueIndex:idx_topic_tag_embeddings_tag_type_hash" json:"topic_tag_id"`
	EmbeddingType string    `gorm:"size:20;uniqueIndex:idx_topic_tag_embeddings_tag_type_hash" json:"embedding_type"`
	EmbeddingVec  string    `gorm:"type:vector;column:embedding" json:"-"`
	Dimension     int       `json:"dimension"`                                                                   // Vector dimension (e.g., 2048 for text-embedding-3-large)
	Model         string    `gorm:"size:50" json:"model"`                                                        // Model used: "text-embedding-ada-002"
	TextHash      string    `gorm:"size:64;uniqueIndex:idx_topic_tag_embeddings_tag_type_hash" json:"text_hash"` // Hash of (label + aliases + category) for re-embedding detection
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`

	TopicTag *models.TopicTag `gorm:"foreignKey:TopicTagID;constraint:OnDelete:CASCADE" json:"topic_tag,omitempty"`
}

// TagMergeSuggestion records a pair of similar tags proposed for manual merging.
type TagMergeSuggestion struct {
	ID            uint      `gorm:"primaryKey" json:"id"`
	NewTagID      uint      `gorm:"uniqueIndex:idx_tag_merge_suggestion_pair" json:"new_tag_id"`
	ExistingTagID uint      `gorm:"uniqueIndex:idx_tag_merge_suggestion_pair" json:"existing_tag_id"`
	NewLabel      string    `gorm:"size:160" json:"new_label"`
	ExistingLabel string    `gorm:"size:160" json:"existing_label"`
	Category      string    `gorm:"size:20" json:"category"`
	Similarity    float64   `gorm:"index:idx_tag_merge_suggestion_status_sim" json:"similarity"`
	Status        string    `gorm:"size:20;default:pending;index:idx_tag_merge_suggestion_status_sim" json:"status"` // pending, merged, dismissed
	Source        string    `gorm:"size:20" json:"source"`                                                           // incremental, full_scan
	LLMVerdict    string    `gorm:"type:text" json:"llm_verdict"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// TopicTagAnalysis 主题标签分析快照
type TopicTagAnalysis struct {
	ID           uint64    `gorm:"primaryKey" json:"id"`
	TopicTagID   uint64    `gorm:"index:idx_tag_analysis_date,unique" json:"topic_tag_id"`
	AnalysisType string    `gorm:"index:idx_tag_analysis_date,unique" json:"analysis_type"` // event, person, keyword
	WindowType   string    `gorm:"index:idx_tag_analysis_date,unique" json:"window_type"`   // daily, weekly
	AnchorDate   time.Time `gorm:"index:idx_tag_analysis_date,unique" json:"anchor_date"`
	ArticleCount int       `json:"article_count"`
	PayloadJSON  string    `gorm:"type:text" json:"payload_json"` // 首批 Postgres 切换阶段继续保留为文本 JSON 载荷，不做结构化拆分。
	Source       string    `json:"source"`                        // ai, heuristic, cached
	Version      int       `json:"version"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type TopicTagSemanticLabel struct {
	TopicTagID      uint `gorm:"primaryKey" json:"topic_tag_id"`
	SemanticLabelID uint `gorm:"primaryKey" json:"semantic_label_id"`

	TopicTag      *models.TopicTag      `gorm:"foreignKey:TopicTagID;constraint:OnDelete:CASCADE" json:"topic_tag,omitempty"`
	SemanticLabel *models.SemanticLabel `gorm:"foreignKey:SemanticLabelID;constraint:OnDelete:CASCADE" json:"semantic_label,omitempty"`
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

	TopicTag      *models.TopicTag      `gorm:"foreignKey:TopicTagID;constraint:OnDelete:CASCADE" json:"topic_tag,omitempty"`
	SemanticBoard *models.SemanticLabel `gorm:"foreignKey:SemanticBoardID;constraint:OnDelete:CASCADE" json:"semantic_board,omitempty"`
}

type BoardComposition struct {
	BoardID          uint `gorm:"primaryKey" json:"board_id"`
	AuxiliaryLabelID uint `gorm:"primaryKey" json:"auxiliary_label_id"`

	Board          *models.SemanticLabel `gorm:"foreignKey:BoardID;constraint:OnDelete:CASCADE" json:"board,omitempty"`
	AuxiliaryLabel *models.SemanticLabel `gorm:"foreignKey:AuxiliaryLabelID;constraint:OnDelete:CASCADE" json:"auxiliary_label,omitempty"`
}

// CompositeComponent stores the ordered auxiliary-label components of a
// composite semantic label (add-composite-labels). PK is (CompositeID,
// ComponentLabelID): a component appears at most once per composite; Position
// carries the 1-based ordering. Deleting the composite label row cascades.
type CompositeComponent struct {
	CompositeID      uint `gorm:"primaryKey" json:"composite_id"`
	ComponentLabelID uint `gorm:"primaryKey" json:"component_label_id"`
	Position         int  `json:"position"`

	Composite      *models.SemanticLabel `gorm:"foreignKey:CompositeID;constraint:OnDelete:CASCADE" json:"composite,omitempty"`
	ComponentLabel *models.SemanticLabel `gorm:"foreignKey:ComponentLabelID;constraint:OnDelete:CASCADE" json:"component_label,omitempty"`
}

// DefaultTagCategories returns the standard category definitions
func DefaultTagCategories() []TagCategoryMeta {
	return []TagCategoryMeta{
		{Key: models.TagCategoryEvent, Label: "事件", DefaultIcon: "mdi:calendar-star", Color: "#f59e0b"},
		{Key: models.TagCategoryPerson, Label: "人物", DefaultIcon: "mdi:account", Color: "#10b981"},
		{Key: models.TagCategoryKeyword, Label: "关键词", DefaultIcon: "mdi:tag", Color: "#6366f1"},
	}
}

// GetCategoryMeta returns the metadata for a category key
func GetCategoryMeta(category string) TagCategoryMeta {
	for _, meta := range DefaultTagCategories() {
		if meta.Key == category {
			return meta
		}
	}
	// Default to keyword
	return TagCategoryMeta{Key: models.TagCategoryKeyword, Label: "关键词", DefaultIcon: "mdi:tag", Color: "#6366f1"}
}

func (TopicTagSemanticLabel) TableName() string { return "topic_tag_semantic_labels" }

func (TopicTagBoardLabel) TableName() string { return "topic_tag_board_labels" }

func (BoardComposition) TableName() string { return "board_composition" }

func (CompositeComponent) TableName() string { return "composite_components" }
