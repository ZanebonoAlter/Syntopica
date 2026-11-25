<!-- domain: dev -->

# docs-milestone-structure Specification

## Purpose

TBD
## Requirements
### Requirement: Milestone folder naming convention

每个里程碑 SHALL 以 `v{major}.{minor}-{semantic-kebab-name}/` 命名存放在 `docs/` 下。版本号与 git tag / release 版本一致。活跃里程碑使用 `active` 作为语义名，完成后重命名为语义名。**新建里程碑不再强制包含 `user-guide/` 子目录**——面向用户的功能说明定位已移交 `docs/reference/flow/` 五段式模板（见 docs-reference-layer capability 的 Flow directory positioning requirement），`user-guide/` 为 legacy 历史保留。

#### Scenario: Creating a new milestone folder

- **WHEN** 开始新版本开发
- **THEN** 在 `docs/` 下创建 `v{version}-{name}/` 目录，内含 `SUMMARY.md`、`design/`、`changes/`、`debug/` 子目录（不再要求 `user-guide/`）

#### Scenario: Active milestone naming

- **WHEN** 里程碑尚未确定语义名（开发进行中）
- **THEN** 使用 `v{version}-active/` 作为临时名称

### Requirement: Milestone internal structure

每个里程碑文件夹 SHALL 包含三个固定子目录：`design/`、`changes/`、`debug/`，以及一个 `SUMMARY.md` 文件。历史已存在的 v1.x 里程碑中的 `user-guide/` 子目录作为历史快照物理保留，不删除、不要求维护。

#### Scenario: Milestone directory listing

- **WHEN** 列出新建里程碑目录
- **THEN** 可见 SUMMARY.md、design/、changes/、debug/ 四项（不含 user-guide/）

#### Scenario: Historical v1.x user-guide retained

- **WHEN** 列出已存在的 v1.x 里程碑目录
- **THEN** 其 user-guide/ 子目录（若历史已存在）作为历史快照保留，不删除

### Requirement: SUMMARY.md content
每个里程碑的 `SUMMARY.md` SHALL 包含：版本号、核心价值描述、阶段完成状态、关键设计决策摘要。该文件从 `docs/releases/MILESTONE_v{version}_SUMMARY.md` 迁移而来。

#### Scenario: Reading a milestone summary
- **WHEN** 打开 `docs/v1.2-tag-intelligence/SUMMARY.md`
- **THEN** 看到 v1.2 的目标、核心流程、完成状态和技术决策

### Requirement: Design documents placement
设计方案文档 SHALL 放入对应里程碑的 `design/` 目录。包含 `-design` 或 `-redesign` 后缀的 plans 文件归类至此。

#### Scenario: Filing a design document
- **WHEN** 一个新设计方案完成
- **THEN** 文件放入当前活跃里程碑的 `design/` 目录

### Requirement: Changes documents placement
变更记录、实施计划、修复记录 SHALL 放入对应里程碑的 `changes/` 目录。

#### Scenario: Filing a change record
- **WHEN** 一个实施计划或修复方案完成
- **THEN** 文件放入对应里程碑的 `changes/` 目录

### Requirement: Debug documents placement
调试记录、踩坑文档 SHALL 放入对应里程碑的 `debug/` 目录。

#### Scenario: Filing a debug record
- **WHEN** 一次调试过程记录完成
- **THEN** 文件放入对应里程碑的 `debug/` 目录

### Requirement: Plans directory elimination

历史散装 plan 文档（`docs/plans/`）SHALL 整体迁移至 `docs/archive/plans/` 退出活跃视野，**保留 git 历史不删除**。原「归类到对应里程碑再删除 docs/plans」的强约束降级：plan 作为 openspec 之前的工作文档历史归档，不再强制回填里程碑。

#### Scenario: Plans directory archived

- **WHEN** 迁移完成
- **THEN** `docs/plans/` 不再存在，原散装 plan 文件位于 `docs/archive/plans/`

