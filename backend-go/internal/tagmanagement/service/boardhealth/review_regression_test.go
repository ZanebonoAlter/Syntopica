package boardhealth

import (
	"encoding/json"
	"math"
	"testing"

	"github.com/stretchr/testify/require"
)

// 用例来源：review.md A-L1 / A-L2（seam A 独立审查 Low 项）的最小新增回归。
// 纯函数层，无 DB——按任务12授权只补这两组，不替代/不重跑既有门禁。

// TestParseFloatDefaultNonFiniteGuard —— A-L1：parseFloatDefault 必须拒收
// 非有限值（NaN 会绕过 0..100 数值比较被静默当作阈值），回退传入
// fallback；正常数值（含 0 与 100 边界）行为不变。
func TestParseFloatDefaultNonFiniteGuard(t *testing.T) {
	const fallback = 0.08
	cases := []struct {
		name  string
		value string
		want  float64 // 期望返回值；ok=false 时应为 fallback
		ok    bool
	}{
		// NaN：ParseFloat 大小写不敏感、可带符号/首尾空白——全部非有限。
		{"NaN", "NaN", fallback, false},
		{"nan 小写", "nan", fallback, false},
		{"NAN 全大写", "NAN", fallback, false},
		{"带符号 NaN", "-nan", fallback, false},
		{"首尾空白 NaN", "  NaN\t", fallback, false},
		// ±Inf / Infinity：同为非有限，一律拒收。
		{"Inf", "Inf", fallback, false},
		{"+Inf", "+Inf", fallback, false},
		{"-Inf", "-Inf", fallback, false},
		{"infinity 大小写混合", "-INFINITY", fallback, false},
		{"正 infinity 带空格", " Infinity ", fallback, false},
		// 正常边界与典型值不变。
		{"下边界 0", "0", 0, true},
		{"默认 gap", "0.08", 0.08, true},
		{"上边界 100", "100", 100, true},
		{"区间内", "45.5", 45.5, true},
		// 既有越界/坏文本回退不变。
		{"越下界", "-0.01", fallback, false},
		{"越上界", "100.01", fallback, false},
		{"负数", "-1", fallback, false},
		{"坏文本", "abc", fallback, false},
		{"空串", "", fallback, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := parseFloatDefault(tc.value, fallback)
			if !tc.ok {
				require.Equal(t, fallback, got, "must fall back on %q", tc.value)
				return
			}
			require.Equal(t, tc.want, got)
		})
	}
	// 防夹具漂移：fallback 本身有限。
	require.False(t, math.IsNaN(fallback))
}

// TestConvertTriageClearsNonRewriteSuggestion —— A-L2：convertTriage 在
// 持久化转换时清空非 rewrite 类别的多余 suggestion（rewrite 保留）；不因
// 多余文案丢弃合法类别/证据；输入 slice 原值保持；JSON 省略非 rewrite 的
// suggestion（models 侧 omitempty 契约）。
func TestConvertTriageClearsNonRewriteSuggestion(t *testing.T) {
	input := []TriageItem{
		{AuxID: 1, Category: "keep", Evidence: "cosine_mean=0.7 绝对健康", Suggestion: "多余文案"},
		{AuxID: 2, Category: "remove", Evidence: "magnet failed=0.52 all=0.68 failed_n=40", Suggestion: "不该有的建议"},
		{AuxID: 3, Category: "insufficient", Evidence: "failed_n=2 低于最小样本量", Suggestion: "另一段多余"},
		{AuxID: 4, Category: "rewrite", Evidence: "desc 预筛命中此次", Suggestion: "中国东部直辖市"},
	}

	out := convertTriage(input)
	require.Len(t, out, 4)

	byAux := map[uint]int{}
	for i, item := range out {
		byAux[item.AuxID] = i
	}
	// 非 rewrite：suggestion 清空，类别/证据原样保留。
	for _, auxID := range []uint{1, 2, 3} {
		item := out[byAux[auxID]]
		require.Empty(t, item.Suggestion, "aux %d: non-rewrite suggestion must be cleared", auxID)
	}
	require.Equal(t, "keep", out[byAux[1]].Category)
	require.Equal(t, "cosine_mean=0.7 绝对健康", out[byAux[1]].Evidence)
	require.Equal(t, "remove", out[byAux[2]].Category)
	require.Equal(t, "magnet failed=0.52 all=0.68 failed_n=40", out[byAux[2]].Evidence)
	require.Equal(t, "insufficient", out[byAux[3]].Category)
	require.Equal(t, "failed_n=2 低于最小样本量", out[byAux[3]].Evidence)
	// rewrite：有效文案保留。
	require.Equal(t, "rewrite", out[byAux[4]].Category)
	require.Equal(t, "中国东部直辖市", out[byAux[4]].Suggestion)

	// JSON 契约：非 rewrite 条目省略 suggestion 键，rewrite 携带。
	keepJSON, err := json.Marshal(out[byAux[1]])
	require.NoError(t, err)
	require.NotContains(t, string(keepJSON), "suggestion")
	rewriteJSON, err := json.Marshal(out[byAux[4]])
	require.NoError(t, err)
	require.Contains(t, string(rewriteJSON), `"suggestion":"中国东部直辖市"`)

	// 输入 slice 不被改写（转换只动输出副本）。
	require.Equal(t, "多余文案", input[0].Suggestion)
	require.Equal(t, "不该有的建议", input[1].Suggestion)
	require.Equal(t, "另一段多余", input[2].Suggestion)
	require.Equal(t, "中国东部直辖市", input[3].Suggestion)
}
