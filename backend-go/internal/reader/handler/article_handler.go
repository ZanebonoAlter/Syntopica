package handler

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/logging"
	"syntopica-backend/internal/reader/repository"
	"syntopica-backend/internal/reader/search"
	"syntopica-backend/internal/reader/service"
	tagging "syntopica-backend/internal/tagmanagement"
)

const (
	defaultPerPage = 20
	maxPerPage     = 100
)

// articleListColumns is the explicit narrow projection for GET /api/articles.
// Body fields (description/content/firecrawl_content/ai_content_summary) are
// deliberately absent — they are served only by GET /api/articles/:id.
// articles.ai_detection_status 是列表行允许的非正文类小字段（疑似 AI 标记）。
const articleListColumns = "articles.id, articles.feed_id, articles.title, articles.link, articles.image_url, articles.pub_date, articles.author, articles.read, articles.favorite, articles.archived, articles.summary_status, articles.summary_generated_at, articles.firecrawl_status, articles.firecrawl_error, articles.firecrawl_crawled_at, articles.completion_error, articles.created_at, articles.ai_detection_status"

// articleListTagCountExpr keeps the pre-existing per-row correlated subquery
// (the articles.tag_count column is 97.5% NULL and cannot be used, design D8).
const articleListTagCountExpr = "(SELECT COUNT(*) FROM article_topic_tags att_cnt WHERE att_cnt.article_id = articles.id) AS tag_count"

func loadArticleWithTagCount(articleID uint) (*models.Article, error) {
	var article models.Article
	if err := repository.Repo.DB().Model(&models.Article{}).
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
	// 疑似 AI 标记的人工纠正（design D10/D14）：复用同一个 PUT 接口。与上面的
	// 已读/收藏是正交字段——不带这两个键的调用行为完全不变。
	AIDetectionStatus *string `json:"ai_detection_status"`
	AIDetectionSource *string `json:"ai_detection_source"`
}

