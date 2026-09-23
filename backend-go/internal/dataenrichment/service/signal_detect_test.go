package service

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"testing"

	"syntopica-backend/internal/dataenrichment/repository"
	"syntopica-backend/internal/platform/airouter"
)

// Signal detect unit tests (board-signal-reports SD-1..SD-5). 纯逻辑 + stub
// LLM：不起真模型、无 DB、无 SQLite。断言锚点：发现全程取数调用=0、计算=0、
// compose=0——detector 类型层面不持有 tool registry（无法发起任何工具调用），
// 测试钉住 LLM Chat 恰为尝试数、且无其他副作用。

// sdReply is one canned Chat outcome; sdStubRouter replays them in order and
// FAILS any call beyond the scripted list (so "LLM was never called" and
// "LLM was called exactly twice" are both mechanically enforced).
type sdReply struct {
	content string
	err     error
}

type sdStubRouter struct {
	replies []sdReply
	calls   []airouter.ChatRequest
}

func (r *sdStubRouter) Chat(_ context.Context, req airouter.ChatRequest) (*airouter.ChatResult, error) {
	r.calls = append(r.calls, req)
	if len(r.calls) > len(r.replies) {
		return nil, fmt.Errorf("sdStubRouter: unexpected Chat call #%d", len(r.calls))
	}
	rep := r.replies[len(r.calls)-1]
	if rep.err != nil {
		return nil, rep.err
	}
	return &airouter.ChatResult{Content: rep.content}, nil
}

func sdFixtureMaterial() *SignalMaterial {
	return &SignalMaterial{
		Granularity:  repository.SignalGranularityMonth,
		Period:       "2026-08",
		AnalysisMode: repository.SignalAnalysisModeRetrospective,
		Lanes: []SignalLaneMaterial{{
			LaneID: 7,
			Label:  "原油",
			Status: "active",
			Articles: []SignalArticleSlice{
				{SectionID: 101, ClusterLabel: "炼厂开工异动", ArticleCount: 5},
				{SectionID: 102, ClusterLabel: "库存去化背离", ArticleCount: 3},
			},
		}},
		Gaps: []SignalMaterialGap{{Reason: "泳道无历史归属记录"}},
	}
}

func sdSignal(signal string, score int, refs ...string) map[string]any {
	return map[string]any{
		"signal":            signal,
		"why_it_matters":    "w:" + signal,
		"research_question": "q:" + signal,
		"evidence_refs":     refs,
		"score":             score,
		"rationale":         "r:" + signal,
	}
}

func sdResponse(t *testing.T, signals []map[string]any) string {
	t.Helper()
	payload, err := json.Marshal(map[string]any{"signals": signals})
	if err != nil {
		t.Fatalf("marshal stub response: %v", err)
	}
	return string(payload)
}

func newSDRouter(replies ...sdReply) *sdStubRouter {
	return &sdStubRouter{replies: replies}
}

func newSDDetector(router AirRouter) *SignalDetector {
	return NewSignalDetector(router, "data_enrichment_analysis")
}

// ── session id（design §9：发现独立 session）──────────────────────────────────

func TestGenerateSignalDiscoverySessionIDFormat(t *testing.T) {
	id := GenerateSignalDiscoverySessionID(5)
	re := regexp.MustCompile(`^board_signal_discovery_5_[0-9a-f]{8}$`)
	if !re.MatchString(id) {
		t.Fatalf("session id %q does not match board_signal_discovery_{id}_{hex8}", id)
	}
	if GenerateSignalDiscoverySessionID(5) == id {
		t.Fatalf("session ids must be unique per call")
	}
}

// ── SD-2：空材料零 LLM，安静零候选 ────────────────────────────────────────────

