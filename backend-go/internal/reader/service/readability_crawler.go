package service

import (
	"context"
	"fmt"
	"net/http"
	nurl "net/url"
	"strings"
	"time"

	"github.com/JohannesKaufmann/html-to-markdown"
	"github.com/go-shiori/go-readability"
	"go.opentelemetry.io/otel"

	"syntopica-backend/internal/platform/httpclient"
	"syntopica-backend/internal/platform/tracing"
)

// minUsableContentRunes 是判定 readability 结果"合格可用"的最小正文字数（按 rune 计）。
// 对齐 ContentCompletionService.IsContentIncomplete 的 200 字阈值但更保守，取 500。
const minUsableContentRunes = 500

// maxJunkSignalHits 是正文允许的最大垃圾特征词命中数；超过即判定为 SPA 空壳/页脚垃圾。
// 调研实测：36氪页脚命中 3 个（备案+ICP+版权），博客园真实正文命中 0 个。
const maxJunkSignalHits = 2 // <3 即命中 ≥3 判失败

// junkSignals 是 SPA 空壳内容（页脚/备案/导航）的共性特征词。
// 误判方向是"保守多降级"——漏掉一个词只导致多调一次 Firecrawl，不会降低正文质量。
var junkSignals = []string{
	"备案",
	"ICP",
	"版权",
	"Copyright",
	"©",
	"举报电话",
	"All Rights",
	"京ICP",
	"登录去登录",
}

// isUsableArticle 判定 readability 提取的 Markdown 是否为合格正文。
// 双重校验：长度 ≥500 字 且 垃圾特征词命中 ≤2 个。任一不满足返回 false，触发降级。
func isUsableArticle(md string) bool {
	if len([]rune(md)) < minUsableContentRunes {
		return false
	}
	hits := 0
	for _, sig := range junkSignals {
		if strings.Contains(md, sig) {
			hits++
		}
	}
	return hits <= maxJunkSignalHits
}

// desktopUserAgent 模拟桌面浏览器，避免被简单 UA 黑名单挡掉。
const desktopUserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 " +
	"(KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36"

// ReadabilityCrawler 是进程内纯 Go 正文抓取器，零外部服务依赖。
// 基于 go-readability（Mozilla Readability.js 移植）+ html-to-markdown。
type ReadabilityCrawler struct {
	client *http.Client
}

// NewReadabilityCrawler 构造一个进程内 readability 抓取器。
func NewReadabilityCrawler() *ReadabilityCrawler {
	return &ReadabilityCrawler{
		// 正文提取属外部内容抓取链路（narrow-outbound-proxy-scope）：
		// 抓取经工厂 client（跟随全局出站代理 + 熔断回退），readability 只做解析。
		client: httpclient.New(httpclient.WithTimeout(30*time.Second), httpclient.WithCrawlScope()),
	}
}

// ScrapePage 抓取 URL，用 readability 提正文后转 Markdown，返回中立 ScrapeResult。
// 抓取走工厂 client（WithCrawlScope，跟随全局出站代理）：readability.FromURL
// 内部自建裸 client 不受代理/熔断管控，故改为「自行抓取 + 库仅解析」两段式。
// 注意：返回结果是否"合格"需调用方用 isUsableArticle 判定；本方法不自行过滤。
func (c *ReadabilityCrawler) ScrapePage(ctx context.Context, url string) (*ScrapeResult, error) {
	_, span := otel.Tracer(tracing.ServiceName).Start(ctx, "ReadabilityCrawler.ScrapePage")
	defer span.End()

	parsedURL, err := nurl.ParseRequestURI(url)
	if err != nil {
		return nil, fmt.Errorf("failed to parse URL: %v", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", desktopUserAgent)
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	// 与 readability.FromURL 同款前置校验：非 HTML 不进解析器。
	if cp := resp.Header.Get("Content-Type"); !strings.Contains(cp, "text/html") {
		return nil, fmt.Errorf("URL is not a HTML document")
	}

	parser := readability.NewParser()
	article, err := parser.Parse(resp.Body, parsedURL)
	if err != nil {
		return nil, err
	}

	conv := md.NewConverter("", true, nil)
	md, err := conv.ConvertString(article.Content)
	if err != nil {
		return nil, err
	}

	return &ScrapeResult{
		Markdown: md,
		HTML:     article.Content,
		Title:    article.Title,
		OGImage:  article.Image,
		Source:   "readability",
	}, nil
}
