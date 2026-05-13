package tagging

import (
	"context"
	"testing"

	"my-robot-backend/internal/domain/models"
)

func TestBatchCallLLMEmptyItems(t *testing.T) {
	result, err := BatchCallLLMForTagJudgment(context.Background(), nil, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Results) != 0 {
		t.Fatal("expected empty results")
	}
}

func TestParseBatchTagJudgmentResponse(t *testing.T) {
	content := `{
		"tags": {
			"AI": {
				"merges": [],
				"abstracts": [],
				"none": ["人工智能"]
			},
			"GPT-5": {
				"merges": [{"target": "GPT-5发布", "label": "GPT-5", "children": [], "reason": "same event"}],
				"abstracts": [],
				"none": []
			}
		}
	}`
	items := []BatchTagJudgmentItem{
		{Label: "AI", Category: "keyword", Candidates: []TagCandidate{
			{Tag: &models.TopicTag{ID: 1, Label: "人工智能", Slug: "ren-gong-zhi-neng"}, Similarity: 0.85},
		}},
		{Label: "GPT-5", Category: "event", Candidates: []TagCandidate{
			{Tag: &models.TopicTag{ID: 2, Label: "GPT-5发布", Slug: "gpt-5-fa-bu"}, Similarity: 0.92},
		}},
	}

	result, err := parseBatchTagJudgmentResponse(context.Background(), content, items)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if len(result.Results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(result.Results))
	}
	if _, ok := result.Results["AI"]; !ok {
		t.Fatal("missing AI result")
	}
	if _, ok := result.Results["GPT-5"]; !ok {
		t.Fatal("missing GPT-5 result")
	}
}

func TestParseBatchTagJudgmentMissingTag(t *testing.T) {
	content := `{
		"tags": {
			"AI": {
				"merges": [],
				"abstracts": [],
				"none": ["人工智能"]
			}
		}
	}`
	items := []BatchTagJudgmentItem{
		{Label: "AI", Category: "keyword", Candidates: []TagCandidate{
			{Tag: &models.TopicTag{ID: 1, Label: "人工智能", Slug: "ren-gong-zhi-neng"}, Similarity: 0.85},
		}},
		{Label: "GPT-5", Category: "event", Candidates: []TagCandidate{
			{Tag: &models.TopicTag{ID: 2, Label: "GPT-5发布", Slug: "gpt-5-fa-bu"}, Similarity: 0.92},
		}},
	}

	result, err := parseBatchTagJudgmentResponse(context.Background(), content, items)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if len(result.Results) != 1 {
		t.Fatalf("expected 1 result (missing tag should be skipped), got %d", len(result.Results))
	}
}

func TestParseBatchTagJudgmentInvalidJSON(t *testing.T) {
	items := []BatchTagJudgmentItem{
		{Label: "AI", Category: "keyword", Candidates: []TagCandidate{
			{Tag: &models.TopicTag{ID: 1, Label: "人工智能"}, Similarity: 0.85},
		}},
	}

	_, err := parseBatchTagJudgmentResponse(context.Background(), "not valid json", items)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}
