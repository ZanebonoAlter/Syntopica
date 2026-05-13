package concept

import (
	"context"
	"fmt"
	"strconv"

	"my-robot-backend/internal/domain/models"
	"my-robot-backend/internal/platform/airouter"
	"my-robot-backend/internal/platform/database"
	"my-robot-backend/internal/platform/logging"
)

const (
	defaultEmbeddingThreshold float64 = 0.7
)

// ConceptMatchResult holds the result of matching a tag to a concept.
type ConceptMatchResult struct {
	ConceptID  uint    `json:"concept_id"`
	Name       string  `json:"name"`
	Similarity float64 `json:"similarity"`
}

// getEmbeddingThreshold reads the similarity threshold from ai_settings.
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

// MatchTagToConcept reads the tag's semantic embedding from topic_tag_embeddings,
// compares it against active concepts (filtered by category) via cosine similarity,
// and returns the best match above the configured threshold.
func MatchTagToConcept(ctx context.Context, tagLabel string, tagDescription string, category string, tagID uint) (*ConceptMatchResult, error) {
	// Read tag's semantic embedding from topic_tag_embeddings
	var tagEmb models.TopicTagEmbedding
	if err := database.DB.Where("topic_tag_id = ? AND embedding_type = ?", tagID, "semantic").
		First(&tagEmb).Error; err != nil {
		return nil, fmt.Errorf("read tag embedding for tag %d: %w", tagID, err)
	}

	tagVec, err := parseConceptEmbeddingVec(&tagEmb.EmbeddingVec)
	if err != nil {
		return nil, fmt.Errorf("parse tag embedding for tag %d: %w", tagID, err)
	}

	// Filter concepts by category AND status='active' AND embedding IS NOT NULL
	var concepts []models.BoardConcept
	if err := database.DB.Where("category = ? AND status = ? AND embedding IS NOT NULL", category, "active").
		Find(&concepts).Error; err != nil {
		return nil, fmt.Errorf("list active concepts for category %s: %w", category, err)
	}

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
