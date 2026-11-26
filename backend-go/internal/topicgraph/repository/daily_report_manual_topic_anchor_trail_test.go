package repository

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// ── 手动建泳道的首义留痕（tasks 3.1 / 3.2 / 3.3） ─────────────────────────────
// 契约（spec persistent-topic「建话题时写入首义留痕」「留痕写入失败整体回滚」
// 「历史手动泳道不回填」）：
//   - 留痕与建话题同事务，input_summary = 实际参与聚合的 section id；
//   - 留痕失败 → 整个事务回滚，不留「有话题、无留痕」的半成品；
//   - 历史手动泳道继续按「来源不可考」读，不回填。

// TestManualTopicAnchorTrail_WritesTrailRowWithParticipatingSections covers
// spec「建话题时写入首义留痕」+「留痕只记参与聚合的节点」: the trail row is
// appended inside the creation transaction and records exactly the sections
// that took part in the mean pooling (a section without a vector is skipped
// and must not show up in the trail).
func TestManualTopicAnchorTrail_WritesTrailRowWithParticipatingSections(t *testing.T) {
	repo := setupManualTopicTestDB(t)
	boardID := seedTestBoard(t, repo.db)
	now := time.Now()
	olderReport := seedTestReport(t, repo.db, boardID, now.AddDate(0, 0, -1))
	newerReport := seedTestReport(t, repo.db, boardID, now)
	s1 := seedAnchorSection(t, repo.db, olderReport, "s1", []float64{1, 0, 0}, nil)
	s2 := seedAnchorSection(t, repo.db, newerReport, "s2", []float64{0, 1, 0}, nil)
	noVector := seedAnchorSection(t, repo.db, newerReport, "s3", nil, nil)

	topic, skipped, err := repo.CreateManualTopic(boardID, "manual lane", []uint{s1.ID, s2.ID, noVector.ID})
	require.NoError(t, err)
	require.NotNil(t, topic)
	assert.Equal(t, []uint{noVector.ID}, skipped, "a section without a vector is skipped")

	// Exactly one trail row, same 口径 as a manual re-anchor (same table, same
	// field semantics, no new source enum).
	var overrides []TopicAnchorOverride
	require.NoError(t, repo.db.Where("topic_id = ?", topic.ID).Order("id ASC").Find(&overrides).Error)
	require.Len(t, overrides, 1)
	assert.Equal(t, TopicAnchorSourceSelectedSections, overrides[0].Source)
	assert.Equal(t, AnchorOperatorUser, overrides[0].Operator)

	// The trail records the sections that actually built the 首义 vector.
	recorded := parseAnchorInputSummary(overrides[0].InputSummary)
	assert.ElementsMatch(t, []uint{s1.ID, s2.ID}, recorded)
	assert.NotContains(t, recorded, noVector.ID, "the skipped section took no part in the aggregation")

	// The read side reports the manual source and the node set.
	state, err := repo.GetTopicAnchorState(topic.ID)
	require.NoError(t, err)
	assert.Equal(t, TopicAnchorSourceManual, state.Source)
	assert.ElementsMatch(t, []uint{s1.ID, s2.ID}, state.SectionIDs)
	assert.Equal(t, 1, state.OverrideCount, "the creation trail counts as one 首义留痕")
	assert.True(t, state.AnchorOrigin.Resolved)
}

