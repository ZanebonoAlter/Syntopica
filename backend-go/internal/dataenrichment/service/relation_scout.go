package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"syntopica-backend/internal/dataenrichment/repository"
	"syntopica-backend/internal/platform/airouter"
	"syntopica-backend/internal/platform/logging"
)

// ── Scout stage (design D1): source → search plan → raw evidence → candidates ──
//
// Two program-orchestrated LLM steps (no agent loop — the tool sequence is
// deterministic and cheap): plan queries, then extract relation candidates
// from the raw results. Model text NEVER binds internal ids (that is the
// resolver's job) and NEVER writes history by itself.

const (
	relationScoutPlanOperation    = "data_enrichment.relation_scout_plan"
	relationScoutExtractOperation = "data_enrichment.relation_scout_extract"
	relationVerifyPlanOperation   = "data_enrichment.relation_verify_plan"
	relationVerifyJudgeOperation  = "data_enrichment.relation_verify_judge"

	relationMaxQueries    = 4
	relationMaxCandidates = 3
)

const relationScoutPlanPrompt = `你是一名研究侦察员，为"跨版块关系发现"制定外部检索计划。你会收到一个内部来源观察（快照文本）。

任务：设计 2-4 条互不重复的中文搜索查询，从不同角度探查这个观察可能涉及的【外部】关联对象或机制：
- 直接影响角度（谁可能驱动了这个观察）
- 共同驱动角度（什么第三方因素可能同时驱动双方）
- 背景核实角度（该观察所处的事件脉络）

纪律：
1. 查询必须具体（含对象与事件词），不得只重复观察原文
2. 不得预设关系成立——查询是探查，不是求证
3. 只输出 JSON：{"queries":["..."]}`

const relationScoutExtractPrompt = `你是一名候选提取员。你会收到：来源观察、若干条外部搜索的原始结果（标题+URL+摘要）。任务：从原始材料中提取 0-3 条"候选关系"。

每条候选：
- target_concept：外部材料中提到的、可能与来源观察所在知识域有关的具体概念（中文实体名，如"日本国债收益率"、"全球通胀预期"）。只写概念本身，不写句子
- claim：一句话关系陈述（谁与谁、什么关系），必须能被材料支持
- relation_type 初判：causal(传导/因果) | common_driver(共同驱动) | divergence(分化) | correlated(仅相关) | contextual(背景补充) | unclear(不明确)
- mechanism：材料给出的作用机制（没有就留空，不得编造）
- evidence：1-3 条引用，每条 {"ref":"s1"(搜索结果的 ref 编号),"url":...,"title":...,"quote":逐字摘录原文片段(不得改写)}
- counter_evidence：材料中反对该关系的片段（如有），同上格式

硬性纪律：
1. quote 必须逐字摘自对应搜索结果原文，不得转述
2. 材料不支持的候选不要提——宁缺毋滥
3. 只输出 JSON：{"candidates":[{"target_concept":"","claim":"","relation_type":"","mechanism":"","evidence":[],"counter_evidence":[]}]}`

// scoutPlanOutput is the LLM plan response.
type scoutPlanOutput struct {
	Queries []string `json:"queries"`
}

// scoutCandidate is one extracted candidate (pre-resolve, pre-verify).
type scoutCandidate struct {
	TargetConcept   string                        `json:"target_concept"`
	Claim           string                        `json:"claim"`
	RelationType    string                        `json:"relation_type"`
	Mechanism       string                        `json:"mechanism"`
	Evidence        []repository.RelationEvidence `json:"evidence"`
	CounterEvidence []repository.RelationEvidence `json:"counter_evidence"`
}

type scoutExtractOutput struct {
	Candidates []scoutCandidate `json:"candidates"`
}

// scoutSearchResult is one executed web_search with a stable ref id (s1, s2…).
type scoutSearchResult struct {
	Ref     string            `json:"ref"`
	Query   string            `json:"query"`
	Results []WebSearchResult `json:"results"`
}

// relationScoutOutcome carries everything the later stages need.
type relationScoutOutcome struct {
	Searches   []scoutSearchResult
	Candidates []scoutCandidate
	RawByRef   map[string]string // ref → raw tool JSON (quote verification)
	Gaps       []relationEvidenceGap
	LLMCalls   int
}

