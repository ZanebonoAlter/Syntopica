package service

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"syntopica-backend/internal/platform/airouter"
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

// TestPromptVersion_Is6 guards the prompt version bump: "5.0" is the section
// title decouple (2026-08-29), "6.0" is intra-lane theme bucketing
// (add-intra-lane-theme-bucketing D10 — published versions are never reused).
func TestPromptVersion_Is6(t *testing.T) {
	if promptVersion != "6.0" {
		t.Errorf("promptVersion should be \"6.0\", got %q", promptVersion)
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

// ---- highlights response validation (fix-daily-report-highlights-null) ----
//
// The 2026-11-03 incident: the model echoed the JSON schema back verbatim
// (valid JSON, no "highlights" key). parseHighlightsResponse used to return
// (nil, nil) for that shape — the failure was silent, the persisted
// highlights column became a JSON null scalar, and the frontend hid the whole
// "今日重点" block. The white-box branch table (test-cases.md B1..B11) pins
// the new contract: legality is decided by the top-level "highlights" key.

// schemaEchoHighlightsResponse is the verbatim response captured from
// ai_call_logs (id 2612630): the model copied the request schema back.
const schemaEchoHighlightsResponse = `{"type":"object","properties":{"highlights":{"type":"array","items":{"type":"object","properties":{"reason":{"type":"string"},"tag_ids":{"type":"array"},"title":{"type":"string"}}}},"required":["highlights"]}`

// highlightsTestTags is the tag set used by the parser tests (ids 1 and 2 are
// the only legal tag_ids).
func highlightsTestTags() []repository.TagInput {
	return []repository.TagInput{
		{ID: 1, Label: "降准"},
		{ID: 2, Label: "逆回购"},
	}
}

// TestParseHighlightsResponse_RejectsInvalidJSON (B2): a truncated response is
// an invalid response — the parser must return an error, not an empty result.
func TestParseHighlightsResponse_RejectsInvalidJSON(t *testing.T) {
	cases := []struct {
		name    string
		content string
	}{
		{"truncated object", `{"highlights":`},
		{"empty string", ``},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			highlights, err := parseHighlightsResponse(c.content, highlightsTestTags())
			require.Error(t, err)
			require.Nil(t, highlights)
		})
	}
}

// TestParseHighlightsResponse_RejectsNonObject (B3): a top-level JSON array
// has no "highlights" key — invalid.
func TestParseHighlightsResponse_RejectsNonObject(t *testing.T) {
	highlights, err := parseHighlightsResponse(`[{"title":"x"}]`, highlightsTestTags())
	require.Error(t, err)
	require.Nil(t, highlights)
}

// TestParseHighlightsResponse_RejectsSchemaEcho (B4, core regression): the
// model echoed the JSON schema back. That is valid JSON but carries no
// top-level "highlights" key, so it MUST be rejected with an error — the
// silent (nil, nil) path is what made the whole block disappear.
func TestParseHighlightsResponse_RejectsSchemaEcho(t *testing.T) {
	highlights, err := parseHighlightsResponse(schemaEchoHighlightsResponse, highlightsTestTags())
	require.Error(t, err, "schema echo must be rejected, not silently read as empty")
	require.Nil(t, highlights)
	require.Contains(t, err.Error(), "highlights")
}

// TestParseHighlightsResponse_RejectsNonArrayHighlights (B5): the key exists
// but its value is not an array — invalid (type mismatch).
func TestParseHighlightsResponse_RejectsNonArrayHighlights(t *testing.T) {
	cases := []struct {
		name    string
		content string
	}{
		{"object value", `{"highlights":{}}`},
		{"string value", `{"highlights":"none"}`},
		{"null value", `{"highlights":null}`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			highlights, err := parseHighlightsResponse(c.content, highlightsTestTags())
			require.Error(t, err)
			require.Nil(t, highlights)
		})
	}
}

// TestParseHighlightsResponse_AcceptsEmptyArray (B6): the key exists with an
// empty array — a legal degradation ("宁可不生成"), no error, no retry.
func TestParseHighlightsResponse_AcceptsEmptyArray(t *testing.T) {
	highlights, err := parseHighlightsResponse(`{"highlights":[]}`, highlightsTestTags())
	require.NoError(t, err)
	require.NotNil(t, highlights, "empty array must decode to a non-nil empty slice")
	require.Empty(t, highlights)
}

// TestParseHighlightsResponse_Normal (B7): three well-formed highlights come
// back verbatim (title / reason preserved, tag_ids filtered).
func TestParseHighlightsResponse_Normal(t *testing.T) {
	content := `{"highlights":[` +
		`{"title":"要闻一","reason":"理由一","tag_ids":[1]},` +
		`{"title":"要闻二","reason":"理由二","tag_ids":[2]},` +
		`{"title":"要闻三","reason":"理由三","tag_ids":[1,2]}]}`

	highlights, err := parseHighlightsResponse(content, highlightsTestTags())
	require.NoError(t, err)
	require.Len(t, highlights, 3)
	assert.Equal(t, "要闻一", highlights[0].Title)
	assert.Equal(t, "理由一", highlights[0].Reason)
	assert.Equal(t, []uint{1}, highlights[0].TagIDs)
	assert.Equal(t, "要闻三", highlights[2].Title)
	assert.Equal(t, []uint{1, 2}, highlights[2].TagIDs)
}

