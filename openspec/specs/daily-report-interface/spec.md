<!-- domain: daily-report, ui -->

# daily-report-interface Specification

## Purpose

日报的对外契约层：①**接口**——生成异步触发、WebSocket 进度广播、查询端点、重新分桶端点；②**前端展示**——报纸布局时间线、生成进度、TagsPage 内容区、站内阅读预览全屏层与直达、刊头「今日重点」空态与重点依据定位；③**管线状态可视化**——雷达页右栏的「标签队列积压 → 日报生成窗口 → 当日是否已出」状态块、双队列清空判据、缺口区与手动补生成入口（含二次确认）。本族只规定「对外长什么样」；生成算法见 `daily-report-system`，补档与告警见 `daily-report-ops`。

## Requirements

### Requirement: 日报生成 API — 异步触发

<!-- from: daily-report-system (2026-05-26 board-interaction-overhaul) -->

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

<!-- from: daily-report-system (2026-05-26 board-interaction-overhaul) -->

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

### Requirement: 日报查询 API

<!-- from: daily-report-system (2026-05-26 board-interaction-overhaul) -->

系统 SHALL 提供以下查询端点：
- `GET /api/semantic-boards/:id/daily-reports?days=7`：查询该 board 最近 N 天的日报列表
- `GET /api/daily-reports/:id`：查询单篇日报详情（含关联 sections，每个 section 通过 GORM Preload 包含 threads 列表）
- `GET /api/semantic-boards/:id/section-timeline?days=14`：查询板块 section 时间线（含关系）

`GET /api/semantic-boards/:id/daily-reports` SHALL 在未提供有效正数 `days` 时使用 7 天默认值，并对任意正数 `days` 查询实际请求范围，不得静默截断到 30 天。

`GetReportByID` SHALL 使用嵌套 Preload `"Sections.Threads"` 加载 section 及其关联线程。`DailyReportSection` 的 JSON 响应中 `threads` 字段 SHALL 包含 `[]DailyReportThread` 对象数组（每条含 id、report_id、section_id、title、summary、tag_ids、confidence），section 和 thread 均不含 status/prev_*_id 字段。

`GET /api/daily-reports/:id` 返回的每条 thread 的 `related_article_ids` SHALL 是**读路径投影**：系统 SHALL 先按「归一 link 相同即同一篇内容」折叠同内容副本，再返回折叠后的数组。折叠 SHALL 保持原数组顺序、SHALL 以**首个出现**为准只保留一条（沿用日报数组契约的「保序 + 首个出现为准」口径）。判重键 SHALL 为归一 link：归一 SHALL 复用 `textutil.NormalizeArticleLink` 的既有规则（去 fragment、去 query、scheme/host 小写），SHALL NOT 另立口径，SHALL NOT 引入标题相似度、时间窗口等其它判据。`link` 为空或归一后为空、以及该 id 在库里查不到时**内容身份不可判定**：该 id SHALL NOT 参与**基于 link 的**折叠、SHALL 原样保留（不删除、不改指其它 id）。同一 article id 在数组内重复出现时 SHALL 折叠为一条（首个出现为准）——同一 id 必属同一篇文章，该折叠与「归一 link 相同」等价，故在内容身份不可判定时同样适用。折叠 SHALL 只作用于响应：落库值 `daily_report_threads.related_article_ids` SHALL 保持为该 thread `tag_ids` 对应文章的并集，SHALL NOT 被任何身份/去重逻辑改写。折叠所需的文章 link SHALL 在单次读路径中批量获取；获取失败 SHALL 降级为原样返回（不折叠、不阻断详情接口）。

#### Scenario: 查询板块日报列表
- **WHEN** 请求 `GET /api/semantic-boards/5/daily-reports?days=7`
- **THEN** 系统 SHALL 返回 board #5 最近 7 天的日报列表，每条含 id、title、summary、period_date、status、cluster_count、article_count

#### Scenario: 查询日报详情含线程
- **WHEN** 请求 `GET /api/daily-reports/42`
- **THEN** 系统 SHALL 返回日报 #42 完整内容，每个 section 包含 threads 列表（每条含 id、title、summary、tag_ids、confidence、related_article_ids），section 和 thread 均不含 status/prev_*_id 字段

#### Scenario: API 查询超过 30 天
- **WHEN** 请求 `GET /api/semantic-boards/5/daily-reports?days=42`
- **THEN** 系统 SHALL 返回最近 42 天范围内的全部匹配日报

#### Scenario: 无日报时返回空
- **WHEN** 请求 `GET /api/semantic-boards/5/daily-reports?days=7`，但该 board 无日报
- **THEN** 系统 SHALL 返回空数组

#### Scenario: 线索下同 link 副本折叠
- **WHEN** 某条 thread 的 `related_article_ids` 含两个 `link` 相同的文章 id（跨 feed 副本）
- **THEN** `GET /api/daily-reports/:id` 返回的该 thread `related_article_ids` SHALL 只含其中首个出现的 id，数组长度减一、顺序不变
- **AND** `daily_report_threads.related_article_ids` 的落库值 SHALL 保持未折叠

#### Scenario: 归一后 link 相同也折叠
- **WHEN** 同一条 thread 的两篇文章 link 仅在 fragment、query 或 host 大小写上不同（归一后相同）
- **THEN** 返回的 `related_article_ids` SHALL 折叠为一条（首个出现为准）

#### Scenario: link 为空不折叠
- **WHEN** 某条 thread 的 `related_article_ids` 含 `link` 为空的文章
- **THEN** 该文章 SHALL 原样保留在返回的 `related_article_ids` 中，SHALL NOT 被当作同内容副本剔除

#### Scenario: 同标题不同 link 不折叠
- **WHEN** 同一条 thread 的两篇文章标题相同但 `link` 不同（多源转载）
- **THEN** 返回的 `related_article_ids` SHALL 保留两个 id（本口径只认归一 link 相同）

#### Scenario: 同一 id 重复引用折叠为一条
- **WHEN** 同一条 thread 的 `related_article_ids` 含重复出现的同一个 article id
- **THEN** 返回的 `related_article_ids` SHALL 只保留该 id 的首个出现（与 link 是否可判定无关）

### Requirement: 重新分桶 API

<!-- from: daily-report-system (2026-05-26 board-interaction-overhaul) -->

系统 SHALL 提供 `POST /api/daily-reports/:id/rebucket`，请求体含 `section_id`（该日报内任一条 section），用于对**该 section 所在的桶组**（同一 `report_id` + `cluster_index`）重跑泳道内主题分桶。

