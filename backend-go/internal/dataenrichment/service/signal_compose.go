package service

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"syntopica-backend/internal/dataenrichment/repository"
	"syntopica-backend/internal/platform/airouter"
)

// ── 成文（board-signal-reports design §6，tasks 4.1）──────────────────────────
//
// LLM 输出 {title, sections, charts}；代码负责：四段唯一序与 implication 结构
// 化枚举校验、引用可解析性（悬空/失败计算引用拒绝）、可见篇幅 <3000 非空白
// 字符、图表兼容性（同源同单位/折线≥2 非空点/null 断线不补零/禁混频）、数据
// 时效声明（机械注入各源最新可得期+研究时点，facts 段必须出现「数据截至」，
// design §10.3）。
// appendix 由代码从研究账本生成——LLM 不得生成/覆写；数值渲染由引用占位符
// + 附录数据完成（前端按附录渲染，模型永远不提供展示数值）。
//
// 有界重试：不合格回注重试，attempts≤3（初次+2 重试），retries=attempts-1；
// 耗尽 → error（job failed，无不合格 result 落库）。

// 首版批准的 direction 枚举（design §6）。
var signalReportDirections = map[string]bool{
	"up": true, "down": true, "diverge": true, "conditional": true,
}

// 图表 kind 枚举。
const (
	SignalChartKindLine       = "line"
	SignalChartKindComparison = "comparison"
)

// 四段唯一序（design §6）。
var signalReportSectionOrder = []string{"thesis", "facts", "causal", "implication"}

// signalReportMaxVisibleRunes 是可见标题/正文/图题的非空白 Unicode 字符上限
// （数字英文均计；附录/UI 元信息不计）。2999 通过、3000 拒绝（SV-3）。
const signalReportMaxVisibleRunes = 3000

const signalComposeMaxAttempts = 3

// 引用占位符：[[data:c1:o2]] / [[calc:k1]] / [[news:31]]。
var signalRefTokenRe = regexp.MustCompile(`\[\[(data|calc|news):([^\]]+)\]\]`)

// ── payload 形状（sectors jsonb，design §7）──────────────────────────────────

// SignalReportSection 是报告的一节。thesis/facts/causal 只有 text；
// implication 额外携带五个结构化字段（非空校验 + direction 枚举，SV-2）。
type SignalReportSection struct {
	Kind string `json:"kind"`
	Text string `json:"text"`

	Verdict          string `json:"verdict,omitempty"`
	Direction        string `json:"direction,omitempty"` // up|down|diverge|conditional
	Horizon          string `json:"horizon,omitempty"`
	TriggerCondition string `json:"trigger_condition,omitempty"`
	SelfDoubt        string `json:"self_doubt,omitempty"`
}

// SignalReportChart 存 LLM 声明的形状 {chart_id,kind,claim,refs}；refs 由代码
// 校验并（折线）按期间升序排序后落库。每个 ref 的「原值/代码计算」可由 ref
// 形态区分（cN:oM=观测、kN=计算），数值由前端从附录渲染（FE-8）。
type SignalReportChart struct {
	ChartID string   `json:"chart_id"`
	Kind    string   `json:"kind"`
	Claim   string   `json:"claim"`
	Refs    []string `json:"refs"`
}

type SignalReportBody struct {
	Title    string                `json:"title"`
	Sections []SignalReportSection `json:"sections"`
	Charts   []SignalReportChart   `json:"charts"`
}

type SignalReportGenerationMeta struct {
	SessionID        string `json:"session_id"`
	Attempts         int    `json:"attempts"`
	Retries          int    `json:"retries"`
	Decisions        int    `json:"decisions"`
	SourceCalls      int    `json:"source_calls"`
	CalculationCalls int    `json:"calculation_calls"`
	StopReason       string `json:"stop_reason"`
	AnalysisMode     string `json:"analysis_mode"`
	Cutoff           string `json:"cutoff"`
}

type SignalReportAppendix struct {
	Calls        []*signalResearchCallRecord `json:"calls"`
	Calculations []*SignalCalculationResult  `json:"calculations"`
	Gaps         []signalResearchGap         `json:"gaps"`
}

// SignalReportPayload is the immutable sectors payload for kind=signal_report.
type SignalReportPayload struct {
	SchemaVersion  int                        `json:"schema_version"`
	SignalSnapshot json.RawMessage            `json:"signal_snapshot"`
	Report         SignalReportBody           `json:"report"`
	Appendix       SignalReportAppendix       `json:"appendix"`
	GenerationMeta SignalReportGenerationMeta `json:"generation_meta"`
}

// ── 输入与编排 ────────────────────────────────────────────────────────────────

type SignalComposeInput struct {
	SessionID    string
	Candidate    *repository.BoardSignalCandidate
	Material     *SignalMaterial
	Ledger       *signalResearchLedger
	Loop         *AgentLoopResult
	StopReason   string
	Decisions    int
	AnalysisMode string
	Cutoff       time.Time
}

