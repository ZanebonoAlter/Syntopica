package handler

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/logging"
	"syntopica-backend/internal/tagmanagement/repository"
	"syntopica-backend/internal/tagmanagement/service"
)

type confirmSemanticBoardUpgradeHTTPRequest struct {
	Decision          service.SemanticBoardUpgradeDecision `json:"decision"`
	BoardLabel        string                               `json:"board_label"`
	Description       string                               `json:"description"`
	AuxiliaryLabelIDs []uint                               `json:"auxiliary_label_ids"`
	TargetBoardID     *uint                                `json:"target_board_id"`
	SuggestionID      *uint                                `json:"suggestion_id,omitempty"`
}
type semanticBoardUpgradeSuggestionDTO struct {
	Decision          service.SemanticBoardUpgradeDecision `json:"decision"`
	BoardLabel        string                               `json:"board_label"`
	Description       string                               `json:"description"`
	AuxiliaryLabelIDs []uint                               `json:"auxiliary_label_ids"`
	AuxiliaryLabels   []struct {
		ID    uint   `json:"id"`
		Label string `json:"label"`
	} `json:"auxiliary_labels"`
	TargetBoardID    *uint  `json:"target_board_id,omitempty"`
	TargetBoardLabel string `json:"target_board_label,omitempty"`
	Reason           string `json:"reason"`
}

// suggestUpgrades 生成升级建议（四格矩阵入口，spec: suggestUpgrades API）。
// 参数：direction=create|expand、source=aux|composite、target_board_id（expand
// 必填，指向有效活跃版块；create 必须不携带）、days（可选，仅 create×aux 生效）。
// 旧 mode 参数（discover_new/expand_existing）不再接受。
func (h *semanticBoardHandler) suggestUpgrades(c *gin.Context) {
	req, err := parseUpgradeGenerateRequest(c)
	if err != nil {
		respondError(c, http.StatusBadRequest, err)
		return
	}
	if req.Direction == service.UpgradeDirectionExpand {
		var count int64
		if err := h.db.WithContext(c.Request.Context()).Model(&models.SemanticLabel{}).
			Where("id = ? AND label_type = ? AND status = ?", req.TargetBoardID, "board", "active").
			Count(&count).Error; err != nil {
			respondError(c, http.StatusInternalServerError, err)
			return
		}
		if count == 0 {
			respondError(c, http.StatusBadRequest, fmt.Errorf("target board %d not found or not active", req.TargetBoardID))
			return
		}
	}
	svc := service.NewSemanticBoardUpgradeService(h.db, semanticBoardUpgradeLLMFactory(), nil)
	suggestions, _, err := svc.GenerateSuggestions(c.Request.Context(), req)
	if err != nil {
		respondError(c, http.StatusBadRequest, err)
		return
	}
	respondOK(c, gin.H{"suggestions": h.suggestionsToDTO(c.Request.Context(), suggestions)})
}

// parseUpgradeGenerateRequest 从 query 参数解析四格生成请求（spec 校验场景：
// expand 缺 target / create 带 target / 非法值 / 旧 mode 参数均 400）。
func parseUpgradeGenerateRequest(c *gin.Context) (service.UpgradeGenerateRequest, error) {
	if legacy := strings.TrimSpace(c.Query("mode")); legacy != "" {
		return service.UpgradeGenerateRequest{}, fmt.Errorf("mode parameter is no longer accepted; use direction/source/target_board_id")
	}
	req := service.UpgradeGenerateRequest{
		Direction: strings.TrimSpace(c.Query("direction")),
		Source:    strings.TrimSpace(c.Query("source")),
	}
	if req.Direction != service.UpgradeDirectionCreate && req.Direction != service.UpgradeDirectionExpand {
		return service.UpgradeGenerateRequest{}, fmt.Errorf("direction is required (create|expand)")
	}
	if req.Source != service.UpgradeSourceAux && req.Source != service.UpgradeSourceComposite {
		return service.UpgradeGenerateRequest{}, fmt.Errorf("source is required (aux|composite)")
	}
	if req.Direction == service.UpgradeDirectionExpand {
		target, err := strconv.ParseUint(strings.TrimSpace(c.Query("target_board_id")), 10, 64)
		if err != nil || target == 0 {
			return service.UpgradeGenerateRequest{}, fmt.Errorf("target_board_id is required for expand direction")
		}
		req.TargetBoardID = uint(target)
	}
	if raw := strings.TrimSpace(c.Query("days")); raw != "" {
		days, err := strconv.Atoi(raw)
		if err != nil || days < 0 {
			return service.UpgradeGenerateRequest{}, fmt.Errorf("days must be a non-negative integer")
		}
		req.Days = days
	}
	if err := req.Validate(); err != nil {
		return service.UpgradeGenerateRequest{}, err
	}
	return req, nil
}
func (h *semanticBoardHandler) executeUpgrade(c *gin.Context) {
	var req confirmSemanticBoardUpgradeHTTPRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, fmt.Errorf("invalid request body"))
		return
	}
	result, err := service.NewSemanticBoardUpgradeService(h.db, nil, semanticBoardLabelEmbedder).ConfirmSuggestion(c.Request.Context(), service.ConfirmSemanticBoardUpgradeRequest(req))
	if err != nil {
		respondError(c, http.StatusBadRequest, err)
		return
	}
	if result.CompositeLabelID != nil {
		respondOK(c, gin.H{"semantic_board_id": result.SemanticBoardID, "auxiliary_label_ids": result.AuxiliaryLabelIDs, "composite_label_id": *result.CompositeLabelID})
		return
	}
	respondOK(c, gin.H{"semantic_board_id": result.SemanticBoardID, "auxiliary_label_ids": result.AuxiliaryLabelIDs})
}

