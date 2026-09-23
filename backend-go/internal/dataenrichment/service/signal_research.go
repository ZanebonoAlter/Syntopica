package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"syntopica-backend/internal/dataenrichment/repository"
	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/airouter"
)

// ── 深入研究闭环（board-signal-reports design §4/§5/§6，tasks 3.3/3.4）────────
//
// 一次研究只服务一条候选信号：读取发现时冻结的 InputSnapshot（PC-5，不重新
// 装配材料、不偷换最新），围绕候选的 research_question 做问题驱动的数据研究。
//
// 复用而非复制 runToolLoop：
//   - maxLoops=40 经 toolLoopParams 传入（总决策 ≤40 轮）；
//   - 三防御（/no_think、ResultFull 完整不截断、dedupKeyFor 重复拦截）全部
//     来自共享循环本体；
//   - 研究 policy 实现 toolLoopPolicy（每次动作必带非空 question、四源白名
//     单校验、执行账本观察）；calculate 动作经 toolLoopActionRunner 钩子在
//     循环 default 分支处理——不 fork 循环，不新增通用 Registry 工具；
//   - 无覆盖早停反馈（tasks 3.8 / design §10.2）：policy 另实现可选
//     toolLoopFeedbackProvider，同源连续 3 次空手（kept=0 或源错误）→ 该源
//     反馈一次、四源全触发再终局提示一次；反馈是历史行，不是工具结果，
//     不新增 ToolCallRecord、不消耗轮次；
//   - 每轮至多一个动作，因此「总执行 ≤40」由 maxLoops 机械保证；非法/未授权
//     /重复动作不执行但计轮（policy 返回 blocked 记录，循环继续）。
//
// cutoff 过滤（design §2，OB-1/2）：外部工具结果由包装层按候选 cutoff 筛选
// 后才进 agent；完整原响应留在账本并写回 result.tool_calls（工具日志）；筛选
// 后观测完整进入附录（不截 50 点）。

// 研究阶段常量（prepare/detect 见 signal_detect.go；save 不是 phase）。
const (
	SignalStageResearch = "research"
	SignalStageCompose  = "compose"
)

// calculate 是 signal_research loop 的循环原生动作（经 toolLoopActionRunner），
// 不是 Registry 工具（design §5：不新增通用 Registry 工具）。
const signalResearchActionCalculate = "calculate"

// signalResearchToolNames 是研究 loop 的完整工具白名单：仅四源，不含
// web_search / 脚本 / 探索工具；旧流程 allowedTools 零变化（约束「工具面
// 不变量」唯一放开的口子，见 specs/research-data-sources MODIFIED）。
var signalResearchToolNames = []string{
	"eia_wpsr_table1",
	"jodi_oil_primary",
	"wb_wdi",
	"un_comtrade_trade",
}

func isSignalSourceTool(name string) bool {
	for _, n := range signalResearchToolNames {
		if n == name {
			return true
		}
	}
	return false
}

// 预算上限（design §4）：总决策 ≤40 轮；总执行 ≤40 次（取数+计算合计，每轮
// 至多一个动作故由轮上限机械保证）；不强迫跑满，提前结束合法。
const signalResearchMaxLoops = 40

// signalSourceNoCoverageStreak 是无覆盖早停反馈的触发阈值：同一源连续空手
// 该次数即注入一次「勿换参」反馈（tasks 3.8 / design §10.2，不改 40 轮上限
// 与计轮规则）。
const signalSourceNoCoverageStreak = 3

// GenerateSignalResearchSessionID mints the research session id — an
// independent session per research run（design §9：发现与研究各有独立
// session_id，通过 candidate 串联；编排类多次调用共享该 session）。
func GenerateSignalResearchSessionID(candidateID uint) string {
	return fmt.Sprintf("board_signal_report_%d_%s", candidateID, RandomHex(8))
}

// airouter operation 标签：研究与成文同 session、不同 operation（可重建一次
// 研究编排的全过程，ai-logging 规范）。
const (
	signalResearchOperation = "data_enrichment.signal_research"
	signalComposeOperation  = "data_enrichment.signal_compose"
)

// 停止原因（design §4）：finished=模型提前/正常收束；budget_exhausted=预算
// 耗尽——不是失败，以已有证据成文并披露缺口；取消/超时/循环异常终止不走
// 这两个值，而是直接 failed（不伪装正常完成）。
const (
	SignalStopReasonFinished        = "finished"
	SignalStopReasonBudgetExhausted = "budget_exhausted"
)

// ── cutoff 过滤（OB-1/2：筛选先于 agent，晚于 cutoff 的观测不得进入）────────

// signalObservationPeriodEnd extracts the instant at/after which the
// observation's period is fully over, in the business timezone. ok=false means
// the period could not be parsed — such observations are DROPPED (structure
// drift is never silently waved through into the agent).
//
// 统一规则：观测期结束时刻 ≤ cutoff 才保留。
//   - EIA 周度：period=周结日 YYYY-MM-DD，结束时刻=当日零点（业务时区）；
//     phase-1 契约「period < cutoff 即可」在本实现下的等价保守形式。
//   - JODI 月度：period=YYYY-MM，结束时刻=次月 1 日零点——目标周期的整月
//     数据在历史 cutoff（=周期终点）处恰好相等，必须保留。
//   - WDI 年度：year=YYYY，结束时刻=次年 1 月 1 日零点。
//   - Comtrade：A=YYYY 同年度；M=YYYYMM 同月度。
func signalObservationPeriodEnd(toolName string, obs map[string]any) (time.Time, bool) {
	shanghai := models.ShanghaiTZ
	switch toolName {
	case "eia_wpsr_table1":
		p, _ := obs["period"].(string)
		t, err := time.ParseInLocation("2006-01-02", strings.TrimSpace(p), shanghai)
		if err != nil {
			return time.Time{}, false
		}
		return t, true
	case "jodi_oil_primary":
		p, _ := obs["period"].(string)
		t, err := time.ParseInLocation("2006-01", strings.TrimSpace(p), shanghai)
		if err != nil {
			return time.Time{}, false
		}
		return t.AddDate(0, 1, 0), true
	case "wb_wdi":
		p, _ := obs["year"].(string)
		t, err := time.ParseInLocation("2006", strings.TrimSpace(p), shanghai)
		if err != nil {
			return time.Time{}, false
		}
		return t.AddDate(1, 0, 0), true
	case "un_comtrade_trade":
		p, _ := obs["period"].(string)
		freq, _ := obs["freq_code"].(string)
		p = strings.TrimSpace(p)
		switch freq {
		case "A":
			t, err := time.ParseInLocation("2006", p, shanghai)
			if err != nil {
				return time.Time{}, false
			}
			return t.AddDate(1, 0, 0), true
		case "M":
			t, err := time.ParseInLocation("200601", p, shanghai)
			if err != nil {
				return time.Time{}, false
			}
			return t.AddDate(0, 1, 0), true
		default:
			return time.Time{}, false
		}
	default:
		return time.Time{}, false
	}
}

