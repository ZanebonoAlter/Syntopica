# Design: merge-tag-extraction-branches

## Context

现状（`backend-go/internal/tagmanagement/service/core/extractor_enhanced.go`）：`ExtractTags`（:44）起两个 goroutine 并行跑 `extractEventPersonCandidates` / `extractKeywordCandidates`，各自经 `extractBranchCandidates`（:121）发一次 `Router.Chat`（CapabilityTopicTagging）——同一份 `buildExtractionUserPrompt(input)` user prompt 送两遍，两份不同 system prompt（`buildEventPersonPrompt` / `buildKeywordPrompt`，各 ~1200 字），每分支 maxRetries=3、maxTokens=2048、temperature 0.2、JSONMode+JSONSchema。

实测（2026-09-19，`ai_call_logs` 3 天）：`tagmanagement.extractor_enhanced` 5674 次 × 平均 4.4s；~950 篇/晚。本地 qwen（llama.cpp，并发 2 slot）下 prefill 是主要成本，user prompt p50 ~3.9k 字符送两遍 = 直接翻倍。

## Goals / Non-Goals

**Goals:**

- mono 提取每篇 LLM 调用 2 → 1，prefill 减半；打标段服务器时间预期 -35~45%。
- 契约保留：双数组语义分流（event/person 带辅助标签、keyword 带 description）、数组级部分产出、heuristic 兜底链、去重优先级与数量上限。

**Non-Goals:**

- 不动数量上限（spec ≤5 / 代码 `maxArticleTags=6` 的既有漂移原样保留，见 D5）。
- 不动 aggregate 按栏目调用路径（`extractor_section.go` 的融合 prompt 本来就是单次调用/片）。
- 不动入库链路（三级去重、keyword 直入、embedding——semantic-board 红线 1/2 不涉及）。
- 不动 maxRetries=3 / maxTokens=2048 / temperature。

## Decisions

### D1: schema 双数组而非单数组带 category 字段

`{"event_person_tags": [tag…], "keyword_tags": [tag…]}` 顶层双数组，JSONSchema 按数组分流校验。备选（单 `tags` 数组 + category 字段，即现状解析结构）否决：现状 event/person 分支靠解析后强校验"category=keyword 即 parse error 触发重试"来防串扰，单数组下该防串扰仍要保留且重试成本更高；双数组让 schema 在解码侧就约束分流，9B 模型错放位置的概率更低。

### D2: 两份系统提示合并去重为一份

共同规则（排序、宁缺毋滥、拒绝泛词清单、输出格式正反例骨架）只保留一份；各自独有规则归位：辅助标签规则 → event/person 段，description 规则 → keyword 段。目标合并后 ≤1500 字（现两份合计 ~2400 字，system prompt 也省一份 prefill）。`TestBuildExtractionSystemPrompt*` 断言随之更新。

### D3: 失败语义按 spec 演进，复用既有兜底代码

整体失败（重试耗尽）→ 沿用 `extractWithHeuristic`；keyword 数组空 → 沿用 `heuristicKeywordCandidates` 展示兜底；event/person 数组空 → 仅记录 branchErrors。`mergeExtractedTags` / `resolveCandidate` / `limitArticleTags` 等纯函数零改动（`TestMergeExtractedTags*` 保持绿）。

### D4: operation 与 metadata 保持可观测

外层 operation 保留 `tagmanagement.extractor_enhanced`（利于前后同口径对比），`request_meta.operation` 由 `tag_extraction_event_person`/`tag_extraction_keyword` 改为 `tag_extraction_merged`。调用次数验证直接查 ai_call_logs。

### D5: 记录既有契约漂移（不在本 change 修）

spec「合并后总数 ≤5、keyword ≤3」vs 代码 `maxArticleTags=6`。MODIFIED/ADDED 块照抄原契约数字；漂移留档，未来 change 决定归一方向。

## Risks / Trade-offs

- [9B 模型对双数组 JSON 遵从率下降，解析失败重试率升高（重试=重复 prefill，反噬收益）] → 上线后观测 ai_call_logs 失败率/attempt 分布（V5）；保留 maxRetries=3 兜底；单 commit revert 回滚。
- [系统提示合并措辞变化引起提取质量漂移] → 上线前后对固定 feed 集（含少数派/阮一峰样本）标签对比一周（V5 人工抽查）；prompt 长度断言防失控。
- [合并输出 token 变多（两份标签一次出）逼近 maxTokens] → 实测单分支输出 p50 远低于 2048，合并后仍在界内；若逼近上限属本 change 范围内调 maxTokens 一次决策，不扩契约。
- [失去"单分支独立重试"的韧性（原两分支各 3 次机会，合并后共 3 次）] → 接受：重试预算语义为"每篇提取共 3 次"；数组级缺失路径（非失败）不受影响。

## Migration Plan

1. 合并部署即生效（新文章打标走单次调用）；存量标签不受影响、不重打标。
2. 回滚：revert 单 commit，无数据迁移、无状态。
3. 上线观测见 tasks.md V5（调用数减半 + 解析失败率 + 质量抽查）。

## Open Questions

（无——schema 结构、prompt 合并策略、失败语义均已在 spec/design 定案；数量契约漂移是既有事实不阻塞。）
