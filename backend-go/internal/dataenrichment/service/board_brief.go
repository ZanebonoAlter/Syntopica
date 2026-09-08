package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"syntopica-backend/internal/dataenrichment/repository"
	"syntopica-backend/internal/platform/airouter"
	"syntopica-backend/internal/platform/logging"
)

// ── board_brief：版块简报（design D1/D3，tasks 3.1-3.4 / M2/M3）──────────────
//
// 简报回答"发生了什么、是否有关、哪些看不清"，不预设板块存在统一因果系统：
// 关系仅在素材支持时声明（枚举 common_driver/possible_causal/divergent/
// context_only/unclear），"暂未发现统一关系""多个并行趋势""全 sparse"都是正常
// 结果。输入只有补全后的态势卡 + 同 kind review 摘要（3.5 接入前恒空），
// 禁止 web_search/fetch_page 工具、方法卡全文与旧作者画像注入。
//
// 输出韧性：坏 JSON 或结构不合格 → 纠错重试一次 → 仍坏机械降级为按质量排序
// 的观察清单（不造关系/问题），主流程始终持久化 brief。

// boardBriefOperation is the ai_call_logs operation for the brief LLM call.
const boardBriefOperation = "data_enrichment.board_brief"

// Relation type enum (D1). 同期发生 MUST NOT 自动等同因果。
const (
	RelationCommonDriver   = "common_driver"
	RelationPossibleCausal = "possible_causal"
	RelationDivergent      = "divergent"
	RelationContextOnly    = "context_only"
	RelationUnclear        = "unclear"
)

// Output count caps (D3): 关键观察 5 / 关系 6 / 研究问题 4。uncertainties 只
// 按 rune 截断不按数量截（D3 未定数量上限）。
const (
	boardBriefMaxObservations  = 5
	boardBriefMaxRelationships = 6
	boardBriefMaxQuestions     = 4
)

// Output rune caps (机械裁剪，防止把态势卡换一种形式全量复述)。
const (
	boardBriefMaxSummaryRunes   = 500
	boardBriefMaxStatementRunes = 300
	boardBriefMaxBasisRunes     = 200
	boardBriefMaxExplainRunes   = 300
	boardBriefMaxQuestionRunes  = 200
	boardBriefMaxNoteRunes      = 150
)

// BoardBriefPayload is the sectors jsonb shape for result_kind=board_brief
// (D3). Unlike the legacy thesis/argument/depth shape there is no forced
// thesis, no fixed mechanism layers, no historical analogy requirement.
type BoardBriefPayload struct {
	Scope             string                   `json:"scope"`       // always "board"
	ResultKind        string                   `json:"result_kind"` // always "board_brief"
	Summary           string                   `json:"summary"`
	Observations      []BoardBriefObservation  `json:"observations"`
	Relationships     []boardBriefRelationship `json:"relationships"`
	Uncertainties     []boardBriefUncertainty  `json:"uncertainties"`
	ResearchQuestions []boardBriefQuestion     `json:"research_questions"`
	LaneRefs          []laneRef                `json:"lane_refs"`
	// CrossBoardRelations is the SERVER-ASSEMBLED list of confirmed, unexpired
	// cross-board relations consumed as background context for this brief
	// (add-evidence-backed-cross-board-relations 5.x). The LLM never
	// generates it and it never enters the relationships lane validation.
	CrossBoardRelations []boardBriefCrossRelation `json:"cross_board_relations"`
	Degraded            bool                      `json:"degraded,omitempty"`      // mechanical fallback used
	DegradedWhy         string                    `json:"degraded_why,omitempty"`  // last parse/LLM failure
	RetryReason         string                    `json:"retry_reason,omitempty"`  // why attempt 1 was rejected
	AllSparse           bool                      `json:"all_sparse,omitempty"`    // all lanes lacked material
	ReviewDigestUsed    string                    `json:"review_digest,omitempty"` // 3.5 wiring: same-kind digest injected
}

// BoardBriefObservation is one lane-anchored observation. Observations are
// internal news-memory reads, NOT externally verified facts; basis must point
// back to the lane's material and as-of date.
type BoardBriefObservation struct {
	ID        string `json:"id"`
	LaneID    uint   `json:"lane_id"`
	Statement string `json:"statement"`
	Basis     string `json:"basis"`
	AsOfDate  string `json:"as_of_date"`
}

