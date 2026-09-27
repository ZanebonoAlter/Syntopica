package core

import (
	"context"
	"encoding/json"
	"fmt"
	"go.opentelemetry.io/otel"
	"strings"
	"syntopica-backend/internal/platform/airouter"
	"syntopica-backend/internal/platform/jsonutil"
	"syntopica-backend/internal/platform/logging"
	"syntopica-backend/internal/platform/tracing"
)

// TagExtractor handles extracting and resolving tags from AI summaries
type TagExtractor struct {
	embeddingService *EmbeddingService
	router           tagChatRouter
}

type tagChatRouter interface {
	Chat(context.Context, airouter.ChatRequest) (*airouter.ChatResult, error)
}

// NewTagExtractor creates a new tag extractor
func NewTagExtractor() *TagExtractor {
	return &TagExtractor{
		embeddingService: NewEmbeddingService(),
		router:           airouter.NewRouter(),
	}
}

// ExtractionResult represents the result of tag extraction
type ExtractionResult struct {
	Tags    []TopicTag
	Skipped []string // Tags that were skipped during validation
	Errors  []string
	Source  string // "llm" or "heuristic"
}

const defaultTagExtractionScore = 0.7

// ExtractTags extracts tags from a summary using a single LLM call whose
// response carries the event/person and keyword tag arrays together.
func (te *TagExtractor) ExtractTags(ctx context.Context, input ExtractionInput) (*ExtractionResult, error) {
	ctx, span := otel.Tracer(tracing.ServiceName).Start(ctx, "TagExtractor.ExtractTags")
	defer span.End()

	eventPersonTags, keywordTags, err := te.extractMergedCandidates(ctx, input)
	if err != nil {
		return te.extractWithHeuristic(input, err)
	}

	// 空 keyword / 空 event-person 数组是「宁缺毋滥」提示词下的合法结论：
	// 只记观察信息，不再用 heuristicKeywordCandidates 回填规则词候选
	// （fix-tagging-pollution：旧回填把分类名「新闻」等泛词顶进 llm 来源结果）。
	branchErrors := make([]string, 0, 2)
	if len(eventPersonTags) == 0 {
		branchErrors = append(branchErrors, "event/person extraction: empty event/person array (accepted as-is)")
	}
	if len(keywordTags) == 0 {
		branchErrors = append(branchErrors, "keyword extraction: empty keyword array (accepted as-is)")
	}

	candidates := mergeExtractedTags(eventPersonTags, keywordTags)

	if len(candidates) == 0 {
		// 零候选同样原样返回（Source=llm）：不降级 heuristic——旧降级会让
		// 空结果文章顶上规则词标签，使 tagger 层兜底收窄形同虚设。
		return &ExtractionResult{
			Tags:   []TopicTag{},
			Errors: branchErrors,
			Source: "llm",
		}, nil
	}

	// Resolve each candidate against existing tags
	tags := make([]TopicTag, 0, len(candidates))
	var skipped []string
	errs := branchErrors

	for _, candidate := range candidates {
		tag, skip, err := te.resolveCandidate(ctx, candidate, input)
		if err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", candidate.Label, err))
			continue
		}
		if skip {
			skipped = append(skipped, candidate.Label)
			continue
		}
		tags = append(tags, *tag)
	}

	return &ExtractionResult{
		Tags:    tags,
		Skipped: skipped,
		Errors:  errs,
		Source:  "llm",
	}, nil
}

// extractMergedCandidates runs the single merged extraction call (event/person
// and keyword arrays in one response) with the shared retry budget.
func (te *TagExtractor) extractMergedCandidates(ctx context.Context, input ExtractionInput) ([]ExtractedTag, []ExtractedTag, error) {
	userPrompt := buildExtractionUserPrompt(input)

	maxTokens := 2048
	temperature := 0.2
	metadata := map[string]any{
		"operation": "tag_extraction_merged",
		"title":     input.Title,
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
		result, err := te.router.Chat(ctx, airouter.ChatRequest{
			Operation:  "tagmanagement.extractor_enhanced",
			Capability: airouter.CapabilityTopicTagging,
			Messages: []airouter.Message{
				{Role: "system", Content: buildMergedExtractionPrompt()},
				{Role: "user", Content: userPrompt},
			},
			Temperature: &temperature,
			MaxTokens:   &maxTokens,
			Metadata:    metadata,
			JSONMode:    true,
			JSONSchema:  mergedTagExtractionSchema(),
		})
		if err != nil {
			lastErr = fmt.Errorf("AI extraction failed (attempt %d/%d): %w", attempt, maxRetries, err)
			continue
		}

		eventPersonTags, keywordTags, err := parseMergedExtractionTags(result.Content)
		if err != nil {
			lastErr = fmt.Errorf("parse extraction result failed (attempt %d/%d): %w", attempt, maxRetries, err)
			continue
		}

		return eventPersonTags, keywordTags, nil
	}
	return nil, nil, lastErr
}

