## Context

两张日志表持续写入：
- `ai_call_logs`: 每次 AI 调用写一条，embedding 占 87%，~14k 行/天
- `otel_spans`: 每个 OTel span 写一条，~20k 行/天

当前 `otel_spans` 的清理藏在 `SQLiteSpanExporter.cleanupLoop`（独立 goroutine，24h ticker，7 天保留），`ai_call_logs` 没有任何清理。项目已有成熟的 scheduler 系统（6 个定时任务），统一纳入即可。

## Goals / Non-Goals

**Goals:**
- 统一日志表清理到一个 scheduler，24h 执行一次，保留 7 天
- `ai_call_logs` 添加 `created_at` 索引避免 DELETE 全表扫描
- 可通过 `/api/schedulers/log_cleanup` 查看状态和手动触发
- 移除 `SQLiteSpanExporter` 内嵌的 cleanup goroutine，职责收敛

**Non-Goals:**
- 不做分区表改造（当前规模不需要）
- 不做采样/降级写入（日志量还在可控范围）
- 不改 otel_spans 的写入逻辑

## Decisions

### D1: 新建独立 scheduler 而非复用现有

**选择**: 新建 `LogCleanupScheduler`，遵循 `BlockedArticleRecoveryScheduler` 的模式（最简单的现有实现）。

**理由**: 清理两张表是独立职责，不应混入 tracing 或 airouter 模块。scheduler 系统已提供统一的生命周期管理（Start/Stop/TriggerNow/GetStatus）。

**替代方案**: 在 `SQLiteSpanExporter` 里加 `ai_call_logs` 清理 → 跨模块耦合，不合理。

### D2: 执行间隔 24 小时

**选择**: 24h ticker，启动后首次延迟 5 分钟执行。

**理由**: 日志表不需要精确到小时的清理，24h 足够。延迟启动避免启动时 IO 峰值。

### D3: 分表顺序清理

**选择**: 先清理 `ai_call_logs`，再清理 `otel_spans`，串行执行。

**理由**: 数据量小（每天几万行），DELETE 毫秒级完成，无需并行。串行更简单，日志顺序可读。

### D4: 保留天数硬编码 7 天

**选择**: 常量 `retentionDays = 7`，不暴露配置。

**理由**: 单用户系统，没有不同保留策略的需求。和 `tracing.Config.RetentionDays` 保持一致。

### D5: otel_spans 使用 start_time_unix_nano 而非 created_at 做清理条件

**选择**: `otel_spans` 清理条件用 `WHERE start_time_unix_nano < cutoff_nano`，而非 `WHERE created_at < cutoff`。

**理由**: `otel_spans` 已有 `idx_otel_spans_start_time` 索引（`model.go:49`），无需新增 `created_at` 索引。语义上也更精确——"7 天前发生的 span"比"7 天前入库的 span"更合理。两者时间差可忽略（exporter 批量写入延迟通常 < 1 秒）。

**替代方案**: 给 `otel_spans.created_at` 也加索引 → 多一个索引维护成本，且与已有的 `start_time_unix_nano` 索引功能重叠。

## Risks / Trade-offs

- **[DELETE 锁表]** → 单次删除量 ~14k-20k 行，PostgreSQL 处理很快，风险低。如果未来量级增大可改用批量删除（每次 1000 行循环）。
- **[遗漏清理]** → 移除 `cleanupLoop` 后如果新 scheduler 未正确启动，otel_spans 会重新膨胀。通过启动日志和 API 状态可观测。
- **[索引迁移]** → `ai_call_logs` 新增 `created_at` 索引是 ONLINE 操作（PostgreSQL），不影响写入。`otel_spans` 复用已有 `start_time_unix_nano` 索引，无迁移。