// boardBriefRelationship is one cross-lane relation. Requires ≥2 distinct
// active lanes; possible_causal is capped at medium confidence (insufficient
// causal evidence must never read as high).
type boardBriefRelationship struct {
	LaneIDs      []uint   `json:"lane_ids"`
	Type         string   `json:"type"`
	Explanation  string   `json:"explanation"`
	Confidence   string   `json:"confidence"` // low|medium|high
	EvidenceRefs []string `json:"evidence_refs"`
}

// boardBriefUncertainty is one open unknown.
type boardBriefUncertainty struct {
	Question       string `json:"question"`
	WhyUncertain   string `json:"why_uncertain"`
	NeededEvidence string `json:"needed_evidence"`
}

// boardBriefQuestion is one candidate research question (0-4). Questions must
// be concrete and evidence-answerable, never abstract method names.
type boardBriefQuestion struct {
	ID             string `json:"id"`
	Question       string `json:"question"`
	Rationale      string `json:"rationale"`
	RelatedLaneIDs []uint `json:"related_lane_ids"`
}

// boardBriefCrossRelation is the mechanical cross-board relation reference
// the server attaches to a brief (never LLM-generated). RelationID points at
// the adjudicated cross_board_relations row for traceability.
type boardBriefCrossRelation struct {
	RelationID    uint   `json:"relation_id"`
	OtherBoardID  uint   `json:"other_board_id"`
	Direction     string `json:"direction"` // outgoing (this board is source) | incoming (this board is target)
	RelationType  string `json:"relation_type"`
	Claim         string `json:"claim"`
	QualityGrade  string `json:"quality_grade"`
	ConfirmedAt   string `json:"confirmed_at,omitempty"`
	ExpiresAt     string `json:"expires_at,omitempty"`
	EvidenceURL   string `json:"evidence_url,omitempty"`
	EvidenceQuote string `json:"evidence_quote,omitempty"`
}

// boardBriefInput carries everything the brief is allowed to see.
type boardBriefInput struct {
	SessionID string
	CardsMD   string // rendered situation cards
	// ReviewDigest is the same-kind (board_brief) review summary reserved for
	// task 3.5; empty until the review judge wiring lands. It is advisory
	// context only ("曾经哪里判断失误"), never this run's facts.
	ReviewDigest string
	AllSparse    bool                // every lane card signalled thin/no material
	Cards        []LaneSituationCard // active lane whitelist + fallback source
	// CrossRelationsMD is the rendered "confirmed external relation
	// background" block (budgeted, deterministic order); empty = none.
	CrossRelationsMD string
	// CrossRelations are the selected relation refs frozen into the output
	// payload and snapshot (server-side mechanical assembly).
	CrossRelations []boardBriefCrossRelation
}

// boardBriefPromptInput snapshots the deterministic prompt inputs.
type boardBriefPromptInput struct {
	CardsMD      string `json:"cards_md"`
	ReviewDigest string `json:"review_digest"`
	AllSparse    bool   `json:"all_sparse"`
	// CrossRelationsMD freezes the exact background block injected into the
	// prompt; TruncatedCrossRelations counts relations dropped by the budget.
	CrossRelationsMD        string `json:"cross_relations_md,omitempty"`
	TruncatedCrossRelations int    `json:"truncated_cross_relations,omitempty"`
}

// boardBriefGenerationMeta records how the persisted brief was produced
// (attempts / retry / degradation) so input_snapshot can replay it.
type boardBriefGenerationMeta struct {
	Attempts    int    `json:"attempts"` // LLM calls made (1 or 2)
	Degraded    bool   `json:"degraded"`
	RetryReason string `json:"retry_reason,omitempty"`
	DegradedWhy string `json:"degraded_why,omitempty"`
}

// boardBriefInputSnapshot is the result's input_snapshot payload: enough to
// rebuild the cards, the freshness outcome, the prompt inputs and the
// generation path (retry/degrade) of this brief.
type boardBriefInputSnapshot struct {
	Cards        []LaneSituationCard       `json:"cards"`
	Freshness    *FreshnessGateReport      `json:"freshness"`
	PromptInputs boardBriefPromptInput     `json:"prompt_inputs"`
	Generation   *boardBriefGenerationMeta `json:"generation"`
}

