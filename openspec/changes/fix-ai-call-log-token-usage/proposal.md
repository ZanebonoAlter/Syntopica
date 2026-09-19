<!-- complexity: complex -->
<!-- ui-impact: none -->
<!-- constraint-domains: ai-summary -->

## Why

`ai_call_logs.token_usage` 字段自上线以来从未记录过真实值：全库 134,064 条日志中，chat 调用写入的 token_usage 全部是 `{"prompt":0,"completion":0,"total":0}`（json tag 与 OpenAI 兼容接口实际返回的 `prompt_tokens`/`completion_tokens`/`total_tokens` 键错位，encoding/json 静默填零），embedding 调用则完全没有解析与写入管道（全 NULL）。下游 session 聚合（`GetSession` 的 `summary.total_tokens`）与调用日志展示因此永远为零，AI 用量观测形同虚设。`ai-logging` spec 既有 Scenario「token 用量记录」要求的正是非零 jsonb，属于实现与 spec 脱节，需修复对齐。

## What Changes

- **chat 路径解析修复**：`openAICompatibleClient.Chat` 改用 OpenAI 兼容标准键（`prompt_tokens`/`completion_tokens`/`total_tokens`）解析 provider 响应的 `usage` 块；`ai_call_logs.token_usage` 的存储形状保持 `{"prompt":N,"completion":N,"total":N}` 不变（消费方零改动）。
- **embedding 路径补管道**：`openAICompatibleClient.Embed` 解析响应 `usage` 块 → `EmbeddingResult` 新增 `Usage` 字段 → `Router.Embed` 成功日志填充 `TokenUsage`。
- **失败与无 usage 兜底**：provider 报错或响应无 `usage` 块时 `token_usage` 保持 NULL（现有 `Omit("token_usage")` 逻辑已支持），不写全零值。
- **明确不可回填**：历史全零/NULL 行无法恢复原始用量（响应未存），本变更只对修复后的新调用生效。
- 无 DB 迁移、无 API 契约变更、无前端改动（现有展示位自动从全零变为真实值）。

## Capabilities

### New Capabilities

（无）

### Modified Capabilities

- `ai-logging`：收紧「token 用量记录」Requirement——明确 Chat 解析 OpenAI 兼容标准 usage 键、Embed 路径同样记录 token 用量、存储 jsonb 形状不变、无 usage/失败时置 NULL（禁止全零值落库）。

## Impact

- **代码**：`backend-go/internal/platform/airouter/`（`openai_compatible.go` 解析、`router.go` 写入、`embedding.go` 结果结构）。
- **数据库**：无 schema 变更；`token_usage` 列已存在（jsonb）。
- **消费方**：`session_handler`（tokenUsageAgg 聚合）、`ai_call_log_handler` 列表——读取形状不变，开始拿到真实值。
- **数据**：存量 ~15,163 条全零 chat 行与 ~11.9 万条 NULL embedding 行不迁移、不回填。
- **验证手段**：真实 chat/embedding 调用后查 `token_usage` 非零/非 NULL；单测覆盖键错位回归（`prompt_tokens` 解析）与存储形状稳定。
