<!-- complexity: simple -->
<!-- ui-impact: none -->
<!-- constraint-domains: semantic-board, ai-summary -->

# Proposal: guard-tag-exemplar-leakage

## Why

打标融合提取的 system prompt（`buildMergedExtractionPrompt`）以 **PostgreSQL 作为 keyword 提取的 few-shot 正例**，在 prompt 中出现 3 次（keyword 定义举例、description 示例、输出格式正例 JSON）。本地小模型（Qwen3.5-4B-UD）会在正文完全未提及数据库的文章上**照抄示例词**——2026-10-23 周刊第 416 期（article 153877）实测：`ai_call_logs` 中响应的 `{"label":"PostgreSQL","description":"开源关系型数据库"}` 与 prompt 示例一字不差，而该文章正文（content/firecrawl_content 共 2.3 万字符）中 postgres/数据库/SQL 全部 0 命中；event 标签的 auxiliary_labels 也被塞入了同一示例词。

摸底显示污染是**大面积**的：PostgreSQL 标签（id=1556）挂的 36 篇文章中 **25 篇（69%）正文 0 次出现 postgres**；对照组 Transformer 标签 6 篇全部有真实提及——泄漏模式明确、只针对 prompt 示例词。

## What Changes

- **解析端新增「示例词泄漏护栏」**：LLM 输出的 keyword 标签候选（含所有标签的 auxiliary_labels 项）若与 prompt 示例词集合匹配（Slugify 归一后等值），且在**模型可见输入**（ExtractionInput 的 Title + Summary）中 0 次出现该词（大小写不敏感 Contains），则该候选在持久化前丢弃。mono 与 aggregate 两条提取路径共享同一护栏。
- **示例词集合提取为代码常量**：与 prompt 文本同源维护，附防漂移单测（断言集合中每个词都出现在当前 prompt 文本中，prompt 改示例词而忘更新集合时测试红）。
- **存量清理**：一次性移除「keyword 标签 ∈ 示例词集合 且 文章正文（title+content+firecrawl_content）0 次出现」的 `article_topic_tags` 关联（实测约 25 行，全部为 PostgreSQL 标签误挂），随后清理因此孤儿的标签；正文确有提及的 11 篇关联保留，PostgreSQL 标签本身保留。
- **不修改 prompt 示例词**（用户决策：换示例词是换汤不换药，结构性护栏兜底为准）。

## Capabilities

- **Modified Capabilities**:
  - `tagging-domain`：新增 Requirement「示例词泄漏护栏」——与既有「泛词标签黑名单拦截」「heuristic 兜底纪律」同谱系的打标污染治理条款。

## Impact

- **代码**：`backend-go/internal/tagmanagement/`（extractor 解析层 + 一次性存量清理）；不新增 API 路由、不改数据库 schema、无前端改动。
- **数据**：删除约 25 行 `article_topic_tags` 误挂关联 + 孤儿标签清理（PostgreSQL 标签及其 11 条真实关联保留）；下游标签计数（tag_count/feed_count 对账）由既有周期对账自然修正。
- **行为**：护栏只丢弃「示例词且无正文依据」的候选，不影响其它任何标签的提取与持久化；LLM 返回空结果仍是合法结论（既有条款不变）。
