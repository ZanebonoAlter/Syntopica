package repository

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/testutil"
)

// TestBoardUpgradeSuggestionCountDismissedInCooldown verifies the cooldown
// rule for dismissed suggestions (spec: dismissed 冷却期): a suggestion
// dismissed within the cooldown window blocks re-generation; one past the
// window allows re-generation.
//
// §3.3: 3-day-old dismissal blocks (within 14d); 15-day-old allows.
func TestBoardUpgradeSuggestionCountDismissedInCooldown(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repo := NewBoardUpgradeSuggestionRepository(db)
	now := time.Now()

	threeDaysAgo := now.AddDate(0, 0, -3)
	fifteenDaysAgo := now.AddDate(0, 0, -15)

	// Dismissed 3 days ago → still inside the default 14-day cooldown.
	require.NoError(t, db.Create(&models.BoardUpgradeSuggestion{
		BatchID: "c-in", Mode: "discover_new", Decision: "create_new",
		BoardLabel: "In Cooldown", SuggestionHash: "cd-in",
		Status: "dismissed", ResolvedAt: &threeDaysAgo,
	}).Error)
	// Dismissed 15 days ago → cooldown window (14d) has elapsed.
	require.NoError(t, db.Create(&models.BoardUpgradeSuggestion{
		BatchID: "c-out", Mode: "discover_new", Decision: "create_new",
		BoardLabel: "Expired", SuggestionHash: "cd-out",
		Status: "dismissed", ResolvedAt: &fifteenDaysAgo,
	}).Error)

	blocked, err := repo.CountDismissedInCooldown(context.Background(), "cd-in", 14)
	require.NoError(t, err)
	require.Equal(t, int64(1), blocked, "dismissed 3 days ago is within the 14-day cooldown")

	expired, err := repo.CountDismissedInCooldown(context.Background(), "cd-out", 14)
	require.NoError(t, err)
	require.Equal(t, int64(0), expired, "dismissed 15 days ago is past the 14-day cooldown")
}

// TestBoardUpgradeSuggestionInsertPendingIsIdempotent verifies the partial
// unique index uq_board_upgrade_suggestions_hash makes a second insert with the
// same pending hash a no-op (spec: 建议生成幂等).
//
// §3.2: same cluster + same decision re-generated → original kept, no duplicate.
func TestBoardUpgradeSuggestionInsertPendingIsIdempotent(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repo := NewBoardUpgradeSuggestionRepository(db)

	first := &models.BoardUpgradeSuggestion{
		BatchID: "b2", Mode: "discover_new", Decision: "create_new",
		BoardLabel: "Dup Board", AuxiliaryLabelIDs: []uint{7},
		Confidence: "llm", Status: "pending", SuggestionHash: "dup-hash-3-2",
	}
	inserted1, err := repo.InsertPending(context.Background(), first)
	require.NoError(t, err)
	require.True(t, inserted1, "first insert must succeed")

	// Re-generate the same cluster+decision → same suggestion_hash, still pending.
	repeated := &models.BoardUpgradeSuggestion{
		BatchID: "b2-later", Mode: "discover_new", Decision: "create_new",
		BoardLabel: "Dup Board", AuxiliaryLabelIDs: []uint{7},
		Confidence: "llm", Status: "pending", SuggestionHash: "dup-hash-3-2",
	}
	inserted2, err := repo.InsertPending(context.Background(), repeated)
	require.NoError(t, err, "duplicate pending hash must be a no-op, not an error")
	require.False(t, inserted2, "duplicate must report inserted=false")

	var count int64
	require.NoError(t, db.Model(&models.BoardUpgradeSuggestion{}).Where("suggestion_hash = ?", "dup-hash-3-2").Count(&count).Error)
	require.Equal(t, int64(1), count, "only one pending row for the hash may exist")
}

// TestBoardUpgradeSuggestionInsertPendingPersistsRows verifies InsertPending
// stores each suggestion as a pending row (spec: 升级建议持久化存储).
//
// §3.1: 3 non-skip suggestions → 3 pending rows persisted.
func TestBoardUpgradeSuggestionInsertPendingPersistsRows(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repo := NewBoardUpgradeSuggestionRepository(db)

	base := time.Now()
	sugs := []*models.BoardUpgradeSuggestion{
		{BatchID: "b1", Mode: "discover_new", Decision: "create_new", BoardLabel: "Board A", Description: "d A", AuxiliaryLabelIDs: []uint{101}, Confidence: "llm", Status: "pending", SuggestionHash: "hash-3-1-a", CreatedAt: base},
		{BatchID: "b1", Mode: "discover_new", Decision: "create_new", BoardLabel: "Board B", Description: "d B", AuxiliaryLabelIDs: []uint{102}, Confidence: "llm", Status: "pending", SuggestionHash: "hash-3-1-b", CreatedAt: base},
		{BatchID: "b1", Mode: "discover_new", Decision: "create_new", BoardLabel: "Board C", Description: "d C", AuxiliaryLabelIDs: []uint{103}, Confidence: "llm", Status: "pending", SuggestionHash: "hash-3-1-c", CreatedAt: base},
	}
	for _, s := range sugs {
		inserted, err := repo.InsertPending(context.Background(), s)
		require.NoError(t, err)
		require.True(t, inserted, "distinct-hash insert must succeed")
	}

	var rows []models.BoardUpgradeSuggestion
	require.NoError(t, db.Order("id ASC").Find(&rows).Error)
	require.Len(t, rows, 3, "three non-skip suggestions must be persisted")
	for _, r := range rows {
		require.Equal(t, "pending", r.Status, "new suggestions start as pending")
		require.Equal(t, "discover_new", r.Mode)
		require.Equal(t, "create_new", r.Decision)
	}
}

