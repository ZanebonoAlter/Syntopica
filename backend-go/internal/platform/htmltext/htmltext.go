// Package htmltext is the single implementation of the article summary
// cleaning contract (openspec change fix-reader-lede-blank-space, spec
// article-description-sanitization).
//
// Three projections live here:
//
//   - PlainText / PlainSummary collapse third-party feed HTML into a plain
//     text summary. Script and style content is dropped, hidden subtrees
//     (elements declaring display:none / visibility:hidden / opacity:0 /
//     font-size:0, or carrying the hidden attribute) are removed together
//     with their descendants, tags become separators, HTML entities are
//     resolved and whitespace is folded.
//
//   - PlainTextAll is the variant that keeps hidden subtrees. The summary
//     projection drops them because a lede must not carry what the reader
//     cannot see; the summary-generation fallback needs the opposite, because
//     a source that hides its whole body (智源社区 does) would leave the
//     completion chain with nothing to summarise.
//
//   - SanitizeHTML is the conservative variant that keeps the markup. It
//     removes script/style elements, event handler attributes and elements
//     that are semantically not displayed, and strips the visual hiding
//     declarations from the remaining inline styles.
//
// The two hiding treatments differ on purpose. "Semantically not displayed"
// (display:none, hidden attribute) means the source never shows that content —
// ads, print-only blocks, keyword-stuffing hiding spots — so the element and
// its subtree are dropped. "Visually hidden" (visibility:hidden / opacity:0 /
// font-size:0) is a marker lazy-render scripts put on real article bodies;
// dropping it would empty the article, so the declaration is stripped and the
// content becomes visible again.
//
// PlainText keeps the stricter rule on purpose: a summary must not carry
// content the reader cannot see, so a visually hidden subtree contributes no
// text there. PlainTextAll is the escape hatch for the one consumer that wants
// the content regardless of how the source styles it.
//
// Both are idempotent: cleaning an already cleaned value returns it
// unchanged. The list lede (reader/handler/article_excerpt.go) and the
// stored article summary (reader/service/feed_service.go) share this
// implementation on purpose — two regex pipelines would drift and give the
// same article two different ledes.
package htmltext