// boardBriefPrompt is the brief contract. Deliberately free of tool
// descriptions, method-card content, thesis/反转句式/机制层/历史类比
// requirements (M2.8).
const boardBriefPrompt = `你是一位务实的新闻板块情报编辑。基于下方「泳道态势卡」（系统内部新闻记忆的压缩摘要，不是外部核验事实），为该板块生成一份可快速扫描的版块简报。

简报只回答三件事：最近发生了什么变化？哪些泳道之间可能有关？哪些还看不清？

硬性纪律：
1. 只依据态势卡中出现过的素材做观察；每条观察必须指向一条泳道（lane_id 只能取态势卡中列出的泳道编号），写明依据（basis：来自该泳道什么事实）与截止时间（as_of_date，取该卡最近日期）
2. 跨泳道关系必须谨慎：只有素材支持时才声明；同期发生不等于因果。关系类型枚举：common_driver（可能共同驱动）/ possible_causal（可能因果传导；置信最多 medium，缺失的中间环节要写进 uncertainties）/ divergent（方向分化）/ context_only（仅同板块背景相关，无共同驱动或传导证据）/ unclear（尚无法判断）
3. 找不到统一关系是正常结论：如实写"暂未发现统一关系"；存在多个并行趋势时分别展示，不要压缩成一个底层原因
4. 用直白语言陈述观察，就事论事；简报不需要宏大命题、反转句式或固定的层层递进结构，也不需要把板块装进更大的叙事框架
5. research_questions 给 0-4 个具体、可被证据支持或削弱的问题（不是抽象方法名）；没有值得调查的问题就返回空数组，空数组不是失败
6. 数量上限：observations 最多 5 条（挑最重要的）、relationships 最多 6 条、research_questions 最多 4 条；低质量泳道可少写或并入一句

输出严格 JSON（不要 markdown 包裹、不要任何其他文字）：
{"scope":"board","result_kind":"board_brief","summary":"1-3 句人话概览",
 "observations":[{"id":"o1","lane_id":0,"statement":"...","basis":"...","as_of_date":"YYYY-MM-DD"}],
 "relationships":[{"lane_ids":[1,2],"type":"unclear","explanation":"...","confidence":"low","evidence_refs":["o1"]}],
 "uncertainties":[{"question":"...","why_uncertain":"...","needed_evidence":"..."}],
 "research_questions":[{"id":"q1","question":"...","rationale":"...","related_lane_ids":[1,2]}],
 "lane_refs":[{"lane_id":0,"note":"该泳道在本简报中的角色"}]}`

const boardBriefSparseNote = "\n\n注意：该板块所有泳道的素材信号都很稀薄（无可观察事实）。简报如实说明素材不足即可：observations 与 relationships 返回空数组，不生成 research_questions，不要编造内容。"

// boardBriefRetryLead is the stable prefix of the corrective retry note
// (test anchor; the full note embeds the concrete failure reason).
const boardBriefRetryLead = "上次输出不是合法的版块简报"

const boardBriefRetryNote = "\n\n---\n" + boardBriefRetryLead + "（问题：%s）。请重新输出完整 JSON：严格遵循上述 schema、不要 markdown 代码块、不要额外文字；所有 lane_id 与 related_lane_ids 必须来自态势卡中列出的泳道编号。"

// assembleBoardBriefPrompt builds the brief prompt (pure function; M2.8
// contract is unit-tested against it).
func assembleBoardBriefPrompt(in boardBriefInput) string {
	prompt := boardBriefPrompt
	if in.AllSparse {
		prompt += boardBriefSparseNote
	}
	if d := strings.TrimSpace(in.ReviewDigest); d != "" {
		prompt += "\n\n---\n历史认知提醒（仅作偏差提醒，不作为本次事实）：\n" + d
	}
	if d := strings.TrimSpace(in.CrossRelationsMD); d != "" {
		prompt += "\n\n---\n已确认外部关系背景（此前人工确认的跨版块关系，非本次态势卡事实；summary 与 uncertainties 可参考，但不得据此生成本期 observations）：\n" + d
	}
	prompt += "\n\n---\n泳道态势卡：\n" + in.CardsMD
	return prompt
}

