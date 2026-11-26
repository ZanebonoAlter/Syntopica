// Package textutil holds pure text-manipulation helpers shared across layers.
// It is a leaf package (no syntopica-internal dependencies) so it can be
// imported from both internal/platform/database and
// internal/tagmanagement/service/core without forming an import cycle
// (core itself depends on database via quality_score, so database cannot
// import core back).
package textutil

import (
	"regexp"
	"strings"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
)

// whitespacePattern matches any run of whitespace (spaces, tabs, newlines,
// non-breaking spaces \u00a0, etc.).
var whitespacePattern = regexp.MustCompile(`\s+`)

// NormalizeLabelKey removes all whitespace and lowercases the value.
//
// Two labels whose normalize key matches are treated as the same entity
// (e.g. "SK 海力士" and "SK海力士" -> "sk海力士"). This is the SINGLE shared
// implementation used by both the auxiliary-label create-side dedup gate (L1)
// and the one-shot text-variant merge migration (design D5/D6 + auxiliary-label
// spec: "该归一化键 SHALL 与一次性迁移所用归一化函数为同一实现"), so that new
// labels created after the migration cannot re-introduce text-variant duplicates.
func NormalizeLabelKey(value string) string {
	cleaned := strings.ToLower(strings.TrimSpace(value))
	cleaned = whitespacePattern.ReplaceAllString(cleaned, "")
	return cleaned
}

// StripURLFragment removes the URL fragment (#...) from a link, preserving
// `#!` hashbang fragments (SPA route identity on some sites).
//
// Some feeds append a drifting anchor to every entry link — V2EX's #replyN is
// the reply count at feed-generation time — so the same story changes link
// between refreshes and slips past the (feed_id, link) dedupe as a "new"
// article. The fragment is client-side only and never reaches the server, so
// stripping it loses nothing. This is the SINGLE shared implementation used by
// both the RSS parser (ingestion-time link normalization) and the one-shot
// stored-link migration, so freshly parsed entries and migrated rows always
// compare equal.
func StripURLFragment(link string) string {
	i := strings.IndexByte(link, '#')
	if i < 0 || (i+1 < len(link) && link[i+1] == '!') {
		return link
	}
	return link[:i]
}

// NormalizeArticleLink 把文章链接规范成「内容身份键」（fix-duplicate-article-heat
// design D4）：同一篇内容被两个 feed 各抓一份时必须得到同一个键，下游才能按
// 内容去重而不是按 feed 计数。
//
// 规则（顺序固定，纯函数、无 IO）：
//  1. 去首尾空白；空输入返回 ""；
//  2. 斩掉 fragment——复用 StripURLFragment（ingestion 与存量链接迁移共用的
//     单一实现，保留 `#!` hashbang 特例）；
//  3. 斩掉 query（从第一个 "?" 起）；
//  4. scheme 与 host 转小写，path 原样保留。
//
// 第 4 步手工切分 `scheme://host[/path]`，**不用 url.Parse 全量重写**：后者会
// 改写 path 语义（尾斜杠、百分号编码、点段），与 rss-article-dedup 的
// `(feed_id, link)` 判重口径打架。不含 "://" 的文本原样返回（只做过
// trim/fragment/query）；超长值不截断。
func NormalizeArticleLink(link string) string {
	cleaned := strings.TrimSpace(link)
	if cleaned == "" {
		return ""
	}
	cleaned = StripURLFragment(cleaned)
	if i := strings.IndexByte(cleaned, '?'); i >= 0 {
		cleaned = cleaned[:i]
	}
	scheme, rest, ok := strings.Cut(cleaned, "://")
	if !ok || !isURLScheme(scheme) {
		return cleaned
	}
	host, path, hasPath := strings.Cut(rest, "/")
	host = strings.ToLower(host)
	scheme = strings.ToLower(scheme)
	if !hasPath {
		return scheme + "://" + host
	}
	return scheme + "://" + host + "/" + path
}

