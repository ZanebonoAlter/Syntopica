package auxrepair

// board-health-triage-advice seam B4 (task 2.4) — the matching phase
// (frozen scope → existing board matching engine → per-item persistence and
// job counters) against a real testcontainer Postgres.
//
// Covered: the whole frozen scope runs through the engine (unmounted tags
// included), tags created after the freeze stay out, legacy ''/NULL tags stay
// applicable, deleted/disabled/merged tags are recorded as no longer
// applicable (never a success, total never shrinks), a legal zero-hit counts
// as a success, the REAL engine persists a board mount, a partial failure
// continues with the siblings and never completes, a retry fills only the
// failures without double counting, the ownership preconditions refuse the
// run (missing / expired / replaced owner), an uncommitted description runs
// no engine call, an empty scope completes, and the counters always add up.

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/testutil"
	tagmodels "syntopica-backend/internal/tagmanagement/models"
	"syntopica-backend/internal/tagmanagement/service/board"
)

// ── stub matcher ────────────────────────────────────────────────────────────

// stubMatcher records every engine call, can fail for a specific tag and can
// run a hook inside the call (to simulate a takeover while matching).
type stubMatcher struct {
	mu       sync.Mutex
	calls    []uint
	failTags map[uint]error
	hook     func(tagID uint)
}

func (m *stubMatcher) MatchTopicTag(ctx context.Context, topicTagID uint) ([]board.SemanticBoardMatchResult, error) {
	m.mu.Lock()
	m.calls = append(m.calls, topicTagID)
	m.mu.Unlock()
	if m.hook != nil {
		m.hook(topicTagID)
	}
	if err, ok := m.failTags[topicTagID]; ok {
		return nil, err
	}
	// A legal zero-hit: the engine ran and found no board. That is a SUCCESS.
	return nil, nil
}

func (m *stubMatcher) callCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.calls)
}

func (m *stubMatcher) calledTags() []uint {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]uint, len(m.calls))
	copy(out, m.calls)
	return out
}

// ── helpers ─────────────────────────────────────────────────────────────────

// prepareCommittedRepairJob creates a confirmed repair task and runs the save
// phase, so the job is committed and ready for the matching phase (the
// description + storage vector are already written).
func prepareCommittedRepairJob(t *testing.T, svc *Service, labelID uint, key, description string) *JobView {
	t.Helper()
	job := createAuxExecJob(t, svc, labelID, key, description)
	_, err := svc.ClaimJobForExecution(context.Background(), job.ID, "executor-1", time.Minute)
	require.NoError(t, err)
	committed, err := svc.ExecuteEmbeddingPhase(context.Background(), job.ID, "executor-1")
	require.NoError(t, err)
	require.True(t, committed.DescriptionCommitted)
	return committed
}

// frozenItemStatuses returns the persisted item rows of a job keyed by tag id.
func frozenItemStatuses(t *testing.T, db *gorm.DB, jobID uint) map[uint]string {
	t.Helper()
	var items []tagmodels.AuxiliaryDescriptionRepairItem
	require.NoError(t, db.Where("job_id = ?", jobID).Order("topic_tag_id ASC").Find(&items).Error)
	out := make(map[uint]string, len(items))
	for _, it := range items {
		out[it.TopicTagID] = it.Status
	}
	return out
}

// ── the whole scope runs ────────────────────────────────────────────────────

