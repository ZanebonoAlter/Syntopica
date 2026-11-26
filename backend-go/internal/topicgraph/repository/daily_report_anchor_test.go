package repository

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// ── 人工重设首义（lane-manual-correction D3 / tasks 2.1-2.3） ──────────────
// PostgreSQL 集成测试（testcontainer 黄金 schema）。repository 层禁 SQLite。

// assertVecEqual compares two pgvector literals element-wise. PostgreSQL
// normalizes the literal it stores ("[0,0,1]"), so a raw string comparison
// with the producer's format ("[0.000000,0.000000,1.000000]") never matches.
func assertVecEqual(t *testing.T, expected, actual string) {
	t.Helper()
	want, err := repoParsePgVector(expected)
	require.NoError(t, err)
	got, err := repoParsePgVector(actual)
	require.NoError(t, err)
	require.Len(t, got, len(want))
	for i := range want {
		assert.InDelta(t, want[i], got[i], 1e-6, "vector element %d differs", i)
	}
}

// seedAnchorTopic creates a topic with a valid 首义 vector (pgvector rejects
// the empty string, so the column must always carry a real literal).
func seedAnchorTopic(t *testing.T, db *gorm.DB, boardID uint, label string, status string, embedding []float64) BoardPersistentTopic {
	t.Helper()
	topic := BoardPersistentTopic{
		SemanticBoardID: boardID,
		Label:           label,
		Description:     "seed",
		Embedding:       FloatsToPgVector(embedding),
		Status:          status,
		Source:          TopicSourceAuto,
		FirstSeenDate:   NormalizeReportDate(time.Now()),
		LastSeenDate:    NormalizeReportDate(time.Now()),
		HitCount:        1,
		ConsecutiveHits: 1,
	}
	require.NoError(t, db.Create(&topic).Error)
	return topic
}

// seedAnchorSection creates a section with a given embedding (nil = no vector)
// and the given topic assignment.
func seedAnchorSection(t *testing.T, db *gorm.DB, reportID uint, label string, embedding []float64, topicID *uint) DailyReportSection {
	t.Helper()
	sec := DailyReportSection{
		ReportID:           reportID,
		ClusterLabel:       label,
		ArticleCount:       1,
		Embedding:          FloatsToPgVector(embedding),
		PersistentTopicID:  topicID,
		TopicMatchDistance: 0.1,
	}
	if embedding == nil {
		// pgvector rejects "" — a section without a vector is stored as NULL.
		sec.Embedding = ""
		require.NoError(t, db.Omit("embedding").Create(&sec).Error)
		return sec
	}
	require.NoError(t, db.Create(&sec).Error)
	return sec
}

// TestSetTopicAnchorFromSections_WritesEmbeddingAppendsOverrideSetsPending is
// the happy path (spec「选定节点范围重设首义」): one transaction writes the new
// 首义, appends one audit row and flags the topic for a forced recompute —
// without touching the centroid.
func TestSetTopicAnchorFromSections_WritesEmbeddingAppendsOverrideSetsPending(t *testing.T) {
	repo := setupManualTopicTestDB(t)
	boardID := seedTestBoard(t, repo.db)
	reportID := seedTestReport(t, repo.db, boardID, time.Now())

	topic := seedAnchorTopic(t, repo.db, boardID, "lane A", TopicStatusActive, []float64{0, 0, 1})
	s1 := seedAnchorSection(t, repo.db, reportID, "s1", []float64{1, 0, 0}, &topic.ID)
	s2 := seedAnchorSection(t, repo.db, reportID, "s2", []float64{0, 1, 0}, &topic.ID)

	// Pre-existing centroid — the rewrite must NOT touch it (D1: the centroid
	// belongs to the next daily-report refresh round).
	require.NoError(t, repo.db.Model(&BoardPersistentTopic{}).Where("id = ?", topic.ID).
		Update("centroid", FloatsToPgVector([]float64{0, 0, 1})).Error)

	result, err := repo.SetTopicAnchorFromSections(topic.ID, []uint{s1.ID, s2.ID}, AnchorOperatorUser)
	require.NoError(t, err)

	assert.Equal(t, topic.ID, result.TopicID)
	assert.Equal(t, TopicAnchorSourceSelectedSections, result.Source)
	assert.Equal(t, []uint{s1.ID, s2.ID}, result.SectionIDs)
	assert.Equal(t, 1, result.OverrideCount)
	assert.Equal(t, AnchorEffectiveFromNextReport, result.EffectiveFrom)

	var reloaded BoardPersistentTopic
	require.NoError(t, repo.db.First(&reloaded, topic.ID).Error)
	// New 首义 = mean pooling of the two chosen nodes.
	expected := FloatsToPgVector([]float64{0.5, 0.5, 0})
	assertVecEqual(t, expected, reloaded.Embedding)
	// Pending flag set.
	assert.True(t, reloaded.CentroidPendingRecompute)
	// Centroid untouched.
	assertVecEqual(t, FloatsToPgVector([]float64{0, 0, 1}), reloaded.Centroid)

	// Audit trail: exactly one appended row carrying before/after + input.
	var overrides []TopicAnchorOverride
	require.NoError(t, repo.db.Where("topic_id = ?", topic.ID).Order("id ASC").Find(&overrides).Error)
	require.Len(t, overrides, 1)
	require.NotNil(t, overrides[0].OldEmbedding)
	assertVecEqual(t, FloatsToPgVector([]float64{0, 0, 1}), *overrides[0].OldEmbedding)
	assertVecEqual(t, expected, overrides[0].NewEmbedding)
	assert.Equal(t, TopicAnchorSourceSelectedSections, overrides[0].Source)
	assert.Equal(t, fmt.Sprintf("[%d,%d]", s1.ID, s2.ID), overrides[0].InputSummary)
	assert.Equal(t, AnchorOperatorUser, overrides[0].Operator)
	assert.False(t, overrides[0].CreatedAt.IsZero())
}

