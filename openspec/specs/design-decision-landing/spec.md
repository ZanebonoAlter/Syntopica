<!-- domain: dev -->

# design-decision-landing Specification

## Purpose
约束 design.md 的每条技术决策必须声明一个可解析的落点（代码位置、任务、Scenario 或无代码承接制品），并给出取舍；让「这项决策是否真的发生」在实现与审查阶段可核对，而不是停留在设计文档的叙述里。

## Requirements

### Requirement: Decision 必须携带可解析落点

design.md 中每条 Decision SHALL 在「落点」行声明四类之一：仓库内可解析的文件路径（可带符号）、存在的任务 id、存在的 Scenario 标题、或「无代码落点」并写明承接制品与理由。落点 MUST 能被仓库现状解析；空泛表述（「见 tasks」「后续补」「待定」）不视为落点。

#### Scenario: 落点指向真实代码

- **WHEN** 一条 Decision 的落点为 `backend-go/internal/foo/bar.go:Baz`
- **THEN** 该文件在仓库中存在，且 `Baz` 可在文件中找到，判定通过

#### Scenario: 落点指向任务

- **WHEN** 一条 Decision 的落点为任务 id（如 `2.3`）
- **THEN** 该 id 在当前 change 的 tasks.md 中存在，判定通过

#### Scenario: 落点指向 Scenario

- **WHEN** 一条 Decision 的落点为当前 change delta spec 中的 Scenario 标题
- **THEN** 该标题在 specs 目录中存在，判定通过

#### Scenario: 无代码落点必须写承接制品

- **WHEN** 决策不涉及代码（流程、文档或审查规则）
- **THEN** 落点写作「无代码落点：<承接制品>，因为 <理由>」；只写「无代码落点」或只写「不涉及代码」不通过

#### Scenario: 空泛落点不算

- **WHEN** 落点为「见 tasks」「后续补」「待定」或同类表述
- **THEN** 视为缺失，不计入落点覆盖

### Requirement: Decision 必须给出取舍

每条 Decision SHALL 写出取舍：被否的方案与选择当前方案的理由；确实没有备选时 MUST 写「无备选：<约束>」，不得留空。

#### Scenario: 有备选

- **WHEN** 决策存在被考虑过的替代方案
- **THEN** 取舍行写出替代方案与不选它的理由

#### Scenario: 无备选

- **WHEN** 决策由硬约束唯一确定
- **THEN** 取舍行写「无备选：<约束>」，判定通过；留空判定不通过

### Requirement: 复核与观测

review 执行要点 SHALL 包含 design 落点复核：逐条 Decision 检查落点可解析性与取舍行；有结论无承接的决策记 Medium 并标注「可删项：D1..Dn（无下游承接）」。harness-retro 的效能看板 SHALL 输出 design 决策数、落点率、每决策字数与取舍率，且该观测只读、不写账本、不阻断。已归档 change SHALL NOT 被新规则回溯要求补写或阻断。

#### Scenario: review 记录无承接决策

- **WHEN** review 读到一条没有可解析落点的 Decision
- **THEN** review.md 记一条 Medium，标题含「可删项：D1..Dn」

#### Scenario: 流程决策不误伤

- **WHEN** 决策是无代码落点且写明承接制品与理由
- **THEN** 不视为缺陷，不记 review 条目

#### Scenario: retro 输出设计落点指标

- **WHEN** 运行 `bash scripts/harness/harness-retro.sh --days N`
- **THEN** ⑦段包含 `m7.design_decisions`、`m7.design_landing_rate`、`m7.design_words_per_decision`、`m7.design_alt_rate`

#### Scenario: 存量 change 不回溯

- **WHEN** 对已归档 change 运行任何检查
- **THEN** 不因其 design 缺少落点或取舍行而报错或阻断
