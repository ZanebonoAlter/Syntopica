package tagging

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"my-robot-backend/internal/domain/models"
	"my-robot-backend/internal/platform/airouter"
	"my-robot-backend/internal/platform/database"
	"my-robot-backend/internal/platform/jsonutil"
)

var semanticBoardLabelEmbedder auxiliaryLabelEmbedder = defaultAuxiliaryLabelEmbedder
var semanticBoardUpgradeLLMFactory = newSemanticBoardUpgradeLLM

type semanticBoardHandler struct {
	db        *gorm.DB
	auxiliary *AuxiliaryLabelService
	backfill  *SemanticBoardBackfillService
}

type semanticBoardRequest struct {
	Label           string `json:"label"`
	Description     string `json:"description"`
	DisplayOrder    *int   `json:"display_order"`
	Protected       *bool  `json:"protected"`
	Status          string `json:"status"`
	AuxiliaryLabels []uint `json:"auxiliary_labels"`
}

type suggestedAuxiliaryDTO struct {
	ID         uint     `json:"id"`
	Label      string   `json:"label"`
	Slug       string   `json:"slug"`
	Aliases    []string `json:"aliases"`
	RefCount   int      `json:"ref_count"`
	Similarity float64  `json:"similarity"`
}

type suggestAuxiliariesResponse struct {
	Items    []suggestedAuxiliaryDTO `json:"items"`
	Total    int                     `json:"total"`
	Page     int                     `json:"page"`
	PageSize int                     `json:"page_size"`
}

type addCompositionRequest struct {
	AuxiliaryLabelID uint `json:"auxiliary_label_id"`
}

type semanticBoardDTO struct {
	ID           uint     `json:"id"`
	Label        string   `json:"label"`
	Slug         string   `json:"slug"`
	Aliases      []string `json:"aliases"`
	RefCount     int      `json:"ref_count"`
	TagCount     int64    `json:"tag_count"`
	Description  string   `json:"description"`
	DisplayOrder int      `json:"display_order"`
	Source       string   `json:"source"`
	Status       string   `json:"status"`
	Protected    bool     `json:"protected"`
	CreatedAt    any      `json:"created_at"`
	UpdatedAt    any      `json:"updated_at"`
}

type semanticBoardAuxiliaryDTO struct {
	ID           uint     `json:"id"`
	Label        string   `json:"label"`
	Slug         string   `json:"slug"`
	Aliases      []string `json:"aliases"`
	RefCount     int      `json:"ref_count"`
	Description  string   `json:"description"`
	DisplayOrder int      `json:"display_order"`
	Source       string   `json:"source"`
	Status       string   `json:"status"`
	Protected    bool     `json:"protected"`
}

type mergeAuxiliaryAliasRequest struct {
	SourceID uint `json:"source_id"`
	TargetID uint `json:"target_id"`
}

type confirmSemanticBoardUpgradeHTTPRequest struct {
	Decision          SemanticBoardUpgradeDecision `json:"decision"`
	BoardLabel        string                       `json:"board_label"`
	Description       string                       `json:"description"`
	AuxiliaryLabelIDs []uint                       `json:"auxiliary_label_ids"`
	TargetBoardID     *uint                        `json:"target_board_id"`
}

type semanticBoardUpgradeSuggestionDTO struct {
	Decision          SemanticBoardUpgradeDecision `json:"decision"`
	BoardLabel        string                       `json:"board_label"`
	Description       string                       `json:"description"`
	AuxiliaryLabelIDs []uint                       `json:"auxiliary_label_ids"`
	TargetBoardID     *uint                        `json:"target_board_id,omitempty"`
	Reason            string                       `json:"reason"`
}

type semanticBoardUpgradeCandidateDTO struct {
	ID       uint   `json:"id"`
	Label    string `json:"label"`
	Slug     string `json:"slug"`
	RefCount int    `json:"ref_count"`
}

type semanticBoardUpgradeClusterDTO struct {
	Candidates                   []semanticBoardUpgradeCandidateDTO `json:"candidates"`
	ExistingBoardID              *uint                              `json:"existing_board_id,omitempty"`
	ExistingBoardLabel           string                             `json:"existing_board_label"`
	ExistingBoardDescription     string                             `json:"existing_board_description"`
	ExistingBoardAuxiliaryLabels []string                           `json:"existing_board_auxiliary_labels"`
}

type airouterSemanticBoardUpgradeLLM struct{}

