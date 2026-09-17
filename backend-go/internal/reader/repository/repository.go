package repository

import (
	"context"

	"gorm.io/gorm"
	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/articlerefs"
)

// ============================================================================
// ReaderRepository — centralized data access for the reader feature module
// ============================================================================

// Repo is the package-level repository singleton, initialized by InitRepository.
var Repo *ReaderRepository

type ReaderRepository struct {
	db *gorm.DB
}

func NewReaderRepository(db *gorm.DB) *ReaderRepository {
	return &ReaderRepository{db: db}
}

// InitRepository initializes the package-level repository singleton.
func InitRepository(db *gorm.DB) {
	Repo = NewReaderRepository(db)
}

// DB returns the underlying gorm.DB for complex ad-hoc queries.
func (r *ReaderRepository) DB() *gorm.DB {
	return r.db
}

// ============================================================================
// Category operations
// ============================================================================

func (r *ReaderRepository) ListCategories() ([]models.Category, error) {
	var categories []models.Category
	err := r.db.Order("name ASC").Find(&categories).Error
	return categories, err
}

func (r *ReaderRepository) GetCategory(id uint) (*models.Category, error) {
	var cat models.Category
	err := r.db.First(&cat, id).Error
	if err != nil {
		return nil, err
	}
	return &cat, nil
}

func (r *ReaderRepository) GetCategoryByName(name string) (*models.Category, error) {
	var cat models.Category
	err := r.db.Where("name = ?", name).First(&cat).Error
	if err != nil {
		return nil, err
	}
	return &cat, nil
}

func (r *ReaderRepository) CreateCategory(cat *models.Category) error {
	return r.db.Create(cat).Error
}

func (r *ReaderRepository) UpdateCategory(cat *models.Category, updates map[string]interface{}) error {
	return r.db.Model(cat).Updates(updates).Error
}

// DeleteCategory removes the category row only. It does not remove the feeds
// (and articles) filed under it and therefore maintains no article references:
// deleting a category is DeleteCategoryCascade's job.
func (r *ReaderRepository) DeleteCategory(cat *models.Category) error {
	return r.db.Delete(cat).Error
}

func (r *ReaderRepository) GetFeedCountGroupedByCategory() (map[uint]int, error) {
	type FeedCount struct {
		CategoryID uint
		Count      int
	}
	var rows []FeedCount
	if err := r.db.Model(&models.Feed{}).
		Select("category_id, count(*) as count").
		Where("category_id IS NOT NULL").
		Group("category_id").
		Scan(&rows).Error; err != nil {
		return nil, err
	}
	m := make(map[uint]int, len(rows))
	for _, r := range rows {
		m[r.CategoryID] = r.Count
	}
	return m, nil
}

// ============================================================================
// Feed operations
// ============================================================================

func (r *ReaderRepository) GetFeed(id uint) (*models.Feed, error) {
	var feed models.Feed
	err := r.db.First(&feed, id).Error
	if err != nil {
		return nil, err
	}
	return &feed, nil
}

func (r *ReaderRepository) GetFeedByURL(url string) (*models.Feed, error) {
	var feed models.Feed
	err := r.db.Where("url = ?", url).First(&feed).Error
	if err != nil {
		return nil, err
	}
	return &feed, nil
}

func (r *ReaderRepository) CreateFeed(feed *models.Feed) error {
	return r.db.Create(feed).Error
}

func (r *ReaderRepository) SaveFeed(feed *models.Feed) error {
	return r.db.Save(feed).Error
}

func (r *ReaderRepository) UpdateFeed(feed *models.Feed, updates map[string]interface{}) error {
	return r.db.Model(feed).Updates(updates).Error
}

func (r *ReaderRepository) DeleteReadingBehaviorsByFeed(feedID uint) error {
	return r.db.Where("feed_id = ?", feedID).Delete(&models.ReadingBehavior{}).Error
}

