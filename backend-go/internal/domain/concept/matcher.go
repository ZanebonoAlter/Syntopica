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
	titleEmbeddingWeight      float64 = 2.0
	keywordEmbeddingWeight    float64 = 1.0
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

// MatchTagToConcept reads the tag's embedding(s) from topic_tag_embeddings,
// compares against active concepts (filtered by category) via cosine similarity,
// and returns the best match above the configured threshold.
//
// For event tags, all semantic + event_keyword embeddings are loaded and a
// weighted average similarity is computed (title × 2.0, keyword × 1.0).
// For non-event tags, the existing single-embedding logic is used.
func MatchTagToConcept(ctx context.Context, tagLabel string, tagDescription string, category string, tagID uint) (*ConceptMatchResult, error) {
	var concepts []models.BoardConcept
	if err := database.DB.Where("category = ? AND status = ? AND embedding IS NOT NULL", category, "active").
		Find(&concepts).Error; err != nil {
		return nil, fmt.Errorf("list active concepts for category %s: %w", category, err)
	}

	if len(concepts) == 0 {
		return nil, nil
	}

	threshold := getEmbeddingThreshold()

	if category == models.TagCategoryEvent {
		return matchEventTagToConcept(tagID, concepts, threshold)
	}

	return matchSingleEmbeddingTag(tagID, concepts, threshold)
}

// matchEventTagToConcept loads all semantic + event_keyword embeddings for an
// event tag and computes a weighted-average similarity against each concept.
func matchEventTagToConcept(tagID uint, concepts []models.BoardConcept, threshold float64) (*ConceptMatchResult, error) {
	var tagEmbs []models.TopicTagEmbedding
	if err := database.DB.Where("topic_tag_id = ? AND embedding_type IN ?", tagID,
		[]string{"semantic", "event_keyword"}).Find(&tagEmbs).Error; err != nil {
		return nil, fmt.Errorf("read event tag embeddings for tag %d: %w", tagID, err)
	}

	if len(tagEmbs) == 0 {
		return nil, fmt.Errorf("no embeddings found for event tag %d", tagID)
	}

	type embRow struct {
		vec    []float64
		weight float64
	}
	rows := make([]embRow, 0, len(tagEmbs))
	for _, emb := range tagEmbs {
		vec, err := parseConceptEmbeddingVec(&emb.EmbeddingVec)
		if err != nil {
			logging.Warnf("concept-matcher: skip embedding %d for tag %d — bad vector: %v", emb.ID, tagID, err)
			continue
		}
		w := keywordEmbeddingWeight
		if emb.EmbeddingType == "semantic" {
			w = titleEmbeddingWeight
		}
		rows = append(rows, embRow{vec: vec, weight: w})
	}

	if len(rows) == 0 {
		return nil, fmt.Errorf("no valid embeddings for event tag %d", tagID)
	}

	var bestMatch *ConceptMatchResult
	var bestScore float64

	for _, concept := range concepts {
		conceptVec, err := parseConceptEmbeddingVec(concept.Embedding)
		if err != nil {
			logging.Warnf("concept-matcher: skip concept %d (%s) — bad embedding: %v", concept.ID, concept.Name, err)
			continue
		}

		var totalWeight, weightedSum float64
		for _, row := range rows {
			sim, simErr := airouter.CosineSimilarity(row.vec, conceptVec)
			if simErr != nil {
				continue
			}
			weightedSum += sim * row.weight
			totalWeight += row.weight
		}

		if totalWeight == 0 {
			continue
		}

		score := weightedSum / totalWeight

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

// matchSingleEmbeddingTag is the original single-embedding matching logic used
// for non-event tags (keyword, person).
func matchSingleEmbeddingTag(tagID uint, concepts []models.BoardConcept, threshold float64) (*ConceptMatchResult, error) {
	var tagEmb models.TopicTagEmbedding
	if err := database.DB.Where("topic_tag_id = ? AND embedding_type = ?", tagID, "semantic").
		First(&tagEmb).Error; err != nil {
		return nil, fmt.Errorf("read tag embedding for tag %d: %w", tagID, err)
	}

	tagVec, err := parseConceptEmbeddingVec(&tagEmb.EmbeddingVec)
	if err != nil {
		return nil, fmt.Errorf("parse tag embedding for tag %d: %w", tagID, err)
	}

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
