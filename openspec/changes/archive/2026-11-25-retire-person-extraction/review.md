# Review — retire-person-extraction

> 格式契约：`openspec/specs/review-trace-gate`。审查方式：子线程聚焦 review（`change-review` 档 + `minimax-cn/MiniMax-M3.1-Flash:max`；原计划 `zai-coding-cn/glm-5.4` 因额度窗口耗尽改派），按 §0.6「review 执行要点」执行。

## 审查范围

- 制品：`proposal.md` / `design.md`（D1–D3）/ `specs/tagging-domain/spec.md` / `tasks.md`
- 代码：`extractor_enhanced.go`（merged prompt + merged schema）、`extractor_section.go`（section prompt + section schema）、`exemplar_guard.go`（示例词护栏）、`extractor_enhanced_test.go` / `extractor_section_test.go`
- 文档：`docs/reference/flow/content-enrichment.md`（首轮）、`docs/reference/flow/semantic-board.md`（终版）

## 审查结论

High 1、Medium 4、Low 2。核心实现（两条提取路径的 prompt 收敛、解析层 person 兼容原样保留、字段名 `event_person_tags` 不动）达标。

---

### R-1 2026-11-25【High】merged schema 的 `event_person_tags[].category` 描述写成「event 或 keyword」，与 prompt 的「不要串放」正面矛盾

- **状态**：已修复
- **位置**：`extractor_enhanced.go` `mergedTagExtractionSchema()` 的 `event_person_tags[].category.Description`
- **问题**：merged 路径是双数组，`event_person_tags` 只该放 event。描述写成「event 或 keyword」相当于**允许 keyword 串放进 event 数组**，正好招回本 change 要消灭的整篇解析失败（184183 就是结构发散）。这与 `semantic-board.md:56` 既有红线「提取 prompt 的输出格式指令 MUST 与下发 schema 一致，两边不可互相串用」相撞。
- **证据**：同一 schema 的 `keyword_tags[].category` 写的是「必须为 keyword」（对称），只有 event 数组被放宽。
- **建议**：改为 `必须为 event`（或 `event`）；section 单数组的「event 或 keyword」保持不变。
- **处置**：已改为 `必须为 event`；design D1 拆开写两条路径的 schema 描述；spec 增补 Scenario「schema 类别描述不招回串放」。

### R-2 2026-11-25【Medium】proposal.md 未随实现同步：域声明仍是 ai-summary，What Changes / Impact 只写 merged 路径与一个文件

- **状态**：已修复
- **位置**：`proposal.md` 头部 `constraint-domains`、What Changes、Impact
- **问题**：实现已改两条路径 + 两个 schema + 示例词护栏 + 文档，proposal 仍只描述 merged 路径与单文件；域声明 ai-summary 也不覆盖打标 prompt 的真实约束域。
- **建议**：`constraint-domains` 改 `semantic-board`；What Changes 补 section 路径与 schema 描述；Impact 补 `extractor_section.go` / `exemplar_guard.go` / 文档行。
- **处置**：已全部同步。

### R-3 2026-11-25【Medium】新增红线落在 `content-enrichment.md`，但该文档辖区不含 `internal/tagmanagement/`

- **状态**：已修复
- **位置**：`docs/reference/flow/content-enrichment.md` 第 17 条
- **问题**：`content-enrichment.md` 的 `doc-impact-applies` 只覆盖 `internal/reader/` 等路径，**不含 `backend-go/internal/tagmanagement/`**——这条打标红线对真正会改它的路径既不会被注入、也不会进路由清单（JIT 静默失效，无门禁拦）。正确的权威宿主是 `semantic-board.md`：头部辖区含 `internal/tagmanagement/`，「业务约束与不变量」节已合规，第 56 行还已有同类纪律（prompt 输出格式 MUST 与 schema 一致）。
- **建议**：红线移到 `semantic-board.md`「业务约束与不变量」节。
- **处置**：已从 `content-enrichment.md` 撤回，作为第 39 条写入 `semantic-board.md`（紧接第 38 条 ANN 红线）。

