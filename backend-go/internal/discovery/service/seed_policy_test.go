package service

import (
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// ── improve-discovery-recommendations 4.2：seed policy 纯函数测试（S3 故事账本）──
//
// 权威：design.md D3（衰减公式/参数表/归属规则）+ test-cases.md S3 白盒边界。
// 全部纯内存，无 DB、无时间源（时间注入固定值）。

func seedPolicyTestNow() time.Time {
	return time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC)
}

func entryAt(id uint64, ageDays float64) EntryFacts {
	return EntryFacts{ID: id, Status: "active", CreatedAt: seedPolicyTestNow().Add(-time.Duration(ageDays * float64(24*time.Hour)))}
}

// ── Validate：七参数逐域报错 + 边界值合法 ──

func TestSeedPolicyValidateDefaults(t *testing.T) {
	require.NoError(t, DefaultSeedPolicyConfig().Validate(), "D3 默认值必须全部合法")
}

func TestSeedPolicyValidateDomains(t *testing.T) {
	type c struct {
		name    string
		mutate  func(*SeedPolicyConfig)
		errPart string // 期望错误包含的字段名
	}
	cases := []c{
		{"window 太小", func(x *SeedPolicyConfig) { x.WindowDays = 0 }, "interest_window_days"},
		{"window 太大", func(x *SeedPolicyConfig) { x.WindowDays = 366 }, "interest_window_days"},
		{"max_entries 太小", func(x *SeedPolicyConfig) { x.MaxEntries = 0 }, "interest_max_entries"},
		{"max_entries 太大", func(x *SeedPolicyConfig) { x.MaxEntries = 21 }, "interest_max_entries"},
		{"half_life 太小", func(x *SeedPolicyConfig) { x.HalfLifeDays = 0 }, "interest_half_life_days"},
		{"half_life 太大", func(x *SeedPolicyConfig) { x.HalfLifeDays = 91 }, "interest_half_life_days"},
		{"half_life 超 window", func(x *SeedPolicyConfig) { x.WindowDays = 7; x.HalfLifeDays = 8 }, "不得超过"},
		{"maturity 太小", func(x *SeedPolicyConfig) { x.MaturityArticles = 0 }, "behavior_maturity_articles"},
		{"maturity 太大", func(x *SeedPolicyConfig) { x.MaturityArticles = 1001 }, "behavior_maturity_articles"},
		{"budget 太小", func(x *SeedPolicyConfig) { x.SeedBudget = -1 }, "seed_candidate_budget"},
		{"budget 太大", func(x *SeedPolicyConfig) { x.SeedBudget = 17 }, "seed_candidate_budget"},
		{"similarity 太小", func(x *SeedPolicyConfig) { x.MatchSimilarity = -0.01 }, "seed_match_similarity"},
		{"similarity 太大", func(x *SeedPolicyConfig) { x.MatchSimilarity = 1.01 }, "seed_match_similarity"},
		{"margin 太小", func(x *SeedPolicyConfig) { x.MatchMargin = -0.01 }, "seed_match_margin"},
		{"margin 太大", func(x *SeedPolicyConfig) { x.MatchMargin = 1.01 }, "seed_match_margin"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := DefaultSeedPolicyConfig()
			tc.mutate(&cfg)
			err := cfg.Validate()
			require.Error(t, err)
			require.Contains(t, err.Error(), tc.errPart, "错误必须带字段名/域说明")
		})
	}
}

func TestSeedPolicyValidateBoundariesValid(t *testing.T) {
	cfg := SeedPolicyConfig{WindowDays: 1, MaxEntries: 1, HalfLifeDays: 1, MaturityArticles: 1, SeedBudget: 0, MatchSimilarity: 0, MatchMargin: 0}
	require.NoError(t, cfg.Validate(), "下边界应全部合法（budget/margin 允许 0）")
	cfg = SeedPolicyConfig{WindowDays: 365, MaxEntries: 20, HalfLifeDays: 90, MaturityArticles: 1000, SeedBudget: 16, MatchSimilarity: 1, MatchMargin: 1}
	require.NoError(t, cfg.Validate(), "上边界应全部合法（half_life=90 ≤ window=365）")
}

// ── InterestWeight：公式与年龄/N 边界 ──

