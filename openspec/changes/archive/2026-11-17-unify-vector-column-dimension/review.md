# review: unify-vector-column-dimension

> 本文件是本 change 的实测记录 + 写入路径清单 + 处置账本（tasks 1.1 / 1.2 / 3.2 的
> 落点，以及实现过程中的取舍与已知后果）。
> 实测环境：容器 `syntopica-postgres`（PG 18.4 + pgvector 0.8.3），**全程在一次性
> 临时库 `uvcd_probe` 上进行，未触碰生产库 `syntopica`**；临时库已在收尾时删除。

## 1. 前置实测结论

### 1.1 `vector(2560)` → 无维度限定 `vector` 的 ALTER 语义（tasks 1.1）

**结论：可行、不重写表、不丢数据、幂等。** 已有实测记录（无需重做）：

- 权威存档：`docs/reference/database/RESTORE.md` §8「附录：`ALTER vector(2560)` →
  `vector` 可行性实测」（2026-11-04 实测，临时库 `dr_probe_tmp`）。
- 五条结论：① ALTER 成功；② 存量数据维度不变（仍 2560）；③ **不重写表**
  （`relfilenode` 不变，走 binary coercion，去 typmod 是放宽约束）；④ 在已是无限定
  `vector` 的列上重放幂等；⑤ 无维度限定列可容纳混合维度（768 与 2560 共存）。
- 与 design D2 的关系：design 的「Risks 第一条（若实测显示会全表重写则停下回报）」
  **未触发**，迁移实现方式维持原案（版本迁移 + 幂等 + lock_timeout 守卫）。

补充确认（本 change 实测，临时库 `uvcd_probe`）：迁移要做的六列里，
`semantic_labels` / `daily_report_sections` / `board_persistent_topics` /
`term_notes` 的 ALTER 与 `topic_tag_embeddings` 同机制（同 base type 去 typmod），
无跨维度 cast 需求。

### 1.2 halfvec 索引在列类型变更后的有效性（tasks 1.2）

实测命令（一次性临时库 `uvcd_probe`，未碰生产库）：

```sql
CREATE TABLE probe_emb (id bigserial PRIMARY KEY, embedding_type text NOT NULL DEFAULT 'identity', embedding vector(2560));
INSERT INTO probe_emb (embedding) SELECT array_fill(0.1::real, ARRAY[2560])::vector FROM generate_series(1, 20);
CREATE INDEX idx_probe_halfvec ON probe_emb USING hnsw ((embedding::halfvec(2560)) halfvec_cosine_ops) WHERE embedding_type = 'identity';
SET lock_timeout='5s';
ALTER TABLE probe_emb ALTER COLUMN embedding TYPE vector;
```

实测输出（改后状态）：

```
 phase | col_type | relfilenode
-------+----------+-------------
 AFTER | vector   |     2303485      ← relfilenode 未变 ⇒ 未重写表
 AFTER_dims | dims | rows
-------------+------+-------
            2560 |    20             ← 存量数据维度不变

 indisvalid | indisready | pg_get_indexdef
------------+------------+-------------------------------------------------------------
 t          | t          | CREATE INDEX idx_probe_halfvec ON public.probe_emb USING hnsw (((embedding)::halfvec(2560)) halfvec_cosine_ops) WHERE (embedding_type = 'identity'::text)
```

**结论：**

| 观察项 | 结论 | 对本 change 的影响 |
| --- | --- | --- |
| ALTER 后索引 `indisvalid` | 仍为 `t`（未被重建、仍有效） | halfvec 索引不需要在迁移里重建 |
| ALTER 后索引定义 | 表达式不变，仍是 `((embedding)::halfvec(2560)) halfvec_cosine_ops` | 索引表达式内嵌维度仍是「运行时维度」的落点 |
| ALTER 后 ANN 查询 | `EXPLAIN` 显示 `Index Scan using idx_probe_halfvec2560b` | 语义召回的 ANN 路径不受影响 |
| ALTER 后写入异维向量 | `ERROR: expected 2560 dimensions, not 768`（identity 行被索引表达式拦住） | halfvec 部分索引事实上承担了「identity 行维度约束」，维度变化时**必须重建索引**，否则新维度写入被拒 |
| ALTER 回 `vector(2560)` | 索引仍 `indisvalid=t` | 双向都安全 |

