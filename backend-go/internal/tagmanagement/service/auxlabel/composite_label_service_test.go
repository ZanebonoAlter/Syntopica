package auxlabel

import (
	"context"
	"fmt"
	"math"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/testutil"
)

func seedCompositeComponentLabel(t *testing.T, db *gorm.DB, label, slug, labelType, status string) *models.SemanticLabel {
	t.Helper()
	row := &models.SemanticLabel{Label: label, Slug: slug, LabelType: labelType, Status: status, Source: "llm_extract"}
	require.NoError(t, db.Create(row).Error)
	return row
}

// unitVec builds a 2D direction padded to the test embedding dimension, so
// cosine against {1,0} equals x.
func unitVec(t *testing.T, x float64) []float64 {
	t.Helper()
	return testutil.PadVector([]float64{x, math.Sqrt(1 - x*x)}, testutil.TestEmbeddingDim)
}

func TestCreateCompositeLabelComponentCountBoundary(t *testing.T) {
	db := setupAuxiliaryLabelTestDB(t)
	embedder := &recordingAuxiliaryEmbedder{}
	service := NewCompositeLabelService(db, embedder.embed)

	mkIDs := func(prefix string, n int) []uint {
		ids := make([]uint, 0, n)
		for i := 0; i < n; i++ {
			row := seedCompositeComponentLabel(t, db, prefix+string(rune('A'+i)), "comp-cnt-"+prefix+string(rune('a'+i)), "auxiliary", "active")
			ids = append(ids, row.ID)
		}
		return ids
	}

	for _, n := range []int{1, 6} {
		_, err := service.CreateCompositeLabel(context.Background(), "组合", "", mkIDs(fmt.Sprintf("case%d-", n), n), "manual")
		require.Error(t, err, "component count %d must be rejected", n)
	}
	for _, n := range []int{2, 5} {
		_, err := service.CreateCompositeLabel(context.Background(), "组合", "", mkIDs(fmt.Sprintf("ok%d-", n), n), "manual")
		require.NoError(t, err, "component count %d must be accepted", n)
	}
}

func TestCreateCompositeLabelComponentValidation(t *testing.T) {
	db := setupAuxiliaryLabelTestDB(t)
	embedder := &recordingAuxiliaryEmbedder{}
	service := NewCompositeLabelService(db, embedder.embed)
	ctx := context.Background()

	// Non-auxiliary component type.
	board := seedCompositeComponentLabel(t, db, "Board", "comp-val-board", "board", "active")
	aux := seedCompositeComponentLabel(t, db, "Aux", "comp-val-aux", "auxiliary", "active")
	_, err := service.CreateCompositeLabel(ctx, "组合", "", []uint{aux.ID, board.ID}, "manual")
	require.ErrorContains(t, err, "active auxiliary")

	// Disabled auxiliary component.
	disabled := seedCompositeComponentLabel(t, db, "Off", "comp-val-off", "auxiliary", "disabled")
	_, err = service.CreateCompositeLabel(ctx, "组合", "", []uint{aux.ID, disabled.ID}, "manual")
	require.ErrorContains(t, err, "active auxiliary")

	// Nonexistent component ID.
	_, err = service.CreateCompositeLabel(ctx, "组合", "", []uint{aux.ID, 999999}, "manual")
	require.ErrorContains(t, err, "active auxiliary")

	// Duplicate IDs collapse to one → below the 2-component minimum.
	_, err = service.CreateCompositeLabel(ctx, "组合", "", []uint{aux.ID, aux.ID}, "manual")
	require.ErrorContains(t, err, "2-5 distinct components")

	// Zero ID rejected.
	_, err = service.CreateCompositeLabel(ctx, "组合", "", []uint{aux.ID, 0}, "manual")
	require.ErrorContains(t, err, "must not be zero")

	// Empty / whitespace label rejected.
	_, err = service.CreateCompositeLabel(ctx, "   ", "", []uint{aux.ID, disabled.ID}, "manual")
	require.ErrorContains(t, err, "must not be empty")

	// Invalid source rejected.
	_, err = service.CreateCompositeLabel(ctx, "组合", "", []uint{aux.ID, disabled.ID}, "hacker")
	require.ErrorContains(t, err, "manual or upgrade_suggest")
}

