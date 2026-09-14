package scheduler

import (
	"context"
	"fmt"

	"syntopica-backend/internal/admin/repository"
	"syntopica-backend/internal/platform/logging"
	tagging "syntopica-backend/internal/tagmanagement"
)

// AuxLabelCleanupJob disables auxiliary labels with no active topic_tag
// references, then reclaims the tag edges that fell out of the retention
// window (offline-catchup design D1/D3).
//
// Both steps are the same domain (topic-tag derived-data hygiene) at the same
// hour-scale cadence, so they share one job instead of adding a second
// scheduler entry. Archiving no longer deletes edges (design D4), which makes
// this pass the single owner of edge removal — and therefore of the orphan tag
// cleanup that follows it.
func AuxLabelCleanupJob(ctx context.Context) (*JobResult, error) {
	service := tagging.NewAuxiliaryLabelService(repository.Repo.DB(), nil)
	result, err := service.GC(ctx, tagging.AuxLabelGCRequest{
		Mode:      tagging.AuxLabelGCModeDisable,
		GraceDays: 1,
	})
	if err != nil {
		return nil, fmt.Errorf("aux label GC failed: %w", err)
	}

	data := map[string]interface{}{
		"last_disabled_count": result.AffectedCount,
	}
	summary := fmt.Sprintf("disabled %d labels", result.AffectedCount)

	// Second step: delete article_topic_tags older than the shared retention
	// window (tag_edge_retention_days) and reclaim the tags they orphaned.
	edgeResult, edgeErr := tagging.EdgeGC(ctx, tagging.EdgeGCRequest{
		RetentionDays: tagging.LoadTagEdgeRetentionDays(repository.Repo.DB()),
	})
	if edgeErr != nil {
		// The aux label pass already committed and is this job's primary duty;
		// a failing sibling step is logged and surfaced, never escalated into a
		// whole-job failure (松耦合: one step must not fail the run).
		logging.Warnf("aux_label_cleanup: tag edge GC failed: %v", edgeErr)
		data["edge_gc_error"] = edgeErr.Error()
	} else {
		data["edge_deleted_count"] = edgeResult.DeletedEdges
		summary = fmt.Sprintf("%s, reclaimed %d tag edges (%d orphaned tags)",
			summary, edgeResult.DeletedEdges, edgeResult.OrphanedTags)
	}

	return &JobResult{
		Data:    data,
		Summary: summary,
	}, nil
}
