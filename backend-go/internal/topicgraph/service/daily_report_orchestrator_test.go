package service

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/testutil"
	"syntopica-backend/internal/topicgraph/repository"
)

func TestBuildQualityBreakdownJSON(t *testing.T) {
	tags := []repository.TagInput{
		{ID: 1, Label: "AI芯片", MatchReason: "direct_hit", Score: 1.0},
		{ID: 2, Label: "GPT-5发布", MatchReason: "max_sim", Score: 0.85},
		{ID: 3, Label: "AI竞赛", MatchReason: "weighted", Score: 0.59},
		{ID: 4, Label: "GPU短缺", MatchReason: "max_sim", Score: 0.72},
	}

	// Test with 3 tag IDs
	raw := buildQualityBreakdownJSON(tags, []uint{1, 2, 3})
	require.NotNil(t, raw)

	var entries []map[string]any
	err := json.Unmarshal(raw, &entries)
	require.NoError(t, err)
	assert.Len(t, entries, 3)

	// Verify each entry has the expected fields
	for _, e := range entries {
		assert.NotNil(t, e["tag_id"])
		assert.NotNil(t, e["label"])
		assert.NotNil(t, e["match_reason"])
		assert.NotNil(t, e["score"])
		assert.NotContains(t, e, "downgraded", "downgraded 键已随标记退役移除")
	}

	// Verify first entry (direct_hit)
	assert.Equal(t, float64(1), entries[0]["tag_id"])
	assert.Equal(t, "AI芯片", entries[0]["label"])
	assert.Equal(t, "direct_hit", entries[0]["match_reason"])
	assert.Equal(t, float64(1), entries[0]["score"])

	// Test empty tag IDs
	raw2 := buildQualityBreakdownJSON(tags, []uint{})
	require.NotNil(t, raw2)
	var emptyEntries []map[string]any
	err = json.Unmarshal(raw2, &emptyEntries)
	require.NoError(t, err)
	assert.Len(t, emptyEntries, 0)
}

func TestBuildQualityBreakdownJSON_NoDowngradedKey(t *testing.T) {
	// downgraded 标记已于 2026-11-12 退役：breakdown 不再写 downgraded 键。
	tags := []repository.TagInput{
		{ID: 10, Label: "标签", MatchReason: "max_sim", Score: 0.82},
	}

	raw := buildQualityBreakdownJSON(tags, []uint{10})
	var entries []map[string]any
	err := json.Unmarshal(raw, &entries)
	require.NoError(t, err)
	assert.Len(t, entries, 1)
	assert.NotContains(t, entries[0], "downgraded")
	assert.Equal(t, "max_sim", entries[0]["match_reason"])
	assert.Equal(t, float64(0.82), entries[0]["score"])
}

func TestFilterTagsByQuality_PreservesDirectHitHitRateMaxSim(t *testing.T) {
	tags := []repository.TagInput{
		{ID: 1, MatchReason: "direct_hit", Score: 1.0},
		{ID: 2, MatchReason: "hit_rate", Score: 0.9},
		{ID: 3, MatchReason: "max_sim", Score: 0.8},
		{ID: 4, MatchReason: "weighted", Score: 0.5},
	}
	result := filterTagsByQuality(tags)
	assert.Len(t, result, 4) // weighted pulled back since < 10
}

func TestFilterTagsByQuality_PullsBackWeightedWhenFew(t *testing.T) {
	tags := []repository.TagInput{
		{ID: 1, MatchReason: "direct_hit", Score: 1.0},
		{ID: 2, MatchReason: "direct_hit", Score: 0.9},
		{ID: 3, MatchReason: "direct_hit", Score: 0.8},
		{ID: 4, MatchReason: "weighted", Score: 0.5},
		{ID: 5, MatchReason: "weighted", Score: 0.4},
	}
	result := filterTagsByQuality(tags)
	// < 10 kept → weighted pulled back
	assert.Len(t, result, 5)
}

func TestFilterTagsByQuality_TruncatesAbove30(t *testing.T) {
	tags := make([]repository.TagInput, 0, 35)
	for i := 0; i < 35; i++ {
		tags = append(tags, repository.TagInput{
			ID: uint(i + 1), MatchReason: "max_sim", Score: float64(100 - i),
		})
	}
	result := filterTagsByQuality(tags)
	assert.Len(t, result, 30)
}