func RegisterSemanticBoardRoutes(rg *gin.RouterGroup) {
	handler := &semanticBoardHandler{
		db:        database.DB,
		auxiliary: NewAuxiliaryLabelService(database.DB, nil),
		backfill:  NewSemanticBoardBackfillService(database.DB),
	}

	boards := rg.Group("/semantic-boards")
	{
		boards.GET("/upgrade-candidates", handler.getUpgradeCandidates)
		boards.POST("/upgrade-suggest", handler.suggestUpgrades)
		boards.POST("/upgrade-execute", handler.executeUpgrade)
		boards.POST("/backfill", handler.enqueueBackfill)
		boards.GET("/backfill/:id", handler.getBackfillJob)
		boards.GET("/matching-config", handler.getMatchingConfig)
		boards.PUT("/matching-config", handler.updateMatchingConfig)

		boards.GET("/suggest-auxiliaries", handler.suggestAuxiliaries)

		boards.GET("", handler.listSemanticBoards)
		boards.POST("", handler.createSemanticBoard)
		boards.GET("/:id", handler.getSemanticBoard)
		boards.PUT("/:id", handler.updateSemanticBoard)
		boards.DELETE("/:id", handler.deleteSemanticBoard)
		boards.GET("/:id/suggest-auxiliaries", handler.suggestAuxiliariesForBoard)
		boards.GET("/:id/composition", handler.getBoardComposition)
		boards.POST("/:id/composition", handler.addBoardComposition)
		boards.DELETE("/:id/composition/:auxiliary_label_id", handler.removeBoardComposition)
	}

	auxiliary := rg.Group("/auxiliary-labels")
	{
		auxiliary.GET("", handler.listAuxiliaryLabels)
		auxiliary.POST("/merge-alias", handler.mergeAuxiliaryAlias)
		auxiliary.POST("/:id/disable", handler.disableAuxiliaryLabel)
	}

	tags := rg.Group("/tags")
	{
		tags.GET("/:id/auxiliary-labels", handler.getTagAuxiliaryLabels)
		tags.GET("/:id/semantic-boards", handler.getTagSemanticBoards)
	}
}

func (h *semanticBoardHandler) listSemanticBoards(c *gin.Context) {
	boards, err := h.loadSemanticBoards(c.Request.Context(), c.Query("search"), c.Query("status"))
	if err != nil {
		respondError(c, http.StatusInternalServerError, err)
		return
	}
	respondOK(c, gin.H{"items": boards, "total": len(boards)})
}

func (h *semanticBoardHandler) getSemanticBoard(c *gin.Context) {
	id, ok := parseUintParam(c, "id")
	if !ok {
		return
	}
	var label models.SemanticLabel
	if err := h.db.WithContext(c.Request.Context()).Where("id = ? AND label_type = ?", id, "board").First(&label).Error; err != nil {
		respondError(c, http.StatusNotFound, fmt.Errorf("semantic board not found"))
		return
	}
	tagCounts, err := h.loadSemanticBoardTagCounts(c.Request.Context(), []uint{label.ID})
	if err != nil {
		respondError(c, http.StatusInternalServerError, err)
		return
	}
	respondOK(c, semanticBoardToDTO(label, tagCounts[label.ID]))
}

func (h *semanticBoardHandler) createSemanticBoard(c *gin.Context) {
	var req semanticBoardRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, fmt.Errorf("invalid request body"))
		return
	}
	label := strings.TrimSpace(req.Label)
	if label == "" {
		respondError(c, http.StatusBadRequest, fmt.Errorf("label is required"))
		return
	}
	pgVector, _, err := semanticBoardLabelEmbedder(c.Request.Context(), label)
	if err != nil {
		respondError(c, http.StatusInternalServerError, err)
		return
	}
	protected := true
	if req.Protected != nil {
		protected = *req.Protected
	}
	board := models.SemanticLabel{
		Label:        label,
		Slug:         uniqueSemanticLabelSlug(h.db.WithContext(c.Request.Context()), Slugify(label)),
		Embedding:    &pgVector,
		LabelType:    "board",
		Description:  strings.TrimSpace(req.Description),
		Source:       "manual",
		Status:       "active",
		Protected:    protected,
		DisplayOrder: intValue(req.DisplayOrder),
	}
	if err := h.db.WithContext(c.Request.Context()).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&board).Error; err != nil {
			return err
		}
		return insertBoardComposition(tx, board.ID, req.AuxiliaryLabels)
	}); err != nil {
		respondError(c, http.StatusBadRequest, err)
		return
	}
	respondOK(c, gin.H{"id": board.ID})
}

func (h *semanticBoardHandler) updateSemanticBoard(c *gin.Context) {
	id, ok := parseUintParam(c, "id")
	if !ok {
		return
	}
	var req semanticBoardRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, fmt.Errorf("invalid request body"))
		return
	}
	var board models.SemanticLabel
	if err := h.db.WithContext(c.Request.Context()).Where("id = ? AND label_type = ?", id, "board").First(&board).Error; err != nil {
		respondError(c, http.StatusNotFound, fmt.Errorf("semantic board not found"))
		return
	}
	if label := strings.TrimSpace(req.Label); label != "" && label != board.Label {
		pgVector, _, err := semanticBoardLabelEmbedder(c.Request.Context(), label)
		if err != nil {
			respondError(c, http.StatusInternalServerError, err)
			return
		}
		board.Label = label
		board.Slug = uniqueSemanticLabelSlug(h.db.WithContext(c.Request.Context()).Where("id <> ?", board.ID), Slugify(label))
		board.Embedding = &pgVector
	}
	board.Description = strings.TrimSpace(req.Description)
	if req.DisplayOrder != nil {
		board.DisplayOrder = *req.DisplayOrder
	}
	if req.Protected != nil {
		board.Protected = *req.Protected
	}
	if req.Status == "active" || req.Status == "disabled" {
		board.Status = req.Status
	}
	if err := h.db.WithContext(c.Request.Context()).Save(&board).Error; err != nil {
		respondError(c, http.StatusBadRequest, err)
		return
	}
	respondOK(c, gin.H{"id": board.ID})
}

