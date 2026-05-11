package narrative

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"my-robot-backend/internal/domain/models"
	"my-robot-backend/internal/platform/airouter"
	"my-robot-backend/internal/platform/database"
	"my-robot-backend/internal/platform/jsonutil"
	"my-robot-backend/internal/platform/logging"
)

type ConceptSuggestion struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

func SuggestBoardConcepts(ctx context.Context) ([]ConceptSuggestion, error) {
	var abstractTags []models.TopicTag
	database.DB.Where("source = ? AND status = ?", "abstract", "active").
		Order("quality_score DESC, feed_count DESC").
		Limit(50).
		Find(&abstractTags)

	if len(abstractTags) == 0 {
		return nil, nil
	}

	existingConcepts, _ := ListActiveConcepts()

	prompt := buildConceptSuggestionPrompt(abstractTags, existingConcepts)

	temperature := 0.4
	maxTokens := 4000
	result, err := airouter.NewRouter().Chat(ctx, airouter.ChatRequest{
		Capability: airouter.CapabilityTopicTagging,
		Messages: []airouter.Message{
			{Role: "system", Content: conceptSuggestionSystemPrompt},
			{Role: "user", Content: prompt},
		},
		Temperature: &temperature,
		MaxTokens:   &maxTokens,
		JSONMode:    true,
		JSONSchema: &airouter.JSONSchema{
			Type: "object",
			Properties: map[string]airouter.SchemaProperty{
				"concepts": {
					Type: "array",
					Items: &airouter.SchemaProperty{
						Type: "object",
						Properties: map[string]airouter.SchemaProperty{
							"name":        {Type: "string", Description: "板块概念名称，不超过20字"},
							"description": {Type: "string", Description: "板块概念描述，50-200字"},
						},
						Required: []string{"name", "description"},
					},
				},
			},
			Required: []string{"concepts"},
		},
		Metadata: map[string]any{
			"operation": "board_concept_suggestion",
			"tag_count": len(abstractTags),
		},
	})
	if err != nil {
		return nil, fmt.Errorf("concept suggestion AI call failed: %w", err)
	}

	logging.Infof("concept-suggestion: raw LLM response length=%d", len(result.Content))

	content := jsonutil.SanitizeLLMJSON(result.Content)
	var raw struct {
		Concepts []ConceptSuggestion `json:"concepts"`
	}
	if err := json.Unmarshal([]byte(content), &raw); err != nil {
		return nil, fmt.Errorf("parse concept suggestion response: %w", err)
	}

	var valid []ConceptSuggestion
	for _, c := range raw.Concepts {
		if strings.TrimSpace(c.Name) == "" {
			continue
		}
		if strings.TrimSpace(c.Description) == "" {
			c.Description = c.Name
		}
		valid = append(valid, c)
	}

	return valid, nil
}

const conceptSuggestionSystemPrompt = `你是一名内容板块架构师。你的任务是将一组抽象标签归类到少数几个互不重叠的持久板块中。

## 核心原则

1. **互斥优先**：每个板块必须有明确的边界，同一条新闻只能归入一个板块。如果你发现两个板块的描述超过 30% 重叠，合并它们。
2. **粗粒度**：宁可 5 个宽板块，不要 12 个窄板块。每个板块应能容纳 3 个以上的抽象标签。
3. **按场景切分，不按技术栈切分**：
   - 好：「AI 工程实践」（含模型部署、RAG、Agent 框架、向量数据库）
   - 差：「LLM 基础架构」「Agent 框架」「RAG 技术」——这三个高度重叠，应合并
4. **名称 2-6 个字**，描述 1-2 句话（30-80 字），只说覆盖什么，不说怎么做到的。

## 板块划分方法

1. 先通读全部标签，找出 3-5 个最大的自然聚类
2. 对每个聚类，用一句话概括其共同场景
3. 把剩余标签分配到最近的聚类，放不进去的单独成板
4. 检查：任意两个板块之间是否有 >30% 的标签同时属于两者？如果有，合并

## 输出格式

1. 顶层必须是 JSON 对象，且只能包含一个字段：concepts
2. concepts 必须是 JSON 数组；返回 {"concepts":[]} 如果无法建议
3. 每个概念包含 name（2-6 字）和 description（30-80 字）字段
4. 只返回一个合法 JSON 对象，不要输出 Markdown 代码块、解释文字、前后缀，禁止输出第二个 JSON 块`

func buildConceptSuggestionPrompt(tags []models.TopicTag, existing []models.BoardConcept) string {
	var sb strings.Builder

	if len(existing) > 0 {
		sb.WriteString("## 已有板块概念（禁止重复或高度相似）\n\n")
		for _, c := range existing {
			fmt.Fprintf(&sb, "- %s", c.Name)
			if c.Description != "" {
				fmt.Fprintf(&sb, "：%s", c.Description)
			}
			sb.WriteByte('\n')
		}
		sb.WriteString("\n")
	}

	sb.WriteString("## 当前活跃的抽象标签\n\n")
	for _, t := range tags {
		fmt.Fprintf(&sb, "- [TagID:%d] %s", t.ID, t.Label)
		if t.Description != "" {
			fmt.Fprintf(&sb, "\n  描述: %s", t.Description)
		}
		fmt.Fprintf(&sb, "\n  分类: %s, 源数: %d\n", t.Category, t.FeedCount)
	}
	sb.WriteString("\n请按上述原则为未被已有板块覆盖的标签建议新板块。不要建议与已有板块功能重复的概念。每个板块的 description 只需一句话说明覆盖范围。\n")
	return sb.String()
}
