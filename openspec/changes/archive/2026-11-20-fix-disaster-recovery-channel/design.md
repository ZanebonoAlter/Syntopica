## Context

- **已实测复现**：2026-11-04 的 0-7 embedded PG 真负载验证中，1GB 真档恢复后应用起不来；主线程只读复核确认全部机制（详见 proposal 的根因列表）。
- **备份语义现状**：`scripts/db/backup-key-tables.sh` 用排除法圈定范围，15 张可再生表被排除，`schema_migrations` 是其中之一，理由写的是「恢复时 AutoMigrate 自动重跑」。
- **该理由不成立**：`internal/app/static.go` 无关，但 `internal/platform/database/db.go` 的启动序列是 Phase 1 `RunAutoMigrate`（建表/加列，注释明确 "Never drops or alters"）→ Phase 2 `runDatabaseMigrations`（版本迁移）。重放版本迁移的是后者，且 `schema_migrations` 一丢就是**在有数据的库上**从零重放。
- **维度现状**：现网 `topic_tag_embeddings.embedding` 与 `daily_report_sections.embedding` 均为 `vector(2560)`（51139 + 5823 行实测 `vector_dims` 全 2560）；迁移 `20260403_0003` 的目标是 `vector(4096)`；`embedding_config` 的 `embedding_dimension` 已被迁移 `20260614_0001` 删为死配置。
- **已有 spec 约束**：`db-migration-safety` 已要求「版本迁移闭包 SHALL 幂等可重复执行」——`20260403_0003` 当前违反该约束；`db-migration-execution` 要求 `ALTER COLUMN ... TYPE` 必须处于 `withLockTimeout` 守卫内。
- **恢复耗时量级**：0-7 实测 SD 卡上 restore 2095s（34.9min）、dump 596s（9.9min），带宽约 1.7MB/s。
- **无任何恢复流程资产**：`docs/reference/database/` 全目录检索「pg_restore / 恢复」只命中一处无关内容；备份脚本只负责写侧（`pg_restore -l` 校验目录），无恢复脚本、无恢复文档、无恢复演练。

## Goals / Non-Goals

**Goals:**

- 让**恢复出来的库能正常起应用**，且恢复过程可验证（数据完整 + 迁移状态可解释）。
- 把「迁移历史」纳入备份档，消除「在有数据的库上重放 DDL」这一根本风险源。
- 让向量列维度迁移**幂等且维度无关**，使任何历史档（含不含迁移历史的 7 份存量档）都能安全恢复。
- 交付**可照做**的恢复流程（脚本 + 文档 + 演练），让灾备能力从「相信有备份」变成「验证过能恢复」。

**Non-Goals:**

- 不统一现网存量向量列的类型（→ 下一个 change `unify-vector-column-dimension`）。
- 不改 `ensureVectorDimension` 的自动纠正逻辑（→ 下一个 change）。
- 不改备份脚本的排除法形态（新增业务表自动进备份这一性质保持）。
- 不做备份档加密、异地同步、保留期策略调整。
- 不引入新的备份工具（继续用 `pg_dump -Fc`）。

## Decisions

### D1. `schema_migrations` 纳入备份档

- **结论**：从 `scripts/db/backup-key-tables.sh` 的排除清单移除 `schema_migrations`。
- **落点**：`scripts/db/backup-key-tables.sh`
- **取舍**：保留排除法整体形态（不改为全库 dump——那会把日志/缓存拖进档，体积与恢复时间爆炸，且这些表本就该丢）。取舍代价是「新增业务表自动进备份」这条性质必须保住，故排除清单仍需人工维护——这是既有设计，不在本 change 动。体积影响：该表数十行，<100KB，对 7 份保留策略无影响。

### D2. `20260403_0003` 的目标类型改为无维度限定的 `vector`

- **结论**：把该迁移的 `ADD COLUMN` 与 `ALTER COLUMN TYPE` 目标类型从 `vector(4096)` 改为无维度限定的 `vector`；`ALTER` 语句保留在 `withLockTimeout` 守卫内。
- **落点**：`backend-go/internal/platform/database/postgres_migrations.go`
- **取舍**：不删除 `ALTER` 语句。删掉也能让恢复不炸，但存量 `vector(2560)` 列将永远不会被转为无限定，且会与 `db-migration-execution` 里「`ALTER COLUMN embedding TYPE vector(N)` 须在守卫内」的约束脱节。改为无限定 `vector` 后：`vector(2560)` → `vector` 是**去掉 typmod**、放宽而非收紧约束，对任意维度的存量数据都成立——这才是幂等且维度无关的写法。副作用：列级维度校验消失（由下一个 change 在应用层用 `Dimension` 字段补回），本 change 只保证恢复可用。

### D3. 恢复脚本默认恢复到新建的一次性独立库，并对生产库设硬闸

- **结论**：`scripts/db/restore-key-tables.sh` 默认新建独立恢复库；目标库名与生产库名相同时拒绝执行，除非显式覆盖开关 + 交互二次确认。
- **落点**：任务 3.2
- **取舍**：不提供「默认恢复到某个指定库」的行为。恢复是最容易造成不可逆后果的操作，默认值必须是安全的（新建库）。需要恢复进指定库的运维场景走显式参数 + 二次确认。

### D4. 扩展预装在 `pg_restore` 之前

