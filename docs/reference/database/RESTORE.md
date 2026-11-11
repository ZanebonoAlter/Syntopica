# 灾备恢复流程（RESTORE）

<!-- doc-impact-applies: scripts/db/, backend-go/internal/platform/database/ | section=恢复流程 -->
> 备份是灾备的「写」一半，**没有可验证的「读」一半，备份档只是占空间的字节**。
> 本文是 Syntopica 备份档的恢复契约与可照做流程；配套脚本 `scripts/db/restore-key-tables.sh`。
> 能力契约见 openspec `disaster-recovery`（`openspec/specs/disaster-recovery/spec.md`）。

---

## 0. 速查

| 项 | 值 |
| --- | --- |
| 备份脚本 | `scripts/db/backup-key-tables.sh`（cron 每日 04:00） |
| 恢复脚本 | `scripts/db/restore-key-tables.sh` |
| 备份档位置 | `backups/pg-key-<YYYYMMDD-HHMMSS>.dump`（保留最近 7 份） |
| 行数基线 | `backups/pg-key-<YYYYMMDD-HHMMSS>.dump.counts.tsv`（与档同生命周期） |
| 备份容器 | `syntopica-postgres`（库名 `syntopica`，PG 18.4 + pgvector 0.8.3） |
| 生产库名 | `syntopica` —— **恢复脚本硬闸保护对象** |

---

## 1. 前置条件

| 条件 | 检查方式 | 不满足时 |
| --- | --- | --- |
| 备份容器在跑 | `docker inspect -f '{{.State.Running}}' syntopica-postgres` | 先起容器；恢复全程依赖容器内的 `pg_dump/pg_restore/psql`（与 server 版本一致） |
| 备份档存在且可列目录 | `docker exec -i syntopica-postgres pg_restore -l < backups/pg-key-*.dump \| head` | 档损坏 → 换档（见 §5 换档路径） |
| 磁盘有档大小的余量 | `df -h .` | 恢复库 + 档需同时在盘上 |
| 恢复目标不是生产库 | 缺省 `--target-db` 时脚本**新建**一次性独立库 | 需要恢复进生产库时见 §4 的 `--allow-prod`（高危） |

> **红线**：生产库 `syntopica` 是真库。任何恢复演练**一律恢复到一次性新建的独立库**；
> `psql -c` 没有 dry-run，敲下去就是执行。

---

## 2. 恢复流程（照做即可）

### 步骤 1 — 选档并核对

```bash
cd /home/zanebono/software/Syntopica
ls -l backups/                       # 选最新一档
docker exec -i syntopica-postgres pg_restore -l < backups/pg-key-20261104-040001.dump | head
```

**期望结果**：`pg_restore -l` 打出 TOC 目录（约 640 条目，含 `TABLE public articles` 等）。

### 步骤 2 — 恢复到一次性独立库（默认行为）

```bash
bash scripts/db/restore-key-tables.sh \
  --dump backups/pg-key-20261104-040001.dump \
  --report /tmp/restore-report.md
```

**期望结果**（日志）：

```
[... ] 新建一次性恢复库：syntopica_restore_20261104_223057
[... ] 扩展预装完成：vector / pg_trgm
[... ] TOC 条目：原始 639 条 → 过滤后 638 条
[... ] pg_restore 完成（2100s）
[... ] 删除一次性恢复库：syntopica_restore_20261104_223057
[... ] 恢复完成（退出码 0）
```

脚本依次做四件事，**任何一步失败都非零退出**：

1. **落库闸门**：缺省新建 `syntopica_restore_<时间戳>` 一次性库；目标库名 == `syntopica` 时**直接拒绝**（退出码 2），除非 `--allow-prod` + 交互输入库名二次确认。
2. **扩展预装**：restore **之前**在目标库建 `vector` 与 `pg_trgm`；建不出来就在 restore 之前退出（退出码 3）并清掉刚建的库（不留半恢复库）。
3. **TOC 过滤 + `pg_restore`**：剔除 `SCHEMA public` 条目后 restore；失败退出码 4，目标库保留供排查。
4. **count 校验 + 迁移状态对账**（详见 §3）。

### 步骤 3 — 校验恢复结果

```bash
# a) 看报告（count 逐表比对 + 迁移对账缺口 + 耗时）
cat /tmp/restore-report.md

# b) 人工抽查若干张表行数
docker exec syntopica-postgres psql -U postgres -d syntopica_restore_20261104_223057 -c \
  "SELECT 'articles' AS t, count(*) FROM articles
   UNION ALL SELECT 'topic_tag_embeddings', count(*) FROM topic_tag_embeddings
   UNION ALL SELECT 'daily_report_sections', count(*) FROM daily_report_sections;"
```

