package repository

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ── 人工修正留痕历史（review R-2） ───────────────────────────────────────
// PostgreSQL 集成测试（testcontainer 黄金 schema）。
//
// 留痕从「覆盖写最近一次」升级为 append-only：
//   - section_assignment_overrides —— 归属变更（移除 / 加回）各追加一行；
//   - topic_anchor_overrides      —— 首义改写留痕（早已 append-only）现在有读出口。

// TestSetSectionTopicAssignment_AppendsAssignmentHistoryOnDetachAndAttach
// 覆盖 spec「移除留痕可查」: 「移除 → 加回」两步各自留一行，前一次的 detach
// 痕迹不被后一次覆盖（section 上的留痕三列只保留最近一次，完整历史在追加表）。
func TestSetSectionTopicAssignment_AppendsAssignmentHistoryOnDetachAndAttach(t *testing.T) {
	repo := setupManualTopicTestDB(t)
	boardID := seedTestBoard(t, repo.db)
	reportID := seedAssignmentReport(t, repo.db, boardID, "2026-11-10")

	topic := seedAssignmentTopic(t, repo.db, boardID, "lane A", TopicStatusActive, []float64{1, 0, 0}, []float64{0, 0, 1})
	sec := seedAssignmentSection(t, repo.db, reportID, "s1", []float64{1, 0, 0}, &topic.ID)

	_, err := repo.SetSectionTopicAssignment(sec.ID, nil)
	require.NoError(t, err)
	_, err = repo.SetSectionTopicAssignment(sec.ID, &topic.ID)
	require.NoError(t, err)

	history, err := repo.ListSectionAssignmentHistory(sec.ID)
	require.NoError(t, err)
	require.Len(t, history, 2, "移除 → 加回各留一行，历史不被覆盖")

	// 时间升序：先 detach 后 attach。
	assert.Equal(t, ManualAssignmentActionDetach, history[0].Action)
	require.NotNil(t, history[0].TopicIDFrom)
	assert.Equal(t, topic.ID, *history[0].TopicIDFrom)
	assert.Nil(t, history[0].TopicIDTo)

	assert.Equal(t, ManualAssignmentActionAttach, history[1].Action)
	assert.Nil(t, history[1].TopicIDFrom, "加回时此前无归属")
	require.NotNil(t, history[1].TopicIDTo)
	assert.Equal(t, topic.ID, *history[1].TopicIDTo)

	assert.NotZero(t, history[0].CreatedAt)
	assert.Equal(t, AnchorOperatorUser, history[0].Operator)
	assert.False(t, history[1].CreatedAt.Before(history[0].CreatedAt), "留痕按时间升序返回")
}

// TestDetachTopicSectionsByDate_AppendsHistoryPerSection 覆盖 spec「批量移除
// 某一天」: 当天每个被释放的节点各留一行 detach。
func TestDetachTopicSectionsByDate_AppendsHistoryPerSection(t *testing.T) {
	repo := setupManualTopicTestDB(t)
	boardID := seedTestBoard(t, repo.db)
	reportID := seedAssignmentReport(t, repo.db, boardID, "2026-11-10")

	topic := seedAssignmentTopic(t, repo.db, boardID, "lane A", TopicStatusActive, []float64{1, 0, 0}, nil)
	s1 := seedAssignmentSection(t, repo.db, reportID, "s1", []float64{1, 0, 0}, &topic.ID)
	s2 := seedAssignmentSection(t, repo.db, reportID, "s2", []float64{0, 1, 0}, &topic.ID)

	result, err := repo.DetachTopicSectionsByDate(topic.ID, time.Date(2026, 11, 10, 0, 0, 0, 0, time.UTC))
	require.NoError(t, err)
	assert.Equal(t, 2, result.Detached)

	for _, sec := range []DailyReportSection{s1, s2} {
		history, err := repo.ListSectionAssignmentHistory(sec.ID)
		require.NoError(t, err)
		require.Len(t, history, 1)
		assert.Equal(t, ManualAssignmentActionDetach, history[0].Action)
		require.NotNil(t, history[0].TopicIDFrom)
		assert.Equal(t, topic.ID, *history[0].TopicIDFrom)
		assert.Nil(t, history[0].TopicIDTo)
	}
}

// TestListTopicAnchorOverrideHistory_ReturnsRowsOldestFirst 覆盖 spec「首义改写
// 历史可查」: 改写 2 次 → 历史返回 2 条、按时间升序、带改前/改后向量摘要、
// 选定节点 id 与操作者。
func TestListTopicAnchorOverrideHistory_ReturnsRowsOldestFirst(t *testing.T) {
	repo := setupManualTopicTestDB(t)
	boardID := seedTestBoard(t, repo.db)
	reportID := seedTestReport(t, repo.db, boardID, time.Now())

	topic := seedAnchorTopic(t, repo.db, boardID, "lane A", TopicStatusActive, []float64{0, 0, 1})
	s1 := seedAnchorSection(t, repo.db, reportID, "s1", []float64{1, 0, 0}, &topic.ID)
	s2 := seedAnchorSection(t, repo.db, reportID, "s2", []float64{0, 1, 0}, &topic.ID)

	_, err := repo.SetTopicAnchorFromSections(topic.ID, []uint{s1.ID}, AnchorOperatorUser)
	require.NoError(t, err)
	_, err = repo.SetTopicAnchorFromSections(topic.ID, []uint{s1.ID, s2.ID}, AnchorOperatorUser)
	require.NoError(t, err)

	history, err := repo.ListTopicAnchorOverrideHistory(topic.ID)
	require.NoError(t, err)
	require.Len(t, history, 2, "两次改写 → 两条留痕")

	assert.Equal(t, []uint{s1.ID}, history[0].SectionIDs)
	assert.Equal(t, []uint{s1.ID, s2.ID}, history[1].SectionIDs)
	assert.Equal(t, TopicAnchorSourceSelectedSections, history[0].Source)
	assert.Equal(t, AnchorOperatorUser, history[0].Operator)
	assert.False(t, history[1].CreatedAt.Before(history[0].CreatedAt), "留痕按时间升序返回")

	// 改后向量摘要：维度 + 前若干分量 + 模长（不外泄整条向量）。
	require.NotNil(t, history[0].NewVector)
	assert.Equal(t, 3, history[0].NewVector.Dimension)
	assert.Len(t, history[0].NewVector.Head, 3)
	// 改前向量摘要：第一次改写前话题已有首义 → 非 nil。
	require.NotNil(t, history[0].OldVector)
	assert.Equal(t, 3, history[0].OldVector.Dimension)
}

// TestListSectionAssignmentHistory_UnknownSectionIsNotFound: 没有留痕也没有节点
// → 404 语义的哨兵错误。
func TestListSectionAssignmentHistory_UnknownSectionIsNotFound(t *testing.T) {
	repo := setupManualTopicTestDB(t)

	_, err := repo.ListSectionAssignmentHistory(987654)
	assert.ErrorIs(t, err, ErrSectionNotFound)
}

// TestListTopicAnchorOverrideHistory_UnknownTopicIsNotFound: 同上（话题侧）。
func TestListTopicAnchorOverrideHistory_UnknownTopicIsNotFound(t *testing.T) {
	repo := setupManualTopicTestDB(t)

	_, err := repo.ListTopicAnchorOverrideHistory(987654)
	assert.ErrorIs(t, err, ErrTopicNotFound)
}
