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
	"syntopica-backend/internal/tagmanagement/repository"
	"syntopica-backend/internal/tagmanagement/service/auxlabel"
	"syntopica-backend/internal/tagmanagement/service/core"
)

// createComposeAux creates an active auxiliary label with an explicit ref_count
// and a deterministic embedding.
func createComposeAux(t *testing.T, db *gorm.DB, label string, slug string, refCount int) models.SemanticLabel {
	t.Helper()
	pgVector := core.FloatsToPgVector(testutil.PadVector([]float64{1, 0, 0}, testutil.TestEmbeddingDim))
	semanticLabel := models.SemanticLabel{Label: label, Slug: slug, LabelType: "auxiliary", Status: "active", RefCount: refCount, Embedding: &pgVector}
	require.NoError(t, db.Create(&semanticLabel).Error)
	return semanticLabel
}

// createComposeEventTag creates an event tag mounting the given aux labels.
func createComposeEventTag(t *testing.T, db *gorm.DB, slug string, auxIDs ...uint) models.TopicTag {
	t.Helper()
	tag := models.TopicTag{Label: slug, Slug: slug, Category: "event", Status: "active"}
	require.NoError(t, db.Create(&tag).Error)
	for _, auxID := range auxIDs {
		require.NoError(t, db.Create(&models.TopicTagSemanticLabel{TopicTagID: tag.ID, SemanticLabelID: auxID}).Error)
	}
	return tag
}

// createComposeArticles creates n articles (created `age` ago) all linked to
// the given tags.
func createComposeArticles(t *testing.T, db *gorm.DB, n int, age time.Duration, topicTagIDs ...uint) {
	t.Helper()
	created := time.Now().Add(-age)
	for i := 0; i < n; i++ {
		seq := time.Now().UnixNano()
		feed := models.Feed{Title: fmt.Sprintf("feed-%d-%d", seq, i), URL: fmt.Sprintf("https://example.com/%d-%d", seq, i), CreatedAt: created}
		require.NoError(t, db.Create(&feed).Error)
		article := models.Article{FeedID: feed.ID, Title: fmt.Sprintf("article-%d-%d", seq, i), CreatedAt: created}
		require.NoError(t, db.Create(&article).Error)
		for _, tagID := range topicTagIDs {
			require.NoError(t, db.Create(&models.ArticleTopicTag{ArticleID: article.ID, TopicTagID: tagID}).Error)
		}
	}
}

func composeConfig() SemanticBoardUpgradeConfig {
	return SemanticBoardUpgradeConfig{
		RefCountThreshold:             5,
		CoTagWindowDays:               30,
		CompositeCoTagMinCooccurrence: 10,
	}
}

func TestCollectComposeCandidatesThresholdBoundary(t *testing.T) {
	db := setupSemanticBoardUpgradeTestDB(t)
	service := NewSemanticBoardUpgradeService(db, nil, nil)

	auxA := createComposeAux(t, db, "美国国债", "cc-us-treasury", 8)
	auxB := createComposeAux(t, db, "收益率", "cc-yield", 6)

	// 9 co-occurring articles → below threshold → no candidate.
	tag9 := createComposeEventTag(t, db, "cc-below", auxA.ID, auxB.ID)
	createComposeArticles(t, db, 9, 0, tag9.ID)
	candidates, err := service.collectComposeCandidates(context.Background(), composeConfig())
	require.NoError(t, err)
	require.Empty(t, candidates, "co-occurrence 9 < 10 must not qualify")

	// +2 articles → 11 ≥ 10 → candidate with cooccurrence=11 (boundary inclusive).
	createComposeArticles(t, db, 2, 0, tag9.ID)
	candidates, err = service.collectComposeCandidates(context.Background(), composeConfig())
	require.NoError(t, err)
	require.Len(t, candidates, 1)
	require.Equal(t, []uint{auxA.ID, auxB.ID}, candidates[0].ComponentIDs)
	require.Equal(t, 11, candidates[0].Cooccurrence)
}

