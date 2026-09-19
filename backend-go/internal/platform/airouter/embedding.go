package airouter

import (
	"fmt"
	"math"
)

// EmbeddingRequest represents a request to generate embeddings
type EmbeddingRequest struct {
	Input          []string `json:"input"`
	Model          string   `json:"model"`
	EncodingFormat string   `json:"encoding_format,omitempty"`
	Dimensions     int      `json:"dimensions,omitempty"`
	Operation      string   // 必填，业务操作名
	SessionID      string   // 可选，编排分组键
	Metadata       map[string]any
}

// EmbeddingResult represents the result of an embedding request
type EmbeddingResult struct {
	Embeddings [][]float64 `json:"embeddings"`
	Model      string      `json:"model"`
	Dimensions int         `json:"dimensions"`
	Provider   string      `json:"provider"`
	// Usage carries the provider-reported token usage (OpenAI-compatible
	// "usage" block). Nil when the response carries no usage block, so call
	// logs keep token_usage NULL instead of fabricating zeros.
	Usage *TokenUsage `json:"usage,omitempty"`
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

	return dotProduct / (math.Sqrt(normA) * math.Sqrt(normB)), nil
}
