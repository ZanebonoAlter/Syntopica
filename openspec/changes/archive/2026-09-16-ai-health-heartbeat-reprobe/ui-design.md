<!-- ui-impact: none -->
<!-- ui-approval: not-required -->
<!-- ui-prototype: none -->

## N/A Reason

本 change 只调整后端健康探测器的循环策略（心跳降级与去抖），无任何用户可见界面结构、文案或交互变化。健康降级态的呈现完全复用现有 AiHealthBanner 与 `/schedulers/status.ai_healthy` 字段，前端零改动。依据 proposal 的 `ui-impact: none` 声明，不制作原型。
