<!-- complexity: simple -->
<!-- ui-impact: minor -->
<!-- constraint-domains: semantic-board -->

## Why

升级建议弹窗的「候选时间窗」目前只在「创建版块 × 单标签」一格显示，选「版块扩充」方向时控件隐藏、后端显式忽略 days 参数（`semantic_board_expand.go:56` 注释）。但扩充候选召回同样有时间维度需求：用户只想看「近期活跃」的扩充候选，而不是全库语义/固定共现窗口捞出来的陈旧候选。既然同一个弹窗里已有现成控件，放开限制让扩充方向也能用，是最小代价的功能补齐。

## What Changes

- 前端 `UpgradeSuggestionPanel.vue`：「候选时间窗」下拉的 `v-if` 从 `create×aux` 单格放开到全部四格（创建×单标签 / 创建×组合 / 扩充×单标签 / 扩充×组合），选扩充方向时请求体携带 days
- 后端扩充×单标签召回（`recallExpandAuxCandidates`）：
  - 相似路结果集增加「近 N 天有文章引用该标签」过滤（复用 create×aux 已有的 `EXISTS (article_topic_tags → articles.created_at >= cutoff)` 子查询模式）
  - 共现路窗口收紧：cutoff 取 `UI days` 与全局配置 `CoTagWindowDays` 中更严者（days=0 不收紧，等于现状）
- 后端扩充×组合召回（`generateExpandCompose`）：共现窗口同样按上述规则收紧
- days=0（全部）= 不过滤 = 与现状完全一致，天然向后兼容
- 定时任务 `board_upgrade_suggest` 不受影响（仅跑创建方向，扩充纯手动触发——红线 10）

## Capabilities

**Modified Capabilities:**

- `upgrade-candidate-time-window`：时间窗契约从「创建×单标签路」扩展到「扩充路」——新增扩充方向各召回路的过滤行为与 Scenario；API 参数语义不变（`days` 查询参数对扩充 generate 请求同样生效）
- `board-upgrade-expand`：扩充候选召回需求增加时间窗过滤条款（相似路近期活跃过滤 + 共现路窗口收紧）

**New Capabilities:** 无

## Impact

- 前端：`front/app/features/tags/components/UpgradeSuggestionPanel.vue`（v-if 放开 + emit 携带 days）、`useTagsPage.ts` / `semanticBoards.ts`（generate 请求参数）
- 后端：`backend-go/internal/tagmanagement/handler/board_upgrade_handler.go`（days 解析透传）、`service/board/semantic_board_upgrade.go`（请求结构）、`semantic_board_expand.go`（两路召回 + compose 路过滤）
- 无表结构变更、无 API 形状变更（`days` 早已是查询参数，只是扩充路开始消费）
- 测试：扩充召回的过滤行为用例（days 收紧/放开的候选集差异）
