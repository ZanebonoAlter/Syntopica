# graceful-shutdown-hardening Specification

## Purpose
TBD - created by archiving change graceful-shutdown-hardening. Update Purpose after archive.

## Requirements

### Requirement: 关停顺序与端口先行释放

后端 SHALL 显式持有 `*http.Server` 并以 `ListenAndServe` 启动；收到 SIGTERM/SIGINT 后按固定顺序执行关停：**① `srv.Shutdown`（5s 超时，关闭 listener 使端口立即释放并排空在途请求）→ ② `Registry.StopAll(30s)`（runtime 非空时）→ ③ 工作队列停止（10s 上限外包）→ ④ 耗时汇总日志**。端口释放 MUST 先于 ②③ 执行；②③ 任一步骤异常或超时 MUST NOT 阻断后续步骤与进程退出（逐步记录 warn 后继续）。

#### Scenario: SIGTERM 后端口先行释放

- **WHEN** 后端运行中收到 SIGTERM，Registry 停止耗时超过 10s
- **THEN** 端口在 `srv.Shutdown` 完成时（远早于进程退出）即不再接受新连接，`/health` 新连接被立即拒绝

#### Scenario: 在途请求排空

- **WHEN** 信号到达时存在未完成的在途 HTTP 请求
- **THEN** `srv.Shutdown` 等待其在 5s 内完成；5s 到期后强制关闭，不无限等

#### Scenario: 逐步异常不阻断退出

- **WHEN** ③ 工作队列停止卡死超过 10s 上限
- **THEN** 记录 warn 后继续执行后续步骤并正常退出，不挂死、不重试

### Requirement: 关停有界性与 defer 链完整执行

关停全链路 MUST 有界：HTTP 排空 5s、Registry 30s（维持现状）、工作队列 10s。信号处理完成 MUST NOT 以 `os.Exit` 结束，而应使 main 正常 return——`tracer Shutdown`（5s 超时 context，替换现状 `context.Background()`）与 `logging.Close()` 等 defer MUST 真实执行。`ListenAndServe` 因 `Shutdown` 返回的 `http.ErrServerClosed` MUST NOT 被当作启动失败；端口占用等真实启动错误 MUST 仍走 `Fatalf`（行为兼容，退出码 0 语义不变）。

#### Scenario: defer 链恢复执行

- **WHEN** 优雅关停完成后进程退出
- **THEN** tracer Shutdown 与 logging.Close 的日志/副作用出现（不再被 os.Exit 跳过），进程退出码为 0

#### Scenario: 端口占用启动失败行为不变

- **WHEN** 启动时目标端口已被其他进程监听
- **THEN** 进程以 Fatalf 退出（错误日志 + 非 0 退出码），与现状一致

#### Scenario: ErrServerClosed 不误判

- **WHEN** 优雅关停引发 `ListenAndServe` 返回 `http.ErrServerClosed`
- **THEN** 不记启动失败、不触发 Fatalf

### Requirement: 双模式覆盖

信号处理 SHALL 在所有运行模式下注册：`DEMO_READ_ONLY=1`（runtime 为 nil）时跳过 Registry 步骤但仍执行 HTTP 摘端口与工作队列停止；非 demo 模式行为不变。

#### Scenario: demo 模式优雅摘端口

- **WHEN** `DEMO_READ_ONLY=1` 启动的后端收到 SIGTERM
- **THEN** 端口优雅释放、进程退出码 0（现状为默认硬杀）

### Requirement: 关停耗时可观测

四步关停 SHALL 在完成时输出单行耗时汇总日志（含各步秒数与总计），使关停慢的构成可被单次关停直接采集。

#### Scenario: 耗时汇总行出现

- **WHEN** 一次优雅关停完成
- **THEN** 日志含 `http=`、`registry=`、`workers=`、`total=` 的耗时汇总行