**两条附带实测（决定了本 change 的实现分支）：**

1. **裸 `vector` 列建不了 float32 HNSW 索引**：`CREATE INDEX ... USING hnsw
   (embedding vector_cosine_ops)` → `ERROR: column does not have dimensions`。
   → 统一成裸 vector 之后，float32 索引只能「继承」旧库已有的，**无法新建**。
   → 运行期纠正器（`ensureVectorDimension` 等）在裸列分支**不再尝试建 float32 索引**，
   并清理遗留的同名索引（它会把列钉在旧维度上）。
2. **float32 索引在列变裸后仍存活且仍强制旧维度**：ALTER 后索引 `indisvalid=t`，
   但写入异维向量被拒（`different vector dimensions 256 and 768`）。
   → 统一迁移在 ALTER **之前**先删掉三张表上的 float32 索引
   （`idx_topic_tag_embeddings_embedding` / `idx_daily_report_sections_embedding` /
   `idx_board_persistent_topics_embedding`），避免留下钉死维度的遗留索引。
   → 现网这三个索引**本来就不存在**（2560 维 > pgvector float32 HNSW 的 2000 维
   硬上限），所以现网无 ANN 损失；语义召回走 halfvec 表达式索引，不受影响。

3. **裸列上采样维度可行**：`SELECT vector_dims(embedding) FROM ... WHERE embedding
   IS NOT NULL LIMIT 1` → 返回实际维度。→ 迁移在列无限定时用数据采样得到维度
   （表空则跳过，不猜维度）。

## 2. 向量写入路径全量清单（tasks 3.2）

判据：spec Requirement「向量写入前做维度一致性校验」的对象是**随值留存的维度字段**
——记录没有该字段时没有可比对目标，该要求对其不适用（列的维度一致性改由「同一次
embed 调用的同一批向量」保证）。

