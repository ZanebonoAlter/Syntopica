package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"gorm.io/gorm"

	"syntopica-backend/internal/platform/airouter"
	"syntopica-backend/internal/platform/logging"
	"syntopica-backend/internal/platform/tracing"
	tagging "syntopica-backend/internal/tagmanagement"
	"syntopica-backend/internal/topicgraph/repository"
)

// 重新分桶（add-intra-lane-theme-bucketing design D7/D11）
//
// 对单个 (report, cluster_index) 桶组重跑两阶段分桶并整组替换 section 行：
//   - 事务开头取与 SaveReport 同键的 (board, date) advisory lock（D11）；
//   - 锁内复核报告 status 与目标 lane_tier（TOCTOU）；
//   - thread 行复用、只迁移 section_id（关键事件注记不丢）；
//   - 未达阈值 / 收敛为 1 组 → 200 + buckets=1 且**不写库**；
//   - 任一失败整体回滚（LLM/解析/DB/embedding）。

// RebucketResult is the API payload: the group's newest sections + bucket count.
type RebucketResult struct {
	Buckets  int
	Sections []repository.DailyReportSection
}

// rebucketEmbedFn is the swappable embedding hook for the rebucket path. The
// default performs the real airouter call; tests inject deterministic vectors.
var rebucketEmbedFn embedFunc = func(ctx context.Context, req airouter.EmbeddingRequest, capability airouter.Capability) (*airouter.EmbeddingResult, error) {
	return airouter.NewRouter().Embed(ctx, req, capability)
}

// RebucketSectionGroup re-runs intra-lane theme bucketing for the bucket group
// containing sectionID. See the file header for the transaction contract.
func RebucketSectionGroup(ctx context.Context, reportID, sectionID uint) (*RebucketResult, error) {
	ctx, span := tracing.Tracer(tracing.ServiceName).Start(ctx, "workflow.daily_report.rebucket")
	defer span.End()

	db := repository.Repo.DB()
	boardID, periodDate, err := repository.Repo.LoadRebucketGroupMeta(reportID, sectionID)
	if err != nil {
		return nil, err
	}

	var result RebucketResult
	err = db.Transaction(func(tx *gorm.DB) error {
		// Same key as SaveReport: a concurrent same-day rerun either finishes
		// before we read the group or waits for us to commit (D11).
		if lockErr := repository.LockDailyReportSave(tx, boardID, periodDate); lockErr != nil {
			return lockErr
		}
		group, loadErr := repository.LoadRebucketGroup(tx, reportID, sectionID)
		if loadErr != nil {
			return loadErr
		}
		if group.Report.Status != repository.ReportStatusCompleted {
			return repository.ErrRebucketReportNotDone
		}
		head := group.Sections[0]
		if strings.HasPrefix(head.LaneTier, "watch_") {
			return repository.ErrRebucketWatchGroup
		}

		// The group's cluster identity = union of its sections' tag ids.
		clusterTagIDs := groupTagIDs(group.Sections)

		// Below threshold: legal no-op (spec 未达阈值不改数据 / 不调 LLM).
		if _, trigger := planBucketCount(len(group.Threads), len(clusterTagIDs), loadBucketConfig(tx)); !trigger {
			result = RebucketResult{Buckets: 1, Sections: groupSectionsWithThreads(group)}
			return nil
		}

		plan, planErr := planRebucket(ctx, tx, group, clusterTagIDs)
		if planErr != nil {
			return planErr
		}
		if plan == nil {
			// Repair converged to one group: legal no-op (spec 拆不动时返回单桶).
			result = RebucketResult{Buckets: 1, Sections: groupSectionsWithThreads(group)}
			return nil
		}

		sections, applyErr := repository.ApplyRebucketGroup(tx, repository.RebucketApplySpec{
			ReportID:        reportID,
			BoardID:         boardID,
			PeriodDate:      periodDate,
			ClusterIndex:    head.ClusterIndex,
			TargetSectionID: sectionID,
			Buckets:         plan,
		})
		if applyErr != nil {
			return applyErr
		}
		result = RebucketResult{Buckets: len(sections), Sections: sections}
		return nil
	})
	if err != nil {
		return nil, err
	}

	// Post-commit centroid refresh (same pattern as SaveReport): the split
	// sections are purer than the merged one, so the anchor should follow.
	if len(result.Sections) > 1 {
		if topicID := result.Sections[0].PersistentTopicID; topicID != nil {
			if cerr := repository.Repo.UpdateCentroidOnSectionChange(nil, *topicID); cerr != nil {
				logging.Warnf("daily-report rebucket: centroid refresh failed for topic %d: %v", *topicID, cerr)
			}
		}
	}
	logging.Infof("daily-report rebucket: report %d group %d → %d bucket(s)", reportID, result.Sections[0].ClusterIndex, result.Buckets)
	return &result, nil
}

