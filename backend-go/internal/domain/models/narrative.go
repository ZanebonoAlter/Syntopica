package models

import "time"

type NarrativeSummary struct {
	ID                uint64         `gorm:"primaryKey" json:"id"`
	Title             string         `gorm:"size:300;not null" json:"title"`
	Summary           string         `gorm:"type:text;not null" json:"summary"`
	Status            string         `gorm:"size:20;not null;index" json:"status"`
	Period            string         `gorm:"size:20;not null;default:daily" json:"period"`
	PeriodDate        time.Time      `gorm:"index:idx_narrative_period_date;not null" json:"period_date"`
	Generation        int            `gorm:"not null;default:0" json:"generation"`
	ParentIDs         string         `gorm:"type:text" json:"parent_ids"`
	RelatedTagIDs     string         `gorm:"type:text" json:"related_tag_ids"`
	RelatedArticleIDs string         `gorm:"type:text" json:"related_article_ids"`
	Source            string         `gorm:"size:20;default:ai" json:"source"`
	ScopeType         string         `gorm:"size:20;not null;default:global" json:"scope_type"`
	ScopeCategoryID   *uint          `gorm:"index:idx_narrative_scope" json:"scope_category_id"`
	ScopeLabel        string         `gorm:"size:100" json:"scope_label"`
	BoardID           *uint          `gorm:"index" json:"board_id"`
	Board             NarrativeBoard `gorm:"foreignKey:BoardID" json:"board,omitempty"`
	CreatedAt         time.Time      `json:"created_at"`
	UpdatedAt         time.Time      `json:"updated_at"`
}

func (NarrativeSummary) TableName() string {
	return "narrative_summaries"
}

const (
	NarrativeStatusEmerging   = "emerging"
	NarrativeStatusContinuing = "continuing"
	NarrativeStatusSplitting  = "splitting"
	NarrativeStatusMerging    = "merging"
	NarrativeStatusEnding     = "ending"

	NarrativeScopeTypeGlobal       = "global"
	NarrativeScopeTypeFeedCategory = "feed_category"
)