func TestSignalDetect_EmptyMaterialSkipsLLM(t *testing.T) {
	router := newSDRouter(sdReply{content: sdResponse(t, []map[string]any{sdSignal("不该被调用", 9, "101")})})
	for name, material := range map[string]*SignalMaterial{
		"nil material": nil,
		"empty lanes":  {Granularity: repository.SignalGranularityMonth, Period: "2026-08"},
	} {
		detections, err := newSDDetector(router).Detect(t.Context(), material, "board_signal_discovery_5_abcd1234")
		if err != nil {
			t.Fatalf("%s: empty material must not error: %v", name, err)
		}
		if len(detections) != 0 {
			t.Fatalf("%s: empty material must yield zero candidates, got %d", name, len(detections))
		}
	}
	if len(router.calls) != 0 {
		t.Fatalf("empty material must skip the LLM entirely, got %d Chat calls", len(router.calls))
	}
}

// ── SD-1：合法解析（含 markdown 修复包装）+ 请求契约 ─────────────────────────

func TestSignalDetect_ValidResponseParses(t *testing.T) {
	router := newSDRouter(sdReply{content: sdResponse(t, []map[string]any{sdSignal("炼厂开工与库存背离", 8, "101", "102")})})
	detections, err := newSDDetector(router).Detect(t.Context(), sdFixtureMaterial(), "board_signal_discovery_5_abcd1234")
	if err != nil {
		t.Fatalf("valid response: %v", err)
	}
	if len(detections) != 1 {
		t.Fatalf("want 1 detection, got %d", len(detections))
	}
	d := detections[0]
	if d.Signal != "炼厂开工与库存背离" || d.Score != 8 || d.WhyItMatters != "w:炼厂开工与库存背离" ||
		d.ResearchQuestion != "q:炼厂开工与库存背离" || d.Rationale != "r:炼厂开工与库存背离" {
		t.Fatalf("detection fields lost: %+v", d)
	}
	if len(d.EvidenceRefs) != 2 || d.EvidenceRefs[0] != "101" || d.EvidenceRefs[1] != "102" {
		t.Fatalf("evidence refs lost: %v", d.EvidenceRefs)
	}

	// 请求契约：/no_think 防线、operation、session、capability（OB-1）。
	if len(router.calls) != 1 {
		t.Fatalf("exactly one Chat call expected, got %d（取数=0/计算=0/compose=0 由无 registry 保证）", len(router.calls))
	}
	req := router.calls[0]
	if len(req.Messages) != 2 {
		t.Fatalf("want system+user messages, got %d", len(req.Messages))
	}
	if req.Messages[1].Role != "user" || !strings.HasPrefix(req.Messages[1].Content, "/no_think") {
		t.Fatalf("user message must carry the /no_think defense prefix")
	}
	if req.Operation != "data_enrichment.signal_detect" {
		t.Fatalf("operation = %q", req.Operation)
	}
	if req.SessionID != "board_signal_discovery_5_abcd1234" {
		t.Fatalf("session id not passed through: %q", req.SessionID)
	}
	if req.Capability != airouter.Capability("data_enrichment_analysis") {
		t.Fatalf("capability = %q", req.Capability)
	}
	if !contains(req.Messages[1].Content, "101") || !contains(req.Messages[1].Content, "炼厂开工异动") {
		t.Fatalf("prompt must embed the material slices (whitelist source)")
	}
}

func TestSignalDetect_FencedJSONParses(t *testing.T) {
	raw := "```json\n" + sdResponse(t, []map[string]any{sdSignal("修复包装", 7, "101")}) + "\n```"
	router := newSDRouter(sdReply{content: raw})
	detections, err := newSDDetector(router).Detect(t.Context(), sdFixtureMaterial(), "s")
	if err != nil || len(detections) != 1 {
		t.Fatalf("fenced JSON must parse (SD-1 修复包装): %v, %d detections", err, len(detections))
	}
}

// ── SD-1：非法 schema/路由错误 → 重试 1 次（共 2 尝试）后报错，不吞成空数组 ──

