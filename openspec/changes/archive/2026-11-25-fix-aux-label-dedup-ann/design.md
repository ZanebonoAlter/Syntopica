## Context

见 proposal.md（Why）。实现侧现状（2026-11-24 对照代码与真库核对）：

- `auxiliary_label_service.go` 的 `auxLabelCache` 同时缓存两类数据：`activeLabels`（元数据，5 分钟 TTL）与 `mergeEmbeddings`（`map[uint][]float64`，10 分钟 TTL）。
- `sqlMergeMatcher(ctx, db, labels, mergePgVector, mergeVector)` 收到的是「已加载的 active 列表 + 新标签的向量」，函数内只做缓存查询与全量余弦；`mergePgVector`（batch embedder 已产出的 PG 文本形式）当前未被使用（参数名 `_`）。裁决口径：`sim >= threshold` 命中，`RefCount` 降序、同值取 `ID` 小者。
- `packageAuxLabelCacheGetOrLoad` 每次重载用 GORM `Find` 一次性取回全部 `id, merge_embedding` **文本**再逐行 `ParsePgVector`（实测 26,767 行、9.6 秒）；重载瞬间「向量文本 + 旧 map + 新 map」同时在堆上，是 RSS 峰值的来源。
- L1（`planAuxiliaryLabels`）对 active 列表线性扫描，每行重算 `NormalizeLabelKey` / `Slugify`，首个命中即 break；alias 命中有两种口径（trim + `EqualFold`，或 `Slugify(alias) == Slugify(label)`）。
- `planAuxiliaryLabels` 整体在事务外（既有约束「事务内不调 embedder」，由 `TestAttachTwoPhaseNoEmbedderInsideTransaction` 钉住）。
- `semantic_labels` 两个向量列均为裸 `vector`（无维度限定）；真库 active 辅助标签 26,767 行，`embedding` / `merge_embedding` 无 NULL，**维度全部为 2560**（无混合维度）；现有索引只有主键与 `label_type` / `slug`(UNIQUE) / `status` 三个 btree。pgvector 0.8.3、PostgreSQL 18.4，`maintenance_work_mem` 默认 64MB、`max_parallel_maintenance_workers` 默认 2。
- `topic_tag_embeddings` 已有 halfvec 表达式 HNSW 索引，且**查询侧已有两段式 ANN 范式**：`service/core/embedding.go:findSimilarByVector`（speed-up-tag-embedding D1）= halfvec HNSW 取候选池 → 池内 `col <=> $1::vector` 精确 float32 重排，`SET LOCAL hnsw.ef_search`（不小于候选池）放在只读事务内。索引侧范式在 `core/embedding.go:ensureTopicTagHalfvecIndex` 与 `database/postgres_migrations.go`（`TopicTagIdentityHalfvecIndexDDL` / `EnsureTopicTagIdentityHalfvecIndex` / `halfvecExpressionDimension` / `ensureTopicTagIdentityHalfvecIndexForColumn`）；构建期需 `maintenance_work_mem='256MB'` + `max_parallel_maintenance_workers=0`（Docker 默认 64MB `/dev/shm` 会让并行建 HNSW 报 53100）。
- 最新 migration 版本为 `20261117_0002`；本 change 的迁移号最终定为 `20261124_0002`（实现期间 `20261124_0001` 被同树另一 change 的 `topicLaneConstraintMigration` 占用——迁移按 version 排序、`schema_migrations` 以 version 为主键，同号会让其中一条被静默当成已执行跳过）。
- 启动顺序：migrations → `cmd/server/main.go` 的 `EnsureVectorDimensionOnce`（一次真实 embedder 调用探测维度 → `EnsureSemanticLabelVectorDimension` / `EnsureSemanticLabelMergeVectorDimension` → `vectorDimEnsurers` 回调）。
- 去重阈值来自 `loadAuxLabelDedupeSim(db)`（`ai_settings` 的 `auxiliary_label_dedupe_sim` 行，缺省 0.95）。
- `auxiliary-label` spec 的既有不变量：L2 命中只 addAlias、不改 Label、不重算/不触碰向量列；alias 变更不得触碰向量列。

