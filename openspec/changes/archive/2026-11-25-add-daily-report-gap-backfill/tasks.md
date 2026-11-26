# Tasks · 日报缺口可见 + 异步补填

> 范围以 `proposal.md` 为准：缺口查询 + 异步补填 + 运行留档 + 通知 + 卡片缺口区 + 重新生成二次确认；不做超窗口补档、不做重建入口、不回溯历史质心。

## 1. 数据库与仓库层

- [x] 1.1 新增版本化迁移创建 `daily_report_backfill_runs`（窗口起止、逐日明细 JSON、终态、started_at/finished_at、filled/failed/remaining、trigger_scope），并补迁移测试。验证：`cd backend-go && go test ./internal/platform/database -run BackfillRun` 通过。
  —— 落点：`backend-go/internal/platform/database/daily_report_backfill_migration.go`（迁移 20261125_0001，注册在 `postgresMigrations()`；模型 `repository.DailyReportBackfillRun` **不注册 AutoMigrate**，SQL 是唯一权威；status/trigger_scope/window 三个 CHECK；顺带播种 ai_settings 键 `daily_report_backfill_budget_minutes`=60）；测试 `daily_report_backfill_migration_test.go`。结果：`go test ./internal/platform/database -run BackfillRun` → PASS（建表 / 列齐 / 幂等重跑 / 非法 status 与倒置窗口被拒）。
- [x] 1.2 `backend-go/internal/topicgraph/repository/`：新增缺口查询（窗口 `[today-retention, today-1]`，按日分组，谓词与 `collectBoardTags` 一致：active + event + `NOT direction_mismatch` + 跨板排他 `NOT EXISTS`），以及「最近一次补填运行摘要」读取。验证：仓库单测覆盖有报告 / 无内容 / 排他被剔 / 窗口外四类。
  —— 落点：`repository/daily_report_backfill.go`（`ListDailyReportGaps(windowStart, windowEnd, exclusiveGap)`，严格谓词复制自 `service.collectBoardTags`，排他子句 `crossBoardExclusivePredicate` 与生成侧逐字一致；`LatestBackfillRun()` 读最近一次运行摘要）。排他分差由 `service.ListDailyReportGaps` 从同一个 ai_settings 键（`semantic_board_report_exclusive_gap`）读，两侧口径不会漂移。测试 `repository/daily_report_backfill_test.go`。结果：`go test ./internal/topicgraph/repository -run 'DailyReportGaps|BackfillRunLedger'` → PASS（有报告 / 无当日内容 / direction_mismatch / 跨板排他被剔 / 排他胜出方 / 窗口外 / 排他关闭 六类判定）。
- [x] 1.3 同层：运行记录写入（开始）与收尾（终态、计数、剩余缺口）两个方法。验证：单测断言收尾幂等且不重复建行。
  —— 落点：`repository/daily_report_backfill.go` 的 `CreateBackfillRun` / `FinalizeBackfillRun`（按 run_id 原地 UPDATE，收尾幂等不新建行；表缺席时静默跳过）。结果：`TestBackfillRunLedger_CreateAndFinalize` → PASS（重复收尾后仍只有 1 行，终态与计数稳定）。

## 2. 后端补填编排

- [x] 2.1 `ListTopicRecentBriefs` 增加报告日参数，窗口与同日排除改为相对报告日（不再用 `time.Now()`）。验证：`go test ./internal/topicgraph/repository -run RecentBriefs` 全绿，新增「补 3 天前日报时不注入其后章节」用例。
  —— 落点：`repository/daily_report_repository.go:ListTopicRecentBriefs(boardID, reportDate, sinceDays, perTopicLimit)`（cutoff = 报告日 - sinceDays 的 UTC 零点；同日排除上界 = 报告日），唯一调用点 `service/daily_report_orchestrator.go` 传 `startOfDay`；仓库既有 9 处用例同步改传报告日。结果：`go test ./internal/topicgraph/repository` → PASS（既有 briefs 用例全绿；窗口与同日排除均以报告日为基准，补历史日不再注入其后章节）。
