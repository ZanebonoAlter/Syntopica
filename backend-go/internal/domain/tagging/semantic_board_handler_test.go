package tagging

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"my-robot-backend/internal/domain/models"
	"my-robot-backend/internal/platform/database"
)

type fakeSemanticBoardHandlerLLM struct {
	suggestions []SemanticBoardUpgradeSuggestion
}

func (f fakeSemanticBoardHandlerLLM) SuggestSemanticBoardUpgrades(ctx context.Context, prompt string) ([]SemanticBoardUpgradeSuggestion, error) {
	return f.suggestions, nil
}

func setupSemanticBoardHandlerRouter(t *testing.T) (*gorm.DB, *gin.Engine) {
	t.Helper()
	g := gin.New()
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.Exec("PRAGMA foreign_keys = ON").Error)
	database.DB = db
	require.NoError(t, db.AutoMigrate(&models.Feed{}, &models.Article{}, &models.TopicTag{}, &models.TopicTagEmbedding{}, &models.ArticleTopicTag{}, &models.SemanticLabel{}, &models.TopicTagSemanticLabel{}, &models.TopicTagBoardLabel{}, &models.BoardComposition{}, &models.AISettings{}))

	semanticBoardLabelEmbedder = func(ctx context.Context, input string, mode auxiliaryLabelEmbeddingMode) (string, []float64, error) {
		return floatsToPgVector([]float64{1, 0, 0}), []float64{1, 0, 0}, nil
	}
	semanticBoardUpgradeLLMFactory = func() semanticBoardUpgradeLLM {
		return fakeSemanticBoardHandlerLLM{suggestions: []SemanticBoardUpgradeSuggestion{{Decision: SemanticBoardUpgradeDecisionCreateNew, BoardLabel: "AI Board", Description: "AI stories", AuxiliaryLabelIDs: []uint{1}}}}
	}
	t.Cleanup(func() {
		semanticBoardLabelEmbedder = defaultAuxiliaryLabelEmbedder
		semanticBoardUpgradeLLMFactory = newSemanticBoardUpgradeLLM
	})

	api := g.Group("/api")
	RegisterSemanticBoardRoutes(api)
	return db, g
}