| # | 向量列 | model 声明 | 写入路径（file:line） | 随值留存的维度字段 | 维度校验 |
| --- | --- | --- | --- | --- | --- |
| 1 | `topic_tag_embeddings.embedding` | `tagmanagement/models/tag_models.go:24` | `tagmanagement/service/core/embedding.go:93`、`:126`、`:182`（生成）→ **唯一落库口** `core/embedding.go:564 SaveEmbedding` | `Dimension` ✅ | ✅ **本次新增**（`core/embedding.go:569`，调 `airouter.ValidatePgVectorDimension`） |
| 2 | `related_event_embeddings.embedding` | `tagmanagement/models/related_event_embedding.go:27` | `tagmanagement/service/related/refresh.go:193-201` | `Dimension` ✅ | ✅ 已有（`related/refresh.go:183 validateVector`：空向量 / 维度不符 / 非有限值 / 零范数四道闸） |
| 3 | `candidate_embeddings.embedding` | `discovery/models/discovery_run.go:104` | `discovery/service/candidate_embedding_service.go:421-434`（`writeEmbedding`） | `Dimension` ✅ | ✅ **本次新增**（`:410` 入口校验，维度不符 → 拒写、保留旧向量待下批重试） |
| 4 | `preference_vectors.embedding` | `discovery/models/preference_vector.go:22` | ① `discovery/service/preference_profile_service.go:341`、`:352`（source=behavior 质心）② `:497`（source=seed 加权合并） | `Dimension` ✅ | ① **构造上同源**（dim 取自源标签向量行的 `Dimension`，质心长度 = 源向量长度；源向量本身受 #1 的写入闸保护）② ✅ **本次新增**（`WriteSeed` 入口校验，`:477`） |
| 5 | `route_embeddings.embedding` | `discovery/models/catalog.go:62` | `discovery/service/catalog_extras.go:125-134` | `Dimension` ✅ | ✅ **本次新增**（`:127`，维度不符 → 跳过该路由并记日志，与该函数既有的失败处理一致） |
| 6 | `discovery_interest_entries.embedding` | `discovery/models/discovery_run.go:63` | `discovery/service/discovery_run_service.go:840-849` | `Dimension` ✅ | ✅ **本次新增**（`:836`，维度不符 → 整轮发布失败、不落半成品） |
| 7 | `semantic_labels.embedding` / `merge_embedding` | `models/semantic_label.go:16-17` | `tagmanagement/handler/board_crud_handler.go:244`（版块人工改写）、`tagmanagement/service/auxlabel/auxiliary_label_service.go:341-342`（辅助标签）、`tagmanagement/service/auxlabel/composite_label_service.go:214`（复合标签）、`tagmanagement/service/auxrepair/embedding.go:294`（描述修复链） | ❌ 无 | 不适用（记录不随值留存维度；写入方全部来自单次 embed 调用的返回值） |
| 8 | `daily_report_sections.embedding` | `topicgraph/repository/daily_report_models.go:214` | `topicgraph/service/daily_report_orchestrator.go:325`、`daily_report_rebucket.go:295`、`topicgraph/repository/daily_report_repository.go:1324`、`daily_report_backfill_topics.go:188`、`daily_report_matching.go:351`、`daily_report_assignment.go:308` | ❌ 无 | 不适用（同上） |
| 9 | `board_persistent_topics.embedding` / `centroid` | `daily_report_models.go:98`、`:111` | `topicgraph/repository/daily_report_topic_repository.go:822`、`:810`、`:1009`、`:1011`、`:1277`、`daily_report_manual_topic.go:311`、`:486`、`:597`、`daily_report_rebucket.go:255` | ❌ 无 | 不适用 |
| 10 | `daily_report_threads.embedding` | `daily_report_models.go:287` | `topicgraph/repository/daily_report_topic_repository.go:595` | ❌ 无 | 不适用 |
| 11 | `topic_anchor_overrides.old_embedding` / `new_embedding` | `daily_report_models.go:147-148` | `daily_report_topic_repository.go:1290` | ❌ 无 | 不适用 |
| 12 | `board_topic_watches.embedding_cache` | `daily_report_models.go:641` | `topicgraph/repository/topic_watch_repository.go:458` | ❌ 无 | 不适用 |
| 13 | `board_stories.embedding` / `subject_embedding` | `topicgraph/repository/story_models.go:65-66` | `topicgraph/repository/story_repository.go:281`、`topicgraph/service/story_attribute.go:560`、`:590` | ❌ 无 | 不适用（**注**：事件线建线召回向量链路已停写，见 flow/topic-graph 红线 16） |
| 14 | `term_notes.embedding` | `topicgraph/repository/margin_notes_repository.go:82` | 无（P2 预留，P1 永不写入） | ❌ 无 | 不适用 |
| 15 | `entity_registry.name_embedding` / `name_desc_embedding` | `tagmanagement/models/entity_subject.go:58`、`:67` | `tagmanagement/service/entity/resolver.go:258`、`:267` | ❌ 无 | 不适用 |

**结论**：带维度字段的 6 条路径**全部**过维度一致性校验（#2 原有，#1/#3/#4②/#5/#6 本次
补齐）；不带维度字段的 9 类列不在该 Requirement 射程内，未新增字段（spec 明确
「复用既有的 `Dimension` 字段，不新增字段」）。

## 3. 实现变更清单（按包分组）

### 3.1 `backend-go/internal/platform/database/postgres_migrations.go`

