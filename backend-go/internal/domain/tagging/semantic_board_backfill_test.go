package tagging

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"my-robot-backend/internal/domain/models"
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
