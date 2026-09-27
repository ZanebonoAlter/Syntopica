# Design — graceful-shutdown-hardening

> 事实依据：`docs/research/orphan-dev-processes/explore-findings.md`「后端优雅关停偏慢」节（2026-09-17 实测 TERM → ≈50s 才退出，端口全程 LISTEN）。

## D1 关停顺序：摘端口先行

信号处理序列固定为四步，**端口最先释放**：

```
SIGTERM/SIGINT → ① srv.Shutdown(ctx 5s)   # 关 listener（端口立即释放）+ 排空在途请求
               → ② Registry.StopAll(30s)  # 上限保持现状，调度器持久化/幂等语义不动
               → ③ stopWorkersFn ⊢ 10s    # 外部包超时，workers.go 本体零改动
               → ④ 耗时日志单行汇总 → close(done)
```

理由：start-dev.sh 观察窗与容器 SIGTERM 宽限期关心的都是**端口与进程存活**；端口 1~2s 内释放后，后续步骤再慢也不阻塞运维语义。50s 的构成数据由 ④ 的耗时日志在实战验收时采集，属观测产物不在本 change 猜测。

## D2 main 结构：正常 return 恢复 defer 链

`r.Run(addr)` 换 `srv := &http.Server{Addr, Handler: r}` + goroutine 跑 `ListenAndServe`，错误经 channel 回主 goroutine；`select { done / err }` 后 **main 正常 return**（不再 `os.Exit(0)`）——defer 链恢复执行：`tp.Shutdown`（换 `context.WithTimeout(5s)`，修 `context.Background()` 永等）与 `logging.Close()` 真正落地。`ListenAndServe` 返回 `http.ErrServerClosed`（Shutdown 引发）不算错。

**否决**：信号处理里 `os.Exit`（回到跳 defer 的老坑）；main 无限等 done（关停链若 panic，进程挂死——处理 goroutine 顶层 recover 后仍关 done，fail-open）。

## D3 双模式：demo 模式也有优雅摘端口

`SetupGracefulShutdown` 改为**总是注册**，签名扩展为 `(runtime *Runtime, srv *http.Server)`；`runtime == nil`（DEMO_READ_ONLY=1）时跳过 ② Registry 步骤，其余照走。现状 demo 模式 SIGTERM 是默认硬杀——顺手修掉。

## D4 workers 有界：注入缝而非改内部

包级 `var stopWorkersFn = tagmanagement.StopAllWorkers`，信号处理经 `go stopWorkersFn()` + `select / time.After(10s)` 包界；超时只告警不重试（os.Exit 语义下卡死的 worker 随进程终止，队列持久化保证可恢复）。workers.go **零改动**——三个 Stop 的内部阻塞语义（等当前任务收尾）保持，界加在外层。

注入缝动机：`StopAllWorkers` 依赖 DB 单例（GetTagQueue），单测注入空函数即可跑 `go test ./internal/app`，无需 DB。

## D5 可观测性

四步各计耗时，完成时单行汇总日志（`shutdown steps: http=1.2s registry=30.0s workers=10.0s total=41.3s`），50s 构成数据一次关停即可采集；任一步骤异常不阻断后续步骤（逐步 try/recover，对应失败记 warn）。

## D6 行为兼容

- 端口占用启动失败：`ListenAndServe` 返回非 `ErrServerClosed` 错误 → `Fatalf`，行为不变；
- 退出码 0；
- `/health` 在 ① 完成后对新连接立即拒绝（listener 已关）；
- `DEMO_READ_ONLY` 对调度器的跳过语义不变（只是关停处理总注册）。

## D7 测试策略

- 同包单测（`go test ./internal/app`，禁 DB 依赖）：注入 `stopWorkersFn` / 传 nil runtime / httptest 或自建 listener 验证摘端口时序与有界性；耗时用注入时钟或短上限（如 200ms 注入值）避免慢测；
- 集成（真进程，人工留痕）：起后端 → TERM → 端口 ≤5s 释放、进程退出码 0、日志含耗时行、start-dev.sh stop 不再触发 KILL 升级。

## D8 实现契约（apply 冻结，单测据此落用例）

