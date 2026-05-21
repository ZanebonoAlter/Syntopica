package models

import "time"

const (
	RebuildJobStatusPending   = "pending"
	RebuildJobStatusRunning   = "running"
	RebuildJobStatusPaused    = "paused"
	RebuildJobStatusCompleted = "completed"
	RebuildJobStatusFailed    = "failed"
)

const (
	RebuildJobTriggerTemplateChange = "template_change"
	RebuildJobTriggerSectorRegen    = "sector_regen"
	RebuildJobTriggerManual         = "manual"
)

type RebuildJob struct {
	ID             uint        `gorm:"primaryKey" json:"id"`
	Category       string      `gorm:"size:20;not null;index:idx_rebuild_jobs_category_status" json:"category"`
	Trigger        string      `gorm:"size:30;not null" json:"trigger"`
	Status         string      `gorm:"size:20;not null;default:pending;index:idx_rebuild_jobs_category_status" json:"status"`
	TotalTags      int         `gorm:"default:0" json:"total_tags"`
	ProcessedTags  int         `gorm:"default:0" json:"processed_tags"`
	FailedTags     int         `gorm:"default:0" json:"failed_tags"`
	EstimatedEnd   *time.Time  `json:"estimated_end,omitempty"`
	StartedAt      *time.Time  `json:"started_at,omitempty"`
	CompletedAt    *time.Time  `json:"completed_at,omitempty"`
	LastTagID      int         `gorm:"default:0" json:"last_tag_id"`
	ConfigSnapshot MetadataMap `gorm:"type:jsonb;serializer:json;default:'{}'" json:"config_snapshot,omitempty"`
	ErrorDetail    string      `gorm:"type:text" json:"error_detail,omitempty"`
	CreatedAt      time.Time   `json:"created_at"`
}

func (RebuildJob) TableName() string {
	return "rebuild_jobs"
}
