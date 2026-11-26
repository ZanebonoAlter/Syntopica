<!-- domain: daily-report -->

# daily-report-ops Specification

## Purpose

日报的运维侧契约：①**调度**——复用既有定时任务、调度窗口的生成时刻与兜底时刻可读；②**补档**——缺档自动补档、重建窗口守卫、缺口异步补填、补填运行留档与终态告知；③**告警口径**——终态通知的计数与 WebSocket 终态消息一致；④**兼容**——推荐与历史兼容、缺口可见性。本族只管「日报没出来怎么办」；怎么生成见 `daily-report-system`，怎么暴露见 `daily-report-interface`。

## Requirements

### Requirement: 缺档日报自动补档

<!-- from: daily-report-system (2026-05-26 board-interaction-overhaul) -->

定时日报任务在生成完当天报告后 SHALL 自动补档：在保留窗口内（`tag_edge_retention_days`，与标签边回收同键同口径）逐日检查每个当日应生成报告的板块是否存在 `(board, period_date)` 报告，缺失则按既有生成流水线重建（幂等 upsert，只补缺失、不重建已存在报告）。补档 SHALL 以处理队列已清空（无 pending/leased 的打标任务）为前置条件；队列未清空时本轮 SHALL 跳过补档并于下一轮重试，缺档在窗口内 SHALL NOT 因顺延而丢失。

补档重建出的报告 SHALL 与该 `period_date` 当天按时生成的结果等价：所有按日期取数/扫描的下游环节（含关键字物化轨的当日文章扫描、以及注入聚类 prompt 的话题近期内容上下文）SHALL 使用该 `period_date` 作为窗口，SHALL NOT 使用补档执行时刻。补档路径 SHALL NOT 混入晚于该 `period_date` 的文章与章节，SHALL NOT 因执行时刻不同而产生与当天生成不同的内容口径。

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

#### Scenario: 补档注入的话题上下文不含晚于报告日的章节

- **GIVEN** 在 11-03 补档重建 10-31 的日报，11-01 与 11-02 均已存在
- **WHEN** 聚类阶段注入话题近期内容上下文
- **THEN** 注入窗口 SHALL 以 10-31 为基准计算，SHALL NOT 包含 11-01 / 11-02 的章节

### Requirement: 日报推荐与历史兼容

<!-- from: daily-report-system (2026-05-26 board-interaction-overhaul) -->

日报读取 SHALL 以可选附加信息区分未处理建议与已确认标记，不重命名原 thread 字段或改变原文章展开行为。新日报推荐 SHALL 不阻塞日报保存及终态；历史缺字段 SHALL 降为无建议，但仍可从标记读取结果恢复已确认状态。读取历史 SHALL NOT 触发批量 AI 回刷。

#### Scenario: 旧日报可直接手动标记
- **WHEN** 打开没有推荐字段的旧日报且条目仍具有效泳道归属
- **THEN** 正常阅读并可手动标记，不调用 AI 回刷；已有持久标记仍可显示

#### Scenario: 推荐失败不改变日报终态
- **WHEN** 日报已保存而推荐失败
- **THEN** 报告仍为成功，不发第二条日报终态通知，不删除人工节点

### Requirement: 日报调度窗口的生成时刻与兜底时刻可读

<!-- from: daily-report-system (2026-05-26 board-interaction-overhaul) -->

调度器状态载荷 SHALL 为 `daily_report` 调度器同时暴露**生成时刻**（`schedule_time`，取 `ai_settings.daily_report_time`，缺省 21:00）与**强制兜底时刻**（`schedule_deadline`，取 `ai_settings.daily_report_deadline`，缺省 23:30）两个字段，使前端能够同时呈现「计划何时生成」与「最迟必须出结果」两个时刻。两个字段 SHALL 出现在 `/api/poll` 与 `/api/schedulers/status` 的 `daily_report` 条目上。兜底时刻早于生成时刻时 SHALL 沿用既有回落逻辑（回落到默认 23:30）并 SHALL NOT 输出非法值。

#### Scenario: 状态载荷同时给出两个时刻
- **WHEN** 请求 `GET /api/poll`
- **THEN** `data.schedulers` 中 `name` 为 `daily_report` 的条目 SHALL 同时包含 `schedule_time` 与 `schedule_deadline` 两个字段

