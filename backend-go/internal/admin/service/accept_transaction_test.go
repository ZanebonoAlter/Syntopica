package service

import (
	"context"
	"fmt"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/database"
	"syntopica-backend/internal/platform/safefetch"
	readersvc "syntopica-backend/internal/reader/service"
)

// ── improve-discovery-recommendations 4.5：accept 安全验证 + 短事务去重 ──
//
// sqlite 内存库只迁移本切片相关表；safefetch 只允许外网可达，单测必须注入 mock
// 抓取（readersvc.SetSubscriptionFetcher）。覆盖：原生/填参成功建源、验证失败不建源
// 不标 accepted、HTTP200 非 RSS 拒、私网错误脱敏、重复地址仅一个 Feed、超长最终 URL 拒。

const acceptTestValidRSS = `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0"><channel><title>Accept</title><link>https://example.com</link>
<description>d</description>
<item><title>i</title><link>https://example.com/1</link></item></channel></rss>`

func setupAcceptTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:accept_%d?mode=memory&cache=shared", time.Now().UnixNano())), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&models.Feed{}, &models.RSSHubRoute{}, &models.FeedCandidate{}, &models.FeedRecommendation{},
	))
	// resolveRSSHubBaseURL 经 aisettings 读全局 database.DB（表缺失 → 如实回落默认 base），
	// 这里把全局指向 sqlite 测试库，避免空指针。
	database.DB = db
	return db
}

// mockSubscriptionFetch 注入可控抓取，返回调用计数（供「不应发起抓取」断言）。
// 计数用 atomic.Int64：TestAcceptConcurrentSameRecommendationIdempotent 里 4 个
// goroutine 并发 accept 会并发调用本 mock，普通 int++ 是数据竞争（-race 实锤）。
func mockSubscriptionFetch(t *testing.T, status int, body string, err error) *atomic.Int64 {
	t.Helper()
	calls := &atomic.Int64{}
	restore := readersvc.SetSubscriptionFetcher(func(_ context.Context, rawURL string, _ safefetch.Options) (*safefetch.Result, error) {
		calls.Add(1)
		if err != nil {
			return nil, err
		}
		return &safefetch.Result{StatusCode: status, Body: []byte(body), FinalURL: rawURL}, nil
	})
	t.Cleanup(restore)
	return calls
}

func acceptTestFeedCount(t *testing.T, db *gorm.DB) int64 {
	t.Helper()
	var n int64
	require.NoError(t, db.Model(&models.Feed{}).Count(&n).Error)
	return n
}

// acceptRSSFixture 建原生 RSS 候选 + 推荐行（candidate_id 直接落位，走 kind=rss 路径）。
func acceptRSSFixture(t *testing.T, db *gorm.DB, feedURL string) *models.FeedRecommendation {
	t.Helper()
	stableKey, err := BuildRSSStableKey(feedURL)
	require.NoError(t, err)
	normalized, err := NormalizeRSSURL(feedURL)
	require.NoError(t, err)
	enabled := true
	cand := models.FeedCandidate{
		StableKey: stableKey, Kind: "rss", FeedURL: normalized, CanonicalKey: *normalized,
		ManualMetadata: models.MetadataMap{}, RecommendationEnabled: &enabled,
		AccessScope: "public", Revision: 1,
	}
	require.NoError(t, db.Create(&cand).Error)
	return acceptRecommendationRow(t, db, cand.ID, 0)
}

// acceptRSSHubFixture 建 RSSHub 路由候选 + 推荐行（kind=rsshub 路径）。
func acceptRSSHubFixture(t *testing.T, db *gorm.DB, namespace, path string, requiresParams bool) *models.FeedRecommendation {
	t.Helper()
	route := models.RSSHubRoute{
		Namespace: namespace, Path: path, Name: "Route " + namespace, Status: "ok",
		UsableDirectly: !requiresParams, RequiresParameters: requiresParams, Parameters: "{}",
	}
	require.NoError(t, db.Create(&route).Error)
	stableKey, err := BuildRSSHubStableKey(namespace, path)
	require.NoError(t, err)
	enabled := true
	cand := models.FeedCandidate{
		StableKey: stableKey, Kind: "rsshub", RouteID: &route.ID,
		ManualMetadata: models.MetadataMap{}, RecommendationEnabled: &enabled,
		AccessScope: "public", Revision: 1,
	}
	require.NoError(t, db.Create(&cand).Error)
	return acceptRecommendationRow(t, db, cand.ID, route.ID)
}

