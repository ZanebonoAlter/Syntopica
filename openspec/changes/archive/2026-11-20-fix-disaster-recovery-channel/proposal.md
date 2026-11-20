<!-- complexity: complex -->
<!-- ui-impact: none -->
<!-- 无业务域声明：本 change 为纯数据层/运维层（备份脚本 + 迁移闭包 + 恢复流程），不涉 flow 业务域 -->

## Why

**现网每日 04:00 的备份档，恢复出来应用起不来。** 这不是客户端形态的独有问题——服务器装法同样中招，即「每天自动产出的安全网，实际已经张不开了」。

2026-11-04 由 `add-desktop-shell-remote` 迭代 0 的 embedded PG 真负载验证（0-7）实测复现并定位，主线程只读复核确认了全部机制。三个根因叠加：

1. **迁移历史不在备份档里**：`scripts/db/backup-key-tables.sh` 的排除清单含 `schema_migrations`，理由写的是「恢复时 AutoMigrate 自动重跑」——**这个理由是错的**。AutoMigrate 只负责建表与加列（注释明确 "Never drops or alters"），重跑版本迁移的是 `RunMigrations`；迁移历史一丢，整条迁移链会在**有数据的库**上从零重放。
2. **迁移 `20260403_0003` 会把向量列改成 `vector(4096)`**（`postgres_migrations.go:296-310`），而现网实际列类型是 `vector(2560)`（主线程只读查证 `format_type`：`topic_tag_embeddings.embedding` 与 `daily_report_sections.embedding` 均为 `vector(2560)`，51139 + 5823 行实测 `vector_dims` 全为 2560）。2560 维数据去 ALTER 成 4096 → pgvector 维度不匹配 → 迁移失败 → `runDatabaseMigrations` 返回 error → **应用退出**。
3. **目标库缺扩展**：dump 含 `CREATE SCHEMA public` 但不含 `CREATE EXTENSION`，目标库未预装 `vector` / `pg_trgm` → dump 里的 GIN trigram 索引建不出来，报 `gin_trgm_ops does not exist`。

补充一条此前没人注意的事实：**`db-migration-safety` spec 早已要求「版本迁移闭包 SHALL 幂等可重复执行」，`20260403_0003` 当前是违规的**——它重复执行、或库中存在非 4096 维数据时必然失败。本 change 不是新增一套备份机制，而是**让实现追上已有 spec**，并补上从未存在过的恢复流程。

## What Changes

- **L1 根因**：`schema_migrations` 移出备份排除清单，纳入备份档。它不是「可再生数据」，而是恢复保真的必要输入。
- **L2 兜底**：迁移 `20260403_0003` 的目标类型从 `vector(4096)` 改为**无维度限定的 `vector`**——维度从此不再写进 DDL，任意维度的数据都能 ALTER 成功（不再维度不匹配），该迁移对任何存量库都是安全 no-op。同时满足 `db-migration-execution` 对 `ALTER COLUMN ... TYPE` 必须处于 `withLockTimeout` 守卫内的要求。
- **L3 流程**：新增**恢复脚本**（`scripts/db/restore-key-tables.sh`）+ **恢复文档** + **恢复演练**：
  - 恢复脚本默认恢复到**新建的一次性独立库**，并对「恢复到真库」设硬闸（目标库名等于生产库名时拒绝执行，除非显式覆盖且二次确认）；
  - restore 前在目标库预装 `vector` + `pg_trgm` 扩展；
  - restore 后做 **count 完整性校验**（关键表行数）与 **迁移状态对账**（已应用迁移触及的表是否都在档里）；
  - 恢复流程写成可照做的文档，并纳入定期演练。

**不含**：把现网存量向量列统一改成无限定 `vector`、`ensureVectorDimension` 改造、model 清理——这些属于紧随其后的 `unify-vector-column-dimension` change（维度 schema 收口，串行执行）。本 change 只保证「恢复出来的库能起来、且起来后 schema 与现网不冲突」。

## Capabilities

### New Capabilities

- `disaster-recovery`: 备份档的恢复流程契约——恢复脚本的前置闸门、扩展预装、数据完整性校验、迁移状态对账与定期演练

### Modified Capabilities

- `pg-scheduled-backup`: 「备份范围（排除法）」——`schema_migrations` 移出排除清单（原理由「恢复时 AutoMigrate 自动重跑」不成立）
- `db-migration-safety`: 「版本迁移闭包 SHALL 幂等可重复执行」——补齐向量列维度迁移的幂等与维度无关约束

## Impact

- **改动代码**：
  - `scripts/db/backup-key-tables.sh`：排除清单移除 `schema_migrations`（安全阀语义变更，备份档体积影响可忽略——该表仅数十行）。
  - `backend-go/internal/platform/database/postgres_migrations.go`：迁移 `20260403_0003` 的目标类型改为无限定 `vector`（`ALTER` 语句保留在 `withLockTimeout` 守卫内）。
- **新增**：`scripts/db/restore-key-tables.sh`（恢复脚本）、`scripts/db/restore-key-tables.smoke.sh`（脚本冒烟）、`docs/reference/database/RESTORE.md`（恢复流程文档）。
- **不改**：`front/`、`backend-go/internal/app/`、数据库 schema（无表/列增删）。
- **部署后用户可见变化**：备份档内容变化（多一张 `schema_migrations` 表，体积影响 <100KB）；**恢复出来的库现在能正常起应用**。应用日常运行行为零变化（现网 `schema_migrations` 已含 88 条，迁移照常跳过）。
- **旧数据降级**：不涉及数据迁移。已有的 7 份历史备份档不含 `schema_migrations`，恢复它们时仍会走「迁移链从零重放」路径——本 change 的 L2 保证该路径不再炸（这是 L2 独立于 L1 存在的原因）。
