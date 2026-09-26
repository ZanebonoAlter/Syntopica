package models

import "time"

// BoardUpgradeSuggestion stores persisted upgrade suggestions for semantic boards.
// Each row represents one suggestion from a generation run, with a lifecycle
// (pending → confirmed / dismissed) tracked via status and resolved_at.
type BoardUpgradeSuggestion struct {
	ID      uint   `gorm:"primaryKey" json:"id"`
	BatchID string `gorm:"size:64;not null;index" json:"batch_id"`

	Mode        string `gorm:"size:32;not null" json:"mode"`
	Decision    string `gorm:"size:32;not null" json:"decision"` // create_new | merge_into_existing | watch
	BoardLabel  string `gorm:"size:160;not null" json:"board_label"`
	Description string `gorm:"type:text" json:"description"`

	TargetBoardID     *uint          `json:"target_board_id,omitempty"`
	AuxiliaryLabelIDs []uint         `gorm:"type:jsonb;serializer:json;default:'[]'" json:"auxiliary_label_ids"`
	Confidence        string         `gorm:"size:16;not null;default:llm" json:"confidence"`       // high | llm
	Evidence          map[string]any `gorm:"type:jsonb;serializer:json" json:"evidence"`           // {shortlist, margins, cotag_events, lane_briefs} snapshot
	Status            string         `gorm:"size:16;not null;default:pending;index" json:"status"` // pending | confirmed | dismissed
	DismissReason     *string        `gorm:"type:text" json:"dismiss_reason,omitempty"`
	CreatedAt         time.Time      `json:"created_at"`
	ResolvedAt        *time.Time     `json:"resolved_at,omitempty"`
	ResolvedBy        *string        `gorm:"size:50" json:"resolved_by,omitempty"`

	// SuggestionHash is a stable fingerprint of (mode, decision, target_board_id, sorted_auxiliary_label_ids).
	// A partial unique index on this column WHERE status='pending' enforces:
	//   - Idempotent generation: same cluster + decision won't insert a duplicate pending row.
	//   - Dismissed rows with the same hash are re-checked for cooldown on re-generation.
	SuggestionHash string `gorm:"size:64;not null" json:"suggestion_hash"`
}

func (BoardUpgradeSuggestion) TableName() string {
	return "board_upgrade_suggestions"
}
