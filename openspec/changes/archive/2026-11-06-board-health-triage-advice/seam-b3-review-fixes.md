# B3 独立 review 发现修复记录（tasks 2.3 复验）

> 2026-10-29。B3 独立 review（只读审 `/tmp/board-health-b3-review-NfMkzs2r` 的 B2-fixed/B3 快照，未改任何文件、未跑测试）判定 **BLOCK**：2 项 Medium + 2 项 Low。全部接受并已修复；按父线程指示：先暂撤 tasks 2.3 的完成勾并在 review.md 记录阻断，修复并专项复验后再恢复。
> 修复者：单写者（`minimax-cn/MiniMax-M3.1-Flash:max`）。写入范围：`repository/auxiliary_description_repair_stage.go`（守卫/helper）、`service/auxrepair/embedding.go` + `embedding_pg_test.go`、B3 最小恢复底座、`review.md` / `tasks 2.3` / 本文件。**未改生产路由、未弱化 contract、未新增并发承诺以外的行为。**

## 1. 发现 → 处置对照

| 编号 | 级别 | review 发现 | 处置 | 复验 |
| --- | --- | --- | --- | --- |
| R1 | Medium | 租约用事务开始固定的 `now()`，且在等 aux 行锁**之前**检查 → 跨过期仍可保存 | ① 所有租约判定改用数据库**实时**时钟 `clock_timestamp()`（`now()`/`statement_timestamp()` 固定在事务/语句开始，均不可用）；② `CommitDescription` 在**取得 job + aux 全部锁之后**再复核一次实时租约；③ 最终 job 提交标记的 UPDATE 带 `claim_owner + phase_token + lease_expires_at > clock_timestamp()` 条件并**检查 RowsAffected**，失权/过期 → 整体回滚（含已写的 description/vector） | `TestAuxiliaryDescriptionRepairExecuteRejectsLeaseExpiredWhileWaiting` → PASS（另一事务**真实持有 aux 行锁**，保存进入等待并跨过租约，释放后必须拒绝：旧 desc/vector 不变、committed=false）；`TestAuxiliaryDescriptionRepairExecuteRollsBackWhenCommitMarkerFails` → PASS（在 label 写处注入 2.5s 延时，租约在写后到期 → 提交标记条件失败 → **连已写 label/vector 一起回滚**） |
| R2 | Medium | 同有效 owner 并发两个 `ExecuteEmbeddingPhase` 时只靠 owner/lease/未committed 条件的阶段标记挡不住，**两者都会 embed** | 新增**原子 CAS 领取阶段执行权** `AcquireRepairPhase`：要求 (a) 描述未提交（保存阶段）或已提交（匹配阶段）、(b) 调用方持 claim 且**实时**租约有效、(c) **无人持有 phase token**、(d) 状态在允许来源集合内；领取时写入新 token + 目标 stage/status 并**清空旧 error**。失败领取方返回 **busy**，**绝不**顺手写失败状态破坏真实运行者。恢复语义：claim 接管（换 owner 或租约已过期）会**作废**旧 phase token，旧在途调用凭 token 失配无法提交；新尝试用**新 token**（同 owner 重复调用也拿不到执行权） | `TestAuxiliaryDescriptionRepairExecuteRefusesConcurrentSameOwner` → PASS（第一次调用在桩 embedder 内阻塞，第二次同 owner 调用被 busy 拒绝、**embed 调用数仍为 1**、第一次的任务状态未被改动；释放后**仅一次**原子保存）；`TestAuxiliaryDescriptionRepairExecuteAuthorizedRecoveryAfterLeaseExpiry` → PASS（租约过期 → 新 owner 接管作废旧 token → 旧在途调用不能提交 → 新 owner 接管完成）；`TestAuxiliaryDescriptionRepairExecuteRetryAfterFailure` → PASS（失败 → 明确重试仍可执行，不是靠永久拒绝 embedding/saving 状态假修） |
| R3 | Low | 开始新尝试与成功提交未清空旧 error | 领取阶段执行权时清空 `error`；成功提交（最终标记）也写 `error=''` | `TestAuxiliaryDescriptionRepairExecuteRetryAfterFailure` → PASS（`embedding_failed` 带错误原因 → 明确重试成功 → **持久 Error 为空**、旧失败不再透出、view.Error 为空） |
| R4 | Low | 旧 storage 与桩新 storage 同向量 → 漏测「根本没更新」 | 桩改用**不同但合法同维**向量（`auxRepairTestVectorB`）；断言 `旧 != 新`、`保存后 == 新`；并断言 `CommittedVersion ==` 回读 PG 标签算出的 `AuxiliaryVersionToken`（不只是「不等于旧的」） | `TestAuxiliaryDescriptionRepairExecuteCommitsAtomically` → PASS（数值比较：旧向量与新向量不同、保存后等于新向量；`CommittedVersion` 与回读标签的令牌相等） |

