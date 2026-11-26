package core

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// 防漂移（spec Scenario: 示例词集合与 prompt 防漂移）：集合每个词必须原文
// 出现在对应 prompt 文本中。prompt 改示例词忘同步集合（或反之）时此测试红。
func TestExemplarLabelsCoverPrompts(t *testing.T) {
	merged := buildMergedExtractionPrompt()
	for _, label := range mergedPromptExemplarLabels {
		require.Contains(t, merged, label, "mergedPromptExemplarLabels 词 %q 不在 buildMergedExtractionPrompt 文本中——两侧必须同步维护", label)
	}

	section := buildSectionExtractionPrompt()
	for _, label := range sectionPromptExemplarLabels {
		require.Contains(t, section, label, "sectionPromptExemplarLabels 词 %q 不在 buildSectionExtractionPrompt 文本中——两侧必须同步维护", label)
	}
}

// spec Scenario: 无提及文章的示例词 keyword 被丢弃。
func TestGuardExemplarLeakageDropsWithoutEvidence(t *testing.T) {
	tags := []ExtractedTag{
		{Label: "Qwen3-Coder 模型区别对待用户事件", Category: "event"},
		{Label: "PostgreSQL", Category: "keyword", Description: "开源关系型数据库"},
	}
	kept, dropped := guardExemplarLeakage(tags, "科技周刊第416期：编程已经面目全非。", mergedExemplarSlugs)

	require.Len(t, kept, 1)
	require.Equal(t, "Qwen3-Coder 模型区别对待用户事件", kept[0].Label)
	require.Equal(t, []string{"PostgreSQL"}, dropped)
}

// spec Scenario: 可见输入确有提及的示例词保留（大小写不敏感证据）。
func TestGuardExemplarLeakageKeepsWithEvidence(t *testing.T) {
	tags := []ExtractedTag{{Label: "PostgreSQL", Category: "keyword"}}

	kept, dropped := guardExemplarLeakage(tags, "PostgreSQL update: new version", mergedExemplarSlugs)
	require.Len(t, kept, 1)
	require.Empty(t, dropped)

	// 小写证据同样放行
	kept, dropped = guardExemplarLeakage(tags, "升级 postgresql 到 17", mergedExemplarSlugs)
	require.Len(t, kept, 1)
	require.Empty(t, dropped)
}

// spec Scenario: 非示例词候选不受影响（无提及也不拦）。
func TestGuardExemplarLeakageKeepsNonExemplars(t *testing.T) {
	tags := []ExtractedTag{{Label: "LangChain", Category: "keyword"}}

	kept, dropped := guardExemplarLeakage(tags, "一篇完全无关的文章正文", mergedExemplarSlugs)
	require.Len(t, kept, 1)
	require.Empty(t, dropped)
}

// spec Scenario: auxiliary_labels 泄漏项被过滤（主标签保留、无依据 aux 项剔除）。
func TestGuardExemplarLeakageFiltersAuxiliaryLabels(t *testing.T) {
	tags := []ExtractedTag{{
		Label:    "科技周刊推荐开源工具与资源事件",
		Category: "event",
		AuxiliaryLabels: []AuxiliaryLabel{
			{Label: "sslx", Description: "命令行工具"},
			{Label: "PostgreSQL", Description: "开源关系型数据库"},
			{Label: "Vim", Description: "开源编辑器"},
		},
	}}

	kept, dropped := guardExemplarLeakage(tags, "推荐 sslx、Vim 教程及编译原理教材", sectionExemplarSlugs)
	require.Len(t, kept, 1)
	require.Empty(t, dropped, "主标签非示例词，不丢弃")
	require.Len(t, kept[0].AuxiliaryLabels, 2)
	require.Equal(t, "sslx", kept[0].AuxiliaryLabels[0].Label)
	require.Equal(t, "Vim", kept[0].AuxiliaryLabels[1].Label)
}

// spec Scenario: 大小写变体同样拦截（Slugify 归一后命中）。
func TestGuardExemplarLeakageCatchesCaseVariants(t *testing.T) {
	tags := []ExtractedTag{{Label: "POSTGRESQL", Category: "keyword"}}

	kept, dropped := guardExemplarLeakage(tags, "正文里没有任何数据库字样", mergedExemplarSlugs)
	require.Empty(t, kept)
	require.Equal(t, []string{"POSTGRESQL"}, dropped)
}

