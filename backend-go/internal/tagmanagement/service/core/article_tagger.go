package core

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"go.opentelemetry.io/otel"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/logging"
	"syntopica-backend/internal/platform/tracing"
	"syntopica-backend/internal/tagmanagement/repository"
)

const maxArticleTags = 6

// tagSourceReuse marks article_topic_tags rows copied from a sibling copy of
// the same article (same link, another feed) instead of extracted by the AI
// (dedupe-rss-articles D3).
const tagSourceReuse = "reuse"

// reuseTagsFromSiblingArticle copies article_topic_tags rows from a sibling
// copy of the article (same link under another feed) onto this article with
// source="reuse". Returns true when at least one tag link was copied, meaning
// the AI extraction can be skipped entirely. Articles without a link never
// participate — an empty link would match every other empty-link row.

// tagExtractorFactory builds the extractor used by tagArticle. Overridden in
// tests to inject a fake router.
var tagExtractorFactory = NewTagExtractor

func FeedCategoryName(feed models.Feed) string {
	if feed.Category != nil && strings.TrimSpace(feed.Category.Name) != "" {
		return feed.Category.Name
	}
	if feed.CategoryID != nil {
		var cat models.Category
		if err := repository.Repo.DB().First(&cat, *feed.CategoryID).Error; err == nil && cat.Name != "" {
			return cat.Name
		}
	}
	return ""
}

type tagArticleOptions struct {
	Force bool
}

// TagArticle extracts and stores tags for a single article.
func TagArticle(ctx context.Context, article *models.Article, feedName, categoryName string) error {
	ctx, span := otel.Tracer(tracing.ServiceName).Start(ctx, "core.TagArticle")
	defer span.End()
	return tagArticle(ctx, article, feedName, categoryName, tagArticleOptions{})
}

func RetagArticle(ctx context.Context, article *models.Article, feedName, categoryName string) error {
	ctx, span := otel.Tracer(tracing.ServiceName).Start(ctx, "core.RetagArticle")
	defer span.End()
	return tagArticle(ctx, article, feedName, categoryName, tagArticleOptions{Force: true})
}

func tagArticle(ctx context.Context, article *models.Article, feedName, categoryName string, options tagArticleOptions) error {
	if article == nil || article.ID == 0 {
		return nil
	}

	if options.Force {
		var oldTagIDs []uint
		if err := repository.Repo.DB().Model(&models.ArticleTopicTag{}).
			Where("article_id = ?", article.ID).
			Pluck("topic_tag_id", &oldTagIDs).Error; err != nil {
			return err
		}

		if err := repository.Repo.DB().Where("article_id = ?", article.ID).Delete(&models.ArticleTopicTag{}).Error; err != nil {
			return err
		}

		CleanupOrphanedTags(oldTagIDs)
	}

	// Cross-feed reuse (dedupe-rss-articles D3): a sibling copy of this
	// article (same link under another feed) already carries the tagging
	// result — copy the tag links instead of calling the AI again. Runs
	// before the already-tagged skip so a partially-tagged copy also gets
	// topped up from its sibling (existing links are never duplicated, and
	// the top-up stops at maxArticleTags).
	// Force retag never reuses (an explicit re-extraction was requested).
	if !options.Force {
		reused, err := reuseTagsFromSiblingArticle(article)
		if err != nil {
			logging.Warnf("cross-feed tag reuse failed for article %d, falling back to AI extraction: %v", article.ID, err)
		} else if reused {
			return nil
		}
	}

	// Skip if already tagged
	var existingCount int64
	repository.Repo.DB().Model(&models.ArticleTopicTag{}).Where("article_id = ?", article.ID).Count(&existingCount)
	if existingCount > 0 {
		return nil
	}

	// Build input for extraction
	input := ExtractionInput{
		Title:        article.Title,
		Summary:      buildArticleSummary(*article),
		FeedName:     feedName,
		CategoryName: categoryName,
		ArticleID:    &article.ID,
		PubDate:      formatPubDate(article.PubDate),
	}

	// Use the extraction system
	extractor := tagExtractorFactory()

	var tags []TopicTag
	var source string
	handled := false
	if article.ContentForm == "aggregate" {
		aggTags, aggHandled, aggErr := tagAggregateArticle(ctx, article, input, extractor)
		if aggErr != nil {
			logging.Warnf("aggregate tagging failed for article %d, falling back to mono path: %v", article.ID, aggErr)
		} else if aggHandled {
			if len(aggTags) > 0 {
				tags, source, handled = aggTags, "llm", true
			} else {
				// All sections failed or returned empty candidates: fall back to
				// the mono path (dual-branch extraction with heuristic fallback)
				// so aggregate articles never end up with zero tags.
				logging.Infof("aggregate tagging yielded no tags for article %d, falling back to mono path", article.ID)
			}
		}
		// handled=false (splitter yielded no sections) falls through to the mono path
	}
	if !handled {
		result, err := extractor.ExtractTags(context.Background(), input)
		if err != nil || len(result.Tags) == 0 {
			// Fall back to legacy heuristic extraction
			tags = legacyExtractTopics(input)
			source = "heuristic"
		} else {
			tags = result.Tags
			source = result.Source
		}
		tags = limitArticleTags(tags)
	}

	if len(tags) == 0 {
		return nil
	}

	return persistArticleTags(ctx, article, tags, source)
}