// TestAuxiliaryDescriptionRepairMatchingRunsWholeFrozenScope: every frozen
// item is handed to the engine (unmounted tags included), the counters come
// from the persisted items, and the phase ends as completed. Tags created
// AFTER the freeze stay out of the scope, and legacy ”/NULL tags stay
// applicable.
func TestAuxiliaryDescriptionRepairMatchingRunsWholeFrozenScope(t *testing.T) {
	db := testutil.SetupTestDB(t)
	aux := seedAuxExecLabel(t, db, "上海", "active", "旧描述")
	mounted := seedAuxExecTag(t, db, "已挂标签", "active")
	unmounted := seedAuxExecTag(t, db, "未归属标签", "active")
	legacy := seedAuxExecTag(t, db, "空状态标签", "active")
	// The legacy '' status is part of the active population (the predicate also
	// covers NULL for pre-NOT-NULL databases; the current schema forbids
	// NULL, so '' is the reachable legacy case).
	require.NoError(t, db.Model(&models.TopicTag{}).Where("id = ?", legacy.ID).Update("status", "").Error)

	matcher := &stubMatcher{}
	var invalidations int
	svc, err := NewService(db,
		WithEmbedder((&stubEmbedder{vector: auxRepairTestVector()}).embed),
		WithMatcher(matcher),
		WithCacheInvalidator(func() { invalidations++ }),
	)
	require.NoError(t, err)

	job := prepareCommittedRepairJob(t, svc, aux.ID, "match-scope", "中国东部沿海的直辖市")
	require.Equal(t, 3, job.Total, "the frozen scope holds every active tag (incl. the legacy '' one)")

	// A tag created AFTER the freeze must not join this job.
	late := seedAuxExecTag(t, db, "后来新增标签", "active")

	invalidationsBefore := invalidations
	view, err := svc.ExecuteMatchingPhase(context.Background(), job.ID, "executor-1")
	require.NoError(t, err)

	// The engine was called for every frozen item — and only for them.
	require.Equal(t, 3, matcher.callCount())
	called := matcher.calledTags()
	require.ElementsMatch(t, []uint{mounted.ID, unmounted.ID, legacy.ID}, called)
	require.NotContains(t, called, late.ID, "a tag created after the freeze is not part of the job")

	// The counters: processed = every item, no failures, nothing remaining.
	require.Equal(t, "completed", view.Status)
	require.Equal(t, "matching", view.Stage)
	require.Equal(t, 3, view.Processed)
	require.Equal(t, 0, view.Failed)
	require.Equal(t, 0, view.NotApplicable)
	require.Equal(t, 0, view.Remaining)
	require.Equal(t, view.Total, view.Processed+view.Failed+view.NotApplicable+view.Remaining)
	require.Empty(t, view.Error, "a successful phase clears the old error")

	items := frozenItemStatuses(t, db, job.ID)
	require.Len(t, items, 3)
	for tagID, status := range items {
		require.Equal(t, tagmodels.AuxiliaryDescriptionRepairItemSucceeded, status, "tag %d", tagID)
	}
	require.Greater(t, invalidations, invalidationsBefore, "the match cache is invalidated before every run")
}

// ── no longer applicable ────────────────────────────────────────────────────

// TestAuxiliaryDescriptionRepairMatchingMarksNotApplicable: a frozen tag that
// was deleted, disabled or merged after the freeze is recorded as no longer
// applicable — the engine is NOT called for it, it counts neither as success
// nor as failure, and the frozen total does not shrink.
func TestAuxiliaryDescriptionRepairMatchingMarksNotApplicable(t *testing.T) {
	db := testutil.SetupTestDB(t)
	aux := seedAuxExecLabel(t, db, "深圳", "active", "旧描述")
	deleted := seedAuxExecTag(t, db, "被删除标签", "active")
	disabled := seedAuxExecTag(t, db, "被禁用标签", "active")
	merged := seedAuxExecTag(t, db, "被合并标签", "active")
	alive := seedAuxExecTag(t, db, "存活标签", "active")

	matcher := &stubMatcher{}
	svc, err := NewService(db,
		WithEmbedder((&stubEmbedder{vector: auxRepairTestVector()}).embed),
		WithMatcher(matcher),
		WithCacheInvalidator(func() {}),
	)
	require.NoError(t, err)

	job := prepareCommittedRepairJob(t, svc, aux.ID, "match-na", "南方的窗口城市")
	require.Equal(t, 4, job.Total)

	// The scope changes after the freeze.
	require.NoError(t, db.Delete(&models.TopicTag{}, deleted.ID).Error)
	require.NoError(t, db.Model(&models.TopicTag{}).Where("id = ?", disabled.ID).Update("status", "disabled").Error)
	require.NoError(t, db.Model(&models.TopicTag{}).Where("id = ?", merged.ID).Update("status", "merged").Error)

	view, err := svc.ExecuteMatchingPhase(context.Background(), job.ID, "executor-1")
	require.NoError(t, err)

	require.Equal(t, 1, matcher.callCount(), "only the still applicable tag reaches the engine")
	require.Equal(t, []uint{alive.ID}, matcher.calledTags())

	require.Equal(t, 1, view.Processed)
	require.Equal(t, 0, view.Failed)
	require.Equal(t, 3, view.NotApplicable)
	require.Equal(t, 0, view.Remaining)
	require.Equal(t, view.Total, view.Processed+view.Failed+view.NotApplicable+view.Remaining)
	require.Equal(t, 4, view.Total, "the frozen total never shrinks")
	require.Equal(t, "completed", view.Status, "every item has a final verdict")

	items := frozenItemStatuses(t, db, job.ID)
	require.Equal(t, tagmodels.AuxiliaryDescriptionRepairItemNotApplicable, items[deleted.ID])
	require.Equal(t, tagmodels.AuxiliaryDescriptionRepairItemNotApplicable, items[disabled.ID])
	require.Equal(t, tagmodels.AuxiliaryDescriptionRepairItemNotApplicable, items[merged.ID])
	require.Equal(t, tagmodels.AuxiliaryDescriptionRepairItemSucceeded, items[alive.ID])
}

