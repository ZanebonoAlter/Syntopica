# scheduler-accuracy Delta

## MODIFIED Requirements

### Requirement: DailyReport 在可配置墙钟时刻执行
DailyReport 调度器 SHALL 每天生成一次当日报告，生成时机 SHALL 同时满足：(a) 不早于配置的墙钟时刻（`AISettings` key=`daily_report_time`，值格式 `HH:MM`，默认 `21:00`）；(b) tag 队列（`tag_jobs`）与 embedding 队列（`embedding_queues`）均无 pending/leased 任务。墙钟时刻到点后若队列非空，SHALL 按分钟级周期复查直至队列清空触发。若到兜底时刻（`AISettings` key=`daily_report_deadline`，值格式 `HH:MM`，默认 `23:30`）队列仍非空，SHALL 在兜底时刻强制生成，当日 SHALL NOT 再次生成（单版完整制，不做二次重算）。SHALL NOT 使用「从启动时刻起每 24 小时」的固定间隔模式。

兜底时刻配置 SHALL NOT 早于墙钟时刻，违反时 SHALL 回退默认 `23:30` 并记录警告。当日报告已存在时 SHALL NOT 重复生成（既有幂等语义不变）。

#### Scenario: 队列提前清空则在墙钟时刻准时生成
- **WHEN** 日报时刻配置为 `21:00`，当日 20:40 tag/embedding 队列均已清空
- **THEN** DailyReport 在当日 21:00 触发执行

#### Scenario: 墙钟时队列非空则等待清空
- **GIVEN** 日报时刻配置为 `21:00`，21:00 时 tag_jobs 仍有 pending
- **WHEN** 队列于 22:10 清空
- **THEN** DailyReport 在 22:10 后的下一个复查点触发执行（不早于 21:00）

#### Scenario: 队列持续非空则兜底强制生成
- **GIVEN** 日报时刻 `21:00`、兜底时刻 `23:30`，队列直至 23:30 仍有 pending
- **THEN** DailyReport 在 23:30 强制触发执行，且当日不再重复生成

#### Scenario: 服务在目标时刻前启动
- **WHEN** 日报时刻配置为 `21:00`，服务在当日 18:00 启动
- **THEN** DailyReport 按队列感知时机在当日 21:00（或其后队列清空/兜底时刻）触发执行

#### Scenario: 服务在目标时刻后启动
- **WHEN** 日报时刻配置为 `21:00`、兜底 `23:30`，服务在当日 23:50 启动（当日 21:00 已过、兜底已过且报告未生成）
- **THEN** DailyReport 在次日按队列感知时机触发；当日缺档由既有缺档补档机制处理（SHALL NOT 在启动后立即触发，也 SHALL NOT 等待 24 小时）

#### Scenario: 重启不丢失调度
- **WHEN** 服务在 22:00 重启（墙钟已过、兜底未到、当日报告未生成、队列非空）
- **THEN** 重启后 DailyReport 继续按队列感知逻辑在当日兜底时刻前择机触发（SHALL NOT 因重启退化为「重启后 24 小时」）

#### Scenario: 默认时刻
- **WHEN** `AISettings` 中不存在 `daily_report_time` 与 `daily_report_deadline` 键
- **THEN** DailyReport 使用默认墙钟 `21:00` 与默认兜底 `23:30`

#### Scenario: 非法配置值回退默认
- **WHEN** `daily_report_time` 或 `daily_report_deadline` 为非法格式（如 `"25:99"`、`"abc"`）
- **THEN** 对应项回退默认值（`21:00` / `23:30`），并各记录一条警告日志

#### Scenario: 兜底早于墙钟的非法关系
- **WHEN** `daily_report_deadline` 配置为 `20:00`（早于 `daily_report_time` 的 `21:00`）
- **THEN** 兜底回退默认 `23:30` 并记录警告，墙钟时刻保持 `21:00`
