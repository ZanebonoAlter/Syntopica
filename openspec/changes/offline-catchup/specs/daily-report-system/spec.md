## ADDED Requirements

### Requirement: 缺档日报自动补档

定时日报任务在生成完当天报告后 SHALL 自动补档：在保留窗口内（`tag_edge_retention_days`，与标签边回收同键同口径）逐日检查每个当日应生成报告的板块是否存在 `(board, period_date)` 报告，缺失则按既有生成流水线重建（幂等 upsert，只补缺失、不重建已存在报告）。补档 SHALL 以处理队列已清空（无 pending/leased 的打标任务）为前置条件；队列未清空时本轮 SHALL 跳过补档并于下一轮重试，缺档在窗口内 SHALL NOT 因顺延而丢失。

#### Scenario: 停机缺档次日自动补齐

- **GIVEN** 系统停机 3 天，那 3 天的日报缺失，恢复后打标积压已清空
- **WHEN** 次日 21:00 定时日报任务执行
- **THEN** 生成当天报告后自动重建窗口内缺失的 3 天报告，零人工干预

#### Scenario: 队列未清空顺延

- **GIVEN** 恢复当天 21:00 时打标队列仍有 pending 任务
- **WHEN** 定时日报任务执行
- **THEN** 当天报告照常生成，补档本轮跳过，次日队列清空后再补

#### Scenario: 只补缺不重建已有

- **GIVEN** 窗口内某板块昨日报告已存在
- **WHEN** 补档扫描执行
- **THEN** 该 (board, date) 被跳过，既有报告不被重建

### Requirement: 日报重建窗口守卫

对早于保留窗口下界的日期，系统 SHALL 拒绝重建日报：`POST /api/daily-reports/generate` 返回错误（4xx，说明标签边已按窗口回收、候选不全），调度器 `TriggerNowWithDate` 同口径拒绝。目的：防止超窗日期的"空报告覆盖好报告"（同日重建是整份覆盖语义）。窗口内日期的手动重建 SHALL 不受影响。

#### Scenario: 超窗日期拒绝重建

- **GIVEN** 保留窗口 7 天，请求重建 10 天前的日报
- **WHEN** 调用 POST /api/daily-reports/generate {date: 10天前}
- **THEN** 返回 4xx 错误并说明窗口约束，既有报告（若存在）不被覆盖

#### Scenario: 窗口内日期正常重建

- **GIVEN** 保留窗口 7 天，请求重建 3 天前的日报
- **WHEN** 调用 POST /api/daily-reports/generate {date: 3天前}
- **THEN** 照常异步触发重建

#### Scenario: 调度器指定日期触发同口径

- **WHEN** 经 TriggerNowWithDate 触发早于窗口下界的日期
- **THEN** 拒绝执行（accepted=false），与 API 守卫口径一致
