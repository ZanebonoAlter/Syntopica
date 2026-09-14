package service

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/safefetch"
)

// ── 候选可用性检查 S14（spec C6：Availability Checks Reflect Actual Endpoints）──
//
// sqlite 内存库只迁移本切片相关表；抓取经 SetCandidateCheckFetcher 注入 mock
// （safefetch 只允许外网可达且拒私网，单测不能真发请求）；时钟经 svc.now 注入，
// 驱动「连续 3 次跨 24h」等时间窗分支。

// validRSSBody 是可解析的最小 RSS 2.0（成功路径用）。
const validRSSBody = `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0"><channel><title>Check Feed</title><link>https://example.com</link>
<description>d</description><item><title>i1</title><link>https://example.com/1</link></item></channel></rss>`

func setupCandidateCheckTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:check-%s-%d?mode=memory&cache=shared", t.Name(), time.Now().UnixNano())), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&models.FeedCandidate{}, &models.Feed{}, &models.RSSHubRoute{}, &models.CandidateAvailability{},
	))
	return db
}

// newCandidateCheckTestService 构造服务并固定 RSSHub 实例基址（不读 ai_settings）。
func newCandidateCheckTestService(t *testing.T, db *gorm.DB) *CandidateCheckService {
	t.Helper()
	svc := NewCandidateCheckService(db)
	svc.baseURL = func() string { return "https://hub.test" }
	return svc
}

// candidateFetchStub 记录注入抓取的调用次数与端点。
type candidateFetchStub struct {
	count int
	urls  []string
}

func stubCandidateFetch(t *testing.T, fn func(rawURL string) (*CandidateFetchResult, error)) *candidateFetchStub {
	t.Helper()
	stub := &candidateFetchStub{}
	restore := SetCandidateCheckFetcher(func(_ context.Context, rawURL string, _ safefetch.Options) (*CandidateFetchResult, error) {
		stub.count++
		stub.urls = append(stub.urls, rawURL)
		return fn(rawURL)
	})
	t.Cleanup(restore)
	return stub
}

func okFeedResult() *CandidateFetchResult {
	return &CandidateFetchResult{Result: &safefetch.Result{StatusCode: 200, Body: []byte(validRSSBody)}}
}

func httpStatusResult(code int) *CandidateFetchResult {
	return &CandidateFetchResult{Result: &safefetch.Result{StatusCode: code}}
}

func seedRSSCandidate(t *testing.T, db *gorm.DB, feedURL, scope string, enabled bool) *models.FeedCandidate {
	t.Helper()
	url := feedURL
	cand := models.FeedCandidate{
		StableKey: "rss:" + feedURL, Kind: "rss",
		FeedURL: &url, CanonicalKey: feedURL,
		ManualMetadata:        models.MetadataMap{ManualFieldName: "Check Target"},
		RecommendationEnabled: &enabled, AccessScope: scope, Revision: 1,
	}
	require.NoError(t, db.Create(&cand).Error)
	return &cand
}

func seedRSSHubCandidate(t *testing.T, db *gorm.DB, routeID uint, stableKey string) *models.FeedCandidate {
	t.Helper()
	enabled := true
	cand := models.FeedCandidate{
		StableKey: stableKey, Kind: "rsshub", RouteID: &routeID,
		ManualMetadata:        models.MetadataMap{ManualFieldName: "Hub Target"},
		RecommendationEnabled: &enabled, AccessScope: "public", Revision: 1,
	}
	require.NoError(t, db.Create(&cand).Error)
	return &cand
}

func loadAvailabilityRow(t *testing.T, db *gorm.DB, candidateID uint) models.CandidateAvailability {
	t.Helper()
	var row models.CandidateAvailability
	require.NoError(t, db.Where("candidate_id = ?", candidateID).First(&row).Error)
	return row
}