func acceptRecommendationRow(t *testing.T, db *gorm.DB, candidateID, routeID uint) *models.FeedRecommendation {
	t.Helper()
	rec := models.FeedRecommendation{
		RouteID: routeID, CandidateID: &candidateID, Source: RecommendationSourceManualRefresh,
		Status: "pending", RecommendationHash: fmt.Sprintf("accept-hash-%d", time.Now().UnixNano()),
	}
	require.NoError(t, db.Create(&rec).Error)
	return &rec
}

func reloadRecommendation(t *testing.T, db *gorm.DB, id uint) models.FeedRecommendation {
	t.Helper()
	var rec models.FeedRecommendation
	require.NoError(t, db.First(&rec, id).Error)
	return rec
}

// S6-1 原生 RSS：安全验证通过 → 短事务建源 + 标 accepted。
func TestAcceptNativeRSSVerifiesAndCreatesFeed(t *testing.T) {
	db := setupAcceptTestDB(t)
	mockSubscriptionFetch(t, 200, acceptTestValidRSS, nil)
	rec := acceptRSSFixture(t, db, "https://feeds.example.com/tech.xml")
	svc := NewRecommendationService(db, nil, nil)

	feed, err := svc.AcceptRecommendation(context.Background(), rec.ID, nil, nil)
	require.NoError(t, err)
	require.Equal(t, "https://feeds.example.com/tech.xml", feed.URL)

	after := reloadRecommendation(t, db, rec.ID)
	require.Equal(t, "accepted", after.Status)
	require.NotNil(t, after.AcceptedFeedID)
	require.Equal(t, feed.ID, *after.AcceptedFeedID)
	require.Equal(t, int64(1), acceptTestFeedCount(t, db))
}

// S6-2 填参 RSSHub：按既有 buildFeedURL 填参 → 验证 → 建源。
func TestAcceptRSSHubFilledParamsVerifiesAndCreatesFeed(t *testing.T) {
	db := setupAcceptTestDB(t)
	mockSubscriptionFetch(t, 200, acceptTestValidRSS, nil)
	rec := acceptRSSHubFixture(t, db, "bilibili", "/user/dynamic/:uid", true)
	svc := NewRecommendationService(db, nil, nil)

	feed, err := svc.AcceptRecommendation(context.Background(), rec.ID, nil, map[string]string{"uid": "42"})
	require.NoError(t, err)
	require.Equal(t, DefaultRSSHubBaseURL+"/bilibili/user/dynamic/42", feed.URL)
	require.Equal(t, "Route bilibili", feed.Title)

	after := reloadRecommendation(t, db, rec.ID)
	require.Equal(t, "accepted", after.Status)
	require.Equal(t, int64(1), acceptTestFeedCount(t, db))
}

// S6 原生 RSS 验证失败（超时）→ 不建源、不标 accepted、保持 pending 可重试。
func TestAcceptNativeRSSVerificationFailureKeepsPending(t *testing.T) {
	db := setupAcceptTestDB(t)
	mockSubscriptionFetch(t, 0, "", safefetch.ErrTimeout)
	rec := acceptRSSFixture(t, db, "https://feeds.example.com/down.xml")
	svc := NewRecommendationService(db, nil, nil)

	_, err := svc.AcceptRecommendation(context.Background(), rec.ID, nil, nil)
	require.Error(t, err)
	require.ErrorIs(t, err, readersvc.ErrFeedVerification)
	require.ErrorIs(t, err, safefetch.ErrTimeout)
	require.Equal(t, int64(0), acceptTestFeedCount(t, db))

	after := reloadRecommendation(t, db, rec.ID)
	require.Equal(t, "pending", after.Status)
	require.Nil(t, after.AcceptedFeedID)
}

