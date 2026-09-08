package repository_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"syntopica-backend/internal/dataenrichment/repository"
	"syntopica-backend/internal/platform/database"
	"syntopica-backend/internal/platform/testutil"
)

// setupCrossBoardRelationDB opens a migrated testcontainer Postgres and
// registers the relation models. Rows created here use distinctive hashes so
// cleanup never fights other tests.
func setupCrossBoardRelationDB(t *testing.T) *repository.Repository {
	t.Helper()
	if testing.Short() {
		t.Skip("integration test: requires testcontainer Postgres")
	}
	db := testutil.OpenTestDB(t)
	require.NoError(t, db.Exec("CREATE EXTENSION IF NOT EXISTS vector").Error)
	require.NoError(t, database.RunAutoMigrate(db))
	// Versioned migrations own the partial unique index + CHECKs the relation
	// lifecycle depends on (idempotent ON CONFLICT insert).
	require.NoError(t, database.RunMigrations(db))
	t.Cleanup(func() {
		_ = db.Exec(`DELETE FROM cross_board_relations WHERE suggestion_hash LIKE 'cbr-test-%'`).Error
		_ = db.Exec(`DELETE FROM cross_board_relation_runs WHERE source_key LIKE 'cbr-test-run-%'`).Error
		_ = db.Exec(`DELETE FROM semantic_labels WHERE label LIKE 'cbr-test-board-%'`).Error
	})
	return repository.NewRepository(db)
}

// cbrTestBoard inserts a board-type semantic label and returns its id.
func cbrTestBoard(t *testing.T, db *gorm.DB, label string) uint {
	t.Helper()
	var id uint
	err := db.Raw(`INSERT INTO semantic_labels (label, slug, label_type, created_at, updated_at)
		VALUES (?, ?, 'board', now(), now()) RETURNING id`, label, label+"-slug").Scan(&id).Error
	require.NoError(t, err)
	require.NotZero(t, id)
	return id
}

// cbrTestRelation builds a minimal valid relation row with a unique hash.
func cbrTestRelation(sourceBoard uint, targetBoard *uint, status string) *repository.CrossBoardRelation {
	return &repository.CrossBoardRelation{
		SourceBoardID:       sourceBoard,
		TargetBoardID:       targetBoard,
		TargetConcept:       "测试概念",
		RelationType:        repository.RelationTypeCausal,
		Claim:               "A 与 B 存在传导关系",
		VerificationVerdict: repository.RelationVerdictSupported,
		QualityGrade:        repository.RelationGradeMedium,
		Status:              status,
		SuggestionHash:      "cbr-test-" + time.Now().Format("150405.000000000") + "-" + status,
		EvidenceVersion:     "v1",
	}
}

func TestCrossBoardRelationHashStability(t *testing.T) {
	target := uint(4242)
	h1 := repository.ComputeRelationHash(7, "observation", "o1", &target, "概念", "causal", "claim  text", "v1")
	h2 := repository.ComputeRelationHash(7, "observation", "o1", &target, "概念", "causal", "claim\t text", "v1")
	require.Equal(t, h1, h2, "whitespace folding must not change the hash")
	require.Len(t, h1, 64)

	h3 := repository.ComputeRelationHash(7, "observation", "o1", nil, "不同概念", "causal", "claim  text", "v1")
	require.NotEqual(t, h1, h3, "unresolved concept must hash differently from resolved board")
	h4 := repository.ComputeRelationHash(7, "observation", "o1", &target, "概念", "causal", "claim  text", "v2")
	require.NotEqual(t, h1, h4, "evidence version must participate")
}

func TestCrossBoardRelationInsertIdempotent(t *testing.T) {
	repo := setupCrossBoardRelationDB(t)
	ctx := context.Background()
	sourceBoard := cbrTestBoard(t, repo.DB(), "cbr-test-board-src")

	rel := cbrTestRelation(sourceBoard, nil, repository.RelationStatusUnresolved)
	rel.SuggestionHash = "cbr-test-idem-1"
	inserted, err := repo.InsertOpenRelation(ctx, rel)
	require.NoError(t, err)
	require.True(t, inserted)

	dup := cbrTestRelation(sourceBoard, nil, repository.RelationStatusProposed)
	dup.SuggestionHash = "cbr-test-idem-1"
	dup.Claim = "different claim but same hash slot"
	inserted2, err := repo.InsertOpenRelation(ctx, dup)
	require.NoError(t, err)
	require.False(t, inserted2, "same open hash must be a no-op (idempotent discovery)")

	var count int64
	require.NoError(t, repo.DB().Model(&repository.CrossBoardRelation{}).
		Where("suggestion_hash = ?", "cbr-test-idem-1").Count(&count).Error)
	require.EqualValues(t, 1, count)
}