func TestCollectComposeCandidatesRefCountAndDisabled(t *testing.T) {
	db := setupSemanticBoardUpgradeTestDB(t)
	service := NewSemanticBoardUpgradeService(db, nil, nil)
	config := composeConfig()

	// ref_count = threshold-1 → excluded even with 10 co-occurring articles.
	weakAux := createComposeAux(t, db, "弱标签", "cc-weak", 4)
	okAux := createComposeAux(t, db, "强标签", "cc-strong", 5)
	tagWeak := createComposeEventTag(t, db, "cc-weakpair", weakAux.ID, okAux.ID)
	createComposeArticles(t, db, 10, 0, tagWeak.ID)

	// disabled aux → excluded.
	pgVector := core.FloatsToPgVector(testutil.PadVector([]float64{1, 0, 0}, testutil.TestEmbeddingDim))
	disabledAux := models.SemanticLabel{Label: "禁用标签", Slug: "cc-disabled", LabelType: "auxiliary", Status: "disabled", RefCount: 9, Embedding: &pgVector}
	require.NoError(t, db.Create(&disabledAux).Error)
	tagDisabled := createComposeEventTag(t, db, "cc-disabledpair", okAux.ID, disabledAux.ID)
	createComposeArticles(t, db, 10, 0, tagDisabled.ID)

	candidates, err := service.collectComposeCandidates(context.Background(), config)
	require.NoError(t, err)
	require.Empty(t, candidates, "ref_count below threshold or disabled aux must not qualify")
}

func TestCollectComposeCandidatesWindowBoundary(t *testing.T) {
	db := setupSemanticBoardUpgradeTestDB(t)
	service := NewSemanticBoardUpgradeService(db, nil, nil)
	config := composeConfig()

	auxA := createComposeAux(t, db, "窗口甲", "cc-win-a", 8)
	auxB := createComposeAux(t, db, "窗口乙", "cc-win-b", 8)

	// 10 articles at day-31 → outside window → no candidate.
	oldTag := createComposeEventTag(t, db, "cc-oldwin", auxA.ID, auxB.ID)
	createComposeArticles(t, db, 10, 31*24*time.Hour, oldTag.ID)
	candidates, err := service.collectComposeCandidates(context.Background(), config)
	require.NoError(t, err)
	require.Empty(t, candidates, "articles older than CoTagWindowDays must not count")

	// 10 articles inside the window (day-29) → candidate.
	freshTag := createComposeEventTag(t, db, "cc-freshwin", auxA.ID, auxB.ID)
	createComposeArticles(t, db, 10, 29*24*time.Hour, freshTag.ID)
	candidates, err = service.collectComposeCandidates(context.Background(), config)
	require.NoError(t, err)
	require.Len(t, candidates, 1)
	require.Equal(t, 10, candidates[0].Cooccurrence)
}

func TestCollectComposeCandidatesTripleAbsorbsPairs(t *testing.T) {
	db := setupSemanticBoardUpgradeTestDB(t)
	service := NewSemanticBoardUpgradeService(db, nil, nil)
	config := composeConfig()

	auxA := createComposeAux(t, db, "三元甲", "cc-tri-a", 8)
	auxB := createComposeAux(t, db, "三元乙", "cc-tri-b", 8)
	auxC := createComposeAux(t, db, "三元丙", "cc-tri-c", 8)

	// 10 articles whose tag mounts all three → three qualified pairs + a
	// qualified triple; the triple absorbs its sub-pairs.
	tag := createComposeEventTag(t, db, "cc-triple", auxA.ID, auxB.ID, auxC.ID)
	createComposeArticles(t, db, 10, 0, tag.ID)

	candidates, err := service.collectComposeCandidates(context.Background(), config)
	require.NoError(t, err)
	require.Len(t, candidates, 1, "triple must replace its qualified sub-pairs")
	require.Equal(t, []uint{auxA.ID, auxB.ID, auxC.ID}, candidates[0].ComponentIDs)
	require.Equal(t, 10, candidates[0].Cooccurrence)
}