// TestManualTopicAnchorTrail_FailureRollsBackTheWholeTransaction covers
// spec「留痕写入失败整体回滚」: the trail write is part of the creation
// transaction, so a failure rolls the topic creation and the assignment
// rewrite back — no half-created topic, no "topic without a trail".
func TestManualTopicAnchorTrail_FailureRollsBackTheWholeTransaction(t *testing.T) {
	repo := setupManualTopicTestDB(t)
	boardID := seedTestBoard(t, repo.db)
	reportID := seedTestReport(t, repo.db, boardID, time.Now())
	origin := seedAnchorTopic(t, repo.db, boardID, "origin lane", TopicStatusActive, []float64{0, 0, 1})
	s1 := seedAnchorSection(t, repo.db, reportID, "s1", []float64{1, 0, 0}, &origin.ID)

	injectAnchorOverrideWriteFailure(t, repo.db)

	topic, _, err := repo.CreateManualTopic(boardID, "manual lane", []uint{s1.ID})
	require.Error(t, err, "the trail write failure must surface")
	assert.Nil(t, topic)

	// No half-created topic: only the original one is left.
	var topics []BoardPersistentTopic
	require.NoError(t, repo.db.Where("semantic_board_id = ?", boardID).Find(&topics).Error)
	require.Len(t, topics, 1)
	assert.Equal(t, origin.ID, topics[0].ID)

	// The assignment rewrite was rolled back too.
	var section DailyReportSection
	require.NoError(t, repo.db.First(&section, s1.ID).Error)
	require.NotNil(t, section.PersistentTopicID)
	assert.Equal(t, origin.ID, *section.PersistentTopicID)

	// And no trail row.
	var trailCount int64
	require.NoError(t, repo.db.Model(&TopicAnchorOverride{}).Count(&trailCount).Error)
	assert.Zero(t, trailCount)
}

// TestManualTopicAnchorTrail_LegacyManualTopicStillReadsAsAuto covers
// spec「历史手动泳道不回填」: a topic created before the trail existed (no
// override row) keeps reading as 「创建时写入 · 来源不可考」 — the read side
// never infers a node set after the fact.
func TestManualTopicAnchorTrail_LegacyManualTopicStillReadsAsAuto(t *testing.T) {
	repo := setupManualTopicTestDB(t)
	boardID := seedTestBoard(t, repo.db)
	reportID := seedTestReport(t, repo.db, boardID, time.Now())
	legacy := BoardPersistentTopic{
		SemanticBoardID: boardID,
		Label:           "legacy manual lane",
		Embedding:       FloatsToPgVector([]float64{0, 0, 1}),
		Status:          TopicStatusActive,
		Source:          TopicSourceManual,
		FirstSeenDate:   NormalizeReportDate(time.Now()),
		LastSeenDate:    NormalizeReportDate(time.Now()),
		HitCount:        1,
	}
	require.NoError(t, repo.db.Create(&legacy).Error)
	seedAnchorSection(t, repo.db, reportID, "s1", []float64{1, 0, 0}, &legacy.ID)

	state, err := repo.GetTopicAnchorState(legacy.ID)
	require.NoError(t, err)
	assert.Equal(t, TopicAnchorSourceAuto, state.Source, "a topic without a trail row reads as auto")
	assert.Empty(t, state.SectionIDs, "no trail row → no node set (never inferred)")
	assert.Zero(t, state.OverrideCount)
	assert.False(t, state.AnchorOrigin.Resolved)
	assert.Nil(t, state.AnchorOrigin.SectionID)
	assert.Nil(t, state.AnchorOrigin.Title)
	assert.Nil(t, state.AnchorOrigin.PeriodDate)
}

// injectAnchorOverrideWriteFailure makes every insert into
// topic_anchor_overrides fail inside this test, so the creation transaction's
// trail write can be exercised on its failure path. The callback is removed
// again when the test ends (the callback registry lives on this DB handle).
func injectAnchorOverrideWriteFailure(t *testing.T, db *gorm.DB) {
	t.Helper()
	const name = "test_inject_anchor_override_write_failure"
	require.NoError(t, db.Callback().Create().Before("gorm:create").Register(name, func(tx *gorm.DB) {
		if tx.Statement != nil && tx.Statement.Schema != nil && tx.Statement.Schema.Table == "topic_anchor_overrides" {
			tx.AddError(errors.New("injected anchor override write failure"))
		}
	}))
	t.Cleanup(func() {
		_ = db.Callback().Create().Remove(name)
	})
}
