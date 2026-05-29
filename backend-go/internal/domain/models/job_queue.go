package models

import "time"

type JobStatus string

const (
	JobStatusPending   JobStatus = "pending"
	JobStatusLeased    JobStatus = "leased"
	JobStatusCompleted JobStatus = "completed"
	JobStatusFailed    JobStatus = "failed"
)

type FirecrawlJob struct {
	ID             uint       `gorm:"primaryKey" json:"id"`
	ArticleID      uint       `gorm:"index;not null" json:"article_id"`
	Status         string     `gorm:"size:20;index;not null;default:pending" json:"status"`
	Priority       int        `gorm:"default:0;index" json:"priority"`
	AttemptCount   int        `gorm:"default:0" json:"attempt_count"`
	MaxAttempts    int        `gorm:"default:5" json:"max_attempts"`
	AvailableAt    time.Time  `gorm:"index;not null" json:"available_at"`
	LeasedAt       *time.Time `json:"leased_at"`
	LeaseExpiresAt *time.Time `gorm:"index" json:"lease_expires_at"`
	LastError      string     `gorm:"type:text" json:"last_error"`
	URLSnapshot    string     `gorm:"size:1000" json:"url_snapshot"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
	Article        *Article   `gorm:"foreignKey:ArticleID;constraint:OnDelete:CASCADE" json:"article,omitempty"`
}

func (FirecrawlJob) TableName() string {
	return "firecrawl_jobs"
}

type TagJob struct {
	ID                   uint       `gorm:"primaryKey" json:"id"`
	ArticleID            uint       `gorm:"index;not null" json:"article_id"`
	Status               string     `gorm:"size:20;index;not null;default:pending" json:"status"`
	Priority             int        `gorm:"default:0;index" json:"priority"`
	AttemptCount         int        `gorm:"default:0" json:"attempt_count"`
	MaxAttempts          int        `gorm:"default:5" json:"max_attempts"`
	AvailableAt          time.Time  `gorm:"index;not null" json:"available_at"`
	LeasedAt             *time.Time `json:"leased_at"`
	LeaseExpiresAt       *time.Time `gorm:"index" json:"lease_expires_at"`
	LastError            string     `gorm:"type:text" json:"last_error"`
	FeedNameSnapshot     string     `gorm:"size:200" json:"feed_name_snapshot"`
	CategoryNameSnapshot string     `gorm:"size:100" json:"category_name_snapshot"`
	ForceRetag           bool       `gorm:"default:false" json:"force_retag"`
	Reason               string     `gorm:"size:50" json:"reason"`
	CreatedAt            time.Time  `json:"created_at"`
	UpdatedAt            time.Time  `json:"updated_at"`
	Article              *Article   `gorm:"foreignKey:ArticleID;constraint:OnDelete:CASCADE" json:"article,omitempty"`
}

func (TagJob) TableName() string {
	return "tag_jobs"
}
