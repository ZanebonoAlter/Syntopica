package service

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/airouter"
	"syntopica-backend/internal/platform/logging"
	"syntopica-backend/internal/platform/tracing"
	tagging "syntopica-backend/internal/tagmanagement"
	"syntopica-backend/internal/topicgraph/repository"
)

func GenerateDailyReport(ctx context.Context, boardID uint, date time.Time) (*repository.BoardDailyReport, []repository.DailyReportSection, [][]repository.DailyReportThread, error) {
	ctx, span := tracing.Tracer(tracing.ServiceName).Start(ctx, "workflow.daily_report.generate")
	defer span.End()

	startOfDay := repository.NormalizeReportDate(date)

	// Step 1: Collect board event tags
	tags, articleCounts, err := collectBoardTags(boardID, date)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("collect board tags: %w", err)
	}
	if len(tags) == 0 {
		return nil, nil, nil, nil
	}

	// Build tag→articleIDs map (before dedup/filter changes the tag slice)
	tagArticleMap := make(map[uint][]uint, len(tags))
	for i, t := range tags {
		tagArticleMap[t.ID] = articleCounts[i]
	}

	// Step 2: Deduplicate
	tags = DeduplicateTags(ctx, tags, articleCounts)

	// Step 2.5: Quality filter
	tags = filterTagsByQuality(tags)
	if len(tags) == 0 {
		return nil, nil, nil, nil
	}

	// Step 3: Cluster via LLM (inject the board's durable narrative frames so
	// the LLM reuses them and carries a matched_topic_id per group).
	topicCfg := repository.LoadPersistentTopicConfig(repository.Repo.DB())
	existingTopics, anchorStats, err := repository.Repo.ListAnchorableTopicsByBoard(boardID, startOfDay, topicCfg)
	if err != nil {
		logging.Warnf("daily-report: load existing topics for board %d failed: %v", boardID, err)
		existingTopics = nil // non-fatal: cluster from scratch
	} else {
		logging.Infof("daily-report: board %d cluster anchors active=%d candidates=%d filtered_window=%d truncated_limit=%d",
			boardID, anchorStats.ActiveCount, anchorStats.CandidateCount,
			anchorStats.FilteredByWindow, anchorStats.TruncatedByLimit)
	}

	// Step 3.5: Load recent briefs for anchorable topics (active AND candidate
	// — candidate-topic-l2-gate) (Slice D — lane context injection). On failure,
	// degrade to label-only injection — briefs are purely an enrichment layer
	// and SHALL NOT block ClusterTags.
	const (
		briefsSinceDays   = 7
		briefsPerTopicCap = 5
	)
	var topicBriefs map[uint][]repository.TopicRecentBrief
	if existingTopics != nil {
		briefs, briefsErr := repository.Repo.ListTopicRecentBriefs(boardID, briefsSinceDays, briefsPerTopicCap)
		if briefsErr != nil {
			logging.Warnf("daily-report: load topic briefs for board %d failed (degrading to label-only): %v", boardID, briefsErr)
		} else {
			topicBriefs = briefs
			topicsWithBriefs := 0
			for _, items := range briefs {
				if len(items) > 0 {
					topicsWithBriefs++
				}
			}
			logging.Infof("daily-report: board %d topic briefs loaded: %d anchorable topics have recent content",
				boardID, topicsWithBriefs)
		}
	}
	// Lane-driven clustering (daily-report-lane-driven-clustering): bucket tags
	// by centroid distance (L1/L2/L3) instead of a single LLM pass. Load the
	// day's tag semantic embeddings so BucketTagsByCentroid can route each tag;
	// tags without an embedding degrade to the L3 new-narrative lane.
	tagIDs := make([]uint, len(tags))
	for i, t := range tags {
		tagIDs[i] = t.ID
	}
	tagEmbStr, embErr := repository.Repo.ListTagSemanticEmbeddings(tagIDs)
	tagEmb := make(map[uint][]float64, len(tagEmbStr))
	if embErr != nil {
		logging.Warnf("daily-report: load tag embeddings for board %d failed (tags without embeddings route to L3): %v", boardID, embErr)
	} else {
		for id, s := range tagEmbStr {
			if v, perr := parsePgVector(s); perr == nil && len(v) > 0 {
				tagEmb[id] = v
			}
		}
	}
	clusters, err := ClusterTagsLane(ctx, tags, existingTopics, tagEmb, topicBriefs, topicCfg)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("cluster tags (lane): %w", err)
	}

	// Step 4: Query yesterday's report for continuity
	prevReport := findPreviousReportBrief(boardID, startOfDay)

	// Step 5: Generate in parallel (A + C×K)
	type highlightsResult struct {
		data []repository.Highlight
		err  error
	}
	type threadsResult struct {
		clusterIdx   int
		data         []repository.Thread
		sectionTitle string
		err          error
	}

	highlightsCh := make(chan highlightsResult, 1)
	threadsCh := make(chan threadsResult, len(clusters))

	// Call A: Highlights
	go func() {
		data, err := GenerateHighlights(ctx, tags, clusters)
		highlightsCh <- highlightsResult{data: data, err: err}
	}()

	// Call C×K: Threads per cluster
	for i, cluster := range clusters {
		go func(idx int, c repository.ClusterGroup) {
			data, sectionTitle, err := GenerateClusterThreads(ctx, c, tags)
			threadsCh <- threadsResult{clusterIdx: idx, data: data, sectionTitle: sectionTitle, err: err}
		}(i, cluster)
	}

	// Collect highlights
	hr := <-highlightsCh
	if hr.err != nil {
		return nil, nil, nil, fmt.Errorf("generate highlights: %w", hr.err)
	}

	// Collect threads
	threadsByCluster := make(map[int][]repository.Thread, len(clusters))
	sectionTitleByCluster := make(map[int]string, len(clusters))
	for i := 0; i < len(clusters); i++ {
		tr := <-threadsCh
		cluster := clusters[tr.clusterIdx]
		// The LLM-authored daily section title survives even when threads fall
		// back to synthesis (spec fallback chain level 1 takes precedence).
		sectionTitleByCluster[tr.clusterIdx] = tr.sectionTitle
		if tr.err != nil {
			logging.Warnf("daily-report: threads failed for cluster %d: %v (falling back to tag-anchored synthesis)", tr.clusterIdx, tr.err)
			threadsByCluster[tr.clusterIdx] = synthesizeFallbackThreads(cluster, filterTagsByIDs(tags, cluster.TagIDs))
			continue
		}
		if len(tr.data) == 0 {
			// Fact-anchor empty response is legal ("宁可不写"), but never persist an
			// empty-shell section under a live persistent topic — synthesize one
			// tag-anchored thread so the section stays readable.
			logging.Warnf("daily-report: threads LLM returned empty for cluster '%s' (synthesizing tag-anchored fallback)", cluster.GroupName)
			threadsByCluster[tr.clusterIdx] = synthesizeFallbackThreads(cluster, filterTagsByIDs(tags, cluster.TagIDs))
			continue
		}
		threadsByCluster[tr.clusterIdx] = tr.data
	}

	// Step 6: Assemble report
	highlightsJSON, _ := json.Marshal(hr.data)

	// Calculate article count
	totalArticles := 0
	for _, t := range tags {
		totalArticles += t.ArticleCount
	}

	// Title: use first highlight if available, else board name + date
	title := fmt.Sprintf("日报 %s", startOfDay.Format("2006-01-02"))
	if len(hr.data) > 0 {
		title = hr.data[0].Title
	}

	// Summary: use first highlight reason as summary fallback
	summary := ""
	if len(hr.data) > 0 {
		summary = hr.data[0].Reason
	}

	clustersJSON, _ := json.Marshal(clusters)

	report := &repository.BoardDailyReport{
		SemanticBoardID:         boardID,
		PeriodDate:              startOfDay,
		Title:                   title,
		Summary:                 summary,
		Highlights:              highlightsJSON,
		Dynamics:                "",
		ArticleCount:            totalArticles,
		EventTagCount:           len(tags),
		ClusterCount:            len(clusters),
		Status:                  "completed",
		RawClusters:             clustersJSON,
		GenerationPromptVersion: promptVersion,
	}
	if prevReport != nil {
		report.PrevReportID = &prevReport.ID
	}

	// Lane-driven sections carry LaneTier + a ClusterLabel resolved from the
	// owning topic's label (L1/L2) or the new-narrative group name (L3).
	topicLabelByID := make(map[uint]string, len(existingTopics))
	for _, t := range existingTopics {
		topicLabelByID[t.ID] = t.Label
	}

	// Build sections
	var sections []repository.DailyReportSection
	for i, cluster := range clusters {
		threads := threadsByCluster[i]
		// Populate related article IDs
		populateThreadArticles(threads, tagArticleMap)
		clusterTags := filterTagsByIDs(tags, cluster.TagIDs)
		clusterArticleCount := 0
		for _, t := range clusterTags {
			clusterArticleCount += t.ArticleCount
		}

		tagIDsJSON := marshalJSONArray(cluster.TagIDs)

		// Calculate best_tier and avg_score
		tagIDSet := make(map[uint]bool)
		for _, tid := range cluster.TagIDs {
			tagIDSet[tid] = true
		}
		bestTier := 4 // worst possible
		totalScore := 0.0
		matchCount := 0
		for _, t := range tags {
			if tagIDSet[t.ID] {
				tier := tagging.MatchTier(t.MatchReason, t.Downgraded)
				if tier < bestTier {
					bestTier = tier
				}
				totalScore += t.Score
				matchCount++
			}
		}
		avgScore := 0.0
		if matchCount > 0 {
			avgScore = totalScore / float64(matchCount)
		}

		breakdownJSON := buildQualityBreakdownJSON(tags, cluster.TagIDs)

		clusterLabel := resolveClusterLabel(sectionTitleByCluster[i], threadsByCluster[i], cluster, topicLabelByID)
		sections = append(sections, repository.DailyReportSection{
			ClusterIndex:     i,
			ClusterLabel:     clusterLabel,
			ClusterTagIDs:    tagIDsJSON,
			ArticleCount:     clusterArticleCount,
			BestTier:         bestTier,
			AvgScore:         avgScore,
			QualityBreakdown: breakdownJSON,
			MatchedTopicID:   cluster.MatchedTopicID,
			LaneTier:         clusterLaneToTier(cluster.Lane),
		})
	}

	// Build thread batches (convert []repository.Thread → []repository.DailyReportThread per cluster)
	var threadBatches [][]repository.DailyReportThread
	for i := range clusters {
		threads := threadsByCluster[i]
		var batch []repository.DailyReportThread
		for _, th := range threads {
			tagIDsJSON, _ := json.Marshal(th.TagIDs)
			articleIDsJSON, _ := json.Marshal(th.RelatedArticleIDs)
			batch = append(batch, repository.DailyReportThread{
				Title:             th.Title,
				Summary:           th.Summary,
				TagIDs:            tagIDsJSON,
				Confidence:        th.Confidence,
				RelatedArticleIDs: articleIDsJSON,
			})
		}
		threadBatches = append(threadBatches, batch)
	}

	// Generate section embeddings from content-assembled texts (tags'
	// label/description/article excerpts; see buildSectionEmbedText). The
	// old cluster_label-text embedding froze lane-hit sections onto the topic
	// label and pinned centroids to the first-day title forever.
	var embedTexts []string
	var embedIndices []int
	for i := range sections {
		clusterTags := filterTagsByIDs(tags, mustUnmarshalTagIDs(sections[i].ClusterTagIDs))
		var threads []repository.DailyReportThread
		if i < len(threadBatches) {
			threads = threadBatches[i]
		}
		text := buildSectionEmbedText(clusterTags, threads, sections[i].ClusterLabel)
		if strings.TrimSpace(text) != "" {
			embedTexts = append(embedTexts, text)
			embedIndices = append(embedIndices, i)
		}
	}
	if len(embedTexts) > 0 {
		embedCtx, embedSpan := tracing.Tracer(tracing.ServiceName).Start(ctx, "workflow.daily_report.section_embedding")
		embedResult, embedErr := airouter.NewRouter().Embed(embedCtx, airouter.EmbeddingRequest{
			Input:     embedTexts,
			Operation: "section.embedding",
			SessionID: SessionIDFromContext(ctx),
			Metadata: map[string]any{
				"operation": "daily_report_section_embedding",
				"board_id":  boardID,
			},
		}, airouter.CapabilityEmbedding)
		embedSpan.End()
		if embedErr != nil {
			logging.Warnf("daily-report: section embedding failed for board %d: %v", boardID, embedErr)
		} else if len(embedResult.Embeddings) >= len(embedTexts) {
			for j, idx := range embedIndices {
				sections[idx].Embedding = repository.FloatsToPgVector(embedResult.Embeddings[j])
			}
		}
	}

	// Step 6.5: compute thread↔section title fit distances (System 3 observability).
	// Non-fatal: threads without fit signal render as normal on the frontend.
	computeThreadFitDistances(ctx, sections, threadBatches, boardID, airouter.NewRouter().Embed)

	// Step 7: Merge similar same-day sections (two-stage: embedding + LLM).
	// Gated by daily_report_section_merge_enabled (default off; see
	// fix-section-merge-blackhole) and constrained by the anchor boundary.
	sections, threadBatches, mergeErr := MergeSimilarSections(ctx, sections, threadBatches, tags, topicCfg.SectionMergeEnabled)
	if mergeErr != nil {
		logging.Warnf("daily-report: section merge failed for board %d: %v", boardID, mergeErr)
		// Non-fatal: continue with unmerged sections
	}

	// Step 7.5: Watch materialization (watch-materialized-topic) — append the
	// materialized-track sections after the regular pipeline and merge. Each
	// watch's failure degrades to a skip + warn (spec: 物化失败不阻断)； report-level
	// counters stay at the regular-clustering caliber (spec: 计数不重复). The
	// appended sections carry lane_tier watch_keyword / watch_sentence, which
	// downstream hooks (assignment/lifecycle, relations, hint tracks) use as
	// the exclusion/ownership marker.
	matWatches, matErr := repository.Repo.ListActiveMaterializedWatchesByBoard(boardID)
	if matErr != nil {
		logging.Warnf("daily-report: list materialized watches failed for board %d (skipping materialization): %v", boardID, matErr)
	} else if len(matWatches) > 0 {
		nextIdx := len(sections)
		appended := 0
		var kwWatches []repository.BoardTopicWatch
		sentenceCfg := LoadWatchSentenceConfig(repository.Repo.DB())
		// Adjudication config (watch-materialize-llm-adjudication): one shared
		// load serves both materialized tracks; the router provides Embed for
		// the sentence retrieval and Chat for the adjudication calls.
		matCfg := LoadWatchMaterializeConfig(repository.Repo.DB())
		router := airouter.NewRouter()
		for _, w := range matWatches {
			switch w.Type {
			case repository.WatchTypeKeywordTopic:
				kwWatches = append(kwWatches, w)
			case repository.WatchTypeSentenceTopic:
				sec, threads, sErr := MaterializeSentenceWatch(ctx, w, date, sentenceCfg, matCfg, router.Embed, router.Chat)
				if sErr != nil {
					logging.Warnf("daily-report: sentence materialization failed for watch %d (skipped): %v", w.ID, sErr)
					continue
				}
				if sec == nil {
					continue // no hit today — legal
				}
				sec.ClusterIndex = nextIdx + appended
				sections = append(sections, *sec)
				threadBatches = append(threadBatches, threads)
				appended++
			}
		}
		if len(kwWatches) > 0 {
			kwSections, kwBatches, kErr := MaterializeKeywordWatches(ctx, boardID, kwWatches, nextIdx+appended, matCfg, router.Chat)
			if kErr != nil {
				logging.Warnf("daily-report: keyword materialization failed for board %d (skipped): %v", boardID, kErr)
			} else {
				sections = append(sections, kwSections...)
				threadBatches = append(threadBatches, kwBatches...)
			}
		}
	}

	return report, sections, threadBatches, nil
}