// loadBucketConfig reads the intra-lane bucketing config from ai_settings.
func loadBucketConfig(tx *gorm.DB) repository.PersistentTopicConfig {
	return repository.LoadPersistentTopicConfig(tx)
}

// groupSectionsWithThreads builds the no-op response payload: the group's
// sections with their threads attached (the reader replaces sections in place,
// so "threads is always an array" must hold here too).
func groupSectionsWithThreads(group *repository.RebucketGroup) []repository.DailyReportSection {
	bySection := make(map[uint][]repository.DailyReportThread, len(group.Sections))
	for _, thread := range group.Threads {
		bySection[thread.SectionID] = append(bySection[thread.SectionID], thread)
	}
	out := make([]repository.DailyReportSection, len(group.Sections))
	for i, section := range group.Sections {
		section.Threads = bySection[section.ID]
		if section.Threads == nil {
			section.Threads = []repository.DailyReportThread{}
		}
		out[i] = section
	}
	return out
}

// topicLabel returns the persistent topic's label ("" when absent).
func topicLabel(tx *gorm.DB, topicID *uint) string {
	if topicID == nil {
		return ""
	}
	var topic repository.BoardPersistentTopic
	if err := tx.Select("id, label").First(&topic, *topicID).Error; err != nil {
		return ""
	}
	return strings.TrimSpace(topic.Label)
}

// groupTagIDs returns the ordered union of all cluster_tag_ids in the group.
func groupTagIDs(sections []repository.DailyReportSection) []uint {
	var ids []uint
	seen := make(map[uint]bool)
	for _, sec := range sections {
		var sectionTagIDs []uint
		if len(sec.ClusterTagIDs) > 0 {
			_ = json.Unmarshal(sec.ClusterTagIDs, &sectionTagIDs)
		}
		for _, id := range sectionTagIDs {
			if seen[id] {
				continue
			}
			seen[id] = true
			ids = append(ids, id)
		}
	}
	return ids
}

// groupBreakdownEntries decodes the group's frozen quality_breakdown snapshots
// into per-tag inputs (dedup by tag id — sibling sections may repeat a tag).
// These carry the ONLY authoritative score/match_reason data for an existing
// report, so rebucket recomputes best_tier / avg_score / breakdown from them.
func groupBreakdownEntries(sections []repository.DailyReportSection) map[uint]repository.TagInput {
	out := make(map[uint]repository.TagInput)
	for _, sec := range sections {
		if len(sec.QualityBreakdown) == 0 {
			continue
		}
		var entries []struct {
			TagID       uint    `json:"tag_id"`
			Label       string  `json:"label"`
			MatchReason string  `json:"match_reason"`
			Score       float64 `json:"score"`
		}
		if err := json.Unmarshal(sec.QualityBreakdown, &entries); err != nil {
			continue
		}
		for _, e := range entries {
			if _, ok := out[e.TagID]; ok {
				continue
			}
			out[e.TagID] = repository.TagInput{
				ID:          e.TagID,
				Label:       e.Label,
				MatchReason: e.MatchReason,
				Score:       e.Score,
			}
		}
	}
	return out
}

