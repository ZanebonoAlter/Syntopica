package core

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"syntopica-backend/internal/platform/airouter"
)

const validSectionResponse = `{"tags":[` +
	`{"label":"伊朗袭击以色列","category":"event","aliases":["伊以冲突"],"description":"伊朗对以色列发动军事打击的新闻事件","auxiliary_labels":[{"label":"伊朗","description":"中东地区国家"},{"label":"以色列","description":"中东国家"},{"label":"导弹袭击","description":"以导弹进行军事打击的行动"}]},` +
	`{"label":"Sam Altman","category":"person","description":"","auxiliary_labels":[{"label":"OpenAI","description":"人工智能研究公司"},{"label":"ChatGPT","description":"对话式AI产品"},{"label":"GPT-5","description":"大语言模型版本"}]},` +
	`{"label":"Kubernetes","category":"keyword","aliases":["K8s"],"description":"开源容器编排系统"}]}`

// recordingTagChatRouter captures every request and always replies with content.
type recordingTagChatRouter struct {
	mu       sync.Mutex
	requests []airouter.ChatRequest
	content  string
	err      error
}

func newRecordingTagChatRouter() *recordingTagChatRouter {
	return &recordingTagChatRouter{}
}

func (r *recordingTagChatRouter) Chat(_ context.Context, req airouter.ChatRequest) (*airouter.ChatResult, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.requests = append(r.requests, req)
	if r.err != nil {
		return nil, r.err
	}
	return &airouter.ChatResult{Content: r.content}, nil
}

func TestParseSectionTagsMixedCategories(t *testing.T) {
	tags, err := parseSectionTags(validSectionResponse)

	require.NoError(t, err)
	require.Len(t, tags, 3)
	require.Equal(t, "event", tags[0].Category)
	require.Len(t, tags[0].AuxiliaryLabels, 3)
	require.Equal(t, "person", tags[1].Category)
	require.Len(t, tags[1].AuxiliaryLabels, 3)
	require.Equal(t, "keyword", tags[2].Category)
	require.Empty(t, tags[2].AuxiliaryLabels)
	require.Equal(t, "开源容器编排系统", tags[2].Description)
}

func TestParseSectionTagsInvalidCategoryDefaultsToKeyword(t *testing.T) {
	tags, err := parseSectionTags(`{"tags":[{"label":"某概念","category":"topic","description":"某概念的解释"}]}`)

	require.NoError(t, err)
	require.Len(t, tags, 1)
	require.Equal(t, "keyword", tags[0].Category)
}

func TestParseSectionTagsTruncatesToFour(t *testing.T) {
	var b strings.Builder
	b.WriteString(`{"tags":[`)
	for i := 0; i < 6; i++ {
		if i > 0 {
			b.WriteString(",")
		}
		b.WriteString(`{"label":"关键词` + string(rune('A'+i)) + `","category":"keyword","description":"描述` + string(rune('A'+i)) + `"}`)
	}
	b.WriteString(`]}`)

	tags, err := parseSectionTags(b.String())

	require.NoError(t, err)
	require.Len(t, tags, 4)
	require.Equal(t, "关键词A", tags[0].Label)
	require.Equal(t, "关键词D", tags[3].Label)
}

func TestParseSectionTagsDegradesEventWithoutAuxiliaryLabels(t *testing.T) {
	parsed, err := parseSectionTags(`{"tags":[{"label":"某发布会","category":"event","description":"某发布会的描述"},{"label":"Kubernetes","category":"keyword","description":"容器编排"}]}`)

	require.NoError(t, err)
	require.Len(t, parsed, 2)
	require.Equal(t, "某发布会", parsed[0].Label)
	require.Equal(t, "event", parsed[0].Category)
	require.Empty(t, parsed[0].AuxiliaryLabels, "invalid aux should degrade to empty, tag kept")
	require.Len(t, parsed[1].AuxiliaryLabels, 0)
}

