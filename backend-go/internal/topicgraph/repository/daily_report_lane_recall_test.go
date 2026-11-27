package repository

import (
	"fmt"
	"math"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"syntopica-backend/internal/models"
)

// ── 泳道优先召回：纯逻辑单测（lane-first-recall）──────────────────────────────
//
// 这些用例不需要 Postgres：召回决策（距离上限 / Top-K）、多泳道同标签取最近、
// 泳道向量可用性判定、参数解析与回退都是纯函数。SQL 侧的 ANN 召回 + 精确复核
// 由 review.md 记录的真库离线核对（2026-11-26 board 15220）与归档前的集成套件
// 覆盖——本仓库红线：含副作用的 SQL 不碰真库。

// ── 场景「召回参数可在线覆盖」+「参数缺失/非法回退默认」 ──

func TestLaneRecallConfigDefaults(t *testing.T) {
	want := LaneRecallConfig{TopK: 8, MaxDistance: 0.16}
	assert.Equal(t, want, DefaultLaneRecallConfig())
	// No rows at all (fresh deployment before the seed migration) keeps defaults.
	assert.Equal(t, want, parseLaneRecallConfig(nil))
	assert.Equal(t, want, parseLaneRecallConfig(map[string]string{}))
}

func TestLaneRecallConfigOnlineOverride(t *testing.T) {
	got := parseLaneRecallConfig(map[string]string{
		"lane_recall_top_k":        "12",
		"lane_recall_max_distance": "0.2",
	})
	assert.Equal(t, 12, got.TopK)
	assert.InDelta(t, 0.2, got.MaxDistance, 1e-9)
}

func TestLaneRecallConfigInvalidFallsBack(t *testing.T) {
	cases := []struct {
		name     string
		topK     string
		maxDist  string
		wantTopK int
		wantDist float64
	}{
		{"non numeric", "abc", "xyz", 8, 0.16},
		{"zero top k", "0", "0", 8, 0.16},
		{"negative top k", "-4", "-0.1", 8, 0.16},
		{"top k above ceiling", "10000", "0.5", 8, 0.5},
		{"distance above one", "8", "1.7", 8, 0.16},
		{"blank", "  ", "  ", 8, 0.16},
		{"empty string", "", "", 8, 0.16},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := parseLaneRecallConfig(map[string]string{
				"lane_recall_top_k":        tc.topK,
				"lane_recall_max_distance": tc.maxDist,
			})
			assert.Equal(t, tc.wantTopK, got.TopK)
			assert.InDelta(t, tc.wantDist, got.MaxDistance, 1e-9)
		})
	}
}

// TestLaneRecallConfigLoadFromSettings covers the DB read path: an empty
// ai_settings table yields the defaults (no error, no panic), and a seeded row
// overrides only its own field.
func TestLaneRecallConfigLoadFromSettings(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:lane_recall_cfg?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.AISettings{}))

	assert.Equal(t, DefaultLaneRecallConfig(), LoadLaneRecallConfig(db), "missing rows must keep defaults")

	require.NoError(t, db.Create(&models.AISettings{Key: "lane_recall_top_k", Value: "5"}).Error)
	got := LoadLaneRecallConfig(db)
	assert.Equal(t, 5, got.TopK, "seeded value must win")
	assert.InDelta(t, 0.16, got.MaxDistance, 1e-9, "untouched key keeps its default")

	require.NoError(t, db.Model(&models.AISettings{}).Where("key = ?", "lane_recall_top_k").
		Update("value", "not-a-number").Error)
	assert.Equal(t, 8, LoadLaneRecallConfig(db).TopK, "invalid value falls back to default")
}

// ── 场景「当天没有专属新闻的泳道保持空白」+ Top-K / 距离上限截断 ──

func TestLaneRecallDecisionDistanceCeiling(t *testing.T) {
	cands := []LaneRecalledTag{
		{TagID: 1, LaneID: 2687, Distance: 0.075},
		{TagID: 2, LaneID: 2687, Distance: 0.159},
		{TagID: 3, LaneID: 2687, Distance: 0.16}, // exactly at the ceiling → excluded
		{TagID: 4, LaneID: 2687, Distance: 0.241},
	}
	got := recallDecision(cands, 8, 0.16)
	require.Len(t, got, 2)
	assert.Equal(t, uint(1), got[0].TagID)
	assert.Equal(t, uint(2), got[1].TagID)
}