func TestCreateCompositeLabelEmbedsPhraseAndStoresComponents(t *testing.T) {
	db := setupAuxiliaryLabelTestDB(t)
	embedder := &recordingAuxiliaryEmbedder{
		vectors: map[string][]float64{"美债收益率. 美国国债与收益率的组合": unitVec(t, 0.5)},
	}
	service := NewCompositeLabelService(db, embedder.embed)

	auxA := seedCompositeComponentLabel(t, db, "美国国债", "comp-emb-us-treasury", "auxiliary", "active")
	auxB := seedCompositeComponentLabel(t, db, "收益率", "comp-emb-yield", "auxiliary", "active")

	result, err := service.CreateCompositeLabel(context.Background(), "美债收益率", "美国国债与收益率的组合", []uint{auxA.ID, auxB.ID}, "upgrade_suggest")

	require.NoError(t, err)
	require.Equal(t, CompositeOutcomeCreated, result.Outcome)
	require.Equal(t, "composite", result.Label.LabelType)
	require.Equal(t, "upgrade_suggest", result.Label.Source)
	require.Equal(t, "active", result.Label.Status)
	require.NotNil(t, result.Label.Embedding)
	require.Nil(t, result.Label.MergeEmbedding, "merge_embedding must stay unused for composites")
	require.Equal(t, []string{"美债收益率. 美国国债与收益率的组合"}, embedder.calls, "embedder input must be the composite phrase (label + \". \" + description), never component vectors")
	require.Equal(t, AuxiliaryLabelEmbeddingModeStorage, embedder.modes[0])

	var comps []models.CompositeComponent
	require.NoError(t, db.Where("composite_id = ?", result.Label.ID).Order("position").Find(&comps).Error)
	require.Len(t, comps, 2)
	require.Equal(t, auxA.ID, comps[0].ComponentLabelID)
	require.Equal(t, 1, comps[0].Position)
	require.Equal(t, auxB.ID, comps[1].ComponentLabelID)
	require.Equal(t, 2, comps[1].Position)
}

func TestCreateCompositeLabelEmbedderFailureRollsBack(t *testing.T) {
	db := setupAuxiliaryLabelTestDB(t)
	failing := &failingCompositeEmbedder{}
	service := NewCompositeLabelService(db, failing.embed)

	auxA := seedCompositeComponentLabel(t, db, "A", "comp-fail-a", "auxiliary", "active")
	auxB := seedCompositeComponentLabel(t, db, "B", "comp-fail-b", "auxiliary", "active")

	_, err := service.CreateCompositeLabel(context.Background(), "组合", "", []uint{auxA.ID, auxB.ID}, "manual")
	require.ErrorContains(t, err, "generate composite embedding")

	var labelCount, compCount int64
	require.NoError(t, db.Model(&models.SemanticLabel{}).Where("label_type = ?", "composite").Count(&labelCount).Error)
	require.NoError(t, db.Model(&models.CompositeComponent{}).Count(&compCount).Error)
	require.Zero(t, labelCount, "no composite row may survive an embedder failure")
	require.Zero(t, compCount, "no component rows may survive an embedder failure")
}

