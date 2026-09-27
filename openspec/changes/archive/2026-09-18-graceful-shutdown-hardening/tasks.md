# Tasks — graceful-shutdown-hardening

## 1. main.go：显式 http.Server + 正常退出

- [x] 1.1 `r.Run(addr)` 换 `srv := &http.Server{Addr: addr, Handler: r}`；`ListenAndServe` 放 goroutine、错误经 channel 回主 goroutine；`select { done / err }`：err 非 `http.ErrServerClosed` → `Fatalf`（行为不变），否则 return（defer 链恢复：tracer `tp.Shutdown(5s ctx)` 替换 `context.Background()`、`logging.Close()`）
- [x] 1.2 `SetupGracefulShutdown(runtime, srv)` 签名扩展；`DEMO_READ_ONLY=1` 分支改为传 `nil` runtime 并总是注册信号处理

## 2. runtime.go：四步关停协议

> apply 留痕（§8 局部澄清，2026-09-18）：实现契约冻结在 design.md **D8**（包级 var：`httpShutdownTimeout`/`registryShutdownTimeout`/`workersShutdownTimeout` + 注入缝 `stopWorkersFn`/`stopRegistryFn`/`signalChanFactory`/`listenAndServeFn` + 同步 `runShutdownSequence` + `RunServer`）；白盒用例分支表见 `test-cases.md`（24 自动化 + 4 实战）。D8 的注入缝是 D4 的机械延伸（让步级 panic/阻塞分支可在无 DB 下单测），生产行为不变。

- [x] 2.1 信号处理顺序重构：① `srv.Shutdown(ctx 5s)` → ② runtime 非 nil 时 `Registry.StopAll(30s)` → ③ `stopWorkersFn` goroutine + `select / time.After(10s)` → ④ 耗时单行汇总日志（`shutdown steps: http=…s registry=…s workers=…s total=…s`）；移除 `os.Exit(0)`，完成后 close(done)
- [x] 2.2 包级 `var stopWorkersFn = tagmanagement.StopAllWorkers`（注入缝，D4）；处理 goroutine 顶层 recover 关 done（fail-open 防挂死）
- [x] 2.3 main.go tracer defer 的 `context.Background()` 换 `context.WithTimeout(5s)`

## 3. 测试

- [x] 3.1 同包单测 `backend-go/internal/app/runtime_shutdown_test.go`：注入 stopWorkersFn / stopRegistryFn / shutdownHTTPStepFn / signalChanFactory / listenAndServeFn、nil runtime / nil srv、自建 listener——覆盖 `test-cases.md` 分支表（顺序、有界、nil 安全、ErrServerClosed、端口占用、耗时日志行）；**禁 DB 初始化**
- [x] 3.2 workers.go / scheduler 内部零改动核对（只加外层界）

## 4. 文档

<!-- doc-impact: flow architecture -->
- [x] `docs/reference/architecture/runtime.md`：启动顺序（9~11 步）与「优雅退出怎么做」节改写为四步关停协议（apply 留痕：改动使该文档旧描述失效，§11.1 条件 2 文档同步；§8 局部范围扩展）
- [x] `docs/reference/flow/scheduler.md`：变更溯源表补一行（归档后，§12.2 —— 已完成 2026-09-18）
- [x] §12.2 溯源：`flow/scheduler.md` 尾部「变更溯源」表已追加 `| 2026-09-18 | graceful-shutdown-hardening | 关停协议重构：摘端口先行 + 有界 + defer 链恢复 | openspec/changes/archive/2026-09-18-graceful-shutdown-hardening |`

## 5. 测试

- 影响测试命令（后端，按 change-scope 只跑影响包）：
  - `cd backend-go && go test ./internal/app` → PASS（新增 runtime_shutdown_test）
  - `cd backend-go && golangci-lint run ./internal/app ./cmd/server && go vet ./internal/app ./cmd/server && go build ./...` → 全绿

## 6. 验证

