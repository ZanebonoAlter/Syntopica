# Design: fix-embedding-model-column-width

## Context

embedding 模型名的写入源只有一处语义：`airouter.EmbeddingResult.Model`，它取自路由绑定的 `ai_models.name`。模型注册表 `ai_models.name` 的列宽是 `VARCHAR(100)`（自 `20261005_0001` ai-provider-model-overhaul 起），但**五张落库表的 `model` 列停留在 `VARCHAR(50)`**——它们建于模型注册表两层化之前，沿用了旧的窄宽度约定，之后再没人对齐过。

本地 llama.cpp embedding 模型注册时「名字」填的是 GGUF 文件绝对路径（61 字符），超限 11 字符 → `SQLSTATE 22001` → `SaveEmbedding` 失败 → 整条 embedding 任务失败（实测 689 条 failed 全是这一条错误）。

放大器在缓存：embedding 缓存命中时返回 `cached.Model`（缓存行里存的是当初产出该向量的模型名），所以「主路由是远端短名模型」也挡不住——缓存里躺着的本地模型路径照样被写进 50 字符的列。

约束：
- 加宽必须在**存量有行的生产表**上跑（`topic_tag_embeddings` 约 4 万行 / 868 MB）；
- gorm tag 与 DDL 必须同步，否则下次 AutoMigrate 把列改回窄列，等于没修；
- discovery 域四张表同病，不能只修触发报错的那一张。

## Goals / Non-Goals

**Goals**
- 五张表 `model` 列宽与 `ai_models.name` 对齐（100），任意注册得下的模型名都能落库。
- 建立「存 embedding 模型名的列宽 ≥ 模型注册表 `name` 列宽」不变式，并用测试守住。
- 存量向量零改动、零重算；失败任务可重排找回。

**Non-Goals**
- 不改模型名本身（不把本地模型改名成短别名）——那会让 `ai_embedding_cache` 缓存键失效、触发一次性全量重算，且属于配置层而非根因层。
- 不在 Go 侧截断/哈希模型名——截断会让 `candidate_embeddings` 的 `(candidate_id, model, dimension)` 唯一键把不同模型误判为同一条向量。
- 不动 embedding 调用链、路由策略、缓存策略（另见 Risks 里的已知相邻问题）。
- 不动前端。

## Decisions

### D1. 目标列宽取 100，与 `ai_models.name` 对齐

- **结论**：五张表 `model` 列 `VARCHAR(50)` → `VARCHAR(100)`，目标值取注册表 `ai_models.name` 的实际列宽，而不是另取一个「够用」的数字。
- **落点**：`backend-go/internal/platform/database/postgres_migrations.go`
- **取舍**：备选取 200/255（TEXT）留余量——被否：列宽上限失去「与注册表同宽」这个可校验的语义，不变式退化成「≥50」这种无法从注册表推导的口头约定；取 100 后「注册表能存下 ⇒ 所有落库处能存下」成为可机械断言的等式。

### D2. 一次修五张表，不只修触发报错的那张

- **结论**：同批加宽 `topic_tag_embeddings`（tagmanagement）、`candidate_embeddings` / `preference_vectors` / `route_embeddings` / `discovery_interest_entries`（discovery）。
- **落点**：`backend-go/internal/discovery/models/catalog.go`
- **取舍**：备选只修 `topic_tag_embeddings`（用户第一直觉，改动最小）——被否：discovery 四张表存的是同一个值，只是当前还没跑到那条链路；分两批修等于把同一个 bug 留一半在生产库里，且第二个人还得重新排查一次。

### D3. gorm tag 同步改成 `size:100`，杜绝 AutoMigrate 漂回

- **结论**：五处模型定义的 `model` 字段由 `size:50` 改为 `size:100`，与 DDL 同批落地。
- **落点**：`backend-go/internal/tagmanagement/models/tag_models.go`
- **取舍**：备选只写迁移不动 tag——被否：GORM AutoMigrate 在启动时按模型定义比对列宽，不一致就 ALTER 回去；那样迁移每次启动都会被自动"回滚"，问题复发且更难查（迁移记录显示已执行，列却是窄的）。

### D4. 显式版本化迁移（幂等），不依赖 AutoMigrate 副作用

