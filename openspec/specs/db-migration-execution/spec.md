<!-- domain: db -->

# db-migration-execution Specification

## Purpose
TBD - created by archiving change db-ddl-hardening-architecture. Update Purpose after archive.
## Requirements
### Requirement: 迁移可声明脱离外层事务执行（事务不兼容 DDL 解锁）

`Migration` 结构体 SHALL 提供 `RunOutsideTx bool` 字段（零值 false）。默认（`RunOutsideTx == false`）时，迁移 `Up` 闭包 SHALL 在外层事务内执行（`db.Transaction(Up(tx) + INSERT schema_migrations)`），与历史行为完全一致。当迁移声明 `RunOutsideTx: true` 时，执行器 SHALL 在**无外层事务**的连接上调用 `Up(db)`，为 PostgreSQL 事务不兼容 DDL（如 `CREATE INDEX CONCURRENTLY`）解锁。事务外迁移的 version 记录语义 SHALL 与事务路径对齐：**`Up` 失败时不记入 `schema_migrations`（下次启动可重试），`Up` 成功后才单独 `INSERT schema_migrations`**。该字段 SHALL 仅用于单条事务不兼容 DDL；需要原子性的多步操作 SHALL 留在事务内（默认路径）。

#### Scenario: 事务外迁移可执行 CONCURRENTLY 等 PG 事务不兼容 DDL

- **WHEN** 一条迁移声明 `RunOutsideTx: true` 且其 `Up` 闭包含 `CREATE INDEX CONCURRENTLY`
- **THEN** 执行器在无外层事务的连接上调用 `Up`
- **AND** `CREATE INDEX CONCURRENTLY` 成功执行（不报 SQLSTATE 25001「在事务块内运行」错误）

#### Scenario: 事务外迁移 Up 失败不记录 version（保证可重试）

- **WHEN** 一条 `RunOutsideTx: true` 的迁移 `Up` 返回 error
- **THEN** 该迁移版本**不**记入 `schema_migrations` 表
- **AND** 下次启动该迁移会被重新执行（可重试）

#### Scenario: 事务外迁移 Up 成功才记录 version

- **WHEN** 一条 `RunOutsideTx: true` 的迁移 `Up` 执行成功
- **THEN** 执行器单独 `INSERT INTO schema_migrations` 记录该版本
- **AND** 下次启动不再重复执行该迁移

#### Scenario: 默认路径保持事务内执行不变（零值兼容）

- **WHEN** 迁移未声明 `RunOutsideTx`（或为 false）
- **THEN** `Up` 闭包在 `db.Transaction` 内执行，成功后同事务内 `INSERT schema_migrations`
- **AND** 现有全部迁移的执行行为与引入该字段之前完全一致

### Requirement: 迁移可声明回滚能力并标注不可逆性

`Migration` 结构体 SHALL 提供 `Down func(db *gorm.DB) error` 字段（nil = 不可逆）。破坏性迁移（含 `TRUNCATE`/`DROP` 等数据销毁操作）SHALL 在其 `Description` 中显式标注不可逆性（如「⚠️ 不可逆，生产执行前需备份」）。执行器当前 SHALL 不实现 down 回滚执行路径——`Down` 字段为声明性占位，供未来按需补 `RunDownMigrations`；物理不可恢复的操作（TRUNCATE）SHALL 标注不可逆而非伪造「恢复 DDL」。

#### Scenario: 不可逆迁移 Down 为 nil

- **WHEN** 一条迁移不提供 `Down` 闭包（nil）
- **THEN** 该迁移被声明为不可逆
- **AND** 执行器不为其提供回滚路径

#### Scenario: 破坏性迁移 Description 标注不可逆性

- **WHEN** 迁移 `Up` 含 `TRUNCATE`/`DROP` 等数据销毁操作（如 `20260706_0001`/`20260712_0001`/`20260718_0001`）
- **THEN** 其 `Description` 字段末尾标注「⚠️ 不可逆」类警示
- **AND** 提示生产执行前需备份

### Requirement: 长锁 DDL 须在 lock_timeout 守卫内执行

涉及 `ALTER COLUMN ... TYPE`（全表重写）、`ADD CONSTRAINT ... UNIQUE`（全表扫描）或 `ADD CONSTRAINT ... FOREIGN KEY`（全表行校验）等长 `AccessExclusiveLock` 的迁移 DDL SHALL 通过 `withLockTimeout(db, timeout, fn)` 守卫执行。该 helper SHALL 在事务内 `SET LOCAL lock_timeout = <timeout>` 后执行 `fn`，并在 `fn` 返回后 `SET LOCAL lock_timeout = DEFAULT` 显式复位。当锁竞争使语句在 `timeout`（默认 5s）内无法获取锁时，该语句 SHALL 失败（而非无限阻塞），以保证大表写入不被无限期锁住。

#### Scenario: withLockTimeout 在事务内设置 GUC 并于执行后复位

- **WHEN** 在事务内调用 `withLockTimeout(db, "3s", fn)`
- **THEN** `fn` 执行期间 `SHOW lock_timeout` 返回 `3s`（GUC 生效）
- **AND** `fn` 返回后同事务内 `SHOW lock_timeout` 回到 `DEFAULT`（显式复位）

#### Scenario: 长锁 DDL 守卫点覆盖四类高危操作

- **WHEN** 迁移含 `ALTER COLUMN embedding TYPE vector(N)`、`ADD CONSTRAINT ... UNIQUE`、扩展既有 `UNIQUE` 约束、或 `ADD CONSTRAINT ... FOREIGN KEY`
- **THEN** 这些语句各自被 `withLockTimeout(db, "5s", ...)` 守卫包裹
- **AND** 守卫在成功路径上不影响迁移结果（仅在锁竞争超 5s 时让该语句失败而非无限阻塞）

#### Scenario: ADD CONSTRAINT FOREIGN KEY 迁移须在 lock_timeout 守卫内执行

- **WHEN** 一条版本迁移含 `ALTER TABLE topic_watch_hits ADD CONSTRAINT fk_topic_watch_hits_watch FOREIGN KEY (watch_id) REFERENCES board_topic_watches(id) ON DELETE CASCADE`（如 `20260801_0002`）
- **THEN** 该 ADD CONSTRAINT 语句被 `withLockTimeout(db, "5s", fn)` 包裹
- **AND** FK 校验扫描期间持有 `AccessExclusiveLock`，锁竞争超 5s 时语句失败而非无限阻塞