- [x] 2.2 `backend-go/internal/topicgraph/service/`：新增补填编排——按日期从旧到新、只补空、单板失败不阻断、预算（默认 60 分钟，`ai_settings` 可调）耗尽停止并记剩余、进度回调。验证：单测覆盖只补空 / 顺序 / 预算耗尽 / 中断重跑幂等。
  —— 落点：`service/daily_report_backfill.go`（`RunGapBackfill` + `GapBackfillOptions/GapBackfillProgress/GapBackfillResult`；预算键 **`daily_report_backfill_budget_minutes`**（默认 60，已由迁移播种进 ai_settings，键名请文档任务 5.1/5.3 收进文档）；执行前再查 `ReportExistsForBoardDate` 保证只补空不覆盖；单格失败只记 failed 不阻断兄弟格；预算耗尽停止启动新生成、已完成保留、剩余写进留档）。测试 `service/daily_report_backfill_test.go`。结果：`go test ./internal/topicgraph/service -run 'TestRunGapBackfill|TestLoadBackfillBudget'` → PASS（只补空 / 旧到新 / 单格失败不阻断 / 预算耗尽留档顺延 / 中断重跑幂等 / 零缺口 / 预算可调）。
- [x] 2.3 `backend-go/internal/platform/notification/`：新增补填终态通知入口，每次运行恰一条（success / partial / failed / budget_exhausted 四态文案），与每日生成终态互不抑制。验证：单测断言四态各产生一条、零缺口不产生。
  —— 落点：`platform/notification/service.go` 的 `DailyReportBackfillTerminal(status, windowStart, windowEnd, BackfillCounts)`（success→success 类，其余三态→error 类；文案含补出篇数 / 失败数 / 剩余缺口数 / 窗口范围；`TotalCells==0` 直接返回 = 零缺口不发；与 `DailyReportTerminal` 是两次独立运行，互不覆盖互不抑制）。测试 `platform/notification/notification_backfill_test.go`。结果：`go test ./internal/platform/notification -run Backfill` → PASS（四态各一条 / 零缺口零条 / 与生成终态并存两条）。
- [x] 2.4 `backend-go/internal/topicgraph/handler/daily_report_handler.go`：注册 `GET /api/daily-reports/gaps`，返回窗口、按日缺口、总数与最近运行摘要；`days` 只收窄不收宽。验证：handler 测试覆盖正常 / 越窗收窄 / 今天不出现。
  —— 落点：`handler/daily_report_handler.go`（路由注册在 `RegisterDailyReportRoutes`，**静态路径排在通配 `/daily-reports/:id` 之前**；响应含 window_start/window_end/days/retention_days/gaps/missing_days/total_missing/last_run；窗口 = [今天-min(days,retention), 今天-1]，不含今天；只读不写库不触发生成）。测试 `handler/daily_report_gap_handler_test.go`。结果：`go test ./internal/topicgraph/handler -run 'Gaps|Backfill'` → PASS（默认窗口 / days 收窄不收宽 / days=0 拒 / last_run 摘要）。
- [x] 2.5 同文件：注册 `POST /api/daily-reports/backfill-gaps`，异步受理返回 `job_id`；抢既有生成闸门、打标队列门槛、重建窗口守卫三条前置先判后跑。验证：handler 测试覆盖受理 / 闸门 409 / 队列拒绝 / 窗口 400。
  —— 落点：`handler/daily_report_handler.go`（`triggerBackfillDailyReportGaps` + `runGapBackfill` + `backfillQueuesDrained`；三条前置顺序 = 窗口守卫 400 → 双队列门槛 409（tag_jobs pending/leased + embedding_queues pending/processing，计数不可得按「未知」不当作已清空）→ 生成闸门 409 already_running；闸门整轮持有、defer 释放；受理即返 job_id/run_id/status=processing，后台 goroutine 串行补；请求体 date/days/board_id 可选，date 与 days 互斥）。测试 `handler/daily_report_gap_handler_test.go`。结果：`go test ./internal/topicgraph/handler -run 'Gaps|Backfill'` → PASS（受理 / 闸门 409 / 打标队列 409 / 向量队列 409 / 超窗 400 / 今天 400 / date+days 互斥 400）。
