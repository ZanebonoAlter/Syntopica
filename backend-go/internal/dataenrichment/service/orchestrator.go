package service

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	mrand "math/rand"
	"strings"
	"sync"

	"go.opentelemetry.io/otel"
	"gorm.io/gorm"

	"syntopica-backend/internal/dataenrichment/repository"
	"syntopica-backend/internal/platform/airouter"
	"syntopica-backend/internal/platform/logging"
	"syntopica-backend/internal/platform/tracing"
)

// ── Types ───────────────────────────────────────────────────────────────────

// ToolCallRecord mirrors PoC roles.py ToolCall for agent loop tracing.
type ToolCallRecord struct {
	Step          int            `json:"step"`
	Thought       string         `json:"thought"`
	Tool          string         `json:"tool"`
	Args          map[string]any `json:"args"`
	ResultPreview string         `json:"result_preview"`
	ResultFull    string         `json:"result_full"`

	// Structured decision annotations (board-level-deep-analysis 4.4 shared
	// investigation loop). Only the policy path (toolLoopParams.policy != nil)
	// stamps them; legacy callers keep the exact old JSON shape (omitempty +
	// zero values ⇒ old keys never appear).
	Purpose       string   `json:"purpose,omitempty"`        // neutral|support|counter（agent 声明）
	HypothesisIDs []string `json:"hypothesis_ids,omitempty"` // 声明的目标假设 id
	Outcome       string   `json:"outcome,omitempty"`        // ok|error|blocked
	BlockedReason string   `json:"blocked_reason,omitempty"` // Outcome=blocked 时的稳定机器码
}

// AgentLoopResult holds the output of a single agent loop run.
type AgentLoopResult struct {
	Topic     string           `json:"topic"`
	ToolCalls []ToolCallRecord `json:"tool_calls"`
	FinalData string           `json:"final_data"`
	Loops     int              `json:"loops"`
	Error     string           `json:"error,omitempty"`
}

// EnrichmentOutput is the composite result of one EnrichTopic run.
type EnrichmentOutput struct {
	Result     *repository.TopicEnrichmentResult
	Review     *repository.TopicEnrichmentReview // nil if no review generated
	AgentLoops []*AgentLoopResult                // per-topic agent loop traces
}

// ── OrchestratorService ─────────────────────────────────────────────────────

// OrchestratorService runs the cycle-B data enrichment flow (探索判断 agent 编排).
// See design.md §3 and spec under causal-analysis-agent.
type OrchestratorService struct {
	airouter           AirRouter
	repo               *repository.Repository
	lifelineReader     LifelineReader
	renderer           *LifelineRenderer
	toolRegistry       *Registry
	boardConfigReader  BoardConfigReader
	lensSource         LensSource // 视角来源（默认 AgentLensSource，可注入外部源）
	capability         airouter.Capability
	freshnessRefresher FreshnessRefresher  // D9 新鲜度门（nil=禁用；wire.go 注入循环 A 服务）
	boardResolver      BoardConfigResolver // 版块级配置解析（EnrichBoard 用；wire.go 注入）

	// autoDiscoveryExec executes the auto relation-discovery batch after a
	// brief persists (7.1). Default = detached goroutine with per-board
	// in-flight guard; tests inject a synchronous recorder.
	autoDiscoveryExec func(ctx context.Context, boardID, parentID uint, sources []RelationSourceRef)
	autoMu            sync.Mutex
	autoInFlight      map[uint]bool
}

// NewOrchestratorService creates a new orchestrator with required dependencies.
// The lens source defaults to AgentLensSource (LLM-generated); callers needing
// an external viewpoint source (video commentators, research reports) may swap
// the field after construction. Constructor signature is kept stable so existing
// wiring/tests do not change.
func NewOrchestratorService(
	airouter AirRouter,
	repo *repository.Repository,
	lifelineReader LifelineReader,
	renderer *LifelineRenderer,
	toolRegistry *Registry,
	boardConfigReader BoardConfigReader,
	capability airouter.Capability,
) *OrchestratorService {
	return &OrchestratorService{
		airouter:          airouter,
		repo:              repo,
		lifelineReader:    lifelineReader,
		renderer:          renderer,
		toolRegistry:      toolRegistry,
		boardConfigReader: boardConfigReader,
		lensSource:        NewAgentLensSource(airouter, capability),
		capability:        capability,
	}
}

// ── EnrichTopic: main entry ─────────────────────────────────────────────────

// EnrichTopic runs the full cycle-B flow for a persistent topic.
// Returns the immutable result snapshot and optionally a review.
func (o *OrchestratorService) EnrichTopic(ctx context.Context, topicID uint) (*EnrichmentOutput, error) {
	return o.enrichTopic(ctx, topicID, "")
}

// EnrichTopicLens runs the same flow with a caller-provided lens (D8 下钻入口
// prefill_lens): the lens overrides the first proposed candidate, focusing the
// agent loop and analyze on the drilled-down question.
func (o *OrchestratorService) EnrichTopicLens(ctx context.Context, topicID uint, prefillLens string) (*EnrichmentOutput, error) {
	return o.enrichTopic(ctx, topicID, prefillLens)
}