// SignalComposer runs the bounded compose step（同 session、独立 operation）。
type SignalComposer struct {
	router     AirRouter
	capability airouter.Capability
}

// Compose renders the final report payload: ≤3 validated LLM attempts, then
// error.返回值是完整 sectors payload JSON（含代码生成的 appendix 与
// generation_meta）。失败时优先返回最后一次【校验】错误（实质的内容问题），
// 仅在从未得到有效响应时返回 chat 错误——否则传输层噪声会掩盖真实拒因。
func (c *SignalComposer) Compose(ctx context.Context, in SignalComposeInput) (json.RawMessage, error) {
	ledgerView := buildSignalLedgerView(in)
	userMsg := assembleSignalComposePrompt(in, ledgerView, "")
	var lastChatErr error
	var lastValidationErr error
	for attempt := 1; attempt <= signalComposeMaxAttempts; attempt++ {
		resp, err := c.router.Chat(ctx, airouter.ChatRequest{
			Capability:  c.capability,
			Operation:   signalComposeOperation,
			SessionID:   in.SessionID,
			Messages:    []airouter.Message{{Role: "system", Content: signalComposeSystemPrompt}, {Role: "user", Content: userMsg}},
			Temperature: floatPtr(0.2),
			JSONMode:    true,
		})
		if err != nil {
			lastChatErr = fmt.Errorf("compose chat attempt %d: %w", attempt, err)
			continue
		}
		draft, perr := parseSignalComposeDraft(resp.Content)
		if perr == nil {
			perr = validateSignalReportDraft(draft, ledgerView)
		}
		if perr == nil {
			payload, berr := buildSignalReportPayload(in, draft, attempt)
			if berr != nil {
				lastChatErr = berr
				continue
			}
			return payload, nil
		}
		lastValidationErr = fmt.Errorf("compose attempt %d: %w", attempt, perr)
		// 回注：把失败原因带给下一稿。
		userMsg = assembleSignalComposePrompt(in, ledgerView, perr.Error())
	}
	if lastValidationErr != nil {
		return nil, lastValidationErr
	}
	return nil, lastChatErr
}

// ── LLM 契约 ─────────────────────────────────────────────────────────────────

const signalComposeSystemPrompt = `你是一位给普通读者写解读的分析编辑。你要把研究员的数据研究笔记写成一篇大白话解读报告：先说结论、再讲发生了什么、然后说这意味着什么、最后讲接下来怎么看。

风格纪律（硬性）：
- 大白话：不用「去库/转多/补库」这类行话替代解释；字段名和公式只出现在引用与附录，不写进正文。
- 报告是观点，数据只是观点里引用的证据：两三个关键数字嵌进论证句即可，不堆数据、不写数据面板。
- 结论必须落在证据上；证据不足时明确说「目前证据还不够，先维持原判断」（direction 用 conditional），不硬做方向判断。
- 因果段老实摆出竞争解释：哪个更站得住、哪些还不能排除。
- 后果段融进一两句自我质疑（我怎么知道自己可能是错的），不设独立的风险/证伪小节。
- 标题即判断（不是「关于X的分析」）。

引用纪律：
- 正文里的每个数据量都用引用占位符表达，由系统渲染成数值和原生单位：观测写 [[data:c1:o2]]，代码计算写 [[calc:k1]]。
- 禁止自己在正文里另写统计数值（含「大约」）：除引用渲染产生的数字外，正文不出现手写数据量。
- 新闻背景引用写 [[news:切片ID]]，只能用候选依据清单里的 ID。
- 每个图表的 claim 里如出现数据量，同样只能用引用占位符。

时效纪律：
- facts 段必须出现「数据截至」四个字并给出数据最新期：从用户消息「数据可用性」行取各源最新可得期中的最新者原样写入（如「数据截至 2026-07」）；零观测（「数据可用性」行注明本研究无可用数据观测）时写「数据截至：无可用数据期」。
- 「数据截至」后的期间必须原样取自「数据可用性」行，不得自己编造或推断更新的期间。

图表纪律：
- 0~3 张图，每张只讲一个论点（claim）；没有可画的数据就 0 张，不要硬凑。
- kind=line：同一系列的多个期间，系统会按时间排序；缺失期间保持断线，不补零。
- kind=comparison：同源同单位的一组值的对比。
- refs 只能填观测 id 或已成功计算的 id；折线至少两个非空点。

篇幅：可见标题+正文+图题合计少于 3000 个非空白字符（数字英文都算），超过会被拒。

输出严格 JSON（不要 markdown 包裹、不要任何其他内容）：
{"title":"判断式标题","sections":[
 {"kind":"thesis","text":"先说结论（引用占位符表达数据）"},
 {"kind":"facts","text":"发生了什么（引用占位符表达数据）"},
 {"kind":"causal","text":"这意味着什么：机制与竞争解释"},
 {"kind":"implication","text":"接下来怎么看（含自我质疑）","verdict":"一句话判断","direction":"up|down|diverge|conditional","horizon":"看多久","trigger_condition":"什么信号会出现说明判断错了/对了","self_doubt":"我怎么知道自己可能错了"}
],"charts":[{"chart_id":"ch1","kind":"line","claim":"这张图说明什么","refs":["c1:o1","c1:o2"]}]}

sections 四段必须齐、按上面顺序、各出现一次。`

