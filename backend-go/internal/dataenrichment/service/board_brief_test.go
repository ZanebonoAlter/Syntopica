package service

import (
	"context"
	"strings"
	"testing"

	"syntopica-backend/internal/platform/airouter"
)

// ── board_brief parser / prompt / 机械降级（tasks 3.1/3.2/3.4，M2/M3）───────
//
// 用例清单（test-cases M2/M3 + 任务 3.1）：
//   M2.2 无统一关系仍正常 | M2.3 并行趋势不压缩 | M2.4 全 sparse 不产问题
//   M2.5 坏 JSON 重试一次 | M2.6 机械降级只列观察 | M2.7 数量上限
//   M2.8 prompt 无工具/方法卡/反转句式 | M3.2 非法枚举剔除 | M3.3 幽灵 lane
//   M3.4 悬空 evidence_ref | M3.5 possible_causal 边界 | M3.6 质量分不当证据

func briefTestCards() []LaneSituationCard {
	return []LaneSituationCard{
		{LaneID: 1, Label: "泳道一", FactsDigest: "事实甲：一期产能落地", FactsSource: "lifeline_week", LastSeenDate: "2026-08-26", QualityScore: 20},
		{LaneID: 2, Label: "泳道二", FactsDigest: "事实乙：二期招标启动", FactsSource: "lifeline_month", LastSeenDate: "2026-08-25", QualityScore: 15},
		{LaneID: 3, Label: "泳道三", FactsDigest: "事实丙：配套政策征求意见", FactsSource: "section_fingerprint", LastSeenDate: "2026-08-24", QualityScore: 10},
	}
}

func parseBriefOrFatal(t *testing.T, llmJSON string, cards []LaneSituationCard, allSparse bool) *BoardBriefPayload {
	t.Helper()
	parsed, err := ParseJSONResponse(llmJSON)
	if err != nil {
		t.Fatalf("parse json: %v", err)
	}
	brief, ok := parseBoardBrief(parsed, cards, allSparse)
	if !ok {
		t.Fatalf("parseBoardBrief: structurally invalid payload: %s", llmJSON)
	}
	return brief
}

// M2.1/M2.2 有素材但没有统一关系：正常 brief，relationships 可空，不算失败。
func TestParseBoardBrief_NoUnifiedRelationship(t *testing.T) {
	brief := parseBriefOrFatal(t, `{"summary":"板块三条泳道各自有进展，暂未发现统一关系。","observations":[
		{"id":"o1","lane_id":1,"statement":"一期产能落地","basis":"态势卡周摘要","as_of_date":"2026-08-26"},
		{"id":"o2","lane_id":2,"statement":"二期招标启动","basis":"态势卡月摘要","as_of_date":"2026-08-25"}],
		"relationships":[],"uncertainties":[],"research_questions":[]}`, briefTestCards(), false)
	if brief.Summary == "" || len(brief.Observations) != 2 {
		t.Fatalf("brief shape wrong: %+v", brief)
	}
	if len(brief.Relationships) != 0 {
		t.Fatalf("no-relationship board must allow empty relationships, got %d", len(brief.Relationships))
	}
	if brief.Degraded {
		t.Fatal("no unified relationship is a normal outcome, not degradation")
	}
}

// M2.3 并行趋势：divergent 关系 + 独立观察分别保留，不压缩成单一原因。
func TestParseBoardBrief_ParallelTrends(t *testing.T) {
	brief := parseBriefOrFatal(t, `{"summary":"板块存在两条方向相反的并行趋势。","observations":[
		{"id":"o1","lane_id":1,"statement":"扩张趋势","basis":"周摘要","as_of_date":"2026-08-26"},
		{"id":"o2","lane_id":2,"statement":"收缩趋势","basis":"月摘要","as_of_date":"2026-08-25"}],
		"relationships":[{"lane_ids":[1,2],"type":"divergent","explanation":"一条扩张一条收缩","confidence":"medium","evidence_refs":["o1","o2"]}],
		"uncertainties":[],"research_questions":[{"id":"q1","question":"分化会持续吗","rationale":"影响后续布局","related_lane_ids":[1,2]}]}`,
		briefTestCards(), false)
	if len(brief.Observations) != 2 {
		t.Fatalf("parallel observations must be kept separately, got %d", len(brief.Observations))
	}
	if len(brief.Relationships) != 1 || brief.Relationships[0].Type != RelationDivergent {
		t.Fatalf("divergent relationship must survive: %+v", brief.Relationships)
	}
	if len(brief.ResearchQuestions) != 1 {
		t.Fatalf("research question kept, got %d", len(brief.ResearchQuestions))
	}
}

