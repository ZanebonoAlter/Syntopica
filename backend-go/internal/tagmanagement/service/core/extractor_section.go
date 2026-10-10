package core

import (
	"context"
	"fmt"
	"strings"

	"syntopica-backend/internal/platform/airouter"
	"syntopica-backend/internal/platform/logging"
)

// maxSectionTags caps how many tags one fused section call may yield.
const maxSectionTags = 4

// sectionExtractionOperation is the router metadata["operation"] value used by
// the fused per-section extraction calls.
const sectionExtractionOperation = "tag_extraction_section"

// buildSectionExtractionPrompt is the fused system prompt for aggregate-article
// per-section extraction: one call yields event/person/keyword candidates for a
// single column slice of the summary.
func buildSectionExtractionPrompt() string {
	return `你是一个专业的新闻分析助手，负责从聚合型文章（如科技周刊、排行榜合集）的一个栏目片中提取 event、person、keyword 三类标签。

输入是文章的一个栏目片（含栏目标题），不是完整文章。只基于本片内容提取，不要推测片外内容。

event（事件）：完整描述的新闻事件名词短语，必须具备语义完整性。
- 正确示例："苹果WWDC 2024发布会"、"央行禁止比特币交易"
- 错误示例："3月30"（裸日期）、"禁止交易"（无主体动作）、"门票涨价"（无归属状态）、"北京中关村"（裸地名）

person（人物）：具体的个人姓名。
- 正确示例："Sam Altman"、"李飞飞"
- 错误示例："CEO"（泛称）、"发言人"（角色而非具体人）

keyword（关键词）：专业术语、技术概念、产品名称、组织机构等具有持久辨识度的实体或术语。
- 正确示例："Transformer架构"、"PostgreSQL"、"Kubernetes"
- 错误示例："2026"（时间词）、"公司"（泛称）、"技术"（过于宽泛）

提取规则：
- 每片最多输出 4 个标签，宁缺毋滥
- 一个标签只描述一个主题，禁止把本片内多条无关新闻捏造合并成一个标签
- 拒绝纯年份/日期/时间词、过于宽泛的通用词、本片中未展开讨论的附带提及词
- 标签按优先级从高到低排序

辅助标签要求：
- event/person 标签必须输出 auxiliary_labels，数量 3-5 个，每项包含 label 和 description
- auxiliary_labels 应是具体语义锚点（关键实体、人物、地点、动作、技术名词）
- 不要输出泛词，如"事件"、"情况"、"技术"、"公司"
- keyword 不输出 auxiliary_labels（keyword 用标签自身 label + description 直接进入辅助标签池）

description 要求：
- event：中文1句话解释事件，不超过50字，不重复标签名
- keyword：必填，中文1句话，不超过50字，解释标签指代什么
- person：可留空

输出格式正例：
{"tags":[{"label":"某国央行上调基准利率","category":"event","aliases":["央行加息"],"description":"某国央行上调基准利率的政策事件","auxiliary_labels":[{"label":"央行","description":"货币当局"},{"label":"基准利率","description":"货币政策利率"},{"label":"利率上调","description":"提高利率的举措"}]},{"label":"Kubernetes","category":"keyword","aliases":["K8s"],"description":"开源容器编排系统"}]}`
}

// sectionExtractionSchema is the fused JSON schema for per-section extraction.
// Only label/category are required here; the 3-5 auxiliary_labels constraint is
// enforced by the parser so keyword tags don't fail schema validation.
func sectionExtractionSchema() *airouter.JSONSchema {
	return &airouter.JSONSchema{
		Type: "object",
		Properties: map[string]airouter.SchemaProperty{
			"tags": {
				Type: "array",
				Items: &airouter.SchemaProperty{
					Type: "object",
					Properties: map[string]airouter.SchemaProperty{
						"label":       {Type: "string", Description: "标签名称"},
						"category":    {Type: "string", Description: "event、person 或 keyword"},
						"aliases":     {Type: "array", Items: &airouter.SchemaProperty{Type: "string"}},
						"description": {Type: "string", Description: "标签的简短中文描述；event 必填，keyword 必填，person 可留空"},
						"auxiliary_labels": {
							Type: "array",
							Items: &airouter.SchemaProperty{
								Type: "object",
								Properties: map[string]airouter.SchemaProperty{
									"label":       {Type: "string", Description: "具体语义锚点"},
									"description": {Type: "string", Description: "锚点含义说明"},
								},
								Required: []string{"label", "description"},
							},
							Description: "event/person 必填 3-5 个带 description 的具体语义锚点；keyword 不输出",
						},
					},
					Required: []string{"label", "category"},
				},
			},
		},
		Required: []string{"tags"},
	}
}

