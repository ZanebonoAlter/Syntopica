# Seam B6 交接 — 生产源代码接线（tasks 2.6）

> board-health-triage-advice seam B 第六个批次。交付任务 2.6 的**源代码**部分：四个 HTTP 资源接入生产路由、域根持有修复链生命周期、app 启动/关停接线、以及一条完整的生产后端故事（context → 创建 → 后台执行 → 任务查询 → 明确重试 → 队列续跑）。
> **本批只做源代码接线**：未启动任何 dev/app 服务、未运行部署或重启、未写静态产物；真实日常环境**没有**被测试写链触碰（全部写链只发生在一次性 testcontainer PG）。
> 模型 `minimax-cn/MiniMax-M3.1-Flash:max`（用户指定）；承接 seam-b5-executor-handoff.md、seam-b5-review-fixes.md。

## 1. 改动文件

| 文件 | 改动 |
| --- | --- |
| `backend-go/internal/tagmanagement/wire.go` | 域根持有修复链生命周期：`InitDescriptionRepairChain(db) error`（显式、失败即返回，不缓存 nil）、`InstallDescriptionRepairChain(svc, exec)`（依赖注入缝：生产用生产协作者，测试用桩，**同一条接线路径**）、`DescriptionRepairService()`、`StartDescriptionRepairExecutor(ctx)`、`StopDescriptionRepairExecutor()`；并 re-export `handler.RegisterAuxiliaryDescriptionRepairRoutes` |
| `backend-go/internal/tagmanagement/routes.go` | 在既有路由集合上**追加**四个修复资源（用**同一个** service 实例）；未删除/改动任何旧路由 |
| `backend-go/internal/app/runtime.go` | `Runtime` 持有**应用生命周期 ctx**（`appCtx`/`appCancel`）；`StartRuntime` 用它启动修复 worker；关停序列新增 `repair` 步：**先 cancel 应用 ctx、再 join worker**，早于 registry/workers 步（DB 释放前收束） |
| `backend-go/cmd/server/main.go` | 在 `database.InitDB` 之后、`SetupRoutes` **之前**显式初始化修复链；失败 → `logging.Fatalf`（不静默吞成 nil 服务/假成功/路由消失） |
| `backend-go/internal/tagmanagement/service/service.go` | 门面追加 `AuxRepairExecutor` 类型别名（生命周期类型） |
| `backend-go/internal/tagmanagement/description_repair_wiring_test.go` | **新增**：域接线的完整后端故事（临时 PG） |
| `backend-go/internal/app/runtime_repair_worker_test.go` | **新增**：应用层 start/stop 缝（无 DB，用记录型 fake） |

**未改动**：board 匹配算法与阈值、`board` 包、RootDB/全局 DSN、旧迁移、其他业务域、前端、任何生产配置项（**未新增 env/配置项**：执行器用既有默认结构 `DefaultAuxRepairExecutorConfig()`）。

## 2. 实际生命周期接口与路径

### 2.1 域根（tagmanagement）

```go
// 初始化（app 启动路径）：DB ready 之后、路由注册之前
func InitDescriptionRepairChain(db *gorm.DB) error
func InstallDescriptionRepairChain(svc *service.AuxRepairService, exec *service.AuxRepairExecutor) // 依赖注入缝
func DescriptionRepairService() *service.AuxRepairService
func StartDescriptionRepairExecutor(ctx context.Context) error   // ctx = 应用生命周期
func StopDescriptionRepairExecutor()                              // cancel + join，幂等
```

- **同一个实例**：`InitDescriptionRepairChain` 建出的 service 同时交给 `InstallDescriptionRepairChain`（域根单例）与 `RegisterRoutes`（路由）——HTTP 面与后台 worker 永远是同一扇门。
- **无懒全局**：没有 `sync.Once` 缓存的 nil 服务；初始化失败直接返回错误，由 app 侧 Fatalf。

### 2.2 路由

`internal/tagmanagement/routes.go` → `RegisterRoutes(api)` 内追加：

```go
RegisterAuxiliaryDescriptionRepairRoutes(rg, DescriptionRepairService())
```

四个资源（沿用 `/api/auxiliary-labels/:id` 与 `:job_id` 通配名，**旧路由全部保留**）：

| 方法 | 路径 |
| --- | --- |
| GET | `/api/auxiliary-labels/:id/description-repair-context` |
| POST | `/api/auxiliary-labels/:id/description-repairs` |
| GET | `/api/auxiliary-labels/:id/description-repairs/:job_id` |
| POST | `/api/auxiliary-labels/:id/description-repairs/:job_id/retry` |

