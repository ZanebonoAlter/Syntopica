package service

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"gorm.io/gorm"

	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/safefetch"
)

// ── 共享建源服务（improve-discovery-recommendations design D7/D9）──
//
// 建源 = 「规范化地址 → 有界安全抓取 → 可解析 RSS/Atom 才算过 → 短事务按规范化
// URL 去重/建 Feed」。普通建源（reader CreateFeed handler）与推荐 accept 共用本
// 服务，消除两套路径：任何入口都不能绕过安全验证直写 Feed。
//
// 依赖方向：admin/service → reader/service（推荐 accept 复用），反向禁止；因此本包
// 的 NormalizeSubscriptionURL 与 admin/service.NormalizeRSSURL（候选身份 D1）
// 保持同一套规范化规则，而不是互相 import（会形成循环）。

const (
	// SubscriptionURLMaxRunes 是订阅地址总长上限（rune 计数，与 feeds.url
	// varchar(500) 兼容，不按字节截断中文）。
	SubscriptionURLMaxRunes = 500

	feedCreateDefaultTitle           = "Untitled Feed"
	feedCreateDefaultIcon            = "mdi:rss"
	feedCreateDefaultColor           = "#8b5cf6"
	feedCreateDefaultMaxArticles     = 100
	feedCreateDefaultRefreshInterval = 60
)

// 建源错误类别（handler / accept 据此映射 HTTP 状态与用户提示）。
var (
	// ErrInvalidFeedURL 表示地址本身不合法（空、超长、非 http/https、带 userinfo 等）。
	ErrInvalidFeedURL = errors.New("invalid feed url")
	// ErrFeedVerification 表示抓取或解析失败（私网拒绝/超时/重定向越界/非 RSS 内容等）。
	ErrFeedVerification = errors.New("feed verification failed")
)

// FetchFunc 是有界安全抓取的可替换实现（生产为 safefetch.Fetch）。
type FetchFunc func(ctx context.Context, rawURL string, opts safefetch.Options) (*safefetch.Result, error)

// subscriptionFetcher 是包级可替换抓取函数（存 FetchFunc）：生产用 safefetch.Fetch，
// 测试注入 mock（safefetch 只允许外网可达，单测必须替换）。用 atomic.Value 承载——
// 测试换装（SetSubscriptionFetcher）与在途请求读（VerifySubscriptionURLWithOptions）
// 可能并发发生，普通包级变量读写是数据竞争（-race 实锤：残留 goroutine 读 + 下个
// 用例 cleanup 还原写）。生产语义不变（只换实现不换契约）。
var subscriptionFetcher atomic.Value // 存 FetchFunc

// currentSubscriptionFetcher 取当前抓取实现；从未换装（atomic 为空值）时用生产实现。
func currentSubscriptionFetcher() FetchFunc {
	if f, ok := subscriptionFetcher.Load().(FetchFunc); ok && f != nil {
		return f
	}
	return safefetch.Fetch
}

// SetSubscriptionFetcher 覆盖抓取实现并返回还原函数（测试用）。f 为 nil 视为还原
// 生产实现（防 nil 解引用）。
func SetSubscriptionFetcher(f FetchFunc) func() {
	prev := currentSubscriptionFetcher()
	if f == nil {
		f = safefetch.Fetch
	}
	subscriptionFetcher.Store(f)
	return func() { subscriptionFetcher.Store(prev) }
}

// FeedCreateOptions 是建源时可选写入的 Feed 字段；零值走与既有 handler 一致的默认
// （Title=Untitled Feed / Icon=mdi:rss+fallback / Color=#8b5cf6 / MaxArticles=100 /
// RefreshInterval=60）。地址由服务规范化后写入，调用方不得自行传 URL。
type FeedCreateOptions struct {
	Title                 string
	Description           string
	CategoryID            *uint
	Icon                  string
	Color                 string
	MaxArticles           int
	RefreshInterval       int
	ArticleSummaryEnabled bool
	CompletionOnRefresh   bool
	MaxCompletionRetries  int
	FirecrawlEnabled      bool
	TaggingEnabled        bool
}

// FeedCreateService 是共享建源服务。
type FeedCreateService struct {
	db *gorm.DB
}

// NewFeedCreateService 构造（db 用于短事务内去重/建源）。
func NewFeedCreateService(db *gorm.DB) *FeedCreateService {
	return &FeedCreateService{db: db}
}

