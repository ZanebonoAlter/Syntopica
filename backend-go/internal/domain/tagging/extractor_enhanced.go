package tagging

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"syntopica-backend/internal/platform/airouter"
	"syntopica-backend/internal/platform/jsonutil"
	"strings"
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

// ExtractTags extracts tags from a summary using independent event/person and keyword branches.
func (te *TagExtractor) ExtractTags(ctx context.Context, input ExtractionInput) (*ExtractionResult, error) {
	eventPersonCh := make(chan extractionBranchResult, 1)
	keywordCh := make(chan extractionBranchResult, 1)

	go func() {
		tags, err := te.extractEventPersonCandidates(ctx, input)
		eventPersonCh <- extractionBranchResult{tags: tags, err: err}
	}()
	go func() {
		tags, err := te.extractKeywordCandidates(ctx, input)
		keywordCh <- extractionBranchResult{tags: tags, err: err}
	}()

	eventPersonResult := <-eventPersonCh
	keywordResult := <-keywordCh

	if eventPersonResult.err != nil && keywordResult.err != nil {
		return te.extractWithHeuristic(input, errors.Join(eventPersonResult.err, keywordResult.err))
	}

	branchErrors := make([]string, 0, 2)
	if eventPersonResult.err != nil {
		branchErrors = append(branchErrors, fmt.Sprintf("event/person extraction failed: %v", eventPersonResult.err))
	}
	if keywordResult.err != nil {
		branchErrors = append(branchErrors, fmt.Sprintf("keyword extraction failed: %v", keywordResult.err))
		keywordResult.tags = heuristicKeywordCandidates(input)
	}

	candidates := mergeExtractedTags(eventPersonResult.tags, keywordResult.tags)

	if len(candidates) == 0 {
		return te.extractWithHeuristic(input, errors.New("no candidates extracted"))
	}

	// Step 2: Resolve each candidate against existing tags
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

type extractionBranchResult struct {
	tags []ExtractedTag
	err  error
}

func (te *TagExtractor) extractEventPersonCandidates(ctx context.Context, input ExtractionInput) ([]ExtractedTag, error) {
	return te.extractBranchCandidates(ctx, input, buildEventPersonPrompt(), eventPersonExtractionSchema(), "tag_extraction_event_person", parseEventPersonTags)
}

func (te *TagExtractor) extractKeywordCandidates(ctx context.Context, input ExtractionInput) ([]ExtractedTag, error) {
	return te.extractBranchCandidates(ctx, input, buildKeywordPrompt(), keywordExtractionSchema(), "tag_extraction_keyword", parseKeywordTags)
}

func (te *TagExtractor) extractBranchCandidates(ctx context.Context, input ExtractionInput, systemPrompt string, schema *airouter.JSONSchema, operation string, parse func(string) ([]ExtractedTag, error)) ([]ExtractedTag, error) {
	userPrompt := buildExtractionUserPrompt(input)

	maxTokens := 2048
	temperature := 0.2
	metadata := map[string]any{
		"operation": operation,
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
			Capability: airouter.CapabilityTopicTagging,
			Messages: []airouter.Message{
				{Role: "system", Content: systemPrompt},
				{Role: "user", Content: userPrompt},
			},
			Temperature: &temperature,
			MaxTokens:   &maxTokens,
			Metadata:    metadata,
			JSONMode:    true,
			JSONSchema:  schema,
		})
		if err != nil {
			lastErr = fmt.Errorf("AI extraction failed (attempt %d/%d): %w", attempt, maxRetries, err)
			continue
		}

		tags, err := parse(result.Content)
		if err != nil {
			lastErr = fmt.Errorf("parse extraction result failed (attempt %d/%d): %w", attempt, maxRetries, err)
			continue
		}

		return tags, nil
	}
	return nil, lastErr
}

