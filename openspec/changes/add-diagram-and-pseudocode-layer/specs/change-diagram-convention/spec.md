## MODIFIED Requirements

### Requirement: 图表达制品的固定位置与性质

change 的图 SHALL 写在 change 根的 `design-diagram.md`。该文件是非 schema 制品，MUST NOT 进入 schema DAG，不参与 `openspec validate` 的 delta 解析。

它的**存在与否**由 `diagrams` marker 决定并受归档门禁检查（形式三项，见「diagrams 门禁只查形式」）；它的**内容质量**（图类型选择、准确性、语法）MUST NOT 进入门禁。

已归档 change 允许补档，补档 MUST 只新增该文件，不修改 proposal / design / specs / tasks 原制品。

#### Scenario: 新 change 创建图文档

- **WHEN** 一个 change 声明 `diagrams: full` 且需要图表达
- **THEN** 在 change 根创建 `design-diagram.md`，不改 `.openspec.yaml`、不改 schema 制品形状

#### Scenario: 归档 change 补图

- **WHEN** 给一个已归档 change 补样板图
- **THEN** 只新增 `design-diagram.md`，proposal / design / specs / tasks 保持原样（§12 补档范畴，只增不改）；补档不追溯 marker 要求

### Requirement: 三类图的适用判据

`design-diagram.md` SHALL 按 `diagrams` marker 的档位决定是否存在；一旦存在，MUST 只画对本次 change 有意义、且内容确属该 change 事实的图：

- **ER 图**（`erDiagram`）：change 涉及表结构、字段、关系或索引变更
- **用户故事 / 顺序图**（`sequenceDiagram`）：一次用户动作跨三层及以上调用，或失败点取决于消息先后顺序
- **状态流转**（`stateDiagram-v2`）：存在明确的合法状态集合与非法跃迁
- **用例图**（`flowchart` + subgraph，左角色右用例）：change 涉及多角色、多用例，或大型 SRS 的功能分解
- **功能路径**（`flowchart`）：存在两个以上分支，或存在失败回退路径

判据 SHALL NOT 作为「该不该有图」的闸门（见「图的存在由 marker 决定」）；它只回答「该画哪几类」。

#### Scenario: 选择图类型

- **WHEN** 为 change 写图
- **THEN** 按内容逐类对判据，画成立的图；MUST NOT 为凑齐类型而画

#### Scenario: 图与文字分工

- **WHEN** `design-diagram.md` 与 `design.md` 同时存在
- **THEN** `design.md` 的 Decision 仍保留「结论 / 落点 / 取舍」三行，图 MUST NOT 替代它们；图只承担结构、路径、状态、时序，约束与取舍仍由文字承担

### Requirement: 图块的最小可读约束

每个图的代码块语言标记 MUST 为 `mermaid`。每个代码块前一行 SHALL 写明该图回答的问题，代码块后 SHALL 用一句话说明「图不表达的约束」（例如未画出的失败分支、维度限制、性能前提）。

**图 SHALL 在对话中呈现**：写图的人 SHALL 在生成或修改图后，把 mermaid 源码块直接贴进对话回复里，SHALL NOT 只落盘不贴。理由：归档是为了日后可查，贴出来才是给当下的决策者看——只看文件等于没画。

#### Scenario: 图块格式合规

- **WHEN** 检查 `design-diagram.md`
- **THEN** 每个代码块标注为 `mermaid`，块前有问题行、块后有约束行

#### Scenario: 图在对话中可见

- **WHEN** 新增或修改 `design-diagram.md` 的图
- **THEN** 同一回合的回复里 SHALL 出现该图的 mermaid 源码块；只有文件改动、对话里没有图，视为未完成

#### Scenario: 不做自动门禁

- **WHEN** change 归档
- **THEN** 不存在针对 mermaid 语法或图准确性的硬门禁；语法由渲染现场暴露（对话里贴出后不渲染即为错），语义由看图的人判断

## ADDED Requirements

### Requirement: 图的存在由 marker 决定，默认画

change 的 `proposal.md` 头部 SHALL 声明二值 marker 之一：

```
<!-- diagrams: full(白话理由) -->
<!-- diagrams: none(白话理由) -->
```

- `full`：change 根 SHALL 存在 `design-diagram.md`，且其中 SHALL 至少含一个 ` ```mermaid ` 代码块
- `none`：括号内理由 MUST 非空，且 SHALL 用大白话说明为什么不需要图（如「只改措辞」「无表结构变更」「无状态机」「单层调用无时序可言」）
- `full`：括号内理由同样 MUST 非空，用来说明这个 change 的哪一部分需要图
- marker 缺失：新 change（有 `.openspec.yaml`）缺失 marker SHALL 使归档门禁判红

**不判理由的正确性**：机器只判「有没有写理由」，理由成立与否归 review 与看图的人。

#### Scenario: full 档有一块图

- **WHEN** change 声明 `<!-- diagrams: full(评测口径变更 + 涉及三张表的字段增删) -->`
- **THEN** `design-diagram.md` SHALL 存在且至少含一个 mermaid 代码块

#### Scenario: none 档理由非空

- **WHEN** change 声明 `<!-- diagrams: none(只改文档措辞，无表结构、无状态机、无跨层调用) -->`
- **THEN** 归档门禁通过；理由为空或缺失括号内容则判红（`full` 档同理）

#### Scenario: marker 缺失

- **WHEN** 新 change 的 proposal.md 头部无 `diagrams` marker
- **THEN** 归档门禁 SHALL 判红并提示「写 full(理由) 或 none(理由)」

### Requirement: 伪代码成为 design.md 的强制节

`design.md` SHALL 包含 `## 核心逻辑（伪代码）` 节，或在 `## 核心逻辑（伪代码）` 节内写一句 `N/A：<理由>`。

该节 SHALL 覆盖三件事：

1. 关键数据结构（签名级，不写字段类型细节）
2. 主流程控制流（分支与循环的骨架）
3. 失败与边界分支（空集合、并发、重复调用、超时降级）

粒度 SHALL 停在「看伪代码能发现问题」为止：能看出漏了空集合分支、两次查询可合并、顺序会读到旧值，即为合格；写到可直接编译为过细。

#### Scenario: 伪代码节存在

- **WHEN** 检查一个有业务逻辑变更的 change 的 `design.md`
- **THEN** 存在 `## 核心逻辑（伪代码）` 节且非空

#### Scenario: 无逻辑变更时写理由

- **WHEN** change 只改文档、配置值或命名，无控制流变化
- **THEN** 该节写 `N/A：<理由>`，不虚构伪代码

### Requirement: diagrams 门禁只查形式

归档门禁 SHALL 检查三件眼睛不可靠的事：

1. `diagrams` marker 存在且值为 `full` / `none` 之一
2. `none` 档理由非空
3. `full` 档下 `design-diagram.md` 存在且含至少一个 mermaid 代码块

门禁 SHALL NOT 检查：图的类型选择是否恰当、图画得对不对、伪代码是否准确、`none` 的理由是否成立。

已归档 change MUST NOT 被追溯要求补 marker（无 marker 视为 legacy，不阻断）。

#### Scenario: 门禁只判形式

- **WHEN** 归档一个 `full` 档 change
- **THEN** 门禁确认图文件与代码块存在即放行；图内容质量不进门禁