// persistArticleTags stores the extracted tags for an article: dedupe, then for
// each tag find-or-create the topic tag, attach auxiliary labels and link it to
// the article. Shared by the mono and aggregate extraction paths.
func persistArticleTags(ctx context.Context, article *models.Article, tags []TopicTag, source string) error {
	// Build article context for description generation
	articleContext := ""
	pubDateStr := formatPubDate(article.PubDate)
	if pubDateStr != "" {
		articleContext = "[日期: " + pubDateStr + "] "
	}
	if article.Title != "" {
		articleContext += article.Title
	}
	articleSummary := buildArticleSummary(*article)
	if articleSummary != "" {
		if articleContext != "" {
			articleContext += ". "
		}
		runes := []rune(articleSummary)
		if len(runes) > 800 {
			articleSummary = string(runes[:800])
		}
		articleContext += articleSummary
	}

	dedupedTags := dedupeTagsWithCategory(tags)

	seenTagIDs := make(map[uint]struct{})
	for _, tag := range dedupedTags {
		dbTag, err := findOrCreateTag(ctx, tag, source, articleContext, article.ID)
		if err != nil {
			logging.Warnf("findOrCreateTag failed for tag %q (category=%s, slug=%s, source=%s, article=%d): %v", tag.Label, tag.Category, Slugify(tag.Label), source, article.ID, err)
			continue
		}

		if _, alreadyAdded := seenTagIDs[dbTag.ID]; alreadyAdded {
			continue
		}
		seenTagIDs[dbTag.ID] = struct{}{}
		if len(tag.AuxiliaryLabels) > 0 {
			if err := AuxServiceFactory(repository.Repo.DB(), nil).AttachAuxiliaryLabels(ctx, dbTag.ID, tag.AuxiliaryLabels); err != nil {
				logging.Warnf("attach auxiliary labels failed for tag %d: %v", dbTag.ID, err)
			}
		}

		if dbTag.Category == "keyword" && strings.TrimSpace(tag.Description) != "" {
			keywordLabel := []AuxiliaryLabel{{Label: tag.Label, Description: tag.Description}}
			if err := AuxServiceFactory(repository.Repo.DB(), nil).AttachAuxiliaryLabels(ctx, dbTag.ID, keywordLabel); err != nil {
				logging.Warnf("keyword direct-to-pool failed for tag %d: %v", dbTag.ID, err)
			}
		}

		link := models.ArticleTopicTag{
			ArticleID:  article.ID,
			TopicTagID: dbTag.ID,
			Score:      tag.Score,
			Source:     source,
		}
		articleExists, err := createArticleTopicTagLink(&link)
		if err != nil {
			return err
		}
		if !articleExists {
			logging.Infof("Article %d was deleted during tagging, skipping remaining tags", article.ID)
			return nil
		}

		if dbTag.Category == "event" {
			qs := getEmbeddingQueueService()
			if qs != nil {
				if err := qs.Enqueue(dbTag.ID); err != nil {
					logging.Warnf("Failed to enqueue re-embedding for event tag %d: %v", dbTag.ID, err)
				}
			}
		}
	}

	return nil
}

func createArticleTopicTagLink(link *models.ArticleTopicTag) (bool, error) {
	articleExists := false
	err := repository.Repo.DB().Transaction(func(tx *gorm.DB) error {
		var article models.Article
		result := tx.Clauses(clause.Locking{Strength: "KEY SHARE"}).
			Select("id").
			Where("id = ?", link.ArticleID).
			Limit(1).
			Find(&article)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return nil
		}

		articleExists = true
		return tx.Create(link).Error
	})
	return articleExists, err
}

