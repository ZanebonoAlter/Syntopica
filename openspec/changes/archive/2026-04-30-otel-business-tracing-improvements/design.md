## Context

当前项目的 OpenTelemetry 使用状态：
- **SDK 已就绪**：`TracerProvider` 通过 SQLite span exporter 初始化，W3C TraceContext 传播已配置，所有 HTTP 请求经由 Gin `otelgin` 中间件自动创建 span
- **手动 span 稀疏**：6 个 domain span（`FeedService.RefreshFeed`、`ContentCompletionService.CompleteArticle` 等）+ 8 个 scheduler wrapper span（`tracing.TraceSchedulerTick`）
- **context 断层严重**：调度器用 `_ = ctx` 丢弃 trace context；`runCleanupCycle`、`processJob`、`TagArticle` 等关键函数无 `ctx` 参数；LLM 调用处用 `context.Background()` 创建孤立 span
- **AICallLog 独立运作**：`ai_call_logs` 表已记录 operation、capability、latency、success，但与 OTel trace 无关联

## Goals / Non-Goals

**Goals:**
- 每个 `Router.Chat` / `Router.Embed` span 自动携带 `ai.operation`、`ai.capability` 属性，在 trace viewer 中可区分 LLM 调用类型
- `ai_call_logs` 表新增 `trace_id` 字段，每次写入自动关联当前 trace
- 打通 `runCleanupCycle` → LLM 调用 和 `processJob` → LLM 调用 两条关键链路的 context 传递，使 span 形成父子拓扑
- 两条主链路入口创建 `workflow.*` 父 span，并通过 baggage 透传业务上下文到所有子 LLM 调用

**Non-Goals:**
- 不引入新的 OTel 依赖包（无需 otelgorm、Jaeger/OTLP exporter）
- 不对所有函数签名做 ctx 改造（只改关键链路上的 ~10 个函数）
- 不改变 LLM 调用行为、API 响应格式、数据库表（仅新增列）
- 不新增前端的 trace 可视化（复用已有 `/api/traces/*` 端点）

## Decisions

### D1: 分三阶段实施

| 阶段 | 目的 | 代码量 | 函数签名变更 |
|------|------|--------|-------------|
| Phase 1 | Span 属性注入 + AICallLog 关联 | ~15 行 | 0 |
| Phase 2 | 关键链路 ctx 传递 | ~40 行 | ~10 个函数 |
| Phase 3 | 父 span + baggage | ~20 行 | 0（依赖 Phase 2 已补 ctx） |

**理由**：Phase 1 可独立交付（LLM span 立即可区分），Phase 2/3 需要改函数签名，风险稍高但提供完整 trace 拓扑。

### D2: Span 属性来源选型

**选择**：从 `ChatRequest.Metadata` 读取 `operation` + 从 `ChatRequest.Capability` 读取 `capability`

**替代方案**：
- 用 OTel Baggage 透传 → 需要 Phase 2 ctx 传递就绪后才能生效，不能独立交付 Phase 1
- 在 LLM 调用处手动 `span.SetAttributes()` → 每处调用都要改，维护成本高

**理由**：Metadata 是现有的、每次调用都会携带的数据，Router.Chat 是唯一入口，在此集中处理零侵入。

### D3: Baggage key 命名规范

使用 `workflow.` 前缀与业务无关的 OTel 属性区分：
- `workflow.name` — 业务流程名称（如 `hierarchy_cleanup`、`article_tagging`）
- `workflow.trigger` — 触发方式（`scheduled`、`article_created`）
- `workflow.domain` — 业务域（`tag_management`、`content_processing`）

在 span 上映射为 `attribute`，在日志中作为 `baggage.*` 前缀透出。

### D4: AICallLog trace_id 字段

在 `AICallLog` 模型新增 `TraceID string` 字段（NULLABLE），写入时从 `trace.SpanContextFromContext(ctx).TraceID().String()` 获取。向后兼容——旧记录的该字段为 NULL。

## Risks / Trade-offs

- **[风险] Phase 2 修改 `TagArticle` 公共函数签名可能影响未知调用方** → 通过 `grep` + `go build ./...` 确保所有调用方同步更新
- **[风险] context 传递过深可能导致超时扩散** → 不在入口 ctx 上设置 deadline，仅用于 trace 传播；LLM 调用内部仍有自己的超时控制
- **[取舍] 不对 goroutine 内的 LLM 调用做 trace 传播** → 如 `generateTagDescription()` 的 `go func()` 异步调用，暂不处理跨 goroutine 的 trace 连接，保持 Scope 可控

## Open Questions

- 是否需要为调度器 tick span 增加 `workflow` attribute（例如 `scheduler.tag_hierarchy_cleanup.cycle` 同时也设 `workflow.name=hierarchy_cleanup`）？建议在 Phase 3 一起做，保持一致性。
