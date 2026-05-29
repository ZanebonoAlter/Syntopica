package models

import "time"

// TopicTagRelation stores hierarchical relationships between tags.
// Used for abstract tag → child tag mapping.
type TopicTagRelation struct {
	ID              uint      `gorm:"primaryKey" json:"id"`
	ParentID        uint      `gorm:"not null;uniqueIndex:idx_tag_relation_pair" json:"parent_id"`
	ChildID         uint      `gorm:"not null;uniqueIndex:idx_tag_relation_pair" json:"child_id"`
	RelationType    string    `gorm:"size:20;not null;default:abstract" json:"relation_type"` // abstract, synonym, related
	SimilarityScore float64   `json:"similarity_score"`
	CreatedAt       time.Time `json:"created_at"`

	Parent *TopicTag `gorm:"foreignKey:ParentID" json:"parent,omitempty"`
	Child  *TopicTag `gorm:"foreignKey:ChildID" json:"child,omitempty"`
}

// TableName specifies the table name for TopicTagRelation
func (TopicTagRelation) TableName() string {
	return "topic_tag_relations"
}
