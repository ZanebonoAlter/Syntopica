package auxrepair

// board-health-triage-advice seam B3 (task 2.3) — the execution phase
// (storage embedding + atomic save) against a real testcontainer Postgres
// with a STUB embedder (no AI call, no network).
//
// Covered: the successful commit (only description + storage embedding move;
// label / aliases / merge_embedding / matches / suggestions / snapshots stay
// untouched), the embedding failure, the save-transaction rollback, the
// version change during the generation (conflict), the disabled / deleted
// label during the generation (unavailable), the ownership preconditions
// (missing / expired / replaced owner), the single storage-mode call, the
// re-entry into an already committed job, and the match-cache invalidation
// happening AFTER the commit.

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
	"syntopica-backend/internal/platform/database"
	"syntopica-backend/internal/platform/testutil"
	tagmodels "syntopica-backend/internal/tagmanagement/models"
	"syntopica-backend/internal/tagmanagement/repository"
	"syntopica-backend/internal/tagmanagement/service/auxlabel"
	"syntopica-backend/internal/tagmanagement/service/core"
)

// init registers the description-repair models for THIS test binary's
// AutoMigrate (production registers them via tagmanagement/wire.go's init →
// database.RegisterModels; this package cannot import the tagmanagement root
// because root → handler → service would close a cycle). Same pattern as
// repository/auxiliary_description_repair_test.go.
func init() {
	database.RegisterModels(
		&tagmodels.AuxiliaryDescriptionRepairJob{},
		&tagmodels.AuxiliaryDescriptionRepairItem{},
	)
}

// ── stub embedder ───────────────────────────────────────────────────────────

// stubEmbedCall records one embedding invocation.
type stubEmbedCall struct {
	input string
	mode  auxlabel.AuxiliaryLabelEmbeddingMode
}

// stubEmbedder is the injectable storage-embedding unit: it counts calls,
// records the exact input and mode, and can run a hook INSIDE the call (to
// simulate a concurrent edit while the embedding is generated).
type stubEmbedder struct {
	mu     sync.Mutex
	calls  []stubEmbedCall
	err    error
	vector string
	hook   func()
	// failCalls maps the 0-based call index to the error it must fail with
	// (a retry can then succeed).
	failCalls map[int]error
}

func (s *stubEmbedder) embed(ctx context.Context, input string, mode auxlabel.AuxiliaryLabelEmbeddingMode) (string, []float64, error) {
	s.mu.Lock()
	s.calls = append(s.calls, stubEmbedCall{input: input, mode: mode})
	index := len(s.calls) - 1
	callErr, hasCallErr := s.failCalls[index]
	s.mu.Unlock()
	if s.hook != nil {
		s.hook()
	}
	if hasCallErr {
		return "", nil, callErr
	}
	if s.err != nil {
		return "", nil, s.err
	}
	return s.vector, nil, nil
}

func (s *stubEmbedder) callCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.calls)
}

func (s *stubEmbedder) callAt(i int) stubEmbedCall {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls[i]
}

// ── fixtures ────────────────────────────────────────────────────────────────

var auxRepairExecSeq int

// auxRepairTestVector is a valid pgvector literal of the production
// dimension (the semantic_labels vector column rejects shorter vectors).
func auxRepairTestVector() string {
	return core.FloatsToPgVector(testutil.PadVector([]float64{1, 0, 0}, testutil.TestEmbeddingDim))
}

// auxRepairTestVectorB is a SECOND valid vector of the same dimension — the
// tests need an old and a new storage vector that really differ (review R4:
// identical vectors cannot prove that the vector was rewritten at all).
func auxRepairTestVectorB() string {
	return core.FloatsToPgVector(testutil.PadVector([]float64{0, 0, 1}, testutil.TestEmbeddingDim))
}

// auxRepairTestVectorFloats is the same vector as floats (the comparison
// against a stored vector must be numeric — Postgres normalizes the literal).
func auxRepairTestVectorFloats(t *testing.T) []float64 {
	t.Helper()
	vec, err := auxlabel.ParsePgVector(auxRepairTestVector())
	require.NoError(t, err)
	return vec
}

// auxRepairStoredFloats parses a vector read back from Postgres.
func auxRepairStoredFloats(t *testing.T, value string) []float64 {
	t.Helper()
	vec, err := auxlabel.ParsePgVector(value)
	require.NoError(t, err)
	return vec
}

func seedAuxExecLabel(t *testing.T, db *gorm.DB, label, status, description string) models.SemanticLabel {
	t.Helper()
	auxRepairExecSeq++
	vec := auxRepairTestVector()
	mergeVec := core.FloatsToPgVector(testutil.PadVector([]float64{0, 1, 0}, testutil.TestEmbeddingDim))
	l := models.SemanticLabel{
		Label:          label,
		Slug:           fmt.Sprintf("aux-exec-%d", auxRepairExecSeq),
		LabelType:      "auxiliary",
		Status:         status,
		Description:    description,
		Aliases:        []string{"别名一", "别名二"},
		Embedding:      &vec,
		MergeEmbedding: &mergeVec,
	}
	require.NoError(t, db.Create(&l).Error)
	return l
}

func seedAuxExecTag(t *testing.T, db *gorm.DB, label, status string) models.TopicTag {
	t.Helper()
	auxRepairExecSeq++
	tag := models.TopicTag{Label: label, Slug: fmt.Sprintf("aux-exec-tag-%d", auxRepairExecSeq), Status: status}
	require.NoError(t, db.Create(&tag).Error)
	return tag
}

