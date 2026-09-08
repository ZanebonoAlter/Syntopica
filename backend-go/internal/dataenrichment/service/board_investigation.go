package service

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"

	"syntopica-backend/internal/dataenrichment/repository"
	"syntopica-backend/internal/platform/airouter"
	"syntopica-backend/internal/platform/logging"
)

// ── board_investigation 前置阶段：方法选择 → 竞争假设 ───────────────────────
//
// design D4/D6、tasks 4.1/4.3、test-cases M4/M7。调查链的前两步：
//
//  1. board_method_select：只按「用户问题 + 父简报投影 + enabled 方法元数据
//     （id/name/title/summary/selection_meta）」选 0-2 张卡。绝不加载正文、
//     绝不读取尚未生成的候选假设（无选择循环，单次调用）。坏 JSON/LLM 失败
//     降级选 0 张，调查照常继续。不用关键词硬匹配冒充语义选择。
//  2. 按选择顺序加载正文 → AssembleSelectedAnalysisMethods 整卡预算装配 →
//     board_hypothesize：问题 + 父简报投影 + 最终注入的方法全文 → 2-4 个
//     竞争假设。假设阶段只声明 support_needed/disconfirm_needed/scope，
//     不预选赢家、不带 assessment/confidence/evidence（那是 4.4 研究 loop
//     与 4.5 综合的事）。方法正文只进本调用（未来 synth 也可）。
//
// 韧性契约（M4.3-M4.6）：坏 JSON/数量或必填不合格/无 H0 → 纠错重试一次；
// 第二次仍无 H0 → 机械补入朴素 H0（必要时裁到 ≤4）；第二次结构不可用 →
// 返回 error，绝不凭空造完整非零假设。0 方法正常。
//
// 本切片不写 result 表：input_snapshot/method_refs 持久化待 4.6。

// ai_call_logs operations（airouter 强制非空；清单见 standard/backend/ai-logging.md）。
const (
	boardMethodSelectOperation = "data_enrichment.board_method_select"
	boardHypothesizeOperation  = "data_enrichment.board_hypothesize"
)

// Question source enum（D5）：generated=简报候选问题，custom=用户自填。
const (
	QuestionSourceGenerated = "generated"
	QuestionSourceCustom    = "custom"
)

// Hypothesis count / length caps（D4 / M4.5 / M4.6）。
const (
	boardHypothesisMinCount = 2
	boardHypothesisMaxCount = 4

	boardHypothesisMaxLabelRunes  = 120
	boardHypothesisMaxScopeRunes  = 200
	boardHypothesisMaxNeededRunes = 160
	boardHypothesisMaxNeededItems = 4
)

// Selector-side caps.
const (
	boardMethodSelectReasonRunes = 300
	// 方法正文整卡注入预算（rune）。超限整卡舍弃（AssembleSelectedAnalysisMethods），
	// 不撕裂单卡内容（M7.4）。
	boardInvestigationMethodRuneBudget = 4000
	// 父简报投影渲染预算（rune）；payload 字段在简报生成时已各自截断。
	boardInvestigationBriefProjectionRunes = 3000
)

// 方法库 summaries 读取失败时的安全降级原因码：写入 trace/snapshot 的只有
// 这个稳定常量，完整 err 只进日志（M4 韧性：内部错误不泄入快照）。
const methodSummariesUnavailableWhy = "method summaries unavailable"

// selector LLM 调用/解析失败时的安全降级原因码，语义同上：DegradedWhy 会随
// input_snapshot 固化，只能携带稳定 reason code，完整 err（可能含内部地
// 址、SQL、上游报文）只进 Warn 日志，绝不进 trace/snapshot。
const (
	methodSelectionChatUnavailableWhy = "method_selection_unavailable"
	methodSelectionInvalidResponseWhy = "invalid_response"
)

// selector 返回空 reason 时的稳定占位（trace 里不出现空串）。
const boardMethodSelectReasonPlaceholder = "reason_unspecified"

// Hypothesize 阶段重试原因码（snapshot 安全）：RetryReason 会随
// input_snapshot 固化，只能携带稳定机器码；完整 err（可能含内部地址、SQL、
// 上游报文）只进 Warn 日志与纠错 retry prompt（后者经 ai_call_logs 完整留痕，
// 属规范要求的调用记录，不进 snapshot）。review Minor 修复：此前 RetryReason
// 直接携带 lastErr.Error()，内部细节会随快照落库。
const (
	hypothesisRetryChat      = "chat_error"
	hypothesisRetryParse     = "parse_error"
	hypothesisRetryStructure = "invalid_structure"
	hypothesisRetryNoH0      = "no_null_hypothesis"
)