- 新增共享 helper（单一真相源，两条迁移共用，不复制粘贴）：
  - `VectorColumnType(db, table, column)`（导出）：读列的实际类型声明。
  - `sampledVectorDimension(db, table, column)`：裸列从数据采样维度。
  - `topicTagEmbeddingColumnDimension(db)`：列有限定 → 解析；列无限定 → 采样；
    表空 → 0。
  - `TopicTagIdentityHalfvecIndexState` / `TopicTagIdentityHalfvecIndexDimension`：
    读既有 halfvec 索引表达式内嵌维度 + `indisvalid`。
  - `EnsureTopicTagIdentityHalfvecIndex(db, dim)`：索引缺失 / 维度不符 / invalid
    → 先 DROP 再建。
  - `VectorIndexExists(db, indexName)`。
  - `ensureTopicTagIdentityHalfvecIndexForColumn(db, migrationTag)`：维度解析 +
    上限判断 + 建索引（`20261023_0001` 与 `20261117_0002` 共用）。
- 改 `20261023_0001`（**已记录版本，供灾备从零重放**）：维度来源从「只解析列
  format_type」改为「列有限定则解析、无限定则采样数据」，并改用共享 helper 重建
  索引（invalid / 维度不符都先 DROP）。
- 新增 `20261117_0001`：六列统一为无维度限定 `vector`（幂等、逐表
  `withLockTimeout(5s)` 守卫、逐表打点日志、表/列不存在 no-op），并在 ALTER 前删掉
  三张表上的 float32 索引；同迁移清理 `tmp_hnsw_test`（**只删 invalid 的**——同名
  有效索引一律保留，不误删用户索引）。
- 新增 `20261117_0002`：裸 `vector` 列按数据采样维度补建 halfvec 索引；表空则跳过
  并记日志（等运行时首次写入时由 `ensureVectorDimension` 建）。
- `term_notes` 建表语句 `vector(2560)` → `vector`（新库从零建不再写死维度）。

### 3.2 `backend-go/internal/tagmanagement/service/core/embedding.go`

- `ensureVectorDimension` 改三分支（design D3/D4 + 缺口 B）：裸 `vector` → 不 ALTER、
  只按运行时维度校验/重建索引；`vector(dim)` → 列不动、确保索引存在；`vector(其他N)`
  → 保留原 DROP+ALTER+重建（注释标明是迁移过渡期分支）。
- 新增 `ensureTopicTagVectorIndexes` / `ensureTopicTagHalfvecIndex`。
- `SaveEmbedding` 写入前加维度一致性校验（不一致 → 拒绝写入 + 错误日志，含期望
  维度与实际维度）。

### 3.3 `backend-go/internal/tagmanagement/service/auxlabel/auxiliary_label_service.go`

- `ensureSemanticLabelVectorColumnDim` 改三分支（裸列不 ALTER；维度一致不动；其余
  走原 ALTER 分支）。

### 3.4 `backend-go/internal/topicgraph/repository/daily_report_models.go`

- `ensureSectionEmbeddingDimension` / `ensurePersistentTopicEmbeddingDimension` 收敛到
  共享的 `ensureVectorColumnDimensionAndIndex`（三分支同构；裸列分支删遗留 float32
  索引而不新建——裸列建不了 float32 索引）。

### 3.5 `backend-go/internal/platform/airouter/embedding_dimension.go`（新文件）

- `ValidateEmbeddingDimension(vec []float64, dim int)`、
  `PgVectorDimension(pgVector string)`、
  `ValidatePgVectorDimension(pgVector string, dim int)`：写入侧维度校验的共用实现
  （platform 层——三个域都依赖它，不引入跨域依赖）。

### 3.6 `backend-go/internal/discovery/service/*`（写入路径补齐）

- `candidate_embedding_service.go`、`preference_profile_service.go`、
  `catalog_extras.go`、`discovery_run_service.go`：四处写入前加维度一致性校验。

### 3.7 `backend-go/internal/topicgraph/repository/margin_notes_repository.go`