// listUpgradeSuggestions reads persisted suggestions (spec: 建议查询 API 读持久化表).
// status defaults to "pending"; decision empty → default list excludes watch
// (observation pool), decision="watch" → pool only, otherwise exact match.
func (h *semanticBoardHandler) listUpgradeSuggestions(c *gin.Context) {
	status := strings.TrimSpace(c.Query("status"))
	if status == "" {
		status = "pending"
	}
	decision := strings.TrimSpace(c.Query("decision"))
	repo := repository.NewBoardUpgradeSuggestionRepository(h.db)
	rows, err := repo.List(c.Request.Context(), status, decision)
	if err != nil {
		respondError(c, http.StatusInternalServerError, err)
		return
	}
	respondOK(c, gin.H{"suggestions": h.upgradeSuggestionsToRowDTO(c.Request.Context(), rows)})
}

// dismissUpgradeSuggestion marks a pending suggestion dismissed (spec: 建议 dismiss
// 与 confirm 联动). Body is optional; an optional reason is recorded.
func (h *semanticBoardHandler) dismissUpgradeSuggestion(c *gin.Context) {
	id, err := strconv.ParseUint(strings.TrimSpace(c.Param("id")), 10, 64)
	if err != nil || id == 0 {
		respondError(c, http.StatusBadRequest, fmt.Errorf("invalid suggestion id"))
		return
	}
	var req struct {
		Reason string `json:"reason"`
	}
	_ = c.ShouldBindJSON(&req) // body optional; missing/empty body is fine
	repo := repository.NewBoardUpgradeSuggestionRepository(h.db)
	if err := repo.MarkDismissed(c.Request.Context(), uint(id), req.Reason); err != nil {
		respondError(c, http.StatusInternalServerError, err)
		return
	}
	respondOK(c, gin.H{"id": uint(id), "status": "dismissed"})
}

// generateUpgradeSuggestions runs one four-grid generation pass synchronously
// and persists the suggestions (spec: suggestUpgrades API 四格参数——与内存
// suggest 端点共享参数解析与校验).
func (h *semanticBoardHandler) generateUpgradeSuggestions(c *gin.Context) {
	req, err := parseUpgradeGenerateRequest(c)
	if err != nil {
		respondError(c, http.StatusBadRequest, err)
		return
	}
	if req.Direction == service.UpgradeDirectionExpand {
		var count int64
		if err := h.db.WithContext(c.Request.Context()).Model(&models.SemanticLabel{}).
			Where("id = ? AND label_type = ? AND status = ?", req.TargetBoardID, "board", "active").
			Count(&count).Error; err != nil {
			respondError(c, http.StatusInternalServerError, err)
			return
		}
		if count == 0 {
			respondError(c, http.StatusBadRequest, fmt.Errorf("target board %d not found or not active", req.TargetBoardID))
			return
		}
	}
	svc := service.NewSemanticBoardUpgradeService(h.db, semanticBoardUpgradeLLMFactory(), nil)
	inserted, skipped, cooldownBlocked, err := svc.GenerateAndPersist(c.Request.Context(), req)
	if err != nil {
		respondError(c, http.StatusInternalServerError, err)
		return
	}
	respondOK(c, gin.H{
		"inserted":         inserted,
		"skipped":          skipped,
		"cooldown_blocked": cooldownBlocked,
	})
}