func (o *OrchestratorService) enrichTopic(ctx context.Context, topicID uint, prefillLens string) (*EnrichmentOutput, error) {
	ctx, span := otel.Tracer(tracing.ServiceName).Start(ctx, "OrchestratorService.EnrichTopic")
	defer span.End()
	// 1. Generate session ID.
	sessionID := generateSessionID(topicID)

	// 2. Read board config.
	cfg, err := o.boardConfigReader.GetBoardConfig(ctx, topicID)
	if err != nil {
		return nil, fmt.Errorf("enrich topic %d: board config: %w", topicID, err)
	}
	if !cfg.EnrichmentEnabled {
		return nil, fmt.Errorf("enrich topic %d: enrichment not enabled for this board", topicID)
	}

	// 3. D9 freshness gate — top up this lane's stale lifelines before the
	// analysis reads them (same gate as EnrichBoard; M4.9).
	_ = o.ensureLaneFreshness(ctx, []uint{topicID})

	// 4. Render lifeline (14-day detail).
	lifelineText, err := o.renderer.RenderLifelineForAgent(o.lifelineReader, topicID, cfg.WindowDays)
	if err != nil {
		return nil, fmt.Errorf("enrich topic %d: render lifeline: %w", topicID, err)
	}

	// 4. Read context layers (filtered by config).
	contextText, contextSnap, err := o.readContextLayers(ctx, topicID, cfg.ContextLayers)
	if err != nil {
		return nil, fmt.Errorf("enrich topic %d: read context: %w", topicID, err)
	}

	// 5. Read applied reviews (for interpret input).
	appliedReviews, err := o.repo.ListAppliedTopicEnrichmentReviews(ctx, topicID)
	if err != nil {
		return nil, fmt.Errorf("enrich topic %d: list reviews: %w", topicID, err)
	}
	reviewIDs := make([]uint, 0, len(appliedReviews))
	reviewText := ""
	for _, r := range appliedReviews {
		reviewIDs = append(reviewIDs, r.ID)
		reviewText += fmt.Sprintf("- [review #%d] %s\n", r.ID, r.DeviationSummary)
	}

	// 6. Step 1: Interpret — form classification + research topics.
	ictx := interpretContext{
		SessionID:    sessionID,
		LifelineText: lifelineText,
		ContextText:  contextText,
		ReviewText:   reviewText,
	}
	interp, err := o.interpret(ctx, ictx)
	if err != nil {
		return nil, fmt.Errorf("enrich topic %d: interpret: %w", topicID, err)
	}

	// 7. Lens candidates + select first.
	// TODO(阶段2b): 视角选择交互——把候选返回前端让用户选/自填。本阶段默认选第一个。
	lenses, err := o.lensSource.Propose(ctx, ictx, interp.Form)
	if err != nil {
		return nil, fmt.Errorf("enrich topic %d: lens propose: %w", topicID, err)
	}
	selectedLens := lenses[0]
	if prefillLens != "" {
		// D8 prefill: drilled-down lens wins over the first proposal.
		selectedLens = Lens{Name: prefillLens, Description: "prefill_lens（版块报告下钻）"}
		logging.Infof("enrich topic %d: prefill_lens overrides lens proposal: %s", topicID, prefillLens)
	}

	// 8. Step 2: Agent loop per topic (selected lens focuses research).
	// Exploration entry points (list_boards/list_lanes/get_lane_detail),
	// web_search and (once wired) fetch_page are ALWAYS available (always-on).
	// cfg.AllowedTools carries board source-typed tools; with the financial
	// direction removed ToolsForSourceType always returns nil, so this is
	// effectively always empty now (the mechanism is retained as an extension
	// point). See board_config_impl.go.
	allowedTools := o.buildAgentAllowedTools(cfg.AllowedTools)
	agentResults := make([]*AgentLoopResult, 0, len(interp.Topics))
	topicsData := make([]map[string]any, 0, len(interp.Topics))
	for _, t := range interp.Topics {
		ar, err := o.runAgentLoop(ctx, sessionID, t.topic, selectedLens.Name, lifelineText, allowedTools)
		agentResults = append(agentResults, ar)
		if err != nil {
			// Agent loop error is non-fatal; record and continue.
			ar.Error = err.Error()
		}
		topicsData = append(topicsData, map[string]any{
			"topic": t.topic,
			"data":  ar.FinalData,
		})
	}

	// 9. Step 3: Analyze — layered insight (fact_layer + insight_layer) by form + lens.
	analysis, err := o.analyze(ctx, sessionID, interp.Form, selectedLens.Name, lifelineText, contextText, topicsData)
	if err != nil {
		return nil, fmt.Errorf("enrich topic %d: analyze: %w", topicID, err)
	}

	// 10. Build input snapshot for traceability.
	inputSnap := buildInputSnapshot(contextSnap, reviewIDs, cfg.WindowDays, cfg.ContextLayers)

	// 11. Build tool_calls JSON from all agent loops.
	allToolCalls := make([]ToolCallRecord, 0)
	for _, ar := range agentResults {
		allToolCalls = append(allToolCalls, ar.ToolCalls...)
	}
	toolCallsJSON, _ := json.Marshal(allToolCalls)

	// 12. Write immutable result. Sectors jsonb stores {form,lens,analysis}
	// (column reused, no DDL — stage-1 migration cleared old evolution-positioning data).
	sectorsJSON, _ := json.Marshal(analysis) // analyzeOutput marshals to {form,lens,analysis}
	snapJSON, _ := json.Marshal(inputSnap)

	result := &repository.TopicEnrichmentResult{
		PersistentTopicID:   repository.TopicIDPtr(topicID),
		EvolutionAssessment: "", // vestigial column; new schema lives in Sectors
		Sectors:             sectorsJSON,
		ToolCalls:           toolCallsJSON,
		InputSnapshot:       snapJSON,
		SessionID:           sessionID,
	}
	if err := o.repo.CreateTopicEnrichmentResult(ctx, result); err != nil {
		return nil, fmt.Errorf("enrich topic %d: save result: %w", topicID, err)
	}

	// 13. Step 4: Review judge (if prev result exists with new-format sectors).
	var review *repository.TopicEnrichmentReview
	prevResult, prevErr := o.repo.GetPrevLatestTopicEnrichmentResult(ctx, topicID, result.ID)
	if prevErr != nil {
		if !errors.Is(prevErr, gorm.ErrRecordNotFound) {
			// Real DB error — log and skip review.
			logging.Warnf("enrich topic %d: get prev result for review judge: %v", topicID, prevErr)
		}
		// No previous result (ErrRecordNotFound) is normal — silently skip review.
	}
	if prevErr == nil && prevResult != nil {
		// Guard: skip review if prev result has no form field (old-format/empty sectors).
		if extractForm(prevResult.Sectors) == "" {
			logging.Infof("enrich topic %d: prev result has no form (old-format/empty sectors), skip review judge", topicID)
		} else {
			prevJSON, _ := json.Marshal(map[string]any{
				"analysis": json.RawMessage(prevResult.Sectors),
			})
			currJSON, _ := json.Marshal(map[string]any{
				"analysis": json.RawMessage(result.Sectors),
			})
			rj, rjErr := o.runReviewJudge(ctx, sessionID, string(prevJSON), string(currJSON))
			if rjErr == nil && rj != nil && rj.ShouldReview {
				conf := rj.Confidence
				verdictObj := map[string]any{
					"new_findings":     rj.NewFindings,
					"overturned":       rj.Overturned,
					"confidence_shift": rj.ConfidenceShift,
				}
				verdictJSON, _ := json.Marshal(verdictObj)
				review = &repository.TopicEnrichmentReview{
					PersistentTopicID: repository.TopicIDPtr(topicID),
					PrevResultID:      &prevResult.ID,
					CurrResultID:      result.ID,
					Verdict:           verdictJSON,
					DeviationSummary:  rj.Reason,
					AffectedContext:   rj.AffectedContext,
					Confidence:        &conf,
					Applied:           false,
					Source:            "llm_assisted",
				}
				if err := o.repo.CreateTopicEnrichmentReview(ctx, review); err != nil {
					return nil, fmt.Errorf("enrich topic %d: save review: %w", topicID, err)
				}
			}
		}
	}

	return &EnrichmentOutput{
		Result:     result,
		Review:     review,
		AgentLoops: agentResults,
	}, nil
}

// ── Interpret (角色①) ──────────────────────────────────────────────────────

// interpretTopic is one research topic extracted by the interpreter to feed
// the agent loop (e.g. an A-share ETF industry direction worth querying).
type interpretTopic struct {
	topic  string
	reason string
}

// interpretContext bundles the text inputs the interpreter + lens source need.
// Kept as a struct so LensSource implementations stay signature-stable as the
// inputs evolve.
type interpretContext struct {
	SessionID    string
	LifelineText string
	ContextText  string
	ReviewText   string
}

// interpretResult is the output of the interpreter role: the topic's form
// classification plus the research topics that feed the agent loop.
type interpretResult struct {
	Form       string // event_chain|theme_vein|single_point|sparse
	FormReason string // 判据说明（为什么这个形态）
	Topics     []interpretTopic
}

// Form classification constants. The form enum is extensible: adding a new form
// only requires a new AnalysisBody impl + a branch in parseAnalyzeOutput, not
// an architecture change. Spec "话题形态判断".
const (
	FormEventChain  = "event_chain"
	FormThemeVein   = "theme_vein"
	FormSinglePoint = "single_point"
	FormStructural  = "structural"
	FormSparse      = "sparse"
)

// Certainty grading constants for insights. Spec "见解依据与确定性分级".
const (
	CertHigh     = "high"     // 已验证
	CertMedium   = "medium"   // 推演·有据
	CertLow      = "low"      // 假设·情景
	CertQuestion = "question" // 提问·指出条件非预言成败
)

func isValidForm(f string) bool {
	switch f {
	case FormEventChain, FormThemeVein, FormSinglePoint, FormStructural, FormSparse:
		return true
	}
	return false
}

// interpretPrompt is the LLM prompt for the interpreter role.
// Performs form classification (first) + research topic extraction (second).
// Spec "话题形态判断".
const interpretPrompt = `你是一位结构化分析编辑。下面是一个持久话题的演进脉络（跨多天的事件演进）与分层新闻上下文。

第一步·形态判断：先判断这个话题的【形态】，五选一：
- event_chain（事件链）：高密度、时序呈线性因果演进（如“官宣→否认→条款”），有清晰的因果链条
- theme_vein（主题脉络）：线索高度发散、多线并行（如“产业范式转移”下多个 AI 线索），无线性因果
- single_point（单点影响）：单一事件/单一时点，影响评估本身即见解
- structural（结构演化）：持续性结构命题、无单一离散事件驱动（如“人民币国际化进程”“美元霸权演变”），长时段结构演化
- sparse（骨感）：料严重不足（命中极少、脉络单薄），无法支撑推演

判据（基于语义综合判断，无需精确数字）：
- 丰满度：事件/线索总量是否足够
- 聚合度：能否聚成时间线/板块结构
- 线性 vs 平行 vs 结构：是“A→B→C”的因果，还是多线并行的脉络，还是长时段的结构命题

第二步·提炼研究方向：基于形态，提炼需要补数据的【研究方向】——领域自适应，不限于金融产业。常见方向包括：历史机制（曾发生过什么、机制如何）、关键数据（可量化的指标/阈值）、可比案例（他国/他行业的同类过程）。每个方向给“为什么要查它”的理由，聚焦 3-5 个。

输出严格 JSON（不要其他内容）：
{"form": "event_chain|theme_vein|single_point|structural|sparse", "form_reason": "为什么是这个形态（一两句）", "topics": [{"topic": "研究方向词", "reason": "关联演进:...所以需要查它"}]}`