// runRelationScout executes the scout stage. Budget failures degrade to gaps;
// total LLM/search failure returns an error only when nothing ran.
func (o *OrchestratorService) runRelationScout(ctx context.Context, sessionID, sourceText string, maxSearches int) (*relationScoutOutcome, error) {
	out := &relationScoutOutcome{RawByRef: map[string]string{}, Gaps: []relationEvidenceGap{}}

	// Step 1: plan queries.
	planResp, err := o.airouter.Chat(ctx, airouter.ChatRequest{
		Capability: o.capability,
		Operation:  relationScoutPlanOperation,
		SessionID:  sessionID,
		Messages: []airouter.Message{
			{Role: "system", Content: relationScoutPlanPrompt},
			{Role: "user", Content: "/no_think\n来源观察：\n" + sourceText},
		},
		Temperature: floatPtr(0.2),
		JSONMode:    true,
	})
	out.LLMCalls++
	if err != nil {
		out.Gaps = append(out.Gaps, relationEvidenceGap{Reason: "scout_plan_llm_error", Detail: err.Error()})
		return out, fmt.Errorf("relation scout plan: %w", err)
	}
	planParsed, parseErr := ParseJSONResponse(planResp.Content)
	if parseErr != nil {
		out.Gaps = append(out.Gaps, relationEvidenceGap{Reason: "scout_plan_parse_error"})
		return out, fmt.Errorf("relation scout plan parse: %s", prefix(planResp.Content, 120))
	}
	plan := scoutPlanOutput{}
	if b, mErr := json.Marshal(planParsed); mErr == nil {
		_ = json.Unmarshal(b, &plan)
	}
	if len(plan.Queries) == 0 {
		out.Gaps = append(out.Gaps, relationEvidenceGap{Reason: "scout_plan_empty"})
		return out, nil // honest no-op: no queries → no discovery
	}

	// Step 2: execute web_search per query (budget-capped).
	if maxSearches <= 0 {
		maxSearches = 1
	}
	executed := 0
	for _, q := range plan.Queries {
		if executed >= maxSearches {
			out.Gaps = append(out.Gaps, relationEvidenceGap{Reason: "search_budget_exhausted", Detail: q})
			continue
		}
		executed++
		results, sErr := o.toolRegistry.webSearcher.Search(ctx, q)
		if sErr != nil {
			out.Gaps = append(out.Gaps, relationEvidenceGap{Reason: "web_search_error", Detail: q + ": " + sErr.Error()})
			continue
		}
		ref := fmt.Sprintf("s%d", len(out.Searches)+1)
		sr := scoutSearchResult{Ref: ref, Query: q, Results: results}
		out.Searches = append(out.Searches, sr)
		if b, mErr := json.Marshal(map[string]any{"query": q, "results": results}); mErr == nil {
			out.RawByRef[ref] = string(b)
		}
	}
	if len(out.Searches) == 0 {
		out.Gaps = append(out.Gaps, relationEvidenceGap{Reason: "no_search_results"})
		return out, nil // Bocha down / zero hits: honest insufficient material
	}

	// Step 3: extract candidates from the raw results.
	resultsJSON, _ := json.Marshal(out.Searches)
	extractResp, err := o.airouter.Chat(ctx, airouter.ChatRequest{
		Capability: o.capability,
		Operation:  relationScoutExtractOperation,
		SessionID:  sessionID,
		Messages: []airouter.Message{
			{Role: "system", Content: relationScoutExtractPrompt},
			{Role: "user", Content: "/no_think\n来源观察：\n" + sourceText + "\n\n搜索结果（ref 编号供引用）：\n" + string(resultsJSON)},
		},
		Temperature: floatPtr(0.2),
		JSONMode:    true,
	})
	out.LLMCalls++
	if err != nil {
		out.Gaps = append(out.Gaps, relationEvidenceGap{Reason: "scout_extract_llm_error", Detail: err.Error()})
		return out, nil // degrade: keep searches, no candidates
	}
	extractParsed, parseErr := ParseJSONResponse(extractResp.Content)
	if parseErr != nil {
		out.Gaps = append(out.Gaps, relationEvidenceGap{Reason: "scout_extract_parse_error"})
		return out, nil
	}
	extract := scoutExtractOutput{}
	if b, mErr := json.Marshal(extractParsed); mErr == nil {
		_ = json.Unmarshal(b, &extract)
	}
	if len(extract.Candidates) > relationMaxCandidates {
		extract.Candidates = extract.Candidates[:relationMaxCandidates]
	}
	// Mechanically stamp refs are kept as-is; quote verification happens later.
	for _, c := range extract.Candidates {
		if strings.TrimSpace(c.TargetConcept) == "" || strings.TrimSpace(c.Claim) == "" {
			continue // structurally empty candidate
		}
		out.Candidates = append(out.Candidates, c)
	}
	if len(out.Candidates) == 0 {
		out.Gaps = append(out.Gaps, relationEvidenceGap{Reason: "no_candidates_extracted"})
	}
	return out, nil
}

