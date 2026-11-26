package repository

// board-health-triage-advice seam B1 (task 2.1) integration tests: the
// persistent repair-job storage against a real testcontainer Postgres
// (repository 层禁 SQLite). Covers: schema/constraints, atomic scope freeze,
// idempotency (same/different payload, replay after label change), per-label
// mutex incl. retryable-failed stages and concurrency, source-version guard,
// label guards, transaction rollback, deletion non-blocking, and the
// zero-write query/context reads.

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/database"
	"syntopica-backend/internal/platform/testutil"
	tagmodels "syntopica-backend/internal/tagmanagement/models"
)

// init registers the repair-job models for THIS test binary's AutoMigrate:
// production registers them via tagmanagement/wire.go's init →
// database.RegisterModels, but this package cannot import the tagmanagement
// root (root → repository import cycle), and testutil's golden-schema mirror
// list only covers the pre-existing tag models. Runs before the first
// SetupTestDB (Go init order) so the golden schema includes both tables and
// migration 20261028_0001 creates the partial unique mutex index. Same
// pattern as service/board/semantic_board_upgrade_test.go's local init().
func init() {
	database.RegisterModels(
		&tagmodels.AuxiliaryDescriptionRepairJob{},
		&tagmodels.AuxiliaryDescriptionRepairItem{},
	)
}

// ── seed helpers ────────────────────────────────────────────────────────────

var repairSeedSeq int

func seedRepairLabel(t *testing.T, db *gorm.DB, label, status, description string) models.SemanticLabel {
	t.Helper()
	repairSeedSeq++
	l := models.SemanticLabel{
		Label:       label,
		Slug:        fmt.Sprintf("repair-seed-%d-%s", repairSeedSeq, strings.ToLower(label)),
		LabelType:   "auxiliary",
		Status:      status,
		Description: description,
	}
	require.NoError(t, db.Create(&l).Error)
	return l
}

func seedRepairTag(t *testing.T, db *gorm.DB, label, status string) models.TopicTag {
	t.Helper()
	repairSeedSeq++
	tag := models.TopicTag{Label: label, Slug: fmt.Sprintf("repair-tag-%d", repairSeedSeq), Status: status}
	require.NoError(t, db.Create(&tag).Error)
	return tag
}

// repairContextSnapshot captures the label fields creation must never touch.
type repairLabelSnapshot struct {
	Label          string
	Description    string
	Status         string
	UpdatedAt      string
	Embedding      *string
	MergeEmbedding *string
}

func snapshotRepairLabel(t *testing.T, db *gorm.DB, id uint) repairLabelSnapshot {
	t.Helper()
	var l models.SemanticLabel
	require.NoError(t, db.Where("id = ?", id).First(&l).Error)
	return repairLabelSnapshot{
		Label: l.Label, Description: l.Description, Status: l.Status,
		UpdatedAt: l.UpdatedAt.UTC().Format("2006-01-02T15:04:05.999999999"),
		Embedding: l.Embedding, MergeEmbedding: l.MergeEmbedding,
	}
}

func requireRepairLabelUnchanged(t *testing.T, db *gorm.DB, id uint, before repairLabelSnapshot) {
	t.Helper()
	after := snapshotRepairLabel(t, db, id)
	require.Equal(t, before, after, "creation/reads must not modify the source label row")
}

// createRepairJobViaCtx is the canonical happy-path creation: read the live
// context (version token), then submit with that token.
func createRepairJobViaCtx(t *testing.T, repo *AuxiliaryDescriptionRepairRepository, labelID uint, key, description string) (*tagmodels.AuxiliaryDescriptionRepairJob, error) {
	t.Helper()
	ctx, err := repo.GetRepairContext(context.Background(), labelID)
	require.NoError(t, err)
	return repo.CreateRepairJob(context.Background(), CreateRepairJobParams{
		AuxiliaryLabelID: labelID,
		IdempotencyKey:   key,
		ExpectedVersion:  ctx.Version,
		NewDescription:   description,
	})
}

// ── schema & DB-level constraints ───────────────────────────────────────────