func (h *semanticBoardHandler) deleteSemanticBoard(c *gin.Context) {
	id, ok := parseUintParam(c, "id")
	if !ok {
		return
	}
	result := h.db.WithContext(c.Request.Context()).Model(&models.SemanticLabel{}).Where("id = ? AND label_type = ?", id, "board").Update("status", "disabled")
	if result.Error != nil {
		respondError(c, http.StatusInternalServerError, result.Error)
		return
	}
	if result.RowsAffected == 0 {
		respondError(c, http.StatusNotFound, fmt.Errorf("semantic board not found"))
		return
	}
	respondOK(c, gin.H{"id": id})
}

func (h *semanticBoardHandler) getBoardComposition(c *gin.Context) {
	boardID, ok := parseUintParam(c, "id")
	if !ok {
		return
	}
	var rows []models.SemanticLabel
	if err := h.db.WithContext(c.Request.Context()).Model(&models.SemanticLabel{}).
		Joins("JOIN board_composition ON board_composition.auxiliary_label_id = semantic_labels.id").
		Where("board_composition.board_id = ? AND semantic_labels.label_type = ?", boardID, "auxiliary").
		Order("semantic_labels.ref_count DESC, semantic_labels.id ASC").
		Find(&rows).Error; err != nil {
		respondError(c, http.StatusInternalServerError, err)
		return
	}
	items := make([]semanticBoardAuxiliaryDTO, 0, len(rows))
	for _, row := range rows {
		items = append(items, auxiliaryToDTO(row))
	}
	respondOK(c, gin.H{"items": items, "total": len(items)})
}

func (h *semanticBoardHandler) removeBoardComposition(c *gin.Context) {
	boardID, ok := parseUintParam(c, "id")
	if !ok {
		return
	}
	auxiliaryID, ok := parseUintParam(c, "auxiliary_label_id")
	if !ok {
		return
	}
	if err := h.auxiliary.RemoveBoardComposition(c.Request.Context(), boardID, auxiliaryID); err != nil {
		respondError(c, http.StatusBadRequest, err)
		return
	}
	respondOK(c, gin.H{"board_id": boardID, "auxiliary_label_id": auxiliaryID})
}

func (h *semanticBoardHandler) listAuxiliaryLabels(c *gin.Context) {
	query := h.db.WithContext(c.Request.Context()).Where("label_type = ?", "auxiliary")
	if status := strings.TrimSpace(c.Query("status")); status != "" {
		query = query.Where("status = ?", status)
	}
	if search := strings.TrimSpace(c.Query("search")); search != "" {
		query = query.Where("LOWER(label) LIKE ? OR LOWER(slug) LIKE ?", "%"+strings.ToLower(search)+"%", "%"+strings.ToLower(Slugify(search))+"%")
	}
	var labels []models.SemanticLabel
	if err := query.Order("ref_count DESC, id ASC").Find(&labels).Error; err != nil {
		respondError(c, http.StatusInternalServerError, err)
		return
	}
	items := make([]semanticBoardAuxiliaryDTO, 0, len(labels))
	for _, label := range labels {
		items = append(items, auxiliaryToDTO(label))
	}
	respondOK(c, gin.H{"items": items, "total": len(items)})
}

func (h *semanticBoardHandler) disableAuxiliaryLabel(c *gin.Context) {
	id, ok := parseUintParam(c, "id")
	if !ok {
		return
	}
	if err := h.auxiliary.DisableAuxiliaryLabel(c.Request.Context(), id); err != nil {
		respondError(c, http.StatusBadRequest, err)
		return
	}
	respondOK(c, gin.H{"id": id})
}

func (h *semanticBoardHandler) mergeAuxiliaryAlias(c *gin.Context) {
	var req mergeAuxiliaryAliasRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, fmt.Errorf("invalid request body"))
		return
	}
	if err := h.auxiliary.MergeAuxiliaryLabelAlias(c.Request.Context(), req.SourceID, req.TargetID); err != nil {
		respondError(c, http.StatusBadRequest, err)
		return
	}
	respondOK(c, gin.H{"source_id": req.SourceID, "target_id": req.TargetID})
}