- `type:vector(2560)` → `type:vector`（model 归一；至此全仓 model 声明再无维度
  硬编码）。

### 3.8 测试

- 新增 `backend-go/internal/tagmanagement/service/core/embedding_dimension_test.go`：
  7 个用例覆盖 spec 的 6 个 Scenario（维度一致写入成功 / 不一致拒绝写入 / 维度变化
  后索引重建 / 超上限跳过 / 维度取自模型输出（配置表不得覆盖）/ 配置表不持有维度键
  且列类型不携带维度），外加一条「裸列不得被运行期改回 vector(N)」。
- 扩展 `backend-go/internal/platform/database/vector_dimension_migration_test.go`：
  新增 6 个用例（六列统一 / 行数与 vector_dims 不变 / 重复执行幂等 / invalid
  索引残留被清理且有效索引不被误删 / 裸列时按数据采样建 halfvec 索引 / **裸列
  并存多种维度时跳过建索引**——审查 R-2 的回归用例）。

## 4. 已知后果与遗留项

| # | 事项 | 处置 |
| --- | --- | --- |
| 1 | 裸 `vector` 列建不了 float32 HNSW 索引（pgvector `column does not have dimensions`） | 已记入 design D9：现网这三个 float32 索引本就不存在（2560 > 2000 维硬上限），无实际 ANN 损失；语义召回走 halfvec 表达式索引 |
| 2 | 存活下来的 float32 索引会把列钉在旧维度上（新维度写入被拒） | 统一迁移在 ALTER 前删掉；运行期在裸列分支也清理遗留索引 |
| 3 | 裸列可容纳混合维度向量（768 与 2560 共存） | 写入侧维度校验（§2 的 6 条路径）挡住「声明维度 ≠ 向量维度」；跨行的混合维度仍可能发生（pgvector 在比较查询时才报），属本 change 的既定取舍（design D2 记录的放宽语义） |
| 4 | 维度 > 4000 时 halfvec 索引建不了 | 跳过建索引 + 记日志，语义召回回退精确扫描（spec 明示的行为不中断） |
| 5 | `20261023_0001` 已在生产记录版本，不会再跑 | 其维度解析逻辑已同步改好（灾备从零重放时正确）；现网补建由新增的 `20261117_0002` 承担 |
| 6 | `semantic_labels` 的 `idx_semantic_labels_embedding` / `idx_semantic_labels_merge_embedding` | 全仓无任何地方创建这两个索引（`ensureSemanticLabelVectorColumnDim` 里的 DROP 是历史空动作），本 change 未新增索引也未删除行为 |
| 7 | `testutil.TestEmbeddingDim = 4096` 的注释已过时 | 该常量只是测试用补零长度；黄金 schema 的向量列已是无限定 `vector`，注释待后续单独修正（本 change 未动，避免扩大改动面） |

## 5. 验证留痕

### 5.1 存量库实跑核对（tasks 7.9）

**方式**：`bash scripts/db/restore-key-tables.sh --dump backups/pg-key-20261112-040001.dump --keep`（一次性独立库
`syntopica_restore_20261117_140747`，**生产库 `syntopica` 全程未被写入**；脚本自身的落库硬闸也拒绝对生产库操作）
→ 在恢复库上跑生产的 AutoMigrate + 版本迁移链（本 change 的 `20261117_0001` / `20261117_0002`）→ 核对列类型 / 行数 / 向量维度 / 索引有效性。

**恢复侧**（脚本报告 `/tmp/uvcd_restore_report.md`）：

- count 完整性校验：**通过（70 张表全部一致）**；pg_restore 耗时 4470s。
- 迁移状态对账：恢复库的 `schema_migrations` 已有 91 条；本 change 的两条新迁移在应用启动后**新增记录**（尾部可见 `20261117_0001` / `20261117_0002`）。

**迁移侧**（恢复库上的实跑输出）：