// createAuxExecJob creates a confirmed repair task through the service (the
// version token comes from the live context — the client never builds it).
func createAuxExecJob(t *testing.T, svc *Service, labelID uint, key, description string) *JobView {
	t.Helper()
	rc, err := svc.GetRepairContext(context.Background(), labelID)
	require.NoError(t, err)
	job, err := svc.CreateJob(context.Background(), labelID, CreateJobRequest{
		Description:     description,
		ExpectedVersion: rc.AuxiliaryLabel.Version,
		IdempotencyKey:  key,
	})
	require.NoError(t, err)
	return job
}

func loadAuxExecLabel(t *testing.T, db *gorm.DB, id uint) models.SemanticLabel {
	t.Helper()
	var l models.SemanticLabel
	require.NoError(t, db.Where("id = ?", id).First(&l).Error)
	return l
}

func loadAuxExecJob(t *testing.T, db *gorm.DB, id uint) tagmodels.AuxiliaryDescriptionRepairJob {
	t.Helper()
	var job tagmodels.AuxiliaryDescriptionRepairJob
	require.NoError(t, db.Where("id = ?", id).First(&job).Error)
	return job
}

// ── success ─────────────────────────────────────────────────────────────────

// TestAuxiliaryDescriptionRepairExecuteCommitsAtomically covers the happy
// path: exactly ONE storage embedding of "label: new description", the atomic
// save of description + storage embedding, the task advanced to the matching
// stage, and the match-cache invalidation happening AFTER the commit.
//
// Everything else must stay untouched: the label text, its aliases, its
// merge embedding, the match rows, the upgrade suggestions and the old
// health snapshots.
func TestAuxiliaryDescriptionRepairExecuteCommitsAtomically(t *testing.T) {
	db := testutil.SetupTestDB(t)
	aux := seedAuxExecLabel(t, db, "上海", "active", "上海知名商业综合体")
	tag := seedAuxExecTag(t, db, "活跃标签", "active")

	// Neighbouring data that must not change: a match row, an upgrade
	// suggestion and an old health snapshot.
	board := models.SemanticLabel{Label: "中国地理", Slug: "aux-exec-board", LabelType: "board", Status: "active"}
	require.NoError(t, db.Create(&board).Error)
	require.NoError(t, db.Create(&tagmodels.TopicTagBoardLabel{TopicTagID: tag.ID, SemanticBoardID: board.ID}).Error)
	require.NoError(t, db.Create(&tagmodels.BoardUpgradeSuggestion{
		BatchID: "b1", Mode: "create_new", Decision: "create_new", BoardLabel: "中国地理",
		Description: "旧建议", SuggestionHash: "aux-exec-hash", Status: "pending",
	}).Error)
	require.NoError(t, db.Create(&tagmodels.BoardHealthSnapshot{
		SemanticBoardID: board.ID, CheckedAt: time.Now(),
		Metrics: tagmodels.BoardHealthMetrics{
			Board:  tagmodels.BoardHealthBoardMetrics{ReasonDist: map[string]int{}},
			Labels: []tagmodels.BoardHealthLabelMetrics{},
		},
	}).Error)

	before := loadAuxExecLabel(t, db, aux.ID)
	// A DIFFERENT new vector than the seeded one (review R4): identical
	// vectors could not prove that the storage vector was rewritten.
	newVector := auxRepairTestVectorB()
	require.NotNil(t, before.Embedding, "the seeded label must carry a vector")
	stub := &stubEmbedder{vector: newVector}
	var invalidations int
	var observedDescription string
	var observedCommitted bool
	var jobID uint
	svc, err := NewService(db,
		WithEmbedder(stub.embed),
		WithCacheInvalidator(func() {
			invalidations++
			// The commit must already be durable when the cache is invalidated.
			observedDescription = loadAuxExecLabel(t, db, aux.ID).Description
			observedCommitted = loadAuxExecJob(t, db, jobID).DescriptionCommitted
		}),
	)
	require.NoError(t, err)

	// The embedding must be generated OUTSIDE any transaction.
	stub.hook = func() {
		var idleInTx int64
		require.NoError(t, db.Raw(
			`SELECT count(*) FROM pg_stat_activity WHERE datname = current_database() AND state = 'idle in transaction'`).
			Row().Scan(&idleInTx))
		require.Equal(t, int64(0), idleInTx, "the storage embedding must not run inside an open transaction")
	}

	job := createAuxExecJob(t, svc, aux.ID, "exec-1", "  中国东部沿海的直辖市  ")
	jobID = job.ID
	_, err = svc.ClaimJobForExecution(context.Background(), job.ID, "executor-1", time.Minute)
	require.NoError(t, err)

	view, err := svc.ExecuteEmbeddingPhase(context.Background(), job.ID, "executor-1")
	require.NoError(t, err)
	require.Equal(t, "matching", view.Stage)
	require.Equal(t, "matching", view.Status)
	require.True(t, view.DescriptionCommitted)
	require.Equal(t, "中国东部沿海的直辖市", view.NewDescription)

	// Exactly one storage embedding of "label: trimmed description".
	require.Equal(t, 1, stub.callCount())
	call := stub.callAt(0)
	require.Equal(t, "上海: 中国东部沿海的直辖市", call.input)
	require.Equal(t, auxlabel.AuxiliaryLabelEmbeddingModeStorage, call.mode)

	// The label: only description + storage embedding moved.
	after := loadAuxExecLabel(t, db, aux.ID)
	require.Equal(t, "中国东部沿海的直辖市", after.Description)
	require.NotNil(t, after.Embedding)
	// Postgres normalizes the vector literal on read ("[1,0,...]"), so the
	// comparison is numeric, not textual.
	require.Equal(t, auxRepairStoredFloats(t, newVector), auxRepairStoredFloats(t, *after.Embedding),
		"the stored storage vector must be the NEW one")
	require.NotEqual(t, auxRepairStoredFloats(t, *before.Embedding), auxRepairStoredFloats(t, *after.Embedding),
		"the old and the new storage vector must differ (review R4)")
	require.Equal(t, before.Label, after.Label, "the label text must not change")
	require.Equal(t, before.Aliases, after.Aliases, "the aliases must not change")
	require.Equal(t, before.MergeEmbedding, after.MergeEmbedding, "the merge embedding must not change")
	require.Equal(t, before.Slug, after.Slug)
	require.Equal(t, "active", after.Status)

	// The task: committed, handed to the matching stage, with the committed
	// version recorded internally.
	persisted := loadAuxExecJob(t, db, job.ID)
	require.True(t, persisted.DescriptionCommitted)
	require.Equal(t, tagmodels.AuxiliaryDescriptionRepairStageMatching, persisted.Stage)
	require.Equal(t, tagmodels.AuxiliaryDescriptionRepairStatusMatching, persisted.Status)
	require.NotNil(t, persisted.CommittedVersion)
	require.NotEqual(t, job.ExpectedVersion, *persisted.CommittedVersion,
		"the committed version is the label version AFTER the save")
	// It must be exactly the version token of the label as it is now in PG
	// (review R4) — not merely "different from the old one".
	require.Equal(t, repository.AuxiliaryVersionToken(&after), *persisted.CommittedVersion,
		"the committed version must match the saved label row")
	require.NotNil(t, persisted.CommittedAt)

	// The match cache was invalidated once — AFTER the commit was durable.
	require.Equal(t, 1, invalidations)
	require.Equal(t, "中国东部沿海的直辖市", observedDescription)
	require.True(t, observedCommitted)

	// Neighbouring data is untouched.
	var matches, suggestions, snapshots int64
	require.NoError(t, db.Model(&tagmodels.TopicTagBoardLabel{}).Count(&matches).Error)
	require.Equal(t, int64(1), matches)
	require.NoError(t, db.Model(&tagmodels.BoardUpgradeSuggestion{}).Count(&suggestions).Error)
	require.Equal(t, int64(1), suggestions)
	require.NoError(t, db.Model(&tagmodels.BoardHealthSnapshot{}).Count(&snapshots).Error)
	require.Equal(t, int64(1), snapshots)

	// The frozen scope is untouched (the matching stage owns it).
	var items int64
	require.NoError(t, db.Model(&tagmodels.AuxiliaryDescriptionRepairItem{}).
		Where("job_id = ?", job.ID).Count(&items).Error)
	require.Equal(t, int64(1), items)
}

