package repository

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ── 首义读端点扩展：anchor_origin / centroid ────────────────────────────────
// align-lane-correction-workface tasks 1.2 / 1.3 / 1.5（spec「读端点返回来源与
// 滚动窗口」「自动首义不可考时显式不可考」「读端点零写副作用」）。
//
// 契约要点：
//   - 来源不可考即不可考（resolved=false + 节点 null），绝不拿「最早成员」顶替；
//   - centroid 段与质心计算共用同一份窗口取数实现；
//   - 读端点零写副作用（不写表、不碰 centroid/embedding、不置待重算标记）。

// TestAnchorReadExt_AutoAnchorSourceIsNotResolvable covers the auto case: the
// 首义 was written when the topic was created — a single vector with no node
// list — so the source node is **not** knowable and the read side says so
// explicitly instead of inventing one.
func TestAnchorReadExt_AutoAnchorSourceIsNotResolvable(t *testing.T) {
	repo := setupManualTopicTestDB(t)
	boardID := seedTestBoard(t, repo.db)
	reportID := seedTestReport(t, repo.db, boardID, time.Now())
	topic := seedAnchorTopic(t, repo.db, boardID, "lane A", TopicStatusActive, []float64{0, 0, 1})
	seedAnchorSection(t, repo.db, reportID, "s1", []float64{1, 0, 0}, &topic.ID)

	state, err := repo.GetTopicAnchorState(topic.ID)
	require.NoError(t, err)

	assert.Equal(t, TopicAnchorSourceAuto, state.Source)
	assert.Empty(t, state.SectionIDs)
	assert.Zero(t, state.OverrideCount)
	assert.Empty(t, state.EffectiveFrom)

	// 来源不可考：resolved=false 且三个节点字段一律 null。
	assert.False(t, state.AnchorOrigin.Resolved)
	assert.Nil(t, state.AnchorOrigin.SectionID)
	assert.Nil(t, state.AnchorOrigin.Title)
	assert.Nil(t, state.AnchorOrigin.PeriodDate)
}

// TestAnchorReadExt_ManualAnchorResolvesOriginNode covers the manual case:
// the trail records the chosen nodes, so the source node is knowable and the
// read side reports it with title and report period.
func TestAnchorReadExt_ManualAnchorResolvesOriginNode(t *testing.T) {
	repo := setupManualTopicTestDB(t)
	boardID := seedTestBoard(t, repo.db)
	reportID := seedTestReport(t, repo.db, boardID, time.Now())
	topic := seedAnchorTopic(t, repo.db, boardID, "lane A", TopicStatusActive, []float64{0, 0, 1})
	s1 := seedAnchorSection(t, repo.db, reportID, "first pick", []float64{1, 0, 0}, &topic.ID)
	s2 := seedAnchorSection(t, repo.db, reportID, "second pick", []float64{0, 1, 0}, &topic.ID)

	_, err := repo.SetTopicAnchorFromSections(topic.ID, []uint{s1.ID, s2.ID}, AnchorOperatorUser)
	require.NoError(t, err)

	state, err := repo.GetTopicAnchorState(topic.ID)
	require.NoError(t, err)

	assert.Equal(t, TopicAnchorSourceManual, state.Source)
	assert.Equal(t, []uint{s1.ID, s2.ID}, state.SectionIDs)
	assert.Equal(t, 1, state.OverrideCount)
	assert.Equal(t, AnchorEffectiveFromNextReport, state.EffectiveFrom)

	assert.True(t, state.AnchorOrigin.Resolved)
	require.NotNil(t, state.AnchorOrigin.SectionID)
	assert.Equal(t, s1.ID, *state.AnchorOrigin.SectionID)
	require.NotNil(t, state.AnchorOrigin.Title)
	assert.Equal(t, "first pick", *state.AnchorOrigin.Title)
	require.NotNil(t, state.AnchorOrigin.PeriodDate)

	var report BoardDailyReport
	require.NoError(t, repo.db.First(&report, reportID).Error)
	assert.Equal(t, report.PeriodDate.UTC().Format("2006-01-02"), *state.AnchorOrigin.PeriodDate)
}

