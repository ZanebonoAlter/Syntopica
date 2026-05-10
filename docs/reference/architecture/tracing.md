# 业务链路追踪实现说明

本文档描述当前仓库里已经落地的 tracing 方案，重点说明第一版 `go-instrument` 接入后的真实状态，避免把规划内容当成现状。

## 当前结论

- 后端已经接入 OpenTelemetry，Span 会落到 PostgreSQL 的 `otel_spans` 表
- HTTP 请求入口已接入 `otelgin`，调度器入口已接入手动 root span
- 第一版 `go-instrument` 已经把一批 exported 方法改成自动创建 span
- 业务语义型 attributes / events 目前只在少量手动 span 中补充，自动注入的方法大多还是基础 span
- 前端 `client.ts` 已具备 `traceparent` 透传和捕获逻辑，但后端代码里没有额外手动回写响应头

## 技术选型

| 组件 | 当前实现 | 说明 |
|------|----------|------|
| Trace SDK | `go.opentelemetry.io/otel` / `go.opentelemetry.io/otel/sdk` | OpenTelemetry 官方 Go SDK |
| HTTP 中间件 | `otelgin` | 为 Gin 请求自动创建 `SERVER` span |
| 自动注入 | `go-instrument` 生成后的代码已提交到仓库 | 当前不是运行时 agent，而是直接把生成代码写回 `.go` 文件 |
| Context 传播 | W3C Trace Context | 前端请求会带 `traceparent`，后端全局注册 `TraceContext + Baggage` propagator |
| Exporter | `SQLiteSpanExporter`（名称保留历史，实际写入 PostgreSQL） | 自定义 exporter，通过 GORM 写入 PostgreSQL |
| 调试输出 | `stdouttrace` | `Config.Debug=true` 时额外输出到控制台 |

## 初始化与入口

### 后端初始化

服务启动时会在 `backend-go/cmd/server/main.go` 中完成 tracing 初始化：

1. 调用 `tracing.InitTracerProvider(database.DB, tracing.DefaultConfig())`
2. 注册全局 `TracerProvider`
3. 注册全局 propagator：`TraceContext + Baggage`
4. 给 Gin 挂载 `otelgin.Middleware("rss-reader-backend")`

当前资源属性固定为：

- `service.name = rss-reader-backend`
- `service.version = 1.0.0`

### 前端请求链路

`front/app/api/client.ts` 当前逻辑是：

- 如果内存里已有 `currentTraceId`，请求时自动带上 `traceparent`
- 收到响应后尝试从响应头读取 `traceparent`
- 读取成功后把 `trace_id` 存回 `currentTraceId`
- 开发时会 `console.debug('[trace]', traceId)`

注意：后端代码里目前没有手动 `Set("traceparent")` 的逻辑，所以前端是否能持续复用同一条 trace，取决于实际响应头是否由中间件链路带回。

## 当前埋点分层

### 1. HTTP 请求

Gin 全局中间件已经覆盖 HTTP 请求入口，根 span 由 `otelgin` 自动创建。

典型名称类似：

- `GET /api/traces/recent`
- `POST /api/feeds/:id/refresh`

### 2. `go-instrument` 自动注入的方法

当前仓库中已经能看到注入后的代码，表现为方法体开头直接出现：

```go
ctx, span := otel.Tracer("rss-reader-backend").Start(ctx, "FeedService.RefreshFeed")
defer span.End()
```

目前已落地的自动注入方法：

| 文件 | 方法 | 当前效果 |
|------|------|----------|
| `backend-go/internal/domain/feeds/service.go` | `FeedService.RefreshFeed` | 自动创建 span，自动记录 named error |
| `backend-go/internal/domain/contentprocessing/firecrawl_service.go` | `FirecrawlService.ScrapePage` | 自动创建 span，自动记录 named error |
| `backend-go/internal/domain/contentprocessing/content_completion_service.go` | `ContentCompletionService.CompleteArticle` | 自动创建 span，只有薄包装，无自动错误记录 |
| `backend-go/internal/domain/contentprocessing/content_completion_service.go` | `ContentCompletionService.CompleteArticleWithForce` | 自动创建 span，只有薄包装，无自动错误记录 |
| `backend-go/internal/domain/contentprocessing/content_completion_service.go` | `ContentCompletionService.CompleteArticleWithMetadata` | 自动创建 span，自动记录 named error |
| `backend-go/internal/platform/airouter/router.go` | `Router.Chat` | 自动创建 span，自动记录 named error |