func (o *OrchestratorService) interpret(ctx context.Context, ictx interpretContext) (*interpretResult, error) {
	prompt := interpretPrompt + "\n\n---\n"
	if ictx.ContextText != "" {
		prompt += "分层新闻上下文:\n" + ictx.ContextText + "\n\n"
	}
	if ictx.ReviewText != "" {
		prompt += "历史认知记录(避免重蹈已知偏差):\n" + ictx.ReviewText + "\n\n"
	}
	prompt += "话题演进脉络:\n" + ictx.LifelineText

	resp, err := o.airouter.Chat(ctx, airouter.ChatRequest{
		Capability:  o.capability,
		Operation:   "data_enrichment.interpret",
		SessionID:   ictx.SessionID,
		Messages:    []airouter.Message{{Role: "user", Content: prompt}},
		Temperature: floatPtr(0.2),
		JSONMode:    true,
	})
	if err != nil {
		return nil, fmt.Errorf("interpret chat: %w", err)
	}

	parsed, err := ParseJSONResponse(resp.Content)
	if err != nil {
		return nil, fmt.Errorf("interpret parse: %w", err)
	}

	form, _ := parsed["form"].(string)
	if !isValidForm(form) {
		return nil, fmt.Errorf("interpret: invalid or missing form: %q", form)
	}
	formReason, _ := parsed["form_reason"].(string)

	topicsRaw, ok := parsed["topics"].([]any)
	if !ok {
		return nil, fmt.Errorf("interpret: missing or invalid 'topics' field in response")
	}

	topics := make([]interpretTopic, 0, len(topicsRaw))
	for _, t := range topicsRaw {
		tm, ok := t.(map[string]any)
		if !ok {
			continue
		}
		topic, _ := tm["topic"].(string)
		reason, _ := tm["reason"].(string)
		if topic != "" {
			topics = append(topics, interpretTopic{topic: topic, reason: reason})
		}
	}

	// sparse 形态允许零主题（骨感型可能无可查方向）；其它形态至少要一个。
	if len(topics) == 0 && form != FormSparse {
		return nil, fmt.Errorf("interpret: no topics extracted from response")
	}

	return &interpretResult{Form: form, FormReason: formReason, Topics: topics}, nil
}

// ── Agent Loop (角色②) ─────────────────────────────────────────────────────

const maxAgentLoops = 6

// agentLoopSystemPrompt is the system prompt for the data query agent.
// The selected lens focuses what the data should help analyze.
// Ported from PoC roles_evolved.py:research_topic_evolved, lens slot added by
// causal-analysis-agent (spec "分析视角候选与选择").
const agentLoopSystemPrompt = `你是一位研究助理 / 事实核查员。背景：有一个持久话题正在演进（见下方脉络），本次分析视角是「%s」。你需要为结构化分析师搜集背景事实、历史 precedents 与一手原文，支撑深度层（系统重定位 / 多层机制 / 历史类比 / 可核查证据链）。

可用工具：
%s

工作流程（重要）：
1. 先用 web_search 检索该话题的背景、历史 precedents 与专家分析（换 2-3 个角度/关键词）
2. 对关键命中（权威机构、一手数据源），用 fetch_page 抓取正文，取可核查原文摘录（不是 AI 转述）
3. 必要时用 list_boards / list_lanes / get_lane_detail 下钻内部脉络，看本系统里已有的演进
4. 拿到足以支撑深度层的素材后，立即宣布完成

关键纪律（违反会导致死循环）：
- 工具返回的数据是完整的。hit_count 就是真实命中数，不要因为“看起来不全”重查同一个查询
- 取代表性的几条原文即可，不必把每个 URL 都 fetch_page
- 绝对不要用相同参数重复调用同一个工具

每一轮输出严格 JSON，二选一：
- 继续调工具：{"action": "call_tool", "thought": "...", "tool": "工具名", "args": {...}}
- 宣布完成：{"action": "finish", "thought": "...", "summary": "给分析师的简明素材汇总（含可核查 URL 与原文摘录）"}

不要输出 JSON 以外的任何内容。

话题演进脉络（背景）：
%s`

// runAgentLoop executes the agent loop for a single research topic.
// Implements the three defenses from spec:
//
//	① /no_think prefix in user message (PoC double-insurance, complements DB-level enable_thinking=false)
//	② Full tool results in history (never truncated)
//	③ Deduplication: same tool+args blocked
func (o *OrchestratorService) runAgentLoop(ctx context.Context, sessionID, topic, lens, lifelineText string, allowedTools []string) (*AgentLoopResult, error) {
	toolsDesc := buildToolsDesc(o.toolRegistry, allowedTools)
	system := fmt.Sprintf(agentLoopSystemPrompt, lens, toolsDesc, lifelineText)
	return runToolLoop(ctx, o.airouter, o.toolRegistry, o.capability, toolLoopParams{
		sessionID:    sessionID,
		systemPrompt: system,
		taskLine:     "当前要查询的产业主题: " + topic,
		operation:    "data_enrichment.tool_use",
		allowedTools: allowedTools,
		maxLoops:     maxAgentLoops,
		resultTopic:  topic,
	})
}

// toolLoopParams bundles the inputs to runToolLoop so the core loop stays
// signature-stable as callers (EnrichTopic's runAgentLoop, QAAgent.Ask) evolve.
type toolLoopParams struct {
	sessionID    string   // airouter session tag
	systemPrompt string   // pre-built system prompt (caller-specific)
	taskLine     string   // first body line after /no_think (e.g. "当前要查询的产业主题: X" / "用户追问: X")
	operation    string   // airouter operation tag
	allowedTools []string // tools the loop may call (guard + dedup scope)
	maxLoops     int      // loop cap
	resultTopic  string   // optional: sets AgentLoopResult.Topic (enrichment sets it; QA leaves blank)

	// policy (optional, board-level-deep-analysis 4.4): per-decision validation
	// + finish discipline for the shared investigation research loop. nil =
	// legacy behavior, byte-for-byte unchanged (regression-tested).
	policy toolLoopPolicy
}

// toolLoopPolicy is the optional decision-policy extension point for
// runToolLoop. It adds per-call declaration validation, finish-discipline
// gating and call observation WITHOUT forking the loop itself — the three
// defenses (/no_think, full results, dedup) stay shared so they cannot drift
// between callers. policy=nil keeps legacy behavior exactly as before.
type toolLoopPolicy interface {
	// CheckCall validates a call_tool decision BEFORE the tool executes.
	// Blocked verdicts carry a stable machine reason (for records/snapshots)
	// plus agent-facing feedback (for the rewrite); Purpose/HypothesisIDs are
	// the parsed declarations stamped onto every record of this decision.
	CheckCall(step int, decision map[string]any) toolCallVerdict
	// ObserveCall is invoked only after a tool actually executed — never for
	// policy-blocked or dedup-intercepted calls (拦截不得冒充完成纪律).
	ObserveCall(step int, toolName string, args map[string]any, resultFull, purpose string, hypothesisIDs []string)
	// CheckFinish may reject a finish decision; the loop then continues with
	// the corrective feedback appended to the agent-visible history.
	CheckFinish(step int, summary string) toolFinishVerdict
}

// toolCallVerdict is the outcome of toolLoopPolicy.CheckCall.
type toolCallVerdict struct {
	Blocked       bool
	BlockedReason string   // stable machine code (records/snapshots)
	Feedback      string   // agent-facing rewrite instruction
	Purpose       string   // parsed/validated purpose declaration
	HypothesisIDs []string // parsed/validated target hypothesis ids
}

// toolFinishVerdict is the outcome of toolLoopPolicy.CheckFinish.
type toolFinishVerdict struct {
	Blocked  bool
	Feedback string
}

// Structured ToolCallRecord.Outcome values (policy path only).
const (
	toolCallOutcomeOK      = "ok"
	toolCallOutcomeError   = "error"
	toolCallOutcomeBlocked = "blocked"
)

// toolResultErrorText extracts the "error" field from a tool result JSON
// (registry convention: failures degrade to {"error": "..."}). Empty string
// means the result carries no error. Shared by the loop (Outcome stamping)
// and the investigation policy (gap classification) so both agree on what a
// failed tool call is.
func toolResultErrorText(resultFull string) string {
	var m map[string]any
	if err := json.Unmarshal([]byte(resultFull), &m); err != nil {
		return ""
	}
	if e, ok := m["error"].(string); ok {
		return e
	}
	return ""
}

