# Seam B5 交接 — 安全执行循环 / 心跳 / 重启恢复 / 明确重试（tasks 2.5）

> board-health-triage-advice seam B 第五个小批次。交付任务 2.5：**显式生命周期的执行器（扫描领取 + 租约 + 心跳 + 重启恢复）+ 用户的明确 retry + 第四个 HTTP 资源**。
> **不含**：生产接线（routes.go / wire.go / app 生命周期，属 2.6）、前端（seam C）。不宣称整条修复链或整个用户故事已交付。
> 模型 `minimax-cn/MiniMax-M3.1-Flash:max`（用户指定）；承接 seam-b4-matching-handoff.md（B4 已交付）、seam-b3-review-fixes.md（B3 四项审查修复已复验）。

## 1. 新增/改动文件（backend-go/internal/ 下）

| 文件 | 状态 | 内容 |
| --- | --- | --- |
| `tagmanagement/service/auxrepair/executor.go` | **新增** | 执行器：`ExecutorConfig`（租约/心跳/轮询/批次 + 关系校验）、`Start/Stop/RunOnce/Notify`、扫描-领取-执行一轮、心跳续租 |
| `tagmanagement/service/auxrepair/retry.go` | **新增** | service 层 `RetryJob`（同任务重新排入 / no-op / 409）+ worker 唤醒登记 |
| `tagmanagement/service/auxrepair/executor_pg_test.go` | **新增** | 临时 PG 回归（12 例）：并发领取唯一赢家、心跳跨初始 TTL、失权不复活租约、已提交任务被新执行器恢复（不重嵌）、未提交中断恢复、失败不自动重跑 + 显式 retry 只跑一次、retry 语义（no-op/409/404）、Start/Stop 幂等、请求 ctx 取消不影响 app 任务、只写 job/items、配置校验、通知不启动 worker |
| `tagmanagement/repository/auxiliary_description_repair_executor.go` | **新增** | 执行器 SQL：扫描领取（`FOR UPDATE SKIP LOCKED`）、租约续期、retry 重新排入（同一短事务 + 行锁） |
| `tagmanagement/handler/auxiliary_description_repair_handler.go` | 改动 | **新增第四个资源** `POST /:id/description-repairs/:job_id/retry`（沿用 signed-bigint 参数与安全信封；body 只允许空/`{}`） |
| `tagmanagement/handler/auxiliary_description_repair_handler_test.go` | 改动 | 新增 `TestAuxiliaryDescriptionRepairRetryEndpoint`（HTTP 契约） |
| `tagmanagement/service/auxrepair/auxrepair.go` | 改动 | `Service` 增 worker 唤醒登记；`CreateJob` 成功后唤醒（只唤醒已有 worker） |
| `tagmanagement/service/service.go` | 改动（**仅追加** facade 段） | `AuxRepairExecutorConfig`、`DefaultAuxRepairExecutorConfig`、`NewAuxRepairExecutor`、`AuxRepairRetryNotAllowed` |

**未改动**：board 匹配算法与阈值、`board` 包本身、RootDB / 全局 DSN、旧迁移、生产 routes/wire/app 生命周期、前端、2.2–2.4 的既有语义与 DTO 字段。

## 2. 导出符号与默认值

```go
// service/auxrepair
type ExecutorConfig struct {
    PollInterval      time.Duration // 空闲扫描间隔（默认 5s）
    LeaseDuration     time.Duration // 一次领取的所有权时长（默认 60s）
    HeartbeatInterval time.Duration // 心跳续租间隔（默认 20s，必须 < 租约）
    HeartbeatTimeout  time.Duration // 单次心跳的 SQL 超时（默认 10s）
    MaxBatch          int           // 一轮最多执行几个任务（默认 1 = 单执行槽）
}
func DefaultExecutorConfig() ExecutorConfig
func (c ExecutorConfig) Validate() error   // 全为正 + 心跳 < 租约 + 批次 ≥ 1

func NewExecutor(svc *Service, cfg ExecutorConfig) (*Executor, error) // 不启动任何东西
func (e *Executor) Start(ctx context.Context) error                   // ctx = 应用生命周期
func (e *Executor) Stop()                                             // 取消 + 收束 + 幂等
func (e *Executor) RunOnce(ctx context.Context) (int, error)          // 一轮扫描-执行（可控）
func (e *Executor) Notify()                                           // 只唤醒已有 worker
func (e *Executor) Running() bool

// service/auxrepair
func (s *Service) RetryJob(ctx context.Context, auxiliaryLabelID, jobID uint) (*JobView, error)
var ErrStageRetryNotAllowed = errors.New("repair task cannot be retried in its current state")

// service 门面
type AuxRepairExecutorConfig = auxrepair.ExecutorConfig
var DefaultAuxRepairExecutorConfig = auxrepair.DefaultExecutorConfig
var NewAuxRepairExecutor = auxrepair.NewExecutor
var AuxRepairRetryNotAllowed = auxrepair.ErrStageRetryNotAllowed
```

