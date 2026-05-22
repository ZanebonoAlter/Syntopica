## Why

`unify-tag-hierarchy` 已经完成了 Sector、Template、Rebuild、PendingChange 和 `/tags` 页面等功能点，但用户无法从“标签混乱”走到“结构被修复并验证”的闭环。当前实现存在关键断点：Template 确认流程提前执行、Rebuild WebSocket 前后端协议不一致、Sector 冷启动未接入放置调度、`PlaceTagInHierarchy` 无法在需要时创建 Node、聚类 anchor signal 未被消费，导致任务完成但用户故事失败。

## What Changes

- 新增层级闭环编排能力，将 Sector bootstrap、Tag 放置、Node 创建、Template 重建、PendingChange 审批和 UI 刷新串为一个可验证用户旅程。
- 修正 Template 变更流程：影响预览与确认应用分离，确认前不得保存新模板、删除旧 Node 或启动 rebuild job。
- 修正 Rebuild 事件协议：后端推送与前端监听使用同一种消息结构，并在 `/tags` 页面实时展示进度、完成、失败和刷新结果。
- 修正 Sector 冷启动：当 category 的 unplaced Tag 超过阈值时，placement/rebuild 编排必须触发 auto Sector 生成，然后重试放置。
- 修正 `PlaceTagInHierarchy`：匹配到 Sector 但找不到合适父 Node 时，必须在满足信息增益约束的情况下创建 Node；否则返回可解释 blocker，而不是静默停在 `unplaced`。
- 修正 anchor signal：聚类输出必须被持久化或直接输入放置流程；如果无法消费，则删除该概念和相关 UI/统计承诺。
- 修正 LLM Sector 审批：确认 API 返回真实逐项执行结果，前端展示后端事实，不再按 diff 自行推断成功数。
- 增加用户故事级验收覆盖，验证 `/tags` 页面从发现问题、调整结构、触发重建到结果刷新的端到端闭环。

## Capabilities

### New Capabilities

- `tag-hierarchy-closure`: 标签层级闭环编排，覆盖 `/tags` 用户故事、状态机、Sector bootstrap、放置重试、重建进度和结果刷新。
- `hierarchy-rebuild-events`: Template 变更与 rebuild job 的确认式启动、WebSocket 事件协议和 UI 进度反馈。

### Modified Capabilities

- `tag-hierarchy-quality`: 修正 Node 创建与质量约束，确保 `PlaceTagInHierarchy` 是唯一 Node 创建入口且能闭合放置路径。
- `board-concept-management`: 修正 Sector 生成、LLM 审批确认和删除/合并/拆分结果反馈。
- `tag-to-board-matching`: 修正冷启动 unclassified bucket 到 Sector bootstrap 的行为，并明确匹配失败后的用户可见状态。

## Impact

- **后端编排**: 新增或收敛 hierarchy orchestration service，连接 `AutoGenerateSectors`、`PlaceTagInHierarchy`、`RebuildService`、PendingChange 审批和 cleanup/placement scheduler。
- **后端核心逻辑**: 修改 `hierarchy_placement.go`、`sector_generation.go`、`rebuild_service.go`、`hierarchy_handler.go`、`cleanup_v2.go`、`tag_hierarchy_placement.go`。
- **后端 API**: 拆分 config preview/apply 或增加 confirm 参数；统一 rebuild event payload；Sector confirm API 返回真实执行结果。
- **前端**: 修改 `/tags` 页面、`TemplateSettingsDialog`、`SectorApprovalPanel`、`PendingChangePanel`、`useWebSocketRebuild`，让 UI 按后端状态驱动。
- **测试**: 增加 Go service/API 测试、前端组件/组合式测试、以及 acceptance story 覆盖标签层级闭环。
- **文档**: 更新 `docs/reference/architecture/` 与标签生命周期文档，记录闭环状态机和故障排查入口。