// TestAnchorReadExt_OriginSkipsDeletedTrailNodes covers the single-round-trip
// origin lookup (review R-7): the trail's first node no longer exists, so the
// source node falls to the next **surviving** node in SectionIDs order — the
// same verdict the former per-node loop produced, from one `WHERE id IN ?`.
func TestAnchorReadExt_OriginSkipsDeletedTrailNodes(t *testing.T) {
	repo := setupManualTopicTestDB(t)
	boardID := seedTestBoard(t, repo.db)
	reportID := seedTestReport(t, repo.db, boardID, time.Now())
	topic := seedAnchorTopic(t, repo.db, boardID, "lane A", TopicStatusActive, []float64{0, 0, 1})
	s2 := seedAnchorSection(t, repo.db, reportID, "second pick", []float64{0, 1, 0}, &topic.ID)

	// 留痕：第一个节点在表里不存在（已被删除），第二个仍在。
	const deletedSectionID uint = 987654
	require.NoError(t, repo.db.Create(&TopicAnchorOverride{
		TopicID:      topic.ID,
		NewEmbedding: FloatsToPgVector([]float64{0, 1, 0}),
		Source:       TopicAnchorSourceSelectedSections,
		InputSummary: fmt.Sprintf("[%d,%d]", deletedSectionID, s2.ID),
		Operator:     AnchorOperatorUser,
	}).Error)

	state, err := repo.GetTopicAnchorState(topic.ID)
	require.NoError(t, err)
	assert.Equal(t, TopicAnchorSourceManual, state.Source)
	assert.Equal(t, []uint{deletedSectionID, s2.ID}, state.SectionIDs)

	// 顺序优先 + 删除跳过：第一个存在的留痕节点 = s2。
	assert.True(t, state.AnchorOrigin.Resolved)
	require.NotNil(t, state.AnchorOrigin.SectionID)
	assert.Equal(t, s2.ID, *state.AnchorOrigin.SectionID)
	require.NotNil(t, state.AnchorOrigin.Title)
	assert.Equal(t, "second pick", *state.AnchorOrigin.Title)
	require.NotNil(t, state.AnchorOrigin.PeriodDate)
}

// TestAnchorReadExt_CentroidBlockDescribesRollingWindow covers the centroid
// block: window size + α from the existing config, the participating sections
// (same order as the members detail) and the not-degraded verdict.
func TestAnchorReadExt_CentroidBlockDescribesRollingWindow(t *testing.T) {
	repo := setupManualTopicTestDB(t)
	boardID := seedTestBoard(t, repo.db)
	reportID := seedTestReport(t, repo.db, boardID, time.Now())
	topic := seedAnchorTopic(t, repo.db, boardID, "lane A", TopicStatusActive, []float64{0, 0, 1})
	s1 := seedAnchorSection(t, repo.db, reportID, "s1", []float64{1, 0, 0}, &topic.ID)
	s2 := seedAnchorSection(t, repo.db, reportID, "s2", []float64{0, 1, 0}, &topic.ID)
	s3 := seedAnchorSection(t, repo.db, reportID, "s3", []float64{0, 0, 1}, &topic.ID)

	state, err := repo.GetTopicAnchorState(topic.ID)
	require.NoError(t, err)

	cfg := LoadPersistentTopicConfig(repo.db)
	assert.Equal(t, cfg.CentroidWindow, state.Centroid.WindowSize)
	assert.Equal(t, DefaultPersistentTopicConfig().CentroidWindow, state.Centroid.WindowSize,
		"window size comes from the existing config (default 30), never hardcoded elsewhere")
	assert.InDelta(t, cfg.CentroidAnchorWeight, state.Centroid.AnchorWeight, 1e-9)
	assert.False(t, state.Centroid.Degraded, "3 parseable vectors + a usable 首义 → not degraded")

	require.Len(t, state.Centroid.SectionIDs, 3)
	require.Len(t, state.Centroid.Members, 3)
	assert.Equal(t, len(state.Centroid.SectionIDs), state.Centroid.SectionCount,
		"section_count = len(section_ids)（文档示例可复现，不靠客户端自己数）")
	assert.ElementsMatch(t, []uint{s1.ID, s2.ID, s3.ID}, state.Centroid.SectionIDs)
	titles := map[uint]string{s1.ID: "s1", s2.ID: "s2", s3.ID: "s3"}
	for i, id := range state.Centroid.SectionIDs {
		assert.Equal(t, id, state.Centroid.Members[i].SectionID)
		assert.Equal(t, titles[id], state.Centroid.Members[i].Title)
		assert.NotEmpty(t, state.Centroid.Members[i].PeriodDate)
	}
}

// TestAnchorReadExt_CentroidDegradedWhenWindowTooSmall covers the degraded
// verdict: fewer than 2 parseable vectors means the centroid fell back to the
// 首义 vector.
func TestAnchorReadExt_CentroidDegradedWhenWindowTooSmall(t *testing.T) {
	repo := setupManualTopicTestDB(t)
	boardID := seedTestBoard(t, repo.db)
	reportID := seedTestReport(t, repo.db, boardID, time.Now())
	topic := seedAnchorTopic(t, repo.db, boardID, "lane A", TopicStatusActive, []float64{0, 0, 1})
	seedAnchorSection(t, repo.db, reportID, "s1", []float64{1, 0, 0}, &topic.ID)

	state, err := repo.GetTopicAnchorState(topic.ID)
	require.NoError(t, err)

	assert.True(t, state.Centroid.Degraded, "one parseable vector → 质心退回首义向量")
	assert.Len(t, state.Centroid.SectionIDs, 1)
}