// ── the real engine ─────────────────────────────────────────────────────────

// TestAuxiliaryDescriptionRepairMatchingWithRealEngine: the production
// matcher (bound to this service's DB) runs and PERSISTS a board mount — the
// repair chain really updates the match table, it does not simulate success.
func TestAuxiliaryDescriptionRepairMatchingWithRealEngine(t *testing.T) {
	db := testutil.SetupTestDB(t)
	// The package-level board cache is shared; the DB was reset, so drop it.
	board.InvalidateBoardCache()

	aux := seedAuxExecLabel(t, db, "上海", "active", "上海知名商业综合体")
	other := seedAuxExecLabel(t, db, "中国", "active", "国家")
	boardLabel := models.SemanticLabel{Label: "中国地理", Slug: "match-real-board", LabelType: "board", Status: "active"}
	require.NoError(t, db.Create(&boardLabel).Error)
	require.NoError(t, db.Create(&tagmodels.BoardComposition{BoardID: boardLabel.ID, AuxiliaryLabelID: aux.ID}).Error)
	require.NoError(t, db.Create(&tagmodels.BoardComposition{BoardID: boardLabel.ID, AuxiliaryLabelID: other.ID}).Error)

	tag := seedAuxExecTag(t, db, "直辖市新闻", "active")
	require.NoError(t, db.Create(&tagmodels.TopicTagSemanticLabel{TopicTagID: tag.ID, SemanticLabelID: aux.ID}).Error)
	require.NoError(t, db.Create(&tagmodels.TopicTagSemanticLabel{TopicTagID: tag.ID, SemanticLabelID: other.ID}).Error)

	// No matcher override: the production engine (board.NewSemanticBoardMatchingService)
	// bound to this service's DB.
	svc, err := NewService(db,
		WithEmbedder((&stubEmbedder{vector: auxRepairTestVector()}).embed),
		WithCacheInvalidator(func() {}),
	)
	require.NoError(t, err)

	job := prepareCommittedRepairJob(t, svc, aux.ID, "match-real", "中国东部沿海的直辖市")
	view, err := svc.ExecuteMatchingPhase(context.Background(), job.ID, "executor-1")
	require.NoError(t, err)
	require.Equal(t, 1, view.Processed)
	require.Equal(t, 0, view.Remaining)
	require.Equal(t, "completed", view.Status)

	// The engine really wrote the mount.
	var mounts []tagmodels.TopicTagBoardLabel
	require.NoError(t, db.Where("topic_tag_id = ?", tag.ID).Find(&mounts).Error)
	require.Len(t, mounts, 1)
	require.Equal(t, boardLabel.ID, mounts[0].SemanticBoardID)
	require.Equal(t, "direct_hit", mounts[0].MatchReason)
}

// ── partial failure & retry ────────────────────────────────────────────────