func (r *ReaderRepository) ListFeeds(opts FeedFilter) ([]models.Feed, int64, error) {
	query := r.db.Model(&models.Feed{})
	if opts.CategoryID > 0 {
		query = query.Where("category_id = ?", opts.CategoryID)
	}
	if opts.Uncategorized {
		query = query.Where("category_id IS NULL")
	}
	var total int64
	query.Count(&total)

	var feeds []models.Feed
	err := query.Order("title ASC").Find(&feeds).Error
	return feeds, total, err
}

func (r *ReaderRepository) ListAllFeeds() ([]models.Feed, error) {
	var feeds []models.Feed
	err := r.db.Find(&feeds).Error
	return feeds, err
}

func (r *ReaderRepository) BulkUpdateFeedStatus(feedIDs []uint, updates map[string]interface{}) error {
	return r.db.Model(&models.Feed{}).Where("id IN ?", feedIDs).Updates(updates).Error
}

func (r *ReaderRepository) UpdateFeedWhere(where map[string]interface{}, updates map[string]interface{}) error {
	return r.db.Model(&models.Feed{}).Where(where).Updates(updates).Error
}

// ============================================================================
// Article operations
// ============================================================================

func (r *ReaderRepository) GetArticle(id uint) (*models.Article, error) {
	var article models.Article
	err := r.db.First(&article, id).Error
	if err != nil {
		return nil, err
	}
	return &article, nil
}

func (r *ReaderRepository) GetArticleWithStats(articleID uint) (*models.Article, error) {
	var article models.Article
	err := r.db.Model(&models.Article{}).
		Joins("LEFT JOIN feeds ON feeds.id = articles.feed_id").
		Select("articles.*").
		First(&article, articleID).Error
	return &article, err
}

func (r *ReaderRepository) GetArticleWithTagCount(articleID uint) (*models.Article, error) {
	// TODO: replace subquery with a proper repository method if needed
	var article models.Article
	err := r.db.First(&article, articleID).Error
	return &article, err
}

func (r *ReaderRepository) GetArticleStats() (total, unread, favorite int64, err error) {
	type Stats struct {
		Total    int64
		Unread   int64
		Favorite int64
	}
	var s Stats
	err = r.db.Model(&models.Article{}).
		Select("COUNT(*) as total, SUM(CASE WHEN NOT read THEN 1 ELSE 0 END) as unread, SUM(CASE WHEN favorite THEN 1 ELSE 0 END) as favorite").
		Where("archived = ?", false).
		Scan(&s).Error
	return s.Total, s.Unread, s.Favorite, err
}

func (r *ReaderRepository) UpdateArticle(id uint, updates map[string]interface{}) error {
	return r.db.Model(&models.Article{}).Where("id = ?", id).Updates(updates).Error
}

func (r *ReaderRepository) BulkUpdateArticles(ids []uint, updates map[string]interface{}) error {
	return r.db.Model(&models.Article{}).Where("id IN ?", ids).Updates(updates).Error
}

func (r *ReaderRepository) CountArticlesByFeed(feedID uint) (int64, error) {
	var count int64
	err := r.db.Model(&models.Article{}).Where("feed_id = ?", feedID).Count(&count).Error
	return count, err
}

func (r *ReaderRepository) ListArticlesByFeed(feedID uint, order string, limit int) ([]models.Article, error) {
	var articles []models.Article
	query := r.db.Model(&models.Article{}).
		Select("id, feed_id, pub_date, title").
		Where("feed_id = ?", feedID).
		Order(order)
	if limit > 0 {
		query = query.Limit(limit)
	}
	err := query.Find(&articles).Error
	return articles, err
}

// ListArticlesByFeedAndStatuses returns articles for a feed whose summary_status
// is in the given set, omitting heavy computed columns (tag_count, relevance_score).
func (r *ReaderRepository) ListArticlesByFeedAndStatuses(feedID uint, statuses []string) ([]models.Article, error) {
	var articles []models.Article
	err := r.db.Omit("tag_count", "relevance_score").
		Where("feed_id = ? AND summary_status IN ?", feedID, statuses).
		Find(&articles).Error
	return articles, err
}

// articleDeleteChunk bounds the IN (?) lists that name article ids. A feed can
// hold thousands of rows (max_articles=9999 means unlimited) while PostgreSQL
// caps a statement at 65535 bind parameters, so the rows are removed in windows
// instead of one flat list.
const articleDeleteChunk = 1000

