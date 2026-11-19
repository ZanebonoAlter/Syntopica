# test-cases: unify-vector-column-dimension

> 本 change 是**基础设施/数据契约类**变更（无界面、无用户可见交互）。故事主角是
> 「部署并运行 Syntopica 的运维方 + 写入向量的应用路径」——故事讲的是：升级后向量
> 列不再声明维度、存量向量值不动；写入路径对维度不一致的向量硬拦；换模型后 ANN
> 索引跟着运行时维度走，语义召回不退化。
> 场景（Scenario）即断言片段，本文件把 Requirement 的节拍串成完整故事。

## 故事 S1: 升级后向量列统一为无维度限定，存量数据分毫不动（锚 Requirement: 向量列的 schema 不承载维度）

### 主链路（节拍串联）

| 步 | 动作 | 来源 Scenario | 期望 | 层 | 落点（测试/人工） |
| 1 | 运维在带存量向量的库上跑升级（启动即触发迁移） | 存量向量列统一为无维度限定 | 六列的 `format_type` 均为 `vector`（无 typmod） | testcontainer PG | `internal/platform/database/vector_dimension_migration_test.go` → `TestVectorDimensionMigrationUnifiesAllVectorColumns` |
| 2 | 同一次升级后核对存量向量 | 列类型变更不丢弃向量值 | 行数不变、非空向量的 `vector_dims` 不变 | testcontainer PG | 同文件 → `TestVectorDimensionMigrationKeepsRowsAndVectorDims` |
| 3 | 重复跑升级（重放 / 灾备从零重放） | 迁移对已统一的列幂等 | 迁移成功、列类型不变、无额外 DDL 效果 | testcontainer PG | 同文件 → `TestVectorDimensionMigrationReplayIsIdempotent` |
| 4 | 升级后语义召回仍然走 ANN | （沿用 optimize-search-semantic-index 口径） | halfvec identity 索引存在且 `indisvalid=true` | 人工 + testcontainer PG | 人工：一次性临时库实测（`review.md` §1.2）；自动：同文件 → `TestVectorDimensionMigrationBuildsHalfvecIndexForDimensionlessColumn` |
| 5 | 升级后清理失败残留 | 无效索引残留被清理 | `tmp_hnsw_test`（invalid）被删；同名**有效**索引保留 | testcontainer PG | 同文件 → `TestVectorDimensionMigrationDropsInvalidIndexLeftover` |

### 变体走查

| # | 变体（组/条目） | 期望答案 | 层 | 落点 |
| --- | --- | --- | --- | --- |
| 1 | 输入变体：列已是无限定 `vector` | 跳过（幂等，不取锁、不 ALTER） | testcontainer PG | `TestVectorDimensionMigrationReplayIsIdempotent` |
| 2 | 输入变体：列是 `vector(2560)`（存量形态） | ALTER 到无限定；存量值不动 | testcontainer PG | `TestVectorDimensionMigrationUnifiesAllVectorColumns` |
| 3 | 输入变体：列是 `vector(其他N)` | 同上（不限 N，去 typmod 是放宽约束） | testcontainer PG | 同上（测试用 2560，机制与 N 无关） |
| 4 | 前置变体：表不存在 / 列不存在（最小部署） | 迁移 no-op，不报错、不阻断启动 | testcontainer PG | 迁移内 `tableExists` / `columnExists` 守卫（人工核对代码路径） |
| 5 | 前置变体：表空（无任何向量） | 维度采样不到 → 跳过建索引 + 记日志，不猜维度 | testcontainer PG | `TestVectorDimensionMigrationBuildsHalfvecIndexForDimensionlessColumn`（前半段） |
| 6 | 前置变体：表有数据（维度 64） | 从数据采样得到 64 → 按 64 建索引 | testcontainer PG | 同上（后半段） |
| 7 | 时间窗口变体：迁移重复执行 / 启动重放 | 幂等（步 3） | testcontainer PG | `TestVectorDimensionMigrationReplayIsIdempotent` |
| 8 | 幂等变体：索引已存在且维度正确 | 不重建（CREATE INDEX IF NOT EXISTS 生效） | testcontainer PG | `core` 包的 `TestVectorIndexRebuiltOnDimensionChange`（前半段） |
| 9 | 可用性变体：迁移被锁阻塞（大表 ALTER 抢不到锁） | 5s lock_timeout 快速失败而非无限阻塞写入 | testcontainer PG | `withLockTimeout` 守卫（既有 helper，`internal/platform/database/lock_timeout_test.go` 覆盖其语义） |
| 10 | 可用性变体：invalid 索引残留 | 被清理（步 5） | testcontainer PG | `TestVectorDimensionMigrationDropsInvalidIndexLeftover` |
| 11 | 输入变体：空串 / 纯空白 / 超长输入 | **不适用**——本故事无文本输入面（迁移只吃库里的结构与数据，不吃用户输入） | — | 划除留痕 |
| 12 | UI 变体：误输入反馈 / 空态 / 错误态 | **不适用**——本 change 无界面改动（proposal 声明 `ui-impact: none`） | — | 划除留痕 |