// isURLScheme 判断 s 是否为 RFC 3986 的 scheme 记号
// (ALPHA *( ALPHA / DIGIT / "+" / "-" / "." ))：挡住「只是正文里含 ://」的
// 文本，避免小写化步骤把 host 部分改坏。
func isURLScheme(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z':
		case i > 0 && (c >= '0' && c <= '9' || c == '+' || c == '-' || c == '.'):
		default:
			return false
		}
	}
	return true
}

// subjectKeySeparatorPattern matches runs of whitespace or hyphens inside a
// subject key's canonical-name part ("Donald-Trump" / "donald trump").
var subjectKeySeparatorPattern = regexp.MustCompile(`[\s-]+`)

// NormalizeSubjectKey normalizes an event-fingerprint subject key of the form
// "类型:规范名" (person:trump / company:bytedance, add-event-story-heat): trims,
// lowercases, and turns whitespace/hyphens in the name part into underscores,
// preserving the "类型:" prefix. A key without a colon is normalized the same
// way WITHOUT inventing a type prefix — an untyped key is decided (untrusted)
// by the downstream validation layer, not here. Pure function, no entity table
// lookup: alias merging (Trump/川普 → person:trump) is the model's job in the
// extraction call.
func NormalizeSubjectKey(value string) string {
	cleaned := strings.ToLower(strings.TrimSpace(value))
	if cleaned == "" {
		return ""
	}
	if idx := strings.IndexByte(cleaned, ':'); idx >= 0 {
		name := subjectKeySeparatorPattern.ReplaceAllString(strings.TrimSpace(cleaned[idx+1:]), "_")
		return cleaned[:idx+1] + name
	}
	return subjectKeySeparatorPattern.ReplaceAllString(cleaned, "_")
}

// entityCorpSuffixesCJK are trailing corporate markers stripped by
// NormalizeEntityText regardless of the remaining length — CJK names stay
// meaningful however short ("某公司" → "某" is a legitimate trim).
var entityCorpSuffixesCJK = []string{"公司", "集团"}

// entityCorpSuffixesASCII are the ASCII counterparts. Stripping them is
// guarded by minEntityRemainderRunes: without the guard a plain word that
// merely ENDS in a suffix would be gutted ("zinc" → "z" via "inc",
// add-event-story-heat batch 2b fix).
var entityCorpSuffixesASCII = []string{"inc", "ltd", "corp"}

// minEntityRemainderRunes is the minimum remaining length (runes) required
// before an ASCII corporate suffix may be stripped. CJK suffixes are exempt.
const minEntityRemainderRunes = 3

// NormalizeEntityText normalizes a fingerprint subject display name for the
// literal-matching tier of subject normalization (add-event-story-heat design
// §4.1 layer ③): NFKC fold (full-width → ASCII), lowercase, strip ALL
// whitespace, then drop trailing corporate suffixes (公司/集团 always;
// inc/ltd/corp only when ≥ 3 runes remain, repeatedly). It does NOT invent or
// compare subject keys — that is the exact key layer's job.
func NormalizeEntityText(value string) string {
	cleaned := norm.NFKC.String(strings.TrimSpace(value))
	cleaned = strings.ToLower(cleaned)
	cleaned = whitespacePattern.ReplaceAllString(cleaned, "")

	trimmed := true
	for trimmed {
		trimmed = false
		for _, suffix := range entityCorpSuffixesCJK {
			if suffix == "" {
				continue
			}
			if cleaned != suffix && strings.HasSuffix(cleaned, suffix) {
				cleaned = strings.TrimSuffix(cleaned, suffix)
				trimmed = true
				break
			}
		}
		if trimmed {
			continue
		}
		for _, suffix := range entityCorpSuffixesASCII {
			if suffix == "" || !strings.HasSuffix(cleaned, suffix) || cleaned == suffix {
				continue
			}
			remainder := strings.TrimSuffix(cleaned, suffix)
			if utf8.RuneCountInString(remainder) >= minEntityRemainderRunes {
				cleaned = remainder
				trimmed = true
			}
			// Whether or not the guard let this suffix through, no other ASCII
			// suffix can also match the same tail — stop scanning.
			break
		}
	}
	return cleaned
}
