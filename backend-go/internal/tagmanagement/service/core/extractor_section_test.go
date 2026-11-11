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
	require.Equal(t, extractionMaxOutputTokens, *req.MaxTokens)
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

// add-event-story-heat：事件指纹与标签同一次融合调用产出。event 携带五字段
// （subject_key 归一化），person/keyword 即使模型误输出指纹字段也一律忽略。
const sectionFingerprintResponse = `{"tags":[` +
	`{"label":"特朗普宣布参加总统选举","category":"event","description":"特朗普宣布参选的政治事件","auxiliary_labels":[{"label":"特朗普","description":"美国政治人物"},{"label":"总统选举","description":"国家领导人选举"},{"label":"参选宣布","description":"正式加入竞选"}],"subject":"特朗普","subject_key":"Person:Donald-Trump","action":"宣布","object":"参加总统选举","occurred_at":"2026-11-01"},` +
	`{"label":"李飞飞谈空间智能","category":"person","description":"","auxiliary_labels":[{"label":"空间智能","description":"三维世界理解技术"},{"label":"World Labs","description":"空间智能公司"},{"label":"李飞飞","description":"人工智能学者"}],"subject":"李飞飞","subject_key":"person:feifei_li","action":"谈","object":"空间智能"},` +
	`{"label":"PostgreSQL","category":"keyword","description":"开源关系型数据库","subject":"PostgreSQL","subject_key":"product:postgresql","action":"发布","object":"新版本"}]}`

func TestParseSectionTagsEventFingerprint(t *testing.T) {
	tags, err := parseSectionTags(sectionFingerprintResponse)

	require.NoError(t, err)
	require.Len(t, tags, 3)

	// event：五字段解析，subject_key 归一化（大小写 + 连字符 → 下划线）。
	fp := tags[0].Fingerprint
	require.NotNil(t, fp)
	require.True(t, fp.Complete())
	require.Equal(t, "特朗普", fp.Subject)
	require.Equal(t, "person:donald_trump", fp.SubjectKey)
	require.Equal(t, "宣布", fp.Action)
	require.Equal(t, "参加总统选举", fp.Object)
	require.Equal(t, "2026-11-01", fp.OccurredAt)

	// person：指纹字段被忽略，不构造。
	require.Nil(t, tags[1].Fingerprint)
	// keyword：指纹字段被忽略，不构造。
	require.Nil(t, tags[2].Fingerprint)
}

func TestParseSectionTagsIncompleteFingerprintKeepsTag(t *testing.T) {
	tags, err := parseSectionTags(`{"tags":[{"label":"某公司产品涨价","category":"event","description":"某公司产品涨价事件","auxiliary_labels":[{"label":"某公司","description":"商业公司"},{"label":"产品","description":"商品"},{"label":"涨价","description":"提高价格"}],"subject":"某公司","action":"涨价"}]}`)

	// 缺 subject_key/object：标签照常保留，指纹不完整但不丢弃。
	require.NoError(t, err)
	require.Len(t, tags, 1)
	require.NotNil(t, tags[0].Fingerprint)
	require.False(t, tags[0].Fingerprint.Complete())
	require.Equal(t, "某公司", tags[0].Fingerprint.Subject)
	require.Equal(t, "涨价", tags[0].Fingerprint.Action)
}

func TestParseSectionTagsEventWithoutFingerprintFields(t *testing.T) {
	// 旧模型/存量响应无指纹字段：event 标签照常解析，Fingerprint 为 nil。
	tags, err := parseSectionTags(validSectionResponse)

	require.NoError(t, err)
	require.Len(t, tags, 3)
	require.Nil(t, tags[0].Fingerprint)
}

func TestExtractTagsFromSectionSingleCallWithFingerprint(t *testing.T) {
	router := newFakeTagChatRouter()
	router.enqueue("tag_extraction_section", fakeTagChatResponse{content: sectionFingerprintResponse})
	extractor := &TagExtractor{router: router}

	tags, err := extractor.extractTagsFromSection(context.Background(), router, ExtractionInput{
		Title: "国际政治周刊", Summary: "unused", FeedName: "政治周刊", CategoryName: "政治",
	}, "政治动态", "特朗普宣布参加总统选举，李飞飞谈空间智能与 PostgreSQL 的结合。", 0)

	require.NoError(t, err)
	require.Len(t, tags, 3)
	require.NotNil(t, tags[0].Fingerprint)
	require.True(t, tags[0].Fingerprint.Complete())
	// 指纹与标签同一次调用产出：调用次数仍为 1。
	require.Equal(t, 1, router.callCount("tag_extraction_section"))
}

