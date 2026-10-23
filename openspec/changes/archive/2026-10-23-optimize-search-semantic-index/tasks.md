# Tasks: optimize-search-semantic-index

## 1. 数据库索引与迁移

- [x] 1.1 `postgres_migrations.go` 新增 `TopicTagIdentityHalfvecIndexName` 常量 + `TopicTagIdentityHalfvecIndexDDL(dim)` 单一 DDL 事实源 + `vectorColumnDimension(typeStr)` 解析器；迁移 `20261023_0001` 读列 `format_type`、解析维度、≤4000 时 `SET LOCAL maintenance_work_mem='256MB'` 后建部分 HNSW 索引；已存在但 INVALID 时先 DROP 再建。验证：`cd backend-go && go test ./internal/platform/database -run 'TestVectorColumnDimension|TestTopicTagIdentityHalfvecIndexDDL' -v` 通过
- [x] 1.2 迁移接入 `postgresMigrations()` 的 append 链（版本排序自动生效）。验证：`go test ./internal/platform/database -run TestPostgresMigrations -v` 通过（或既有迁移计数测试通过）

## 2. 语义召回改写

- [x] 2.1 `semantic.go` 新增 `matchTagsANN`：`hnsw.ef_search=128` + `ORDER BY (embedding::halfvec(N)) <=> ?::halfvec(N) LIMIT 128` 取候选 ID，再按候选 ID 用 `1 - (embedding <=> ?::vector)` 精确复算；`matchTagsExact` 保留旧全表查询作为 >4000 维/ANN 失败的回退；`rankMatchedTags` 在 Go 侧按 tag 去重（max）、`MinCosine` 过滤、降序取 `TopN`。验证：`go test ./internal/reader/search -run TestSemanticSearch -v` 通过（PG testcontainer）
- [x] 2.2 语义路径分阶段计时：embedding / ANN 候选 / 精确复算 / 文章召回；超阈值 warn。验证：`go test ./internal/reader/search -run TestSemanticSearch -v` 通过且日志无异常刷屏

## 3. 维度守卫

- [x] 3.1 `tagmanagement/service/core/embedding.go` 的 `ensureVectorDimension`：ALTER 前 drop 旧 float32 索引与 halfvec 索引；dim ≤2000 重建旧索引；dim ≤4000 用共享 DDL 重建 halfvec identity 索引；>4000 跳过并记日志。验证：`go build ./...` 通过 + `go test ./internal/tagmanagement/service/core -run TestSaveEmbedding -v` 通过（不破坏现有保存链路）

## 4. 测试

- [x] 4.1 `semantic_pg_test.go` 新增：索引存在且 `indisvalid=true`；同 tag 多 text_hash 行去重取 max；160 标签 ANN top-8 与暴力精确 top-8 一致。验证：`go test ./internal/reader/search -run 'TestSemanticSearch.*PG' -v` 全绿
- [x] 4.2 既有语义边界测试（cosine 0.54/0.55/0.56、top_n 8/9、embedding 失败/超时、零命中、归档口径）不回归。验证：`go test ./internal/reader/search` 全绿

## 5. 文档

<!-- doc-impact: database, flow -->

- [x] 5.1 `docs/reference/database/tables/topic-tags.md` §4.2：更新维度事实（运行时维度/当前 2560，不再写死 4096）、新增 halfvec identity HNSW 索引说明与名称。
- [x] 5.2 `docs/reference/database/tables/_conventions.md`：更新「HNSW 仅 ≤2000」总述为「float32 HNSW ≤2000；halfvec HNSW ≤4000（identity 部分索引）」；向量维度表补 `topic_tag_embeddings.embedding` 的运行时维度与 halfvec 索引。
- [x] 5.3 `docs/reference/flow/reading.md`：§站内搜索语义路补「ANN 候选 + 精确复算 + 索引」说明；代码入口迁移列表补 `20261023_0001`；变更溯源表补 2026-10-23 行。

## 6. 验证

- [x] 6.1 `cd backend-go && golangci-lint run ./internal/reader/search/... ./internal/platform/database/... ./internal/tagmanagement/service/core/...`，期望 0 issues
- [x] 6.2 `cd backend-go && go vet ./internal/reader/search/... ./internal/platform/database/... ./internal/tagmanagement/service/core/...`，期望无输出
- [x] 6.3 `cd backend-go && go build ./...`，期望通过
- [x] 6.4 `cd backend-go && go test ./internal/reader/search ./internal/platform/database ./internal/tagmanagement/service/core`，期望全绿（PG testcontainer 需 Docker）
- [x] 6.5 真库只读核对：部署后 `EXPLAIN (ANALYZE, BUFFERS)` 新候选查询走 `Index Scan using idx_topic_tag_embeddings_identity_halfvec_hnsw`（冷 171ms / 热 7.5ms）；连续 5 次 `GET /api/search?q=英伟达` 全部 `semantic.skipped=false` 且返回 5 个语义标签，tookMs 首查 4027ms（冷启动）/ 后续 325/308/287/284ms（首查 4.0s 为关键词+索引冷缓存，非语义超时；语义在 3s 内完成）。索引 valid / 114MB，迁移 20261023_0001 已记录。
- [x] 6.6 `bash scripts/harness/scenario-trace.sh openspec/changes/optimize-search-semantic-index`，期望退出码 0（skip_specs 无 Scenario）
- [x] 6.7 `bash scripts/harness/check-tasks-tail.sh openspec/changes/optimize-search-semantic-index`，期望退出码 0
- [x] 6.8 `bash scripts/harness/archive-readiness.sh optimize-search-semantic-index`，期望全绿（2026-10-23 六项全绿后归档）

### Scenario → 测试文件映射

（本 change `skip_specs: true`，无 delta Scenario；语义行为契约由既有 `article-search` 主 spec 覆盖，本次以 PG 集成测试对账。）

## 7. Review

- [x] 7.1 派子线程聚焦 review（审查制品 + diff），意见按 High/Medium/Low 处置后 `review.md` 建账
