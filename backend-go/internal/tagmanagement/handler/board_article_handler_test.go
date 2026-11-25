package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"syntopica-backend/internal/models"
	tagmodels "syntopica-backend/internal/tagmanagement/models"
	"syntopica-backend/internal/tagmanagement/repository"
)

// 用例来源：openspec/changes/rework-workshop-into-radar-and-desk/test-cases.md
// S2 步 11–13（B-7 弱证据过滤）。SQLite 内存库，走完整路由注册路径。

type archiveTestBoardReport struct {
	ID              uint `gorm:"primarykey"`
	SemanticBoardID uint `gorm:"not null;index"`
	PeriodDate      time.Time
}

func (archiveTestBoardReport) TableName() string { return "board_daily_reports" }

type archiveTestSection struct {
	ID       uint `gorm:"primarykey"`
	ReportID uint `gorm:"index;not null"`
}

func (archiveTestSection) TableName() string { return "daily_report_sections" }

type archiveTestThread struct {
	ID                uint `gorm:"primarykey"`
	SectionID         uint `gorm:"index;not null"`
	RelatedArticleIDs string
}

func (archiveTestThread) TableName() string { return "daily_report_threads" }

type archiveFixture struct {
	db      *gorm.DB
	router  *gin.Engine
	boardID uint
	// weakIDs 是两条弱证据文章（未打标 / max_sim）。downgraded 标记已退役
	// （2026-11-12）：hit_rate 一律强证据，原降级标记行（hit_rate+降级，现实中
	// 不存在的组合）不再算弱。
	weakIDs []uint
	// allIDs 按创建顺序。
	allIDs []uint
}

// archiveStrongTagDescription 是强证据标签的描述，用于验证 description
// 随 filtered_tags 返回（scenario：标签描述随标签返回）。
const archiveStrongTagDescription = "强证据标签的描述文本"

func setupArchiveFixture(t *testing.T) *archiveFixture {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:board_archive_%d?mode=memory&cache=shared", time.Now().UnixNano())), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&models.Feed{},
		&models.Article{},
		&models.TopicTag{},
		&models.ArticleTopicTag{},
		&models.SemanticLabel{},
		&tagmodels.TopicTagBoardLabel{},
		&models.TagJob{},
		&archiveTestBoardReport{},
		&archiveTestSection{},
		&archiveTestThread{},
	))
	repository.InitRepository(db)

	board := models.SemanticLabel{
		Label:     "测试版块",
		Slug:      fmt.Sprintf("archive-board-%d", time.Now().UnixNano()),
		LabelType: "board",
		Status:    "active",
	}
	require.NoError(t, db.Create(&board).Error)

	feed := models.Feed{Title: "测试源", URL: fmt.Sprintf("https://example.com/%d.xml", time.Now().UnixNano())}
	require.NoError(t, db.Create(&feed).Error)

	// 四个标签分别代表强证据 / 相似度 / 降级 / 方向未通过。
	seedTag := func(slug string, desc string, link tagmodels.TopicTagBoardLabel) models.TopicTag {
		tag := models.TopicTag{
			Slug:        fmt.Sprintf("%s-%d", slug, time.Now().UnixNano()),
			Label:       slug,
			Description: desc,
			Status:      "active",
			IsCanonical: true,
		}
		require.NoError(t, db.Create(&tag).Error)
		link.TopicTagID = tag.ID
		link.SemanticBoardID = board.ID
		require.NoError(t, db.Create(&link).Error)
		return tag
	}
	strongTag := seedTag("strong", archiveStrongTagDescription, tagmodels.TopicTagBoardLabel{Score: 0.9, MatchReason: "direct_hit"})
	simTag := seedTag("sim", "", tagmodels.TopicTagBoardLabel{Score: 0.55, MatchReason: "max_sim"})
	hitRateTag := seedTag("hitrate", "", tagmodels.TopicTagBoardLabel{Score: 0.4, MatchReason: "hit_rate"})
	mismatchTag := seedTag("mismatch", "", tagmodels.TopicTagBoardLabel{Score: 0.3, MatchReason: "direct_hit", DirectionMismatch: true})

	fixture := &archiveFixture{db: db, boardID: board.ID}
	pubDate := time.Date(2026, 10, 20, 8, 0, 0, 0, time.UTC)
	makeArticle := func(title string, tags ...models.TopicTag) uint {
		article := models.Article{
			FeedID:  feed.ID,
			Title:   title,
			Link:    fmt.Sprintf("https://example.com/%s-%d", title, time.Now().UnixNano()),
			PubDate: &pubDate,
		}
		require.NoError(t, db.Create(&article).Error)
		for _, tag := range tags {
			require.NoError(t, db.Create(&models.ArticleTopicTag{
				ArticleID:  article.ID,
				TopicTagID: tag.ID,
				Score:      0.9,
			}).Error)
		}
		fixture.allIDs = append(fixture.allIDs, article.ID)
		return article.ID
	}

	// 1 未打标（仅方向未通过标签）；2 max_sim；3 hit_rate 命中；4..20 强证据。
	fixture.weakIDs = append(fixture.weakIDs, makeArticle("untagged", mismatchTag))
	fixture.weakIDs = append(fixture.weakIDs, makeArticle("sim", simTag))
	makeArticle("hitrate", hitRateTag) // hit_rate：强证据，不进 weak 列表
	for i := 4; i <= 20; i++ {
		makeArticle(fmt.Sprintf("a-%d", i), strongTag)
	}

	// 两份日报，其中第一份的分区线索覆盖前 12 篇。
	report := archiveTestBoardReport{SemanticBoardID: board.ID, PeriodDate: time.Date(2026, 10, 22, 0, 0, 0, 0, time.UTC)}
	require.NoError(t, db.Create(&report).Error)
	section := archiveTestSection{ReportID: report.ID}
	require.NoError(t, db.Create(&section).Error)
	refs, err := json.Marshal(fixture.allIDs[:12])
	require.NoError(t, err)
	require.NoError(t, db.Create(&archiveTestThread{SectionID: section.ID, RelatedArticleIDs: string(refs)}).Error)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	RegisterSemanticBoardRoutes(router.Group("/api"))
	fixture.router = router
	return fixture
}

