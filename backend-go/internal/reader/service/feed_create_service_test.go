package service

import (
	"context"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/safefetch"
)

// ── 共享建源服务测试（improve-discovery-recommendations 4.5 / design D7/D9）──

const testValidRSSBody = `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0"><channel><title>Example</title><link>https://example.com</link>
<description>example</description>
<item><title>Hello</title><link>https://example.com/1</link></item></channel></rss>`

func setupFeedCreateTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:feedcreate_%d?mode=memory&cache=shared", time.Now().UnixNano())), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.Feed{}))
	return db
}

// mockFeedCreateFetch 注入可控抓取，返回调用计数（供「不应发起抓取」断言）。
func mockFeedCreateFetch(t *testing.T, status int, body string, err error) *int {
	t.Helper()
	calls := 0
	restore := SetSubscriptionFetcher(func(_ context.Context, rawURL string, _ safefetch.Options) (*safefetch.Result, error) {
		calls++
		if err != nil {
			return nil, err
		}
		return &safefetch.Result{StatusCode: status, Body: []byte(body), FinalURL: rawURL}, nil
	})
	t.Cleanup(restore)
	return &calls
}

func countFeeds(t *testing.T, db *gorm.DB) int64 {
	t.Helper()
	var n int64
	require.NoError(t, db.Model(&models.Feed{}).Count(&n).Error)
	return n
}

func TestCreateFeedWithVerificationCreatesNormalizedFeed(t *testing.T) {
	db := setupFeedCreateTestDB(t)
	calls := mockFeedCreateFetch(t, 200, testValidRSSBody, nil)
	svc := NewFeedCreateService(db)

	feed, created, err := svc.CreateFeedWithVerification(context.Background(), "HTTPS://Feeds.Example.com:443/tech.xml#frag",
		FeedCreateOptions{Title: "Tech"})
	require.NoError(t, err)
	require.True(t, created)
	require.Equal(t, "https://feeds.example.com/tech.xml", feed.URL, "默认端口与 fragment 应被规范化去掉")
	require.Equal(t, "mdi:rss", feed.Icon)
	require.Equal(t, 100, feed.MaxArticles)
	require.Equal(t, 1, *calls)
	require.Equal(t, int64(1), countFeeds(t, db))
}

func TestCreateFeedWithVerificationReusesExistingURL(t *testing.T) {
	db := setupFeedCreateTestDB(t)
	mockFeedCreateFetch(t, 200, testValidRSSBody, nil)
	svc := NewFeedCreateService(db)
	ctx := context.Background()

	first, created, err := svc.CreateFeedWithVerification(ctx, "https://feeds.example.com/a.xml", FeedCreateOptions{})
	require.NoError(t, err)
	require.True(t, created)

	second, created, err := svc.CreateFeedWithVerification(ctx, "https://feeds.example.com/a.xml", FeedCreateOptions{})
	require.NoError(t, err)
	require.False(t, created, "同规范化地址第二次建源应复用既有 Feed")
	require.Equal(t, first.ID, second.ID)
	require.Equal(t, int64(1), countFeeds(t, db))
}

func TestCreateFeedWithVerificationRejectsNonRSSBody(t *testing.T) {
	db := setupFeedCreateTestDB(t)
	mockFeedCreateFetch(t, 200, "<html><body>not a feed</body></html>", nil)
	svc := NewFeedCreateService(db)

	_, _, err := svc.CreateFeedWithVerification(context.Background(), "https://example.com/page.html", FeedCreateOptions{})
	require.Error(t, err)
	require.ErrorIs(t, err, ErrFeedVerification)
	require.ErrorContains(t, err, "not a parseable RSS/Atom")
	require.Equal(t, int64(0), countFeeds(t, db))
}

func TestCreateFeedWithVerificationRejectsNonSuccessStatus(t *testing.T) {
	db := setupFeedCreateTestDB(t)
	mockFeedCreateFetch(t, 500, testValidRSSBody, nil)
	svc := NewFeedCreateService(db)

	_, _, err := svc.CreateFeedWithVerification(context.Background(), "https://example.com/feed", FeedCreateOptions{})
	require.ErrorIs(t, err, ErrFeedVerification)
	require.ErrorContains(t, err, "HTTP 500")
	require.Equal(t, int64(0), countFeeds(t, db))
}