func TestSignalDetect_InvalidSchemaRetriesOnceThenFails(t *testing.T) {
	cases := map[string]string{
		"missing rationale":   sdResponse(t, []map[string]any{{"signal": "s", "why_it_matters": "w", "research_question": "q", "evidence_refs": []string{"101"}, "score": 7}}),
		"empty signal":        sdResponse(t, []map[string]any{sdSignal("  ", 7, "101")}),
		"null evidence refs":  sdResponse(t, []map[string]any{sdSignal("s", 7)}),
		"missing signals key": `{"other":1}`,
		"signals not array":   `{"signals":"nope"}`,
		"score out of range":  sdResponse(t, []map[string]any{sdSignal("s", 11, "101")}),
		"score not integer":   sdResponse(t, []map[string]any{{"signal": "s", "why_it_matters": "w", "research_question": "q", "evidence_refs": []string{"101"}, "score": 6.5, "rationale": "r"}}),
		"score not number":    `{"signals":[{"signal":"s","why_it_matters":"w","research_question":"q","evidence_refs":["101"],"score":"7","rationale":"r"}]}`,
		"empty refs array":    `{"signals":[{"signal":"s","why_it_matters":"w","research_question":"q","evidence_refs":[],"score":7,"rationale":"r"}]}`,
	}
	for name, content := range cases {
		router := newSDRouter(sdReply{content: content}, sdReply{content: content})
		detections, err := newSDDetector(router).Detect(t.Context(), sdFixtureMaterial(), "s")
		if err == nil {
			t.Fatalf("%s: must fail after retry, got detections %+v", name, detections)
		}
		if detections != nil {
			t.Fatalf("%s: error path must not return candidates", name)
		}
		if len(router.calls) != signalDetectMaxAttempts {
			t.Fatalf("%s: want exactly %d attempts, got %d", name, signalDetectMaxAttempts, len(router.calls))
		}
	}
}

func TestSignalDetect_InvalidSchemaSingleBadSignalSinksResponse(t *testing.T) {
	// 一条信号破约 → 整响应作废（破坏数值契约的模型不能被部分信任）。
	content := sdResponse(t, []map[string]any{sdSignal("good", 9, "101"), sdSignal("bad", 11, "101")})
	router := newSDRouter(sdReply{content: content}, sdReply{content: content})
	detections, err := newSDDetector(router).Detect(t.Context(), sdFixtureMaterial(), "s")
	if err == nil || len(router.calls) != 2 {
		t.Fatalf("response with one invalid signal must invalidate the whole response and retry: err=%v calls=%d", err, len(router.calls))
	}
	if detections != nil {
		t.Fatalf("no candidates may survive a failed response")
	}
}

func TestSignalDetect_RouterErrorRetriesOnceThenFails(t *testing.T) {
	router := newSDRouter(sdReply{err: fmt.Errorf("route down")}, sdReply{err: fmt.Errorf("route down")})
	detections, err := newSDDetector(router).Detect(t.Context(), sdFixtureMaterial(), "s")
	if err == nil {
		t.Fatalf("persistent router errors must surface (job → failed), not become empty arrays")
	}
	if detections != nil || len(router.calls) != 2 {
		t.Fatalf("want 2 attempts and no detections, got calls=%d detections=%v", len(router.calls), detections)
	}
}

func TestSignalDetect_RetryRecoversOnSecondAttempt(t *testing.T) {
	router := newSDRouter(
		sdReply{err: fmt.Errorf("transient")},
		sdReply{content: sdResponse(t, []map[string]any{sdSignal("重试成功", 7, "102")})},
	)
	detections, err := newSDDetector(router).Detect(t.Context(), sdFixtureMaterial(), "s")
	if err != nil || len(detections) != 1 {
		t.Fatalf("second attempt must recover: err=%v detections=%d", err, len(detections))
	}
	if len(router.calls) != 2 {
		t.Fatalf("want exactly 2 calls, got %d", len(router.calls))
	}
}