// parseBoardBrief validates and sanitizes one parsed LLM payload against the
// active card set. Returns ok=false for STRUCTURAL failures that warrant a
// corrective retry: missing summary, or zero valid observations while the
// board has material. Item-level violations (ghost lanes, illegal enums,
// dangling refs, over-cap counts) are scrubbed in place and never fail the
// whole payload.
func parseBoardBrief(parsed map[string]any, cards []LaneSituationCard, allSparse bool) (*BoardBriefPayload, bool) {
	active := make(map[uint]bool, len(cards))
	cardByLane := make(map[uint]LaneSituationCard, len(cards))
	for _, c := range cards {
		active[c.LaneID] = true
		cardByLane[c.LaneID] = c
	}

	brief := &BoardBriefPayload{
		Scope:               "board",
		ResultKind:          repository.ResultKindBoardBrief,
		Observations:        []BoardBriefObservation{},
		Relationships:       []boardBriefRelationship{},
		Uncertainties:       []boardBriefUncertainty{},
		ResearchQuestions:   []boardBriefQuestion{},
		LaneRefs:            []laneRef{},
		CrossBoardRelations: []boardBriefCrossRelation{},
		AllSparse:           allSparse,
	}
	brief.Summary = truncateRunes(strings.TrimSpace(getString(parsed, "summary")), boardBriefMaxSummaryRunes)
	if brief.Summary == "" {
		return nil, false
	}

	// Observations: ghost lanes and empty statements drop THAT item only.
	if raw, ok := parsed["observations"].([]any); ok {
		for _, v := range raw {
			m, ok := v.(map[string]any)
			if !ok {
				continue
			}
			laneID, ok := m["lane_id"].(float64)
			if !ok || !active[uint(laneID)] {
				if ok {
					logging.Warnf("board brief: observation lane %d not in active set — dropped (ghost reference)", uint(laneID))
				}
				continue
			}
			stmt := truncateRunes(strings.TrimSpace(getString(m, "statement")), boardBriefMaxStatementRunes)
			if stmt == "" {
				continue
			}
			obs := BoardBriefObservation{
				ID:        strings.TrimSpace(getString(m, "id")),
				LaneID:    uint(laneID),
				Statement: stmt,
				Basis:     truncateRunes(strings.TrimSpace(getString(m, "basis")), boardBriefMaxBasisRunes),
				AsOfDate:  strings.TrimSpace(getString(m, "as_of_date")),
			}
			brief.Observations = append(brief.Observations, obs)
			if len(brief.Observations) >= boardBriefMaxObservations {
				break
			}
		}
	}
	if allSparse {
		// 全板块无可观察素材：任何观察都是编造，机械丢弃（同 research_questions）。
		brief.Observations = []BoardBriefObservation{}
	}
	if !allSparse && len(brief.Observations) == 0 {
		return nil, false // material exists but nothing valid survived → retry
	}
	// Stable observation ids for evidence_refs (LLM-provided id wins).
	obsIDs := make(map[string]bool, len(brief.Observations))
	for i := range brief.Observations {
		if brief.Observations[i].ID == "" {
			brief.Observations[i].ID = fmt.Sprintf("o%d", i+1)
		}
		obsIDs[brief.Observations[i].ID] = true
	}

	// Relationships: enum / lane / explanation / evidence-ref rules (M3).
	// 全 sparse：素材不足时任何跨泳道关系都是编造，与 observations/
	// research_questions 同样机械清空——无论 lane_id 真实、枚举合法还是置信
	// 多高（防 LLM 无视 sparse 提示硬造关系）。
	if !allSparse {
		if raw, ok := parsed["relationships"].([]any); ok {
			for _, v := range raw {
				m, ok := v.(map[string]any)
				if !ok {
					continue
				}
				typ := getString(m, "type")
				if !boardBriefRelationTypes[typ] {
					logging.Warnf("board brief: relationship type %q illegal — entry dropped", typ)
					continue
				}
				explanation := truncateRunes(strings.TrimSpace(getString(m, "explanation")), boardBriefMaxExplainRunes)
				if explanation == "" {
					logging.Warnf("board brief: relationship without explanation — entry dropped")
					continue
				}
				laneIDs := scrubLaneIDs(m["lane_ids"], active)
				if len(laneIDs) < 2 {
					logging.Warnf("board brief: relationship fewer than 2 valid distinct lanes — entry dropped")
					continue
				}
				confidence := getString(m, "confidence")
				if !boardBriefConfidences[confidence] {
					confidence = "low"
				}
				// Dangling evidence refs scrubbed; refs must survive observations.
				var refs []string
				if rawRefs, ok := m["evidence_refs"].([]any); ok {
					for _, r := range rawRefs {
						if id, ok := r.(string); ok && obsIDs[strings.TrimSpace(id)] {
							refs = append(refs, strings.TrimSpace(id))
						}
					}
				}
				// M3.5: possible_causal without any valid evidence support
				// downgrades to unclear; it can never claim high confidence.
				if typ == RelationPossibleCausal {
					if len(refs) == 0 {
						typ = RelationUnclear
					}
					if confidence == "high" {
						confidence = "medium"
					}
				}
				brief.Relationships = append(brief.Relationships, boardBriefRelationship{
					LaneIDs: laneIDs, Type: typ, Explanation: explanation,
					Confidence: confidence, EvidenceRefs: refs,
				})
				if len(brief.Relationships) >= boardBriefMaxRelationships {
					break
				}
			}
		}
	}

	// Uncertainties: question required; others kept as written (rune-capped).
	if raw, ok := parsed["uncertainties"].([]any); ok {
		for _, v := range raw {
			m, ok := v.(map[string]any)
			if !ok {
				continue
			}
			q := truncateRunes(strings.TrimSpace(getString(m, "question")), boardBriefMaxQuestionRunes)
			if q == "" {
				continue
			}
			brief.Uncertainties = append(brief.Uncertainties, boardBriefUncertainty{
				Question:       q,
				WhyUncertain:   truncateRunes(strings.TrimSpace(getString(m, "why_uncertain")), boardBriefMaxQuestionRunes),
				NeededEvidence: truncateRunes(strings.TrimSpace(getString(m, "needed_evidence")), boardBriefMaxQuestionRunes),
			})
		}
	}

	// Research questions: question+rationale required, ghost lanes scrubbed;
	// all-sparse boards never carry questions (spec: 素材不足不生成研究问题).
	if !allSparse {
		if raw, ok := parsed["research_questions"].([]any); ok {
			for _, v := range raw {
				m, ok := v.(map[string]any)
				if !ok {
					continue
				}
				q := truncateRunes(strings.TrimSpace(getString(m, "question")), boardBriefMaxQuestionRunes)
				rationale := truncateRunes(strings.TrimSpace(getString(m, "rationale")), boardBriefMaxQuestionRunes)
				if q == "" || rationale == "" {
					continue
				}
				brief.ResearchQuestions = append(brief.ResearchQuestions, boardBriefQuestion{
					ID:             strings.TrimSpace(getString(m, "id")),
					Question:       q,
					Rationale:      rationale,
					RelatedLaneIDs: scrubLaneIDs(m["related_lane_ids"], active),
				})
				if len(brief.ResearchQuestions) >= boardBriefMaxQuestions {
					break
				}
			}
			for i := range brief.ResearchQuestions {
				if brief.ResearchQuestions[i].ID == "" {
					brief.ResearchQuestions[i].ID = fmt.Sprintf("q%d", i+1)
				}
			}
		}
	}

	brief.LaneRefs = briefLaneRefs(parsed["lane_refs"], brief.Observations, active, cardByLane)
	return brief, true
}