// deleteArticlesWithRefs removes every article row of a feed and, first, the
// daily-report references those rows carry. An article that disappears without
// its references leaves 线索 resolving to "文章 #<id>" instead of a source
// (heal-dangling-article-refs D2). Runs on the caller's transaction so the
// reference rewrite and the row deletion commit or roll back together.
func (r *ReaderRepository) deleteArticlesWithRefs(tx *gorm.DB, feedID uint) error {
	return r.deleteArticlesOfFeedsWithRefs(tx, []uint{feedID})
}

// deleteArticlesOfFeedsWithRefs is the batched form of deleteArticlesWithRefs for
// callers deleting more than one feed at a time (category deletion): one pluck,
// one prune, one delete instead of a pass per feed. Callers own the transaction.
//
// The rows are named by id rather than deleted by feed_id so that exactly the
// articles whose references were just pruned are the ones removed.
func (r *ReaderRepository) deleteArticlesOfFeedsWithRefs(tx *gorm.DB, feedIDs []uint) error {
	if len(feedIDs) == 0 {
		return nil
	}
	var articleIDs []uint
	if err := tx.Model(&models.Article{}).
		Where("feed_id IN ?", feedIDs).
		Pluck("id", &articleIDs).Error; err != nil {
		return err
	}
	if len(articleIDs) == 0 {
		return nil
	}
	// Order matters: the references these articles carry, then the rows that
	// belong to the articles, then the articles themselves — all inside the
	// caller's transaction, so a failure rolls the whole deletion back.
	if _, err := articlerefs.PruneArticleRefs(tx, articleIDs); err != nil {
		return err
	}
	// Resolve the dependent tables once for the whole call: HasTable is a
	// metadata query and a 9999-article feed would otherwise repeat it for every
	// chunk.
	dependents := existingArticleDependents(tx)
	for start := 0; start < len(articleIDs); start += articleDeleteChunk {
		end := start + articleDeleteChunk
		if end > len(articleIDs) {
			end = len(articleIDs)
		}
		chunk := articleIDs[start:end]
		if err := r.deleteArticleDependents(tx, chunk, dependents); err != nil {
			return err
		}
		if err := tx.Where("id IN ?", chunk).Delete(&models.Article{}).Error; err != nil {
			return err
		}
	}
	return nil
}

// articleDependent names one per-article row family and the model used to
// delete it.
type articleDependent struct {
	table string
	model interface{}
}

// articleDependents lists the row families that must not outlive their article:
// the tag edges plus the queued jobs. Order is fixed so the deletes are
// predictable when a database still carries the legacy cascade foreign keys.
var articleDependents = []articleDependent{
	{"article_topic_tags", &models.ArticleTopicTag{}},
	{"tag_jobs", &models.TagJob{}},
	{"firecrawl_jobs", &models.FirecrawlJob{}},
}

// existingArticleDependents keeps the entries this database actually has.
//
// Boundary (deliberate, narrow-deployment compatibility): GORM's HasTable
// returns a bare bool, so a metadata query that fails reads as false — the
// dependent cleanup is then skipped while the article rows are still deleted,
// which can leave orphans behind. That fail-open trade is preferred over
// failing a user's deletion on a table probe. Only table existence is guarded,
// not column existence: a missing column makes the DELETE itself error, which
// rolls the whole transaction back (fail-closed).
func existingArticleDependents(tx *gorm.DB) []articleDependent {
	existing := make([]articleDependent, 0, len(articleDependents))
	for _, dependent := range articleDependents {
		if tx.Migrator().HasTable(dependent.table) {
			existing = append(existing, dependent)
		}
	}
	return existing
}