## Goals / Non-Goals

**Goals:**

- L2 去重的数据库扫描量与 active 辅助标签总数无关（走索引）。
- 进程不再持有全量 merge_embedding 副本（省 ~547MB 常驻）。
- 阈值判定口径不变：裁决基于候选在原 `vector` 列上的精确距离（float32 重排），不引入 halfvec 近似误差。
- 归并结果与旧实现逐条一致（同一批标签归并到同一目标 ID），差异以真实数据对账暴露并逐条归因。
- 维度变化时索引在旧维度行清理后自动重建，不留旧维度表达式索引；列维度过渡不被旧索引卡死。

**Non-Goals:**

- 不改去重阈值、RefCount 优先级、L1/L3 判定口径。
- 不改 `semantic_labels` 表结构、不改 `embedding`（storage）列的任何读写路径。
- 不引入外部向量库。
- 不改 `composite_label_service` 的相似度路径（它只查少量行，不在本次瓶颈内）。

## Decisions

### D1. 索引形态：halfvec 表达式部分 HNSW（不建 float32 索引、不加列维度）

- **结论**：新建 `idx_semantic_labels_merge_halfvec_hnsw`：`USING hnsw ((merge_embedding::halfvec(<dim>)) halfvec_cosine_ops) WHERE label_type='auxiliary' AND status='active' AND merge_embedding IS NOT NULL`。
- **落点**：backend-go/internal/platform/database/postgres_migrations.go
- **取舍**：float32 HNSW 被否——2560 维超过 pgvector float32 上限 2000，建不出来；给列加 `vector(2560)` 维度限定被否——项目已确立「列不承载运行时维度」的口径（`unify-vector-column-dimension`），且换模型会锁死列。

### D2. 维度守卫：启动期按运行时维度校验并重建；迁移内维度来源；与列维度过渡联动

- **结论**：新增 `ensureSemanticLabelMergeHalfvecIndex(db, dim)`，复用 `ensureTopicTagHalfvecIndex` 的「读取索引表达式内嵌维度（`halfvecExpressionDimension`）→ 不符或 invalid 则 DROP 重建」逻辑，构建期在固定连接上 `SET maintenance_work_mem='256MB'` / `max_parallel_maintenance_workers=0` 并以 `RESET` 兜底；挂载点是 `EnsureVectorDimensionOnce` 的维度探测之后（dim 已知才校验）。失败只 WARN 不阻断启动（查询退化为顺序扫描，结果仍正确）。
- **迁移内建索引的维度来源**：`20261124_0002` 跑在维度探测之前，拿不到运行时维度——沿用 `ensureTopicTagIdentityHalfvecIndexForColumn` 的「列无限定 → 存量数据采样（表空则跳过）」口径；维度不符由本守卫在启动期重建。
- **换模型窗口的诚实口径**（R-2）：`semantic_labels` 两列是裸 `vector`、无写入口维度校验，换模型后新旧维度行可共存。此时 `merge_embedding::halfvec(<newDim>)` 遇旧维度行会让**整条召回 SQL 报错 → 该 tag 的全部辅助标签（含本应 L1 命中的）一个都挂不上**，且守卫建新维度索引也必然失败。因此守卫在重建前 MUST 用 `semanticLabelMergeColumnDimension` 比对存量维度与运行时 dim：不一致或 mixed 时打 **ERROR**（不是 WARN）并给出可执行清理 SQL（`UPDATE … SET merge_embedding = NULL WHERE … vector_dims(merge_embedding) <> <dim>`，随后走既有 backfill 重建），之后照旧尝试建索引。**「退化为顺序扫描、结果仍正确」只在维度一致时成立**。
- **与列维度过渡的联动**：`ensureSemanticLabelVectorColumnDim` 的 Branch 3（`ALTER COLUMN … TYPE vector(N)`）执行前 MUST 先 `DROP INDEX IF EXISTS idx_semantic_labels_merge_halfvec_hnsw`——表达式索引依赖列，带着它 ALTER 会失败（先例 `core/embedding.go:ensureVectorDimension`「Drop indexes first」）；同步改掉该函数「semantic_labels has no vector index」的过期注释。
- **落点**：backend-go/internal/platform/database/postgres_migrations.go、backend-go/internal/tagmanagement/service/auxlabel/auxiliary_label_service.go:ensureSemanticLabelVectorColumnDim、backend-go/internal/tagmanagement/service/auxlabel/auxiliary_label_service.go:EnsureVectorDimensionOnce
- **取舍**：只在 migration 里建一次被否——维度是运行时属性，换 embedding 模型后旧表达式索引会静默失效（查询退化回顺序扫描且不报错）；每次启动无条件重建也被否——26,767 行建索引是分钟级开销。