func mergeExtractedTags(eventPersonTags, keywordTags []ExtractedTag) []ExtractedTag {
	bySlug := make(map[string]ExtractedTag)
	order := make([]string, 0, len(eventPersonTags)+len(keywordTags))
	add := func(tag ExtractedTag) {
		slug := Slugify(tag.Label)
		if slug == "" {
			return
		}
		if current, ok := bySlug[slug]; ok {
			if categoryPriority(tag.Category) > categoryPriority(current.Category) {
				bySlug[slug] = tag
			}
			return
		}
		bySlug[slug] = tag
		order = append(order, slug)
	}
	for _, tag := range eventPersonTags {
		add(tag)
	}
	keywordCount := 0
	for _, tag := range keywordTags {
		if keywordCount >= 3 {
			break
		}
		if validateCategory(tag.Category) != "keyword" {
			continue
		}
		before := len(bySlug)
		add(tag)
		if len(bySlug) > before {
			keywordCount++
		}
	}

	merged := make([]ExtractedTag, 0, len(order))
	for _, slug := range order {
		if len(merged) >= 5 {
			break
		}
		merged = append(merged, bySlug[slug])
	}
	return merged
}

func categoryPriority(category string) int {
	switch validateCategory(category) {
	case "person":
		return 3
	case "event":
		return 2
	case "keyword":
		return 1
	default:
		return 0
	}
}

// resolveCandidate validates and normalizes a single candidate tag.
// Matching against existing tags is handled by findOrCreateTag downstream,
// so this function only does validation/normalization — no DB queries.
func (te *TagExtractor) resolveCandidate(ctx context.Context, candidate ExtractedTag, input ExtractionInput) (*TopicTag, bool, error) {
	category := validateCategory(candidate.Category)
	slug := Slugify(candidate.Label)
	if slug == "" {
		return nil, true, nil
	}
	return &TopicTag{
		Label:           strings.TrimSpace(candidate.Label),
		Slug:            slug,
		Category:        category,
		Aliases:         candidate.Aliases,
		Score:           defaultTagExtractionScore,
		Description:     strings.TrimSpace(candidate.Description),
		AuxiliaryLabels: candidate.AuxiliaryLabels,
	}, false, nil
}

// extractWithHeuristic falls back to rule-based extraction
func (te *TagExtractor) extractWithHeuristic(input ExtractionInput, originalErr error) (*ExtractionResult, error) {
	tags := ExtractTopics(input)
	result := make([]TopicTag, len(tags))
	for i, t := range tags {
		// Map old 'kind' to new 'category'
		category := "keyword"
		if t.Kind == "entity" {
			// Entities default to keyword category (organizations, products go here)
			// Future: could add heuristics to detect person/event
			category = "keyword"
		}
		result[i] = TopicTag{
			Label:    t.Label,
			Slug:     t.Slug,
			Category: category,
			Score:    t.Score,
			IsNew:    true,
		}
	}
	return &ExtractionResult{
		Tags:   result,
		Source: "heuristic",
		Errors: []string{originalErr.Error()},
	}, nil
}

// Helper functions

