package repository

import (
	"context"
	"fmt"
	"time"
)

// ── 旧研究历史只读列表（redesign-enrichment-research-workbench D2 GET /history）
//
// 按 board 或所属 topic JOIN 汇总可验证归属的旧 result，id DESC 排他游标分
// 页。纯读取：无 LLM、无编排调用、不改任何行（spec「旧研究历史只读访问」：
// 所有历史读取零 LLM，signal_report 不混入历史集合，含已归档话题）。

// LegacyHistoryRow is one read-only list row: identity only — no sectors
// payload (列表不带正文，正文走既有 GET /results/:rid 等详情接口，D2 不新增
// 一套历史详情 API).
type LegacyHistoryRow struct {
	ID                uint      `json:"id"`
	ResultKind        string    `json:"result_kind"`
	AnalysisScope     string    `json:"analysis_scope"`
	PersistentTopicID *uint     `json:"persistent_topic_id"`
	SemanticBoardID   *uint     `json:"semantic_board_id"`
	CreatedAt         time.Time `json:"created_at"`
	SessionID         string    `json:"session_id"`
}

// ValidLegacyHistoryKind reports whether kind is one of the four legacy kinds
// the history list may serve. signal_report is deliberately absent: new
// reports have their own list (GET /signal-reports) and must never surface
// here.
func ValidLegacyHistoryKind(kind string) bool {
	switch kind {
	case ResultKindTopicAnalysis, ResultKindBoardBrief, ResultKindBoardInvestigation, ResultKindLegacyBoardAnalysis:
		return true
	}
	return false
}

// ListLegacyResults returns the board's verifiable legacy results, newest id
// first, keyed by an exclusive result-id cursor (beforeID, 0 = head); limit
// normalizes to the shared default 20 / cap 100. Two disjoint ownership
// branches (spec「旧研究历史只读访问」）:
//
//   - board scope: board_brief / board_investigation / legacy_board_analysis
//     rows stamped with the board id;
//   - topic scope: topic_analysis rows whose persistent topic belongs to the
//     board — archived topics included (no status filter; 归属按 topic 的板块
//     归属验证，不查今天 active 泳道). Topics deleted from the board drop out
//     naturally: 无法验证归属不猜回填.
//
// kind optionally filters one kind ("" = all four); topicID optionally narrows
// to one topic's rows. The `<> signal_report` predicate is belt-and-braces —
// none of the four kinds is signal_report, so the branches above can never
// match one anyway.
func (r *Repository) ListLegacyResults(ctx context.Context, boardID uint, kind string, topicID uint, beforeID uint, limit int) ([]LegacyHistoryRow, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	if kind != "" && !ValidLegacyHistoryKind(kind) {
		return nil, fmt.Errorf("invalid legacy history kind %q", kind)
	}
	q := r.db.WithContext(ctx).
		Table("topic_enrichment_result AS tr").
		Select("tr.id, tr.result_kind, tr.analysis_scope, tr.persistent_topic_id, tr.semantic_board_id, tr.created_at, tr.session_id").
		Where("tr.result_kind <> ?", ResultKindSignalReport).
		Where(`(
			tr.analysis_scope = 'board' AND tr.semantic_board_id = ?
				AND tr.result_kind IN (?, ?, ?)
		) OR (
			tr.analysis_scope = 'topic' AND tr.result_kind = ?
				AND tr.persistent_topic_id IN (
					SELECT id FROM board_persistent_topics WHERE semantic_board_id = ?
				)
		)`, boardID, ResultKindBoardBrief, ResultKindBoardInvestigation, ResultKindLegacyBoardAnalysis,
			ResultKindTopicAnalysis, boardID)
	if kind != "" {
		q = q.Where("tr.result_kind = ?", kind)
	}
	if topicID > 0 {
		q = q.Where("tr.persistent_topic_id = ?", topicID)
	}
	if beforeID > 0 {
		q = q.Where("tr.id < ?", beforeID)
	}
	var rows []LegacyHistoryRow
	err := q.Order("tr.id DESC").Limit(limit).Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("list legacy results: %w", err)
	}
	return rows, nil
}
