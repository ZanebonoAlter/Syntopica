# Design — AI 健康提示移入通知中心

## Context

- 现状：`app.vue` 挂载 `AiHealthBanner`（fixed 顶部居中），可见性 = `!analysisPaused && !aiHealthy`，内含「重新检测」（`useHealthReprobe` + `loadSchedulersStatus` 刷新关闭）与「去配置」（`/settings?section=ai-health`）。
- 健康态来源：`useSchedulerStatus` 的 `aiHealthy`/`analysisPaused`（useState 全局共享，30s 轮询），已是全局响应式状态，通知中心组件可直接消费，无需新数据链路。
- 通知中心为**落库通知**（白名单=日报终态），spec 明确禁止高频过程类事件产生通知；AI 健康态是易翻转的运行时状态，落库必刷屏。
- 顶部栏已有常驻 heart-pulse 健康指示（绿/红，点击跳设置），本 change 不动。

## Goals / Non-Goals

**Goals:**
- 前台零占用：删除顶部悬浮 banner 后，「意图运行但未就绪」的提示只存在于通知中心。
- 提示能力平移不缩水：文案、重新检测、去配置入口全部保留。

**Non-Goals:**
- 不做后端改动（健康态/重探 API 全复用）。
- 不做落库通知（不新增通知类型、不动白名单、不动未读数语义）。
- 不动顶部栏 heart-pulse 常驻指示与 favicon 暂停角标。

## Decisions

### D1: 虚拟置顶条（客户端状态驱动），不落库

**选择**：面板内由 `!analysisPaused && !aiHealthy` 驱动的置顶条，纯前端组件逻辑。
**替代方案**：健康翻转时后端写通知表——违背 notification-center 白名单约束（高频过程类事件），且状态翻转会产生成对通知噪音；Toast 一次性提示——刷新/切页即丢，持续「为什么不跑分析」的信号消失。均否决。

### D2: 置顶条挂在列表容器之上、分页序列之外

渲染于 `.notif-panel__list` 之前（头部之下），不进入 `view.list` 分页数组——数据结构与后端契约零耦合，空列表/加载中/错误态时仍可见（spec scenario）。「重新检测」复用 `useHealthReprobe().reprobeHealth()` + 成功后 `loadSchedulersStatus()` 刷新可见性（与原 banner 行为一致）。

### D3: 铃铛警示态用图标+配色变体，不占用未读角标

未就绪时铃铛图标切警示形态（`mdi:bell-alert`）+ warning 色令牌；未读数角标逻辑与数值完全不动（两信号正交，spec 明确不计未读数）。备选「强制显示角标 1」会污染未读数语义，否决。

### D4: 删除组件而非隐藏

`AiHealthBanner.vue` 及其测试整体删除（含 `app.vue` 挂载点），不留开关。组件已无其他引用方（仅 app.vue 与 error.vue 注释提及，注释同步改）。

## Risks / Trade-offs

- [发现性下降：banner 常驻可见 → 需要点铃铛才见详情] → 铃铛警示态（图标+配色）承担信号位；顶部栏 heart-pulse 常驻指示仍在（红态即未就绪），双层信号兜底。
- [健康态轮询间隔内（最长 30s）铃铛/置顶条状态滞后] → 与原 banner 一致（同源状态），不新增延迟；手动重探后立即刷新。
- [现有 AiHealthBanner.test.ts 用例作废] → 用例迁移到 NotificationPanel/NotificationBell 测试（显隐条件、按钮行为、暂停态不显），断言语义平移。

## Migration Plan

纯前端改动，`pnpm build` 部署即生效；无数据迁移、无回滚数据风险（回滚 = 还原代码重新构建）。

## Open Questions

（无）
