package core

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// fix-tagging-pollution spec「快讯类文章 keyword 为空时保持 LLM 原样」：
// LLM 返回 1 个 event + 空 keyword 数组 → 结果仅含该 event，
// 不得注入分类名/规则词候选（旧契约 :59-61 规则词回填）。
func TestExtractTagsKeepsLLMResultWhenKeywordArrayEmpty(t *testing.T) {
	router := newFakeTagChatRouter()
	router.enqueue("tag_extraction_merged", fakeTagChatResponse{
		content: `{"event_person_tags":[{"label":"美联储宣布维持利率不变","category":"event","description":"美联储议息会议后的利率决议","auxiliary_labels":[{"label":"美联储","description":"美国中央银行"},{"label":"议息会议","description":"利率决议会议"},{"label":"利率决议","description":"货币政策决定"}]}],"keyword_tags":[]}`,
	})
	extractor := &TagExtractor{router: router}

	// CategoryName「新闻」是旧回填污染的典型来源（华尔街见闻/凤凰网分类名）。
	result, err := extractor.ExtractTags(context.Background(), ExtractionInput{
		Title:        "美联储维持利率不变",
		Summary:      "美联储在最新议息会议后宣布维持联邦基金利率不变，符合市场预期。",
		FeedName:     "华尔街见闻",
		CategoryName: "新闻",
	})

	require.NoError(t, err)
	require.Len(t, result.Tags, 1, "empty keyword array is a legal outcome: result keeps the event tag only")
	require.Equal(t, "美联储宣布维持利率不变", result.Tags[0].Label)
	require.NotContains(t, topicLabels(result.Tags), "新闻", "category-name backfill must not appear")
	require.Equal(t, 1, router.callCount("tag_extraction_merged"))
}

// fix-tagging-pollution spec「空摘要文章两个数组均为空」：两数组均空 → 原样返回
// 零候选（Tags=[]、Errors 照记），不触发任何规则词降级。旧契约 :67
// 在 candidates==0 时调规则词兜底返回非空结果（err=nil），
// 使 tagger 层兜底收窄形同虚设——本用例守住该分支。
func TestExtractTagsReturnsZeroCandidatesWhenBothArraysEmpty(t *testing.T) {
	router := newFakeTagChatRouter()
	router.enqueue("tag_extraction_merged", fakeTagChatResponse{
		content: `{"event_person_tags":[],"keyword_tags":[]}`,
	})
	extractor := &TagExtractor{router: router}

	result, err := extractor.ExtractTags(context.Background(), ExtractionInput{
		Title:        "快讯",
		Summary:      " ", // 纯空白摘要：LLM 判空是「宁缺毋滥」下的合法结论
		FeedName:     "华尔街见闻",
		CategoryName: "新闻",
	})

	require.NoError(t, err)
	require.Empty(t, result.Tags, "zero candidates must stay a legal zero result, not degrade to rule-word tags")
	require.NotEmpty(t, result.Errors, "branch notes for both empty arrays must still be recorded")
	require.Equal(t, 1, router.callCount("tag_extraction_merged"))
}

// 指纹已摘除（2026-11-12）：merged 响应残留的指纹字段一律被解析层忽略。
func TestParseMergedExtractionTagsIgnoresFingerprint(t *testing.T) {
	content := `{"event_person_tags":[` +
		`{"label":"字节跳动发布AI视频模型","category":"event","description":"字节跳动发布AI视频模型的产品事件","auxiliary_labels":[{"label":"字节跳动","description":"中国科技公司"},{"label":"AI视频模型","description":"生成视频的AI模型"},{"label":"模型发布","description":"产品发布行为"}],"subject":"字节跳动","subject_key":"Company:Byte-Dance","action":"发布","object":"AI视频模型","occurred_at":"2026-11-02"},` +
		`{"label":"Sam Altman","category":"person","description":"","auxiliary_labels":[{"label":"OpenAI","description":"人工智能研究公司"},{"label":"ChatGPT","description":"对话式AI产品"},{"label":"GPT-5","description":"大语言模型版本"}],"subject":"Sam Altman","subject_key":"person:sam_altman","action":"执掌","object":"OpenAI"}],` +
		`"keyword_tags":[{"label":"PostgreSQL","category":"keyword","description":"开源关系型数据库","subject":"PostgreSQL","subject_key":"product:postgresql","action":"发布","object":"17版本"}]}`

	eventPersonTags, keywordTags, err := parseMergedExtractionTags(content)

	require.NoError(t, err)
	require.Len(t, eventPersonTags, 2)
	require.Len(t, keywordTags, 1)

	for i := range eventPersonTags {
		require.Nil(t, eventPersonTags[i].Fingerprint, "残留指纹字段被忽略")
	}
	require.Nil(t, keywordTags[0].Fingerprint)
}

