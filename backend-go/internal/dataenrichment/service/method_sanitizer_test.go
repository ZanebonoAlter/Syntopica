package service

import (
	"encoding/json"
	"strings"
	"testing"
)

// ── M7.7 修辞清洗：确定性行级过滤，宁漏勿错 ──────────────────────────────────
//
// 用例清单（方法注入边界加固切片）：
//   固定金句/强制不是而是/人格口吻指令行 → 对应原因码剔除
//   普通证据步骤/描述性使用 → 保留
//   否定性边界说明（禁止/不要/避免…）与中性语气纪律 → 保留（不反向误杀）
//   stats 只含计数与原因码，被过滤原文不落任何结构体

func TestMethodSanitizer_FiltersHighConfidenceRhetoricLines(t *testing.T) {
	cases := []struct {
		line string
		want string
	}{
		// 固定金句/套话指令
		{"每段结尾插入一句金句升华主题", methodFilterFixedPhrase},
		{"- 每篇收尾固定使用同一句开场白", methodFilterFixedPhrase},
		{"End every section with a signature punchline", methodFilterFixedPhrase},
		// 强制「不是…而是…」句式指令（省略号模板/引号模板 + 明确修辞/模板信号）
		{"用「不是产能过剩，而是需求重构」的句式改写每段结论", methodFilterForcedPattern},
		{"把结论写成“不是被动调整，而是主动下注”的固定句式", methodFilterForcedPattern},
		{"每段必须写成不是复苏而是再平衡", methodFilterForcedPattern},
		{"Use the 'not collapse but repositioning' pattern in the closing", methodFilterForcedPattern},
		// 模仿具体作者/人格/口吻/情绪/立场
		{"模仿《内部看美国》作者的口吻写作", methodFilterPersona},
		{"以基金经理的口吻陈述观点", methodFilterPersona},
		{"写作时带着愤怒和讽刺的情绪", methodFilterPersona},
		{"Write in the style of a provocative columnist", methodFilterPersona},
	}
	for _, c := range cases {
		if got := methodLineFilterReason(c.line); got != c.want {
			t.Errorf("line %q: want reason %q, got %q", c.line, c.want, got)
		}
	}
}

func TestMethodSanitizer_OrdinaryEvidenceStepsSurvive(t *testing.T) {
	survive := []string{
		"步骤一：列出传导链每一环的证据来源，逐一核对时间先后",
		"对照相似机制的历史案例，标注时间范围与差异",
		"这不是产能问题而是需求问题", // 描述性使用：无句式/写成等风格元语言 → 不删
		"结论必须能被证据削弱，不能被削弱的叙事直接淘汰",
		"Check two independent sources before drawing conclusions",
	}
	for _, line := range survive {
		if got := methodLineFilterReason(line); got != "" {
			t.Errorf("ordinary evidence line %q must survive, got reason %q", line, got)
		}
	}
}

// 普通证据纪律/中性语气要求不是修辞模仿：persona/forced_pattern 规则收敛后
// 这些行必须一字不删地保留（误删证据纪律的代价高于漏过修辞指令）。
func TestMethodSanitizer_EvidenceDisciplineAndNeutralToneSurvive(t *testing.T) {
	survive := []string{
		// 中性语气纪律：不是模仿具体作者/人格
		"以审慎的语气标注每条证据的不确定性",
		"用严谨的语气陈述已核实结论，未核实的单独标注",
		"用平稳的语气复述官方口径原文，不添加判断",
		"Always adopt a neutral and objective tone",
		// forced_pattern signal 收窄：宽泛词（表述/框架）单独出现不触发
		"结论表述中需区分：这不是复苏而是再平衡，两者证据不同",
		"用因果框架检验「不是A而是B」式流行叙事是否成立",
	}
	for _, line := range survive {
		if got := methodLineFilterReason(line); got != "" {
			t.Errorf("evidence discipline / neutral tone line %q must survive, got reason %q", line, got)
		}
	}
}

func TestMethodSanitizer_NegativeBoundaryAndNeutralToneSurvive(t *testing.T) {
	// 否定性边界说明：主题词命中但语义是禁用/劝阻 → 保留，绝不反向误杀。
	survive := []string{
		"禁止使用固定金句和套话",
		"不要使用「不是…而是…」的对仗句式",
		"避免模仿任何作者的人格与口吻",
		"不要立场鲜明地写作，观点留待证据",
		"保持客观中性的语气，不带情绪",
		"Do not use the 'not X but Y' pattern",
	}
	for _, line := range survive {
		if got := methodLineFilterReason(line); got != "" {
			t.Errorf("negative boundary line %q must survive, got reason %q", line, got)
		}
	}
}

func TestMethodSanitizer_ContentLevelStatsAndNoTextLeak(t *testing.T) {
	personaLine := "模仿《内部看美国》作者的口吻写作"
	goldenLine := "每段结尾插入一句金句升华主题"
	stepLine := "步骤一：列出传导链每一环的证据来源"
	content := stepLine + "\n" + personaLine + "\n- " + goldenLine

	cleaned, stats := sanitizeAnalysisMethodContent(content)
	if stats.FilteredLines != 2 {
		t.Fatalf("want 2 filtered lines, got %d", stats.FilteredLines)
	}
	if got := stats.ReasonCounts[methodFilterPersona]; got != 1 {
		t.Fatalf("persona count = %d, want 1", got)
	}
	if got := stats.ReasonCounts[methodFilterFixedPhrase]; got != 1 {
		t.Fatalf("fixed_phrase count = %d, want 1", got)
	}
	// 列表前缀行被剥掉前缀参与匹配，保留行原样注入。
	if cleaned != stepLine {
		t.Fatalf("cleaned content must keep only the evidence step verbatim, got %q", cleaned)
	}
	// stats 落 JSON 只有计数与原因码，绝不携带被删原文。
	blob, err := json.Marshal(stats)
	if err != nil {
		t.Fatalf("marshal stats: %v", err)
	}
	for _, banned := range []string{personaLine, goldenLine, "模仿", "金句"} {
		if strings.Contains(string(blob), banned) {
			t.Fatalf("sanitize stats must not carry filtered text %q: %s", banned, blob)
		}
	}
}