func TestLaneRecallDecisionEmptyLane(t *testing.T) {
	// The measured "hungry lane" shape: rank1 at 0.264 (中东冲突波及旅游业: Booking).
	got := recallDecision([]LaneRecalledTag{
		{TagID: 9, LaneID: 1974, Distance: 0.264},
		{TagID: 10, LaneID: 1974, Distance: 0.301},
	}, 8, 0.16)
	assert.Empty(t, got, "a lane with no dedicated news recalls nothing instead of pulling neighbours in")
	assert.Empty(t, recallDecision(nil, 8, 0.16))
}

func TestLaneRecallDecisionTopK(t *testing.T) {
	cands := make([]LaneRecalledTag, 0, 10)
	for i := 1; i <= 10; i++ {
		cands = append(cands, LaneRecalledTag{TagID: uint(i), LaneID: 2687, Distance: 0.05 + float64(i)*0.005})
	}
	got := recallDecision(cands, 8, 0.16)
	require.Len(t, got, 8, "top_k caps the day's new leads per lane")
	assert.Equal(t, uint(1), got[0].TagID)
	assert.Equal(t, uint(8), got[7].TagID)

	// top_k larger than the admitted set keeps the admitted set intact.
	assert.Len(t, recallDecision(cands, 32, 0.16), 10)
	// A useless config recalls nothing rather than defaulting silently.
	assert.Empty(t, recallDecision(cands, 0, 0.16))
	assert.Empty(t, recallDecision(cands, 8, 0))

	// The decision sorts internally: an unsorted caller cannot break the cut.
	shuffled := []LaneRecalledTag{
		{TagID: 3, Distance: 0.14},
		{TagID: 1, Distance: 0.09},
		{TagID: 2, Distance: 0.11},
	}
	got = recallDecision(shuffled, 2, 0.16)
	require.Len(t, got, 2)
	assert.Equal(t, uint(1), got[0].TagID)
	assert.Equal(t, uint(2), got[1].TagID)
	assert.Equal(t, []uint{3, 1, 2}, []uint{shuffled[0].TagID, shuffled[1].TagID, shuffled[2].TagID},
		"the caller's slice must not be reordered in place")
}

// TestLaneRecallMergeNearestLaneWins covers the batch-level merge rule: a tag
// inside two lanes' radius keeps the nearest witness (the final lane decision is
// re-made downstream by the L1/L2 bucketing).
func TestLaneRecallMergeNearestLaneWins(t *testing.T) {
	recalled := make(map[uint]LaneRecalledTag)
	mergeNearestLaneRecall(recalled, []LaneRecalledTag{
		{TagID: 303080, LaneID: 2687, Distance: 0.1556},
	})
	mergeNearestLaneRecall(recalled, []LaneRecalledTag{
		{TagID: 303080, LaneID: 2688, Distance: 0.1043},
		{TagID: 303909, LaneID: 2688, Distance: 0.1181},
	})
	require.Len(t, recalled, 2)
	assert.Equal(t, uint(2688), recalled[303080].LaneID)
	assert.InDelta(t, 0.1043, recalled[303080].Distance, 1e-9)
	assert.Equal(t, uint(2688), recalled[303909].LaneID)

	// A farther witness never overwrites a nearer one.
	mergeNearestLaneRecall(recalled, []LaneRecalledTag{
		{TagID: 303080, LaneID: 2687, Distance: 0.2},
	})
	assert.Equal(t, uint(2688), recalled[303080].LaneID)
}

func TestLaneRecallSortHitsDeterministic(t *testing.T) {
	hits := []LaneRecalledTag{
		{TagID: 30, Distance: 0.15},
		{TagID: 10, Distance: 0.15},
		{TagID: 20, Distance: 0.10},
	}
	SortLaneRecallHits(hits)
	assert.Equal(t, []uint{20, 10, 30}, []uint{hits[0].TagID, hits[1].TagID, hits[2].TagID})
}

// ── 场景「泳道向量缺失时跳过该泳道」 ──

