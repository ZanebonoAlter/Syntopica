<!-- complexity: simple -->
<!-- ui-impact: none -->
<!-- constraint-domains: daily-report, topic-graph, semantic-board, data-enrichment -->

## Why

2026-11-19 文档治理审计（方法：Gin 路由表 268 条 / 真库 86 表 / 环境变量全集 vs `docs/reference/` 机械对比 + flow 语义抽查）发现 9 项「文档落后实际代码」，集中在三个死角：**删除类清理不彻底**（端点/文件引用残留）、**汇总索引回填缺位**（`database/_index.md` 表清单）、**存量从未文档化**（老端点/老表/环境变量）。业务口径层（红线、停写标注、修正回路等 2026-11 新口径）五项语义抽查零矛盾，本 change 只还机械债，不改任何行为。

## What Changes（9 项，按层次分三组）

### A. API 层（`docs/reference/api/`）
- **A1** `dataenrichment.md`：`analysis-methods` CRUD（表格 42-44 行 + 详情 523-548 行）按已删除事实改写为退役说明——代码 2026-09-27 `restructure-settings-navigation` 移除且无 410 stub（对照 `reference-roles` 已有的退役写法）。
- **A2** `daily-reports.md`：补 `GET /api/daily-reports/today`（2026-11-03 `redesign-enrichment-research-workbench` 引入；从 handler 取真实行为/响应形态，禁止编造）。
- **A3** 端点文档化（放 `system.md` 或就近合适文档，以现有分域惯例为准）：`GET|POST /analysis/pause`（analysis handler，2026-08 引入）、`GET|POST /settings/bocha`、`GET|POST /settings/proxy`（对照 datasources.md / discovery.md 已有的 comtrade/fred/rsshub/searxng 写法）。

### B. Database 层（`docs/reference/database/`）
- **B1** 补 3 张零文档表（归入现有域文档 + `_conventions.md`）：`section_assignment_overrides`、`topic_anchor_overrides`（2026-11-17 `unify-lane-workbench` 引入，模型+用途见 tagmanagement/topicgraph repository）、`tag_category_meta`（2026-03 引入，`tagging/models/tag_models.go`）。
- **B2** `_index.md`「完整表清单」回填缺收的 16 张（board 信号域 3 张、discovery 域 6 张、`data_sources`、`user_preferences`、B1 三张 + 其余），头部计数「65 业务 + 5 废弃 + 1 框架」改为真实口径并留补齐记录行。

### C. Flow / 配置层（`docs/reference/flow/` + `configuration.md`）
- **C1** `semantic-board.md`：热度计算入口修正——现行唯一活实现是 `lane_heat_repository.go`（`heat-v2-lane-clues-1d`），`story_heat.go`（heat-v1）已停写（234/472 行两处「唯一实现」表述改写）。
- **C2** `daily-report.md` + `topic-graph.md` 代码入口 8 处失效前端引用回收/改路径（TagsPage.vue、BoardDailyReportTimeline.vue、BoardThreadBrowser.vue 已删；SelectionAskBubble.vue、useMarginNoteAdmin.ts、SectionLifecyclePanel.vue 目录漂移；`tests/e2e/topic-graph.spec.ts` 已删）。
- **C3** 变更溯源补链：`daily-report.md` 补 `fix-lane-metric-provenance` 行、修 lane-manual-correction-loop / unify-lane-workbench 2 死链（指向 archive/ 路径，后者日期 11-16→11-17）；`topic-graph.md` 末行 11-06 之后补 `retire-heuristic-tag-fallback`、`fix-lane-metric-provenance` 2 行 + 修 1 死链。
- **C4** `configuration.md` 补 4 个环境变量：`FINGENIUS_API_KEY`（dataenrichment/fingenius_client.go）、`COMPRESS_MIN_BYTES`（middleware/compress.go，默认 1KiB）、`DEMO_READ_ONLY`（demo 只读模式开关）、`RSSHUB_REWRITE`（cmd/dump-sanitizer 工具覆盖项）。

## Capabilities

- 无 spec delta（docs-only：行为零变化，`docs/reference/` 投影对齐代码现状）。

## Impact

- **纯文档**：不改任何代码、不改行为、无迁移。修复后 agent 消费文档（约束注入 / 派发 brief / 归档对账）拿到的路径与端点不再指向已删实体。
- **部署影响**：无。无需用户操作，无旧数据降级问题。
