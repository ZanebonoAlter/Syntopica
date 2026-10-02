<!-- complexity: simple -->
<!-- ui-impact: none -->
<!-- constraint-domains: ai-summary, daily-report -->

## Why

2026-10-01 用户发现一篇日报（全版面生成）消耗 DeepSeek 约 ¥0.8 且 AI 调用记录超百条。诊断实锤根因：`deepseek-flash` / `deepseek-v4-pro` 的模型默认思考档为 `low`（≠off），经 `thinkingToWire` deepseek 方言向 API 发送 `thinking:{"type":"enabled"}`；而 DeepSeek 推理模型的 `max_tokens` 是**含 reasoning 的输出总预算**——lane_snapshot（预算 1536）等调用中模型思考烧满全部预算后 `content` 为空，被 router 判为 `empty_response` 可重试错误，降级到本地 Qwen 兜底。

并发 20 路真实 prompt 重放复现：18/20 路 `finish_reason=length`、`completion_tokens=1536` 全部是 `reasoning_tokens`、content 空、延迟 ~7s（与现网失败 avg 7.3s 吻合）。后果三重：① 每次失败 DeepSeek 照常计费（用户在官方后台看到的消耗），约一半的钱付给颗粒无收的空思考；② 业务 1 次产出记 2 条调用记录（失败+fallback），放大调用量近一倍；③ 日报产出实际由本地 4B 模型兜底完成，DeepSeek 形同虚设。数据佐证：9-25 至 10-01 digest_polish 路由 DeepSeek 失败率 60-88%（9-25 达 173/197），qwen 兜底成功率 100%——从接入第一天起即如此。本地 qwen 两模型 `thinking_level=off` 无此问题。

诊断过程中还发现归因盲区：失败调用（HTTP 200 + 响应含 usage 块）的 `token_usage` 未落库（NULL）、`finish_reason` 丢失，导致"烧了多少冤枉钱"只能靠手工复现才能算清。

## What Changes

- **配置修正（数据步骤，按场景分粒度，不一刀切）**：
  - `deepseek-flash`（digest_polish 路由：总结/聚类/抽取类任务，不依赖深思考）——模型默认档 `low→off`，并在 digest_polish 路由绑定级显式声明 `off`（双保险，防配置回漂）；重放实验证实关思考后 lane_snapshot 类调用 completion 从 ~1536 降至 ~314 token 且正常产出。
  - `deepseek-v4-pro`（data_enrichment_analysis 路由：深度分析**确实需要思考**）——模型默认档保持 `low` 不动，该路由调用行为零变化。
  - 未来确需思考的其它调用由请求级 `ThinkingLevel` 显式升档（四级解析请求级最高，通道现成）；新路由复用 flash 时 SHALL 在绑定级显式评估思考档。模型同步逻辑只更新 `last_seen_at`，不会回洗该配置。
- **失败调用 token 用量记账（代码）**：provider 返回 HTTP 200 且携带 `usage` 块、但响应被判为失败（empty_response / parse_error 等归一化错误）时，失败行 SHALL 落库该 `token_usage`——空思考烧掉的 reasoning token 在账上一目了然，不再需要手工复现归因。
- **失败归因信息补强（代码）**：失败行的 `response_snippet`（或 error_message）SHALL 携带 `finish_reason`（如 `length`），使"预算耗尽型空响应"与"服务端真返回空"可区分。
- 不改降级链语义：`empty_response` 仍为 retryable、仍走 ordered_failover；不改七档值域与四级解析优先级；不改思考方言映射。

## Capabilities

### New Capabilities

（无）

### Modified Capabilities

- `ai-logging`: 「AICallLog 完整记录字段」requirement 中 token 用量记录行为扩展——由"调用失败时 token_usage SHALL 为 NULL"改为"调用失败时若 provider 响应携带 usage 块则 SHALL 落库（含 reasoning 细分），否则为 NULL"；并新增失败行 `finish_reason` 归因要求。

## Impact

- **配置（数据步骤，无 schema 变更）**：`UPDATE ai_models SET thinking_level='off'`（id=16 deepseek-flash，仅此一个）；`ai_route_models` digest_polish×deepseek-flash 绑定级补 `off`。v4-pro 与 data_enrichment_analysis 路由零改动。部署后 DeepSeek 侧日报调用空响应率应从 ~60% 骤降、账单显著回落。
- **代码**：
  - `backend-go/internal/platform/airouter/openai_compatible.go`：`ProviderError` 携带响应侧 `usage` 与 `finish_reason`（空响应归一化路径当前直接丢弃 usage）。
  - `backend-go/internal/platform/airouter/router.go`：失败行记账路径（`LogCall` 失败分支）落 `TokenUsage` 与 finish_reason 归因文本。
- **测试影响包**：`go test ./internal/platform/airouter/...`（按 change-scope.sh 机械判定）。
- **部署后行为变化**：DeepSeek 付费调用 token 消耗大幅下降（预期日报管线账单砍半以上）、digest_polish 路由 fallback 触发率骤降（更多产出由 DeepSeek 直接完成，日报文案质量回升）、`ai_call_logs` 失败行开始携带 usage 与 finish_reason；data_enrichment_analysis（v4-pro 思考）行为完全不变。旧数据零迁移零降级；无需用户手动操作。