// TestSetTopicAnchorFromSections_AggregationMatchesManualTopicMean pins the
// aggregation口径: the new 首义 is the very same mean pooling the manual-topic
// creation path uses (task 2.2).
func TestSetTopicAnchorFromSections_AggregationMatchesManualTopicMean(t *testing.T) {
	repo := setupManualTopicTestDB(t)
	boardID := seedTestBoard(t, repo.db)
	reportID := seedTestReport(t, repo.db, boardID, time.Now())

	topic := seedAnchorTopic(t, repo.db, boardID, "lane A", TopicStatusActive, []float64{0, 0, 1})
	s1 := seedAnchorSection(t, repo.db, reportID, "s1", []float64{0.1, 0.2, 0.3}, &topic.ID)
	s2 := seedAnchorSection(t, repo.db, reportID, "s2", []float64{0.4, 0.5, 0.6}, &topic.ID)

	_, err := repo.SetTopicAnchorFromSections(topic.ID, []uint{s1.ID, s2.ID}, AnchorOperatorUser)
	require.NoError(t, err)

	manualMean, skipped := aggregateEmbeddings([][]float64{
		{0.1, 0.2, 0.3},
		{0.4, 0.5, 0.6},
	})
	require.Zero(t, skipped)

	var reloaded BoardPersistentTopic
	require.NoError(t, repo.db.First(&reloaded, topic.ID).Error)
	assertVecEqual(t, FloatsToPgVector(manualMean), reloaded.Embedding)
}

// TestSetTopicAnchorFromSections_SingleNodeDegeneratesToThatVector covers the
// boundary value「首义集 1（退化均值）」.
func TestSetTopicAnchorFromSections_SingleNodeDegeneratesToThatVector(t *testing.T) {
	repo := setupManualTopicTestDB(t)
	boardID := seedTestBoard(t, repo.db)
	reportID := seedTestReport(t, repo.db, boardID, time.Now())

	topic := seedAnchorTopic(t, repo.db, boardID, "lane A", TopicStatusActive, []float64{0, 0, 1})
	s1 := seedAnchorSection(t, repo.db, reportID, "s1", []float64{0.2, 0.4, 0.6}, &topic.ID)

	_, err := repo.SetTopicAnchorFromSections(topic.ID, []uint{s1.ID}, AnchorOperatorUser)
	require.NoError(t, err)

	var reloaded BoardPersistentTopic
	require.NoError(t, repo.db.First(&reloaded, topic.ID).Error)
	assertVecEqual(t, FloatsToPgVector([]float64{0.2, 0.4, 0.6}), reloaded.Embedding)
}