func TestCrossBoardRelationInsertRejectsClosedStatus(t *testing.T) {
	repo := setupCrossBoardRelationDB(t)
	ctx := context.Background()
	_, err := repo.InsertOpenRelation(ctx, cbrTestRelation(1, nil, repository.RelationStatusConfirmed))
	require.Error(t, err, "only open statuses may be inserted")
}

func TestCrossBoardRelationConfirmLifecycle(t *testing.T) {
	repo := setupCrossBoardRelationDB(t)
	ctx := context.Background()
	sourceBoard := cbrTestBoard(t, repo.DB(), "cbr-test-board-src")
	targetBoard := cbrTestBoard(t, repo.DB(), "cbr-test-board-tgt")
	target := targetBoard

	rel := cbrTestRelation(sourceBoard, &target, repository.RelationStatusProposed)
	rel.SuggestionHash = "cbr-test-confirm-1"
	_, err := repo.InsertOpenRelation(ctx, rel)
	require.NoError(t, err)
	var id uint
	require.NoError(t, repo.DB().Model(&repository.CrossBoardRelation{}).
		Where("suggestion_hash = ?", rel.SuggestionHash).Pluck("id", &id).Error)

	// Confirm with a 48h TTL.
	ttl := 48 * time.Hour
	require.NoError(t, repo.ConfirmCrossBoardRelation(ctx, id, "tester", ttl))

	got, err := repo.GetCrossBoardRelationByID(ctx, id)
	require.NoError(t, err)
	require.Equal(t, repository.RelationStatusConfirmed, got.Status)
	require.NotNil(t, got.ConfirmedAt)
	require.NotNil(t, got.ExpiresAt)
	require.WithinDuration(t, time.Now().Add(ttl), *got.ExpiresAt, time.Minute)
	require.Equal(t, "tester", *got.ResolvedBy)

	// Double confirm: idempotent conflict, no state corruption.
	err = repo.ConfirmCrossBoardRelation(ctx, id, "tester2", ttl)
	require.ErrorIs(t, err, repository.ErrRelationStateConflict)
	got2, _ := repo.GetCrossBoardRelationByID(ctx, id)
	require.Equal(t, "tester", *got2.ResolvedBy, "failed re-confirm must not overwrite audit fields")
}

func TestCrossBoardRelationConfirmInvalidStates(t *testing.T) {
	repo := setupCrossBoardRelationDB(t)
	ctx := context.Background()
	sourceBoard := cbrTestBoard(t, repo.DB(), "cbr-test-board-src")
	targetBoard := cbrTestBoard(t, repo.DB(), "cbr-test-board-tgt")
	target := targetBoard

	for name, status := range map[string]string{
		"unresolved": repository.RelationStatusUnresolved,
		"dismissed":  repository.RelationStatusDismissed,
		"expired":    repository.RelationStatusExpired,
	} {
		t.Run(name, func(t *testing.T) {
			// closed statuses cannot go through InsertOpenRelation; insert via raw DB.
			rel := cbrTestRelation(sourceBoard, &target, status)
			rel.SuggestionHash = "cbr-test-confirm-bad-" + name
			require.NoError(t, repo.DB().Create(rel).Error)
			require.NotZero(t, rel.ID)
			err := repo.ConfirmCrossBoardRelation(ctx, rel.ID, "tester", time.Hour)
			require.ErrorIs(t, err, repository.ErrRelationStateConflict)
		})
	}

	t.Run("unresolved-target-nil", func(t *testing.T) {
		rel := cbrTestRelation(sourceBoard, nil, repository.RelationStatusUnresolved)
		rel.SuggestionHash = "cbr-test-confirm-bad-nil-target"
		inserted, err := repo.InsertOpenRelation(ctx, rel)
		require.NoError(t, err)
		require.True(t, inserted)
		err = repo.ConfirmCrossBoardRelation(ctx, rel.ID, "tester", time.Hour)
		require.ErrorIs(t, err, repository.ErrRelationStateConflict)
	})
}

func TestCrossBoardRelationConfirmTargetVanished(t *testing.T) {
	repo := setupCrossBoardRelationDB(t)
	ctx := context.Background()
	sourceBoard := cbrTestBoard(t, repo.DB(), "cbr-test-board-src")
	ghost := uint(987654321)
	rel := cbrTestRelation(sourceBoard, &ghost, repository.RelationStatusProposed)
	rel.SuggestionHash = "cbr-test-confirm-ghost"
	inserted, err := repo.InsertOpenRelation(ctx, rel)
	require.NoError(t, err)
	require.True(t, inserted)
	err = repo.ConfirmCrossBoardRelation(ctx, rel.ID, "tester", time.Hour)
	require.ErrorIs(t, err, repository.ErrRelationStateConflict, "confirm must re-validate target board existence")
}