// TestAuxiliaryDescriptionRepairMatchingPartialFailureAndRetry: a failing
// item does not stop the siblings, the phase never completes, and a retry
// (through a NEW service instance over the same DB) fills only the failed
// item — without counting anything twice.
func TestAuxiliaryDescriptionRepairMatchingPartialFailureAndRetry(t *testing.T) {
	db := testutil.SetupTestDB(t)
	aux := seedAuxExecLabel(t, db, "武汉", "active", "旧描述")
	ok1 := seedAuxExecTag(t, db, "标签一", "active")
	broken := seedAuxExecTag(t, db, "标签二", "active")
	ok2 := seedAuxExecTag(t, db, "标签三", "active")

	matcher := &stubMatcher{failTags: map[uint]error{broken.ID: errors.New("engine exploded")}}
	svc, err := NewService(db,
		WithEmbedder((&stubEmbedder{vector: auxRepairTestVector()}).embed),
		WithMatcher(matcher),
		WithCacheInvalidator(func() {}),
	)
	require.NoError(t, err)

	job := prepareCommittedRepairJob(t, svc, aux.ID, "match-partial", "江城的别称")
	view, err := svc.ExecuteMatchingPhase(context.Background(), job.ID, "executor-1")
	require.NoError(t, err)

	// The siblings were processed despite the failure.
	require.Equal(t, 3, matcher.callCount())
	require.Equal(t, 2, view.Processed)
	require.Equal(t, 1, view.Failed)
	require.Equal(t, 0, view.NotApplicable)
	require.Equal(t, 0, view.Remaining)
	require.Equal(t, "matching_failed", view.Status, "a partial failure never completes")
	require.NotEmpty(t, view.Error)

	items := frozenItemStatuses(t, db, job.ID)
	require.Equal(t, tagmodels.AuxiliaryDescriptionRepairItemFailed, items[broken.ID])
	require.Equal(t, tagmodels.AuxiliaryDescriptionRepairItemSucceeded, items[ok1.ID])
	require.Equal(t, tagmodels.AuxiliaryDescriptionRepairItemSucceeded, items[ok2.ID])

	// Retry through a NEW service instance over the same DB: only the failed
	// item is processed again, and the counters move — nothing is counted twice.
	retryMatcher := &stubMatcher{}
	retrySvc, err := NewService(db,
		WithEmbedder((&stubEmbedder{vector: auxRepairTestVector()}).embed),
		WithMatcher(retryMatcher),
		WithCacheInvalidator(func() {}),
	)
	require.NoError(t, err)
	_, err = retrySvc.ClaimJobForExecution(context.Background(), job.ID, "executor-2", time.Minute)
	require.NoError(t, err)
	retryView, err := retrySvc.ExecuteMatchingPhase(context.Background(), job.ID, "executor-2")
	require.NoError(t, err)

	require.Equal(t, 1, retryMatcher.callCount(), "the retry only touches the failed item")
	require.Equal(t, []uint{broken.ID}, retryMatcher.calledTags())
	require.Equal(t, 3, retryView.Processed, "the retried item moved from failed to processed")
	require.Equal(t, 0, retryView.Failed)
	require.Equal(t, 0, retryView.NotApplicable)
	require.Equal(t, 0, retryView.Remaining)
	require.Equal(t, retryView.Total, retryView.Processed+retryView.Failed+retryView.NotApplicable+retryView.Remaining)
	require.Equal(t, "completed", retryView.Status)
}

// ── ownership preconditions ─────────────────────────────────────────────────