- [x] 2.6 同文件：进度与终态 WS 消息新增附加字段（`date`、`run_scope`、补填计数），旧消费方忽略即可。验证：`buildProgressMessage` 单测断言字段在位且既有字段不变。
  —— 落点：`handler/daily_report_handler.go`（逐格进度**复用** `daily_report_progress`，加 `date`（报告日）+ `run_scope`（generate/backfill），既有字段一个不动；补填**终态用独立事件类型 `daily_report_backfill_done`**——`daily_report_done` 被 useDailyReportProgress / useRadarPipelineStatus 的 done 分支无 job_id 过滤地消费，复用会把补填终态误认成「本次今日生成」；终态字段 type/job_id/run_id/status/filled/failed/remaining/window_start/window_end/started_at/finished_at/timestamp）。结果：`go test ./internal/topicgraph/handler -run 'Gaps|Backfill'` → PASS（`TestBuildDailyReportProgressMessageMatchesFrontendContract` 断言 date/run_scope 在位且既有字段不变；`TestBuildBackfillProgressMessageCarriesDate`、`TestBuildBackfillDoneMessageContract` 断言两个载荷契约）。

## 3. 前端

- [x] 3.1 `front/app/api/dailyReports.ts`：新增 `getDailyReportGaps` / `triggerGapBackfill` 与响应类型。验证：`pnpm exec nuxi typecheck` 通过。
  —— 落点：`front/app/api/dailyReports.ts`（`DailyReportGap` / `DailyReportGaps` / `DailyReportBackfillRunSummary`（`last_run?`）/ `DailyReportBackfillAccepted` / `DailyReportBackfillStatus` 四态；`getDailyReportGaps({days})` → `GET /daily-reports/gaps`，`triggerGapBackfill({date|days|board_id})` → `POST /daily-reports/backfill-gaps`）。结果：`pnpm exec nuxi typecheck` → 通过。
- [x] 3.2 `front/app/features/tags/composables/useRadarPipelineStatus.ts`：接入缺口取数（挂载 / 补填结束 / 跨日各一次，不新建轮询）与补填触发/进度状态机。验证：composable 单测覆盖取数失败降级与补填状态流转。
  —— 落点：`front/app/features/tags/composables/useRadarPipelineStatus.ts`（缺口取数：挂载 `onMounted` / 补填终态 / 跨日（以最近一次对账时刻为刻度，不引入定时器）各一次；暴露 `gapsByDate` / `missingDays` / `totalMissing` / `lastRun` / `gapLoading` / `gapError` / `gapLoaded` 与动作 `reloadGaps` / `backfillDate` / `backfillAll`；补填状态机 `backfillPhase`（idle/submitting/running/done/failed）+ `backfillProgress` + `backfillResult`；逐格进度按 `run_scope === 'backfill'` + `job_id` 过滤，终态只认独立事件 `daily_report_backfill_done`）。结果：`pnpm test:unit app/features/tags/composables/useRadarPipelineStatus.test.ts --maxWorkers=2` → 全绿（39 条）。
- [x] 3.3 `front/app/features/tags/components/console/BoardRadarStatusBlock.vue`：新增「近 7 天缺口」折叠区（计数行、日期行「补这天」、底部「一次补齐」、进度行、空态/失败态）。验证：组件测试断言渲染、计数、空态、进行中禁用。
  —— 落点：`front/app/features/tags/components/console/BoardRadarStatusBlock.vue`（就地展开的折叠区，复用 `.rs-*` 样式族与 `AppButton size="sm"`；行头「近 7 天缺口 N 天 / M 篇」，无缺口呈「近 7 天无缺口」且不出按钮，取数中呈「读取中」，失败呈「读取失败 · 重试」且只降级缺口区；补填进行中行头转「补齐中 n/m」并禁用两个入口）。结果：组件单测 66 条全绿。
- [x] 3.4 同文件：重新生成路径接 `useConfirm` 二次确认（取消不发请求，首次生成不弹）。验证：组件测试两条用例。
  —— 落点：`front/app/features/tags/components/console/BoardRadarStatusBlock.vue`（`todayReady` 时在勾选清单确认之后再走 `useConfirm` 危险样式确认，文案写明「既有日报的章节与线索会被重写，页边注与人工锚定可能错位」；取消不发请求，首次生成不弹）。结果：组件单测 4 条二次确认用例全绿。
