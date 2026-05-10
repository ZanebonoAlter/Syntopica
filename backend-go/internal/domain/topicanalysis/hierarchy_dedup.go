package topicanalysis

import (
	"context"
	"encoding/json"
	"fmt"

	"my-robot-backend/internal/domain/models"
	"my-robot-backend/internal/platform/airouter"
	"my-robot-backend/internal/platform/database"
	"my-robot-backend/internal/platform/logging"
)

const (
	DedupL2Threshold = 0.95
	DedupL1Threshold = 0.90
)

func dedupL2(ctx context.Context, parentTag *models.TopicTag) {
	es := NewEmbeddingService()
	similar, err := es.FindSimilarAbstractTags(ctx, parentTag.ID, parentTag.Category, 5)
	if err != nil {
		logging.Warnf("L2 dedup: find similar tags for %d failed: %v", parentTag.ID, err)
		return
	}

	for _, candidate := range similar {
		if candidate.Tag == nil || candidate.Tag.ID == parentTag.ID {
			continue
		}
		if candidate.Similarity >= DedupL2Threshold {
			logging.Infof("L2 dedup: tag %d(%s) similar to %d(%s) at %.4f >= %.2f, merging",
				parentTag.ID, parentTag.Label, candidate.Tag.ID, candidate.Tag.Label, candidate.Similarity, DedupL2Threshold)
			if err := MergeTags(parentTag.ID, candidate.Tag.ID); err != nil {
				logging.Warnf("L2 dedup: merge %d into %d failed: %v", parentTag.ID, candidate.Tag.ID, err)
			}
			return
		}
	}
}

func dedupL1(ctx context.Context, parentTag *models.TopicTag) {
	es := NewEmbeddingService()
	similar, err := es.FindSimilarAbstractTags(ctx, parentTag.ID, parentTag.Category, 5)
	if err != nil {
		logging.Warnf("L1 dedup: find similar tags for %d failed: %v", parentTag.ID, err)
		return
	}

	for _, candidate := range similar {
		if candidate.Tag == nil || candidate.Tag.ID == parentTag.ID {
			continue
		}
		if candidate.Similarity >= DedupL1Threshold {
			shouldMerge, mergedName, err := callLLMForL1Dedup(ctx, parentTag, candidate.Tag)
			if err != nil {
				logging.Warnf("L1 dedup: LLM check failed for %d vs %d: %v", parentTag.ID, candidate.Tag.ID, err)
				continue
			}
			if shouldMerge {
				logging.Infof("L1 dedup: LLM confirmed merge %d(%s) into %d(%s) as '%s'",
					parentTag.ID, parentTag.Label, candidate.Tag.ID, candidate.Tag.Label, mergedName)
				if mergedName != "" && mergedName != parentTag.Label {
					database.DB.Model(parentTag).Update("label", mergedName)
				}
				if err := MergeTags(parentTag.ID, candidate.Tag.ID); err != nil {
					logging.Warnf("L1 dedup: merge %d into %d failed: %v", parentTag.ID, candidate.Tag.ID, err)
				}
			}
			return
		}
	}
}

func callLLMForL1Dedup(ctx context.Context, tag1, tag2 *models.TopicTag) (shouldMerge bool, mergedName string, err error) {
	prompt := buildL1DedupPrompt(tag1, tag2)

	router := airouter.NewRouter()
	req := airouter.ChatRequest{
		Capability: airouter.CapabilityTopicTagging,
		Messages: []airouter.Message{
			{Role: "system", Content: "You are a tag deduplication expert. Decide whether two L1 type tags should be merged."},
			{Role: "user", Content: prompt},
		},
		JSONMode:    true,
		Temperature: func() *float64 { f := 0.1; return &f }(),
		Metadata: map[string]any{
			"operation": "l1_dedup",
			"tag1_id":   tag1.ID,
			"tag2_id":   tag2.ID,
		},
	}

	result, err := router.Chat(ctx, req)
	if err != nil {
		return false, "", fmt.Errorf("LLM L1 dedup: %w", err)
	}

	var resp l1DedupResponse
	if err := json.Unmarshal([]byte(result.Content), &resp); err != nil {
		return false, "", fmt.Errorf("parse L1 dedup response: %w", err)
	}

	return resp.ShouldMerge, resp.MergedName, nil
}