func TestCompositeDedupeL1ReusesOnSetEquality(t *testing.T) {
	db := setupAuxiliaryLabelTestDB(t)
	embedder := &recordingAuxiliaryEmbedder{}
	service := NewCompositeLabelService(db, embedder.embed)
	ctx := context.Background()

	auxA := seedCompositeComponentLabel(t, db, "美国国债", "comp-l1-us", "auxiliary", "active")
	auxB := seedCompositeComponentLabel(t, db, "收益率", "comp-l1-yield", "auxiliary", "active")

	first, err := service.CreateCompositeLabel(ctx, "美国国债收益率", "", []uint{auxA.ID, auxB.ID}, "manual")
	require.NoError(t, err)
	require.Equal(t, CompositeOutcomeCreated, first.Outcome)

	// Same set, different order and different label → L1 reuse, no embedder call.
	callsBefore := len(embedder.calls)
	second, err := service.CreateCompositeLabel(ctx, "美债收益率", "", []uint{auxB.ID, auxA.ID}, "manual")
	require.NoError(t, err)
	require.Equal(t, CompositeOutcomeReusedL1, second.Outcome)
	require.Equal(t, first.Label.ID, second.Label.ID)
	require.Equal(t, callsBefore, len(embedder.calls), "L1 hit must not call the embedder")
	require.Equal(t, 1, second.Label.RefCount, "L1 reuse increments ref_count once")
}

func TestCompositeDedupeL2Boundary(t *testing.T) {
	db := setupAuxiliaryLabelTestDB(t)
	embedder := &recordingAuxiliaryEmbedder{
		vectors: map[string][]float64{
			"美国国债收益率": unitVec(t, 1),
			"0.9499":  unitVec(t, 0.9499),
			"0.95":    unitVec(t, 0.95),
			"0.9501":  unitVec(t, 0.9501),
		},
	}
	service := NewCompositeLabelService(db, embedder.embed)
	ctx := context.Background()

	auxA := seedCompositeComponentLabel(t, db, "美国国债", "comp-l2-us", "auxiliary", "active")
	auxB := seedCompositeComponentLabel(t, db, "收益率", "comp-l2-yield", "auxiliary", "active")
	auxC := seedCompositeComponentLabel(t, db, "利率", "comp-l2-rate", "auxiliary", "active")
	auxD := seedCompositeComponentLabel(t, db, "国债", "comp-l2-bond", "auxiliary", "active")
	auxE := seedCompositeComponentLabel(t, db, "中美利差", "comp-l2-spread", "auxiliary", "active")

	base, err := service.CreateCompositeLabel(ctx, "美国国债收益率", "", []uint{auxA.ID, auxB.ID}, "manual")
	require.NoError(t, err)
	baseID := base.Label.ID

	// Each case uses a distinct component pair so earlier creates never L1-hit later ones.
	cases := []struct {
		label  string
		pair   []uint
		expect CompositeCreateOutcome
	}{
		{"0.9499", []uint{auxA.ID, auxC.ID}, CompositeOutcomeCreated},
		{"0.95", []uint{auxA.ID, auxD.ID}, CompositeOutcomeAliasL2},
		{"0.9501", []uint{auxA.ID, auxE.ID}, CompositeOutcomeAliasL2},
	}
	for _, tc := range cases {
		result, err := service.CreateCompositeLabel(ctx, tc.label, "", tc.pair, "manual")
		require.NoError(t, err, tc.label)
		require.Equal(t, tc.expect, result.Outcome, "cosine %s against base", tc.label)
		if tc.expect == CompositeOutcomeAliasL2 {
			require.Equal(t, baseID, result.Label.ID, "L2 hit reuses the existing composite")
			require.Contains(t, result.Label.Aliases, tc.label, "L2 hit appends the new label as alias")
		}
	}

	// Anti-blackhole: the L2 base keeps its original label and vector.
	var reloaded models.SemanticLabel
	require.NoError(t, db.Where("id = ?", baseID).First(&reloaded).Error)
	require.Equal(t, "美国国债收益率", reloaded.Label)
	require.NotNil(t, reloaded.Embedding)
	require.Nil(t, reloaded.MergeEmbedding)
}