// TestAuxiliaryDescriptionRepairMatchingRefusesWithoutLiveOwnership: without
// a live ownership the phase must refuse — no engine call, no item write, no
// counter or final state change.
func TestAuxiliaryDescriptionRepairMatchingRefusesWithoutLiveOwnership(t *testing.T) {
	t.Run("missing owner", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		aux := seedAuxExecLabel(t, db, "南京", "active", "旧描述")
		seedAuxExecTag(t, db, "标签", "active")

		matcher := &stubMatcher{}
		svc, err := NewService(db,
			WithEmbedder((&stubEmbedder{vector: auxRepairTestVector()}).embed),
			WithMatcher(matcher),
			WithCacheInvalidator(func() {}),
		)
		require.NoError(t, err)
		// The job was never claimed → nobody owns it.
		job := createAuxExecJob(t, svc, aux.ID, "match-no-owner", "六朝古都")

		_, err = svc.ExecuteMatchingPhase(context.Background(), job.ID, "executor-1")
		require.ErrorIs(t, err, ErrStageOwnerLost)
		require.Equal(t, 0, matcher.callCount(), "a refused phase must not call the engine")
		require.Equal(t, tagmodels.AuxiliaryDescriptionRepairStatusPending, loadAuxExecJob(t, db, job.ID).Status)
	})

	t.Run("expired lease", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		aux := seedAuxExecLabel(t, db, "西安", "active", "旧描述")
		seedAuxExecTag(t, db, "标签", "active")

		matcher := &stubMatcher{}
		svc, err := NewService(db,
			WithEmbedder((&stubEmbedder{vector: auxRepairTestVector()}).embed),
			WithMatcher(matcher),
			WithCacheInvalidator(func() {}),
		)
		require.NoError(t, err)
		job := prepareCommittedRepairJob(t, svc, aux.ID, "match-expired", "十三朝古都")
		// The lease expires after the description was committed.
		require.NoError(t, db.Model(&tagmodels.AuxiliaryDescriptionRepairJob{}).Where("id = ?", job.ID).
			Update("lease_expires_at", time.Now().Add(-time.Minute)).Error)

		_, err = svc.ExecuteMatchingPhase(context.Background(), job.ID, "executor-1")
		require.ErrorIs(t, err, ErrStageOwnerLost)
		require.Equal(t, 0, matcher.callCount())
	})

	t.Run("owner replaced during the run", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		aux := seedAuxExecLabel(t, db, "重庆", "active", "旧描述")
		seedAuxExecTag(t, db, "标签", "active")

		matcher := &stubMatcher{}
		svc, err := NewService(db,
			WithEmbedder((&stubEmbedder{vector: auxRepairTestVector()}).embed),
			WithMatcher(matcher),
			WithCacheInvalidator(func() {}),
		)
		require.NoError(t, err)
		job := prepareCommittedRepairJob(t, svc, aux.ID, "match-replaced", "山城的别称")

		// Another executor takes the job over WHILE the engine runs.
		matcher.hook = func(tagID uint) {
			require.NoError(t, db.Model(&tagmodels.AuxiliaryDescriptionRepairJob{}).Where("id = ?", job.ID).
				Update("claim_owner", "executor-2").Error)
		}

		_, err = svc.ExecuteMatchingPhase(context.Background(), job.ID, "executor-1")
		require.ErrorIs(t, err, ErrStageOwnerLost)

		// Nothing was written: the item, the counters and the final state are
		// untouched, and the new owner is not stepped on.
		items := frozenItemStatuses(t, db, job.ID)
		for _, status := range items {
			require.Equal(t, tagmodels.AuxiliaryDescriptionRepairItemPending, status)
		}
		persisted := loadAuxExecJob(t, db, job.ID)
		require.Equal(t, 0, persisted.Processed)
		require.Equal(t, 0, persisted.Failed)
		require.Equal(t, 0, persisted.NotApplicable)
		require.Equal(t, tagmodels.AuxiliaryDescriptionRepairStatusMatching, persisted.Status)
		require.NotNil(t, persisted.ClaimOwner)
		require.Equal(t, "executor-2", *persisted.ClaimOwner)
	})
}

// ── preconditions & empty scope ────────────────────────────────────────────

// TestAuxiliaryDescriptionRepairMatchingRequiresCommittedDescription: the
// phase must not run before the description was committed (the engine would
// match the OLD vector) — and it must not call the engine at all.
func TestAuxiliaryDescriptionRepairMatchingRequiresCommittedDescription(t *testing.T) {
	db := testutil.SetupTestDB(t)
	aux := seedAuxExecLabel(t, db, "青岛", "active", "旧描述")
	seedAuxExecTag(t, db, "标签", "active")

	matcher := &stubMatcher{}
	svc, err := NewService(db,
		WithEmbedder((&stubEmbedder{vector: auxRepairTestVector()}).embed),
		WithMatcher(matcher),
		WithCacheInvalidator(func() {}),
	)
	require.NoError(t, err)

	job := createAuxExecJob(t, svc, aux.ID, "match-uncommitted", "海滨之城")
	_, err = svc.ClaimJobForExecution(context.Background(), job.ID, "executor-1", time.Minute)
	require.NoError(t, err)

	_, err = svc.ExecuteMatchingPhase(context.Background(), job.ID, "executor-1")
	require.ErrorIs(t, err, ErrStageNotCommitted)
	require.Equal(t, 0, matcher.callCount())
}

