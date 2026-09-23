# Phase 6b 实现报告：研究超时对齐 + 研究进展持久化（tasks 4.6/4.7）

> 2026-09-22 增量实现线程。用户拍板扩展「超时与预算对齐 + 进展持久化（断了不能白跑）」，依据 design §4（150 分钟+进展持久化段）、spec 两个 Requirement 三条新 Scenario、tasks 4.6/4.7。制品由 controller 更新完毕，本线程只做实现+测试+文档同步。未 commit/push/部署。

## ① 文件清单

**后端（Go）**

| 文件 | 改动 |
| --- | --- |
| `backend-go/internal/dataenrichment/handler/analysis_runner.go` | 新增常量 `signalResearchJobTimeout = 150 * time.Minute`（`analysisJobTimeout=30min` 不动，任务 4.6）；`launch` 的 job ctx 注入任务身份（`analysisJobIDContextKey` + `jobIDFromContext` 导出 helper），job fn 经 ctx 拿 job_id（StartSignal 签名不动） |
| `backend-go/internal/dataenrichment/handler/signal_research.go` | research job fn 改传 `signalResearchJobTimeout`；fn 内 `jobIDFromContext(ctx)` 取 job_id 传入 service；`SignalResearchRunner` 接口加 `jobID string` 参数 |
| `backend-go/internal/dataenrichment/handler/signal_discovery.go` | `listSignalCandidates` 每行增 `last_research_progress` 摘要（`ListLatestSignalResearchProgress` 批量一次 IN 查询）；新增 `getSignalResearchProgress` 端点 handler + `serializeSignalResearchProgressSummary` |
| `backend-go/internal/dataenrichment/handler/handler.go` | 注册路由 `GET /signals/:candidateId/research-progress` |
| `backend-go/internal/dataenrichment/handler/analysis_runner_test.go` | 新增 `TestSignalResearchJobTimeoutBudgetAlignment`（常量断言，任务 4.6 测试） |
| `backend-go/internal/dataenrichment/handler/signal_research_test.go` | stub 增 `lastJobID` 记录；新增 3 用例（job_id 透传/列表摘要/进展端点） |
| `backend-go/internal/dataenrichment/repository/signal_models.go` | 新表模型 `BoardSignalResearchProgress` + status 枚举常量（running/abandoned/superseded） |
| `backend-go/internal/dataenrichment/repository/models.go` | `RegisterModels` 注册新表（AutoMigrate 建表） |
| `backend-go/internal/dataenrichment/repository/signal_repository.go` | 4 个 Repository API（见 ②）；imports 增 `errors`/`gorm/clause` |
| `backend-go/internal/dataenrichment/repository/signal_research_progress_test.go` | **新文件**，隔离 PG 4 用例（见 ④） |
| `backend-go/internal/dataenrichment/service/signal_research.go` | `SignalProgressStore` 可选接口 + `SetSignalProgressStore`；`signalProgressRecorder`（saveRunning/abandon/supersede）；policy 增 `progressHook`（每轮非终态决策轮出口触发）；`ResearchCandidate` 加 `jobID string` 参数（defer 统一终态写入）；`progressLedgerJSON` 复用 `SignalReportAppendix` 序列化 |
| `backend-go/internal/dataenrichment/service/signal_research_test.go` | 既有 4 处调用点补 `""` jobID；新增 5 用例（见 ④） |
| `backend-go/internal/dataenrichment/wire.go` | `signalResearch.SetSignalProgressStore(repo)` 接线 |
| `backend-go/internal/platform/database/postgres_migrations.go` | 新增 `boardSignalResearchProgressMigration()`（`20260922_0002`），追加到迁移列表 |

**前端（Nuxt/Vue）**

| 文件 | 改动 |
| --- | --- |
| `front/app/api/boardSignals.ts` | 类型：`SignalResearchProgressStatus`/`SignalResearchProgressSummary`/`SignalResearchProgress`；`SignalCandidateRow.last_research_progress`；API `getSignalResearchProgress` |
| `front/app/features/tags/composables/useSignalWorkbench.ts` | `handleResearchTerminal` failed 分支：先置基础错误（旧时序），重拉列表后以候选行 `last_research_progress` 追加「（已保留 N 轮进展（X 次取数））」（`researchFailureMessage`）；不新增页面/弹窗 |
| `front/app/features/tags/composables/useSignalWorkbench.test.ts` | 新增 2 用例（失败文案携带摘要/无进展不附加）；`handleResearchTerminal` 签名修正后既有用例不变 |
| `front/app/features/tags/components/SignalCandidateList.test.ts` | `makeCandidate` 补必填新字段；新增 1 用例（错误块完整渲染带摘要文案，候选行渲染不受新字段影响） |