// filterSignalToolResult applies the candidate cutoff to one tool result's
// observations BEFORE the result enters the agent:
//   - 晚于 cutoff 的观测整条剔除（不得作为截止前已实现数据）；
//   - 期间无法解析的观测剔除（零容忍，不做模糊放行）；
//   - 每条保留观测打上 observation_id（"c1:o2"），后续计算/成文引用只能用
//     它；
//   - 顶层附 filter_meta（cutoff/kept/dropped），periods 重算为保留期的升序
//     去重列表；
//   - 解析失败的响应（源错误帧）原样放行——错误语义由账本记录，不是观测。
//
// kept/dropped 供账本与 gap 判定使用。
func filterSignalToolResult(toolName, raw string, cutoff time.Time, callID string) (filtered string, kept, dropped int) {
	var payload map[string]any
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return raw, 0, 0
	}
	rawObs, ok := payload["observations"].([]any)
	if !ok {
		return raw, 0, 0
	}
	keptObs := make([]any, 0, len(rawObs))
	keptPeriods := make([]string, 0, len(rawObs))
	seenPeriods := map[string]bool{}
	droppedAfterCutoff, droppedUnparseable := 0, 0
	for _, o := range rawObs {
		obs, isMap := o.(map[string]any)
		if !isMap {
			droppedUnparseable++
			continue
		}
		end, ok := signalObservationPeriodEnd(toolName, obs)
		if !ok {
			droppedUnparseable++
			continue
		}
		if end.After(cutoff) {
			droppedAfterCutoff++
			continue
		}
		obs["observation_id"] = fmt.Sprintf("%s:o%d", callID, len(keptObs)+1)
		keptObs = append(keptObs, obs)
		if p, _ := obs["period"].(string); p != "" && !seenPeriods[p] {
			seenPeriods[p] = true
			keptPeriods = append(keptPeriods, p)
		}
	}
	sortKeptPeriods(keptPeriods)
	payload["observations"] = keptObs
	payload["filter_meta"] = map[string]any{
		"cutoff":               cutoff.UTC().Format(time.RFC3339),
		"kept":                 len(keptObs),
		"dropped":              droppedAfterCutoff + droppedUnparseable,
		"dropped_after_cutoff": droppedAfterCutoff,
		"dropped_unparseable":  droppedUnparseable,
	}
	if _, has := payload["periods"]; has {
		payload["periods"] = keptPeriods
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return raw, 0, 0
	}
	return string(b), len(keptObs), droppedAfterCutoff + droppedUnparseable
}

func sortKeptPeriods(periods []string) {
	// YYYY / YYYY-MM / YYYY-MM-DD / YYYYMM 在同源内字典序即时间序；观测按源
	// 升序进入，这里用插入排序保持稳定且不引入新依赖语义。
	for i := 1; i < len(periods); i++ {
		for j := i; j > 0 && periods[j] < periods[j-1]; j-- {
			periods[j], periods[j-1] = periods[j-1], periods[j]
		}
	}
}

// ── 研究账本（appendix calls/calculations/gaps + 观测索引的唯一事实源）───────

// signalResearchGap 是附录 gaps[] 的一行（工具层缺口；材料层缺口随快照继承）。
type signalResearchGap struct {
	CallID string `json:"call_id,omitempty"`
	Tool   string `json:"tool,omitempty"`
	Reason string `json:"reason"`
}

// signalResearchCallRecord 是附录 calls[] 的一行（design §6：
// call_id/question/tool/args/status/error/来源时间与 hash/完整筛选观测/
// filter_meta）。RawResponse（完整原响应）只写回 result.tool_calls 工具日志，
// 不进附录（附录载筛选后全集，工具日志载原响应——OB-2 双侧可重建）。
type signalResearchCallRecord struct {
	CallID       string           `json:"call_id"`
	Question     string           `json:"question"`
	Tool         string           `json:"tool"`
	Args         map[string]any   `json:"args"`
	Status       string           `json:"status"` // ok | error
	Error        string           `json:"error,omitempty"`
	RetrievedAt  string           `json:"retrieved_at,omitempty"`
	LastModified string           `json:"last_modified,omitempty"`
	SourceSHA256 string           `json:"source_sha256,omitempty"`
	Documents    any              `json:"documents,omitempty"`
	Observations []map[string]any `json:"observations"`
	FilterMeta   map[string]any   `json:"filter_meta,omitempty"`

	// rawResponse 是未筛选的完整原响应：只写回 result.tool_calls 工具日志
	// （OB-2），不进附录、不进 agent 历史。
	rawResponse string
}

// signalResearchLedger 累积一次研究运行的取数/计算/缺口账本与观测索引。
// 单 goroutine 读写（循环体 + 包装层同序执行），无需加锁。
type signalResearchLedger struct {
	cutoff   time.Time
	nextCall int
	nextCalc int
	calls    []*signalResearchCallRecord
	calcs    []*SignalCalculationResult
	gaps     []signalResearchGap
	obsIndex map[string]*SignalCalcObservation
}

func newSignalResearchLedger(cutoff time.Time) *signalResearchLedger {
	return &signalResearchLedger{
		cutoff:   cutoff,
		obsIndex: map[string]*SignalCalcObservation{},
	}
}

func (l *signalResearchLedger) beginCall(tool string) string {
	l.nextCall++
	return fmt.Sprintf("c%d", l.nextCall)
}

// completeCall 由包装层在真实执行后调用：登记原始响应、筛选统计与状态/gap。
func (l *signalResearchLedger) completeCall(callID, tool string, args map[string]any, raw, filtered string, kept, dropped int) {
	rec := &signalResearchCallRecord{
		CallID:       callID,
		Tool:         tool,
		Args:         args,
		Status:       "ok",
		Observations: []map[string]any{},
		rawResponse:  raw,
	}
	if errMsg := signalSourceErrorText(filtered); errMsg != "" {
		rec.Status = "error"
		rec.Error = errMsg
		l.gaps = append(l.gaps, signalResearchGap{CallID: callID, Tool: tool, Reason: "取数失败：" + errMsg})
	} else {
		var payload struct {
			RetrievedAt  string           `json:"retrieved_at"`
			LastModified string           `json:"last_modified"`
			SourceSHA256 string           `json:"source_sha256"`
			Documents    any              `json:"documents"`
			Observations []map[string]any `json:"observations"`
		}
		if err := json.Unmarshal([]byte(filtered), &payload); err == nil {
			rec.RetrievedAt = payload.RetrievedAt
			rec.LastModified = payload.LastModified
			rec.SourceSHA256 = payload.SourceSHA256
			rec.Documents = payload.Documents
			if payload.Observations != nil {
				rec.Observations = payload.Observations
			}
		}
		if kept == 0 {
			reason := "该调用在 cutoff 内无观测覆盖"
			if dropped > 0 {
				reason = "全部观测晚于 cutoff 或期间不可解析，cutoff 内无覆盖"
			}
			l.gaps = append(l.gaps, signalResearchGap{CallID: callID, Tool: tool, Reason: reason})
		}
	}
	rec.FilterMeta = map[string]any{"cutoff": l.cutoff.UTC().Format(time.RFC3339), "kept": kept, "dropped": dropped}
	l.calls = append(l.calls, rec)
}

