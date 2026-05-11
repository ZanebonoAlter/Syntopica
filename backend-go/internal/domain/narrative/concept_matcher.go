package narrative

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"my-robot-backend/internal/domain/models"
	"my-robot-backend/internal/platform/airouter"
	"my-robot-backend/internal/platform/database"
	"my-robot-backend/internal/platform/logging"
)

const (
	defaultEmbeddingThreshold float64 = 0.7
	defaultHotspotThreshold   int     = 3
	unclassifiedTriggerCount  int     = 5
)

type ConceptMatchResult struct {
	ConceptID  uint    `json:"concept_id"`
	Name       string  `json:"name"`
	Similarity float64 `json:"similarity"`
}

func getEmbeddingThreshold() float64 {
	var setting models.AISettings
	if err := database.DB.Where("key = ?", "narrative_board_embedding_threshold").First(&setting).Error; err != nil {
		return defaultEmbeddingThreshold
	}
	if val, err := strconv.ParseFloat(setting.Value, 64); err == nil && val > 0 && val <= 1.0 {
		return val
	}
	return defaultEmbeddingThreshold
}

func getHotspotThreshold() int {
	var setting models.AISettings
	if err := database.DB.Where("key = ?", "narrative_board_hotspot_threshold").First(&setting).Error; err != nil {
		return defaultHotspotThreshold
	}
	if val, err := strconv.Atoi(setting.Value); err == nil && val >= 2 {
		return val
	}
	return defaultHotspotThreshold
}

func GenerateTagEmbedding(ctx context.Context, tag TagInput) ([]float64, error) {
	text := tag.Label
	if tag.Description != "" {
		text = tag.Label + "\n" + tag.Description
	}

	router := airouter.NewRouter()
	result, err := router.Embed(ctx, airouter.EmbeddingRequest{
		Input: []string{text},
		Metadata: map[string]any{
			"tag_id":    tag.ID,
			"tag_label": tag.Label,
			"operation": "narrative_tag_matching",
		},
	}, airouter.CapabilityEmbedding)
	if err != nil {
		return nil, fmt.Errorf("generate tag embedding: %w", err)
	}

	if len(result.Embeddings) == 0 || len(result.Embeddings[0]) == 0 {
		return nil, fmt.Errorf("empty embedding result for tag %d", tag.ID)
	}

	return result.Embeddings[0], nil
}

func MatchTagToConcept(ctx context.Context, tag TagInput) (*ConceptMatchResult, error) {
	tagVec, err := GenerateTagEmbedding(ctx, tag)
	if err != nil {
		return nil, err
	}

	var concepts []models.BoardConcept
	database.DB.Where("is_active = ? AND embedding IS NOT NULL", true).
		Find(&concepts)

	if len(concepts) == 0 {
		return nil, nil
	}

	threshold := getEmbeddingThreshold()
	var bestMatch *ConceptMatchResult
	var bestScore float64

	for _, concept := range concepts {
		conceptVec, err := parseConceptEmbeddingVec(concept.Embedding)
		if err != nil {
			logging.Warnf("concept-matcher: skip concept %d (%s) — bad embedding: %v", concept.ID, concept.Name, err)
			continue
		}

		score, err := airouter.CosineSimilarity(tagVec, conceptVec)
		if err != nil {
			logging.Warnf("concept-matcher: similarity error for concept %d (%s): %v", concept.ID, concept.Name, err)
			continue
		}

		if score >= threshold && (bestMatch == nil || score > bestScore) {
			bestMatch = &ConceptMatchResult{
				ConceptID:  concept.ID,
				Name:       concept.Name,
				Similarity: score,
			}
			bestScore = score
		}
	}

	return bestMatch, nil
}