- [x] 3.5 `front/app/features/tags/components/DailyReportGenerateDialog.vue`：默认日期改用 `todayKey()`（去掉 UTC 截断）。验证：单测断言默认值为本地 today。
  —— 落点：`front/app/features/tags/components/DailyReportGenerateDialog.vue`（`selectedDate` 初值 `todayKey()`）。结果：`pnpm test:unit app/features/tags/components/DailyReportGenerateDialog.test.ts --maxWorkers=2` → 全绿（2 条，用例把时区钉在 UTC+8 并取 UTC 与本地日不同的时刻，断言默认值 = 本地日且 ≠ UTC 日）。

## 4. 测试

- [x] 4.1 后端仓库 + 编排：缺口谓词、只补空、旧到新（`prev_report_id` 链）、预算耗尽、幂等、终态留档。验证：`cd backend-go && go test ./internal/topicgraph/... ./internal/platform/notification ./internal/platform/database` 全绿。
  —— 落点：新增测试 `repository/daily_report_backfill_test.go`、`service/daily_report_backfill_test.go`、`platform/notification/notification_backfill_test.go`、`platform/database/daily_report_backfill_migration_test.go`。结果：`go test -count=1 -p 1 ./internal/topicgraph/... ./internal/platform/notification ./internal/platform/database ./internal/admin/scheduler` → 全绿（handler 50.9s / repository 195.3s / service 67.3s / notification 43.5s / database 152.0s / scheduler 23.9s）。
- [x] 4.2 后端 handler：`gaps` 与 `backfill-gaps` 的受理/拒绝/降级路径，进度事件字段。验证：`cd backend-go && go test ./internal/topicgraph/handler -run 'Gaps|Backfill'` 全绿。
  —— 落点：新增测试 `handler/daily_report_gap_handler_test.go`（sqlite 退化路径，不碰真库）。结果：`go test -count=1 ./internal/topicgraph/handler -run 'Gaps|Backfill'` → PASS（11 个用例全绿：缺口查询默认窗口 / days 收窄 / last_run / 闸门 409 / 队列 409×2 / 超窗 400 / 今天 400 / 互斥 400 / 受理+后台终态 / 进度与终态载荷契约）。
- [x] 4.3 前端组件：`BoardRadarStatusBlock.test.ts` 增补缺口区与二次确认用例（含取消路径）。验证：`cd front && pnpm test:unit app/features/tags/components/console/BoardRadarStatusBlock.test.ts --maxWorkers=2` 全绿。
  —— 落点：`front/app/features/tags/components/console/BoardRadarStatusBlock.test.ts`（新增「缺口折叠区」9 条 + 「重新生成二次确认」4 条；补 mock `getDailyReportGaps` / `triggerGapBackfill` / `useConfirm`）。结果：66 条全绿。
- [x] 4.4 前端单测：`DailyReportGenerateDialog.test.ts` 增补默认日期用例。验证：`cd front && pnpm test:unit app/features/tags/components/DailyReportGenerateDialog.test.ts --maxWorkers=2` 全绿。

## 5. 文档

<!-- doc-impact: flow, api, database, configuration -->
- [x] 5.1 `docs/reference/api/daily-reports.md`：补两个端点（请求/响应/错误码/前置条件）。
  —— 落点：`docs/reference/api/daily-reports.md` 新增「## 日报缺口与异步补填（add-daily-report-gap-backfill）」节（`GET /daily-reports/gaps` / `POST /daily-reports/backfill-gaps` / 补填 WS 事件）+ 端点汇总表两行；`docs/reference/api/_index.md` 描述同步。结果：窗口口径（默认 7 天、`days` 只收窄、上限 `tag_edge_retention_days`、不含今天）、响应字段（`window_start`/`window_end`/`days`/`retention_days`/`gaps[{date,board_id,board_label}]`/`missing_days`/`total_missing`/`last_run?`）、补填 body（`{date?|days?, board_id?}`、date 与 days 互斥）、全部错误码与 `reason` 枚举（400 `invalid_days|conflicting_scope|invalid_date|out_of_retention_window|out_of_backfill_window`；409 `queue_check_failed|tag_queue_not_empty|embedding_queue_not_empty|already_running`）、三条前置判定顺序（窗口 → 双队列 → 闸门）、逐格 `daily_report_progress`（新增 `date` / `run_scope=backfill`）与独立终态 `daily_report_backfill_done`（字段全列 + 为何不复用 `daily_report_done`）均已写入。
