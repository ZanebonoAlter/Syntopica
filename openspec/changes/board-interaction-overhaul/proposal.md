## Why

上一次变更 (`2026-05-23-semantic-label-board-system`) 完成了辅助标签→语义板块→叙事板的核心链路，但留下了 5 个交互性问题：(1) max_sim 匹配规则只看单个辅助标签最高相似度，导致跨域误匹配（如"中国科技媒体"被挂到"科技行业ETF"）；(2) 板块文章列表缺少 feed/category/时间筛选，且展示只有 feedId 无名称；(3) 升级建议面板显示 #id 而非标签名称；(4) 缺少辅助标签→事件→文章的展示链和事件时间线；(5) 叙事功能被隔离在 /topics 页面的独立 tab 中，与板块管理割裂，用户需要跨页面理解同一个板块的 composition 和叙事。(6) Feed refresh 接口串行执行（refreshAllFeedsWorker 逐个串行刷新，前端 onMounted 也用 await 串行加载 feeds/articles/stats/watched-tags），导致页面加载慢、批量刷新耗时长；(7) 板块文章列表的 tag chip 只显示 label，匹配得分和计算公式藏在 HTML title tooltip 里，用户必须 hover 才能看到，不够直观；(8) 现有叙事系统生成多条独立线索（emerging/continuing/ending），不是用户期望的"一天一篇日报"；叙事生成通过 HTTP POST 同步调用，单板块 5 分钟超时，全部板块 10 分钟超时，前端转圈阻塞体验差；且 embedding 区分度不够导致去重不可靠

## What Changes

- **max_sim 匹配规则加双因子约束**：max_sim ≥ 0.8 的直接挂载规则新增 `hits ≥ min(2, N)` 和 `hit_rate ≥ 0.3` 两个必要条件，要求至少 min(2, N) 个辅助标签与 board 构成标签的相似度超过 sim_threshold，且命中率不低于 30%，防止单标签高相似度导致的跨域误匹配
- **升级建议 DTO 增强**：后端 UpgradeSuggestion 返回 `auxiliary_labels [{id, label}]` 和 `target_board_label`，前端展示 label 替代 #id
- **新增 Board 文章列表 API**：`GET /semantic-boards/:id/articles`，支持 feed_id/category_id/start_date/end_date/auxiliary_label_id 筛选，每篇文章返回 feed_name 和过滤后的 tags（只返回属于当前 board 的 event/person/keyword 标签，含匹配度信息 match_reason + score）
- **新增 Board 叙事时间线 API**：`GET /semantic-boards/:id/narratives?days=7`，返回该 board 最近 N 天的叙事列表
- **叙事生成取消 scope 分类**：每个 SemanticBoard 每天只生成一份叙事，不再区分 global/category scope，事件标签来源为该 board 下所有文章对应的 event tags
- **叙事功能迁移到 /tags 页面**：在板块详情中嵌入 BoardNarrativeTimeline 组件，每条叙事展示为"小文章卡片"（标题+摘要+status+关联标签+文章数），点击展开关联文章；/topics 页面的叙事 tab 删除
- **TagsPage 文章列表增强**：新增 Feed 下拉筛选、时间范围选择器、文章行展示 feed_name 和事件标签 chips（每个 chip 的 tooltip 显示匹配原因和分数）
- **Feed refresh 并行化**：后端 `refreshAllFeedsWorker` 改为并发刷新（semaphore=3 限流），前端 `FeedLayoutShell.vue` 的 `onMounted` 改为 `Promise.all` 并行加载 feeds/watched-tags 和 articles/stats（两波）
- **匹配得分可视化增强**：每篇文章的每个 tag chip 带颜色（direct_hit=绿/hit_rate=蓝/max_sim=橙/weighted=灰）和分数文字，在文章行右侧 end 处展示匹配方式和分数
- **日报系统替代旧叙事**：新增 `BoardDailyReport` + `DailyReportSection` 数据模型，4 步流水线（精确去重→LLM语义分组→并行分段生成→组装存储），结构化日报包含今日重点、板块动态、聚类叙事线索三部分；`POST /api/daily-reports/generate` 立即返回 `{job_id}`，后台 goroutine 异步执行生成，通过已有 `ws.Hub` 广播 `daily_report_progress` / `daily_report_done` 消息；前端 `useDailyReportProgress.ts` composable 监听 WS 事件，`NarrativeGenerateDialog.vue` 改为进度板模式；完全废弃旧的 NarrativeBoard/NarrativeSummary 生成逻辑，复用 `scheduler_tasks` 的 narrative_summary 定时任务