// assembleSignalComposePrompt builds the compose user message; prevError 非空
// 时作为回注反馈附在末尾（有界重试的修正输入）。
func assembleSignalComposePrompt(in SignalComposeInput, view *signalLedgerView, prevError string) string {
	var sb strings.Builder
	c := in.Candidate
	fmt.Fprintf(&sb, "候选信号：%s\n研究问题：%s\n新闻依据切片ID（news 引用白名单）：%s\n\n", c.Signal, c.ResearchQuestion, strings.Join(repository.SignalEvidenceRefs(c.EvidenceRefs), "、"))
	sb.WriteString("研究收束状态：" + in.StopReason)
	if in.StopReason == SignalStopReasonBudgetExhausted {
		sb.WriteString("（预算耗尽：正文必须在相应段落如实披露数据缺口，不得宣称研究完整）")
	}
	sb.WriteString("\n\n研究笔记（研究员的收束总结）：\n" + strings.TrimSpace(in.Loop.FinalData) + "\n\n")
	sb.WriteString(signalDataAvailabilityLine(in, view) + "\n\n")
	sb.WriteString(view.userText)
	if prevError != "" {
		sb.WriteString("\n\n上一稿不合格，原因：" + prevError + "\n请修正后重新输出完整 JSON（四段齐全、引用可解析、篇幅合规）。")
	}
	return sb.String()
}

// ── 数据时效声明（design §10.3，tasks 4.9）────────────────────────────────────

// signalPeriodRank 把混杂 period 形态（YYYY-MM-DD / YYYY-MM / YYYY / YYYYMM /
// YYYY/MM / YYYY年M月[日]）规范化为可比较序数（(年*12+月)*32+日：月为主序、
// 日破同月平局，EIA 周度期同月内也能取到真正最大）；不可解析返回 false。
// 同源内取最新必须先规范化——直接字符串比较会把 "2026-07" 排在 "202601" 之后。
// 中文/斜杠写法是为 facts 期值容忍新增的（review M1），账本既有形态行为不变。
func signalPeriodRank(period string) (int, bool) {
	p := strings.TrimSpace(period)
	p = strings.NewReplacer("年", "-", "月", "-", "日", "", "/", "-").Replace(p)
	p = strings.Trim(p, "-")
	if p == "" {
		return 0, false
	}
	parse := func(s string) (int, bool) {
		v, err := strconv.Atoi(s)
		if err != nil {
			return 0, false
		}
		return v, true
	}
	var year, month, day int
	if !strings.Contains(p, "-") {
		switch len(p) {
		case 4: // YYYY
			y, ok := parse(p)
			if !ok {
				return 0, false
			}
			year, month = y, 1
		case 6: // YYYYMM
			y, ok1 := parse(p[:4])
			m, ok2 := parse(p[4:])
			if !ok1 || !ok2 {
				return 0, false
			}
			year, month = y, m
		default:
			return 0, false
		}
	} else {
		parts := strings.Split(p, "-")
		if len(parts) < 2 || len(parts) > 3 {
			return 0, false
		}
		y, ok := parse(parts[0])
		if !ok {
			return 0, false
		}
		year = y
		m, ok := parse(parts[1])
		if !ok {
			return 0, false
		}
		month = m
		if len(parts) == 3 {
			d, ok := parse(parts[2])
			if !ok {
				return 0, false
			}
			day = d
		}
	}
	if year <= 0 || month < 1 || month > 12 || day < 0 || day > 31 {
		return 0, false
	}
	return (year*12+month)*32 + day, true
}

// signalPeriodMonthKey 把期值归一到月粒度：signalPeriodRank 的日分量严格小于
// 32，整除 32 即 (年*12+月)。「2026-07-31」与「2026-07」同月视为一致；年度
// 期值（如 WDI 的 2024）归一为 2024-01（review M1 月粒度比较口径）。
func signalPeriodMonthKey(period string) (int, bool) {
	rank, ok := signalPeriodRank(period)
	if !ok {
		return 0, false
	}
	return rank / 32, true
}

// signalSourceLatest 是一个源（工具）的机械计算最新可得期。
type signalSourceLatest struct {
	Tool   string
	Latest string // 空 = 该源在账本内无观测
}

// signalSourceLatestPeriods 按源聚合账本观测的最新可得期。源集合 = 账本调用
// 出现过的工具 ∪ 观测索引里的工具——取数失败/零覆盖的源也如实列出「无观测」。
// 工具名升序保证注入行稳定可测。
func signalSourceLatestPeriods(in SignalComposeInput, view *signalLedgerView) []signalSourceLatest {
	tools := map[string]bool{}
	for _, call := range in.Ledger.calls {
		tools[call.Tool] = true
	}
	maxRank := map[string]int{}
	latest := map[string]string{}
	for _, obs := range view.obsByID {
		if obs == nil {
			continue
		}
		tools[obs.Tool] = true
		rank, ok := signalPeriodRank(obs.Period)
		if !ok {
			continue
		}
		if cur, seen := maxRank[obs.Tool]; !seen || rank > cur {
			maxRank[obs.Tool] = rank
			latest[obs.Tool] = obs.Period
		}
	}
	out := make([]signalSourceLatest, 0, len(tools))
	for tool := range tools {
		out = append(out, signalSourceLatest{Tool: tool, Latest: latest[tool]})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Tool < out[j].Tool })
	return out
}

