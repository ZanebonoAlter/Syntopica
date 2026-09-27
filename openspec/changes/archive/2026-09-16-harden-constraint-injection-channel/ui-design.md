<!-- ui-impact: none -->
<!-- ui-design-state: n/a -->

# UI Design Contract

## N/A Reason

本 change 为 harness 工具链改造（`.pi/extensions/constraint-injection.ts` 及其 smoke 测试 + 规则文档），**不触及 `front/` 任何代码、样式或用户可见的 Syntopica 界面**。

- 注入内容展示形态（稳定层 system prompt 块 / 动态层追加消息）属 pi TUI 内由 extension 自身 `ctx.ui.setWidget` 与 `pi.sendMessage` 控制的表现，不在 Syntopica 前端 UI 范畴（无 Vue 组件、无路由、无布局契约）。
- 故无布局模式（page shell 四模式）、无弹窗档位（dialog 四档）、无双视口验收（1440×900 / 1920×1080）适用面。
- 前端零改动 → 无 UI 验收证据需求；`ui-impact: none` 与 `ui-design-state: n/a` 与 proposal 头声明一致。