服务端 SHALL：先取与日报重跑**同键**的事务级串行锁（`(semantic_board_id, period_date)` 粒度），并在锁内复核报告状态；以组内全部 thread 为输入重跑两阶段分桶；按新分桶结果替换该组的 section 行（保留/更新原有行以尽量减少 ID 变化，新增不足的桶行，删除多余的桶行）；thread 行 SHALL 复用并把 `section_id` 迁到新桶，SHALL NOT 删除 thread（关键事件注记因此不丢）；`topic_key_event_markers.source_section_id` SHALL 按 `source_thread_id` 重新派生；`topic_watch_hits.section_id` SHALL 随所在 section 的替换同步迁移或按组删除重建，SHALL NOT 留下悬空命中行；`report_annotations.section_id` SHALL 按 `thread_id` 重新派生（无 thread 的报头注记挂该组 `bucket_index=0` 的 section）；重新计算每条 section 的 `cluster_label` / `bucket_title` / `bucket_index` / `cluster_tag_ids` / `article_count` / `embedding` / `topic_match_distance` / `best_tier` / `avg_score` / `quality_breakdown`（并沿用报告期的 `topic_status_at_report` 快照）；其中 `article_count` SHALL 按该桶线索 `related_article_ids` 的**去重并集**重算（重分桶只持有 thread 事实、不重新选文，故与生成期「Σ tag 文章数」口径存在有意差异，属展示计数）；`best_tier` / `avg_score` / `quality_breakdown` SHALL 取自报告期冻结的 `quality_breakdown` 快照按桶内 tag 子集重算（不重新打分）；重算该话题质心；**双向**重建该组涉及的 section 关系（含跨天入边的反向引用）。

前置门槛：报告 status 非 `done` SHALL 返回 409；目标 section 的 `lane_tier` 为 `watch_keyword` / `watch_sentence` 时 SHALL 返回 400（物化组由关注轨自己管理，不接受主题分桶）；未达「泳道内主题分桶」触发阈值时 SHALL 直接返回 200 + `buckets=1` 且不改动任何数据。

失败（LLM 失败、解析失败、数据库事务失败）SHALL 整体回滚并返回错误；「校验后收敛为 1 组」SHALL NOT 视为失败（同上，200 + `buckets=1` 且不改数据）。响应 SHALL 返回该组最新的 section 列表与 `buckets` 计数。

#### Scenario: 重新分桶替换一组 section

- **GIVEN** 日报 #100 的 `cluster_index=3` 组当前为 1 条 18-tag 的 section
- **WHEN** 调用 `POST /api/daily-reports/100/rebucket` 传入该 section 的 id
- **THEN** 响应 SHALL 返回 2-6 条 section，全部 `cluster_index=3`，原 thread 行的 `section_id` SHALL 已指向新桶

#### Scenario: 重新分桶保留关键事件注记

- **GIVEN** 该组某条 thread 上有 confirmed 关键事件注记
- **WHEN** 执行重新分桶
- **THEN** 注记 SHALL 仍然存在，其 `source_thread_id` SHALL 不变，`source_section_id` SHALL 重新指向该 thread 现在所属的 section

#### Scenario: 拆不动时返回单桶且不改数据

- **GIVEN** 该组内容确实只有一个主题方向
- **WHEN** 调用重新分桶
- **THEN** 响应 SHALL 为 200 且 `buckets=1`，数据库 SHALL NOT 发生 section 替换

#### Scenario: 失败整体回滚

- **GIVEN** 重新分桶的归属阶段 LLM 调用失败
- **WHEN** 请求返回错误
- **THEN** 该组 section 与 thread 的 `section_id` SHALL 保持调用前状态，SHALL NOT 出现部分替换

#### Scenario: 拒绝非完成态日报

- **GIVEN** 日报 #100 的 status 为 generating
- **WHEN** 调用重新分桶
- **THEN** 系统 SHALL 返回 409，SHALL NOT 修改任何数据

#### Scenario: 拒绝 watch 物化组

- **GIVEN** 目标 section 的 `lane_tier=watch_sentence`
- **WHEN** 调用重新分桶
- **THEN** 系统 SHALL 返回 400，SHALL NOT 修改任何数据

#### Scenario: 未达阈值不改数据

- **GIVEN** 目标组只有 4 条 thread、5 个 tag（低于阈值）
- **WHEN** 调用重新分桶
- **THEN** 系统 SHALL 返回 200 + `buckets=1`，SHALL NOT 调用分桶 LLM，SHALL NOT 修改任何 section/thread 行

#### Scenario: 与同日重跑并发时串行化

- **GIVEN** 同一 board 同一天的重跑（SaveReport）与 rebucket 并发发起
- **WHEN** 两者竞争
- **THEN** 两者 SHALL 通过同键事务级 advisory lock 串行化，不出现「thread 已被重跑删除、rebucket 的 UPDATE 静默影响 0 行」或「向已重建报告插入旧 cluster_index 幽灵 section」

#### Scenario: watch 命中与页边注跟随重派生

- **GIVEN** 目标组关联了 `topic_watch_hits` 命中行与 `report_annotations` 页边注
- **WHEN** 重新分桶替换该组 section
- **THEN** `topic_watch_hits.section_id` SHALL 指向 thread 现在所属的 section（或按组重建），SHALL NOT 悬空
- **AND** `report_annotations.section_id` SHALL 按 `thread_id` 重新派生，注记内容不变

### Requirement: 日报时间线组件 BoardDailyReportTimeline（报纸布局）

<!-- from: daily-report-system (2026-05-26 board-interaction-overhaul) -->

前端 SHALL 提供 `BoardDailyReportTimeline.vue` 组件，展示板块日报列表，并以**阅读台**（`/tags/:boardId/:date`）呈现选中日报。阅读台 SHALL 沿用旧版阅读层的**三栏**：左侧目录栏（本期目录 / 持续追踪 / 历史日报 / 版块切换）、正文（刊头 + 分区正文）、宽屏下常驻的右侧页边注栏；MUST NOT 渲染已撤下的右栏演进/本期话题。页边注在 <1100px SHALL 收为右下浮动入口 + 抽屉。日期历史 SHALL 由左侧目录栏的历史列表承载（「加载更早」增量），不再使用日期下拉。

**入口与容器**：阅读台 SHALL 作为独立路由页面渲染，取代原先 `position: fixed; inset: 0` 的全屏浮层。离开阅读台（返回按钮或 Esc）SHALL 回到该版块的矩阵视图，并保留进入前的选中泳道。阅读台 SHALL 在页面容器内整体滚动（顶栏 sticky，左目录与页边注栏各自 sticky），MUST NOT 把正文锁进固定高度的小滚动窗。

