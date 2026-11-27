package service

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/airouter"
	"syntopica-backend/internal/platform/logging"
	"syntopica-backend/internal/platform/tracing"
	tagging "syntopica-backend/internal/tagmanagement"
	"syntopica-backend/internal/topicgraph/repository"

	"gorm.io/gorm"
)

func GenerateDailyReport(ctx context.Context, boardID uint, date time.Time) (*repository.BoardDailyReport, []repository.DailyReportSection, [][]repository.DailyReportThread, error) {
	ctx, span := tracing.Tracer(tracing.ServiceName).Start(ctx, "workflow.daily_report.generate")
	defer span.End()

	startOfDay := repository.NormalizeReportDate(date)

	// Step 1: Collect board event tags (both channels: board-mounted + lane recall)
	tags, articleCounts, laneRecalled, err := collectBoardTags(boardID, date)
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

	// Step 2: Deduplicate (fix-digest-tag-leak: tiebreak expanded to
	// article_count > identity×board cosine > id; any embedding load failure
	// degrades to the legacy ID order — the pipeline must not break).
	tags = DeduplicateTags(ctx, tags, articleCounts, loadDedupRelevance(boardID, tags))

	// Step 2.5: Quality filter (lane-recalled tags pass regardless of the
	// `kept >= 10` board-evidence switch — lane-first-recall D3)
	tags = filterTagsByQuality(tags, laneRecalled)
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
		briefs, briefsErr := repository.Repo.ListTopicRecentBriefs(boardID, startOfDay, briefsSinceDays, briefsPerTopicCap)
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
	clusters, err := ClusterTagsLane(ctx, tags, existingTopics, tagEmb, topicBriefs, topicCfg, LoadLaneConstraintOption(existingTopics, tagIDs), WithLaneRecalledTags(laneRecalled))
	if err != nil {
		return nil, nil, nil, fmt.Errorf("cluster tags (lane): %w", err)
	}

	if inputs, ok := ctx.Value(reportHighlightInputsKey{}).(*reportHighlightInputs); ok {
		inputs.tags = tags
		inputs.clusters = clusters
	}

	// Step 4: Query yesterday's report for continuity
	prevReport := findPreviousReportBrief(boardID, startOfDay)

	// Step 5: Generate threads in parallel (C×K). Highlights wait for saved identities.
	type threadsResult struct {
		clusterIdx   int
		data         []repository.Thread
		sectionTitle string
		err          error
	}

	threadsCh := make(chan threadsResult, len(clusters))

	// Call C×K: Threads per cluster
	for i, cluster := range clusters {
		go func(idx int, c repository.ClusterGroup) {
			data, sectionTitle, err := GenerateClusterThreads(ctx, c, tags)
			threadsCh <- threadsResult{clusterIdx: idx, data: data, sectionTitle: sectionTitle, err: err}
		}(i, cluster)
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

	// Step 6: Assemble with a temporary title and empty highlights. The saved
	// report is finalized only after bounded attribution and display dedup.

	// Calculate article count
	totalArticles := 0
	for _, t := range tags {
		totalArticles += t.ArticleCount
	}

	title := fmt.Sprintf("日报 %s", startOfDay.Format("2006-01-02"))

	clustersJSON, _ := json.Marshal(clusters)

	report := &repository.BoardDailyReport{
		SemanticBoardID:         boardID,
		PeriodDate:              startOfDay,
		Title:                   title,
		Summary:                 "",
		Highlights:              []byte("[]"),
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
	//
	// Step 5.5 — intra-lane theme bucketing (add-intra-lane-theme-bucketing):
	// an over-wide cluster's day is split into 2-6 theme directions AFTER its
	// threads exist (threads are the grouping unit) and BEFORE section
	// embeddings are computed (each bucket embeds its own tag set). Any failure
	// or a repair that converges back to one group keeps the current single
	// section — bucketing never blocks the report (spec 分桶失败不阻断流水线).
	plans := resolveSectionPlans(ctx, clusters, threadsByCluster, sectionTitleByCluster, tags, topicLabelByID, topicCfg)

	// 疑似 AI 文章的排序后置（design D8）：只影响线索文章在展开列表里的先后，不改
	// related_article_ids 的成员与条数（红线 45）。先算一次候选文章里的疑似集合，
	// 避免每篇文章一次查询；查询失败降级为空集（不重排、不阻断日报生成）。
	suspectArticleIDs := loadSuspectArticleSet(uniqueUintIDs(threadCandidateArticleIDs(tagArticleMap)))

	var sections []repository.DailyReportSection
	for pi := range plans {
		plan := &plans[pi]
		cluster := clusters[plan.clusterIdx]
		threads := plan.threads
		// Populate related article IDs
		populateThreadArticles(threads, tagArticleMap, suspectArticleIDs)
		plan.threads = threads
		clusterTags := filterTagsByIDs(tags, plan.tagIDs)
		clusterArticleCount := 0
		for _, t := range clusterTags {
			clusterArticleCount += t.ArticleCount
		}

		tagIDsJSON := marshalJSONArray(plan.tagIDs)

		// Calculate best_tier and avg_score
		tagIDSet := make(map[uint]bool)
		for _, tid := range plan.tagIDs {
			tagIDSet[tid] = true
		}
		bestTier := 4 // worst possible
		totalScore := 0.0
		matchCount := 0
		for _, t := range tags {
			if tagIDSet[t.ID] {
				tier := tagging.MatchTier(t.MatchReason)
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

		breakdownJSON := buildQualityBreakdownJSON(tags, plan.tagIDs)

		clusterLabel := resolveClusterLabel(plan.sectionTitle, threads, cluster, topicLabelByID)
		sections = append(sections, repository.DailyReportSection{
			ClusterIndex:     plan.clusterIdx,
			ClusterLabel:     clusterLabel,
			ClusterTagIDs:    tagIDsJSON,
			ArticleCount:     clusterArticleCount,
			BestTier:         bestTier,
			AvgScore:         avgScore,
			QualityBreakdown: breakdownJSON,
			BucketTitle:      plan.bucketTitle,
			BucketIndex:      plan.bucketIndex,
			MatchedTopicID:   cluster.MatchedTopicID,
			LaneTier:         clusterLaneToTier(cluster.Lane),
		})
	}

	// Build thread batches (convert []repository.Thread → []repository.DailyReportThread per section)
	var threadBatches [][]repository.DailyReportThread
	for pi := range plans {
		threads := plans[pi].threads
		var batch []repository.DailyReportThread
		for _, th := range threads {
			tagIDsJSON := marshalJSONArray(th.TagIDs)
			articleIDsJSON := marshalJSONArray(th.RelatedArticleIDs)
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
			kwSections, kwBatches, kErr := MaterializeKeywordWatches(ctx, boardID, kwWatches, date, nextIdx+appended, matCfg, router.Chat)
			if kErr != nil {
				logging.Warnf("daily-report: keyword materialization failed for board %d (skipped): %v", boardID, kErr)
			} else {
				sections = append(sections, kwSections...)
				threadBatches = append(threadBatches, kwBatches...)
			}
		}
	}

	// Last step before the write: drop references to articles that vanished
	// between candidate collection and this point (a concurrent duplicate merge
	// or feed deletion). It runs here — after watch materialization appended its
	// own threads — so both the clustered and the materialized references are
	// covered by one probe (heal-dangling-article-refs D3).
	filterVanishedArticleRefs(repository.Repo.DB(), threadBatches)

	return report, sections, threadBatches, nil
}

// resolveHighlightsPayload maps the highlights generation result to the value
// persisted in board_daily_reports.highlights.
//
// A failed generation degrades to an empty JSON array — NEVER a JSON null
// scalar (a null scalar breaks jsonb_array_elements_text and makes the
// frontend hide the whole "今日重点" block). The second return value reports
// whether the report degraded, so the caller can log it. Degradation never
// fabricates highlight content out of other fields.
//
// Pure function (no DB / airouter access) so the mapping is unit-testable.
func resolveHighlightsPayload(data []repository.Highlight, err error) ([]byte, bool) {
	if err != nil {
		return marshalJSONArray(nil), true
	}
	return marshalJSONArray(data), false
}

// sectionPlan is one section the orchestrator will assemble. A regular cluster
// yields exactly one plan; a bucketed cluster yields one plan per theme
// direction (same clusterIdx, ascending bucketIndex).
type sectionPlan struct {
	clusterIdx   int
	threads      []repository.Thread
	tagIDs       []uint
	bucketTitle  string
	bucketIndex  int
	sectionTitle string
}

// resolveSectionPlans is the Step 5.5 bucket planner: for every cluster it asks
// BucketCluster whether the day's threads split into theme directions. Any
// failure (LLM / JSON / repair) degrades to the single-section plan, so the
// report never fails because of bucketing (spec 分桶失败不阻断流水线).
//
// Kept free of DB/airouter side effects apart from the swappable bucketChatFn,
// so the single-vs-split decision is unit-testable.
func resolveSectionPlans(ctx context.Context, clusters []repository.ClusterGroup, threadsByCluster map[int][]repository.Thread, sectionTitles map[int]string, tags []repository.TagInput, topicLabels map[uint]string, cfg repository.PersistentTopicConfig) []sectionPlan {
	plans := make([]sectionPlan, 0, len(clusters))
	for i, cluster := range clusters {
		clusterThreads := threadsByCluster[i]
		// L1/L2 groups arrive with an empty GroupName (assembleLaneGroups keys them
		// by topic). Fill the lane label so the bucketing prompt sees which lane it
		// is splitting and the title fallback can use it (spec 兜底链).
		laneLabel := strings.TrimSpace(cluster.GroupName)
		if laneLabel == "" && cluster.MatchedTopicID != nil {
			laneLabel = topicLabels[*cluster.MatchedTopicID]
		}
		buckets, bucketErr := BucketCluster(ctx, cluster, clusterThreads, tags, laneLabel, cfg)
		if bucketErr != nil {
			logging.Warnf("daily-report: intra-lane bucketing failed for cluster %d '%s' (keeping single section): %v", i, cluster.GroupName, bucketErr)
		}
		if len(buckets) < 2 {
			plans = append(plans, sectionPlan{
				clusterIdx: i, threads: clusterThreads, tagIDs: cluster.TagIDs,
				sectionTitle: sectionTitles[i],
			})
			continue
		}
		for bi, bucket := range buckets {
			plans = append(plans, sectionPlan{
				clusterIdx: i, threads: bucket.Threads, tagIDs: bucket.TagIDs,
				bucketTitle: bucket.Title, bucketIndex: bi,
				sectionTitle: bucket.SectionTitle,
			})
		}
	}
	return plans
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

// dailyReportCollectConfig controls the tag-collection stage of daily report
// generation. ExclusiveGap implements cross-board exclusivity
// (fix-board-match-cross-domain): a tag whose mount on another board outscores
// the current board's mount by at least the gap is excluded from this board's
// candidates — weak attachments must not pollute the report when a clearly
// stronger home exists. Gap <= 0 disables exclusivity entirely.
type dailyReportCollectConfig struct {
	ExclusiveGap float64
}

const semanticBoardReportExclusiveGapKey = "semantic_board_report_exclusive_gap"

// loadDailyReportCollectConfig reads the collection config from ai_settings,
// falling back to defaults on missing/invalid rows (same mechanism as
// LoadWatchMaterializeConfig).
func loadDailyReportCollectConfig(db *gorm.DB) dailyReportCollectConfig {
	cfg := dailyReportCollectConfig{ExclusiveGap: 0.3}
	if db == nil {
		return cfg
	}
	var row struct {
		Value string
	}
	if err := db.Table("ai_settings").
		Select("value").
		Where("key = ?", semanticBoardReportExclusiveGapKey).
		Take(&row).Error; err != nil {
		return cfg
	}
	if v, err := strconv.ParseFloat(strings.TrimSpace(row.Value), 64); err == nil && v >= 0 {
		cfg.ExclusiveGap = v
	}
	return cfg
}

// laneRecallTagReason marks tags whose only admission evidence is the lane
// channel (lane-first-recall). It is deliberately a new match_reason instead of
// borrowing "weighted": the quality filter, the quality_breakdown payload and
// the L3 gate all need to tell "board statistical guess" from "lane semantic
// witness" apart. Unknown reasons degrade safely downstream (MatchTier → the
// weighted tier, the report UI → a neutral chip).
const laneRecallTagReason = "lane_recall"

// collectBoardTags collects the day's candidate tags from BOTH recall channels
// (lane-first-recall design D1):
//
//   - board channel (existing): tags mounted on this board, plus a fallback that
//     live-matches a random `Limit(50)` slice of unmounted tags;
//   - lane channel (new): for every active lane, the nearest at most
//     `lane_recall_top_k` tags from the day's FULL event tag pool within
//     `lane_recall_max_distance`.
//
// The returned `laneRecalled` set marks the tags whose seat in the candidate set
// rests on lane evidence; `filterTagsByQuality` lets those through the
// `kept >= 10` switch and the L3 gate refuses to let them seed a new lane.
//
// The lane channel SHALL NOT write topic_tag_board_labels: a mount is a permanent
// fact, a recall is a single-day decision.
func collectBoardTags(boardID uint, date time.Time) ([]repository.TagInput, [][]uint, map[uint]bool, error) {
	startOfDay := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, date.Location())
	endOfDay := startOfDay.Add(24 * time.Hour)
	collectCfg := loadDailyReportCollectConfig(repository.Repo.DB())

	type tagRow struct {
		ID           uint    `json:"id"`
		Label        string  `json:"label"`
		Category     string  `json:"category"`
		Description  string  `json:"description"`
		Source       string  `json:"source"`
		MatchReason  string  `json:"match_reason"`
		Score        float64 `json:"score"`
		ArticleCount int     `json:"article_count"`
	}

	var rows []tagRow
	boardTagsQuery := repository.Repo.DB().Model(&models.TopicTag{}).
		Select(`topic_tags.id AS id,
			topic_tags.label AS label,
			topic_tags.category AS category,
			topic_tags.description AS description,
			topic_tags.source AS source,
			topic_tag_board_labels.match_reason AS match_reason,
			topic_tag_board_labels.score AS score,
			COUNT(DISTINCT articles.id) AS article_count`).
		Joins("JOIN topic_tag_board_labels ON topic_tag_board_labels.topic_tag_id = topic_tags.id").
		Joins("JOIN article_topic_tags ON article_topic_tags.topic_tag_id = topic_tags.id").
		Joins("JOIN articles ON articles.id = article_topic_tags.article_id").
		Where("topic_tag_board_labels.semantic_board_id = ?", boardID).
		Where("NOT COALESCE(topic_tag_board_labels.direction_mismatch, false)").
		Where("topic_tags.status = ? AND topic_tags.category = ?", "active", models.TagCategoryEvent).
		Where("articles.pub_date >= ? AND articles.pub_date < ?", startOfDay, endOfDay)
	// Cross-board exclusivity (fix-board-match-cross-domain): skip tags whose
	// mount here is outsold by another board by >= ExclusiveGap — e.g. a 0.7
	// borrowed-word attachment next to a 1.0 true home. The kill switch is a
	// Go-side branch (gap <= 0 keeps the legacy query untouched) — do NOT
	// inline the gap as a `? <= 0 OR …` SQL parameter: binding a parameter as
	// the left operand of a comparison silently mis-evaluates on pgx.
	if collectCfg.ExclusiveGap > 0 {
		boardTagsQuery = boardTagsQuery.Where(
			"NOT EXISTS (SELECT 1 FROM topic_tag_board_labels stronger WHERE stronger.topic_tag_id = topic_tag_board_labels.topic_tag_id AND stronger.semantic_board_id <> topic_tag_board_labels.semantic_board_id AND stronger.score >= topic_tag_board_labels.score + ?)",
			collectCfg.ExclusiveGap)
	}
	err := boardTagsQuery.
		Group("topic_tags.id, topic_tags.label, topic_tags.category, topic_tags.description, topic_tags.source, topic_tag_board_labels.match_reason, topic_tag_board_labels.score").
		Order("article_count DESC, topic_tags.id ASC").
		Scan(&rows).Error
	if err != nil {
		return nil, nil, nil, fmt.Errorf("query board event tags: %w", err)
	}

	if len(rows) == 0 {
		// No mounted tags means the board channel (main query + live-match
		// fallback) has nothing to work with, but the lane channel reads the
		// full day pool and may still find lane-witnessed tags.
		logging.Infof("[daily-report] board %d has no mounted event tags on %s; lane channel is the only ingress",
			boardID, startOfDay.Format("2006-01-02"))
		laneTags, laneSets, laneRecalled := collectLaneRecalledTags(boardID, nil, startOfDay, endOfDay, collectCfg)
		if len(laneTags) == 0 {
			return nil, nil, nil, nil
		}
		applyAISuspectRatios(laneTags, laneSets)
		return laneTags, laneSets, laneRecalled, nil
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
			// Same cross-board exclusivity as the main query: when the best match
			// for this tag lives on another board and outscores ours by >= the
			// gap, the fallback must not admit it here either.
			if collectCfg.ExclusiveGap > 0 && matches[0].Score-boardMatch.Score >= collectCfg.ExclusiveGap {
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
				ArticleContext: buildArticleContextForTag(t.ID, startOfDay, endOfDay),
			})
			articleIDSets = append(articleIDSets, artIDs)
		}
	}

	// Lane channel (lane-first-recall D1): union in the tags witnessed by this
	// board's active lanes. Same cross-board exclusivity rule as the board
	// channel; a dual-channel hit keeps its board row (task 3.4).
	laneTags, laneArticleSets, laneRecalled := collectLaneRecalledTags(boardID, tags, startOfDay, endOfDay, collectCfg)
	tags = append(tags, laneTags...)
	articleIDSets = append(articleIDSets, laneArticleSets...)

	// 疑似占比（design D9）：只给 filterTagsByQuality 的名额竞争截断当末位排序因子，
	// 不参与「进日报」判定、不改变候选成员。查询失败时占比保持 0（= 不影响排序）。
	applyAISuspectRatios(tags, articleIDSets)

	return tags, articleIDSets, laneRecalled, nil
}

// boardQualityMatchReason reports whether a match reason already proves board
// membership by the quality filter's own standard. Used only to decide whether a
// dual-channel hit owes its seat to the lane channel.
func boardQualityMatchReason(reason string) bool {
	switch reason {
	case "direct_hit", "hit_rate", "max_sim":
		return true
	default:
		return false
	}
}

// laneRecallAdmission splits the lane channel's witnesses into what the
// candidate set actually gains (lane-first-recall task 3.4):
//
//   - newIDs: tags the board channel does not have at all — they enter with
//     match_reason=lane_recall so nothing downstream mistakes them for a board
//     statistical match;
//   - rescued: tags the board channel did have, but at a non-quality tier
//     (weighted/unknown). Their row is kept untouched and the lane evidence is
//     only an additional marker, so the quality filter stops dropping them;
//   - a tag whose board row is already quality-tier is neither moved nor
//     marked: the board row wins on a dual hit.
//
// Tags denied by cross-board exclusivity are dropped. Everything returned in
// `rescued` (plus everything successfully loaded from `newIDs`) forms the
// laneRecalled marker set used by the quality filter and the L3 gate.
func laneRecallAdmission(hits []repository.LaneRecalledTag, existingReason map[uint]string, allowed map[uint]bool) ([]uint, map[uint]bool) {
	rescued := make(map[uint]bool)
	newIDs := make([]uint, 0, len(hits))
	seen := make(map[uint]bool, len(hits))
	for _, hit := range hits {
		if seen[hit.TagID] {
			continue
		}
		seen[hit.TagID] = true
		if reason, onBoard := existingReason[hit.TagID]; onBoard {
			if !boardQualityMatchReason(reason) {
				rescued[hit.TagID] = true
			}
			continue
		}
		if !allowed[hit.TagID] {
			continue
		}
		newIDs = append(newIDs, hit.TagID)
	}
	return newIDs, rescued
}

// collectLaneRecalledTags runs the lane recall channel and returns the tags it
// contributes, their day article sets, and the `laneRecalled` marker set.
//
// Rules implemented here:
//   - tags already recalled by the board channel are not duplicated (the board
//     row wins on MatchReason/Score, task 3.4); only the ones whose board row is
//     a non-quality tier get the lane marker, so a board-proof tag keeps its
//     existing L3 eligibility unchanged;
//   - cross-board exclusivity applies exactly as it does to the board channel
//     (`semantic_board_report_exclusive_gap`): a tag owned clearly stronger by
//     another board is not collected here;
//   - nothing is written: no topic_tag_board_labels row, no topic/relation row.
func collectLaneRecalledTags(boardID uint, existing []repository.TagInput, startOfDay, endOfDay time.Time, collectCfg dailyReportCollectConfig) ([]repository.TagInput, [][]uint, map[uint]bool) {
	recallCfg := repository.LoadLaneRecallConfig(repository.Repo.DB())
	hits, err := repository.Repo.RecallTagsForBoard(boardID, startOfDay, recallCfg)
	if err != nil {
		logging.Warnf("[daily-report] lane recall for board %d failed (board channel only): %v", boardID, err)
		return nil, nil, nil
	}
	if len(hits) == 0 {
		return nil, nil, nil
	}
	ordered := make([]repository.LaneRecalledTag, 0, len(hits))
	for _, hit := range hits {
		ordered = append(ordered, hit)
	}
	// Deterministic order (distance asc, tag id asc) so the same inputs always
	// produce the same candidate order.
	repository.SortLaneRecallHits(ordered)

	existingReason := make(map[uint]string, len(existing))
	for _, tag := range existing {
		existingReason[tag.ID] = tag.MatchReason
	}
	allowed := laneRecallExclusivityAllowed(ordered, existingReason, boardID, collectCfg.ExclusiveGap)

	newIDs, laneRecalled := laneRecallAdmission(ordered, existingReason, allowed)

	var tags []repository.TagInput
	var articleIDSets [][]uint
	for _, tagID := range newIDs {
		var tag models.TopicTag
		if err := repository.Repo.DB().First(&tag, tagID).Error; err != nil {
			logging.Warnf("[daily-report] lane recall: load tag %d failed, skipped: %v", tagID, err)
			continue
		}
		var artIDs []uint
		repository.Repo.DB().Model(&models.ArticleTopicTag{}).
			Select("DISTINCT article_topic_tags.article_id").
			Joins("JOIN articles ON articles.id = article_topic_tags.article_id").
			Where("article_topic_tags.topic_tag_id = ? AND articles.pub_date >= ? AND articles.pub_date < ?", tag.ID, startOfDay, endOfDay).
			Pluck("article_topic_tags.article_id", &artIDs)
		if len(artIDs) == 0 {
			continue
		}
		distance := hits[tag.ID].Distance
		tags = append(tags, repository.TagInput{
			ID:           tag.ID,
			Label:        tag.Label,
			Category:     tag.Category,
			Description:  tag.Description,
			ArticleCount: len(artIDs),
			Source:       tag.Source,
			MatchReason:  laneRecallTagReason,
			// Score carries the lane evidence that admitted the tag: cosine
			// similarity to the witnessing lane's anchor (1 - distance). It is
			// NOT a board score — MatchReason says where it came from.
			Score:          1 - distance,
			ArticleContext: buildArticleContextForTag(tag.ID, startOfDay, endOfDay),
		})
		articleIDSets = append(articleIDSets, artIDs)
		laneRecalled[tagID] = true
	}
	if len(tags) > 0 || len(laneRecalled) > 0 {
		logging.Infof("[daily-report] lane recall board %d: witnesses=%d new_tags=%d dual_channel=%d",
			boardID, len(ordered), len(tags), len(laneRecalled)-len(tags))
	}
	if len(laneRecalled) == 0 {
		return tags, articleIDSets, nil
	}
	return tags, articleIDSets, laneRecalled
}

// laneRecallExclusivityAllowed applies the board channel's cross-board
// exclusivity rule (`semantic_board_report_exclusive_gap`) to lane-recalled
// tags, read-only:
//
//	thisBoardScore = max score of this tag's mount here (0 when unmounted here)
//	bestOtherScore = max score of its mounts on other boards
//	drop when bestOtherScore - thisBoardScore >= gap
//
// i.e. "another board clearly owns this tag" (design D6: 同一 tag 在别板明显
// 更强时不收). gap <= 0 switches the rule off, matching the main query. Only
// topic_tag_board_labels is read — the lane channel never writes mounts.
func laneRecallExclusivityAllowed(hits []repository.LaneRecalledTag, existingReason map[uint]string, boardID uint, gap float64) map[uint]bool {
	allowed := make(map[uint]bool, len(hits))
	if len(hits) == 0 {
		return allowed
	}
	if gap <= 0 {
		for _, hit := range hits {
			allowed[hit.TagID] = true
		}
		return allowed
	}

	pending := make([]uint, 0, len(hits))
	for _, hit := range hits {
		if _, ok := existingReason[hit.TagID]; ok {
			continue // board-channel tags were already judged by the main query
		}
		pending = append(pending, hit.TagID)
	}
	if len(pending) == 0 {
		return allowed
	}

	var rows []laneMountRow
	err := repository.Repo.DB().Raw(
		`SELECT topic_tag_id, semantic_board_id, score FROM topic_tag_board_labels WHERE topic_tag_id IN ?`, pending).
		Scan(&rows).Error
	if err != nil {
		logging.Warnf("[daily-report] lane recall exclusivity lookup failed (keeping recalled tags): %v", err)
		for _, id := range pending {
			allowed[id] = true
		}
		return allowed
	}
	for id, ok := range laneRecallExclusivityVerdict(pending, rows, boardID, gap) {
		allowed[id] = ok
	}
	return allowed
}

// laneMountRow is one topic_tag_board_labels row the exclusivity check reads.
type laneMountRow struct {
	TopicTagID      uint
	SemanticBoardID uint
	Score           float64
}

// laneRecallExclusivityVerdict is the pure arithmetic of the exclusivity rule
// (split out so it is testable without Postgres): per tag, this board's best
// mount score against the best score on any other board.
//
// gap <= 0 means "rule disabled" and is handled HERE as well as in the caller.
// The duplicate is deliberate: the formula `bestOther - ownScore < gap` turns
// into `0 - 0 < 0` = false for a tag mounted nowhere when gap is 0, i.e. the
// exact opposite of "disabled". Callers keep their own early return only so the
// disabled path never opens a DB query; this guard makes the pure function safe
// on its own instead of relying on every caller remembering (review R-13).
func laneRecallExclusivityVerdict(pending []uint, rows []laneMountRow, boardID uint, gap float64) map[uint]bool {
	verdict := make(map[uint]bool, len(pending))
	if gap <= 0 {
		for _, id := range pending {
			verdict[id] = true
		}
		return verdict
	}
	ownScore := make(map[uint]float64, len(pending))
	bestOther := make(map[uint]float64, len(pending))
	for _, row := range rows {
		if row.SemanticBoardID == boardID {
			if row.Score > ownScore[row.TopicTagID] {
				ownScore[row.TopicTagID] = row.Score
			}
			continue
		}
		if _, ok := bestOther[row.TopicTagID]; !ok || row.Score > bestOther[row.TopicTagID] {
			bestOther[row.TopicTagID] = row.Score
		}
	}
	for _, id := range pending {
		verdict[id] = bestOther[id]-ownScore[id] < gap
	}
	return verdict
}

// threadCandidateArticleIDs 把 tag→article 映射展平成候选文章 id 列表。
func threadCandidateArticleIDs(tagArticleMap map[uint][]uint) []uint {
	ids := make([]uint, 0, len(tagArticleMap))
	for _, articleIDs := range tagArticleMap {
		ids = append(ids, articleIDs...)
	}
	return ids
}

// applyAISuspectRatios 给每个候选标签算它名下当天文章的「疑似 AI 生成」比例
// （按比例而非命中篇数：10 篇里 1 篇疑似 = 0.1，2 篇全疑似 = 1.0）。
//
// 判定字段只是软降权因子：查询失败只记 warn 并保持全 0（= 不影响排序），日报生成
// 不因它中断。
func applyAISuspectRatios(tags []repository.TagInput, articleIDSets [][]uint) {
	if len(tags) == 0 || len(articleIDSets) == 0 {
		return
	}
	var candidateIDs []uint
	for _, ids := range articleIDSets {
		candidateIDs = append(candidateIDs, ids...)
	}
	suspects := loadSuspectArticleSet(uniqueUintIDs(candidateIDs))
	if suspects == nil {
		return
	}

	for i := range tags {
		if i >= len(articleIDSets) || len(articleIDSets[i]) == 0 {
			continue
		}
		hits := 0
		for _, id := range articleIDSets[i] {
			if suspects[id] {
				hits++
			}
		}
		tags[i].AISuspectRatio = float64(hits) / float64(len(articleIDSets[i]))
	}
}

// loadSuspectArticleSet 查给定文章集合里判定状态为「疑似 AI 生成」的 id 集合。
// 数据库不可用或查询失败返回 nil——调用方据此保持原行为（不重排 / 占比全 0）：
// 判定字段是软降权因子，无权让日报生成失败。
func loadSuspectArticleSet(articleIDs []uint) map[uint]bool {
	if len(articleIDs) == 0 || repository.Repo == nil || repository.Repo.DB() == nil {
		return nil
	}
	var suspectIDs []uint
	if err := repository.Repo.DB().Model(&models.Article{}).
		Where("id IN ? AND ai_detection_status = ?", articleIDs, models.AIDetectionStatusSuspect).
		Pluck("id", &suspectIDs).Error; err != nil {
		logging.Warnf("[daily-report] load ai detection status failed (keep original order / zero ratios): %v", err)
		return nil
	}
	suspects := make(map[uint]bool, len(suspectIDs))
	for _, id := range suspectIDs {
		suspects[id] = true
	}
	return suspects
}

// uniqueUintIDs 去重合并 id 列表（保持首次出现顺序）。
func uniqueUintIDs(ids []uint) []uint {
	seen := make(map[uint]bool, len(ids))
	unique := make([]uint, 0, len(ids))
	for _, id := range ids {
		if seen[id] {
			continue
		}
		seen[id] = true
		unique = append(unique, id)
	}
	return unique
}

func findPreviousReportBrief(boardID uint, date time.Time) *repository.BoardDailyReport {
	var report repository.BoardDailyReport
	// Order 必须显式按报告日倒序：GORM 的 First() 自带 `ORDER BY <主键> ASC`，
	// 不写 Order 时拿到的是**最早**一份报告而不是紧邻的前一日（review R-1：
	// 补填按旧→新串行就是为了让 prev_report_id 指向上一天，主键序会把它指向
	// 史前第一份）。
	err := repository.Repo.DB().Where("semantic_board_id = ? AND period_date < ? AND status = ?",
		boardID, repository.NormalizeReportDate(date).Format("2006-01-02"), "completed").
		Order("period_date DESC").
		First(&report).Error
	if err != nil {
		return nil
	}
	return &report
}

// buildQualityBreakdownJSON builds the quality_breakdown JSON from tags filtered by tagIDs.
// Each entry carries per-tag match detail: tag_id, label, match_reason, score.
// downgraded 键已于 2026-11-12 随标记退役移除；历史 JSON 中的残留键读取端忽略。
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
	}
	entries := make([]qEntry, 0, len(tagIDs))
	for _, t := range tags {
		if tagIDSet[t.ID] {
			entries = append(entries, qEntry{
				TagID:       t.ID,
				Label:       t.Label,
				MatchReason: t.MatchReason,
				Score:       t.Score,
			})
		}
	}
	b, _ := json.Marshal(entries)
	return b
}

