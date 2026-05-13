package article

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"my-robot-backend/internal/domain/models"
	"my-robot-backend/internal/domain/tagging"
	"my-robot-backend/internal/domain/tagging/watched"
	"my-robot-backend/internal/platform/database"
)

func loadArticleWithTagCount(articleID uint) (*models.Article, error) {
	var article models.Article
	if err := database.DB.Model(&models.Article{}).
		Joins("LEFT JOIN feeds ON articles.feed_id = feeds.id").
		Select("articles.*, feeds.category_id AS category_id, (SELECT COUNT(*) FROM article_topic_tags att_cnt WHERE att_cnt.article_id = articles.id) AS tag_count").
		First(&article, articleID).Error; err != nil {
		return nil, err
	}

	return &article, nil
}

type UpdateArticleRequest struct {
	Read     *bool `json:"read"`
	Favorite *bool `json:"favorite"`
}

type BulkUpdateArticlesRequest struct {
	IDs           []uint `json:"ids"`
	FeedID        *uint  `json:"feed_id"`
	CategoryID    *uint  `json:"category_id"`
	Uncategorized *bool  `json:"uncategorized"`
	Read          *bool  `json:"read"`
	Favorite      *bool  `json:"favorite"`
}

func GetArticlesStats(c *gin.Context) {
	type StatsResult struct {
		Total    int64
		Unread   int64
		Favorite int64
	}
	var result StatsResult
	database.DB.Model(&models.Article{}).
		Select("COUNT(*) as total, COALESCE(SUM(CASE WHEN NOT read THEN 1 ELSE 0 END), 0) as unread, COALESCE(SUM(CASE WHEN favorite THEN 1 ELSE 0 END), 0) as favorite").
		Scan(&result)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"total":    result.Total,
			"unread":   result.Unread,
			"favorite": result.Favorite,
		},
	})
}