func TestCollectComposeCandidatesTopLimit(t *testing.T) {
	db := setupSemanticBoardUpgradeTestDB(t)
	service := NewSemanticBoardUpgradeService(db, nil, nil)
	config := composeConfig()

	// 21 independent pairs (aux_i × common) each with 10 co-occurring articles
	// → capped at composeCandidateLimit (20), stable order (frequency desc,
	// component ids asc).
	common := createComposeAux(t, db, "公共标签", "cc-common", 99)
	for i := 0; i < 21; i++ {
		partner := createComposeAux(t, db, fmt.Sprintf("伙伴%d", i), fmt.Sprintf("cc-partner-%d", i), 10)
		tag := createComposeEventTag(t, db, fmt.Sprintf("cc-pair-%d", i), common.ID, partner.ID)
		createComposeArticles(t, db, 10, 0, tag.ID)
	}

	candidates, err := service.collectComposeCandidates(context.Background(), config)
	require.NoError(t, err)
	require.Len(t, candidates, composeCandidateLimit)
	// All candidates co-occur 10 times and share the common component, so the
	// stable order is determined by the partner id (position [1]).
	for i := 1; i < len(candidates); i++ {
		require.Less(t, candidates[i-1].ComponentIDs[1], candidates[i].ComponentIDs[1],
			"equal-frequency candidates must sort deterministically by component ids")
	}
}

// composeAwareLLM routes by mode: cluster prompts get cluster decisions,
// compose prompts get compose decisions.
type composeAwareLLM struct {
	clusterSuggestions []SemanticBoardUpgradeSuggestion
	composeSuggestions []SemanticBoardUpgradeSuggestion
	composeError       error
	composeCalls       int
	lastComposePrompt  string
}

func (f *composeAwareLLM) SuggestSemanticBoardUpgrades(ctx context.Context, prompt string, mode string) ([]SemanticBoardUpgradeSuggestion, error) {
	if mode == "create:composite" || mode == "expand:composite" {
		f.composeCalls++
		f.lastComposePrompt = prompt
		if f.composeError != nil {
			return nil, f.composeError
		}
		return f.composeSuggestions, nil
	}
	return f.clusterSuggestions, nil
}

func TestGenerateSuggestionsComposeRoundTrip(t *testing.T) {
	db := setupSemanticBoardUpgradeTestDB(t)
	auxA := createComposeAux(t, db, "美国国债", "gr-us-treasury", 8)
	auxB := createComposeAux(t, db, "收益率", "gr-yield", 6)
	tag := createComposeEventTag(t, db, "gr-event", auxA.ID, auxB.ID)
	createComposeArticles(t, db, 12, 0, tag.ID)

	llm := &composeAwareLLM{composeSuggestions: []SemanticBoardUpgradeSuggestion{{
		Decision:          SemanticBoardUpgradeDecisionCompose,
		BoardLabel:        "美债收益率",
		Description:       "美国国债与收益率的组合",
		AuxiliaryLabelIDs: []uint{auxA.ID, auxB.ID},
	}, {
		Decision:          SemanticBoardUpgradeDecisionCompose,
		BoardLabel:        "幻觉组合",
		AuxiliaryLabelIDs: []uint{auxA.ID, auxB.ID, 99999}, // hallucinated id → filtered
	}, {
		Decision: SemanticBoardUpgradeDecisionSkip,
	}}}
	service := NewSemanticBoardUpgradeService(db, llm, nil)

	// Cold start: aux pool (2) < RefCountThreshold (5) — compose round still runs.
	suggestions, _, err := service.GenerateSuggestions(context.Background(), UpgradeGenerateRequest{Direction: UpgradeDirectionCreate, Source: UpgradeSourceComposite})
	require.NoError(t, err)
	require.Len(t, suggestions, 1, "one valid compose suggestion; hallucinated ids and skip filtered")
	require.Equal(t, SemanticBoardUpgradeDecisionCompose, suggestions[0].Decision)
	require.Equal(t, "美债收益率", suggestions[0].BoardLabel)
	require.Equal(t, []uint{auxA.ID, auxB.ID}, suggestions[0].AuxiliaryLabelIDs)
	require.Equal(t, "llm", suggestions[0].Confidence)
	require.Equal(t, 1, llm.composeCalls)
	require.Contains(t, llm.lastComposePrompt, "美债收益率")
	require.Contains(t, llm.lastComposePrompt, fmt.Sprintf("ID:%d", auxA.ID))
}

