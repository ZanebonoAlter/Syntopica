package core

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// 泛化主体禁则（generic subject ban）——提取侧与解析侧同一套语义：
// 模型编的泛称/角色/载体（文章、用户、作者、开发者…）不是事件身份，
// 提示必须先禁掉，解析侧再兜底拒判（topicgraph/service/story_fingerprint.go）。

// 场景：栏目片提取提示必须明令 subject 只能是具体专有名词，并给出泛称黑名单。
// WHEN 构建栏目片提取 system prompt：事件指纹段带主体禁则（具体专有名词 +
// 泛称黑名单 + 找不到主体就不输出五项）。
func TestSectionExtractionPromptBansGenericSubject(t *testing.T) {
	prompt := buildSectionExtractionPrompt()

	require.Contains(t, prompt, "subject 必须是具体专有名词", "必须要求主体是具体专有名词")
	require.Contains(t, prompt, "禁止把泛称/角色/载体当主体", "必须给出泛称禁则")
	for _, banned := range []string{"文章", "用户", "作者", "开发者", "记者", "编辑", "网友", "媒体", "博主", "专家", "分析师", "业内人士", "消息人士", "团队", "公司", "机构", "组织", "平台", "市场", "行业", "产品", "技术", "系统", "应用", "工具", "服务", "内容", "信息", "新闻", "报告", "公告", "数据", "事件", "项目"} {
		require.Contains(t, prompt, banned, "泛称黑名单必须含 %q", banned)
	}
	require.Contains(t, prompt, "找不到具体主体时：不要输出这五项", "找不到具体主体时不得输出五项指纹")
	// 事件标签本身照常入库——禁则只约束指纹，不约束标签产出。
	require.Contains(t, prompt, "该 event 标签仍正常入库")
}

// merged 路径（单篇摘要提取）同步同一规则，措辞可略简。
func TestMergedExtractionPromptBansGenericSubject(t *testing.T) {
	prompt := buildMergedExtractionPrompt()

	require.Contains(t, prompt, "具体专有名词", "必须要求主体是具体专有名词")
	require.Contains(t, prompt, "禁止把泛称/角色/载体当主体", "必须给出泛称禁则")
	require.Contains(t, prompt, "找不到具体主体就不输出这五项", "找不到具体主体时不得输出五项指纹")
	for _, banned := range []string{"用户", "作者", "开发者", "文章", "公司", "平台"} {
		require.Contains(t, prompt, banned, "泛称示例必须含 %q", banned)
	}
}

// schema 描述是模型侧的字段级契约：subject/subject_key 必须写明是专有名词。
func TestEventFingerprintSchemaDescriptionsBanGenericSubject(t *testing.T) {
	props := EventFingerprintSchema().Properties
	require.Contains(t, props["subject"].Description, "必须是具体专有名词")
	require.Contains(t, props["subject"].Description, "泛称一律不得作为主体")
	require.Contains(t, props["subject_key"].Description, "必须是具体专有名词")
	require.Contains(t, props["subject_key"].Description, "泛称一律不得作为主体")

	merged := mergedTagExtractionSchema().Properties["event_person_tags"].Items.Properties
	require.Contains(t, merged["subject"].Description, "必须是具体专有名词")
	require.Contains(t, merged["subject_key"].Description, "必须是具体专有名词")
}
