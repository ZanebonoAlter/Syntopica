<!-- complexity: complex -->
<!-- ui-impact: minor -->
<!-- constraint-domains: daily-report, scheduler -->

## Why

日报只有当晚主轮一条生成路径。停机、那晚生成失败、或打标队列没排空，都会留下空洞，而用户既**看不见**缺口（任何列表里都不出现），也**点不到**缺口（阅读台日期下拉只列已有报告的日子，唯一的补填按钮藏在手打 URL 后面）。后端其实已经具备按日期生成、7 天重建窗口与调度器「只补空」回填，缺的是把缺口和触发口搬到用户面前，并让补填的结果留痕可查。

## What Changes

- 新增 `GET /api/daily-reports/gaps`：近 N 天（默认 7，上限受 `tag_edge_retention_days` 约束）「该板当天有候选内容、但没有报告」的按日缺口清单。候选口径与生成时的 Step1 谓词一致（status/category/direction_mismatch/跨板排他），避免补了还是空的假阳性。
- 新增 `POST /api/daily-reports/backfill-gaps`：**异步**、服务端按日期**从旧到新**串行、**只补空**、整轮 **60 分钟**预算；同一进程共用既有日报生成闸门；打标队列未排空时拒绝。
- 补填运行留档：新增 `daily_report_backfill_runs` 记录，保存窗口、每日结果、终态（success / partial / failed / budget_exhausted）、耗时；**成功与失败都发终态通知**（每次运行至多一条）。
- 总览管线状态块新增「近 7 天缺口」折叠区：计数、按日补、一次补齐、内联进度；复用既有卡片与折叠模式。
- 「重新生成今日日报」加二次确认（复用 `useConfirm` / `AppConfirmDialog`）：重建会删除并重建 section/thread，页边注与人工锚定会错位。
- 修正补档日期口径：`ListTopicRecentBriefs` 的窗口与同日排除改为相对**报告日期**（原先用 `time.Now()`，补历史日会把未来内容注入 prompt）。
- WS 进度事件新增 `date` 字段（附加字段，旧消费方忽略）。
- 修复 `DailyReportGenerateDialog` 默认日期用 UTC 截断（`toISOString`）导致本地 0–8 点默认成昨天。

## Capabilities

### New Capabilities

（无）

### Modified Capabilities

- `daily-report-system`: 新增缺口查询与异步补填契约（只补空、旧到新、幂等、60 分钟预算、运行留档）；补档路径的日期口径以报告日为准。
- `radar-pipeline-status`: 管线状态块新增缺口区与补齐入口；重新生成需二次确认。
- `notification-center`: 通知白名单加入「补填终态」，与每日生成终态通知互不覆盖。

## Impact

- 后端：`internal/topicgraph/{handler,service,repository}`（缺口查询、补填编排、运行记录）、`internal/admin/scheduler/job_daily_report.go`（共享闸门与 `generateAndSaveReport` seam）、迁移（新表）、`internal/platform/database`。
- 前端：`features/tags/components/console/BoardRadarStatusBlock.vue`、`composables/useRadarPipelineStatus.ts`（或新增 `useRadarGapStatus`）、`api/dailyReports.ts`、`DailyReportGenerateDialog.vue`。
- 数据库：新表 `daily_report_backfill_runs`（只增不改）；无既有表结构变更。
- 内容侧：补出来的历史日报用**当前**话题质心与 `topic_status_at_report` 当前状态，锚定紧实度与快照语义按此口径在 flow 文档标注（不回填历史）。
- 成本：补填每天每板都是一次完整 LLM 流水线，补齐会消耗额度，UI 需明示。
