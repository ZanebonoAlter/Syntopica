package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"syntopica-backend/internal/platform/airouter"
	"syntopica-backend/internal/topicgraph/repository"
)

// capturingChat records the last ChatRequest and answers with a canned
// response/error (nil resp → call error).
type capturingChat struct {
	calls   int
	lastReq airouter.ChatRequest
	resp    string
	err     error
}

func (cc *capturingChat) f(_ context.Context, req airouter.ChatRequest) (*airouter.ChatResult, error) {
	cc.calls++
	cc.lastReq = req
	if cc.err != nil {
		return nil, cc.err
	}
	return &airouter.ChatResult{Content: cc.resp}, nil
}

func adjCandidates(ids ...uint) []repository.WatchScanArticle {
	out := make([]repository.WatchScanArticle, 0, len(ids))
	for _, id := range ids {
		out = append(out, repository.WatchScanArticle{ID: id, Title: "文章", Summary: "摘要"})
	}
	return out
}

// ── parseWatchAdjudication ──────────────────────────────────────────────

func TestParseWatchAdjudication_RelatedFilteringAndClamp(t *testing.T) {
	sent := adjCandidates(1, 2, 3)
	content := `{"verdicts":[
		{"article_id":1,"related":true,"confidence":0.8,"reason":"贴合"},
		{"article_id":2,"related":false,"confidence":0.9,"reason":"平行事件"},
		{"article_id":3,"related":true,"confidence":5,"reason":"越界"},
		{"article_id":99,"related":true,"confidence":0.7,"reason":"幻觉"}
	],"section_title":"美伊冲突推高油价"}`
	res, err := parseWatchAdjudication(content, sent)
	require.NoError(t, err)
	assert.InEpsilon(t, 0.8, res.Confidence[1], 1e-9)
	assert.NotContains(t, res.Confidence, 2, "related=false excluded")
	assert.InEpsilon(t, 1.0, res.Confidence[3], 1e-9, "confidence >1 clamped to 1")
	assert.NotContains(t, res.Confidence, 99, "hallucinated id filtered")
	assert.Equal(t, "美伊冲突推高油价", res.Title)
}

func TestParseWatchAdjudication_AllRejectedIsLegalEmptyKeep(t *testing.T) {
	sent := adjCandidates(1, 2)
	content := `{"verdicts":[
		{"article_id":1,"related":false,"confidence":0.9,"reason":"无关"},
		{"article_id":2,"related":false,"confidence":0.8,"reason":"无关"}
	],"section_title":""}`
	res, err := parseWatchAdjudication(content, sent)
	require.NoError(t, err, "valid ids all related=false is a legal full reject")
	assert.Empty(t, res.Confidence)
	assert.Empty(t, res.Title, "blank title falls back to the caller's chain")
}

func TestParseWatchAdjudication_AllHallucinatedIsError(t *testing.T) {
	sent := adjCandidates(1)
	res, err := parseWatchAdjudication(`{"verdicts":[{"article_id":99,"related":true,"confidence":0.9}]}`, sent)
	require.Error(t, err, "no valid article id → indistinguishable from garbage → error so caller degrades")
	assert.Nil(t, res)
}

func TestParseWatchAdjudication_BadJSON(t *testing.T) {
	_, err := parseWatchAdjudication("not-json{", adjCandidates(1))
	require.Error(t, err)
}

func TestParseWatchAdjudication_TitleTruncated(t *testing.T) {
	long := strings.Repeat("题", watchSectionTitleRunes+10)
	res, err := parseWatchAdjudication(`{"verdicts":[],"section_title":"`+long+`"}`, adjCandidates(1))
	require.NoError(t, err)
	assert.Len(t, []rune(res.Title), watchSectionTitleRunes)
}

// ── adjudicateWatchArticles ─────────────────────────────────────────────

