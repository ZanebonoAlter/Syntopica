package repository

// 泳道当天热度（retire-story-adopt-event-metrics task 1.3）集成/纯函数测试。
// repository 包一律 testcontainer PG（禁 SQLite — standard/backend/testing.md）。

import (
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/testutil"
)

func seedLaneHeatTopic(t *testing.T, db *gorm.DB, boardID uint, label, status string) BoardPersistentTopic {
	t.Helper()
	topic := BoardPersistentTopic{
		SemanticBoardID: boardID,
		Label:           label,
		Embedding:       FloatsToPgVector([]float64{0}),
		Status:          status,
		Source:          TopicSourceAuto,
		FirstSeenDate:   NormalizeReportDate(time.Now()),
		LastSeenDate:    NormalizeReportDate(time.Now()),
		HitCount:        3,
		ConsecutiveHits: 3,
	}
	require.NoError(t, db.Create(&topic).Error)
	return topic
}

func seedLaneHeatThread(t *testing.T, db *gorm.DB, reportID, sectionID uint, scope string) {
	t.Helper()
	// daily_report_threads.embedding is a non-nullable vector column — mirror
	// production with a 1-dim vector (same as the other lane tests).
	require.NoError(t, db.Create(&DailyReportThread{
		ReportID:  reportID,
		SectionID: sectionID,
		Title:     "线索",
		Scope:     scope,
		Embedding: FloatsToPgVector([]float64{0}),
	}).Error)
}

func seedLaneHeatSection(t *testing.T, db *gorm.DB, reportID, topicID uint, label string) uint {
	t.Helper()
	sectionID := seedTestSection(t, db, reportID, label)
	require.NoError(t, db.Model(&DailyReportSection{}).
		Where("id = ?", sectionID).
		Update("persistent_topic_id", topicID).Error)
	return sectionID
}

// TestLaneHeatPureFunction 覆盖白盒分支表 A 的函数层部分：活跃 ×1.0、
// 候选 ×权重、真零不是未知、权重 0 / 1 边界、坏权重回退默认、确定性。
func TestLaneHeatPureFunction(t *testing.T) {
	cases := []struct {
		name   string
		status string
		clues  int
		weight float64
		want   float64
	}{
		{"active_6", TopicStatusActive, 6, 0.5, 6.0},
		{"candidate_4_w05", TopicStatusCandidate, 4, 0.5, 2.0},
		{"candidate_4_w02", TopicStatusCandidate, 4, 0.2, 0.8},
		{"both_zero_is_zero_not_unknown", TopicStatusActive, 0, 0.5, 0},
		{"candidate_zero", TopicStatusCandidate, 0, 0.5, 0},
		{"candidate_weight_zero", TopicStatusCandidate, 4, 0, 0},
		{"candidate_weight_one", TopicStatusCandidate, 4, 1, 4.0},
		{"bad_weight_falls_back_to_default", TopicStatusCandidate, 4, 1.5, 2.0},
		{"nan_weight_falls_back_to_default", TopicStatusCandidate, 4, math.NaN(), 2.0},
		{"inf_weight_falls_back_to_default", TopicStatusCandidate, 4, math.Inf(1), 2.0},
		{"active_ignores_weight", TopicStatusActive, 6, 1, 6.0},
		{"negative_clues_are_zero", TopicStatusActive, -3, 0.5, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ComputeLaneHeat(tc.status, tc.clues, tc.weight)
			assert.InDelta(t, tc.want, got, 1e-9)
			// 确定性：同一输入重复计算同值。
			assert.Equal(t, got, ComputeLaneHeat(tc.status, tc.clues, tc.weight))
		})
	}
}

