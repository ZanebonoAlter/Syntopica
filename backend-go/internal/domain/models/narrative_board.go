package models

import "time"

type NarrativeBoard struct {
	ID              uint      `gorm:"primaryKey" json:"id"`
	PeriodDate      time.Time `gorm:"index:idx_narrative_boards_period;not null" json:"period_date"`
	Name            string    `gorm:"size:300;not null" json:"name"`
	Description     string    `gorm:"type:text" json:"description"`
	ScopeType       string    `gorm:"size:20;not null;default:global" json:"scope_type"`
	ScopeCategoryID *uint     `gorm:"index:idx_narrative_boards_scope" json:"scope_category_id"`
	ScopeLabel      string    `gorm:"size:100" json:"scope_label"`
	EventTagIDs     string    `gorm:"type:text" json:"event_tag_ids"`
	PrevBoardIDs    string    `gorm:"type:text" json:"prev_board_ids"`
	SemanticBoardID *uint     `gorm:"index:idx_narrative_boards_semantic_board_id" json:"semantic_board_id,omitempty"`
	IsSystem        bool      `gorm:"not null;default:false" json:"is_system"`
	CreatedAt       time.Time `json:"created_at"`
}

func (NarrativeBoard) TableName() string {
	return "narrative_boards"
}