// buildMergedExtractionPrompt returns the single system prompt for the merged
// extraction call: event/person rules (auxiliary label requirements) and
// keyword rules (description requirements) share the common rule block. It
// replaces the former two per-branch prompts; tests cap it at 1500 runes.
func buildMergedExtractionPrompt() string {
	return `你是新闻分析助手，一次性从新闻摘要提取 event/person 与 keyword 标签，同时输出两个数组。

【event/person 数组】
event（事件）：语义完整的事件名词短语，如"央行禁止比特币交易"；拒绝裸日期、无主体动作短语、裸地名、泛化活动名。
person（人物）：具体个人姓名，如"Sam Altman"；拒绝泛称（"CEO"）与非姓名（"某公司创始人"）。

辅助标签要求：
- 每个 event/person 标签必须输出 auxiliary_labels，数量 3-5 个
- auxiliary_labels 是对象数组，每项含 label 和 description，如 {"label":"伊朗","description":"中东地区国家"}
- 辅助标签须与事件核心直接相关，是具体语义锚点（实体、人物、地点、动作）；背景提及、移除后事件仍成立的实体不要输出
- description 简短具体，不能为空，不能只重复 label；拒绝"事件"、"技术"、"发展"、"市场"等泛词
- 反例：{"label":"技术","description":"技术"}、"伊朗"（字符串而非对象）
- event description 中文1句话不超50字、不重复标签名；person 可留空

【keyword 数组】
keyword（关键词）：有持久辨识度的专业术语、技术概念、产品、组织机构，如"Transformer架构"、"PostgreSQL"；拒绝时间词（"2026"）、泛称（"公司"）、空泛词（"发展"）。

description 要求：每个 keyword 必须输出 description，中文1句话不超50字、不重复标签名，如 "PostgreSQL" → "开源关系型数据库"。

【共同规则】
- event/person 数组只放 event/person，keyword 数组只放 keyword，不要串放
- 最多返回 3 个 keyword；宁缺毋滥，不需要每篇都凑数量
- keyword 不输出 auxiliary_labels 字段，将用标签自身 label + description 直接进入辅助标签池
- 拒绝日期/时间词、过于宽泛的通用词、未展开讨论的附带提及词
- 标签按优先级从高到低排序，最重要的放前面

【输出格式】
正例：
{"event_person_tags":[{"label":"伊朗袭击以色列","category":"event","description":"伊朗对以色列的军事打击","auxiliary_labels":[{"label":"伊朗","description":"中东国家"},{"label":"以色列","description":"中东国家"},{"label":"导弹袭击","description":"军事打击行动"}]}],"keyword_tags":[{"label":"PostgreSQL","category":"keyword","description":"开源关系型数据库"}]}
反例：
- keyword 标签放进 event/person 数组（串放）
- auxiliary_labels 写成字符串数组（必须是含 label/description 的对象数组）
- {"label":"技术","category":"keyword","description":"技术"}（标签与描述都过于泛化）`
}

func buildExtractionUserPrompt(input ExtractionInput) string {
	var b strings.Builder
	fmt.Fprintf(&b, `请从以下新闻摘要中提取标签：

标题: %s
来源: %s
分类: %s
`, input.Title, input.FeedName, input.CategoryName)
	if input.PubDate != "" {
		fmt.Fprintf(&b, "发布日期: %s\n", input.PubDate)
	}
	fmt.Fprintf(&b, `
摘要内容:
%s

请返回JSON对象格式: {"tags": [标签列表]}。`, input.Summary)
	return b.String()
}

type rawExtractedTag struct {
	Label           string          `json:"label"`
	Category        string          `json:"category"`
	Aliases         []string        `json:"aliases"`
	Description     string          `json:"description"`
	AuxiliaryLabels json.RawMessage `json:"auxiliary_labels"`
}

func parseRawTagObjects(content string) ([]rawExtractedTag, error) {
	content = jsonutil.SanitizeLLMJSON(content)

	var raw []rawExtractedTag

	if err := json.Unmarshal([]byte(content), &raw); err != nil {
		var wrapped struct {
			Tags json.RawMessage `json:"tags"`
		}
		if wrappedErr := json.Unmarshal([]byte(content), &wrapped); wrappedErr != nil {
			return nil, fmt.Errorf("failed to parse tags: %w", err)
		}
		if err := json.Unmarshal(wrapped.Tags, &raw); err != nil {
			return nil, fmt.Errorf("failed to parse tags.tags: %w", err)
		}
	}
	return raw, nil
}

func parseEventPersonTagObjects(raw []rawExtractedTag) ([]ExtractedTag, error) {
	result := make([]ExtractedTag, 0, len(raw))
	for _, t := range raw {
		if strings.TrimSpace(t.Label) == "" {
			continue
		}
		cat := validateCategory(t.Category)
		if cat == "keyword" {
			return nil, fmt.Errorf("event/person branch returned keyword tag %q", t.Label)
		}
		auxiliaryLabels, err := parseAuxiliaryLabels(t.AuxiliaryLabels, cat)
		if err != nil {
			logging.Warnf("event/person extraction: dropping invalid auxiliary labels for %q (category=%s): %v", t.Label, cat, err)
			auxiliaryLabels = nil
		}
		result = append(result, ExtractedTag{
			Label:           strings.TrimSpace(t.Label),
			Category:        cat,
			Aliases:         t.Aliases,
			Description:     truncateDescription(strings.TrimSpace(t.Description), maxTagDescriptionRunes),
			AuxiliaryLabels: auxiliaryLabels,
		})
	}

	return result, nil
}

