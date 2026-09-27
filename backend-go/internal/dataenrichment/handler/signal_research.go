package handler

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"syntopica-backend/internal/dataenrichment/repository"
	"syntopica-backend/internal/dataenrichment/service"
)

// ── 深入研究 API（board-signal-reports design §7，tasks 4.3）──────────────────
//
// POST /semantic-boards/:id/enrichment/analysis/signals/:candidateId/research
//   body {regenerate?:false}：候选归属校验（跨板块 404）、增强开关 400、
//   同板块互斥 409（running 含自身候选——单候选单任务）；无 running 且已有
//   成功报告且 regenerate=false → 200 返回已有 result_id（零新 LLM）；否则
//   202 新 research job（启动即报 candidate_id → 派生「研究中」）。
// GET  /semantic-boards/:id/enrichment/analysis/signal-reports?granularity&period
//      [&before_id&limit&source_signal_id]
//   仅成功报告，默认 20 最大 100；source_signal_id 过滤时校验候选归属。
// GET  /semantic-boards/:id/enrichment/analysis/signal-reports/:rid
//   owner/kind 不匹配 404；返回不可变 payload；无任何 review 字段。

// SignalResearchRunner abstracts service.SignalResearchService for handler
// tests (handler 层测试用 stub，不起真模型). jobID 是 runner 经 ctx 注入的
// 任务身份（tasks 4.7 进展持久化的滚动行主键；stub 下可为空）。
type SignalResearchRunner interface {
	ResearchCandidate(ctx context.Context, boardID, candidateID uint, jobID string, hook func(stage string)) (uint, error)
}

// SetSignalResearch wires the research runner post-construction（与
// SetSignalDiscovery 同一 post-construction 模式，InitHandler 签名保持稳定）.
func (h *EnrichmentHandler) SetSignalResearch(runner SignalResearchRunner) {
	h.signalResearch = runner
}

// SetSignalResearchOnInstance attaches the runner to the package singleton
// (wire.go path; must run after InitHandler and before RegisterRoutes).
func SetSignalResearchOnInstance(runner SignalResearchRunner) {
	if instance == nil {
		panic("handler.InitHandler must be called before SetSignalResearchOnInstance")
	}
	instance.signalResearch = runner
}

