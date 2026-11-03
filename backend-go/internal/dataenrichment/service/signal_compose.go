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

// 引用占位符：[[data:c1:o2]] / [[calc:k1]] / [[news:31]] / [[source:n1|w1]]
// （source = question-v1 已读文字证据，独立命名空间，不参与数值计算/图表）。
var signalRefTokenRe = regexp.MustCompile(`\[\[(data|calc|news|source):([^\]]+)\]\]`)

// ── payload 形状（sectors jsonb，design §7）──────────────────────────────────

// SignalReportSection 是报告的一节。schema2：thesis/facts/causal 只有 text；
// implication 额外携带五个结构化字段（非空校验 + direction 枚举，SV-2）。
// schema3（redesign-enrichment-research-workbench D4）：text 为代码生成的兼容
// 四段文本（旧 reader 可读），模型产出存 Blocks；implication 条件字段不变。
type SignalReportSection struct {
	Kind string `json:"kind"`
	Text string `json:"text"`
	// Blocks 是 schema3 的受控块序列（paragraph|evidence_table|chart，扁平
	// 不嵌套）；schema2 载荷无此字段（旧行不回填）。
	Blocks []SignalReportBlock `json:"blocks,omitempty"`

	Verdict          string `json:"verdict,omitempty"`
	Direction        string `json:"direction,omitempty"` // up|down|diverge|conditional
	Horizon          string `json:"horizon,omitempty"`
	TriggerCondition string `json:"trigger_condition,omitempty"`
	SelfDoubt        string `json:"self_doubt,omitempty"`
}

// schema3 受控块的三种平面类型（design D4：白名单直接分支，不设计插件注册、
// 任意嵌套或新的 DSL 引擎）。
const (
	SignalBlockParagraph     = "paragraph"
	SignalBlockEvidenceTable = "evidence_table"
	SignalBlockChart         = "chart"
)

// SignalReportEvidenceRow is one evidence-table row: a label plus same-unit
// observation/calculation refs — values are rendered by the reader from the
// appendix, the model never copies them into the label.
type SignalReportEvidenceRow struct {
	Label string   `json:"label"`
	Refs  []string `json:"refs"`
}

// SignalReportBlock is one controlled block. Unknown types/fields are
// rejected at validation (回注)， not silently dropped.
type SignalReportBlock struct {
	Type    string                    `json:"type"`
	Text    string                    `json:"text,omitempty"`
	Title   string                    `json:"title,omitempty"`
	Rows    []SignalReportEvidenceRow `json:"rows,omitempty"`
	ChartID string                    `json:"chart_id,omitempty"`
}

// SignalReportDataFreshness is the code-generated per-source freshness
// statement (schema3 only; design §10.3 新口径：系统机械计算分源时效，模型不再
// 被要求重复全局「数据截至」句；写了期值仍校验不得编造).
type SignalReportDataFreshness struct {
	Sources         []SignalReportSourceFreshness `json:"sources"`
	Cutoff          string                        `json:"cutoff"`
	GeneratedAt     string                        `json:"generated_at"`
	HasObservations bool                          `json:"has_observations"`
}

// SignalReportSourceFreshness is one tool's mechanically computed latest
// available period (empty latest = no observations from that tool).
type SignalReportSourceFreshness struct {
	Tool   string `json:"tool"`
	Latest string `json:"latest,omitempty"`
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
	// Sources/TextCalls 是 question-v1 增量：已读文字证据与检索/读取流水
	// （旧 schema2/schema3 载荷无此字段，缺省兼容读取）。
	Sources   []*signalResearchSourceRecord `json:"sources,omitempty"`
	TextCalls []*signalTextCallRecord       `json:"text_calls,omitempty"`
}

// SignalReportAnswer 是 question-v1 的答题范围（D6）：模型基于本次证据给出
// 的回答程度声明，经结构校验——不是系统认证的真实性，也不是 job 状态。
// explained=窄范围内已解释；partial=主问题有未答部分；unresolved=尚不能判断。
type SignalReportAnswer struct {
	Status     string   `json:"status"`
	Reason     string   `json:"reason"`
	Unanswered []string `json:"unanswered,omitempty"`
}

// answer 状态枚举与字段预算（D6：reason ≤400rune；unanswered ≤5 项、每项
// ≤300rune，计入 3000 可见字符总量）。
const (
	SignalAnswerExplained  = "explained"
	SignalAnswerPartial    = "partial"
	SignalAnswerUnresolved = "unresolved"
)