// runToolLoop is the shared agent core used by both the enrichment agent loop
// (runAgentLoop) and the report follow-up QA agent (QAAgent.Ask). It preserves
// the three defenses from spec:
//   - ① /no_think prefix in user message (PoC double-insurance)
//   - ② Full tool results in history (never truncated)
//   - ③ Deduplication: same tool+args blocked
//
// The system prompt + taskLine are caller-specific; everything inside (chat,
// parse, allowed-tools guard, dedup, execute, history accumulation) is shared
// so the two agents never drift on loop discipline.
func runToolLoop(ctx context.Context, router AirRouter, toolRegistry *Registry, capability airouter.Capability, p toolLoopParams) (*AgentLoopResult, error) {
	// Build allowedTools set for O(1) lookup.
	allowedSet := make(map[string]bool, len(p.allowedTools))
	for _, t := range p.allowedTools {
		allowedSet[t] = true
	}

	result := &AgentLoopResult{Topic: p.resultTopic}
	historyLines := make([]string, 0)
	seenCalls := make(map[string]bool) // dedup key → true

	for step := 1; step <= p.maxLoops; step++ {
		result.Loops = step

		historyBlock := strings.Join(historyLines, "\n")
		if historyBlock == "" {
			historyBlock = "(尚无工具调用)"
		}

		// 防御① /no_think prefix (双保险 — DB provider 配置是主防线).
		userMsg := fmt.Sprintf("/no_think\n%s\n\n已有的工具调用历史:\n%s\n\n请决定下一步(调工具或宣布完成),输出 JSON。", p.taskLine, historyBlock)

		resp, err := router.Chat(ctx, airouter.ChatRequest{
			Capability:  capability,
			Operation:   p.operation,
			SessionID:   p.sessionID,
			Messages:    []airouter.Message{{Role: "system", Content: p.systemPrompt}, {Role: "user", Content: userMsg}},
			Temperature: floatPtr(0.2),
			JSONMode:    true,
		})
		if err != nil {
			result.Error = fmt.Sprintf("第%d轮 LLM 调用失败: %v", step, err)
			break
		}

		decision, parseErr := ParseJSONResponse(resp.Content)
		if parseErr != nil {
			result.Error = fmt.Sprintf("第%d轮 LLM 输出无法解析: %s", step, prefix(resp.Content, 200))
			break
		}

		action, _ := decision["action"].(string)
		thought, _ := decision["thought"].(string)

		switch action {
		case "finish":
			summary, _ := decision["summary"].(string)
			// Decision policy: a finish that has not met the research discipline
			// is bounced back into the loop with corrective feedback
			// (board-level-deep-analysis 4.4). Legacy callers (policy=nil)
			// finish as before.
			if p.policy != nil {
				if v := p.policy.CheckFinish(step, summary); v.Blocked {
					historyLines = append(historyLines, fmt.Sprintf("第%d步: 宣布完成被拦: %s — 请继续按研究纪律调用工具。", step, v.Feedback))
					continue
				}
			}
			result.FinalData = summary
			return result, nil

		case "call_tool":
			toolName, _ := decision["tool"].(string)
			argsRaw := decision["args"]
			args, ok := argsRaw.(map[string]any)
			if !ok {
				args = map[string]any{}
			}

			// Decision policy (board-level-deep-analysis 4.4): consulted before
			// guard/dedup/execute. Blocked calls are recorded with a stable
			// machine reason + agent-facing feedback; the tool never executes
			// and the dedup key is not consumed. Legacy callers skip entirely.
			var polPurpose string
			var polHypIDs []string
			if p.policy != nil {
				v := p.policy.CheckCall(step, decision)
				polPurpose, polHypIDs = v.Purpose, v.HypothesisIDs
				if v.Blocked {
					fb, _ := json.Marshal(map[string]any{"error": "调用被拦: " + v.BlockedReason, "feedback": v.Feedback})
					tc := ToolCallRecord{
						Step:          step,
						Thought:       thought + " [被拦:" + v.BlockedReason + "]",
						Tool:          toolName,
						Args:          args,
						ResultPreview: string(fb),
						ResultFull:    string(fb),
						Purpose:       polPurpose,
						HypothesisIDs: polHypIDs,
						Outcome:       toolCallOutcomeBlocked,
						BlockedReason: v.BlockedReason,
					}
					result.ToolCalls = append(result.ToolCalls, tc)
					historyLines = append(historyLines, fmt.Sprintf("第%d步: 调用 %s(%s) — 被拦[%s]: %s", step, toolName, argsToJSON(args), v.BlockedReason, v.Feedback))
					continue
				}
			}

			// Guard: reject tools not in allowedTools.
			if len(allowedSet) > 0 && !allowedSet[toolName] {
				errJSON := `{"error":"该工具当前不可用"}`
				tc := ToolCallRecord{
					Step:          step,
					Thought:       thought + " [被拦:工具不可用]",
					Tool:          toolName,
					Args:          args,
					ResultPreview: errJSON,
					ResultFull:    errJSON,
				}
				if p.policy != nil {
					tc.Purpose, tc.HypothesisIDs = polPurpose, polHypIDs
					tc.Outcome, tc.BlockedReason = toolCallOutcomeBlocked, "tool_not_allowed"
				}
				result.ToolCalls = append(result.ToolCalls, tc)
				historyLines = append(historyLines, fmt.Sprintf("第%d步: 调用 %s(%s) — 结果: %s", step, toolName, argsToJSON(args), errJSON))
				continue
			}

			// 防御③ Dedup check.
			dedupKey := dedupKeyFor(toolName, args)
			if seenCalls[dedupKey] {
				errJSON := fmt.Sprintf(`{"error":"已用相同参数调用过 %s,不要重复,基于已有数据继续。"}`, toolName)
				tc := ToolCallRecord{
					Step:          step,
					Thought:       thought + " [被拦:重复]",
					Tool:          toolName,
					Args:          args,
					ResultPreview: errJSON,
					ResultFull:    errJSON,
				}
				if p.policy != nil {
					tc.Purpose, tc.HypothesisIDs = polPurpose, polHypIDs
					tc.Outcome, tc.BlockedReason = toolCallOutcomeBlocked, "duplicate_call"
				}
				result.ToolCalls = append(result.ToolCalls, tc)
				historyLines = append(historyLines, fmt.Sprintf("第%d步: 调用 %s(%s) — 结果: %s", step, toolName, argsToJSON(args), errJSON))
				continue
			}
			seenCalls[dedupKey] = true

			// Execute tool.
			toolResult, toolErr := toolRegistry.Execute(ctx, toolName, args)
			if toolErr != nil {
				// json.Marshal 保证错误文本中的引号/换行被合法转义——ResultFull
				// 必须可解析（toolResultErrorText / outcome 判定 / investigation gap
				// 分类都依赖这一点）。普通文本序列化结果与旧格式逐字节一致
				// （policy=nil 旧 JSON 兼容）。
				if b, mErr := json.Marshal(map[string]any{"error": toolErr.Error()}); mErr == nil {
					toolResult = string(b)
				} else {
					toolResult = `{"error":"tool execution failed"}`
				}
			}

			// 防御② Full result in history (no truncation for LLM).
			preview := toolResult
			if len(preview) > 300 {
				preview = preview[:300]
			}

			tc := ToolCallRecord{
				Step:          step,
				Thought:       thought,
				Tool:          toolName,
				Args:          args,
				ResultPreview: preview,
				ResultFull:    toolResult,
			}
			if p.policy != nil {
				tc.Purpose, tc.HypothesisIDs = polPurpose, polHypIDs
				if toolResultErrorText(toolResult) != "" {
					tc.Outcome = toolCallOutcomeError
				} else {
					tc.Outcome = toolCallOutcomeOK
				}
				// 只有真正执行过的调用才进入纪律观察（拦截不冒充尝试）。
				p.policy.ObserveCall(step, toolName, args, toolResult, polPurpose, polHypIDs)
			}
			result.ToolCalls = append(result.ToolCalls, tc)
			historyLines = append(historyLines, fmt.Sprintf("第%d步: 调用 %s(%s) — 想法: %s — 结果: %s", step, toolName, argsToJSON(args), thought, toolResult))

		default:
			result.Error = fmt.Sprintf("第%d轮 action 不合法: %s", step, action)
			return result, nil
		}
	}

	// Exhausted maxLoops.
	if result.Error == "" {
		result.Error = fmt.Sprintf("达到最大循环数 %d 未完成", p.maxLoops)
	}
	return result, nil
}

// ── Analyze (角色③) ────────────────────────────────────────────────────────

// analyzeOutput is the structured output of the analyst role.
//
// Replaces the old evolution-positioning schema (position/signals/causal_chain)
// with a polymorphic "exploration judgment" payload keyed by Form. Spec
// "分层见解产出" / "见解依据与确定性分级".
type analyzeOutput struct {
	Form     string       `json:"form"`     // event_chain|theme_vein|single_point|sparse
	Lens     string       `json:"lens"`     // 选定视角（具体问题式）
	Analysis AnalysisBody `json:"analysis"` // 按 form 多态
}

// AnalysisBody is the polymorphic analysis payload. Concrete type varies by
// Form; per-form structs live below. JSON round-trip is handled by a custom
// UnmarshalJSON on *analyzeOutput that dispatches by Form. The sealed-interface
// marker keeps the set of valid bodies closed.
type AnalysisBody interface {
	isAnalysisBody()
}

// Ref is a typed evidence reference: a news article quote or an agent tool
// result. Spec "见解依据与确定性分级" (双类引用 📰新闻/🔧工具).
type Ref struct {
	SourceType string `json:"source_type"` // "news"|"tool"
	Ref        string `json:"ref"`
	Quote      string `json:"quote,omitempty"`
}

