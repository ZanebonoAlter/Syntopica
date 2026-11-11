package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"syntopica-backend/internal/platform/airouter"
	"syntopica-backend/internal/platform/jsonutil"
	"syntopica-backend/internal/platform/logging"
	"syntopica-backend/internal/topicgraph/repository"
)

// clusterHighlightsSystemPrompt rewrites the 「今日重点」 selection prompt for
// the same-day event-cluster candidate source (retire-story-adopt-event-metrics
// D13): the model picks at most 3 clusters from the ≤8 whitelisted ones and may
// only quote the candidate facts. Counts / dates / 口径 are server snapshots —
// the model SHALL NOT emit them.
const clusterHighlightsSystemPrompt = `你是一名新闻编辑。本次给出最多8条候选，每条是同一版块当天的一簇同事件标签（cluster_id、代表标签、描述、簇内文章数与标签事实）。从候选中选最多3条不同候选作为今日重点，通常2–3条，只有1条可用时可选1条，事实不足可以输出合法空数组。禁止编造事件、数字、情绪或因果；title与reason只能引用该候选facts里的label/description，不得臆测事件因果，不得输出篇数、来源数、日期或热度（这些由服务端补充）。每项必填cluster_id（候选内的整数）、title（非空事实标题）、reason（非空事实理由）、tag_ids（该候选facts内的标签ID）。只输出 {"highlights":[...]}，不要额外字段。`

// clusterHighlightSchema requires cluster_id instead of the retired story_id.
func clusterHighlightSchema() *airouter.JSONSchema {
	schema := highlightsJSONSchema()
	item := schema.Properties["highlights"].Items
	item.Properties["cluster_id"] = airouter.SchemaProperty{Type: "integer"}
	item.Required = append(item.Required, "cluster_id")
	return schema
}

// clusterHighlightFact is one candidate as the model sees it: identity + the
// whitelisted member facts. ArticleCount is context, never a number to repeat.
type clusterHighlightFact struct {
	ClusterID    uint                      `json:"cluster_id"`
	Label        string                    `json:"label"`
	Description  string                    `json:"description"`
	ArticleCount int                       `json:"article_count"`
	Facts        []repository.EventTagFact `json:"facts"`
}

// GenerateClusterHighlights asks the model to pick at most 3 clusters out of the
// whitelisted candidates. Invalid responses (out-of-whitelist cluster_id,
// out-of-cluster tag_ids, blank title/reason, type errors) are retried once with
// the same arguments; a legal empty array is returned without a retry. Zero
// candidates returns an empty array (the caller uses the tag-fallback mode).
func GenerateClusterHighlights(ctx context.Context, candidates []repository.EventCluster) ([]repository.Highlight, error) {
	if len(candidates) == 0 {
		return []repository.Highlight{}, nil
	}
	facts := make([]clusterHighlightFact, 0, len(candidates))
	tagCount := 0
	for _, candidate := range candidates {
		facts = append(facts, clusterHighlightFact{
			ClusterID:    candidate.ClusterID,
			Label:        candidate.Label,
			Description:  candidate.Description,
			ArticleCount: candidate.ArticleCount,
			Facts:        candidate.Facts,
		})
		tagCount += len(candidate.TagIDs)
	}
	prompt, err := json.Marshal(facts)
	if err != nil {
		return nil, err
	}
	callCtx := withHighlightsCallCounts(ctx, tagCount, len(candidates))
	schema := clusterHighlightSchema()
	var lastErr error
	for attempt := 0; attempt < 2; attempt++ {
		content, err := highlightsChatFn(callCtx, clusterHighlightsSystemPrompt, string(prompt), schema, highlightsOperation)
		if err == nil {
			var highlights []repository.Highlight
			highlights, err = parseClusterHighlightsResponse(content, candidates)
			if err == nil {
				return highlights, nil
			}
		}
		lastErr = err
		logging.Warnf("daily-report: cluster highlights attempt %d/2 failed: %v", attempt+1, err)
	}
	return nil, lastErr
}

