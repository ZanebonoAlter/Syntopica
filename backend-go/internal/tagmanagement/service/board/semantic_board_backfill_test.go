package board

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/testutil"
	"syntopica-backend/internal/tagmanagement/service/core"
)

func TestSemanticBoardBackfillAllModeRewritesActiveTags(t *testing.T) {
	db := setupSemanticBoardMatchingTestDB(t)
	board := createMatchLabel(t, db, "AI Board", "ai-board", "board", "active", nil)
	auxiliary := createMatchLabel(t, db, "OpenAI", "openai", "auxiliary", "active", []float64{1, 0, 0})
	replacedBoard := createMatchLabel(t, db, "Old Board", "old-board", "board", "active", nil)
	tagA := createMatchTag(t, db, "tag-a")
	tagB := createMatchTag(t, db, "tag-b")
	inactive := createMatchTag(t, db, "inactive")
	require.NoError(t, db.Model(&models.TopicTag{}).Where("id = ?", inactive.ID).Update("status", "merged").Error)
	require.NoError(t, db.Create(&models.BoardComposition{BoardID: board.ID, AuxiliaryLabelID: auxiliary.ID}).Error)
	require.NoError(t, db.Create(&models.TopicTagSemanticLabel{TopicTagID: tagA.ID, SemanticLabelID: auxiliary.ID}).Error)
	require.NoError(t, db.Create(&models.TopicTagSemanticLabel{TopicTagID: tagB.ID, SemanticLabelID: auxiliary.ID}).Error)
	require.NoError(t, db.Create(&models.TopicTagSemanticLabel{TopicTagID: inactive.ID, SemanticLabelID: auxiliary.ID}).Error)
	require.NoError(t, db.Create(&models.TopicTagBoardLabel{TopicTagID: tagA.ID, SemanticBoardID: replacedBoard.ID, Score: 0.2, MatchReason: "stale"}).Error)
	require.NoError(t, db.Create(&models.TopicTagBoardLabel{TopicTagID: inactive.ID, SemanticBoardID: replacedBoard.ID, Score: 0.2, MatchReason: "stale"}).Error)
	upsertMatchSetting(t, db, "semantic_board_match_direct_hit_min_overlap", "1")
	service := NewSemanticBoardBackfillService(db)

	job, err := service.Enqueue(context.Background(), SemanticBoardBackfillRequest{Mode: SemanticBoardBackfillModeAll})

	require.NoError(t, err)
	job = waitForSemanticBoardBackfillJob(t, service, job.ID)
	require.Equal(t, SemanticBoardBackfillStatusCompleted, job.Status)
	require.Equal(t, 2, job.Total)
	require.Equal(t, 2, job.Processed)
	require.Zero(t, job.Failed)
	requireTopicTagBoardIDs(t, db, tagA.ID, []uint{board.ID})
	requireTopicTagBoardIDs(t, db, tagB.ID, []uint{board.ID})
	requireTopicTagBoardIDs(t, db, inactive.ID, []uint{replacedBoard.ID})
}