func TestParseSectionTagsSkipsKeywordWithoutDescription(t *testing.T) {
	parsed, err := parseSectionTags(`{"tags":[{"label":"PostgreSQL","category":"keyword"},{"label":"Docker","category":"keyword","description":"容器引擎"}]}`)

	require.NoError(t, err)
	require.Len(t, parsed, 1)
	require.Equal(t, "Docker", parsed[0].Label)
}

func TestExtractTagsFromSectionSingleCall(t *testing.T) {
	router := newFakeTagChatRouter()
	router.enqueue("tag_extraction_section", fakeTagChatResponse{content: validSectionResponse})
	extractor := &TagExtractor{router: router}

	// 正文含 fixture 里的示例词提及（Sam Altman/Kubernetes 均为 section prompt
	// few-shot 正例）：护栏的放行分支需要正文有依据（guard-tag-exemplar-leakage）。
	sectionBody := "AI缓存技术取得新突破。Sam Altman 团队将其部署到 Kubernetes 集群。"

	tags, err := extractor.extractTagsFromSection(context.Background(), router, ExtractionInput{
		Title: "科技周刊第100期", Summary: "unused", FeedName: "科技周刊", CategoryName: "科技",
	}, "科技动态", sectionBody, 0)

	require.NoError(t, err)
	require.Len(t, tags, 3)
	require.Equal(t, 1, router.callCount("tag_extraction_section"))
}

func TestExtractTagsFromSectionRetriesOnInvalidJSON(t *testing.T) {
	router := newFakeTagChatRouter()
	router.enqueue("tag_extraction_section", fakeTagChatResponse{content: "not json"})
	router.enqueue("tag_extraction_section", fakeTagChatResponse{content: "still not json"})
	router.enqueue("tag_extraction_section", fakeTagChatResponse{content: "nope"})
	extractor := &TagExtractor{router: router}

	tags, err := extractor.extractTagsFromSection(context.Background(), router, ExtractionInput{Title: "标题"}, "科技动态", "正文", 1)

	require.Error(t, err)
	require.Nil(t, tags)
	require.Equal(t, 3, router.callCount("tag_extraction_section"))
}

func TestExtractTagsFromSectionCarriesSectionMetadata(t *testing.T) {
	router := newRecordingTagChatRouter()
	router.content = validSectionResponse
	extractor := &TagExtractor{router: router}

	_, err := extractor.extractTagsFromSection(context.Background(), router, ExtractionInput{
		Title: "科技周刊第100期", FeedName: "科技周刊", CategoryName: "科技", PubDate: "2026-09-01",
	}, "科技动态", "AI缓存技术取得新突破。Sam Altman 团队将其部署到 Kubernetes 集群。", 2)

	require.NoError(t, err)
	require.Len(t, router.requests, 1)
	req := router.requests[0]
	require.Equal(t, "tagmanagement.extractor_section", req.Operation)
	require.Equal(t, airouter.CapabilityTopicTagging, req.Capability)
	require.NotNil(t, req.Temperature)
	require.Equal(t, 0.2, *req.Temperature)
	require.NotNil(t, req.MaxTokens)
	require.Equal(t, 2048, *req.MaxTokens)
	require.True(t, req.JSONMode)
	require.NotNil(t, req.JSONSchema)
	require.Equal(t, "tag_extraction_section", req.Metadata["operation"])
	require.Equal(t, 2, req.Metadata["section_index"])
	require.Equal(t, "科技动态", req.Metadata["section_title"])

	require.Len(t, req.Messages, 2)
	require.Equal(t, "system", req.Messages[0].Role)
	require.Equal(t, "user", req.Messages[1].Role)
	userPrompt := req.Messages[1].Content
	require.Contains(t, userPrompt, "科技周刊第100期")
	require.Contains(t, userPrompt, "科技动态")
	require.Contains(t, userPrompt, "AI缓存技术取得新突破。")
}

