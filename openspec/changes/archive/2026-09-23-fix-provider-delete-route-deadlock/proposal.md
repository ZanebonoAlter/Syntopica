<!-- complexity: simple -->
<!-- ui-impact: minor -->
<!-- constraint-domains: ai-summary -->

## Why

AI provider 一旦被挂到任何能力线路（`ai_route_providers` 关联）上，删除接口就返回 409「provider is still used by one or more AI routes」；而页面上的解绑路径全部堵死（详见 design 的死锁分析），形成死锁：provider 永远无法删除，只能动数据库。用户已在实际使用中踩中此 bug。

## What Changes

- **后端 `DELETE /providers/:provider_id`**：删除 provider 前自动级联解绑——删除该 provider 在 `ai_route_providers` 中的全部关联记录，再删 provider 本身（同一事务）；响应 message 中告知摘除了几条线路关联。移除原 409 拦截。
- **空线路语义（用户已确认）**：级联解绑可能导致某能力线路一个 provider 都不剩，允许该状态存在；后端调用该能力时走既有的「无可用 provider」报错路径，行为可预期。`UpdateRoute` handler 维持现状（仍要求非空 provider_ids），本次不改 route 更新语义。
- **前端 `deleteBackupProvider`**：修正死代码——删除成功后的 `removeProviderFromRoute` 循环只改本地 state 且永远执行不到（顺序反了），改为依赖删除成功后的 `loadData()` 重新 hydrate 线路选择；确认弹窗文案补充「将从所有线路解绑」提示。
- **前端补漏（2026-09-23 用户验收反馈，原制品遗漏）**：删除按钮的 `:disabled="isProviderLinked(id)"` 与提示「还挂在某条路由上，先移除再删」仍是旧死锁语义——挂线路 provider 按钮直接禁用，级联解绑在 UI 上永远触达不了。改为：移除禁用，提示行改为告知性文案「挂在线路上，删除将自动从所有线路解绑」。
- **前端统一确认弹窗（同上反馈）**：基于 `AppDialog` 封装全局 promise 式 `useConfirm()` + `AppConfirmDialog`（挂载 `app.vue`，与 `NotifyContainer`/`useNotify` 同模式），`deleteBackupProvider` 弃用原生 `confirm()` 改走新组件；存量其余原生 `confirm()` 调用点不在本 change 范围，后续另立 change 迁移。

## Capabilities

### New Capabilities

（无）

### Modified Capabilities

- `ai-capability-routing`: 新增「Provider 删除级联解绑」Requirement——删除被线路引用的 provider 时自动解除全部线路关联而非拒绝删除；被摘空的线路允许存在。

## Impact

- 后端：`backend-go/internal/admin/handler/ai_handler.go`（`DeleteProvider`）；测试 `backend-go/internal/admin/handler/ai_handler_test.go`（`TestDeleteProviderBlocksLinkedProvider` 改写为级联解绑语义的复现测试）。
- 前端：`front/app/features/ai/composables/useAIRouterSettings.ts`（`deleteBackupProvider`）；`front/app/features/ai/components/AIRouterBackupProviders.vue`（删除按钮 disabled / 提示行）；新增 `front/app/composables/useConfirm.ts` + `front/app/components/ui/AppConfirmDialog.vue`；`front/app/app.vue` 挂载。
- 行为变化：删除挂在线路上的 provider 不再报 409，而是成功并顺手解绑；API 响应 shape 不变（`success`/`message`），无 breaking。
- 部署后影响：已处于死锁状态的存量 provider（无法删除的）删除后即可正常删除；若某能力线路因此被摘空，该能力的 AI 调用会报「无可用 provider」，用户需自行补挂。无数据迁移。