func parseKeywordTagObjects(raw []rawExtractedTag) ([]ExtractedTag, error) {
	result := make([]ExtractedTag, 0, len(raw))
	for _, t := range raw {
		label := strings.TrimSpace(t.Label)
		if label == "" {
			continue
		}
		cat := validateCategory(t.Category)
		if cat != "keyword" {
			return nil, fmt.Errorf("keyword branch returned %s tag %q", cat, label)
		}
		description := strings.TrimSpace(t.Description)
		if description == "" {
			return nil, fmt.Errorf("keyword tag %q requires description", label)
		}
		result = append(result, ExtractedTag{
			Label:       label,
			Category:    cat,
			Aliases:     t.Aliases,
			Description: truncateDescription(description, maxTagDescriptionRunes),
		})
	}
	return result, nil
}

type rawMergedExtraction struct {
	EventPersonTags []rawExtractedTag `json:"event_person_tags"`
	KeywordTags     []rawExtractedTag `json:"keyword_tags"`
}

// parseMergedExtractionTags splits a single-call response into event/person
// and keyword candidates, reusing each branch's per-item field validation.
// Besides the canonical double-array object it tolerates the response shapes
// the former single-array parsers accepted: the double arrays wrapped in a
// "tags" object, and legacy single arrays (bare or under "tags"), whose
// items are routed by category.
func parseMergedExtractionTags(content string) ([]ExtractedTag, []ExtractedTag, error) {
	merged, err := unmarshalMergedExtraction(jsonutil.SanitizeLLMJSON(content))
	if err != nil {
		return nil, nil, fmt.Errorf("failed to parse merged extraction: %w", err)
	}

	eventPersonTags, err := parseEventPersonTagObjects(merged.EventPersonTags)
	if err != nil {
		return nil, nil, err
	}
	keywordTags, err := parseKeywordTagObjects(merged.KeywordTags)
	if err != nil {
		return nil, nil, err
	}
	return eventPersonTags, keywordTags, nil
}

func unmarshalMergedExtraction(content string) (*rawMergedExtraction, error) {
	// {"tags": ...} wrapper: either the double arrays again or a legacy array.
	var wrapped struct {
		Tags json.RawMessage `json:"tags"`
	}
	if err := json.Unmarshal([]byte(content), &wrapped); err == nil && len(wrapped.Tags) > 0 {
		var merged rawMergedExtraction
		if err := json.Unmarshal(wrapped.Tags, &merged); err == nil {
			return &merged, nil
		}
		raw, err := parseRawTagObjects(content)
		if err != nil {
			return nil, err
		}
		return routeLegacyTagObjects(raw), nil
	}

	// Canonical double-array object. An empty object stays whitelisted (both
	// arrays empty, per the array-empty path contract); any non-empty object
	// missing both target keys is a malformed response and must surface as a
	// parse error so the retry loop kicks in, instead of silently degrading to
	// the array-empty path.
	var probe map[string]json.RawMessage
	if err := json.Unmarshal([]byte(content), &probe); err == nil {
		hasEventPerson, hasKeyword := false, false
		for key := range probe {
			switch {
			case strings.EqualFold(key, "event_person_tags"):
				hasEventPerson = true
			case strings.EqualFold(key, "keyword_tags"):
				hasKeyword = true
			}
		}
		if len(probe) > 0 && !hasEventPerson && !hasKeyword {
			return nil, fmt.Errorf("merged extraction object has neither event_person_tags nor keyword_tags keys")
		}
		var merged rawMergedExtraction
		if err := json.Unmarshal([]byte(content), &merged); err != nil {
			return nil, err
		}
		return &merged, nil
	}

	// Legacy bare array: route items by category.
	raw, err := parseRawTagObjects(content)
	if err != nil {
		return nil, err
	}
	return routeLegacyTagObjects(raw), nil
}

// routeLegacyTagObjects splits legacy single-array responses by category:
// event/person items go to the event/person array, the rest to keywords.
func routeLegacyTagObjects(raw []rawExtractedTag) *rawMergedExtraction {
	var merged rawMergedExtraction
	for _, t := range raw {
		if validateCategory(t.Category) == "keyword" {
			merged.KeywordTags = append(merged.KeywordTags, t)
		} else {
			merged.EventPersonTags = append(merged.EventPersonTags, t)
		}
	}
	return &merged
}