// ── SD-3：score 门槛——5 过滤、6 入选 ────────────────────────────────────────

func TestSignalDetect_ScoreThreshold(t *testing.T) {
	router := newSDRouter(sdReply{content: sdResponse(t, []map[string]any{
		sdSignal("五分不够", 5, "101"),
		sdSignal("六分达标", 6, "102"),
	})})
	detections, err := newSDDetector(router).Detect(t.Context(), sdFixtureMaterial(), "s")
	if err != nil {
		t.Fatalf("threshold run: %v", err)
	}
	if len(detections) != 1 || detections[0].Signal != "六分达标" {
		t.Fatalf("score 5 must be filtered, 6 kept: %+v", detections)
	}
}

// ── SD-4：悬空/周期外引用剔除，无依据信号丢弃（高分不绕过）──────────────────

func TestSignalDetect_EvidenceWhitelistFiltering(t *testing.T) {
	router := newSDRouter(sdReply{content: sdResponse(t, []map[string]any{
		// 混合引用：101 在白名单内，999 悬空、lane-9 外部格式 → 只剩 101。
		sdSignal("混合引用", 8, "101", "999", "lane-9"),
		// 全部悬空引用的高分信号 → 整条丢弃（高分不绕过）。
		sdSignal("全悬空高分", 10, "999", "2026-09"),
	})})
	detections, err := newSDDetector(router).Detect(t.Context(), sdFixtureMaterial(), "s")
	if err != nil {
		t.Fatalf("whitelist run must not error: %v", err)
	}
	if len(detections) != 1 {
		t.Fatalf("only the signal with valid evidence survives, got %+v", detections)
	}
	if got := detections[0].EvidenceRefs; len(got) != 1 || got[0] != "101" {
		t.Fatalf("dangling/foreign refs must be dropped, kept %v", got)
	}
}

// ── SD-5：多条达标全部保留（保存交给 repository；无自动 research）────────────

func TestSignalDetect_MultipleSignalsAllKept(t *testing.T) {
	router := newSDRouter(sdReply{content: sdResponse(t, []map[string]any{
		sdSignal("信号一", 9, "101"),
		sdSignal("信号二", 7, "102"),
		sdSignal("信号三", 6, "101", "102"),
		sdSignal("不达标", 3, "101"),
	})})
	detections, err := newSDDetector(router).Detect(t.Context(), sdFixtureMaterial(), "s")
	if err != nil {
		t.Fatalf("multi-signal run: %v", err)
	}
	if len(detections) != 3 {
		t.Fatalf("want 3 kept candidates, got %d", len(detections))
	}
	for i, want := range []string{"信号一", "信号二", "信号三"} {
		if detections[i].Signal != want {
			t.Fatalf("order must be preserved: [%d]=%s want %s", i, detections[i].Signal, want)
		}
	}
	// 发现全程零取数/零计算/零成文：LLM Chat 恰 1 次，detector 无 registry
	// 字段（编译期即不存在工具面），无任何其他副作用通道。
	if len(router.calls) != 1 {
		t.Fatalf("detect must be exactly one Chat call, got %d", len(router.calls))
	}
}

// 检测结果 → repository 候选行的映射保持全字段（保存前最后一跳）。
func TestSignalDetectionsToCandidates(t *testing.T) {
	detections := []SignalDetection{{
		Signal:           "s",
		WhyItMatters:     "w",
		ResearchQuestion: "q",
		EvidenceRefs:     []string{"101"},
		Score:            8,
		Rationale:        "r",
	}}
	candidates, err := SignalDetectionsToCandidates(detections)
	if err != nil || len(candidates) != 1 {
		t.Fatalf("map: %v", err)
	}
	c := candidates[0]
	if c.Signal != "s" || c.WhyItMatters != "w" || c.ResearchQuestion != "q" || c.Rationale != "r" || c.Score != 8 {
		t.Fatalf("candidate fields lost: %+v", c)
	}
	if refs := repository.SignalEvidenceRefs(c.EvidenceRefs); len(refs) != 1 || refs[0] != "101" {
		t.Fatalf("evidence refs roundtrip: %v", refs)
	}
}

