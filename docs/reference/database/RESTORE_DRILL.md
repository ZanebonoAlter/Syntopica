# 灾备恢复演练记录（RESTORE_DRILL）

> 配套流程：[`RESTORE.md`](RESTORE.md)。恢复能力靠**演练**验证，不靠「相信有备份」。
> 演练失败视为**灾备能力缺陷**，必须进入修复流程并在此补记。
> 本页记录每次演练的时间、档来源、校验结果、迁移对账结果与总耗时。

---

## 演练环境约定（2026-11-05 起）

| 项 | 值 |
| --- | --- |
| 演练目标 | **一次性独立 PG 容器** `drill-pg`（`pgvector/pgvector:pg18-trixie`，独立数据目录，宿主端口 55433） |
| 为什么不用生产容器 | 恢复过程会重建 GIN trigram 索引等重操作，**一旦把 PG 进程搞崩，生产库会连带中断**（2026-11-05 实测两次，见 `RESTORE.md` §6.5）。演练在独立容器里做 ⇒ 演练失败也碰不到生产库 |
| 恢复脚本 | `scripts/db/restore-key-tables.sh`（客户端走**宿主机** PG 客户端，不进容器 PID 命名空间） |
| 档来源 | `backups/pg-key-<timestamp>.dump`（每日 04:00 cron 产出，保留最近 7 份） |

---

## 2026-11-05 演练 ①：最新档（L1 之后产出，含迁移历史 + 行数基线）

### 基本信息

| 项 | 值 |
| --- | --- |
| 演练时间 | 2026-11-05 00:38 → 01:53 |
| 档来源 | `backups/pg-key-20261104-223430.dump`（1,159,584,434 B / 1.1G，产出于 2026-11-04 22:43） |
| 行数基线 | `backups/pg-key-20261104-223430.dump.counts.tsv`（64 张表） |
| 目标库 | 容器 `drill-pg` → 库 `syntopica_drill_new`（新建，一次性） |
| 恢复耗时 | ≈ 53 min（00:38:13 → ≈01:31；其中 `idx_articles_description_trgm` 的 GIN trigram 索引重建 ≈ 25 min） |
| 后端验证 | 2026-11-05 01:50 起后端（`SERVER_PORT=5199`，DSN 指向 `127.0.0.1:55433/syntopica_drill_new`） |

### 结果

| 校验项 | 结果 |
| --- | --- |
| count 完整性校验 | ✅ **64 张表全部一致，0 差异**（逐表：基线行数 == 恢复库实际行数） |
| 迁移状态对账 | ✅ 恢复库带 `schema_migrations`（88 行），「迁移已应用但表不在档内」的缺口 = 被排除清单里的可再生表（`ai_call_logs` / `ai_embedding_cache` / `embedding_queues` / `merge_reembedding_queues` / `cross_board_relation_runs` / `topic_watch_hits`），由 AutoMigrate 重建 |
| 后端启动 | ✅ `Database initialized successfully`（AutoMigrate + 全量版本迁移在恢复库上跑通） |
| `/health` | ✅ **HTTP 200** `{"database":"connected","status":"healthy"}` |

### 过程中的两个事故（均已修复，记录留痕）

1. **恢复脚本在 restore 成功后被「编辑正在运行的脚本」打断**：bash 会按文件偏移增量读取脚本，运行中改文件 → 后续命令读到了错位的内容 → 脚本以 `command not found` 退出（restore 本身由孤儿化的 `pg_restore` 客户端完成，数据完整）。count 校验与报告由人工按脚本同款 SQL 复核（0 差异）。**教训：不要在脚本运行期间编辑它。**
2. **PG 集群被客户端非零退出连带拆掉**（详见 `RESTORE.md` §6.5）：恢复脚本最初用 `docker exec` 在 PG 容器内跑 `pg_restore`/`psql`，客户端非零退出被容器里的 postmaster 当成「unknown child exited abnormally」→ 整个集群崩溃重启（**生产库连接被连带打断**）。已改为宿主机客户端 + 容器内执行时屏蔽退出码；演练目标也改为独立容器。

---

## 2026-11-05 演练 ②：历史档（L1 之前产出，不含迁移历史）

### 基本信息

| 项 | 值 |
| --- | --- |
| 演练时间 | 2026-11-05 01:52 → （见下） |
| 档来源 | `backups/pg-key-20261104-040001.dump`（1,069,609,331 B / 1.0G，产出于 2026-11-04 04:06，**L1 修复之前**） |
| 档内 `schema_migrations` | **无**（`pg_restore -l` 里 0 条 TABLE public schema_migrations） |
| 行数基线 | **无**（L1 之前没有 `.counts.tsv`）→ count 校验跳过并在报告中标注 |
| 目标库 | 容器 `drill-pg` → 库 `syntopica_drill_hist`（新建，一次性） |
| 验证重点 | 恢复后**迁移链从零重放**（有数据的库上重放 DDL）不再因维度冲突失败 |

### 结果