// annotateCall 由 policy 在 ObserveCall（真实执行后）调用：补 question 并把
// 筛选后观测入索引（计算与成文的唯一合法引用宇宙）。
func (l *signalResearchLedger) annotateCall(tool, question string) {
	for i := len(l.calls) - 1; i >= 0; i-- {
		rec := l.calls[i]
		if rec.Tool != tool || rec.Question != "" {
			continue
		}
		rec.Question = question
		for _, obs := range rec.Observations {
			ref, _ := obs["observation_id"].(string)
			if ref == "" {
				continue
			}
			l.obsIndex[ref] = signalCalcObservationFromMap(ref, rec.CallID, tool, obs)
		}
		return
	}
}

// signalCalcObservationFromMap normalizes one source observation into the
// calculator's index entry（系列身份/单位/期间/流量维度按源定制；value 缺失
// 保留 nil，绝不转 0）。
func signalCalcObservationFromMap(ref, callID, tool string, obs map[string]any) *SignalCalcObservation {
	entry := &SignalCalcObservation{Ref: ref, CallID: callID, Tool: tool}
	entry.Period, _ = obs["period"].(string)
	entry.Value = optionalSignalFloat(obs["value"])
	switch tool {
	case "eia_wpsr_table1":
		label, _ := obs["label"].(string)
		flow, _ := obs["flow"].(string)
		unit, _ := obs["unit"].(string)
		entry.SeriesID = "eia|" + label
		entry.Flow = flow
		entry.Unit = unit
	case "jodi_oil_primary":
		geo, _ := obs["geo"].(string)
		flow, _ := obs["flow"].(string)
		product, _ := obs["product"].(string)
		unit, _ := obs["unit"].(string)
		entry.SeriesID = "jodi|" + geo + "|" + flow + "|" + product + "|" + unit
		entry.Flow = flow
		entry.Unit = unit
	case "wb_wdi":
		indicator, _ := obs["indicator_id"].(string)
		country, _ := obs["country_iso3"].(string)
		// WDI 期间键是 year（WDIObservation 无 period 字段）——不覆盖会使
		// difference/mean 因空期间被误拒、期间排序退化为输入原序。
		entry.Period, _ = obs["year"].(string)
		entry.SeriesID = "wdi|" + indicator + "|" + country
		// WDI 观测不带单位列（原生单位按指标定义）；同系列计算不受影响，
		// 跨系列计算由 series 判定拒绝。
		entry.Unit = ""
	default: // un_comtrade_trade
		reporter, _ := obs["reporter_code"].(float64)
		partner, _ := obs["partner_code"].(float64)
		cmd, _ := obs["cmd_code"].(string)
		flow, _ := obs["flow_code"].(string)
		entry.SeriesID = fmt.Sprintf("comtrade|%.0f|%.0f|%s|%s", reporter, partner, cmd, flow)
		entry.Flow = flow
		// Comtrade 观测行含 qty_kg/net_wgt_kg/value_usd 三种量；计算索引取
		// 贸易额 value_usd（原生单位 USD）为该行规范值，其余量随观测原样
		// 留在附录展示。
		entry.Unit = "USD"
		entry.Value = optionalSignalFloat(obs["value_usd"])
	}
	return entry
}

func optionalSignalFloat(v any) *float64 {
	f, ok := v.(float64)
	if !ok {
		return nil
	}
	return &f
}

// normalizeSignalErrorFrame 把 wiring 类型化错误帧归一为共享循环可识别的形状：
// 检测到 error_code 键时附加 "error" 字符串键（取 message，缺省回退
// error_code）。非错误帧、已是 {"error"} 形状的非类型化错误、非法 JSON 均
// 原样返回。
func normalizeSignalErrorFrame(result string) string {
	var m map[string]any
	if err := json.Unmarshal([]byte(result), &m); err != nil {
		return result
	}
	code, _ := m["error_code"].(string)
	if code == "" {
		return result
	}
	if e, ok := m["error"].(string); ok && e != "" {
		return result
	}
	msg, _ := m["message"].(string)
	if msg = strings.TrimSpace(msg); msg == "" {
		msg = code
	}
	m["error"] = msg
	out, err := json.Marshal(m)
	if err != nil {
		return result
	}
	return string(out)
}

// signalSourceErrorText 判定四源工具的错误帧：wiring 层把类型化错误序列化为
// {"error_code","message","detail"}，非类型化为 {"error"}——两处都算失败。
func signalSourceErrorText(resultJSON string) string {
	var m map[string]any
	if err := json.Unmarshal([]byte(resultJSON), &m); err != nil {
		return ""
	}
	if e, ok := m["error"].(string); ok && e != "" {
		return e
	}
	if code, _ := m["error_code"].(string); code != "" {
		msg, _ := m["message"].(string)
		return strings.TrimSpace(code + " " + msg)
	}
	return ""
}

func (l *signalResearchLedger) nextCalcID() string {
	l.nextCalc++
	return fmt.Sprintf("k%d", l.nextCalc)
}

// ── 研究 policy（toolLoopPolicy + toolLoopActionRunner + toolLoopFeedbackProvider）────────────

type signalResearchPolicy struct {
	ledger *signalResearchLedger
	// seenCalcKey 去重相同计算请求（重复动作不执行但计轮）。
	seenCalcKey map[string]bool
	// lastStep 记录最后一次真实做出的决策轮（CheckCall/RunLoopAction/
	// CheckFinish 都算决策）：循环收束分类的机械依据——所有决策轮都经过
	// policy，lastStep==上限 即预算耗尽，否则为异常终止。
	lastStep int
	// pendingQuestion 在 CheckCall 放行时记录、ObserveCall 消费（同一步内
	// 顺序执行，无并发）。
	pendingQuestion string
	// progressHook 在每个非终态决策轮结束后触发一次（tasks 4.7 每轮滚动
	// upsert 进展）。挂点选在轮次数据全部落账之后：ObserveCall 补完
	// question/观测索引后、RunLoopAction 记账/拦截后、CheckCall/CheckFinish
	// 拦截后。循环内 guard/dedup 拦截轮不触发（账本无变化，下一轮追平）。
	progressHook func(roundsDone int)

	// 无覆盖早停反馈状态（tasks 3.8）：sourceStreak 按源连续空手计数（成功
	// 取数清零）；sourceNudged 标记该源反馈已注入（每源最多一次，不随成功
	// 重置，防刷屏）；finaleNudged 标记终局提示已注入（恰一次）；
	// pendingFeedback 是本轮待注入内容，RunLoopFeedback 取走即清空。
	sourceStreak    map[string]int
	sourceNudged    map[string]bool
	finaleNudged    bool
	pendingFeedback []string
}

