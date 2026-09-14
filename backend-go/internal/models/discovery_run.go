package models

import "time"

// ── improve-discovery-recommendations：发现运行账本 / 兴趣记录 / 候选排除 / 候选向量 ──
//
// 设计见 openspec/changes/improve-discovery-recommendations/design.md D2/D3/D6。
// pgvector 列沿用 PreferenceVector 写法（EmbeddingVec string + type:vector + column:embedding），
// Dimension/Model 入库供重算/检索前校验同维同模型；jsonb 列用 MetadataMap（已实现 Value/Scan）。
// 候选主实体 FeedCandidate 由独立文件承载（另一切片），本文件字段仅以 CandidateID 引用不强挂 FK。

// DiscoveryRun 是一次发现运行（手动查询 ask / 个性化刷新 refresh）的账本行。
// D2：request_key 唯一——并发提交相同 request_key 返回同一运行，失败重试复用 key，
// 用户重新主动提问生成新 key；网络与 LLM 调用在事务外，运行结果经短事务原子发布。
type DiscoveryRun struct {
	ID            uint       `gorm:"primaryKey" json:"id"`
	RequestKey    string     `gorm:"size:64;uniqueIndex:idx_discovery_runs_request_key" json:"request_key"`
	Kind          string     `gorm:"size:20" json:"kind"` // ask | refresh
	Query         string     `gorm:"size:500" json:"query"`
	BoardFilterID *uint      `json:"board_filter_id,omitempty"`
	Status        string     `gorm:"size:20;default:'running'" json:"status"` // running | succeeded | failed
	ErrorCode     string     `gorm:"size:50" json:"error_code"`
	StartedAt     time.Time  `json:"started_at"`
	FinishedAt    *time.Time `json:"finished_at,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

func (DiscoveryRun) TableName() string { return "discovery_runs" }

// DiscoveryRunItem 是一次运行实际发布的候选快照。
// D2：run_id+candidate_id 联合唯一——同运行同候选只一行，查询结果经 run 读取不另造幂等池；
// RecallSources 记录实际召回来源集合（board/behavior/seed 多路命中去重后保留来源），
// ReasonSnapshot 固化发布时刻的推荐理由。
type DiscoveryRunItem struct {
	ID               uint        `gorm:"primaryKey" json:"id"`
	RunID            uint        `gorm:"index;uniqueIndex:idx_discovery_run_item_uniq" json:"run_id"`
	CandidateID      uint        `gorm:"uniqueIndex:idx_discovery_run_item_uniq" json:"candidate_id"`
	RecommendationID *uint       `json:"recommendation_id,omitempty"`
	Rank             int         `json:"rank"`
	RecallSources    MetadataMap `gorm:"type:jsonb;serializer:json;default:'{}'" json:"recall_sources"` // {"board":[…],"behavior":[…],"seed":[…]}
	ReasonSnapshot   string      `gorm:"type:text" json:"reason_snapshot"`
	CreatedAt        time.Time   `json:"created_at"`
}

func (DiscoveryRunItem) TableName() string { return "discovery_run_items" }

// DiscoveryInterestEntry 是一次成功查询落下的独立兴趣记录（替代旧 seed 合并通道）。
// D3：run_id 唯一——活跃条目一 run 一种子，多主题查询不互相平均；可空：迁移来的旧
// seed 无原始 run，legacy 行写 NULL（PG/sqlite 普通唯一索引均允许多 NULL，不参与唯一
// 约束）。board_id 可空（归属仅当最高相似度达阈值且领先第二名达 margin，否则 NULL
// 不强行挂版块）；失败查询不落行，成功零结果也写一条完整记录。status=legacy_inactive
// 标记迁移来的旧 seed（不编造原查询，legacy_ref 保留旧 preference_vectors 引用溯源）。
type DiscoveryInterestEntry struct {
	ID                uint      `gorm:"primaryKey" json:"id"`
	RunID             *uint     `gorm:"index;uniqueIndex:idx_discovery_interest_run" json:"run_id,omitempty"`
	QueryText         string    `gorm:"size:500" json:"query_text"`
	BoardID           *uint     `gorm:"index" json:"board_id,omitempty"`
	EmbeddingVec      string    `gorm:"type:vector;column:embedding" json:"-"`
	Dimension         int       `json:"dimension"`
	Model             string    `gorm:"size:50" json:"model"`
	EmbeddingConfigID *uint     `json:"embedding_config_id,omitempty"`
	Status            string    `gorm:"size:20;default:'active'" json:"status"` // active | inactive | legacy_inactive
	LegacyRef         string    `gorm:"size:128" json:"legacy_ref,omitempty"`   // 迁移溯源：旧 seed 行引用，可空
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

func (DiscoveryInterestEntry) TableName() string { return "discovery_interest_entries" }

// CandidatePreference 是候选的跨 source 长期排除权威（D2/D5）。
// 与目录 recommendation_enabled 正交：关推荐开关不动排除，恢复排除只恢复资格、
// 不建订阅不立即出卡。excluded_at 非空即长期排除（跨 qa/refresh 生效）；
// snoozed_until 是「暂时不看」冷却到期时刻（now >= snoozed_until 即恢复资格）。
type CandidatePreference struct {
	ID           uint       `gorm:"primaryKey" json:"id"`
	CandidateID  uint       `gorm:"uniqueIndex:idx_candidate_preferences_candidate" json:"candidate_id"`
	ExcludedAt   *time.Time `json:"excluded_at,omitempty"`
	SnoozedUntil *time.Time `json:"snoozed_until,omitempty"`
	Note         string     `gorm:"size:255" json:"note,omitempty"` // 可空备注
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

func (CandidatePreference) TableName() string { return "candidate_preferences" }

// CandidateEmbedding 存候选有效介绍的向量（D6 版本化生成指纹方案）。
// D6 以 (candidate_id, embedding_config_id, model, dim) 定位；embedding_config_id 可空，
// 而 PG 普通唯一索引对 NULL 不去重（同 NULL 组合可插多行），故唯一键先以
// (candidate_id, model, dimension) 收敛——模型切换期新旧维度向量可共存但不重复，
// 混算轮次由检索前同维同模型校验阻断（不同模型不得用旧向量兜底）。
// text_hash 为文本指纹（有效元数据 + 生成器版本），变更仅标 dirty 入队重算。
type CandidateEmbedding struct {
	ID                uint      `gorm:"primaryKey" json:"id"`
	CandidateID       uint      `gorm:"uniqueIndex:idx_candidate_embedding_uniq" json:"candidate_id"`
	Model             string    `gorm:"size:50;uniqueIndex:idx_candidate_embedding_uniq" json:"model"`
	Dimension         int       `gorm:"uniqueIndex:idx_candidate_embedding_uniq" json:"dimension"`
	EmbeddingConfigID *uint     `json:"embedding_config_id,omitempty"`
	TextHash          string    `gorm:"size:64;index" json:"text_hash"`
	EmbeddingVec      string    `gorm:"type:vector;column:embedding" json:"-"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

func (CandidateEmbedding) TableName() string { return "candidate_embeddings" }
