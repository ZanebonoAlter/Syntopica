package models

import (
	"time"
)

type ReadingBehavior struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	ArticleID   uint      `gorm:"index;not null" json:"article_id"`
	FeedID      uint      `gorm:"index" json:"feed_id"`
	CategoryID  *uint     `gorm:"index" json:"category_id"`
	SessionID   string    `gorm:"size:100;index" json:"session_id"`
	EventType   string    `gorm:"size:20;index" json:"event_type"`
	ScrollDepth int       `gorm:"default:0" json:"scroll_depth"`
	ReadingTime int       `gorm:"default:0" json:"reading_time"`
	CreatedAt   time.Time `gorm:"index" json:"created_at"`
	Article     *Article  `gorm:"foreignKey:ArticleID" json:"article,omitempty"`
	Feed        *Feed     `gorm:"foreignKey:FeedID" json:"feed,omitempty"`
}

func (r *ReadingBehavior) ToDict() map[string]interface{} {
	return map[string]interface{}{
		"id":           r.ID,
		"article_id":   r.ArticleID,
		"feed_id":      r.FeedID,
		"category_id":  r.CategoryID,
		"session_id":   r.SessionID,
		"event_type":   r.EventType,
		"scroll_depth": r.ScrollDepth,
		"reading_time": r.ReadingTime,
		"created_at":   FormatDatetimeCST(r.CreatedAt),
	}
}
