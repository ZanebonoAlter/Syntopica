package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"syntopica-backend/internal/platform/airouter"
	"syntopica-backend/internal/platform/jsonutil"
	"syntopica-backend/internal/platform/logging"
	"syntopica-backend/internal/platform/tracing"
	"syntopica-backend/internal/topicgraph/repository"
)

const promptVersion = "5.0"

// ---------------------------------------------------------------------------
// LLM Call A: GenerateHighlights
// ---------------------------------------------------------------------------

const highlightsSystemPrompt = `你是一名专业的新闻分析师。你收到了一个看板的事件标签和聚类分组信息。

你的任务是生成 2-3 条当日要闻（highlights），每条要闻应该：
1. 有一个简洁有力的标题（中文，不超过20字）
2. 有一个简短的理由说明（中文，50-100字）
3. 关联到相关的标签ID

事实锚约束：title / reason 必须仅基于所列标签（label / description / 代表文章）中的事实。禁止编造未在标签中出现的：具体事件、具体数字（涨跌幅 / 金额 / 连板数 / 百分比 / 跌停涨停）、市场情绪判断（恐慌 / 狂热 / 崩盘 / 抛售）、因果推断（“引发”“导致”“因此”）。若所列标签信息不足以支撑某条要闻，宁可不写，也不要补全。

输出要求：
1. 顶层 JSON 对象，只包含 highlights 字段
2. highlights 是数组，每个元素包含 title（字符串）、reason（字符串）、tag_ids（整数数组）
3. 只返回合法 JSON，不要 Markdown 代码块或解释文字`

