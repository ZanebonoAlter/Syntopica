## 1. 数据模型与迁移

- [x] 1.1 创建 `semantic_labels` 模型：id, label, slug, embedding(vector), label_type, aliases(jsonb), ref_count, description, display_order, source, status, protected, created_at, updated_at
- [x] 1.2 创建 `topic_tag_semantic_labels` 关联模型：topic_tag_id, semantic_label_id（仅辅助标签）
- [x] 1.3 创建 `topic_tag_board_labels` 关联模型：topic_tag_id, semantic_board_id, score, match_reason, created_at, updated_at
- [x] 1.4 创建 `board_composition` 关联模型：board_id, auxiliary_label_id
- [x] 1.5 迁移 `narrative_boards`：新增 `semantic_board_id`，逐步移除 `abstract_tag_id` / `board_concept_id` 依赖
- [x] 1.6 编写数据库迁移：新建 4 张表，添加 slug、label_type、status、embedding、topic_tag_id、semantic_board_id 索引
- [x] 1.7 编写数据库迁移：删除旧 board_concepts、topic_tags.concept_id、层级相关表/索引（不做历史数据迁移）
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

- [x] 3.1 实现禁用辅助标签：status=disabled，后续匹配和升级候选排除
- [x] 3.2 实现手动合并 alias：迁移 topic_tag_semantic_labels，积累 aliases
- [x] 3.3 实现从 board_composition 移除辅助标签
- [x] 3.4 编写治理能力测试：禁用、alias 合并、composition 移除不自动回填

## 4. SemanticBoard 匹配逻辑

- [x] 4.1 创建 semantic_board_matching.go：读取 tag 辅助标签和 active SemanticBoard composition
- [x] 4.2 实现直接命中匹配（tag 辅助标签 ∈ board 构成标签）
- [x] 4.3 实现间接匹配：计算命中率（hit_count / tag 辅助标签总数）和 max_sim
- [x] 4.4 实现三规则挂载判断：命中率、max_sim、加权综合分
- [x] 4.5 实现多 board 挂载：按分数排序，默认最多 3 个，写入 topic_tag_board_labels
- [x] 4.6 实现匹配参数读取：从 ai_settings 读取 semantic_board_match_* 配置
- [x] 4.7 编写匹配逻辑单元测试：覆盖直接命中、三规则、多 board 截断、无匹配、冷启动无 board

## 5. SemanticBoard 升级与冷启动建议

- [x] 5.1 创建 semantic_board_upgrade.go：收集 ref_count ≥ semantic_board_upgrade_ref_count_threshold 的候选辅助标签
- [x] 5.2 实现候选 + 已有 SemanticBoard 的 embedding 预聚类（cosine 距离阈值 0.7）
- [x] 5.3 实现簇内 co-tag 事件补充：时间窗口 30 天、共现频率排序 top20、embedding 去重（>0.85）、硬上限 15
- [x] 5.4 实现 LLM 建议 prompt：每个簇的标签列表 + 事件上下文 → merge_into_existing / create_new / skip
- [x] 5.5 实现建议结果持久化或返回结构：用户确认前不写 SemanticBoard/board_composition
- [x] 5.6 实现用户确认执行：创建新 SemanticBoard 或更新已有 board_composition
- [x] 5.7 编写升级机制测试：覆盖冷启动无 board、候选收集、聚类、LLM mock、确认执行

## 6. 回填队列

- [x] 6.1 创建 semantic_board_backfill.go：支持 all、unassigned、board 三种回填模式
- [x] 6.2 实现异步队列逐个执行 board 匹配并重写 topic_tag_board_labels
- [x] 6.3 添加回填进度查询和失败记录
- [x] 6.4 编写回填测试：覆盖全量、无归属、指定 board、幂等重跑

## 7. NarrativeBoard 生成改造

- [x] 7.1 删除 abstract tree → hotspot NarrativeBoard 路径，不再从 topic_tag_relations 生成每日板
- [x] 7.2 改造每日板输入收集：按日期、scope、semantic_board_id 收集归属 event tags
- [x] 7.3 改造 NarrativeBoard 创建：写入 semantic_board_id、event_tag_ids、scope_type、scope_category_id
- [x] 7.4 改造 prev_board_ids 匹配：按 semantic_board_id + scope + 前一日日期续接
- [x] 7.5 改造 board narrative context：使用 SemanticBoard label/description
- [x] 7.6 允许同一 event tag 出现在多个 NarrativeBoard 中
- [x] 7.7 编写叙事生成测试：覆盖无 board 冷启动、分类 scope、global scope、多 board 重复、prev 续接