func (h *semanticBoardHandler) getUpgradeCandidates(c *gin.Context) {
	service := NewSemanticBoardUpgradeService(h.db, nil)
	config := service.loadUpgradeConfig(c.Request.Context())
	candidates, err := service.collectCandidates(c.Request.Context(), config)
	if err != nil {
		respondError(c, http.StatusInternalServerError, err)
		return
	}
	clusters, err := service.clusterCandidates(c.Request.Context(), candidates, config)
	if err != nil {
		respondError(c, http.StatusInternalServerError, err)
		return
	}
	respondOK(c, gin.H{"candidates": upgradeCandidatesToDTO(candidates), "clusters": upgradeClustersToDTO(clusters), "config": semanticBoardUpgradeConfigToMap(config)})
}

func (h *semanticBoardHandler) suggestUpgrades(c *gin.Context) {
	service := NewSemanticBoardUpgradeService(h.db, semanticBoardUpgradeLLMFactory())
	suggestions, err := service.GenerateSuggestions(c.Request.Context())
	if err != nil {
		respondError(c, http.StatusBadRequest, err)
		return
	}
	respondOK(c, gin.H{"suggestions": suggestionsToDTO(suggestions)})
}

func (h *semanticBoardHandler) executeUpgrade(c *gin.Context) {
	var req confirmSemanticBoardUpgradeHTTPRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, fmt.Errorf("invalid request body"))
		return
	}
	result, err := NewSemanticBoardUpgradeService(h.db, nil).ConfirmSuggestion(c.Request.Context(), ConfirmSemanticBoardUpgradeRequest{
		Decision:          req.Decision,
		BoardLabel:        req.BoardLabel,
		Description:       req.Description,
		AuxiliaryLabelIDs: req.AuxiliaryLabelIDs,
		TargetBoardID:     req.TargetBoardID,
	})
	if err != nil {
		respondError(c, http.StatusBadRequest, err)
		return
	}
	respondOK(c, gin.H{"semantic_board_id": result.SemanticBoardID, "auxiliary_label_ids": result.AuxiliaryLabelIDs})
}

func (h *semanticBoardHandler) enqueueBackfill(c *gin.Context) {
	var req SemanticBoardBackfillRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, fmt.Errorf("invalid request body"))
		return
	}
	job, err := h.backfill.Enqueue(c.Request.Context(), req)
	if err != nil {
		respondError(c, http.StatusBadRequest, err)
		return
	}
	respondOK(c, job)
}

func (h *semanticBoardHandler) getBackfillJob(c *gin.Context) {
	jobID := strings.TrimSpace(c.Param("id"))
	job, ok := h.backfill.GetJob(jobID)
	if !ok {
		respondError(c, http.StatusNotFound, fmt.Errorf("backfill job not found"))
		return
	}
	respondOK(c, job)
}

func (h *semanticBoardHandler) getMatchingConfig(c *gin.Context) {
	respondOK(c, semanticBoardMatchConfigToMap(NewSemanticBoardMatchingService(h.db).loadConfig(c.Request.Context())))
}

func (h *semanticBoardHandler) updateMatchingConfig(c *gin.Context) {
	var body map[string]any
	if err := c.ShouldBindJSON(&body); err != nil {
		respondError(c, http.StatusBadRequest, fmt.Errorf("invalid request body"))
		return
	}
	for key, raw := range body {
		if !isSemanticBoardMatchConfigKey(key) {
			respondError(c, http.StatusBadRequest, fmt.Errorf("unsupported config key %q", key))
			return
		}
		value := strings.TrimSpace(fmt.Sprint(raw))
		if value == "" {
			respondError(c, http.StatusBadRequest, fmt.Errorf("config value for %s is required", key))
			return
		}
		if err := validateSemanticBoardMatchConfigValue(key, value); err != nil {
			respondError(c, http.StatusBadRequest, err)
			return
		}
		setting := models.AISettings{Key: key, Value: value, Description: "SemanticBoard matching config"}
		if err := h.db.WithContext(c.Request.Context()).Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "key"}}, DoUpdates: clause.AssignmentColumns([]string{"value", "description", "updated_at"})}).Create(&setting).Error; err != nil {
			respondError(c, http.StatusInternalServerError, err)
			return
		}
	}
	respondOK(c, semanticBoardMatchConfigToMap(NewSemanticBoardMatchingService(h.db).loadConfig(c.Request.Context())))
}

func (h *semanticBoardHandler) getTagAuxiliaryLabels(c *gin.Context) {
	tagID, ok := parseUintParam(c, "id")
	if !ok {
		return
	}
	var labels []models.SemanticLabel
	if err := h.db.WithContext(c.Request.Context()).Model(&models.SemanticLabel{}).
		Joins("JOIN topic_tag_semantic_labels ON topic_tag_semantic_labels.semantic_label_id = semantic_labels.id").
		Where("topic_tag_semantic_labels.topic_tag_id = ? AND semantic_labels.label_type = ?", tagID, "auxiliary").
		Order("semantic_labels.ref_count DESC, semantic_labels.id ASC").
		Find(&labels).Error; err != nil {
		respondError(c, http.StatusInternalServerError, err)
		return
	}
	items := make([]semanticBoardAuxiliaryDTO, 0, len(labels))
	for _, label := range labels {
		items = append(items, auxiliaryToDTO(label))
	}
	respondOK(c, gin.H{"items": items, "total": len(items)})
}

