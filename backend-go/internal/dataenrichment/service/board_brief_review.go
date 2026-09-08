package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"gorm.io/gorm"

	"syntopica-backend/internal/dataenrichment/repository"
	"syntopica-backend/internal/platform/airouter"
	"syntopica-backend/internal/platform/logging"
)

// ── board_brief review judge（tasks 3.5，design D11 / spec「review 按结果种类隔离」）──
//
// 契约：
//   - 第一份 board_brief 不跑 judge；第二份起只找同 board、kind=board_brief
//     的上一份（legacy_board_analysis / investigation / topic 永不参与）。
//   - 比较内容仅限简报字段 summary/observations/relationships/uncertainties/
//     lane_refs（lane_refs 只为理解语境），绝不读 thesis/argument/depth——
//     boardBriefComparableFields 是唯一进入 prompt 的载荷构造器。
//   - prompt 明确允许“没有值得记录的认知更新”（should_review=false），不为
//     输出而编造更新。
//   - judge 认为有更新才写 TopicEnrichmentReview（Prev=上一份 brief、
//     Curr=当前 brief、board 所有权、topic id nil）；无更新不写。
//   - 全程 non-fatal：judge/写库失败只记日志，已落库简报不回滚、
//     topic_lifeline_context 永不回写（业务红线 #1/#21）。
//   - 已 apply 的同 kind review digest 在生成下一份简报【前】读取注入
//     （boardBriefReviewDigest），只作偏差提醒不当本次事实。

// boardBriefReviewOperation reuses the shared review-judge operation: the
// board-brief comparison is the same cognitive-audit role as the topic one
// and stays queryable under one operation, grouped by the board session id
// (ai-logging.md「数据增强 SessionID 规则」).
const boardBriefReviewOperation = "data_enrichment.review_judge"

// boardBriefReviewPrompt compares two board_brief snapshots. It deliberately
// speaks only the brief vocabulary (observations/relationships/uncertainties)
// — never thesis/argument/depth — and explicitly allows "nothing worth
// recording".
const boardBriefReviewPrompt = `你是一位 AI 系统质量审计员。比较同一语义版块先后生成的两份【版块简报】（board_brief），判断本次简报相对上一份是否出现了值得记录的认知更新。

两份简报都是对板块内部新闻记忆的观察汇总，字段含义：
- summary：板块概览（用于理解语境）
- observations：按泳道的关键观察（新增或消失的观察是判断重点）
- relationships：跨泳道关系（类型与置信度变化是判断重点）
- uncertainties：不确定项（重要未知项被解决或新增也算变化）
- lane_refs：泳道引用（仅为理解语境，本身不需要比较）

判断标准：
- should_review=true：出现了新的重要观察、上一份的观察或关系被推翻/消失、关系类型或置信度发生实质变化、重要不确定项被解决或明显新增
- should_review=false：只有措辞调整、顺序变化、证据补充等无实质认知变化的情况——「没有值得记录的认知更新」是完全正常的结果，如实返回 false，不要为了产出而编造更新

字段填写要求：
- new_findings：本次【新出现】的重要观察（上一份没有的）
- overturned：本次【消失或被推翻】的上一份观察/关系
- confidence_shift：关系置信变化，每项 {"relation":"关系简述","from":"low|medium|high","to":"low|medium|high"}
- affected_context：固定留空字符串
- reason：一句话说明为什么值得/不值得记录

输出严格 JSON（不要其他内容）：
{"should_review": true/false, "reason": "...", "new_findings": ["新观察1"], "overturned": ["被推翻的旧观察/关系"], "confidence_shift": [{"relation":"...","from":"low","to":"medium"}], "affected_context": "", "confidence": 0.0-1.0}

---
上一份简报:
%s

本次简报:
%s`

// boardBriefComparableFields projects one board_brief sectors payload down to
// the ONLY fields the review judge may see: summary, observations,
// relationships, uncertainties, lane_refs. Anything else (a legacy
// thesis/argument/depth shape, stray keys) is dropped. Returns nil when the
// payload is unreadable or lacks a summary — the judge then skips instead of
// comparing an empty shell (hard guard: legacy thesis never reaches the
// prompt).
func boardBriefComparableFields(sectors json.RawMessage) json.RawMessage {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(sectors, &raw); err != nil {
		return nil
	}
	if _, ok := raw["summary"]; !ok {
		return nil // not a brief-shaped payload — refuse to compare
	}
	out := make(map[string]json.RawMessage, 5)
	for _, field := range []string{"summary", "observations", "relationships", "uncertainties", "lane_refs"} {
		if v, ok := raw[field]; ok {
			out[field] = v
		}
	}
	b, err := json.Marshal(out)
	if err != nil {
		return nil
	}
	return b
}

