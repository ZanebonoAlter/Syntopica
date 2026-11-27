# flow-restructure Specification

<!-- domain: harness -->

## Purpose

TBD - created by archiving change restructure-flow-semantic-board. Update Purpose after archive.

## Requirements

### Requirement: 链路设计按功能族组织

flow 文档的「链路设计」节 SHALL 按功能族组织，族划分与 `openspec/specs/` 对齐；族数与族名按各文档自己的 spec 结构确定（`semantic-board.md` 6 族 / `daily-report.md` 3 族，对应关系见各文档族首的 `> spec:` 行）。

每个族 SHALL 对应 1–3 个 spec，MUST NOT 存在找不到对应 spec 的族（杂项筐）。

族顺序 SHALL 按「读者查规则的典型路径」排列，MUST NOT 按 change 时间顺序，也 MUST NOT 按技术层（后端/前端）排列。

#### Scenario: 查标签规则只在一族内

- **WHEN** 读者想确认「辅助标签 L2 命中现在该怎么处理」
- **THEN** 该规则 SHALL 全部落在 `semantic-board.md`「标签体系」族内

#### Scenario: 查日报选题只在一族内

- **WHEN** 读者想确认「日报今日重点怎么选题」
- **THEN** 该规则 SHALL 全部落在 `daily-report.md`「生成与匹配」族内

#### Scenario: 族有 spec 对应

- **WHEN** 检查任一族的族名
- **THEN** SHALL 能在 `openspec/specs/` 找到对应 spec

#### Scenario: 不再有 change 命名的块标题

- **WHEN** 检查链路设计节的所有块标题
- **THEN** MUST NOT 出现 `add-*` / `split-*` / `unify-*` 等 change 名作为主体，也 MUST NOT 用「后端：」「前端：」前缀分组

### Requirement: 每族内分「当前规则」与「决策记录」两层

每族 SHALL 含 `#### 当前规则` 与 `#### 决策记录` 两小节。

- **当前规则**：现在必须遵守的规则与阈值。**「不得做 X」类禁令属现行约束，SHALL 留在本层**
- **决策记录**：change 名 + 日期 + 为什么这么做 / 踩过什么坑，SHALL 保留（spec 里没有这类内容）

分界判据 SHALL 是「**换一种做法会不会错**」——会错即现行规则，不因其在讲历史而下沉。

族内无决策记录时 SHALL 写一行「（暂无）」，MUST NOT 留空节。

#### Scenario: 现状与历史不混

- **WHEN** 读某一族的「当前规则」
- **THEN** SHALL 能独立读懂，不需要上文解释「为什么这么做」

#### Scenario: 现行约束不下沉

- **WHEN** 某段写「慢 I/O MUST 在事务外完成」而下一句讲「历史实现曾导致 idle in transaction」
- **THEN** 可执行约束 SHALL 在当前规则，历史原因 SHALL 在决策记录

#### Scenario: 禁令留当前规则

- **WHEN** 某段写「补填不得覆盖已有报告」「watch 观察池已退役，不得再生成」
- **THEN** 该禁令 SHALL 留在当前规则

#### Scenario: 坑不丢

- **WHEN** 检查「标签体系」族
- **THEN** SHALL 同时保留两样：当前规则里的「L2 命中不得改 Label / 重算 MergeEmbedding」（禁令），与决策记录里的「主标签路径曾因此形成合并黑洞」（原因）

### Requirement: 红线节与变更溯源原文保留

`## 业务约束与不变量` 节 SHALL 原样保留：节名 MUST NOT 变更，红线内容与编号顺序 MUST NOT 变更（`semantic-board` 39 条 / `daily-report` 73 条编号、注入 71 条）。

`## 变更溯源`、`## 代码入口` 表 SHALL 原样保留。

#### Scenario: 节名不动

- **WHEN** 重排后检查文档结构
- **THEN** `## 业务约束与不变量` 节名 SHALL 与重排前逐字相同

#### Scenario: 注入不受影响

- **WHEN** 重排后执行 `bash scripts/harness/constraint-digest.sh --brief --domains <域>`
- **THEN** 输出 SHALL 与重排前逐字相同（`semantic-board` 39 条 / `daily-report` 73 条编号、注入 71 条）

### Requirement: 搬迁不丢内容

重排 SHALL 满足：
- **正文内容零丢失**：原文每个非空行（块标题除外）在重排后 SHALL 存在；标题层级变化不计为丢失
- 行数 SHALL NOT 作为判据：重排本身会新增结构行（族标题、层标题、spec 标注）
- 术语 grep 计数前后一致（差异须逐个人工核对并说明）
- **原文备份保留在可回退位置**（`git show <base>:<path>` 可取证，或仓库外备份）

#### Scenario: 正文行对账

- **WHEN** 重排完成
- **THEN** 缺失行 SHALL 全部可归因为标题层级改写或经说明的有意拆分

#### Scenario: 术语对账

- **WHEN** 抽样 20 个高频术语（`MergeEmbedding`/`composite_hit`/`max_sim` 等）
- **THEN** 每个术语的 grep 计数 SHALL 前后一致或差异有书面说明

### Requirement: 只改一个域，方法验证后再推广

每次 change SHALL 只重排一个 flow 文档，MUST NOT 同时改动多个。

推广判据 SHALL 为「链路设计 >150 行且块数 ≥10」。

#### Scenario: 一次一个

- **WHEN** 检查任一重排 change 的 diff
- **THEN** SHALL 只含一个 flow 文档

#### Scenario: 小文档不动

- **WHEN** 评估 `ai-summary.md`（链路 49 行）
- **THEN** SHALL 因未达阈值而不重排
