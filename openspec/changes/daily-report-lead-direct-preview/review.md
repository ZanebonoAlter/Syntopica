# Review — daily-report-lead-direct-preview

> 审查范围：本 change 全部 diff（`DailyReportDeskShell.vue` / `DailyReportDeskShell.test.ts` / 删除 `DeskArticleDrawer.vue` / `ArticlePreviewModal.vue` overlayStack 接入 / flow 文档 / openspec 制品），对照 delta spec 与 test-cases.md 逐条走查；agent-browser 实页复核（见 ui-verification/report.md）。

### R-1 2026-10-30【High】Esc 关弹窗连阅读台一起退（实现中发现，当场修复）
- **发现**：agent-browser 实页验收 A-4 初测：弹窗开着按 Esc，弹窗关闭且 URL 退到 `/tags/3030`——阅读台整个退出，违背 delta「并保留阅读台上下文」。根因：阅读台 document 级 Esc 监听查 `overlayStack` 计数让位，旧链路 `AppDrawer` 开抽屉时 `pushOverlay()`，直达链路唯一浮层 `AppDialog` 不进栈，Reader 查栈得 false 后同击退台。此问题组件测试测不出（DeskShell 测试 stub 了弹窗，无真实 document 级监听竞争），只有实页能暴露。
- **状态**：已修复（`ArticlePreviewModal` 自登浮层栈：watch visible → push/pop + onBeforeUnmount 兜底；不动共享组件 `AppDialog`；复测第一击关弹窗留台、第二击退台，分层完整；对矩阵页/搜索页宿主计数无副作用）

### R-2 2026-10-30【Low】树莓派 `pnpm build` OOM（exit 134，heap out of memory）
- **发现**：首次 build 在默认堆上限下崩溃；`free -h` 显示 available 5.8Gi，非物理耗尽，是 node 默认 old-space 不足。
- **状态**：已处置（`NODE_OPTIONS=--max-old-space-size=4096` 重跑成功，deploy 链路同样通过）。属环境工具链问题非代码缺陷；是否把该默认固化进 deploy 脚本留待运维侧评估，不在本 change 范围内强推。

- 审毕其余无发现（审查范围：直达接线数据流（getArticle→normalize→弹窗 props）、失败分支不留空弹窗、重复点击幂等、onUnmounted 栈兜底对称性、四宿主（阅读台/矩阵页/搜索页/页边注）overlayStack 影响面、flow 文档与 spec 措辞一致性）。
