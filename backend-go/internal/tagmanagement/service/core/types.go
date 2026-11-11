package core

type ExtractionInput struct {
	Title        string
	Summary      string
	FeedName     string
	CategoryName string
	ArticleID    *uint
	SummaryID    *uint
	PubDate      string
}

// TopicTag represents a tag extracted from AI summaries
// Used for API responses and internal processing
type AuxiliaryLabel struct {
	Label       string `json:"label"`
	Description string `json:"description"`
}

// EventFingerprint is the structured identity of an event tag
// (add-event-story-heat): subject / subject_key / action / object plus an
// optional occurred_at. Produced inside the same extraction call as the tag
// itself; keyword/person tags never carry one. Incomplete fingerprints never
// block the tag — downstream identity judgment treats them as untrusted.
type EventFingerprint struct {
	Subject    string `json:"subject,omitempty"`     // 主体原文（展示用）
	SubjectKey string `json:"subject_key,omitempty"` // 归一化实体键，如 person:trump / company:bytedance
	Action     string `json:"action,omitempty"`      // 动作动词短语（发布/宣布/涨价/回应/下架/召回）
	Object     string `json:"object,omitempty"`      // 动作对象（产品/政策/资产/事项）
	OccurredAt string `json:"occurred_at,omitempty"` // 发生日期 YYYY-MM-DD，可空
}

// Complete reports whether the four identity fields are all non-empty
// (occurred_at is optional). A nil fingerprint is never complete.
func (f *EventFingerprint) Complete() bool {
	if f == nil {
		return false
	}
	return f.Subject != "" && f.SubjectKey != "" && f.Action != "" && f.Object != ""
}

type TopicTag struct {
	ID              uint              `json:"id,omitempty"`
	Label           string            `json:"label"`
	Slug            string            `json:"slug"`
	Category        string            `json:"category"`              // event, person, keyword
	Icon            string            `json:"icon,omitempty"`        // Iconify icon id
	Aliases         []string          `json:"aliases,omitempty"`     // Alternative names
	Description     string            `json:"description,omitempty"` // LLM-generated tag description
	AuxiliaryLabels []AuxiliaryLabel  `json:"auxiliary_labels,omitempty"`
	Fingerprint     *EventFingerprint `json:"fingerprint,omitempty"` // 仅 event 标签：结构化事件指纹
	// SubjectDesc — 主体稳定短身份说明（normalize-event-subject-entities
	// D2）：仅影子态第一轮同次调用产出、仅 event 携带。不进 event_fingerprint，
	// 落 topic_tags.metadata.entity_subject 独立惰性键。off 态恒空。
	SubjectDesc  string   `json:"subject_desc,omitempty"`
	Score        float64  `json:"score"`
	IsNew        bool     `json:"is_new,omitempty"`      // True if newly created
	MatchedTo    uint     `json:"matched_to,omitempty"`  // ID of existing tag if matched
	Kind         string   `json:"kind,omitempty"`        // Legacy: maps to Category for backward compat
	FeedCount    int      `json:"feed_count,omitempty"`  // Distinct feed count referencing this tag
	IsAbstract   bool     `json:"is_abstract,omitempty"` // True if this is an abstract (parent) tag
	ChildSlugs   []string `json:"child_slugs,omitempty"` // Slugs of child tags (only for abstract tags)
	QualityScore float64  `json:"quality_score,omitempty"`
	IsLowQuality bool     `json:"is_low_quality,omitempty"`
	IsWatched    bool     `json:"is_watched,omitempty"`
	ArticleCount int      `json:"article_count,omitempty"`
}

type AggregatedTopicTag struct {
	Slug         string   `json:"slug"`
	Label        string   `json:"label"`
	Category     string   `json:"category"`
	Kind         string   `json:"kind,omitempty"`
	Icon         string   `json:"icon,omitempty"`
	Aliases      []string `json:"aliases,omitempty"`
	Score        float64  `json:"score"`
	ArticleCount int      `json:"article_count"`
	FeedCount    int      `json:"feed_count,omitempty"`
}

// ExtractedTag is the raw output from AI extraction
type ExtractedTag struct {
	Label           string            `json:"label"`
	Category        string            `json:"category"` // event, person, keyword
	Aliases         []string          `json:"aliases,omitempty"`
	Description     string            `json:"description,omitempty"`
	AuxiliaryLabels []AuxiliaryLabel  `json:"auxiliary_labels,omitempty"`
	Fingerprint     *EventFingerprint `json:"fingerprint,omitempty"` // 仅 event 标签：同一次调用产出的结构化指纹
	// SubjectDesc — 主体稳定短身份说明（仅影子态同次调用产出、仅 event；
	// off 响应无此字段）。随提及原文一起落 metadata.entity_subject。
	SubjectDesc string `json:"subject_desc,omitempty"`
}

// TagResolutionRequest is sent to AI for ambiguous tag matching
type TagResolutionRequest struct {
	CandidateTag   ExtractedTag     `json:"candidate_tag"`
	SimilarTags    []SimilarTagInfo `json:"similar_tags"`
	SummaryContext string           `json:"summary_context"`
}

// SimilarTagInfo provides context about similar existing tags
type SimilarTagInfo struct {
	ID         uint     `json:"id"`
	Label      string   `json:"label"`
	Category   string   `json:"category"`
	Aliases    []string `json:"aliases"`
	Similarity float64  `json:"similarity"`
	UsageCount int      `json:"usage_count,omitempty"`
	FeedCount  int      `json:"feed_count,omitempty"`
}

// TagResolutionResponse is AI's decision on tag matching
type TagResolutionResponse struct {
	Decision    string `json:"decision"` // "reuse" or "create_new"
	ReuseTagID  uint   `json:"reuse_tag_id,omitempty"`
	Reason      string `json:"reason"`
	NewLabel    string `json:"new_label,omitempty"` // Fine-tuned label if creating
	NewCategory string `json:"new_category,omitempty"`
}

// TopicTagSummary represents a brief tag reference on an article card
type TopicTagSummary struct {
	Slug     string `json:"slug"`
	Label    string `json:"label"`
	Category string `json:"category"`
}

// GetTopicArticlesParams holds query parameters for GetTopicArticles API
type GetTopicArticlesParams struct {
	Page       int    `form:"page" binding:"min=1"`
	PageSize   int    `form:"page_size" binding:"min=1,max=100"`
	WindowType string `form:"type" binding:"oneof=daily weekly"`
	AnchorDate string `form:"date"`
}

// PendingArticle represents an article that has a tag but is not yet in any digest
type PendingArticle struct {
	ID        uint   `json:"id"`
	Title     string `json:"title"`
	Link      string `json:"link"`
	PubDate   string `json:"pub_date,omitempty"`
	FeedName  string `json:"feed_name"`
	FeedIcon  string `json:"feed_icon,omitempty"`
	FeedColor string `json:"feed_color,omitempty"`
}

type SimilarityEdge struct {
	TagAID     uint
	TagBID     uint
	Similarity float64
}
