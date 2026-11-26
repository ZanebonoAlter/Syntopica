# Review · 日报缺口可见 + 异步补填（add-daily-report-gap-backfill）

> 审查类型：openspec change 制品审查（只读）。
> 审查范围：proposal / design / tasks / ui-design / 三份 spec delta 与实现 diff 的逐条对账；
> 后端 `internal/topicgraph/{repository,service,handler}`、`internal/platform/{notification,database}`、
> 前端 `front/app/{api/dailyReports.ts,utils/eventTypes.ts,features/tags/composables/useRadarPipelineStatus.ts,
> features/tags/components/console/BoardRadarStatusBlock.vue,features/tags/components/DailyReportGenerateDialog.vue,
> composables/useDailyReportProgress.ts}`、文档 `docs/reference/{api/daily-reports.md,database/tables/daily-report-watch.md,flow/daily-report.md}`。
> 工作区脏（含其它 active change 的改动），本审查只对本 change 的文件下结论。
> 审查日期：2026-11-25。

### R-1 2026-11-25【Medium】`findPreviousReportBrief` 取的是「最早的前一份报告」而非「紧邻的前一天」——旧到新排序所要保的 `prev_report_id` 链实际不成立，且本 change 的 spec 场景 0 覆盖

- **状态**：已修复
- **处置**：`findPreviousReportBrief` 改为 `Order("period_date DESC")`（GORM `First()` 默认按主键 ASC 是最早一份）；新增 `service/daily_report_prev_report_test.go` 断言指向紧邻前一日、跳过非 completed、无更早报告返回 nil
- **位置**：`backend-go/internal/topicgraph/service/daily_report_orchestrator.go:801`（`findPreviousReportBrief`）；`backend-go/internal/topicgraph/service/daily_report_backfill.go:305`（`orderBackfillCells` 注释「旧到新是 prev_report_id 链成立的前提」）；`backend-go/internal/topicgraph/service/daily_report_backfill_test.go:125`（`TestRunGapBackfill_OldestFirst` 只断言调用顺序）
- **问题**：`findPreviousReportBrief` 用 GORM `First()`（语义 = `ORDER BY <主键> ASC LIMIT 1`）取「`period_date < 报告日` 且 `status='completed'`」的第一行，命中的是该板**最早**的一份历史报告，而不是紧邻报告日的那一份。已实测确认 GORM `First()` 取最小主键。于是：某板在窗口内更早的日子（如 11-01）已有报告时，本次补出的 11-20 的 `prev_report_id` 指向 11-01，而不是本次补出的 11-19 → design D3「旧到新是为了 `prev_report_id` 链」、service 注释所述目的、以及 spec「日报缺口异步补填 → Scenario: 从旧到新串行」的 `AND 11-20 的报告 prev_report_id SHALL 指向本次补出的 11-19 报告」三处在真实数据下都不成立（仅在「该板此前一份报告都没有」的最小样本里成立）。补填侧测试只断言了生成调用顺序，没有任何用例断言 `prev_report_id` 的实际指向，故这条偏差无人拦截。
- **建议**：二选一并落到制品上——(a) 把 `findPreviousReportBrief` 改为按 `period_date DESC` 取紧邻一份（`Order("period_date DESC")` + `First`），同时修掉当日生成路径的同款缺陷（该缺陷是既有的、与本 change 无关，但正是本 change 场景所依赖的语义，宜单独归档或显式声明一并修）；(b) 若「取最早」确为刻意语义，则把 spec 场景与 design D3 / service 注释改成不承诺「指向本次补出的前一天」，并把旧到新的理由改述为「保持补填产物与当日生成口径一致」。无论选哪条，都要补一条能断言 `prev_report_id` 实际指向的回归用例（当前为 0）。
- **状态**：已修复
- **处置**：`findPreviousReportBrief` 改为 `Order("period_date DESC")`（GORM `First()` 默认按主键 ASC 是最早一份）；新增 `service/daily_report_prev_report_test.go` 断言指向紧邻前一日、跳过非 completed、无更早报告返回 nil

### R-2 2026-11-25【Medium】spec delta 的缺口窗口规范句与实现 / flow 红线不一致：spec 写「`[今天 - tag_edge_retention_days, 今天 - 1]`」，实现是 `min(days, retention)` 且 `days` 默认 7

- **状态**：已修复
- **处置**：spec 窗口句改为 `[今天 - min(days, tag_edge_retention_days), 今天 - 1]`（`days` 默认 7、只收窄）；新增 `TestListDailyReportGaps_RetentionAboveDefaultDaysStaysAtSeven` 锁住「retention=30 时缺省仍 7 天」
- **位置**：`openspec/changes/add-daily-report-gap-backfill/specs/daily-report-system/spec.md`（ADDED「日报缺口可见性」的窗口规范句与「Scenario: 窗口外不返回」）；实现 `backend-go/internal/topicgraph/handler/daily_report_handler.go:1060`（`dailyReportGapWindow`）与 `:1085`（`days` 缺省 7）；flow 红线 `docs/reference/flow/daily-report.md:451` 约束 66；前端 `front/app/features/tags/composables/useRadarPipelineStatus.ts`（`GAP_WINDOW_DAYS = 7`，恒传 `days: 7`）
- **问题**：spec 的规范句是「窗口 SHALL 为 `[今天 - tag_edge_retention_days, 今天 - 1]`」「`days` 参数只能收窄该窗口」，实现（与 flow 红线 66、proposal「默认 7，上限受 `tag_edge_retention_days` 约束」一致）是 `min(days, retention)` 且 `days` 缺省 7。`tag_edge_retention_days` 是 `ai_settings` 可配项（默认 7）：配成 30 时，spec 字面要求 30 天窗口，实现只给 7 天，前端又恒传 `days: 7`，于是 UI 永远看不到 7 天以外的缺口（那 7 天是真缺口但不可见、不可补）。spec 的配套场景只覆盖了「retention=7 且 days=30 被收窄到 7」，恰好没能暴露这条落差。归档后 spec 会被同步进主 spec，这句规范会成为长期契约。
- **建议**：把 spec 的规范句改成「窗口 SHALL 为 `[今天 - min(days, tag_edge_retention_days), 今天 - 1]`（`days` 默认 7，只能收窄、不得越过保留窗口下界）」，与 flow 红线 66 与实现同口径；并补一条「`tag_edge_retention_days=30` 时 `days` 缺省仍只给 7 天窗口」的验收用例锁住口径。若产品本意是「缺省给满保留窗口」，则改实现 + 前端不固定传 7 —— 两侧择一，不要让 spec / flow / 实现各说各话。
- **状态**：已修复
- **处置**：spec 窗口句改为 `[今天 - min(days, tag_edge_retention_days), 今天 - 1]`（`days` 默认 7、只收窄）；新增 `TestListDailyReportGaps_RetentionAboveDefaultDaysStaysAtSeven` 锁住「retention=30 时缺省仍 7 天」

### R-3 2026-11-25【Low】预算耗尽时，逐日明细里「未轮到」的日子 remaining 记 0，与运行级 `remaining_count` 对不上

- **状态**：已修复
- **处置**：预算 `break` 后按未跑到的格子补齐逐日 `remaining`，使逐日之和 == 运行级 `remaining_count`；`TestRunGapBackfill_BudgetExhaustedKeepsCompleted` 增加逐日断言
- **位置**：`backend-go/internal/topicgraph/service/daily_report_backfill.go:194`（`newBackfillDayStats`）、`:200-205`（预算守卫 `break`）、`:264`（`result.Remaining`）、`:361`（`details`）；落库见 `backend-go/internal/topicgraph/repository/daily_report_backfill.go:278`（`FinalizeBackfillRun` 的 `remaining_count`）
- **问题**：`dayStats` 只在「处理到某格」时才记 filled / failed / remaining。预算耗尽 `break` 之后，尚未轮到的日子在 `daily_details` 里是 `{filled:0, failed:0, remaining:0}`，而那些格子的确仍是缺口。运行级 `remaining_count`（= `TotalCells - Filled - Skipped`）是对的，但逐日求和 < 它：按 `daily_details` 读留档的人（未来的「续跑」功能、运维排查）会以为那些日子没有缺口，与「剩余缺口写入运行结果」的留档意图相悖。
- **建议**：`newBackfillDayStats` 时按 cells 统计每格的待处理数，预算耗尽 `break` 之后把「未处理到的日子」的 remaining 一次性补齐，使逐日 `remaining` 之和 == 运行级 `remaining_count`；并在 service 层补一条「预算耗尽后逐日 remaining 之和 == `result.Remaining`」断言。
- **状态**：已修复
- **处置**：预算 `break` 后按未跑到的格子补齐逐日 `remaining`，使逐日之和 == 运行级 `remaining_count`；`TestRunGapBackfill_BudgetExhaustedKeepsCompleted` 增加逐日断言

### R-4 2026-11-25【Low】补填终态 WS 在「受理响应尚未到达」时被直接丢弃（逐格进度有缓冲、终态没有）

- **状态**：已修复
- **处置**：终态走与逐格进度同样的缓冲（`submitting` 期间缓存、拿到 `job_id` 后按 id 回放）；新增用例「补填终态先于受理响应到达：缓冲后回放，不永久卡 running」
- **位置**：`front/app/features/tags/composables/useRadarPipelineStatus.ts:384`（`onBackfillDone`）vs `:489`（`onProgress` 的 `bufferedBackfillProgress` 缓冲）；前端契约 `front/app/api/dailyReports.ts`（`DailyReportBackfillAccepted`）
- **问题**：逐格进度消息在 `backfillPhase === 'submitting'` 期间会被缓存、拿到 `job_id` 后回放；终态消息 `daily_report_backfill_done` 没有同等缓冲——`backfillJobId` 为空即 `return`。若终态先于 HTTP 响应抵达（零缺口轮次最快，仅需几次 SQL 往返即可收尾），UI 会永远停在 `running`：「补这天」「一次补齐」持续禁用，且终态是 `reloadGaps()` 的唯一触发点，缺口清单也不会刷新。用户只能刷新页面。
- **建议**：终态走与进度同样的缓冲语义（`submitting` 期间缓存 `daily_report_backfill_done`，拿到 `job_id` 后按 `job_id` 回放）；或退一步，在终态丢失时以一次 `reloadGaps()` 兜底并解冻按钮。补一条「终态先于受理响应到达」的组件用例锁住该时序。
- **状态**：已修复
- **处置**：终态走与逐格进度同样的缓冲（`submitting` 期间缓存、拿到 `job_id` 后按 id 回放）；新增用例「补填终态先于受理响应到达：缓冲后回放，不永久卡 running」

### R-5 2026-11-25【Low】手动补填不触发板块体检，与调度器补档 / 手动生成两条既有路径不一致

- **状态**：已采纳（不修）
- **处置**：补填不触发体检是刻意取舍（一次可能跨多板，逐板体检放大 LLM 成本；体检由每日主轮尾步负责），已在 spec「日报缺口异步补填」与 `flow/daily-report.md` §留档与通知 显式记口径
- **位置**：`backend-go/internal/topicgraph/handler/daily_report_handler.go:1368`（`runGapBackfill` 收尾只发通知 + 广播终态，不触发体检）vs `:397`（`launchBoardHealthCheckup`，手动生成路径）与 `backend-go/internal/admin/scheduler/job_daily_report.go:487`（`runBoardHealthCheckupAfterBackfill`，调度器补档路径）
- **问题**：`fix-board-health-triage-degrade` 确立的口径是「补出来的日报是用户可见的新依据，体检必须跟上」——手动生成会为产出板块补一次体检，调度器补档回填也会补一次。新的手动补填端点产出的是同一种日报，却不触发体检，手动补出来的报告因此不进体检证据，两条补填路径的体检口径分叉。本 change 的 proposal / spec 均未提及体检，故不构成 spec 违约，属实现与既有行为的落差。
- **建议**：补填终态时对本次补出的板块集合触发一次体检（沿用 `triggerBoardHealthCheckup` seam，自带新鲜度守卫，不额外烧额度）；若刻意不做，请在 proposal 与 `docs/reference/flow/daily-report.md` 里显式记一句「手动补填不触发体检」，避免后续把这条分叉当成 bug 反复排查。
- **状态**：已采纳（不修）
- **处置**：补填不触发体检是刻意取舍（一次可能跨多板，逐板体检放大 LLM 成本；体检由每日主轮尾步负责），已在 spec「日报缺口异步补填」与 `flow/daily-report.md` §留档与通知 显式记口径

### R-6 2026-11-25【Low】spec 写「按日分组的缺报板块」，实际响应是平铺 `gaps[]` 数组 + 计数，分组由前端做

- **状态**：已修复
- **处置**：spec 响应句改为「平铺的缺口格子（报告日 + 板块 id + 名称）+ 缺口总天数与总篇数」，按日分组归消费方（与 API 文档 / 实现同形）
- **位置**：`openspec/changes/add-daily-report-gap-backfill/specs/daily-report-system/spec.md`（ADDED「日报缺口可见性」的响应句）；实现 `backend-go/internal/topicgraph/handler/daily_report_handler.go:1127`（响应体 `gaps` 为平铺数组 + `missing_days` / `total_missing`）；API 文档 `docs/reference/api/daily-reports.md`（明确写「扁平列表，前端按 date 分组」）；前端 `front/app/features/tags/composables/useRadarPipelineStatus.ts`（`gapsByDate` 在前端分组）
- **问题**：spec 的响应契约句是「给出……**按日分组**的缺报板块（id + 名称）」，而实际契约是「平铺 `gaps[]`（按日期升序、同日按 `board_id` 升序）+ `missing_days` + `total_missing`」，按日分组由消费方自己做。语义可达，但契约描述与实际形状不符——归档同步进主 spec 后会成为「API 必须按日分组返回」的长期承诺，与已发布的 API 文档冲突。
- **建议**：把 spec 响应句改成「给出平铺的缺报格子列表（每格含报告日 + 板块 id + 名称，按报告日升序、同日按板块 id 升序）、有缺口的日期数与缺口格子总数」，与 API 文档 / 实现同形；分组口径留在消费侧说明里。
- **状态**：已修复
- **处置**：spec 响应句改为「平铺的缺口格子（报告日 + 板块 id + 名称）+ 缺口总天数与总篇数」，按日分组归消费方（与 API 文档 / 实现同形）

### R-7 2026-11-25【Medium】缺口区补填入口未沿用队列门槛：队列积压时按钮仍可点，点了必然 409

- **状态**：已修复
- **位置**：`front/app/features/tags/components/console/BoardRadarStatusBlock.vue`（缺口区 `补这天` / `一次补齐` 的 `:disabled` 原为 `backfillBusy`）vs 同卡片生成入口的 `generateDisabled`（`!pollReady || !queuesDrained || submitting`）；服务端 `triggerBackfillDailyReportGaps` 的双队列 409
- **问题**：同卡片里「立即生成今日日报」在队列积压时禁用并给缺料说明，而新加的补填入口只看「补填是否在跑」，不看队列。补填的典型触发场景（停机 / 故障恢复后补缺）恰好就是队列最可能堆着的时候——实测现场：标签队列 95 篇待打标，补填按钮可用，点下去 409。属实现期自查发现（不在首轮审查的 11 条断言范围内）。
- **处置**：`useRadarPipelineStatus` 新增 `backfillBlocked = !queuesDrained`，缺口区两个按钮并入该条件；缺口区补缺料说明，并区分「队列还没清空」与「队列状态未知」（Embedding 计数不可得时不得谎称未清空）。补 4 条组件/composable 用例。spec `radar-pipeline-status` 增补该门槛与 Scenario「队列积压时补填入口禁用」。
- **备注**：失败任务仍不计入活跃量（与服务端 `backfillQueuesDrained` 同口径），卡片上只以红字提示，不阻断——与既有生成门槛一致。
