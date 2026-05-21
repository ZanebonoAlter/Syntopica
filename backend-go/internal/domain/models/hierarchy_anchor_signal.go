package models

import "time"

type HierarchyAnchorSignal struct {
	ID           uint      `gorm:"primaryKey" json:"id"`
	Category     string    `gorm:"size:20;not null;index" json:"category"`
	CenterTagID  uint      `gorm:"not null;index" json:"center_tag_id"`
	MemberTagIDs []uint    `gorm:"type:jsonb;serializer:json;not null" json:"member_tag_ids"`
	ExpiresAt    time.Time `gorm:"not null;index" json:"expires_at"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

func (HierarchyAnchorSignal) TableName() string {
	return "hierarchy_anchor_signals"
}