**宽屏布局**：桌面宽屏（>1100px）SHALL 采用三栏——左目录定宽、正文自适应铺满剩余宽度（MUST NOT 再压成 40rem 居中窄栏）、右侧页边注栏 `clamp(15rem, 17vw, 17rem)`；刊头（含头条与「今日重点」最多 3 条 highlights）SHALL 在正文上方全宽展示，MUST NOT 隐藏 highlights。1100px 及以下 SHALL 单栏（页边注收为右下浮动按钮 + 抽屉）；767.98px 及以下 SHALL 单栏且不得产生页面级横向溢出。

**主题**：阅读台的 surface、文字、边框、阴影和交互状态 SHALL 使用现有 semantic theme token，并同时支持 editorial/dark。topic 的稳定 `persistent_topic.color` MAY 用作身份强调色。组件 SHALL NOT 以固定浅色背景为前提硬编码黑色透明色。

**内容映射**：
1. masthead 展示真实日报标题、日期、article_count 和 cluster_count；刊头 SHALL NOT 固定声称"号外"。
2. 头条优先取 `highlights[0]`；highlights 为空时回退到质量最高的 section，且 SHALL NOT 生成 API 中不存在的新闻文案。
3. highlights 按 API 顺序最多展示三项；为空时不渲染该区块。
4. section 按 `persistent_topic.status` 分为"关心的话题"(active)、"突发的新话题"(candidate/已归属但未 active)和"其他动态"(未归属)。分区内继续按 best_tier 升序、avg_score 降序排列。
5. `dynamics` 为空时不渲染"板块动态"区块。

**目录与日期联动**：左侧目录栏 SHALL 提供「头条」「话题正文」锚点、持续追踪话题、历史日报日期列表与版块切换；历史日报 SHALL 按日期倒序列出当前已加载日报，点击 SHALL 切换日期；「加载更早」SHALL 扩充该列表的日期范围。分区索引下拉与 scroll-spy 已于 2026-10-27 用户复审撤下，阅读台 SHALL NOT 依赖它们。

**话题卡片与线索相关事件**：active 话题卡片 SHALL 支持原位展开。展开后每条当日线索（thread）SHALL 在其行内渲染「相关线索」chip 行，数据来自 `thread-related-events` 的 `GET /api/daily-reports/:reportId/thread-relations`；首次展开时 SHALL 以 `days=7` 请求一次并覆盖整份日报的全部线索，成功后按 `(reportId, days)` 缓存。MUST NOT 为每条线索各发一次请求。

每条线索 SHALL 最多渲染 3 个 chip，其余折叠为「还有 N 条」（N 来自响应中该线索的推荐总数，展开后仍只渲染 chip，不跳转、不弹层）。**没有相关事件的线索 SHALL NOT 渲染 chip 行**——不占位、不留最小高度、不给「暂无相关」文案。

chip SHALL 在指针悬停或键盘聚焦时显示预览浮层，内容为：候选线索的标题、其摘要、来源文章标题、相似度档位与分数、该候选的日报日期。预览浮层 SHALL 以固定定位渲染并做视口边界翻转，MUST NOT 被线索行的 `overflow` 裁剪。

点击 chip SHALL 直接打开站内阅读预览弹窗（复用既有入口与既有出口），弹窗内 SHALL 显示该推荐的相关度档位、分数与候选日期作为阅读上下文；MUST NOT 跳转离开阅读台。

相关线索档位 SHALL 为 `7 天 / 30 天` 两档，默认 7 天，且 SHALL 以**分区级工具条**呈现（每个内容分区一条，不放在线索行内——否则无候选的线索行会留下只有档位控件的空行）。切换档位 SHALL 以新档位重新请求（含失败后重试），切回已成功加载过的档位 SHALL 命中缓存不再请求。推荐加载失败 SHALL NOT 阻断日报阅读：该线索的 chip 行不渲染，卡片其余内容照常呈现。

daily-report 阅读台 SHALL NOT 再渲染 7 天折线、节点、贝塞尔连线、节点计数或「在侦探墙打开完整生命线」入口。本 change 之后「完整话题生命线」在界面上**暂无入口**（原来的唯一入口随 mini 生命线退役，版块态势页当前未挂载侦探墙）——该缺口 SHALL 如实登记为已知限制，MUST NOT 在文档里宣称「从版块态势页进入」而实际不可达。原「话题总览」入口与 desk 索引里的「在矩阵中查看这条线」入口已于 2026-10-27 复审随新版索引一并撤下；泳道/矩阵聚焦入口保留在版块态势页自身，阅读台不再内嵌该出口。

**thread 文章**：thread 有 `related_article_ids` 时 SHALL 可原位展开文章标题列表。前端只加载尚未缓存的 article id，相同文章跨 thread 复用缓存；单篇失败不阻塞其他文章并提供重试。点击文章 SHALL 直接打开站内阅读预览弹窗（详情拉取期间弹窗呈现加载态、失败给出可见错误提示），并保留阅读台上下文。

**section 生命周期**：点击 section header SHALL 继续打开右侧 SectionLifecyclePanel，不因新增线索相关事件 chip 而移除 section 维度入口。

"加载更早" SHALL 每次将 `days` 增加 7 并重新请求该完整时间范围。组件 SHALL 用响应替换当前列表，避免累计请求造成重复日报。切换 board SHALL 将 `days` 重置为 14（阅读台按 14 天展示历史列表，可继续「加载更早」），并清理与旧 board 相关的展开状态。

#### Scenario: 展示日报卡片列表
- **WHEN** 选中 board "AI与机器学习"，该 board 有 3 天的日报
- **THEN** BoardDailyReportTimeline SHALL 渲染 3 张日报卡片，按日期倒序

#### Scenario: 打开全屏日报详情
- **WHEN** 用户点击某日报卡片或从版块态势页点击「打开今日日报」
- **THEN** 路由 SHALL 变为 `/tags/:boardId/:date` 并渲染占满视口的三栏阅读台（左目录 / 正文 / 页边注）
- **AND** 返回后 SHALL 回到该版块矩阵并保留进入前的选中泳道

#### Scenario: 宽屏双栏与窄屏降级
- **WHEN** 分别以 1920px、1100px 和 720px viewport 打开同一日报
- **THEN** 1920px SHALL 显示「左目录 + 宽屏正文 + 右页边注栏」三栏，刊头与「今日重点」top3 可见，不出现右栏演进
- **AND** 1100px SHALL 单栏并把页边注收为右下浮动入口 + 抽屉
- **AND** 720px SHALL 单栏且不产生页面级横向溢出

#### Scenario: 双主题渲染
- **WHEN** 用户在 editorial 与 dark 主题间切换
- **THEN** 日报 surface、文字、边框和交互状态 SHALL 使用对应主题语义 token 并保持可读

#### Scenario: 按持久话题状态分区
- **WHEN** 日报包含 2 个 active、1 个 candidate 和 1 个未归属 section
- **THEN** 页面 SHALL 分别在"关心的话题""突发的新话题""其他动态"展示 2、1、1 个 section

