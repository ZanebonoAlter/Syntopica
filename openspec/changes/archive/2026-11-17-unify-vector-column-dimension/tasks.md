## 0. 前置补漏（tasks 原始清单未列，但实现必需）

- [x] 0.1 **索引维度源改造（缺口 A）**：`20261023_0001` 在列是裸 `vector` 时必须仍能建索引——列有限定 → 沿用 `vectorColumnDimension` 解析；列无限定 → 从数据采样（`SELECT vector_dims(embedding) FROM topic_tag_embeddings WHERE embedding IS NOT NULL LIMIT 1`）；表空 → 跳过并记日志，等运行时首次写入时由 `ensureVectorDimension` 建。**新增一条迁移版本承载「裸 vector 时补建 halfvec 索引」**（`20261117_0002`），两条迁移共用同一个 helper（`ensureTopicTagIdentityHalfvecIndexForColumn`），不复制粘贴；INVALID 索引先 DROP 再建、`SET LOCAL maintenance_work_mem='256MB'`、`max_parallel_maintenance_workers=0` 逻辑不变。落点：`backend-go/internal/platform/database/postgres_migrations.go`，验证：`cd backend-go && go test ./internal/platform/database/... -run VectorDimension` 退出码 0
- [x] 0.2 **4 个运行期纠正器三分支改造（缺口 B）**：`ensureVectorDimension`（core）+ `ensureSemanticLabelVectorColumnDim`（auxlabel，embedding / merge_embedding 两列）+ `ensureSectionEmbeddingDimension` + `ensurePersistentTopicEmbeddingDimension`（topicgraph）一律改三分支——① 列类型 == 裸 `vector`：**不执行 ALTER**，按运行时 dim 校验/重建索引（索引表达式内嵌维度不符才重建）；② 列类型 == `vector(dim)`：确保索引存在；③ 列类型 == `vector(其他N)`：保留现有 DROP INDEX + ALTER + 重建（兼容尚未跑新迁移的旧库，代码注释标明是迁移过渡期分支）。验证：`grep -n "ALTER TABLE topic_tag_embeddings ALTER COLUMN embedding TYPE" backend-go/internal/tagmanagement/service/core/embedding.go` 仅出现在分支③；`cd backend-go && go test ./internal/tagmanagement/service/core/... ./internal/tagmanagement/service/auxlabel/... ./internal/topicgraph/...` 退出码 0

## 1. 前置：确认 ALTER 语义与耗时（阻断项）

- [x] 1.1 复核 `fix-disaster-recovery-channel` 任务 2.2 的实测结论：`vector(2560)` → 无维度限定 `vector` 的 ALTER 是否成功、是否重写数据、耗时多少。**若实测显示会全表重写**，停下并回报（design Risks 第一条），本 change 的迁移实现方式需重估后再继续 —— 结论：`docs/reference/database/RESTORE.md` §8 已有实测（2026-11-04，临时库 `dr_probe_tmp`）：**ALTER 成功、不重写表（relfilenode 不变）、存量维度不变、重放幂等**；design 的退路**未触发**，迁移实现方式维持原案。复核记录见 `review.md` §1.1
- [x] 1.2 实测 halfvec 索引在列类型变更后的有效性：在临时库中造一张含向量的表 + 建 halfvec HNSW 索引 → 执行列类型变更 → 确认索引重建后 `indisvalid=true` 且 `pg_get_indexdef` 表达式仍为 `((embedding)::halfvec(N)) halfvec_cosine_ops` —— 实测（一次性临时库 `uvcd_probe`，未碰生产库）：ALTER 后 `indisvalid/indisready` 仍为 `t/t`、索引定义不变、EXPLAIN 仍走 `Index Scan`；**附带实测两条关键事实**：裸 `vector` 列建不了 float32 HNSW 索引（`column does not have dimensions`）、存活下来的 float32 索引会把列钉在旧维度上。实测命令与输出记入 `review.md` §1.2

## 2. 迁移：存量列统一 + 无效索引清理

