# features/tags — 叙事工坊前端目录约定

> 来源 change：`rework-workshop-into-radar-and-desk`（task 0.2）。本文件只约定**目录归属与新旧共存规则**，行为契约见 `openspec/changes/<change>/specs/` 与 `docs/reference/flow/semantic-board.md`。

## 目录职责

| 目录 | 归属 | 说明 |
| --- | --- | --- |
| `components/`（根） | 旧工坊（tab 结构） | `TagsPage.vue`、`BoardListSidebar.vue`、`BoardHealthPanel.vue`、`BoardSourcePanel.vue`、`BoardThreadBrowser.vue`、`BoardTimelinePanel.vue`、`BoardDailyReportTimeline.vue` 及既有面板。**保留至阶段④**，阶段④随路由切换删除退役面板。 |
| `components/console/` | 新工坊骨架（雷达 / 版块态势 / 归档 / 编排 / 阅读台） | 只放新路由页承载的壳与视图组件：`BoardConsoleShell.vue`、`BoardRail.vue`、`BoardMatrixView.vue`、`BoardLaneEvolution.vue`、`SectionDrawer.vue`、`BoardArchiveView.vue`、`BoardHealthSignal.vue`、`BoardHealthDetail.vue`、`BoardGovernanceRail.vue`、`BoardGovernanceDrawer.vue`、`BoardSettingsForm.vue`、`BoardComposeOverlay.vue`、`BoardNarrowSummary.vue`；L0 雷达 `BoardRadarView.vue`；阅读台路由壳 `components/daily-report/DailyReportDeskShell.vue`（两栏 reader 在 `components/DailyReportReader.vue`，路由页 `pages/tags/[boardId]/[date].vue`）。 |
| `components/lane-dynamics/` | 旧卡片墙（阶段④退役） | 新矩阵**不复用**卡片组件：矩阵/演进带只用 `~/api/laneDynamics` 的响应类型，不 import `LaneDynamicsCard`。 |
| `components/daily-report/` | 日报阅读呈现（报纸布局） | 既有 reader 内部能力（分区映射/页边注/lifeline 缓存/深链）**零改动复用**；阅读台壳（阶段②）只做容器与联动，不重写 reader。 |
| `composables/` | 页面状态与派生逻辑 | 新壳逻辑用 `useBoardConsole`（路由 ↔ 选中泳道 ↔ 窗口 ↔ lane-dynamics）；体检数据用 `useBoardHealth`（面板/右轨共用）；归档用 `useBoardArchive`（日期范围取数 + 平铺排序/筛选，`in_report` 口径由后端派生）；来源构成 `useBoardSources`、构成管理 `useBoardComposition`。 |

## 新旧共存规则（阶段①–③）

1. **路由**：`/tags` 已由 `BoardRadarView` 承载（雷达是工坊入口）；`/tags/:boardId` 由 `BoardConsoleShell`、`/tags/:boardId/:date` 由阅读台、`/tags/:boardId/research` 由研究台承载。旧 `TagsPage` 仅保留文件到 8.4 物理清理，不再作为 `/tags` 入口。改路由映射只改 `front/app/pages/tags/`。
2. **不改旧面板行为**：新壳需要的数据一律走 API/composable 自取，不往旧面板里塞 props 变体（`BoardHealthPanel` 的拆分只做内部区块复位，对外 props/事件不变）。
3. **共享组件不复制**：`AppButton` / `AppDialog` / `AppSectionHeader` / `AppInput` 直接用；两个工坊共享的主题 token 与布局契约见 `docs/reference/standard/frontend/`。
4. **测试宿主**：旧测试跟随旧组件（阶段①–③保持绿灯）；新组件测试放同目录 `*.test.ts`，断言新宿主选择器，不改旧测试的宿主断言。
5. **阶段④清理**：删除 `TagsPage.vue` 的 `contentTab` 分支与退役面板时，同步删除其专属测试，并在 tasks.md 与归档说明留痕（`test-cases.md`「继承与调整」表）。

## 命名

- 新壳组件统一 `Board*` / `DailyReportDesk*` 前缀，子区块用 `*Drawer`、`*View`、`*Rail`、`*Detail`、`*Signal`。
- 组件内 `data-testid` 与旧面板保持同一命名族（`board-health-*`、`lane-*`），便于阶段③把宿主断言从旧面板迁移到新壳。