// TestAuxiliaryDescriptionRepairSchemaConstraints verifies the golden schema
// carries the two full unique indexes (AutoMigrate) and the partial mutex
// index (migration 20261028_0001), and that the DB enforces all three on raw
// SQL — the guarantees the repository relies on under concurrency.
func TestAuxiliaryDescriptionRepairSchemaConstraints(t *testing.T) {
	db := testutil.SetupTestDB(t)

	type idxRow struct {
		Indexname string
		Indexdef  string
	}
	var idx []idxRow
	require.NoError(t, db.Raw(`
		SELECT indexname, indexdef FROM pg_indexes
		WHERE tablename IN ('auxiliary_description_repair_jobs', 'auxiliary_description_repair_items')
		ORDER BY indexname`).Scan(&idx).Error)
	defs := map[string]string{}
	for _, r := range idx {
		defs[r.Indexname] = r.Indexdef
	}
	require.Contains(t, defs, "uq_aux_desc_repair_jobs_key", "idempotency_key unique index missing: %v", defs)
	require.Contains(t, defs["uq_aux_desc_repair_jobs_key"], "UNIQUE")
	require.Contains(t, defs, "uq_aux_desc_repair_items_job_tag", "(job_id, topic_tag_id) unique index missing: %v", defs)
	require.Contains(t, defs, "uq_aux_desc_repair_jobs_label_active", "partial mutex index missing (migration 20261028_0001): %v", defs)
	mutexDef := defs["uq_aux_desc_repair_jobs_label_active"]
	require.Contains(t, mutexDef, "UNIQUE")
	require.Contains(t, mutexDef, "(auxiliary_label_id)")
	// PG normalizes `status NOT IN (...)` to `status <> ALL (ARRAY[...])` in
	// indexdef — assert the three terminal literals instead of raw SQL text.
	for _, terminal := range tagmodels.AuxiliaryDescriptionRepairTerminalStatuses {
		require.Contains(t, mutexDef, "'"+terminal+"'", "mutex predicate must exclude terminal status %s: %s", terminal, mutexDef)
	}

	// DB-level mutex: two unfinished jobs for one label must collide…
	label := seedRepairLabel(t, db, "mutex", "active", "d")
	mkJob := func(key, status string) *tagmodels.AuxiliaryDescriptionRepairJob {
		return &tagmodels.AuxiliaryDescriptionRepairJob{
			AuxiliaryLabelID: label.ID, IdempotencyKey: key, ExpectedVersion: "v1_x",
			NewDescription: "nd", Status: status,
		}
	}
	require.NoError(t, db.Create(mkJob("k1", "pending")).Error)
	err := db.Create(mkJob("k2", "matching_failed")).Error
	require.Error(t, err, "a retryable-failed job must still hold the mutex")
	var pgErr *pgconn.PgError
	require.True(t, errors.As(err, &pgErr) && pgErr.Code == "23505",
		"mutex violation must surface as SQLSTATE 23505 through this driver stack, got %v", err)

	// …while terminal statuses release it.
	require.NoError(t, db.Create(mkJob("k3", "completed")).Error)
	require.NoError(t, db.Create(mkJob("k4", "conflict")).Error)
	require.NoError(t, db.Create(mkJob("k5", "unavailable")).Error)
	var unfinished int64
	require.NoError(t, db.Model(&tagmodels.AuxiliaryDescriptionRepairJob{}).
		Where("auxiliary_label_id = ? AND status NOT IN ?", label.ID,
			tagmodels.AuxiliaryDescriptionRepairTerminalStatuses).Count(&unfinished).Error)
	require.Equal(t, int64(1), unfinished, "only the pending job is unfinished")

	// Composite scope uniqueness: (job_id, topic_tag_id) may not repeat.
	// (Fresh label: k1 pending still holds the mutex on the first label.)
	otherLabel := seedRepairLabel(t, db, "mutex-scope", "active", "d")
	job := &tagmodels.AuxiliaryDescriptionRepairJob{
		AuxiliaryLabelID: otherLabel.ID, IdempotencyKey: "k6", ExpectedVersion: "v1_x",
		NewDescription: "nd", Status: "pending",
	}
	require.NoError(t, db.Create(job).Error)
	require.NoError(t, db.Create(&tagmodels.AuxiliaryDescriptionRepairItem{JobID: job.ID, TopicTagID: 1, Status: "pending"}).Error)
	err = db.Create(&tagmodels.AuxiliaryDescriptionRepairItem{JobID: job.ID, TopicTagID: 1, Status: "pending"}).Error
	require.Error(t, err)
	require.True(t, errors.As(err, &pgErr) && pgErr.Code == "23505", "scope dup must be 23505, got %v", err)
}

// ── creation: atomic scope freeze & zero side effects ───────────────────────

