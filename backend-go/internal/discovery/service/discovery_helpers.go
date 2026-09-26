package service

import (
	"crypto/sha256"
	"encoding/hex"
	"math"
	"strconv"
	"strings"
)

// ── preference-vector-feed-discovery 公共纯函数（无 DB 依赖，可单测） ──
//
// 权威：design.md D1（行为加权分级 + 30 天衰减）、D3（参数解析）、
// D5/C（recommendation_hash = route_id+board_id，不含 source）、D7/A（种子加权合并）。

// D1 行为权重分级（收藏 > 深读 > 普通打开）。
const (
	BehaviorWeightFavorite  = 1.0
	BehaviorWeightDeepRead  = 0.6
	BehaviorWeightOpen      = 0.3
	DeepReadScrollThreshold = 80  // scroll_depth ≥ 80% 算深读
	DeepReadTimeThreshold   = 120 // reading_time ≥ 120s 算深读
	PreferenceDecayDays     = 30.0
)

// D7/A 种子合并与冷启动默认值（均可由 ai_settings 覆盖，见 service）。
const (
	SeedMergeAlphaDefault     = 0.4 // new_vec = normalize(α×incoming + (1−α)×existing)
	SeedMatchThresholdDefault = 0.5 // 问答 embedding 与板块向量匹配阈值
	MinTagsPerBoardDefault    = 3   // 桶内不同标签数 < 此值则退全局桶
	RecommendationTopNDefault = 8   // 每版块粗筛 top-N
)

// （4.4 起：旧「推荐 dismiss 冷却 30 天 hash 池」退役，冷却权威改为
// candidate_preferences.snoozed_until（默认 30 天 = SnoozeDaysDefault）；
// feed_recommendations.dismissed_at 历史行只读。）

// parsePgVector 解析 pgvector 文本格式 "[1,2,3]" → []float64（内联，避免跨包依赖）。
func parsePgVector(s string) ([]float64, error) {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "[")
	s = strings.TrimSuffix(s, "]")
	if s == "" {
		return nil, nil
	}
	parts := strings.Split(s, ",")
	out := make([]float64, 0, len(parts))
	for _, p := range parts {
		f, err := strconv.ParseFloat(strings.TrimSpace(p), 64)
		if err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, nil
}

// floatsToPgVector 把 []float64 → pgvector 文本格式 "[1,2,3]"。
func floatsToPgVector(v []float64) string {
	parts := make([]string, len(v))
	for i, f := range v {
		parts[i] = strconv.FormatFloat(f, 'f', -1, 64)
	}
	return "[" + strings.Join(parts, ",") + "]"
}

// articleBehaviorLevel 按一篇文章的行为集合返回权重档位（D1）。
// hasFavorite：该文章是否出现过 favorite 事件；maxScroll/maxTime：该文章行为峰值。
func articleBehaviorLevel(hasFavorite bool, maxScroll, maxTime int) float64 {
	if hasFavorite {
		return BehaviorWeightFavorite
	}
	if maxScroll >= DeepReadScrollThreshold || maxTime >= DeepReadTimeThreshold {
		return BehaviorWeightDeepRead
	}
	return BehaviorWeightOpen
}

// timeDecay 返回 exp(-days/30)（D1 时间衰减）。
func timeDecay(days float64) float64 {
	return math.Exp(-days / PreferenceDecayDays)
}

// normalizeVector 把向量归一化为单位长度；零向量返回 nil（避免除零）。
func normalizeVector(v []float64) []float64 {
	var norm float64
	for _, x := range v {
		norm += x * x
	}
	if norm == 0 {
		return nil
	}
	inv := 1.0 / math.Sqrt(norm)
	out := make([]float64, len(v))
	for i, x := range v {
		out[i] = x * inv
	}
	return out
}

// mergeSeedVectors 做 D7/A 加权合并：normalize(α×incoming + (1−α)×existing)。
// existing 为空 → 归一化 incoming 作为首条种子；incoming 为空 → 保持 existing。
// 维度不一致时降级为归一化 incoming（避免错维相加）。
func mergeSeedVectors(incoming, existing []float64, alpha float64) []float64 {
	if len(incoming) == 0 {
		return existing
	}
	if len(existing) == 0 || len(incoming) != len(existing) {
		return normalizeVector(incoming)
	}
	merged := make([]float64, len(incoming))
	for i := range incoming {
		merged[i] = alpha*incoming[i] + (1-alpha)*existing[i]
	}
	return normalizeVector(merged)
}

// ComputeRecommendationHash 返回 route_id+board_id 的稳定指纹（D5/C，不含 source）。
// boardID 为 nil 表示全局桶 → "0"。sha256 前 32 hex 字符。
// qa 与 manual_refresh 共享同一幂等池与 dismiss 冷却池。
func ComputeRecommendationHash(routeID uint, boardID *uint) string {
	board := "0"
	if boardID != nil {
		board = strconv.FormatUint(uint64(*boardID), 10)
	}
	raw := strconv.FormatUint(uint64(routeID), 10) + "|" + board
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])[:32]
}

// ComputeCandidateRecommendationHash 返回原生 rss 候选（无 route_id）的推荐幂等指纹：
// 带 "cand:" 命名空间前缀，与 route 哈希（纯数字前缀）不可能碰撞；不含 source。
func ComputeCandidateRecommendationHash(candidateID uint, boardID *uint) string {
	board := "0"
	if boardID != nil {
		board = strconv.FormatUint(uint64(*boardID), 10)
	}
	raw := "cand:" + strconv.FormatUint(uint64(candidateID), 10) + "|" + board
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])[:32]
}

// ParseRouteParameters 解析 RSSHub path 的参数段（D3）：
//   - 无 :param 段，或所有参数段可选（带 ?）→ usable_directly=true
//   - 存在必填参数段（不带 ?）→ requires_parameters=true
//
// 参数段形如 :uid / :category{.+}? / :b?；以 ? 结尾视为可选。
func ParseRouteParameters(path string) (usableDirectly, requiresParameters bool) {
	for _, seg := range strings.Split(path, "/") {
		if !strings.HasPrefix(seg, ":") {
			continue
		}
		if !strings.HasSuffix(seg, "?") {
			requiresParameters = true
		}
	}
	usableDirectly = !requiresParameters
	return
}
