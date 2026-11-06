# B6 复审 4 项 Low 收尾记录（独立复审 run `db933a23` 的 Finding 1-4）

> 2026-10-29。B6 独立复审（只读审固定快照 `/tmp/board-health-b6-review-62WReNwQ`，MiniMax 通道，run `db933a23`）结论：**OK with notes，无 High/Medium 阻断**；B3 旧四 R、B4、B5 四项修复经本次复审**确认真实关闭**；B6 接线未发现阻断缺陷。新提 **4 项 Low**，本批逐项收尾。
> 修复者：单写者（`minimax-cn/MiniMax-M3.1-Flash:max`）。写入范围：`service/auxrepair/embedding.go`、`app/runtime.go` + `runtime_repair_worker_test.go`、`tagmanagement/wire.go`（注释 + 必要观测口）与对应精准测试。**未触前端/C1、board 算法、其他业务域、旧迁移、环境 DSN、真库。**

## 1. 四项 Low → 处置

| # | 级别 | 发现 | 处置 | 复验 |
| --- | --- | --- | --- | --- |
| F1 | Low | 租约权威已移到 DB 时钟，但 service 预检仍用**应用时钟**：`ownsJob` 的 `s.now().Before(lease)` 与 matching 的同一预检——应用时钟**领先** DB 超过租约时长时，预检直接判失权 → 任务每轮被重领、永远不推进（活锁）。 | 预检**只判 owner**；租约的权威交给 SQL：`AcquireRepairPhase`（embedding 阶段，**AI 调用之前**）与 `lockOwnedMatchingJob`（matching 阶段写入守卫）都用 `clock_timestamp()` 判定。**过期仍是零 AI/零引擎调用**（阶段领取先于引擎调用）。应用时钟降级为**诊断**（与 DB 时钟明显背离时记一条 Warn），`WithClock` 与构造签名保持兼容。 | `TestAuxiliaryDescriptionRepairExecutorLeaseClockAheadStillCompletes` → PASS（应用时钟**领先 1 小时**：真实 `RunOnce` 一次跑完 embedding→保存→匹配，任务 completed、processed=2、embedding 调用数=1 → 无热扫、无误报 ownerLost；下一轮 RunOnce=0） |
| F2 | Low | 关停 repair 步直接 `Stop()`（内部 `wg.Wait()`），**无有界超时**；worker 卡在不响应 ctx 的外部调用时，优雅关停会无限挂住。 | repair 步改为**有界 join**：`cancel 应用 ctx`（不可省略）→ 后台 goroutine 执行 stop → `select` 等 `repairShutdownTimeout`（10s，与 workers 步同量级；单测覆写为 200ms）→ **超时只告警并继续**后面的 registry/workers 步，**不谎称已 join**；panic 隔离同 workers 步。 | `TestRepairWorkerShutdownIsBounded` → PASS（注入阻塞 stop 缝：总耗时受限、后续 workers 步仍执行、应用 ctx 已 cancel；cleanup 释放阻塞，无残留 goroutine，不等生产的 10s） |
| F3 | Low（覆盖缺口） | app→域**真实 Start 绑定**无用例：app 测试只替换 fake 缝，生产绑定仅靠阅读确认。 | 新增**真绑定**回归：**不替换 start 缝**，在一次性 testcontainer PG 上 `tagging.InitDescriptionRepairChain(db)` 装上**真实域链**，用生产缝 `startRepairExecutorFn(appCtx)` 启动真实执行器；断言：worker 真的在跑；**被取消的请求 ctx 不会停掉 worker**（worker 跑在应用 ctx 上）；取消**应用** ctx 后 join 立即返回（worker 随应用 ctx 结束）。全程**不排任何修复任务** → 不会调用任何 provider。 | `TestRepairWorkerRealDomainBinding` → PASS |
| F4 | Low（API 形状） | `InstallDescriptionRepairChain` 允许在路由注册后调用 → 路由持旧 service、worker 用新 service，形成「两扇门」（生产 main 顺序本来正确）。 | 不新增热替换能力、不引入全局状态机：**把顺序约束写进 API 文档注释**（DB ready 之后 / 路由注册之前 / worker 启动之前；运行中替换需先停旧链并重建宿主），并加一条**轻量顺序回归**钉住「域服务的就是装上的那一个、路由在安装之后注册且由该服务应答」。 | `TestDescriptionRepairWiringOrderContract` → PASS（`require.Same` 断言域服务即安装实例；路由应答的是 service 的错误信封而非 Gin 的 404） |

## 2. 精确验证命令与结果（串行，GOMAXPROCS=2 / -p 1）

| # | 命令 | 结果 |
| --- | --- | --- |
| 1 | `cd backend-go && GOMAXPROCS=2 go test -count=1 -p 1 -run 'TestAuxiliaryDescriptionRepairExecutorLeaseClockAheadStillCompletes' -v ./internal/tagmanagement/service/auxrepair` | **ok**，PASS（F1） |
| 2 | `cd backend-go && GOMAXPROCS=2 go test -count=1 -p 1 -run 'TestRepairWorker' -v ./internal/app` | **ok**，4/4 PASS（F2 + F3 + 既有两例） |
| 3 | `cd backend-go && GOMAXPROCS=2 go test -count=1 -p 1 -run 'TestDescriptionRepairWiringOrderContract' -v ./internal/tagmanagement` | **ok**，PASS（F4） |
| 4 | `cd backend-go && GOMAXPROCS=2 go test -count=1 -p 1 ./internal/tagmanagement/service/auxrepair` | ok 16.2s —— **受影响包回归**（F1 改动了共享的阶段预检，故对该包整体复跑一次；F1 的证据是上面第 1 条） |
| 5 | `cd backend-go && GOMAXPROCS=2 go test -count=1 -p 1 -run 'TestRepairWorker\|TestDescriptionRepairWiring' ./internal/app/ ./internal/tagmanagement/` | ok 13.1s / ok 7.6s |
| 6 | `cd backend-go && GOMAXPROCS=2 golangci-lint run ./internal/tagmanagement/... ./internal/app/...` | **0 issues** |
| 7 | `cd backend-go && GOMAXPROCS=2 go vet ./internal/tagmanagement/... ./internal/app/... && go build ./...` | ok |

**未跑**（按纪律刻意不跑）：旧 34 例的整包复跑（除上面第 4 条的受影响包回归）、前端测试、外部已绿门禁、真库/环境 DSN/psql、dev/app 起停、部署、派生子线程。

## 3. 边界与遗留

- **未宣称整个 change 完成**：B6 复审的 4 项 Low 已收尾，但**前端（seam C）、参考文档对账、真实部署验证**仍未完成；正式账本仍是 11/29。
- **关停语义**：超时只是**不无限挂住**——卡住的 worker 会随进程退出被回收（与 workers 步同一取舍）；任务留在库里，租约到期后由新执行者接管（失租恢复路径已就位）。
- **时钟权威**：租约期限与判定**全部**在 DB（`clock_timestamp()`）；应用时钟只用于诊断与日志，**不再决定任何 DB 租约**。
- 本记录只解除 B6 复审的 4 项 Low，**不替整个 change 最终放行**。
