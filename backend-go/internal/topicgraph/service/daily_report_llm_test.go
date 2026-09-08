package service

import (
	"strings"
	"testing"

	"syntopica-backend/internal/topicgraph/repository"
)

// TestBuildHighlightsPrompt_InjectsArticleContext verifies the highlights prompt carries
// representative article titles+summaries when ArticleContext is set, and omits the line when empty.
func TestBuildHighlightsPrompt_InjectsArticleContext(t *testing.T) {
	tags := []repository.TagInput{
		{ID: 1, Label: "降准", ArticleCount: 5, ArticleContext: "《央行降准》下调0.5个百分点"},
		{ID: 2, Label: "无上下文标签", ArticleCount: 2}, // ArticleContext empty
	}
	got := buildHighlightsPrompt(tags, nil)
	if !strings.Contains(got, "《央行降准》下调0.5个百分点") {
		t.Errorf("highlights prompt should include article context, got:\n%s", got)
	}
	// empty ArticleContext tag must still list the tag but emit no context line
	if !strings.Contains(got, "无上下文标签") {
		t.Errorf("highlights prompt should still list the no-context tag, got:\n%s", got)
	}
}

// TestBuildThreadsPrompt_InjectsArticleContext verifies threads prompt carries article context.
func TestBuildThreadsPrompt_InjectsArticleContext(t *testing.T) {
	cluster := repository.ClusterGroup{GroupName: "货币政策", TagIDs: []uint{1}}
	tags := []repository.TagInput{
		{ID: 1, Label: "降准", ArticleCount: 5, ArticleContext: "《央行降准》释放长期资金"},
	}
	got := buildThreadsPrompt(cluster, tags)
	if !strings.Contains(got, "《央行降准》释放长期资金") {
		t.Errorf("threads prompt should include article context, got:\n%s", got)
	}
}

// TestBuildClusterPrompt_InjectsArticleContext verifies cluster prompt carries article context.
func TestBuildClusterPrompt_InjectsArticleContext(t *testing.T) {
	tags := []repository.TagInput{
		{ID: 1, Label: "降准", ArticleCount: 5, ArticleContext: "《央行降准》释放约1万亿流动性"},
	}
	got := buildClusterPrompt(tags)
	if !strings.Contains(got, "《央行降准》释放约1万亿流动性") {
		t.Errorf("cluster prompt should include article context, got:\n%s", got)
	}
}

// ---- prompt hygiene: fact-anchor constraints & prompt version ----

// TestThreadsSystemPrompt_HasFactAnchor guards that the threads system prompt
// carries the fact-anchor constraint (base on listed tag facts, no fabrication).
func TestThreadsSystemPrompt_HasFactAnchor(t *testing.T) {
	if !strings.Contains(threadsSystemPrompt, "禁止编造") {
		t.Errorf("threadsSystemPrompt should contain fact-anchor keyword 禁止编造")
	}
	if !strings.Contains(threadsSystemPrompt, "所列标签") {
		t.Errorf("threadsSystemPrompt should require basing on 所列标签 facts")
	}
}

// TestHighlightsSystemPrompt_HasFactAnchor guards the same for highlights.
func TestHighlightsSystemPrompt_HasFactAnchor(t *testing.T) {
	if !strings.Contains(highlightsSystemPrompt, "禁止编造") {
		t.Errorf("highlightsSystemPrompt should contain fact-anchor keyword 禁止编造")
	}
	if !strings.Contains(highlightsSystemPrompt, "所列标签") {
		t.Errorf("highlightsSystemPrompt should require basing on 所列标签 facts")
	}
}

// TestPromptVersion_Is5 guards the prompt version bump (5.0 = section title decouple).
func TestPromptVersion_Is5(t *testing.T) {
	if promptVersion != "5.0" {
		t.Errorf("promptVersion should be \"5.0\", got %q", promptVersion)
	}
}

// ---- section title decouple (spec: section 展示标题内容化) ----

// TestThreadsSystemPrompt_RequestsSectionTitle guards Scenario 标题遵守事实锚约束:
// the system prompt must instruct the model to author a daily section_title
// under the fact-anchor rule and must forbid parroting the topic/group name.
func TestThreadsSystemPrompt_RequestsSectionTitle(t *testing.T) {
	if !strings.Contains(threadsSystemPrompt, "section_title") {
		t.Errorf("threadsSystemPrompt should request section_title output")
	}
	if !strings.Contains(threadsSystemPrompt, "不得复述") {
		t.Errorf("threadsSystemPrompt should forbid parroting topic/group names")
	}
}

