<!-- ui-impact: none -->
<!-- ui-approval: not-required -->
<!-- ui-prototype: none -->

# UI 设计契约（none：N/A）

本 change 为 pi 扩展（`.pi/extensions/constraint-injection.ts`）内部状态机修复，涉及面：

- Syntopica 前端（`front/`）：零改动，N/A。
- TUI 状态栏 widget（constraint-injection 的 `ctx.ui.setWidget`）：不改展示内容与触发时机，N/A。
- steer 消息（动态层投递形态）：不改消息结构、customType、display 策略；修复仅**减少**重复消息条数（同内容第二次投递不再发生），属行为收敛而非 UI 变化。

无新增页面/弹窗/导航/交互模式，无受影响前端状态，无复用组件与布局模式变更。
