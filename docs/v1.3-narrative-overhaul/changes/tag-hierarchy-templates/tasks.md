## 1. 数据库

- [x] 1.1 创建 `hierarchy_config` 表（id, templates JSONB, version INT, updated_at）
- [x] 1.2 创建 `hierarchy_config_versions` 表（id, config_id FK, version, templates, change_log, created_at）
- [x] 1.3 创建 `hierarchy_pending_changes` 表（id, tag_id FK, tag_label, change_type, current_parent_id, current_parent_label, reason, status, created_at, resolved_at）
- [x] 1.4 插入默认配置（5 个模板的默认层级定义，version=1）
- [x] 1.5 定义 GORM models（`models/hierarchy_config.go`, `models/hierarchy_pending_change.go`）

## 2. 核心模板系统

- [x] 2.1 新建 `topicanalysis/hierarchy_template.go`：定义 `CategoryHierarchyTemplate`、`AbstractionLevel`、`LevelConstraints` 结构体
- [x] 2.2 实现 5 个默认模板工厂函数（`buildDefaultEventTemplate()` 等）
- [x] 2.3 新建 `topicanalysis/hierarchy_config.go`：`HierarchyTemplateManager` 结构体，实现 `LoadFromDB()`、`LoadSystemDefaults()`、`GetTemplate(category, subType)`、`Reload()`
- [x] 2.4 实现深度反推层级函数 `ResolveLevelFromDepth(category string, depth int) int`
- [x] 2.5 实现 `GetTagLevel(tag *models.TopicTag) int`（封装深度查询 + 模板映射）
- [x] 2.6 系统启动时初始化 manager（`app/runtime.go` 中调用 `GetHierarchyManager().LoadFromDB()`）

## 3. 向上聚合放置算法

- [x] 3.1 新建 `topicanalysis/hierarchy_placement.go`：实现 `PlaceTagInHierarchy(tag, template)` 核心函数
- [x] 3.2 实现 `findL2Candidates(tag, template)` — 在 L2 候选池中 embedding 搜索
- [x] 3.3 实现 `matchL2Parent(tag, candidates)` — 三级阈值判断（0.85 直接挂 / 0.60-0.85 LLM / <0.60 创建新 L2）
- [x] 3.4 实现 `matchL1Parent(l2Tag, template)` — L1 匹配逻辑（阈值 0.80/0.55）
- [x] 3.5 实现 `createL2Tag(tag, template)` — LLM 生成新 L2 名称和描述
- [x] 3.6 实现 `createL1EventType(l2Tag, template)` — LLM 生成事件类型，含已有类型 few-shot
- [x] 3.7 在 `findOrCreateTag` 最后一步调用 `PlaceTagInHierarchy`

## 4. LLM Prompt 改造

- [x] 4.1 `abstract_tag_judgment.go`：修改 `buildTagJudgmentPrompt`，注入层级上下文（标签当前层级、允许的父层级范围、同级已有标签参考）
- [x] 4.2 新增 `buildL2MatchPrompt(tag, candidates)` — L2 父标签选择的 prompt
- [x] 4.3 新增 `buildL1MatchPrompt(l2Tag, existingTypes)` — L1 事件类型选择的 prompt（含已有类型列表）
- [x] 4.4 新增 `buildL2CreationPrompt(tag)` — 创建新 L2 的 prompt
- [x] 4.5 新增 `buildL1CreationPrompt(l2Tag)` — 创建新 L1 的 prompt
- [x] 4.6 `abstract_tag_hierarchy.go`：修改 `batchJudgeAbstractRelationships` prompt，注入层级定义

## 5. 去重机制

- [x] 5.1 实现 `dedupL2(parentTag)` — 创建 L2 后检查 embedding > 0.95 的已有 L2，存在时直接合并
- [x] 5.2 实现 `dedupL1(parentTag)` — 创建 L1 后检查 embedding > 0.90 的已有 L1，存在时 LLM 判断是否合并
- [x] 5.3 新建 LLM prompt `buildL1DedupPrompt(tag1, tag2)` — L1 去重判断
- [x] 5.4 在 `createL2Tag` 和 `createL1EventType` 完成后自动调用去重

