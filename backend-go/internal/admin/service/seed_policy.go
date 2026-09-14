package service

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
)

// ── improve-discovery-recommendations 4.2：独立兴趣列表与有界影响（design D3）──
//
// 纯函数策略层：无 DB / 网络 / 时间源（时间一律由调用方注入）。
// 条目强度 w = 2^(-ageDays/halfLife) × (1 - min(N/maturity, 1))；
// 整个 seed 预算 floor(seed_budget × max(w))，按 w 比例最大余数法分配整数名额。
// 配置持久化见 seed_policy_config.go（IO 层）；接线见 discovery_run_service.go。

// SeedPolicyConfig 是兴趣衰减 / 种子预算七参数（design D3 默认参数表）。
// 持久化为 ai_settings 表 key=discovery_seed_policy 的 JSON blob，
// JSON 字段名与 design 表一致（interest_window_days 等）。
type SeedPolicyConfig struct {
	WindowDays       int     `json:"interest_window_days"`       // 默认 30，[1,365]
	MaxEntries       int     `json:"interest_max_entries"`       // 默认 5，[1,20]
	HalfLifeDays     int     `json:"interest_half_life_days"`    // 默认 7，[1,90] 且 ≤ WindowDays
	MaturityArticles int     `json:"behavior_maturity_articles"` // 默认 20，[1,1000]
	SeedBudget       int     `json:"seed_candidate_budget"`      // 默认 4，[0,16]
	MatchSimilarity  float64 `json:"seed_match_similarity"`      // 默认 0.78，[0,1]
	MatchMargin      float64 `json:"seed_match_margin"`          // 默认 0.03，[0,1]
}

// DefaultSeedPolicyConfig 返回 D3 表默认值。
func DefaultSeedPolicyConfig() SeedPolicyConfig {
	return SeedPolicyConfig{
		WindowDays:       30,
		MaxEntries:       5,
		HalfLifeDays:     7,
		MaturityArticles: 20,
		SeedBudget:       4,
		MatchSimilarity:  0.78,
		MatchMargin:      0.03,
	}
}

// Validate 逐项校验七参数，错误信息带字段名与合法域（半衰期额外要求不超过窗口）。
// 返回的 error 汇总全部违规项，一次暴露所有问题。
func (c SeedPolicyConfig) Validate() error {
	var problems []string
	if c.WindowDays < 1 || c.WindowDays > 365 {
		problems = append(problems, fmt.Sprintf("interest_window_days=%d 超出 [1,365]", c.WindowDays))
	}
	if c.MaxEntries < 1 || c.MaxEntries > 20 {
		problems = append(problems, fmt.Sprintf("interest_max_entries=%d 超出 [1,20]", c.MaxEntries))
	}
	if c.HalfLifeDays < 1 || c.HalfLifeDays > 90 {
		problems = append(problems, fmt.Sprintf("interest_half_life_days=%d 超出 [1,90]", c.HalfLifeDays))
	} else if c.HalfLifeDays > c.WindowDays {
		problems = append(problems, fmt.Sprintf("interest_half_life_days=%d 不得超过 interest_window_days=%d",
			c.HalfLifeDays, c.WindowDays))
	}
	if c.MaturityArticles < 1 || c.MaturityArticles > 1000 {
		problems = append(problems, fmt.Sprintf("behavior_maturity_articles=%d 超出 [1,1000]", c.MaturityArticles))
	}
	if c.SeedBudget < 0 || c.SeedBudget > 16 {
		problems = append(problems, fmt.Sprintf("seed_candidate_budget=%d 超出 [0,16]", c.SeedBudget))
	}
	if c.MatchSimilarity < 0 || c.MatchSimilarity > 1 {
		problems = append(problems, fmt.Sprintf("seed_match_similarity=%v 超出 [0,1]", c.MatchSimilarity))
	}
	if c.MatchMargin < 0 || c.MatchMargin > 1 {
		problems = append(problems, fmt.Sprintf("seed_match_margin=%v 超出 [0,1]", c.MatchMargin))
	}
	if len(problems) == 0 {
		return nil
	}
	return fmt.Errorf("seed policy 配置非法: %s", strings.Join(problems, "; "))
}