// FactClaim is a verified fact node in the fact_layer (event_chain).
type FactClaim struct {
	Claim    string `json:"claim"`
	Evidence []Ref  `json:"evidence"`
	Verified bool   `json:"verified"`
}

// TimelineNode is a dated event in the timeline (event_chain).
type TimelineNode struct {
	Date  string `json:"date"`
	Event string `json:"event"`
	Ref   *Ref   `json:"ref,omitempty"`
}

// Insight is a推演/假设/提问 insight — the core output of the analysis. Cert ∈
// high|medium|low|question. Every insight MUST carry Evidence; insights with
// empty evidence are dropped at parse time (spec "见解必须挂依据").
type Insight struct {
	Cert        string `json:"cert"` // high|medium|low|question
	Title       string `json:"title"`
	Logic       string `json:"logic"`
	Evidence    []Ref  `json:"evidence"`
	WebVerified []Ref  `json:"web_verified,omitempty"`
}

// hasEvidence reports whether an insight carries at least one supporting ref.
func (in Insight) hasEvidence() bool {
	return len(in.Evidence) > 0 || len(in.WebVerified) > 0
}

// Depth is the structured depth layer (mapping the "内部看美国" analysis genes).
// Required for every non-sparse form; SparseAnalysis carries no Depth by design.
type Depth struct {
	SystemReframe     string              `json:"system_reframe"`     // ②系统重定位：一句话放进哪个大系统讲
	MechanismLayers   []MechanismLayer    `json:"mechanism_layers"`   // ④多层机制拆解
	HistoricalAnalogy []HistoricalAnalogy `json:"historical_analogy"` // ③历史类比
	RegimeShift       *RegimeShift        `json:"regime_shift"`       // ⑥范式转折（可空）
	Boundary          string              `json:"boundary"`           // ⑤反过度解读边界（非空！）
	EvidenceChain     []EvidenceChainItem `json:"evidence_chain"`     // ⑦可核查证据链
}

// MechanismLayer is one sub-mechanism in the multi-layer mechanism breakdown.
type MechanismLayer struct {
	Layer     string `json:"layer"`      // 子机制名
	DeepLogic string `json:"deep_logic"` // 深层逻辑
	Basis     string `json:"basis"`      // 依据
}

// HistoricalAnalogy is a historical precedent comparison.
type HistoricalAnalogy struct {
	Case      string `json:"case"`      // 历史案例
	Mechanism string `json:"mechanism"` // 机制类比
	Diff      string `json:"diff"`      // 何处不同
}

// RegimeShift is an optional paradigm-shift judgment (null when none is warranted).
type RegimeShift struct {
	Judgment string `json:"judgment"` // 范式转折判断
	Evidence string `json:"evidence"` // 依据
}

// EvidenceChainItem is one verifiable-evidence entry. source_type ∈
// news|web|page|lane (lane = board-scope lane reference, Ref carries lane_id);
// web/page entries must carry a clickable url + a direct quote (not an AI
// paraphrase). Kind is the orthogonal two-level classification (quote 文段引用
// / series 数字序列 / chart 图表; empty = legacy behaviour, board-level-deep-
// analysis D4).
type EvidenceChainItem struct {
	SourceType  string `json:"source_type"`           // news|web|page|lane
	Ref         string `json:"ref,omitempty"`         // news 引用 id / lane 泳道 id
	URL         string `json:"url,omitempty"`         // web/page 可核查 URL
	Quote       string `json:"quote,omitempty"`       // 原文摘录（非转述）
	Institution string `json:"institution,omitempty"` // 来源机构
	Date        string `json:"date,omitempty"`        // 日期
	Kind        string `json:"kind,omitempty"`        // quote|series|chart（可选两级分类）
	LaneNote    string `json:"lane_note,omitempty"`   // lane 证据的论据说明
}

// EventChainAnalysis is the body for form=event_chain.
type EventChainAnalysis struct {
	FactLayer    []FactClaim    `json:"fact_layer"`
	Timeline     []TimelineNode `json:"timeline"`
	InsightLayer []Insight      `json:"insight_layer"`
	Depth        Depth          `json:"depth"`
}

func (EventChainAnalysis) isAnalysisBody() {}

// Vein is a parallel theme thread (theme_vein.veins).
type Vein struct {
	Name     string `json:"name"`
	Desc     string `json:"desc"`
	Evidence []Ref  `json:"evidence"`
}

// ThemeVeinAnalysis is the body for form=theme_vein.
type ThemeVeinAnalysis struct {
	Veins        []Vein    `json:"veins"`
	CrossInsight []Insight `json:"cross_insight"`
	Depth        Depth     `json:"depth"`
}

func (ThemeVeinAnalysis) isAnalysisBody() {}

// ImpactAssessment is the single-point impact (itself the insight).
type ImpactAssessment struct {
	Implication string `json:"implication"`
	Ripple      string `json:"ripple"`
	Benchmark   string `json:"benchmark"`
}

// SinglePointAnalysis is the body for form=single_point.
type SinglePointAnalysis struct {
	Impact   ImpactAssessment `json:"impact"`
	Evidence []Ref            `json:"evidence"`
	Depth    Depth            `json:"depth"`
}

func (SinglePointAnalysis) isAnalysisBody() {}

// SparseAnalysis is the body for form=sparse — honestly marks information
// insufficiency. By design it has NO insight_layer and NO Depth (spec "骨感型
// 不硬推演" / "sparse 不产深度层").
type SparseAnalysis struct {
	Notice  string `json:"notice"`
	Summary string `json:"summary"`
}

func (SparseAnalysis) isAnalysisBody() {}

// StructuralAnalysis is the body for form=structural — long-horizon structural
// evolution with no single discrete event driver (e.g. "人民币国际化进程").
type StructuralAnalysis struct {
	EvolutionNarrative string  `json:"evolution_narrative"` // 结构演化叙述
	Phases             []Phase `json:"phases"`              // 关键阶段
	Depth              Depth   `json:"depth"`
}

// Phase is one dated milestone in the structural evolution narrative.
type Phase struct {
	Period string `json:"period"`
	Event  string `json:"event"`
	Ref    *Ref   `json:"ref,omitempty"`
}

func (StructuralAnalysis) isAnalysisBody() {}

// UnmarshalJSON dispatches the analysis body by the form field, enabling JSON
// round-trip of analyzeOutput (e.g. reading a stored Sectors snapshot back).
func (a *analyzeOutput) UnmarshalJSON(data []byte) error {
	var raw struct {
		Form     string          `json:"form"`
		Lens     string          `json:"lens"`
		Analysis json.RawMessage `json:"analysis"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	a.Form = raw.Form
	a.Lens = raw.Lens
	switch raw.Form {
	case FormEventChain:
		var body EventChainAnalysis
		if err := json.Unmarshal(raw.Analysis, &body); err != nil {
			return fmt.Errorf("event_chain analysis: %w", err)
		}
		a.Analysis = body
	case FormThemeVein:
		var body ThemeVeinAnalysis
		if err := json.Unmarshal(raw.Analysis, &body); err != nil {
			return fmt.Errorf("theme_vein analysis: %w", err)
		}
		a.Analysis = body
	case FormSinglePoint:
		var body SinglePointAnalysis
		if err := json.Unmarshal(raw.Analysis, &body); err != nil {
			return fmt.Errorf("single_point analysis: %w", err)
		}
		a.Analysis = body
	case FormStructural:
		var body StructuralAnalysis
		if err := json.Unmarshal(raw.Analysis, &body); err != nil {
			return fmt.Errorf("structural analysis: %w", err)
		}
		a.Analysis = body
	case FormSparse:
		var body SparseAnalysis
		if err := json.Unmarshal(raw.Analysis, &body); err != nil {
			return fmt.Errorf("sparse analysis: %w", err)
		}
		a.Analysis = body
	default:
		return fmt.Errorf("unknown form: %q", raw.Form)
	}
	return nil
}

// analyzePrompt is the LLM prompt for the analyst role.
// Produces layered output (fact_layer + insight_layer) shaped by form + lens.
// Spec "分层见解产出" / "见解依据与确定性分级" / "骨感型诚实标注".
const analyzePrompt = `你是一位结构化分析师。话题形态已判断为「%s」，本次分析视角是「%s」。

下面提供：
1) 话题演进脉络 (lifeline)
2) 分层新闻上下文
3) 各主题素材（agent 检索到的背景/历史/原文）

你的任务：按形态 + 视角，产出【分层分析】——事实层（梳理+验证，铺垫）+ 见解层（推演/假设/提问，★产出主体）+ 深度层（非 sparse 形态强制，见下方铁律）。发挥 AI 多层推演 + 跨领域联想 + 假设性提问的优势。

【按形态产出结构（严格按本形态选一）】