// TestSetTopicAnchorFromSections_DeduplicatesSelection covers the variant
// 「section_ids 重复 id → 去重 200；selected_sections 去重」.
func TestSetTopicAnchorFromSections_DeduplicatesSelection(t *testing.T) {
	repo := setupManualTopicTestDB(t)
	boardID := seedTestBoard(t, repo.db)
	reportID := seedTestReport(t, repo.db, boardID, time.Now())

	topic := seedAnchorTopic(t, repo.db, boardID, "lane A", TopicStatusActive, []float64{0, 0, 1})
	s1 := seedAnchorSection(t, repo.db, reportID, "s1", []float64{1, 0, 0}, &topic.ID)

	result, err := repo.SetTopicAnchorFromSections(topic.ID, []uint{s1.ID, s1.ID}, AnchorOperatorUser)
	require.NoError(t, err)
	assert.Equal(t, []uint{s1.ID}, result.SectionIDs)

	var count int64
	require.NoError(t, repo.db.Model(&TopicAnchorOverride{}).Where("topic_id = ?", topic.ID).Count(&count).Error)
	assert.Equal(t, int64(1), count)
}

// TestSetTopicAnchorFromSections_RejectsInvalidSelection covers task 2.3: a
// chosen node that breaks any rule rejects the WHOLE write (no partial
// result, no silent skip).
func TestSetTopicAnchorFromSections_RejectsInvalidSelection(t *testing.T) {
	repo := setupManualTopicTestDB(t)
	boardID := seedTestBoard(t, repo.db)
	reportID := seedTestReport(t, repo.db, boardID, time.Now())
	otherBoardID := seedTestBoardNamed(t, repo.db, "anchor-other-board")
	otherReportID := seedTestReport(t, repo.db, otherBoardID, time.Now().AddDate(0, 0, 1))

	topic := seedAnchorTopic(t, repo.db, boardID, "lane A", TopicStatusActive, []float64{0, 0, 1})
	otherTopic := seedAnchorTopic(t, repo.db, otherBoardID, "lane B", TopicStatusActive, []float64{0, 0, 1})

	good := seedAnchorSection(t, repo.db, reportID, "good", []float64{1, 0, 0}, &topic.ID)
	crossTopic := seedAnchorSection(t, repo.db, reportID, "cross-topic", []float64{1, 0, 0}, &otherTopic.ID)
	crossBoard := seedAnchorSection(t, repo.db, otherReportID, "cross-board", []float64{1, 0, 0}, &otherTopic.ID)
	noVector := seedAnchorSection(t, repo.db, reportID, "no-vector", nil, &topic.ID)
	dimMismatch := seedAnchorSection(t, repo.db, reportID, "dim", []float64{1, 0}, &topic.ID)

	cases := []struct {
		name       string
		sectionIDs []uint
		reason     string
	}{
		{"cross topic", []uint{good.ID, crossTopic.ID}, "not_in_topic"},
		{"cross board", []uint{good.ID, crossBoard.ID}, "other_board"},
		{"no vector", []uint{good.ID, noVector.ID}, "no_embedding"},
		{"dimension mismatch", []uint{good.ID, dimMismatch.ID}, "dim_mismatch"},
		{"unknown section", []uint{good.ID, 999999}, "section_not_found"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := repo.SetTopicAnchorFromSections(topic.ID, tc.sectionIDs, AnchorOperatorUser)
			require.Error(t, err)

			var selErr *AnchorSelectionError
			require.ErrorAs(t, err, &selErr)
			assert.Equal(t, tc.reason, selErr.Reason)

			// Nothing was written: 首义 unchanged, no audit row, no flag.
			var reloaded BoardPersistentTopic
			require.NoError(t, repo.db.First(&reloaded, topic.ID).Error)
			assertVecEqual(t, FloatsToPgVector([]float64{0, 0, 1}), reloaded.Embedding)
			assert.False(t, reloaded.CentroidPendingRecompute)

			var count int64
			require.NoError(t, repo.db.Model(&TopicAnchorOverride{}).Where("topic_id = ?", topic.ID).Count(&count).Error)
			assert.Zero(t, count)
		})
	}
}

