package models

import (
	"time"

	"syntopica-backend/internal/models"
)

// ── preference-vector-feed-discovery：偏好向量 / RSSHub 路由目录 / 订阅源推荐 ──
//
// pgvector 列写法沿用 topic_tag_embeddings（EmbeddingVec string + type:vector + column:embedding），
// Dimension/Model 入库以便重算/粗筛前校验同维同模型。jsonb 列用 MetadataMap（已实现 Value/Scan）。

// PreferenceVector 是按 SemanticBoard（board_id=NULL 表示全局桶）聚合的偏好向量。
// source=behavior 由 scheduler 全量重算（不覆盖 seed 行）；source=seed 由问答加权合并累积。
// UNIQUE(board_id, source)：board_id 非 NULL 组合由 GORM uniqueIndex 保证；
// 全局桶（board_id IS NULL）单行由 service 层 upsert 保证（PG 普通 unique 允许多 NULL）。
type PreferenceVector struct {
	ID             uint               `gorm:"primaryKey" json:"id"`
	BoardID        *uint              `gorm:"index;uniqueIndex:idx_preference_vectors_board_source" json:"board_id,omitempty"`
	Source         string             `gorm:"size:20;uniqueIndex:idx_preference_vectors_board_source" json:"source"` // behavior | seed
	EmbeddingVec   string             `gorm:"type:vector;column:embedding" json:"-"`
	Dimension      int                `json:"dimension"`
	Model          string             `gorm:"size:50" json:"model"`
	TagWeights     models.MetadataMap `gorm:"type:jsonb;serializer:json;default:'{}'" json:"tag_weights"` // {tag_label: weight} top 列表
	LastComputedAt time.Time          `json:"last_computed_at"`
	CreatedAt      time.Time          `json:"created_at"`
	UpdatedAt      time.Time          `json:"updated_at"`

	Board *models.SemanticLabel `gorm:"foreignKey:BoardID" json:"board,omitempty"`
}

func (PreferenceVector) TableName() string { return "preference_vectors" }