func (h *semanticBoardHandler) getTagSemanticBoards(c *gin.Context) {
	tagID, ok := parseUintParam(c, "id")
	if !ok {
		return
	}
	type row struct {
		models.SemanticLabel
		Score       float64
		MatchReason string
	}
	var rows []row
	if err := h.db.WithContext(c.Request.Context()).Table("semantic_labels").
		Select("semantic_labels.*, topic_tag_board_labels.score, topic_tag_board_labels.match_reason").
		Joins("JOIN topic_tag_board_labels ON topic_tag_board_labels.semantic_board_id = semantic_labels.id").
		Where("topic_tag_board_labels.topic_tag_id = ? AND semantic_labels.label_type = ? AND semantic_labels.status = ?", tagID, "board", "active").
		Order("topic_tag_board_labels.score DESC, semantic_labels.id ASC").
		Scan(&rows).Error; err != nil {
		respondError(c, http.StatusInternalServerError, err)
		return
	}
	items := make([]gin.H, 0, len(rows))
	for _, row := range rows {
		items = append(items, gin.H{"board": semanticBoardToDTO(row.SemanticLabel, 0), "score": row.Score, "match_reason": row.MatchReason})
	}
	respondOK(c, gin.H{"items": items, "total": len(items)})
}

func (h *semanticBoardHandler) loadSemanticBoards(ctx context.Context, search string, status string) ([]semanticBoardDTO, error) {
	query := h.db.WithContext(ctx).Where("label_type = ?", "board")
	if strings.TrimSpace(status) != "" {
		query = query.Where("status = ?", strings.TrimSpace(status))
	} else {
		query = query.Where("status = ?", "active")
	}
	if search = strings.TrimSpace(search); search != "" {
		query = query.Where("LOWER(label) LIKE ? OR LOWER(slug) LIKE ?", "%"+strings.ToLower(search)+"%", "%"+strings.ToLower(Slugify(search))+"%")
	}
	var labels []models.SemanticLabel
	if err := query.Order("display_order ASC, id ASC").Find(&labels).Error; err != nil {
		return nil, err
	}
	ids := make([]uint, 0, len(labels))
	for _, label := range labels {
		ids = append(ids, label.ID)
	}
	tagCounts, err := h.loadSemanticBoardTagCounts(ctx, ids)
	if err != nil {
		return nil, err
	}
	items := make([]semanticBoardDTO, 0, len(labels))
	for _, label := range labels {
		items = append(items, semanticBoardToDTO(label, tagCounts[label.ID]))
	}
	return items, nil
}

func (h *semanticBoardHandler) loadSemanticBoardTagCounts(ctx context.Context, boardIDs []uint) (map[uint]int64, error) {
	counts := map[uint]int64{}
	if len(boardIDs) == 0 {
		return counts, nil
	}
	var rows []struct {
		SemanticBoardID uint
		Count           int64
	}
	if err := h.db.WithContext(ctx).Model(&models.TopicTagBoardLabel{}).
		Select("semantic_board_id, COUNT(*) AS count").
		Where("semantic_board_id IN ?", boardIDs).
		Group("semantic_board_id").
		Scan(&rows).Error; err != nil {
		return nil, err
	}
	for _, row := range rows {
		counts[row.SemanticBoardID] = row.Count
	}
	return counts, nil
}

func (airouterSemanticBoardUpgradeLLM) SuggestSemanticBoardUpgrades(ctx context.Context, prompt string) ([]SemanticBoardUpgradeSuggestion, error) {
	result, err := airouter.NewRouter().Chat(ctx, airouter.ChatRequest{
		Capability: airouter.CapabilityTopicTagging,
		Messages: []airouter.Message{
			{Role: "system", Content: "Return JSON only in this shape: {\"suggestions\":[{\"decision\":\"create_new|merge_into_existing|skip\",\"board_label\":\"\",\"description\":\"\",\"auxiliary_label_ids\":[1],\"target_board_id\":1,\"reason\":\"\"}]}"},
			{Role: "user", Content: prompt},
		},
		JSONMode: true,
		Metadata: map[string]any{"operation": "semantic_board_upgrade_suggest"},
	})
	if err != nil {
		return nil, err
	}
	var parsed struct {
		Suggestions []struct {
			Decision          SemanticBoardUpgradeDecision `json:"decision"`
			BoardLabel        string                       `json:"board_label"`
			Description       string                       `json:"description"`
			AuxiliaryLabelIDs []uint                       `json:"auxiliary_label_ids"`
			TargetBoardID     *uint                        `json:"target_board_id"`
			Reason            string                       `json:"reason"`
		} `json:"suggestions"`
	}
	if err := json.Unmarshal([]byte(jsonutil.SanitizeLLMJSON(result.Content)), &parsed); err != nil {
		return nil, err
	}
	suggestions := make([]SemanticBoardUpgradeSuggestion, 0, len(parsed.Suggestions))
	for _, raw := range parsed.Suggestions {
		suggestions = append(suggestions, SemanticBoardUpgradeSuggestion{Decision: raw.Decision, BoardLabel: raw.BoardLabel, Description: raw.Description, AuxiliaryLabelIDs: raw.AuxiliaryLabelIDs, TargetBoardID: raw.TargetBoardID, Reason: raw.Reason})
	}
	return suggestions, nil
}

