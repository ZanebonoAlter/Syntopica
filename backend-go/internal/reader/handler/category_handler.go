package handler

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"syntopica-backend/internal/models"
	"syntopica-backend/internal/reader/repository"
)

type CreateCategoryRequest struct {
	Name        string `json:"name" binding:"required"`
	Slug        string `json:"slug"`
	Icon        string `json:"icon"`
	Color       string `json:"color"`
	Description string `json:"description"`
}

type UpdateCategoryRequest struct {
	Name        string `json:"name"`
	Slug        string `json:"slug"`
	Icon        string `json:"icon"`
	Color       string `json:"color"`
	Description string `json:"description"`
}

// ============================================================================
// Category handlers — HTTP param parsing only, all DB via repository.Repo
// ============================================================================

func GetCategories(c *gin.Context) {
	categories, err := repository.Repo.ListCategories()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	feedCountMap, _ := repository.Repo.GetFeedCountGroupedByCategory()

	data := make([]map[string]interface{}, len(categories))
	for i, cat := range categories {
		catDict := cat.ToDict()
		catDict["feed_count"] = feedCountMap[cat.ID]
		data[i] = catDict
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    data,
	})
}

func CreateCategory(c *gin.Context) {
	var req CreateCategoryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Category name is required",
		})
		return
	}

	if _, err := repository.Repo.GetCategoryByName(req.Name); err == nil {
		c.JSON(http.StatusConflict, gin.H{
			"success": false,
			"error":   "Category with this name already exists",
		})
		return
	}

	slug := req.Slug
	if slug == "" {
		slug = models.GenerateSlug(req.Name)
	}

	category := models.Category{
		Name:        req.Name,
		Slug:        slug,
		Icon:        req.Icon,
		Color:       req.Color,
		Description: req.Description,
	}

	if category.Icon == "" {
		category.Icon = "folder"
	}
	if category.Color == "" {
		category.Color = "#6366f1"
	}

	if err := repository.Repo.CreateCategory(&category); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"success": true,
		"data":    category.ToDict(),
	})
}

func UpdateCategory(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("category_id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Invalid category ID",
		})
		return
	}

	category, err := repository.Repo.GetCategory(uint(id))
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{
				"success": false,
				"error":   "Category not found",
			})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"error":   err.Error(),
			})
		}
		return
	}

	var req UpdateCategoryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Invalid request body",
		})
		return
	}

	updates := make(map[string]interface{})
	if req.Name != "" {
		updates["name"] = req.Name
	}
	if req.Slug != "" {
		updates["slug"] = req.Slug
	}
	if req.Icon != "" {
		updates["icon"] = req.Icon
	}
	if req.Color != "" {
		updates["color"] = req.Color
	}
	if req.Description != "" {
		updates["description"] = req.Description
	}

	if len(updates) > 0 {
		if err := repository.Repo.UpdateCategory(category, updates); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"error":   err.Error(),
			})
			return
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    category.ToDict(),
	})
}

func DeleteCategory(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("category_id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Invalid category ID",
		})
		return
	}

	category, err := repository.Repo.GetCategory(uint(id))
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{
				"success": false,
				"error":   "Category not found",
			})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"error":   err.Error(),
			})
		}
		return
	}

	// Deleting a category takes its feeds and their articles with it, so the
	// repository method owns the transaction: it drops the daily-report
	// references of those articles before the rows go
	// (heal-dangling-article-refs D2).
	if err := repository.Repo.DeleteCategoryCascade(category.ID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Category deleted successfully",
	})
}
