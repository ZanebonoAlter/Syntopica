## Why

当前标签层级系统缺乏抽象层级的语义定义，导致跨层级关系混乱（如具体事件挂在同类事件下、技术工具挂在不相关的技术栈下）。需要通过引入可配置的层级模板，为 Event、Person、Keyword 三类标签定义严格的抽象层级梯度，从根本上解决层级归属错误问题。

## What Changes

- **新增层级模板系统**：为 Event、Person、Keyword 定义固定的层级模板，明确每层级的语义（如 Event: 类型→主体→实例；Person: 地域→领域→角色→人物）
- **新增标签属性**：TopicTag 增加 `abstraction_level`（层级深度）和 `sub_type`（细分类型）字段
- **新增配置管理**：管理员可通过 Web UI 调整现有模板的层级定义（名称、描述、约束），不可新增模板
- **新增待处理清单**：模板修改后，系统自动扫描并标记违反新规则的现有标签关系，等待人工触发重新整理
- **BREAKING**: 数据库 schema 变更（新增 `hierarchy_config`、`hierarchy_pending_changes` 表，TopicTag 新增字段）

## Capabilities

### New Capabilities
- `hierarchy-level-config`: 层级模板配置管理（定义、调整、生效流程）
- `hierarchy-pending-review`: 待处理标签关系审查（扫描、标记、重新整理）

### Modified Capabilities
- `tag-hierarchy-quality`: 扩展层级质量检查，增加基于模板的层级约束验证（父层级必须小于子层级、类型匹配等）

## Impact

- **数据库**: 新增配置表和待处理表，TopicTag 表增加字段
- **后端 API**: 新增 `/api/hierarchy/config`、`/api/hierarchy/pending`、`/api/hierarchy/rebuild` 等管理接口
- **前端**: 新增层级配置管理页面和待处理清单页面
- **定时清理**: TagHierarchyCleanupScheduler Phase 3 之后增加配置变更检查
- **标签创建流程**: `findOrCreateTag` 和抽象标签创建时自动应用层级模板
