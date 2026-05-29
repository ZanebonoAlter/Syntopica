package models

import "time"

// TopicTagAnalysis 主题标签分析快照
type TopicTagAnalysis struct {
	ID           uint64    `gorm:"primaryKey" json:"id"`
	TopicTagID   uint64    `gorm:"index:idx_tag_analysis_date,unique" json:"topic_tag_id"`
	AnalysisType string    `gorm:"index:idx_tag_analysis_date,unique" json:"analysis_type"` // event, person, keyword
	WindowType   string    `gorm:"index:idx_tag_analysis_date,unique" json:"window_type"`   // daily, weekly
	AnchorDate   time.Time `gorm:"index:idx_tag_analysis_date,unique" json:"anchor_date"`
	ArticleCount int       `json:"article_count"`
	PayloadJSON  string    `gorm:"type:text" json:"payload_json"` // 首批 Postgres 切换阶段继续保留为文本 JSON 载荷，不做结构化拆分。
	Source       string    `json:"source"`                        // ai, heuristic, cached
	Version      int       `json:"version"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// TopicAnalysisCursor 分析游标（用于增量更新）
type TopicAnalysisCursor struct {
	ID            uint64 `gorm:"primaryKey"`
	TopicTagID    uint64 `gorm:"uniqueIndex:idx_cursor_tag_type_window"`
	AnalysisType  string `gorm:"uniqueIndex:idx_cursor_tag_type_window"`
	WindowType    string `gorm:"uniqueIndex:idx_cursor_tag_type_window"`
	LastArticleID uint64
	LastUpdatedAt time.Time
	CreatedAt     time.Time
	UpdatedAt     time.Time
}