// TestAuxiliaryDescriptionRepairMatchingEmptyScopeCompletes: a job without
// any frozen tag completes — nothing to match is not a failure.
func TestAuxiliaryDescriptionRepairMatchingEmptyScopeCompletes(t *testing.T) {
	db := testutil.SetupTestDB(t)
	aux := seedAuxExecLabel(t, db, "厦门", "active", "旧描述")

	matcher := &stubMatcher{}
	svc, err := NewService(db,
		WithEmbedder((&stubEmbedder{vector: auxRepairTestVector()}).embed),
		WithMatcher(matcher),
		WithCacheInvalidator(func() {}),
	)
	require.NoError(t, err)

	job := prepareCommittedRepairJob(t, svc, aux.ID, "match-empty", "海上的花园城市")
	require.Equal(t, 0, job.Total)

	view, err := svc.ExecuteMatchingPhase(context.Background(), job.ID, "executor-1")
	require.NoError(t, err)
	require.Equal(t, "completed", view.Status)
	require.Equal(t, 0, view.Processed)
	require.Equal(t, 0, view.Failed)
	require.Equal(t, 0, view.NotApplicable)
	require.Equal(t, 0, view.Remaining)
	require.Equal(t, 0, matcher.callCount())
}

// ── the embedding phase is not repeated ────────────────────────────────────

// TestAuxiliaryDescriptionRepairMatchingDoesNotReembed: the matching phase
// never embeds again — the confirmed description and its vector are already
// committed, and a retry must not call the AI.
func TestAuxiliaryDescriptionRepairMatchingDoesNotReembed(t *testing.T) {
	db := testutil.SetupTestDB(t)
	aux := seedAuxExecLabel(t, db, "苏州", "active", "旧描述")
	seedAuxExecTag(t, db, "标签", "active")

	embedder := &stubEmbedder{vector: auxRepairTestVector()}
	matcher := &stubMatcher{}
	svc, err := NewService(db,
		WithEmbedder(embedder.embed),
		WithMatcher(matcher),
		WithCacheInvalidator(func() {}),
	)
	require.NoError(t, err)

	job := prepareCommittedRepairJob(t, svc, aux.ID, "match-no-reembed", "江南水乡")
	require.Equal(t, 1, embedder.callCount(), "the save phase embedded once")

	_, err = svc.ExecuteMatchingPhase(context.Background(), job.ID, "executor-1")
	require.NoError(t, err)
	require.Equal(t, 1, embedder.callCount(), "the matching phase must not embed again")

	// The label keeps the committed description and vector.
	after := loadAuxExecLabel(t, db, aux.ID)
	require.Equal(t, "江南水乡", after.Description)
	require.NotNil(t, after.Embedding)
}

// ── neighbours are untouched ────────────────────────────────────────────────

