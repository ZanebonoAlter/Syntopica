package tagging

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"syntopica-backend/internal/platform/airouter"

	"github.com/stretchr/testify/require"
)

func TestExtractTopicsFindsCanonicalAITopicsAndEntities(t *testing.T) {
	result := ExtractTopics(ExtractionInput{
		Title:        "OpenAI pushes GPT-5 agent workflow",
		Summary:      "OpenAI is shipping a new AI agent workflow around GPT-5 with multimodal planning and coding automation.",
		FeedName:     "Latent Space",
		CategoryName: "AI",
	})

	require.GreaterOrEqual(t, len(result), 3)
	require.Contains(t, topicLabels(result), "OpenAI")
	require.Contains(t, topicLabels(result), "AI Agent")
	require.Contains(t, topicLabels(result), "GPT-5")
	require.Contains(t, topicSlugs(result), "openai")
	require.Contains(t, topicSlugs(result), "ai-agent")
	require.Contains(t, topicSlugs(result), "gpt-5")
}

func TestExtractTopicsDeduplicatesAliases(t *testing.T) {
	result := ExtractTopics(ExtractionInput{
		Title:        "OpenAI API update",
		Summary:      "OpenAI says the Open AI API now supports agent memory. OPENAI tooling remains the focus.",
		FeedName:     "OpenAI Blog",
		CategoryName: "AI",
	})

	labels := topicLabels(result)
	require.Equal(t, 1, countMatches(labels, "OpenAI"))
	openAI := findTopic(result, "OpenAI")
	require.NotNil(t, openAI)
	require.Greater(t, openAI.Score, 0.0)
}

func TestExtractTopicsFallsBackToFeedAndCategoryWhenTextIsSparse(t *testing.T) {
	result := ExtractTopics(ExtractionInput{
		Title:        "Daily Brief",
		Summary:      "Short update.",
		FeedName:     "NVIDIA Research",
		CategoryName: "Infra",
	})

	require.Contains(t, topicLabels(result), "NVIDIA")
	require.Contains(t, topicLabels(result), "Infra")
}

func TestParseExtractedTagsAcceptsWrappedTagsObject(t *testing.T) {
	parsed, err := parseKeywordTags(`{"tags":[{"label":"OpenAI","category":"keyword","aliases":["Open AI"],"description":"人工智能研究公司"}]}`)

	require.NoError(t, err)
	require.Len(t, parsed, 1)
	require.Equal(t, "OpenAI", parsed[0].Label)
	require.Equal(t, "keyword", parsed[0].Category)
	require.Equal(t, []string{"Open AI"}, parsed[0].Aliases)
	require.Empty(t, parsed[0].AuxiliaryLabels)
}

func TestParseExtractedTagsAcceptsAuxiliaryLabelObjects(t *testing.T) {
	parsed, err := parseEventPersonTags(`{"tags":[{"label":"伊朗袭击以色列","category":"event","auxiliary_labels":[{"label":"伊朗","description":"中东地区国家"},{"label":"以色列","description":"中东地区国家"},{"label":"导弹袭击","description":"军事打击行动"}]}]}`)

	require.NoError(t, err)
	require.Len(t, parsed, 1)
	require.Equal(t, []AuxiliaryLabel{
		{Label: "伊朗", Description: "中东地区国家"},
		{Label: "以色列", Description: "中东地区国家"},
		{Label: "导弹袭击", Description: "军事打击行动"},
	}, parsed[0].AuxiliaryLabels)
}

func TestParseKeywordTagsRequiresDescriptionAndIgnoresAuxiliaryLabels(t *testing.T) {
	parsed, err := parseKeywordTags(`{"tags":[{"label":"PostgreSQL","category":"keyword","description":"开源关系型数据库管理系统"},{"label":"Claude Code","category":"keyword","description":"Anthropic推出的AI编程助手","auxiliary_labels":[]}]}`)

	require.NoError(t, err)
	require.Len(t, parsed, 2)
	require.Empty(t, parsed[0].AuxiliaryLabels)
	require.Empty(t, parsed[1].AuxiliaryLabels)

	_, err = parseKeywordTags(`{"tags":[{"label":"Claude Code","category":"keyword"}]}`)
	require.Error(t, err)
}

