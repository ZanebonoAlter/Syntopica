package tagging

import (
	"context"

	"my-robot-backend/internal/domain/models"
	"my-robot-backend/internal/platform/logging"
)

const dedupThreshold = 0.95

// dedupAtDepth checks same-depth, same-concept tags for near-duplicates
// using embedding similarity. If similar enough, executes merge.
func dedupAtDepth(ctx context.Context, tag *models.TopicTag, depth int) {
	if tag.ConceptID == nil {
		return
	}

	es := NewEmbeddingService()
	similar, err := es.FindSimilarAbstractTags(ctx, tag.ID, tag.Category, 10)
	if err != nil {
		logging.Warnf("dedupAtDepth: find similar tags for %d failed: %v", tag.ID, err)
		return
	}

	for _, candidate := range similar {
		if candidate.Tag == nil || candidate.Tag.ID == tag.ID {
			continue
		}
		if candidate.Tag.Source != "abstract" {
			continue
		}
		if candidate.Tag.ConceptID == nil || *candidate.Tag.ConceptID != *tag.ConceptID {
			continue
		}
		candidateDepth := getTagDepthFromRoot(candidate.Tag.ID)
		if candidateDepth != depth {
			continue
		}
		if candidate.Similarity >= dedupThreshold {
			logging.Infof("dedupAtDepth: tag %d(%s) near-duplicate with %d(%s) at sim=%.4f, merging",
				tag.ID, tag.Label, candidate.Tag.ID, candidate.Tag.Label, candidate.Similarity)
			if err := MergeTags(tag.ID, candidate.Tag.ID); err != nil {
				logging.Warnf("dedupAtDepth: merge %d into %d failed: %v", tag.ID, candidate.Tag.ID, err)
			}
			return
		}
	}
}
