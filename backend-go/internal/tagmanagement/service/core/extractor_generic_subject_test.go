package core

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// 事件指纹（subject/subject_key/action/object/occurred_at）已于 2026-11-12
// 从提取链路整体摘除：融合任务过重导致 4B 本地模型 JSON 输出大面积发散
// （parse_error 潮 → heuristic 兜底 → 版块全是降级泛词标签、日报断粮）。
// 本文件是摘除的反向守卫：prompt 与 schema 均不得再出现指纹指令/字段。
// 主体禁则（泛称黑名单）随指纹一并退役；存量 topic_tags.metadata.
// event_fingerprint 仍可由 topicgraph 侧只读解析。

// 场景：栏目片提取提示不得再要求指纹五字段与主体禁则。
func TestSectionExtractionPromptFingerprintRemoved(t *testing.T) {
	prompt := buildSectionExtractionPrompt()

	require.NotContains(t, prompt, "事件指纹", "指纹指令已摘除")
	require.NotContains(t, prompt, "subject_key", "指纹字段指令已摘除")
	require.NotContains(t, prompt, "主体禁则", "主体禁则随指纹一并摘除")
	require.NotContains(t, prompt, "subject", "指纹字段指令已摘除")
}

// merged 路径（单篇摘要提取）同步同一守卫。
func TestMergedExtractionPromptFingerprintRemoved(t *testing.T) {
	prompt := buildMergedExtractionPrompt()

	require.NotContains(t, prompt, "事件指纹", "指纹指令已摘除")
	require.NotContains(t, prompt, "subject_key", "指纹字段指令已摘除")
}

// schema 字段级守卫：两条路径的提取 schema 均不含指纹五字段。
func TestExtractionSchemasFingerprintFieldsRemoved(t *testing.T) {
	sectionProps := sectionExtractionSchema().Properties["tags"].Items.Properties
	for _, prop := range []string{"subject", "subject_key", "action", "object", "occurred_at"} {
		require.NotContains(t, sectionProps, prop, "section schema 不得残留指纹字段 %q", prop)
	}

	mergedProps := mergedTagExtractionSchema().Properties["event_person_tags"].Items.Properties
	for _, prop := range []string{"subject", "subject_key", "action", "object", "occurred_at"} {
		require.NotContains(t, mergedProps, prop, "merged schema 不得残留指纹字段 %q", prop)
	}
}