- **结论**：新增迁移 `20261103_0001`，逐表 `ALTER TABLE ... ALTER COLUMN model TYPE VARCHAR(100)`；**条件执行**——表不存在跳过、列不存在跳过、列宽已 ≥100 跳过（真 no-op：不 ALTER、不取 `ACCESS EXCLUSIVE` 锁、不重建含该列的索引），只有列宽 <100 才 ALTER；可重复执行。
- **落点**：`backend-go/internal/platform/database/postgres_migrations.go`
- **取舍**：备选只靠 AutoMigrate 改列宽——被否：AutoMigrate 的 ALTER 行为随 GORM 版本变化、不可审计、也没有"这条变更发生过"的记录；版本化迁移是本仓库 DDL 变更的既有落点（见 `docs/reference/standard/backend/testing.md` 的迁移规范）。备选无条件 ALTER（首版实现）——被否：PG 实测对已是 `varchar(100)` 的列再执行 ALTER 仍会**重建**该列上的索引（表体不重写但索引 oid 变），且要拿 `ACCESS EXCLUSIVE` 锁；条件化后「幂等」才名副其实。

### D5. 恢复被 GORM AutoMigrate 顺带丢掉的 NOT NULL 约束（迁移 `20261103_0002`）

- **结论**：本 change 把 gorm tag 从 `size:50` 改成 `size:100` 触发了 Phase 1 AutoMigrate 对**存量列**发 ALTER，而 gorm.io/driver/postgres v1.5.11 的 `AlterColumn` 在 `field.NotNull == false`（本仓库约定 not null 不写进 tag）时会执行 `ALTER COLUMN … DROP NOT NULL` —— `topic_tag_embeddings.model` 的 NOT NULL（由迁移 `20260723_0001` 固化）因此被静默丢弃。新增迁移 `20261103_0002`，用同文件既有的 `ensureNotNullDefault(db, "topic_tag_embeddings", "model", "''")` 恢复（幂等；生产无 NULL 行，回填为空操作）。
- **落点**：`backend-go/internal/platform/database/postgres_migrations.go`
- **取舍**：备选给 gorm tag 加 `not null`——被否：本仓库约定是「not null 不写进 tag、由迁移固化」（`20260723_0001` 的 Description 明示），加 tag 会违背约定、且会让 GORM 在每次 ALTER 时都多发一条 `SET NOT NULL`。备选并入 `20261103_0001`——被否：0001 已在生产记录过，改它的 body 不会在生产重跑，必须新版本号。其余 4 张表的 `model` 列**本来就是可空**（迁移前 `\d` 快照已确认），不动它们的空值性。

### D6. 存量失败任务走既有 `RetryFailed` 通道，不新增代码路径

- **结论**：修复落地后调用既有 `POST /api/embedding/queue/retry`（内部即 `EmbeddingQueueService.RetryFailed`：failed → pending、清空 error_message），把因 22001 失败的任务重排；不为「按错误码筛选重排」新增任何代码或 SQL。
- **落点**：`backend-go/internal/tagmanagement/service/core/embedding_queue.go:RetryFailed`
- **取舍**：备选写一条只挑 `error_message LIKE '%character varying%'` 的定向 UPDATE——被否：绕开既有服务层通道、手写 SQL 改生产队列表，且 `RetryFailed` 顺带做「标签已删 → 丢弃孤儿任务」的清理，漏掉这步会让孤儿任务在 pending/processing 间永久打转。

### D7. 不在 Go 侧对模型名做截断/兜底

- **结论**：`SaveEmbedding` 及其它写向量处保持原样写入模型名，不加截断、不加哈希、不加 fallback 值。
- **落点**：`backend-go/internal/tagmanagement/service/core/embedding.go:SaveEmbedding`
- **取舍**：备选在 Go 侧截断到 50 字符——被否：截断后的名字可能与另一个模型的短名撞车（`candidate_embeddings` 唯一键含 `model`），会让两个不同模型的向量互相覆盖；且错误会被静默吞掉，运维看不到「模型名超长」这个真实问题。

## Risks / Trade-offs