func TestCompositeDedupeL2AliasIdempotent(t *testing.T) {
	db := setupAuxiliaryLabelTestDB(t)
	embedder := &recordingAuxiliaryEmbedder{
		vectors: map[string][]float64{
			"美国国债收益率": unitVec(t, 1),
			"美债收益率":   unitVec(t, 0.96),
		},
	}
	service := NewCompositeLabelService(db, embedder.embed)
	ctx := context.Background()

	auxA := seedCompositeComponentLabel(t, db, "美国国债", "comp-idem-us", "auxiliary", "active")
	auxB := seedCompositeComponentLabel(t, db, "收益率", "comp-idem-yield", "auxiliary", "active")
	auxC := seedCompositeComponentLabel(t, db, "利率", "comp-idem-rate", "auxiliary", "active")

	_, err := service.CreateCompositeLabel(ctx, "美国国债收益率", "", []uint{auxA.ID, auxB.ID}, "manual")
	require.NoError(t, err)

	for i := 0; i < 2; i++ {
		result, err := service.CreateCompositeLabel(ctx, "美债收益率", "", []uint{auxA.ID, auxC.ID}, "manual")
		require.NoError(t, err)
		require.Equal(t, CompositeOutcomeAliasL2, result.Outcome)
	}

	var reloaded models.SemanticLabel
	require.NoError(t, db.Where("label = ?", "美国国债收益率").First(&reloaded).Error)
	aliasCount := 0
	for _, a := range reloaded.Aliases {
		if a == "美债收益率" {
			aliasCount++
		}
	}
	require.Equal(t, 1, aliasCount, "alias appended exactly once across repeats")
	require.Equal(t, 2, reloaded.RefCount, "ref_count incremented once per create (2 alias hits)")
}

func TestCompositeDedupeSimConfigurable(t *testing.T) {
	db := setupAuxiliaryLabelTestDB(t)
	db.Exec(`INSERT INTO ai_settings (key, value, description) VALUES ('composite_label_dedupe_sim', '0.90', 'test') ON CONFLICT (key) DO UPDATE SET value = '0.90'`)

	embedder := &recordingAuxiliaryEmbedder{
		vectors: map[string][]float64{
			"美国国债收益率": unitVec(t, 1),
			"美债利率":    unitVec(t, 0.92),
		},
	}
	service := NewCompositeLabelService(db, embedder.embed)
	ctx := context.Background()

	auxA := seedCompositeComponentLabel(t, db, "美国国债", "comp-cfg-us", "auxiliary", "active")
	auxB := seedCompositeComponentLabel(t, db, "收益率", "comp-cfg-yield", "auxiliary", "active")
	auxC := seedCompositeComponentLabel(t, db, "利率", "comp-cfg-rate", "auxiliary", "active")

	_, err := service.CreateCompositeLabel(ctx, "美国国债收益率", "", []uint{auxA.ID, auxB.ID}, "manual")
	require.NoError(t, err)

	// cosine 0.92 < default 0.95 but >= configured 0.90 → alias.
	result, err := service.CreateCompositeLabel(ctx, "美债利率", "", []uint{auxA.ID, auxC.ID}, "manual")
	require.NoError(t, err)
	require.Equal(t, CompositeOutcomeAliasL2, result.Outcome)
}

func TestCompositeDedupeDisabledCompositeSemantics(t *testing.T) {
	db := setupAuxiliaryLabelTestDB(t)
	embedder := &recordingAuxiliaryEmbedder{
		vectors: map[string][]float64{"美债收益率": unitVec(t, 1)},
	}
	service := NewCompositeLabelService(db, embedder.embed)
	ctx := context.Background()

	auxA := seedCompositeComponentLabel(t, db, "美国国债", "comp-dis-us", "auxiliary", "active")
	auxB := seedCompositeComponentLabel(t, db, "收益率", "comp-dis-yield", "auxiliary", "active")

	first, err := service.CreateCompositeLabel(ctx, "美债收益率", "", []uint{auxA.ID, auxB.ID}, "manual")
	require.NoError(t, err)
	require.NoError(t, service.DisableCompositeLabel(ctx, first.Label.ID))

	// L1 still hits the disabled composite (exact identity must not fork the space).
	result, err := service.CreateCompositeLabel(ctx, "美债收益率复刻", "", []uint{auxB.ID, auxA.ID}, "manual")
	require.NoError(t, err)
	require.Equal(t, CompositeOutcomeReusedL1, result.Outcome)
	require.Equal(t, first.Label.ID, result.Label.ID)
	require.Equal(t, "disabled", result.Label.Status, "L1 hit on a disabled composite must not auto-enable it")

	// A different component set cannot L2-match the disabled composite (vector is NULL).
	result2, err := service.CreateCompositeLabel(ctx, "中债收益率", "", []uint{auxB.ID, 999998}, "manual")
	require.ErrorContains(t, err, "active auxiliary")
	_ = result2
}

