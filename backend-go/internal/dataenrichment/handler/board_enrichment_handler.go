package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"syntopica-backend/internal/dataenrichment/repository"
	"syntopica-backend/internal/dataenrichment/service"
)

// ── 版块级分析 API（design D9，tasks 5.x / M9）───────────────────────────────
//
// POST /semantic-boards/:id/enrichment/analysis/trigger                —— 触发版块简报（board_brief）
// POST /semantic-boards/:id/enrichment/analysis/investigations/trigger —— 触发问题调查（board_investigation）
// GET  /semantic-boards/:id/enrichment/analysis/results[?kind=]        —— 历史列表（kind 过滤）
// GET  /semantic-boards/:id/enrichment/analysis/results/:rid           —— 单份详情（三 kind + legacy）
//
// 单泳道 trigger 的可选 body {prefill_lens} 见 handler.go triggerEnrichment。

// triggerBoardEnrichment starts the board_brief cycle-B flow in the
// background and returns immediately (fix-board-analysis-material 8.x +
// board-level-deep-analysis D9): the run survives client disconnects; the
// 202 envelope carries the job identity for polling by job_id.
func (h *EnrichmentHandler) triggerBoardEnrichment(c *gin.Context) {
	boardID, ok := parseBoardID(c)
	if !ok {
		return
	}

	err := h.orchestrator.BoardEnrichmentEnabled(c.Request.Context(), boardID)
	if err != nil {
		// Board-not-enabled is a client error (M6.1); everything else 500.
		// Message keeps the machine-matchable English prefix (containsNotEnabled,
		// tests) and appends a human hint — the flag lives in the board edit dialog
		// / panel quick-toggle, which was undiscoverable before.
		if containsNotEnabled(err.Error()) {
			respondError(c, http.StatusBadRequest,
				"enrichment not enabled for this board：请先开启「数据增强」开关（工作台分析区一键开启，或板块编辑弹窗→分析配置）")
			return
		}
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}

	st, err := h.analysis.Start(AnalysisScopeBoard, boardID, AnalysisJobKindBoardBrief, analysisJobTimeout, func(ctx context.Context) (uint, error) {
		output, err := h.orchestrator.EnrichBoard(ctx, boardID)
		if err != nil {
			return 0, err
		}
		return output.Result.ID, nil
	})
	if err != nil {
		var runErr *RunningJobError
		if errors.As(err, &runErr) {
			// 409 携当前任务身份：前端恢复该 job 轮询，不误把另一 kind 当成本次触发。
			respondErrorWithData(c, http.StatusConflict, "board analysis already running", runErr.Current)
			return
		}
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}
	respondAccepted(c, gin.H{
		"status":    "started",
		"job_id":    st.JobID,
		"job_kind":  AnalysisJobKindBoardBrief,
		"scope":     AnalysisScopeBoard,
		"target_id": boardID,
	})
}

// boardBriefQuestionRef is the handler-side projection of a stored brief's
// research_questions — just enough to resolve a generated question_id back
// to its canonical text (the brief is the source of truth; body-supplied
// text for generated questions is ignored so question_key stays stable).
type boardBriefQuestionRef struct {
	ID       string `json:"id"`
	Question string `json:"question"`
}

type boardBriefSectorRef struct {
	ResultKind        string                  `json:"result_kind"`
	ResearchQuestions []boardBriefQuestionRef `json:"research_questions"`
}