▶ event_chain（事件链）：
{"fact_layer":[{"claim":"已验证的事实陈述","evidence":[{"source_type":"news|web|page","ref":"引用 id 或 url","quote":"原话"}],"verified":true}],"timeline":[{"date":"2026-07-01","event":"事件","ref":{"source_type":"news","ref":"..."}}],"insight_layer":[{"cert":"high|medium|low|question","title":"见解标题","logic":"凭什么 A→B 的推演逻辑","evidence":[{"source_type":"news","ref":"...","quote":"..."}],"web_verified":[{"source_type":"web|page","ref":"..."}]}],"depth":{...见深度层铁律...}}

▶ theme_vein（主题脉络）：
{"veins":[{"name":"线索名","desc":"线索描述","evidence":[{"source_type":"news","ref":"..."}]}],"cross_insight":[{"cert":"...","title":"...","logic":"...","evidence":[...]}],"depth":{...}}

▶ single_point（单点影响）：
{"impact":{"implication":"直接影响","ripple":"连锁涟漪","benchmark":"可比历史基准"},"evidence":[{"source_type":"news","ref":"..."}],"depth":{...}}

▶ structural（结构演化）：
{"evolution_narrative":"结构演化叙述","phases":[{"period":"...","event":"...","ref":{...}}],"depth":{...}}

▶ sparse（骨感）：
{"notice":"信息不足的诚实说明（命中少/脉络薄，哪些还说不准）","summary":"能确定的轻量摘要"}

【见解层铁律】
1. 每条 insight 必须挂 evidence（文章依据或时间线节点），无依据的见解会被丢弃——不要产出悬空见解
2. 确定性分级 cert 四选一：
   - high：已验证的事实推论
   - medium：推演·有据（凭证据链推出来的）
   - low：假设·情景（条件成立才会如此）
   - question：提问·指出决定成败的条件，不要预言成败结果
3. 见解要给 logic（凭什么 A→B），关键中间环节尽量有 evidence/web_verified 支撑
4. sparse 形态【不产出】 insight_layer/cross_insight/depth，只给 notice + summary

【深度层（非 sparse 形态强制，sparse 不产出）】
depth 块字段（映射结构化深度分析基因）：
- system_reframe：一句话——这个话题该放进哪个更大的系统来讲（系统重定位）
- mechanism_layers：多层子机制拆解，每层给 layer(子机制名)+deep_logic(深层逻辑)+basis(依据)
- historical_analogy：历史类比，给 case(案例)+mechanism(机制类比)+diff(何处不同)
- regime_shift：范式转折判断（确实有才填，无则 null）
- boundary：★反过度解读边界——明确写出“目前还不能下结论的边界”，不可空泛，不可省略
- evidence_chain：可核查证据链，source_type ∈ news(分层新闻)|web(web_search 网页)|page(fetch_page 正文)；web/page 必须带 url + quote(原文摘录，非转述) + institution(来源机构) + date

【证据纪律】
evidence_chain 的质量以与研究问题的【直接相关性】为第一优先级：优先收录直接支撑或检验本视角结论的一手依据（检索用机构名 + 数据关键词组合，而非事件转述关键词），其次为可核查的二手材料，最后才是背景新闻。证据类型数量不是质量指标——不为凑类型强行加入与问题无关的历史类比、报告或数据序列；材料不足就如实减少，不注水。
- web/page 证据必须可核查：url + quote(原文摘录，非转述) + institution(来源机构) + date，四要素缺一不可
- 与主解释相冲突或构成反证的材料，同样收入 evidence_chain，并在 depth.boundary 里如实呈现其张力——不得因不符合主解释而丢弃或弱化
- 确实检索不到所需材料时，在 boundary 里诚实标注缺口并降低相关结论的确定性，不要编造证据

【引用格式】source_type ∈ news（来自分层新闻上下文）| web（web_search 网页结果）| page（fetch_page 正文）；news 的 ref 指向具体 id，web/page 带 url；quote 直接摘录原话/原文

