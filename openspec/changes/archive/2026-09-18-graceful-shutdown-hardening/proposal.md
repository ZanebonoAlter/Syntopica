<!-- complexity: complex -->
<!-- ui-impact: none -->
<!-- constraint-domains: scheduler -->

## Why

2026-09-17 实测取证（`docs/research/orphan-dev-processes/explore-findings.md`「后端优雅关停偏慢」节）：后端收到 SIGTERM 后 **≈50 秒**才退出，期间端口一直 LISTEN。三个叠加缺陷：

1. **HTTP 端口从不显式摘除**：gin 经 `r.Run()` 启动，`SetupGracefulShutdown`（runtime.go:356）里没有任何 `http.Server.Shutdown` 调用——端口要等 `os.Exit(0)` 才释放，期间产生「调度器已停、端口还在接单」的半死窗口；
2. **`StopAllWorkers()` 无超时**（workers.go:24）：tag queue / embedding / merge-reembedding 三个 Stop 串行阻塞，任一卡住整个关停无限等（50s 的主要嫌疑）；
3. **`os.Exit(0)` 跳过全部 defer**：`main.go:86` 的 tracer flush（`tp.Shutdown(context.Background())`）与 `logging.Close()` **从未真正执行过**——OTLP 遥测尾部丢失、日志缓冲不落盘。

连带后果：start-dev.sh 的 10s 观察窗必然误报「仍在监听」（本次已用 KILL 升级止血，但生产语义下 KILL -9 不可取）；将来容器化部署时编排器的 SIGTERM 宽限期（通常 10~30s）内必然被强杀。

## What Changes

- **关停顺序重构**（main.go + runtime.go）：`r.Run()` 换 `srv.ListenAndServe()`（`*http.Server` 显式持有）；SIGTERM 处理顺序改为：**① `srv.Shutdown(5s ctx)` 先摘端口、排空在途请求 → ② `Registry.StopAll(30s)`（保持现状上限）→ ③ `StopAllWorkers` 外部包 10s 上限（workers.go 本体不动）→ ④ 记录每步耗时 → main 正常 return**（defer 链恢复执行：tracer flush + logging.Close 落地）。不再 `os.Exit(0)`。
- **双模式覆盖**：`DEMO_READ_ONLY=1`（无 Registry）也注册信号处理做 HTTP 优雅摘除（runtime 传 nil，Registry 步骤跳过）——今天 demo 模式 SIGTERM 是硬杀。
- **可观测性**：每步关停打耗时日志（http/registry/workers/…），一次关停就能看清 50s 花在哪。
- **行为兼容**：端口占用启动失败仍 `Fatalf`；退出码 0；`DEMO_READ_ONLY` 语义不变。

## Capabilities

### New Capabilities

- `graceful-shutdown-hardening`：后端关停协议——摘端口先行、逐步有界、耗时可观测、defer 链完整执行、双模式覆盖。

### Modified Capabilities

- 无（scheduler 业务行为不变；`scheduler` flow 的调度器可中断/可恢复约束不变，仅生命周期收尾方式变）。

## Stakeholder Concerns

- **调度器任务安全**：`Registry.StopAll(30s)` 上限保持不变，调度器内部持久化/幂等语义（flow/scheduler 约束）不动；摘端口先行只影响「不再接新请求」，在途请求有 5s 排空。
- **关停总时长**：预算 http 5s + registry 30s + workers 10s ≈ 45s 上限，但**端口在第一步就释放**（start-dev.sh 观察窗、容器 SIGTERM 宽限期关心的都是端口与进程存活——端口 1~2s 内释放；进程总时长可超但不影响端口语义）。
- **测试可行性**：`StopAllWorkers` 依赖 DB 单例，经包级函数变量注入缝隔离单测（对齐既有打桩风格）。