func TestSemanticBoardHandlerSuggestAuxiliaries(t *testing.T) {
	db, router := setupSemanticBoardHandlerRouter(t)

	// Create auxiliary labels with different embeddings
	_ = createHandlerSemanticLabel(t, db, "OpenAI", "openai", "auxiliary", "active", 8, []float64{1, 0, 0})
	_ = createHandlerSemanticLabel(t, db, "图像生成", "tu-xiang-sheng-cheng", "auxiliary", "active", 5, []float64{0, 1, 0})
	_ = createHandlerSemanticLabel(t, db, "量子计算", "liang-zi-ji-suan", "auxiliary", "active", 3, []float64{0, 0, 1})
	disabled := createHandlerSemanticLabel(t, db, "Disabled", "disabled", "auxiliary", "disabled", 1, []float64{1, 0, 0})
	_ = disabled

	// Case 1: Basic suggestion — embedder returns [1,0,0], so OpenAI should be top
	resp := performJSON(t, router, http.MethodGet, "/api/semantic-boards/suggest-auxiliaries?label=AI", nil)
	require.Equal(t, http.StatusOK, resp.Code)
	var body map[string]any
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &body))
	data := body["data"].(map[string]any)
	items := data["items"].([]any)
	require.True(t, len(items) >= 1, "should have at least 1 suggestion")
	first := items[0].(map[string]any)
	require.Equal(t, "OpenAI", first["label"])
	require.True(t, first["similarity"].(float64) > 0.9)

	// Case 2: Missing label → 400
	badResp := performJSON(t, router, http.MethodGet, "/api/semantic-boards/suggest-auxiliaries", nil)
	require.Equal(t, http.StatusBadRequest, badResp.Code)

	// Case 3: Search filter
	searchResp := performJSON(t, router, http.MethodGet, "/api/semantic-boards/suggest-auxiliaries?label=AI&search=Open", nil)
	require.Equal(t, http.StatusOK, searchResp.Code)

	// Case 4: Pagination
	pageResp := performJSON(t, router, http.MethodGet, "/api/semantic-boards/suggest-auxiliaries?label=AI&page=1&page_size=1", nil)
	require.Equal(t, http.StatusOK, pageResp.Code)
	var pageBody map[string]any
	require.NoError(t, json.Unmarshal(pageResp.Body.Bytes(), &pageBody))
	pageData := pageBody["data"].(map[string]any)
	pageItems := pageData["items"].([]any)
	require.Len(t, pageItems, 1)
	require.Equal(t, float64(3), pageData["total"]) // 3 active auxiliaries

	// Case 5: exclude_board_id — create a board with composition
	board := createHandlerSemanticLabel(t, db, "AI Board", "ai-board", "board", "active", 0, []float64{1, 0, 0})
	require.NoError(t, db.Create(&models.BoardComposition{BoardID: board.ID, AuxiliaryLabelID: 1}).Error)
	excludeResp := performJSON(t, router, http.MethodGet, fmt.Sprintf("/api/semantic-boards/suggest-auxiliaries?label=AI&exclude_board_id=%d", board.ID), nil)
	require.Equal(t, http.StatusOK, excludeResp.Code)
	var excludeBody map[string]any
	require.NoError(t, json.Unmarshal(excludeResp.Body.Bytes(), &excludeBody))
	excludeData := excludeBody["data"].(map[string]any)
	excludeItems := excludeData["items"].([]any)
	require.Equal(t, float64(2), excludeData["total"], "excluded auxiliary should be removed before pagination")
	for _, item := range excludeItems {
		require.NotEqual(t, float64(1), item.(map[string]any)["id"], "excluded auxiliary should not appear")
	}

	excludePagedResp := performJSON(t, router, http.MethodGet, fmt.Sprintf("/api/semantic-boards/suggest-auxiliaries?label=AI&exclude_board_id=%d&page=1&page_size=1", board.ID), nil)
	require.Equal(t, http.StatusOK, excludePagedResp.Code)
	var excludePagedBody map[string]any
	require.NoError(t, json.Unmarshal(excludePagedResp.Body.Bytes(), &excludePagedBody))
	excludePagedData := excludePagedBody["data"].(map[string]any)
	require.Equal(t, float64(2), excludePagedData["total"], "exclude_board_id should apply before pagination")
	require.Len(t, excludePagedData["items"].([]any), 1)
}

func TestSemanticBoardHandlerSuggestAuxiliariesForBoard(t *testing.T) {
	db, router := setupSemanticBoardHandlerRouter(t)

	_ = createHandlerSemanticLabel(t, db, "OpenAI", "openai", "auxiliary", "active", 8, []float64{1, 0, 0})
	_ = createHandlerSemanticLabel(t, db, "量子计算", "liang-zi-ji-suan", "auxiliary", "active", 3, []float64{0, 0, 1})

	board := createHandlerSemanticLabel(t, db, "AI Board", "ai-board", "board", "active", 0, []float64{1, 0, 0})
	require.NoError(t, db.Create(&models.BoardComposition{BoardID: board.ID, AuxiliaryLabelID: 1}).Error)

	// Should exclude already-composed labels
	resp := performJSON(t, router, http.MethodGet, fmt.Sprintf("/api/semantic-boards/%d/suggest-auxiliaries", board.ID), nil)
	require.Equal(t, http.StatusOK, resp.Code)
	var body map[string]any
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &body))
	items := body["data"].(map[string]any)["items"].([]any)
	for _, item := range items {
		require.NotEqual(t, float64(1), item.(map[string]any)["id"], "should exclude composed auxiliary")
	}

	// Non-existent board → 404
	notFound := performJSON(t, router, http.MethodGet, "/api/semantic-boards/999/suggest-auxiliaries", nil)
	require.Equal(t, http.StatusNotFound, notFound.Code)
}

