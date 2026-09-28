<!-- complexity: simple -->
<!-- ui-impact: none -->
<!-- constraint-domains: scheduler -->

## Why

2026-09-28 实测：本地 LLM（qwen）整体未启动、只开 embedding 时，健康门仍判健康——宽松判定「≥1 embedding 主通 **且** ≥1 任意非 embedding 主通」被 `digest_polish` / `data_enrichment_analysis` 的云端 deepseek-flash 主力满足，而真正决定文章入库的 `topic_tagging`（本地 qwen、无兜底）已不可达。门开着 → tag worker 继续消费 → 打标调用失败 → heuristic 降级标签入库（当日 675 次失败调用、150+ 条 heuristic 链接，其中 76 篇文章整篇只剩降级标签）。

健康门的语义应当是**文章入库的前置拦截**：文章不入库，后续分析没有意义（用户 2026-09-28 拍板）。「任意一条 LLM 通就算健康」的口径与这个语义不符。

## What Changes

- 健康就绪判定从「宽松双类各一条」改为**核心双路由硬条件**：≥1 embedding 路由主 provider 可达 **且** `topic_tagging` 路由主 provider 可达；任一不满足即 NOT 健康。
- `topic_tagging` 路由缺失 / 未绑定任何启用 provider / 主 provider 不可达，一律 NOT 健康（fail-closed——打标不可用时文章无法入库，前置拦截必须关）。
- 其余非 embedding 路由（summary / digest_polish / data_enrichment_news / data_enrichment_analysis / feed_discovery / open_notebook）**不再参与**健康判定；单条非核心路由不可达不推翻整体（维持既有豁免语义，豁免范围扩大到核心双路由之外）。
- 探测机制（只探主 provider、启动自动拉起、60s 心跳复检、连续 2 次失败去抖、启动竞态 fail-closed、快照与健康 API 返回结构）全部不变——**只改判定口径，不改机制**。
- 无 BREAKING API 变化：`GET /api/ai/health` 返回结构不变，routes 明细照旧，仅 `healthy` 计算规则变。

## Capabilities

### New Capabilities

（无）

### Modified Capabilities

- `ai-model-health`: 「健康就绪判定（宽松）」需求改写为「健康就绪判定（核心路由硬条件）」——判定条件从「双类各一条」改为「embedding ∧ topic_tagging」；场景集重写（核心双路由均通即健康 / 缺 embedding 不健康 / 缺 topic_tagging 不健康 / topic_tagging 未配置不健康 / 云端 deepseek 可达不救命 / 单条非核心路由不可达不推翻整体）。

## Impact

- **代码**：`backend-go/internal/platform/aihealth/aihealth.go`（`runProbeLocked` 判定逻辑与 `hasReachableCapability*` 辅助函数）；测试 `aihealth_test.go`、`internal/platform/analysispause/health_gate_compose_test.go`（3 个 compose 场景的口径同步改写）。
- **行为（已知取舍，用户拍板）**：本地 LLM 挂掉时，即使 deepseek 云端可用，分析类 job（daily_report / digest_polish / board_upgrade_suggest 等）与 tag worker 池一并停——全链路停摆直到 topic_tagging 主 provider 恢复。取舍理由：打标失败会产生 heuristic 降级标签污染入库数据，比"分析晚点跑"代价大。
- **文档**：`docs/reference/flow/scheduler.md` §业务约束 7「健康门维度」判定口径 + 变更溯源行；`docs/reference/flow/ai-summary.md` 健康入口如提及宽松判定则同步。
- **前端**：无改动。设置页「AI 健康状态」卡片展示 routes 明细与 healthy，语义自然跟随，无需改版。
