package tagging

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"syntopica-backend/internal/domain/models"
	"syntopica-backend/internal/platform/database"
)

type fakeSemanticBoardUpgradeLLM struct {
	prompt      string
	suggestions []SemanticBoardUpgradeSuggestion
	calls       int
}

var upgradeFeedSeq uint64

func (f *fakeSemanticBoardUpgradeLLM) SuggestSemanticBoardUpgrades(ctx context.Context, prompt string) ([]SemanticBoardUpgradeSuggestion, error) {
	f.calls++
	f.prompt = prompt
	return f.suggestions, nil
}

func setupSemanticBoardUpgradeTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.Exec("PRAGMA foreign_keys = ON").Error)
	database.DB = db
	require.NoError(t, db.AutoMigrate(&models.Feed{}, &models.Article{}, &models.TopicTag{}, &models.TopicTagEmbedding{}, &models.ArticleTopicTag{}, &models.SemanticLabel{}, &models.TopicTagSemanticLabel{}, &models.TopicTagBoardLabel{}, &models.BoardComposition{}, &models.AISettings{}))
	return db
}

func TestSemanticBoardUpgradeCollectsCandidates(t *testing.T) {
	db := setupSemanticBoardUpgradeTestDB(t)
	include := createUpgradeLabel(t, db, "Included", "included", "auxiliary", "active", 5, []float64{1, 0, 0})
	createUpgradeLabel(t, db, "Below", "below", "auxiliary", "active", 4, []float64{1, 0, 0})
	createUpgradeLabel(t, db, "Disabled", "disabled", "auxiliary", "disabled", 8, []float64{1, 0, 0})
	createUpgradeLabel(t, db, "No Embedding", "no-embedding", "auxiliary", "active", 8, nil)
	composed := createUpgradeLabel(t, db, "Composed", "composed", "auxiliary", "active", 8, []float64{0, 1, 0})
	board := createUpgradeLabel(t, db, "Board", "board", "board", "active", 0, nil)
	require.NoError(t, db.Create(&models.BoardComposition{BoardID: board.ID, AuxiliaryLabelID: composed.ID}).Error)
	service := NewSemanticBoardUpgradeService(db, nil)

	candidates, err := service.collectCandidates(context.Background(), service.LoadUpgradeConfig(context.Background()))

	require.NoError(t, err)
	require.Len(t, candidates, 1)
	require.Equal(t, include.ID, candidates[0].ID)
	require.Equal(t, []float64{1, 0, 0}, candidates[0].Embedding)
}

func TestSemanticBoardUpgradeClustersCandidatesWithExistingBoards(t *testing.T) {
	db := setupSemanticBoardUpgradeTestDB(t)
	candidateA := createUpgradeLabel(t, db, "OpenAI", "openai", "auxiliary", "active", 5, []float64{1, 0, 0})
	candidateB := createUpgradeLabel(t, db, "GPT", "gpt", "auxiliary", "active", 5, []float64{0.95, 0.3122498999, 0})
	candidateC := createUpgradeLabel(t, db, "Battery", "battery", "auxiliary", "active", 5, []float64{0, 1, 0})
	boardAux := createUpgradeLabel(t, db, "AI", "ai", "auxiliary", "active", 2, []float64{1, 0, 0})
	board := createUpgradeLabel(t, db, "AI Board", "ai-board", "board", "active", 0, nil)
	require.NoError(t, db.Create(&models.BoardComposition{BoardID: board.ID, AuxiliaryLabelID: boardAux.ID}).Error)
	service := NewSemanticBoardUpgradeService(db, nil)
	candidates := []SemanticBoardUpgradeCandidate{
		{ID: candidateA.ID, Label: candidateA.Label, RefCount: 5, Embedding: []float64{1, 0, 0}},
		{ID: candidateB.ID, Label: candidateB.Label, RefCount: 5, Embedding: []float64{0.95, 0.3122498999, 0}},
		{ID: candidateC.ID, Label: candidateC.Label, RefCount: 5, Embedding: []float64{0, 1, 0}},
	}

	clusters, err := service.clusterCandidates(context.Background(), candidates, service.LoadUpgradeConfig(context.Background()))

	require.NoError(t, err)
	require.Len(t, clusters, 2)
	require.NotNil(t, clusters[0].ExistingBoardID)
	require.Equal(t, board.ID, *clusters[0].ExistingBoardID)
	require.Equal(t, []uint{candidateA.ID, candidateB.ID}, upgradeCandidateIDs(clusters[0].Candidates))
	require.Nil(t, clusters[1].ExistingBoardID)
	require.Equal(t, []uint{candidateC.ID}, upgradeCandidateIDs(clusters[1].Candidates))
}