// TestBoardUpgradeSuggestionListFilters verifies the query API read path (spec:
// 建议查询 API 读持久化表).
//
// §5.1: default = pending + non-watch, high-confidence first then created desc;
// decision=watch → observation pool; status/decision exact filters honored.
func TestBoardUpgradeSuggestionListFilters(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repo := NewBoardUpgradeSuggestionRepository(db)
	now := time.Now()

	mk := func(hash, status, decision, confidence string, age time.Duration) *models.BoardUpgradeSuggestion {
		return &models.BoardUpgradeSuggestion{
			BatchID: "t", Mode: "discover_new", Decision: decision,
			BoardLabel: "L-" + hash, AuxiliaryLabelIDs: []uint{1},
			Confidence: confidence, Status: status, SuggestionHash: hash,
			CreatedAt: now.Add(-age),
		}
	}

	// pending non-watch: high(old), llm(newer), merge(oldest)
	require.NoError(t, db.Create(mk("h1-high", "pending", "create_new", "high", 2*time.Hour)).Error)
	require.NoError(t, db.Create(mk("h2-llm", "pending", "create_new", "llm", 1*time.Hour)).Error)
	require.NoError(t, db.Create(mk("h3-merge", "pending", "merge_into_existing", "llm", 3*time.Hour)).Error)
	// pending watch — excluded from default list
	require.NoError(t, db.Create(mk("hw", "pending", "watch", "llm", 1*time.Hour)).Error)
	// confirmed create_new — excluded when status=pending
	require.NoError(t, db.Create(mk("hc", "confirmed", "create_new", "high", 1*time.Hour)).Error)

	// default list: pending + non-watch, high first, then created desc among llm.
	rows, err := repo.List(context.Background(), "pending", "")
	require.NoError(t, err)
	require.Len(t, rows, 3)
	require.Equal(t, "h1-high", rows[0].SuggestionHash, "high confidence sorts first")
	require.Equal(t, "h2-llm", rows[1].SuggestionHash, "llm: newer before older")
	require.Equal(t, "h3-merge", rows[2].SuggestionHash)

	// observation pool
	watchRows, err := repo.List(context.Background(), "pending", "watch")
	require.NoError(t, err)
	require.Len(t, watchRows, 1)
	require.Equal(t, "hw", watchRows[0].SuggestionHash)

	// status filter with default (non-watch) decision exclusion
	confirmedRows, err := repo.List(context.Background(), "confirmed", "")
	require.NoError(t, err)
	require.Len(t, confirmedRows, 1)
	require.Equal(t, "hc", confirmedRows[0].SuggestionHash)

	// exact decision filter among pending
	cnRows, err := repo.List(context.Background(), "pending", "create_new")
	require.NoError(t, err)
	require.Len(t, cnRows, 2)
}

// TestBoardUpgradeSuggestionMarkDismissed verifies the dismiss lifecycle (spec:
// 建议 dismiss 与 confirm 联动): pending → dismissed + resolved_at + reason.
//
// §5.2.
func TestBoardUpgradeSuggestionMarkDismissed(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repo := NewBoardUpgradeSuggestionRepository(db)

	require.NoError(t, db.Create(&models.BoardUpgradeSuggestion{
		BatchID: "d", Mode: "discover_new", Decision: "create_new",
		BoardLabel: "Dismiss Me", AuxiliaryLabelIDs: []uint{9},
		Confidence: "llm", Status: "pending", SuggestionHash: "d-1",
	}).Error)
	var sug models.BoardUpgradeSuggestion
	require.NoError(t, db.Where("suggestion_hash = ?", "d-1").First(&sug).Error)

	require.NoError(t, repo.MarkDismissed(context.Background(), sug.ID, "not now"))
	require.NoError(t, db.Where("id = ?", sug.ID).First(&sug).Error)
	require.Equal(t, "dismissed", sug.Status)
	require.NotNil(t, sug.ResolvedAt)
	require.NotNil(t, sug.DismissReason)
	require.Equal(t, "not now", *sug.DismissReason)
	require.NotNil(t, sug.ResolvedBy)
	require.Equal(t, "manual", *sug.ResolvedBy)

	// double-dismiss on an already-resolved row is a no-op (no error, no change)
	require.NoError(t, repo.MarkDismissed(context.Background(), sug.ID, "again"))
}
