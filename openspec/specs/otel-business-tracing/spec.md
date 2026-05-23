## Purpose

在 OpenTelemetry trace 中注入业务上下文（capability、operation），使 LLM 调用可按业务流程区分；将 AICallLog 与 trace 链路关联；为关键 tag-processing 流程建立端到端 span 父子拓扑。

## Requirements

### Requirement: LLM span carries business attributes
Every `Router.Chat` and `Router.Embed` span SHALL include the following attributes extracted from the request:
- `ai.capability`: the capability string from `ChatRequest.Capability` (e.g., `topic_tagging`, `article_completion`, `embedding`)
- `ai.operation`: the operation string from `ChatRequest.Metadata["operation"]` when present

#### Scenario: Article tagging LLM call creates tagged span
- **WHEN** `Router.Chat` is called with `Capability: "topic_tagging"` and `Metadata: {"operation": "tag_judgment"}`
- **THEN** the resulting span has attribute `ai.capability=topic_tagging` and `ai.operation=tag_judgment`

#### Scenario: Cleanup LLM call creates tagged span
- **WHEN** `Router.Chat` is called with `Capability: "topic_tagging"` and `Metadata: {"operation": "tag_flat_merge"}`
- **THEN** the resulting span has attribute `ai.capability=topic_tagging` and `ai.operation=tag_flat_merge`

#### Scenario: LLM call without operation metadata still works
- **WHEN** `Router.Chat` is called without an `operation` key in Metadata
- **THEN** the span still has `ai.capability` set and does not panic

### Requirement: AICallLog records trace context
The `AICallLog` model SHALL include a `TraceID` field of type `string` (NULLABLE), and when writing a log entry, the system SHALL populate it from the current span context's trace ID.

#### Scenario: Successful LLM call records trace_id
- **WHEN** `Router.Chat` successfully calls an LLM provider within an active trace
- **THEN** the `AICallLog` row written has `trace_id` set to the current span's trace ID (32-char hex string)

#### Scenario: Failed LLM call still records trace_id
- **WHEN** `Router.Chat` fails on all provider attempts within an active trace
- **THEN** the `AICallLog` row for each failed attempt has `trace_id` set

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