### 效果核对

- **触发原因**：迁移在**有数据的生产表**上执行（`topic_tag_embeddings` 5.1 万行 × 2560 维 ≈ 512MB 量级），而「去 typmod 是否重写表」决定了升级窗口的长度。
- **核对方法**：一次性临时库（`dr_probe_tmp` / `uvcd_probe`）上实测 `ALTER ... TYPE vector` 前后对比 `pg_class.relfilenode`（重写则变）与 `vector_dims`（数据不变则同）；生产库全程未碰。
- **量化结果**：`relfilenode` 改前改后均为 `1686539` / `2303485`（**未变 ⇒ 未重写表**）；20 行样本的 `vector_dims` 均为 2560（**存量值不变**）；索引 `indisvalid` 改前改后均为 `t`。
- **结论**：**达标交付**——升级窗口只付元数据改写成本，不付全表重写。

### 白盒附加

分支表与边界值见下方「白盒附加」节。

---

## 故事 S2: 写入路径对维度不一致的向量硬拦（锚 Requirement: 向量写入前做维度一致性校验）

### 主链路（节拍串联）

| 步 | 动作 | 来源 Scenario | 期望 | 层 | 落点（测试/人工） |
| 1 | 应用写入一条维度与声明一致的向量 | 维度一致时写入成功 | 写入成功；落库 `dimension` 与 `vector_dims` 都等于声明维度 | testcontainer PG | `internal/tagmanagement/service/core/embedding_dimension_test.go` → `TestEmbeddingDimensionConsistentWriteSucceeds` |
| 2 | 应用写入一条维度与声明不符的向量 | 维度不一致时拒绝写入 | 写入被拒；错误信息含期望维度与实际维度；**不落库** | testcontainer PG | 同文件 → `TestEmbeddingDimensionMismatchRejected` |
| 3 | 运维检查「这批向量到底几维」 | 维度取自模型实际输出 | 落库维度 = 向量实际维度；配置表里的维度键**不能覆盖**它 | testcontainer PG | 同文件 → `TestEmbeddingDimensionComesFromModelOutput` |
| 4 | 运维检查「配置表里还有没有维度键」 | 配置表不持有维度键 | `embedding_config` 无维度键；向量列的 `atttypmod` 全为 -1（类型声明不带维度） | testcontainer PG | 同文件 → `TestEmbeddingConfigHoldsNoDimensionKey` |

### 变体走查

