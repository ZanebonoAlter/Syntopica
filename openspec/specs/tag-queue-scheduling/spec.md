# tag-queue-scheduling Specification

## Purpose
打标队列（tag_jobs）的消费调度契约：在有限的夜间分析窗口内优先消化最具阅读与日报价值的最新文章（新任务优先）。陈年积压不做额外 TTL 淘汰——既有 7 天归档 GC 已兜底（2026-09-24 验收期用户决策取消）。

## Requirements
### Requirement: 新任务优先消费
TagQueue 的 lease 顺序 SHALL 以新任务优先为准：priority 相同时按入队时间新→旧消费；priority 更高的任务 SHALL 优先于更晚入队的低优任务（插队语义保留）。重试退避语义不变：available_at 在未来的任务 SHALL NOT 被 lease。

#### Scenario: 新旧任务并存时新的先消费
- **GIVEN** 队列中存在昨日入队的 pending 任务 A 与今日入队的 pending 任务 B，priority 相同
- **WHEN** TagQueue lease 一批任务
- **THEN** B 先于 A 被 lease 处理

#### Scenario: 高优先级旧任务可插队
- **GIVEN** 旧任务 A priority=10，新任务 B priority=0
- **WHEN** TagQueue lease
- **THEN** A 先于 B 被 lease（priority 高者优先）

#### Scenario: 退避中的任务不被 lease
- **GIVEN** 任务 C 因失败进入退避，available_at 为未来时刻
- **WHEN** TagQueue lease（即使 C 是最新任务）
- **THEN** C 不被 lease，轮到下一顺位任务