## Capabilities

### New Capabilities
- `board-article-api`: 板块文章列表独立 API，支持 feed/time/辅助标签筛选，返回过滤后的 board 维度 tags
- `board-narrative-timeline`: 板块叙事时间线 API + 前端组件，取消 scope 分类，叙事以"小文章卡片"形式嵌入板块详情页
- `refresh-parallelization`: 后端 refresh-all 并发化 + 前端 onMounted 并行化
- `match-score-visualization`: tag chip 带颜色和分数的文章行右侧匹配信息展示
- `daily-report-system`: 日报生成流水线（去重→分组→并行生成→组装），新数据模型，异步 WebSocket 进度推送，替代旧叙事系统

### Modified Capabilities
- `tag-to-board-matching`: max_sim 规则新增 hits ≥ min(2, N) 和 hit_rate ≥ 0.3 双因子约束
- `board-management-api`: 升级建议 DTO 增强（auxiliary_labels + target_board_label）；新增 board 文章列表和叙事时间线路由
- `narrative-board-generation`: 完全替代，改为日报生成流水线
- `narrative-board-frontend`: BoardNarrativeTimeline 替换为 BoardDailyReportTimeline，展示结构化日报；NarrativeGenerateDialog 改为触发日报生成
- `board-article-api`: 文章列表返回的 filtered_tags 已有 match_reason/score，前端展示增强

## Impact

- **后端**: `semantic_board_matching.go` 匹配规则改造；`semantic_board_handler.go` 新增 2 个路由 + DTO 增强；`service.go` + `board_narrative_generator.go` 叙事生成逻辑取消 scope；`narrative_board_generator.go` 叙事板创建逻辑调整；`feed/handler.go` refreshAllFeedsWorker 并发改造；`FeedLayoutShell.vue` onMounted 并行化；新增 `daily_report` 包（generator.go, cluster.go, models.go, handler.go）；narrative handler 改异步；`narrative_summary` scheduler_task 复用
- **前端**: `TagsPage.vue` 集成 BoardDailyReportTimeline + 文章列表改造 + Tab 切换（板块内容/日报/文章）；新增 `BoardDailyReportTimeline.vue` 组件替代 BoardNarrativeTimeline；`UpgradeSuggestionPanel.vue` #id→label；`TopicGraphPage.vue` 删除叙事 tab 和 NarrativePanel 引用；`FeedLayoutShell.vue` onMounted Promise.all；`TagsPage.vue` 文章行匹配信息展示；新增 `useDailyReportProgress.ts` composable；`NarrativeGenerateDialog.vue` 改为触发日报生成并显示进度板
- **数据模型**: `SemanticBoardMatchConfig` 新增 2 个配置项；`NarrativeBoard.scope_type` 语义变更（统一为 board 维度）；旧 scope 数据需兼容处理；新增 `board_daily_reports` + `daily_report_sections` 表；废弃 `narrative_boards` + `narrative_summaries` 表的写入（旧数据保留只读）
- **API**: 新增 `GET /semantic-boards/:id/articles`、`GET /semantic-boards/:id/narratives`（旧叙事数据只读）；升级建议响应格式变更（非破坏性新增字段）；新增 `POST /api/daily-reports/generate`、`GET /api/semantic-boards/:id/daily-reports`、`GET /api/daily-reports/:id`；新增 WS 消息类型 `daily_report_progress`/`daily_report_done`
- **删除**: /topics 页面叙事 tab、NarrativePanel.vue、相关 scope 切换逻辑