// ── embedding failure ───────────────────────────────────────────────────────

// TestAuxiliaryDescriptionRepairExecuteEmbeddingFailureKeepsOldValues: a
// failed embedding leaves the old description AND the old vectors untouched
// and records the retryable embedding_failed state.
func TestAuxiliaryDescriptionRepairExecuteEmbeddingFailureKeepsOldValues(t *testing.T) {
	db := testutil.SetupTestDB(t)
	aux := seedAuxExecLabel(t, db, "北京", "active", "北京知名商业综合体")
	before := loadAuxExecLabel(t, db, aux.ID)

	stub := &stubEmbedder{err: errors.New("provider exploded")}
	var invalidations int
	svc, err := NewService(db,
		WithEmbedder(stub.embed),
		WithCacheInvalidator(func() { invalidations++ }),
	)
	require.NoError(t, err)

	job := createAuxExecJob(t, svc, aux.ID, "exec-fail", "中国的首都")
	_, err = svc.ClaimJobForExecution(context.Background(), job.ID, "executor-1", time.Minute)
	require.NoError(t, err)

	_, err = svc.ExecuteEmbeddingPhase(context.Background(), job.ID, "executor-1")
	require.ErrorIs(t, err, ErrStageEmbeddingFailed)
	require.Equal(t, 1, stub.callCount())

	after := loadAuxExecLabel(t, db, aux.ID)
	require.Equal(t, before.Description, after.Description, "a failed generation must not change the description")
	require.Equal(t, before.Embedding, after.Embedding, "a failed generation must not touch the vectors")
	require.Equal(t, before.MergeEmbedding, after.MergeEmbedding)

	persisted := loadAuxExecJob(t, db, job.ID)
	require.Equal(t, tagmodels.AuxiliaryDescriptionRepairStatusEmbeddingFailed, persisted.Status)
	require.Equal(t, tagmodels.AuxiliaryDescriptionRepairStageEmbedding, persisted.Stage)
	require.False(t, persisted.DescriptionCommitted)
	require.Nil(t, persisted.CommittedVersion)
	require.Equal(t, 0, invalidations, "a failed save must not invalidate the match cache")
}

// ── save failure (transaction rollback) ─────────────────────────────────────

