package models

import "time"

// ── preference-vector-feed-discovery：偏好向量 / RSSHub 路由目录 / 订阅源推荐 ──
//
// pgvector 列写法沿用 topic_tag_embeddings（EmbeddingVec string + type:vector + column:embedding），
// Dimension/Model 入库以便重算/粗筛前校验同维同模型。jsonb 列用 MetadataMap（已实现 Value/Scan）。

// PreferenceVector 是按 SemanticBoard（board_id=NULL 表示全局桶）聚合的偏好向量。
// source=behavior 由 scheduler 全量重算（不覆盖 seed 行）；source=seed 由问答加权合并累积。
// UNIQUE(board_id, source)：board_id 非 NULL 组合由 GORM uniqueIndex 保证；
// 全局桶（board_id IS NULL）单行由 service 层 upsert 保证（PG 普通 unique 允许多 NULL）。
type PreferenceVector struct {
	ID             uint        `gorm:"primaryKey" json:"id"`
	BoardID        *uint       `gorm:"index;uniqueIndex:idx_preference_vectors_board_source" json:"board_id,omitempty"`
	Source         string      `gorm:"size:20;uniqueIndex:idx_preference_vectors_board_source" json:"source"` // behavior | seed
	EmbeddingVec   string      `gorm:"type:vector;column:embedding" json:"-"`
	Dimension      int         `json:"dimension"`
	Model          string      `gorm:"size:50" json:"model"`
	TagWeights     MetadataMap `gorm:"type:jsonb;serializer:json;default:'{}'" json:"tag_weights"` // {tag_label: weight} top 列表
	LastComputedAt time.Time   `json:"last_computed_at"`
	CreatedAt      time.Time   `json:"created_at"`
	UpdatedAt      time.Time   `json:"updated_at"`

	Board *SemanticLabel `gorm:"foreignKey:BoardID" json:"board,omitempty"`
}

func (PreferenceVector) TableName() string { return "preference_vectors" }

// RSSHubRoute 是从自建 RSSHub 实例 /api/namespace 同步的路由元数据。
// requires_parameters/usable_directly 在入库时按 path 参数段解析（D3）。
// content_hash 用于增量 diff；status 标可用性校验结果。
type RSSHubRoute struct {
	ID                 uint       `gorm:"primaryKey" json:"id"`
	Namespace          string     `gorm:"size:100;uniqueIndex:idx_rsshub_routes_ns_path" json:"namespace"`
	Path               string     `gorm:"size:255;uniqueIndex:idx_rsshub_routes_ns_path" json:"path"`
	Name               string     `gorm:"size:255" json:"name"`
	URL                string     `gorm:"type:text" json:"url"`
	Description        string     `gorm:"type:text" json:"description"`
	Parameters         string     `gorm:"type:jsonb;column:parameters" json:"parameters"` // 原始 JSON（数组/对象）
	Example            string     `gorm:"type:text" json:"example"`
	RequiresParameters bool       `json:"requires_parameters"`
	UsableDirectly     bool       `json:"usable_directly"`
	ContentHash        string     `gorm:"size:64;index" json:"content_hash"`             // D2 diff
	Status             string     `gorm:"size:20;index;default:'unknown'" json:"status"` // unknown | ok | broken | gone
	LastCheckedAt      *time.Time `json:"last_checked_at,omitempty"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`

	// ParamOptions 反向关联（route_param_options 字典）。不在此序列化：推荐卡片由
	// RecommendationCard.ParamOptions（按 param_name 分组的 map）单独承载契约字段。
	ParamOptions []RouteParamOption `gorm:"foreignKey:RouteID" json:"-"`
}

func (RSSHubRoute) TableName() string { return "rsshub_routes" }