## 6. 配置管理 API

- [x] 6.1 `GET /api/hierarchy/config` — 返回当前配置（模板 + 层级定义 + 版本号）
- [x] 6.2 `PUT /api/hierarchy/config` — 更新配置（验证模板不可增删、生成待处理清单、递增版本号）
- [x] 6.3 `GET /api/hierarchy/pending` — 返回待处理标签列表（支持 status 过滤）
- [x] 6.4 `POST /api/hierarchy/rebuild` — 触发重新整理（支持 category 过滤、dry_run 预览、WebSocket 进度广播）
- [x] 6.5 实现 `previewConfigImpact(newConfig)` — 扫描现有标签，计算违反新规则的标签数和类型
- [x] 6.6 实现 `generatePendingChanges(impact)` — 将影响结果写入 `hierarchy_pending_changes` 表
- [x] 6.7 实现 `processPendingChanges(changes)` — 对每个待处理标签重新走 `PlaceTagInHierarchy`
- [x] 6.8 在 `router.go` 注册 `/api/hierarchy/` 路由组

## 7. 清理调度器改造

- [x] 7.1 `tag_cleanup.go` Phase 3：新增 3d（模板深度检查）和 3e（跨分类检查）步骤
- [x] 7.2 `adopt_narrower_queue_handler.go` Phase 4：收养时检查目标标签与候选标签的层级是否匹配
- [x] 7.3 `hierarchy_cleanup.go`：重写 `ReviewHierarchyTrees` 为模板对齐审查
- [x] 7.4 实现 `Phase6_CheckLevelAlignment(forest, template)` — 层级对齐检查
- [x] 7.5 实现 `Phase6_DedupL1(template)` — L1 embedding 去重
- [x] 7.6 实现 `Phase6_DedupL2(template)` — L2 embedding 去重
- [x] 7.7 实现 `Phase6_SampleAuditLeaves(template)` — 10% L3 叶子归属 LLM 抽查
- [x] 7.8 `jobs/tag_hierarchy_cleanup.go`：更新调度器入口，调用新的 Phase 3/4/6 实现

## 8. 数据迁移

- [x] 8.1 新建 `cmd/backfill-tag-levels/main.go` — 为现有标签打层级的迁移脚本
- [x] 8.2 实现 `backfillLevelsByDepth(category)` — 按深度对所有 active 标签赋值层级
- [x] 8.3 实现 `findInvalidRelations()` — 扫描现有关系，找出深度超限和跨分类的关系
- [x] 8.4 实现 `repairInvalidRelations(dryRun)` — 断开违规关系，子标签重新走放置流程
- [x] 8.5 支持 `--dry-run`、`--category` 参数

## 9. 前端

- [x] 9.1 新建 `front/app/features/hierarchy-config/` 目录
- [x] 9.2 `HierarchyConfigPage.vue` — 层级配置页面（模板选择器 + 层级列表编辑 + 保存按钮）
- [x] 9.3 `HierarchyPendingList.vue` — 待处理标签列表（显示标签名、当前路径、问题类型、忽略/调整操作）
- [x] 9.4 `RebuildTrigger.vue` — rebuild 触发按钮（含 category 选择、dry_run 开关、进度展示）
- [x] 9.5 `front/app/api/hierarchyConfig.ts` — API client（getConfig, updateConfig, getPending, triggerRebuild）
- [x] 9.6 集成 WebSocket 收听 rebuild 进度广播
- [x] 9.7 在设置导航中添加"层级配置"入口

## 10. 测试

- [x] 10.1 `hierarchy_template_test.go` — 测试模板加载、默认回退、层级反推
- [x] 10.2 `hierarchy_placement_test.go` — 测试向上聚合算法（L2 匹配、L1 匹配、边界情况）
- [x] 10.3 `hierarchy_config_test.go` — 测试配置保存/加载、版本递增、影响预览
- [x] 10.4 `hierarchy_cleanup_test.go`：更新 Phase 6 测试，覆盖模板对齐检查
- [x] 10.5 集成测试：完整流程（创建标签 → 聚合 → 修改配置 → 待处理 → rebuild）