type BulkUpdateArticlesRequest struct {
	IDs           []uint `json:"ids"`
	FeedID        *uint  `json:"feed_id"`
	CategoryID    *uint  `json:"category_id"`
	Uncategorized *bool  `json:"uncategorized"`
	All           *bool  `json:"all"`
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
	repository.Repo.DB().Model(&models.Article{}).
		Select("COUNT(*) as total, COALESCE(SUM(CASE WHEN NOT read THEN 1 ELSE 0 END), 0) as unread, COALESCE(SUM(CASE WHEN favorite THEN 1 ELSE 0 END), 0) as favorite").
		Where("archived = ?", false).
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
	conceptID, _ := strconv.Atoi(c.Query("concept_id"))
	auxiliaryLabelID, _ := strconv.Atoi(c.Query("auxiliary_label_id"))
	uncategorized := c.Query("uncategorized") == "true"
	read := c.Query("read")
	favorite := c.Query("favorite")
	search := c.Query("search")
	startDate := c.Query("start_date")
	endDate := c.Query("end_date")
	watchedTagIDsStr := c.Query("watched_tag_ids")
	watchedTagsMode := c.Query("watched_tags") == "true"
	archived := c.Query("archived") == "true"
	sortBy := c.Query("sort_by")

	requestedPerPage := perPage
	if perPage <= 0 {
		perPage = defaultPerPage
	} else if perPage > maxPerPage {
		perPage = maxPerPage
		logging.Warnf("articles list: per_page=%d exceeds max=%d, clamped to %d (path=%s)", requestedPerPage, maxPerPage, perPage, c.Request.URL.Path)
	}

	var expandedTagIDs []uint
	var childTagIDs []uint
	usingWatchedTags := false

	if watchedTagsMode {
		watchedIDs, children, err := tagging.GetWatchedTagIDsExpanded(repository.Repo.DB())
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

	// Build base query with standard joins. Archived articles are hidden by
	// default; `archived=true` returns only the archive (article-retention spec).
	query := repository.Repo.DB().Model(&models.Article{}).
		Joins("LEFT JOIN feeds ON articles.feed_id = feeds.id").
		Where("articles.archived = ?", archived)

	// Apply watched tag filter via JOIN
	if usingWatchedTags {
		query = query.Joins("JOIN article_topic_tags att ON att.article_id = articles.id AND att.topic_tag_id IN ?", expandedTagIDs)
	}

	// Select fields — narrow projection, vary by watched tags mode and sort
	if usingWatchedTags {
		switch sortBy {
		case "relevance":
			query = query.
				Select(articleListColumns+", feeds.category_id AS category_id, "+articleListTagCountExpr+", (SELECT COALESCE(SUM(CASE WHEN att2.topic_tag_id IN ? THEN 2.0 ELSE 1.0 END), 0) FROM article_topic_tags att2 WHERE att2.article_id = articles.id AND att2.topic_tag_id IN ?) AS relevance_score", childTagIDs, expandedTagIDs).
				Group("articles.id, feeds.category_id")
		default:
			query = query.
				Select("DISTINCT " + articleListColumns + ", feeds.category_id AS category_id, " + articleListTagCountExpr)
		}
	} else {
		if conceptID > 0 || auxiliaryLabelID > 0 {
			query = query.
				Select("DISTINCT " + articleListColumns + ", feeds.category_id AS category_id, " + articleListTagCountExpr)
		} else {
			query = query.
				Select(articleListColumns + ", feeds.category_id AS category_id, " + articleListTagCountExpr)
		}
	}

	if feedID > 0 {
		query = query.Where("articles.feed_id = ?", feedID)
	}

	if categoryID > 0 {
		query = query.Where("feeds.category_id = ?", categoryID)
	}

	if conceptID > 0 {
		query = query.Joins("JOIN article_topic_tags att_concept ON att_concept.article_id = articles.id"+
			" JOIN topic_tag_board_labels ttbl ON ttbl.topic_tag_id = att_concept.topic_tag_id AND ttbl.semantic_board_id = ?", conceptID)
	}

	if auxiliaryLabelID > 0 {
		query = query.Joins("JOIN article_topic_tags att_aux ON att_aux.article_id = articles.id"+
			" JOIN topic_tag_semantic_labels ttsl_filter ON ttsl_filter.topic_tag_id = att_aux.topic_tag_id AND ttsl_filter.semantic_label_id = ?", auxiliaryLabelID)
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
		query = applyArticlesSearchFilter(c.Request.Context(), query, search)
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
		countQuery := repository.Repo.DB().Table("articles").
			Joins("JOIN article_topic_tags att ON att.article_id = articles.id AND att.topic_tag_id IN ?", expandedTagIDs).
			Where("articles.archived = ?", archived)
		if feedID > 0 {
			countQuery = countQuery.Where("articles.feed_id = ?", feedID)
		}
		if categoryID > 0 {
			countQuery = countQuery.Joins("JOIN feeds ON articles.feed_id = feeds.id").Where("feeds.category_id = ?", categoryID)
		}
		if uncategorized {
			countQuery = countQuery.Joins("JOIN feeds ON articles.feed_id = feeds.id").Where("feeds.category_id IS NULL")
		}
		if conceptID > 0 {
			countQuery = countQuery.Joins("JOIN article_topic_tags att_concept ON att_concept.article_id = articles.id JOIN topic_tag_board_labels ttbl ON ttbl.topic_tag_id = att_concept.topic_tag_id AND ttbl.semantic_board_id = ?", conceptID)
		}
		if auxiliaryLabelID > 0 {
			countQuery = countQuery.Joins("JOIN article_topic_tags att_aux ON att_aux.article_id = articles.id JOIN topic_tag_semantic_labels ttsl_filter ON ttsl_filter.topic_tag_id = att_aux.topic_tag_id AND ttsl_filter.semantic_label_id = ?", auxiliaryLabelID)
		}
		if read == "true" || read == "false" {
			countQuery = countQuery.Where("articles.read = ?", read == "true")
		}
		if favorite == "true" || favorite == "false" {
			countQuery = countQuery.Where("articles.favorite = ?", favorite == "true")
		}
		if search != "" {
			countQuery = applyArticlesSearchFilter(c.Request.Context(), countQuery, search)
		}
		if startDate != "" {
			countQuery = countQuery.Where("DATE(articles.pub_date) >= ?", startDate)
		}
		if endDate != "" {
			countQuery = countQuery.Where("DATE(articles.pub_date) <= ?", endDate)
		}
		countQuery.Select("COUNT(DISTINCT articles.id)").Scan(&total)
	} else {
		if conceptID > 0 || auxiliaryLabelID > 0 {
			cq := repository.Repo.DB().Model(&models.Article{}).
				Select("COUNT(DISTINCT articles.id)").
				Where("articles.archived = ?", archived)
			if conceptID > 0 {
				cq = cq.
					Joins("JOIN article_topic_tags att_concept ON att_concept.article_id = articles.id").
					Joins("JOIN topic_tag_board_labels ttbl ON ttbl.topic_tag_id = att_concept.topic_tag_id AND ttbl.semantic_board_id = ?", conceptID)
			}
			if auxiliaryLabelID > 0 {
				cq = cq.
					Joins("JOIN article_topic_tags att_aux ON att_aux.article_id = articles.id").
					Joins("JOIN topic_tag_semantic_labels ttsl_filter ON ttsl_filter.topic_tag_id = att_aux.topic_tag_id AND ttsl_filter.semantic_label_id = ?", auxiliaryLabelID)
			}
			cq.Scan(&total)
		} else {
			query.Count(&total)
		}
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

	articleIDs := make([]uint, 0, len(articles))
	for _, article := range articles {
		articleIDs = append(articleIDs, article.ID)
	}
	excerpts, excerptErr := excerptsByArticleID(repository.Repo.DB(), articleIDs)
	if excerptErr != nil {
		// Excerpts are decorative: never fail the list because of them.
		logging.Warnf("articles list: failed to build excerpts (path=%s): %v", c.Request.URL.Path, excerptErr)
		excerpts = map[uint]string{}
	}

	data := make([]map[string]interface{}, len(articles))
	for i := range articles {
		data[i] = articles[i].ToListDict(excerpts[articles[i].ID])
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

// applyArticlesSearchFilter applies the search filter to an articles-list
// query (add-article-search 2.8). The retired tsvector path (simple tokenizer,
// 97% recall loss on Chinese) is replaced by the search package's D2 pipeline
// fragment (L0/L1/L2 LIKE + L3 similarity, postgres); the sqlite LIKE branch
// stays as-is（列表端点方言分支先例）。归档口径由调用方查询自身保持。
func applyArticlesSearchFilter(ctx context.Context, query *gorm.DB, raw string) *gorm.DB {
	if repository.Repo.DB().Name() == "postgres" {
		if cond, args, ok := search.ListSearchWhere(ctx, repository.Repo.DB(), search.DefaultDict(), raw); ok {
			return query.Where(cond, args...)
		}
		return query
	}
	searchTerm := "%" + raw + "%"
	return query.Where("articles.title LIKE ? OR articles.description LIKE ?", searchTerm, searchTerm)
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
		if err := repository.Repo.DB().Where("parent_id IN ?", queue).Find(&relations).Error; err != nil {
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
	if err := repository.Repo.DB().First(&article, uint(id)).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"success": false, "error": "Article not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	var feed models.Feed
	if err := repository.Repo.DB().Preload("Category").First(&feed, article.FeedID).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	queue := tagging.NewTagJobQueue(repository.Repo.DB())
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
	if err := repository.Repo.DB().Where("article_id = ? AND status IN ?", article.ID,
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
	if err := repository.Repo.DB().First(&article, uint(id)).Error; err != nil {
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
	aiDetectionUpdates, aiErr := buildManualAIDetectionUpdates(req)
	if aiErr != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   aiErr.Error(),
		})
		return
	}
	for key, value := range aiDetectionUpdates {
		updates[key] = value
	}

	if len(updates) > 0 {
		if err := repository.Repo.DB().Model(&article).Updates(updates).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"error":   err.Error(),
			})
			return
		}
	}

	repository.Repo.DB().Model(&models.Article{}).
		Joins("LEFT JOIN feeds ON articles.feed_id = feeds.id").
		Select("articles.*, feeds.category_id AS category_id, (SELECT COUNT(*) FROM article_topic_tags att_cnt WHERE att_cnt.article_id = articles.id) AS tag_count").
		First(&article, uint(id))

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    article.ToDict(),
	})
}

