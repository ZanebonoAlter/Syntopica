package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"gorm.io/gorm"

	"syntopica-backend/internal/dataenrichment/repository"
	"syntopica-backend/internal/platform/airouter"
	"syntopica-backend/internal/platform/logging"
)

// ── board_investigation review judge（task 4.7，design D11 / M8.7/M8.8）─────
//
// 契约：
//   - 只有同一 semantic_board + 同一 parent_result_id + 同一 question_key
//     的重跑才互比（GetPrevBoardInvestigationByQuestion 严格链查询）；
//     首份/不同问题/不同父简报/别版块 → 0 judge、0 review。generated/custom
//     问题同用 ComputeQuestionKey（trim+空白折叠 hash），规范化文本相同即
//     同链。
//   - 比较内容仅限调查认知层：question、hypotheses 的评估状态与支持/反证/
//     缺口、conclusion、证据元数据（quote 摘录除外）——
//     boardInvestigationComparableFields 是唯一进入 prompt 的载荷构造器；
//     tool_calls、方法卡正文、父简报全文、thesis/argument/depth 永不进入。
//   - prompt 明确比较假设状态、支持/反证与证据缺口变化；明确允许
//     「没有值得记录的认知更新」（should_review=false），不预选赢家、
//     不评判哪次调查「更好」。
//   - judge 认为有更新才写 TopicEnrichmentReview（Prev=上一份同链调查、
//     Curr=当前调查、board 所有权、topic id nil、Applied=false、
//     AffectedContext 强制空）；无更新不写。
//   - 全程 non-fatal：chat/parse/写库失败只记日志，当前调查已落库的行
//     不回滚不改写，topic_lifeline_context 永不回写（业务红线 #1），
//     父简报与两份调查的 sectors/input_snapshot 均不可变。
//   - 复用共享 review-judge operation 与同一调查 session（airouter 统一
//     ai_call_logs，ai-logging.md「数据增强 SessionID 规则」）。

// boardInvestigationReviewOperation reuses the shared review-judge operation:
// the investigation comparison is the same cognitive-audit role as the
// topic/brief judges and stays queryable under one operation, grouped by the
// investigation session id.
const boardInvestigationReviewOperation = "data_enrichment.review_judge"

// boardInvestigationReviewPrompt compares two board_investigation snapshots
// of the SAME parent brief + question. It deliberately speaks only the
// investigation vocabulary (hypotheses assessment states, support/counter
// evidence, gaps, conclusion) and explicitly allows "nothing worth
// recording" — it never ranks the two runs or preselects a hypothesis winner.
const boardInvestigationReviewPrompt = `你是一位 AI 系统质量审计员。比较同一板块、同一父简报、同一调查问题先后完成的两次【板块深度调查】（board_investigation）结果，判断本次调查相对上一份是否出现了值得记录的认知更新。

两次调查都是对同一问题的竞争假设检验，字段含义：
- question：调查问题（同一问题的重跑，用于确认语境）
- hypotheses：最终竞争假设。assessment 五态（supported=证据充分支持 / plausible=初步支持但不足定论 / insufficient=支持与反证都不足 / weakened=被反证明显削弱 / refuted=被反证直接推翻）；confidence 三档（low|medium|high）；support_evidence / counter_evidence 是证据 id 引用；gaps 是诚实缺口；derived_from 标注假设改写来源
- conclusion：最终结论（summary / confidence / scope / boundary）
- evidence_chain：证据条目元数据（id、来源类型、机构/日期/链接、supports/counters 指向哪些假设）——用于判断证据增减与指向变化

判断标准（对照两次调查）：
- should_review=true：假设的 assessment 或 confidence 发生实质迁移（如 plausible→supported、plausible→refuted）、出现新假设或假设被合并/推翻（derived_from 变化）、同一假设的支持/反证证据发生实质增减、重要 gaps 被解决或明显新增、conclusion 的置信或边界发生实质变化
- should_review=false：仅措辞调整、证据等量替换、顺序变化——「没有值得记录的认知更新」是完全正常的结果，如实返回 false，不要为了产出而编造更新
- 只描述变化本身，不评判哪次调查「更好」，不预选假设赢家

字段填写要求：
- new_findings：本次【新出现】的假设结论或证据（上一份没有的）
- overturned：本次【被推翻、消失或明显降级】的上一份假设结论/证据指向
- confidence_shift：假设状态变化，每项 {"insight":"假设id或简述","from":"supported|plausible|insufficient|weakened|refuted","to":"supported|plausible|insufficient|weakened|refuted"}
- affected_context：固定留空字符串
- reason：一句话说明为什么值得/不值得记录

输出严格 JSON（不要其他内容）：
{"should_review": true/false, "reason": "...", "new_findings": ["新结论1"], "overturned": ["被推翻的旧结论"], "confidence_shift": [{"insight":"h1","from":"plausible","to":"supported"}], "affected_context": "", "confidence": 0.0-1.0}

---
上一份调查:
%s

本次调查:
%s`