#### Scenario: 线索行渲染相关事件 chip
- **WHEN** 用户展开一个 active 话题卡片，其中某条线索在窗口内有 5 条推荐
- **THEN** 该线索行 SHALL 渲染前 3 个 chip 与「还有 2 条」
- **AND** 整份日报的推荐 SHALL 由一次 `thread-relations` 请求取得

#### Scenario: 无相关事件的线索不渲染 chip 行
- **WHEN** 某条线索在窗口内没有任何相似度达到阈值的候选线索（任一档位）
- **THEN** 该线索 MUST NOT 渲染 chip 行或任何占位/空态文案
- **AND** 线索标题、摘要、文章数 SHALL 与未引入本特性时一致
- **AND** 档位控件 SHALL 位于分区级工具条，MUST NOT 因该线索无候选而出现在线索行内

#### Scenario: chip 悬停出预览
- **WHEN** 用户将指针悬停在某个 chip 上，或用 Tab 聚焦它
- **THEN** 系统 SHALL 显示预览浮层，含候选线索标题、摘要、来源文章标题、相似度档位与分数与该候选日期
- **AND** 浮层 MUST NOT 被线索行容器裁剪

#### Scenario: 点击 chip 打开阅读预览
- **WHEN** 用户点击某个 chip
- **THEN** 系统 SHALL 打开站内阅读预览弹窗并加载该来源文章
- **AND** 弹窗内 SHALL 显示该推荐的关联度与出现次数
- **AND** 路由 MUST NOT 离开当前日报阅读台

#### Scenario: 7/30 档位切换与缓存
- **WHEN** 用户从 7 天档切到 30 天档，再切回 7 天档
- **THEN** 切到 30 天 SHALL 以 `days=30` 请求一次
- **AND** 切回 7 天 SHALL 直接使用缓存，SHALL NOT 再次请求

#### Scenario: 推荐加载失败不阻断阅读
- **WHEN** `thread-relations` 返回错误
- **THEN** 线索的 chip 行 SHALL 不渲染
- **AND** 日报正文、线索标题摘要、文章列表与关键事件动作 SHALL 照常可用

#### Scenario: identity 连线跨越空白日期
- **WHEN** 窗口内周一与周三有节点、周二无节点，且存在周一到周三的 identity 关系
- **THEN** 日报 SHALL NOT 渲染任何连线或节点图
- **AND** identity 关系 SHALL 继续由后端保留，供版块态势页与侦探墙使用

#### Scenario: 展开并缓存话题生命线
- **WHEN** 用户展开一个 active 话题卡片
- **THEN** 日报 SHALL NOT 再调用 `getTopicLifeline(topicId)`
- **AND** 该位置的能力改由线索相关事件承担，行为以「线索行渲染相关事件 chip」为准
- **AND** 后端 `lifeline` 端点 SHALL 继续存在（版块态势页与侦探墙仍使用）

#### Scenario: lifeline 加载失败
- **WHEN** 旧 lifeline 端点返回错误
- **THEN** 日报话题卡片 SHALL NOT 因此进入错误态（该调用已从日报退役）
- **AND** 错误呈现归属线索相关事件，见「推荐加载失败不阻断阅读」

#### Scenario: 进入完整话题生命线
- **WHEN** 用户在日报阅读台展开 active 话题卡片
- **THEN** 系统 SHALL NOT 渲染「在侦探墙打开完整生命线」入口
- **AND** 完整生命线入口当前 SHALL 视为已知缺口（无界面入口），MUST NOT 渲染任何不可达的入口按钮

#### Scenario: 窄容器排版不崩坏
- **WHEN** 阅读台正文列宽度在 690–900px（视口 900–1400px）且话题卡片已展开
- **THEN** 线索行 SHALL 不产生区块级横向溢出
- **AND** chip 行 SHALL 在容器宽内换行，MUST NOT 出现逐字换行的竖排列
- **AND** 布局 MUST NOT 按「容器宽 / 7 列 × 2」计算宽度

#### Scenario: 展开 thread 文章并复用缓存
- **WHEN** 两个 thread 引用同一个 article id，用户依次展开两个 thread
- **THEN** 前端 SHALL 只为该 article id 请求一次文章详情
- **AND** 点击文章 SHALL 直接打开站内阅读预览弹窗并保留阅读台上下文

#### Scenario: 键盘关闭详情
- **WHEN** 阅读台打开且用户按 Esc
- **THEN** 系统 SHALL 返回矩阵视图，并将焦点恢复到打开阅读台的入口

#### Scenario: 空状态
- **WHEN** 选中 board 但该 board 无日报
- **THEN** 组件 SHALL 展示"暂无日报"并提供生成日报入口

#### Scenario: 加载超过 30 天
- **WHEN** 当前 `days=28`，用户连续两次点击"加载更早"
- **THEN** 组件 SHALL 依次以 `days=35` 和 `days=42` 请求，并展示对应范围内的日报

#### Scenario: 切换 board 重置状态
- **WHEN** 用户从一个 board 切换到另一个 board
- **THEN** 组件 SHALL 将 `days` 重置为 14、加载新 board 日报，并清理旧 board 的 topic/thread 展开状态

### Requirement: 日报生成进度前端

<!-- from: daily-report-system (2026-05-26 board-interaction-overhaul) -->

前端 SHALL 提供 `useDailyReportProgress.ts` composable，连接 `/ws`，过滤 `daily_report_progress`/`daily_report_done` 消息。`NarrativeGenerateDialog.vue` SHALL 改为触发日报生成（调用 `generateDailyReport`），触发后显示进度板模式：每个 board 一行，实时更新状态（等待/生成中/完成+条数），使用 `useDailyReportProgress` composable。

#### Scenario: 触发生成并显示进度
- **WHEN** 用户在 NarrativeGenerateDialog 选择日期和 board，点击"生成"
- **THEN** 对话框 SHALL 切换为进度板模式，实时显示每个 board 的生成状态

#### Scenario: 生成完成
- **WHEN** 收到 `daily_report_done` 消息
- **THEN** 进度板 SHALL 显示"全部完成"提示和总数统计

### Requirement: TagsPage 内容 Tab

<!-- from: daily-report-system (2026-05-26 board-interaction-overhaul) -->

TagsPage 选中 board 时 SHALL 显示三个 Tab：板块内容(composition)、日报(daily-reports)、文章(articles)。Tab 切换 SHALL 用 `v-if` 控制三个面板的显隐。默认 Tab 为"板块内容"。"日报" Tab 面板 SHALL 使用 `BoardDailyReportTimeline` 组件。

#### Scenario: Tab 切换到日报
- **WHEN** 用户点击"日报" Tab
- **THEN** 系统 SHALL 显示 BoardDailyReportTimeline 面板，隐藏 BoardCompositionPanel 和文章列表

