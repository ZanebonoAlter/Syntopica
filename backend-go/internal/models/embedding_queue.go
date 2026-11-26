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
	CreatedAt    time.Time  `gorm:"index" json:"created_at"`
	StartedAt    *time.Time `json:"started_at"`
	CompletedAt  *time.Time `json:"completed_at"`
	// NextRetryAt 推迟可被重新 claim 的时刻（speed-up-tag-embedding D4：
	// markFailed 指数退避 30s*2^(n-1)，retry_count<3 时回 pending；到期前
	// claim 过滤不领取）。可空列，AutoMigrate 部署时自动加列，无回填。
	NextRetryAt *time.Time `json:"next_retry_at"`
	Tag         *TopicTag  `gorm:"foreignKey:TagID" json:"tag,omitempty"`
}

func (EmbeddingQueue) TableName() string {
	return "embedding_queues"
}