// merged 路径：指纹不完整（缺 object）时标签保留、指纹不可信但不丢弃；
// 指纹经 resolveCandidate 拷贝到 TopicTag，走同一次调用、零额外请求。
func TestExtractTagsCarriesEventFingerprintToTopicTag(t *testing.T) {
	router := newFakeTagChatRouter()
	router.enqueue("tag_extraction_merged", fakeTagChatResponse{
		content: `{"event_person_tags":[{"label":"美联储加息25个基点","category":"event","description":"美联储上调基准利率的货币政策事件","auxiliary_labels":[{"label":"美联储","description":"美国中央银行"},{"label":"议息会议","description":"利率决议会议"},{"label":"利率决议","description":"货币政策决定"}],"subject":"美联储","subject_key":"org:federal_reserve","action":"上调","object":"基准利率"}],"keyword_tags":[{"label":"货币政策","category":"keyword","description":"央行调节货币供给的政策工具"}]}`,
	})
	extractor := &TagExtractor{router: router}

	result, err := extractor.ExtractTags(context.Background(), ExtractionInput{
		Title:        "某国央行利率决议",
		Summary:      "某国央行在议息会议后宣布上调基准利率。",
		FeedName:     "财经快讯",
		CategoryName: "财经",
	})

	require.NoError(t, err)
	require.Len(t, result.Tags, 2)

	var eventTag, keywordTag *TopicTag
	for i := range result.Tags {
		switch result.Tags[i].Category {
		case "event":
			eventTag = &result.Tags[i]
		case "keyword":
			keywordTag = &result.Tags[i]
		}
	}
	require.NotNil(t, eventTag, "event tag must survive to TopicTag")
	// 指纹已摘除（2026-11-12）：响应残留的指纹字段被解析层忽略，恒为 nil。
	require.Nil(t, eventTag.Fingerprint)
	require.NotNil(t, keywordTag)
	require.Nil(t, keywordTag.Fingerprint, "keyword tags never carry a fingerprint")
	require.Equal(t, 1, router.callCount("tag_extraction_merged"))
}

// 指纹五字段已于 2026-11-12 摘除（4B 模型融合任务过重致 parse_error 潮）：
// schema 与 prompt 均不得再出现指纹字段/指令。
func TestMergedExtractionSchemaAndPromptFingerprintRemoved(t *testing.T) {
	schema := mergedTagExtractionSchema()
	eventPerson := schema.Properties["event_person_tags"]
	require.NotNil(t, eventPerson.Items)
	for _, prop := range []string{"subject", "subject_key", "action", "object", "occurred_at"} {
		require.NotContains(t, eventPerson.Items.Properties, prop)
	}
	require.ElementsMatch(t, []string{"label", "category", "auxiliary_labels"}, eventPerson.Items.Required)

	prompt := buildMergedExtractionPrompt()
	require.NotContains(t, prompt, "事件指纹")
	require.NotContains(t, prompt, "subject_key")
}

// mergedOffResponse — off 态的 merged 响应（无 subject_desc 字段）。
const mergedOffResponse = `{"event_person_tags":[` +
	`{"label":"美联储维持利率不变","category":"event","description":"美联储议息会议后的利率决议","auxiliary_labels":[{"label":"美联储","description":"美国中央银行"},{"label":"议息会议","description":"利率决议会议"},{"label":"利率决议","description":"货币政策决定"}],"subject":"美联储","subject_key":"org:federal_reserve","action":"维持","object":"利率"},` +
	`{"label":"Sam Altman","category":"person","description":"","auxiliary_labels":[{"label":"OpenAI","description":"人工智能研究公司"},{"label":"ChatGPT","description":"对话式AI产品"},{"label":"GPT-5","description":"大语言模型版本"}]}],` +
	`"keyword_tags":[{"label":"PostgreSQL","category":"keyword","description":"开源关系型数据库"}]}`

// mergedShadowResponse — mergedOffResponse 的 event 标签多一个 subject_desc
// （person/keyword 不带）。
const mergedShadowResponse = `{"event_person_tags":[` +
	`{"label":"美联储维持利率不变","category":"event","description":"美联储议息会议后的利率决议","auxiliary_labels":[{"label":"美联储","description":"美国中央银行"},{"label":"议息会议","description":"利率决议会议"},{"label":"利率决议","description":"货币政策决定"}],"subject":"美联储","subject_key":"org:federal_reserve","action":"维持","object":"利率","subject_desc":"美国中央银行"},` +
	`{"label":"Sam Altman","category":"person","description":"","auxiliary_labels":[{"label":"OpenAI","description":"人工智能研究公司"},{"label":"ChatGPT","description":"对话式AI产品"},{"label":"GPT-5","description":"大语言模型版本"}]}],` +
	`"keyword_tags":[{"label":"PostgreSQL","category":"keyword","description":"开源关系型数据库"}]}`

