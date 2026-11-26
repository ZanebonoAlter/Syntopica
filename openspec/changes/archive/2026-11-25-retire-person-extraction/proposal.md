<!-- complexity: simple -->
<!-- ui-impact: none -->
<!-- constraint-domains: semantic-board -->

## Why

打标 prompt 让模型**一次调用同时提取 event/person 两个类别 + keyword**，每个 event/person 还要输出 3–5 个 `auxiliary_labels`。这个 schema 已经让模型反复出错：

- 20 分钟内 184183 一篇文章连续 3 次 `parse extraction result failed: json: cannot unmarshal object into Go struct field rawMergedExtraction.event_person_tags`——模型把 `event_person_tags` 返回成了 object 而不是数组。
- `topic_tags` 里 person 只有 **861 个（4.9%）**，event 6,968（40%）、keyword 9,837（56%）。
- 这 861 个里 **385 个（44%）已经有同名 auxiliary 标签**；Top 10 person（黄仁勋/习近平/马斯克/…）**10/10 都有 auxiliary 对应**。辅助标签体系已经覆盖人物叫法。
- 现有 person 标签里还混入 `英伟达`（公司）这类误分类，说明该类别识别本身不稳定。

## What Changes

- **两条打标路径的 prompt 同步收敛**：`buildMergedExtractionPrompt()`（单篇合并提取）与 `buildSectionExtractionPrompt()`（聚合型文章栏目片提取）都去掉 person 类别与「person 可留空」等规则，只保留 event + keyword。
- **两个 JSON Schema 的 category 描述同步**（它们会随结构化输出请求发给模型）：merged 的 `event_person_tags[].category` → `必须为 event`（与 `keyword_tags[].category` 的 `必须为 keyword` 对称），section 的 `tags[].category` → `event 或 keyword`；`description` 字段的 `person 可留空` 删除。
- **不改** JSON 字段名 `event_person_tags`（只放 event），保证解析层、存量数据、前端与既有测试零改动。
- **不改** 解析层对 person 的兼容：模型偶尔仍返回 person 时照旧接受（不报错、不丢数据），退役只在 prompt 层生效。
- **不动** 存量 861 个 person 标签，也不动 `story_fingerprint` 的 `person:` subject_key 前缀——后者来自 **event 标签**的 `metadata.event_fingerprint.subject_key`（表示「事件主体是人」），与本类别无关。
- `person_metadata_backfill` 保留：只处理存量 person 标签，跑完自然停。

## Capabilities

### New Capabilities

（无）

### Modified Capabilities

- `tagging-domain`: 打标提取的类别集合由 event/person/keyword 收敛为 event/keyword（单篇与聚合两条路径一致）；person 不再作为提取目标。

## Impact

- **代码**：`backend-go/internal/tagmanagement/service/core/extractor_enhanced.go`（merged prompt + merged schema）、`extractor_section.go`（section prompt + section schema）、`exemplar_guard.go`（两条路径的示例词护栏同步）、`extractor_enhanced_test.go` / `extractor_section_test.go`（回归闸与 golden）
- **文档**：`docs/reference/flow/semantic-board.md`「业务约束与不变量」补一条红线
- **行为**：新文章不再产生 person 类 topic tag；已有 person 标签照常展示/归并
- **数据**：无迁移、无删除
- **不影响**：JSON 字段名、解析容错、API、前端、日报、`story_fingerprint` 的 subject_key 前缀
