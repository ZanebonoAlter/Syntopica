# Design: section 展示标题内容化

## Context

现状：`daily_report_orchestrator.go` 构建 section 时，cluster 若带 `MatchedTopicID` 则 `clusterLabel = topic.Label`——展示标题冻结在话题创建时的事件名上，后续每天新内容顶着旧标题（钉子户现象，见 proposal Why 与 `docs/research/daily-report-sticky-title/explore-findings.md` 的取证）。

既有约束（本设计遵守）：

- 文案事实锚约束（`daily_report.highlights` / `daily_report.threads` 的禁编造规则）
- L2 裁决 prompt 历史隔离（标题属"框架名"信号，非 thread 文案，不构成叙事渗透）
- section 内容化 embedding：`buildSectionEmbedText(clusterTags, threads, clusterLabel)` 的 label 仅兜底
- lane 归属标记：`lane_tier` / `topic_match_*` 字段与标题来源正交

## Goals / Non-Goals

**Goals**

- `cluster_label` 反映当天实际内容；话题退为归属锚
- 复用既有 threads LLM 调用产出标题，零新增 AI 调用成本
- 故障时降级回旧行为（兜底链），永不出空标题

**Non-Goals**

- 不改话题 label 本身（话题名仍是话题身份，在图谱/泳道 UI 中照旧展示）
- 不回刷历史 section（自然分界）
- 不改 lane 分桶 / 归属 / 升级逻辑（candidate L1 gate 等保持现状）
- 不动前端（字段与接口不变，前端展示逻辑自适应新标题）

## Decisions

### D1: 标题由 threads LLM 调用顺带产出，不新增独立调用

`GenerateClusterThreads`（operation `daily_report.threads`）已按 cluster 吃进所聚标签事实，在其 JSON schema 顶层增加可选字段 `section_title`（string，description 注明「一句话概括当日该板块实际内容；须基于所列标签事实，禁止编造；不得复述候选话题名」）。

- 备选 A（否决）：独立 `daily_report.section_title` 调用——每 cluster 多一次 LLM 调用，成本与延迟翻倍，收益仅剩隔离性。
- 备选 B（否决）：取当日首条 thread title 直接当标题——thread 是"叙事线"视角，单条 thread 常只覆盖 cluster 局部，作板块标题偏窄；且 `{"threads":[]}` 时无值可用。
- prompt 历史隔离合规：threads prompt 本就无历史叙事注入，标题同样基于当日标签事实。

### D2: 标题解析兜底链

section 构建处（orchestrator section 循环）按序取值，首个非空者胜：

1. 该 cluster threads 响应中的 `section_title`（LLM 当日标题）
2. `threadsByCluster[i]` 首条 thread 的 `title`（含 `synthesizeFallbackThreads` 合成兜底——其 title 为 top tag label 纯转录，天然内容化）
3. `MatchedTopicID` 话题 label（旧行为，保底）
4. `cluster.GroupName`（L3 分组名，最终兜底）

原 `clusterLabel = topic.Label` 覆盖逻辑删除；L3 / watch 物化 section 天然落在 1 或 2，行为不变或对齐。

### D3: promptVersion 4.0 → 5.0

按既有惯例，threads prompt 变更随版本号生效并落 `board_daily_reports.generation_prompt_version`。

### D4: embedding 兜底文本不动

`buildSectionEmbedText` 第三参继续传 `clusterLabel`——内容化标题作为兜底文本比话题名复读更贴近实际内容，质心漂移方向正确（既有设计意图），无代码变更。

## Risks / Trade-offs

- [LLM 标题偶发复述话题名] → prompt 明示「不得复述候选话题名」；验收场景固化（同名即失败）；兜底链 3 仍以话题名保底，可观测但不阻断。
- [同话题跨天标题各异，用户误以为话题分裂] → 时间线/图谱以 `persistent_topic_id` 串联（spec 场景已固化）；前端无需改，但发版说明需向用户讲明标题语义变化。
- [`section_title` 未返回（模型漏字段）] → JSON schema 中设为可选、兜底链兜住；不因缺字段 fail 整个 cluster。
- [标题质量依赖小模型（CapabilityDigestPolish）] → 事实锚约束 + 后续可按 ai-capability-routing 调能力档，不阻塞本期。

## Migration Plan

1. 后端发布即生效（下一期日报起新 section 走内容化标题），无 DB schema 变更、无迁移脚本。
2. 回滚：还原 orchestrator 标题解析与 threads schema 即恢复旧行为，已生成的内容化标题保留无害。

## Open Questions

- threads 响应解析（`parseThreadsResponse`）扩展 `section_title` 时是否需要独立小结构体返回，实现时按现有代码风格定即可，不影响 spec。
