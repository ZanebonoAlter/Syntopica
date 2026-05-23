package tagging

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"my-robot-backend/internal/domain/models"
	"my-robot-backend/internal/platform/database"
)

func setupAuxiliaryLabelTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.Exec("PRAGMA foreign_keys = ON").Error)
	database.DB = db
	require.NoError(t, db.AutoMigrate(&models.TopicTag{}, &models.SemanticLabel{}, &models.TopicTagSemanticLabel{}, &models.TopicTagBoardLabel{}, &models.BoardComposition{}))
	return db
}

type recordingAuxiliaryEmbedder struct {
	calls   []string
	vectors map[string][]float64
}

func (e *recordingAuxiliaryEmbedder) embed(ctx context.Context, input string, mode auxiliaryLabelEmbeddingMode) (string, []float64, error) {
	e.calls = append(e.calls, input)
	if e.vectors != nil {
		if vec, ok := e.vectors[input]; ok {
			return floatsToPgVector(vec), vec, nil
		}
	}
	vec := []float64{1, 0, 0}
	return floatsToPgVector(vec), vec, nil
}

func TestAuxiliaryLabelServiceL1SlugAndAliasExactMatch(t *testing.T) {
	db := setupAuxiliaryLabelTestDB(t)
	existing := models.SemanticLabel{Label: "OpenAI", Slug: "openai", LabelType: "auxiliary", Status: "active", Aliases: []string{"Open AI"}}
	require.NoError(t, db.Create(&existing).Error)
	embedder := &recordingAuxiliaryEmbedder{}
	service := NewAuxiliaryLabelService(db, embedder.embed)

	label, err := service.ResolveAuxiliaryLabel(context.Background(), "Open AI", "")

	require.NoError(t, err)
	require.Equal(t, existing.ID, label.ID)
	require.Empty(t, embedder.calls)
}

func TestAuxiliaryLabelServiceExcludesDisabledLabels(t *testing.T) {
	db := setupAuxiliaryLabelTestDB(t)
	disabled := models.SemanticLabel{Label: "OpenAI", Slug: "openai", LabelType: "auxiliary", Status: "disabled", Aliases: []string{"Open AI"}}
	require.NoError(t, db.Create(&disabled).Error)
	disabledVec := floatsToPgVector([]float64{1, 0, 0})
	disabledCandidate := models.SemanticLabel{Label: "OpenAI Candidate", Slug: "openai-candidate", LabelType: "auxiliary", Status: "disabled", Embedding: &disabledVec}
	require.NoError(t, db.Create(&disabledCandidate).Error)
	embedder := &recordingAuxiliaryEmbedder{vectors: map[string][]float64{"OpenAI": {0, 1, 0}}}
	service := NewAuxiliaryLabelService(db, embedder.embed)

	label, err := service.ResolveAuxiliaryLabel(context.Background(), "OpenAI", "")

	require.NoError(t, err)
	require.NotEqual(t, disabled.ID, label.ID)
	require.Equal(t, "active", label.Status)
	require.Equal(t, "auxiliary", label.LabelType)
	require.Equal(t, []string{"OpenAI", "OpenAI"}, embedder.calls)
}

func TestAuxiliaryLabelServiceDisableAuxiliaryLabelMarksOnlyAuxiliaryLabels(t *testing.T) {
	db := setupAuxiliaryLabelTestDB(t)
	auxiliary := models.SemanticLabel{Label: "OpenAI", Slug: "openai", LabelType: "auxiliary", Status: "active"}
	require.NoError(t, db.Create(&auxiliary).Error)
	board := models.SemanticLabel{Label: "AI Board", Slug: "ai-board", LabelType: "board", Status: "active"}
	require.NoError(t, db.Create(&board).Error)
	service := NewAuxiliaryLabelService(db, (&recordingAuxiliaryEmbedder{}).embed)

	require.NoError(t, service.DisableAuxiliaryLabel(context.Background(), auxiliary.ID))
	require.Error(t, service.DisableAuxiliaryLabel(context.Background(), board.ID))

	var reloadedAuxiliary models.SemanticLabel
	require.NoError(t, db.First(&reloadedAuxiliary, auxiliary.ID).Error)
	require.Equal(t, "disabled", reloadedAuxiliary.Status)

	var reloadedBoard models.SemanticLabel
	require.NoError(t, db.First(&reloadedBoard, board.ID).Error)
	require.Equal(t, "active", reloadedBoard.Status)
}