// ── Verifier stage (design D5): blind, competing-explanations judge ─────────

const relationVerifyPlanPrompt = `你是独立验证计划员。你会收到一个待验证的关系假设（关系陈述+关系类型+机制）与其支持证据。任务：设计 1-2 条【反证/替代解释】检索查询——寻找不支持该关系、或支持其他解释（如共同第三方驱动、巧合相关）的材料。

纪律：
1. 查询必须独立成文，不得复述假设结论
2. 只输出 JSON：{"counter_queries":["..."]}`

const relationVerifyJudgePrompt = `你是独立验证员（盲验）：你没有参与此前的发现过程，看不到发现阶段的任何自评分。你会收到：来源观察、关系假设、支持证据（逐字引用）、反证检索的原始结果、替代解释清单。

必须同时评估四个竞争解释，选证据最站得住的一个：
- H1 因果传导：来源域与目标域之间存在直接作用链（claim 所述关系成立，类型 causal）
- H2 共同驱动：双方都受同一第三方因素驱动（common_driver）
- H3 仅相关/巧合：同时出现但无机制联系（correlated 或 contextual）
- H0 无关系：材料不足以建立任何联系

裁决纪律：
1. verdict 只能是 supported（假设的关系类型获证据支持）| contested（支持与反证相持）| insufficient（材料不足以裁决）| rejected（反证直接推翻）
2. relation_type 为终判类型（causal|common_driver|divergence|correlated|contextual|unclear）；若材料只支持共同驱动而不支持直接传导，MUST 输出 common_driver 而非 causal
3. mechanism 只有在材料明确给出时才填写，否则留空——不得编造机制
4. 证据不足就输出 insufficient，不强造赢家
5. 只输出 JSON：{"relation_type":"","verdict":"","mechanism":"","winning_hypothesis":"H1|H2|H3|H0","reasoning":"一句话","counter_summary":"反证要点"}`

// relationVerifyOutput is the judge's verdict payload.
type relationVerifyOutput struct {
	RelationType      string `json:"relation_type"`
	Verdict           string `json:"verdict"`
	Mechanism         string `json:"mechanism"`
	WinningHypothesis string `json:"winning_hypothesis"`
	Reasoning         string `json:"reasoning"`
	CounterSummary    string `json:"counter_summary"`
}

// relationVerifyInput is the blinded material set (no scout self-scores).
type relationVerifyInput struct {
	SourceText    string
	Claim         string
	ClaimedType   string
	ClaimedMech   string
	SupportMD     string // rendered raw support evidence
	CounterMD     string // rendered counter-search raw results
	InternalBrief string // resolved internal target material digest (may be empty)
}

