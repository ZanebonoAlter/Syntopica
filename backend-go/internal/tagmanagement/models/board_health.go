package models

import (
	"time"

	"syntopica-backend/internal/models"
)

// BoardHealthSnapshot persists one board-health checkup round
// (add-board-health-checkup design D2). One row per board per round; manual
// daily-report re-runs produce same-day rows on purpose (real data points,
// kept in the trend series verbatim). AutoMigrate creates the table + index;
// the FK ON DELETE CASCADE lives in versioned migration 20261006_0001
// (AutoMigrate runs with DisableForeignKeyConstraintWhenMigrating).
type BoardHealthSnapshot struct {
	ID                   uint                   `gorm:"primaryKey" json:"id"`
	SemanticBoardID      uint                   `gorm:"not null;index:idx_board_health_snapshots_board_checked,priority:1" json:"semantic_board_id"`
	CheckedAt            time.Time              `gorm:"not null;index:idx_board_health_snapshots_board_checked,priority:2" json:"checked_at"`
	Metrics              BoardHealthMetrics     `gorm:"type:jsonb;serializer:json;not null" json:"metrics"`
	UncapturedCandidates []BoardHealthCandidate `gorm:"type:jsonb;serializer:json;not null;default:'[]'" json:"uncaptured_candidates"`
	// LLMScore is the LLM adjudication (0-100). NULL = degraded round
	// (bad JSON / timeout): statistics persisted, score/comment absent.
	LLMScore   *float64  `json:"llm_score"`
	LLMComment *string   `gorm:"type:text" json:"llm_comment"`
	CreatedAt  time.Time `json:"created_at"`

	SemanticBoard *models.SemanticLabel `gorm:"foreignKey:SemanticBoardID" json:"semantic_board,omitempty"`
}

func (BoardHealthSnapshot) TableName() string { return "board_health_snapshots" }

// BoardHealthMetrics is the jsonb payload of a snapshot (design D4): board-level
// aggregates over ALL matched rows + label-level affinity distributions
// (composition-label embedding × every mounted tag identity embedding).
// Computed by the boardhealth package (口径唯一实现).
type BoardHealthMetrics struct {
	Board  BoardHealthBoardMetrics   `json:"board"`
	Labels []BoardHealthLabelMetrics `json:"labels"`
}

// BoardHealthBoardMetrics aggregates every matched row of the board (口径①：
// 全部匹配行). VectorMissing counts mounted tags whose identity embedding
// is absent (skipped by the label-level affinity recompute, never zero-
// filled); a composition label missing its OWN embedding marks that label
// insufficient instead of counting here.
type BoardHealthBoardMetrics struct {
	MatchedTags   int            `json:"matched_tags"`
	ReasonDist    map[string]int `json:"reason_dist"`
	GhostMatches  int            `json:"ghost_matches"`
	VectorMissing int            `json:"vector_missing"`
}

// BoardHealthLabelMetrics is one composition label's affinity distribution
// (口径②修正：构成标签 embedding × 全部挂载 tags 的 identity embedding 余弦
// 分布，不依赖 tag 的 aux 挂载数量). Distribution fields are pointers so an
// insufficient label serializes them as null (spec: 样本不足不产出分布统计).
type BoardHealthLabelMetrics struct {
	AuxID        uint     `json:"aux_id"`
	Label        string   `json:"label"`
	Samples      int      `json:"samples"`
	CosineMean   *float64 `json:"cosine_mean"`
	CosineMedian *float64 `json:"cosine_median"`
	CosineP25    *float64 `json:"cosine_p25"`
	CosineP75    *float64 `json:"cosine_p75"`
	LowSimRatio  *float64 `json:"low_sim_ratio"`
	Insufficient bool     `json:"insufficient"`
}

// BoardHealthCandidate is one uncaptured member-derived candidate (事实画像
// diff): an active aux label carried by tags already matched to the board but
// absent from the board composition. MemberTags / MemberArticles are the
// evidence frequencies (top 50 persisted, design D4).
type BoardHealthCandidate struct {
	AuxID          uint   `json:"aux_id"`
	Label          string `json:"label"`
	MemberTags     int    `json:"member_tags"`
	MemberArticles int    `json:"member_articles"`
}