// TestParseHighlightsResponse_DropsBlankTitle (B8): blank-title entries are
// dropped (existing filter semantics, unchanged by this fix).
func TestParseHighlightsResponse_DropsBlankTitle(t *testing.T) {
	content := `{"highlights":[` +
		`{"title":"   ","reason":"空白标题","tag_ids":[1]},` +
		`{"title":"有效要闻","reason":"理由","tag_ids":[1]}]}`

	highlights, err := parseHighlightsResponse(content, highlightsTestTags())
	require.NoError(t, err)
	require.Len(t, highlights, 1)
	assert.Equal(t, "有效要闻", highlights[0].Title)
}

// TestParseHighlightsResponse_FiltersUnknownTagIDs (B9): tag_ids not present in
// the day's tag set are dropped, legal ones survive (existing filter
// semantics, unchanged by this fix).
func TestParseHighlightsResponse_FiltersUnknownTagIDs(t *testing.T) {
	content := `{"highlights":[{"title":"要闻","reason":"理由","tag_ids":[1,999999]}]}`

	highlights, err := parseHighlightsResponse(content, highlightsTestTags())
	require.NoError(t, err)
	require.Len(t, highlights, 1)
	assert.Equal(t, []uint{1}, highlights[0].TagIDs)
}

// TestGenerateHighlights_EmptyTagsSkipsCall (B1): no tags → no LLM call at
// all (existing early-return path, zero regression).
func TestGenerateHighlights_EmptyTagsSkipsCall(t *testing.T) {
	var calls int32
	orig := highlightsChatFn
	t.Cleanup(func() { highlightsChatFn = orig })
	highlightsChatFn = func(ctx context.Context, system, user string, schema *airouter.JSONSchema, operation string) (string, error) {
		atomic.AddInt32(&calls, 1)
		return `{"highlights":[]}`, nil
	}

	highlights, err := GenerateHighlights(context.Background(), nil, nil)
	require.NoError(t, err)
	require.Nil(t, highlights)
	assert.Equal(t, int32(0), atomic.LoadInt32(&calls), "empty tags must not trigger an LLM call")
}

// TestGenerateHighlights_RetriesOnSchemaEcho (B10): the first call returns the
// schema echo (rejected), the retry returns 3 valid highlights — the retry
// result is adopted and the first response is discarded. Exactly 2 calls.
func TestGenerateHighlights_RetriesOnSchemaEcho(t *testing.T) {
	var calls int32
	orig := highlightsChatFn
	t.Cleanup(func() { highlightsChatFn = orig })
	highlightsChatFn = func(ctx context.Context, system, user string, schema *airouter.JSONSchema, operation string) (string, error) {
		if atomic.AddInt32(&calls, 1) == 1 {
			return schemaEchoHighlightsResponse, nil
		}
		return `{"highlights":[` +
			`{"title":"要闻一","reason":"理由一","tag_ids":[1]},` +
			`{"title":"要闻二","reason":"理由二","tag_ids":[2]},` +
			`{"title":"要闻三","reason":"理由三","tag_ids":[1]}]}`, nil
	}

	highlights, err := GenerateHighlights(context.Background(), highlightsTestTags(), nil)
	require.NoError(t, err)
	require.Len(t, highlights, 3, "the retry result must be adopted")
	assert.Equal(t, "要闻一", highlights[0].Title)
	assert.Equal(t, int32(2), atomic.LoadInt32(&calls), "exactly one retry")
}

// TestGenerateHighlights_GivesUpAfterOneRetry (B11): both attempts return the
// schema echo → error, and the call count is exactly 2 (never an unbounded
// retry loop).
func TestGenerateHighlights_GivesUpAfterOneRetry(t *testing.T) {
	var calls int32
	orig := highlightsChatFn
	t.Cleanup(func() { highlightsChatFn = orig })
	highlightsChatFn = func(ctx context.Context, system, user string, schema *airouter.JSONSchema, operation string) (string, error) {
		atomic.AddInt32(&calls, 1)
		return schemaEchoHighlightsResponse, nil
	}

	highlights, err := GenerateHighlights(context.Background(), highlightsTestTags(), nil)
	require.Error(t, err)
	require.Nil(t, highlights)
	assert.Equal(t, int32(2), atomic.LoadInt32(&calls), "exactly two attempts (one retry), never more")
}

// ---- call-failure branch (the retry must also cover a failed CALL, not only
// a rejected response) ----