// triggerBoardInvestigation starts a board_investigation against a stored
// board_brief. POST /semantic-boards/:id/enrichment/analysis/investigations/trigger
// Body: {"briefing_result_id": N, "question_id"?: "q1", "question"?: "..."}。
// question_id 非空 → source=generated（文本从父简报候选解析）；否则 custom。
// 同步预检（trim/枚举/父存在/同板块/kind=board_brief）失败 → 400/404 且 0 后台调用；
// 合法 → 202 + 独立 job_id，后台 detached 调 InvestigateBoardQuestion。
func (h *EnrichmentHandler) triggerBoardInvestigation(c *gin.Context) {
	boardID, ok := parseBoardID(c)
	if !ok {
		return
	}

	var req struct {
		BriefingResultID uint64 `json:"briefing_result_id"`
		QuestionID       string `json:"question_id"`
		Question         string `json:"question"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "briefing_result_id is required")
		return
	}
	if req.BriefingResultID == 0 {
		respondError(c, http.StatusBadRequest, "briefing_result_id is required")
		return
	}

	// 板块开关同步预检（服务层入口还会再验一次；这里同步挡住免得占住 job 槽后秒错）。
	if err := h.orchestrator.BoardEnrichmentEnabled(c.Request.Context(), boardID); err != nil {
		if containsNotEnabled(err.Error()) {
			respondError(c, http.StatusBadRequest,
				"enrichment not enabled for this board：请先开启「数据增强」开关（工作台分析区一键开启，或板块编辑弹窗→分析配置）")
			return
		}
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}

	// 父简报同步校验（全部在启动 job 之前，失败 0 后台调用）。
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

	question := service.BoardInvestigationQuestion{
		Text:   strings.TrimSpace(req.Question),
		Source: service.QuestionSourceCustom,
	}
	if qid := strings.TrimSpace(req.QuestionID); qid != "" {
		// generated：question_id 必须能在父简报 research_questions 里解析出文本。
		var brief boardBriefSectorRef
		if err := json.Unmarshal(parent.Sectors, &brief); err != nil {
			respondError(c, http.StatusBadRequest, "briefing result sectors unreadable")
			return
		}
		text := ""
		for _, rq := range brief.ResearchQuestions {
			if strings.TrimSpace(rq.ID) == qid {
				text = strings.TrimSpace(rq.Question)
				break
			}
		}
		if text == "" {
			respondError(c, http.StatusBadRequest, "question_id not found in briefing research_questions: "+qid)
			return
		}
		question.ID = qid
		question.Text = text
		question.Source = service.QuestionSourceGenerated
	} else if question.Text == "" {
		respondError(c, http.StatusBadRequest, "question or question_id is required")
		return
	}

	parentID := uint(req.BriefingResultID)
	st, err := h.analysis.Start(AnalysisScopeBoard, boardID, AnalysisJobKindBoardInvestigation, analysisJobTimeout, func(ctx context.Context) (uint, error) {
		output, err := h.orchestrator.InvestigateBoardQuestion(ctx, boardID, parentID, question)
		if err != nil {
			return 0, err
		}
		return output.Result.ID, nil
	})
	if err != nil {
		var runErr *RunningJobError
		if errors.As(err, &runErr) {
			respondErrorWithData(c, http.StatusConflict, "board analysis already running", runErr.Current)
			return
		}
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}
	respondAccepted(c, gin.H{
		"status":    "started",
		"job_id":    st.JobID,
		"job_kind":  AnalysisJobKindBoardInvestigation,
		"scope":     AnalysisScopeBoard,
		"target_id": boardID,
	})
}

// listBoardResults returns the board-scope analysis history (newest first).
// GET /semantic-boards/:id/enrichment/analysis/results[?kind=]
// kind ∈ board_brief|board_investigation|legacy_board_analysis；缺省 = 全部 board 档。
func (h *EnrichmentHandler) listBoardResults(c *gin.Context) {
	boardID, ok := parseBoardID(c)
	if !ok {
		return
	}
	var (
		results []repository.TopicEnrichmentResult
		err     error
	)
	switch kind := strings.TrimSpace(c.Query("kind")); kind {
	case "":
		results, err = h.repo.ListBoardEnrichmentResults(c.Request.Context(), boardID)
	case repository.ResultKindBoardBrief, repository.ResultKindBoardInvestigation, repository.ResultKindLegacyBoardAnalysis:
		results, err = h.repo.ListBoardEnrichmentResultsByKind(c.Request.Context(), boardID, kind)
	default:
		respondError(c, http.StatusBadRequest, "invalid kind filter: "+kind)
		return
	}
	if err != nil {
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}
	list := make([]gin.H, 0, len(results))
	for i := range results {
		list = append(list, serializeBoardResult(&results[i]))
	}
	respondOK(c, list)
}

// getBoardResult returns one board-scope result by id. Ownership is
// delegated to requireBoardResult: board owner AND analysis_scope=board —
// a raw/dirty row that carries the board's id but a topic scope stays a
// uniform 404, same rule as the board QA routes (6.x review hardening:
// repository list queries already filter scope; the by-id read path must
// not be the one place where a scope-mismatched row leaks).
func (h *EnrichmentHandler) getBoardResult(c *gin.Context) {
	result, ok := h.requireBoardResult(c)
	if !ok {
		return
	}
	respondOK(c, serializeBoardResult(result))
}

// serializeBoardResult shapes one board-scope result for the API (mirrors the
// topic trigger's shape; sectors stays raw JSON for the frontend scope branch).
func serializeBoardResult(r *repository.TopicEnrichmentResult) gin.H {
	return gin.H{
		"id":               r.ID,
		"analysis_scope":   r.AnalysisScope,
		"result_kind":      repository.EffectiveResultKind(r),
		"parent_result_id": r.ParentResultID,
		"question_key":     r.QuestionKey,
		"sectors":          tryParseJSON(r.Sectors),
		"tool_calls":       tryParseJSON(r.ToolCalls),
		"input_snapshot":   tryParseJSON(r.InputSnapshot),
		"session_id":       r.SessionID,
		"created_at":       r.CreatedAt,
	}
}

func parseBoardID(c *gin.Context) (uint, bool) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		respondError(c, http.StatusBadRequest, "invalid board id")
		return 0, false
	}
	return uint(id), true
}

// getAnalysisStatus reports async analysis jobs.
// GET /enrichment/analysis-status?job_id=<id>   — 精确查一个 job（含已完成的）；未知 job_id → 404
// GET /enrichment/analysis-status?scope=board|topic&id=<n> — 当前/最近任务（重进恢复；无任务 = idle）
func (h *EnrichmentHandler) getAnalysisStatus(c *gin.Context) {
	if jobID := strings.TrimSpace(c.Query("job_id")); jobID != "" {
		st, ok := h.analysis.StatusByJobID(jobID)
		if !ok {
			respondError(c, http.StatusNotFound, "unknown job_id")
			return
		}
		respondOK(c, st)
		return
	}
	scope := c.Query("scope")
	if scope != AnalysisScopeBoard && scope != AnalysisScopeTopic {
		respondError(c, http.StatusBadRequest, "scope must be board or topic")
		return
	}
	id, err := strconv.ParseUint(c.Query("id"), 10, 64)
	if err != nil || id == 0 {
		respondError(c, http.StatusBadRequest, "invalid id")
		return
	}
	st, ok := h.analysis.Status(scope, uint(id))
	if !ok {
		// Never triggered (or process restarted): idle, nothing running.
		respondOK(c, gin.H{"scope": scope, "target_id": id, "running": false, "finished": false})
		return
	}
	respondOK(c, st)
}

// containsNotEnabled keeps the 4xx mapping local (no strings import at call site).
func containsNotEnabled(msg string) bool {
	return strings.Contains(msg, "not enabled")
}