// TestLaneHeatBoardAttention 关注度 = Σ 泳道热度，保留一位小数。
func TestLaneHeatBoardAttention(t *testing.T) {
	assert.InDelta(t, 8.0, ComputeBoardAttention([]float64{6.0, 2.0}), 1e-9)
	assert.InDelta(t, 0.8, ComputeBoardAttention([]float64{0.8}), 1e-9)
	assert.InDelta(t, 0.4, ComputeBoardAttention([]float64{0.2, 0.2}), 1e-9)
	// 一位小数：0.33 + 0.33 + 0.33 = 0.99 → 1.0
	assert.InDelta(t, 1.0, ComputeBoardAttention([]float64{0.33, 0.33, 0.33}), 1e-9)
	assert.Zero(t, ComputeBoardAttention(nil))
}

// TestLaneHeatCountsCoverCandidateAndExcludeComposite（task 1.3 查询层）：
// 覆盖 active + candidate 两类泳道；综合稿不计；分桶合并取合计不取峰值；
// 只算当天（前一天线索不混入）。
func TestLaneHeatCountsCoverCandidateAndExcludeComposite(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repo := NewTopicGraphRepository(db)

	boardID := seedTestBoard(t, db)
	today := NormalizeReportDate(time.Now())
	yesterday := today.AddDate(0, 0, -1)
	reportToday := seedTestReport(t, db, boardID, today)
	reportYesterday := seedTestReport(t, db, boardID, yesterday)

	active := seedLaneHeatTopic(t, db, boardID, "活跃泳道", TopicStatusActive)
	candidate := seedLaneHeatTopic(t, db, boardID, "候选泳道", TopicStatusCandidate)
	archived := seedLaneHeatTopic(t, db, boardID, "归档泳道", TopicStatusArchived)

	// 活跃泳道：当天两个桶 3 + 4 = 7（不取峰值 4），另加 2 条综合稿（不计）。
	bucketA := seedLaneHeatSection(t, db, reportToday, active.ID, "桶A")
	bucketB := seedLaneHeatSection(t, db, reportToday, active.ID, "桶B")
	for i := 0; i < 3; i++ {
		seedLaneHeatThread(t, db, reportToday, bucketA, ThreadScopeSingle)
	}
	for i := 0; i < 4; i++ {
		seedLaneHeatThread(t, db, reportToday, bucketB, ThreadScopeSingle)
	}
	seedLaneHeatThread(t, db, reportToday, bucketA, ThreadScopeComposite)
	seedLaneHeatThread(t, db, reportToday, bucketB, ThreadScopeComposite)

	// 候选泳道：当天 4 条线索（候选也必须有当天线索数，design D7）。
	candSection := seedLaneHeatSection(t, db, reportToday, candidate.ID, "候选桶")
	for i := 0; i < 4; i++ {
		seedLaneHeatThread(t, db, reportToday, candSection, ThreadScopeSingle)
	}

	// 归档泳道：有线索但不参与热度。
	archSection := seedLaneHeatSection(t, db, reportToday, archived.ID, "归档桶")
	seedLaneHeatThread(t, db, reportToday, archSection, ThreadScopeSingle)

	// 前一天：活跃泳道 20 条线索必须不计入当天热度（只表达当天）。
	oldSection := seedLaneHeatSection(t, db, reportYesterday, active.ID, "昨天桶")
	for i := 0; i < 20; i++ {
		seedLaneHeatThread(t, db, reportYesterday, oldSection, ThreadScopeSingle)
	}

	clues, err := repo.CountLaneCluesByDate(boardID, today)
	require.NoError(t, err)
	assert.Equal(t, 7, clues[active.ID], "分桶取合计 7，不取桶峰值 4；综合稿不计")
	assert.Equal(t, 4, clues[candidate.ID], "候选泳道同样要能取到当天线索数")
	assert.Equal(t, 1, clues[archived.ID])

	heats, err := repo.LaneHeatsByDate(boardID, today, 0.5)
	require.NoError(t, err)
	assert.InDelta(t, 7.0, heats[active.ID], 1e-9)
	assert.InDelta(t, 2.0, heats[candidate.ID], 1e-9, "4 条线索 × 0.5")
	_, archivedHeated := heats[archived.ID]
	assert.False(t, archivedHeated, "归档话题不是泳道，不参与当天热度")

	// 权重在线调整后立即生效；活跃泳道不受影响。
	heats, err = repo.LaneHeatsByDate(boardID, today, 0.2)
	require.NoError(t, err)
	assert.InDelta(t, 0.8, heats[candidate.ID], 1e-9, "4 × 0.2 = 0.8")
	assert.InDelta(t, 7.0, heats[active.ID], 1e-9, "活跃泳道热度不变")

	// 权重 0：候选不贡献热度，但候选话题本体不被隐藏 / 归档 / 转正。
	heats, err = repo.LaneHeatsByDate(boardID, today, 0)
	require.NoError(t, err)
	assert.Zero(t, heats[candidate.ID])
	var after BoardPersistentTopic
	require.NoError(t, db.First(&after, candidate.ID).Error)
	assert.Equal(t, TopicStatusCandidate, after.Status, "候选权重不得影响话题生命周期")
	assert.Equal(t, 3, after.HitCount)

	// 确定性：同一份数据重复计算同值；历史日可回算。
	again, err := repo.LaneHeatsByDate(boardID, today, 0.5)
	require.NoError(t, err)
	assert.InDelta(t, heats[candidate.ID]+2.0, again[candidate.ID], 1e-9)
	yesterdayHeats, err := repo.LaneHeatsByDate(boardID, yesterday, 0.5)
	require.NoError(t, err)
	assert.InDelta(t, 20.0, yesterdayHeats[active.ID], 1e-9, "历史日按当时的报告几何回算")
	assert.Zero(t, yesterdayHeats[candidate.ID], "候选泳道当天无线索")
}

