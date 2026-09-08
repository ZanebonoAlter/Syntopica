package handler

import (
	"context"
	"errors"
	"fmt"
	"hash/fnv"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"syntopica-backend/internal/dataenrichment/repository"
	"syntopica-backend/internal/dataenrichment/service"
	"syntopica-backend/internal/platform/config"
)

// ── 跨版块关系发现 API（add-evidence-backed-cross-board-relations 任务组 4）──
//
//	POST /semantic-boards/:id/enrichment/relations/discover        202 job / 409
//	GET  /semantic-boards/:id/enrichment/relations?status=&limit=  列表（双侧匹配）
//	GET  /semantic-boards/:id/enrichment/relations/:rid            详情
//	POST /semantic-boards/:id/enrichment/relations/:rid/confirm    proposed→confirmed（409 非法态）
//	POST /semantic-boards/:id/enrichment/relations/:rid/dismiss    proposed/unresolved→dismissed
//	POST /semantic-boards/:id/enrichment/relations/:rid/re-resolve unresolved 重解析
//
// 发现 job 复用 analysis runner（202/409/analysis-status 轮询约定）；job 的
// scope 为 "relation"，target 为 source 引用的稳定散列（同 board 多 source
// 可并行，同 source 重复触发 409）。

const (
	AnalysisJobKindRelationDiscovery = "relation_discovery"
	AnalysisScopeRelation            = "relation"
	relationJobTimeout               = 12 * time.Minute
)

// relationSourceJobID hashes (parentID, kind, key) into a uint job target so
// the analysis runner's (scope, id) keying can serialize per source.
func relationSourceJobID(parentID uint, kind, key string) uint {
	h := fnv.New32a()
	_, _ = fmt.Fprintf(h, "%d|%s|%s", parentID, kind, key)
	return uint(h.Sum32())
}