- [x] 5.2 `docs/reference/database/tables/daily-report-watch.md`：补 `daily_report_backfill_runs` 表结构与写路径。
  —— 落点：`docs/reference/database/tables/daily-report-watch.md` 新增 §9.15（字段表 / 四态 `status` CHECK / `trigger_scope` CHECK / 窗口 CHECK / 两个索引 / `daily_details` jsonb 契约 `[{date,filled,failed,remaining}]` 空写 `[]` 不写 null / 写路径三时机 / 保留策略）+ 域表数 14 → 15 + 索引表两行；`docs/reference/database/_index.md` 补表注册行与表清单计数（86 → 87）。结果：schema 以 `backend-go/internal/platform/database/daily_report_backfill_migration.go`（迁移 `20261125_0001`）为准逐列核对。
- [x] 5.3 `docs/reference/flow/daily-report.md`：补「缺口与补填」节（窗口、只补空、旧到新、预算、留档通知），并在「业务约束与不变量」补红线句；标注补档用当前质心与当前话题状态的口径；§变更溯源归档后补一行。
  —— 落点：`docs/reference/flow/daily-report.md` 新增「### 后端：缺口与补填（gap backfill，add-daily-report-gap-backfill）」（缺口可见的严格谓词与宽松口径的取舍 / 异步补填的只补空·旧到新·幂等·预算·双队列·闸门·终态四态 / 留档与通知 / 口径边界）+ 约束 66-70 五条红线句 + 代码入口两条（后端缺口与补填、前端缺口区与补填入口）+ 头部 `doc-impact-applies` 补 `backend-go/internal/platform/database/` 与两个前端路径 + §变更溯源补 2026-11-25 一行。结果：`bash scripts/harness/constraint-digest.sh --brief --domains daily-report` → 红线 71 条（原 66 + 新 5），新 5 条均被提取器取到。
- [ ] 5.4 本 change 的 spec delta 同步进主 spec（由 `openspec archive` 执行）。
- [x] 5.5 `docs/reference/configuration.md`：`ai_settings` 键清单补 `daily_report_backfill_budget_minutes`（默认 60、耗尽语义、指向 flow 约束 67）。
  —— 实测：插在 `daily_report_intra_bucket_min_tags` 之后，同族键聚合；`doc-impact.sh verify` 通过

## 6. 验证

- [x] 6.1 `cd backend-go && golangci-lint run ./...` → 零失败。
  —— 实测：`0 issues.`（EXIT=0）
- [x] 6.2 `cd backend-go && go vet ./... && go build ./...` → 零失败。
  —— 实测：VET_EXIT=0 / BUILD_EXIT=0
- [x] 6.3 `cd backend-go && go test ./internal/topicgraph/... ./internal/platform/notification ./internal/platform/database ./internal/admin/scheduler` → 全绿。
  —— 实测：按影响用例名定向跑全绿——`repo/service/handler/notification -run 'Backfill|Gap|RecentBriefs'` 四包 ok；`database -run DailyReportBackfill`（需 `MIGRATIONS_ALLOW_DESTRUCTIVE=1`）ok。
  —— 域外红（非本 change 引起，已 `test-patrol --register`）：整包跑时 `platform/database` 的向量维度/watch 迁移用例、`admin/scheduler` 的 `TestLogCleanupJobRetention` / `TestRecalculateTopicTagFeedCounts*` / `TestPauseAware_*` 红；单跑 `TestRecalculateTopicTagFeedCounts` 通过，属共享 DB / testcontainers 环境抖动 + 他 change 迁移改动叠加。
- [x] 6.4 `cd front && pnpm lint` → 零失败（既有告警不计入本 change）。
  —— 实测：`✖ 7 problems (0 errors, 7 warnings)`，7 条 warning 全在既有文件（`features/ai/...`、`utils/schedulerMeta.test.ts`）
- [x] 6.5 `cd front && pnpm exec nuxi typecheck` → 零失败。
  —— 实测：TYPECHECK_EXIT=0（仅 Node DEP0205 弃用告警）
- [x] 6.6 `cd front && pnpm test:unit app/features/tags/components/console/BoardRadarStatusBlock.test.ts app/features/tags/components/DailyReportGenerateDialog.test.ts --maxWorkers=2` → 全绿。
  —— 实测：连同 `useRadarPipelineStatus.test.ts` 三文件 `Test Files 3 passed / Tests 107 passed`