输出严格 JSON（不要 markdown 包裹）：
{"form":"%s","lens":"%s","analysis":{...按形态...}}`

func (o *OrchestratorService) analyze(ctx context.Context, sessionID, form, lens, lifelineText, contextText string, topicsData []map[string]any) (*analyzeOutput, error) {
	topicsBlock := ""
	for _, td := range topicsData {
		topic, _ := td["topic"].(string)
		data, _ := td["data"].(string)
		if data == "" {
			data = "(无数据)"
		}
		topicsBlock += fmt.Sprintf("【%s】\n查询数据:\n%s\n\n", topic, data)
	}

	prompt := fmt.Sprintf(analyzePrompt, form, lens, form, lens)
	if contextText != "" {
		prompt += "\n\n---\n分层新闻上下文:\n" + contextText
	}
	prompt += "\n\n话题演进脉络:\n" + lifelineText + "\n\n各主题实时数据:\n" + topicsBlock

	out, perr := o.callAndParseAnalyze(ctx, sessionID, prompt)
	if perr != nil {
		// Retry once: feed the parse/depth error back so the LLM can correct a
		// malformed or depth-less output (spec: "解析失败重试一次").
		retryPrompt := prompt + "\n\n---\n上次输出解析失败：" + perr.Error() +
			"。请按上述 schema 重新输出完整 JSON（非 sparse 形态必须含完整 depth 块：system_reframe 非空、mechanism_layers≥1、boundary 非空、evidence_chain≥1），不要输出 markdown 包裹。"
		out2, perr2 := o.callAndParseAnalyze(ctx, sessionID, retryPrompt)
		if perr2 != nil {
			return nil, fmt.Errorf("analyze (after retry): %w", perr2)
		}
		return out2, nil
	}
	return out, nil
}

// callAndParseAnalyze runs a single analyze LLM call and parses the result,
// enforcing the per-form schema + depth contract via parseAnalyzeOutput.
func (o *OrchestratorService) callAndParseAnalyze(ctx context.Context, sessionID, prompt string) (*analyzeOutput, error) {
	resp, err := o.airouter.Chat(ctx, airouter.ChatRequest{
		Capability:  o.capability,
		Operation:   "data_enrichment.analyze",
		SessionID:   sessionID,
		Messages:    []airouter.Message{{Role: "user", Content: prompt}},
		Temperature: floatPtr(0.3),
		JSONMode:    true,
	})
	if err != nil {
		return nil, fmt.Errorf("analyze chat: %w", err)
	}
	parsed, err := ParseJSONResponse(resp.Content)
	if err != nil {
		return nil, fmt.Errorf("analyze parse: %w", err)
	}
	return parseAnalyzeOutput(parsed)
}

// parseAnalyzeOutput builds an analyzeOutput from the parsed LLM map, enforcing
// per-form structure and the "insight must have evidence" rule (spec: insights
// with empty evidence are dropped with a warning).
func parseAnalyzeOutput(parsed map[string]any) (*analyzeOutput, error) {
	form, _ := parsed["form"].(string)
	lens, _ := parsed["lens"].(string)
	if !isValidForm(form) {
		return nil, fmt.Errorf("analyze: invalid or missing form: %q", form)
	}
	analysisRaw, ok := parsed["analysis"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("analyze: missing or invalid 'analysis' field")
	}

	var body AnalysisBody
	switch form {
	case FormEventChain:
		body = parseEventChainAnalysis(analysisRaw)
	case FormThemeVein:
		body = parseThemeVeinAnalysis(analysisRaw)
	case FormSinglePoint:
		body = parseSinglePointAnalysis(analysisRaw)
	case FormStructural:
		body = parseStructuralAnalysis(analysisRaw)
	case FormSparse:
		body = parseSparseAnalysis(analysisRaw)
	}

	// Depth layer (spec "分析深度层产出"): every non-sparse form MUST carry a
	// complete depth block; sparse forbids it (depth is ignored if present).
	if form != FormSparse {
		if d, ok := depthOf(body); ok {
			if err := validateDepth(form, d); err != nil {
				return nil, err
			}
		}
	}
	return &analyzeOutput{Form: form, Lens: lens, Analysis: body}, nil
}

// depthOf extracts the Depth block from any non-sparse analysis body. Returns
// ok=false for SparseAnalysis (which carries no Depth by construction).
func depthOf(body AnalysisBody) (Depth, bool) {
	switch b := body.(type) {
	case EventChainAnalysis:
		return b.Depth, true
	case ThemeVeinAnalysis:
		return b.Depth, true
	case SinglePointAnalysis:
		return b.Depth, true
	case StructuralAnalysis:
		return b.Depth, true
	}
	return Depth{}, false
}

// validateDepth enforces the non-sparse depth contract: system_reframe and
// boundary must be non-empty, mechanism_layers ≥ 1, evidence_chain ≥ 1. A
// missing/empty field returns an error so analyze can retry once.
func validateDepth(form string, d Depth) error {
	if strings.TrimSpace(d.SystemReframe) == "" {
		return fmt.Errorf("analyze: depth.system_reframe required for %s (系统重定位)", form)
	}
	if len(d.MechanismLayers) < 1 {
		return fmt.Errorf("analyze: depth.mechanism_layers require >=1 for %s", form)
	}
	if strings.TrimSpace(d.Boundary) == "" {
		return fmt.Errorf("analyze: depth.boundary required for %s (反过度解读边界)", form)
	}
	if len(d.EvidenceChain) < 1 {
		return fmt.Errorf("analyze: depth.evidence_chain require >=1 for %s", form)
	}
	return nil
}

func parseEventChainAnalysis(m map[string]any) EventChainAnalysis {
	var body EventChainAnalysis
	if fl, ok := m["fact_layer"].([]any); ok {
		for _, f := range fl {
			fm, ok := f.(map[string]any)
			if !ok {
				continue
			}
			body.FactLayer = append(body.FactLayer, FactClaim{
				Claim:    getString(fm, "claim"),
				Evidence: parseRefs(fm["evidence"]),
				Verified: getBool(fm, "verified"),
			})
		}
	}
	if tl, ok := m["timeline"].([]any); ok {
		for _, t := range tl {
			tm, ok := t.(map[string]any)
			if !ok {
				continue
			}
			node := TimelineNode{Date: getString(tm, "date"), Event: getString(tm, "event")}
			if refMap, ok := tm["ref"].(map[string]any); ok {
				node.Ref = parseRef(refMap)
			}
			body.Timeline = append(body.Timeline, node)
		}
	}
	if il, ok := m["insight_layer"].([]any); ok {
		for _, i := range il {
			ins := parseInsight(i)
			if !ins.hasEvidence() {
				logging.Warnf("analyze: drop evidence-less insight: %q", ins.Title)
				continue
			}
			body.InsightLayer = append(body.InsightLayer, ins)
		}
	}
	body.Depth = parseDepth(m)
	return body
}

func parseThemeVeinAnalysis(m map[string]any) ThemeVeinAnalysis {
	var body ThemeVeinAnalysis
	if vs, ok := m["veins"].([]any); ok {
		for _, v := range vs {
			vm, ok := v.(map[string]any)
			if !ok {
				continue
			}
			body.Veins = append(body.Veins, Vein{
				Name:     getString(vm, "name"),
				Desc:     getString(vm, "desc"),
				Evidence: parseRefs(vm["evidence"]),
			})
		}
	}
	if ci, ok := m["cross_insight"].([]any); ok {
		for _, i := range ci {
			ins := parseInsight(i)
			if !ins.hasEvidence() {
				logging.Warnf("analyze: drop evidence-less cross_insight: %q", ins.Title)
				continue
			}
			body.CrossInsight = append(body.CrossInsight, ins)
		}
	}
	body.Depth = parseDepth(m)
	return body
}

func parseSinglePointAnalysis(m map[string]any) SinglePointAnalysis {
	var body SinglePointAnalysis
	if im, ok := m["impact"].(map[string]any); ok {
		body.Impact = ImpactAssessment{
			Implication: getString(im, "implication"),
			Ripple:      getString(im, "ripple"),
			Benchmark:   getString(im, "benchmark"),
		}
	}
	body.Evidence = parseRefs(m["evidence"])
	body.Depth = parseDepth(m)
	return body
}

func parseSparseAnalysis(m map[string]any) SparseAnalysis {
	return SparseAnalysis{
		Notice:  getString(m, "notice"),
		Summary: getString(m, "summary"),
	}
}

// parseStructuralAnalysis parses the form=structural body: the structural
// evolution narrative + dated phases (timeline-style) + depth block.
func parseStructuralAnalysis(m map[string]any) StructuralAnalysis {
	var body StructuralAnalysis
	body.EvolutionNarrative = getString(m, "evolution_narrative")
	if ps, ok := m["phases"].([]any); ok {
		for _, p := range ps {
			pm, ok := p.(map[string]any)
			if !ok {
				continue
			}
			node := Phase{Period: getString(pm, "period"), Event: getString(pm, "event")}
			if refMap, ok := pm["ref"].(map[string]any); ok {
				node.Ref = parseRef(refMap)
			}
			body.Phases = append(body.Phases, node)
		}
	}
	body.Depth = parseDepth(m)
	return body
}

// parseDepth parses the 6-field depth block nested under the analysis map's
// "depth" key. mechanism_layers / historical_analogy / evidence_chain use
// sub-parsers; regime_shift is a nullable object (absent or non-object → nil).
// Returns a zero Depth when the depth field is missing — callers rely on
// validateDepth to enforce non-emptiness.
func parseDepth(m map[string]any) Depth {
	var d Depth
	dm, ok := m["depth"].(map[string]any)
	if !ok {
		return d // no depth block — validateDepth will reject for non-sparse forms
	}
	d.SystemReframe = getString(dm, "system_reframe")
	d.Boundary = getString(dm, "boundary")
	if ml, ok := dm["mechanism_layers"].([]any); ok {
		for _, v := range ml {
			vm, ok := v.(map[string]any)
			if !ok {
				continue
			}
			d.MechanismLayers = append(d.MechanismLayers, MechanismLayer{
				Layer:     getString(vm, "layer"),
				DeepLogic: getString(vm, "deep_logic"),
				Basis:     getString(vm, "basis"),
			})
		}
	}
	if ha, ok := dm["historical_analogy"].([]any); ok {
		for _, v := range ha {
			vm, ok := v.(map[string]any)
			if !ok {
				continue
			}
			d.HistoricalAnalogy = append(d.HistoricalAnalogy, HistoricalAnalogy{
				Case:      getString(vm, "case"),
				Mechanism: getString(vm, "mechanism"),
				Diff:      getString(vm, "diff"),
			})
		}
	}
	if rs, ok := dm["regime_shift"].(map[string]any); ok {
		d.RegimeShift = &RegimeShift{
			Judgment: getString(rs, "judgment"),
			Evidence: getString(rs, "evidence"),
		}
	}
	if ec, ok := dm["evidence_chain"].([]any); ok {
		for _, v := range ec {
			vm, ok := v.(map[string]any)
			if !ok {
				continue
			}
			d.EvidenceChain = append(d.EvidenceChain, EvidenceChainItem{
				SourceType:  getString(vm, "source_type"),
				Ref:         getString(vm, "ref"),
				URL:         getString(vm, "url"),
				Quote:       getString(vm, "quote"),
				Institution: getString(vm, "institution"),
				Date:        getString(vm, "date"),
				Kind:        normalizeEvidenceKind(getString(vm, "kind")),
				LaneNote:    getString(vm, "lane_note"),
			})
		}
	}
	return d
}

func parseInsight(v any) Insight {
	im, ok := v.(map[string]any)
	if !ok {
		return Insight{}
	}
	return Insight{
		Cert:        getString(im, "cert"),
		Title:       getString(im, "title"),
		Logic:       getString(im, "logic"),
		Evidence:    parseRefs(im["evidence"]),
		WebVerified: parseRefs(im["web_verified"]),
	}
}

func parseRefs(v any) []Ref {
	raw, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]Ref, 0, len(raw))
	for _, r := range raw {
		if rm, ok := r.(map[string]any); ok {
			if ref := parseRef(rm); ref != nil {
				out = append(out, *ref)
			}
		}
	}
	return out
}

func parseRef(m map[string]any) *Ref {
	ref := getString(m, "ref")
	st := getString(m, "source_type")
	if ref == "" && st == "" {
		return nil
	}
	return &Ref{SourceType: st, Ref: ref, Quote: getString(m, "quote")}
}

func getString(m map[string]any, key string) string {
	v, _ := m[key].(string)
	return v
}

func getBool(m map[string]any, key string) bool {
	v, _ := m[key].(bool)
	return v
}

// ── Review Judge ────────────────────────────────────────────────────────────

// runReviewJudge compares previous and current enrichment results.
// See design.md §4.3 and spec "分析认知循环 review judge".
func (o *OrchestratorService) runReviewJudge(ctx context.Context, sessionID, prevResultJSON, currResultJSON string) (*ReviewJudgeOutput, error) {
	prompt := fmt.Sprintf(reviewJudgePrompt, prevResultJSON, currResultJSON)

	resp, err := o.airouter.Chat(ctx, airouter.ChatRequest{
		Capability:  o.capability,
		Operation:   "data_enrichment.review_judge",
		SessionID:   sessionID,
		Messages:    []airouter.Message{{Role: "user", Content: prompt}},
		Temperature: floatPtr(0.2),
		JSONMode:    true,
	})
	if err != nil {
		return nil, fmt.Errorf("review judge chat: %w", err)
	}

	parsed, err := ParseJSONResponse(resp.Content)
	if err != nil {
		return nil, fmt.Errorf("review judge parse: %w", err)
	}

	return parseReviewJudgeOutput(parsed), nil
}

// parseReviewJudgeOutput maps one parsed LLM payload onto ReviewJudgeOutput.
// Shared by the topic insight judge and the board_brief judge (task 3.5).
func parseReviewJudgeOutput(parsed map[string]any) *ReviewJudgeOutput {
	shouldReview, _ := parsed["should_review"].(bool)
	reason, _ := parsed["reason"].(string)
	affectedContext, _ := parsed["affected_context"].(string)
	confidence, _ := parsed["confidence"].(float64)

	return &ReviewJudgeOutput{
		ShouldReview:    shouldReview,
		Reason:          reason,
		NewFindings:     parseStringSlice(parsed["new_findings"]),
		Overturned:      parseStringSlice(parsed["overturned"]),
		ConfidenceShift: parseConfidenceShift(parsed["confidence_shift"]),
		AffectedContext: affectedContext,
		Confidence:      confidence,
	}
}

func parseStringSlice(v any) []string {
	raw, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(raw))
	for _, s := range raw {
		if str, ok := s.(string); ok && str != "" {
			out = append(out, str)
		}
	}
	return out
}

func parseConfidenceShift(v any) []map[string]any {
	raw, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]map[string]any, 0, len(raw))
	for _, s := range raw {
		if sm, ok := s.(map[string]any); ok {
			out = append(out, sm)
		}
	}
	return out
}

// ── Helpers ─────────────────────────────────────────────────────────────────

// generateSessionID creates a session ID like "data_enrichment_{topicID}_{uuid8}".
func generateSessionID(topicID uint) string {
	uuid8 := RandomHex(8)
	return fmt.Sprintf("data_enrichment_%d_%s", topicID, uuid8)
}

// RandomHex returns n random hex characters using crypto/rand.
// Falls back to math/rand if crypto/rand fails (session IDs do not require
// cryptographic strength). Never panics on nil index.
func RandomHex(n int) string {
	const chars = "0123456789abcdef"
	b := make([]byte, n)
	for i := range b {
		idx, err := rand.Int(rand.Reader, big.NewInt(int64(len(chars))))
		if err != nil {
			//nolint:gosec // intentional fallback — session IDs don't require crypto strength
			b[i] = chars[mrand.Intn(len(chars))]
		} else {
			b[i] = chars[idx.Int64()]
		}
	}
	return string(b)
}

func floatPtr(f float64) *float64 { return &f }

// dedupKeyFor creates a stable deduplication key from tool name + canonical args JSON.
// Go's json.Marshal on map[string]any sorts keys alphabetically, providing canonical form.
func dedupKeyFor(toolName string, args map[string]any) string {
	canonical, _ := json.Marshal(args)
	return toolName + "\x00" + string(canonical)
}

func argsToJSON(args map[string]any) string {
	b, _ := json.Marshal(args)
	return string(b)
}

// explorationToolNames are the always-on tools the agent loop gets regardless
// of board source_type: multi-level board/lane navigation plus the web_search
// external-knowledge fallback (fetch_page is added once the readability backend
// is wired). No per-source_type conditional tools exist anymore — the A-share
// financial tools were removed when the direction shifted to structured depth.
var explorationToolNames = []string{"list_boards", "list_lanes", "get_lane_detail", "web_search", "fetch_page", "search_internal_context"}

// buildAgentAllowedTools returns the effective allowed-tools list for the agent
// loop: the always-on exploration entry points + web_search, plus the board's
// configured source-typed tools (cfg.AllowedTools, currently always empty after
// the financial removal — ToolsForSourceType always returns nil; the mechanism
// is retained as an extension point for future structured external sources).
// Dedup preserves first-seen order.
func (o *OrchestratorService) buildAgentAllowedTools(configuredTools []string) []string {
	seen := make(map[string]bool, len(explorationToolNames)+len(configuredTools))
	out := make([]string, 0, len(explorationToolNames)+len(configuredTools))
	for _, t := range explorationToolNames {
		if !seen[t] {
			seen[t] = true
			out = append(out, t)
		}
	}
	for _, t := range configuredTools {
		if !seen[t] {
			seen[t] = true
			out = append(out, t)
		}
	}
	return out
}

// buildToolsDesc renders tool descriptions for the agent system prompt.
// Only includes tools in allowedTools; returns empty string if allowedTools is empty.
// Ported from PoC roles.py:_build_tools_desc.
func buildToolsDesc(registry *Registry, allowedTools []string) string {
	if len(allowedTools) == 0 {
		return ""
	}
	allowedSet := make(map[string]bool, len(allowedTools))
	for _, t := range allowedTools {
		allowedSet[t] = true
	}

	parts := make([]string, 0)
	for name, tool := range registry.Tools() {
		if !allowedSet[name] {
			continue
		}
		schemaJSON, _ := json.Marshal(tool.InputSchema)
		parts = append(parts, fmt.Sprintf("**%s**: %s\n  参数: %s", name, tool.Description, string(schemaJSON)))
	}
	return strings.Join(parts, "\n\n")
}

func prefix(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

// ── Context layers ──────────────────────────────────────────────────────────

// contextSnapshot wraps metadata about the context layers read.
type contextSnapshot struct {
	Layers map[string]contextLayerInfo `json:"layers"`
}

type contextLayerInfo struct {
	AsOf string `json:"as_of"`
}

// inputSnapshotJSON is the shape stored in TopicEnrichmentResult.InputSnapshot.
type inputSnapshotJSON struct {
	ContextLayers map[string]contextLayerInfo `json:"context_layers"`
	LifelineRange lifelineRange               `json:"lifeline_range_section"`
	ReviewIDs     []uint                      `json:"review_ids"`
	WindowDays    int                         `json:"window_days"`
	ConfigLayers  []string                    `json:"config_context_layers"`
}

type lifelineRange struct {
	From string `json:"from,omitempty"`
	To   string `json:"to,omitempty"`
}

// readContextLayers reads pre-computed context summaries from topic_lifeline_context.
// Groups by granularity, selects the LATEST period for each layer (not all historical).
// Returns concatenated text for prompts and a snapshot for traceability.
func (o *OrchestratorService) readContextLayers(ctx context.Context, topicID uint, layers []string) (string, *contextSnapshot, error) {
	contexts, err := o.repo.ListTopicLifelineContextsByTopic(ctx, topicID)
	if err != nil {
		return "", nil, fmt.Errorf("read context layers: %w", err)
	}

	// Group by granularity, keep the MAX period per granularity (latest).
	byGran := make(map[string]*repository.TopicLifelineContext)
	for i := range contexts {
		lc := &contexts[i]
		existing, ok := byGran[lc.Granularity]
		if !ok || lc.Period > existing.Period {
			byGran[lc.Granularity] = lc
		}
	}

	// Filter by requested layers, skip ungenerated ones.
	snap := &contextSnapshot{Layers: make(map[string]contextLayerInfo)}
	var parts []string
	for _, layer := range layers {
		lc, ok := byGran[layer]
		if !ok {
			// Layer not generated yet — skip (don't fail).
			continue
		}
		parts = append(parts, fmt.Sprintf("## %s 层级上下文 (period=%s, 截止 %s)\n%s", layer, lc.Period, lc.AsOfDate.Format("2006-01-02"), lc.Content))
		snap.Layers[layer] = contextLayerInfo{AsOf: lc.AsOfDate.Format("2006-01-02")}
	}

	return strings.Join(parts, "\n\n"), snap, nil
}

// buildInputSnapshot creates the traceability snapshot for the result.
func buildInputSnapshot(snap *contextSnapshot, reviewIDs []uint, windowDays int, configLayers []string) *inputSnapshotJSON {
	return &inputSnapshotJSON{
		ContextLayers: snap.Layers,
		LifelineRange: lifelineRange{}, // populated when lifeline data has date range
		ReviewIDs:     reviewIDs,
		WindowDays:    windowDays,
		ConfigLayers:  configLayers,
	}
}

// ── helper ────────────────────────────────────────────────────────────────

// extractForm extracts the "form" field from the composite sectors JSON
// ({form,lens,analysis}). Returns empty string if sectors is old-format/empty
// or form is missing. Guards review judge against running on
// pre-causal-analysis-agent data (old evolution-positioning sectors had no
// form field). Spec "分析认知对比".
func extractForm(sectorsJSON json.RawMessage) string {
	if len(sectorsJSON) == 0 {
		return ""
	}
	var m map[string]any
	if err := json.Unmarshal(sectorsJSON, &m); err != nil {
		return ""
	}
	form, _ := m["form"].(string)
	return form
}
