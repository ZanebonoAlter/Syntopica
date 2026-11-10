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
	// TriageGuard is the board-level verdict of the triage plausibility gates
	// (fix-board-health-magnet-signal design D6/G3): actionable share of the
	// round, the threshold, whether the round was degraded and why. A degraded
	// round keeps score/comment and the evidence itself — only the actionable
	// items are downgraded to 证据不足.
	//
	// nil = 本轮未产生闸门结论（分诊清单为空、整轮裁决降级）或写该字段之前
	// 的历史快照（消费方按「无闸门记录」处理，不臆造结论）。
	TriageGuard *BoardHealthTriageGuard `json:"triage_guard"`
}

// BoardHealthTriageGuard is the outcome of the triage plausibility gates
// (fix-board-health-magnet-signal design D6): how many triage items were
// actionable (remove+rewrite) out of the total, the configured threshold, the
// resulting ratio, whether the round was degraded, and the reason.
//
// Actionable/Ratio 取自 **LLM 原始裁决**（核心主体拦截 G1 与摘除证据校验 G2
// 生效之前的状态，design D6 / review R-5）——如实反映「本轮模型有多激进」；
// 若取降级后的状态，remove 主导轮次会被 G2 先清零（47 条全降为证据不足），
// 闸门痕迹被抹掉、闸门记录失真。
type BoardHealthTriageGuard struct {
	Actionable int     `json:"actionable"`
	Total      int     `json:"total"`
	Ratio      float64 `json:"ratio"`
	Threshold  float64 `json:"threshold"`
	Degraded   bool    `json:"degraded"`
	Reason     string  `json:"reason"`
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
// (fix-board-health-magnet-signal design D1/D2): the composition label's
// affinity mean over the FAILED mount subset (direction_mismatch=true) vs
// over the HEALTHY mount subset (direction_mismatch=false).
//
// 判定口径：suspicious = failed_mean - good_mean ≥ MagnetGap ——「磁铁」的
// 语义是「把跑题的挂载吸过来」，所以失败侧的贴合均值必须**高于**健康侧。
// 旧口径（all_mean - failed_mean）与语义相反，2026-11-09 真库实证：失败挂载
// 按定义就与本板方向不符、与核心主体天然不相似，旧口径于是「越是核心主体
// gap 越大、越像污染源」，全板 41/47 成立、信号失去区分度。
//
// GoodMean/GoodSamples 是判定用的健康侧；AllMean/AllSamples **保留但退出
// 嫌疑判定**（历史快照解析 + 仍展示全板均值）。注意 all_mean ≡ cosine_mean
// （同集合同算法），两者不是两份互相印证的独立证据，别当成双重信号。
//
// Means are nil when that side has no vector-bearing sample or the label's
// own embedding is missing; Insufficient (either side short on samples, or
// label vector missing) suppresses the suspicion verdict —— 样本不足/向量
// 缺失不硬凑 (spec「标签分诊信号计算」). No per-label attribution of WHICH
// tag was attracted by which label (红线：磁铁不做单标签吸引归因，失败子集
// 是版块级同一组行).
type BoardHealthMagnetSignal struct {
	GoodMean      *float64 `json:"good_mean"`
	FailedMean    *float64 `json:"failed_mean"`
	AllMean       *float64 `json:"all_mean"`
	GoodSamples   int      `json:"good_samples"`
	FailedSamples int      `json:"failed_samples"`
	AllSamples    int      `json:"all_samples"`
	Suspicious    bool     `json:"suspicious"`
	Insufficient  bool     `json:"insufficient"`
	// Caliber 标记**磁铁口径**（fix-board-health-magnet-signal design D12 /
	// review R-4），取值约定：
	//   - "healthy_subset" = 新口径（本轮由后端写入）：判定用「失败子集 vs
	//     健康子集」（failed_mean − good_mean ≥ MagnetGap）；
	//   - 空串 = 旧口径快照（all 侧口径：all_mean − failed_mean）或本字段
	//     写入之前的历史快照（反序列化后自然为空）。
	//
	// 口径标记与样本是否充足**无关**：健康侧样本不足（insufficient）时照样
	// 写 "healthy_subset"，好让消费方区分「旧快照」与「新快照但健康侧不可算」。
	// 消费方 MUST NOT 用 good_mean === null 判定旧快照——新快照在健康侧不可
	// 算时 good_mean 同样是 null（无 omitempty），两者不可区分。
	Caliber string `json:"caliber"`
}

// BoardHealthMagnetCaliberHealthySubset is the caliber marker of the NEW
// magnet口径（design D12）：判定用「失败子集 vs 健康子集」。空串 = 旧口径
// 快照或该字段写入之前的历史快照。
const BoardHealthMagnetCaliberHealthySubset = "healthy_subset"

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
	LabelType string `json:"label_type"`
	// CoreSubject marks the label as the board's own subject (核心主体)
	// (fix-board-health-magnet-signal design D3 / review R-6)：构成标签名
	// （空白/大小写归一后）出现在版块 **description** 或任一 **alias** 中 →
	// 核心主体；**仅**出现在版块 **标题** 中时还须标签名 ≥ 3 字符（2 字短词
	// 可能是简称「美伊」也可能是泛化地域词「中东」，不据标题推断）。核心主体
	// 是版块自身要谈论的对象，MUST NOT 作为摘除或重写对象——规则层闸门 G1
	// 会把 LLM 给它的 remove/rewrite 改判 keep。
	//
	// 历史快照缺该字段 = false（消费方按「非核心主体」处理），不得臆造。
	CoreSubject  bool     `json:"core_subject"`
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
