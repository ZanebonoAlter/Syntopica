## 1. 数据库迁移

- [x] 1.1 创建 `rebuild_jobs` 表迁移 (id, category, trigger, status, total_tags, processed_tags, failed_tags, estimated_end, started_at, completed_at, last_tag_id, config_snapshot, error_detail, created_at)
- [x] 1.2 给 `board_concepts` 表添加 `source` (TEXT default 'auto')、`protected` (BOOLEAN default false)、`declining` (BOOLEAN default false)、`peak_tag_count` (INT default 0) 字段迁移
- [x] 1.3 确认 `topic_tags` 表的 `concept_id` 字段可为 Node (source='abstract') 使用，如有需要添加索引
- [x] 1.4 运行迁移并验证表结构

## 2. 后端模型层

- [x] 2.1 创建 `RebuildJob` model (`backend-go/internal/domain/models/rebuild_job.go`)
- [x] 2.2 更新 `BoardConcept` model 添加 Source, Protected, Declining, PeakTagCount 字段
- [x] 2.3 创建 RebuildJob CRUD repository 方法 (Create, GetByID, UpdateProgress, ListByCategory, ListActive)

## 3. Sector 生成服务

- [x] 3.1 实现 `SectorGenerationService` 基础结构 (`backend-go/internal/domain/tagging/sector_generation.go`)，定义 Auto/LLM/Manual 三种模式的接口
- [x] 3.2 实现 Auto 模式：检测 unplaced Tag 阈值 → LLM 生成 proposal → 0.85 去重 → 创建 Sector + embedding
- [x] 3.3 实现 LLM 模式：收集现有 Sector + 层级树 → LLM 增量建议 → diff 计算 → 返回预览数据供前端展示
- [x] 3.4 实现 LLM 模式执行：用户确认后 → 创建/合并/拆分 Sector → Tag 归属迁移 → 触发层级放置
- [x] 3.5 实现 Manual 模式：用户输入 label + 可选 description → LLM 补全 description → 创建 Sector + embedding + protected
- [x] 3.6 实现 Sector 健康检查：auto 空→DELETE、LLM 衰退→标记 declining、manual 不动
- [x] 3.7 所有 LLM 调用使用 structured output / function calling 传递 JSON schema，不在 prompt 内描述格式

## 4. 合并逻辑重写 (源 DELETE)

- [x] 4.1 实现 `HardMergeTags(sourceID, targetID)` 函数：迁移 article_topic_tags → 迁移 topic_tag_relations → 删除 topic_tag_embeddings → DELETE topic_tags 源行
- [x] 4.2 在 MergeTags / embedding.go 中替换旧的 status='merged' 逻辑为 HardMergeTags
- [x] 4.3 更新所有引用 `topic_tags.status = 'merged'` 的查询，移除 merged 状态相关逻辑
- [x] 4.4 更新所有引用 `topic_tags.status = 'inactive'` 的查询，移除 inactive 状态相关逻辑

## 5. 清理机制重写

- [x] 5.1 重写 `tag_cleanup.go` Phase 1 (僵尸 Tag 清理)：DELETE 无文章/无关系/age>7d 的 Tag
- [x] 5.2 重写 Phase 2 (低质量 Tag)：DELETE quality_score<0.15 且 article_count=1 的 Tag
- [x] 5.3 重写 Phase 3 (空 Node)：DELETE 无子节点的 Node，硬删除
- [x] 5.4 新增 Phase 4 (同 Level 去重)：同 Sector 同 Level Node embedding 相似>0.90 → HardMergeTags
- [x] 5.5 恢复并重写 Phase 5 (Template 校验)：检测 depth 超限/leaf 位置错/children 超限 → 生成 hierarchy_pending_changes
- [x] 5.6 新增 Phase 6 (Sector 健康检查)：调用 SectorGenerationService 的健康检查逻辑
- [x] 5.7 重写 Phase 7 (聚类)：ClusterUnclassifiedTags 不再创建 Node，输出聚类信号作为 anchor 输入
- [x] 5.8 更新 `runCleanupCycle` (tag_hierarchy_cleanup.go) 按新 Phase 顺序执行，保留 time budget
- [x] 5.9 移除旧的 CleanupEmptyAbstractNodes、CleanupSingleChildAbstractNodes、CleanupStaleZeroScoreTags 实现
- [x] 5.10 移除 hierarchy_cleanup.go 中 ReviewHierarchyTrees 的已禁用代码和 validateAndCreateReviewAbstract