**生命周期约束**：
- `NewService` **不自启**任何东西；构造不跑 matcher、不发 AI、不匹配（与 B3/B4 一致）。
- `Start(appCtx)` **幂等**：重复调用不会起第二个 worker；`ctx` 是**应用生命周期**——提交任务的 HTTP ctx 与任务执行**无关**，关页面不取消已确认任务。
- `Stop()` 取消 loop 与在途 attempt 的心跳、join 后返回，**幂等**。
- 通知只**唤醒已有 worker**（非阻塞 channel send，无 worker 时丢弃）：**不因每次 POST 起 goroutine**。
- 默认单执行槽 + 有界轮询：空队列或 DB 报错 → **等待/退避**，绝不忙循环。

## 3. 扫描与领取语义

- **队列就是数据库**：没有内存 map、没有进程内队列表；任务本身即队列。
- **可领取集合**（`claimableRepairJobsPredicate`）：
  - `status = 'pending'` 且未被活租约持有 → 新任务；
  - `status IN ('embedding','saving','matching')` 且**租约已过期**（`clock_timestamp()` 判定）→ 上一次 attempt 死掉的中断任务。
- **不自动领取**：`embedding_failed` / `save_failed` / `matching_failed` —— 明确失败**只等用户 retry**，不自动无限重试烧 AI。
- **不扫描**：`completed` / `conflict` / `unavailable`。
- **领取是原子 CAS**：一个短事务内 `SELECT ... FOR UPDATE SKIP LOCKED` + 写 claim；多实例只一方拿到该任务。
- **每次执行新唯一 owner token**：每次执行现场生成 `auxrepair-<instance>-<random>`（不是固定进程名）；领取时同时**作废旧的 phase token**，旧在途 attempt 因此无法提交。
- **按已提交状态分流**：`description_committed = true` → **只续匹配**（绝不重嵌/重存）；未提交 → 走 embedding 阶段。

## 4. 心跳（续租）语义

- 续租条件：`id = ? AND claim_owner = ? AND lease_expires_at IS NOT NULL AND lease_expires_at > clock_timestamp()`——**只有当前唯一 owner 且租约尚未过期**才能延长。
- **不复活过期租约**、**不重置 phase_token**、**不改 status/计数**、**不动别人的 owner**。
- 失权（或不确定的 DB 故障）→ **取消本次 phase ctx**：老 actor 既不能提交、也不能标进度（其后续写全部被 owner+token+实时租约围栏拒绝）。
- 心跳的 SQL 调用带**有限 context**（`HeartbeatTimeout`）；收尾停 ticker 与 goroutine；正常失败与 Stop 只做 owner 围栏清理，**不把失权写成别人的新失败**。
- 所有权 + 阶段 token + 最终实时租约 + RowsAffected 的防线**保持不变**，没有退回 `now()` 固定事务时间。

## 5. Retry 语义（第四个 HTTP 资源）

`POST /api/auxiliary-labels/:id/description-repairs/:job_id/retry`

| 任务状态 | 结果 |
| --- | --- |
| 不存在 / 属于别的辅助标签 | **404** |
| 运行中（活租约） | **202 + 同一任务**，**完全 no-op**：不重排、不起 worker、**连 UpdatedAt/计数/范围都不变** |
| completed | **202 + 同一任务**，完全 no-op |
| conflict / unavailable | **409**（要求重新核对最新版本；retry 不得重置来源版本） |
| 明确失败（embedding_failed / save_failed / matching_failed） | **202 + 同一任务**重新排入：清旧 error、作废旧执行权（phase token + claim），**不改** `expected_version` / `new_description` / 冻结 scope，**不新建 job** |
| 中断（未结束阶段但租约已过期） | **202 + 同一任务**重新排入（同上） |

