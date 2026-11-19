<!-- complexity: complex -->
<!-- ui-impact: none -->
<!-- constraint-domains: daily-report, semantic-board, topic-graph -->
<!-- 域声明理由：编辑路径 backend-go/internal/tagmanagement/（→ semantic-board / topic-graph）与 backend-go/internal/topicgraph/（→ daily-report）命中 doc-impact-applies 标签 -->

## Why

向量维度被写进了 DDL，于是同一个仓库里同时活着三个数：**现网 2560**（`topic_tag_embeddings.embedding` / `daily_report_sections.embedding` / `semantic_labels.*` / `term_notes.embedding` / `board_persistent_topics.embedding`）、**迁移 0003 硬编码的 4096**、**seed 里已被删除的 1024 死配置**。由���长出三处结构性问题：

1. **schema 与维度耦合**：换 embedding 模型/维度就要 ALTER 全表重写，而这条 ALTER 路径正是 2026-11-04 定位到的灾备恢复失败根因（`vector` → `vector(4096)` 不 binary coercible，pgvector 无跨维度 cast → 迁移失败 → 应用退出）。
2. **model 与实际 schema 长期对不上**：库里 6 张表是 `vector(2560)` 限定、8 张是无限定 `vector`，而 **14 处 model 声明几乎全是无限定 `type:vector`**（唯一例外 `margin_notes_repository.go:82` 写死 `vector(2560)`）。GORM AutoMigrate 明确 "Never drops or alters"（`internal/platform/database/db.go` Phase 1 注释），所以两边永远不会自动收敛——每次都靠人肉记忆。
3. **库里有失败残留**：一个名为 `tmp_hnsw_test` 的 **invalid 索引**（`indisvalid=false`）躺在 `topic_tag_embeddings` 上——float32 HNSW 建在 2560 维列上，超过 pgvector 2000 维硬上限故创建失败，残骸从未清理。

维度本该是**运行时属性**（`embedding_config.embedding_dimension` 早已被迁移 `20260614_0001` 删为死配置，真库无此行），不是 schema 契约。运行时真相源一直存在：`embedding.go:99` 把模型实际输出维度写进每行的 `Dimension` 字段，`ensureVectorDimension` 也据此自动纠正列类型——只是纠正动作本身又落回 DDL，成了新的分叉源。

本 change 是 `fix-disaster-recovery-channel` 之后紧接的第二个收口：把存量维度统一退出 schema，让「维度只活在运行时与数据里」成为可长期维持的终态。

## What Changes

- **存量列统一**：新增迁移，把 5 张表的 6 个向量列从 `vector(2560)` 改为**无维度限定的 `vector`**（幂等：已是无限定则跳过）。
- **清理失败残留**：新增迁移 `DROP INDEX` 掉 invalid 索引 `tmp_hnsw_test`。
- **纠正器改造**：`ensureVectorDimension` 在列已是无限定 `vector` 时**不再 ALTER 列**，改为按运行时模型维度校验并按需重建 halfvec 索引（索引表达式内嵌维度，维度变了索引必须跟着重建）。
- **model 归一**：唯一的 `type:vector(2560)`（`margin_notes_repository.go:82`）改为 `type:vector`，与其它 14 处一致。
- **索引维度跟随运行时**：`idx_topic_tag_embeddings_identity_halfvec_hnsw` 的表达式 `embedding::halfvec(2560)` 内嵌维度，维度变更后必须重建——把这条责任明确落到 `ensureVectorDimension`。

**依赖**：本 change 的核心假设「`vector(2560)` → `vector` 可行且不重写数据」由 `fix-disaster-recovery-channel` 的任务 2.2 实测提供（该任务在含非空向量的表上实测该 ALTER）。**该实测未出结论前不得开始实现本 change 的迁移部分。**

**不含**：不引入 embedding 空间的显式版本号（属素材包协议 change）；不改动素材包下发逻辑；不更换 embedding 模型。

## Capabilities

### New Capabilities

- `embedding-dimension-contract`: 向量维度的契约——运行时模型输出是唯一真相源、schema 不承载维度、写入前一致性校验、索引维度跟随运行时

### Modified Capabilities

（无——`db-migration-safety` 的「向量列维度迁移不硬编码维度」约束已由 `fix-disaster-recovery-channel` 引入，本 change 是它的存量落地，不重复修改）

## Impact

- **改动代码**：
  - `backend-go/internal/platform/database/postgres_migrations.go`：新增一条版本迁移（列类型统一 + DROP invalid 索引）。
  - `backend-go/internal/tagmanagement/service/core/embedding.go`：`ensureVectorDimension` 的列类型分支改造（无限定列不再 ALTER）。
  - `backend-go/internal/topicgraph/repository/margin_notes_repository.go:82`：`type:vector(2560)` → `type:vector`。
- **数据影响**：向量列的存储值**不变**（只改列的 typmod 约束，由「必须 2560 维」放宽为「不限维度」）；`Dimension` 字段与实际向量值不变。
- **索引影响**：`idx_topic_tag_embeddings_identity_halfvec_hnsw`（表达式内嵌 `halfvec(2560)`）在列类型变更后需验证仍有效；`tmp_hnsw_test`（invalid）被删除。**无 float32 HNSW 索引需要重建**（2560 维超过 pgvector float32 HNSW 的 2000 维硬上限，本就不存在）。
- **部署后用户可见变化**：无界面变化。行为差异有两处——① 向量列不再拒绝非 2560 维的写入（改由应用层校验，见 capability）；② `ensureVectorDimension` 不再自动把列 ALTER 成限定类型。
- **旧数据降级**：不涉及数据迁移，存量向量值原样保留。
- **执行窗口**：实测（2026-11-17，一次性恢复库 `syntopica_restore_*`，档 `pg-key-20261112-040001.dump`）：**不重写表体**（带 halfvec 索引的同形表 ALTER 前后 `relfilenode` / 表大小均不变），但 `topic_tag_embeddings`（33,832 行 × 2560 维）上的 ALTER 耗时 **310s**——成本在**该表 3 个 halfvec HNSW 索引被 PG 随 ALTER 重建**，不在数据重写；同一迁移里其余五列（合计 21 万行）在同一秒内完成。**需在低峰执行**（启动期 ~5 分钟的索引重建），迁移内逐表打点日志可观察进度。详见 `review.md` §5.1。