func TestSectionExtractionSchemaAndPromptCarryFingerprintFields(t *testing.T) {
	schema := sectionExtractionSchema()
	tags := schema.Properties["tags"]
	require.NotNil(t, tags.Items)
	for _, prop := range []string{"subject", "subject_key", "action", "object", "occurred_at"} {
		require.Contains(t, tags.Items.Properties, prop)
		require.Equal(t, "string", tags.Items.Properties[prop].Type)
	}
	// 指纹四项不设 required：keyword/person 与旧模型不应被 schema 拒绝。
	require.ElementsMatch(t, []string{"label", "category"}, tags.Items.Required)

	prompt := buildSectionExtractionPrompt()
	require.Contains(t, prompt, "事件指纹")
	require.Contains(t, prompt, "subject_key")
	require.Contains(t, prompt, "person:trump")
	require.Contains(t, prompt, "company:bytedance")
	require.Contains(t, prompt, "person/keyword 不输出这五项")
}

// ---- normalize-event-subject-entities：影子态第一轮（D2/D10）----

// shadowSectionResponse is validSectionResponse plus the shadow-only
// subject_desc field on the event tag (person/keyword stay without one).
const shadowSectionResponse = `{"tags":[` +
	`{"label":"伊朗袭击以色列","category":"event","aliases":["伊以冲突"],"description":"伊朗对以色列发动军事打击的新闻事件","auxiliary_labels":[{"label":"伊朗","description":"中东地区国家"},{"label":"以色列","description":"中东国家"},{"label":"导弹袭击","description":"以导弹进行军事打击的行动"}],"subject":"伊朗","subject_key":"org:iran","action":"袭击","object":"以色列军事目标","subject_desc":"中东地区国家"},` +
	`{"label":"Sam Altman","category":"person","description":"","auxiliary_labels":[{"label":"OpenAI","description":"人工智能研究公司"},{"label":"ChatGPT","description":"对话式AI产品"},{"label":"GPT-5","description":"大语言模型版本"}]},` +
	`{"label":"Kubernetes","category":"keyword","aliases":["K8s"],"description":"开源容器编排系统"}]}`

// 场景：关闭态第一轮提示与字段输出不变（aggregate-tagging ADDED）。
// WHEN 系统以关闭态（默认）运行提取：第一轮提示与 schema 与上线前逐字一致，
// 不因本能力新增字段输出或模型请求。
func TestSectionExtraction_OffPromptAndSchemaUnchanged(t *testing.T) {
	SetEntitySubjectModeForTest("off")
	t.Cleanup(func() { SetEntitySubjectModeForTest("") })

	require.False(t, EntitySubjectShadowEnabled())

	// 冻结 golden：off 态 prompt 的首尾锚点与关键行（与上线前逐字一致的
	// 回归守卫——任何人改 buildSectionExtractionPrompt 都会红）。
	//
	// ⚠️ 跨 change 耦合（review R-21）：下面两行锚点
	// 「事件指纹（仅 event 标签输出，person/keyword 不输出这五项）」与
	// `subject_key：主体归一化实体键，格式"类型:规范名"` 由并发 change
	// `add-event-story-heat` 引入（不是本 change 的产物）。该 change 若微调
	// 这两行的措辞，本 golden 会红——那是跨 change 同步，不是本 change 回归；
	// 改措辞时须同步更新本 golden（design Risks 已声明的已知代价）。
	prompt := buildSectionExtractionPrompt()
	require.True(t, strings.HasPrefix(prompt, "你是一个专业的新闻分析助手，负责从聚合型文章（如科技周刊、排行榜合集）的一个栏目片中提取 event、person、keyword 三类标签。"))
	require.Contains(t, prompt, "事件指纹（仅 event 标签输出，person/keyword 不输出这五项）")
	require.Contains(t, prompt, `subject_key：主体归一化实体键，格式"类型:规范名"`)
	require.True(t, strings.HasSuffix(prompt, `{"label":"Kubernetes","category":"keyword","aliases":["K8s"],"description":"开源容器编排系统"}]}`))
	require.NotContains(t, prompt, "subject_desc", "off 态提示不得出现影子字段")
	require.NotContains(t, prompt, "影子模式", "off 态提示不得出现影子段")

	// schema 字段集冻结：五指纹字段在、subject_desc 不在、Required 不变。
	schema := sectionExtractionSchema()
	props := schema.Properties["tags"].Items.Properties
	require.ElementsMatch(t,
		[]string{"label", "category", "aliases", "description", "auxiliary_labels", "subject", "subject_key", "action", "object", "occurred_at"},
		mapKeys(props))
	require.NotContains(t, props, "subject_desc")
	require.Equal(t, []string{"label", "category"}, schema.Properties["tags"].Items.Required)

	// EventFingerprintSchema 是五字段真相源：不因影子态扩容。
	require.ElementsMatch(t,
		[]string{"subject", "subject_key", "action", "object", "occurred_at"},
		mapKeys(EventFingerprintSchema().Properties))

	// 行为守卫：off 态发出的请求 prompt/schema 与 golden 相同、调用数为 1。
	router := newRecordingTagChatRouter()
	router.content = validSectionResponse
	extractor := &TagExtractor{router: router}
	_, err := extractor.extractTagsFromSection(context.Background(), router, ExtractionInput{Title: "标题"}, "栏目", "正文提及 Sam Altman 与 Kubernetes", 0)
	require.NoError(t, err)
	require.Len(t, router.requests, 1)
	require.Equal(t, prompt, router.requests[0].Messages[0].Content, "off 态 system prompt 与 golden 逐字一致")
	_, hasDesc := router.requests[0].JSONSchema.Properties["tags"].Items.Properties["subject_desc"]
	require.False(t, hasDesc, "off 态 schema 不含 subject_desc")
}

