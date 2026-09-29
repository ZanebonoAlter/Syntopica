package airouter

// 思考档七档体系（ai-provider-model-overhaul，specs/ai-thinking-levels）：
// 固定值域 off/minimal/low/medium/high/xhigh/max，全局默认 medium，取代原
// enable_thinking 布尔的二值语义。值域扩展属跨域语义变更，不得随手加档。

const (
	ThinkingOff     = "off"
	ThinkingMinimal = "minimal"
	ThinkingLow     = "low"
	ThinkingMedium  = "medium"
	ThinkingHigh    = "high"
	ThinkingXhigh   = "xhigh"
	ThinkingMax     = "max"
)

// DefaultThinkingLevel 是全局默认思考档（specs/ai-thinking-levels「七档值域与全局默认」）。
const DefaultThinkingLevel = ThinkingMedium

// ValidThinkingLevels 是合法思考档全列表（校验错误文案与值域判定共用，保持同序）。
var ValidThinkingLevels = []string{
	ThinkingOff, ThinkingMinimal, ThinkingLow, ThinkingMedium,
	ThinkingHigh, ThinkingXhigh, ThinkingMax,
}

// IsValidThinkingLevel 判定 s 是否合法七档值。
func IsValidThinkingLevel(s string) bool {
	for _, valid := range ValidThinkingLevels {
		if s == valid {
			return true
		}
	}
	return false
}

// effectiveThinkingLevel 按四级优先级解析单次调用的生效思考档：
// 请求级 > capability 绑定级 > 模型默认档 > 全局默认（medium）。
// 空串与 nil 一律视为「未指定」逐级回落；非法值（如 ultra）在运行时按未指定
// 处理（防御：保存时已被 UpsertRouteModels 拒绝，正常数据不会走到这里）。
// 注意：钳制不在此处发生——解析之后、wire 之前才按模型支持度钳制（见 clampLevel）。
func effectiveThinkingLevel(reqLevel, bindingLevel, modelLevel *string) string {
	for _, level := range []*string{reqLevel, bindingLevel, modelLevel} {
		if level != nil && IsValidThinkingLevel(*level) {
			return *level
		}
	}
	return DefaultThinkingLevel
}

// clampLevel 按模型声明支持度钳制档位：未声明支持 xhigh/max（supportsXhighMax=false）
// 时 xhigh/max 一律降为 high，其余档位原样。声明支持的模型全档放行。
func clampLevel(level string, supportsXhighMax bool) string {
	if supportsXhighMax {
		return level
	}
	switch level {
	case ThinkingXhigh, ThinkingMax:
		return ThinkingHigh
	default:
		return level
	}
}

// thinkingToWire 按供应商思考方言把生效档位翻译为请求参数（design D4 + test-cases W1 矩阵）。
// 返回的 map 直接附加到 chat payload；返回 nil 表示不发任何思考控制参数（none 方言、
// reasoning-effort 的 off 档）。映射前先按 supportsXhighMax 钳制（钳制发生在 wire 之前）。
//
//   - qwen-chat-template：全档显式发 chat_template_kwargs.enable_thinking——off→false，
//     其余→true。沿用 buildPayload 的既有理由：Qwen3/Qwythos 模板在 kwarg 缺省时默认
//     开启思考，省略 ≠ off，必须显式发 false 才能真正关闭（显式发 false 是语义保留点）。
//   - deepseek：off→thinking={"type":"disabled"}，其余→{"type":"enabled"}。
//   - reasoning-effort：off→不发该字段；minimal..high 直传档位字符串。
//   - none：恒不发（档位仅影响响应侧 <think> 清理）。
//   - 未知方言按 none 处理（不发），与 provider 未知类型跳过的防御姿态一致。
func thinkingToWire(dialect, level string, supportsXhighMax bool) map[string]any {
	level = clampLevel(level, supportsXhighMax)
	switch dialect {
	case "qwen-chat-template":
		return map[string]any{
			"chat_template_kwargs": map[string]any{"enable_thinking": level != ThinkingOff},
		}
	case "deepseek":
		thinkingType := "enabled"
		if level == ThinkingOff {
			thinkingType = "disabled"
		}
		return map[string]any{
			"thinking": map[string]any{"type": thinkingType},
		}
	case "reasoning-effort":
		if level == ThinkingOff {
			return nil
		}
		return map[string]any{"reasoning_effort": level}
	default: // "none" 及未知方言
		return nil
	}
}