// M2.4 全 sparse：诚实素材不足；即使 LLM 无视提示硬造观察/关系/问题也机械清空。
// 关系输入特意用真实 lane_id + 合法枚举 + 高置信（这些在非 sparse 路径全部
// 合法存活），确保清空来自 allSparse 纪律本身而非兜底过滤的假阳性。
func TestParseBoardBrief_AllSparseDropsQuestions(t *testing.T) {
	cards := []LaneSituationCard{
		{LaneID: 7, Label: "空泳道甲", FactsSource: "none", LastSeenDate: "2026-08-01"},
		{LaneID: 8, Label: "空泳道乙", FactsSource: "none", LastSeenDate: "2026-08-01"},
	}
	brief := parseBriefOrFatal(t, `{"summary":"该板块近期无可观察素材。","observations":[
		{"id":"o1","lane_id":7,"statement":"幻觉观察","basis":"b","as_of_date":"2026-08-26"}],
		"relationships":[
		{"lane_ids":[7,8],"type":"common_driver","explanation":"幻觉共同驱动","confidence":"high","evidence_refs":["o1"]},
		{"lane_ids":[7,8],"type":"context_only","explanation":"幻觉背景相关","confidence":"medium","evidence_refs":[]},
		{"lane_ids":[7,8],"type":"possible_causal","explanation":"幻觉传导","confidence":"high","evidence_refs":["o1"]}],
		"uncertainties":[{"question":"动向如何","why_uncertain":"无素材","needed_evidence":"等待命中"}],
		"research_questions":[{"id":"q1","question":"不该出现","rationale":"sparse","related_lane_ids":[7]}]}`, cards, true)
	if len(brief.Observations) != 0 || len(brief.Relationships) != 0 {
		t.Fatalf("sparse brief must stay empty (relationships cleared regardless of lane/enum/confidence legality), got %+v", brief)
	}
	if len(brief.ResearchQuestions) != 0 {
		t.Fatalf("all-sparse must not generate research questions, got %d", len(brief.ResearchQuestions))
	}
	if len(brief.Uncertainties) != 1 {
		t.Fatalf("uncertainty kept: %+v", brief.Uncertainties)
	}
	if brief.Summary == "" {
		t.Fatal("summary must survive the sparse clear")
	}
}

// M3.1 五枚举全部合法保留；confidence 缺省/非法 → low。
func TestParseBoardBrief_LegalRelationEnums(t *testing.T) {
	var sb strings.Builder
	sb.WriteString(`{"summary":"s","observations":[{"id":"o1","lane_id":1,"statement":"a","basis":"b","as_of_date":"2026-08-26"}],"relationships":[`)
	for i, typ := range []string{RelationCommonDriver, RelationPossibleCausal, RelationDivergent, RelationContextOnly, RelationUnclear} {
		if i > 0 {
			sb.WriteString(",")
		}
		sb.WriteString(`{"lane_ids":[1,2],"type":"` + typ + `","explanation":"e","confidence":"medium","evidence_refs":["o1"]}`)
	}
	sb.WriteString(`],"uncertainties":[],"research_questions":[]}`)
	brief := parseBriefOrFatal(t, sb.String(), briefTestCards(), false)
	if len(brief.Relationships) != 5 {
		t.Fatalf("all five legal enums must survive, got %d", len(brief.Relationships))
	}
}