func TestDisableCompositeLabelNullsVectorsKeepsRows(t *testing.T) {
	db := setupAuxiliaryLabelTestDB(t)
	embedder := &recordingAuxiliaryEmbedder{
		vectors: map[string][]float64{"美债收益率": unitVec(t, 1), "美债收益率. 重算": unitVec(t, 0.9)},
	}
	service := NewCompositeLabelService(db, embedder.embed)
	ctx := context.Background()

	auxA := seedCompositeComponentLabel(t, db, "美国国债", "comp-null-us", "auxiliary", "active")
	auxB := seedCompositeComponentLabel(t, db, "收益率", "comp-null-yield", "auxiliary", "active")
	created, err := service.CreateCompositeLabel(ctx, "美债收益率", "", []uint{auxA.ID, auxB.ID}, "manual")
	require.NoError(t, err)
	// Seed an alias directly (re-creating the same set hits L1, which adds no alias).
	require.NoError(t, db.Exec(`UPDATE semantic_labels SET aliases = '["美债收益率别名"]'::jsonb WHERE id = ?`, created.Label.ID).Error)

	require.NoError(t, service.DisableCompositeLabel(ctx, created.Label.ID))

	var reloaded models.SemanticLabel
	require.NoError(t, db.Where("id = ?", created.Label.ID).First(&reloaded).Error)
	require.Equal(t, "disabled", reloaded.Status)
	require.Nil(t, reloaded.Embedding, "disable must drop the vector")
	require.Nil(t, reloaded.MergeEmbedding)
	var comps int64
	require.NoError(t, db.Model(&models.CompositeComponent{}).Where("composite_id = ?", created.Label.ID).Count(&comps).Error)
	require.Equal(t, int64(2), comps, "components survive disable")
	require.NotEmpty(t, reloaded.Aliases, "aliases survive disable")

	// Disable is idempotent-scoped: unknown id → not found.
	require.ErrorIs(t, service.DisableCompositeLabel(ctx, 999997), gorm.ErrRecordNotFound)
}

func TestEnableCompositeLabelReembeds(t *testing.T) {
	db := setupAuxiliaryLabelTestDB(t)
	embedder := &recordingAuxiliaryEmbedder{
		vectors: map[string][]float64{"美债收益率": unitVec(t, 1)},
	}
	service := NewCompositeLabelService(db, embedder.embed)
	ctx := context.Background()

	auxA := seedCompositeComponentLabel(t, db, "美国国债", "comp-ena-us", "auxiliary", "active")
	auxB := seedCompositeComponentLabel(t, db, "收益率", "comp-ena-yield", "auxiliary", "active")
	created, err := service.CreateCompositeLabel(ctx, "美债收益率", "", []uint{auxA.ID, auxB.ID}, "manual")
	require.NoError(t, err)
	require.NoError(t, service.DisableCompositeLabel(ctx, created.Label.ID))

	require.NoError(t, service.EnableCompositeLabel(ctx, created.Label.ID))

	var reloaded models.SemanticLabel
	require.NoError(t, db.Where("id = ?", created.Label.ID).First(&reloaded).Error)
	require.Equal(t, "active", reloaded.Status)
	require.NotNil(t, reloaded.Embedding, "enable must regenerate the vector")
}

