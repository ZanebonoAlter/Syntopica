package models

import (
	"strings"
	"time"
)

// 判定状态取值（add-ai-generated-article-detection design D14 命名契约，前后端共用）。
// 判定状态为「未判定」的文章不对外呈现任何疑似标记（无占位文案）。
const (
	// AIDetectionStatusUnknown 未判定（存量文章与判定失败的文章都停在这里）。
	AIDetectionStatusUnknown = "unknown"
	// AIDetectionStatusSuspect 疑似 AI 生成（置信分达到阈值）。
	AIDetectionStatusSuspect = "suspect"
	// AIDetectionStatusNotAI 已确认非 AI（置信分低于阈值）。
	AIDetectionStatusNotAI = "not_ai"
)

// 判定来源取值（design D14）。
const (
	// AIDetectionSourceLLM 自动判定。
	AIDetectionSourceLLM = "llm"
	// AIDetectionSourceManual 人工标记（人工结论优先，自动判定不得覆盖）。
	AIDetectionSourceManual = "manual"
)

// NormalizeAIDetectionStatus 把空串归一为「未判定」。
// 未落库的文章实体零值是空串，投影到接口前应走本函数，避免对外出现非契约值。
func NormalizeAIDetectionStatus(status string) string {
	if strings.TrimSpace(status) == "" {
		return AIDetectionStatusUnknown
	}
	return status
}

type Article struct {
	ID          uint       `gorm:"primaryKey" json:"id"`
	FeedID      uint       `gorm:"index;not null" json:"feed_id"`
	CategoryID  *uint      `gorm:"-" json:"category_id"`
	Title       string     `gorm:"size:500;not null" json:"title"`
	Description string     `gorm:"type:text" json:"description"`
	Content     string     `gorm:"type:text" json:"content"`
	Link        string     `gorm:"size:1000;index" json:"link"`
	ImageURL    string     `gorm:"size:1000" json:"image_url"`
	PubDate     *time.Time `json:"pub_date"`
	Author      string     `gorm:"size:200" json:"author"`
	Read        bool       `gorm:"default:false" json:"read"`
	Favorite    bool       `gorm:"default:false" json:"favorite"`
	// Archived marks an article as aged out of the feed's active window
	// (MaxArticles). Archived rows keep all text fields readable (daily report
	// threads reference them by ID) but are excluded from reader lists/stats,
	// search (search_vector nulled), and have their topic tags / reading
	// behaviors removed. See article-retention spec.
	Archived                   bool       `gorm:"default:false" json:"archived"`
	SummaryStatus              string     `gorm:"size:20;default:complete" json:"summary_status"`
	SummaryGeneratedAt         *time.Time `json:"summary_generated_at"`
	SummaryProcessingStartedAt *time.Time `json:"summary_processing_started_at"`

	CompletionAttempts int    `gorm:"default:0" json:"completion_attempts"`
	CompletionError    string `gorm:"type:text" json:"completion_error"`
	// ContentForm marks the article's content form judged during summary
	// generation: "mono" (single-topic) or "aggregate" (multi-topic digest).
	// Empty means the form is unknown (legacy articles or model did not output
	// the mark) and downstream flows fall back to the mono path.
	ContentForm      string `gorm:"size:20" json:"content_form"`
	AIContentSummary string `gorm:"type:text" json:"ai_content_summary"`
	// AIDetection* 是「疑似 AI 生成」判定结论四元组（add-ai-generated-article-detection design D6/D14）。
	// 判定状态 unknown（未判定）/ suspect（疑似）/ not_ai（已确认非 AI）；置信分 0-1；
	// 判定来源 llm（自动判定）/ manual（人工标记，空=尚无来源）；判定时间。
	// 人工结论优先：来源为 manual 时自动判定不得覆盖（除非用户显式触发重新判定）。
	// 存量文章判定状态一律为「未判定」，界面不展示任何疑似标记。
	AIDetectionStatus  string     `gorm:"column:ai_detection_status;size:20;default:unknown" json:"ai_detection_status"`
	AIDetectionScore   float64    `gorm:"column:ai_detection_score;default:0" json:"ai_detection_score"`
	AIDetectionSource  string     `gorm:"column:ai_detection_source;size:20" json:"ai_detection_source"`
	AIDetectionAt      *time.Time `gorm:"column:ai_detection_at" json:"ai_detection_at"`
	FirecrawlStatus    string     `gorm:"size:20;default:pending" json:"firecrawl_status"`
	FirecrawlError     string     `gorm:"type:text" json:"firecrawl_error"`
	FirecrawlContent   string     `gorm:"type:text" json:"firecrawl_content"`
	FirecrawlCrawledAt *time.Time `json:"firecrawl_crawled_at"`
	CreatedAt          time.Time  `json:"created_at"`
	TagCount           int        `gorm:"->;column:tag_count" json:"tag_count"`
	RelevanceScore     float64    `gorm:"->;column:relevance_score" json:"relevance_score"`
	Feed               Feed       `gorm:"foreignKey:FeedID" json:"feed,omitempty"`
}

