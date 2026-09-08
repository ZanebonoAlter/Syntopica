package service

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
)

// ── 共享调查研究循环（tasks 4.2/4.4，design D4/D7，test-cases M5）────────────
//
// D4：一个共享研究 agent 接收全部假设与证据需求，统一调用内部工具、
// web_search、fetch_page——绝不按假设各跑一套循环。研究不预设赢家：
// 支持与反证同等重要，证据不足是允许的结论（只要纪律尝试过）。
//
// 复用而非复制 runToolLoop：/no_think、allowed tools guard、dedupKeyFor、
// 完整 ResultFull、maxAgentLoops=6、工具错误 JSON 降级全部来自共享循环本体，
// 通过可选 toolLoopPolicy 钩子注入调查纪律（声明校验 / finish 门 / 观察），
// 三防御不会漂移；policy=nil 的旧调用（topic/QA）行为不变（有回归测试）。
//
// 纪律契约（spec「调研同时寻找支持与反证」）：
//   - call_tool 必须声明 purpose=neutral|support|counter 与目标 hypothesis_ids
//     （neutral 可空）；support 至少一个目标且必须存在；counter 必须且只能
//     声明一个非零假设 id——多目标/空目标/零假设目标一律执行前拦截，
//     不计 attempt，每个非零假设由不同的实际执行调用满足 coverage。
//   - finish 前：至少一次独立中性事实核查；每个非零假设至少一次
//     counter/替代解释核查尝试。H0 可被 support/counter 但无 counter 配额。
//   - 重复拦截 / policy 拦截不冒充尝试（ObserveCall 只在真正执行后触发）；
//     外部工具未配置或失败算尝试，但如实落 gap，不伪造证据。
//   - 中性查询照抄假设 label 被机械拒绝（保守判定，宁放行复杂正常查询）。
//   - 幽灵 lane（get_lane_detail 白名单外 lane_id）执行前拦截并留痕。
//
// 韧性（D7）：finish 即使证据不足也可成功（纪律已尝试即可）；max loops /
// LLM / 工具失败返回 partial result + gaps，不抛错阻断后续综合；只有
// context 取消才返回 error。gap 只带稳定 reason 枚举 + tool/step/假设 id，
// 完整错误留在 ToolCallRecord.ResultFull 与 ai 日志，不泄入快照。
//
// 输入边界：本层只接收「问题 + 父简报投影 + 泳道白名单 + 全部初始假设 +
// 方法派生证据需求」；方法卡原文、作者文风、winner 一律不进入（prompt
// 装配函数无对应参数，结构上保证）。operation 复用 data_enrichment.tool_use
// （ai-logging.md 版块调查链 session 规则明列 research loop 用该 operation）。

// boardInvestigationResearchOperation 沿用共享查询员 operation：调查链
// session 规则（ai-logging.md）将 tool_use 列为调查回放组成（M10.3）。
const boardInvestigationResearchOperation = "data_enrichment.tool_use"

// Research purpose 枚举（agent 每次 call_tool 必须声明）。
const (
	ResearchPurposeNeutral = "neutral" // 不偏向任何假设的中性事实核查
	ResearchPurposeSupport = "support" // 支持某假设
	ResearchPurposeCounter = "counter" // 反证 / 替代解释 / 削弱某假设
)

// 研究 gap 稳定 reason 枚举（落 snapshot 安全；完整错误只在
// ToolCallRecord.ResultFull / ai_call_logs）。
const (
	researchGapToolUnavailable = "tool_unavailable" // 工具未配置（web/fetch 后端缺失等）
	researchGapToolError       = "tool_error"       // 工具执行失败/参数错误
	researchGapMissingNeutral  = "missing_neutral"  // 全程无独立中性事实核查
	researchGapMissingCounter  = "missing_counter"  // 某非零假设无 counter 尝试
	researchGapMaxLoops        = "max_loops"        // 循环上限耗尽未完成
	researchGapLLMError        = "llm_error"        // LLM 调用/解析失败
)