handler 只绑参数/错误/DTO，经 `service` 门面薄调用，**不深 import 业务子包、不写 SQL**。

### 2.3 app 启动与关停

```
main.go
  database.InitDB(...)                     ← AutoMigrate + 迁移完成，DB ready
  taggingdomain.InitRepository(db)
  taggingdomain.InitDescriptionRepairChain(database.DB)   ← 显式创建 service + executor；失败 Fatalf
  appbootstrap.SetupRoutes(r)             ← 路由注册（用域根单例 service）
  appbootstrap.StartRuntime()             ← 应用生命周期 ctx + 启动修复 worker
  appbootstrap.SetupGracefulShutdown(rt, srv)
```

关停顺序（`runShutdownSequence`）：**① HTTP 摘端口 → ② 修复 worker（先 cancel 应用 ctx，再 join）→ ③ registry → ④ workers**。修复 worker 在 DB 释放前必然收束；demo 只读模式（`rt == nil`）下该步是安全 no-op（worker 从未启动）。

**不绑 HTTP ctx**：worker 跑在应用 ctx 上；提交任务的请求 ctx 取消**不影响**已确认任务。**不在路由注册/GET/构造时偷启 goroutine**：`NewService`/`NewExecutor`/路由注册都不启动任何东西，只有 `Start`（显式、幂等）才起 worker。

### 2.4 配置

执行器配置用既有默认结构 `service.DefaultAuxRepairExecutorConfig()`（轮询 5s / 租约 60s / 心跳 20s / 心跳超时 10s / 单执行槽），由 `ExecutorConfig.Validate()` 校验关系（**心跳 < 租约**、各项为正、批次 ≥ 1）。**未新增用户配置项/env**；若将来需要可调，走既有配置结构注入（2.6 之后的独立事项）。

## 3. 验证（一次性 testcontainer PG；严禁真库）

| # | 命令 | 结果 |
| --- | --- | --- |
| 1 | `cd backend-go && GOMAXPROCS=2 go test -count=1 -p 1 -run 'TestDescriptionRepairWiring' -v ./internal/tagmanagement` | **ok**，4/4 PASS |
| 2 | `cd backend-go && GOMAXPROCS=2 go test -count=1 -p 1 -run 'TestRepairWorkerLifecycle' -v ./internal/app` | **ok**，2/2 PASS |
| 3 | `cd backend-go && GOMAXPROCS=2 go test -count=1 -p 1 -run 'TestAuxiliaryDescriptionRepairExecutorResumesTaskWithNullLease' -v ./internal/tagmanagement/service/auxrepair` | **ok**，PASS（前置核查：NULL 租约的已保存 matching 任务**可被真实 RunOnce 领取**） |
| 4 | `cd backend-go && GOMAXPROCS=2 go test -count=1 -p 1 -run 'TestAuxiliaryDescriptionRepair' ./internal/tagmanagement/handler` | ok 24.4s（受影响包精准回归） |
| 5 | `cd backend-go && GOMAXPROCS=2 go test -count=1 -p 1 ./internal/tagmanagement/service/auxrepair` | ok 15.8s（受影响包回归：本次改动触及共享租约/守卫路径） |
| 6 | `cd backend-go && GOMAXPROCS=2 go test -count=1 -p 1 -run 'TestDescriptionRepairWiring' ./internal/tagmanagement` | ok 20.3s |
| 7 | `cd backend-go && GOMAXPROCS=2 golangci-lint run ./internal/tagmanagement/... ./internal/app/... ./cmd/...` | **0 issues** |
| 8 | `cd backend-go && GOMAXPROCS=2 go vet ./internal/tagmanagement/... ./internal/app/... ./cmd/... && go build ./...` | ok |

### 3.1 完整后端故事（`TestDescriptionRepairWiringFullBackendStory`）

真实注册的路由 + 真实启动的 worker + 桩 embedder + 真实 matcher（显式绑 testcontainer DB）：

1. `GET /api/auxiliary-labels/:id/description-repair-context` → 200，**零写**（任务表仍 0 行）；
2. `POST /api/auxiliary-labels/:id/description-repairs` → **202**，任务入库（pending）；
3. **应用生命周期 ctx 上的 worker 自动跑完整条链**：embedding（1 次调用）→ 事务内原子保存（描述 + storage 向量）→ 匹配；
4. `GET /api/auxiliary-labels/:id/description-repairs/:job_id` → **completed**，`stage=matching`、`description_committed=true`、`total=1 / processed=1 / failed=0 / not_applicable=0 / remaining=0`；响应**不含** `claim_owner`/`phase_token`（owner/token 不外泄）；
5. 标签确实拿到新描述与新向量。