func (Article) TableName() string {
	return "articles"
}

func (a *Article) ToDict() map[string]interface{} {
	return map[string]interface{}{
		"id":                            a.ID,
		"feed_id":                       a.FeedID,
		"category_id":                   a.CategoryID,
		"title":                         a.Title,
		"description":                   a.Description,
		"content":                       a.Content,
		"link":                          a.Link,
		"image_url":                     a.ImageURL,
		"pub_date":                      FormatDatetimeCSTPtr(a.PubDate),
		"author":                        a.Author,
		"read":                          a.Read,
		"favorite":                      a.Favorite,
		"summary_status":                a.SummaryStatus,
		"summary_generated_at":          FormatDatetimeCSTPtr(a.SummaryGeneratedAt),
		"summary_processing_started_at": FormatDatetimeCSTPtr(a.SummaryProcessingStartedAt),

		"completion_attempts": a.CompletionAttempts,
		"completion_error":    a.CompletionError,
		"ai_content_summary":  a.AIContentSummary,
		// 疑似 AI 判定四元组（design D14）：详情是唯一给出判定来源与判定时间的读路径。
		"ai_detection_status":  NormalizeAIDetectionStatus(a.AIDetectionStatus),
		"ai_detection_score":   a.AIDetectionScore,
		"ai_detection_source":  a.AIDetectionSource,
		"ai_detection_at":      FormatDatetimeCSTPtr(a.AIDetectionAt),
		"firecrawl_status":     a.FirecrawlStatus,
		"firecrawl_error":      a.FirecrawlError,
		"firecrawl_content":    a.FirecrawlContent,
		"firecrawl_crawled_at": FormatDatetimeCSTPtr(a.FirecrawlCrawledAt),
		"created_at":           FormatDatetimeCST(a.CreatedAt),
		"tag_count":            a.TagCount,
		"relevance_score":      a.RelevanceScore,
	}
}

// ToListDict returns the narrow payload for GET /api/articles list rows.
// It intentionally omits the body fields (description/content/firecrawl_content/
// ai_content_summary) — those are served only by GET /api/articles/:id.
// excerpt is the pre-computed plain-text lede (see handler.buildExcerpt).
func (a *Article) ToListDict(excerpt string) map[string]interface{} {
	return map[string]interface{}{
		"id":                   a.ID,
		"feed_id":              a.FeedID,
		"category_id":          a.CategoryID,
		"title":                a.Title,
		"link":                 a.Link,
		"image_url":            a.ImageURL,
		"pub_date":             FormatDatetimeCSTPtr(a.PubDate),
		"author":               a.Author,
		"read":                 a.Read,
		"favorite":             a.Favorite,
		"archived":             a.Archived,
		"summary_status":       a.SummaryStatus,
		"summary_generated_at": FormatDatetimeCSTPtr(a.SummaryGeneratedAt),
		"firecrawl_status":     a.FirecrawlStatus,
		"firecrawl_error":      a.FirecrawlError,
		"firecrawl_crawled_at": FormatDatetimeCSTPtr(a.FirecrawlCrawledAt),
		"completion_error":     a.CompletionError,
		"created_at":           FormatDatetimeCST(a.CreatedAt),
		"tag_count":            a.TagCount,
		"relevance_score":      a.RelevanceScore,
		// 疑似 AI 标记是列表行允许的非正文类小字段（reading.md 约束 9 末句 +
		// article-list-projection spec）：只给判定状态，未判定也带该键（值 unknown），
		// 不携带判定理由/样本等正文类内容。
		"ai_detection_status": NormalizeAIDetectionStatus(a.AIDetectionStatus),
		"excerpt":             excerpt,
	}
}
