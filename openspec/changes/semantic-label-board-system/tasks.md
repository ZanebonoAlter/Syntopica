## 1. 数据模型与迁移

- [x] 1.1 创建 `semantic_labels` 模型：id, label, slug, embedding(vector), label_type, aliases(jsonb), ref_count, description, display_order, source, status, protected, created_at, updated_at
- [x] 1.2 创建 `topic_tag_semantic_labels` 关联模型：topic_tag_id, semantic_label_id（仅辅助标签）
- [x] 1.3 创建 `topic_tag_board_labels` 关联模型：topic_tag_id, semantic_board_id, score, match_reason, created_at, updated_at
- [x] 1.4 创建 `board_composition` 关联模型：board_id, auxiliary_label_id
- [x] 1.5 迁移 `narrative_boards`：新增 `semantic_board_id`，逐步移除 `abstract_tag_id` / `board_concept_id` 依赖
- [x] 1.6 编写数据库迁移：新建 4 张表，添加 slug、label_type、status、embedding、topic_tag_id、semantic_board_id 索引
- [ ] 1.7 编写数据库迁移：删除旧 board_concepts、topic_tags.concept_id、层级相关表/索引（不做历史数据迁移）
- [x] 1.8 Seed `ai_settings`：semantic_board_match_*、semantic_board_upgrade_* 默认配置

## 2. 辅助标签提取与入库

- [x] 2.1 修改 tagger.go 提取 prompt：在 LLM 提取 tag 时增加辅助标签输出（3-5 个），修改 JSON schema
- [x] 2.2 修改 ExtractedTag / TopicTag 类型：增加 AuxiliaryLabels []string 字段
- [x] 2.3 编写辅助标签数量和质量校验：拒绝 0 个、超过 5 个、空字符串和明显泛词
- [x] 2.4 创建 auxiliary_label_service.go：实现 L1 slug/alias 精确匹配查找
- [x] 2.5 实现 L2 embedding ≥0.95 merge 逻辑：保留 ref_count 更大的一方，小方 label 加入 aliases
- [x] 2.6 实现 L3 新建辅助标签：创建 semantic_label（label_type=auxiliary），生成 embedding
- [x] 2.7 实现 tag 与辅助标签关联写入及 ref_count 自增
- [x] 2.8 编写入库服务单元测试：覆盖 L1/L2/L3、禁用标签排除、质量校验

## 3. 辅助标签治理能力

- [ ] 3.1 实现禁用辅助标签：status=disabled，后续匹配和升级候选排除
- [ ] 3.2 实现手动合并 alias：迁移 topic_tag_semantic_labels，积累 aliases
- [ ] 3.3 实现从 board_composition 移除辅助标签
- [ ] 3.4 编写治理能力测试：禁用、alias 合并、composition 移除不自动回填

## 4. SemanticBoard 匹配逻辑

- [ ] 4.1 创建 semantic_board_matching.go：读取 tag 辅助标签和 active SemanticBoard composition
- [ ] 4.2 实现直接命中匹配（tag 辅助标签 ∈ board 构成标签）
- [ ] 4.3 实现间接匹配：计算命中率（hit_count / tag 辅助标签总数）和 max_sim
- [ ] 4.4 实现三规则挂载判断：命中率、max_sim、加权综合分
- [ ] 4.5 实现多 board 挂载：按分数排序，默认最多 3 个，写入 topic_tag_board_labels
- [ ] 4.6 实现匹配参数读取：从 ai_settings 读取 semantic_board_match_* 配置
- [ ] 4.7 编写匹配逻辑单元测试：覆盖直接命中、三规则、多 board 截断、无匹配、冷启动无 board

## 5. SemanticBoard 升级与冷启动建议

- [ ] 5.1 创建 semantic_board_upgrade.go：收集 ref_count ≥ semantic_board_upgrade_ref_count_threshold 的候选辅助标签
- [ ] 5.2 实现候选 + 已有 SemanticBoard 的 embedding 预聚类（cosine 距离阈值 0.7）
- [ ] 5.3 实现簇内 co-tag 事件补充：时间窗口 30 天、共现频率排序 top20、embedding 去重（>0.85）、硬上限 15
- [ ] 5.4 实现 LLM 建议 prompt：每个簇的标签列表 + 事件上下文 → merge_into_existing / create_new / skip
- [ ] 5.5 实现建议结果持久化或返回结构：用户确认前不写 SemanticBoard/board_composition
- [ ] 5.6 实现用户确认执行：创建新 SemanticBoard 或更新已有 board_composition
- [ ] 5.7 编写升级机制测试：覆盖冷启动无 board、候选收集、聚类、LLM mock、确认执行

## 6. 回填队列

- [ ] 6.1 创建 semantic_board_backfill.go：支持 all、unassigned、board 三种回填模式
- [ ] 6.2 实现异步队列逐个执行 board 匹配并重写 topic_tag_board_labels
- [ ] 6.3 添加回填进度查询和失败记录
- [ ] 6.4 编写回填测试：覆盖全量、无归属、指定 board、幂等重跑

## 7. NarrativeBoard 生成改造

