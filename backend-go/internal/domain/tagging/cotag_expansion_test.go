package tagging

import (
	"testing"
)

func TestExpandEventCandidatesByArticleCoTags(t *testing.T) {
	// Integration test - requires database.
	// Verified via command/manual flow instead of unit test.
}

func TestCalculateCoTagTopN(t *testing.T) {
	tests := []struct {
		name         string
		subtreeDepth int
		expected     int
	}{
		{"raw event tag depth 0", 0, 5},
		{"depth 1 abstract", 1, 5},
		{"depth 2 abstract", 2, 7},
		{"depth 3 abstract", 3, 9},
		{"depth 5 abstract", 5, 13},
		{"depth 10 capped", 10, 15},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := calculateCoTagTopN(tt.subtreeDepth)
			if got != tt.expected {
				t.Errorf("calculateCoTagTopN(%d) = %d, want %d", tt.subtreeDepth, got, tt.expected)
			}
		})
	}
}

func TestAggregateKeywordsByChildCoverage(t *testing.T) {
	// Integration test - verify keyword aggregation by child tag coverage count.
}