const (
	maxContextArticles     = 3
	maxContextSummaryRunes = 200
)

// buildArticleContextForTag returns a compact "《title》summary; ..." string of up to
// maxContextArticles representative articles for the given tag in [start, end), to ground the
// daily report LLM prompts in actual event content. Summary precedence mirrors article_tagger's
// buildArticleSummary (AIContentSummary > FirecrawlContent > Content > Description) but is
// reimplemented here because buildArticleSummary is unexported in tagmanagement/service/core.
func buildArticleContextForTag(tagID uint, start, end time.Time) string {
	type articleRow struct {
		Title            string
		AIContentSummary string
		FirecrawlContent string
		Content          string
		Description      string
	}
	var rows []articleRow
	err := repository.Repo.DB().Model(&models.Article{}).
		Joins("JOIN article_topic_tags ON article_topic_tags.article_id = articles.id").
		Where("article_topic_tags.topic_tag_id = ? AND articles.pub_date >= ? AND articles.pub_date < ?", tagID, start, end).
		Order("articles.pub_date DESC").
		Limit(maxContextArticles).
		Find(&rows).Error
	if err != nil || len(rows) == 0 {
		return ""
	}
	var parts []string
	for _, r := range rows {
		summary := pickArticleSummary(r.AIContentSummary, r.FirecrawlContent, r.Content, r.Description)
		if strings.TrimSpace(summary) == "" {
			continue
		}
		runes := []rune(summary)
		if len(runes) > maxContextSummaryRunes {
			summary = string(runes[:maxContextSummaryRunes])
		}
		title := strings.TrimSpace(r.Title)
		if title == "" {
			parts = append(parts, summary)
		} else {
			parts = append(parts, fmt.Sprintf("《%s》%s", title, summary))
		}
	}
	return strings.Join(parts, " ; ")
}

