package search

// add-article-search tasks 2.6 — GET /api/search handler 编排与响应契约
// 测试（内存 sqlite + 注入替身；test-cases.md 故事 S1 步 1/4/8、S2 步 1/5
// 的 handler 层落点）。PG 专属行为（trigram、pgvector cosine）已在
// repository_pg_test.go / semantic_pg_test.go 覆盖，此处不重复。

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/database"
)

// handlerFakeSemantic 语义路注入替身：err 模拟 SQL 失败（非哨兵——编排层
// 对任何 error 都必须降级）；hits/tags 模拟命中。
type handlerFakeSemantic struct {
	err   error
	hits  []SemanticHit
	tags  []MatchedTag
	calls int
}

func (f *handlerFakeSemantic) Search(_ context.Context, _ string, _ string) ([]SemanticHit, []MatchedTag, error) {
	f.calls++
	if f.err != nil {
		return nil, nil, f.err
	}
	return f.hits, f.tags, nil
}

// setupSearchHandlerTest 构造 sqlite 内存库 + 绑定 database.DB（DefaultDict
// 单例按全局 DB 指针换绑）+ Service（语义/扩词替身与 excerpt 桩注入）。
func setupSearchHandlerTest(t *testing.T, semantic SemanticRunner, expander Expander) *Service {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:search-handler-%s-%d?mode=memory&cache=shared", t.Name(), time.Now().UnixNano())), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.Feed{}, &models.Article{}, &models.TopicTag{}, &models.ArticleTopicTag{}, &models.AISettings{}))
	database.DB = db
	return NewService(db, semantic, expander, func(description, _ string, _ bool) string {
		return "E:" + description
	})
}

func seedHandlerFeed(t *testing.T) uint {
	t.Helper()
	feed := models.Feed{Title: "测试源", URL: fmt.Sprintf("https://example.com/%d", time.Now().UnixNano())}
	require.NoError(t, database.DB.Create(&feed).Error)
	return feed.ID
}

func seedHandlerArticle(t *testing.T, feedID uint, title, description string, archived bool) uint {
	t.Helper()
	article := models.Article{
		FeedID:      feedID,
		Title:       title,
		Description: description,
		Link:        fmt.Sprintf("https://example.com/a/%d", time.Now().UnixNano()),
		Archived:    archived,
	}
	require.NoError(t, database.DB.Create(&article).Error)
	return article.ID
}

// runSearchHandler 以给定 query 参数调 HandleSearch，返回 recorder。
func runSearchHandler(t *testing.T, svc *Service, params url.Values) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/search?"+params.Encode(), http.NoBody)
	svc.HandleSearch(ctx)
	return recorder
}

// searchResponse 响应契约断言结构（test-cases「响应契约断言」节）。
type searchResponse struct {
	Success bool   `json:"success"`
	Error   string `json:"error"`
	Data    struct {
		Results []struct {
			ID         uint   `json:"id"`
			FeedID     uint   `json:"feedId"`
			Title      string `json:"title"`
			Archived   bool   `json:"archived"`
			Excerpt    string `json:"excerpt"`
			HitReasons []struct {
				Type  string  `json:"type"`
				Text  string  `json:"text"`
				Score float64 `json:"score"`
			} `json:"hitReasons"`
		} `json:"results"`
		Semantic struct {
			Skipped bool `json:"skipped"`
			Tags    []struct {
				Label string  `json:"label"`
				Score float64 `json:"score"`
			} `json:"tags"`
		} `json:"semantic"`
		Expand *struct {
			Words []string `json:"words"`
		} `json:"expand"`
		Total   int   `json:"total"`
		Page    int   `json:"page"`
		PerPage int   `json:"perPage"`
		TookMs  int64 `json:"tookMs"`
	} `json:"data"`
}

func decodeSearchResponse(t *testing.T, recorder *httptest.ResponseRecorder) searchResponse {
	t.Helper()
	var body searchResponse
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &body), "body: %s", recorder.Body.String())
	return body
}