func TestSemanticBoardUpgradeLoadsCoTagEventContext(t *testing.T) {
	db := setupSemanticBoardUpgradeTestDB(t)
	require.NoError(t, db.Create(&models.AISettings{Key: "semantic_board_upgrade_cotag_hard_limit", Value: "2"}).Error)
	auxiliary := createUpgradeLabel(t, db, "OpenAI", "openai", "auxiliary", "active", 5, []float64{1, 0, 0})
	seed := createUpgradeTopicTag(t, db, "seed", models.TagCategoryKeyword)
	eventA := createUpgradeTopicTag(t, db, "Launch", models.TagCategoryEvent)
	eventB := createUpgradeTopicTag(t, db, "Release", models.TagCategoryEvent)
	eventSimilar := createUpgradeTopicTag(t, db, "Similar Launch", models.TagCategoryEvent)
	eventC := createUpgradeTopicTag(t, db, "Conference", models.TagCategoryEvent)
	createUpgradeTopicEmbedding(t, db, eventA.ID, []float64{1, 0, 0})
	createUpgradeTopicEmbedding(t, db, eventSimilar.ID, []float64{0.99, 0.1410673598, 0})
	createUpgradeTopicEmbedding(t, db, eventB.ID, []float64{0, 1, 0})
	createUpgradeTopicEmbedding(t, db, eventC.ID, []float64{0, 0, 1})
	require.NoError(t, db.Create(&models.TopicTagSemanticLabel{TopicTagID: seed.ID, SemanticLabelID: auxiliary.ID}).Error)
	createUpgradeArticleWithTags(t, db, seed.ID, eventA.ID, eventB.ID)
	createUpgradeArticleWithTags(t, db, seed.ID, eventA.ID, eventSimilar.ID)
	createUpgradeArticleWithTags(t, db, seed.ID, eventSimilar.ID, eventC.ID)
	service := NewSemanticBoardUpgradeService(db, nil)
	cluster := SemanticBoardUpgradeCluster{Candidates: []SemanticBoardUpgradeCandidate{{ID: auxiliary.ID, Label: auxiliary.Label, Embedding: []float64{1, 0, 0}}}}

	events, err := service.loadCoTagEventContext(context.Background(), cluster, service.LoadUpgradeConfig(context.Background()))

	require.NoError(t, err)
	require.Len(t, events, 2)
	require.Equal(t, eventA.ID, events[0].TopicTagID)
	require.Equal(t, 2, events[0].Frequency)
	require.Equal(t, eventB.ID, events[1].TopicTagID)
}

func TestSemanticBoardUpgradeGenerateSuggestionsUsesLLMMock(t *testing.T) {
	db := setupSemanticBoardUpgradeTestDB(t)
	auxiliaryA := createUpgradeLabel(t, db, "OpenAI", "openai", "auxiliary", "active", 5, []float64{1, 0, 0})
	auxiliaryB := createUpgradeLabel(t, db, "GPT", "gpt", "auxiliary", "active", 5, []float64{0.95, 0.3122498999, 0})
	createUpgradeLabel(t, db, "Transformer", "transformer", "auxiliary", "active", 5, []float64{0.9, 0.4358898943, 0})
	createUpgradeLabel(t, db, "LLM", "llm", "auxiliary", "active", 5, []float64{0.85, 0.5267826876, 0})
	createUpgradeLabel(t, db, "Deep Learning", "deep-learning", "auxiliary", "active", 5, []float64{0.8, 0.6, 0})
	fakeLLM := &fakeSemanticBoardUpgradeLLM{suggestions: []SemanticBoardUpgradeSuggestion{
		{Decision: SemanticBoardUpgradeDecisionCreateNew, BoardLabel: "AI", AuxiliaryLabelIDs: []uint{auxiliaryA.ID, auxiliaryB.ID}},
		{Decision: SemanticBoardUpgradeDecisionSkip, Reason: "too broad"},
		{Decision: "invalid", AuxiliaryLabelIDs: []uint{auxiliaryA.ID}},
		{Decision: SemanticBoardUpgradeDecisionCreateNew, BoardLabel: "Unknown", AuxiliaryLabelIDs: []uint{99999}},
	}}
	service := NewSemanticBoardUpgradeService(db, fakeLLM)

	suggestions, err := service.GenerateSuggestions(context.Background())

	require.NoError(t, err)
	require.Len(t, suggestions, 2)
	require.Equal(t, 1, fakeLLM.calls)
	require.Contains(t, fakeLLM.prompt, "OpenAI")
	require.Contains(t, fakeLLM.prompt, "GPT")
	var boardCount int64
	require.NoError(t, db.Model(&models.SemanticLabel{}).Where("label_type = ?", "board").Count(&boardCount).Error)
	require.Zero(t, boardCount)
	var compositionCount int64
	require.NoError(t, db.Model(&models.BoardComposition{}).Count(&compositionCount).Error)
	require.Zero(t, compositionCount)
}