// CreateFeedWithVerification 完整建源：规范化 → 安全抓取 + RSS 解析 → 短事务按
// 规范化 URL 去重/建源。created=false 表示地址已存在（复用既有 Feed，不重复建）。
// 验证在事务外完成，网络调用绝不持有数据库事务。
func (s *FeedCreateService) CreateFeedWithVerification(ctx context.Context, rawURL string, opts FeedCreateOptions) (*models.Feed, bool, error) {
	normalized, err := s.VerifySubscriptionURL(ctx, rawURL)
	if err != nil {
		return nil, false, err
	}
	var (
		feed    *models.Feed
		created bool
	)
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		f, c, txErr := s.CreateOrReuseFeed(tx, normalized, opts)
		if txErr != nil {
			return txErr
		}
		feed, created = f, c
		return nil
	})
	if err != nil {
		return nil, false, err
	}
	return feed, created, nil
}

// VerifySubscriptionURL 校验一个订阅地址：规范化（≤ SubscriptionURLMaxRunes 个
// rune）→ safefetch 有界抓取 → HTTP 2xx 且响应体可解析为 RSS/Atom。成功返回可落
// Feed.URL 的规范化地址；失败返回分类错误（ErrInvalidFeedURL / ErrFeedVerification），
// 且错误信息不携带完整订阅地址（私网/越界场景不回显内网 URL）。
func (s *FeedCreateService) VerifySubscriptionURL(ctx context.Context, rawURL string) (string, error) {
	return s.VerifySubscriptionURLWithOptions(ctx, rawURL, safefetch.Options{})
}

// VerifySubscriptionURLWithOptions 是 VerifySubscriptionURL 的可注入 safefetch 选项版：
// 候选 access_scope=private_allowed（用户已确认授权）时由调用方传入仅含该候选端点
// 解析 IP 的 AllowedIPs（design D9：按来源放行指定 host，不是全局关闭 SSRF）。
func (s *FeedCreateService) VerifySubscriptionURLWithOptions(ctx context.Context, rawURL string, opts safefetch.Options) (string, error) {
	normalized, err := NormalizeSubscriptionURL(rawURL)
	if err != nil {
		return "", fmt.Errorf("%w: %w", ErrInvalidFeedURL, err)
	}
	res, err := currentSubscriptionFetcher()(ctx, normalized, opts)
	if err != nil {
		return "", fmt.Errorf("%w: %w", ErrFeedVerification, sanitizeFetchError(err))
	}
	if res == nil {
		return "", fmt.Errorf("%w: empty fetch result", ErrFeedVerification)
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return "", fmt.Errorf("%w: endpoint returned HTTP %d", ErrFeedVerification, res.StatusCode)
	}
	if _, err := ParseFeedBody(res.Body); err != nil {
		return "", fmt.Errorf("%w: response is not a parseable RSS/Atom feed", ErrFeedVerification)
	}
	return normalized, nil
}

// CreateOrReuseFeed 在给定事务句柄内按规范化 URL 查重：已存在 → 返回既有 Feed 且
// created=false（幂等，重复 accept 不产生第二个订阅）；不存在 → 按 opts 建 Feed。
// 本方法不开事务，供 accept 与建源组合进调用方的短事务。
func (s *FeedCreateService) CreateOrReuseFeed(tx *gorm.DB, normalizedURL string, opts FeedCreateOptions) (*models.Feed, bool, error) {
	var existing models.Feed
	err := tx.Where("url = ?", normalizedURL).First(&existing).Error
	if err == nil {
		return &existing, false, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, false, err
	}
	feed := buildFeedFromOptions(normalizedURL, opts)
	if err := tx.Create(&feed).Error; err != nil {
		return nil, false, err
	}
	return &feed, true, nil
}

// buildFeedFromOptions 由选项构造 Feed（默认值与既有 handler/accept 行为一致）。
func buildFeedFromOptions(normalizedURL string, opts FeedCreateOptions) models.Feed {
	now := time.Now()
	feed := models.Feed{
		Title:                 opts.Title,
		Description:           opts.Description,
		URL:                   normalizedURL,
		CategoryID:            opts.CategoryID,
		Icon:                  opts.Icon,
		Color:                 opts.Color,
		MaxArticles:           opts.MaxArticles,
		RefreshInterval:       opts.RefreshInterval,
		ArticleSummaryEnabled: opts.ArticleSummaryEnabled,
		CompletionOnRefresh:   opts.CompletionOnRefresh,
		MaxCompletionRetries:  opts.MaxCompletionRetries,
		FirecrawlEnabled:      opts.FirecrawlEnabled,
		TaggingEnabled:        opts.TaggingEnabled,
		LastUpdated:           &now,
	}
	if feed.Title == "" {
		feed.Title = feedCreateDefaultTitle
	}
	if feed.Icon == "" {
		feed.Icon = feedCreateDefaultIcon
		feed.IconSource = "fallback"
	} else {
		feed.IconSource = "custom"
	}
	if feed.Color == "" {
		feed.Color = feedCreateDefaultColor
	}
	if feed.MaxArticles == 0 {
		feed.MaxArticles = feedCreateDefaultMaxArticles
	}
	if feed.RefreshInterval == 0 {
		feed.RefreshInterval = feedCreateDefaultRefreshInterval
	}
	return feed
}