- **[ALTER 期间写阻塞——主风险在 Phase 1 AutoMigrate，不在本 change 的迁移]** → Phase 1 `RunAutoMigrate` 按新 tag 先发 ALTER，**该路径没有 lock_timeout 守卫**（会无限等 `ACCESS EXCLUSIVE`）；Phase 2 的迁移已改成条件 no-op（列宽已达标就不 ALTER），5s 的 `lock_timeout` 守卫只覆盖「确实需要加宽的窄列」这一分支。`ALTER COLUMN … TYPE varchar(50)→varchar(100)` 是同类型 typmod 变更，**不重写表体**（PG 实测 relfilenode 不变），但会**重建该列上的索引**（`idx_candidate_embedding_uniq`；3099 行，成本可忽略）。**部署动作：选 embedding worker 空闲窗口重启**；若 AutoMigrate 抢不到锁，启动会失败（`InitDB` → `Fatalf`），重拉一次即可（迁移未记录、下次启动重试）。
- **[gorm tag 变更会顺带丢掉存量列的 NOT NULL]** → GORM postgres driver 的 `AlterColumn` 在 `field.NotNull == false`（本仓库约定 tag 不写 not null）时执行 `DROP NOT NULL`——本 change 的 `size:50→100` 变更即触发了这条路径，`topic_tag_embeddings.model` 的 NOT NULL 被丢弃。已由迁移 `20261103_0002` 恢复（见 D5），并由测试断言守住。**全库级同类风险**（任何存量列的 size 变更都会踩）归属另立 change。
- **[存量数据未丢]** → 加宽不截断、不删行（PG 语义 + 生产实测：迁移前后 `topic_tag_embeddings` 行数只增不减，增量来自 worker 正常写入）；生产迁移前 `NULL`/`''` 的 `model` 行均为 0，`20261103_0002` 的回填是空操作。
- **[含 `model` 的唯一索引重建]** → `idx_candidate_embedding_uniq (candidate_id, model, dimension)` 随列宽重建；`candidate_embeddings` 实际 **3099 行**（不是 0），重建成本可忽略。`topic_tag_embeddings` 的 868 MB 是**含两个 HNSW 索引的 relation 总大小**，堆（heap）只有 10 MB——ALTER 不动堆，但 `ACCESS EXCLUSIVE` 会阻塞走该索引的检索。
- **[回滚会截断数据]** → 把列改回 `VARCHAR(50)` 会截断 61 字符的本地模型名（静默数据损坏）。故在 proposal 中明确「不建议回滚」，回滚前须先把 `ai_models.name` 改回 ≤50 字符。
- **[相邻问题不在本 change 范围：缓存键与产出模型不一致]** → embedding 缓存键取 `bindings[0].Model.Name`（主绑定），但缓存行里存的是当初产出向量的模型名；命中缓存时返回的是后者。本次只解决「存不下」，不解决「跨模型共用缓存键可能混用不同模型的向量」。后者属另一个 change 的议题。
- **[将来再加窄列]** → 由列宽不变式测试守住（数据驱动：全库所有 `model` 列宽度 ≥ `ai_models.name`，新增窄列即红），任何把 `model` 列改窄或新增窄列的提交会在测试里红。

## Migration Plan

1. 合并本 change（两个迁移 + gorm tag + 测试 + 文档）。
2. 迁移在启动时由 `RunMigrations` 执行（Phase 1 AutoMigrate 先、Phase 2 迁移后）：
   - `20261103_0001` 加宽五张表的 `model` 列（条件执行：已达标则完全不动）；
   - `20261103_0002` 恢复 `topic_tag_embeddings.model` 的 NOT NULL（Phase 1 的 ALTER 顺带丢的）。
3. 重启后端（`bash scripts/dev/start-dev.sh --restart`，**选 embedding worker 空闲窗口**）→ 校验 `information_schema` 中五张表 `model` 列宽 = 100 且 `topic_tag_embeddings.model` 仍为 NOT NULL。
4. 触发一次失败任务重排（`POST /api/embedding/queue/retry`）→ 观察 embedding 队列失败计数归零、待处理数下降。
5. 无回滚需求（加宽不可逆但也无破坏；回滚路径见 Risks）。

## Open Questions

（无）