func TestSemanticBoardBackfillUnassignedModeSkipsAssignedTags(t *testing.T) {
	db := setupSemanticBoardMatchingTestDB(t)
	board := createMatchLabel(t, db, "AI Board", "ai-board", "board", "active", nil)
	auxiliary := createMatchLabel(t, db, "OpenAI", "openai", "auxiliary", "active", []float64{1, 0, 0})
	assigned := createMatchTag(t, db, "assigned")
	unassigned := createMatchTag(t, db, "unassigned")
	require.NoError(t, db.Create(&models.BoardComposition{BoardID: board.ID, AuxiliaryLabelID: auxiliary.ID}).Error)
	require.NoError(t, db.Create(&models.TopicTagSemanticLabel{TopicTagID: assigned.ID, SemanticLabelID: auxiliary.ID}).Error)
	require.NoError(t, db.Create(&models.TopicTagSemanticLabel{TopicTagID: unassigned.ID, SemanticLabelID: auxiliary.ID}).Error)
	require.NoError(t, db.Create(&models.TopicTagBoardLabel{TopicTagID: assigned.ID, SemanticBoardID: board.ID, Score: 0.4, MatchReason: "existing"}).Error)
	upsertMatchSetting(t, db, "semantic_board_match_direct_hit_min_overlap", "1")
	service := NewSemanticBoardBackfillService(db)

	job, err := service.Enqueue(context.Background(), SemanticBoardBackfillRequest{Mode: SemanticBoardBackfillModeUnassigned})

	require.NoError(t, err)
	job = waitForSemanticBoardBackfillJob(t, service, job.ID)
	require.Equal(t, SemanticBoardBackfillStatusCompleted, job.Status)
	require.Equal(t, 1, job.Total)
	requireTopicTagBoardIDs(t, db, assigned.ID, []uint{board.ID})
	requireTopicTagBoardIDs(t, db, unassigned.ID, []uint{board.ID})
}

func TestSemanticBoardBackfillBoardModeReprocessesAffectedTags(t *testing.T) {
	db := setupSemanticBoardMatchingTestDB(t)
	targetBoard := createMatchLabel(t, db, "AI Board", "ai-board", "board", "active", nil)
	otherBoard := createMatchLabel(t, db, "Other Board", "other-board", "board", "active", nil)
	targetAuxiliary := createMatchLabel(t, db, "OpenAI", "openai", "auxiliary", "active", []float64{1, 0, 0})
	similarAuxiliary := createMatchLabel(t, db, "LLM", "llm", "auxiliary", "active", []float64{0.9, 0.435889894354067, 0})
	unrelatedAuxiliary := createMatchLabel(t, db, "Energy", "energy", "auxiliary", "active", []float64{0, 1, 0})
	disabledAuxiliary := createMatchLabel(t, db, "Disabled", "disabled", "auxiliary", "disabled", []float64{1, 0, 0})
	existing := createMatchTag(t, db, "existing-target")
	candidate := createMatchTag(t, db, "candidate-target")
	indirectCandidate := createMatchTag(t, db, "indirect-target")
	disabledOnly := createMatchTag(t, db, "disabled-only")
	unaffected := createMatchTag(t, db, "unaffected")
	require.NoError(t, db.Create(&models.BoardComposition{BoardID: targetBoard.ID, AuxiliaryLabelID: targetAuxiliary.ID}).Error)
	require.NoError(t, db.Create(&models.BoardComposition{BoardID: targetBoard.ID, AuxiliaryLabelID: disabledAuxiliary.ID}).Error)
	require.NoError(t, db.Create(&models.TopicTagSemanticLabel{TopicTagID: existing.ID, SemanticLabelID: unrelatedAuxiliary.ID}).Error)
	require.NoError(t, db.Create(&models.TopicTagSemanticLabel{TopicTagID: candidate.ID, SemanticLabelID: targetAuxiliary.ID}).Error)
	require.NoError(t, db.Create(&models.TopicTagSemanticLabel{TopicTagID: indirectCandidate.ID, SemanticLabelID: similarAuxiliary.ID}).Error)
	require.NoError(t, db.Create(&models.TopicTagSemanticLabel{TopicTagID: disabledOnly.ID, SemanticLabelID: disabledAuxiliary.ID}).Error)
	require.NoError(t, db.Create(&models.TopicTagSemanticLabel{TopicTagID: unaffected.ID, SemanticLabelID: unrelatedAuxiliary.ID}).Error)
	require.NoError(t, db.Create(&models.TopicTagBoardLabel{TopicTagID: existing.ID, SemanticBoardID: targetBoard.ID, Score: 0.4, MatchReason: "stale"}).Error)
	require.NoError(t, db.Create(&models.TopicTagBoardLabel{TopicTagID: disabledOnly.ID, SemanticBoardID: otherBoard.ID, Score: 0.4, MatchReason: "existing"}).Error)
	require.NoError(t, db.Create(&models.TopicTagBoardLabel{TopicTagID: unaffected.ID, SemanticBoardID: otherBoard.ID, Score: 0.4, MatchReason: "existing"}).Error)
	upsertMatchSetting(t, db, "semantic_board_match_direct_hit_min_overlap", "1")
	service := NewSemanticBoardBackfillService(db)

	job, err := service.Enqueue(context.Background(), SemanticBoardBackfillRequest{Mode: SemanticBoardBackfillModeBoard, BoardID: &targetBoard.ID})

	require.NoError(t, err)
	job = waitForSemanticBoardBackfillJob(t, service, job.ID)
	require.Equal(t, SemanticBoardBackfillStatusCompleted, job.Status)
	require.Equal(t, 3, job.Total)
	requireTopicTagBoardIDs(t, db, existing.ID, []uint{})
	requireTopicTagBoardIDs(t, db, candidate.ID, []uint{targetBoard.ID})
	requireTopicTagBoardIDs(t, db, indirectCandidate.ID, []uint{targetBoard.ID})
	requireTopicTagBoardIDs(t, db, disabledOnly.ID, []uint{otherBoard.ID})
	requireTopicTagBoardIDs(t, db, unaffected.ID, []uint{otherBoard.ID})
}