func MatchTagToConceptWithVec(tagVec []float64) (*ConceptMatchResult, error) {
	var concepts []models.BoardConcept
	database.DB.Where("is_active = ? AND embedding IS NOT NULL", true).
		Find(&concepts)

	if len(concepts) == 0 {
		return nil, nil
	}

	threshold := getEmbeddingThreshold()
	var bestMatch *ConceptMatchResult
	var bestScore float64

	for _, concept := range concepts {
		conceptVec, err := parseConceptEmbeddingVec(concept.Embedding)
		if err != nil {
			continue
		}

		score, err := airouter.CosineSimilarity(tagVec, conceptVec)
		if err != nil {
			continue
		}

		if score >= threshold && (bestMatch == nil || score > bestScore) {
			bestMatch = &ConceptMatchResult{
				ConceptID:  concept.ID,
				Name:       concept.Name,
				Similarity: score,
			}
			bestScore = score
		}
	}

	return bestMatch, nil
}

type UnclassifiedBucket struct {
	Tags []TagInput `json:"tags"`
	Date time.Time  `json:"date"`
}

var pendingUnclassified []TagInput

func AddToUnclassifiedBucket(tag TagInput) {
	pendingUnclassified = append(pendingUnclassified, tag)
}

func GetUnclassifiedBucket() []TagInput {
	return pendingUnclassified
}

func ClearUnclassifiedBucket() {
	pendingUnclassified = nil
}

func TriggerUnclassifiedSuggestionIfNeeded(ctx context.Context) ([]ConceptSuggestion, error) {
	if len(pendingUnclassified) <= unclassifiedTriggerCount {
		return nil, nil
	}

	logging.Infof("concept-matcher: unclassified bucket has %d items, triggering LLM suggestion", len(pendingUnclassified))
	suggestions, err := SuggestBoardConcepts(ctx)
	if err != nil {
		return nil, fmt.Errorf("unclassified suggestion failed: %w", err)
	}

	return suggestions, nil
}

func BuildBoardFromMatchedTags(conceptID uint, conceptName string, matchedTags []TagInput, date time.Time, categoryID *uint) (*models.NarrativeBoard, error) {
	if len(matchedTags) == 0 {
		return nil, nil
	}

	startOfDay := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, date.Location())

	eventIDs := make([]uint, 0, len(matchedTags))
	for _, t := range matchedTags {
		eventIDs = append(eventIDs, t.ID)
	}

	eventIDsJSON, _ := json.Marshal(eventIDs)
	abstractIDsJSON, _ := json.Marshal([]uint{})

	prevBoardIDs := matchConceptPreviousBoard(conceptID, date, categoryID)
	prevIDsJSON, _ := json.Marshal(prevBoardIDs)

	scopeType := models.NarrativeScopeTypeFeedCategory
	if categoryID == nil {
		scopeType = models.NarrativeScopeTypeGlobal
	}

	board := &models.NarrativeBoard{
		PeriodDate:      startOfDay,
		Name:            conceptName,
		Description:     "",
		ScopeType:       scopeType,
		ScopeCategoryID: categoryID,
		EventTagIDs:     string(eventIDsJSON),
		AbstractTagIDs:  string(abstractIDsJSON),
		PrevBoardIDs:    string(prevIDsJSON),
		BoardConceptID:  &conceptID,
		IsSystem:        false,
	}

	if err := database.DB.Create(board).Error; err != nil {
		return nil, fmt.Errorf("create matched concept board: %w", err)
	}

	logging.Infof("concept-matcher: created board %d (%s) from concept %d with %d tags",
		board.ID, board.Name, conceptID, len(matchedTags))
	return board, nil
}

func matchConceptPreviousBoard(conceptID uint, date time.Time, categoryID *uint) []uint {
	yesterday := date.AddDate(0, 0, -1)
	startOfYesterday := time.Date(yesterday.Year(), yesterday.Month(), yesterday.Day(), 0, 0, 0, 0, yesterday.Location())
	endOfYesterday := startOfYesterday.Add(24 * time.Hour)

	query := database.DB.Where("board_concept_id = ? AND period_date >= ? AND period_date < ?",
		conceptID, startOfYesterday, endOfYesterday)
	if categoryID != nil {
		query = query.Where("scope_category_id = ?", *categoryID)
	}

	var boards []models.NarrativeBoard
	query.Find(&boards)

	if len(boards) == 0 {
		return nil
	}

	var ids []uint
	for _, b := range boards {
		ids = append(ids, b.ID)
	}
	return ids
}
