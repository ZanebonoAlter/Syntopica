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
