## MODIFIED Requirements

### Requirement: 日报生成 API — 异步触发
系统 SHALL 提供 `POST /api/daily-reports/generate` 端点，接受 `{date: string, board_id?: number, board_ids?: number[]}` 参数。board_id 为空时生成所有活跃 board 的日报。端点 SHALL 立即返回 `{job_id: string, status: "processing"}`，后台 goroutine 异步执行生成。

端点 SHALL 额外接受可选 `board_ids` 数组表示**多板块集合作用域**：非空时后台仅为该集合内的板块生成日报，一次请求 SHALL 只占用一个执行闸、只返回一个 `job_id`、只广播一次终态、只发送一次终态通知。`board_id` 与 `board_ids` SHALL 互斥：两者同时非空时端点 SHALL 返回 HTTP 400 且 SHALL NOT 启动任何管线。`board_ids` 为空数组或缺省时，SHALL 保持「生成所有活跃 board」的既有语义。集合作用域下的板块 SHALL 逐个生成，任一板块失败 SHALL NOT 中断其余板块，最终失败计数 SHALL 体现在终态通知中。

集合内的重复板块 id SHALL 按集合语义折叠（保留首次出现顺序，每个 id 至多生成一次），SHALL NOT 因重复 id 产生重复生成或重复计入终态计数。`board_id` / `board_ids` 任一字段类型非法（如 `board_ids` 传入非数组、`board_id` 传入非数字）时，端点 SHALL 返回 HTTP 400 且 SHALL NOT 降级为全量作用域——畸形请求 SHALL NOT 触发全站生成管线。

同一时刻 SHALL 至多存在一条日报生成管线，互斥范围 SHALL 覆盖共享同一 PostgreSQL 数据库的**全部后端实例**（定时调度或手动触发，无论发起自哪个实例，共享同一把数据库级执行闸）。已有管线在跑时，手动触发（本端点及调度器 `TriggerNowWithDate`）SHALL NOT 启动新管线，SHALL 返回 409 与 `reason: "already_running"`（文案：「日报生成正在执行中，请稍后再试。」）；定时调度路径获取闸失败时 SHALL 记 skip（reason `already_running`），闸由持闸管线在结束时释放（获取失败方无锁可释放，SHALL NOT 调用释放）。闸在管线结束（成功、失败或 panic）后 MUST 释放；持闸进程崩溃或连接断开时，数据库级闸 SHALL 随连接终结自动释放，SHALL NOT 造成永久死锁。数据库不可达导致闸无法探测时，SHALL 回退进程内互斥（单实例行为不劣于现状），SHALL NOT 因闸探测失败而阻断生成。闸探测失败路径 SHALL 归还该次探测占用的数据库连接，SHALL NOT 泄漏连接池连接。被拒触发 SHALL NOT 产生任何完成/失败终态通知。

#### Scenario: 触发单板日报生成
- **WHEN** 请求 `POST /api/daily-reports/generate {date: "2026-05-25", board_id: 5}`
- **THEN** 系统 SHALL 立即返回 `{job_id: "xxx", status: "processing"}`，后台开始为 board #5 生成日报

#### Scenario: 触发全板日报生成
- **WHEN** 请求 `POST /api/daily-reports/generate {date: "2026-05-25"}`
- **THEN** 系统 SHALL 立即返回 `{job_id: "xxx", status: "processing"}`，后台依次为所有活跃 board 生成日报

#### Scenario: 触发多板块集合生成
- **WHEN** 请求 `POST /api/daily-reports/generate {date: "2026-05-25", board_ids: [3, 7, 9]}`
- **THEN** 系统 SHALL 立即返回 `{job_id: "xxx", status: "processing"}`，后台仅为 board #3、#7、#9 生成日报，SHALL NOT 为集合外的板块生成

#### Scenario: 多板块集合共用一个执行闸与一次终态
- **WHEN** 请求带 `board_ids: [3, 7]` 的生成
- **THEN** 整个集合 SHALL 只占用一个执行闸并只返回一个 `job_id`，SHALL 只广播一次 `daily_report_done` 终态消息，SHALL 只发送一条终态通知