```
INFO Migration 20261117_0001: unifying topic_tag_embeddings.embedding from vector(2560) to dimensionless vector
WARN [SLOW SQL] 310707.948ms > 200ms | rows=0 | ALTER TABLE topic_tag_embeddings ALTER COLUMN embedding TYPE vector
INFO Migration 20261117_0001: unifying daily_report_sections.embedding from vector(2560) to dimensionless vector
INFO Migration 20261117_0001: unifying semantic_labels.embedding from vector(2560) to dimensionless vector
INFO Migration 20261117_0001: unifying semantic_labels.merge_embedding from vector(2560) to dimensionless vector
INFO Migration 20261117_0001: unifying term_notes.embedding from vector(2560) to dimensionless vector
INFO Migration 20261117_0001: unifying board_persistent_topics.embedding from vector(2560) to dimensionless vector
INFO Database initialized successfully
```

**核对结果**：

| 核对项 | 结果 |
| --- | --- |
| 全部 20 个向量列的列类型 | 均为无维度限定的 `vector`（`format_type` = `vector`）——含 `topic_tag_embeddings.embedding` / `daily_report_sections.embedding` / `semantic_labels.embedding`+`merge_embedding` / `term_notes.embedding` / `board_persistent_topics.embedding` 六个目标列 |
| 行数 | `topic_tag_embeddings` 33832、`daily_report_sections` 6533、`semantic_labels` 211853、`term_notes` 17、`board_persistent_topics` 1961（与恢复档基线一致） |
| 向量实际维度 | 全部为 2560（`topic_tag_embeddings` 33832/33832、`daily_report_sections` 6307、`semantic_labels.embedding` 15063、`merge_embedding` 15035、`board_persistent_topics` 1961）——**存量向量值分毫未变** |
| halfvec 索引 | `idx_topic_tag_embeddings_identity_halfvec_hnsw` valid=true（表达式 `((embedding)::halfvec(2560)) halfvec_cosine_ops)`）；同表的 semantic / event_keyword 两个 halfvec 索引同样 valid=true |
| invalid 索引残留 | `tmp_hnsw_test` 已不存在（被 `20261117_0001` 清理） |

**关键耗时发现（部署窗口）**：`topic_tag_embeddings` 的 ALTER 耗时 **310s**，而同一迁移里的其余五列（合计 21 万行）**在同一秒内完成**——差异不在表体重写，而在**该表上的 3 个 halfvec HNSW 索引被 PG 随 ALTER 重建**（33k 行 × 2560 维的 HNSW 构建）。

**决定性测探（表体确实未被重写）**：在恢复库上造一张带 halfvec 索引的同形表（1000 行 2560 维）：

```
 relfilenode_before            relfilenode_after             size_before | size_after
 2467732                      2467732                       32 kB      | 32 kB      ← 未变
```

⇒ **ALTER 不重写表体**（与 `RESTORE.md` §8 的 20 行小表结论一致，这次是在**带索引**的表上补验）。310s 是索引重建成本，不是数据重写成本。

**收尾**：演练结束后已 `dropdb` 一次性恢复库（释放 ~3GB 磁盘）；全程未写生产库。

### 5.2 其余验证

见 tasks.md 各条的「验证」列与 `review-trace` 记录。实测相关的完整输出见本文件 §1.2。

---

## 6. 审查意见建账（只读审查子线程，2026-11-17）

> 审查范围：change 制品（proposal / design / tasks / spec delta / test-cases / review）
> + 实现 diff（postgres_migrations.go、core/embedding.go、airouter/embedding_dimension.go、
> auxlabel/auxiliary_label_service.go、topicgraph/repository/daily_report_models.go、
> discovery 四处写入路径）。审查子线程只读，未改任何仓库文件。
> 总评：实现兑现 spec 的 5 条 Requirement / 10 个 Scenario，**无 High 级问题**；
> 3 处 Medium（其中 1 处是安全相关的锁守卫失效）+ 4 处 Low，均已处置。