func TestAuxiliaryLabelServiceMergeAuxiliaryLabelAliasMigratesLinksAndPreservesBoardLabels(t *testing.T) {
	db := setupAuxiliaryLabelTestDB(t)
	tagA := models.TopicTag{Label: "OpenAI 发布 GPT-5", Slug: "openai-gpt-5", Category: "event", Status: "active"}
	tagB := models.TopicTag{Label: "GPT-5 模型评测", Slug: "gpt-5-review", Category: "event", Status: "active"}
	require.NoError(t, db.Create(&tagA).Error)
	require.NoError(t, db.Create(&tagB).Error)
	target := models.SemanticLabel{Label: "OpenAI", Slug: "openai", LabelType: "auxiliary", Status: "active", Aliases: []string{"Open AI"}, RefCount: 1}
	require.NoError(t, db.Create(&target).Error)
	source := models.SemanticLabel{Label: "Open AI", Slug: "open-ai", LabelType: "auxiliary", Status: "active", Aliases: []string{"openai", "ChatGPT"}, RefCount: 2}
	require.NoError(t, db.Create(&source).Error)
	board := models.SemanticLabel{Label: "AI Board", Slug: "ai-board", LabelType: "board", Status: "active"}
	require.NoError(t, db.Create(&board).Error)
	require.NoError(t, db.Create(&models.TopicTagSemanticLabel{TopicTagID: tagA.ID, SemanticLabelID: target.ID}).Error)
	require.NoError(t, db.Create(&models.TopicTagSemanticLabel{TopicTagID: tagA.ID, SemanticLabelID: source.ID}).Error)
	require.NoError(t, db.Create(&models.TopicTagSemanticLabel{TopicTagID: tagB.ID, SemanticLabelID: source.ID}).Error)
	require.NoError(t, db.Create(&models.TopicTagBoardLabel{TopicTagID: tagA.ID, SemanticBoardID: board.ID, Score: 0.8, MatchReason: "existing"}).Error)
	service := NewAuxiliaryLabelService(db, (&recordingAuxiliaryEmbedder{}).embed)

	require.NoError(t, service.MergeAuxiliaryLabelAlias(context.Background(), source.ID, target.ID))

	var targetLinks int64
	require.NoError(t, db.Model(&models.TopicTagSemanticLabel{}).Where("semantic_label_id = ?", target.ID).Count(&targetLinks).Error)
	require.Equal(t, int64(2), targetLinks)
	var sourceLinks int64
	require.NoError(t, db.Model(&models.TopicTagSemanticLabel{}).Where("semantic_label_id = ?", source.ID).Count(&sourceLinks).Error)
	require.Zero(t, sourceLinks)

	var reloadedTarget models.SemanticLabel
	require.NoError(t, db.First(&reloadedTarget, target.ID).Error)
	require.Equal(t, 2, reloadedTarget.RefCount)
	require.Contains(t, reloadedTarget.Aliases, "ChatGPT")
	require.Len(t, reloadedTarget.Aliases, 2)

	var reloadedSource models.SemanticLabel
	require.NoError(t, db.First(&reloadedSource, source.ID).Error)
	require.Equal(t, "disabled", reloadedSource.Status)
	require.Zero(t, reloadedSource.RefCount)

	var boardLabelCount int64
	require.NoError(t, db.Model(&models.TopicTagBoardLabel{}).Count(&boardLabelCount).Error)
	require.Equal(t, int64(1), boardLabelCount)
}

func TestAuxiliaryLabelServiceRemoveBoardCompositionDeletesOnlyRequestedRow(t *testing.T) {
	db := setupAuxiliaryLabelTestDB(t)
	board := models.SemanticLabel{Label: "AI Board", Slug: "ai-board", LabelType: "board", Status: "active"}
	require.NoError(t, db.Create(&board).Error)
	otherBoard := models.SemanticLabel{Label: "Hardware Board", Slug: "hardware-board", LabelType: "board", Status: "active"}
	require.NoError(t, db.Create(&otherBoard).Error)
	auxiliary := models.SemanticLabel{Label: "OpenAI", Slug: "openai", LabelType: "auxiliary", Status: "active"}
	require.NoError(t, db.Create(&auxiliary).Error)
	otherAuxiliary := models.SemanticLabel{Label: "GPU", Slug: "gpu", LabelType: "auxiliary", Status: "active"}
	require.NoError(t, db.Create(&otherAuxiliary).Error)
	tag := models.TopicTag{Label: "OpenAI 发布 GPT-5", Slug: "openai-gpt-5", Category: "event", Status: "active"}
	require.NoError(t, db.Create(&tag).Error)
	require.NoError(t, db.Create(&models.BoardComposition{BoardID: board.ID, AuxiliaryLabelID: auxiliary.ID}).Error)
	require.NoError(t, db.Create(&models.BoardComposition{BoardID: board.ID, AuxiliaryLabelID: otherAuxiliary.ID}).Error)
	require.NoError(t, db.Create(&models.BoardComposition{BoardID: otherBoard.ID, AuxiliaryLabelID: auxiliary.ID}).Error)
	require.NoError(t, db.Create(&models.TopicTagBoardLabel{TopicTagID: tag.ID, SemanticBoardID: board.ID, Score: 0.8, MatchReason: "existing"}).Error)
	service := NewAuxiliaryLabelService(db, (&recordingAuxiliaryEmbedder{}).embed)

	require.NoError(t, service.RemoveBoardComposition(context.Background(), board.ID, auxiliary.ID))

	var requested int64
	require.NoError(t, db.Model(&models.BoardComposition{}).Where("board_id = ? AND auxiliary_label_id = ?", board.ID, auxiliary.ID).Count(&requested).Error)
	require.Zero(t, requested)
	var remaining int64
	require.NoError(t, db.Model(&models.BoardComposition{}).Count(&remaining).Error)
	require.Equal(t, int64(2), remaining)
	var boardLabelCount int64
	require.NoError(t, db.Model(&models.TopicTagBoardLabel{}).Count(&boardLabelCount).Error)
	require.Equal(t, int64(1), boardLabelCount)
}