// TestAuxiliaryDescriptionRepairCreateFreezesAllActiveTags covers the frozen
// scope: ALL active topic_tags at creation — including tags with no aux link
// and no match row — non-active tags excluded, later-created tags not added,
// and creation leaving labels/vectors/matches/suggestions untouched.
func TestAuxiliaryDescriptionRepairCreateFreezesAllActiveTags(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repo := NewAuxiliaryDescriptionRepairRepository(db)

	label := seedRepairLabel(t, db, "上海", "active", "上海知名商业综合体")
	require.NoError(t, db.Exec(`UPDATE semantic_labels SET embedding = '[1,0]'::vector, merge_embedding = '[0,1]'::vector WHERE id = ?`, label.ID).Error)
	before := snapshotRepairLabel(t, db, label.ID)

	matchedTag := seedRepairTag(t, db, "有匹配标签", "active")
	orphanTag := seedRepairTag(t, db, "完全未归属标签", "active") // no link, no match row
	legacyTag := seedRepairTag(t, db, "空status历史标签", "")   // legacy '' status = active population
	mergedTag := seedRepairTag(t, db, "已合并标签", "merged")
	// One existing match row + one aux link to prove scope ≠ those tables.
	board := seedRepairBoard(t, db, "中国地理", "active", label.ID)
	require.NoError(t, db.Create(&tagmodels.TopicTagSemanticLabel{TopicTagID: matchedTag.ID, SemanticLabelID: label.ID}).Error)
	require.NoError(t, db.Create(&tagmodels.TopicTagBoardLabel{TopicTagID: matchedTag.ID, SemanticBoardID: board.ID}).Error)

	job, err := createRepairJobViaCtx(t, repo, label.ID, "key-freeze-1", "  中国东部沿海的直辖市  ")
	require.NoError(t, err)
	require.Equal(t, tagmodels.AuxiliaryDescriptionRepairStatusPending, job.Status)
	require.Empty(t, job.Stage)
	require.False(t, job.DescriptionCommitted)
	require.Equal(t, "上海知名商业综合体", job.OldDescription)
	require.Equal(t, "中国东部沿海的直辖市", job.NewDescription, "confirmed text is stored trimmed")
	require.Equal(t, 3, job.Total, "scope = all 3 active tags (matched + orphan + legacy-''), merged excluded")
	require.Equal(t, 0, job.Processed)
	require.Equal(t, 0, job.Failed)

	var items []tagmodels.AuxiliaryDescriptionRepairItem
	require.NoError(t, db.Where("job_id = ?", job.ID).Order("topic_tag_id ASC").Find(&items).Error)
	gotIDs := make([]uint, 0, len(items))
	for _, it := range items {
		require.Equal(t, tagmodels.AuxiliaryDescriptionRepairItemPending, it.Status)
		require.Empty(t, it.Error)
		gotIDs = append(gotIDs, it.TopicTagID)
	}
	require.ElementsMatch(t, []uint{matchedTag.ID, orphanTag.ID, legacyTag.ID}, gotIDs,
		"frozen scope must contain all active tags incl. unmounted/legacy, not the merged one")

	// Fixed set: a tag created afterwards never joins the job's scope.
	lateTag := seedRepairTag(t, db, "后来新增标签", "active")
	require.NoError(t, db.Where("job_id = ?", job.ID).Find(&items).Error)
	for _, it := range items {
		require.NotEqual(t, lateTag.ID, it.TopicTagID, "later tags must not be added to a frozen scope")
	}
	var reloaded tagmodels.AuxiliaryDescriptionRepairJob
	require.NoError(t, db.Where("id = ?", job.ID).First(&reloaded).Error)
	require.Equal(t, 3, reloaded.Total, "total stays the frozen size after later tags appear")

	// Zero side effects: the whole create path writes only job/item rows.
	requireRepairLabelUnchanged(t, db, label.ID, before)
	var matchRows int64
	require.NoError(t, db.Model(&tagmodels.TopicTagBoardLabel{}).Where("topic_tag_id IN ?", []uint{matchedTag.ID, orphanTag.ID, legacyTag.ID, mergedTag.ID}).Count(&matchRows).Error)
	require.Equal(t, int64(1), matchRows, "no new match rows may be written by creation")
	var suggestions int64
	require.NoError(t, db.Model(&tagmodels.BoardUpgradeSuggestion{}).Count(&suggestions).Error)
	require.Equal(t, int64(0), suggestions, "creation must not write upgrade suggestions")
	var snapshots int64
	require.NoError(t, db.Model(&tagmodels.BoardHealthSnapshot{}).Count(&snapshots).Error)
	require.Equal(t, int64(0), snapshots, "creation must not write board-health snapshots")
}

// TestAuxiliaryDescriptionRepairCreateEmptyScope verifies a job over an empty
// active-tag population still persists (total=0, no items) — the repair chain
// later completes matching trivially; B1 only stores it.
func TestAuxiliaryDescriptionRepairCreateEmptyScope(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repo := NewAuxiliaryDescriptionRepairRepository(db)
	label := seedRepairLabel(t, db, "空范围", "active", "old")

	job, err := createRepairJobViaCtx(t, repo, label.ID, "key-empty-scope", "新的描述")
	require.NoError(t, err)
	require.Equal(t, 0, job.Total)
	var items int64
	require.NoError(t, db.Model(&tagmodels.AuxiliaryDescriptionRepairItem{}).Where("job_id = ?", job.ID).Count(&items).Error)
	require.Equal(t, int64(0), items)
}