### D3. 候选检索：halfvec HNSW 召回 + float32 精确重排 + Go 侧既有口径裁决

- **结论**：`sqlMergeMatcher` 分两段，对齐 `findSimilarByVector` 既有范式：
  1. **召回**：`WHERE label_type='auxiliary' AND status='active' AND merge_embedding IS NOT NULL ORDER BY merge_embedding::halfvec(<dim>) <=> $1::halfvec(<dim>) LIMIT 32`。WHERE 三谓词 MUST 与部分索引谓词逐字一致（否则规划器不用部分索引）；typmod MUST 带维度（裸 `::halfvec` 不走索引）。
  2. **精排**：对召回候选用原列精确距离 `merge_embedding <=> $1::vector` 重算（≤32 行，无需索引），得到与旧实现同量纲的 float32 余弦距离；携带 id / label / slug / aliases / ref_count 等裁决所需列。

  Go 侧按 `1 - dist >= threshold`（`loadAuxLabelDedupeSim`）过滤，`RefCount` 降序 / `ID` 升序选唯一 best。`mergeMatcherFunc` 签名只移除 `labels`，**保留 `mergePgVector`** 作为 `$1` 绑定参数（batch embedder 已产出，免去 2560 维 float64 重新序列化）。
- **落点**：backend-go/internal/tagmanagement/service/auxlabel/auxiliary_label_service.go:sqlMergeMatcher
- **取舍**：直接拿 halfvec 距离裁决被否——fp16 存储误差（分量级 ~5e-4、cosine 级 ~1e-3）会让 0.95 边界附近的标签判定翻转，项目既有范式正是为「阈值口径不变」才加的精排；把阈值与 RefCount 排序完全下推到 SQL 被否——带阈值的 ANN 过滤在候选不足时会退化为顺序扫描，且无法复用 HNSW 的距离排序；`WHERE 1-dist >= threshold ORDER BY ref_count DESC LIMIT 1` 被否——同上，且丢失「阈值内取 RefCount 最大者」的对账口径。

### D4. 删除 merge embedding 向量缓存

- **结论**：从 `auxLabelCache` 移除 `mergeEmbeddings` / `mergeEmbeddingsAt`，删除 `auxMergeEmbeddingsTTL` 与 `packageAuxLabelCacheGetOrLoad`；`InvalidateAuxLabelCache` 只清 `activeLabels` + `activeLabelsAt`；`applyCreate` / `applyDisable` / `applyMerge` / `invalidateOnAliasChange` / `lightweightLabelCopy` 中关于 merge map 的注释同步删除；`aux_label_cache_test.go` 中针对 merge 缓存的测试同步删改（`TestAuxLabelCacheAliasChangePreservesMergeEmbeddings` 删除，其余 5 个去掉 merge 断言）——否则测试包编译失败。
- **落点**：backend-go/internal/tagmanagement/service/auxlabel/auxiliary_label_service.go:auxLabelCache、backend-go/internal/tagmanagement/service/auxlabel/aux_label_cache_test.go
- **取舍**：保留缓存但改为只缓存少量热标签被否——需要额外的失效与一致性逻辑，且召回查询已是毫秒级，缓存收益可忽略。

