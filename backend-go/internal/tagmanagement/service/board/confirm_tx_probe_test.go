package board

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/testutil"
	tagmodels "syntopica-backend/internal/tagmanagement/models"
	"syntopica-backend/internal/tagmanagement/repository"
	"syntopica-backend/internal/tagmanagement/service/auxlabel"
	"syntopica-backend/internal/tagmanagement/service/core"
)

// --- fix-aux-label-idle-tx-cache design D1：ConfirmSuggestion 事务内零 embedder ---

// 主链路步10（create_new 方向）：板向量在事务前预计算，事务闭包内纯写。
// 探针断言：embedder 的每次调用都发生在第一个 BEGIN 之前。
func TestConfirmSuggestionEmbedderRunsOutsideTransaction(t *testing.T) {
	db := setupSemanticBoardUpgradeTestDB(t)
	auxA := createUpgradeLabel(t, db, "探针甲", "probe-a", "auxiliary", "active", 5, []float64{1, 0, 0})
	auxB := createUpgradeLabel(t, db, "探针乙", "probe-b", "auxiliary", "active", 5, []float64{0, 1, 0})

	probeDB, probe := testutil.NewBeginProbeDB(t)
	embedder := func(ctx context.Context, input string, mode auxlabel.AuxiliaryLabelEmbeddingMode) (string, []float64, error) {
		probe.NoteExternal("board-embedder")
		vec := testutil.PadVector([]float64{1, 0, 0}, testutil.TestEmbeddingDim)
		return core.FloatsToPgVector(vec), vec, nil
	}
	service := NewSemanticBoardUpgradeService(probeDB, nil, embedder)

	result, err := service.ConfirmSuggestion(context.Background(), ConfirmSemanticBoardUpgradeRequest{
		Decision:          SemanticBoardUpgradeDecisionCreateNew,
		BoardLabel:        "探针版块",
		Description:       "事务外预计算",
		AuxiliaryLabelIDs: []uint{auxA.ID, auxB.ID},
	})
	require.NoError(t, err)
	require.NotZero(t, result.SemanticBoardID)

	require.True(t, probe.Began(), "confirm must have run (and closed) its write transaction")
	require.Empty(t, probe.Violations(), "board embedding must be precomputed before BEGIN (idle-in-transaction red line)")

	var board models.SemanticLabel
	require.NoError(t, db.First(&board, result.SemanticBoardID).Error)
	require.NotNil(t, board.Embedding, "precomputed embedding must be persisted with the board row")
}

// 主链路步10（compose 方向）：组合短语 LLM embedding 在 PlanCompositeLabel
// （事务外）完成，事务内 ApplyCompositeLabel 纯写。
func TestConfirmComposeSuggestionEmbedderOutsideTransaction(t *testing.T) {
	db := setupSemanticBoardUpgradeTestDB(t)
	repo := repository.NewBoardUpgradeSuggestionRepository(db)
	auxA := createUpgradeLabel(t, db, "组合探针甲", "probe-ca", "auxiliary", "active", 8, []float64{1, 0, 0})
	auxB := createUpgradeLabel(t, db, "组合探针乙", "probe-cb", "auxiliary", "active", 8, []float64{0, 1, 0})

	probeDB, probe := testutil.NewBeginProbeDB(t)
	embedder := func(ctx context.Context, input string, mode auxlabel.AuxiliaryLabelEmbeddingMode) (string, []float64, error) {
		probe.NoteExternal("compose-embedder")
		vec := testutil.PadVector([]float64{1, 0, 0}, testutil.TestEmbeddingDim)
		return core.FloatsToPgVector(vec), vec, nil
	}
	service := NewSemanticBoardUpgradeService(probeDB, nil, embedder)

	suggestion := &tagmodels.BoardUpgradeSuggestion{
		BatchID: "probe-compose", Mode: "", Decision: "compose", BoardLabel: "探针组合",
		Description: "组合描述", AuxiliaryLabelIDs: []uint{auxA.ID, auxB.ID},
		Status: "pending", SuggestionHash: ComputeSuggestionHash("", "compose", nil, []uint{auxA.ID, auxB.ID}),
	}
	ok, err := repo.InsertPending(context.Background(), suggestion)
	require.NoError(t, err)
	require.True(t, ok)

	result, err := service.ConfirmSuggestion(context.Background(), ConfirmSemanticBoardUpgradeRequest{
		Decision:          SemanticBoardUpgradeDecisionCompose,
		BoardLabel:        "探针组合",
		Description:       "组合描述",
		AuxiliaryLabelIDs: []uint{auxA.ID, auxB.ID},
		SuggestionID:      &suggestion.ID,
	})
	require.NoError(t, err)
	require.NotNil(t, result.CompositeLabelID)

	require.True(t, probe.Began(), "compose confirm must run its write transaction (label + MarkConfirmed)")
	require.Empty(t, probe.Violations(), "composite-phrase embedding must happen in PlanCompositeLabel before BEGIN")

	var confirmed tagmodels.BoardUpgradeSuggestion
	require.NoError(t, db.Where("id = ?", suggestion.ID).First(&confirmed).Error)
	require.Equal(t, "confirmed", confirmed.Status, "same-tx MarkConfirmed must still hold")
}
