package narrative

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"syntopica-backend/internal/platform/airouter"
	"syntopica-backend/internal/platform/jsonutil"
	"syntopica-backend/internal/platform/logging"
)

type NarrativeOutput struct {
	Title           string  `json:"title"`
	Summary         string  `json:"summary"`
	Status          string  `json:"status"`
	RelatedTagIDs   []uint  `json:"related_tag_ids"`
	ParentIDs       []uint  `json:"parent_ids"`
	ConfidenceScore float64 `json:"confidence_score"`
}

func GenerateNarratives(ctx context.Context, tagInputs []TagInput, prevNarratives []PreviousNarrative) ([]NarrativeOutput, error) {
	if len(tagInputs) == 0 {
		return nil, nil
	}

	prompt := buildNarrativePrompt(tagInputs, prevNarratives)

	temperature := 0.4
	maxTokens := 8000
	result, err := airouter.NewRouter().Chat(ctx, airouter.ChatRequest{
		Capability: airouter.CapabilityTopicTagging,
		Messages: []airouter.Message{
			{Role: "system", Content: narrativeSystemPrompt},
			{Role: "user", Content: prompt},
		},
		Temperature: &temperature,
		MaxTokens:   &maxTokens,
		JSONMode:    true,
		JSONSchema: &airouter.JSONSchema{
			Type: "object",
			Properties: map[string]airouter.SchemaProperty{
				"narratives": {
					Type: "array",
					Items: &airouter.SchemaProperty{
						Type: "object",
						Properties: map[string]airouter.SchemaProperty{
							"title":            {Type: "string", Description: "叙事标题，带判断的短句，不超过30字"},
							"summary":          {Type: "string", Description: "叙事摘要，200-400字"},
							"status":           {Type: "string", Description: "emerging/continuing/splitting/merging/ending"},
							"related_tag_ids":  {Type: "array", Items: &airouter.SchemaProperty{Type: "integer"}},
							"parent_ids":       {Type: "array", Items: &airouter.SchemaProperty{Type: "integer"}},
							"confidence_score": {Type: "number", Description: "0-1 置信度"},
						},
						Required: []string{"title", "summary", "status", "related_tag_ids", "parent_ids"},
					},
				},
			},
			Required: []string{"narratives"},
		},
		Metadata: map[string]any{
			"operation":            "narrative_generation",
			"tag_input_count":      len(tagInputs),
			"prev_narrative_count": len(prevNarratives),
		},
	})
	if err != nil {
		return nil, fmt.Errorf("narrative AI call failed: %w", err)
	}

	logging.Infof("narrative: raw LLM response length=%d, first_500=%s", len(result.Content), truncateStr(result.Content, 500))

	outputs, err := parseNarrativeResponse(result.Content)
	if err != nil {
		logging.Warnf("narrative parse error: %v, raw_response_length=%d, last_300=%s", err, len(result.Content), truncateStr(result.Content, 300))
		return nil, fmt.Errorf("parse narrative response: %w", err)
	}

	outputs = validateNarrativeOutputs(outputs, tagInputs, prevNarratives)

	logging.Infof("generated %d narratives from %d tag inputs", len(outputs), len(tagInputs))
	return outputs, nil
}

const narrativeSystemPrompt = `你是一名专业的新闻叙事分析师。你的任务是基于当天的话题标签数据，识别出正在形成的重要叙事线索（narrative threads）。

每个叙事线索应该：
1. 有一个简洁有力的标题（中文，不超过30字，必须是带判断的短句，不能是纯名词）
2. 有一段客观的摘要描述（中文，200-400字，包含关键事实和发展脉络）
3. 有一个状态标签：emerging（新出现）、continuing（持续发展）、splitting（分化）、merging（合并）、ending（趋于结束）
4. 每条叙事必须横跨至少两个类别（event/person/keyword）
5. 按因果、影响、主题关联分组，不要按语义相似度归类
6. 关联到相关的标签ID
7. 如果是从已有叙事延续而来，标明父叙事ID
8. 给出置信度分数（0-1）
9. 不要为了凑数而强行合并不相关的标签
10. 数量不固定，有几条写几条，没有就返回空数组

输出要求：
1. 顶层必须是 JSON 对象，且只能包含一个字段：narratives
2. narratives 必须是 JSON 数组；没有符合条件的叙事时，返回 {"narratives":[]}
3. narratives 数组中的每个元素都必须包含 title、summary、status、related_tag_ids、parent_ids、confidence_score 字段
4. status 只能是 emerging、continuing、splitting、merging、ending 之一
5. related_tag_ids 和 parent_ids 必须始终输出数组，即使为空也要输出 []
6. 只返回一个合法 JSON 对象，不要输出 Markdown 代码块、解释文字、前后缀，禁止输出第二个 JSON 块`

