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
	CreatedAt      time.Time  `gorm:"index" json:"created_at"`
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
	CreatedAt            time.Time  `gorm:"index" json:"created_at"`
	UpdatedAt            time.Time  `json:"updated_at"`
	Article              *Article   `gorm:"foreignKey:ArticleID;constraint:OnDelete:CASCADE" json:"article,omitempty"`
}

func (TagJob) TableName() string {
	return "tag_jobs"
}

// DetectionJob 是「疑似 AI 生成」判定作业（add-ai-generated-article-detection design D1）。
// 骨架与 TagJob 同构：一篇一作业、乐观锁 claim + 租约、重试上限；与标签打标队列
// 相互分离——判定失败不影响打标（spec「判定失败不影响打标」）。
// 判定作业不攒批：一次判定 = 一篇文章 = 一次 LLM 调用（design D2）。
type DetectionJob struct {
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
	// Reason 记录判定入队来源（firecrawl_completed / firecrawl_failed / article_ingested
	// / manual_recheck），便于运维回溯某篇判定从哪个内容就绪节点进来。
	Reason    string    `gorm:"size:50" json:"reason"`
	CreatedAt time.Time `gorm:"index" json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Article   *Article  `gorm:"foreignKey:ArticleID;constraint:OnDelete:CASCADE" json:"article,omitempty"`
}

func (DetectionJob) TableName() string {
	return "detection_jobs"
}
