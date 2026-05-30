package models

import (
	"time"
)

type Feed struct {
	ID              uint       `gorm:"primaryKey" json:"id"`
	Title           string     `gorm:"size:200;not null" json:"title"`
	Description     string     `gorm:"type:text" json:"description"`
	URL             string     `gorm:"size:500;unique;not null" json:"url"`
	CategoryID      *uint      `gorm:"index" json:"category_id"`
	Icon            string     `gorm:"size:1000;default:rss" json:"icon"`
	Color           string     `gorm:"size:20;default:#8b5cf6" json:"color"`
	LastUpdated     *time.Time `json:"last_updated"`
	CreatedAt       time.Time  `json:"created_at"`
	MaxArticles     int        `gorm:"default:100" json:"max_articles"`
	RefreshInterval int        `gorm:"default:60" json:"refresh_interval"`
	RefreshStatus   string     `gorm:"size:20;default:idle" json:"refresh_status"`
	RefreshError    string     `gorm:"type:text" json:"refresh_error"`
	LastRefreshAt   *time.Time `json:"last_refresh_at"`

	ArticleSummaryEnabled bool      `gorm:"default:false" json:"article_summary_enabled"`
	CompletionOnRefresh   bool      `gorm:"default:true" json:"completion_on_refresh"`
	MaxCompletionRetries  int       `gorm:"default:3" json:"max_completion_retries"`
	FirecrawlEnabled      bool      `gorm:"default:false" json:"firecrawl_enabled"`
	TaggingEnabled        bool      `gorm:"default:true" json:"tagging_enabled"`
	Articles              []Article `gorm:"foreignKey:FeedID;constraint:OnDelete:CASCADE" json:"articles,omitempty"`
	Category              *Category `gorm:"foreignKey:CategoryID" json:"category,omitempty"`
}

func (Feed) TableName() string {
	return "feeds"
}

type FeedStats struct {
	ArticleCount int
	UnreadCount  int
}

func (f *Feed) ToDict(stats *FeedStats) map[string]interface{} {
	data := map[string]interface{}{
		"id":               f.ID,
		"title":            f.Title,
		"description":      f.Description,
		"url":              f.URL,
		"category_id":      f.CategoryID,
		"icon":             f.Icon,
		"color":            f.Color,
		"last_updated":     FormatDatetimeCSTPtr(f.LastUpdated),
		"created_at":       FormatDatetimeCST(f.CreatedAt),
		"max_articles":     f.MaxArticles,
		"refresh_interval": f.RefreshInterval,
		"refresh_status":   f.RefreshStatus,
		"refresh_error":    f.RefreshError,
		"last_refresh_at":  FormatDatetimeCSTPtr(f.LastRefreshAt),

		"article_summary_enabled": f.ArticleSummaryEnabled,
		"completion_on_refresh":   f.CompletionOnRefresh,
		"max_completion_retries":  f.MaxCompletionRetries,
		"firecrawl_enabled":       f.FirecrawlEnabled,
		"tagging_enabled":         f.TaggingEnabled,
	}

	if stats != nil {
		data["article_count"] = stats.ArticleCount
		data["unread_count"] = stats.UnreadCount
	}

	return data
}