func TestSemanticBoardBackfillIsIdempotent(t *testing.T) {
	db := setupSemanticBoardMatchingTestDB(t)
	board := createMatchLabel(t, db, "AI Board", "ai-board", "board", "active", nil)
	auxiliary := createMatchLabel(t, db, "OpenAI", "openai", "auxiliary", "active", []float64{1, 0, 0})
	tag := createMatchTag(t, db, "idempotent")
	require.NoError(t, db.Create(&models.BoardComposition{BoardID: board.ID, AuxiliaryLabelID: auxiliary.ID}).Error)
	require.NoError(t, db.Create(&models.TopicTagSemanticLabel{TopicTagID: tag.ID, SemanticLabelID: auxiliary.ID}).Error)
	upsertMatchSetting(t, db, "semantic_board_match_direct_hit_min_overlap", "1")
	service := NewSemanticBoardBackfillService(db)

	first, err := service.Enqueue(context.Background(), SemanticBoardBackfillRequest{Mode: SemanticBoardBackfillModeAll})
	require.NoError(t, err)
	waitForSemanticBoardBackfillJob(t, service, first.ID)
	second, err := service.Enqueue(context.Background(), SemanticBoardBackfillRequest{Mode: SemanticBoardBackfillModeAll})
	require.NoError(t, err)
	waitForSemanticBoardBackfillJob(t, service, second.ID)

	var rows []models.TopicTagBoardLabel
	require.NoError(t, db.Where("topic_tag_id = ?", tag.ID).Find(&rows).Error)
	require.Len(t, rows, 1)
	require.Equal(t, board.ID, rows[0].SemanticBoardID)
}

func TestSemanticBoardBackfillRecordsFailures(t *testing.T) {
	db := setupSemanticBoardMatchingTestDB(t)
	tag := createMatchTag(t, db, "failing")
	service := NewSemanticBoardBackfillService(db)
	service.matcher = failingSemanticBoardMatcher{err: errors.New("match failed")}

	job, err := service.Enqueue(context.Background(), SemanticBoardBackfillRequest{Mode: SemanticBoardBackfillModeAll})

	require.NoError(t, err)
	job = waitForSemanticBoardBackfillJob(t, service, job.ID)
	require.Equal(t, SemanticBoardBackfillStatusFailed, job.Status)
	require.Equal(t, 1, job.Total)
	require.Equal(t, 1, job.Processed)
	require.Equal(t, 1, job.Failed)
	require.Len(t, job.Failures, 1)
	require.Equal(t, tag.ID, job.Failures[0].TopicTagID)
	require.Contains(t, job.Failures[0].Error, "match failed")
}

type failingSemanticBoardMatcher struct {
	err error
}