// boardBriefRelationTypes / boardBriefConfidences are the closed enums.
var boardBriefRelationTypes = map[string]bool{
	RelationCommonDriver:   true,
	RelationPossibleCausal: true,
	RelationDivergent:      true,
	RelationContextOnly:    true,
	RelationUnclear:        true,
}

var boardBriefConfidences = map[string]bool{"low": true, "medium": true, "high": true}

// scrubLaneIDs keeps only active, distinct lane ids in first-seen order.
func scrubLaneIDs(raw any, active map[uint]bool) []uint {
	list, ok := raw.([]any)
	if !ok {
		return nil
	}
	seen := map[uint]bool{}
	out := make([]uint, 0, len(list))
	for _, v := range list {
		id, ok := v.(float64)
		if !ok || !active[uint(id)] || seen[uint(id)] {
			continue
		}
		seen[uint(id)] = true
		out = append(out, uint(id))
	}
	return out
}

// briefLaneRefs sanitizes LLM-provided lane_refs (ghost drop) and, when the
// model returned none, derives them from the surviving observations so every
// referenced lane stays first-class and drillable.
func briefLaneRefs(raw any, observations []BoardBriefObservation, active map[uint]bool, cardByLane map[uint]LaneSituationCard) []laneRef {
	refs := []laneRef{}
	seen := map[uint]bool{}
	if list, ok := raw.([]any); ok {
		for _, v := range list {
			m, ok := v.(map[string]any)
			if !ok {
				continue
			}
			id, ok := m["lane_id"].(float64)
			if !ok || !active[uint(id)] || seen[uint(id)] {
				continue
			}
			seen[uint(id)] = true
			refs = append(refs, laneRef{LaneID: uint(id), Note: truncateRunes(strings.TrimSpace(getString(m, "note")), boardBriefMaxNoteRunes)})
		}
	}
	if len(refs) == 0 {
		for _, obs := range observations {
			if seen[obs.LaneID] {
				continue
			}
			seen[obs.LaneID] = true
			note := cardByLane[obs.LaneID].Label
			refs = append(refs, laneRef{LaneID: obs.LaneID, Note: truncateRunes(note, boardBriefMaxNoteRunes)})
		}
	}
	return refs
}

