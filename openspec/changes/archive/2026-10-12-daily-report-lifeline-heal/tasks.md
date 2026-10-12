# Tasks: daily-report-lifeline-heal

## 1. 容错版补齐函数（service 层）

- [x] 1.1 在 `backend-go/internal/dataenrichment/service/lifeline_context.go` 新增容错补齐函数（如 `HealMissingTolerant`）：逐 (topic, period) 补齐，单点失败计数继续不中断，返回 (healed, failed, error) 汇总；既有 `HealMissing` 行为不变。验证：`cd backend-go && go test ./internal/dataenrichment/service -run TestHealMissingTolerant -v` 通过
- [x] 1.2 容错函数单测（mock LLM）：缺行补齐、单 topic LLM 失败后续 topic 仍补、已存在行不被重写、无缺口时零 LLM 调用（mock 调用计数断言）。验证：同 1.1 命令全绿

## 2. 日报 job 尾部挂载

- [x] 2.1 `backend-go/internal/dataenrichment/wire.go` 暴露 topic lister 获取入口（getter 或导出构造），不改既有 `GetLifelineService` 签名。验证：`cd backend-go && go build ./...` 通过（`GetTopicLister` 已存在；另在 root 包 scheduler_jobs.go 补了 GranularityMonth/Year re-export，满足 admin 包 depguard 边界规则）
- [x] 2.2 `backend-go/internal/admin/scheduler/job_daily_report.go` 在 tail steps 后追加异步 lifeline heal goroutine：recover + 日志（healed/failed 计数、粒度、耗时），month+year 两粒度各一轮；JobResult.Data 增加 `lifeline_heal: {triggered: true}` 痕迹字段。验证：`cd backend-go && go test ./internal/admin/scheduler -run TestDailyReport -v` 通过（含新增触发断言）
- [x] 2.3 job 层单测：日报 job 成功路径触发 heal（注入 fake heal 函数断言调用）、heal panic 不影响日报 job 返回、日报主流程失败时不触发 heal。验证：同 2.2 命令全绿

## 3. 既有调度回归守护

- [x] 3.1 跑既有 `lifeline` 调度测试确认 `HealMissing` 原行为未回归（`scheduler_jobs_test.go` 既有用例不改仍绿）。验证：`cd backend-go && go test ./internal/dataenrichment` 通过

## 4. Review

- [x] 4.1 派 change-review-glm 子线程聚焦 review（审查全部制品 + 后端实现 diff + 两份 flow 文档），8 条意见按 High/Medium/Low 逐条处置（R-1/R-2 已修复，R-3~R-7 已驳回，R-8 误报丢弃）后 review.md 建账。验证：`bash scripts/harness/review-trace.sh openspec/changes/daily-report-lifeline-heal` 退出码 0（实跑通过：建账 8 条格式合法）

## 5. 测试

- [x] 5.1 编写 `test-cases.md`（complex 档强制）：主链路故事表（串 specs 新增 5 个 Scenario 的节拍落点）、五组变体走查、⓪ 继承与调整表（`bash scripts/harness/test-assets.sh data-enrichment` 反查旧 Scenario×处置×旧测试）、白盒附加节（容错分支表+period 边界）。验证：文件存在于本 change 目录且五问句逐项有答案（白盒分支已全部落地为自动化用例，含同 topic 跨 period 容错续补）

## 6. 文档

<!-- doc-impact: flow -->
<!-- doc-impact-excuse: database=无表结构/迁移变更；复用既有 UpsertTopicLifelineContext 写路径，零新表新列 -->

- [x] 6.1 `docs/reference/flow/scheduler.md`：daily_report job 职责追加「尾部 lifeline 月/年缺口补齐（异步、顺延不丢）」描述；归档后按 §12 补变更溯源行。验证：`grep -n "lifeline" docs/reference/flow/scheduler.md` 能命中新增描述
- [x] 6.2 `docs/reference/flow/data-enrichment.md`：循环 A 触发时机表补充「每日日报成功后（month/year 粒度检查自愈）」一行；§308 代码入口补容错函数与新挂载点。验证：`grep -n "每日补齐\|日报成功后" docs/reference/flow/data-enrichment.md` 命中

## 7. 验证

- [x] 7.1 `cd backend-go && golangci-lint run ./internal/dataenrichment/... ./internal/admin/scheduler/...` → 0 issues
- [x] 7.2 `cd backend-go && go vet ./internal/dataenrichment/... ./internal/admin/scheduler/...` → 无输出
- [x] 7.3 `cd backend-go && go test ./internal/dataenrichment ./internal/dataenrichment/service ./internal/admin/scheduler` → 全部 ok（影响包，不跑全量）。注：`TestInvestigateBoardQuestion_PersistsFullInvestigation`（调查链，非本 change 文件、先行于本 change 测试执行且单独跑亦红）已按纪律登记测试欠账台账（context=daily-report-lifeline-heal）后继续；本 change 新增/既有用例全部绿
- [x] 7.4 `cd backend-go && go build ./...` → 编译通过
- [ ] 7.5 人工：部署后当晚观察日志 `lifeline heal` 行出现 healed 计数 > 0（首轮回填历史缺口），次日观察 healed=0 稳态（`grep "lifeline heal" <后端日志>`）；一周后 `psql` 查 `topic_lifeline_context` 中 2026-10 月期行数与 active 话题数一致

Scenario→测试映射（delta MODIFIED「循环 A 触发与汇总算法」共 9 个 Scenario；前 4 个为旧 Scenario 保留，映射既有测试）：

| Scenario | 测试文件 |
| --- | --- |
| 每 period 独立汇总 | backend-go/internal/dataenrichment/service/lifeline_context_test.go |
| 历史周期可翻 | backend-go/internal/dataenrichment/service/lifeline_context_test.go |
| 检查自愈补遗漏 period | backend-go/internal/dataenrichment/service/lifeline_context_test.go |
| 用户手动首次生成缺失 period | backend-go/internal/dataenrichment/handler/handler_test.go |
| 日报成功后补齐月报缺口 | backend-go/internal/admin/scheduler/job_daily_report_test.go backend-go/internal/dataenrichment/service/lifeline_heal_tolerant_test.go |
| 月初定时触发被跳过后由每日补齐兜底 | backend-go/internal/admin/scheduler/job_daily_report_test.go backend-go/internal/dataenrichment/service/lifeline_heal_tolerant_test.go |
| 补齐失败顺延不丢 | backend-go/internal/dataenrichment/service/lifeline_heal_tolerant_test.go |
| 稳态零成本 | backend-go/internal/dataenrichment/service/lifeline_heal_tolerant_test.go |
| 年报年中新增话题由每日补齐覆盖 | backend-go/internal/dataenrichment/service/lifeline_heal_tolerant_test.go |