func TestFilterTagsByQuality_TierTruncation(t *testing.T) {
	// downgraded 已退役：max_sim 统一 tier 2。30 条 direct_hit（tier 0）+
	// max_sim/weighted 追加项 → 截断只保 30 条 tier 0。
	tags := make([]repository.TagInput, 0, 35)
	for i := 0; i < 30; i++ {
		tags = append(tags, repository.TagInput{
			ID: uint(i + 1), MatchReason: "direct_hit", Score: float64(100 - i),
		})
	}
	tags = append(tags, repository.TagInput{ID: 31, MatchReason: "max_sim", Score: 0.99})
	tags = append(tags, repository.TagInput{ID: 32, MatchReason: "max_sim", Score: 0.95})
	tags = append(tags, repository.TagInput{ID: 35, MatchReason: "weighted", Score: 0.5})

	result := filterTagsByQuality(tags)
	assert.Len(t, result, 30)
	for _, r := range result {
		assert.Equal(t, "direct_hit", r.MatchReason, "tier 0 满额时低档不得挤占")
	}
}

// TestBuildQualityBreakdownJSON_MergedSections covers the spec scenario
// "合并后重算明细": when MergeSimilarSections merges section A (tag IDs {1,2})
// with section B (tag IDs {3,4}), the recomputed quality_breakdown SHALL be
// the union of all four tags' details. MergeSimilarSections itself calls
// buildQualityBreakdownJSON with the merged tagIDSet, so this test asserts the
// union semantics the merge path relies on.
func TestBuildQualityBreakdownJSON_MergedSections(t *testing.T) {
	tags := []repository.TagInput{
		{ID: 1, Label: "AI芯片", MatchReason: "direct_hit", Score: 1.0},
		{ID: 2, Label: "GPT-5发布", MatchReason: "max_sim", Score: 0.85},
		{ID: 3, Label: "AI竞赛", MatchReason: "weighted", Score: 0.59},
		{ID: 4, Label: "GPU短缺", MatchReason: "max_sim", Score: 0.72},
	}

	// Merged tagIDSet = union of section A {1,2} and section B {3,4}.
	raw := buildQualityBreakdownJSON(tags, []uint{1, 2, 3, 4})
	require.NotNil(t, raw)

	var entries []map[string]any
	err := json.Unmarshal(raw, &entries)
	require.NoError(t, err)

	// Union of both sections → 4 entries, one per source tag.
	assert.Len(t, entries, 4)

	seenIDs := make(map[float64]bool, 4)
	for _, e := range entries {
		id, ok := e["tag_id"].(float64)
		require.True(t, ok)
		seenIDs[id] = true
		// Every entry carries the full per-tag detail (spec: tag_id/label/
		// match_reason/score/downgraded).
		assert.NotNil(t, e["label"])
		assert.NotNil(t, e["match_reason"])
		assert.NotNil(t, e["score"])
		assert.NotContains(t, e, "downgraded", "downgraded 键已随标记退役移除")
	}
	// All four merged tag IDs are present (union, no drops).
	assert.True(t, seenIDs[1] && seenIDs[2] && seenIDs[3] && seenIDs[4])
}

// ---- section display title decouple (spec: section 展示标题内容化) ----

// TestResolveClusterLabel_FallbackChain walks the whole fallback chain
// (spec Scenario 标题生成失败时降级兜底 + white-box branch table B1..B4):
// LLM daily title → first thread title → matched topic label → group name.
func TestResolveClusterLabel_FallbackChain(t *testing.T) {
	topicID := uint(935)
	labels := map[uint]string{topicID: "日本首相高市早苗宣布不于7月释放石油储备"}
	cluster := repository.ClusterGroup{GroupName: "L3分组名", MatchedTopicID: &topicID}

	// B1: LLM daily title wins even when a topic is matched (Scenario 命中既有话题的 section 标题反映当天内容).
	assert.Equal(t, "高市执政基础不稳引发党内反弹担忧",
		resolveClusterLabel("高市执政基础不稳引发党内反弹担忧", nil, cluster, labels))

	// B1 with only whitespace degrades to B2 (variant V4).
	assert.Equal(t, "首条叙事线标题",
		resolveClusterLabel("   ", []repository.Thread{{Title: "首条叙事线标题"}}, cluster, labels))

	// B2: missing section_title falls to the first non-blank thread title.
	assert.Equal(t, "首条叙事线标题",
		resolveClusterLabel("", []repository.Thread{{Title: "首条叙事线标题"}, {Title: "第二条"}}, cluster, labels))

	// B3: empty threads + matched topic → legacy topic-label safety net (Scenario 标题生成失败时降级兜底).
	assert.Equal(t, "日本首相高市早苗宣布不于7月释放石油储备",
		resolveClusterLabel("", nil, cluster, labels))

	// B4: no title, no threads, no topic label → group name (Scenario L3 新话题标题行为不变).
	noTopic := repository.ClusterGroup{GroupName: "L3分组名"}
	assert.Equal(t, "L3分组名", resolveClusterLabel("", nil, noTopic, labels))

	// B4 edge: matched topic missing from the label map → group name.
	assert.Equal(t, "L3分组名", resolveClusterLabel("", nil, cluster, map[uint]string{}))
}

