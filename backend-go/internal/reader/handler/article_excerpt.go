package handler

import (
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"gorm.io/gorm"
)

// excerptMaxRunes bounds the list-row lede length (spec: ≤200 字符).
const excerptMaxRunes = 200

// excerptSourceLimit bounds how much raw HTML the DB hands to the app per
// article. substr(x, 1, 2000) truncates by character on both sqlite and
// postgres, so 2000 runes is always enough to fill excerptMaxRunes even when
// the head of the document is mostly markup.
const excerptSourceLimit = 2000

var (
	excerptScriptStyleRe = regexp.MustCompile(`(?is)<(?:script|style)\b[^>]*>.*?</(?:script|style)\s*>`)
	excerptTagRe         = regexp.MustCompile(`<[^>]*>`)
	excerptEntityRe      = regexp.MustCompile(`(?i)&(?:nbsp|amp|lt|gt|quot|apos|#\d+|#x[0-9a-f]+);`)
)

// buildExcerpt derives the list lede from an article's HTML sources: it tries
// description first and falls back to content. Returns "" when neither source
// yields substantive text, or when the lede repeats the article body (see
// excerptRepeatsBody). noFirecrawlBody reports whether the article has no
// Firecrawl body, i.e. the reading page will display `content` as the body.
func buildExcerpt(description, content string, noFirecrawlBody bool) string {
	lede := cleanExcerptSource(description)
	if lede == "" {
		lede = cleanExcerptSource(content)
	}
	if lede == "" || excerptRepeatsBody(description, lede, content, noFirecrawlBody) {
		return ""
	}
	return lede
}

// excerptRepeatsBody reports whether the candidate lede repeats the article
// body and would therefore be hidden by the reading page's dedupe guard
// (front/app/utils/articleContentGuards.ts, shouldShowArticleDescription).
//
// The criteria are a deliberate SUBSET of that guard: an excerpt is blanked
// only when the guard would also hide the description, and MUST NOT suppress a
// lede the guard would render. Rationale: 92% of non-archived articles carry
// description == content, so shipping the lede would flash it for one frame
// before the detail response hides it and the body jumps up.
func excerptRepeatsBody(description, lede, content string, noFirecrawlBody bool) bool {
	if content == "" {
		return false // no body to repeat
	}

	contentNorm := normalizeForLedeCompare(content)
	if contentNorm == "" {
		return false
	}

	// Guard rule 1: description and content are identical after normalization
	// (no length floor; covers same-source short text).
	// Guard rule 2: a ≥40-rune description contained by the body.
	// Guard rule 3 (body contained by description) is skipped: 0 observed
	// cases, and under-suppressing never causes the flash this function exists
	// to prevent.
	if descriptionNorm := normalizeForLedeCompare(description); descriptionNorm != "" {
		if descriptionNorm == contentNorm {
			return true
		}
		return len([]rune(descriptionNorm)) >= 40 && strings.Contains(contentNorm, descriptionNorm)
	}

	// The description yielded no lede, so the lede came from content itself.
	// No Firecrawl body ⇒ the reading page displays `content` as the body
	// (front/app/utils/articleContentSource.ts +
	// useArticleContentView.displayContent), and the guard's equality rule
	// (empty description ⇒ ledeText falls back to the excerpt) has NO length
	// floor — so the lede is hidden even for short V2EX-style posts. Suppress
	// unconditionally here (the previous ≥40-rune floor let those flash).
	if noFirecrawlBody {
		return true
	}

	// With a Firecrawl body the displayed text is firecrawl_content, not
	// `content`, so the guard may well render the lede; only suppress the long
	// content-derived lede the old containment rule already covered.
	ledeNorm := normalizeForLedeCompare(lede)
	return len([]rune(ledeNorm)) >= 40 && strings.Contains(contentNorm, ledeNorm)
}

