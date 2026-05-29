package topicgraph

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"syntopica-backend/internal/domain/tagging"
)

func GetTopicGraph(c *gin.Context) {
	kind := c.Param("type")
	anchor, err := tagging.ParseAnchorDate(c.Query("date"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}

	categoryID := parseOptionalUintParam(c, "category_id")
	feedID := parseOptionalUintParam(c, "feed_id")

	graph, err := BuildTopicGraph(kind, anchor, categoryID, feedID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": graph})
}

func GetTopicDetail(c *gin.Context) {
	slug := c.Param("slug")
	kind := c.Query("type")
	if kind == "" {
		kind = "all"
	}
	anchor, err := tagging.ParseAnchorDate(c.Query("date"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}

	detail, err := BuildTopicDetail(kind, slug, anchor, parseOptionalUintParam(c, "category_id"), parseOptionalUintParam(c, "feed_id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": detail})
}

// GetTopicsByCategory returns tags grouped by category (event, person, keyword)
func GetTopicsByCategory(c *gin.Context) {
	kind := c.DefaultQuery("type", "daily")
	anchor, err := tagging.ParseAnchorDate(c.Query("date"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}

	result, err := BuildTopicsByCategory(kind, anchor, parseOptionalUintParam(c, "category_id"), parseOptionalUintParam(c, "feed_id"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": result})
}

// GetTopicArticles returns paginated articles for a topic
func GetTopicArticles(c *gin.Context) {
	slug := c.Param("slug")
	if slug == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "slug is required"})
		return
	}

	// Parse query parameters
	kind := c.DefaultQuery("type", "all")
	page := 1
	pageSize := 15

	if p := c.Query("page"); p != "" {
		if parsed, err := parseIntParam(p, 1, 1000); err == nil {
			page = parsed
		}
	}
	if ps := c.Query("page_size"); ps != "" {
		if parsed, err := parseIntParam(ps, 1, 100); err == nil {
			pageSize = parsed
		}
	}

	anchor, err := tagging.ParseAnchorDate(c.Query("date"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}

	articles, total, err := FetchTopicArticles(slug, kind, anchor, page, pageSize)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"articles":  articles,
			"total":     total,
			"page":      page,
			"page_size": pageSize,
		},
	})
}

// GetDigestsByArticleTagHandler returns digests that contain articles with the given tag
func GetDigestsByArticleTagHandler(c *gin.Context) {
	tagSlug := c.Param("slug")
	if tagSlug == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "tag slug is required"})
		return
	}

	kind := c.Query("type")
	if kind == "" {
		kind = "all"
	}
	anchor, err := tagging.ParseAnchorDate(c.Query("date"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}

	limit := 20
	if l := c.Query("limit"); l != "" {
		if parsed, err := parseIntParam(l, 1, 100); err == nil {
			limit = parsed
		}
	}

	tagKind := c.Query("kind")

	digests, err := GetDigestsByArticleTag(tagSlug, kind, anchor, limit, tagKind)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"digests": digests,
			"total":   len(digests),
		},
	})
}

// parseIntParam parses an integer query parameter with bounds checking
func parseIntParam(value string, min, max int) (int, error) {
	var result int
	if _, err := fmt.Sscanf(value, "%d", &result); err != nil {
		return 0, err
	}
	if result < min {
		return min, nil
	}
	if result > max {
		return max, nil
	}
	return result, nil
}

func parseOptionalUintParam(c *gin.Context, key string) *uint {
	val := c.Query(key)
	if val == "" {
		return nil
	}
	v, err := strconv.ParseUint(val, 10, 64)
	if err != nil {
		return nil
	}
	u := uint(v)
	return &u
}

// GetPendingArticlesByTagHandler returns articles with the given tag that are not in any digest
func GetPendingArticlesByTagHandler(c *gin.Context) {
	tagSlug := c.Param("slug")
	if tagSlug == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "tag slug is required"})
		return
	}

	kind := c.Query("type")
	if kind == "" {
		kind = "all"
	}
	anchor, err := tagging.ParseAnchorDate(c.Query("date"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}

	result, err := GetPendingArticlesByTag(tagSlug, kind, anchor)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    result,
	})
}
