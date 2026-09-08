package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"syntopica-backend/internal/dataenrichment/repository"
	"syntopica-backend/internal/platform/airouter"
	"syntopica-backend/internal/platform/logging"
)

// ── board_synthesize：调查综合 + board_investigation 持久化（tasks 4.5/4.6）──
//
// design D4/D5/D7、test-cases M6。调查链的最后两步：
//
//  1. board_synthesize（4.5）：Router.Chat JSONMode，输入 = 调查问题 + 父简报
//     投影 + 初始假设 + 共享研究循环的 FinalData/完整工具调用/coverage/gaps +
//     实际注入的方法正文与注入留痕。输出调查专用 schema（hypotheses 五态
//     assessment / conclusion / 调查证据链 / lane_refs），可修改/合并/拆分假设
//     （derived_from 最小追溯），不预选赢家、不强制论文结构（无 thesis/
//     argument/depth/system_reframe/固定机制层/历史类比配额）。坏 JSON/非法
//     结构纠错重试一次；仍失败返回 error——绝不机械编造调查结论（对比 brief
//     的机械降级：调查的评估性结论没有诚实的降级产物）。
//
//  2. InvestigateBoardQuestion（4.6 编排 + 持久化）：enabled 预检 → 父简报
//     校验（同 board + kind=board_brief + sectors 可解析）→ question 归一 +
//     question_key → D10 补全门（lane 集来自父简报不可变快照；month/year
//     lifeline 补全，nonfatal，先于任何调查 LLM）→
//     HypothesizeBoardInvestigation（select→load→hypothesize）→
//     RunBoardInvestigationResearch（一个共享 loop，AllowedTools 空 = 完整
//     默认工具集；lane 集来自父简报不可变快照）→ board_synthesize → 一次
//     CreateBoardInvestigationResult。全部阶段成功并完成 sanitize 后才构造
//     不可变快照；任何中途失败 = 0 调查行、父简报不动。同一简报可派生多份
//     调查（同题重跑也允许）。不重跑 cards、不改写父 brief；补全门只更新
//     month/year lifeline（get_lane_detail 随后消费），报告固化进快照。
//
// 证据纪律（D7 + spec「调研同时寻找支持与反证」「调查结论可改写、拆分或放弃
// 假设」）：
//   - 证据 id + source_type ∈ news|web|page|lane + supports/counters 显式极性；
//     method 绝不可作 source；lane ref 只允许父简报白名单；web/page 强制
//     url+quote(原文逐字摘录)+institution+date，quote 保守机械 substring 核对
//     研究工具 ResultFull，核不到逐项剔除；幽灵/悬空/非法逐项丢弃并清理假设
//     引用；支持/反证分开，gap 保留。
//   - 机械质量护栏：「只有同向新闻转述」不得 high supported——高置信 supported
//     必须有可核查非 news（web/page）依据且 research 做过反证（本假设或其
//     derived_from 初始假设），否则降级 confidence 并写 gap + boundary。
//   - 同源核对（review 4.5 Important）：web/page 证据的 quote 与 url 必须
//     绑定到同一条研究工具调用的同一结果对象——page 只认 args.url 与
//     evidence.url 规范化相等的 fetch_page 调用；web 只认同一条
//     ResultFull 里 url 与 quote 同在一个结果对象；跨调用/跨对象拼凑一律
//     剔除。URL 规范化只做安全等价（trim/大小写/默认端口/尾斜杠），不同
//     query 永不合并。
//   - M13 一致性门：lane 证据 ref 缺失时读数值 lane_id 别名（float64 整数
//     (0,2^53) 归一十进制；ref 非空永不看别名）；存活证据 supports/counters
//     极性按 first-seen 并集回填假设引用（不改证据极性，受 max refs cap）；
//     supported 无存活 support、refuted/weakened 无存活 counter 在清洗后
//     升级为结构失败（纠错重试，两次仍败 0 行）——矛盾快照不落库。
//
// Snapshot 安全红线：input_snapshot 不携带内部 err——synthesize/hypothesize
// 的 RetryReason 都是稳定原因码（synthesisRetry* / hypothesisRetry*），完整
// 错误只在日志与 ai_call_logs（重试 prompt 中的纠错文本属调用留痕，进
// ai_call_logs 是规范要求，不进 snapshot）。

// boardSynthesizeOperation is the ai_call_logs operation for the synthesis
// LLM call（清单见 standard/backend/ai-logging.md）。
const boardSynthesizeOperation = "data_enrichment.board_synthesize"

// Hypothesis assessment enum（D4：五态，终局评估）。
const (
	HypothesisSupported    = "supported"
	HypothesisPlausible    = "plausible"
	HypothesisInsufficient = "insufficient"
	HypothesisWeakened     = "weakened"
	HypothesisRefuted      = "refuted"
)

var boardHypothesisAssessments = map[string]bool{
	HypothesisSupported:    true,
	HypothesisPlausible:    true,
	HypothesisInsufficient: true,
	HypothesisWeakened:     true,
	HypothesisRefuted:      true,
}

// Synthesis retry stable reason codes（snapshot 安全；完整 err 只进日志）。
const (
	synthesisRetryChat      = "chat_error"
	synthesisRetryParse     = "parse_error"
	synthesisRetryStructure = "invalid_structure"

	// synthesisRepairTerminalRootDelimiter marks the only mechanical JSON
	// repair accepted by board synthesis: every inner delimiter/string is
	// complete and the response only omitted the root object's final `}`.
	synthesisRepairTerminalRootDelimiter = "terminal_root_delimiter"
)

// Output caps（机械裁剪，防止把研究结果换一种形式全量复述）。
const (
	boardSynthesisMaxHypotheses    = 8 // 初始 ≤4，拆分/合并后放宽
	boardSynthesisMaxEvidence      = 24
	boardSynthesisMaxLabelRunes    = 300
	boardSynthesisMaxSummaryRunes  = 600
	boardSynthesisMaxScopeRunes    = 300
	boardSynthesisMaxBoundaryRunes = 800 // 护栏追加后仍受控
	boardSynthesisMaxQuoteRunes    = 400
	boardSynthesisMaxGapRunes      = 200
	boardSynthesisMaxGapItems      = 6
	boardSynthesisMaxEvidenceRefs  = 8
	boardSynthesisMaxDerivedFrom   = 4
	// boardSynthesisToolResultRunes：prompt 中单个工具结果的渲染预算。核对
	// quote 用完整 ResultFull（不受此限），这里只控制 prompt 体积。
	boardSynthesisToolResultRunes = 6000
)

// boardSynthesisQualityGuardGap is the stable per-hypothesis gap note stamped
// by the mechanical quality guard.
const boardSynthesisQualityGuardGap = "质量护栏：仅有同向转述证据或未完成反证核查，高置信评级被降级"

// boardSynthesisQualityGuardBoundary tags the boundary note appended when the
// guard downgrades at least one hypothesis.
const boardSynthesisQualityGuardBoundary = "质量护栏"

// ── 调查 schema 类型 ────────────────────────────────────────────────────────

// boardInvestigationHypothesis is one FINAL hypothesis with its assessment.
// DerivedFrom traces modifications back to initial hypothesis ids (M6.5)；
// unchanged hypotheses keep their initial id (stable id) with no DerivedFrom.
type boardInvestigationHypothesis struct {
	ID              string   `json:"id"`
	Label           string   `json:"label"`
	IsNull          bool     `json:"is_null"`
	DerivedFrom     []string `json:"derived_from"` // 同源初始假设 id（未改动=同 id 无 trace）
	Assessment      string   `json:"assessment"`   // supported|plausible|insufficient|weakened|refuted
	Confidence      string   `json:"confidence"`   // low|medium|high
	Scope           string   `json:"scope"`
	SupportEvidence []string `json:"support_evidence"` // 证据 id 引用（非 nil []）
	CounterEvidence []string `json:"counter_evidence"` // 证据 id 引用（非 nil []）
	Gaps            []string `json:"gaps"`             // 非 nil []，JSON 永不出 null
}

// boardInvestigationConclusion is the bounded final conclusion (D4): no fixed
// mechanism layers, no historical analogy quota — depth lives in evidence
// quality, alternatives and boundary.
type boardInvestigationConclusion struct {
	Summary    string `json:"summary"`
	Confidence string `json:"confidence"` // low|medium|high
	Scope      string `json:"scope"`
	Boundary   string `json:"boundary"`
}

// boardInvestigationEvidence is the investigation-specific evidence item:
// explicit polarity (Supports/Counters carry hypothesis ids) plus the shared
// verifiable fields. Methods can never appear here (parser rejects the source).
type boardInvestigationEvidence struct {
	ID          string   `json:"id"`
	SourceType  string   `json:"source_type"` // news|web|page|lane
	Ref         string   `json:"ref,omitempty"`
	URL         string   `json:"url,omitempty"`
	Quote       string   `json:"quote,omitempty"`
	Institution string   `json:"institution,omitempty"`
	Date        string   `json:"date,omitempty"`
	Kind        string   `json:"kind,omitempty"`
	LaneNote    string   `json:"lane_note,omitempty"`
	Supports    []string `json:"supports"`
	Counters    []string `json:"counters"`
}