- 已 `description_committed = true` 的任务重新排入时**只续匹配**。
- **不提前查活体标签**：历史任务即使标签后来变了也仍可 retry；来源再变化由现存阶段的版本校验拒写，retry 不是绕过它的后门。
- 并发 retry 与扫描器由**数据库仲裁**（行锁 + CAS），同一任务不会两跑。
- **body 只允许空或 `{}`**：非法 JSON / 第二对象 / 尾随垃圾 / **任何参数对象** → **400**（不能让用户以为改了请求）。
- handler 只绑参数/调用薄 service：**不同步跑阶段、不 spawn worker**；返回 **202 + 现有 JobView**（不含 owner/token）。

## 5b. 2026-10-29 复审修订（两项行为变化）

1. **一次 attempt 连续跑完两个阶段**：embedding 阶段提交成功后，执行器**在同一次 attempt 内**继续跑匹配阶段（同租约、同心跳）——否则描述已提交的任务要等租约到期（生产最长 60s）才进入匹配。语义不变：已 committed 的任务**绝不**再 embed/保存。
2. **租约期限由数据库算**：领取与心跳续租写 `clock_timestamp() + make_interval(secs => ?)`（参数化），不再用应用时钟 `now()+lease` 写、DB 时钟判；应用时钟落后不会再造成「新租约一出生即过期」的热循环。`ClaimJobForExecution(ctx, id, owner, lease time.Duration)` 的外部签名不变。
3. **panic 围栏**（阶段级 / 执行器级 / 心跳 goroutine）与**阶段 CAS 的 RowsAffected 检查**、**retry 拒绝 JSON null**：见 `seam-b5-review-fixes.md`。

## 6. 故障窗（PG 已验证）

| 故障窗 | 行为 |
| --- | --- |
| 多执行器并发扫描同一任务 | 只有一方领取（SKIP LOCKED + CAS），embedding 只发生一次 |
| embedding 阻塞超过**初始**租约 | 心跳续租 → 保存成功（租约被心跳保持） |
| 执行中被接管（owner 换人、租约已过期） | 心跳**不复活**租约；老 actor 的提交被拒（描述/向量未变、committed=false）、新 owner 不被踩 |
| 保存后中断（描述已提交、匹配未完） | 新执行器扫描恢复：**embedding 次数不增**、`committed_version`/`committed_at` 不变，只续未完成/失败项 |
| 未保存中断 | 安全恢复：重新走 embedding 并成功保存（旧值在此之前一直有效） |
| 明确失败 | 扫描器**不再自动跑**（不烧 AI）；用户 retry 后**只跑一次**新尝试 |
| HTTP 请求 ctx 被取消 | 不影响 app 任务（worker 跑在应用 ctx 上） |
| Start/Stop 重复调用 | 幂等、无双 worker；Stop 后 worker 真正停止（不再领取任务） |

## 7. 本批实跑验证（backend-go，串行，GOMAXPROCS=2 / -p 1）

| # | 命令 | 结果 |
| --- | --- | --- |
| 1 | `go test -count=1 -p 1 -run 'TestAuxiliaryDescriptionRepairExecutor\|TestAuxiliaryDescriptionRepairRetry' -v ./internal/tagmanagement/service/auxrepair` | **ok**，12/12 PASS（B5 新增用例） |
| 2 | `go test -count=1 -p 1 -v ./internal/tagmanagement/service/auxrepair` | **ok 27.1s**，**34/34 PASS**（B3 12 + B4 10 + B5 12） |
| 3 | `go test -count=1 -p 1 -run 'TestAuxiliaryDescriptionRepair' -v ./internal/tagmanagement/handler` | **ok 6.3s**，9/9 PASS（含新增 `TestAuxiliaryDescriptionRepairRetryEndpoint`） |
| 4 | `golangci-lint run ./internal/tagmanagement/...` | **0 issues** |
| 5 | `go vet ./internal/tagmanagement/... && go build ./...` | ok |