func TestCrossBoardRelationDismissCooldown(t *testing.T) {
	repo := setupCrossBoardRelationDB(t)
	ctx := context.Background()
	sourceBoard := cbrTestBoard(t, repo.DB(), "cbr-test-board-src")

	rel := cbrTestRelation(sourceBoard, nil, repository.RelationStatusProposed)
	rel.SuggestionHash = "cbr-test-dismiss-1"
	inserted, err := repo.InsertOpenRelation(ctx, rel)
	require.NoError(t, err)
	require.True(t, inserted)

	require.NoError(t, repo.DismissCrossBoardRelation(ctx, rel.ID, "噪音", "tester"))

	got, err := repo.GetCrossBoardRelationByID(ctx, rel.ID)
	require.NoError(t, err)
	require.Equal(t, repository.RelationStatusDismissed, got.Status)
	require.NotNil(t, got.DismissedAt)
	require.Equal(t, "噪音", *got.DismissReason)

	// Cooldown: dismissed within 14 days blocks re-discovery of the same hash.
	count, err := repo.CountDismissedRelationsInCooldown(ctx, rel.SuggestionHash, 14)
	require.NoError(t, err)
	require.EqualValues(t, 1, count)

	// Re-dismiss of a terminal row conflicts without mutation.
	err = repo.DismissCrossBoardRelation(ctx, rel.ID, "again", "tester")
	require.ErrorIs(t, err, repository.ErrRelationStateConflict)

	// Same hash may now insert a NEW open row (dismissed rows don't block inserts).
	again := cbrTestRelation(sourceBoard, nil, repository.RelationStatusUnresolved)
	again.SuggestionHash = "cbr-test-dismiss-1"
	inserted2, err := repo.InsertOpenRelation(ctx, again)
	require.NoError(t, err)
	require.True(t, inserted2, "partial unique index only covers open statuses")
}

func TestCrossBoardRelationDismissUnresolvedAllowed(t *testing.T) {
	repo := setupCrossBoardRelationDB(t)
	ctx := context.Background()
	sourceBoard := cbrTestBoard(t, repo.DB(), "cbr-test-board-src")
	rel := cbrTestRelation(sourceBoard, nil, repository.RelationStatusUnresolved)
	rel.SuggestionHash = "cbr-test-dismiss-unres-1"
	inserted, err := repo.InsertOpenRelation(ctx, rel)
	require.NoError(t, err)
	require.True(t, inserted)
	require.NoError(t, repo.DismissCrossBoardRelation(ctx, rel.ID, "无法解析的垃圾", "tester"))
	count, err := repo.CountDismissedRelationsInCooldown(ctx, rel.SuggestionHash, 14)
	require.NoError(t, err)
	require.EqualValues(t, 1, count)
}

func TestCrossBoardRelationActiveExpiryBoundary(t *testing.T) {
	repo := setupCrossBoardRelationDB(t)
	ctx := context.Background()
	sourceBoard := cbrTestBoard(t, repo.DB(), "cbr-test-board-src")
	targetBoard := cbrTestBoard(t, repo.DB(), "cbr-test-board-tgt")
	target := targetBoard

	mkConfirmed := func(hash string, expiresExpr string) uint {
		rel := cbrTestRelation(sourceBoard, &target, repository.RelationStatusProposed)
		rel.SuggestionHash = hash
		inserted, err := repo.InsertOpenRelation(ctx, rel)
		require.NoError(t, err)
		require.True(t, inserted)
		require.NoError(t, repo.ConfirmCrossBoardRelation(ctx, rel.ID, "tester", time.Hour))
		// Override expiry at the DB level to hit exact boundaries.
		require.NoError(t, repo.DB().Exec(
			`UPDATE cross_board_relations SET expires_at = `+expiresExpr+` WHERE id = ?`, rel.ID).Error)
		return rel.ID
	}

	past := mkConfirmed("cbr-test-exp-past", "NOW() - INTERVAL '1 hour'")
	eqNow := mkConfirmed("cbr-test-exp-eq", "NOW()")
	future := mkConfirmed("cbr-test-exp-future", "NOW() + INTERVAL '1 hour'")

	rows, err := repo.ListActiveConfirmedRelationsForBoard(ctx, sourceBoard)
	require.NoError(t, err)
	ids := map[uint]bool{}
	for _, r := range rows {
		ids[r.ID] = true
	}
	require.False(t, ids[past], "expired-at-past must be excluded")
	require.False(t, ids[eqNow], "expires_at == now is already stale at read time (time advanced)")
	require.True(t, ids[future], "future expiry must be active")
}