### 3. 手动 root span：Scheduler

`backend-go/internal/platform/tracing/scheduler.go` 提供了统一封装：

```go
tracing.TraceSchedulerTick("auto_refresh", "cron", func(ctx context.Context) {
    // scheduler body
})
```

当前已接入的 scheduler 入口共 7 个：

- `auto_refresh`
- `firecrawl`
- `content_completion`
- `preference_update`
- `narrative_summary`

这类 span 的特点：

- 使用 tracer 名称 `scheduler`
- span 名称形如 `scheduler.auto_refresh.cycle`
- 使用 `trace.WithNewRoot()`，每次 tick 都是新 trace
- 自动附带 `scheduler.name`、`scheduler.trigger`

### 4. 手动业务 span

目前真正补充了业务事件的主要是摘要队列：

- `SummaryQueue.processBatch`
- `SummaryQueue.processJob`

这两处使用 `tracing.Tracer("summaries")` 手动创建 span，并补充了：

- `input` / `output` events
- `batch.id`、`job.feed_id`、`job.status` 等 attributes
- 成功 / 失败状态

### 5. 异步 trace helper 的实际状态

`backend-go/internal/platform/tracing/helpers.go` 中已经提供：

- `GoWithTrace`
- `TraceAsyncOp`

当前实现并不是把 goroutine 继续挂在原 span 下面，而是：

- 新起一个 `WithNewRoot()` 的异步 root span
- 用 attribute `parent_trace_id` 记录来源 trace
- 再附带 `async.operation`

目前仓库里还没有实际业务代码调用 `GoWithTrace` / `TraceAsyncOp`，所以这部分属于“工具已就位，业务尚未接入”。

## `go-instrument` 第一版的真实约束

当前版本的注入效果，从仓库现状看有几个明确约束：

### 方法签名要求

- `ctx context.Context` 需要作为第一个参数
- 想自动记录错误，返回值里需要有 named `err error`
- 当前主要覆盖 exported 方法

### 当前生成代码特征

- 会直接把 span 创建代码写进源文件
- 会插入 `/*line ...*/` 注释，尽量保持调试定位
- 对 named error 方法会自动生成 `defer` 做 `SetStatus` + `RecordError`
- 对普通代理方法只会加基础 span，不会自动补错误状态

### 还没自动完成的部分

`go-instrument` 现在只解决“有 span”这件事，还没有统一补齐：

- `SpanKind` 细分
- 业务 attributes
- input / output events
- 外部调用专用字段（例如 URL、provider、capability）

所以当前链路数据已经能看，但业务语义还不够丰满。

## 数据落库模型

所有 span 目前写入 PostgreSQL 表 `otel_spans`（通过 GORM）。

核心字段如下：

| 字段 | 说明 |
|------|------|
| `trace_id` | 32 位十六进制 trace ID |
| `span_id` | 16 位十六进制 span ID |
| `parent_span_id` | 父 span；根 span 可能为空或全零 |
| `name` | span 名称 |
| `kind` | OpenTelemetry span kind 数值 |
| `status_code` | `0=UNSET`，`1=ERROR`，`2=OK` |
| `start_time_unix_nano` / `end_time_unix_nano` | 起止时间 |
| `duration_ms` | 毫秒耗时 |
| `service_name` / `service_version` | 资源信息 |
| `resource_attributes` | Resource attributes 的 JSON |
| `scope_name` / `scope_version` | Instrumentation scope 信息 |
| `attributes` | span attributes 的 JSON |
| `events` | span events 的 JSON |
| `links` | span links 的 JSON |
| `created_at` | 入库时间 |

当前索引：

- `idx_otel_spans_trace_id`
- `idx_otel_spans_name`
- `idx_otel_spans_start_time`
- `idx_otel_spans_kind`
- `idx_otel_spans_status`

