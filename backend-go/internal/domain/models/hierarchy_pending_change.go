package models

import (
	"time"
)

type HierarchyPendingChange struct {
	ID                 uint       `gorm:"primaryKey" json:"id"`
	TagID              uint       `gorm:"not null;index;constraint:OnDelete:CASCADE" json:"tag_id"`
	TagLabel           string     `gorm:"size:160;not null" json:"tag_label"`
	ChangeType         string     `gorm:"size:50;not null" json:"change_type"`
	CurrentParentID    *uint      `json:"current_parent_id,omitempty"`
	CurrentParentLabel string     `gorm:"size:160" json:"current_parent_label"`
	Reason             string     `gorm:"type:text" json:"reason"`
	Status             string     `gorm:"size:20;not null;default:pending;index" json:"status"`
	CreatedAt          time.Time  `json:"created_at"`
	ResolvedAt         *time.Time `json:"resolved_at,omitempty"`

	Tag    *TopicTag `gorm:"foreignKey:TagID" json:"tag,omitempty"`
	Parent *TopicTag `gorm:"foreignKey:CurrentParentID" json:"parent,omitempty"`
}

func (HierarchyPendingChange) TableName() string {
	return "hierarchy_pending_changes"
}