// TestCheckCandidateRequiresParametersIssuesNoRequest S14-1：路由必需参数未填且无实例
// → requires_parameters，零请求、无复查计划（参数缺失不是源失效）。
func TestCheckCandidateRequiresParametersIssuesNoRequest(t *testing.T) {
	db := setupCandidateCheckTestDB(t)
	svc := newCandidateCheckTestService(t, db)
	now := time.Date(2026, 9, 13, 12, 0, 0, 0, time.UTC)
	svc.now = func() time.Time { return now }
	stub := stubCandidateFetch(t, func(string) (*CandidateFetchResult, error) { return okFeedResult(), nil })

	route := models.RSSHubRoute{Namespace: "ns", Path: "/feed/:id", RequiresParameters: true, UsableDirectly: false}
	require.NoError(t, db.Create(&route).Error)
	cand := seedRSSHubCandidate(t, db, route.ID, "ns/feed/:id")

	res, err := svc.CheckCandidate(context.Background(), cand.ID, safefetch.Options{})
	require.NoError(t, err)
	require.Equal(t, AvailabilityStatusRequiresParams, res.Availability)
	require.Empty(t, res.LastErrorCode, "requires_parameters 不记失败错误码")
	require.Equal(t, 0, stub.count, "需参数无实例不得发请求")
	require.Nil(t, res.NextCheckAt)

	row := loadAvailabilityRow(t, db, cand.ID)
	require.Equal(t, AvailabilityStatusRequiresParams, row.Status)
	require.Empty(t, row.LastEndpointKey)
	require.Nil(t, row.NextCheckAt)
	require.Equal(t, 0, row.ConsecutiveFailures)
}

// TestCheckCandidateTimeoutIsTransientFailure S14-2：单次超时记网络错误，不判死
// （保持 unknown、计数 1、按默认周期复查）。
func TestCheckCandidateTimeoutIsTransientFailure(t *testing.T) {
	db := setupCandidateCheckTestDB(t)
	svc := newCandidateCheckTestService(t, db)
	now := time.Date(2026, 9, 13, 12, 0, 0, 0, time.UTC)
	svc.now = func() time.Time { return now }
	stub := stubCandidateFetch(t, func(string) (*CandidateFetchResult, error) {
		return nil, fmt.Errorf("%w (after %s)", safefetch.ErrTimeout, safefetch.DefaultTimeout)
	})

	cand := seedRSSCandidate(t, db, "https://example.com/timeout", "public", true)
	res, err := svc.CheckCandidate(context.Background(), cand.ID, safefetch.Options{})
	require.NoError(t, err)

	require.Equal(t, 1, stub.count)
	require.Equal(t, []string{"https://example.com/timeout"}, stub.urls)
	require.Equal(t, AvailabilityStatusUnknown, res.Availability)
	require.Equal(t, OutcomeNetworkError, res.LastErrorCode)
	require.NotNil(t, res.LastCheckedAt)
	require.Equal(t, now, *res.LastCheckedAt)
	require.Equal(t, now.Add(DefaultCheckInterval), *res.NextCheckAt)

	row := loadAvailabilityRow(t, db, cand.ID)
	require.Equal(t, 1, row.ConsecutiveFailures)
	require.NotNil(t, row.FirstFailureAt)
	require.Equal(t, now, *row.FirstFailureAt)
}

// TestCheckCandidateHTTP200NonRSSIsNotPassing S14-2：HTTP 200 但正文不是 RSS/Atom
// → 内容校验失败（不显示通过），单次仍不判死。
func TestCheckCandidateHTTP200NonRSSIsNotPassing(t *testing.T) {
	db := setupCandidateCheckTestDB(t)
	svc := newCandidateCheckTestService(t, db)
	now := time.Date(2026, 9, 13, 12, 0, 0, 0, time.UTC)
	svc.now = func() time.Time { return now }
	stubCandidateFetch(t, func(string) (*CandidateFetchResult, error) {
		return &CandidateFetchResult{Result: &safefetch.Result{
			StatusCode: 200, Body: []byte("<html><body>not a feed</body></html>"),
		}}, nil
	})

	cand := seedRSSCandidate(t, db, "https://example.com/html", "public", true)
	res, err := svc.CheckCandidate(context.Background(), cand.ID, safefetch.Options{})
	require.NoError(t, err)

	require.NotEqual(t, AvailabilityStatusOK, res.Availability, "非 RSS 不得显示检查通过")
	require.Equal(t, AvailabilityStatusUnknown, res.Availability)
	require.Equal(t, OutcomeHTTPOkContentInvalid, res.LastErrorCode)
	require.Equal(t, 1, loadAvailabilityRow(t, db, cand.ID).ConsecutiveFailures)
}