// TestResolveClusterLabel_LongTitleStaysWithinColumnLimit guards variant V5:
// the label column is gorm size:200, so a runaway LLM title must be
// rune-truncated before persisting.
func TestResolveClusterLabel_LongTitleStaysWithinColumnLimit(t *testing.T) {
	long := strings.Repeat("超", 600)
	got := resolveClusterLabel(long, nil, repository.ClusterGroup{GroupName: "g"}, nil)
	if l := len([]rune(got)); l != 200 {
		t.Errorf("resolved label length %d runes, want exactly 200 (column budget)", l)
	}
}

// TestFindPreviousReportBrief covers tasks 3.1's alignment with the
// 「日报关联昨日报告」 Scenario: the previous brief is the most recent
// COMPLETED report strictly before the target date (a generating/failed report
// must not become prev_report_id).
func TestFindPreviousReportBrief(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repository.InitRepository(db)
	t.Cleanup(func() { repository.Repo = nil })

	board := models.SemanticLabel{Label: "prev-report", Slug: "prev-report", LabelType: "board", Status: "active"}
	require.NoError(t, db.Create(&board).Error)
	today := repository.NormalizeReportDate(time.Now())

	yesterday := repository.BoardDailyReport{
		SemanticBoardID: board.ID,
		PeriodDate:      today.AddDate(0, 0, -1),
		Title:           "yesterday",
		Status:          "completed",
	}
	require.NoError(t, db.Create(&yesterday).Error)
	// A more recent, non-completed report must be skipped.
	require.NoError(t, db.Create(&repository.BoardDailyReport{
		SemanticBoardID: board.ID,
		PeriodDate:      today,
		Title:           "today in flight",
		Status:          "generating",
	}).Error)

	prev := findPreviousReportBrief(board.ID, today)
	require.NotNil(t, prev)
	assert.Equal(t, yesterday.ID, prev.ID, "prev_report_id links the last completed report")

	// A board with only an in-flight report has no previous brief.
	emptyBoard := models.SemanticLabel{Label: "prev-report-empty", Slug: "prev-report-empty", LabelType: "board", Status: "active"}
	require.NoError(t, db.Create(&emptyBoard).Error)
	require.NoError(t, db.Create(&repository.BoardDailyReport{
		SemanticBoardID: emptyBoard.ID,
		PeriodDate:      today,
		Title:           "only in flight",
		Status:          "generating",
	}).Error)
	assert.Nil(t, findPreviousReportBrief(emptyBoard.ID, today))
}

// ---- highlights payload resolution (fix-daily-report-highlights-null) ----

// TestResolveHighlightsPayload_DegradesToEmptyArray (B12): a failed highlights
// generation degrades to an empty JSON array — the payload is exactly "[]",
// never the JSON null scalar that used to hide the whole "今日重点" block.
func TestResolveHighlightsPayload_DegradesToEmptyArray(t *testing.T) {
	payload, degraded := resolveHighlightsPayload(nil, errors.New("highlights AI call failed: schema echo"))

	assert.True(t, degraded, "a failed highlights generation must report degradation")
	assert.Equal(t, []byte("[]"), payload)
	assert.NotEqual(t, []byte("null"), payload, "highlights must never persist a JSON null scalar")

	// The payload must decode as an empty array (jsonb contract).
	var decoded []map[string]any
	require.NoError(t, json.Unmarshal(payload, &decoded))
	assert.Empty(t, decoded)
}

// TestResolveHighlightsPayload_Normal (B13): a successful generation persists
// the highlights array unchanged and is NOT flagged as degraded.
func TestResolveHighlightsPayload_Normal(t *testing.T) {
	data := []repository.Highlight{
		{Title: "要闻一", Reason: "理由一", TagIDs: []uint{1}},
		{Title: "要闻二", Reason: "理由二", TagIDs: []uint{2}},
		{Title: "要闻三", Reason: "理由三", TagIDs: []uint{1, 2}},
	}

	payload, degraded := resolveHighlightsPayload(data, nil)

	assert.False(t, degraded, "a successful generation must not report degradation")
	require.NotEqual(t, []byte("null"), payload)

	var decoded []repository.Highlight
	require.NoError(t, json.Unmarshal(payload, &decoded))
	require.Len(t, decoded, 3)
	assert.Equal(t, "要闻一", decoded[0].Title)
	assert.Equal(t, "理由二", decoded[1].Reason)
	assert.Equal(t, []uint{1, 2}, decoded[2].TagIDs)
}

// TestResolveHighlightsPayload_EmptyButLegalIsNotDegraded (B6/B13 seam): a
// legal empty response is a normal (non-degraded) report with "[]" persisted.
func TestResolveHighlightsPayload_EmptyButLegalIsNotDegraded(t *testing.T) {
	payload, degraded := resolveHighlightsPayload([]repository.Highlight{}, nil)

	assert.False(t, degraded)
	assert.Equal(t, []byte("[]"), payload)
}
