package scheduler

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"

	"syntopica-backend/internal/admin/repository"
	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/logging"
	"syntopica-backend/internal/platform/scheduler"
	tagging "syntopica-backend/internal/tagmanagement"
)

// TagQualityScoreJob recomputes persistent quality scores for topic tags.
func TagQualityScoreJob(ctx context.Context) (*scheduler.JobResult, error) {
	startTime := time.Now()

	if err := tagging.ComputeAllQualityScores(); err != nil {
		return nil, fmt.Errorf("failed to compute quality scores: %w", err)
	}

	// Reconcile: fix ref_count for auxiliary labels
	if result := repository.Repo.DB().Exec(`
		UPDATE semantic_labels
		SET ref_count = (
			SELECT COUNT(*) FROM topic_tag_semantic_labels
			WHERE semantic_label_id = semantic_labels.id
		)
		WHERE label_type = 'auxiliary'`); result.Error != nil {
		logging.Warnf("TagQualityScore: failed to reconcile auxiliary label ref_count: %v", result.Error)
	}

	// Reconcile: fix denormalized feed_count on topic tags. Tagging paths do
	// not maintain it incrementally (only hard-merge recomputes it), so
	// list/clustering ordering by feed_count would drift over time without
	// this periodic full recalculation.
	if err := RecalculateTopicTagFeedCounts(repository.Repo.DB()); err != nil {
		logging.Warnf("TagQualityScore: failed to reconcile topic tag feed_count: %v", err)
	}

	// Reconcile: remove orphan auxiliary labels
	var orphanAuxDeleted int64
	cutoff := time.Now().AddDate(0, 0, -1)
	if result := repository.Repo.DB().Where(
		"label_type = ? AND ref_count = 0 AND protected = false AND status = ? AND created_at < ? AND id NOT IN (SELECT auxiliary_label_id FROM board_composition)",
		"auxiliary", "active", cutoff,
	).Delete(&models.SemanticLabel{}); result.Error != nil {
		logging.Warnf("TagQualityScore: failed to clean orphan auxiliary labels: %v", result.Error)
	} else if result.RowsAffected > 0 {
		orphanAuxDeleted = result.RowsAffected
		logging.Infof("TagQualityScore: cleaned up %d orphan auxiliary labels", result.RowsAffected)
	}

	return &scheduler.JobResult{
		Data: map[string]interface{}{
			"updated_count":      0, // ComputedAllQualityScores doesn't return a count
			"orphan_aux_deleted": orphanAuxDeleted,
			"finished_at":        time.Now().Format(time.RFC3339),
			"started_at":         startTime.Format(time.RFC3339),
		},
		Summary: fmt.Sprintf("quality scores recomputed (orphan_aux_deleted=%d)", orphanAuxDeleted),
	}, nil
}

// RecalculateTopicTagFeedCounts rebuilds the denormalized feed_count on
// topic_tags as the number of DISTINCT feeds whose articles reference the tag
// via article_topic_tags. Called periodically by TagQualityScoreJob because
// tagging paths do not maintain feed_count incrementally (only hard-merge
// recomputes it), so ordering by feed_count would otherwise drift over time.
func RecalculateTopicTagFeedCounts(db *gorm.DB) error {
	return db.Exec(`
		UPDATE topic_tags
		SET feed_count = (
			SELECT COUNT(DISTINCT a.feed_id)
			FROM article_topic_tags att
			JOIN articles a ON a.id = att.article_id
			WHERE att.topic_tag_id = topic_tags.id
		)`).Error
}