// M3.2 非法 relation enum 剔除该条并留痕（不拒整份）；非法 confidence 降 low。
func TestParseBoardBrief_IllegalRelationEnumDropped(t *testing.T) {
	brief := parseBriefOrFatal(t, `{"summary":"s","observations":[
		{"id":"o1","lane_id":1,"statement":"a","basis":"b","as_of_date":"2026-08-26"},
		{"id":"o2","lane_id":2,"statement":"c","basis":"d","as_of_date":"2026-08-25"}],
		"relationships":[
		{"lane_ids":[1,2],"type":"causal_chain","explanation":"非法枚举","confidence":"high","evidence_refs":[]},
		{"lane_ids":[1,2],"type":"unclear","explanation":"合法条","confidence":"ultra","evidence_refs":[]}],
		"uncertainties":[],"research_questions":[]}`, briefTestCards(), false)
	if len(brief.Relationships) != 1 || brief.Relationships[0].Type != RelationUnclear {
		t.Fatalf("illegal enum dropped, legal kept: %+v", brief.Relationships)
	}
	if brief.Relationships[0].Confidence != "low" {
		t.Fatalf("illegal confidence must default to low, got %q", brief.Relationships[0].Confidence)
	}
}

// M3.3 幽灵 lane：observation 逐条剔除；关系剔除幽灵 id 后不足 2 个有效 lane 则整条拒绝。
func TestParseBoardBrief_GhostLanes(t *testing.T) {
	brief := parseBriefOrFatal(t, `{"summary":"s","observations":[
		{"id":"o1","lane_id":1,"statement":"真实","basis":"b","as_of_date":"2026-08-26"},
		{"id":"o2","lane_id":999,"statement":"幽灵","basis":"b","as_of_date":"2026-08-26"}],
		"relationships":[
		{"lane_ids":[1,999],"type":"unclear","explanation":"幽灵后只剩1条","confidence":"low","evidence_refs":[]},
		{"lane_ids":[1,2,999],"type":"context_only","explanation":"剔除幽灵仍够2条","confidence":"low","evidence_refs":[]}],
		"uncertainties":[],"research_questions":[{"id":"q1","question":"问题","rationale":"理由","related_lane_ids":[2,888]}]}`,
		briefTestCards(), false)
	if len(brief.Observations) != 1 || brief.Observations[0].LaneID != 1 {
		t.Fatalf("ghost observation dropped, real kept: %+v", brief.Observations)
	}
	if len(brief.Relationships) != 1 {
		t.Fatalf("relationship reduced below 2 valid lanes must be dropped entirely: %+v", brief.Relationships)
	}
	rel := brief.Relationships[0]
	if len(rel.LaneIDs) != 2 || rel.LaneIDs[0] != 1 || rel.LaneIDs[1] != 2 {
		t.Fatalf("ghost lane id must be scrubbed from surviving relationship: %+v", rel.LaneIDs)
	}
	if q := brief.ResearchQuestions[0]; len(q.RelatedLaneIDs) != 1 || q.RelatedLaneIDs[0] != 2 {
		t.Fatalf("ghost related_lane_id scrubbed from questions: %+v", q.RelatedLaneIDs)
	}
	// lane_refs derived from surviving observations must be ghost-free too.
	for _, lr := range brief.LaneRefs {
		if lr.LaneID == 999 || lr.LaneID == 888 {
			t.Fatalf("lane_refs leaked ghost lane: %+v", brief.LaneRefs)
		}
	}
}

