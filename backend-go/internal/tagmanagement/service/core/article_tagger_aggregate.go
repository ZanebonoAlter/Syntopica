package core

import (
	"context"

	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/logging"
)

// maxAggregateArticleTags caps how many tags one aggregate article may store
// across all its sections.
const maxAggregateArticleTags = 15

// tagAggregateArticle runs the aggregate tagging pipeline: split the article's
// summary into column sections, extract tags per section with the fused prompt,
// dedupe across sections by slug (first section wins) and tier tag scores by
// section position. It returns handled=false when the splitter yields no
// sections so the caller falls back to the mono path. Persistence is left to
// the caller so both paths share one storage chain.
func tagAggregateArticle(ctx context.Context, article *models.Article, input ExtractionInput, extractor *TagExtractor) ([]TopicTag, bool, error) {
	sections := splitSections(article.AIContentSummary)
	if len(sections) == 0 {
		return nil, false, nil
	}

	var tags []TopicTag
	seenSlugs := make(map[string]struct{})
	for i, sec := range sections {
		extracted, err := extractor.extractTagsFromSection(ctx, extractor.router, input, sec.Title, sec.Content, i)
		if err != nil {
			logging.Warnf("aggregate tagging: section %d (%s) failed, skipping: %v", i, sec.Title, err)
			continue
		}
		score := sectionScoreTier(i, len(sections))
		for _, et := range extracted {
			slug := Slugify(et.Label)
			if slug == "" {
				continue
			}
			if _, dup := seenSlugs[slug]; dup {
				continue
			}
			seenSlugs[slug] = struct{}{}
			tags = append(tags, TopicTag{
				Label:           et.Label,
				Slug:            slug,
				Category:        et.Category,
				Aliases:         et.Aliases,
				Description:     et.Description,
				AuxiliaryLabels: et.AuxiliaryLabels,
				Fingerprint:     et.Fingerprint,
				SubjectDesc:     et.SubjectDesc,
				Score:           score,
			})
			if len(tags) >= maxAggregateArticleTags {
				return tags, true, nil
			}
		}
	}
	return tags, true, nil
}

// sectionScoreTier maps a section's position to its score tier: the first body
// column scores 0.9 (headline), the last 0.5 (tail columns like quotes or
// photos) and the middle ones 0.7.
func sectionScoreTier(index, total int) float64 {
	switch index {
	case 0:
		return 0.9
	case total - 1:
		return 0.5
	default:
		return 0.7
	}
}