// runBoardBriefReviewJudge executes the brief-vs-brief judge LLM call via
// airouter (unified ai_call_logs, board session id). Reuses ReviewJudgeOutput
// and the shared parser — only the prompt differs from the topic judge.
func (o *OrchestratorService) runBoardBriefReviewJudge(ctx context.Context, sessionID string, prevJSON, currJSON json.RawMessage) (*ReviewJudgeOutput, error) {
	prompt := fmt.Sprintf(boardBriefReviewPrompt, string(prevJSON), string(currJSON))
	resp, err := o.airouter.Chat(ctx, airouter.ChatRequest{
		Capability:  o.capability,
		Operation:   boardBriefReviewOperation,
		SessionID:   sessionID,
		Messages:    []airouter.Message{{Role: "user", Content: prompt}},
		Temperature: floatPtr(0.2),
		JSONMode:    true,
	})
	if err != nil {
		return nil, fmt.Errorf("board brief review judge chat: %w", err)
	}
	parsed, err := ParseJSONResponse(resp.Content)
	if err != nil {
		return nil, fmt.Errorf("board brief review judge parse: %w", err)
	}
	return parseReviewJudgeOutput(parsed), nil
}

// judgeBoardBriefAgainstPrev runs the same-kind review judge for a freshly
// persisted brief: find the previous board_brief of the same board (kind-
// isolated query), compare brief fields only, write one review row when the
// judge signals a cognitive update. Every failure path is non-fatal — the
// persisted brief stays untouched and nil is returned.
func (o *OrchestratorService) judgeBoardBriefAgainstPrev(ctx context.Context, boardID uint, sessionID string, current *repository.TopicEnrichmentResult) *repository.TopicEnrichmentReview {
	prev, err := o.repo.GetPrevLatestBoardEnrichmentResultByKind(ctx, boardID, repository.ResultKindBoardBrief, current.ID)
	if err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			// Real DB error — log and skip review (first brief = normal skip).
			logging.Warnf("enrich board %d: get prev brief for review judge: %v", boardID, err)
		}
		return nil
	}

	prevPayload := boardBriefComparableFields(prev.Sectors)
	currPayload := boardBriefComparableFields(current.Sectors)
	if prevPayload == nil || currPayload == nil {
		logging.Infof("enrich board %d: prev/current brief not brief-shaped, skip review judge", boardID)
		return nil
	}

	rj, err := o.runBoardBriefReviewJudge(ctx, sessionID, prevPayload, currPayload)
	if err != nil {
		// Non-fatal discipline: log, keep the persisted brief, no review row.
		logging.Warnf("enrich board %d: board brief review judge failed (non-fatal): %v", boardID, err)
		return nil
	}
	if rj == nil || !rj.ShouldReview {
		return nil
	}

	conf := rj.Confidence
	verdictJSON, _ := json.Marshal(map[string]any{
		"new_findings":     rj.NewFindings,
		"overturned":       rj.Overturned,
		"confidence_shift": rj.ConfidenceShift,
	})
	review := &repository.TopicEnrichmentReview{
		SemanticBoardID:  repository.BoardIDPtr(boardID),
		PrevResultID:     uintPtr(prev.ID), // BaseResultID = previous board_brief
		CurrResultID:     current.ID,       // NewResultID = current board_brief
		Verdict:          verdictJSON,
		DeviationSummary: rj.Reason,
		// Hard guard: affected_context is a varchar(10) column — a model that
		// ignores the prompt's“留空”instruction would fail the whole INSERT and
		// lose the review row. Board briefs have no lifeline granularity, so
		// the field is unconditionally emptied (model value never trusted).
		AffectedContext: "",
		Confidence:      &conf,
		Applied:         false,
		Source:          "llm_assisted",
	}
	if err := o.repo.CreateTopicEnrichmentReview(ctx, review); err != nil {
		// Non-fatal: the immutable brief is already persisted; a failed review
		// write must not surface the run as failed.
		logging.Warnf("enrich board %d: save board brief review (non-fatal): %v", boardID, err)
		return nil
	}
	return review
}

// boardBriefReviewDigest renders applied same-kind (board_brief) reviews as
// the advisory「历史认知提醒」block for the NEXT brief (D11: 只作偏差提醒，
// 不当本次事实，也不当方法卡/外部证据). Latest first, bounded so an applied
// review history can never blow the brief prompt budget.
func (o *OrchestratorService) boardBriefReviewDigest(ctx context.Context, boardID uint) (string, error) {
	reviews, err := o.repo.ListAppliedBoardEnrichmentReviewsByKind(ctx, boardID, repository.ResultKindBoardBrief)
	if err != nil {
		return "", err
	}
	if len(reviews) == 0 {
		return "", nil
	}
	const maxReviews = 5
	const maxRunes = 2000
	var b strings.Builder
	used := 0
	for i, r := range reviews {
		if i >= maxReviews {
			b.WriteString("- […更早的复盘提醒已截断]\n")
			break
		}
		line := fmt.Sprintf("- [review #%d] %s\n", r.ID, strings.TrimSpace(r.DeviationSummary))
		if used+len([]rune(line)) > maxRunes {
			b.WriteString("- […更早的复盘提醒已截断]\n")
			break
		}
		b.WriteString(line)
		used += len([]rune(line))
	}
	return b.String(), nil
}