## 8. API 层

- [x] 8.1 实现 SemanticBoard CRUD API：GET/POST/PUT/DELETE /api/semantic-boards
- [x] 8.2 实现 board composition API：查看构成、移除辅助标签
- [x] 8.3 实现辅助标签池查询和治理 API：GET /api/auxiliary-labels、disable、merge-alias
- [x] 8.4 实现升级候选查看 API：GET /api/semantic-boards/upgrade-candidates
- [x] 8.5 实现升级建议 API：POST /api/semantic-boards/upgrade-suggest
- [x] 8.6 实现升级确认 API：POST /api/semantic-boards/upgrade-execute
- [x] 8.7 实现回填触发和进度 API：POST /api/semantic-boards/backfill、GET /api/semantic-boards/backfill/:id
- [x] 8.8 实现匹配参数配置 API：GET/PUT /api/semantic-boards/matching-config
- [x] 8.9 实现 tag 关联查询 API：GET /api/tags/:id/auxiliary-labels 和 GET /api/tags/:id/semantic-boards
- [x] 8.10 注册所有新路由到 router.go，并移除层级/旧 concept 路由

## 9. 删除废弃代码

- [x] 9.1 删除层级体系文件：hierarchy_template.go, hierarchy_config.go, hierarchy_placement.go, hierarchy_cleanup.go, hierarchy_dedup.go, hierarchy_aggregation.go, hierarchy_handler.go, hierarchy_orchestration.go, hierarchy_prompts.go 及对应测试
- [x] 9.2 删除抽象标签文件：abstract_tag_crud.go, abstract_tag_handler.go, abstract_tag_hierarchy.go, abstract_tag_judgment.go, abstract_tag_service.go, abstract_tag_tree.go, abstract_tag_update_queue.go 及对应测试
- [x] 9.3 删除相关队列：adopt_narrower_queue.go/handler, multi_parent_resolve_queue.go, tree_bridge.go 及对应测试
- [x] 9.4 删除旧 concept 包文件：concept/bootstrap.go, concept/matcher.go, concept/suggest.go 及旧 BoardConcept CRUD
- [x] 9.5 清理 models 中的旧字段：topic_tags.concept_id、board_concepts 模型、层级相关模型
- [x] 9.6 清理 router.go 中废弃的路由和 handler 引用
- [x] 9.7 清理 services.go、workers.go、scheduler 中的废弃依赖注入和 worker
- [x] 9.8 删除 keyword sub_type 契约与持久化字段：prompt 示例和说明中移除、tagExtractionSchema() 移除、parseExtractedTags raw struct 移除、validateSubType() 函数删除、ExtractedTag.SubType 删除、TopicTag.SubType 删除、tagger.go/article_tagger.go 引用删除、models/topic_graph.go SubType 字段删除；新增迁移删除 idx_topic_tags_sub_type 和 topic_tags.sub_type，或明确保留 DB 旧列但模型不再读写
- [x] 9.9 删除 tag_extraction confidence 输出字段：prompt 示例中移除、tagExtractionSchema() 移除、parseExtractedTags 移除（TopicTag.Score 默认 0.7）、ExtractedTag.Confidence 删除；确认不再把 LLM 自评置信度当业务分数，前端 AI 分析面板置信度展示移除或改为非 tag_extraction 来源
- [x] 9.10 重写 tagExtraction prompt/schema：拆分为 event/person 与 keyword 两类中文模板示例，所有 prompt 说明、字段描述、正反例均使用中文；消除 sub_type/confidence/auxiliary_labels 字段归属歧义；auxiliary_labels 增加正反例对比约束
- [x] 9.11 parseAuxiliaryLabels 增加旧字符串数组兜底解析：将 string[] 临时转为 {label, description: label} 对象后继续走现有 description 质量校验；该兼容仅用于产生清晰错误或保守拒绝，不放宽 event/person 必须提供有效 description 的规格
- [x] 9.12 验证：go build ./... && go test ./... && golangci-lint run ./...
- [x] 9.14 删除 tag_extraction evidence 输出字段：prompt 示例和说明中移除、tagExtractionSchema() 移除、parseExtractedTags raw struct 移除、ExtractedTag.Evidence 删除；evidence 无下游消费者（不入库、不入 domain TopicTag、前端不使用）
- [x] 9.15 extractCandidates LLM 调用 + parseExtractedTags 格式转换失败时最多重试 3 次，3 次均失败后再降级 heuristic
- [x] 9.13 验证：pnpm lint && pnpm exec nuxi typecheck && pnpm build