func TestEnableCompositeLabelEmbedderFailureStaysDisabled(t *testing.T) {
	db := setupAuxiliaryLabelTestDB(t)
	embedder := &recordingAuxiliaryEmbedder{
		vectors: map[string][]float64{"美债收益率": unitVec(t, 1)},
	}
	service := NewCompositeLabelService(db, embedder.embed)
	ctx := context.Background()

	auxA := seedCompositeComponentLabel(t, db, "美国国债", "comp-enf-us", "auxiliary", "active")
	auxB := seedCompositeComponentLabel(t, db, "收益率", "comp-enf-yield", "auxiliary", "active")
	created, err := service.CreateCompositeLabel(ctx, "美债收益率", "", []uint{auxA.ID, auxB.ID}, "manual")
	require.NoError(t, err)
	require.NoError(t, service.DisableCompositeLabel(ctx, created.Label.ID))

	failing := &failingCompositeEmbedder{}
	failingService := NewCompositeLabelService(db, failing.embed)
	require.Error(t, failingService.EnableCompositeLabel(ctx, created.Label.ID))

	var reloaded models.SemanticLabel
	require.NoError(t, db.Where("id = ?", created.Label.ID).First(&reloaded).Error)
	require.Equal(t, "disabled", reloaded.Status, "embed failure must leave the row disabled")
	require.Nil(t, reloaded.Embedding)
}

func TestListCompositeLabelsShowsOrderedComponents(t *testing.T) {
	db := setupAuxiliaryLabelTestDB(t)
	embedder := &recordingAuxiliaryEmbedder{}
	service := NewCompositeLabelService(db, embedder.embed)
	ctx := context.Background()

	// Empty state.
	views, err := service.ListCompositeLabels(ctx, "")
	require.NoError(t, err)
	require.Empty(t, views)

	auxA := seedCompositeComponentLabel(t, db, "美国国债", "comp-lst-us", "auxiliary", "active")
	auxB := seedCompositeComponentLabel(t, db, "收益率", "comp-lst-yield", "auxiliary", "active")
	created, err := service.CreateCompositeLabel(ctx, "美债收益率", "组合描述", []uint{auxA.ID, auxB.ID}, "manual")
	require.NoError(t, err)
	require.NoError(t, service.DisableCompositeLabel(ctx, created.Label.ID))

	views, err = service.ListCompositeLabels(ctx, "")
	require.NoError(t, err)
	require.Len(t, views, 1)
	require.Equal(t, "美债收益率", views[0].Label)
	require.Equal(t, "disabled", views[0].Status)
	require.Len(t, views[0].Components, 2)
	require.Equal(t, "美国国债", views[0].Components[0].Label)
	require.Equal(t, 1, views[0].Components[0].Position)
	require.Equal(t, "收益率", views[0].Components[1].Label)
	require.Equal(t, 2, views[0].Components[1].Position)

	views, err = service.ListCompositeLabels(ctx, "active")
	require.NoError(t, err)
	require.Empty(t, views, "status filter excludes disabled composites")
}

type failingCompositeEmbedder struct{}

func (e *failingCompositeEmbedder) embed(ctx context.Context, input string, mode AuxiliaryLabelEmbeddingMode) (string, []float64, error) {
	return "", nil, context.DeadlineExceeded
}

