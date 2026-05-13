package narrative

import (
	"context"
	"fmt"

	"my-robot-backend/internal/domain/models"
	"my-robot-backend/internal/domain/tagging"
	"my-robot-backend/internal/platform/database"
	"my-robot-backend/internal/platform/logging"
)

const maxPairsPerNarrative = 5

func feedbackNarrativesToTags(outputs []NarrativeOutput) {
	sem := make(chan struct{}, 3)
	for _, out := range outputs {
		if len(out.RelatedTagIDs) < 2 {
			continue
		}
		sem <- struct{}{}
		go func(o NarrativeOutput) {
			defer func() { <-sem }()
			checkNarrativeEventTagClustering(o)
		}(out)
	}
	for i := 0; i < cap(sem); i++ {
		sem <- struct{}{}
	}
}

func checkNarrativeEventTagClustering(out NarrativeOutput) {
	defer func() {
		if r := recover(); r != nil {
			logging.Warnf("checkNarrativeEventTagClustering panic: %v", r)
		}
	}()

	var tags []models.TopicTag
	database.DB.Where("id IN ? AND category = ? AND status = ?", out.RelatedTagIDs, "event", "active").Find(&tags)
	if len(tags) < 2 {
		return
	}

	var eventTagIDs []uint
	for _, t := range tags {
		eventTagIDs = append(eventTagIDs, t.ID)
	}

	var relatedIDs []uint
	database.DB.Model(&models.TopicTagRelation{}).
		Where("(parent_id IN ? OR child_id IN ?) AND relation_type = ?", eventTagIDs, eventTagIDs, "abstract").
		Pluck("parent_id", &relatedIDs)
	var childIDs []uint
	database.DB.Model(&models.TopicTagRelation{}).
		Where("(parent_id IN ? OR child_id IN ?) AND relation_type = ?", eventTagIDs, eventTagIDs, "abstract").
		Pluck("child_id", &childIDs)
	relatedIDs = append(relatedIDs, childIDs...)
	relatedSet := make(map[uint]bool, len(relatedIDs))
	for _, id := range relatedIDs {
		relatedSet[id] = true
	}

	var unclusteredIDs []uint
	for _, id := range eventTagIDs {
		if !relatedSet[id] {
			unclusteredIDs = append(unclusteredIDs, id)
		}
	}
	if len(unclusteredIDs) < 2 {
		return
	}

	es := tagging.NewEmbeddingService()
	ctx := context.Background()

	pairsChecked := 0
	for i := 0; i < len(unclusteredIDs) && pairsChecked < maxPairsPerNarrative; i++ {
		for j := i + 1; j < len(unclusteredIDs) && pairsChecked < maxPairsPerNarrative; j++ {
			pairsChecked++
			idA, idB := unclusteredIDs[i], unclusteredIDs[j]

			var embA, embB models.TopicTagEmbedding
			if err := database.DB.Where("topic_tag_id = ? AND embedding_type = ?", idA, tagging.EmbeddingTypeSemantic).First(&embA).Error; err != nil {
				continue
			}
			if err := database.DB.Where("topic_tag_id = ? AND embedding_type = ?", idB, tagging.EmbeddingTypeSemantic).First(&embB).Error; err != nil {
				continue
			}

			sim, err := computeEmbeddingSimilarity(embA.EmbeddingVec, embB.EmbeddingVec)
			if err != nil {
				continue
			}

			thresholds := es.GetThresholds()

			if sim >= thresholds.LowSimilarity && sim < thresholds.HighSimilarity {
				logging.Infof("narrative-tag-feedback: event tags %d and %d have similarity %.4f (in middle band), triggering abstract extraction with narrative context",
					idA, idB, sim)

				narrativeContext := fmt.Sprintf("Narrative: %s\nSummary: %s", out.Title, out.Summary)
				triggerAbstractExtractionWithContext(ctx, idA, idB, sim, narrativeContext)
			}
		}
	}
}

func computeEmbeddingSimilarity(vecAStr, vecBStr string) (float64, error) {
	query := "SELECT ($1::vector <=> $2::vector) AS distance"
	var distance float64
	if err := database.DB.Raw(query, vecAStr, vecBStr).Scan(&distance).Error; err != nil {
		return 0, err
	}
	return 1.0 - distance, nil
}

func triggerAbstractExtractionWithContext(ctx context.Context, tagAID, tagBID uint, sim float64, narrativeContext string) {
	var tagA, tagB models.TopicTag
	if err := database.DB.First(&tagA, tagAID).Error; err != nil {
		return
	}
	if err := database.DB.First(&tagB, tagBID).Error; err != nil {
		return
	}

	candidates := []tagging.TagCandidate{
		{Tag: &tagA, Similarity: sim},
		{Tag: &tagB, Similarity: sim},
	}

	result, err := tagging.ExtractAbstractTag(ctx, candidates, tagA.Label, tagA.Category,
		tagging.WithNarrativeContext(narrativeContext))
	if err != nil || result == nil || !result.HasAction() {
		logging.Warnf("narrative-tag-feedback: tag judgment with context failed for %d+%d: %v", tagAID, tagBID, err)
		return
	}

	if result.HasMerge() {
		targetID := result.Merge.Target.ID
		sourceID := tagAID
		if targetID == tagAID {
			sourceID = tagBID
		}
		if sourceID == targetID {
			return
		}
		if mergeErr := tagging.MergeTags(sourceID, targetID); mergeErr != nil {
			logging.Warnf("narrative-tag-feedback: merge of %d into %d failed: %v", sourceID, targetID, mergeErr)
			return
		}
		logging.Infof("narrative-tag-feedback: merged tag %d into %d (AI judged same concept)", sourceID, targetID)

		for _, child := range result.MergeChildren {
			if child.ID != targetID {
				if mergeErr := tagging.MergeTags(child.ID, targetID); mergeErr != nil {
					logging.Warnf("narrative-tag-feedback: merge child %d into %d failed: %v", child.ID, targetID, mergeErr)
				}
			}
		}

		if !result.HasAbstract() {
			return
		}
	}

	if result.HasAbstract() {
		logging.Infof("narrative-tag-feedback: created abstract tag %d (%s) from narrative-driven clustering of %d+%d",
			result.Abstract.Tag.ID, result.Abstract.Tag.Label, tagAID, tagBID)
	}
}
