
## 观察矩阵行来源与「新泳道不可见」根因

矩阵行 = `useConsoleSelection.laneGroups` → `laneConsoleView.buildConsoleLaneGroups` → `observationLanes.buildObservationRows(sourceWindow, axis)`，source 是 `GET /semantic-boards/:id/lane-dynamics?days=N`。后端 `computeBoardLaneDynamics`（lane_snapshot_repository.go:614 起）只为「窗口内存在锚定 section」的 active/watch 话题生成 lane（aggs 只来自 listLaneWindowSections），窗口内 0 分区的 active 泳道完全不返回。

实测：新建泳道「以色列内部选举」= board_persistent_topics id 2645、board 1974、status active、source manual，5 个分区全在 2026-10-27~11-09；当前矩阵窗口 11-11~11-24（14 天档）→ 后端不返回 → 矩阵与右轨均不可见，行头 ⋯（编辑/归档）也一并消失。

可用补充数据源：`GET /semantic-boards/:id/topics`（`useDailyReportsApi.listBoardTopics` → front/app/api/dailyReports.ts:782）返回全部 persistent topic（含 status / first_seen_date / last_seen_date / section_count / color / can_activate）。active 且不在 rows 里的即可作为「窗口内未进日报」行；last_seen_date 可解释「最近一次 YYYY-MM-DD」。

**引用**：front/app/features/tags/components/console/observationLanes.ts:72、backend-go/internal/topicgraph/repository/lane_snapshot_repository.go:614、front/app/features/tags/components/console/laneConsoleView.ts:33、front/app/api/dailyReports.ts:782

<!-- pinned 2026-11-25T08:06:19Z -->

## 观察矩阵组件装配链与改名/其他话题现状

装配链：`BoardConsoleShell.vue`（状态/弹层/阅读接线）→ `consoleStageBinding.buildConsoleStageProps` → `BoardConsoleStage.vue`（宽屏 `BoardWideWorkface`）→ `ObservationMatrix.vue` → `ObservationMatrixGrid.vue` → `ObservationMatrixLaneRow.vue`（多根：行头 div + 逐日格；行头 ⋯ = `RowActionMenu`，菜单项「编辑泳道/归档泳道」，emit 上抛到壳层 `overlays.openLaneCorrection/openLaneArchive`）。

改名现状：只有 `BoardComposeOverlay.vue` 修正模式标题处就地改名 → `useLaneCorrection.commitRename` → `useDailyReportsApi.updateTopic(id,{label})`（PATCH `/daily-reports/topics/:id`，handler `updateTopic`）；成功后壳层 `handleLaneRenamed()` 调 `load()`。矩阵行没有任何改名入口。

「当日其他话题」现状：`dayOtherTopics.buildOtherTopics` 只用**所选日**的 `reportDay`（`useLaneDayReading` 单份 store）+ A 名单排除，跨日无数据。矩阵里是 `DayOtherTopicsGroup.vue`（summary + 仅绘制所选日格）。整行跨日读数需要后端 lane-dynamics 新增「每日未入泳道分区的非综合稿线索数」聚合（sections where persistent_topic_id IS NULL OR NOT IN active/watch 话题集，threads scope != 'composite'）。

**引用**：front/app/features/tags/components/console/ObservationMatrixGrid.vue、front/app/features/tags/components/console/ObservationMatrixLaneRow.vue、front/app/features/tags/composables/useLaneCorrection.ts:952、front/app/features/tags/components/console/dayOtherTopics.ts、front/app/api/normalizers/laneDynamicsWindow.ts

<!-- pinned 2026-11-25T08:06:19Z -->