// signalDataAvailabilityLine 机械计算的数据时效声明（design §10.3）：各源最新
// 可得期 + 研究时点；账本一个观测都没有时如实写「本研究无可用数据观测」。
func signalDataAvailabilityLine(in SignalComposeInput, view *signalLedgerView) string {
	cutoffDate := in.Cutoff.UTC().Format("2006-01-02")
	if len(view.obsByID) == 0 {
		return "数据可用性：本研究无可用数据观测。研究时点 " + cutoffDate + "（晚于此的数据已被剔除）。"
	}
	parts := make([]string, 0, 4)
	for _, s := range view.sourceLatest {
		if s.Latest == "" {
			parts = append(parts, s.Tool+" 无观测")
		} else {
			parts = append(parts, s.Tool+" 最新 "+s.Latest)
		}
	}
	return "数据可用性：" + strings.Join(parts, "；") + "。研究时点 " + cutoffDate + "（晚于此的数据已被剔除）。"
}

// signalLedgerView 是 compose 视角的账本投影：给 LLM 的可读清单 + 校验用的
// 索引。LLM 只看到这个视图——它无法引用视图之外的观测或计算。
type signalLedgerView struct {
	obsByID  map[string]*SignalCalcObservation
	calcByID map[string]*SignalCalculationResult
	newsRefs map[string]bool
	userText string
	// sourceLatest 是机械计算的数据可用性（各源最新可得期），注入行与期值
	// 校验共用同一份结果，避免两处口径漂移（review M1）。
	sourceLatest []signalSourceLatest
}

func buildSignalLedgerView(in SignalComposeInput) *signalLedgerView {
	view := &signalLedgerView{
		obsByID:  in.Ledger.obsIndex,
		calcByID: map[string]*SignalCalculationResult{},
		newsRefs: map[string]bool{},
	}
	for _, ref := range repository.SignalEvidenceRefs(in.Candidate.EvidenceRefs) {
		view.newsRefs[ref] = true
	}
	var obsLines, calcLines, gapLines []string
	for _, call := range in.Ledger.calls {
		head := fmt.Sprintf("- [%s] %s（%s）状态=%s", call.CallID, call.Tool, call.Question, call.Status)
		if call.Error != "" {
			head += " 错误=" + call.Error
		}
		obsLines = append(obsLines, head)
		for _, obs := range call.Observations {
			id, _ := obs["observation_id"].(string)
			if id == "" {
				continue
			}
			obsLines = append(obsLines, "    "+signalObservationLine(id, obs))
		}
	}
	for _, calc := range in.Ledger.calcs {
		view.calcByID[calc.CalcID] = calc
		line := fmt.Sprintf("- %s | %s | 状态=%s", calc.CalcID, calc.Expression, calc.Status)
		if calc.Status == SignalCalcStatusOK {
			line += fmt.Sprintf(" = %s %s", calc.Value, calc.Unit)
		}
		if calc.Reason != "" {
			line += "（" + calc.Reason + "）"
		}
		calcLines = append(calcLines, line)
	}
	for _, g := range in.Ledger.gaps {
		gapLines = append(gapLines, "- "+g.Reason)
	}
	if in.Material != nil {
		for _, g := range in.Material.Gaps {
			gapLines = append(gapLines, "- "+g.Reason)
		}
	}

	var sb strings.Builder
	sb.WriteString("已取得的观测（引用宇宙；value=null 表示缺失，不得当 0 使用）：\n")
	if len(obsLines) == 0 {
		sb.WriteString("（无——本次研究没有取得任何观测）\n")
	} else {
		sb.WriteString(strings.Join(obsLines, "\n") + "\n")
	}
	sb.WriteString("\n代码计算（只能引用状态=ok 的；missing/rejected 不可引用）：\n")
	if len(calcLines) == 0 {
		sb.WriteString("（无）\n")
	} else {
		sb.WriteString(strings.Join(calcLines, "\n") + "\n")
	}
	sb.WriteString("\n数据缺口（预算耗尽或证据不足时，正文须如实披露）：\n")
	if len(gapLines) == 0 {
		sb.WriteString("（无）\n")
	} else {
		sb.WriteString(strings.Join(gapLines, "\n") + "\n")
	}
	view.sourceLatest = signalSourceLatestPeriods(in, view)
	view.userText = sb.String()
	return view
}

