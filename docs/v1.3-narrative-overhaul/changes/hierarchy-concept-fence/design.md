## Context

标签层级模板系统已于 2026-05-10 上线，运行至 5/13 暴露严重质量问题：abstract 根节点无限膨胀（231 个"美伊"相关 abstract），PlaceTagInHierarchy 覆盖率仅 6%，深度违规率 50-76%。根因是三个标签创建来源互相打架且 abstract 创建无边界约束。

本设计引入 `board_concept` 作为"围栏"约束 abstract 创建范围，重写放置逻辑为通用 depth-based N 层泛化设计，全删旧数据从零重建。

## Goals / Non-Goals

**Goals:**
- abstract 创建限定在 concept 围栏内，防止根节点无限膨胀
- 通用 depth-based 放置支持任意 N 层模板
- concept bootstrap 从标签 embedding 聚类自动生成（手动触发）
- Source A 只做标签复用（merge），不再创建 abstract
- 所有放置逻辑通过 depth 参数化，废弃 Level 概念
- concept 代码从 narrative 包抽取为独立包

**Non-Goals:**
- 前端 concept 管理 UI（后续按需添加）
- 修改现有模板定义（5 个固定模板不变）
- 新增 embedding 类型（现有 Identity + Semantic 足够）
- embedding 生成机制本身的优化
- 旧文章重跑 tagging

## Decisions

### Decision 1: 全删旧数据从零重建

**选择**: 删除所有 topic_tags (source='abstract')、topic_tag_relations、narrative_boards、board_concepts。非 abstract 标签也删除。

**替代方案**: 渐进式迁移（保留旧 abstract，逐步关联 concept）。

**理由**: 旧 abstract 质量极差（1725 个中仅 160 active，891 merged），迁移成本高于重建。全删消除了所有向后兼容负担，数据模型变更更简洁。旧文章不重跑，只处理新文章。

### Decision 2: concept 按 category 隔离

**选择**: 每个 category (event/keyword/person) 有独立的 concept 集。board_concepts 新增 `category` 字段。

**替代方案**: 跨 category 共享 concept（一个"芯片产业"概念同时覆盖 event 和 keyword）。

**理由**: 层级模板是 per-category 的（event 3 层、person 2 层），concept 作为围栏应与模板对齐。三种标签的语义空间差异大，聚类质量在同类内更高。

### Decision 3: concept 状态模型

**选择**: `board_concepts` 表的 `is_active` 替换为 `status` 字段（pending/active/inactive/merged）。

**状态机**: `pending → active → inactive`（用户停用），`active → merged`（合并到其他 concept），`pending → inactive`（忽略不需要的 suggestion）。

**理由**: bootstrap 聚类生成的 concept 需要 pending 状态等待用户确认。单一 Status 字段避免与 IsActive 不同步的 bug。

### Decision 4: topic_tags 新增 concept_id

**选择**: `topic_tags` 新增 `concept_id *uint` 可空字段，仅 abstract 标签关联 concept。

**替代方案**: 新增关联表 `topic_tag_concepts(tag_id, concept_id)`。

**理由**: PRD 明确 concept 是"围栏"，一个 abstract 不跨两个围栏。concept_id 在 abstract 创建时设定，之后不变（"出生证明"而非"当前归属"）。

### Decision 5: concept 包抽取

**选择**: 新建 `domain/concept/` 包，从 `narrative` 迁移 concept 相关代码（matcher、service、handler、embedding、bootstrap）。

**包内容**:
- `service.go` — concept CRUD
- `matcher.go` — MatchTagToConcept（复用已有 semantic embedding）
- `bootstrap.go` — pgvector 聚类 + LLM 命名
- `handler.go` — API 路由
- `embedding.go` — concept embedding 生成

**依赖方向**: `narrative/ → import concept/`, `tagging/ → import concept/`

**理由**: concept 是层级放置的核心依赖，同时被 narrative（board 生成）和 tagging（层级放置）使用。独立包消除循环依赖风险。

### Decision 6: MatchTagToConcept 复用已有 embedding

**选择**: 从 `topic_tag_embeddings` 表读取 tag 的 semantic embedding，和 concept embedding 做 cosine similarity。不重新调用 embedding API。

**替代方案**: 每次调用时重新生成 embedding（现有实现）。

**理由**: 放置流程是热路径（每个新标签都会调用），省掉一次 embedding API 调用有实际价值。tag 的 semantic embedding 信息量更丰富（含 description + aliases + context），匹配质量不会更差。

### Decision 7: Source A 删除 abstract 创建

**选择**: `findOrCreateTag` 中 `HasAbstract()` 路径、`createChildOfAbstract` 函数、abstract co-tag 扩展全部删除。Source A 只保留 cache hit → exact reuse → candidates merge → create new tag → `go PlaceTagInHierarchy()`。

**替代方案**: Source A 的 abstract 创建也加 concept 约束。

**理由**: Source A 同步执行时 tag 的 embedding 可能还没生成，MatchTagToConcept 无法工作。删除后职责清晰：Source A 管标签复用（merge），Source B 管层级放置（abstract）。

### Decision 8: anchor 信号优先级

**选择**: cotag 优先（文章共现），不够再用 embedding 补充。