// 场景：关闭态第一轮提示与字段输出不变（aggregate-tagging ADDED），merged
// 路径（review R-20：section 路径有双层 golden，merged 路径平行接线此前零测试）。
// WHEN 系统以关闭态（默认）跑 merged 提取：提示与 schema 与上线前逐字一致、
// 调用数为 1、schema 不含 subject_desc。
func TestEnhancedExtraction_OffPromptAndSchemaUnchanged(t *testing.T) {
	SetEntitySubjectModeForTest("off")
	t.Cleanup(func() { SetEntitySubjectModeForTest("") })
	require.False(t, EntitySubjectShadowEnabled())

	// 冻结 golden：off 态 merged prompt 的首尾锚点与关键行。基线以本次
	// （retire-person-extraction 收敛 person 之后）为起点；任何人改
	// buildMergedExtractionPrompt 都会红，措辞变更须同步本 golden。其中
	// 「【事件指纹】…」与 subject_key 行由并发 change add-event-story-heat 引入，
	// 措辞变更需同步本 golden（不是本 change 回归）。
	prompt := buildMergedExtractionPrompt()
	require.True(t, strings.HasPrefix(prompt, "你是新闻分析助手，一次性从新闻摘要提取 event 与 keyword 标签，同时输出两个数组。"),
		"retire-person-extraction 后 prompt 首句只提 event/keyword")
	require.NotContains(t, prompt, "【事件指纹】", "指纹指令已摘除")
	require.NotContains(t, prompt, "subject_key", "指纹字段指令已摘除")
	require.True(t, strings.HasSuffix(prompt, `{"label":"技术","category":"keyword","description":"技术"}（标签与描述都过于泛化）`))
	require.NotContains(t, prompt, "subject_desc", "off 态提示不得出现影子字段")
	require.NotContains(t, prompt, "影子模式", "off 态提示不得出现影子段")

	// schema 字段集冻结：五指纹字段在、subject_desc 不在、Required 不变。
	schema := mergedTagExtractionSchema()
	items := schema.Properties["event_person_tags"].Items
	require.ElementsMatch(t,
		[]string{"label", "category", "aliases", "description", "auxiliary_labels"},
		mapKeys(items.Properties))
	require.NotContains(t, items.Properties, "subject_desc")
	require.Equal(t, []string{"label", "category", "auxiliary_labels"}, items.Required)
	require.ElementsMatch(t, []string{"event_person_tags", "keyword_tags"}, schema.Required)

	// 行为守卫：off 态发出的请求 prompt/schema 与 golden 相同、调用数为 1。
	router := newRecordingTagChatRouter()
	router.content = mergedOffResponse
	extractor := &TagExtractor{router: router}
	eventPerson, keyword, err := extractor.extractMergedCandidates(context.Background(), ExtractionInput{Title: "标题"})
	require.NoError(t, err)
	require.Len(t, router.requests, 1, "merged 提取一次调用")
	require.Equal(t, prompt, router.requests[0].Messages[0].Content, "off 态 system prompt 与 golden 逐字一致")
	_, hasDesc := router.requests[0].JSONSchema.Properties["event_person_tags"].Items.Properties["subject_desc"]
	require.False(t, hasDesc, "off 态 schema 不含 subject_desc")

	// off 响应无 subject_desc：解析后为空（不编造），指纹照常。
	require.Len(t, eventPerson, 2)
	require.Len(t, keyword, 1)
	var event *ExtractedTag
	for i := range eventPerson {
		if eventPerson[i].Category == "event" {
			event = &eventPerson[i]
		}
	}
	require.NotNil(t, event)
	require.Empty(t, event.SubjectDesc, "off 态不产短身份说明")
	require.Nil(t, event.Fingerprint, "指纹已摘除")
}