| # | 变体（组/条目） | 期望答案 | 层 | 落点 |
| --- | --- | --- | --- | --- |
| 1 | 输入变体：声明维度 0（维度未知） | 放行（没有可比对的目标，不构成不一致） | 单元 | `airouter.ValidatePgVectorDimension` 的 `dim <= 0` 分支 |
| 2 | 输入变体：向量文本畸形（非 `[a,b,c]` 形态 / 空串） | 判为维度校验失败 → 拒写（畸形向量本来也写不进库） | 单元 | 同上（`PgVectorDimension` 返回错误） |
| 3 | 输入变体：向量 8 维、声明 16 维 | 拒写（步 2） | testcontainer PG | `TestEmbeddingDimensionMismatchRejected` |
| 4 | 前置变体：父标签已删（写不进去） | 仍返回「父标签不存在」（维度校验先过，语义顺序不变） | testcontainer PG | 既有 `embedding_test.go` → `TestSaveEmbeddingReturnsTagNotFoundWhenParentDeleted` |
| 5 | 前置变体：表里已有别的维度的存量向量 | 维度不一致的那次写入被拒；既有行不动 | testcontainer PG | `TestEmbeddingDimensionMismatchRejected`（写入前后行数断言） |
| 6 | 时间窗口变体：同一批次内多向量 | 每条各自校验（批内异构长度会被逐条拦下） | 单元 + 代码核对 | `related/refresh.go` 的既有逐条 `validateVector` |
| 7 | 幂等变体：重复写同一条 | 幂等（先删同 tag+type 的旧 hash 行再写） | testcontainer PG | 既有 `embedding_test.go` → `TestSaveEmbeddingCleansUpStaleRecords` |
| 8 | 可用性变体：依赖挂了（provider 返回空向量） | 空向量不写入（既有闸门） | testcontainer PG | 既有 `related/refresh.go` `validateVector`（`ReasonEmptyVector`） |
| 9 | UI 变体 | **不适用**——无界面改动 | — | 划除留痕 |
| 10 | 前置变体：配置表被写入一个伪造维度键 | 不影响写入结果（配置表不参与维度判定） | testcontainer PG | `TestEmbeddingDimensionComesFromModelOutput`（故意塞 `embedding_dimension=999`） |

### 效果核对

- **触发原因**：本 Requirement 的价值全在「拦住错维度的向量」——但拦截率不可由单测断言（错维度是罕见事件）。
- **核对方法**：核对**覆盖面**而非拦截率——列出全部向量写入路径（`review.md` §2，15 行），逐行标注「有/无随值留存的维度字段」与「是否过校验」；带维度字段的 6 条路径必须全部过校验。
- **量化结果**：6/6 条带维度字段的路径过校验（#2 原有、#1/#3/#4②/#5/#6 本次补齐）；9 类不带维度字段的列判定为「不适用」且未新增字段。
- **结论**：**达标交付**——覆盖面可枚举、可核对。

### 白盒附加

见下方「白盒附加」节。

---

## 故事 S3: 换 embedding 模型后 ANN 索引跟着运行时维度走（锚 Requirement: 向量索引的维度跟随运行时维度）

### 主链路（节拍串联）

| 步 | 动作 | 来源 Scenario | 期望 | 层 | 落点（测试/人工） |
| 1 | 运行时维度与索引表达式内嵌维度不一致 | 维度变化后索引重建 | 索引被重建为内嵌新维度的表达式；重建失败记警告、不阻断向量写入 | testcontainer PG | `core/embedding_dimension_test.go` → `TestVectorIndexRebuiltOnDimensionChange` |
| 2 | 运行时维度超过 halfvec 上限（4000） | 超过索引维度上限时跳过并记日志 | 跳过建索引（语义召回回退精确扫描，行为不中断）；不留旧维度索引把列钉死 | testcontainer PG | 同文件 → `TestVectorIndexSkippedAboveHalfvecLimit` |
| 3 | 列已是无维度限定时的运行期纠正 | （design D3） | **不 ALTER 列**（不把分叉源搬回 DDL） | testcontainer PG | 同文件 → `TestVectorDimensionColumnStaysDimensionless` |
| 4 | 索引表达式内嵌维度不符时写入新维度向量 | （实测结论） | 旧索引会拒写（`expected N dimensions, not M`）→ 所以维度漂移必须重建索引 | 人工 | `review.md` §1.2（一次性临时库实测） |

### 变体走查

