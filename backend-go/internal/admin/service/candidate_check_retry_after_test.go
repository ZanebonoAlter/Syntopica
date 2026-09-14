package service

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/safefetch"
)

// ── 4.6 补洞：429 Retry-After 端到端消费 + discovery_v2 开关拦截检查入口 ──
//
// 本文件走生产抓取实现（fetchCandidateAvailability → safefetch）对着本地 httptest
// 服务器取证：safefetch.Result.RetryAfterSeconds 必须真的影响 next_check_at，
// 而不是只有注入路径才能生效。

func setupCandidateCheckSwitchTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:check-switch-%s-%d?mode=memory&cache=shared", t.Name(), time.Now().UnixNano())), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&models.FeedCandidate{}, &models.RSSHubRoute{}, &models.CandidateAvailability{}, &models.AISettings{},
	))
	return db
}

func loopbackAllowedOptions(t *testing.T) safefetch.Options {
	t.Helper()
	_, cidr, err := net.ParseCIDR("127.0.0.0/8")
	require.NoError(t, err)
	return safefetch.Options{AllowedIPs: []*net.IPNet{cidr}}
}

// restoreProductionCandidateFetcher 保证本文件测的是生产抓取实现（其它测试的注入用
// Cleanup 还原，这里显式再钉一次，避免顺序耦合）。
func restoreProductionCandidateFetcher(t *testing.T) {
	t.Helper()
	restore := SetCandidateCheckFetcher(fetchCandidateAvailability)
	t.Cleanup(restore)
}

// TestCheckCandidate429HonoursRetryAfterHeader（4.6：429 按 Retry-After 有界推迟）：
// 响应带 Retry-After: 3600 → next_check_at = now+1h（不是默认 7 天）；无头 → 默认周期；
// 超过封顶 → 截到 7 天（有界推迟，不产生超长空窗）。
func TestCheckCandidate429HonoursRetryAfterHeader(t *testing.T) {
	db := setupCandidateCheckSwitchTestDB(t)
	restoreProductionCandidateFetcher(t)
	svc := newCandidateCheckTestService(t, db)
	now := time.Date(2026, 9, 13, 12, 0, 0, 0, time.UTC)
	svc.now = func() time.Time { return now }

	cases := []struct {
		name       string
		retryAfter string
		wantDelay  time.Duration
	}{
		{name: "retry-after seconds", retryAfter: "3600", wantDelay: time.Hour},
		{name: "no header falls back to default interval", retryAfter: "", wantDelay: DefaultCheckInterval},
		{name: "retry-after capped by max", retryAfter: "99999999", wantDelay: DefaultMaxRetryAfter},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if tc.retryAfter != "" {
					w.Header().Set("Retry-After", tc.retryAfter)
				}
				w.WriteHeader(http.StatusTooManyRequests)
			}))
			defer srv.Close()

			cand := seedRSSCandidate(t, db, srv.URL+"/feed.xml", "public", true)
			res, err := svc.CheckCandidate(context.Background(), cand.ID, loopbackAllowedOptions(t))
			require.NoError(t, err)
			require.Equal(t, AvailabilityStatusUnknown, res.Availability, "429 不改变状态、不判死")
			require.Equal(t, OutcomeHTTP429, res.LastErrorCode)
			require.NotNil(t, res.NextCheckAt)
			require.WithinDuration(t, now.Add(tc.wantDelay), *res.NextCheckAt, time.Second)
		})
	}
}

// TestCheckCandidateRejectedWhenV2Disabled（4.6 开关）：关闭 discovery_v2 后检查入口
// 直接返回 configuration（handler → 503），且不发起任何请求。
func TestCheckCandidateRejectedWhenV2Disabled(t *testing.T) {
	db := setupCandidateCheckSwitchTestDB(t)
	restoreProductionCandidateFetcher(t)
	svc := newCandidateCheckTestService(t, db)
	require.NoError(t, SaveDiscoveryV2Enabled(db, false))

	cand := seedRSSCandidate(t, db, "https://example.com/feed.xml", "public", true)
	stub := stubCandidateFetch(t, func(string) (*CandidateFetchResult, error) {
		return okFeedResult(), nil
	})

	_, err := svc.CheckCandidate(context.Background(), cand.ID, safefetch.Options{})
	require.Error(t, err)
	var ce *CandidateError
	require.True(t, errors.As(err, &ce))
	require.Equal(t, CandidateErrorCodeConfiguration, ce.Code)
	require.Equal(t, 0, stub.count, "配置禁用不得发起抓取")
}