func signalObservationLine(id string, obs map[string]any) string {
	series := signalObservationDisplaySeries(obs)
	period, _ := obs["period"].(string)
	unit, _ := obs["unit"].(string)
	if v, ok := obs["value"].(float64); ok {
		return fmt.Sprintf("%s | %s | %s | %s | %s", id, series, period, trimSignalFloat(v), unit)
	}
	raw, _ := obs["raw_value"].(string)
	if raw == "" {
		raw = "null"
	}
	return fmt.Sprintf("%s | %s | %s | 缺失(%s)", id, series, period, raw)
}

func signalObservationDisplaySeries(obs map[string]any) string {
	if label, ok := obs["label"].(string); ok && label != "" {
		return label
	}
	parts := make([]string, 0, 4)
	for _, k := range []string{"geo", "flow", "product", "indicator_id", "country_iso3", "cmd_code"} {
		if v, ok := obs[k].(string); ok && v != "" {
			parts = append(parts, v)
		}
	}
	return strings.Join(parts, "/")
}

func trimSignalFloat(v float64) string {
	return formatSignalDecimal(bigRatFromFloat(v))
}

// ── draft 解析与校验（SV-1..SV-7）────────────────────────────────────────────

type signalComposeSectionDraft struct {
	Kind             string `json:"kind"`
	Text             string `json:"text"`
	Verdict          string `json:"verdict"`
	Direction        string `json:"direction"`
	Horizon          string `json:"horizon"`
	TriggerCondition string `json:"trigger_condition"`
	SelfDoubt        string `json:"self_doubt"`
}

type signalComposeChartDraft struct {
	ChartID string   `json:"chart_id"`
	Kind    string   `json:"kind"`
	Claim   string   `json:"claim"`
	Refs    []string `json:"refs"`
}

type signalComposeDraft struct {
	Title    string                      `json:"title"`
	Sections []signalComposeSectionDraft `json:"sections"`
	Charts   []signalComposeChartDraft   `json:"charts"`
}

func parseSignalComposeDraft(content string) (*signalComposeDraft, error) {
	parsed, err := ParseJSONResponse(content)
	if err != nil {
		return nil, fmt.Errorf("parse: %w", err)
	}
	b, err := json.Marshal(parsed)
	if err != nil {
		return nil, fmt.Errorf("re-marshal: %w", err)
	}
	var draft signalComposeDraft
	if err := json.Unmarshal(b, &draft); err != nil {
		return nil, fmt.Errorf("shape: %w", err)
	}
	return &draft, nil
}

// signalAsOfPeriodTokenRe 匹配「数据截至」后的期值写法：YYYY-MM-DD /
// YYYY-MM / YYYY/MM / YYYY年M月[日] / YYYYMM / YYYY（裸年）。长形态在前，
// 避免 "2026-07-15" 被截成 "2026-07"。
var signalAsOfPeriodTokenRe = regexp.MustCompile(`\d{4}[年/-]\d{1,2}(?:[月/-]\d{1,2})?日?|\d{6}|\d{4}`)

// validateSignalFactsAsOf 校验 facts 段的「数据截至」声明（design §10.3，
// tasks 4.9；review M1 治编造期值）：期值必须月粒度等于机械计算的各源最新期
// 之一（signalPeriodMonthKey vs view.sourceLatest）；零观测账本唯一合法值是
// 「无可用数据期」。失败走既有 problems 聚合回注通道，反馈点名「期值必须取自
// 数据可用性行/不得编造」——不用模型自造期值蒙过字样检查。
func validateSignalFactsAsOf(text string, view *signalLedgerView, add func(string, ...any)) {
	idx := strings.Index(text, "数据截至")
	if idx < 0 {
		add("facts 段缺「数据截至」数据时效声明：必须写明数据最新期（从用户消息「数据可用性」行取各源最新期原样引用；零观测写「数据截至：无可用数据期」）")
		return
	}
	tail := text[idx+len("数据截至"):]
	token := signalAsOfPeriodTokenRe.FindString(tail)
	if len(view.obsByID) == 0 {
		// 零观测：唯一合法值是「无可用数据期」，任何期值都是编造。
		if token != "" {
			add("facts 段「数据截至」写了「%s」，但本次研究无可用数据观测：期值只能写「无可用数据期」，必须取自用户消息「数据可用性」行，不得编造", token)
			return
		}
		if !strings.Contains(tail, "无可用数据期") {
			add("facts 段「数据截至」后缺少「无可用数据期」：本次研究无可用数据观测，期值必须如实写「无可用数据期」，不得编造")
		}
		return
	}
	if strings.Contains(tail, "无可用数据期") {
		add("facts 段「数据截至」写了「无可用数据期」，但本次研究有可用观测：期值必须原样取自用户消息「数据可用性」行的各源最新期，不得编造")
		return
	}
	if token == "" {
		add("facts 段「数据截至」后缺少可识别的数据期（如 2026-07）：期值必须原样取自用户消息「数据可用性」行，不得编造")
		return
	}
	key, ok := signalPeriodMonthKey(token)
	if !ok {
		add("facts 段「数据截至 %s」不是可识别的数据期：期值必须原样取自用户消息「数据可用性」行，不得编造", token)
		return
	}
	allowed := make(map[int]bool, len(view.sourceLatest))
	for _, s := range view.sourceLatest {
		if s.Latest == "" {
			continue
		}
		if k, ok := signalPeriodMonthKey(s.Latest); ok {
			allowed[k] = true
		}
	}
	if !allowed[key] {
		add("facts 段「数据截至 %s」不在数据可用性行的各源最新期内（月粒度比较）：期值必须原样取自「数据可用性」行，不得编造或推断更新的期间", token)
	}
}

