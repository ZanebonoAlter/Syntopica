## Purpose

给 `openspec/specs/` 下 177 个 main spec 补一条可机器读取的业务域索引，让「改一个功能时能串起这个域历史上定过什么、有没有冲突、哪些必须沿用」从人工考古变成一条命令。索引只增不改：不改任何 spec 正文，随时可回退。

## ADDED Requirements

### Requirement: main spec 的域声明

每个 `openspec/specs/*/spec.md` MUST 在文件头部（第一个 `#` 标题之前）声明业务域：

```
<!-- domain: <域名>[, <域名>...] -->
```

取值 MUST 来自封闭词表，拼写完全一致：

- **业务域**（10 个，等于 `docs/reference/flow/*.md` 的 basename）：`semantic-board`、`topic-graph`、`daily-report`、`content-enrichment`、`data-enrichment`、`ai-summary`、`discovery`、`reading`、`scheduler`、`research-data-sources`
- **工具域**（5 个）：`harness`、`ui`、`dev`、`api`、`db`

一个 capability 可属多个域（逗号分隔，上限 3 个）。域声明 MUST NOT 替代或修改 Requirement 正文。

#### Scenario: 新 capability 建立时同时声明域

- **WHEN** 一个 change 的 delta 通过 ADDED Requirement 新建一个 capability
- **THEN** 归档合并后该 capability 的 `spec.md` 头部带 `<!-- domain: ... -->`，取值来自词表

#### Scenario: 多域归属

- **WHEN** 一个 capability 的 Requirement 同时覆盖版块归属与话题聚簇（如 `workshop-lane-compose`）
- **THEN** 声明为 `<!-- domain: semantic-board, topic-graph -->`，不强行二选一

#### Scenario: 词表外的值被拒

- **WHEN** 声明的域不在 15 个取值内（如 `board`、`tagging` 这类自造名），或为空值，或超过 3 个
- **THEN** `check-standards.sh` 的 J 段报 WARN 并指明非法值（观察期不阻断归档）；`domain-history.sh` 对词表外域名拒绝查询并打印合法词表

### Requirement: 历史 change 的域归属两跳推导

历史 archived change 的域归属 MUST NOT 依赖人工回填，SHALL 按两跳推导：

```
archive change → 其 delta 指向的 capability 名 → 该 capability 的 main spec domain marker
```

一个 change 的域集合 = 其全部 delta capability 的 domain 并集。delta 指向的 capability 在 `openspec/specs/` 中不存在时（历史重构/退役），该 delta 记入 `orphan` 清单，由人工补标。

#### Scenario: 推导出的域被用于反查

- **WHEN** 执行 `bash scripts/harness/domain-history.sh semantic-board`
- **THEN** 输出给出命中的 archived change 总数，并列出最近 12 条（按日期倒序）；无需任何 per-change 域声明

#### Scenario: 孤儿 delta

- **WHEN** archive change 的 delta 指向已退役的 capability（如 `story-heat-evidence`）
- **THEN** 该 delta 出现在 `orphan` 清单里，不静默丢弃

### Requirement: 按域反查历史契约

`bash scripts/harness/domain-history.sh <域名>` MUST 输出该域的历史契约摘要，退出码恒定 0（无命中不是错误）：

1. **main spec 清单**：capability 名 + 行数 + 最近变更时间
2. **历史时间线**：命中总数 + 最近 12 条（倒序），含被改的 capability
3. **flow 红线**：复用 `constraint-digest.sh --brief --domains`
4. **可能冲突**：历史 `REMOVED` 的 Requirement 标题
5. **必须沿用**：历史 `MODIFIED` 的 Requirement 标题，倒序取 15 条

第 4、5 项 MUST 标注「提示性罗列，不做判断」。节头匹配 MUST 大小写不敏感。

#### Scenario: 开工前查历史

- **WHEN** 准备修改 `semantic-board` 域的一个功能
- **THEN** 先跑 `domain-history.sh semantic-board`，能看到该域所有 main spec、最近变更时间线、以及历史 REMOVED/MODIFIED 条目

#### Scenario: 域不存在

- **WHEN** 传入词表外的域名
- **THEN** 输出可用域名清单并以退出码 0 结束（不抛错，避免打断自动化链路）

### Requirement: 孤儿 delta 显式列出

当 archived change 的 delta 指向的 capability 没有 main spec（已退役/重构）或 main spec 缺 `<!-- domain: -->` 时，`domain-history.sh` MUST 在输出末尾单列一段「孤儿 delta」，逐条给出 change 名与孤儿 capability，并说明补标方式。时间线中命中本域且含孤儿 delta 的 change MUST 带视觉标记（如 `[含孤儿 delta]`），无孤儿的 change MUST NOT 带。

#### Scenario: 孤儿 delta 被列出

- **WHEN** 某 archived change 同时改了一个有 domain 的 capability 和一个已退役 capability
- **THEN** 该 change 出现在第 2 段时间线并带孤儿标记，其孤儿 capability 也出现在「孤儿 delta」段

#### Scenario: 无孤儿时不输出噪音

- **WHEN** 本域命中的 change 全部只改有 domain 的 capability
- **THEN** 「孤儿 delta」段输出「无」而不是空列表
