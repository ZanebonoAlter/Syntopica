
## 关停链路现状与改造点（代码事实）

现状调用链（2026-09-18 核对）：
- `backend-go/cmd/server/main.go:120` `r.Run(addr)`（gin）启动，端口无显式摘除；main 末尾 `logging.Fatalf` 启动失败路径。
- `backend-go/cmd/server/main.go:86-90` tracer defer `tp.Shutdown(context.Background())`（永等，需换 5s ctx）。
- `backend-go/internal/app/runtime.go:356-373` `SetupGracefulShutdown(runtime *Runtime)`：signal.Notify(SIGINT,SIGTERM) → goroutine 里 `tagging.StopAllWorkers()`（无超时，内部三队列串行 Stop：tag queue / embedding / merge-reembedding，见 `internal/tagmanagement/service/core/workers.go:24`，均依赖 `GetTagQueue()` DB 单例）→ `runtime.Registry.StopAll(30s)` → `os.Exit(0)`（跳 defer）。
- `internal/admin/wire.go:149` `type SchedulerRegistry = scheduler.Registry`（别名）；`NewSchedulerRegistry = scheduler.NewRegistry`，可 `Register(name, scheduler.Scheduler)` 注入 fake，无需 DB。
- `internal/admin/scheduler/registry.go:80` `StopAll(timeout)`：内部 goroutine + select time.After，超时仅 warn 不阻塞。
- demo 模式（main.go:110-116）：`DEMO_READ_ONLY=1` 时不 StartRuntime 且不注册信号 → TERM 硬杀。

可测性缝（已存在）：
- `logging.SetWriters(info, err)` / `logging.ResetWriters()`（`internal/platform/logging/logging.go:76-89`）可把 slog 输出捕获到 buffer，用于断言 `shutdown steps: http=… registry=… workers=… total=…` 汇总行；仓库已有用法 `internal/platform/logging/logging_test.go:12`、`internal/admin/service/discovery_run_service_test.go:477`。
- `internal/app` 同包测试可用 httptest/自建 listener 起真实 `http.Server`；`internal/app/runtime_test.go` 已有「不起 StartRuntime、用 SQLite 内存库」的测试惯例，本次单测要求**禁 DB**。
- 失败路径：`logging.Fatalf`（logging.go:116）内部 `os.Exit(1)` → 启动失败仍非 0 退出码（行为兼容）。
- 本地运行态：pidfile `.pi/run/backend.pgid`（go run 父进程），真实监听 PID 为编译产物子进程（ps 可查 `main` 子进程）；实战验收须对**监听 PID** 发 TERM。

**引用**：backend-go/cmd/server/main.go:86、backend-go/cmd/server/main.go:120、backend-go/internal/app/runtime.go:356、backend-go/internal/tagmanagement/service/core/workers.go:24、backend-go/internal/admin/scheduler/registry.go:80、backend-go/internal/platform/logging/logging.go:76、backend-go/internal/admin/wire.go:149

<!-- pinned 2026-09-17T15:39:00Z -->