// parseClusterHighlightsResponse validates the model's answer against the
// whitelist: cluster_id must be a candidate, tag_ids must belong to that
// cluster's facts, titles/reasons must be non-blank. Duplicate clusters keep the
// first entry; result order follows the server's candidate order (never the
// model's), and the display list is capped at 3.
func parseClusterHighlightsResponse(content string, candidates []repository.EventCluster) ([]repository.Highlight, error) {
	content = jsonutil.SanitizeLLMJSON(content)
	if !json.Valid([]byte(content)) {
		return nil, fmt.Errorf("invalid cluster highlights JSON")
	}
	decoder := json.NewDecoder(strings.NewReader(content))
	decoder.DisallowUnknownFields()
	var top struct {
		Highlights json.RawMessage `json:"highlights"`
	}
	if err := decoder.Decode(&top); err != nil {
		return nil, err
	}
	raw := bytes.TrimSpace(top.Highlights)
	if len(raw) == 0 || raw[0] != '[' {
		return nil, fmt.Errorf("cluster highlights must be an array")
	}
	var entries []struct {
		ClusterID uint   `json:"cluster_id"`
		Title     string `json:"title"`
		Reason    string `json:"reason"`
		TagIDs    []uint `json:"tag_ids"`
	}
	decoder = json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&entries); err != nil {
		return nil, err
	}
	byID := make(map[uint]repository.EventCluster, len(candidates))
	for _, candidate := range candidates {
		byID[candidate.ClusterID] = candidate
	}
	selected := make(map[uint]repository.Highlight)
	for _, entry := range entries {
		cluster, ok := byID[entry.ClusterID]
		if !ok || entry.ClusterID == 0 {
			return nil, fmt.Errorf("cluster id outside candidate whitelist")
		}
		if _, duplicate := selected[entry.ClusterID]; duplicate {
			continue
		}
		title, reason := strings.TrimSpace(entry.Title), strings.TrimSpace(entry.Reason)
		allowed := make(map[uint]bool, len(cluster.TagIDs))
		for _, id := range cluster.TagIDs {
			allowed[id] = true
		}
		tags := make([]uint, 0)
		for _, id := range entry.TagIDs {
			if allowed[id] {
				tags = appendUintSet(tags, []uint{id})
			}
		}
		if title == "" || reason == "" || len(tags) == 0 {
			return nil, fmt.Errorf("cluster highlight has no grounded facts or text")
		}
		selected[entry.ClusterID] = repository.EventHighlight(cluster, title, reason, tags)
	}
	out := make([]repository.Highlight, 0, 3)
	// Model selects and writes; the server's candidate order stays the display
	// order (so tied article counts keep their documented tie-break order).
	for _, candidate := range candidates {
		if highlight, ok := selected[candidate.ClusterID]; ok {
			out = append(out, highlight)
			if len(out) == 3 {
				break
			}
		}
	}
	return out, nil
}

const reportAttributionBudget = 60 * time.Second

// Synchronous on purpose: ctx reaches DB, embedding and classifier; there is
// no timeout-race goroutine that can outlive this report's attribution budget.
var attributeReportOnline = func(ctx context.Context, boardID, reportID uint, date time.Time) error {
	var store storyStore
	if storyStoreForTest != nil {
		store = storyStoreForTest
	} else {
		store = storyStore(repoStoryStore{repository.NewTopicGraphRepository(repository.Repo.DB().WithContext(ctx))})
	}
	_, err := attributeStoriesForReportWithStats(ctx, store, boardID, reportID, date, false, true)
	return err
}

var reportAttributionTimeout = reportAttributionBudget

// storyAttributionEnabled is the stop-write switch reader. A nil repository (or
// a test double) keeps the documented default: attribution still runs, so
// deployments that never touched ai_settings behave exactly as before.
var storyAttributionEnabled = func() bool {
	if repository.Repo == nil {
		return repository.DefaultStoryAttributionEnabled
	}
	return repository.Repo.StoryAttributionEnabled()
}