// TestSearchHandlerKeywordHitWithReasons covers S1 步 1：关键词命中 +
// type=keyword hit_reasons + 响应契约结构（mock 语义路不可用时同样命中
// ——不依赖语义路径可用性）。
func TestSearchHandlerKeywordHitWithReasons(t *testing.T) {
	semantic := &handlerFakeSemantic{err: fmt.Errorf("pgvector unavailable on sqlite")}
	svc := setupSearchHandlerTest(t, semantic, nil)

	feedID := seedHandlerFeed(t)
	hitID := seedHandlerArticle(t, feedID, "国漫崛起的产业观察", "<p>国漫正当时</p>", false)
	seedHandlerArticle(t, feedID, "无关文章：新能源车销量", "无关导语", false)

	recorder := runSearchHandler(t, svc, url.Values{"q": {"国漫崛起"}})
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())

	body := decodeSearchResponse(t, recorder)
	require.True(t, body.Success)
	require.Equal(t, 1, body.Data.Total)
	require.Len(t, body.Data.Results, 1)

	item := body.Data.Results[0]
	require.Equal(t, hitID, item.ID)
	require.Equal(t, feedID, item.FeedID)
	require.False(t, item.Archived)
	require.Contains(t, item.Title, "国漫")
	// excerpt 经注入的 ExcerptFunc 从有界源字段构建
	require.Equal(t, "E:<p>国漫正当时</p>", item.Excerpt)
	// L2 整串命中：type=keyword，text=查询原文，score>0（title 权重 1.0）
	require.NotEmpty(t, item.HitReasons)
	require.Equal(t, ReasonKeyword, item.HitReasons[0].Type)
	require.Equal(t, "国漫崛起", item.HitReasons[0].Text)
	require.Positive(t, item.HitReasons[0].Score)

	// 契约：semantic 块 + 分页/耗时字段
	require.True(t, body.Data.Semantic.Skipped)
	require.Equal(t, 1, body.Data.Page)
	require.Equal(t, searchDefaultPerPage, body.Data.PerPage)
	require.GreaterOrEqual(t, body.Data.TookMs, int64(0))
	// expand=off：expand 块缺席
	require.Nil(t, body.Data.Expand)
}

// TestSearchHandlerEmptyQueryRejected covers S1 步 4：q 缺失/空串/纯空格/
// 全角空格/tab → 400 且零召回动作（语义替身零调用）。
func TestSearchHandlerEmptyQueryRejected(t *testing.T) {
	semantic := &handlerFakeSemantic{}
	svc := setupSearchHandlerTest(t, semantic, nil)

	for _, q := range []string{"", "   ", "\u3000", "\t", " \u3000\t "} {
		recorder := runSearchHandler(t, svc, url.Values{"q": {q}})
		require.Equal(t, http.StatusBadRequest, recorder.Code, "q=%q", q)
		body := decodeSearchResponse(t, recorder)
		require.False(t, body.Success)
		require.NotEmpty(t, body.Error)
	}
	// q 参数整体缺失
	recorder := runSearchHandler(t, svc, url.Values{})
	require.Equal(t, http.StatusBadRequest, recorder.Code)
	require.Zero(t, semantic.calls, "400 路径不得触发任何召回（语义路零调用）")
}

// TestSearchHandlerInvalidPagination covers S1 步 8：per_page=0 / page=-1 /
// 非整数 → 400 不执行搜索；per_page>100 → 钳 100 + warn（列表端点风格）。
func TestSearchHandlerInvalidPagination(t *testing.T) {
	semantic := &handlerFakeSemantic{}
	svc := setupSearchHandlerTest(t, semantic, nil)
	seedHandlerArticle(t, seedHandlerFeed(t), "特斯拉电池技术", "", false)

	for _, tc := range []struct {
		name   string
		params url.Values
	}{
		{"per_page=0", url.Values{"q": {"特斯拉"}, "per_page": {"0"}}},
		{"page=-1", url.Values{"q": {"特斯拉"}, "page": {"-1"}}},
		{"page=0", url.Values{"q": {"特斯拉"}, "page": {"0"}}},
		{"page=abc", url.Values{"q": {"特斯拉"}, "page": {"abc"}}},
		{"per_page=abc", url.Values{"q": {"特斯拉"}, "per_page": {"abc"}}},
		{"per_page=-5", url.Values{"q": {"特斯拉"}, "per_page": {"-5"}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			recorder := runSearchHandler(t, svc, tc.params)
			require.Equal(t, http.StatusBadRequest, recorder.Code)
		})
	}
	require.Zero(t, semantic.calls, "非法分页不得执行搜索（语义路零调用）")

	// per_page 超上限：钳 100 + warn，200 正常返回
	recorder := runSearchHandler(t, svc, url.Values{"q": {"特斯拉"}, "per_page": {"10000"}})
	require.Equal(t, http.StatusOK, recorder.Code)
	body := decodeSearchResponse(t, recorder)
	require.Equal(t, searchMaxPerPage, body.Data.PerPage)
	require.Equal(t, 1, body.Data.Total)
}

