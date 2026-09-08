# Design — 物化轨 LLM 裁决

## Context

见 proposal.md（Why：库内实证的意图丢失/沾边命中/分流 bug）。现状代码事实：`watch_materialize_keyword.go`（`MaterializeKeywordWatches`，DNF 召回→机械组装）、`watch_materialize_sentence.go`（`MaterializeSentenceWatch`，向量检索→标签解析→`ListArticlesByTagsForDay` 全量并集）、`daily_report_watch.go`（`evaluateWatchHitsWithChat` 分流 bug）。label 轨已有可复用的批量 AI 调用范式（`evaluateLabelWatchHitsWithChat`：JSONMode+JSONSchema+幻觉 ID 过滤+`CapabilityDigestPolish`+失败吞掉）。

## Goals / Non-Goals

**Goals**

- 两物化轨召回后各插一道 LLM 文章级裁决（过滤 + 置信度），标题生成与裁决同批。
- 修 `evaluateWatchHitsWithChat` 分流：物化轨不再误入 label 轨 AI 判定。
- 裁决全程可降级、可关闭、可观测（ai_settings 开关 + 日志 + ai_call_logs 天然记账）。

**Non-Goals**（proposal 已声明，此处列设计侧边界）

- 不动召回层（DNF 语义、向量阈值/top-K、`MatchKeywordInstant` 即时回扫）。
- 不做跨板块去重。
- 不清洗 thread summary 的原文 HTML（另一层问题）。
- 不回刷历史板块（标题/置信度均保持原样）。

## Decisions

### D1. 裁决编排：每 watch 单次批量调用（非全 board 打包）

每个物化 watch 独立一次 Chat 调用，打包该 watch 当日全部候选文章；板块标题生成合并在同一调用内（schema 加 `section_title` 字段），满足 spec"标题不单独增加调用轮次"。

理由：(a) 不同 watch 意图语义差异大（一句话 vs 关键字），混包 prompt 语义混杂；(b) 单 watch 失败降级影响面最小；(c) 标题基于该 watch 的通过文章生成，天然同批。备选（全 board 打包一次）省调用数但一个 watch 候选暴涨会拖垮全部，且标题与文章裁决的对应关系要靠嵌套结构表达，复杂度不值——当前量级（3 watch/天）调用成本忽略不计。

### D2. 插入点与共用函数：召回后、组装 section 前

- keyword 轨：`matchKeywordArticles` 之后过滤命中文章。
- sentence 轨：`ListArticlesByTagsForDay` 并集之后过滤。

两轨共用一个 `adjudicateWatchArticles(ctx, 意图描述, candidates, chat)`：输入候选（id + title + summary 截断），输出（通过集 + 各篇置信度 + 板块标题 + error）。keyword 轨的意图描述= 关键字表达式 + 「贴合追踪意图而非字面包含」说明；sentence 轨 = 检索句（Query 回退 Label）。

裁决输入文本截断：每篇 title + summary 截 200 runes（对齐 `keywordWatchSummaryRunes` 量级，防极端日爆 prompt）。

### D3. 裁决标准写进 system prompt：意图限定词 + 实质因果链

system prompt 明确两层标准（用户定调，库内实证校准）：

1. 文章须贴合追踪意图的**重心/限定词**（「美伊形势**对市场影响**」→ 仅平行存在的战报/人道新闻不算）；
2. **实质因果链成立即算贴合**（美伊军事行动波及沙特船只=直接后果，保留；顺带提及关键词但主题无关=剔除）。

温度 0.1、JSONMode + JSONSchema，schema：

```json
{
  "verdicts": [{"article_id": int, "related": bool, "confidence": float, "reason": str}],
  "section_title": "str（可空）"
}
```

幻觉防护：`validArticleIDs` 过滤（复用 label 轨模式）。`confidence` 取 0~1 写入 `thread.Confidence`；降级/关闭/无裁决时保持现状 1.0（历史与降级一致性）。