// TestSetTopicAnchorFromSections_RejectsUnknownAndArchivedTopic covers the
// precondition variant「anchor 目标 topic 不存在/已归档 → 404/409 无写入」.
func TestSetTopicAnchorFromSections_RejectsUnknownAndArchivedTopic(t *testing.T) {
	repo := setupManualTopicTestDB(t)
	boardID := seedTestBoard(t, repo.db)
	reportID := seedTestReport(t, repo.db, boardID, time.Now())

	archived := seedAnchorTopic(t, repo.db, boardID, "archived lane", TopicStatusArchived, []float64{0, 0, 1})
	sec := seedAnchorSection(t, repo.db, reportID, "s1", []float64{1, 0, 0}, &archived.ID)

	_, err := repo.SetTopicAnchorFromSections(999999, []uint{sec.ID}, AnchorOperatorUser)
	assert.ErrorIs(t, err, ErrTopicNotFound)

	_, err = repo.SetTopicAnchorFromSections(archived.ID, []uint{sec.ID}, AnchorOperatorUser)
	assert.ErrorIs(t, err, ErrTopicArchived)

	var count int64
	require.NoError(t, repo.db.Model(&TopicAnchorOverride{}).Where("topic_id = ?", archived.ID).Count(&count).Error)
	assert.Zero(t, count)
}

// TestUpdateCentroidOnSectionChange_DoesNotTouchEmbedding is the regression
// guard for task 2.6: the algorithm-side refresh writes the centroid column
// only — the 首义 column (embedding) stays byte-identical.
func TestUpdateCentroidOnSectionChange_DoesNotTouchEmbedding(t *testing.T) {
	repo := setupManualTopicTestDB(t)
	boardID := seedTestBoard(t, repo.db)
	reportID := seedTestReport(t, repo.db, boardID, time.Now())

	topic := seedAnchorTopic(t, repo.db, boardID, "lane A", TopicStatusActive, []float64{0, 0, 1})
	seedAnchorSection(t, repo.db, reportID, "s1", []float64{1, 0, 0}, &topic.ID)
	seedAnchorSection(t, repo.db, reportID, "s2", []float64{0, 1, 0}, &topic.ID)

	before := topic.Embedding
	require.NoError(t, repo.UpdateCentroidOnSectionChange(nil, topic.ID))

	var reloaded BoardPersistentTopic
	require.NoError(t, repo.db.First(&reloaded, topic.ID).Error)
	assertVecEqual(t, before, reloaded.Embedding)
	assert.NotEmpty(t, reloaded.Centroid, "质心应被刷新")
}

// TestRefreshBoardCentroids_IncludesPendingTopicWithoutTouch covers task 2.5 /
// spec「无新内容命中也仍会重算」: a topic flagged for a forced recompute joins
// the refresh round even when the day's report never touched it.
func TestRefreshBoardCentroids_IncludesPendingTopicWithoutTouch(t *testing.T) {
	repo := setupManualTopicTestDB(t)
	boardID := seedTestBoard(t, repo.db)
	reportID := seedTestReport(t, repo.db, boardID, time.Now())

	touched := seedAnchorTopic(t, repo.db, boardID, "touched lane", TopicStatusActive, []float64{0, 0, 1})
	pending := seedAnchorTopic(t, repo.db, boardID, "pending lane", TopicStatusActive, []float64{0, 0, 1})
	seedAnchorSection(t, repo.db, reportID, "s1", []float64{1, 0, 0}, &touched.ID)
	seedAnchorSection(t, repo.db, reportID, "s2", []float64{0, 1, 0}, &touched.ID)
	seedAnchorSection(t, repo.db, reportID, "s3", []float64{1, 1, 0}, &pending.ID)
	seedAnchorSection(t, repo.db, reportID, "s4", []float64{0, 1, 1}, &pending.ID)

	require.NoError(t, repo.db.Model(&BoardPersistentTopic{}).Where("id = ?", pending.ID).
		Update("centroid_pending_recompute", true).Error)

	updated, failed := repo.RefreshBoardCentroids(boardID, []uint{touched.ID})
	require.Zero(t, failed)
	assert.Equal(t, 2, updated, "touched ∪ pending 两个话题都要重算")

	var reloadedTouched, reloadedPending BoardPersistentTopic
	require.NoError(t, repo.db.First(&reloadedTouched, touched.ID).Error)
	require.NoError(t, repo.db.First(&reloadedPending, pending.ID).Error)
	assert.NotEmpty(t, reloadedTouched.Centroid)
	assert.NotEmpty(t, reloadedPending.Centroid, "无新命中的话题也必须被重算")
	assert.False(t, reloadedPending.CentroidPendingRecompute, "重算成功后清标记")
}