// buildManualAIDetectionUpdates 把人工标记 / 取消标记转成 articles 的判定字段更新
// （design D10/D14）：来源固定 manual——客户端伪造 llm 直接报错，自动结论只由判定
// 链路写。空串是「没写」：取消标记本身就会把来源清空，所以 unknown + 空串与省略同义
// （review R-14）；其余状态下空串仍按非法来源拒绝（标记必须显式给 manual）。
//
// 标记：status + score（疑似取上界 1、非 AI 取下界 0，保持 0-1 契约）+ manual +
// 判定时间。取消（unknown）：置信分 / 来源 / 时间全部清空——未判定不得残留上一轮
// 结论，界面也不呈现任何占位文案。
func buildManualAIDetectionUpdates(req UpdateArticleRequest) (map[string]interface{}, error) {
	if req.AIDetectionStatus == nil {
		if req.AIDetectionSource != nil {
			return nil, errors.New("ai_detection_source requires ai_detection_status")
		}
		return nil, nil
	}

	status := strings.ToLower(strings.TrimSpace(*req.AIDetectionStatus))
	switch status {
	case models.AIDetectionStatusSuspect, models.AIDetectionStatusNotAI, models.AIDetectionStatusUnknown:
	default:
		return nil, fmt.Errorf("invalid ai_detection_status: %q (want suspect|not_ai|unknown)", *req.AIDetectionStatus)
	}

	if req.AIDetectionSource != nil {
		source := strings.ToLower(strings.TrimSpace(*req.AIDetectionSource))
		// 空串只在取消标记时合法（清空分支会把来源一并置空）；标记态必须显式给 manual。
		emptyClearsDetection := source == "" && status == models.AIDetectionStatusUnknown
		if source != models.AIDetectionSourceManual && !emptyClearsDetection {
			return nil, fmt.Errorf("invalid ai_detection_source: %q (only manual is accepted)", *req.AIDetectionSource)
		}
	}

	if status == models.AIDetectionStatusUnknown {
		return map[string]interface{}{
			"ai_detection_status": status,
			"ai_detection_score":  0.0,
			"ai_detection_source": "",
			"ai_detection_at":     nil,
		}, nil
	}

	score := 1.0
	if status == models.AIDetectionStatusNotAI {
		score = 0.0
	}
	return map[string]interface{}{
		"ai_detection_status": status,
		"ai_detection_score":  score,
		"ai_detection_source": models.AIDetectionSourceManual,
		"ai_detection_at":     time.Now().In(models.ShanghaiTZ),
	}, nil
}