## 10. 前端改造

- [x] 10.1 替换 boardConcepts API client 为 semanticBoards / auxiliaryLabels API client
- [x] 10.2 修改板块详情页：显示 SemanticBoard composition 和辅助标签筛选 chips
- [x] 10.3 修改标签卡片组件：显示最多 3 个所属 SemanticBoard
- [x] 10.4 新增升级建议面板：展示候选簇 + LLM 建议，支持确认/拒绝操作
- [x] 10.5 新增辅助标签治理 UI：禁用、alias 合并、从 board 移除
- [x] 10.6 新增回填触发按钮和进度展示
- [x] 10.7 新增匹配参数配置页面：semantic_board_match_* 阈值和权重表单
- [x] 10.8 修改 NarrativeBoard UI：保留每日叙事板视图，展示 semantic_board_id / concept_name 来源
- [x] 10.9 移除旧层级/sector 管理 UI 入口
- [x] 10.10 调整升级建议面板：确认单个 create_new / merge_into_existing 建议后不关闭弹窗，而是移除或标记该建议为已处理，并保留剩余建议继续处理
- [x] 10.11 升级建议面板增加“重新生成建议”入口：已有建议时也可重新调用 upgrade-suggest，并用新结果替换当前建议列表
- [x] 10.12 确认执行至少一个升级建议后，在面板内提示用户可手动触发匹配回填；不自动启动回填

## 11. 手动 Board 辅助标签推荐与 Composition 管理

- [x] 11.1 后端：新增 suggest-auxiliaries API（仅用于人工推荐；使用 board label + description embedding vs active 辅助标签 storage embedding cosine similarity 排序 + 分页 + 搜索过滤，不参与自动匹配规则）
- [x] 11.2 后端：新增 POST /:id/composition 添加接口（幂等写入 board_composition，不自动回填历史 tag-board 归属）
- [x] 11.3 后端：编写推荐 API 和添加接口的单元测试
- [x] 11.4 前端：新增可复用组件 AuxiliaryLabelPicker.vue（推荐列表 + 搜索 + 分页 + 勾选）
- [x] 11.5 前端：改造 AddSemanticBoardDialog.vue（嵌入 AuxiliaryLabelPicker，选中 IDs 随创建请求提交）
- [x] 11.6 前端：改造 BoardCompositionPanel.vue（增加"添加"按钮，弹出 AuxiliaryLabelPicker，支持手动触发推荐）
- [x] 11.7 前端：编辑已有 board 时支持手动触发推荐（复用 AuxiliaryLabelPicker，排除已关联标签）
- [x] 11.8 前端：补充 API client 新接口（suggestAuxiliaries、addComposition）
- [x] 11.9 前端：添加/移除 board_composition 后提示用户可手动触发 board 回填，不自动启动回填
- [x] 11.10 验证：后端 go build + go test + golangci-lint
- [x] 11.11 验证：前端 pnpm lint + nuxi typecheck + pnpm build

## 12. 辅助标签 Description 增强与 Embedding 分离

- [x] 12.1 数据模型：semantic_labels 新增 merge_embedding(vector) 字段；现有 embedding 字段保留为 storage embedding
- [x] 12.2 迁移与索引：添加 merge_embedding 迁移、维度检查和必要索引；更新模型与迁移测试
- [x] 12.3 改造 tagger prompt：keyword 标签不再要求 auxiliary_labels；event/person 的辅助标签改为带 description 的对象格式 {"label": "伊朗", "description": "中东地区国家"}
- [x] 12.4 改造 ExtractedTag / TopicTag 类型：auxiliary_labels 解析为 []struct{Label, Description}；keyword 允许空 auxiliary_labels；event/person 必须 3-5 个对象
- [x] 12.5 增加辅助标签 description 质量校验：非空、长度上限、不能只重复 label，拒绝明显泛词
- [x] 12.6 改造 article_tagger.go：keyword tag 走 KeywordDirectToPool 逻辑（用 tag label + 同步提取的 tag description 直入辅助池）；event/person 走 AttachAuxiliaryLabels 新逻辑
- [x] 12.7 改造 auxiliary_label_service.go：ResolveAuxiliaryLabel 接收 label + description 参数，L3 新建时写入 description 字段
- [x] 12.8 实现 embedding 分离：L2 merge 判断使用 merge_embedding（label-only）；L3 新建同时写入 merge_embedding（label-only）和 embedding（label + description storage embedding）
- [x] 12.9 改造 defaultAuxiliaryLabelEmbedder：支持 label-only 和 label+description 两种 embedding 模式，并在 ai_call_logs 中区分 operation
- [x] 12.10 更新 board 推荐、board matching、upgrade clustering、backfill：继续使用 storage embedding（semantic_labels.embedding），不使用 merge_embedding
- [x] 12.11 更新已有测试覆盖：keyword 直入、description 写入、merge_embedding vs storage embedding 分离、event/person 对象格式解析
- [x] 12.12 验证：go build ./... && go test ./... && golangci-lint run ./...