func GetArticles(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	perPage, _ := strconv.Atoi(c.DefaultQuery("per_page", "20"))
	feedID, _ := strconv.Atoi(c.Query("feed_id"))
	categoryID, _ := strconv.Atoi(c.Query("category_id"))
	uncategorized := c.Query("uncategorized") == "true"
	read := c.Query("read")
	favorite := c.Query("favorite")
	search := c.Query("search")
	startDate := c.Query("start_date")
	endDate := c.Query("end_date")
	watchedTagIDsStr := c.Query("watched_tag_ids")
	watchedTagsMode := c.Query("watched_tags") == "true"
	sortBy := c.Query("sort_by")

	maxPerPage := 100
	if perPage <= 0 || perPage > maxPerPage {
		perPage = maxPerPage
	}

	var expandedTagIDs []uint
	var childTagIDs []uint
	usingWatchedTags := false

	if watchedTagsMode {
		watchedIDs, children, err := watched.GetWatchedTagIDsExpanded(database.DB)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "failed to expand watched tags"})
			return
		}
		if len(watchedIDs) > 0 {
			expandedTagIDs = append(expandedTagIDs, watchedIDs...)
			expandedTagIDs = append(expandedTagIDs, children...)
			childTagIDs = children
			usingWatchedTags = true
		}
	} else if watchedTagIDsStr != "" {
		parsedIDs, children, err := parseAndExpandWatchedTagIDs(watchedTagIDsStr)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid watched_tag_ids format"})
			return
		}
		if len(parsedIDs) > 0 {
			expandedTagIDs = append(expandedTagIDs, parsedIDs...)
			expandedTagIDs = append(expandedTagIDs, children...)
			childTagIDs = children
			usingWatchedTags = true
		}
	}

	// Build base query with standard joins
	query := database.DB.Model(&models.Article{}).
		Joins("LEFT JOIN feeds ON articles.feed_id = feeds.id")

	// Apply watched tag filter via JOIN
	if usingWatchedTags {
		query = query.Joins("JOIN article_topic_tags att ON att.article_id = articles.id AND att.topic_tag_id IN ?", expandedTagIDs)
	}

	// Select fields — vary by watched tags mode and sort
	articleCols := "articles.id, articles.feed_id, articles.title, articles.description, articles.content, articles.link, articles.image_url, articles.pub_date, articles.author, articles.read, articles.favorite, articles.summary_status, articles.summary_generated_at, articles.summary_processing_started_at, articles.completion_attempts, articles.completion_error, articles.ai_content_summary, articles.firecrawl_status, articles.firecrawl_error, articles.firecrawl_content, articles.firecrawl_crawled_at, articles.created_at"
	if usingWatchedTags {
		switch sortBy {
		case "relevance":
			query = query.
				Select(articleCols+", feeds.category_id AS category_id, (SELECT COUNT(*) FROM article_topic_tags att_cnt WHERE att_cnt.article_id = articles.id) AS tag_count, (SELECT COALESCE(SUM(CASE WHEN att2.topic_tag_id IN ? THEN 2.0 ELSE 1.0 END), 0) FROM article_topic_tags att2 WHERE att2.article_id = articles.id AND att2.topic_tag_id IN ?) AS relevance_score", childTagIDs, expandedTagIDs).
				Group("articles.id, feeds.category_id")
		default:
			query = query.
				Select("DISTINCT articles.*, feeds.category_id AS category_id, (SELECT COUNT(*) FROM article_topic_tags att_cnt WHERE att_cnt.article_id = articles.id) AS tag_count")
		}
	} else {
		query = query.
			Select("articles.*, feeds.category_id AS category_id, (SELECT COUNT(*) FROM article_topic_tags att_cnt WHERE att_cnt.article_id = articles.id) AS tag_count")
	}

	if feedID > 0 {
		query = query.Where("articles.feed_id = ?", feedID)
	}

	if categoryID > 0 {
		query = query.Where("feeds.category_id = ?", categoryID)
	}

	if uncategorized {
		query = query.Where("feeds.category_id IS NULL")
	}

	if read == "true" || read == "false" {
		query = query.Where("articles.read = ?", read == "true")
	}

	if favorite == "true" || favorite == "false" {
		query = query.Where("articles.favorite = ?", favorite == "true")
	}

	if search != "" {
		if database.DB.Name() == "postgres" {
			query = query.Where("articles.search_vector @@ plainto_tsquery('simple', ?)", search)
		} else {
			searchTerm := "%" + search + "%"
			query = query.Where("articles.title LIKE ? OR articles.description LIKE ?", searchTerm, searchTerm)
		}
	}

	if startDate != "" {
		query = query.Where("DATE(articles.pub_date) >= ?", startDate)
	}

	if endDate != "" {
		query = query.Where("DATE(articles.pub_date) <= ?", endDate)
	}

	// Ordering
	if usingWatchedTags && sortBy == "relevance" {
		query = query.Order("relevance_score DESC, articles.pub_date DESC")
	} else {
		query = query.Order("articles.pub_date DESC")
	}

	// Count — handle separately for watched tags to avoid JOIN inflation
	var total int64
	if usingWatchedTags {
		countQuery := database.DB.Table("articles").
			Joins("JOIN article_topic_tags att ON att.article_id = articles.id AND att.topic_tag_id IN ?", expandedTagIDs)
		if feedID > 0 {
			countQuery = countQuery.Where("articles.feed_id = ?", feedID)
		}
		if categoryID > 0 {
			countQuery = countQuery.Joins("JOIN feeds ON articles.feed_id = feeds.id").Where("feeds.category_id = ?", categoryID)
		}
		if uncategorized {
			countQuery = countQuery.Joins("JOIN feeds ON articles.feed_id = feeds.id").Where("feeds.category_id IS NULL")
		}
		if read == "true" || read == "false" {
			countQuery = countQuery.Where("articles.read = ?", read == "true")
		}
		if favorite == "true" || favorite == "false" {
			countQuery = countQuery.Where("articles.favorite = ?", favorite == "true")
		}
		if search != "" {
			if database.DB.Name() == "postgres" {
				countQuery = countQuery.Where("articles.search_vector @@ plainto_tsquery('simple', ?)", search)
			} else {
				searchTerm := "%" + search + "%"
				countQuery = countQuery.Where("articles.title LIKE ? OR articles.description LIKE ?", searchTerm, searchTerm)
			}
		}
		if startDate != "" {
			countQuery = countQuery.Where("DATE(articles.pub_date) >= ?", startDate)
		}
		if endDate != "" {
			countQuery = countQuery.Where("DATE(articles.pub_date) <= ?", endDate)
		}
		countQuery.Select("COUNT(DISTINCT articles.id)").Scan(&total)
	} else {
		query.Count(&total)
	}

	var articles []models.Article
	offset := (page - 1) * perPage
	if err := query.Offset(offset).Limit(perPage).Find(&articles).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	data := make([]map[string]interface{}, len(articles))
	for i, article := range articles {
		data[i] = article.ToDict()
	}

	pages := int(total) / perPage
	if int(total)%perPage > 0 {
		pages++
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    data,
		"pagination": gin.H{
			"page":     page,
			"per_page": perPage,
			"total":    total,
			"pages":    pages,
		},
	})
}

// parseAndExpandWatchedTagIDs parses comma-separated tag IDs and recursively expands abstract tag children.
func parseAndExpandWatchedTagIDs(raw string) ([]uint, []uint, error) {
	parts := strings.Split(raw, ",")
	ids := make([]uint, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		id, err := strconv.ParseUint(p, 10, 32)
		if err != nil {
			return nil, nil, err
		}
		if id > 0 {
			ids = append(ids, uint(id))
		}
	}
	if len(ids) == 0 {
		return nil, nil, nil
	}

	childIDs := collectDescendantTagIDs(ids)
	return ids, childIDs, nil
}

