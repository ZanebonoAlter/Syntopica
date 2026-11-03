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
//
// board-health-triage-advice seam A extension: Triage (top-level, design
// D4) carries the LLM label-level adjudication of the round; nil (JSON
// null) on degraded rounds and on history snapshots written before the
// field existed — consumers treat null as "no triage" (old shape).
type BoardHealthMetrics struct {
	Board  BoardHealthBoardMetrics   `json:"board"`
	Labels []BoardHealthLabelMetrics `json:"labels"`
	Triage []BoardHealthTriageItem   `json:"triage"`
}

// BoardHealthBoardMetrics aggregates every matched row of the board (口径①：
// 全部匹配行). VectorMissing counts mounted tags whose identity embedding
// is absent (skipped by the label-level affinity recompute, never zero-
// filled); a composition label missing its OWN embedding marks that label
// insufficient instead of counting here.
//
// MagnetFlow (board-health-triage-advice D2) is the board-level flow
// evidence for magnet suspicion: where the board's direction-mismatch tags
// are VALIDLY mounted elsewhere, top-1 aggregate. nil (JSON null) when the
// board has no failed mounts this round.
type BoardHealthBoardMetrics struct {
	MatchedTags   int                    `json:"matched_tags"`
	ReasonDist    map[string]int         `json:"reason_dist"`
	GhostMatches  int                    `json:"ghost_matches"`
	VectorMissing int                    `json:"vector_missing"`
	MagnetFlow    *BoardHealthMagnetFlow `json:"magnet_flow"`
}

// BoardHealthMagnetFlow is the top-1 valid-flow destination of the board's
// failed (direction_mismatch) mounts (design D2). Failed tags whose only
// other mounts are themselves direction_mismatch are NOT reliable flow —
// FlowTags counts only tags with ≥1 valid mount on another active board.
// TopBoard nil = explicit unknown (failed mounts exist but none has a
// reliable destination); do not fabricate a flow target.
type BoardHealthMagnetFlow struct {
	FailedTags int     `json:"failed_tags"` // failed mounts considered
	FlowTags   int     `json:"flow_tags"`   // failed tags with ≥1 valid other-board mount
	TopBoard   *string `json:"top_board"`   // top-1 destination board label; nil = unknown
	TopTags    int     `json:"top_tags"`    // failed tags validly mounted on TopBoard
}

// BoardHealthMagnetSignal is the per-label magnet (磁铁污染嫌疑) signal
// (board-health-triage-advice design D2): the composition label's affinity
// mean over ALL vector-bearing mounted tags vs over the direction_mismatch
// subset of them. Means are nil when that side has no vector-bearing sample
// or the label's own embedding is missing; Insufficient (either side short
// on samples, or label vector missing) suppresses the suspicion verdict —
// 样本不足/向量缺失不硬凑 (spec「标签分诊信号计算」). No per-label
// attribution of WHICH tag was attracted by which label (红线：磁铁不做
// 单标签吸引归因，失败子集是版块级同一组行).
type BoardHealthMagnetSignal struct {
	AllMean       *float64 `json:"all_mean"`
	FailedMean    *float64 `json:"failed_mean"`
	AllSamples    int      `json:"all_samples"`
	FailedSamples int      `json:"failed_samples"`
	Suspicious    bool     `json:"suspicious"`
	Insufficient  bool     `json:"insufficient"`
}

// BoardHealthNeighbor is one pgvector nearest neighbor of a composition
// label within the active auxiliary-label vector space (排除自身).
type BoardHealthNeighbor struct {
	AuxID  uint    `json:"aux_id"`
	Label  string  `json:"label"`
	Cosine float64 `json:"cosine"`
}

// BoardHealthTriageItem is one label-level LLM triage verdict persisted in
// metrics.triage (design D3/D4). Category is the closed set
// remove|rewrite|keep|insufficient; Evidence must cite real signal values
// (no fabrication); Suggestion carries the proposed new description and is
// required for rewrite items only.
type BoardHealthTriageItem struct {
	AuxID      uint   `json:"aux_id"`
	Category   string `json:"category"`
	Evidence   string `json:"evidence"`
	Suggestion string `json:"suggestion,omitempty"`
}

// BoardHealthLabelMetrics is one composition label's affinity distribution
// (口径②修正：构成标签 embedding × 全部挂载 tags 的 identity embedding 余弦
// 分布，不依赖 tag 的 aux 挂载数量). Distribution fields are pointers so an
// insufficient label serializes them as null (spec: 样本不足不产出分布统计).
type BoardHealthLabelMetrics struct {
	AuxID uint   `json:"aux_id"`
	Label string `json:"label"`
	// LabelType is the composition label's own type (auxiliary | composite).
	// 描述修复链全程只认 auxiliary：组合标签进入体检分诊是**只读**的（前端据此
	// 标不可执行并给出明确说明），只有摘除（既有 composition DELETE 链）可处理。
	// 旧快照缺该字段时为空串（= 未知），前端按"不可执行"处理，不臆造类型。
	LabelType    string   `json:"label_type"`
	Samples      int      `json:"samples"`
	CosineMean   *float64 `json:"cosine_mean"`
	CosineMedian *float64 `json:"cosine_median"`
	CosineP25    *float64 `json:"cosine_p25"`
	CosineP75    *float64 `json:"cosine_p75"`
	LowSimRatio  *float64 `json:"low_sim_ratio"`
	Insufficient bool     `json:"insufficient"`
	// Description snapshots the label's description at checkup time — the
	// description-pollution evidence source (病C: storage embedding =
	// embed("label: description")); empty → omitted.
	Description string `json:"description,omitempty"`
	// Magnet is the per-label magnet signal (design D2); nil on history
	// snapshots written before seam A.
	Magnet *BoardHealthMagnetSignal `json:"magnet"`
	// Neighbors is the pgvector top-3 nearest active auxiliary labels
	// (excluding self); nil = label embedding missing (signal insufficient,
	// uncomputed), non-nil empty = computed but the space holds no other
	// label.
	Neighbors []BoardHealthNeighbor `json:"neighbors"`
	// DescFlags lists the hit article-style word roots (事件性预筛);
	// nil = no hit / empty description.
	DescFlags []string `json:"desc_flags"`
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
