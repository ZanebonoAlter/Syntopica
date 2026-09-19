package core

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

func TestParseMergedExtractionTagsAcceptsWrappedDoubleArrays(t *testing.T) {
	// 兼容形态：双数组整体被包在 {"tags": …} 里。
	eventParsed, keywordParsed, err := parseMergedExtractionTags(`{"tags":{"event_person_tags":[],"keyword_tags":[{"label":"OpenAI","category":"keyword","aliases":["Open AI"],"description":"人工智能研究公司"}]}}`)

	require.NoError(t, err)
	require.Empty(t, eventParsed)
	require.Len(t, keywordParsed, 1)
	require.Equal(t, "OpenAI", keywordParsed[0].Label)
	require.Equal(t, "keyword", keywordParsed[0].Category)
	require.Equal(t, []string{"Open AI"}, keywordParsed[0].Aliases)
	require.Empty(t, keywordParsed[0].AuxiliaryLabels)
}

func TestParseMergedExtractionTagsAcceptsCanonicalDoubleArrays(t *testing.T) {
	eventParsed, keywordParsed, err := parseMergedExtractionTags(`{"event_person_tags":[{"label":"伊朗袭击以色列","category":"event","auxiliary_labels":[{"label":"伊朗","description":"中东地区国家"},{"label":"以色列","description":"中东地区国家"},{"label":"导弹袭击","description":"军事打击行动"}]}],"keyword_tags":[]}`)

	require.NoError(t, err)
	require.Len(t, eventParsed, 1)
	require.Equal(t, []AuxiliaryLabel{
		{Label: "伊朗", Description: "中东地区国家"},
		{Label: "以色列", Description: "中东地区国家"},
		{Label: "导弹袭击", Description: "军事打击行动"},
	}, eventParsed[0].AuxiliaryLabels)
	require.Empty(t, keywordParsed)
}

func TestParseMergedExtractionTagsRequiresKeywordDescription(t *testing.T) {
	_, keywordParsed, err := parseMergedExtractionTags(`{"event_person_tags":[],"keyword_tags":[{"label":"PostgreSQL","category":"keyword","description":"开源关系型数据库管理系统"},{"label":"Claude Code","category":"keyword","description":"Anthropic推出的AI编程助手","auxiliary_labels":[]}]}`)

	require.NoError(t, err)
	require.Len(t, keywordParsed, 2)
	require.Empty(t, keywordParsed[0].AuxiliaryLabels)
	require.Empty(t, keywordParsed[1].AuxiliaryLabels)

	_, _, err = parseMergedExtractionTags(`{"keyword_tags":[{"label":"Claude Code","category":"keyword"}]}`)
	require.Error(t, err)
}

func TestParseMergedExtractionTagsIgnoresKeywordAuxiliaryLabels(t *testing.T) {
	eventParsed, keywordParsed, err := parseMergedExtractionTags(`{"event_person_tags":[],"keyword_tags":[{"label":"OpenAI","category":"keyword","description":"人工智能研究公司","auxiliary_labels":[{"label":"GPT-5","description":"大语言模型"}]}]}`)

	require.NoError(t, err)
	require.Empty(t, eventParsed)
	require.Len(t, keywordParsed, 1)
	require.Empty(t, keywordParsed[0].AuxiliaryLabels, "keyword auxiliary labels are ignored")
}

func TestParseMergedExtractionTagsRejectsKeywordCategoryInEventPersonArray(t *testing.T) {
	_, _, err := parseMergedExtractionTags(`{"event_person_tags":[{"label":"OpenAI","category":"keyword","description":"人工智能研究公司"}],"keyword_tags":[]}`)

	require.Error(t, err, "event/person array must not contain keyword tags")
}

func TestParseMergedExtractionTagsAcceptsEmptyArrays(t *testing.T) {
	// 纯分隔符/全空形态不报错，走数组空路径（由上层记录缺失或兜底）。
	for _, content := range []string{"{}", `{"tags":[]}`, `{"event_person_tags":[],"keyword_tags":[]}`} {
		eventParsed, keywordParsed, err := parseMergedExtractionTags(content)
		require.NoError(t, err, content)
		require.Empty(t, eventParsed, content)
		require.Empty(t, keywordParsed, content)
	}
}