// triggerSignalResearch handles POST /signals/:candidateId/research.
func (h *EnrichmentHandler) triggerSignalResearch(c *gin.Context) {
	boardID, ok := parseBoardID(c)
	if !ok {
		return
	}
	candidateID, ok := parseIDParam(c, "candidateId")
	if !ok {
		return
	}
	// body 可缺省（{} / 空 body 都合法）：仅 regenerate 一个语义开关。
	regenerate := false
	if c.Request.Body != nil && c.Request.ContentLength != 0 {
		var req struct {
			Regenerate bool `json:"regenerate"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			respondError(c, http.StatusBadRequest, "invalid request body")
			return
		}
		regenerate = req.Regenerate
	}
	// 候选存在性与板块归属：跨板块一律 404（不暴露存在性，API-3）。
	candidate, err := h.repo.GetSignalCandidateByID(c.Request.Context(), candidateID)
	if err != nil {
		respondError(c, http.StatusNotFound, "signal candidate not found")
		return
	}
	if candidate.SemanticBoardID != boardID {
		respondError(c, http.StatusNotFound, "signal candidate not found")
		return
	}
	if h.signalResearch == nil {
		respondError(c, http.StatusInternalServerError, "signal research service not wired")
		return
	}
	// 板块增强开关同步预检（同 discovery/旧 brief 语义）。
	if err := h.orchestrator.BoardEnrichmentEnabled(c.Request.Context(), boardID); err != nil {
		if containsNotEnabled(err.Error()) {
			respondError(c, http.StatusBadRequest,
				"enrichment not enabled for this board：请先开启「数据增强」开关（工作台分析区一键开启，或板块编辑弹窗→分析配置）")
			return
		}
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}
	// 同板块互斥先行（design §7：先共享 board 互斥，running 一律 409——含
	// 自身候选重复点击；也不把「已有报告 + 他任务在跑」误答成 200 复用）。
	// Status 只是前置提示；创建时的原子裁决仍由 StartSignal 的锁兜底
	// （防检查/执行竞态）。
	if st, ok := h.analysis.Status(AnalysisScopeBoard, boardID); ok && st.Running {
		respondErrorWithData(c, http.StatusConflict, "board analysis already running", st)
		return
	}
	// 幂等复用（API-5）：无 running 且已有成功报告、未显式 regenerate →
	// 200 返回已有 result_id，零新 LLM。
	if !regenerate {
		if latest, err := h.repo.GetLatestSignalReportIDForCandidate(c.Request.Context(), candidateID); err == nil && latest != nil {
			respondOK(c, gin.H{"status": "already_reported", "result_id": *latest})
			return
		}
	}

	st, err := h.analysis.StartSignal(AnalysisScopeBoard, boardID, AnalysisJobKindBoardSignalReport, signalResearchJobTimeout,
		func(ctx context.Context, report func(SignalJobPatch)) error {
			// 启动即报 candidate_id（2a 合同：派生「研究中」依赖 live job 的
			// candidate_id patch）+ 周期身份。
			report(SignalJobPatch{
				Phase:       service.SignalStageResearch,
				CandidateID: candidateID,
				Granularity: candidate.Granularity,
				Period:      candidate.Period,
			})
			// job_id 由 runner 经 ctx 注入（tasks 4.7）：研究进展表以它为滚动行
			// 主键，断了不能白跑。
			resultID, rerr := h.signalResearch.ResearchCandidate(ctx, boardID, candidateID, jobIDFromContext(ctx), func(stage string) {
				report(SignalJobPatch{Phase: stage})
			})
			if rerr != nil {
				patch := SignalJobPatch{Outcome: SignalOutcomeFailed}
				var stageErr *service.SignalStageError
				if errors.As(rerr, &stageErr) {
					patch.ErrorStage = stageErr.Stage
				}
				report(patch)
				return rerr
			}
			// 成功：outcome=succeeded + result_id（JB-2：result_id 仅成功存在；
			// 经 patch 上报——StartSignal 的 fn 只返 error，无法带出返回值）。
			report(SignalJobPatch{Outcome: SignalOutcomeSucceeded, ResultID: resultID})
			return nil
		})
	if err != nil {
		var runErr *RunningJobError
		if errors.As(err, &runErr) {
			// 409 携当前任务身份：同板块任一任务在跑（含自身候选重复点击）。
			respondErrorWithData(c, http.StatusConflict, "board analysis already running", runErr.Current)
			return
		}
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}
	respondAccepted(c, gin.H{
		"status":       "started",
		"job_id":       st.JobID,
		"job_kind":     AnalysisJobKindBoardSignalReport,
		"scope":        AnalysisScopeBoard,
		"target_id":    boardID,
		"candidate_id": candidateID,
		"granularity":  candidate.Granularity,
		"period":       candidate.Period,
	})
}

const (
	signalReportsDefaultLimit = 20
	signalReportsMaxLimit     = 100
)

// listSignalReports handles GET /signal-reports：仅成功 signal_report，按 id
// 倒序游标分页；可带 source_signal_id 过滤该候选的版本序列（校验归属）。
func (h *EnrichmentHandler) listSignalReports(c *gin.Context) {
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
	limit, ok := parseListLimit(c, signalReportsDefaultLimit, signalReportsMaxLimit)
	if !ok {
		return
	}

	var (
		results []repository.TopicEnrichmentResult
		err     error
	)
	if raw := strings.TrimSpace(c.Query("source_signal_id")); raw != "" {
		v, perr := strconv.ParseUint(raw, 10, 64)
		if perr != nil || v == 0 {
			respondError(c, http.StatusBadRequest, "invalid source_signal_id")
			return
		}
		candidate, cerr := h.repo.GetSignalCandidateByID(c.Request.Context(), uint(v))
		if cerr != nil || candidate.SemanticBoardID != boardID {
			// 跨板块/不存在的候选一律 404（校验归属，API-3）。
			respondError(c, http.StatusNotFound, "signal candidate not found")
			return
		}
		results, err = h.repo.ListSignalReportVersionsByCandidate(c.Request.Context(), boardID, uint(v), beforeID, limit)
	} else {
		results, err = h.repo.ListSignalReportResults(c.Request.Context(), boardID, granularity, period, beforeID, limit)
	}
	if err != nil {
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}
	list := make([]gin.H, 0, len(results))
	for i := range results {
		list = append(list, serializeSignalReportSummary(&results[i]))
	}
	respondOK(c, list)
}

// getSignalReport handles GET /signal-reports/:rid：owner/kind 不匹配一律
// 404；返回不可变 payload（含工具日志/输入快照），无任何 review 字段。
func (h *EnrichmentHandler) getSignalReport(c *gin.Context) {
	boardID, ok := parseBoardID(c)
	if !ok {
		return
	}
	rid, ok := parseIDParam(c, "rid")
	if !ok {
		return
	}
	result, err := h.repo.GetTopicEnrichmentResultByID(c.Request.Context(), rid)
	if err != nil {
		respondError(c, http.StatusNotFound, "signal report not found")
		return
	}
	if result.ResultKind != repository.ResultKindSignalReport || result.AnalysisScope != "board" ||
		result.SemanticBoardID == nil || *result.SemanticBoardID != boardID {
		respondError(c, http.StatusNotFound, "signal report not found")
		return
	}
	respondOK(c, serializeSignalReportDetail(result))
}

// serializeSignalReportSummary 是列表行：报告身份 + 不可变 payload（sectors）。
// 刻意不含 tool_calls/input_snapshot（完整工具日志只在详情暴露，列表不带大
// 载荷）；也不含任何 review/judge 键（design §7：响应无 review 字段）。
func serializeSignalReportSummary(r *repository.TopicEnrichmentResult) gin.H {
	return gin.H{
		"id":                r.ID,
		"analysis_scope":    r.AnalysisScope,
		"result_kind":       r.ResultKind,
		"semantic_board_id": r.SemanticBoardID,
		"granularity":       r.Granularity,
		"period":            r.Period,
		"source_signal_id":  r.SourceSignalID,
		"sectors":           tryParseJSON(r.Sectors),
		"session_id":        r.SessionID,
		"created_at":        r.CreatedAt,
	}
}

// serializeSignalReportDetail 是详情：追加完整工具日志与输入快照（OB-2 可
// 重建性）；同样无 review 字段。
func serializeSignalReportDetail(r *repository.TopicEnrichmentResult) gin.H {
	out := serializeSignalReportSummary(r)
	out["tool_calls"] = tryParseJSON(r.ToolCalls)
	out["input_snapshot"] = tryParseJSON(r.InputSnapshot)
	return out
}
