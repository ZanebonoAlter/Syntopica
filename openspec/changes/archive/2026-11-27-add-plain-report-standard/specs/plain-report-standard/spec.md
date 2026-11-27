# plain-report-standard

<!-- domain: harness -->

## ADDED Requirements

### Requirement: 向用户汇报时第一段用大白话

向用户汇报的任何场合 SHALL 在第一段用不含内部术语的日常语言说明「发生了什么、对用户有什么影响」，结构化信息（表格/清单/编号/术语）SHALL 放在其后。

#### Scenario: change 完工汇报

- **WHEN** 一个 change 完工向用户汇报
- **THEN** 首段说清「以后你会看到什么变化」与「需要你做什么」，不使用 `reasonCode` / `policy` / 门禁项编号 / 文件路径作为主语
- **AND** 详细改动清单、门禁对账结果放在首段之后

#### Scenario: review 发现汇报

- **WHEN** 汇报 review 发现
- **THEN** 首段用日常语言说明问题的实际后果（「统计一直没记上」而不是「记账为零」）
- **AND** 再给 `R-<N>` 编号、级别、`file:line` 与复现命令

#### Scenario: 制品 marker 的理由

- **WHEN** 填写 `<!-- diagrams: none(...) -->` 或 `<!-- ui-impact: none -->` 等 marker 的理由括号
- **THEN** 括号内写「实际改了什么、用户会感觉到什么不同」，不写术语堆叠
- **AND** 反例判据：把理由读给不熟悉本项目的人听，他能复述出这次改动的范围

#### Scenario: 派发前的方案说明

- **WHEN** 向用户说明即将派发的子线程任务或选定方案
- **THEN** 首段说清「要做什么、为什么这样做」，规范条款编号与脚本命令放在其后

### Requirement: 汇报可读性不做机械校验

系统 SHALL NOT 为「汇报是否够通俗」设置自动检查或门禁。

#### Scenario: 不新增机检

- **WHEN** 有人提议为汇报可读性加 lint / 门禁 / 正则检查
- **THEN** 该提议应被驳回，理由是「只能判字样在不在的检查会催生应付式写法」（与 `ui-prototype-mechanics` 撤掉三项 CSS 检查同一教训）

#### Scenario: 规范位置在必经路径

- **WHEN** 开发者或模型需要写 flow 文档 / 制品
- **THEN** 该规范位于 `docs/reference/standard/shared/doc-authoring.md`
- **AND** `AGENTS.md` 的完工汇报段指向同一节（不在两处复制正文）
