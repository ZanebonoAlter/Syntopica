## Why

标签系统存在三套互相冲突的设计：旧抽象标签自由创建、层级模板约束、聚类直接产出抽象。清理机制 (CleanupEmptyAbstractNodes, CleanupSingleChildAbstractNodes) 与层级模板的生长需求矛盾，导致 Phase 2.5 创建的 Node 被 Phase 3 立刻清掉；board_concepts 的 concept fence 存在冷启动鸡生蛋问题；前端层级管理功能散落在 TopicGraphPage 和 GlobalSettingsDialog 两个入口，用户无法形成闭环操作流。

## What Changes

- **BREAKING** 统一术语：abstract tag → Node，board_concept → Sector，leaf tag → Tag，层级 level 保持不变
- **BREAKING** 合并后源 Tag/Node 直接 DELETE（不再保留 status=merged / status=inactive 软状态）
- **BREAKING** 空节点直接 DELETE（不再 inactive）
- 清理机制重写为 template-aware：空 Node 删除、同 Level 去重、Template 校验生成 PendingChange
- ClusterUnclassifiedTags 改为 template 内约束，不直接创建 Node，聚类结果作为 anchor 信号输入 PlaceTagInHierarchy
- Sector (board_concept) 三种生成模式：auto（unplaced 阈值触发）、LLM（增量建议 + diff 预览确认）、manual（protected）
- Sector 健康检查：auto 创建的空 Sector 可删除、LLM 创建的衰退 Sector 标记提示、manual 创建的只能手动删
- Template 变更时删除该 Category 所有 Node + 关系，全量重建 PlaceTagInHierarchy，展示影响标签数 + 预估时间
- 新增 `rebuild_jobs` 表支持异步重建任务：限流、断点续传、进度汇报
- 新增 `/tags` 层级管理页面：Sector 管理 + 层级树 + 模板设置 + 重建进度 + PendingChange 审批，统一入口
- TopicGraphPage 移除 hierarchy tab，只保留 graph + narrative
- GlobalSettingsDialog 移除 hierarchy tab，功能合并到 /tags
- JSON 序列化要求通过 LLM function calling / structured output 参数传递，不在 prompt 内描述格式
- 叙事生成允许引用断裂（被删 Tag/Node 的 ID 不再同步更新叙事数据）
- Node 增加 `concept_id` 字段，归属 Sector

## Capabilities

### New Capabilities

- `sector-management`: Sector (board_concept) 三种生成模式 (auto/LLM/manual)、健康检查、删除规则、Tag 归属 Sector 逻辑
- `hierarchy-rebuild`: Template 变更触发的异步全量重建：rebuild_jobs 表、限流、断点续传、进度 WebSocket 推送
- `hierarchy-cleanup-v2`: Template-aware 的清理机制替代旧逻辑：空 Node 删除、同 Level 去重（源 DELETE）、Template 校验生成 PendingChange、Sector 健康检查
- `hierarchy-management-ui`: /tags 层级管理页面：Sector 列表 + 层级树编辑 + 模板设置 + 重建进度 + PendingChange 批量审批

### Modified Capabilities

- `board-concept-management`: 重命名为 Sector，增加 protected 标记、来源标记 (auto/llm/manual)、健康检查触发条件、衰退标记
- `tag-hierarchy-quality`: Template-aware 合并（源 DELETE 而非 status=merged）、Whitespace 去重保留、Degenerate tree flattening 保留但改为直接删除中间 Node
- `tagging-domain`: 聚类行为变更（不直接创建 Node），Node 生命周期变更（直接 DELETE 无软状态）

## Impact

- **后端核心变更**: `tag_cleanup.go` 重写、`tag_clustering.go` 重写、`hierarchy_placement.go` 适配、`abstract_tag_judgment.go` 清理已废弃路径、`hierarchy_cleanup.go` 移除 ReviewHierarchyTrees、`embedding.go` MergeTags 改为源 DELETE
- **后端新增**: `rebuild_jobs` 表 + model、Sector 三种生成 service、重建 job 调度器、Node concept_id 字段迁移
- **后端删除**: `topic_tags.status` 的 merged/inactive 状态相关逻辑、`CleanupEmptyAbstractNodes`/`CleanupSingleChildAbstractNodes` 旧实现、`tag_hierarchy_cleanup.go` 中已废弃的 Phase
- **前端重构**: `/tags` 新页面（合并 TopicGraphPage hierarchy tab + GlobalSettings hierarchy tab + BoardConceptManager + TagQueuePanel）、TopicGraphPage 和 GlobalSettingsDialog 对应 tab 移除
- **数据库迁移**: `rebuild_jobs` 新表、`board_concepts` 增加 source/protected 字段、`topic_tags` 确保 concept_id 可用于 Node
- **API 变更**: 新增 rebuild job CRUD + 进度 WebSocket、Sector 生成 API (auto/llm/manual)、PendingChange 批量审批 API；`/topic-tags/hierarchy` 保持兼容