// TestAuxiliaryDescriptionRepairMatchingLeavesNeighboursAlone: the matching
// phase writes the board mounts (its purpose) but never touches the label
// description/vectors, the upgrade suggestions, the health snapshots or the
// daily reports.
func TestAuxiliaryDescriptionRepairMatchingLeavesNeighboursAlone(t *testing.T) {
	db := testutil.SetupTestDB(t)
	aux := seedAuxExecLabel(t, db, "天津", "active", "旧描述")
	tag := seedAuxExecTag(t, db, "标签", "active")

	boardLabel := models.SemanticLabel{Label: "华北", Slug: "match-neighbour-board", LabelType: "board", Status: "active"}
	require.NoError(t, db.Create(&boardLabel).Error)
	require.NoError(t, db.Create(&tagmodels.BoardUpgradeSuggestion{
		BatchID: "b1", Mode: "create_new", Decision: "create_new", BoardLabel: "华北",
		Description: "旧建议", SuggestionHash: "match-neighbour-hash", Status: "pending",
	}).Error)
	require.NoError(t, db.Create(&tagmodels.BoardHealthSnapshot{
		SemanticBoardID: boardLabel.ID, CheckedAt: time.Now(),
		Metrics: tagmodels.BoardHealthMetrics{
			Board:  tagmodels.BoardHealthBoardMetrics{ReasonDist: map[string]int{}},
			Labels: []tagmodels.BoardHealthLabelMetrics{},
		},
	}).Error)

	matcher := &stubMatcher{}
	svc, err := NewService(db,
		WithEmbedder((&stubEmbedder{vector: auxRepairTestVector()}).embed),
		WithMatcher(matcher),
		WithCacheInvalidator(func() {}),
	)
	require.NoError(t, err)

	job := prepareCommittedRepairJob(t, svc, aux.ID, "match-neighbour", "北方的港口城市")
	labelBefore := loadAuxExecLabel(t, db, aux.ID)

	_, err = svc.ExecuteMatchingPhase(context.Background(), job.ID, "executor-1")
	require.NoError(t, err)

	after := loadAuxExecLabel(t, db, aux.ID)
	require.Equal(t, labelBefore.Description, after.Description)
	require.Equal(t, labelBefore.Embedding, after.Embedding)
	require.Equal(t, labelBefore.MergeEmbedding, after.MergeEmbedding)
	require.Equal(t, labelBefore.Aliases, after.Aliases)

	var suggestions, snapshots int64
	require.NoError(t, db.Model(&tagmodels.BoardUpgradeSuggestion{}).Count(&suggestions).Error)
	require.Equal(t, int64(1), suggestions)
	require.NoError(t, db.Model(&tagmodels.BoardHealthSnapshot{}).Count(&snapshots).Error)
	require.Equal(t, int64(1), snapshots)

	// The frozen scope of the job is untouched by the neighbours.
	items := frozenItemStatuses(t, db, job.ID)
	require.Len(t, items, 1)
	require.Equal(t, tagmodels.AuxiliaryDescriptionRepairItemSucceeded, items[tag.ID])
}

// ── a tag that vanishes during the engine call ──────────────────────────────

// TestAuxiliaryDescriptionRepairMatchingVanishedTagBecomesNotApplicable: when
// the engine fails BECAUSE the tag disappeared, the database confirmation
// turns it into "not applicable" — a plain engine failure stays a failure.
func TestAuxiliaryDescriptionRepairMatchingVanishedTagBecomesNotApplicable(t *testing.T) {
	db := testutil.SetupTestDB(t)
	aux := seedAuxExecLabel(t, db, "哈尔滨", "active", "旧描述")
	gone := seedAuxExecTag(t, db, "消失的标签", "active")
	alive := seedAuxExecTag(t, db, "存活标签", "active")

	matcher := &stubMatcher{}
	svc, err := NewService(db,
		WithEmbedder((&stubEmbedder{vector: auxRepairTestVector()}).embed),
		WithMatcher(matcher),
		WithCacheInvalidator(func() {}),
	)
	require.NoError(t, err)

	job := prepareCommittedRepairJob(t, svc, aux.ID, "match-vanished", "冰雪之城")

	// The tag disappears while the engine runs, and the engine reports it.
	matcher.hook = func(tagID uint) {
		if tagID == gone.ID {
			require.NoError(t, db.Delete(&models.TopicTag{}, gone.ID).Error)
		}
	}
	matcher.failTags = map[uint]error{gone.ID: errors.New("tag not found")}

	view, err := svc.ExecuteMatchingPhase(context.Background(), job.ID, "executor-1")
	require.NoError(t, err)
	require.Equal(t, 1, view.Processed)
	require.Equal(t, 0, view.Failed)
	require.Equal(t, 1, view.NotApplicable)
	require.Equal(t, 0, view.Remaining)
	require.Equal(t, "completed", view.Status)

	items := frozenItemStatuses(t, db, job.ID)
	require.Equal(t, tagmodels.AuxiliaryDescriptionRepairItemNotApplicable, items[gone.ID])
	require.Equal(t, tagmodels.AuxiliaryDescriptionRepairItemSucceeded, items[alive.ID])
}

// ── helpers reused from the save-phase test file ───────────────────────────

var _ = fmt.Sprintf
