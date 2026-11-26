package repository

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ── 清标记竞态（review R-11） ────────────────────────────────────────────
//
// centroid_pending_recompute 是「人工修正最终落地」的唯一保障（下次日报生成
// 按它纳入重算范围）。清标记必须是**条件更新**：只有「本次重算开始时读到的
// 留痕条数没有变化」时才清——重算期间落下的人工修正（追加一行留痕 + 重新
// 置位）不能被这一轮的清理抹掉。

// TestClearCentroidPendingFlag_KeepsFlagWhenCorrectionLandsDuringRecompute:
// 重算读版本 → 重算期间人工改首义（追加留痕 + 置位）→ 清标记（带版本条件）
// → 标记必须仍在。
func TestClearCentroidPendingFlag_KeepsFlagWhenCorrectionLandsDuringRecompute(t *testing.T) {
	repo := setupManualTopicTestDB(t)
	boardID := seedTestBoard(t, repo.db)
	topic := seedAnchorTopic(t, repo.db, boardID, "lane A", TopicStatusActive, []float64{0, 0, 1})
	require.NoError(t, repo.db.Model(&BoardPersistentTopic{}).Where("id = ?", topic.ID).
		Update("centroid_pending_recompute", true).Error)

	// 重算开始时读到的留痕版本。
	version, err := repo.correctionTrailVersion(nil, topic.ID)
	require.NoError(t, err)

	// 重算进行中：人工改首义 → 追加一行留痕 + 重新置位标记（同事务口径）。
	require.NoError(t, repo.db.Create(&TopicAnchorOverride{
		TopicID:      topic.ID,
		NewEmbedding: FloatsToPgVector([]float64{0, 0, 1}),
		Source:       TopicAnchorSourceSelectedSections,
		InputSummary: "[]",
		Operator:     AnchorOperatorUser,
	}).Error)
	require.NoError(t, repo.db.Model(&BoardPersistentTopic{}).Where("id = ?", topic.ID).
		Update("centroid_pending_recompute", true).Error)

	// 本轮清理：版本已变 → 不清标记。
	require.NoError(t, repo.clearCentroidPendingFlag(nil, topic.ID, version))

	var reloaded BoardPersistentTopic
	require.NoError(t, repo.db.First(&reloaded, topic.ID).Error)
	assert.True(t, reloaded.CentroidPendingRecompute, "重算期间落下的人工修正必须保住待重算标记")
}

// TestClearCentroidPendingFlag_ClearsFlagWhenNoCorrectionLanded: 没有新修正
// 时正常清标记（条件更新不能把正常路径变成「永远不清」）。
func TestClearCentroidPendingFlag_ClearsFlagWhenNoCorrectionLanded(t *testing.T) {
	repo := setupManualTopicTestDB(t)
	boardID := seedTestBoard(t, repo.db)
	topic := seedAnchorTopic(t, repo.db, boardID, "lane A", TopicStatusActive, []float64{0, 0, 1})
	require.NoError(t, repo.db.Model(&BoardPersistentTopic{}).Where("id = ?", topic.ID).
		Update("centroid_pending_recompute", true).Error)

	version, err := repo.correctionTrailVersion(nil, topic.ID)
	require.NoError(t, err)
	require.NoError(t, repo.clearCentroidPendingFlag(nil, topic.ID, version))

	var reloaded BoardPersistentTopic
	require.NoError(t, repo.db.First(&reloaded, topic.ID).Error)
	assert.False(t, reloaded.CentroidPendingRecompute, "无新修正时照常清标记")
}

// TestRefreshCentroidsIsolated_KeepsFlagWhenCorrectionLandsDuringRefresh 把
// 竞态钉在整轮刷新口径上：重算回调里追加一条留痕（模拟并发的人工修正），
// 本轮清理不得抹掉标记。
func TestRefreshCentroidsIsolated_KeepsFlagWhenCorrectionLandsDuringRefresh(t *testing.T) {
	repo := setupManualTopicTestDB(t)
	boardID := seedTestBoard(t, repo.db)
	topic := seedAnchorTopic(t, repo.db, boardID, "lane A", TopicStatusActive, []float64{0, 0, 1})
	require.NoError(t, repo.db.Model(&BoardPersistentTopic{}).Where("id = ?", topic.ID).
		Update("centroid_pending_recompute", true).Error)

	updated, failed := refreshCentroidsIsolated(
		[]uint{topic.ID},
		func(topicID uint) (int64, error) { return repo.correctionTrailVersion(nil, topicID) },
		func(topicID uint) error {
			// 重算进行中：并发的人工修正落库（追加留痕 + 置位）。
			require.NoError(t, repo.db.Create(&TopicAnchorOverride{
				TopicID:      topicID,
				NewEmbedding: FloatsToPgVector([]float64{0, 0, 1}),
				Source:       TopicAnchorSourceSelectedSections,
				InputSummary: "[]",
				Operator:     AnchorOperatorUser,
			}).Error)
			require.NoError(t, repo.db.Model(&BoardPersistentTopic{}).Where("id = ?", topicID).
				Update("centroid_pending_recompute", true).Error)
			return nil
		},
		func(topicID uint, version int64) error {
			return repo.clearCentroidPendingFlag(nil, topicID, version)
		},
	)

	assert.Equal(t, 1, updated)
	assert.Equal(t, 0, failed)

	var reloaded BoardPersistentTopic
	require.NoError(t, repo.db.First(&reloaded, topic.ID).Error)
	assert.True(t, reloaded.CentroidPendingRecompute, "并发的人工修正不能被本轮清理抹掉")
}