func reuseTagsFromSiblingArticle(article *models.Article) (bool, error) {
	if strings.TrimSpace(article.Link) == "" {
		return false, nil
	}

	// Highest-scored sibling tags first: a partially tagged copy is topped up
	// only to the per-article cap (same maxArticleTags the AI path applies in
	// limitArticleTags), so the cap is spent on the best-scored tags.
	var siblingLinks []models.ArticleTopicTag
	if err := repository.Repo.DB().
		Where("article_id != ? AND article_id IN (SELECT id FROM articles WHERE link = ?)", article.ID, article.Link).
		Order("score DESC, topic_tag_id ASC").
		Find(&siblingLinks).Error; err != nil {
		return false, err
	}
	if len(siblingLinks) == 0 {
		return false, nil
	}

	ownTagIDs := make(map[uint]struct{})
	var ownIDs []uint
	if err := repository.Repo.DB().Model(&models.ArticleTopicTag{}).
		Where("article_id = ?", article.ID).
		Pluck("topic_tag_id", &ownIDs).Error; err != nil {
		return false, err
	}
	for _, id := range ownIDs {
		ownTagIDs[id] = struct{}{}
	}

	// Already at the cap: nothing to top up. Returning false lets the caller's
	// already-tagged guard keep the AI path away as well.
	if len(ownTagIDs) >= maxArticleTags {
		return false, nil
	}

	copied := false
	for _, link := range siblingLinks {
		if _, dup := ownTagIDs[link.TopicTagID]; dup {
			// Already present on this article: skip (unique index
			// idx_article_topic_tags_link would reject a duplicate anyway).
			continue
		}
		if len(ownTagIDs) >= maxArticleTags {
			break
		}
		ownTagIDs[link.TopicTagID] = struct{}{}
		newLink := models.ArticleTopicTag{
			ArticleID:  article.ID,
			TopicTagID: link.TopicTagID,
			Score:      link.Score,
			Source:     tagSourceReuse,
		}
		articleExists, err := createArticleTopicTagLink(&newLink)
		if err != nil {
			return copied, err
		}
		if !articleExists {
			return copied, nil
		}
		copied = true
	}

	if copied {
		// Keep the counter in sync with the edges just written (the read path
		// recomputes tag_count via subquery, but the column is still part of the
		// row contract). Raw SQL on purpose: the model marks tag_count as
		// read-only (`gorm:"->"`), so GORM's Update() would silently skip it.
		repository.Repo.DB().Exec(
			"UPDATE articles SET tag_count = (SELECT COUNT(*) FROM article_topic_tags WHERE article_id = ?) WHERE id = ?",
			article.ID, article.ID)
	}
	return copied, nil
}

func limitArticleTags(tags []TopicTag) []TopicTag {
	if len(tags) <= maxArticleTags {
		return tags
	}
	return tags[:maxArticleTags]
}

const maxSummaryRunesForTagging = 4000

func buildArticleSummary(article models.Article) string {
	var body string
	if s := strings.TrimSpace(article.AIContentSummary); s != "" {
		body = s
	} else if s := strings.TrimSpace(article.FirecrawlContent); s != "" {
		body = s
	} else if s := strings.TrimSpace(article.Content); s != "" {
		body = s
	} else if s := strings.TrimSpace(article.Description); s != "" {
		body = s
	}
	if body == "" {
		return ""
	}
	runes := []rune(body)
	if len(runes) > maxSummaryRunesForTagging {
		body = string(runes[:maxSummaryRunesForTagging])
	}
	return body
}

// TagArticles batch tags multiple articles for a feed
// This is called from auto_summary when processing a feed's articles
func TagArticles(ctx context.Context, articles []models.Article, feedName, categoryName string) error {
	ctx, span := otel.Tracer(tracing.ServiceName).Start(ctx, "core.TagArticles")
	defer span.End()
	if len(articles) == 0 {
		return nil
	}

	for i := range articles {
		if err := TagArticle(ctx, &articles[i], feedName, categoryName); err != nil {
			logging.Warnf("Failed to tag article %d: %v", articles[i].ID, err)
		}
	}

	return nil
}