// boardInvestigationPayload is the sectors jsonb shape for
// result_kind=board_investigation. MethodRefs is set mechanically from the
// hypothesis stage (never adopted from the LLM); ParentBriefingID/Question
// are stamped by the orchestrator before persistence.
type boardInvestigationPayload struct {
	Scope            string                         `json:"scope"` // always "board"
	ResultKind       string                         `json:"result_kind"`
	ParentBriefingID uint                           `json:"parent_briefing_id"`
	Question         BoardInvestigationQuestion     `json:"question"`
	Hypotheses       []boardInvestigationHypothesis `json:"hypotheses"`
	Conclusion       boardInvestigationConclusion   `json:"conclusion"`
	EvidenceChain    []boardInvestigationEvidence   `json:"evidence_chain"`
	LaneRefs         []laneRef                      `json:"lane_refs"`
	MethodRefs       []AnalysisMethodRef            `json:"method_refs"`
	RetryReason      string                         `json:"retry_reason,omitempty"` // 稳定原因码
}

// boardSynthesisGenerationMeta records how the synthesis was produced.
// RetryReason is a stable machine code only (no internal error text).
type boardSynthesisGenerationMeta struct {
	Attempts     int    `json:"attempts"` // LLM calls made (1 or 2)
	RetryReason  string `json:"retry_reason,omitempty"`
	RepairReason string `json:"repair_reason,omitempty"`
}

// boardInvestigationResearchSnapshot is the replayable digest of the shared
// research loop persisted in input_snapshot. The complete ordered tool call
// record (purpose/outcome/full results) lives in result.ToolCalls.
type boardInvestigationResearchSnapshot struct {
	Coverage         boardResearchCoverage `json:"coverage"`
	Gaps             []boardResearchGap    `json:"gaps"`
	FinishRejections []string              `json:"finish_rejections,omitempty"`
	FinalData        string                `json:"final_data"`
	Loops            int                   `json:"loops"`
}

// boardInvestigationInputSnapshot is the result's input_snapshot payload:
// enough to replay the whole investigation without the method table — parent
// raw sectors + projection, question + key, method candidates/selection/
// clean-inject trace + actually injected content, initial hypotheses (with
// safe retry codes), research digest, synthesis meta.
// boardInvestigationFreshnessPhase 标记补全门在调查链中的执行阶段（供回放
// 定位）：固定 pre_hypothesize——先于方法选择/假设/研究/综合任何 LLM。
const boardInvestigationFreshnessPhase = "pre_hypothesize"

// boardInvestigationFreshnessSnapshot 固化调查链 D10 补全门结果进
// input_snapshot（供回放）：Phase 标明执行阶段；Lanes 为从父简报不可变快照
// 推导的参与泳道数（0 = 无 lane 安全跳过）；Report 沿 FreshnessGateReport
// 既有结构（checked / refreshed=成功补全次数 / failed / duration_ms /
// details——budget_exhausted、refresh_failed 逐条见 details）。gate 自身
// 不在调查 session 产生 LLM 调用；失败与限额溢出均 nonfatal，调查用旧档
// 继续。gate 不重跑 brief/cards、不改写父简报，无需在 sectors 重复。
type boardInvestigationFreshnessSnapshot struct {
	Phase  string               `json:"phase"`
	Lanes  int                  `json:"lanes"`
	Report *FreshnessGateReport `json:"report"`
}

type boardInvestigationInputSnapshot struct {
	ParentBriefID    uint                       `json:"parent_brief_id"`
	ParentSectors    json.RawMessage            `json:"parent_sectors"`
	ParentProjection string                     `json:"parent_projection"`
	Question         BoardInvestigationQuestion `json:"question"`
	QuestionKey      string                     `json:"question_key"`
	LaneWhitelist    []uint                     `json:"lane_whitelist"`
	// DynamicGrants freezes the runtime grant audit (cross-board lanes
	// authorized by trusted tool results this run) for replay.
	DynamicGrants     []LaneGrant                          `json:"dynamic_grants"`
	Freshness         *boardInvestigationFreshnessSnapshot `json:"freshness"`
	Methods           boardMethodSelection                 `json:"methods"`
	MethodPrompt      string                               `json:"method_prompt"`
	MethodCards       []AnalysisMethodCardTrace            `json:"method_cards"`
	MethodRefs        []AnalysisMethodRef                  `json:"method_refs"`
	EvidenceNeeds     []string                             `json:"evidence_needs"`
	InitialHypotheses *boardHypothesisGeneration           `json:"initial_hypotheses"`
	Research          boardInvestigationResearchSnapshot   `json:"research"`
	Synthesis         *boardSynthesisGenerationMeta        `json:"synthesis"`
}

// ── Prompt ──────────────────────────────────────────────────────────────────

const boardSynthesizePromptLead = `你是一位证据仲裁员，负责一次板块深度调查的最终综合。你会收到：调查问题、父简报投影、初始竞争假设（尚未评估）、共享研究循环的完整材料（完成汇总、全部工具调用与结果、研究覆盖与缺口）、以及本次调查实际注入的分析方法正文与注入留痕。请基于研究结果产出可核查的最终判断。

硬性纪律：
1. 为每个假设标记 assessment：supported（证据充分支持）| plausible（初步支持但不足以定论）| insufficient（支持与反证都不足）| weakened（被反证明显削弱）| refuted（被反证直接推翻）。允许零假设成为最可信解释，也允许所有非零假设都是 insufficient/refuted——不强选赢家，不为对称而对称
2. 可以修改、合并、拆分或新增假设：新假设用 derived_from 标注来源初始假设 id；未改动的假设沿用原 id。评估只依据研究结果，不因初始排序保留赢家地位
3. 每个假设分别列出 support_evidence / counter_evidence（引用证据 id）与 gaps（诚实缺口）；支持与反证分开，不得把反证埋进支持叙述
4. conclusion 必须包含：summary（直白语言，先说具体变化）、confidence（low|medium|high）、scope（适用范围）、boundary（目前不能下结论的边界与下一步需要的材料）；证据不足就直说
5. evidence_chain 每条必须带 id 与 source_type（news=内部新闻记忆 / web=外部网页检索 / page=抓取的原文页 / lane=泳道内部事实），并用 supports/counters 标明它支持/反对哪些假设 id；web/page 必须带 url、quote（从研究工具结果原文逐字摘录，不得转述改写）、institution、date；lane 的 ref 只能取白名单内泳道编号；方法卡不是证据来源，不得作为 source_type
6. 只有同向新闻转述、没有一手可核查依据或未完成反证核查的假设，不得评为高置信 supported
7. 不要求固定层数的机制拆解、宏大叙事、反转句式或历史类比；深度由证据质量、替代解释与边界处理体现，用直白语言

输出严格 JSON（不要 markdown 包裹、不要任何其他文字）：
{"hypotheses":[{"id":"h0","label":"...","is_null":true,"derived_from":[],"assessment":"plausible","confidence":"medium","scope":"...","support_evidence":["e1"],"counter_evidence":[],"gaps":["..."]}],
 "conclusion":{"summary":"...","confidence":"low|medium|high","scope":"...","boundary":"..."},
 "evidence_chain":[{"id":"e1","source_type":"web","url":"...","quote":"...","institution":"...","date":"YYYY-MM-DD","supports":["h1"],"counters":[]}],
 "lane_refs":[{"lane_id":1,"note":"该泳道在本次调查中的角色"}]}`

// boardSynthesizeRetryLead is the stable prefix of the corrective retry note
// (test anchor; the full note embeds the concrete failure reason for the
// agent — that text goes to ai_call_logs via the logged prompt, never into
// the snapshot).
const boardSynthesizeRetryLead = "上次输出不是合格的调查综合"

const boardSynthesizeRetryNote = "\n\n---\n" + boardSynthesizeRetryLead + "（问题：%s）。请重新输出完整 JSON：严格遵循上述 schema、每个假设带合法 assessment 枚举、conclusion 四字段齐全、evidence_chain 带显式 supports/counters；web/page 的 quote 必须逐字摘自工具结果原文。"