### D4. 降级分层（对齐 spec「物化失败降级」）

- 召回失败 → 跳过该 watch 当期（现状不变）。
- 裁决调用失败 / JSON 解析失败 / verdicts 全幻觉 → **回退召回全量聚合**（旧行为），标题走兜底链（watch 名），`Confidence` 全部 1.0，Warn 日志记剔除率不可用。
- 裁决成功但全剔 → 不产 section（合法：当日无贴合内容，sentence 话题自然衰减——与「无命中不产空 section」现状语义一致）。

### D5. 分流修复 + 存量违规 hits 清理

`evaluateWatchHitsWithChat` 分流改为显式三路：`label` → AI 判定组；`keyword` → 文本匹配组；`keyword_topic`/`sentence_topic` → 跳过。若 repository 无显式 `WatchTypeLabel` 常量则补上（与迁移 CHECK 约束的取值一致）。

存量清理：上线迁移/启动清理一次性 `DELETE FROM topic_watch_hits WHERE watch_id IN (SELECT id FROM board_topic_watches WHERE type IN ('keyword_topic','sentence_topic'))`——这些行本就不该存在（spec 红线），删除无下游依赖（只读覆盖层），消除时间线存量重复曝光。

### D6. 配置与可观测

`ai_settings`（沿用 `watch_sentence_retrieval_*` 加载模式，`LoadWatchMaterializeConfig`）：

- `watch_materialize_llm_filter_enabled`（默认 `true`，false 时两轨回退纯机械/向量聚合）
- `watch_materialize_candidate_limit`（单 watch 送裁候选上限，默认 40，超限按 article id 截断 + Warn——极端日防 prompt 爆炸；截断只影响裁决层，关闭开关时不受限）

prompt version 记入调用 metadata；每次裁决记日志：候选数 / 通过数 / 剔除率 / 标题 / 是否降级。airouter 管线自动落 `ai_call_logs`（operation `watch_materialize.adjudicate`），SessionID 复用日报 session（与 label 轨同惯例）。

### D7. 前端装饰字段：读路径 transient 透出

`DailyReportSection` 加 `WatchLabel string \`gorm:"-"\``（参照 `TopicWatchHit.WatchLabel` 先例），由日报详情读路径按 `lane_tier=watch_*` + 板块归属 watch 回填（keyword 轨板块名可从固定名称解析，sentence 轨经 `PersistentTopicID`/归属 watch 查询——具体关联在 tasks 细化，无 schema 变更）。`SectionWatchBadge` 加可选 prop 显示 `「关键字物化板块 · harness」`式装饰。

## Risks / Trade-offs

- [LLM 裁决标准主观，因果链判定可能过严/过松] → prompt 事实锚措辞 + promptVersion 版本化 + 每次裁决日志（剔除率可观察），阈值不调代码可迭代 prompt。
- [裁决全剔导致板块消失，sentence 话题连续命中断] → 这是「当日确实无贴合内容」的正确语义（与用户意图一致）；观察期内若剔除率异常（如长期 100%）靠日志发现，调 prompt 或关开关。
- [候选文章数极端日超限截断] → 只截裁决输入（D6），关闭开关时全量；截断记 Warn。
- [降级回退全量时板块质量回到现状] → 接受：宁可旧行为也不空板块/不阻断（与既有物化降级哲学一致）。
- [一次性删 hits 影响时间线展示] → 这些命中本属违规数据；删除后时间线预告只剩合法 label/keyword 轨，展示更准。

## Migration Plan

1. 部署即生效：下一期日报生成起，物化轨走裁决（开关默认开）。
2. 存量违规 hits 一次性清理（D5，幂等）。
3. 回滚：`ai_settings` 关 `watch_materialize_llm_filter_enabled` 即回旧行为（无需回滚代码）；分流修复无回滚需求（本就是 spec 要求行为）。

## Open Questions

（无——裁决 prompt 措辞细节与截断长度可在实现期微调，不影响 spec/结构/任务拆分。）
