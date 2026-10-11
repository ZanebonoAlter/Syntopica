package core

import (
	"fmt"
	"regexp"
	"strconv"
	"time"

	"syntopica-backend/internal/models"
	"syntopica-backend/internal/tagmanagement/repository"
)

// anchorYearPattern matches standalone half-width 4-digit years 2000–2099.
// Word boundaries keep version strings ("v2024") and longer digit runs out;
// full-width digits (２０２４) are intentionally not covered — news-tag
// descriptions overwhelmingly use half-width digits (known boundary, see
// test-cases 白盒附加).
var anchorYearPattern = regexp.MustCompile(`\b20\d{2}\b`)

// ExtractEarliestYear returns the earliest year mentioned in text as a time
// anchor. ok=false when no year is present.
func ExtractEarliestYear(text string) (int, bool) {
	earliest := 0
	for _, m := range anchorYearPattern.FindAllString(text, -1) {
		y, err := strconv.Atoi(m)
		if err != nil {
			continue
		}
		if earliest == 0 || y < earliest {
			earliest = y
		}
	}
	if earliest == 0 {
		return 0, false
	}
	return earliest, true
}

// EventAnchorScanRow is one event-tag row loaded for the anchor scan.
type EventAnchorScanRow struct {
	ID          uint
	Label       string
	Description string
	CreatedAt   time.Time
}

// EventAnchorCandidate is an event tag whose description time anchor is at
// least one year older than the tag creation year — the mechanical filter for
// "possibly stale" descriptions. Whether such a tag is actually pathological
// (event-subject anchor) or a legal historical reference is decided by human
// review of the candidate list, never automatically.
type EventAnchorCandidate struct {
	EventAnchorScanRow
	EarliestYear       int
	DescriptionPreview string
}

// FilterEventAnchorCandidates keeps rows whose description carries an
// earliest-year anchor at least one year before the tag creation year.
// No year → not a candidate; anchor in the creation year or later → not a
// candidate (gap < 1).
func FilterEventAnchorCandidates(rows []EventAnchorScanRow) []EventAnchorCandidate {
	out := make([]EventAnchorCandidate, 0, len(rows))
	for _, row := range rows {
		year, ok := ExtractEarliestYear(row.Description)
		if !ok {
			continue
		}
		if row.CreatedAt.Year()-year < 1 {
			continue
		}
		out = append(out, EventAnchorCandidate{
			EventAnchorScanRow: row,
			EarliestYear:       year,
			DescriptionPreview: previewRunes(row.Description, 80),
		})
	}
	return out
}

// previewRunes truncates s to at most n runes, appending an ellipsis when cut.
func previewRunes(s string, n int) string {
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[:n]) + "…"
}

// ScanEventTagAnchorCandidates scans active event tags with a non-empty
// description and returns those whose earliest description year precedes the
// creation year by ≥1.
func ScanEventTagAnchorCandidates() ([]EventAnchorCandidate, error) {
	var rows []EventAnchorScanRow
	if err := repository.Repo.DB().Model(&models.TopicTag{}).
		Select("id", "label", "description", "created_at").
		Where("category = ? AND status = ?", models.TagCategoryEvent, "active").
		Where("description IS NOT NULL AND TRIM(description) <> ''").
		Order("id").
		Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("load event tags: %w", err)
	}
	return FilterEventAnchorCandidates(rows), nil
}