// planRebucket builds the new bucket layout (titles, tag sets, thread ids and
// every derived field) inside the transaction. It returns (nil, nil) when the
// cluster cannot be split (below threshold / repair degenerated).
func planRebucket(ctx context.Context, tx *gorm.DB, group *repository.RebucketGroup, clusterTagIDs []uint) ([]repository.RebucketBucket, error) {
	head := group.Sections[0]

	// Group threads in the shape the bucket stages expect (line indexes refer to
	// this slice order, which is the DB id order — stable).
	plainThreads := make([]repository.Thread, len(group.Threads))
	for i, th := range group.Threads {
		var tagIDs []uint
		if len(th.TagIDs) > 0 {
			_ = json.Unmarshal(th.TagIDs, &tagIDs)
		}
		plainThreads[i] = repository.Thread{
			Title:      th.Title,
			Summary:    th.Summary,
			TagIDs:     tagIDs,
			Confidence: th.Confidence,
		}
	}

	tags, err := loadRebucketTags(tx, clusterTagIDs, groupBreakdownEntries(group.Sections))
	if err != nil {
		return nil, err
	}
	// Prompt header + last-resort title fallback: use the owning topic's label
	// (the lane the reader sees), not the head section's stale daily title.
	laneLabel := head.ClusterLabel
	if label := topicLabel(tx, head.PersistentTopicID); label != "" {
		laneLabel = label
	}
	cluster := repository.ClusterGroup{
		GroupName:      laneLabel,
		TagIDs:         clusterTagIDs,
		MatchedTopicID: head.PersistentTopicID,
		Lane:           laneFromTier(head.LaneTier),
	}

	cfg := loadBucketConfig(tx)
	buckets, err := BucketCluster(ctx, cluster, plainThreads, tags, laneLabel, cfg)
	if err != nil {
		return nil, err
	}
	if len(buckets) < 2 {
		return nil, nil
	}

	// Embedding text per bucket (its OWN tag set — never the whole cluster).
	threadByID := make(map[uint]repository.DailyReportThread, len(group.Threads))
	for _, th := range group.Threads {
		threadByID[th.ID] = th
	}
	var texts []string
	var textBucketIdx []int
	for i, bucket := range buckets {
		threads := make([]repository.DailyReportThread, 0, len(bucket.Threads))
		for _, line := range bucket.LineIndexes {
			threads = append(threads, threadByID[group.Threads[line].ID])
		}
		text := buildSectionEmbedText(filterTagsByIDs(tags, bucket.TagIDs), threads, bucket.SectionTitle)
		if strings.TrimSpace(text) == "" {
			continue
		}
		texts = append(texts, text)
		textBucketIdx = append(textBucketIdx, i)
	}
	embeddings := make([]string, len(buckets))
	if len(texts) > 0 {
		embedResult, embedErr := rebucketEmbedFn(ctx, airouter.EmbeddingRequest{
			Input:     texts,
			Operation: "section.embedding",
			SessionID: SessionIDFromContext(ctx),
			Metadata: map[string]any{
				"operation": "daily_report_rebucket_section_embedding",
				"report_id": group.Report.ID,
			},
		}, airouter.CapabilityEmbedding)
		if embedErr != nil {
			return nil, fmt.Errorf("rebucket section embedding: %w", embedErr)
		}
		if len(embedResult.Embeddings) < len(texts) {
			return nil, fmt.Errorf("rebucket section embedding: got %d vectors for %d texts", len(embedResult.Embeddings), len(texts))
		}
		for j, idx := range textBucketIdx {
			embeddings[idx] = repository.FloatsToPgVector(embedResult.Embeddings[j])
		}
	}

	plan := make([]repository.RebucketBucket, len(buckets))
	for i, bucket := range buckets {
		bestTier, avgScore := bucketQualityScores(tags, bucket.TagIDs)
		threadIDs := make([]uint, 0, len(bucket.LineIndexes))
		articleSet := make(map[uint]bool)
		for _, line := range bucket.LineIndexes {
			th := group.Threads[line]
			threadIDs = append(threadIDs, th.ID)
			var articleIDs []uint
			if len(th.RelatedArticleIDs) > 0 {
				_ = json.Unmarshal(th.RelatedArticleIDs, &articleIDs)
			}
			for _, id := range articleIDs {
				articleSet[id] = true
			}
		}
		plan[i] = repository.RebucketBucket{
			SectionTitle:     bucket.SectionTitle,
			BucketTitle:      bucket.Title,
			TagIDs:           bucket.TagIDs,
			ArticleCount:     len(articleSet),
			BestTier:         bestTier,
			AvgScore:         avgScore,
			QualityBreakdown: buildQualityBreakdownJSON(tags, bucket.TagIDs),
			Embedding:        embeddings[i],
			ThreadIDs:        threadIDs,
		}
	}
	return plan, nil
}

// loadRebucketTags merges the DB label/description with the report's frozen
// breakdown scores, preserving the cluster tag order.
func loadRebucketTags(tx *gorm.DB, clusterTagIDs []uint, breakdown map[uint]repository.TagInput) ([]repository.TagInput, error) {
	dbTags, err := repository.ListTagInputsByIDs(tx, clusterTagIDs)
	if err != nil {
		return nil, err
	}
	tags := make([]repository.TagInput, 0, len(clusterTagIDs))
	for _, id := range clusterTagIDs {
		tag := dbTags[id]
		tag.ID = id
		if entry, ok := breakdown[id]; ok {
			if strings.TrimSpace(tag.Label) == "" {
				tag.Label = entry.Label
			}
			tag.MatchReason = entry.MatchReason
			tag.Score = entry.Score
		}
		tags = append(tags, tag)
	}
	return tags, nil
}

// bucketQualityScores recomputes best_tier / avg_score from the bucket's tags
// (mirrors the orchestrator's computation; worst tier is 4).
func bucketQualityScores(tags []repository.TagInput, tagIDs []uint) (int, float64) {
	idSet := make(map[uint]bool, len(tagIDs))
	for _, id := range tagIDs {
		idSet[id] = true
	}
	bestTier := 4
	totalScore := 0.0
	count := 0
	for _, tag := range tags {
		if !idSet[tag.ID] {
			continue
		}
		if tier := tagging.MatchTier(tag.MatchReason); tier < bestTier {
			bestTier = tier
		}
		totalScore += tag.Score
		count++
	}
	if count == 0 {
		return bestTier, 0
	}
	return bestTier, totalScore / float64(count)
}

// laneFromTier maps a persisted lane_tier back to the ClusterGroup lane label.
func laneFromTier(tier string) string {
	switch tier {
	case laneL1Direct:
		return "l1"
	case laneL2LLM:
		return "l2"
	case laneL3New:
		return "l3"
	default:
		return ""
	}
}
