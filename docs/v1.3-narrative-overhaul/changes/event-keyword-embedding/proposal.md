## Why

`hierarchy-concept-fence` 变更上线后冷启动阶段存在两个问题：(1) 旧的 `ExtractAbstractTag` 抽象创建路径没有被删除，让 keyword 靠 LLM 判断捡了残羹，但 event 因 LLM 规则极严格几乎零聚合；(2) event 标签 embedding 把 label + description + 文章上下文揉成单一长文本，不同事件的向量距离被上下文噪声拉远，导致同类事件无法聚类，bootstrap 产生大量孤立点和垃圾 concept。

## What Changes

- **BREAKING**: 删除 `findOrCreateTag` 中 `ExtractAbstractTag` → `processAbstractJudgment` 整条旧的抽象创建链路（Source A 只做标签复用/合并，不再创建 abstract）
- Event 标签新增多关键词 embedding：LLM 在生成 description 同时提取事件关键词（如"美国""伊朗""袭击"），每个关键词单独 embed 为一行 `topic_tag_embeddings`（embedding_type=`event_keyword`），标题行保留为 `semantic`
- **BREAKING**: `topic_tag_embeddings` 唯一索引改为 `(topic_tag_id, embedding_type, text_hash)`，允许同一 tag 多条关键词行
- Event 标签延迟入队：创建时跳过 `ensureTagEmbedding`，等 description + 关键词生成后触发 re-embedding
- Concept 匹配加权：`MatchTagToConcept` 对 event 标签取所有关键词行 + 标题行（权重 x2）的加权平均相似度
- Bootstrap 最小 cluster 5 个 tag，不达标时降级创建 1 个 default concept 兜底

## Capabilities

### New Capabilities
- `event-keyword-embedding`: Event 标签的多关键词语义 embedding 生成与存储，LLM 关键词提取，加权 concept 匹配
- `cold-start-bootstrap`: Bootstrap 最小 cluster 阈值与降级 default concept 创建

### Modified Capabilities
- `tag-hierarchy-quality`: 删除 Source A 中的旧抽象创建路径（ExtractAbstractTag 整条链路）
- `board-concept-management`: Bootstrap 聚类增加最小 cluster 过滤，增加 default concept 创建逻辑
- `tag-to-board-matching`: MatchTagToConcept 支持 event 多关键词加权匹配

## Impact

- `backend-go/internal/domain/tagging/tagger.go`: 删除 LLM 判断 → abstract 创建链路；event 标签跳过初始 embedding 入队
- `backend-go/internal/domain/tagging/abstract_tag_service.go`: 删除 `processAbstractJudgment` 调用路径（调用方移除后清理）
- `backend-go/internal/domain/tagging/abstract_tag_judgment.go`: 清理不再使用的调用路径
- `backend-go/internal/domain/tagging/embedding_queue.go`: event 关键词 embedding 生成逻辑
- `backend-go/internal/domain/tagging/embedding.go`: `buildTagEmbeddingText` event 分支调整；`FindSimilarAbstractTags` 保持使用标题行
- `backend-go/internal/domain/concept/matcher.go`: 多行 embedding 加权平均匹配
- `backend-go/internal/domain/concept/bootstrap.go`: 最小 cluster 过滤 + default concept 降级
- `backend-go/internal/domain/models/topic_graph.go`: `TopicTagEmbedding` 唯一索引变更
- `backend-go/internal/platform/database/postgres_migrations.go`: 索引重建 migration