// deleteArticleDependents removes the per-article rows that must not outlive
// their article: the tag edges, the queued tag jobs and the queued firecrawl
// jobs. A long-lived database cascades them through foreign keys
// (fk_article_topic_tags_article / fk_tag_jobs_article / fk_firecrawl_jobs_article),
// but AutoMigrate does not create those constraints in a freshly built database
// (DisableForeignKeyConstraintWhenMigrating), so they are removed explicitly and
// the deletion behaves identically either way. Runs on the caller's transaction.
//
// Deliberately out of scope: topic_tags left without edges are reclaimed by the
// aux_label_cleanup maintenance job — the same owner as when a cascade removes
// the edges — and reading_behaviors is untouched, because its NO ACTION foreign
// key already fails such a delete today (DeleteFeedCascade removes them per feed
// first); deleting them here would silently widen the operation.
//
// dependents carries the tables the caller resolved once (see
// existingArticleDependents): a table absent from a narrow deployment is skipped
// rather than failing the delete.
func (r *ReaderRepository) deleteArticleDependents(tx *gorm.DB, articleIDs []uint, dependents []articleDependent) error {
	if len(articleIDs) == 0 {
		return nil
	}
	for _, dependent := range dependents {
		if err := tx.Where("article_id IN ?", articleIDs).Delete(dependent.model).Error; err != nil {
			return err
		}
	}
	return nil
}

func (r *ReaderRepository) DeleteArticlesByFeed(feedID uint) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		return r.deleteArticlesWithRefs(tx, feedID)
	})
}

func (r *ReaderRepository) DeleteCascadeByFeed(feedID uint) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("feed_id = ?", feedID).Delete(&models.ReadingBehavior{}).Error; err != nil {
			return err
		}
		return r.deleteArticlesWithRefs(tx, feedID)
	})
}

// DeleteFeedCascade deletes a feed together with its dependent rows. The
// articles used to disappear through the articles.feed_id ON DELETE CASCADE
// alone, which left daily-report references pointing at deleted rows; they are
// now deleted explicitly — together with their own dependent rows — after their
// references are pruned, in the same transaction as the feed row itself.
func (r *ReaderRepository) DeleteFeedCascade(feedID uint) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("feed_id = ?", feedID).Delete(&models.ReadingBehavior{}).Error; err != nil {
			return err
		}
		if err := r.deleteArticlesWithRefs(tx, feedID); err != nil {
			return err
		}
		return tx.Where("id = ?", feedID).Delete(&models.Feed{}).Error
	})
}

// DeleteCategoryCascade deletes a category together with the feeds and articles
// filed under it, pruning the daily-report references those articles carry first.
//
// The rows used to disappear through the legacy fk_categories_feeds ON DELETE
// CASCADE — an AutoMigrate-era constraint that no versioned migration creates
// and that DisableForeignKeyConstraintWhenMigrating keeps out of freshly built
// databases. Depending on it would make "references are pruned only for rows
// that really go away" true in some databases and false in others: without the
// cascade the prune would hit articles that survive. The deletion is therefore
// explicit, which is what the constraint does wherever it exists.
//
// reading_behaviors / user_preferences are deliberately untouched: their NO
// ACTION foreign keys already fail this deletion today, and deleting them to make
// it succeed would silently widen an admin operation. Everything runs in one
// transaction, so a constraint failure rolls the prune back with it.
func (r *ReaderRepository) DeleteCategoryCascade(categoryID uint) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		var feedIDs []uint
		if err := tx.Model(&models.Feed{}).
			Where("category_id = ?", categoryID).
			Pluck("id", &feedIDs).Error; err != nil {
			return err
		}
		if err := r.deleteArticlesOfFeedsWithRefs(tx, feedIDs); err != nil {
			return err
		}
		if len(feedIDs) > 0 {
			if err := tx.Where("id IN ?", feedIDs).Delete(&models.Feed{}).Error; err != nil {
				return err
			}
		}
		return tx.Where("id = ?", categoryID).Delete(&models.Category{}).Error
	})
}

func (r *ReaderRepository) PluckArticlesTitlesByFeed(feedID uint) ([]string, error) {
	var titles []string
	err := r.db.Model(&models.Article{}).
		Where("feed_id = ?", feedID).
		Pluck("title", &titles).Error
	return titles, err
}

func (r *ReaderRepository) CreateArticle(article *models.Article) error {
	return r.db.Create(article).Error
}

func (r *ReaderRepository) SaveArticle(article *models.Article) error {
	return r.db.Save(article).Error
}