// BoardInvestigationQuestion is the investigation question contract (D5).
// 显示用 ID 可选（generated 取简报候选 id；custom 可空）；question_key 由
// 持久层（4.6）按规范化问题文本 hash 生成，显示 id 不参与跨请求身份。
type BoardInvestigationQuestion struct {
	ID     string `json:"id,omitempty"`
	Text   string `json:"text"`
	Source string `json:"source"` // generated|custom
}

// Normalize trims text/id and validates the source enum. The trimmed text is
// also the future question_key hash input; key derivation itself stays in the
// persistence layer.
func (q *BoardInvestigationQuestion) Normalize() error {
	q.Text = strings.TrimSpace(q.Text)
	q.ID = strings.TrimSpace(q.ID)
	if q.Text == "" {
		return errors.New("investigation question: text must be non-empty")
	}
	if q.Source != QuestionSourceGenerated && q.Source != QuestionSourceCustom {
		return fmt.Errorf("investigation question: source must be %s or %s, got %q",
			QuestionSourceGenerated, QuestionSourceCustom, q.Source)
	}
	return nil
}

// ── 方法候选投影与选择 trace ────────────────────────────────────────────────

// boardMethodCandidate is the selector-safe projection of an enabled method
// card: metadata only, Content never loaded (D6/M7 阶段隔离)。
type boardMethodCandidate struct {
	ID               uint     `json:"id"`
	Name             string   `json:"name"`
	Title            string   `json:"title"`
	Summary          string   `json:"summary"`
	ApplicableWhen   []string `json:"applicable_when"`
	AvoidWhen        []string `json:"avoid_when"`
	RequiredEvidence []string `json:"required_evidence"`
	FailureModes     []string `json:"failure_modes"`
}

// boardMethodCandidates projects enabled summaries (Content empty by query
// construction in ListEnabledAnalysisMethodSummaries) into selector candidates.
func boardMethodCandidates(methods []repository.AnalysisMethod) []boardMethodCandidate {
	out := make([]boardMethodCandidate, 0, len(methods))
	for _, m := range methods {
		title := strings.TrimSpace(m.Title)
		if title == "" {
			title = m.Name
		}
		out = append(out, boardMethodCandidate{
			ID:               m.ID,
			Name:             m.Name,
			Title:            title,
			Summary:          strings.TrimSpace(m.Summary),
			ApplicableWhen:   m.SelectionMeta.ApplicableWhen,
			AvoidWhen:        m.SelectionMeta.AvoidWhen,
			RequiredEvidence: m.SelectionMeta.RequiredEvidence,
			FailureModes:     m.SelectionMeta.FailureModes,
		})
	}
	return out
}

// boardMethodSelected is one post-parse selection entry (relevance order).
// avoid_matched=true 的提名在 parser 中必被剔除，永不进入本列表。
type boardMethodSelected struct {
	ID     uint   `json:"id"`
	Title  string `json:"title"`
	Reason string `json:"reason"`
}

// boardMethodSelection is the full selector trace：候选摘要（可追溯）、解析后
// 选中（含理由）、剔除留痕（unknown_id/avoid_matched/duplicate/selection_limit
// /budget_exceeded/load_failed/load_missed）与降级状态。4.6 把它整体固化进
// input_snapshot。
type boardMethodSelection struct {
	Candidates  []boardMethodCandidate  `json:"candidates"`
	Selected    []boardMethodSelected   `json:"selected"`
	Dropped     []DroppedAnalysisMethod `json:"dropped"`
	Degraded    bool                    `json:"degraded,omitempty"`
	DegradedWhy string                  `json:"degraded_why,omitempty"`
	// Attempts: selector LLM calls made. 0-1 only — 无重试、无选择循环。
	Attempts int `json:"attempts"`
}

// boardMethodSelectPrompt is the selector contract：只有元数据输入，输出仅
// selected 列表；不索取评估/赢家（那是 hypothesize 之后的事）。
const boardMethodSelectPrompt = `你是一位严谨的研究方法编排员。一次板块深度调查即将开始：先选方法卡，再生成竞争假设（假设尚未生成，本步骤不读取任何假设内容）。只根据下方「调查问题」与「父简报投影」，从「方法候选」中为假设生成阶段选择 0-2 张最适配的方法卡。

硬性纪律：
1. 只能选择方法候选中列出的方法 id，最多 2 张；没有适配方法就返回空数组 selected，不强选最接近的方法
2. avoid_when（禁用条件）命中当前情形的卡不得入选：若你认为某卡本可适配但禁用条件命中，请把它列入 selected 并标 avoid_matched=true（系统会强制剔除该卡）
3. 只依据候选给出的名称/摘要/适用/禁用/所需证据/失败模式判断，不臆造卡的能力
4. 方法卡是过程与检查清单，不是事实来源，也与作者文风无关；选择只看问题与简报内容的匹配

输出严格 JSON（不要 markdown 包裹、不要任何其他文字）：
{"selected":[{"id":11,"reason":"该卡适用条件与本问题/简报的对应关系","avoid_matched":false}]}`