// M3.4/M3.5 悬空 evidence_ref 剔除；possible_causal 无有效依据降 unclear；
// possible_causal 置信不得为 high（缺环写入 uncertainties 由 LLM 完成）。
func TestParseBoardBrief_PossibleCausalBoundaries(t *testing.T) {
	brief := parseBriefOrFatal(t, `{"summary":"s","observations":[
		{"id":"o1","lane_id":1,"statement":"a","basis":"b","as_of_date":"2026-08-26"},
		{"id":"o2","lane_id":2,"statement":"c","basis":"d","as_of_date":"2026-08-25"}],
		"relationships":[
		{"lane_ids":[1,2],"type":"possible_causal","explanation":"全部引用悬空","confidence":"low","evidence_refs":["oX","oY"]},
		{"lane_ids":[1,2],"type":"possible_causal","explanation":"有有效引用但置信虚高","confidence":"high","evidence_refs":["o1"]},
		{"lane_ids":[1,2],"type":"context_only","explanation":"悬空引用被剔除","confidence":"low","evidence_refs":["oZ"]}],
		"uncertainties":[{"question":"传导环节?","why_uncertain":"中间环节缺失","needed_evidence":"链路数据"}],
		"research_questions":[]}`, briefTestCards(), false)
	if len(brief.Relationships) != 3 {
		t.Fatalf("boundary cases must keep the relationship entries: %+v", brief.Relationships)
	}
	downgraded, clamped, cleaned := brief.Relationships[0], brief.Relationships[1], brief.Relationships[2]
	if downgraded.Type != RelationUnclear {
		t.Fatalf("possible_causal with zero valid evidence refs must downgrade to unclear, got %s", downgraded.Type)
	}
	if clamped.Type != RelationPossibleCausal || clamped.Confidence != "medium" {
		t.Fatalf("possible_causal confidence must cap at medium, got %s/%s", clamped.Type, clamped.Confidence)
	}
	if len(cleaned.EvidenceRefs) != 0 {
		t.Fatalf("dangling evidence refs must be scrubbed: %+v", cleaned.EvidenceRefs)
	}
	if len(clamped.EvidenceRefs) != 1 || clamped.EvidenceRefs[0] != "o1" {
		t.Fatalf("valid evidence ref kept: %+v", clamped.EvidenceRefs)
	}
}

// M2.7 数量上限：observations ≤5、relationships ≤6、research_questions ≤4。
func TestParseBoardBrief_CountCaps(t *testing.T) {
	var sb strings.Builder
	sb.WriteString(`{"summary":"s","observations":[`)
	for i := 1; i <= 7; i++ {
		if i > 1 {
			sb.WriteString(",")
		}
		sb.WriteString(`{"id":"o` + itoa(i) + `","lane_id":1,"statement":"观察` + itoa(i) + `","basis":"b","as_of_date":"2026-08-26"}`)
	}
	sb.WriteString(`],"relationships":[`)
	for i := 0; i < 8; i++ {
		if i > 0 {
			sb.WriteString(",")
		}
		sb.WriteString(`{"lane_ids":[1,2],"type":"unclear","explanation":"e` + itoa(i) + `","confidence":"low","evidence_refs":[]}`)
	}
	sb.WriteString(`],"uncertainties":[],"research_questions":[`)
	for i := 1; i <= 5; i++ {
		if i > 1 {
			sb.WriteString(",")
		}
		sb.WriteString(`{"id":"q` + itoa(i) + `","question":"问题` + itoa(i) + `","rationale":"r","related_lane_ids":[1]}`)
	}
	sb.WriteString(`]}`)
	brief := parseBriefOrFatal(t, sb.String(), briefTestCards(), false)
	if len(brief.Observations) != boardBriefMaxObservations {
		t.Fatalf("observations cap = %d, got %d", boardBriefMaxObservations, len(brief.Observations))
	}
	if len(brief.Relationships) != boardBriefMaxRelationships {
		t.Fatalf("relationships cap = %d, got %d", boardBriefMaxRelationships, len(brief.Relationships))
	}
	if len(brief.ResearchQuestions) != boardBriefMaxQuestions {
		t.Fatalf("research questions cap = %d, got %d", boardBriefMaxQuestions, len(brief.ResearchQuestions))
	}
}