func seedRepairBoard(t *testing.T, db *gorm.DB, label, status string, auxLabelID uint) models.SemanticLabel {
	t.Helper()
	repairSeedSeq++
	b := models.SemanticLabel{
		Label: label, Slug: fmt.Sprintf("repair-board-%d", repairSeedSeq),
		LabelType: "board", Status: status,
	}
	require.NoError(t, db.Create(&b).Error)
	require.NoError(t, db.Create(&tagmodels.BoardComposition{BoardID: b.ID, AuxiliaryLabelID: auxLabelID}).Error)
	return b
}

// ── creation guards ─────────────────────────────────────────────────────────

// TestAuxiliaryDescriptionRepairCreateGuards covers label guards, version
// guard and the PG-path enforcement of the three description rules: every
// rejection leaves zero job/item rows and the label untouched.
func TestAuxiliaryDescriptionRepairCreateGuards(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repo := NewAuxiliaryDescriptionRepairRepository(db)

	active := seedRepairLabel(t, db, "活跃", "active", "old")
	disabled := seedRepairLabel(t, db, "停用", "disabled", "old")
	board := seedRepairBoard(t, db, "版块不是辅助标签", "active", active.ID)

	ctxLabel, err := repo.GetRepairContext(context.Background(), active.ID)
	require.NoError(t, err)
	token := ctxLabel.Version

	cases := []struct {
		name    string
		params  CreateRepairJobParams
		wantErr error
	}{
		{name: "label not found", params: CreateRepairJobParams{AuxiliaryLabelID: 999999, IdempotencyKey: "g1", ExpectedVersion: token, NewDescription: "d"}, wantErr: ErrRepairLabelNotFound},
		{name: "board label is not auxiliary", params: CreateRepairJobParams{AuxiliaryLabelID: board.ID, IdempotencyKey: "g2", ExpectedVersion: token, NewDescription: "d"}, wantErr: ErrRepairLabelNotFound},
		{name: "disabled label", params: CreateRepairJobParams{AuxiliaryLabelID: disabled.ID, IdempotencyKey: "g3", ExpectedVersion: token, NewDescription: "d"}, wantErr: ErrRepairLabelNotActive},
		{name: "stale version", params: CreateRepairJobParams{AuxiliaryLabelID: active.ID, IdempotencyKey: "g4", ExpectedVersion: "v1_stale", NewDescription: "d"}, wantErr: ErrRepairVersionConflict},
		{name: "description empty after trim", params: CreateRepairJobParams{AuxiliaryLabelID: active.ID, IdempotencyKey: "g5", ExpectedVersion: token, NewDescription: "  \u3000 "}, wantErr: ErrRepairDescriptionEmpty},
		{name: "description too long", params: CreateRepairJobParams{AuxiliaryLabelID: active.ID, IdempotencyKey: "g6", ExpectedVersion: token, NewDescription: strings.Repeat("字", 501)}, wantErr: ErrRepairDescriptionTooLong},
		{name: "description repeats label", params: CreateRepairJobParams{AuxiliaryLabelID: active.ID, IdempotencyKey: "g7", ExpectedVersion: token, NewDescription: "活跃"}, wantErr: ErrRepairDescriptionRepeatsLabel},
		{name: "idempotency key missing", params: CreateRepairJobParams{AuxiliaryLabelID: active.ID, ExpectedVersion: token, NewDescription: "d"}, wantErr: ErrRepairIdempotencyKeyEmpty},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := repo.CreateRepairJob(context.Background(), tc.params)
			require.ErrorIs(t, err, tc.wantErr)
		})
	}
	var jobs, items int64
	require.NoError(t, db.Model(&tagmodels.AuxiliaryDescriptionRepairJob{}).Count(&jobs).Error)
	require.NoError(t, db.Model(&tagmodels.AuxiliaryDescriptionRepairItem{}).Count(&items).Error)
	require.Equal(t, int64(0), jobs)
	require.Equal(t, int64(0), items)
}

// ── idempotency ─────────────────────────────────────────────────────────────

