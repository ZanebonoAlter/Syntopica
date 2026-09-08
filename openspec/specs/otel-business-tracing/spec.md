## Purpose

在 OpenTelemetry trace 中注入业务上下文（capability、operation），使 LLM 调用可按业务流程区分；将 AICallLog 与 trace 链路关联；为关键 tag-processing 流程建立端到端 span 父子拓扑。
## Requirements
### Requirement: LLM span carries business attributes

Every `Router.Chat` and `Router.Embed` span SHALL include the following business attributes, sourced from the request's first-class fields (**NOT** from `ChatRequest.Metadata`):

- `ai.capability`: from `ChatRequest.Capability` (Chat) / the `capability` parameter (Embed)
- `ai.operation`: from `ChatRequest.Operation` (mandatory, non-empty — enforced by the `ai-logging` capability's forced-Operation requirement)
- `ai.session_id`: from `ChatRequest.SessionID` (set when non-empty; omitted only for non-orchestrated single calls)

`Router.Embed` SHALL be brought to parity with `Router.Chat` — previously (`router.go:191`) Embed spans only carried `ai.capability` and lacked `ai.operation`.

airouter SHALL NOT source these attributes from `ChatRequest.Metadata["operation"]` anymore (deprecates the old `router.go:111-113` weak-read path). Callers transition to passing `Operation`/`SessionID` as first-class fields; this is exercised by the `ai-logging` capability's "现有 AI 调用补齐 Operation 与 SessionID" requirement.

> **为什么 MODIFY 而非 ADD**：现行 spec（`openspec/specs/otel-business-tracing/spec.md`）已要求 Chat span 带 `ai.operation`，只是来源是 `Metadata["operation"]` 且 Operation 非强制。本 delta 把来源改为一等字段、补 session_id、补 Embed parity、并把"漏填静默通过"升级为"airouter 强制拒绝"。属于同一 requirement 的收紧，不新增并列 requirement。

#### Scenario: Chat span carries first-class operation and session_id

- **WHEN** `Router.Chat` is called with `Operation: "daily_report.cluster_tags"` and `SessionID: "daily_report_42_ab12cd34"`
- **THEN** the resulting span has attributes `ai.operation=daily_report.cluster_tags` and `ai.session_id=daily_report_42_ab12cd34`, sourced from the fields, not from Metadata

#### Scenario: Embed span parity with Chat

- **WHEN** `Router.Embed` is called with `Operation: "section.embedding"` and `SessionID` set
- **THEN** the resulting span has `ai.operation` and `ai.session_id` set (previously Embed only carried `ai.capability`)

#### Scenario: Empty operation no longer silently produces untagged span

- **WHEN** `Router.Chat` is called with an empty `Operation`
- **THEN** the call is rejected by airouter (per the `ai-logging` forced-Operation requirement) and no span/LogCall is created — supersedes the prior "LLM call without operation metadata still works" scenario

### Requirement: AICallLog records trace context
The `AICallLog` model SHALL include a `TraceID` field of type `string` (NULLABLE), and when writing a log entry, the system SHALL populate it from the current span context's trace ID. The `AICallLog` model SHALL also have a btree index on `created_at` for efficient retention-based cleanup.

#### Scenario: Successful LLM call records trace_id
- **WHEN** `Router.Chat` successfully calls an LLM provider within an active trace
- **THEN** the `AICallLog` row written has `trace_id` set to the current span's trace ID (32-char hex string)

#### Scenario: Failed LLM call still records trace_id
- **WHEN** `Router.Chat` fails on all provider attempts within an active trace
- **THEN** the `AICallLog` row for each failed attempt has `trace_id` set

#### Scenario: created_at index supports cleanup
- **WHEN** a DELETE query filters on `ai_call_logs.created_at`
- **THEN** the database uses the btree index on `created_at` (no sequential scan)

### Requirement: Context propagated through key tag-processing call chains
The following function call chains SHALL pass `context.Context` from entry point to LLM invocation:
- `runCleanupCycle` → `ExecuteFlatMerge` → `callFlatMergeLLM` → `Router.Chat`
- `processJob` → `TagArticle` / `tagArticle` → `findOrCreateTag` → `callLLMForTagJudgment` → `Router.Chat`

#### Scenario: Hierarchy cleanup trace has connected span tree
- **WHEN** the `TagHierarchyCleanupScheduler` triggers a cleanup cycle
- **THEN** the `workflow.hierarchy_cleanup.cycle` span is the parent of all `Router.Chat` spans created during that cycle

#### Scenario: Article tagging trace has connected span tree
- **WHEN** a tag job is processed by the TagQueue worker
- **THEN** the `workflow.article_tagging` span is the parent of all `Router.Chat` spans created during that job

### Requirement: Workflow entry points create parent spans with baggage
The following workflow entry points SHALL create a parent span and set OTel baggage:
- `runCleanupCycle`: span name `workflow.hierarchy_cleanup.cycle`, baggage `workflow.name=hierarchy_cleanup`, `workflow.domain=tag_management`
- TagJob worker (`processJob`): span name `workflow.article_tagging`, baggage `workflow.name=article_tagging`, `workflow.domain=tag_management`

The `Router.Chat` method SHALL propagate baggage values from the current context into span attributes with the `baggage.` prefix.

#### Scenario: Baggage propagated to LLM spans
- **WHEN** a hierarchy cleanup cycle runs with baggage `workflow.name=hierarchy_cleanup`
- **THEN** each `Router.Chat` span within that cycle has attribute `baggage.workflow.name=hierarchy_cleanup`

#### Scenario: Separate workflows produce distinct trace trees
- **WHEN** a hierarchy cleanup runs concurrently with an article tagging job
- **THEN** their respective `Router.Chat` spans are grouped under different parent spans (different trace IDs or parent span IDs)

### Requirement: Session-level aggregation endpoint

系统 SHALL 暴露 `GET /api/ai/sessions/:session_id`，按 session_id 一次性返回该编排的业务调用日志与链路时间线。

实现 SHALL 通过 trace_id 反查聚合（非 `otel_spans.attributes` 的 jsonb 查询）：

1. 先 `ai_call_logs WHERE session_id=?` 得调用记录与 trace_id 集合
2. 再 `otel_spans WHERE trace_id IN (...)` 得完整链路
3. 用 `BuildSpanTree` 组装 timeline 树

#### Scenario: 按编排 session 返回全貌

- **WHEN** `GET /api/ai/sessions/daily_report_42_ab12cd34`
- **THEN** 响应 SHALL 包含该 session 下全部 AICallLog（`call_logs`，按 created_at 正序）+ 通过 trace_id 关联的全部 otel_spans 组装的 `timeline`（span 树）

#### Scenario: 聚合 token 统计

- **WHEN** 响应返回
- **THEN** `summary.total_tokens` SHALL 为该 session 下所有 call_logs 的 token_usage 之和（prompt/completion/total 分别聚合）

#### Scenario: session 不存在

- **WHEN** `GET /api/ai/sessions/<不存在的id>`
- **THEN** SHALL 返回 `success=true` 但 `call_logs` 与 `timeline` 均为空数组（不报 404，便于前端空态处理）

### Requirement: Daily report orchestration carries business spans

日报管线的编排步骤 SHALL 创建 `workflow.daily_report.*` 业务 span，与 `Router.Chat`/`Router.Embed` span 形成父子拓扑，使一次日报生成的 trace 能重建编排步骤顺序。

span 名 SHALL 与 AICallLog.operation 对齐（`workflow.` 前缀 + operation 值；本地无 LLM 步骤用语义名）：

- LLM/embedding 节点：`workflow.daily_report.cluster_tags` / `.highlights` / `.cluster_threads` / `.merge_arbitration` / `.section_embedding`
- 本地节点（无 LLM，记耗时/拓扑）：`workflow.daily_report.dedup` / `.thread_fit`
- 编排入口 root span：`workflow.daily_report.generate`

**Step5 并发约束**：`GenerateHighlights` + K 个 `GenerateClusterThreads` 在 goroutine 并发，parent span context SHALL 在 goroutine 启动前取好再传入，确保并发 span 共享正确 parent。

#### Scenario: 日报 trace 含编排步骤 span

- **WHEN** 一次日报生成完成
- **THEN** 其 trace 的 timeline SHALL 包含 `workflow.daily_report.*` span 作为 `Router.Chat`/`Router.Embed` span 的父节点（或兄弟编排节点），非仅有最外层 HTTP span + LLM span

### Requirement: Database operations are auto-traced

系统 SHALL 通过自写 GORM trace 插件（`internal/platform/tracing/gorm_plugin.go` 的 `NewGORMPlugin()`，实现 GORM `Plugin` 接口 + callback）在 db 初始化处挂载，使所有 GORM 数据库操作自动产生 `SpanKind=Client` 的 span，并携带 `db.system`、`db.operation`、`db.statement` 等标准 DB semantic attributes。该 span SHALL 挂在当前 trace 的父 span 下（当存在活跃 trace 时），不要求业务代码做任何改动。

> 注：自写而非用 `gorm.io/plugin/opentelemetry`，因其最新版强制 gorm v1.30 升级（超 change 范围），见 design 决策 1。

当 `InstrumentGORM` 配置为 `false` 时，系统 SHALL 不挂载该插件（退化为现状，仅 SlowLogger）。

#### Scenario: 请求链路含 GORM 子 span

- **WHEN** 一个带活跃 trace 的 HTTP 请求执行若干 GORM 查询
- **THEN** 该 trace 的 span 树 SHALL 包含对应数量的 `SpanKind=Client` DB span，作为 HTTP / 业务 span 的子节点

#### Scenario: DB span 携带语句与操作类型

- **WHEN** 一条 `SELECT * FROM feeds WHERE id = ?` 执行
- **THEN** 对应 DB span 的 attributes SHALL 包含 `db.operation = SELECT` 与 `db.statement`（参数占位符化）

#### Scenario: 可按配置关闭

- **WHEN** `InstrumentGORM = false`
- **THEN** db 初始化 SHALL 不调用 `db.Use(tracing.NewGORMPlugin())`，GORM 操作不产生 span

### Requirement: Outbound HTTP calls are auto-traced

系统 SHALL 提供统一的出站 HTTP 客户端构造入口（`internal/platform/httpclient`），其产出的 `*http.Client` SHALL 通过 `otelhttp.NewTransport` 包装 transport，使所有经该 client 发出的出站请求自动产生 `SpanKind=Client` span，并透传 `traceparent` / `tracestate`（W3C Trace Context）到下游服务。

`airouter`（LLM / Embedding 调用）、`dataenrichment`、`admin` 等现有裸 `&http.Client{}` 调用点 SHALL 改为使用该统一入口。改动 SHALL 限定在客户端构造层，不进入 domain 业务逻辑。

当 `InstrumentHTTP = false` 时，工厂 SHALL 返回未包装 otelhttp 的普通 client（退化为现状行为）。

#### Scenario: LLM 调用出现在 trace

- **WHEN** `airouter` 经统一 client 发起对 LLM provider 的 HTTP 调用（在活跃 trace 内）
- **THEN** 该 trace 的 span 树 SHALL 包含一个 `SpanKind=Client` 的出站 HTTP span，作为调用方 span 的子节点

#### Scenario: traceparent 透传到下游

- **WHEN** 经统一 client 发起出站请求时存在活跃 trace
- **THEN** 请求 header SHALL 携带 `traceparent`，使下游服务（若接入 trace）能延续同一 trace

#### Scenario: 保留各调用点自定义

- **WHEN** 某调用点原本设置 `Timeout: 120s`
- **THEN** 改用统一 client 后 SHALL 仍能设置同等 Timeout（通过 functional options）

#### Scenario: 可按配置关闭

- **WHEN** `InstrumentHTTP = false`
- **THEN** 工厂产出的 client transport SHALL 不含 otelhttp 包装

### Requirement: Sampling strategy and trace configuration externalization

系统 SHALL 在 `TracerProvider` 初始化时配置 `ParentBased(TraceIDRatioBased(ratio))` 采样器：root span（无父 span，如 HTTP 入站、scheduler tick）按 `ratio` 决定是否采样；一旦 root span 被采样，其下所有子 span（DB / 出站 HTTP / 业务）SHALL 全部被采样，保证整条链路完整、不在中间断链。

`SampleRatio`、`InstrumentGORM`、`InstrumentHTTP` SHALL 可从环境变量（`TRACE_SAMPLE_RATIO` 等）读取。未配置任何环境变量时，系统 SHALL 默认 `SampleRatio = 0.05`（低采样，详见「Tracing sample ratio defaults low and is configurable」）、两个 instrumentation 开关为 `true`。

#### Scenario: 全采下链路完整

- **WHEN** `SampleRatio = 1.0`，一个 HTTP 请求被处理（含 DB 与出站 HTTP 调用）
- **THEN** 该请求的整条 trace（HTTP root + 业务 span + DB span + 出站 HTTP span）SHALL 全部记录到 `otel_spans`

#### Scenario: 降采样仍保持链路完整

- **WHEN** `SampleRatio = 0.5`，某 HTTP 请求的 root span 被采样命中
- **THEN** 该 root 下的所有子 span SHALL 全部记录（不在子节点层做二次丢弃）

#### Scenario: 不配环境变量默认低采样

- **WHEN** 未设置任何 trace 相关环境变量
- **THEN** 系统 SHALL 以 `SampleRatio = 0.05`、`InstrumentGORM = true`、`InstrumentHTTP = true` 运行；恢复全采需显式设置 `TRACE_SAMPLE_RATIO=1.0`

### Requirement: Tracing sample ratio defaults low and is configurable

The tracing subsystem SHALL default its span sample ratio to 0.05 (5%) instead of 1.0 (full sampling). Full sampling at ~820k spans/day keeps `otel_spans` at a steady 4.3GB under the 7-day retention and generates constant dead-tuple churn; 5% covers routine debugging while cutting daily span volume ~95%.

The sample ratio SHALL be overridable by environment variable (`TRACE_SAMPLE_RATIO`, accepted range 0.0–1.0) without code changes, so an incident session can temporarily restore a higher ratio. Invalid values (unparseable, out of range) SHALL fall back to the default 0.05 with a warn log.

Sampling SHALL use parent-based semantics: spans that inherit an already-sampled trace context are always recorded, so a sampled root request keeps its full downstream tree (including GORM/HTTP child spans within it).

#### Scenario: Default sampling is low

- **WHEN** the backend starts with no tracing-related environment variables
- **THEN** the effective sample ratio is 0.05 and otel_spans daily insert volume drops by roughly 95% versus full sampling

#### Scenario: Environment override raises sampling

- **WHEN** the backend starts with `TRACE_SAMPLE_RATIO=1.0`
- **THEN** the effective sample ratio is 1.0 (full sampling) without any code change

#### Scenario: Invalid override falls back safely

- **WHEN** the backend starts with `TRACE_SAMPLE_RATIO=abc` or `=2.5`
- **THEN** the backend starts successfully with the default 0.05 ratio and logs a warn about the invalid value

#### Scenario: Sampled root keeps its span tree

- **WHEN** a root span is selected by the sampler and it triggers child spans (LLM call, SQL query)
- **THEN** all descendant spans of the sampled root are recorded and linked in one trace, regardless of the 0.05 ratio

### Requirement: Service-layer methods are auto-instrumented via build-time AST weaving

系统 SHALL 提供一个 AST 织入器（复用 `go-instrument` 或自写 `go/ast` 工具，apply 阶段选型定），在构建前（`go generate` 或 Makefile target）自动扫描 `internal/*/service/**/*.go` 与 `internal/platform/airouter/*.go` 的 exported 方法，为每个首参为 `ctx context.Context` 的方法注入 OpenTelemetry span（span 名 `TypeName.MethodName`），不要求开发者手写埋点代码。

织入 SHALL 幂等：方法体已含 `otel.Tracer` / `tracer.Start` / `tracing.Tracer` 调用的，SHALL 跳过（避免与现有 go-instrument 注入方法、手写业务 span 重复注入）。

当方法返回值含 named `err error` 时，织入 SHALL 额外注入错误记录（`span.RecordError(err)` + `span.SetStatus(Error)`）。

#### Scenario: 新 service 方法自动获得 span

- **WHEN** 开发者在 `internal/<x>/service/` 下新增 exported 方法 `FooBar(ctx context.Context, ...) error`，并运行 `go generate`
- **THEN** 该方法 SHALL 被注入 span（名 `<TypeName>.FooBar`），含 named err 错误记录，且 `otel_spans` 在调用时记录该 span

#### Scenario: 幂等不重复注入

- **WHEN** 织入器对同一文件连续运行两次
- **THEN** 第二次 SHALL 不产生重复 span 代码（检测到已有 `otel.Tracer` / `tracer.Start` 即跳过）

#### Scenario: 与手写业务 span 共存不冲突

- **WHEN** 某方法已手写 `tracing.Tracer(...).Start(...)`（如 `workflow.daily_report.generate`）
- **THEN** 织入器 SHALL 跳过该方法，不注入第二个 span

#### Scenario: 无 ctx 首参的方法被跳过

- **WHEN** 某 exported 方法首参不是 `context.Context`
- **THEN** 织入器 SHALL 跳过该方法（不注入 span），避免无 ctx 传递的无效 span

#### Scenario: 构建集成

- **WHEN** CI / 本地构建执行 `go generate ./...`（或 Makefile instrument target）
- **THEN** 所有目标 service 包 SHALL 被织入器扫描并更新，构建产物含最新 span 埋点

