<!-- domain: dev -->

# change-diagram-convention Specification

## Purpose

约束 change 制品中 mermaid 图的位置、类型选择与文字分工，让用户故事核心路径、状态流转和跨层时序在评审与回溯时可以直接看见，而不必从长段落里重建。

## Requirements

### Requirement: 图表达制品的固定位置与性质

change 需要图表达时 SHALL 在 change 根创建 `design-diagram.md`。该文件是非 schema 制品，MUST NOT 进入 schema DAG，不参与 `openspec validate` 的 delta 解析，也不阻断 apply / archive 门禁。已归档 change 允许补档，补档 MUST 只新增该文件，不修改 proposal / design / specs / tasks 原制品。

#### Scenario: 新 change 创建图文档

- **WHEN** 一个 change 涉及多步路径、状态机或跨层调用，且决定用图表达
- **THEN** 在 change 根创建 `design-diagram.md`，不改 `.openspec.yaml`、不改 schema 制品形状

#### Scenario: 归档 change 补图

- **WHEN** 给一个已归档 change 补样板图
- **THEN** 只新增 `design-diagram.md`，proposal / design / specs / tasks 保持原样（§12 补档范畴，只增不改）

### Requirement: 三类图的适用判据

`design-diagram.md` SHOULD 按需要包含以下三类图；一旦画，MUST 符合对应判据，且只在判据成立时画：

- **用户故事核心路径**（`flowchart`）：存在两个以上分支，或存在失败回退路径
- **状态流转**（`stateDiagram-v2`）：存在明确的合法状态集合与非法跃迁
- **顺序图**（`sequenceDiagram`）：跨三层及以上调用，且失败点取决于先后顺序

没有分支就不画 flowchart，没有状态集合就不画 stateDiagram，单层调用不画 sequenceDiagram。

#### Scenario: 选择图类型

- **WHEN** 为 change 写图
- **THEN** 逐类对照判据，只画判据成立的图，不为了凑齐三类而画

#### Scenario: 图与文字分工

- **WHEN** `design-diagram.md` 与 `design.md` 同时存在
- **THEN** `design.md` 的 Decision 仍保留「结论 / 落点 / 取舍」三行，图 MUST NOT 替代它们；图只承担路径、状态、时序，约束与取舍仍由文字承担

### Requirement: 图块的最小可读约束

每个图的代码块语言标记 MUST 为 `mermaid`。每个代码块前一行 SHALL 写明该图回答的问题，代码块后 SHALL 用一句话说明「图不表达的约束」（例如未画出的失败分支、维度限制、性能前提）。本约定不引入自动校验门禁；语法正确性由渲染抽查保证（前端 `markdown-mermaid-render` 或 `npx @mermaid-js/mermaid-cli`）。

#### Scenario: 图块格式合规

- **WHEN** 检查 `design-diagram.md`
- **THEN** 每个代码块标注为 `mermaid`，块前有问题行、块后有约束行

#### Scenario: 不做自动门禁

- **WHEN** change 归档
- **THEN** 不存在针对 `design-diagram.md` 的硬门禁；该文件缺失或图不准确不阻断归档，只在评审中作为质量项提出