### D5. L1 精确匹配改 map 索引

- **结论**：`loadActiveAuxiliaryLabels` 返回的轻量列表额外构建三个 map（与列表同生命周期、同 TTL），L1 从线性扫描改为 map 查找：
  - `bySlug`：`Slugify(row.Label)`（即 `row.Slug`）**以及**每个别名的 `Slugify(alias)` → 行（覆盖 `semanticAliasesContain` 的 slug 相等口径）；
  - `byAliasFold`：`strings.ToLower(strings.TrimSpace(alias))` → 行（覆盖 trim + `EqualFold` 口径）；
  - `byNormKey`：`NormalizeLabelKey(row.Label)` → 行。

  查找顺序 slug → aliasFold → normKey。map 构建**先到先得**（同 key 多行保留列表中首个，对齐顺序扫描的 break 语义），MUST NOT 后者覆盖。
- **落点**：backend-go/internal/tagmanagement/service/auxlabel/auxiliary_label_service.go:planAuxiliaryLabels
- **取舍**：保持线性扫描被否——每行重算 `NormalizeLabelKey` / `Slugify`，26,767 行 × 每批标签数，是与 L2 余弦同量级的开销（并非「顺手」）；把 map 建在 DB 侧（GIN 索引）被否——会把匹配口径拆到两处。旧实现「首个命中」取决于无 ORDER BY 的 `Find` 物理顺序，本身不严格确定，对账时不视为回归。

### D6. 召回质量：`hnsw.ef_search` 与截断兜底

- **结论**：
  - L2 查询包在**每批一个只读事务**里（embedder 调用仍在事务外，不违反「事务内不调 embedder」既有约束），事务内 `SET LOCAL hnsw.ef_search = 100` 后逐标签查询——pgvector 默认 `ef_search=40`，小于 LIMIT 时返回行数被截断，召回率也靠它。
  - 候选 `LIMIT` 取 32（阈值 0.95 下实际命中通常 0~3）；若精排后 32 条**全部** ≥ 阈值，则同事务内 `SET LOCAL hnsw.ef_search = 200` 后以 `LIMIT 128` 重查一次（保证「候选被截断」时不会漏掉 RefCount 更大的目标）。
  - 召回是**近似**的，等价边界与对账口径见 D8。
- **落点**：backend-go/internal/tagmanagement/service/auxlabel/auxiliary_label_service.go:sqlMergeMatcher
- **取舍**：不设 `ef_search` 被否——LIMIT 128 兜底在默认值下最多返回约 40 行，形同虚设；固定 `LIMIT 32` 不再兜底被否——存在「≥32 条同义标签」的理论漏选面；直接 `LIMIT 128` 常驻被否——候选越多精排越慢，而绝大多数请求前几条就结束。

### D7. migration 编号与部署形态

- **结论**：新增 `20261124_0002`（按存量采样维度建索引，`CREATE INDEX IF NOT EXISTS`，幂等）；`IF NOT EXISTS` 命中旧维度索引时由 D2 的守卫在启动期重建。不加数据回填、不加表结构变更。
- **落点**：backend-go/internal/platform/database/postgres_migrations.go
- **取舍**：用 `CONCURRENTLY` 建索引被否——migration 在事务内执行，`CONCURRENTLY` 不允许；改用会话级 GUC + 普通 `CREATE INDEX`，在建索引期间接受一次 26,767 行的短暂 SHARE 锁（单用户应用、启动期执行）。

### D8. 语义等价边界与对账