// filterTagsByQuality applies the board-evidence gate to the day's candidate
// tags (lane-first-recall D3).
//
// The `kept >= 10` switch answers "can this tag prove it belongs to this BOARD",
// using statistical evidence (hits / hit rate / overlap share). A single-article
// new event structurally cannot satisfy it, which is why the switch used to drop
// them in bulk and the board fell back to old tags only (2026-11-26).
//
// Tags witnessed by a lane carry a different, stronger kind of evidence — the
// semantic distance to an established lane — so they neither raise `kept` over
// the threshold (which would start dropping the remaining `weighted` tags) nor
// get dropped by it. They also bypass the 30-tag board-evidence cap: that cap
// exists to bound noisy board evidence, and lane witnesses are bounded already
// by `lane_recall_top_k` per lane.
func filterTagsByQuality(tags []repository.TagInput, laneRecalled map[uint]bool) []repository.TagInput {
	// Separate by match reason quality
	var kept []repository.TagInput
	var weighted []repository.TagInput
	var laneKept []repository.TagInput
	for _, t := range tags {
		switch t.MatchReason {
		case "direct_hit", "hit_rate", "max_sim":
			kept = append(kept, t)
		default: // "weighted" or unknown
			if laneRecalled[t.ID] {
				laneKept = append(laneKept, t)
				continue
			}
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
			ti := tagging.MatchTier(kept[i].MatchReason)
			tj := tagging.MatchTier(kept[j].MatchReason)
			if ti != tj {
				return ti < tj
			}
			if kept[i].Score != kept[j].Score {
				return kept[i].Score > kept[j].Score
			}
			// 末位因子（design D9）：同层级同分数时，疑似占比高的标签靠后，
			// 名额不够时优先舍弃。占比按比例算，不按命中篇数。
			return kept[i].AISuspectRatio < kept[j].AISuspectRatio
		})
		kept = kept[:30]
	}

	return append(kept, laneKept...)
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
