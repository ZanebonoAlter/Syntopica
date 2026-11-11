## ADDED Requirements

### Requirement: 事件主体稳定身份说明随同一次提取产出

事件标签的提取 SHALL 在产出该标签的**同一次模型调用**内，额外产出该标签主体的原文提及（`subject`）与**稳定短身份说明**（`subject_desc`）；SHALL NOT 为该说明发起额外模型调用。该字段 SHALL **仅在影子模式下请求**：关闭态下第一轮的提示与 schema SHALL 与上线前逐字一致，SHALL NOT 新增字段输出或模型请求。

- 稳定短身份说明 SHALL 只描述主体自身的身份（类别 / 国别 / 角色 / 所属组织等），SHALL NOT 包含当次事件、动作、对象或时间；第一轮提示 SHALL 显式声明该约束。
- 稳定短身份说明 SHALL NOT 写入事件指纹 `event_fingerprint`，SHALL NOT 改变既有指纹完整性判定与关键词一致性校验结论；SHALL 落在独立 metadata 键。
- `keyword` 与 `person` 类别的标签 SHALL NOT 产出稳定短身份说明。
- 稳定短身份说明为空时 SHALL 照常产出标签并照常入库；SHALL NOT 因缺该说明降级、报错或丢弃标签。

#### Scenario: 影子态同一次调用产出主体原文与稳定短身份说明

- **WHEN** 影子模式下某栏目片返回 3 个 event 标签
- **THEN** 3 个标签 SHALL 各自带上主体原文与稳定短身份说明，该片的模型调用次数 SHALL 仍为 1
- **AND** `event_fingerprint` SHALL 与未产出该说明时结构相同，指纹完整性与关键词一致性校验结论 SHALL 不变
- **AND** keyword 与 person 类别的标签 SHALL NOT 携带该说明
- **AND** 某 event 标签的稳定短身份说明为空时，该标签 SHALL 照常入库，SHALL NOT 因缺该说明报错或丢弃

#### Scenario: 关闭态第一轮提示与字段输出不变

- **WHEN** 系统以关闭态（默认）运行提取
- **THEN** 第一轮提示与 schema SHALL 与上线前逐字一致
- **AND** SHALL NOT 因本能力新增字段输出或模型请求