- **结论**：新旧实现的等价口径分三层，验收用一次性对账程序在真实库上验证：
  1. **阈值判定**：精排用 float32 原列距离，与旧实现（float32 存量 → float64 运算）差异 ~1e-7，视为等价；
  2. **召回**：HNSW 近似召回可能漏掉真近邻——漏掉的后果是多建一个重复标签（L3 而非 L2）；以 `ef_search=100` + 真实样本对账 `mismatch=0` 为验收线，出现差异逐条记录（label / 旧目标 / 新目标 / 相似度 / 归因）；
  3. **可见性（良性差异，明确接受）**：旧实现新建标签的 merge 向量要等 10 分钟 reload 才对 L2 可见，新实现提交后立即可见——更不容易在同一时间窗产生重复标签。

  对账样本取库中现有标签的 `merge_embedding`（省 API 调用，可复现），额外统计「阈值内有 ≥2 个邻居」的样本数——这才是 RefCount 裁决真正起作用的样本。程序打印当次召回走的是索引还是顺序扫描（`EXPLAIN`），部署前（索引未建）只验证阈值与裁决层，部署后再跑一遍验证召回层。
- **落点**：tasks.md 4.1（一次性对账程序 `backend-go/cmd/aux-label-dedup-parity/`）
- **取舍**：用新打标的真实 LLM 输出做样本被否——成本高且不可复现。

## Risks / Trade-offs

- [HNSW 近似召回漏掉真近邻] → 后果是多建重复标签；`ef_search=100` 留 3 倍余量，0.95 这类近重复邻居是 HNSW 最易命中的；部署后对账 `mismatch=0` 为验收线，出现差异逐条归因，不默许「精度换速度」。
- [建索引期间 CPU 与锁] → 26,767 行、仅首次部署启动执行一次，预计 1~2 分钟；关闭并行维护避免 `/dev/shm` 报错；失败时降级为「跳过索引 + SQL 顺序扫描」的兜底路径（**维度一致时**结果仍正确、单标签百毫秒级）。
- [维度切换窗口]（R-2）→ 裸 `vector` 列允许新旧维度行共存；未清理旧维度行时召回 SQL 报错（不是变慢）且守卫建索引失败。守卫打 ERROR 并给清理 SQL；Migration Plan 把「换模型先清旧向量」列为前置步骤。
- [维度守卫失败导致静默退化] → **仅在存量维度一致时**查询退化为顺序扫描（结果仍正确）；维度不一致时守卫显式报错，不再伪装成「退化」。启动日志记录索引状态便于排查。
- [HNSW 增量写入开销] → 每批打标签只新增个位数标签，毫秒级；无批量重建场景。
- [归并结果与旧实现不一致] → 验收含对账；不一致即视为缺陷，逐条归因。
- [并发会话共享工作树] → `postgres_migrations.go` 等文件带有其他 change 的未提交改动，本 change 只做增量编辑、不 commit 他人的脏文件。

## Migration Plan

1. 上线：migration 建索引 → 启动守卫校验维度 → matcher 走新路径。首次启动多等一次建索引时间。
2. **换 embedding 模型时（先做）**：清理旧维度向量（`UPDATE semantic_labels SET embedding = NULL, merge_embedding = NULL WHERE label_type='auxiliary' AND status='active' AND vector_dims(merge_embedding) <> <new_dim>`）→ 重启（守卫重建索引、列维度过渡先删旧索引）→ 走既有 backfill 补向量。**不先清就重启**：守卫会打 ERROR 且召回整批报错（R-2）。
3. 回滚：还原代码 + `DROP INDEX idx_semantic_labels_merge_halfvec_hnsw`，进程内缓存会自然恢复旧行为（内存回到 547MB），数据无变化。
4. 灰度：单用户应用，无灰度；部署前先在本地用真实库跑对账脚本（阈值/裁决层），重启后再跑一遍（召回层）。

## Open Questions

无。