// assembleBoardSynthesizePrompt builds the synthesis prompt (pure function,
// contract unit-tested). Method content enters as the CLEANED injection plus
// drop machine codes — never the filtered original rhetoric.
func assembleBoardSynthesizePrompt(
	question BoardInvestigationQuestion, brief *BoardBriefPayload,
	stage *boardHypothesisStageResult, research *BoardInvestigationResearchResult,
	laneWhitelist []uint,
) string {
	var sb strings.Builder
	sb.WriteString(boardSynthesizePromptLead)
	sb.WriteString("\n\n---\n调查问题：" + question.Text)
	sourceDesc := "用户自填问题"
	if question.Source == QuestionSourceGenerated {
		sourceDesc = "来自父简报的研究候选问题"
	}
	if question.ID != "" {
		sourceDesc += "（简报候选 id " + question.ID + "）"
	}
	sb.WriteString("\n来源：" + question.Source + "（" + sourceDesc + "）")
	sb.WriteString("\n\n---\n父简报投影：\n" + renderBriefProjectionForInvestigation(brief))

	sb.WriteString("\n\n---\n初始竞争假设（评估前集合；id 供 derived_from/证据引用）：")
	for _, h := range stage.Hypotheses.Hypotheses {
		tag := "竞争假设"
		if h.IsNull {
			tag = "零假设"
		}
		fmt.Fprintf(&sb, "\n- %s [%s] %s", h.ID, tag, h.Label)
	}

	sb.WriteString(renderMethodTraceForSynthesis(stage))
	sb.WriteString(renderResearchForSynthesis(research))
	sb.WriteString("\n\n---\n泳道白名单（lane 证据与引用只允许这些编号）：" + renderResearchLaneWhitelist(laneWhitelist))
	return sb.String()
}

// renderMethodTraceForSynthesis renders the actually-injected method content
// plus the selection/drop machine-code trail. Drop reasons are machine codes
// only; filtered original lines never appear (M7.7).
func renderMethodTraceForSynthesis(stage *boardHypothesisStageResult) string {
	if stage == nil {
		return ""
	}
	var sb strings.Builder
	if p := strings.TrimSpace(stage.MethodPrompt); p != "" {
		sb.WriteString("\n\n---\n分析方法参考（本次调查实际注入的清洗后正文，仅约束评估过程，不是事实来源）：\n" + p)
	}
	if len(stage.MethodRefs) > 0 || len(stage.Methods.Dropped) > 0 {
		sb.WriteString("\n\n方法注入留痕（机码，供审计）：")
		for _, r := range stage.MethodRefs {
			fmt.Fprintf(&sb, "\n- 方法#%d《%s》已注入（content_hash=%s）", r.ID, r.Title, r.ContentHash)
		}
		for _, d := range stage.Methods.Dropped {
			fmt.Fprintf(&sb, "\n- 方法#%d《%s》未注入（%s）", d.ID, d.Title, d.Reason)
		}
	}
	return sb.String()
}

// renderResearchForSynthesis renders the research digest: coverage, gaps,
// finish summary and the complete ordered tool call record (args + purpose +
// outcome + per-call budgeted result text so quotes can be copied verbatim).
func renderResearchForSynthesis(research *BoardInvestigationResearchResult) string {
	if research == nil || research.Loop == nil {
		return "\n\n---\n研究结果：（研究循环未产出任何结果）"
	}
	var sb strings.Builder
	sb.WriteString("\n\n---\n研究覆盖（机械统计，机码）：")
	fmt.Fprintf(&sb, "\n- neutral_attempted=%v", research.Coverage.NeutralAttempted)
	fmt.Fprintf(&sb, "\n- counter_attempted_by_hypothesis=%s", strings.Join(research.Coverage.CounterAttemptedByHypothesis, ","))
	if len(research.Gaps) > 0 {
		sb.WriteString("\n\n研究缺口（稳定原因码）：")
		for _, g := range research.Gaps {
			line := fmt.Sprintf("- reason=%s", g.Reason)
			if g.Tool != "" {
				line += " tool=" + g.Tool
			}
			if len(g.HypothesisIDs) > 0 {
				line += " 假设=" + strings.Join(g.HypothesisIDs, "、")
			}
			sb.WriteString("\n" + line)
		}
	}
	fd := strings.TrimSpace(research.Loop.FinalData)
	if fd == "" {
		fd = "（研究循环未产出完成总结——依据下方工具结果自行评估，并在 gaps 里如实记录）"
	}
	sb.WriteString("\n\n---\n研究完成汇总：\n" + fd)

	sb.WriteString("\n\n---\n工具调用完整记录（按步序；结果原文供逐字摘录 quote）：")
	for _, tc := range research.Loop.ToolCalls {
		targets := "（无）"
		if len(tc.HypothesisIDs) > 0 {
			targets = strings.Join(tc.HypothesisIDs, "、")
		}
		outcome := tc.Outcome
		if outcome == "" {
			outcome = "ok"
		}
		fmt.Fprintf(&sb, "\n- 步骤%d [%s] 目标%s %s(%s) → %s", tc.Step, tc.Purpose, targets, tc.Tool, argsToJSON(tc.Args), outcome)
		if tc.BlockedReason != "" {
			fmt.Fprintf(&sb, "（拦截：%s）", tc.BlockedReason)
		}
		result := tc.ResultFull
		if result == "" {
			result = tc.ResultPreview
		}
		if r := len([]rune(result)); r > boardSynthesisToolResultRunes {
			result = truncateRunes(result, boardSynthesisToolResultRunes) + "\n[结果超长已截断——quote 请只摘录以上可见原文]"
		}
		sb.WriteString("\n  结果: " + result)
	}
	return sb.String()
}

// ── Parser + sanitizer ──────────────────────────────────────────────────────

// assignFinalHypothesisIDs guarantees unique final hypothesis ids (trimmed
// explicit id wins, missing ids auto-assign fh1..fhN, collisions suffixed).
func assignFinalHypothesisIDs(hs []boardInvestigationHypothesis) {
	seen := make(map[string]bool, len(hs))
	for i := range hs {
		base := strings.TrimSpace(hs[i].ID)
		if base == "" {
			base = fmt.Sprintf("h%d", i+1)
		}
		id := base
		for n := 2; seen[id]; n++ {
			id = fmt.Sprintf("%s_%d", base, n)
		}
		seen[id] = true
		hs[i].ID = id
	}
}

// scrubGapList trims/caps gap entries.
func scrubGapList(raw any) []string {
	list, ok := raw.([]any)
	if !ok {
		return nil
	}
	out := []string{}
	for _, v := range list {
		s, ok := v.(string)
		if !ok {
			continue
		}
		s = truncateRunes(strings.TrimSpace(s), boardSynthesisMaxGapRunes)
		if s == "" {
			continue
		}
		out = append(out, s)
		if len(out) >= boardSynthesisMaxGapItems {
			break
		}
	}
	return out
}

// scrubEvidenceRefList trims/dedupes/caps id lists (final scrub against
// surviving evidence ids happens after evidence processing).
func scrubEvidenceRefList(raw any) []string {
	return scrubIDList(raw)
}

// ── web/page 证据同源核对（review 4.5 Important）─────────────────────────

// normalizeToolURL applies only SAFE equivalences to a URL string: surrounding
// whitespace, scheme/host case, scheme-default ports (http:80 / https:443),
// trailing slashes on the path, and the fragment (never sent to a server,
// same resource). Query strings compare EXACTLY — different query pages must
// never merge. Returns "" when the value is empty or does not parse as an
// absolute URL（不可解析的 URL 永不绑定证据，保守拒绝）。
func normalizeToolURL(raw string) string {
	s := strings.TrimSpace(raw)
	if s == "" {
		return ""
	}
	u, err := url.Parse(s)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return ""
	}
	scheme := strings.ToLower(u.Scheme)
	host := strings.ToLower(u.Hostname())
	if port := u.Port(); port != "" && !isSchemeDefaultPort(scheme, port) {
		host += ":" + port
	}
	path := strings.TrimRight(u.EscapedPath(), "/")
	q := strings.TrimSpace(u.RawQuery)
	if q != "" {
		q = "?" + q
	}
	return scheme + "://" + host + path + q
}

// isSchemeDefaultPort reports whether port is the scheme's default
// (http:80 / https:443) — defaults are dropped for safe equivalence.
func isSchemeDefaultPort(scheme, port string) bool {
	return (scheme == "http" && port == "80") || (scheme == "https" && port == "443")
}

// toolCallErrored reports whether a recorded tool call failed（结构化 Outcome
// 标记或 registry 错误 JSON 约定，两者任一命中即失败；blocked 调用的
// ResultFull 也是错误 JSON，天然被拒）。
func toolCallErrored(tc ToolCallRecord) bool {
	return tc.Outcome == toolCallOutcomeError || toolResultErrorText(tc.ResultFull) != ""
}

// stringLeafValues collects every decoded string leaf of a JSON value.
func stringLeafValues(node any, out *[]string) {
	switch v := node.(type) {
	case string:
		*out = append(*out, v)
	case map[string]any:
		for _, child := range v {
			stringLeafValues(child, out)
		}
	case []any:
		for _, child := range v {
			stringLeafValues(child, out)
		}
	}
}