func TestSemanticBoardHandlerAddComposition(t *testing.T) {
	db, router := setupSemanticBoardHandlerRouter(t)

	auxiliary := createHandlerSemanticLabel(t, db, "OpenAI", "openai", "auxiliary", "active", 8, []float64{1, 0, 0})
	board := createHandlerSemanticLabel(t, db, "AI Board", "ai-board", "board", "active", 0, []float64{1, 0, 0})

	// Case 1: Normal add
	resp := performJSON(t, router, http.MethodPost, fmt.Sprintf("/api/semantic-boards/%d/composition", board.ID), map[string]any{"auxiliary_label_id": auxiliary.ID})
	require.Equal(t, http.StatusOK, resp.Code)
	var count int64
	require.NoError(t, db.Model(&models.BoardComposition{}).Where("board_id = ? AND auxiliary_label_id = ?", board.ID, auxiliary.ID).Count(&count).Error)
	require.Equal(t, int64(1), count)

	// Case 2: Idempotent add
	resp2 := performJSON(t, router, http.MethodPost, fmt.Sprintf("/api/semantic-boards/%d/composition", board.ID), map[string]any{"auxiliary_label_id": auxiliary.ID})
	require.Equal(t, http.StatusOK, resp2.Code)
	require.NoError(t, db.Model(&models.BoardComposition{}).Where("board_id = ? AND auxiliary_label_id = ?", board.ID, auxiliary.ID).Count(&count).Error)
	require.Equal(t, int64(1), count)

	// Case 3: Board not found
	notFound := performJSON(t, router, http.MethodPost, "/api/semantic-boards/999/composition", map[string]any{"auxiliary_label_id": auxiliary.ID})
	require.Equal(t, http.StatusNotFound, notFound.Code)

	// Case 4: Auxiliary not found
	badAux := performJSON(t, router, http.MethodPost, fmt.Sprintf("/api/semantic-boards/%d/composition", board.ID), map[string]any{"auxiliary_label_id": 999})
	require.Equal(t, http.StatusBadRequest, badAux.Code)

	// Case 5: Missing body
	badBody := performJSON(t, router, http.MethodPost, fmt.Sprintf("/api/semantic-boards/%d/composition", board.ID), nil)
	require.Equal(t, http.StatusBadRequest, badBody.Code)
}

func TestSemanticBoardHandlerCRUDAndComposition(t *testing.T) {
	db, router := setupSemanticBoardHandlerRouter(t)
	auxiliary := createHandlerSemanticLabel(t, db, "OpenAI", "openai", "auxiliary", "active", 8, []float64{1, 0, 0})
	tag := createHandlerTopicTag(t, db, "GPT-5", models.TagCategoryEvent)

	created := performJSON(t, router, http.MethodPost, "/api/semantic-boards", map[string]any{
		"label":            "AI与机器学习",
		"description":      "AI topic board",
		"auxiliary_labels": []uint{auxiliary.ID},
	})
	require.Equal(t, http.StatusOK, created.Code)
	createdID := responseDataFloat(t, created, "id")
	require.NotZero(t, createdID)

	boardID := uint(createdID)
	require.NoError(t, db.Create(&models.TopicTagBoardLabel{TopicTagID: tag.ID, SemanticBoardID: boardID, Score: 0.9, MatchReason: "direct_hit"}).Error)

	list := performJSON(t, router, http.MethodGet, "/api/semantic-boards", nil)
	require.Equal(t, http.StatusOK, list.Code)
	require.Contains(t, list.Body.String(), "AI与机器学习")
	require.Contains(t, list.Body.String(), "tag_count")

	updated := performJSON(t, router, http.MethodPut, fmt.Sprintf("/api/semantic-boards/%d", boardID), map[string]any{
		"label":       "AI生态",
		"description": "updated",
	})
	require.Equal(t, http.StatusOK, updated.Code)

	composition := performJSON(t, router, http.MethodGet, fmt.Sprintf("/api/semantic-boards/%d/composition", boardID), nil)
	require.Equal(t, http.StatusOK, composition.Code)
	require.Contains(t, composition.Body.String(), "OpenAI")

	removed := performJSON(t, router, http.MethodDelete, fmt.Sprintf("/api/semantic-boards/%d/composition/%d", boardID, auxiliary.ID), nil)
	require.Equal(t, http.StatusOK, removed.Code)
	var count int64
	require.NoError(t, db.Model(&models.BoardComposition{}).Where("board_id = ? AND auxiliary_label_id = ?", boardID, auxiliary.ID).Count(&count).Error)
	require.Zero(t, count)

	deleted := performJSON(t, router, http.MethodDelete, fmt.Sprintf("/api/semantic-boards/%d", boardID), nil)
	require.Equal(t, http.StatusOK, deleted.Code)
	var board models.SemanticLabel
	require.NoError(t, db.First(&board, boardID).Error)
	require.Equal(t, "disabled", board.Status)
}

