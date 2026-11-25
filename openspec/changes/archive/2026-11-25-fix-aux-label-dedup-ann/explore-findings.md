
## aux L2 去重 ANN 化：既有两段式范式/ef_search/测试残留/守卫联动事实

对 change fix-aux-label-dedup-ann 的 design 核对结论（2026-11-24，design 阶段，尚无代码落地）。

## 现状代码（backend-go/internal/tagmanagement/service/auxlabel/auxiliary_label_service.go，1307 行）
- `auxLabelCache{activeLabels(5min TTL), mergeEmbeddings map[uint][]float64(10min TTL)}`（L83）；`mergeMatcherFunc(ctx, db, labels, mergePgVector, mergeVector)`（L76，mergePgVector 当前未用）。
- L1（planAuxiliaryLabels ~L262）：对 active 列表线性扫，逐行 `Slug==slug || semanticAliasesContain(aliases,label) || NormalizeLabelKey(row.Label)==normKey`，首个命中 break（列表是 Find 无 ORDER BY 的顺序）。`semanticAliasesContain` 两种口径：trim+EqualFold，或 `Slugify(alias)==Slugify(label)`。每行都重算 NormalizeLabelKey/Slugify → L1 本身也是每标签百毫秒级。
- L2（sqlMergeMatcher L1001）：`sim < threshold` 跳过（即 >= 命中），RefCount 降序、ID 升序选 best；best 是轻量行副本（无向量）。`loadAuxLabelDedupeSim` 读 ai_settings `auxiliary_label_dedupe_sim`，缺省 0.95。
- `packageAuxLabelCacheGetOrLoad`（L1029）：GORM Find 一次性拉全部 `id, merge_embedding` 文本再 ParsePgVector → reload 峰值=文本+旧 map+新 map。`applyCreate/applyDisable/applyMerge` 只增量维护 activeLabels，不动 map（新标签向量要等 10min reload 才对 L2 可见）。
- plan 阶段整体在事务外（测试 TestAttachTwoPhaseNoEmbedderInsideTransaction 约束「事务内不调 embedder」）。

## 真库事实（只读查询，2026-11-24 23:30）
pgvector 0.8.3 / PG 18.4；active aux=26,767，embedding/merge_embedding 无 NULL，**维度全为 2560（无混合维度）**；semantic_labels 索引仅 pkey/label_type/slug(UNIQUE)/status；两向量列均为裸 `vector`；`maintenance_work_mem=64MB`、`max_parallel_maintenance_workers=2`。migration 最新 20261117_0002，20261124_0001 未被占用（工作区未提交改动也没碰）。slow 日志 `dedup gate slow` 2099 条且在涨。当前后端稳态 RSS 814MB。

## 项目既有 ANN 范式（design D3 偏离了它）
`service/core/embedding.go:findSimilarByVector`（L255，speed-up-tag-embedding D1）是两段式：① `ORDER BY col::halfvec(dim) <=> $1::halfvec(dim) LIMIT pool`（typmod 必须带维度，裸 ::halfvec 不走索引）；② 池内用 `col <=> $1::vector` 精确 float32 重排——「identical ranking semantics to the old full-table scan，0.92/0.95/0.8 阈值不变」。`SET LOCAL hnsw.ef_search = 120`（pool 50，必须 ≥ LIMIT，否则默认 40 截断）放在只读事务里（L302）。新 matcher 应照抄两段式：halfvec 召回 → float32 精排 → Go 侧阈值+RefCount/ID 裁决；LIMIT 32 配 ef_search≥100，D6 的 LIMIT 128 兜底必须 ef_search≥128 才真拿得到行。

## 索引守卫复用点与联动坑
- 可复用：`postgres_migrations.go` `EnsureTopicTagIdentityHalfvecIndex`(L~4523，exists→读内嵌维度/invalid→DROP→建)、`halfvecExpressionDimension`(L4449，解析 `::halfvec(N)`)、`VectorIndexExists`；运行期 GUC 版 `core/embedding.go:ensureTopicTagHalfvecIndex`(L716，Connection+会话级 SET/RESET maintenance_work_mem=256MB、max_parallel_maintenance_workers=0)。迁移内维度来源先例：`ensureTopicTagIdentityHalfvecIndexForColumn`「列无限定→存量采样」。
- 坑：`ensureSemanticLabelVectorColumnDim`(auxlabel L1243) 注释写死「semantic_labels has no vector index」，Branch 3 只 `DROP INDEX IF EXISTS idx_semantic_labels_<col>`。新表达式索引 `(merge_embedding::halfvec(2560))` 存在时 Branch 3 的 ALTER COLUMN TYPE 会因依赖索引重建失败——需先 DROP 新索引再 ALTER（先例 embedding.go L647「Drop indexes first」），并改过期注释。启动顺序：migrations → main.go:97 `EnsureVectorDimensionOnce`（先两个列守卫，再 vectorDimEnsurers 回调，dim 来自一次真实 embedder 调用，失败则 ensurer 不跑）。

## 测试残留（D4 会让测试包编不过）
`auxlabel/aux_label_cache_test.go` 27 处引用 merge 缓存，涉及 TestAuxLabelCacheCreateIsIncremental / CreateAppendsLightweightCopy / DisableIsIncremental / MergeAliasIsIncremental / AliasChangePreservesMergeEmbeddings / TTLExpiryReloads。现有 L1 测试名是 `TestAuxiliaryLabelServiceL1*`（`-run TestAuxiliaryLabelL1` 匹配不到任何测试）。L2 测试 `TestAuxiliaryLabelServiceL2MergeKeepsHigherRefCountAndAppendsAlias`(auxiliary_label_service_test.go L253) 用 testutil.TestEmbeddingDim 小维度+真库，无索引时新 SQL 走顺序扫描仍正确。

**引用**：backend-go/internal/tagmanagement/service/auxlabel/auxiliary_label_service.go:sqlMergeMatcher、backend-go/internal/tagmanagement/service/auxlabel/auxiliary_label_service.go:packageAuxLabelCacheGetOrLoad、backend-go/internal/tagmanagement/service/auxlabel/auxiliary_label_service.go:planAuxiliaryLabels、backend-go/internal/tagmanagement/service/auxlabel/auxiliary_label_service.go:ensureSemanticLabelVectorColumnDim、backend-go/internal/tagmanagement/service/core/embedding.go:findSimilarByVector、backend-go/internal/tagmanagement/service/core/embedding.go:ensureTopicTagHalfvecIndex、backend-go/internal/platform/database/postgres_migrations.go:EnsureTopicTagIdentityHalfvecIndex、backend-go/internal/platform/database/postgres_migrations.go:halfvecExpressionDimension、backend-go/internal/tagmanagement/service/auxlabel/aux_label_cache_test.go

<!-- pinned 2026-11-24T15:36:24Z -->