#### Scenario: Tab 切换到文章
- **WHEN** 用户点击"文章" Tab
- **THEN** 系统 SHALL 显示带筛选的文章列表，隐藏其他面板

### Requirement: 站内阅读预览的全屏层必须盖过宿主浮层

<!-- from: daily-report-system (2026-05-26 board-interaction-overhaul) -->

文章预览弹窗（`ArticlePreviewModal`，日报阅读台 / 版块态势页 / 搜索页共用）内的「全屏」SHALL 展示覆盖整个视口的正文层，且该层 SHALL 盖过宿主弹窗遮罩——用户点「全屏」后 SHALL 立即看到全屏正文，SHALL NOT 仍停留在弹窗视图（表现为「点了没反应」）。全屏层层级 SHALL 由宿主显式传入（`ArticleContentView` 的 `fullscreenZIndex` prop），SHALL NOT 在正文组件内硬编码低于宿主遮罩的层级；阅读页原地全屏 SHALL 保持既有层级行为不变。

#### Scenario: 弹窗内进入全屏
- **WHEN** 用户在文章预览弹窗内点击「全屏」
- **THEN** 全屏正文层 SHALL 覆盖视口并显示在弹窗遮罩之上
- **AND** 视口中心命中元素 SHALL 落在全屏层内（而非弹窗内容区）

#### Scenario: 退出全屏
- **WHEN** 用户在全屏层内点击「退出全屏」
- **THEN** 系统 SHALL 回到弹窗内的常规阅读视图

#### Scenario: 阅读页原地全屏不回归
- **WHEN** 用户在主阅读页（非弹窗宿主）点击「全屏」
- **THEN** 全屏层 SHALL 保持既有层级行为（不被页面浮层压住）

### Requirement: 阅读台线索文章直达站内阅读预览

<!-- from: daily-report-system (2026-05-26 board-interaction-overhaul) -->

日报阅读台（`/tags/:boardId/:date`）内点击线索引用文章 SHALL **直接打开站内原文预览弹窗**（复用既有 `ArticlePreviewModal`），SHALL NOT 先弹出中间文章抽屉。打开弹窗前 SHALL 拉取文章详情：详情请求进行中弹窗 SHALL 呈现加载态；请求失败 SHALL 给出可见错误提示且不留空弹窗。预览弹窗内的收藏等状态动作 SHALL 真实落库并即时反映到界面：收藏 SHALL 采用乐观更新——点击后星标立即翻转，不等待网络往返；请求失败 SHALL 回滚到点击前状态并给出可见错误提示。SHALL NOT 出现「点击无响应」的空转实现（事件被空函数吞掉）或状态长时间不更新。

**适用范围**：本 Requirement 覆盖日报阅读台（`DailyReportDeskShell`）与版块矩阵页日报时间线（`useTagsPage.openArticlePreview`）两条预览入口；其他宿主的收藏入口沿用同一乐观更新契约。预览弹窗内部的全屏层级、内嵌网页高度等契约由既有 Requirement 承载，本 Requirement 不重复约束。

#### Scenario: 点击线索文章直达预览弹窗

- **WHEN** 用户在阅读台点击线索展开列表中的一篇引用文章
- **THEN** 系统 SHALL 直接打开站内阅读预览弹窗展示该文章
- **AND** SHALL NOT 出现中间文章抽屉层级

#### Scenario: 详情加载中呈现加载态

- **WHEN** 用户点击文章且详情请求尚未返回
- **THEN** 预览弹窗 SHALL 呈现加载态
- **AND** 请求返回后 SHALL 以完整文章内容替换加载态

#### Scenario: 详情加载失败可见反馈

- **WHEN** 文章详情请求失败
- **THEN** 系统 SHALL 给出可见的错误提示
- **AND** SHALL NOT 留下空白无内容的弹窗

#### Scenario: 站内阅读预览收藏即时生效

- **WHEN** 用户在日报阅读台打开的预览弹窗内点击工具栏的收藏
- **THEN** 星标 SHALL 立即切换为已收藏
- **AND** 系统 SHALL 调用文章更新接口持久化该收藏状态

#### Scenario: 收藏失败回滚并提示

- **WHEN** 收藏请求失败
- **THEN** 星标 SHALL 回滚为点击前的状态
- **AND** 系统 SHALL 给出可见的错误提示

#### Scenario: 连点不丢更新

- **WHEN** 用户在收藏请求返回前再次点击收藏
- **THEN** 界面 SHALL 以最新一次点击为准展示状态
- **AND** 迟到的失败响应 SHALL NOT 覆盖更新后的状态

#### Scenario: 预览内文章状态更新回流

- **WHEN** 预览弹窗发出文章状态更新
- **THEN** 弹窗当前文章 SHALL 应用该更新

### Requirement: 刊头「今日重点」空态可见性

<!-- from: daily-report-system (2026-05-26 board-interaction-overhaul) -->

日报刊头组件 SHALL 在 `highlights` 为空（`null` 或 `[]`）时渲染**可见的空态占位**，SHALL NOT 整块隐藏。正常态（至少 1 条 highlights）的渲染 SHALL 保持不变。

空态占位 SHALL 复用刊头既有边框语言与空态文案家族（「本期」+「暂无 X」），SHALL 携带可被组件测试定位的 `data-testid`，SHALL NOT 引入可交互元素（无按钮 / 无跳转 / 无批注锚点）。空态与正常态 SHALL 互斥渲染，SHALL NOT 出现重复 id。

#### Scenario: highlights 为空数组时渲染空态占位
- **WHEN** 日报 `highlights` 为 `[]`
- **THEN** 刊头 SHALL 渲染空态占位（文案含「本期暂无重点」）且 SHALL NOT 渲染要闻条目

#### Scenario: highlights 为 null 时渲染空态占位
- **WHEN** 日报 `highlights` 为 `null`（历史坏数据或接口异常）
- **THEN** 刊头 SHALL 正常渲染空态占位，SHALL NOT 抛错，SHALL NOT 整块消失

#### Scenario: highlights 正常时渲染要闻且无空态占位
- **WHEN** 日报 `highlights` 含 3 条
- **THEN** 刊头 SHALL 渲染 3 条要闻（最多展示 3 条），SHALL NOT 出现空态占位

### Requirement: 重点依据与正文定位

<!-- from: daily-report-system (2026-05-26 board-interaction-overhaul) -->

前端 SHALL 在今日重点旁显示服务端快照「N 篇来源」（0 是合法值，按内容身份去重后的簇内文章数），可读提示附数据截至日、研究时点与聚簇口径标记。字段缺失 SHALL 省略数字而非伪造 0。带有效 `cluster_id` 且有正文目标的重点 SHALL 可键盘激活，定位本 report 当前正文 DOM 顺序中该聚簇第一条 thread；必要时展开目标所在正文容器，滚动、聚焦并高亮 2s。锚点 SHALL 为 `report-thread-{thread.id}`，聚簇只读解析。