// TestAuxiliaryDescriptionRepairExecuteSaveFailureRollsBack: the atomic
// save fails INSIDE the transaction (injected CHECK constraint on the task
// row) — the whole transaction rolls back, so the label keeps its old
// description and vectors and the task is not committed.
func TestAuxiliaryDescriptionRepairExecuteSaveFailureRollsBack(t *testing.T) {
	db := testutil.SetupTestDB(t)
	aux := seedAuxExecLabel(t, db, "广州", "active", "广州知名商业综合体")
	before := loadAuxExecLabel(t, db, aux.ID)

	const chk = "aux_repair_commit_inject_chk"
	require.NoError(t, db.Exec(fmt.Sprintf(
		`ALTER TABLE auxiliary_description_repair_jobs ADD CONSTRAINT %s CHECK (committed_version IS NULL OR length(committed_version) <= 5)`, chk)).Error)
	t.Cleanup(func() {
		_ = db.Exec(fmt.Sprintf(`ALTER TABLE auxiliary_description_repair_jobs DROP CONSTRAINT IF EXISTS %s`, chk)).Error
	})

	stub := &stubEmbedder{vector: auxRepairTestVector()}
	var invalidations int
	svc, err := NewService(db,
		WithEmbedder(stub.embed),
		WithCacheInvalidator(func() { invalidations++ }),
	)
	require.NoError(t, err)

	job := createAuxExecJob(t, svc, aux.ID, "exec-save-fail", "南方的中心城市")
	_, err = svc.ClaimJobForExecution(context.Background(), job.ID, "executor-1", time.Minute)
	require.NoError(t, err)

	_, err = svc.ExecuteEmbeddingPhase(context.Background(), job.ID, "executor-1")
	require.ErrorIs(t, err, ErrStageSaveFailed)

	after := loadAuxExecLabel(t, db, aux.ID)
	require.Equal(t, before.Description, after.Description, "the rolled-back transaction must not keep the new description")
	require.Equal(t, before.Embedding, after.Embedding, "the rolled-back transaction must not keep the new vector")
	require.Equal(t, before.MergeEmbedding, after.MergeEmbedding)

	persisted := loadAuxExecJob(t, db, job.ID)
	require.Equal(t, tagmodels.AuxiliaryDescriptionRepairStatusSaveFailed, persisted.Status)
	require.False(t, persisted.DescriptionCommitted)
	require.Nil(t, persisted.CommittedVersion)
	require.Equal(t, 0, invalidations)
}

// ── conflict / unavailable ──────────────────────────────────────────────────

// TestAuxiliaryDescriptionRepairExecuteConflictOnVersionChange: the label is
// edited WHILE the embedding is generated — the commit detects the changed
// source version, refuses to overwrite the newer description and records the
// terminal conflict state.
func TestAuxiliaryDescriptionRepairExecuteConflictOnVersionChange(t *testing.T) {
	db := testutil.SetupTestDB(t)
	aux := seedAuxExecLabel(t, db, "上海", "active", "上海知名商业综合体")
	before := loadAuxExecLabel(t, db, aux.ID)

	stub := &stubEmbedder{vector: auxRepairTestVector()}
	svc, err := NewService(db,
		WithEmbedder(stub.embed),
		WithCacheInvalidator(func() {}),
	)
	require.NoError(t, err)

	job := createAuxExecJob(t, svc, aux.ID, "exec-conflict", "中国东部沿海的直辖市")
	_, err = svc.ClaimJobForExecution(context.Background(), job.ID, "executor-1", time.Minute)
	require.NoError(t, err)

	// A concurrent edit lands between the pre-check and the commit.
	stub.hook = func() {
		require.NoError(t, db.Model(&models.SemanticLabel{}).Where("id = ?", aux.ID).
			Update("description", "别人刚改过的描述").Error)
	}

	_, err = svc.ExecuteEmbeddingPhase(context.Background(), job.ID, "executor-1")
	require.ErrorIs(t, err, ErrStageConflict)

	after := loadAuxExecLabel(t, db, aux.ID)
	require.Equal(t, "别人刚改过的描述", after.Description, "the newer description must not be overwritten")
	require.Equal(t, before.Embedding, after.Embedding, "the newer label keeps its own vector")
	require.Equal(t, before.MergeEmbedding, after.MergeEmbedding)

	persisted := loadAuxExecJob(t, db, job.ID)
	require.Equal(t, tagmodels.AuxiliaryDescriptionRepairStatusConflict, persisted.Status)
	require.False(t, persisted.DescriptionCommitted)
	require.Nil(t, persisted.CommittedVersion)
}

