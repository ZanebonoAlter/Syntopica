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
	// weakIDs 是三条弱证据文章（未打标 / max_sim / downgraded）。
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
	downTag := seedTag("down", "", tagmodels.TopicTagBoardLabel{Score: 0.4, MatchReason: "hit_rate", Downgraded: true})
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

	// 1 未打标（仅方向未通过标签）；2 max_sim；3 降级；4..20 强证据。
	fixture.weakIDs = append(fixture.weakIDs, makeArticle("untagged", mismatchTag))
	fixture.weakIDs = append(fixture.weakIDs, makeArticle("sim", simTag))
	fixture.weakIDs = append(fixture.weakIDs, makeArticle("downgraded", downTag))
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
	require.Len(t, rows, 3)
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

	seed := func(slug, reason string, score float64, downgraded bool) models.TopicTag {
		tag := models.TopicTag{Slug: fmt.Sprintf("%s-%d", slug, time.Now().UnixNano()), Label: slug, Status: "active", IsCanonical: true}
		require.NoError(t, db.Create(&tag).Error)
		require.NoError(t, db.Create(&tagmodels.TopicTagBoardLabel{TopicTagID: tag.ID, SemanticBoardID: board.ID, Score: score, MatchReason: reason, Downgraded: downgraded}).Error)
		return tag
	}
	directA := seed("direct-a", "direct_hit", 0.9, false)
	directB := seed("direct-b", "direct_hit", 0.8, false)
	rateTag := seed("rate", "hit_rate", 0.85, false)
	simA := seed("sim-a", "max_sim", 0.7, false)
	simB := seed("sim-b", "max_sim", 0.6, false)
	weightHigh := seed("weight-high", "weighted", 0.95, false) // 高分但 tier 最差：锁 tier 优先于分数
	weightLow := seed("weight-low", "weighted", 0.5, false)

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