// TestParseThreadsResponse_SectionTitle covers the parser variants V1/V2/V4:
// present title is trimmed and returned, missing/blank titles degrade to "".
func TestParseThreadsResponse_SectionTitle(t *testing.T) {
	tags := []repository.TagInput{{ID: 1, Label: "日本政治"}}

	cases := []struct {
		name    string
		content string
		want    string
	}{
		{"present title", `{"section_title":" 高市执政基础不稳 ","threads":[{"title":"党内反弹","summary":"s","tag_ids":[1],"confidence":0.9}]}`, "高市执政基础不稳"},
		{"missing field", `{"threads":[{"title":"党内反弹","summary":"s","tag_ids":[1],"confidence":0.9}]}`, ""},
		{"blank title", `{"section_title":"  ","threads":[]}`, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, got, err := parseThreadsResponse(c.content, tags)
			if err != nil {
				t.Fatalf("parse error: %v", err)
			}
			if got != c.want {
				t.Errorf("section_title = %q, want %q", got, c.want)
			}
		})
	}
}

// ---- fallback thread synthesis (empty LLM response guard) ----

// TestSynthesizeFallbackThreads_AnchorsOnTopScoringTag verifies the fallback
// picks the highest-score tag as title/summary anchor and links ALL cluster
// tags, so a fact-anchor "empty threads" response never persists a section
// with zero threads (empty shell under a live persistent topic).
func TestSynthesizeFallbackThreads_AnchorsOnTopScoringTag(t *testing.T) {
	cluster := repository.ClusterGroup{GroupName: "货币政策", TagIDs: []uint{1, 2}}
	tags := []repository.TagInput{
		{ID: 1, Label: "降准", Description: "央行下调准备金率", Score: 0.6},
		{ID: 2, Label: "逆回购", Description: "开展逆回购操作", Score: 0.9},
	}
	got := synthesizeFallbackThreads(cluster, tags)
	if len(got) != 1 {
		t.Fatalf("fallback should synthesize exactly 1 thread, got %d", len(got))
	}
	th := got[0]
	if th.Title != "逆回购" {
		t.Errorf("title should anchor on top-scoring tag, got %q", th.Title)
	}
	if th.Summary != "开展逆回购操作" {
		t.Errorf("summary should be top tag description, got %q", th.Summary)
	}
	if len(th.TagIDs) != 2 || th.TagIDs[0] != 1 || th.TagIDs[1] != 2 {
		t.Errorf("tag_ids should link all cluster tags, got %v", th.TagIDs)
	}
	if th.Confidence != fallbackThreadConfidence {
		t.Errorf("confidence should mark low-trust synthesis (%v), got %v", fallbackThreadConfidence, th.Confidence)
	}
}

// TestSynthesizeFallbackThreads_DescriptionFallback verifies the placeholder
// summary when the top tag has no description.
func TestSynthesizeFallbackThreads_DescriptionFallback(t *testing.T) {
	cluster := repository.ClusterGroup{TagIDs: []uint{7}}
	tags := []repository.TagInput{{ID: 7, Label: "纯标签", ArticleCount: 2}}
	got := synthesizeFallbackThreads(cluster, tags)
	if len(got) != 1 {
		t.Fatalf("fallback should synthesize 1 thread, got %d", len(got))
	}
	if !strings.Contains(got[0].Summary, "纯标签") || !strings.Contains(got[0].Summary, "暂未提炼出叙事线索") {
		t.Errorf("placeholder summary should mention tag label and the no-narrative note, got %q", got[0].Summary)
	}
}

// TestSynthesizeFallbackThreads_NoTagsReturnsNil guards the empty-input edge.
func TestSynthesizeFallbackThreads_NoTagsReturnsNil(t *testing.T) {
	got := synthesizeFallbackThreads(repository.ClusterGroup{TagIDs: []uint{1}}, nil)
	if got != nil {
		t.Errorf("no cluster tags should yield nil, got %v", got)
	}
}