// boardUpgradeSuggestionRowDTO is the JSON shape for a persisted suggestion row
// served by GET /upgrade-suggestions. It carries the lifecycle fields the panel
// needs (status/confidence/evidence) plus resolved label names for display.
type boardUpgradeSuggestionRowDTO struct {
	ID                uint           `json:"id"`
	BatchID           string         `json:"batch_id"`
	Mode              string         `json:"mode"`
	Decision          string         `json:"decision"`
	BoardLabel        string         `json:"board_label"`
	Description       string         `json:"description"`
	TargetBoardID     *uint          `json:"target_board_id,omitempty"`
	TargetBoardLabel  string         `json:"target_board_label,omitempty"`
	AuxiliaryLabelIDs []uint         `json:"auxiliary_label_ids"`
	AuxiliaryLabels   []idLabelDTO   `json:"auxiliary_labels"`
	Confidence        string         `json:"confidence"`
	Evidence          map[string]any `json:"evidence,omitempty"`
	Status            string         `json:"status"`
	DismissReason     *string        `json:"dismiss_reason,omitempty"`
	CreatedAt         time.Time      `json:"created_at"`
	ResolvedAt        *time.Time     `json:"resolved_at,omitempty"`
}

type idLabelDTO struct {
	ID     uint   `json:"id"`
	Label  string `json:"label"`
	Status string `json:"status"`
}