// TestAuxiliaryDescriptionRepairExecuteUnavailableOnDisabledOrDeletedLabel:
// the label is disabled / deleted while the embedding is generated — the task
// ends as unavailable (terminal) and the label is left alone.
func TestAuxiliaryDescriptionRepairExecuteUnavailableOnDisabledOrDeletedLabel(t *testing.T) {
	t.Run("disabled", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		aux := seedAuxExecLabel(t, db, "深圳", "active", "深圳知名商业综合体")

		stub := &stubEmbedder{vector: auxRepairTestVector()}
		svc, err := NewService(db, WithEmbedder(stub.embed), WithCacheInvalidator(func() {}))
		require.NoError(t, err)

		job := createAuxExecJob(t, svc, aux.ID, "exec-disabled", "南方的窗口城市")
		_, err = svc.ClaimJobForExecution(context.Background(), job.ID, "executor-1", time.Minute)
		require.NoError(t, err)

		stub.hook = func() {
			require.NoError(t, db.Model(&models.SemanticLabel{}).Where("id = ?", aux.ID).
				Update("status", "disabled").Error)
		}

		_, err = svc.ExecuteEmbeddingPhase(context.Background(), job.ID, "executor-1")
		require.ErrorIs(t, err, ErrStageUnavailable)

		after := loadAuxExecLabel(t, db, aux.ID)
		require.Equal(t, "disabled", after.Status)
		require.Equal(t, "深圳知名商业综合体", after.Description, "the repair must not write over a disabled label")

		jobRow := loadAuxExecJob(t, db, job.ID)
		require.Equal(t, tagmodels.AuxiliaryDescriptionRepairStatusUnavailable, jobRow.Status)
		require.False(t, jobRow.DescriptionCommitted)
	})

	t.Run("deleted", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		aux := seedAuxExecLabel(t, db, "杭州", "active", "杭州知名商业综合体")

		stub := &stubEmbedder{vector: auxRepairTestVector()}
		svc, err := NewService(db, WithEmbedder(stub.embed), WithCacheInvalidator(func() {}))
		require.NoError(t, err)

		job := createAuxExecJob(t, svc, aux.ID, "exec-deleted", "南方的电商之城")
		_, err = svc.ClaimJobForExecution(context.Background(), job.ID, "executor-1", time.Minute)
		require.NoError(t, err)

		stub.hook = func() {
			require.NoError(t, db.Delete(&models.SemanticLabel{}, aux.ID).Error)
		}

		_, err = svc.ExecuteEmbeddingPhase(context.Background(), job.ID, "executor-1")
		require.ErrorIs(t, err, ErrStageUnavailable)

		var gone models.SemanticLabel
		require.Error(t, db.Where("id = ?", aux.ID).First(&gone).Error, "the label must stay deleted")
		jobRow := loadAuxExecJob(t, db, job.ID)
		require.Equal(t, tagmodels.AuxiliaryDescriptionRepairStatusUnavailable, jobRow.Status)
		require.False(t, jobRow.DescriptionCommitted)
	})

	t.Run("no longer auxiliary", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		aux := seedAuxExecLabel(t, db, "西安", "active", "西安知名商业综合体")

		stub := &stubEmbedder{vector: auxRepairTestVector()}
		svc, err := NewService(db, WithEmbedder(stub.embed), WithCacheInvalidator(func() {}))
		require.NoError(t, err)

		job := createAuxExecJob(t, svc, aux.ID, "exec-non-aux", "十三朝古都")
		_, err = svc.ClaimJobForExecution(context.Background(), job.ID, "executor-1", time.Minute)
		require.NoError(t, err)

		// The row is re-typed while the embedding is generated.
		stub.hook = func() {
			require.NoError(t, db.Model(&models.SemanticLabel{}).Where("id = ?", aux.ID).
				Update("label_type", "board").Error)
		}

		_, err = svc.ExecuteEmbeddingPhase(context.Background(), job.ID, "executor-1")
		require.ErrorIs(t, err, ErrStageUnavailable)

		after := loadAuxExecLabel(t, db, aux.ID)
		require.Equal(t, "board", after.LabelType)
		require.Equal(t, "西安知名商业综合体", after.Description, "the repair must not write over a re-typed label")
		jobRow := loadAuxExecJob(t, db, job.ID)
		require.Equal(t, tagmodels.AuxiliaryDescriptionRepairStatusUnavailable, jobRow.Status)
		require.False(t, jobRow.DescriptionCommitted)
	})
}

// ── ownership preconditions ─────────────────────────────────────────────────

// TestAuxiliaryDescriptionRepairExecuteRefusesWithoutLiveOwnership: without
// a live ownership the phase must refuse — no embedding call, no write, and
// never a step on the current owner's job.
func TestAuxiliaryDescriptionRepairExecuteRefusesWithoutLiveOwnership(t *testing.T) {
	t.Run("missing owner", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		aux := seedAuxExecLabel(t, db, "成都", "active", "成都知名商业综合体")
		before := loadAuxExecLabel(t, db, aux.ID)

		stub := &stubEmbedder{vector: auxRepairTestVector()}
		svc, err := NewService(db, WithEmbedder(stub.embed), WithCacheInvalidator(func() {}))
		require.NoError(t, err)

		job := createAuxExecJob(t, svc, aux.ID, "exec-no-owner", "西部的中心城市")
		_, err = svc.ExecuteEmbeddingPhase(context.Background(), job.ID, "executor-1")
		require.ErrorIs(t, err, ErrStageOwnerLost)
		require.Equal(t, 0, stub.callCount(), "a refused execution must not call the AI")

		after := loadAuxExecLabel(t, db, aux.ID)
		require.Equal(t, before.Description, after.Description)
		require.Equal(t, before.Embedding, after.Embedding)
		require.Equal(t, tagmodels.AuxiliaryDescriptionRepairStatusPending, loadAuxExecJob(t, db, job.ID).Status)
	})

	t.Run("expired lease", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		aux := seedAuxExecLabel(t, db, "重庆", "active", "重庆知名商业综合体")
		before := loadAuxExecLabel(t, db, aux.ID)

		stub := &stubEmbedder{vector: auxRepairTestVector()}
		svc, err := NewService(db, WithEmbedder(stub.embed), WithCacheInvalidator(func() {}))
		require.NoError(t, err)

		job := createAuxExecJob(t, svc, aux.ID, "exec-expired", "山城的别称")
		// A negative lease is already expired — the guard must refuse it.
		_, err = svc.ClaimJobForExecution(context.Background(), job.ID, "executor-1", -time.Minute)
		require.NoError(t, err)

		_, err = svc.ExecuteEmbeddingPhase(context.Background(), job.ID, "executor-1")
		require.ErrorIs(t, err, ErrStageOwnerLost)
		require.Equal(t, 0, stub.callCount())

		after := loadAuxExecLabel(t, db, aux.ID)
		require.Equal(t, before.Description, after.Description)
		require.Equal(t, before.Embedding, after.Embedding)
	})

	t.Run("owner replaced", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		aux := seedAuxExecLabel(t, db, "武汉", "active", "武汉知名商业综合体")
		before := loadAuxExecLabel(t, db, aux.ID)

		stub := &stubEmbedder{vector: auxRepairTestVector()}
		svc, err := NewService(db, WithEmbedder(stub.embed), WithCacheInvalidator(func() {}))
		require.NoError(t, err)

		job := createAuxExecJob(t, svc, aux.ID, "exec-replaced", "江城的别称")
		_, err = svc.ClaimJobForExecution(context.Background(), job.ID, "executor-1", time.Minute)
		require.NoError(t, err)
		// Another executor takes the job over.
		require.NoError(t, db.Model(&tagmodels.AuxiliaryDescriptionRepairJob{}).Where("id = ?", job.ID).
			Update("claim_owner", "executor-2").Error)

		_, err = svc.ExecuteEmbeddingPhase(context.Background(), job.ID, "executor-1")
		require.ErrorIs(t, err, ErrStageOwnerLost)
		require.Equal(t, 0, stub.callCount())

		after := loadAuxExecLabel(t, db, aux.ID)
		require.Equal(t, before.Description, after.Description)
		require.Equal(t, before.Embedding, after.Embedding)
		jobRow := loadAuxExecJob(t, db, job.ID)
		require.NotNil(t, jobRow.ClaimOwner)
		require.Equal(t, "executor-2", *jobRow.ClaimOwner, "the new owner must not be stepped on")
		require.Equal(t, tagmodels.AuxiliaryDescriptionRepairStatusPending, jobRow.Status)
	})
}