func newSemanticBoardUpgradeLLM() semanticBoardUpgradeLLM {
	return airouterSemanticBoardUpgradeLLM{}
}

func insertBoardComposition(tx *gorm.DB, boardID uint, auxiliaryIDs []uint) error {
	ids := uniqueUintSlice(auxiliaryIDs)
	if len(ids) == 0 {
		return nil
	}
	if err := validateActiveAuxiliaryLabels(tx, ids); err != nil {
		return err
	}
	rows := make([]models.BoardComposition, 0, len(ids))
	for _, id := range ids {
		rows = append(rows, models.BoardComposition{BoardID: boardID, AuxiliaryLabelID: id})
	}
	return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&rows).Error
}

func semanticBoardToDTO(label models.SemanticLabel, tagCount int64) semanticBoardDTO {
	return semanticBoardDTO{ID: label.ID, Label: label.Label, Slug: label.Slug, Aliases: label.Aliases, RefCount: label.RefCount, TagCount: tagCount, Description: label.Description, DisplayOrder: label.DisplayOrder, Source: label.Source, Status: label.Status, Protected: label.Protected, CreatedAt: label.CreatedAt, UpdatedAt: label.UpdatedAt}
}

func auxiliaryToDTO(label models.SemanticLabel) semanticBoardAuxiliaryDTO {
	return semanticBoardAuxiliaryDTO{ID: label.ID, Label: label.Label, Slug: label.Slug, Aliases: label.Aliases, RefCount: label.RefCount, Description: label.Description, DisplayOrder: label.DisplayOrder, Source: label.Source, Status: label.Status, Protected: label.Protected}
}

func upgradeCandidatesToDTO(candidates []SemanticBoardUpgradeCandidate) []semanticBoardUpgradeCandidateDTO {
	items := make([]semanticBoardUpgradeCandidateDTO, 0, len(candidates))
	for _, candidate := range candidates {
		items = append(items, semanticBoardUpgradeCandidateDTO{ID: candidate.ID, Label: candidate.Label, Slug: candidate.Slug, RefCount: candidate.RefCount})
	}
	return items
}

func upgradeClustersToDTO(clusters []SemanticBoardUpgradeCluster) []semanticBoardUpgradeClusterDTO {
	items := make([]semanticBoardUpgradeClusterDTO, 0, len(clusters))
	for _, cluster := range clusters {
		items = append(items, semanticBoardUpgradeClusterDTO{Candidates: upgradeCandidatesToDTO(cluster.Candidates), ExistingBoardID: cluster.ExistingBoardID, ExistingBoardLabel: cluster.ExistingBoardLabel, ExistingBoardDescription: cluster.ExistingBoardDescription, ExistingBoardAuxiliaryLabels: cluster.ExistingBoardAuxiliaryLabels})
	}
	return items
}

func suggestionsToDTO(suggestions []SemanticBoardUpgradeSuggestion) []semanticBoardUpgradeSuggestionDTO {
	items := make([]semanticBoardUpgradeSuggestionDTO, 0, len(suggestions))
	for _, suggestion := range suggestions {
		items = append(items, semanticBoardUpgradeSuggestionDTO{Decision: suggestion.Decision, BoardLabel: suggestion.BoardLabel, Description: suggestion.Description, AuxiliaryLabelIDs: suggestion.AuxiliaryLabelIDs, TargetBoardID: suggestion.TargetBoardID, Reason: suggestion.Reason})
	}
	return items
}

func semanticBoardMatchConfigToMap(config SemanticBoardMatchConfig) gin.H {
	return gin.H{
		"semantic_board_match_sim_threshold":      config.SimThreshold,
		"semantic_board_match_direct_hit_rate":    config.DirectHitRate,
		"semantic_board_match_direct_max_sim":     config.DirectMaxSim,
		"semantic_board_match_weight_sim":         config.WeightSim,
		"semantic_board_match_weight_density":     config.WeightDensity,
		"semantic_board_match_weighted_threshold": config.WeightedThreshold,
		"semantic_board_match_max_boards":         config.MaxBoards,
	}
}