// SelectEventAnchorCandidatesByIDs intersects explicit IDs with the scan
// candidates, collapsing duplicate IDs. Any requested ID absent from the
// candidate list is reported in the returned error so the caller refuses to
// touch healthy tags instead of silently skipping them.
func SelectEventAnchorCandidatesByIDs(candidates []EventAnchorCandidate, ids []uint) ([]EventAnchorCandidate, error) {
	byID := make(map[uint]EventAnchorCandidate, len(candidates))
	for _, c := range candidates {
		byID[c.ID] = c
	}

	selected := make([]EventAnchorCandidate, 0, len(ids))
	unknown := make([]uint, 0)
	seen := make(map[uint]struct{}, len(ids))
	for _, id := range ids {
		if _, dup := seen[id]; dup {
			continue
		}
		seen[id] = struct{}{}
		if c, ok := byID[id]; ok {
			selected = append(selected, c)
		} else {
			unknown = append(unknown, id)
		}
	}
	if len(unknown) > 0 {
		return nil, fmt.Errorf("IDs %v are not anchor-scan candidates (already fixed, healthy, or non-existent); nothing was modified", unknown)
	}
	return selected, nil
}

// EventAnchorFixResult reports the outcome of one --apply regeneration.
type EventAnchorFixResult struct {
	TagID          uint
	Label          string
	OldDescription string
	NewDescription string
	Err            error
}

// ApplyEventTagAnchorFix regenerates descriptions for the given candidate
// tags, anchored to each tag's first-mounted article context. The candidate
// whitelist is enforced again inside (defence in depth for the CLI). A failed
// tag is recorded in its result and does NOT abort the batch — remaining tags
// are still processed. Repeated execution is idempotent: the description
// update overwrites the same row and, once anchored, the tag no longer shows
// up in scans.
func ApplyEventTagAnchorFix(candidates []EventAnchorCandidate, ids []uint) ([]EventAnchorFixResult, error) {
	selected, err := SelectEventAnchorCandidatesByIDs(candidates, ids)
	if err != nil {
		return nil, err
	}

	results := make([]EventAnchorFixResult, 0, len(selected))
	for _, c := range selected {
		res := EventAnchorFixResult{TagID: c.ID, Label: c.Label, OldDescription: c.Description}
		newDesc, err := RegenerateEventTagDescription(c.ID)
		if err != nil {
			res.Err = err
		} else {
			res.NewDescription = newDesc
		}
		results = append(results, res)
	}
	return results, nil
}

// RegenerateEventTagDescription regenerates an event tag description via the
// same prompt / LLM / persistence path as new-tag description generation,
// anchored to the tag's first-mounted article (earliest article_topic_tags
// row): "[日期: 发布日期] 标题. 摘要≤800字".
func RegenerateEventTagDescription(tagID uint) (string, error) {
	var tag models.TopicTag
	if err := repository.Repo.DB().Select("id", "label", "category").
		Where("id = ?", tagID).First(&tag).Error; err != nil {
		return "", fmt.Errorf("load tag %d: %w", tagID, err)
	}
	if tag.Category != models.TagCategoryEvent {
		return "", fmt.Errorf("tag %d has category %q, want event", tagID, tag.Category)
	}

	articleContext, err := firstMountArticleContext(tagID)
	if err != nil {
		return "", fmt.Errorf("first-mount context for tag %d: %w", tagID, err)
	}

	desc, keywords, ok := requestTagDescription(true, tagID, tag.Label, tag.Category, articleContext)
	if !ok {
		return "", fmt.Errorf("description LLM call failed for tag %d after 3 attempts", tagID)
	}
	if err := saveTagDescription(true, tagID, desc, keywords); err != nil {
		return "", fmt.Errorf("persist regenerated description for tag %d: %w", tagID, err)
	}
	return desc, nil
}

// firstMountArticleContext loads the earliest-mounted article of the tag and
// builds its articleContext with the persistArticleTags format.
func firstMountArticleContext(tagID uint) (string, error) {
	var link models.ArticleTopicTag
	if err := repository.Repo.DB().Where("topic_tag_id = ?", tagID).
		Order("created_at ASC, id ASC").First(&link).Error; err != nil {
		return "", fmt.Errorf("no mounted article: %w", err)
	}
	var article models.Article
	if err := repository.Repo.DB().Where("id = ?", link.ArticleID).First(&article).Error; err != nil {
		return "", fmt.Errorf("load article %d: %w", link.ArticleID, err)
	}
	return buildArticleContext(&article), nil
}
