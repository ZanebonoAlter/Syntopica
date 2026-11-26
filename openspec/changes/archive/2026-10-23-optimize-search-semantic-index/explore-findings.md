
## 语义超时根因：标签全表余弦扫描 3.74s

语义超时根因（真库实测 2026-10-23）：`backend-go/internal/reader/search/semantic.go` 的标签召回 SQL（`GROUP BY e.topic_tag_id + MAX(1-(e.embedding <=> ?::vector)) + HAVING + LIMIT`）在 14,658 条 identity 行上 Seq Scan + TOAST detoast，EXPLAIN ANALYZE 3.74s（shared hit=48,706 read=33,906），吃满 `SemanticSearcher.timeout=3s`。`topic_tag_embeddings.embedding` 是 `vector(2560)`，TOAST 总大小 713MB；float32 HNSW 因 pgvector 2000 维上限建不了，现存 `tmp_hnsw_test` 为 INVALID。embedding 短 query 非缓存 miss 平均 ~1.4s（本地 Qwen3-Embedding-4B），不是超时主因。

**引用**：backend-go/internal/reader/search/semantic.go:SemanticSearcher.Search、backend-go/internal/tagmanagement/service/core/embedding.go:ensureVectorDimension

<!-- pinned 2026-10-23T11:05:39Z -->

## halfvec HNSW 构建/查询实测与维度守卫

halfvec HNSW 实测（真库临时表，pgvector 0.8.3）：`CREATE TEMP TABLE ... SELECT embedding::halfvec(2560)` 14,658 行；`maintenance_work_mem=256MB` 建 `USING hnsw (embedding_h halfvec_cosine_ops)` 28.7s、索引 114MB；`ORDER BY embedding_h <=> ?::halfvec(2560) LIMIT 50` 走 Index Scan，执行 15ms（冷 local buffers）。`maintenance_work_mem=64MB` 也能建但 72.7s + NOTICE「hnsw graph no longer fits」。float32 HNSW 直接报 `column cannot have more than 2000 dimensions for hnsw index`。迁移在事务内 `SET LOCAL maintenance_work_mem='256MB'` 后 `CREATE INDEX IF NOT EXISTS`；`ensureVectorDimension` 需在 ALTER 前 drop 该表达式索引（依赖 typmod），≤4000 重建，>4000 跳过回退旧扫描。

**引用**：backend-go/internal/platform/database/postgres_migrations.go:postgresMigrations、backend-go/internal/tagmanagement/service/core/embedding.go:ensureVectorDimension

<!-- pinned 2026-10-23T11:05:39Z -->

## 迁移/PG 测试接线点

测试与迁移接线：PG 集成测试在 `backend-go/internal/reader/search/semantic_pg_test.go`（`testutil.SetupTestDB` 会跑 `database.RunMigrations`，golden schema 的 embedding 维度由 `goldenVectorDim` 实测，当前 2560；`seedTagWithEmbeddingPG` 用 `cosineVec(c, dim)` 构造 [c, sqrt(1-c²), 0...]）。迁移版本排序由 `migrationsSorted()` 按 Version 字典序；`postgresMigrations()` 尾部 append 链在 `postgres_migrations.go:2442-2443`，新增迁移追加到该链即可。`RunOutsideTx` 可用于 CONCURRENTLY，但本次用默认 in-tx（`SET LOCAL` 生效、失败回滚）。`ensureVectorDimension` 使用 `repository.Repo.DB()`，测试可用 `repository.InitRepository(db)` 注入 testutil DB。

**引用**：backend-go/internal/reader/search/semantic_pg_test.go、backend-go/internal/platform/database/postgres_migrations.go:postgresMigrations、backend-go/internal/platform/testutil/testutil.go:runTestMigrations

<!-- pinned 2026-10-23T11:05:39Z -->