// SignalReportPayload is the immutable sectors payload for kind=signal_report.
type SignalReportPayload struct {
	SchemaVersion  int                        `json:"schema_version"`
	SignalSnapshot json.RawMessage            `json:"signal_snapshot"`
	Report         SignalReportBody           `json:"report"`
	Appendix       SignalReportAppendix       `json:"appendix"`
	GenerationMeta SignalReportGenerationMeta `json:"generation_meta"`
	// DataFreshness 是 schema3 的机械分源时效（schema2 载荷无此字段）。
	DataFreshness *SignalReportDataFreshness `json:"data_freshness,omitempty"`
	// ResearchContract/Answer 是 question-v1 增量（旧 schema2/schema3 缺省可
	// 读，不把展示 schema 当研究协议）。
	ResearchContract string              `json:"research_contract,omitempty"`
	Answer           *SignalReportAnswer `json:"answer,omitempty"`
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
			Messages:    []airouter.Message{{Role: "system", Content: signalComposeSystemPromptOf(ledgerView.contract)}, {Role: "user", Content: userMsg}},
			Temperature: floatPtr(0.2),
			JSONMode:    true,
		})
		if err != nil {
			lastChatErr = fmt.Errorf("compose chat attempt %d: %w", attempt, err)
			continue
		}
		draft, perr := parseSignalComposeDraft(resp.Content)
		var normalized [][]signalComposeBlock
		if perr == nil {
			normalized, perr = validateSignalReportDraft(draft, ledgerView)
		}
		if perr == nil {
			payload, berr := buildSignalReportPayload(in, draft, normalized, ledgerView, attempt)
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
- 系统会机械计算并排展示各源时效与研究时点，你不必在正文写「数据截至」句；如果你确实写到某个数据期，期值必须原样取自用户消息「数据可用性」行（不得编造或推断更新的期间）；零观测时只能写「数据截至：无可用数据期」。

结构纪律（受控块，schema3）：
- 每段的正文放在 blocks 里，只有三种平面块，不嵌套、不自造类型、不加额外字段：
  {"type":"paragraph","text":"…（引用占位符表达数据）"}
  {"type":"evidence_table","title":"关键观测","rows":[{"label":"同口径观测组","refs":["c1:o1","c1:o2"]}]}
  {"type":"chart","chart_id":"ch1"}
- thesis 与 implication 的 blocks 只放一个 paragraph；证据表和 chart 引用块只能出现在 facts/causal。
- 证据表同一行的 refs 必须同单位同口径（不暗示跨源可算价差）；行 label 不写数值，数值由系统从账本渲染。
- chart 块只能挂已定义的 chart_id，每张图最多挂一处；图表本身仍在 charts 数组里声明。
- 未知块类型/未知字段会被整稿拒绝。

图表纪律：
- 0~3 张图，每张只讲一个论点（claim）；没有可画的数据就 0 张，不要硬凑；正文要用到哪张图，就在 facts/causal 里放对应 chart 块。
- kind=line：同一系列的多个期间，系统会按时间排序；缺失期间保持断线，不补零。
- kind=comparison：同源同单位的一组值的对比。
- refs 只能填观测 id 或已成功计算的 id；折线至少两个非空点。

篇幅：可见标题+正文+图题合计少于 3000 个非空白字符（数字英文都算），超过会被拒。

输出严格 JSON（不要 markdown 包裹、不要任何其他内容）：
{"title":"判断式标题","sections":[
 {"kind":"thesis","blocks":[{"type":"paragraph","text":"先说结论（引用占位符表达数据）"}]},
 {"kind":"facts","blocks":[
   {"type":"paragraph","text":"发生了什么（引用占位符表达数据）"},
   {"type":"evidence_table","title":"关键观测","rows":[{"label":"同口径观测组","refs":["c1:o1","c1:o2"]}]},
   {"type":"chart","chart_id":"ch1"}
 ]},
 {"kind":"causal","blocks":[{"type":"paragraph","text":"这意味着什么：机制与竞争解释"}]},
 {"kind":"implication","blocks":[{"type":"paragraph","text":"接下来怎么看（含自我质疑）"}],"verdict":"一句话判断","direction":"up|down|diverge|conditional","horizon":"看多久","trigger_condition":"什么信号会出现说明判断错了/对了","self_doubt":"我怎么知道自己可能错了"}
],"charts":[{"chart_id":"ch1","kind":"line","claim":"这张图说明什么","refs":["c1:o1","c1:o2"]}]}

sections 四段必须齐、按上面顺序、各出现一次；每段必有 blocks；implication 的五个结构化字段放在段上（不放块里）。`

// isForeignCandidateGap 报告一行冻结缺口是否属于「其它候选」：freeze gap 由
// 代码生成、前缀形状稳定（候选「<signal>：…）；批次快照共享，本候选报告与
// prompt 不携带他候选的截断/超期缺口（E1 真实样本发现的跨候选噪音）。
func isForeignCandidateGap(reason, candidateSignal string) bool {
	const prefix = "候选「"
	if !strings.HasPrefix(reason, prefix) {
		return false
	}
	end := strings.Index(reason, "」：")
	if end < 0 {
		return false
	}
	return reason[len(prefix):end] != candidateSignal
}

// signalComposeAnswerPrompt 是 question-v1 专用增补（旧协议 system prompt 逐字
// 不变）：E1 真实样本发现模型无从得知 answer 契约——要么把 unanswered 写成字
// 符串，要么被拒后干脆丢字段。显式给出 JSON 形状与类型，缺 answer 整稿拒收。
const signalComposeAnswerPrompt = `

答题范围（本次必填，缺 answer 字段整稿拒收）：
- 顶层必须带 answer 字段，形状如下（unanswered 是字符串数组，不是字符串）：
{"answer":{"status":"explained|partial|unresolved","reason":"一句话说明对研究问题的回答程度与依据边界（不超过400字符）","unanswered":["还没回答的部分（每条不超过300字符）"]}}
- status 三选一：explained=问题已解释清楚；partial=只答了一部分；unresolved=暂无法判断。
- explained 时 unanswered 可为空数组 []；partial/unresolved 必填 1~5 条。
- answer 说的是「回答了研究问题的多少」，不是可信度、不是任务状态；没取得证据就如实写未取得，不把「没取到」写成「没有影响」。

引用纪律补充（本次适用）：
- 引用已读文字证据用 [[source:n1]] / [[source:w1]] 占位，由系统渲染为可核查来源片段；只能用研究笔记/已读文字证据清单里列出的 ID，搜索摘要不是正文证据、不可引用。`

// signalComposeSystemPromptOf 按协议返回 system prompt：question-v1 追加答题
// 范围纪律；旧协议（空 contract）返回原样，行为零变化。
func signalComposeSystemPromptOf(contract string) string {
	if contract == SignalResearchContractQuestionV1 {
		return signalComposeSystemPrompt + signalComposeAnswerPrompt
	}
	return signalComposeSystemPrompt
}

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
	// sourcesByID 是 question-v1 的可引用文字证据索引（已读本地 nN 与
	// 时间合格的外部 wN；搜索摘要永不进入）。
	sourcesByID map[string]*signalResearchSourceRecord
	// contract 是发现时冻结的研究协议（question-v1 / 空=旧协议）：answer 与
	// source 引用只在 question-v1 下要求/放行。
	contract string
	// sourceLatest 是机械计算的数据可用性（各源最新可得期），注入行与期值
	// 校验共用同一份结果，避免两处口径漂移（review M1）。
	sourceLatest []signalSourceLatest
}

func buildSignalLedgerView(in SignalComposeInput) *signalLedgerView {
	view := &signalLedgerView{
		obsByID:     in.Ledger.obsIndex,
		calcByID:    map[string]*SignalCalculationResult{},
		newsRefs:    map[string]bool{},
		sourcesByID: in.Ledger.sourceIndex(),
	}
	if in.Material != nil {
		view.contract = in.Material.ResearchContract
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
			if isForeignCandidateGap(g.Reason, in.Candidate.Signal) {
				continue
			}
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
	if len(view.sourcesByID) > 0 {
		sb.WriteString("\n已读文字证据（question 协议：可用 [[source:ID]] 引用，不能进 calculate/图表）：\n")
		for _, rec := range view.sourcesByID {
			meta := rec.Title
			if rec.URL != "" {
				meta += " " + rec.URL
			}
			if rec.PublishedAt != nil {
				meta += " 发布:" + rec.PublishedAt.Format("2006-01-02")
			} else {
				meta += " 发布:未知"
			}
			at := ""
			if rec.FetchedAt != nil {
				at = " 取回:" + rec.FetchedAt.UTC().Format(time.RFC3339)
			} else if rec.CapturedAt != nil {
				at = " 冻结:" + rec.CapturedAt.UTC().Format(time.RFC3339)
			}
			fmt.Fprintf(&sb, "- %s | %s |%s | 片段%d字\n", rec.ID, meta, at, len([]rune(rec.Text)))
		}
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

// ── question-v1 答题范围与 source 引用校验（D6）───────────────────────────────

// validateSignalAnswerShape 校验 answer 形状：status 枚举、reason 非空
// ≤400rune、unanswered ≤5 项且每项非空 ≤300rune（partial/unresolved 必须
// 非空 unanswered——主问题未答部分要写明）。
func validateSignalAnswerShape(a *SignalReportAnswer) error {
	if a == nil {
		return fmt.Errorf("answer 缺失")
	}
	switch a.Status {
	case SignalAnswerExplained, SignalAnswerPartial, SignalAnswerUnresolved:
	default:
		return fmt.Errorf("status 必须是 explained|partial|unresolved，收到 %q", a.Status)
	}
	reason := strings.TrimSpace(a.Reason)
	if reason == "" {
		return fmt.Errorf("reason 不能为空")
	}
	if len([]rune(reason)) > 400 {
		return fmt.Errorf("reason 长度 %d 超过上限 400 字符", len([]rune(reason)))
	}
	if (a.Status == SignalAnswerPartial || a.Status == SignalAnswerUnresolved) && len(a.Unanswered) == 0 {
		return fmt.Errorf("%s 的 unanswered 必须非空（列出未答部分）", a.Status)
	}
	if len(a.Unanswered) > 5 {
		return fmt.Errorf("unanswered 最多 5 项，收到 %d 项", len(a.Unanswered))
	}
	for i, u := range a.Unanswered {
		t := strings.TrimSpace(u)
		if t == "" {
			return fmt.Errorf("unanswered[%d] 不能为空", i)
		}
		if len([]rune(t)) > 300 {
			return fmt.Errorf("unanswered[%d] 长度 %d 超过上限 300 字符", i, len([]rune(t)))
		}
	}
	return nil
}

// validateSignalAnswerEvidence 校验 answer 与证据的一致性：explained 必须有
// 至少一条成功读取的文字证据或有效数值依据（结构校验，非真实性认证）；
// partial/unresolved 允许只有缺口。
func validateSignalAnswerEvidence(a *SignalReportAnswer, hasEvidence bool) error {
	if a == nil {
		return fmt.Errorf("answer 缺失")
	}
	if a.Status == SignalAnswerExplained && !hasEvidence {
		return fmt.Errorf("explained 需要至少一条成功读取的文字证据或有效数值依据；没有证据请改用 unresolved 并说明缺口")
	}
	return nil
}

// hasAnyEvidence 报告本次研究是否存在任何可引用证据（文字来源或数值观测）。
func (v *signalLedgerView) hasAnyEvidence() bool {
	return len(v.sourcesByID) > 0 || len(v.obsByID) > 0
}

// draftAnswerOrNil 把草稿 answer 映射为载荷字段（仅 question-v1；旧协议
// 载荷不带 answer，即使模型画蛇添足也不保存）。
func draftAnswerOrNil(draft *signalComposeDraft, view *signalLedgerView) *SignalReportAnswer {
	if view.contract != SignalResearchContractQuestionV1 || draft.Answer == nil {
		return nil
	}
	return &SignalReportAnswer{Status: draft.Answer.Status, Reason: draft.Answer.Reason, Unanswered: draft.Answer.Unanswered}
}

// ── draft 解析与校验（SV-1..SV-7）────────────────────────────────────────────

// signalComposeAnswerDraft 是 answer 字段的原始 JSON（question-v1 必填；
// 结构校验经 validateSignalAnswerShape/Evidence）。
type signalComposeAnswerDraft struct {
	Status     string   `json:"status"`
	Reason     string   `json:"reason"`
	Unanswered []string `json:"unanswered"`
}

type signalComposeSectionDraft struct {
	Kind             string `json:"kind"`
	Text             string `json:"text"`
	Verdict          string `json:"verdict"`
	Direction        string `json:"direction"`
	Horizon          string `json:"horizon"`
	TriggerCondition string `json:"trigger_condition"`
	SelfDoubt        string `json:"self_doubt"`
	// Blocks 是 schema3 受控块的原始 JSON；解析后严格校验（未知类型/字段
	// 拒绝）。缺省时 text 归一为单 paragraph（兼容旧稿形状，不再另发 schema2）。
	Blocks []json.RawMessage `json:"blocks"`
}

type signalComposeChartDraft struct {
	ChartID string   `json:"chart_id"`
	Kind    string   `json:"kind"`
	Claim   string   `json:"claim"`
	Refs    []string `json:"refs"`
}

// signalComposeBlock is one strictly-decoded draft block (design D4：三种平
// 面类型的白名单分支).
type signalComposeBlock struct {
	Type    string
	Text    string
	Title   string
	Rows    []SignalReportEvidenceRow
	ChartID string
}

// decodeSignalComposeBlock strictly decodes one raw block: unknown type or
// any unknown field → error（回注聚合，不静默丢弃）.
func decodeSignalComposeBlock(raw json.RawMessage) (*signalComposeBlock, error) {
	var probe struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(raw, &probe); err != nil {
		return nil, fmt.Errorf("块不是对象")
	}
	var allowedKeys []string
	switch probe.Type {
	case SignalBlockParagraph:
		allowedKeys = []string{"type", "text"}
	case SignalBlockEvidenceTable:
		allowedKeys = []string{"type", "title", "rows"}
	case SignalBlockChart:
		allowedKeys = []string{"type", "chart_id"}
	default:
		return nil, fmt.Errorf("未知块类型 %q（只接受 paragraph|evidence_table|chart）", probe.Type)
	}
	var keys map[string]json.RawMessage
	if err := json.Unmarshal(raw, &keys); err != nil {
		return nil, fmt.Errorf("块不可解析")
	}
	for k := range keys {
		known := false
		for _, f := range allowedKeys {
			if k == f {
				known = true
				break
			}
		}
		if !known {
			return nil, fmt.Errorf("%s 块含未知字段 %q", probe.Type, k)
		}
	}
	switch probe.Type {
	case SignalBlockParagraph:
		var b struct {
			Text string `json:"text"`
		}
		if err := json.Unmarshal(raw, &b); err != nil {
			return nil, fmt.Errorf("paragraph.text 必须是字符串")
		}
		return &signalComposeBlock{Type: probe.Type, Text: b.Text}, nil
	case SignalBlockEvidenceTable:
		var envelope struct {
			Title string            `json:"title"`
			Rows  []json.RawMessage `json:"rows"`
		}
		if err := json.Unmarshal(raw, &envelope); err != nil {
			return nil, fmt.Errorf("evidence_table 形状非法")
		}
		block := &signalComposeBlock{Type: probe.Type, Title: envelope.Title}
		for _, rawRow := range envelope.Rows {
			var keys map[string]json.RawMessage
			if err := json.Unmarshal(rawRow, &keys); err != nil {
				return nil, fmt.Errorf("行不是对象")
			}
			for k := range keys {
				if k != "label" && k != "refs" {
					return nil, fmt.Errorf("行含未知字段 %q", k)
				}
			}
			var row SignalReportEvidenceRow
			if err := json.Unmarshal(rawRow, &row); err != nil {
				return nil, fmt.Errorf("行形状非法（label/refs）")
			}
			block.Rows = append(block.Rows, row)
		}
		return block, nil
	default: // chart
		var b struct {
			ChartID string `json:"chart_id"`
		}
		if err := json.Unmarshal(raw, &b); err != nil {
			return nil, fmt.Errorf("chart.chart_id 必须是字符串")
		}
		return &signalComposeBlock{Type: probe.Type, ChartID: b.ChartID}, nil
	}
}

// normalizeSignalComposeSections strictly decodes every section's blocks;
// text-only sections (schema2-era shape) normalize to a single paragraph —
// new runs still persist as schema3. Decode errors are collected via add.
func normalizeSignalComposeSections(draft *signalComposeDraft, add func(string, ...any)) [][]signalComposeBlock {
	normalized := make([][]signalComposeBlock, len(draft.Sections))
	for i, s := range draft.Sections {
		var blocks []signalComposeBlock
		if len(s.Blocks) == 0 {
			if strings.TrimSpace(s.Text) == "" {
				add("sections[%d]（%s）的 text/blocks 不能同时为空", i, s.Kind)
				continue
			}
			blocks = []signalComposeBlock{{Type: SignalBlockParagraph, Text: s.Text}}
		} else {
			for j, raw := range s.Blocks {
				b, derr := decodeSignalComposeBlock(raw)
				if derr != nil {
					add("sections[%d]（%s）blocks[%d]：%v", i, s.Kind, j, derr)
					continue
				}
				blocks = append(blocks, *b)
			}
		}
		normalized[i] = blocks
	}
	return normalized
}

type signalComposeDraft struct {
	Title    string                      `json:"title"`
	Sections []signalComposeSectionDraft `json:"sections"`
	Charts   []signalComposeChartDraft   `json:"charts"`
	// Answer 是 question-v1 的答题范围（旧协议缺省）。
	Answer *signalComposeAnswerDraft `json:"answer"`
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
// tasks 4.9；review M1 治编造期值；add-fast-frequency-datasources 日度适配）：
// 期值必须原样取自机械计算的各源最新期（signalPeriodRank vs
// view.sourceLatest）——日度期值（含日分量）按日粒度精确匹配日度最新期，月
// 度/年度期值保持月粒度匹配月度源（跨粒度缩写/编造一律拒）；零观测账本唯一
// 合法值是「无可用数据期」。失败走既有 problems 聚合回注通道，反馈点名「期
// 值必须取自数据可用性行/不得编造」——不用模型自造期值蒙过字样检查。
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
	key, ok := signalPeriodRank(token)
	if !ok {
		add("facts 段「数据截至 %s」不是可识别的数据期：期值必须原样取自用户消息「数据可用性」行的各源最新期，不得编造", token)
		return
	}
	// 分形匹配（add-fast-frequency-datasources）：token 含日分量（日度期值，如
	// yahoo/gdelt/fred 的 YYYY-MM-DD）→ 按日粒度精确匹配可用性行里的日度最新
	// 期；token 月度/年度 → 月粒度匹配非日度源（日度源最新期的缩写月不算原样
	// 引用，拒绝——防跨粒度编造与日度缩写）。
	if key%32 > 0 {
		for _, s := range view.sourceLatest {
			if s.Latest == "" {
				continue
			}
			if rk, ok := signalPeriodRank(s.Latest); ok && rk == key {
				return
			}
		}
		add("facts 段「数据截至 %s」不在数据可用性行的各源最新期内（含日度期值时按日粒度精确匹配，日度最新期不得缩写为月）：期值必须原样取自「数据可用性」行，不得编造或推断更新的期间", token)
		return
	}
	monthKey := key / 32
	allowed := make(map[int]bool, len(view.sourceLatest))
	for _, s := range view.sourceLatest {
		if s.Latest == "" {
			continue
		}
		if rk, ok := signalPeriodRank(s.Latest); ok && rk%32 == 0 {
			allowed[rk/32] = true
		}
	}
	if !allowed[monthKey] {
		add("facts 段「数据截至 %s」不在数据可用性行的各源最新期内（月粒度比较；可用性行含日度期值时日度期须原样写全 YYYY-MM-DD）：期值必须原样取自「数据可用性」行，不得编造或推断更新的期间", token)
	}
}

// validateSignalReportDraft enforces the full §6 contract（schema3 受控块）and
// returns the normalized blocks plus one aggregate feedback message（回注重试
// 用）。零错误 → nil。
func validateSignalReportDraft(draft *signalComposeDraft, view *signalLedgerView) ([][]signalComposeBlock, error) {
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
	normalized := normalizeSignalComposeSections(draft, add)

	// 受控块校验（schema3 / D4）：放置、形状、引用、口径、复制值、图挂载。
	mountedCharts := map[string]bool{}
	ledgerValues := signalLedgerValueStrings(view)
	for i, s := range draft.Sections {
		blocks := normalized[i]
		if i < len(signalReportSectionOrder) && len(blocks) == 0 && s.Kind == signalReportSectionOrder[i] {
			// 已由归一化分支报过空段问题，不重复报。
			continue
		}
		paragraphCount := 0
		var factsParagraphs []string
		for j, b := range blocks {
			switch b.Type {
			case SignalBlockParagraph:
				paragraphCount++
				if strings.TrimSpace(b.Text) == "" {
					add("sections[%d]（%s）blocks[%d] 的 text 不能为空", i, s.Kind, j)
				}
				if s.Kind == "facts" {
					factsParagraphs = append(factsParagraphs, b.Text)
				}
			case SignalBlockEvidenceTable:
				if s.Kind == "thesis" || s.Kind == "implication" {
					add("sections[%d]（%s）不能放证据表（thesis/implication 只用段落；表置于 facts/causal）", i, s.Kind)
				}
				if strings.TrimSpace(b.Title) == "" {
					add("sections[%d]（%s）blocks[%d] 证据表缺 title", i, s.Kind, j)
				}
				if len(b.Rows) == 0 {
					add("sections[%d]（%s）blocks[%d] 证据表不能没有行", i, s.Kind, j)
					continue
				}
				for k, row := range b.Rows {
					if strings.TrimSpace(row.Label) == "" {
						add("sections[%d]（%s）blocks[%d] 第 %d 行缺 label", i, s.Kind, j, k+1)
					}
					if signalLabelCopiesLedgerValue(row.Label, ledgerValues) {
						add("sections[%d]（%s）blocks[%d] 第 %d 行 label 疑似复制了账本数值；表格只放 label 与引用 ID，数值由系统从账本渲染", i, s.Kind, j, k+1)
					}
					if len(row.Refs) == 0 {
						add("sections[%d]（%s）blocks[%d] 第 %d 行 refs 不能为空", i, s.Kind, j, k+1)
						continue
					}
					unitSet := map[string]bool{}
					for _, ref := range row.Refs {
						if strings.HasPrefix(ref, "news") || strings.Contains(ref, ":") && strings.HasPrefix(ref, "news") {
							add("sections[%d]（%s）blocks[%d] 第 %d 行引用了新闻切片 %s；表格只收观测/计算 id", i, s.Kind, j, k+1, ref)
							continue
						}
						if err := resolveSignalRef(signalRefToken{kind: refKindOf(ref), id: ref}, view); err != nil {
							add("sections[%d]（%s）blocks[%d] 第 %d 行引用 %s 无法解析：%v", i, s.Kind, j, k+1, ref, err)
							continue
						}
						_, unit, _, _, _, attrErr := signalRefChartAttributes(ref, view)
						if attrErr != nil {
							continue // 已由 resolveSignalRef 报过
						}
						unitSet[unit] = true
					}
					if len(unitSet) > 1 {
						add("sections[%d]（%s）blocks[%d] 第 %d 行混用了多个单位 %v；同一行并列观测须同口径，不暗示跨源可算价差", i, s.Kind, j, k+1, keysOfSet(unitSet))
					}
				}
			case SignalBlockChart:
				if s.Kind == "thesis" || s.Kind == "implication" {
					add("sections[%d]（%s）不能挂图表块（thesis/implication 只用段落；图置于 facts/causal）", i, s.Kind)
				}
				if b.ChartID == "" {
					add("sections[%d]（%s）blocks[%d] chart 块缺 chart_id", i, s.Kind, j)
					continue
				}
				if !signalChartDefined(draft, b.ChartID) {
					add("sections[%d]（%s）blocks[%d] 挂载了未定义的 chart %q；图表必须先在 charts 数组里声明", i, s.Kind, j, b.ChartID)
					continue
				}
				if mountedCharts[b.ChartID] {
					add("sections[%d]（%s）blocks[%d] 重复挂载 chart %q；每张图最多挂一处", i, s.Kind, j, b.ChartID)
					continue
				}
				mountedCharts[b.ChartID] = true
			}
		}
		if s.Kind == "thesis" || s.Kind == "implication" {
			if paragraphCount != 1 || len(blocks) != 1 {
				add("sections[%d]（%s）的 blocks 必须恰好一个 paragraph", i, s.Kind)
			}
		}
		// 数据时效声明（schema3 新口径）：不强制「数据截至」短语（系统已机械
		// 生成分源时效）；若模型自己写了，期值仍不得编造。
		if s.Kind == "facts" && strings.Contains(strings.Join(factsParagraphs, "\n"), "数据截至") {
			validateSignalFactsAsOf(strings.Join(factsParagraphs, "\n"), view, add)
		}
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

	// question-v1 答题范围（D6）：answer 必填、形状与证据门校验；reason/
	// unanswered 计入可见字符总量（与四段共用 3000 预算）。
	var answer *SignalReportAnswer
	if view.contract == SignalResearchContractQuestionV1 {
		if draft.Answer == nil {
			add("缺 answer 字段（question-v1 必填：status=explained|partial|unresolved、reason、unanswered）")
		} else {
			answer = &SignalReportAnswer{Status: draft.Answer.Status, Reason: draft.Answer.Reason, Unanswered: draft.Answer.Unanswered}
			if err := validateSignalAnswerShape(answer); err != nil {
				add("answer 不合格：%v", err)
				answer = nil
			} else if err := validateSignalAnswerEvidence(answer, view.hasAnyEvidence()); err != nil {
				add("answer 不合格：%v", err)
				answer = nil
			}
		}
	}

	// 篇幅（SV-3）：标题 + 段落文本 + 证据表标题/行标签 + 图题 + 后果段结构化
	// 字段的非空白 rune 总数。兼容 text 不重复计数（它在校验后由代码生成）；
	// 表格 refs/附录/UI 元信息不计。answer 可见文案同预算。
	visible := countNonSpaceRunes(draft.Title)
	if answer != nil {
		visible += countNonSpaceRunes(answer.Reason)
		for _, u := range answer.Unanswered {
			visible += countNonSpaceRunes(u)
		}
	}
	for _, blocks := range normalized {
		for _, b := range blocks {
			switch b.Type {
			case SignalBlockParagraph:
				visible += countNonSpaceRunes(b.Text)
			case SignalBlockEvidenceTable:
				visible += countNonSpaceRunes(b.Title)
				for _, row := range b.Rows {
					visible += countNonSpaceRunes(row.Label)
				}
			}
		}
	}
	for _, s := range draft.Sections {
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

	// 引用可解析（SV-4）：标题与段落文本中的全部占位符（表格 refs 已单独校验）。
	refs := collectSignalRefs(draft.Title)
	for _, blocks := range normalized {
		for _, b := range blocks {
			if b.Type == SignalBlockParagraph {
				refs = append(refs, collectSignalRefs(b.Text)...)
			}
		}
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
		return normalized, nil
	}
	return nil, fmt.Errorf("报告不合格：%s", strings.Join(problems, "；"))
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
	case "source":
		if _, ok := view.sourcesByID[ref.id]; !ok {
			return fmt.Errorf("文字来源 %s 不在本次已读证据内（搜索摘要/未读来源/跨 run 引用不可用）", ref.id)
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

// signalLedgerValueStrings 收集本次账本的可渲染数值字符串（观测格式化值 +
// 成功计算值），供「复制值检测」用；短值（<3 rune）不收，避免 "1"/"42" 这类
// 年份/序号误伤。
func signalLedgerValueStrings(view *signalLedgerView) []string {
	out := make([]string, 0, len(view.obsByID)+len(view.calcByID))
	for _, obs := range view.obsByID {
		if obs == nil || obs.Value == nil {
			continue
		}
		if s := trimSignalFloat(*obs.Value); countNonSpaceRunes(s) >= 3 {
			out = append(out, s)
		}
	}
	for _, calc := range view.calcByID {
		if calc.Status != SignalCalcStatusOK || calc.Value == "" {
			continue
		}
		if countNonSpaceRunes(calc.Value) >= 3 {
			out = append(out, calc.Value)
		}
	}
	return out
}

// signalLabelCopiesLedgerValue reports whether label contains a ledger value
// verbatim（块内引用必须核查：模型不得把数值复制造表格 label）。
func signalLabelCopiesLedgerValue(label string, values []string) bool {
	for _, v := range values {
		if strings.Contains(label, v) {
			return true
		}
	}
	return false
}

// refKindOf 判定表格 refs 的引用类型：k 前缀（无冒号）= 计算，其余 = 观测。
func refKindOf(ref string) string {
	if strings.HasPrefix(ref, "k") && !strings.Contains(ref, ":") {
		return "calc"
	}
	return "data"
}

// signalChartDefined reports whether chartID is declared in the draft's
// charts array（挂载前必须先定义）。
func signalChartDefined(draft *signalComposeDraft, chartID string) bool {
	for _, ch := range draft.Charts {
		if ch.ChartID == chartID {
			return true
		}
	}
	return false
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

// signalSectionCompatText generates the schema3 section's reader-compatible
// four-段 text: paragraphs verbatim, tables rendered as labelled ref lines
// (placeholders intact so old readers still resolve values from the
// appendix), charts as a mount anchor. Compat text is NOT part of the model
// budget (design D4).
func signalSectionCompatText(blocks []signalComposeBlock) string {
	parts := make([]string, 0, len(blocks))
	for _, b := range blocks {
		switch b.Type {
		case SignalBlockParagraph:
			parts = append(parts, b.Text)
		case SignalBlockEvidenceTable:
			lines := make([]string, 0, len(b.Rows)+1)
			lines = append(lines, "【证据表】"+b.Title)
			for _, row := range b.Rows {
				tokens := make([]string, 0, len(row.Refs))
				for _, ref := range row.Refs {
					tokens = append(tokens, "[["+refKindOf(ref)+":"+ref+"]]")
				}
				lines = append(lines, "- "+row.Label+"："+strings.Join(tokens, "、"))
			}
			parts = append(parts, strings.Join(lines, "\n"))
		case SignalBlockChart:
			parts = append(parts, "【图】"+b.ChartID)
		}
	}
	return strings.Join(parts, "\n\n")
}

// signalReportDataFreshness mechanically derives the schema3 freshness
// statement from the compose view (same source as the prompt's 可用性行).
func signalReportDataFreshness(in SignalComposeInput, view *signalLedgerView) *SignalReportDataFreshness {
	sources := make([]SignalReportSourceFreshness, 0, len(view.sourceLatest))
	for _, s := range view.sourceLatest {
		sources = append(sources, SignalReportSourceFreshness(s))
	}
	return &SignalReportDataFreshness{
		Sources:         sources,
		Cutoff:          in.Cutoff.UTC().Format(time.RFC3339),
		GeneratedAt:     time.Now().UTC().Format(time.RFC3339),
		HasObservations: len(view.obsByID) > 0,
	}
}

func buildSignalReportPayload(in SignalComposeInput, draft *signalComposeDraft, normalized [][]signalComposeBlock, view *signalLedgerView, attempt int) (json.RawMessage, error) {
	sections := make([]SignalReportSection, 0, len(draft.Sections))
	for i, s := range draft.Sections {
		var blocks []SignalReportBlock
		if i < len(normalized) {
			for _, b := range normalized[i] {
				blocks = append(blocks, SignalReportBlock{
					Type: b.Type, Text: b.Text, Title: b.Title,
					Rows:    append([]SignalReportEvidenceRow(nil), b.Rows...),
					ChartID: b.ChartID,
				})
			}
		}
		sections = append(sections, SignalReportSection{
			Kind:             s.Kind,
			Text:             signalSectionCompatText(normalized[i]),
			Blocks:           blocks,
			Verdict:          s.Verdict,
			Direction:        s.Direction,
			Horizon:          s.Horizon,
			TriggerCondition: s.TriggerCondition,
			SelfDoubt:        s.SelfDoubt,
		})
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
			if isForeignCandidateGap(g.Reason, in.Candidate.Signal) {
				continue
			}
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
		SchemaVersion:  3,
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
			Sources:      in.Ledger.sources,
			TextCalls:    in.Ledger.textCalls,
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
		DataFreshness: signalReportDataFreshness(in, view),
	}
	if in.Material != nil {
		payload.ResearchContract = in.Material.ResearchContract
	}
	payload.Answer = draftAnswerOrNil(draft, view)
	b, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal payload: %w", err)
	}
	return b, nil
}