// 编译期钉住：研究 policy 实现可选反馈扩展（循环经 type assertion 征询，
// investigationPolicy 等未实现者与 policy=nil 字节不变）。
var _ toolLoopFeedbackProvider = (*signalResearchPolicy)(nil)

func newSignalResearchPolicy(ledger *signalResearchLedger) *signalResearchPolicy {
	return &signalResearchPolicy{
		ledger:       ledger,
		seenCalcKey:  map[string]bool{},
		sourceStreak: map[string]int{},
		sourceNudged: map[string]bool{},
	}
}

func (p *signalResearchPolicy) markStep(step int) {
	if step > p.lastStep {
		p.lastStep = step
	}
}

// notifyProgress fires the per-round progress hook（nil 安全：未接线时零开销）。
func (p *signalResearchPolicy) notifyProgress() {
	if p.progressHook != nil {
		p.progressHook(p.lastStep)
	}
}

// CheckCall validates every call_tool decision BEFORE execution：工具必须属
// 于四源白名单，且必须带非空 question（对应研究问题的一步）。Blocked 调用
// 不执行、不消耗去重键，但该决策轮照常消耗（防无限空转）。
func (p *signalResearchPolicy) CheckCall(step int, decision map[string]any) toolCallVerdict {
	p.markStep(step)
	toolName, _ := decision["tool"].(string)
	if !isSignalSourceTool(toolName) {
		p.notifyProgress()
		return toolCallVerdict{
			Blocked:       true,
			BlockedReason: "tool_not_in_research_whitelist",
			Feedback:      fmt.Sprintf("%q 不在本次研究的工具白名单内；只能调用四源工具：%s。与研究无关的源不要乱调凑数量，四源都不相关时请直接 finish。", toolName, strings.Join(signalResearchToolNames, " / ")),
		}
	}
	question, _ := decision["question"].(string)
	question = strings.TrimSpace(question)
	if question == "" {
		p.notifyProgress()
		return toolCallVerdict{
			Blocked:       true,
			BlockedReason: "missing_question",
			Feedback:      "每次 call_tool 必须带非空 question 字段，说明这次取数回答研究问题的哪一步；请补上 question 重新决定。",
		}
	}
	p.pendingQuestion = question
	return toolCallVerdict{}
}

// ObserveCall 只对真实执行过的调用触发（拦截不冒充执行）。除账本注记外，
// 在此维护按源连续空手计数（tasks 3.8）——calculate 与被拦/dedup/guard 轮
// 不会到达这里，天然不参与计数。
func (p *signalResearchPolicy) ObserveCall(_ int, toolName string, _ map[string]any, resultFull string, _ string, _ []string) {
	if isSignalSourceTool(toolName) {
		p.trackSourceCoverage(toolName, resultFull)
	}
	q := p.pendingQuestion
	p.pendingQuestion = ""
	if q == "" {
		return
	}
	p.ledger.annotateCall(toolName, q)
	// 本轮取数的账本变更（响应入账 + question 注记 + 覆盖计数）到此全部
	// 完成：滚动保存一次进展（tasks 4.7）。
	p.notifyProgress()
}

// CheckFinish：研究不设机械配额——证据足够、或四源都查不到相关数据时都可
// 以收束（design §4：四源不匹配时可提前 finish）。
func (p *signalResearchPolicy) CheckFinish(step int, _ string) toolFinishVerdict {
	p.markStep(step)
	return toolFinishVerdict{}
}

// RunLoopAction handles every non-call_tool/finish action inside the shared
// loop's default branch：calculate 走受限解释器；其余一律按非法动作拦截
// （不执行、计轮、给反馈）——这正是「非法/重复动作不执行但计轮」的机制。
func (p *signalResearchPolicy) RunLoopAction(step int, action string, decision map[string]any) (ToolCallRecord, string, bool) {
	p.markStep(step)
	blocked := func(reason, feedback string) (ToolCallRecord, string, bool) {
		fb, _ := json.Marshal(map[string]any{"error": "动作被拦: " + reason, "feedback": feedback})
		rec := ToolCallRecord{
			Thought:       getString(decision, "thought") + " [被拦:" + reason + "]",
			Args:          map[string]any{"action": action},
			ResultPreview: string(fb),
			ResultFull:    string(fb),
			Outcome:       toolCallOutcomeBlocked,
			BlockedReason: reason,
		}
		return rec, fmt.Sprintf("第%d步: 动作 %q — 被拦[%s]: %s", step, action, reason, feedback), true
	}

	if action != signalResearchActionCalculate {
		p.notifyProgress()
		return blocked("invalid_action",
			fmt.Sprintf("每轮只能输出三选一：call_tool（带 tool/args/question）、calculate（带 op/inputs/question）或 finish（带 summary）。收到的是 %q。", action))
	}

	if _, exists := decision["value"]; exists {
		p.notifyProgress()
		return blocked("value_forbidden", "计算值只能由代码计算并记录，模型不能提交 value 字段；请只声明 op 与 inputs。")
	}
	question := strings.TrimSpace(getString(decision, "question"))
	if question == "" {
		p.notifyProgress()
		return blocked("missing_question", "每次 calculate 必须带非空 question 字段，说明这次计算回答研究问题的哪一步。")
	}
	op := strings.TrimSpace(getString(decision, "op"))
	switch op {
	case SignalCalcOpDifference, SignalCalcOpPercentChange, SignalCalcOpMean:
	default:
		p.notifyProgress()
		return blocked("invalid_op", fmt.Sprintf("未知算子 %q；白名单只有 difference / percent_change / mean，不接受脚本或表达式。", op))
	}
	inputs := scrubIDList(decision["inputs"])
	if len(inputs) == 0 {
		p.notifyProgress()
		return blocked("invalid_inputs", "calculate.inputs 必须是非空字符串数组，元素是工具结果里的 observation_id（如 \"c1:o2\"）。")
	}
	key := op + "\x00" + strings.Join(inputs, "\x1f")
	if p.seenCalcKey[key] {
		p.notifyProgress()
		return blocked("duplicate_calculation", "同样的计算已经做过；请基于已有结果继续研究，或提出不同的问题。")
	}
	p.seenCalcKey[key] = true

	res := runSignalCalculation(p.ledger.obsIndex, SignalCalculationRequest{Op: op, Inputs: inputs, Question: question}, p.ledger.nextCalcID(), time.Now())
	p.ledger.calcs = append(p.ledger.calcs, res)
	// 本轮计算的账本变更到此全部完成：滚动保存一次进展（tasks 4.7）。
	p.notifyProgress()
	calcJSON, _ := json.Marshal(res)
	rec := ToolCallRecord{
		Thought:       getString(decision, "thought"),
		Tool:          signalResearchActionCalculate,
		Args:          map[string]any{"op": op, "inputs": inputs, "question": question},
		ResultPreview: truncateRunes(string(calcJSON), 300),
		ResultFull:    string(calcJSON),
	}
	line := fmt.Sprintf("第%d步: 计算 %s — 结果: %s", step, res.Expression, string(calcJSON))
	if res.Status == SignalCalcStatusRejected {
		rec.Outcome = toolCallOutcomeError
		line = fmt.Sprintf("第%d步: 计算 %s — 被拒绝[%s]：请改用合法输入或换计算", step, res.Expression, res.Reason)
	} else {
		rec.Outcome = toolCallOutcomeOK
	}
	return rec, line, true
}