// TestSearchHandlerSemanticDegradedStill200 covers S2 步 5：语义路任何
// error（含非哨兵 SQL 失败）→ semantic.skipped=true + HTTP 200，关键词
// 结果正常（红线 11：增强层失败不得让搜索整体报错）。
func TestSearchHandlerSemanticDegradedStill200(t *testing.T) {
	// 注意：err 是普通 error（非 ErrSemanticSkipped）——编排层对语义路
	// 任何 error 都必须降级而非让端点报错。
	semantic := &handlerFakeSemantic{err: fmt.Errorf("semantic sql exploded")}
	svc := setupSearchHandlerTest(t, semantic, nil)

	feedID := seedHandlerFeed(t)
	seedHandlerArticle(t, feedID, "特斯拉电池技术突破", "", false)

	recorder := runSearchHandler(t, svc, url.Values{"q": {"特斯拉"}})
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())

	body := decodeSearchResponse(t, recorder)
	require.True(t, body.Success)
	require.True(t, body.Data.Semantic.Skipped)
	require.Empty(t, body.Data.Semantic.Tags, "skipped 时不携带 tags 键内容")
	require.Equal(t, 1, body.Data.Total)
	require.Equal(t, ReasonKeyword, body.Data.Results[0].HitReasons[0].Type)
}

// TestSearchHandlerSemanticHitContract covers S2 步 1（handler 层）：语义
// 命中 → hit_reasons 含 type=semantic（标签+cosine）、semantic 块列出
// 命中标签、skipped=false；与关键词榜 RRF 融合。
func TestSearchHandlerSemanticHitContract(t *testing.T) {
	semantic := &handlerFakeSemantic{}
	svc := setupSearchHandlerTest(t, semantic, nil)
	feedID := seedHandlerFeed(t)
	// 关键词路命不中的标题（sqlite 无 trigram）：语义路是唯一召回来源
	semArticleID := seedHandlerArticle(t, feedID, "能量密度与热管理的平衡术", "", false)
	kwArticleID := seedHandlerArticle(t, feedID, "电动车电池安全技术盘点", "", false)

	semantic.hits = []SemanticHit{{
		Article:     ArticleSearchRow{ID: semArticleID, FeedID: feedID, Title: "能量密度与热管理的平衡术"},
		Cosine:      0.82,
		MatchedTags: []MatchedTag{{TagID: 1, Label: "电池安全", Cosine: 0.82}, {TagID: 2, Label: "热管理", Cosine: 0.61}},
	}}
	semantic.tags = []MatchedTag{{TagID: 1, Label: "电池安全", Cosine: 0.82}, {TagID: 2, Label: "热管理", Cosine: 0.61}}

	recorder := runSearchHandler(t, svc, url.Values{"q": {"电动车电池安全"}})
	require.Equal(t, http.StatusOK, recorder.Code)

	body := decodeSearchResponse(t, recorder)
	require.False(t, body.Data.Semantic.Skipped)
	require.Len(t, body.Data.Semantic.Tags, 2)
	require.Equal(t, "电池安全", body.Data.Semantic.Tags[0].Label)
	require.InDelta(t, 0.82, body.Data.Semantic.Tags[0].Score, 1e-9)

	// 两路各召回一篇：语义命中的文章带全量 semantic hit_reasons（标签全进）
	byID := map[uint][]struct {
		Type  string  `json:"type"`
		Text  string  `json:"text"`
		Score float64 `json:"score"`
	}{}
	for _, r := range body.Data.Results {
		byID[r.ID] = r.HitReasons
	}
	require.Contains(t, byID, semArticleID)
	require.Contains(t, byID, kwArticleID)
	semReasons := byID[semArticleID]
	require.Len(t, semReasons, 2)
	require.Equal(t, ReasonSemantic, semReasons[0].Type)
	require.Equal(t, "电池安全", semReasons[0].Text)
	require.InDelta(t, 0.82, semReasons[0].Score, 1e-9)
	require.Equal(t, ReasonKeyword, byID[kwArticleID][0].Type)
}