**制品与文档**

| 文件 | 改动 |
| --- | --- |
| `openspec/changes/board-signal-reports/tasks.md` | 4.6/4.7 勾选带证据；Scenario 落点表新增「超时后进展不白跑」「成功后进展归档」两行 + 「研究失败可重试」行补新增测试文件；6.T1/6.T2 命令未变（同包级命令已覆盖新测试），不改 |
| `docs/reference/api/board-signals.md` | 路由表增进展端点行；GET /signals 示例与字段表增 `last_research_progress`；research 端点补 150 分钟超时语义；新增 `GET /signals/:candidateId/research-progress` 节 |
| `docs/reference/database/tables/data-enrichment.md` | 新增 §10.13 `board_signal_research_progress` 全字段表 + 关联说明 |
| `docs/reference/flow/data-enrichment.md` | 「研究超时与进展持久化」小节新增；job 内存态段 30min 表述改为「发现 30min/研究 150min」；约束 #28 补超时独立化、#31 补每轮 upsert 挂点、#33 补进展表不回写语义；代码入口补 `signalProgressRecorder`/迁移 `20260922_0002` |

## ② 表结构与挂点说明

**表 `board_signal_research_progress`**：`id` PK；`job_id` VARCHAR(64) NOT NULL UNIQUE（同 job 一行滚动更新，值 = analysis runner 任务身份）；`semantic_board_id`/`candidate_id`/`granularity`/`period` NOT NULL（复合 FK `fk_board_signal_research_progress_candidate (candidate_id, semantic_board_id, granularity, period) → board_signal_candidate(id, …)` ON DELETE RESTRICT，比 brief 的简单 FK 更进一步：owner/周期与候选 DB 级钉死，沿用本 change #29 约束思路）；`rounds_done`/`source_calls`/`calculation_calls` INT NOT NULL DEFAULT 0；`ledger` JSONB NOT NULL DEFAULT '{}'（`calls`/`calculations`/`gaps` 三段，**服务层复用 `SignalReportAppendix` 结构体序列化，与报告 appendix 同源**）；`status` VARCHAR(16) CHECK 限 running|abandoned|superseded；`stop_reason` VARCHAR(32)；`error` TEXT；`created_at`/`updated_at`。索引：`(candidate_id, updated_at DESC)`、`job_id` 唯一。建表由 AutoMigrate，约束/索引由迁移 `20260922_0002`（幂等，DROP IF EXISTS + ADD）。

**每轮 upsert 的精确位置**：研究 policy（`signalResearchPolicy`）新增 `progressHook func(roundsDone int)` + `notifyProgress()`，触发点 = 每个非终态决策轮的收尾——
- `ObserveCall`（取数真实执行后、`annotateCall` 注记 question/观测索引完成后）——数据最重的路径；
- `RunLoopAction` 全部出口（calculate 记账后 1 处 + 5 个拦截 return 前各 1 处）；
- `CheckCall` 两个拦截 return 前（`tool_not_in_research_whitelist`/`missing_question`）。
- 已知留白：runToolLoop 循环体内的 guard（allowedSet）/dedup 拦截轮不经过 policy 出口，不触发该轮 hook——账本无变化，轮次计数由下一轮（或终态写入）追平；仅当 budget 耗尽前最后一轮恰为 dedup 拦截时 `rounds_done` 滞后 1，属可接受近似（报告里说明）。
- `finish` 是终态轮，不做轮内保存（由 defer 终态写入覆盖）。

