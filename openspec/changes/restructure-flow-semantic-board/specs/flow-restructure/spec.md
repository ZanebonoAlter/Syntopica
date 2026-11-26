## ADDED Requirements

### Requirement: 链路设计按功能族组织

`docs/reference/flow/semantic-board.md` 的「链路设计」节 SHALL 按 6 个功能族组织：标签体系 / 版块匹配 / 升级建议 / 泳道与工作面 / 版块体检 / 版块治理。

每个族 SHALL 对应 1–2 个 `openspec/specs/` 下的 spec，MUST NOT 存在找不到对应 spec 的族（杂项筐）。

族顺序 SHALL 按「读者查规则的典型路径」排列，MUST NOT 按 change 时间顺序。

#### Scenario: 查标签规则只在一族内

- **WHEN** 读者想确认「辅助标签 L2 命中现在该怎么处理」
- **THEN** 该规则 SHALL 全部落在「标签体系」族内，不需要跨族查找

#### Scenario: 族有 spec 对应

- **WHEN** 检查任一族的族名
- **THEN** SHALL 能在 `openspec/specs/` 找到 1–2 个对应 spec

#### Scenario: 不再有 change 命名的块标题

- **WHEN** 检查链路设计节的所有三级标题
- **THEN** MUST NOT 出现 `add-*` / `split-*` / `unify-*` 等 change 名作为标题主体

### Requirement: 每族内分「当前规则」与「决策记录」两层

每族 SHALL 含 `#### 当前规则` 与 `#### 决策记录` 两小节。

- **当前规则**：现在必须遵守的规则与阈值。其中「**不得复活 X**」类禁令（如「watch 观察池已退役，不得再生成 decision=watch」）属现行约束，SHALL 留在本层；MUST NOT 夹带的是「过去怎么错、为什么改」的叙述
- **决策记录**：change 名 + 日期 + 为什么这么做 / 踩过什么坑，SHALL 保留（spec 里没有这类内容）

族内无决策记录时 SHALL 写一行「（暂无）」，MUST NOT 留空节。

#### Scenario: 现状与历史不混

- **WHEN** 读某一族的「当前规则」
- **THEN** SHALL 能独立读懂，不需要上文解释「为什么这么做」

#### Scenario: 坑不丢

- **WHEN** 检查「标签体系」族
- **THEN** SHALL 同时保留两样：当前规则里的「L2 命中不得改 Label / 重算 MergeEmbedding」（禁令），与决策记录里的「主标签路径曾因此形成合并黑洞」（原因）

### Requirement: 红线节与变更溯源原文保留

`## 业务约束与不变量` 节 SHALL 原样保留：节名 MUST NOT 变更，39 条红线的内容与编号顺序 MUST NOT 变更。

`## 变更溯源` 表 SHALL 原样保留。

#### Scenario: 节名不动

- **WHEN** 重排后检查文档结构
- **THEN** `## 业务约束与不变量` 节名 SHALL 与重排前逐字相同

#### Scenario: 注入不受影响

- **WHEN** 重排后执行 `bash scripts/harness/constraint-digest.sh --brief --domains semantic-board`
- **THEN** SHALL 输出与重排前数量相同的红线条目（39 条）

### Requirement: 搬迁不丢内容

重排 SHALL 满足：
- **正文内容零丢失**：原文每个非空行（块标题除外）在重排后 SHALL 存在；标题层级变化（`###`→`#####`）不计为丢失
- 行数 SHALL NOT 作为判据：重排本身会新增结构行（族标题、层标题、`> spec:` 说明），非空白行数必然增加
- 20 个高频术语的 grep 计数前后一致（差异须逐个人工核对并说明）
- 原文备份保留在可回退位置

#### Scenario: 正文行对账

- **WHEN** 重排完成
- **THEN** 原文的每个非空正文行 SHALL 在重排后存在（块标题因层级变化可不同）
- **AND** 缺失行 SHALL 全部可归因为标题层级改写

#### Scenario: 术语对账

- **WHEN** 抽样 20 个高频术语（`MergeEmbedding`/`composite_hit`/`max_sim`/`suggestion_hash` 等）
- **THEN** 每个术语的 grep 计数 SHALL 前后一致或差异有书面说明

### Requirement: 只改一个域，方法验证后再推广

本 change SHALL 只重排 `semantic-board.md`，MUST NOT 同时改动其他 flow 文档。

推广判据 SHALL 为「链路设计 >150 行且块数 ≥10」，低于此阈值的文档（如 `ai-summary` 49 行、`research-data-sources` 20 行）MUST NOT 重排。

#### Scenario: 一次一个

- **WHEN** 检查本 change 的 diff
- **THEN** SHALL 只含 `docs/reference/flow/semantic-board.md` 一个 flow 文档

#### Scenario: 小文档不动

- **WHEN** 评估 `ai-summary.md`
- **THEN** SHALL 因链路设计仅 49 行而不重排
