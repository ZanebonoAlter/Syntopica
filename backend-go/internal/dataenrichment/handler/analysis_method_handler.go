package handler

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"syntopica-backend/internal/dataenrichment/repository"
)

type analysisMethodRequest struct {
	Name          *string                                 `json:"name"`
	Title         *string                                 `json:"title"`
	Summary       *string                                 `json:"summary"`
	SelectionMeta *repository.AnalysisMethodSelectionMeta `json:"selection_meta"`
	Content       *string                                 `json:"content"`
	Enabled       *bool                                   `json:"enabled"`
}

type analysisMethodEnableRequest struct {
	Enabled *bool `json:"enabled"`
}

func (h *EnrichmentHandler) listAnalysisMethods(c *gin.Context) {
	list, err := h.repo.ListAnalysisMethods(c.Request.Context())
	if err != nil {
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}
	respondOK(c, list)
}

func (h *EnrichmentHandler) createAnalysisMethod(c *gin.Context) {
	var req analysisMethodRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "invalid body: "+err.Error())
		return
	}
	name := deref(req.Name)
	content := derefContent(req.Content)
	if name == "" || content == "" {
		respondError(c, http.StatusBadRequest, "name and content are required")
		return
	}
	method := &repository.AnalysisMethod{
		Name: name, Title: deref(req.Title), Summary: deref(req.Summary), Content: content,
		Enabled: req.Enabled != nil && *req.Enabled,
	}
	if req.SelectionMeta != nil {
		method.SelectionMeta = *req.SelectionMeta
	}
	if err := h.repo.CreateAnalysisMethod(c.Request.Context(), method); err != nil {
		if isUniqueViolation(err) {
			respondError(c, http.StatusConflict, "analysis method name already exists")
			return
		}
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}
	respondOK(c, method)
}

func (h *EnrichmentHandler) getAnalysisMethod(c *gin.Context) {
	id, ok := parseIDParam(c, "id")
	if !ok {
		return
	}
	method, err := h.repo.GetAnalysisMethodByID(c.Request.Context(), id)
	if err != nil {
		respondError(c, http.StatusNotFound, err.Error())
		return
	}
	respondOK(c, method)
}

func (h *EnrichmentHandler) updateAnalysisMethod(c *gin.Context) {
	id, ok := parseIDParam(c, "id")
	if !ok {
		return
	}
	var req analysisMethodRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "invalid body: "+err.Error())
		return
	}
	method, err := h.repo.GetAnalysisMethodByID(c.Request.Context(), id)
	if err != nil {
		respondError(c, http.StatusNotFound, err.Error())
		return
	}
	if req.Name != nil {
		method.Name = strings.TrimSpace(*req.Name)
		if method.Name == "" {
			respondError(c, http.StatusBadRequest, "name cannot be empty")
			return
		}
	}
	if req.Title != nil {
		method.Title = strings.TrimSpace(*req.Title)
	}
	if req.Summary != nil {
		method.Summary = strings.TrimSpace(*req.Summary)
	}
	if req.SelectionMeta != nil {
		method.SelectionMeta = *req.SelectionMeta
	}
	if req.Content != nil {
		method.Content = derefContent(req.Content)
		if method.Content == "" {
			respondError(c, http.StatusBadRequest, "content cannot be empty")
			return
		}
	}
	if req.Enabled != nil {
		method.Enabled = *req.Enabled
	}
	if err := h.repo.UpdateAnalysisMethod(c.Request.Context(), method); err != nil {
		if isUniqueViolation(err) {
			respondError(c, http.StatusConflict, "analysis method name already exists")
			return
		}
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}
	respondOK(c, method)
}

func (h *EnrichmentHandler) setAnalysisMethodEnabled(c *gin.Context) {
	id, ok := parseIDParam(c, "id")
	if !ok {
		return
	}
	var req analysisMethodEnableRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.Enabled == nil {
		respondError(c, http.StatusBadRequest, "enabled is required")
		return
	}
	if err := h.repo.SetAnalysisMethodEnabled(c.Request.Context(), id, *req.Enabled); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			respondError(c, http.StatusNotFound, err.Error())
			return
		}
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}
	method, err := h.repo.GetAnalysisMethodByID(c.Request.Context(), id)
	if err != nil {
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}
	respondOK(c, method)
}

func (h *EnrichmentHandler) deleteAnalysisMethod(c *gin.Context) {
	id, ok := parseIDParam(c, "id")
	if !ok {
		return
	}
	if err := h.repo.DeleteAnalysisMethod(c.Request.Context(), id); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			respondError(c, http.StatusNotFound, err.Error())
			return
		}
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}
	respondOK(c, gin.H{"deleted": id})
}

func derefContent(s *string) string {
	if s == nil {
		return ""
	}
	return strings.TrimSpace(*s)
}