// TestCheckCandidateGoneBreaksImmediately S14-3：HTTP 410 确定失效，不等失败门槛。
func TestCheckCandidateGoneBreaksImmediately(t *testing.T) {
	db := setupCandidateCheckTestDB(t)
	svc := newCandidateCheckTestService(t, db)
	now := time.Date(2026, 9, 13, 12, 0, 0, 0, time.UTC)
	svc.now = func() time.Time { return now }
	stubCandidateFetch(t, func(string) (*CandidateFetchResult, error) { return httpStatusResult(410), nil })

	cand := seedRSSCandidate(t, db, "https://example.com/gone", "public", true)
	res, err := svc.CheckCandidate(context.Background(), cand.ID, safefetch.Options{})
	require.NoError(t, err)

	require.Equal(t, AvailabilityStatusBroken, res.Availability)
	require.Equal(t, OutcomeHTTP410, res.LastErrorCode)
	row := loadAvailabilityRow(t, db, cand.ID)
	require.Equal(t, AvailabilityStatusBroken, row.Status)
	require.Equal(t, 1, row.ConsecutiveFailures)
}

// TestCheckCandidateRateLimitPostponesWithoutEscalation S14-3：429 按 Retry-After 有界
// 推迟、不计失败；无 Retry-After 用默认周期；连续 429 跨 24h 也不升级 broken。
func TestCheckCandidateRateLimitPostponesWithoutEscalation(t *testing.T) {
	db := setupCandidateCheckTestDB(t)
	svc := newCandidateCheckTestService(t, db)
	base := time.Date(2026, 9, 13, 12, 0, 0, 0, time.UTC)

	// 带 Retry-After：三次跨 24h 仍保持 unknown、计数不涨。
	stubCandidateFetch(t, func(string) (*CandidateFetchResult, error) {
		return &CandidateFetchResult{
			Result:            &safefetch.Result{StatusCode: 429},
			RetryAfterSeconds: 3600,
		}, nil
	})
	cand := seedRSSCandidate(t, db, "https://example.com/limited", "public", true)
	for _, at := range []time.Time{base, base.Add(time.Hour), base.Add(25 * time.Hour)} {
		at := at
		svc.now = func() time.Time { return at }
		res, err := svc.CheckCandidate(context.Background(), cand.ID, safefetch.Options{})
		require.NoError(t, err)
		require.Equal(t, AvailabilityStatusUnknown, res.Availability, "限流不得升级失效")
		require.Equal(t, OutcomeHTTP429, res.LastErrorCode)
		require.Equal(t, at.Add(time.Hour), *res.NextCheckAt)
	}
	row := loadAvailabilityRow(t, db, cand.ID)
	require.Equal(t, 0, row.ConsecutiveFailures)
	require.Nil(t, row.FirstFailureAt)

	// 无 Retry-After：按默认周期（7 天）复查。
	stubCandidateFetch(t, func(string) (*CandidateFetchResult, error) { return httpStatusResult(429), nil })
	fresh := seedRSSCandidate(t, db, "https://example.com/limited2", "public", true)
	svc.now = func() time.Time { return base }
	res, err := svc.CheckCandidate(context.Background(), fresh.ID, safefetch.Options{})
	require.NoError(t, err)
	require.Equal(t, base.Add(DefaultCheckInterval), *res.NextCheckAt)
}

