package topicextraction

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"my-robot-backend/internal/domain/topicanalysis"
	"my-robot-backend/internal/domain/topictypes"
	"my-robot-backend/internal/platform/airouter"
	"my-robot-backend/internal/platform/jsonutil"
)

// TagExtractor handles extracting and resolving tags from AI summaries
type TagExtractor struct {
	embeddingService *topicanalysis.EmbeddingService
	router           *airouter.Router
}

// NewTagExtractor creates a new tag extractor
func NewTagExtractor() *TagExtractor {
	return &TagExtractor{
		embeddingService: topicanalysis.NewEmbeddingService(),
		router:           airouter.NewRouter(),
	}
}

// ExtractionResult represents the result of tag extraction
type ExtractionResult struct {
	Tags    []topictypes.TopicTag
	Skipped []string // Tags that were skipped due to low confidence
	Errors  []string
	Source  string // "llm" or "heuristic"
}

// ExtractTags extracts tags from a summary using two-stage process:
// 1. AI extracts candidate tags with categories
// 2. For ambiguous candidates, AI decides whether to reuse or create
func (te *TagExtractor) ExtractTags(ctx context.Context, input topictypes.ExtractionInput) (*ExtractionResult, error) {
	// Step 1: Extract candidate tags
	candidates, err := te.extractCandidates(ctx, input)
	if err != nil {
		// Fall back to heuristic extraction
		return te.extractWithHeuristic(input, err)
	}

	if len(candidates) == 0 {
		return te.extractWithHeuristic(input, errors.New("no candidates extracted"))
	}

	// Step 2: Resolve each candidate against existing tags
	tags := make([]topictypes.TopicTag, 0, len(candidates))
	var skipped []string
	var errs []string

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

// extractCandidates extracts candidate tags from the summary
func (te *TagExtractor) extractCandidates(ctx context.Context, input topictypes.ExtractionInput) ([]topictypes.ExtractedTag, error) {
	systemPrompt := buildExtractionSystemPrompt()
	userPrompt := buildExtractionUserPrompt(input)

	maxTokens := 2048
	temperature := 0.2
	metadata := map[string]any{
		"operation": "tag_extraction",
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
		JSONSchema:  tagExtractionSchema(),
	})
	if err != nil {
		return nil, fmt.Errorf("AI extraction failed: %w", err)
	}

	return parseExtractedTags(result.Content)
}

// resolveCandidate validates and normalizes a single candidate tag.
// Matching against existing tags is handled by findOrCreateTag downstream,
// so this function only does validation/normalization — no DB queries.
func (te *TagExtractor) resolveCandidate(ctx context.Context, candidate topictypes.ExtractedTag, input topictypes.ExtractionInput) (*topictypes.TopicTag, bool, error) {
	category := validateCategory(candidate.Category)
	slug := topictypes.Slugify(candidate.Label)
	if slug == "" {
		return nil, true, nil
	}
	return &topictypes.TopicTag{
		Label:       strings.TrimSpace(candidate.Label),
		Slug:        slug,
		Category:    category,
		Aliases:     candidate.Aliases,
		Score:       candidate.Confidence,
		Description: strings.TrimSpace(candidate.Description),
	}, false, nil
}

