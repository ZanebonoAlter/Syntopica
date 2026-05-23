## 1. 数据库 Schema 变更

- [x] 1.1 创建 migration：DROP 旧 UNIQUE 索引 `idx_topic_tag_embeddings_tag_type`，CREATE 新索引 `(topic_tag_id, embedding_type, text_hash)`（幂等检查）
- [x] 1.2 更新 `TopicTagEmbedding` GORM model 的 uniqueIndex 注解匹配新索引
- [x] 1.3 运行 migration 并验证索引变更生效（migration 代码已创建，运行时由应用自动执行）

## 2. 删除旧 ExtractAbstractTag 抽象创建路径

- [x] 2.1 `tagger.go` 中删除 `findOrCreateTag` 的 LLM 判断 → abstract 创建链路（candidates 分支调用 `ExtractAbstractTag` → `ProcessJudgment` → `processAbstractJudgment` 的代码块）
- [x] 2.2 `tagger.go` 中删除 candidate 分支的 merge 处理代码（`result.HasMerge()` 路径），保留为 fall through 到创建新标签
- [x] 2.3 `tagger.go` 中删除 event 专用的 event_fallback 逻辑（已无 candidate 分支需要兜底）
- [x] 2.4 验证 `ExtractAbstractTag` 函数仍被 cleanup 调度器 `ReviewHierarchyTrees` 使用，不删除函数本身
- [x] 2.5 运行 `go build ./...` 确认编译通过，无引用错误

## 3. Event 关键词提取（generateTagDescription 改造）

- [x] 3.1 修改 `generateTagDescription` LLM prompt，对 event 标签增加 `keywords` 输出要求（JSON schema 新增 `keywords: string[]` 字段）
- [x] 3.2 解析 LLM 返回的 keywords，存入 `tag.Metadata["event_keywords"]`
- [x] 3.3 生成完成后调用 `qs.Enqueue(tagID)` 触发 re-embedding
- [x] 3.4 非 event 标签的 `generateTagDescription` 行为不变（回归验证）

## 4. Event 延迟入队

- [x] 4.1 `findOrCreateTag` 中增加 category 判断：event 标签跳过 `ensureTagEmbedding` 调用
- [x] 4.2 `article_tagger.go` 中 event 标签的文章关联后 re-embedding 逻辑保留不变（二次 embedding 仍需要，会用新的 keywords）
- [x] 4.3 验证 event 标签创建后 `PlaceTagInHierarchy` 返回 `"pending_embedding"`，1h 调度器重试后正常

## 5. 多行 Embedding 生成

- [x] 5.1 `embedding_queue.go` `processNext()` 中，event 标签的 semantic embedding 生成改为：标题行（label + description）+ 关键词行（逐一 embed）
- [x] 5.2 关键词行 `embedding_type` 设为 `"event_keyword"`，`text_hash` 按 `SHA256("event_keyword\n" + keyword)` 计算
- [x] 5.3 `buildTagEmbeddingText` event 分支移除文章上下文（`相关报道`）拼接，标题行只含 label + description
- [x] 5.4 `SaveEmbedding` 的 UPSERT 逻辑适配新索引（按 topic_tag_id + embedding_type + text_hash 查重）
- [x] 5.5 非 event 标签的 embedding 生成行为不变（回归验证）

## 6. Concept 匹配加权（MatchTagToConcept 改造）

- [x] 6.1 `matcher.go` 中 event 标签查询所有 `embedding_type IN ('semantic', 'event_keyword')` 的行
- [x] 6.2 计算各行的 cosine similarity 后加权平均：title ×2, keyword ×1
- [x] 6.3 keyword/person 标签匹配逻辑不变（回归验证）
- [x] 6.4 新增常量 `titleEmbeddingWeight = 2.0`、`keywordEmbeddingWeight = 1.0`

## 7. Bootstrap 最小 Cluster + Default Concept

- [x] 7.1 `bootstrap.go` `findConnectedComponents` 返回后过滤 size < 5 的 cluster
- [x] 7.2 所有被过滤的 tag 收集起来，若无任何有效 cluster 则创建 default concept（status=active）
- [x] 7.3 default concept 命名：event→"事件"，keyword→"关键词"，person→"人物"
- [x] 7.4 default concept 创建后调用 `GenerateConceptEmbedding` 生成 embedding

## 8. 验证与清理

- [x] 8.1 运行全量测试：`go test ./...`（关注 tagging/concept 包）
- [x] 8.2 运行质量门禁：`golangci-lint run ./... && go vet ./... && go build ./...`
- [ ] 8.3 数据库验证：确认旧索引已删除、新索引已创建、已有 event 标签 re-embedding 正确生成多行（需部署后验证）
- [x] 8.4 更新 `docs/reference/database/` 中 `topic_tag_embeddings` 索引文档
- [x] 8.5 更新 `docs/reference/architecture/` 标签创建流程文档（删除旧路径描述）