// buildSectionUserPrompt renders the user prompt for one section slice. It keeps
// the article-level metadata from buildExtractionUserPrompt and frames the body
// as a single column slice of the article.
func buildSectionUserPrompt(input ExtractionInput, sectionTitle, sectionText string, sectionIndex int) string {
	var b strings.Builder
	fmt.Fprintf(&b, `请从以下聚合型文章的一个栏目片中提取标签：

标题: %s
来源: %s
分类: %s
`, input.Title, input.FeedName, input.CategoryName)
	if input.PubDate != "" {
		fmt.Fprintf(&b, "发布日期: %s\n", input.PubDate)
	}
	fmt.Fprintf(&b, `
栏目片序号: %d
栏目标题: %s

栏目片内容:
%s

请返回JSON对象格式: {"tags": [标签列表]}。`, sectionIndex, sectionTitle, sectionText)
	return b.String()
}

// parseSectionTags parses the fused per-section extraction response. event/person
// tags with invalid auxiliary labels degrade (tag kept, aux dropped, warning
// logged); keyword tags missing a description are skipped with a warning. Only
// JSON-level parse failures return an error (triggering the caller's retry);
// per-tag validation issues never fail the whole section. More than
// maxSectionTags tags are truncated to the first maxSectionTags.
func parseSectionTags(content string) ([]ExtractedTag, error) {
	raw, err := parseRawTagObjects(content)
	if err != nil {
		return nil, err
	}

	result := make([]ExtractedTag, 0, len(raw))
	for _, t := range raw {
		label := strings.TrimSpace(t.Label)
		if label == "" {
			continue
		}
		cat := validateCategory(t.Category)
		description := strings.TrimSpace(t.Description)

		var auxiliaryLabels []AuxiliaryLabel
		switch cat {
		case "event", "person":
			auxiliaryLabels, err = parseAuxiliaryLabels(t.AuxiliaryLabels, cat)
			if err != nil {
				logging.Warnf("section extraction: dropping invalid auxiliary labels for %q (category=%s): %v", label, cat, err)
				auxiliaryLabels = nil
			}
		case "keyword":
			if description == "" {
				logging.Warnf("section extraction: skipping keyword tag %q without description", label)
				continue
			}
		}

		if len(result) >= maxSectionTags {
			continue
		}
		result = append(result, ExtractedTag{
			Label:           label,
			Category:        cat,
			Aliases:         t.Aliases,
			Description:     truncateDescription(description, maxTagDescriptionRunes),
			AuxiliaryLabels: auxiliaryLabels,
		})
	}

	return result, nil
}

// extractTagsFromSection runs exactly one fused LLM extraction call for one
// section slice (retried up to 3 times on call/parse failure, aligned with the
// existing maxRetries). When all retries are exhausted the last error is
// returned; the caller decides whether to skip the section.
func (te *TagExtractor) extractTagsFromSection(ctx context.Context, router tagChatRouter, input ExtractionInput, sectionTitle, sectionText string, sectionIndex int) ([]ExtractedTag, error) {
	userPrompt := buildSectionUserPrompt(input, sectionTitle, sectionText, sectionIndex)

	maxTokens := 2048
	temperature := 0.2
	metadata := map[string]any{
		"operation":     sectionExtractionOperation,
		"section_index": sectionIndex,
		"section_title": sectionTitle,
		"title":         input.Title,
	}
	if input.FeedName != "" {
		metadata["feed_name"] = input.FeedName
	}
	if input.ArticleID != nil {
		metadata["article_id"] = *input.ArticleID
	}
	if input.SummaryID != nil {
		metadata["summary_id"] = *input.SummaryID
	}

	const maxRetries = 3
	var lastErr error
	for attempt := 1; attempt <= maxRetries; attempt++ {
		result, err := router.Chat(ctx, airouter.ChatRequest{
			Operation:  "tagmanagement.extractor_section",
			Capability: airouter.CapabilityTopicTagging,
			Messages: []airouter.Message{
				{Role: "system", Content: buildSectionExtractionPrompt()},
				{Role: "user", Content: userPrompt},
			},
			Temperature: &temperature,
			MaxTokens:   &maxTokens,
			Metadata:    metadata,
			JSONMode:    true,
			JSONSchema:  sectionExtractionSchema(),
		})
		if err != nil {
			lastErr = fmt.Errorf("section extraction failed (attempt %d/%d): %w", attempt, maxRetries, err)
			continue
		}

		tags, err := parseSectionTags(result.Content)
		if err != nil {
			lastErr = fmt.Errorf("parse section extraction result failed (attempt %d/%d): %w", attempt, maxRetries, err)
			continue
		}

		return tags, nil
	}
	return nil, lastErr
}