// collectDescendantTagIDs recursively collects all descendant tag IDs for the given parent IDs.
func collectDescendantTagIDs(parentIDs []uint) []uint {
	allChildren := make([]uint, 0)
	visited := make(map[uint]bool)
	queue := make([]uint, len(parentIDs))
	copy(queue, parentIDs)

	for len(queue) > 0 {
		var relations []models.TopicTagRelation
		if err := database.DB.Where("parent_id IN ?", queue).Find(&relations).Error; err != nil {
			break
		}

		queue = queue[:0]
		for _, rel := range relations {
			if !visited[rel.ChildID] {
				visited[rel.ChildID] = true
				allChildren = append(allChildren, rel.ChildID)
				queue = append(queue, rel.ChildID)
			}
		}
	}

	return allChildren
}

func GetArticle(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("article_id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Invalid article ID",
		})
		return
	}

	article, err := loadArticleWithTagCount(uint(id))
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{
				"success": false,
				"error":   "Article not found",
			})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"error":   err.Error(),
			})
		}
		return
	}

	articleData := article.ToDict()
	tags, err := tagging.GetArticleTags(article.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}
	articleData["tags"] = tags

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    articleData,
	})
}

func RetagArticleHandler(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("article_id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Invalid article ID"})
		return
	}

	var article models.Article
	if err := database.DB.First(&article, uint(id)).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"success": false, "error": "Article not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	var feed models.Feed
	if err := database.DB.Preload("Category").First(&feed, article.FeedID).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	queue := tagging.NewTagJobQueue(database.DB)
	if err := queue.Enqueue(tagging.TagJobRequest{
		ArticleID:    article.ID,
		FeedName:     feed.Title,
		CategoryName: tagging.FeedCategoryName(feed),
		ForceRetag:   true,
		Reason:       "manual_api_trigger",
	}); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	// Query both pending and leased: Enqueue may reuse an existing leased job,
	// or a worker may claim the job between Enqueue and this query.
	var tagJob models.TagJob
	if err := database.DB.Where("article_id = ? AND status IN ?", article.ID,
		[]string{string(models.JobStatusPending), string(models.JobStatusLeased)}).
		Order("id DESC").
		First(&tagJob).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "failed to retrieve job_id"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "标签任务已提交，请稍后刷新查看结果",
		"data": gin.H{
			"job_id":     tagJob.ID,
			"article_id": article.ID,
			"status":     tagJob.Status,
		},
	})
}

func UpdateArticle(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("article_id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Invalid article ID",
		})
		return
	}

	var article models.Article
	if err := database.DB.First(&article, uint(id)).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{
				"success": false,
				"error":   "Article not found",
			})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"error":   err.Error(),
			})
		}
		return
	}

	var req UpdateArticleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Invalid request body",
		})
		return
	}

	updates := make(map[string]interface{})
	if req.Read != nil {
		updates["read"] = *req.Read
	}
	if req.Favorite != nil {
		updates["favorite"] = *req.Favorite
	}

	if len(updates) > 0 {
		if err := database.DB.Model(&article).Updates(updates).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"error":   err.Error(),
			})
			return
		}
	}

	database.DB.Model(&models.Article{}).
		Joins("LEFT JOIN feeds ON articles.feed_id = feeds.id").
		Select("articles.*, feeds.category_id AS category_id, (SELECT COUNT(*) FROM article_topic_tags att_cnt WHERE att_cnt.article_id = articles.id) AS tag_count").
		First(&article, uint(id))

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    article.ToDict(),
	})
}

func BulkUpdateArticles(c *gin.Context) {
	var req BulkUpdateArticlesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	updates := make(map[string]interface{})
	if req.Read != nil {
		updates["read"] = *req.Read
	}
	if req.Favorite != nil {
		updates["favorite"] = *req.Favorite
	}

	if len(updates) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "At least one field (read or favorite) must be specified",
		})
		return
	}

	if len(req.IDs) == 0 && req.FeedID == nil && req.CategoryID == nil && (req.Uncategorized == nil || !*req.Uncategorized) {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Must specify a scope: ids, feed_id, category_id, or uncategorized",
		})
		return
	}

	query := database.DB.Model(&models.Article{})

	switch {
	case len(req.IDs) > 0:
		query = query.Where("id IN ?", req.IDs)
	case req.FeedID != nil:
		query = query.Where("feed_id = ?", *req.FeedID)
	case req.CategoryID != nil:
		query = query.Where("feed_id IN (SELECT id FROM feeds WHERE category_id = ?)", *req.CategoryID)
	case req.Uncategorized != nil && *req.Uncategorized:
		query = query.Where("feed_id IN (SELECT id FROM feeds WHERE category_id IS NULL)")
	}

	result := query.Updates(updates)

	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   result.Error.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": result.RowsAffected,
	})
}
