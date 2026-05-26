package daily_report

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"syntopica-backend/internal/domain/models"
	"syntopica-backend/internal/platform/airouter"
	"syntopica-backend/internal/platform/database"
	"syntopica-backend/internal/platform/jsonutil"
	"syntopica-backend/internal/platform/logging"
)

const promptVersion = "1.0"

// ---------------------------------------------------------------------------
// LLM Call A: GenerateHighlights
// ---------------------------------------------------------------------------

const highlightsSystemPrompt = `你是一名专业的新闻分析师。你收到了一个看板的事件标签和聚类分组信息。

你的任务是生成 2-3 条当日要闻（highlights），每条要闻应该：
1. 有一个简洁有力的标题（中文，不超过20字）
2. 有一个简短的理由说明（中文，50-100字）
3. 关联到相关的标签ID

输出要求：
1. 顶层 JSON 对象，只包含 highlights 字段
2. highlights 是数组，每个元素包含 title（字符串）、reason（字符串）、tag_ids（整数数组）
3. 只返回合法 JSON，不要 Markdown 代码块或解释文字`

// GenerateHighlights produces 2-3 highlights for the report.
func GenerateHighlights(ctx context.Context, tags []TagInput, clusters []ClusterGroup) ([]Highlight, error) {
	if len(tags) == 0 {
		return nil, nil
	}

	prompt := buildHighlightsPrompt(tags, clusters)

	temperature := 0.3
	maxTokens := 2000
	result, err := airouter.NewRouter().Chat(ctx, airouter.ChatRequest{
		Capability: airouter.CapabilityTopicTagging,
		Messages: []airouter.Message{
			{Role: "system", Content: highlightsSystemPrompt},
			{Role: "user", Content: prompt},
		},
		Temperature: &temperature,
		MaxTokens:   &maxTokens,
		JSONMode:    true,
		JSONSchema: &airouter.JSONSchema{
			Type: "object",
			Properties: map[string]airouter.SchemaProperty{
				"highlights": {
					Type: "array",
					Items: &airouter.SchemaProperty{
						Type: "object",
						Properties: map[string]airouter.SchemaProperty{
							"title":   {Type: "string", Description: "要闻标题，不超过20字"},
							"reason":  {Type: "string", Description: "要闻理由，50-100字"},
							"tag_ids": {Type: "array", Items: &airouter.SchemaProperty{Type: "integer"}},
						},
						Required: []string{"title", "reason", "tag_ids"},
					},
				},
			},
			Required: []string{"highlights"},
		},
		Metadata: map[string]any{
			"operation":     "daily_report_highlights",
			"tag_count":     len(tags),
			"cluster_count": len(clusters),
		},
	})
	if err != nil {
		return nil, fmt.Errorf("highlights AI call failed: %w", err)
	}

	logging.Infof("daily-report: highlights LLM response length=%d", len(result.Content))
	return parseHighlightsResponse(result.Content, tags)
}

func buildHighlightsPrompt(tags []TagInput, clusters []ClusterGroup) string {
	var sb strings.Builder
	sb.WriteString("## 事件标签\n\n")
	for _, t := range tags {
		fmt.Fprintf(&sb, "- [ID:%d] %s (文章数:%d)\n", t.ID, t.Label, t.ArticleCount)
	}
	if len(clusters) > 0 {
		sb.WriteString("\n## 聚类分组\n\n")
		for i, c := range clusters {
			fmt.Fprintf(&sb, "- 组%d: %s (标签IDs: %v)\n", i+1, c.GroupName, c.TagIDs)
		}
	}
	sb.WriteString("\n请生成 2-3 条当日要闻。\n")
	return sb.String()
}