// validateSignalReportDraft enforces the full §6 contract and returns one
// aggregate feedback message（回注重试用）。零错误 → nil。
func validateSignalReportDraft(draft *signalComposeDraft, view *signalLedgerView) error {
	var problems []string
	add := func(format string, args ...any) {
		problems = append(problems, fmt.Sprintf(format, args...))
	}

	if strings.TrimSpace(draft.Title) == "" {
		add("title 不能为空")
	}

	// 四段唯一序（SV-1）。
	if len(draft.Sections) != len(signalReportSectionOrder) {
		add("sections 必须恰好四段（thesis/facts/causal/implication），收到 %d 段", len(draft.Sections))
	} else {
		for i, want := range signalReportSectionOrder {
			got := draft.Sections[i].Kind
			if got != want {
				add("sections[%d] 必须是 %s，收到 %q（四段必须按序唯一）", i, want, got)
			}
		}
	}
	for i, s := range draft.Sections {
		if strings.TrimSpace(s.Text) == "" {
			add("sections[%d]（%s）的 text 不能为空", i, s.Kind)
		}
	}
	// 数据时效声明（design §10.3，tasks 4.9；review M1）：facts 段必须出现
	// 「数据截至」，且所写期值必须月粒度等于各源最新期之一——编造/不可解析
	// 期值一律回注重试。段缺失本身已由四段唯一序校验兜住，不重复报。
	for _, s := range draft.Sections {
		if s.Kind != "facts" {
			continue
		}
		validateSignalFactsAsOf(s.Text, view, add)
		break
	}
	// implication 结构化字段（SV-2）。
	for _, s := range draft.Sections {
		if s.Kind != "implication" {
			continue
		}
		for field, v := range map[string]string{
			"verdict": s.Verdict, "direction": s.Direction, "horizon": s.Horizon,
			"trigger_condition": s.TriggerCondition, "self_doubt": s.SelfDoubt,
		} {
			if strings.TrimSpace(v) == "" {
				add("implication 的 %s 不能为空", field)
			}
		}
		if s.Direction != "" && !signalReportDirections[s.Direction] {
			add("implication.direction 必须是 up|down|diverge|conditional，收到 %q", s.Direction)
		}
	}

	// 篇幅（SV-3）：标题+正文+图题+后果段结构化字段的非空白 rune 总数。
	// 结构化字段（触发条件/自我质疑等）在前端是可见句，计入预算；附录/UI
	// 元信息不计。
	visible := countNonSpaceRunes(draft.Title)
	for _, s := range draft.Sections {
		visible += countNonSpaceRunes(s.Text)
		if s.Kind == "implication" {
			for _, v := range []string{s.Verdict, s.Direction, s.Horizon, s.TriggerCondition, s.SelfDoubt} {
				visible += countNonSpaceRunes(v)
			}
		}
	}
	for _, ch := range draft.Charts {
		visible += countNonSpaceRunes(ch.Claim)
	}
	if visible >= signalReportMaxVisibleRunes {
		add("可见标题/正文/图题合计 %d 个非空白字符，达到上限 %d；请压缩后重交", visible, signalReportMaxVisibleRunes)
	}

	// 引用可解析（SV-4）：正文/标题/图题中的全部占位符。
	refs := collectSignalRefs(draft.Title)
	for _, s := range draft.Sections {
		refs = append(refs, collectSignalRefs(s.Text)...)
	}
	for _, ref := range refs {
		if err := resolveSignalRef(ref, view); err != nil {
			add("引用 [[%s:%s]] 无法解析：%v", ref.kind, ref.id, err)
		}
	}

	// 图表（SV-5）。
	if len(draft.Charts) > 3 {
		add("charts 最多 3 张，收到 %d 张", len(draft.Charts))
	}
	seenChartIDs := map[string]bool{}
	for i, ch := range draft.Charts {
		if strings.TrimSpace(ch.ChartID) == "" {
			add("charts[%d] 缺 chart_id", i)
		} else if seenChartIDs[ch.ChartID] {
			add("charts[%d] 的 chart_id %q 重复", i, ch.ChartID)
		}
		seenChartIDs[ch.ChartID] = true
		if ch.Kind != SignalChartKindLine && ch.Kind != SignalChartKindComparison {
			add("charts[%d] kind 必须是 line|comparison，收到 %q", i, ch.Kind)
			continue
		}
		if strings.TrimSpace(ch.Claim) == "" {
			add("charts[%d]（%s）缺 claim", i, ch.ChartID)
		}
		for _, ref := range collectSignalRefs(ch.Claim) {
			if err := resolveSignalRef(ref, view); err != nil {
				add("charts[%d] claim 引用 [[%s:%s]] 无法解析：%v", i, ref.kind, ref.id, err)
			}
		}
		if len(ch.Refs) == 0 {
			add("charts[%d]（%s）refs 不能为空", i, ch.ChartID)
			continue
		}
		seen := map[string]bool{}
		seriesSet := map[string]bool{}
		unitSet := map[string]bool{}
		sourceSet := map[string]bool{}
		nonNull := 0
		var sortKeys []signalChartSortKey
		validRefs := true
		for _, ref := range ch.Refs {
			if seen[ref] {
				add("charts[%d]（%s）refs 重复引用 %s", i, ch.ChartID, ref)
				continue
			}
			seen[ref] = true
			series, unit, source, period, hasValue, rerr := signalRefChartAttributes(ref, view)
			if rerr != nil {
				add("charts[%d]（%s）refs 里的 %s 无法引用：%v", i, ch.ChartID, ref, rerr)
				validRefs = false
				continue
			}
			seriesSet[series] = true
			unitSet[unit] = true
			sourceSet[source] = true
			if hasValue {
				nonNull++
			} else if ch.Kind == SignalChartKindComparison {
				add("charts[%d]（%s）comparison 引用了缺失观测 %s（缺失值不能作对比柱）", i, ch.ChartID, ref)
			}
			sortKeys = append(sortKeys, signalChartSortKey{ref: ref, period: period, order: len(sortKeys)})
		}
		if !validRefs {
			continue
		}
		if len(sourceSet) > 1 {
			add("charts[%d]（%s）混用了多个数据源 %v；禁止跨源对比", i, ch.ChartID, keysOfSet(sourceSet))
		}
		if len(unitSet) > 1 {
			add("charts[%d]（%s）混用了多个单位 %v；禁止跨单位换算成图", i, ch.ChartID, keysOfSet(unitSet))
		}
		if ch.Kind == SignalChartKindLine {
			if len(seriesSet) > 1 {
				add("charts[%d]（%s）折线混连了多个系列 %v；折线只能画同一系列", i, ch.ChartID, keysOfSet(seriesSet))
			}
			if nonNull < 2 {
				add("charts[%d]（%s）折线至少需要两个非空点，收到 %d 个", i, ch.ChartID, nonNull)
			}
			// 折线按期间升序原地排序（null 断点保留，不补零）。
			sortSignalChartRefs(ch.Refs, sortKeys)
		}
	}

	if len(problems) == 0 {
		return nil
	}
	return fmt.Errorf("报告不合格：%s", strings.Join(problems, "；"))
}