func TestExtractTagsFromSectionRetriesOnRouterError(t *testing.T) {
	router := newFakeTagChatRouter()
	router.enqueue("tag_extraction_section", fakeTagChatResponse{err: errors.New("router down")})
	router.enqueue("tag_extraction_section", fakeTagChatResponse{err: errors.New("router down")})
	router.enqueue("tag_extraction_section", fakeTagChatResponse{err: errors.New("router down")})
	extractor := &TagExtractor{router: router}

	tags, err := extractor.extractTagsFromSection(context.Background(), router, ExtractionInput{Title: "标题"}, "科技动态", "正文", 0)

	require.Error(t, err)
	require.Nil(t, tags)
	require.Equal(t, 3, router.callCount("tag_extraction_section"))
	require.Contains(t, err.Error(), "section extraction failed (attempt 3/3)")
}

func TestExtractTagsFromSectionSucceedsAfterRetry(t *testing.T) {
	router := newFakeTagChatRouter()
	router.enqueue("tag_extraction_section", fakeTagChatResponse{content: "broken"})
	router.enqueue("tag_extraction_section", fakeTagChatResponse{content: validSectionResponse})
	extractor := &TagExtractor{router: router}

	tags, err := extractor.extractTagsFromSection(context.Background(), router, ExtractionInput{Title: "标题"}, "科技动态", "Sam Altman 的 Kubernetes 部署实践。", 0)

	require.NoError(t, err)
	require.Len(t, tags, 3)
	require.Equal(t, 2, router.callCount("tag_extraction_section"))
}

func TestSectionExtractionSchemaFusedShape(t *testing.T) {
	schema := sectionExtractionSchema()
	require.Equal(t, "object", schema.Type)
	require.Contains(t, schema.Required, "tags")
	tags := schema.Properties["tags"]
	require.NotNil(t, tags.Items)
	require.Equal(t, "object", tags.Items.Type)
	require.ElementsMatch(t, []string{"label", "category"}, tags.Items.Required)
	for _, prop := range []string{"label", "category", "aliases", "description", "auxiliary_labels"} {
		require.Contains(t, tags.Items.Properties, prop)
	}
	aux := tags.Items.Properties["auxiliary_labels"]
	require.Equal(t, "array", aux.Type)
	require.NotNil(t, aux.Items)
	require.Contains(t, aux.Items.Properties, "label")
	require.Contains(t, aux.Items.Properties, "description")
}

func TestSectionExtractionPromptIncludesFusedRules(t *testing.T) {
	prompt := buildSectionExtractionPrompt()

	require.Contains(t, prompt, "event（事件）")
	require.Contains(t, prompt, "person（人物）")
	require.Contains(t, prompt, "keyword（关键词）")
	require.Contains(t, prompt, "栏目片")
	require.Contains(t, prompt, "最多输出 4 个标签")
	require.Contains(t, prompt, "auxiliary_labels")
	require.Contains(t, prompt, "3-5")
	require.Contains(t, prompt, "keyword 不输出 auxiliary_labels")
	require.Contains(t, prompt, "泛词")
	require.Contains(t, prompt, "正例")
}

func TestParseSectionTagsToleratesTrailingCommas(t *testing.T) {
	parsed, err := parseSectionTags(`{"tags":[{"label":"Kubernetes","category":"keyword","description":"开源容器编排系统",},{"label":"苹果WWDC发布会","category":"event","auxiliary_labels":[{"label":"苹果","description":"科技公司"},{"label":"WWDC","description":"开发者大会"},{"label":"发布会","description":"产品发布活动"},],},],}`)

	require.NoError(t, err)
	require.Len(t, parsed, 2)
	require.Equal(t, "Kubernetes", parsed[0].Label)
	require.Equal(t, "苹果WWDC发布会", parsed[1].Label)
	require.Len(t, parsed[1].AuxiliaryLabels, 3)
}
