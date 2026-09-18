# pg-scheduled-backup Specification

## Purpose
为 Syntopica 提供无人值守的 PostgreSQL 关键业务表定时备份：每日自动产出一致性好、体积可控的备份档，日志/缓存/队列等可再生数据不占备份空间，旧档自动轮转，失败可从日志与退出码察觉。恢复能力是磁盘故障/误操作后业务数据（articles、标签体系、日报、AI 配置等）的唯一安全网。

## Requirements

### Requirement: 备份范围（排除法）

备份脚本 SHALL 使用 pg_dump 排除法圈定备份范围：显式排除下列可再生表，其余 public schema 下全部表自动纳入备份——

- 日志/追踪：`ai_call_logs`、`otel_spans`
- 缓存：`ai_embedding_cache`
- 任务队列：`firecrawl_jobs`、`tag_jobs`、`embedding_queues`、`merge_reembedding_queues`
- 通知：`notifications`
- 运行记录：`discovery_runs`、`discovery_run_items`、`cross_board_relation_runs`、`topic_watch_hits`
- 运行时状态：`scheduler_tasks`（调度执行状态，非用户配置）
- 行为明细：`reading_behaviors`
- 迁移记录：`schema_migrations`（恢复时 AutoMigrate 自动重跑）

新增业务表 MUST 默认被备份（无需改脚本）；新增日志/缓存类表时由维护者手动补入排除清单。

#### Scenario: 排除清单内的表不进备份档

- **WHEN** 执行备份脚本且数据库可连
- **THEN** 产出的 dump 中不含排除清单内任何表（`pg_restore -l` 列表中无上述表名）
- **AND** 含全部关键业务表（至少：`articles`、`feeds`、`categories`、`topic_tags`、`semantic_labels`、`topic_tag_embeddings`、`daily_report_threads`、`daily_report_sections`、`ai_providers`、`ai_settings`、`discovery_interest_entries`、`preference_vectors`）

#### Scenario: 新建业务表自动被备份

- **WHEN** 数据库中新建一张不在排除清单内的表（如未来新增的业务表）后执行备份
- **THEN** 该表进入备份档，无需修改脚本

### Requirement: 备份产物格式与命名

备份脚本 SHALL 以 PostgreSQL 自定义格式（`pg_dump -Fc`）产出单个备份档，命名 `pg-key-<yyyyMMdd-HHmmss>.dump`，输出目录为仓库 `backups/`。备份 SHALL 通过容器内 `pg_dump` 执行（版本与 server 一致），并采用与库连接可用性前置校验——容器未运行或 `pg_isready` 不通过时 MUST 报错退出（非零退出码）且不产出任何备份档。

#### Scenario: 正常备份产出

- **WHEN** 容器 `syntopica-postgres` 运行中且执行备份脚本
- **THEN** `backups/` 下新增一个 `pg-key-<时间戳>.dump` 文件，`pg_restore -l` 可列出目录且大小大于 0

#### Scenario: 容器不可用时失败而非空档

- **WHEN** 容器 `syntopica-postgres` 未运行时执行备份脚本
- **THEN** 脚本以非零退出码结束并输出错误信息，`backups/` 不新增文件

### Requirement: 保留策略（轮转）

每次备份成功后，脚本 SHALL 清理旧备份档，仅保留最近 7 份 `pg-key-*.dump`（按时间新到旧），更旧的自动删除；备份失败时 SHALL NOT 触发轮转删除。

#### Scenario: 超出保留数的旧档被清理

- **WHEN** `backups/` 已有 7 份备份且本次备份成功
- **THEN** 清理后 `backups/` 中 `pg-key-*.dump` 恰为 7 份，且包含本次新档

#### Scenario: 备份失败不删旧档

- **WHEN** 本次 dump 失败（非零退出）且 `backups/` 已有 7 份旧档
- **THEN** 旧档全部保留，数量不变

### Requirement: 定时执行

宿主机 crontab SHALL 包含一条每日 04:00（Asia/Shanghai）执行备份脚本的条目，stdout/stderr 追加写入 `logs/db-backup.log`；同一时刻至多一个备份进程（重入保护）。

#### Scenario: crontab 条目存在且格式正确

- **WHEN** 执行 `crontab -l`
- **THEN** 存在一行 `0 4 * * *` 触发 `scripts/db/backup-key-tables.sh` 且输出重定向到 `logs/db-backup.log`

### Requirement: 失败可观测

脚本 SHALL 以非零退出码报告失败，并把关键步骤（开始、dump 结果、轮转结果、结束/失败原因）写入 `logs/db-backup.log`，使无人值守失败可从日志与退出码察觉；成功时日志包含本次档名与大小。

#### Scenario: 失败写入日志并可察觉

- **WHEN** 备份过程中任一步骤失败（连接失败/dump 失败/轮转异常）
- **THEN** `logs/db-backup.log` 追加了含时间戳的失败原因行，脚本退出码非零
