## Purpose

调度器的时间表达必须真实且可控。本能力确保：(1) 调度器对外的 `next_run` 始终等于真实的下次触发时刻，而非误导性的「当前时刻」；(2) DailyReport 在可配置的墙钟时刻（默认 21:00）执行，且服务重启后仍能按时触发（不依赖「连续运行满周期」）。

## Requirements

### Requirement: next_run 反映真实下次触发时刻
所有基于固定间隔（`Interval`）的调度器，其 `next_run` 字段 SHALL 等于真实的下次触发时刻：有 `StartupDelay` 时为 `启动时刻 + startupDelay`，否则为 `启动时刻 + interval`。SHALL NOT 在 `StartupDelay=0` 时将 `next_run` 设为「当前时刻」。

#### Scenario: 无启动延迟时 next_run 等于启动时刻加间隔
- **WHEN** `auto_refresh` 调度器（`Interval=60s`，`StartupDelay=0`）在 t=0 启动
- **THEN** 其 `next_run` 等于 `t+60s`（而非 `t=0`）

#### Scenario: 有启动延迟时 next_run 等于启动时刻加延迟
- **WHEN** `log_cleanup` 调度器（`Interval=86400s`，`StartupDelay=5min`）在 t=0 启动
- **THEN** 其 `next_run` 等于 `t+5min`（首次触发时刻），执行后变为 `执行时刻 + interval`

#### Scenario: 前端展示的下次执行时间准确
- **WHEN** 前端读取调度器状态的 `next_run` 字段
- **THEN** 显示的剩余时间与实际下次触发时刻一致，SHALL NOT 永远显示「即将开始」

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
### Requirement: 日报时刻可配置且可保存
系统 SHALL 通过 `AISettings` 存储 `daily_report_time`，复用现有 `GET/PUT /api/settings` 接口。保存时 SHALL 校验 `HH:MM` 格式（`00:00`–`23:59`），非法值 SHALL 被拒绝（返回错误，不写入）。

#### Scenario: 保存合法时刻
- **WHEN** 通过设置接口保存 `daily_report_time` = `"09:30"`
- **THEN** 值被写入 `ai_settings`，后续 DailyReport 调度使用 09:30

#### Scenario: 保存非法时刻被拒绝
- **WHEN** 通过设置接口保存 `daily_report_time` = `"25:00"`
- **THEN** 返回校验错误，原值不变

### Requirement: 配置变更在下次执行后生效
DailyReport 的墙钟计算 SHALL 在每次执行后重新读取配置时刻。配置变更 SHALL 在当前周期结束后生效（最迟一个执行周期）。

#### Scenario: 修改时刻后下次执行按新时刻
- **WHEN** 当前日报时刻为 `21:00`，在某次执行后将其改为 `09:00`
- **THEN** 下次 DailyReport 在次日 09:00 触发（重新读取配置）