type signalChartSortKey struct {
	ref    string
	period string
	order  int
}

// sortSignalChartRefs reorders refs in place by period ascending（稳定：同
// 期间保持原顺序）。
func sortSignalChartRefs(refs []string, keys []signalChartSortKey) {
	byRef := make(map[string]signalChartSortKey, len(keys))
	for _, k := range keys {
		byRef[k.ref] = k
	}
	sortable := append([]signalChartSortKey(nil), keys...)
	for i := 1; i < len(sortable); i++ {
		for j := i; j > 0 && sortable[j].period < sortable[j-1].period; j-- {
			sortable[j], sortable[j-1] = sortable[j-1], sortable[j]
		}
	}
	for idx, k := range sortable {
		if idx < len(refs) {
			refs[idx] = k.ref
		}
	}
	_ = byRef
}

// signalRefToken is one parsed citation placeholder.
type signalRefToken struct {
	kind string // data | calc | news
	id   string
}

func collectSignalRefs(text string) []signalRefToken {
	matches := signalRefTokenRe.FindAllStringSubmatch(text, -1)
	out := make([]signalRefToken, 0, len(matches))
	for _, m := range matches {
		out = append(out, signalRefToken{kind: m[1], id: strings.TrimSpace(m[2])})
	}
	return out
}

// resolveSignalRef verifies one placeholder resolves in this run's evidence
// universe（观测索引 / 成功计算 / 候选新闻白名单）。
func resolveSignalRef(ref signalRefToken, view *signalLedgerView) error {
	switch ref.kind {
	case "data":
		if _, ok := view.obsByID[ref.id]; !ok {
			return fmt.Errorf("观测 %s 不在本次已取得的观测中", ref.id)
		}
	case "calc":
		calc, ok := view.calcByID[ref.id]
		if !ok {
			return fmt.Errorf("计算 %s 不在本次已登记的计算中", ref.id)
		}
		if calc.Status != SignalCalcStatusOK {
			return fmt.Errorf("计算 %s 状态为 %s，不可引用", ref.id, calc.Status)
		}
	case "news":
		if !view.newsRefs[ref.id] {
			return fmt.Errorf("新闻切片 %s 不在候选依据白名单内", ref.id)
		}
	default:
		return fmt.Errorf("未知引用类型 %s", ref.kind)
	}
	return nil
}