func TestParseMergedExtractionTagsEmptyObjectWhitelist(t *testing.T) {
	// `{}` 是键集检查的白名单特例：保留双空数组路径。
	eventParsed, keywordParsed, err := parseMergedExtractionTags(`{}`)

	require.NoError(t, err)
	require.Empty(t, eventParsed)
	require.Empty(t, keywordParsed)
}

func TestParseMergedExtractionTagsRejectsObjectWithoutTargetKeys(t *testing.T) {
	// 错形态响应（合法 JSON 但两个目标键都不存在）必须报错进重试，不能静默当双空数组。
	for _, content := range []string{
		`{"foo":"bar"}`,
		`{"result":[{"label":"PostgreSQL","category":"keyword","description":"开源关系型数据库"}]}`,
	} {
		_, _, err := parseMergedExtractionTags(content)
		require.Error(t, err, content)
	}
}

func TestExtractTagsRetriesThenFallsBackToHeuristicOnUnparsableResponse(t *testing.T) {
	// parse err 分支：错形态响应 ×3 重试耗尽 → heuristic 兜底（区别于 router err 分支）。
	router := newFakeTagChatRouter()
	router.enqueue("tag_extraction_merged", fakeTagChatResponse{content: `{"foo":"bar"}`})
	router.enqueue("tag_extraction_merged", fakeTagChatResponse{content: `{"foo":"bar"}`})
	router.enqueue("tag_extraction_merged", fakeTagChatResponse{content: `{"foo":"bar"}`})
	extractor := &TagExtractor{router: router}

	result, err := extractor.ExtractTags(context.Background(), ExtractionInput{
		Title:   "OpenAI pushes GPT-5 agent workflow",
		Summary: "OpenAI is shipping a new AI agent workflow around GPT-5 with coding automation.",
	})

	require.NoError(t, err)
	require.Equal(t, "heuristic", result.Source)
	require.NotEmpty(t, result.Tags)
	require.Contains(t, strings.Join(result.Errors, "\n"), "parse extraction result failed")
	require.Equal(t, 3, router.callCount("tag_extraction_merged"))
}

