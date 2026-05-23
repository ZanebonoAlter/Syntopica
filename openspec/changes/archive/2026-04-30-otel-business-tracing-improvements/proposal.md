## Why

项目已有 OpenTelemetry SDK（Gin 自动埋点、SQLite span 导出器、trace 查询 API），但业务代码中手动 span 覆盖不全，且最关键的问题——无法在 trace 中区分 LLM 调用来自哪个业务流程（标签清理 vs 新标签创建）——没有解决。同时 AICallLog 表已记录了每次 LLM 调用的 operation 元数据，但与 OTel trace 完全隔离。

## What Changes

- **Router.Chat / Router.Embed span 注入业务属性**：从 request metadata 读取 `operation`、`capability` 等写入 span attribute，让每个 LLM 调用在 trace viewer 中可区分
- **AICallLog 关联 trace context**：新增 `trace_id` 字段，打通数据库日志和 OTel 链路
- **关键流程入口补 ctx 参数**：修复 `runCleanupCycle`、`processJob`、`TagArticle` 等函数缺失 context 的问题，让 span 形成父子拓扑
- **流程入口设 baggage + 父 span**：在 `runCleanupCycle`、TagJob worker 等入口创建 `workflow.*` 父 span，并以 baggage 透传业务上下文

## Capabilities

### New Capabilities
- `otel-business-tracing`: Router.Chat/Embed span 自动携带业务属性（operation, capability），AICallLog 关联 trace_id，关键业务流程（hierarchy_cleanup, article_tagging, content_completion）支持端到端 trace 链路

## Impact

- **后端代码**：`internal/platform/airouter/router.go`（span 属性注入）、`internal/platform/airouter/store.go`（AICallLog trace_id）、`internal/domain/models/ai_models.go`（新增字段）、`internal/jobs/tag_hierarchy_cleanup.go`（ctx 传递）、`internal/domain/topicextraction/tag_queue.go`（ctx 传递）、`internal/domain/topicextraction/article_tagger.go`（ctx 参数）、`internal/domain/topicanalysis/` 若干函数签名（ctx 参数）
- **数据库**：`ai_call_logs` 表新增 `trace_id` 和 `span_id` 列（向后兼容，非必填）
- **无明显影响**：不改变业务逻辑，不改变 API 响应格式，不改变 LLM 调用行为