// S6 填参 RSSHub 验证失败（HTTP 200 但非 RSS）→ 不建源、不标 accepted。
func TestAcceptRSSHubNonRSSBodyRejected(t *testing.T) {
	db := setupAcceptTestDB(t)
	mockSubscriptionFetch(t, 200, "<html><body>not a feed</body></html>", nil)
	rec := acceptRSSHubFixture(t, db, "ns", "/user/:uid", true)
	svc := NewRecommendationService(db, nil, nil)

	_, err := svc.AcceptRecommendation(context.Background(), rec.ID, nil, map[string]string{"uid": "1"})
	require.Error(t, err)
	require.ErrorIs(t, err, readersvc.ErrFeedVerification)
	require.ErrorContains(t, err, "not a parseable RSS/Atom")
	require.Equal(t, int64(0), acceptTestFeedCount(t, db))
	require.Equal(t, "pending", reloadRecommendation(t, db, rec.ID).Status)
}

// S6 原生候选 HTTP 200 但内容非 RSS → 拒绝（不因 HTTP 成功就放行）。
func TestAcceptNativeNonRSSBodyRejected(t *testing.T) {
	db := setupAcceptTestDB(t)
	mockSubscriptionFetch(t, 200, "plain text, definitely not a feed", nil)
	rec := acceptRSSFixture(t, db, "https://example.com/blog")
	svc := NewRecommendationService(db, nil, nil)

	_, err := svc.AcceptRecommendation(context.Background(), rec.ID, nil, nil)
	require.Error(t, err)
	require.ErrorIs(t, err, readersvc.ErrFeedVerification)
	require.Equal(t, int64(0), acceptTestFeedCount(t, db))
}

// S6 私网/保留地址拒绝的错误必须脱敏：不含完整订阅 URL（含 path/query）。
func TestAcceptPrivateAddressErrorIsSanitized(t *testing.T) {
	db := setupAcceptTestDB(t)
	target := "http://192.168.1.50/internal/feed.xml?token=secret"
	mockSubscriptionFetch(t, 0, "", &safefetch.PrivateAddressError{IP: net.ParseIP("192.168.1.50")})
	rec := acceptRSSFixture(t, db, target)
	svc := NewRecommendationService(db, nil, nil)

	_, err := svc.AcceptRecommendation(context.Background(), rec.ID, nil, nil)
	require.Error(t, err)
	require.ErrorIs(t, err, safefetch.ErrPrivateAddress)
	require.NotContains(t, err.Error(), target)
	require.NotContains(t, err.Error(), "http://192.168.1.50")
	require.NotContains(t, err.Error(), "token=secret")
	require.Equal(t, int64(0), acceptTestFeedCount(t, db))
}

// S6-3 重复 accept 同有效地址 → 仅一个 Feed；同一推荐重复 accept 幂等。
func TestAcceptSameURLReusesSingleFeed(t *testing.T) {
	db := setupAcceptTestDB(t)
	mockSubscriptionFetch(t, 200, acceptTestValidRSS, nil)
	rec1 := acceptRSSFixture(t, db, "https://feeds.example.com/same.xml")
	rec2 := acceptRecommendationRow(t, db, *rec1.CandidateID, 0)
	svc := NewRecommendationService(db, nil, nil)
	ctx := context.Background()

	first, err := svc.AcceptRecommendation(ctx, rec1.ID, nil, nil)
	require.NoError(t, err)
	second, err := svc.AcceptRecommendation(ctx, rec2.ID, nil, nil)
	require.NoError(t, err)
	require.Equal(t, first.ID, second.ID, "同地址不同推荐应复用同一 Feed")

	// 同一推荐再次 accept：幂等短路，返回同一 Feed。
	again, err := svc.AcceptRecommendation(ctx, rec1.ID, nil, nil)
	require.NoError(t, err)
	require.Equal(t, first.ID, again.ID)
	require.Equal(t, int64(1), acceptTestFeedCount(t, db), "重复 accept 不得产生第二个订阅")
}

