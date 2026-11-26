<!-- complexity: complex -->
<!-- ui-impact: minor -->
<!-- constraint-domains: daily-report, topic-graph -->

## Why

域型泳道（`AGENT编程工具相关`、`LLM生态相关新闻`）把当天全部线索塞进一条 section：2026-10-23 板块 3030 的一条 section 带 18 个 tag / 17 条 thread 平铺（做游戏、插件、MCP、harness、Skill、安全、推理基础设施混在一起），读者得读完 17 条才能自己归类，读完也记不住"这一天这个领域在往哪几个方向使劲"。

根因是 section 的产生单位被写死成「话题 × 天」：`assembleLaneGroups` 按话题聚合成唯一分组，于是**话题有多宽，section 就有多宽**。近 14 天数据：每板块单条 section 最大 tag 数 3030=25、1980=15，而 1974（中东，叙事型泳道）只有 5——问题只出现在"域当泳道用"的板块，全站 51/621 条 section（8.2%）够得着"大筐"标准。

## What Changes

- **新增「泳道内主题分桶」步骤**：在 lane 分桶（归属）与 section 组装之间插入一次阈值触发的两阶段 LLM 调用（定组名 → 只做归属），把一条大 section 拆成 2-6 条**同话题**的 section。归属语义不变，只在话题桶内部加层次。
- **section 数据模型补桶字段**：`bucket_title`（主题方向名，可空）、`bucket_index`（同 `cluster_index` 内的桶序号）。同一 report 内 `cluster_index` 允许重复（兄弟桶共享），用 `bucket_index` 区分。
- **同一话题同一天多条 section 成为常规形态**：命中计数按「日」去重（`hit_count` 只 +1），质心按各 section 计入，泳道视图按天聚合。
- **新增 `POST /api/daily-reports/:id/rebucket`**：对指定 section 所在的桶组重跑分桶，替换该组 section 但复用 thread 行（关键事件注记随 thread 保留）；与同日重跑共用 `(board, date)` 事务锁串行化；`watch_*` 物化组拒绝重分桶。
- **前端（minor）**：阅读视图复用现有 `group.sections.length > 1` 渲染位显示桶（子卡标题 = 当日章节标题 + 主题方向徽标）；泳道视图与既有泳道布局**不改**；子卡标题栏新增一个「重新分桶」动作。
- **泳道动态排序口径改按天去重**：`展示范围与排序` 从"近 14 天 section 数"改为"近 14 天有锚定 section 的天数"，避免拆分把计数灌水、打乱排序。
- **降级与回滚**：任一阶段 LLM 失败 / 解析失败 / 校验失败 → 保持现状单 section，日报不失败；分桶产物不参与同日相似合并（防止被粘回一条）。

**不做**：子桶跨天身份（B 方案：`parent_topic_id` 两级话题树）、话题生命周期与归属算法变更、L1 阈值重新校准、泳道视图/页面结构重做（后续统筹优化另开 change）。

## Capabilities

### New Capabilities
无。

### Modified Capabilities
- `daily-report-system`: 生成编排流水线新增"泳道内主题分桶"步骤；`DailyReportSection` 数据模型新增 `bucket_title` / `bucket_index` 并允许同 report 重复 `cluster_index`；新增重新分桶 HTTP API；`section lane 归属标记` 明确同话题多 section 语义。
- `persistent-topic`: 同一 topic 在同一篇日报内可拥有多条 section 时的命中计数（按日去重）与质心口径（各 section 计入）。
- `board-lane-dynamics`: 泳道展示范围与排序口径由"锚定 section 数"改为"有锚定 section 的天数"（同日多条只计一次）。

## Impact

**后端**
- `backend-go/internal/topicgraph/service/`：新增分桶服务（两阶段 prompt + 确定性校验器）与 rebucket 组级重建服务；`daily_report_orchestrator.go` 插入 Step5.5
- `backend-go/internal/topicgraph/repository/`：`DailyReportSection` 模型加两列（+ migration）、`SaveReport` 组写入、`planLifecycle` 口径确认（已是集合去重）、重新分桶的组替换事务
- `backend-go/internal/topicgraph/handler/daily_report_handler.go`：新增 rebucket 路由与 DTO 字段透出
- 配置：`daily_report_intra_bucket_min_threads`(默认 6) / `daily_report_intra_bucket_min_tags`(默认 10) / `daily_report_intra_bucket_enabled`(默认开)
- LLM：`CapabilityDigestPolish` 路由，两个新 operation（`daily_report.bucket_theme_names` / `daily_report.bucket_assign`），每次 1~3s / ~2.2k tokens，预估 ~8 次调用/天

**前端**
- `front/app/features/tags/components/daily-report/DailyReportTopicSection.vue`：子卡头部显示桶名徽标 + 「重新分桶」动作
- `front/app/features/tags/components/daily-report/dailyReportMagazine.ts`：`groupSectionsByTopic` 组内按 `bucket_index` 排序
- `front/app/api/dailyReports.ts`：`DailyReportSection` 增加 `bucket_title` / `bucket_index`；新增 rebucket 客户端

**数据**
- `daily_report_sections` 加列（nullable / default 0），存量行 `bucket_index=0`、`bucket_title=NULL`，历史日报保持单 section 形态，无需回刷
- 无表结构重构、无历史数据迁移；`topic_key_event_markers.source_section_id` 在重新分桶后按 `source_thread_id` 重新派生