- [x] 2.1 `backend-go/internal/platform/database/postgres_migrations.go`：新增一条版本迁移，把带维度约束的向量列改为**无维度限定**的 `vector`——覆盖 `topic_tag_embeddings.embedding`、`daily_report_sections.embedding`、`semantic_labels.embedding`、`semantic_labels.merge_embedding`、`term_notes.embedding`、`board_persistent_topics.embedding`；已是无限定的列直接跳过（幂等）；每张表用 `withLockTimeout(db, "5s", ...)` 守卫并逐表打点日志，验证：`grep -n "ALTER COLUMN embedding TYPE vector" backend-go/internal/platform/database/postgres_migrations.go` 命中新迁移且目标类型无维度限定 —— 落点：迁移 `20261117_0001`
- [x] 2.2 同一条迁移内：`DROP INDEX IF EXISTS tmp_hnsw_test`（已确认的 invalid 索引残留），验证：迁移在无该索引的库上执行不报错 —— 实现为「**只删 invalid 的**」（catalog 判定 `indisvalid=false` 才删；同名**有效**索引一律保留，不误删用户索引），验证：`TestVectorDimensionMigrationDropsInvalidIndexLeftover`
- [x] 2.3 迁移幂等与保值单测（扩展 `fix-disaster-recovery-channel` 建立的 `backend-go/internal/platform/database/vector_dimension_migration_test.go`）：① 迁移执行后各列类型为无维度限定；② 行数不变；③ 重复执行成功且列类型不变；④ 非空向量的 `vector_dims` 不变，验证：`cd backend-go && go test ./internal/platform/database/... -run VectorDimension` 退出码 0
- [x] 2.4 **`term_notes` 建表语句去维度限定（补漏）**：`postgres_migrations.go` 的 `CREATE TABLE IF NOT EXISTS term_notes` 里 `embedding vector(2560) DEFAULT NULL` → `embedding vector DEFAULT NULL`（新库从零建时不再写死维度），验证：`grep -rn "vector(2560)" backend-go/internal/platform/database/postgres_migrations.go` 仅剩注释/历史说明

## 3. 运行时：纠正器改造与写入校验

- [x] 3.1 `backend-go/internal/tagmanagement/service/core/embedding.go`：`ensureVectorDimension` 增加分支——列已是无维度限定 `vector` 时**不执行** `ALTER COLUMN ... TYPE`，改为按当前运行时维度校验向量索引（表达式内嵌维度不符则重建，复用既有 `TopicTagIdentityHalfvecIndexDDL(dim)` 与 `maintenance_work_mem` 设置/复位逻辑），验证：`grep -n "ALTER TABLE topic_tag_embeddings ALTER COLUMN embedding TYPE" backend-go/internal/tagmanagement/service/core/embedding.go` 仅出现在「列有限定」分支内
- [x] 3.2 排查并列出**全部向量写入路径**（不止 `SaveEmbedding`），确认每条路径都经过维度一致性校验点；补齐遗漏路径的校验，验证：产出写入路径清单（文件:行）并记入 `review.md` —— 清单见 `review.md` §2（15 行，含 6 条带维度字段的路径逐行判定）；本次补齐 4 条（candidate_embeddings / preference_vectors-seed / route_embeddings / discovery_interest_entries），另 1 条（topic_tag_embeddings）补齐，related_event_embeddings 原有
- [x] 3.3 维度一致性校验单测：维度一致 → 写入成功；维度不一致 → 拒绝写入且记错误日志（含期望维度与实际维度），验证：`cd backend-go && go test ./internal/tagmanagement/service/core/...` 退出码 0 —— 落点：`backend-go/internal/tagmanagement/service/core/embedding_dimension_test.go`
- [x] 3.4 **写入侧校验的共用实现 + 覆盖面判定（补漏）**：共用校验实现放 `backend-go/internal/platform/airouter/embedding_dimension.go`（`ValidateEmbeddingDimension` / `PgVectorDimension` / `ValidatePgVectorDimension`；platform 层，三域共用不引入跨域依赖）；**不新增**维度字段到没有它的表（design D12）——带维度字段的 6 条路径全部过校验，其余 9 类列判定为「不适用」，逐行判定见 `review.md` §2

## 4. model 归一

- [x] 4.1 `backend-go/internal/topicgraph/repository/margin_notes_repository.go:82`：`type:vector(2560)` → `type:vector`，与其它 13 处声明一致，验证：`grep -rn "type:vector(" backend-go/internal --include=*.go | grep -v "_test"` 仅剩测试文件命中
- [x] 4.2 确认 model 归一不引起 AutoMigrate 副作用（AutoMigrate 不 ALTER 类型），验证：`cd backend-go && go test ./internal/topicgraph/...` 退出码 0

## 5. 测试

- [x] 5.1 影响包测试（只跑受影响包），验证：`cd backend-go && go test ./internal/platform/database/... ./internal/tagmanagement/service/core/... ./internal/topicgraph/...` 退出码 0
- [x] 5.2 后端质量门禁，验证：`cd backend-go && golangci-lint run ./...` 退出码 0、`go vet ./...` 退出码 0、`go build ./...` 退出码 0