#### Scenario: 兜底时刻缺省回落默认值
- **WHEN** `ai_settings` 中不存在 `daily_report_deadline` 配置行
- **THEN** `schedule_deadline` SHALL 为默认值 `23:30`

#### Scenario: 兜底时刻早于生成时刻时回落
- **WHEN** `ai_settings` 中 `daily_report_deadline` 早于 `daily_report_time`
- **THEN** `schedule_deadline` SHALL 回落到默认值 `23:30`，SHALL NOT 输出早于生成时刻的非法组合

### Requirement: 日报缺口可见性

<!-- from: daily-report-system (2026-05-26 board-interaction-overhaul) -->

系统 SHALL 提供只读缺口查询 `GET /api/daily-reports/gaps`：在重建窗口内逐日返回「该板块当天有候选内容、但没有 `(board, period_date)` 报告」的格子。窗口 SHALL 为 `[今天 - min(days, tag_edge_retention_days), 今天 - 1]`（`days` 默认 7，不含今天，今天由当日状态行与手动生成入口负责）。`days` 只能在本侧收紧默认窗口，SHALL NOT 越过保留窗口下界（保留窗口是硬上界，不是默认值）。

缺口判定 SHALL 与日报生成 Step1 的候选集口径一致（active + event、非 `direction_mismatch`、跨板排他），SHALL NOT 用宽松口径把生成时必然被剔除的板块算成缺口。

响应 SHALL 给出窗口边界、平铺的缺口格子（报告日 + 板块 id + 名称）、缺口总天数与总篇数，以及最近一次补填运行摘要（无记录时缺省）。查询 SHALL NOT 写库、SHALL NOT 触发任何生成。

#### Scenario: 列出窗口内缺口

- **GIVEN** 窗口内 11-22 有 2 个板块缺报、11-21 有 1 个板块缺报，其余日期均已有报告
- **WHEN** 调用 `GET /api/daily-reports/gaps`
- **THEN** 返回 2 个日期、3 个缺报格子，且 `missing_days=2`、`total_missing=3`

#### Scenario: 已有报告的日子不是缺口

- **WHEN** 某 (board, date) 存在报告
- **THEN** 该格子 SHALL NOT 出现在缺口响应中

#### Scenario: 当日无候选内容的板块不是缺口

- **GIVEN** 某板块当天全部候选 tag 因 `direction_mismatch` 或跨板排他被剔除
- **WHEN** 查询该日缺口
- **THEN** 该板块 SHALL NOT 出现在缺口响应中（补它必然产出空报告）

#### Scenario: 窗口外不返回

- **GIVEN** 保留窗口 7 天，`days=30` 被请求
- **WHEN** 调用缺口查询
- **THEN** 返回的窗口下界 SHALL 为今天-7，SHALL NOT 返回更早的日期

#### Scenario: 今天不出现在缺口区

- **WHEN** 今天还没有日报
- **THEN** 缺口响应 SHALL NOT 包含今天（由当日状态行负责呈现）

### Requirement: 日报缺口异步补填

<!-- from: daily-report-system (2026-05-26 board-interaction-overhaul) -->

系统 SHALL 提供 `POST /api/daily-reports/backfill-gaps`：请求立即返回 `job_id`，补填在服务端后台串行执行，SHALL NOT 阻塞 HTTP 响应。

补填 SHALL 只处理空格子（已存在报告的跳过，不重建不覆盖），按日期从旧到新处理，并与定时任务和手动生成共用日报生成闸门。打标队列未排空、窗口外日期、闸门被占时 SHALL 拒绝并说明原因。整轮预算 SHALL 为 60 分钟（`ai_settings` 可调），耗尽时保留已完成结果并记录剩余缺口。补填 SHALL 幂等：中断后重新触发只补仍缺的格子。补填 SHALL NOT 触发版块体检（一次补填可能跨多板，逐板体检会放大 LLM 成本；体检由每日主轮的尾步负责）。

#### Scenario: 异步受理

- **WHEN** 调用 `POST /api/daily-reports/backfill-gaps`
- **THEN** 立即返回 `job_id` 与处理中状态，补填在后台继续
- **AND** 请求线程 SHALL NOT 等待补齐完成

#### Scenario: 只补空不重建

- **GIVEN** 缺口 3 篇，其中某格子已有报告
- **WHEN** 补填执行完成
- **THEN** 只新增那 3 篇缺失报告，既有报告的内容、section 与 thread 不变

#### Scenario: 从旧到新串行