func parseHighlightsResponse(content string, tags []TagInput) ([]Highlight, error) {
	content = jsonutil.SanitizeLLMJSON(content)

	var raw struct {
		Highlights []Highlight `json:"highlights"`
	}
	if err := json.Unmarshal([]byte(content), &raw); err != nil {
		return nil, fmt.Errorf("parse highlights JSON: %w", err)
	}

	validTagIDs := make(map[uint]bool, len(tags))
	for _, t := range tags {
		validTagIDs[t.ID] = true
	}

	var result []Highlight
	for _, h := range raw.Highlights {
		if strings.TrimSpace(h.Title) == "" {
			continue
		}
		var validIDs []uint
		for _, id := range h.TagIDs {
			if validTagIDs[id] {
				validIDs = append(validIDs, id)
			}
		}
		h.TagIDs = validIDs
		result = append(result, h)
	}
	return result, nil
}

// ---------------------------------------------------------------------------
// LLM Call B: GenerateDynamics
// ---------------------------------------------------------------------------

const dynamicsSystemPrompt = `你是一名专业的新闻分析师。你收到了一个看板当日的事件标签信息。

你的任务是生成一段看板动态总结（dynamics），要求：
1. 中文，200-500字
2. 概括当日该看板下的主要动态和发展趋势
3. 客观陈述事实，不添加主观判断
4. 按重要性排序

输出要求：
1. 顶层 JSON 对象，只包含 dynamics 字段
2. dynamics 是字符串
3. 只返回合法 JSON，不要 Markdown 代码块或解释文字`

// GenerateDynamics produces the dynamics text for the report.
func GenerateDynamics(ctx context.Context, tags []TagInput, clusters []ClusterGroup) (string, error) {
	if len(tags) == 0 {
		return "", nil
	}

	prompt := buildDynamicsPrompt(tags, clusters)

	temperature := 0.3
	maxTokens := 2000
	result, err := airouter.NewRouter().Chat(ctx, airouter.ChatRequest{
		Capability: airouter.CapabilityTopicTagging,
		Messages: []airouter.Message{
			{Role: "system", Content: dynamicsSystemPrompt},
			{Role: "user", Content: prompt},
		},
		Temperature: &temperature,
		MaxTokens:   &maxTokens,
		JSONMode:    true,
		JSONSchema: &airouter.JSONSchema{
			Type: "object",
			Properties: map[string]airouter.SchemaProperty{
				"dynamics": {Type: "string", Description: "看板动态总结，200-500字"},
			},
			Required: []string{"dynamics"},
		},
		Metadata: map[string]any{
			"operation":     "daily_report_dynamics",
			"tag_count":     len(tags),
			"cluster_count": len(clusters),
		},
	})
	if err != nil {
		return "", fmt.Errorf("dynamics AI call failed: %w", err)
	}

	logging.Infof("daily-report: dynamics LLM response length=%d", len(result.Content))
	return parseDynamicsResponse(result.Content)
}

func buildDynamicsPrompt(tags []TagInput, clusters []ClusterGroup) string {
	var sb strings.Builder
	sb.WriteString("## 事件标签\n\n")
	for _, t := range tags {
		fmt.Fprintf(&sb, "- [ID:%d] %s (文章数:%d", t.ID, t.Label, t.ArticleCount)
		if t.Description != "" {
			fmt.Fprintf(&sb, ", 描述:%s", t.Description)
		}
		sb.WriteString(")\n")
	}
	if len(clusters) > 0 {
		sb.WriteString("\n## 聚类分组\n\n")
		for i, c := range clusters {
			fmt.Fprintf(&sb, "- 组%d: %s (%d个标签)\n", i+1, c.GroupName, len(c.TagIDs))
		}
	}
	sb.WriteString("\n请生成该看板的当日动态总结。\n")
	return sb.String()
}

func parseDynamicsResponse(content string) (string, error) {
	content = jsonutil.SanitizeLLMJSON(content)

	var raw struct {
		Dynamics string `json:"dynamics"`
	}
	if err := json.Unmarshal([]byte(content), &raw); err != nil {
		return "", fmt.Errorf("parse dynamics JSON: %w", err)
	}
	return raw.Dynamics, nil
}

// ---------------------------------------------------------------------------
// LLM Call C: GenerateClusterThreads (per cluster)
// ---------------------------------------------------------------------------