// 中性查询照抄判定的保守参数。
const (
	// boardNeutralQueryMinLabelRunes：短于该长度的 label 不参与包含判定
	// （避免琐碎短词误伤正常查询）；相等判定不受此限。
	boardNeutralQueryMinLabelRunes = 6
	// boardNeutralQueryPaddingRunes：查询只比 label 多不超过该 rune 数
	// （“查一下X”“X的证据”类垫字）仍视为照抄；复杂事实查询自然放行。
	boardNeutralQueryPaddingRunes = 8
	// boardResearchTopicMaxRunes：AgentLoopResult.Topic 的问句截断上限。
	boardResearchTopicMaxRunes = 80
)

// BoardInvestigationResearchInput is the contract for the shared research
// loop. 它承载 4.3 阶段产物（问题 + 全部初始假设）与调查编排装配的
// 父简报投影/泳道白名单/方法派生证据需求。刻意不设方法卡正文、作者
// 文风、winner 字段——研究 agent 与修辞隔离（D6）。
type BoardInvestigationResearchInput struct {
	SessionID     string                     `json:"session_id"`
	Question      BoardInvestigationQuestion `json:"question"`
	Brief         *BoardBriefPayload         `json:"brief"`          // 父简报投影源
	LaneWhitelist []uint                     `json:"lane_whitelist"` // 活跃泳道白名单（get_lane_detail 守门）
	// DynamicGrants carries the session-scoped grant set shared with the
	// orchestrating investigation (created in InvestigateBoardQuestion). nil →
	// a private set is created here so the loop still works standalone.
	DynamicGrants *DynamicLaneGrantSet `json:"-"`
	Hypotheses    []boardHypothesis    `json:"hypotheses"`              // 全部初始假设（一个 loop 统一服务）
	EvidenceNeeds []string             `json:"evidence_needs"`          // 由已选方法卡派生的证据需求（非方法正文）
	AllowedTools  []string             `json:"allowed_tools,omitempty"` // 空 = explorationToolNames
}

// boardResearchGap is one structured, snapshot-safe research gap.
type boardResearchGap struct {
	Reason        string   `json:"reason"`                   // 稳定枚举，见 researchGap* 常量
	Tool          string   `json:"tool,omitempty"`           // 工具类 gap 的工具名
	HypothesisIDs []string `json:"hypothesis_ids,omitempty"` // 目标假设（missing_counter 单假设/工具类沿用声明）
	Step          int      `json:"step,omitempty"`           // 工具类 gap 的循环步号
}

// boardResearchCoverage summarizes which discipline attempts were made.
// 「尝试」= 真正执行过的调用（拦截不计）；执行但工具失败也算尝试（gap 另记）。
type boardResearchCoverage struct {
	NeutralAttempted             bool     `json:"neutral_attempted"`
	CounterAttemptedByHypothesis []string `json:"counter_attempted_by_hypothesis"` // 有 ≥1 次 counter 尝试的假设 id（输入顺序）
}

// BoardInvestigationResearchResult is the reusable artifact for 4.5
// (synthesize 消费 Loop/FinalData/Coverage/Gaps) and 4.6（tool_calls 与
// gaps 固化进 investigation sectors/input_snapshot）。
type BoardInvestigationResearchResult struct {
	Loop             *AgentLoopResult      `json:"loop"` // 完整工具调用顺序（含 purpose/hypothesis_ids/outcome/blocked_reason）+ FinalData + Loops
	Coverage         boardResearchCoverage `json:"coverage"`
	Gaps             []boardResearchGap    `json:"gaps"`
	FinishRejections []string              `json:"finish_rejections,omitempty"` // finish 被纪律门拒绝的反馈留痕（安全摘要）
}

// ── Prompt ──────────────────────────────────────────────────────────────────

const boardInvestigationResearchPromptLead = `你是一位证据纪律严明的研究员，正在为一次板块深度调查执行共享检索：全部竞争假设共用这一个研究循环（绝不按假设各跑一套）。研究不预设赢家：支持证据、反证与替代解释同等重要，证据不足是允许的结论。`