// webObjectURLMatches reports whether the object carries a url-ish scalar
// field (url/link/href) equal to targetURL under safe normalization.
func webObjectURLMatches(obj map[string]any, targetURL string) bool {
	for _, key := range []string{"url", "link", "href"} {
		if s, ok := obj[key].(string); ok && normalizeToolURL(s) == targetURL {
			return true
		}
	}
	return false
}

// findBoundWebResult walks the decoded ResultFull tree; at every object whose
// url-ish field matches, it checks the quote against THAT object's own string
// subtree（URL 与 quote 必须同在一个结果对象，不允许对象 A 出 URL、对象 B 出 quote）。
func findBoundWebResult(node any, targetURL, quote string) bool {
	switch v := node.(type) {
	case map[string]any:
		if webObjectURLMatches(v, targetURL) {
			var leaves []string
			stringLeafValues(v, &leaves)
			for _, s := range leaves {
				if strings.Contains(s, quote) {
					return true
				}
			}
		}
		for _, child := range v {
			if findBoundWebResult(child, targetURL, quote) {
				return true
			}
		}
	case []any:
		for _, child := range v {
			if findBoundWebResult(child, targetURL, quote) {
				return true
			}
		}
	}
	return false
}

// webResultObjectBindsQuote verifies a web evidence quote against ONE
// web_search ResultFull: the real executeWebSearch schema is
// {"query","hit_count","results":[{title,url,snippet}]}，递归遍历覆盖该
// schema 并对未来后端（Tavily 等）保持稳健——只要 URL 与 quote 同在一个
// 结果对象内即可，不依赖字段名猜测以外的脆弱约定。
func webResultObjectBindsQuote(resultFull, evidenceURL, quote string) bool {
	if quote == "" {
		return false
	}
	target := normalizeToolURL(evidenceURL)
	if target == "" {
		return false
	}
	var root any
	if err := json.Unmarshal([]byte(resultFull), &root); err != nil {
		return false
	}
	return findBoundWebResult(root, target, quote)
}

// pageResultCarriesQuote checks the quote against a fetch_page ResultFull:
// the raw JSON text OR the decoded string leaves（JSON 转义不得吞掉含引号/
// 换行的逐字摘录）。
func pageResultCarriesQuote(resultFull, quote string) bool {
	if quote == "" {
		return false
	}
	if strings.Contains(resultFull, quote) {
		return true
	}
	var root any
	if err := json.Unmarshal([]byte(resultFull), &root); err != nil {
		return false
	}
	var leaves []string
	stringLeafValues(root, &leaves)
	for _, s := range leaves {
		if strings.Contains(s, quote) {
			return true
		}
	}
	return false
}

// investigationEvidenceVerified binds a web/page evidence item's url AND quote
// to the SAME executed tool call（review 4.5 Important）：
//   - page：只接受 Tool=fetch_page 且 args.url 与 evidence.url 规范化相等的
//     调用，quote 必须是该调用 ResultFull 子串；调用 error 拒绝。
//   - web：只接受 Tool=web_search 的同一条 ResultFull，且 url 与 quote 必须
//     出现在同一个结果对象里；调用 error 拒绝。
//
// 全部 ResultFull 并集碰巧命中（URL 在调用 A、quote 在调用 B）不算通过。
func investigationEvidenceVerified(e boardInvestigationEvidence, toolCalls []ToolCallRecord) bool {
	switch e.SourceType {
	case "page":
		target := normalizeToolURL(e.URL)
		if target == "" {
			return false
		}
		for _, tc := range toolCalls {
			if tc.Tool != "fetch_page" || toolCallErrored(tc) {
				continue
			}
			argURL, _ := tc.Args["url"].(string)
			if normalizeToolURL(argURL) != target {
				continue
			}
			if pageResultCarriesQuote(tc.ResultFull, e.Quote) {
				return true
			}
		}
		return false
	case "web":
		for _, tc := range toolCalls {
			if tc.Tool != "web_search" || toolCallErrored(tc) {
				continue
			}
			if webResultObjectBindsQuote(tc.ResultFull, e.URL, e.Quote) {
				return true
			}
		}
		return false
	default:
		return false
	}
}