// 场景：影子态同一次调用产出主体原文与稳定短身份说明（aggregate-tagging ADDED）。
// WHEN 影子模式下某栏目片返回 3 个标签：各 event 标签带主体原文与短说明，
// 该片调用次数仍为 1；event_fingerprint 结构与校验结论不变；keyword 与
// person 不携带；短说明为空照常入库。
func TestSectionExtraction_ShadowAddsSubjectDescSameCall(t *testing.T) {
	SetEntitySubjectModeForTest("shadow")
	t.Cleanup(func() { SetEntitySubjectModeForTest("") })
	require.True(t, EntitySubjectShadowEnabled())

	router := newRecordingTagChatRouter()
	router.content = shadowSectionResponse
	extractor := &TagExtractor{router: router}

	tags, err := extractor.extractTagsFromSection(context.Background(), router, ExtractionInput{Title: "标题"}, "栏目", "正文提及 Sam Altman 与 Kubernetes", 0)
	require.NoError(t, err)
	require.Len(t, router.requests, 1, "该片模型调用次数仍为 1（零新增调用）")

	req := router.requests[0]
	// 影子 prompt = 原 prompt 逐字前缀 + 追加段（off 态产物不被改写）。
	require.Contains(t, req.Messages[0].Content, buildSectionExtractionPrompt(), "影子 prompt 以原 prompt 为前缀")
	require.Contains(t, req.Messages[0].Content, "subject_desc")
	require.Contains(t, req.Messages[0].Content, "不得包含当次事件、动作、对象或时间")
	// 影子 schema = 原 schema + subject_desc 属性（Required 不变）。
	_, hasDesc := req.JSONSchema.Properties["tags"].Items.Properties["subject_desc"]
	require.True(t, hasDesc)
	require.Equal(t, []string{"label", "category"}, req.JSONSchema.Properties["tags"].Items.Required)

	// 解析：event 带 SubjectDesc；person/keyword 不带；指纹结构不变。
	require.Len(t, tags, 3)
	var event, person, keyword *ExtractedTag
	for i := range tags {
		switch tags[i].Category {
		case "event":
			event = &tags[i]
		case "person":
			person = &tags[i]
		case "keyword":
			keyword = &tags[i]
		}
	}
	require.NotNil(t, event)
	require.Equal(t, "中东地区国家", event.SubjectDesc)
	require.NotNil(t, event.Fingerprint)
	require.Equal(t, "org:iran", event.Fingerprint.SubjectKey)
	require.True(t, event.Fingerprint.Complete())
	require.NotNil(t, person)
	require.Empty(t, person.SubjectDesc, "person 不携带短说明")
	require.Nil(t, person.Fingerprint)
	require.NotNil(t, keyword)
	require.Empty(t, keyword.SubjectDesc, "keyword 不携带短说明")

	// 短说明缺失的 event 照常产出（不报错、不丢弃）。
	noDesc := `{"tags":[{"label":"某公司发布新品","category":"event","description":"某公司发布新产品","auxiliary_labels":[{"label":"某公司","description":"企业"},{"label":"新品","description":"新产品"}],"subject":"某公司","subject_key":"company:x","action":"发布","object":"新品"}]}`
	parsed, err := parseSectionTags(noDesc)
	require.NoError(t, err)
	require.Len(t, parsed, 1)
	require.Empty(t, parsed[0].SubjectDesc, "缺说明 → 空，不编造")
	require.NotNil(t, parsed[0].Fingerprint)

	// off 响应（无 subject_desc 字段）在影子解析器下也安全：字段缺省为空。
	parsedOff, err := parseSectionTags(validSectionResponse)
	require.NoError(t, err)
	for _, tg := range parsedOff {
		require.Empty(t, tg.SubjectDesc)
	}
}

func mapKeys(m map[string]airouter.SchemaProperty) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}