// mechanicalBoardBrief is the M2.6 fallback: honest, cards-only observations
// in quality order (cards arrive pre-sorted), no relationships, no research
// questions, one honest uncertainty per no-material lane. Quality scores
// order the list but never appear as evidence text (M3.6).
func mechanicalBoardBrief(cards []LaneSituationCard, allSparse bool, why string) *BoardBriefPayload {
	brief := &BoardBriefPayload{
		Scope:             "board",
		ResultKind:        repository.ResultKindBoardBrief,
		Observations:      []BoardBriefObservation{},
		Relationships:     []boardBriefRelationship{},
		Uncertainties:     []boardBriefUncertainty{},
		ResearchQuestions: []boardBriefQuestion{},
		LaneRefs:          []laneRef{},
		Degraded:          true,
		DegradedWhy:       why,
		AllSparse:         allSparse,
	}
	if allSparse {
		brief.Summary = "该板块所有泳道近期均无可观察素材，暂无法形成简报内容（自动简报生成不可用，已按素材不足如实记录）。"
		for _, c := range cards {
			brief.Uncertainties = append(brief.Uncertainties, boardBriefUncertainty{
				Question:       fmt.Sprintf("泳道《%s》近期动向如何？", c.Label),
				WhyUncertain:   "该泳道无可观察素材（事实摘要缺失）",
				NeededEvidence: "等待后续新闻命中或补充背景",
			})
		}
		return brief
	}
	brief.Summary = "自动简报生成不可用，以下为按质量排序的泳道观察清单（机械降级，未做跨泳道关系判断）。"
	for _, c := range cards {
		if c.FactsDigest == "" {
			brief.Uncertainties = append(brief.Uncertainties, boardBriefUncertainty{
				Question:       fmt.Sprintf("泳道《%s》近期动向如何？", c.Label),
				WhyUncertain:   "该泳道无可观察素材（事实摘要缺失）",
				NeededEvidence: "等待后续新闻命中或补充背景",
			})
			continue
		}
		if len(brief.Observations) >= boardBriefMaxObservations {
			break
		}
		brief.Observations = append(brief.Observations, BoardBriefObservation{
			ID:        fmt.Sprintf("o%d", len(brief.Observations)+1),
			LaneID:    c.LaneID,
			Statement: truncateRunes(c.FactsDigest, boardBriefMaxStatementRunes),
			Basis:     fmt.Sprintf("来源:%s 截止:%s", c.FactsSource, c.LastSeenDate),
			AsOfDate:  c.LastSeenDate,
		})
		brief.LaneRefs = append(brief.LaneRefs, laneRef{LaneID: c.LaneID, Note: truncateRunes(c.Label, boardBriefMaxNoteRunes)})
	}
	return brief
}