| # | 变体（组/条目） | 期望答案 | 层 | 落点 |
| --- | --- | --- | --- | --- |
| 1 | 输入变体：维度 = 4000（halfvec 上限边界） | 可建索引 | testcontainer PG | `TestVectorIndexRebuiltOnDimensionChange`（16/32 为边界内的样本；上限边界由 `dim <= 4000` 判定） |
| 2 | 输入变体：维度 = 4001（超上限） | 跳过 + 记日志 | testcontainer PG | `TestVectorIndexSkippedAboveHalfvecLimit`（5000） |
| 3 | 输入变体：维度 = 2000（float32 HNSW 上限边界） | 裸列上不建 float32 索引（pgvector 限制），只保留 halfvec | 代码核对 | `core.ensureTopicTagVectorIndexes` 的裸列分支 |
| 4 | 前置变体：索引已存在且维度正确 | 不重建 | testcontainer PG | `TestVectorIndexRebuiltOnDimensionChange`（前半段） |
| 5 | 前置变体：索引 invalid（历史构建失败残留） | 先 DROP 再建 | testcontainer PG | `database.EnsureTopicTagIdentityHalfvecIndex`（invalid 判定） |
| 6 | 前置变体：表空（采样不到维度） | 跳过建索引 + 记日志，等首次写入时由运行期建 | testcontainer PG | `TestVectorDimensionMigrationBuildsHalfvecIndexForDimensionlessColumn` |
| 7 | 幂等变体：运行期重复调用纠正器 | 已是正确表达式则不动 | testcontainer PG | `TestVectorIndexRebuiltOnDimensionChange`（第二次调用） |
| 8 | 可用性变体：索引构建失败（数据维度与索引维度不符） | 记警告日志，不阻断向量写入 | testcontainer PG | `ensureTopicTagHalfvecIndex` 的错误分支（记 Warnf 后返回） |
| 9 | UI 变体 | **不适用**——无界面改动 | — | 划除留痕 |

### 效果核对

- **触发原因**：ANN 索引是语义召回的性能底座（真库实测：全表扫描 3.74s vs 索引 top-50 15ms），索引失效 = 召回慢一个数量级（行为仍正确）。
- **核对方法**：一次性临时库实测「ALTER 列类型前后索引的 `indisvalid` 与 `pg_get_indexdef`」+「ANN 查询的 EXPLAIN 是否走 Index Scan」。
- **量化结果**：ALTER 前后 `indisvalid` 均为 `t`；索引定义不变（`((embedding)::halfvec(2560)) halfvec_cosine_ops`）；EXPLAIN 显示 `Index Scan using idx_probe_halfvec2560b`。
- **结论**：**达标交付**——列类型统一不损伤 ANN 路径。

---

## 白盒附加（复杂档）

### 分支表

| # | 条件/分支 | 输入 | 期望 | 测试用例名 |
| --- | --- | --- | --- | --- |
| B1 | 列类型 = 裸 `vector` | `ensureVectorDimension(dim)` | 不 ALTER；按 dim 校验/重建 halfvec 索引 | `TestVectorDimensionColumnStaysDimensionless` / `TestVectorIndexRebuiltOnDimensionChange` |
| B2 | 列类型 = `vector(dim)`（与运行时一致） | 同上 | 列不动；确保索引存在 | `TestEmbeddingDimensionConsistentWriteSucceeds`（隐式）+ `ensureTopicTagVectorIndexes` 分支 2 |
| B3 | 列类型 = `vector(其他N)` | 同上 | DROP 索引 + ALTER + 重建（迁移过渡期分支） | 代码核对（迁移 `20261117_0001` 跑完后该分支不可达） |
| B4 | 索引表达式内嵌维度 ≠ 运行时维度 | `dim` 变化 | 先 DROP 再建；建失败记警告 | `TestVectorIndexRebuiltOnDimensionChange` |
| B5 | 索引 invalid | `indisvalid=false` | 先 DROP 再建 | `database.EnsureTopicTagIdentityHalfvecIndex`（invalid 判定） |
| B6 | 运行时维度 > 4000 | `dim=5000` | 跳过建索引 + 记日志；不留旧维度索引 | `TestVectorIndexSkippedAboveHalfvecLimit` |
| B7 | 写入：声明维度 = 向量实际维度 | `dim=8` | 写入成功 | `TestEmbeddingDimensionConsistentWriteSucceeds` |
| B8 | 写入：声明维度 ≠ 向量实际维度 | `声明 16 / 实际 8` | 拒写 + 错误日志（含两维）+ 不落库 | `TestEmbeddingDimensionMismatchRejected` |
| B9 | 写入：声明维度 ≤ 0（未知） | `dim=0` | 放行（无可比对目标） | `airouter.ValidatePgVectorDimension` 单元分支 |
| B10 | 迁移：列已是裸 `vector` | 重放 | 跳过（幂等） | `TestVectorDimensionMigrationReplayIsIdempotent` |
| B11 | 迁移：同名 invalid 索引 | `tmp_hnsw_test` invalid | 删除 | `TestVectorDimensionMigrationDropsInvalidIndexLeftover` |
| B12 | 迁移：同名有效索引 | `tmp_hnsw_test` valid | 保留（不误删） | 同上 |
| B13 | 迁移：裸列 + 有数据 | 64 维样本 | 采样 64 → 建 halfvec(64) | `TestVectorDimensionMigrationBuildsHalfvecIndexForDimensionlessColumn` |
| B14 | 迁移：裸列 + 表空 | 无数据 | 跳过 + 记日志（不猜维度） | 同上 |