// ── re-entry ────────────────────────────────────────────────────────────────

// TestAuxiliaryDescriptionRepairExecuteSkipsAlreadyCommittedJob: re-entering
// the embedding stage of a committed job must not embed again, must not save
// again and must not move the job backwards.
func TestAuxiliaryDescriptionRepairExecuteSkipsAlreadyCommittedJob(t *testing.T) {
	db := testutil.SetupTestDB(t)
	aux := seedAuxExecLabel(t, db, "南京", "active", "南京知名商业综合体")

	stub := &stubEmbedder{vector: auxRepairTestVector()}
	svc, err := NewService(db, WithEmbedder(stub.embed), WithCacheInvalidator(func() {}))
	require.NoError(t, err)

	job := createAuxExecJob(t, svc, aux.ID, "exec-reentry", "六朝古都")
	_, err = svc.ClaimJobForExecution(context.Background(), job.ID, "executor-1", time.Minute)
	require.NoError(t, err)
	_, err = svc.ExecuteEmbeddingPhase(context.Background(), job.ID, "executor-1")
	require.NoError(t, err)
	require.Equal(t, 1, stub.callCount())

	saved := loadAuxExecLabel(t, db, aux.ID)
	savedJob := loadAuxExecJob(t, db, job.ID)

	// Second run: the job is already committed → no new embedding, no new save.
	_, err = svc.ExecuteEmbeddingPhase(context.Background(), job.ID, "executor-1")
	require.ErrorIs(t, err, ErrStageAlreadyCommitted)
	require.Equal(t, 1, stub.callCount(), "a committed job must not be embedded again")

	after := loadAuxExecLabel(t, db, aux.ID)
	require.Equal(t, saved.Description, after.Description)
	require.Equal(t, saved.Embedding, after.Embedding)
	afterJob := loadAuxExecJob(t, db, job.ID)
	require.Equal(t, savedJob.Status, afterJob.Status, "the stage must not go backwards")
	require.Equal(t, savedJob.Stage, afterJob.Stage)
	require.True(t, afterJob.DescriptionCommitted)
}

// ── review R1: the lease is judged by the database's REAL-TIME clock ──────