带簇内标签的重点 SHALL 可展开（默认收起）列出本期正文中该簇的全部线索：前端用 `tag_ids`（缺失时回退 `cluster_id`）与本 report 正文 thread 的 `tag_ids` 求交集现算，按正文 DOM 顺序列出，每行标注所属话题名；命中代表标签（`cluster_id`）的线索 SHALL 排最前并显式标注「代表线索」。行点击 SHALL 精确定位该条 thread（必要时展开所在正文容器，滚动/聚焦/高亮 2s，语义同重点定位）。展开 SHALL 纯前端现算，SHALL NOT 依赖后端新增字段或快照补齐（历史 highlights 同样可展开）；交集为空 SHALL 显示「本期正文未找到对应线索」，SHALL NOT 编造线索或跳到近似位置。重点条目主体点击与 `cluster` 深链的定位语义不变（仍为正文 DOM 顺序首条）。

无 `cluster_id` 的重点 SHALL 仅展示文案、不猜标签归属。无目标 SHALL 禁用定位并提示「本期正文暂无对应内容」；点击时目标消失 SHALL 原地轻提示，SHALL NOT 打开新详情页、近似跳 section 或自动重建。重复激活 SHALL 替换单个高亮与计时器，切 report/卸载清理；reduced-motion SHALL 不平滑滚动，划选 SHALL NOT 触发定位。

跨页寻址 SHALL 通过日报阅读台路由的 `cluster` 查询参数（`/tags/:boardId/:date?cluster=<cluster_id>`）：参数 SHALL 在该期日报详情就绪后，按聚簇身份解析并定位本 report 当前正文 DOM 顺序中该聚簇第一条 thread，展开/滚动/聚焦/高亮 2s，与刊头重点定位同一路径。参数所指聚簇在本期正文无落点时 SHALL NOT 滚动、SHALL NOT 跳到近似位置，SHALL 提示「本期正文暂无对应内容」。旧的 `story` 查询参数 SHALL 继续被解析但 SHALL 被忽略，并提示该定位方式已下线，SHALL NOT 报错、SHALL NOT 定位到任何内容。翻页或切换 report 后 SHALL 清理该参数，SHALL NOT 在其它报告重复定位；无效或非法的参数值 SHALL 忽略且 SHALL NOT 报错阻断阅读。

#### Scenario: 点击第一条可见正文线程
- **WHEN** 同一聚簇在本期两个正文位置出现，用户点击或键盘激活重点
- **THEN** SHALL 展开需要展开的正文容器并滚动/聚焦 DOM 中第一条对应 thread，高亮 2s，SHALL NOT 按最小 id 或跳别的 report

#### Scenario: 失去目标与旧重点
- **WHEN** 重点无 `cluster_id` 或有 id 但正文已无对应 thread
- **THEN** 无 id 的重点 SHALL 保持静态文案；无目标的聚簇重点 SHALL 提示本期正文暂无对应内容，SHALL NOT 产生空跳转

#### Scenario: 重复定位与减弱动画
- **WHEN** 连续定位两条重点，随后切换 report，且用户设置 reduced-motion
- **THEN** SHALL 始终最多一个高亮，切换时清理，无平滑动画，划选文案 SHALL NOT 触发定位

#### Scenario: 展示筛选不改变热度
- **WHEN** 某 thread 因其它原因未进入正文展示
- **THEN** 该聚簇的 `article_count` SHALL 不因其被筛掉而改变，SHALL NOT 在定位时把用户带到其它内容

#### Scenario: 深链参数定位与失去目标
- **WHEN** 用户打开 `/tags/1974/2026-11-06?cluster=7`，该期正文含该聚簇的 thread
- **THEN** 阅读台 SHALL 定位该聚簇在正文 DOM 顺序中的第一条 thread 并高亮 2s
- **AND** 当该期正文已无该聚簇的 thread 时 SHALL 提示「本期正文暂无对应内容」且不滚动，SHALL NOT 跳到近似位置

#### Scenario: 旧 story 深链被忽略不报错
- **WHEN** 用户打开携带 `?story=42` 的历史链接
- **THEN** 页面 SHALL 正常打开并提示该定位方式已下线
- **AND** SHALL NOT 定位到任何 thread，SHALL NOT 报错

### Requirement: 雷达总览页右栏底部常驻管线状态块

<!-- from: radar-pipeline-status (2026-10-31 add-radar-pipeline-status) -->

L0 今日雷达（`/tags`）的阅读右栏 SHALL 在底部常驻一个管线状态块，状态块与上方内容区（今日该读单清单）分离：内容区独立滚动，状态块固定在右栏底部不随内容滚动。

状态块默认**折叠为一行摘要**（matrix-clue-fallback-and-rail-merge）：正常态只占一行，形如「状态词 · 日报 x/y · 队列 {标签队列文章数}[+{Embedding 队列标签数}]」——两条队列量纲不同（待打标**文章** / 待向量化**标签**），SHALL 分段呈现、SHALL NOT 相加成一个数；状态词呈现读取中 / 正常 / 日报未生成 / 队列积压 / 有失败（失败态 SHALL 带条数）；摘要行本身 SHALL 呈现异常词与计数（折叠 SHALL NOT 藏异常信号）。点击摘要行 SHALL 展开全部详情，再点 SHALL 收起；用户手动展开/收起随时可用。异常态 SHALL **自动展开**（含挂载时即异常）：当日日报已过生成时刻（`schedule_time`）仍未生成、任一队列出现失败任务（标签或 Embedding）、生成请求 failed、或队列活跃量达到明显积压阈值（当前 100）；异常消除后 SHALL 回到折叠摘要。取数窗口（总览取数中/取数失败）内数据为**未知**而非「异常消除」，SHALL NOT 据此折叠再展开（折叠态 SHALL NOT 抖动）。

展开态 SHALL 展示五项事实：

1. **标签队列**：待打标文章数（`pending + processing`）与今日完成进度（`completed_today` / 本轮总量）；
2. **Embedding 队列**：待向量化标签数（`pending + processing`）与今日完成进度（`completed_today` / 本轮总量）——单位是**标签**（embedding_queues 行按 tag_id 排），与标签队列的**文章**单位不同，两个队列 SHALL 各自独立呈现、各带自己的计数与进度；
3. **日报生成窗口**：生成时刻（`schedule_time`）与强制兜底时刻（`schedule_deadline`）两个时刻均 SHALL 显式展示；
4. **最新日报落点**：各活跃板块最新日报日期（`report_date`）中的最大值；
5. **当日日报状态**：以各板块 `report_date` 与本地当天日期比对得出「今日已出 / 今日未出」。