// ── 无覆盖早停反馈（tasks 3.8 / design §10.2，经 toolLoopFeedbackProvider）──
//
// 计数只发生在 ObserveCall（真实执行过的四源 call_tool）：calculate 与被拦
// （blocked/dedup/guard）轮不经过 ObserveCall，天然不参与计数。反馈经
// runToolLoop 在本轮历史行之后追加一条独立历史行「系统提示（源覆盖）：…」
// ——不是工具结果、不新增 ToolCallRecord、不消耗决策轮；maxLoops=40 与
// 计轮/预算语义不变，也不强制终止（引导收束，不代替模型决策）。

// signalFilterKept 读取筛选后结果里的 filter_meta.kept；结果不带 filter_meta
// （解析失败/错误帧/无 observations 形状）按 0 处理——与账本 gap 判定
// （completeCall kept==0 记缺口）同口径。
func signalFilterKept(resultJSON string) int {
	var m struct {
		FilterMeta struct {
			Kept int `json:"kept"`
		} `json:"filter_meta"`
	}
	if err := json.Unmarshal([]byte(resultJSON), &m); err != nil {
		return 0
	}
	return m.FilterMeta.Kept
}

// trackSourceCoverage 维护按源连续空手计数并装配待注入反馈。空手 = 结果带
// 源错误帧（{"error"} 或 {"error_code"} 两形状，signalSourceErrorText 都认）
// 或 cutoff 筛选后 kept==0（no_data / 全部被剔）；成功取数（kept>0 且无错）
// 清零该源计数。连续 3 次空手 → 该源反馈一次（每源最多注入一次，不随成功
// 重置）；四源全部触发过 → 终局提示恰一次。
func (p *signalResearchPolicy) trackSourceCoverage(toolName, resultFull string) {
	if signalSourceErrorText(resultFull) != "" || signalFilterKept(resultFull) == 0 {
		p.sourceStreak[toolName]++
		if p.sourceStreak[toolName] < signalSourceNoCoverageStreak || p.sourceNudged[toolName] {
			return
		}
		p.sourceNudged[toolName] = true
		p.pendingFeedback = append(p.pendingFeedback, fmt.Sprintf(
			"%s：该源在本研究问题上已连续%d次无覆盖，换参数也不会有数据；若四源皆无覆盖请直接 finish（summary 里如实列缺口）",
			toolName, signalSourceNoCoverageStreak))
		allNudged := true
		for _, name := range signalResearchToolNames {
			if !p.sourceNudged[name] {
				allNudged = false
				break
			}
		}
		if allNudged && !p.finaleNudged {
			p.finaleNudged = true
			p.pendingFeedback = append(p.pendingFeedback,
				"四个数据源均确认无覆盖：请基于已有证据 finish，不要继续换参试探")
		}
		return
	}
	p.sourceStreak[toolName] = 0
}

// RunLoopFeedback（toolLoopFeedbackProvider）返回本轮待注入的反馈内容，取走
// 即清空（每条只注入一次）；空串 = 本轮无反馈。多条同轮触发（第4源反馈 +
// 终局提示）以换行拼接为一条历史行内容。
func (p *signalResearchPolicy) RunLoopFeedback() string {
	if len(p.pendingFeedback) == 0 {
		return ""
	}
	msg := strings.Join(p.pendingFeedback, "\n")
	p.pendingFeedback = nil
	return msg
}

// ── cutoff 过滤包装层（filtered registry）────────────────────────────

// SignalUnavailableNoticeAnchor 是「当前不可用」可用性标注段的起始锚点。
// wiring.UnavailableNoticeProbe 用它拼装研究期现算标注；克隆期先按锚点剥离
// 旧标注再重贴（design §10.4 可用性判断一律走 live resolver；service 包不得
// import datasources/wiring，共享锚点避免两侧字符串漂移）。
const SignalUnavailableNoticeAnchor = "\n【当前不可用：未配置 "

// applySignalToolAvailability 在研究克隆期重算工具描述的可用性段（review M2）：
// 先剥离旧标注（启动期烘焙或上一轮克隆留下的——annotation 总是描述尾段），
// 再按 live probe 追加；probe 返回空串=可用，旧标注一并移除。probe 为 nil 时
// 不触碰描述（旧测试/未注入路径零变化）。
func applySignalToolAvailability(desc, toolName string, probe func(string) string) string {
	if probe == nil {
		return desc
	}
	if i := strings.Index(desc, SignalUnavailableNoticeAnchor); i >= 0 {
		desc = desc[:i]
	}
	return desc + probe(toolName)
}

// buildSignalResearchRegistry clones the four research tools from the shared
// registry, wrapping Execute so the candidate cutoff is applied BEFORE the
// result reaches the agent（design §2：筛选先于 agent；完整原响应进账本并回写
// 工具日志；筛选后观测完整进附录）。Registry 未注册的源不进子注册表——调用
// 会得到未知工具错误并记 gap（LP-5 部分源不可用）。availabilityProbe 非 nil
// 时按 live resolver 重算每个源描述的可用性段（design §10.4；review M2）。
func buildSignalResearchRegistry(base *Registry, cutoff time.Time, ledger *signalResearchLedger, availabilityProbe func(string) string) *Registry {
	sub := &Registry{tools: make(map[string]*Tool, len(signalResearchToolNames))}
	for _, name := range signalResearchToolNames {
		inner := base.tools[name]
		if inner == nil {
			continue
		}
		sub.tools[name] = &Tool{
			Name:        inner.Name,
			Description: applySignalToolAvailability(inner.Description, inner.Name, availabilityProbe),
			InputSchema: inner.InputSchema,
			Execute: func(ctx context.Context, args map[string]any) (string, error) {
				callID := ledger.beginCall(name)
				raw, err := inner.Execute(ctx, args)
				filtered, kept, dropped := filterSignalToolResult(name, raw, cutoff, callID)
				ledger.completeCall(callID, name, args, raw, filtered, kept, dropped)
				// wiring 类型化错误帧（error_code）归一后返给 agent：共享循环的
				// toolResultErrorText 只认顶层 "error" 键，不归一则 result
				// tool_calls 的 Outcome 会把失败源调用误标 ok（账本 status 判定
				// 走 signalSourceErrorText，两形状都认，不受此影响）。
				return normalizeSignalErrorFrame(filtered), err
			},
		}
	}
	return sub
}