### 边界值清单

| 变量 | 边界值 | 期望 | 测试用例名 |
| --- | --- | --- | --- |
| 向量维度 | 4000 | halfvec 索引可建（≤4000） | `TestVectorIndexRebuiltOnDimensionChange`（判定分支 `dim <= 4000`） |
| 向量维度 | 4001 | 跳过建索引 + 记日志 | `TestVectorIndexSkippedAboveHalfvecLimit`（5000 为超限样本） |
| 向量维度 | 2000 | float32 HNSW 理论上限；裸列上仍不建（pgvector 限制） | 代码核对 |
| 声明维度 | 0 | 放行 | `airouter` 单元分支 |
| 声明维度 | 负数 | 放行（`<= 0` 同分支） | `airouter` 单元分支 |

### 不适用划除（留痕）

- **UI 交互类变体**（误输入反馈 / 空态 / 错误态 / 加载态 / 重复提交）：本 change 无前端改动（proposal 头 `ui-impact: none`），无界面落点。
- **输入文本类变体**（空串 / 纯空白 / 超长输入）：迁移与维度校验都不吃用户文本输入（只吃库结构与向量本身），无对应面。
- **跨端/跨端协议类**：本 change 不涉及素材包协议与 embedding 空间版本号（design D8 明确不在范围内）。

---

## 问句⓪ 继承与调整

本 change 是**纯新增 capability**（`embedding-dimension-contract`），delta spec 里**没有** MODIFIED / REMOVED Requirements → 问句⓪不适用（旧测试不存在断言旧契约的问题）。

但为免遗漏，逐条核对了受影响的既有测试资产（`bash scripts/harness/test-assets.sh embedding-dimension-contract` 无历史资产——该 capability 是本次新增）：

| 既有测试 | 处置 | 说明 |
| --- | --- | --- |
| `internal/platform/database/vector_dimension_migration_test.go`（前 5 个用例） | 继承（继续跑） | 覆盖 `20260403_0003` 的旧口径，本 change 未改该迁移的行为（只改 `20261023_0001` 的维度来源解析） |
| `internal/tagmanagement/service/core/embedding_test.go` | 继承（继续跑） | `SaveEmbedding` 新增维度校验后仍绿（用例里的维度与向量一致） |
| `internal/platform/database/embedding_model_column_width_test.go` | 继承（继续跑） | 该测试已兼容「列无 typmod」的形态（回落到 halfvec 索引维度 → 再回落到 `TestEmbeddingDim`） |
| `internal/tagmanagement/service/core/embedding_index_recall_test.go` | 继承（继续跑） | 自建 `idx_tte_*` 索引，不依赖生产索引名 |

---

## 问句⑤ 展示字段盘点

本 change **不改任何数据结构里用户界面直接展示的字段**（向量列本身 `json:"-"` 不出接口；`dimension` 字段既不改名也不改语义）。故问句⑤不适用。