## 2. 精确验证命令与结果（串行，GOMAXPROCS=2 / -p 1）

| # | 命令 | 结果 |
| --- | --- | --- |
| 1 | `cd backend-go && GOMAXPROCS=2 go test -count=1 -p 1 -run 'TestAuxiliaryDescriptionRepairExecuteRefusesConcurrentSameOwner\|TestAuxiliaryDescriptionRepairExecuteRetryAfterFailure\|TestAuxiliaryDescriptionRepairExecuteAuthorizedRecoveryAfterLeaseExpiry' -v ./internal/tagmanagement/service/auxrepair` | **ok 6.8s**，3/3 PASS（R2 + R3） |
| 2 | `cd backend-go && GOMAXPROCS=2 go test -count=1 -p 1 -run 'TestAuxiliaryDescriptionRepairExecuteRefusesConcurrentSameOwner\|TestAuxiliaryDescriptionRepairExecuteRejectsLeaseExpiredWhileWaiting\|TestAuxiliaryDescriptionRepairExecuteRollsBackWhenCommitMarkerFails' -v ./internal/tagmanagement/service/auxrepair` | **ok 19.0s**，3/3 PASS（R1 + R2） |
| 3 | `cd backend-go && GOMAXPROCS=2 go test -count=1 -p 1 -v ./internal/tagmanagement/service/auxrepair` | **ok 15.0s**，**22/22 PASS**（B3 保存阶段 12 例 + B4 匹配阶段 10 例，含 4 个子例组；**计数更正 2026-10-29**：原文「21/21 = 13 + 8」与现源码不符，未为数字重跑） |
| 4 | `cd backend-go && GOMAXPROCS=2 go test -count=1 -p 1 -run 'TestAuxiliaryDescriptionRepair' ./internal/tagmanagement/handler` | ok 20.0s（B2 的 DTO 用例仍绿；`not_applicable`/`remaining` 为新增字段，不破坏既有断言） |
| 5 | `cd backend-go && GOMAXPROCS=2 golangci-lint run ./internal/tagmanagement/...` | **0 issues** |
| 6 | `cd backend-go && GOMAXPROCS=2 go vet ./internal/tagmanagement/... && go build ./...` | ok |

**未跑**（按指示刻意不跑）：B1/B2/B3 原批整套复跑、外部已绿门禁、全量 `go test ./...`、真库/环境 DSN/psql、dev 起停与部署、派生写者。

## 3. 改动文件与指纹（backend-go/internal/ 下）

| 文件 | 改动 | SHA256 |
| --- | --- | --- |
| `tagmanagement/repository/auxiliary_description_repair_stage.go` | 实时租约（`clock_timestamp()`）、阶段执行权原子 CAS（`AcquireRepairPhase`）、owner+token 守卫、提交标记 RowsAffected 检查、claim 接管作废旧 token | 7e6df3a3f2edc804433e3daa51af5677de78c674d6f33719ef60202a94ac1faf |
| `tagmanagement/service/auxrepair/embedding.go` | 阶段执行权领取、busy 语义、token 传递、错误清理 | 5a119ab0dc2d4ecb47066287dbb4bb66d1f397106f69b0a4d16379856640cf59 |
| `tagmanagement/service/auxrepair/embedding_pg_test.go` | R1/R2/R3/R4 回归 + 桩 embedder 支持按调用序号失败 | 4051480bb42b1d543d248ccb4fd5638a198a7a749d91108ec1c0155159c312b0 |
| `tagmanagement/models/auxiliary_description_repair.go` | 新增内部列 `phase_token`（nullable、`json:"-"`、不进 DTO） | 5b0686896b675955c1e3147723d6f580a617a2d26e6248c82294d47b8e016b20 |

## 4. 状态与边界

- **tasks 2.3 的完成勾已恢复**（修复 + 专项复验全绿，见 tasks.md）。
- **诚实的边界**：本批仍**不是**完整的 2.5——没有 worker、没有 heartbeat、没有 lease 续期、没有重启恢复调度器；「阶段执行权」只是底层 CAS 原语（领取/作废/校验），完整的 claim/heartbeat/restart 流程属 2.5。
- **诚实的边界**：引擎调用在途**不可中断**——若进程在引擎调用中崩溃，该项保持 pending，重启后会被**至少一次**重放（引擎本身是幂等的：它替换而非追加挂载）。本批**不宣称 exactly-once**；围栏化靠 token + 实时租约 + RowsAffected（失权者一律无法提交）。
- 本记录只解除 B3 阶段的 BLOCK，**不替整个 change 最终放行**（review.md「最终审查待办」不变）。
