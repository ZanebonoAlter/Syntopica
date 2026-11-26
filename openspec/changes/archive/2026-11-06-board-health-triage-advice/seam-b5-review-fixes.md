# B5 独立复审发现修复记录（tasks 2.5 复验）

> 2026-10-29。B5 独立复审（只读审固定快照 `/tmp/board-health-b5-review-orPVR6ud`，MiniMax 通道，run `db933a23`）结论 **OK with notes**：B3 旧四项 R **全部真实关闭**、B4 正确、B5 正确（含「NULL 租约明确已支持」，**不需要为该猜测改代码**）；新增 **1 Medium + 4 Low**。父线程接受其中 4 项修复、明确驳回 1 项性能建议。
> 修复者：单写者（`minimax-cn/MiniMax-M3.1-Flash:max`）。写入范围：`service/auxrepair/{executor.go,embedding.go,matching.go}`、`repository/auxiliary_description_repair_{stage,executor}.go`、retry handler、精准新增/变更测试、本文件 + `review.md` + `tasks 2.5`。
> 另一路复审（Codex run `43672238`）**因 usage limit 失败，无结论**——不记作「审毕无发现」。

## 0. 复审确认关闭的旧项（不再改动）

- **B3 旧四 R 全部真实关闭**：实时租约（`clock_timestamp()`）、取得双锁后复核、提交标记带 owner+token+实时租约且查 RowsAffected；阶段 CAS 原子领 token、失败方 busy 且不写失败状态、接管废旧 token；领取与提交均清 error；`CommittedVersion` 由回读 PG 行算出。
- **B4 正确**：冻结谓词同一份、NA 与成功分列、total 不缩、items+counters 同事务单源、owner+token+实时租约三重围栏、每次运行前失效 cache、部分失败→matching_failed、真实 matcher 显式绑 DB。
- **B5 正确**：`claimableRepairJobsPredicate` 第二支显式含 `lease_expires_at IS NULL` → **「status=matching 且 owner/lease 为空」可被领取**（保存后恢复、matching_failed→retry→队列→自动执行都走得通）。**未因此猜测改任何代码**；本批另加一条真实 RunOnce 回归把该行为钉死（见 §1.6）。

## 1. 新发现 → 处置

| 编号 | 级别 | 发现 | 处置 | 复验 |
| --- | --- | --- | --- | --- |
| R5-1 | Medium | worker 无 panic 兜底：matcher/embedder panic 会从 goroutine 外溢**终止整个后端** | ① 阶段级围栏：`ExecuteEmbeddingPhase` / `ExecuteMatchingPhase` 在拿到 phase token 后注册 `defer recover()`，日志含 job/阶段/stack，任务状态按**持久化真相**落失败态，公开错误只有通用文案（`ErrStageAttemptPanicked`，不含 panic 细节）。② 执行器级围栏：`runJob` 顶层 recover（生命周期 defer 仍收束 HB 与槽位）、`runRound` 每轮 recover（worker 不死）、心跳 goroutine 自身 recover（并 cancel phase ctx）。③ **阶段失败按已提交状态分流**（**纠正复审报告的错误断言**：匹配阶段描述**已提交**，panic 后必须保留 `description_committed=true` 与新描述/向量，任务落 `matching_failed`；embedding/保存前 panic 才保持未提交，事务 panic 仍回滚）。④ 失败落库仍走 owner+token+实时租约守卫：失权或 DB 不可写时不踩新 owner、不假报已记录（留待失租恢复） | `TestAuxiliaryDescriptionRepairPanicInEmbeddingKeepsTaskUncommitted`、`...PanicInMatchingKeepsCommittedData`、`...ExecutorKeepsWorkingAfterPanic` → PASS（进程/worker 存活、HB 收束、阶段失败正确、已存数据不回退、其他任务继续处理、失败不被自动再跑） |
| R5-2 | Low | 领取/续租写 `应用 now()+lease`、判定用 DB 时钟：应用时钟落后会让新租约一出生即过期 → worker 热循环 | 租约**期限改由数据库算**：`clock_timestamp() + make_interval(secs => ?)`（参数化，不拼字符串），领取与心跳续租统一走该表达式；应用时钟不再决定 DB 租约的最终权威。`ClaimJobForExecution(ctx, id, owner, lease time.Duration)` 的**外部签名不变**（只把 duration 交给仓储层） | `TestAuxiliaryDescriptionRepairExecutorLeaseUsesDatabaseClock` → PASS（应用时钟落后 1 小时时新租约仍在未来、任务一次完成、embed 调用数仍为 1 → 无热扫；另用探针任务断言 DB 写出的期限在未来） |
| R5-3 | Low | 阶段 CAS 的 `UPDATE(phase_token IS NULL)` 未查 RowsAffected → 可能「生成了 token 却按成功返回」 | CAS 增查 RowsAffected：0 → `ErrRepairJobBusy`（不写任何状态、不调用 AI）；另加防御回归（`phase_token` 为非 NULL 空串） | `TestAuxiliaryDescriptionRepairPhaseAcquireRejectsForeignToken` → PASS（busy、零 AI 调用、任务状态与原 token 不变） |
| R5-4 | Low | retry handler 把 JSON `null` 当空体放行（契约只允许空 body 或 `{}`） | `validateEmptyRetryBody` 区分**真空体（EOF）**与 **JSON null（nil map → 400）**；`{}` 与真空体仍合法；非法/额外 body 仍 400 | `TestAuxiliaryDescriptionRepairRetryEndpoint`（新增 `null` 负向）→ PASS |
| R5-5 | Low（**驳回**） | 逐项计数聚合 O(N²)（每项一事务 + 全量聚合 items） | **不采纳**：逐项结果与计数**同事务**是本 change 的明示合同（implementation-contract §1 + data-map「计数字段来源」），「每 K 项再刷新」会静默弱化它；计数单源与精确即时计数保留。当前**无量化瓶颈证据**（未测出真实耗时），后续有实测再单独立项优化——本轮不新增实现、不另起 change | 见 review.md 的驳回理由 |