func TestInterestWeightBoundaries(t *testing.T) {
	now := seedPolicyTestNow()
	cfg := DefaultSeedPolicyConfig() // window 30 / halfLife 7 / maturity 20

	t.Run("age=0 N=0 满强度", func(t *testing.T) {
		require.InDelta(t, 1.0, InterestWeight(entryAt(1, 0), 0, now, cfg), 1e-9)
	})
	t.Run("N=19 临近成熟", func(t *testing.T) {
		require.InDelta(t, 0.05, InterestWeight(entryAt(1, 0), 19, now, cfg), 1e-9)
	})
	t.Run("N=20 成熟让位", func(t *testing.T) {
		require.Zero(t, InterestWeight(entryAt(1, 0), 20, now, cfg))
	})
	t.Run("N=21 截断不反升", func(t *testing.T) {
		require.Zero(t, InterestWeight(entryAt(1, 0), 21, now, cfg))
	})
	t.Run("N 负数按 0", func(t *testing.T) {
		require.InDelta(t, 1.0, InterestWeight(entryAt(1, 0), -5, now, cfg), 1e-9)
	})
	t.Run("age=7d 半衰减半", func(t *testing.T) {
		require.InDelta(t, 0.5, InterestWeight(entryAt(1, 7), 0, now, cfg), 1e-9)
	})
	t.Run("age=29.99d 仍参与但微弱", func(t *testing.T) {
		w := InterestWeight(entryAt(1, 29.99), 0, now, cfg)
		require.Greater(t, w, 0.0)
		require.Less(t, w, math.Exp2(-29.0/7.0), "窗口末端强度应低于 29d 处")
	})
	t.Run("age=30d 恰达窗口归零", func(t *testing.T) {
		require.Zero(t, InterestWeight(entryAt(1, 30), 0, now, cfg))
	})
	t.Run("age=30.01d 归零", func(t *testing.T) {
		require.Zero(t, InterestWeight(entryAt(1, 30.01), 0, now, cfg))
	})
	t.Run("时钟回拨按 age=0", func(t *testing.T) {
		future := EntryFacts{ID: 1, Status: "active", CreatedAt: now.Add(time.Hour)}
		require.InDelta(t, 1.0, InterestWeight(future, 0, now, cfg), 1e-9)
	})
}

func TestInterestWeightMonotonicInAge(t *testing.T) {
	now := seedPolicyTestNow()
	cfg := DefaultSeedPolicyConfig()
	prev := math.Inf(1)
	for d := 0; d <= 29; d++ {
		w := InterestWeight(entryAt(uint64(d+1), float64(d)), 5, now, cfg)
		require.LessOrEqual(t, w, prev+1e-15, "同 N 年龄增不减（day=%d）", d)
		prev = w
	}
}

func TestInterestWeightMonotonicInN(t *testing.T) {
	now := seedPolicyTestNow()
	cfg := DefaultSeedPolicyConfig()
	prev := math.Inf(1)
	for n := 0; n <= 25; n++ {
		w := InterestWeight(entryAt(1, 3), n, now, cfg)
		require.LessOrEqual(t, w, prev+1e-15, "同龄 N 增强度不减（N=%d）", n)
		prev = w
	}
}

// ── ParticipatingEntries：参与集窗口/K 上限/同秒破同分/历史保留 ──

func TestParticipatingEntriesCapAndWindow(t *testing.T) {
	now := seedPolicyTestNow()
	cfg := DefaultSeedPolicyConfig() // MaxEntries=5, Window=30

	// 6 条窗口内 active（不同时刻）→ 最近 5 条参与，最老的第 6 条退出但保留。
	entries := []EntryFacts{
		entryAt(10, 1), entryAt(11, 2), entryAt(12, 3), entryAt(13, 4), entryAt(14, 5), entryAt(15, 6),
	}
	got := ParticipatingEntries(entries, now, cfg)
	require.Len(t, got, 6, "未入选条目不删除，全部保留并带标记")
	for _, pe := range got {
		if pe.Entry.ID == 15 {
			require.False(t, pe.Participates, "第 6 条（最老）超出 K=5 上限退出参与")
		} else {
			require.True(t, pe.Participates)
		}
	}

	// 非窗口/非 active 状态不参与但保留。
	mixed := []EntryFacts{
		entryAt(20, 29.99), // 窗口内
		entryAt(21, 30),    // 恰达窗口 → 出局
		entryAt(22, 31),    // 超窗
		{ID: 23, Status: "inactive", BoardID: nil, CreatedAt: now.Add(-time.Hour)},
		{ID: 24, Status: "legacy_inactive", BoardID: nil, CreatedAt: now.Add(-time.Hour)},
	}
	got = ParticipatingEntries(mixed, now, cfg)
	require.Len(t, got, 5)
	for _, pe := range got {
		if pe.Entry.ID == 20 {
			require.True(t, pe.Participates, "29.99d 仍在窗口内")
		} else {
			require.False(t, pe.Participates, "id=%d 不应参与", pe.Entry.ID)
		}
	}
}