// TestSearchHandlerSemanticZeroHitsNotSkipped covers S2 变体「前置：零命中
// ≠ 服务不可用」：语义路零命中 → skipped=false + 空标签列表。
func TestSearchHandlerSemanticZeroHitsNotSkipped(t *testing.T) {
	semantic := &handlerFakeSemantic{} // hits/tags nil, err nil → 零命中
	svc := setupSearchHandlerTest(t, semantic, nil)
	seedHandlerArticle(t, seedHandlerFeed(t), "国漫观察", "", false)

	recorder := runSearchHandler(t, svc, url.Values{"q": {"国漫"}})
	require.Equal(t, http.StatusOK, recorder.Code)
	body := decodeSearchResponse(t, recorder)
	require.False(t, body.Data.Semantic.Skipped)
	require.Empty(t, body.Data.Semantic.Tags)
	require.Equal(t, 1, body.Data.Total)
}

// TestSearchHandlerArchivedScopes covers S1 步 6/7（handler 层）：默认含
// 归档（archived 标记随行）、active-only 过滤、非法枚举回退默认。
func TestSearchHandlerArchivedScopes(t *testing.T) {
	svc := setupSearchHandlerTest(t, &handlerFakeSemantic{}, nil)
	feedID := seedHandlerFeed(t)
	activeID := seedHandlerArticle(t, feedID, "特斯拉新车发布", "", false)
	archivedID := seedHandlerArticle(t, feedID, "特斯拉历史财报", "", true)

	// 默认 include：两篇均在，归档行带标记
	recorder := runSearchHandler(t, svc, url.Values{"q": {"特斯拉"}})
	require.Equal(t, http.StatusOK, recorder.Code)
	body := decodeSearchResponse(t, recorder)
	require.Equal(t, 2, body.Data.Total)
	archivedSeen := map[uint]bool{}
	for _, r := range body.Data.Results {
		archivedSeen[r.ID] = r.Archived
	}
	require.False(t, archivedSeen[activeID])
	require.True(t, archivedSeen[archivedID])

	// active-only：归档不出现
	recorder = runSearchHandler(t, svc, url.Values{"q": {"特斯拉"}, "archived": {"active-only"}})
	require.Equal(t, http.StatusOK, recorder.Code)
	body = decodeSearchResponse(t, recorder)
	require.Equal(t, 1, body.Data.Total)
	require.Equal(t, activeID, body.Data.Results[0].ID)

	// 非法枚举回退默认 include（不报错）
	recorder = runSearchHandler(t, svc, url.Values{"q": {"特斯拉"}, "archived": {"bogus"}})
	require.Equal(t, http.StatusOK, recorder.Code)
	body = decodeSearchResponse(t, recorder)
	require.Equal(t, 2, body.Data.Total)
}

// TestSearchHandlerPaginationSlice covers offset 分页切片：越页空结果不报错。
func TestSearchHandlerPaginationSlice(t *testing.T) {
	svc := setupSearchHandlerTest(t, &handlerFakeSemantic{}, nil)
	feedID := seedHandlerFeed(t)
	for i := 0; i < 3; i++ {
		seedHandlerArticle(t, feedID, fmt.Sprintf("国漫产业报告第%d篇", i), "", false)
	}

	body := decodeSearchResponse(t, runSearchHandler(t, svc, url.Values{"q": {"国漫"}, "per_page": {"2"}, "page": {"2"}}))
	require.Equal(t, 3, body.Data.Total)
	require.Equal(t, 2, body.Data.Page)
	require.Equal(t, 2, body.Data.PerPage)
	require.Len(t, body.Data.Results, 1)

	body = decodeSearchResponse(t, runSearchHandler(t, svc, url.Values{"q": {"国漫"}, "per_page": {"2"}, "page": {"5"}}))
	require.Equal(t, 3, body.Data.Total)
	require.Empty(t, body.Data.Results)
}

