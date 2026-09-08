<!-- complexity: complex -->
<!-- ui-impact: minor -->
<!-- constraint-domains: daily-report -->

## Why

物化轨 watch（keyword_topic / sentence_topic）的板块聚合质量不符合追踪意图（库内实证，board 1974/1980，2026-08-26~09-04 数据）：

- **sentence 轨意图限定词丢失**：一句话向量命中宽泛辅助标签后，标签下当天文章**全量并集**、无文章级过滤。「美伊形式**对市场影响**」板块 09-02 期 20 篇约一半是纯战报/人道/外交新闻（"俄罗斯被曝帮伊朗研发导弹"、"伊朗婚礼现场遭袭"），与"市场影响"无关；「vibe coding**开源生态**跟踪」板块退化为泛 AI 编程内容流（LangChain 教程、AIOps、推理引擎集群）。
- **keyword 轨字面命中沾边就进**：「关键字『harness』」板块 18 篇中相当部分标题不含 harness、仅摘要正文顺带提及，与真目标（"DeepSeek Harness 源码解析"）混排。
- **现存 bug**：`evaluateWatchHitsWithChat` 分流只排除 `type=keyword`，`keyword_topic`/`sentence_topic` 全部漏进 label 轨 AI 批量判定，持续产生 `topic_watch_hits`（库内 08-26 起有据）——违反 `topic-watch` spec 既有红线（"物化轨 SHALL NOT 产生命中提示记录"），造成时间线重复曝光 + 白跑 AI 调用。

提示轨 label 的 AI 命中判定（同库实证）能紧扣意图重心（"美伊紧张关系重燃**直接推高布伦特原油价格**"），证明 LLM 裁决在本系统已有效运行、只是物化轨没用上。召回层（向量标签检索 / DNF 匹配）保留不动，在召回后加 LLM 文章级精排裁决。

## What Changes

- **物化轨 LLM 文章级裁决**：sentence 轨（标签命中→文章并集后）与 keyword 轨（DNF 命中后）各加一道 LLM 批量裁决——单次调用打包 watch × 当日候选文章（标题+摘要截断），判定每篇文章是否贴合追踪意图；裁决标准含**意图限定词 + 实质因果链**（美伊军事行动波及沙特船只=有因果，算贴合；纯平行事件不算）。通过裁决的文章才聚合进板块。
- **置信度承载**：`DailyReportThread.Confidence` 从硬编码 1.0 改为承载裁决置信度（物化轨 thread）。
- **板块当日标题**：物化板块 `cluster_label` 由 LLM 生成当日贴合标题（现=watch 名照抄）；watch 名作为小装饰保留在板块展示中，与现有话题板块展示一致。兜底链：LLM 标题 → watch 名。
- **降级策略**：裁决 AI 调用失败 → 回退该 watch 当日召回全量结果（旧行为），不阻断日报生成。
- **修复物化轨误入提示轨**：分流逻辑改为仅 `type=label` 走 label 轨 AI 命中判定，`keyword_topic`/`sentence_topic` 直接跳过；`topic-watch` spec 的 AI 命中判定 requirement 适用范围同步收紧（消除"对所有 active 关注执行判定"与隔离红线的措辞矛盾）。
- **配置与可观测**：裁决管线经 `ai_settings` 在线可调（开关、候选文章上限），prompt 版本化，裁决结果（含剔除文章数）记日志。
- **明确排除**：跨板块重叠不去重（用户决策，保持现状）；提示轨 keyword 零 AI 红线不动；召回层参数（阈值/top-K）不动。

## Capabilities

### New Capabilities

（无）

### Modified Capabilities

- `watch-materialized-topic`: 物化两轨召回后新增 LLM 文章级裁决（过滤+置信度）与板块当日标题生成，含降级回退；物化管线边界补充（裁决不改归属、不改生命周期）
- `topic-watch`: "关注标记 AI 命中判定" requirement 适用范围收紧为 label 轨（排除 keyword_topic/sentence_topic 误入），消除与既有隔离红线的措辞矛盾

## Impact

- **后端代码**：`backend-go/internal/topicgraph/service/watch_materialize_keyword.go`、`watch_materialize_sentence.go`（裁决插入）、`daily_report_watch.go`（分流修复）、`keyword_match.go`（复用 DNF）；airouter Chat 调用（复用 label 轨批量模式：JSONMode+JSONSchema+幻觉 ID 过滤）
- **配置**：`ai_settings` 新增裁决开关键（沿用 `watch_sentence_retrieval_*` 的加载模式）；prompt version 递增
- **前端**：物化板块标题渲染 + watch 名小装饰（现有板块结构内，minor）；thread Confidence 若前端有消费点同步
- **数据库**：无 schema 变更（Confidence 字段既有）；历史物化板块不回刷
- **AI 成本**：每 board 每天 +物化 watch 数次批量调用（当前实测 3 watch × ≤20 篇候选/天，量级可忽略）