// 中文示例词同样受护栏管辖：正文有提及放行、无提及丢弃。
func TestGuardExemplarLeakageChineseExemplars(t *testing.T) {
	tags := []ExtractedTag{{Label: "央行", Category: "keyword"}}

	kept, dropped := guardExemplarLeakage(tags, "某国央行上调基准利率", mergedExemplarSlugs)
	require.Len(t, kept, 1)
	require.Empty(t, dropped)

	kept, dropped = guardExemplarLeakage(tags, "一篇与货币政策无关的正文", mergedExemplarSlugs)
	require.Empty(t, kept)
	require.Equal(t, []string{"央行"}, dropped)
}

// 主标签被丢弃时其 auxiliary_labels 一并丢弃。
func TestGuardExemplarLeakageDropsAuxWithMainTag(t *testing.T) {
	tags := []ExtractedTag{{
		Label:           "PostgreSQL",
		Category:        "keyword",
		AuxiliaryLabels: []AuxiliaryLabel{{Label: "开源数据库", Description: "x"}},
	}}

	kept, dropped := guardExemplarLeakage(tags, "无关正文", mergedExemplarSlugs)
	require.Empty(t, kept)
	require.Equal(t, []string{"PostgreSQL"}, dropped)
}

// 空切片与全部丢弃的边界。
func TestGuardExemplarLeakageEmptyAndAllDropped(t *testing.T) {
	kept, dropped := guardExemplarLeakage(nil, "正文", mergedExemplarSlugs)
	require.Nil(t, kept)
	require.Nil(t, dropped)

	tags := []ExtractedTag{{Label: "PostgreSQL"}, {Label: "Kubernetes"}}
	kept, dropped = guardExemplarLeakage(tags, "无关正文", sectionExemplarSlugs)
	require.Empty(t, kept)
	require.Len(t, dropped, 2)
}

// mono 路径行为（spec Scenario 1 落点）：LLM 返回无依据 PostgreSQL，ExtractTags
// 结果不含它，且丢弃记入 Errors 观测尾注（2026-10-23 周刊 416 期复现场景）。
func TestExtractTagsDropsExemplarLeakageViaMonoPath(t *testing.T) {
	router := newFakeTagChatRouter()
	router.enqueue("tag_extraction_merged", fakeTagChatResponse{content: `{"event_person_tags":[{"label":"OpenAI发布GPT-5","category":"event","auxiliary_labels":[{"label":"OpenAI","description":"人工智能研究公司"},{"label":"GPT-5","description":"大语言模型版本"}]}],"keyword_tags":[{"label":"PostgreSQL","category":"keyword","description":"开源关系型数据库"}]}`})
	extractor := &TagExtractor{router: router}

	result, err := extractor.ExtractTags(context.Background(), ExtractionInput{
		Title:   "科技爱好者周刊（第 416 期）：编程已经面目全非",
		Summary: "AI 推翻了传统的软件开发，编程已经面目全非。Token 为王。",
	})

	require.NoError(t, err)
	require.Len(t, result.Tags, 1)
	require.Equal(t, "OpenAI发布GPT-5", result.Tags[0].Label)
	require.Contains(t, strings.Join(result.Errors, "\n"), `exemplar leakage guard dropped "PostgreSQL"`)
}

// section 路径行为（spec Scenario: section 路径同样生效）：栏目片无 Kubernetes
// 提及时，section 提取返回的 Kubernetes 候选被丢弃。
func TestExtractTagsFromSectionDropsExemplarLeakage(t *testing.T) {
	router := newFakeTagChatRouter()
	router.enqueue("tag_extraction_section", fakeTagChatResponse{content: `{"tags":[{"label":"Rust语言","category":"keyword","description":"系统级编程语言"},{"label":"Kubernetes","category":"keyword","description":"开源容器编排系统"}]}`})
	extractor := &TagExtractor{router: router}

	tags, err := extractor.extractTagsFromSection(context.Background(), router,
		ExtractionInput{Title: "科技周刊第100期"}, "言论", "开发者讨论提示词缓存。", 2)

	require.NoError(t, err)
	require.Len(t, tags, 1)
	require.Equal(t, "Rust语言", tags[0].Label)
}