#### Scenario: 集合内单板失败不影响其余板块
- **WHEN** 请求带 `board_ids: [3, 7]` 的生成，其中 board #7 生成失败
- **THEN** 系统 SHALL 继续为 board #3 完成生成，终态通知 SHALL 同时体现成功与失败计数，SHALL NOT 因单板失败丢弃其余板块结果

#### Scenario: 单板块与集合参数互斥
- **WHEN** 请求同时携带 `board_id` 与非空 `board_ids`
- **THEN** 端点 SHALL 返回 HTTP 400 且 SHALL NOT 启动任何生成管线、SHALL NOT 获取执行闸

#### Scenario: 集合内重复板块折叠
- **WHEN** 请求携带 `board_ids: [3, 7, 3]`
- **THEN** 端点 SHALL 只为 board #3 与 #7 各生成一次，SHALL NOT 重复生成 #3，终态计数 SHALL NOT 重复计入 #3

#### Scenario: 集合字段类型非法不降级为全量
- **WHEN** 请求携带类型非法的 `board_ids`（如传入对象而非数组）
- **THEN** 端点 SHALL 返回 HTTP 400 且 SHALL NOT 按全量作用域启动生成管线、SHALL NOT 获取执行闸

#### Scenario: 单板块字段类型非法不降级为全量
- **WHEN** 请求携带类型非法的 `board_id`（如传入字符串而非数字）
- **THEN** 端点 SHALL 返回 HTTP 400 且 SHALL NOT 按全量作用域启动生成管线、SHALL NOT 获取执行闸

#### Scenario: 生成进行中重复手动触发
- **WHEN** 某日报生成管线（定时或手动）尚未结束，再次请求 `POST /api/daily-reports/generate`
- **THEN** 系统 SHALL 返回 HTTP 409、`{success: false, reason: "already_running"}`，SHALL NOT 启动第二条管线，SHALL NOT 广播进度或发送终态通知

#### Scenario: 生成进行中调度到期
- **WHEN** 手动管线在跑期间定时调度到期触发 `DailyReportJob`
- **THEN** 本次调度 SHALL 记为 skip（reason `already_running`），SHALL NOT 与手动管线并发生成，SHALL NOT 发送重复完成通知

#### Scenario: 跨实例互斥
- **WHEN** 后端实例 A 的日报管线在跑，连接同一数据库的后端实例 B（如临时验证实例、冒烟副本）收到手动触发或定时调度触发
- **THEN** 实例 B SHALL 被同一把数据库级闸拒绝（手动 409 `already_running` / 调度 skip），两实例 SHALL NOT 并发生成，ai 调用日志 SHALL NOT 出现同 board 同日两个并发生成 session

#### Scenario: 持闸实例崩溃后闸自动释放
- **WHEN** 持闸实例在管线中途崩溃（进程被杀、容器停止）导致数据库连接断开
- **THEN** 数据库级闸 SHALL 随连接终结自动释放，后续任意实例的下一次触发 SHALL 正常受理（不残留 409 / skip），SHALL NOT 需要人工清锁

#### Scenario: 数据库不可达时回退进程内互斥
- **WHEN** 实例尝试获取数据库级闸时数据库连接不可用
- **THEN** 系统 SHALL 回退到进程内互斥并照常开始生成（行为不劣于升级前），SHALL 在日志中记录回退原因，SHALL NOT 返回错误或无限重试

#### Scenario: 管线结束后闸释放
- **WHEN** 一次生成管线结束（全部版面成功、部分失败或 panic）后，再次请求 `POST /api/daily-reports/generate`
- **THEN** 端点 SHALL 正常受理（返回 `status: "processing"`），不残留 409

#### Scenario: 手动触发与定时调度互斥不产生重复通知
- **WHEN** 手动触发被 409 拒绝
- **THEN** 通知中心 SHALL NOT 出现同日期的第二条「日报已生成」条目（通知只由实际运行的管线发送一次）

