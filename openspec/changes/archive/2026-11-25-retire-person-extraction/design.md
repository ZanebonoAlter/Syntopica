## Context

- 打标走 `extractor_enhanced.go` 的单次合并调用：`buildMergedExtractionPrompt()` 给 prompt，`unmarshalMergedExtraction()` 解析，`rawMergedExtraction` 的两个字段是 `event_person_tags` / `keyword_tags`。
- 模型侧输出结构由 prompt 决定（没有 JSON schema enum 约束类别，`unwrapSchemaEnvelope` 只处理 schema 骨架回显）。
- 解析层 `validateCategory()` 把未知类别映射到 keyword，`categoryPriority()` 给 person 一个优先级档位——两者保留（存量 person 标签与偶发返回都要能用）。
- 真实库：person 861（活跃）、event 6,968、keyword 9,837；person 中 385 个有同名 auxiliary 标签，Top 10 全覆盖。
- `story_fingerprint.go` 的 `validSubjectKeyPrefixes` 含 `person`，但那是 **event 标签** `metadata.event_fingerprint.subject_key` 的前缀白名单（`person:trump`），与 topic_tags.category 无关。
- `person_metadata_backfill.go` 按 `category='person'` 找缺 metadata 的标签补结构（LLM，每批 100、间隔 500ms），退役后只处理存量。

## Goals / Non-Goals

**Goals:**

- prompt 不再要求提取 person，减少同一次调用要守的规则数，降低 `event_person_tags` 结构错误率。
- 新文章不再产生 person 类 topic tag。

**Non-Goals:**

- 不改 JSON 字段名 `event_person_tags`（改名会牵动解析、测试、前端类型）。
- 不删除存量 861 个 person 标签，不改其 status，不做归并。
- 不改解析层的 person 兼容（偶发返回照旧接受）。
- 不动 `story_fingerprint` 的 `subject_key` 前缀白名单、`person_metadata_backfill`、前端 person 图标/配色。
- 不引入「按开关动态切换提取类别」的配置项（YAGNI：单用户、需求明确）。

## Decisions

### D1. 只改 prompt 文本与 schema 描述，不改契约

- **结论**：**两条打标路径同步收敛**：
  - 单篇 merged 路径 `buildMergedExtractionPrompt()`：「【event/person 数组】」→「【event 数组】」，删 person 定义行、`每个 event/person 标签` 措辞、`person 可留空` 尾巴，正/反例中的 person 提法全部清除。
  - 聚合 section 路径 `buildSectionExtractionPrompt()`：首句「event、person、keyword 三类标签」→「event、keyword 两类标签」，删整个 person 定义块（含 `Sam Altman`/`李飞飞` 示例）、`event/person 标签必须输出 auxiliary_labels`、`person：可留空`、影子段 `person/keyword 不输出`。
  - 两个 JSON Schema 里的**描述字符串**同步（它们会随结构化输出请求发给模型）：`category` 的 `"event、person 或 keyword"` / `"event 或 person"` → `"event 或 keyword"`，`description` 字段的 `person 可留空` 删除，`auxiliary_labels` 的 `event/person 必填` → `event 必填`。
  - `event_person_tags` 字段名保持原样（只放 event）。
- **落点**：`extractor_enhanced.go:buildMergedExtractionPrompt` / `extractor_section.go:buildSectionExtractionPrompt` / 两个 schema 构造函数
- **取舍**：只改 merged 被否——聚合型文章仍会持续产出 person，两条路径行为不一致，退役不彻底；改字段名为 `event_tags` 被否——要同步改 `rawMergedExtraction`、`unwrapSchemaEnvelope` 的 key 探测、schema 回显测试、前端类型与所有 fixture，收益只是名字好看；加配置开关被否——单用户场景没有第二套需求。

### D2. 解析层保留 person 兼容

- **结论**：`validateCategory`、`categoryPriority`、`rawExtractedTag` 的 person 分支一律保留；模型偶发返回 person 时照常入库（不报错、不静默丢弃）。
- **落点**：`extractor_enhanced.go:validateCategory` / `:categoryPriority` / `:routeLegacyTagObjects`
- **取舍**：解析层硬拒 person 被否——会把「模型没听 prompt」升级成「整篇解析失败」，正好是这次要消除的失败模式；过滤掉 person 条目也会静默丢数据。

### D3. 测试以「两条 prompt 都不含 person 提取要求」为断言

- **结论**：
  - 新增 `TestMergedExtractionPromptExcludesPerson`：断言 merged prompt 不含 `person（人物）` / `person 可留空` / `每个 event/person 标签` / `event/person 数组只放`，仍含 `event（事件）`、`keyword（关键词）`、`event_person_tags`。
  - `TestSectionExtractionPromptIncludesFusedRules` 的 `require.Contains(prompt, "person（人物）")` 反转为 `require.NotContains`（这是两条路径同步的回归闸）。
  - `TestEnhancedExtraction_OffPromptAndSchemaUnchanged` / `TestSectionExtraction_OffPromptAndSchemaUnchanged` 的 prompt 首句 golden 同步新措辞。
  - `mergedPromptExemplarLabels` / `sectionPromptExemplarLabels` 删 `Sam Altman`、`李飞飞`（exemplar 与 prompt 必须同步的既有守卫）。
  - 既有解析测试（mock 返回含 person）保持不变，继续钉住 D2 的兼容性。
- **落点**：`extractor_enhanced_test.go` / `extractor_section_test.go` / `exemplar_guard.go`
- **取舍**：改既有 fixture 去掉 person 被否——那会顺带删掉 D2 的回归保护。

## Risks / Trade-offs

- [模型惯性仍偶发返回 person] → 解析层接受（D2），不会失败；新数据里会稀疏出现 person，属可接受噪声。
- [辅助标签覆盖不足的冷门人物] → 该类人物本来也只靠 person 标签存在，没有下游消费（`story_fingerprint` 用 event 的 subject_key）；缺失不影响任何视图。
- [prompt 变短后提醒弱化] → 只删类别定义，`auxiliary_labels` / description / 反例等对 event 与 keyword 的约束原样保留。

## Migration Plan

1. 上线：prompt 变更随进程重启生效，无需数据迁移。
2. 回滚：还原 prompt 文本即可，存量数据无影响（person 标签从未被删）。
3. 存量 person 标签：保留；若后续确认完全不需要，再单独评估归并/归档。

## Open Questions

无。
