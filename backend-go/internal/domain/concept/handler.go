package concept

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"my-robot-backend/internal/domain/models"
	"my-robot-backend/internal/platform/logging"
)

// RegisterConceptRoutes registers concept HTTP routes on the given router group.
func RegisterConceptRoutes(router *gin.RouterGroup) {
	group := router.Group("/hierarchy/concepts")
	{
		group.GET("", listConceptsHandler)
		group.POST("", createConceptHandler)
		group.PUT("/:id", updateConceptHandler)
		group.DELETE("/:id", deactivateConceptHandler)
		group.POST("/suggest", suggestConceptsHandler)
		group.POST("/:id/confirm", confirmConceptHandler)
		group.POST("/bootstrap", bootstrapConceptsHandler)
	}
}

type listConceptsQuery struct {
	Category string `form:"category"`
	All      bool   `form:"all"`
}

func listConceptsHandler(c *gin.Context) {
	var q listConceptsQuery
	if err := c.ShouldBindQuery(&q); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid query params"})
		return
	}
	if q.Category == "" {
		q.Category = "event"
	}

	var concepts []models.BoardConcept
	var err error

	if q.All {
		concepts, err = ListConcepts(q.Category)
	} else {
		concepts, err = ListActiveConcepts(q.Category)
	}

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	if concepts == nil {
		concepts = []models.BoardConcept{}
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": concepts})
}

type createConceptRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Category    string `json:"category"`
}

func createConceptHandler(c *gin.Context) {
	var req createConceptRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid request body"})
		return
	}

	if req.Name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "name is required"})
		return
	}
	if req.Category == "" {
		req.Category = "event"
	}

	concept, err := CreateConcept(req.Name, req.Description, req.Category)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if embErr := GenerateConceptEmbedding(ctx, concept); embErr != nil {
		logging.Warnf("concept: failed to generate embedding for concept %d: %v", concept.ID, embErr)
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": concept})
}

type updateConceptRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

func updateConceptHandler(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid id"})
		return
	}

	var req updateConceptRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid request body"})
		return
	}

	if req.Name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "name is required"})
		return
	}

	concept, err := UpdateConcept(uint(id), req.Name, req.Description)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if embErr := GenerateConceptEmbedding(ctx, concept); embErr != nil {
		logging.Warnf("concept: failed to regenerate embedding for concept %d: %v", concept.ID, embErr)
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": concept})
}

func deactivateConceptHandler(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid id"})
		return
	}

	if err := DeactivateConcept(uint(id)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"deactivated": true}})
}

func confirmConceptHandler(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid id"})
		return
	}

	if err := ConfirmConcept(uint(id)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"confirmed": true}})
}

type bootstrapRequest struct {
	Category string `json:"category"`
}

func bootstrapConceptsHandler(c *gin.Context) {
	var req bootstrapRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid request body"})
		return
	}
	if req.Category == "" {
		req.Category = "event"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	concepts, err := BootstrapConcepts(ctx, req.Category)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	if concepts == nil {
		concepts = []models.BoardConcept{}
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": concepts})
}

type suggestRequest struct {
	Category string `json:"category"`
}

func suggestConceptsHandler(c *gin.Context) {
	var req suggestRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid request body"})
		return
	}
	if req.Category == "" {
		req.Category = "event"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	suggestions, err := SuggestConcepts(ctx, req.Category)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	if suggestions == nil {
		suggestions = []Suggestion{}
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": suggestions})
}
