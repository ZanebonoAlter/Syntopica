## Context

`hierarchy-concept-fence` 变更上线后，实际运行暴露了冷启动阶段的聚合缺陷：

1. **旧路径残留**：Decision 7 要求「Source A 删除 abstract 创建」，但 `findOrCreateTag` 中 `ExtractAbstractTag` → `processAbstractJudgment` 链路仍在运行。该路径不经过 concept 围栏，直接由 LLM 判断创建 abstract（concept_id=NULL）。keyword 因 LLM 规则宽松产生了 30 个 abstract，event 因规则极严格（需因果关联）仅产生 2 个。

2. **Event embedding 质量**：当前 event embedding 把 label + description + article context 揉成单一长文本 embed，不同事件因文章上下文差异导致向量距离偏大，同类事件无法聚类。

3. **Bootstrap 孤立点**：无最小 cluster 限制，单标签也能生成 concept，大量垃圾 concept 冲击用户确认流程。

当前两条路径并存：旧路径（ExtractAbstractTag，不走 concept）实际在工作；新路径（PlaceTagInHierarchy，走 concept 围栏）因无 concept 完全闲置。

## Goals / Non-Goals

**Goals:**
- 删除旧 `ExtractAbstractTag` 抽象创建链路，让 Source A 真正只做标签复用/合并
- Event 标签通过多关键词拆分 embedding 提升语义粒度，改善 concept 匹配和 bootstrap 聚类质量
- Event 标签延迟入队，等 description + keywords 就绪后才生成 embedding（减少空 embedding 的 API 调用）
- Bootstrap 增加最小 cluster 过滤（≥5 tags），不达标时创建 default concept 兜底
- MatchTagToConcept 支持 event 多行关键词 embedding 加权平均匹配

**Non-Goals:**
- 不改变 keyword/person 标签的 embedding 行为
- 不改变 `FindSimilarAbstractTags` 的查询逻辑（仍用标题行）
- 不改变 bootstrap 聚类算法本身（仍用 pgvector 连通图）
- 不新增前端功能（关键词不在 UI 展示）

## Decisions

### Decision 1: 删除 ExtractAbstractTag 整条链路

**选择**: 删除 `findOrCreateTag` 中 `ExtractAbstractTag` → `ProcessJudgment` → `processAbstractJudgment` 的调用（tagger.go:154）。保留 `ExtractAbstractTag` 函数本身（ cleanup 调度器仍在用）。

**理由**: 旧路径绕过 concept 围栏创建 abstract，与 hierarchy-concept-fence 的设计目标冲突。删除后 Source A 只做 exact match → candidates merge → create new tag，职责单一。

**影响**: `findOrCreateTag` 中 candidate 分支的 LLM 判断 → abstract 创建逻辑完全移除，代之以直接 fall through 到创建新标签。cleanup 调度器 `ReviewHierarchyTrees` 中的调用不受影响。

### Decision 2: Event 多关键词 embedding

**选择**: 在 `generateTagDescription` 中让 LLM 同时返回 `description` 和 `keywords`（string array）。keywords 存入 `topic_tags.metadata` JSONB（key=`event_keywords`）。Embedding 生成时：
- 标题行：`embedding_type='semantic'`，文本为 label + description（不含文章上下文）
- 关键词行：`embedding_type='event_keyword'`，每个关键词单独一行，文本为关键词本身
- 每行的 `text_hash = SHA256(embedding_type + "\n" + text)` 唯一区分

**替代方案**: 已排除——所有关键词揉成一个文本 embed 单行（丢失细粒度语义）；新增 `embedding_sub_type` 列（多余，text_hash 已能区分）。

### Decision 3: 唯一索引改为三元组

**选择**: `topic_tag_embeddings` 唯一索引从 `(topic_tag_id, embedding_type)` 改为 `(topic_tag_id, embedding_type, text_hash)`。

**理由**: 同一 tag 的多个关键词行都是 `event_keyword` 类型，需要 text_hash 区分。text_hash 已有字段，不改 schema 结构。

