# auxiliary-label — retire-heuristic-tag-fallback delta

## MODIFIED Requirements

### Requirement: Tag 提取单次调用输出 event/person 与 keyword 双数组

系统 SHALL 以单次 LLM 调用完成 tag 提取，输出同时包含 event/person 标签数组与 keyword 标签数组（schema 双数组约束）。event/person 标签 SHALL 携带 3-5 个辅助标签；keyword 标签 SHALL 携带 description，不输出 auxiliary_labels。

**【2026-11-12 本 change 起生效】单次调用（含既有重试上限）整体失败时，系统 SHALL 直接判失败，SHALL NOT 回退到 heuristic keyword 提取**——失败错误上抛给编排层，使打标任务进入队列重试，文章保持无标签。调用成功但 event/person 数组为空或全部未通过校验时，系统 SHALL 仅保留 keyword 产出并记录缺失原因，不视为失败。调用成功但 keyword 数组为空或全部未通过校验时，系统 SHALL NOT 生成 heuristic keyword 兜底（原「使用 heuristic keyword 作为展示兜底」条款与实现脱节——合并提取的 `mergeExtractedTags` 从未实现该兜底，本 change 一并作废）。

合并后的标签总数 SHALL 不超过 5 个；keyword 数组最多保留 3 个标签。若同一 slug 同时出现在多个 category 中，系统 SHALL 按 person > event > keyword 的优先级保留更具体的分类，并丢弃低优先级重复项。

#### Scenario: event/person 数组为空但 keyword 有产出

- **WHEN** 单次调用成功返回，event/person 数组为空或全部未通过校验，keyword 数组有产出
- **THEN** 系统 SHALL 保留 keyword 标签，记录 event/person 数组缺失原因，不生成 event/person 标签，且不触发任何规则词兜底

#### Scenario: keyword 数组为空但 event/person 有产出

- **WHEN** 单次调用成功返回，keyword 数组为空或全部未通过校验，event/person 数组有产出
- **THEN** 系统 SHALL 保留 event/person 标签，SHALL NOT 生成 heuristic keyword 作为展示兜底（原「使用 heuristic keyword 作为展示兜底」条款作废——规则词兜底已整体退役）

#### Scenario: 双数组间去重

- **WHEN** 单次调用成功返回，同一 slug 同时出现在 event/person 数组与 keyword 数组
- **THEN** 系统 SHALL 按 person > event > keyword 的优先级保留更具体的分类，丢弃低优先级重复项

#### Scenario: 单次调用整体失败不回退 heuristic

- **WHEN** 单次提取调用重试耗尽仍失败
- **THEN** 系统 SHALL 返回失败并把错误上抛给编排层（不产出任何标签、不回退 heuristic keyword 提取），使打标任务标记为失败并进入队列重试

#### Scenario: mono 文章提取调用数为 1

- **WHEN** 一篇 mono 文章走完整打标流程
- **THEN** 提取 SHALL 只发起 1 次 LLM 调用（双数组在同一次响应中返回），无额外调用