// TestAuxiliaryDescriptionRepairIdempotency: same key + same payload returns
// the stored job (no second row); the replay is judged by the STORED request —
// even after the source label has since changed; same key + different payload
// is an explicit conflict (spec「双击与请求重试不重复执行」).
func TestAuxiliaryDescriptionRepairIdempotency(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repo := NewAuxiliaryDescriptionRepairRepository(db)
	seedRepairTag(t, db, "幂等标签", "active")

	label := seedRepairLabel(t, db, "幂等", "active", "old")
	labelCtx, err := repo.GetRepairContext(context.Background(), label.ID)
	require.NoError(t, err)
	original := CreateRepairJobParams{
		AuxiliaryLabelID: label.ID,
		IdempotencyKey:   "idem-1",
		ExpectedVersion:  labelCtx.Version,
		NewDescription:   "第一次确认的描述",
	}
	first, err := repo.CreateRepairJob(context.Background(), original)
	require.NoError(t, err)

	// Byte-identical retry of the same request → the very same job.
	again, err := repo.CreateRepairJob(context.Background(), original)
	require.NoError(t, err)
	require.Equal(t, first.ID, again.ID)
	var jobs int64
	require.NoError(t, db.Model(&tagmodels.AuxiliaryDescriptionRepairJob{}).Count(&jobs).Error)
	require.Equal(t, int64(1), jobs)

	// Same key, different description → explicit conflict, not a replay.
	_, err = repo.CreateRepairJob(context.Background(), CreateRepairJobParams{
		AuxiliaryLabelID: label.ID, IdempotencyKey: "idem-1",
		ExpectedVersion: original.ExpectedVersion, NewDescription: "改了主意的描述",
	})
	require.ErrorIs(t, err, ErrRepairIdempotencyConflict)

	// Same key, different expected_version → also a different payload (a
	// deliberately stale token, so only the version component differs).
	_, err = repo.CreateRepairJob(context.Background(), CreateRepairJobParams{
		AuxiliaryLabelID: label.ID, IdempotencyKey: "idem-1",
		ExpectedVersion: "v1_stale_but_wellformed_token_0123456789ab", NewDescription: "第一次确认的描述",
	})
	require.ErrorIs(t, err, ErrRepairIdempotencyConflict)

	// Replay is judged by the STORED request: change the label AFTER the job
	// exists (bumps updated_at → new token), then re-send the ORIGINAL request
	// → still the same job, no version conflict, no second row.
	require.NoError(t, db.Model(&models.SemanticLabel{}).Where("id = ?", label.ID).
		Update("description", "被别人改过的描述").Error)
	replay, err := repo.CreateRepairJob(context.Background(), original)
	require.NoError(t, err, "stored-request replay must win over the current label state")
	require.Equal(t, first.ID, replay.ID)

	// Disable the label afterwards: the same stored request still replays
	// (idempotency precedes the active-status gate); a NEW key is rejected.
	require.NoError(t, db.Model(&models.SemanticLabel{}).Where("id = ?", label.ID).
		Update("status", "disabled").Error)
	replay2, err := repo.CreateRepairJob(context.Background(), original)
	require.NoError(t, err)
	require.Equal(t, first.ID, replay2.ID)
	_, err = repo.CreateRepairJob(context.Background(), CreateRepairJobParams{
		AuxiliaryLabelID: label.ID, IdempotencyKey: "idem-2",
		ExpectedVersion: original.ExpectedVersion, NewDescription: "停用后的新请求",
	})
	require.ErrorIs(t, err, ErrRepairLabelNotActive)
	require.NoError(t, db.Model(&tagmodels.AuxiliaryDescriptionRepairJob{}).Count(&jobs).Error)
	require.Equal(t, int64(1), jobs)
}

// ── per-label mutex ─────────────────────────────────────────────────────────

// TestAuxiliaryDescriptionRepairMutexLifecycle: an unfinished job (including
// retryable failed stages) blocks new jobs for the same label; terminal
// statuses release the mutex.
func TestAuxiliaryDescriptionRepairMutexLifecycle(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repo := NewAuxiliaryDescriptionRepairRepository(db)
	label := seedRepairLabel(t, db, "互斥", "active", "old")
	seedRepairTag(t, db, "互斥标签", "active")

	first, err := createRepairJobViaCtx(t, repo, label.ID, "mtx-1", "第一次")
	require.NoError(t, err)

	_, err = createRepairJobViaCtx(t, repo, label.ID, "mtx-2", "第二次")
	require.ErrorIs(t, err, ErrRepairJobAlreadyActive)

	// Retryable failed stage still holds the mutex.
	require.NoError(t, db.Model(&tagmodels.AuxiliaryDescriptionRepairJob{}).Where("id = ?", first.ID).
		Updates(map[string]any{"status": "matching_failed", "stage": "matching"}).Error)
	_, err = createRepairJobViaCtx(t, repo, label.ID, "mtx-2", "第二次")
	require.ErrorIs(t, err, ErrRepairJobAlreadyActive)

	// Terminal statuses release it.
	for _, terminal := range tagmodels.AuxiliaryDescriptionRepairTerminalStatuses {
		require.NoError(t, db.Model(&tagmodels.AuxiliaryDescriptionRepairJob{}).Where("id = ?", first.ID).
			Update("status", terminal).Error)
		next, err := createRepairJobViaCtx(t, repo, label.ID, "mtx-after-"+terminal, terminal+"后的新任务")
		require.NoError(t, err, "terminal status %s must release the mutex", terminal)
		require.NoError(t, db.Model(&tagmodels.AuxiliaryDescriptionRepairJob{}).Where("id = ?", next.ID).
			Update("status", terminal).Error)
	}
}

