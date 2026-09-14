package service

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/mmcdole/gofeed"
	"golang.org/x/net/html"

	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/httpclient"
)

type RSSParser struct {
	client *http.Client
}

func NewRSSParser() *RSSParser {
	return &RSSParser{
		client: httpclient.New(httpclient.WithTimeout(20 * time.Second)),
	}
}

type ParsedFeed struct {
	Title       string
	Description string
	Link        string
	Image       string
	Language    string
	Entries     []ParsedEntry
}

type ParsedEntry struct {
	Title       string
	Link        string
	Description string
	Content     string
	PubDate     *time.Time
	Author      string
	Tags        []string
	ImageURL    string
}

func (p *RSSParser) ParseFeedURL(feedURL string) (*ParsedFeed, error) {
	resp, err := p.client.Get(feedURL)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch feed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("feed returned HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read feed body: %w", err)
	}

	return ParseFeedBody(body)
}

// ParseFeedBody 解析已获取的 feed 响应体（RSS/Atom）为 ParsedFeed，不发起网络请求。
// 与 ParseFeedURL 的解析段语义逐字一致（UTF-8 清洗 → gofeed → 统一结构）：解析失败
// 即「不是可解析的 RSS/Atom」，共享建源服务据此拒绝非 feed 响应（design D7/D9）。
func ParseFeedBody(body []byte) (*ParsedFeed, error) {
	cleaned := sanitizeUTF8(body)

	fp := gofeed.NewParser()
	feed, err := fp.Parse(bytes.NewReader(cleaned))
	if err != nil {
		return nil, fmt.Errorf("failed to parse feed: %w", err)
	}

	return new(RSSParser).convertGofeedToParsed(feed), nil
}

func sanitizeUTF8(data []byte) []byte {
	if utf8.Valid(data) {
		return data
	}
	valid := make([]byte, 0, len(data))
	for i := 0; i < len(data); {
		_, size := utf8.DecodeRune(data[i:])
		switch {
		case size == 1 && data[i] < 0x80:
			valid = append(valid, data[i])
			i++
		case size == 1:
			valid = append(valid, replacement...)
			i++
		default:
			valid = append(valid, data[i:i+size]...)
			i += size
		}
	}
	return valid
}

var replacement = []byte("\uFFFD")

func (p *RSSParser) convertGofeedToParsed(feed *gofeed.Feed) *ParsedFeed {
	parsed := &ParsedFeed{
		Title:       feed.Title,
		Description: feed.Description,
		Link:        feed.Link,
		Image:       extractFeedImage(feed),
		Language:    feed.Language,
		Entries:     make([]ParsedEntry, 0, len(feed.Items)),
	}

	for _, item := range feed.Items {
		entry := ParsedEntry{
			Title:       item.Title,
			Link:        item.Link,
			Description: item.Description,
			Content:     extractContent(item),
			PubDate:     parseDate(item),
			Author:      extractAuthor(item),
			Tags:        extractFeedTags(item),
			ImageURL:    extractItemImage(item),
		}

		if entry.Title == "" {
			entry.Title = "No title"
		}

		parsed.Entries = append(parsed.Entries, entry)
	}

	return parsed
}

func extractFeedImage(feed *gofeed.Feed) string {
	if feed.Image != nil {
		return feed.Image.URL
	}
	return ""
}

func extractContent(item *gofeed.Item) string {
	if item.Content != "" {
		return item.Content
	}
	return item.Description
}

func parseDate(item *gofeed.Item) *time.Time {
	if item.PublishedParsed != nil {
		cstTime := item.PublishedParsed.In(models.ShanghaiTZ)
		return &cstTime
	}
	if item.UpdatedParsed != nil {
		cstTime := item.UpdatedParsed.In(models.ShanghaiTZ)
		return &cstTime
	}
	return nil
}

func extractAuthor(item *gofeed.Item) string {
	if item.Author != nil {
		return item.Author.Name
	}
	return ""
}

func extractFeedTags(item *gofeed.Item) []string {
	tags := append([]string{}, item.Categories...)
	return tags
}

// extractItemImage resolves the best available preview image for a feed item.
//
// Priority (covers the common image carriers that gofeed itself misses):
//  1. item.Image — gofeed already aggregates enclosure, media:content,
//     itunes:image and the first <img> in content/description here.
//  2. <media:thumbnail> — a known gofeed gap; many news/podcast feeds expose
//     the thumbnail only through this Media RSS element.
//  3. <enclosure type="image/*"> — final fallback for feeds that rely solely
//     on enclosures but whose type gofeed failed to classify into item.Image.
func extractItemImage(item *gofeed.Item) string {
	if item.Image != nil && item.Image.URL != "" {
		return item.Image.URL
	}
	if url := mediaThumbnailURL(item); url != "" {
		return url
	}
	for _, enc := range item.Enclosures {
		if strings.HasPrefix(enc.Type, "image/") && enc.URL != "" {
			return enc.URL
		}
	}
	return ""
}

