package airouter

import (
	"fmt"
)

// EmbeddingRequest represents a request to generate embeddings
type EmbeddingRequest struct {
	Input          []string `json:"input"`
	Model          string   `json:"model"`
	EncodingFormat string   `json:"encoding_format,omitempty"`
	Dimensions     int      `json:"dimensions,omitempty"`
	Metadata       map[string]any
}

// EmbeddingResult represents the result of an embedding request
type EmbeddingResult struct {
	Embeddings [][]float64 `json:"embeddings"`
	Model      string      `json:"model"`
	Dimensions int         `json:"dimensions"`
	Provider   string      `json:"provider"`
}

// CosineSimilarity calculates the cosine similarity between two embedding vectors
func CosineSimilarity(a, b []float64) (float64, error) {
	if len(a) != len(b) {
		return 0, fmt.Errorf("vector dimensions don't match: %d vs %d", len(a), len(b))
	}

	var dotProduct, normA, normB float64
	for i := range a {
		dotProduct += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}

	if normA == 0 || normB == 0 {
		return 0, nil
	}

	return dotProduct / (sqrt(normA) * sqrt(normB)), nil
}

// Borrowed from math package for efficiency
func sqrt(x float64) float64 {
	if x <= 0 {
		return 0
	}
	// Newton's method for sqrt
	z := x
	for i := 0; i < 100; i++ {
		z -= (z*z - x) / (2 * z)
		if z*z-x < 1e-10 && -(z*z-x) < 1e-10 {
			break
		}
	}
	return z
}
