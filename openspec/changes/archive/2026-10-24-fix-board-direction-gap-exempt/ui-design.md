<!-- ui-impact: none -->
<!-- ui-approval: not-required -->
<!-- ui-prototype: none -->

# UI Design: fix-board-direction-gap-exempt

## N/A（ui-impact: none）

纯后端判定逻辑 change（direction_mismatch 豁免扩围 + 配置键 + 存量重算），无任何前端改动：

- 前端消费的 `direction_mismatch` 字段语义与 DTO 不变，仅标记值分布变化（mismatch 数下降）——体检面板/挂载列表默认隐藏行为照旧，属数据变化非 UI 契约变化。
- 布局/组件/交互零改动，无 `ui-prototype/` 需求。