- **结论**：恢复脚本在 restore 前于目标库创建 `vector` 与 `pg_trgm`；创建失败则退出，不进入 restore 阶段。
- **落点**：任务 3.3
- **取舍**：不修改备份脚本的 dump 产出方式去内嵌 `CREATE EXTENSION`（dump 是 `pg_dump -Fc` 产物，控制其内容超出本 change 范围，且会让档绑定特定扩展版本）。预装是恢复侧的责任。

### D5. TOC 过滤掉 `SCHEMA public`

- **结论**：restore 时过滤 dump TOC 中的 `SCHEMA public` 条目。
- **落点**：任务 3.4
- **取舍**：不改备份侧。`pg_dump` 产出的 `CREATE SCHEMA public` 在恢复到已含该 schema 的库时会冲突；过滤是恢复侧的标准做法。

### D6. count 校验基线由备份产物自身携带

- **结论**：备份脚本在产出 dump 的同时，于同目录写一份同时间戳的行数基线清单（各表行数）；恢复脚本用它逐表比对恢复结果。
- **落点**：任务 1.3
- **取舍**：不在恢复脚本里硬编码各表行数（必然随业务漂移而失真）。也不从源库现查（恢复场景源库可能已损毁）。基线与档同生命周期（轮转时一并删除）。代价：备份脚本产物从 1 个变 2 个，轮转逻辑要同步处理（任务 1.4）。

### D7. 迁移状态对账在恢复时动态解析迁移源码

- **结论**：恢复脚本解析 `backend-go/internal/platform/database/postgres_migrations.go` 中各迁移闭包触及的表名，与恢复库实际存在的表集合比对，列出「迁移已应用但表不在档内」的缺口；缺口不阻断退出码，只进报告。
- **落点**：任务 3.6
- **取舍**：不维护「迁移 → 表」的手写映射表（必然漂移）。动态解析源码是唯一不会漂移的来源，代价是解析规则要跟着迁移写法变化（表名大多以 `TABLE <name>` 形式出现在 SQL 串里，解析面可控）。缺口不阻断的理由：这些表都是可再生表，恢复后由 AutoMigrate 按 model 重建，是否可接受交人工判读。

### D8. 存量 7 份历史备份档仍走「迁移链从零重放」，由 D2 保证不炸

- **结论**：不追溯修补历史备份档；恢复它们时迁移链会从零重放，靠 `20260403_0003` 的无限定目标类型保证不再因维度冲突失败。
- **落点**：`backend-go/internal/platform/database/postgres_migrations.go`
- **取舍**：不做「历史档补 schema_migrations」的补救（无源可补——档里没有该表的历史行）。L2 独立于 L1 存在正是为此：即使备份档是 L1 之前产出的，恢复也不炸。

### D9. 存量向量列统一与 `ensureVectorDimension` 改造留给下一个 change

- **结论**：本 change 不统一现网 7 张 `vector(2560)` 表的列类型，不改 `ensureVectorDimension` 的自动 ALTER 行为。
- **落点**：无代码落点：下一个 change `unify-vector-column-dimension`，因为存量列统一会触发 51139 + 5823 行的全表重写（约 512MB），必须独立评估耗时、锁窗口与回滚策略，不与恢复通道修复捆绑
- **取舍**：接受「本 change 落地后，存量列仍是 `vector(2560)`」这一中间态。该中间态不影响恢复可用性（维度不匹配是 D2 修掉的），只影响长期收敛。

### D10. 恢复演练作为常规动作并留痕

- **结论**：用真实备份档做定期恢复演练，记录演练时间、档来源、校验结果与耗时；演练失败视为灾备能力缺陷。
- **落点**：任务 5.3
- **取舍**：不自动化到 cron（一次演练 35min+，且需要独占资源与人工判读缺口）。先做「有据可查的人工定期演练」，自动化留待稳定后。

## Risks / Trade-offs

- **⚠️ `ALTER vector(2560)` → `vector` 的可行性未经实测**：Postgres 对同 base type 去 typmod 通常走 binary coercion（不需 cast 函数），但 pgvector 是否支持必须实测——这是 D2 的关键假设。**任务 3.2 专项验证**。若不支持，退路：把 `ALTER` 段改为「列已是 `vector` 则跳过；否则跳过并记警告」，恢复可用性不受影响（不再维度冲突），但存量列统一顺延到下一个 change。
- **`schema_migrations` 进档后的「迁移已应用、表不在档内」缺口**：实测有 6 张被排除表被迁移链触及（`ai_call_logs` 5 处、`ai_embedding_cache` 3 处、`merge_reembedding_queues` 2 处、`cross_board_relation_runs` 2 处、`embedding_queues` 1 处、`topic_watch_hits` 1 处）。缓解：D7 对账检出并报告；这些表恢复后由 AutoMigrate 重建。
- **恢复脚本误伤生产库**：D3 硬闸 + 默认新建独立库 + 生产库名拒绝执行。
- **备份脚本轮转副作用**：跑一次备份会轮转删除最老的一档。验证时须先记录 `backups/` 现有档数，跑完确认总数仍 ≤7，避免在验证过程中丢掉一档真档。
- **恢复耗时**：实测 34.9min（SD 卡 1.7MB/s）。文档须写明量级并安排演练窗口；桌面端后续的 FR-G4 内置备份时机上限也要按这个量级重估（属于客户端 change，本 change 只留事实）。
- **备份产物从 1 个变 2 个**（dump + count 基线）：轮转与清理逻辑需同步，避免基线文件成为孤儿垃圾（任务 1.4）。