func TestAdjudicateWatchArticles_HappyPathSingleCall(t *testing.T) {
	chat := &capturingChat{resp: `{"verdicts":[{"article_id":1,"related":true,"confidence":0.9,"reason":"贴合"}],"section_title":"板块标题"}`}
	res, err := adjudicateWatchArticles(context.Background(), 5, 7, "一句话追踪：美伊形势对市场影响", adjCandidates(1, 2), DefaultWatchMaterializeConfig(), chat.f)
	require.NoError(t, err)
	assert.Equal(t, 1, chat.calls, "one batch call per watch")
	assert.Contains(t, chat.lastReq.Messages[1].Content, "美伊形势对市场影响", "intent travels in the user prompt")
	assert.Contains(t, chat.lastReq.Messages[1].Content, "[id:1]", "candidates listed by id")
	assert.InEpsilon(t, 0.9, res.Confidence[1], 1e-9)
	assert.Equal(t, "板块标题", res.Title)
	assert.True(t, chat.lastReq.JSONMode)
}

func TestAdjudicateWatchArticles_CallError(t *testing.T) {
	chat := &capturingChat{err: errors.New("provider down")}
	_, err := adjudicateWatchArticles(context.Background(), 5, 7, "intent", adjCandidates(1), DefaultWatchMaterializeConfig(), chat.f)
	require.Error(t, err)
}

func TestAdjudicateWatchArticles_EmptyCandidatesZeroCalls(t *testing.T) {
	chat := &capturingChat{resp: `{}`}
	res, err := adjudicateWatchArticles(context.Background(), 5, 7, "intent", nil, DefaultWatchMaterializeConfig(), chat.f)
	require.NoError(t, err)
	assert.Zero(t, chat.calls)
	assert.Empty(t, res.Confidence)
}

func TestAdjudicateWatchArticles_CandidateLimitTruncation(t *testing.T) {
	candidates := adjCandidates()
	for i := uint(1); i <= 41; i++ {
		candidates = append(candidates, repository.WatchScanArticle{ID: i, Title: "文章", Summary: "摘要"})
	}
	chat := &capturingChat{resp: `{"verdicts":[]}`}
	_, err := adjudicateWatchArticles(context.Background(), 5, 7, "intent", candidates, WatchMaterializeConfig{Enabled: true, CandidateLimit: 40}, chat.f)
	require.NoError(t, err)
	assert.Equal(t, 1, chat.calls)
	assert.Equal(t, strings.Count(chat.lastReq.Messages[1].Content, "[id:"), 40, "candidates truncated to the limit")
	assert.NotContains(t, chat.lastReq.Messages[1].Content, "[id:41]", "overflow candidate not sent")
}

// ── helpers ─────────────────────────────────────────────────────────────

func TestFilterAdjudicated(t *testing.T) {
	articles := adjCandidates(1, 2, 3)
	adj := &watchAdjudicationResult{Confidence: map[uint]float64{1: 0.9, 3: 0.7}}
	kept := filterAdjudicated(articles, adj)
	require.Len(t, kept, 2)
	assert.Equal(t, uint(1), kept[0].ID)
	assert.Equal(t, uint(3), kept[1].ID)
}

func TestApplyThreadConfidence(t *testing.T) {
	threads := []repository.DailyReportThread{
		{Title: "a", Confidence: 1.0, RelatedArticleIDs: mustMarshalUintArray([]uint{1})},
		{Title: "b", Confidence: 1.0, RelatedArticleIDs: mustMarshalUintArray([]uint{2})},
	}
	adj := &watchAdjudicationResult{Confidence: map[uint]float64{1: 0.65}}
	applyThreadConfidence(threads, adj)
	assert.InEpsilon(t, 0.65, threads[0].Confidence, 1e-9)
	assert.InEpsilon(t, 1.0, threads[1].Confidence, 1e-9, "un-adjudicated article keeps default 1.0")
	applyThreadConfidence(threads, nil) // degraded: no-op, no panic
	assert.InEpsilon(t, 0.65, threads[0].Confidence, 1e-9)
}

func TestParseUintJSONArray(t *testing.T) {
	assert.Nil(t, parseUintJSONArray(repository.JSON("")))
	assert.Nil(t, parseUintJSONArray(repository.JSON("garbage")))
	assert.Equal(t, []uint{3, 5}, parseUintJSONArray(mustMarshalUintArray([]uint{3, 5})))
}
