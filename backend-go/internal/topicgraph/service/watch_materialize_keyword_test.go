package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"syntopica-backend/internal/topicgraph/repository"
)

// ── DNF article matching (design §D2: same parseKeywordExpr semantics) ──

func strPtrOf(s string) *string { return &s }

func TestMatchKeywordArticles_BasicAndCaseInsensitive(t *testing.T) {
	articles := []repository.WatchScanArticle{
		{ID: 1, Title: "Harness v2 released", Summary: "minor updates"},
		{ID: 2, Title: "Unrelated news", Summary: "nothing here"},
		{ID: 3, Title: "something", Summary: "uses HARNESS internally"},
	}
	hits := matchKeywordArticles("harness", articles)
	require.Len(t, hits, 2)
	assert.Equal(t, uint(1), hits[0].ID)
	assert.Equal(t, uint(3), hits[1].ID)
}

func TestMatchKeywordArticles_ANDSemantics(t *testing.T) {
	articles := []repository.WatchScanArticle{
		{ID: 1, Title: "ASML earnings", Summary: "lithography"},
		{ID: 2, Title: "镓锗出口管制", Summary: ""},
		{ID: 3, Title: "ASML 受镓锗出口影响", Summary: ""},
	}
	// "ASML|镓锗 出口" ⇒ (ASML OR 镓锗) AND 出口
	hits := matchKeywordArticles("ASML|镓锗 出口", articles)
	require.Len(t, hits, 2)
	assert.Equal(t, uint(2), hits[0].ID)
	assert.Equal(t, uint(3), hits[1].ID)
}

func TestMatchKeywordArticles_InvalidExpressionMatchesNothing(t *testing.T) {
	articles := []repository.WatchScanArticle{{ID: 1, Title: "harness", Summary: ""}}
	assert.Empty(t, matchKeywordArticles("ASML|", articles), "trailing '|' is unfinished ⇒ no hits")
	assert.Empty(t, matchKeywordArticles("|", articles))
}

func TestMatchKeywordArticles_TitleOnlyArticle(t *testing.T) {
	// Summary missing degrades to title-only — a tag-less, summary-less
	// article still hits on its title (spec: 漏网文章可被捞回).
	articles := []repository.WatchScanArticle{{ID: 9, Title: "new harness release", Summary: ""}}
	hits := matchKeywordArticles("harness", articles)
	require.Len(t, hits, 1)
}

// ── section/thread assembly contract (design §D1 field contract) ──

func TestBuildKeywordWatchSection_FieldContract(t *testing.T) {
	w := repository.BoardTopicWatch{ID: 5, Label: "harness", Type: repository.WatchTypeKeywordTopic}
	tagJSON := "[3,1,2]"
	hits := []repository.WatchScanArticle{
		{ID: 101, Title: "article one", Summary: "summary one", TagIDs: &tagJSON},
		{ID: 102, Title: "article two", Summary: "", TagIDs: nil},
	}

	sec, threads := buildKeywordWatchSection(w, hits, 7)

	assert.Equal(t, 7, sec.ClusterIndex, "cluster index continues after regular clusters")
	assert.Equal(t, "关键字『harness』相关话题", sec.ClusterLabel, "fixed predictable section name")
	assert.Equal(t, repository.JSON("[]"), sec.ClusterTagIDs, "article-anchored section carries no tag cluster")
	assert.Equal(t, 2, sec.ArticleCount)
	assert.Equal(t, 4, sec.BestTier, "no board-match signal ⇒ worst tier")
	assert.Equal(t, 0.0, sec.AvgScore)
	assert.Equal(t, LaneTierWatchKeyword, sec.LaneTier)
	assert.Nil(t, sec.PersistentTopicID, "keyword track never owns a persistent topic")
	assert.Empty(t, sec.Embedding, "watch sections carry no embedding")

	require.Len(t, threads, 2)
	assert.Equal(t, "article one", threads[0].Title)
	assert.Equal(t, "summary one", threads[0].Summary)
	assert.Equal(t, repository.JSON("[3,1,2]"), threads[0].TagIDs, "article's own tag ids preserved")
	assert.Equal(t, repository.JSON("[101]"), threads[0].RelatedArticleIDs)
	assert.Equal(t, float64(1.0), threads[0].Confidence, "mechanical deterministic match")
	assert.Equal(t, repository.JSON("[]"), threads[1].TagIDs, "tag-less article degrades to empty array")
}

func TestTruncateRunes(t *testing.T) {
	assert.Equal(t, "abc", truncateRunes("abcdef", 3))
	assert.Equal(t, "中文三", truncateRunes("中文三个字", 3))
	assert.Equal(t, "short", truncateRunes("short", 10))
	assert.Equal(t, "", truncateRunes("x", 0))
}

func TestParseWatchTagIDs(t *testing.T) {
	assert.Nil(t, parseWatchTagIDs(nil))
	assert.Nil(t, parseWatchTagIDs(strPtrOf("")))
	assert.Nil(t, parseWatchTagIDs(strPtrOf("not json")))
	assert.Equal(t, []uint{1, 2, 3}, parseWatchTagIDs(strPtrOf("[1,2,3]")))
	assert.Equal(t, []uint{}, parseWatchTagIDs(strPtrOf("[]")))
}

func TestMustMarshalUintArray(t *testing.T) {
	assert.Equal(t, repository.JSON("[]"), mustMarshalUintArray(nil))
	assert.Equal(t, repository.JSON("[7]"), mustMarshalUintArray([]uint{7}))
	assert.Equal(t, repository.JSON("[1,2,3]"), mustMarshalUintArray([]uint{1, 2, 3}))
}

// ── report date is mandatory (fix-duplicate-article-heat D1/D7) ──
//
// The zero-value fallback (date = time.Now()) silently scanned "today" while
// rebuilding a historical report, leaking the same articles into several
// consecutive daily reports. A missing report date must fail loudly instead.

func TestScanBoardDayArticles_ZeroDateReturnsError(t *testing.T) {
	_, err := scanBoardDayArticles(1, time.Time{})
	require.Error(t, err, "zero report date must fail, never degrade to today")
	assert.Contains(t, err.Error(), "report date is required")
}

func TestMaterializeKeywordWatches_ZeroDateReturnsError(t *testing.T) {
	watches := []repository.BoardTopicWatch{{ID: 1, Label: "harness", Type: repository.WatchTypeKeywordTopic}}
	_, _, err := MaterializeKeywordWatches(context.Background(), 1, watches, time.Time{}, 0, WatchMaterializeConfig{Enabled: false}, nil)
	require.Error(t, err, "keyword materialization must refuse a missing report date")
	assert.Contains(t, err.Error(), "scan day articles: report date is required")
}