func buildNarrativePrompt(tags []TagInput, prev []PreviousNarrative) string {
	var sb strings.Builder

	sb.WriteString("## 今日话题标签数据\n\n")
	for _, t := range tags {
		fmt.Fprintf(&sb, "- [ID:%d] %s (分类:%s, 文章数:%d", t.ID, t.Label, t.Category, t.ArticleCount)
		if t.ParentLabel != "" {
			fmt.Fprintf(&sb, ", 归属:%s", t.ParentLabel)
		}
		if t.IsWatched {
			sb.WriteString(", 关注")
		}
		if t.Description != "" {
			fmt.Fprintf(&sb, ", 描述:%s", t.Description)
		}
		sb.WriteString(")\n")
	}

	if len(prev) > 0 {
		sb.WriteString("\n## 昨日叙事线索（供延续/对比参考）\n\n")
		for _, p := range prev {
			fmt.Fprintf(&sb, "- [ID:%d] %s (状态:%s, 第%d代)\n  摘要: %s\n",
				p.ID, p.Title, p.Status, p.Generation, p.Summary)
		}
	}

	sb.WriteString("\n请基于以上数据，识别今日的叙事线索。注意发现标签之间的关联，识别新兴趋势，标注与昨日叙事的延续关系。\n")
	return sb.String()
}

func parseNarrativeResponse(content string) ([]NarrativeOutput, error) {
	content = jsonutil.SanitizeLLMJSON(content)

	var raw struct {
		Narratives []NarrativeOutput `json:"narratives"`
	}
	if err := json.Unmarshal([]byte(content), &raw); err == nil {
		return raw.Narratives, nil
	}

	var direct []NarrativeOutput
	if err := json.Unmarshal([]byte(content), &direct); err != nil {
		return nil, fmt.Errorf("failed to parse narrative JSON: %w", err)
	}
	return direct, nil
}

func truncateStr(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

var validNarrativeStatuses = map[string]bool{
	"emerging": true, "continuing": true, "splitting": true, "merging": true, "ending": true,
}

func validateNarrativeOutputs(outputs []NarrativeOutput, tagInputs []TagInput, prevNarratives []PreviousNarrative) []NarrativeOutput {
	validTagIDs := make(map[uint]bool, len(tagInputs))
	for _, t := range tagInputs {
		validTagIDs[t.ID] = true
	}

	validParentIDs := make(map[uint64]bool, len(prevNarratives))
	for _, p := range prevNarratives {
		validParentIDs[p.ID] = true
	}

	var valid []NarrativeOutput
	for _, out := range outputs {
		if strings.TrimSpace(out.Title) == "" || strings.TrimSpace(out.Summary) == "" {
			logging.Warnf("narrative: skipping output with empty title or summary")
			continue
		}

		if !validNarrativeStatuses[out.Status] {
			logging.Warnf("narrative: fixing invalid status '%s' to 'emerging' for '%s'", out.Status, out.Title)
			out.Status = "emerging"
		}

		if len(prevNarratives) == 0 && len(out.ParentIDs) > 0 {
			logging.Warnf("narrative: clearing parent_ids for '%s' — no previous narratives exist", out.Title)
			out.ParentIDs = nil
		}

		filteredTagIDs := filterValidIDs(out.RelatedTagIDs, validTagIDs, "related_tag_id", out.Title)
		if len(filteredTagIDs) == 0 {
			logging.Warnf("narrative: skipping '%s' — no valid related_tag_ids after filtering", out.Title)
			continue
		}
		out.RelatedTagIDs = filteredTagIDs

		if len(out.ParentIDs) > 0 {
			out.ParentIDs = filterValidParentIDs(out.ParentIDs, validParentIDs, out.Title)
		}

		if out.ParentIDs == nil {
			out.ParentIDs = []uint{}
		}
		if out.RelatedTagIDs == nil {
			out.RelatedTagIDs = []uint{}
		}

		valid = append(valid, out)
	}
	return valid
}

func filterValidIDs(ids []uint, validSet map[uint]bool, label, title string) []uint {
	var filtered []uint
	for _, id := range ids {
		if validSet[id] {
			filtered = append(filtered, id)
		} else {
			logging.Warnf("narrative: dropping invalid %s %d in '%s'", label, id, title)
		}
	}
	return filtered
}

func filterValidParentIDs(ids []uint, validSet map[uint64]bool, title string) []uint {
	var filtered []uint
	for _, id := range ids {
		if validSet[uint64(id)] {
			filtered = append(filtered, id)
		} else {
			logging.Warnf("narrative: dropping invalid parent_id %d in '%s'", id, title)
		}
	}
	return filtered
}
