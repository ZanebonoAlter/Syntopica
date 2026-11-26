# 审查 brief（窄范围版）：unify-vector-column-dimension

## 审查类型

**类型 A：openspec change 制品审查**（只读）。

> ⚠️ **执行纪律（上一轮审查因超时失败，这一轮必须遵守）**：
> 1. **禁止使用 `git diff`**（上一次它「丢了 HEAD 锚点」卡死）。改用 `read` 工具直接读文件。
> 2. **禁止无范围的全仓 `grep`**（上一次一条 grep 跑了 300s 超时）。grep 必须带具体路径前缀（如 `backend-go/internal/tagmanagement/`），并带 `-n` 与 `--include`。
> 3. **禁止跑 `go test` / `go build` / `golangci-lint` / 任何迁移、任何数据库命令**——只读。
> 4. **限时**：请在 15 次工具调用内完成核心审查并输出报告；宁可少查几处，也不要卡住。发现需要长时间穷举才能确认的问题，写成「待人工核实」条目，不要自己穷举。

## 审查对象（按优先级，只读这 5 个文件 + 2 个制品）

1. `backend-go/internal/platform/database/postgres_migrations.go` —— **只看本 change 新增的部分**：
   - `VectorColumnType` / `sampledVectorDimension` / `topicTagEmbeddingColumnDimension` / `halfvecExpressionDimension` / `topicTagIdentityHalfvecIndexState` / `TopicTagIdentityHalfvecIndexState` / `VectorIndexExists` / `EnsureTopicTagIdentityHalfvecIndex` / `ensureTopicTagIdentityHalfvecIndexForColumn`
   - `searchSemanticHalfvecIndexMigration`（20261023_0001，被改写）
   - `unifyVectorColumnDimensionMigration`（20261117_0001，新增）
   - `ensureTopicTagHalfvecIndexMigration`（20261117_0002，新增）
   - `vectorDimensionlessColumns` / `dropInvalidVectorIndexLeftovers`
   （用 grep 定位行号：`grep -n "func VectorColumnType\|func sampledVectorDimension\|func topicTagEmbeddingColumnDimension\|func halfvecExpressionDimension\|func TopicTagIdentityHalfvecIndexState\|func VectorIndexExists\|func EnsureTopicTagIdentityHalfvecIndex\|func ensureTopicTagIdentityHalfvecIndexForColumn\|func searchSemanticHalfvecIndexMigration\|func unifyVectorColumnDimensionMigration\|func ensureTopicTagHalfvecIndexMigration\|var vectorDimensionlessColumns\|func dropInvalidVectorIndexLeftovers" backend-go/internal/platform/database/postgres_migrations.go`）
2. `backend-go/internal/tagmanagement/service/core/embedding.go` —— 只看 `ensureVectorDimension` / `ensureTopicTagVectorIndexes` / `ensureTopicTagHalfvecIndex` / `SaveEmbedding`（grep 定位：`grep -n "func ensureVectorDimension\|func ensureTopicTagVectorIndexes\|func ensureTopicTagHalfvecIndex\|func (s \*EmbeddingService) SaveEmbedding" backend-go/internal/tagmanagement/service/core/embedding.go`）
3. `backend-go/internal/platform/airouter/embedding_dimension.go`（新文件，全文 ~70 行，直接读）
4. `backend-go/internal/tagmanagement/service/auxlabel/auxiliary_label_service.go` —— 只看 `ensureSemanticLabelVectorColumnDim`（grep 定位：`grep -n "func ensureSemanticLabelVectorColumnDim" backend-go/internal/tagmanagement/service/auxlabel/auxiliary_label_service.go`）
5. `backend-go/internal/topicgraph/repository/daily_report_models.go` —— 只看 `ensureVectorColumnDimensionAndIndex` / `ensureFloat32HNSWIndex`（grep 定位：`grep -n "func ensureVectorColumnDimensionAndIndex\|func ensureFloat32HNSWIndex" backend-go/internal/topicgraph/repository/daily_report_models.go`）
6. 制品：`openspec/changes/unify-vector-column-dimension/specs/embedding-dimension-contract/spec.md` 与 `review.md`（各读一遍，用于核对实现是否兑现契约）
7. 变更清单（用于核对覆盖面，只读 3 个小片段）：
   - `backend-go/internal/discovery/service/candidate_embedding_service.go` 的 `writeEmbedding`
   - `backend-go/internal/discovery/service/discovery_run_service.go` 的兴趣条目写入
   - `backend-go/internal/discovery/service/catalog_extras.go` 的 `EmbedPendingRoutes`

## 审查重点

1. **ALTER 是否在有数据表上安全**（迁移 `20261117_0001`）：幂等性、`withLockTimeout` 守卫、表/列不存在 no-op、是否可能重写表或丢数据、是否误删有效索引（`tmp_hnsw_test` 只删 invalid 的）。
2. **索引维度来源是否正确**（`20261023_0001` / `20261117_0002` 共用 helper）：列有限定 → 解析；列无限定 → 从数据采样；表空 → 跳过不猜维度。维度漂移时是否重建。
3. **运行期三分支是否自洽**：裸 `vector` 分支不得 ALTER、不得建 float32 索引（pgvector 会报 `column does not have dimensions`）；`vector(dim)` 分支确保索引存在；`vector(其他N)` 分支保留旧的 DROP+ALTER+重建。
4. **写入侧维度校验**：`airouter/embedding_dimension.go` 的语义（dim ≤ 0 放行、pgvector 文本解析是否可靠）+ 6 条带维度字段的写入路径是否都接上了校验。
5. **制品一致性**：spec 的 5 条 Requirement / 10 个 Scenario 与实现是否一一对应；`review.md` 的结论有没有超出实测支撑。

## 输出要求（三段式，中文，控制在 1500 字内）

1. **总评**：一句话结论 + 是否可归档（可归档 / 需修 N 处 / 阻断）。
2. **问题清单**：按 High / Medium / Low 分级，每条 `文件:行` + 问题 + 建议改法；某等级无问题就写「无」。不要为了凑数造问题。
3. **核实记录**：你实际跑了哪些只读命令与它们的结论。

## 纪律

- 只读审查，禁止修改任何仓库文件，禁止执行改变状态的命令。
- 遇到 brief 与现实冲突时如实记录在报告里，不自行改需求。