func TestGenerateSuggestionsComposeLLMFailureDegrades(t *testing.T) {
	db := setupSemanticBoardUpgradeTestDB(t)
	auxA := createComposeAux(t, db, "降级甲", "deg-a", 8)
	auxB := createComposeAux(t, db, "降级乙", "deg-b", 8)
	tag := createComposeEventTag(t, db, "deg-event", auxA.ID, auxB.ID)
	createComposeArticles(t, db, 12, 0, tag.ID)

	llm := &composeAwareLLM{composeError: errors.New("bad json from llm")}
	service := NewSemanticBoardUpgradeService(db, llm, nil)

	// 单入口语境：手动触发的 create×composite 对 LLM 失败诚实报错（不产半成品、
	// 不静默空列表误导用户）；定时任务的降级容错由 job 层负责（段失败仅记日志
	// 继续兄弟段，红线 10/16 语境适配——旧「整轮混跑不崩」语义已随四格拆分退役）。
	_, _, err := service.GenerateSuggestions(context.Background(), UpgradeGenerateRequest{Direction: UpgradeDirectionCreate, Source: UpgradeSourceComposite})
	require.Error(t, err, "compose LLM failure must surface on the manual single-source entry")
}

func TestGenerateAndPersistComposeLifecycle(t *testing.T) {
	db := setupSemanticBoardUpgradeTestDB(t)
	repo := repository.NewBoardUpgradeSuggestionRepository(db)
	auxA := createComposeAux(t, db, "落库甲", "lp-a", 8)
	auxB := createComposeAux(t, db, "落库乙", "lp-b", 8)
	tag := createComposeEventTag(t, db, "lp-event", auxA.ID, auxB.ID)
	createComposeArticles(t, db, 12, 0, tag.ID)

	llm := &composeAwareLLM{composeSuggestions: []SemanticBoardUpgradeSuggestion{{
		Decision:          SemanticBoardUpgradeDecisionCompose,
		BoardLabel:        "落库组合",
		Description:       "落库组合描述",
		AuxiliaryLabelIDs: []uint{auxA.ID, auxB.ID},
	}}}
	service := NewSemanticBoardUpgradeService(db, llm, nil)

	// First run → inserted pending compose row.
	inserted, skipped, blocked, err := service.GenerateAndPersist(context.Background(), UpgradeGenerateRequest{Direction: UpgradeDirectionCreate, Source: UpgradeSourceComposite})
	require.NoError(t, err)
	require.Equal(t, 1, inserted)
	require.Equal(t, 0, skipped)
	require.Equal(t, 0, blocked)

	pending, err := repo.List(context.Background(), "pending", string(SemanticBoardUpgradeDecisionCompose))
	require.NoError(t, err)
	require.Len(t, pending, 1)
	require.Equal(t, "落库组合", pending[0].BoardLabel)
	require.Equal(t, "compose", pending[0].Decision)
	firstHash := pending[0].SuggestionHash

	// Second run, same suggestion → idempotent skip (same hash already pending).
	inserted2, skipped2, blocked2, err := service.GenerateAndPersist(context.Background(), UpgradeGenerateRequest{Direction: UpgradeDirectionCreate, Source: UpgradeSourceComposite})
	require.NoError(t, err)
	require.Equal(t, 0, inserted2)
	require.Equal(t, 1, skipped2)
	require.Equal(t, 0, blocked2)

	// Dismiss, then regenerate within cooldown → cooldownBlocked.
	require.NoError(t, repo.MarkDismissed(context.Background(), pending[0].ID, "不相关"))
	inserted3, skipped3, blocked3, err := service.GenerateAndPersist(context.Background(), UpgradeGenerateRequest{Direction: UpgradeDirectionCreate, Source: UpgradeSourceComposite})
	require.NoError(t, err)
	require.Equal(t, 0, inserted3)
	require.Equal(t, 0, skipped3)
	require.Equal(t, 1, blocked3)

	// Hash stability: same component set (any order) → same hash.
	require.Equal(t, ComputeSuggestionHash("create:composite", "compose", nil, []uint{auxA.ID, auxB.ID}), firstHash)
	require.Equal(t, ComputeSuggestionHash("create:composite", "compose", nil, []uint{auxB.ID, auxA.ID}), firstHash)
	require.NotEqual(t, ComputeSuggestionHash("create:composite", "compose", nil, []uint{auxA.ID}), firstHash)
}