const boardInvestigationResearchDiscipline = `

工作纪律：
1. lane 优先：先核对相关泳道的内部事实（get_lane_detail，lane_id 只能取下方白名单编号），内部材料不足再 web_search / fetch_page 外部核查
2. 每次 call_tool 必须声明 purpose（neutral=不偏向任何假设的中性事实核查 / support=支持某假设 / counter=反证、替代解释或削弱）与 hypothesis_ids（目标假设 id）：neutral 可空数组；support 至少一个目标；counter 必须且只能声明一个非零假设 id——一次只反证一个假设，多目标、空目标、以零假设为目标都会被拦截；同 tool 同参数的调用只能算一次尝试，反证不同假设请换查询角度
3. 宣布完成前必须满足：至少一次独立的中性事实核查（neutral，查询不得照抄任何假设的结论表述）；每个非零假设（is_null=false）至少一次反证/替代解释核查尝试（counter）。零假设可被支持或削弱，但无反证配额。未满足的 finish 会被拦下并给出反馈
4. 中性查询不得照抄任何假设的结论表述——照抄假设结论词的 neutral 查询会被机械拦截；被拦后把查询改写为事实性问题（对象/时间/数字/机构）
5. 工具结果完整返回；同 tool 同参数重复调用会被拦截——请换角度或换关键词，不要重复
6. 工具未配置或失败会返回错误 JSON：这仍算一次尝试——继续研究，在 finish 总结里如实说明缺口；不得伪造证据，也不得把 web_search 的 snippet 当作 fetch_page 的原文摘录
7. 泳道质量/密度信号只影响材料详略排序，不是证据；不得把质量分当关系或结论依据
8. 证据优先级：与问题直接相关的一手依据 > 可核查二手材料 > 背景新闻；没有证据类型配额，检索不到历史材料时不强行类比

每一轮输出严格 JSON，二选一：
- 继续调工具：{"action":"call_tool","thought":"...","tool":"工具名","args":{...},"purpose":"neutral|support|counter","hypothesis_ids":["h1"]}
- 宣布完成：{"action":"finish","thought":"...","summary":"按假设分组的素材汇总：每个假设的支持证据、反证与缺口，含可核查来源（URL/原文摘录/泳道 lane_id）"}

不要输出 JSON 以外的任何内容。`

// assembleBoardInvestigationResearchPrompt builds the research system prompt
// (pure function, contract unit-tested: 问题/假设/证据需求/泳道白名单入，
// 方法卡正文/作者文风/赢家结构上无从进入)。
func assembleBoardInvestigationResearchPrompt(in BoardInvestigationResearchInput, toolsDesc string) string {
	var sb strings.Builder
	sb.WriteString(boardInvestigationResearchPromptLead)
	fmt.Fprintf(&sb, "\n\n---\n调查问题：%s（来源：%s）", in.Question.Text, in.Question.Source)
	sb.WriteString("\n\n---\n竞争假设（初始集合，id 用于 hypothesis_ids 声明）：")
	for _, h := range in.Hypotheses {
		tag := "竞争假设"
		if h.IsNull {
			tag = "零假设"
		}
		fmt.Fprintf(&sb, "\n- %s [%s] %s", h.ID, tag, h.Label)
		if len(h.SupportNeeded) > 0 {
			fmt.Fprintf(&sb, "\n  支持需要: %s", strings.Join(h.SupportNeeded, "；"))
		}
		if len(h.DisconfirmNeeded) > 0 {
			fmt.Fprintf(&sb, "\n  削弱需要: %s", strings.Join(h.DisconfirmNeeded, "；"))
		}
		if h.Scope != "" {
			fmt.Fprintf(&sb, "\n  范围: %s", h.Scope)
		}
	}
	if len(in.EvidenceNeeds) > 0 {
		sb.WriteString("\n\n---\n方法派生证据需求（检查清单提示，无配额、与问题无关可忽略）：")
		for _, n := range in.EvidenceNeeds {
			sb.WriteString("\n- " + n)
		}
	}
	sb.WriteString("\n\n---\n泳道白名单（get_lane_detail 只允许这些 lane_id）：" + renderResearchLaneWhitelist(in.LaneWhitelist))
	sb.WriteString("\n\n---\n父简报投影：\n" + renderBriefProjectionForInvestigation(in.Brief))
	sb.WriteString("\n\n---\n可用工具：\n" + toolsDesc)
	sb.WriteString(boardInvestigationResearchDiscipline)
	return sb.String()
}