- **GIVEN** 11-19、11-20、11-21 三天都缺报
- **WHEN** 补填执行
- **THEN** 生成顺序 SHALL 为 11-19 → 11-20 → 11-21
- **AND** 11-20 的报告 `prev_report_id` SHALL 指向本次补出的 11-19 报告

#### Scenario: 闸门被占时拒绝

- **GIVEN** 定时日报任务正在执行
- **WHEN** 触发补填
- **THEN** 返回冲突错误并说明日报生成正在执行，SHALL NOT 并行启动第二条流水线

#### Scenario: 打标队列未排空时拒绝

- **GIVEN** `tag_jobs` 仍有 pending 任务
- **WHEN** 触发补填
- **THEN** 拒绝执行并说明队列未排空、补出的候选不全

#### Scenario: 窗口外日期被拒绝

- **GIVEN** 请求补填窗口下界之前的日期
- **WHEN** 提交补填请求
- **THEN** 返回 4xx 并说明标签边已按窗口回收，SHALL NOT 产出具空报告

#### Scenario: 预算耗尽留档顺延

- **GIVEN** 预算 60 分钟，补到第 4 天的途中预算耗尽
- **WHEN** 本轮结束
- **THEN** 已完成的报告保留，剩余缺口写入运行结果，终态为 `budget_exhausted`
- **AND** 再次触发时只补剩余缺口

#### Scenario: 中断重跑幂等

- **GIVEN** 上一次补填中途进程终止，部分格子已补出
- **WHEN** 重新触发补填
- **THEN** 只处理仍缺的格子，SHALL NOT 重建已补出的报告

### Requirement: 补填运行留档与终态告知

<!-- from: daily-report-system (2026-05-26 board-interaction-overhaul) -->

每次补填运行 SHALL 持久化一条运行记录，至少含：触发窗口、逐日的成功/失败/空结果明细、终态、开始与结束时间、补出篇数与未完成缺口。终态取值 SHALL 为 `success`（全部补齐或无缺口）、`partial`（部分失败）、`failed`（全部失败）、`budget_exhausted`（预算耗尽）之一。

每次运行 SHALL 恰好产生一条终态通知，且**成功与失败都必须发**：成功发 success 类，部分失败 / 全部失败 / 预算耗尽发 error 类，文案含补出篇数、失败数、剩余缺口数。补填终态通知 SHALL NOT 与每日生成终态通知互相覆盖或合并（两者是不同运行），单次补填运行 SHALL NOT 逐格子发通知。

#### Scenario: 全部成功发一条完成通知

- **WHEN** 一次补填补齐 5 篇、无失败
- **THEN** 通知中心新增一条 success 通知，含补出篇数与日期范围

#### Scenario: 部分失败发一条失败汇总

- **WHEN** 一次补填补齐 3 篇、2 个格子生成失败
- **THEN** 产生一条 error 通知（含成功数、失败数），终态为 `partial`
- **AND** SHALL NOT 为每个失败格子各发一条通知

#### Scenario: 全部失败发通知

- **WHEN** 一次补填所有格子均生成失败
- **THEN** 产生一条 error 通知，终态为 `failed`

#### Scenario: 预算耗尽发通知

- **WHEN** 补填因预算耗尽停止
- **THEN** 产生一条 error 通知，含已补篇数与剩余缺口数，终态为 `budget_exhausted`

#### Scenario: 零缺口不发通知

- **GIVEN** 当前窗口内没有缺口
- **WHEN** 触发补填
- **THEN** 不产生运行记录以外的副作用，SHALL NOT 产生通知

#### Scenario: 运行结果可从缺口查询读到

- **WHEN** 一次补填结束后再次查询缺口
- **THEN** 响应携带最近一次运行摘要（终态、补出篇数、结束时间）

### Requirement: 定时任务复用

<!-- from: daily-report-system (2026-05-26 board-interaction-overhaul) -->
<!-- kind: impl -->

系统 SHALL 复用 `scheduler_tasks` 表中的 `narrative_summary` 任务，改造执行逻辑调用 `daily_report.GenerateDailyReport`。check_interval 保持 86400s。定时任务 SHALL 异步执行并通过 WS 广播进度。

#### Scenario: 定时触发日报生成
- **WHEN** `narrative_summary` 定时任务按 check_interval 触发
- **THEN** 系统 SHALL 为所有活跃 board 生成当日日报，使用与手动触发相同的异步 WS 流程