// TestAuxiliaryDescriptionRepairConcurrentSameLabel: two concurrent creates,
// same label, different keys — the FOR UPDATE serialization plus the partial
// unique index guarantee exactly one job survives.
func TestAuxiliaryDescriptionRepairConcurrentSameLabel(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repo := NewAuxiliaryDescriptionRepairRepository(db)
	label := seedRepairLabel(t, db, "并发互斥", "active", "old")
	seedRepairTag(t, db, "并发标签", "active")

	ctxLabel, err := repo.GetRepairContext(context.Background(), label.ID)
	require.NoError(t, err)

	var wg sync.WaitGroup
	results := make([]error, 2)
	wg.Add(2)
	for i := 0; i < 2; i++ {
		go func(i int) {
			defer wg.Done()
			_, err := repo.CreateRepairJob(context.Background(), CreateRepairJobParams{
				AuxiliaryLabelID: label.ID,
				IdempotencyKey:   fmt.Sprintf("conc-mtx-%d", i),
				ExpectedVersion:  ctxLabel.Version,
				NewDescription:   fmt.Sprintf("并发描述 %d", i),
			})
			results[i] = err
		}(i)
	}
	wg.Wait()

	var success, mutexErr int
	for _, err := range results {
		switch {
		case err == nil:
			success++
		case errors.Is(err, ErrRepairJobAlreadyActive):
			mutexErr++
		default:
			t.Fatalf("unexpected concurrent create error: %v", err)
		}
	}
	require.Equal(t, 1, success)
	require.Equal(t, 1, mutexErr, "the loser must report the per-label mutex, not a raw DB error")
	var jobs int64
	require.NoError(t, db.Model(&tagmodels.AuxiliaryDescriptionRepairJob{}).Count(&jobs).Error)
	require.Equal(t, int64(1), jobs)
}

// TestAuxiliaryDescriptionRepairConcurrentCrossLabelSameKey: the same
// idempotency key racing on two DIFFERENT labels cannot be serialized by the
// label lock — the loser hits the key unique index (23505) and must resolve
// to an explicit payload conflict. This exercises the unique-violation
// resolution path end to end.
func TestAuxiliaryDescriptionRepairConcurrentCrossLabelSameKey(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repo := NewAuxiliaryDescriptionRepairRepository(db)
	labelA := seedRepairLabel(t, db, "跨标签A", "active", "oldA")
	labelB := seedRepairLabel(t, db, "跨标签B", "active", "oldB")
	seedRepairTag(t, db, "跨标签竞争", "active")

	ctxA, err := repo.GetRepairContext(context.Background(), labelA.ID)
	require.NoError(t, err)
	ctxB, err := repo.GetRepairContext(context.Background(), labelB.ID)
	require.NoError(t, err)

	var wg sync.WaitGroup
	results := make([]error, 2)
	wg.Add(2)
	for i := 0; i < 2; i++ {
		go func(i int) {
			defer wg.Done()
			params := CreateRepairJobParams{
				IdempotencyKey: "shared-key",
				NewDescription: "竞争描述",
			}
			if i == 0 {
				params.AuxiliaryLabelID, params.ExpectedVersion = labelA.ID, ctxA.Version
			} else {
				params.AuxiliaryLabelID, params.ExpectedVersion = labelB.ID, ctxB.Version
			}
			_, err := repo.CreateRepairJob(context.Background(), params)
			results[i] = err
		}(i)
	}
	wg.Wait()

	var success, conflict int
	for _, err := range results {
		switch {
		case err == nil:
			success++
		case errors.Is(err, ErrRepairIdempotencyConflict):
			conflict++
		default:
			t.Fatalf("unexpected concurrent create error: %v", err)
		}
	}
	require.Equal(t, 1, success)
	require.Equal(t, 1, conflict, "loser of the key race must resolve 23505 into an explicit conflict")
	var jobs int64
	require.NoError(t, db.Model(&tagmodels.AuxiliaryDescriptionRepairJob{}).Count(&jobs).Error)
	require.Equal(t, int64(1), jobs)
}