// signalRefChartAttributes resolves the chart-compatibility attributes of one
// ref: series identity / unit / source tool / period / non-null-ness. 计算
// 点继承其输入观测的系列与源（计算本身已强制同源同系列）。
func signalRefChartAttributes(ref string, view *signalLedgerView) (series, unit, source, period string, hasValue bool, err error) {
	if strings.HasPrefix(ref, "k") {
		calc, ok := view.calcByID[ref]
		if !ok {
			return "", "", "", "", false, fmt.Errorf("计算 %s 不存在", ref)
		}
		if calc.Status != SignalCalcStatusOK {
			return "", "", "", "", false, fmt.Errorf("计算 %s 状态为 %s", ref, calc.Status)
		}
		if len(calc.Inputs) == 0 {
			return "", "", "", "", false, fmt.Errorf("计算 %s 没有输入观测", ref)
		}
		base, ok := view.obsByID[calc.Inputs[0]]
		if !ok {
			return "", "", "", "", false, fmt.Errorf("计算 %s 的输入 %s 不在观测索引", ref, calc.Inputs[0])
		}
		// 期间取输入观测中最大者（difference/percent_change 语义上的「截至」
		// 期；mean 为窗口末端）。
		maxPeriod := base.Period
		for _, in := range calc.Inputs[1:] {
			if o, ok := view.obsByID[in]; ok && o.Period > maxPeriod {
				maxPeriod = o.Period
			}
		}
		return base.SeriesID, calc.Unit, base.Tool, maxPeriod, true, nil
	}
	obs, ok := view.obsByID[ref]
	if !ok {
		return "", "", "", "", false, fmt.Errorf("观测 %s 不存在", ref)
	}
	hasValue = obs.Value != nil
	return obs.SeriesID, obs.Unit, obs.Tool, obs.Period, hasValue, nil
}

func keysOfSet(set map[string]bool) []string {
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	// 稳定输出便于测试断言。
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j] < out[j-1]; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}

func countNonSpaceRunes(s string) int {
	n := 0
	for _, r := range s {
		if !unicode.IsSpace(r) {
			n++
		}
	}
	return n
}

// ── payload 组装（appendix 代码生成；LLM 无_appendix 入口）────────────────────

func buildSignalReportPayload(in SignalComposeInput, draft *signalComposeDraft, attempt int) (json.RawMessage, error) {
	sections := make([]SignalReportSection, 0, len(draft.Sections))
	for _, s := range draft.Sections {
		sections = append(sections, SignalReportSection(s))
	}
	charts := make([]SignalReportChart, 0, len(draft.Charts))
	for _, ch := range draft.Charts {
		charts = append(charts, SignalReportChart{
			ChartID: ch.ChartID, Kind: ch.Kind, Claim: ch.Claim,
			Refs: append([]string(nil), ch.Refs...),
		})
	}

	gaps := make([]signalResearchGap, 0, len(in.Ledger.gaps)+4)
	if in.Material != nil {
		for _, g := range in.Material.Gaps {
			gaps = append(gaps, signalResearchGap{Reason: g.Reason})
		}
	}
	gaps = append(gaps, in.Ledger.gaps...)

	snapshot := map[string]any{
		"candidate_id":      in.Candidate.ID,
		"discovery_id":      in.Candidate.DiscoveryID,
		"semantic_board_id": in.Candidate.SemanticBoardID,
		"granularity":       in.Candidate.Granularity,
		"period":            in.Candidate.Period,
		"signal":            in.Candidate.Signal,
		"why_it_matters":    in.Candidate.WhyItMatters,
		"research_question": in.Candidate.ResearchQuestion,
		"evidence_refs":     repository.SignalEvidenceRefs(in.Candidate.EvidenceRefs),
		"score":             in.Candidate.Score,
		"rationale":         in.Candidate.Rationale,
		"analysis_mode":     in.AnalysisMode,
		"cutoff":            in.Cutoff.UTC().Format(time.RFC3339),
	}
	snapshotJSON, err := json.Marshal(snapshot)
	if err != nil {
		return nil, fmt.Errorf("marshal signal snapshot: %w", err)
	}

	payload := SignalReportPayload{
		SchemaVersion:  2,
		SignalSnapshot: snapshotJSON,
		Report: SignalReportBody{
			Title:    draft.Title,
			Sections: sections,
			Charts:   charts,
		},
		Appendix: SignalReportAppendix{
			Calls:        in.Ledger.calls,
			Calculations: in.Ledger.calcs,
			Gaps:         gaps,
		},
		GenerationMeta: SignalReportGenerationMeta{
			SessionID:        in.SessionID,
			Attempts:         attempt,
			Retries:          attempt - 1,
			Decisions:        in.Decisions,
			SourceCalls:      len(in.Ledger.calls),
			CalculationCalls: len(in.Ledger.calcs),
			StopReason:       in.StopReason,
			AnalysisMode:     in.AnalysisMode,
			Cutoff:           in.Cutoff.UTC().Format(time.RFC3339),
		},
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal payload: %w", err)
	}
	return b, nil
}