// GenerateHighlights produces 2-3 highlights for the report.
func GenerateHighlights(ctx context.Context, tags []repository.TagInput, clusters []repository.ClusterGroup) ([]repository.Highlight, error) {
	ctx, span := tracing.Tracer(tracing.ServiceName).Start(ctx, "workflow.daily_report.highlights")
	defer span.End()

	if len(tags) == 0 {
		return nil, nil
	}

	prompt := buildHighlightsPrompt(tags, clusters)

	temperature := 0.3
	maxTokens := 2000
	result, err := airouter.NewRouter().Chat(ctx, airouter.ChatRequest{
		Operation:  "daily_report.highlights",
		SessionID:  SessionIDFromContext(ctx),
		Capability: airouter.CapabilityDigestPolish,
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
							"reason":  {Type: "string", Description: "要闻理由，50-100字；须基于所列标签事实，禁止编造"},
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
func buildHighlightsPrompt(tags []repository.TagInput, clusters []repository.ClusterGroup) string {
	var sb strings.Builder
	sb.WriteString("## 事件标签\n\n")
	for _, t := range tags {
		fmt.Fprintf(&sb, "- [ID:%d] %s (文章数:%d)\n", t.ID, t.Label, t.ArticleCount)
		if t.ArticleContext != "" {
			fmt.Fprintf(&sb, "  代表文章: %s\n", t.ArticleContext)
		}
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
func parseHighlightsResponse(content string, tags []repository.TagInput) ([]repository.Highlight, error) {
	content = jsonutil.SanitizeLLMJSON(content)

	var raw struct {
		Highlights []repository.Highlight `json:"highlights"`
	}
	if err := json.Unmarshal([]byte(content), &raw); err != nil {
		return nil, fmt.Errorf("parse highlights JSON: %w", err)
	}

	validTagIDs := make(map[uint]bool, len(tags))
	for _, t := range tags {
		validTagIDs[t.ID] = true
	}

	var result []repository.Highlight
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

const threadsSystemPrompt = `你是一名专业的新闻叙事分析师。你收到了一个事件聚类分组及其标签信息。

你的任务是识别该聚类中的叙事线索（threads），并为该板块拟定一个展示标题 section_title，要求：
1. section_title 一句话概括该聚类当日实际内容（中文，不超过30字）；必须基于所列标签事实，遵守与 threads 相同的事实锚约束（禁编造事件/数字/情绪/因果）；不得复述聚类名或既有话题名，应反映当日内容本身；无法概括时可返回空字符串
2. 每条线索有一个简洁有力的标题（中文，不超过30字，必须是带判断的短句）
3. 有一段客观的摘要（中文，100-200字）
4. 关联到相关的标签ID
5. 给出置信度分数（0-1）

事实锚约束：title / summary 必须仅基于所列标签（label / description / 代表文章）中的事实。禁止编造未在标签中出现的：具体事件、具体数字（涨跌幅 / 金额 / 连板数 / 百分比 / 跌停涨停）、市场情绪判断（恐慌 / 狂热 / 崩盘 / 抛售）、因果推断（“引发”“导致”“因此”）。若所列标签信息不足以支撑某条叙事，宁可不写（返回空 threads），也不要补全。

输出要求：
1. 顶层 JSON 对象，只包含 section_title 和 threads 字段
2. section_title 是字符串（可为空串）；threads 是数组，没有时返回 {"section_title":"", "threads":[]}
3. 每个 threads 元素包含 title、summary、tag_ids、confidence 字段
4. 只返回合法 JSON，不要 Markdown 代码块或解释文字`

// GenerateClusterThreads produces threads for a single cluster.
func GenerateClusterThreads(ctx context.Context, cluster repository.ClusterGroup, tags []repository.TagInput) ([]repository.Thread, string, error) {
	ctx, span := tracing.Tracer(tracing.ServiceName).Start(ctx, "workflow.daily_report.cluster_threads")
	defer span.End()

	clusterTags := filterTagsByIDs(tags, cluster.TagIDs)
	if len(clusterTags) == 0 {
		return nil, "", nil
	}

	prompt := buildThreadsPrompt(cluster, clusterTags)

	temperature := 0.3
	maxTokens := 2000
	result, err := airouter.NewRouter().Chat(ctx, airouter.ChatRequest{
		Operation:  "daily_report.threads",
		SessionID:  SessionIDFromContext(ctx),
		Capability: airouter.CapabilityDigestPolish,
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
				"section_title": {Type: "string", Description: "一句话概括该聚类当日实际内容；须基于所列标签事实，禁止编造；不得复述聚类名或既有话题名"},
				"threads": {
					Type: "array",
					Items: &airouter.SchemaProperty{
						Type: "object",
						Properties: map[string]airouter.SchemaProperty{
							"title":      {Type: "string", Description: "叙事标题"},
							"summary":    {Type: "string", Description: "叙事摘要，100-200字；须基于所列标签事实，禁止编造"},
							"tag_ids":    {Type: "array", Items: &airouter.SchemaProperty{Type: "integer"}},
							"confidence": {Type: "number", Description: "0-1 置信度"},
						},
						Required: []string{"title", "summary", "tag_ids", "confidence"},
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
		return nil, "", fmt.Errorf("threads AI call failed for cluster %s: %w", cluster.GroupName, err)
	}

	logging.Infof("daily-report: threads LLM response for cluster '%s' length=%d", cluster.GroupName, len(result.Content))
	return parseThreadsResponse(result.Content, clusterTags)
}

// fallbackThreadConfidence marks synthesized fallback threads as lower-trust
// than LLM-authored ones (LLM threads are typically >= 0.9).
const fallbackThreadConfidence = 0.5

// synthesizeFallbackThreads guarantees every section keeps at least one
// narrative thread. The threads prompt's fact-anchor rule ("宁可不写也不要补全"")
// legitimately yields {"threads":[]} when the listed tags feel too thin to the
// model; without this fallback the section persists as an empty shell under a
// live persistent topic (hit_count keeps growing). The fallback anchors on the
// top-scoring tag's label/description — pure transcription, no fabrication —
// and links ALL cluster tag IDs so article rollups stay complete.
func synthesizeFallbackThreads(cluster repository.ClusterGroup, tags []repository.TagInput) []repository.Thread {
	if len(tags) == 0 {
		return nil
	}
	top := tags[0]
	for _, t := range tags[1:] {
		if t.Score > top.Score {
			top = t
		}
	}
	summary := strings.TrimSpace(top.Description)
	if summary == "" {
		summary = fmt.Sprintf("「%s」相关动态今日暂未提炼出叙事线索，以下为标签原文留痕。", top.Label)
	}
	tagIDs := make([]uint, len(tags))
	for i, t := range tags {
		tagIDs[i] = t.ID
	}
	return []repository.Thread{{
		Title:      top.Label,
		Summary:    summary,
		TagIDs:     tagIDs,
		Confidence: fallbackThreadConfidence,
	}}
}
func buildThreadsPrompt(cluster repository.ClusterGroup, tags []repository.TagInput) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "## 聚类: %s\n\n", cluster.GroupName)
	for _, t := range tags {
		fmt.Fprintf(&sb, "- [ID:%d] %s (文章数:%d", t.ID, t.Label, t.ArticleCount)
		if t.Description != "" {
			fmt.Fprintf(&sb, ", 描述:%s", t.Description)
		}
		if t.ArticleContext != "" {
			fmt.Fprintf(&sb, ", 代表文章: %s", t.ArticleContext)
		}
		sb.WriteString(")\n")
	}
	sb.WriteString("\n请识别该聚类中的叙事线索。\n")
	return sb.String()
}
func parseThreadsResponse(content string, tags []repository.TagInput) ([]repository.Thread, string, error) {
	content = jsonutil.SanitizeLLMJSON(content)

	var raw struct {
		SectionTitle string              `json:"section_title"`
		Threads      []repository.Thread `json:"threads"`
	}
	if err := json.Unmarshal([]byte(content), &raw); err != nil {
		return nil, "", fmt.Errorf("parse threads JSON: %w", err)
	}
	sectionTitle := strings.TrimSpace(raw.SectionTitle)

	validTagIDs := make(map[uint]bool, len(tags))
	for _, t := range tags {
		validTagIDs[t.ID] = true
	}

	var result []repository.Thread
	for _, th := range raw.Threads {
		if strings.TrimSpace(th.Title) == "" {
			continue
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
	return result, sectionTitle, nil
}

type llmMergePair struct {
	i, j     int
	distance float64
}

// llmArbitrateMerges uses LLM to decide whether gray-zone section pairs should be merged.
func llmArbitrateMerges(ctx context.Context, sections []repository.DailyReportSection, pairs []llmMergePair, tagLabelMap map[uint]string) ([]llmMergePair, error) {
	ctx, span := tracing.Tracer(tracing.ServiceName).Start(ctx, "workflow.daily_report.merge_arbitration")
	defer span.End()

	var sb strings.Builder
	sb.WriteString("以下是一些同日生成的叙事分组（section）配对，它们语义相似但不确定是否属于同一叙事框架。\n")
	sb.WriteString("请判断每对是否应该合并为一个 section。合并标准：它们描述的是同一个更大的叙事/故事。\n\n")

	for idx, p := range pairs {
		labelA := sections[p.i].ClusterLabel
		labelB := sections[p.j].ClusterLabel
		var tagIDsA, tagIDsB []uint
		_ = json.Unmarshal(sections[p.i].ClusterTagIDs, &tagIDsA)
		_ = json.Unmarshal(sections[p.j].ClusterTagIDs, &tagIDsB)

		var labelsA, labelsB []string
		for _, id := range tagIDsA {
			if l, ok := tagLabelMap[id]; ok {
				labelsA = append(labelsA, l)
			}
		}
		for _, id := range tagIDsB {
			if l, ok := tagLabelMap[id]; ok {
				labelsB = append(labelsB, l)
			}
		}

		fmt.Fprintf(&sb, "配对 %d:\n", idx)
		fmt.Fprintf(&sb, "  Section A: \"%s\" (标签: %s)\n", labelA, strings.Join(labelsA, ", "))
		fmt.Fprintf(&sb, "  Section B: \"%s\" (标签: %s)\n", labelB, strings.Join(labelsB, ", "))
		fmt.Fprintf(&sb, "  语义距离: %.3f\n\n", p.distance)
	}

	sb.WriteString("请返回 JSON，格式为 {\"merge_pairs\": [配对索引列表]}，只包含应该合并的配对索引（0-based）。\n")

	temperature := 0.1
	maxTokens := 2048
	result, err := airouter.NewRouter().Chat(ctx, airouter.ChatRequest{
		Operation:  "daily_report.merge_arbitration",
		SessionID:  SessionIDFromContext(ctx),
		Capability: airouter.CapabilityDigestPolish,
		Messages: []airouter.Message{
			{Role: "system", Content: "你是一名专业的新闻叙事分析师。你的任务是判断两个叙事分组是否描述的是同一个更大的故事/叙事框架。只返回应该合并的配对索引。"},
			{Role: "user", Content: sb.String()},
		},
		Temperature: &temperature,
		MaxTokens:   &maxTokens,
		JSONMode:    true,
		JSONSchema: &airouter.JSONSchema{
			Type: "object",
			Properties: map[string]airouter.SchemaProperty{
				"merge_pairs": {
					Type:  "array",
					Items: &airouter.SchemaProperty{Type: "integer"},
				},
			},
			Required: []string{"merge_pairs"},
		},
		Metadata: map[string]any{
			"operation":  "daily_report_section_merge_arbitration",
			"pair_count": len(pairs),
		},
	})
	if err != nil {
		return nil, fmt.Errorf("LLM merge arbitration failed: %w", err)
	}

	content := jsonutil.SanitizeLLMJSON(result.Content)
	var response struct {
		MergePairs []int `json:"merge_pairs"`
	}
	if err := json.Unmarshal([]byte(content), &response); err != nil {
		return nil, fmt.Errorf("parse merge arbitration response: %w", err)
	}

	var mergeResult []llmMergePair
	for _, idx := range response.MergePairs {
		if idx >= 0 && idx < len(pairs) {
			mergeResult = append(mergeResult, pairs[idx])
		}
	}
	return mergeResult, nil
}
