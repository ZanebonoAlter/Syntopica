package models

import "time"

// Notification is a persistent, user-facing task result notification
// (add-notification-center). The whitelist is daily-report terminal states
// only: the creators exposed by internal/platform/notification enforce it at
// the API surface, so high-frequency process events (tagging, firecrawl,
// auto-refresh) can never write rows.
type Notification struct {
	ID        uint       `gorm:"primaryKey" json:"id"`
	Type      string     `gorm:"size:20;not null" json:"type"` // success | error
	Title     string     `gorm:"size:200;not null" json:"title"`
	Summary   string     `gorm:"size:500" json:"summary"`
	LinkType  string     `gorm:"size:50" json:"link_type,omitempty"` // e.g. daily-report
	LinkID    string     `gorm:"size:100" json:"link_id,omitempty"`
	IsRead    bool       `gorm:"index;not null;default:false" json:"is_read"`
	CreatedAt time.Time  `gorm:"index" json:"created_at"`
	ReadAt    *time.Time `json:"read_at,omitempty"`
}

func (Notification) TableName() string {
	return "notifications"
}