func TestSemanticBoardUpgradeGenerateSuggestionsSkipsWhenCandidateCountBelowThreshold(t *testing.T) {
	db := setupSemanticBoardUpgradeTestDB(t)
	createUpgradeLabel(t, db, "OpenAI", "openai", "auxiliary", "active", 5, []float64{1, 0, 0})
	createUpgradeLabel(t, db, "GPT", "gpt", "auxiliary", "active", 5, []float64{0.95, 0.3122498999, 0})
	createUpgradeLabel(t, db, "Transformer", "transformer", "auxiliary", "active", 5, []float64{0.9, 0.4358898943, 0})
	fakeLLM := &fakeSemanticBoardUpgradeLLM{suggestions: []SemanticBoardUpgradeSuggestion{{Decision: SemanticBoardUpgradeDecisionCreateNew}}}
	service := NewSemanticBoardUpgradeService(db, fakeLLM)

	suggestions, err := service.GenerateSuggestions(context.Background())

	require.NoError(t, err)
	require.Empty(t, suggestions)
	require.Zero(t, fakeLLM.calls)
}

func TestSemanticBoardUpgradePromptIncludesExistingBoardContext(t *testing.T) {
	db := setupSemanticBoardUpgradeTestDB(t)
	createUpgradeLabel(t, db, "OpenAI", "openai", "auxiliary", "active", 5, []float64{1, 0, 0})
	createUpgradeLabel(t, db, "GPT", "gpt", "auxiliary", "active", 5, []float64{0.95, 0.3122498999, 0})
	createUpgradeLabel(t, db, "Transformer", "transformer", "auxiliary", "active", 5, []float64{0.9, 0.4358898943, 0})
	createUpgradeLabel(t, db, "LLM", "llm", "auxiliary", "active", 5, []float64{0.85, 0.5267826876, 0})
	createUpgradeLabel(t, db, "Deep Learning", "deep-learning", "auxiliary", "active", 5, []float64{0.8, 0.6, 0})
	boardAux := createUpgradeLabel(t, db, "AI", "ai", "auxiliary", "active", 2, []float64{1, 0, 0})
	board := createUpgradeLabel(t, db, "AI Board", "ai-board", "board", "active", 0, nil)
	require.NoError(t, db.Model(&models.SemanticLabel{}).Where("id = ?", board.ID).Update("description", "Artificial intelligence board").Error)
	require.NoError(t, db.Create(&models.BoardComposition{BoardID: board.ID, AuxiliaryLabelID: boardAux.ID}).Error)
	fakeLLM := &fakeSemanticBoardUpgradeLLM{suggestions: []SemanticBoardUpgradeSuggestion{{Decision: SemanticBoardUpgradeDecisionSkip}}}
	service := NewSemanticBoardUpgradeService(db, fakeLLM)

	_, err := service.GenerateSuggestions(context.Background())

	require.NoError(t, err)
	require.Contains(t, fakeLLM.prompt, "关联已有板块：AI Board")
	require.Contains(t, fakeLLM.prompt, "Artificial intelligence board")
	require.Contains(t, fakeLLM.prompt, "板块现有构成标签")
	require.Contains(t, fakeLLM.prompt, "AI")
}

func TestSemanticBoardUpgradeConfirmCreateNew(t *testing.T) {
	db := setupSemanticBoardUpgradeTestDB(t)
	auxiliaryA := createUpgradeLabel(t, db, "OpenAI", "openai", "auxiliary", "active", 5, []float64{1, 0, 0})
	auxiliaryB := createUpgradeLabel(t, db, "GPT", "gpt", "auxiliary", "active", 5, []float64{0, 1, 0})
	service := NewSemanticBoardUpgradeService(db, nil)

	result, err := service.ConfirmSuggestion(context.Background(), ConfirmSemanticBoardUpgradeRequest{
		Decision:          SemanticBoardUpgradeDecisionCreateNew,
		BoardLabel:        "AI Models",
		Description:       "AI model ecosystem",
		AuxiliaryLabelIDs: []uint{auxiliaryB.ID, auxiliaryA.ID, auxiliaryA.ID},
	})

	require.NoError(t, err)
	require.NotZero(t, result.SemanticBoardID)
	require.Equal(t, []uint{auxiliaryA.ID, auxiliaryB.ID}, result.AuxiliaryLabelIDs)
	var board models.SemanticLabel
	require.NoError(t, db.First(&board, result.SemanticBoardID).Error)
	require.Equal(t, "board", board.LabelType)
	require.Equal(t, "llm_suggest", board.Source)
	require.Equal(t, "active", board.Status)
	require.Equal(t, "AI model ecosystem", board.Description)
	var rows []models.BoardComposition
	require.NoError(t, db.Order("auxiliary_label_id ASC").Find(&rows).Error)
	require.Len(t, rows, 2)
	require.Equal(t, auxiliaryA.ID, rows[0].AuxiliaryLabelID)
	require.Equal(t, auxiliaryB.ID, rows[1].AuxiliaryLabelID)
}