// generateBoardBrief runs the brief LLM call with one corrective retry, then
// degrades mechanically. Never fails: the caller always gets a persistable
// brief. All calls go through airouter (unified ai_call_logs).
func (o *OrchestratorService) generateBoardBrief(ctx context.Context, in boardBriefInput) (*BoardBriefPayload, *boardBriefGenerationMeta) {
	meta := &boardBriefGenerationMeta{}
	prompt := assembleBoardBriefPrompt(in)
	var lastErr error
	for attempt := 1; attempt <= 2; attempt++ {
		meta.Attempts = attempt
		attemptPrompt := prompt
		if attempt > 1 && lastErr != nil {
			attemptPrompt = prompt + fmt.Sprintf(boardBriefRetryNote, lastErr.Error())
		}
		resp, err := o.airouter.Chat(ctx, airouter.ChatRequest{
			Capability:  o.capability,
			Operation:   boardBriefOperation,
			SessionID:   in.SessionID,
			Messages:    []airouter.Message{{Role: "user", Content: attemptPrompt}},
			Temperature: floatPtr(0.3),
			JSONMode:    true,
		})
		if err != nil {
			lastErr = fmt.Errorf("board brief chat: %w", err)
			if attempt == 1 {
				meta.RetryReason = lastErr.Error()
			}
			continue
		}
		parsed, err := ParseJSONResponse(resp.Content)
		if err != nil {
			lastErr = fmt.Errorf("board brief parse: %w", err)
			if attempt == 1 {
				meta.RetryReason = lastErr.Error()
			}
			continue
		}
		brief, ok := parseBoardBrief(parsed, in.Cards, in.AllSparse)
		if !ok {
			lastErr = errors.New("board brief: invalid payload (summary missing or no valid observation)")
			if attempt == 1 {
				meta.RetryReason = lastErr.Error()
			}
			continue
		}
		brief.ReviewDigestUsed = strings.TrimSpace(in.ReviewDigest)
		// sectors.retry_reason 与 input_snapshot.generation.retry_reason 保持
		// 一致：第二次成功同样留痕（首次成功两者同空）。
		brief.RetryReason = meta.RetryReason
		return brief, meta
	}
	logging.Warnf("board brief: both attempts unusable (%v) — mechanical fallback", lastErr)
	brief := mechanicalBoardBrief(in.Cards, in.AllSparse, lastErr.Error())
	meta.Degraded = true
	meta.DegradedWhy = lastErr.Error()
	brief.RetryReason = meta.RetryReason
	return brief, meta
}

// persistBoardBriefResult stores the immutable board_brief snapshot. Atomic:
// the brief is fully assembled before the single Create; no half rows.
// Review is nil here by design — EnrichBoard runs the same-kind review
// judge after this call (non-fatal; the persisted brief is never rewritten).
func (o *OrchestratorService) persistBoardBriefResult(
	ctx context.Context, boardID uint, sessionID string,
	brief *BoardBriefPayload, meta *boardBriefGenerationMeta,
	promptIn boardBriefPromptInput, cards []LaneSituationCard,
	freshness *FreshnessGateReport,
) (*BoardEnrichmentOutput, error) {
	sectorsJSON, err := json.Marshal(brief)
	if err != nil {
		return nil, fmt.Errorf("enrich board %d: marshal brief: %w", boardID, err)
	}
	snapshot := boardBriefInputSnapshot{
		Cards:        cards,
		Freshness:    freshness,
		PromptInputs: promptIn,
		Generation:   meta,
	}
	snapJSON, err := json.Marshal(snapshot)
	if err != nil {
		return nil, fmt.Errorf("enrich board %d: marshal brief snapshot: %w", boardID, err)
	}
	result := &repository.TopicEnrichmentResult{
		SemanticBoardID: repository.BoardIDPtr(boardID),
		AnalysisScope:   "board",
		ResultKind:      repository.ResultKindBoardBrief,
		Sectors:         sectorsJSON,
		ToolCalls:       json.RawMessage("[]"), // brief never runs tools (D2)
		InputSnapshot:   snapJSON,
		SessionID:       sessionID,
	}
	if err := o.repo.CreateTopicEnrichmentResult(ctx, result); err != nil {
		return nil, fmt.Errorf("enrich board %d: save brief: %w", boardID, err)
	}
	return &BoardEnrichmentOutput{Result: result, Review: nil, Freshness: freshness}, nil
}

// BoardBriefInputForTest exposes the brief input shape to the external test
// package (3.5 review-digest wiring will construct it with a non-empty
// ReviewDigest).
type BoardBriefInputForTest = boardBriefInput

// GenerateBoardBriefForTest exposes the brief generator to tests.
func (o *OrchestratorService) GenerateBoardBriefForTest(ctx context.Context, in boardBriefInput) (*BoardBriefPayload, *boardBriefGenerationMeta) {
	return o.generateBoardBrief(ctx, in)
}