var GenericAuxiliaryLabels = map[string]struct{}{
	"事件": {}, "情况": {}, "问题": {}, "技术": {}, "发展": {}, "行业": {},
	"趋势": {}, "市场": {}, "影响": {}, "创新": {}, "未来": {}, "公司": {},
}

func parseAuxiliaryLabels(raw json.RawMessage, category string) ([]AuxiliaryLabel, error) {
	if len(raw) == 0 || string(raw) == "null" {
		if category == "keyword" {
			return nil, nil
		}
		return nil, fmt.Errorf("event/person tags require 3-5 auxiliary labels")
	}

	var labels []AuxiliaryLabel
	if err := json.Unmarshal(raw, &labels); err != nil {
		var legacy []string
		if legacyErr := json.Unmarshal(raw, &legacy); legacyErr != nil {
			return nil, fmt.Errorf("must be an array of objects with label and description: %w", err)
		}
		labels = make([]AuxiliaryLabel, 0, len(legacy))
		for _, label := range legacy {
			labels = append(labels, AuxiliaryLabel{Label: label, Description: label})
		}
	}
	if len(labels) == 0 && category == "keyword" {
		return nil, nil
	}
	if category == "event" || category == "person" {
		if len(labels) < 3 || len(labels) > 5 {
			return nil, fmt.Errorf("event/person tags require 3-5 auxiliary labels")
		}
	}

	result := make([]AuxiliaryLabel, 0, len(labels))
	seen := make(map[string]struct{}, len(labels))
	for _, item := range labels {
		label := strings.TrimSpace(item.Label)
		description := strings.TrimSpace(item.Description)
		if label == "" {
			return nil, fmt.Errorf("label must not be empty")
		}
		if _, generic := GenericAuxiliaryLabels[label]; generic {
			return nil, fmt.Errorf("label %q is too generic", label)
		}
		if err := validateAuxiliaryLabelDescription(label, description); err != nil {
			return nil, err
		}
		key := strings.ToLower(label)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, AuxiliaryLabel{Label: label, Description: description})
	}
	return result, nil
}

const maxTagDescriptionRunes = 200
const maxAuxiliaryDescriptionRunes = 200

func truncateDescription(s string, maxRunes int) string {
	runes := []rune(s)
	if len(runes) <= maxRunes {
		return s
	}
	return string(runes[:maxRunes])
}

func validateAuxiliaryLabelDescription(label, description string) error {
	description = strings.TrimSpace(description)
	if description == "" {
		return fmt.Errorf("description must not be empty")
	}
	if len([]rune(description)) > maxAuxiliaryDescriptionRunes {
		return fmt.Errorf("description must not exceed 500 characters")
	}
	if strings.EqualFold(strings.TrimSpace(label), description) {
		return fmt.Errorf("description must not only repeat label")
	}
	return nil
}

func validateCategory(cat string) string {
	cat = strings.ToLower(strings.TrimSpace(cat))
	switch cat {
	case "event", "person", "keyword":
		return cat
	default:
		return "keyword"
	}
}

func mergedTagExtractionSchema() *airouter.JSONSchema {
	return &airouter.JSONSchema{
		Type: "object",
		Properties: map[string]airouter.SchemaProperty{
			"event_person_tags": {
				Type: "array",
				Items: &airouter.SchemaProperty{
					Type: "object",
					Properties: map[string]airouter.SchemaProperty{
						"label":       {Type: "string", Description: "标签名称"},
						"category":    {Type: "string", Description: "event 或 person"},
						"aliases":     {Type: "array", Items: &airouter.SchemaProperty{Type: "string"}},
						"description": {Type: "string", Description: "event 标签的简短描述；person 可留空"},
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
							Description: "3-5个带description的具体语义锚点",
						},
					},
					Required: []string{"label", "category", "auxiliary_labels"},
				},
			},
			"keyword_tags": {
				Type: "array",
				Items: &airouter.SchemaProperty{
					Type: "object",
					Properties: map[string]airouter.SchemaProperty{
						"label":       {Type: "string", Description: "keyword 标签名称"},
						"category":    {Type: "string", Description: "必须为 keyword"},
						"aliases":     {Type: "array", Items: &airouter.SchemaProperty{Type: "string"}},
						"description": {Type: "string", Description: "keyword 的简短中文描述，必填"},
					},
					Required: []string{"label", "category", "description"},
				},
			},
		},
		Required: []string{"event_person_tags", "keyword_tags"},
	}
}