func TestCreateFeedWithVerificationRejectsOverlongURLBeforeFetch(t *testing.T) {
	db := setupFeedCreateTestDB(t)
	calls := mockFeedCreateFetch(t, 200, testValidRSSBody, nil)
	svc := NewFeedCreateService(db)

	overlong := "https://example.com/" + strings.Repeat("a", SubscriptionURLMaxRunes)
	_, _, err := svc.CreateFeedWithVerification(context.Background(), overlong, FeedCreateOptions{})
	require.ErrorIs(t, err, ErrInvalidFeedURL)
	require.Equal(t, 0, *calls, "地址非法应在发起抓取前就地拒绝")
	require.Equal(t, int64(0), countFeeds(t, db))
}

// 私网拒绝的错误必须脱敏：只暴露被拒 IP，不回显完整（含 path/query 的）订阅地址。
func TestVerifySubscriptionURLSanitizesPrivateAddressError(t *testing.T) {
	db := setupFeedCreateTestDB(t)
	target := "http://192.168.1.50/internal/feed.xml?token=secret"
	mockFeedCreateFetch(t, 0, "", &safefetch.PrivateAddressError{IP: net.ParseIP("192.168.1.50")})
	svc := NewFeedCreateService(db)

	_, err := svc.VerifySubscriptionURL(context.Background(), target)
	require.Error(t, err)
	require.ErrorIs(t, err, safefetch.ErrPrivateAddress)
	require.NotContains(t, err.Error(), target)
	require.NotContains(t, err.Error(), "http://192.168.1.50")
	require.NotContains(t, err.Error(), "token=secret")
}

func TestNormalizeSubscriptionURL(t *testing.T) {
	cases := []struct {
		name    string
		raw     string
		want    string
		wantErr bool
	}{
		{name: "lowercases host and drops default port", raw: "HTTPS://Example.COM:443/Feed?B=2&a=1", want: "https://example.com/Feed?B=2&a=1"},
		{name: "keeps path case and query order", raw: "https://example.com/Foo?a=1&b=2", want: "https://example.com/Foo?a=1&b=2"},
		{name: "drops fragment", raw: "https://example.com/feed#top", want: "https://example.com/feed"},
		{name: "rejects userinfo", raw: "https://user:pass@example.com/feed", wantErr: true},
		{name: "rejects non-http scheme", raw: "ftp://example.com/feed", wantErr: true},
		{name: "rejects blank", raw: "   ", wantErr: true},
		{name: "rejects overlong", raw: "https://example.com/" + strings.Repeat("a", SubscriptionURLMaxRunes), wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := NormalizeSubscriptionURL(tc.raw)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}

// Medium 10：url.Parse 失败的信息不得透传原文（*url.Error 会拼上含 userinfo/凭据的完整 URL）。
func TestNormalizeSubscriptionURLSanitizesParseError(t *testing.T) {
	// 空格在 host 中使 url.Parse 失败；原文含凭据与主机。
	raw := "http://user:supersecret@exa mple.com/feed"
	_, err := NormalizeSubscriptionURL(raw)
	require.Error(t, err)
	require.NotContains(t, err.Error(), "supersecret", "不得回显凭据")
	require.NotContains(t, err.Error(), "exa mple.com", "不得回显原始 URL")
	require.NotContains(t, err.Error(), raw)
	require.Contains(t, err.Error(), "invalid feed url")
}

func TestParseFeedBodyParsesRSS(t *testing.T) {
	parsed, err := ParseFeedBody([]byte(testValidRSSBody))
	require.NoError(t, err)
	require.Equal(t, "Example", parsed.Title)
	require.Len(t, parsed.Entries, 1)

	_, err = ParseFeedBody([]byte("<html><body>nope</body></html>"))
	require.Error(t, err)
}