func TestSemanticBoardHandlerAuxiliaryGovernanceAndTagAssociations(t *testing.T) {
	db, router := setupSemanticBoardHandlerRouter(t)
	tag := createHandlerTopicTag(t, db, "OpenAI 发布", models.TagCategoryEvent)
	target := createHandlerSemanticLabel(t, db, "OpenAI", "openai", "auxiliary", "active", 1, []float64{1, 0, 0})
	source := createHandlerSemanticLabel(t, db, "Open AI", "open-ai", "auxiliary", "active", 1, []float64{1, 0, 0})
	board := createHandlerSemanticLabel(t, db, "AI Board", "ai-board", "board", "active", 0, nil)
	disabledBoard := createHandlerSemanticLabel(t, db, "Disabled Board", "disabled-board", "board", "disabled", 0, nil)
	require.NoError(t, db.Create(&models.TopicTagSemanticLabel{TopicTagID: tag.ID, SemanticLabelID: target.ID}).Error)
	require.NoError(t, db.Create(&models.TopicTagSemanticLabel{TopicTagID: tag.ID, SemanticLabelID: source.ID}).Error)
	require.NoError(t, db.Create(&models.TopicTagBoardLabel{TopicTagID: tag.ID, SemanticBoardID: board.ID, Score: 0.75, MatchReason: "weighted"}).Error)
	require.NoError(t, db.Create(&models.TopicTagBoardLabel{TopicTagID: tag.ID, SemanticBoardID: disabledBoard.ID, Score: 0.9, MatchReason: "disabled"}).Error)

	auxiliaries := performJSON(t, router, http.MethodGet, "/api/auxiliary-labels?search=Open", nil)
	require.Equal(t, http.StatusOK, auxiliaries.Code)
	require.Contains(t, auxiliaries.Body.String(), "OpenAI")

	merge := performJSON(t, router, http.MethodPost, "/api/auxiliary-labels/merge-alias", map[string]any{"source_id": source.ID, "target_id": target.ID})
	require.Equal(t, http.StatusOK, merge.Code)
	var reloadedTarget models.SemanticLabel
	require.NoError(t, db.First(&reloadedTarget, target.ID).Error)
	require.Contains(t, reloadedTarget.Aliases, "Open AI")

	disable := performJSON(t, router, http.MethodPost, fmt.Sprintf("/api/auxiliary-labels/%d/disable", target.ID), nil)
	require.Equal(t, http.StatusOK, disable.Code)

	tagAux := performJSON(t, router, http.MethodGet, fmt.Sprintf("/api/tags/%d/auxiliary-labels", tag.ID), nil)
	require.Equal(t, http.StatusOK, tagAux.Code)
	require.Contains(t, tagAux.Body.String(), "OpenAI")

	tagBoards := performJSON(t, router, http.MethodGet, fmt.Sprintf("/api/tags/%d/semantic-boards", tag.ID), nil)
	require.Equal(t, http.StatusOK, tagBoards.Code)
	require.Contains(t, tagBoards.Body.String(), "AI Board")
	require.Contains(t, tagBoards.Body.String(), "weighted")
	require.NotContains(t, tagBoards.Body.String(), "Disabled Board")
}