func TestParticipatingEntriesSameSecondTieByIDDesc(t *testing.T) {
	now := seedPolicyTestNow()
	cfg := SeedPolicyConfig{WindowDays: 30, MaxEntries: 1, HalfLifeDays: 7, MaturityArticles: 20, SeedBudget: 4}
	same := now.Add(-2 * time.Hour)
	entries := []EntryFacts{
		{ID: 10, Status: "active", CreatedAt: same},
		{ID: 11, Status: "active", CreatedAt: same},
	}
	got := ParticipatingEntries(entries, now, cfg)
	for _, pe := range got {
		require.Equal(t, pe.Entry.ID == 11, pe.Participates, "同秒以 ID 降序破同分：大 ID 参与先")
	}
}

// ── SeedShareAllocation：预算有界 + 最大余数法 + 破同分 ──

func TestSeedShareAllocation(t *testing.T) {
	cfg := DefaultSeedPolicyConfig() // SeedBudget=4

	t.Run("全零权重预算为0", func(t *testing.T) {
		got := SeedShareAllocation([]WeightedEntry{{ID: 1, Weight: 0}, {ID: 2, Weight: 0}}, cfg)
		require.Equal(t, map[uint64]int{1: 0, 2: 0}, got)
	})
	t.Run("单条满权拿满预算 floor(4×1)=4", func(t *testing.T) {
		got := SeedShareAllocation([]WeightedEntry{{ID: 1, Weight: 1}}, cfg)
		require.Equal(t, map[uint64]int{1: 4}, got)
	})
	t.Run("条数增长不越过 floor(4×maxW) 上限", func(t *testing.T) {
		var es []WeightedEntry
		for i := 1; i <= 10; i++ {
			es = append(es, WeightedEntry{ID: uint64(i), Weight: 0.3})
		}
		got := SeedShareAllocation(es, cfg) // 预算 floor(4×0.3)=1
		sum := 0
		for _, v := range got {
			sum += v
		}
		require.Equal(t, 1, sum, "10 条同权条目总份额恒等于预算 1")
	})
	t.Run("最大余数法配额和恒等预算", func(t *testing.T) {
		got := SeedShareAllocation([]WeightedEntry{
			{ID: 1, Weight: 1}, {ID: 2, Weight: 0.34}, {ID: 3, Weight: 0.33}, {ID: 4, Weight: 0.33},
		}, cfg)
		// 预算 4；quota: 2.0/0.68/0.66/0.66 → floor 2/0/0/0，余 2 给余量大的 2、3。
		require.Equal(t, map[uint64]int{1: 2, 2: 1, 3: 1, 4: 0}, got)
	})
	t.Run("同权 ID 升序破同分", func(t *testing.T) {
		got := SeedShareAllocation([]WeightedEntry{
			{ID: 30, Weight: 1}, {ID: 10, Weight: 1}, {ID: 20, Weight: 1},
		}, cfg)
		// 预算 4；quota 各 4/3 → floor 1×3，余 1 同余量给最小 ID 10。
		require.Equal(t, map[uint64]int{10: 2, 20: 1, 30: 1}, got)
	})
	t.Run("0 权条目恒得 0 不转赠", func(t *testing.T) {
		got := SeedShareAllocation([]WeightedEntry{{ID: 1, Weight: 1}, {ID: 2, Weight: 0}}, cfg)
		require.Equal(t, map[uint64]int{1: 4, 2: 0}, got)
	})
	t.Run("budget=0 配置全零", func(t *testing.T) {
		c0 := cfg
		c0.SeedBudget = 0
		got := SeedShareAllocation([]WeightedEntry{{ID: 1, Weight: 1}}, c0)
		require.Equal(t, map[uint64]int{1: 0}, got)
	})
	t.Run("弱权重预算向下取整为0", func(t *testing.T) {
		got := SeedShareAllocation([]WeightedEntry{{ID: 1, Weight: 0.2}}, cfg) // floor(0.8)=0
		require.Equal(t, map[uint64]int{1: 0}, got)
	})
}

// 同龄条目 N 增 → 分配名额不增（spec「行为成熟后让位」的份额层表现）。
func TestSeedShareMonotonicInBehaviorMaturity(t *testing.T) {
	now := seedPolicyTestNow()
	cfg := DefaultSeedPolicyConfig()
	prev := math.Inf(1)
	for n := 0; n <= 22; n += 2 {
		w := InterestWeight(entryAt(1, 3), n, now, cfg)
		// 单条目场景：名额 = floor(4×w)，随 N 单调不增。
		share := SeedShareAllocation([]WeightedEntry{{ID: 1, Weight: w}}, cfg)[1]
		require.LessOrEqual(t, float64(share), prev, "同龄 N=%d 名额不增", n)
		prev = float64(share)
	}
}