func TestAuxiliaryLabelServiceL2MergeKeepsHigherRefCountAndAppendsAlias(t *testing.T) {
	db := setupAuxiliaryLabelTestDB(t)
	existingVec := floatsToPgVector([]float64{1, 0, 0})
	lowRef := models.SemanticLabel{Label: "GPT-5", Slug: "gpt-5", LabelType: "auxiliary", Status: "active", RefCount: 1, MergeEmbedding: &existingVec}
	require.NoError(t, db.Create(&lowRef).Error)
	highRef := models.SemanticLabel{Label: "GPT-5 High Ref", Slug: "gpt-5-high-ref", LabelType: "auxiliary", Status: "active", RefCount: 9, MergeEmbedding: &existingVec}
	require.NoError(t, db.Create(&highRef).Error)
	embedder := &recordingAuxiliaryEmbedder{vectors: map[string][]float64{"GPT 5": {0.999, 0.001, 0}}}
	service := NewAuxiliaryLabelService(db, embedder.embed)

	label, err := service.ResolveAuxiliaryLabel(context.Background(), "GPT 5", "")

	require.NoError(t, err)
	require.Equal(t, highRef.ID, label.ID)
	require.Contains(t, label.Aliases, "GPT 5")

	var reloaded models.SemanticLabel
	require.NoError(t, db.First(&reloaded, highRef.ID).Error)
	require.Contains(t, reloaded.Aliases, "GPT 5")
}

func TestAuxiliaryLabelServiceL3CreatesAuxiliaryLabelWithEmbedding(t *testing.T) {
	db := setupAuxiliaryLabelTestDB(t)
	embedder := &recordingAuxiliaryEmbedder{vectors: map[string][]float64{"多模态模型": {0, 1, 0}, "多模态模型: AI技术术语": {0, 0.9, 0.1}}}
	service := NewAuxiliaryLabelService(db, embedder.embed)

	label, err := service.ResolveAuxiliaryLabel(context.Background(), "多模态模型", "AI技术术语")

	require.NoError(t, err)
	require.NotZero(t, label.ID)
	require.Equal(t, "多模态模型", label.Label)
	require.Equal(t, "auxiliary", label.LabelType)
	require.Equal(t, "llm_extract", label.Source)
	require.Equal(t, "active", label.Status)
	require.Equal(t, "AI技术术语", label.Description)
	require.NotNil(t, label.Embedding)
	require.NotNil(t, label.MergeEmbedding)
	require.NotEqual(t, *label.Embedding, *label.MergeEmbedding)
}

func TestAuxiliaryLabelServiceAttachAuxiliaryLabelsIncrementsRefCountOnce(t *testing.T) {
	db := setupAuxiliaryLabelTestDB(t)
	tag := models.TopicTag{Label: "OpenAI 发布 GPT-5", Slug: "openai-gpt-5", Category: "event", Status: "active"}
	require.NoError(t, db.Create(&tag).Error)
	existing := models.SemanticLabel{Label: "OpenAI", Slug: "openai", LabelType: "auxiliary", Status: "active"}
	require.NoError(t, db.Create(&existing).Error)
	service := NewAuxiliaryLabelService(db, (&recordingAuxiliaryEmbedder{}).embed)

	labels := []AuxiliaryLabel{{Label: "OpenAI", Description: "人工智能公司"}, {Label: "GPT-5", Description: "大语言模型"}, {Label: "模型发布", Description: "新产品发布"}}
	require.NoError(t, service.AttachAuxiliaryLabels(context.Background(), tag.ID, labels))
	require.NoError(t, service.AttachAuxiliaryLabels(context.Background(), tag.ID, labels))

	var count int64
	require.NoError(t, db.Model(&models.TopicTagSemanticLabel{}).Where("topic_tag_id = ? AND semantic_label_id = ?", tag.ID, existing.ID).Count(&count).Error)
	require.Equal(t, int64(1), count)

	var reloaded models.SemanticLabel
	require.NoError(t, db.First(&reloaded, existing.ID).Error)
	require.Equal(t, 1, reloaded.RefCount)
}
