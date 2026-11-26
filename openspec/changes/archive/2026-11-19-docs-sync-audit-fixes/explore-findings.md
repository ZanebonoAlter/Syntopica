
## 2026-11-19 文档治理审计：9 项落后事实与证据

## 审计方法
- 代码路由全集：临时 main（空 gorm.DB 句柄 + 各域 InitRepository + gin.New + SetupRoutes）打印 r.Routes()，268 条，输出曾在 /tmp/gin_routes.txt
- 真库表清单：docker exec syntopica-postgres psql -tAc "SELECT table_name FROM information_schema.tables WHERE table_schema='public'"，86 张
- API 文档端点提取陷阱：文档用三种形态记录端点（行内反引号 / 表格 `| GET | /api/x |` / 标题 `## GET /x` 简写相对前缀），单一正则必漏——终判用「路径静态段子串搜索」兜底

## 9 项落后事实（含证据定位）
- A1: dataenrichment.md L42-44 表格 + L523-548 详情仍把 analysis-methods 当活端点；代码无此路由（09-27 dddc3408 restructure-settings-navigation 删除，无 410 stub）；同文档 L45-46 reference-roles 已按退役写法标注（「已退役，一律 410」），照此改写 analysis-methods
- A2: GET /api/daily-reports/today 代码存在（11-03 772f04da 引入），daily-reports.md 无记载（该文档 11-17 e33821d3 更新过仍未补）
- A3: GET|POST /analysis/pause（08-29 7b8792af 引入）、GET|POST /settings/bocha（08-24 引入）、GET|POST /settings/proxy（07-30 d74c1758 引入）零文档；对照写法：datasources.md L23-35（comtrade/fred）、discovery.md L164-191（rsshub/searxng）
- B1: 真库 86 表中零文档痕迹 3 张：section_assignment_overrides、topic_anchor_overrides（均 11-17 e33821d3 unify-lane-workbench 引入）、tag_category_meta（2026-03-14 1f4c8377 引入，模型 TagCategoryMeta 在 tagmanagement/models/tag_models.go）
- B2: database/_index.md 完整表清单缺 16 张：board_health_snapshots、board_signal_disposition、board_signal_research_progress、candidate_availability、candidate_embeddings、candidate_preferences、data_sources、discovery_interest_entries、discovery_run_items、discovery_runs、feed_candidates、section_assignment_overrides、tag_category_meta、topic_anchor_overrides、topic_lane_snapshots、user_preferences（前 13 张在域文档有记载仅索引没收；索引头部计数「65 业务 + 5 废弃 + 1 框架」已失真）
- C1: semantic-board.md L234/L472 仍写「热度计算入口/唯一实现 = story_heat.go（heat-v1）」；实际唯一活实现 lane_heat_repository.go（LaneHeatRuleVersion = "heat-v2-lane-clues-1d"），story_heat.go heat-v1 已停写
- C2: flow 文档 8 处失效前端引用——daily-report.md 三处 TagsPage.vue（已删，仅剩 useTagsPage.ts + pages/tags/index.vue）+ BoardDailyReportTimeline.vue（已删）+ SettingsSectionSearxng.vue（已删，剩 components/dialog/SearxngConfigPanel.vue）+ components/SelectionAskBubble.vue（迁至 components/daily-report/）+ tags/composables/useMarginNoteAdmin.ts（实际 settings/composables/）；topic-graph.md 两处 BoardThreadBrowser.vue（已删）+ tests/e2e/topic-graph.spec.ts（已删）+ daily-report/SectionLifecyclePanel.vue（实际 components/）。删除主因 eb5c9077 rework-workshop-into-radar-and-desk 与 dddc3408 restructure-settings-navigation
- C3: daily-report.md 溯源漏 fix-lane-metric-provenance（11-16）行 + lane-manual-correction-loop/unify-lane-workbench 两行死链（指 changes/<name> 应指 changes/archive/2026-11-16-…/2026-11-17-…，后者行内日期还写 11-16）；topic-graph.md 溯源末行停在 11-06（add-event-story-heat），漏 retire-heuristic-tag-fallback（11-12，提交 3ecdf3b7 就地改写红线 12/13）与 fix-lane-metric-provenance（11-16）+ lane-manual-correction-loop 死链 1 处
- C4: configuration.md 缺 4 环境变量：FINGENIUS_API_KEY（dataenrichment/service/fingenius_client.go:111）、COMPRESS_MIN_BYTES（platform/middleware/compress.go:89 默认 1KiB）、DEMO_READ_ONLY（app/runtime.go:102、router.go 只读中间件+跳过 ws）、RSSHUB_REWRITE（cmd/dump-sanitizer/sanitize.go:92）

## 已排除的疑似项（勿修）
- ai-admin.md providers 段 = 刻意迁移说明（明写已删除）；board-signals.md 相对路径简写 = 全覆盖；ui-navigation.md :boardId vs [boardId] 写法差异；api/_index.md 收录完整；工作区未提交文档改动 = unify-vector-column-dimension 进行中

<!-- pinned 2026-11-18T16:01:53Z -->