// GetArticleTags retrieves all tags for a specific article
func GetArticleTags(articleID uint) ([]TopicTag, error) {
	var links []models.ArticleTopicTag
	err := repository.Repo.DB().Where("article_id = ?", articleID).
		Preload("TopicTag").
		Find(&links).Error
	if err != nil {
		return nil, err
	}

	tagIDs := make([]uint, 0, len(links))
	for _, link := range links {
		if link.TopicTag != nil {
			tagIDs = append(tagIDs, link.TopicTagID)
		}
	}

	articleCounts := make(map[uint]int)
	if len(tagIDs) > 0 {
		type countRow struct {
			TopicTagID uint
			Count      int
		}
		var rows []countRow
		if err := repository.Repo.DB().Model(&models.ArticleTopicTag{}).
			Select("topic_tag_id, COUNT(*) as count").
			Where("topic_tag_id IN ?", tagIDs).
			Group("topic_tag_id").
			Scan(&rows).Error; err != nil {
			logging.Warnf("GetArticleTags: failed to batch-fetch article counts: %v", err)
		}
		for _, row := range rows {
			articleCounts[row.TopicTagID] = row.Count
		}
	}

	result := make([]TopicTag, 0, len(links))
	for _, link := range links {
		if link.TopicTag == nil {
			continue
		}
		result = append(result, TopicTag{
			ID:           link.TopicTag.ID,
			Label:        link.TopicTag.Label,
			Slug:         link.TopicTag.Slug,
			Category:     link.TopicTag.Category,
			Icon:         link.TopicTag.Icon,
			Aliases:      parseAliasesFromJSON(link.TopicTag.Aliases),
			Score:        link.Score,
			Description:  link.TopicTag.Description,
			IsWatched:    link.TopicTag.IsWatched,
			ArticleCount: articleCounts[link.TopicTagID],
		})
	}

	return result, nil
}

// GetArticlesByTag retrieves articles tagged with a specific tag
func GetArticlesByTag(slug, category string, limit int) ([]models.Article, error) {
	var articles []models.Article

	query := repository.Repo.DB().
		Joins("JOIN article_topic_tags ON article_topic_tags.article_id = articles.id").
		Joins("JOIN topic_tags ON topic_tags.id = article_topic_tags.topic_tag_id").
		Where("topic_tags.slug = ?", slug)

	if category != "" {
		query = query.Where("topic_tags.category = ?", category)
	}

	err := query.
		Omit("tag_count", "relevance_score").
		Order("articles.pub_date DESC").
		Limit(limit).
		Find(&articles).Error

	return articles, err
}

func CleanupOrphanedTags(tagIDs []uint) {
	if len(tagIDs) == 0 {
		return
	}

	var orphanIDs []uint
	repository.Repo.DB().Model(&models.TopicTag{}).
		Where("id IN ?", tagIDs).
		Where("id NOT IN (SELECT topic_tag_id FROM article_topic_tags)").
		Pluck("id", &orphanIDs)

	if len(orphanIDs) == 0 {
		return
	}

	// Collect affected aux label IDs before CASCADE deletes them
	var affectedAuxLabelIDs []uint
	repository.Repo.DB().Model(&models.TopicTagSemanticLabel{}).
		Where("topic_tag_id IN ?", orphanIDs).
		Distinct("semantic_label_id").
		Pluck("semantic_label_id", &affectedAuxLabelIDs)

	// All child tables have ON DELETE CASCADE, so deleting topic_tags
	// automatically cleans up embeddings, queues, relations, labels, etc.
	if err := repository.Repo.DB().Where("id IN ?", orphanIDs).Delete(&models.TopicTag{}).Error; err != nil {
		logging.Warnf("Failed to delete %d orphaned topic tags: %v", len(orphanIDs), err)
	} else {
		logging.Infof("Cleaned up %d orphaned topic tags", len(orphanIDs))
	}

	// Recount refs for affected auxiliary labels after CASCADE
	if len(affectedAuxLabelIDs) > 0 {
		auxService := AuxServiceFactory(repository.Repo.DB(), nil)
		if err := auxService.RecountRefs(context.Background(), affectedAuxLabelIDs); err != nil {
			logging.Warnf("Failed to recount refs after orphan cleanup: %v", err)
		}
	}
}

func parseAliasesFromJSON(aliases string) []string {
	if strings.TrimSpace(aliases) == "" {
		return nil
	}
	var result []string
	if err := json.Unmarshal([]byte(aliases), &result); err != nil {
		return nil
	}
	return result
}

func formatPubDate(pubDate *time.Time) string {
	if pubDate == nil {
		return ""
	}
	return pubDate.Format("2006-01-02")
}