// RouteParamOption 是某 RSSHub 路由某参数的可选值字典条目（feed-param-options）。
// source 限定 manual（人工录入）/ scraped（文档抓取）——LLM 绝不生成参数值（spec 铁律 D5）。
// UNIQUE(route_id, param_name, value) 防同参数重复录入同一值。
type RouteParamOption struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	RouteID   uint      `gorm:"index;uniqueIndex:idx_route_param_option_uniq" json:"route_id"`
	ParamName string    `gorm:"size:100;uniqueIndex:idx_route_param_option_uniq" json:"param_name"`
	Value     string    `gorm:"size:255;uniqueIndex:idx_route_param_option_uniq" json:"value"`
	Label     string    `gorm:"size:255" json:"label"`
	Source    string    `gorm:"size:20;default:'manual'" json:"source"` // manual | scraped（never llm）
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	Route *RSSHubRoute `gorm:"foreignKey:RouteID;constraint:OnDelete:CASCADE" json:"route,omitempty"`
}

func (RouteParamOption) TableName() string { return "route_param_options" }

// RouteEmbedding 存路由的语义向量（文本取 namespace+name+description 摘要）。
// UNIQUE(route_id)：单路由单向量；text_hash 变更入队重算。
type RouteEmbedding struct {
	ID           uint      `gorm:"primaryKey" json:"id"`
	RouteID      uint      `gorm:"uniqueIndex:idx_route_embeddings_route" json:"route_id"`
	EmbeddingVec string    `gorm:"type:vector;column:embedding" json:"-"`
	Dimension    int       `json:"dimension"`
	Model        string    `gorm:"size:50" json:"model"`
	TextHash     string    `gorm:"size:64;index" json:"text_hash"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`

	Route *RSSHubRoute `gorm:"foreignKey:RouteID;constraint:OnDelete:CASCADE" json:"route,omitempty"`
}

func (RouteEmbedding) TableName() string { return "route_embeddings" }

// FeedRecommendation 是订阅源推荐卡片。
// recommendation_hash 以候选稳定身份和展示桶为基础（不含 source），qa/manual_refresh
// 共享同一幂等池与 dismiss 冷却池（D5/D6）。pending 唯一性由 PG 部分唯一索引
// idx_feed_recommendations_hash_pending（WHERE status='pending'，由 migrator 的
// PG-only 迁移步骤创建/维护）保证，历史行允许同 hash 多条；模型 tag 只保留普通
// 查询索引 idx_feed_recommendations_hash_lookup（旧全表唯一索引由迁移 DROP，
// 故刻意改名避免同名冲突）。
type FeedRecommendation struct {
	ID                 uint       `gorm:"primaryKey" json:"id"`
	RouteID            uint       `gorm:"index;index:idx_feed_rec_status" json:"route_id"`
	BoardID            *uint      `gorm:"index" json:"board_id,omitempty"`
	Source             string     `gorm:"size:20" json:"source"` // manual_refresh | qa
	Score              float64    `gorm:"index:idx_feed_rec_status" json:"score"`
	LLMReason          string     `gorm:"type:text" json:"llm_reason"`
	Status             string     `gorm:"size:20;index:idx_feed_rec_status;default:'pending'" json:"status"` // pending | accepted | dismissed | legacy（迁移历史行）
	AcceptedFeedID     *uint      `gorm:"index" json:"accepted_feed_id,omitempty"`
	RecommendationHash string     `gorm:"size:64;index:idx_feed_recommendations_hash_lookup" json:"recommendation_hash"`
	DismissedAt        *time.Time `json:"dismissed_at,omitempty"`
	CandidateID        *uint      `gorm:"index" json:"candidate_id,omitempty"` // D2 统一候选引用（迁移回填；不挂 FK 强约束）
	LastSelectedAt     *time.Time `json:"last_selected_at,omitempty"`          // D5 最近一次成功入选时刻（再入选刷新）
	ExpiresAt          *time.Time `json:"expires_at,omitempty"`                // D5 到期时刻（now >= expires_at 即退出默认列表）
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`

	Route        *RSSHubRoute   `gorm:"foreignKey:RouteID" json:"route,omitempty"`
	Board        *SemanticLabel `gorm:"foreignKey:BoardID" json:"board,omitempty"`
	AcceptedFeed *Feed          `gorm:"foreignKey:AcceptedFeedID" json:"accepted_feed,omitempty"`
}

func (FeedRecommendation) TableName() string { return "feed_recommendations" }