**期望结果**：count 校验「通过」，抽查行数与报告一致。

### 步骤 4 — 在恢复库上起后端（证明「恢复出来的库能起来」）

```bash
# 保留恢复库（不删），用恢复库的 DSN 起后端
DATABASE_DSN='postgres://postgres:postgres@localhost:5432/syntopica_restore_20261104_223057?sslmode=disable' \
  go run cmd/server/main.go &
sleep 5
curl -s -o /dev/null -w '%{http_code}\n' http://localhost:5100/health   # 期望 200
```

> 恢复脚本默认在成功后**删除**目标库；要留着起后端请加 `--keep`。

---

## 3. 恢复脚本的两项校验

### 3.1 count 完整性校验

- 基线来源：**备份产物自身携带**的 `<档名>.counts.tsv`（备份时用与 dump **同一个 MVCC 快照**采集，见 §6.1）。
- 逐表比对恢复库实际行数；**任一表不一致 → 报告列出基线值与实际值，退出码 5**（恢复结果不得被当作可用）。
- L1 修复**之前**产出的历史档没有基线 → 跳过校验，报告里写明「跳过（该档无 .counts.tsv 基线）」。

### 3.2 迁移状态对账

- 脚本解析 `backend-go/internal/platform/database/postgres_migrations.go` 各迁移闭包触及的表名（SQL 串里 `TABLE <name>` 形式；`DROP TABLE` 的目标不算缺口——被迁移删掉的表本就不该在档里）。
- 与恢复库实际表集合比对，列出**「迁移已记录为应用、但其作用的表不在档内」**的缺口。
- 已应用集合的来源：恢复库的 `schema_migrations` 表（档内有它时）；历史档无该表 → 回退为「源码里的全部迁移」并在报告中标注。
- **缺口只进报告，不影响退出码**——被排除的都是可再生表（日志/缓存/队列），恢复后由 AutoMigrate 按 model 重建，是否可接受交人工判读。

当前排除清单中被迁移触及的 6 张（实测）：

| 表 | 被触及处数 |
| --- | --- |
| `ai_call_logs` | 5 |
| `ai_embedding_cache` | 3 |
| `merge_reembedding_queues` | 2 |
| `cross_board_relation_runs` | 2 |
| `embedding_queues` | 1 |
| `topic_watch_hits` | 1 |

---

## 4. 恢复到生产库（高危，默认拒绝）

生产库恢复**必须**显式声明并二次确认：

```bash
bash scripts/db/restore-key-tables.sh \
  --dump backups/pg-key-20261104-040001.dump \
  --target-db syntopica \
  --allow-prod
# 交互提示：继续请输入库名 'syntopica'（其它输入一律取消）
```

- 不带 `--allow-prod` → 退出码 2，生产库一行未动。
- 带 `--allow-prod` 但输入的库名不对 → 退出码 2，生产库一行未动。
- 确认通过 → 恢复到既有生产库（同名对象由 restore 覆盖）。

---

## 5. 失败时的回滚 / 换档路径

| 症状 | 处置 |
| --- | --- |
| 档损坏（`pg_restore -l` 失败） | **换档**：取上一档重试（保留 7 份，每日 04:00 一档）；仍失败说明连续多档损坏，查备份脚本日志 |
| restore 中途失败（退出码 4） | 目标库保留供排查；`docker exec syntopica-postgres dropdb -U postgres <目标库>` 清理后换档重试 |
| count 不一致（退出码 5） | **不要**拿这份恢复库起生产；先查差异表（报告里逐表列出基线/实际）。多为「档与基线不同源」或源库在 dump 窗口内被写入 → 换档重试 |
| 迁移对账有缺口 | 不阻断恢复；确认缺的是否都是排除清单里的可再生表。是 → 由 AutoMigrate 重建；否 → 人工处置 |
| 后端起不来（`/health` 非 200） | 看后端日志的迁移报错；多半是恢复库里缺某张被排除但被代码依赖的表 → 人工补建或换档 |
| 恢复演练做错了、怕动到生产 | 恢复脚本默认只新建独立库；误操作生产的唯一路径是 `--allow-prod` + 输入了库名 |

---

## 6. 已知失败模式与对策

### 6.1 `gin_trgm_ops does not exist`

**成因**：备份档由 `pg_dump` 产出，**不含** `CREATE EXTENSION` 语句；目标库没预装 `pg_trgm` 时，档内的 GIN trigram 索引建不出来。

**对策**：恢复脚本在 restore **之前**预装 `vector` 与 `pg_trgm`（§2 步骤 2），预装失败即退出，不进 restore 阶段。

### 6.2 迁移维度冲突（`vector` 维度不匹配）