func TestCrossBoardRelationExpireConfirmedBatch(t *testing.T) {
	repo := setupCrossBoardRelationDB(t)
	ctx := context.Background()
	sourceBoard := cbrTestBoard(t, repo.DB(), "cbr-test-board-src")
	targetBoard := cbrTestBoard(t, repo.DB(), "cbr-test-board-tgt")
	target := targetBoard

	for _, hash := range []string{"cbr-test-batch-1", "cbr-test-batch-2"} {
		rel := cbrTestRelation(sourceBoard, &target, repository.RelationStatusProposed)
		rel.SuggestionHash = hash
		_, err := repo.InsertOpenRelation(ctx, rel)
		require.NoError(t, err)
		require.NoError(t, repo.ConfirmCrossBoardRelation(ctx, rel.ID, "tester", time.Hour))
		require.NoError(t, repo.DB().Exec(
			`UPDATE cross_board_relations SET expires_at = NOW() - INTERVAL '1 minute' WHERE suggestion_hash = ?`, hash).Error)
	}

	n, err := repo.ExpireConfirmedRelations(ctx)
	require.NoError(t, err)
	require.GreaterOrEqual(t, n, int64(2))

	// Idempotent: second run finds nothing from this batch.
	n2, err := repo.ExpireConfirmedRelations(ctx)
	require.NoError(t, err)

	got, err := repo.ListCrossBoardRelations(ctx, repository.CrossBoardRelationFilter{
		BoardID:  &sourceBoard,
		Statuses: []string{repository.RelationStatusExpired},
	})
	require.NoError(t, err)
	hashes := map[string]bool{}
	for _, r := range got {
		hashes[r.SuggestionHash] = true
	}
	require.True(t, hashes["cbr-test-batch-1"])
	require.True(t, hashes["cbr-test-batch-2"])
	_ = n2
}

func TestCrossBoardRelationListBoardFilter(t *testing.T) {
	repo := setupCrossBoardRelationDB(t)
	ctx := context.Background()
	boardA := cbrTestBoard(t, repo.DB(), "cbr-test-board-a")
	boardB := cbrTestBoard(t, repo.DB(), "cbr-test-board-b")
	target := boardB

	// A→B proposed and B-side unresolved.
	relAB := cbrTestRelation(boardA, &target, repository.RelationStatusProposed)
	relAB.SuggestionHash = "cbr-test-list-ab"
	_, err := repo.InsertOpenRelation(ctx, relAB)
	require.NoError(t, err)
	relUnres := cbrTestRelation(boardB, nil, repository.RelationStatusUnresolved)
	relUnres.SuggestionHash = "cbr-test-list-unres"
	_, err = repo.InsertOpenRelation(ctx, relUnres)
	require.NoError(t, err)

	rows, err := repo.ListCrossBoardRelations(ctx, repository.CrossBoardRelationFilter{
		BoardID:  &boardA,
		Statuses: []string{repository.RelationStatusProposed, repository.RelationStatusUnresolved},
	})
	require.NoError(t, err)
	hashes := map[string]bool{}
	for _, r := range rows {
		hashes[r.SuggestionHash] = true
	}
	require.True(t, hashes["cbr-test-list-ab"], "board A sees its outgoing relation")

	rowsB, err := repo.ListCrossBoardRelations(ctx, repository.CrossBoardRelationFilter{
		BoardID: &boardB,
	})
	require.NoError(t, err)
	hashesB := map[string]bool{}
	for _, r := range rowsB {
		hashesB[r.SuggestionHash] = true
	}
	require.True(t, hashesB["cbr-test-list-ab"], "board B sees the incoming relation (target side)")
	require.True(t, hashesB["cbr-test-list-unres"], "board B sees its unresolved rows")
}

func TestCrossBoardRelationRunCreateAndUpdate(t *testing.T) {
	repo := setupCrossBoardRelationDB(t)
	ctx := context.Background()
	run := &repository.CrossBoardRelationRun{
		SourceBoardID:  11,
		ParentResultID: 22,
		SourceKind:     repository.RelationSourceObservation,
		SourceKey:      "cbr-test-run-1",
		SourceText:     "观察文本",
		TriggerKind:    repository.RelationTriggerManual,
		Status:         repository.RelationRunStatusQueued,
	}
	require.NoError(t, repo.CreateRelationRun(ctx, run))
	require.NotZero(t, run.ID)

	require.NoError(t, repo.UpdateRelationRunStatus(ctx, run.ID, repository.RelationRunStatusFailed, "bocha 超时"))
	got, err := repo.GetRelationRunByID(ctx, run.ID)
	require.NoError(t, err)
	require.Equal(t, repository.RelationRunStatusFailed, got.Status)
	require.Equal(t, "bocha 超时", got.Error)
}