// TestSearchHandlerExpandHappyPath covers S2 步 4：expand=on 且 LLM 正常 →
// 扩词补跑字面召回、命中带 type=expand hit_reasons、expand.words 回显、
// 与主结果融合排序。
func TestSearchHandlerExpandHappyPath(t *testing.T) {
	expander := &fakeExpander{words: []string{"电池热失控"}}
	svc := setupSearchHandlerTest(t, &handlerFakeSemantic{}, expander)
	feedID := seedHandlerFeed(t)
	// 关键词路（整串/词典/多词）命不中的标题：只有扩展词能召回
	expandID := seedHandlerArticle(t, feedID, "地库事件复盘：电池热失控链路", "", false)
	seedHandlerArticle(t, feedID, "完全无关的文章", "", false)

	recorder := runSearchHandler(t, svc, url.Values{"q": {"电动车自燃"}, "expand": {"on"}})
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())

	body := decodeSearchResponse(t, recorder)
	require.NotNil(t, body.Data.Expand)
	require.Equal(t, []string{"电池热失控"}, body.Data.Expand.Words)
	require.Equal(t, 1, body.Data.Total)
	require.Equal(t, expandID, body.Data.Results[0].ID)
	require.Len(t, body.Data.Results[0].HitReasons, 1)
	require.Equal(t, ReasonExpand, body.Data.Results[0].HitReasons[0].Type)
	require.Equal(t, "电池热失控", body.Data.Results[0].HitReasons[0].Text)
	require.Equal(t, 1, expander.calls)
}

// TestSearchHandlerExpandLLMFailure covers S2 步 5：expand=on 且 LLM 失败 →
// 主结果 200 正常、扩展区空词表（expand 块在场、words 空）。
func TestSearchHandlerExpandLLMFailure(t *testing.T) {
	expander := &fakeExpander{err: fmt.Errorf("llm down")}
	svc := setupSearchHandlerTest(t, &handlerFakeSemantic{}, expander)
	feedID := seedHandlerFeed(t)
	seedHandlerArticle(t, feedID, "国漫观察", "", false)

	recorder := runSearchHandler(t, svc, url.Values{"q": {"国漫"}, "expand": {"on"}})
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())

	body := decodeSearchResponse(t, recorder)
	require.True(t, body.Success)
	require.Equal(t, 1, body.Data.Total)
	require.Equal(t, ReasonKeyword, body.Data.Results[0].HitReasons[0].Type)
	require.NotNil(t, body.Data.Expand, "expand=on 时块在场（区分于 off 的缺席）")
	require.Empty(t, body.Data.Expand.Words)
}

// TestSearchHandlerExpandCacheHit covers S1 变体「幂等」：10min TTL 内同
// query 重复 expand → 缓存命中，LLM 替身零重复调用（第二次调用计数不变）。
func TestSearchHandlerExpandCacheHit(t *testing.T) {
	expander := &fakeExpander{words: []string{"电池热失控"}}
	svc := setupSearchHandlerTest(t, &handlerFakeSemantic{}, expander)
	seedHandlerArticle(t, seedHandlerFeed(t), "电池热失控复盘", "", false)

	params := url.Values{"q": {"电动车自燃"}, "expand": {"on"}}
	first := decodeSearchResponse(t, runSearchHandler(t, svc, params))
	require.Equal(t, 1, expander.calls)

	second := decodeSearchResponse(t, runSearchHandler(t, svc, params))
	require.Equal(t, 1, expander.calls, "TTL 缓存命中：第二次零 LLM 调用")
	require.Equal(t, first.Data.Expand.Words, second.Data.Expand.Words)
	require.Equal(t, first.Data.Total, second.Data.Total)
}

// TestSearchHandlerExpandIllegalValueFallsBack covers expand 非法枚举回退
// off（不报错、不触发 LLM）。
func TestSearchHandlerExpandIllegalValueFallsBack(t *testing.T) {
	expander := &fakeExpander{words: []string{"电池热失控"}}
	svc := setupSearchHandlerTest(t, &handlerFakeSemantic{}, expander)
	seedHandlerArticle(t, seedHandlerFeed(t), "国漫观察", "", false)

	recorder := runSearchHandler(t, svc, url.Values{"q": {"国漫"}, "expand": {"yes"}})
	require.Equal(t, http.StatusOK, recorder.Code, "expand 非法值回退 off 不报错")
	body := decodeSearchResponse(t, recorder)
	require.Nil(t, body.Data.Expand)
	require.Zero(t, expander.calls)
}
