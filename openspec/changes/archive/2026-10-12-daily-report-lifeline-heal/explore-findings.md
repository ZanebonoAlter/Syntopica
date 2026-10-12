
## lifeline heal 尾步实现事实（挂载点/接缝/边界）

daily-report-lifeline-heal 实现事实（实现完成 13/14，剩 7.5 人工观察）：

**新增函数**：`service.LifelineContextService.HealMissingTolerant(ctx, granularity, now, lister) (healed, failed, err)` — `backend-go/internal/dataenrichment/service/lifeline_context.go`（HealMissing 正下方）。逐 (topic, period) 容错：SectionDates/列行/refreshArchive 三处失败均 failed++ continue；仅 lister 失败返回 error；返回计数汇总。HealMissing 原函数未动。

**挂载点**：`backend-go/internal/admin/scheduler/job_daily_report.go` 成功裁决块内 `failedCount == 0` 分支——`runBoardHealthCheckupTailStep(baseCtx)` 之后 `launchLifelineHealAsync()`（异步 goroutine，边界 recover）。接缝为两个包级 var：`runLifelineHealRound`（同步跑 month+year 两轮 HealMissingTolerant，无全局超时，nil service/lister 时 warn 跳过）、`launchLifelineHealAsync`（goroutine+recover）。JobResult.Data 增 `lifeline_heal: {triggered: true}`（仅触发时写入，healTriggered 局部旗标）。手动 TriggerNowWithDate 走同一闭包同触发。

**边界 re-export**：`dataenrichment/scheduler_jobs.go` 头部新增 `GranularityMonth/GranularityYear` 常量 re-export（admin 包 depguard 边界规则禁止深路径 import dataenrichment/repository；scheduler 侧只 import dataenrichment root）。

**测试**：service 包 `lifeline_heal_tolerant_test.go` 11 用例（funcAirouter 失败注入按 SessionID 前缀 `lifeline_context_<topic>_<gran>_`）；scheduler 包 job_daily_report_test.go 3 用例（healCapture 缝注入/失败不触发/panic 隔离经 done channel 同步）。

**已知外来红**：`TestInvestigateBoardQuestion_PersistsFullInvestigation`（board_investigation_persist_test.go:239 断言缺 `methods` 键）非本 change 引起——后端脏文件仅本 change 4+1 文件，且该用例按字母序先行于本 change 测试、单独跑亦红；已登记 test-patrol 台账（context=daily-report-lifeline-heal，status=open）。

**部署**：后端在跑旧代码（PID 993779），今晚 21:00 日报前需 `bash scripts/dev/start-dev.sh --restart`；首晚自动回填历史缺口（约一两百次本地 LLM 调用），观察日志 `lifeline heal`。

**引用**：backend-go/internal/dataenrichment/service/lifeline_context.go、backend-go/internal/admin/scheduler/job_daily_report.go、backend-go/internal/dataenrichment/scheduler_jobs.go、backend-go/internal/dataenrichment/service/lifeline_heal_tolerant_test.go

<!-- pinned 2026-10-12T12:09:01Z -->
