# Design — fix-ai-call-log-token-usage

## Context

`ai_call_logs.token_usage`（jsonb）从未记录过真实值：全库 134,064 行中 chat 行全部是 `{"prompt":0,"completion":0,"total":0}`，embedding 行全部 NULL（见 proposal Why）。

关键事实链：

1. chat 行的 token_usage **非 NULL 且非空串**，说明 `openAICompatibleClient.Chat` 里 `parsed.Usage` 是非 nil 的——即 provider 响应确实携带 `usage` 块，只是键没映射上。
2. `airouter.TokenUsage` 的 json tag 是 `prompt`/`completion`/`total`；OpenAI 兼容接口（llama.cpp / vLLM / Ollama 及各类中转）标准返回键是 `prompt_tokens`/`completion_tokens`/`total_tokens`。`encoding/json` 对缺失键静默填零 → 存储值恒为全零。
3. 同一个 `TokenUsage` struct 身兼两职：**解析 wire 格式**（需要标准键）与**序列化存储格式**（`{"prompt":N,...}`，`session_handler.tokenUsageAgg` 与管理端按此形状读取）。
4. embedding 路径（占全库行数 ~88%）无任何管道：`openAICompatibleClient.Embed` 不解析 usage、`EmbeddingResult` 无 Usage 字段、`Router.Embed` 的 LogCall 不设置 TokenUsage。
5. `Store.LogCall` 已有正确兜底：TokenUsage 为空串时 `Omit("token_usage")` 让 DB 置 NULL（jsonb 不接受空串）。

相关代码：`backend-go/internal/platform/airouter/openai_compatible.go`（两个 client 的响应解析）、`router.go`（`Router.Chat`/`Router.Embed` 的 LogCall 调用点）、`embedding.go`（`EmbeddingResult`）、`store.go`（`encodeTokenUsage` 在 router.go；`LogCall` 在 store.go）。

## Goals / Non-Goals

**Goals**

- chat 调用按 OpenAI 兼容标准键正确解析 provider usage，落库真实值。
- embedding 调用补齐 usage 解析 → 结果结构 → LogCall 的完整管道。
- 存储形状与消费方（session 聚合、调用日志列表 API）零改动。

**Non-Goals**

- 不回填历史数据（原始响应未持久化，物理不可恢复）。
- 不改 token_usage 的存储 jsonb 形状、不动 `session_handler`/`ai_call_log_handler` 消费方。
- 不做失败请求的用量估算（provider 报错时无 usage 块，保持 NULL）。
- 不新增任何基于 token 用量的告警/统计面板（ui-impact: none）。

## Decisions

### D1：`TokenUsage` 自定义 `UnmarshalJSON`，双键族兼容；`MarshalJSON` 保持默认（存储形状不变）

**选择**：给 `TokenUsage` 加自定义 `UnmarshalJSON`——内部经 alias wire struct 同时接受标准键（`prompt_tokens`/`completion_tokens`/`total_tokens`，`*int` 以容忍缺键）与历史键（`prompt`/`completion`/`total`）。不加自定义 `MarshalJSON`，序列化沿用 struct tag，存储 jsonb 仍是 `{"prompt":N,"completion":N,"total":N}`。

**为什么**：一处修复覆盖所有解析点（chat 现有、embedding 新增、未来流式）；`encodeTokenUsage` 与全部消费方零改动；历史键兼容是防御性的（当前无代码把存储 jsonb 反序列化回 `TokenUsage`，但成本≈0，防未来误用踩同一坑）。

**备选**：client 层独立 wire struct（`usageWire`）+ 显式映射到 `TokenUsage`。分层更"纯"，但要在 Chat/Embed 两个解析点各加一套类型与映射，churn 更大、收益相同。弃。

### D2：embedding 管道三段补齐，缓存命中不记用量

- `EmbeddingResult` 增加 `Usage *TokenUsage`（`json:"usage,omitempty"`）。
- `openAICompatibleClient.Embed` 的响应解析 struct 增加 `Usage *TokenUsage`（依赖 D1 的双键解析，标准键直接生效）。
- `Router.Embed` provider 成功路径的 `LogCall` 增加 `TokenUsage: encodeTokenUsage(res.Usage)`。

缓存命中路径（`ai_embedding_cache` 直接返回）不发起 provider 调用、无用量，`token_usage` 保持 NULL——与 delta spec「embedding 缓存命中不记用量」Scenario 对应，且 RequestMeta 已有 `cache_hit: true` 标记可区分。

**备选**：在缓存命中行也回填缓存写入时的 usage——需要给 `AIEmbeddingCache` 加列或塞进 metadata，复杂化且语义含混（缓存命中≈零边际成本）。弃。

### D3：失败与无 usage 块一律 NULL，禁止全零落库

失败路径与"成功但响应无 usage 块"不设置 TokenUsage（空串 → `LogCall` 既有 `Omit` 逻辑 → NULL）。全零值从此在语义上等于"数据损坏"，不应再产生。不校验"成功必须有 usage"——部分自建 provider 可能不带 usage 块，NULL 是诚实的表达。

### D4：不迁移、不回填

历史全零/NULL 行保持原样。修复只影响新写入。无 DB 迁移、无回滚脚本需求（回滚 = revert 提交，新旧行互不干扰）。

## Risks / Trade-offs

- [个别 provider 返回非标准 usage 键（既非 `prompt_tokens` 也非 `prompt`）] → 解析后仍全零。缓解：双键族已覆盖 OpenAI 官方 + 主流兼容实现；若未来某 provider 仍全零，属 provider 侧问题，可在 client 层加该 provider 的键映射（本 change 不做）。可在实现后真实调用一次验证（tasks 验证节）。
- [usage 数值异常巨大/为负] → 不做范围校验，信任 provider。观测性字段，业务不消费 `ChatResult.Usage`，无下游放大风险。
- [自定义 UnmarshalJSON 的隐蔽性] → 双键族行为必须在单测中显式固定（标准键解析 / 历史键兼容 / 缺键不报错三个用例），防止后人"清理"掉兼容逻辑。
- [流式响应] → 当前 client 仅非流式（无 `stream: true`），不存在 `stream_options.include_usage` 问题；若未来引入流式需重新设计 usage 采集。记录于此，不在本 change 范围。

## Migration Plan

纯代码部署：`go build` 通过 + 影响包测试绿即可上线，无 DB 操作、无配置变更。上线后新 chat/embedding 调用自然产生真实 token_usage；旧行不动。回滚 = revert。

## Open Questions

（无）