func TestConfirmComposeSuggestion(t *testing.T) {
	db := setupSemanticBoardUpgradeTestDB(t)
	repo := repository.NewBoardUpgradeSuggestionRepository(db)
	auxA := createComposeAux(t, db, "确认甲", "cf-a", 8)
	auxB := createComposeAux(t, db, "确认乙", "cf-b", 8)

	embedder := func(ctx context.Context, input string, mode auxlabel.AuxiliaryLabelEmbeddingMode) (string, []float64, error) {
		vec := testutil.PadVector([]float64{1, 0, 0}, testutil.TestEmbeddingDim)
		return core.FloatsToPgVector(vec), vec, nil
	}
	service := NewSemanticBoardUpgradeService(db, nil, embedder)

	suggestion := &models.BoardUpgradeSuggestion{
		BatchID: "test-batch", Mode: "", Decision: "compose", BoardLabel: "确认组合",
		Description: "确认组合描述", AuxiliaryLabelIDs: []uint{auxA.ID, auxB.ID},
		Status: "pending", SuggestionHash: ComputeSuggestionHash("", "compose", nil, []uint{auxA.ID, auxB.ID}),
	}
	ok, err := repo.InsertPending(context.Background(), suggestion)
	require.NoError(t, err)
	require.True(t, ok)

	result, err := service.ConfirmSuggestion(context.Background(), ConfirmSemanticBoardUpgradeRequest{
		Decision:          SemanticBoardUpgradeDecisionCompose,
		BoardLabel:        "确认组合",
		Description:       "确认组合描述",
		AuxiliaryLabelIDs: []uint{auxA.ID, auxB.ID},
		SuggestionID:      &suggestion.ID,
	})
	require.NoError(t, err)
	require.NotNil(t, result.CompositeLabelID)
	require.Equal(t, uint(0), result.SemanticBoardID)

	// Composite label + components created; suggestion confirmed.
	var composite models.SemanticLabel
	require.NoError(t, db.Where("id = ?", *result.CompositeLabelID).First(&composite).Error)
	require.Equal(t, "composite", composite.LabelType)
	require.Equal(t, "upgrade_suggest", composite.Source)
	require.Equal(t, "active", composite.Status)
	var components []models.CompositeComponent
	require.NoError(t, db.Where("composite_id = ?", composite.ID).Order("position ASC").Find(&components).Error)
	require.Len(t, components, 2)
	var confirmed models.BoardUpgradeSuggestion
	require.NoError(t, db.Where("id = ?", suggestion.ID).First(&confirmed).Error)
	require.Equal(t, "confirmed", confirmed.Status)
}