// TestLaneHeatCandidateWeightSetting（task 1.3）：权重读 ai_settings，缺项 0.5、
// 坏值告警回退 0.5、域 [0,1]。
func TestLaneHeatCandidateWeightSetting(t *testing.T) {
	db := testutil.SetupTestDB(t)

	require.InDelta(t, DefaultLaneHeatCandidateWeight, LoadLaneHeatCandidateWeight(db), 1e-9, "缺项回退默认 0.5")

	require.NoError(t, db.Create(&models.AISettings{Key: LaneHeatCandidateWeightKey, Value: "0.2"}).Error)
	require.InDelta(t, 0.2, LoadLaneHeatCandidateWeight(db), 1e-9)

	require.NoError(t, db.Model(&models.AISettings{}).Where("key = ?", LaneHeatCandidateWeightKey).Update("value", "1.5").Error)
	require.InDelta(t, DefaultLaneHeatCandidateWeight, LoadLaneHeatCandidateWeight(db), 1e-9, "越界存量值回退默认")
	require.NoError(t, db.Model(&models.AISettings{}).Where("key = ?", LaneHeatCandidateWeightKey).Update("value", "NaN").Error)
	require.InDelta(t, DefaultLaneHeatCandidateWeight, LoadLaneHeatCandidateWeight(db), 1e-9, "非有限数回退默认")
	require.NoError(t, db.Model(&models.AISettings{}).Where("key = ?", LaneHeatCandidateWeightKey).Update("value", " ").Error)
	require.InDelta(t, DefaultLaneHeatCandidateWeight, LoadLaneHeatCandidateWeight(db), 1e-9, "空串回退默认")

	for _, raw := range []string{"0", "0.0001", "0.5", "0.9999", "1"} {
		value, ok := ParseLaneHeatCandidateWeight(raw)
		require.True(t, ok, raw)
		require.InDelta(t, mustFloat(t, raw), value, 1e-12)
	}
	for _, raw := range []string{"-0.0001", "1.0001", "NaN", "+Inf", "", " "} {
		value, ok := ParseLaneHeatCandidateWeight(raw)
		require.False(t, ok, raw)
		require.InDelta(t, DefaultLaneHeatCandidateWeight, value, 1e-12)
	}
}

func mustFloat(t *testing.T, raw string) float64 {
	t.Helper()
	value, ok := ParseLaneHeatCandidateWeight(raw)
	require.True(t, ok)
	return value
}