// 长度上限：summary/statement/explanation/question 超长按 rune 截断。
func TestParseBoardBrief_LengthCaps(t *testing.T) {
	long := strings.Repeat("长", 1000)
	brief := parseBriefOrFatal(t, `{"summary":"`+long+`","observations":[
		{"id":"o1","lane_id":1,"statement":"`+long+`","basis":"`+long+`","as_of_date":"2026-08-26"}],
		"relationships":[{"lane_ids":[1,2],"type":"unclear","explanation":"`+long+`","confidence":"low","evidence_refs":[]}],
		"uncertainties":[{"question":"`+long+`","why_uncertain":"w","needed_evidence":"n"}],
		"research_questions":[{"id":"q1","question":"`+long+`","rationale":"r","related_lane_ids":[1]}]}`,
		briefTestCards(), false)
	if got := len([]rune(brief.Summary)); got > boardBriefMaxSummaryRunes {
		t.Fatalf("summary rune cap exceeded: %d", got)
	}
	if got := len([]rune(brief.Observations[0].Statement)); got > boardBriefMaxStatementRunes {
		t.Fatalf("statement rune cap exceeded: %d", got)
	}
	if got := len([]rune(brief.Relationships[0].Explanation)); got > boardBriefMaxExplainRunes {
		t.Fatalf("explanation rune cap exceeded: %d", got)
	}
	if got := len([]rune(brief.ResearchQuestions[0].Question)); got > boardBriefMaxQuestionRunes {
		t.Fatalf("question rune cap exceeded: %d", got)
	}
}

// 结构不合格（触发重试判据）：summary 空 / 非全 sparse 却零有效观察。
func TestParseBoardBrief_StructuralInvalid(t *testing.T) {
	cases := []struct {
		name string
		json string
	}{
		{"missing summary", `{"observations":[{"id":"o1","lane_id":1,"statement":"a","basis":"b","as_of_date":"2026-08-26"}],"relationships":[],"uncertainties":[],"research_questions":[]}`},
		{"no valid observations with material", `{"summary":"s","observations":[{"id":"o1","lane_id":999,"statement":"幽灵","basis":"b","as_of_date":"2026-08-26"}],"relationships":[],"uncertainties":[],"research_questions":[]}`},
		{"missing observations field with material", `{"summary":"s","relationships":[],"uncertainties":[],"research_questions":[]}`},
	}
	for _, c := range cases {
		parsed, err := ParseJSONResponse(c.json)
		if err != nil {
			t.Fatalf("%s: parse json: %v", c.name, err)
		}
		if _, ok := parseBoardBrief(parsed, briefTestCards(), false); ok {
			t.Fatalf("%s: must be structurally invalid (retry signal)", c.name)
		}
	}
}

// M2.6 机械降级：只从真实 cards 生成克制 observations/uncertainties，
// 不造关系、不造研究问题；basis 引用事实来源与截止日期而非质量分。
func TestMechanicalBoardBrief_FallbackShape(t *testing.T) {
	cards := briefTestCards()
	cards = append(cards, LaneSituationCard{LaneID: 9, Label: "空泳道", FactsSource: "none", LastSeenDate: "2026-08-01"})
	brief := mechanicalBoardBrief(cards, false, "两次输出不可解析")
	if brief.Degraded != true || brief.DegradedWhy == "" {
		t.Fatalf("fallback must be marked degraded: %+v", brief)
	}
	if len(brief.Observations) != 3 {
		t.Fatalf("observations only from cards with real digests, got %d", len(brief.Observations))
	}
	for i, obs := range brief.Observations {
		if obs.LaneID != cards[i].LaneID {
			t.Fatalf("fallback keeps quality order: %d vs %d", obs.LaneID, cards[i].LaneID)
		}
		if obs.Statement != cards[i].FactsDigest {
			t.Fatalf("statement must be the real digest: %q", obs.Statement)
		}
		if strings.Contains(obs.Basis, "质量") || strings.Contains(obs.Basis, "QualityScore") {
			t.Fatalf("quality score must not act as evidence text: %q", obs.Basis)
		}
		if !strings.Contains(obs.Basis, cards[i].FactsSource) || !strings.Contains(obs.Basis, cards[i].LastSeenDate) {
			t.Fatalf("basis must cite facts source and as-of date: %q", obs.Basis)
		}
	}
	if len(brief.Relationships) != 0 {
		t.Fatalf("fallback must not invent relationships: %+v", brief.Relationships)
	}
	if len(brief.ResearchQuestions) != 0 {
		t.Fatalf("fallback must not invent research questions: %+v", brief.ResearchQuestions)
	}
	if len(brief.Uncertainties) != 1 || !strings.Contains(brief.Uncertainties[0].Question, "空泳道") {
		t.Fatalf("one honest uncertainty per no-material lane: %+v", brief.Uncertainties)
	}
}