**成因**：迁移 `20260403_0003` 曾把 `topic_tag_embeddings.embedding` 的目标类型写死为 `vector(4096)`，而现网存量列是 `vector(2560)`。历史备份档不含 `schema_migrations` → 恢复后**整条版本迁移链在有数据的库上从零重放** → ALTER 到 `vector(4096)` 时维度冲突 → 恢复出来的库起不来。

**对策**：迁移目标类型已改为**无维度限定的 `vector`**（去掉 typmod 是放宽而非收紧约束，任意维度的存量数据都能安全 ALTER）。可行性已实测，见 §7 附录。

### 6.3 `schema_migrations` 缺失

**成因**：备份脚本曾把 `schema_migrations` 放进排除清单，理由写的是「恢复时 AutoMigrate 自动重跑」——**该理由不成立**：AutoMigrate 只建表/加列（"Never drops or alters"），重放版本迁移的是 Phase 2 `runDatabaseMigrations`，而它正是在有数据的库上跑的那一步。

**对策**：`schema_migrations` 已移出排除清单，**随档走**。它不是可再生数据，而是恢复保真的必要输入。

### 6.4 迁移链重放不幂等（`current transaction is aborted`）

**成因**：PG 里**任何**一条语句报错都会中止当前事务——「捕获 `already exists` 然后继续执行下一条」在事务内不成立（下一条只会拿到 `25P02`）。另有「DROP 复合唯一约束被外键挡住」（`2BP01`）。

**对策**：相关迁移已改为 DO 块守卫（约束不存在才添加）/ 先查后加。重放全链的幂等性由
`backend-go/internal/platform/database/vector_dimension_migration_test.go`
的 `TestVectorDimensionMigrationChainReplayOnPopulatedDBIsIdempotent` 守住。

### 6.5 客户端非零退出会把整个 PG 集群拆掉（连带打断生产库连接）

**成因**（2026-11-05 实测，两次）：用 `docker exec` 在 **PG 容器里**跑客户端（`pg_restore` / `psql` / `pg_dump`）时，客户端进程是容器里 postmaster（PID 1）的子进程。客户端**非零退出**会被 postmaster 记成「unknown child exited abnormally」→ 它随即 `terminating any other active server processes` → **整个集群崩溃重启**（连带打断生产库的所有连接；恢复耗 3–5 分钟）。

**对策**（已落在脚本里）：

1. **客户端一律在宿主机跑**（连容器的发布端口 5432），退出码与 postmaster 无关；宿主机无兼容客户端时才退回容器内执行，且**必须屏蔽退出码**（让 postmaster 只看到 0）。
2. **禁用 `pg_terminate_backend`**：SIGTERM 杀 backend 会被 postmaster 当成 backend 崩溃 → 集群重启。收尾一律靠「杀客户端」（服务端 backend 看到客户端断开后正常退出）。
3. **演练在独立容器里做**：恢复演练用一次性 PG 容器（独立数据目录、独立端口），恢复过程即使把 PG 进程搞崩，也**碰不到生产库**。

> 排障线索：PG 日志里出现 `untracked child process (PID x) exited with exit code N` + `terminating any other active server processes`，就是这条路径。

### 6.6 GIN trigram 索引（`idx_articles_description_trgm`）构建很慢

**现象**：`articles.description` 全表约 232MB 文本（59152 行，均值 3.9KB，最大单值 1.68MB），恢复时重建该 GIN trigram 索引**耗时 20 分钟以上**（backend RSS 稳定在 ~180MB，不是内存问题，是 SD 卡 I/O 慢）。

**对策**：这是恢复耗时的主要构成之一（见 §7 实测耗时）；排期时把恢复窗口按 40–60 分钟算，不要按「几分钟」估。

### 6.7 迁移链重放会重写整表（`20260417_0002` 的 search_vector 回填）

**现象**：历史备份档（无迁移历史）恢复后，迁移链从零重放；`20260417_0002` 里的 `UPDATE articles SET search_vector = …` 会把 59152 行全部重写（含 ~1GB 的 GIN trigram 索引重建）——实测在 SD 卡上**超过 2 小时**，恢复窗口直接爆掉。

**对策**：该 UPDATE 已加 `WHERE search_vector IS DISTINCT FROM <同一表达式>` 守卫——存量行的 search_vector 本来就由同一个表达式算过（迁移或触发器），加守卫后**终态完全相同**，但重复执行是零写入（只付「算 tsvector」的 CPU，不付重写 + 重建 GIN 索引的 I/O）。

> 代价：tsvector 的计算仍要扫全表（CPU 主导，约 20–30 min）。彻底解决需要把 search_vector 的回填改成「只补缺失/变化行」并配合增量索引维护——属于后续 change 的范围。