## 6. 重建任务系统

- [x] 6.1 实现 `RebuildService` (`backend-go/internal/domain/tagging/rebuild_service.go`)：创建 job、按 batch 执行、断点续传
- [x] 6.2 实现 batch 处理逻辑：SELECT Tags WHERE id > last_tag_id LIMIT batch_size → PlaceTagInHierarchy → 更新 processed_tags/last_tag_id
- [x] 6.3 实现限流：batch 间 sleep 可配置间隔（默认 1s），batch size 可配置（默认 20）
- [x] 6.4 实现预估时间计算：从 ai_call_logs 查询最近 100 次 PlaceTagInHierarchy 平均耗时
- [x] 6.5 实现 Template 变更触发重建：保存 template → DELETE abstract Nodes/relations → 创建 rebuild_job
- [x] 6.6 实现 WebSocket 进度推送：每 batch 完成后推送 rebuild_progress/rebuild_complete 消息
- [x] 6.7 实现断点续传：启动时检测 status='running' 的 job → 设为 paused → 支持从 last_tag_id 恢复

## 7. 后端 API

- [x] 7.1 Sector CRUD API：GET/POST/PUT/DELETE `/api/narratives/board-concepts`，支持 source/protected 参数
- [x] 7.2 Sector LLM 重新生成 API：POST `/api/narratives/board-concepts/regenerate` 返回预览，POST `/api/narratives/board-concepts/regenerate/confirm` 执行
- [x] 7.3 Rebuild API：POST `/api/hierarchy/rebuild` 创建重建任务，GET `/api/hierarchy/rebuild/:id` 查询进度
- [x] 7.4 Rebuild 触发 API：PUT `/api/hierarchy/config` 保存 template 变更时自动创建 rebuild_job
- [x] 7.5 PendingChange 批量审批 API：POST `/api/hierarchy/pending/approve` 支持全部确认或指定 IDs
- [x] 7.6 保护 Sector 删除：DELETE `/api/narratives/board-concepts/:id` 对 protected Sector 要求 confirm=true 参数

## 8. 前端 — /tags 页面基础

- [x] 8.1 创建 `/tags` 路由和页面组件 (`front/app/pages/tags.vue`)，实现两面板 + 底栏布局
- [x] 8.2 实现 Category 切换器 (event/person/keyword) 顶栏
- [x] 8.3 迁移并重构 `TagHierarchy.vue` 到新页面，作为右面板层级树组件
- [x] 8.4 迁移 TagHierarchyRow 行内编辑功能 (重命名、分离、重新分配)
- [x] 8.5 实现 "未归属" 标签列表展示

## 9. 前端 — Sector 管理

- [x] 9.1 实现 Sector 列表组件：显示 label + source icon + Tag count，支持点击筛选
- [x] 9.2 实现 "添加板块" 弹窗：label + 可选 description → manual 模式创建
- [x] 9.3 实现 "LLM 重新生成" 流程：点击 → 调用 regenerate API → 展示 diff 预览弹窗 → 确认执行
- [x] 9.4 实现 Sector 删除：protected 需二次确认，非 protected 单确认
- [x] 9.5 Sector 列表数据来自更新后的 board-concepts API (含 source/protected/declining)

## 10. 前端 — 模板设置与重建