// NormalizeSubscriptionURL 规范化订阅地址（与候选身份 NormalizeRSSURL 同一套规则，
// design D1）：仅 http/https；host 小写；http:80 / https:443 默认端口移除；fragment
// 移除；path 大小写与 query 顺序原样保留；拒绝 userinfo、无 host、非法端口；输入总长
// ≤ SubscriptionURLMaxRunes 个 rune。返回可直接落 feeds.url 的地址。
func NormalizeSubscriptionURL(raw string) (string, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return "", errors.New("feed url must not be empty or blank")
	}
	if utf8.RuneCountInString(s) > SubscriptionURLMaxRunes {
		return "", fmt.Errorf("feed url exceeds %d runes", SubscriptionURLMaxRunes)
	}
	u, err := url.Parse(s)
	if err != nil {
		// Medium 10：url.Parse 的 *url.Error 会把完整原文（含 userinfo/凭据）拼进
		// 错误串，直接透传会在 handler 400 里回显；改为固定文案 + 原因类别。
		return "", fmt.Errorf("invalid feed url: %s", classifyURLParseError(err))
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", fmt.Errorf("feed url scheme must be http or https, got %q", u.Scheme)
	}
	if u.User != nil {
		return "", errors.New("feed url must not contain userinfo")
	}
	hostname := u.Hostname()
	if hostname == "" {
		return "", errors.New("feed url must contain a host")
	}
	port := u.Port()
	if port != "" {
		n, err := strconv.Atoi(port)
		if err != nil || n <= 0 || n > 65535 {
			return "", fmt.Errorf("feed url port %q is invalid", port)
		}
		if (u.Scheme == "http" && n == 80) || (u.Scheme == "https" && n == 443) {
			port = ""
		}
	}
	host := strings.ToLower(hostname)
	if strings.Contains(host, ":") { // IPv6 字面量补回方括号
		host = "[" + host + "]"
	}
	if port != "" {
		host = host + ":" + port
	}
	u.Host = host
	u.Fragment = ""
	u.RawFragment = ""
	return u.String(), nil
}

// sanitizeFetchError 保证返回的错误不携带完整订阅 URL：私网拒绝/重定向越界/超时/
// 体积超限等类型化错误原样透传（其文案只含已解析 IP 或固定短语），未分类错误剥离
// 可能内嵌完整 URL 的 *url.Error 外壳，只保留底层原因（design D7「无权探测返回脱敏
// 错误」）。
func sanitizeFetchError(err error) error {
	if err == nil {
		return nil
	}
	var privateErr *safefetch.PrivateAddressError
	switch {
	case errors.As(err, &privateErr):
		return privateErr
	case errors.Is(err, safefetch.ErrPrivateAddress),
		errors.Is(err, safefetch.ErrTooManyRedirects),
		errors.Is(err, safefetch.ErrTimeout),
		errors.Is(err, safefetch.ErrUnsupportedScheme),
		errors.Is(err, safefetch.ErrBodyTooLarge):
		return err
	}
	var urlErr *url.Error
	if errors.As(err, &urlErr) && urlErr.Err != nil {
		return fmt.Errorf("request failed: %w", urlErr.Err)
	}
	return err
}

// classifyURLParseError 把 url.Parse 错误归类为固定短语（Medium 10）：*url.Error 的
// Error() 会拼上完整原文（含 userinfo/凭据），不得透传；只暴露原因类别，不回显 URL。
func classifyURLParseError(err error) string {
	var urlErr *url.Error
	inner := err
	if errors.As(err, &urlErr) && urlErr.Err != nil {
		inner = urlErr.Err
	}
	msg := inner.Error()
	switch {
	case strings.Contains(msg, "invalid port"):
		return "invalid port"
	case strings.Contains(msg, "invalid URL escape"):
		return "invalid escape sequence"
	case strings.Contains(msg, "invalid control character"):
		return "invalid control character"
	case strings.Contains(msg, "missing protocol scheme"):
		return "missing scheme"
	case strings.Contains(msg, "invalid URI for request"):
		return "malformed url"
	default:
		return "malformed url"
	}
}
