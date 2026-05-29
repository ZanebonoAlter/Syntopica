package models

import "time"

type SemanticLabel struct {
	ID             uint      `gorm:"primaryKey" json:"id"`
	Label          string    `gorm:"size:160;not null" json:"label"`
	Slug           string    `gorm:"size:160;not null;uniqueIndex:idx_semantic_labels_slug" json:"slug"`
	Embedding      *string   `gorm:"type:vector(4096);column:embedding" json:"-"`
	MergeEmbedding *string   `gorm:"type:vector(4096);column:merge_embedding" json:"-"`
	LabelType      string    `gorm:"size:20;not null;index:idx_semantic_labels_label_type" json:"label_type"`
	Aliases        []string  `gorm:"type:jsonb;serializer:json;default:'[]'" json:"aliases"`
	RefCount       int       `gorm:"not null;default:0" json:"ref_count"`
	Description    string    `gorm:"type:text" json:"description"`
	DisplayOrder   int       `gorm:"not null;default:0" json:"display_order"`
	Source         string    `gorm:"size:50;not null;default:llm_extract" json:"source"`
	Status         string    `gorm:"size:20;not null;default:active;index:idx_semantic_labels_status" json:"status"`
	Protected      bool      `gorm:"not null;default:false" json:"protected"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

func (SemanticLabel) TableName() string {
	return "semantic_labels"
}

type TopicTagSemanticLabel struct {
	TopicTagID      uint `gorm:"primaryKey;not null" json:"topic_tag_id"`
	SemanticLabelID uint `gorm:"primaryKey;not null" json:"semantic_label_id"`

	TopicTag      *TopicTag      `gorm:"foreignKey:TopicTagID;constraint:OnDelete:CASCADE" json:"topic_tag,omitempty"`
	SemanticLabel *SemanticLabel `gorm:"foreignKey:SemanticLabelID;constraint:OnDelete:CASCADE" json:"semantic_label,omitempty"`
}

func (TopicTagSemanticLabel) TableName() string {
	return "topic_tag_semantic_labels"
}

type TopicTagBoardLabel struct {
	TopicTagID      uint      `gorm:"primaryKey;not null" json:"topic_tag_id"`
	SemanticBoardID uint      `gorm:"primaryKey;not null" json:"semantic_board_id"`
	Score           float64   `gorm:"not null;default:0" json:"score"`
	MatchReason     string    `gorm:"type:text" json:"match_reason"`
	Downgraded      bool      `gorm:"not null;default:false" json:"downgraded"`
	DirectionMismatch bool    `gorm:"not null;default:false" json:"direction_mismatch"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`

	TopicTag      *TopicTag      `gorm:"foreignKey:TopicTagID;constraint:OnDelete:CASCADE" json:"topic_tag,omitempty"`
	SemanticBoard *SemanticLabel `gorm:"foreignKey:SemanticBoardID;constraint:OnDelete:CASCADE" json:"semantic_board,omitempty"`
}

func (TopicTagBoardLabel) TableName() string {
	return "topic_tag_board_labels"
}

type BoardComposition struct {
	BoardID          uint `gorm:"primaryKey;not null" json:"board_id"`
	AuxiliaryLabelID uint `gorm:"primaryKey;not null" json:"auxiliary_label_id"`

	Board          *SemanticLabel `gorm:"foreignKey:BoardID;constraint:OnDelete:CASCADE" json:"board,omitempty"`
	AuxiliaryLabel *SemanticLabel `gorm:"foreignKey:AuxiliaryLabelID;constraint:OnDelete:CASCADE" json:"auxiliary_label,omitempty"`
}

func (BoardComposition) TableName() string {
	return "board_composition"
}
