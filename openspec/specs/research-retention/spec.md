<!-- domain: dev -->

# research-retention Specification

## Purpose
TBD - created by archiving change amend-dev-workflow. Update Purpose after archive.
## Requirements
### Requirement: 调研产物两级落点

调研产物 SHALL 按是否归属于某个 openspec change 落点：

- **change 强相关调研** SHALL 存放于 `openspec/changes/<name>/research.md`，随 change 归档永久保留，且 proposal（或 design）中存在对该文件的引用。
- **无 change 归属的通用调研** SHALL 存放于 `docs/research/<topic>.md`（topic 用 kebab-case）。
- `docs/experience/` SHALL 仅存放踩坑教训/事后复盘类文档，不再存放调研快照。

`research.md` MUST 包含：调研方式、关键发现、**关键代码摘录（每段带源路径与快照日期）**、采纳/不采纳决策表。快照类内容 SHALL 标注"快照非权威"。

#### Scenario: change 驱动型调研随 change 留存

- **WHEN** 一次外部仓库/技术选型调研直接驱动 change `X` 的决策
- **THEN** 调研数据存在于 `openspec/changes/X/research.md`，含带源路径与日期的关键代码摘录，且 `X` 的 proposal 引用了它

#### Scenario: 通用调研进独立池

- **WHEN** 一次调研不归属于任何具体 change（如跨项目工具对比）
- **THEN** 调研文档落于 `docs/research/<topic>.md`，不落入 `docs/experience/`

#### Scenario: 踩坑复盘归 experience

- **WHEN** 一次事故/踩坑产生事后教训文档
- **THEN** 该文档落于 `docs/experience/`，且不含事前调研快照内容

#### Scenario: 归档后仍可回查

- **WHEN** 一个已归档 change 的实现细节被质疑，需要回查当初采纳的原始数据
- **THEN** 可通过归档目录内该 change 的 `research.md` 直接取到关键代码摘录与决策表，无需重新调研源仓库