// TestAnchorReadExt_CentroidDegradedWhenAnchorUnusable covers the second
// degraded cause: the 首义 itself is unusable (empty column), so the centroid
// cannot be the α-blend.
func TestAnchorReadExt_CentroidDegradedWhenAnchorUnusable(t *testing.T) {
	repo := setupManualTopicTestDB(t)
	boardID := seedTestBoard(t, repo.db)
	reportID := seedTestReport(t, repo.db, boardID, time.Now())
	// A topic whose 首义 column is NULL — the anchor is unusable.
	topic := BoardPersistentTopic{
		SemanticBoardID: boardID,
		Label:           "lane-no-anchor",
		Status:          TopicStatusActive,
		Source:          TopicSourceAuto,
		FirstSeenDate:   NormalizeReportDate(time.Now()),
		LastSeenDate:    NormalizeReportDate(time.Now()),
		HitCount:        1,
	}
	require.NoError(t, repo.db.Omit("embedding").Create(&topic).Error)
	seedAnchorSection(t, repo.db, reportID, "s1", []float64{1, 0, 0}, &topic.ID)
	seedAnchorSection(t, repo.db, reportID, "s2", []float64{0, 1, 0}, &topic.ID)

	state, err := repo.GetTopicAnchorState(topic.ID)
	require.NoError(t, err)

	assert.True(t, state.Centroid.Degraded, "首义不可用 → 质心当前退回首义向量")
	assert.Len(t, state.Centroid.SectionIDs, 2)
}

// TestAnchorReadExt_ReadHasNoWriteSideEffects covers the zero-write red line:
// reading the anchor must not write any table, must not touch the
// centroid/embedding columns, must not flag a recompute and must not backfill
// any snapshot.
func TestAnchorReadExt_ReadHasNoWriteSideEffects(t *testing.T) {
	repo := setupManualTopicTestDB(t)
	boardID := seedTestBoard(t, repo.db)
	reportID := seedTestReport(t, repo.db, boardID, time.Now())
	topic := seedAnchorTopic(t, repo.db, boardID, "lane A", TopicStatusActive, []float64{0, 0, 1})
	s1 := seedAnchorSection(t, repo.db, reportID, "s1", []float64{1, 0, 0}, &topic.ID)
	seedAnchorSection(t, repo.db, reportID, "s2", []float64{0, 1, 0}, &topic.ID)
	// A persisted centroid + a pending flag are already there — the read must
	// leave both exactly as they are.
	require.NoError(t, repo.db.Model(&BoardPersistentTopic{}).Where("id = ?", topic.ID).
		Updates(map[string]any{
			"centroid":                   FloatsToPgVector([]float64{0.3, 0.3, 0.3}),
			"centroid_pending_recompute": true,
		}).Error)

	before := snapshotAnchorSideEffects(t, repo, topic.ID, s1.ID)
	state, err := repo.GetTopicAnchorState(topic.ID)
	require.NoError(t, err)
	require.NotNil(t, state)
	after := snapshotAnchorSideEffects(t, repo, topic.ID, s1.ID)

	assert.Equal(t, before, after, "reading the anchor must not change any row, value or flag")
}

// anchorSideEffectSnapshot is the "before/after" fingerprint of everything a
// write side could touch: the topic's vector columns + pending flag, the
// section's assignment snapshot, and the row counts of the touched tables.
type anchorSideEffectSnapshot struct {
	Embedding          string
	Centroid           string
	PendingRecompute   bool
	SectionTopicID     uint
	SectionDistance    float64
	SectionStatusAtRpt *string
	TopicCount         int64
	SectionCount       int64
	OverrideCount      int64
	AssignmentTrailCnt int64
}

func snapshotAnchorSideEffects(t *testing.T, repo *TopicGraphRepository, topicID, sectionID uint) anchorSideEffectSnapshot {
	t.Helper()
	var topic BoardPersistentTopic
	require.NoError(t, repo.db.First(&topic, topicID).Error)
	var section DailyReportSection
	require.NoError(t, repo.db.First(&section, sectionID).Error)

	var sectionTopicID uint
	if section.PersistentTopicID != nil {
		sectionTopicID = *section.PersistentTopicID
	}
	snap := anchorSideEffectSnapshot{
		Embedding:          topic.Embedding,
		Centroid:           topic.Centroid,
		PendingRecompute:   topic.CentroidPendingRecompute,
		SectionTopicID:     sectionTopicID,
		SectionDistance:    section.TopicMatchDistance,
		SectionStatusAtRpt: section.TopicStatusAtReport,
	}
	require.NoError(t, repo.db.Model(&BoardPersistentTopic{}).Count(&snap.TopicCount).Error)
	require.NoError(t, repo.db.Model(&DailyReportSection{}).Count(&snap.SectionCount).Error)
	require.NoError(t, repo.db.Model(&TopicAnchorOverride{}).Count(&snap.OverrideCount).Error)
	require.NoError(t, repo.db.Model(&SectionAssignmentOverride{}).Count(&snap.AssignmentTrailCnt).Error)
	return snap
}
