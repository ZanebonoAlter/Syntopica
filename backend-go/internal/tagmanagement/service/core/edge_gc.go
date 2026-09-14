package core

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"

	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/logging"
	"syntopica-backend/internal/tagmanagement/repository"
)

// DefaultTagEdgeRetentionDays is the fallback retention window (in days) for
// article_topic_tags edges when ai_settings has no usable value.
const DefaultTagEdgeRetentionDays = 7

// TagEdgeRetentionDaysKey is the shared ai_settings key driving three
// consumers with one wording (design D2): the edge GC window, the daily report
// backfill scan window and the rebuild guard lower bound. Edge present =
// candidates trustworthy = rebuildable; edge reclaimed = not trustworthy =
// rebuild rejected. A single key keeps the three from drifting apart.
const TagEdgeRetentionDaysKey = "tag_edge_retention_days"

// EdgeGCRequest configures one edge GC pass.
type EdgeGCRequest struct {
	// RetentionDays is how many days an edge is kept. Values <= 0 fall back to
	// DefaultTagEdgeRetentionDays with a warn (callers should normally pass
	// LoadTagEdgeRetentionDays, which already handles the fallback).
	RetentionDays int
	// Now overrides the clock used to compute the cutoff. Tests set it so the
	// "created_at exactly at the cutoff" case is deterministic; the zero value
	// means time.Now().
	Now time.Time
}

// EdgeGCResult reports what one edge GC pass did.
type EdgeGCResult struct {
	RetentionDays int       `json:"retention_days"`
	Cutoff        time.Time `json:"cutoff"`
	DeletedEdges  int64     `json:"deleted_edges"`
	AffectedTags  int       `json:"affected_tags"`
	OrphanedTags  int       `json:"orphaned_tags"`
}

// EdgeGC reclaims article_topic_tags edges older than the retention window and
// then removes the topic tags left without any edge (tag edge time-window GC,
// design D3). Deletion is strictly older-than: an edge whose created_at equals
// the cutoff (now - RetentionDays*24h) is kept.
//
// Archiving no longer deletes edges (design D4); this pass is the single owner
// of edge removal and therefore of orphan tag cleanup.
func EdgeGC(ctx context.Context, req EdgeGCRequest) (EdgeGCResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	retentionDays := req.RetentionDays
	if retentionDays <= 0 {
		logging.Warnf("edge_gc: invalid retention days %d; using default %d", retentionDays, DefaultTagEdgeRetentionDays)
		retentionDays = DefaultTagEdgeRetentionDays
	}

	now := req.Now
	if now.IsZero() {
		now = time.Now()
	}
	cutoff := now.Add(-time.Duration(retentionDays) * 24 * time.Hour)

	result := EdgeGCResult{RetentionDays: retentionDays, Cutoff: cutoff}

	db := repository.Repo.DB().WithContext(ctx)

	// Collect the affected tags before deleting: once the edges are gone there
	// is no way back to their topic_tag_id.
	var affectedTagIDs []uint
	if err := db.Model(&models.ArticleTopicTag{}).
		Where("created_at < ?", cutoff).
		Distinct().
		Pluck("topic_tag_id", &affectedTagIDs).Error; err != nil {
		return result, fmt.Errorf("collect expired tag edges: %w", err)
	}
	result.AffectedTags = len(affectedTagIDs)
	if len(affectedTagIDs) == 0 {
		return result, nil
	}

	deleted := db.Where("created_at < ?", cutoff).Delete(&models.ArticleTopicTag{})
	if deleted.Error != nil {
		return result, fmt.Errorf("delete expired tag edges: %w", deleted.Error)
	}
	result.DeletedEdges = deleted.RowsAffected

	// Snapshot the orphan candidates before delegating the actual deletion —
	// countOrphanedTags uses the same predicate as CleanupOrphanedTags, so the
	// reported number matches what gets removed.
	result.OrphanedTags = countOrphanedTags(db, affectedTagIDs)
	CleanupOrphanedTags(affectedTagIDs)

	return result, nil
}

// countOrphanedTags counts the tags in tagIDs that have no remaining edge. It
// mirrors the predicate inside CleanupOrphanedTags (which deletes but does not
// report a count).
func countOrphanedTags(db *gorm.DB, tagIDs []uint) int {
	if len(tagIDs) == 0 {
		return 0
	}
	var orphanIDs []uint
	if err := db.Model(&models.TopicTag{}).
		Where("id IN ?", tagIDs).
		Where("id NOT IN (SELECT topic_tag_id FROM article_topic_tags)").
		Pluck("id", &orphanIDs).Error; err != nil {
		logging.Warnf("edge_gc: counting orphaned topic tags failed: %v", err)
		return 0
	}
	return len(orphanIDs)
}

// LoadTagEdgeRetentionDays reads the shared retention window from ai_settings
// (key TagEdgeRetentionDaysKey). Reading is best-effort: a missing row, a
// non-numeric value or a value <= 0 falls back to DefaultTagEdgeRetentionDays
// with a warn and never blocks the caller (mirrors the persistent_topic_*
// pattern).
func LoadTagEdgeRetentionDays(db *gorm.DB) int {
	if db == nil {
		logging.Warnf("tag_edge_retention_days: nil db; using default %d", DefaultTagEdgeRetentionDays)
		return DefaultTagEdgeRetentionDays
	}

	var setting models.AISettings
	if err := db.Where("key = ?", TagEdgeRetentionDaysKey).First(&setting).Error; err != nil {
		logging.Warnf("tag_edge_retention_days: %q unavailable (%v); using default %d", TagEdgeRetentionDaysKey, err, DefaultTagEdgeRetentionDays)
		return DefaultTagEdgeRetentionDays
	}

	days, err := strconv.Atoi(strings.TrimSpace(setting.Value))
	if err != nil || days <= 0 {
		logging.Warnf("tag_edge_retention_days: invalid value %q; using default %d", setting.Value, DefaultTagEdgeRetentionDays)
		return DefaultTagEdgeRetentionDays
	}
	return days
}