func contains(haystack, needle string) bool {
	return strings.Contains(haystack, needle)
}

// ── tasks 3.7 发现半边：能力块注入（prompt 契约）────────────────────────────

// sdCapabilityFixture is a hand-written minimal block for prompt-contract
// assertions; the real production text is wiring.SourceCapabilityText(), which
// the wiring package's own test pins against datasources.Catalog().
const sdCapabilityFixture = "【可用数据源目录】eia_wpsr | 美国（仅美国数据）| weekly\n【硬限制】四源均无价格序列、裂解价差、运价、政策文件；发现阶段只做选题，不调用任何数据源取数。"

// 注入后：system prompt = 旧 const 前缀 + 能力块；用户 prompt 不变；仍是且仅
// 是一次 Chat（不因此出现任何工具面/取数路径——类型上无 registry）。
func TestSignalDetect_SourceCapabilityInjectedIntoSystemPrompt(t *testing.T) {
	router := newSDRouter(sdReply{content: sdResponse(t, []map[string]any{sdSignal("库存去化背离", 7, "102")})})
	detector := newSDDetector(router)
	detector.SetSourceCapabilityText(sdCapabilityFixture)
	detections, err := detector.Detect(t.Context(), sdFixtureMaterial(), "board_signal_discovery_5_abcd1234")
	if err != nil {
		t.Fatalf("detect with capability block: %v", err)
	}
	if len(detections) != 1 {
		t.Fatalf("capability block must not change the detect contract, got %d detections", len(detections))
	}
	if len(router.calls) != 1 {
		t.Fatalf("exactly one Chat call expected, got %d", len(router.calls))
	}
	wantSystem := signalDetectSystemPrompt + "\n\n" + sdCapabilityFixture
	if got := router.calls[0].Messages[0].Content; got != wantSystem {
		t.Fatalf("system prompt must be base + capability block\nwant: %q\ngot:  %q", wantSystem, got)
	}
	if !contains(router.calls[0].Messages[0].Content, "eia_wpsr") ||
		!contains(router.calls[0].Messages[0].Content, "硬限制") ||
		!contains(router.calls[0].Messages[0].Content, "不调用任何数据源取数") {
		t.Fatalf("capability block lost from system prompt")
	}
	// 用户 prompt 不受注入影响（材料契约不变）。
	userMsg := router.calls[0].Messages[1].Content
	if !strings.HasPrefix(userMsg, "/no_think") || !contains(userMsg, "炼厂开工异动") {
		t.Fatalf("user prompt must stay the material-only contract: %q", userMsg)
	}
}

// 未注入（空文本）：system prompt 与旧 const 字节一致——旧行为可测不漂移。
func TestSignalDetect_EmptyCapabilityKeepsLegacyPromptBytes(t *testing.T) {
	router := newSDRouter(sdReply{content: sdResponse(t, []map[string]any{sdSignal("背离", 7, "101")})})
	detector := newSDDetector(router)
	detector.SetSourceCapabilityText("") // 显式空文本：显式退化路径
	if _, err := detector.Detect(t.Context(), sdFixtureMaterial(), "board_signal_discovery_5_abcd1234"); err != nil {
		t.Fatalf("detect without capability block: %v", err)
	}
	if len(router.calls) != 1 {
		t.Fatalf("exactly one Chat call expected, got %d", len(router.calls))
	}
	if got := router.calls[0].Messages[0].Content; got != signalDetectSystemPrompt {
		t.Fatalf("empty capability must keep the legacy prompt byte-identically\nwant: %q\ngot:  %q", signalDetectSystemPrompt, got)
	}
}