// TestListComponentOptionsRanksByBoardThenRefCount（S12 主链路步1 + 变体1/2）：
// 推荐排序 = active 版块挂载数 DESC → ref_count DESC → label ASC；
// disabled 版块挂载不计入；无任何挂载时纯 ref_count 降序；结果带挂载版块名。
func TestListComponentOptionsRanksByBoardThenRefCount(t *testing.T) {
	db := setupAuxiliaryLabelTestDB(t)
	embedder := &recordingAuxiliaryEmbedder{}
	service := NewCompositeLabelService(db, embedder.embed)

	// 版块：两个 active + 一个 disabled
	boardAI := seedCompositeComponentLabel(t, db, "AI 版块", "board-ai", "board", "active")
	boardMacro := seedCompositeComponentLabel(t, db, "宏观版块", "board-macro", "board", "active")
	boardOld := seedCompositeComponentLabel(t, db, "旧版块", "board-old", "board", "disabled")

	// aux：双挂载（AI+宏观）/ 单挂载（仅 disabled 旧版块→不计）/ 无挂载高 ref / 无挂载低 ref
	auxYield := seedCompositeComponentLabel(t, db, "收益率", "comp-yield", "auxiliary", "active")
	auxTreasury := seedCompositeComponentLabel(t, db, "美国国债", "comp-treasury", "auxiliary", "active")
	auxStale := seedCompositeComponentLabel(t, db, "旧概念", "comp-stale", "auxiliary", "active")
	auxHot := seedCompositeComponentLabel(t, db, "热门通用", "comp-hot", "auxiliary", "active")
	auxCold := seedCompositeComponentLabel(t, db, "冷门通用", "comp-cold", "auxiliary", "active")
	require.NoError(t, db.Model(&models.SemanticLabel{}).Where("id = ?", auxYield.ID).Update("ref_count", 5).Error)
	require.NoError(t, db.Model(&models.SemanticLabel{}).Where("id = ?", auxTreasury.ID).Update("ref_count", 30).Error)
	require.NoError(t, db.Model(&models.SemanticLabel{}).Where("id = ?", auxStale.ID).Update("ref_count", 50).Error)
	require.NoError(t, db.Model(&models.SemanticLabel{}).Where("id = ?", auxHot.ID).Update("ref_count", 20).Error)
	require.NoError(t, db.Model(&models.SemanticLabel{}).Where("id = ?", auxCold.ID).Update("ref_count", 1).Error)
	for _, m := range []struct {
		board uint
		aux   uint
	}{
		{boardAI.ID, auxYield.ID},
		{boardMacro.ID, auxYield.ID},
		{boardOld.ID, auxStale.ID}, // disabled 版块挂载不计入
	} {
		require.NoError(t, db.Create(&models.BoardComposition{BoardID: m.board, AuxiliaryLabelID: m.aux}).Error)
	}

	options, err := service.ListComponentOptions(context.Background(), 0, 0, 0)
	require.NoError(t, err)
	require.NotEmpty(t, options)

	// 期望顺序（board_count DESC → ref_count DESC）：
	// 收益率(2 挂载, ref 5) > 旧概念(0 挂载, ref 50, disabled 版块挂载不计) >
	// 美国国债(0 挂载, ref 30) > 热门通用(0 挂载, ref 20) > 冷门通用(0 挂载, ref 1)
	labels := make([]string, 0, len(options))
	for _, o := range options {
		labels = append(labels, o.Label)
	}
	require.Equal(t, []string{"收益率", "旧概念", "美国国债", "热门通用", "冷门通用"}, labels)

	byLabel := make(map[string]ComponentOptionView, len(options))
	for _, o := range options {
		byLabel[o.Label] = o
	}
	require.Equal(t, 2, byLabel["收益率"].BoardCount)
	require.Len(t, byLabel["收益率"].MountedBoards, 2)
	require.Zero(t, byLabel["旧概念"].BoardCount, "disabled 版块挂载不计入 board_count")
	require.Empty(t, byLabel["旧概念"].MountedBoards)
}

// TestListComponentOptionsLimitCapsResult：limit 生效且非法值回退默认 50。
func TestListComponentOptionsLimitCapsResult(t *testing.T) {
	db := setupAuxiliaryLabelTestDB(t)
	embedder := &recordingAuxiliaryEmbedder{}
	service := NewCompositeLabelService(db, embedder.embed)
	for i := 0; i < 3; i++ {
		seedCompositeComponentLabel(t, db, fmt.Sprintf("aux-%02d", i), fmt.Sprintf("comp-%02d", i), "auxiliary", "active")
	}

	options, err := service.ListComponentOptions(context.Background(), 2, 0, 0)
	require.NoError(t, err)
	require.Len(t, options, 2)

	// limit=0 → 默认 50；超过 3 行只返回 3
	all, err := service.ListComponentOptions(context.Background(), 0, 0, 0)
	require.NoError(t, err)
	require.Len(t, all, 3)
}