// ── prompt ───────────────────────────────────────────────────────────────────

// assembleSignalResearchPrompt builds the research system prompt（纯函数，可
// 无 LLM 契约测试）。输入只有候选快照与冻结材料——没有方法卡、没有最新材料
// 的入口（结构上保证 PC-5）。
func assembleSignalResearchPrompt(candidate *repository.BoardSignalCandidate, material *SignalMaterial, toolsDesc string) string {
	candidateJSON, _ := json.Marshal(map[string]any{
		"signal":            candidate.Signal,
		"why_it_matters":    candidate.WhyItMatters,
		"research_question": candidate.ResearchQuestion,
		"score":             candidate.Score,
		"rationale":         candidate.Rationale,
		"evidence_refs":     repository.SignalEvidenceRefs(candidate.EvidenceRefs),
	})
	materialJSON, _ := json.Marshal(material)

	var sb strings.Builder
	sb.WriteString("你是一位严谨的数据研究员。新闻板块在一个周期内发现了一个候选信号，你的任务是围绕它的问题驱动地做数据研究：每一步取数或计算都对应一个理解问题，寻找能支持或推翻候选信号解释的官方数据证据。研究不预选结论；主动寻找可能推翻主要解释的事实，并记录未能排除的其他解释。\n\n")
	sb.WriteString("---\n候选信号（本次研究唯一对象，冻结）：\n")
	sb.Write(candidateJSON)
	sb.WriteString("\n\n---\n冻结周期材料（发现时的新闻快照，仅作背景；不得当作官方数据引用）：\n")
	sb.Write(materialJSON)
	sb.WriteString("\n\n---\n可用数据源工具：\n")
	sb.WriteString(toolsDesc)
	sb.WriteString(`

工作纪律：
1. 取数优先服务研究问题：不要为凑数量乱调与研究无关的源/指标/经济体；四源都查不到相关数据时，直接 finish 并如实说明。
2. 每次 call_tool / calculate 必须带非空 question 字段，说明这一步回答研究问题的哪一小问。
3. 工具返回的 observations 已按研究时点（cutoff）过滤，每条带 observation_id（如 "c1:o2"）。后续计算与成文只能引用这些 observation_id；晚于 cutoff 的数据已被剔除，不要假设更新的数据存在。
4. calculate 是受限计算，值由代码计算（你不能自己算数、不能提交 value）：
   - difference(a,b)：同一系列不同时点，或同期间 imports−exports（净流量）；
   - percent_change(current,base)：同源同系列同单位，基数必须为正；
   - mean(...)：同系列明确互异期间集合，任一缺期会得到 missing。
   跨源换算、混单位、库存减流量、悬空引用都会被拒绝；被拒绝的计算也消耗执行预算。
5. 同 tool 同参数重复调用会被拦截；重复的计算请求也会被拦截——被拦的轮次同样消耗预算，请推进研究而不是原地重复。
6. 单次取数失败/参数错误会被记录并继续；不要用相同参数重试失败调用。
7. 预算：最多 40 轮决策、40 次执行（取数+计算合计）。证据足够即提前结束；预算耗尽不是失败，但正文必须披露缺口。
8. finish 的 summary 是给成文环节的研究笔记：列出关键发现（带 observation_id / 计算id）、仍未排除的解释、以及数据缺口。

每一轮输出严格 JSON，三选一：
- 取数：{"action":"call_tool","thought":"...","tool":"工具名","args":{...},"question":"这次取数要回答的问题"}
- 计算：{"action":"calculate","thought":"...","op":"difference|percent_change|mean","inputs":["c1:o1","c1:o2"],"question":"这次计算要回答的问题"}
- 结束：{"action":"finish","thought":"...","summary":"研究笔记（关键发现/未排除解释/缺口）"}

不要输出 JSON 以外的任何内容。`)
	return sb.String()
}

// ── 研究进展持久化（tasks 4.7「断了不能白跑」）──────────────────────────────────

// progressLedgerJSON serializes the in-memory ledger for the progress table's
// ledger jsonb column. 与报告 appendix 同源：复用 SignalReportAppendix 结构体
// 序列化（calls/calculations/gaps 三段），不维护第二份账本形状；rawResponse
// 为非导出字段不参与序列化（完整原响应只进工具日志，OB-2）。
func (l *signalResearchLedger) progressLedgerJSON() json.RawMessage {
	calls := l.calls
	if calls == nil {
		calls = []*signalResearchCallRecord{}
	}
	calcs := l.calcs
	if calcs == nil {
		calcs = []*SignalCalculationResult{}
	}
	gaps := l.gaps
	if gaps == nil {
		gaps = []signalResearchGap{}
	}
	b, err := json.Marshal(SignalReportAppendix{Calls: calls, Calculations: calcs, Gaps: gaps})
	if err != nil {
		return json.RawMessage("{}")
	}
	return b
}

// signalProgressRecorder persists one research job's rolling progress. 每轮
// 结束后 saveRunning（job ctx，死了就随下一轮/终态写追平）；终态写入必须脱
// 离已死的 job ctx —— abandon/supersede 用 context.WithoutCancel(runCtx)+
// 独立超时，否则超时场景下进展终态也写不进库（本任务的关键正确性点）。全
// 部尽力而为：进展写入失败只丢快照，不掩盖主流程结果。
type signalProgressRecorder struct {
	store     SignalProgressStore
	jobID     string
	candidate *repository.BoardSignalCandidate
}

// newSignalProgressRecorder returns nil when progress persistence is off
// (store 未接线或 jobID 为空——直调 service 的旧测试路径零影响).
func (s *SignalResearchService) newSignalProgressRecorder(jobID string, candidate *repository.BoardSignalCandidate) *signalProgressRecorder {
	if s.progress == nil || jobID == "" || candidate == nil {
		return nil
	}
	return &signalProgressRecorder{store: s.progress, jobID: jobID, candidate: candidate}
}

func (r *signalProgressRecorder) row(roundsDone int, ledger *signalResearchLedger) *repository.BoardSignalResearchProgress {
	return &repository.BoardSignalResearchProgress{
		JobID:            r.jobID,
		SemanticBoardID:  r.candidate.SemanticBoardID,
		CandidateID:      r.candidate.ID,
		Granularity:      r.candidate.Granularity,
		Period:           r.candidate.Period,
		RoundsDone:       roundsDone,
		SourceCalls:      len(ledger.calls),
		CalculationCalls: len(ledger.calcs),
		Ledger:           ledger.progressLedgerJSON(),
		Status:           repository.SignalResearchProgressRunning,
	}
}