import (
	"regexp"
	"strconv"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// ellipsis terminates a truncated summary.
const ellipsis = "…"

var (
	// markupLikeRe detects markup in an already entity-decoded string. A
	// string without any tag-shaped fragment is treated as plain text: the
	// HTML parser would otherwise swallow a stray "<" ("a &lt; c" decodes to
	// "a < c" and "<" followed by a letter starts a tag), and the cleaning
	// would stop being idempotent.
	markupLikeRe = regexp.MustCompile(`<[a-zA-Z][^>]*>`)

	// cssLeadingNumberRe extracts the numeric prefix of a CSS value so that
	// "0", "0.0", "0px" and "0%" are all recognised as zero.
	cssLeadingNumberRe = regexp.MustCompile(`^[+-]?(?:\d+(?:\.\d+)?|\.\d+)`)
)

// PlainText collapses raw feed HTML into plain text: script and style content
// is dropped, hidden subtrees are removed with their descendants, tags become
// separators, HTML entities are resolved and whitespace is folded into single
// spaces.
//
// It is the shared cleaning core behind PlainSummary and behind the list lede
// (reader/handler/article_excerpt.go): both paths must produce the same text
// for the same input, only the length cap differs.
//
// PlainText does not truncate; callers that need a bounded summary use
// PlainSummary.
func PlainText(raw string) string {
	return plainText(raw, true)
}

// PlainTextAll is PlainText without the hidden-subtree removal: script and
// style payloads are still dropped, but every text node contributes, however
// the source hides it.
//
// The summary-generation fallback needs this variant. Some sources ship the
// whole article under an invisible root (智源社区 puts it in an opacity:0
// div), so the visible projection is empty — using it as the fallback source
// would leave those articles without a summary forever. Whether a source
// hides its own body is a styling decision of that site, not a statement
// about the content being real.
func PlainTextAll(raw string) string {
	return plainText(raw, false)
}

// plainText is the shared cleaning core. dropHidden decides whether hidden
// subtrees contribute their text.
func plainText(raw string, dropHidden bool) string {
	if raw == "" {
		return ""
	}

	// Entities are resolved before parsing so the markup check sees the same
	// characters the parser would produce. One pass only, so "&amp;lt;" never
	// collapses into "<".
	decoded := html.UnescapeString(raw)
	if !markupLikeRe.MatchString(decoded) {
		return foldWhitespace(decoded)
	}

	nodes, err := html.ParseFragment(strings.NewReader(decoded), bodyContext())
	if err != nil {
		// A fragment the parser rejects is still better served as text than
		// dropped outright.
		return foldWhitespace(decoded)
	}

	var sb strings.Builder
	for _, node := range nodes {
		collectText(&sb, node, dropHidden)
	}
	return foldWhitespace(sb.String())
}

// HasMarkup reports whether raw carries markup the cleaning acts on.
//
// The check runs on the entity-decoded string, exactly like PlainText does:
// "&lt;b&gt;" decodes to "<b>" and is markup for the reader, while a bare "<"
// inside plain text is not. Callers that must decide "is this value a
// document or a text?" use this instead of re-implementing the tag shape —
// two judgments would drift, and the backfill would rewrite rows the cleaner
// considers plain text.
func HasMarkup(raw string) bool {
	return markupLikeRe.MatchString(html.UnescapeString(raw))
}

// PlainSummary returns the plain-text summary of raw feed HTML, truncated to
// maxRunes runes with a trailing ellipsis when it does not fit.
//
// The cap counts runes, not bytes: a Chinese summary must not be cut in the
// middle of a multi-byte character.
func PlainSummary(raw string, maxRunes int) string {
	text := PlainText(raw)
	if maxRunes <= 0 {
		return ""
	}

	runes := []rune(text)
	if len(runes) <= maxRunes {
		return text
	}
	if maxRunes == 1 {
		return ellipsis
	}
	return string(runes[:maxRunes-1]) + ellipsis
}

// SanitizeHTML conservatively cleans raw feed HTML while keeping its
// structure and layout: script/style elements, event handler attributes and
// semantically hidden elements (display:none, hidden attribute) are removed,
// and the visual hiding declarations (visibility:hidden / opacity:0 /
// font-size:0) are stripped from the remaining inline styles.
//
// The two hiding treatments differ on purpose. display:none (and the hidden
// attribute) means the source never renders that content — ads, print-only
// blocks — so the element is dropped. visibility:hidden / opacity:0 /
// font-size:0 are "invisible but occupying" markers that lazy-render scripts
// use on real article bodies; dropping them would erase the body, so the
// declaration is stripped and the content becomes visible again.
func SanitizeHTML(raw string) string {
	if raw == "" {
		return ""
	}

	nodes, err := html.ParseFragment(strings.NewReader(raw), bodyContext())
	if err != nil {
		return ""
	}
	kept := make([]*html.Node, 0, len(nodes))
	for _, node := range nodes {
		// The top level needs the same treatment as the children: a fragment
		// that STARTS with a script, a style block or a display:none element
		// would keep it, because only the children were checked for removal.
		// The check runs BEFORE the attribute cleaning, otherwise the
		// display:none declaration would be stripped first and the element
		// would survive as visible content.
		if node.Type == html.ElementNode && isDroppedElement(node) {
			continue
		}
		sanitizeNode(node)
		kept = append(kept, node)
	}

	var sb strings.Builder
	for _, node := range kept {
		if err := html.Render(&sb, node); err != nil {
			continue
		}
	}
	return sb.String()
}

// bodyContext is the parse context for feed HTML fragments: article bodies
// are body-level content, not document-level markup.
func bodyContext() *html.Node {
	return &html.Node{
		Type:     html.ElementNode,
		Data:     "body",
		DataAtom: atom.Body,
	}
}

// collectText appends the text of node (and its descendants) to sb. With
// dropHidden set, hidden subtrees contribute no text.
func collectText(sb *strings.Builder, node *html.Node, dropHidden bool) {
	switch node.Type {
	case html.TextNode:
		sb.WriteString(node.Data)
	case html.ElementNode:
		// Every tag becomes a separator so that "<p>a</p><p>b</p>" reads as
		// "a b" instead of "ab".
		sb.WriteString(" ")
		if isScriptOrStyle(node) || (dropHidden && isHiddenElement(node)) {
			return
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			collectText(sb, child, dropHidden)
		}
	}
}

// sanitizeNode cleans node's attributes and recurses into its children.
func sanitizeNode(node *html.Node) {
	if node.Type == html.ElementNode {
		stripEventAttributes(node)
		stripHiddenStyleDeclarations(node)
	}

	for child := node.FirstChild; child != nil; {
		next := child.NextSibling
		if child.Type == html.ElementNode && isDroppedElement(child) {
			node.RemoveChild(child)
			child = next
			continue
		}
		sanitizeNode(child)
		child = next
	}
}

// isDroppedElement reports whether the element must disappear from the
// sanitized markup entirely.
func isDroppedElement(node *html.Node) bool {
	if isScriptOrStyle(node) {
		return true
	}
	for _, attr := range node.Attr {
		if attr.Key == "hidden" {
			return true
		}
		if attr.Key == "style" && declaresDisplayNone(attr.Val) {
			return true
		}
	}
	return false
}

// isScriptOrStyle reports whether the element carries executable or
// presentational payload instead of article content.
func isScriptOrStyle(node *html.Node) bool {
	return node.Data == "script" || node.Data == "style"
}

// isHiddenElement reports whether the element is invisible to the reader. A
// hidden element contributes no text: the lede must not carry content the
// reader cannot see (spec article-description-sanitization).
func isHiddenElement(node *html.Node) bool {
	for _, attr := range node.Attr {
		switch attr.Key {
		case "hidden":
			return true
		case "style":
			if declaresHidden(attr.Val) {
				return true
			}
		}
	}
	return false
}

// stripEventAttributes removes on* event handler attributes so the cleaned
// markup cannot execute anything when injected into the DOM.
func stripEventAttributes(node *html.Node) {
	kept := node.Attr[:0]
	for _, attr := range node.Attr {
		if len(attr.Key) > 2 && strings.HasPrefix(strings.ToLower(attr.Key), "on") {
			continue
		}
		kept = append(kept, attr)
	}
	node.Attr = kept
}

// stripHiddenStyleDeclarations removes the hiding declarations from an
// element's inline style, keeping every other declaration (colour, spacing,
// typography) untouched.
func stripHiddenStyleDeclarations(node *html.Node) {
	for i, attr := range node.Attr {
		if attr.Key != "style" {
			continue
		}
		kept := keepVisibleDeclarations(attr.Val)
		if kept == "" {
			node.Attr = append(node.Attr[:i], node.Attr[i+1:]...)
			return
		}
		node.Attr[i].Val = kept
		return
	}
}

// keepVisibleDeclarations drops the hiding declarations from an inline style
// and returns the remaining declarations.
func keepVisibleDeclarations(style string) string {
	parts := strings.Split(style, ";")
	kept := make([]string, 0, len(parts))
	for _, part := range parts {
		declaration := strings.TrimSpace(part)
		if declaration == "" {
			continue
		}
		property, value, ok := strings.Cut(declaration, ":")
		if !ok {
			kept = append(kept, declaration)
			continue
		}
		switch strings.ToLower(strings.TrimSpace(property)) {
		case "display":
			if isDisplayNone(value) {
				continue
			}
		case "visibility":
			if isVisibilityHidden(value) {
				continue
			}
		case "opacity", "font-size":
			if isZeroLength(value) {
				continue
			}
		}
		kept = append(kept, declaration)
	}
	return strings.Join(kept, "; ")
}

// declaresHidden reports whether an inline style declares any of the hiding
// declarations (display:none / visibility:hidden / opacity:0 / font-size:0).
func declaresHidden(style string) bool {
	for _, part := range strings.Split(style, ";") {
		property, value, ok := strings.Cut(part, ":")
		if !ok {
			continue
		}
		switch strings.ToLower(strings.TrimSpace(property)) {
		case "display":
			if isDisplayNone(value) {
				return true
			}
		case "visibility":
			if isVisibilityHidden(value) {
				return true
			}
		case "opacity", "font-size":
			if isZeroLength(value) {
				return true
			}
		}
	}
	return false
}

// declaresDisplayNone reports whether an inline style hides the element from
// the layout entirely.
func declaresDisplayNone(style string) bool {
	for _, part := range strings.Split(style, ";") {
		property, value, ok := strings.Cut(part, ":")
		if !ok {
			continue
		}
		if strings.ToLower(strings.TrimSpace(property)) == "display" && isDisplayNone(value) {
			return true
		}
	}
	return false
}

func isDisplayNone(value string) bool {
	return strings.EqualFold(strings.TrimSpace(value), "none")
}

func isVisibilityHidden(value string) bool {
	trimmed := strings.TrimSpace(value)
	return strings.EqualFold(trimmed, "hidden") || strings.EqualFold(trimmed, "collapse")
}

// isZeroLength reports whether a CSS value is zero (0, 0.0, 0px, 0%, …),
// which makes the element invisible or unclickable.
func isZeroLength(value string) bool {
	trimmed := strings.TrimSpace(value)
	if idx := strings.Index(trimmed, "!"); idx >= 0 {
		trimmed = strings.TrimSpace(trimmed[:idx])
	}
	number := cssLeadingNumberRe.FindString(trimmed)
	if number == "" {
		return false
	}
	parsed, err := strconv.ParseFloat(number, 64)
	return err == nil && parsed == 0
}

// foldWhitespace collapses every run of Unicode whitespace into a single
// space and trims the ends.
func foldWhitespace(s string) string {
	return strings.Join(strings.Fields(s), " ")
}