// TestListComponentOptionsBoardAndRelatedContext：board_id 版块置顶（in_board）
// 与 related_to 共现联动（cooccurrence 降序）两条推荐通路。
func TestListComponentOptionsBoardAndRelatedContext(t *testing.T) {
	db := setupAuxiliaryLabelTestDB(t)
	service := NewCompositeLabelService(db, (&recordingAuxiliaryEmbedder{}).embed)

	board := seedCompositeComponentLabel(t, db, "宏观版块", "ctx-macro", "board", "active")
	// 组件：本版块挂载的 / 全局高频的 / 与「美联储」共现的
	auxInBoard := seedCompositeComponentLabel(t, db, "美联储", "ctx-fed", "auxiliary", "active")
	auxGlobal := seedCompositeComponentLabel(t, db, "全球热词", "ctx-global", "auxiliary", "active")
	auxCooc := seedCompositeComponentLabel(t, db, "加息", "ctx-hike", "auxiliary", "active")
	auxCold := seedCompositeComponentLabel(t, db, "冷词", "ctx-cold", "auxiliary", "active")
	require.NoError(t, db.Model(&models.SemanticLabel{}).Where("id = ?", auxGlobal.ID).Update("ref_count", 99).Error)
	require.NoError(t, db.Create(&models.BoardComposition{BoardID: board.ID, AuxiliaryLabelID: auxInBoard.ID}).Error)
	// 共现：3 个 tag 同时挂 美联储+加息；1 个 tag 挂 美联储+冷词
	for i := 0; i < 3; i++ {
		tag := &models.TopicTag{Slug: fmt.Sprintf("ctx-tag-%d", i), Label: fmt.Sprintf("ctx tag %d", i)}
		require.NoError(t, db.Create(tag).Error)
		require.NoError(t, db.Create(&models.TopicTagSemanticLabel{TopicTagID: tag.ID, SemanticLabelID: auxInBoard.ID}).Error)
		require.NoError(t, db.Create(&models.TopicTagSemanticLabel{TopicTagID: tag.ID, SemanticLabelID: auxCooc.ID}).Error)
	}
	soloTag := &models.TopicTag{Slug: "ctx-tag-solo", Label: "ctx tag solo"}
	require.NoError(t, db.Create(soloTag).Error)
	require.NoError(t, db.Create(&models.TopicTagSemanticLabel{TopicTagID: soloTag.ID, SemanticLabelID: auxInBoard.ID}).Error)
	require.NoError(t, db.Create(&models.TopicTagSemanticLabel{TopicTagID: soloTag.ID, SemanticLabelID: auxCold.ID}).Error)

	// 版块上下文：本版块挂载的「美联储」置顶（ref_count 最低也压过 ref=99 的全球热词）
	opts, err := service.ListComponentOptions(context.Background(), 0, board.ID, 0)
	require.NoError(t, err)
	require.Equal(t, "美联储", opts[0].Label)
	require.True(t, opts[0].InBoard)
	byLabel := make(map[string]ComponentOptionView, len(opts))
	for _, o := range opts {
		byLabel[o.Label] = o
	}
	require.False(t, byLabel["全球热词"].InBoard)

	// 共现联动：related_to=美联储 → 加息(3 次) > 冷词(1 次) > 其它(0)
	opts2, err := service.ListComponentOptions(context.Background(), 0, board.ID, auxInBoard.ID)
	require.NoError(t, err)
	require.Equal(t, "加息", opts2[0].Label)
	require.Equal(t, 3, opts2[0].Cooccurrence)
	labels2 := make([]string, 0, len(opts2))
	for _, o := range opts2 {
		labels2 = append(labels2, o.Label)
	}
	require.Equal(t, []string{"加息", "冷词", "美联储", "全球热词"}, labels2)
}