// ── transaction atomicity ───────────────────────────────────────────────────

// TestAuxiliaryDescriptionRepairTxRollbackNoPartialJob: a failure while
// persisting the frozen items (after the job row was already inserted inside
// the transaction) rolls the whole job back — no half task survives. The
// fault is injected with a CHECK constraint on the items table (testcontainer
// only; dropped in cleanup).
func TestAuxiliaryDescriptionRepairTxRollbackNoPartialJob(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repo := NewAuxiliaryDescriptionRepairRepository(db)
	label := seedRepairLabel(t, db, "回滚", "active", "old")
	victim := seedRepairTag(t, db, "回滚受害标签", "active")
	seedRepairTag(t, db, "回滚旁观标签", "active")

	const chk = "aux_repair_item_inject_chk"
	require.NoError(t, db.Exec(fmt.Sprintf(
		`ALTER TABLE auxiliary_description_repair_items ADD CONSTRAINT %s CHECK (topic_tag_id <> %d)`, chk, victim.ID)).Error)
	t.Cleanup(func() {
		_ = db.Exec(fmt.Sprintf(`ALTER TABLE auxiliary_description_repair_items DROP CONSTRAINT IF EXISTS %s`, chk)).Error
	})

	_, err := createRepairJobViaCtx(t, repo, label.ID, "rollback-1", "回滚场景描述")
	require.Error(t, err, "the item-batch insert must fail against the injected CHECK constraint")

	var jobs, items int64
	require.NoError(t, db.Model(&tagmodels.AuxiliaryDescriptionRepairJob{}).Where("idempotency_key = ?", "rollback-1").Count(&jobs).Error)
	require.NoError(t, db.Model(&tagmodels.AuxiliaryDescriptionRepairItem{}).Count(&items).Error)
	require.Equal(t, int64(0), jobs, "job row inserted inside the tx must vanish on rollback")
	require.Equal(t, int64(0), items)

	// After the fault is gone the same request creates cleanly.
	require.NoError(t, db.Exec(fmt.Sprintf(`ALTER TABLE auxiliary_description_repair_items DROP CONSTRAINT %s`, chk)).Error)
	job, err := createRepairJobViaCtx(t, repo, label.ID, "rollback-1", "回滚场景描述")
	require.NoError(t, err)
	require.Equal(t, 2, job.Total)
	require.NoError(t, db.Model(&tagmodels.AuxiliaryDescriptionRepairItem{}).Where("job_id = ?", job.ID).Count(&items).Error)
	require.Equal(t, int64(2), items)
}

// ── deletion non-blocking & history ─────────────────────────────────────────

// TestAuxiliaryDescriptionRepairDeletionNotBlocked: neither deleting a frozen
// topic_tag nor deleting the auxiliary label is blocked by the repair tables,
// and the frozen IDs stay on the item/job rows (no FK, no cascade, no lost
// history) — later seams record them as no longer applicable.
func TestAuxiliaryDescriptionRepairDeletionNotBlocked(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repo := NewAuxiliaryDescriptionRepairRepository(db)
	label := seedRepairLabel(t, db, "删除", "active", "old")
	tag := seedRepairTag(t, db, "待删标签", "active")

	job, err := createRepairJobViaCtx(t, repo, label.ID, "del-1", "删除场景描述")
	require.NoError(t, err)
	require.Equal(t, 1, job.Total)

	// Tag deletion must not be blocked by the frozen item row…
	require.NoError(t, db.Delete(&models.TopicTag{}, tag.ID).Error)
	// …and the frozen ID survives on the item (history kept, scope count kept).
	var item tagmodels.AuxiliaryDescriptionRepairItem
	require.NoError(t, db.Where("job_id = ? AND topic_tag_id = ?", job.ID, tag.ID).First(&item).Error)
	require.Equal(t, tag.ID, item.TopicTagID)
	var jobAfter tagmodels.AuxiliaryDescriptionRepairJob
	require.NoError(t, db.Where("id = ?", job.ID).First(&jobAfter).Error)
	require.Equal(t, 1, jobAfter.Total)

	// Label deletion (aux GC delete path) is not blocked either; job history kept.
	require.NoError(t, db.Delete(&models.SemanticLabel{}, label.ID).Error)
	require.NoError(t, db.Where("id = ?", job.ID).First(&jobAfter).Error)
	require.Equal(t, label.ID, jobAfter.AuxiliaryLabelID)

	// Queries still work; the label-scoped read now reports not-found.
	_, err = repo.GetRepairContext(context.Background(), label.ID)
	require.ErrorIs(t, err, ErrRepairLabelNotFound)
}

// ── queries & context reads ─────────────────────────────────────────────────

