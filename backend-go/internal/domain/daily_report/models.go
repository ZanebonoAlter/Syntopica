package daily_report

import (
	"database/sql/driver"
	"fmt"
	"time"
)

// BoardDailyReport — one report per board per day
type BoardDailyReport struct {
	ID                      uint           `gorm:"primarykey" json:"id"`
	SemanticBoardID         uint           `gorm:"index;not null" json:"semantic_board_id"`
	PeriodDate              time.Time      `gorm:"type:date;not null" json:"period_date"`
	Title                   string         `json:"title"`
	Summary                 string         `json:"summary"`
	Highlights              JSON           `gorm:"type:jsonb" json:"highlights"`
	Dynamics                string         `gorm:"type:text" json:"dynamics"`
	ArticleCount            int            `json:"article_count"`
	EventTagCount           int            `json:"event_tag_count"`
	ClusterCount            int            `json:"cluster_count"`
	Status                  string         `gorm:"size:20;default:generating" json:"status"`
	RawClusters             JSON           `gorm:"type:jsonb" json:"raw_clusters,omitempty"`
	PrevReportID            *uint          `json:"prev_report_id,omitempty"`
	GenerationPromptVersion string         `gorm:"size:20" json:"generation_prompt_version,omitempty"`
	CreatedAt               time.Time      `json:"created_at"`
	UpdatedAt               time.Time      `json:"updated_at"`

	Sections []DailyReportSection `gorm:"foreignKey:ReportID" json:"sections,omitempty"`
}

func (BoardDailyReport) TableName() string {
	return "board_daily_reports"
}

// DailyReportSection — one section per cluster
type DailyReportSection struct {
	ID            uint      `gorm:"primarykey" json:"id"`
	ReportID      uint      `gorm:"index;not null" json:"report_id"`
	ClusterIndex  int       `json:"cluster_index"`
	ClusterLabel  string    `gorm:"size:200" json:"cluster_label"`
	ClusterTagIDs JSON      `gorm:"type:jsonb" json:"cluster_tag_ids"`
	Threads       []DailyReportThread `gorm:"foreignKey:SectionID" json:"threads,omitempty"`
	ArticleCount  int       `json:"article_count"`
	BestTier      int       `gorm:"default:0" json:"best_tier"`
	AvgScore      float64   `gorm:"default:0" json:"avg_score"`
	Status        string    `gorm:"size:20;default:emerging" json:"status"`
	PrevSectionID *uint     `json:"prev_section_id,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
}

func (DailyReportSection) TableName() string {
	return "daily_report_sections"
}

// DailyReportThread — one narrative thread, stored independently
type DailyReportThread struct {
	ID                uint      `gorm:"primarykey" json:"id"`
	ReportID          uint      `gorm:"index;not null" json:"report_id"`
	SectionID         uint      `gorm:"index;not null" json:"section_id"`
	Title             string    `json:"title"`
	Summary           string    `json:"summary"`
	Status            string    `gorm:"size:20;default:emerging" json:"status"`
	TagIDs            JSON      `gorm:"type:jsonb" json:"tag_ids"`
	Confidence        float64   `gorm:"default:0" json:"confidence"`
	PrevThreadID      *uint     `json:"prev_thread_id,omitempty"`
	RelatedArticleIDs JSON      `gorm:"type:jsonb" json:"related_article_ids,omitempty"`
	CreatedAt         time.Time `json:"created_at"`
}

func (DailyReportThread) TableName() string {
	return "daily_report_threads"
}

// JSON is a custom type for GORM jsonb columns.
type JSON []byte

func (j JSON) Value() (driver.Value, error) {
	if j == nil {
		return nil, nil
	}
	return string(j), nil
}

func (j *JSON) Scan(value interface{}) error {
	if value == nil {
		*j = nil
		return nil
	}
	var bytes []byte
	switch v := value.(type) {
	case []byte:
		bytes = v
	case string:
		bytes = []byte(v)
	default:
		return fmt.Errorf("failed to unmarshal JSON value: %v", value)
	}
	*j = append((*j)[0:0], bytes...)
	return nil
}

func (j JSON) MarshalJSON() ([]byte, error) {
	if j == nil {
		return []byte("null"), nil
	}
	return j, nil
}

func (j *JSON) UnmarshalJSON(data []byte) error {
	*j = append((*j)[0:0], data...)
	return nil
}

// TagInput mirrors narrative.TagInput for use in the daily report pipeline.
type TagInput struct {
	ID           uint    `json:"id"`
	Label        string  `json:"label"`
	Category     string  `json:"category"`
	Description  string  `json:"description"`
	ArticleCount int     `json:"article_count"`
	Source       string  `json:"source"`
	MatchReason  string  `json:"match_reason"`
	Score        float64 `json:"score"`
}

// ClusterGroup represents a group of tags clustered by the LLM.
type ClusterGroup struct {
	GroupName string `json:"group_name"`
	TagIDs    []uint `json:"tag_ids"`
}

// Highlight represents a key highlight in the daily report.
type Highlight struct {
	Title  string `json:"title"`
	Reason string `json:"reason"`
	TagIDs []uint `json:"tag_ids"`
}

// Thread represents a narrative thread within a cluster section.
type Thread struct {
	Title             string  `json:"title"`
	Summary           string  `json:"summary"`
	Status            string  `json:"status"` // emerging, continuing, splitting, merging, ending
	TagIDs            []uint  `json:"tag_ids"`
	Confidence        float64 `json:"confidence"`
	PrevThreadID      *uint   `json:"prev_thread_id,omitempty"`
	RelatedArticleIDs []uint  `json:"related_article_ids,omitempty"`
}
