package airouter

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func strPtr(s string) *string { return &s }

func TestIsValidThinkingLevel(t *testing.T) {
	for _, level := range ValidThinkingLevels {
		require.True(t, IsValidThinkingLevel(level), "七档值 %q 必须合法", level)
	}
	for _, invalid := range []string{"", "ultra", "high ", "HIGH", "off,low"} {
		require.False(t, IsValidThinkingLevel(invalid), "非法值 %q 必须被拒绝", invalid)
	}
}

// TestEffectiveThinkingLevel 覆盖 test-cases W2 六行（四级解析优先级）：
// 请求级 > 绑定级 > 模型默认 > 全局默认（medium）；空串/nil 视为未指定逐级回落；
// 非法值运行时按未指定处理（防御回落）；钳制不发生在解析阶段。
func TestEffectiveThinkingLevel(t *testing.T) {
	cases := []struct {
		name    string
		req     *string
		binding *string
		model   *string
		want    string
	}{
		{"W2-1 请求级压一切", strPtr("high"), strPtr("low"), strPtr("off"), "high"},
		{"W2-2 绑定级压过模型默认", nil, strPtr("high"), strPtr("medium"), "high"},
		{"W2-3 绑定未覆盖用模型默认", strPtr(""), strPtr(""), strPtr("off"), "off"},
		{"W2-4 全未指定用全局默认", nil, nil, nil, "medium"},
		{"W2-5 解析阶段不钳制 xhigh", strPtr("xhigh"), strPtr("low"), nil, "xhigh"},
		{"W2-6 非法绑定级回落（防御）", nil, strPtr("ultra"), nil, "medium"},
		{"空串与 nil 等价", strPtr(""), strPtr(""), strPtr(""), "medium"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, effectiveThinkingLevel(tc.req, tc.binding, tc.model))
		})
	}
}

// TestClampLevel 覆盖钳制边界（specs/ai-thinking-levels「xhigh/max 可用性钳制」）：
// 未声明支持 → xhigh/max 降 high；声明支持 → 全档放行。
func TestClampLevel(t *testing.T) {
	cases := []struct {
		level            string
		supportsXhighMax bool
		want             string
	}{
		{ThinkingOff, false, ThinkingOff},
		{ThinkingMedium, false, ThinkingMedium},
		{ThinkingHigh, false, ThinkingHigh},
		{ThinkingXhigh, false, ThinkingHigh},
		{ThinkingMax, false, ThinkingHigh},
		{ThinkingXhigh, true, ThinkingXhigh},
		{ThinkingMax, true, ThinkingMax},
		{ThinkingLow, true, ThinkingLow},
	}
	for _, tc := range cases {
		got := clampLevel(tc.level, tc.supportsXhighMax)
		require.Equal(t, tc.want, got, "clampLevel(%q, %v)", tc.level, tc.supportsXhighMax)
	}
}