// TestAuxiliaryDescriptionRepairQueries: ownership-checked job fetch, latest
// task lookup, persistence across repository instances (no in-memory state),
// and the zero-write repair context (live version token, direct boards incl.
// disabled, active-tag count matching the frozen-scope predicate, latest
// task, no task created by reads).
func TestAuxiliaryDescriptionRepairQueries(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repo := NewAuxiliaryDescriptionRepairRepository(db)

	label := seedRepairLabel(t, db, "查询", "active", "查询旧描述")
	activeBoard := seedRepairBoard(t, db, "引用它的版块", "active", label.ID)
	disabledBoard := seedRepairBoard(t, db, "停用引用版块", "disabled", label.ID)
	activeTag := seedRepairTag(t, db, "查询标签", "active")
	seedRepairTag(t, db, "查询合并标签", "merged")
	seedRepairLabel(t, db, "别的辅助标签", "active", "") // never referenced by boards

	before := snapshotRepairLabel(t, db, label.ID)

	ctxData, err := repo.GetRepairContext(context.Background(), label.ID)
	require.NoError(t, err)
	require.Equal(t, AuxiliaryVersionToken(&ctxData.Label), ctxData.Version)
	require.Equal(t, "查询", ctxData.Label.Label)
	require.Equal(t, "查询旧描述", ctxData.Label.Description)
	require.Equal(t, "active", ctxData.Label.Status)
	require.Len(t, ctxData.DirectBoards, 2)
	require.Equal(t, activeBoard.ID, ctxData.DirectBoards[0].ID)
	require.Equal(t, "active", ctxData.DirectBoards[0].Status)
	require.Equal(t, disabledBoard.ID, ctxData.DirectBoards[1].ID)
	require.Equal(t, "disabled", ctxData.DirectBoards[1].Status, "disabled direct boards stay visible with their status")
	require.Equal(t, int64(1), ctxData.ActiveTagCount, "count = active population only (merged excluded)")
	require.Nil(t, ctxData.LatestTask, "no task exists yet — reads must not create one")

	// Zero-write proof: the label row is byte-identical after the reads.
	requireRepairLabelUnchanged(t, db, label.ID, before)
	var jobsAfterRead int64
	require.NoError(t, db.Model(&tagmodels.AuxiliaryDescriptionRepairJob{}).Count(&jobsAfterRead).Error)
	require.Equal(t, int64(0), jobsAfterRead, "GET/context reads must not create jobs")

	job, err := createRepairJobViaCtx(t, repo, label.ID, "q-1", "查询场景新描述")
	require.NoError(t, err)

	// Latest task + a fresh repository instance (restart surrogate): the job
	// must still be found — persistence lives in Postgres, not a process map.
	freshRepo := NewAuxiliaryDescriptionRepairRepository(db)
	latest, err := freshRepo.GetLatestRepairJob(context.Background(), label.ID)
	require.NoError(t, err)
	require.Equal(t, job.ID, latest.ID)

	got, err := freshRepo.GetRepairJob(context.Background(), label.ID, job.ID)
	require.NoError(t, err)
	require.Equal(t, job.IdempotencyKey, got.IdempotencyKey)
	var scope []tagmodels.AuxiliaryDescriptionRepairItem
	require.NoError(t, db.Where("job_id = ?", job.ID).Find(&scope).Error)
	require.Len(t, scope, 1)
	require.Equal(t, activeTag.ID, scope[0].TopicTagID)

	// Ownership: the same job id under another label is not found.
	secondLabel := seedRepairLabel(t, db, "另一个标签", "active", "x")
	_, err = freshRepo.GetRepairJob(context.Background(), secondLabel.ID, job.ID)
	require.ErrorIs(t, err, gorm.ErrRecordNotFound, "cross-label job fetch must 404")

	// Latest-task ordering: after a second (terminal) job, latest is the newer one.
	require.NoError(t, db.Model(&tagmodels.AuxiliaryDescriptionRepairJob{}).Where("id = ?", job.ID).
		Update("status", "completed").Error)
	second, err := createRepairJobViaCtx(t, repo, label.ID, "q-2", "第二个任务描述")
	require.NoError(t, err)
	latest, err = freshRepo.GetLatestRepairJob(context.Background(), label.ID)
	require.NoError(t, err)
	require.Equal(t, second.ID, latest.ID)

	// Context now surfaces the latest task.
	ctxData, err = repo.GetRepairContext(context.Background(), label.ID)
	require.NoError(t, err)
	require.NotNil(t, ctxData.LatestTask)
	require.Equal(t, second.ID, ctxData.LatestTask.ID)
	requireRepairLabelUnchanged(t, db, label.ID, before)
}