// waitForBlockedLabelLock blocks until some backend of this database waits on
// a row lock — the deterministic proof that the repair phase really waits for
// the label row instead of relying on a start-up order.
func waitForBlockedLabelLock(t *testing.T, db *gorm.DB) {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		var waiting int64
		require.NoError(t, db.Raw(
			`SELECT count(*) FROM pg_stat_activity WHERE datname = current_database() AND wait_event_type = 'Lock'`).
			Row().Scan(&waiting))
		if waiting > 0 {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("the repair phase never waited on the label lock")
}

// TestAuxiliaryDescriptionRepairExecuteRejectsLeaseExpiredWhileWaiting
// (review R1, first regression): the phase waits for the label row while
// ANOTHER transaction holds the lock, the lease expires during that wait —
// the save must be refused and nothing may be written.
//
// The lease is judged with now() (frozen at the transaction start) this would
// still pass the check; the fix uses the database's real-time clock.
func TestAuxiliaryDescriptionRepairExecuteRejectsLeaseExpiredWhileWaiting(t *testing.T) {
	db := testutil.SetupTestDB(t)
	aux := seedAuxExecLabel(t, db, "上海", "active", "旧描述")
	before := loadAuxExecLabel(t, db, aux.ID)

	stub := &stubEmbedder{vector: auxRepairTestVectorB()}
	svc, err := NewService(db,
		WithEmbedder(stub.embed),
		WithCacheInvalidator(func() {}),
	)
	require.NoError(t, err)

	job := createAuxExecJob(t, svc, aux.ID, "lease-expired", "中国东部沿海的直辖市")
	// A SHORT lease: it must expire while the phase waits for the label lock.
	_, err = svc.ClaimJobForExecution(context.Background(), job.ID, "executor-1", time.Second)
	require.NoError(t, err)

	// Another transaction holds the label row lock.
	holder := db.Begin()
	require.NoError(t, holder.Exec("SELECT id FROM semantic_labels WHERE id = ? FOR UPDATE", aux.ID).Error)

	done := make(chan error, 1)
	go func() {
		_, err := svc.ExecuteEmbeddingPhase(context.Background(), job.ID, "executor-1")
		done <- err
	}()

	// Wait until the phase really waits for the lock, then let the lease expire.
	waitForBlockedLabelLock(t, db)
	var leaseUntil time.Time
	require.NoError(t, db.Model(&tagmodels.AuxiliaryDescriptionRepairJob{}).Where("id = ?", job.ID).
		Pluck("lease_expires_at", &leaseUntil).Error)
	if d := time.Until(leaseUntil); d > 0 {
		time.Sleep(d + 100*time.Millisecond)
	}

	// Release the lock — the phase continues with an expired lease.
	require.NoError(t, holder.Commit().Error)

	select {
	case err := <-done:
		require.ErrorIs(t, err, ErrStageOwnerLost, "an expired lease must refuse the save")
	case <-time.After(30 * time.Second):
		t.Fatalf("the repair phase did not finish")
	}

	after := loadAuxExecLabel(t, db, aux.ID)
	require.Equal(t, before.Description, after.Description, "the old description must survive")
	require.Equal(t, before.Embedding, after.Embedding, "the old storage vector must survive")
	persisted := loadAuxExecJob(t, db, job.ID)
	require.False(t, persisted.DescriptionCommitted, "nothing may be committed")
	require.Nil(t, persisted.CommittedVersion)
}

// TestAuxiliaryDescriptionRepairExecuteRollsBackWhenCommitMarkerFails
// (review R1, second regression): the label write happens, then the final
// commit marker fails — the WHOLE transaction must roll back, including the
// already written description and vector.
//
// The delay is injected on the label write, so the lease expires between the
// post-lock check and the commit marker: the marker's own condition then
// fails (RowsAffected == 0) and the transaction must undo everything.
func TestAuxiliaryDescriptionRepairExecuteRollsBackWhenCommitMarkerFails(t *testing.T) {
	db := testutil.SetupTestDB(t)
	aux := seedAuxExecLabel(t, db, "广州", "active", "旧描述")
	before := loadAuxExecLabel(t, db, aux.ID)

	// A BEFORE UPDATE trigger that delays the description write by 2.5s — the
	// lease (1s) expires while the label row is being written.
	require.NoError(t, db.Exec(`
		CREATE OR REPLACE FUNCTION aux_repair_delay_label_write() RETURNS trigger AS $$
		BEGIN
			IF NEW.description IS DISTINCT FROM OLD.description THEN
				PERFORM pg_sleep(2.5);
			END IF;
			RETURN NEW;
		END $$ LANGUAGE plpgsql`).Error)
	require.NoError(t, db.Exec(`
		CREATE TRIGGER aux_repair_delay_label_write_trg
		BEFORE UPDATE ON semantic_labels
		FOR EACH ROW EXECUTE FUNCTION aux_repair_delay_label_write()`).Error)
	t.Cleanup(func() {
		_ = db.Exec("DROP TRIGGER IF EXISTS aux_repair_delay_label_write_trg ON semantic_labels").Error
		_ = db.Exec("DROP FUNCTION IF EXISTS aux_repair_delay_label_write()").Error
	})

	stub := &stubEmbedder{vector: auxRepairTestVectorB()}
	svc, err := NewService(db,
		WithEmbedder(stub.embed),
		WithCacheInvalidator(func() {}),
	)
	require.NoError(t, err)

	job := createAuxExecJob(t, svc, aux.ID, "marker-fails", "南方的中心城市")
	_, err = svc.ClaimJobForExecution(context.Background(), job.ID, "executor-1", time.Second)
	require.NoError(t, err)

	_, err = svc.ExecuteEmbeddingPhase(context.Background(), job.ID, "executor-1")
	require.ErrorIs(t, err, ErrStageOwnerLost,
		"the commit marker must refuse to commit under an expired lease")

	after := loadAuxExecLabel(t, db, aux.ID)
	require.Equal(t, before.Description, after.Description,
		"the label write must be rolled back with the failed commit marker")
	require.Equal(t, before.Embedding, after.Embedding,
		"the vector write must be rolled back with the failed commit marker")
	persisted := loadAuxExecJob(t, db, job.ID)
	require.False(t, persisted.DescriptionCommitted)
	require.Nil(t, persisted.CommittedVersion)
}

// ── review R2: the phase execution right ───────────────────────────────────

// TestAuxiliaryDescriptionRepairExecuteRefusesConcurrentSameOwner (review
// R2): two attempts under the SAME valid owner must not both embed. The
// second one is refused as busy, it does NOT change the first attempt's
// state, and after the first attempt is released exactly ONE save happens.
func TestAuxiliaryDescriptionRepairExecuteRefusesConcurrentSameOwner(t *testing.T) {
	db := testutil.SetupTestDB(t)
	aux := seedAuxExecLabel(t, db, "成都", "active", "旧描述")

	entered := make(chan struct{}, 1)
	release := make(chan struct{})
	var once sync.Once
	stub := &stubEmbedder{
		vector: auxRepairTestVectorB(),
		hook: func() {
			once.Do(func() {
				entered <- struct{}{}
				<-release
			})
		},
	}
	svc, err := NewService(db,
		WithEmbedder(stub.embed),
		WithCacheInvalidator(func() {}),
	)
	require.NoError(t, err)

	job := createAuxExecJob(t, svc, aux.ID, "concurrent-owner", "西部的中心城市")
	_, err = svc.ClaimJobForExecution(context.Background(), job.ID, "executor-1", time.Minute)
	require.NoError(t, err)

	firstDone := make(chan error, 1)
	go func() {
		_, err := svc.ExecuteEmbeddingPhase(context.Background(), job.ID, "executor-1")
		firstDone <- err
	}()
	<-entered // the first attempt is inside the embedder

	// The second attempt under the SAME owner must be refused — and it must
	// not touch the running attempt's state.
	_, err = svc.ExecuteEmbeddingPhase(context.Background(), job.ID, "executor-1")
	require.ErrorIs(t, err, ErrStageBusy)
	require.Equal(t, 1, stub.callCount(), "the refused attempt must not embed")

	during := loadAuxExecJob(t, db, job.ID)
	require.Equal(t, tagmodels.AuxiliaryDescriptionRepairStatusEmbedding, during.Status)
	require.NotNil(t, during.PhaseToken)

	// Release the first attempt: exactly one atomic save happens.
	close(release)
	select {
	case err := <-firstDone:
		require.NoError(t, err)
	case <-time.After(30 * time.Second):
		t.Fatalf("the first attempt did not finish")
	}

	require.Equal(t, 1, stub.callCount(), "exactly one embedding call")
	after := loadAuxExecLabel(t, db, aux.ID)
	require.Equal(t, "西部的中心城市", after.Description, "exactly one save")
	persisted := loadAuxExecJob(t, db, job.ID)
	require.True(t, persisted.DescriptionCommitted)
	require.Equal(t, tagmodels.AuxiliaryDescriptionRepairStatusMatching, persisted.Status)
	require.Nil(t, persisted.PhaseToken, "the finished attempt releases the phase right")
}

// TestAuxiliaryDescriptionRepairExecuteRetryAfterFailure (review R2 + R3):
// a failed attempt leaves the retryable state and the error message; the
// explicit retry clears the old error and succeeds — the persisted Error of
// the successful task is EMPTY.
func TestAuxiliaryDescriptionRepairExecuteRetryAfterFailure(t *testing.T) {
	db := testutil.SetupTestDB(t)
	aux := seedAuxExecLabel(t, db, "武汉", "active", "旧描述")

	stub := &stubEmbedder{
		vector: auxRepairTestVectorB(),
		failCalls: map[int]error{
			0: errors.New("provider exploded"),
		},
	}
	svc, err := NewService(db,
		WithEmbedder(stub.embed),
		WithCacheInvalidator(func() {}),
	)
	require.NoError(t, err)

	job := createAuxExecJob(t, svc, aux.ID, "retry-after-failure", "江城的别称")
	_, err = svc.ClaimJobForExecution(context.Background(), job.ID, "executor-1", time.Minute)
	require.NoError(t, err)

	_, err = svc.ExecuteEmbeddingPhase(context.Background(), job.ID, "executor-1")
	require.ErrorIs(t, err, ErrStageEmbeddingFailed)
	failed := loadAuxExecJob(t, db, job.ID)
	require.Equal(t, tagmodels.AuxiliaryDescriptionRepairStatusEmbeddingFailed, failed.Status)
	require.NotEmpty(t, failed.Error, "the failure reason is persisted")
	require.Nil(t, failed.PhaseToken, "a failed attempt releases the phase right")

	// The explicit retry succeeds and the old failure message is gone.
	view, err := svc.ExecuteEmbeddingPhase(context.Background(), job.ID, "executor-1")
	require.NoError(t, err)
	require.Equal(t, "matching", view.Status)
	require.Empty(t, view.Error, "the old failure must not be exposed by the successful task")
	require.Equal(t, 2, stub.callCount())
	persisted := loadAuxExecJob(t, db, job.ID)
	require.Empty(t, persisted.Error, "the persisted error must be cleared")
	require.True(t, persisted.DescriptionCommitted)
}

// TestAuxiliaryDescriptionRepairExecuteAuthorizedRecoveryAfterLeaseExpiry
// (review R2): the dead attempt's phase right is voided by the claim
// takeover, so a new attempt (with a fresh token) may take over — while the
// stale in-flight attempt can no longer commit.
func TestAuxiliaryDescriptionRepairExecuteAuthorizedRecoveryAfterLeaseExpiry(t *testing.T) {
	db := testutil.SetupTestDB(t)
	aux := seedAuxExecLabel(t, db, "南京", "active", "旧描述")
	before := loadAuxExecLabel(t, db, aux.ID)

	stub := &stubEmbedder{vector: auxRepairTestVectorB()}
	svc, err := NewService(db,
		WithEmbedder(stub.embed),
		WithCacheInvalidator(func() {}),
	)
	require.NoError(t, err)

	job := createAuxExecJob(t, svc, aux.ID, "recovery", "六朝古都")
	_, err = svc.ClaimJobForExecution(context.Background(), job.ID, "executor-1", time.Minute)
	require.NoError(t, err)

	// The first attempt dies inside the embedder: the lease expires and the
	// job keeps the phase state of the dead attempt.
	entered := make(chan struct{}, 1)
	release := make(chan struct{})
	var once sync.Once
	stub.hook = func() {
		once.Do(func() {
			entered <- struct{}{}
			<-release
		})
	}
	staleDone := make(chan error, 1)
	go func() {
		_, err := svc.ExecuteEmbeddingPhase(context.Background(), job.ID, "executor-1")
		staleDone <- err
	}()
	<-entered

	// The lease expires; a NEW owner takes the job over (the claim takeover
	// voids the dead attempt's phase token).
	require.NoError(t, db.Model(&tagmodels.AuxiliaryDescriptionRepairJob{}).Where("id = ?", job.ID).
		Update("lease_expires_at", time.Now().Add(-time.Minute)).Error)
	_, err = svc.ClaimJobForExecution(context.Background(), job.ID, "executor-2", time.Minute)
	require.NoError(t, err)
	takenOver := loadAuxExecJob(t, db, job.ID)
	require.Nil(t, takenOver.PhaseToken, "the takeover voids the stale phase right")

	// The stale attempt must not be able to commit.
	close(release)
	select {
	case err := <-staleDone:
		require.Error(t, err, "the stale attempt must not succeed")
	case <-time.After(30 * time.Second):
		t.Fatalf("the stale attempt did not finish")
	}
	afterStale := loadAuxExecLabel(t, db, aux.ID)
	require.Equal(t, before.Description, afterStale.Description, "the stale attempt must not save")
	require.False(t, loadAuxExecJob(t, db, job.ID).DescriptionCommitted)

	// The new owner may take over the phase and finish the work.
	recovered, err := svc.ExecuteEmbeddingPhase(context.Background(), job.ID, "executor-2")
	require.NoError(t, err)
	require.True(t, recovered.DescriptionCommitted)
	require.Equal(t, "matching", recovered.Status)
}