// assembleBoardMethodSelectPrompt builds the selector prompt (pure function;
// contract unit-tested: no method Content, no hypotheses, no legacy roles).
func assembleBoardMethodSelectPrompt(q BoardInvestigationQuestion, brief *BoardBriefPayload, candidates []boardMethodCandidate) string {
	var sb strings.Builder
	sb.WriteString(boardMethodSelectPrompt)
	sb.WriteString("\n\n---\n调查问题：" + q.Text)
	sourceDesc := "用户自填问题"
	if q.Source == QuestionSourceGenerated {
		sourceDesc = "来自父简报的研究候选问题"
	}
	if q.ID != "" {
		sourceDesc += "（简报候选 id " + q.ID + "）"
	}
	sb.WriteString("\n来源：" + q.Source + "（" + sourceDesc + "）")
	sb.WriteString("\n\n---\n父简报投影：\n" + renderBriefProjectionForInvestigation(brief))
	sb.WriteString("\n\n---\n方法候选（只有元数据，没有操作正文）：")
	for _, c := range candidates {
		fmt.Fprintf(&sb, "\n- #%d《%s》（name: %s）", c.ID, c.Title, c.Name)
		if c.Summary != "" {
			sb.WriteString("\n  摘要: " + c.Summary)
		}
		sb.WriteString(metaLine("适用", c.ApplicableWhen))
		sb.WriteString(metaLine("禁用", c.AvoidWhen))
		sb.WriteString(metaLine("所需证据", c.RequiredEvidence))
		sb.WriteString(metaLine("已知失败模式", c.FailureModes))
	}
	return sb.String()
}

func metaLine(label string, items []string) string {
	if len(items) == 0 {
		return ""
	}
	return "\n  " + label + ": " + strings.Join(items, "；")
}

// parseBoardMethodSelection enforces, deterministically and in LLM order:
// enabled-candidate whitelist → avoid_matched removal → dedupe → max 2.
// Every rejected nomination is traced in dropped with a machine reason.
func parseBoardMethodSelection(parsed map[string]any, candidates []boardMethodCandidate) ([]boardMethodSelected, []DroppedAnalysisMethod) {
	whitelist := make(map[uint]boardMethodCandidate, len(candidates))
	for _, c := range candidates {
		whitelist[c.ID] = c
	}
	selected := []boardMethodSelected{}
	dropped := []DroppedAnalysisMethod{}
	seen := map[uint]bool{}
	raw, ok := parsed["selected"].([]any)
	if !ok {
		return selected, dropped
	}
	for _, v := range raw {
		m, ok := v.(map[string]any)
		if !ok {
			continue
		}
		// ID 只接受正整数且不超过 JS safe integer（2^53-1）：小数（11.5 不得
		// 截断成 11）、负数、零、非数值、超大数（float64 精度丢失、uint 转换
		// 会溢出 wrap）一律忽略（无法用 uint ID 落 trace，只记日志；宁齐勿错）。
		const boardMethodSelectMaxSafeID = 9007199254740991 // 2^53-1，JS safe integer 上限
		idf, ok := m["id"].(float64)
		if !ok || idf <= 0 || idf != math.Trunc(idf) || idf > boardMethodSelectMaxSafeID {
			logging.Warnf("board method select: entry without safe positive integer id — ignored")
			continue
		}
		id := uint(idf)
		cand, known := whitelist[id]
		title := fmt.Sprintf("方法#%d", id)
		if known {
			title = cand.Title
		}
		reason := strings.TrimSpace(getString(m, "reason"))
		if reason == "" {
			reason = boardMethodSelectReasonPlaceholder
		}
		reason = truncateRunes(reason, boardMethodSelectReasonRunes)
		switch {
		case !known:
			dropped = append(dropped, DroppedAnalysisMethod{ID: id, Title: title, Reason: "unknown_id"})
		case getBool(m, "avoid_matched"):
			// avoid_when 命中必剔除（M7.2），不强选最接近项。
			dropped = append(dropped, DroppedAnalysisMethod{ID: id, Title: title, Reason: "avoid_matched"})
		case seen[id]:
			dropped = append(dropped, DroppedAnalysisMethod{ID: id, Title: title, Reason: "duplicate"})
		case len(selected) >= maxSelectedAnalysisMethods:
			dropped = append(dropped, DroppedAnalysisMethod{ID: id, Title: title, Reason: "selection_limit"})
		default:
			seen[id] = true
			selected = append(selected, boardMethodSelected{ID: id, Title: title, Reason: reason})
		}
	}
	return selected, dropped
}

