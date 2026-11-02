<!-- complexity: complex -->
<!-- ui-impact: major -->
<!-- constraint-domains: semantic-board, daily-report, topic-graph, reading -->

## Why

叙事工坊现在是「5 个平级 tab + 纵向卡片堆」：同一批数据（泳道 × 日期 × 事件）被画了三遍（泳道卡片墙 / 话题总览 DAG / 侦探墙），关键状态（体检、追踪、来源）只在某一个 tab 里可见，切 tab 即失忆；而用户每天的主任务是「看今天的日报、判断要不要深读文章」，日报阅读器却被夹在 tab 里、前面还挡着一屏治理表。用户确认的方向是**注意力路由 + 观看便利**（4K 本地屏，非挂墙数据大屏）：先回答「今天该看谁」，再回答「这条线怎么走到今天」，最后落到「今天写了什么、原文在哪」。

## What Changes

- **新增雷达首屏**：`/tags` 无版块选中时展示跨版块雷达（每版块行：今日事件数、3 日环比、14 天泳道密度、最新日报标题），右侧「今日该读」+「今天可能升温」；点行进入版块态势。
- **版块态势页重做**：面包屑 + 三区（版块栏 / 矩阵舞台 / 治理右轨）。泳道 × 日期矩阵取代泳道卡片墙（格子 = 当日 section 数，空白即断档），点格子开 section 抽屉（事件 + 当日日报线索 + 动作）；选中泳道展开演进带与今日事件；14 天 ≤ 2 个分区的泳道折叠进「休眠」。
- **治理降级为常驻信号**：体检分/趋势/幽灵/未收编候选/来源占比进右轨；质量表、组合标签、来源全量、版块设置进治理抽屉；匹配参数、标签合并、回填/升级建议与标签池从常驻工具栏收进版块 ⋯ /「更多」菜单（设置页完整迁移另立变更）。
- **新增归档视图**：文章按命中的构成标签分组（一组 ≈ 一次事件），标出「未纳入日报」并可按遗漏/弱证据筛选，展开展示匹配证据（规则 direct_hit/hit_rate/max_sim/weighted/composite_hit、分数、降级、方向校验）；排序为「未入日报 → 最新 → 篇数」。
- **编排态重做（2026-10-30 用户修订，原型待审）**：全屏点线画布＋左侧候选泳道系列；候选按同ID原关系连通片段、首末报告日期差至少3天筛选（不是连续命中3天/固定3节点），点击整组带入基础成员，不自动补相似或截20条。7/14/30/90天＋自选起止共窗控制候选/图/推荐，有草稿改日期先确认；补充推荐以当前有效成员平均质心计算，优先非active，active仅次级只读。原关系不按top6截断、不伪造手工边；无边/不达候选条件仍可人工补选。历史管理资格沿server can_activate，不改变累计命中/生命周期。2026-10-31 用户追加修订（原型待审）：整组带入必须看得见也能撤——组与成员用原生复选框并带半选态，支持取消整组/清空已选/取消本次，取消不吞组外补选与手写名；已有active泳道默认不展示（避免与待划分话题交叉成乱麻），需要上下文时显式开启只读参照；质心相近点以大小＋颜色深浅区分，已选用独立✓标识，避免推荐视觉被误读成已选。详见新建泳道阶段合同及workshop-lane-compose delta；本轮未改产品或后台。
- **日报阅读台重排（2026-10-25 复审修订，2026-10-27 用户复审撤销）**：~~分区索引栏（日期历史下拉并入顶部）+ 正文（40rem）两栏；删除最左历史栏与右栏演进/本期话题，页边注收为右下抽屉~~ → **恢复旧版阅读页**：大刊头 + 「今日重点」top3、左目录、宽屏正文、右侧常驻页边注栏；仅保留路由 `/tags/:boardId/:date` 与 watch 命中定位/深链定位能力（日报阅读台本不在 UI 批准范围，详见 human-feedback FB-10）。
- **BREAKING**：`/tags` 默认视图从「板块内容 tab」变为雷达；原五个 content tab 退役（板块内容/话题总览合并为矩阵视图，文章 → 归档视图，研究 → 版块 ⋯ 全屏工作台，日报 → 阅读台）；`/tags?board=&report=&annotation=` 深链语义保留。
- **窄屏降级**：<768px 只保留「雷达 → 版块摘要（泳道列表 + 迷你 14 天条）→ 日报」，不做矩阵横向滚动；编排态禁用并提示用宽屏。

## Capabilities

### New Capabilities
- `workshop-radar`: 工坊首屏雷达——跨版块热度排序、今日该读、升温提示，以及支撑它的版块总览聚合端点。
- `workshop-board-console`: 版块态势页——面包屑三区骨架、矩阵视图、section 抽屉、演进带、休眠泳道、治理右轨与治理抽屉的分工。
- `workshop-article-archive`: 归档视图——按事件分组的文章档案、未纳入日报标记与筛选、逐篇匹配证据。
- `workshop-lane-compose`: 编排态——section 时间线画布、候选采纳与确认启用、关联推荐、聚类质量指标、保存新泳道。

### Modified Capabilities
- `board-lane-dynamics`: 「泳道动态视图入口」「泳道卡片构成」由卡片墙改为矩阵 + 演进带（卡片形态退役，端点与聚合口径不变）。
- `source-board-hit-rate`: 「板块页板块内容内的来源构成面板」位置与形态改为右轨 top3 + 治理抽屉全量（不再挂在「板块内容」tab 底部，tab 结构本身退役）。
- `board-article-api`: 文章列表需支撑归档口径——返回「是否出现在窗口内日报分区线索中」与匹配证据字段，并支持按遗漏/弱证据筛选。
- `daily-report-system`: 「日报时间线组件 BoardDailyReportTimeline（报纸布局）」重排为阅读台「索引 + 正文」两栏，日期历史并进索引、页边注改抽屉，保留 watch 定位/深链。

## Impact

- **前端路由**：`front/app/pages/tags.vue` 由单页改为 `/tags`（雷达）与 `/tags/:boardId`（版块态势）；保留 query 深链。
- **前端组件**：`TagsPage.vue`、`BoardListSidebar.vue`、`BoardHealthPanel.vue`、`LaneDynamicsPanel.vue`、`BoardSourcePanel.vue`、`BoardThreadBrowser.vue`、`BoardDailyReportTimeline.vue`、`BoardTimelinePanel.vue`、`AuxiliaryLabelPool.vue`、`settings.vue`（标签池/全局参数迁入）；新增雷达、矩阵、归档、编排、阅读台壳组件。
- **后端 API**：新增 `GET /api/semantic-boards/overview`；`GET /api/semantic-boards/:id/articles` 增加 `in_report` 与匹配证据字段（复用现有 sourcestats / 日报分区线索口径，不复制公式）。
- **文档**：`docs/reference/flow/semantic-board.md`（视图与治理入口）、`flow/daily-report.md`（阅读台）、`flow/topic-graph.md`（编排态）、`standard/frontend/layout.md`（固定视口栅格的例外登记）。
- **测试**：新增组件单测（雷达排序、矩阵/归档/编排交互、休眠折叠、深链）；既有 `BoardHealthPanel` / `BoardThreadBrowser` 相关测试需按新宿主调整。
