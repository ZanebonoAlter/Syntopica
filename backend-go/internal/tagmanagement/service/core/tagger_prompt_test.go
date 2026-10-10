package core

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestEventTagDescriptionPromptTimeAnchorDiscipline pins the time-anchor
// contract for the event-tag description prompt (fix-event-tag-time-anchor):
// the discipline sentences must be present, and the few-shot examples must not
// carry real historical event + year/month anchors that the LLM would copy
// instead of reading the article Context.
func TestEventTagDescriptionPromptTimeAnchorDiscipline(t *testing.T) {
	prompt := buildEventTagDescriptionPrompt("某测试事件", "event", "[日期: 2026-10-09] 某文章标题. 摘要内容")

	// 时间锚约束句存在：以 Context [日期] 为参照 + 禁止 Context 外时间 + 同名历史事件以 Context 为准
	require.Contains(t, prompt, "[日期: …]")
	require.Contains(t, prompt, "MUST be anchored to the [日期: …]")
	require.Contains(t, prompt, "MUST NOT introduce any time information absent from the Context")
	require.Contains(t, prompt, "follow the Context, not your memory")

	// 示例不含真实历史事件年月组合（污染源：示例时间锚被照抄）
	for _, banned := range []string{"伊朗", "以色列", "袭击", "WWDC", "2024年", "2023年", "2025年"} {
		require.NotContains(t, prompt, banned, "event prompt example must not carry real-event time anchor %q", banned)
	}

	// 中性示例存在（示范格式，不示范事实）
	require.Contains(t, prompt, "某国央行上调基准利率")
}

// TestSectionExtractionPromptNeutralExamples pins the section extraction
// prompt's output example to a neutral/fictional event: no real event name that
// would bias extraction towards unrelated articles.
func TestSectionExtractionPromptNeutralExamples(t *testing.T) {
	prompt := buildSectionExtractionPrompt()

	require.Contains(t, prompt, "正例")
	require.Contains(t, prompt, "某国央行上调基准利率")
	for _, banned := range []string{"伊朗", "以色列"} {
		require.NotContains(t, prompt, banned, "section prompt example must stay neutral, found %q", banned)
	}
}

// TestMergedExtractionPromptNeutralExamples is the merged-prompt counterpart of
// TestSectionExtractionPromptNeutralExamples.
func TestMergedExtractionPromptNeutralExamples(t *testing.T) {
	prompt := buildMergedExtractionPrompt()

	require.Contains(t, prompt, "正例")
	require.Contains(t, prompt, "某国央行上调基准利率")
	for _, banned := range []string{"伊朗", "以色列"} {
		require.NotContains(t, prompt, banned, "merged prompt example must stay neutral, found %q", banned)
	}
}