## 6. 文档

<!-- doc-impact: database, flow -->

- [x] 6.1 `docs/reference/database/` 增补向量维度契约说明：维度来自运行时模型输出、schema 不承载维度、写入前校验、索引维度跟随运行时 —— 落点：`tables/_conventions.md`「阅读约定」第 3 条 + 「向量维度规则总述」节
- [x] 6.2 `docs/reference/flow/daily-report.md` 同步：`daily_report_sections.embedding` 列类型由 `vector(2560)` 改为无维度限定，维度契约改为「取内容化文本 + 运行时维度」；核对「section embedding 必须取内容化文本而非标题，话题匹配锚点必须用 centroid」这条红线未被本 change 触及 —— 已核对：约束 4 的口径未动，只在其末尾追加维度契约说明
- [x] 6.3 `docs/reference/flow/semantic-board.md` / `docs/reference/flow/topic-graph.md`：同步向量列类型与维度契约的变化（若对应节有 embedding 维度描述）—— 两处对应红线末尾各追加一句维度契约说明（列类型不承载维度 + 维度是运行时属性），业务口径未动
- [x] 6.4 变更溯源：archive 后按 §12.2 补 flow 变更溯源链接（**归档时补，本阶段不勾**）——已补 `daily-report.md` / `semantic-board.md` / `topic-graph.md` 三处变更溯源行，指向本归档目录

## 7. 验证

- [x] 7.1 `cd backend-go && go test ./internal/platform/database/... ./internal/tagmanagement/service/core/... ./internal/topicgraph/...` → 退出码 0
- [x] 7.2 `cd backend-go && golangci-lint run ./...` → 退出码 0
- [x] 7.3 `cd backend-go && go vet ./...` → 退出码 0
- [x] 7.4 `cd backend-go && go build ./...` → 退出码 0
- [x] 7.5 `openspec validate unify-vector-column-dimension --strict` → 退出码 0
- [x] 7.6 `bash scripts/harness/scenario-trace.sh openspec/changes/unify-vector-column-dimension` → 退出码 0
- [x] 7.7 `bash scripts/harness/doc-impact.sh verify openspec/changes/unify-vector-column-dimension` → 退出码 0
- [x] 7.8 `bash scripts/harness/check-standards.sh --change unify-vector-column-dimension` → 退出码 0
- [x] 7.9 **存量库实跑核对**（在一次性恢复库上，不碰生产库）：恢复到最新备份档 → 应用启动（跑本 change 的迁移）→ 查 `pg_attribute` 确认全部向量列无维度限定 → 查各表行数与向量 `vector_dims` 与恢复前一致 → halfvec 索引 `indisvalid=true`，验证：核对输出记入 `review.md` §5
- [x] 7.10 `bash scripts/harness/review-trace.sh openspec/changes/unify-vector-column-dimension` → 退出码 0

### Scenario → 测试文件

| Scenario | 测试文件 |
| --- | --- |
| 维度取自模型实际输出 | `backend-go/internal/tagmanagement/service/core/embedding_dimension_test.go` |
| 配置表不持有维度键 | `backend-go/internal/tagmanagement/service/core/embedding_dimension_test.go` |
| 存量向量列统一为无维度限定 | `backend-go/internal/platform/database/vector_dimension_migration_test.go` |
| 迁移对已统一的列幂等 | `backend-go/internal/platform/database/vector_dimension_migration_test.go` |
| 列类型变更不丢弃向量值 | `backend-go/internal/platform/database/vector_dimension_migration_test.go` |
| 维度一致时写入成功 | `backend-go/internal/tagmanagement/service/core/embedding_dimension_test.go` |
| 维度不一致时拒绝写入 | `backend-go/internal/tagmanagement/service/core/embedding_dimension_test.go` |
| 维度变化后索引重建 | `backend-go/internal/tagmanagement/service/core/embedding_dimension_test.go` |
| 超过索引维度上限时跳过并记日志 | `backend-go/internal/tagmanagement/service/core/embedding_dimension_test.go` |
| 无效索引残留被清理 | `backend-go/internal/platform/database/vector_dimension_migration_test.go` |

- [x] 7.11 Review：派只读审查子线程审查全量制品与 diff（重点审：ALTER 是否在有数据表上安全、写入路径校验覆盖是否完整、索引重建逻辑在无限定列下是否仍被触发），意见按 High/Medium/Low 处置后在 `review.md` 建账，验证：`bash scripts/harness/review-trace.sh openspec/changes/unify-vector-column-dimension` 退出码 0