// EntryFacts 是参与判定所需的最小兴趣条目事实（纯函数不触 DB）。
type EntryFacts struct {
	ID        uint64    // discovery_interest_entries.id
	Status    string    // active | inactive | legacy_inactive
	BoardID   *uint64   // nil = 未匹配（全局组）
	CreatedAt time.Time // 条目落库时刻（兴趣表达时间）
}

// EntryParticipation 标记单条条目是否入选参与集（未入选不删除、仅退出参与，
// 历史仍可见——spec「有界衰减」）。
type EntryParticipation struct {
	Entry        EntryFacts
	Participates bool
}

// InterestWeight 条目强度（design D3）：
// w = 2^(-ageDays/halfLife) × (1 - min(N/maturity, 1))。
//   - ageDays < 0（系统时钟回拨）按 0 处理；
//   - age ≥ window → 0（且该条目不参与本轮，见 ParticipatingEntries）；
//   - behaviorCount 为对应范围（版块或全局）窗口内去重阅读文章数，负数按 0、
//     超过 maturity 截断到 1（成熟后让位，份额归 0 但记录保留）。
func InterestWeight(entry EntryFacts, behaviorCount int, now time.Time, cfg SeedPolicyConfig) float64 {
	ageDays := now.Sub(entry.CreatedAt).Hours() / 24
	if ageDays < 0 {
		ageDays = 0 // 时钟回拨防御
	}
	if ageDays >= float64(cfg.WindowDays) {
		return 0
	}
	if cfg.HalfLifeDays <= 0 || cfg.MaturityArticles <= 0 {
		return 0 // 非法配置防御（正常入口已 Validate）
	}
	decay := math.Exp2(-ageDays / float64(cfg.HalfLifeDays))
	n := float64(behaviorCount)
	if n < 0 {
		n = 0
	}
	factor := 1 - math.Min(n/float64(cfg.MaturityArticles), 1)
	return decay * factor
}

// ParticipatingEntries 计算参与集：status=active 且仍在窗口内的条目
// （参与判据 now < created_at+window：age 恰达 window 的当刻退出——窗口两端
// 以该判据为准）；按 created_at 降序取前 MaxEntries，同刻以 ID 降序稳定破同分。
// 返回保留全部输入条目并带参与标记：未入选者不删除、仅退出参与（历史保留）。
func ParticipatingEntries(entries []EntryFacts, now time.Time, cfg SeedPolicyConfig) []EntryParticipation {
	out := make([]EntryParticipation, len(entries))
	eligible := make([]int, 0, len(entries))
	for i, e := range entries {
		out[i] = EntryParticipation{Entry: e}
		if e.Status != "active" {
			continue
		}
		if !e.CreatedAt.AddDate(0, 0, cfg.WindowDays).After(now) {
			continue // now >= created_at+window → 出局
		}
		eligible = append(eligible, i)
	}
	sort.Slice(eligible, func(a, b int) bool {
		x, y := entries[eligible[a]], entries[eligible[b]]
		if !x.CreatedAt.Equal(y.CreatedAt) {
			return x.CreatedAt.After(y.CreatedAt) // 最近优先
		}
		return x.ID > y.ID // 同刻以 ID 降序破同分
	})
	k := cfg.MaxEntries
	if k > len(eligible) {
		k = len(eligible)
	}
	for _, idx := range eligible[:k] {
		out[idx].Participates = true
	}
	return out
}

// WeightedEntry 是参与条目的 (ID, w) 输入（w 来自 InterestWeight）。
type WeightedEntry struct {
	ID     uint64
	Weight float64
}

