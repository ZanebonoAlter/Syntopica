## 1. 域闸本体（internal/topicgraph）

- [x] 1.1 新增 `internal/topicgraph/service/generation_gate.go`：`TryAcquireDailyReportGeneration() bool` / `ReleaseDailyReportGeneration()`（`atomic.Bool` CAS，非阻塞），域根 `wire.go` re-export 转发（apply 阶段用户拍板：闸放 service 子包，域根包 import handler、handler 反向 import 域根即循环导入）；配 `generation_gate_test.go`：首次 Try 成功、占用中第二次 Try 失败、Release 后再 Try 成功、并发 100 goroutine Try 恰好一胜。验证：`go test ./internal/topicgraph/service -run TestDailyReportGenerationGate` 全绿

## 2. 调度路径接闸（internal/admin/scheduler）

- [x] 2.1 `DailyReportJob` 闭包体开头 TryAcquire（经域根 re-export `daily_report.TryAcquireDailyReportGeneration`），失败返回 `JobResult{skipped: "already_running"}`；成功则 `defer Release`，位置先于 `waitForDailyReportWindow`（窗口等待期占闸）；闸用包级变量留 seam 供测试替换。验证：`go test ./internal/admin/scheduler -run DailyReport` 中新增用例——闸被占时 job 返回 skip 且不调 `generateAndSaveReport`

## 3. 手动端点接闸（internal/topicgraph/handler）

- [x] 3.1 `triggerGenerateDailyReport` 在 rebuild-window 校验后 TryAcquire（`service.TryAcquireDailyReportGeneration`，handler 已 import service），被拒返回 `409 {success:false, error:"日报生成正在执行中，请稍后再试。", reason:"already_running"}`；Release 挂进 `generateSingleBoard`/`generateAllBoards` 的 defer（含 panic 路径）。验证：handler 层测试——闸被占时请求端点得 409 且 `generateAndSaveReport` 未被调用；空闲时受理返回 `processing`

## 4. 测试（故事层对账）

- [x] 4.1 按 `test-cases.md` 主链路四节拍补齐/对账用例（单测 + handler 层），执行 `bash scripts/harness/test-assets.sh daily-report-system` 反查旧资产并填「继承与调整」表。验证：`go test ./internal/topicgraph/... ./internal/admin/scheduler/...` 故事相关用例全绿

## 5. 文档

<!-- doc-impact: daily-report, scheduler, flow, api -->
<!-- doc-impact-excuse: database=无 schema 变更（proposal 明确），启发式命中来自测试文件的 AutoMigrate 测试基建（setupDailyReportJobTest/setupSessionTestDB 同类） -->

- [x] 5.1 `docs/reference/flow/daily-report.md`「业务约束与不变量」节新增红线句：日报生成管线全局互斥（调度/手动同一把域闸），重复触发 409 `already_running` 且不发第二条终态通知；`docs/reference/flow/scheduler.md` 对应节补一句指向。验证：`grep -n "全局互斥" docs/reference/flow/daily-report.md docs/reference/flow/scheduler.md` 两处命中
- [x] 5.2 `docs/reference/api/daily-reports.md` POST `/daily-reports/generate` 节补「防重入闸」段：409 `already_running` 错误体契约（与调度路径同闸同文案，`flow/daily-report.md` 约束 26 指向）

## 6. 验证

- [x] 6.1 `cd backend-go && go build ./...` → 期望编译零错误
- [x] 6.2 `cd backend-go && go vet ./internal/topicgraph/... ./internal/admin/scheduler/...` → 期望零告警
- [x] 6.3 `cd backend-go && golangci-lint run ./internal/topicgraph/... ./internal/admin/scheduler/...` → 期望零告警
- [x] 6.4 `cd backend-go && go test ./internal/topicgraph/... ./internal/admin/scheduler/...` → 期望全部 PASS（含 1.1/2.1/3.1 新增用例与 4.1 故事用例）
- [x] 6.5 `bash scripts/harness/change-scope.sh` 核对影响包清单与 6.4 一致 → 期望无遗漏包
- [x] 6.6 门禁复跑：`archive-readiness.sh` 四项全绿（含本节映射表对账 scenario-trace）。Scenario → 测试资产映射（daily-report-system ADDED Requirement）：

| Scenario | 测试文件 |
| --- | --- |
| 触发单板日报生成 | backend-go/internal/topicgraph/handler/daily_report_handler_test.go |
| 触发全板日报生成 | backend-go/internal/topicgraph/handler/daily_report_handler_test.go |
| 生成进行中重复手动触发 | backend-go/internal/topicgraph/handler/daily_report_handler_test.go |
| 生成进行中调度到期 | backend-go/internal/admin/scheduler/job_daily_report_test.go |
| 管线结束后闸释放 | backend-go/internal/topicgraph/service/generation_gate_test.go, backend-go/internal/topicgraph/handler/daily_report_handler_test.go, backend-go/internal/admin/scheduler/job_daily_report_test.go |
| 手动触发与定时调度互斥不产生重复通知 | backend-go/internal/topicgraph/service/generation_gate_test.go |

（对应用例：`TestTriggerGenerateDailyReportAlreadyRunning`/`TestDailyReportJobScheduledPathSkipsWhenGateHeld`/`TestDailyReportJobSkipsWhenGateHeld`/`TestTriggerGenerateDailyReportReleasesGateAfterRun`/`TestDailyReportJobReleasesGateAfterRun`/`TestGenerateSingleBoardReleasesGateOnPanic`/`TestDailyReportGenerationGate`（含并发 100 goroutine 恰好一胜）；单板/全板受理路径由既有 `TestTriggerGenerateDailyReportRetentionGuardAllowsInsideWindow` 等继承覆盖。）