**终态写法**（`ResearchCandidate` 内，recorder 非nil 时 defer 注册）：`err != nil` → `abandon(ctx, policy.lastStep, ledger, err)`；成功且 `resultID != 0` → `supersede(ctx)`。**关键正确性点**：`abandon`/`supersede` 均用 `context.WithoutCancel(runCtx)` + 独立 10s 超时——job 超时（ctx 已死）后终态仍落库；`stop_reason` 派生：`errors.Is(err, DeadlineExceeded)` → `timeout`，`SignalStageError.Stage` 已知 → stage 值（research/compose/save），否则 `failed`。进展写入全部尽力而为（失败只丢快照不掩盖主流程结果）。store 未接线或 jobID 为空 → recorder 为 nil，零行为变化（旧直调路径兼容）。

**Repository API**：`UpsertSignalResearchProgress`（`ON CONFLICT (job_id) DO UPDATE` 滚动覆盖计数/账本/状态/updated_at，GORM clause 方言中立，SQLite handler 测试同路径）、`GetLatestSignalResearchProgress`（`ORDER BY updated_at DESC, id DESC` 取一行；无进展返回 `nil,nil` 而非错误）、`ListLatestSignalResearchProgress`（IN 查询 + Go 侧按候选取首行，**候选列表摘要一次查询不加重既有 N+1**——本线程选择批量 IN 方案）、`MarkSignalResearchProgressSuperseded`（status 翻转行保留；未知 job_id 静默成功）。

## ③ API 变更

- `GET /signals` 每行新增可选 `last_research_progress`：`{rounds_done, source_calls, calculation_calls, status, stop_reason(null 可空), updated_at}`；从未研究 → `null`。
- 新端点 `GET /signals/:candidateId/research-progress`：候选最近进展全量（含 `ledger` jsonb 与 `job_id`/`error`/`created_at`）；候选不存在/跨板块 → 404（不暴露存在性，同款语义）；候选存在但从未研究 → 200 `data=null`。只读，不触发新研究。
- 前端错误态：研究失败文案追加「（已保留 N 轮进展（X 次取数））」（从重拉后候选行读）。

## ④ 测试结果（命令 + 退出码 + 用例数）

| 命令（cwd） | 退出码 | 结果 |
| --- | --- | --- |
| `bash scripts/harness/change-scope.sh` | 0 | 输出域映射含树上多 change 预存脏改；按 6.T1 先例只跑本任务辖区（dataenrichment + platform/database + 前端信号域） |
| `go test -short ./internal/dataenrichment/... -count=1` | 0 | 4 包 ok（dataenrichment/handler/repository/service） |
| `go test ./internal/platform/database/ -run "TestBoardSignal" -count=1` | 0 | 3 用例 ok（含迁移幂等重跑，覆盖 20260922_0001+0002） |
| `go test ./internal/dataenrichment/repository/ -run "TestUpsertSignalResearchProgress\|TestMarkSignalResearchProgress\|TestSignalResearchProgressDBConstraints\|TestListLatestSignalResearchProgress\|TestCreateSignalDiscoveryBatch\|TestListSignalCandidatesByPeriod\|TestSignalEvidenceRefHelpers" -count=1` | 0 | 隔离 PG：新增 4 用例（滚动更新/超立方归档保留+重启新实例直查/DB 级 CHECK+复合FK 拒 4 类非法行/批量 latest）+ 既有 6 用例全绿 |
| `go test ./internal/dataenrichment/service/ -short -run "TestSignalResearch_" -count=1` | 0 | 21 用例（既有 16 + 新增 5：每轮 upsert 递增+superseded 恰一次、**ctx 已死后终态 abandoned+timeout 且写入 ctx 存活（WithoutCancel 生效）**、拦截轮计轮不冒充取数、store 未接线/空 jobID 零调用、stop_reason 派生表） |
| `go test ./internal/dataenrichment/handler/ -short -run "TestSignalResearch\|TestSignalReports\|TestSignalDiscovery\|TestSignalList" -count=1` | 0 | 全绿（新增 3：202 信封 job_id 与 stub 收到的 jobID 一致（ctx 注入链路）、列表行摘要/无进展 null、进展端点 200 全量 ledger + 200 null + 404×2） |
| `golangci-lint run ./... && go vet ./... && go build ./...` | 0 | 全量三连通过（见 ⑤ 并发说明：中途一次失败系并发线程 margin_notes 在途半成品，稳定后复跑全绿） |
| `cd front && pnpm lint` | 0 | 0 errors（7 warnings 为既有风格警告，非本改动文件新增） |
| `cd front && pnpm exec nuxi typecheck` | — | 本改动文件 0 错误；剩余 5 个错误全部位于 margin notes 相关文件（并发线程在途工作，非本任务辖区） |
| `cd front && pnpm exec vitest run app/features/tags/components/SignalCandidateList.test.ts app/features/tags/composables/useSignalWorkbench.test.ts --maxWorkers=2` | 0 | 31/31（17+14） |
| `cd front && pnpm exec vitest run <信号域 4 文件> --maxWorkers=2`（含 SignalReportView/BoardEnrichmentPanel） | 0 | 77/77（回归无破坏） |
| `openspec validate board-signal-reports --strict` | 0 | valid |
| `bash scripts/harness/archive-readiness.sh board-signal-reports` | 0 | 四项全绿（scenario-trace 含两条新增 Scenario 映射） |