func (m failingSemanticBoardMatcher) MatchTopicTag(context.Context, uint) ([]SemanticBoardMatchResult, error) {
	return nil, m.err
}

func waitForSemanticBoardBackfillJob(t *testing.T, service *SemanticBoardBackfillService, jobID string) *SemanticBoardBackfillJob {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		job, ok := service.GetJob(jobID)
		require.True(t, ok)
		if job.Status == SemanticBoardBackfillStatusCompleted || job.Status == SemanticBoardBackfillStatusFailed {
			return job
		}
		time.Sleep(10 * time.Millisecond)
	}
	job, ok := service.GetJob(jobID)
	require.True(t, ok)
	require.FailNowf(t, "semantic board backfill job did not finish", "job_id=%s status=%s processed=%d total=%d", job.ID, job.Status, job.Processed, job.Total)
	return job
}

func requireTopicTagBoardIDs(t *testing.T, db *gorm.DB, topicTagID uint, expected []uint) {
	t.Helper()
	var rows []models.TopicTagBoardLabel
	require.NoError(t, db.Where("topic_tag_id = ?", topicTagID).Order("semantic_board_id ASC").Find(&rows).Error)
	require.Len(t, rows, len(expected))
	actual := make([]uint, 0, len(rows))
	for _, row := range rows {
		actual = append(actual, row.SemanticBoardID)
	}
	require.Equal(t, expected, actual)
}

// upsertMatchSetting 幂等覆盖某条 semantic_board_match_* 配置：迁移已 seed 这些 key，
// 且 ResetTestData 每个测试前会重建 seed 行，直接 db.Create 会撞 uni_ai_settings_key
// 唯一约束。用 FirstOrCreate+Assign 做幂等 upsert（存在则改 value，不存在则建）。
func upsertMatchSetting(t *testing.T, db *gorm.DB, key, value string) {
	t.Helper()
	require.NoError(t, db.Where("key = ?", key).
		Assign(models.AISettings{Value: value}).
		FirstOrCreate(&models.AISettings{Key: key}).Error)
}