### Requirement: 日报生成 WebSocket 进度广播
生成 goroutine SHALL 通过 `ws.GetHub().BroadcastRaw()` 广播两类消息：
- `daily_report_progress`：每完成一个 board 后广播 `{"type": "daily_report_progress", "job_id": "...", "board_id": N, "board_name": "...", "status": "completed|failed", "saved": N, "progress": "current/total"}`
- `daily_report_done`：全部完成后广播 `{"type": "daily_report_done", "job_id": "...", "total_saved": N, "total_boards": N, "failed_count": N, "empty_count": N}`

`daily_report_done` SHALL 自带本次运行的**三态计数**：`total_saved`（产出日报的板块数）、`failed_count`（生成报错、真正失败的板块数）、`empty_count`（当日无内容、`report == nil` 的板块数）。三态互斥且穷尽于本次运行涉及的板块，`total_saved + failed_count + empty_count == total_boards` SHALL 恒成立。`failed_count` / `empty_count` SHALL 与终态通知口径一致（`notification.DailyReportTerminal`）：当日无内容的板块既不计入成功数、也不计入失败数。消费方 SHALL 直接读 `failed_count`，SHALL NOT 用 `total_boards - total_saved` 反推失败数——那会把当日无内容的板块错算成失败，与通知中心口径不一致。`failed_count` / `empty_count` 为新增字段：只读 `total_saved` / `total_boards` 的既有消费方 SHALL 继续正常工作。

#### Scenario: 单板生成完成广播
- **WHEN** board #5 的日报生成成功
- **THEN** 系统 SHALL 广播 `{"type": "daily_report_progress", "job_id": "xxx", "board_id": 5, "board_name": "AI与机器学习", "status": "completed", "saved": 1, "progress": "1/1"}`

#### Scenario: 全部生成完成广播
- **WHEN** 3 个 board 的日报全部生成完毕
- **THEN** 系统 SHALL 广播 `{"type": "daily_report_done", "job_id": "xxx", "total_saved": 3, "total_boards": 3, "failed_count": 0, "empty_count": 0}`

#### Scenario: 终态消息给出三态计数
- **WHEN** 一次生成涉及 3 个 board，其中 1 个产出日报、1 个生成报错、1 个当日无内容
- **THEN** 系统 SHALL 广播 `{"type": "daily_report_done", "job_id": "xxx", "total_saved": 1, "total_boards": 3, "failed_count": 1, "empty_count": 1}`，且 `total_saved + failed_count + empty_count == total_boards` 成立

#### Scenario: 当日无内容的板块既不算成功也不算失败
- **WHEN** 某 board 当日无内容（生成无报错但 `report == nil`）
- **THEN** 该 board SHALL 计入终态消息的 `empty_count`，SHALL NOT 计入 `failed_count`、SHALL NOT 计入 `total_saved`；终态通知的成功/失败计数 SHALL 与终态消息同口径（无内容的板块不进失败数）

#### Scenario: 单板块生成失败
- **WHEN** board #5 的日报生成报错
- **THEN** 系统 SHALL 广播 `{"type": "daily_report_done", "job_id": "xxx", "total_saved": 0, "total_boards": 1, "failed_count": 1, "empty_count": 0}`，SHALL NOT 把该板块计入 `empty_count`

## ADDED Requirements

### Requirement: 日报调度窗口的生成时刻与兜底时刻可读
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

### Requirement: 日报终态通知的计数口径与 WS 终态消息一致
通知中心的日报终态通知 SHALL 与 `daily_report_done` 终态消息共用同一套**三态计数口径**：通知文案中的「共 N 个版面」SHALL 等于终态消息的 `total_boards`（**含**当日无内容的板块），通知中的成功数 SHALL 等于 `total_saved`，失败数 SHALL 等于 `failed_count`。当日无内容（`report == nil`）的板块 SHALL NOT 计入成功数、SHALL NOT 计入失败数，SHALL 以「其中 M 个当日无内容」显式给出，且 M 计入 N。成功侧文案 SHALL 表达「产出日报的板块数」，SHALL NOT 把板块数表述为「条目数」。