| 校验项 | 结果 |
| --- | --- |
| 恢复本身 | ✅ **退出码 0**：历史档完整恢复到一次性独立库（pg_restore 4479s / 74.7 min，报告已产出） |
| count 完整性校验 | ⏭️ 跳过（该档是 L1 之前产出的，**无 `.counts.tsv` 基线**——脚本按设计跳过并在报告里标注） |
| 迁移状态对账 | ✅ 恢复库**无** `schema_migrations` → 脚本按设计回退为「源码中的全部迁移」（110 个），并列出「迁移已触及但表不在档内」的缺口（被排除清单里的可再生表） |
| **迁移链从零重放** | ✅ **过关键点**：恢复库里的 `topic_tag_embeddings.embedding` 是 `vector(2560)`（存量 53248 行），迁移 `20260403_0003` 的 `ALTER TABLE topic_tag_embeddings ALTER COLUMN embedding TYPE vector` **成功执行**（耗时 131s）——**这正是 L1/L2 修复前会维度冲突失败的那一步**。链条继续往下跑，未因维度冲突中断 |
| 后端启动 → `/health` | ⏳ **未完成**：链条在 `20260417_0002` 的 `UPDATE articles SET search_vector = …` 上耗时过长（见「遗留」），演练记录里如实标注 |

### 迁移链重放的实测耗时（历史档特有）

| 阶段 | 实测 |
| --- | --- |
| `20260403_0003` 向量列类型迁移（`vector(2560)` → `vector`） | ✅ 131s（**无维度冲突**；目标类型无维度限定，去 typmod 不重写表） |
| `20260417_0002` search_vector 回填（59152 行 / 232MB 文本） | ⏳ 首次跑（无守卫）：>2h（重写全表 + 重建 ~1GB GIN 索引）→ 已加 `IS DISTINCT FROM` 守卫（终态相同、零写入）；加守卫后仍需扫全表算 tsvector（CPU 主导） |

> 结论：**灾备恢复的主要成本不在「恢复」而在「迁移链重放」**。恢复出来的库能起来（最新档已验证 /health 200），历史档的链条也能重放（关键维度冲突已消除），但重放要按**小时级**排期。

### 遗留（需要下一个 change 跟进）

1. `20260417_0002` 的 search_vector 回填仍是全表扫描（守卫去掉了写入，没去掉扫描）——彻底解决需要「只补缺失/变化行」+ 增量索引维护。
2. 本次演练的 `/health` 200 未取到（链条未跑完）——链条跑完后应补测一次并回填本表。

---

### 生产库未被触碰核对（2026-11-05）

演练全程生产库 `syntopica`（容器 `syntopica-postgres`，端口 5432）**没有被写入**：

| 证据 | 结果 |
| --- | --- |
| 演练目标 | 独立容器 `drill-pg`（端口 55433）+ 独立数据目录 ⇒ 恢复写入**结构上不可能**落到生产库 |
| 生产库里的演练残留 | 0（`information_schema.tables` 里无 drill/smoke/restore 字样的表；生产容器里只剩 `postgres` / `syntopica` / `syntopica_e1`（演练前就有）/ template*） |
| 静态表行数（前 → 后） | `schema_migrations` 88→88、`feeds` 28→28、`categories` 5→5、`ai_settings` 46→46、`embedding_config` 2→2、`discovery_interest_entries` 10→10、`preference_vectors` 10→10 **完全一致** |
| 变化表的行数（前 → 后） | `articles` 59152→59446、`semantic_labels` 204910→205205、`ai_call_logs` 150110→154155、`topic_tags` 21979→19960、`topic_tag_embeddings` 53457→48993、`article_topic_tags` 28456→25474 等——**全部来自应用自身的正常运行**（04:00 日报管线 + 标签合并清理任务），与演练无关：演练全程没有任何一次连接指向生产库写数据 |
| 落库硬闸自证 | `scripts/db/restore-key-tables.smoke.sh` 断言「指定生产库名 → 非零退出且生产库行数不变」「二次确认不通过 → 非零退出且生产库行数不变」「演练全程源库行数不变」全部通过 |

> 注：生产库行数**不是**逐字节不变——因为应用一直在跑（抓取/日报/标签清理）。能证明的是「演练没有写入生产库」：静态表与 schema 完全一致、演练目标在独立容器、恢复脚本对生产库名有硬闸。

---

## 演练失败记录（归档留痕）

| 日期 | 现象 | 根因 | 处置 |
| --- | --- | --- | --- |
| 2026-11-05 | 恢复过程中 PG 集群崩溃重启两次，生产库连接被连带打断 | 恢复脚本用 `docker exec` 在 PG 容器内跑客户端，客户端非零退出被 postmaster 当成致命 → 集群拆掉 | 客户端改走宿主机；容器内执行时屏蔽退出码；演练改在独立容器做（`RESTORE.md` §6.5） |
| 2026-11-05 | `pg_restore` 在 `CREATE INDEX idx_articles_description_trgm` 处失败（`server closed the connection unexpectedly`） | 该 GIN trigram 索引覆盖 232MB 文本（最大单值 1.68MB），在 8GB 树莓派上构建极慢（>20 min）且不稳定 | 恢复耗时按 40–60 min 估（`RESTORE.md` §7）；演练改在独立容器做，生产库不再被牵连 |
| 2026-11-05 | 04:00 cron 备份失败（`ERROR: snapshot "..." does not exist`） | 演练期间为让盘 I/O 给演练，SIGSTOP 了备份的快照持有者进程 → 快照事务提前结束 → 基线采集失败 | 脚本行为**正确**（失败即非零退出、不留残档、不轮转，旧档 7 份完好）；已手工重跑补出当天的档 |
| 2026-11-05 | 恢复脚本在 restore 成功后被「编辑正在运行的脚本」打断 | bash 按文件偏移增量读取脚本，运行中改文件 → 读到错位内容 → 脚本 `command not found` 退出 | 教训：不要在脚本运行期间编辑它（restore 本身由孤儿化客户端完成，数据完整） |
