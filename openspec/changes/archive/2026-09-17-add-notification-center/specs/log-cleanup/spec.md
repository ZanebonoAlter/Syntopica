## ADDED Requirements

### Requirement: 队列表 completed/failed 行保留清理
既有 scheduled log cleanup 作业 SHALL 同时清理 `tag_jobs` 与 `firecrawl_jobs`：status='completed' 且创建时间早于 1 天的行（每日重置语义）、status='failed' 且创建时间早于 30 天的行（保留近期失败供面板重试）。清理查询 MUST 有 created_at 索引支撑。

#### Scenario: 定时清理移除过期 completed 行
- **WHEN** scheduled cleanup 运行且 tag_jobs/firecrawl_jobs 存在 status='completed'、创建时间早于 1 天的行
- **THEN** 这些行被删除，今日 completed 行保留

#### Scenario: failed 行保留 30 天
- **WHEN** scheduled cleanup 运行且存在 failed 行创建时间在 30 天内
- **THEN** 这些行保留，TagQueuePanel 重试入口可用

#### Scenario: 存量堆积一次性自动淘汰
- **WHEN** 清理任务上线后首次运行（存量约 1.9w tag_jobs / 7.2w embedding / 2.6k firecrawl completed 行）
- **THEN** 过期行全部删除，无需手工 SQL

#### Scenario: 无过期行时正常空跑
- **WHEN** scheduled cleanup 运行且无过期行
- **THEN** 作业正常完成，不报错

## MODIFIED Requirements

### Requirement: embedding_queues completed 保留策略

既有 scheduled log cleanup 作业 SHALL 同时清理 `embedding_queues` 中 status='completed' 且创建时间早于保留期（默认 **1 天**）的行，防止 completed 历史行无限累积。清理查询 MUST 有 created_at（或等价时间列）索引支撑。

#### Scenario: 定时清理移除过期 completed 行
- **WHEN** scheduled cleanup 运行且存在 status='completed'、创建时间早于 1 天的行
- **THEN** 这些行被删除，1 天内的 completed 行保留

#### Scenario: 无过期行时正常空跑
- **WHEN** scheduled cleanup 运行且无过期行
- **THEN** 作业正常完成，不报错
