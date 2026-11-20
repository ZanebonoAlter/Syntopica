## MODIFIED Requirements

### Requirement: 备份范围（排除法）

备份脚本 SHALL 使用 pg_dump 排除法圈定备份范围：显式排除下列可再生表，其余 public schema 下全部表自动纳入备份——

- 日志/追踪：`ai_call_logs`、`otel_spans`
- 缓存：`ai_embedding_cache`
- 任务队列：`firecrawl_jobs`、`tag_jobs`、`embedding_queues`、`merge_reembedding_queues`
- 通知：`notifications`
- 运行记录：`discovery_runs`、`discovery_run_items`、`cross_board_relation_runs`、`topic_watch_hits`
- 运行时状态：`scheduler_tasks`（调度执行状态，非用户配置）
- 行为明细：`reading_behaviors`

`schema_migrations`（迁移历史）SHALL 纳入备份档：它不是可再生数据，而是恢复保真的必要输入——缺失它会使恢复后的库从零重放整条版本迁移链，在有数据的库上重放 DDL 存在维度/约束冲突风险（见 `disaster-recovery`）。

新增业务表 MUST 默认被备份（无需改脚本）；新增日志/缓存类表时由维护者手动补入排除清单。

#### Scenario: 排除清单内的表不进备份档

- **WHEN** 执行备份脚本且数据库可连
- **THEN** 产出的 dump 中不含排除清单内任何表（`pg_restore -l` 列表中无上述表名）
- **AND** 含全部关键业务表（至少：`articles`、`feeds`、`categories`、`topic_tags`、`semantic_labels`、`topic_tag_embeddings`、`daily_report_threads`、`daily_report_sections`、`ai_providers`、`ai_settings`、`discovery_interest_entries`、`preference_vectors`）

#### Scenario: 迁移历史进入备份档

- **WHEN** 执行备份脚本且数据库可连
- **THEN** 产出的 dump 中**包含** `schema_migrations` 表及其全部已应用版本行
- **AND** 该表体积相对备份档总体积可忽略（不显著影响档大小与保留策略）

#### Scenario: 新建业务表自动被备份

- **WHEN** 数据库中新建一张不在排除清单内的表（如未来新增的业务表）后执行备份
- **THEN** 该表进入备份档，无需修改脚本