// selectBoardAnalysisMethods runs the single selector LLM call. Failure of any
// kind (transport, bad JSON) degrades to zero methods — never a retry loop,
// never a keyword-matching fallback (语义选择只有 LLM 或空)。
func (o *OrchestratorService) selectBoardAnalysisMethods(ctx context.Context, sessionID string, q BoardInvestigationQuestion, brief *BoardBriefPayload, candidates []boardMethodCandidate) boardMethodSelection {
	sel := boardMethodSelection{
		Candidates: candidates,
		Selected:   []boardMethodSelected{},
		Dropped:    []DroppedAnalysisMethod{},
		Attempts:   1,
	}
	resp, err := o.airouter.Chat(ctx, airouter.ChatRequest{
		Capability:  o.capability,
		Operation:   boardMethodSelectOperation,
		SessionID:   sessionID,
		Messages:    []airouter.Message{{Role: "user", Content: assembleBoardMethodSelectPrompt(q, brief, candidates)}},
		Temperature: floatPtr(0.2), // 选择任务求稳，低温度
		JSONMode:    true,
	})
	if err != nil {
		logging.Warnf("board method select chat failed (degrade to 0 methods): %v", err)
		sel.Degraded, sel.DegradedWhy = true, methodSelectionChatUnavailableWhy
		return sel
	}
	parsed, err := ParseJSONResponse(resp.Content)
	if err != nil {
		logging.Warnf("board method select response unparsable (degrade to 0 methods): %v", err)
		sel.Degraded, sel.DegradedWhy = true, methodSelectionInvalidResponseWhy
		return sel
	}
	sel.Selected, sel.Dropped = parseBoardMethodSelection(parsed, candidates)
	return sel
}

// ── 父简报投影 ──────────────────────────────────────────────────────────────

// renderBriefProjectionForInvestigation renders the parent brief projection
// (summary/observations/relationships/uncertainties/research_questions/
// lane_refs) shared by the selector and hypothesize prompts. Compact, honest,
// rune-budgeted; sections with no content are omitted rather than invented.
func renderBriefProjectionForInvestigation(brief *BoardBriefPayload) string {
	if brief == nil {
		return "（无父简报投影）"
	}
	var sb strings.Builder
	if s := strings.TrimSpace(brief.Summary); s != "" {
		sb.WriteString("概览: " + s + "\n")
	}
	if len(brief.Observations) > 0 {
		sb.WriteString("关键观察:\n")
		for _, o := range brief.Observations {
			fmt.Fprintf(&sb, "- [%s] 泳道#%d：%s（依据：%s；截至 %s）\n", o.ID, o.LaneID, o.Statement, o.Basis, o.AsOfDate)
		}
	}
	if len(brief.Relationships) > 0 {
		sb.WriteString("跨泳道关系:\n")
		for _, r := range brief.Relationships {
			lanes := make([]string, 0, len(r.LaneIDs))
			for _, id := range r.LaneIDs {
				lanes = append(lanes, fmt.Sprintf("#%d", id))
			}
			fmt.Fprintf(&sb, "- 泳道%s：%s（置信 %s）— %s\n", strings.Join(lanes, "×"), r.Type, r.Confidence, r.Explanation)
		}
	}
	if len(brief.Uncertainties) > 0 {
		sb.WriteString("不确定项:\n")
		for _, u := range brief.Uncertainties {
			fmt.Fprintf(&sb, "- %s（需要：%s）\n", u.Question, u.NeededEvidence)
		}
	}
	if len(brief.ResearchQuestions) > 0 {
		sb.WriteString("候选研究问题:\n")
		for _, rq := range brief.ResearchQuestions {
			fmt.Fprintf(&sb, "- [%s] %s\n", rq.ID, rq.Question)
		}
	}
	if len(brief.LaneRefs) > 0 {
		sb.WriteString("泳道:\n")
		for _, lr := range brief.LaneRefs {
			fmt.Fprintf(&sb, "- 泳道#%d：%s\n", lr.LaneID, lr.Note)
		}
	}
	out := strings.TrimSpace(sb.String())
	if out == "" {
		return "（父简报无可用投影内容）"
	}
	if r := len([]rune(out)); r > boardInvestigationBriefProjectionRunes {
		out = truncateRunes(out, boardInvestigationBriefProjectionRunes) + "\n[父简报投影已截断]"
	}
	return out
}

// ── 竞争假设 ────────────────────────────────────────────────────────────────