**Migration**: DROP 旧索引 → CREATE 新索引。幂等：检查新索引是否存在后执行。

### Decision 4: Event 延迟入队

**选择**: `findOrCreateTag` 创建 event 标签时跳过 `ensureTagEmbedding` 调用。`generateTagDescription` 完成后触发 `qs.Enqueue(tagID)` 重新入队。

**理由**: 标签创建时 description 为空（async 生成中），embedding 无意义。等 description + keywords 都就绪后再生成 embedding，质量更高且节省一轮 API 调用。

**影响**: `PlaceTagInHierarchy` 第一次调用会返回 `"pending_embedding"`，1h 调度器重试后（embedding 已就绪）正常放置。

### Decision 5: Concept 匹配加权平均

**选择**: `MatchTagToConcept` 对 event 标签查询所有 `embedding_type IN ('semantic', 'event_keyword')` 的行，标题行（semantic）权重 ×2，关键词行权重 ×1，计算与 concept 的加权平均 cosine similarity。

**替代方案**: 取最大值（忽略其他维度语义）；平均值（标题权重不够突出）。加权平均保留标题主导性同时补充关键词语义桥接。

**查询方式**: 加载所有匹配行，Go 内存中计算各行的 similarity 后加权平均，避免复杂 SQL。

### Decision 6: Bootstrap 最小 cluster + default concept

**选择**: `findConnectedComponents` 返回后，过滤掉 size < 5 的 cluster（不调用 LLM，不创建 concept）。所有被过滤的 tag 归属到一个 default concept（per category 1 个）。

**default concept 命名**: 用 category 自动生成，如 `"事件"`(event)、`"关键词"`(keyword)、`"人物"`(person)。status 直接设为 `active`（无需确认），category 隔离。

**理由**: 冷启动时 event 标签分散，孤立点多，强制最小 cluster 避免垃圾 concept。小 tag 集也能通过 default concept 兜底，让 placement 能正常工作。

## Risks / Trade-offs

**[关键词提取质量依赖 LLM]** → 关键词可能不准确或遗漏。缓解：`generateTagDescription` 已有重试机制（最多 3 次），关键词缺失时仍生成标题 embedding。

**[Event embedding 不再包含文章上下文]** → 去除"相关报道"后 embedding 可能丢失部分语义。缓解：多关键词从 description 提炼，description 本身由文章上下文生成，间接保留了上下文信息。标题 embedding 用 label + description 覆盖事件本质。

**[旧路径删除影响现有 abstract]** → 数据库中已有的 30 keyword + 2 event + 2 person abstract（concept_id=NULL）失去来源。缓解：这些 abstract 已存在于 DB，有 parent-child 关系，不影响现有数据。cleanup 调度器会通过 `ReviewHierarchyTrees` 继续维护（merge/move/复用，不创建新的）。

**[索引变更不可逆]** → unique constraint 从二元变三元，允许多行同 embedding_type。缓解：索引重建在 migration 中幂等执行，可回滚。

**[default concept 可能过于宽泛]** → 一个 category 的所有未聚类标签归入同一个 concept。缓解：仅冷启动兜底，后续 bootstrap 可重新聚类。用户可手动创建更精细的 concept 并重新匹配。

## Migration Plan

1. 部署新代码
2. 运行 migration：DROP 旧 UNIQUE 索引 → CREATE 新 `(topic_tag_id, embedding_type, text_hash)` 索引
3. 数据库已有 event 标签会被重入队（`generateTagDescription` 已完成）或等待下次 re-embedding
4. 手动运行 bootstrap 以生成 concept（含 default concept 降级）
5. 回滚：恢复旧索引 → 回退代码

## Open Questions

- 关键词权重 x2 是否最优？可能需要在运行中根据匹配准确率调整（当前设为可配置常量）
- default concept 是否应该在用户确认后仍可被 bootstrap 重新聚类覆盖？当前设计：后续 bootstrap 可重新生成 concept，用户手动 merge 或 deactivate default