// saveRunning 每轮结束后滚动 upsert（status=running，无 stop_reason）。同一
// job_id 永远命中同一行。
func (r *signalProgressRecorder) saveRunning(ctx context.Context, roundsDone int, ledger *signalResearchLedger) {
	if r == nil {
		return
	}
	_ = r.store.UpsertSignalResearchProgress(ctx, r.row(roundsDone, ledger))
}

// signalProgressStopReason derives the abandoned row's stop_reason：job 截止
// （DeadlineExceeded，含包装链）→ timeout；stage 已知 → error_stage 值；
// 其余 → failed。
func signalProgressStopReason(err error) string {
	if errors.Is(err, context.DeadlineExceeded) {
		return SignalProgressStopReasonTimeout
	}
	var stageErr *SignalStageError
	if errors.As(err, &stageErr) && stageErr.Stage != "" {
		return stageErr.Stage
	}
	return "failed"
}

// abandon writes the terminal abandoned snapshot AFTER the job ctx died
// (timeout/失败/取消）。WithoutCancel 保留值/甩掉取消：job_id ctx 值仍在，
// DB 写不再被已死 ctx 拒绝；独立短超时防 DB 挂起拖住 goroutine。
func (r *signalProgressRecorder) abandon(runCtx context.Context, roundsDone int, ledger *signalResearchLedger, runErr error) {
	if r == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(runCtx), 10*time.Second)
	defer cancel()
	row := r.row(roundsDone, ledger)
	row.Status = repository.SignalResearchProgressAbandoned
	row.StopReason = signalProgressStopReason(runErr)
	row.Error = runErr.Error()
	_ = r.store.UpsertSignalResearchProgress(ctx, row)
}

// supersede marks the row archived after the immutable report saved（成功后
// 进展归档，行保留供追溯）。同样脱离 run ctx：成功与 mark 之间 ctx 恰好到期
// 也不能丢终态。
func (r *signalProgressRecorder) supersede(runCtx context.Context) {
	if r == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(runCtx), 10*time.Second)
	defer cancel()
	_ = r.store.MarkSignalResearchProgressSuperseded(ctx, r.jobID)
}

// ── 服务编排（research → compose → 保存）─────────────────────────────────────

// SignalResearchStore narrows the repository surface research may touch.
// 刻意不含任何 lifeline / review / 新闻写路径：新链不可能污染主数据（NR-2，
// 编译级保证，与 2a detector 无 registry 同一手法）。
type SignalResearchStore interface {
	GetSignalCandidateByID(ctx context.Context, id uint) (*repository.BoardSignalCandidate, error)
	GetSignalDiscoveryByID(ctx context.Context, id uint) (*repository.BoardSignalDiscovery, error)
	CreateTopicEnrichmentResult(ctx context.Context, result *repository.TopicEnrichmentResult) error
}

// SignalProgressStore is the optional research-progress persistence surface
// (tasks 4.7「断了不能白跑」): per-round rolling upsert keyed by job_id + the
// post-success supersede mark. Deliberately separate from SignalResearchStore
// so existing stubs stay valid; nil（未接线）= 进展持久化关闭，其余行为不变。
// 实现方：repository.Repository（真实链路）。
type SignalProgressStore interface {
	UpsertSignalResearchProgress(ctx context.Context, p *repository.BoardSignalResearchProgress) error
	MarkSignalResearchProgressSuperseded(ctx context.Context, jobID string) error
}

// 研究 job 的研究进展停止原因（tasks 4.7）：timeout=job 截止（含取消传播）；
// 其余失败落在 error_stage（research|compose|save），无 stage 时回退 failed。
const SignalProgressStopReasonTimeout = "timeout"

// SignalResearchService runs one research job end to end: frozen snapshot →
// 40-round research loop → bounded compose → immutable result save.
type SignalResearchService struct {
	router     AirRouter
	capability airouter.Capability
	registry   *Registry
	store      SignalResearchStore
	composer   *SignalComposer
	now        func() time.Time
	// progress 是可选的进展持久化面（tasks 4.7）；nil = 关闭（旧测试/无 DB
	// 路径零影响）。
	progress SignalProgressStore
	// unavailableNotice 是可选的源可用性现算探针（review M2 / design §10.4）：
	// 非 nil 时研究克隆四源工具前按工具名现算「当前不可用」标注（空串=可用），
	// 能追加也能移除启动期旧标注；nil = 不触碰描述。外部经
	// SetToolAvailabilityProbe 注入——service 包不 import datasources/wiring。
	unavailableNotice func(toolName string) string
}

// NewSignalResearchService wires the research service. registry is the shared
// tool registry (four research tools must be registered; the service wraps
// them with the per-run cutoff filter). capability follows the existing
// board-analysis pattern.
func NewSignalResearchService(router AirRouter, capability airouter.Capability, registry *Registry, store SignalResearchStore) *SignalResearchService {
	return &SignalResearchService{
		router:     router,
		capability: capability,
		registry:   registry,
		store:      store,
		composer:   &SignalComposer{router: router, capability: capability},
		now:        time.Now,
	}
}

// SetSignalProgressStore attaches the optional progress store (post-
// construction wiring, same pattern as the handler runners).
func (s *SignalResearchService) SetSignalProgressStore(store SignalProgressStore) {
	s.progress = store
}

// SetToolAvailabilityProbe attaches the optional live source-availability
// probe (post-construction wiring，仿 SetSourceCapabilityText 先例：service
// 包类型上零依赖，外层注入普通 func(string) string）。返回非空串=该工具本周
// 次不可用及要追加的标注文本；空串=可用（已烘的旧标注会被移除）。nil = 不
// 触碰描述。
func (s *SignalResearchService) SetToolAvailabilityProbe(probe func(toolName string) string) {
	s.unavailableNotice = probe
}