func semanticBoardUpgradeConfigToMap(config SemanticBoardUpgradeConfig) gin.H {
	return gin.H{
		"semantic_board_upgrade_ref_count_threshold":        config.RefCountThreshold,
		"semantic_board_upgrade_cluster_distance_threshold": config.ClusterDistanceThreshold,
		"semantic_board_upgrade_cotag_window_days":          config.CoTagWindowDays,
		"semantic_board_upgrade_cotag_top_n":                config.CoTagTopN,
		"semantic_board_upgrade_cotag_dedupe_sim_threshold": config.CoTagDedupeSimThreshold,
		"semantic_board_upgrade_cotag_hard_limit":           config.CoTagHardLimit,
	}
}

func isSemanticBoardMatchConfigKey(key string) bool {
	switch key {
	case "semantic_board_match_sim_threshold", "semantic_board_match_direct_hit_rate", "semantic_board_match_direct_max_sim", "semantic_board_match_weight_sim", "semantic_board_match_weight_density", "semantic_board_match_weighted_threshold", "semantic_board_match_max_boards":
		return true
	default:
		return false
	}
}

func validateSemanticBoardMatchConfigValue(key string, value string) error {
	if key == "semantic_board_match_max_boards" {
		parsed, err := strconv.Atoi(value)
		if err != nil || parsed <= 0 {
			return fmt.Errorf("%s must be a positive integer", key)
		}
		return nil
	}
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil || parsed < 0 || parsed > 1 {
		return fmt.Errorf("%s must be a number between 0 and 1", key)
	}
	return nil
}

func (h *semanticBoardHandler) suggestAuxiliaries(c *gin.Context) {
	label := strings.TrimSpace(c.Query("label"))
	if label == "" {
		respondError(c, http.StatusBadRequest, fmt.Errorf("label is required"))
		return
	}
	description := strings.TrimSpace(c.Query("description"))
	queryText := label
	if description != "" {
		queryText = label + " " + description
	}

	page, pageSize := parsePaginationParams(c)

	_, queryVector, err := semanticBoardLabelEmbedder(c.Request.Context(), queryText)
	if err != nil {
		respondError(c, http.StatusInternalServerError, err)
		return
	}

	results, err := h.computeAuxiliarySuggestions(c.Request.Context(), queryVector, c.Query("search"), 0, page, pageSize)
	if err != nil {
		respondError(c, http.StatusInternalServerError, err)
		return
	}

	// Apply optional exclude_board_id filter
	if excludeStr := strings.TrimSpace(c.Query("exclude_board_id")); excludeStr != "" {
		excludeID, parseErr := strconv.ParseUint(excludeStr, 10, 64)
		if parseErr == nil && excludeID > 0 {
			results = h.filterExcludedBoardComposition(c.Request.Context(), results, uint(excludeID))
		}
	}

	respondOK(c, results)
}

func (h *semanticBoardHandler) suggestAuxiliariesForBoard(c *gin.Context) {
	boardID, ok := parseUintParam(c, "id")
	if !ok {
		return
	}

	var board models.SemanticLabel
	if err := h.db.WithContext(c.Request.Context()).Where("id = ? AND label_type = ?", boardID, "board").First(&board).Error; err != nil {
		respondError(c, http.StatusNotFound, fmt.Errorf("semantic board not found"))
		return
	}

	queryText := board.Label
	if board.Description != "" {
		queryText = board.Label + " " + board.Description
	}

	page, pageSize := parsePaginationParams(c)

	_, queryVector, err := semanticBoardLabelEmbedder(c.Request.Context(), queryText)
	if err != nil {
		respondError(c, http.StatusInternalServerError, err)
		return
	}

	results, err := h.computeAuxiliarySuggestions(c.Request.Context(), queryVector, c.Query("search"), boardID, page, pageSize)
	if err != nil {
		respondError(c, http.StatusInternalServerError, err)
		return
	}

	respondOK(c, results)
}

func (h *semanticBoardHandler) addBoardComposition(c *gin.Context) {
	boardID, ok := parseUintParam(c, "id")
	if !ok {
		return
	}
	var req addCompositionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, fmt.Errorf("invalid request body"))
		return
	}
	if req.AuxiliaryLabelID == 0 {
		respondError(c, http.StatusBadRequest, fmt.Errorf("auxiliary_label_id is required"))
		return
	}

	var board models.SemanticLabel
	if err := h.db.WithContext(c.Request.Context()).Where("id = ? AND label_type = ?", boardID, "board").First(&board).Error; err != nil {
		respondError(c, http.StatusNotFound, fmt.Errorf("semantic board not found"))
		return
	}

	var auxiliary models.SemanticLabel
	if err := h.db.WithContext(c.Request.Context()).Where("id = ? AND label_type = ? AND status = ?", req.AuxiliaryLabelID, "auxiliary", "active").First(&auxiliary).Error; err != nil {
		respondError(c, http.StatusBadRequest, fmt.Errorf("active auxiliary label not found"))
		return
	}

	row := models.BoardComposition{BoardID: boardID, AuxiliaryLabelID: req.AuxiliaryLabelID}
	if err := h.db.WithContext(c.Request.Context()).Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error; err != nil {
		respondError(c, http.StatusInternalServerError, err)
		return
	}
	respondOK(c, gin.H{"board_id": boardID, "auxiliary_label_id": req.AuxiliaryLabelID})
}