// SeedShareAllocation 按最大余数法把整数召回名额分给各参与条目（design D3）：
//   - 总预算 = floor(seed_budget × max(w))——条数增长不能越过该上限；全零权重 → 预算 0；
//   - 名额按 w 占 Σw 比例分配：先取整，再按小数余量从大到小补足；
//   - 余量同权以 ID 升序破同分；
//   - 0 权条目恒得 0；某条目未用完的名额不转赠其他条目（由调用方在召回时遵守）。
func SeedShareAllocation(entries []WeightedEntry, cfg SeedPolicyConfig) map[uint64]int {
	out := make(map[uint64]int, len(entries))
	maxW, sum := 0.0, 0.0
	for _, e := range entries {
		out[e.ID] = 0
		if e.Weight > maxW {
			maxW = e.Weight
		}
		if e.Weight > 0 {
			sum += e.Weight
		}
	}
	budget := int(math.Floor(float64(cfg.SeedBudget) * maxW))
	if budget <= 0 || sum <= 0 {
		return out
	}
	type share struct {
		id    uint64
		quota float64
		floor int
	}
	shares := make([]share, 0, len(entries))
	used := 0
	for _, e := range entries {
		q := float64(budget) * e.Weight / sum
		shares = append(shares, share{id: e.ID, quota: q, floor: int(q)})
		used += int(q)
	}
	remaining := budget - used
	sort.SliceStable(shares, func(i, j int) bool {
		fi := shares[i].quota - float64(shares[i].floor)
		fj := shares[j].quota - float64(shares[j].floor)
		if fi != fj {
			return fi > fj // 余量大者先得
		}
		return shares[i].id < shares[j].id // 同余量 ID 升序破同分
	})
	for i := 0; i < remaining && i < len(shares); i++ {
		shares[i].floor++
	}
	for _, sh := range shares {
		out[sh.id] = sh.floor
	}
	return out
}

// BoardVec 是参与归属比较的真实版块向量（调用方负责只装载 label_type='board'
// 且 active、维度与查询 embedding 一致的版块）。
type BoardVec struct {
	ID  uint64
	Vec []float64
}

// MatchBoard 返回查询向量应归属的版块 id（design D3）：
// 余弦相似度直接由向量计算（消费 pgvector 距离值时 sim = 1 - distance，禁止混淆）；
// 单候选只验绝对阈值；多候选需 max ≥ match_similarity 且 (max - 次max) ≥ match_margin；
// 维度不一致的版块跳过（原因见 MatchBoardDetailed）；不满足返回 nil——
// 未匹配组，不挂标签、不建版块。
func MatchBoard(queryVec []float64, boards []BoardVec, cfg SeedPolicyConfig) *uint64 {
	id, _ := MatchBoardDetailed(queryVec, boards, cfg)
	return id
}

// MatchBoardDetailed 与 MatchBoard 相同判定，额外返回被跳过版块的原因
// （维度不一致 / 零向量），供调用方记日志归因。
func MatchBoardDetailed(queryVec []float64, boards []BoardVec, cfg SeedPolicyConfig) (*uint64, []string) {
	type scored struct {
		id  uint64
		sim float64
	}
	var hits []scored
	var skipped []string
	for _, b := range boards {
		if len(b.Vec) == 0 || len(b.Vec) != len(queryVec) {
			skipped = append(skipped, fmt.Sprintf("board %d: 维度不一致 (%d != %d)", b.ID, len(b.Vec), len(queryVec)))
			continue
		}
		sim, ok := cosineSim(queryVec, b.Vec)
		if !ok {
			skipped = append(skipped, fmt.Sprintf("board %d: 零向量", b.ID))
			continue
		}
		hits = append(hits, scored{id: b.ID, sim: sim})
	}
	if len(hits) == 0 {
		return nil, skipped
	}
	sort.Slice(hits, func(i, j int) bool {
		if hits[i].sim != hits[j].sim {
			return hits[i].sim > hits[j].sim
		}
		return hits[i].id < hits[j].id
	})
	best := hits[0]
	if best.sim < cfg.MatchSimilarity {
		return nil, skipped
	}
	if len(hits) >= 2 && best.sim-hits[1].sim < cfg.MatchMargin {
		return nil, skipped
	}
	id := best.id
	return &id, skipped
}
