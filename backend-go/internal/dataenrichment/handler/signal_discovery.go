package handler

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"syntopica-backend/internal/dataenrichment/repository"
	"syntopica-backend/internal/dataenrichment/service"
)

// ── 信号发现 API（board-signal-reports design §7，tasks 2.4）──────────────────
//
// POST /semantic-boards/:id/enrichment/analysis/signal-discoveries  {granularity, period}
//   周期非法/未来 → 400 且无 job（PC-1/PC-2）；增强开关关闭 → 400；
//   同板块任一任务在跑 → 409（共享 board 互斥，与旧 brief/investigation 同锁）；
//   合法 → 202 + discovery job（prepare 材料 → detect ≤2 尝试 → 原子保存批次）。
// GET  /semantic-boards/:id/enrichment/analysis/signals?granularity&period&before_id&limit
//   候选列表：id 倒序游标、默认 20 最大 100、批次发现时间、服务端派生状态
//   （研究中/已有报告/待研究）；空新批次天然不清旧记录（只查候选表）。
//
// discovery 永不返回 result_id（design §7：不带 report result_id、不启动
// research——那是 2b 的辖区；现有回调无条件读 output.Result.ID 的 nil 隐患
// 由 job fn 恒返 0 规避）。

// SignalDiscoveryRunner abstracts service.SignalDiscoveryService for handler
// tests (handler 层测试用 stub，不起真模型).
type SignalDiscoveryRunner interface {
	DiscoverSignals(ctx context.Context, boardID uint, granularity, period string, hook func(stage string)) (*service.SignalDiscoveryOutcome, error)
}

// SetSignalDiscovery wires the signal-discovery runner post-construction —
// same pattern as OrchestratorService.SetBoardConfigResolver (M5.1): keeps
// the positional NewHandler/InitHandler signatures stable for existing
// callers while production wiring (wire.go) can attach the dependency.
func (h *EnrichmentHandler) SetSignalDiscovery(runner SignalDiscoveryRunner) {
	h.signals = runner
}

// SetSignalDiscoveryOnInstance attaches the runner to the package singleton
// created by InitHandler (wire.go path; RegisterRoutes panics when unset,
// so this must run after InitHandler and before route registration).
func SetSignalDiscoveryOnInstance(runner SignalDiscoveryRunner) {
	if instance == nil {
		panic("handler.InitHandler must be called before SetSignalDiscoveryOnInstance")
	}
	instance.signals = runner
}

// Derived candidate statuses (design §3: 候选状态为服务端派生，running 位
// 不持久化——「研究中」来自 live job，其余来自 repository 查询).
const (
	SignalCandidateStatusPending     = "pending"     // 待研究
	SignalCandidateStatusResearching = "researching" // 研究中（live research job）
	SignalCandidateStatusReported    = "reported"    // 已有成功报告
)

const (
	signalCandidatesDefaultLimit = 20
	signalCandidatesMaxLimit     = 100
)