- [x] `cd backend-go && go test ./internal/app` → PASS（25 个关停用例 + 既有包内用例；独立进程重复 5 轮全绿）
- [x] `cd backend-go && golangci-lint run ./internal/app ./cmd/server` → 0 issues
- [x] `cd backend-go && go vet ./internal/app ./cmd/server && go build ./...` → 全绿
- [x] 实战验收（留痕到本节，实测 2026-09-18；复验于最终代码）：
  - 全模式：`start-dev.sh back` 起新后端 → `kill -TERM <监听pid>` → **端口 7ms 释放**、进程 60ms 退出、nohup.out 含且仅含 1 行 `shutdown steps: http=0.0s registry=0.0s workers=0.0s total=0.0s` + `Graceful shutdown completed`（改动前同场景端口需等 os.Exit，≈50s）
  - `start-dev.sh stop back`（对活进程）→ **0.076s 完成、零「kill -9 兜底」、零「仍在监听」告警**
  - demo 模式（`DEMO_READ_ONLY=1`，5199 隔离端口）→ TERM 后端口 **9ms 释放**、**退出码 0**（改动前为默认硬杀）、汇总行存在；TERM 后 `otel_spans` 新增 13 行 → 证明 tracer `tp.Shutdown` defer 真实执行（批处理定时器 5s + 关停仅 29ms，只能是 defer flush）
  - 端口占用：先占住 5199 再起进程 → `exit_code=1` + `Failed to start server: listen tcp :5199: bind: address already in use`（行为不变）
- [x] review（独立子线程，deepseek-v4-pro）：1 High（测试夹具 `Serve`/`Shutdown` 竞态使「端口已释放」断言偶发误红且 http 步存在覆盖空洞）已修——`startTestHTTPServer` 增 `/__ready` 就绪探针对齐 Serve 注册 listener（INV-9）；Medium「TC-11 序列级 http panic」在并发补入的 `shutdownHTTPStepFn` 缝后由 `TestShutdownSequence_HTTPPanicIsolated` 覆盖；其余 Low 已记录为残留风险
- [x] `bash scripts/doc-impact.sh verify openspec/changes/graceful-shutdown-hardening` → PASS（声明 flow architecture）
- [x] `bash scripts/check-standards.sh --change graceful-shutdown-hardening` → 166 通过 / 0 失败
- [x] `bash scripts/scenario-trace.sh openspec/changes/graceful-shutdown-hardening` → 退出码 0（8/8）

> 残留风险（已知，不阻断）：① `Registry.StopAll` 内部 goroutine 的 panic 不可被调用方 recover（D8 只保证同步调用面隔离）；② 测试失败路径（boundedWait 超时）与仍存活的序列 goroutine 存在写包级 var 的理论竞态（仅失败路径，本机 `-race` 不可用：ThreadSanitizer 不支持本内核 VA 位宽）；③ `signalChanFactory` 仅测试注入面可能返回 nil 通道（生产 make 无条件非 nil，不做 nil 防御）。

### Scenario → 测试文件映射

| Scenario | 测试文件 |
| --- | --- |
| SIGTERM 后端口先行释放 | backend-go/internal/app/runtime_shutdown_test.go |
| 在途请求排空 | backend-go/internal/app/runtime_shutdown_test.go |
| 逐步异常不阻断退出 | backend-go/internal/app/runtime_shutdown_test.go |
| defer 链恢复执行 | backend-go/internal/app/runtime_shutdown_test.go |
| 端口占用启动失败行为不变 | backend-go/internal/app/runtime_shutdown_test.go |
| ErrServerClosed 不误判 | backend-go/internal/app/runtime_shutdown_test.go |
| demo 模式优雅摘端口 | backend-go/internal/app/runtime_shutdown_test.go |
| 耗时汇总行出现 | backend-go/internal/app/runtime_shutdown_test.go |

> 上表中「defer 链恢复执行」「耗时汇总行出现」另有人工/实战验收部分（MAN-01 / MAN-04，本节实战验收条目留痕；scenario-trace 单元格只接受仓库根相对路径或「人工…」前缀，故不混写）。
