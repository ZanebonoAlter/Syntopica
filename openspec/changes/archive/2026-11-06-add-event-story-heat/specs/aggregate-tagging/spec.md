## MODIFIED Requirements

### Requirement: 逐片融合提取标签
聚合路径 SHALL 对每个栏目片发起恰好 1 次 LLM 调用，使用 event/person/keyword 三分类融合的单一 prompt（每片产出上限 4 个标签候选），SHALL NOT 对单篇文章发起超过 8 次提取调用。

每个 `category='event'` 的标签 SHALL 在同一次调用内同时产出结构化指纹 `subject` / `subject_key` / `action` / `object`，`occurred_at` MAY 产出。指纹字段 SHALL NOT 触发额外调用，SHALL NOT 由第二条提取链路补取。`keyword` 与 `person` 标签 SHALL NOT 产出这几项。

`subject_key` SHALL 为归一化实体键，格式 `类型:规范名`（类型限于 `person` / `company` / `org` / `product` / `other`，规范名用小写英文或拼音），由模型在同一次调用内完成别名归并（如 Trump / 川普 / 特朗普 → `person:trump`）。`subject_key` SHALL NOT 依赖 `topic_tags.aliases` 生成——实测 person/event 的 aliases 覆盖率为 0%、keyword 仅 7.3%。

指纹 SHALL 与标签的 `metadata.event_keywords` 做一致性校验：`subject` 与 `object` 均 SHALL 能在关键词集合中找到（允许包含关系与别名展开）。校验不通过或关键词缺失时，标签 SHALL 照常入库并参与既有流程，指纹 SHALL 标记为不可信并记录 warning（含标签名与不一致字段）。

单片调用失败时 SHALL 记录 warning 并跳过该片继续处理其余片，SHALL NOT 因单片失败丢弃整篇或整篇重试。

单个标签的 auxiliary_labels 校验失败或 keyword description 缺失 SHALL 降级处理（丢弃校验不过的部分、保留其余标签），SHALL NOT 因单标签问题让整片候选报废；每次降级 SHALL 记录 warning（含标签名与原因）。JSON 整体解析失败 SHALL 维持原有重试语义（重试 3 次后跳过该片）。

#### Scenario: 每片一次调用产出候选

- **WHEN** 切片器产出 5 个栏目片
- **THEN** 发起 5 次融合 prompt 提取调用，每次调用的输入仅含对应片文本

#### Scenario: 同一次调用产出事件指纹

- **WHEN** 某片返回 3 个 event 标签与 1 个 keyword 标签
- **THEN** 3 个 event 标签 SHALL 各自携带 subject/subject_key/action/object，keyword 标签 SHALL NOT 携带；该片调用次数 SHALL 仍为 1

#### Scenario: 跨语言别名归一为同一主体键

- **WHEN** 两次提取分别返回 subject=`特朗普` 与 subject=`Trump`
- **THEN** 两次的 `subject_key` SHALL 相同（如 `person:trump`），SHALL NOT 因原文字面不同而产出不同键

#### Scenario: 指纹校验失败不阻断

- **WHEN** 某 event 标签的 subject 不在 `event_keywords` 中且无法通过别名匹配
- **THEN** 该标签 SHALL 照常入库，指纹标记不可信并记录 warning，同片其余标签不受影响

#### Scenario: 关键词缺失时指纹不可信

- **WHEN** 某 event 标签的 `metadata.event_keywords` 为空或缺失
- **THEN** 该标签 SHALL 入库，指纹 SHALL 标记不可信，SHALL NOT 因缺少校验依据丢弃标签

#### Scenario: 单片失败不影响其余片

- **WHEN** 5 片中第 3 片的 LLM 调用重试耗尽后仍失败
- **THEN** 第 1、2、4、5 片的标签候选正常进入 reduce 阶段，失败信息记录在日志

#### Scenario: 单标签 aux 校验失败降级保留

- **WHEN** 某片返回 3 个标签，其中 1 个 event 标签的 auxiliary_labels 数量为 2（不满足 3-5 条）
- **THEN** 该 event 标签仍入库（无 aux 锚点），同片其余 2 个标签不受影响，降级记录在 warning 日志

#### Scenario: keyword 缺 description 单标签跳过

- **WHEN** 某片返回的 keyword 标签 description 为空
- **THEN** 该 keyword 标签被跳过并记录 warning，同片其余标签正常进入 reduce 阶段