func TestConfirmComposeSuggestionEmbedderFailureRollsBack(t *testing.T) {
	db := setupSemanticBoardUpgradeTestDB(t)
	repo := repository.NewBoardUpgradeSuggestionRepository(db)
	auxA := createComposeAux(t, db, "回滚甲", "rb-a", 8)
	auxB := createComposeAux(t, db, "回滚乙", "rb-b", 8)

	failingEmbedder := func(ctx context.Context, input string, mode auxlabel.AuxiliaryLabelEmbeddingMode) (string, []float64, error) {
		return "", nil, errors.New("embedder down")
	}
	service := NewSemanticBoardUpgradeService(db, nil, failingEmbedder)

	suggestion := &models.BoardUpgradeSuggestion{
		BatchID: "test-batch-rb", Mode: "", Decision: "compose", BoardLabel: "回滚组合",
		AuxiliaryLabelIDs: []uint{auxA.ID, auxB.ID}, Status: "pending",
		SuggestionHash: ComputeSuggestionHash("", "compose", nil, []uint{auxA.ID, auxB.ID}),
	}
	ok, err := repo.InsertPending(context.Background(), suggestion)
	require.NoError(t, err)
	require.True(t, ok)

	_, err = service.ConfirmSuggestion(context.Background(), ConfirmSemanticBoardUpgradeRequest{
		Decision:          SemanticBoardUpgradeDecisionCompose,
		BoardLabel:        "回滚组合",
		AuxiliaryLabelIDs: []uint{auxA.ID, auxB.ID},
		SuggestionID:      &suggestion.ID,
	})
	require.Error(t, err)

	// No composite created; suggestion stays pending.
	var count int64
	require.NoError(t, db.Model(&models.SemanticLabel{}).Where("label_type = ?", "composite").Count(&count).Error)
	require.Zero(t, count)
	var still models.BoardUpgradeSuggestion
	require.NoError(t, db.Where("id = ?", suggestion.ID).First(&still).Error)
	require.Equal(t, "pending", still.Status)
}

func TestConfirmComposeSuggestionDedupeReuse(t *testing.T) {
	db := setupSemanticBoardUpgradeTestDB(t)
	repo := repository.NewBoardUpgradeSuggestionRepository(db)
	auxA := createComposeAux(t, db, "复用甲", "du-a", 8)
	auxB := createComposeAux(t, db, "复用乙", "du-b", 8)

	// Pre-existing composite with the same component set (L1 reuse path).
	existing := models.SemanticLabel{Label: "已有组合", Slug: "du-existing", LabelType: "composite", Status: "active", Source: "manual"}
	require.NoError(t, db.Create(&existing).Error)
	require.NoError(t, db.Create(&models.CompositeComponent{CompositeID: existing.ID, ComponentLabelID: auxA.ID, Position: 1}).Error)
	require.NoError(t, db.Create(&models.CompositeComponent{CompositeID: existing.ID, ComponentLabelID: auxB.ID, Position: 2}).Error)

	embedder := func(ctx context.Context, input string, mode auxlabel.AuxiliaryLabelEmbeddingMode) (string, []float64, error) {
		vec := testutil.PadVector([]float64{1, 0, 0}, testutil.TestEmbeddingDim)
		return core.FloatsToPgVector(vec), vec, nil
	}
	service := NewSemanticBoardUpgradeService(db, nil, embedder)

	suggestion := &models.BoardUpgradeSuggestion{
		BatchID: "test-batch-du", Mode: "", Decision: "compose", BoardLabel: "复用组合",
		AuxiliaryLabelIDs: []uint{auxA.ID, auxB.ID}, Status: "pending",
		SuggestionHash: ComputeSuggestionHash("", "compose", nil, []uint{auxA.ID, auxB.ID}),
	}
	ok, err := repo.InsertPending(context.Background(), suggestion)
	require.NoError(t, err)
	require.True(t, ok)

	result, err := service.ConfirmSuggestion(context.Background(), ConfirmSemanticBoardUpgradeRequest{
		Decision:          SemanticBoardUpgradeDecisionCompose,
		BoardLabel:        "复用组合",
		AuxiliaryLabelIDs: []uint{auxA.ID, auxB.ID},
		SuggestionID:      &suggestion.ID,
	})
	require.NoError(t, err)
	require.NotNil(t, result.CompositeLabelID)
	require.Equal(t, existing.ID, *result.CompositeLabelID, "L1 dedupe must reuse the existing composite")

	// No duplicate composite created; suggestion still confirmed.
	var count int64
	require.NoError(t, db.Model(&models.SemanticLabel{}).Where("label_type = ?", "composite").Count(&count).Error)
	require.Equal(t, int64(1), count)
	var confirmed models.BoardUpgradeSuggestion
	require.NoError(t, db.Where("id = ?", suggestion.ID).First(&confirmed).Error)
	require.Equal(t, "confirmed", confirmed.Status)
}