// parseBoardInvestigationSynthesis validates one parsed synthesis payload.
//
// Structural failures (warrant a corrective retry, then error): missing
// hypotheses array / zero valid hypotheses / illegal assessment enum /
// initial hypothesis vanished without trace（同 id 或 derived_from 均无）/
// conclusion missing or incomplete (summary+scope+boundary) / definitive
// assessment contradiction（M13：supported 无存活 support、refuted/weakened
// 无存活 counter——在 lane_id 别名归一与极性并集回填之后判定）。
// Item-level violations never fail the payload on their own: evidence
// scrubbing (illegal source_type incl. method, web/page missing
// url/quote/institution/date, quote not bound to the same tool-call source
// as url, ghost lane, dangling polarity), confidence normalization,
// derived_from / evidence-ref scrubbing, ghost lane_refs——单项非法只丢该
// 条；唯有清洗后终局 assessment 与存活证据矛盾（definitive 定论失去对应
// 存活证据）升级为结构失败。
//
// initialHypoIDs is the initial hypothesis id set (derived_from whitelist +
// coverage check); laneWhitelist comes from the parent brief snapshot;
// toolCalls is the complete ordered research tool call record for quote + url
// same-source verification.
func parseBoardInvestigationSynthesis(
	parsed map[string]any,
	initialHypoIDs map[string]bool,
	laneWhitelist []uint,
	boardID uint,
	dynamicGrants *DynamicLaneGrantSet,
	toolCalls []ToolCallRecord,
) (*boardInvestigationPayload, error) {
	// 1. Final hypotheses（assessment 非法 = 结构失败 → 重试）。
	rawHyps, ok := parsed["hypotheses"].([]any)
	if !ok {
		return nil, errors.New("synthesis: hypotheses missing or not an array")
	}
	hyps := []boardInvestigationHypothesis{}
	for _, v := range rawHyps {
		m, ok := v.(map[string]any)
		if !ok {
			continue
		}
		label := truncateRunes(strings.TrimSpace(getString(m, "label")), boardSynthesisMaxLabelRunes)
		if label == "" {
			logging.Warnf("board synthesize: hypothesis without label — dropped")
			continue
		}
		assessment := strings.TrimSpace(getString(m, "assessment"))
		if !boardHypothesisAssessments[assessment] {
			return nil, fmt.Errorf("synthesis: hypothesis %q has illegal assessment %q (want supported|plausible|insufficient|weakened|refuted)", label, assessment)
		}
		confidence := strings.TrimSpace(getString(m, "confidence"))
		if confidence != "low" && confidence != "medium" && confidence != "high" {
			confidence = "low"
		}
		derived := []string{}
		for _, id := range scrubIDList(m["derived_from"]) {
			if initialHypoIDs[id] && len(derived) < boardSynthesisMaxDerivedFrom {
				derived = append(derived, id)
			}
		}
		hyps = append(hyps, boardInvestigationHypothesis{
			ID:              strings.TrimSpace(getString(m, "id")),
			Label:           label,
			IsNull:          getBool(m, "is_null"),
			DerivedFrom:     derived,
			Assessment:      assessment,
			Confidence:      confidence,
			Scope:           truncateRunes(strings.TrimSpace(getString(m, "scope")), boardSynthesisMaxScopeRunes),
			SupportEvidence: nonNilStrings(scrubEvidenceRefList(m["support_evidence"])),
			CounterEvidence: nonNilStrings(scrubEvidenceRefList(m["counter_evidence"])),
			Gaps:            nonNilStrings(scrubGapList(m["gaps"])),
		})
		if len(hyps) >= boardSynthesisMaxHypotheses {
			break
		}
	}
	if len(hyps) == 0 {
		return nil, errors.New("synthesis: no structurally valid hypotheses")
	}
	assignFinalHypothesisIDs(hyps)
	validHypo := make(map[string]bool, len(hyps))
	for _, h := range hyps {
		validHypo[h.ID] = true
	}

	// 1b. 初始假设覆盖（review 4.5）：每个初始假设 id（含 H0）必须被同 id
	// 保留，或出现在某 final hypothesis 的 derived_from 里——允许明确的
	// merge/split，但任何初始假设（含零假设）不得无痕消失。违规 = 结构
	// 失败 → 纠错重试；两次仍失败 = error（0 行），绝不静默丢假设。
	if len(initialHypoIDs) > 0 {
		covered := make(map[string]bool, len(hyps))
		for _, h := range hyps {
			if initialHypoIDs[h.ID] {
				covered[h.ID] = true
			}
			for _, d := range h.DerivedFrom {
				if initialHypoIDs[d] {
					covered[d] = true
				}
			}
		}
		var vanished []string
		for id := range initialHypoIDs {
			if !covered[id] {
				vanished = append(vanished, id)
			}
		}
		if len(vanished) > 0 {
			sort.Strings(vanished)
			return nil, fmt.Errorf("synthesis: initial hypotheses vanished without trace: %s — every initial id must survive unchanged or be merged/split via derived_from", strings.Join(vanished, ", "))
		}
	}

	// 2. Conclusion（四字段必含，缺失 = 结构失败）。
	concRaw, ok := parsed["conclusion"].(map[string]any)
	if !ok {
		return nil, errors.New("synthesis: conclusion missing")
	}
	summary := truncateRunes(strings.TrimSpace(getString(concRaw, "summary")), boardSynthesisMaxSummaryRunes)
	scope := truncateRunes(strings.TrimSpace(getString(concRaw, "scope")), boardSynthesisMaxScopeRunes)
	boundary := truncateRunes(strings.TrimSpace(getString(concRaw, "boundary")), boardSynthesisMaxBoundaryRunes)
	if summary == "" || scope == "" || boundary == "" {
		return nil, errors.New("synthesis: conclusion must carry non-empty summary/scope/boundary")
	}
	confidence := strings.TrimSpace(getString(concRaw, "confidence"))
	if confidence != "low" && confidence != "medium" && confidence != "high" {
		confidence = "low"
	}
	conclusion := boardInvestigationConclusion{Summary: summary, Confidence: confidence, Scope: scope, Boundary: boundary}

	// 3. Evidence chain（逐项清洗，绝不拒整份）。
	laneAllowed := make(map[uint]bool, len(laneWhitelist))
	for _, id := range laneWhitelist {
		laneAllowed[id] = true
	}
	// Cross-board lanes authorized this session (trusted-tool grants only)
	// join the sanitize set alongside the parent-brief whitelist (spec:
	// 未授权跨版块引用被剔除 — grants are exactly what makes a reference
	// authorized).
	for _, id := range dynamicGrants.GrantedIDs() {
		laneAllowed[id] = true
	}
	evs := []boardInvestigationEvidence{}
	if rawEvs, ok := parsed["evidence_chain"].([]any); ok {
		for _, v := range rawEvs {
			m, ok := v.(map[string]any)
			if !ok {
				continue
			}
			e := boardInvestigationEvidence{
				ID:          strings.TrimSpace(getString(m, "id")),
				SourceType:  strings.TrimSpace(getString(m, "source_type")),
				Ref:         strings.TrimSpace(getString(m, "ref")),
				URL:         strings.TrimSpace(getString(m, "url")),
				Quote:       truncateRunes(strings.TrimSpace(getString(m, "quote")), boardSynthesisMaxQuoteRunes),
				Institution: strings.TrimSpace(getString(m, "institution")),
				Date:        strings.TrimSpace(getString(m, "date")),
				Kind:        normalizeEvidenceKind(strings.TrimSpace(getString(m, "kind"))),
				LaneNote:    truncateRunes(strings.TrimSpace(getString(m, "lane_note")), boardBriefMaxNoteRunes),
				Supports:    scrubIDList(m["supports"]),
				Counters:    scrubIDList(m["counters"]),
			}
			switch e.SourceType {
			case "news":
				// 内部新闻记忆引用：ref 保留（无法机械核对，不伪造）。
			case "web", "page":
				if e.URL == "" || e.Quote == "" || e.Institution == "" || e.Date == "" {
					logging.Warnf("board synthesize: %s evidence %q missing url/quote/institution/date — dropped", e.SourceType, e.ID)
					continue
				}
				if !investigationEvidenceVerified(e, toolCalls) {
					logging.Warnf("board synthesize: %s evidence %q quote/url not bound to the same tool call source — dropped", e.SourceType, e.ID)
					continue
				}
			case "lane":
				// M13 lane_id 别名归一：qwen 实测会漏发 ref、把泳道编号放进
				// lane_id 数值字段（result12）——仅当原始 ref trim 后为空才读
				// 别名（float64 整数 (0,2^53) 归一十进制）；ref 已非空永不看
				// 别名，显式幽灵 ref 不得被合法别名掩盖；归一后仍过白名单。
				if e.Ref == "" {
					if alias, ok := laneIDAliasRef(m["lane_id"]); ok {
						e.Ref = alias
					}
				}
				var laneID uint64
				if !parseInt64Ref(e.Ref, &laneID) || !laneAllowed[uint(laneID)] {
					logging.Warnf("board synthesize: lane evidence ref %q not in parent brief whitelist — dropped (ghost reference)", e.Ref)
					continue
				}
			default:
				// 含 method：方法卡绝不可作证据来源（spec「方法卡不进入证据链」）。
				logging.Warnf("board synthesize: illegal source_type %q — entry dropped", e.SourceType)
				continue
			}
			// 极性清洗：引用必须指向最终假设；无任何有效极性 → 悬空剔除。
			e.Supports = filterIDs(e.Supports, validHypo)
			e.Counters = filterIDs(e.Counters, validHypo)
			if len(e.Supports) == 0 && len(e.Counters) == 0 {
				logging.Warnf("board synthesize: evidence %q carries no valid supports/counters — dropped (dangling polarity)", e.ID)
				continue
			}
			evs = append(evs, e)
			if len(evs) >= boardSynthesisMaxEvidence {
				break
			}
		}
	}
	// 证据 id 唯一化（缺省 e1..eN，LLM 重复 id 加后缀）。
	seenEv := make(map[string]bool, len(evs))
	for i := range evs {
		base := evs[i].ID
		if base == "" {
			base = fmt.Sprintf("e%d", i+1)
		}
		id := base
		for n := 2; seenEv[id]; n++ {
			id = fmt.Sprintf("%s_%d", base, n)
		}
		seenEv[id] = true
		evs[i].ID = id
	}

	// 4. 假设证据引用清洗（剔除的证据不得残留引用）。
	evIDs := seenEv
	for i := range hyps {
		hyps[i].SupportEvidence = capIDs(filterIDs(hyps[i].SupportEvidence, evIDs), boardSynthesisMaxEvidenceRefs)
		hyps[i].CounterEvidence = capIDs(filterIDs(hyps[i].CounterEvidence, evIDs), boardSynthesisMaxEvidenceRefs)
	}

	// 4b. 极性并集回填（M13）：evidence 的 supports/counters 是假设引用的
	// 权威来源——LLM 假设侧漏列的引用由存活证据极性按 first-seen 并集
	// 补齐（绝不改写证据极性），仍受 max refs cap 约束。
	mergeEvidencePolarityIntoHypotheses(hyps, evs)

	// 4c. 终局一致性门（M13 definitive invariant）：supported 至少一条存活
	// support、refuted/weakened 至少一条存活 counter——清洗（含别名归一与
	// 极性回填）之后仍缺 = 结构失败（纠错重试，两次仍败 0 行），杜绝
	// 「证据被清洗掉、定论却落库」的矛盾快照；plausible/insufficient
	// 不强制（无直接证据仍允许）。
	if err := checkDefinitiveAssessmentConsistency(hyps); err != nil {
		return nil, err
	}

	// 5. lane_refs 幽灵清洗。
	laneRefs := []laneRef{}
	if rawRefs, ok := parsed["lane_refs"].([]any); ok {
		seenLane := map[uint]bool{}
		for _, v := range rawRefs {
			m, ok := v.(map[string]any)
			if !ok {
				continue
			}
			idf, ok := m["lane_id"].(float64)
			// float64 → uint 转换护栏（review 4.5）：JSON 大数（如 2^64+1）经
			// float64 解码后 uint() 转换溢出行为未定义——非整数、负数、超出
			// float64 精确整数域（2^53）的一律拒绝，不得回绕命中合法 lane。
			if !ok || idf < 0 || idf != math.Trunc(idf) || idf >= 1<<53 ||
				!laneAllowed[uint(idf)] || seenLane[uint(idf)] {
				continue
			}
			seenLane[uint(idf)] = true
			ref := laneRef{LaneID: uint(idf), Note: truncateRunes(strings.TrimSpace(getString(m, "note")), boardBriefMaxNoteRunes)}
			if g, granted := dynamicGrants.lookup(uint(idf)); granted {
				// Cross-board reference carries its owning board id (spec:
				// 调查引用其他版块泳道); board-local lanes keep board_id=0.
				if g.BoardID != 0 && g.BoardID != boardID {
					ref.BoardID = g.BoardID
				}
			}
			laneRefs = append(laneRefs, ref)
		}
	}

	return &boardInvestigationPayload{
		Scope:         "board",
		ResultKind:    repository.ResultKindBoardInvestigation,
		Hypotheses:    hyps,
		Conclusion:    conclusion,
		EvidenceChain: evs,
		LaneRefs:      laneRefs,
		MethodRefs:    []AnalysisMethodRef{},
	}, nil
}

