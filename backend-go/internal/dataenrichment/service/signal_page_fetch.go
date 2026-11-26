package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"

	md "github.com/JohannesKaufmann/html-to-markdown"
	"github.com/go-shiori/go-readability"

	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/safefetch"
)

// ── signal 专用安全页面适配（add-question-driven-news-research D4）───────────
//
// 单独的安全页面适配，不扩改全站 crawler：先经 platform/safefetch 取得有界
// 响应（逐跳 DNS+固定 IP 拨号、禁私网/metadata、代理置空、2MiB/10s/3跳），
// 再把 bytes 与 final URL 交既有 readability 依赖解析。禁止先用
// readability.FromURL 抓完再做 URL 校验——现有 ReaderPageFetcher/旧调查保持
// 原授权与行为，本适配只服务 question-v1 研究。

// 网页正文上限（D4：网页正文最多 4000rune，模型与附录使用相同片段）。
const SignalWebPageMaxRunes = 4000

// typed gap 种类（S8：失败有类型和原因，不冒充成功）。
const (
	signalPageGapUnsafeURL     = "unsafe_url"     // 非法/非 http(s)/带凭证
	signalPageGapUnsafeTarget  = "unsafe_target"  // 私网/loopback/metadata/重定向内网
	signalPageGapTooLarge      = "too_large"      // 超响应体积
	signalPageGapTimeout       = "timeout"        // 超时
	signalPageGapNotHTML       = "not_html"       // 非 HTML/text 内容
	signalPageGapHTTPStatus    = "http_status"    // 非 2xx
	signalPageGapEmptyBody     = "empty_body"     // 空正文
	signalPageGapFetchFailed   = "fetch_failed"   // 传输层失败
	signalPageGapUnknownDate   = "unknown_date"   // 无可验证日期（正文不进事实材料）
	signalPageGapLateDate      = "late_date"      // 日期晚于 cutoff（事后材料）
	signalPageGapNotConfigured = "not_configured" // 适配未接线
)

// SignalPageFetchError 是安全抓页的 typed gap：Kind 程序可判，Detail 携带底
// 层原因（传输层错误必须携带底层原因——约束 37）。
type SignalPageFetchError struct {
	Kind   string
	Detail string
}

func (e *SignalPageFetchError) Error() string {
	return fmt.Sprintf("signal page fetch [%s]: %s", e.Kind, e.Detail)
}

// SignalPageFetchResult 是一次成功抓取并解析后的产物：正文（≤4000rune，
// 模型与附录同片段）、声明发布/更新时间（机器元数据，出处可查）与 hash。
type SignalPageFetchResult struct {
	Title         string
	FinalURL      string
	Text          string
	Truncated     bool
	OriginalRunes int
	PublishedAt   *time.Time
	ModifiedAt    *time.Time
	PublishedFrom string
	ContentHash   string
}

// SignalSafePageFetcher 经 safefetch + readability-from-bytes 的安全页面适配。
type SignalSafePageFetcher struct {
	options safefetch.Options
}

// NewSignalSafePageFetcher 构造默认限制（10s/2MiB/3跳，AllowedIPs 空=私网
// 全拒）的生产适配。
func NewSignalSafePageFetcher() *SignalSafePageFetcher {
	return &SignalSafePageFetcher{}
}

// NewSignalSafePageFetcherWithOptions 注入自定义安全选项（测试用显式
// AllowedIPs 授权 loopback httptest；授权按请求，绝不全局放开）。
func NewSignalSafePageFetcherWithOptions(opts safefetch.Options) *SignalSafePageFetcher {
	return &SignalSafePageFetcher{options: opts}
}

// FetchPage implements SignalPageFetchAdapter（runtime 只认接口；nil 适配=
// 未配置 typed gap）。
func (f *SignalSafePageFetcher) FetchPage(ctx context.Context, rawURL string) (*SignalPageFetchResult, error) {
	return f.Fetch(ctx, rawURL)
}