### 3.2 复审点名缺失的组合（`TestDescriptionRepairWiringRetryContinuesOnlyFailedItems`）

`matching_failed` → `POST .../retry` → **队列自动领取** → **只续失败项**（匹配的桩引擎对该标签只失败一次，重试时自然成功——**没有手调阶段/状态凑绿**）→ 任务 completed；**embedding 调用数不增**（仍为 1）。

### 3.3 其余断言

- 运行中/已完成的重复 retry → **完全 no-op**（连 `UpdatedAt` 都不变）。
- 旧辅助标签路由仍可达：`GET /api/auxiliary-labels`、`GET /api/auxiliary-labels/clusters` → 200。
- 应用层：worker 的 start 缝收到**应用 ctx**；关停时该 ctx 被 cancel 且 worker 被 stop 恰好一次；无 runtime 时该步是安全 no-op。

## 4. 前置核查：NULL 租约的已保存 matching 任务会不会被扫描？

**结论：会，且已被真实执行路径钉死。** `claimableRepairJobsPredicate` 的第二支显式含 `lease_expires_at IS NULL`，因此「status=matching 且 owner/lease 为空」（显式释放、retry重排等状态；Stop中断也可在原租约到期后恢复）**可被领取**；新增回归 `TestAuxiliaryDescriptionRepairExecutorResumesTaskWithNullLease` 用**真实 Executor RunOnce**（不是手工调 ExecuteMatchingPhase）验证：released（owner/lease 皆 NULL）的已提交任务被领取 → 只跑匹配 → completed，**embedding 次数不增**。因此**没有 B5 缺陷、不需要撤回 2.5**。

## 5. 未解决 / 未做

- **B3旧四R与B4已由db933a23独立复审确认**；B5新增四项修复和B6生命周期接线尚待复审，不把Codex额度失败当作通过。
- **前端**：进度/恢复/retry 入口属 seam C，未动。
- **参考文档对账**：`docs/reference/api|flow|database` 四处与 ai-logging 清单仍属 tasks 6.x。
- **数据隔离红线**：该链会重算**所在库全部 active tags** 的匹配；真实交互故事必须落一次性 PG（命名空间样本不是隔离手段）。
- **运维**：无跨机调度、无失败任务自动重试、无指标/告警。

## 6. 指纹（backend-go/internal/ 下，成稿时磁盘状态）

| 文件 | SHA256 |
| --- | --- |
| tagmanagement/wire.go | ebf5c1dc85b102627176eaa8e0e8f96310c56ae89bd98c1182eb07bd7a9222d4 |
| tagmanagement/routes.go | e6c964d6dce59b5f75bcebc9a1a36e04a3ad8800e32ad453ec91ef6960ad1737 |
| tagmanagement/service/auxrepair/executor.go | e3246ca0d3d3527e5e119e08dafc4d5b3b5b52cafcedbc2c65a7fcde561b93d3 |
| tagmanagement/service/auxrepair/embedding.go | 65e0a0a858c9d56e8abce75c7952f9e4d98f99128c7e4548020ff2fb61ff3170 |
| tagmanagement/service/auxrepair/matching.go | 6b88151bd1f46cafd6f311de7df7cf39d28d26548f974dbd1c8570d7c4a47808 |
| tagmanagement/repository/auxiliary_description_repair_stage.go | 3856aa60e5eb08a435ad269520082cef9265e76841c90fb060461927378a5d7f |
| tagmanagement/repository/auxiliary_description_repair_executor.go | d9f01d5f80056104d23fa895dd79dd985938a4a684aa2c5d7f446057fbff9769 |
| tagmanagement/handler/auxiliary_description_repair_handler.go | a6ac06d3c02167d1b31d1dbdc8fd0d41bec4f0a63c622942267f532808f197b1 |
| tagmanagement/description_repair_wiring_test.go | 55b026d7a8b5d3434f70a533db540241a7349b10a4752089bea80339d1090df7 |
| tagmanagement/service/service.go（含外部既有脏改） | 86910dcc69d642359e5308aefd6a117db897d16002772ac9dbf918d3651e39f4 |
| app/runtime.go | 10e5a1cc5655cca31a0047016436bb86f18a948307ac1b069584c7d588a548c2 |
| app/runtime_repair_worker_test.go | ade5b7fd971dfd2faf279ba7718a8fdca24ad6bdf449833ad68285111f7928ab |

> 续做时以 `sha256sum` 复核为准。