- [ ] 7.1 删除 abstract tree → hotspot NarrativeBoard 路径，不再从 topic_tag_relations 生成每日板
- [ ] 7.2 改造每日板输入收集：按日期、scope、semantic_board_id 收集归属 event tags
- [ ] 7.3 改造 NarrativeBoard 创建：写入 semantic_board_id、event_tag_ids、scope_type、scope_category_id
- [ ] 7.4 改造 prev_board_ids 匹配：按 semantic_board_id + scope + 前一日日期续接
- [ ] 7.5 改造 board narrative context：使用 SemanticBoard label/description
- [ ] 7.6 允许同一 event tag 出现在多个 NarrativeBoard 中
- [ ] 7.7 编写叙事生成测试：覆盖无 board 冷启动、分类 scope、global scope、多 board 重复、prev 续接

## 8. API 层

- [ ] 8.1 实现 SemanticBoard CRUD API：GET/POST/PUT/DELETE /api/semantic-boards
- [ ] 8.2 实现 board composition API：查看构成、移除辅助标签
- [ ] 8.3 实现辅助标签池查询和治理 API：GET /api/auxiliary-labels、disable、merge-alias
- [ ] 8.4 实现升级候选查看 API：GET /api/semantic-boards/upgrade-candidates
- [ ] 8.5 实现升级建议 API：POST /api/semantic-boards/upgrade-suggest
- [ ] 8.6 实现升级确认 API：POST /api/semantic-boards/upgrade-execute
- [ ] 8.7 实现回填触发和进度 API：POST /api/semantic-boards/backfill、GET /api/semantic-boards/backfill/:id
- [ ] 8.8 实现匹配参数配置 API：GET/PUT /api/semantic-boards/matching-config
- [ ] 8.9 实现 tag 关联查询 API：GET /api/tags/:id/auxiliary-labels 和 GET /api/tags/:id/semantic-boards
- [ ] 8.10 注册所有新路由到 router.go，并移除层级/旧 concept 路由

## 9. 删除废弃代码

- [ ] 9.1 删除层级体系文件：hierarchy_template.go, hierarchy_config.go, hierarchy_placement.go, hierarchy_cleanup.go, hierarchy_dedup.go, hierarchy_aggregation.go, hierarchy_handler.go, hierarchy_orchestration.go, hierarchy_prompts.go 及对应测试
- [ ] 9.2 删除抽象标签文件：abstract_tag_crud.go, abstract_tag_handler.go, abstract_tag_hierarchy.go, abstract_tag_judgment.go, abstract_tag_service.go, abstract_tag_tree.go, abstract_tag_update_queue.go 及对应测试
- [ ] 9.3 删除相关队列：adopt_narrower_queue.go/handler, multi_parent_resolve_queue.go, tree_bridge.go 及对应测试
- [ ] 9.4 删除旧 concept 包文件：concept/bootstrap.go, concept/matcher.go, concept/suggest.go 及旧 BoardConcept CRUD
- [ ] 9.5 清理 models 中的旧字段：topic_tags.concept_id、board_concepts 模型、层级相关模型
- [ ] 9.6 清理 router.go 中废弃的路由和 handler 引用
- [ ] 9.7 清理 services.go、workers.go、scheduler 中的废弃依赖注入和 worker

## 10. 前端改造

- [ ] 10.1 替换 boardConcepts API client 为 semanticBoards / auxiliaryLabels API client
- [ ] 10.2 修改板块详情页：显示 SemanticBoard composition 和辅助标签筛选 chips
- [ ] 10.3 修改标签卡片组件：显示最多 3 个所属 SemanticBoard
- [ ] 10.4 新增升级建议面板：展示候选簇 + LLM 建议，支持确认/拒绝操作
- [ ] 10.5 新增辅助标签治理 UI：禁用、alias 合并、从 board 移除
- [ ] 10.6 新增回填触发按钮和进度展示
- [ ] 10.7 新增匹配参数配置页面：semantic_board_match_* 阈值和权重表单
- [ ] 10.8 修改 NarrativeBoard UI：保留每日叙事板视图，展示 semantic_board_id / concept_name 来源
- [ ] 10.9 移除旧层级/sector 管理 UI 入口

## 11. 文档更新

- [ ] 11.1 更新 `docs/reference/database/DATA_LIFECYCLE.md`：tag → auxiliary label → SemanticBoard → NarrativeBoard 新链路
- [ ] 11.2 更新 `docs/reference/database/ER_DIAGRAM.md` 和 `DATABASE_FIELDS.md`
- [ ] 11.3 更新 `docs/reference/architecture/backend.md` 和 `data-flow.md`
- [ ] 11.4 更新 `docs/reference/api/_index.md`，补充 semantic board / auxiliary label API
- [ ] 11.5 标记旧 tag hierarchy / board_concepts 文档为废弃或删除

## 12. 集成验证

- [ ] 12.1 端到端测试：文章 → tag 提取（含辅助标签）→ 入库 → SemanticBoard 匹配
- [ ] 12.2 端到端测试：冷启动辅助标签积累 → 手动升级建议 → 用户确认 → 回填 → NarrativeBoard 生成
- [ ] 12.3 端到端测试：一个 event tag 归属多个 SemanticBoard，并在多个 NarrativeBoard 中重复展示
- [ ] 12.4 验证：go build ./... && go test ./... && golangci-lint run ./...
- [ ] 12.5 验证：pnpm lint && pnpm exec nuxi typecheck && pnpm build