// Fetch 抓取并解析一个 URL：normalize（URL 门）→ safefetch（网络边界）→
// 状态/内容类型检查 → readability bytes 解析（含日期提取与截取）。
func (f *SignalSafePageFetcher) Fetch(ctx context.Context, rawURL string) (*SignalPageFetchResult, error) {
	norm, err := normalizeSignalWebURL(rawURL)
	if err != nil {
		return nil, &SignalPageFetchError{Kind: signalPageGapUnsafeURL, Detail: err.Error()}
	}
	res, err := safefetch.Fetch(ctx, norm, f.options)
	if err != nil {
		return nil, &SignalPageFetchError{Kind: mapSafeFetchError(err), Detail: err.Error()}
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return nil, &SignalPageFetchError{Kind: signalPageGapHTTPStatus, Detail: fmt.Sprintf("http status %d", res.StatusCode)}
	}
	if !signalFetchableContentType(res.ContentType) {
		return nil, &SignalPageFetchError{Kind: signalPageGapNotHTML, Detail: fmt.Sprintf("content-type %q 非 HTML/text", res.ContentType)}
	}
	out, perr := parseSignalFetchedPage(string(res.Body), res.FinalURL, time.Now())
	if perr != nil {
		return nil, perr
	}
	return out, nil
}

// mapSafeFetchError 把 safefetch 错误归入 typed gap 种类。
func mapSafeFetchError(err error) string {
	var pae *safefetch.PrivateAddressError
	switch {
	case errors.As(err, &pae):
		return signalPageGapUnsafeTarget
	case errors.Is(err, safefetch.ErrBodyTooLarge):
		return signalPageGapTooLarge
	case errors.Is(err, safefetch.ErrTimeout):
		return signalPageGapTimeout
	case errors.Is(err, safefetch.ErrUnsupportedScheme):
		return signalPageGapUnsafeURL
	case errors.Is(err, safefetch.ErrTooManyRedirects):
		return signalPageGapFetchFailed
	default:
		return signalPageGapFetchFailed
	}
}

// signalFetchableContentType 只接受 HTML/text（不下载 PDF、不装浏览器）。
func signalFetchableContentType(ct string) bool {
	c := strings.ToLower(strings.TrimSpace(ct))
	return strings.HasPrefix(c, "text/html") || strings.HasPrefix(c, "text/plain") || strings.HasPrefix(c, "application/xhtml")
}

// ── URL 门（归一化：去掉 userinfo/fragment、host 小写、保留 query）────────

// normalizeSignalWebURL 归一化一个外搜/抓页 URL：仅 http(s)；拒绝 userinfo
// 凭证；去掉 fragment；host 小写归一；query string 原样保留（不因归一化混
// 合不同文档）。非法输入返回错误（模型自造 URL 在入口即拒）。
func normalizeSignalWebURL(raw string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return "", fmt.Errorf("URL 无法解析：%q", raw)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", fmt.Errorf("仅允许 http(s) URL，收到 scheme %q", u.Scheme)
	}
	if u.User != nil {
		return "", errors.New("拒绝带凭证（userinfo）的 URL")
	}
	if u.Host == "" {
		return "", errors.New("URL 缺少 host")
	}
	u.Host = strings.ToLower(u.Host)
	u.Fragment = ""
	u.RawFragment = ""
	return u.String(), nil
}

// ── 正文解析（readability from bytes + 机器元数据日期）──────────────────────

