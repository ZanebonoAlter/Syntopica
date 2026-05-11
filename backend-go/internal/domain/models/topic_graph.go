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

// TagCategoryMeta defines default display properties for each category
type TagCategoryMeta struct {
	Key         string // category key: event, person, keyword
	Label       string // display label: 事件, 人物, 关键词
	DefaultIcon string // Iconify icon id
	Color       string // default color for nodes/badges
}

// DefaultTagCategories returns the standard category definitions
func DefaultTagCategories() []TagCategoryMeta {
	return []TagCategoryMeta{
		{Key: TagCategoryEvent, Label: "事件", DefaultIcon: "mdi:calendar-star", Color: "#f59e0b"},
		{Key: TagCategoryPerson, Label: "人物", DefaultIcon: "mdi:account", Color: "#10b981"},
		{Key: TagCategoryKeyword, Label: "关键词", DefaultIcon: "mdi:tag", Color: "#6366f1"},
	}
}

// GetCategoryMeta returns the metadata for a category key
func GetCategoryMeta(category string) TagCategoryMeta {
	for _, meta := range DefaultTagCategories() {
		if meta.Key == category {
			return meta
		}
	}
	// Default to keyword
	return TagCategoryMeta{Key: TagCategoryKeyword, Label: "关键词", DefaultIcon: "mdi:tag", Color: "#6366f1"}
}

// TopicTag represents a tag extracted from AI summaries
// Tags are categorized into event, person, or keyword
type TopicTag struct {
	ID           uint        `gorm:"primaryKey" json:"id"`
	Slug         string      `gorm:"size:120;not null;index:idx_topic_tags_category_slug" json:"slug"`
	Label        string      `gorm:"size:160;not null" json:"label"`
	Category     string      `gorm:"size:20;not null;default:keyword;index:idx_topic_tags_category_slug" json:"category"` // event, person, keyword
	Icon         string      `gorm:"size:100" json:"icon"`                                                                // Iconify icon id, overrides category default
	Aliases      string      `gorm:"type:text" json:"aliases"`                                                            // JSON array of alias strings
	Description  string      `gorm:"type:text" json:"description"`                                                        // LLM-generated tag description
	IsCanonical  bool        `gorm:"default:false" json:"is_canonical"`                                                   // true if this is a canonical tag (not merged)
	Source       string      `gorm:"size:20;default:llm" json:"source"`                                                   // llm, heuristic, manual
	FeedCount    int         `gorm:"default:0" json:"feed_count"`                                                         // distinct feed count referencing this tag
	Status       string      `gorm:"size:20;not null;default:active;index" json:"status"`                                 // active, merged
	MergedIntoID *uint       `gorm:"index" json:"merged_into_id,omitempty"`                                               // points to target tag when merged
	IsWatched    bool        `gorm:"default:false" json:"is_watched"`                                                     // user-watched tag for feed filtering
	WatchedAt    *time.Time  `json:"watched_at,omitempty"`                                                                // when the tag was watched
	QualityScore float64     `gorm:"default:0" json:"quality_score"`
	Metadata     MetadataMap `gorm:"type:jsonb;serializer:json;default:'{}'" json:"metadata,omitempty"`
	CreatedAt    time.Time   `json:"created_at"`
	UpdatedAt    time.Time   `json:"updated_at"`

	// Deprecated: Use Category instead.
	// Kind is retained for backward compatibility, maps to Category.
	Kind string `gorm:"size:20;default:keyword" json:"kind"`

	// Deprecated: Each tag now has multiple embeddings (identity + semantic).
	// Use direct queries on topic_tag_embeddings with embedding_type filter instead.
	Embedding  *TopicTagEmbedding  `gorm:"foreignKey:TopicTagID" json:"embedding,omitempty"`
	Embeddings []TopicTagEmbedding `gorm:"foreignKey:TopicTagID" json:"embeddings,omitempty"`
	MergedInto *TopicTag           `gorm:"foreignKey:MergedIntoID" json:"merged_into,omitempty"`
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

// TopicTagEmbedding stores vector embeddings for tag similarity matching
type TopicTagEmbedding struct {
	ID            uint      `gorm:"primaryKey" json:"id"`
	TopicTagID    uint      `gorm:"not null;uniqueIndex:idx_topic_tag_embeddings_tag_type" json:"topic_tag_id"`
	EmbeddingType string    `gorm:"size:20;not null;default:identity;uniqueIndex:idx_topic_tag_embeddings_tag_type" json:"embedding_type"`
	Vector        string    `gorm:"type:text;not null" json:"vector"` // Deprecated: legacy JSON text payload. Use EmbeddingVec for pgvector.
	EmbeddingVec  string    `gorm:"type:vector;column:embedding" json:"-"`
	Dimension     int       `gorm:"not null" json:"dimension"`     // Vector dimension (e.g., 1536 for ada-002)
	Model         string    `gorm:"size:50;not null" json:"model"` // Model used: "text-embedding-ada-002"
	TextHash      string    `gorm:"size:64" json:"text_hash"`      // Hash of (label + aliases + category) for re-embedding detection
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`

	TopicTag *TopicTag `gorm:"foreignKey:TopicTagID;constraint:OnDelete:CASCADE" json:"topic_tag,omitempty"`
}

// TableName specifies the table name for TopicTagEmbedding
func (TopicTagEmbedding) TableName() string {
	return "topic_tag_embeddings"
}

// ArticleTopicTag represents the many-to-many relationship between articles and tags
// This allows individual articles to be tagged for more granular topic tracking
type ArticleTopicTag struct {
	ID         uint      `gorm:"primaryKey" json:"id"`
	ArticleID  uint      `gorm:"index:idx_article_topic_tag_article;uniqueIndex:idx_article_topic_tags_link;not null" json:"article_id"`
	TopicTagID uint      `gorm:"index:idx_article_topic_tag_topic;uniqueIndex:idx_article_topic_tags_link;not null" json:"topic_tag_id"`
	Score      float64   `gorm:"default:0" json:"score"`
	Source     string    `gorm:"size:20;default:llm" json:"source"` // llm, heuristic, manual
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