func renderResearchLaneWhitelist(ids []uint) string {
	if len(ids) == 0 {
		return "（无）"
	}
	parts := make([]string, 0, len(ids))
	for _, id := range ids {
		parts = append(parts, fmt.Sprintf("#%d", id))
	}
	return strings.Join(parts, "、")
}

// deriveLaneWhitelistFromBrief mechanically derives the lane whitelist from
// the parent brief's immutable snapshot: every lane id referenced by
// lane_refs / observations / relationships / research_questions（去重、升序
// 稳定排序）。父简报是泳道事实的唯一来源（review 4.4）。
func deriveLaneWhitelistFromBrief(brief *BoardBriefPayload) []uint {
	if brief == nil {
		return nil
	}
	seen := make(map[uint]bool)
	ids := make([]uint, 0, len(brief.LaneRefs))
	add := func(id uint) {
		if id == 0 || seen[id] {
			return
		}
		seen[id] = true
		ids = append(ids, id)
	}
	for _, lr := range brief.LaneRefs {
		add(lr.LaneID)
	}
	for _, o := range brief.Observations {
		add(o.LaneID)
	}
	for _, r := range brief.Relationships {
		for _, id := range r.LaneIDs {
			add(id)
		}
	}
	for _, q := range brief.ResearchQuestions {
		for _, id := range q.RelatedLaneIDs {
			add(id)
		}
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}

// effectiveInvestigationLaneWhitelist resolves the whitelist actually used
// by BOTH the prompt and the guard（同源）. The parent brief's derived set is
// authoritative: a caller-provided whitelist may only narrow/confirm it
// (intersection) and can never introduce lanes absent from the brief（幽灵
// lane 进不来）。输入空 + 父简报有 lane → 自动使用父简报集合；父简报无
// lane → 空白名单（父简报外无可查泳道）。
func effectiveInvestigationLaneWhitelist(brief *BoardBriefPayload, input []uint) []uint {
	derived := deriveLaneWhitelistFromBrief(brief)
	if len(input) == 0 {
		return derived
	}
	allowed := make(map[uint]bool, len(derived))
	for _, id := range derived {
		allowed[id] = true
	}
	seen := make(map[uint]bool, len(input))
	out := make([]uint, 0, len(input))
	for _, id := range input {
		if id == 0 || seen[id] || !allowed[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// ── 调查纪律 policy（runToolLoop 的可选决策钩子实现）────────────────────────

// investigationPolicy enforces the D4/D7 research discipline inside the
// shared runToolLoop. It is the only toolLoopPolicy implementation; state is
// confined to one research run.
type investigationPolicy struct {
	laneWhitelist map[uint]bool
	// dynamicGrants holds session-scoped get_lane_detail grants added at
	// runtime by trusted tool results (search_internal_context / list_lanes).
	// nil = no dynamic authorization (legacy behaviour unchanged).
	dynamicGrants *DynamicLaneGrantSet
	hypotheses    map[string]boardHypothesis
	hypoOrder     []string // 输入顺序的全部假设 id（coverage 输出用）
	nonNullOrder  []string // 非零假设 id（counter 配额只约束它们）
	normLabels    []struct {
		norm  string
		runes int
	}

	neutralAttempted bool
	counterBy        map[string]bool
	executed         []boardResearchExecutedCall
	finishRejections []string
	presetGaps       []boardResearchGap // 工具集层面预置的 tool_unavailable（每工具一条，见 presetToolGaps）
}

// boardResearchExecutedCall is one observed (actually executed) tool call.
type boardResearchExecutedCall struct {
	Step          int
	Tool          string
	Purpose       string
	HypothesisIDs []string
	ResultFull    string
}

func newInvestigationPolicy(hypotheses []boardHypothesis, laneWhitelist []uint) *investigationPolicy {
	p := &investigationPolicy{
		laneWhitelist: make(map[uint]bool, len(laneWhitelist)),
		hypotheses:    make(map[string]boardHypothesis, len(hypotheses)),
		counterBy:     map[string]bool{},
	}
	for _, id := range laneWhitelist {
		p.laneWhitelist[id] = true
	}
	seen := map[string]bool{}
	for _, h := range hypotheses {
		id := strings.TrimSpace(h.ID)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		p.hypotheses[id] = h
		p.hypoOrder = append(p.hypoOrder, id)
		if !h.IsNull {
			p.nonNullOrder = append(p.nonNullOrder, id)
		}
		n := normalizeResearchQuery(h.Label)
		p.normLabels = append(p.normLabels, struct {
			norm  string
			runes int
		}{norm: n, runes: len([]rune(n))})
	}
	return p
}

// normalizeResearchQuery: trim + 空白折叠 + ASCII 小写 + 去包裹引号/括号/
// 标点。刻意不做分词或子串模糊匹配（保守判定，宁放行复杂正常查询）。
func normalizeResearchQuery(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.Join(strings.Fields(s), " ")
	s = strings.Trim(s, "\"'“”‘’『』「」()（）[]【】.,，。!！?？;；:：")
	return strings.TrimSpace(s)
}

// isConclusionCopy 机械判定「中性查询照抄假设结论」：归一化后相等，或
// 查询含完整 label 且只比 label 多少量垫字。保守宁松不宁紧。
func (p *investigationPolicy) isConclusionCopy(query string) bool {
	nq := normalizeResearchQuery(query)
	if nq == "" {
		return false
	}
	qr := len([]rune(nq))
	for _, nl := range p.normLabels {
		if nq == nl.norm {
			return true
		}
		if nl.runes >= boardNeutralQueryMinLabelRunes &&
			strings.Contains(nq, nl.norm) &&
			qr <= nl.runes+boardNeutralQueryPaddingRunes {
			return true
		}
	}
	return false
}

// scrubIDList: []any → 去空格/去重后的 id 列表。
func scrubIDList(raw any) []string {
	list, ok := raw.([]any)
	if !ok {
		return nil
	}
	out := []string{}
	seen := map[string]bool{}
	for _, v := range list {
		s, ok := v.(string)
		if !ok {
			continue
		}
		s = strings.TrimSpace(s)
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}

func (p *investigationPolicy) hypothesisIDText() string {
	return strings.Join(p.hypoOrder, "、")
}

// CheckCall validates the declaration + tool-specific discipline BEFORE the
// tool executes. Blocked verdicts never execute and never consume the dedup
// key; the feedback tells the agent how to rewrite.
func (p *investigationPolicy) CheckCall(_ int, decision map[string]any) toolCallVerdict {
	toolName, _ := decision["tool"].(string)
	args, _ := decision["args"].(map[string]any)
	if args == nil {
		args = map[string]any{}
	}

	purpose := strings.TrimSpace(getString(decision, "purpose"))
	if purpose != ResearchPurposeNeutral && purpose != ResearchPurposeSupport && purpose != ResearchPurposeCounter {
		return toolCallVerdict{
			Blocked:       true,
			BlockedReason: "invalid_purpose",
			Feedback:      "每次调用必须声明 purpose=neutral（中性事实核查）/ support（支持某假设）/ counter（反证或替代解释）。请补上 purpose 后重新调用。",
		}
	}

	hypIDs := scrubIDList(decision["hypothesis_ids"])
	for _, id := range hypIDs {
		if _, ok := p.hypotheses[id]; !ok {
			return toolCallVerdict{
				Blocked:       true,
				BlockedReason: "invalid_hypothesis_target",
				Feedback:      fmt.Sprintf("hypothesis_ids 中的 %q 不在本次调查的假设集合内（可用 id：%s）。", id, p.hypothesisIDText()),
			}
		}
	}
	if len(hypIDs) == 0 && purpose != ResearchPurposeNeutral {
		if purpose == ResearchPurposeCounter {
			// counter 空目标：与多目标/零假设目标同一拦截类别（review 4.2）。
			return toolCallVerdict{
				Blocked:       true,
				BlockedReason: "invalid_counter_target",
				Feedback:      fmt.Sprintf("counter 调用必须且只能声明一个非零假设 id（可用 id：%s）；一次反证一个假设。", p.hypothesisIDText()),
			}
		}
		return toolCallVerdict{
			Blocked:       true,
			BlockedReason: "invalid_hypothesis_target",
			Feedback:      fmt.Sprintf("support 调用必须声明至少一个目标 hypothesis_ids（可用 id：%s）；neutral 可为空数组。", p.hypothesisIDText()),
		}
	}
	// counter 声明纪律（review 4.2）：必须且只能一个非零假设——多目标、
	// 零假设目标在执行前拦截，不计 attempt；每个非零假设因此必须由不同
	// 的实际执行调用满足 coverage（同 tool 同 args 换 target 仍被 dedup
	// 挡，不能刷配额）。
	if purpose == ResearchPurposeCounter {
		if len(hypIDs) > 1 {
			return toolCallVerdict{
				Blocked:       true,
				BlockedReason: "invalid_counter_target",
				Feedback:      fmt.Sprintf("counter 调用一次只能反证一个非零假设（收到 %d 个：%s）。请拆成多次调用，每次换一个角度反证一个假设。", len(hypIDs), strings.Join(hypIDs, "、")),
			}
		}
		if h, ok := p.hypotheses[hypIDs[0]]; ok && h.IsNull {
			return toolCallVerdict{
				Blocked:       true,
				BlockedReason: "invalid_counter_target",
				Feedback:      fmt.Sprintf("counter 调用不能以零假设 %s 为目标（零假设无反证配额）。请反证非零假设：%s。", hypIDs[0], strings.Join(p.nonNullOrder, "、")),
			}
		}
	}

	if toolName == "get_lane_detail" {
		rawLane := args["lane_id"]
		laneID, ok := toUint(rawLane)
		if !ok {
			// 类型错误与白名单外分开反馈：不误称合法数字 id「不存在」（review 4.4）。
			return toolCallVerdict{
				Blocked:       true,
				BlockedReason: "ghost_lane",
				Feedback:      fmt.Sprintf("lane_id 必须是白名单内的数字编号（收到 %v，不是合法数字）。允许：%s。请用数字 lane_id 重新调用。", rawLane, renderResearchLaneWhitelist(whitelistOrder(p))),
			}
		}
		if !p.laneAllowed(laneID) {
			return toolCallVerdict{
				Blocked:       true,
				BlockedReason: "ghost_lane",
				Feedback:      fmt.Sprintf("lane_id=%d 不在本次调查的泳道白名单内（允许：%s）。请先用 search_internal_context 或 list_lanes 发现并授权泳道，或改用 web_search。", laneID, renderResearchLaneWhitelist(whitelistOrder(p))),
			}
		}
	}

	if purpose == ResearchPurposeNeutral && toolName == "web_search" {
		query, _ := args["query"].(string)
		if p.isConclusionCopy(query) {
			return toolCallVerdict{
				Blocked:       true,
				BlockedReason: "neutral_query_conclusion_copy",
				Feedback:      "该中性查询照抄了某个假设的结论表述——中性核查必须用独立的事实性问法（对象/时间/数字/机构），不得复述假设结论词。请改写查询后重试。",
			}
		}
	}

	return toolCallVerdict{Purpose: purpose, HypothesisIDs: hypIDs}
}

// laneAllowed reports whether get_lane_detail(laneID) may execute: the
// static parent-brief whitelist OR a session-scoped dynamic grant issued by a
// trusted tool result. Model-guessed ids outside both sets stay blocked
// (spec: 猜测的泳道仍被拦截).
func (p *investigationPolicy) laneAllowed(laneID uint) bool {
	return p.laneWhitelist[laneID] || p.dynamicGrants.Has(laneID)
}

// whitelistOrder rebuilds the whitelist in the original input order for
// feedback rendering (map iteration order is unstable).
func whitelistOrder(p *investigationPolicy) []uint {
	ids := make([]uint, 0, len(p.laneWhitelist))
	for id := range p.laneWhitelist {
		ids = append(ids, id)
	}
	// 单用户小集合（≤12 泳道），简单排序足够。
	for i := 0; i < len(ids); i++ {
		for j := i + 1; j < len(ids); j++ {
			if ids[j] < ids[i] {
				ids[i], ids[j] = ids[j], ids[i]
			}
		}
	}
	// Dynamic grants render alongside the static whitelist so the agent sees
	// every lane it may call this session.
	for _, id := range p.dynamicGrants.GrantedIDs() {
		if !p.laneWhitelist[id] {
			ids = append(ids, id)
		}
	}
	return ids
}

// presetToolGaps records one tool_unavailable gap per external verification
// tool (web_search / fetch_page) absent from the RESOLVED allowed set — even
// if the agent never calls the tool, the result must state that external
// verification was unavailable（review 4.4）. One gap per tool（去重）；被
// 排除的工具不可能产生执行 gap（allowed-tools guard 在 Execute 前拦截），
// 天然不重复；预置 gap 永不增加 finish 纪律要求。默认 explorationToolNames
// 两个外部工具都在，不会预置。
func (p *investigationPolicy) presetToolGaps(allowedTools []string) {
	allowed := make(map[string]bool, len(allowedTools))
	for _, t := range allowedTools {
		allowed[t] = true
	}
	for _, name := range []string{"web_search", "fetch_page"} {
		if !allowed[name] {
			p.presetGaps = append(p.presetGaps, boardResearchGap{Reason: researchGapToolUnavailable, Tool: name})
		}
	}
}

// ObserveCall only fires for actually-executed calls（拦截不冒充尝试）。
// 工具失败（错误 JSON）也算尝试：gap 由 buildResearchResult 另行落档。
func (p *investigationPolicy) ObserveCall(step int, toolName string, _ map[string]any, resultFull, purpose string, hypothesisIDs []string) {
	p.executed = append(p.executed, boardResearchExecutedCall{
		Step: step, Tool: toolName, Purpose: purpose,
		HypothesisIDs: hypothesisIDs, ResultFull: resultFull,
	})
	// Trusted-tool results extend the session grant set (design D3): only
	// structured server responses may authorize lanes, never model text.
	p.dynamicGrants.ObserveTrustedResult(step, toolName, resultFull)
	if purpose == ResearchPurposeNeutral {
		p.neutralAttempted = true
	}
	if purpose == ResearchPurposeCounter {
		for _, id := range hypothesisIDs {
			if _, ok := p.hypotheses[id]; ok {
				p.counterBy[id] = true
			}
		}
	}
}

// CheckFinish gates finish on the mechanical discipline: ≥1 neutral attempt
// and, for every non-null hypothesis, ≥1 counter attempt. H0 has no counter
// quota. 被拒 finish 的反馈进入 agent 可见历史，循环继续。
func (p *investigationPolicy) CheckFinish(_ int, _ string) toolFinishVerdict {
	var missing []string
	if !p.neutralAttempted {
		missing = append(missing, "还缺至少一次独立的中性事实核查（purpose=neutral，查询不得照抄假设结论）")
	}
	for _, id := range p.nonNullOrder {
		if !p.counterBy[id] {
			missing = append(missing, fmt.Sprintf("非零假设 %s（%s）还缺一次反证/替代解释核查尝试（purpose=counter）", id, truncateRunes(p.hypotheses[id].Label, 40)))
		}
	}
	if len(missing) == 0 {
		return toolFinishVerdict{}
	}
	fb := "研究纪律未满足：" + strings.Join(missing, "；") + "。请继续调用工具补齐后再宣布完成。"
	p.finishRejections = append(p.finishRejections, fb)
	return toolFinishVerdict{Blocked: true, Feedback: fb}
}

// buildResearchResult assembles the reusable artifact. Gap 只带稳定 reason
// 枚举 + tool/step/假设 id；完整工具错误留在 ToolCallRecord.ResultFull，
// LLM 错误原文留在 AgentLoopResult.Error，均不进 gap。
func (p *investigationPolicy) buildResearchResult(loop *AgentLoopResult) *BoardInvestigationResearchResult {
	coverage := boardResearchCoverage{
		NeutralAttempted:             p.neutralAttempted,
		CounterAttemptedByHypothesis: []string{},
	}
	for _, id := range p.hypoOrder {
		if p.counterBy[id] {
			coverage.CounterAttemptedByHypothesis = append(coverage.CounterAttemptedByHypothesis, id)
		}
	}

	gaps := []boardResearchGap{}
	for _, c := range p.executed {
		errText := toolResultErrorText(c.ResultFull)
		if errText == "" {
			continue
		}
		reason := researchGapToolError
		if strings.Contains(errText, "未配置") || strings.Contains(errText, "not configured") {
			reason = researchGapToolUnavailable
		}
		gaps = append(gaps, boardResearchGap{
			Reason: reason, Tool: c.Tool, HypothesisIDs: c.HypothesisIDs, Step: c.Step,
		})
	}
	// 工具集层面预置：显式 AllowedTools 缺 web_search / fetch_page 时即使
	// agent 未调用也明确外部核查不可用（稳定 reason，不泄露内部细节）。
	gaps = append(gaps, p.presetGaps...)
	if !p.neutralAttempted {
		gaps = append(gaps, boardResearchGap{Reason: researchGapMissingNeutral})
	}
	for _, id := range p.nonNullOrder {
		if !p.counterBy[id] {
			gaps = append(gaps, boardResearchGap{Reason: researchGapMissingCounter, HypothesisIDs: []string{id}})
		}
	}
	// 终止分类：finish 成功时纪律必然已满足（missing_* 不可能出现）；未
	// finish 则按 loop.Error 的固定文案分类（同包文案，测试锚定）——LLM
	// 调用失败 / 输出无法解析 / action 不合法都是模型输出问题，归
	// llm_error；其余（达最大循环数未完成）归 max_loops（review 4.4）。
	if loop.FinalData == "" && loop.Error != "" {
		if strings.Contains(loop.Error, "LLM 调用失败") || strings.Contains(loop.Error, "无法解析") || strings.Contains(loop.Error, "action 不合法") {
			gaps = append(gaps, boardResearchGap{Reason: researchGapLLMError})
		} else {
			gaps = append(gaps, boardResearchGap{Reason: researchGapMaxLoops})
		}
	}

	return &BoardInvestigationResearchResult{
		Loop:             loop,
		Coverage:         coverage,
		Gaps:             gaps,
		FinishRejections: p.finishRejections,
	}
}

// ── 入口：单一共享研究循环 ───────────────────────────────────────────────────

// runBoardInvestigationResearch runs ONE shared tool loop serving all
// hypotheses. max loops / LLM / tool failures return a partial result with
// gaps (不抛错阻断综合)；只有 context 取消返回 error。
func (o *OrchestratorService) RunBoardInvestigationResearch(ctx context.Context, in BoardInvestigationResearchInput) (*BoardInvestigationResearchResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if strings.TrimSpace(in.Question.Text) == "" {
		return nil, errors.New("board investigation research: question text required")
	}
	if len(in.Hypotheses) == 0 {
		return nil, errors.New("board investigation research: hypotheses required")
	}

	allowedTools := in.AllowedTools
	if len(allowedTools) == 0 {
		allowedTools = explorationToolNames
	}
	// 泳道白名单以父简报不可变快照机械推导为唯一事实源：调用方输入只能
	// 收窄/确认，父简报外幽灵 lane 一律进不来；先解析再同时进 prompt 与
	// policy，保证两者同源（review 4.4）。
	in.LaneWhitelist = effectiveInvestigationLaneWhitelist(in.Brief, in.LaneWhitelist)
	policy := newInvestigationPolicy(in.Hypotheses, in.LaneWhitelist)
	if in.DynamicGrants == nil {
		in.DynamicGrants = NewDynamicLaneGrantSet()
	}
	policy.dynamicGrants = in.DynamicGrants
	policy.presetToolGaps(allowedTools)

	loop, err := runToolLoop(ctx, o.airouter, o.toolRegistry, o.capability, toolLoopParams{
		sessionID:    in.SessionID,
		systemPrompt: assembleBoardInvestigationResearchPrompt(in, buildToolsDesc(o.toolRegistry, allowedTools)),
		taskLine:     "调查问题: " + in.Question.Text,
		operation:    boardInvestigationResearchOperation,
		allowedTools: allowedTools,
		maxLoops:     maxAgentLoops,
		resultTopic:  truncateRunes(in.Question.Text, boardResearchTopicMaxRunes),
		policy:       policy,
	})
	if err != nil {
		return nil, err
	}
	if cerr := ctx.Err(); cerr != nil {
		return nil, fmt.Errorf("board investigation research canceled: %w", cerr)
	}
	return policy.buildResearchResult(loop), nil
}
