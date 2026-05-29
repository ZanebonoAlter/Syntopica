package models

import (
	"time"
)

type UserPreference struct {
	ID                uint       `gorm:"primaryKey" json:"id"`
	FeedID            *uint      `gorm:"index" json:"feed_id"`
	CategoryID        *uint      `gorm:"index" json:"category_id"`
	PreferenceScore   float64    `gorm:"default:0" json:"preference_score"`
	AvgReadingTime    int        `gorm:"default:0" json:"avg_reading_time"`
	InteractionCount  int        `gorm:"default:0" json:"interaction_count"`
	ScrollDepthAvg    float64    `gorm:"default:0" json:"scroll_depth_avg"`
	LastInteractionAt *time.Time `gorm:"index" json:"last_interaction_at"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
	Feed              *Feed      `gorm:"foreignKey:FeedID" json:"feed,omitempty"`
	Category          *Category  `gorm:"foreignKey:CategoryID" json:"category,omitempty"`
}

func (u *UserPreference) ToDict() map[string]interface{} {
	feedTitle := ""
	if u.Feed != nil {
		feedTitle = u.Feed.Title
	}

	categoryName := ""
	if u.Category != nil {
		categoryName = u.Category.Name
	}

	return map[string]interface{}{
		"id":                  u.ID,
		"feed_id":             u.FeedID,
		"category_id":         u.CategoryID,
		"preference_score":    u.PreferenceScore,
		"avg_reading_time":    u.AvgReadingTime,
		"interaction_count":   u.InteractionCount,
		"scroll_depth_avg":    u.ScrollDepthAvg,
		"last_interaction_at": FormatDatetimeCSTPtr(u.LastInteractionAt),
		"created_at":          FormatDatetimeCST(u.CreatedAt),
		"updated_at":          FormatDatetimeCST(u.UpdatedAt),
		"feed_title":          feedTitle,
		"category_name":       categoryName,
	}
}