// pickArticleSummary returns the first non-blank string in precedence order:
// AIContentSummary > FirecrawlContent > Content > Description.
func pickArticleSummary(fields ...string) string {
	for _, s := range fields {
		if trimmed := strings.TrimSpace(s); trimmed != "" {
			return trimmed
		}
	}
	return ""
}
func collectBoardTags(boardID uint, date time.Time) ([]repository.TagInput, [][]uint, error) {
	startOfDay := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, date.Location())
	endOfDay := startOfDay.Add(24 * time.Hour)

	type tagRow struct {
		ID           uint    `json:"id"`
		Label        string  `json:"label"`
		Category     string  `json:"category"`
		Description  string  `json:"description"`
		Source       string  `json:"source"`
		MatchReason  string  `json:"match_reason"`
		Score        float64 `json:"score"`
		Downgraded   bool    `json:"downgraded"`
		ArticleCount int     `json:"article_count"`
	}

	var rows []tagRow
	err := repository.Repo.DB().Model(&models.TopicTag{}).
		Select(`topic_tags.id AS id,
			topic_tags.label AS label,
			topic_tags.category AS category,
			topic_tags.description AS description,
			topic_tags.source AS source,
			topic_tag_board_labels.match_reason AS match_reason,
			topic_tag_board_labels.score AS score,
			topic_tag_board_labels.downgraded AS downgraded,
			COUNT(DISTINCT articles.id) AS article_count`).
		Joins("JOIN topic_tag_board_labels ON topic_tag_board_labels.topic_tag_id = topic_tags.id").
		Joins("JOIN article_topic_tags ON article_topic_tags.topic_tag_id = topic_tags.id").
		Joins("JOIN articles ON articles.id = article_topic_tags.article_id").
		Where("topic_tag_board_labels.semantic_board_id = ?", boardID).
		Where("NOT COALESCE(topic_tag_board_labels.direction_mismatch, false)").
		Where("topic_tags.status = ? AND topic_tags.category = ?", "active", models.TagCategoryEvent).
		Where("articles.pub_date >= ? AND articles.pub_date < ?", startOfDay, endOfDay).
		Group("topic_tags.id, topic_tags.label, topic_tags.category, topic_tags.description, topic_tags.source, topic_tag_board_labels.match_reason, topic_tag_board_labels.score, topic_tag_board_labels.downgraded").
		Order("article_count DESC, topic_tags.id ASC").
		Scan(&rows).Error
	if err != nil {
		return nil, nil, fmt.Errorf("query board event tags: %w", err)
	}

	if len(rows) == 0 {
		return nil, nil, nil
	}

	var tags []repository.TagInput
	var articleIDSets [][]uint
	for _, row := range rows {
		tags = append(tags, repository.TagInput{
			ID:             row.ID,
			Label:          row.Label,
			Category:       row.Category,
			Description:    row.Description,
			ArticleCount:   row.ArticleCount,
			Source:         row.Source,
			MatchReason:    row.MatchReason,
			Score:          row.Score,
			Downgraded:     row.Downgraded,
			ArticleContext: buildArticleContextForTag(row.ID, startOfDay, endOfDay),
		})

		// Get article IDs for this tag on this date
		var artIDs []uint
		repository.Repo.DB().Model(&models.ArticleTopicTag{}).
			Select("DISTINCT article_topic_tags.article_id").
			Joins("JOIN articles ON articles.id = article_topic_tags.article_id").
			Where("article_topic_tags.topic_tag_id = ? AND articles.pub_date >= ? AND articles.pub_date < ?",
				row.ID, startOfDay, endOfDay).
			Pluck("article_topic_tags.article_id", &artIDs)
		articleIDSets = append(articleIDSets, artIDs)
	}

	// Fallback: find event tags with auxiliaries but no board labels, compute matches
	var unmatchedTagIDs []uint
	repository.Repo.DB().Model(&models.TopicTag{}).
		Select("DISTINCT topic_tags.id").
		Joins("JOIN article_topic_tags ON article_topic_tags.topic_tag_id = topic_tags.id").
		Joins("JOIN articles ON articles.id = article_topic_tags.article_id").
		Joins("JOIN topic_tag_semantic_labels ON topic_tag_semantic_labels.topic_tag_id = topic_tags.id").
		Where("topic_tags.status = ? AND topic_tags.category = ?", "active", models.TagCategoryEvent).
		Where("articles.pub_date >= ? AND articles.pub_date < ?", startOfDay, endOfDay).
		Where("NOT EXISTS (SELECT 1 FROM topic_tag_board_labels WHERE topic_tag_board_labels.topic_tag_id = topic_tags.id)").
		Limit(50).
		Pluck("topic_tags.id", &unmatchedTagIDs)

	if len(unmatchedTagIDs) > 0 {
		logging.Infof("[daily-report] fallback: found %d unmatched event tags for board %d, computing matches", len(unmatchedTagIDs), boardID)
		matcher := tagging.NewSemanticBoardMatchingService(repository.Repo.DB())
		for _, tid := range unmatchedTagIDs {
			matches, matchErr := matcher.MatchTopicTag(context.Background(), tid)
			if matchErr != nil {
				logging.Warnf("[daily-report] fallback match failed for tag %d: %v", tid, matchErr)
				continue
			}
			var boardMatch *tagging.SemanticBoardMatchResult
			for i := range matches {
				if matches[i].SemanticBoardID == boardID && !matches[i].DirectionMismatch {
					boardMatch = &matches[i]
					break
				}
			}
			if boardMatch == nil {
				continue
			}

			var t models.TopicTag
			if err := repository.Repo.DB().First(&t, tid).Error; err != nil {
				continue
			}
			var artIDs []uint
			repository.Repo.DB().Model(&models.ArticleTopicTag{}).
				Select("DISTINCT article_topic_tags.article_id").
				Joins("JOIN articles ON articles.id = article_topic_tags.article_id").
				Where("article_topic_tags.topic_tag_id = ? AND articles.pub_date >= ? AND articles.pub_date < ?", tid, startOfDay, endOfDay).
				Pluck("article_topic_tags.article_id", &artIDs)

			tags = append(tags, repository.TagInput{
				ID:             t.ID,
				Label:          t.Label,
				Category:       t.Category,
				Description:    t.Description,
				Source:         t.Source,
				ArticleCount:   len(artIDs),
				MatchReason:    boardMatch.MatchReason,
				Score:          boardMatch.Score,
				Downgraded:     boardMatch.Downgraded,
				ArticleContext: buildArticleContextForTag(t.ID, startOfDay, endOfDay),
			})
			articleIDSets = append(articleIDSets, artIDs)
		}
	}

	return tags, articleIDSets, nil
}
func findPreviousReportBrief(boardID uint, date time.Time) *repository.BoardDailyReport {
	var report repository.BoardDailyReport
	err := repository.Repo.DB().Where("semantic_board_id = ? AND period_date < ? AND status = ?",
		boardID, repository.NormalizeReportDate(date).Format("2006-01-02"), "completed").
		First(&report).Error
	if err != nil {
		return nil
	}
	return &report
}