### JSON 序列化说明

当前实现里：

- attributes / events / links 都是序列化后存成 `TEXT`
- 空 events / links 存 `[]`
- 空 attributes 当前 helper 返回的是 `{}`，但有值时实际写入的是数组 JSON

也就是说，`attributes` 字段的空值和非空值格式目前并不完全对称，文档和消费端都需要按当前实现理解。

## 数据保留策略

默认配置来自 `tracing.DefaultConfig()`：

- `Enabled = true`
- `TableName = otel_spans`
- `RetentionDays = 7`
- `BufferSize = 100`
- `FlushInterval = 5`
- `Debug = false`

其中当前真正生效的主要是：

- `RetentionDays`
- `BufferSize`
- `Debug`

清理策略由 `SQLiteSpanExporter.cleanupLoop()` 负责（名称保留历史，实际通过 GORM 操作 PostgreSQL）：

- 进程启动后起一个后台 goroutine
- 每 24 小时执行一次清理
- 删除 `created_at < now - RetentionDays` 的记录

## 查询 API

当前后端已经注册在 `backend-go/internal/app/router.go`：

### `GET /api/traces?trace_id={trace_id}`

按 `trace_id` 查询完整 span 列表，按开始时间升序返回。

响应结构：

```json
{
  "success": true,
  "data": {
    "trace_id": "...",
    "spans": []
  }
}
```

### `GET /api/traces/recent?limit=50`

查询最近 trace 摘要。

### `GET /api/traces/search?operation=...&status=error&min_duration_ms=...&limit=50`

当前搜索逻辑不是组合过滤，而是按优先级单选：

1. 如果 `status=error`，走错误 trace 查询
2. 否则如果有 `operation`，按 root span 名称模糊查询
3. 否则如果有 `min_duration_ms`，查慢 trace
4. 否则退化为 recent

也就是说，当前并不支持文档式的多条件组合筛选，也没有 `since` / `until` 时间窗查询。

### `GET /api/traces/stats`

返回聚合统计，包括：

- `total_traces`
- `total_spans`
- `error_traces`
- `success_rate`
- `p50_ms` / `p95_ms` / `p99_ms`
- `top_operations`
- `last_24h_traces`

### `GET /api/traces/:trace_id/timeline`

返回按父子关系构建的 span tree，用于时间线 / 甘特图场景。

### `GET /api/traces/:trace_id/otlp`

把某条 trace 转成近似 OTLP JSON 的结构导出，方便后续外部分析平台消费。

## 代码结构

当前 tracing 基础设施集中在 `backend-go/internal/platform/tracing/`：

| 文件 | 作用 |
|------|------|
| `config.go` | tracing 默认配置 |
| `model.go` | `otel_spans` 表结构与 JSON 序列化辅助 |
| `exporter.go` | `SQLiteSpanExporter`（名称保留历史，实际写入 PostgreSQL）、入库、过期清理 |
| `provider.go` | 初始化全局 `TracerProvider` 与 propagator |
| `helpers.go` | `Tracer`、`StartSpan`、`GoWithTrace` 等工具 |
| `scheduler.go` | scheduler / async 的包装入口 |
| `query.go` | trace 查询、统计、树结构构建 |
| `handler.go` | `/api/traces` HTTP handler |

## 当前问题与下一步建议

从仓库现状看，这一版 tracing 已经能用于排查，但还属于“基础链路通了，业务语义待补”的阶段：

1. `go-instrument` 注入的方法多数还没补 attributes / events
2. 外部调用还没有系统化区分 `CLIENT` span
3. 前端虽然支持 `traceparent`，但后端没有显式回写响应头
4. 异步 helper 已经存在，但暂无真实业务调用
5. 查询 API 已可用，但筛选能力仍偏基础

如果继续往下推进，优先级建议是：

1. 给 `FirecrawlService.ScrapePage`、`Router.Chat`、`FeedService.RefreshFeed` 补关键 attributes / events
2. 明确哪些外部调用要手动补 `SpanKind=CLIENT`
3. 再决定是否把 `go-instrument` 接入 `go generate` 或脚本化流程