func TestParseExtractedTagsRejectsInvalidAuxiliaryLabels(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{
			name:  "event missing labels",
			input: `{"tags":[{"label":"OpenAI发布GPT-5","category":"event"}]}`,
		},
		{
			name:  "too few objects",
			input: `{"tags":[{"label":"OpenAI发布GPT-5","category":"event","auxiliary_labels":[{"label":"OpenAI","description":"人工智能公司"},{"label":"GPT-5","description":"大语言模型"}]}]}`,
		},
		{
			name:  "too many objects",
			input: `{"tags":[{"label":"OpenAI发布GPT-5","category":"event","auxiliary_labels":[{"label":"OpenAI","description":"人工智能公司"},{"label":"GPT-5","description":"大语言模型"},{"label":"模型发布","description":"产品发布行为"},{"label":"Sam Altman","description":"OpenAI负责人"},{"label":"AI助手","description":"人工智能辅助工具"},{"label":"API","description":"应用程序接口"}]}]}`,
		},
		{
			name:  "missing description",
			input: `{"tags":[{"label":"OpenAI发布GPT-5","category":"event","auxiliary_labels":[{"label":"OpenAI","description":"人工智能公司"},{"label":"GPT-5"},{"label":"模型发布","description":"产品发布行为"}]}]}`,
		},
		{
			name:  "empty description",
			input: `{"tags":[{"label":"OpenAI发布GPT-5","category":"event","auxiliary_labels":[{"label":"OpenAI","description":"人工智能公司"},{"label":"GPT-5","description":""},{"label":"模型发布","description":"产品发布行为"}]}]}`,
		},
		{
			name:  "description equals label",
			input: `{"tags":[{"label":"OpenAI发布GPT-5","category":"event","auxiliary_labels":[{"label":"OpenAI","description":"人工智能公司"},{"label":"GPT-5","description":"GPT-5"},{"label":"模型发布","description":"产品发布行为"}]}]}`,
		},
		{
			name:  "generic label",
			input: `{"tags":[{"label":"OpenAI发布GPT-5","category":"event","auxiliary_labels":[{"label":"OpenAI","description":"人工智能公司"},{"label":"技术","description":"宽泛技术词"},{"label":"模型发布","description":"产品发布行为"}]}]}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseEventPersonTags(tt.input)
			require.Error(t, err)
		})
	}
}

func TestParseExtractedTagsRejectsOldAuxiliaryLabelStringArrayViaDescriptionValidation(t *testing.T) {
	_, err := parseEventPersonTags(`{"tags":[{"label":"OpenAI发布GPT-5","category":"event","auxiliary_labels":["OpenAI","GPT-5","模型发布"]}]}`)

	require.Error(t, err)
	require.Contains(t, err.Error(), "description must not only repeat label")
}

func TestParseExtractedTagsAcceptsSurroundingText(t *testing.T) {
	parsed, err := parseKeywordTags("以下是提取结果：\n```json\n[{\"label\":\"AI Agent\",\"category\":\"keyword\",\"description\":\"能自主执行任务的人工智能系统\"}]\n```\n请使用这些标签。")

	require.NoError(t, err)
	require.Len(t, parsed, 1)
	require.Equal(t, "AI Agent", parsed[0].Label)
	require.Equal(t, "keyword", parsed[0].Category)
}

func topicLabels(items []TopicTag) []string {
	labels := make([]string, 0, len(items))
	for _, item := range items {
		labels = append(labels, item.Label)
	}
	return labels
}

func topicSlugs(items []TopicTag) []string {
	slugs := make([]string, 0, len(items))
	for _, item := range items {
		slugs = append(slugs, item.Slug)
	}
	return slugs
}

func countMatches(items []string, needle string) int {
	count := 0
	for _, item := range items {
		if item == needle {
			count++
		}
	}
	return count
}

func findTopic(items []TopicTag, label string) *TopicTag {
	for _, item := range items {
		if item.Label == label {
			return &item
		}
	}
	return nil
}

func TestParseExtractedTagsFromRealOllamaResponse(t *testing.T) {
	input := "```json\n[\n  {\n    \"label\": \"李飞飞\",\n    \"category\": \"person\",\n    \"aliases\": [\"AI 教母\"],\n    \"auxiliary_labels\": [{\"label\": \"李飞飞\", \"description\": \"人工智能领域学者\"}, {\"label\": \"World Labs\", \"description\": \"空间智能创业公司\"}, {\"label\": \"空间智能\", \"description\": \"三维世界理解技术方向\"}]\n  }\n]\n```"
	parsed, err := parseEventPersonTags(input)
	require.NoError(t, err)
	require.Len(t, parsed, 1)
	require.Equal(t, "李飞飞", parsed[0].Label)
	require.Equal(t, "person", parsed[0].Category)
}

func TestParseExtractedTagsWithUnescapedQuotes(t *testing.T) {
	input := `[{"label":"李飞飞","category":"person","aliases":["AI 教母"],"auxiliary_labels":[{"label":"李飞飞","description":"人工智能领域学者"},{"label":"World Labs","description":"空间智能创业公司"},{"label":"空间智能","description":"三维世界理解技术方向"}]}]`
	parsed, err := parseEventPersonTags(input)
	require.NoError(t, err, "valid JSON should parse fine")
	require.Len(t, parsed, 1)
}

func TestBuildExtractionSystemPromptLimitsAndOrdersTags(t *testing.T) {
	prompt := buildKeywordPrompt()

	require.True(t, strings.Contains(prompt, "最多返回 3 个 keyword") || strings.Contains(prompt, "最多返回3个 keyword"))
	require.True(t, strings.Contains(prompt, "按优先级从高到低排序") || strings.Contains(prompt, "按优先级排序"))
}

func TestBuildExtractionSystemPromptIncludesAuxiliaryLabelRules(t *testing.T) {
	prompt := buildEventPersonPrompt()
	keywordPrompt := buildKeywordPrompt()

	require.Contains(t, prompt, "auxiliary_labels")
	require.Contains(t, prompt, "3-5")
	require.Contains(t, prompt, "description")
	require.Contains(t, keywordPrompt, "keyword 不输出 auxiliary_labels")
	require.Contains(t, prompt, "泛词")
	require.Contains(t, prompt, "正例")
	require.Contains(t, prompt, "反例")
	require.NotContains(t, prompt, "sub_type")
	require.NotContains(t, prompt, "confidence")
}

func TestTagExtractionSchemaIncludesAuxiliaryLabelObjects(t *testing.T) {
	schema := eventPersonExtractionSchema()
	tags := schema.Properties["tags"]
	require.NotNil(t, tags.Items)
	require.NotContains(t, tags.Items.Properties, "sub_type")
	require.NotContains(t, tags.Items.Properties, "confidence")
	require.NotContains(t, tags.Items.Properties, "evidence")
	aux, ok := tags.Items.Properties["auxiliary_labels"]
	require.True(t, ok)
	require.Equal(t, "array", aux.Type)
	require.NotNil(t, aux.Items)
	require.Equal(t, "object", aux.Items.Type)
	require.Contains(t, aux.Items.Properties, "label")
	require.Contains(t, aux.Items.Properties, "description")
	require.Contains(t, aux.Items.Required, "label")
	require.Contains(t, aux.Items.Required, "description")
	require.ElementsMatch(t, []string{"label", "category", "auxiliary_labels"}, tags.Items.Required)

	keywordSchema := keywordExtractionSchema()
	keywordTags := keywordSchema.Properties["tags"]
	require.NotContains(t, keywordTags.Items.Properties, "auxiliary_labels")
	require.ElementsMatch(t, []string{"label", "category", "description"}, keywordTags.Items.Required)
}

func TestResolveCandidateDefaultsBusinessScore(t *testing.T) {
	extractor := &TagExtractor{}

	tag, skip, err := extractor.resolveCandidate(t.Context(), ExtractedTag{
		Label:    "PostgreSQL",
		Category: "keyword",
	}, ExtractionInput{})

	require.NoError(t, err)
	require.False(t, skip)
	require.Equal(t, 0.7, tag.Score)
}

func TestFixBrokenJSONWithUnescapedQuotes(t *testing.T) {
	input := `[{"label":"李飞飞","category":"person","aliases":["AI 教母"],"auxiliary_labels":[{"label":"李飞飞","description":"人工智能领域学者"},{"label":"World Labs","description":"空间智能创业公司"},{"label":"空间智能","description":"三维世界理解技术方向"}]},{"label":"World Labs","category":"keyword","aliases":[]}]`

	_, err := parseEventPersonTags(input)
	require.Error(t, err, "event/person parser should reject keyword tags from mixed responses")
}

func TestMergeExtractedTagsLimitsAndDedupesByCategoryPriority(t *testing.T) {
	merged := mergeExtractedTags([]ExtractedTag{
		{Label: "Sam Altman", Category: "person"},
		{Label: "OpenAI发布GPT-5", Category: "event"},
	}, []ExtractedTag{
		{Label: "Sam Altman", Category: "keyword"},
		{Label: "OpenAI", Category: "keyword"},
		{Label: "GPT-5", Category: "keyword"},
		{Label: "AI Agent", Category: "keyword"},
		{Label: "PostgreSQL", Category: "keyword"},
	})

	require.Len(t, merged, 5)
	require.Equal(t, "person", merged[0].Category)
	require.Equal(t, "Sam Altman", merged[0].Label)
	require.Equal(t, 3, countExtractedCategory(merged, "keyword"))
}

func TestMergeExtractedTagsKeepsHigherPriorityDuplicate(t *testing.T) {
	merged := mergeExtractedTags([]ExtractedTag{{Label: "Claude Code", Category: "event"}}, []ExtractedTag{{Label: "Claude Code", Category: "keyword"}})

	require.Len(t, merged, 1)
	require.Equal(t, "event", merged[0].Category)
}

func TestExtractTagsKeepsKeywordBranchWhenEventPersonFails(t *testing.T) {
	router := newFakeTagChatRouter()
	router.enqueue("tag_extraction_event_person", fakeTagChatResponse{err: errors.New("event branch down")})
	router.enqueue("tag_extraction_event_person", fakeTagChatResponse{err: errors.New("event branch down")})
	router.enqueue("tag_extraction_event_person", fakeTagChatResponse{err: errors.New("event branch down")})
	router.enqueue("tag_extraction_keyword", fakeTagChatResponse{
		content: `{"tags":[{"label":"PostgreSQL","category":"keyword","description":"开源关系型数据库管理系统"}]}`,
		delay:   20 * time.Millisecond,
	})
	extractor := &TagExtractor{router: router}

	result, err := extractor.ExtractTags(context.Background(), ExtractionInput{Title: "PostgreSQL update", Summary: "PostgreSQL releases a new version."})

	require.NoError(t, err)
	require.Equal(t, "llm", result.Source)
	require.Len(t, result.Tags, 1)
	require.Equal(t, "PostgreSQL", result.Tags[0].Label)
	require.Contains(t, strings.Join(result.Errors, "\n"), "event/person extraction failed")
	require.Equal(t, 3, router.callCount("tag_extraction_event_person"))
	require.Equal(t, 1, router.callCount("tag_extraction_keyword"))
}

func TestExtractTagsFallsBackToHeuristicKeywordWhenKeywordBranchFails(t *testing.T) {
	router := newFakeTagChatRouter()
	router.enqueue("tag_extraction_event_person", fakeTagChatResponse{content: `{"tags":[{"label":"OpenAI发布GPT-5","category":"event","auxiliary_labels":[{"label":"OpenAI","description":"人工智能研究公司"},{"label":"GPT-5","description":"大语言模型版本"},{"label":"模型发布","description":"产品发布行为"}]}]}`})
	router.enqueue("tag_extraction_keyword", fakeTagChatResponse{err: errors.New("keyword branch down")})
	router.enqueue("tag_extraction_keyword", fakeTagChatResponse{err: errors.New("keyword branch down")})
	router.enqueue("tag_extraction_keyword", fakeTagChatResponse{err: errors.New("keyword branch down")})
	extractor := &TagExtractor{router: router}

	result, err := extractor.ExtractTags(context.Background(), ExtractionInput{
		Title:   "OpenAI pushes GPT-5 agent workflow",
		Summary: "OpenAI is shipping a new AI agent workflow around GPT-5 with coding automation.",
	})

	require.NoError(t, err)
	require.Equal(t, "llm", result.Source)
	require.Contains(t, topicLabels(result.Tags), "OpenAI发布GPT-5")
	require.Contains(t, strings.Join(result.Errors, "\n"), "keyword extraction failed")
	for _, tag := range result.Tags {
		if tag.Category == "keyword" {
			require.Empty(t, tag.Description)
			require.Empty(t, tag.AuxiliaryLabels)
		}
	}
}

func countExtractedCategory(items []ExtractedTag, category string) int {
	count := 0
	for _, item := range items {
		if item.Category == category {
			count++
		}
	}
	return count
}

type fakeTagChatResponse struct {
	content string
	err     error
	delay   time.Duration
}

type fakeTagChatRouter struct {
	mu        sync.Mutex
	responses map[string][]fakeTagChatResponse
	calls     map[string]int
}

func newFakeTagChatRouter() *fakeTagChatRouter {
	return &fakeTagChatRouter{responses: make(map[string][]fakeTagChatResponse), calls: make(map[string]int)}
}

func (f *fakeTagChatRouter) enqueue(operation string, response fakeTagChatResponse) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.responses[operation] = append(f.responses[operation], response)
}

func (f *fakeTagChatRouter) Chat(ctx context.Context, req airouter.ChatRequest) (*airouter.ChatResult, error) {
	operation, _ := req.Metadata["operation"].(string)
	f.mu.Lock()
	f.calls[operation]++
	responses := f.responses[operation]
	if len(responses) == 0 {
		f.mu.Unlock()
		return nil, fmt.Errorf("no fake response for %s", operation)
	}
	response := responses[0]
	f.responses[operation] = responses[1:]
	f.mu.Unlock()

	if response.delay > 0 {
		select {
		case <-time.After(response.delay):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	if response.err != nil {
		return nil, response.err
	}
	return &airouter.ChatResult{Content: response.content}, nil
}

func (f *fakeTagChatRouter) callCount(operation string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls[operation]
}