---

## 7. 实测耗时量级（树莓派 4 / SD 卡 ~1.7MB/s）

| 操作 | 实测耗时 |
| --- | --- |
| `pg_dump`（1.1GB 档） | ≈ 9 min（2026-11-04 22:34→22:43 实测） |
| 行数基线采集（64 张表） | 与 dump 同一快照，一次跑完 |
| `pg_restore`（1.16GB 档 → 4.4GB 库） | ≈ 75 min（实测 4479s，含 GIN trigram 索引重建 ~25 min） |
| 恢复后端启动（含迁移链从零重放） | 含版本迁移（~2 min）+ search_vector 回填的 CPU 扫描（~25 min，见 §6.7） |
| **一次完整恢复演练（最新档）** | **≈ 80 min** |
| **一次完整恢复演练（历史档，迁移链从零重放）** | **≈ 100 min+** |

⇒ 恢复演练是**低频重资源**动作（建议按需人工执行，不要并发跑）；桌面端后续的「内置备份」时机上限要按这个量级重估。

> 耗时构成：restore 阶段是 **SD 卡写入瓶颈**（4.4GB 库 ≈ 1.4MB/s 写），不是 CPU 也不是内存。
> 另见 §6.6：`idx_articles_description_trgm` 的重建单独占 20+ 分钟。

---

## 8. 附录：`ALTER vector(2560)` → `vector` 可行性实测

> 这是 `20260403_0003` 目标类型改法的**关键假设**（design D2），未实测前不得进入实现。
> 实测日期：2026-11-04；环境：容器 `syntopica-postgres`，PG 18.4 + pgvector 0.8.3。
> **全程在一次性临时库 `dr_probe_tmp` 上进行，未触碰生产库 `syntopica`。**

```bash
# 1) 一次性临时库 + 扩展 + 小表（含非空 2560 维向量：1 条取自现网 topic_tag_embeddings
#    的真实向量 + 19 条等价合成向量；刻意用小表，避免全表重写的耗时干扰语义验证）
docker exec syntopica-postgres createdb -U postgres dr_probe_tmp
docker exec syntopica-postgres psql -U postgres -d dr_probe_tmp -c "CREATE EXTENSION IF NOT EXISTS vector;"
docker exec syntopica-postgres psql -U postgres -d dr_probe_tmp \
  -c "CREATE TABLE probe_emb (id bigserial PRIMARY KEY, embedding vector(2560));"
# 插入 20 行 2560 维向量
```

改前状态：

```
 phase |   col_type   | relfilenode
-------+--------------+-------------
 BEFORE | vector(2560) |     1686539
 BEFORE_dims | dims | rows
-------------+------+-------
            2560 |    20
```

执行 ALTER：

```sql
SET lock_timeout='5s';
ALTER TABLE probe_emb ALTER COLUMN embedding TYPE vector;
```

输出与改后状态：

```
SET
ALTER TABLE

 phase | col_type | relfilenode
-------+----------+-------------
 AFTER | vector   |     1686539      ← relfilenode 未变 ⇒ 未重写表（仅改元数据）
 AFTER_dims | dims | rows
-------------+------+-------
            2560 |    20             ← 存量数据维度不变
```

重放（列已是无限定 `vector` 时再跑一次）：

```
 REPLAY_type_before | vector
ALTER TABLE
 REPLAY_type_after  | vector | 1686539      ← 列类型不变、仍未重写表
 REPLAY_dims: 2560 | 20
```

无维度限定列接受混合维度：

```sql
INSERT INTO probe_emb (embedding) VALUES (array_fill(0.5::real, ARRAY[768])::vector);
-- MIXED_dims: 768 | 1 / 2560 | 20
```

**结论**：

1. `ALTER TABLE ... ALTER COLUMN embedding TYPE vector`（`vector(2560)` → `vector`）**成功**；
2. **存量数据维度不变**（仍为 2560）；
3. **不重写表**（`relfilenode` 不变）——「去掉 typmod」是放宽约束，PG 走 binary coercion，无需 cast 函数；
4. 在已是无限定 `vector` 的列上**重放幂等**（列类型与 relfilenode 均不变）；
5. 无维度限定的 `vector` 列可容纳不同维度的向量（768 与 2560 共存）。

⇒ **design D2 的退路未触发**，迁移目标类型按「无维度限定 `vector`」落地。

（临时库 `dr_probe_tmp` 已 `dropdb` 删除。）

---

## 9. 恢复演练留痕

演练记录见 [`RESTORE_DRILL.md`](RESTORE_DRILL.md)（演练时间、档来源、count 校验结果、迁移对账结果、总耗时）。
演练失败视为**灾备能力缺陷**，须进入修复流程并在此补记。