// TestCheckCandidateThreeFailuresAcross24hBreaks S14-3：连续 3 次失败且跨度 ≥24h 才升级。
func TestCheckCandidateThreeFailuresAcross24hBreaks(t *testing.T) {
	db := setupCandidateCheckTestDB(t)
	svc := newCandidateCheckTestService(t, db)
	base := time.Date(2026, 9, 13, 12, 0, 0, 0, time.UTC)
	stubCandidateFetch(t, func(string) (*CandidateFetchResult, error) {
		return nil, errors.New("connection refused")
	})

	cand := seedRSSCandidate(t, db, "https://example.com/flaky", "public", true)

	svc.now = func() time.Time { return base }
	res, err := svc.CheckCandidate(context.Background(), cand.ID, safefetch.Options{})
	require.NoError(t, err)
	require.Equal(t, AvailabilityStatusUnknown, res.Availability)

	svc.now = func() time.Time { return base.Add(2 * time.Hour) }
	res, err = svc.CheckCandidate(context.Background(), cand.ID, safefetch.Options{})
	require.NoError(t, err)
	require.Equal(t, AvailabilityStatusUnknown, res.Availability, "两次失败不判死")
	require.Equal(t, 2, loadAvailabilityRow(t, db, cand.ID).ConsecutiveFailures)

	svc.now = func() time.Time { return base.Add(25 * time.Hour) }
	res, err = svc.CheckCandidate(context.Background(), cand.ID, safefetch.Options{})
	require.NoError(t, err)
	require.Equal(t, AvailabilityStatusBroken, res.Availability)

	row := loadAvailabilityRow(t, db, cand.ID)
	require.Equal(t, 3, row.ConsecutiveFailures)
	require.Equal(t, base, *row.FirstFailureAt, "首次失败时间保持不动")
}

// TestCheckCandidateSuccessRestoresAvailabilityWithoutTouchingPreferenceOrSubscription
// S14-4（spec C6 修复后复查）：曾失效的源复查成功 → 可用性转 ok + 检查时间；用户已手动
// 关闭的推荐开关与已有订阅都不受影响。
func TestCheckCandidateSuccessRestoresAvailabilityWithoutTouchingPreferenceOrSubscription(t *testing.T) {
	db := setupCandidateCheckTestDB(t)
	svc := newCandidateCheckTestService(t, db)
	now := time.Date(2026, 9, 13, 12, 0, 0, 0, time.UTC)
	svc.now = func() time.Time { return now }
	stubCandidateFetch(t, func(string) (*CandidateFetchResult, error) { return okFeedResult(), nil })

	// 用户已手动关闭推荐（recommendation_enabled=false）的已订阅源。
	cand := seedRSSCandidate(t, db, "https://example.com/repaired", "public", false)
	require.NoError(t, db.Create(&models.Feed{Title: "Repaired", URL: "https://example.com/repaired"}).Error)

	old := now.Add(-48 * time.Hour)
	require.NoError(t, db.Create(&models.CandidateAvailability{
		CandidateID: cand.ID, Status: AvailabilityStatusBroken, ConsecutiveFailures: 3,
		FirstFailureAt: &old, LastEndpointKey: "https://example.com/repaired",
	}).Error)

	res, err := svc.CheckCandidate(context.Background(), cand.ID, safefetch.Options{})
	require.NoError(t, err)
	require.Equal(t, AvailabilityStatusOK, res.Availability)
	require.Empty(t, res.LastErrorCode)
	require.Equal(t, now, *res.LastCheckedAt)
	require.Equal(t, now.Add(DefaultCheckInterval), *res.NextCheckAt)

	row := loadAvailabilityRow(t, db, cand.ID)
	require.Equal(t, AvailabilityStatusOK, row.Status)
	require.Equal(t, 0, row.ConsecutiveFailures)
	require.Nil(t, row.FirstFailureAt)
	require.NotNil(t, row.LastSuccessAt)
	require.Equal(t, now, *row.LastSuccessAt)

	var stored models.FeedCandidate
	require.NoError(t, db.First(&stored, cand.ID).Error)
	require.NotNil(t, stored.RecommendationEnabled)
	require.False(t, *stored.RecommendationEnabled, "检查成功不得解除人工关闭的推荐开关")
	var feedCount int64
	require.NoError(t, db.Model(&models.Feed{}).Count(&feedCount).Error)
	require.EqualValues(t, 1, feedCount, "检查不得增删订阅")
}

