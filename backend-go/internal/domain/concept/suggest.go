package concept

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

var callSuggestLLMFn = callSuggestLLM

const suggestMinTags = 10
const suggestMaxTags = 50

type Suggestion struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

func SuggestConcepts(ctx context.Context, category string) ([]Suggestion, error) {
	tags, err := loadUnassignedTags(category)
	if err != nil {
		return nil, fmt.Errorf("suggest concepts: load tags: %w", err)
	}

	if len(tags) < suggestMinTags {
		logging.Infof("suggest: category=%q has %d unassigned tags, need at least %d", category, len(tags), suggestMinTags)
		return []Suggestion{}, nil
	}

	prompt := buildSuggestPrompt(tags)

	content, err := callSuggestLLMFn(ctx, prompt)
	if err != nil {
		logging.Errorf("suggest: LLM call failed: %v", err)
		return []Suggestion{}, nil
	}

	logging.Infof("suggest: raw LLM response length=%d", len(content))

	sanitized := jsonutil.SanitizeLLMJSON(content)
	var raw struct {
		Suggestions []Suggestion `json:"suggestions"`
	}
	if err := json.Unmarshal([]byte(sanitized), &raw); err != nil {
		logging.Errorf("suggest: parse LLM response: %v", err)
		return []Suggestion{}, nil
	}

	for i := range raw.Suggestions {
		raw.Suggestions[i].Name = strings.TrimSpace(raw.Suggestions[i].Name)
		raw.Suggestions[i].Description = strings.TrimSpace(raw.Suggestions[i].Description)
	}

	var filtered []Suggestion
	for _, s := range raw.Suggestions {
		if s.Name != "" {
			filtered = append(filtered, s)
		}
	}

	if len(filtered) == 0 {
		return []Suggestion{}, nil
	}
	if len(filtered) > 5 {
		filtered = filtered[:5]
	}

	return filtered, nil
}

func loadUnassignedTags(category string) ([]models.TopicTag, error) {
	var tags []models.TopicTag
	if err := database.DB.
		Where("category = ? AND status = ? AND concept_id IS NULL", category, "active").
		Order("id DESC").
		Limit(suggestMaxTags).
		Find(&tags).Error; err != nil {
		return nil, fmt.Errorf("load unassigned tags: %w", err)
	}
	return tags, nil
}

func buildSuggestPrompt(tags []models.TopicTag) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "以下是一组未分配板块概念的标签，请分析并建议%d-%d个板块概念：\n\n", 3, 5)

	for _, t := range tags {
		fmt.Fprintf(&sb, "- %s", t.Label)
		if t.Description != "" {
			fmt.Fprintf(&sb, "\n  描述: %s", t.Description)
		}
		sb.WriteByte('\n')
	}

	sb.WriteString("\n请返回一个JSON对象，包含一个suggestions数组，其中每个元素有name（2-6字）和description（30-80字）字段。")
	sb.WriteString("\n建议的概念应该能概括一组语义相近的标签，避免过于宽泛或过于具体。")
	return sb.String()
}

func callSuggestLLM(ctx context.Context, prompt string) (string, error) {
	temperature := 0.4
	maxTokens := 2000
	result, err := airouter.NewRouter().Chat(ctx, airouter.ChatRequest{
		Capability: airouter.CapabilityTopicTagging,
		Messages: []airouter.Message{
			{Role: "system", Content: suggestSystemPrompt},
			{Role: "user", Content: prompt},
		},
		Temperature: &temperature,
		MaxTokens:   &maxTokens,
		JSONMode:    true,
		JSONSchema: &airouter.JSONSchema{
			Type: "object",
			Properties: map[string]airouter.SchemaProperty{
				"suggestions": {
					Type: "array",
					Items: &airouter.SchemaProperty{
						Type: "object",
						Properties: map[string]airouter.SchemaProperty{
							"name":        {Type: "string", Description: "概念名称，2-6字"},
							"description": {Type: "string", Description: "概念描述，30-80字"},
						},
						Required: []string{"name", "description"},
					},
				},
			},
			Required: []string{"suggestions"},
		},
		Metadata: map[string]any{
			"operation": "concept_suggest",
		},
	})
	if err != nil {
		return "", err
	}
	return result.Content, nil
}

const suggestSystemPrompt = `你是一名内容架构师，负责为标签组建议板块概念。

## 核心原则
1. 名称 2-6 个字，描述 30-80 字
2. 名称应概括一组语义相近标签的共同主题
3. 描述应说明该板块涵盖的内容范围
4. 建议 3-5 个概念，按重要性排序
5. 每个概念应覆盖一组不同的标签
6. 避免建议名为"其他"或过于宽泛（如"科技"）的概念`
