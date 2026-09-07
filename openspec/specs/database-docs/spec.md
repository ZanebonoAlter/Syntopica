# database-docs

## Purpose

TBD: This spec governs the accuracy and maintenance of database documentation files (`docs/reference/database/*.md`) relative to the actual codebase.

## Requirements

### Requirement: DATA_LIFECYCLE.md SHALL NOT reference removed schedulers

The `database/DATA_LIFECYCLE.md` document SHALL describe data flows using schedulers that are actually registered in `runtime.go`.

#### Scenario: No AutoTagMerge scheduler references

- **WHEN** a developer reads the tag merge data flow diagram
- **THEN** the document SHALL NOT reference an `AutoTagMerge` scheduler running at 3600s (not registered in `runtime.go`)
- **AND** SHALL align the merge flow with the actual mechanisms (manual `HardMergeTags` + `tag_quality_score` recomputation)

#### Scenario: No narrative_summary / NarrativeSummaryScheduler references

- **WHEN** a developer reads the narrative / ai_call_logs data flow
- **THEN** the document SHALL NOT reference `narrative_summary` as a capability/scheduler or `NarrativeSummaryScheduler` (not registered in `runtime.go`)
- **AND** SHALL align the narrative flow with the actual current schedulers registered in `runtime.go` (e.g. `daily_report`)

### Requirement: Database table docs SHALL be organized into per-domain files

数据库表参考文档 `docs/reference/database/` SHALL 按业务域组织：每个域文档承载该域的字段字典与域级 ER 图，全局性内容（阅读约定、完整表清单、FK 真相、FK 引用矩阵、关系模式说明）SHALL 集中于入口文档，导航索引 SHALL 列出全部域文档及其覆盖的表。

#### Scenario: 域文档包含字段字典与 ER 图

- **WHEN** 读者打开任一域文档（如 `tables/daily-report-watch.md`）
- **THEN** 该文档 SHALL 同时包含本域全部表的字段字典小节与对应的域级 ER 图
- **AND** 文档总长 SHALL 不超过 300 行（单次可读规模）

#### Scenario: 全局约定集中于入口文档

- **WHEN** 读者需要跨域的全局事实（FK 真相、向量维度运行时规则、枚举约定）
- **THEN** 这些内容 SHALL 位于入口文档（`_index.md` 或其指向的全局文档），而非散落在域文档内重复

#### Scenario: 旧单文件移除且无死链

- **WHEN** 切分完成后在仓库内全文检索 `DATABASE_FIELDS.md` 或 `ER_DIAGRAM.md` 链接
- **THEN** 除 openspec 归档历史外 SHALL 无存活链接指向已移除文件

### Requirement: Database table docs SHALL reference existing Go models

域文档与入口表清单 SHALL 引用代码库中真实存在的 Go model 路径与表-model 映射。

#### Scenario: topic_analysis_jobs mapping is corrected

- **WHEN** 读者查阅入口表清单中 `topic_analysis_jobs` 的主映射行
- **THEN** 文档 SHALL NOT 声称该表映射到 `topicanalysis.topicAnalysisJobRecord`（该包与 model 不存在，表未注册于 `migrator.go`）
- **AND** SHALL 将该行移除或归入"无 Go 代码引用 / 已废弃"节

#### Scenario: model path uses current location

- **WHEN** 读者查阅 `ai_summaries`（或其替代状态说明）的 model 路径引用
- **THEN** 文档 SHALL 引用 `internal/models/`（当前共享 model 位置）
- **AND** SHALL NOT 引用已删除的 `internal/domain/models/`