func TestSemanticBoardHandlerUpgradeBackfillAndConfig(t *testing.T) {
	db, router := setupSemanticBoardHandlerRouter(t)
	require.NoError(t, db.Create(&models.AISettings{Key: "semantic_board_upgrade_ref_count_threshold", Value: "1"}).Error)
	auxiliary := createHandlerSemanticLabel(t, db, "OpenAI", "openai", "auxiliary", "active", 5, []float64{1, 0, 0})
	tag := createHandlerTopicTag(t, db, "GPT-5", models.TagCategoryEvent)
	require.NoError(t, db.Create(&models.TopicTagSemanticLabel{TopicTagID: tag.ID, SemanticLabelID: auxiliary.ID}).Error)

	candidates := performJSON(t, router, http.MethodGet, "/api/semantic-boards/upgrade-candidates", nil)
	require.Equal(t, http.StatusOK, candidates.Code)
	require.Contains(t, candidates.Body.String(), "OpenAI")

	suggest := performJSON(t, router, http.MethodPost, "/api/semantic-boards/upgrade-suggest", nil)
	require.Equal(t, http.StatusOK, suggest.Code)
	require.Contains(t, suggest.Body.String(), "create_new")

	execute := performJSON(t, router, http.MethodPost, "/api/semantic-boards/upgrade-execute", map[string]any{
		"decision":            "create_new",
		"board_label":         "AI Board",
		"description":         "AI stories",
		"auxiliary_label_ids": []uint{auxiliary.ID},
	})
	require.Equal(t, http.StatusOK, execute.Code)
	boardID := uint(responseDataFloat(t, execute, "semantic_board_id"))
	require.NotZero(t, boardID)

	config := performJSON(t, router, http.MethodPut, "/api/semantic-boards/matching-config", map[string]any{
		"semantic_board_match_sim_threshold": "0.7",
		"semantic_board_match_max_boards":    "2",
	})
	require.Equal(t, http.StatusOK, config.Code)
	loadedConfig := performJSON(t, router, http.MethodGet, "/api/semantic-boards/matching-config", nil)
	require.Equal(t, http.StatusOK, loadedConfig.Code)
	require.Contains(t, loadedConfig.Body.String(), "0.7")
	invalidConfig := performJSON(t, router, http.MethodPut, "/api/semantic-boards/matching-config", map[string]any{"semantic_board_match_sim_threshold": "2"})
	require.Equal(t, http.StatusBadRequest, invalidConfig.Code)

	backfill := performJSON(t, router, http.MethodPost, "/api/semantic-boards/backfill", map[string]any{"mode": "board", "board_id": boardID})
	require.Equal(t, http.StatusOK, backfill.Code)
	jobID := responseDataString(t, backfill, "id")
	require.NotEmpty(t, jobID)

	require.Eventually(t, func() bool {
		status := performJSON(t, router, http.MethodGet, "/api/semantic-boards/backfill/"+jobID, nil)
		return status.Code == http.StatusOK && strings.Contains(status.Body.String(), "completed")
	}, time.Second, 10*time.Millisecond)
}

func performJSON(t *testing.T, router *gin.Engine, method string, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var payload []byte
	if body != nil {
		var err error
		payload, err = json.Marshal(body)
		require.NoError(t, err)
	}
	req := httptest.NewRequest(method, path, bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)
	return recorder
}

func responseDataFloat(t *testing.T, recorder *httptest.ResponseRecorder, key string) float64 {
	t.Helper()
	var body map[string]any
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &body))
	data, ok := body["data"].(map[string]any)
	require.True(t, ok)
	value, ok := data[key].(float64)
	require.True(t, ok)
	return value
}

func responseDataString(t *testing.T, recorder *httptest.ResponseRecorder, key string) string {
	t.Helper()
	var body map[string]any
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &body))
	data, ok := body["data"].(map[string]any)
	require.True(t, ok)
	value, ok := data[key].(string)
	require.True(t, ok)
	return value
}

func createHandlerTopicTag(t *testing.T, db *gorm.DB, label string, category string) models.TopicTag {
	t.Helper()
	tag := models.TopicTag{Label: label, Slug: Slugify(label), Category: category, Status: "active"}
	require.NoError(t, db.Create(&tag).Error)
	return tag
}

func createHandlerSemanticLabel(t *testing.T, db *gorm.DB, label string, slug string, labelType string, status string, refCount int, vector []float64) models.SemanticLabel {
	t.Helper()
	semanticLabel := models.SemanticLabel{Label: label, Slug: slug, LabelType: labelType, Status: status, RefCount: refCount}
	if vector != nil {
		pgVector := floatsToPgVector(vector)
		semanticLabel.Embedding = &pgVector
	}
	require.NoError(t, db.Create(&semanticLabel).Error)
	return semanticLabel
}