// triggerSignalDiscovery validates the period synchronously (400 without a
// job on any violation), then starts the discovery job under the shared board
// mutex. Job fn: prepare（材料装配）→ detect（≤2 尝试）→ 原子保存批次+候选 →
// outcome=discovered|no_signal（0 条；失败 outcome=failed + error_stage）。
func (h *EnrichmentHandler) triggerSignalDiscovery(c *gin.Context) {
	boardID, ok := parseBoardID(c)
	if !ok {
		return
	}
	var req struct {
		Granularity string `json:"granularity"`
		Period      string `json:"period"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "granularity and period are required")
		return
	}
	// 周期校验（业务时区 + 未来拒绝）：与 service 同一 ParseSignalPeriod 口径，
	// 400 且不占 job 槽——后续增强开关/互斥检查都排在它后面。
	if _, err := service.ParseSignalPeriod(req.Granularity, req.Period, time.Now()); err != nil {
		respondError(c, http.StatusBadRequest, err.Error())
		return
	}
	if h.signals == nil {
		respondError(c, http.StatusInternalServerError, "signal discovery service not wired")
		return
	}
	// 板块增强开关同步预检（同 triggerBoardEnrichment；M6.1 语义）。
	if err := h.orchestrator.BoardEnrichmentEnabled(c.Request.Context(), boardID); err != nil {
		if containsNotEnabled(err.Error()) {
			respondError(c, http.StatusBadRequest,
				"enrichment not enabled for this board：请先开启「数据增强」开关（工作台分析区一键开启，或板块编辑弹窗→分析配置）")
			return
		}
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}

	st, err := h.analysis.StartSignal(AnalysisScopeBoard, boardID, AnalysisJobKindBoardSignalDiscovery, analysisJobTimeout,
		func(ctx context.Context, report func(SignalJobPatch)) error {
			outcome, derr := h.signals.DiscoverSignals(ctx, boardID, req.Granularity, req.Period, func(stage string) {
				report(SignalJobPatch{Phase: stage})
			})
			if derr != nil {
				patch := SignalJobPatch{Outcome: SignalOutcomeFailed}
				var stageErr *service.SignalStageError
				if errors.As(derr, &stageErr) {
					patch.ErrorStage = stageErr.Stage
				}
				report(patch)
				return derr
			}
			kind := SignalOutcomeDiscovered
			if outcome.CandidateCount == 0 {
				kind = SignalOutcomeNoSignal // 零候选：正常完成，不伪装失败也不清旧
			}
			report(SignalJobPatch{
				Outcome:        kind,
				DiscoveryID:    outcome.DiscoveryID,
				CandidateCount: outcome.CandidateCount,
			})
			// 恒返 0：discovery 无 result_id（omitempty 后状态里不出现该键）。
			return nil
		})
	if err != nil {
		var runErr *RunningJobError
		if errors.As(err, &runErr) {
			// 409 携当前任务身份：与旧 brief/investigation 共享同一互斥语义。
			respondErrorWithData(c, http.StatusConflict, "board analysis already running", runErr.Current)
			return
		}
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}
	respondAccepted(c, gin.H{
		"status":      "started",
		"job_id":      st.JobID,
		"job_kind":    AnalysisJobKindBoardSignalDiscovery,
		"scope":       AnalysisScopeBoard,
		"target_id":   boardID,
		"granularity": req.Granularity,
		"period":      req.Period,
	})
}

// listSignalCandidates returns one period's candidates, newest batch first,
// keyed by a candidate-id cursor. 派生状态链（design §3）：有 running research
// job → researching；否则有成功报告 → reported；否则 → pending。
func (h *EnrichmentHandler) listSignalCandidates(c *gin.Context) {
	boardID, ok := parseBoardID(c)
	if !ok {
		return
	}
	granularity := strings.TrimSpace(c.Query("granularity"))
	period := strings.TrimSpace(c.Query("period"))
	if !repository.ValidSignalGranularity(granularity) {
		respondError(c, http.StatusBadRequest, "granularity must be month|year")
		return
	}
	if !repository.ValidSignalPeriod(granularity, period) {
		respondError(c, http.StatusBadRequest, "period does not match granularity (month=YYYY-MM, year=YYYY)")
		return
	}
	beforeID, ok := parseBeforeID(c)
	if !ok {
		return
	}
	limit, ok := parseListLimit(c, signalCandidatesDefaultLimit, signalCandidatesMaxLimit)
	if !ok {
		return
	}

	items, err := h.repo.ListSignalCandidatesByPeriod(c.Request.Context(), boardID, granularity, period, beforeID, limit)
	if err != nil {
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}
	// 上次研究进展摘要（tasks 4.7）：一次 IN 查询取齐本页候选的最近进展行，
	// 不逐行查询（不加重现有 N+1 备案）。
	candidateIDs := make([]uint, 0, len(items))
	for i := range items {
		candidateIDs = append(candidateIDs, items[i].ID)
	}
	progressByCandidate, err := h.repo.ListLatestSignalResearchProgress(c.Request.Context(), candidateIDs)
	if err != nil {
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}
	out := make([]gin.H, 0, len(items))
	for i := range items {
		item := items[i]
		status, latest := h.deriveSignalCandidateStatus(c.Request.Context(), boardID, item.ID)
		var progress gin.H
		if p, ok := progressByCandidate[item.ID]; ok {
			progress = serializeSignalResearchProgressSummary(&p)
		}
		out = append(out, gin.H{
			"id":                     item.ID,
			"discovery_id":           item.DiscoveryID,
			"granularity":            item.Granularity,
			"period":                 item.Period,
			"signal":                 item.Signal,
			"why_it_matters":         item.WhyItMatters,
			"research_question":      item.ResearchQuestion,
			"evidence_refs":          tryParseJSON(item.EvidenceRefs),
			"score":                  item.Score,
			"rationale":              item.Rationale,
			"discovery_created_at":   item.DiscoveryCreatedAt,
			"status":                 status,
			"latest_result_id":       latest,
			"last_research_progress": progress,
		})
	}
	respondOK(c, out)
}

// serializeSignalResearchProgressSummary 是候选行/端点共用的进展摘要：轮次
// 与计数 + 状态 + 失败原因。stop_reason/error 为空时显式 null（成功归档行无
// 失败语义，不留给前端猜空串）。
func serializeSignalResearchProgressSummary(p *repository.BoardSignalResearchProgress) gin.H {
	stopReason := any(nil)
	if p.StopReason != "" {
		stopReason = p.StopReason
	}
	return gin.H{
		"rounds_done":       p.RoundsDone,
		"source_calls":      p.SourceCalls,
		"calculation_calls": p.CalculationCalls,
		"status":            p.Status,
		"stop_reason":       stopReason,
		"updated_at":        p.UpdatedAt,
	}
}

// getSignalResearchProgress handles GET /signals/:candidateId/research-progress
// (tasks 4.7)：候选最近一次研究的完整进展（含 ledger jsonb 全量账本）。候选
// 不存在/跨板块 → 404（同款不暴露存在性）；候选存在但从未研究 → 200 data=null。
func (h *EnrichmentHandler) getSignalResearchProgress(c *gin.Context) {
	boardID, ok := parseBoardID(c)
	if !ok {
		return
	}
	candidateID, ok := parseIDParam(c, "candidateId")
	if !ok {
		return
	}
	candidate, err := h.repo.GetSignalCandidateByID(c.Request.Context(), candidateID)
	if err != nil || candidate.SemanticBoardID != boardID {
		respondError(c, http.StatusNotFound, "signal candidate not found")
		return
	}
	progress, err := h.repo.GetLatestSignalResearchProgress(c.Request.Context(), candidateID)
	if err != nil {
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}
	if progress == nil {
		respondOK(c, nil)
		return
	}
	stopReason := any(nil)
	errText := any(nil)
	if progress.StopReason != "" {
		stopReason = progress.StopReason
	}
	if progress.Error != "" {
		errText = progress.Error
	}
	respondOK(c, gin.H{
		"job_id":            progress.JobID,
		"candidate_id":      progress.CandidateID,
		"semantic_board_id": progress.SemanticBoardID,
		"granularity":       progress.Granularity,
		"period":            progress.Period,
		"rounds_done":       progress.RoundsDone,
		"source_calls":      progress.SourceCalls,
		"calculation_calls": progress.CalculationCalls,
		"ledger":            tryParseJSON(progress.Ledger),
		"status":            progress.Status,
		"stop_reason":       stopReason,
		"error":             errText,
		"created_at":        progress.CreatedAt,
		"updated_at":        progress.UpdatedAt,
	})
}

// deriveSignalCandidateStatus implements the design §3 status chain. The
// running half comes from the live job table (never persisted, so a restart
// can never leave a candidate stuck on 研究中); the report half comes from the
// result table.
func (h *EnrichmentHandler) deriveSignalCandidateStatus(ctx context.Context, boardID, candidateID uint) (string, *uint) {
	if _, running := h.analysis.RunningSignalReportForCandidate(boardID, candidateID); running {
		return SignalCandidateStatusResearching, nil
	}
	latest, err := h.repo.GetLatestSignalReportIDForCandidate(ctx, candidateID)
	if err != nil || latest == nil {
		return SignalCandidateStatusPending, nil
	}
	return SignalCandidateStatusReported, latest
}