// TestCheckCandidatePrivatePendingIsForbiddenWithNoRequest：未授权私有来源任何检查
// 都不得发起（403 forbidden，零请求、零落库）。
func TestCheckCandidatePrivatePendingIsForbiddenWithNoRequest(t *testing.T) {
	db := setupCandidateCheckTestDB(t)
	svc := newCandidateCheckTestService(t, db)
	stub := stubCandidateFetch(t, func(string) (*CandidateFetchResult, error) { return okFeedResult(), nil })

	cand := seedRSSCandidate(t, db, "https://10.0.0.9/feed", "private_pending", true)
	_, err := svc.CheckCandidate(context.Background(), cand.ID, safefetch.Options{})
	require.Error(t, err)

	var ce *CandidateError
	require.ErrorAs(t, err, &ce)
	require.Equal(t, CandidateErrorCodeForbidden, ce.Code)
	require.Equal(t, 0, stub.count, "private_pending 不得发起请求")

	var rows int64
	require.NoError(t, db.Model(&models.CandidateAvailability{}).Count(&rows).Error)
	require.EqualValues(t, 0, rows)
}

// TestCheckCandidateEndpointChangeInvalidatesOldState S14 白盒变体：端点（实际地址）
// 变化 → 旧结论作废回 unknown 重新评估。
func TestCheckCandidateEndpointChangeInvalidatesOldState(t *testing.T) {
	db := setupCandidateCheckTestDB(t)
	svc := newCandidateCheckTestService(t, db)
	base := time.Date(2026, 9, 13, 12, 0, 0, 0, time.UTC)

	stubCandidateFetch(t, func(string) (*CandidateFetchResult, error) { return httpStatusResult(410), nil })
	cand := seedRSSCandidate(t, db, "https://example.com/old", "public", true)
	svc.now = func() time.Time { return base }
	res, err := svc.CheckCandidate(context.Background(), cand.ID, safefetch.Options{})
	require.NoError(t, err)
	require.Equal(t, AvailabilityStatusBroken, res.Availability)

	// 用户把地址改为另一个实例地址。
	require.NoError(t, db.Model(&models.FeedCandidate{}).Where("id = ?", cand.ID).
		Update("feed_url", "https://example.com/new").Error)

	stubCandidateFetch(t, func(string) (*CandidateFetchResult, error) {
		return nil, errors.New("connection refused")
	})
	svc.now = func() time.Time { return base.Add(2 * time.Hour) }
	res, err = svc.CheckCandidate(context.Background(), cand.ID, safefetch.Options{})
	require.NoError(t, err)
	require.Equal(t, AvailabilityStatusUnknown, res.Availability, "端点变化后旧 broken 作废")
	require.Equal(t, 1, loadAvailabilityRow(t, db, cand.ID).ConsecutiveFailures)

	row := loadAvailabilityRow(t, db, cand.ID)
	require.Equal(t, "https://example.com/new", row.LastEndpointKey)
}

// TestCheckCandidateTemplateInstancesAreIndependent S14 白盒变体：同一模板的多个实例
// 互不影响（状态按候选分别保存）。
func TestCheckCandidateTemplateInstancesAreIndependent(t *testing.T) {
	db := setupCandidateCheckTestDB(t)
	svc := newCandidateCheckTestService(t, db)
	base := time.Date(2026, 9, 13, 12, 0, 0, 0, time.UTC)
	svc.now = func() time.Time { return base }

	route := models.RSSHubRoute{Namespace: "ns", Path: "/list", UsableDirectly: true}
	require.NoError(t, db.Create(&route).Error)
	first := seedRSSHubCandidate(t, db, route.ID, "ns/list")
	second := seedRSSHubCandidate(t, db, route.ID, "ns/list#instance2")

	stubCandidateFetch(t, func(string) (*CandidateFetchResult, error) { return httpStatusResult(410), nil })
	res, err := svc.CheckCandidate(context.Background(), first.ID, safefetch.Options{})
	require.NoError(t, err)
	require.Equal(t, AvailabilityStatusBroken, res.Availability)

	stubCandidateFetch(t, func(string) (*CandidateFetchResult, error) { return okFeedResult(), nil })
	res, err = svc.CheckCandidate(context.Background(), second.ID, safefetch.Options{})
	require.NoError(t, err)
	require.Equal(t, AvailabilityStatusOK, res.Availability)

	require.Equal(t, AvailabilityStatusBroken, loadAvailabilityRow(t, db, first.ID).Status)
	require.Equal(t, AvailabilityStatusOK, loadAvailabilityRow(t, db, second.ID).Status)
}