// filterIDs keeps only ids present in valid, first-seen order（始终非 nil）。
func filterIDs(ids []string, valid map[string]bool) []string {
	out := []string{}
	for _, id := range ids {
		if valid[id] {
			out = append(out, id)
		}
	}
	return out
}

// nonNilStrings normalizes a scrubbed list to a non-nil slice so the persisted
// JSON always carries [] and never null（review 4.5：gaps/support_evidence/
// counter_evidence/derived_from 等列表统一非 nil []）。
func nonNilStrings(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

// capIDs truncates an already-scrubbed id list.
func capIDs(ids []string, max int) []string {
	if len(ids) <= max {
		return ids
	}
	return ids[:max]
}

// parseInt64Ref parses a decimal ref string into out via strconv.ParseUint:
// overflow（如 2^64+1）与超长/非纯十进制一律拒绝——旧手写数位循环在溢出
// 时静默回绕，能把 2^64+1 回绕成 1 命中合法 lane（review 4.5）。
func parseInt64Ref(ref string, out *uint64) bool {
	if ref == "" {
		return false
	}
	n, err := strconv.ParseUint(ref, 10, 64)
	if err != nil {
		return false
	}
	*out = n
	return true
}

// laneIDAliasRef normalizes a numeric lane_id alias into a decimal ref
// string（M13）：仅 float64 编码的 (0, 2^53) 内整数合格——0/负数/小数/
// 超出 float64 精确整数域/字符串或 null 一律拒绝，不得截断或回绕命中
// 合法 lane（1.5 不得 floor 成 1，2^53 及以上精度不可信）。
func laneIDAliasRef(raw any) (string, bool) {
	idf, ok := raw.(float64)
	if !ok || idf <= 0 || idf != math.Trunc(idf) || idf >= 1<<53 {
		return "", false
	}
	return strconv.FormatUint(uint64(idf), 10), true
}

// mergeEvidencePolarityIntoHypotheses unions each surviving evidence's
// supports/counters into the referenced hypotheses' ref lists（M13 极性
// 并集回填）：first-seen 去重、不改写证据极性、仍受 max refs cap 约束——
// LLM 假设侧漏列的引用由证据极性补齐，清洗后的假设引用是双方的并集。
func mergeEvidencePolarityIntoHypotheses(hyps []boardInvestigationHypothesis, evs []boardInvestigationEvidence) {
	if len(hyps) == 0 || len(evs) == 0 {
		return
	}
	idx := make(map[string]int, len(hyps))
	for i := range hyps {
		idx[hyps[i].ID] = i
	}
	for _, e := range evs {
		for _, hid := range e.Supports {
			if i, ok := idx[hid]; ok {
				hyps[i].SupportEvidence = appendIDCapped(hyps[i].SupportEvidence, e.ID)
			}
		}
		for _, hid := range e.Counters {
			if i, ok := idx[hid]; ok {
				hyps[i].CounterEvidence = appendIDCapped(hyps[i].CounterEvidence, e.ID)
			}
		}
	}
}

// appendIDCapped appends id when first-seen, keeping the list within the
// max refs cap（M13）。
func appendIDCapped(list []string, id string) []string {
	for _, existing := range list {
		if existing == id {
			return list
		}
	}
	if len(list) >= boardSynthesisMaxEvidenceRefs {
		return list
	}
	return append(list, id)
}

// checkDefinitiveAssessmentConsistency enforces the M13 definitive
// invariant：supported 至少一条存活 support、refuted/weakened 至少一条
// 存活 counter（清洗 + 极性回填之后判定）；plausible/insufficient 不强制。
// 返回稳定 structure error（含假设 label，供纠错重试 prompt 定位），由
// synthesizeBoardInvestigation 既有重试机制处理。
func checkDefinitiveAssessmentConsistency(hyps []boardInvestigationHypothesis) error {
	for i := range hyps {
		h := &hyps[i]
		switch h.Assessment {
		case HypothesisSupported:
			if len(h.SupportEvidence) == 0 {
				return fmt.Errorf("synthesis: hypothesis %q assessed supported but no surviving supporting evidence — cite surviving evidence or downgrade to plausible/insufficient", h.Label)
			}
		case HypothesisRefuted, HypothesisWeakened:
			if len(h.CounterEvidence) == 0 {
				return fmt.Errorf("synthesis: hypothesis %q assessed %s but no surviving counter evidence — cite surviving evidence or downgrade to plausible/insufficient", h.Label, h.Assessment)
			}
		}
	}
	return nil
}

// ── 机械质量护栏（D7「只有同向材料」Scenario）────────────────────────────────

// applyInvestigationQualityGuard downgrades any supported+high hypothesis
// that lacks a verifiable non-news (web/page) supporting evidence or whose
// counter check was never attempted (the hypothesis itself or, for
// modified/split hypotheses, any of its derived_from initial ids). The
// downgrade is traced per hypothesis (gap) and in the conclusion boundary.
func applyInvestigationQualityGuard(p *boardInvestigationPayload, research *BoardInvestigationResearchResult) {
	if p == nil {
		return
	}
	counter := map[string]bool{}
	if research != nil {
		for _, id := range research.Coverage.CounterAttemptedByHypothesis {
			counter[id] = true
		}
	}
	downgraded := []string{}
	for i := range p.Hypotheses {
		h := &p.Hypotheses[i]
		if h.Assessment != HypothesisSupported || h.Confidence != "high" {
			continue
		}
		hasVerifiable := false
		for _, e := range p.EvidenceChain {
			if e.SourceType != "web" && e.SourceType != "page" {
				continue
			}
			for _, id := range e.Supports {
				if id == h.ID {
					hasVerifiable = true
					break
				}
			}
			if hasVerifiable {
				break
			}
		}
		counterOK := h.IsNull || counter[h.ID]
		if !counterOK {
			for _, d := range h.DerivedFrom {
				if counter[d] {
					counterOK = true
					break
				}
			}
		}
		if hasVerifiable && counterOK {
			continue
		}
		h.Confidence = "medium"
		h.Gaps = append(h.Gaps, boardSynthesisQualityGuardGap)
		if len(h.Gaps) > boardSynthesisMaxGapItems {
			h.Gaps = h.Gaps[len(h.Gaps)-boardSynthesisMaxGapItems:]
		}
		downgraded = append(downgraded, truncateRunes(h.Label, 30))
	}
	if len(downgraded) == 0 {
		return
	}
	note := "（" + boardSynthesisQualityGuardBoundary + "：" + strings.Join(downgraded, "；") + " 因仅有同向转述证据或未完成反证核查，高置信评级已降级）"
	if p.Conclusion.Boundary != "" {
		p.Conclusion.Boundary += "。" + note
	} else {
		p.Conclusion.Boundary = note
	}
	p.Conclusion.Boundary = truncateRunes(p.Conclusion.Boundary, boardSynthesisMaxBoundaryRunes)
}

// ── Synthesis LLM 调用（一次纠错重试）──────────────────────────────────────

// parseBoardSynthesisJSONResponse preserves ParseJSONResponse as the normal
// strict path and adds one deliberately narrow production hardening boundary.
// Some OpenAI-compatible fallbacks return a semantically complete synthesis
// ending in the lane_refs array's `]` but omit only the root object's final
// `}`. We repair that exact shape only when a lexical scan proves:
//   - strings/escapes are closed and every inner delimiter is balanced;
//   - the sole remaining delimiter is the first/root `{`;
//   - the final non-space byte is `]`;
//   - appending one `}` yields valid JSON with a top-level lane_refs array.
//
// Any mismatch, nested truncation, unterminated string, trailing prose or
// missing lane_refs returns the original strict parse error and follows the
// existing corrective-retry/no-row path. This is intentionally synthesis-only
// rather than a generic JSON or provider-client repair.
func parseBoardSynthesisJSONResponse(text string) (map[string]any, bool, error) {
	parsed, parseErr := ParseJSONResponse(text)
	if parseErr == nil {
		return parsed, false, nil
	}

	candidate := strings.TrimSpace(text)
	if strings.HasPrefix(candidate, "```") {
		if idx := strings.Index(candidate, "\n"); idx >= 0 {
			candidate = candidate[idx+1:]
		} else {
			candidate = candidate[3:]
		}
		candidate = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(candidate), "```"))
	}
	if start := strings.Index(candidate, "{"); start >= 0 {
		candidate = strings.TrimSpace(candidate[start:])
	} else {
		return nil, false, parseErr
	}
	if !strings.HasSuffix(candidate, "]") {
		return nil, false, parseErr
	}

	stack := make([]byte, 0, 8)
	inString := false
	escaped := false
	for i := 0; i < len(candidate); i++ {
		ch := candidate[i]
		if inString {
			if escaped {
				escaped = false
				continue
			}
			switch ch {
			case '\\':
				escaped = true
			case '"':
				inString = false
			}
			continue
		}
		switch ch {
		case '"':
			inString = true
		case '{', '[':
			stack = append(stack, ch)
		case '}', ']':
			want := byte('{')
			if ch == ']' {
				want = '['
			}
			if len(stack) == 0 || stack[len(stack)-1] != want {
				return nil, false, parseErr
			}
			stack = stack[:len(stack)-1]
		}
	}
	if inString || len(stack) != 1 || stack[0] != '{' {
		return nil, false, parseErr
	}

	var repaired map[string]any
	if err := json.Unmarshal([]byte(candidate+"}"), &repaired); err != nil {
		return nil, false, parseErr
	}
	if _, ok := repaired["lane_refs"].([]any); !ok {
		return nil, false, parseErr
	}
	return repaired, true, nil
}