> 本节的包级 var 注入缝是 D4 的机械延伸（§8「局部澄清」留痕：为让 test-cases.md TC-09~TC-11 的步级 panic 分支可在无 DB 条件下注入，除 D4 的 `stopWorkersFn` 外另立 4 个缝——`stopRegistryFn` / `shutdownHTTPStepFn` / `signalChanFactory` / `listenAndServeFn`；均为 3~6 行，默认实现与线上语义逐一对应，不引入生产行为差异）。

`internal/app/runtime.go` 尾部新增（命名以此为准，test-cases.md 的「前置实现契约」表按此对齐）：

```go
type shutdownTimings struct{ HTTP, Registry, Workers, Total time.Duration }

var ( // 包级 var（非 const）：单测覆写短上限，避免真实 5s/30s/10s 等待
    httpShutdownTimeout     = 5 * time.Second
    registryShutdownTimeout = 30 * time.Second
    workersShutdownTimeout  = 10 * time.Second
)

var (
    stopWorkersFn     = tagging.StopAllWorkers                       // D4 注入缝
    stopRegistryFn    = func(r *Runtime, timeout time.Duration) { r.Registry.StopAll(timeout) }
    shutdownHTTPStepFn = func(srv *http.Server) error {               // ① 步缝（TC-11 panic 注入面）
        ctx, cancel := context.WithTimeout(context.Background(), httpShutdownTimeout)
        defer cancel()
        return srv.Shutdown(ctx)
    }
    signalChanFactory = func() <-chan os.Signal { /* 建 chan + signal.Notify(SIGINT,SIGTERM) */ }
)

func runShutdownStep(name string, fn func()) time.Duration      // 独立 recover，panic 记 warn 并返回耗时
func runShutdownSequence(rt *Runtime, srv *http.Server) shutdownTimings   // 四步 + 汇总行
func SetupGracefulShutdown(rt *Runtime, srv *http.Server) <-chan struct{} // 信号 → 序列 → close(done)

var listenAndServeFn = func(srv *http.Server) error { return srv.ListenAndServe() }
func RunServer(srv *http.Server, done <-chan struct{}) error    // 返回启动错误；ErrServerClosed 不算错误
```

不变式（实现必须满足）：

1. `done` **必关闭**：信号 goroutine `defer close(done)` + 顶层 recover（panic 也不挂 main）；
2. `RunServer` 只在**非** `ErrServerClosed` 的 `ListenAndServe` 错误上返回 err；`ErrServerClosed` 不解除 `select`——必须等 `done`（否则 ① 排空期间 main 提前 return，②③ 被跳过）；
3. workers 步骤在**同步段**捕获 `fn := stopWorkersFn` 再进 goroutine（避免单测还原 var 时与仍在跑的 goroutine 竞态）；该 goroutine 自身带 recover（panic 不杀进程）；
4. 汇总行格式 `shutdown steps: http=%.1fs registry=%.1fs workers=%.1fs total=%.1fs`，步骤被跳过（nil runtime/srv）时字段照打、值 0.0s；
5. 生产路径不再出现 `os.Exit`；`logging.Fatalf`（启动失败）保留在 main 且行为不变。

测试契约（同文件 `internal/app/runtime_shutdown_test.go`，`package app`，**禁 DB**）：`logging.SetWriters` 捕获日志；fake scheduler 经 `admin.NewSchedulerRegistry()` + `Register` 注入；listener 用 `net.Listen("tcp","127.0.0.1:0")` + `go srv.Serve(ln)` 自建；信号路径用注入的 `signalChanFactory` 触发，**不发真实信号**。

## 风险与缓解

| 风险 | 缓解 |
| --- | --- |
| 在途请求 5s 排不完 | ctx 到期 `Shutdown` 强制关闭——有界优先；请求方有重试语义（前端幂等读为主） |
| StopAll 30s + workers 10s 关停总时长仍长 | 端口已先行释放，不影响运维语义；耗时日志采集后另开 change 再收敛总时长 |
| 改动 main 启动路径引入回归 | 行为兼容清单（D6）逐项入用例；`go build` + lint + `go test ./internal/app` 门禁；实战验收真进程过一遍 |