// extractWithHeuristic falls back to rule-based extraction
func (te *TagExtractor) extractWithHeuristic(input topictypes.ExtractionInput, originalErr error) (*ExtractionResult, error) {
	tags := ExtractTopics(input)
	result := make([]topictypes.TopicTag, len(tags))
	for i, t := range tags {
		// Map old 'kind' to new 'category'
		category := "keyword"
		if t.Kind == "entity" {
			// Entities default to keyword category (organizations, products go here)
			// Future: could add heuristics to detect person/event
			category = "keyword"
		}
		result[i] = topictypes.TopicTag{
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

func buildExtractionSystemPrompt() string {
	return `你是一个专业的新闻分析助手，负责从新闻摘要中提取有信息量的结构化标签。

标签分为三类：
1. event（事件）：完整描述的新闻事件名词短语，必须具备语义完整性
   - 正确示例："苹果WWDC 2024发布会"、"央行禁止比特币交易"、"某景区门票涨价风波"
   - 错误示例："3月30"（裸日期）、"禁止交易"（无主体动作）、"门票涨价"（无归属状态）、"北京中关村"（裸地名）、"AI体验活动"（泛化活动名）
    
2. person（人物）：具体的个人姓名
   - 正确示例："Sam Altman"、"Elon Musk"
   - 错误示例："CEO"（泛称）、"发言人"（角色而非具体人）

3. keyword（关键词）：专业术语、技术概念、产品名称、组织机构等有辨识度的实体
   - 正确示例："Transformer架构"、"RAG检索增强生成"、"PostgreSQL"、"Kubernetes"、"苹果公司"、"ChatGPT"
   - 错误示例："2026"、"Q3"、"星期二"（时间词）、"公司"（泛称）、"技术"（过于宽泛）、"发展"（无具体含义）

提取规则：
- 宁缺毋滥，只提取真正有信息量的标签，不需要每篇都凑数量
- 必须拒绝以下无意义标签：
  * 纯年份/日期/时间词（如"2026"、"2024年"、"Q3"、"上半年"）
  * 过于宽泛的通用词（如"技术"、"发展"、"创新"、"行业"、"未来"、"趋势"、"市场"、"影响"）
  * 文章中未展开讨论的附带提及词
	- 优先提取专业术语和技术概念，而非泛化描述词
	- event类标签必须是语义完整的名词短语，能独立传达事件内容
	- 拒绝语义片段：不要把裸日期、无主体动作、无归属状态、裸地名、泛化活动名当作event，应归入keyword
	- 无法判断语义完整性时，优先归入keyword类别
	- 最多返回 5 个标签，其中 keyword 类最多 3 个
	- 宁少勿多：如果文章只聚焦一个话题，2-3 个标签就够了
	- keyword 类标签必须是具有持久辨识度的实体或术语，不接受只在一篇文章出现的临时性描述词。如果一个 keyword 只在单篇文章中有意义，不要提取它
	- 标签必须按优先级从高到低排序，最重要的标签放前面
	- 标签应该简洁、准确

每个标签输出格式：
{"label": "标签名称", "category": "event|person|keyword", "confidence": 0.0-1.0, "aliases": ["别名1"], "evidence": "提取依据", "description": "标签的简短描述（中文，1-2句，客观事实，仅event和keyword需要，person可不填）"}

描述要求（仅 event 和 keyword）：
- 中文，1-2句话，客观事实
- 解释标签指代什么，不重复标签名
- 例如 "ChatGPT" → "OpenAI开发的大型语言模型聊天机器人"
- 例如 "苹果WWDC 2024" → "苹果公司于2024年6月举办的全球开发者大会"
- person 标签的 description 可留空，系统会后续单独生成`
}

func buildExtractionUserPrompt(input topictypes.ExtractionInput) string {
	return fmt.Sprintf(`请从以下新闻摘要中提取标签：

标题: %s
来源: %s
分类: %s

摘要内容:
%s

请返回JSON对象格式: {"tags": [标签列表]}。`, input.Title, input.FeedName, input.CategoryName, input.Summary)
}

func parseExtractedTags(content string) ([]topictypes.ExtractedTag, error) {
	content = jsonutil.SanitizeLLMJSON(content)

	var raw []struct {
		Label       string   `json:"label"`
		Category    string   `json:"category"`
		Confidence  float64  `json:"confidence"`
		Aliases     []string `json:"aliases"`
		Evidence    string   `json:"evidence"`
		Description string   `json:"description"`
	}

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

	result := make([]topictypes.ExtractedTag, 0, len(raw))
	for _, t := range raw {
		if strings.TrimSpace(t.Label) == "" {
			continue
		}
		cat := validateCategory(t.Category)
		conf := t.Confidence
		if conf <= 0 {
			conf = 0.7
		}
		result = append(result, topictypes.ExtractedTag{
			Label:       strings.TrimSpace(t.Label),
			Category:    cat,
			Confidence:  conf,
			Aliases:     t.Aliases,
			Evidence:    t.Evidence,
			Description: strings.TrimSpace(t.Description),
		})
	}

	return result, nil
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

func tagExtractionSchema() *airouter.JSONSchema {
	return &airouter.JSONSchema{
		Type: "object",
		Properties: map[string]airouter.SchemaProperty{
			"tags": {
				Type: "array",
				Items: &airouter.SchemaProperty{
					Type: "object",
					Properties: map[string]airouter.SchemaProperty{
						"label":       {Type: "string", Description: "标签名称"},
						"category":    {Type: "string", Description: "event, person 或 keyword"},
						"confidence":  {Type: "number", Description: "置信度 0.0-1.0，仅提取有信息量的标签，宁缺毋滥"},
						"aliases":     {Type: "array", Items: &airouter.SchemaProperty{Type: "string"}},
						"evidence":    {Type: "string", Description: "提取依据"},
						"description": {Type: "string", Description: "标签的简短描述（中文，1-2句，客观事实。仅event和keyword需要，person可留空）"},
					},
					Required: []string{"label", "category"},
				},
			},
		},
		Required: []string{"tags"},
	}
}
