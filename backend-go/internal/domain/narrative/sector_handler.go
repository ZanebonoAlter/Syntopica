package narrative

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"my-robot-backend/internal/domain/concept"
	"my-robot-backend/internal/domain/models"
	"my-robot-backend/internal/domain/tagging"
	"my-robot-backend/internal/platform/database"
	"my-robot-backend/internal/platform/logging"
)

type listSectorsQuery struct {
	Category string `form:"category"`
	All      bool   `form:"all"`
}

func listSectorsHandler(c *gin.Context) {
	var q listSectorsQuery
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
		concepts, err = concept.ListConcepts(q.Category)
	} else {
		concepts, err = concept.ListActiveConcepts(q.Category)
	}

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	if concepts == nil {
		concepts = []models.BoardConcept{}
	}

	ids := make([]uint, len(concepts))
	for i, c := range concepts {
		ids[i] = c.ID
	}

	countMap := make(map[uint]int)
	if len(ids) > 0 {
		type countRow struct {
			ConceptID uint `gorm:"column:concept_id"`
			Count     int  `gorm:"column:count"`
		}
		var rows []countRow
		if err := database.DB.Model(&models.TopicTag{}).
			Select("concept_id, COUNT(*) as count").
			Where("concept_id IN ? AND status = ?", ids, "active").
			Group("concept_id").
			Find(&rows).Error; err != nil {
			logging.Warnf("sector-list: failed to count tags per concept: %v", err)
		}
		for _, r := range rows {
			countMap[r.ConceptID] = r.Count
		}
	}

	type sectorResponse struct {
		models.BoardConcept
		TagCount int `json:"tag_count"`
	}

	result := make([]sectorResponse, len(concepts))
	for i, bc := range concepts {
		result[i] = sectorResponse{
			BoardConcept: bc,
			TagCount:     countMap[bc.ID],
		}
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": result})
}

type createSectorRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Category    string `json:"category"`
	Source      string `json:"source"`
}

func createSectorHandler(c *gin.Context) {
	var req createSectorRequest
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

	var bc *models.BoardConcept
	var err error

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	if req.Source == "manual" {
		bc, err = tagging.ManualCreateSector(ctx, database.DB, req.Category, req.Name, req.Description)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
			return
		}
	} else {
		bc, err = concept.CreateConcept(req.Name, req.Description, req.Category)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
			return
		}

		source := req.Source
		if source == "" {
			source = "auto"
		}
		if err := database.DB.Model(&models.BoardConcept{}).Where("id = ?", bc.ID).
			Update("source", source).Error; err != nil {
			logging.Warnf("sector-create: set source for concept %d: %v", bc.ID, err)
		}

		if embErr := concept.GenerateConceptEmbedding(ctx, bc); embErr != nil {
			logging.Warnf("sector-create: failed to generate embedding for concept %d: %v", bc.ID, embErr)
		}
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": bc})
}

type updateSectorRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

func updateSectorHandler(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid id"})
		return
	}

	var req updateSectorRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid request body"})
		return
	}

	if req.Name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "name is required"})
		return
	}

	bc, err := concept.UpdateConcept(uint(id), req.Name, req.Description)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if embErr := concept.GenerateConceptEmbedding(ctx, bc); embErr != nil {
		logging.Warnf("sector-update: failed to regenerate embedding for concept %d: %v", bc.ID, embErr)
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": bc})
}

func deleteSectorHandler(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid id"})
		return
	}

	bc, err := concept.GetConceptByID(uint(id))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "error": "concept not found"})
		return
	}

	if bc.Protected {
		confirm := c.Query("confirm")
		if confirm != "true" {
			c.JSON(http.StatusConflict, gin.H{
				"success": false,
				"error":   "protected sector requires confirm=true query parameter",
			})
			return
		}
	}

	if err := concept.DeactivateConcept(uint(id)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	if err := database.DB.Model(&models.TopicTag{}).
		Where("concept_id = ?", uint(id)).
		Update("concept_id", nil).Error; err != nil {
		logging.Warnf("sector-delete: failed to clear concept_id on tags for concept %d: %v", id, err)
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"deactivated": true}})
}

type regenerateSectorsRequest struct {
	Category string `json:"category"`
}

func regenerateSectorsHandler(c *gin.Context) {
	var req regenerateSectorsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid request body"})
		return
	}
	if req.Category == "" {
		req.Category = "event"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	diff, err := tagging.LLMSuggestSectors(ctx, database.DB, req.Category)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	logging.Infof("sector-regenerate: category=%q, diff keep=%d add=%d merge=%d split=%d affected_tags=%d",
		req.Category, len(diff.Keep), len(diff.Add), len(diff.Merge), len(diff.Split), diff.AffectedTagCount)

	c.JSON(http.StatusOK, gin.H{"success": true, "data": diff})
}

type confirmRegenerateSectorsRequest struct {
	Category string             `json:"category"`
	Diff     tagging.SectorDiff `json:"diff"`
}

func confirmRegenerateSectorsHandler(c *gin.Context) {
	var req confirmRegenerateSectorsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid request body"})
		return
	}
	if req.Category == "" {
		req.Category = "event"
	}

	logging.Infof("sector-confirm: category=%q starting execution", req.Category)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	result, err := tagging.LLMExecuteSectorDiff(ctx, database.DB, req.Category, &req.Diff)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	logging.Infof("sector-confirm: category=%q execution completed success=%d failed=%d", req.Category, result.SuccessCount, result.FailedCount)

	c.JSON(http.StatusOK, gin.H{"success": true, "data": result})
}
