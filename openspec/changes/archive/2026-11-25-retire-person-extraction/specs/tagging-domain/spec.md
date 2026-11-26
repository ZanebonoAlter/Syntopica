# tagging-domain Delta

## ADDED Requirements

### Requirement: 打标提取类别收敛为 event 与 keyword

系统 SHALL 在打标提取 prompt 中只要求提取 `event`（事件）与 `keyword`（关键词），SHALL NOT 要求提取 `person`（人物）；该约束 SHALL 覆盖单篇合并提取与聚合栏目片提取两条路径。

#### Scenario: prompt 不再要求提取人物

- **WHEN** 构建单篇合并提取 prompt 或聚合栏目片提取 prompt
- **THEN** prompt SHALL 包含 event 与 keyword 的提取规则
- **AND** prompt SHALL NOT 包含人物类别的提取要求（如「person（人物）」「每个 event/person 标签」）

#### Scenario: schema 类别描述不招回串放

- **WHEN** 构建单篇合并提取的结构化输出 schema
- **THEN** `event_person_tags[].category` 的描述 SHALL 限定为 event（与 `keyword_tags[].category` 的 keyword 对称）
- **AND** 聚合路径单数组的 `tags[].category` 描述 SHALL 为「event 或 keyword」

#### Scenario: 模型仍返回人物时的兼容

- **WHEN** 模型输出中仍带有 `category: "person"` 的条目
- **THEN** 解析 SHALL 成功（按既有类别校验路径入库），整篇提取 SHALL NOT 因该条目失败
- **AND** 模型 SHALL 继续通过 `event_person_tags` 字段返回 event 条目（字段名保持，维持解析契约）

#### Scenario: 存量人物标签与事件主体前缀不受影响

- **WHEN** 打标 prompt 变更上线后
- **THEN** 既有 `category='person'` 标签 SHALL 保留原状态与引用（不删除、不改状态、不自动归并）
- **AND** 事件指纹解析 SHALL 继续接受以 `person:` 开头的 `subject_key`（该前缀描述「事件主体是人」，取自 event 标签的 `metadata.event_fingerprint`）