// boardHypothesis is one competing hypothesis at declaration time. By
// construction it carries NO assessment/confidence/winner/evidence — those
// belong to the research loop (4.4) and synthesis (4.5).
type boardHypothesis struct {
	ID               string   `json:"id"`
	Label            string   `json:"label"`
	IsNull           bool     `json:"is_null"`
	SupportNeeded    []string `json:"support_needed"`
	DisconfirmNeeded []string `json:"disconfirm_needed"`
	Scope            string   `json:"scope"`
}

// boardHypothesisGeneration traces how the final hypothesis set was produced.
type boardHypothesisGeneration struct {
	Hypotheses  []boardHypothesis `json:"hypotheses"`
	Attempts    int               `json:"attempts"`               // LLM calls made (1 or 2)
	RetryReason string            `json:"retry_reason,omitempty"` // why attempt 1 was rejected
	H0Injected  bool              `json:"h0_injected,omitempty"`  // mechanical H0 added on attempt 2
}

// boardHypothesizePrompt is the hypothesis contract：2-4 个互斥竞争假设 +
// 必含朴素零假设 + 每个假设先声明证据需求；显式禁止预判赢家。
const boardHypothesizePrompt = `你是一位恪守证据纪律的研究设计员。针对下方「调查问题」与「父简报投影」，提出 2-4 个相互竞争的假设，为后续检索与核查做准备。

硬性纪律：
1. 必须包含至少一个零假设（is_null=true）：即"没有统一机制，这些变化可由各自独立的普通因素分别解释"或与调查问题等价的朴素解释；零假设不得被弱化成变相的结构命题
2. 非零假设之间尽量互斥（能被不同证据区分开），不要把同一个宏大叙事换个说法重复多遍；无法被证据支持或削弱的宏大叙事不算合格假设
3. 每个假设必须声明：support_needed（什么证据会支持它）、disconfirm_needed（什么证据会削弱或推翻它）、scope（适用范围），三者都不得为空
4. 此阶段不判断谁更可信：不要输出任何评估、置信度、赢家或证据——那是后续研究阶段的任务
5. 若下方给出「分析方法参考」，把它当作操作步骤与检查清单来约束假设设计；方法不是事实来源，不要为迎合方法而编造内容

输出严格 JSON（不要 markdown 包裹、不要任何其他文字）：
{"hypotheses":[{"id":"h0","label":"...","is_null":true,"support_needed":["..."],"disconfirm_needed":["..."],"scope":"..."}]}`

// boardHypothesizeRetryLead is the stable prefix of the corrective retry note
// (test anchor; the full note embeds the concrete failure reason).
const boardHypothesizeRetryLead = "上次输出不是合格的假设集合"

const boardHypothesizeRetryNote = "\n\n---\n" + boardHypothesizeRetryLead + "（问题：%s）。请重新输出完整 JSON：2-4 个假设、必含 is_null=true 的零假设（朴素解释，不得弱化为变相结构命题）、每个假设带非空 support_needed/disconfirm_needed/scope；不要输出评估、置信度或赢家。"

// assembleBoardHypothesizePrompt builds the hypothesize prompt. methodsPrompt
// is the whole-card assembly from AssembleSelectedAnalysisMethods; it is the
// ONLY place method content enters the investigation chain (future synth
// excepted). Empty methodsPrompt renders no method section (0 方法正常).
func assembleBoardHypothesizePrompt(q BoardInvestigationQuestion, brief *BoardBriefPayload, methodsPrompt string) string {
	var sb strings.Builder
	sb.WriteString(boardHypothesizePrompt)
	sb.WriteString("\n\n---\n调查问题：" + q.Text)
	sourceDesc := "用户自填问题"
	if q.Source == QuestionSourceGenerated {
		sourceDesc = "来自父简报的研究候选问题"
	}
	if q.ID != "" {
		sourceDesc += "（简报候选 id " + q.ID + "）"
	}
	sb.WriteString("\n来源：" + q.Source + "（" + sourceDesc + "）")
	sb.WriteString("\n\n---\n父简报投影：\n" + renderBriefProjectionForInvestigation(brief))
	if p := strings.TrimSpace(methodsPrompt); p != "" {
		sb.WriteString("\n\n---\n分析方法参考（仅约束假设设计的过程与检查清单，不是事实来源）：\n" + p)
	}
	return sb.String()
}

// scrubNeededList trims entries, drops empties, caps item runes and count.
func scrubNeededList(raw any) []string {
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
		s = truncateRunes(strings.TrimSpace(s), boardHypothesisMaxNeededRunes)
		if s == "" {
			continue
		}
		out = append(out, s)
		if len(out) >= boardHypothesisMaxNeededItems {
			break
		}
	}
	return out
}