// TestGenerateHighlights_RetriesOnCallError (R-2a): the first CALL fails
// (provider / transport error, not a parse rejection). The call must be
// retried once with the same parameters and the retry result adopted —
// exactly 2 calls, no error.
func TestGenerateHighlights_RetriesOnCallError(t *testing.T) {
	var calls int32
	orig := highlightsChatFn
	t.Cleanup(func() { highlightsChatFn = orig })
	highlightsChatFn = func(ctx context.Context, system, user string, schema *airouter.JSONSchema, operation string) (string, error) {
		if atomic.AddInt32(&calls, 1) == 1 {
			return "", errors.New("boom")
		}
		return `{"highlights":[{"title":"重试要闻","reason":"理由","tag_ids":[1]}]}`, nil
	}

	highlights, err := GenerateHighlights(context.Background(), highlightsTestTags(), nil)
	require.NoError(t, err)
	require.Len(t, highlights, 1, "the retry result must be adopted")
	assert.Equal(t, "重试要闻", highlights[0].Title)
	assert.Equal(t, int32(2), atomic.LoadInt32(&calls), "a failed call must be retried exactly once")
}

// TestGenerateHighlights_GivesUpAfterCallError (R-2b): both CALLS fail →
// error, exactly 2 calls (never an unbounded retry loop), and the original
// provider error must stay reachable through the %w chain so the caller can
// tell "why did the LLM call fail" apart from "the response was garbage".
func TestGenerateHighlights_GivesUpAfterCallError(t *testing.T) {
	errBoom := errors.New("boom")
	var calls int32
	orig := highlightsChatFn
	t.Cleanup(func() { highlightsChatFn = orig })
	highlightsChatFn = func(ctx context.Context, system, user string, schema *airouter.JSONSchema, operation string) (string, error) {
		atomic.AddInt32(&calls, 1)
		return "", errBoom
	}

	highlights, err := GenerateHighlights(context.Background(), highlightsTestTags(), nil)
	require.Error(t, err)
	require.Nil(t, highlights)
	require.ErrorIs(t, err, errBoom, "the original call error must remain in the %w chain")
	assert.Equal(t, int32(2), atomic.LoadInt32(&calls), "exactly two attempts (one retry), never more")
}

// TestGenerateHighlights_EmptyArrayDoesNotRetry (R-3): a legal empty array
// ("宁可不生成") is NOT a failure — it must NOT trigger the retry branch.
// The call count stays at 1 and the result is an empty (non-nil) slice.
func TestGenerateHighlights_EmptyArrayDoesNotRetry(t *testing.T) {
	var calls int32
	orig := highlightsChatFn
	t.Cleanup(func() { highlightsChatFn = orig })
	highlightsChatFn = func(ctx context.Context, system, user string, schema *airouter.JSONSchema, operation string) (string, error) {
		atomic.AddInt32(&calls, 1)
		return `{"highlights":[]}`, nil
	}

	highlights, err := GenerateHighlights(context.Background(), highlightsTestTags(), nil)
	require.NoError(t, err)
	require.NotNil(t, highlights, "a legal empty array must decode to a non-nil empty slice")
	require.Empty(t, highlights)
	assert.Equal(t, int32(1), atomic.LoadInt32(&calls), "a legal empty array must not trigger a retry")
}

// TestHighlightsChatFn_CarriesTagAndClusterCounts (R-6): the hook signature
// has no metadata slot, so the tag / cluster counts travel through the ctx.
// Both the first attempt AND the retry must observe the same counts — the
// retry is a same-parameter retry, and ai_call_logs metadata must stay
// identical to the pre-hook call.
func TestHighlightsChatFn_CarriesTagAndClusterCounts(t *testing.T) {
	tags := []repository.TagInput{
		{ID: 1, Label: "降准"},
		{ID: 2, Label: "逆回购"},
		{ID: 3, Label: "LPR"},
	}
	clusters := []repository.ClusterGroup{
		{GroupName: "货币政策", TagIDs: []uint{1, 2}},
		{GroupName: "利率", TagIDs: []uint{3}},
	}

	var calls int32
	var seen []highlightsCallCounts
	orig := highlightsChatFn
	t.Cleanup(func() { highlightsChatFn = orig })
	highlightsChatFn = func(ctx context.Context, system, user string, schema *airouter.JSONSchema, operation string) (string, error) {
		atomic.AddInt32(&calls, 1)
		seen = append(seen, highlightsCallCountsFrom(ctx))
		if len(seen) == 1 {
			// Force the retry branch so the second attempt is observed too.
			return schemaEchoHighlightsResponse, nil
		}
		return `{"highlights":[{"title":"要闻","reason":"理由","tag_ids":[1]}]}`, nil
	}

	_, err := GenerateHighlights(context.Background(), tags, clusters)
	require.NoError(t, err)
	require.Len(t, seen, 2, "the first attempt must be retried once")
	for i, counts := range seen {
		assert.Equal(t, len(tags), counts.tagCount, "call %d: tag count must be carried in ctx", i+1)
		assert.Equal(t, len(clusters), counts.clusterCount, "call %d: cluster count must be carried in ctx", i+1)
	}
}
