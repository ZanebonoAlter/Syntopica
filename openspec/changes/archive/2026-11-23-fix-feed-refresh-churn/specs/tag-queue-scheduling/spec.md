# tag-queue-scheduling Delta — fix-feed-refresh-churn

## ADDED Requirements

### Requirement: 打标作业入队数据库级幂等

同一篇文章 SHALL 至多存在一条未完成（pending/leased）打标作业，该唯一性 SHALL 由数据库部分唯一索引兜底，不依赖应用层 check-then-insert 的时序。并发入队撞索引时 SHALL 按复用处理（原地更新既有作业），SHALL NOT 报错中断调用方。

#### Scenario: 并发入队不产生重复作业

- **WHEN** 两个并发调用同时为同一篇文章入队打标作业（如正文抓取完成回调与刷新链同时到达）
- **THEN** 数据库唯一性保证只成功插入一条，另一条按已存在复用，最终未完成作业至多一条

#### Scenario: 存量重复行建索引前先清理

- **WHEN** 迁移执行时 tag_jobs 已存在同文章多条未完成作业
- **THEN** 迁移 SHALL 先清理重复（保留最小 id 行）再创建唯一索引，迁移幂等可重跑

#### Scenario: 重试失败作业不得与已有活动作业冲突

- **WHEN** 运维触发失败作业批量重试（`POST /api/tag-queue/retry`），其中某篇文章已有 pending/leased 作业（失败后再入队的常态）
- **THEN** 该文章的 failed 行 SHALL 保持 failed 不参与本次重试，活动作业保持至多一条
- **AND** 请求 SHALL 成功返回，SHALL NOT 因唯一索引冲突回滚成 500
- **AND** 同文章堆积多条 failed 行时 SHALL 至多重试一条（最新一条），不得把同文章的失败行一起翻成 pending

#### Scenario: 冲突行转终态时不得报错中断调用方

- **WHEN** 入队撞唯一索引后，冲突行在「插入零行」与「重查」之间转入终态（completed/failed），两次插入机会都没能接住
- **THEN** SHALL 记一条告警并按「已存在」语义返回成功，SHALL NOT 向调用方返回错误
