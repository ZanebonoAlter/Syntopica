package core

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

// fix-tagging-pollution spec「快讯类文章 keyword 为空时保持 LLM 原样」：
// LLM 返回 1 个 event + 空 keyword 数组 → 结果仅含该 event、来源 llm，
// 不得注入分类名/规则词候选（旧契约 :59-61 heuristicKeywordCandidates 回填）。
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
	require.Equal(t, "llm", result.Source)
	require.Len(t, result.Tags, 1, "empty keyword array is a legal outcome: result keeps the event tag only")
	require.Equal(t, "美联储宣布维持利率不变", result.Tags[0].Label)
	require.NotContains(t, topicLabels(result.Tags), "新闻", "category-name backfill must not appear")
	require.Equal(t, 1, router.callCount("tag_extraction_merged"))
}

// fix-tagging-pollution spec「空摘要文章两个数组均为空」：两数组均空 → 原样返回
// 零候选（Tags=[]、Source=llm、Errors 照记），不触发 heuristic 降级。旧契约 :67
// 在 candidates==0 时调 extractWithHeuristic 返回非空 heuristic 结果（err=nil），
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
	require.Equal(t, "llm", result.Source, "zero candidates must stay source=llm, not downgrade to heuristic")
	require.Empty(t, result.Tags)
	require.NotEmpty(t, result.Errors, "branch notes for both empty arrays must still be recorded")
	require.Equal(t, 1, router.callCount("tag_extraction_merged"))
}
