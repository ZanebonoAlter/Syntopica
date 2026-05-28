package daily_report

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"syntopica-backend/internal/platform/airouter"
	"syntopica-backend/internal/platform/jsonutil"
	"syntopica-backend/internal/platform/logging"
)

func buildClusterSystemPrompt(tagCount int) string {
	base := `你是一名专业的事件分组分析师。你的任务是将一组事件标签按"同一核心事件"进行分组。

分组规则：
1. 属于同一核心事件的标签归入一组
2. 每组 2-8 个标签；如果某个标签找不到同类，可以单独成组
3. 分组粒度：比"同一主题"更细，比"完全相同"更宽。例如"G7 峰会开幕"和"G7 峰会联合声明"是同一核心事件，但"G7 峰会"和"美联储加息"不是
4. 每组给出一个简洁的中文组名（不超过20字）
5. 必须确保每个输入标签恰好出现在一个组中

输出要求：
1. 顶层 JSON 对象，只包含 groups 字段
2. groups 是数组，每个元素包含 group_name（字符串）和 tag_ids（整数数组）
3. 只返回合法 JSON，不要 Markdown 代码块或解释文字`

	if tagCount > 25 {
		base += "\n6. 标签数量较多，请分成 8-15 组，合并关联性强的小事件"
	} else if tagCount > 15 {
		base += "\n6. 请分成 6-12 组"
	}

	return base
}

// ClusterTags groups deduplicated tags into clusters using LLM.
// Returns cluster groups with group names and member tag IDs.
func ClusterTags(ctx context.Context, tags []TagInput) ([]ClusterGroup, error) {
	if len(tags) == 0 {
		return nil, nil
	}
	// If very few tags, skip LLM and return each as its own group.
	if len(tags) <= 2 {
		groups := make([]ClusterGroup, len(tags))
		for i, t := range tags {
			groups[i] = ClusterGroup{
				GroupName: t.Label,
				TagIDs:    []uint{t.ID},
			}
		}
		return groups, nil
	}

	prompt := buildClusterPrompt(tags)

	temperature := 0.1
	maxTokens := 4000
	result, err := airouter.NewRouter().Chat(ctx, airouter.ChatRequest{
		Capability: airouter.CapabilityTopicTagging,
		Messages: []airouter.Message{
			{Role: "system", Content: buildClusterSystemPrompt(len(tags))},
			{Role: "user", Content: prompt},
		},
		Temperature: &temperature,
		MaxTokens:   &maxTokens,
		JSONMode:    true,
		JSONSchema: &airouter.JSONSchema{
			Type: "object",
			Properties: map[string]airouter.SchemaProperty{
				"groups": {
					Type: "array",
					Items: &airouter.SchemaProperty{
						Type: "object",
						Properties: map[string]airouter.SchemaProperty{
							"group_name": {Type: "string", Description: "组名，不超过20字"},
							"tag_ids":    {Type: "array", Items: &airouter.SchemaProperty{Type: "integer"}},
						},
						Required: []string{"group_name", "tag_ids"},
					},
				},
			},
			Required: []string{"groups"},
		},
		Metadata: map[string]any{
			"operation": "daily_report_clustering",
			"tag_count": len(tags),
		},
	})
	if err != nil {
		return nil, fmt.Errorf("cluster tags AI call failed: %w", err)
	}

	logging.Infof("daily-report: cluster LLM response length=%d", len(result.Content))

	groups, err := parseClusterResponse(result.Content, tags)
	if err != nil {
		return nil, fmt.Errorf("parse cluster response: %w", err)
	}

	logging.Infof("daily-report: clustered %d tags into %d groups", len(tags), len(groups))
	return groups, nil
}

func buildClusterPrompt(tags []TagInput) string {
	var sb strings.Builder
	sb.WriteString("## 待分组的事件标签\n\n")
	for _, t := range tags {
		fmt.Fprintf(&sb, "- [ID:%d] %s (文章数:%d", t.ID, t.Label, t.ArticleCount)
		if t.Description != "" {
			fmt.Fprintf(&sb, ", 描述:%s", t.Description)
		}
		sb.WriteString(")\n")
	}
	sb.WriteString("\n请将以上标签按核心事件分组。\n")
	return sb.String()
}

func parseClusterResponse(content string, tags []TagInput) ([]ClusterGroup, error) {
	content = jsonutil.SanitizeLLMJSON(content)

	var raw struct {
		Groups []ClusterGroup `json:"groups"`
	}
	if err := json.Unmarshal([]byte(content), &raw); err != nil {
		return nil, fmt.Errorf("failed to parse cluster JSON: %w", err)
	}

	// Validate: ensure all tags are accounted for, no duplicates, no unknown IDs.
	validTagIDs := make(map[uint]bool, len(tags))
	for _, t := range tags {
		validTagIDs[t.ID] = true
	}

	assigned := make(map[uint]bool)
	var result []ClusterGroup
	for _, g := range raw.Groups {
		if strings.TrimSpace(g.GroupName) == "" {
			continue
		}
		var validIDs []uint
		for _, id := range g.TagIDs {
			if validTagIDs[id] && !assigned[id] {
				validIDs = append(validIDs, id)
				assigned[id] = true
			}
		}
		if len(validIDs) == 0 {
			continue
		}
		result = append(result, ClusterGroup{
			GroupName: g.GroupName,
			TagIDs:    validIDs,
		})
	}

	// Assign any unassigned tags to their own group.
	for _, t := range tags {
		if !assigned[t.ID] {
			result = append(result, ClusterGroup{
				GroupName: t.Label,
				TagIDs:    []uint{t.ID},
			})
			assigned[t.ID] = true
		}
	}

	return result, nil
}