// boundedReportAttribution is the ONLY online story-attribution entry point.
// With the stop-write switch off it returns immediately: no board_stories /
// story_signals row and no daily_report_threads.story_id is written. The
// implementation below is kept intact so flipping the key back to true restores
// production without a rollout (design D1/D2 — 不删代码不删表).
func boundedReportAttribution(ctx context.Context, report *repository.BoardDailyReport) (err error) {
	if !storyAttributionEnabled() {
		logging.Infof("daily-report: story attribution disabled report=%d (ai_settings.%s=false), nothing written",
			report.ID, repository.StoryAttributionEnabledKey)
		return nil
	}
	started := time.Now()
	budget, cancel := context.WithTimeout(ctx, reportAttributionTimeout)
	defer cancel()
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("story attribution panic: %v", recovered)
		}
		logging.Infof("daily-report: attribution report=%d elapsed=%s timeout=%t error=%v", report.ID, time.Since(started), budget.Err() == context.DeadlineExceeded, err)
	}()
	err = attributeReportOnline(budget, report.SemanticBoardID, report.ID, report.PeriodDate)
	if err == nil {
		err = budget.Err()
	}
	return err
}

type reportHighlightInputsKey struct{}
type reportHighlightInputs struct {
	tags     []repository.TagInput
	clusters []repository.ClusterGroup
}

// finalizeDailyReportHighlights runs the report's tail: store attribution (if
// the stop-write switch allows it), the retired dedup no-op, then the highlights.
//
// The 「今日重点」candidate source is the day's same-event clusters (top 8 by
// article count → tag count → representative tag id). Clustering failure only
// degrades the highlights to the existing tag-fallback mode — it never blocks the
// report body (spec: 降级不阻断日报).
func finalizeDailyReportHighlights(ctx context.Context, report *repository.BoardDailyReport, inputs *reportHighlightInputs) {
	attributionErr := boundedReportAttribution(ctx, report)
	// applyStoryDedupForReport depends on story_id and is a natural no-op while
	// attribution is stopped; it is kept wired so a rollback (switch back on)
	// restores the former behavior without touching the orchestrator.
	_, dedupErr := applyStoryDedupForReport(report.ID, report.PeriodDate)
	if attributionErr != nil || dedupErr != nil {
		logging.Warnf("daily-report: story tail degraded report=%d attribution=%v dedup=%v", report.ID, attributionErr, dedupErr)
	}
	tags, fallbackClusters := inputs.tags, inputs.clusters

	var candidates []repository.EventCluster
	clusterSet, clusterErr := buildDailyReportEventClusters(ctx, report.SemanticBoardID, report.ID, report.PeriodDate)
	if clusterErr != nil {
		logging.Warnf("daily-report: event cluster degraded report=%d: %v", report.ID, clusterErr)
	} else if clusterSet != nil {
		candidates = repository.RankEventClusterCandidates(clusterSet.Clusters, 8)
		if clusterSet.Truncated {
			logging.Warnf("daily-report: event cluster truncated report=%d tags=%d", report.ID, clusterSet.TagCount)
		}
	}

	var highlights []repository.Highlight
	var err error
	if len(candidates) > 0 {
		highlights, err = GenerateClusterHighlights(ctx, candidates)
	} else {
		// 零候选（当天无 event 标签 / 全为综合稿）：走既有标签回退模式，
		// cluster_id 为空、快照字段缺省（不伪造聚簇身份或篇数）。
		logging.Infof("daily-report: no event cluster candidates report=%d, using tag highlights", report.ID)
		highlights, err = GenerateHighlights(ctx, tags, fallbackClusters)
	}
	if err != nil {
		logging.Warnf("daily-report: highlights failed: %v", err)
		highlights = nil
	}
	if updated, err := repository.Repo.FinalizeReportHighlights(report, highlights); err != nil || !updated {
		logging.Warnf("daily-report: final highlights write skipped report=%d updated=%t error=%v", report.ID, updated, err)
	}
}