const threadsSystemPrompt = `你是一名专业的新闻叙事分析师。你收到了一个事件聚类分组及其标签信息。

你的任务是识别该聚类中的叙事线索（threads），每条线索应该：
1. 有一个简洁有力的标题（中文，不超过30字，必须是带判断的短句）
2. 有一段客观的摘要（中文，100-200字）
3. 有一个状态标签：emerging（新出现）、continuing（持续发展）、splitting（分化）、merging（合并）、ending（趋于结束）
4. 关联到相关的标签ID
5. 给出置信度分数（0-1）

输出要求：
1. 顶层 JSON 对象，只包含 threads 字段
2. threads 是数组；没有时返回 {"threads":[]}
3. 每个元素包含 title、summary、status、tag_ids、confidence 字段
4. 只返回合法 JSON，不要 Markdown 代码块或解释文字`

// GenerateClusterThreads produces threads for a single cluster.
func GenerateClusterThreads(ctx context.Context, cluster ClusterGroup, tags []TagInput, prevThreadSummaries []string) ([]Thread, error) {
	clusterTags := filterTagsByIDs(tags, cluster.TagIDs)
	if len(clusterTags) == 0 {
		return nil, nil
	}

	prompt := buildThreadsPrompt(cluster, clusterTags, prevThreadSummaries)

	temperature := 0.3
	maxTokens := 2000
	result, err := airouter.NewRouter().Chat(ctx, airouter.ChatRequest{
		Capability: airouter.CapabilityTopicTagging,
		Messages: []airouter.Message{
			{Role: "system", Content: threadsSystemPrompt},
			{Role: "user", Content: prompt},
		},
		Temperature: &temperature,
		MaxTokens:   &maxTokens,
		JSONMode:    true,
		JSONSchema: &airouter.JSONSchema{
			Type: "object",
			Properties: map[string]airouter.SchemaProperty{
				"threads": {
					Type: "array",
					Items: &airouter.SchemaProperty{
						Type: "object",
						Properties: map[string]airouter.SchemaProperty{
							"title":      {Type: "string", Description: "叙事标题"},
							"summary":    {Type: "string", Description: "叙事摘要，100-200字"},
							"status":     {Type: "string", Description: "emerging/continuing/splitting/merging/ending"},
							"tag_ids":    {Type: "array", Items: &airouter.SchemaProperty{Type: "integer"}},
							"confidence": {Type: "number", Description: "0-1 置信度"},
						},
						Required: []string{"title", "summary", "status", "tag_ids", "confidence"},
					},
				},
			},
			Required: []string{"threads"},
		},
		Metadata: map[string]any{
			"operation":    "daily_report_threads",
			"cluster_name": cluster.GroupName,
			"tag_count":    len(clusterTags),
		},
	})
	if err != nil {
		return nil, fmt.Errorf("threads AI call failed for cluster %s: %w", cluster.GroupName, err)
	}

	logging.Infof("daily-report: threads LLM response for cluster '%s' length=%d", cluster.GroupName, len(result.Content))
	return parseThreadsResponse(result.Content, clusterTags)
}

func buildThreadsPrompt(cluster ClusterGroup, tags []TagInput, prevThreadSummaries []string) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "## 聚类: %s\n\n", cluster.GroupName)
	for _, t := range tags {
		fmt.Fprintf(&sb, "- [ID:%d] %s (文章数:%d", t.ID, t.Label, t.ArticleCount)
		if t.Description != "" {
			fmt.Fprintf(&sb, ", 描述:%s", t.Description)
		}
		sb.WriteString(")\n")
	}
	if len(prevThreadSummaries) > 0 {
		sb.WriteString("\n## 该聚类昨日相关叙事（供延续参考）\n\n")
		for _, s := range prevThreadSummaries {
			fmt.Fprintf(&sb, "- %s\n", s)
		}
	}
	sb.WriteString("\n请识别该聚类中的叙事线索。\n")
	return sb.String()
}