// TestListPendingCentroidTopicIDs_IsBoardScoped keeps the pending set inside
// the board that owns the topics.
func TestListPendingCentroidTopicIDs_IsBoardScoped(t *testing.T) {
	repo := setupManualTopicTestDB(t)
	boardA := seedTestBoard(t, repo.db)
	boardB := seedTestBoardNamed(t, repo.db, "pending-board-b")

	pendingA := seedAnchorTopic(t, repo.db, boardA, "a pending", TopicStatusActive, []float64{0, 0, 1})
	plainA := seedAnchorTopic(t, repo.db, boardA, "a plain", TopicStatusActive, []float64{0, 0, 1})
	pendingB := seedAnchorTopic(t, repo.db, boardB, "b pending", TopicStatusActive, []float64{0, 0, 1})

	require.NoError(t, repo.db.Model(&BoardPersistentTopic{}).Where("id IN ?", []uint{pendingA.ID, pendingB.ID}).
		Update("centroid_pending_recompute", true).Error)

	ids, err := repo.ListPendingCentroidTopicIDs(boardA)
	require.NoError(t, err)
	assert.Equal(t, []uint{pendingA.ID}, ids)
	assert.NotContains(t, ids, plainA.ID)
	assert.NotContains(t, ids, pendingB.ID)
}

// TestRefreshCentroidsIsolated_FailedTopicKeepsPendingFlag covers spec
// 「重算失败保留待重算标记」+「不中断其余话题的刷新」: the failing topic keeps
// its flag (the next round retries it) while the remaining topics are still
// refreshed and cleared.
func TestRefreshCentroidsIsolated_FailedTopicKeepsPendingFlag(t *testing.T) {
	repo := setupManualTopicTestDB(t)
	boardID := seedTestBoard(t, repo.db)

	failing := seedAnchorTopic(t, repo.db, boardID, "failing lane", TopicStatusActive, []float64{0, 0, 1})
	healthy := seedAnchorTopic(t, repo.db, boardID, "healthy lane", TopicStatusActive, []float64{0, 0, 1})
	require.NoError(t, repo.db.Model(&BoardPersistentTopic{}).Where("id IN ?", []uint{failing.ID, healthy.ID}).
		Update("centroid_pending_recompute", true).Error)

	refresh := func(topicID uint) error {
		if topicID == failing.ID {
			return assert.AnError
		}
		return nil
	}

	updated, failed := refreshCentroidsIsolated([]uint{failing.ID, healthy.ID},
		func(topicID uint) (int64, error) { return repo.correctionTrailVersion(nil, topicID) },
		refresh,
		func(topicID uint, version int64) error {
			return repo.clearCentroidPendingFlag(nil, topicID, version)
		})
	assert.Equal(t, 1, updated)
	assert.Equal(t, 1, failed)

	var reloadedFailing, reloadedHealthy BoardPersistentTopic
	require.NoError(t, repo.db.First(&reloadedFailing, failing.ID).Error)
	require.NoError(t, repo.db.First(&reloadedHealthy, healthy.ID).Error)
	assert.True(t, reloadedFailing.CentroidPendingRecompute, "重算失败的对话题保留标记")
	assert.False(t, reloadedHealthy.CentroidPendingRecompute, "其余话题照常刷新并清标记")
}

// TestSetTopicAnchorFromSections_AppendsHistoryOnRepeatedRewrite covers the
// idempotency variant「同一 anchor 重复提交 → override 追加两行（历史保留）、
// 最新生效；次数 +2」.
func TestSetTopicAnchorFromSections_AppendsHistoryOnRepeatedRewrite(t *testing.T) {
	repo := setupManualTopicTestDB(t)
	boardID := seedTestBoard(t, repo.db)
	reportID := seedTestReport(t, repo.db, boardID, time.Now())

	topic := seedAnchorTopic(t, repo.db, boardID, "lane A", TopicStatusActive, []float64{0, 0, 1})
	s1 := seedAnchorSection(t, repo.db, reportID, "s1", []float64{1, 0, 0}, &topic.ID)

	first, err := repo.SetTopicAnchorFromSections(topic.ID, []uint{s1.ID}, AnchorOperatorUser)
	require.NoError(t, err)
	assert.Equal(t, 1, first.OverrideCount)

	second, err := repo.SetTopicAnchorFromSections(topic.ID, []uint{s1.ID}, AnchorOperatorUser)
	require.NoError(t, err)
	assert.Equal(t, 2, second.OverrideCount)

	var overrides []TopicAnchorOverride
	require.NoError(t, repo.db.Where("topic_id = ?", topic.ID).Order("id ASC").Find(&overrides).Error)
	require.Len(t, overrides, 2, "留痕只追加，不覆盖历史行")
}