// 场景：影子态同一次调用产出主体原文与稳定短身份说明（aggregate-tagging
// ADDED），merged 路径（review R-20）。
// WHEN 影子模式跑 merged 提取：system prompt = 原 prompt 逐字前缀 + 追加段；
// schema = 原 schema + subject_desc（Required 不变）；同一次调用即产出
// subject_desc，调用数仍为 1。
func TestEnhancedExtraction_ShadowAddsSubjectDescSameCall(t *testing.T) {
	SetEntitySubjectModeForTest("shadow")
	t.Cleanup(func() { SetEntitySubjectModeForTest("") })
	require.True(t, EntitySubjectShadowEnabled())

	router := newRecordingTagChatRouter()
	router.content = mergedShadowResponse
	extractor := &TagExtractor{router: router}

	eventPerson, keyword, err := extractor.extractMergedCandidates(context.Background(), ExtractionInput{Title: "标题"})
	require.NoError(t, err)
	require.Len(t, router.requests, 1, "同一次调用产出 subject_desc：调用数仍为 1（零新增调用）")

	req := router.requests[0]
	// 影子 prompt = 原 prompt 逐字前缀 + 追加段（off 态产物不被改写）。
	require.True(t, strings.HasPrefix(req.Messages[0].Content, buildMergedExtractionPrompt()), "影子 prompt 以原 prompt 逐字前缀开头")
	require.Contains(t, req.Messages[0].Content, "subject_desc")
	require.Contains(t, req.Messages[0].Content, "不得包含当次事件、动作、对象或时间")
	// 影子 schema = 原 schema + subject_desc 属性（Required 不变）。
	items := req.JSONSchema.Properties["event_person_tags"].Items
	_, hasDesc := items.Properties["subject_desc"]
	require.True(t, hasDesc)
	require.Equal(t, []string{"label", "category", "auxiliary_labels"}, items.Required)

	// 解析：event 带 SubjectDesc；person/keyword 不带；指纹结构不变。
	require.Len(t, eventPerson, 2)
	require.Len(t, keyword, 1)
	var event, person *ExtractedTag
	for i := range eventPerson {
		switch eventPerson[i].Category {
		case "event":
			event = &eventPerson[i]
		case "person":
			person = &eventPerson[i]
		}
	}
	require.NotNil(t, event)
	require.Equal(t, "美国中央银行", event.SubjectDesc)
	require.Nil(t, event.Fingerprint, "指纹已摘除")
	require.NotNil(t, person)
	require.Empty(t, person.SubjectDesc, "person 不携带短说明")
	require.Nil(t, person.Fingerprint)
	require.Empty(t, keyword[0].SubjectDesc, "keyword 不携带短说明")

	// 短说明缺失的 event 照常产出（不编造）。
	eventPersonTags, _, err := parseMergedExtractionTags(mergedOffResponse)
	require.NoError(t, err)
	require.Len(t, eventPersonTags, 2)
	require.Empty(t, eventPersonTags[0].SubjectDesc)
	require.Nil(t, eventPersonTags[0].Fingerprint, "指纹已摘除，残留字段被忽略")
}

// TestBuildExtractionUserPromptFormatMatchesSchema 守护：user prompt 末句的输出
// 格式指令必须与下发模型的 JSON Schema 一致（merged 路径是双数组）。
// 旧写法 `{"tags": [标签列表]}` 是 merge-tag-extraction-branches 之前的单数组形态，
// 与双数组 schema 矛盾，会让小模型吐出「schema 骨架 + 值塞 properties」的混合体。
func TestBuildExtractionUserPromptFormatMatchesSchema(t *testing.T) {
	prompt := buildExtractionUserPrompt(ExtractionInput{
		Title:   "某国央行上调基准利率",
		Summary: "正文摘要",
	})

	require.NotContains(t, prompt, `{"tags": [标签列表]}`, "prompt 不得再指向已废弃的单数组写法")
	require.Contains(t, prompt, "event_person_tags")
	require.Contains(t, prompt, "keyword_tags")
}

// TestMergedExtractionPromptExcludesPerson 守护 retire-person-extraction：
// 合并提取 prompt 不再要求模型提取 person 类别（辅助标签体系已覆盖人物叫法，
// 双类别同一数组会挤压注意力导致 event_person_tags 结构错误）。字段名
// event_person_tags 故意保留，因此断言只看「提取要求」措辞。
func TestMergedExtractionPromptExcludesPerson(t *testing.T) {
	prompt := buildMergedExtractionPrompt()

	for _, banned := range []string{
		"person（人物）",         // 类别定义行
		"person 可留空",         // event 描述规则里的尾巴
		"每个 event/person 标签", // 辅助标签要求
		"event/person 数组只放",  // 共同规则串放约束
	} {
		require.NotContainsf(t, prompt, banned, "prompt 不得再包含人物提取要求：%q", banned)
	}

	// 保留项：event / keyword 规则与既有 JSON 字段名（契约不变）。
	require.Contains(t, prompt, "event（事件）")
	require.Contains(t, prompt, "keyword（关键词）")
	require.Contains(t, prompt, "event_person_tags")

	// 与措辞无关的强断言（review R-5）：剔除字段名后整篇 prompt 不得再出现
	// "person"——新增任何 person 措辞（含示例词、说明句）都会红。
	require.NotContains(t, strings.ReplaceAll(prompt, "event_person_tags", ""), "person")
}
