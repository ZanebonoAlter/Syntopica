# tagging-domain Delta

## ADDED Requirements

### Requirement: Event 标签描述的时间锚纪律

event 类标签的 LLM 描述生成 SHALL 遵守时间锚纪律：描述中的事件发生时间 MUST 以所给文章上下文（含文章发布日期）为参照，MUST NOT 引入文章上下文中不存在的时间信息（包括模型先验知识中的历史事件时间）。标签抽取与描述生成环节的 prompt 示例 MUST 使用时间中性或虚构的事件示例，MUST NOT 使用自带具体历史时间锚的真实事件作为示例。

#### Scenario: 新建 event 标签时描述时间锚与文章日期一致

- **WHEN** 一篇发布于 D 日的文章触发新建 event 标签，且文章内容未提及其它时间信息
- **THEN** 该标签 description 中的事件时间表述与 D 日（或文章明确给出的相对时间）一致，不出现与文章无关的历史年份

#### Scenario: 描述生成不引入先验知识时间

- **WHEN** 标签名称与某个著名历史事件同名（如真实世界曾发生过的袭击/发布/政策事件），而触发文章讲的是当日语境的其它内容
- **THEN** 生成的 description 以触发文章上下文为准，MUST NOT 复述该历史事件的发生时间

#### Scenario: prompt 示例不含真实历史时间锚

- **WHEN** 检查标签抽取 prompt 与 event 描述生成 prompt 的示例文本
- **THEN** 示例中使用时间中性或虚构的事件示例，不出现"真实事件名 + 具体历史年月"的组合

### Requirement: 存量病态时间锚标签的可修复性

系统 SHALL 提供一次性存量修复能力：筛出"事件主体时间锚早于标签创建时间至少一年"的 event 标签候选，经确认后按该标签首挂文章（或指定文章）的上下文重新生成 description。修复 SHALL 支持按标签 ID 精准指定；修复操作 MUST 幂等（重复执行不产生重复副作用），且 MUST NOT 触碰非候选标签的 description。

#### Scenario: 筛选存量病态候选

- **WHEN** 对存量 event 标签执行扫描
- **THEN** 输出"事件主体时间锚早于创建时间 ≥1 年"的标签清单（含标签 ID、label、创建时间、当前 description），合法历史参照（如"创某年以来新高"对比句）不在强制修复范围内

#### Scenario: 指定标签重生成描述

- **WHEN** 对清单中的指定标签 ID 执行修复
- **THEN** 该标签 description 基于其首挂文章的发布日期与摘要上下文重新生成，时间锚与新上下文一致；修复后再次执行扫描，该标签不再出现在候选清单中

#### Scenario: 修复不影响健康标签

- **WHEN** 修复流程执行
- **THEN** 时间锚合法（与创建时间同期，或为合法历史参照）的标签 description 保持原样
