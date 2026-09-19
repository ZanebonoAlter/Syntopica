# auxiliary-label Delta

## REMOVED Requirements

### Requirement: Tag 提取拆分为 event/person 与 keyword 双分支调用

**Reason**: 双分支 = 同一篇 user prompt 送两遍 LLM，prefill 翻倍（本地 qwen 并发 2 瓶颈下是晚间批处理主要消耗之一）；单次调用双数组输出可覆盖同样语义，调用次数减半。

**Migration**: 下方 ADDED 需求"Tag 提取单次调用输出 event/person 与 keyword 双数组"承接全部保留契约（数量上限、去重优先级、keyword 直入、部分失败语义的连续演化）；入库侧（三级去重/防黑洞）不涉及本变更。

## ADDED Requirements

### Requirement: Tag 提取单次调用输出 event/person 与 keyword 双数组

系统 SHALL 以单次 LLM 调用完成 tag 提取，输出同时包含 event/person 标签数组与 keyword 标签数组（schema 双数组约束）。event/person 标签 SHALL 携带 3-5 个辅助标签；keyword 标签 SHALL 携带 description，不输出 auxiliary_labels。

单次调用（含既有重试上限）整体失败时，系统 SHALL 回退到 heuristic keyword 提取。调用成功但 event/person 数组为空或全部未通过校验时，系统 SHALL 仅保留 keyword 产出并记录缺失原因，不视为失败、不触发全量 heuristic 回退。调用成功但 keyword 数组为空或全部未通过校验时，系统 SHALL 使用 heuristic keyword 作为展示兜底，但 heuristic keyword 因缺少同次 LLM description，默认不进入辅助标签池。

合并后的标签总数 SHALL 不超过 5 个；keyword 数组最多保留 3 个标签。若同一 slug 同时出现在多个 category 中，系统 SHALL 按 person > event > keyword 的优先级保留更具体的分类，并丢弃低优先级重复项。

#### Scenario: event/person 数组为空但 keyword 有产出

- **WHEN** 单次调用成功返回，event/person 数组为空或全部未通过校验，keyword 数组有产出
- **THEN** 系统 SHALL 保留 keyword 标签，记录 event/person 数组缺失原因，不生成 event/person 标签，且不触发全量 heuristic 回退

#### Scenario: keyword 数组为空但 event/person 有产出

- **WHEN** 单次调用成功返回，keyword 数组为空或全部未通过校验，event/person 数组有产出
- **THEN** 系统 SHALL 保留 event/person 标签，并使用 heuristic keyword 作为展示兜底；heuristic keyword 默认不写入辅助标签池

#### Scenario: 双数组间去重

- **WHEN** 同一次调用的 event/person 数组输出 person tag "Sam Altman"，keyword 数组也输出 keyword tag "Sam Altman"
- **THEN** 系统 SHALL 保留 person tag，并丢弃重复的 keyword tag

#### Scenario: 单次调用整体失败回退 heuristic

- **WHEN** 单次提取调用重试耗尽仍失败
- **THEN** 系统 SHALL 回退 heuristic keyword 提取（结果 source=heuristic），失败原因记录在提取结果错误信息中

#### Scenario: mono 文章提取调用数为 1

- **WHEN** 一篇 mono 文章完成打标且 LLM 提取路径成功
- **THEN** `ai_call_logs` 中该文章的 tag 提取调用（operation=tagmanagement.extractor_enhanced）恰为 1 次