// S6-2 填参后最终 URL 超 500 rune → 就地拒绝、不抓取、不建源。
func TestAcceptRSSHubOverlongFinalURLRejected(t *testing.T) {
	db := setupAcceptTestDB(t)
	calls := mockSubscriptionFetch(t, 200, acceptTestValidRSS, nil)
	rec := acceptRSSHubFixture(t, db, "ns", "/long/:token", true)
	svc := NewRecommendationService(db, nil, nil)

	_, err := svc.AcceptRecommendation(context.Background(), rec.ID, nil, map[string]string{"token": strings.Repeat("a", 600)})
	require.Error(t, err)
	require.ErrorIs(t, err, readersvc.ErrInvalidFeedURL)
	require.Equal(t, int64(0), calls.Load(), "超长地址须在抓取前拒绝")
	require.Equal(t, int64(0), acceptTestFeedCount(t, db))
	require.Equal(t, "pending", reloadRecommendation(t, db, rec.ID).Status)
}

// TestMarkRecommendationAcceptedConditional（Medium 11）：条件更新 WHERE status='pending'；
// 已 accepted 行再次标记返回 updated=false，且不覆盖已有 accepted_feed_id（并发输家不偷写）。
func TestMarkRecommendationAcceptedConditional(t *testing.T) {
	db := setupAcceptTestDB(t)
	rec := acceptRSSFixture(t, db, "https://feeds.example.com/conditional.xml")
	require.NoError(t, db.Create(&models.Feed{Title: "f", URL: "https://feeds.example.com/conditional.xml"}).Error)
	var feed models.Feed
	require.NoError(t, db.First(&feed).Error)

	updated, err := markRecommendationAccepted(db, rec.ID, feed.ID)
	require.NoError(t, err)
	require.True(t, updated)

	// 第二次（模拟并发输家）：影响行数 0，不得把 accepted_feed_id 改成别人的。
	updated2, err := markRecommendationAccepted(db, rec.ID, uint(9999))
	require.NoError(t, err)
	require.False(t, updated2, "非 pending 行不得再标 accepted")
	after := reloadRecommendation(t, db, rec.ID)
	require.NotNil(t, after.AcceptedFeedID)
	require.Equal(t, feed.ID, *after.AcceptedFeedID, "不得覆盖已有关联 feed")
}

// TestAcceptConcurrentSameRecommendationIdempotent（Medium 11 并发）：同一 pending 推荐
// 并发 accept → 全部成功且收敛到同一 Feed，accepted_feed_id 唯一。
func TestAcceptConcurrentSameRecommendationIdempotent(t *testing.T) {
	db := setupAcceptTestDB(t)
	// sqlite 共享缓存并发写易 SQLITE_BUSY：单连接串行化，仍覆盖「条件更新 + 幂等复用」。
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	mockSubscriptionFetch(t, 200, acceptTestValidRSS, nil)
	rec := acceptRSSFixture(t, db, "https://feeds.example.com/concurrent.xml")
	svc := NewRecommendationService(db, nil, nil)

	const n = 4
	feeds := make([]*models.Feed, n)
	errs := make([]error, n)
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			feeds[i], errs[i] = svc.AcceptRecommendation(context.Background(), rec.ID, nil, nil)
		}(i)
	}
	close(start)
	wg.Wait()
	for i, e := range errs {
		require.NoError(t, e, "并发 accept 第 %d 个必须成功（幂等）", i)
	}
	for i := 1; i < n; i++ {
		require.NotNil(t, feeds[i])
		require.Equal(t, feeds[0].ID, feeds[i].ID, "并发 accept 收敛到同一 Feed")
	}
	after := reloadRecommendation(t, db, rec.ID)
	require.Equal(t, "accepted", after.Status)
	require.NotNil(t, after.AcceptedFeedID)
	require.Equal(t, int64(1), acceptTestFeedCount(t, db), "同地址只建一个订阅")
}
