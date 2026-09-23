<!-- ui-impact: minor -->
<!-- ui-approval: not-required -->
<!-- ui-prototype: none -->

## UI 设计契约（minor：复用/状态契约）

> 2026-09-23 用户验收反馈增补：删除按钮 disabled 死锁语义补漏（D5）+ 全局 useConfirm 确认弹窗（D6）。仍为 minor——全部复用既有 `AppDialog`/`AppButton` 契约封装，无页面布局/导航变更。

## 入口与入口变更

- 入口不变：设置页 → AI 路由管理（`AIProviderManagement.vue`）→ 备用模型列表的「删除」按钮（`deleteBackupProvider`）。
- 入口变更：无新增页面入口、无导航/布局调整。删除确认从原生 `confirm()` 换为全局 `AppConfirmDialog`（AppDialog sm 档）；删除按钮不再因挂线路禁用；提示行改为告知性文案。

## 受影响状态

| 状态 | 影响 |
| --- | --- |
| success | 删除成功路径不变（确认 → DELETE → `loadData()` 刷新）；确认弹窗换用全局 `AppConfirmDialog`（danger 按钮，文案含「将从所有线路解绑」）；成功消息维持现状。此前删除挂线路 provider 必失败的 409 死锁态消失，页面不再出现「删除备用模型失败」误报 |
| 挂线路（行内提示） | 删除按钮**可点**（不再 disabled）；行内提示改告知「挂在线路上，删除将自动从所有线路解绑」，不再是阻断式「先移除再删」 |
| error | 删除失败的错误提示路径保留（网络/其他后端错误仍会 pushMessage error），文案不变 |
| loading / empty | 不受影响（`saving` 按钮禁用逻辑、空列表态均不动） |

## 复用组件与布局模式

- 全部复用既有组件：`AppButton`（删除按钮 + 确认弹窗 footer 按钮）、`AppDialog`（确认弹窗基座，`size="sm"` 档，弹窗契约见 `standard/frontend/layout.md`）、`pushMessage`（toast 反馈）。新增 `AppConfirmDialog`（AppDialog+AppButton 封装，无新样式体系）与 `useConfirm` composable，挂载 `app.vue`（与 `NotifyContainer` 同模式）。
- 确认弹窗交互：仅确认/取消两钮关闭 + Escape=取消；不点遮罩关、无右上关闭钮（对齐原生 confirm 的阻断语义）。
- 不改动 layout mode，不新增页面级布局。

## 验收映射

- 组件测试：`useConfirm.test.ts` + `AppConfirmDialog.test.ts`（resolve 布尔、两钮/Escape 关闭、danger variant）；`useAIRouterSettings.test.ts` 断言「删除挂线路 provider 成功后调用 `loadData` 且不再调用本地 `removeProviderFromRoute`」+ mock `useConfirm`。
- 人工验证：挂线路的 provider 在页面上点删除（按钮可点）→ 弹确认弹窗（含解绑提示）→ 确认后列表刷新、provider 消失、线路面板同步（该线路被摘空时显示为空）；取消则无任何请求。