func TestSemanticBoardUpgradeConfirmMergeIntoExisting(t *testing.T) {
	db := setupSemanticBoardUpgradeTestDB(t)
	auxiliaryA := createUpgradeLabel(t, db, "OpenAI", "openai", "auxiliary", "active", 5, []float64{1, 0, 0})
	auxiliaryB := createUpgradeLabel(t, db, "GPT", "gpt", "auxiliary", "active", 5, []float64{0, 1, 0})
	board := createUpgradeLabel(t, db, "AI Board", "ai-board", "board", "active", 0, nil)
	require.NoError(t, db.Create(&models.BoardComposition{BoardID: board.ID, AuxiliaryLabelID: auxiliaryA.ID}).Error)
	service := NewSemanticBoardUpgradeService(db, nil)

	result, err := service.ConfirmSuggestion(context.Background(), ConfirmSemanticBoardUpgradeRequest{
		Decision:          SemanticBoardUpgradeDecisionMergeIntoExisting,
		TargetBoardID:     &board.ID,
		AuxiliaryLabelIDs: []uint{auxiliaryA.ID, auxiliaryB.ID},
	})

	require.NoError(t, err)
	require.Equal(t, board.ID, result.SemanticBoardID)
	var rows []models.BoardComposition
	require.NoError(t, db.Where("board_id = ?", board.ID).Order("auxiliary_label_id ASC").Find(&rows).Error)
	require.Len(t, rows, 2)
	require.Equal(t, auxiliaryA.ID, rows[0].AuxiliaryLabelID)
	require.Equal(t, auxiliaryB.ID, rows[1].AuxiliaryLabelID)
}

func createUpgradeLabel(t *testing.T, db *gorm.DB, label string, slug string, labelType string, status string, refCount int, vector []float64) models.SemanticLabel {
	t.Helper()
	semanticLabel := models.SemanticLabel{Label: label, Slug: slug, LabelType: labelType, Status: status, RefCount: refCount}
	if vector != nil {
		pgVector := floatsToPgVector(vector)
		semanticLabel.Embedding = &pgVector
	}
	require.NoError(t, db.Create(&semanticLabel).Error)
	return semanticLabel
}

func createUpgradeTopicTag(t *testing.T, db *gorm.DB, label string, category string) models.TopicTag {
	t.Helper()
	tag := models.TopicTag{Label: label, Slug: Slugify(label), Category: category, Status: "active"}
	require.NoError(t, db.Create(&tag).Error)
	return tag
}

func createUpgradeTopicEmbedding(t *testing.T, db *gorm.DB, topicTagID uint, vector []float64) {
	t.Helper()
	pgVector := floatsToPgVector(vector)
	require.NoError(t, db.Create(&models.TopicTagEmbedding{TopicTagID: topicTagID, EmbeddingType: "semantic", Vector: "[]", EmbeddingVec: pgVector, Dimension: len(vector), Model: "test", TextHash: fmt.Sprintf("hash-%d", topicTagID)}).Error)
}

func createUpgradeArticleWithTags(t *testing.T, db *gorm.DB, topicTagIDs ...uint) {
	t.Helper()
	now := time.Now()
	seq := atomic.AddUint64(&upgradeFeedSeq, 1)
	feed := models.Feed{Title: fmt.Sprintf("feed-%d", seq), URL: fmt.Sprintf("https://example.com/%d", seq), CreatedAt: now}
	require.NoError(t, db.Create(&feed).Error)
	article := models.Article{FeedID: feed.ID, Title: fmt.Sprintf("article-%d", now.UnixNano()), CreatedAt: now}
	require.NoError(t, db.Create(&article).Error)
	for _, topicTagID := range topicTagIDs {
		require.NoError(t, db.Create(&models.ArticleTopicTag{ArticleID: article.ID, TopicTagID: topicTagID}).Error)
	}
}

func upgradeCandidateIDs(candidates []SemanticBoardUpgradeCandidate) []uint {
	ids := make([]uint, 0, len(candidates))
	for _, candidate := range candidates {
		ids = append(ids, candidate.ID)
	}
	return ids
}
