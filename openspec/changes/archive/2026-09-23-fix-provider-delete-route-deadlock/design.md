## Context

Bug 死锁链路（探索结论，2026-09-23 实测代码）：

1. 后端 `DeleteProvider`（`backend-go/internal/admin/handler/ai_handler.go`）：`ai_route_providers` 存在关联即 409。
2. 前端 `deleteBackupProvider`（`front/app/features/ai/composables/useAIRouterSettings.ts`）：**顺序反了**——先调删除接口（被 409 拦下抛错），解绑循环 `removeProviderFromRoute` 放在删除成功之后，永远执行不到；且该函数只改本地 `routeSelections` state，不调任何 API。
3. 用户手动解绑的两条路也堵死：`saveRoutes` 对空线路 `continue` 跳过不发请求；后端 `UpdateRoute` handler 对空 `provider_ids` 直接 400（而 store 层 `UpsertRoute` 本身支持空列表清空绑定，注释明示 "Empty providerIDs clears bindings — no check"）。

→ 三层叠加：挂在线路上的 provider 永远删不掉。

## Goals / Non-Goals

- **Goals**：解开死锁——删除 provider 时级联解绑（用户已确认此方向）；被摘空的线路允许存在（用户已确认）。
- **Non-Goals**：不放开 `UpdateRoute` handler 的空列表 400（route 更新语义不动，如需显式清空线路另立 change）；不改线路编辑 UI 交互；不做数据迁移。

## Decisions

### D1：级联解绑放 handler 层，单事务

`DeleteProvider` 内用 `db.Transaction` 包裹：删 `ai_route_providers` 关联 → 删 provider。先 `Find` 关联记录拿数量与 capability 列表（供响应 message），再删。不放在 `airouter.Store` 层：这是管理面删除语义，不是路由解析逻辑，handler 与既有 First/Count/Delete 直连 `repository.Repo.DB()` 的风格一致。

### D2：响应 message 带解绑信息

成功响应 `message` 形如 `provider deleted (detached from N route(s))`；N=0 时维持 `provider deleted`。前端只看 `success` 字段，无需联动改动。

### D3：前端删死代码，依赖 `loadData()` 重建 state

`deleteBackupProvider` 里删除成功后的 `for (...) removeProviderFromRoute(...)` 循环删除：`await loadData()` 末尾的 `hydrateRouteSelections()` 已用后端真值重建 `routeSelections`，本地手动过滤多余且语义错误。确认弹窗文案追加「将从所有线路解绑」。

### D4：空线路的运行时行为不新增代码

`UpsertRoute` 建线路时 providerIDs 可为空、`ai_route_providers` 无行——路由解析侧已有「无可用 provider」报错路径（`TestDeleteProviderRemovesUnusedProvider` 同构场景由 airouter 既有错误覆盖），摘空线路天然落入该路径，不新增代码，仅补一条 spec 级 Scenario 佐证。

### D5：删除按钮禁用改告知（2026-09-23 用户验收反馈补漏）

原制品漏看 `AIRouterBackupProviders.vue` 两个 UI 元素：删除按钮 `:disabled="ctx.isProviderLinked(provider.id)"` 与提示行「还挂在某条路由上，先移除再删」——都是旧死锁语义。后端 409 移除后，这两处反而把级联解绑的新能力在 UI 上封死（挂线路 provider 按钮禁用，永远点不到删除）。改为：移除 disabled；提示行改告知性文案「挂在线路上，删除将自动从所有线路解绑」。`isProviderLinked` 函数保留（提示行 v-if 与「已挂载」语义仍有用）。

### D6：全局 promise 式 useConfirm（用户确认做）

用户反馈原生 `confirm()` 体验差且项目无统一确认弹窗。项目已有 `AppDialog`（layout 契约基座）但无确认弹窗封装。新增：

- `front/app/composables/useConfirm.ts`：`confirm(options): Promise<boolean>`，全局单例（`useState` 存可序列化 state，resolve 存模块级 Map 以 id 关联，避免向 state 塞函数）。选项：`title` / `message` / `confirmText`（默认「确认」）/ `cancelText`（默认「取消」）/ `danger`（true 时确认按钮 variant=danger）。
- `front/app/components/ui/AppConfirmDialog.vue`：内部复用 `AppDialog`（`size="sm"`、`closeOnOverlay=false`、`showClose=false`——确认动作只经两个按钮关闭，Escape=取消），footer 用 `AppButton`（cancel=secondary / confirm=danger|primary）。
- 挂载 `front/app/app.vue`（与 `NotifyContainer` 并列，同一全局通道模式）。
- 本 change 只接入 `deleteBackupProvider` 一处（顺带验证组件）；存量 13+ 处原生 `confirm()` 及 `FeedLayoutShell` 的 `alert()` 错误提示不在本 change 范围，后续另立 change 迁移。

## Risks / Trade-offs

- 用户可能没意识到删除会连带解绑 → 确认弹窗文案明确提示 + 挂线路 provider 行内告知文案（D5）；删除是显式确认动作，风险可接受。
- useConfirm 为全局新组件，后续迁移存量是另一 change 的范围风险（组件 API 设计不当会返工）→ API 保持与原生 confirm 语义对齐（Promise<boolean>），选项面收窄（title/message/confirmText/cancelText/danger），不过度设计。
- 摘空线路后该能力 AI 调用报错，用户可能困惑 → 错误信息既有且可预期；spec 已把该行为固化为 Scenario。

## Migration

无数据迁移。存量死锁 provider 部署新版后直接可删；删完若某能力线路被摘空，需用户自行补挂 provider。
