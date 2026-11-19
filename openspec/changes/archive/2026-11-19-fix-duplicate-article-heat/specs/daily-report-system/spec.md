## MODIFIED Requirements

### Requirement: 缺档日报自动补档
定时日报任务在生成完当天报告后 SHALL 自动补档：在保留窗口内（`tag_edge_retention_days`，与标签边回收同键同口径）逐日检查每个当日应生成报告的板块是否存在 `(board, period_date)` 报告，缺失则按既有生成流水线重建（幂等 upsert，只补缺失、不重建已存在报告）。补档 SHALL 以处理队列已清空（无 pending/leased 的打标任务）为前置条件；队列未清空时本轮 SHALL 跳过补档并于下一轮重试，缺档在窗口内 SHALL NOT 因顺延而丢失。

补档重建出的报告 SHALL 与该 `period_date` 当天按时生成的结果等价：所有按日期取数/扫描的下游环节（含关键字物化轨的当日文章扫描）SHALL 使用该 `period_date` 作为窗口，SHALL NOT 使用补档执行时刻。补档路径 SHALL NOT 混入晚于该 `period_date` 的文章，SHALL NOT 因执行时刻不同而产生与当天生成不同的内容口径。

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

#### Scenario: 补档内容不含晚于报告日的文章

- **GIVEN** 系统在 11-03 夜间补档重建 10-31、11-01、11-02 三天的日报
- **WHEN** 三份报告分别生成
- **THEN** 每份报告引用的文章 SHALL 全部 `pub_date` 落在各自 `period_date` 当日
- **AND** 11-03 当天的文章 SHALL NOT 出现在这三份报告的任一 section / thread 中