// RecheckAIDetection handles POST /api/articles/:article_id/ai-detection/recheck
// （design D10 / task 4.7）：显式重新判定入口。入队一条 manual_recheck 作业，判定
// 执行时绕过「人工结论优先」的跳过——自动结论可以覆盖人工结论。
// 这里刻意不判 feed.ai_detection_enabled：订阅源开关管的是「内容就绪时是否自动
// 判定」，用户显式点重判是独立动作，开关关闭时也应生效。
func RecheckAIDetection(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("article_id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Invalid article ID",
		})
		return
	}

	var article models.Article
	if err := repository.Repo.DB().Omit("tag_count", "relevance_score").First(&article, uint(id)).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
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

	if err := service.RequestAIDetectionRecheck(repository.Repo.DB(), article.ID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	// 查询 pending / leased 两种状态：Enqueue 可能复用已有作业，worker 也可能在
	// 入队与本次查询之间抢到作业。
	var job models.DetectionJob
	if err := repository.Repo.DB().Where("article_id = ? AND status IN ?", article.ID,
		[]string{string(models.JobStatusPending), string(models.JobStatusLeased)}).
		Order("id DESC").
		First(&job).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "failed to retrieve job_id",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "重新判定已提交，请稍后刷新查看结果",
		"data": gin.H{
			"job_id":     job.ID,
			"article_id": article.ID,
			"status":     job.Status,
		},
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

	// fix-bulk-markall-all-scope：all 为显式全站 scope，与其他 scope 互斥（语义冲突 → 400，不静默取优先级）
	allScope := req.All != nil && *req.All
	hasOtherScope := len(req.IDs) > 0 || req.FeedID != nil || req.CategoryID != nil || (req.Uncategorized != nil && *req.Uncategorized)
	if allScope && hasOtherScope {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "all cannot be combined with other scopes",
		})
		return
	}

	if !allScope && len(req.IDs) == 0 && req.FeedID == nil && req.CategoryID == nil && (req.Uncategorized == nil || !*req.Uncategorized) {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Must specify a scope: ids, feed_id, category_id, or uncategorized",
		})
		return
	}

	query := repository.Repo.DB().Model(&models.Article{})

	switch {
	case allScope:
		// 显式 all：全站。GORM 禁止无 WHERE 的批量 UPDATE（safety），恒真条件表达“无范围限定”的显式语义
		query = query.Where("1 = 1")
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
