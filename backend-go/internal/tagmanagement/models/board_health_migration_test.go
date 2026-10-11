package models_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/testutil"
	tagmodels "syntopica-backend/internal/tagmanagement/models"
)

// TestBoardHealthSnapshotMigration verifies migration 20261006_0001
// (add-board-health-checkup tasks 1.1) via the golden schema, which runs the
// production first-run order (AutoMigrate creates the table, then versioned
// migrations add the FK semantic_board_id → semantic_labels(id) ON DELETE
// CASCADE). Docker required; skipped under -short.
func TestBoardHealthSnapshotMigration(t *testing.T) {
	db := testutil.SetupTestDB(t)

	// 1. The migration ran and added the FK.
	var fkExists bool
	require.NoError(t, db.Raw(`SELECT EXISTS (SELECT 1 FROM information_schema.table_constraints
		WHERE constraint_name='fk_board_health_snapshots_board' AND table_name='board_health_snapshots')`).Scan(&fkExists).Error)
	require.True(t, fkExists, "migration 20261006_0001 must add fk_board_health_snapshots_board")

	// 2. CASCADE: deleting the board removes its snapshots.
	board := models.SemanticLabel{Label: "迁移探针板", Slug: "migration-probe-board", LabelType: "board", Status: "active"}
	require.NoError(t, db.Create(&board).Error)
	score := 77.0
	comment := "构成清晰"
	require.NoError(t, db.Create(&tagmodels.BoardHealthSnapshot{
		SemanticBoardID: board.ID, CheckedAt: time.Now(),
		Metrics:              tagmodels.BoardHealthMetrics{Board: tagmodels.BoardHealthBoardMetrics{ReasonDist: map[string]int{}}, Labels: []tagmodels.BoardHealthLabelMetrics{}},
		UncapturedCandidates: []tagmodels.BoardHealthCandidate{},
		LLMScore:             &score, LLMComment: &comment,
	}).Error)
	require.NoError(t, db.Delete(&models.SemanticLabel{}, board.ID).Error)
	var left int64
	require.NoError(t, db.Model(&tagmodels.BoardHealthSnapshot{}).Where("semantic_board_id = ?", board.ID).Count(&left).Error)
	require.Zero(t, left, "ON DELETE CASCADE removes snapshots with the board")

	// 3. jsonb serializer round-trip on the golden schema.
	board2 := models.SemanticLabel{Label: "序列化探针板", Slug: "serializer-probe-board", LabelType: "board", Status: "active"}
	require.NoError(t, db.Create(&board2).Error)
	snapshot := tagmodels.BoardHealthSnapshot{
		SemanticBoardID: board2.ID, CheckedAt: time.Now(),
		Metrics: tagmodels.BoardHealthMetrics{
			Board: tagmodels.BoardHealthBoardMetrics{
				MatchedTags: 3,
				ReasonDist: map[string]int{"direct_hit": 3}, GhostMatches: 1, VectorMissing: 0,
			},
			Labels: []tagmodels.BoardHealthLabelMetrics{{AuxID: 9, Label: "推理", Samples: 3, Insufficient: true}},
		},
		UncapturedCandidates: []tagmodels.BoardHealthCandidate{{AuxID: 10, Label: "算力", MemberTags: 4, MemberArticles: 2}},
		LLMScore:             &score, LLMComment: &comment,
	}
	require.NoError(t, db.Create(&snapshot).Error)
	var loaded tagmodels.BoardHealthSnapshot
	require.NoError(t, db.Where("semantic_board_id = ?", board2.ID).First(&loaded).Error)
	require.Equal(t, 3, loaded.Metrics.Board.MatchedTags)
	require.Equal(t, map[string]int{"direct_hit": 3}, loaded.Metrics.Board.ReasonDist)
	require.Equal(t, 1, loaded.Metrics.Board.GhostMatches)
	require.Len(t, loaded.Metrics.Labels, 1)
	require.True(t, loaded.Metrics.Labels[0].Insufficient)
	require.Nil(t, loaded.Metrics.Labels[0].CosineMean, "insufficient label serializes null distribution")
	require.Len(t, loaded.UncapturedCandidates, 1)
	require.Equal(t, 4, loaded.UncapturedCandidates[0].MemberTags)
	require.InDelta(t, 77.0, *loaded.LLMScore, 1e-9)
	require.Equal(t, "构成清晰", *loaded.LLMComment)

	_ = gorm.ErrRecordNotFound // gorm import shape guard
}