// M2.4 机械降级全 sparse 变体：零观察、诚实 summary、零问题。
func TestMechanicalBoardBrief_AllSparse(t *testing.T) {
	cards := []LaneSituationCard{{LaneID: 7, Label: "空泳道", FactsSource: "none", LastSeenDate: "2026-08-01"}}
	brief := mechanicalBoardBrief(cards, true, "两次输出不可解析")
	if len(brief.Observations) != 0 || len(brief.ResearchQuestions) != 0 {
		t.Fatalf("all-sparse fallback must stay empty: %+v", brief)
	}
	if !strings.Contains(brief.Summary, "素材") {
		t.Fatalf("summary must honestly state thin material: %q", brief.Summary)
	}
}

// M2.8 prompt 契约：含态势卡与关系枚举纪律；无工具说明、无方法卡全文、
// 无「不是A而是B」式立论要求、无强制机制层/历史类比/系统重定位。
func TestAssembleBoardBriefPrompt_Contract(t *testing.T) {
	in := boardBriefInput{
		CardsMD: "## 泳道态势卡\n- 泳道#1《泳道一》事实: 事实甲",
		Cards:   briefTestCards(),
	}
	prompt := assembleBoardBriefPrompt(in)
	for _, want := range []string{
		"泳道态势卡",                // 输入块
		"事实甲",                  // 卡内容透传
		RelationCommonDriver,   // 枚举语义说明
		RelationPossibleCausal, //
		RelationDivergent,      //
		RelationContextOnly,    //
		RelationUnclear,        //
		"暂未发现统一关系",             // 无统一关系是正常结论
		"research_questions",   // schema 契约
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("board brief prompt missing %q", want)
		}
	}
	for _, banned := range []string{
		"可用工具", "web_search", "fetch_page", // 无工具说明
		"分析方法参考", "方法卡", // 无方法卡
		"而是", "thesis", // 无反转句式/命题要求
		"机制层", "历史类比", "系统重定位", // 无固定深度结构
	} {
		if strings.Contains(prompt, banned) {
			t.Fatalf("board brief prompt must not contain %q", banned)
		}
	}
	// Review digest 只在非空时注入；空值不出现区块标题。
	if strings.Contains(prompt, "历史认知") {
		t.Fatal("empty review digest must not render a section")
	}
	in.ReviewDigest = "- [review #3] 曾把共现误判为因果"
	prompt = assembleBoardBriefPrompt(in)
	if !strings.Contains(prompt, "曾把共现误判为因果") {
		t.Fatal("review digest must be injectable (3.5 wiring point)")
	}
}

