## Context

见 `proposal.md`。当前事实：`POST /api/daily-reports/generate` 已接受 `date` 且有重建窗口守卫；调度器已有 `backfillMissingReports` 这一「只补空、旧到新、预算超了顺延」的范式可复用；缺口不可见与不可达是前端缺口。补档路径已有两处按执行时刻取数的穿越点（briefs 上下文、话题质心/状态快照），其中 briefs 违反既有「补档产物与当天生成等价」约束。

## Goals / Non-Goals

**Goals:**

- 缺口可见（只读）与缺口可补（异步、只补空、留档、通知）两件事各自闭环。
- 补填与既有生成共用一条流水线、一个闸门，不新增第二条生成路径。
- 补填运行结果（成功/部分失败/失败/超预算）持久留档且每次都通知。

**Non-Goals:**

- 不放开重建窗口（`tag_edge_retention_days`），不改标签边回收。
- 不对已有报告做重建（重建仍走既有入口 + 二次确认）。
- 不回溯历史话题质心与 `topic_status_at_report` 快照，不为此改造载重。
- 不做补档矩阵页/缺口日历，不改通知面板 UI。

## Decisions

### D1. 缺口判定复制生成时的候选集谓词

- **结论**：缺口 SQL 与生成 Step1 同谓词（active + event + `NOT direction_mismatch` + 跨板排他 `NOT EXISTS`），不用现成的宽松版 `CollectBoardIDsForDate`。
- **落点**：`backend-go/internal/topicgraph/service/daily_report_orchestrator.go:collectBoardTags`
- **取舍**：沿用 `CollectBoardIDsForDate` 更省事，但它不看 direction_mismatch 与排他分差，会把生成时必然被剔的板块算成缺口，导致补填反复空跑烧 LLM。

### D2. 补填是服务端异步运行，不靠前端循环调生成接口

- **结论**：新增 `POST /api/daily-reports/backfill-gaps`，受理即返回 `job_id`，服务端后台串行执行；「一次补齐」与「补这天」都走这一端点。
- **落点**：`backend-go/internal/topicgraph/handler/daily_report_handler.go`
- **取舍**：前端逐日调 `POST /generate` 可零后端改动，但换页即断、进度只能靠全局广播拼、重试与幂等逻辑落在浏览器；且与定时任务的闸门竞争要前端处理 409。

### D3. 只补空、旧到新、幂等

- **结论**：已存在报告的格子跳过；日期升序处理，保证 `prev_report_id` 链；任意中断后重跑只补仍缺的。
- **落点**：`backend-go/internal/admin/scheduler/job_daily_report.go:backfillMissingReports`
- **取舍**：一次补齐时直接覆盖能避免陈旧内容，但会删除重建 section/thread，页边注与人工锚定全部错位——重建必须由用户显式发起，不能藏在「补填」里。

### D4. 运行留档用独立表，不复用调度任务结果

- **结论**：新表 `daily_report_backfill_runs`（窗口、逐日明细 JSON、终态、started/finished、补出篇数、剩余缺口），缺口查询响应附带最近一次运行摘要。
- **落点**：`backend-go/internal/platform/database/postgres_migrations.go:postgresMigrations`
- **取舍**：塞进 `scheduler_tasks.last_execution_result` 不建表，但只能留最后一次、结构随调度器变动，且手动补填不走调度任务，留档会丢。

### D5. 通知复用既有终态通道，每次运行恰一条

- **结论**：补填终态调用通知服务的同类入口（新增补填终态入口），成功也发；与每日生成终态是两次独立运行，互不抑制。
- **落点**：`backend-go/internal/platform/notification/service.go:DailyReportTerminal`
- **取舍**：只发 WS 不落库，页面没开就永久丢失；逐格子发通知则会刷屏。

### D6. 整轮预算 60 分钟，耗尽即留档顺延

- **结论**：预算从 `ai_settings` 读（默认 60）；耗尽停止启动新生成，已完成的保留，终态 `budget_exhausted` 并记剩余缺口。
- **落点**：Scenario「预算耗尽留档顺延」
- **取舍**：不限时跑完能一次补干净，但会长时间占用闸门并压住当晚主轮；按格子计预算则切片过碎。

### D7. 补档 prompt 上下文以报告日为窗口

- **结论**：话题近期内容上下文（briefs）的计算基准从执行时刻改为 `period_date`，窗口与同日排除都相对它。
- **落点**：`backend-go/internal/topicgraph/repository/daily_report_repository.go:ListTopicRecentBriefs`
- **取舍**：不修只在文档标注，代价是补出的历史日报会看到「未来」章节，直接违反既有「补档产物与当天生成等价」约束。

### D8. 缺口取数是一次性请求，不进轮询载荷

- **结论**：状态块挂载/补填结束/跨日时各取一次缺口；不新建周期轮询，也不塞进雷达总览的轮询载荷。
- **落点**：`front/app/features/tags/composables/useRadarPipelineStatus.ts`
- **取舍**：并入总览轮询载荷可少一个请求，但每次轮询都要跑一遍 7 天缺口 SQL，违背状态块「不新建周期性请求」的既有约束。

### D9. 重新生成用全局确认弹窗做二次确认

- **结论**：勾选清单确认后，再走 `useConfirm` 的危险确认；取消则不发请求。
- **落点**：`front/app/features/tags/components/console/BoardRadarStatusBlock.vue`
- **取舍**：把警告做成第二次点击（内联二次确认）更轻，但用户明确要求二次确认弹窗，且弹窗能承载「页边注与人工锚定会错位」这句必须被看见的话。

### D10. 进度事件加 `date` 字段；补填终态用独立事件类型

- **结论**：逐格子进度复用 `daily_report_progress`（新增 `date`、`run_scope=backfill`）；补填终态用独立类型 `daily_report_backfill_done`（含 `run_id`、四态 `status`、filled/failed/remaining、窗口）。
- **落点**：`backend-go/internal/topicgraph/handler/daily_report_handler.go:buildProgressMessage`
- **取舍**：补填终态也复用 `daily_report_done`（新增附加字段）更少事件类型，但那个事件被前端无 job_id 过滤地当成本次今日生成结果消费，复用会让补填终态污染当日状态；新开一套完整 `daily_report_backfill_*` 进度事件则前端要双份订阅。

## Risks / Trade-offs

- [补档用当前质心与当前话题状态，锚定紧实度与快照语义与当天生成不完全等价] → 在 flow 文档标注口径；不回溯历史（回溯需要质心历史表，属另一 change）。
- [缺口假阳性残留：无 board label 的 fallback 匹配路径不参与缺口判定] → 与现有 `CollectBoardIDsForDate` 同盲区，不新增问题；真出现空跑按 Open Question 处理。
- [60 分钟可能补不完 7 天 × 多板] → 留档 `budget_exhausted` + 剩余缺口，重复触发即续跑；UI 明示「补齐会消耗额度」。
- [补填占用闸门，可能压住当晚主轮] → 与既有生成互斥且预算受限；跨零点未补完由调度器下一轮继续。
- [重建语义仍具破坏性] → 本次只加二次确认，不改变后端重建行为。

## Migration Plan

新表由版本化迁移创建（只增不改），老数据无需回填；`gaps` 在无运行记录时缺省 `last_run`。回滚只需回退二进制，新表留存无害。

## Open Questions

- 某个格子反复出现「有内容但补出空报告」时是否需要一张尝试记录表来止损？当前靠严口径把假阳性压到最低，先不建表。
