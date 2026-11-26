<!-- complexity: simple -->
<!-- ui-impact: none -->

## Why

后端进程的日志同时流向两个地方：已配置 lumberjack 轮转的 `logs/app.log`，以及**无任何轮转的 stdout**（由 `start-dev.sh` 重定向到 `backend-go/nohup.out`）。Gin 访问日志更是完全绕过 slog，只写 stdout。结果是同一份日志写两遍，一份被 50MB×30 的轮转管住，另一份无界增长——2026-11-24 夜间 `nohup.out` 长到 **756MB / 285 万行**，而根分区已用 79%。

## What Changes

- `logging` 包暴露统一的文件 `io.Writer`，供非 slog 通道（Gin）复用同一个 lumberjack 实例，避免多套轮转配置。
- `cmd/server/main.go` 在 `gin.Default()` 之前把 `gin.DefaultWriter` / `gin.DefaultErrorWriter` 指向该 writer（仅在 `log.file.enabled=true` 时）。
- `logging.Init` 在文件日志启用时**不再向 stdout 扇出**（当前 `fanoutHandler` 双写是 `nohup.out` 膨胀的一半来源）；新增 `LOG_CONSOLE=true` 显式开关，用于需要 `tail` stdout 的临时调试。
- `configs/config.yaml` 的 `log.level` 由 `debug` 调整为 `info`，`debug` 通过 `LOG_LEVEL` 环境变量按需覆盖。
- 同步更新 `scripts/dev/start-dev.sh` 的日志提示、`docs/reference/configuration.md` 与 `development.md` 的日志查看方式（`tail -f backend-go/logs/app.log`）。

**BREAKING**（开发体验）：默认情况下 `backend-go/nohup.out` 只剩启动横幅，运行日志不再出现在其中。

## Capabilities

### New Capabilities

（无——纯运维/日志管道改动，不改变对外行为契约，`.openspec.yaml` 置 `skip_specs: true`）

### Modified Capabilities

（无）

## Impact

- **代码**：`backend-go/internal/platform/logging/logging.go`、`backend-go/cmd/server/main.go`
- **配置**：`backend-go/configs/config.yaml`（`log.level`）
- **脚本/文档**：`scripts/dev/start-dev.sh` 注释、`docs/reference/configuration.md`、`docs/reference/development.md`
- **运行时**：`backend-go/nohup.out` 预期从 GB 级降到 KB 级；全部运行日志进 `backend-go/logs/app.log`（50MB 切分、30 份、30 天、gzip）
- **不影响**：API 契约、数据库、前端、调度器行为