// runRelationVerifier runs the two-step blind verification (plan counter
// queries → judge). Uses its OWN session id so scout context never leaks.
func (o *OrchestratorService) runRelationVerifier(ctx context.Context, sessionID string, in relationVerifyInput, maxSearches int) (*relationVerifyOutput, []scoutSearchResult, []relationEvidenceGap, int) {
	gaps := []relationEvidenceGap{}
	llmCalls := 0

	// Step 1: counter queries.
	counterMD := ""
	var counterSearches []scoutSearchResult
	rawExtra := map[string]string{}
	planResp, err := o.airouter.Chat(ctx, airouter.ChatRequest{
		Capability: o.capability,
		Operation:  relationVerifyPlanOperation,
		SessionID:  sessionID,
		Messages: []airouter.Message{
			{Role: "system", Content: relationVerifyPlanPrompt},
			{Role: "user", Content: "/no_think\n关系假设：\n" + in.Claim + "\n类型：" + in.ClaimedType + "\n机制：" + in.ClaimedMech + "\n\n支持证据：\n" + in.SupportMD},
		},
		Temperature: floatPtr(0.2),
		JSONMode:    true,
	})
	llmCalls++
	counterQueries := []string{}
	if err == nil {
		if parsed, pErr := ParseJSONResponse(planResp.Content); pErr == nil {
			if b, mErr := json.Marshal(parsed); mErr == nil {
				var plan struct {
					CounterQueries []string `json:"counter_queries"`
				}
				_ = json.Unmarshal(b, &plan)
				counterQueries = plan.CounterQueries
			}
		}
	} else {
		gaps = append(gaps, relationEvidenceGap{Reason: "verify_plan_llm_error", Detail: err.Error()})
	}
	if maxSearches <= 0 {
		maxSearches = 1
	}
	executed := 0
	for _, q := range counterQueries {
		if executed >= maxSearches {
			gaps = append(gaps, relationEvidenceGap{Reason: "counter_search_budget_exhausted", Detail: q})
			break
		}
		executed++
		results, sErr := o.toolRegistry.webSearcher.Search(ctx, q)
		if sErr != nil {
			gaps = append(gaps, relationEvidenceGap{Reason: "counter_search_error", Detail: q})
			continue
		}
		ref := fmt.Sprintf("c%d", len(counterSearches)+1)
		counterSearches = append(counterSearches, scoutSearchResult{Ref: ref, Query: q, Results: results})
		if b, mErr := json.Marshal(map[string]any{"query": q, "results": results}); mErr == nil {
			rawExtra[ref] = string(b)
		}
	}
	if len(counterSearches) > 0 {
		b, _ := json.Marshal(counterSearches)
		counterMD = string(b)
	} else if len(counterQueries) > 0 {
		gaps = append(gaps, relationEvidenceGap{Reason: "no_counter_results"})
	}

	// Step 2: blind judge.
	judgeUser := "/no_think\n来源观察：\n" + in.SourceText +
		"\n\n关系假设：\n" + in.Claim + "\n类型：" + in.ClaimedType +
		"\n\n支持证据（原始）：\n" + in.SupportMD +
		"\n\n反证检索原始结果：\n" + orDash(counterMD) +
		"\n\n内部目标材料摘要：\n" + orDash(in.InternalBrief)
	judgeResp, err := o.airouter.Chat(ctx, airouter.ChatRequest{
		Capability: o.capability,
		Operation:  relationVerifyJudgeOperation,
		SessionID:  sessionID,
		Messages: []airouter.Message{
			{Role: "system", Content: relationVerifyJudgePrompt},
			{Role: "user", Content: judgeUser},
		},
		Temperature: floatPtr(0.2),
		JSONMode:    true,
	})
	llmCalls++
	if err != nil {
		gaps = append(gaps, relationEvidenceGap{Reason: "verify_judge_llm_error", Detail: err.Error()})
		return nil, counterSearches, gaps, llmCalls
	}
	parsed, pErr := ParseJSONResponse(judgeResp.Content)
	if pErr != nil {
		gaps = append(gaps, relationEvidenceGap{Reason: "verify_judge_parse_error"})
		return nil, counterSearches, gaps, llmCalls
	}
	verdict := relationVerifyOutput{}
	if b, mErr := json.Marshal(parsed); mErr == nil {
		_ = json.Unmarshal(b, &verdict)
	}
	// Enum scrub: illegal type/verdict degrade instead of failing the run.
	if !repository.ValidRelationType(verdict.RelationType) {
		verdict.RelationType = repository.RelationTypeUnclear
	}
	if !repository.ValidRelationVerdict(verdict.Verdict) {
		verdict.Verdict = repository.RelationVerdictInsufficient
	}
	verdict.CounterSummary = strings.TrimSpace(verdict.CounterSummary)
	if len(counterSearches) > 0 && verdict.CounterSummary == "" {
		verdict.CounterSummary = "（已执行反证检索，未见明显反证要点）"
	}
	logging.Infof("relation verifier: type=%s verdict=%s winning=%s", verdict.RelationType, verdict.Verdict, verdict.WinningHypothesis)
	return &verdict, counterSearches, gaps, llmCalls
}

func orDash(s string) string {
	if strings.TrimSpace(s) == "" {
		return "（无）"
	}
	return s
}

// RunRelationScoutForTest exposes the scout stage for budget tests.
func (o *OrchestratorService) RunRelationScoutForTest(ctx context.Context, sessionID, sourceText string, maxSearches int) (*relationScoutOutcome, error) {
	return o.runRelationScout(ctx, sessionID, sourceText, maxSearches)
}