func parseThreadsResponse(content string, tags []TagInput) ([]Thread, error) {
	content = jsonutil.SanitizeLLMJSON(content)

	var raw struct {
		Threads []Thread `json:"threads"`
	}
	if err := json.Unmarshal([]byte(content), &raw); err != nil {
		return nil, fmt.Errorf("parse threads JSON: %w", err)
	}

	validTagIDs := make(map[uint]bool, len(tags))
	for _, t := range tags {
		validTagIDs[t.ID] = true
	}

	validStatuses := map[string]bool{
		"emerging": true, "continuing": true, "splitting": true,
		"merging": true, "ending": true,
	}

	var result []Thread
	for _, th := range raw.Threads {
		if strings.TrimSpace(th.Title) == "" {
			continue
		}
		if !validStatuses[th.Status] {
			th.Status = "emerging"
		}
		var validIDs []uint
		for _, id := range th.TagIDs {
			if validTagIDs[id] {
				validIDs = append(validIDs, id)
			}
		}
		th.TagIDs = validIDs
		result = append(result, th)
	}
	return result, nil
}

// ---------------------------------------------------------------------------
// Orchestrator: GenerateDailyReport
// ---------------------------------------------------------------------------

// GenerateDailyReport is the main pipeline that generates a daily report for a board.
func GenerateDailyReport(ctx context.Context, boardID uint, date time.Time) (*BoardDailyReport, []DailyReportSection, error) {
	startOfDay := normalizeReportDate(date)

	// Step 1: Collect board event tags
	tags, articleCounts, err := collectBoardTags(boardID, date)
	if err != nil {
		return nil, nil, fmt.Errorf("collect board tags: %w", err)
	}
	if len(tags) == 0 {
		return nil, nil, nil
	}

	// Step 2: Deduplicate
	tags = DeduplicateTags(tags, articleCounts)

	// Step 3: Cluster via LLM
	clusters, err := ClusterTags(ctx, tags)
	if err != nil {
		return nil, nil, fmt.Errorf("cluster tags: %w", err)
	}

	// Step 4: Query yesterday's report for continuity
	prevReport := findPreviousReport(boardID, startOfDay)

	// Step 5: Generate in parallel (A + B + C×K)
	type highlightsResult struct {
		data []Highlight
		err  error
	}
	type dynamicsResult struct {
		data string
		err  error
	}
	type threadsResult struct {
		clusterIdx int
		data       []Thread
		err        error
	}

	highlightsCh := make(chan highlightsResult, 1)
	dynamicsCh := make(chan dynamicsResult, 1)
	threadsCh := make(chan threadsResult, len(clusters))

	// Call A: Highlights
	go func() {
		data, err := GenerateHighlights(ctx, tags, clusters)
		highlightsCh <- highlightsResult{data: data, err: err}
	}()

	// Call B: Dynamics
	go func() {
		data, err := GenerateDynamics(ctx, tags, clusters)
		dynamicsCh <- dynamicsResult{data: data, err: err}
	}()

	// Call C×K: Threads per cluster
	for i, cluster := range clusters {
		go func(idx int, c ClusterGroup) {
			var prevSummaries []string
			if prevReport != nil {
				prevSummaries = getPrevThreadSummaries(*prevReport, c)
			}
			data, err := GenerateClusterThreads(ctx, c, tags, prevSummaries)
			threadsCh <- threadsResult{clusterIdx: idx, data: data, err: err}
		}(i, cluster)
	}

	// Collect highlights
	hr := <-highlightsCh
	if hr.err != nil {
		return nil, nil, fmt.Errorf("generate highlights: %w", hr.err)
	}

	// Collect dynamics
	dr := <-dynamicsCh
	if dr.err != nil {
		return nil, nil, fmt.Errorf("generate dynamics: %w", dr.err)
	}

	// Collect threads
	threadsByCluster := make(map[int][]Thread, len(clusters))
	for i := 0; i < len(clusters); i++ {
		tr := <-threadsCh
		if tr.err != nil {
			logging.Warnf("daily-report: threads failed for cluster %d: %v", tr.clusterIdx, tr.err)
			continue
		}
		threadsByCluster[tr.clusterIdx] = tr.data
	}

	// Step 6: Match previous threads for continuity
	for idx, cluster := range clusters {
		threads := threadsByCluster[idx]
		if prevReport != nil {
			matchPreviousThreads(threads, *prevReport, cluster)
		}
	}

	// Step 7: Assemble report
	highlightsJSON, _ := json.Marshal(hr.data)
	dynamics := dr.data

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

	// Summary: use dynamics text (truncated by rune to avoid breaking multi-byte UTF-8)
	summary := dynamics
	if len([]rune(summary)) > 200 {
		summary = string([]rune(summary)[:200])
	}

	clustersJSON, _ := json.Marshal(clusters)

	report := &BoardDailyReport{
		SemanticBoardID:         boardID,
		PeriodDate:              startOfDay,
		Title:                   title,
		Summary:                 summary,
		Highlights:              highlightsJSON,
		Dynamics:                dynamics,
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

	// Build sections
	var sections []DailyReportSection
	for i, cluster := range clusters {
		threads := threadsByCluster[i]
		clusterTags := filterTagsByIDs(tags, cluster.TagIDs)
		clusterArticleCount := 0
		for _, t := range clusterTags {
			clusterArticleCount += t.ArticleCount
		}

		tagIDsJSON, _ := json.Marshal(cluster.TagIDs)
		threadsJSON, _ := json.Marshal(threads)

		sections = append(sections, DailyReportSection{
			ClusterIndex:  i,
			ClusterLabel:  cluster.GroupName,
			ClusterTagIDs: tagIDsJSON,
			Threads:       threadsJSON,
			ArticleCount:  clusterArticleCount,
		})
	}

	return report, sections, nil
}

// ---------------------------------------------------------------------------
// Continuity matching
// ---------------------------------------------------------------------------

// matchPreviousThreads sets PrevThreadID on threads that match yesterday's threads.
// Strategy:
// 1. Tag ID intersection → continuing
// 2. No match → emerging
func matchPreviousThreads(threads []Thread, prevReport BoardDailyReport, cluster ClusterGroup) {
	if len(threads) == 0 {
		return
	}

	// Extract previous threads from sections
	var prevThreadList []Thread
	for _, section := range prevReport.Sections {
		if section.Threads != nil {
			var secThreads []Thread
			if err := json.Unmarshal(section.Threads, &secThreads); err == nil {
				prevThreadList = append(prevThreadList, secThreads...)
			}
		}
	}

	if len(prevThreadList) == 0 {
		return
	}

	for i := range threads {
		th := &threads[i]
		bestMatchIdx := -1
		bestOverlap := 0

		for j, prevTh := range prevThreadList {
			overlap := countTagOverlap(th.TagIDs, prevTh.TagIDs)
			if overlap > bestOverlap {
				bestOverlap = overlap
				bestMatchIdx = j
			}
		}

		if bestMatchIdx >= 0 && bestOverlap > 0 {
			// Mark as continuing — the match was found via tag overlap
			if th.Status == "emerging" {
				th.Status = "continuing"
			}
		}
	}
}

func countTagOverlap(a, b []uint) int {
	set := make(map[uint]bool, len(a))
	for _, id := range a {
		set[id] = true
	}
	count := 0
	for _, id := range b {
		if set[id] {
			count++
		}
	}
	return count
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// collectBoardTags loads event tags for a semantic board on a given date.
func collectBoardTags(boardID uint, date time.Time) ([]TagInput, [][]uint, error) {
	startOfDay := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, date.Location())
	endOfDay := startOfDay.Add(24 * time.Hour)

	type tagRow struct {
		ID           uint   `json:"id"`
		Label        string `json:"label"`
		Category     string `json:"category"`
		Description  string `json:"description"`
		Source       string `json:"source"`
		ArticleCount int    `json:"article_count"`
	}

	var rows []tagRow
	err := database.DB.Model(&models.TopicTag{}).
		Select(`topic_tags.id AS id,
			topic_tags.label AS label,
			topic_tags.category AS category,
			topic_tags.description AS description,
			topic_tags.source AS source,
			COUNT(DISTINCT articles.id) AS article_count`).
		Joins("JOIN topic_tag_board_labels ON topic_tag_board_labels.topic_tag_id = topic_tags.id").
		Joins("JOIN article_topic_tags ON article_topic_tags.topic_tag_id = topic_tags.id").
		Joins("JOIN articles ON articles.id = article_topic_tags.article_id").
		Where("topic_tag_board_labels.semantic_board_id = ?", boardID).
		Where("topic_tags.status = ? AND topic_tags.category = ?", "active", models.TagCategoryEvent).
		Where("articles.pub_date >= ? AND articles.pub_date < ?", startOfDay, endOfDay).
		Group("topic_tags.id, topic_tags.label, topic_tags.category, topic_tags.description, topic_tags.source").
		Order("article_count DESC, topic_tags.id ASC").
		Scan(&rows).Error
	if err != nil {
		return nil, nil, fmt.Errorf("query board event tags: %w", err)
	}

	if len(rows) == 0 {
		return nil, nil, nil
	}

	var tags []TagInput
	var articleIDSets [][]uint
	for _, row := range rows {
		tags = append(tags, TagInput{
			ID:           row.ID,
			Label:        row.Label,
			Category:     row.Category,
			Description:  row.Description,
			ArticleCount: row.ArticleCount,
			Source:       row.Source,
		})

		// Get article IDs for this tag on this date
		var artIDs []uint
		database.DB.Model(&models.ArticleTopicTag{}).
			Select("DISTINCT article_topic_tags.article_id").
			Joins("JOIN articles ON articles.id = article_topic_tags.article_id").
			Where("article_topic_tags.topic_tag_id = ? AND articles.pub_date >= ? AND articles.pub_date < ?",
				row.ID, startOfDay, endOfDay).
			Pluck("article_topic_tags.article_id", &artIDs)
		articleIDSets = append(articleIDSets, artIDs)
	}

	return tags, articleIDSets, nil
}

// findPreviousReport finds the most recent report for the board before the given date.
func findPreviousReport(boardID uint, date time.Time) *BoardDailyReport {
	var report BoardDailyReport
	err := database.DB.Where("semantic_board_id = ? AND period_date < ? AND status = ?",
		boardID, normalizeReportDate(date).Format("2006-01-02"), "completed").
		Order("period_date DESC").
		Preload("Sections").
		First(&report).Error
	if err != nil {
		return nil
	}
	return &report
}

// getPrevThreadSummaries extracts thread summaries from a previous report
// that are relevant to a given cluster.
func getPrevThreadSummaries(prevReport BoardDailyReport, cluster ClusterGroup) []string {
	clusterTagSet := make(map[uint]bool, len(cluster.TagIDs))
	for _, id := range cluster.TagIDs {
		clusterTagSet[id] = true
	}

	var summaries []string
	for _, section := range prevReport.Sections {
		var threads []Thread
		if section.Threads != nil {
			if err := json.Unmarshal(section.Threads, &threads); err != nil {
				continue
			}
		}
		for _, th := range threads {
			for _, tagID := range th.TagIDs {
				if clusterTagSet[tagID] {
					summaries = append(summaries, fmt.Sprintf("%s: %s", th.Title, th.Summary))
					break
				}
			}
		}
	}
	return summaries
}

func filterTagsByIDs(tags []TagInput, ids []uint) []TagInput {
	idSet := make(map[uint]bool, len(ids))
	for _, id := range ids {
		idSet[id] = true
	}
	var result []TagInput
	for _, t := range tags {
		if idSet[t.ID] {
			result = append(result, t)
		}
	}
	return result
}