状态块 SHALL NOT 新建独立的周期性请求：全部数据 SHALL 复用已常驻的对账数据分发与雷达页已有的总览载荷。状态块 SHALL 遵循 R-17 语义：**首次取数失败不得把初始零值呈现为业务结论**——对账从未成功时摘要与详情 SHALL 呈现「读取中」而非「今日日报未生成」；对账失败但有旧值时 SHALL 保留旧值并标注更新时刻。

#### Scenario: 状态块常驻右栏底部
- **WHEN** 打开 L0 雷达总览页
- **THEN** 右栏底部 SHALL 常驻管线状态块（默认折叠为一行摘要），内容区（今日该读单清单）独立滚动，状态块不随内容区滚动

#### Scenario: 默认折叠成一行摘要
- **WHEN** 对账成功、双队列清空、今日日报已出（正常态）
- **THEN** 状态块 SHALL 只呈现一行摘要（如「管线正常 · 日报 13/13 · 队列 0」），五项事实与手动生成入口 SHALL NOT 展开
- **AND** 点击摘要行 SHALL 展开五项事实与生成入口，再次点击 SHALL 收起

#### Scenario: 异常态自动展开
- **WHEN** 已过生成时刻而当日日报未生成，或任一队列出现失败任务，或生成请求 failed，或队列活跃量 ≥ 明显积压阈值
- **THEN** 状态块 SHALL 自动展开详情（含挂载时即异常的情形），摘要行 SHALL 呈现对应异常词与计数
- **AND** 异常消除后 SHALL 回到折叠摘要
- **AND** 总览取数窗口内（取数中/取数失败）SHALL NOT 把「数据未知」当作「异常消除」而收起再展开（折叠态 SHALL NOT 抖动）

#### Scenario: 状态块展示五项事实
- **WHEN** 对账成功且总览载荷取数成功，状态块处于展开态
- **THEN** 状态块 SHALL 同时呈现标签队列待打标数与今日完成进度、Embedding 队列待向量化数与今日完成进度、日报生成时刻与兜底时刻、最新日报落点日期、当日日报是否已出

#### Scenario: 首次对账未成功不呈现业务结论
- **WHEN** 状态块挂载后对账从未成功过
- **THEN** 状态块 SHALL 呈现「读取中」，SHALL NOT 呈现「今日日报未生成」或任何等价的业务结论

#### Scenario: 对账失败保留旧值
- **WHEN** 状态块已有一次成功对账的值，随后对账失败
- **THEN** 状态块 SHALL 继续显示上一次的值并标注更新时刻，SHALL NOT 清零、SHALL NOT 转错误态

#### Scenario: 总览取数失败不判未生成
- **WHEN** 对账成功但雷达总览载荷取数失败
- **THEN** 状态块的队列行 SHALL 照常显示，日报行 SHALL 呈现「读取失败」，SHALL NOT 判定为「今日日报未生成」

### Requirement: 管线状态块的手动日报生成入口

<!-- from: radar-pipeline-status (2026-10-31 add-radar-pipeline-status) -->

状态块 SHALL 提供「立即生成今日日报」入口。点击后 SHALL 在状态块内就地展开板块作用域清单，SHALL NOT 打开弹窗、SHALL NOT 跳转页面。清单中**勾选表示参与本次生成**，默认全部勾选，用户 SHALL 能取消勾选不参与的板块，并 SHALL 能一键全选/全不选。确认生成后，系统 SHALL 以所勾选板块集合为作用域发起一次手动生成；未勾选任何板块时 SHALL 禁止提交。

生成期间按钮 SHALL 进入进度态并显示进度；生成完成后 SHALL 内联显示结果并刷新雷达总览数据。

状态块 SHALL 在标签队列仍有活跃待打标文章时**禁用**生成按钮并给出缺料说明：手动生成不等队列，当日才完成打标的文章不会进入当日日报（文章对日报的价值窗口截止于其 `pub_date` 当日报告生成时刻）。

#### Scenario: 默认全选并可取消勾选
- **WHEN** 展开板块作用域清单
- **THEN** 全部板块 SHALL 处于勾选态，SHALL 能取消勾选任一板块使其不参与本次生成，SHALL 展示当前参与数与总数

#### Scenario: 生成以勾选集合为作用域
- **WHEN** 取消勾选两个板块后确认生成
- **THEN** 系统 SHALL 发起一次仅覆盖剩余勾选板块的手动生成请求，SHALL NOT 为被取消勾选的板块生成日报

#### Scenario: 未勾选任何板块禁止提交
- **WHEN** 取消勾选全部板块
- **THEN** 确认生成按钮 SHALL 处于禁用态，SHALL NOT 发起生成请求

#### Scenario: 队列未清空时禁用生成
- **WHEN** 标签队列存在活跃待打标文章（`pending + processing > 0`）
- **THEN** 生成按钮 SHALL 处于禁用态，且状态块 SHALL 给出缺料说明（手动生成不等队列，当日才完成打标的文章不会进入当日日报）

#### Scenario: 清单展开期间队列复堵仍禁止提交
- **WHEN** 生成清单已展开、用户已勾选板块，期间标签队列出现新的活跃待打标文章（`pending + processing > 0`）
- **THEN** 确认生成按钮 SHALL 处于禁用态，SHALL NOT 发起生成请求（队列门槛与入口按钮同款判据）

#### Scenario: 今日已出时重生成给出替换警示
- **WHEN** 至少一个板块的 `report_date` 等于本地今天（今日日报已出），用户展开生成清单
- **THEN** 入口按钮文案 SHALL 为「重新生成今日日报」，清单内 SHALL 给出替换警示（说明今日日报的标题、章节与线索将被重写），确认按钮文案 SHALL 为「确认替换」

#### Scenario: 队列清空后生成可用
- **WHEN** 标签队列无活跃待打标文章
- **THEN** 生成按钮 SHALL 可用，缺料说明 SHALL 消失

#### Scenario: 生成期间显示进度
- **WHEN** 生成请求已受理且收到生成进度消息
- **THEN** 按钮 SHALL 处于进度态并显示当前进度，SHALL NOT 可重复触发

#### Scenario: 生成完成后内联显示结果并刷新
- **WHEN** 收到生成终态消息
- **THEN** 状态块 SHALL 内联显示生成结果（成功板块数与失败数），并 SHALL 触发雷达总览数据刷新

### Requirement: 管线状态块的双队列清空判据

<!-- from: radar-pipeline-status (2026-10-31 add-radar-pipeline-status) -->