// normalizeForLedeCompare reduces a raw HTML source to the form used for
// lede/body dedupe: the same cleaning as cleanExcerptSource (script/style
// removal, tag stripping, entity decoding, whitespace folding) plus case
// folding. Unlike cleanExcerptSource it keeps the full text (no 200-rune
// truncation, no substantive-text floor) and MUST NOT be used for display.
func normalizeForLedeCompare(raw string) string {
	return strings.ToLower(stripExcerptMarkup(raw))
}

// stripExcerptMarkup removes script/style blocks and tags, decodes common
// entities and folds Unicode whitespace into single spaces.
func stripExcerptMarkup(raw string) string {
	s := excerptScriptStyleRe.ReplaceAllString(raw, " ")
	s = excerptTagRe.ReplaceAllString(s, " ")
	s = decodeHTMLEntities(s)
	return strings.Join(strings.Fields(s), " ")
}

// cleanExcerptSource strips script/style blocks and HTML tags, decodes common
// entities, collapses Unicode whitespace, drops text without any letter or
// number, then truncates to excerptMaxRunes runes (no ellipsis).
func cleanExcerptSource(raw string) string {
	if raw == "" {
		return ""
	}

	s := stripExcerptMarkup(raw)

	if !hasSubstantiveText(s) {
		return ""
	}

	if runes := []rune(s); len(runes) > excerptMaxRunes {
		s = string(runes[:excerptMaxRunes])
	}
	return s
}

func hasSubstantiveText(s string) bool {
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsNumber(r) {
			return true
		}
	}
	return false
}

// decodeHTMLEntities resolves the entity set relevant to excerpt text in a
// single pass (so "&amp;lt;" never double-decodes into "<").
func decodeHTMLEntities(s string) string {
	return excerptEntityRe.ReplaceAllStringFunc(s, func(entity string) string {
		lower := strings.ToLower(entity)
		switch lower {
		case "&nbsp;":
			return " "
		case "&amp;":
			return "&"
		case "&lt;":
			return "<"
		case "&gt;":
			return ">"
		case "&quot;":
			return `"`
		case "&apos;", "&#39;":
			return "'"
		}

		var (
			code int64
			err  error
		)
		if strings.HasPrefix(lower, "&#x") {
			code, err = strconv.ParseInt(lower[3:len(lower)-1], 16, 32)
		} else {
			code, err = strconv.ParseInt(lower[2:len(lower)-1], 10, 32)
		}
		if err != nil || code <= 0 || code > unicode.MaxRune {
			return entity
		}
		return string(rune(code))
	})
}

type excerptSourceRow struct {
	ID          uint
	Description string
	Content     string
	// NoFirecrawlBody is 1 when firecrawl_content is empty, so buildExcerpt
	// can apply the fallback-lede rule without hauling the Firecrawl body out
	// of the DB. The CASE expression is portable across postgres and sqlite.
	NoFirecrawlBody int
}

// excerptsByArticleID fetches excerpt sources for the given article IDs in a
// single query, truncating each source in the DB (substr) so only a few KB
// travel to the app. Returns an empty map for an empty id list.
func excerptsByArticleID(db *gorm.DB, ids []uint) (map[uint]string, error) {
	if len(ids) == 0 {
		return map[uint]string{}, nil
	}

	var rows []excerptSourceRow
	if err := db.Table("articles").
		Select(
			"id, substr(description, 1, ?) AS description, substr(content, 1, ?) AS content, CASE WHEN firecrawl_content = '' THEN 1 ELSE 0 END AS no_firecrawl_body",
			excerptSourceLimit, excerptSourceLimit,
		).
		Where("id IN ?", ids).
		Scan(&rows).Error; err != nil {
		return nil, err
	}

	excerpts := make(map[uint]string, len(rows))
	for _, row := range rows {
		excerpts[row.ID] = buildExcerpt(row.Description, row.Content, row.NoFirecrawlBody == 1)
	}
	return excerpts, nil
}