### R-4 2026-11-25【Medium】spec delta 只约束「合并提取 prompt」，实现已改两条路径 → 契约覆盖不全

- **状态**：已修复
- **位置**：`specs/tagging-domain/spec.md` Requirement 正文与 Scenario
- **问题**：Requirement 只写 merged 路径，section（聚合）路径无契约覆盖，未来改 section prompt 不受约束。
- **建议**：Requirement 显式点名两条路径；Scenario 的 WHEN 覆盖两者；补 schema 描述一致性场景。
- **处置**：已补。

### R-5 2026-11-25【Medium】测试断言强度偏弱：靠 4 个固定短语，新增 person 措辞不会红

- **状态**：已修复
- **位置**：`TestMergedExtractionPromptExcludesPerson`（新增）/ `TestSectionExtractionPromptIncludesFusedRules`
- **问题**：只断言 4 个固定短语不在 prompt 里，任何人加一句新的 person 说明或示例词都不会触发。
- **建议**：加与措辞无关的强断言——剔除字段名后整篇不得出现 `person`。
- **处置**：merged 用 `strings.ReplaceAll(prompt, "event_person_tags", "")` 后断言无 `person`；section 无字段名可豁免，直接断言整篇无 `person`。

### R-6 2026-11-25【Low】off-prompt golden 测试注释仍称「与上线前逐字一致」，基线已迁移

- **状态**：已修复
- **位置**：`extractor_enhanced_test.go` / `extractor_section_test.go` 的 golden 注释
- **处置**：注释改为「基线以本次（retire-person-extraction 收敛 person 之后）为起点；措辞变更须同步本 golden」。

### R-7 2026-11-25【Low】两条路径的示例词护栏同步删了 `Sam Altman` / `李飞飞`，属既有防线收窄

- **状态**：已采纳（必要同步，非缺陷）
- **位置**：`exemplar_guard.go` 的 `mergedPromptExemplarLabels` / `sectionPromptExemplarLabels`
- **说明**：这两项是「示例词集合必须原文出现在 prompt 中」的防漂移守卫，prompt 删除 person 示例后集合必须同步，否则守卫自身变红。副作用是 `guardExemplarLeakage` 不再把这两个词当示例词拦截——但既然 prompt 不再要求 person，模型偶发吐出的 person 条目本就走兼容接受路径（design D2），不构成遗漏。后续若要恢复对这两个词的泄漏拦截，应作为独立议题而非本 change 的附带项。

---

## 覆盖清单（子线程实际核过的不变量）

| # | 检查 | 结果 |
| --- | --- | --- |
| 1 | merged prompt 内 person 出现次数（剔除字段名后） | 0 次（prompt 1385 runes，未超 1500 上限） |
| 2 | section prompt 内 person 出现次数 | 0 次 |
| 3 | 解析层 `validateCategory` / `categoryPriority` / `routeLegacyTagObjects` / `unmarshalMergedExtraction` 是否被改 | `git diff` 确认零改动（design D2 保持兼容） |
| 4 | `event_person_tags` 字段名 | 保留（merged 正例 JSON 与 schema Required 均未动） |
| 5 | 两个 schema 的 category / description / auxiliary_labels 描述 | merged 收窄为 event / keyword 各自专属；section 单数组为「event 或 keyword」 |
| 6 | 示例词护栏与 prompt 同步 | 两个 `*PromptExemplarLabels` 删掉 person 示例，`TestExemplarLabelsCoverPrompts` 绿 |
| 7 | 两条 off-prompt golden | 首句锚点同步；schema 字段集与 Required 未动 |
| 8 | 文档红线宿主 | `semantic-board.md` 第 39 条（辖区含 tagmanagement） |
| 9 | `guardExemplarLeakage` 行为 | 未改动，仅示例词集合同步（见 R-7） |