### R-1 2026-11-17【Medium】运行期 DDL 的 5s 锁超时守卫失效（SET LOCAL 在事务外是 no-op）

- **状态**：已修复（`auxlabel.ensureSemanticLabelVectorColumnDim` 与 `topicgraph.ensureVectorColumnDimensionAndIndex` 的分支 3 改为「固定连接 + 会话级 `SET lock_timeout` + `defer RESET`」；分支 1/2 不再执行无意义的 `SET LOCAL`）

### R-2 2026-11-17【Medium】裸列按单行 LIMIT 1 采样维度非确定，多维并存时会建出会拦写入的索引

- **状态**：已修复（`sampledVectorDimension` 改为 `GROUP BY dimension` 统计：单一维度 → 用它；**多维并存 → 返回 mixed=true，调用方跳过建索引并记日志**（宁可精确扫描也不建会拦写入的索引）；新增回归用例 `TestVectorDimensionMigrationSkipsIndexOnMixedDimensions`）

### R-3 2026-11-17【Medium】索引存在但表达式内嵌维度取不到时不重建，坏索引永久留存

- **状态**：已修复（`EnsureTopicTagIdentityHalfvecIndex` 的重建判据从「取到维度」改成「索引存在」：`VectorIndexExists` 判存在，存在即比对 `indisvalid` 与内嵌维度，不符或不可解析 → 先 DROP 再建）

### R-4 2026-11-17【Low】invalid 索引残清理不限定 schema，与同文件其它 helper 口径不一致

- **状态**：已修复（`dropInvalidVectorIndexLeftovers` 加 `pg_namespace` 限定 `public`，DROP 改为 `public.tmp_hnsw_test`）

### R-5 2026-11-17【Low】float32 HNSW 索引 DDL 硬编码列名 embedding，忽略入参 column

- **状态**：已修复（`ensureFloat32HNSWIndex` 增列名入参，DDL 用参数化的列名）

### R-6 2026-11-17【Low】SaveEmbedding 里两个完全相同的条件块相邻

- **状态**：已修复（合并为一个 `if` 块：先校验维度、再 ensure 索引）

### R-7 2026-11-17【Low】review.md §2 的 file:line 漂移

- **状态**：已修复（§2 表格的 6 条带维度字段的路径行号已按当前代码刷新）

### R-8 2026-11-17【Low】mixed 维度日志把「检索不可用」说成「只是回退精确扫描」

- **状态**：已修复（`postgres_migrations.go` mixed 分支：日志级别 `Infof` → `Warnf`，文案改为 `cross-dimension comparisons will error until the minority-dimension rows are cleaned`；补注释说明「能力受限 ≠ 单纯变慢」）。无测试断言该文案（已 grep 确认）
- **来源**：第二轮只读审查（`change-review` 档，`zai-coding-cn/glm-5.3:max`），复查 7 项核心问题全过后的 2 处 Low 之一
- **问题**：`ensureTopicTagIdentityHalfvecIndexForColumn` 的 mixed 分支日志写的是
  `semantic search falls back to exact scan`，读起来像「只是慢一点」。但混合维度下
  精确扫描同样要跨行比较向量，pgvector 会直接报 `different vector dimensions N and M`
  ——**检索在清理少数派维度之前是不可用的**，不是降级变慢
- **同轮第二处 Low（`margin_notes_repository.go:70` 的「P2 预留」注释）**：不改。
  该注释描述的是事实（`TermNote.Embedding` P1 永不写入、保留给 P2 相似词归并），
  与「维度是运行时属性、不属于 schema 契约」的表述并存且不矛盾；删掉注释反而丢信息。
  同轮另把 `testutil.TestEmbeddingDim` 的注释从「production schema enforces vector(4096)」
  修正为「仅供 fixture 尺寸，不是 schema 契约」——原注释在本 change 后已失实
