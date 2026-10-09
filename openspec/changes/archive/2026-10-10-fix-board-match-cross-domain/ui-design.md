<!-- ui-impact: none -->
<!-- ui-approval: not-required -->
<!-- ui-prototype: none -->

## N/A Reason

纯后端匹配与日报收集逻辑变更（`semantic_board_matching.go` / `daily_report_orchestrator.go` / ai_settings 配置项），无任何前端页面、组件或交互改动，与 proposal 的 `ui-impact: none` 声明一致。matching-config 新增 key 虽经既有 API 透出，但沿用现有 `MatchingConfigDialog.vue` 的动态 key 渲染机制，不涉及 UI 契约变化。