// upgradeSuggestionsToRowDTO maps persisted rows to the panel DTO, batch-resolving
// auxiliary-label and target-board names in two queries regardless of row count.
func (h *semanticBoardHandler) upgradeSuggestionsToRowDTO(ctx context.Context, rows []models.BoardUpgradeSuggestion) []boardUpgradeSuggestionRowDTO {
	labelIDSet := make(map[uint]struct{})
	boardIDSet := make(map[uint]struct{})
	for _, r := range rows {
		for _, id := range r.AuxiliaryLabelIDs {
			labelIDSet[id] = struct{}{}
		}
		if r.TargetBoardID != nil {
			boardIDSet[*r.TargetBoardID] = struct{}{}
		}
	}
	labelNames := make(map[uint]string)
	labelStatuses := make(map[uint]string)
	if len(labelIDSet) > 0 {
		ids := make([]uint, 0, len(labelIDSet))
		for id := range labelIDSet {
			ids = append(ids, id)
		}
		var labels []models.SemanticLabel
		if err := h.db.WithContext(ctx).Where("id IN ?", ids).Select("id, label, status").Find(&labels).Error; err == nil {
			for _, l := range labels {
				labelNames[l.ID] = l.Label
				labelStatuses[l.ID] = l.Status
			}
		}
	}
	boardNames := make(map[uint]string)
	if len(boardIDSet) > 0 {
		ids := make([]uint, 0, len(boardIDSet))
		for id := range boardIDSet {
			ids = append(ids, id)
		}
		var labels []models.SemanticLabel
		if err := h.db.WithContext(ctx).Where("id IN ? AND label_type = ?", ids, "board").Select("id, label").Find(&labels).Error; err == nil {
			for _, l := range labels {
				boardNames[l.ID] = l.Label
			}
		}
	}
	items := make([]boardUpgradeSuggestionRowDTO, 0, len(rows))
	for _, r := range rows {
		dto := boardUpgradeSuggestionRowDTO{
			ID: r.ID, BatchID: r.BatchID, Mode: r.Mode, Decision: r.Decision,
			BoardLabel: r.BoardLabel, Description: r.Description,
			TargetBoardID: r.TargetBoardID, AuxiliaryLabelIDs: r.AuxiliaryLabelIDs,
			Confidence: r.Confidence, Evidence: r.Evidence, Status: r.Status,
			DismissReason: r.DismissReason, CreatedAt: r.CreatedAt, ResolvedAt: r.ResolvedAt,
		}
		for _, id := range r.AuxiliaryLabelIDs {
			dto.AuxiliaryLabels = append(dto.AuxiliaryLabels, idLabelDTO{ID: id, Label: labelNames[id], Status: labelStatuses[id]})
		}
		if r.TargetBoardID != nil {
			dto.TargetBoardLabel = boardNames[*r.TargetBoardID]
		}
		items = append(items, dto)
	}
	return items
}
func (h *semanticBoardHandler) enqueueBackfill(c *gin.Context) {
	var req service.SemanticBoardBackfillRequest
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
func (h *semanticBoardHandler) backfillBoardEmbeddings(c *gin.Context) {
	ctx := c.Request.Context()
	var boards []models.SemanticLabel
	if err := h.db.WithContext(ctx).
		Where("label_type = ? AND embedding IS NULL", "board").
		Find(&boards).Error; err != nil {
		respondError(c, http.StatusInternalServerError, err)
		return
	}
	count := 0
	for _, board := range boards {
		input := semanticBoardEmbeddingInput(board.Label, board.Description)
		pgVector, _, err := semanticBoardLabelEmbedder(ctx, input, service.AuxiliaryLabelEmbeddingModeStorage)
		if err != nil {
			logging.Warnf("[backfill-embeddings] failed for board %d (%s): %v", board.ID, board.Label, err)
			continue
		}
		if err := h.db.WithContext(ctx).Model(&models.SemanticLabel{}).Where("id = ?", board.ID).Update("embedding", pgVector).Error; err != nil {
			logging.Warnf("[backfill-embeddings] db update failed for board %d: %v", board.ID, err)
			continue
		}
		count++
	}
	respondOK(c, gin.H{"backfilled": count, "total": len(boards)})
}
func (h *semanticBoardHandler) suggestionsToDTO(ctx context.Context, suggestions []service.SemanticBoardUpgradeSuggestion) []semanticBoardUpgradeSuggestionDTO {
	// Collect unique IDs for batch lookup
	labelIDSet := make(map[uint]struct{})
	boardIDSet := make(map[uint]struct{})
	for _, s := range suggestions {
		for _, id := range s.AuxiliaryLabelIDs {
			labelIDSet[id] = struct{}{}
		}
		if s.TargetBoardID != nil {
			boardIDSet[*s.TargetBoardID] = struct{}{}
		}
	}

	// Batch lookup auxiliary labels
	labelNames := make(map[uint]string)
	if len(labelIDSet) > 0 {
		ids := make([]uint, 0, len(labelIDSet))
		for id := range labelIDSet {
			ids = append(ids, id)
		}
		var labels []models.SemanticLabel
		if err := h.db.WithContext(ctx).Where("id IN ?", ids).Select("id, label").Find(&labels).Error; err == nil {
			for _, l := range labels {
				labelNames[l.ID] = l.Label
			}
		}
	}

	// Batch lookup board labels
	boardNames := make(map[uint]string)
	if len(boardIDSet) > 0 {
		ids := make([]uint, 0, len(boardIDSet))
		for id := range boardIDSet {
			ids = append(ids, id)
		}
		var labels []models.SemanticLabel
		if err := h.db.WithContext(ctx).Where("id IN ? AND label_type = ?", ids, "board").Select("id, label").Find(&labels).Error; err == nil {
			for _, l := range labels {
				boardNames[l.ID] = l.Label
			}
		}
	}

	items := make([]semanticBoardUpgradeSuggestionDTO, 0, len(suggestions))
	for _, s := range suggestions {
		dto := semanticBoardUpgradeSuggestionDTO{
			Decision:          s.Decision,
			BoardLabel:        s.BoardLabel,
			Description:       s.Description,
			AuxiliaryLabelIDs: s.AuxiliaryLabelIDs,
			TargetBoardID:     s.TargetBoardID,
			Reason:            s.Reason,
		}
		for _, id := range s.AuxiliaryLabelIDs {
			dto.AuxiliaryLabels = append(dto.AuxiliaryLabels, struct {
				ID    uint   `json:"id"`
				Label string `json:"label"`
			}{ID: id, Label: labelNames[id]})
		}
		if s.TargetBoardID != nil {
			if name, ok := boardNames[*s.TargetBoardID]; ok {
				dto.TargetBoardLabel = name
			}
		}
		items = append(items, dto)
	}
	return items
}
func semanticBoardUpgradeConfigToMap(config service.SemanticBoardUpgradeConfig) gin.H {
	return gin.H{
		"semantic_board_upgrade_ref_count_threshold":        config.RefCountThreshold,
		"semantic_board_upgrade_cluster_distance_threshold": config.ClusterDistanceThreshold,
		"semantic_board_upgrade_cotag_window_days":          config.CoTagWindowDays,
		"semantic_board_upgrade_cotag_top_n":                config.CoTagTopN,
		"semantic_board_upgrade_cotag_dedupe_sim_threshold": config.CoTagDedupeSimThreshold,
		"semantic_board_upgrade_cotag_hard_limit":           config.CoTagHardLimit,
		"semantic_board_upgrade_cluster_method":             config.ClusterMethod,
	}
}