## ⑤ 未做项/残余风险

1. **树上并发线程干扰**：实现期间 margin notes change 线程在同一树上活跃（`topicgraph/service/margin_notes_qa.go`、`margin_notes_handler_test.go`、前端 marginNote* 文件），曾短暂导致全量 lint/vet/typecheck 红报；其稳定后 `go vet ./...`/`go build ./...`/辖区 lint 已复跑全绿，typecheck 剩余错误全在其文件。本线程未触碰其任何文件，也无遗留干扰。
2. **dedup/guard 拦截轮的 rounds_done 滞后近似**（见 ②）：仅当预算耗尽前最后一轮恰为循环内 dedup 拦截时滞后 1 轮；账本数据无损失。
3. **进展表写入为尽力而为**：upsert/终态写失败只丢快照（不记重试队列），不掩盖主流程结果；40 次写压力极低，风险可忽略。
4. **`useSignalWorkbench` 研究失败 404 分支（重启）未追加进展摘要**：spec「研究失败可重试」的摘要要求落在 outcome=failed 分支；重启 404 属「重启恢复」Scenario，其文案保持不变（进展行虽在 DB，但该分支只重拉列表不拼摘要，避免过度工程）。如需可后续增量。
5. **预存脏改零接触**：admin/topicgraph/dump-sanitizer 等树上其他 change 文件未动；`docs/reference/database/tables/data-enrichment.md` 头部「本域 5 张表」计数系他处遗留漂移，未顺手修。
6. **前端 SignalCandidateList.vue 组件本体零改动**：进展摘要完全经错误文案（composable 拼装）表达，满足 brief「错误信息携带进展摘要、不新增页面/弹窗」；如后续想要候选行内联「上次研究进展」常显标签，属新 UI 需求。

## ⑥ 裁决请求

1. **复合 FK vs 简单 FK**：brief 表结构注释写 `candidate_id BIGINT NOT NULL, -- FK → board_signal_candidate(id)`，实现取复合 FK `(candidate_id, semantic_board_id, granularity, period) → candidate 同列复合唯一`（owner/周期 DB 级钉死，与本 change #29「一致性由 DB 复合 FK 强制」的既有口径一致，直写幽灵归属被 PG 拒绝并有测试钉住）。请 controller 确认接受此加强；如需严格回落简单 FK，改一条迁移语句即可。
2. **stop_reason 枚举口径**：实现为 `timeout`（job 截止）/`research|compose|save`（error_stage 值）/`failed`（无 stage 兜底），与 brief「timeout|failed|error_stage 值」一致；`context.Canceled`（非截止类取消）落在 stage/failed 而非 timeout。如需把显式 cancel 单列，请裁决。
3. **成功归档行清空 stop_reason/error**：`MarkSignalResearchProgressSuperseded` 会清空这两列（成功行无失败语义）；若希望保留历史痕迹请裁决（当前行会先以 running 滚动、正常成功路径两列本就为空，实际无损）。
4. **dev 进程残留**：会话开始时 dev-process-guard 报告 6 个窗口外泄漏进程（agent-browser/chromium，疑似此前 UI 验收残留），非本会话产生，未处置，请主线程按 guard 建议人工确认清理。
5. 其余均按 brief 执行完毕：4.6/4.7 已实现并测试，tasks.md 已勾选带证据，Scenario 落点表已补两行并扩「研究失败可重试」行，三处文档已同步。归档等用户指令。