type scoredAuxiliary struct {
	label     models.SemanticLabel
	similarity float64
}

func (h *semanticBoardHandler) computeAuxiliarySuggestions(ctx context.Context, queryVector []float64, search string, excludeBoardID uint, page, pageSize int) (*suggestAuxiliariesResponse, error) {
	query := h.db.WithContext(ctx).Where("label_type = ? AND status = ?", "auxiliary", "active")
	if s := strings.TrimSpace(search); s != "" {
		query = query.Where("LOWER(label) LIKE ? OR LOWER(slug) LIKE ?", "%"+strings.ToLower(s)+"%", "%"+strings.ToLower(Slugify(s))+"%")
	}

	// Exclude labels already in the board's composition
	if excludeBoardID > 0 {
		query = query.Where("id NOT IN (?)", h.db.Table("board_composition").Select("auxiliary_label_id").Where("board_id = ?", excludeBoardID))
	}

	var labels []models.SemanticLabel
	if err := query.Find(&labels).Error; err != nil {
		return nil, err
	}

	scored := make([]scoredAuxiliary, 0, len(labels))
	for _, label := range labels {
		if label.Embedding == nil || *label.Embedding == "" {
			continue
		}
		vec, err := parsePgVector(*label.Embedding)
		if err != nil {
			continue
		}
		sim, err := airouter.CosineSimilarity(queryVector, vec)
		if err != nil {
			continue
		}
		scored = append(scored, scoredAuxiliary{label: label, similarity: sim})
	}

	// Sort by similarity descending
	sort.Slice(scored, func(i, j int) bool {
		if scored[i].similarity == scored[j].similarity {
			return scored[i].label.ID < scored[j].label.ID
		}
		return scored[i].similarity > scored[j].similarity
	})

	total := len(scored)
	start := (page - 1) * pageSize
	if start > total {
		start = total
	}
	end := start + pageSize
	if end > total {
		end = total
	}

	items := make([]suggestedAuxiliaryDTO, 0, end-start)
	for i := start; i < end; i++ {
		s := scored[i]
		items = append(items, suggestedAuxiliaryDTO{
			ID:         s.label.ID,
			Label:      s.label.Label,
			Slug:       s.label.Slug,
			Aliases:    s.label.Aliases,
			RefCount:   s.label.RefCount,
			Similarity: roundSimilarity(s.similarity),
		})
	}

	return &suggestAuxiliariesResponse{Items: items, Total: total, Page: page, PageSize: pageSize}, nil
}

func (h *semanticBoardHandler) filterExcludedBoardComposition(ctx context.Context, resp *suggestAuxiliariesResponse, boardID uint) *suggestAuxiliariesResponse {
	var ids []uint
	for _, item := range resp.Items {
		ids = append(ids, item.ID)
	}
	if len(ids) == 0 {
		return resp
	}
	var excluded []uint
	h.db.WithContext(ctx).Model(&models.BoardComposition{}).
		Where("board_id = ? AND auxiliary_label_id IN ?", boardID, ids).
		Pluck("auxiliary_label_id", &excluded)
	excludedSet := make(map[uint]struct{}, len(excluded))
	for _, id := range excluded {
		excludedSet[id] = struct{}{}
	}
	filtered := make([]suggestedAuxiliaryDTO, 0, len(resp.Items))
	for _, item := range resp.Items {
		if _, ok := excludedSet[item.ID]; !ok {
			filtered = append(filtered, item)
		}
	}
	resp.Items = filtered
	resp.Total = len(filtered)
	return resp
}

func parsePaginationParams(c *gin.Context) (page, pageSize int) {
	page = 1
	pageSize = 20
	if p := c.Query("page"); p != "" {
		if v, err := strconv.Atoi(p); err == nil && v > 0 {
			page = v
		}
	}
	if ps := c.Query("page_size"); ps != "" {
		if v, err := strconv.Atoi(ps); err == nil && v > 0 {
			pageSize = v
		}
	}
	if pageSize > 100 {
		pageSize = 100
	}
	return
}

func roundSimilarity(v float64) float64 {
	return float64(int(v*10000)) / 10000
}

func intValue(value *int) int {
	if value == nil {
		return 0
	}
	return *value
}

func parseUintParam(c *gin.Context, name string) (uint, bool) {
	parsed, err := strconv.ParseUint(c.Param(name), 10, 64)
	if err != nil || parsed == 0 {
		respondError(c, http.StatusBadRequest, fmt.Errorf("invalid %s", name))
		return 0, false
	}
	return uint(parsed), true
}

func respondOK(c *gin.Context, data any) {
	c.JSON(http.StatusOK, gin.H{"success": true, "data": data})
}

func respondError(c *gin.Context, status int, err error) {
	c.JSON(status, gin.H{"success": false, "error": err.Error()})
}
