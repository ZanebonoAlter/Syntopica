## MODIFIED Requirements

### Requirement: Reference directory structure
`docs/reference/` SHALL 包含以下子目录和文件：
- `architecture/` — 系统总览、后端架构、前端架构、组件、数据流、运行时、链路追踪
- `api/` — API 参考文档（按路由前缀拆分）
- `database/` — 数据库字段参考（`DATABASE_FIELDS.md`）、全局实体关系图（`ER_DIAGRAM.md`）、数据生命周期（`DATA_LIFECYCLE.md`）、目录索引（`_index.md`）
- `development.md` — 开发规范（构建、测试、代码风格、目录约定、提交检查）
- 其他跨里程碑功能指南（configuration.md、deployment.md、testing.md 等）

#### Scenario: Reference directory listing
- **WHEN** 列出 `docs/reference/`
- **THEN** 可见 architecture/、api/、database/ 目录和若干 .md 文件

#### Scenario: Database directory contains three documents plus index
- **WHEN** 列出 `docs/reference/database/`
- **THEN** 可见 `DATABASE_FIELDS.md`、`ER_DIAGRAM.md`、`DATA_LIFECYCLE.md`、`_index.md`

## ADDED Requirements

### Requirement: DATABASE_FIELDS covers all 38 tables
`DATABASE_FIELDS.md` SHALL document all 38 tables present in the `public` schema of the `rss_reader` database, including `ai_summaries`、`ai_summary_feeds`、`ai_summary_topics`、`digest_configs`、`otel_spans`、`hierarchy_config`、`hierarchy_config_versions`、`adopt_narrower_queues`、`multi_parent_resolve_queues`.

#### Scenario: No undocumented tables
- **WHEN** reader compares `DATABASE_FIELDS.md` table list against `\dt` output
- **THEN** every table in the database has a corresponding field description section

### Requirement: Deprecated tables clearly marked
Tables with zero data rows and no Go code references SHALL be documented in a dedicated section titled "已废弃/预留表", with a note "当前无 Go 代码引用，可能为旧版功能遗留".

#### Scenario: Reader identifies abandoned tables
- **WHEN** reader views the deprecated tables section
- **THEN** `ai_summaries`、`ai_summary_feeds`、`ai_summary_topics`、`digest_configs` are listed with their field descriptions and marked as deprecated/reserved

### Requirement: Cross-references between architecture and database docs
`architecture/overview.md` SHALL reference `database/ER_DIAGRAM.md` and `database/DATA_LIFECYCLE.md` in its "相关文档" section. `architecture/data-flow.md` SHALL include a note at the top pointing to `database/DATA_LIFECYCLE.md` for data state transition perspective.

#### Scenario: Reader navigates from architecture to database docs
- **WHEN** reader is viewing `architecture/overview.md` and wants to understand table relationships
- **THEN** they find a link to `database/ER_DIAGRAM.md` in the related docs section