// parseSignalFetchedPage 把已取回的 HTML bytes 交 readability 解析正文并转
// markdown 纯文本（不执行脚本、不渲染 HTML/CSS），提取机器元数据日期并按
// 4000rune 截取（truncated/original_runes 如实标注，模型与附录同片段）。
// 纯函数：安全适配与测试 fake 共用同一解析路径。
func parseSignalFetchedPage(html, finalURL string, now time.Time) (*SignalPageFetchResult, error) {
	if strings.TrimSpace(html) == "" {
		return nil, &SignalPageFetchError{Kind: signalPageGapEmptyBody, Detail: "响应体为空"}
	}
	parsedURL, err := url.Parse(finalURL)
	if err != nil || parsedURL.Host == "" {
		parsedURL = &url.URL{Scheme: "https", Host: "example.invalid"}
	}
	article, rerr := readability.FromReader(bytes.NewReader([]byte(html)), parsedURL)
	if rerr != nil {
		return nil, &SignalPageFetchError{Kind: signalPageGapNotHTML, Detail: "readability 解析失败：" + rerr.Error()}
	}
	// L3：不用 readability Excerpt 兜底——摘要形态不得充当可引用正文；空正文
	// 按既有 empty_body typed gap 处理（spec「来源截取与空正文」同语义）。
	text := article.Content
	if !signalUsableWebText(text) {
		return nil, &SignalPageFetchError{Kind: signalPageGapEmptyBody, Detail: "正文为空或不可读"}
	}
	// readability 返回 HTML 片段；转 markdown 纯文本（脚本/样式/标签不保留）。
	converter := md.NewConverter("", true, nil)
	if markdown, cerr := converter.ConvertString(text); cerr == nil && strings.TrimSpace(markdown) != "" {
		text = markdown
	}
	bounded, truncated, original := boundSignalWebText(text)
	pub, mod, from := extractSignalPageDates(html)
	return &SignalPageFetchResult{
		Title:         article.Title,
		FinalURL:      finalURL,
		Text:          bounded,
		Truncated:     truncated,
		OriginalRunes: original,
		PublishedAt:   pub,
		ModifiedAt:    mod,
		PublishedFrom: from,
		ContentHash:   hashSignalText(bounded),
	}, nil
}

// boundSignalWebText 把网页正文截到 SignalWebPageMaxRunes（rune 感知，CJK
// 不切半个字）。
func boundSignalWebText(s string) (string, bool, int) {
	runes := []rune(s)
	original := len(runes)
	if original <= SignalWebPageMaxRunes {
		return s, false, original
	}
	return string(runes[:SignalWebPageMaxRunes]), true, original
}

// signalUsableWebText 判定网页正文可用（非空白）。
func signalUsableWebText(s string) bool {
	return strings.TrimSpace(s) != ""
}

// ── 机器元数据日期提取（D5：不从 URL/snippet/模型猜日期）────────────────────

var (
	signalMetaPublishedRe = regexp.MustCompile(`<meta[^>]+(?:property|name)=["']article:published_time["'][^>]+content=["']([^"']+)["']`)
	signalMetaContentPub  = regexp.MustCompile(`<meta[^>]+content=["']([^"']+)["'][^>]+(?:property|name)=["']article:published_time["']`)
	signalMetaModifiedRe  = regexp.MustCompile(`<meta[^>]+(?:property|name)=["']article:modified_time["'][^>]+content=["']([^"']+)["']`)
	signalMetaContentMod  = regexp.MustCompile(`<meta[^>]+content=["']([^"']+)["'][^>]+(?:property|name)=["']article:modified_time["']`)
	// L2：JSON-LD 键只在 <script type="application/ld+json"> 块内匹配——页面
	// 任意位置的普通 JS 对象同名键不是结构化数据，不应被当作机器元数据。
	signalJSONLDScriptRe = regexp.MustCompile(`(?is)<script[^>]+type=["']application/ld\+json["'][^>]*>(.*?)</script>`)
	signalJSONLDPubRe    = regexp.MustCompile(`"datePublished"\s*:\s*"([^"]+)"`)
	signalJSONLDModRe    = regexp.MustCompile(`"dateModified"\s*:\s*"([^"]+)"`)
)

