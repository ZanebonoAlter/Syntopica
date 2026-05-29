# Issue 01: 线索总览（BoardThreadBrowser）信息过载，缺乏实用价值

> **Status:** open
> **Priority:** low
> **Component:** front/app/features/tags/components/BoardThreadBrowser.vue, BoardDailyReportTimeline.vue

## 问题描述

`BoardThreadBrowser`（Gantt 图风格的线程总览视图）在实际使用中几乎没有价值：

- **线索数量太多**：每个板块每天有 10+ 条线索，30 天视图下有数百个节点，Gantt 图变成密密麻麻的色点，无法阅读
- **缺乏筛选/聚合**：没有按状态筛选、没有折叠单次出现的孤立线索、没有高亮活跃链
- **与日报视图割裂**：切到总览后完全脱离了日报上下文，用户无法建立"这条线索对应哪天的哪个报道"的直觉

## 根因

设计时假设线索数量少（每天 2-3 条核心线索），实际生成器为每个 cluster 都创建 1-3 条线索，总量远超预期。Gantt 图在数据量大时信息密度过高，缺乏聚合机制。

## 可能的修复方向

1. **移除 BoardThreadBrowser**，只保留日报内的 `ThreadLineagePanel`（点击线索查看血统链）
2. **改为聚合视图**：只显示"活跃链"（跨越 2+ 天的链），孤立线索折叠为计数
3. **增加筛选**：按状态、按天数、按活跃度过滤
4. **降级为 tooltip**：在日报列表的每个条目上显示该日线索与前一日的关联数，点击仍打开 lineage panel

## 影响范围

- `BoardThreadBrowser.vue` — 主要修改对象
- `BoardDailyReportTimeline.vue` — 切换按钮和视图逻辑
- `front/app/api/dailyReports.ts` — `getBoardThreadTimeline` 可能不再需要
- `backend-go` handler/repository — `GetBoardThreadTimeline` 对应调整
