## Why

日报中线索（thread）是独立的展示维度，每天 10-30 条，14 天总览下 140-420 个节点，导致线程总览（BoardThreadBrowser）信息过载、不可读。线索平铺在 cluster card 里也分散了用户注意力。实际使用中，用户关注的是"话题（section/聚类）的生命周期"，线索只是话题下的细节。

## What Changes

- **Section 获得独立生命周期**：`DailyReportSection` 新增 `status`（emerging/continuing/ending）和 `prev_section_id`（指向前一天同一话题的 section）。状态由后端通过 `cluster_tag_ids` Jaccard 相似度推导，不由 LLM 判断。
- **线程总览改为话题总览**：`BoardThreadBrowser` 从 thread 粒度改为 section 粒度，节点数量从 140-420 降至 70-140，Gantt 图恢复可读性。
- **报纸 Modal 中线索折叠**：cluster card 默认只展示 section 级状态和线索数量，点击展开线索详情。线索不再显示独立状态。
- **生命周期面板改为 section 维度**：`ThreadLineagePanel` 改造为 `SectionLifecyclePanel`，展示 section 跨天演进链。面板定位在 Modal 右侧外侧，不遮挡内容。
- **新增后端 API**：section timeline（总览用）和 section lifecycle（单链追溯用）。
- **移除** `getBoardThreadTimeline` 前端调用，总览数据源改为 section timeline API。

## Capabilities

### New Capabilities
- `section-lifecycle`: Section 级别的生命周期匹配、状态推导、API 查询和前端展示（总览 Gantt、lifecycle panel、报纸 Modal 内线索折叠）

### Modified Capabilities
- `daily-report-system`: DailyReportSection 模型新增 status/prev_section_id 字段；generator 增加基于 cluster_tag_ids Jaccard 相似度的 section 匹配逻辑；前端 BoardDailyReportTimeline 和 BoardThreadBrowser 交互改造

## Impact

**后端（backend-go）**：
- `internal/domain/daily_report/models.go` — DailyReportSection 加 Status、PrevSectionID 字段
- `internal/domain/daily_report/generator.go` — 新增 section 匹配和状态推导逻辑
- `internal/domain/daily_report/repository.go` — 新增 section timeline 和 lifecycle 查询
- `internal/domain/daily_report/handler.go` — 新增 2 个 API endpoint
- `internal/app/router.go` — 注册新路由
- 数据库 migration — daily_report_sections 加 2 列

**前端（front）**：
- `app/api/dailyReports.ts` — 新增接口和方法
- `app/features/tags/components/BoardDailyReportTimeline.vue` — cluster card 改造
- `app/features/tags/components/BoardThreadBrowser.vue` — 改为 section 粒度 Gantt
- `app/features/tags/components/ThreadLineagePanel.vue` — 改为 SectionLifecyclePanel

**兼容性**：Thread 的生成逻辑和 prev_thread_id 保留不动，section 匹配是独立的一层。