// TestThinkingToWireMatrix 覆盖 test-cases W1 矩阵：4 方言 × 7 档全部 28 格
// （supportsXhighMax=false，xhigh/max 先钳 high 再映射）。nil=不发任何思考参数。
func TestThinkingToWireMatrix(t *testing.T) {
	qwenTrue := map[string]any{"chat_template_kwargs": map[string]any{"enable_thinking": true}}
	qwenFalse := map[string]any{"chat_template_kwargs": map[string]any{"enable_thinking": false}}
	deepseekEnabled := map[string]any{"thinking": map[string]any{"type": "enabled"}}
	deepseekDisabled := map[string]any{"thinking": map[string]any{"type": "disabled"}}

	cases := []struct {
		name    string
		dialect string
		level   string
		want    map[string]any
	}{
		// qwen-chat-template：全档显式发布尔（off→false，minimal..high→true，xhigh/max 钳 high→true）
		{"qwen/off", "qwen-chat-template", ThinkingOff, qwenFalse},
		{"qwen/minimal", "qwen-chat-template", ThinkingMinimal, qwenTrue},
		{"qwen/low", "qwen-chat-template", ThinkingLow, qwenTrue},
		{"qwen/medium", "qwen-chat-template", ThinkingMedium, qwenTrue},
		{"qwen/high", "qwen-chat-template", ThinkingHigh, qwenTrue},
		{"qwen/xhigh 钳 high", "qwen-chat-template", ThinkingXhigh, qwenTrue},
		{"qwen/max 钳 high", "qwen-chat-template", ThinkingMax, qwenTrue},
		// deepseek：off→disabled，其余→enabled（xhigh/max 钳 high→enabled）
		{"deepseek/off", "deepseek", ThinkingOff, deepseekDisabled},
		{"deepseek/minimal", "deepseek", ThinkingMinimal, deepseekEnabled},
		{"deepseek/low", "deepseek", ThinkingLow, deepseekEnabled},
		{"deepseek/medium", "deepseek", ThinkingMedium, deepseekEnabled},
		{"deepseek/high", "deepseek", ThinkingHigh, deepseekEnabled},
		{"deepseek/xhigh 钳 high", "deepseek", ThinkingXhigh, deepseekEnabled},
		{"deepseek/max 钳 high", "deepseek", ThinkingMax, deepseekEnabled},
		// reasoning-effort：off→不发字段，minimal..high 直传，xhigh/max 钳 high
		{"effort/off 不发", "reasoning-effort", ThinkingOff, nil},
		{"effort/minimal", "reasoning-effort", ThinkingMinimal, map[string]any{"reasoning_effort": "minimal"}},
		{"effort/low", "reasoning-effort", ThinkingLow, map[string]any{"reasoning_effort": "low"}},
		{"effort/medium", "reasoning-effort", ThinkingMedium, map[string]any{"reasoning_effort": "medium"}},
		{"effort/high", "reasoning-effort", ThinkingHigh, map[string]any{"reasoning_effort": "high"}},
		{"effort/xhigh 钳 high", "reasoning-effort", ThinkingXhigh, map[string]any{"reasoning_effort": "high"}},
		{"effort/max 钳 high", "reasoning-effort", ThinkingMax, map[string]any{"reasoning_effort": "high"}},
		// none：恒不发
		{"none/off", "none", ThinkingOff, nil},
		{"none/minimal", "none", ThinkingMinimal, nil},
		{"none/low", "none", ThinkingLow, nil},
		{"none/medium", "none", ThinkingMedium, nil},
		{"none/high", "none", ThinkingHigh, nil},
		{"none/xhigh", "none", ThinkingXhigh, nil},
		{"none/max", "none", ThinkingMax, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, thinkingToWire(tc.dialect, tc.level, false))
		})
	}
}

// TestThinkingToWireDeclaredSupport 验证声明支持 xhigh/max 的模型全档放行：
// 钳制短路，档位原值直达方言映射。
func TestThinkingToWireDeclaredSupport(t *testing.T) {
	require.Equal(t,
		map[string]any{"reasoning_effort": "xhigh"},
		thinkingToWire("reasoning-effort", ThinkingXhigh, true))
	require.Equal(t,
		map[string]any{"reasoning_effort": "max"},
		thinkingToWire("reasoning-effort", ThinkingMax, true))
	// qwen 只有布尔档，xhigh/max 直达仍映射 true
	require.Equal(t,
		map[string]any{"chat_template_kwargs": map[string]any{"enable_thinking": true}},
		thinkingToWire("qwen-chat-template", ThinkingMax, true))
	// off 不受钳制影响
	require.Equal(t,
		map[string]any{"thinking": map[string]any{"type": "disabled"}},
		thinkingToWire("deepseek", ThinkingOff, true))
}

// TestThinkingToWireUnknownDialect 未知方言按 none 处理（不发思考参数，防御）。
func TestThinkingToWireUnknownDialect(t *testing.T) {
	require.Nil(t, thinkingToWire("", ThinkingHigh, false))
	require.Nil(t, thinkingToWire("mistral", ThinkingHigh, false))
}