func heuristicKeywordCandidates(input ExtractionInput) []ExtractedTag {
	topics := ExtractTopics(input)
	tags := make([]ExtractedTag, 0, len(topics))
	for _, topic := range topics {
		tags = append(tags, ExtractedTag{
			Label:    topic.Label,
			Category: "keyword",
			Aliases:  topic.Aliases,
		})
	}
	return tags
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

func buildEventPersonPrompt() string {
	return `你是一个专业的新闻分析助手，只负责从新闻摘要中提取 event 和 person 标签。

event（事件）：完整描述的新闻事件名词短语，必须具备语义完整性。
- 正确示例："苹果WWDC 2024发布会"、"央行禁止比特币交易"、"某景区门票涨价风波"
- 错误示例："3月30"（裸日期）、"禁止交易"（无主体动作）、"门票涨价"（无归属状态）、"北京中关村"（裸地名）、"AI体验活动"（泛化活动名）

person（人物）：具体的个人姓名。
- 正确示例："Sam Altman"、"Elon Musk"、"李飞飞"
- 错误示例："CEO"（泛称）、"发言人"（角色而非具体人）、"某公司创始人"（非姓名）

提取规则：
- 只输出 event 或 person，不要输出 keyword
- 宁缺毋滥，不需要每篇都凑数量
- 拒绝纯年份/日期/时间词、过于宽泛的通用词、文章中未展开讨论的附带提及词
- event 标签必须是能独立传达事件内容的完整名词短语
- 标签必须按优先级从高到低排序，最重要的标签放前面

辅助标签要求：
- 每个 event/person 标签必须输出 auxiliary_labels，数量 3-5 个
- auxiliary_labels 必须是对象数组，每项包含 label 和 description，例如 {"label":"伊朗","description":"中东地区国家"}
- auxiliary_labels 应是具体语义锚点，如关键实体、人物、地点、动作、技术名词
- description 必须简短具体，不能为空，不能只重复 label
- 不要输出明显泛词，如"事件"、"情况"、"问题"、"技术"、"发展"、"行业"、"趋势"、"市场"、"影响"、"创新"、"未来"、"公司"
- 正例：{"label":"伊朗","description":"中东地区国家"}、{"label":"导弹袭击","description":"以导弹进行军事打击的行动"}
- 反例：{"label":"技术","description":"技术"}、{"label":"事件","description":"事件"}、"伊朗"（字符串而非对象）

输出格式正例：
{"tags":[{"label":"伊朗袭击以色列","category":"event","aliases":["伊以冲突"],"description":"伊朗对以色列发动军事打击的新闻事件","auxiliary_labels":[{"label":"伊朗","description":"中东地区国家"},{"label":"以色列","description":"中东国家"},{"label":"导弹袭击","description":"以导弹进行军事打击的行动"}]}]}

输出格式反例：
- {"label":"OpenAI","category":"keyword"}：本分支不输出 keyword
- {"label":"OpenAI发布GPT-5","category":"event","auxiliary_labels":["OpenAI","GPT-5","模型发布"]}：auxiliary_labels 不能是字符串数组

描述要求：
- event description 用中文1句话解释事件，不超过50字，不重复标签名
- person description 可留空，系统会后续单独生成`
}

func buildKeywordPrompt() string {
	return `你是一个专业的新闻分析助手，只负责从新闻摘要中提取 keyword 标签。

keyword（关键词）：专业术语、技术概念、产品名称、组织机构等具有持久辨识度的实体或术语。
- 正确示例："Transformer架构"、"RAG检索增强生成"、"PostgreSQL"、"Kubernetes"、"苹果公司"、"SaaS服务"、"量子计算"
- 错误示例："2026"、"Q3"、"星期二"（时间词）、"公司"（泛称）、"技术"（过于宽泛）、"发展"（无具体含义）

提取规则：
- 只输出 keyword，不要输出 event 或 person
- 最多返回 3 个 keyword，合并后系统总标签数最多 5 个
- keyword 必须具有长期可复用的辨识度，不接受只在单篇文章出现的临时性描述词
- 优先提取专业术语、技术概念、产品、组织机构，而非泛化描述词
- 拒绝纯年份/日期/时间词、过于宽泛的通用词、文章中未展开讨论的附带提及词
- 标签必须按优先级从高到低排序，最重要的标签放前面

description 要求：
- 每个 keyword 必须输出 description
- 中文，1句话，不超过50字
- 解释标签指代什么，不重复标签名
- 例如 "ChatGPT" → "OpenAI开发的大型语言模型聊天机器人"
- 例如 "PostgreSQL" → "开源关系型数据库管理系统"

辅助标签要求：
- keyword 不输出 auxiliary_labels 字段
- keyword 将用标签自身 label + description 直接进入辅助标签池

输出格式正例：
{"tags":[{"label":"PostgreSQL","category":"keyword","aliases":[],"description":"开源关系型数据库管理系统"}]}

输出格式反例：
- {"label":"OpenAI发布GPT-5","category":"event"}：本分支不输出 event
- {"label":"OpenAI","category":"keyword","auxiliary_labels":[{"label":"AI","description":"人工智能"}]}：keyword 不应生成额外辅助标签
- {"label":"技术","category":"keyword","description":"技术"}：标签和描述都过于泛化`
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

func parseEventPersonTags(content string) ([]ExtractedTag, error) {
	raw, err := parseRawTagObjects(content)
	if err != nil {
		return nil, err
	}

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
			return nil, fmt.Errorf("invalid auxiliary labels for %q: %w", t.Label, err)
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

func parseKeywordTags(content string) ([]ExtractedTag, error) {
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

var genericAuxiliaryLabels = map[string]struct{}{
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
		if _, generic := genericAuxiliaryLabels[label]; generic {
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

func eventPersonExtractionSchema() *airouter.JSONSchema {
	return &airouter.JSONSchema{
		Type: "object",
		Properties: map[string]airouter.SchemaProperty{
			"tags": {
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
		},
		Required: []string{"tags"},
	}
}

func keywordExtractionSchema() *airouter.JSONSchema {
	return &airouter.JSONSchema{
		Type: "object",
		Properties: map[string]airouter.SchemaProperty{
			"tags": {
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
		Required: []string{"tags"},
	}
}