## 2. 顺带修正的行为（非缺陷，属接线补全）

- **一次 attempt 连续跑完两个阶段**：embedding 阶段提交成功后，执行器**在同一次 attempt 内**继续跑匹配阶段（同租约、同心跳）——否则描述已提交的任务要等租约到期（生产最长 60s）才进入匹配。B6 的「app 执行器自动 embedding/save/matching」要求的是一条自动流，故一并补上。相应地，`TestAuxiliaryDescriptionRepairExecutorDoesNotAutoRetryFailedTask` 的终态断言由 `matching` 改为 `completed`（该批用例的其余断言不变）。

## 3. 精确验证命令与结果（串行，GOMAXPROCS=2 / -p 1）

| # | 命令 | 结果 |
| --- | --- | --- |
| 1 | `cd backend-go && GOMAXPROCS=2 go test -count=1 -p 1 -run 'TestAuxiliaryDescriptionRepairPanic\|TestAuxiliaryDescriptionRepairExecutorKeepsWorkingAfterPanic\|TestAuxiliaryDescriptionRepairPhaseAcquireRejectsForeignToken' -v ./internal/tagmanagement/service/auxrepair` | **ok**，3/3 PASS（R5-1 / R5-3） |
| 2 | `cd backend-go && GOMAXPROCS=2 go test -count=1 -p 1 -run 'TestAuxiliaryDescriptionRepairExecutorLeaseUsesDatabaseClock' -v ./internal/tagmanagement/service/auxrepair` | **ok**，PASS（R5-2） |
| 3 | `cd backend-go && GOMAXPROCS=2 go test -count=1 -p 1 -run 'TestAuxiliaryDescriptionRepairRetryEndpoint' -v ./internal/tagmanagement/handler` | **ok**，PASS（R5-4） |
| 4 | `cd backend-go && GOMAXPROCS=2 go test -count=1 -p 1 ./internal/tagmanagement/service/auxrepair` | **ok 15.8s** —— 回归检查（本次改动触及共享的租约/守卫路径，故对受影响包整体复跑一次；**新增/变更用例的证据是上面 1-3**） |
| 5 | `cd backend-go && GOMAXPROCS=2 go test -count=1 -p 1 -run 'TestAuxiliaryDescriptionRepair' ./internal/tagmanagement/handler` | ok 24.4s（受影响包的精准回归） |
| 6 | `cd backend-go && GOMAXPROCS=2 golangci-lint run ./internal/tagmanagement/... ./internal/app/... ./cmd/...` | **0 issues** |
| 7 | `cd backend-go && GOMAXPROCS=2 go vet ./internal/tagmanagement/... ./internal/app/... ./cmd/... && go build ./...` | ok |

**未跑**（按纪律刻意不跑）：全量 `go test ./...`；B1–B4 原批整套；外部已绿门禁；真库/环境 DSN/psql；dev 起停/部署；未派生任何子线程。

## 4. 状态与边界

- **tasks 2.5 的完成勾在修复期间暂撤，专项复验全绿后已恢复**（见 tasks.md 留痕）。
- **诚实的边界**：panic 围栏保证「进程存活 + 任务状态正确 + 不假报」，但**引擎调用的外部副作用**（匹配表的替换写）在 panic 时可能已部分发生——引擎的写入是替换式的（幂等），重放安全；仍**不宣称 exactly-once**。失败任务只等用户 retry，不自动无限重跑。
- 本记录只解除 B5 阶段的审查意见，**不替整个 change 最终放行**（2.6 接线的独立复审、前端、文档对账仍未完成）。
