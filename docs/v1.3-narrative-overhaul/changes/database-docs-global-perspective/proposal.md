## Why

当前 `docs/reference/database/DATABASE_FIELDS.md` 是一个表级字段字典（744 行，29 张表），缺少两个关键视角：（1）全局实体关系图——无法直观看出 `topic_tags` 作为 12 条入边枢纽这类结构性事实；（2）数据生命周期——无法从数据状态变迁角度理解一篇文章从入库到进入叙事摘要经历了哪些表的写入。同时，实际数据库有 38 张表，文档覆盖缺口 9 张（含 `ai_summaries` 关系链、`hierarchy_config` 等层级相关表、`otel_spans` 追踪表）。

## What Changes

- 新建 `docs/reference/database/ER_DIAGRAM.md`：全局实体关系图（ASCII 全局概览 + 每域 Mermaid erDiagram + FK 引用矩阵）
- 新建 `docs/reference/database/DATA_LIFECYCLE.md`：6 条数据生命周期链（4 条核心 + 2 条预留功能），粒度为状态字段流
- 新建 `docs/reference/database/_index.md`：数据库文档全景概览 + 导航
- 更新 `DATABASE_FIELDS.md`：补齐至 38 张表、迁移"工作流程"和"配置要求"章节、新增"已废弃/预留表"章节
- 更新 `architecture/overview.md`：相关文档加 database 链接
- 更新 `architecture/data-flow.md`：头部加 DATA_LIFECYCLE.md 交叉引用

## Capabilities

### New Capabilities
- `database-er-diagram`: 全局实体关系图文档，覆盖 38 张表、35 条 FK、6 个业务域
- `database-data-lifecycle`: 数据生命周期文档，6 条链路的状态字段流转视角
- `database-docs-index`: 数据库文档目录索引页，全景概览 + 导航

### Modified Capabilities
- `docs-reference-layer`: 扩展数据库文档覆盖至 38 张表，重组章节结构

## Impact

- 文档层变更，不影响运行时代码
- 涉及文件：`docs/reference/database/`（3 新建 + 1 更新）、`docs/reference/architecture/`（2 更新）
- `DATABASE_FIELDS.md` 大幅修改：补 9 张表字段说明、迁移 2 个章节、修正交叉引用路径
