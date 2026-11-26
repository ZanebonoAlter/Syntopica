<!-- complexity: simple -->
<!-- ui-impact: none -->
<!-- constraint-domains: semantic-board, topic-graph, discovery -->

## Why

标签向量落库报 `ERROR: value too long for type character varying(50) (SQLSTATE 22001)`，embedding 队列任务批量失败（实测 689 条 failed 全是这一条错误）。

根因是**列宽与模型名长度脱钩**：`topic_tag_embeddings.model` 列宽 `VARCHAR(50)`，而 embedding 模型名取自 `ai_models.name`（`VARCHAR(100)`）。本地 llama.cpp embedding 模型注册时「名字」直接填 GGUF 文件绝对路径 —— `/home/zanebono/.syntopica/models/Qwen3-Embedding-4B-Q6_K.gguf`（61 字符），超限 11 字符 → Postgres 直接拒绝 INSERT → 整条 embedding 任务失败（向量已经算出来却存不进去）。

同一条错误还有隐蔽放大器：命中 embedding 缓存时返回的是 `cached.Model`（缓存行里存的就是当初本地模型写的 61 字符路径），所以**即使主路由是远端模型也会照样炸**。

同类隐患不止一处：`candidate_embeddings` / `preference_vectors` / `route_embeddings` / `discovery_interest_entries` 四张 discovery 域表共用同一个 `model VARCHAR(50)`，存的是同一个值，一走 discovery 链路就会复现同一错误。

## What Changes

- **列宽对齐**：5 张 embedding 表的 `model` 列 `VARCHAR(50)` → `VARCHAR(100)`，与模型注册表 `ai_models.name`（`VARCHAR(100)`）对齐。建立不变式「**存 embedding 模型名的列宽 MUST NOT 窄于 `ai_models.name` 列宽**」——模型名在注册表里能放下，在任何落库处就必须放得下。
- **gorm tag 同步**：5 处模型定义的 `model` 字段 `size:50` → `size:100`，防止 GORM AutoMigrate 把列漂回窄列（tag 与 DDL 漂移 = 下次重启又炸）。
- **显式迁移**：`20261103_0001`（幂等 + **条件执行**：列不存在或列宽已达标则完全不动，不取锁、不重建索引；加宽不截断数据、不重写表体）+ `20261103_0002`（恢复 `topic_tag_embeddings.model` 的 NOT NULL——见下）。
- **顺带修复一条本 change 自己引入的约束回归**：改 gorm tag 触发 GORM AutoMigrate 对存量列发 ALTER，而 GORM postgres driver 的 `AlterColumn` 在 `field.NotNull == false`（本仓库约定 not null 不写进 tag）时执行 `DROP NOT NULL`——`topic_tag_embeddings.model` 原本由迁移 `20260723_0001` 固化的 NOT NULL 被静默丢弃。`20261103_0002` 用既有 `ensureNotNullDefault` 恢复（幂等），测试补断言守住。
- **回归测试**：① 迁移幂等 + 存量行不丢（testcontainer PG，存量值取**恰好 50 字符**以对截断敏感 + 真实向量原样保留断言）；② 列宽不变式**数据驱动**（全库所有 `model` 列宽度 ≥ `ai_models.name`，将来新增窄列即红）+ gorm tag 与 DDL 一致 + `model` 仍是 NOT NULL。
- **存量数据修复**：把 `embedding_queues` 中因 22001 失败的任务重排为 pending，由后台 worker 重新生成向量（走既有 `RetryFailed` 通道，不新增代码路径）。

## Capabilities

### New Capabilities

（无）

### Modified Capabilities

- `tag-embedding-management`: 新增 Requirement「embedding 模型名列宽不窄于模型注册表」——`topic_tag_embeddings.model` 列宽契约 + 保存长模型名不因超限失败 + gorm tag 与 DDL 一致。
- `feed-discovery`: 新增 Requirement「discovery 向量表模型名列宽不窄于模型注册表」——discovery 域 4 张向量表（候选向量 / 偏好向量 / 路由向量 / 兴趣条目）的 `model` 列宽契约。

## Impact

- **后端代码**：
  - `backend-go/internal/platform/database/postgres_migrations.go`（新增迁移 `20261103_0001`）
  - `backend-go/internal/tagmanagement/models/tag_models.go`（`TopicTagEmbedding.Model`）
  - `backend-go/internal/discovery/models/catalog.go`（`RouteEmbedding.Model`）
  - `backend-go/internal/discovery/models/discovery_run.go`（`CandidateEmbedding.Model`、`DiscoveryInterestEntry.Model`）
  - `backend-go/internal/discovery/models/preference_vector.go`（`PreferenceVector.Model`）
- **测试**：`backend-go/internal/platform/database/embedding_model_column_width_test.go`（新增，testcontainer PG）
- **数据**：
  - 5 张表 `model` 列类型变更（加宽 → 不截断、不丢行；`topic_tag_embeddings` 实际 4.1 万行，堆 10 MB / 含 HNSW 索引共 868 MB，varchar typmod 变更不重写表体）
  - `topic_tag_embeddings.model` 的 NOT NULL 约束恢复（被本 change 的 tag 变更顺带丢弃，由 `20261103_0002` 还原）
  - `embedding_queues` 689 条 failed 任务重排为 pending（数据修复，非 schema 变更）
- **前端**：无（`ui-impact: none`，不涉及任何页面/组件/文案）
- **API**：无接口签名/响应变更
- **依赖**：无新增依赖
- **部署时机**：选 embedding worker 空闲窗口重启（Phase 1 AutoMigrate 的 ALTER 无 lock_timeout 守卫，会等 `ACCESS EXCLUSIVE` 锁）

### 部署后影响 + 需要的操作 + 旧数据降级

- **(a) 用户可见行为变化**：标签向量写入不再因模型名超限失败——本地 embedding 模型（llama.cpp GGUF）产出的向量能正常落库；版块匹配、标签召回、日报去重的语义路恢复可用。管理后台的「embedding 队列」失败计数归零、待处理数下降。**除此之外无任何可见变化**（不改前端、不改算法、不改模型调用）。
- **(b) 需要用户手动执行的操作**：① 重启后端（迁移在启动时执行；用 `bash scripts/dev/start-dev.sh --restart`，**选 embedding worker 空闲窗口**）；② 失败任务重排由本 change 收尾时调用既有 `POST /api/embedding/queue/retry` 完成，**若用户自行部署需手动触发一次**（或等本 change 的收尾动作已执行）。
- **(c) 旧数据降级**：本 change **不删任何行、不改任何既有向量**——`model` 列只加宽不截断，历史 `model` 值（`qwen3-embedding` 等短名）原样保留；旧向量语义不变，无需重新生成。回滚路径：把列改回 `VARCHAR(50)` 会截断 61 字符的本地模型名（数据丢失风险），**不建议回滚**；若必须回滚，先把 `ai_models.name` 改回 ≤50 字符再降列宽。
