<!-- complexity: complex -->
<!-- ui-impact: minor -->

# 归档时间选择：折线导航 + 默认当天

## Why

归档视图的日期筛选现在是「数据日 / 近 7 / 14 / 30 天 chip + 两个裸日期输入」，且默认日期落在**版块最新日报日**（`asOf`）而不是日历当天——用户要看「今天进了什么」，得先分辨数据锚点在哪天再手改日期；想看「哪几天密集」则完全看不出来。

矩阵/追踪页已经有一条经过验证的时间折线控件（`ObservationRangeControls`：拖空白刷选、拖选区平移、双端手柄、键盘、外置轴、非法输入不静默纠正）。归档没有理由再维护第二套裸日期输入：同一套手势、把曲线数据换成「每日进版块文章数」，就能一眼看出哪天密集并直接拖出区间。

## What Changes

- **默认日期改为日历今天**（Asia/Shanghai）；从历史格子「看归档」带入的 `initialDate` 仍优先；不再以最新日报日 `asOf` 作默认（今天没文章就是 0 篇空态，不自动漂移）。
- **删除**工具行的「数据日 / 近 7 天 / 近 14 天 / 近 30 天」快捷 chip 与「从 / 到」裸日期输入的原有位置；工具行只留组内排序、全部/未纳入日报/弱证据筛选、⚙筛选、ⓘ匹配流程。
- **新增归档时间导航块**（工具行下方、文章列表上方，占一条带轴曲线的高度）：
  - 主图 = **90 天视窗**内的「每日进版块文章数」折线，复用矩阵时间视窗的刷选/双端手柄/键盘/日期表单契约（拖空白=新建选区、拖选区=平移选区、拖两端=改起止；拖动中只预览，松手 commit 才重拉列表）；
  - 主图自带「从 / 到」日期表单（由复用组件提供），表单可选界 = 全历史，超出视窗的日期提交后视窗自动跟随；
  - 底部**全历史导航条** = 最早有文章日 → 今天的迷你折线 + 可拖动的「当前 90 天视窗」窗口：拖动窗口即平移视窗（可以一直拖到去年），已提交区间随视窗同步平移（保持相对位置），松手提交一次；单击某处定位视窗；
  - 视窗与选区都不写 URL（归档无深链日期契约），`initialDate` 仅作一次性入口。
- **新增后端聚合端点** `GET /api/semantic-boards/:id/article-daily-counts`：返回该版块 `earliest_date`（最早命中文章日）与稀疏 `days[{date, count}]`（只含 count>0 的日期，缺口即已知 0）。成员口径与归档列表主查询**逐字一致**（`article_topic_tags ⋈ topic_tag_board_labels` 的版块标签集合 + `feeds` 内连接），因此「曲线上某天的高度 = 列表当天篇数」。
- **曲线口径固定** = 全版块命中文章数：**不随**来源 / 构成标签 / 方向不符筛选变化（筛选取只影响下方列表），图注显式写明；不随 `report_status` 变化。
- **BREAKING**（产品行为）：归档不再提供「近 7 / 14 / 30 天」一键快捷，也不再有独立的「从 / 到」工具行位置；日期只能通过折线刷选或图上的日期表单设置。
- 顺带修正口径漂移：`normalizeArchiveRange` 的日期回退由宿主本地时区 today 改为与页面同源的 `todayInShanghai`。

## Capabilities

### New Capabilities

无（本 change 不引入新领域能力，全部落在既有归档视图与既有版块文章 API 上）。

### Modified Capabilities

- `workshop-article-archive`：归档的「入口与日期筛选」要求整体重写——默认日期口径、日期控件形态（折线导航块取代快捷 chip + 工具行日期输入）、视窗/选区/导航条交互契约、图注口径与空/加载/错误状态。
- `board-article-api`：新增「按日聚合文章数」端点要求（路径、参数、返回结构、成员口径与列表一致、稀疏返回语义）。

## Impact

- **前端**
  - `front/app/features/tags/components/console/BoardArchiveView.vue`：默认日期、删 chip/工具行日期输入、挂载导航块、range 提交链路。
  - `front/app/features/tags/components/console/ObservationRangeControls.vue`：新增**可选项**（`unitLabel` 图注单位、`title` 标题、`collapseStorageKey` 折叠偏好键）与 `footer` 具名插槽，矩阵/工作面缺省行为与 DOM 零变化。
  - 新增 `front/app/features/tags/components/console/ArchiveHistoryStrip.vue`（全历史导航条 + 视窗拖动）。
  - 新增 `front/app/features/tags/components/console/archiveTimeSeries.ts`（每日计数 → 稀疏日期键序列、视窗切片、导航条分桶的纯函数）。
  - `front/app/features/tags/composables/useBoardArchive.ts`：range 归一与 `todayInShanghai` 同源、删除退役的 `archiveWindowRange`。
  - 新建 `front/app/features/tags/composables/useArchiveDailyCounts.ts`：每日计数获取 + 60s 模块级缓存 + stale-while-revalidate + 切版块竞态守卫。
  - `front/app/api/semanticBoards.ts`：新增 `getBoardArticleDailyCounts`。
- **后端**
  - 新建 `backend-go/internal/tagmanagement/handler/board_article_daily_counts_handler.go`（handler + 聚合 SQL），并把 `boardTagIDs` 取法提取为 `loadBoardTagIDs` 与列表主查询共用。
  - `backend-go/internal/tagmanagement/handler/board_crud_handler.go` 同族路由注册（紧挨 `/:id/articles`）。
- **测试**
  - `backend-go/internal/tagmanagement/handler/board_article_handler_test.go`：聚合口径（与列表成员一致、稀疏返回、空版块）。
  - `front/app/features/tags/components/console/BoardArchiveView.test.ts`：默认今天、chip 移除、手势 commit 触发重拉。
  - 新增 `ArchiveHistoryStrip.test.ts`、`archiveTimeSeries.test.ts`；`ObservationRangeControls.test.ts` 补「新参数缺省时矩阵行为不变」。
- **文档**：`docs/reference/api/`（端点）、`docs/reference/flow/semantic-board.md`（归档日期口径不变量）、`docs/reference/standard/frontend/interaction.md` 无需改（沿用既有模式与手势，不引入新模式）。
- **数据/部署**：无迁移、无配置项；端点只读，旧前端打新后端或反之都不会崩（新端点未被调用时是死路由）。