func TestLaneRecallSkipsLaneWithoutVector(t *testing.T) {
	assert.Nil(t, laneRecallAnchor("", ""), "no centroid and no embedding")
	assert.Nil(t, laneRecallAnchor("NULL", ""), "unparseable centroid")
	assert.Nil(t, laneRecallAnchor("[]", "[]"), "empty vectors")
	assert.False(t, laneRecallAnchorUsable(nil))
	assert.False(t, laneRecallAnchorUsable([]float64{}))

	// Centroid wins (the same anchor the L1/L2 bucketing uses), embedding is the
	// degradation path.
	assert.InDeltaSlice(t, []float64{0.25, 0.5}, laneRecallAnchor("[0.25,0.5]", "[1,0]"), 1e-9)
	assert.InDeltaSlice(t, []float64{1, 0}, laneRecallAnchor("", "[1,0]"), 1e-9)

	// Dimension mismatch against the tag vector space is not repairable here: an
	// over-long vector cannot use the halfvec HNSW index, so the lane is skipped
	// (logged) instead of falling back to a full float32 scan of the whole pool.
	assert.True(t, laneRecallAnchorUsable(make([]float64, 2560)))
	assert.False(t, laneRecallAnchorUsable(make([]float64, laneRecallHalfvecMaxDim+1)))
	assert.False(t, laneRecallAnchorUsable([]float64{math.NaN(), 0}))
}

// TestLaneRecallPgVectorRoundTrip pins that the query parameter the recall uses
// is the same textual pgvector literal the rest of the package writes.
func TestLaneRecallPgVectorRoundTrip(t *testing.T) {
	raw := FloatsToPgVector([]float64{0.125, -0.5})
	got, err := repoParsePgVector(raw)
	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.InDelta(t, 0.125, got[0], 1e-6)
	assert.InDelta(t, -0.5, got[1], 1e-6)
}

// TestLaneRecallCandidateSQLUsesIndexedExpression is a static guard on the one
// property the ANN index depends on: stage 1 must cast to halfvec WITH the
// dimension typmod (a bare ::halfvec never uses the semantic halfvec HNSW
// index) and stage 2 must read the original float32 column.
func TestLaneRecallCandidateSQLUsesIndexedExpression(t *testing.T) {
	formatted := fmt.Sprintf(laneRecallAnnSQLTemplate, 2560, 2560)
	assert.Contains(t, formatted, "ORDER BY e.embedding::halfvec(2560) <=> ?::halfvec(2560)")
	assert.NotContains(t, formatted, "::halfvec <=>", "a bare halfvec cast would bypass the index")
	assert.Contains(t, formatted, "e.embedding_type = 'semantic'")
	assert.Contains(t, formatted, "t.status = 'active'")
	assert.Contains(t, formatted, "a.pub_date >= ? AND a.pub_date < ?")

	assert.Contains(t, laneRecallExactSQL, "e.embedding <=> ?::vector")
	assert.NotContains(t, laneRecallExactSQL, "halfvec", "the adjudicating stage must use the exact column")
}

// TestLaneRecallCandidateSQLExcludesDirectionMismatch is the regression guard for
// review R-9: the lane channel MUST NOT re-admit a tag that the board channel
// excluded for direction mismatch. Without this clause the lane channel silently
// bypassed semantic-board red line 4 (mismatch recorded but not reported) —
// measured on 2026-11-26 board 15220: 12 mismatch tags existed that day, 1 of
// them within the default 0.16 ceiling, 8 within 0.20.
func TestLaneRecallCandidateSQLExcludesDirectionMismatch(t *testing.T) {
	ann := fmt.Sprintf(laneRecallAnnSQLTemplate, 2560, 2560)
	exact := laneRecallExactSQL
	for name, sql := range map[string]string{"ann": ann, "exact": exact} {
		assert.Contains(t, sql, "NOT EXISTS", "%s stage must exclude direction-mismatch tags", name)
		assert.Contains(t, sql, "COALESCE(bl.direction_mismatch, false)",
			"%s stage must treat NULL mismatch as false", name)
		assert.Contains(t, sql, "bl.semantic_board_id = ?",
			"%s stage must scope the mismatch check to the lane's own board", name)
	}
}
