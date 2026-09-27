package models

import "time"

// TopicAnalysisCursor 分析游标（用于增量更新）——迁自 internal/models
// （decouple-backend-domains D3：topicgraph 域独占）。
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