// TestSemanticBoardBackfillAllModeAppliesNewRules（add-composite-labels S7）：
// 存量 direct_hit 记录（旧语义 score=1.0）经 mode="all" 重算后按新契约改写——
// score=direct_hit_score_factor(0.7)、方向不符记录 direction_mismatch=true；
// 组合命中候选重算后变 composite_hit score=1.0；连续重算幂等。
func TestSemanticBoardBackfillAllModeAppliesNewRules(t *testing.T) {
	db := setupSemanticBoardMatchingTestDB(t)

	// -- direct_hit 降级场景：tagX 与 boardX 单标签重叠（min_overlap=2），方向不符 --
	tagX := createMatchTag(t, db, "backfill-direct")
	auxA := createMatchLabel(t, db, "BackfillA", "bf-a", "auxiliary", "active", []float64{1, 0, 0})
	auxB := createMatchLabel(t, db, "BackfillB", "bf-b", "auxiliary", "active", []float64{0, 1, 0})
	boardX := createMatchLabel(t, db, "Backfill Board X", "bf-board-x", "board", "active", []float64{0, 0, 1})
	require.NoError(t, db.Create(&models.TopicTagSemanticLabel{TopicTagID: tagX.ID, SemanticLabelID: auxA.ID}).Error)
	require.NoError(t, db.Create(&models.TopicTagSemanticLabel{TopicTagID: tagX.ID, SemanticLabelID: auxB.ID}).Error)
	require.NoError(t, db.Create(&models.BoardComposition{BoardID: boardX.ID, AuxiliaryLabelID: auxA.ID}).Error)
	require.NoError(t, db.Create(&models.BoardComposition{BoardID: boardX.ID, AuxiliaryLabelID: auxB.ID}).Error)
	// tag identity embedding [1,0,0] vs board embedding [0,0,1] → cosine 0 < 0.5
	pgVector := core.FloatsToPgVector(testutil.PadVector([]float64{1, 0, 0}, testutil.TestEmbeddingDim))
	require.NoError(t, db.Create(&models.TopicTagEmbedding{TopicTagID: tagX.ID, EmbeddingType: "identity", EmbeddingVec: pgVector, Dimension: testutil.TestEmbeddingDim, Model: "test", TextHash: fmt.Sprintf("hash-%d", tagX.ID)}).Error)
	// 存量旧记录：旧语义 direct_hit score=1.0、无 direction_mismatch
	require.NoError(t, db.Create(&models.TopicTagBoardLabel{TopicTagID: tagX.ID, SemanticBoardID: boardX.ID, Score: 1.0, MatchReason: "direct_hit"}).Error)

	// -- composite_hit 场景：tagY 与 boardY 共享组合标签 --
	tagY := createMatchTag(t, db, "backfill-composite")
	composite := createMatchLabel(t, db, "重算组合", "bf-comp", "composite", "active", []float64{1, 0, 0})
	boardY := createMatchLabel(t, db, "Backfill Board Y", "bf-board-y", "board", "active", nil)
	require.NoError(t, db.Create(&models.TopicTagSemanticLabel{TopicTagID: tagY.ID, SemanticLabelID: composite.ID}).Error)
	require.NoError(t, db.Create(&models.BoardComposition{BoardID: boardY.ID, AuxiliaryLabelID: composite.ID}).Error)
	// 存量旧记录：组合标签在旧规则下不参与匹配（仅 aux 交集），先挂一条旧 direct_hit 痕迹
	require.NoError(t, db.Create(&models.TopicTagBoardLabel{TopicTagID: tagY.ID, SemanticBoardID: boardY.ID, Score: 1.0, MatchReason: "direct_hit"}).Error)

	service := NewSemanticBoardBackfillService(db)
	job, err := service.Enqueue(context.Background(), SemanticBoardBackfillRequest{Mode: SemanticBoardBackfillModeAll})
	require.NoError(t, err)
	job = waitForSemanticBoardBackfillJob(t, service, job.ID)
	require.Equal(t, SemanticBoardBackfillStatusCompleted, job.Status)

	// direct_hit 存量 → 降级 0.7 + 方向不符标记
	var rowX models.TopicTagBoardLabel
	require.NoError(t, db.Where("topic_tag_id = ?", tagX.ID).First(&rowX).Error)
	require.Equal(t, "direct_hit", rowX.MatchReason)
	require.InDelta(t, 0.7, rowX.Score, 0.0001)
	require.True(t, rowX.DirectionMismatch, "direction cosine below threshold must be flagged after backfill")

	// 组合候选 → composite_hit 1.0
	var rowY models.TopicTagBoardLabel
	require.NoError(t, db.Where("topic_tag_id = ?", tagY.ID).First(&rowY).Error)
	require.Equal(t, "composite_hit", rowY.MatchReason)
	require.InDelta(t, 1.0, rowY.Score, 0.0001)
	require.False(t, rowY.DirectionMismatch)

	// 幂等：重跑 mode="all" 结果稳定（无重复挂载）
	job2, err := service.Enqueue(context.Background(), SemanticBoardBackfillRequest{Mode: SemanticBoardBackfillModeAll})
	require.NoError(t, err)
	job2 = waitForSemanticBoardBackfillJob(t, service, job2.ID)
	require.Equal(t, SemanticBoardBackfillStatusCompleted, job2.Status)
	requireTopicTagBoardIDs(t, db, tagX.ID, []uint{boardX.ID})
	requireTopicTagBoardIDs(t, db, tagY.ID, []uint{boardY.ID})
	var rowX2 models.TopicTagBoardLabel
	require.NoError(t, db.Where("topic_tag_id = ?", tagX.ID).First(&rowX2).Error)
	require.InDelta(t, 0.7, rowX2.Score, 0.0001)
	require.True(t, rowX2.DirectionMismatch)
}
