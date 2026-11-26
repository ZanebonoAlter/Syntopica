# Review 建账 — fix-board-health-triage-degrade

审查方式：派子线程 `change-review` 只读审查（制品 + `git diff -- backend-go/`），重点核查「补齐不伪造 / 覆盖率判定 / 守卫不误杀 / 预算与隔离 / 测试有效性 / 契约一致性」六条。审查者额外跑了单条 `go test -run`、`go vet`、`openspec validate`，未改文件、未写库。

结论：有条件通过（1 High + 4 Medium + 5 Low）；全部处置如下。

### R-1 2026-11-04【High】回填触发在日报 job 内同步执行，与 delta spec「回填触发 SHALL 异步」冲突

- **状态**：已修复
- **问题**：spec 要求「手动端点与回填的触发 SHALL 异步执行（不影响主 job 返回值）」，实现却在 job 函数体内同步调用。风险路径：主轮有失败版面时尾步被 `failedCount==0` 跳过，回填这一轮会真跑满 30 分钟 LLM，把 job 返回拖住。
- **处置**：`job_daily_report.go:runBoardHealthCheckupAfterBackfill` 改为 goroutine：`context.Background()` 派生 30 分钟预算、goroutine 边界 recover（gosec G118 已标注理由）。`TestDailyReportJobBackfillTriggersGuardedCheckup` 改 `require.Eventually` + `sync/atomic`；负向用例补 200ms 窗口确认「确实没触发」。
- **证据**：`backend-go/internal/admin/scheduler/job_daily_report.go`（异步启动）、`job_daily_report_test.go`（Eventually 断言）；`go test -short ./internal/admin/...` 绿。

### R-2 2026-11-04【Medium】新鲜度守卫不看 `llm_score`，降级轮把该板标记为「新鲜」→ 6h 内无法重跑

- **状态**：已修复
- **问题**：降级轮照样写 `checked_at` 快照（`llm_score` 为空），守卫按 `MAX(checked_at)` 判新鲜 → 「没有依据」的板反而被锁 6 小时，与 change 目的相拗。
- **处置**：守卫 SQL 加 `llm_score IS NOT NULL`（`run.go:boardsCheckedRecently`）——降级快照不挡重生。同步改 delta spec（守卫条文 + 新增 Scenario「降级快照不把板块算成新鲜」）、flow 红线 27 与正文、design D3、test-cases 变体表；新增用例 `TestRunCheckupGuardIgnoresDegradedSnapshot`。
- **证据**：`backend-go/internal/tagmanagement/service/boardhealth/run.go`、`run_test.go`；PG 用例通过。

### R-3 2026-11-04【Medium】降级板轮次日志 `covered=` 恒为 0，与同一行 reason 自相矛盾

- **状态**：已修复
- **问题**：覆盖率不足时真实覆盖是 68/96，但结构化字段打 `covered=0`（`adjudicateBoardHealth` 在 error 时把 coverage 整个丢弃），而 reason 里写着 `covered=68/96`。
- **处置**：`normalizeTriage` 改为 `(items, covered, err)`，覆盖率不足时回传真实 `covered`；`adjudicateBoardHealth` 在 error 路径也带 `triageCoverage{Labels, Covered}`；`run.go` 降级分支写入 `outcome.Covered`。覆盖率错误文案统一为 `triage covers N/M (x%) below min coverage 50%`（带阈值，对齐 design D2）。
- **证据**：`llm.go:normalizeTriage`、`run.go:adjudicateBoardHealth/checkupBoard`；`TestNormalizeTriageCoverage` 增断言「降级路径也要回传真实覆盖数」，`TestRunCheckupDegradedBoardLogsReason` 断言完整文案。

### R-4 2026-11-04【Medium】「三条路径共用同一新鲜度守卫」表述与实现（主轮零守卫）三方不一致

- **状态**：已修复
- **问题**：flow 红线 27 与 delta spec 写「三条路径共用守卫」，实现（与 design D3）给主轮尾步保留零守卫，flow 同节 ASCII 图又写「零守卫」。
- **处置**：定为「主轮零守卫（每夜必须有当日依据），回填与手动走 6h 守卫」，理由写进 design D3：宁可多跑一轮 LLM，也不能因为白天手动生成过就整夜没有当日依据。同步改 delta spec（守卫范围 + 新增 Scenario「主轮尾步不受新鲜度守卫约束」）、flow 正文/红线 27/变更溯源、test-cases 变体表；新增用例 `TestRunCheckupZeroGuardSkipsNothing` 已有覆盖。
- **证据**：`openspec/changes/fix-board-health-triage-degrade/specs/board-health-checkup/spec.md`、`design.md` D3、`docs/reference/flow/semantic-board.md` 红线 27。

### R-5 2026-11-04【Medium】test-cases 承诺的「手动连续点两次 → 第二次跳过」无落点

- **状态**：已修复（落点修正，非新增代码）
- **问题**：该变体落到 handler 层，但 handler 测试用 stub seam 替换了体检入口，守卫在 handler 层不可观测。
- **处置**：守卫只在体检入口实现，不要求 handler 重复实现。test-cases 该行改指 `boardhealth/run_test.go`（守卫层），handler 层保留「seam 被异步调用 + scope 正确」断言，并显式注明分工。
- **证据**：`openspec/changes/fix-board-health-triage-degrade/test-cases.md` §2 幂等表。

### R-6 2026-11-04【Low】delta spec 数量词自相矛盾（「三条路径」却说「四者」）

- **状态**：已修复
- **处置**：改为「三者」。

### R-7 2026-11-04【Low】panic / 超时板的轮次日志 `labels=0`，与真实构成标签数不符

- **状态**：已修复
- **处置**：把 panic 回收下沉到 `checkupBoard`（命名返回值 + defer recover），采集完成后的 panic 仍能报出真实 `outcome.Labels`；`checkupBoardSafely` 的 recover 保留为第二道兜底。
- **证据**：`backend-go/internal/tagmanagement/service/boardhealth/run.go`。

### R-8 2026-11-04【Low】降级板每次打两行日志，前一行不符合每板一行口径

- **状态**：已修复
- **处置**：删除 `checkupBoard` 内那条 `Warnf`，降级原因统一由 `runCheckup` 的每板一行输出（`reason=`）。

### R-9 2026-11-04【Low】覆盖率不足 reason 未带阈值数值

- **状态**：已修复
- **处置**：文案改为 `triage covers 68/96 (70.8%) below min coverage 50%`，测试断言同步加「reason 要带阈值数值」。

### R-10 2026-11-04【Low】轮次日志 `boards=` 是守卫过滤后的板数，易误读

- **状态**：已修复
- **处置**：改为 `boards=<scope 总数> processed=<实际处理>`，测试断言同步更新。

## 审查者明确「审毕无发现」的范围

- 补齐项的类别/文案/落库形态（唯一构造点 + `convertTriage` 二次清空 suggestion + `metrics.triage` 唯一写点；前端 `insufficient` 走只读折叠组）。
- 覆盖率计算不会被非法/重复项抬高（致命集校验在覆盖率计算之前 return error）。
- 守卫 SQL 语义、BoardIDs 与守卫叠加、非 board/disabled 静默丢弃。
- 两级时间预算的 ctx 派生（scheduler 用 job ctx、handler 用 Background）与三处 panic recover。
- 测试判别力（四个点名用例在修复前代码上必然失败，断言到具体数值而非 not nil）。
