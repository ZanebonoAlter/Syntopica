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
	require.NoError(t, db.AutoMigrate(&models.TopicTag{}, &models.SemanticLabel{}, &models.TopicTagSemanticLabel{}))
	return db
}

type recordingAuxiliaryEmbedder struct {
	calls   []string
	vectors map[string][]float64
}

func (e *recordingAuxiliaryEmbedder) embed(ctx context.Context, label string) (string, []float64, error) {
	e.calls = append(e.calls, label)
	if e.vectors != nil {
		if vec, ok := e.vectors[label]; ok {
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

	label, err := service.ResolveAuxiliaryLabel(context.Background(), "Open AI")

	require.NoError(t, err)
	require.Equal(t, existing.ID, label.ID)
	require.Empty(t, embedder.calls)
}

func TestAuxiliaryLabelServiceExcludesDisabledLabels(t *testing.T) {
	db := setupAuxiliaryLabelTestDB(t)
	disabled := models.SemanticLabel{Label: "OpenAI", Slug: "openai", LabelType: "auxiliary", Status: "disabled", Aliases: []string{"Open AI"}}
	require.NoError(t, db.Create(&disabled).Error)
	embedder := &recordingAuxiliaryEmbedder{vectors: map[string][]float64{"OpenAI": {0, 1, 0}}}
	service := NewAuxiliaryLabelService(db, embedder.embed)

	label, err := service.ResolveAuxiliaryLabel(context.Background(), "OpenAI")

	require.NoError(t, err)
	require.NotEqual(t, disabled.ID, label.ID)
	require.Equal(t, "active", label.Status)
	require.Equal(t, "auxiliary", label.LabelType)
	require.Equal(t, []string{"OpenAI"}, embedder.calls)
}

func TestAuxiliaryLabelServiceL2MergeKeepsHigherRefCountAndAppendsAlias(t *testing.T) {
	db := setupAuxiliaryLabelTestDB(t)
	existingVec := floatsToPgVector([]float64{1, 0, 0})
	lowRef := models.SemanticLabel{Label: "GPT-5", Slug: "gpt-5", LabelType: "auxiliary", Status: "active", RefCount: 1, Embedding: &existingVec}
	require.NoError(t, db.Create(&lowRef).Error)
	highRef := models.SemanticLabel{Label: "GPT-5 High Ref", Slug: "gpt-5-high-ref", LabelType: "auxiliary", Status: "active", RefCount: 9, Embedding: &existingVec}
	require.NoError(t, db.Create(&highRef).Error)
	embedder := &recordingAuxiliaryEmbedder{vectors: map[string][]float64{"GPT 5": {0.999, 0.001, 0}}}
	service := NewAuxiliaryLabelService(db, embedder.embed)

	label, err := service.ResolveAuxiliaryLabel(context.Background(), "GPT 5")

	require.NoError(t, err)
	require.Equal(t, highRef.ID, label.ID)
	require.Contains(t, label.Aliases, "GPT 5")

	var reloaded models.SemanticLabel
	require.NoError(t, db.First(&reloaded, highRef.ID).Error)
	require.Contains(t, reloaded.Aliases, "GPT 5")
}

func TestAuxiliaryLabelServiceL3CreatesAuxiliaryLabelWithEmbedding(t *testing.T) {
	db := setupAuxiliaryLabelTestDB(t)
	embedder := &recordingAuxiliaryEmbedder{vectors: map[string][]float64{"多模态模型": {0, 1, 0}}}
	service := NewAuxiliaryLabelService(db, embedder.embed)

	label, err := service.ResolveAuxiliaryLabel(context.Background(), "多模态模型")

	require.NoError(t, err)
	require.NotZero(t, label.ID)
	require.Equal(t, "多模态模型", label.Label)
	require.Equal(t, "auxiliary", label.LabelType)
	require.Equal(t, "llm_extract", label.Source)
	require.Equal(t, "active", label.Status)
	require.NotNil(t, label.Embedding)
}

func TestAuxiliaryLabelServiceAttachAuxiliaryLabelsIncrementsRefCountOnce(t *testing.T) {
	db := setupAuxiliaryLabelTestDB(t)
	tag := models.TopicTag{Label: "OpenAI 发布 GPT-5", Slug: "openai-gpt-5", Category: "event", Status: "active"}
	require.NoError(t, db.Create(&tag).Error)
	existing := models.SemanticLabel{Label: "OpenAI", Slug: "openai", LabelType: "auxiliary", Status: "active"}
	require.NoError(t, db.Create(&existing).Error)
	service := NewAuxiliaryLabelService(db, (&recordingAuxiliaryEmbedder{}).embed)

	labels := []string{"OpenAI", "GPT-5", "模型发布"}
	require.NoError(t, service.AttachAuxiliaryLabels(context.Background(), tag.ID, labels))
	require.NoError(t, service.AttachAuxiliaryLabels(context.Background(), tag.ID, labels))

	var count int64
	require.NoError(t, db.Model(&models.TopicTagSemanticLabel{}).Where("topic_tag_id = ? AND semantic_label_id = ?", tag.ID, existing.ID).Count(&count).Error)
	require.Equal(t, int64(1), count)

	var reloaded models.SemanticLabel
	require.NoError(t, db.First(&reloaded, existing.ID).Error)
	require.Equal(t, 1, reloaded.RefCount)
}

func TestValidateAuxiliaryLabelsRejectsLowQualityLabels(t *testing.T) {
	require.NoError(t, validateAuxiliaryLabels([]string{"OpenAI", "GPT-5", "模型发布"}))
	require.Error(t, validateAuxiliaryLabels([]string{"OpenAI", "技术", "模型发布"}))
	require.Error(t, validateAuxiliaryLabels([]string{"OpenAI", "", "模型发布"}))
	require.Error(t, validateAuxiliaryLabels([]string{"OpenAI", "GPT-5"}))
}