// mediaThumbnailURL pulls the first <media:thumbnail url="..."> from the
// Media RSS extension. Gofeed stores Media RSS under Extensions["media"] and
// never surfaces thumbnails on item.Image, so this must be read explicitly.
func mediaThumbnailURL(item *gofeed.Item) string {
	media := item.Extensions["media"]
	thumbs := media["thumbnail"]
	for _, t := range thumbs {
		if u := t.Attrs["url"]; u != "" {
			return u
		}
	}
	return ""
}

// FetchFaviconURL derives a site's favicon URL from its homepage URL.
//
// siteURL should be the content site's homepage (typically the RSS channel
// <link>), NOT the RSS feed URL — using the feed URL would yield the
// aggregator's favicon (feedburner, rsshub, etc.) rather than the content
// site's. Returns "" when the URL can't be parsed so callers keep the feed in
// the fallback state rather than showing a broken image.
func (p *RSSParser) FetchFaviconURL(siteURL string) string {
	parsedURL, err := url.Parse(siteURL)
	if err != nil || parsedURL.Host == "" || parsedURL.Scheme == "" {
		return ""
	}
	return fmt.Sprintf("%s://%s/favicon.ico", parsedURL.Scheme, parsedURL.Host)
}

const (
	// maxHomepageBytes caps how much of a site homepage is read for icon link
	// discovery (spec: ≤512KB).
	maxHomepageBytes = 512 * 1024
	// homepageTimeout bounds the homepage fetch (spec: ≤5s).
	homepageTimeout = 5 * time.Second
)

// ProbeFaviconCandidates returns favicon candidate URLs for a content site in
// priority order: homepage <link rel="icon"> href (resolved to absolute) then
// {scheme}://{host}/favicon.ico. Returns nil when siteURL is empty or
// unparseable so the caller keeps the feed in fallback state. The homepage
// fetch is bounded (512KB / 5s) and failures only drop the HTML tier — the
// /favicon.ico guess is always the last candidate.
func (p *RSSParser) ProbeFaviconCandidates(siteURL string) []string {
	guess := p.FetchFaviconURL(siteURL)
	if guess == "" {
		return nil
	}
	base, _ := url.Parse(siteURL)
	homeURL := siteURL
	if base != nil && base.Scheme != "" && base.Host != "" {
		homeURL = base.Scheme + "://" + base.Host + "/"
	}
	// RSS channel link 常带具体路径（如 https://wallstreetcn.com/live/global），
	// 该页面往往因 SPA/鉴权返回非 200 或缺少 <link rel="icon">。先试 siteURL
	// 指向的页面，失败再回退站点首页（favicon 声明一般在首页 <head>）。
	for _, page := range []string{siteURL, homeURL} {
		if page == "" {
			continue
		}
		href := p.fetchIconLinkHref(page)
		if href == "" {
			continue
		}
		if abs := resolveIconURL(base, href); abs != "" {
			// The <link> href resolved to the same URL the /favicon.ico guess
			// produces — list it once to avoid a duplicate download.
			if abs == guess {
				return []string{guess}
			}
			return []string{abs, guess}
		}
	}
	return []string{guess}
}

// fetchIconLinkHref fetches the site homepage and returns the first
// <link rel="icon|shortcut icon|apple-touch-icon" href="..."> value, or "".
// Non-200 responses, oversized bodies and parse errors all yield "" — the
// caller falls back to the /favicon.ico guess.
func (p *RSSParser) fetchIconLinkHref(homepageURL string) string {
	client := httpclient.New(httpclient.WithTimeout(homepageTimeout))
	resp, err := client.Get(homepageURL)
	if err != nil {
		return ""
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return ""
	}

	tz := html.NewTokenizer(io.LimitReader(resp.Body, maxHomepageBytes+1))
	for {
		tt := tz.Next()
		switch tt {
		case html.ErrorToken:
			return ""
		case html.StartTagToken, html.SelfClosingTagToken:
			name, _ := tz.TagName()
			if string(name) != "link" {
				continue
			}
			if rel, href := linkAttrs(tz); isIconRel(rel) && href != "" {
				return href
			}
		}
	}
}

// linkAttrs reads the attributes of the current <link> start tag.
func linkAttrs(tz *html.Tokenizer) (rel, href string) {
	for {
		key, val, more := tz.TagAttr()
		switch strings.ToLower(string(key)) {
		case "rel":
			rel = strings.ToLower(string(val))
		case "href":
			href = string(val)
		}
		if !more {
			return rel, href
		}
	}
}

// isIconRel reports whether a link rel attribute declares a favicon relation
// (icon, shortcut icon, apple-touch-icon, ...).
func isIconRel(rel string) bool {
	for _, token := range strings.Fields(rel) {
		if token == "icon" || token == "apple-touch-icon" || token == "shortcut" {
			return true
		}
	}
	return false
}

// resolveIconURL resolves an href against a site base URL, returning the
// absolute URL (href may be absolute or relative).
func resolveIconURL(base *url.URL, href string) string {
	ref, err := url.Parse(href)
	if err != nil {
		return ""
	}
	return base.ResolveReference(ref).String()
}

func (p *RSSParser) FetchFeedMetadata(feedURL string) (title, description string, err error) {
	parsed, err := p.ParseFeedURL(feedURL)
	if err != nil {
		return "", "", err
	}

	return parsed.Title, parsed.Description, nil
}