// 载荷允许清单：假设条目十个白名单字段；证据条目元数据字段
// （quote 除外——逐字摘录是供前端核查的原文，比较评估状态用不上且会
// 撑爆 prompt 预算）。顶层四键（question/hypotheses/conclusion/
// evidence_chain）在 boardInvestigationComparableFields 内直接装配。
var (
	boardInvestigationHypothesisAllowed = []string{
		"id", "label", "is_null", "derived_from", "assessment",
		"confidence", "scope", "support_evidence", "counter_evidence", "gaps",
	}

	boardInvestigationEvidenceAllowed = []string{
		"id", "source_type", "ref", "url", "institution", "date",
		"kind", "lane_note", "supports", "counters",
	}
)

// boardInvestigationComparableFields projects one board_investigation sectors
// payload down to the ONLY fields the review judge may see. Anything else
// (method_refs, lane_refs, retry_reason, stray hypothesis/evidence keys,
// legacy thesis/argument/depth shapes, evidence quotes) is dropped. Returns
// nil when the payload is unreadable or not investigation-shaped (missing
// hypotheses array / hypotheses not a list / result_kind tag disagreeing) —
// the judge then skips instead of comparing an empty shell.
func boardInvestigationComparableFields(sectors json.RawMessage) json.RawMessage {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(sectors, &raw); err != nil {
		return nil
	}
	// Shape guards: must carry a hypotheses list; an explicit result_kind tag
	// (always written by the current chain) must agree — legacy/brief/topic
	// payloads die here even if a stray hypotheses key exists.
	hypRaw, ok := raw["hypotheses"]
	if !ok {
		return nil
	}
	var hyps []map[string]json.RawMessage
	if err := json.Unmarshal(hypRaw, &hyps); err != nil {
		return nil
	}
	if kindRaw, has := raw["result_kind"]; has {
		var kind string
		if err := json.Unmarshal(kindRaw, &kind); err != nil || kind != repository.ResultKindBoardInvestigation {
			return nil
		}
	}

	out := make(map[string]json.RawMessage, 4)
	if v, has := raw["question"]; has {
		out["question"] = v
	}
	out["hypotheses"] = projectRawObjectList(hyps, boardInvestigationHypothesisAllowed)
	if v, has := raw["conclusion"]; has {
		out["conclusion"] = v
	}
	if evRaw, has := raw["evidence_chain"]; has {
		var evs []map[string]json.RawMessage
		if err := json.Unmarshal(evRaw, &evs); err != nil {
			return nil // present but not a list = not the investigation shape
		}
		out["evidence_chain"] = projectRawObjectList(evs, boardInvestigationEvidenceAllowed)
	}
	b, err := json.Marshal(out)
	if err != nil {
		return nil
	}
	return b
}

// projectRawObjectList rebuilds each raw object keeping only allowlisted keys
// (present ones only — no empty-key backfill, so minimal payloads stay
// minimal and dropped fields never reappear).
func projectRawObjectList(items []map[string]json.RawMessage, allowed []string) json.RawMessage {
	projected := make([]map[string]json.RawMessage, 0, len(items))
	for _, item := range items {
		p := make(map[string]json.RawMessage, len(allowed))
		for _, key := range allowed {
			if v, has := item[key]; has {
				p[key] = v
			}
		}
		projected = append(projected, p)
	}
	b, err := json.Marshal(projected)
	if err != nil {
		return json.RawMessage("[]")
	}
	return b
}

