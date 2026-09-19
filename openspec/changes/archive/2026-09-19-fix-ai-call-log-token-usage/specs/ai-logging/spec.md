# ai-logging Delta

## MODIFIED Requirements

### Requirement: AICallLog 完整记录字段

`ai_call_logs` 表 SHALL 包含以下字段以满足 [`ai-logging.md`](../../../../../docs/reference/standard/backend/ai-logging.md) R2：`id`、`created_at`、`operation`（业务操作名）、`capability`、`route_name`、`provider_name`、`model`、`success`、`is_fallback`、`latency_ms`、`error_code`、`error_message`、`prompt`（完整 messages 文本）、`response_snippet`、`token_usage`（prompt/completion/total 的 jsonb）、`trace_id`、`session_id`（编排分组键）、`request_meta`。

`operation` SHALL NOT NULL。`prompt`、`token_usage`、`session_id` SHALL nullable（兼容历史数据与非编排的单次调用）。表 SHALL 在 `(session_id)` 上建索引、在 `(operation, created_at)` 上建复合索引。

建表与加列 SHALL 通过显式迁移完成（开发执行规范 §10），不依赖 gorm AutoMigrate。

chat 调用成功且 provider 响应携带 `usage` 块时，系统 SHALL 按 OpenAI 兼容接口标准键（`prompt_tokens`、`completion_tokens`、`total_tokens`）解析用量，并将 `ai_call_logs.token_usage` 落库为 `{"prompt":N,"completion":N,"total":N}` 的 jsonb。存储 jsonb 形状 SHALL 保持不变（消费方如 session 聚合按此形状读取）。provider 响应不携带 `usage` 块或调用失败时，`token_usage` SHALL 为 NULL，SHALL NOT 落库全零值。

#### Scenario: 完整 prompt 落库

- **WHEN** airouter 执行一次 `Chat` 调用成功
- **THEN** 写入的 `ai_call_logs.prompt` SHALL 包含本次 system+user messages 的完整文本（超 20000 runes 时截断并标注 `[truncated]`），不得为空字符串或摘要

#### Scenario: token 用量记录

- **WHEN** chat 调用成功且 provider 响应包含 `usage` 块（键为 `prompt_tokens`/`completion_tokens`/`total_tokens`）
- **THEN** `ai_call_logs.token_usage` SHALL 为 `{"prompt":N,"completion":N,"total":N}` 且 `total` 大于 0，与响应 `usage` 块数值一致

#### Scenario: 无 usage 块不落全零

- **WHEN** chat 调用成功但 provider 响应不包含 `usage` 块，或调用失败
- **THEN** `ai_call_logs.token_usage` SHALL 为 NULL，SHALL NOT 写入 `{"prompt":0,"completion":0,"total":0}`

#### Scenario: 历史行回填

- **WHEN** 迁移执行后，历史已存在的 `ai_call_logs` 行
- **THEN** 其 `operation` SHALL 为 `unknown`（回填），`prompt`/`token_usage`/`session_id` SHALL 为 NULL，不报错

#### Scenario: operation 列约束

- **WHEN** 尝试插入 `operation=NULL` 或空字符串
- **THEN** 系统 SHALL 因 NOT NULL 约束拒绝（回填完成后）

## ADDED Requirements

### Requirement: embedding 调用 token 用量记录

airouter 的 embedding 调用（`Router.Embed`）SHALL 与 chat 调用同等记录 token 用量：provider 响应携带 `usage` 块且调用成功时，`ai_call_logs.token_usage` SHALL 为 `{"prompt":N,"completion":N,"total":N}` 的 jsonb（按 OpenAI 兼容标准键解析）；响应无 `usage` 块或调用失败时 SHALL 为 NULL。命中本地 embedding 缓存（`ai_embedding_cache`）直接返回、未发起 provider 调用时不产生用量，`token_usage` SHALL 为 NULL。

#### Scenario: embedding 成功调用记录用量

- **WHEN** `Router.Embed` 调用 provider 成功且响应包含 `usage` 块
- **THEN** 写入的 `ai_call_logs.token_usage` SHALL 为非零的 `{"prompt":N,"completion":N,"total":N}`，与响应 `usage` 块数值一致

#### Scenario: embedding 缓存命中不记用量

- **WHEN** embedding 请求命中本地缓存（未发起 provider 调用）
- **THEN** 对应 `ai_call_logs` 行的 `token_usage` SHALL 为 NULL

#### Scenario: session 聚合涵盖 embedding 用量

- **WHEN** 按同一 `session_id` 聚合（如 `GET /api/ai/sessions/:session_id`）
- **THEN** `summary.total_tokens` SHALL 同时累加该 session 下 chat 与 embedding 调用的用量
