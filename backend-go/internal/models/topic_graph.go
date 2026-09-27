package models

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"time"
)

// TagCategory constants define the supported tag categories
const (
	TagCategoryEvent   = "event"   // 时间相关的事件，如发布会、版本更新
	TagCategoryPerson  = "person"  // 具体人物
	TagCategoryKeyword = "keyword" // 关键词，兜底类别（组织、产品、概念等）
)

// TopicTag represents a tag extracted from AI summaries
// Tags are categorized into event, person, or keyword
type TopicTag struct {
	ID           uint        `gorm:"primaryKey" json:"id"`
	Slug         string      `gorm:"size:120;index:idx_topic_tags_category_slug" json:"slug"`
	Label        string      `gorm:"size:160" json:"label"`
	Category     string      `gorm:"size:20;index:idx_topic_tags_category_slug" json:"category"` // event, person, keyword
	Icon         string      `gorm:"size:100" json:"icon"`                                       // Iconify icon id, overrides category default
	Aliases      string      `gorm:"type:text" json:"aliases"`                                   // JSON array of alias strings
	Description  string      `gorm:"type:text" json:"description"`                               // LLM-generated tag description
	IsCanonical  bool        `json:"is_canonical"`                                               // true if this is a canonical tag (not merged)
	Source       string      `gorm:"size:20" json:"source"`                                      // llm, heuristic, manual
	FeedCount    int         `json:"feed_count"`                                                 // distinct feed count referencing this tag
	Status       string      `gorm:"size:20;default:active;index" json:"status"`                 // active, merged
	MergedIntoID *uint       `gorm:"index" json:"merged_into_id,omitempty"`                      // points to target tag when merged
	IsWatched    bool        `json:"is_watched"`                                                 // user-watched tag for feed filtering
	WatchedAt    *time.Time  `json:"watched_at,omitempty"`                                       // when the tag was watched
	QualityScore float64     `json:"quality_score"`
	Metadata     MetadataMap `gorm:"type:jsonb;serializer:json;default:'{}'" json:"metadata,omitempty"`
	CreatedAt    time.Time   `json:"created_at"`
	UpdatedAt    time.Time   `json:"updated_at"`

	// Deprecated: Kind is no longer written. Use Category as the authoritative field.
	// This field is retained for backward compatibility and will be removed with a DB migration.
	Kind string `gorm:"size:20" json:"kind"`

	// Deprecated embedding associations (Embedding/Embeddings []TopicTagEmbedding)
	// removed in decouple-backend-domains: zero consumers (no Preload, no field
	// access in production); association fields carry no DDL. Query
	// topic_tag_embeddings directly with embedding_type filter.
	MergedInto *TopicTag `gorm:"foreignKey:MergedIntoID" json:"merged_into,omitempty"`
}

type MetadataMap map[string]any

func (m MetadataMap) Value() (driver.Value, error) {
	if m == nil {
		return "{}", nil
	}
	b, err := json.Marshal(m)
	if err != nil {
		return nil, err
	}
	return string(b), nil
}

func (m *MetadataMap) Scan(value any) error {
	if value == nil {
		*m = MetadataMap{}
		return nil
	}

	var data []byte
	switch v := value.(type) {
	case []byte:
		data = v
	case string:
		data = []byte(v)
	default:
		return fmt.Errorf("scan metadata map: unsupported value type %T", value)
	}

	if len(data) == 0 {
		*m = MetadataMap{}
		return nil
	}
	return json.Unmarshal(data, m)
}

func (TopicTag) TableName() string {
	return "topic_tags"
}

// ArticleTopicTag represents the many-to-many relationship between articles and tags
// This allows individual articles to be tagged for more granular topic tracking
type ArticleTopicTag struct {
	ID         uint      `gorm:"primaryKey" json:"id"`
	ArticleID  uint      `gorm:"index:idx_article_topic_tag_article;uniqueIndex:idx_article_topic_tags_link" json:"article_id"`
	TopicTagID uint      `gorm:"index:idx_article_topic_tag_topic;uniqueIndex:idx_article_topic_tags_link" json:"topic_tag_id"`
	Score      float64   `json:"score"`
	Source     string    `gorm:"size:20" json:"source"` // llm, heuristic, manual
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`

	// Relations
	Article  *Article  `gorm:"foreignKey:ArticleID;constraint:OnDelete:CASCADE" json:"article,omitempty"`
	TopicTag *TopicTag `gorm:"foreignKey:TopicTagID;constraint:OnDelete:CASCADE" json:"topic_tag,omitempty"`
}

// TableName specifies the table name for ArticleTopicTag
func (ArticleTopicTag) TableName() string {
	return "article_topic_tags"
}