// ResearchCandidate executes one candidate's research and returns the saved
// result id. jobID 是 runner 的任务身份（tasks 4.7 进展持久化滚动行主键；空
// 串 = 不持久化进展，直调 service 的旧路径零影响）。hook receives phase
// transitions（research|compose）for live job progress; failures carry
// *SignalStageError（Stage: research|compose|save）。取消/超时/循环异常终止 →
// failed，不写 result，候选保留可重试；已完成的轮次进展不随 job 死亡丢失：
// 每轮滚动 upsert + 终态 abandoned（后台 ctx）/成功 superseded。
func (s *SignalResearchService) ResearchCandidate(ctx context.Context, boardID, candidateID uint, jobID string, hook func(stage string)) (resultID uint, err error) {
	report := func(stage string) {
		if hook != nil {
			hook(stage)
		}
	}
	report(SignalStageResearch)

	candidate, err := s.store.GetSignalCandidateByID(ctx, candidateID)
	if err != nil {
		return 0, &SignalStageError{Stage: SignalStageResearch, Err: err}
	}
	if candidate.SemanticBoardID != boardID {
		return 0, &SignalStageError{Stage: SignalStageResearch, Err: fmt.Errorf("候选 %d 不属于板块 %d", candidateID, boardID)}
	}
	discovery, err := s.store.GetSignalDiscoveryByID(ctx, candidate.DiscoveryID)
	if err != nil {
		return 0, &SignalStageError{Stage: SignalStageResearch, Err: err}
	}
	if discovery.SemanticBoardID != boardID {
		return 0, &SignalStageError{Stage: SignalStageResearch, Err: fmt.Errorf("发现批次 %d 不属于板块 %d", discovery.ID, boardID)}
	}
	// PC-5：只读发现时冻结的快照——不重新装配材料、不偷换最新。
	var material SignalMaterial
	if err := json.Unmarshal(discovery.InputSnapshot, &material); err != nil {
		return 0, &SignalStageError{Stage: SignalStageResearch, Err: fmt.Errorf("解冻输入快照: %w", err)}
	}
	if material.Granularity != candidate.Granularity || material.Period != candidate.Period {
		return 0, &SignalStageError{Stage: SignalStageResearch, Err: errors.New("候选与发现批次的周期不一致（数据可疑，拒绝研究）")}
	}

	// 进展持久化（tasks 4.7）：身份齐备后建 recorder；终态（abandoned/
	// superseded）统一在 defer 里写——失败路径返回点分散，defer 保证无遗漏，
	// 且在返回值定型（result 已落库 / 错误已包装）之后执行。
	recorder := s.newSignalProgressRecorder(jobID, candidate)

	sessionID := GenerateSignalResearchSessionID(candidateID)
	cutoff := discovery.Cutoff
	ledger := newSignalResearchLedger(cutoff)
	filteredRegistry := buildSignalResearchRegistry(s.registry, cutoff, ledger, s.unavailableNotice)
	policy := newSignalResearchPolicy(ledger)
	if recorder != nil {
		policy.progressHook = func(roundsDone int) {
			recorder.saveRunning(ctx, roundsDone, ledger)
		}
		defer func() {
			if err != nil {
				recorder.abandon(ctx, policy.lastStep, ledger, err)
			} else if resultID != 0 {
				recorder.supersede(ctx)
			}
		}()
	}
	allowedTools := append([]string{}, signalResearchToolNames...)

	loop, err := runToolLoop(ctx, s.router, filteredRegistry, s.capability, toolLoopParams{
		sessionID:    sessionID,
		systemPrompt: assembleSignalResearchPrompt(candidate, &material, buildToolsDesc(filteredRegistry, allowedTools)),
		taskLine:     "研究问题: " + candidate.ResearchQuestion,
		operation:    signalResearchOperation,
		allowedTools: allowedTools,
		maxLoops:     signalResearchMaxLoops,
		resultTopic:  truncateRunes(candidate.Signal, boardResearchTopicMaxRunes),
		policy:       policy,
	})
	if err != nil {
		return 0, &SignalStageError{Stage: SignalStageResearch, Err: err}
	}
	// 收束分类（design §4）：finish → finished；预算耗尽 →
	// budget_exhausted（不是失败，以已有证据成文并披露缺口）；取消/超时/
	// 循环异常终止 → failed。所有决策轮都经过 policy（CheckCall/
	// RunLoopAction/CheckFinish），lastStep==上限 即 40 轮决策全部消耗；
	// 否则循环在决策中途死亡（LLM 错误/解析失败），不伪装收束。
	stopReason := ""
	switch {
	case loop.FinalData != "":
		stopReason = SignalStopReasonFinished
	case policy.lastStep >= signalResearchMaxLoops:
		stopReason = SignalStopReasonBudgetExhausted
	}
	if stopReason == "" {
		if cerr := ctx.Err(); cerr != nil {
			return 0, &SignalStageError{Stage: SignalStageResearch, Err: fmt.Errorf("研究被取消或超时: %w", cerr)}
		}
		reason := loop.Error
		if reason == "" {
			reason = "研究循环异常终止"
		}
		return 0, &SignalStageError{Stage: SignalStageResearch, Err: errors.New(reason)}
	}

	report(SignalStageCompose)
	payload, err := s.composer.Compose(ctx, SignalComposeInput{
		SessionID:    sessionID,
		Candidate:    candidate,
		Material:     &material,
		Ledger:       ledger,
		Loop:         loop,
		StopReason:   stopReason,
		Decisions:    loop.Loops,
		AnalysisMode: material.AnalysisMode,
		Cutoff:       cutoff,
	})
	if err != nil {
		return 0, &SignalStageError{Stage: SignalStageCompose, Err: err}
	}

	// 保存（save 不是 phase；失败经 SignalStageError 落 error_stage）。
	granularity, period := candidate.Granularity, candidate.Period
	boardIDCopy := candidate.SemanticBoardID
	result := &repository.TopicEnrichmentResult{
		AnalysisScope:   "board",
		ResultKind:      repository.ResultKindSignalReport,
		SemanticBoardID: &boardIDCopy,
		Granularity:     &granularity,
		Period:          &period,
		SourceSignalID:  &candidate.ID,
		Sectors:         payload,
		ToolCalls:       jsonRawOrNil(mergeRawResponsesIntoRecords(loop.ToolCalls, ledger)),
		InputSnapshot:   append([]byte(nil), discovery.InputSnapshot...),
		SessionID:       sessionID,
	}
	if err := s.store.CreateTopicEnrichmentResult(ctx, result); err != nil {
		return 0, &SignalStageError{Stage: SignalStageSave, Err: err}
	}
	return result.ID, nil
}

// mergeRawResponsesIntoRecords restores each executed source call's FULL raw
// response into the durable tool-call records（OB-2「完整原响应进工具日志」；
// agent 历史里看到的是筛选后版本，工具日志里留原始版本）。执行顺序与账本
// 一致（一次循环一步执行），按序配对；blocked/计算记录原样保留。
func mergeRawResponsesIntoRecords(records []ToolCallRecord, ledger *signalResearchLedger) []ToolCallRecord {
	out := append([]ToolCallRecord(nil), records...)
	ci := 0
	for i := range out {
		rec := &out[i]
		if !isSignalSourceTool(rec.Tool) || (rec.Outcome != toolCallOutcomeOK && rec.Outcome != toolCallOutcomeError) {
			continue
		}
		if ci < len(ledger.calls) {
			rec.ResultFull = ledger.calls[ci].rawResponse
			ci++
		}
	}
	return out
}

func jsonRawOrNil(v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	return b
}