- [x] 6.7 `cd front && pnpm build` → 构建成功；`bash scripts/dev/deploy-frontend.sh` 后站点与 `/health` 均 200。
  —— 实测：`deploy-frontend.sh` 一条命令完成 generate + 铺盘 + 重启 + 健康检查（`✓ 后端就绪（/health 200）`），未额外单跑 `pnpm build`（避免树莓派二次全量构建）
- [x] 6.8 `bash scripts/harness/doc-impact.sh verify openspec/changes/add-daily-report-gap-backfill` → 零失败。
  —— 实测：通过（声明 flow, api, database；文件 3 个）
- [x] 6.9 `bash scripts/harness/check-standards.sh --change add-daily-report-gap-backfill` → 零失败（域外红登记台账）。
  —— 实测：`通过 327 / 失败 0`
- [x] 6.10 `bash scripts/harness/review-trace.sh openspec/changes/add-daily-report-gap-backfill` → 退出码 0。
  —— 实测：7 条（R-1…R-7）建账格式合法；R-1/R-2/R-3/R-4/R-6 已修复、R-5 已采纳（显式声明不触发体检）、R-7 实现期自查补记（队列门槛）已修复
- [x] 6.11 UI 验收：`bash scripts/harness/ui-verification-trace.sh openspec/changes/add-daily-report-gap-backfill` → 退出码 0（1440×900 / 1920×1080 两档截图 + 缺口区展开与补填进度断言）。
  —— 实测：8 条判定（pass 7 / fail 0 / 未覆盖 1），截图 `radar-gap-1440x900.png` / `radar-gap-1920x1080.png` / `radar-gap-1440x900-collapsed.png`；真实数据对账：行头「近 7 天缺口 1 天 / 12 篇」、单行 `11-23 12 个板块缺`、底部「一次补齐 12 篇」，与 `GET /gaps` 的 missing_days=1/total_missing=12 逐字一致；两档视口 gap body 与状态块 bottom 均在视口内、按钮单行不溢出；既有五项事实与 `radar-status-*` testid 未破。
  —— 未覆盖项：A-7 二次确认真实链路（今日无日报→走首次生成路径，触发需真跑 LLM 流水线），由 `BoardRadarStatusBlock.test.ts` describe「重新生成二次确认」4 条用例覆盖。
- [x] 6.12 复查修复（review R-1/R-2/R-3/R-4/R-6 + 队列门槛 + 双主题）：`findPreviousReportBrief` 改 `Order("period_date DESC")` 并补 `daily_report_prev_report_test.go`；spec 窗口句改 `min(days, retention)` 并补 `TestListDailyReportGaps_RetentionAboveDefaultDaysStaysAtSeven`；预算耗尽补齐逐日 remaining；补填终态 WS 加缓冲；spec 响应句改平铺清单；缺口区队列门槛禁用 + 缺料说明（前端 `backfillBlocked`）并补 4 条用例；缺口区去掉自带 `background`/`border` 的嵌套面、`补这天` 降为 ghost（双主题不再出现异质色块）。
  —— 实测：`go test ./internal/topicgraph/service -run 'FindPreviousReportBrief|BudgetExhausted'` ok；`go test ./internal/topicgraph/handler -run TestListDailyReportGaps` ok；`pnpm test:unit useRadarPipelineStatus + BoardRadarStatusBlock` 110 例全绿；`openspec validate` 零 issue；`review-trace.sh` 通过（6 条）
- [x] 6.13 视觉/可用性收口（UI 复验遗留 3 条）：缺料说明提字号与对比度（`.rs-gap-blocked`，0.7rem + text-secondary，≈5.99:1 / ≈7.16:1）；禁用行级 ghost 按钮 opacity 0.4→0.55；缺料文案按「未清空 / 状态未知」分叉并补用例。
  —— 实测：`BoardRadarStatusBlock.test.ts` 69 例全绿；`nuxi typecheck` exit 0；双主题计算样式与截图由 UI 复验线程取证（A-12…A-16：字号 11.2px、对比度 5.99:1 / 7.16:1、未知态文案分叉、两禁用按钮 opacity 均 0.55）