// synthesizeBoardInvestigation runs the synthesis call with one corrective
// retry. Two unusable attempts return an error — the caller then leaves zero
// investigation rows; no mechanical fabrication of assessment conclusions.
func (o *OrchestratorService) synthesizeBoardInvestigation(
	ctx context.Context, sessionID string,
	question BoardInvestigationQuestion, brief *BoardBriefPayload,
	stage *boardHypothesisStageResult, research *BoardInvestigationResearchResult,
	laneWhitelist []uint,
	boardID uint, dynamicGrants *DynamicLaneGrantSet,
) (*boardInvestigationPayload, *boardSynthesisGenerationMeta, error) {
	initialHypoIDs := map[string]bool{}
	if stage != nil {
		for _, h := range stage.Hypotheses.Hypotheses {
			initialHypoIDs[h.ID] = true
		}
	}
	toolCalls := []ToolCallRecord{}
	if research != nil && research.Loop != nil {
		toolCalls = append(toolCalls, research.Loop.ToolCalls...)
	}
	meta := &boardSynthesisGenerationMeta{}
	prompt := assembleBoardSynthesizePrompt(question, brief, stage, research, laneWhitelist)
	var lastErr error
	for attempt := 1; attempt <= 2; attempt++ {
		meta.Attempts = attempt
		attemptPrompt := prompt
		if attempt > 1 && lastErr != nil {
			attemptPrompt = prompt + fmt.Sprintf(boardSynthesizeRetryNote, lastErr.Error())
		}
		resp, err := o.airouter.Chat(ctx, airouter.ChatRequest{
			Capability:  o.capability,
			Operation:   boardSynthesizeOperation,
			SessionID:   sessionID,
			Messages:    []airouter.Message{{Role: "user", Content: attemptPrompt}},
			Temperature: floatPtr(0.3),
			JSONMode:    true,
		})
		if err != nil {
			lastErr = fmt.Errorf("board synthesize chat: %w", err)
			if attempt == 1 {
				meta.RetryReason = synthesisRetryChat
			}
			continue
		}
		parsed, repaired, err := parseBoardSynthesisJSONResponse(resp.Content)
		if err != nil {
			lastErr = fmt.Errorf("board synthesize parse: %w", err)
			if attempt == 1 {
				meta.RetryReason = synthesisRetryParse
			}
			continue
		}
		payload, err := parseBoardInvestigationSynthesis(parsed, initialHypoIDs, laneWhitelist, boardID, dynamicGrants, toolCalls)
		if err != nil {
			lastErr = err
			if attempt == 1 {
				meta.RetryReason = synthesisRetryStructure
			}
			continue
		}
		if repaired {
			meta.RepairReason = synthesisRepairTerminalRootDelimiter
			logging.Warnf("board synthesize: repaired response (%s)", synthesisRepairTerminalRootDelimiter)
		}
		applyInvestigationQualityGuard(payload, research)
		payload.RetryReason = meta.RetryReason
		return payload, meta, nil
	}
	logging.Warnf("board synthesize: both attempts unusable (%v) — no investigation row", lastErr)
	return nil, meta, fmt.Errorf("board synthesize: %w", lastErr)
}

// ── 调查编排入口（tasks 4.6，供 5.x analysis runner 调用）────────────────────

// BoardInvestigationOutput is the persisted investigation handle returned to
// the handler layer. Review carries the 4.7 same-chain review row (nil on
// first run / judge=false / any non-fatal judge failure).
type BoardInvestigationOutput struct {
	Result *repository.TopicEnrichmentResult `json:"result"`
	Review *repository.TopicEnrichmentReview `json:"review,omitempty"`
}

// InvestigateBoardQuestion runs one board investigation against an existing
// board_brief snapshot. Synchronous service entry for the async runner (5.x).
//
// Ordering contract: every pre-flight failure (disabled board, repo not
// wired, missing/cross-board/legacy parent, unreadable parent sectors,
// invalid question) returns an error with ZERO LLM calls and ZERO gate work.
// The chain is then strictly D10 lifeline top-up (nonfatal; lane set from the
// parent brief's immutable snapshot, same derivation as the research
// whitelist) → hypothesize → shared research → synthesize → one insert;
// situation cards and the parent brief are never re-run or rewritten. The
// gate only refreshes month/year lifelines (later consumed by
// get_lane_detail); its report is frozen into the input snapshot. Synthesis
// failure (or any mid-chain error) leaves zero investigation rows.
func (o *OrchestratorService) InvestigateBoardQuestion(ctx context.Context, boardID uint, parentBriefID uint, question BoardInvestigationQuestion) (*BoardInvestigationOutput, error) {
	// ── 预检（0 LLM 失败路径）──
	if o.repo == nil {
		return nil, fmt.Errorf("board investigation %d: repository not wired", boardID)
	}
	if err := o.BoardEnrichmentEnabled(ctx, boardID); err != nil {
		return nil, err
	}
	parent, err := o.repo.GetTopicEnrichmentResultByID(ctx, parentBriefID)
	if err != nil {
		return nil, fmt.Errorf("board investigation %d: load parent brief %d: %w", boardID, parentBriefID, err)
	}
	if !repository.BoardIDMatches(parent.SemanticBoardID, boardID) {
		return nil, fmt.Errorf("board investigation %d: parent result %d belongs to another board", boardID, parentBriefID)
	}
	if kind := repository.EffectiveResultKind(parent); kind != repository.ResultKindBoardBrief {
		return nil, fmt.Errorf("board investigation %d: parent result %d is kind=%s, not a board_brief", boardID, parentBriefID, kind)
	}
	brief := &BoardBriefPayload{}
	if err := json.Unmarshal(parent.Sectors, brief); err != nil {
		return nil, fmt.Errorf("board investigation %d: parent brief %d sectors unreadable: %w", boardID, parentBriefID, err)
	}
	if brief.ResultKind != repository.ResultKindBoardBrief || strings.TrimSpace(brief.Summary) == "" {
		return nil, fmt.Errorf("board investigation %d: parent result %d sectors not a valid brief payload", boardID, parentBriefID)
	}
	if err := question.Normalize(); err != nil {
		return nil, fmt.Errorf("board investigation %d: %w", boardID, err)
	}
	questionKey := repository.ComputeQuestionKey(question.Text)
	sessionID := generateBoardSessionID(boardID)

	// ── 0.5 D10 补全门：方法选择/假设/研究任何 LLM 之前补全 month/year
	//      lifeline。lane 集从父简报不可变快照推导（与研究白名单同一
	//      helper，同源）；无 lane 安全跳过（gate 早退）；失败/限额溢出
	//      nonfatal 只留痕；不重跑 brief/cards、不改写父简报——后续
	//      get_lane_detail 直接消费补全后的档案。ctx 取消时 gate 内部降级
	//      留痕，后续 LLM 阶段照常以 ctx 错误失败（原子边界 0 行不变）。
	laneWhitelist := deriveLaneWhitelistFromBrief(brief)
	freshness := &boardInvestigationFreshnessSnapshot{
		Phase:  boardInvestigationFreshnessPhase,
		Lanes:  len(laneWhitelist),
		Report: o.ensureLaneFreshness(ctx, laneWhitelist),
	}
	// Session-scoped dynamic grants (design D3): start empty; only trusted
	// tool results during the shared research loop may add cross-board lanes.
	dynamicGrants := NewDynamicLaneGrantSet()

	// ── 1. 假设阶段（select → load → hypothesize）──
	stage, err := o.HypothesizeBoardInvestigation(ctx, sessionID, question, brief)
	if err != nil {
		return nil, fmt.Errorf("board investigation %d: %w", boardID, err)
	}

	// ── 2. 共享研究循环（lane 集来自父简报不可变快照；完整默认工具集）──
	evidenceNeeds := deriveMethodEvidenceNeeds(stage)
	research, err := o.RunBoardInvestigationResearch(ctx, BoardInvestigationResearchInput{
		SessionID:     sessionID,
		Question:      stage.Question,
		Brief:         brief,
		LaneWhitelist: laneWhitelist,
		DynamicGrants: dynamicGrants,
		Hypotheses:    stage.Hypotheses.Hypotheses,
		EvidenceNeeds: evidenceNeeds,
		// AllowedTools 留空 = explorationToolNames 完整默认（lane 内部工具 +
		// web_search + fetch_page）。
	})
	if err != nil {
		return nil, fmt.Errorf("board investigation %d: research: %w", boardID, err)
	}

	// ── 3. 综合阶段（失败 = 0 行，绝不编造）──
	payload, synthMeta, err := o.synthesizeBoardInvestigation(ctx, sessionID, stage.Question, brief, stage, research, laneWhitelist, boardID, dynamicGrants)
	if err != nil {
		return nil, fmt.Errorf("board investigation %d: %w", boardID, err)
	}
	payload.ParentBriefingID = parent.ID
	payload.Question = stage.Question
	payload.MethodRefs = stage.MethodRefs

	// ── 3.5 跨版块引用归属复验（add-evidence-backed-cross-board-relations）──
	// parse 阶段已按 grant 集合剔除幽灵引用；落库前对存活跨版块引用再做
	// DB 级归属校验（泳道被删除 / 迁移到其他版块 = 引用漂移），漂移引用
	// 机械剔除并留痕，不支撑任何结论。
	o.validateCrossBoardLaneRefs(ctx, payload, dynamicGrants)

	// ── 4. sanitize 完成 → 一次性构造不可变快照并落库 ──
	result, err := o.buildBoardInvestigationResult(boardID, parent.ID, questionKey, sessionID, payload, stage, research, synthMeta, brief, evidenceNeeds, parent.Sectors, freshness, dynamicGrants)
	if err != nil {
		return nil, err
	}
	if err := o.repo.CreateBoardInvestigationResult(ctx, result); err != nil {
		return nil, fmt.Errorf("board investigation %d: save investigation: %w", boardID, err)
	}

	// ── 5. 4.7 review judge：仅与同 parent+question_key 的上一份调查比较
	//    （不同问题/父简报/版块 0 judge）；全程 non-fatal，失败只记日志，
	//    已落库调查不回滚，lifeline 永不回写。
	review := o.judgeBoardInvestigationAgainstPrev(ctx, boardID, parent.ID, questionKey, sessionID, result)
	return &BoardInvestigationOutput{Result: result, Review: review}, nil
}