日报的自动生成要求两个消费队列**均**清空：`tag_jobs`（pending+leased）与 `embedding_queues`（pending+processing）——日报的泳道分桶依赖标签向量到质心的距离，embedding 未清空会把文章落进「未归位（unmatched）」桶。状态块 SHALL 呈现**双队列**事实（而非只呈现标签队列），且手动生成门槛 SHALL 与自动生成同款判据：双队列任一有活跃量时 SHALL 禁用生成并给出对应的缺料说明。

两个队列的失败任务都是终态：SHALL 只做红色提示，SHALL NOT 计入活跃量、SHALL NOT 阻断生成。当 Embedding 队列计数不可得时（例如对账响应不含该段），SHALL 按「未知即挡住」处理并呈现「状态未知」，SHALL NOT 断言该队列已清空。

#### Scenario: 双队列任一有积压时禁用生成
- **WHEN** 标签队列无活跃待打标文章，但 Embedding 队列存在活跃待向量化标签（`pending + processing > 0`）
- **THEN** 生成按钮 SHALL 处于禁用态，且状态块 SHALL 给出 Embedding 队列的缺料说明（泳道分桶依赖标签向量，未清空会把文章落进「未归位」桶）

#### Scenario: 两个队列各自的今日完成进度
- **WHEN** 两个队列均有今日完成计数与活跃量
- **THEN** 状态块 SHALL 为两个队列各呈现一条今日完成进度（标签队列分母按文章计、Embedding 队列分母按标签计），SHALL NOT 把两个分母混为同一量纲

#### Scenario: Embedding 队列计数未知时挡住生成
- **WHEN** 对账响应不包含 Embedding 队列计数
- **THEN** 状态块 SHALL 呈现「状态未知」，生成按钮 SHALL 处于禁用态，SHALL NOT 呈现「队列已清空」

#### Scenario: 双队列均清空时生成可用
- **WHEN** 标签队列与 Embedding 队列均无活跃量
- **THEN** 生成按钮 SHALL 可用，SHALL NOT 出现队列缺料说明

### Requirement: 管线状态块的缺口区与补齐入口

<!-- from: radar-pipeline-status (2026-10-31 add-radar-pipeline-status) -->

状态块展开态 SHALL 呈现「近 7 天缺口」折叠区（默认折叠，行头带 `N 天 / M 篇`；无缺口时行头呈现「近 7 天无缺口」且不出按钮）。点击行头 SHALL 就地展开缺口清单，SHALL NOT 打开弹窗、SHALL NOT 跳转页面。

缺口清单 SHALL 逐日列出缺报日期与该日缺报板块数，每行提供「补这天」，清单底部提供「一次补齐」。补齐 SHALL 走异步补填接口，点击后立即受理并进入进度态；补填进行中「补这天」与「一次补齐」SHALL 处于禁用态，SHALL NOT 可重复触发。

缺口区取数 SHALL 为一次性请求（挂载时、补填结束后、跨日切换时），SHALL NOT 新建周期性轮询；取数失败时缺口区 SHALL 独立降级（呈现读取失败与重试），SHALL NOT 影响状态块其余事实行。缺口区 SHALL NOT 呈现今日（今日状态由既有「当日日报状态」行负责）。

补填入口 SHALL 沿用生成入口的双队列门槛：队列有活跃积压或计数不可得时 SHALL 禁用并给出缺料说明。

#### Scenario: 展开缺口清单

- **GIVEN** 窗口内 2 天存在缺口，共 3 篇
- **WHEN** 展开状态块并点击「近 7 天缺口 2 天 / 3 篇」
- **THEN** 就地列出两个日期行与各自的缺报板块数，且底部呈现「一次补齐 3 篇」

#### Scenario: 无缺口时不呈现按钮

- **GIVEN** 近 7 天无缺口
- **WHEN** 展开状态块
- **THEN** 缺口行呈现「近 7 天无缺口」，SHALL NOT 呈现「补这天」「一次补齐」

#### Scenario: 补这天只提交该日

- **WHEN** 点击某个日期行的「补这天」
- **THEN** 发起一次仅覆盖该日期的补填请求，SHALL NOT 为该日之外的缺口生成日报

#### Scenario: 补填进行中禁止重复触发

- **WHEN** 补填已受理且仍在进行
- **THEN** 「补这天」与「一次补齐」SHALL 禁用，进度行呈现已补数与总数

#### Scenario: 补填结束后刷新

- **WHEN** 收到补填终态
- **THEN** 缺口区 SHALL 重新取数，状态块 SHALL 刷新雷达总览数据，并内联呈现补填结果

#### Scenario: 缺口取数失败不影响其他事实

- **WHEN** 缺口查询失败而状态块其余数据正常
- **THEN** 缺口区呈现读取失败与重试入口，其余五项事实照常呈现

#### Scenario: 缺口区不含今天

- **WHEN** 今天尚无日报
- **THEN** 缺口区 SHALL NOT 列出今天，今日状态仍由「当日日报状态」行呈现

#### Scenario: 队列积压时补填入口禁用

- **GIVEN** 标签队列仍有待打标文章（或 Embedding 队列有积压 / 计数不可得）
- **WHEN** 展开缺口区
- **THEN** 「补这天」与「一次补齐」SHALL 处于禁用态，并呈现缺料说明（补出的候选会不全）
- **AND** 队列清空后 SHALL 变为可用

### Requirement: 重新生成今日日报的二次确认

<!-- from: radar-pipeline-status (2026-10-31 add-radar-pipeline-status) -->

当入口按钮处于「重新生成今日日报」语义（至少一个板块今日日报已出）时，用户在板块勾选清单中确认替换后，系统 SHALL 再弹一次确认（复用全局确认弹窗，危险样式），说明既有日报的章节与线索将被重写、页边注与人工锚定可能错位。用户取消时 SHALL NOT 发起任何生成请求；用户确认后才 SHALL 发起重新生成。

#### Scenario: 取消确认不发起生成

- **GIVEN** 今日日报已出，用户展开勾选清单并点击确认替换
- **WHEN** 在二次确认弹窗中取消
- **THEN** SHALL NOT 发起生成请求，既有日报保持不变

#### Scenario: 确认后才发起重新生成

- **GIVEN** 同上
- **WHEN** 在二次确认弹窗中确认
- **THEN** 发起重新生成请求，按钮进入进度态

#### Scenario: 首次生成不需要二次确认

- **GIVEN** 今日日报尚未生成（按钮为「立即生成今日日报」）
- **WHEN** 用户在勾选清单中确认
- **THEN** 直接发起生成，SHALL NOT 弹出二次确认

### Requirement: 日报重建窗口守卫

<!-- from: daily-report-system (2026-05-26 board-interaction-overhaul) -->

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

### Requirement: 日报终态通知的计数口径与 WS 终态消息一致

<!-- from: daily-report-system (2026-05-26 board-interaction-overhaul) -->

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