**用例计数更正（记账）**：上一份 B4 交接写的「21/21（B3 13 + B4 8）」与源码不符——按现源码核对为 **B3 = 12 例、B4 = 10 例、合计 22 例**（B4 交接与 B3 review 修复记录里的该数字已更正；未为数字重跑任何批次）。加入 B5 的 12 例后，本包共 **34 例**。

**未跑**（按纪律刻意不跑）：全量 `go test ./...`；B1/B2/B3/B4 原批整套复跑；外部已绿门禁；真库/环境 DSN/psql；dev 起停/部署；未派生任何子线程。

## 8. 指纹（backend-go/internal/ 下，成稿时磁盘状态）

| 文件 | SHA256 |
| --- | --- |
| tagmanagement/service/auxrepair/executor.go | 66e10895bb355bf1e6e654bf2a42c419ca5e11f11a32a6c78a8c69d78f56f4ba |
| tagmanagement/service/auxrepair/retry.go | dcf55479ee2ff86877e6021c03064778afac5bbc096101f885d43efa42b4d0c6 |
| tagmanagement/service/auxrepair/executor_pg_test.go | 0d64cad871dc7844f990dd8f2b272197677375387856e0fe0ddd23e9c831c136 |
| tagmanagement/service/auxrepair/auxrepair.go | 07e9d71bd37b5a3da7a4f3ee03bba5b942957b418cb1b916ff866c504c41cc3e |
| tagmanagement/repository/auxiliary_description_repair_executor.go | 3394d588d8d23f30d02a46da9077bac91c59616fef7563a074eb89ed35491dd3 |
| tagmanagement/handler/auxiliary_description_repair_handler.go | 86868106241d5c9f5f71f37b500d38dbfefa156feca93e30ed43cef1dd5f8a14 |
| tagmanagement/handler/auxiliary_description_repair_handler_test.go | af54ef78812413767a01983395da199916a53d726ffc50bf7900722d12d4d5f3 |
| tagmanagement/service/service.go（含外部既有脏改） | d330ae0f7be1c51085845fb6fcd0a1aaa2d12d8daa1776501eb660bde4d6f9b0 |

> 续做时以 `sha256sum` 复核为准。

## 9. 2.6 生产接线要求（本批**未做**）

1. 在应用启动路径创建执行器并用**应用生命周期 ctx** 启动：`svc, _ := auxrepair.NewService(db)` → `exec, _ := auxrepair.NewExecutor(svc, cfg)` → `exec.Start(appCtx)`；应用退出时 `exec.Stop()`。
2. 把 `RegisterAuxiliaryDescriptionRepairRoutes(rg, svc)` 接进 `tagmanagement/routes.go`（本批只加路由函数，**未接生产**）。
3. 配置项（租约/心跳/轮询）从配置读取并校验关系（心跳 < 租约）；默认值见 §2。
4. 前后端契约：新增的 retry 资源与既有三资源共用信封与数字 ID wire；前端 normalizer 负责转字符串视图。

## 10. 诚实的边界与后续

- **2.5 之外**：本批实现的是**执行器 + 心跳 + 重启恢复 + 显式 retry**；**没有**跨进程的任务调度器（同一台机器上的多个实例靠 DB CAS 仲裁）、**没有**失败任务的自动重试策略（按合同：等用户 retry）、**没有**运维指标/告警。
- **匹配至少一次**：引擎调用在途不可中断——进程崩溃后该项保持 pending，重启后**至少一次**重放（引擎是替换式写入，幂等）；**不宣称 exactly-once**。
- **数据隔离红线**：该链会重算**所在库全部 active tags** 的匹配；真实交互故事必须落一次性 PG + 对应测试 API，不得在真库自动修复真实标签。
- **文档欠账**：`docs/reference/api/semantic-boards.md`（第四个端点与错误码）、`docs/reference/flow/semantic-board.md`（执行/恢复/重试口径）、`docs/reference/database/tables/semantic-labels.md`（执行元数据列）属 tasks 6.x，本批未动；`ai-logging` 清单仍未登记本链的 embedding operation（复用既有 operation）。
- **未做前端**：retry 入口、恢复动作与进度文案属 seam C。
