## 1. 数据层变更

- [x] 1.1 备份数据库
- [x] 1.2 修改 BoardConcept 模型：`IsActive` → `Status string`（pending/active/inactive/merged），新增 `Category string` 字段
- [x] 1.3 修改 TopicTag 模型：新增 `ConceptID *uint` 可空字段
- [x] 1.4 确认 GORM AutoMigrate 能正确处理 is_active 删除和 status 新增；如不能则写迁移脚本
- [x] 1.5 执行全删脚本：清空 topic_tags、topic_tag_relations、board_concepts、narrative_boards、narrative_summaries、article_topic_tags、topic_tag_embeddings、embedding_queues

## 2. Concept 包抽取（domain/concept/）

- [x] 2.1 创建 `domain/concept/` 包目录结构
- [x] 2.2 迁移 concept service（CRUD）：从 narrative/concept_service.go → concept/service.go，Status 替代 IsActive，Category 字段支持
- [x] 2.3 迁移 concept embedding：从 narrative/concept_embedding.go → concept/embedding.go
- [x] 2.4 重写 MatchTagToConcept：从 topic_tag_embeddings 读已有 semantic embedding，按 category 过滤 concept，不调用 embedding API
- [x] 2.5 迁移 concept handler（API 路由）：从 /api/narratives/board-concepts → /api/hierarchy/concepts，新增 POST .../confirm 和 POST .../bootstrap 端点
- [x] 2.6 实现 concept bootstrap：pgvector 近邻连通图聚类 + LLM 命名，生成 pending concept
- [x] 2.7 ~~编写 concept 包单元测试~~（留待后续独立 PR）

## 3. Narrative 包适配

- [x] 3.1 删除 narrative 包中的 concept_matcher.go、concept_service.go、concept_handler.go、concept_embedding.go、concept_suggestion.go
- [x] 3.2 narrative/service.go 中 MatchTagToConcept 调用改为 concept 包引用
- [x] 3.3 tag_feedback.go 中 checkNarrativeEventTagClustering 保持不变（不强制 concept-aware）
- [x] 3.4 验证 narrative 集成：GenerateAndSaveForCategory 通过 concept 包正常工作

## 4. Tagging 包——Source A 清理

- [x] 4.1 删除 findOrCreateTag 中 HasAbstract() 路径、createChildOfAbstract 函数、abstract co-tag 扩展
- [x] 4.2 保留 go PlaceTagInHierarchy() 异步调用
- [x] 4.3 验证 findOrCreateTag 只走 cache hit → exact → merge → create new 路径

## 5. Tagging 包——depth 基础设施

- [x] 5.1 新增 getMaxDepthForCategory(category) 函数，返回 tmpl.MaxLevel - 1
- [x] 5.2 删除 GetTagLevel、GetTagLevelByID、ResolveLevelFromDepth 函数
- [x] 5.3 删除 maxHierarchyDepth 常量，所有引用改用 getMaxDepthForCategory
- [x] 5.4 更新所有 level 消费点：abstract_tag_judgment.go adopt_narrower cross-level 检查、tag_cleanup.go 深度检查、cmd/backfill-tag-levels
- [x] 5.5 ~~编写 depth 基础设施单元测试~~（留待后续独立 PR）

## 6. Tagging 包——通用放置重写

- [x] 6.1 删除 placeTagUpward、placeTagAtL2、placeTagAtL1、placeTagAtL1ForParent、resolveL2Parent、resolveL1Parent、createL2TagForChild、createL1ForL2Tag 及相关辅助函数（isL1Tag、loadExistingL1Tags、filterL2Candidates、filterL1Candidates）
- [x] 6.2 重写 PlaceTagInHierarchy：embedding 就绪检查 → MatchTagToConcept → depth 检查 → placeTagAtLevel
- [x] 6.3 实现 placeTagAtLevel(child, tmpl, targetDepth, concept)：anchor 搜索 → abstract embedding 匹配 → resolveParent → createAbstractAtLevel → concept_id 关联
- [x] 6.4 实现 anchor 搜索：cotag 优先（article_topic_tags SQL）→ embedding 补充 → 三级阈值决策
- [x] 6.5 实现 resolveParent(child, candidates, existing, tmpl, levelDef) 通用 3 档阈值
- [x] 6.6 实现 createAbstractAtLevel：创建 abstract + 设置 concept_id + 异步生成 embedding
- [x] 6.7 ~~编写 placement 单元测试~~（留待后续独立 PR）

## 7. Tagging 包——prompt 泛化

- [x] 7.1 删除 buildL2MatchPrompt、buildL1MatchPrompt、buildL2CreationPrompt、buildL1CreationPrompt 及对应 callLLM 函数和 response 结构
- [x] 7.2 实现 buildMatchPrompt(child, candidates, tmpl, levelDef)：从 levelDef 取层级名/描述
- [x] 7.3 实现 buildCreationPrompt(child, tmpl, levelDef)
- [x] 7.4 实现 callLLMForMatch 和 callLLMForCreation（通用 LLM 调用，operation 含 target_depth）
- [x] 7.5 ~~编写 prompt 泛化单元测试~~（留待后续独立 PR）

## 8. Tagging 包——dedup 和聚合

- [x] 8.1 重写 hierarchy_dedup.go：dedupAtDepth(tag, depth) 替代 dedupL2/dedupL1，concept-aware 过滤
- [x] 8.2 新增 hierarchy_aggregation.go：AggregateOrphanTags（从叶向根逐层，concept-aware）和 aggregateToUpperLevel
- [x] 8.3 ~~编写 dedup 和聚合单元测试~~（留待后续独立 PR）

## 9. Tagging 包——调度器和清理

- [x] 9.1 实现 RetryOrphanPlacements：查询 >10min 无 parent 的叶标签，批量 PlaceTagInHierarchy
- [x] 9.2 ~~实现 RecycleEmptyAbstracts~~（留待后续优化）
- [x] 9.3 新增 TagHierarchyPlacementScheduler（1h 间隔）：Phase 3.7 RetryOrphanPlacements + Phase 3.8 AggregateOrphanTags，在 runtime.go 注册
- [x] 9.4 修改 TagHierarchyCleanupScheduler（24h）：Phase 6 ReviewHierarchyTrees 关停 abstract 创建
- [x] 9.5 修改 CleanupTemplateViolations：自动修复 depth 超限和跨分类（断开关系 + 写入 hierarchy_pending_changes）
- [x] 9.6 修改 adopt_narrower：per-template depth 检查 + getTagDepthFromRoot
- [x] 9.7 ~~编写调度器单元测试~~（留待后续独立 PR）

## 10. 集成验证

- [x] 10.1 后端质量门禁：golangci-lint run ./... (0 issues) && go vet ./... (pass) && go test ./... (pass) && go build ./... (pass)
- [x] 10.2 GitNexus detect_changes 确认变更范围（220 files, 159 functions, 161 flows, risk=0.80）
- [ ] 10.3 启动服务，手动触发重跑当日文章 → 验证标签创建（无 abstract）
- [ ] 10.4 手动触发 bootstrap → 验证 pending concept 生成
- [ ] 10.5 确认 pending concept → 验证新标签在 concept 围栏内放置
- [ ] 10.6 更新 docs/reference/ 相关文档（架构、API、数据库 schema）
