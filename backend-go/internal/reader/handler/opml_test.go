package handler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/database"
	"syntopica-backend/internal/reader/repository"
)

// ── OPML 导入走共享建源服务（review High 3，最小修复）──
//
// 建源必须经 service.NormalizeSubscriptionURL + FeedCreateService.CreateOrReuseFeed
// （规范化 + 按规范化 URL 短事务查重），禁止直写 models.Feed；但刻意豁免 safefetch
// 网络验证（OPML 为用户自有源清单导入）。用例全部使用不可达的 localhost 地址，
// 且换装 refreshImportedFeed 为空实现——否则后台 goroutine 会在用例结束后继续写
// 全局 repository（GORM Save 带主键是 upsert），污染下一个用例的测试库。

// opmlImportData 是导入响应 data 节的形状（在既有形状上扩展 reused/skipped）。
type opmlImportData struct {
	FeedsAdded      int      `json:"feeds_added"`
	FeedsReused     int      `json:"feeds_reused"`
	FeedsSkipped    int      `json:"feeds_skipped"`
	CategoriesAdded int      `json:"categories_added"`
	Errors          []string `json:"errors"`
	AsyncUpdate     bool     `json:"async_update"`
}

// stubImportedRefresh 换装后台刷新为空实现，并返回已刷新 feedID 的收集器。
func stubImportedRefresh(t *testing.T) *refreshedFeedIDs {
	t.Helper()
	collector := &refreshedFeedIDs{}
	prev := refreshImportedFeed
	refreshImportedFeed = collector.record
	t.Cleanup(func() { refreshImportedFeed = prev })
	return collector
}

type refreshedFeedIDs struct {
	mu  sync.Mutex
	ids []uint
}

func (c *refreshedFeedIDs) record(feedID uint) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ids = append(c.ids, feedID)
}

func (c *refreshedFeedIDs) snapshot() []uint {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]uint(nil), c.ids...)
}

// waitFor 等后台刷新把 expect 次调用全部送达（缺一个就说明后台链路没跑）。
// 每个用例都必须等待——否则未送达的 goroutine 会在下一个用例里读到新换装的
// refreshImportedFeed，把上一个用例的 feedID 记进下一个用例的账上。
func (c *refreshedFeedIDs) waitFor(t *testing.T, expect int) []uint {
	t.Helper()
	require.Eventually(t, func() bool { return len(c.snapshot()) >= expect }, time.Second, 2*time.Millisecond,
		"后台刷新未触发（want %d）", expect)
	ids := c.snapshot()
	require.Len(t, ids, expect)
	return ids
}

func setupOPMLTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := fmt.Sprintf("file:opml_%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.Feed{}, &models.Category{}))
	database.DB = db
	repository.InitRepository(database.DB)
	return db
}

// buildOPML 把 outline 行拼成一份合法的 OPML 文档（每行形如 title|xmlUrl）。
func buildOPML(lines ...string) string {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?><opml version="2.0"><head><title>t</title></head><body>`)
	b.WriteString(`<outline text="Tech">`)
	for _, line := range lines {
		parts := strings.SplitN(line, "|", 2)
		b.WriteString(fmt.Sprintf(`<outline type="rss" text="%s" xmlUrl="%s"/>`, parts[0], parts[1]))
	}
	b.WriteString(`</outline></body></opml>`)
	return b.String()
}

func importOPML(t *testing.T, body string) opmlImportData {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	fw, err := w.CreateFormFile("file", "feeds.opml")
	require.NoError(t, err)
	_, err = fw.Write([]byte(body))
	require.NoError(t, err)
	require.NoError(t, w.Close())

	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/opml/import-opml", &buf)
	ctx.Request.Header.Set("Content-Type", w.FormDataContentType())
	ImportOPML(ctx)

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var resp struct {
		Success bool           `json:"success"`
		Data    opmlImportData `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.True(t, resp.Success)
	return resp.Data
}

func feedURLs(t *testing.T, db *gorm.DB) []string {
	t.Helper()
	var feeds []models.Feed
	require.NoError(t, db.Order("id ASC").Find(&feeds).Error)
	urls := make([]string, 0, len(feeds))
	for _, f := range feeds {
		urls = append(urls, f.URL)
	}
	return urls
}