func (f *archiveFixture) get(t *testing.T, query string) []map[string]any {
	t.Helper()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/semantic-boards/%d/articles?%s", f.boardID, query), nil)
	f.router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var body struct {
		Success    bool             `json:"success"`
		Data       []map[string]any `json:"data"`
		Pagination struct {
			Total int `json:"total"`
		} `json:"pagination"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	return body.Data
}

const archiveWindowQuery = "start_date=2026-10-14&end_date=2026-10-23&per_page=100"

func TestBoardArticles_InReportMarker(t *testing.T) {
	f := setupArchiveFixture(t)
	rows := f.get(t, archiveWindowQuery)
	require.Len(t, rows, 20)
	in, out := 0, 0
	for _, row := range rows {
		require.NotNil(t, row["filtered_tags"], "filtered_tags 必须是 []，不能是 null")
		if row["in_report"] == true {
			in++
		} else if row["in_report"] == false {
			out++
		} else {
			t.Fatalf("missing in_report field: %v", row)
		}
	}
	require.Equal(t, 12, in)
	require.Equal(t, 8, out)
}

func TestBoardArticles_InReportIncludesStartDate(t *testing.T) {
	f := setupArchiveFixture(t)
	// 只在窗口起始日（10-14）的日报线索里引用第 20 篇，闭区间必须计入。
	report := archiveTestBoardReport{SemanticBoardID: f.boardID, PeriodDate: time.Date(2026, 10, 14, 0, 0, 0, 0, time.UTC)}
	require.NoError(t, f.db.Create(&report).Error)
	section := archiveTestSection{ReportID: report.ID}
	require.NoError(t, f.db.Create(&section).Error)
	refs, err := json.Marshal([]uint{f.allIDs[19]})
	require.NoError(t, err)
	require.NoError(t, f.db.Create(&archiveTestThread{SectionID: section.ID, RelatedArticleIDs: string(refs)}).Error)

	rows := f.get(t, "start_date=2026-10-14&end_date=2026-10-23&per_page=100")
	var target map[string]any
	for _, row := range rows {
		if uint(row["id"].(float64)) == f.allIDs[19] {
			target = row
			break
		}
	}
	require.NotNil(t, target)
	require.Equal(t, true, target["in_report"])
}

func TestBoardArticles_NotInReportFilter(t *testing.T) {
	f := setupArchiveFixture(t)
	rows := f.get(t, archiveWindowQuery+"&report_status=not_in_report")
	require.Len(t, rows, 8)
	for _, row := range rows {
		require.Equal(t, false, row["in_report"], row["id"])
	}

	// 旧参数别名等价
	alias := f.get(t, archiveWindowQuery+"&in_report=false")
	require.Len(t, alias, 8)
}

func TestBoardArticles_WeakFilter(t *testing.T) {
	f := setupArchiveFixture(t)
	rows := f.get(t, archiveWindowQuery+"&report_status=weak")
	require.Len(t, rows, 2)
	got := make([]uint, 0, len(rows))
	for _, row := range rows {
		got = append(got, uint(row["id"].(float64)))
	}
	require.ElementsMatch(t, f.weakIDs, got)
}

func TestBoardArticles_FilteredTagDescription(t *testing.T) {
	f := setupArchiveFixture(t)
	rows := f.get(t, archiveWindowQuery)
	byID := make(map[uint]map[string]any, len(rows))
	for _, row := range rows {
		byID[uint(row["id"].(float64))] = row
	}

	// 有描述的标签：description 原文随 filtered_tags 返回。
	strongTags := byID[f.allIDs[3]]["filtered_tags"].([]any)
	require.NotEmpty(t, strongTags)
	strongTag := strongTags[0].(map[string]any)
	require.Equal(t, "strong", strongTag["label"])
	require.Equal(t, archiveStrongTagDescription, strongTag["description"])

	// 无描述的标签：字段存在且为空串（不是字段缺失）。
	simTags := byID[f.allIDs[1]]["filtered_tags"].([]any)
	require.NotEmpty(t, simTags)
	simTag := simTags[0].(map[string]any)
	require.Equal(t, "sim", simTag["label"])
	require.Contains(t, simTag, "description")
	require.Equal(t, "", simTag["description"])
}

// TestBoardArticles_QualitySortGlobalOrder 锁 R-02 修复：sort=quality 是全局序（排序键
// 下推 SQL 后先 ORDER BY 再分页），跨页边界稳定；tier 优先于分数（weighted 0.95
// 不能压过 direct_hit 0.8）；sort=time 为全局 pub_date DESC。
func TestBoardArticles_QualitySortGlobalOrder(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:board_archive_qsort_%d?mode=memory&cache=shared", time.Now().UnixNano())), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&models.Feed{}, &models.Article{}, &models.TopicTag{}, &models.ArticleTopicTag{},
		&models.SemanticLabel{}, &tagmodels.TopicTagBoardLabel{}, &models.TagJob{},
		&archiveTestBoardReport{}, &archiveTestSection{}, &archiveTestThread{},
	))
	repository.InitRepository(db)

	board := models.SemanticLabel{Label: "排序版块", Slug: fmt.Sprintf("qsort-board-%d", time.Now().UnixNano()), LabelType: "board", Status: "active"}
	require.NoError(t, db.Create(&board).Error)
	feed := models.Feed{Title: "排序源", URL: fmt.Sprintf("https://example.com/qsort-%d.xml", time.Now().UnixNano())}
	require.NoError(t, db.Create(&feed).Error)

	seed := func(slug, reason string, score float64) models.TopicTag {
		tag := models.TopicTag{Slug: fmt.Sprintf("%s-%d", slug, time.Now().UnixNano()), Label: slug, Status: "active", IsCanonical: true}
		require.NoError(t, db.Create(&tag).Error)
		require.NoError(t, db.Create(&tagmodels.TopicTagBoardLabel{TopicTagID: tag.ID, SemanticBoardID: board.ID, Score: score, MatchReason: reason}).Error)
		return tag
	}
	directA := seed("direct-a", "direct_hit", 0.9)
	directB := seed("direct-b", "direct_hit", 0.8)
	rateTag := seed("rate", "hit_rate", 0.85)
	simA := seed("sim-a", "max_sim", 0.7)
	simB := seed("sim-b", "max_sim", 0.6)
	weightHigh := seed("weight-high", "weighted", 0.95) // 高分但 tier 最差：锁 tier 优先于分数
	weightLow := seed("weight-low", "weighted", 0.5)

	base := time.Date(2026, 10, 20, 8, 0, 0, 0, time.UTC)
	makeArticle := func(title string, offsetHours int, tags ...models.TopicTag) uint {
		date := base.Add(time.Duration(offsetHours) * time.Hour)
		article := models.Article{FeedID: feed.ID, Title: title, Link: fmt.Sprintf("https://example.com/q-%s-%d", title, time.Now().UnixNano()), PubDate: &date}
		require.NoError(t, db.Create(&article).Error)
		for _, tag := range tags {
			require.NoError(t, db.Create(&models.ArticleTopicTag{ArticleID: article.ID, TopicTagID: tag.ID, Score: 0.9}).Error)
		}
		return article.ID
	}
	// 创建顺序故意与期望质量序无关：weighted 在前、direct 在后。
	idWeightHigh := makeArticle("weight-high", 1, weightHigh)
	makeArticle("sim-a", 2, simA)
	makeArticle("direct-a", 3, directA)
	makeArticle("weight-low", 4, weightLow)
	makeArticle("rate", 5, rateTag)
	makeArticle("sim-b", 6, simB)
	makeArticle("direct-b", 7, directB)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	RegisterSemanticBoardRoutes(router.Group("/api"))
	fetch := func(query string) []map[string]any {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/semantic-boards/%d/articles?%s", board.ID, query), nil)
		router.ServeHTTP(w, req)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		var body struct {
			Data []map[string]any `json:"data"`
		}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
		return body.Data
	}
	titleOf := func(rows []map[string]any) []string {
		out := make([]string, 0, len(rows))
		for _, r := range rows {
			out = append(out, r["title"].(string))
		}
		return out
	}

	// per_page=3 分三页拉 quality：断言拼接后的全局序（含跨页边界）。
	var got []string
	for page := 1; page <= 3; page++ {
		got = append(got, titleOf(fetch(fmt.Sprintf("sort=quality&per_page=3&page=%d", page)))...)
	}
	require.Equal(t, []string{"direct-a", "direct-b", "rate", "sim-a", "sim-b", "weight-high", "weight-low"}, got,
		"quality 全局序应按 tier→score，weighted 0.95 不得压过 direct 0.8；跨页边界不得回退")

	// time 序：全局 pub_date DESC（offset 越大越新）。
	timeRows := titleOf(fetch("sort=time&per_page=3&page=1"))
	require.Equal(t, []string{"direct-b", "sim-b", "rate"}, timeRows)
	_ = idWeightHigh
}

func TestBoardArticles_InvalidReportStatus(t *testing.T) {
	f := setupArchiveFixture(t)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/semantic-boards/%d/articles?report_status=bogus", f.boardID), nil)
	f.router.ServeHTTP(w, req)
	require.Equal(t, http.StatusBadRequest, w.Code)
	require.Contains(t, w.Body.String(), "report_status")
}

func TestBoardArticles_InvalidInReportAlias(t *testing.T) {
	f := setupArchiveFixture(t)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/semantic-boards/%d/articles?in_report=xx", f.boardID), nil)
	f.router.ServeHTTP(w, req)
	require.Equal(t, http.StatusBadRequest, w.Code)
	require.Contains(t, w.Body.String(), "in_report")
}

// ── 版块文章按日计数（archive-time-chart-range） ─────────────────────────────
// 端点口径 MUST 与 /articles 主查询逐字一致：同一 boardTagIDs 取法、feeds 内连接、
// DATE(pub_date) 归日、COUNT(DISTINCT a.id) 去重；固定口径（不吃任何筛选参数）。

// boardDailyCountsResponse 是按日计数端点的响应体（测试只读三个字段）。
type boardDailyCountsResponse struct {
	Success bool `json:"success"`
	Data    struct {
		EarliestDate *string `json:"earliest_date"`
		LatestDate   *string `json:"latest_date"`
		Days         []struct {
			Date  string `json:"date"`
			Count int    `json:"count"`
		} `json:"days"`
	} `json:"data"`
}

// dailyCounts 请求按日计数端点（query 非空时附加，固定口径端点会忽略这些参数）。
func (f *archiveFixture) dailyCounts(t *testing.T, boardID uint, query string) boardDailyCountsResponse {
	t.Helper()
	url := fmt.Sprintf("/api/semantic-boards/%d/article-daily-counts", boardID)
	if query != "" {
		url += "?" + query
	}
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, url, nil)
	f.router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var body boardDailyCountsResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	require.True(t, body.Success)
	return body
}

// articlesTotal 取归档列表的当天篇数（与按日计数对账用）。
func (f *archiveFixture) articlesTotal(t *testing.T, query string) int {
	t.Helper()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/semantic-boards/%d/articles?%s", f.boardID, query), nil)
	f.router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var body struct {
		Pagination struct {
			Total int `json:"total"`
		} `json:"pagination"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	return body.Pagination.Total
}

// makeArticleAt 在指定发布日期造一篇命中给定标签（label）的文章。
func (f *archiveFixture) makeArticleAt(t *testing.T, pubDate time.Time, tagLabels ...string) uint {
	t.Helper()
	var feed models.Feed
	require.NoError(t, f.db.First(&feed).Error)
	article := models.Article{
		FeedID:  feed.ID,
		Title:   fmt.Sprintf("daily-%d", time.Now().UnixNano()),
		Link:    fmt.Sprintf("https://example.com/daily-%d", time.Now().UnixNano()),
		PubDate: &pubDate,
	}
	require.NoError(t, f.db.Create(&article).Error)
	for _, label := range tagLabels {
		var tag models.TopicTag
		require.NoError(t, f.db.Where("label = ?", label).First(&tag).Error)
		require.NoError(t, f.db.Create(&models.ArticleTopicTag{
			ArticleID:  article.ID,
			TopicTagID: tag.ID,
			Score:      0.9,
		}).Error)
	}
	return article.ID
}

// countOf 取某日的计数（缺日 = 稀疏数组里没有该日 = 0）。
func countOf(body boardDailyCountsResponse, date string) int {
	for _, day := range body.Data.Days {
		if day.Date == date {
			return day.Count
		}
	}
	return 0
}

// TestBoardArticleDailyCounts_MatchesArticlesTotal 锁口径不变量：按日计数与
// /articles 的当天篇数逐日相等；同一篇文章命中多个版块标签只计一次。
func TestBoardArticleDailyCounts_MatchesArticlesTotal(t *testing.T) {
	f := setupArchiveFixture(t)
	// 同文多标签：命中 strong + hitrate 两个标签 → 当日只计 1 篇。
	f.makeArticleAt(t, time.Date(2026, 10, 25, 8, 0, 0, 0, time.UTC), "strong", "hitrate")

	body := f.dailyCounts(t, f.boardID, "")
	require.NotNil(t, body.Data.EarliestDate)
	require.NotNil(t, body.Data.LatestDate)
	require.Equal(t, "2026-10-20", *body.Data.EarliestDate)
	require.Equal(t, "2026-10-25", *body.Data.LatestDate)

	require.Equal(t, 1, countOf(body, "2026-10-25"))
	require.Equal(t, 1, f.articlesTotal(t, "start_date=2026-10-25&end_date=2026-10-25&per_page=100"))
	require.Equal(t, 20, countOf(body, "2026-10-20"))
	require.Equal(t, 20, f.articlesTotal(t, "start_date=2026-10-20&end_date=2026-10-20&per_page=100"))

	// 固定口径：feed_id / report_status / 日期窗口都不参与该端点，结果与不带一致。
	withParams := f.dailyCounts(t, f.boardID, "feed_id=1&report_status=weak&start_date=2026-10-25&end_date=2026-10-25")
	require.Equal(t, body.Data.Days, withParams.Data.Days)
}

// TestBoardArticleDailyCounts_SparseAcrossDays 锁稀疏语义：只含 count>0 的日期，
// 按 date 升序；中间没文章的日期不出现在数组里。
func TestBoardArticleDailyCounts_SparseAcrossDays(t *testing.T) {
	f := setupArchiveFixture(t)
	f.makeArticleAt(t, time.Date(2026, 10, 25, 8, 0, 0, 0, time.UTC), "strong")

	body := f.dailyCounts(t, f.boardID, "")
	require.Len(t, body.Data.Days, 2)
	require.Equal(t, "2026-10-20", body.Data.Days[0].Date)
	require.Equal(t, 20, body.Data.Days[0].Count)
	require.Equal(t, "2026-10-25", body.Data.Days[1].Date)
	require.Equal(t, 1, body.Data.Days[1].Count)
}

// TestBoardArticleDailyCounts_CountsDirectionMismatch 方向校验未通过的文章（列表行内
// 默认隐藏其标签）仍计入当日 count——与列表「当天篇数」对齐。
func TestBoardArticleDailyCounts_CountsDirectionMismatch(t *testing.T) {
	f := setupArchiveFixture(t)
	f.makeArticleAt(t, time.Date(2026, 10, 25, 8, 0, 0, 0, time.UTC), "mismatch")

	body := f.dailyCounts(t, f.boardID, "")
	require.Equal(t, 1, countOf(body, "2026-10-25"))
	require.Equal(t, 1, f.articlesTotal(t, "start_date=2026-10-25&end_date=2026-10-25&per_page=100"))
}

// TestBoardArticleDailyCounts_NilPubDate 存量无发布日期的文章不得把整个端点打成 500：
// `pub_date IS NULL` 的命中文章被排除（按日曲线无法给它定位到某一天），其余日期照常返回。
func TestBoardArticleDailyCounts_NilPubDate(t *testing.T) {
	f := setupArchiveFixture(t)
	var feed models.Feed
	require.NoError(t, f.db.First(&feed).Error)
	var tag models.TopicTag
	require.NoError(t, f.db.Where("label = ?", "strong").First(&tag).Error)
	article := models.Article{
		FeedID: feed.ID,
		Title:  "no-pub-date",
		Link:   fmt.Sprintf("https://example.com/no-pub-date-%d", time.Now().UnixNano()),
	}
	require.NoError(t, f.db.Create(&article).Error)
	require.NoError(t, f.db.Create(&models.ArticleTopicTag{
		ArticleID:  article.ID,
		TopicTagID: tag.ID,
		Score:      0.9,
	}).Error)

	body := f.dailyCounts(t, f.boardID, "")
	require.NotNil(t, body.Data.EarliestDate)
	for _, day := range body.Data.Days {
		require.NotEmpty(t, day.Date)
	}
	// 固定 fixture 的既有文章仍照常计入，NULL 行不额外产生日期。
	require.Equal(t, 20, countOf(body, "2026-10-20"))
}

// TestBoardArticleDailyCounts_EmptyBoard 无命中文章：200 + earliest/latest 为 null +
// days 为空数组（不是 404）。
func TestBoardArticleDailyCounts_EmptyBoard(t *testing.T) {
	f := setupArchiveFixture(t)
	body := f.dailyCounts(t, f.boardID+9999, "")
	require.Nil(t, body.Data.EarliestDate)
	require.Nil(t, body.Data.LatestDate)
	require.NotNil(t, body.Data.Days)
	require.Empty(t, body.Data.Days)
}

// TestBoardArticles_AIDetectionFields 覆盖 add-ai-generated-article-detection
// task 4.3 / board-article-api spec：版块文章列表每行携带疑似 AI 判定状态
// （未判定也带该字段，值为 unknown），且该字段不改变返回成员与排序。
func TestBoardArticles_AIDetectionFields(t *testing.T) {
	f := setupArchiveFixture(t)

	rowIDs := func(rows []map[string]any) []uint {
		ids := make([]uint, 0, len(rows))
		for _, row := range rows {
			ids = append(ids, uint(row["id"].(float64)))
		}
		return ids
	}

	baseline := f.get(t, archiveWindowQuery)
	require.NotEmpty(t, baseline)
	baselineIDs := rowIDs(baseline)
	for _, row := range baseline {
		require.Contains(t, row, "ai_detection_status", "未判定文章也必须带判定字段")
		require.Equal(t, models.AIDetectionStatusUnknown, row["ai_detection_status"])
	}

	// 把其中一篇（第 5 行）改成疑似 AI 生成，列表成员与顺序必须不变。
	targetID := baselineIDs[4]
	require.NoError(t, f.db.Model(&models.Article{}).Where("id = ?", targetID).
		Updates(map[string]any{
			"ai_detection_status": models.AIDetectionStatusSuspect,
			"ai_detection_score":  0.88,
			"ai_detection_source": models.AIDetectionSourceLLM,
		}).Error)

	after := f.get(t, archiveWindowQuery)
	require.Equal(t, baselineIDs, rowIDs(after), "判定字段不得改变成员与排序")
	byID := make(map[uint]map[string]any, len(after))
	for _, row := range after {
		byID[uint(row["id"].(float64))] = row
	}
	require.Equal(t, models.AIDetectionStatusSuspect, byID[targetID]["ai_detection_status"])
	require.Equal(t, 0.88, byID[targetID]["ai_detection_score"])
	for _, id := range baselineIDs {
		if id == targetID {
			continue
		}
		require.Equal(t, models.AIDetectionStatusUnknown, byID[id]["ai_detection_status"])
	}
}