## 13. 文档更新

- [ ] 13.1 更新 `docs/reference/database/DATA_LIFECYCLE.md`：tag → auxiliary label → SemanticBoard → NarrativeBoard 新链路
- [ ] 13.2 更新 `docs/reference/database/ER_DIAGRAM.md` 和 `DATABASE_FIELDS.md`，补充 merge_embedding/storage embedding 字段语义
- [ ] 13.3 更新 `docs/reference/architecture/backend.md` 和 `data-flow.md`
- [ ] 13.4 更新 `docs/reference/api/_index.md`，补充 semantic board / auxiliary label / suggest-auxiliaries / composition API
- [ ] 13.5 标记旧 tag hierarchy / board_concepts 文档为废弃或删除

## 14. Tag 提取拆分为双调用

- [x] 14.1 拆分 `buildExtractionSystemPrompt` 为 `buildEventPersonPrompt` 和 `buildKeywordPrompt`：event/person 聚焦事件和人物识别 + auxiliary_labels 3-5 个语义锚点；keyword 聚焦持久辨识度实体/术语 + description
- [x] 14.2 拆分 `tagExtractionSchema` 为 `eventPersonExtractionSchema` 和 `keywordExtractionSchema`：event/person schema 中 `auxiliary_labels` 置为 required，keyword schema 移除 `auxiliary_labels` 字段、`description` 置为 required
- [x] 14.3 拆分 `extractCandidates` 为 `extractEventPersonCandidates` 和 `extractKeywordCandidates`，各自带独立的重试逻辑（maxRetries=3）和 metadata（`tag_extraction_event_person` / `tag_extraction_keyword`）
- [x] 14.4 重写 `ExtractTags`：并行发起两个提取调用但不使用 fail-fast；独立收集两个分支结果和错误，合并成功分支
- [x] 14.5 定义合并策略：合并后最多 5 个 tag、keyword 最多 3 个；同 slug 跨分类重复时按 person > event > keyword 保留
- [x] 14.6 定义部分失败策略：event/person 调用全败时不产生 event/person tag（不降级 heuristic event）；keyword 调用全败时可降级 heuristic keyword 作为展示兜底，但默认不进入辅助标签池；双分支全败时沿用整体 heuristic fallback
- [x] 14.7 拆分 `parseExtractedTags` 为 `parseEventPersonTags` 和 `parseKeywordTags`：共享 JSON sanitize/wrapper/code-fence 容错，各自只解析自己类型的 raw struct，校验规则独立
- [x] 14.8 更新单元测试：覆盖双分支部分失败、非 fail-fast、合并数量上限、跨分类去重优先级、keyword heuristic 不入辅助池
- [ ] 14.9 验证：go build ./... && go test ./... && golangci-lint run ./...

## 15. 集成验证

- [ ] 15.1 端到端测试：文章 → tag 提取（event/person 带 description 辅助标签，keyword 直入）→ 入库 → SemanticBoard 匹配
- [ ] 15.2 端到端测试：冷启动辅助标签积累 → 手动升级建议 → 用户确认 → 回填 → NarrativeBoard 生成
- [ ] 15.3 端到端测试：一个 event tag 归属多个 SemanticBoard，并在多个 NarrativeBoard 中重复展示
- [ ] 15.4 端到端测试：手动创建/编辑 SemanticBoard → 推荐辅助标签 → 写入 composition → 手动 board 回填
- [ ] 15.5 验证：merge_embedding 只影响 L2 merge，storage embedding 继续驱动推荐、匹配、聚类、回填
- [ ] 15.6 端到端测试：升级建议面板支持逐项确认多个建议、保持弹窗打开，并支持重新生成建议
- [ ] 15.7 验证：go build ./... && go test ./... && golangci-lint run ./...
- [ ] 15.8 验证：pnpm lint && pnpm exec nuxi typecheck && pnpm build