// buildQualityBreakdownJSON builds the quality_breakdown JSON from tags filtered by tagIDs.
// Each entry carries per-tag match detail: tag_id, label, match_reason, score, downgraded.
func buildQualityBreakdownJSON(tags []repository.TagInput, tagIDs []uint) []byte {
	tagIDSet := make(map[uint]bool, len(tagIDs))
	for _, tid := range tagIDs {
		tagIDSet[tid] = true
	}
	type qEntry struct {
		TagID       uint    `json:"tag_id"`
		Label       string  `json:"label"`
		MatchReason string  `json:"match_reason"`
		Score       float64 `json:"score"`
		Downgraded  bool    `json:"downgraded"`
	}
	entries := make([]qEntry, 0, len(tagIDs))
	for _, t := range tags {
		if tagIDSet[t.ID] {
			entries = append(entries, qEntry{
				TagID:       t.ID,
				Label:       t.Label,
				MatchReason: t.MatchReason,
				Score:       t.Score,
				Downgraded:  t.Downgraded,
			})
		}
	}
	b, _ := json.Marshal(entries)
	return b
}
func filterTagsByQuality(tags []repository.TagInput) []repository.TagInput {
	// Separate by match reason quality
	var kept []repository.TagInput
	var weighted []repository.TagInput
	for _, t := range tags {
		switch t.MatchReason {
		case "direct_hit", "hit_rate", "max_sim":
			kept = append(kept, t)
		default: // "weighted" or unknown
			weighted = append(weighted, t)
		}
	}

	// If kept < 10, pull back weighted tags (保底)
	if len(kept) < 10 {
		kept = append(kept, weighted...)
	}

	// If kept > 30, truncate by quality (截断)
	if len(kept) > 30 {
		sort.SliceStable(kept, func(i, j int) bool {
			ti := tagging.MatchTier(kept[i].MatchReason, kept[i].Downgraded)
			tj := tagging.MatchTier(kept[j].MatchReason, kept[j].Downgraded)
			if ti != tj {
				return ti < tj
			}
			return kept[i].Score > kept[j].Score
		})
		kept = kept[:30]
	}

	return kept
}