func TestParseExtractedTagsDegradesOnInvalidAuxiliaryLabels(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{
			name:  "event missing labels",
			input: `{"event_person_tags":[{"label":"OpenAI发布GPT-5","category":"event"}]}`,
		},
		{
			name:  "too few objects",
			input: `{"event_person_tags":[{"label":"OpenAI发布GPT-5","category":"event","auxiliary_labels":[{"label":"OpenAI","description":"人工智能公司"},{"label":"GPT-5","description":"大语言模型"}]}]}`,
		},
		{
			name:  "too many objects",
			input: `{"event_person_tags":[{"label":"OpenAI发布GPT-5","category":"event","auxiliary_labels":[{"label":"OpenAI","description":"人工智能公司"},{"label":"GPT-5","description":"大语言模型"},{"label":"模型发布","description":"产品发布行为"},{"label":"Sam Altman","description":"OpenAI负责人"},{"label":"AI助手","description":"人工智能辅助工具"},{"label":"API","description":"应用程序接口"}]}]}`,
		},
		{
			name:  "missing description",
			input: `{"event_person_tags":[{"label":"OpenAI发布GPT-5","category":"event","auxiliary_labels":[{"label":"OpenAI","description":"人工智能公司"},{"label":"GPT-5"},{"label":"模型发布","description":"产品发布行为"}]}]}`,
		},
		{
			name:  "empty description",
			input: `{"event_person_tags":[{"label":"OpenAI发布GPT-5","category":"event","auxiliary_labels":[{"label":"OpenAI","description":"人工智能公司"},{"label":"GPT-5","description":""},{"label":"模型发布","description":"产品发布行为"}]}]}`,
		},
		{
			name:  "description equals label",
			input: `{"event_person_tags":[{"label":"OpenAI发布GPT-5","category":"event","auxiliary_labels":[{"label":"OpenAI","description":"人工智能公司"},{"label":"GPT-5","description":"GPT-5"},{"label":"模型发布","description":"产品发布行为"}]}]}`,
		},
		{
			name:  "generic label",
			input: `{"event_person_tags":[{"label":"OpenAI发布GPT-5","category":"event","auxiliary_labels":[{"label":"OpenAI","description":"人工智能公司"},{"label":"技术","description":"宽泛技术词"},{"label":"模型发布","description":"产品发布行为"}]}]}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			eventParsed, keywordParsed, err := parseMergedExtractionTags(tt.input)
			require.NoError(t, err)
			require.Len(t, eventParsed, 1)
			require.Equal(t, "OpenAI发布GPT-5", eventParsed[0].Label)
			require.Equal(t, "event", eventParsed[0].Category)
			require.Empty(t, eventParsed[0].AuxiliaryLabels, "invalid aux should degrade to empty, tag kept")
			require.Empty(t, keywordParsed)
		})
	}
}

func TestParseExtractedTagsDegradesOldAuxiliaryLabelStringArrayToEmpty(t *testing.T) {
	eventParsed, _, err := parseMergedExtractionTags(`{"event_person_tags":[{"label":"OpenAI发布GPT-5","category":"event","auxiliary_labels":["OpenAI","GPT-5","模型发布"]}]}`)

	require.NoError(t, err)
	require.Len(t, eventParsed, 1)
	require.Empty(t, eventParsed[0].AuxiliaryLabels, "old-format aux should degrade to empty, tag kept")
}

func TestParseExtractedTagsAcceptsSurroundingText(t *testing.T) {
	eventParsed, keywordParsed, err := parseMergedExtractionTags("以下是提取结果：\n```json\n{\"keyword_tags\":[{\"label\":\"AI Agent\",\"category\":\"keyword\",\"description\":\"能自主执行任务的人工智能系统\"}]}\n```\n请使用这些标签。")

	require.NoError(t, err)
	require.Empty(t, eventParsed)
	require.Len(t, keywordParsed, 1)
	require.Equal(t, "AI Agent", keywordParsed[0].Label)
	require.Equal(t, "keyword", keywordParsed[0].Category)
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
	eventParsed, keywordParsed, err := parseMergedExtractionTags(input)
	// 旧单数组形态：按 category 拆分进两个数组。
	require.NoError(t, err)
	require.Len(t, eventParsed, 1)
	require.Equal(t, "李飞飞", eventParsed[0].Label)
	require.Equal(t, "person", eventParsed[0].Category)
	require.Empty(t, keywordParsed)
}

func TestParseExtractedTagsWithUnescapedQuotes(t *testing.T) {
	input := `[{"label":"李飞飞","category":"person","aliases":["AI 教母"],"auxiliary_labels":[{"label":"李飞飞","description":"人工智能领域学者"},{"label":"World Labs","description":"空间智能创业公司"},{"label":"空间智能","description":"三维世界理解技术方向"}]}]`
	eventParsed, _, err := parseMergedExtractionTags(input)
	require.NoError(t, err, "valid JSON should parse fine")
	require.Len(t, eventParsed, 1)
}

func TestBuildExtractionSystemPromptLimitsAndOrdersTags(t *testing.T) {
	prompt := buildMergedExtractionPrompt()

	require.LessOrEqual(t, len([]rune(prompt)), 1500, "merged system prompt must stay within the 1500-rune budget")
	require.Contains(t, prompt, "最多返回 3 个 keyword")
	require.Contains(t, prompt, "按优先级从高到低排序")
}

func TestBuildExtractionSystemPromptIncludesAuxiliaryLabelRules(t *testing.T) {
	prompt := buildMergedExtractionPrompt()

	require.Contains(t, prompt, "auxiliary_labels")
	require.Contains(t, prompt, "3-5")
	require.Contains(t, prompt, "description")
	require.Contains(t, prompt, "keyword 不输出 auxiliary_labels")
	require.Contains(t, prompt, "泛词")
	require.Contains(t, prompt, "正例")
	require.Contains(t, prompt, "反例")
	require.Contains(t, prompt, "event_person_tags")
	require.Contains(t, prompt, "keyword_tags")
	require.NotContains(t, prompt, "sub_type")
	require.NotContains(t, prompt, "confidence")
}

func TestTagExtractionSchemaIncludesAuxiliaryLabelObjects(t *testing.T) {
	schema := mergedTagExtractionSchema()
	require.ElementsMatch(t, []string{"event_person_tags", "keyword_tags"}, schema.Required)

	eventPersonTags := schema.Properties["event_person_tags"]
	require.NotNil(t, eventPersonTags.Items)
	require.NotContains(t, eventPersonTags.Items.Properties, "sub_type")
	require.NotContains(t, eventPersonTags.Items.Properties, "confidence")
	require.NotContains(t, eventPersonTags.Items.Properties, "evidence")
	aux, ok := eventPersonTags.Items.Properties["auxiliary_labels"]
	require.True(t, ok)
	require.Equal(t, "array", aux.Type)
	require.NotNil(t, aux.Items)
	require.Equal(t, "object", aux.Items.Type)
	require.Contains(t, aux.Items.Properties, "label")
	require.Contains(t, aux.Items.Properties, "description")
	require.Contains(t, aux.Items.Required, "label")
	require.Contains(t, aux.Items.Required, "description")
	require.ElementsMatch(t, []string{"label", "category", "auxiliary_labels"}, eventPersonTags.Items.Required)

	keywordTags := schema.Properties["keyword_tags"]
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

func TestParseMergedExtractionTagsRoutesLegacyMixedArrayByCategory(t *testing.T) {
	// 旧单数组里 event/person 与 keyword 混排：现在按 category 拆分，不再整体拒绝。
	input := `[{"label":"李飞飞","category":"person","aliases":["AI 教母"],"auxiliary_labels":[{"label":"李飞飞","description":"人工智能领域学者"},{"label":"World Labs","description":"空间智能创业公司"},{"label":"空间智能","description":"三维世界理解技术方向"}]},{"label":"World Labs","category":"keyword","aliases":[],"description":"空间智能创业公司"}]`

	eventParsed, keywordParsed, err := parseMergedExtractionTags(input)

	require.NoError(t, err)
	require.Len(t, eventParsed, 1)
	require.Equal(t, "person", eventParsed[0].Category)
	require.Len(t, keywordParsed, 1)
	require.Equal(t, "keyword", keywordParsed[0].Category)
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
	// 单次调用成功但 event/person 数组为空 → 数组级缺失：保 keyword、不触发 heuristic。
	router := newFakeTagChatRouter()
	router.enqueue("tag_extraction_merged", fakeTagChatResponse{
		content: `{"event_person_tags":[],"keyword_tags":[{"label":"PostgreSQL","category":"keyword","description":"开源关系型数据库管理系统"}]}`,
	})
	extractor := &TagExtractor{router: router}

	result, err := extractor.ExtractTags(context.Background(), ExtractionInput{Title: "PostgreSQL update", Summary: "PostgreSQL releases a new version."})

	require.NoError(t, err)
	require.Equal(t, "llm", result.Source)
	require.Len(t, result.Tags, 1)
	require.Equal(t, "PostgreSQL", result.Tags[0].Label)
	require.Contains(t, strings.Join(result.Errors, "\n"), "event/person extraction failed")
	require.Equal(t, 1, router.callCount("tag_extraction_merged"))
}

func TestExtractTagsFallsBackToHeuristicKeywordWhenKeywordBranchFails(t *testing.T) {
	// 单次调用成功但 keyword 数组为空 → heuristic keyword 展示兑底（不入辅助池）。
	router := newFakeTagChatRouter()
	router.enqueue("tag_extraction_merged", fakeTagChatResponse{content: `{"event_person_tags":[{"label":"OpenAI发布GPT-5","category":"event","auxiliary_labels":[{"label":"OpenAI","description":"人工智能研究公司"},{"label":"GPT-5","description":"大语言模型版本"},{"label":"模型发布","description":"产品发布行为"}]}],"keyword_tags":[]}`})
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

func TestExtractTagsFallsBackToHeuristicWhenMergedCallFails(t *testing.T) {
	// spec Scenario: 单次调用重试耗尽仍失败 → 整体回退 heuristic，错误信息保留。
	router := newFakeTagChatRouter()
	router.enqueue("tag_extraction_merged", fakeTagChatResponse{err: errors.New("model down")})
	router.enqueue("tag_extraction_merged", fakeTagChatResponse{err: errors.New("model down")})
	router.enqueue("tag_extraction_merged", fakeTagChatResponse{err: errors.New("model down")})
	extractor := &TagExtractor{router: router}

	result, err := extractor.ExtractTags(context.Background(), ExtractionInput{
		Title:   "OpenAI pushes GPT-5 agent workflow",
		Summary: "OpenAI is shipping a new AI agent workflow around GPT-5 with coding automation.",
	})

	require.NoError(t, err)
	require.Equal(t, "heuristic", result.Source)
	require.NotEmpty(t, result.Tags)
	require.Contains(t, strings.Join(result.Errors, "\n"), "AI extraction failed")
	require.Equal(t, 3, router.callCount("tag_extraction_merged"))
}

func TestExtractTagsMakesExactlyOneMergedCall(t *testing.T) {
	// spec Scenario: mono 文章提取调用数为 1（函数级落点，线上对账见 V5①）。
	router := newFakeTagChatRouter()
	router.enqueue("tag_extraction_merged", fakeTagChatResponse{content: `{"event_person_tags":[{"label":"OpenAI发布GPT-5","category":"event","auxiliary_labels":[{"label":"OpenAI","description":"人工智能研究公司"},{"label":"GPT-5","description":"大语言模型版本"},{"label":"模型发布","description":"产品发布行为"}]}],"keyword_tags":[{"label":"PostgreSQL","category":"keyword","description":"开源关系型数据库管理系统"}]}`})
	extractor := &TagExtractor{router: router}

	result, err := extractor.ExtractTags(context.Background(), ExtractionInput{Title: "OpenAI pushes GPT-5", Summary: "OpenAI is shipping GPT-5."})

	require.NoError(t, err)
	require.Equal(t, "llm", result.Source)
	require.Len(t, result.Tags, 2)
	require.Equal(t, 1, router.callCount("tag_extraction_merged"), "merged extraction must make exactly one Chat call")
	require.Equal(t, "tagmanagement.extractor_enhanced", router.lastOperation)
	metaOperation, _ := router.lastMetadata["operation"].(string)
	require.Equal(t, "tag_extraction_merged", metaOperation)
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
	mu            sync.Mutex
	responses     map[string][]fakeTagChatResponse
	calls         map[string]int
	lastOperation string
	lastMetadata  map[string]any
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
	f.lastOperation = req.Operation
	f.lastMetadata = req.Metadata
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

func TestParseEventPersonTagsToleratesTrailingCommas(t *testing.T) {
	eventParsed, _, err := parseMergedExtractionTags(`{"event_person_tags":[{"label":"伊朗袭击以色列","category":"event","auxiliary_labels":[{"label":"伊朗","description":"中东地区国家"},{"label":"以色列","description":"中东国家"},{"label":"导弹袭击","description":"军事打击行动"},],},],"keyword_tags":[]}`)

	require.NoError(t, err)
	require.Len(t, eventParsed, 1)
	require.Equal(t, "伊朗袭击以色列", eventParsed[0].Label)
	require.Len(t, eventParsed[0].AuxiliaryLabels, 3)
}

func TestParseKeywordTagsToleratesTrailingCommas(t *testing.T) {
	_, keywordParsed, err := parseMergedExtractionTags(`{"event_person_tags":[],"keyword_tags":[{"label":"PostgreSQL","category":"keyword","description":"开源关系型数据库",},]}`)

	require.NoError(t, err)
	require.Len(t, keywordParsed, 1)
	require.Equal(t, "PostgreSQL", keywordParsed[0].Label)
}