- [x] 10.1 实现模板设置弹窗：展示 Level 定义列表，支持编辑 name/max_children/is_leaf，支持添加/删除 Level
- [x] 10.2 实现保存确认流程：调用 config impact API → 展示影响 Tag 数 + 预估时间 → 用户确认
- [x] 10.3 实现重建进度条：WebSocket 监听 rebuild_progress → 底栏展示进度条 + 剩余时间
- [x] 10.4 实现重建完成通知：展示完成/失败状态，支持 dismiss

## 11. 前端 — PendingChange 审批

- [x] 11.1 底栏展示待确认变更计数 badge
- [x] 11.2 实现 PendingChange 列表面板：按 Sector/Category 分组，展示 tag label + 当前父级 + 建议操作 + 原因
- [x] 11.3 实现 "全部确认" 和逐个确认操作，调用批量审批 API
- [x] 11.4 实现审批结果反馈（成功/失败数）

## 12. 前端 — 旧页面清理

- [x] 12.1 移除 TopicGraphPage (`front/app/features/topic-graph/components/TopicGraphPage.vue`) 的 hierarchy tab
- [x] 12.2 移除 GlobalSettingsDialog (`front/app/components/dialog/GlobalSettingsDialog.vue`) 的 hierarchy tab
- [x] 12.3 更新导航/侧边栏，添加 /tags 页面入口
- [x] 12.4 保留 BoardConceptManager 的核心逻辑供 /tags 页面复用，或将其迁移

## 13. 文档与测试

- [x] 13.1 更新 `docs/reference/database/DATA_LIFECYCLE.md` 的主题标签生命周期章节，使用统一术语
- [x] 13.2 更新 `docs/reference/architecture/` 相关文档反映新的清理流程和 Sector 概念
- [x] 13.3 为 RebuildService 编写单元测试 (batch 处理、断点续传、限流)
- [x] 13.4 为 HardMergeTags 编写单元测试 (article_topic_tags 迁移、relations 迁移、源删除)
- [x] 13.5 为 Sector 三种生成模式编写单元测试
- [x] 13.6 为清理机制新 Phase 编写单元测试
- [x] 13.7 验证：`golangci-lint run ./...` + `go test ./...` + `go build ./...`
- [x] 13.8 验证：`pnpm lint` + `pnpm exec nuxi typecheck` + `pnpm build`

## 14. 前端 — 图谱标签→时间线联动

- [x] 14.1 TopicGraphPage: `handleTagSelect` 添加 `timelineOpen.value = true`
- [x] 14.2 TopicGraphPage: `handleNodeClick` 添加 `timelineOpen.value = true`

## 15. 前端 — /tags 板块→层级树+叙事联动

- [x] 15.1 TagsPage: 将 `selectedSectorId` 作为 `:sectorId` prop 传递给 TagHierarchy 组件
- [x] 15.2 TagHierarchy: 支持 `sectorId` prop，按 sector 过滤层级树（仅展示 concept_id 匹配的 Tags/Nodes）
- [x] 15.3 TagsPage: 右面板新增叙事/文章时间线区域，选中 Sector 后展示该板块标签的相关文章（按日期分组）
- [x] 15.4 TagsPage: 选中 Sector 时在 SectorList 高亮"全部"选项可恢复完整视图
- [x] 15.5 TagHierarchy: 无匹配 Tags 时展示空状态提示

## 16. 前端 — LLM 板块建议审批面板

- [x] 16.1 创建 `SectorApprovalPanel.vue` 组件：独立审批面板（非复用 PendingChangePanel）
- [x] 16.2 每条 LLM 建议（保留/新增/合并/拆分）展示为独立卡片：变更类型图标 + Sector 名称 + 受影响标签数 + LLM 理由
- [x] 16.3 每条建议支持 [接受] [拒绝] 操作，状态实时更新
- [x] 16.4 支持 "全部批准" 操作，仅对已接受的建议执行
- [x] 16.5 执行中展示进度：当前正在执行的变更 + 完成数/总数
- [x] 16.6 执行完成后展示结果摘要：新增板块数、合并数、受影响标签数、成功/失败数
- [x] 16.7 TagsPage: 将现有 `SectorRegenerateDialog` 确认流程替换为审批面板流程
