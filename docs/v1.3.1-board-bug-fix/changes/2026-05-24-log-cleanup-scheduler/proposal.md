## Why

`ai_call_logs` 和 `otel_spans` 两张日志表无限制增长。`otel_spans` 已有内置 7 天清理（`SQLiteSpanExporter.cleanupLoop`），但 `ai_call_logs` 完全没有清理机制。两者应统一纳入 scheduler 系统管理，而非散落在不同 goroutine 里。

当前规模：ai_call_logs ~14k 行/天，otel_spans ~20k 行/天。不清理会持续膨胀磁盘和索引。

## What Changes

- 新建 `LogCleanupScheduler` 定时任务，24 小时执行一次，清理两张表 7 天前的数据
- 从 `SQLiteSpanExporter` 移除内嵌的 `cleanupLoop`，将清理职责收敛到 scheduler
- 给 `ai_call_logs.created_at` 添加数据库索引（当前缺失，DELETE 会全表扫描）
- `otel_spans` 清理复用已有 `start_time_unix_nano` 索引，无需新增索引
- 注册到 scheduler API，支持状态查看和手动触发

## Capabilities

### New Capabilities

- `log-cleanup`: 统一的日志表数据保留与清理调度能力

### Modified Capabilities

- `otel-business-tracing`: 移除 exporter 内嵌的清理逻辑，改为由 scheduler 驱动

## Impact

- **代码**: `jobs/` 新增文件；`tracing/exporter.go` 移除 cleanup 相关代码；`runtime.go`、`handler.go`、`runtimeinfo/schedulers.go` 注册新 scheduler
- **数据库**: `ai_call_logs` 新增 `created_at` 索引
- **API**: `/api/schedulers` 列表新增 `log_cleanup` 条目
- **运维**: 清理行为可通过 API 监控和手动触发，比隐藏在 goroutine 里更可观测