// assignHypothesisIDs guarantees unique ids: explicit trimmed id wins, missing
// ids auto-assign h1..hN, collisions get deterministic numeric suffixes.
func assignHypothesisIDs(hs []boardHypothesis) {
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

// parseBoardHypotheses validates one parsed LLM payload. Per-item violations
// (empty label/scope, empty support/disconfirm lists) drop THAT item only;
// fewer than 2 surviving valid hypotheses is a structural failure (retry
// signal). >4 truncates to the first 4 (M4.6 截断或重试). Unknown fields
// (assessment/confidence/winner...) are ignored by construction — the struct
// cannot carry them.
func parseBoardHypotheses(parsed map[string]any) ([]boardHypothesis, error) {
	raw, ok := parsed["hypotheses"].([]any)
	if !ok {
		return nil, errors.New("hypotheses: missing or not an array")
	}
	out := []boardHypothesis{}
	for _, v := range raw {
		m, ok := v.(map[string]any)
		if !ok {
			continue
		}
		label := truncateRunes(strings.TrimSpace(getString(m, "label")), boardHypothesisMaxLabelRunes)
		if label == "" {
			logging.Warnf("board hypothesize: hypothesis without label — dropped")
			continue
		}
		support := scrubNeededList(m["support_needed"])
		disconfirm := scrubNeededList(m["disconfirm_needed"])
		scope := truncateRunes(strings.TrimSpace(getString(m, "scope")), boardHypothesisMaxScopeRunes)
		if len(support) == 0 || len(disconfirm) == 0 || scope == "" {
			logging.Warnf("board hypothesize: hypothesis %q missing support/disconfirm/scope — dropped", label)
			continue
		}
		out = append(out, boardHypothesis{
			ID:               strings.TrimSpace(getString(m, "id")),
			Label:            label,
			IsNull:           getBool(m, "is_null"),
			SupportNeeded:    support,
			DisconfirmNeeded: disconfirm,
			Scope:            scope,
		})
		if len(out) >= boardHypothesisMaxCount {
			break
		}
	}
	if len(out) < boardHypothesisMinCount {
		return nil, fmt.Errorf("hypotheses: need at least %d structurally valid hypotheses, got %d", boardHypothesisMinCount, len(out))
	}
	assignHypothesisIDs(out)
	return out, nil
}

func hasNullHypothesis(hs []boardHypothesis) bool {
	for _, h := range hs {
		if h.IsNull {
			return true
		}
	}
	return false
}

// mechanicalNullHypothesis is the deterministic plain H0 (M4.3)：朴素解释，
// 带自己的证据需求，id 与现有集合去冲突。绝不凭空造非零假设。
func mechanicalNullHypothesis(existing []boardHypothesis) boardHypothesis {
	h := boardHypothesis{
		ID:     "h0",
		Label:  "零假设（机械补入）：调查问题涉及的这些变化之间没有统一机制，可由各自独立的普通因素分别解释",
		IsNull: true,
		SupportNeeded: []string{
			"能同时解释多条泳道变化的可信共同机制或传导链（一手材料优先）",
		},
		DisconfirmNeeded: []string{
			"对每条泳道分别成立的局部解释，且这些解释不需要共享驱动",
		},
		Scope: "本次调查问题涉及的全部泳道与观察",
	}
	seen := make(map[string]bool, len(existing))
	for _, e := range existing {
		seen[e.ID] = true
	}
	for n := 2; seen[h.ID]; n++ {
		h.ID = fmt.Sprintf("h0_%d", n)
	}
	return h
}

// generateBoardHypotheses runs the hypothesize LLM call with one corrective
// retry. Error paths: two unusable attempts (bad JSON / structurally invalid)
// return the last error — the caller fails the investigation cleanly instead
// of fabricating a full non-null set. A structurally valid second attempt
// without H0 gets the mechanical plain H0 prepended (truncate to ≤4).
func (o *OrchestratorService) generateBoardHypotheses(ctx context.Context, sessionID string, q BoardInvestigationQuestion, brief *BoardBriefPayload, methodsPrompt string) (*boardHypothesisGeneration, error) {
	gen := &boardHypothesisGeneration{}
	prompt := assembleBoardHypothesizePrompt(q, brief, methodsPrompt)
	var lastErr error
	for attempt := 1; attempt <= 2; attempt++ {
		gen.Attempts = attempt
		attemptPrompt := prompt
		if attempt > 1 && lastErr != nil {
			attemptPrompt = prompt + fmt.Sprintf(boardHypothesizeRetryNote, lastErr.Error())
		}
		resp, err := o.airouter.Chat(ctx, airouter.ChatRequest{
			Capability:  o.capability,
			Operation:   boardHypothesizeOperation,
			SessionID:   sessionID,
			Messages:    []airouter.Message{{Role: "user", Content: attemptPrompt}},
			Temperature: floatPtr(0.3),
			JSONMode:    true,
		})
		if err != nil {
			lastErr = fmt.Errorf("board hypothesize chat: %w", err)
			if attempt == 1 {
				gen.RetryReason = hypothesisRetryChat
			}
			continue
		}
		parsed, err := ParseJSONResponse(resp.Content)
		if err != nil {
			lastErr = fmt.Errorf("board hypothesize parse: %w", err)
			if attempt == 1 {
				gen.RetryReason = hypothesisRetryParse
			}
			continue
		}
		hs, err := parseBoardHypotheses(parsed)
		if err != nil {
			lastErr = err
			if attempt == 1 {
				gen.RetryReason = hypothesisRetryStructure
			}
			continue
		}
		if !hasNullHypothesis(hs) {
			// M4.3/M4.4：缺零假设（含"只有宏大非零解释"）不可直接进入研究。
			lastErr = errors.New("hypotheses: no is_null=true zero hypothesis (仅宏大非零解释不可接受)")
			if attempt == 1 {
				gen.RetryReason = hypothesisRetryNoH0
				continue
			}
			h0 := mechanicalNullHypothesis(hs)
			merged := make([]boardHypothesis, 0, boardHypothesisMaxCount)
			merged = append(merged, h0)
			for _, h := range hs {
				if len(merged) >= boardHypothesisMaxCount {
					break
				}
				merged = append(merged, h)
			}
			hs = merged
			gen.H0Injected = true
			logging.Warnf("board hypothesize: second attempt still lacks H0 — mechanically injected plain null hypothesis")
		}
		gen.Hypotheses = hs
		return gen, nil
	}
	return nil, lastErr
}

// ── 阶段装配（select → load → hypothesize，供 4.4/4.6 复用）─────────────────

// analysisMethodLoader loads full method cards for the selected ids in the
// caller's relevance order (production: repository.GetAnalysisMethodsByIDs).
type analysisMethodLoader func(ctx context.Context, ids []uint) ([]repository.AnalysisMethod, error)

// boardHypothesisStageResult is the reusable stage artifact for tasks 4.4
// (shared research loop consumes Hypotheses + Question) and 4.6 (persists
// Question / Methods / MethodPrompt / MethodRefs into the investigation
// input_snapshot and result). Nothing here writes the result table.
type boardHypothesisStageResult struct {
	Question     BoardInvestigationQuestion `json:"question"`
	Methods      boardMethodSelection       `json:"methods"`
	MethodPrompt string                     `json:"method_prompt"` // actually injected method content ("" = none)
	MethodRefs   []AnalysisMethodRef        `json:"method_refs"`
	MethodCards  []AnalysisMethodCardTrace  `json:"method_cards"` // 按卡清洗/注入留痕（4.6 随 input_snapshot 固化）
	Hypotheses   boardHypothesisGeneration  `json:"hypotheses"`
}

// prepareBoardHypotheses runs the strict-order stage:
//
//	question 校验 → (summariesErr==nil 且有候选才) selector 单次 LLM →
//	(选中且 loader 可用才) load 正文 → 修辞清洗 + 整卡预算装配 →
//	hypothesize（≤2 次 LLM）。
//
// summariesErr 非 nil（方法库读取失败）时：降级 0 方法继续，trace 只留
// Degraded=true + 稳定原因码 methodSummariesUnavailableWhy，selector 0 次，
// hypothesize 照跑；完整 err 只进日志，不进 trace/snapshot。selector/loader
// 失败一律降级 0 方法继续；loader 为 nil 但选中非空时逐卡 no_loader 留痕；
// hypothesize 两次不可用才 error。0 张候选时完全不调 selector（空方法库
// 正常，M7.1）。
func (o *OrchestratorService) prepareBoardHypotheses(
	ctx context.Context, sessionID string,
	question BoardInvestigationQuestion, brief *BoardBriefPayload,
	summaries []repository.AnalysisMethod, summariesErr error,
	load analysisMethodLoader,
) (*boardHypothesisStageResult, error) {
	if err := question.Normalize(); err != nil {
		return nil, err
	}
	if brief == nil {
		return nil, errors.New("board investigation: parent brief required")
	}

	var selection boardMethodSelection
	switch {
	case summariesErr != nil:
		// 方法库读取失败：调查是主线，方法是增益。降级 0 方法、selector 不跑，
		// trace 只留安全原因码，完整 err 仅进日志（不泄入 snapshot）。
		logging.Warnf("board investigation: enabled method summaries unavailable (continue with 0 methods): %v", summariesErr)
		selection = boardMethodSelection{
			Candidates:  []boardMethodCandidate{},
			Selected:    []boardMethodSelected{},
			Dropped:     []DroppedAnalysisMethod{},
			Degraded:    true,
			DegradedWhy: methodSummariesUnavailableWhy,
		}
	default:
		candidates := boardMethodCandidates(summaries)
		if len(candidates) == 0 {
			// 空方法库：不调 selector（M7.1），调查照常。
			selection = boardMethodSelection{
				Candidates: []boardMethodCandidate{},
				Selected:   []boardMethodSelected{},
				Dropped:    []DroppedAnalysisMethod{},
			}
		} else {
			selection = o.selectBoardAnalysisMethods(ctx, sessionID, question, brief, candidates)
		}
	}

	result := &boardHypothesisStageResult{
		Question:    question,
		Methods:     selection,
		MethodRefs:  []AnalysisMethodRef{},
		MethodCards: []AnalysisMethodCardTrace{},
	}

	// 严格顺序：先选后载。选中 0 张时连 loader 都不碰；选中非空但 loader 为
	// nil（调用方装配遗漏）：逐卡 no_loader 留痕，不静默丢弃。dropped 一律
	// 追加到 result.Methods（selection 已被值拷贝进 result，改局部变量不会生效）。
	if len(selection.Selected) > 0 {
		if load == nil {
			for _, s := range selection.Selected {
				result.Methods.Dropped = append(result.Methods.Dropped, DroppedAnalysisMethod{ID: s.ID, Title: s.Title, Reason: "no_loader"})
			}
		} else {
			ids := make([]uint, 0, len(selection.Selected))
			for _, s := range selection.Selected {
				ids = append(ids, s.ID)
			}
			loaded, err := load(ctx, ids)
			if err != nil {
				logging.Warnf("board investigation: method load failed (continue with 0 methods): %v", err)
				for _, s := range selection.Selected {
					result.Methods.Dropped = append(result.Methods.Dropped, DroppedAnalysisMethod{ID: s.ID, Title: s.Title, Reason: "load_failed"})
				}
			} else {
				byID := make(map[uint]repository.AnalysisMethod, len(loaded))
				for _, m := range loaded {
					byID[m.ID] = m
				}
				ordered := make([]repository.AnalysisMethod, 0, len(ids))
				for _, s := range selection.Selected {
					if m, ok := byID[s.ID]; ok {
						ordered = append(ordered, m)
					} else {
						// 选中与加载之间被软删/停用：留痕不阻塞。
						result.Methods.Dropped = append(result.Methods.Dropped, DroppedAnalysisMethod{ID: s.ID, Title: s.Title, Reason: "load_missed"})
					}
				}
				assembly := AssembleSelectedAnalysisMethods(ordered, boardInvestigationMethodRuneBudget)
				result.MethodPrompt = assembly.Prompt
				result.MethodRefs = assembly.MethodRefs
				result.MethodCards = assembly.Cards
				result.Methods.Dropped = append(result.Methods.Dropped, assembly.Dropped...)
			}
		}
	}

	gen, err := o.generateBoardHypotheses(ctx, sessionID, question, brief, result.MethodPrompt)
	if err != nil {
		return nil, fmt.Errorf("board hypothesize: %w", err)
	}
	result.Hypotheses = *gen
	return result, nil
}

// HypothesizeBoardInvestigation runs the stage against the live method library
// (list enabled summaries → select → load → hypothesize)。sessionID 复用版块
// 调查前缀（data_enrichment_board_{board_id}_{uuid8} 家族），由调查编排入口
// 生成并传入（完整调查链接线在 4.4/4.6）。导出以衔接后续调查编排与测试。
// summaries 读取失败由 prepareBoardHypotheses 统一降级留痕（安全原因码 +
// 日志全 err）；repo 未接线是装配错误，直接拒绝。
func (o *OrchestratorService) HypothesizeBoardInvestigation(ctx context.Context, sessionID string, question BoardInvestigationQuestion, brief *BoardBriefPayload) (*boardHypothesisStageResult, error) {
	if o.repo == nil {
		return nil, errors.New("board investigation: repository not wired")
	}
	summaries, err := o.repo.ListEnabledAnalysisMethodSummaries(ctx)
	if err != nil {
		// 降级语义（Degraded trace + 完整 err 日志）统一在 prepareBoardHypotheses
		// 处理；err 非 nil 时 summaries 一律作废，防止半截元数据进 selector。
		summaries = nil
	}
	return o.prepareBoardHypotheses(ctx, sessionID, question, brief, summaries, err, func(ctx context.Context, ids []uint) ([]repository.AnalysisMethod, error) {
		return o.repo.GetAnalysisMethodsByIDs(ctx, ids)
	})
}