// 非法 scheme 与超长地址只跳过自己，合法条目照常入库（批次不中断）。
func TestImportOPMLSkipsInvalidEntriesWithoutAbortingBatch(t *testing.T) {
	db := setupOPMLTestDB(t)
	gin.SetMode(gin.TestMode)
	refreshedIDs := stubImportedRefresh(t)

	longURL := "http://localhost:1/" + strings.Repeat("a", 600)
	data := importOPML(t, buildOPML(
		"Ftp|ftp://feed.example.com/rss",
		"TooLong|"+longURL,
		"Good|http://localhost:1/ok.xml",
	))

	require.Equal(t, 1, data.FeedsAdded)
	require.Equal(t, 2, data.FeedsSkipped)
	require.Equal(t, 0, data.FeedsReused)
	require.Len(t, data.Errors, 2, "坏地址必须进失败明细：%v", data.Errors)
	joined := strings.Join(data.Errors, "\n")
	require.Contains(t, joined, "Skipped feed 'Ftp'")
	require.Contains(t, joined, "exceeds 500 runes")
	require.Equal(t, []string{"http://localhost:1/ok.xml"}, feedURLs(t, db))
	require.Equal(t, []uint{1}, refreshedIDs.waitFor(t, 1), "仅新建的 feed 触发后台刷新")
}

// 同地址重复 outline 只建一个 Feed（第二次 reused）；规范化等价地址（host 大小写 /
// 默认端口）归并到同一个 Feed。
func TestImportOPMLDeduplicatesRepeatedAndEquivalentURLs(t *testing.T) {
	db := setupOPMLTestDB(t)
	gin.SetMode(gin.TestMode)
	refreshedIDs := stubImportedRefresh(t)

	data := importOPML(t, buildOPML(
		"Dup1|http://localhost:1/dup.xml",
		"Dup2|http://localhost:1/dup.xml",
		"Eq1|HTTP://LOCALHOST:80/eq.xml",
		"Eq2|http://localhost/eq.xml",
	))

	require.Equal(t, 2, data.FeedsAdded)
	require.Equal(t, 2, data.FeedsReused)
	require.Equal(t, 0, data.FeedsSkipped)
	require.Empty(t, data.Errors)

	urls := feedURLs(t, db)
	require.Len(t, urls, 2)
	require.Contains(t, urls, "http://localhost:1/dup.xml")
	require.Contains(t, urls, "http://localhost/eq.xml", "等价地址必须归并到规范化形式")
	require.Len(t, refreshedIDs.waitFor(t, 2), 2, "两个新建 feed 各刷新一次（reused 不刷新）")
}

// 规范化由共享服务执行（host 小写、默认端口移除、fragment 移除、path 大小写保留），
// 落库字段取共享服务默认值 —— 行为上证明没有直写 models.Feed 的旁路。
func TestImportOPMLNormalizesURLThroughSharedService(t *testing.T) {
	db := setupOPMLTestDB(t)
	gin.SetMode(gin.TestMode)
	refreshed := stubImportedRefresh(t)

	data := importOPML(t, buildOPML("Case|HTTP://LOCALHOST:80/Eq.Xml#frag"))
	require.Equal(t, 1, data.FeedsAdded)

	var feeds []models.Feed
	require.NoError(t, db.Find(&feeds).Error)
	require.Len(t, feeds, 1)
	require.Equal(t, "http://localhost/Eq.Xml", feeds[0].URL,
		"原始（未规范化）地址不得入库")
	require.Equal(t, "Case", feeds[0].Title)

	// 共享服务默认值（buildFeedFromOptions）。
	require.Equal(t, "mdi:rss", feeds[0].Icon)
	require.Equal(t, "fallback", feeds[0].IconSource)
	require.Equal(t, "#8b5cf6", feeds[0].Color)
	require.NotNil(t, feeds[0].CategoryID)

	// 后台刷新只针对新建 feed，且 feedID 为刚落库的主键。
	require.Equal(t, []uint{feeds[0].ID}, refreshed.waitFor(t, 1))
}

// 再次导入同一份 OPML 不产生第二个 Feed：全部走 reused，feeds_added=0 且不触发刷新。
func TestImportOPMLReimportReusesExistingFeeds(t *testing.T) {
	db := setupOPMLTestDB(t)
	gin.SetMode(gin.TestMode)
	refreshed := stubImportedRefresh(t)

	body := buildOPML("Again|http://localhost:1/again.xml")
	first := importOPML(t, body)
	require.Equal(t, 1, first.FeedsAdded)
	refreshed.waitFor(t, 1)

	second := importOPML(t, body)
	require.Equal(t, 0, second.FeedsAdded)
	require.Equal(t, 1, second.FeedsReused)
	require.Empty(t, second.Errors)

	require.Equal(t, []string{"http://localhost:1/again.xml"}, feedURLs(t, db))
	require.Len(t, refreshed.snapshot(), 1, "仅首次导入的新建 feed 触发后台刷新")
}