// validateCrossBoardLaneRefs re-checks surviving cross-board lane references
// against the database right before persistence (spec: 综合阶段 MUST 剔除
// 未授权、已删除或无法确认归属的引用). A reference whose lane no longer
// exists, or whose real owner board differs from the reference's board id /
// grant provenance, is dropped with a warning; board-local references (board
// id 0) are untouched.
func (o *OrchestratorService) validateCrossBoardLaneRefs(ctx context.Context, payload *boardInvestigationPayload, grants *DynamicLaneGrantSet) {
	if payload == nil || len(payload.LaneRefs) == 0 {
		return
	}
	kept := make([]laneRef, 0, len(payload.LaneRefs))
	for _, ref := range payload.LaneRefs {
		if ref.BoardID == 0 || ref.LaneID == 0 {
			kept = append(kept, ref)
			continue
		}
		var owner uint
		err := o.repo.DB().WithContext(ctx).
			Table("board_persistent_topics").
			Select("semantic_board_id").
			Where("id = ?", ref.LaneID).
			Scan(&owner).Error
		if err != nil || owner == 0 {
			logging.Warnf("board investigation: cross-board lane %d vanished — reference dropped", ref.LaneID)
			continue
		}
		if owner != ref.BoardID {
			logging.Warnf("board investigation: cross-board lane %d owner drifted (%d != %d) — reference dropped", ref.LaneID, owner, ref.BoardID)
			continue
		}
		if !grants.Has(ref.LaneID) {
			logging.Warnf("board investigation: cross-board lane %d not in grant audit — reference dropped", ref.LaneID)
			continue
		}
		kept = append(kept, ref)
	}
	payload.LaneRefs = kept
}

// buildBoardInvestigationResult assembles the immutable row AFTER every stage
// succeeded and the payload is fully sanitized (atomic boundary: nothing is
// persisted before this point).
func (o *OrchestratorService) buildBoardInvestigationResult(
	boardID, parentBriefID uint, questionKey, sessionID string,
	payload *boardInvestigationPayload,
	stage *boardHypothesisStageResult,
	research *BoardInvestigationResearchResult,
	synthMeta *boardSynthesisGenerationMeta,
	brief *BoardBriefPayload,
	evidenceNeeds []string,
	parentSectors json.RawMessage,
	freshness *boardInvestigationFreshnessSnapshot,
	dynamicGrants *DynamicLaneGrantSet,
) (*repository.TopicEnrichmentResult, error) {
	sectorsJSON, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("board investigation %d: marshal sectors: %w", boardID, err)
	}
	toolCallsJSON := boardToolCallsOrEmpty(research)
	researchSnap := boardInvestigationResearchSnapshot{
		Gaps:      []boardResearchGap{},
		FinalData: "",
	}
	if research != nil {
		researchSnap.Coverage = research.Coverage
		researchSnap.Gaps = research.Gaps
		researchSnap.FinishRejections = research.FinishRejections
		if research.Loop != nil {
			researchSnap.FinalData = research.Loop.FinalData
			researchSnap.Loops = research.Loop.Loops
		}
	}
	if researchSnap.Gaps == nil {
		researchSnap.Gaps = []boardResearchGap{}
	}
	snapshot := boardInvestigationInputSnapshot{
		ParentBriefID:     parentBriefID,
		ParentSectors:     json.RawMessage(parentSectors),
		ParentProjection:  renderBriefProjectionForInvestigation(brief),
		Question:          payload.Question,
		QuestionKey:       questionKey,
		LaneWhitelist:     laneWhitelistOrEmpty(deriveLaneWhitelistFromBrief(brief)),
		Freshness:         freshness,
		Methods:           stage.Methods,
		MethodPrompt:      stage.MethodPrompt,
		MethodCards:       stage.MethodCards,
		MethodRefs:        stage.MethodRefs,
		EvidenceNeeds:     evidenceNeeds,
		InitialHypotheses: &stage.Hypotheses,
		Research:          researchSnap,
		Synthesis:         synthMeta,
		DynamicGrants:     dynamicGrants.Audit(),
	}
	if snapshot.MethodCards == nil {
		snapshot.MethodCards = []AnalysisMethodCardTrace{}
	}
	if snapshot.MethodRefs == nil {
		snapshot.MethodRefs = []AnalysisMethodRef{}
	}
	snapJSON, err := json.Marshal(snapshot)
	if err != nil {
		return nil, fmt.Errorf("board investigation %d: marshal snapshot: %w", boardID, err)
	}
	return &repository.TopicEnrichmentResult{
		SemanticBoardID: repository.BoardIDPtr(boardID),
		AnalysisScope:   "board",
		ResultKind:      repository.ResultKindBoardInvestigation,
		ParentResultID:  &parentBriefID,
		QuestionKey:     &questionKey,
		Sectors:         sectorsJSON,
		ToolCalls:       toolCallsJSON,
		InputSnapshot:   snapJSON,
		SessionID:       sessionID,
	}, nil
}

// boardToolCallsOrEmpty marshals the shared research record (complete ordered
// tool calls incl. purpose/outcome/full results); empty → "[]" (never null).
func boardToolCallsOrEmpty(research *BoardInvestigationResearchResult) json.RawMessage {
	if research == nil || research.Loop == nil || len(research.Loop.ToolCalls) == 0 {
		return json.RawMessage("[]")
	}
	b, err := json.Marshal(research.Loop.ToolCalls)
	if err != nil {
		return json.RawMessage("[]")
	}
	return b
}

// laneWhitelistOrEmpty normalizes nil to [] for stable snapshot JSON.
func laneWhitelistOrEmpty(ids []uint) []uint {
	if ids == nil {
		return []uint{}
	}
	return ids
}

// deriveMethodEvidenceNeeds projects the selected+injected methods'
// RequiredEvidence into the research loop's evidence-needs list (D6: the
// research agent receives method-derived checklists, never method prose).
func deriveMethodEvidenceNeeds(stage *boardHypothesisStageResult) []string {
	if stage == nil {
		return nil
	}
	byID := make(map[uint]boardMethodCandidate, len(stage.Methods.Candidates))
	for _, c := range stage.Methods.Candidates {
		byID[c.ID] = c
	}
	needs := []string{}
	seen := map[string]bool{}
	for _, ref := range stage.MethodRefs {
		c, ok := byID[ref.ID]
		if !ok {
			continue
		}
		for _, n := range c.RequiredEvidence {
			n = truncateRunes(strings.TrimSpace(n), boardSynthesisMaxGapRunes)
			if n == "" || seen[n] {
				continue
			}
			seen[n] = true
			needs = append(needs, n)
		}
	}
	return needs
}
