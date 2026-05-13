package tagging

import (
	"testing"
)

func TestBatchAdoptNarrowerBatching(t *testing.T) {
	tasks := []adoptTaskWithCandidates{
		{AbstractTagID: 1, Label: "AI", Candidates: []TagCandidate{{Similarity: 0.8}}},
		{AbstractTagID: 2, Label: "Cloud", Candidates: []TagCandidate{{Similarity: 0.7}}},
		{AbstractTagID: 3, Label: "Security", Candidates: []TagCandidate{}},
	}

	batches := groupAdoptTasksByCategory(tasks, 2)
	if len(batches) != 1 {
		t.Fatalf("expected 1 batch, got %d", len(batches))
	}
	totalTasks := 0
	for _, b := range batches {
		totalTasks += len(b)
	}
	if totalTasks != 2 {
		t.Fatalf("expected 2 tasks with candidates in batches, got %d", totalTasks)
	}
}

func TestGroupAdoptTasksByCategory_EmptyCandidates(t *testing.T) {
	tasks := []adoptTaskWithCandidates{
		{AbstractTagID: 1, Label: "AI", Candidates: []TagCandidate{}},
	}
	batches := groupAdoptTasksByCategory(tasks, 5)
	if len(batches) != 0 {
		t.Fatalf("expected 0 batches for empty candidates, got %d", len(batches))
	}
}