// M2.5 坏 JSON → 纠错重试一次 → 仍坏走机械降级；调用恰好 2 次、全为 board_brief。
func TestGenerateBoardBrief_BadJSONRetriesThenDegrades(t *testing.T) {
	router := &internalMockRouter{responses: []*airouter.ChatResult{
		{Content: "这不是JSON"},
		{Content: `{"summary": 还是坏`},
	}}
	orch := &OrchestratorService{airouter: router, capability: internalTestCap}
	brief, meta := orch.generateBoardBrief(context.Background(), boardBriefInput{
		SessionID: "brief-sess", CardsMD: "cards", Cards: briefTestCards(),
	})
	if len(router.calls) != 2 {
		t.Fatalf("bad JSON must retry exactly once (2 calls), got %d", len(router.calls))
	}
	for i, c := range router.calls {
		if c.Operation != boardBriefOperation {
			t.Fatalf("call %d operation = %q, want %q", i, c.Operation, boardBriefOperation)
		}
		if c.SessionID != "brief-sess" {
			t.Fatalf("call %d session id: %q", i, c.SessionID)
		}
	}
	if !meta.Degraded || meta.Attempts != 2 {
		t.Fatalf("meta must record degrade after 2 attempts: %+v", meta)
	}
	if !brief.Degraded || len(brief.Observations) == 0 {
		t.Fatalf("mechanical fallback brief expected: %+v", brief)
	}
	// 重试 prompt 携带纠错说明。
	if !strings.Contains(router.calls[1].Messages[0].Content, boardBriefRetryLead) {
		t.Fatal("retry prompt must carry the corrective note")
	}
}

// 坏→好：第二次成功则用 LLM 结果，不降级。
func TestGenerateBoardBrief_BadThenGood(t *testing.T) {
	router := &internalMockRouter{responses: []*airouter.ChatResult{
		{Content: "garbage"},
		{Content: `{"summary":"第二次成功。","observations":[{"id":"o1","lane_id":1,"statement":"观察","basis":"周摘要","as_of_date":"2026-08-26"}],"relationships":[],"uncertainties":[],"research_questions":[]}`},
	}}
	orch := &OrchestratorService{airouter: router, capability: internalTestCap}
	brief, meta := orch.generateBoardBrief(context.Background(), boardBriefInput{
		SessionID: "brief-sess", CardsMD: "cards", Cards: briefTestCards(),
	})
	if len(router.calls) != 2 || meta.Degraded {
		t.Fatalf("second-attempt success must be used: calls=%d meta=%+v", len(router.calls), meta)
	}
	if brief.Degraded || brief.Summary != "第二次成功。" {
		t.Fatalf("brief from LLM expected: %+v", brief)
	}
	// sectors.retry_reason 与 input_snapshot.generation.retry_reason 一致留痕。
	if meta.RetryReason == "" || brief.RetryReason != meta.RetryReason {
		t.Fatalf("retry-success brief must carry the same retry_reason as generation meta: brief=%q meta=%q", brief.RetryReason, meta.RetryReason)
	}
}

// 合法一次：正常路径只调 1 次 LLM。
func TestGenerateBoardBrief_SingleCallOnSuccess(t *testing.T) {
	router := &internalMockRouter{responses: []*airouter.ChatResult{
		{Content: `{"summary":"ok","observations":[{"id":"o1","lane_id":1,"statement":"观察","basis":"周摘要","as_of_date":"2026-08-26"}],"relationships":[],"uncertainties":[],"research_questions":[]}`},
	}}
	orch := &OrchestratorService{airouter: router, capability: internalTestCap}
	brief, meta := orch.generateBoardBrief(context.Background(), boardBriefInput{
		SessionID: "brief-sess", CardsMD: "cards", Cards: briefTestCards(),
	})
	if len(router.calls) != 1 || meta.Attempts != 1 || meta.Degraded {
		t.Fatalf("happy path is one call: calls=%d meta=%+v", len(router.calls), meta)
	}
	if brief.Degraded {
		t.Fatal("happy path brief must not be degraded")
	}
	if brief.RetryReason != "" || meta.RetryReason != "" {
		t.Fatalf("first-attempt success must leave both retry_reason empty: brief=%q meta=%q", brief.RetryReason, meta.RetryReason)
	}
}

// itoa avoids strconv import in test string building.
func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var b []byte
	for i > 0 {
		b = append([]byte{byte('0' + i%10)}, b...)
		i /= 10
	}
	return string(b)
}
