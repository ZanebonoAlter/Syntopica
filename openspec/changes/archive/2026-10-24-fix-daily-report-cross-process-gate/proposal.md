<!-- complexity: complex -->
<!-- ui-impact: none -->
<!-- constraint-domains: daily-report, scheduler -->

## Why

2026-10-23 23:30 发生跨进程日报双跑事故：兜底窗口内存在第二个后端实例（临时 go run / smoke 副本）与常驻进程并发生成，AI 调用完整双份（ai_call_logs 每 board 两个交错主 session），常驻进程对 5 个 board 的 SaveReport 撞 `idx_board_daily_reports_board_period` 唯一约束报 duplicate key。根因：日报生成互斥域闸（guard-daily-report-generate 引入的 `TryAcquireDailyReportGeneration`）是进程内 `atomic.Bool`，其注释自述"single-process deployment ... no advisory lock"——前提被临时第二实例破坏，跨进程完全失效。且 10-22~10-23 后端重启 6 次，多实例并存窗口并不罕见。数据虽因唯一约束未损坏，但 AI 消耗翻倍、终态通知口径错乱、失败方 retry 浪费，需要根治。

## What Changes

- 日报生成域闸从进程内 `atomic.Bool` 升级为 **PostgreSQL advisory lock**（`pg_try_advisory_lock`，专用连接持有至 release），互斥范围从"单进程"扩展到"共享同一数据库的全部实例"；三个触发面（定时 job / `POST /api/daily-reports/generate` / scheduler `TriggerNowWithDate`）继续共享同一把闸，对外行为（409 already_running / 调度 skip）不变。
- `SaveReport` 落库的并发安全从 check-then-insert（TOCTOU，双写时后到者报 duplicate key）升级为 **(board, date) 粒度事务级 advisory lock 串行化 + 既有覆盖重建路径**——同板同日并发写时后到者在锁上等待、醒来走既有 Updates 覆盖路径，不炸唯一约束、收敛一条记录（实现取舍见 design D4，拒绝 ON CONFLICT 整句重写）。
- DB 不可达时闸的降级语义明确化：advisory lock 探测失败时回退进程内原子闸（fail-open 到现状行为），保证数据库抖动不阻断单实例正常运行。

## Capabilities

### New Capabilities

（无）

### Modified Capabilities

- `daily-report-system`: 「日报生成 API — 异步触发」Requirement 中"同一时刻 SHALL 至多存在一条日报生成管线"的互斥语义从进程内扩展为数据库级跨进程互斥（共享同一数据库的任何后端实例之间互斥）；「日报数据模型」Requirement 的日报记录唯一性补充原子 upsert 落库措辞（并发写同 board 同日时后者原子覆盖，不产生约束错误）。

## Impact

- **代码**：`backend-go/internal/topicgraph/service/generation_gate.go`（闸实现替换，保持函数签名与 seam 不变）；`backend-go/internal/topicgraph/service/daily_report_watch.go` 无改（闸在更上层）；`SaveReport` 落库点（`backend-go/internal/topicgraph/repository/daily_report_repository.go`）改 ON CONFLICT upsert；`backend-go/internal/topicgraph/wire.go` re-export 不动。
- **测试**：`generation_gate` 单测需覆盖：跨"进程"模拟（两条独立 DB 连接互斥）、持闸连接断开自动释放（advisory lock 随会话终结）、DB 故障回退进程内闸；`SaveReport` 并发双写单测（后写原子覆盖无 duplicate key）。
- **运维**：无迁移、无配置项新增（lock key 为代码常量）；部署后无需用户操作；临时验证实例建议继续用 `DEMO_READ_ONLY=1`（本 change 不强制）。
- **不影响**：前端零改动；手动触发 409 语义、调度 skip 语义、通知口径均不变；daily-report-lifeline-heal / board-health tail steps 不受影响（闸释放时机不变）。