// ── MatchBoard：归属规则 ──

func TestMatchBoard(t *testing.T) {
	cfg := DefaultSeedPolicyConfig() // sim 0.78 / margin 0.03

	t.Run("空集返回nil", func(t *testing.T) {
		require.Nil(t, MatchBoard([]float64{1, 0, 0}, nil, cfg))
	})
	t.Run("单候选达标即归属", func(t *testing.T) {
		got := MatchBoard([]float64{1, 0, 0}, []BoardVec{{ID: 7, Vec: []float64{0.8, 0.6, 0}}}, cfg) // sim=0.8
		require.NotNil(t, got)
		require.EqualValues(t, 7, *got)
	})
	t.Run("单候选不达标返回nil", func(t *testing.T) {
		require.Nil(t, MatchBoard([]float64{1, 0, 0}, []BoardVec{{ID: 7, Vec: []float64{0.7071, 0.7071, 0}}}, cfg)) // sim≈0.707
	})
	t.Run("正交向量 sim=0 不归属（sim=1-distance 语义）", func(t *testing.T) {
		require.Nil(t, MatchBoard([]float64{1, 0, 0}, []BoardVec{{ID: 7, Vec: []float64{0, 1, 0}}}, cfg))
	})
	t.Run("多候选领先不足 margin 返回nil", func(t *testing.T) {
		y := math.Sqrt(1 - 0.99*0.99)
		got := MatchBoard([]float64{1, 0, 0}, []BoardVec{
			{ID: 1, Vec: []float64{1, 0, 0}},    // sim=1.0
			{ID: 2, Vec: []float64{0.99, y, 0}}, // sim≈0.99，差 0.01 < 0.03
		}, cfg)
		require.Nil(t, got, "差一 margin：次高太近不归属")
	})
	t.Run("多候选领先达 margin 归属最高", func(t *testing.T) {
		got := MatchBoard([]float64{1, 0, 0}, []BoardVec{
			{ID: 1, Vec: []float64{1, 0, 0}},
			{ID: 2, Vec: []float64{0.8, 0.6, 0}}, // sim=0.8，差 0.2 ≥ 0.03
		}, cfg)
		require.NotNil(t, got)
		require.EqualValues(t, 1, *got)
	})
	t.Run("多候选但最高不达阈值返回nil", func(t *testing.T) {
		got := MatchBoard([]float64{1, 0, 0}, []BoardVec{
			{ID: 1, Vec: []float64{0.7071, 0.7071, 0}}, // sim≈0.707 < 0.78
			{ID: 2, Vec: []float64{0, 1, 0}},           // sim=0
		}, cfg)
		require.Nil(t, got)
	})
}

func TestMatchBoardDimensionMismatchSkipped(t *testing.T) {
	cfg := DefaultSeedPolicyConfig()
	boards := []BoardVec{
		{ID: 1, Vec: []float64{1, 0, 0, 0}}, // 维度 4 ≠ 查询 3 → 跳过
		{ID: 2, Vec: []float64{1, 0, 0}},    // 命中
	}
	id, skipped := MatchBoardDetailed([]float64{1, 0, 0}, boards, cfg)
	require.NotNil(t, id)
	require.EqualValues(t, 2, *id)
	require.Len(t, skipped, 1)
	require.Contains(t, skipped[0], "board 1", "跳过原因须带板 id")

	// 全部维度不一致 → nil 且有原因。
	id, skipped = MatchBoardDetailed([]float64{1, 0, 0}, []BoardVec{{ID: 9, Vec: []float64{1, 0}}}, cfg)
	require.Nil(t, id)
	require.Len(t, skipped, 1)

	// 零向量板参与比较但 sim=0 恒不达标（cosineSim 对零向量返回 ok=true, sim=0）。
	id, skipped = MatchBoardDetailed([]float64{1, 0, 0}, []BoardVec{{ID: 5, Vec: []float64{0, 0, 0}}}, cfg)
	require.Nil(t, id)
	require.Empty(t, skipped)
	// 零向量板与有效板并存：返回有效板。
	id, skipped = MatchBoardDetailed([]float64{1, 0, 0}, []BoardVec{
		{ID: 5, Vec: []float64{0, 0, 0}},
		{ID: 6, Vec: []float64{1, 0, 0}},
	}, cfg)
	require.NotNil(t, id)
	require.EqualValues(t, 6, *id)
}