// extractSignalPageDates 从限定机器元数据提取发布/更新日期：meta
// article:published_time / article:modified_time 与已支持的 JSON-LD
// Article/NewsArticle 键（datePublished/dateModified）。只处理这些受支持
// 键，不解析复杂任意 JSON 脚本；解析失败/无日期 → nil（调用方记 gap，不猜）。
// 返回的 from 是字段出处（可核查）。
func extractSignalPageDates(html string) (published, modified *time.Time, from string) {
	// L2：JSON-LD 键只在 <script type="application/ld+json"> 块内匹配（块文
	// 本拼接后供键级正则使用；页面普通 JS 对象同名键不是机器元数据）。
	var ld strings.Builder
	for _, m := range signalJSONLDScriptRe.FindAllStringSubmatch(html, -1) {
		ld.WriteString(m[1])
		ld.WriteByte('\n')
	}
	ldText := ld.String()
	type hit struct {
		t *time.Time
		f string
	}
	grab := func(re *regexp.Regexp, hay string) hit {
		if m := re.FindStringSubmatch(hay); m != nil {
			if t, ok := parseSignalPageDate(m[1]); ok {
				return hit{t, strings.Split(re.String(), "[")[0]} // 出处=命中的元数据键
			}
		}
		return hit{}
	}
	first := func(hits ...hit) hit {
		for _, h := range hits {
			if h.t != nil {
				return h
			}
		}
		return hit{}
	}
	if p := first(grab(signalMetaPublishedRe, html), grab(signalMetaContentPub, html), grab(signalJSONLDPubRe, ldText)); p.t != nil {
		published, from = p.t, p.f
	}
	if m := first(grab(signalMetaModifiedRe, html), grab(signalMetaContentMod, html), grab(signalJSONLDModRe, ldText)); m.t != nil {
		modified = m.t
	}
	return published, modified, from
}

// parseSignalPageDate 解析页面日期字符串：RFC3339（带时区按时间戳比较）、
// 常见无时区格式按上海时区，纯日期由调用侧的日末判定处理（此处按当日零点
// 返回，日末语义由 signalDateOnlyEndOfDay 单独提供）。
func parseSignalPageDate(v string) (*time.Time, bool) {
	s := strings.TrimSpace(v)
	if s == "" {
		return nil, false
	}
	// 纯日期按上海时区日末保守判断（当日未结束不能冒充已合窗）。
	if end, ok := signalDateOnlyEndOfDay(s); ok {
		return &end, true
	}
	loc := shanghaiLocation()
	formats := []string{time.RFC3339Nano, time.RFC3339, "2006-01-02T15:04:05", "2006-01-02 15:04:05"}
	for _, f := range formats {
		// L1：无时区格式按上海时区解释（带时区的 RFC3339 仍按自身偏移）。
		if t, err := time.ParseInLocation(f, s, loc); err == nil {
			return &t, true
		}
	}
	return nil, false
}

// signalWebEvidenceTime 取发布/更新日期中更晚的可解析值作边界（不能用更早
// 值绕过过滤）；都为 nil 返回 nil（日期未知→不可引用）。
func signalWebEvidenceTime(published, modified *time.Time) *time.Time {
	if published == nil {
		return modified
	}
	if modified == nil {
		return published
	}
	if modified.After(*published) {
		return modified
	}
	return published
}

// signalDateOnlyEndOfDay 把纯日期（YYYY-MM-DD）解析为上海时区该日最后一刻
// （23:59:59.999999999）：结束时刻不晚于 cutoff 才收录（保守判断，当前未
// 结束日不能冒充已合窗）。
func signalDateOnlyEndOfDay(date string) (time.Time, bool) {
	t, err := time.ParseInLocation("2006-01-02", strings.TrimSpace(date), shanghaiLocation())
	if err != nil {
		return time.Time{}, false
	}
	end := t.Add(24*time.Hour - time.Nanosecond)
	return end, true
}

func shanghaiLocation() *time.Location {
	return models.ShanghaiTZ
}

// jsonUnmarshalStrict 供 decodeUintList 使用（本地小包装避免多处重复）。
func jsonUnmarshalStrict(raw []byte, v any) error {
	dec := json.NewDecoder(bytes.NewReader(raw))
	return dec.Decode(v)
}