func (r *ReaderRepository) ListArticlesIncomplete(limit int) ([]models.Article, error) {
	var articles []models.Article
	err := r.db.Omit("ContentText", "ContentHTML", "ContentPlain", "SummaryText").
		Where("feed_id IN (SELECT id FROM feeds WHERE article_summary_enabled = true AND firecrawl_enabled = false)").
		Where("summary_status IN ?", []string{"pending", ""}).
		Limit(limit).
		Find(&articles).Error
	return articles, err
}

func (r *ReaderRepository) CountPendingArticles() (int64, error) {
	var count int64
	err := r.db.Model(&models.Article{}).
		Where("(summary_status = '' OR summary_status = 'pending')").
		Where("read = false").
		Count(&count).Error
	return count, err
}

func (r *ReaderRepository) ListArticlesForCompletion(batchQuery ItemQuery, limit int) ([]models.Article, int64, error) {
	// Used by content completion service
	query := r.db.Joins("JOIN feeds ON feeds.id = articles.feed_id").
		Where("feeds.article_summary_enabled = true AND feeds.completion_on_refresh = true AND feeds.firecrawl_enabled = false").
		Where("articles.summary_status IN ('', 'pending', 'failed')").
		Where("feeds.max_completion_retries > articles.summary_fail_count").
		Where("(articles.summary_last_attempt IS NULL OR articles.summary_last_attempt < ?)", batchQuery.CutoffTime).
		Omit("ContentText", "ContentHTML", "ContentPlain", "SummaryText").
		Preload("Feed")

	var count int64
	if err := query.Count(&count).Error; err != nil {
		return nil, 0, err
	}

	var articles []models.Article
	err := query.Limit(limit).Order("articles.summary_last_attempt ASC").Find(&articles).Error
	return articles, count, err
}

// ============================================================================
// Tag operations (articles ↔ tags)
// ============================================================================

func (r *ReaderRepository) PluckArticleTopicTagIDs(feedID uint) ([]uint, error) {
	var ids []uint
	err := r.db.Model(&models.ArticleTopicTag{}).
		Joins("JOIN articles ON articles.id = article_topic_tags.article_id").
		Where("articles.feed_id = ?", feedID).
		Pluck("topic_tag_id", &ids).Error
	return ids, err
}

// ============================================================================
// Content completion claim
// ============================================================================

func (r *ReaderRepository) ClaimArticleForCompletion(ctx context.Context, articleID uint, force bool, now interface{}) (bool, error) {
	query := r.db.WithContext(ctx).Model(&models.Article{}).Where("id = ?", articleID)
	if !force {
		query = query.Where("(summary_last_attempt IS NULL OR summary_last_attempt <= ?)", now)
		query = query.Where("summary_status IN ?", []string{"", "pending", "failed"})
	}
	updates := map[string]interface{}{
		"summary_status":       "processing",
		"summary_last_attempt": now,
	}
	result := query.Updates(updates)
	return result.RowsAffected > 0, result.Error
}

// ============================================================================
// Firecrawl operations
// ============================================================================

func (r *ReaderRepository) BulkUpdateFirecrawlStatus(ids []uint, status string) error {
	return r.db.Model(&models.Article{}).
		Where("id IN ?", ids).
		Where("firecrawl_status IS NULL OR firecrawl_status = ''").
		Updates(map[string]interface{}{"firecrawl_status": status}).Error
}

// ============================================================================
// OPML operations
// ============================================================================

func (r *ReaderRepository) ListCategoriesWithFeeds() ([]models.Category, error) {
	var categories []models.Category
	err := r.db.Preload("Feeds").Find(&categories).Error
	return categories, err
}

func (r *ReaderRepository) ListUncategorizedFeeds() ([]models.Feed, error) {
	var feeds []models.Feed
	err := r.db.Where("category_id IS NULL").Find(&feeds).Error
	return feeds, err
}

// ============================================================================
// Types
// ============================================================================

type FeedFilter struct {
	CategoryID    int
	Uncategorized bool
}

type ItemQuery struct {
	CutoffTime string
}
