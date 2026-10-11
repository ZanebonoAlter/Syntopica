# Design — fix-deepseek-thinking-empty-response

## Context

诊断结论见 proposal.md（Why）。技术现场要点：

- `effectiveThinkingLevel` 四级解析（`thinking.go`）：请求级 > 绑定级（`ai_route_models.thinking_level`）> 模型默认档（`ai_models.thinking_level`）> 全局默认 medium。绑定级覆盖通道已存在（spec `ai-thinking-levels`「场景级思考档覆盖」），保存入口校验合法七档。
- `thinkingToWire("deepseek", level)`：off → `thinking={"type":"disabled"}`，其余 → `{"type":"enabled"}`。DeepSeek 推理模型的 `max_tokens` 是**含 reasoning 的输出总预算**——思考烧满预算后 `content` 为空。
- `openai_compatible.go` 空响应归一化路径（HTTP 200 + content 空）当前只返回 `ProviderError{empty_response}`，**丢弃了响应里的 `usage` 块与 `finish_reason`**——这是失败行账目缺失的直接原因。
- `router.go` 失败记账有两处：降级链失败档逐行审计（`LogCall` with attempts）与最终失败行。两处均未落 `TokenUsage`。
- 模型同步（`store_vendor.go` SyncModels）只更新 `last_seen_at`/状态，不回洗 `thinking_level`——配置修正是持久的。

## Goals / Non-Goals

**Goals**

- digest_polish 路由的 DeepSeek 调用停止思考烧钱（空响应率从 ~60% 骤降）。
- data_enrichment_analysis 路由（deepseek-v4-pro）行为零变化——用户明确该场景需要思考。
- 失败调用若 provider 响应带 usage/finish_reason，账目可查——下次同类问题不用手工复现归因。

**Non-Goals**

- 不改降级链语义（empty_response 仍 retryable、仍 ordered_failover）。
- 不做"预算耗尽自动重试"（同档升预算或关思考重发）——留待观测数据证明有残余价值再做。
- 不引入按 token 计价的成本核算 UI（账单金额仍以 provider 后台为准）。
- 不处理 qwythos parse_error 840 次（输出不规范 JSON）与 http_502/503 服务不稳——另行立项。

## Decisions

### D1 思考档修正走"模型默认档 + 绑定级"双层，且只动 flash

**选择**：`ai_models.deepseek-flash.thinking_level: low→off`（模型级）+ `ai_route_models` digest_polish×deepseek-flash 绑定补 `off`（绑定级）。

**备选与取舍**：

| 方案 | 评价 |
| --- | --- |
| A. 两个模型默认档全改 off | ❌ 被 veto：data_enrichment_analysis（v4-pro）需要思考 |
| B. 只做绑定级 off（不动模型默认档） | 部分 ok：修当前灾区，但 flash 被未来路由复用时默认又烧思考 |
| C. 只改模型默认档（不补绑定级） | 部分 ok：当前等价，但绑定级缺失时配置漂移无显式锚点 |
| **D. 模型级 off + 绑定级 off（flash）；v4-pro 全不动（采纳）** | flash 定位"快闪"与不思考自洽；绑定级显式声明是路由视角的文档；双保险防回漂 |

四级解析中绑定级 > 模型默认档，两层同为 `off` 无冲突；v4-pro 保持 `low`，其路由调用继续思考。

**运维约定**（写入 flow 文档变更溯源，tasks 落地）：新路由绑定推理模型（dialect 为 deepseek / reasoning-effort 计入 max_tokens 的方言）时，SHALL 在绑定级显式评估思考档，默认不配即继承模型档。

### D2 失败 usage/finish_reason 经 ProviderError 携带，不改 ChatResponse 结构

`ProviderError` 增加两个可选载荷字段（`Usage *TokenUsage`、`FinishReason string`，零值不序列化语义不变）：

- `openai_compatible.go` 空响应归一化处：构造 `ProviderError` 时带上 `parsed.Usage` 与 `choices[0].finish_reason`。
- `parse_error`（结构化 JSON 提取失败）路径同理——HTTP 200 的解析失败同样可能带 usage。
- 网络/HTTP 错误路径天然无值，保持现状。
- `router.go` 两处失败 `LogCall`：`TokenUsage` 取 `err.Usage`（jsonb 形状同成功路径 + 可选 `reasoning` 键），`response_snippet` 前缀 `finish_reason=<值>` 后接原错误文本。

**备选**：改 `ChatResponse` 加 Err 字段——侵入成功路径结构，所有 provider client 都要动，否决。`ProviderError` 是失败路径的既有载体，加字段面最小。

### D3 reasoning 细分作为可选键，不破坏既有 jsonb 三键形状

`TokenUsage` 解析时若响应带 `completion_tokens_details.reasoning_tokens`，落库附加 `"reasoning":N`。session 聚合等消费方按键读取，附加键不破坏兼容；spec 已同步松绑（"基础三键不变，允许附加可选键"）。

### D4 配置修正不写迁移，走一次性数据步骤

`thinking_level` 是用户可运维的运行时配置（设置页可改），不是 schema。写入 tasks 作为部署后数据步骤（两条 UPDATE），并在验收时以真库查询核对。回滚 = 改回 `low`，无代码耦合。

## Risks / Trade-offs

- [flash 关思考后总结质量下降] → 重放实验 C 显示 lane_snapshot 类任务产出正常；部署后观察 digest_polish 成功率与日报文案质量，必要时绑定级回调 `minimal`（仍远低于 low 的预算消耗）。
- [空响应残余：DeepSeek 偶发真空响应（非 length）] → 降级链兜底不变；失败行现在带 finish_reason，可区分归因。
- [ProviderError 加字段被误当重试信号] → 字段仅观测载荷，Retryable 语义单独维护，测试锁定。
- [失败行开始落 usage 后，账目口径变化] → session 聚合若统计"成功 token"需确认过滤条件；当前聚合按行读取、`success` 过滤在查询侧，附加失败行 usage 不影响成功口径（核对 task 覆盖）。

## Migration Plan

1. 部署代码（失败 usage/finish_reason 记账）——独立可先行，无前置依赖。
2. 数据步骤：`UPDATE ai_models SET thinking_level='off' WHERE id=16 AND name='deepseek-flash'`；digest_polish×flash 绑定级补 `off`（经 UpsertRouteModels 通道或等价 SQL，带合法性校验）。
3. 回滚：代码回滚即可（记账增强无破坏性）；思考档改回 `low` 单条 UPDATE。均无数据不可逆风险。

## Open Questions

（无——影响面与粒度已与用户确认：data_enrichment_analysis 需要思考，仅 flash 关思考。）
