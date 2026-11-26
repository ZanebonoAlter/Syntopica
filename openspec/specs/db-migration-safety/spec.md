<!-- domain: db -->

# Purpose

定义 Syntopica 数据库迁移层的安全性行为约束：破坏性迁移（`TRUNCATE`/`DROP`）须由显式配置开关控制，生产默认拒绝；版本迁移闭包须幂等可重复执行。这些约束防止生产环境误执行数据销毁迁移，并保证迁移在人工补跑或重试时不因非幂等 DDL 报错。

## Requirements

### Requirement: 破坏性迁移须由显式配置开关控制

系统 SHALL 通过配置开关 `AllowDestructiveMigrations`（env `MIGRATIONS_ALLOW_DESTRUCTIVE=1` 开启）控制破坏性数据库迁移（含 `TRUNCATE`/`DROP TABLE`/`DROP COLUMN` 等数据销毁操作）是否执行。该开关默认关闭（false）。当开关关闭时，标记为破坏性的迁移 SHALL 被跳过（`Up` 闭包内 `return nil`，不报错、不中止启动）并记录 `WARN` 日志说明跳过原因与开启方式；该迁移版本 SHALL 仍被记入 `schema_migrations` 表为已应用，避免每次启动重复告警。

#### Scenario: 生产环境默认拒绝破坏性迁移

- **WHEN** 未设置 `MIGRATIONS_ALLOW_DESTRUCTIVE`（或值不为 `1`）启动应用
- **AND** 迁移队列中存在标记为破坏性的迁移（如 `20260706_0001` TRUNCATE `topic_lifeline_context`）
- **THEN** 该破坏性迁移的 `Up` 闭包不执行 TRUNCATE
- **AND** 记录 `WARN` 日志：说明因未开启 `MIGRATIONS_ALLOW_DESTRUCTIVE` 跳过该迁移
- **AND** `schema_migrations` 表写入该迁移版本，下次启动不再重复告警
- **AND** 被破坏性迁移清理的目标表数据保留不变

#### Scenario: 显式开启后破坏性迁移正常执行

- **WHEN** 设置 `MIGRATIONS_ALLOW_DESTRUCTIVE=1` 启动应用
- **THEN** 标记为破坏性的迁移 `Up` 闭包照常执行（含 TRUNCATE）
- **AND** dev/测试环境的历史数据清理迁移（`20260706_0001`/`20260712_0001`/`20260718_0001`）按既有逻辑执行

#### Scenario: 配置层暴露 env 覆盖入口

- **WHEN** `config.go` 的 `applyEnvOverrides` 读取环境变量
- **THEN** `MIGRATIONS_ALLOW_DESTRUCTIVE` 值为 `"1"` 时 `DatabaseConfig.AllowDestructiveMigrations` 置为 `true`，其余值（含未设置）置为 `false`
- **AND** 该覆盖遵循项目既有 5 个 env override 范式（`SERVER_PORT`/`SERVER_MODE`/`DATABASE_DRIVER`/`DATABASE_DSN`/`CORS_ORIGINS`）

### Requirement: 版本迁移闭包 SHALL 幂等可重复执行

每条版本迁移的 `Up` 闭包 SHALL 可重复执行而不报错（幂等），即使对应 `schema_migrations` 版本已应用、人工补跑亦不应失败。涉及 `SET NOT NULL` 等非幂等 DDL 时，SHALL 复用 `ensureNotNullDefault` 抽象（先判 `columnIsNullable`、回填、再 `SET NOT NULL`）或 `DO $$ ... IF ... END $$` 守卫。

**向量列维度迁移的附加约束**：涉及向量列类型（`vector` / `halfvec`）的迁移 SHALL NOT 把维度硬编码进目标类型，目标类型 SHALL 为**无维度限定的 `vector`**。维度由运行时 embedding 模型的实际输出决定（该维度已在 `Dimension` 字段逐行留存），不属于 schema 契约。这样任何维度的存量数据都能被该迁移安全 ALTER，迁移不会因「库中维度 ≠ 硬编码维度」而失败。

迁移内约束 helper SHALL 如实尊重其参数声明：`constrain(table, column, defaultLit, notNull)` 在 `notNull=false` 时 SHALL 仅 SET DEFAULT，不得因 defaultLit 非空而绕过 notNull 参数执行 SET NOT NULL（20260723_0001 的架空 bug——context_layers/aliases 两列被意外 NOT NULL）。

#### Scenario: SET NOT NULL 迁移重复执行不报错

- **WHEN** `ai_call_logs.operation` 列已是 NOT NULL 时重复执行 `20260704_0001` 的 SET NOT NULL 逻辑
- **THEN** 不报错（幂等返回成功），而非因 `ALTER COLUMN ... SET NOT NULL` 对已 NOT NULL 列重复操作而失败

#### Scenario: 重复执行 InitDB 不产生迁移错误

- **WHEN** 同一 testcontainer 数据库连续执行两次 `InitDB`（AutoMigrate + 全量版本迁移）
- **THEN** 第二次执行所有迁移闭包均成功（不因非幂等 DDL 报错）

#### Scenario: constrain helper 尊重 notNull=false 声明

- **WHEN** 空库重放 `20260723_0001` 且 cols 表声明某列 `notNull=false`（如 semantic_labels.context_layers / aliases）
- **THEN** 该列仅被 SET DEFAULT + 回填，不被 SET NOT NULL——列保持 nullable，与 cols 声明一致

#### Scenario: 向量列维度迁移不因存量维度不同而失败

- **WHEN** 在一个向量列已含非空数据、且其维度与迁移硬编码目标维度不同的库上重放向量列类型迁移
- **THEN** 迁移 SHALL 成功（不因 pgvector 维度不匹配而报错）
- **AND** 迁移后的列类型 SHALL 为无维度限定的 `vector`，存量数据维度保持不变

#### Scenario: 向量列类型迁移重复执行不改变列类型

- **WHEN** 向量列类型迁移在列已是无维度限定 `vector` 的库上重放
- **THEN** 迁移 SHALL 成功且列类型保持 `vector`（不产生额外 DDL 效果）