// TestCheckCandidateRejectsConcurrentDuplicate：同候选检查进行中重复调用返回 conflict
// （并发检查会重复累加失败计数、扭曲升级判定）。
func TestCheckCandidateRejectsConcurrentDuplicate(t *testing.T) {
	db := setupCandidateCheckTestDB(t)
	svc := newCandidateCheckTestService(t, db)

	entered := make(chan struct{})
	release := make(chan struct{})
	var enteredOnce sync.Once
	SetCandidateCheckFetcher(func(_ context.Context, _ string, _ safefetch.Options) (*CandidateFetchResult, error) {
		enteredOnce.Do(func() { close(entered) })
		<-release
		return okFeedResult(), nil
	})
	defer SetCandidateCheckFetcher(fetchCandidateAvailability)

	cand := seedRSSCandidate(t, db, "https://example.com/slow", "public", true)
	done := make(chan error, 1)
	go func() {
		_, err := svc.CheckCandidate(context.Background(), cand.ID, safefetch.Options{})
		done <- err
	}()

	<-entered
	_, err := svc.CheckCandidate(context.Background(), cand.ID, safefetch.Options{})
	require.Error(t, err)
	var ce *CandidateError
	require.ErrorAs(t, err, &ce)
	require.Equal(t, CandidateErrorCodeConflict, ce.Code)

	close(release)
	require.NoError(t, <-done)

	// 释放后可以再次检查（防重入不粘滞）。
	res, err := svc.CheckCandidate(context.Background(), cand.ID, safefetch.Options{})
	require.NoError(t, err)
	require.Equal(t, AvailabilityStatusOK, res.Availability)
}

// TestCheckCandidateNotFound：候选不存在 → not_found 类型错误。
func TestCheckCandidateNotFound(t *testing.T) {
	db := setupCandidateCheckTestDB(t)
	svc := newCandidateCheckTestService(t, db)
	_, err := svc.CheckCandidate(context.Background(), 9999, safefetch.Options{})
	var ce *CandidateError
	require.ErrorAs(t, err, &ce)
	require.Equal(t, CandidateErrorCodeNotFound, ce.Code)
}

// TestDueCandidateIDsFiltersPrivateAndRequiresParameters：到期候选取用（4.6 调度入口）：
// private_pending 与 requires_parameters 不入队，未到期的排除。
func TestDueCandidateIDsFiltersPrivateAndRequiresParameters(t *testing.T) {
	db := setupCandidateCheckTestDB(t)
	svc := newCandidateCheckTestService(t, db)
	now := time.Date(2026, 9, 13, 12, 0, 0, 0, time.UTC)

	due := seedRSSCandidate(t, db, "https://example.com/due", "public", true)
	privateCandidate := seedRSSCandidate(t, db, "https://10.0.0.9/due", "private_pending", true)
	requiresRoute := models.RSSHubRoute{Namespace: "ns", Path: "/x/:id", RequiresParameters: true}
	require.NoError(t, db.Create(&requiresRoute).Error)
	requires := seedRSSHubCandidate(t, db, requiresRoute.ID, "ns/x/:id")

	future := seedRSSCandidate(t, db, "https://example.com/future", "public", true)
	futureAt := now.Add(time.Hour)
	require.NoError(t, db.Create(&models.CandidateAvailability{
		CandidateID: future.ID, Status: AvailabilityStatusUnknown, NextCheckAt: &futureAt,
	}).Error)
	require.NoError(t, db.Create(&models.CandidateAvailability{
		CandidateID: requires.ID, Status: AvailabilityStatusRequiresParams,
	}).Error)

	ids, err := svc.DueCandidateIDs(context.Background(), now, 10)
	require.NoError(t, err)
	require.Equal(t, []uint{due.ID}, ids)
	require.NotContains(t, ids, privateCandidate.ID)
}