// resolveClusterLabel picks the section's display title via the fallback
// chain (spec: section 展示标题内容化): 1) LLM-authored daily section_title,
// 2) first thread title, 3) matched topic label (legacy behavior, bottom
// safety net), 4) the lane group name (L3 naming). The topic label is an
// anchor/fallback signal only — never the default display title. The winner
// is rune-truncated to the cluster_label column budget (gorm size:200).
func resolveClusterLabel(sectionTitle string, threads []repository.Thread, cluster repository.ClusterGroup, topicLabelByID map[uint]string) string {
	label := cluster.GroupName
	if t := strings.TrimSpace(sectionTitle); t != "" {
		label = t
	} else {
		fromThread := false
		for _, th := range threads {
			if t := strings.TrimSpace(th.Title); t != "" {
				label = t
				fromThread = true
				break
			}
		}
		if !fromThread && cluster.MatchedTopicID != nil {
			if topicLabel, ok := topicLabelByID[*cluster.MatchedTopicID]; ok && topicLabel != "" {
				label = topicLabel
			}
		}
	}
	if r := []rune(label); len(r) > 200 {
		label = string(r[:200])
	}
	return label
}

func filterTagsByIDs(tags []repository.TagInput, ids []uint) []repository.TagInput {
	idSet := make(map[uint]bool, len(ids))
	for _, id := range ids {
		idSet[id] = true
	}
	var result []repository.TagInput
	for _, t := range tags {
		if idSet[t.ID] {
			result = append(result, t)
		}
	}
	return result
}