// TestFilterExistingComposeCandidates（2026-09-07 报障：确认创建过的组合重复出现在
// 建议列表）：已存在组合的候选不再入列——create 路整体排除；expand 路已挂载目标
// 版块的排除、未挂载的保留（挂载语义）；部分重叠（非完全一致）不受影响。
func TestFilterExistingComposeCandidates(t *testing.T) {
	db := setupSemanticBoardUpgradeTestDB(t)
	embedder := func(ctx context.Context, input string, mode auxlabel.AuxiliaryLabelEmbeddingMode) (string, []float64, error) {
		vec := testutil.PadVector([]float64{1, 0, 0}, testutil.TestEmbeddingDim)
		return core.FloatsToPgVector(vec), vec, nil
	}
	service := NewSemanticBoardUpgradeService(db, nil, embedder)

	auxA := createComposeAux(t, db, "组甲", "fx-a", 8)
	auxB := createComposeAux(t, db, "组乙", "fx-b", 8)
	auxC := createComposeAux(t, db, "组丙", "fx-c", 8)

	// 既有组合 [A,B]。另建 [A,B,C] 三元组合验证部分重叠不误伤。
	existing := models.SemanticLabel{Label: "既有组合", Slug: "fx-existing", LabelType: "composite", Status: "active", Source: "manual"}
	require.NoError(t, db.Create(&existing).Error)
	require.NoError(t, db.Create(&models.CompositeComponent{CompositeID: existing.ID, ComponentLabelID: auxA.ID, Position: 1}).Error)
	require.NoError(t, db.Create(&models.CompositeComponent{CompositeID: existing.ID, ComponentLabelID: auxB.ID, Position: 2}).Error)
	existingTriple := models.SemanticLabel{Label: "既有三元组合", Slug: "fx-existing-triple", LabelType: "composite", Status: "active", Source: "manual"}
	require.NoError(t, db.Create(&existingTriple).Error)
	require.NoError(t, db.Create(&models.CompositeComponent{CompositeID: existingTriple.ID, ComponentLabelID: auxA.ID, Position: 1}).Error)
	require.NoError(t, db.Create(&models.CompositeComponent{CompositeID: existingTriple.ID, ComponentLabelID: auxB.ID, Position: 2}).Error)
	require.NoError(t, db.Create(&models.CompositeComponent{CompositeID: existingTriple.ID, ComponentLabelID: auxC.ID, Position: 3}).Error)

	candidates := []ComposeCandidate{
		{ComponentIDs: []uint{auxA.ID, auxB.ID}, Cooccurrence: 20}, // 与既有二元组合完全一致
		{ComponentIDs: []uint{auxA.ID, auxC.ID}, Cooccurrence: 15}, // 部分重叠（非完全一致），不算重复
	}

	// create 路（targetBoardID=nil）：[A,B] 排除，[A,C] 保留。
	got, err := service.filterExistingComposeCandidates(context.Background(), candidates, nil)
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, []uint{auxA.ID, auxC.ID}, got[0].ComponentIDs)

	// expand 路：组合存在但未挂载目标版块 → 保留（挂载语义，确认时复用）。
	board := models.SemanticLabel{Label: "目标版块", Slug: "fx-board", LabelType: "board", Status: "active", Source: "manual"}
	require.NoError(t, db.Create(&board).Error)
	got, err = service.filterExistingComposeCandidates(context.Background(), candidates, &board.ID)
	require.NoError(t, err)
	require.Len(t, got, 2)

	// expand 路：组合已挂载目标版块 → 排除。
	require.NoError(t, db.Create(&models.BoardComposition{BoardID: board.ID, AuxiliaryLabelID: existing.ID}).Error)
	got, err = service.filterExistingComposeCandidates(context.Background(), candidates, &board.ID)
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, []uint{auxA.ID, auxC.ID}, got[0].ComponentIDs)
}