// runBoardInvestigationReviewJudge executes the investigation-vs-investigation
// judge LLM call via airouter (unified ai_call_logs, investigation session
// id). Reuses ReviewJudgeOutput and the shared parser — only the prompt
// differs from the topic/brief judges.
func (o *OrchestratorService) runBoardInvestigationReviewJudge(ctx context.Context, sessionID string, prevJSON, currJSON json.RawMessage) (*ReviewJudgeOutput, error) {
	prompt := fmt.Sprintf(boardInvestigationReviewPrompt, string(prevJSON), string(currJSON))
	resp, err := o.airouter.Chat(ctx, airouter.ChatRequest{
		Capability:  o.capability,
		Operation:   boardInvestigationReviewOperation,
		SessionID:   sessionID,
		Messages:    []airouter.Message{{Role: "user", Content: prompt}},
		Temperature: floatPtr(0.2),
		JSONMode:    true,
	})
	if err != nil {
		return nil, fmt.Errorf("board investigation review judge chat: %w", err)
	}
	parsed, err := ParseJSONResponse(resp.Content)
	if err != nil {
		return nil, fmt.Errorf("board investigation review judge parse: %w", err)
	}
	return parseReviewJudgeOutput(parsed), nil
}

// judgeBoardInvestigationAgainstPrev runs the same-chain review judge for a
// freshly persisted investigation: find the previous board_investigation of
// the same board + parent brief + question key (strict chain query), compare
// the projected investigation fields only, write one review row when the
// judge signals a cognitive update. Every failure path is non-fatal — the
// persisted investigation stays untouched and nil is returned.
func (o *OrchestratorService) judgeBoardInvestigationAgainstPrev(ctx context.Context, boardID, parentBriefID uint, questionKey, sessionID string, current *repository.TopicEnrichmentResult) *repository.TopicEnrichmentReview {
	prev, err := o.repo.GetPrevBoardInvestigationByQuestion(ctx, boardID, parentBriefID, questionKey, current.ID)
	if err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			// Real DB error — log and skip review (first run = normal skip).
			logging.Warnf("board investigation %d: get prev by question for review judge: %v", boardID, err)
		}
		return nil
	}

	prevPayload := boardInvestigationComparableFields(prev.Sectors)
	currPayload := boardInvestigationComparableFields(current.Sectors)
	if prevPayload == nil || currPayload == nil {
		logging.Infof("board investigation %d: prev/current payload not investigation-shaped, skip review judge", boardID)
		return nil
	}

	rj, err := o.runBoardInvestigationReviewJudge(ctx, sessionID, prevPayload, currPayload)
	if err != nil {
		// Non-fatal discipline: log, keep the persisted investigation, no review row.
		logging.Warnf("board investigation %d: review judge failed (non-fatal): %v", boardID, err)
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
		PrevResultID:     uintPtr(prev.ID), // BaseResultID = previous same-chain investigation
		CurrResultID:     current.ID,       // NewResultID = current investigation
		Verdict:          verdictJSON,
		DeviationSummary: rj.Reason,
		// Hard guard: affected_context is a varchar(10) column — a model that
		// ignores the prompt's“留空”instruction would fail the whole INSERT and
		// lose the review row. Investigations have no lifeline granularity,
		// so the field is unconditionally emptied (model value never trusted).
		AffectedContext: "",
		Confidence:      &conf,
		Applied:         false,
		Source:          "llm_assisted",
	}
	if err := o.repo.CreateTopicEnrichmentReview(ctx, review); err != nil {
		// Non-fatal: the immutable investigation is already persisted; a failed
		// review write must not surface the run as failed.
		logging.Warnf("board investigation %d: save investigation review (non-fatal): %v", boardID, err)
		return nil
	}
	return review
}