该口径 SHALL 同时适用于**手动 / 集合生成路径**与**定时调度路径**（两者 SHALL 共用同一三态写入口 `notification.DailyReportTerminal`）：定时调度路径统计的板块集合 SHALL 完整计入该次运行的板块总数，SHALL NOT 因「不计成功也不计失败」而把当日无内容的板块排除在总数之外——否则同一个「生成日报」动作在两条路径下会给出不同的「共 N 个版面」。定时调度路径的终态通知 SHALL 与手动路径同口径；其 WS 终态消息（`daily_report_complete`）协议 SHALL 保持不变。定时调度路径的收集阶段早退 SHALL 记作「共 1 个版面，0 成功 1 失败」（`TotalBoards=1` / `Saved=0` / `Failed=1` / `Empty=0`），与手动路径的收集阶段失败口径一致。

一次运行 SHALL 至多产生一条终态通知：存在失败板块时 SHALL 只发一条失败汇总（SHALL NOT 另发完成通知），无失败板块时 SHALL 只发一条完成通知——两条通道互斥。

#### Scenario: 1 成功 + 1 失败 + 1 无内容 → 通知与 WS 的总版面数一致
- **WHEN** 一次生成涉及 3 个板块，其中 1 个产出日报、1 个生成报错、1 个当日无内容
- **THEN** 终态消息 SHALL 为 `{"type": "daily_report_done", "job_id": "xxx", "total_saved": 1, "total_boards": 3, "failed_count": 1, "empty_count": 1}`，且终态通知 SHALL 表述为「共 3 个版面，1 成功 1 失败」并给出「其中 1 个当日无内容」，SHALL NOT 把当日无内容的板块计入失败数、SHALL NOT 从「共 N 个版面」中扣除

#### Scenario: 当日无内容的板块走完成通知
- **WHEN** 一次生成只涉及 1 个当日无内容的板块（无失败板块）
- **THEN** 系统 SHALL 只发一条完成通知（SHALL NOT 发失败汇总），通知 SHALL 表述为「共 1 个版面」并给出「其中 1 个当日无内容」，与终态消息的 `{"total_saved": 0, "total_boards": 1, "failed_count": 0, "empty_count": 1}` 同口径

#### Scenario: 收集阶段失败的通知口径
- **WHEN** 生成在收集板块阶段失败
- **THEN** 系统 SHALL 广播 `{"total_saved": 0, "total_boards": 1, "failed_count": 1, "empty_count": 0}` 并只发一条失败汇总通知「共 1 个版面，0 成功 1 失败」

#### Scenario: 定时调度路径与手动路径的总版面数一致
- **WHEN** 定时调度路径的一次运行涉及 3 个板块，其中 1 个产出日报、1 个生成报错、1 个当日无内容
- **THEN** 终态通知 SHALL 表述为「共 3 个版面，1 成功 1 失败」并给出「其中 1 个当日无内容」，SHALL 与手动 / 集合路径对同一次「生成日报」动作给出的口径一致，SHALL NOT 把当日无内容的板块从「共 N 个版面」中扣除

#### Scenario: 定时调度路径收集阶段失败的通知口径
- **WHEN** 定时调度路径在收集板块阶段失败
- **THEN** 系统 SHALL 只发一条失败汇总通知「共 1 个版面，0 成功 1 失败」，SHALL 与手动路径的收集阶段失败口径一致

#### Scenario: 当日无可生成板块不发终态通知
- **WHEN** 当日没有任何可生成的板块（`total_boards == 0`）
- **THEN** 系统 SHALL 只广播一次终态消息 `{"total_saved": 0, "total_boards": 0, "failed_count": 0, "empty_count": 0}`，SHALL NOT 产生终态通知
