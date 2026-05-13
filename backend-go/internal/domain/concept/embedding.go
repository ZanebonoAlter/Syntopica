package concept

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"my-robot-backend/internal/domain/models"
	"my-robot-backend/internal/platform/airouter"
	"my-robot-backend/internal/platform/database"
	"my-robot-backend/internal/platform/logging"
)

// GenerateConceptEmbedding generates an embedding vector for a concept using
// its Name and Description, then persists the vector in the concept's Embedding field.
func GenerateConceptEmbedding(ctx context.Context, concept *models.BoardConcept) error {
	text := concept.Name
	if concept.Description != "" {
		text = concept.Name + "\n" + concept.Description
	}

	router := airouter.NewRouter()
	result, err := router.Embed(ctx, airouter.EmbeddingRequest{
		Input: []string{text},
		Metadata: map[string]any{
			"concept_id":   concept.ID,
			"concept_name": concept.Name,
			"operation":    "board_concept_embedding",
		},
	}, airouter.CapabilityEmbedding)
	if err != nil {
		return fmt.Errorf("generate concept embedding: %w", err)
	}

	if len(result.Embeddings) == 0 || len(result.Embeddings[0]) == 0 {
		return fmt.Errorf("empty embedding result for concept %d", concept.ID)
	}

	pgVecStr := floatsToPgVectorStr(result.Embeddings[0])

	if err := database.DB.Model(concept).Update("embedding", pgVecStr).Error; err != nil {
		return fmt.Errorf("save concept embedding: %w", err)
	}

	logging.Infof("concept-embedding: generated embedding for concept %d (%s), dim=%d model=%s",
		concept.ID, concept.Name, result.Dimensions, result.Model)
	return nil
}

// parseConceptEmbeddingVec parses a vector string into a float64 slice.
func parseConceptEmbeddingVec(embeddingStr *string) ([]float64, error) {
	if embeddingStr == nil || *embeddingStr == "" {
		return nil, fmt.Errorf("empty embedding string")
	}
	var vec []float64
	if err := json.Unmarshal([]byte(*embeddingStr), &vec); err != nil {
		return nil, fmt.Errorf("parse embedding vector: %w", err)
	}
	return vec, nil
}

// floatsToPgVectorStr converts a float64 slice to a pgvector-compatible string.
func floatsToPgVectorStr(v []float64) string {
	parts := make([]string, len(v))
	for i, f := range v {
		parts[i] = fmt.Sprintf("%f", f)
	}
	return "[" + strings.Join(parts, ",") + "]"
}
