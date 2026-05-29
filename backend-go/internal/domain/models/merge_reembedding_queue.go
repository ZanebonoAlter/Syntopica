package models

import "time"

const (
	MergeReembeddingQueueStatusPending    = "pending"
	MergeReembeddingQueueStatusProcessing = "processing"
	MergeReembeddingQueueStatusCompleted  = "completed"
	MergeReembeddingQueueStatusFailed     = "failed"
)

type MergeReembeddingQueue struct {
	ID           uint       `gorm:"primaryKey" json:"id"`
	SourceTagID  uint       `gorm:"not null;index" json:"source_tag_id"`
	TargetTagID  uint       `gorm:"not null;index" json:"target_tag_id"`
	Status       string     `gorm:"size:20;not null;default:pending;index" json:"status"`
	ErrorMessage string     `gorm:"type:text" json:"error_message"`
	RetryCount   int        `gorm:"default:0" json:"retry_count"`
	CreatedAt    time.Time  `json:"created_at"`
	StartedAt    *time.Time `json:"started_at"`
	CompletedAt  *time.Time `json:"completed_at"`
	SourceTag    *TopicTag  `gorm:"foreignKey:SourceTagID" json:"source_tag,omitempty"`
	TargetTag    *TopicTag  `gorm:"foreignKey:TargetTagID" json:"target_tag,omitempty"`
}

func (MergeReembeddingQueue) TableName() string {
	return "merge_reembedding_queues"
}
