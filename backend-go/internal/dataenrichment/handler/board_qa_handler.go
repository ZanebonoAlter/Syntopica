package handler

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"syntopica-backend/internal/dataenrichment/repository"
)

// ── 版块报告追问 QA（board-level-deep-analysis design D5 + tasks 6.2）───────
//
// POST /semantic-boards/:id/enrichment/analysis/results/:rid/qa               — 问一轮
// GET  /semantic-boards/:id/enrichment/analysis/results/:rid/qa               — 多轮历史
// POST /semantic-boards/:id/enrichment/analysis/results/:rid/qa/:qid/sediment — 沉淀一轮
//
// D5 契约：QA 继续按 result id 工作。三种 board kind（board_brief /
// board_investigation / legacy_board_analysis）均可追问——legacy 报告「只读」
// 指 result 行不可变（业务约束#2：result 不可变快照），QA 是独立的
// append-only 行（topic_enrichment_qa，source="qa"），允许追加，不改旧 JSON。
// 复用 topic QA 的 qaRunner.Ask(resultID, question) 与 QA 表——QA 归属 result
// 本身，与 scope/kind 无关。
//
// 所有权：每个入口都验证 board id + result 属主 + analysis_scope=board；
// sediment 额外验证 qa 行属于该 rid。跨 board / 跨 result / 不存在一律 404，
// 无旁路（错误信息不区分「不存在」与「不属于」，防枚举探测）。

// requireBoardResult loads the rid-addressed result and enforces board-route
// ownership: the row must belong to the path board AND be board-scoped.
// Cross-board rows, topic-scoped rows and unknown ids are uniform 404s.
func (h *EnrichmentHandler) requireBoardResult(c *gin.Context) (*repository.TopicEnrichmentResult, bool) {
	boardID, ok := parseBoardID(c)
	if !ok {
		return nil, false
	}
	rid, err := strconv.ParseUint(c.Param("rid"), 10, 64)
	if err != nil {
		respondError(c, http.StatusBadRequest, "invalid result id")
		return nil, false
	}
	result, err := h.repo.GetTopicEnrichmentResultByID(c.Request.Context(), uint(rid))
	if err != nil {
		respondError(c, http.StatusNotFound, "result not found")
		return nil, false
	}
	if !repository.BoardIDMatches(result.SemanticBoardID, boardID) || result.AnalysisScope != "board" {
		respondError(c, http.StatusNotFound, "result not found")
		return nil, false
	}
	return result, true
}

// askBoardQA runs one follow-up round against a board-scope result (any kind:
// brief / investigation / legacy) and returns the answer. The report itself is
// never modified; each round appends a topic_enrichment_qa row (source="qa").
// POST /semantic-boards/:id/enrichment/analysis/results/:rid/qa
// Body: { "question": "..." }
func (h *EnrichmentHandler) askBoardQA(c *gin.Context) {
	result, ok := h.requireBoardResult(c)
	if !ok {
		return
	}

	var req struct {
		Question string `json:"question" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "question is required")
		return
	}

	answer, err := h.qaRunner.Ask(c.Request.Context(), result.ID, req.Question)
	if err != nil {
		respondError(c, http.StatusInternalServerError, fmt.Sprintf("qa failed: %v", err))
		return
	}

	respondOK(c, answer)
}

// listBoardQA returns the multi-round follow-up history of a board-scope
// result, oldest first. QA rows are keyed by result id, so legacy reports
// keep their history exactly like new ones.
// GET /semantic-boards/:id/enrichment/analysis/results/:rid/qa
func (h *EnrichmentHandler) listBoardQA(c *gin.Context) {
	result, ok := h.requireBoardResult(c)
	if !ok {
		return
	}

	list, err := h.repo.ListTopicEnrichmentQAByResultID(c.Request.Context(), result.ID)
	if err != nil {
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}

	respondOK(c, list)
}

// sedimentBoardQA pins one follow-up round of this result as a durable note
// (sedimented=true). Ownership is double-checked: the result belongs to this
// board (requireBoardResult) AND the qa row belongs to this result — a qa id
// from another result (even same board) is a 404, not a redirect.
// Only flips the flag on the qa row; the report (topic_enrichment_result) is
// never rewritten (业务约束#2: result 不可变).
// POST /semantic-boards/:id/enrichment/analysis/results/:rid/qa/:qid/sediment
func (h *EnrichmentHandler) sedimentBoardQA(c *gin.Context) {
	result, ok := h.requireBoardResult(c)
	if !ok {
		return
	}

	qaID, ok := parseIDParam(c, "qid")
	if !ok {
		return
	}

	qa, err := h.repo.GetTopicEnrichmentQAByID(c.Request.Context(), qaID)
	if err != nil {
		respondError(c, http.StatusNotFound, "qa not found")
		return
	}
	if qa.TopicEnrichmentResultID != result.ID {
		respondError(c, http.StatusNotFound, "qa not found for this result")
		return
	}

	if err := h.repo.MarkQASedimented(c.Request.Context(), qaID); err != nil {
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}

	qa, err = h.repo.GetTopicEnrichmentQAByID(c.Request.Context(), qaID)
	if err != nil {
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}

	respondOK(c, qa)
}