// triggerRelationDiscovery starts a manual discovery run.
// Body: {"briefing_result_id": N, "source_kind": "observation|question", "source_key": "o1"}.
// All source validation happens server-side from the parent brief — the
// client never supplies source text (spec: 从观察手动发现).
func (h *EnrichmentHandler) triggerRelationDiscovery(c *gin.Context) {
	boardID, ok := parseBoardID(c)
	if !ok {
		return
	}
	var req struct {
		BriefingResultID uint64 `json:"briefing_result_id"`
		SourceKind       string `json:"source_kind"`
		SourceKey        string `json:"source_key"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.BriefingResultID == 0 {
		respondError(c, http.StatusBadRequest, "briefing_result_id is required")
		return
	}
	sourceKind := strings.TrimSpace(req.SourceKind)
	sourceKey := strings.TrimSpace(req.SourceKey)
	if sourceKind != repository.RelationSourceObservation && sourceKind != repository.RelationSourceQuestion {
		respondError(c, http.StatusBadRequest, "source_kind must be observation or question")
		return
	}
	if sourceKey == "" {
		respondError(c, http.StatusBadRequest, "source_key is required")
		return
	}

	// Parent brief pre-flight (board ownership + kind + source existence) —
	// all BEFORE the job slot is taken, failures cost zero background calls.
	parent, err := h.repo.GetTopicEnrichmentResultByID(c.Request.Context(), uint(req.BriefingResultID))
	if err != nil {
		respondError(c, http.StatusNotFound, "briefing result not found")
		return
	}
	if !repository.BoardIDMatches(parent.SemanticBoardID, boardID) {
		respondError(c, http.StatusNotFound, "briefing result not found for this board")
		return
	}
	if kind := repository.EffectiveResultKind(parent); kind != repository.ResultKindBoardBrief {
		respondError(c, http.StatusBadRequest, "briefing result is "+kind+", not a board_brief")
		return
	}
	if err := service.ValidateRelationSourceKey(parent.Sectors, sourceKind, sourceKey); err != nil {
		respondError(c, http.StatusBadRequest, err.Error())
		return
	}

	parentID := uint(req.BriefingResultID)
	jobTarget := relationSourceJobID(parentID, sourceKind, sourceKey)
	st, err := h.analysis.Start(AnalysisScopeRelation, jobTarget, AnalysisJobKindRelationDiscovery, relationJobTimeout, func(ctx context.Context) (uint, error) {
		out, err := h.orchestrator.RunRelationDiscovery(ctx, service.RelationDiscoveryInput{
			BoardID: boardID,
			Source: service.RelationSourceRef{
				ParentResultID: parentID,
				SourceKind:     sourceKind,
				SourceKey:      sourceKey,
			},
			TriggerKind: repository.RelationTriggerManual,
		})
		if err != nil {
			return 0, err
		}
		return out.RunID, nil
	})
	if err != nil {
		var runErr *RunningJobError
		if errors.As(err, &runErr) {
			respondErrorWithData(c, http.StatusConflict, "relation discovery already running for this source", runErr.Current)
			return
		}
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}
	respondAccepted(c, gin.H{
		"status":    "started",
		"job_id":    st.JobID,
		"job_kind":  AnalysisJobKindRelationDiscovery,
		"scope":     AnalysisScopeRelation,
		"target_id": boardID,
	})
}

// listBoardRelations returns relations touching the board on either side.
// GET /semantic-boards/:id/enrichment/relations?status=unresolved,proposed&limit=50
func (h *EnrichmentHandler) listBoardRelations(c *gin.Context) {
	boardID, ok := parseBoardID(c)
	if !ok {
		return
	}
	var statuses []string
	if raw := strings.TrimSpace(c.Query("status")); raw != "" {
		for _, s := range strings.Split(raw, ",") {
			s = strings.TrimSpace(s)
			if s == "" {
				continue
			}
			if !repository.ValidRelationStatus(s) {
				respondError(c, http.StatusBadRequest, "invalid status filter: "+s)
				return
			}
			statuses = append(statuses, s)
		}
	}
	limit := 50
	if n, err := strconv.Atoi(c.Query("limit")); err == nil && n > 0 && n <= 200 {
		limit = n
	}
	rows, err := h.repo.ListCrossBoardRelations(c.Request.Context(), repository.CrossBoardRelationFilter{
		BoardID:  &boardID,
		Statuses: statuses,
		Limit:    limit,
	})
	if err != nil {
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}
	list := make([]gin.H, 0, len(rows))
	for i := range rows {
		list = append(list, serializeCrossBoardRelation(&rows[i]))
	}
	respondOK(c, list)
}

// getBoardRelation returns one relation detail with provenance.
func (h *EnrichmentHandler) getBoardRelation(c *gin.Context) {
	rel, boardID, ok := h.requireBoardRelation(c)
	if !ok {
		return
	}
	_ = boardID
	payload := serializeCrossBoardRelation(rel)
	payload["mapping_snapshot"] = tryParseJSON(rel.MappingSnapshot)
	payload["gaps"] = tryParseJSON(rel.Gaps)
	// Run linkage for traceability (spec: 追溯已确认关系).
	if rel.RunID != nil {
		if run, err := h.repo.GetRelationRunByID(c.Request.Context(), *rel.RunID); err == nil {
			payload["run"] = gin.H{
				"id": run.ID, "status": run.Status, "trigger_kind": run.TriggerKind,
				"source_kind": run.SourceKind, "source_key": run.SourceKey,
				"budget_snapshot": tryParseJSON(run.BudgetSnapshot),
				"gaps":            tryParseJSON(run.Gaps), "error": run.Error,
				"created_at": run.CreatedAt,
			}
		}
	}
	respondOK(c, payload)
}

// confirmBoardRelation transitions proposed → confirmed with the configured
// TTL; the repository re-validates target existence inside the transaction
// (spec: 用户确认建议 / 确认前重验).
func (h *EnrichmentHandler) confirmBoardRelation(c *gin.Context) {
	rel, _, ok := h.requireBoardRelation(c)
	if !ok {
		return
	}
	cfg := config.EffectiveCrossBoardRelationConfig()
	if err := h.repo.ConfirmCrossBoardRelation(c.Request.Context(), rel.ID, "user",
		time.Duration(cfg.ConfirmedTTLHours)*time.Hour); err != nil {
		if errors.Is(err, repository.ErrRelationStateConflict) {
			respondError(c, http.StatusConflict, "relation is not confirmable (must be proposed with a live target)")
			return
		}
		if errors.Is(err, errRelationNotFound) {
			respondError(c, http.StatusNotFound, "relation not found")
			return
		}
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}
	fresh, err := h.repo.GetCrossBoardRelationByID(c.Request.Context(), rel.ID)
	if err != nil {
		respondOK(c, gin.H{"id": rel.ID, "status": repository.RelationStatusConfirmed})
		return
	}
	respondOK(c, serializeCrossBoardRelation(fresh))
}

// dismissBoardRelation transitions proposed/unresolved → dismissed with a
// mandatory reason (spec: 驳回建议进入冷却).
func (h *EnrichmentHandler) dismissBoardRelation(c *gin.Context) {
	rel, _, ok := h.requireBoardRelation(c)
	if !ok {
		return
	}
	var req struct {
		Reason string `json:"reason"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || strings.TrimSpace(req.Reason) == "" {
		respondError(c, http.StatusBadRequest, "reason is required")
		return
	}
	if err := h.repo.DismissCrossBoardRelation(c.Request.Context(), rel.ID, strings.TrimSpace(req.Reason), "user"); err != nil {
		if errors.Is(err, repository.ErrRelationStateConflict) {
			respondError(c, http.StatusConflict, "relation is not dismissable (must be proposed or unresolved)")
			return
		}
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}
	fresh, err := h.repo.GetCrossBoardRelationByID(c.Request.Context(), rel.ID)
	if err != nil {
		respondOK(c, gin.H{"id": rel.ID, "status": repository.RelationStatusDismissed})
		return
	}
	respondOK(c, serializeCrossBoardRelation(fresh))
}

// reResolveBoardRelation re-runs the conservative resolver on an unresolved
// relation (spec: 外部概念尚无内部目标 → 允许后续重新解析).
func (h *EnrichmentHandler) reResolveBoardRelation(c *gin.Context) {
	rel, boardID, ok := h.requireBoardRelation(c)
	if !ok {
		return
	}
	if rel.Status != repository.RelationStatusUnresolved {
		respondError(c, http.StatusConflict, "relation is not unresolved; re-resolve applies to unresolved only")
		return
	}
	out, err := h.orchestrator.ReResolveRelation(c.Request.Context(), boardID, rel.ID)
	if err != nil {
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}
	respondOK(c, out)
}

// errRelationNotFound marks a lookup miss inside requireBoardRelation.
var errRelationNotFound = errors.New("relation not found")

// requireBoardRelation loads the relation and enforces board ownership
// (either side). Unified 404 on miss/mismatch, mirroring requireBoardResult.
func (h *EnrichmentHandler) requireBoardRelation(c *gin.Context) (*repository.CrossBoardRelation, uint, bool) {
	boardID, ok := parseBoardID(c)
	if !ok {
		return nil, 0, false
	}
	rid, err := strconv.ParseUint(c.Param("rid"), 10, 64)
	if err != nil || rid == 0 {
		respondError(c, http.StatusBadRequest, "invalid relation id")
		return nil, 0, false
	}
	rel, err := h.repo.GetCrossBoardRelationByID(c.Request.Context(), uint(rid))
	if err != nil {
		respondError(c, http.StatusNotFound, "relation not found")
		return nil, 0, false
	}
	if rel.SourceBoardID != boardID && (rel.TargetBoardID == nil || *rel.TargetBoardID != boardID) {
		respondError(c, http.StatusNotFound, "relation not found")
		return nil, 0, false
	}
	return rel, boardID, true
}

// serializeCrossBoardRelation shapes one relation row for the API.
func serializeCrossBoardRelation(r *repository.CrossBoardRelation) gin.H {
	return gin.H{
		"id":                   r.ID,
		"run_id":               r.RunID,
		"source_board_id":      r.SourceBoardID,
		"target_board_id":      r.TargetBoardID,
		"target_lane_id":       r.TargetLaneID,
		"target_concept":       r.TargetConcept,
		"relation_type":        r.RelationType,
		"claim":                r.Claim,
		"mechanism":            r.Mechanism,
		"verification_verdict": r.VerificationVerdict,
		"quality_grade":        r.QualityGrade,
		"evidence":             r.Evidence,
		"counterevidence":      r.Counterevidence,
		"status":               r.Status,
		"suggestion_hash":      r.SuggestionHash,
		"evidence_version":     r.EvidenceVersion,
		"expires_at":           r.ExpiresAt,
		"confirmed_at":         r.ConfirmedAt,
		"dismissed_at":         r.DismissedAt,
		"expired_at":           r.ExpiredAt,
		"dismiss_reason":       r.DismissReason,
		"resolved_by":          r.ResolvedBy,
		"created_at":           r.CreatedAt,
		"updated_at":           r.UpdatedAt,
	}
}
