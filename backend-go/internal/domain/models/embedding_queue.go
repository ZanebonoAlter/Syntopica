package models

import "time"

const (
	EmbeddingQueueStatusPending    = "pending"
	EmbeddingQueueStatusProcessing = "processing"
	EmbeddingQueueStatusCompleted  = "completed"
	EmbeddingQueueStatusFailed     = "failed"
)

type EmbeddingQueue struct {
	ID           uint       `gorm:"primaryKey" json:"id"`
	TagID        uint       `gorm:"not null;index" json:"tag_id"`
	Status       string     `gorm:"size:20;not null;default:pending;index" json:"status"`
	ErrorMessage string     `gorm:"type:text" json:"error_message"`
	RetryCount   int        `gorm:"default:0" json:"retry_count"`
	CreatedAt    time.Time  `json:"created_at"`
	StartedAt    *time.Time `json:"started_at"`
	CompletedAt  *time.Time `json:"completed_at"`
	Tag          *TopicTag  `gorm:"foreignKey:TagID" json:"tag,omitempty"`
}

func (EmbeddingQueue) TableName() string {
	return "embedding_queues"
}