**处理流程**:
1. cotag 找和新标签在同一篇文章的标签 → 筛出有 parent 的 → 有效 anchor
2. 有效 anchor < 2 时，用 semantic embedding 找相似标签 → 筛出有 parent 的 → 补充 anchor
3. top-1 anchor sim ≥ 0.85 → 直接跟随（无 LLM）
4. top-1 anchor sim ∈ [0.70, 0.85) → 取 top-3 anchor 的 parent → 若共识则跟随，若分歧则 LLM 投票
5. 无有效 anchor → fall through 到 abstract embedding 匹配

**孤儿标签处理**: 无 parent 的标签不能当 anchor，各自独立放置，靠 dedup 兜底。

### Decision 9: bootstrap 聚类策略

**选择**: pgvector 原生方案——加载同 category 所有标签的 SemanticEmbedding，通过 pgvector `<=>` 找近邻构建连通图，连通分量即为 cluster。每个 cluster 的标签列表给 LLM 命名生成 pending concept。

**触发**: 仅手动触发（POST `/api/hierarchy/concepts/bootstrap`）。

**输入**: 指定 category 的所有活跃标签的 semantic embedding。

**替代方案**: Go 内实现 k-means（需指定 k）、调用 Python sklearn（引入新依赖）。

**理由**: pgvector 已有基础设施，无需新依赖。连通图方式不需要预先指定 cluster 数量，由距离阈值自然决定。LLM 只负责命名（擅长的事），不做分组（容易遗漏）。

### Decision 10: 通用 depth-based 放置

**选择**: 所有放置/聚合/去重函数用 `depth` 参数化，不硬编码层级名。

**关键函数替换**:
- `placeTagAtLevel(child, tmpl, targetDepth, concept)` → 替代 `placeTagAtL2` + `placeTagAtL1ForParent`
- `resolveParent(child, candidates, existing, tmpl, levelDef)` → 替代 `resolveL2Parent` + `resolveL1Parent`
- `buildMatchPrompt(child, candidates, tmpl, levelDef)` → 替代 4 个硬编码 prompt
- `dedupAtDepth(tag, depth, concept)` → 替代 `dedupL2` + `dedupL1`

**depth 语义**: `getTagDepthFromRoot(tagID)` 返回 BFS 向上跳数，0=无 parent，越大越靠近根。`tmpl.Levels[depth]` 直接索引模板定义。

**删除**: `GetTagLevel` / `GetTagLevelByID` / `ResolveLevelFromDepth` / `maxHierarchyDepth` 常量。

### Decision 11: 调度器拆分

**选择**: 新增 1h 间隔 `TagHierarchyPlacementScheduler`（RetryOrphanPlacements + AggregateOrphanTags），保留 24h `TagHierarchyCleanupScheduler`。

**Placement scheduler (1h)**:
- Phase 3.7: RetryOrphanPlacements（>10min 无 parent 的叶标签）
- Phase 3.8: AggregateOrphanTags（concept-aware 批量向上聚合）

**Cleanup scheduler (24h)**:
- Phase 1-3: 数据清理
- Phase 3d: CleanupTemplateViolations（自动修复）
- Phase 4: adopt_narrower（per-template depth 检查）
- Phase 6: ReviewHierarchyTrees（只做 merge/move/复用，不创建 abstract）

## Risks / Trade-offs

**[冷启动延迟]** → concept 确认前标签以孤儿状态积累。缓解：手动触发 bootstrap + 手动重跑当日文章可快速积累标签。预计几小时内完成 bootstrap 到 active 的循环。

**[聚类质量依赖 embedding]** → Semantic embedding 包含 article context，可能让不同主题标签因共享文章被拉到一起。缓解：连通图的距离阈值可配置，LLM 命名时有二次审查机会，用户确认前都是 pending 状态。

**[concept 包迁移影响 narrative]** → narrative 的 concept 代码全部迁移，可能引入 regression。缓解：narrative 的 board 生成集成测试覆盖 MatchTagToConcept 路径。

**[全删不可逆]** → 旧标签数据永久丢失。缓解：用户已确认旧数据不要，手动重跑新文章可恢复核心标签。全删前建议备份数据库。

**[concept 数量自然上限不明确]** → 聚类可能产生过多或过少的 concept。缓解：手动触发 + 用户确认环节过滤异常。LLM prompt 引导 5-8 个 concept 的粒度。

## Migration Plan

1. 备份数据库
2. 执行全删脚本：删除 topic_tags (所有)、topic_tag_relations、board_concepts、narrative_boards、narrative_summaries、article_topic_tags
3. 运行 GORM AutoMigrate 更新表结构（topic_tags +concept_id, board_concepts +category, -is_active +status）
4. 部署新代码
5. 手动重跑当日文章 → 标签积累
6. 手动触发 bootstrap → 确认 concept
7. 新文章自动进入正常流程

回滚：恢复数据库备份 + 回退代码版本。

## Open Questions

- 聚类距离阈值的具体数值需要在实现时调参（建议初始值 0.65，通过 bootstrap 测试调整）
- anchor 的 cotag 查询性能：当标签关联大量文章时 SQL 可能变慢，可能需要加索引或限制查询范围
